package sunspec

import (
	"fmt"
	"sort"
	"sync"
)

// Fact is one decoded catalog point of an occurrence. FieldID is the
// registry's stable field name, empty for points the registry does not name;
// PointName is the SunSpec point name and Unit the catalog unit, empty when
// unitless. Required marks mandatory value points other than the header and
// scale factors. GroupID, RepeatIndex and Repeated place a point inside a
// repeating group. Nested-layout identity is optional and is attached only by
// WithNestedLayout.
type Fact struct {
	FieldID     string
	PointName   string
	Unit        string
	Required    bool
	GroupID     string
	RepeatIndex uint16
	Repeated    bool
	Value       Value
	path        FactPath
	sourceRange OccurrenceSourceRange
	hasLayout   bool
}

// WithNestedLayout returns a fact carrying optional future nested-layout
// identity. Existing flat facts remain unchanged until this method is used.
func (f Fact) WithNestedLayout(path FactPath, sourceRange OccurrenceSourceRange) (Fact, error) {
	validatedPath, err := NewFactPath(path.Segments())
	if err != nil {
		return Fact{}, err
	}
	validatedRange, err := NewOccurrenceSourceRange(sourceRange.offset, sourceRange.wordCount, sourceRange.spans)
	if err != nil {
		return Fact{}, err
	}
	f.path = validatedPath
	f.sourceRange = validatedRange
	f.hasLayout = true
	return f, nil
}

// NestedPath returns a copy of the nested path and true when the fact carries
// a nested layout.
func (f Fact) NestedPath() (FactPath, bool) {
	if !f.hasLayout {
		return FactPath{}, false
	}
	path, err := NewFactPath(f.path.Segments())
	if err != nil {
		return FactPath{}, false
	}
	return path, true
}

// OccurrenceSourceRange returns a copy of the occurrence-relative source range
// and true when the fact carries a nested layout.
func (f Fact) OccurrenceSourceRange() (OccurrenceSourceRange, bool) {
	if !f.hasLayout {
		return OccurrenceSourceRange{}, false
	}
	rangeValue, err := NewOccurrenceSourceRange(f.sourceRange.offset, f.sourceRange.wordCount, f.sourceRange.spans)
	if err != nil {
		return OccurrenceSourceRange{}, false
	}
	return rangeValue, true
}

// DecodedModel is the decoded form of one admitted occurrence: decoder
// key, ordinal, topology, raw words, source spans and facts in catalog order.
// Accessors return copies.
type DecodedModel struct {
	key           DecoderKey
	ordinal       uint32
	topology      Topology
	qualifies     bool
	geometryValid bool
	raw           []uint16
	spans         []SourceSpan
	facts         []Fact
}

// Key returns the exact decoder key the occurrence was decoded with.
func (m DecodedModel) Key() DecoderKey { return m.key }

// Ordinal returns the 1-based chain position of the occurrence.
func (m DecodedModel) Ordinal() uint32 { return m.ordinal }

// Topology returns the catalog phase topology; TopologyNone for models
// that are not inverter models.
func (m DecodedModel) Topology() Topology { return m.topology }

// Qualifies reports whether every mandatory point decoded valid, with a known
// enum symbol and no unknown bits. It is false when GeometryValid is false.
func (m DecodedModel) Qualifies() bool { return m.qualifies }

// GeometryValid reports whether the count words inside the model agree with
// its declared length. When it is false, Facts is empty.
func (m DecodedModel) GeometryValid() bool { return m.geometryValid }

// RawWords returns a copy of the occurrence words, header included.
func (m DecodedModel) RawWords() []uint16 { return append([]uint16(nil), m.raw...) }

// SourceSpans returns a copy of the occurrence's source spans.
func (m DecodedModel) SourceSpans() []SourceSpan {
	return append([]SourceSpan(nil), m.spans...)
}

// Facts returns copies of the decoded facts in catalog order. The two header
// words produce no fact.
func (m DecodedModel) Facts() []Fact {
	out := make([]Fact, len(m.facts))
	for index, fact := range m.facts {
		out[index] = cloneSunSpecFact(fact)
	}
	return out
}

// Fact returns a copy of the first fact with field identifier fieldID and
// true, or false when there is none.
func (m DecodedModel) Fact(fieldID string) (Fact, bool) {
	for _, fact := range m.facts {
		if fact.FieldID == fieldID {
			return cloneSunSpecFact(fact), true
		}
	}
	return Fact{}, false
}

// TextFact returns the text of the first fact with field identifier fieldID
// and true when that fact decoded valid as text. It returns "" and false when
// the fact is absent, not valid or not text.
func (m DecodedModel) TextFact(fieldID string) (string, bool) {
	fact, ok := m.Fact(fieldID)
	if !ok || fact.Value.State() != ValueValid {
		return "", false
	}
	return fact.Value.Text()
}

func cloneSunSpecFact(fact Fact) Fact {
	fact.Value.raw = append([]uint16(nil), fact.Value.raw...)
	fact.Value.bitSymbols = append([]string(nil), fact.Value.bitSymbols...)
	if fact.hasLayout {
		fact.path.segments = append([]FactPathSegment(nil), fact.path.segments...)
		fact.sourceRange.spans = append([]SourceSpan(nil), fact.sourceRange.spans...)
	}
	return fact
}

// DecodedChain holds the decoded admitted occurrences of one chain in
// chain order. Occurrences that were not admitted are skipped, not decoded.
type DecodedChain struct{ models []DecodedModel }

// Models returns deep copies of the decoded models in chain order.
func (c DecodedChain) Models() []DecodedModel {
	out := make([]DecodedModel, len(c.models))
	for index, model := range c.models {
		model.raw = append([]uint16(nil), model.raw...)
		model.spans = append([]SourceSpan(nil), model.spans...)
		model.facts = model.Facts()
		out[index] = model
	}
	return out
}

// DecoderRegistry is the immutable standard decoder catalog of one
// schema revision. It decodes only occurrences whose exact decoder key belongs
// to its revision and never infers a definition from a model identifier alone.
// It is safe for concurrent use. See
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/sunspec/read-only-core-v1.md
// and
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/sunspec/read-only-core-v2.md.
type DecoderRegistry struct {
	revision    SchemaRevision
	definitions map[DecoderKey]sunSpecModelDefinition
	keys        []DecoderKey
}

var standardSunSpecDecoderKeysCache struct {
	sync.Mutex
	keys map[SchemaRevision][]DecoderKey
}

// NewStandardSunSpecDecoderRegistry builds the registry for
// ModelsRevisionV1 or ModelsRevisionV2. Any other revision, or a
// catalog that fails its own extent or uniqueness checks, returns an error.
func NewStandardSunSpecDecoderRegistry(revision SchemaRevision) (DecoderRegistry, error) {
	definitions, err := standardSunSpecModelDefinitions(revision)
	if err != nil {
		return DecoderRegistry{}, err
	}
	registry := DecoderRegistry{revision: revision, definitions: make(map[DecoderKey]sunSpecModelDefinition, len(definitions))}
	for _, definition := range definitions {
		if _, duplicate := registry.definitions[definition.key]; duplicate {
			return DecoderRegistry{}, fmt.Errorf("SunSpec decoder key is duplicated")
		}
		registry.definitions[definition.key] = definition
	}
	registry.keys = standardSunSpecDecoderKeys(revision, definitions)
	return registry, nil
}

func standardSunSpecDecoderKeys(revision SchemaRevision, definitions []sunSpecModelDefinition) []DecoderKey {
	standardSunSpecDecoderKeysCache.Lock()
	defer standardSunSpecDecoderKeysCache.Unlock()
	if keys, ok := standardSunSpecDecoderKeysCache.keys[revision]; ok {
		return append([]DecoderKey(nil), keys...)
	}
	capacity := len(definitions)
	switch revision {
	case ModelsRevisionV1:
		capacity += 3277 + 89561 + gridSupportRepeatKeyCount()
	case ModelsRevisionV2:
		capacity += int(maxSunSpecDERPorts) + 1 + int(maxSunSpecBESSStrings) + 1 + int(maxSunSpecBESSModules) + 1
		capacity += len(derTripCurveLengthSet(9)) + len(derTripCurveLengthSet(12))
	}
	keys := make([]DecoderKey, 0, capacity)
	for _, definition := range definitions {
		keys = append(keys, definition.key)
	}
	switch revision {
	case ModelsRevisionV1:
		for modules := uint32(0); modules <= maxSunSpecMPPTModules; modules++ {
			keys = append(keys, DecoderKey{ModelID: 160, ModelLength: uint16(8 + 20*modules), SchemaRevision: revision})
		}
		keys = append(keys, environmentalSunSpecDecoderKeys(revision)...)
		keys = append(keys, gridSupportRepeatKeys(revision)...)
	case ModelsRevisionV2:
		for ports := uint32(0); ports <= maxSunSpecDERPorts; ports++ {
			keys = append(keys, DecoderKey{ModelID: 714, ModelLength: uint16(18 + 25*ports), SchemaRevision: revision})
		}
		for strings := uint32(0); strings <= maxSunSpecBESSStrings; strings++ {
			keys = append(keys, DecoderKey{ModelID: 803, ModelLength: uint16(26 + 32*strings), SchemaRevision: revision})
		}
		for modules := uint32(0); modules <= maxSunSpecBESSModules; modules++ {
			keys = append(keys, DecoderKey{ModelID: 804, ModelLength: uint16(46 + 16*modules), SchemaRevision: revision})
		}
		keys = append(keys, derTripCurveKeys(revision, 708, 9)...)
		keys = append(keys, derTripCurveKeys(revision, 709, 12)...)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ModelID != keys[j].ModelID {
			return keys[i].ModelID < keys[j].ModelID
		}
		if keys[i].ModelLength != keys[j].ModelLength {
			return keys[i].ModelLength < keys[j].ModelLength
		}
		return keys[i].SchemaRevision < keys[j].SchemaRevision
	})
	if standardSunSpecDecoderKeysCache.keys == nil {
		standardSunSpecDecoderKeysCache.keys = make(map[SchemaRevision][]DecoderKey)
	}
	standardSunSpecDecoderKeysCache.keys[revision] = append([]DecoderKey(nil), keys...)
	return append([]DecoderKey(nil), keys...)
}

// DecoderKeys returns a new sorted slice of every exact decoder key the
// registry admits, ordered by model identifier, length and revision. Models
// whose length depends on a repeat count contribute one key per admitted
// count. The result is the usual ChainPlanSpec.DecoderKeys.
func (r DecoderRegistry) DecoderKeys() []DecoderKey {
	return append([]DecoderKey(nil), r.keys...)
}

// SchemaRevision returns the schema revision the registry decodes; it is
// empty for the zero value.
func (r DecoderRegistry) SchemaRevision() SchemaRevision { return r.revision }

// DefinitionCount returns the number of fixed-length model definitions the
// registry holds. Definitions whose length follows a repeat count are resolved
// on demand and are not counted. It is zero for the zero value.
func (r DecoderRegistry) DefinitionCount() int { return len(r.definitions) }

// Clone returns a copy of the registry that shares no catalog storage with it:
// the decoder keys, the definitions and their points, enum symbols and fixed
// scales are copied.
func (r DecoderRegistry) Clone() DecoderRegistry {
	cloned := DecoderRegistry{revision: r.revision, keys: append([]DecoderKey(nil), r.keys...), definitions: make(map[DecoderKey]sunSpecModelDefinition, len(r.definitions))}
	for key, definition := range r.definitions {
		definition.points = cloneSunSpecPointDefinitions(definition.points)
		for index := range definition.points {
			if definition.points[index].fixedScale != nil {
				scale := *definition.points[index].fixedScale
				definition.points[index].fixedScale = &scale
			}
		}
		cloned.definitions[key] = definition
	}
	return cloned
}
func (r DecoderRegistry) definition(key DecoderKey) (sunSpecModelDefinition, bool) {
	definition, ok := r.definitions[key]
	if ok {
		return definition, true
	}
	if key.SchemaRevision != r.revision {
		return sunSpecModelDefinition{}, false
	}
	if key.SchemaRevision == ModelsRevisionV2 {
		var resolved sunSpecModelDefinition
		var err error
		switch key.ModelID {
		case 714:
			resolved, err = derDCMeasureV2SunSpecDefinition(key.ModelLength)
		case 803:
			resolved, err = bessBankV2SunSpecDefinition(key.ModelLength)
		case 804:
			resolved, err = bessStringV2SunSpecDefinition(key.ModelLength)
		case 708, 709:
			resolved, err = gridSupportSunSpecDefinition(key.SchemaRevision, key.ModelID, key.ModelLength)
		default:
			return sunSpecModelDefinition{}, false
		}
		return resolved, err == nil
	}
	if key.SchemaRevision != ModelsRevisionV1 {
		return sunSpecModelDefinition{}, false
	}
	var resolved sunSpecModelDefinition
	var err error
	switch key.ModelID {
	case 160:
		resolved, err = mpptSunSpecDefinition(key.SchemaRevision, key.ModelLength)
	case 302, 303, 304:
		resolved, err = environmentalSunSpecDefinition(key.SchemaRevision, key.ModelID, key.ModelLength)
	default:
		resolved, err = gridSupportSunSpecDefinition(key.SchemaRevision, key.ModelID, key.ModelLength)
	}
	return resolved, err == nil
}

// DecodeOccurrence decodes one admitted occurrence. It returns an error when
// the occurrence is not admitted, when its decoder key, wire key and revision
// disagree with each other or with the registry, when the key is not in the
// catalog, or when the words contradict the key. A geometry mismatch is not an
// error: the model is returned with GeometryValid and Qualifies false and no
// facts. A point that fails to decode becomes a fact with a non-valid state.
func (r DecoderRegistry) DecodeOccurrence(occurrence Occurrence) (DecodedModel, error) {
	key, ok := occurrence.DecoderKey()
	if !ok || occurrence.Disposition != ChainDispositionAdmitted || key.ModelID != occurrence.WireKey.ModelID || key.ModelLength != occurrence.WireKey.ModelLength || key.SchemaRevision != occurrence.SchemaRevision || key.SchemaRevision != r.revision {
		return DecodedModel{}, fmt.Errorf("SunSpec occurrence lacks an exact admitted decoder key")
	}
	definition, ok := r.definition(key)
	if !ok {
		return DecodedModel{}, fmt.Errorf("SunSpec decoder key is unsupported")
	}
	words := occurrence.Words()
	if len(words) != int(key.ModelLength)+2 || words[0] != key.ModelID || words[1] != key.ModelLength {
		return DecodedModel{}, fmt.Errorf("SunSpec occurrence words contradict decoder key")
	}
	model := DecodedModel{key: key, ordinal: occurrence.Ordinal, topology: definition.topology, qualifies: true, geometryValid: true, raw: append([]uint16(nil), words...), spans: occurrence.SourceSpans()}
	if definition.layout != nil {
		points, ok := definition.layout(words)
		if !ok {
			model.geometryValid = false
			model.qualifies = false
			return model, nil
		}
		definition.points = points
	} else if definition.geometry != nil && !definition.geometry(words) {
		model.geometryValid = false
		model.qualifies = false
		return model, nil
	}
	scales := make(map[sunSpecScaleKey]Value)
	for _, point := range definition.points {
		if point.pointType != TypeScaleFactor {
			continue
		}
		start := int(point.offset)
		end := start + int(point.size)
		scales[sunSpecScaleKey{name: point.name, group: point.groupID, index: point.repeatIndex}] = decodeSunSpecValue(point, words[start:end], nil)
	}
	for _, point := range definition.points {
		if point.offset < 2 {
			continue
		}
		var scale *Value
		if point.scaleFactor != "" {
			value, exists := sunSpecScaleValue(scales, point)
			if !exists {
				return DecodedModel{}, fmt.Errorf("SunSpec scale factor reference is missing")
			}
			scale = &value
		} else if point.fixedScale != nil {
			value := Value{pointType: TypeScaleFactor, state: ValueValid, signed: int64(*point.fixedScale), hasSigned: true}
			scale = &value
		}
		start := int(point.offset)
		end := start + int(point.size)
		value := decodeSunSpecValue(point, words[start:end], scale)
		if point.mandatory && !mandatorySunSpecValueQualifies(value) {
			model.qualifies = false
		}
		model.facts = append(model.facts, Fact{FieldID: point.fieldID, PointName: point.name, Unit: point.unit, Required: point.required, GroupID: point.groupID, RepeatIndex: point.repeatIndex, Repeated: point.repeated, Value: value})
	}
	return model, nil
}

type sunSpecScaleKey struct {
	name, group string
	index       uint16
}

func sunSpecScaleValue(scales map[sunSpecScaleKey]Value, point sunSpecPointDefinition) (Value, bool) {
	if value, ok := scales[sunSpecScaleKey{name: point.scaleFactor, group: point.groupID, index: point.repeatIndex}]; ok {
		return value, true
	}
	value, ok := scales[sunSpecScaleKey{name: point.scaleFactor}]
	return value, ok
}

func mandatorySunSpecValueQualifies(value Value) bool {
	if value.State() != ValueValid {
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

// DecodeChain decodes every admitted occurrence of snapshot in chain order and
// skips the others. It returns an error when the first occurrence is not the
// Common Model, the Common Model occurs more than once or is not admitted, or
// DecodeOccurrence fails for an admitted occurrence.
func (r DecoderRegistry) DecodeChain(snapshot ChainSnapshot) (DecodedChain, error) {
	return r.decodeOccurrences(snapshot.Occurrences())
}
func (r DecoderRegistry) decodeOccurrences(occurrences []Occurrence) (DecodedChain, error) {
	if len(occurrences) == 0 || occurrences[0].ModelID() != 1 {
		return DecodedChain{}, fmt.Errorf("SunSpec Common Model must be first")
	}
	commonCount := 0
	for _, occurrence := range occurrences {
		if occurrence.ModelID() == 1 {
			commonCount++
		}
	}
	if commonCount != 1 {
		return DecodedChain{}, fmt.Errorf("SunSpec Common Model must occur exactly once")
	}
	if occurrences[0].Disposition != ChainDispositionAdmitted {
		return DecodedChain{}, fmt.Errorf("SunSpec Common Model is unsupported")
	}
	out := DecodedChain{}
	for _, occurrence := range occurrences {
		if occurrence.Disposition != ChainDispositionAdmitted {
			continue
		}
		model, err := r.DecodeOccurrence(occurrence)
		if err != nil {
			return DecodedChain{}, err
		}
		out.models = append(out.models, model)
	}
	return out, nil
}
