package modbusreg

import (
	"reflect"
	"testing"
)

func TestSunSpecFactPathRetainsOrderedHierarchyWithoutAliasing(t *testing.T) {
	input := []SunSpecFactPathSegment{
		{Name: "Crv", Indexed: true, Index: 2},
		{Name: "MustTrip"},
		{Name: "Pt", Indexed: true, Index: 5},
	}
	path, err := NewSunSpecFactPath(input)
	if err != nil {
		t.Fatal(err)
	}
	input[0].Name = "mutated"
	got := path.Segments()
	if !reflect.DeepEqual(got, []SunSpecFactPathSegment{{Name: "Crv", Indexed: true, Index: 2}, {Name: "MustTrip"}, {Name: "Pt", Indexed: true, Index: 5}}) {
		t.Fatalf("segments=%#v", got)
	}
	got[2].Index = 99
	if path.Segments()[2].Index != 5 {
		t.Fatal("returned path segments alias the immutable path")
	}
	if _, err := NewSunSpecFactPath([]SunSpecFactPathSegment{{Name: "Layer", Indexed: true, Index: 0}}); err != nil {
		t.Fatalf("zero-based indexed segment rejected: %v", err)
	}
	for _, segments := range [][]SunSpecFactPathSegment{
		nil,
		{{Name: ""}},
		{{Name: "Crv", Index: 1}},
	} {
		if _, err := NewSunSpecFactPath(segments); err == nil {
			t.Fatalf("invalid segments accepted: %#v", segments)
		}
	}
}

func TestSunSpecOccurrenceSourceRangeRetainsFragmentsWithoutAliasing(t *testing.T) {
	input := []SunSpecSourceSpan{
		{LogicalViewID: 11, PDUOffset: 120, WordCount: 3},
		{LogicalViewID: 12, PDUOffset: 0, WordCount: 1},
	}
	rangeValue, err := NewSunSpecOccurrenceSourceRange(40, 4, input)
	if err != nil {
		t.Fatal(err)
	}
	input[0].PDUOffset = 0
	if rangeValue.OccurrenceOffset() != 40 || rangeValue.WordCount() != 4 {
		t.Fatalf("range=(%d,%d)", rangeValue.OccurrenceOffset(), rangeValue.WordCount())
	}
	got := rangeValue.SourceSpans()
	want := []SunSpecSourceSpan{{LogicalViewID: 11, PDUOffset: 120, WordCount: 3}, {LogicalViewID: 12, PDUOffset: 0, WordCount: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spans=%#v", got)
	}
	got[0].LogicalViewID = 99
	if rangeValue.SourceSpans()[0].LogicalViewID != 11 {
		t.Fatal("returned source spans alias the immutable range")
	}
	for _, invalid := range []struct {
		offset, words uint32
		spans         []SunSpecSourceSpan
	}{
		{0, 0, []SunSpecSourceSpan{{LogicalViewID: 1, PDUOffset: 0, WordCount: 1}}},
		{65536, 1, []SunSpecSourceSpan{{LogicalViewID: 1, PDUOffset: 0, WordCount: 1}}},
		{65535, 2, []SunSpecSourceSpan{{LogicalViewID: 1, PDUOffset: 0, WordCount: 2}}},
		{0, 1, []SunSpecSourceSpan{{LogicalViewID: 0, PDUOffset: 0, WordCount: 1}}},
		{0, 1, []SunSpecSourceSpan{{LogicalViewID: 1, PDUOffset: 0, WordCount: 0}}},
		{0, 1, []SunSpecSourceSpan{{LogicalViewID: 1, PDUOffset: 65535, WordCount: 2}}},
		{0, 2, []SunSpecSourceSpan{{LogicalViewID: 1, PDUOffset: 0, WordCount: 1}}},
	} {
		if _, err := NewSunSpecOccurrenceSourceRange(invalid.offset, invalid.words, invalid.spans); err == nil {
			t.Fatalf("invalid source range accepted: %#v", invalid)
		}
	}
}

func TestSunSpecFactRemainsFlatWithoutNestedPrimitives(t *testing.T) {
	fact := SunSpecFact{GroupID: "module", RepeatIndex: 2, Repeated: true}
	if fact.GroupID != "module" || fact.RepeatIndex != 2 || !fact.Repeated {
		t.Fatalf("flat fact metadata changed: %#v", fact)
	}
}
