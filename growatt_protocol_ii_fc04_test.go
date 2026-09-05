package modbusreg

import "testing"

func TestDecodeGrowattProtocolIIFC04TelemetryKeepsRawAndProducesBoundedFacts(t *testing.T) {
	identity, err := DecodeGrowattProtocolIIIdentity(validGrowattProtocolIIIdentityInput())
	if err != nil {
		t.Fatal(err)
	}
	admission, err := NewGrowattProtocolIIFC04Applicability(validGrowattProtocolIIIdentityInput().Profile, "test-exact-mapping")
	if err != nil {
		t.Fatal(err)
	}
	words := make([]uint16, 59)
	words[0] = 1
	words[1], words[2] = 0, 1234
	words[35], words[36] = 0, 567
	words[37], words[38], words[39] = 5000, 2301, 42
	words[42], words[43] = 2302, 43
	words[46], words[47] = 2303, 44
	words[53], words[54] = 0, 123
	words[55], words[56] = 1, 2
	words[57], words[58] = 0, 8
	status, err := DecodeGrowattProtocolIIFC04Telemetry(identity, admission, GrowattProtocolIIFC04Slice{Offset: 0, Words: words})
	if err != nil {
		t.Fatal(err)
	}
	if status.InverterState != GrowattProtocolIIStateNormal || status.PVInputPowerWatts != 123.4 || status.OutputPowerWatts != 56.7 || status.GridFrequencyHz != 50 || status.Phase1VoltageVolts != 230.1 || status.Phase3CurrentAmps != 4.4 || status.TodayGeneratedEnergyKWh != 12.3 || status.TotalGeneratedEnergyKWh != 6553.8 || status.WorkTimeSeconds != 4 {
		t.Fatalf("status=%#v", status)
	}
	raw := status.RawSlice()
	raw.Words[1] = 0
	if status.RawSlice().Words[1] != 0 {
		t.Fatal("raw slice aliases typed observation")
	}
}

func TestDecodeGrowattProtocolIIFC04TelemetryFailsClosed(t *testing.T) {
	identity, err := DecodeGrowattProtocolIIIdentity(validGrowattProtocolIIIdentityInput())
	if err != nil {
		t.Fatal(err)
	}
	admission, err := NewGrowattProtocolIIFC04Applicability(validGrowattProtocolIIIdentityInput().Profile, "test-exact-mapping")
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*GrowattProtocolIIFC04Slice){
		"short":  func(s *GrowattProtocolIIFC04Slice) { s.Words = s.Words[:58] },
		"extra":  func(s *GrowattProtocolIIFC04Slice) { s.Words = append(s.Words, 0) },
		"offset": func(s *GrowattProtocolIIFC04Slice) { s.Offset = 1 },
		"enum":   func(s *GrowattProtocolIIFC04Slice) { s.Words[0] = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			s := GrowattProtocolIIFC04Slice{Words: make([]uint16, 59)}
			mutate(&s)
			if status, err := DecodeGrowattProtocolIIFC04Telemetry(identity, admission, s); err == nil || status.Identity().UnitID() != 0 {
				t.Fatalf("status/err=%#v/%v", status, err)
			}
		})
	}
}

func TestGrowattProtocolIIFC04TelemetryRequiresSeparateExactApplicability(t *testing.T) {
	admission, err := NewGrowattProtocolIIFC04Applicability(validGrowattProtocolIIIdentityInput().Profile, "test-exact-mapping")
	if err != nil {
		t.Fatal(err)
	}
	words := make([]uint16, 59)
	words[0] = 1

	for name, mutate := range map[string]func(*GrowattProtocolIIIdentityInput){
		"unknown exact tuple": func(input *GrowattProtocolIIIdentityInput) {
			input.Profile.DeviceType = 0xbeef
			input.Profile.ModelBuild = [2]uint16{0x1111, 0x2222}
			input.Profile.ProtocolVersion = 0x9999
			input.Slices[2].Words[0] = 0xbeef
			input.Slices[3].Words = []uint16{0x1111, 0x2222}
			input.Slices[4].Words[0] = 0x9999
		},
		"unknown device type": func(input *GrowattProtocolIIIdentityInput) {
			input.Profile.DeviceType = 0xbeef
			input.Slices[2].Words[0] = 0xbeef
		},
		"unknown model build": func(input *GrowattProtocolIIIdentityInput) {
			input.Profile.ModelBuild = [2]uint16{0x1111, 0x2222}
			input.Slices[3].Words = []uint16{0x1111, 0x2222}
		},
		"unknown protocol version": func(input *GrowattProtocolIIIdentityInput) {
			input.Profile.ProtocolVersion = 0x9999
			input.Slices[4].Words[0] = 0x9999
		},
		"profile mismatch": func(input *GrowattProtocolIIIdentityInput) {
			input.Profile.Family = "MID"
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := validGrowattProtocolIIIdentityInput()
			mutate(&input)
			identity, err := DecodeGrowattProtocolIIIdentity(input)
			if err != nil {
				t.Fatalf("raw identity observation should remain available: %v", err)
			}
			if status, err := DecodeGrowattProtocolIIFC04Telemetry(identity, admission, GrowattProtocolIIFC04Slice{Words: words}); err == nil || status.Identity().UnitID() != 0 {
				t.Fatalf("status/err=%#v/%v", status, err)
			}
		})
	}

	identity, err := DecodeGrowattProtocolIIIdentity(validGrowattProtocolIIIdentityInput())
	if err != nil {
		t.Fatal(err)
	}
	if status, err := DecodeGrowattProtocolIIFC04Telemetry(identity, GrowattProtocolIIFC04Applicability{}, GrowattProtocolIIFC04Slice{Words: words}); err == nil || status.Identity().UnitID() != 0 {
		t.Fatalf("missing admission status/err=%#v/%v", status, err)
	}
}
