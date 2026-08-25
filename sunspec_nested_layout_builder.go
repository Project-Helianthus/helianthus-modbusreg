package modbusreg

import "fmt"

const maxSunSpecNestedLayoutDepth uint32 = 16

type sunSpecNestedTemplateKey struct {
	revision SunSpecSchemaRevision
	modelID  uint16
}

type sunSpecNestedCountSpec struct {
	name                 string
	occurrenceWordOffset uint32
	unavailable          uint16
	min, max             uint32
}

type sunSpecNestedLayoutField struct {
	name      string
	wordCount uint32
	emit      bool
}

type sunSpecNestedLayoutGroup struct {
	name        string
	repeatCount string
	indexBase   uint32
	fields      []sunSpecNestedLayoutField
	children    []sunSpecNestedLayoutGroup
}

type sunSpecNestedLayoutTemplate struct {
	key    sunSpecNestedTemplateKey
	counts []sunSpecNestedCountSpec
	root   sunSpecNestedLayoutGroup
}

type sunSpecNestedLayoutEntry struct {
	path        SunSpecFactPath
	sourceRange SunSpecOccurrenceSourceRange
}

func (e sunSpecNestedLayoutEntry) Path() SunSpecFactPath {
	path, err := NewSunSpecFactPath(e.path.Segments())
	if err != nil {
		return SunSpecFactPath{}
	}
	return path
}

func (e sunSpecNestedLayoutEntry) SourceRange() SunSpecOccurrenceSourceRange {
	rangeValue, err := NewSunSpecOccurrenceSourceRange(e.sourceRange.offset, e.sourceRange.wordCount, e.sourceRange.spans)
	if err != nil {
		return SunSpecOccurrenceSourceRange{}
	}
	return rangeValue
}

type sunSpecNestedOccurrenceLayout struct {
	key     sunSpecNestedTemplateKey
	entries []sunSpecNestedLayoutEntry
}

func (l sunSpecNestedOccurrenceLayout) Entries() []sunSpecNestedLayoutEntry {
	out := make([]sunSpecNestedLayoutEntry, len(l.entries))
	for index, entry := range l.entries {
		out[index] = sunSpecNestedLayoutEntry{path: entry.Path(), sourceRange: entry.SourceRange()}
	}
	return out
}

func newSunSpecNestedLayoutTemplate(key sunSpecNestedTemplateKey, counts []sunSpecNestedCountSpec, root sunSpecNestedLayoutGroup) (sunSpecNestedLayoutTemplate, error) {
	if !validSunSpecRevision(key.revision) || key.modelID == 0 {
		return sunSpecNestedLayoutTemplate{}, fmt.Errorf("SunSpec nested template key is invalid")
	}
	countNames := make(map[string]struct{}, len(counts))
	countOffsets := make(map[uint32]struct{}, len(counts))
	for _, count := range counts {
		if count.name == "" || count.min > count.max || count.max >= uint32(count.unavailable) {
			return sunSpecNestedLayoutTemplate{}, fmt.Errorf("SunSpec nested count specification is invalid")
		}
		if _, exists := countNames[count.name]; exists {
			return sunSpecNestedLayoutTemplate{}, fmt.Errorf("SunSpec nested count name is duplicated")
		}
		if _, exists := countOffsets[count.occurrenceWordOffset]; exists {
			return sunSpecNestedLayoutTemplate{}, fmt.Errorf("SunSpec nested count offset is duplicated")
		}
		countNames[count.name] = struct{}{}
		countOffsets[count.occurrenceWordOffset] = struct{}{}
	}
	if root.name != "" || root.repeatCount != "" || root.indexBase != 0 {
		return sunSpecNestedLayoutTemplate{}, fmt.Errorf("SunSpec nested template root is invalid")
	}
	if err := validateSunSpecNestedLayoutGroup(root, countNames, true, 0); err != nil {
		return sunSpecNestedLayoutTemplate{}, err
	}
	return sunSpecNestedLayoutTemplate{key: key, counts: append([]sunSpecNestedCountSpec(nil), counts...), root: cloneSunSpecNestedLayoutGroup(root)}, nil
}

func validateSunSpecNestedLayoutGroup(group sunSpecNestedLayoutGroup, countNames map[string]struct{}, root bool, depth uint32) error {
	if depth > maxSunSpecNestedLayoutDepth {
		return fmt.Errorf("SunSpec nested layout depth exceeds bounds")
	}
	if !root && group.name == "" {
		return fmt.Errorf("SunSpec nested layout group has no name")
	}
	if group.repeatCount == "" {
		if group.indexBase != 0 {
			return fmt.Errorf("SunSpec static layout group has an index base")
		}
	} else if _, exists := countNames[group.repeatCount]; !exists {
		return fmt.Errorf("SunSpec nested layout group references an unknown count")
	}
	for _, field := range group.fields {
		if field.name == "" || field.wordCount == 0 {
			return fmt.Errorf("SunSpec nested layout field is invalid")
		}
		if root && field.emit {
			return fmt.Errorf("SunSpec nested layout root cannot emit a path")
		}
	}
	for _, child := range group.children {
		if err := validateSunSpecNestedLayoutGroup(child, countNames, false, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func cloneSunSpecNestedLayoutGroup(group sunSpecNestedLayoutGroup) sunSpecNestedLayoutGroup {
	group.fields = append([]sunSpecNestedLayoutField(nil), group.fields...)
	group.children = append([]sunSpecNestedLayoutGroup(nil), group.children...)
	for index := range group.children {
		group.children[index] = cloneSunSpecNestedLayoutGroup(group.children[index])
	}
	return group
}

func buildSunSpecNestedOccurrenceLayout(template sunSpecNestedLayoutTemplate, words []uint16, spans []SunSpecSourceSpan) (sunSpecNestedOccurrenceLayout, error) {
	if len(words) < 2 || uint32(len(words)) > maxSunSpecOccurrenceWords || words[0] != template.key.modelID || uint32(words[1])+2 != uint32(len(words)) {
		return sunSpecNestedOccurrenceLayout{}, fmt.Errorf("SunSpec nested occurrence header is invalid")
	}
	if err := validateSunSpecNestedOccurrenceSpans(spans, uint32(len(words))); err != nil {
		return sunSpecNestedOccurrenceLayout{}, err
	}
	counts, err := resolveSunSpecNestedCounts(template.counts, words)
	if err != nil {
		return sunSpecNestedOccurrenceLayout{}, err
	}

	state := sunSpecNestedLayoutWalk{counts: counts, words: uint32(len(words)), spans: spans}
	if err := state.walk(template.root, nil); err != nil || state.offset != state.words {
		if err == nil {
			err = fmt.Errorf("SunSpec nested occurrence structural extent does not match declared length")
		}
		return sunSpecNestedOccurrenceLayout{}, err
	}
	if state.entryCount > state.words {
		return sunSpecNestedOccurrenceLayout{}, fmt.Errorf("SunSpec nested occurrence entry count exceeds bounds")
	}

	expectedEntries := state.entryCount
	entries := make([]sunSpecNestedLayoutEntry, 0, expectedEntries)
	state = sunSpecNestedLayoutWalk{counts: counts, words: uint32(len(words)), spans: spans, entries: &entries}
	if err := state.walk(template.root, nil); err != nil || state.offset != state.words || uint32(len(entries)) != expectedEntries {
		if err == nil {
			err = fmt.Errorf("SunSpec nested occurrence materialization is inconsistent")
		}
		return sunSpecNestedOccurrenceLayout{}, err
	}
	return sunSpecNestedOccurrenceLayout{key: template.key, entries: entries}, nil
}

func validateSunSpecNestedOccurrenceSpans(spans []SunSpecSourceSpan, expected uint32) error {
	if len(spans) == 0 {
		return fmt.Errorf("SunSpec nested occurrence has no source spans")
	}
	var total uint32
	for _, span := range spans {
		if span.LogicalViewID == 0 || span.WordCount == 0 || uint32(span.PDUOffset)+uint32(span.WordCount) > maxSunSpecOccurrenceWords || uint32(span.WordCount) > expected-total {
			return fmt.Errorf("SunSpec nested occurrence source spans are invalid")
		}
		total += uint32(span.WordCount)
	}
	if total != expected {
		return fmt.Errorf("SunSpec nested occurrence source spans do not cover words")
	}
	return nil
}

func resolveSunSpecNestedCounts(specs []sunSpecNestedCountSpec, words []uint16) (map[string]uint32, error) {
	counts := make(map[string]uint32, len(specs))
	for _, spec := range specs {
		if spec.occurrenceWordOffset >= uint32(len(words)) {
			return nil, fmt.Errorf("SunSpec nested count offset exceeds occurrence")
		}
		value := words[spec.occurrenceWordOffset]
		if value == spec.unavailable || uint32(value) < spec.min || uint32(value) > spec.max {
			return nil, fmt.Errorf("SunSpec nested count is invalid")
		}
		counts[spec.name] = uint32(value)
	}
	return counts, nil
}

type sunSpecNestedLayoutWalk struct {
	counts     map[string]uint32
	words      uint32
	spans      []SunSpecSourceSpan
	offset     uint32
	entryCount uint32
	entries    *[]sunSpecNestedLayoutEntry
}

func (w *sunSpecNestedLayoutWalk) walk(group sunSpecNestedLayoutGroup, parent []SunSpecFactPathSegment) error {
	repetitions := uint32(1)
	if group.repeatCount != "" {
		repetitions = w.counts[group.repeatCount]
	}
	for occurrence := uint32(0); occurrence < repetitions; occurrence++ {
		path := append([]SunSpecFactPathSegment(nil), parent...)
		if group.name != "" {
			segment := SunSpecFactPathSegment{Name: group.name}
			if group.repeatCount != "" {
				if group.indexBase > ^uint32(0)-occurrence {
					return fmt.Errorf("SunSpec nested layout index exceeds bounds")
				}
				segment.Indexed = true
				segment.Index = group.indexBase + occurrence
			}
			path = append(path, segment)
		}
		for _, field := range group.fields {
			start := w.offset
			if start > w.words || field.wordCount > w.words-start {
				return fmt.Errorf("SunSpec nested layout field exceeds occurrence")
			}
			w.offset += field.wordCount
			if !field.emit {
				continue
			}
			if w.entries == nil {
				if w.entryCount >= w.words {
					return fmt.Errorf("SunSpec nested layout entry count exceeds bounds")
				}
				w.entryCount++
				continue
			}
			segments := append([]SunSpecFactPathSegment(nil), path...)
			segments = append(segments, SunSpecFactPathSegment{Name: field.name})
			factPath, err := NewSunSpecFactPath(segments)
			if err != nil {
				return err
			}
			sourceRange, err := NewSunSpecOccurrenceSourceRangeFromSpans(start, field.wordCount, w.spans)
			if err != nil {
				return err
			}
			*w.entries = append(*w.entries, sunSpecNestedLayoutEntry{path: factPath, sourceRange: sourceRange})
		}
		for _, child := range group.children {
			if err := w.walk(child, path); err != nil {
				return err
			}
		}
	}
	return nil
}
