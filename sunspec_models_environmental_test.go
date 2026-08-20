package modbusreg

import "testing"

func TestSunSpecEnvironmentalFixedAndRepeatedModelShapes(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id, length, points uint16
	}{
		{302, 10, 12}, {303, 2, 4}, {304, 12, 8},
		{305, 36, 8}, {306, 4, 6}, {307, 11, 13}, {308, 4, 6},
	} {
		definition, ok := registry.definition(SunSpecDecoderKey{tc.id, tc.length, testSunSpecModelsRevision})
		if !ok || len(definition.points) != int(tc.points) {
			t.Fatalf("model %d/%d points=%d ok=%v", tc.id, tc.length, len(definition.points), ok)
		}
	}
	for _, key := range []SunSpecDecoderKey{
		{302, 6, testSunSpecModelsRevision},
		{303, 0, testSunSpecModelsRevision},
		{304, 5, testSunSpecModelsRevision},
		{305, 35, testSunSpecModelsRevision},
		{303, 65535, testSunSpecModelsRevision},
	} {
		if _, ok := registry.definition(key); ok {
			t.Fatalf("invalid environmental geometry admitted: %#v", key)
		}
	}
	if _, ok := registry.definition(SunSpecDecoderKey{303, maxAddressableSunSpecModelLength, testSunSpecModelsRevision}); !ok {
		t.Fatal("largest addressable repeated geometry absent")
	}
}

func TestSunSpecEnvironmentalRepeatsPreserveGroupIdentity(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	words := modelWords(t, registry, 302, 10, map[string][]uint16{})
	definition, _ := registry.definition(SunSpecDecoderKey{302, 10, testSunSpecModelsRevision})
	for _, point := range definition.points {
		if point.repeated {
			words[point.offset] = 100*point.repeatIndex + point.offset
		}
	}
	decoded, err := registry.DecodeOccurrence(admittedOccurrence(302, 10, words, 4))
	if err != nil {
		t.Fatal(err)
	}
	var global []SunSpecFact
	for _, fact := range decoded.Facts() {
		if fact.FieldID == "environment.irradiance.global_horizontal" {
			global = append(global, fact)
		}
	}
	if len(global) != 2 {
		t.Fatalf("global irradiance facts=%d", len(global))
	}
	for index, fact := range global {
		if !fact.Repeated || fact.GroupID != "repeating" || fact.RepeatIndex != uint16(index+1) {
			t.Fatalf("fact[%d]=%#v", index, fact)
		}
	}
}

func TestSunSpecEnvironmentalInt32AndStringPacking(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	words := modelWords(t, registry, 305, 36, map[string][]uint16{
		"Tm": stringWords("123456.789Z", 6), "Date": stringWords("20260820", 4),
		"Loc": stringWords("Roof A", 20), "Lat": {0x02c2, 0x5c00}, "Long": {0xf0ad, 0x5b80}, "Alt": {0, 250},
	})
	decoded, err := registry.DecodeOccurrence(admittedOccurrence(305, 36, words, 1))
	if err != nil {
		t.Fatal(err)
	}
	latitude, ok := decoded.Fact("environment.location.latitude")
	decimal, hasDecimal := latitude.Value.Decimal()
	if !ok || !hasDecimal || decimal != (SunSpecDecimal{Coefficient: 46291968, Exponent: -7}) {
		t.Fatalf("latitude=%#v ok=%v fact=%#v", decimal, hasDecimal, latitude)
	}
	longitude, ok := decoded.Fact("environment.location.longitude")
	signed, hasSigned := longitude.Value.Signed()
	if !ok || !hasSigned || signed != -257074304 {
		t.Fatalf("longitude=%d ok=%v fact=%#v", signed, hasSigned, longitude)
	}
	location, ok := decoded.Fact("environment.location.description")
	text, hasText := location.Value.Text()
	if !ok || !hasText || text != "Roof A" {
		t.Fatalf("location=%q ok=%v fact=%#v", text, hasText, location)
	}
}

func TestSunSpecEnvironmentalDynamicKeysParticipateInChainAdmission(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	keys := registry.DecoderKeys()
	want := map[SunSpecWireKey]bool{{302, 5}: false, {303, 3}: false, {304, 12}: false}
	for _, key := range keys {
		wire := SunSpecWireKey{key.ModelID, key.ModelLength}
		if _, ok := want[wire]; ok {
			want[wire] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Fatalf("dynamic decoder key absent: %#v", key)
		}
	}
}
