package modbusreg

import "testing"

func TestSunSpecPhaseOneCompatibilityDescriptorStillExact(t *testing.T) {
	profile, err := NewSunSpecPhaseOneProfile(SunSpecPhaseOneVersions{Profile: CurrentSchemaVersion(), Codec: CurrentCodecContractVersion()})
	if err != nil { t.Fatal(err) }
	if _, err := NewSunSpecPhaseOneDecoder(profile); err != nil { t.Fatal(err) }
}
