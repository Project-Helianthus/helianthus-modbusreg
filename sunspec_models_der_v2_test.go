package modbusreg

import (
	"os"
	"strings"
	"testing"
)

func TestSunSpecV2DERDefinitionsAndApacheNotice(t *testing.T) {
	t.Run("Apache attribution", func(t *testing.T) {
		contents, err := os.ReadFile("THIRD_PARTY_NOTICES.md")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{
			"SunSpec/models",
			"https://github.com/sunspec/models",
			"90b4a331dcca1d6eac69c1bead952fddcc5852e0",
			"Models 701/153 and 702/50",
			"modified by Helianthus",
			"Apache License",
			"Version 2.0, January 2004",
		} {
			if !strings.Contains(string(contents), want) {
				t.Fatalf("third-party notice lacks %q", want)
			}
		}
	})

	t.Run("exact V2 shapes", func(t *testing.T) {
		registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []struct {
			id, length uint16
			points     int
			first      []string
			last       string
		}{
			{701, 153, 72, []string{"ID", "L", "ACType"}, "MnAlrmInfo"},
			{702, 50, 51, []string{"ID", "L", "WMaxRtg"}, "S_SF"},
		} {
			definition, ok := registry.definition(SunSpecDecoderKey{ModelID: want.id, ModelLength: want.length, SchemaRevision: SunSpecModelsRevisionV2})
			if !ok || len(definition.points) != want.points {
				t.Fatalf("Model %d/%d definition=%v points=%d", want.id, want.length, ok, len(definition.points))
			}
			for index, name := range want.first {
				if definition.points[index].name != name {
					t.Fatalf("Model %d point %d=%q want=%q", want.id, index, definition.points[index].name, name)
				}
			}
			if definition.points[len(definition.points)-1].name != want.last {
				t.Fatalf("Model %d last=%q want=%q", want.id, definition.points[len(definition.points)-1].name, want.last)
			}
		}
	})
}

func TestSunSpecV2DERMeasurePreservesScaledUnsignedCounter(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	words := v2DERModelWords(t, registry, 701, 153, map[string][]uint16{
		"ACType":     {2},
		"TotWhInj":   {0x8000, 0, 0, 1},
		"TotWh_SF":   {0xffff},
		"MnAlrmInfo": {0x0080, 0},
	})
	key := SunSpecDecoderKey{ModelID: 701, ModelLength: 153, SchemaRevision: SunSpecModelsRevisionV2}
	decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{
		Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 701, ModelLength: 153}, SchemaRevision: SunSpecModelsRevisionV2,
		Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words,
	})
	if err != nil {
		t.Fatal(err)
	}
	fact, ok := decoded.Fact("sunspec.der.v2.701.TotWhInj")
	if !ok {
		t.Fatal("scaled unsigned counter absent")
	}
	decimal, ok := fact.Value.UnsignedDecimal()
	if !ok || decimal != (SunSpecUnsignedDecimal{Coefficient: 0x8000000000000001, Exponent: -1}) {
		t.Fatalf("unsigned decimal=%#v present=%v", decimal, ok)
	}
}

func v2DERModelWords(t *testing.T, registry SunSpecDecoderRegistry, id, length uint16, values map[string][]uint16) []uint16 {
	t.Helper()
	key := SunSpecDecoderKey{ModelID: id, ModelLength: length, SchemaRevision: SunSpecModelsRevisionV2}
	definition, ok := registry.definition(key)
	if !ok {
		t.Fatalf("definition %d/%d absent", id, length)
	}
	words := make([]uint16, int(length)+2)
	words[0], words[1] = id, length
	for _, point := range definition.points {
		value, exists := values[point.name]
		if !exists || point.name == "ID" || point.name == "L" {
			continue
		}
		if len(value) > int(point.size) {
			t.Fatalf("point %s words=%d exceeds=%d", point.name, len(value), point.size)
		}
		copy(words[point.offset:point.offset+point.size], value)
	}
	return words
}
