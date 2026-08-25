package modbusreg

import (
	"bytes"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestTeslaFC100WCVitalsOperationIsExplicitlyVersionQualified(t *testing.T) {
	profile, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled:                     true,
		Node:                        0x10,
		PassiveCompatible:           true,
		CompatibilityVersion:        TeslaHSCCompatibilityV1,
		WCVitalsOperationVersion:    TeslaHSCWCVitalsCompatibilityV1,
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
