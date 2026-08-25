package modbusreg

import (
	"bytes"
	"context"
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
	if _, err := registry.Dispatch(context.Background(), transport, QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: TeslaHSCProfileName, Operation: TeslaTEDAPIOperationWCVitalsV1,
	}); err != nil || transport.calls != 1 {
		t.Fatalf("qualified dispatch = %v, calls = %d", err, transport.calls)
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

func TestTeslaFC100WCVitalsReplayClassifiesEchoAndBoundedTerminal(t *testing.T) {
	echo, err := DecodeTeslaFC100WCVitalsReplay([]byte{0x04, 0x32, 0x02, 0x0a, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	if echo.Kind != TeslaFC100WCVitalsIntermediate || echo.SnapshotLength != 0 || echo.SnapshotDigest != "" {
		t.Fatalf("echo = %#v", echo)
	}

	terminal, err := DecodeTeslaFC100WCVitalsReplay([]byte{0x06, 0x32, 0x04, 0x12, 0x02, 0x0a, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Kind != TeslaFC100WCVitalsTerminal || terminal.SnapshotLength != 0 ||
		terminal.SnapshotDigest != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("terminal = %#v", terminal)
	}

	for _, payload := range [][]byte{
		{0x06, 0x32, 0x04, 0x12, 0x02, 0x12, 0x00},
		{0x07, 0x32, 0x04, 0x12, 0x02, 0x0a, 0x00, 0x00},
		{0x06, 0x32, 0x04, 0x12, 0x02, 0x0a},
	} {
		if _, err := DecodeTeslaFC100WCVitalsReplay(payload); err == nil {
			t.Fatalf("malformed terminal accepted: %x", payload)
		}
	}
}
