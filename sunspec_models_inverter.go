package modbusreg

func integerInverterSunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		inverterSunSpecPoint("ID", "", SunSpecTypeUint16, 1, "", ""), inverterSunSpecPoint("L", "", SunSpecTypeUint16, 1, "", ""),
		inverterSunSpecPoint("A", "inverter.ac.current.total", SunSpecTypeUint16, 1, "A", "A_SF"),
		inverterSunSpecPoint("AphA", "inverter.ac.current.phase_a", SunSpecTypeUint16, 1, "A", "A_SF"),
		inverterSunSpecPoint("AphB", "inverter.ac.current.phase_b", SunSpecTypeUint16, 1, "A", "A_SF"),
		inverterSunSpecPoint("AphC", "inverter.ac.current.phase_c", SunSpecTypeUint16, 1, "A", "A_SF"),
		inverterSunSpecPoint("A_SF", "inverter.scale.current", SunSpecTypeScaleFactor, 1, "", ""),
		inverterSunSpecPoint("PPVphAB", "inverter.ac.voltage.line_ab", SunSpecTypeUint16, 1, "V", "V_SF"),
		inverterSunSpecPoint("PPVphBC", "inverter.ac.voltage.line_bc", SunSpecTypeUint16, 1, "V", "V_SF"),
		inverterSunSpecPoint("PPVphCA", "inverter.ac.voltage.line_ca", SunSpecTypeUint16, 1, "V", "V_SF"),
		inverterSunSpecPoint("PhVphA", "inverter.ac.voltage.phase_a", SunSpecTypeUint16, 1, "V", "V_SF"),
		inverterSunSpecPoint("PhVphB", "inverter.ac.voltage.phase_b", SunSpecTypeUint16, 1, "V", "V_SF"),
		inverterSunSpecPoint("PhVphC", "inverter.ac.voltage.phase_c", SunSpecTypeUint16, 1, "V", "V_SF"),
		inverterSunSpecPoint("V_SF", "inverter.scale.voltage", SunSpecTypeScaleFactor, 1, "", ""),
		inverterSunSpecPoint("W", "inverter.ac.power.active", SunSpecTypeInt16, 1, "W", "W_SF"),
		inverterSunSpecPoint("W_SF", "inverter.scale.active_power", SunSpecTypeScaleFactor, 1, "", ""),
		inverterSunSpecPoint("Hz", "inverter.ac.frequency", SunSpecTypeUint16, 1, "Hz", "Hz_SF"),
		inverterSunSpecPoint("Hz_SF", "inverter.scale.frequency", SunSpecTypeScaleFactor, 1, "", ""),
		inverterSunSpecPoint("VA", "inverter.ac.power.apparent", SunSpecTypeInt16, 1, "VA", "VA_SF"),
		inverterSunSpecPoint("VA_SF", "inverter.scale.apparent_power", SunSpecTypeScaleFactor, 1, "", ""),
		inverterSunSpecPoint("VAr", "inverter.ac.power.reactive", SunSpecTypeInt16, 1, "var", "VAr_SF"),
		inverterSunSpecPoint("VAr_SF", "inverter.scale.reactive_power", SunSpecTypeScaleFactor, 1, "", ""),
		inverterSunSpecPoint("PF", "inverter.ac.power_factor", SunSpecTypeInt16, 1, "Pct", "PF_SF"),
		inverterSunSpecPoint("PF_SF", "inverter.scale.power_factor", SunSpecTypeScaleFactor, 1, "", ""),
		inverterSunSpecPoint("WH", "inverter.ac.energy_lifetime", SunSpecTypeAccumulator32, 2, "Wh", "WH_SF"),
		inverterSunSpecPoint("WH_SF", "inverter.scale.energy", SunSpecTypeScaleFactor, 1, "", ""),
		inverterSunSpecPoint("DCA", "inverter.dc.current", SunSpecTypeUint16, 1, "A", "DCA_SF"),
		inverterSunSpecPoint("DCA_SF", "inverter.scale.dc_current", SunSpecTypeScaleFactor, 1, "", ""),
		inverterSunSpecPoint("DCV", "inverter.dc.voltage", SunSpecTypeUint16, 1, "V", "DCV_SF"),
		inverterSunSpecPoint("DCV_SF", "inverter.scale.dc_voltage", SunSpecTypeScaleFactor, 1, "", ""),
		inverterSunSpecPoint("DCW", "inverter.dc.power", SunSpecTypeInt16, 1, "W", "DCW_SF"),
		inverterSunSpecPoint("DCW_SF", "inverter.scale.dc_power", SunSpecTypeScaleFactor, 1, "", ""),
		inverterSunSpecPoint("TmpCab", "inverter.temperature.cabinet", SunSpecTypeInt16, 1, "C", "Tmp_SF"),
		inverterSunSpecPoint("TmpSnk", "inverter.temperature.heatsink", SunSpecTypeInt16, 1, "C", "Tmp_SF"),
		inverterSunSpecPoint("TmpTrns", "inverter.temperature.transformer", SunSpecTypeInt16, 1, "C", "Tmp_SF"),
		inverterSunSpecPoint("TmpOt", "inverter.temperature.other", SunSpecTypeInt16, 1, "C", "Tmp_SF"),
		inverterSunSpecPoint("Tmp_SF", "inverter.scale.temperature", SunSpecTypeScaleFactor, 1, "", ""),
		inverterSunSpecEnumPoint("St", "inverter.operating_state", inverterStatusSymbols()),
		inverterSunSpecEnumPoint("StVnd", "inverter.vendor_state", nil),
		inverterSunSpecBitfieldPoint("Evt1", "inverter.events.1", inverterEvent1Symbols()),
		inverterSunSpecBitfieldPoint("Evt2", "inverter.events.2", nil),
		inverterSunSpecBitfieldPoint("EvtVnd1", "inverter.vendor_events.1", nil),
		inverterSunSpecBitfieldPoint("EvtVnd2", "inverter.vendor_events.2", nil),
		inverterSunSpecBitfieldPoint("EvtVnd3", "inverter.vendor_events.3", nil),
		inverterSunSpecBitfieldPoint("EvtVnd4", "inverter.vendor_events.4", nil),
	}
}

func floatInverterSunSpecPoints(statusSymbols map[uint64]string) []sunSpecPointDefinition {
	points := []sunSpecPointDefinition{
		inverterSunSpecPoint("ID", "", SunSpecTypeUint16, 1, "", ""), inverterSunSpecPoint("L", "", SunSpecTypeUint16, 1, "", ""),
	}
	for _, point := range []struct{ name, field, unit string }{
		{"A", "inverter.ac.current.total", "A"}, {"AphA", "inverter.ac.current.phase_a", "A"}, {"AphB", "inverter.ac.current.phase_b", "A"}, {"AphC", "inverter.ac.current.phase_c", "A"},
		{"PPVphAB", "inverter.ac.voltage.line_ab", "V"}, {"PPVphBC", "inverter.ac.voltage.line_bc", "V"}, {"PPVphCA", "inverter.ac.voltage.line_ca", "V"},
		{"PhVphA", "inverter.ac.voltage.phase_a", "V"}, {"PhVphB", "inverter.ac.voltage.phase_b", "V"}, {"PhVphC", "inverter.ac.voltage.phase_c", "V"},
		{"W", "inverter.ac.power.active", "W"}, {"Hz", "inverter.ac.frequency", "Hz"}, {"VA", "inverter.ac.power.apparent", "VA"}, {"VAr", "inverter.ac.power.reactive", "var"}, {"PF", "inverter.ac.power_factor", "Pct"},
		{"WH", "inverter.ac.energy_lifetime", "Wh"}, {"DCA", "inverter.dc.current", "A"}, {"DCV", "inverter.dc.voltage", "V"}, {"DCW", "inverter.dc.power", "W"},
		{"TmpCab", "inverter.temperature.cabinet", "C"}, {"TmpSnk", "inverter.temperature.heatsink", "C"}, {"TmpTrns", "inverter.temperature.transformer", "C"}, {"TmpOt", "inverter.temperature.other", "C"},
	} {
		points = append(points, inverterSunSpecPoint(point.name, point.field, SunSpecTypeFloat32, 2, point.unit, ""))
	}
	return append(points,
		inverterSunSpecEnumPoint("St", "inverter.operating_state", statusSymbols), inverterSunSpecEnumPoint("StVnd", "inverter.vendor_state", nil),
		inverterSunSpecBitfieldPoint("Evt1", "inverter.events.1", inverterEvent1Symbols()), inverterSunSpecBitfieldPoint("Evt2", "inverter.events.2", nil),
		inverterSunSpecBitfieldPoint("EvtVnd1", "inverter.vendor_events.1", nil), inverterSunSpecBitfieldPoint("EvtVnd2", "inverter.vendor_events.2", nil),
		inverterSunSpecBitfieldPoint("EvtVnd3", "inverter.vendor_events.3", nil), inverterSunSpecBitfieldPoint("EvtVnd4", "inverter.vendor_events.4", nil),
	)
}

func inverterSunSpecPoint(name, field string, pointType SunSpecPointType, size uint16, unit, scale string) sunSpecPointDefinition {
	return sunSpecPoint(name, field, pointType, size, unit, scale, false, nil, 0)
}

func inverterSunSpecEnumPoint(name, field string, symbols map[uint64]string) sunSpecPointDefinition {
	return sunSpecPoint(name, field, SunSpecTypeEnum16, 1, "", "", false, symbols, 0)
}

func inverterSunSpecBitfieldPoint(name, field string, symbols map[uint64]string) sunSpecPointDefinition {
	var knownMask uint64
	for bit := range symbols {
		if bit < 31 {
			knownMask |= uint64(1) << bit
		}
	}
	return sunSpecPoint(name, field, SunSpecTypeBitfield32, 2, "", "", false, symbols, knownMask)
}

func inverterStatusSymbols() map[uint64]string {
	return map[uint64]string{1: "OFF", 2: "SLEEPING", 3: "STARTING", 4: "MPPT", 5: "THROTTLED", 6: "SHUTTING_DOWN", 7: "FAULT", 8: "STANDBY"}
}

func inverterModel111StatusSymbols() map[uint64]string {
	return map[uint64]string{1: "ggOFF", 2: "ggSLEEPING", 3: "ggSTARTING", 4: "ggMPPT", 5: "ggTHROTTLED", 6: "ggSHUTTING_DOWN", 7: "ggFAULT", 8: "ggSTANDBY"}
}

func inverterEvent1Symbols() map[uint64]string {
	return map[uint64]string{
		0: "GROUND_FAULT", 1: "DC_OVER_VOLT", 2: "AC_DISCONNECT", 3: "DC_DISCONNECT",
		4: "GRID_DISCONNECT", 5: "CABINET_OPEN", 6: "MANUAL_SHUTDOWN", 7: "OVER_TEMP",
		8: "OVER_FREQUENCY", 9: "UNDER_FREQUENCY", 10: "AC_OVER_VOLT", 11: "AC_UNDER_VOLT",
		12: "BLOWN_STRING_FUSE", 13: "UNDER_TEMP", 14: "MEMORY_LOSS", 15: "HW_TEST_FAILURE",
	}
}

func sunSpecPointMandatory(modelID uint16, name string) bool {
	mandatory := map[uint16]map[string]struct{}{
		101: sunSpecNameSet("ID", "L", "A", "AphA", "A_SF", "PhVphA", "V_SF", "W", "W_SF", "Hz", "Hz_SF", "WH", "WH_SF", "TmpCab", "Tmp_SF", "St", "Evt1", "Evt2"),
		102: sunSpecNameSet("ID", "L", "A", "AphA", "AphB", "A_SF", "PhVphA", "PhVphB", "V_SF", "W", "W_SF", "Hz", "Hz_SF", "WH", "WH_SF", "TmpCab", "Tmp_SF", "St", "Evt1", "Evt2"),
		103: sunSpecNameSet("ID", "L", "A", "AphA", "AphB", "AphC", "A_SF", "PhVphA", "PhVphB", "PhVphC", "V_SF", "W", "W_SF", "Hz", "Hz_SF", "WH", "WH_SF", "TmpCab", "Tmp_SF", "St", "Evt1", "Evt2"),
		111: sunSpecNameSet("ID", "L", "A", "AphA", "PhVphA", "W", "Hz", "WH", "TmpCab", "St", "Evt1", "Evt2"),
		112: sunSpecNameSet("ID", "L", "A", "AphA", "AphB", "PhVphA", "PhVphB", "W", "Hz", "WH", "TmpCab", "St", "Evt1", "Evt2"),
		113: sunSpecNameSet("ID", "L", "A", "AphA", "AphB", "AphC", "PhVphA", "PhVphB", "PhVphC", "W", "Hz", "WH", "TmpCab", "St", "Evt1", "Evt2"),
	}
	_, ok := mandatory[modelID][name]
	return ok
}

func sunSpecNameSet(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}
