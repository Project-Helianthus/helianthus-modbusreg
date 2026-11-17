package tesla

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/internal/qualifiedfunctiontest"
)

func TestTeslaFC100WCVitalsOperationIsExplicitlyVersionQualified(t *testing.T) {
	profile, err := NewHSCProfile(HSCProfileConfig{
		Enabled:                  true,
		Node:                     0x10,
		PassiveCompatible:        true,
		CompatibilityVersion:     HSCCompatibilityV1,
		WCVitalsOperationVersion: HSCWCVitalsCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	admission := profile.OperationAdmissionFor(TEDAPIOperationWCVitalsV1)
	if !admission.OutboundAllowed || admission.State != TEDAPIAdmissionAllowedWCVitals {
		t.Fatalf("admission = %#v", admission)
	}
	request, policy, err := profile.EncodeQualifiedFunction(TEDAPIOperationWCVitalsV1)
	if err != nil {
		t.Fatal(err)
	}
	if request.FunctionCode() != modbus.PrivateFunctionCode(100) ||
		!bytes.Equal(request.Payload(), []byte{0x04, 0x32, 0x02, 0x0a, 0x00}) ||
		policy.MaxAttempts() != 1 {
		t.Fatalf("request/policy = %#v/%#v", request, policy)
	}

	withoutOperationVersion, err := NewHSCProfile(HSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: HSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if admission := withoutOperationVersion.OperationAdmissionFor(TEDAPIOperationWCVitalsV1); admission.OutboundAllowed {
		t.Fatalf("unversioned profile admission = %#v", admission)
	}
	if _, _, err := withoutOperationVersion.EncodeQualifiedFunction(TEDAPIOperationWCVitalsV1); err == nil {
		t.Fatal("unversioned profile encoded a vitals request")
	}
	registry, err := modbusreg.NewQualifiedFunctionRegistry([]modbusreg.QualifiedFunctionProfile{{
		Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: HSCProfileName, Codec: profile,
	}})
	if err != nil {
		t.Fatal(err)
	}
	transport := &qualifiedfunctiontest.Exchange{}
	results, err := registry.Dispatch(context.Background(), transport, modbusreg.QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: HSCProfileName, Operation: TEDAPIOperationWCVitalsV1,
	})
	if err != nil || transport.Calls != 1 {
		t.Fatalf("qualified dispatch = %v, calls = %d", err, transport.Calls)
	}
	if len(results) != 1 || len(results[0].Payload) != 0 || results[0].Replay == nil ||
		results[0].Replay.Kind != string(FC100WCVitalsIntermediate) {
		t.Fatalf("echo result = %#v", results)
	}

	blockedRegistry, err := modbusreg.NewQualifiedFunctionRegistry([]modbusreg.QualifiedFunctionProfile{{
		Endpoint: "rtu-a", UnitID: withoutOperationVersion.Node(), VendorProfile: HSCProfileName, Codec: withoutOperationVersion,
	}})
	if err != nil {
		t.Fatal(err)
	}
	blockedTransport := &qualifiedfunctiontest.Exchange{}
	if _, err := blockedRegistry.Dispatch(context.Background(), blockedTransport, modbusreg.QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: withoutOperationVersion.Node(), VendorProfile: HSCProfileName, Operation: TEDAPIOperationWCVitalsV1,
	}); err == nil || blockedTransport.Calls != 0 {
		t.Fatalf("unversioned dispatch = %v, calls = %d", err, blockedTransport.Calls)
	}
}

func TestTeslaFC100WCVitalsDispatchRetainsTerminalOnlyAsRedactedReplay(t *testing.T) {
	profile, err := NewHSCProfile(HSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: HSCCompatibilityV1,
		WCVitalsOperationVersion: HSCWCVitalsCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := modbusreg.NewQualifiedFunctionRegistry([]modbusreg.QualifiedFunctionProfile{{
		Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: HSCProfileName, Codec: profile,
	}})
	if err != nil {
		t.Fatal(err)
	}
	transport := &qualifiedfunctiontest.Exchange{ResponsePayloads: [][]byte{{0x06, 0x32, 0x04, 0x12, 0x02, 0x08, 0x01}}}
	results, err := registry.Dispatch(context.Background(), transport, modbusreg.QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: HSCProfileName, Operation: TEDAPIOperationWCVitalsV1,
	})
	if err != nil || len(results) != 1 || len(results[0].Payload) != 0 || results[0].Replay == nil {
		t.Fatalf("terminal dispatch = %#v, %v", results, err)
	}
	replay := results[0].Replay
	if replay.Kind != string(FC100WCVitalsTerminal) || replay.PayloadLength != 2 ||
		replay.PayloadDigest != "fb8da7eb5b1b399e7321179dac9e9f65773d7331e1e30554e3911e4325e1ef19" {
		t.Fatalf("terminal replay = %#v", replay)
	}
}

func TestTeslaFC100WCVitalsReplayClassifiesEchoAndBoundedTerminal(t *testing.T) {
	echo, err := DecodeTeslaFC100WCVitalsReplay([]byte{0x04, 0x32, 0x02, 0x0a, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	if echo.Kind != FC100WCVitalsIntermediate || echo.SnapshotLength != 0 || echo.SnapshotDigest != "" {
		t.Fatalf("echo = %#v", echo)
	}

	terminal, err := DecodeTeslaFC100WCVitalsReplay([]byte{0x04, 0x32, 0x02, 0x12, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Kind != FC100WCVitalsTerminal || terminal.SnapshotLength != 0 ||
		terminal.SnapshotDigest != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("terminal = %#v", terminal)
	}

	for _, payload := range [][]byte{
		{0x06, 0x32, 0x04, 0x0a, 0x02, 0x12, 0x00},
		{0x07, 0x32, 0x04, 0x12, 0x02, 0x0a, 0x00, 0x00},
		{0x06, 0x32, 0x04, 0x12, 0x02, 0x0a},
	} {
		if _, err := DecodeTeslaFC100WCVitalsReplay(payload); err == nil {
			t.Fatalf("malformed terminal accepted: %x", payload)
		}
	}
}

func TestTeslaFC100WCVitalsReplayRetainsCompleteOpaqueTerminalBody(t *testing.T) {
	for _, body := range [][]byte{{0x08, 0x01}, {0x00, 0x00}} {
		payload := append([]byte{byte(len(body) + 4), 0x32, byte(len(body) + 2), 0x12, byte(len(body))}, body...)
		replay, err := DecodeTeslaFC100WCVitalsReplay(payload)
		if err != nil {
			t.Fatalf("opaque terminal body %x: %v", body, err)
		}
		digest := sha256.Sum256(body)
		if replay.Kind != FC100WCVitalsTerminal || replay.SnapshotLength != len(body) ||
			replay.SnapshotDigest != hex.EncodeToString(digest[:]) {
			t.Fatalf("opaque terminal body %x replay = %#v", body, replay)
		}
	}
}

func TestVitalsAndLifetimeTypedValuesStayUnknown(t *testing.T) {
	vitalsEcho := decodeVitalsReplay(t, []byte{0x04, 0x32, 0x02, 0x0a, 0x00})
	if got := vitalsEcho.TypedValueDisposition(); got != "unknown" || vitalsEcho.Kind != FC100WCVitalsIntermediate || vitalsEcho.SnapshotLength != 0 || vitalsEcho.SnapshotDigest != "" {
		t.Fatalf("vitals intermediate disposition = %q replay = %#v", got, vitalsEcho)
	}
	for _, body := range [][]byte{nil, {0x08, 0x01}, {0x10, 0x02, 0x03}} {
		replay := decodeVitalsReplay(t, wallConnectorTerminalPayload(0x12, body))
		sum := sha256.Sum256(body)
		if got := replay.TypedValueDisposition(); got != "unknown" || replay.Kind != FC100WCVitalsTerminal || replay.SnapshotLength != len(body) || replay.SnapshotDigest != hex.EncodeToString(sum[:]) {
			t.Fatalf("vitals terminal disposition = %q body %x replay = %#v", got, body, replay)
		}
	}

	lifetimeEcho := decodeLifetimeReplay(t, []byte{0x04, 0x32, 0x02, 0x1a, 0x00})
	if got := lifetimeEcho.TypedValueDisposition(); got != "unknown" || lifetimeEcho.Kind != FC100WCLifetimeIntermediate || lifetimeEcho.SnapshotLength != 0 || lifetimeEcho.SnapshotDigest != "" {
		t.Fatalf("lifetime intermediate disposition = %q replay = %#v", got, lifetimeEcho)
	}
	for _, body := range [][]byte{nil, {0x08}, {0x08, 0x01}, {0x11, 0x22, 0x33, 0x44}} {
		replay := decodeLifetimeReplay(t, wallConnectorTerminalPayload(0x22, body))
		sum := sha256.Sum256(body)
		if got := replay.TypedValueDisposition(); got != "unknown" || replay.Kind != FC100WCLifetimeTerminal || replay.SnapshotLength != len(body) || replay.SnapshotDigest != hex.EncodeToString(sum[:]) {
			t.Fatalf("lifetime terminal disposition = %q body %x replay = %#v", got, body, replay)
		}
	}

	if _, err := DecodeTeslaFC100WCVitalsReplay([]byte{0x06, 0x32, 0x04, 0x12, 0x02, 0x0a}); err == nil {
		t.Fatal("malformed vitals body accepted")
	}
	if _, err := DecodeTeslaFC100WCLifetimeReplay([]byte{0x06, 0x32, 0x04, 0x1a, 0x02, 0x08, 0x01}); err == nil {
		t.Fatal("malformed lifetime body accepted")
	}

	lifetimeNames, ok := FC100RecoveredNames(FC100OperationWCGetLifetimeStats)
	if !ok || len(lifetimeNames.Fields) == 0 || !slicesContain(lifetimeNames.Fields, "charging_energy") {
		t.Fatalf("lifetime recovered names = %#v ok = %v", lifetimeNames.Fields, ok)
	}
	vitalsNames, ok := FC100RecoveredNames(FC100OperationWCGetVitals)
	if !ok || len(vitalsNames.Fields) != 0 || slicesContain(vitalsNames.Fields, "charging_energy") {
		t.Fatalf("vitals recovered names = %#v ok = %v", vitalsNames.Fields, ok)
	}
	assertReplayOmitsChargingEnergy(t, FC100WCVitalsReplay{})
	assertReplayOmitsChargingEnergy(t, FC100WCLifetimeReplay{})
	assertWireEntriesHaveNoLabel(t)
}

func decodeVitalsReplay(t *testing.T, payload []byte) FC100WCVitalsReplay {
	t.Helper()
	replay, err := DecodeTeslaFC100WCVitalsReplay(payload)
	if err != nil {
		t.Fatalf("vitals payload %x: %v", payload, err)
	}
	return replay
}

func decodeLifetimeReplay(t *testing.T, payload []byte) FC100WCLifetimeReplay {
	t.Helper()
	replay, err := DecodeTeslaFC100WCLifetimeReplay(payload)
	if err != nil {
		t.Fatalf("lifetime payload %x: %v", payload, err)
	}
	return replay
}

func wallConnectorTerminalPayload(marker byte, body []byte) []byte {
	payload := []byte{byte(len(body) + 4), 0x32, byte(len(body) + 2), marker, byte(len(body))}
	return append(payload, body...)
}

func slicesContain(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertReplayOmitsChargingEnergy(t *testing.T, replay any) {
	t.Helper()
	typ := reflect.TypeOf(replay)
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		folded := strings.ToLower(strings.ReplaceAll(field.Name, "_", ""))
		if strings.Contains(folded, "chargingenergy") || strings.Contains(string(field.Tag), "charging_energy") {
			t.Fatalf("%s field %s carries charging_energy", typ.Name(), field.Name)
		}
	}
}

func assertWireEntriesHaveNoLabel(t *testing.T) {
	t.Helper()
	type key struct {
		field uint64
		kind  uint8
	}
	cases := []struct {
		message []byte
		want    []key
	}{
		{[]byte{0x08, 0x00}, []key{{1, 0}}},
		{[]byte{0x38, 0x01}, []key{{7, 0}}},
		{[]byte{0x78, 0x02}, []key{{15, 0}}},
		{append([]byte{0x09}, make([]byte, 8)...), []key{{1, 1}}},
		{[]byte{0x1d, 0x01, 0x02, 0x03, 0x04}, []key{{3, 5}}},
		{[]byte{0x12, 0x01, 0x7f}, []key{{2, 2}}},
		{[]byte{0x23, 0x28, 0x01, 0x24}, []key{{4, 3}, {5, 0}, {4, 4}}},
	}
	for _, tc := range cases {
		decoded, err := DecodeTeslaFC100TEDAPI(append([]byte{byte(len(tc.message))}, tc.message...))
		if err != nil {
			t.Fatalf("wire message %x: %v", tc.message, err)
		}
		entries := decoded.WireEntries()
		if len(entries) != len(tc.want) {
			t.Fatalf("wire message %x entries = %#v", tc.message, entries)
		}
		for index, entry := range entries {
			if entry.FieldNumber() != tc.want[index].field || entry.WireType() != tc.want[index].kind {
				t.Fatalf("wire entry %d = field %d type %d, want field %d type %d", index, entry.FieldNumber(), entry.WireType(), tc.want[index].field, tc.want[index].kind)
			}
			rejectWireLabel(t, entry)
		}
	}
}

func rejectWireLabel(t *testing.T, entry FC100WireEntry) {
	t.Helper()
	value := reflect.ValueOf(entry)
	typ := value.Type()
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		lower := strings.ToLower(field.Name)
		if strings.Contains(lower, "label") || strings.Contains(lower, "name") {
			t.Fatalf("wire entry field %s is a label", field.Name)
		}
		if field.Type.Kind() == reflect.String && value.Field(index).String() != "" {
			t.Fatalf("wire entry field %s = %q", field.Name, value.Field(index).String())
		}
	}
	for index := 0; index < typ.NumMethod(); index++ {
		name := typ.Method(index).Name
		lower := strings.ToLower(name)
		if strings.Contains(lower, "label") || strings.Contains(lower, "name") {
			t.Fatalf("wire entry method %s is a label", name)
		}
	}
}
