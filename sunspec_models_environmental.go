package modbusreg

import "fmt"

const maxAddressableSunSpecModelLength uint16 = 65533

func environmentalSunSpecDecoderKeys(revision SunSpecSchemaRevision) []SunSpecDecoderKey {
	keys := make([]SunSpecDecoderKey, 0, 89561)
	for length := uint32(5); length <= uint32(maxAddressableSunSpecModelLength); length += 5 {
		keys = append(keys, SunSpecDecoderKey{302, uint16(length), revision})
	}
	for length := uint32(1); length <= uint32(maxAddressableSunSpecModelLength); length++ {
		keys = append(keys, SunSpecDecoderKey{303, uint16(length), revision})
	}
	for length := uint32(6); length <= uint32(maxAddressableSunSpecModelLength); length += 6 {
		keys = append(keys, SunSpecDecoderKey{304, uint16(length), revision})
	}
	return keys
}

func environmentalSunSpecDefinition(revision SunSpecSchemaRevision, id, length uint16) (sunSpecModelDefinition, error) {
	if revision != SunSpecModelsRevisionV1 || length == 0 || length > maxAddressableSunSpecModelLength {
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec environmental key is invalid")
	}
	var group string
	var repeated []sunSpecPointDefinition
	var words uint16
	switch id {
	case 302:
		group, words = "repeating", 5
		repeated = []sunSpecPointDefinition{
			extendedSunSpecPoint("GHI", "environment.irradiance.global_horizontal", SunSpecTypeUint16, 1, "W/m2", "", false),
			extendedSunSpecPoint("POAI", "environment.irradiance.plane_of_array", SunSpecTypeUint16, 1, "W/m2", "", false),
			extendedSunSpecPoint("DFI", "environment.irradiance.diffuse", SunSpecTypeUint16, 1, "W/m2", "", false),
			extendedSunSpecPoint("DNI", "environment.irradiance.direct_normal", SunSpecTypeUint16, 1, "W/m2", "", false),
			extendedSunSpecPoint("OTI", "environment.irradiance.other", SunSpecTypeUint16, 1, "W/m2", "", false),
		}
	case 303:
		group, words = "temp", 1
		repeated = []sunSpecPointDefinition{environmentalScaledPoint("TmpBOM", "environment.temperature.back_of_module", SunSpecTypeInt16, 1, "C", -1, true)}
	case 304:
		group, words = "incl", 6
		repeated = []sunSpecPointDefinition{
			environmentalScaledPoint("Inclx", "environment.inclination.x", SunSpecTypeInt32, 2, "Degrees", -2, true),
			environmentalScaledPoint("Incly", "environment.inclination.y", SunSpecTypeInt32, 2, "Degrees", -2, false),
			environmentalScaledPoint("Inclz", "environment.inclination.z", SunSpecTypeInt32, 2, "Degrees", -2, false),
		}
	default:
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec environmental model is unsupported")
	}
	if length%words != 0 {
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec environmental key has invalid geometry")
	}
	points := meterHeaderSunSpecPoints()
	for index := uint16(1); index <= length/words; index++ {
		for _, point := range repeated {
			point.groupID, point.repeatIndex, point.repeated = group, index, true
			points = append(points, point)
		}
	}
	definitions, err := appendSunSpecDefinition(nil, revision, id, length, SunSpecTopologyNone, false, points)
	if err != nil {
		return sunSpecModelDefinition{}, err
	}
	return definitions[0], nil
}

func locationSunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		extendedSunSpecPoint("ID", "", SunSpecTypeUint16, 1, "", "", true), extendedSunSpecPoint("L", "", SunSpecTypeUint16, 1, "", "", true),
		extendedSunSpecPoint("Tm", "environment.location.time", SunSpecTypeString, 6, "hhmmss.sssZ", "", false),
		extendedSunSpecPoint("Date", "environment.location.date", SunSpecTypeString, 4, "YYYYMMDD", "", false),
		extendedSunSpecPoint("Loc", "environment.location.description", SunSpecTypeString, 20, "text", "", false),
		environmentalScaledPoint("Lat", "environment.location.latitude", SunSpecTypeInt32, 2, "Degrees", -7, false),
		environmentalScaledPoint("Long", "environment.location.longitude", SunSpecTypeInt32, 2, "Degrees", -7, false),
		extendedSunSpecPoint("Alt", "environment.location.altitude", SunSpecTypeInt32, 2, "meters", "", false),
	}
}

func referencePointSunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		extendedSunSpecPoint("ID", "", SunSpecTypeUint16, 1, "", "", true), extendedSunSpecPoint("L", "", SunSpecTypeUint16, 1, "", "", true),
		extendedSunSpecPoint("GHI", "environment.reference.global_horizontal_irradiance", SunSpecTypeUint16, 1, "W/m2", "", false),
		extendedSunSpecPoint("A", "environment.reference.current", SunSpecTypeUint16, 1, "W/m2", "", false),
		extendedSunSpecPoint("V", "environment.reference.voltage", SunSpecTypeUint16, 1, "W/m2", "", false),
		extendedSunSpecPoint("Tmp", "environment.reference.temperature", SunSpecTypeUint16, 1, "W/m2", "", false),
	}
}

func baseMetSunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		extendedSunSpecPoint("ID", "", SunSpecTypeUint16, 1, "", "", true), extendedSunSpecPoint("L", "", SunSpecTypeUint16, 1, "", "", true),
		environmentalScaledPoint("TmpAmb", "environment.temperature.ambient", SunSpecTypeInt16, 1, "C", -1, false),
		extendedSunSpecPoint("RH", "environment.humidity.relative", SunSpecTypeInt16, 1, "Pct", "", false),
		extendedSunSpecPoint("Pres", "environment.pressure", SunSpecTypeInt16, 1, "HPa", "", false),
		extendedSunSpecPoint("WndSpd", "environment.wind.speed", SunSpecTypeInt16, 1, "mps", "", false),
		extendedSunSpecPoint("WndDir", "environment.wind.direction", SunSpecTypeInt16, 1, "deg", "", false),
		extendedSunSpecPoint("Rain", "environment.precipitation.rain", SunSpecTypeInt16, 1, "mm", "", false),
		extendedSunSpecPoint("Snw", "environment.precipitation.snow", SunSpecTypeInt16, 1, "mm", "", false),
		extendedSunSpecPoint("PPT", "environment.precipitation.type", SunSpecTypeInt16, 1, "", "", false),
		extendedSunSpecPoint("ElecFld", "environment.electric_field", SunSpecTypeInt16, 1, "Vm", "", false),
		extendedSunSpecPoint("SurWet", "environment.surface_wetness", SunSpecTypeInt16, 1, "kO", "", false),
		extendedSunSpecPoint("SoilWet", "environment.soil_wetness", SunSpecTypeInt16, 1, "Pct", "", false),
	}
}

func miniMetSunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		extendedSunSpecPoint("ID", "", SunSpecTypeUint16, 1, "", "", true), extendedSunSpecPoint("L", "", SunSpecTypeUint16, 1, "", "", true),
		extendedSunSpecPoint("GHI", "environment.irradiance.global_horizontal", SunSpecTypeUint16, 1, "W/m2", "", false),
		environmentalScaledPoint("TmpBOM", "environment.temperature.back_of_module", SunSpecTypeInt16, 1, "C", -1, false),
		environmentalScaledPoint("TmpAmb", "environment.temperature.ambient", SunSpecTypeInt16, 1, "C", -1, false),
		extendedSunSpecPoint("WndSpd", "environment.wind.speed", SunSpecTypeUint16, 1, "m/s", "", false),
	}
}

func environmentalScaledPoint(name, field string, pointType SunSpecPointType, size uint16, unit string, exponent int16, mandatory bool) sunSpecPointDefinition {
	point := extendedSunSpecPoint(name, field, pointType, size, unit, "", mandatory)
	point.fixedScale = &exponent
	return point
}
