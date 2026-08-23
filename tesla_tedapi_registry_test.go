package modbusreg

import (
	"testing"
)

func TestTeslaTEDAPIRegistryRetainsOnlyRedactedOpaqueObservations(t *testing.T) {
	profile, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled:              true,
		Node:                 0x10,
		PassiveCompatible:    true,
		CompatibilityVersion: TeslaHSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := NewTeslaTEDAPISemanticRegistry()
	if err := registry.Retain(TeslaTEDAPIObservationSpec{
		ID:       "sample-1",
		Profile:  profile,
		Function: teslaHSCFunction100,
		Payload:  []byte{2, 0xaa, 0xbb},
	}); err != nil {
		t.Fatal(err)
	}
	record, ok := registry.Lookup("sample-1")
	if !ok {
		t.Fatal("retained observation missing")
	}
	if record.State != TeslaTEDAPIOpaqueQualified ||
		record.OperationAdmission != TeslaTEDAPIAdmissionBlockedNoAdmissibleOperation ||
		record.PayloadLength != 2 || record.PayloadDigest == "" || record.OutboundAllowed {
		t.Fatalf("record = %#v", record)
	}
}

func TestTeslaTEDAPIRegistryFailsClosedForUnqualifiedOrMalformedInput(t *testing.T) {
	registry := NewTeslaTEDAPISemanticRegistry()
	profile, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled:              true,
		Node:                 0x10,
		PassiveCompatible:    true,
		CompatibilityVersion: "unknown",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Retain(TeslaTEDAPIObservationSpec{
		ID: "sample-2", Profile: profile, Function: teslaHSCFunction101, Payload: []byte{0},
	}); err != nil {
		t.Fatal(err)
	}
	record, _ := registry.Lookup("sample-2")
	if record.State != TeslaTEDAPIFramingOnly ||
		record.OperationAdmission != TeslaTEDAPIAdmissionBlockedProfile ||
		record.OutboundAllowed {
		t.Fatalf("unqualified record = %#v", record)
	}
	if err := registry.Retain(TeslaTEDAPIObservationSpec{
		ID: "bad", Profile: profile, Function: teslaHSCFunction102, Payload: []byte{1},
	}); err == nil {
		t.Fatal("malformed payload accepted")
	}
}
