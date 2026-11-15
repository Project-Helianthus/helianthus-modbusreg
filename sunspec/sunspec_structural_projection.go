package sunspec

// StructuralProjection is a detached offline projection of one
// privately validated structural candidate. It is neither a decoder selection
// nor a profile, qualification, or runtime admission result.
type StructuralProjection struct {
	wireKey  WireKey
	revision SchemaRevision
	ordinal  uint32
	raw      []uint16
	spans    []SourceSpan
	facts    []Fact
}

// WireKey returns the wire key of the projected occurrence.
func (p StructuralProjection) WireKey() WireKey { return p.wireKey }

// SchemaRevision returns the schema revision the chain was walked under.
func (p StructuralProjection) SchemaRevision() SchemaRevision {
	return p.revision
}

// Ordinal returns the 1-based chain position of the occurrence.
func (p StructuralProjection) Ordinal() uint32 { return p.ordinal }

// RawWords returns a copy of the occurrence words, header included.
func (p StructuralProjection) RawWords() []uint16 {
	return append([]uint16(nil), p.raw...)
}

// SourceSpans returns a copy of the occurrence's source spans.
func (p StructuralProjection) SourceSpans() []SourceSpan {
	return append([]SourceSpan(nil), p.spans...)
}

// Facts returns copies of the projected facts in layout order. Each carries a
// nested path and source range; a value that fails to decode keeps its raw
// words and a non-valid state.
func (p StructuralProjection) Facts() []Fact {
	out := make([]Fact, len(p.facts))
	for index, fact := range p.facts {
		out[index] = cloneSunSpecFact(fact)
	}
	return out
}

// ProjectSunSpecStructuralFacts projects only layout sidecars retained during
// post-payload structural selection. It never selects, admits, or decodes an
// occurrence, and does not recompute occurrence geometry.
func ProjectSunSpecStructuralFacts(snapshot ChainSnapshot) []StructuralProjection {
	projections := make([]StructuralProjection, 0)
	for _, occurrence := range snapshot.Occurrences() {
		projection, ok := projectSunSpecStructuralCandidate(occurrence)
		if ok {
			projections = append(projections, projection)
		}
	}
	return projections
}

func projectSunSpecStructuralCandidate(occurrence Occurrence) (StructuralProjection, bool) {
	candidate := occurrence.structuralCandidate
	if candidate == nil || candidate.modelID != 707 || occurrence.SchemaRevision != ModelsRevisionV2 || occurrence.WireKey.ModelID != candidate.modelID || occurrence.Disposition != ChainDispositionUnknownModel || occurrence.decoderKey != nil || candidate.layout.key != (sunSpecNestedTemplateKey{revision: ModelsRevisionV2, modelID: 707}) {
		return StructuralProjection{}, false
	}
	facts, ok := projectSunSpecStructuralLayout(occurrence.words, candidate.layout)
	if !ok {
		return StructuralProjection{}, false
	}
	return StructuralProjection{
		wireKey:  occurrence.WireKey,
		revision: occurrence.SchemaRevision,
		ordinal:  occurrence.Ordinal,
		raw:      append([]uint16(nil), occurrence.words...),
		spans:    append([]SourceSpan(nil), occurrence.spans...),
		facts:    facts,
	}, true
}

func projectSunSpecStructuralLayout(words []uint16, layout sunSpecNestedOccurrenceLayout) ([]Fact, bool) {
	entries := layout.Entries()
	if len(entries) == 0 {
		return []Fact{}, true
	}
	scales := make(map[string]Value)
	for _, entry := range entries {
		definition, raw, ok := sunSpecStructuralEntryDefinition(words, entry)
		if !ok {
			return nil, false
		}
		if definition.pointType == TypeScaleFactor {
			scales[definition.name] = decodeSunSpecValue(definition, raw, nil)
		}
	}
	facts := make([]Fact, 0, len(entries))
	for _, entry := range entries {
		definition, raw, ok := sunSpecStructuralEntryDefinition(words, entry)
		if !ok {
			return nil, false
		}
		var scale *Value
		if definition.scaleFactor != "" {
			value, exists := scales[definition.scaleFactor]
			if !exists {
				return nil, false
			}
			scale = &value
		}
		fact, err := (Fact{
			FieldID: definition.fieldID, PointName: definition.name, Unit: definition.unit,
			Value: decodeSunSpecValue(definition, raw, scale),
		}).WithNestedLayout(entry.Path(), entry.SourceRange())
		if err != nil {
			return nil, false
		}
		facts = append(facts, fact)
	}
	return facts, true
}

func sunSpecStructuralEntryDefinition(words []uint16, entry sunSpecNestedLayoutEntry) (sunSpecPointDefinition, []uint16, bool) {
	if !entry.hasValueMetadata {
		return sunSpecPointDefinition{}, nil, false
	}
	metadata := entry.valueMetadata
	rangeValue := entry.SourceRange()
	start, count := rangeValue.OccurrenceOffset(), rangeValue.WordCount()
	if count == 0 || start > uint32(len(words)) || count > uint32(len(words))-start || count > 0xffff || metadata.fieldID == "" || metadata.pointName == "" {
		return sunSpecPointDefinition{}, nil, false
	}
	definition := sunSpecPointDefinition{
		name: metadata.pointName, fieldID: metadata.fieldID, unit: metadata.unit,
		scaleFactor: metadata.scaleFactor, pointType: metadata.pointType, size: uint16(count),
		symbols: cloneSunSpecSymbols(metadata.symbols),
	}
	return definition, append([]uint16(nil), words[start:start+count]...), true
}
