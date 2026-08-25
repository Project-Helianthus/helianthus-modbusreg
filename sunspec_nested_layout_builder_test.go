package modbusreg

import (
	"reflect"
	"testing"
)

const syntheticNestedLayoutRevision SunSpecSchemaRevision = "test.nested-layout.v1"
const syntheticNestedLayoutModelID uint16 = 990

func syntheticNestedLayoutTemplate(t *testing.T) sunSpecNestedLayoutTemplate {
	t.Helper()
	counts := []sunSpecNestedCountSpec{
		{name: "points", occurrenceWordOffset: 2, unavailable: 0xffff, min: 1, max: 2},
		{name: "curves", occurrenceWordOffset: 3, unavailable: 0xffff, min: 1, max: 2},
	}
	root := sunSpecNestedLayoutGroup{
		fields: []sunSpecNestedLayoutField{
			{name: "ID", wordCount: 1},
			{name: "L", wordCount: 1},
			{name: "NPt", wordCount: 1},
			{name: "NCrv", wordCount: 1},
		},
		children: []sunSpecNestedLayoutGroup{{
			name:        "Crv",
			repeatCount: "curves",
			indexBase:   1,
			fields:      []sunSpecNestedLayoutField{{name: "ReadOnly", wordCount: 1}},
			children: []sunSpecNestedLayoutGroup{{
				name:   "MustTrip",
				fields: []sunSpecNestedLayoutField{{name: "ActPt", wordCount: 1}},
				children: []sunSpecNestedLayoutGroup{{
					name:        "Pt",
					repeatCount: "points",
					indexBase:   1,
					fields: []sunSpecNestedLayoutField{
						{name: "Hz", wordCount: 1, emit: true},
						{name: "Tms", wordCount: 2, emit: true},
					},
				}},
			}},
		}},
	}
	template, err := newSunSpecNestedLayoutTemplate(
		sunSpecNestedTemplateKey{revision: syntheticNestedLayoutRevision, modelID: syntheticNestedLayoutModelID},
		counts,
		root,
	)
	if err != nil {
		t.Fatal(err)
	}
	counts[0].name = "mutated"
	root.children[0].name = "mutated"
	return template
}

func syntheticNestedLayoutWords(points, curves uint16) []uint16 {
	total := uint32(4 + uint32(curves)*(2+3*uint32(points)))
	words := make([]uint16, total)
	words[0] = syntheticNestedLayoutModelID
	words[1] = uint16(total - 2)
	words[2] = points
	words[3] = curves
	return words
}

func syntheticNestedLayoutSpans() []SunSpecSourceSpan {
	return []SunSpecSourceSpan{
		{LogicalViewID: 11, PDUOffset: 100, WordCount: 8},
		{LogicalViewID: 11, PDUOffset: 200, WordCount: 7},
		{LogicalViewID: 12, PDUOffset: 0, WordCount: 5},
	}
}

func TestSunSpecNestedLayoutBuildsImmutableSyntheticEntries(t *testing.T) {
	template := syntheticNestedLayoutTemplate(t)
	words := syntheticNestedLayoutWords(2, 2)
	spans := syntheticNestedLayoutSpans()
	layout, err := buildSunSpecNestedOccurrenceLayout(template, words, spans)
	if err != nil {
		t.Fatal(err)
	}
	entries := layout.Entries()
	if len(entries) != 8 {
		t.Fatalf("entries=%d", len(entries))
	}
	if _, ok := entries[0].PointType(); ok {
		t.Fatal("legacy synthetic entry unexpectedly has typed metadata")
	}
	if _, ok := entries[0].ScaleFactor(); ok {
		t.Fatal("legacy synthetic entry unexpectedly has a scale factor")
	}
	path := entries[0].Path()
	if !reflect.DeepEqual(path.Segments(), []SunSpecFactPathSegment{
		{Name: "Crv", Indexed: true, Index: 1},
		{Name: "MustTrip"},
		{Name: "Pt", Indexed: true, Index: 1},
		{Name: "Hz"},
	}) {
		t.Fatalf("first path=%#v", path.Segments())
	}
	secondRange := entries[1].SourceRange()
	if secondRange.OccurrenceOffset() != 7 || secondRange.WordCount() != 2 || !reflect.DeepEqual(secondRange.SourceSpans(), []SunSpecSourceSpan{
		{LogicalViewID: 11, PDUOffset: 107, WordCount: 1},
		{LogicalViewID: 11, PDUOffset: 200, WordCount: 1},
	}) {
		t.Fatalf("second range=%#v", secondRange)
	}

	entries[0].path.segments[0].Name = "mutated"
	entries[1].sourceRange.spans[0].PDUOffset = 0
	again := layout.Entries()
	if again[0].Path().Segments()[0].Name != "Crv" || again[1].SourceRange().SourceSpans()[0].PDUOffset != 107 {
		t.Fatal("layout entries alias immutable layout state")
	}

	shorter, err := buildSunSpecNestedOccurrenceLayout(template, syntheticNestedLayoutWords(1, 1), []SunSpecSourceSpan{{LogicalViewID: 13, PDUOffset: 0, WordCount: 9}})
	if err != nil {
		t.Fatal(err)
	}
	if len(shorter.Entries()) != 2 {
		t.Fatalf("per-occurrence layout was reused or cached: %d entries", len(shorter.Entries()))
	}
}

func TestSunSpecNestedLayoutRejectsInvalidOccurrenceWithoutEntries(t *testing.T) {
	template := syntheticNestedLayoutTemplate(t)
	words := syntheticNestedLayoutWords(2, 2)
	spans := syntheticNestedLayoutSpans()
	for _, invalid := range []struct {
		name  string
		words []uint16
		spans []SunSpecSourceSpan
	}{
		{"model", append([]uint16{991}, words[1:]...), spans},
		{"declared length", append([]uint16{words[0], words[1] - 1}, words[2:]...), spans},
		{"count sentinel", append([]uint16{words[0], words[1], 0xffff}, words[3:]...), spans},
		{"count below minimum", append([]uint16{words[0], words[1], 0}, words[3:]...), spans},
		{"count above maximum", append([]uint16{words[0], words[1], 3}, words[3:]...), spans},
		{"partial structural group", syntheticNestedLayoutWords(1, 2), []SunSpecSourceSpan{{LogicalViewID: 21, PDUOffset: 0, WordCount: 15}}},
		{"span aggregate", words, spans[:2]},
		{"malformed span", words, []SunSpecSourceSpan{{LogicalViewID: 0, PDUOffset: 0, WordCount: uint16(len(words))}}},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			layout, err := buildSunSpecNestedOccurrenceLayout(template, invalid.words, invalid.spans)
			if err == nil || len(layout.Entries()) != 0 {
				t.Fatalf("invalid occurrence returned layout=%#v err=%v", layout, err)
			}
		})
	}
}
