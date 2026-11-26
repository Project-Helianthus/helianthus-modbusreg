package reader

import (
	"context"
	"math"
	"reflect"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
)

const (
	identifiedBuild = "wc3_26_34_0"
	otherBuild      = "wc3_24_44_3"
)

// vitalsMessage encodes the members of one vitals message; vitalsReply wraps
// it as the response member that carries it.
func vitalsMessage(members ...[]byte) []byte {
	var out []byte
	for _, m := range members {
		out = append(out, m...)
	}
	return out
}

func vitalsReply(members ...[]byte) []byte {
	message := vitalsMessage(members...)
	return append(append([]byte{0x0a}, varint(uint64(len(message)))...), message...)
}

func flagMember(number uint64, value bool) []byte {
	return append(varint(number<<3), map[bool]byte{false: 0, true: 1}[value])
}

func floatBits(f float32) []byte {
	bits := math.Float32bits(f)
	return []byte{0x0d, byte(bits), byte(bits >> 8), byte(bits >> 16), byte(bits >> 24)}
}

func wrapperMember(number uint64, inner []byte) []byte {
	return append(append(varint(number<<3|2), varint(uint64(len(inner)))...), inner...)
}

func vitalsRcpt(lifecycle, instance, generation, id uint64, offset time.Duration) Receipt {
	r := rcpt(id, offset)
	r.Lifecycle, r.Instance, r.Generation = lifecycle, instance, generation
	return r
}

func vehicle(connected, present bool) tesla.WCVitals {
	return tesla.WCVitals{VehicleConnected: tesla.WCVitalsField[bool]{Value: connected, Present: present}}
}

func connectionOf(t *testing.T, f *VitalsFeed) Fact {
	t.Helper()
	fact, ok := f.View().Fact(ElementConnection)
	if !ok {
		t.Fatalf("no connection fact in %+v", f.View())
	}
	return fact
}

// Field presence. Each case changes only the wire presence or value of the
// vehicle flag; a production mapping that reads the value without the presence,
// or that publishes nothing for an omitted flag, fails one of the rows.
func TestVitalsConnectionFollowsTheVehicleFlag(t *testing.T) {
	for name, tc := range map[string]struct {
		vitals tesla.WCVitals
		want   string
	}{
		"present and true":            {vehicle(true, true), SymbolConnectionConnected},
		"present and false":           {vehicle(false, true), SymbolConnectionDisconnected},
		"omitted reads as not set":    {vehicle(false, false), SymbolConnectionDisconnected},
		"value without presence true": {vehicle(true, false), SymbolConnectionDisconnected},
	} {
		t.Run(name, func(t *testing.T) {
			f := NewVitalsFeed()
			if !f.Vitals(VitalsObservation{Vitals: tc.vitals, Receipt: vitalsRcpt(1, 1, 1, 1, time.Second)}) {
				t.Fatal("the reply was not taken")
			}
			fact := connectionOf(t, f)
			if fact.Symbol != tc.want || fact.Role != RoleConnector {
				t.Fatalf("got %+v, want %s on the connector", fact, tc.want)
			}
		})
	}
}

func TestVitalsFeedIsEmptyBeforeAnyReply(t *testing.T) {
	f := NewVitalsFeed()
	if v := f.View(); len(v.Facts) != 0 || len(v.Dispositions) != 0 {
		t.Fatalf("view %+v", v)
	}
	if _, ok := f.Last(); ok {
		t.Fatal("a reply is held")
	}
}

// Generation replacement. The held reply is lifecycle 2, instance 3,
// generation 4, request 10 at +10 s, connected. Every row changes one field of
// the receipt and names the production change it catches.
func TestVitalsReplacementFollowsTheReceiptAxes(t *testing.T) {
	held := vitalsRcpt(2, 3, 4, 10, 10*time.Second)
	for name, tc := range map[string]struct {
		receipt  Receipt
		replaces bool
	}{
		// Lifecycle: ordering on the lifecycle first; ignoring it accepts the older one.
		"older lifecycle with a later receipt": {vitalsRcpt(1, 3, 4, 11, 20*time.Second), false},
		"newer lifecycle with an earlier time": {vitalsRcpt(3, 3, 4, 11, 5*time.Second), true},
		// Instance: ignoring it accepts a reply of a superseded endpoint instance.
		"older instance with a later receipt": {vitalsRcpt(2, 2, 4, 11, 20*time.Second), false},
		"newer instance with an earlier time": {vitalsRcpt(2, 4, 4, 11, 5*time.Second), true},
		// Generation: a reply of an earlier transport generation never replaces the held one.
		"older generation with a later receipt": {vitalsRcpt(2, 3, 3, 11, 20*time.Second), false},
		"newer generation with an earlier time": {vitalsRcpt(2, 3, 5, 11, 5*time.Second), true},
		// Request identity and time inside one generation: only a strictly later reply replaces.
		"the same receipt again":                  {vitalsRcpt(2, 3, 4, 10, 10*time.Second), false},
		"same generation, earlier time":           {vitalsRcpt(2, 3, 4, 12, 9*time.Second), false},
		"same generation, same time, new request": {vitalsRcpt(2, 3, 4, 12, 10*time.Second), false},
		"same generation, later time":             {vitalsRcpt(2, 3, 4, 12, 11*time.Second), true},
	} {
		t.Run(name, func(t *testing.T) {
			f := NewVitalsFeed()
			f.Vitals(VitalsObservation{Vitals: vehicle(true, true), Receipt: held})
			got := f.Vitals(VitalsObservation{Vitals: vehicle(false, true), Receipt: tc.receipt})
			if got != tc.replaces {
				t.Fatalf("taken = %v, want %v", got, tc.replaces)
			}
			wantSymbol, wantReceipt := SymbolConnectionConnected, held
			if tc.replaces {
				wantSymbol, wantReceipt = SymbolConnectionDisconnected, tc.receipt
			}
			fact := connectionOf(t, f)
			if fact.Symbol != wantSymbol || fact.Receipt != wantReceipt {
				t.Fatalf("fact %+v, want %s from %+v", fact, wantSymbol, wantReceipt)
			}
			if last, _ := f.Last(); last.Receipt != wantReceipt {
				t.Fatalf("the reply held carries %+v", last.Receipt)
			}
		})
	}
}

func TestVitalsReplacementOrdersByWallTimeAcrossClockEpochs(t *testing.T) {
	f := NewVitalsFeed()
	held := vitalsRcpt(1, 1, 1, 1, 10*time.Second)
	f.Vitals(VitalsObservation{Vitals: vehicle(true, true), Receipt: held})
	other := vitalsRcpt(1, 1, 1, 2, 0)
	other.ClockEpoch, other.Wall = "other", held.Wall.Add(time.Second)
	if !f.Vitals(VitalsObservation{Vitals: vehicle(false, true), Receipt: other}) {
		t.Fatal("a later wall receipt of another clock epoch was not taken")
	}
	earlier := vitalsRcpt(1, 1, 1, 3, time.Hour)
	earlier.ClockEpoch, earlier.Wall = "third", held.Wall
	if f.Vitals(VitalsObservation{Vitals: vehicle(true, true), Receipt: earlier}) {
		t.Fatal("an earlier wall receipt was taken on a monotonic value of another epoch")
	}
}

// Units and phase identity. Every member is filled; only the connection is
// published, on the connector, and no phase-subject fact or disposition exists.
func TestVitalsProjectNothingButTheConnectionWhateverTheOtherMembersHold(t *testing.T) {
	filled := tesla.WCVitals{}
	v := reflect.ValueOf(&filled).Elem()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		field.FieldByName("Present").SetBool(true)
	}
	filled.VehicleConnected.Value = true
	filled.InstantaneousPowerWatts.Value = tesla.WCVitalsField[float32]{Value: 7400, Present: true}
	filled.InstantaneousLineCurrent.Value = tesla.WCVitalsField[float32]{Value: 32, Present: true}
	filled.VehicleAC.Value = tesla.WCVitalsVehicleAC{VoltageVrms: tesla.WCVitalsField[float32]{Value: 230, Present: true}}
	f := NewVitalsFeed()
	f.Vitals(VitalsObservation{Vitals: filled, Receipt: vitalsRcpt(1, 1, 1, 1, time.Second)})
	view := f.View()
	if len(view.Facts) != 1 || len(view.Dispositions) != 0 {
		t.Fatalf("view %+v", view)
	}
	fact := view.Facts[0]
	if fact.Element != ElementConnection || fact.Role != RoleConnector || fact.Amps != 0 || !fact.Time.IsZero() {
		t.Fatalf("fact %+v", fact)
	}
}

func TestVitalsNativeOnlyListNamesEveryOtherMemberOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, native := range NativeOnlyVitals() {
		if seen[native.Field] || native.Reason == "" {
			t.Fatalf("entry %+v is repeated or without a reason", native)
		}
		seen[native.Field] = true
	}
	typ := reflect.TypeOf(tesla.WCVitals{})
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if name == "VehicleConnected" {
			if seen[name] {
				t.Fatalf("%s is projected and also listed as native", name)
			}
			continue
		}
		if !seen[name] {
			t.Fatalf("member %s has no decision", name)
		}
		delete(seen, name)
	}
	if len(seen) != 0 {
		t.Fatalf("entries name no member: %v", seen)
	}
}

// Units are those the specification states for the member (TEDAPI
// Specification §6.6.1); the line current's unit is provisional there.
func TestVitalsNativeOnlyMembersKeepTheirSpecifiedUnits(t *testing.T) {
	want := map[string]string{
		"UptimeSeconds": "s", "SessionDurationSeconds": "s", "PCBATempC": "°C", "HandleTempC": "°C", "MCUTempC": "°C",
		"InputThermopileV": "V", "ProxV": "V", "PilotHighV": "V", "PilotLowV": "V", "VehicleAC": "V (rms)",
		"GroundResistanceOhms": "Ω", "LineEarthVrms": "V (rms)", "NeutralEarthVrms": "V (rms)",
		"InstantaneousPowerWatts": "W", "InstantaneousLineCurrent": "A (Provisional)",
		"ContactorClosed": "", "FaultStatus": "", "VIN": "", "ScheduledChargingStatus": "", "OCPPStatus": "",
		"PowershareSessionState": "", "DryContactControlState": "", "SmartChargingReason": "",
	}
	for _, native := range NativeOnlyVitals() {
		if unit, ok := want[native.Field]; !ok || unit != native.Unit {
			t.Fatalf("%s: unit %q, want %q (listed %v)", native.Field, native.Unit, unit, ok)
		}
		delete(want, native.Field)
	}
	if len(want) != 0 {
		t.Fatalf("members missing from the list: %v", want)
	}
}

// vitalsRun starts a reader on the scripted line with the build label given
// and the operations configured; push scripts the polls the first cycle makes.
func vitalsRun(t *testing.T, build string, cfg ChargerConfig, push func(*scriptedEndpoint)) (*ChargerReader, *fakeSession, *charger, *clock) {
	t.Helper()
	cfg.Version = build
	c := newCharger(t)
	clk := &clock{at: time.Unix(5, 0)}
	c.deps.Now = clk.now
	open := c.deps.Open
	c.deps.Open = func(config modbus.RTUProductionConfig) (tesla.Gen3ReadEndpoint, error) {
		endpoint, err := open(config)
		if err == nil && c.opens == 1 {
			push(c.endpoint)
		}
		return endpoint, err
	}
	reader, err := NewChargerReader(cfg, syntheticPublication(), c.deps)
	if err != nil || reader == nil {
		t.Fatalf("reader %v err %v", reader, err)
	}
	session := &fakeSession{}
	if err := reader.Start(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Stop(context.Background()) })
	return reader, session, c, clk
}

func vitalsOnlyConfig() ChargerConfig {
	return syntheticConfig(tesla.FC100OperationWCGetVitals)
}

func pushVitals(e *scriptedEndpoint, body []byte) {
	e.push(step{echo: true, terminal: terminal(6, 2, body)})
}

func TestVitalsReadPublishesTheConnectionOnTheConnector(t *testing.T) {
	reader, session, _, _ := vitalsRun(t, identifiedBuild, vitalsOnlyConfig(), func(e *scriptedEndpoint) {
		pushVitals(e, vitalsReply(flagMember(3, true), flagMember(4, true), wrapperMember(22, floatBits(7400)), wrapperMember(24, floatBits(32)),
			wrapperMember(15, floatBits(230))))
	})
	waitFor(t, "the first batch", func() bool { return len(session.committed()) == 1 })
	fact := candidateOf(t, session.committed(), ElementConnection)
	if fact.Value == nil || fact.Value.Symbol == nil || fact.Value.Symbol.Namespace != "evse.status.connection" || fact.Value.Symbol.Token != "connected" {
		t.Fatalf("value %+v", fact.Value)
	}
	if got := fact.Key.Dimensions[0]; got.ID != "evse.dimension.connector" || *got.Value.Text != "connector-a" {
		t.Fatalf("the fact is not on the connector subject: %+v", got)
	}
	if fact.Evidence[0].Kind != "native.tesla.wc3.vitals" {
		t.Fatalf("evidence kind %s", fact.Evidence[0].Kind)
	}
	published := map[string]bool{}
	for _, f := range session.committed()[0].FactUpserts {
		published[string(f.Key.FactID)] = true
	}
	if len(published) != 1 || !published[ElementConnection] {
		t.Fatalf("published elements %v", published)
	}
	if last, ok := reader.Vitals().Last(); !ok || !last.Vitals.InstantaneousPowerWatts.Present || last.Vitals.InstantaneousPowerWatts.Value.Value != 7400 {
		t.Fatalf("the typed vitals are not retained as received: %+v", last)
	}
}

// A later read of the same build replaces the state; the omitted flag reads as
// not connected and the fact's revision advances.
func TestVitalsLaterReadReplacesTheConnection(t *testing.T) {
	reader, session, c, clk := vitalsRun(t, identifiedBuild, vitalsOnlyConfig(), func(e *scriptedEndpoint) {
		pushVitals(e, vitalsReply(flagMember(4, true)))
	})
	waitFor(t, "the first batch", func() bool { return len(session.committed()) == 1 })
	first := candidateOf(t, session.committed(), ElementConnection)
	pushVitals(c.endpoint, vitalsReply(flagMember(3, false)))
	clk.set(time.Unix(100, 0))
	records, err := reader.Acquisition().PollOnce(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("poll: %v %d", err, len(records))
	}
	if err := reader.IngestPolled(context.Background(), records[0]); err != nil {
		t.Fatal(err)
	}
	if len(session.committed()) != 2 {
		t.Fatalf("batches %d", len(session.committed()))
	}
	second := candidateOf(t, session.committed(), ElementConnection)
	if second.Value.Symbol.Token != "disconnected" || second.Revision == first.Revision {
		t.Fatalf("first %+v second %+v", first.Value.Symbol, second.Value.Symbol)
	}
}

// A reply without the vitals member and a build without typed vitals never
// make a fact: nothing is made up from an absent member.
func TestVitalsWithoutTypedMessageLeaveNoFact(t *testing.T) {
	for name, tc := range map[string]struct {
		build string
		body  []byte
	}{
		"identified build, no vitals member": {identifiedBuild, []byte{0x7a, 0x02, 0xaa, 0xbb}},
		"other build, vitals member present": {otherBuild, vitalsReply(flagMember(4, true))},
	} {
		t.Run(name, func(t *testing.T) {
			reader, session, c, _ := vitalsRun(t, tc.build, vitalsOnlyConfig(), func(e *scriptedEndpoint) { pushVitals(e, tc.body) })
			waitFor(t, "the poll", func() bool { return len(c.endpoint.sent()) == 1 })
			waitFor(t, "the retained read", func() bool {
				retained, ok := reader.Acquisition().Retained(tesla.FC100OperationWCGetVitals)
				return ok && retained.HasLastGood
			})
			if _, ok := reader.View().Fact(ElementConnection); ok {
				t.Fatal("a connection fact was made")
			}
			if _, ok := reader.Vitals().Last(); ok {
				t.Fatal("a reply was fed")
			}
			for _, batch := range session.committed() {
				for _, f := range batch.FactUpserts {
					if string(f.Key.FactID) == ElementConnection {
						t.Fatal("a connection candidate was staged")
					}
				}
			}
		})
	}
}

// Only an acknowledged read of the vitals operation of this endpoint and node
// is fed. Each row changes one field of a later real read and must change nothing.
func TestVitalsReadOfAnotherIdentityIsNotFed(t *testing.T) {
	reader, session, c, clk := vitalsRun(t, identifiedBuild, vitalsOnlyConfig(), func(e *scriptedEndpoint) {
		pushVitals(e, vitalsReply(flagMember(4, true)))
	})
	waitFor(t, "the first batch", func() bool { return len(session.committed()) == 1 })
	held, _ := reader.Vitals().Last()
	pushVitals(c.endpoint, vitalsReply(flagMember(3, false)))
	clk.set(time.Unix(100, 0))
	records, err := reader.Acquisition().PollOnce(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("poll: %v %d", err, len(records))
	}
	for _, name := range []string{"another endpoint", "another node", "no request", "another operation", "not acknowledged"} {
		record := records[0]
		switch name {
		case "another endpoint":
			record.Endpoint = "another-endpoint"
		case "another node":
			record.Node++
		case "no request":
			record.RequestID = 0
		case "another operation":
			record.Operation = tesla.FC100OperationWCGetLifetimeStats
		case "not acknowledged":
			record.State = tesla.Gen3ReadEvidenceIndeterminate
		}
		if reader.ingestVitals(record) {
			t.Fatalf("%s: the read was fed", name)
		}
		if last, _ := reader.Vitals().Last(); last.Receipt != held.Receipt {
			t.Fatalf("%s: the reply held changed", name)
		}
	}
	if !reader.ingestVitals(records[0]) {
		t.Fatal("the unchanged read was not fed")
	}
}

// The vitals leave the readback facts as the readback produced them, and the
// readback facts leave the connection as the vitals produced it.
func TestVitalsAndReadbackFactsStayIndependent(t *testing.T) {
	cfg := syntheticConfig(append(append([]tesla.FC100Operation(nil), fiveOperations...), tesla.FC100OperationWCGetProvisional)...)
	cfg.ProvisionalReadbackQualified = true
	reader, _, _, _ := vitalsRun(t, identifiedBuild, cfg, func(e *scriptedEndpoint) {
		for _, op := range cfg.Operations {
			if op == tesla.FC100OperationWCGetVitals {
				pushVitals(e, vitalsReply(flagMember(4, true)))
				continue
			}
			pushPolls(e, []tesla.FC100Operation{op}, scalars{limit: u(16), timeout: u(600), inhibit: b(false), configured: u(32)})
		}
	})
	waitFor(t, "the batch with the readback", func() bool {
		view := reader.View()
		_, a := view.Fact(ElementConnection)
		_, b := view.Fact(ElementAllocatedCurrent)
		return a && b
	})
	view := reader.View()
	wantAmps(t, view, ElementConfiguredCurrent, 32)
	wantAmps(t, view, ElementAllocatedCurrent, 16)
	if fact, _ := view.Fact(ElementConnection); fact.Symbol != SymbolConnectionConnected {
		t.Fatalf("connection %+v", fact)
	}
	if err := reader.IngestPersistent(context.Background(), tesla.PersistentSettingsResult{}); err != nil {
		t.Fatal(err)
	}
	if fact, ok := reader.View().Fact(ElementConnection); !ok || fact.Symbol != SymbolConnectionConnected {
		t.Fatalf("an unrelated feed changed the connection: %+v", fact)
	}
}
