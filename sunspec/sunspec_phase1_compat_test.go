package sunspec

import "testing"

import modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"

func TestSunSpecPhaseOneCompatibilityDescriptorStillExact(t *testing.T) {
	profile, err := NewSunSpecPhaseOneProfile(PhaseOneVersions{Profile: modbusreg.CurrentSchemaVersion(), Codec: modbusreg.CurrentCodecContractVersion()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewPhaseOneDecoder(profile); err != nil {
		t.Fatal(err)
	}
}

func TestSunSpecPhaseOneProfileIsQualifiedAndDefaultEnabled(t *testing.T) {
	profile, err := NewSunSpecPhaseOneProfile(PhaseOneVersions{Profile: modbusreg.CurrentSchemaVersion(), Codec: modbusreg.CurrentCodecContractVersion()})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Maturity() != modbusreg.MaturityQualified || !profile.DefaultEnabled() || profile.State() != modbusreg.ProfileActive {
		t.Fatalf("phase-one maturity=%q default_enabled=%t state=%q, want qualified, default-enabled, active", profile.Maturity(), profile.DefaultEnabled(), profile.State())
	}
}
