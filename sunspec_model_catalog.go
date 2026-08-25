package modbusreg

import "fmt"

const (
	SunSpecModelsRevisionV1 SunSpecSchemaRevision = "sunspec.models@7abdf898-v1"
	SunSpecModelsRevisionV2 SunSpecSchemaRevision = "sunspec.models@90b4a331-v2"
)

type SunSpecTopology string

const (
	SunSpecTopologyNone        SunSpecTopology = "none"
	SunSpecTopologySinglePhase SunSpecTopology = "single_phase"
	SunSpecTopologySplitPhase  SunSpecTopology = "split_phase"
	SunSpecTopologyThreePhase  SunSpecTopology = "three_phase"
)

type sunSpecPointDefinition struct {
	name, fieldID, unit, scaleFactor string
	groupID                          string
	pointType                        SunSpecPointType
	offset, size, repeatIndex        uint16
	mandatory, required, repeated    bool
	symbols                          map[uint64]string
	knownMask                        uint64
	fixedScale                       *int16
}

type sunSpecModelDefinition struct {
	key           SunSpecDecoderKey
	topology      SunSpecTopology
	compatibility bool
	points        []sunSpecPointDefinition
	geometry      func([]uint16) bool
}

func standardSunSpecModelDefinitions(revision SunSpecSchemaRevision) ([]sunSpecModelDefinition, error) {
	switch revision {
	case SunSpecModelsRevisionV1:
		return standardSunSpecModelDefinitionsV1(revision)
	case SunSpecModelsRevisionV2:
		return standardSunSpecModelDefinitionsV2(revision)
	default:
		return nil, fmt.Errorf("SunSpec schema revision is unsupported")
	}
}

func standardSunSpecModelDefinitionsV2(revision SunSpecSchemaRevision) ([]sunSpecModelDefinition, error) {
	definitions, err := appendSunSpecDefinition(nil, revision, 1, 66, SunSpecTopologyNone, false, commonSunSpecPoints())
	if err != nil {
		return nil, err
	}
	for _, model := range []struct {
		id, length uint16
		points     []sunSpecPointDefinition
	}{
		{701, 153, derMeasureACV2SunSpecPoints()},
		{702, 50, derCapacityV2SunSpecPoints()},
	} {
		definitions, err = appendSunSpecDefinition(definitions, revision, model.id, model.length, SunSpecTopologyNone, false, model.points)
		if err != nil {
			return nil, err
		}
	}
	return definitions, nil
}

func standardSunSpecModelDefinitionsV1(revision SunSpecSchemaRevision) ([]sunSpecModelDefinition, error) {
	common := commonSunSpecPoints()
	definitions := make([]sunSpecModelDefinition, 0, 27)
	var err error
	definitions, err = appendSunSpecDefinition(definitions, revision, 1, 66, SunSpecTopologyNone, false, common)
	if err != nil {
		return nil, err
	}
	definitions, err = appendSunSpecDefinition(definitions, revision, 1, 65, SunSpecTopologyNone, true, common[:len(common)-1])
	if err != nil {
		return nil, err
	}
	for _, model := range []struct {
		id, length uint16
		topology   SunSpecTopology
		points     []sunSpecPointDefinition
	}{
		{101, 50, SunSpecTopologySinglePhase, integerInverterSunSpecPoints()},
		{102, 50, SunSpecTopologySplitPhase, integerInverterSunSpecPoints()},
		{103, 50, SunSpecTopologyThreePhase, integerInverterSunSpecPoints()},
		{111, 60, SunSpecTopologySinglePhase, floatInverterSunSpecPoints(inverterModel111StatusSymbols())},
		{112, 60, SunSpecTopologySplitPhase, floatInverterSunSpecPoints(inverterStatusSymbols())},
		{113, 60, SunSpecTopologyThreePhase, floatInverterSunSpecPoints(inverterStatusSymbols())},
	} {
		points := cloneSunSpecPointDefinitions(model.points)
		for index := range points {
			points[index].mandatory = sunSpecPointMandatory(model.id, points[index].name)
			points[index].required = points[index].mandatory && points[index].name != "ID" && points[index].name != "L" && points[index].pointType != SunSpecTypeScaleFactor
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
		definitions, err = appendSunSpecDefinition(definitions, revision, model.id, model.length, SunSpecTopologyNone, false, model.points)
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
		definitions, err = appendSunSpecDefinition(definitions, revision, model.id, model.length, SunSpecTopologyNone, false, model.points)
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
		definitions, err = appendSunSpecDefinition(definitions, revision, model.id, model.length, SunSpecTopologyNone, false, model.points)
		if err != nil {
			return nil, err
		}
	}
	return definitions, nil
}

func appendSunSpecDefinition(values []sunSpecModelDefinition, revision SunSpecSchemaRevision, id, length uint16, topology SunSpecTopology, compatibility bool, points []sunSpecPointDefinition) ([]sunSpecModelDefinition, error) {
	points = cloneSunSpecPointDefinitions(points)
	var extent uint32
	for index := range points {
		if points[index].size == 0 {
			return nil, fmt.Errorf("SunSpec point size is zero")
		}
		if extent > 65535 || extent+uint32(points[index].size) > 65535 {
			return nil, fmt.Errorf("SunSpec model %d/%d catalog exceeds addressable point offsets", id, length)
		}
		points[index].offset = uint16(extent)
		extent += uint32(points[index].size)
	}
	if extent != uint32(length)+2 {
		return nil, fmt.Errorf("SunSpec model %d/%d catalog extent is %d", id, length, extent)
	}
	return append(values, sunSpecModelDefinition{key: SunSpecDecoderKey{id, length, revision}, topology: topology, compatibility: compatibility, points: points}), nil
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

func sunSpecPoint(name, fieldID string, pointType SunSpecPointType, size uint16, unit, scaleFactor string, mandatory bool, symbols map[uint64]string, knownMask uint64) sunSpecPointDefinition {
	return sunSpecPointDefinition{name: name, fieldID: fieldID, pointType: pointType, size: size, unit: unit, scaleFactor: scaleFactor, mandatory: mandatory, required: mandatory && name != "ID" && name != "L" && pointType != SunSpecTypeScaleFactor, symbols: symbols, knownMask: knownMask}
}
