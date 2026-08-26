package modbusreg

import "testing"

func TestTeslaFC100WCPPUSettingsReplayIsVersionGatedAndOpaque(t *testing.T) {
	profile, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1,
		WCPPUSettingsReplayVersion: TeslaHSCWCPPUSettingsReplayCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewTeslaFC100WCPPUSettingsReplayDecoder(profile)
	if err != nil {
		t.Fatal(err)
	}
	replays, err := decoder.Decode([][]byte{
		{0x05, 0x32, 0x03, 0xba, 0x01, 0x00},
		{0x06, 0x32, 0x04, 0xc2, 0x01, 0x01, 0x08},
	})
	if err != nil || len(replays) != 2 ||
		replays[0].Kind != TeslaFC100WCPPUSettingsIntermediate ||
		replays[1].Kind != TeslaFC100WCPPUSettingsTerminal ||
		replays[1].SnapshotLength != 1 || replays[1].SnapshotDigest == "" {
		t.Fatalf("replays = %#v, %v", replays, err)
	}
	for _, invalid := range [][][]byte{
		{{0x05, 0x32, 0x03, 0xba, 0x01, 0x00}},
		{{0x06, 0x32, 0x04, 0xc2, 0x01, 0x01, 0x08}, {0x05, 0x32, 0x03, 0xba, 0x01, 0x00}},
		{{0x08, 0x32, 0x06, 0xc2, 0x01, 0x01, 0x08, 0xca, 0x01, 0x00}},
	} {
		if got, err := decoder.Decode(invalid); err == nil || got != nil {
			t.Fatalf("invalid replay = %#v, %v", got, err)
		}
	}

	ungated, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTeslaFC100WCPPUSettingsReplayDecoder(ungated); err == nil {
		t.Fatal("ungated profile created a PPU replay decoder")
	}
	var zero TeslaFC100WCPPUSettingsReplayDecoder
	if got, err := zero.Decode([][]byte{{0x06, 0x32, 0x04, 0xc2, 0x01, 0x01, 0x08}}); err == nil || got != nil {
		t.Fatalf("zero-value decoder = %#v, %v", got, err)
	}
}
