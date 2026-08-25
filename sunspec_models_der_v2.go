package modbusreg

func derMeasureACV2SunSpecPoints() []sunSpecPointDefinition {
	points := []sunSpecPointDefinition{
		v2DERPoint("701", "ID", SunSpecTypeUint16, "", "", true),
		v2DERPoint("701", "L", SunSpecTypeUint16, "", "", true),
		v2DERPoint("701", "ACType", SunSpecTypeEnum16, "", "", true),
		v2DERPoint("701", "St", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("701", "InvSt", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("701", "ConnSt", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("701", "Alrm", SunSpecTypeBitfield32, "", "", false),
		v2DERPoint("701", "DERMode", SunSpecTypeBitfield32, "", "", false),
	}
	for _, point := range []struct {
		name string
		typ  SunSpecPointType
		unit string
		sf   string
	}{
		{"W", SunSpecTypeInt16, "W", "W_SF"}, {"VA", SunSpecTypeInt16, "VA", "VA_SF"},
		{"Var", SunSpecTypeInt16, "Var", "Var_SF"}, {"PF", SunSpecTypeInt16, "", "PF_SF"},
		{"A", SunSpecTypeInt16, "A", "A_SF"}, {"LLV", SunSpecTypeUint16, "V", "V_SF"},
		{"LNV", SunSpecTypeUint16, "V", "V_SF"}, {"Hz", SunSpecTypeUint32, "Hz", "Hz_SF"},
		{"TotWhInj", SunSpecTypeUint64, "Wh", "TotWh_SF"}, {"TotWhAbs", SunSpecTypeUint64, "Wh", "TotWh_SF"},
		{"TotVarhInj", SunSpecTypeUint64, "Varh", "TotVarh_SF"}, {"TotVarhAbs", SunSpecTypeUint64, "Varh", "TotVarh_SF"},
		{"TmpAmb", SunSpecTypeInt16, "C", "Tmp_SF"}, {"TmpCab", SunSpecTypeInt16, "C", "Tmp_SF"},
		{"TmpSnk", SunSpecTypeInt16, "C", "Tmp_SF"}, {"TmpTrns", SunSpecTypeInt16, "C", "Tmp_SF"},
		{"TmpSw", SunSpecTypeInt16, "C", "Tmp_SF"}, {"TmpOt", SunSpecTypeInt16, "C", "Tmp_SF"},
	} {
		points = append(points, v2DERPoint("701", point.name, point.typ, point.unit, point.sf, false))
	}
	for _, phase := range []struct {
		suffix, line, voltage string
	}{
		{"L1", "VL1L2", "VL1"}, {"L2", "VL2L3", "VL2"}, {"L3", "VL3L1", "VL3"},
	} {
		for _, point := range []struct {
			name string
			typ  SunSpecPointType
			unit string
			sf   string
		}{
			{"W" + phase.suffix, SunSpecTypeInt16, "W", "W_SF"}, {"VA" + phase.suffix, SunSpecTypeInt16, "VA", "VA_SF"},
			{"Var" + phase.suffix, SunSpecTypeInt16, "Var", "Var_SF"}, {"PF" + phase.suffix, SunSpecTypeInt16, "", "PF_SF"},
			{"A" + phase.suffix, SunSpecTypeInt16, "A", "A_SF"}, {phase.line, SunSpecTypeUint16, "V", "V_SF"},
			{phase.voltage, SunSpecTypeUint16, "V", "V_SF"}, {"TotWhInj" + phase.suffix, SunSpecTypeUint64, "Wh", "TotWh_SF"},
			{"TotWhAbs" + phase.suffix, SunSpecTypeUint64, "Wh", "TotWh_SF"}, {"TotVarhInj" + phase.suffix, SunSpecTypeUint64, "Varh", "TotVarh_SF"},
			{"TotVarhAbs" + phase.suffix, SunSpecTypeUint64, "Varh", "TotVarh_SF"},
		} {
			points = append(points, v2DERPoint("701", point.name, point.typ, point.unit, point.sf, false))
		}
	}
	for _, point := range []struct {
		name string
		typ  SunSpecPointType
	}{
		{"ThrotPct", SunSpecTypeUint16}, {"ThrotSrc", SunSpecTypeBitfield32},
		{"A_SF", SunSpecTypeScaleFactor}, {"V_SF", SunSpecTypeScaleFactor}, {"Hz_SF", SunSpecTypeScaleFactor},
		{"W_SF", SunSpecTypeScaleFactor}, {"PF_SF", SunSpecTypeScaleFactor}, {"VA_SF", SunSpecTypeScaleFactor},
		{"Var_SF", SunSpecTypeScaleFactor}, {"TotWh_SF", SunSpecTypeScaleFactor}, {"TotVarh_SF", SunSpecTypeScaleFactor},
		{"Tmp_SF", SunSpecTypeScaleFactor}, {"MnAlrmInfo", SunSpecTypeString},
	} {
		points = append(points, v2DERPoint("701", point.name, point.typ, "", "", false))
	}
	return points
}

func derCapacityV2SunSpecPoints() []sunSpecPointDefinition {
	points := []sunSpecPointDefinition{
		v2DERPoint("702", "ID", SunSpecTypeUint16, "", "", true),
		v2DERPoint("702", "L", SunSpecTypeUint16, "", "", true),
	}
	for _, point := range []struct {
		name, unit, sf string
	}{
		{"WMaxRtg", "W", "W_SF"}, {"WOvrExtRtg", "W", "W_SF"}, {"WOvrExtRtgPF", "", "PF_SF"},
		{"WUndExtRtg", "W", "W_SF"}, {"WUndExtRtgPF", "", "PF_SF"}, {"VAMaxRtg", "VA", "VA_SF"},
		{"VarMaxInjRtg", "Var", "Var_SF"}, {"VarMaxAbsRtg", "Var", "Var_SF"}, {"WChaRteMaxRtg", "W", "W_SF"},
		{"WDisChaRteMaxRtg", "W", "W_SF"}, {"VAChaRteMaxRtg", "VA", "VA_SF"}, {"VADisChaRteMaxRtg", "VA", "VA_SF"},
		{"VNomRtg", "V", "V_SF"}, {"VMaxRtg", "V", "V_SF"}, {"VMinRtg", "V", "V_SF"}, {"AMaxRtg", "A", "A_SF"},
		{"PFOvrExtRtg", "", "PF_SF"}, {"PFUndExtRtg", "", "PF_SF"}, {"ReactSusceptRtg", "S", "S_SF"},
	} {
		points = append(points, v2DERPoint("702", point.name, SunSpecTypeUint16, point.unit, point.sf, false))
	}
	for _, point := range []struct {
		name string
		typ  SunSpecPointType
	}{
		{"NorOpCatRtg", SunSpecTypeEnum16}, {"AbnOpCatRtg", SunSpecTypeEnum16}, {"CtrlModes", SunSpecTypeBitfield32},
		{"IntIslandCatRtg", SunSpecTypeBitfield16},
	} {
		points = append(points, v2DERPoint("702", point.name, point.typ, "", "", false))
	}
	for _, point := range []struct {
		name, unit, sf string
	}{
		{"WMax", "W", "W_SF"}, {"WMaxOvrExt", "W", "W_SF"}, {"WOvrExtPF", "", "PF_SF"},
		{"WMaxUndExt", "W", "W_SF"}, {"WUndExtPF", "", "PF_SF"}, {"VAMax", "VA", "VA_SF"},
		{"VarMaxInj", "Var", "Var_SF"}, {"VarMaxAbs", "Var", "Var_SF"}, {"WChaRteMax", "W", "W_SF"},
		{"WDisChaRteMax", "W", "W_SF"}, {"VAChaRteMax", "VA", "VA_SF"}, {"VADisChaRteMax", "VA", "VA_SF"},
		{"VNom", "V", "V_SF"}, {"VMax", "V", "V_SF"}, {"VMin", "V", "V_SF"}, {"AMax", "A", "A_SF"},
		{"PFOvrExt", "", "PF_SF"}, {"PFUndExt", "", "PF_SF"},
	} {
		points = append(points, v2DERPoint("702", point.name, SunSpecTypeUint16, point.unit, point.sf, false))
	}
	points = append(points, v2DERPoint("702", "IntIslandCat", SunSpecTypeBitfield16, "", "", false))
	for _, name := range []string{"W_SF", "PF_SF", "VA_SF", "Var_SF", "V_SF", "A_SF", "S_SF"} {
		points = append(points, v2DERPoint("702", name, SunSpecTypeScaleFactor, "", "", false))
	}
	return points
}

func v2DERPoint(model, name string, pointType SunSpecPointType, unit, scale string, mandatory bool) sunSpecPointDefinition {
	symbols := map[uint64]string(nil)
	if model == "701" && name == "ACType" {
		symbols = map[uint64]string{0: "SINGLE_PHASE", 1: "SPLIT_PHASE", 2: "THREE_PHASE"}
	}
	return sunSpecPoint(name, "sunspec.der.v2."+model+"."+name, pointType, v2DERPointWords(pointType), unit, scale, mandatory, symbols, sunSpecKnownMask(symbols))
}

func v2DERPointWords(pointType SunSpecPointType) uint16 {
	switch pointType {
	case SunSpecTypeUint32, SunSpecTypeBitfield32:
		return 2
	case SunSpecTypeUint64:
		return 4
	case SunSpecTypeString:
		return 32
	default:
		return 1
	}
}
