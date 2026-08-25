package modbusreg

import (
	"reflect"
	"testing"
)

func TestProjectSunSpecStructuralFactsProjectsOnlyRetained707Candidates(t *testing.T) {
	plain, _ := runV2StructuralCandidateChain(t, nil)
	if got := ProjectSunSpecStructuralFacts(plain); len(got) != 0 {
		t.Fatalf("plain projections=%#v", got)
	}

	snapshot, _ := runV2StructuralCandidateChain(t, []uint16{707})
	projections := ProjectSunSpecStructuralFacts(snapshot)
	if len(projections) != 2 {
		t.Fatalf("projections=%d want=2", len(projections))
	}
	for index, projection := range projections {
		if got := projection.WireKey(); got != (SunSpecWireKey{ModelID: 707, ModelLength: 33}) {
			t.Fatalf("projection %d wire key=%#v", index, got)
		}
		if projection.SchemaRevision() != SunSpecModelsRevisionV2 || projection.Ordinal() != uint32(index+1) {
			t.Fatalf("projection %d identity revision=%q ordinal=%d", index, projection.SchemaRevision(), projection.Ordinal())
		}
		if facts := projection.Facts(); len(facts) != 29 {
			t.Fatalf("projection %d facts=%d want=29", index, len(facts))
		}
	}

	first := projections[0]
	if got := first.RawWords(); len(got) != 35 || got[0] != 707 || got[1] != 33 {
		t.Fatalf("projection raw=%#v", got)
	}
	if got := first.SourceSpans(); len(got) == 0 {
		t.Fatal("projection omitted source spans")
	}

	facts := first.Facts()
	var voltage, elapsed, readOnly SunSpecFact
	for _, fact := range facts {
		path, ok := fact.NestedPath()
		if !ok {
			t.Fatalf("fact %q omitted nested path", fact.FieldID)
		}
		switch {
		case reflect.DeepEqual(path.Segments(), []SunSpecFactPathSegment{{Name: "Crv", Indexed: true, Index: 1}, {Name: "MustTrip"}, {Name: "Pt", Indexed: true, Index: 1}, {Name: "V"}}):
			voltage = fact
		case reflect.DeepEqual(path.Segments(), []SunSpecFactPathSegment{{Name: "Crv", Indexed: true, Index: 1}, {Name: "MustTrip"}, {Name: "Pt", Indexed: true, Index: 1}, {Name: "Tms"}}):
			elapsed = fact
		case reflect.DeepEqual(path.Segments(), []SunSpecFactPathSegment{{Name: "Crv", Indexed: true, Index: 1}, {Name: "ReadOnly"}}):
			readOnly = fact
		}
	}
	if voltage.FieldID != "sunspec.der.v2.707.Crv.MustTrip.Pt.V" || voltage.Unit != "VNomPct" || voltage.Required {
		t.Fatalf("voltage fact=%#v", voltage)
	}
	if decimal, ok := voltage.Value.Decimal(); !ok || decimal.Coefficient != 0 || decimal.Exponent != 0 {
		t.Fatalf("voltage value=%#v", voltage.Value)
	}
	if elapsed.FieldID != "sunspec.der.v2.707.Crv.MustTrip.Pt.Tms" || elapsed.Unit != "Secs" {
		t.Fatalf("elapsed fact=%#v", elapsed)
	}
	if decimal, ok := elapsed.Value.Decimal(); !ok || decimal.Coefficient != 1 || decimal.Exponent != 0 {
		t.Fatalf("elapsed value=%#v", elapsed.Value)
	}
	if number, symbol, ok := readOnly.Value.Enum(); !ok || number != 0 || symbol != "RW" {
		t.Fatalf("read-only enum=%#v", readOnly.Value)
	}
	if source, ok := voltage.OccurrenceSourceRange(); !ok || source.OccurrenceOffset() != 11 || source.WordCount() != 1 || len(source.SourceSpans()) == 0 {
		t.Fatalf("voltage provenance=%#v ok=%v", source, ok)
	}

	facts[0].path.segments[0].Name = "mutated"
	if got, _ := first.Facts()[0].NestedPath(); got.Segments()[0].Name == "mutated" {
		t.Fatal("projection facts alias retained layout")
	}
	raw := first.RawWords()
	raw[0] = 1
	if first.RawWords()[0] != 707 {
		t.Fatal("projection raw words alias retained occurrence")
	}

	api := reflect.TypeFor[SunSpecStructuralProjection]()
	for _, forbidden := range []string{"DecoderKey", "Qualifies", "Topology", "Decode"} {
		if _, exists := api.MethodByName(forbidden); exists {
			t.Fatalf("projection exposes admission/decoder surface %s", forbidden)
		}
	}
}

func TestProjectSunSpecStructuralFactsKeepsInvalidScaleRaw(t *testing.T) {
	snapshot, _ := runV2StructuralCandidateChain(t, []uint16{707})
	snapshot.occurrences[0].words[7] = 0x8000 // V_SF remains structurally present but unavailable.
	projections := ProjectSunSpecStructuralFacts(snapshot)
	if len(projections) != 2 {
		t.Fatalf("projections=%d want=2", len(projections))
	}
	for _, fact := range projections[0].Facts() {
		path, ok := fact.NestedPath()
		if !ok || !reflect.DeepEqual(path.Segments(), []SunSpecFactPathSegment{{Name: "Crv", Indexed: true, Index: 1}, {Name: "MustTrip"}, {Name: "Pt", Indexed: true, Index: 1}, {Name: "V"}}) {
			continue
		}
		if fact.Value.State() != SunSpecValueNotImplemented || len(fact.Value.RawWords()) != 1 || fact.Value.RawWords()[0] != 0 {
			t.Fatalf("scaled voltage did not retain raw unavailable state: %#v", fact.Value)
		}
		if _, ok := fact.Value.Decimal(); ok {
			t.Fatal("scaled voltage retained a decimal under unavailable scale")
		}
		return
	}
	t.Fatal("projected voltage fact absent")
}
