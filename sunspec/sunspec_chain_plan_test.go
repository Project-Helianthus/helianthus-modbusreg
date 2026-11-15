package sunspec

import (
	"reflect"
	"strings"
	"testing"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestSunSpecChainPlannerRequiresExplicitBasesAndBounds(t *testing.T) {
	_, err := NewChainPlan(ChainPlanSpec{})
	if err == nil {
		t.Fatal("empty explicit plan was admitted")
	}
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: SchemaRevision("sunspec.r1@1"),
		BaseCandidates: []uint16{40000},
		Limits:         ChainLimits{MaxTotalWords: 125, MaxOccurrences: 8},
	})
	if err != nil {
		t.Fatalf("NewSunSpecChainPlan: %v", err)
	}
	requests := plan.Requests()
	if len(requests) != 1 || requests[0].Function() != modbusreg.FunctionReadHoldingRegisters || requests[0].WordCount() > 125 {
		t.Fatalf("invalid initial request: %#v", requests)
	}
}

func TestSunSpecChainPlannerRejectsDuplicateBaseCandidates(t *testing.T) {
	_, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: SchemaRevision("sunspec.r1@1"),
		BaseCandidates: []uint16{40000, 40000},
		Limits:         ChainLimits{MaxTotalWords: 125, MaxOccurrences: 8},
	})
	if err == nil {
		t.Fatal("duplicate base candidates were admitted")
	}
}

func TestSunSpecChainPlanStructuralCandidateOptInIsPrivateAndDoesNotChangeInitialReads(t *testing.T) {
	base := ChainPlanSpec{
		SchemaRevision: ModelsRevisionV2,
		BaseCandidates: []uint16{40000},
		Limits:         ChainLimits{MaxTotalWords: 128, MaxOccurrences: 3},
	}
	plain, err := NewChainPlan(base)
	if err != nil {
		t.Fatal(err)
	}
	selectedSpec := base
	selectedSpec.StructuralCandidateIDs = []uint16{707}
	selected, err := NewChainPlan(selectedSpec)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := selected.Requests(), plain.Requests(); len(got) != len(want) || got[0].Address() != want[0].Address() || got[0].WordCount() != want[0].WordCount() || got[0].Purpose() != want[0].Purpose() {
		t.Fatalf("opt-in initial reads=%#v plain=%#v", got, want)
	}
	if _, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision:         base.SchemaRevision,
		BaseCandidates:         base.BaseCandidates,
		Limits:                 base.Limits,
		StructuralCandidateIDs: []uint16{707, 707},
	}); err == nil {
		t.Fatal("duplicate structural candidate IDs were admitted")
	}
}

func TestSunSpecChainPublicRequestsAreReadOnly(t *testing.T) {
	typ := reflect.TypeFor[ReadRequest]()
	for i := 0; i < typ.NumMethod(); i++ {
		if strings.Contains(strings.ToLower(typ.Method(i).Name), "write") || strings.Contains(strings.ToLower(typ.Method(i).Name), "set") {
			t.Fatalf("unexpected control authority: %s", typ.Method(i).Name)
		}
	}
}

func TestSunSpecChainAdmissionIsReplayOnlyAndDoesNotParseSyntheticWireBytes(t *testing.T) {
	typ := reflect.TypeFor[*Chain]()
	if _, ok := typ.MethodByName("Admit"); ok {
		t.Fatal("generic exported Admit surface remains available")
	}
	method, ok := typ.MethodByName("AdmitReplay")
	if !ok {
		t.Fatal("replay-only admission surface is absent")
	}
	if method.Type.NumIn() != 3 || method.Type.In(2) != reflect.TypeFor[modbusreg.LogicalViewSnapshot]() {
		t.Fatalf("unexpected replay admission signature: %v", method.Type)
	}
	c := NewChain(chainPlan(t, []uint16{40000}))
	r := c.NextRequests()[0]
	record := chainView(t, r, 1, []uint16{0x5375, 0x6e53}, "fixture").Record()
	record.WireResponseBytes = []byte{0xde, 0xad, 0xbe, 0xef}
	v, err := modbusreg.NewLogicalViewSnapshot(record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.AdmitReplay(r, v); err != nil {
		t.Fatalf("synthetic replay bytes must remain opaque provenance: %v", err)
	}
}

func TestSunSpecChainPlanRejectsAddressAndAggregateOverflow(t *testing.T) {
	_, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: SchemaRevision("sunspec.r1@1"),
		BaseCandidates: []uint16{65535},
		Limits:         ChainLimits{MaxTotalWords: 126, MaxOccurrences: 1},
	})
	if err == nil {
		t.Fatal("address overflow was admitted")
	}
}

func TestSunSpecV2ChainKeepsCompatibilityAndDERBlocksOpaque(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(ModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: ModelsRevisionV2,
		BaseCandidates: []uint16{40000},
		Limits:         ChainLimits{MaxTotalWords: 512, MaxOccurrences: 4},
		DecoderKeys:    registry.DecoderKeys(),
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	id := uint64(1)
	if _, err := admitNext(t, chain, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	words := append([]uint16{1, 65}, make([]uint16, 65)...)
	words = append(words, 701, 153)
	words = append(words, make([]uint16, 153)...)
	words = append(words, 701, 153)
	words = append(words, make([]uint16, 153)...)
	words = append(words, 702, 49)
	words = append(words, make([]uint16, 49)...)
	for len(words) > 0 {
		request := chain.NextRequests()[0]
		chunk := words[:request.WordCount()]
		words = words[request.WordCount():]
		if _, err := admitNext(t, chain, &id, chunk); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := admitNext(t, chain, &id, []uint16{0xffff, 0})
	if err != nil {
		t.Fatal(err)
	}
	occurrences := snapshot.Occurrences()
	if len(occurrences) != 4 || occurrences[0].Disposition != ChainDispositionUnsupportedLength || occurrences[1].Disposition != ChainDispositionAdmitted || occurrences[2].Disposition != ChainDispositionAdmitted || occurrences[3].Disposition != ChainDispositionUnsupportedLength {
		t.Fatalf("V2 opaque dispositions=%#v", occurrences)
	}
	for _, occurrence := range []Occurrence{occurrences[0], occurrences[3]} {
		if _, ok := occurrence.DecoderKey(); ok {
			t.Fatalf("opaque V2 occurrence has a decoder key: %#v", occurrence)
		}
	}
	for index, occurrence := range occurrences[1:3] {
		if key, ok := occurrence.DecoderKey(); !ok || key.ModelID != 701 || key.ModelLength != 153 || occurrence.Ordinal != uint32(index+2) {
			t.Fatalf("exact V2 DER occurrence is not retained: %#v", occurrence)
		}
	}
}

func TestSunSpecV2ChainKeepsWrongDERPortGeometryOpaque(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(ModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: ModelsRevisionV2,
		BaseCandidates: []uint16{40000},
		Limits:         ChainLimits{MaxTotalWords: 128, MaxOccurrences: 2},
		DecoderKeys:    registry.DecoderKeys(),
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	id := uint64(1)
	if _, err := admitNext(t, chain, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	words := append([]uint16{1, 66}, make([]uint16, 66)...)
	words = append(words, 714, 19)
	words = append(words, make([]uint16, 19)...)
	for len(words) > 0 {
		request := chain.NextRequests()[0]
		chunk := words[:request.WordCount()]
		words = words[request.WordCount():]
		if _, err := admitNext(t, chain, &id, chunk); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := admitNext(t, chain, &id, []uint16{0xffff, 0})
	if err != nil {
		t.Fatal(err)
	}
	occurrences := snapshot.Occurrences()
	if len(occurrences) != 2 || occurrences[1].Disposition != ChainDispositionUnsupportedLength {
		t.Fatalf("wrong V2 714 geometry dispositions=%#v", occurrences)
	}
	if _, ok := occurrences[1].DecoderKey(); ok {
		t.Fatalf("wrong V2 714 geometry has decoder key: %#v", occurrences[1])
	}
}

func TestSunSpecV2ChainRetainsFixedControlObservabilityBlocks(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(ModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: ModelsRevisionV2,
		BaseCandidates: []uint16{40000},
		Limits:         ChainLimits{MaxTotalWords: 128, MaxOccurrences: 3},
		DecoderKeys:    registry.DecoderKeys(),
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	id := uint64(1)
	if _, err := admitNext(t, chain, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	words := append([]uint16{1, 66}, make([]uint16, 66)...)
	words = append(words, 703, 17)
	words = append(words, make([]uint16, 17)...)
	words = append(words, 715, 7)
	words = append(words, make([]uint16, 7)...)
	for len(words) > 0 {
		request := chain.NextRequests()[0]
		chunk := words[:request.WordCount()]
		words = words[request.WordCount():]
		if _, err := admitNext(t, chain, &id, chunk); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := admitNext(t, chain, &id, []uint16{0xffff, 0})
	if err != nil {
		t.Fatal(err)
	}
	occurrences := snapshot.Occurrences()
	if len(occurrences) != 3 || occurrences[1].Disposition != ChainDispositionAdmitted || occurrences[2].Disposition != ChainDispositionAdmitted {
		t.Fatalf("fixed control observability dispositions=%#v", occurrences)
	}
	for index, want := range []WireKey{{ModelID: 703, ModelLength: 17}, {ModelID: 715, ModelLength: 7}} {
		if occurrences[index+1].WireKey != want {
			t.Fatalf("occurrence %d=%#v want=%#v", index+1, occurrences[index+1].WireKey, want)
		}
	}
}

func TestSunSpecV2ChainRetainsFixedBESSBaseBlock(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(ModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: ModelsRevisionV2,
		BaseCandidates: []uint16{40000},
		Limits:         ChainLimits{MaxTotalWords: 256, MaxOccurrences: 2},
		DecoderKeys:    registry.DecoderKeys(),
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	id := uint64(1)
	if _, err := admitNext(t, chain, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	words := append([]uint16{1, 66}, make([]uint16, 66)...)
	words = append(words, 802, 62)
	words = append(words, make([]uint16, 62)...)
	for len(words) > 0 {
		request := chain.NextRequests()[0]
		chunk := words[:request.WordCount()]
		words = words[request.WordCount():]
		if _, err := admitNext(t, chain, &id, chunk); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := admitNext(t, chain, &id, []uint16{0xffff, 0})
	if err != nil {
		t.Fatal(err)
	}
	occurrences := snapshot.Occurrences()
	if len(occurrences) != 2 || occurrences[1].Disposition != ChainDispositionAdmitted || occurrences[1].WireKey != (WireKey{ModelID: 802, ModelLength: 62}) {
		t.Fatalf("BESS base occurrence=%#v", occurrences)
	}
}

func TestSunSpecV2ChainRetainsFixedBESSModuleBlock(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(ModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: ModelsRevisionV2,
		BaseCandidates: []uint16{40000},
		Limits:         ChainLimits{MaxTotalWords: 256, MaxOccurrences: 2},
		DecoderKeys:    registry.DecoderKeys(),
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	id := uint64(1)
	if _, err := admitNext(t, chain, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	words := append([]uint16{1, 66}, make([]uint16, 66)...)
	words = append(words, 805, 42)
	words = append(words, make([]uint16, 42)...)
	for len(words) > 0 {
		request := chain.NextRequests()[0]
		chunk := words[:request.WordCount()]
		words = words[request.WordCount():]
		if _, err := admitNext(t, chain, &id, chunk); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := admitNext(t, chain, &id, []uint16{0xffff, 0})
	if err != nil {
		t.Fatal(err)
	}
	occurrences := snapshot.Occurrences()
	if len(occurrences) != 2 || occurrences[1].Disposition != ChainDispositionAdmitted || occurrences[1].WireKey != (WireKey{ModelID: 805, ModelLength: 42}) {
		t.Fatalf("BESS module occurrence=%#v", occurrences)
	}
}

func TestSunSpecV2ChainRetainsFixedFlowBatteryBlock(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(ModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: ModelsRevisionV2,
		BaseCandidates: []uint16{40000},
		Limits:         ChainLimits{MaxTotalWords: 128, MaxOccurrences: 2},
		DecoderKeys:    registry.DecoderKeys(),
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	id := uint64(1)
	if _, err := admitNext(t, chain, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	words := append([]uint16{1, 66}, make([]uint16, 66)...)
	words = append(words, 806, 1, 7)
	for len(words) > 0 {
		request := chain.NextRequests()[0]
		chunk := words[:request.WordCount()]
		words = words[request.WordCount():]
		if _, err := admitNext(t, chain, &id, chunk); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := admitNext(t, chain, &id, []uint16{0xffff, 0})
	if err != nil {
		t.Fatal(err)
	}
	occurrences := snapshot.Occurrences()
	if len(occurrences) != 2 || occurrences[1].Disposition != ChainDispositionAdmitted || occurrences[1].WireKey != (WireKey{ModelID: 806, ModelLength: 1}) {
		t.Fatalf("flow battery occurrence=%#v", occurrences)
	}
}

func TestSunSpecV2ChainRetainsFixedFlowBatteryStringBlock(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(ModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: ModelsRevisionV2,
		BaseCandidates: []uint16{40000},
		Limits:         ChainLimits{MaxTotalWords: 128, MaxOccurrences: 2},
		DecoderKeys:    registry.DecoderKeys(),
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	id := uint64(1)
	if _, err := admitNext(t, chain, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	words := append([]uint16{1, 66}, make([]uint16, 66)...)
	words = append(words, 807, 34)
	words = append(words, make([]uint16, 34)...)
	for len(words) > 0 {
		request := chain.NextRequests()[0]
		chunk := words[:request.WordCount()]
		words = words[request.WordCount():]
		if _, err := admitNext(t, chain, &id, chunk); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := admitNext(t, chain, &id, []uint16{0xffff, 0})
	if err != nil {
		t.Fatal(err)
	}
	occurrences := snapshot.Occurrences()
	if len(occurrences) != 2 || occurrences[1].Disposition != ChainDispositionAdmitted || occurrences[1].WireKey != (WireKey{ModelID: 807, ModelLength: 34}) {
		t.Fatalf("flow battery string occurrence=%#v", occurrences)
	}
}

func TestSunSpecV2ChainRetainsFixedFlowBatteryModuleBlock(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(ModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: ModelsRevisionV2,
		BaseCandidates: []uint16{40000},
		Limits:         ChainLimits{MaxTotalWords: 128, MaxOccurrences: 2},
		DecoderKeys:    registry.DecoderKeys(),
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	id := uint64(1)
	if _, err := admitNext(t, chain, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	words := append([]uint16{1, 66}, make([]uint16, 66)...)
	words = append(words, 808, 1, 9)
	for len(words) > 0 {
		request := chain.NextRequests()[0]
		chunk := words[:request.WordCount()]
		words = words[request.WordCount():]
		if _, err := admitNext(t, chain, &id, chunk); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := admitNext(t, chain, &id, []uint16{0xffff, 0})
	if err != nil {
		t.Fatal(err)
	}
	occurrences := snapshot.Occurrences()
	if len(occurrences) != 2 || occurrences[1].Disposition != ChainDispositionAdmitted || occurrences[1].WireKey != (WireKey{ModelID: 808, ModelLength: 1}) {
		t.Fatalf("flow battery module occurrence=%#v", occurrences)
	}
}

func TestSunSpecV2ChainRetainsFixedFlowBatteryStackBlock(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(ModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: ModelsRevisionV2,
		BaseCandidates: []uint16{40000},
		Limits:         ChainLimits{MaxTotalWords: 128, MaxOccurrences: 2},
		DecoderKeys:    registry.DecoderKeys(),
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	id := uint64(1)
	if _, err := admitNext(t, chain, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	words := append([]uint16{1, 66}, make([]uint16, 66)...)
	words = append(words, 809, 1, 11)
	for len(words) > 0 {
		request := chain.NextRequests()[0]
		chunk := words[:request.WordCount()]
		words = words[request.WordCount():]
		if _, err := admitNext(t, chain, &id, chunk); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := admitNext(t, chain, &id, []uint16{0xffff, 0})
	if err != nil {
		t.Fatal(err)
	}
	occurrences := snapshot.Occurrences()
	if len(occurrences) != 2 || occurrences[1].Disposition != ChainDispositionAdmitted || occurrences[1].WireKey != (WireKey{ModelID: 809, ModelLength: 1}) {
		t.Fatalf("flow battery stack occurrence=%#v", occurrences)
	}
}

func TestSunSpecV2ChainRetainsDistinctBESSBankOccurrences(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(ModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: ModelsRevisionV2,
		BaseCandidates: []uint16{40000},
		Limits:         ChainLimits{MaxTotalWords: 256, MaxOccurrences: 3},
		DecoderKeys:    registry.DecoderKeys(),
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	id := uint64(1)
	if _, err := admitNext(t, chain, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	words := append([]uint16{1, 66}, make([]uint16, 66)...)
	bankZero := append([]uint16{803, 26}, make([]uint16, 26)...)
	bankTwo := append([]uint16{803, 90}, make([]uint16, 90)...)
	bankTwo[2] = 2
	words = append(words, bankZero...)
	words = append(words, bankTwo...)
	for len(words) > 0 {
		request := chain.NextRequests()[0]
		chunk := words[:request.WordCount()]
		words = words[request.WordCount():]
		if _, err := admitNext(t, chain, &id, chunk); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := admitNext(t, chain, &id, []uint16{0xffff, 0})
	if err != nil {
		t.Fatal(err)
	}
	occurrences := snapshot.Occurrences()
	if len(occurrences) != 3 || occurrences[1].WireKey != (WireKey{ModelID: 803, ModelLength: 26}) || occurrences[2].WireKey != (WireKey{ModelID: 803, ModelLength: 90}) || occurrences[1].Disposition != ChainDispositionAdmitted || occurrences[2].Disposition != ChainDispositionAdmitted || occurrences[1].Ordinal == occurrences[2].Ordinal {
		t.Fatalf("BESS bank occurrences=%#v", occurrences)
	}
}

func TestSunSpecV2ChainKeepsWrongBESSBankGeometryOpaque(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(ModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: ModelsRevisionV2,
		BaseCandidates: []uint16{40000},
		Limits:         ChainLimits{MaxTotalWords: 128, MaxOccurrences: 2},
		DecoderKeys:    registry.DecoderKeys(),
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	id := uint64(1)
	if _, err := admitNext(t, chain, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	words := append([]uint16{1, 66}, make([]uint16, 66)...)
	words = append(words, 803, 27)
	words = append(words, make([]uint16, 27)...)
	for len(words) > 0 {
		request := chain.NextRequests()[0]
		chunk := words[:request.WordCount()]
		words = words[request.WordCount():]
		if _, err := admitNext(t, chain, &id, chunk); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := admitNext(t, chain, &id, []uint16{0xffff, 0})
	if err != nil {
		t.Fatal(err)
	}
	occurrences := snapshot.Occurrences()
	if len(occurrences) != 2 || occurrences[1].Disposition != ChainDispositionUnsupportedLength {
		t.Fatalf("wrong V2 803 geometry occurrences=%#v", occurrences)
	}
	if _, ok := occurrences[1].DecoderKey(); ok {
		t.Fatalf("wrong V2 803 geometry has decoder key: %#v", occurrences[1])
	}
}

func TestSunSpecV2ChainRetainsDistinctBESSStringOccurrences(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(ModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: ModelsRevisionV2,
		BaseCandidates: []uint16{40000},
		Limits:         ChainLimits{MaxTotalWords: 256, MaxOccurrences: 3},
		DecoderKeys:    registry.DecoderKeys(),
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	id := uint64(1)
	if _, err := admitNext(t, chain, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	words := append([]uint16{1, 66}, make([]uint16, 66)...)
	stringZero := append([]uint16{804, 46}, make([]uint16, 46)...)
	stringTwo := append([]uint16{804, 78}, make([]uint16, 78)...)
	stringTwo[3] = 2
	words = append(words, stringZero...)
	words = append(words, stringTwo...)
	for len(words) > 0 {
		request := chain.NextRequests()[0]
		chunk := words[:request.WordCount()]
		words = words[request.WordCount():]
		if _, err := admitNext(t, chain, &id, chunk); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := admitNext(t, chain, &id, []uint16{0xffff, 0})
	if err != nil {
		t.Fatal(err)
	}
	occurrences := snapshot.Occurrences()
	if len(occurrences) != 3 || occurrences[1].WireKey != (WireKey{ModelID: 804, ModelLength: 46}) || occurrences[2].WireKey != (WireKey{ModelID: 804, ModelLength: 78}) || occurrences[1].Disposition != ChainDispositionAdmitted || occurrences[2].Disposition != ChainDispositionAdmitted || occurrences[1].Ordinal == occurrences[2].Ordinal {
		t.Fatalf("BESS string occurrences=%#v", occurrences)
	}
}

func TestSunSpecV2ChainKeepsWrongBESSStringGeometryOpaque(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(ModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: ModelsRevisionV2,
		BaseCandidates: []uint16{40000},
		Limits:         ChainLimits{MaxTotalWords: 128, MaxOccurrences: 2},
		DecoderKeys:    registry.DecoderKeys(),
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	id := uint64(1)
	if _, err := admitNext(t, chain, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	words := append([]uint16{1, 66}, make([]uint16, 66)...)
	words = append(words, 804, 47)
	words = append(words, make([]uint16, 47)...)
	for len(words) > 0 {
		request := chain.NextRequests()[0]
		chunk := words[:request.WordCount()]
		words = words[request.WordCount():]
		if _, err := admitNext(t, chain, &id, chunk); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := admitNext(t, chain, &id, []uint16{0xffff, 0})
	if err != nil {
		t.Fatal(err)
	}
	occurrences := snapshot.Occurrences()
	if len(occurrences) != 2 || occurrences[1].Disposition != ChainDispositionUnsupportedLength {
		t.Fatalf("wrong V2 804 geometry occurrences=%#v", occurrences)
	}
	if _, ok := occurrences[1].DecoderKey(); ok {
		t.Fatalf("wrong V2 804 geometry has decoder key: %#v", occurrences[1])
	}
}
