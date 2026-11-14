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

func TestSunSpecDecoderRegistryKeepsV2RevisionCacheIsolated(t *testing.T) {
	standardSunSpecDecoderKeysCache.Lock()
	priorCache := standardSunSpecDecoderKeysCache.keys
	standardSunSpecDecoderKeysCache.keys = nil
	standardSunSpecDecoderKeysCache.Unlock()
	t.Cleanup(func() {
		standardSunSpecDecoderKeysCache.Lock()
		standardSunSpecDecoderKeysCache.keys = priorCache
		standardSunSpecDecoderKeysCache.Unlock()
	})
	for _, revisions := range [][]SunSpecSchemaRevision{
		{SunSpecModelsRevisionV1, SunSpecModelsRevisionV2},
		{SunSpecModelsRevisionV2, SunSpecModelsRevisionV1},
	} {
		standardSunSpecDecoderKeysCache.Lock()
		standardSunSpecDecoderKeysCache.keys = nil
		standardSunSpecDecoderKeysCache.Unlock()
		registries := make(map[SunSpecSchemaRevision]SunSpecDecoderRegistry, len(revisions))
		for _, revision := range revisions {
			registry, err := NewStandardSunSpecDecoderRegistry(revision)
			if err != nil {
				t.Fatalf("registry %q: %v", revision, err)
			}
			registries[revision] = registry
		}
		v2Keys := registries[SunSpecModelsRevisionV2].DecoderKeys()
		wantV2Static := []SunSpecDecoderKey{
			{ModelID: 1, ModelLength: 66, SchemaRevision: SunSpecModelsRevisionV2},
			{ModelID: 701, ModelLength: 153, SchemaRevision: SunSpecModelsRevisionV2},
			{ModelID: 702, ModelLength: 50, SchemaRevision: SunSpecModelsRevisionV2},
			{ModelID: 703, ModelLength: 17, SchemaRevision: SunSpecModelsRevisionV2},
			{ModelID: 713, ModelLength: 7, SchemaRevision: SunSpecModelsRevisionV2},
			{ModelID: 715, ModelLength: 7, SchemaRevision: SunSpecModelsRevisionV2},
			{ModelID: 802, ModelLength: 62, SchemaRevision: SunSpecModelsRevisionV2},
			{ModelID: 805, ModelLength: 42, SchemaRevision: SunSpecModelsRevisionV2},
			{ModelID: 806, ModelLength: 1, SchemaRevision: SunSpecModelsRevisionV2},
			{ModelID: 807, ModelLength: 34, SchemaRevision: SunSpecModelsRevisionV2},
			{ModelID: 808, ModelLength: 1, SchemaRevision: SunSpecModelsRevisionV2},
			{ModelID: 809, ModelLength: 1, SchemaRevision: SunSpecModelsRevisionV2},
		}
		if len(v2Keys) != len(wantV2Static)+int(maxSunSpecDERPorts)+1+int(maxSunSpecBESSStrings)+1+int(maxSunSpecBESSModules)+1 {
			t.Fatalf("V2 key count=%d", len(v2Keys))
		}
		static := make(map[SunSpecDecoderKey]bool, len(wantV2Static))
		for _, key := range wantV2Static {
			static[key] = true
		}
		dynamicDER, dynamicBESSBanks, dynamicBESSStrings := 0, 0, 0
		for _, key := range v2Keys {
			if key.ModelID == 714 {
				if key.ModelLength < 18 || (key.ModelLength-18)%25 != 0 {
					t.Fatalf("V2 dynamic key=%#v", key)
				}
				dynamicDER++
				continue
			}
			if key.ModelID == 803 {
				if key.ModelLength < 26 || (key.ModelLength-26)%32 != 0 {
					t.Fatalf("V2 dynamic key=%#v", key)
				}
				dynamicBESSBanks++
				continue
			}
			if key.ModelID == 804 {
				if key.ModelLength < 46 || (key.ModelLength-46)%16 != 0 {
					t.Fatalf("V2 dynamic key=%#v", key)
				}
				dynamicBESSStrings++
				continue
			}
			if !static[key] {
				t.Fatalf("unexpected V2 static key=%#v", key)
			}
			delete(static, key)
		}
		if dynamicDER != int(maxSunSpecDERPorts)+1 || dynamicBESSBanks != int(maxSunSpecBESSStrings)+1 || dynamicBESSStrings != int(maxSunSpecBESSModules)+1 || len(static) != 0 {
			t.Fatalf("V2 DER=%d BESS banks=%d strings=%d missing static=%#v", dynamicDER, dynamicBESSBanks, dynamicBESSStrings, static)
		}
		for _, modelID := range []uint16{707, 708, 709} {
			if _, ok := registries[SunSpecModelsRevisionV2].definition(SunSpecDecoderKey{ModelID: modelID, ModelLength: 7, SchemaRevision: SunSpecModelsRevisionV2}); ok {
				t.Fatalf("V2 resolved quarantined Model %d", modelID)
			}
		}
		if _, ok := registries[SunSpecModelsRevisionV2].definition(SunSpecDecoderKey{ModelID: 1, ModelLength: 65, SchemaRevision: SunSpecModelsRevisionV2}); ok {
			t.Fatal("V2 resolved V1 compatibility Common")
		}
		if _, ok := registries[SunSpecModelsRevisionV2].definition(SunSpecDecoderKey{ModelID: 714, ModelLength: 19, SchemaRevision: SunSpecModelsRevisionV2}); ok {
			t.Fatal("V2 resolved a non-geometric DER length")
		}
		if _, ok := registries[SunSpecModelsRevisionV2].definition(SunSpecDecoderKey{ModelID: 803, ModelLength: 27, SchemaRevision: SunSpecModelsRevisionV2}); ok {
			t.Fatal("V2 resolved a non-geometric BESS bank length")
		}
		if _, ok := registries[SunSpecModelsRevisionV2].definition(SunSpecDecoderKey{ModelID: 804, ModelLength: 47, SchemaRevision: SunSpecModelsRevisionV2}); ok {
			t.Fatal("V2 resolved a non-geometric BESS string length")
		}
		for _, key := range registries[SunSpecModelsRevisionV1].DecoderKeys() {
			if key.SchemaRevision != SunSpecModelsRevisionV1 {
				t.Fatalf("V1 cache leaked key %#v", key)
			}
		}
	}
	standardSunSpecDecoderKeysCache.Lock()
	standardSunSpecDecoderKeysCache.keys = nil
	standardSunSpecDecoderKeysCache.Unlock()
	const workers = 32
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			revision := SunSpecModelsRevisionV1
			if worker%2 == 1 {
				revision = SunSpecModelsRevisionV2
			}
			registry, err := NewStandardSunSpecDecoderRegistry(revision)
			if err != nil {
				errors <- err
				return
			}
			for _, key := range registry.DecoderKeys() {
				if key.SchemaRevision != revision {
					errors <- errUnexpectedConcurrentDecode
					return
				}
			}
		}(worker)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
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

func TestSunSpecDecoderRegistryReadAccessorsReportCatalogState(t *testing.T) {
	for _, revision := range []SunSpecSchemaRevision{SunSpecModelsRevisionV1, SunSpecModelsRevisionV2} {
		registry, err := NewStandardSunSpecDecoderRegistry(revision)
		if err != nil {
			t.Fatal(err)
		}
		if registry.SchemaRevision() != revision || registry.SchemaRevision() != registry.revision {
			t.Fatalf("schema revision=%q want %q", registry.SchemaRevision(), revision)
		}
		if registry.DefinitionCount() == 0 || registry.DefinitionCount() != len(registry.definitions) {
			t.Fatalf("%s definition count=%d want %d", revision, registry.DefinitionCount(), len(registry.definitions))
		}
	}
	var zero SunSpecDecoderRegistry
	if zero.SchemaRevision() != "" || zero.DefinitionCount() != 0 {
		t.Fatalf("zero registry revision=%q count=%d", zero.SchemaRevision(), zero.DefinitionCount())
	}
}

func TestSunSpecDecoderRegistryCloneSharesNoCatalogStorage(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	key := SunSpecDecoderKey{ModelID: 113, ModelLength: 60, SchemaRevision: testSunSpecModelsRevision}
	definition := registry.definitions[key]
	scale := int16(-2)
	definition.points = cloneSunSpecPointDefinitions(definition.points)
	definition.points[2].fixedScale = &scale
	registry.definitions[key] = definition

	clone := registry.Clone()
	if clone.revision != registry.revision || !reflect.DeepEqual(clone.keys, registry.keys) || len(clone.definitions) != len(registry.definitions) {
		t.Fatalf("clone revision=%q keys=%d definitions=%d", clone.revision, len(clone.keys), len(clone.definitions))
	}
	for candidate, original := range registry.definitions {
		copied, ok := clone.definitions[candidate]
		if !ok || copied.key != original.key || copied.topology != original.topology || copied.compatibility != original.compatibility || len(copied.points) != len(original.points) || (copied.geometry == nil) != (original.geometry == nil) {
			t.Fatalf("definition %v not copied", candidate)
		}
		for index := range original.points {
			want, got := original.points[index], copied.points[index]
			if (want.fixedScale == nil) != (got.fixedScale == nil) || want.fixedScale != nil && (*want.fixedScale != *got.fixedScale || want.fixedScale == got.fixedScale) {
				t.Fatalf("definition %v point %d fixed scale not deep copied", candidate, index)
			}
			want.fixedScale, got.fixedScale = nil, nil
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("definition %v point %d differs", candidate, index)
			}
		}
	}

	clone.keys[0].ModelLength++
	copied := clone.definitions[key]
	*copied.points[2].fixedScale = 7
	copied.points[0].fieldID = "changed-clone"
	enum := -1
	for index, point := range copied.points {
		if len(point.symbols) > 0 {
			enum = index
			break
		}
	}
	if enum < 0 {
		t.Fatal("model 113 has no enum point")
	}
	for value := range copied.points[enum].symbols {
		copied.points[enum].symbols[value] = "changed-clone"
	}
	delete(clone.definitions, SunSpecDecoderKey{ModelID: 1, ModelLength: 65, SchemaRevision: testSunSpecModelsRevision})
	original := registry.definitions[key]
	if registry.keys[0] == clone.keys[0] || *original.points[2].fixedScale != -2 || original.points[0].fieldID == "changed-clone" || len(registry.definitions) == len(clone.definitions) {
		t.Fatal("clone shares catalog storage with its original")
	}
	for _, symbol := range original.points[enum].symbols {
		if symbol == "changed-clone" {
			t.Fatal("clone shares enum symbols with its original")
		}
	}
	var zero SunSpecDecoderRegistry
	if got := zero.Clone(); got.revision != "" || got.keys != nil || got.definitions == nil || len(got.definitions) != 0 {
		t.Fatalf("zero clone=%#v", got)
	}
}

func TestSunSpecDecodedModelTextFactReadsOnlyValidText(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	model, err := registry.DecodeOccurrence(commonOccurrence(t, registry, "Maker", "Model", "1.0", 1))
	if err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]string{"device.manufacturer": "Maker", "device.model": "Model", "device.version": "1.0"} {
		if got, ok := model.TextFact(field); !ok || got != want {
			t.Fatalf("%s=%q ok=%t want %q", field, got, ok, want)
		}
	}
	if got, ok := model.TextFact("device.absent"); ok || got != "" {
		t.Fatalf("absent field=%q ok=%t", got, ok)
	}
	unset, err := registry.DecodeOccurrence(commonOccurrence(t, registry, "Maker", "Model", "", 1))
	if err != nil {
		t.Fatal(err)
	}
	if fact, ok := unset.Fact("device.version"); !ok || fact.Value.State() == SunSpecValueValid {
		t.Fatalf("unset version fact=%#v ok=%t", fact, ok)
	}
	if got, ok := unset.TextFact("device.version"); ok || got != "" {
		t.Fatalf("non-valid text=%q ok=%t", got, ok)
	}
	inverter, err := registry.DecodeOccurrence(inverterOccurrence(t, registry, 113, nil, 2))
	if err != nil {
		t.Fatal(err)
	}
	if fact, ok := inverter.Fact("inverter.ac.frequency"); !ok || fact.Value.State() != SunSpecValueValid {
		t.Fatalf("numeric fact=%#v ok=%t", fact, ok)
	}
	if got, ok := inverter.TextFact("inverter.ac.frequency"); ok || got != "" {
		t.Fatalf("numeric fact as text=%q ok=%t", got, ok)
	}
	guarded := SunSpecDecodedModel{facts: []SunSpecFact{{FieldID: "device.version", Value: SunSpecValue{pointType: SunSpecTypeString, state: SunSpecValueInvalidEncoding, text: "1.0", hasText: true}}}}
	if got, ok := guarded.TextFact("device.version"); ok || got != "" {
		t.Fatalf("text of a non-valid fact=%q ok=%t", got, ok)
	}
}
