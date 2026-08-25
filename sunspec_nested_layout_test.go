package modbusreg

import (
	"encoding/json"
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

func TestSunSpecFactNestedCarriersCloneAndPreserveLegacyJSON(t *testing.T) {
	base := SunSpecFact{FieldID: "v", PointName: "V", Unit: "V", GroupID: "module", RepeatIndex: 2, Repeated: true}
	legacyJSON, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := base.NestedPath(); ok {
		t.Fatal("flat fact unexpectedly has a nested path")
	}
	if _, ok := base.OccurrenceSourceRange(); ok {
		t.Fatal("flat fact unexpectedly has a source range")
	}

	path, err := NewSunSpecFactPath([]SunSpecFactPathSegment{{Name: "Crv", Indexed: true, Index: 2}, {Name: "MustTrip"}, {Name: "Pt", Indexed: true, Index: 5}})
	if err != nil {
		t.Fatal(err)
	}
	sourceRange, err := NewSunSpecOccurrenceSourceRange(7, 2, []SunSpecSourceSpan{{LogicalViewID: 41, PDUOffset: 50, WordCount: 1}, {LogicalViewID: 42, PDUOffset: 90, WordCount: 1}})
	if err != nil {
		t.Fatal(err)
	}
	withCarriers, err := base.WithNestedLayout(path, sourceRange)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := withCarriers.NestedPath(); !ok || !reflect.DeepEqual(got.Segments(), path.Segments()) {
		t.Fatalf("path=%#v present=%t", got.Segments(), ok)
	}
	if got, ok := withCarriers.OccurrenceSourceRange(); !ok || !reflect.DeepEqual(got.SourceSpans(), sourceRange.SourceSpans()) || got.OccurrenceOffset() != 7 {
		t.Fatalf("source range=%#v present=%t", got, ok)
	}

	returnedPath, _ := withCarriers.NestedPath()
	returnedSegments := returnedPath.Segments()
	returnedSegments[0].Name = "changed"
	returnedRange, _ := withCarriers.OccurrenceSourceRange()
	returnedSpans := returnedRange.SourceSpans()
	returnedSpans[0].PDUOffset = 0
	if got, _ := withCarriers.NestedPath(); got.Segments()[0].Name != "Crv" {
		t.Fatal("fact path accessor aliases internal state")
	}
	if got, _ := withCarriers.OccurrenceSourceRange(); got.SourceSpans()[0].PDUOffset != 50 {
		t.Fatal("fact source range accessor aliases internal state")
	}

	clone := cloneSunSpecFact(withCarriers)
	clonePath, _ := clone.NestedPath()
	cloneSegments := clonePath.Segments()
	cloneSegments[0].Name = "clone"
	cloneRange, _ := clone.OccurrenceSourceRange()
	cloneSpans := cloneRange.SourceSpans()
	cloneSpans[0].PDUOffset = 1
	if got, _ := withCarriers.NestedPath(); got.Segments()[0].Name != "Crv" {
		t.Fatal("fact clone aliases path state")
	}
	if got, _ := withCarriers.OccurrenceSourceRange(); got.SourceSpans()[0].PDUOffset != 50 {
		t.Fatal("fact clone aliases source range state")
	}

	gotJSON, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotJSON, legacyJSON) || base.GroupID != "module" || base.RepeatIndex != 2 || !base.Repeated {
		t.Fatalf("legacy fact changed: json=%s base=%#v", gotJSON, base)
	}
}

func TestSunSpecOccurrenceSourceRangeFromSpansSlicesWithoutCoalescing(t *testing.T) {
	spans := []SunSpecSourceSpan{
		{LogicalViewID: 11, PDUOffset: 100, WordCount: 2},
		{LogicalViewID: 11, PDUOffset: 200, WordCount: 2},
		{LogicalViewID: 12, PDUOffset: 0, WordCount: 2},
	}
	rangeValue, err := NewSunSpecOccurrenceSourceRangeFromSpans(1, 5, spans)
	if err != nil {
		t.Fatal(err)
	}
	want := []SunSpecSourceSpan{
		{LogicalViewID: 11, PDUOffset: 101, WordCount: 1},
		{LogicalViewID: 11, PDUOffset: 200, WordCount: 2},
		{LogicalViewID: 12, PDUOffset: 0, WordCount: 2},
	}
	if !reflect.DeepEqual(rangeValue.SourceSpans(), want) {
		t.Fatalf("sliced spans=%#v", rangeValue.SourceSpans())
	}
	got := rangeValue.SourceSpans()
	got[0].PDUOffset = 999
	if rangeValue.SourceSpans()[0].PDUOffset != 101 {
		t.Fatal("sliced spans alias returned range")
	}

	for _, invalid := range []struct {
		offset, words uint32
		spans         []SunSpecSourceSpan
	}{
		{0, 1, nil},
		{0, 1, []SunSpecSourceSpan{{LogicalViewID: 0, PDUOffset: 0, WordCount: 1}}},
		{0, 1, []SunSpecSourceSpan{{LogicalViewID: 1, PDUOffset: 0, WordCount: 0}}},
		{0, 1, []SunSpecSourceSpan{{LogicalViewID: 1, PDUOffset: 65535, WordCount: 2}}},
		{6, 1, spans},
		{0, 7, spans},
		{0, 1, []SunSpecSourceSpan{{LogicalViewID: 1, PDUOffset: 0, WordCount: 65535}, {LogicalViewID: 2, PDUOffset: 0, WordCount: 2}}},
	} {
		if _, err := NewSunSpecOccurrenceSourceRangeFromSpans(invalid.offset, invalid.words, invalid.spans); err == nil {
			t.Fatalf("invalid aggregate accepted: %#v", invalid)
		}
	}
}
