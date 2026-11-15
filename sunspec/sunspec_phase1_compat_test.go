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
