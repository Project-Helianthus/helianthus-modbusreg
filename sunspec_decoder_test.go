package modbusreg

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

const testSunSpecModelsRevision SunSpecSchemaRevision = "sunspec.models@7abdf898-v1"

func admittedOccurrence(id, length uint16, words []uint16, ordinal uint32) SunSpecOccurrence {
	key := SunSpecDecoderKey{ModelID: id, ModelLength: length, SchemaRevision: testSunSpecModelsRevision}
	return SunSpecOccurrence{Ordinal: ordinal, WireKey: SunSpecWireKey{ModelID: id, ModelLength: length}, SchemaRevision: testSunSpecModelsRevision, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: append([]uint16(nil), words...)}
}

func modelWords(t *testing.T, registry SunSpecDecoderRegistry, id, length uint16, values map[string][]uint16) []uint16 {
	t.Helper()
	definition, ok := registry.definition(SunSpecDecoderKey{ModelID: id, ModelLength: length, SchemaRevision: testSunSpecModelsRevision})
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
		if len(value) != int(point.size) {
			t.Fatalf("point %s words=%d want=%d", point.name, len(value), point.size)
		}
		copy(words[point.offset:point.offset+point.size], value)
	}
	return words
}

func TestSunSpecDecoderRegistryUsesExactImmutableKeys(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	keys := registry.DecoderKeys()
	want := []SunSpecDecoderKey{
		{1, 65, testSunSpecModelsRevision}, {1, 66, testSunSpecModelsRevision},
		{101, 50, testSunSpecModelsRevision}, {102, 50, testSunSpecModelsRevision}, {103, 50, testSunSpecModelsRevision},
		{111, 60, testSunSpecModelsRevision}, {112, 60, testSunSpecModelsRevision}, {113, 60, testSunSpecModelsRevision},
	}
	sort.Slice(want, func(i, j int) bool {
		if want[i].ModelID != want[j].ModelID {
			return want[i].ModelID < want[j].ModelID
		}
		return want[i].ModelLength < want[j].ModelLength
	})
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys=%#v", keys)
	}
	keys[0].ModelID = 999
	if registry.DecoderKeys()[0].ModelID == 999 {
		t.Fatal("registry keys are mutable")
	}
	if _, err := NewStandardSunSpecDecoderRegistry("latest"); err == nil {
		t.Fatal("mutable/latest revision admitted")
	}
	wrong := admittedOccurrence(103, 50, make([]uint16, 52), 1)
	wrong.SchemaRevision = "other"
	if _, err := registry.DecodeOccurrence(wrong); err == nil {
		t.Fatal("wrong occurrence revision decoded")
	}
}

func TestSunSpecDecodedModelsAreDefensiveAndReadOnly(t *testing.T) {
	typ := reflect.TypeFor[SunSpecDecoderRegistry]()
	for index := 0; index < typ.NumMethod(); index++ {
		name := strings.ToLower(typ.Method(index).Name)
		if strings.Contains(name, "write") || strings.Contains(name, "set") || strings.Contains(name, "register") {
			t.Fatalf("registry exposes mutation authority: %s", typ.Method(index).Name)
		}
	}
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	words := modelWords(t, registry, 1, 65, map[string][]uint16{"Mn": stringWords("Maker", 16), "Md": stringWords("Model", 16)})
	decoded, err := registry.DecodeOccurrence(admittedOccurrence(1, 65, words, 1))
	if err != nil {
		t.Fatal(err)
	}
	facts := decoded.Facts()
	if len(facts) == 0 {
		t.Fatal("facts absent")
	}
	facts[0].FieldID = "mutated"
	if decoded.Facts()[0].FieldID == "mutated" {
		t.Fatal("facts were mutable")
	}
	raw := decoded.RawWords()
	raw[0] = 0
	if decoded.RawWords()[0] != 1 {
		t.Fatal("raw words were mutable")
	}
}

func stringWords(value string, words int) []uint16 {
	raw := make([]byte, words*2)
	copy(raw, []byte(value))
	out := make([]uint16, words)
	for index := range out {
		out[index] = uint16(raw[index*2])<<8 | uint16(raw[index*2+1])
	}
	return out
}
