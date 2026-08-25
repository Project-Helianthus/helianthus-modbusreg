package modbusreg

// SunSpecStructuralProjection is a detached offline projection of one
// privately validated structural candidate. It is neither a decoder selection
// nor a profile, qualification, or runtime admission result.
type SunSpecStructuralProjection struct {
	wireKey  SunSpecWireKey
	revision SunSpecSchemaRevision
	ordinal  uint32
	raw      []uint16
	spans    []SunSpecSourceSpan
	facts    []SunSpecFact
}

func (p SunSpecStructuralProjection) WireKey() SunSpecWireKey { return p.wireKey }
func (p SunSpecStructuralProjection) SchemaRevision() SunSpecSchemaRevision {
	return p.revision
}
func (p SunSpecStructuralProjection) Ordinal() uint32 { return p.ordinal }
func (p SunSpecStructuralProjection) RawWords() []uint16 {
	return append([]uint16(nil), p.raw...)
}
func (p SunSpecStructuralProjection) SourceSpans() []SunSpecSourceSpan {
	return append([]SunSpecSourceSpan(nil), p.spans...)
}
func (p SunSpecStructuralProjection) Facts() []SunSpecFact {
	out := make([]SunSpecFact, len(p.facts))
	for index, fact := range p.facts {
		out[index] = cloneSunSpecFact(fact)
	}
	return out
}

// ProjectSunSpecStructuralFacts projects only layout sidecars retained during
// post-payload structural selection. It never selects, admits, or decodes an
// occurrence, and does not recompute occurrence geometry.
func ProjectSunSpecStructuralFacts(snapshot SunSpecChainSnapshot) []SunSpecStructuralProjection {
	projections := make([]SunSpecStructuralProjection, 0)
	for _, occurrence := range snapshot.Occurrences() {
		projection, ok := projectSunSpecStructuralCandidate(occurrence)
		if ok {
			projections = append(projections, projection)
		}
	}
	return projections
}

func projectSunSpecStructuralCandidate(occurrence SunSpecOccurrence) (SunSpecStructuralProjection, bool) {
	candidate := occurrence.structuralCandidate
	if candidate == nil || candidate.modelID != 707 || occurrence.SchemaRevision != SunSpecModelsRevisionV2 || occurrence.WireKey.ModelID != candidate.modelID || occurrence.Disposition != SunSpecChainDispositionUnknownModel || occurrence.decoderKey != nil || candidate.layout.key != (sunSpecNestedTemplateKey{revision: SunSpecModelsRevisionV2, modelID: 707}) {
		return SunSpecStructuralProjection{}, false
	}
	facts, ok := projectSunSpecStructuralLayout(occurrence.words, candidate.layout)
	if !ok {
		return SunSpecStructuralProjection{}, false
	}
	return SunSpecStructuralProjection{
		wireKey:  occurrence.WireKey,
		revision: occurrence.SchemaRevision,
		ordinal:  occurrence.Ordinal,
		raw:      append([]uint16(nil), occurrence.words...),
		spans:    append([]SunSpecSourceSpan(nil), occurrence.spans...),
		facts:    facts,
	}, true
}

func projectSunSpecStructuralLayout(words []uint16, layout sunSpecNestedOccurrenceLayout) ([]SunSpecFact, bool) {
	entries := layout.Entries()
	if len(entries) == 0 {
		return []SunSpecFact{}, true
	}
	scales := make(map[string]SunSpecValue)
	for _, entry := range entries {
		definition, raw, ok := sunSpecStructuralEntryDefinition(words, entry)
		if !ok {
			return nil, false
		}
		if definition.pointType == SunSpecTypeScaleFactor {
			scales[definition.name] = decodeSunSpecValue(definition, raw, nil)
		}
	}
	facts := make([]SunSpecFact, 0, len(entries))
	for _, entry := range entries {
		definition, raw, ok := sunSpecStructuralEntryDefinition(words, entry)
		if !ok {
			return nil, false
		}
		var scale *SunSpecValue
		if definition.scaleFactor != "" {
			value, exists := scales[definition.scaleFactor]
			if !exists {
				return nil, false
			}
			scale = &value
		}
		fact, err := (SunSpecFact{
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
