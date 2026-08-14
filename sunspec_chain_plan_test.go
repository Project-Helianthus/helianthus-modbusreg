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
