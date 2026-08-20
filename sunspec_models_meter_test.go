package modbusreg

import "testing"

func TestSunSpecMeterModelsMatchPinnedCatalogShapes(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id, length, points uint16
	}{
		{201, 105, 74}, {202, 105, 74}, {203, 105, 74}, {204, 105, 74},
		{211, 124, 64}, {212, 124, 64}, {213, 124, 64}, {214, 124, 64},
	} {
		definition, ok := registry.definition(SunSpecDecoderKey{tc.id, tc.length, testSunSpecModelsRevision})
		if !ok {
			t.Fatalf("model %d/%d absent", tc.id, tc.length)
		}
		if len(definition.points) != int(tc.points) {
			t.Fatalf("model %d/%d points=%d want=%d", tc.id, tc.length, len(definition.points), tc.points)
		}
		var extent uint16
		for _, point := range definition.points {
			if point.offset != extent || point.size == 0 {
				t.Fatalf("model %d point %#v is not contiguous", tc.id, point)
			}
			extent += point.size
		}
		if extent != tc.length+2 {
			t.Fatalf("model %d extent=%d", tc.id, extent)
		}
	}
}

func TestSunSpecMeterIntegerAndFloatEncodingsDecodeEquivalently(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	integer := modelWords(t, registry, 203, 105, map[string][]uint16{
		"A": {1234}, "A_SF": {0xffff}, "Hz": {5000}, "Hz_SF": {0xfffe},
		"W": {0xff85}, "W_SF": {0}, "TotWhExp": {0, 42}, "TotWh_SF": {0}, "Evt": {0, 1 << 2},
	})
	decodedInteger, err := registry.DecodeOccurrence(admittedOccurrence(203, 105, integer, 1))
	if err != nil {
		t.Fatal(err)
	}
	current, ok := decodedInteger.Fact("meter.ac.current.total")
	if !ok {
		t.Fatal("integer meter current fact absent")
	}
	decimal, ok := current.Value.Decimal()
	if !ok || decimal != (SunSpecDecimal{Coefficient: 1234, Exponent: -1}) {
		t.Fatalf("integer current=%#v ok=%v", decimal, ok)
	}
	energy, ok := decodedInteger.Fact("meter.energy.export.total")
	if !ok || energy.Value.State() != SunSpecValueValid {
		t.Fatalf("integer export energy=%#v", energy)
	}

	floating := modelWords(t, registry, 213, 124, map[string][]uint16{
		"A": {0x42f6, 0x8000}, "Hz": {0x4248, 0}, "W": {0xc2f6, 0},
		"TotWhExp": {0x4228, 0}, "TotWhImp": {0x4120, 0}, "Evt": {0, 1 << 2},
	})
	decodedFloat, err := registry.DecodeOccurrence(admittedOccurrence(213, 124, floating, 1))
	if err != nil {
		t.Fatal(err)
	}
	current, ok = decodedFloat.Fact("meter.ac.current.total")
	value, hasFloat := current.Value.Float32()
	if !ok || !hasFloat || value != 123.25 {
		t.Fatalf("float current=%v ok=%v fact=%#v", value, hasFloat, current)
	}
	if !decodedInteger.Qualifies() || !decodedFloat.Qualifies() {
		t.Fatalf("qualifies integer=%v float=%v", decodedInteger.Qualifies(), decodedFloat.Qualifies())
	}
}

func TestSunSpecMeterMandatoryAndEventFieldsFailClosed(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	words := modelWords(t, registry, 203, 105, map[string][]uint16{
		"A": {0x8000}, "A_SF": {0}, "Evt": {0, 1 << 1},
	})
	decoded, err := registry.DecodeOccurrence(admittedOccurrence(203, 105, words, 1))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Qualifies() {
		t.Fatal("meter with missing mandatory current qualified")
	}
	events, ok := decoded.Fact("meter.events")
	if !ok || events.Value.State() != SunSpecValueValid {
		t.Fatalf("events=%#v", events)
	}
	bits, unknown, ok := events.Value.Bitfield()
	if !ok || bits != 2 || unknown != 2 {
		t.Fatalf("events bits=%x unknown=%x ok=%v", bits, unknown, ok)
	}
}
