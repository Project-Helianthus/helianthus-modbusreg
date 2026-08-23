package modbusreg

import "testing"

func TestTeslaHSCOperationAdmissionNeverConflatesProfileWithWireAuthorization(t *testing.T) {
	qualified, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled:              true,
		Node:                 0x10,
		PassiveCompatible:    true,
		CompatibilityVersion: TeslaHSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	admission := qualified.OperationAdmission()
	if admission.State != TeslaTEDAPIAdmissionBlockedNoAdmissibleOperation || admission.OutboundAllowed {
		t.Fatalf("qualified admission = %#v", admission)
	}

	framing, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled:              true,
		Node:                 0x10,
		PassiveCompatible:    true,
		CompatibilityVersion: "unknown",
	})
	if err != nil {
		t.Fatal(err)
	}
	if admission := framing.OperationAdmission(); admission.State != TeslaTEDAPIAdmissionBlockedProfile || admission.OutboundAllowed {
		t.Fatalf("framing admission = %#v", admission)
	}

	disabled, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Node:                 0x10,
		CompatibilityVersion: TeslaHSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if admission := disabled.OperationAdmission(); admission.State != TeslaTEDAPIAdmissionBlockedProfile || admission.OutboundAllowed {
		t.Fatalf("disabled admission = %#v", admission)
	}
}
