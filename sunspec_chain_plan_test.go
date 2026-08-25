package modbusreg

import (
	"reflect"
	"strings"
	"testing"
)

func TestSunSpecChainPlannerRequiresExplicitBasesAndBounds(t *testing.T) {
	_, err := NewSunSpecChainPlan(SunSpecChainPlanSpec{})
	if err == nil {
		t.Fatal("empty explicit plan was admitted")
	}
	plan, err := NewSunSpecChainPlan(SunSpecChainPlanSpec{
		SchemaRevision: SunSpecSchemaRevision("sunspec.r1@1"),
		BaseCandidates: []uint16{40000},
		Limits:         SunSpecChainLimits{MaxTotalWords: 125, MaxOccurrences: 8},
	})
	if err != nil {
		t.Fatalf("NewSunSpecChainPlan: %v", err)
	}
	requests := plan.Requests()
	if len(requests) != 1 || requests[0].Function() != FunctionReadHoldingRegisters || requests[0].WordCount() > 125 {
		t.Fatalf("invalid initial request: %#v", requests)
	}
}

func TestSunSpecChainPlannerRejectsDuplicateBaseCandidates(t *testing.T) {
	_, err := NewSunSpecChainPlan(SunSpecChainPlanSpec{
		SchemaRevision: SunSpecSchemaRevision("sunspec.r1@1"),
		BaseCandidates: []uint16{40000, 40000},
		Limits:         SunSpecChainLimits{MaxTotalWords: 125, MaxOccurrences: 8},
	})
	if err == nil {
		t.Fatal("duplicate base candidates were admitted")
	}
}

func TestSunSpecChainPublicRequestsAreReadOnly(t *testing.T) {
	typ := reflect.TypeFor[SunSpecReadRequest]()
	for i := 0; i < typ.NumMethod(); i++ {
		if strings.Contains(strings.ToLower(typ.Method(i).Name), "write") || strings.Contains(strings.ToLower(typ.Method(i).Name), "set") {
			t.Fatalf("unexpected control authority: %s", typ.Method(i).Name)
		}
	}
}

func TestSunSpecChainAdmissionIsReplayOnlyAndDoesNotParseSyntheticWireBytes(t *testing.T) {
	typ := reflect.TypeFor[*SunSpecChain]()
	if _, ok := typ.MethodByName("Admit"); ok {
		t.Fatal("generic exported Admit surface remains available")
	}
	method, ok := typ.MethodByName("AdmitReplay")
	if !ok {
		t.Fatal("replay-only admission surface is absent")
	}
	if method.Type.NumIn() != 3 || method.Type.In(2) != reflect.TypeFor[LogicalViewSnapshot]() {
		t.Fatalf("unexpected replay admission signature: %v", method.Type)
	}
	c := NewSunSpecChain(chainPlan(t, []uint16{40000}))
	r := c.NextRequests()[0]
	v := chainView(t, r, 1, []uint16{0x5375, 0x6e53}, "fixture")
	v.record.WireResponseBytes = []byte{0xde, 0xad, 0xbe, 0xef}
	if _, err := c.AdmitReplay(r, v); err != nil {
		t.Fatalf("synthetic replay bytes must remain opaque provenance: %v", err)
	}
}

func TestSunSpecChainPlanRejectsAddressAndAggregateOverflow(t *testing.T) {
	_, err := NewSunSpecChainPlan(SunSpecChainPlanSpec{
		SchemaRevision: SunSpecSchemaRevision("sunspec.r1@1"),
		BaseCandidates: []uint16{65535},
		Limits:         SunSpecChainLimits{MaxTotalWords: 126, MaxOccurrences: 1},
	})
	if err == nil {
		t.Fatal("address overflow was admitted")
	}
}

func TestSunSpecV2ChainKeepsCompatibilityAndDERBlocksOpaque(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewSunSpecChainPlan(SunSpecChainPlanSpec{
		SchemaRevision: SunSpecModelsRevisionV2,
		BaseCandidates: []uint16{40000},
		Limits:         SunSpecChainLimits{MaxTotalWords: 512, MaxOccurrences: 4},
		DecoderKeys:    registry.DecoderKeys(),
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewSunSpecChain(plan)
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
	if len(occurrences) != 4 || occurrences[0].Disposition != SunSpecChainDispositionUnsupportedLength || occurrences[1].Disposition != SunSpecChainDispositionAdmitted || occurrences[2].Disposition != SunSpecChainDispositionAdmitted || occurrences[3].Disposition != SunSpecChainDispositionUnsupportedLength {
		t.Fatalf("V2 opaque dispositions=%#v", occurrences)
	}
	for _, occurrence := range []SunSpecOccurrence{occurrences[0], occurrences[3]} {
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
