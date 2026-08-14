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

func TestSunSpecChainPublicRequestsAreReadOnly(t *testing.T) {
	typ := reflect.TypeFor[SunSpecReadRequest]()
	for i := 0; i < typ.NumMethod(); i++ {
		if strings.Contains(strings.ToLower(typ.Method(i).Name), "write") || strings.Contains(strings.ToLower(typ.Method(i).Name), "set") {
			t.Fatalf("unexpected control authority: %s", typ.Method(i).Name)
		}
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
