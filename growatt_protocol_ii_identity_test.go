package modbusreg

import "testing"

func TestGrowattProtocolIIIdentityAcceptsOnlyAnExplicitFC03FixtureProfile(t *testing.T) {
	input := validGrowattProtocolIIIdentityInput()
	observation, err := DecodeGrowattProtocolIIIdentity(input)
	if err != nil {
		t.Fatalf("DecodeGrowattProtocolIIIdentity() error = %v", err)
	}
	if observation.UnitID() != 1 || observation.Profile() != input.Profile || observation.OutboundAllowed() {
		t.Fatalf("observation did not preserve the read-only caller profile: %#v", observation)
	}
	if got := observation.FirmwareText(); got != "FW-1" {
		t.Fatalf("FirmwareText() = %q, want %q", got, "FW-1")
	}
	if got := observation.DeviceType(); got != 0x1234 {
		t.Fatalf("DeviceType() = %#x, want %#x", got, uint16(0x1234))
	}
	if got := observation.ModelBuild(); got != [2]uint16{0x4d41, 0x5831} {
		t.Fatalf("ModelBuild() = %#v", got)
	}
	if got := observation.ProtocolVersion(); got != 0x0124 {
		t.Fatalf("ProtocolVersion() = %#x, want %#x", got, uint16(0x0124))
	}

	slices := observation.Slices()
	slices[0].Words[0] = 0
	if got := observation.Slices()[0].Words[0]; got != 0x4657 {
		t.Fatalf("Slices() aliases observation: %#x", got)
	}
}

func TestGrowattProtocolIIIdentityRejectsOutOfContractInputs(t *testing.T) {
	for name, mutate := range map[string]func(*GrowattProtocolIIIdentityInput){
		"broadcast unit": func(input *GrowattProtocolIIIdentityInput) { input.UnitID = 0 },
		"reserved unit":  func(input *GrowattProtocolIIIdentityInput) { input.UnitID = 255 },
		"other function": func(input *GrowattProtocolIIIdentityInput) {
			input.Function = FunctionReadInputRegisters
		},
		"other schema":   func(input *GrowattProtocolIIIdentityInput) { input.Profile.Schema = "Protocol II v1.23 TL3-X" },
		"unknown family": func(input *GrowattProtocolIIIdentityInput) { input.Profile.Family = "MIX" },
		"missing slice":  func(input *GrowattProtocolIIIdentityInput) { input.Slices = input.Slices[:3] },
		"coalesced slice": func(input *GrowattProtocolIIIdentityInput) {
			input.Slices = []GrowattProtocolIIIdentitySlice{{Offset: 9, Words: make([]uint16, 80)}}
		},
		"serial range": func(input *GrowattProtocolIIIdentityInput) {
			input.Slices[0] = GrowattProtocolIIIdentitySlice{Offset: 23, Words: make([]uint16, 6)}
		},
		"wrong device type": func(input *GrowattProtocolIIIdentityInput) { input.Slices[1].Words[0] = 0xbeef },
		"wrong model build": func(input *GrowattProtocolIIIdentityInput) { input.Slices[2].Words[1] = 0xbeef },
		"wrong protocol":    func(input *GrowattProtocolIIIdentityInput) { input.Slices[3].Words[0] = 0x0123 },
		"interior nul": func(input *GrowattProtocolIIIdentityInput) {
			input.Slices[0].Words[0] = 0x4600
			input.Slices[0].Words[1] = 0x5731
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := validGrowattProtocolIIIdentityInput()
			mutate(&input)
			if _, err := DecodeGrowattProtocolIIIdentity(input); err == nil {
				t.Fatal("accepted an out-of-contract Growatt Protocol II identity")
			}
		})
	}
}

func validGrowattProtocolIIIdentityInput() GrowattProtocolIIIdentityInput {
	return GrowattProtocolIIIdentityInput{
		UnitID:   1,
		Function: FunctionReadHoldingRegisters,
		Profile: GrowattProtocolIIIdentityProfile{
			Schema:          "Protocol II v1.24 TL3-X",
			Family:          "MAX",
			DeviceType:      0x1234,
			ModelBuild:      [2]uint16{0x4d41, 0x5831},
			ProtocolVersion: 0x0124,
		},
		Slices: []GrowattProtocolIIIdentitySlice{
			{Offset: 9, Words: []uint16{0x4657, 0x2d31, 0, 0, 0, 0}},
			{Offset: 43, Words: []uint16{0x1234}},
			{Offset: 82, Words: []uint16{0x4d41, 0x5831}},
			{Offset: 88, Words: []uint16{0x0124}},
		},
	}
}
