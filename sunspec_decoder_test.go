package modbusreg

import (
	"reflect"
	"sort"
	"strings"
	"sync"
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
		{120, 26, testSunSpecModelsRevision}, {121, 30, testSunSpecModelsRevision},
		{122, 44, testSunSpecModelsRevision}, {123, 24, testSunSpecModelsRevision},
		{124, 24, testSunSpecModelsRevision},
		{201, 105, testSunSpecModelsRevision}, {202, 105, testSunSpecModelsRevision},
		{203, 105, testSunSpecModelsRevision}, {204, 105, testSunSpecModelsRevision},
		{211, 124, testSunSpecModelsRevision}, {212, 124, testSunSpecModelsRevision},
		{213, 124, testSunSpecModelsRevision}, {214, 124, testSunSpecModelsRevision},
		{305, 36, testSunSpecModelsRevision}, {306, 4, testSunSpecModelsRevision},
		{307, 11, testSunSpecModelsRevision}, {308, 4, testSunSpecModelsRevision},
	}
	sort.Slice(want, func(i, j int) bool {
		if want[i].ModelID != want[j].ModelID {
			return want[i].ModelID < want[j].ModelID
		}
		return want[i].ModelLength < want[j].ModelLength
	})
	got := make(map[SunSpecDecoderKey]bool, len(keys))
	for _, key := range keys {
		got[key] = true
	}
	for _, key := range want {
		if !got[key] {
			t.Fatalf("fixed decoder key absent: %#v", key)
		}
	}
	const dynamicEnvironmentalKeys = 89561
	if len(keys) != len(want)+3277+dynamicEnvironmentalKeys {
		t.Fatalf("decoder key total=%d", len(keys))
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
	occurrence := admittedOccurrence(1, 65, words, 1)
	occurrence.spans = []SunSpecSourceSpan{{LogicalViewID: 17, PDUOffset: 3, WordCount: 65}}
	decoded, err := registry.DecodeOccurrence(occurrence)
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
	spans := decoded.SourceSpans()
	if len(spans) != 1 || spans[0] != occurrence.spans[0] {
		t.Fatalf("source spans=%#v", spans)
	}
	spans[0].LogicalViewID = 99
	if decoded.SourceSpans()[0].LogicalViewID != 17 {
		t.Fatal("source spans were mutable")
	}
}

func TestSunSpecDecoderRejectsMalformedOccurrenceWithoutMutatingEvidence(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	valid := modelWords(t, registry, 103, 50, nil)
	for name, words := range map[string][]uint16{
		"truncated":       append([]uint16(nil), valid[:len(valid)-1]...),
		"extra":           append(append([]uint16(nil), valid...), 0),
		"model mismatch":  append([]uint16{102}, valid[1:]...),
		"length mismatch": append([]uint16{103, 49}, valid[2:]...),
	} {
		t.Run(name, func(t *testing.T) {
			occurrence := admittedOccurrence(103, 50, words, 1)
			before := occurrence.Words()
			if _, err := registry.DecodeOccurrence(occurrence); err == nil {
				t.Fatal("malformed occurrence decoded")
			}
			if !reflect.DeepEqual(occurrence.Words(), before) {
				t.Fatal("raw occurrence evidence changed on rejection")
			}
		})
	}
}

func TestSunSpecDecoderRegistrySupportsConcurrentReadOnlyDecode(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	occurrence := admittedOccurrence(113, 60, modelWords(t, registry, 113, 60, map[string][]uint16{
		"A": {0x4148, 0}, "W": {0x42f6, 0}, "Hz": {0x4248, 0}, "WH": {0x42c8, 0}, "St": {4},
	}), 1)

	const workers = 64
	var wg sync.WaitGroup
	errors := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decoded, err := registry.DecodeOccurrence(occurrence)
			if err != nil {
				errors <- err
				return
			}
			if decoded.Key().ModelID != 113 || len(decoded.Facts()) != 31 {
				errors <- errUnexpectedConcurrentDecode
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
}

type sunSpecTestError string

func (e sunSpecTestError) Error() string { return string(e) }

const errUnexpectedConcurrentDecode sunSpecTestError = "unexpected concurrent SunSpec decode"

func stringWords(value string, words int) []uint16 {
	raw := make([]byte, words*2)
	copy(raw, []byte(value))
	out := make([]uint16, words)
	for index := range out {
		out[index] = uint16(raw[index*2])<<8 | uint16(raw[index*2+1])
	}
	return out
}
