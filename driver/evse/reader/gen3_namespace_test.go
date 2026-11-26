package reader

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

func decodeMember(t *testing.T, raw []byte) (NamespaceMember, map[string]any) {
	t.Helper()
	var member NamespaceMember
	var keys map[string]any
	if err := json.Unmarshal(raw, &member); err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	return member, keys
}

func memberNames(r *recordingRegistry) []string {
	var names []string
	for _, m := range r.members {
		names = append(names, string(m.Local))
	}
	sort.Strings(names)
	return names
}

func allSlots() []string {
	var names []string
	for _, spec := range slotSpecs {
		names = append(names, string(spec.slot))
	}
	sort.Strings(names)
	return names
}

// A payload that was never received is absent with a reason, not an empty
// value: the member states presence and reason and has no payload and no
// receipt member at all.
func TestNamespaceStatesAnAbsentPayloadWithItsReason(t *testing.T) {
	cfg := syntheticConfig(fiveOperations...)
	_, session, _ := run(t, cfg, nil)
	if !reflect.DeepEqual(memberNames(&session.native), allSlots()) {
		t.Fatalf("members %v", memberNames(&session.native))
	}
	for _, name := range allSlots() {
		raw, err := session.native.read(driver.MemberID(name))
		if err != nil {
			t.Fatal(err)
		}
		member, keys := decodeMember(t, raw)
		if member.Present || member.Reason != ReasonNeverReceived || member.Payload != nil || member.Receipt != nil {
			t.Fatalf("%s: %s", name, raw)
		}
		if len(keys) != 4 || keys["reason"] != ReasonNeverReceived || keys["present"] != false {
			t.Fatalf("%s: absent member keys %v", name, keys)
		}
		if _, ok := keys["payload"]; ok {
			t.Fatalf("%s carries a payload member while absent", name)
		}
	}
}

func TestNamespaceDeclaresOneNodePerChargerOfTheConfiguredEndpoint(t *testing.T) {
	cfg := syntheticConfig(fiveOperations...)
	_, session, _ := run(t, cfg, nil)
	reg := &session.native
	if len(reg.nodes) != 1 || string(reg.nodes[0].Local) != cfg.Endpoint || reg.nodes[0].Kind != "resource.charging_station" {
		t.Fatalf("nodes %+v", reg.nodes)
	}
	for _, m := range reg.members {
		if m.Mode != "read" || m.Scope != "diagnostic" {
			t.Fatalf("member %+v", m)
		}
	}
	raw, _ := json.Marshal(reg.nodes)
	more, _ := json.Marshal(reg.members)
	if strings.Contains(string(raw)+string(more), syntheticSerialPath) {
		t.Fatal("the serial path is in the namespace")
	}
}

func setRequestPayload(t *testing.T, request tesla.ProvisionalCurrentLimitRequest) []byte {
	t.Helper()
	body, err := tesla.EncodeTeslaProvisionalCurrentLimitBody(request)
	if err != nil {
		t.Fatal(err)
	}
	built, err := tesla.BuildTeslaFC100OperationRequest(tesla.HSCFC100OperationCompatibilityV1, tesla.FC100OperationWCSetProvisional, body)
	if err != nil {
		t.Fatal(err)
	}
	return built.Payload()
}

func persistentRequestPayload(t *testing.T, amps int32) []byte {
	t.Helper()
	built, err := tesla.BuildTeslaFC100OperationRequest(tesla.HSCFC100OperationCompatibilityV1, tesla.FC100OperationWCConfigureSettings, tesla.EncodeTeslaPersistentSettingsBody(amps))
	if err != nil {
		t.Fatal(err)
	}
	return built.Payload()
}

func contains(requests [][]byte, payload []byte) bool {
	for _, r := range requests {
		if bytes.Equal(r, payload) {
			return true
		}
	}
	return false
}

func stopAndCount(t *testing.T, reader *ChargerReader, c *charger) (before int, sent [][]byte) {
	t.Helper()
	if st := reader.Stop(context.Background()); !st.Confirmed {
		t.Fatalf("stop %+v", st)
	}
	c.endpoint.mu.Lock()
	closed := c.endpoint.closed
	c.endpoint.mu.Unlock()
	if !closed {
		t.Fatal("the transport is not closed")
	}
	for _, s := range c.endpoint.sent() {
		sent = append(sent, s.Payload())
	}
	return len(sent), sent
}

// wantServed checks every slot of the namespace against the expected payload;
// a slot without an expectation must be absent with its reason.
func wantServed(t *testing.T, session *fakeSession, want map[Slot][]byte) {
	t.Helper()
	for _, spec := range slotSpecs {
		raw, err := session.native.read(driver.MemberID(spec.slot))
		if err != nil {
			t.Fatal(err)
		}
		member, keys := decodeMember(t, raw)
		expected, held := want[spec.slot]
		if !held {
			if member.Present || member.Reason != ReasonNeverReceived || len(keys) != 4 {
				t.Fatalf("%s should be absent with its reason: %s", spec.slot, raw)
			}
			continue
		}
		if !member.Present || member.Reason != "" || member.Payload == nil || member.Receipt == nil {
			t.Fatalf("%s: %s", spec.slot, raw)
		}
		if !bytes.Equal(*member.Payload, expected) {
			t.Fatalf("%s: payload %x, want %x", spec.slot, *member.Payload, expected)
		}
		if member.Operation != string(spec.operation) || member.Slot != string(spec.slot) {
			t.Fatalf("%s: %+v", spec.slot, member)
		}
		if member.Receipt.RequestID == 0 || member.Receipt.Generation == 0 || member.Receipt.ClockEpoch != "scripted" || member.Receipt.ReceivedAt.IsZero() {
			t.Fatalf("%s: receipt %+v", spec.slot, member.Receipt)
		}
		// Only what the specification states is served: the payload bytes, never a decoded member.
		var got []string
		for k := range keys {
			got = append(got, k)
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, []string{"operation", "payload", "present", "receipt", "slot"}) {
			t.Fatalf("%s: members %v", spec.slot, got)
		}
	}
}

// The driver namespace serves the last payload of each of the five
// observations and the last retained request, acknowledgement and readback of
// the provisional limit with their receipts, read with the transport closed;
// the read sends no frame. The persistent-settings slots, which nothing has
// filled, are absent with their reason.
func TestNamespaceServesObservationsAndTheProvisionalExchangeWithTheTransportClosed(t *testing.T) {
	cfg := syntheticConfig(fiveOperations...)
	cfg.ProvisionalReadbackQualified, cfg.ProvisionalSetQualified = true, true
	cfg.Windows[tesla.FC100OperationWCGetProvisional] = tesla.Gen3ReadWindow{Freshness: time.Hour, Retention: 2 * time.Hour}
	reader, session, c := run(t, cfg, func(e *scriptedEndpoint) { pushPolls(e, cfg.Operations, scalars{}) })
	waitFor(t, "the five observations", func() bool {
		p, _ := reader.Payload(SlotLoadSharingState)
		return p.Present
	})
	// The mutation's readback is the last t28 by receipt; the prerequisite read is an earlier one.
	prerequisite := scalars{limit: u(6), timeout: u(900), inhibit: b(false), configured: u(32)}
	returned := scalars{limit: u(12), timeout: u(900), inhibit: b(false), configured: u(32)}
	request := tesla.ProvisionalCurrentLimitRequest{LimitCurrentMaxAmps: 16, LimitTimeoutSeconds: 900}
	pushMutation(c, prerequisite, returned, 1000, 1010)
	result := settle(t, reader, request)
	if err := reader.IngestSetResult(context.Background(), result); err != nil {
		t.Fatal(err)
	}

	before, sent := stopAndCount(t, reader, c)
	want := map[Slot][]byte{
		SlotProvisionalRequest:         setRequestPayload(t, request),
		SlotProvisionalAcknowledgement: terminal(6, 26, nil),
		SlotProvisionalReadback:        terminal(6, 28, readbackBody(returned)),
	}
	for _, op := range fiveOperations {
		want[observationSlots[op]] = pollTerminal(op, scalars{})
	}
	wantServed(t, session, want)
	// The mutation's receipts are the scripted set acknowledgement and readback.
	for slot, wall := range map[Slot]int64{SlotProvisionalRequest: 1000, SlotProvisionalAcknowledgement: 1000, SlotProvisionalReadback: 1010} {
		if p, _ := reader.Payload(slot); !p.Receipt.Wall.Equal(time.Unix(wall, 0)) {
			t.Fatalf("%s receipt %v, want %d", slot, p.Receipt.Wall, wall)
		}
	}
	if !contains(sent, want[SlotProvisionalRequest]) {
		t.Fatal("the served request is not a frame that was sent")
	}
	if after := len(c.endpoint.sent()); after != before {
		t.Fatalf("reading the namespace sent %d frames", after-before)
	}
}

// The persistent-settings request and readback are served the same way, with
// the readback of the polled limit read beside them.
func TestNamespaceServesThePersistentSettingsExchangeWithTheTransportClosed(t *testing.T) {
	cfg := mutationConfig()
	cfg.Version, cfg.PersistentSettingsQualified = "26.34.0", true
	reader, session, c, _ := runWith(t, cfg, syntheticPublication(), nil)
	waitFor(t, "the first poll", func() bool {
		p, _ := reader.Payload(SlotCommonSystemInfo)
		return p.Present
	})
	polled := scalars{limit: u(12), timeout: u(600), inhibit: b(false), configured: u(32)}
	c.endpoint.push(step{echo: true, terminal: terminal(6, 28, readbackBody(polled))})
	waitFor(t, "the polled readback", func() bool {
		record, err := reader.Acquisition().Acquire(context.Background(), tesla.FC100OperationWCGetProvisional)
		return err == nil && record.State == tesla.Gen3ReadEvidenceAck && reader.IngestPolled(context.Background(), record) == nil
	})
	c.endpoint.push(step{terminal: terminal(6, 8, []byte{0x0a, 0x02, 0x08, 0x12})})
	var persistent tesla.PersistentSettingsResult
	waitFor(t, "the persistent exchange", func() bool {
		var err error
		persistent, err = reader.Acquisition().ConfigurePersistentCurrent(context.Background(), tesla.PersistentSettingsRequest{
			MaxOutputCurrentAmps: 20, OperationIdentity: "persistent-a", Deadline: time.Now().Add(5 * time.Second)})
		return err == nil
	})
	if err := reader.IngestPersistent(context.Background(), persistent); err != nil {
		t.Fatal(err)
	}

	before, sent := stopAndCount(t, reader, c)
	want := map[Slot][]byte{
		SlotPersistentRequest:   persistentRequestPayload(t, 20),
		SlotPersistentReadback:  terminal(6, 8, []byte{0x0a, 0x02, 0x08, 0x12}),
		SlotProvisionalReadback: terminal(6, 28, readbackBody(polled)),
		SlotCommonSystemInfo:    pollTerminal(tesla.FC100OperationCommonSystemInfo, scalars{}),
	}
	wantServed(t, session, want)
	if !contains(sent, want[SlotPersistentRequest]) {
		t.Fatal("the served request is not a frame that was sent")
	}
	if after := len(c.endpoint.sent()); after != before {
		t.Fatalf("reading the namespace sent %d frames", after-before)
	}
}

// The slot keeps the last payload by receipt, whatever order records arrive in.
func TestRetainedPayloadIsTheLastByReceipt(t *testing.T) {
	older, newer := rcpt(1, time.Second), rcpt(2, 5*time.Second)
	for name, order := range map[string][]Receipt{"newer first": {newer, older}, "older first": {older, newer}} {
		p := newPayloads()
		for _, r := range order {
			p.retain(SlotVitals, tesla.FC100OperationWCGetVitals, []byte{byte(r.RequestID)}, r)
		}
		got := p.get(slotSpec{SlotVitals, tesla.FC100OperationWCGetVitals})
		if !got.Present || !bytes.Equal(got.Bytes, []byte{2}) {
			t.Fatalf("%s: %+v", name, got)
		}
	}
}
