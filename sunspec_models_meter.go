package modbusreg

func integerMeterSunSpecPoints(modelID uint16) []sunSpecPointDefinition {
	points := meterHeaderSunSpecPoints()
	points = appendMeterPoints(points, []string{"A", "AphA", "AphB", "AphC"}, SunSpecTypeInt16, 1, "A", "A_SF")
	points = append(points, extendedSunSpecPoint("A_SF", meterFieldID("A_SF"), SunSpecTypeScaleFactor, 1, "", "", false))
	points = appendMeterPoints(points, []string{"PhV", "PhVphA", "PhVphB", "PhVphC"}, SunSpecTypeInt16, 1, "V", "V_SF")
	lineNames := []string{"PPV", "PhVphAB", "PhVphBC", "PhVphCA"}
	if modelID == 201 {
		lineNames = []string{"PPV", "PPVphAB", "PPVphBC", "PPVphCA"}
	}
	points = appendMeterPoints(points, lineNames, SunSpecTypeInt16, 1, "V", "V_SF")
	points = append(points, extendedSunSpecPoint("V_SF", meterFieldID("V_SF"), SunSpecTypeScaleFactor, 1, "", "", false))
	points = appendMeterPoints(points, []string{"Hz"}, SunSpecTypeInt16, 1, "Hz", "Hz_SF")
	points = append(points, extendedSunSpecPoint("Hz_SF", meterFieldID("Hz_SF"), SunSpecTypeScaleFactor, 1, "", "", false))
	for _, group := range []struct {
		names       []string
		unit, scale string
	}{
		{[]string{"W", "WphA", "WphB", "WphC"}, "W", "W_SF"},
		{[]string{"VA", "VAphA", "VAphB", "VAphC"}, "VA", "VA_SF"},
		{[]string{"VAR", "VARphA", "VARphB", "VARphC"}, "var", "VAR_SF"},
		{[]string{"PF", "PFphA", "PFphB", "PFphC"}, "Pct", "PF_SF"},
	} {
		points = appendMeterPoints(points, group.names, SunSpecTypeInt16, 1, group.unit, group.scale)
		points = append(points, extendedSunSpecPoint(group.scale, meterFieldID(group.scale), SunSpecTypeScaleFactor, 1, "", "", false))
	}
	for _, group := range []struct {
		names       []string
		unit, scale string
	}{
		{[]string{"TotWhExp", "TotWhExpPhA", "TotWhExpPhB", "TotWhExpPhC", "TotWhImp", "TotWhImpPhA", "TotWhImpPhB", "TotWhImpPhC"}, "Wh", "TotWh_SF"},
		{[]string{"TotVAhExp", "TotVAhExpPhA", "TotVAhExpPhB", "TotVAhExpPhC", "TotVAhImp", "TotVAhImpPhA", "TotVAhImpPhB", "TotVAhImpPhC"}, "VAh", "TotVAh_SF"},
		{[]string{"TotVArhImpQ1", "TotVArhImpQ1PhA", "TotVArhImpQ1PhB", "TotVArhImpQ1PhC", "TotVArhImpQ2", "TotVArhImpQ2PhA", "TotVArhImpQ2PhB", "TotVArhImpQ2PhC", "TotVArhExpQ3", "TotVArhExpQ3PhA", "TotVArhExpQ3PhB", "TotVArhExpQ3PhC", "TotVArhExpQ4", "TotVArhExpQ4PhA", "TotVArhExpQ4PhB", "TotVArhExpQ4PhC"}, "varh", "TotVArh_SF"},
	} {
		points = appendMeterPoints(points, group.names, SunSpecTypeAccumulator32, 2, group.unit, group.scale)
		points = append(points, extendedSunSpecPoint(group.scale, meterFieldID(group.scale), SunSpecTypeScaleFactor, 1, "", "", false))
	}
	points = append(points, extendedSunSpecBitfield("Evt", meterFieldID("Evt"), SunSpecTypeBitfield32, false, meterEventSymbols(modelID)))
	return markMeterMandatory(points, modelID)
}

func floatMeterSunSpecPoints(modelID uint16) []sunSpecPointDefinition {
	points := meterHeaderSunSpecPoints()
	groups := []struct {
		names []string
		unit  string
	}{
		{[]string{"A", "AphA", "AphB", "AphC"}, "A"},
		{[]string{"PhV", "PhVphA", "PhVphB", "PhVphC", "PPV", "PPVphAB", "PPVphBC", "PPVphCA"}, "V"},
		{[]string{"Hz"}, "Hz"}, {[]string{"W", "WphA", "WphB", "WphC"}, "W"},
		{[]string{"VA", "VAphA", "VAphB", "VAphC"}, "VA"},
		{[]string{"VAR", "VARphA", "VARphB", "VARphC"}, "var"},
		{[]string{"PF", "PFphA", "PFphB", "PFphC"}, "PF"},
		{[]string{"TotWhExp", "TotWhExpPhA", "TotWhExpPhB", "TotWhExpPhC", "TotWhImp", "TotWhImpPhA", "TotWhImpPhB", "TotWhImpPhC"}, "Wh"},
		{[]string{"TotVAhExp", "TotVAhExpPhA", "TotVAhExpPhB", "TotVAhExpPhC", "TotVAhImp", "TotVAhImpPhA", "TotVAhImpPhB", "TotVAhImpPhC"}, "VAh"},
		{[]string{"TotVArhImpQ1", "TotVArhImpQ1phA", "TotVArhImpQ1phB", "TotVArhImpQ1phC", "TotVArhImpQ2", "TotVArhImpQ2phA", "TotVArhImpQ2phB", "TotVArhImpQ2phC", "TotVArhExpQ3", "TotVArhExpQ3phA", "TotVArhExpQ3phB", "TotVArhExpQ3phC", "TotVArhExpQ4", "TotVArhExpQ4phA", "TotVArhExpQ4phB", "TotVArhExpQ4phC"}, "varh"},
	}
	for _, group := range groups {
		points = appendMeterPoints(points, group.names, SunSpecTypeFloat32, 2, group.unit, "")
	}
	points = append(points, extendedSunSpecBitfield("Evt", meterFieldID("Evt"), SunSpecTypeBitfield32, false, meterEventSymbols(modelID)))
	return markMeterMandatory(points, modelID)
}

func meterHeaderSunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		extendedSunSpecPoint("ID", "", SunSpecTypeUint16, 1, "", "", true),
		extendedSunSpecPoint("L", "", SunSpecTypeUint16, 1, "", "", true),
	}
}

func appendMeterPoints(points []sunSpecPointDefinition, names []string, pointType SunSpecPointType, size uint16, unit, scale string) []sunSpecPointDefinition {
	for _, name := range names {
		points = append(points, extendedSunSpecPoint(name, meterFieldID(name), pointType, size, unit, scale, false))
	}
	return points
}

func markMeterMandatory(points []sunSpecPointDefinition, modelID uint16) []sunSpecPointDefinition {
	mandatory := meterMandatoryPoints(modelID)
	for index := range points {
		points[index].mandatory = mandatory[points[index].name]
		points[index].required = points[index].mandatory && points[index].name != "ID" && points[index].name != "L" && points[index].pointType != SunSpecTypeScaleFactor
	}
	return points
}

func meterMandatoryPoints(modelID uint16) map[string]bool {
	sets := map[uint16][]string{
		201: {"ID", "L", "A", "AphA", "A_SF", "V_SF", "Hz", "W", "W_SF", "TotWhExp", "TotWhImp", "TotWh_SF", "Evt"},
		202: {"ID", "L", "A", "AphB", "AphC", "A_SF", "PhV", "PhVphA", "PhVphB", "PPV", "PhVphAB", "V_SF", "Hz", "W", "W_SF", "TotWhExp", "TotWhImp", "TotWh_SF", "Evt"},
		203: {"ID", "L", "A", "AphA", "AphB", "AphC", "A_SF", "PhV", "PhVphA", "PhVphB", "PhVphC", "PPV", "PhVphAB", "PhVphBC", "PhVphCA", "V_SF", "Hz", "W", "W_SF", "TotWhExp", "TotWhImp", "TotWh_SF", "Evt"},
		204: {"ID", "L", "A", "AphA", "AphB", "AphC", "A_SF", "PPV", "PhVphAB", "PhVphBC", "PhVphCA", "V_SF", "Hz", "W", "W_SF", "TotWhExp", "TotWhImp", "TotWh_SF", "Evt"},
		211: {"ID", "L", "A", "AphA", "Hz", "W", "TotWhExp", "TotWhImp", "Evt"},
		212: {"ID", "L", "A", "AphA", "AphB", "PhV", "PhVphA", "PhVphB", "PPV", "PPVphAB", "Hz", "W", "TotWhExp", "TotWhImp", "Evt"},
		213: {"ID", "L", "A", "AphA", "AphB", "AphC", "PhV", "PhVphA", "PhVphB", "PhVphC", "PPV", "PPVphAB", "PPVphBC", "PPVphCA", "Hz", "W", "TotWhExp", "TotWhImp", "Evt"},
		214: {"ID", "L", "A", "AphA", "AphB", "AphC", "PPV", "PPVphAB", "PPVphBC", "PPVphCA", "Hz", "W", "TotWhExp", "TotWhImp", "Evt"},
	}
	out := make(map[string]bool, len(sets[modelID]))
	for _, name := range sets[modelID] {
		out[name] = true
	}
	return out
}

func meterEventSymbols(modelID uint16) map[uint64]string {
	prefix := "M_EVENT_"
	if modelID == 201 {
		prefix = ""
	}
	out := map[uint64]string{
		2: prefix + "Power_Failure", 3: prefix + "Under_Voltage", 4: prefix + "Low_PF",
		5: prefix + "Over_Current", 6: prefix + "Over_Voltage", 7: prefix + "Missing_Sensor",
	}
	if modelID != 201 {
		for bit := uint64(8); bit <= 15; bit++ {
			out[bit] = "M_EVENT_Reserved" + string(rune('0'+bit-7))
		}
	}
	for bit := uint64(16); bit <= 30; bit++ {
		out[bit] = prefix + "OEM" + twoDigitMeterSymbol(bit-15)
	}
	return out
}

func twoDigitMeterSymbol(value uint64) string {
	if value < 10 {
		return "0" + string(rune('0'+value))
	}
	return string([]byte{byte('0' + value/10), byte('0' + value%10)})
}

func meterFieldID(name string) string {
	fields := map[string]string{
		"A": "meter.ac.current.total", "AphA": "meter.ac.current.phase_a", "AphB": "meter.ac.current.phase_b", "AphC": "meter.ac.current.phase_c", "A_SF": "meter.scale.current",
		"PhV": "meter.ac.voltage.line_neutral.average", "PhVphA": "meter.ac.voltage.line_neutral.phase_a", "PhVphB": "meter.ac.voltage.line_neutral.phase_b", "PhVphC": "meter.ac.voltage.line_neutral.phase_c",
		"PPV": "meter.ac.voltage.line_line.average", "PPVphAB": "meter.ac.voltage.line_line.ab", "PPVphBC": "meter.ac.voltage.line_line.bc", "PPVphCA": "meter.ac.voltage.line_line.ca",
		"PhVphAB": "meter.ac.voltage.line_line.ab", "PhVphBC": "meter.ac.voltage.line_line.bc", "PhVphCA": "meter.ac.voltage.line_line.ca", "V_SF": "meter.scale.voltage",
		"Hz": "meter.ac.frequency", "Hz_SF": "meter.scale.frequency", "W": "meter.ac.power.active.total", "WphA": "meter.ac.power.active.phase_a", "WphB": "meter.ac.power.active.phase_b", "WphC": "meter.ac.power.active.phase_c", "W_SF": "meter.scale.active_power",
		"VA": "meter.ac.power.apparent.total", "VAphA": "meter.ac.power.apparent.phase_a", "VAphB": "meter.ac.power.apparent.phase_b", "VAphC": "meter.ac.power.apparent.phase_c", "VA_SF": "meter.scale.apparent_power",
		"VAR": "meter.ac.power.reactive.total", "VARphA": "meter.ac.power.reactive.phase_a", "VARphB": "meter.ac.power.reactive.phase_b", "VARphC": "meter.ac.power.reactive.phase_c", "VAR_SF": "meter.scale.reactive_power",
		"PF": "meter.ac.power_factor.total", "PFphA": "meter.ac.power_factor.phase_a", "PFphB": "meter.ac.power_factor.phase_b", "PFphC": "meter.ac.power_factor.phase_c", "PF_SF": "meter.scale.power_factor",
		"TotWhExp": "meter.energy.export.total", "TotWhImp": "meter.energy.import.total", "TotWh_SF": "meter.scale.active_energy", "TotVAhExp": "meter.energy.apparent_export.total", "TotVAhImp": "meter.energy.apparent_import.total", "TotVAh_SF": "meter.scale.apparent_energy",
		"TotVArhImpQ1": "meter.energy.reactive_import.q1.total", "TotVArhImpQ2": "meter.energy.reactive_import.q2.total", "TotVArhExpQ3": "meter.energy.reactive_export.q3.total", "TotVArhExpQ4": "meter.energy.reactive_export.q4.total", "TotVArh_SF": "meter.scale.reactive_energy", "Evt": "meter.events",
	}
	if field := fields[name]; field != "" {
		return field
	}
	phaseFields := map[string]string{
		"TotWhExpPhA": "meter.energy.export.phase_a", "TotWhExpPhB": "meter.energy.export.phase_b", "TotWhExpPhC": "meter.energy.export.phase_c",
		"TotWhImpPhA": "meter.energy.import.phase_a", "TotWhImpPhB": "meter.energy.import.phase_b", "TotWhImpPhC": "meter.energy.import.phase_c",
		"TotVAhExpPhA": "meter.energy.apparent_export.phase_a", "TotVAhExpPhB": "meter.energy.apparent_export.phase_b", "TotVAhExpPhC": "meter.energy.apparent_export.phase_c",
		"TotVAhImpPhA": "meter.energy.apparent_import.phase_a", "TotVAhImpPhB": "meter.energy.apparent_import.phase_b", "TotVAhImpPhC": "meter.energy.apparent_import.phase_c",
		"TotVArhImpQ1PhA": "meter.energy.reactive_import.q1.phase_a", "TotVArhImpQ1PhB": "meter.energy.reactive_import.q1.phase_b", "TotVArhImpQ1PhC": "meter.energy.reactive_import.q1.phase_c",
		"TotVArhImpQ2PhA": "meter.energy.reactive_import.q2.phase_a", "TotVArhImpQ2PhB": "meter.energy.reactive_import.q2.phase_b", "TotVArhImpQ2PhC": "meter.energy.reactive_import.q2.phase_c",
		"TotVArhExpQ3PhA": "meter.energy.reactive_export.q3.phase_a", "TotVArhExpQ3PhB": "meter.energy.reactive_export.q3.phase_b", "TotVArhExpQ3PhC": "meter.energy.reactive_export.q3.phase_c",
		"TotVArhExpQ4PhA": "meter.energy.reactive_export.q4.phase_a", "TotVArhExpQ4PhB": "meter.energy.reactive_export.q4.phase_b", "TotVArhExpQ4PhC": "meter.energy.reactive_export.q4.phase_c",
		"TotVArhImpQ1phA": "meter.energy.reactive_import.q1.phase_a", "TotVArhImpQ1phB": "meter.energy.reactive_import.q1.phase_b", "TotVArhImpQ1phC": "meter.energy.reactive_import.q1.phase_c",
		"TotVArhImpQ2phA": "meter.energy.reactive_import.q2.phase_a", "TotVArhImpQ2phB": "meter.energy.reactive_import.q2.phase_b", "TotVArhImpQ2phC": "meter.energy.reactive_import.q2.phase_c",
		"TotVArhExpQ3phA": "meter.energy.reactive_export.q3.phase_a", "TotVArhExpQ3phB": "meter.energy.reactive_export.q3.phase_b", "TotVArhExpQ3phC": "meter.energy.reactive_export.q3.phase_c",
		"TotVArhExpQ4phA": "meter.energy.reactive_export.q4.phase_a", "TotVArhExpQ4phB": "meter.energy.reactive_export.q4.phase_b", "TotVArhExpQ4phC": "meter.energy.reactive_export.q4.phase_c",
	}
	if field := phaseFields[name]; field != "" {
		return field
	}
	return "meter.point." + name
}
