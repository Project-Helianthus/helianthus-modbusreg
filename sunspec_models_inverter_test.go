package modbusreg

import "testing"

func TestSunSpecIntegerAndFloatInvertersDecodeAllFields(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	integerValues := map[string][]uint16{
		"A": {123}, "A_SF": {0xfffe}, "W": {0xff85}, "W_SF": {0}, "Hz": {5000}, "Hz_SF": {0xfffe},
		"WH": {0, 42}, "WH_SF": {0}, "St": {4}, "StVnd": {99}, "Evt1": {0, 3},
	}
	for _, id := range []uint16{101, 102, 103} {
		decoded, err := registry.DecodeOccurrence(admittedOccurrence(id, 50, modelWords(t, registry, id, 50, integerValues), 1))
		if err != nil {
			t.Fatalf("model %d: %v", id, err)
		}
		if len(decoded.Facts()) != 43 {
			t.Fatalf("model %d facts=%d", id, len(decoded.Facts()))
		}
		power, _ := decoded.Fact("inverter.ac.power.active")
		decimal, ok := power.Value.Decimal()
		if !ok || decimal.Coefficient != -123 || decimal.Exponent != 0 {
			t.Fatalf("model %d power=%#v", id, power)
		}
		state, _ := decoded.Fact("inverter.operating_state")
		if number, symbol, ok := state.Value.Enum(); !ok || number != 4 || symbol != "MPPT" {
			t.Fatalf("state=%d/%q/%v", number, symbol, ok)
		}
	}
	floatValues := map[string][]uint16{"A": {0x4148, 0}, "W": {0xc2f6, 0}, "Hz": {0x4248, 0}, "St": {4}, "Evt1": {0, 3}}
	for _, id := range []uint16{111, 112, 113} {
		decoded, err := registry.DecodeOccurrence(admittedOccurrence(id, 60, modelWords(t, registry, id, 60, floatValues), 1))
		if err != nil {
			t.Fatalf("model %d: %v", id, err)
		}
		if len(decoded.Facts()) != 31 {
			t.Fatalf("model %d facts=%d", id, len(decoded.Facts()))
		}
		power, _ := decoded.Fact("inverter.ac.power.active")
		if value, ok := power.Value.Float32(); !ok || value != -123 {
			t.Fatalf("model %d power=%v/%v", id, value, ok)
		}
	}
}

func TestSunSpecModelDecodeFailsClosedOnInvalidRequiredEncoding(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		id, length uint16
		values     map[string][]uint16
	}{
		"invalid scale":            {103, 50, map[string][]uint16{"A": {1}, "A_SF": {11}}},
		"canonical float sentinel": {113, 60, map[string][]uint16{"A": {0x7fc0, 0}}},
		"infinite float":           {113, 60, map[string][]uint16{"A": {0x7f80, 0}}},
	} {
		t.Run(name, func(t *testing.T) {
			decoded, err := registry.DecodeOccurrence(admittedOccurrence(tc.id, tc.length, modelWords(t, registry, tc.id, tc.length, tc.values), 1))
			if err != nil {
				t.Fatal(err)
			}
			fact, ok := decoded.Fact("inverter.ac.current.total")
			if !ok || fact.Value.State() == SunSpecValueValid {
				t.Fatalf("fact=%#v", fact)
			}
			if decoded.Qualifies() {
				t.Fatal("invalid required fact qualified")
			}
		})
	}
}
