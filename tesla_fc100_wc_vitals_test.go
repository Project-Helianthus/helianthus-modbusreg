package modbusreg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestTeslaFC100WCVitalsOperationIsExplicitlyVersionQualified(t *testing.T) {
	profile, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled:                  true,
		Node:                     0x10,
		PassiveCompatible:        true,
		CompatibilityVersion:     TeslaHSCCompatibilityV1,
		WCVitalsOperationVersion: TeslaHSCWCVitalsCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	admission := profile.OperationAdmissionFor(TeslaTEDAPIOperationWCVitalsV1)
	if !admission.OutboundAllowed || admission.State != TeslaTEDAPIAdmissionAllowedWCVitals {
		t.Fatalf("admission = %#v", admission)
	}
	request, policy, err := profile.EncodeQualifiedFunction(TeslaTEDAPIOperationWCVitalsV1)
	if err != nil {
		t.Fatal(err)
	}
	if request.FunctionCode() != modbus.PrivateFunctionCode(100) ||
		!bytes.Equal(request.Payload(), []byte{0x04, 0x32, 0x02, 0x0a, 0x00}) ||
		policy.MaxAttempts() != 1 {
		t.Fatalf("request/policy = %#v/%#v", request, policy)
	}

	withoutOperationVersion, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if admission := withoutOperationVersion.OperationAdmissionFor(TeslaTEDAPIOperationWCVitalsV1); admission.OutboundAllowed {
		t.Fatalf("unversioned profile admission = %#v", admission)
	}
	if _, _, err := withoutOperationVersion.EncodeQualifiedFunction(TeslaTEDAPIOperationWCVitalsV1); err == nil {
		t.Fatal("unversioned profile encoded a vitals request")
	}
	registry, err := NewQualifiedFunctionRegistry([]QualifiedFunctionProfile{{
		Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: TeslaHSCProfileName, Codec: profile,
	}})
	if err != nil {
		t.Fatal(err)
	}
	transport := &qualifiedFunctionTestTransport{}
	results, err := registry.Dispatch(context.Background(), transport, QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: TeslaHSCProfileName, Operation: TeslaTEDAPIOperationWCVitalsV1,
	})
	if err != nil || transport.calls != 1 {
		t.Fatalf("qualified dispatch = %v, calls = %d", err, transport.calls)
	}
	if len(results) != 1 || len(results[0].Payload) != 0 || results[0].Replay == nil ||
		results[0].Replay.Kind != string(TeslaFC100WCVitalsIntermediate) {
		t.Fatalf("echo result = %#v", results)
	}

	blockedRegistry, err := NewQualifiedFunctionRegistry([]QualifiedFunctionProfile{{
		Endpoint: "rtu-a", UnitID: withoutOperationVersion.Node(), VendorProfile: TeslaHSCProfileName, Codec: withoutOperationVersion,
	}})
	if err != nil {
		t.Fatal(err)
	}
	blockedTransport := &qualifiedFunctionTestTransport{}
	if _, err := blockedRegistry.Dispatch(context.Background(), blockedTransport, QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: withoutOperationVersion.Node(), VendorProfile: TeslaHSCProfileName, Operation: TeslaTEDAPIOperationWCVitalsV1,
	}); err == nil || blockedTransport.calls != 0 {
		t.Fatalf("unversioned dispatch = %v, calls = %d", err, blockedTransport.calls)
	}
}

func TestTeslaFC100WCVitalsDispatchRetainsTerminalOnlyAsRedactedReplay(t *testing.T) {
	profile, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1,
		WCVitalsOperationVersion: TeslaHSCWCVitalsCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewQualifiedFunctionRegistry([]QualifiedFunctionProfile{{
		Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: TeslaHSCProfileName, Codec: profile,
	}})
	if err != nil {
		t.Fatal(err)
	}
	transport := &qualifiedFunctionTestTransport{responsePayloads: [][]byte{{0x06, 0x32, 0x04, 0x12, 0x02, 0x08, 0x01}}}
	results, err := registry.Dispatch(context.Background(), transport, QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: TeslaHSCProfileName, Operation: TeslaTEDAPIOperationWCVitalsV1,
	})
	if err != nil || len(results) != 1 || len(results[0].Payload) != 0 || results[0].Replay == nil {
		t.Fatalf("terminal dispatch = %#v, %v", results, err)
	}
	replay := results[0].Replay
	if replay.Kind != string(TeslaFC100WCVitalsTerminal) || replay.PayloadLength != 2 ||
		replay.PayloadDigest != "fb8da7eb5b1b399e7321179dac9e9f65773d7331e1e30554e3911e4325e1ef19" {
		t.Fatalf("terminal replay = %#v", replay)
	}
}

func TestTeslaFC100WCVitalsReplayClassifiesEchoAndBoundedTerminal(t *testing.T) {
	echo, err := DecodeTeslaFC100WCVitalsReplay([]byte{0x04, 0x32, 0x02, 0x0a, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	if echo.Kind != TeslaFC100WCVitalsIntermediate || echo.SnapshotLength != 0 || echo.SnapshotDigest != "" {
		t.Fatalf("echo = %#v", echo)
	}

	terminal, err := DecodeTeslaFC100WCVitalsReplay([]byte{0x04, 0x32, 0x02, 0x12, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Kind != TeslaFC100WCVitalsTerminal || terminal.SnapshotLength != 0 ||
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
		if replay.Kind != TeslaFC100WCVitalsTerminal || replay.SnapshotLength != len(body) ||
			replay.SnapshotDigest != hex.EncodeToString(digest[:]) {
			t.Fatalf("opaque terminal body %x replay = %#v", body, replay)
		}
	}
}
