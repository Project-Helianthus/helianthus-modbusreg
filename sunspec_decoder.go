package modbusreg

import (
	"fmt"
	"sort"
)

type SunSpecFact struct {
	FieldID     string
	PointName   string
	Unit        string
	Required    bool
	GroupID     string
	RepeatIndex uint16
	Repeated    bool
	Value       SunSpecValue
}

type SunSpecDecodedModel struct {
	key           SunSpecDecoderKey
	ordinal       uint32
	topology      SunSpecTopology
	qualifies     bool
	geometryValid bool
	raw           []uint16
	spans         []SunSpecSourceSpan
	facts         []SunSpecFact
}

func (m SunSpecDecodedModel) Key() SunSpecDecoderKey    { return m.key }
func (m SunSpecDecodedModel) Ordinal() uint32           { return m.ordinal }
func (m SunSpecDecodedModel) Topology() SunSpecTopology { return m.topology }
func (m SunSpecDecodedModel) Qualifies() bool           { return m.qualifies }
func (m SunSpecDecodedModel) GeometryValid() bool       { return m.geometryValid }
func (m SunSpecDecodedModel) RawWords() []uint16        { return append([]uint16(nil), m.raw...) }
func (m SunSpecDecodedModel) SourceSpans() []SunSpecSourceSpan {
	return append([]SunSpecSourceSpan(nil), m.spans...)
}
func (m SunSpecDecodedModel) Facts() []SunSpecFact {
	out := make([]SunSpecFact, len(m.facts))
	for index, fact := range m.facts {
		out[index] = cloneSunSpecFact(fact)
	}
	return out
}
func (m SunSpecDecodedModel) Fact(fieldID string) (SunSpecFact, bool) {
	for _, fact := range m.facts {
		if fact.FieldID == fieldID {
			return cloneSunSpecFact(fact), true
		}
	}
	return SunSpecFact{}, false
}
func cloneSunSpecFact(fact SunSpecFact) SunSpecFact {
	fact.Value.raw = append([]uint16(nil), fact.Value.raw...)
	fact.Value.bitSymbols = append([]string(nil), fact.Value.bitSymbols...)
	return fact
}

type SunSpecDecodedChain struct{ models []SunSpecDecodedModel }

func (c SunSpecDecodedChain) Models() []SunSpecDecodedModel {
	out := make([]SunSpecDecodedModel, len(c.models))
	for index, model := range c.models {
		model.raw = append([]uint16(nil), model.raw...)
		model.spans = append([]SunSpecSourceSpan(nil), model.spans...)
		model.facts = model.Facts()
		out[index] = model
	}
	return out
}

type SunSpecDecoderRegistry struct {
	revision    SunSpecSchemaRevision
	definitions map[SunSpecDecoderKey]sunSpecModelDefinition
	keys        []SunSpecDecoderKey
}

func NewStandardSunSpecDecoderRegistry(revision SunSpecSchemaRevision) (SunSpecDecoderRegistry, error) {
	definitions, err := standardSunSpecModelDefinitions(revision)
	if err != nil {
		return SunSpecDecoderRegistry{}, err
	}
	registry := SunSpecDecoderRegistry{revision: revision, definitions: make(map[SunSpecDecoderKey]sunSpecModelDefinition, len(definitions))}
	for _, definition := range definitions {
		if _, duplicate := registry.definitions[definition.key]; duplicate {
			return SunSpecDecoderRegistry{}, fmt.Errorf("SunSpec decoder key is duplicated")
		}
		registry.definitions[definition.key] = definition
		registry.keys = append(registry.keys, definition.key)
	}
	for modules := uint32(0); modules <= maxSunSpecMPPTModules; modules++ {
		registry.keys = append(registry.keys, SunSpecDecoderKey{ModelID: 160, ModelLength: uint16(8 + 20*modules), SchemaRevision: revision})
	}
	sort.Slice(registry.keys, func(i, j int) bool {
		if registry.keys[i].ModelID != registry.keys[j].ModelID {
			return registry.keys[i].ModelID < registry.keys[j].ModelID
		}
		if registry.keys[i].ModelLength != registry.keys[j].ModelLength {
			return registry.keys[i].ModelLength < registry.keys[j].ModelLength
		}
		return registry.keys[i].SchemaRevision < registry.keys[j].SchemaRevision
	})
	return registry, nil
}

func (r SunSpecDecoderRegistry) DecoderKeys() []SunSpecDecoderKey {
	return append([]SunSpecDecoderKey(nil), r.keys...)
}
func (r SunSpecDecoderRegistry) definition(key SunSpecDecoderKey) (sunSpecModelDefinition, bool) {
	definition, ok := r.definitions[key]
	if ok {
		return definition, true
	}
	if key.SchemaRevision != r.revision || key.ModelID != 160 {
		return sunSpecModelDefinition{}, false
	}
	definition, err := mpptSunSpecDefinition(key.SchemaRevision, key.ModelLength)
	return definition, err == nil
}

func (r SunSpecDecoderRegistry) DecodeOccurrence(occurrence SunSpecOccurrence) (SunSpecDecodedModel, error) {
	key, ok := occurrence.DecoderKey()
	if !ok || occurrence.Disposition != SunSpecChainDispositionAdmitted || key.ModelID != occurrence.WireKey.ModelID || key.ModelLength != occurrence.WireKey.ModelLength || key.SchemaRevision != occurrence.SchemaRevision || key.SchemaRevision != r.revision {
		return SunSpecDecodedModel{}, fmt.Errorf("SunSpec occurrence lacks an exact admitted decoder key")
	}
	definition, ok := r.definition(key)
	if !ok {
		return SunSpecDecodedModel{}, fmt.Errorf("SunSpec decoder key is unsupported")
	}
	words := occurrence.Words()
	if len(words) != int(key.ModelLength)+2 || words[0] != key.ModelID || words[1] != key.ModelLength {
		return SunSpecDecodedModel{}, fmt.Errorf("SunSpec occurrence words contradict decoder key")
	}
	model := SunSpecDecodedModel{key: key, ordinal: occurrence.Ordinal, topology: definition.topology, qualifies: true, geometryValid: true, raw: append([]uint16(nil), words...), spans: occurrence.SourceSpans()}
	if definition.geometry != nil && !definition.geometry(words) {
		model.geometryValid = false
		model.qualifies = false
		return model, nil
	}
	scales := make(map[string]SunSpecValue)
	for _, point := range definition.points {
		if point.pointType != SunSpecTypeScaleFactor {
			continue
		}
		scales[point.name] = decodeSunSpecValue(point, words[point.offset:point.offset+point.size], nil)
	}
	for _, point := range definition.points {
		if point.offset < 2 {
			continue
		}
		var scale *SunSpecValue
		if point.scaleFactor != "" {
			value, exists := scales[point.scaleFactor]
			if !exists {
				return SunSpecDecodedModel{}, fmt.Errorf("SunSpec scale factor reference is missing")
			}
			scale = &value
		}
		value := decodeSunSpecValue(point, words[point.offset:point.offset+point.size], scale)
		if point.mandatory && !mandatorySunSpecValueQualifies(value) {
			model.qualifies = false
		}
		model.facts = append(model.facts, SunSpecFact{FieldID: point.fieldID, PointName: point.name, Unit: point.unit, Required: point.required, GroupID: point.groupID, RepeatIndex: point.repeatIndex, Repeated: point.repeated, Value: value})
	}
	return model, nil
}

func mandatorySunSpecValueQualifies(value SunSpecValue) bool {
	if value.State() != SunSpecValueValid {
		return false
	}
	if _, symbol, ok := value.Enum(); ok {
		return symbol != ""
	}
	if _, unknown, ok := value.Bitfield(); ok {
		return unknown == 0
	}
	return true
}

func (r SunSpecDecoderRegistry) DecodeChain(snapshot SunSpecChainSnapshot) (SunSpecDecodedChain, error) {
	return r.decodeOccurrences(snapshot.Occurrences())
}
func (r SunSpecDecoderRegistry) decodeOccurrences(occurrences []SunSpecOccurrence) (SunSpecDecodedChain, error) {
	if len(occurrences) == 0 || occurrences[0].ModelID() != 1 {
		return SunSpecDecodedChain{}, fmt.Errorf("SunSpec Common Model must be first")
	}
	commonCount := 0
	for _, occurrence := range occurrences {
		if occurrence.ModelID() == 1 {
			commonCount++
		}
	}
	if commonCount != 1 {
		return SunSpecDecodedChain{}, fmt.Errorf("SunSpec Common Model must occur exactly once")
	}
	if occurrences[0].Disposition != SunSpecChainDispositionAdmitted {
		return SunSpecDecodedChain{}, fmt.Errorf("SunSpec Common Model is unsupported")
	}
	out := SunSpecDecodedChain{}
	for _, occurrence := range occurrences {
		if occurrence.Disposition != SunSpecChainDispositionAdmitted {
			continue
		}
		model, err := r.DecodeOccurrence(occurrence)
		if err != nil {
			return SunSpecDecodedChain{}, err
		}
		out.models = append(out.models, model)
	}
	return out, nil
}
