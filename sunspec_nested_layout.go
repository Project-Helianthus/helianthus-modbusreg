package modbusreg

import "fmt"

const maxSunSpecOccurrenceWords uint32 = 65536

// SunSpecFactPathSegment identifies one ordered group in a nested fact path.
// Indexed distinguishes a named group from a repeated group with an explicit index.
type SunSpecFactPathSegment struct {
	Name    string
	Index   uint32
	Indexed bool
}

// SunSpecFactPath is an immutable ordered hierarchy for a nested SunSpec fact.
type SunSpecFactPath struct{ segments []SunSpecFactPathSegment }

func NewSunSpecFactPath(segments []SunSpecFactPathSegment) (SunSpecFactPath, error) {
	if len(segments) == 0 {
		return SunSpecFactPath{}, fmt.Errorf("SunSpec fact path has no segments")
	}
	for _, segment := range segments {
		if segment.Name == "" {
			return SunSpecFactPath{}, fmt.Errorf("SunSpec fact path segment has no name")
		}
		if !segment.Indexed && segment.Index != 0 {
			return SunSpecFactPath{}, fmt.Errorf("SunSpec unindexed fact path segment has an index")
		}
	}
	return SunSpecFactPath{segments: append([]SunSpecFactPathSegment(nil), segments...)}, nil
}

func (p SunSpecFactPath) Segments() []SunSpecFactPathSegment {
	return append([]SunSpecFactPathSegment(nil), p.segments...)
}

// SunSpecOccurrenceSourceRange maps an occurrence-relative range to its exact,
// potentially fragmented physical source spans.
type SunSpecOccurrenceSourceRange struct {
	offset, wordCount uint32
	spans             []SunSpecSourceSpan
}

func NewSunSpecOccurrenceSourceRange(offset, wordCount uint32, spans []SunSpecSourceSpan) (SunSpecOccurrenceSourceRange, error) {
	if wordCount == 0 || offset >= maxSunSpecOccurrenceWords || wordCount > maxSunSpecOccurrenceWords-offset {
		return SunSpecOccurrenceSourceRange{}, fmt.Errorf("SunSpec occurrence source range is outside bounds")
	}
	if len(spans) == 0 {
		return SunSpecOccurrenceSourceRange{}, fmt.Errorf("SunSpec occurrence source range has no spans")
	}
	var total uint32
	for _, span := range spans {
		if span.LogicalViewID == 0 || span.WordCount == 0 || uint32(span.PDUOffset)+uint32(span.WordCount) > maxSunSpecOccurrenceWords {
			return SunSpecOccurrenceSourceRange{}, fmt.Errorf("SunSpec occurrence source span is outside bounds")
		}
		if total > wordCount-uint32(span.WordCount) {
			return SunSpecOccurrenceSourceRange{}, fmt.Errorf("SunSpec occurrence source spans exceed range")
		}
		total += uint32(span.WordCount)
	}
	if total != wordCount {
		return SunSpecOccurrenceSourceRange{}, fmt.Errorf("SunSpec occurrence source spans do not cover range")
	}
	return SunSpecOccurrenceSourceRange{offset: offset, wordCount: wordCount, spans: append([]SunSpecSourceSpan(nil), spans...)}, nil
}

func (r SunSpecOccurrenceSourceRange) OccurrenceOffset() uint32 { return r.offset }
func (r SunSpecOccurrenceSourceRange) WordCount() uint32        { return r.wordCount }
func (r SunSpecOccurrenceSourceRange) SourceSpans() []SunSpecSourceSpan {
	return append([]SunSpecSourceSpan(nil), r.spans...)
}
