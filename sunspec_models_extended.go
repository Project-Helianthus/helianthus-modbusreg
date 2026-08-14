package modbusreg

func nameplateSunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		extendedSunSpecPoint("ID", "", SunSpecTypeUint16, 1, "", "", true),
		extendedSunSpecPoint("L", "", SunSpecTypeUint16, 1, "", "", true),
		extendedSunSpecEnum("DERTyp", "der.type", true, map[uint64]string{4: "PV", 82: "PV_STOR"}),
		extendedSunSpecPoint("WRtg", "der.rating.active_power", SunSpecTypeUint16, 1, "W", "WRtg_SF", true),
		extendedSunSpecPoint("WRtg_SF", "der.scale.active_power_rating", SunSpecTypeScaleFactor, 1, "", "", true),
		extendedSunSpecPoint("VARtg", "der.rating.apparent_power", SunSpecTypeUint16, 1, "VA", "VARtg_SF", true),
		extendedSunSpecPoint("VARtg_SF", "der.scale.apparent_power_rating", SunSpecTypeScaleFactor, 1, "", "", true),
		extendedSunSpecPoint("VArRtgQ1", "der.rating.reactive_power.q1", SunSpecTypeInt16, 1, "var", "VArRtg_SF", true),
		extendedSunSpecPoint("VArRtgQ2", "der.rating.reactive_power.q2", SunSpecTypeInt16, 1, "var", "VArRtg_SF", true),
		extendedSunSpecPoint("VArRtgQ3", "der.rating.reactive_power.q3", SunSpecTypeInt16, 1, "var", "VArRtg_SF", true),
		extendedSunSpecPoint("VArRtgQ4", "der.rating.reactive_power.q4", SunSpecTypeInt16, 1, "var", "VArRtg_SF", true),
		extendedSunSpecPoint("VArRtg_SF", "der.scale.reactive_power_rating", SunSpecTypeScaleFactor, 1, "", "", true),
		extendedSunSpecPoint("ARtg", "der.rating.current", SunSpecTypeUint16, 1, "A", "ARtg_SF", true),
		extendedSunSpecPoint("ARtg_SF", "der.scale.current_rating", SunSpecTypeScaleFactor, 1, "", "", true),
		extendedSunSpecPoint("PFRtgQ1", "der.rating.power_factor.q1", SunSpecTypeInt16, 1, "cos()", "PFRtg_SF", true),
		extendedSunSpecPoint("PFRtgQ2", "der.rating.power_factor.q2", SunSpecTypeInt16, 1, "cos()", "PFRtg_SF", true),
		extendedSunSpecPoint("PFRtgQ3", "der.rating.power_factor.q3", SunSpecTypeInt16, 1, "cos()", "PFRtg_SF", true),
		extendedSunSpecPoint("PFRtgQ4", "der.rating.power_factor.q4", SunSpecTypeInt16, 1, "cos()", "PFRtg_SF", true),
		extendedSunSpecPoint("PFRtg_SF", "der.scale.power_factor_rating", SunSpecTypeScaleFactor, 1, "", "", true),
		extendedSunSpecPoint("WHRtg", "storage.rating.energy", SunSpecTypeUint16, 1, "Wh", "WHRtg_SF", false),
		extendedSunSpecPoint("WHRtg_SF", "storage.scale.energy_rating", SunSpecTypeScaleFactor, 1, "", "", false),
		extendedSunSpecPoint("AhrRtg", "storage.rating.charge", SunSpecTypeUint16, 1, "AH", "AhrRtg_SF", false),
		extendedSunSpecPoint("AhrRtg_SF", "storage.scale.charge_rating", SunSpecTypeScaleFactor, 1, "", "", false),
		extendedSunSpecPoint("MaxChaRte", "storage.rating.max_charge_power", SunSpecTypeUint16, 1, "W", "MaxChaRte_SF", false),
		extendedSunSpecPoint("MaxChaRte_SF", "storage.scale.max_charge_power", SunSpecTypeScaleFactor, 1, "", "", false),
		extendedSunSpecPoint("MaxDisChaRte", "storage.rating.max_discharge_power", SunSpecTypeUint16, 1, "W", "MaxDisChaRte_SF", false),
		extendedSunSpecPoint("MaxDisChaRte_SF", "storage.scale.max_discharge_power", SunSpecTypeScaleFactor, 1, "", "", false),
		extendedSunSpecPoint("Pad", "der.pad", SunSpecTypePad, 1, "", "", false),
	}
}

func settingsSunSpecPoints() []sunSpecPointDefinition {
	points := []sunSpecPointDefinition{
		extendedSunSpecPoint("ID", "", SunSpecTypeUint16, 1, "", "", true), extendedSunSpecPoint("L", "", SunSpecTypeUint16, 1, "", "", true),
		extendedSunSpecPoint("WMax", "der.settings.max_active_power", SunSpecTypeUint16, 1, "W", "WMax_SF", true),
		extendedSunSpecPoint("VRef", "der.settings.reference_voltage", SunSpecTypeUint16, 1, "V", "VRef_SF", true),
		extendedSunSpecPoint("VRefOfs", "der.settings.reference_voltage_offset", SunSpecTypeInt16, 1, "V", "VRefOfs_SF", true),
	}
	for _, point := range []struct {
		name, field string
		typ         SunSpecPointType
		unit, sf    string
	}{
		{"VMax", "der.settings.max_voltage", SunSpecTypeUint16, "V", "VMinMax_SF"}, {"VMin", "der.settings.min_voltage", SunSpecTypeUint16, "V", "VMinMax_SF"},
		{"VAMax", "der.settings.max_apparent_power", SunSpecTypeUint16, "VA", "VAMax_SF"},
		{"VArMaxQ1", "der.settings.max_reactive_power.q1", SunSpecTypeInt16, "var", "VArMax_SF"}, {"VArMaxQ2", "der.settings.max_reactive_power.q2", SunSpecTypeInt16, "var", "VArMax_SF"},
		{"VArMaxQ3", "der.settings.max_reactive_power.q3", SunSpecTypeInt16, "var", "VArMax_SF"}, {"VArMaxQ4", "der.settings.max_reactive_power.q4", SunSpecTypeInt16, "var", "VArMax_SF"},
		{"WGra", "der.settings.active_power_gradient", SunSpecTypeUint16, "% WMax/sec", "WGra_SF"},
		{"PFMinQ1", "der.settings.min_power_factor.q1", SunSpecTypeInt16, "cos()", "PFMin_SF"}, {"PFMinQ2", "der.settings.min_power_factor.q2", SunSpecTypeInt16, "cos()", "PFMin_SF"},
		{"PFMinQ3", "der.settings.min_power_factor.q3", SunSpecTypeInt16, "cos()", "PFMin_SF"}, {"PFMinQ4", "der.settings.min_power_factor.q4", SunSpecTypeInt16, "cos()", "PFMin_SF"},
	} {
		points = append(points, extendedSunSpecPoint(point.name, point.field, point.typ, 1, point.unit, point.sf, false))
	}
	points = append(points,
		extendedSunSpecEnum("VArAct", "der.settings.reactive_power_action", false, map[uint64]string{1: "SWITCH", 2: "MAINTAIN"}),
		extendedSunSpecEnum("ClcTotVA", "der.settings.apparent_power_calculation", false, map[uint64]string{1: "VECTOR", 2: "ARITHMETIC"}),
		extendedSunSpecPoint("MaxRmpRte", "der.settings.max_ramp_rate", SunSpecTypeUint16, 1, "% WGra", "MaxRmpRte_SF", false),
		extendedSunSpecPoint("ECPNomHz", "der.settings.nominal_frequency", SunSpecTypeUint16, 1, "Hz", "ECPNomHz_SF", false),
		extendedSunSpecEnum("ConnPh", "der.settings.connected_phase", false, map[uint64]string{1: "A", 2: "B", 3: "C"}),
	)
	for _, scale := range []struct {
		name, field string
		mandatory   bool
	}{
		{"WMax_SF", "der.scale.max_active_power", true}, {"VRef_SF", "der.scale.reference_voltage", true}, {"VRefOfs_SF", "der.scale.reference_voltage_offset", true},
		{"VMinMax_SF", "der.scale.voltage_bounds", false}, {"VAMax_SF", "der.scale.max_apparent_power", false}, {"VArMax_SF", "der.scale.max_reactive_power", false},
		{"WGra_SF", "der.scale.active_power_gradient", false}, {"PFMin_SF", "der.scale.min_power_factor", false}, {"MaxRmpRte_SF", "der.scale.max_ramp_rate", false}, {"ECPNomHz_SF", "der.scale.nominal_frequency", false},
	} {
		points = append(points, extendedSunSpecPoint(scale.name, scale.field, SunSpecTypeScaleFactor, 1, "", "", scale.mandatory))
	}
	return points
}

func statusSunSpecPoints() []sunSpecPointDefinition {
	connection := map[uint64]string{0: "CONNECTED", 1: "AVAILABLE", 2: "OPERATING", 3: "TEST"}
	return []sunSpecPointDefinition{
		extendedSunSpecPoint("ID", "", SunSpecTypeUint16, 1, "", "", true), extendedSunSpecPoint("L", "", SunSpecTypeUint16, 1, "", "", true),
		extendedSunSpecBitfield("PVConn", "der.status.pv_connection", SunSpecTypeBitfield16, true, connection),
		extendedSunSpecBitfield("StorConn", "der.status.storage_connection", SunSpecTypeBitfield16, true, connection),
		extendedSunSpecBitfield("ECPConn", "der.status.ecp_connection", SunSpecTypeBitfield16, true, map[uint64]string{0: "DISCONNECTED", 1: "CONNECTED"}),
		extendedSunSpecPoint("ActWh", "der.status.active_energy", SunSpecTypeAccumulator64, 4, "Wh", "", false),
		extendedSunSpecPoint("ActVAh", "der.status.apparent_energy", SunSpecTypeAccumulator64, 4, "VAh", "", false),
		extendedSunSpecPoint("ActVArhQ1", "der.status.reactive_energy.q1", SunSpecTypeAccumulator64, 4, "varh", "", false),
		extendedSunSpecPoint("ActVArhQ2", "der.status.reactive_energy.q2", SunSpecTypeAccumulator64, 4, "varh", "", false),
		extendedSunSpecPoint("ActVArhQ3", "der.status.reactive_energy.q3", SunSpecTypeAccumulator64, 4, "varh", "", false),
		extendedSunSpecPoint("ActVArhQ4", "der.status.reactive_energy.q4", SunSpecTypeAccumulator64, 4, "varh", "", false),
		extendedSunSpecPoint("VArAval", "der.status.available_reactive_power", SunSpecTypeInt16, 1, "var", "VArAval_SF", false),
		extendedSunSpecPoint("VArAval_SF", "der.scale.available_reactive_power", SunSpecTypeScaleFactor, 1, "", "", false),
		extendedSunSpecPoint("WAval", "der.status.available_active_power", SunSpecTypeUint16, 1, "var", "WAval_SF", false),
		extendedSunSpecPoint("WAval_SF", "der.scale.available_active_power", SunSpecTypeScaleFactor, 1, "", "", false),
		extendedSunSpecBitfield("StSetLimMsk", "der.status.limit_mask", SunSpecTypeBitfield32, false, numberedSymbols("WMax", "VAMax", "VArAval", "VArMaxQ1", "VArMaxQ2", "VArMaxQ3", "VArMaxQ4", "PFMinQ1", "PFMinQ2", "PFMinQ3", "PFMinQ4")),
		extendedSunSpecBitfield("StActCtl", "der.status.active_controls", SunSpecTypeBitfield32, false, map[uint64]string{0: "FixedW", 1: "FixedVAR", 2: "FixedPF", 3: "Volt-VAr", 4: "Freq-Watt-Param", 5: "Freq-Watt-Curve", 6: "Dyn-Reactive-Current", 7: "LVRT", 8: "HVRT", 9: "Watt-PF", 10: "Volt-Watt", 12: "Scheduled", 13: "LFRT", 14: "HFRT"}),
		extendedSunSpecPoint("TmSrc", "der.status.time_source", SunSpecTypeString, 4, "", "", false),
		extendedSunSpecPoint("Tms", "der.status.timestamp", SunSpecTypeUint32, 2, "Secs", "", false),
		extendedSunSpecBitfield("RtSt", "der.status.ride_through", SunSpecTypeBitfield16, false, numberedSymbols("LVRT_ACTIVE", "HVRT_ACTIVE", "LFRT_ACTIVE", "HFRT_ACTIVE")),
		extendedSunSpecPoint("Ris", "der.status.insulation_resistance", SunSpecTypeUint16, 1, "ohms", "Ris_SF", false),
		extendedSunSpecPoint("Ris_SF", "der.scale.insulation_resistance", SunSpecTypeScaleFactor, 1, "", "", false),
	}
}

func storageSunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		extendedSunSpecPoint("ID", "", SunSpecTypeUint16, 1, "", "", true), extendedSunSpecPoint("L", "", SunSpecTypeUint16, 1, "", "", true),
		extendedSunSpecPoint("WChaMax", "storage.control.max_charge_power", SunSpecTypeUint16, 1, "W", "WChaMax_SF", true),
		extendedSunSpecPoint("WChaGra", "storage.control.charge_gradient", SunSpecTypeUint16, 1, "% WChaMax/sec", "WChaDisChaGra_SF", true),
		extendedSunSpecPoint("WDisChaGra", "storage.control.discharge_gradient", SunSpecTypeUint16, 1, "% WChaMax/sec", "WChaDisChaGra_SF", true),
		extendedSunSpecBitfield("StorCtl_Mod", "storage.control.mode", SunSpecTypeBitfield16, true, map[uint64]string{0: "CHARGE", 1: "DiSCHARGE"}),
		extendedSunSpecPoint("VAChaMax", "storage.control.max_charge_apparent_power", SunSpecTypeUint16, 1, "VA", "VAChaMax_SF", false),
		extendedSunSpecPoint("MinRsvPct", "storage.control.minimum_reserve", SunSpecTypeUint16, 1, "% WChaMax", "MinRsvPct_SF", false),
		extendedSunSpecPoint("ChaState", "storage.status.state_of_charge", SunSpecTypeUint16, 1, "% AhrRtg", "ChaState_SF", false),
		extendedSunSpecPoint("StorAval", "storage.status.available_charge", SunSpecTypeUint16, 1, "AH", "StorAval_SF", false),
		extendedSunSpecPoint("InBatV", "storage.status.internal_battery_voltage", SunSpecTypeUint16, 1, "V", "InBatV_SF", false),
		extendedSunSpecEnum("ChaSt", "storage.status.charge_state", false, map[uint64]string{1: "OFF", 2: "EMPTY", 3: "DISCHARGING", 4: "CHARGING", 5: "FULL", 6: "HOLDING", 7: "TESTING"}),
		extendedSunSpecPoint("OutWRte", "storage.control.discharge_rate", SunSpecTypeInt16, 1, "% WDisChaMax", "InOutWRte_SF", false),
		extendedSunSpecPoint("InWRte", "storage.control.charge_rate", SunSpecTypeInt16, 1, " % WChaMax", "InOutWRte_SF", false),
		extendedSunSpecPoint("InOutWRte_WinTms", "storage.control.rate_window", SunSpecTypeUint16, 1, "Secs", "", false),
		extendedSunSpecPoint("InOutWRte_RvrtTms", "storage.control.rate_revert", SunSpecTypeUint16, 1, "Secs", "", false),
		extendedSunSpecPoint("InOutWRte_RmpTms", "storage.control.rate_ramp", SunSpecTypeUint16, 1, "Secs", "", false),
		extendedSunSpecEnum("ChaGriSet", "storage.control.charge_source", false, map[uint64]string{0: "PV", 1: "GRID"}),
		extendedSunSpecPoint("WChaMax_SF", "storage.scale.max_charge_power", SunSpecTypeScaleFactor, 1, "", "", true),
		extendedSunSpecPoint("WChaDisChaGra_SF", "storage.scale.charge_discharge_gradient", SunSpecTypeScaleFactor, 1, "", "", true),
		extendedSunSpecPoint("VAChaMax_SF", "storage.scale.max_charge_apparent_power", SunSpecTypeScaleFactor, 1, "", "", false),
		extendedSunSpecPoint("MinRsvPct_SF", "storage.scale.minimum_reserve", SunSpecTypeScaleFactor, 1, "", "", false),
		extendedSunSpecPoint("ChaState_SF", "storage.scale.state_of_charge", SunSpecTypeScaleFactor, 1, "", "", false),
		extendedSunSpecPoint("StorAval_SF", "storage.scale.available_charge", SunSpecTypeScaleFactor, 1, "", "", false),
		extendedSunSpecPoint("InBatV_SF", "storage.scale.internal_battery_voltage", SunSpecTypeScaleFactor, 1, "", "", false),
		extendedSunSpecPoint("InOutWRte_SF", "storage.scale.charge_discharge_rate", SunSpecTypeScaleFactor, 1, "", "", false),
	}
}

func extendedSunSpecPoint(name, field string, pointType SunSpecPointType, size uint16, unit, scale string, mandatory bool) sunSpecPointDefinition {
	return sunSpecPoint(name, field, pointType, size, unit, scale, mandatory, nil, 0)
}

func extendedSunSpecEnum(name, field string, mandatory bool, symbols map[uint64]string) sunSpecPointDefinition {
	return sunSpecPoint(name, field, SunSpecTypeEnum16, 1, "", "", mandatory, symbols, 0)
}

func extendedSunSpecBitfield(name, field string, pointType SunSpecPointType, mandatory bool, symbols map[uint64]string) sunSpecPointDefinition {
	return sunSpecPoint(name, field, pointType, sunSpecTypeWords(pointType), "", "", mandatory, symbols, sunSpecKnownMask(symbols))
}

func sunSpecKnownMask(symbols map[uint64]string) uint64 {
	var mask uint64
	for bit := range symbols {
		if bit < 63 {
			mask |= uint64(1) << bit
		}
	}
	return mask
}

func sunSpecTypeWords(pointType SunSpecPointType) uint16 {
	if pointType == SunSpecTypeBitfield32 {
		return 2
	}
	return 1
}

func numberedSymbols(values ...string) map[uint64]string {
	out := make(map[uint64]string, len(values))
	for index, value := range values {
		out[uint64(index)] = value
	}
	return out
}
