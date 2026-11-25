package sunspec

import "fmt"

// Supported schema revisions. Each pins one exact model-definition set: V1 the
// read-only core catalog, V2 the offline DER catalog. See
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/sunspec/read-only-core-v1.md
// and
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/sunspec/read-only-core-v2.md.
const (
	ModelsRevisionV1 SchemaRevision = "sunspec.models@7abdf898-v1"
	ModelsRevisionV2 SchemaRevision = "sunspec.models@90b4a331-v2"
)

// Topology is the AC phase topology a catalog model declares.
type Topology string

// Topologies. TopologyNone applies to every model that is not an
// inverter model.
const (
	TopologyNone        Topology = "none"
	TopologySinglePhase Topology = "single_phase"
	TopologySplitPhase  Topology = "split_phase"
	TopologyThreePhase  Topology = "three_phase"
)

type sunSpecPointDefinition struct {
	name, fieldID, unit, scaleFactor string
	groupID                          string
	pointType                        PointType
	offset, size, repeatIndex        uint16
	mandatory, required, repeated    bool
	symbols                          map[uint64]string
	knownMask                        uint64
	fixedScale                       *int16
}

type sunSpecModelDefinition struct {
	key           DecoderKey
	topology      Topology
	compatibility bool
	points        []sunSpecPointDefinition
	geometry      func([]uint16) bool
	layout        func([]uint16) ([]sunSpecPointDefinition, bool)
}

func standardSunSpecModelDefinitions(revision SchemaRevision) ([]sunSpecModelDefinition, error) {
	switch revision {
	case ModelsRevisionV1:
		return standardSunSpecModelDefinitionsV1(revision)
	case ModelsRevisionV2:
		return standardSunSpecModelDefinitionsV2(revision)
	default:
		return nil, fmt.Errorf("SunSpec schema revision is unsupported")
	}
}

func standardSunSpecModelDefinitionsV2(revision SchemaRevision) ([]sunSpecModelDefinition, error) {
	definitions, err := appendSunSpecDefinition(nil, revision, 1, 66, TopologyNone, false, commonSunSpecPoints())
	if err != nil {
		return nil, err
	}
	for _, model := range []struct {
		id, length uint16
		points     []sunSpecPointDefinition
	}{
		{701, 153, derMeasureACV2SunSpecPoints()},
		{702, 50, derCapacityV2SunSpecPoints()},
		{703, 17, derEnterServiceV2SunSpecPoints()},
		{713, 7, derStorageCapacityV2SunSpecPoints()},
		{715, 7, derControlV2SunSpecPoints()},
		{802, 62, bessBaseV2SunSpecPoints()},
		{805, 42, bessModuleV2SunSpecPoints()},
		{806, 1, flowBatteryV2SunSpecPoints()},
		{807, 34, flowBatteryStringV2SunSpecPoints()},
		{808, 1, flowBatteryModuleV2SunSpecPoints()},
		{809, 1, flowBatteryStackV2SunSpecPoints()},
	} {
		definitions, err = appendSunSpecDefinition(definitions, revision, model.id, model.length, TopologyNone, false, model.points)
		if err != nil {
			return nil, err
		}
	}
	return definitions, nil
}

func standardSunSpecModelDefinitionsV1(revision SchemaRevision) ([]sunSpecModelDefinition, error) {
	common := commonSunSpecPoints()
	definitions := make([]sunSpecModelDefinition, 0, 27)
	var err error
	definitions, err = appendSunSpecDefinition(definitions, revision, 1, 66, TopologyNone, false, common)
	if err != nil {
		return nil, err
	}
	definitions, err = appendSunSpecDefinition(definitions, revision, 1, 65, TopologyNone, true, common[:len(common)-1])
	if err != nil {
		return nil, err
	}
	for _, model := range []struct {
		id, length uint16
		topology   Topology
		points     []sunSpecPointDefinition
	}{
		{101, 50, TopologySinglePhase, integerInverterSunSpecPoints()},
		{102, 50, TopologySplitPhase, integerInverterSunSpecPoints()},
		{103, 50, TopologyThreePhase, integerInverterSunSpecPoints()},
		{111, 60, TopologySinglePhase, floatInverterSunSpecPoints(inverterModel111StatusSymbols())},
		{112, 60, TopologySplitPhase, floatInverterSunSpecPoints(inverterStatusSymbols())},
		{113, 60, TopologyThreePhase, floatInverterSunSpecPoints(inverterStatusSymbols())},
	} {
		points := cloneSunSpecPointDefinitions(model.points)
		for index := range points {
			points[index].mandatory = sunSpecPointMandatory(model.id, points[index].name)
			points[index].required = points[index].mandatory && points[index].name != "ID" && points[index].name != "L" && points[index].pointType != TypeScaleFactor
		}
		definitions, err = appendSunSpecDefinition(definitions, revision, model.id, model.length, model.topology, false, points)
		if err != nil {
			return nil, err
		}
	}
	for _, model := range []struct {
		id, length uint16
		points     []sunSpecPointDefinition
	}{
		{120, 26, nameplateSunSpecPoints()},
		{121, 30, settingsSunSpecPoints()},
		{122, 44, statusSunSpecPoints()},
		{123, 24, immediateControlsSunSpecPoints()},
		{124, 24, storageSunSpecPoints()},
	} {
		definitions, err = appendSunSpecDefinition(definitions, revision, model.id, model.length, TopologyNone, false, model.points)
		if err != nil {
			return nil, err
		}
	}
	for _, model := range []struct {
		id, length uint16
		points     []sunSpecPointDefinition
	}{
		{201, 105, integerMeterSunSpecPoints(201)}, {202, 105, integerMeterSunSpecPoints(202)},
		{203, 105, integerMeterSunSpecPoints(203)}, {204, 105, integerMeterSunSpecPoints(204)},
		{211, 124, floatMeterSunSpecPoints(211)}, {212, 124, floatMeterSunSpecPoints(212)},
		{213, 124, floatMeterSunSpecPoints(213)}, {214, 124, floatMeterSunSpecPoints(214)},
	} {
		definitions, err = appendSunSpecDefinition(definitions, revision, model.id, model.length, TopologyNone, false, model.points)
		if err != nil {
			return nil, err
		}
	}
	for _, model := range []struct {
		id, length uint16
		points     []sunSpecPointDefinition
	}{
		{305, 36, locationSunSpecPoints()}, {306, 4, referencePointSunSpecPoints()},
		{307, 11, baseMetSunSpecPoints()}, {308, 4, miniMetSunSpecPoints()},
	} {
		definitions, err = appendSunSpecDefinition(definitions, revision, model.id, model.length, TopologyNone, false, model.points)
		if err != nil {
			return nil, err
		}
	}
	return gridSupportFixedDefinitions(revision, definitions)
}

func appendSunSpecDefinition(values []sunSpecModelDefinition, revision SchemaRevision, id, length uint16, topology Topology, compatibility bool, points []sunSpecPointDefinition) ([]sunSpecModelDefinition, error) {
	points = cloneSunSpecPointDefinitions(points)
	var extent uint32
	for index := range points {
		if points[index].size == 0 {
			return nil, fmt.Errorf("SunSpec point size is zero")
		}
		// Point starts remain uint16. A final one-word point may therefore start
		// at 65535 and bring an isolated offline extent to exactly 65536 words.
		if extent > 65535 || extent+uint32(points[index].size) > 65536 {
			return nil, fmt.Errorf("SunSpec model %d/%d catalog exceeds addressable point offsets", id, length)
		}
		points[index].offset = uint16(extent)
		extent += uint32(points[index].size)
	}
	if extent != uint32(length)+2 {
		return nil, fmt.Errorf("SunSpec model %d/%d catalog extent is %d", id, length, extent)
	}
	return append(values, sunSpecModelDefinition{key: DecoderKey{id, length, revision}, topology: topology, compatibility: compatibility, points: points}), nil
}

func cloneSunSpecPointDefinitions(points []sunSpecPointDefinition) []sunSpecPointDefinition {
	out := append([]sunSpecPointDefinition(nil), points...)
	for index := range out {
		out[index].symbols = cloneSunSpecSymbols(out[index].symbols)
	}
	return out
}

func cloneSunSpecSymbols(symbols map[uint64]string) map[uint64]string {
	if symbols == nil {
		return nil
	}
	out := make(map[uint64]string, len(symbols))
	for value, symbol := range symbols {
		out[value] = symbol
	}
	return out
}

func sunSpecPoint(name, fieldID string, pointType PointType, size uint16, unit, scaleFactor string, mandatory bool, symbols map[uint64]string, knownMask uint64) sunSpecPointDefinition {
	return sunSpecPointDefinition{name: name, fieldID: fieldID, pointType: pointType, size: size, unit: unit, scaleFactor: scaleFactor, mandatory: mandatory, required: mandatory && name != "ID" && name != "L" && pointType != TypeScaleFactor, symbols: symbols, knownMask: knownMask}
}

// FixedPoint describes one point at a fixed position in a model block.
type FixedPoint struct {
	Name       string
	FieldID    string
	Type       PointType
	Unit       string
	WordOffset uint16
	WordWidth  uint16
}

// FixedPointLayout returns a point and its linked scale point (zero when
// absent) from the exact fixed decoder definition. Offsets count from the
// model ID at word zero and include the ID and length header.
func (r DecoderRegistry) FixedPointLayout(key DecoderKey, field string) (point, scale FixedPoint, ok bool) {
	def, found := r.definitions[key]
	if !found || def.layout != nil || def.geometry != nil || field == "" {
		return point, scale, false
	}
	for _, p := range def.points {
		if p.fieldID != field {
			continue
		}
		point = fixedPoint(p)
		for _, s := range def.points {
			if s.pointType == TypeScaleFactor && s.name == p.scaleFactor {
				scale = fixedPoint(s)
				break
			}
		}
		return point, scale, true
	}
	return point, scale, false
}

func fixedPoint(p sunSpecPointDefinition) FixedPoint {
	return FixedPoint{Name: p.name, FieldID: p.fieldID, Type: p.pointType, Unit: p.unit, WordOffset: p.offset, WordWidth: p.size}
}
