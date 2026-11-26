package reader

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/packs/evse"
)

// nativeSetTimeoutSeconds is the one timeout the native setter admits with a
// positive limit; the donor's request carried another, so the replayed readback
// differs from the request and is fed as returned.
const nativeSetTimeoutSeconds = 900

const goldenDir = "testdata/tesla_owner_first_incarnation"

type donorFrame struct {
	ResponseFrames []struct{ Payload string }
	ReceiptWall    time.Time
	// ReceiptMonotonic is on the donor's single clock, in nanoseconds.
	ReceiptMonotonic int64
}

func (f donorFrame) terminal(t *testing.T) []byte {
	t.Helper()
	if len(f.ResponseFrames) != 1 {
		t.Fatalf("donor frame carries %d response frames", len(f.ResponseFrames))
	}
	raw, err := base64.StdEncoding.DecodeString(f.ResponseFrames[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type donorSession struct {
	Input struct {
		Config struct {
			AssetID, SourceID, SourceEpoch, ClockEpoch, EVSEID, ConnectorID string
			DriverGeneration                                                uint64
		}
		Steps []struct {
			Kind       string
			Persistent *struct {
				Exchange             donorFrame
				MaxOutputCurrentAmps uint32
			}
			Provisional *struct {
				Set, Readback       donorFrame
				LimitCurrentMaxAmps uint32
				LimitTimeoutSeconds uint32
				InhibitCharging     bool
			}
		}
	}
}

type donorSequence struct {
	cfg                             donorSession
	persistentFirst, persistentNext donorFrame
	set, readback                   donorFrame
	// limit, timeout and configured are the t28 scalars, read from the frame
	// bytes by a decoder that does not share code with the reader.
	limit, timeout, configured uint32
	inhibit                    bool
	persistentNextAmps         uint32
}

func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(goldenDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// scanVarints reads one protobuf-shaped message into its varint fields and
// its length-delimited fields.
func scanFields(t *testing.T, raw []byte) (varints map[uint64]uint64, nested map[uint64][]byte) {
	t.Helper()
	varints, nested = map[uint64]uint64{}, map[uint64][]byte{}
	read := func() uint64 {
		var v uint64
		for shift := uint(0); ; shift += 7 {
			if len(raw) == 0 {
				t.Fatal("truncated varint")
			}
			c := raw[0]
			raw = raw[1:]
			v |= uint64(c&0x7f) << shift
			if c&0x80 == 0 {
				return v
			}
		}
	}
	for len(raw) > 0 {
		tag := read()
		switch tag & 7 {
		case 0:
			varints[tag>>3] = read()
		case 2:
			n := read()
			nested[tag>>3], raw = raw[:n], raw[n:]
		default:
			t.Fatalf("wire type %d", tag&7)
		}
	}
	return varints, nested
}

// terminalBody strips the length byte and the family and tag envelopes of a
// terminal frame payload and returns the body of the operation's message.
func terminalBody(t *testing.T, payload []byte, tag uint64) []byte {
	t.Helper()
	if int(payload[0]) != len(payload)-1 {
		t.Fatalf("terminal length byte %d for %d bytes", payload[0], len(payload)-1)
	}
	_, family := scanFields(t, payload[1:])
	_, inner := scanFields(t, family[6])
	return inner[tag]
}

func loadDonor(t *testing.T) donorSequence {
	t.Helper()
	var s donorSession
	if err := json.Unmarshal(readGolden(t, "donor_session.json"), &s); err != nil {
		t.Fatal(err)
	}
	d := donorSequence{cfg: s}
	var persistent []donorFrame
	var amps []uint32
	for _, step := range s.Input.Steps {
		switch step.Kind {
		case "ingest-persistent":
			persistent = append(persistent, step.Persistent.Exchange)
			amps = append(amps, step.Persistent.MaxOutputCurrentAmps)
		case "ingest-provisional":
			d.set, d.readback = step.Provisional.Set, step.Provisional.Readback
		}
	}
	if len(persistent) != 2 {
		t.Fatalf("donor session has %d persistent exchanges", len(persistent))
	}
	d.persistentFirst, d.persistentNext, d.persistentNextAmps = persistent[0], persistent[1], amps[1]
	body := terminalBody(t, d.readback.terminal(t), 28)
	top, nested := scanFields(t, body)
	d.configured = uint32(top[2])
	limits, _ := scanFields(t, nested[1])
	d.limit, d.timeout, d.inhibit = uint32(limits[1]), uint32(limits[2]), limits[3] == 1
	return d
}

func (d donorSequence) step(f donorFrame, echo bool, payload []byte) step {
	return step{echo: echo, terminal: payload, wall: f.ReceiptWall.Unix(), epoch: d.cfg.Input.Config.ClockEpoch,
		mono: time.Duration(f.ReceiptMonotonic)}
}

// donorClock is the injected clock of the replay: the donor session's wall
// instants for each owner step.
func donorClock(base time.Time, offset time.Duration) time.Time { return base.Add(offset) }

type replayInput struct {
	persistentFirst, ack, readback, persistentNext []byte
}

func (d donorSequence) payloads(t *testing.T) replayInput {
	// The documented layout acknowledges t25 with a reply that carries the
	// request's own fields; the donor's acknowledgement is the empty t26 of
	// another layout, so the reply is built from the request the setter sends.
	ack := terminal(6, 25, readbackBody(scalars{limit: u(d.limit), timeout: u(nativeSetTimeoutSeconds), inhibit: b(false)}))
	return replayInput{persistentFirst: d.persistentFirst.terminal(t), ack: ack, readback: d.readback.terminal(t), persistentNext: d.persistentNext.terminal(t)}
}

type replayCut struct {
	snapshot semreg.Snapshot
	batches  []semreg.PublicationBatch
	// dispositions holds, per element, the reason of the latest value-less
	// candidate the reader published for it; an element with a value has none.
	dispositions map[string]string
	// priorRevisions counts, per candidate, the value-less publications that
	// precede its latest value in the reader's revision counter.
	priorRevisions map[semreg.CandidateID]uint64
}

// replay runs the donor's three owner steps through the running reader on the
// donor's wall clock and applies every committed batch to a publication kernel
// that carries the EVSE pack, returning the cut after sequences two and three.
func (d donorSequence) replay(t *testing.T, in replayInput) (sequence2, sequence3 replayCut) {
	t.Helper()
	cfg := mutationConfig()
	cfg.Version, cfg.PersistentSettingsQualified = "26.34.0", true
	// The donor's receipts sit up to four seconds after the scripted send, so
	// the response bounds are wide enough to admit them.
	cfg.ResponseTimeout, cfg.MaxResponseDelay, cfg.MaxQuiescence, cfg.AcquisitionDeadline = 10*time.Second, 5*time.Second, 9*time.Second, 60*time.Second
	c := d.cfg.Input.Config
	pub := ChargerPublication{
		Asset: semreg.AssetID(c.AssetID), Source: semreg.SourceID(c.SourceID), Epoch: semreg.SourceEpochID(c.SourceEpoch),
		Generation: c.DriverGeneration, ClockEpoch: "clock:charger-reader", EVSE: c.EVSEID, Connector: c.ConnectorID,
		Policy: syntheticPublication().Policy, FirstIncarnation: true,
	}
	reader, session, ep, clk := runWith(t, cfg, pub, nil)
	waitFor(t, "the first poll and its structural batch", func() bool { return len(ep.endpoint.sent()) == 1 && len(session.committed()) == 1 })
	base := d.persistentFirst.ReceiptWall

	clk.set(donorClock(base, 0))
	ep.endpoint.push(d.step(d.persistentFirst, false, in.persistentFirst))
	initial := persistentResult(t, reader, "donor-t8-first", d.persistentFirst, 16)
	if err := reader.IngestPersistent(context.Background(), initial); err != nil {
		t.Fatal(err)
	}

	clk.set(donorClock(base, 3*time.Second))
	// The documented layout's setter first queries the installation maximum, a
	// terminal of its own that carries the settings body of the donor's t8.
	query := d.persistentFirst
	query.ReceiptMonotonic += int64(500 * time.Millisecond)
	installation := terminal(6, 6, terminalBody(t, in.persistentFirst, 8))
	ep.endpoint.push(d.step(query, true, installation), d.step(d.set, false, in.ack), d.step(d.readback, true, in.readback))
	result := settle(t, reader, tesla.ProvisionalCurrentLimitRequest{LimitCurrentMaxAmps: d.limit, LimitTimeoutSeconds: nativeSetTimeoutSeconds})
	if err := reader.IngestSetResult(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	sequence2 = cutOf(t, d, session)
	original, held := reader.Feed().LastProvisional()
	if !held {
		t.Fatal("the mutation's t28 is not retained")
	}

	clk.set(donorClock(base, 6*time.Second))
	ep.endpoint.push(d.step(d.persistentNext, false, in.persistentNext))
	next := persistentResult(t, reader, "donor-t8-next", d.persistentNext, 18)
	if err := reader.IngestPersistent(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	if retained, held := reader.Feed().LastProvisional(); !held || retained != original {
		t.Fatalf("the persistent-only successor changed the retained t28 or its timing: %+v", retained)
	}
	return sequence2, cutOf(t, d, session)
}

func persistentResult(t *testing.T, reader *ChargerReader, identity string, frame donorFrame, request int32) tesla.PersistentSettingsResult {
	t.Helper()
	var result tesla.PersistentSettingsResult
	waitFor(t, "t7/t8 "+identity, func() bool {
		var err error
		result, err = reader.Acquisition().ConfigurePersistentCurrent(context.Background(), tesla.PersistentSettingsRequest{
			MaxOutputCurrentAmps: request, OperationIdentity: identity, Deadline: time.Now().Add(5 * time.Second)})
		return err == nil || result.Reason != tesla.ErrTeslaGen3ReadAcquisitionBusy.Error()
	})
	return result
}

// cutOf applies every batch the session committed so far to a fresh kernel and
// returns the resulting snapshot. The source descriptor is the one the host
// declares; it is taken from the golden cut.
func cutOf(t *testing.T, d donorSequence, session *fakeSession) replayCut {
	t.Helper()
	var golden semreg.Snapshot
	if err := json.Unmarshal(readGolden(t, "sequence2.canonical.json"), &golden); err != nil {
		t.Fatal(err)
	}
	return applyToKernel(t, semreg.AssetID(d.cfg.Input.Config.AssetID), golden.Sources, session)
}

func applyToKernel(t *testing.T, asset semreg.AssetID, sources []semreg.SourceDescriptor, session *fakeSession) replayCut {
	t.Helper()
	kernel, err := semreg.NewPublicationKernel(asset, evse.New())
	if err != nil {
		t.Fatal(err)
	}
	var snapshot semreg.Snapshot
	batches := session.committed()
	dispositions, prior := map[string]string{}, map[semreg.CandidateID]uint64{}
	for i, batch := range batches {
		if i == 0 {
			batch.SourceUpserts = append([]semreg.SourceDescriptor{}, sources...)
		}
		// The EVSE pack validator refuses a candidate without a value and the
		// setter capability without its typed constraint, so the cut is applied
		// with a value-less candidate as the withdrawal of the candidate it
		// replaces (the gateway's own form) and without the setter capability;
		// every other record is validated by the kernel.
		kept := batch.CapabilityUpserts[:0:0]
		for _, capability := range batch.CapabilityUpserts {
			if capability.Definition.ID != SetAllocated.Capability {
				kept = append(kept, capability)
			}
		}
		batch.CapabilityUpserts = kept
		upserts := batch.FactUpserts[:0:0]
		withdrawals := append([]semreg.CandidateID{}, batch.FactWithdrawals...)
		present := candidateByID(snapshot)
		for _, f := range batch.FactUpserts {
			if f.Value != nil {
				upserts = append(upserts, f)
				delete(dispositions, string(f.Key.FactID))
				continue
			}
			prior[f.CandidateID]++
			dispositions[string(f.Key.FactID)] = string(f.Quality.Reasons[0])
			if _, ok := present[f.CandidateID]; ok {
				withdrawals = append(withdrawals, f.CandidateID)
			}
		}
		batch.FactUpserts, batch.FactWithdrawals = upserts, withdrawals
		sort.Slice(batch.FactWithdrawals, func(a, b int) bool { return batch.FactWithdrawals[a] < batch.FactWithdrawals[b] })
		digest, err := batch.ComputedDigest()
		if err != nil {
			t.Fatal(err)
		}
		batch.BatchDigest = digest
		wallNS, _ := strconv.ParseInt(string(batch.ObservedAt.UnixNanoseconds), 10, 64)
		mono := semreg.MonotonicPoint{ClockEpochID: "clock:charger-reader", Nanoseconds: semreg.Uint64(strconv.FormatInt(wallNS, 10))}
		snapshot, _, err = kernel.Apply(batch, mono)
		if err != nil {
			t.Fatalf("batch %d is refused by the kernel: %v", i+1, err)
		}
	}
	return replayCut{snapshot: snapshot, batches: batches, dispositions: dispositions, priorRevisions: prior}
}

func identitySummary(snapshot semreg.Snapshot) map[string][]string {
	summary := map[string][]string{}
	add := func(kind string, fields ...string) { summary[kind] = append(summary[kind], strings.Join(fields, " ")) }
	for _, b := range snapshot.Bindings {
		add("bindings", string(b.BindingID), string(b.SourceID), string(b.SourceEpochID), string(b.DriverGeneration), string(b.State))
	}
	for _, s := range snapshot.Services {
		add("services", string(s.InstanceID), string(s.Definition.ID), string(s.BindingID))
	}
	for _, c := range snapshot.Capabilities {
		add("capabilities", string(c.InstanceID), string(c.Definition.ID), string(c.ServiceInstance), string(c.BindingID))
	}
	for _, e := range snapshot.Facts {
		for _, c := range e.Candidates {
			add("candidates", string(c.CandidateID), string(e.Key.FactID), string(c.Origin.OriginID))
		}
	}
	for _, c := range snapshot.Cursors {
		add("cursors", string(c.SourceID), string(c.SourceEpochID), string(c.DriverGeneration), string(c.LastSequence))
	}
	for _, v := range summary {
		sort.Strings(v)
	}
	return summary
}

// destinationOnly are the elements the gateway's first incarnation does not
// publish: the time-valued expiry of the allocation and the supply status.
var destinationOnly = map[string]bool{ElementAllocatedExpiry: true, ElementSupply: true}

func candidateByID(snapshot semreg.Snapshot) map[semreg.CandidateID]semreg.FactCandidate {
	out := map[semreg.CandidateID]semreg.FactCandidate{}
	for _, e := range snapshot.Facts {
		for _, c := range e.Candidates {
			out[c.CandidateID] = c
		}
	}
	return out
}

func asJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

// compareCut lists every difference between the reader's cut and the gateway's
// golden cut, after the differences that the slice states. wantConfigured and
// configuredReceipt are the t28 configured current and the receipt that takes
// precedence for it; they are read from the donor frames, never from the
// reader's output.
type cutExpectation struct {
	sequence string
	// configured is the configured current of the cut and its receipt, when it
	// differs from the golden's t8 candidate.
	configured        *uint32
	configuredReceipt *donorFrame
	allocated         *uint32
	expiry            *time.Time
}

func compareCut(t *testing.T, got replayCut, want cutExpectation) []string {
	t.Helper()
	var golden semreg.Snapshot
	if err := json.Unmarshal(readGolden(t, want.sequence+".canonical.json"), &golden); err != nil {
		t.Fatal(err)
	}
	var ids map[string][]string
	if err := json.Unmarshal(readGolden(t, want.sequence+".ids.json"), &ids); err != nil {
		t.Fatal(err)
	}
	var diffs []string
	report := func(format string, args ...any) { diffs = append(diffs, fmt.Sprintf(format, args...)) }

	have := candidateByID(got.snapshot)
	goldenCandidates := candidateByID(golden)
	// Candidates the reader publishes beyond the golden are destination-only
	// or value-less withheld dispositions; nothing else may be extra.
	extra := map[semreg.CandidateID]semreg.FactCandidate{}
	for id, c := range have {
		if _, ok := goldenCandidates[id]; !ok {
			extra[id] = c
		}
	}
	for id, c := range extra {
		element := string(c.Key.FactID)
		if !destinationOnly[element] && c.Value != nil {
			report("candidate %s (%s) is not in the golden cut", id, element)
		}
	}
	// Identifier families: the golden summary, less the families the reader's
	// publication does not carry (the host-declared source and the native
	// identity link) and less the extra candidates.
	summary := identitySummary(got.snapshot)
	for id := range extra {
		element := string(extra[id].Key.FactID)
		trimmed := summary["candidates"][:0:0]
		for _, line := range summary["candidates"] {
			if !strings.HasPrefix(line, string(id)+" "+element+" ") {
				trimmed = append(trimmed, line)
			}
		}
		summary["candidates"] = trimmed
	}
	// The reader publishes the binding, services and capabilities in a batch of
	// their own when it starts, before any observation; every later sequence
	// number is that many above the gateway's.
	ahead := 0
	for _, b := range got.batches {
		if len(b.FactUpserts) == 0 && len(b.BindingUpserts) != 0 {
			ahead++
		}
	}
	for i, line := range ids["cursors"] {
		fields := strings.Fields(line)
		n, _ := strconv.Atoi(fields[len(fields)-1])
		fields[len(fields)-1] = strconv.Itoa(n + ahead)
		ids["cursors"][i] = strings.Join(fields, " ")
	}
	for _, family := range []string{"bindings", "services", "capabilities", "candidates", "cursors"} {
		if !reflect.DeepEqual(summary[family], ids[family]) {
			report("identifiers %s: got %q, golden %q", family, summary[family], ids[family])
		}
	}

	for id, g := range goldenCandidates {
		c, ok := have[id]
		if !ok {
			report("golden candidate %s (%s) is missing", id, g.Key.FactID)
			continue
		}
		element := string(g.Key.FactID)
		wantValue, wantReceived, wantMono := g.Value, g.Times.ReceivedAt, g.Times.ReceiptMonotonic
		if element == ElementConfiguredCurrent && want.configured != nil {
			q := semreg.Value{Kind: semreg.ValueQuantity, Quantity: &semreg.Quantity{Unit: "unit.ampere", Number: semreg.Decimal{Coefficient: strconv.FormatUint(uint64(*want.configured), 10)}}}
			wantValue = &q
			wantReceived = semreg.TimePoint{UnixNanoseconds: semreg.Int64(strconv.FormatInt(want.configuredReceipt.ReceiptWall.UnixNano(), 10)), ClockID: g.Times.ReceivedAt.ClockID, UncertaintyNS: g.Times.ReceivedAt.UncertaintyNS}
			wantMono = semreg.MonotonicPoint{ClockEpochID: g.Times.ReceiptMonotonic.ClockEpochID, Nanoseconds: semreg.Uint64(strconv.FormatInt(want.configuredReceipt.ReceiptMonotonic, 10))}
		}
		for name, pair := range map[string][2]any{
			"key": {g.Key, c.Key}, "value": {wantValue, c.Value}, "quality": {g.Quality, c.Quality},
			"received_at": {wantReceived, c.Times.ReceivedAt}, "receipt_monotonic": {wantMono, c.Times.ReceiptMonotonic},
			"binding": {g.BindingID, c.BindingID}, "source_epoch": {g.SourceEpochID, c.SourceEpochID},
			"generation": {g.DriverGeneration, c.DriverGeneration}, "origin": {g.Origin.OriginID, c.Origin.OriginID},
			"revision": {g.Revision, c.Revision},
		} {
			if name == "revision" {
				n, _ := strconv.ParseUint(string(g.Revision), 10, 64)
				pair[0] = semreg.Uint64(strconv.FormatUint(n+got.priorRevisions[id], 10))
			}
			if asJSON(pair[0]) != asJSON(pair[1]) {
				report("%s of %s: got %s, golden %s", name, element, asJSON(pair[1]), asJSON(pair[0]))
			}
		}
	}

	if want.allocated != nil {
		if c, ok := have[candidateIDOf(golden, ElementAllocatedCurrent)]; !ok || c.Value == nil || c.Value.Quantity == nil ||
			c.Value.Quantity.Number.Coefficient != strconv.FormatUint(uint64(*want.allocated), 10) {
			report("allocated current is not %d A", *want.allocated)
		}
	}
	for _, c := range extra {
		if string(c.Key.FactID) != ElementAllocatedExpiry || c.Value == nil || c.Value.Time == nil {
			continue
		}
		if want.expiry == nil {
			report("an expiry is published")
			continue
		}
		if got := c.Value.Time.UnixNanoseconds; string(got) != strconv.FormatInt(want.expiry.UnixNano(), 10) {
			report("expiry %s, want %d", got, want.expiry.UnixNano())
		}
	}
	return diffs
}

func candidateIDOf(snapshot semreg.Snapshot, element string) semreg.CandidateID {
	for _, e := range snapshot.Facts {
		if string(e.Key.FactID) == element && len(e.Candidates) > 0 {
			return e.Candidates[0].CandidateID
		}
	}
	return ""
}

func expectations(d donorSequence) (cutExpectation, cutExpectation) {
	configured := d.configured
	expiry := d.set.ReceiptWall.Add(time.Duration(d.timeout) * time.Second)
	allocated := d.limit
	readback := d.readback
	return cutExpectation{sequence: "sequence2", configured: &configured, configuredReceipt: &readback, allocated: &allocated, expiry: &expiry},
		cutExpectation{sequence: "sequence3", expiry: &expiry}
}

func TestFirstSequenceReplayedThroughTheRunningReaderEqualsTheGatewayGoldens(t *testing.T) {
	d := loadDonor(t)
	two, three := d.replay(t, d.payloads(t))
	want2, want3 := expectations(d)
	for _, tc := range []struct {
		name string
		cut  replayCut
		want cutExpectation
	}{{"sequence 2", two, want2}, {"sequence 3", three, want3}} {
		if diffs := compareCut(t, tc.cut, tc.want); len(diffs) != 0 {
			t.Errorf("%s differs from the golden:\n%s", tc.name, strings.Join(diffs, "\n"))
		}
	}
}

func TestReplayedDispositionsEqualTheGoldenProjection(t *testing.T) {
	d := loadDonor(t)
	two, three := d.replay(t, d.payloads(t))
	for sequence, cut := range map[string]replayCut{"sequence2": two, "sequence3": three} {
		var golden struct {
			Projection struct {
				Dispositions []struct {
					ItemID  string `json:"item_id"`
					Outcome string
					Reason  string
				}
			}
		}
		if err := json.Unmarshal(readGolden(t, sequence+".public.json"), &golden); err != nil {
			t.Fatal(err)
		}
		have := map[string]semreg.FactCandidate{}
		for _, c := range candidateByID(cut.snapshot) {
			have[string(c.Key.FactID)] = c
		}
		for _, disposition := range golden.Projection.Dispositions {
			c, published := have[disposition.ItemID]
			reason, withheld := cut.dispositions[disposition.ItemID]
			switch disposition.Outcome {
			case "exact":
				if !published || withheld || c.Quality.Availability != semreg.AvailabilityAvailable {
					t.Errorf("%s: %s is not exact (published %v, withheld as %q)", sequence, disposition.ItemID, published, reason)
				}
			case "withheld":
				if published || !withheld || reason != disposition.Reason {
					t.Errorf("%s: %s is not withheld as %q (published %v, reason %q)", sequence, disposition.ItemID, disposition.Reason, published, reason)
				}
			default:
				t.Errorf("%s: golden outcome %q", sequence, disposition.Outcome)
			}
		}
	}
}

func TestMutatedValuesFailTheGoldenComparison(t *testing.T) {
	d := loadDonor(t)
	base := d.payloads(t)
	mutate := func(raw []byte, from, to []byte) []byte {
		out := bytes.Replace(raw, from, to, 1)
		if bytes.Equal(out, raw) {
			t.Fatalf("pattern %x not in %x", from, raw)
		}
		return out
	}
	for name, tc := range map[string]struct {
		input replayInput
		cut   int
	}{
		"allocated current 12 A becomes 13 A":        {replayInput{base.persistentFirst, base.ack, mutate(base.readback, []byte{0x08, 0x0c}, []byte{0x08, 0x0d}), base.persistentNext}, 2},
		"configured current 32 A becomes 33 A":       {replayInput{base.persistentFirst, base.ack, mutate(base.readback, []byte{0x10, 0x20}, []byte{0x10, 0x21}), base.persistentNext}, 2},
		"later persistent current 18 A becomes 17 A": {replayInput{base.persistentFirst, base.ack, base.readback, mutate(base.persistentNext, []byte{0x08, 0x12}, []byte{0x08, 0x11})}, 3},
	} {
		t.Run(name, func(t *testing.T) {
			two, three := d.replay(t, tc.input)
			want2, want3 := expectations(d)
			cut, want := two, want2
			if tc.cut == 3 {
				cut, want = three, want3
			}
			if diffs := compareCut(t, cut, want); len(diffs) == 0 {
				t.Fatal("a mutated value passed the comparison")
			}
		})
	}
}

// The first incarnation's identifiers depend on the asset and the source only:
// two lifecycles, in either discovery order, give the same ones, two assets
// give different ones, and a successor generation, which the gateway scopes by
// epoch and generation, gives others.
func TestFirstIncarnationIdentifiersAreStableAndSuccessorsAreScoped(t *testing.T) {
	first := func(asset, source string) ChargerPublication {
		pub := syntheticPublication()
		pub.Asset, pub.Source, pub.FirstIncarnation = semreg.AssetID(asset), semreg.SourceID(source), true
		return pub
	}
	a, b := first("asset:charger-a", "source:charger-a"), first("asset:charger-b", "source:charger-b")
	one := func(pub ChargerPublication) []string {
		_, s := firstBatch(t, sixPollConfig(), pub, nil)
		return idsOf(t, s.committed()[0])
	}
	firstA, firstB := one(a), one(b)
	secondB, secondA := one(b), one(a)
	if !reflect.DeepEqual(firstA, secondA) || !reflect.DeepEqual(firstB, secondB) {
		t.Fatal("identifiers changed across restart and reverse discovery order")
	}
	if reflect.DeepEqual(firstA, firstB) {
		t.Fatal("two assets share their identifiers")
	}
	successor := a
	successor.FirstIncarnation, successor.Epoch, successor.Generation = false, "epoch:charger-a-next", 8
	if next := one(successor); len(next) != len(firstA) || reflect.DeepEqual(next, firstA) {
		t.Fatalf("a successor keeps the first incarnation's identifiers: %q", next)
	}
	for _, id := range firstA {
		if strings.Contains(id, "charger-a") {
			t.Fatalf("identifier %q carries its asset label", id)
		}
	}
}
