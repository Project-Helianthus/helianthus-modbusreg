package tesla

import "testing"

func TestTeslaHSCOperationAdmissionNeverConflatesProfileWithWireAuthorization(t *testing.T) {
	qualified, err := NewHSCProfile(HSCProfileConfig{
		Enabled:              true,
		Node:                 0x10,
		PassiveCompatible:    true,
		CompatibilityVersion: HSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	admission := qualified.OperationAdmission()
	if admission.State != TEDAPIAdmissionBlockedNoAdmissibleOperation || admission.OutboundAllowed {
		t.Fatalf("qualified admission = %#v", admission)
	}

	framing, err := NewHSCProfile(HSCProfileConfig{
		Enabled:              true,
		Node:                 0x10,
		PassiveCompatible:    true,
		CompatibilityVersion: "unknown",
	})
	if err != nil {
		t.Fatal(err)
	}
	if admission := framing.OperationAdmission(); admission.State != TEDAPIAdmissionBlockedProfile || admission.OutboundAllowed {
		t.Fatalf("framing admission = %#v", admission)
	}

	disabled, err := NewHSCProfile(HSCProfileConfig{
		Node:                 0x10,
		CompatibilityVersion: HSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if admission := disabled.OperationAdmission(); admission.State != TEDAPIAdmissionBlockedProfile || admission.OutboundAllowed {
		t.Fatalf("disabled admission = %#v", admission)
	}
}
