package modbusreg

import "fmt"

const maxSunSpecMPPTModules uint32 = (65535 - 8) / 20

func mpptSunSpecDefinition(revision SunSpecSchemaRevision, length uint16) (sunSpecModelDefinition, error) {
	if revision != SunSpecModelsRevisionV1 || length < 8 || (uint32(length)-8)%20 != 0 {
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec Model 160 key has invalid geometry")
	}
	modules := (uint32(length) - 8) / 20
	if modules > maxSunSpecMPPTModules {
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec Model 160 module count exceeds addressable geometry")
	}
	points := mpptFixedSunSpecPoints()
	for index := uint32(1); index <= modules; index++ {
		for _, point := range mpptModuleSunSpecPoints() {
			point.groupID = "module"
			point.repeatIndex = uint16(index)
			point.repeated = true
			points = append(points, point)
		}
	}
	definitions, err := appendSunSpecDefinition(nil, revision, 160, length, SunSpecTopologyNone, false, points)
	if err != nil {
		return sunSpecModelDefinition{}, err
	}
	definition := definitions[0]
	definition.geometry = func(words []uint16) bool {
		if len(words) != int(length)+2 || len(words) <= 8 || words[8] == 0xffff {
			return false
		}
		return uint32(length) == 8+20*uint32(words[8])
	}
	return definition, nil
}

func mpptFixedSunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		extendedSunSpecPoint("ID", "", SunSpecTypeUint16, 1, "", "", true), extendedSunSpecPoint("L", "", SunSpecTypeUint16, 1, "", "", true),
		extendedSunSpecPoint("DCA_SF", "mppt.scale.dc_current", SunSpecTypeScaleFactor, 1, "", "", false),
		extendedSunSpecPoint("DCV_SF", "mppt.scale.dc_voltage", SunSpecTypeScaleFactor, 1, "", "", false),
		extendedSunSpecPoint("DCW_SF", "mppt.scale.dc_power", SunSpecTypeScaleFactor, 1, "", "", false),
		extendedSunSpecPoint("DCWH_SF", "mppt.scale.dc_energy", SunSpecTypeScaleFactor, 1, "", "", false),
		extendedSunSpecBitfield("Evt", "mppt.events", SunSpecTypeBitfield32, false, mpptEventSymbols()),
		extendedSunSpecPoint("N", "mppt.module.count", SunSpecTypeCount, 1, "", "", false),
		extendedSunSpecPoint("TmsPer", "mppt.sample_period", SunSpecTypeUint16, 1, "", "", false),
	}
}

func mpptModuleSunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		extendedSunSpecPoint("ID", "mppt.module.id", SunSpecTypeUint16, 1, "", "", false),
		extendedSunSpecPoint("IDStr", "mppt.module.label", SunSpecTypeString, 8, "", "", false),
		extendedSunSpecPoint("DCA", "mppt.module.dc_current", SunSpecTypeUint16, 1, "A", "DCA_SF", false),
		extendedSunSpecPoint("DCV", "mppt.module.dc_voltage", SunSpecTypeUint16, 1, "V", "DCV_SF", false),
		extendedSunSpecPoint("DCW", "mppt.module.dc_power", SunSpecTypeUint16, 1, "W", "DCW_SF", false),
		extendedSunSpecPoint("DCWH", "mppt.module.dc_energy", SunSpecTypeAccumulator32, 2, "Wh", "DCWH_SF", false),
		extendedSunSpecPoint("Tms", "mppt.module.timestamp", SunSpecTypeUint32, 2, "Secs", "", false),
		extendedSunSpecPoint("Tmp", "mppt.module.temperature", SunSpecTypeInt16, 1, "C", "", false),
		extendedSunSpecEnum("DCSt", "mppt.module.operating_state", false, map[uint64]string{1: "OFF", 2: "SLEEPING", 3: "STARTING", 4: "MPPT", 5: "THROTTLED", 6: "SHUTTING_DOWN", 7: "FAULT", 8: "STANDBY", 9: "TEST", 10: "RESERVED_10"}),
		extendedSunSpecBitfield("DCEvt", "mppt.module.events", SunSpecTypeBitfield32, false, mpptEventSymbols()),
	}
}

func mpptEventSymbols() map[uint64]string {
	return map[uint64]string{
		0: "GROUND_FAULT", 1: "INPUT_OVER_VOLTAGE", 2: "RESERVED_2", 3: "DC_DISCONNECT", 4: "RESERVED_4", 5: "CABINET_OPEN", 6: "MANUAL_SHUTDOWN", 7: "OVER_TEMP",
		8: "RESERVED_8", 9: "RESERVED_9", 10: "RESERVED_10", 11: "RESERVED_11", 12: "BLOWN_FUSE", 13: "UNDER_TEMP", 14: "MEMORY_LOSS", 15: "ARC_DETECTION",
		16: "RESERVED_16", 17: "RESERVED_17", 18: "RESERVED_18", 19: "RESERVED_19", 20: "TEST_FAILED", 21: "INPUT_UNDER_VOLTAGE", 22: "INPUT_OVER_CURRENT",
	}
}
