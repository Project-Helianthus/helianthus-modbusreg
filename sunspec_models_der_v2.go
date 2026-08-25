package modbusreg

import "fmt"

const maxSunSpecDERPorts uint32 = (65535 - 18) / 25
const maxSunSpecBESSStrings uint32 = (65535 - 26) / 32
const maxSunSpecBESSModules uint32 = (65535 - 46) / 16

func sunSpecV2DERTripLVStructuralCandidate(revision SunSpecSchemaRevision, wireKey SunSpecWireKey, words []uint16, spans []SunSpecSourceSpan) *sunSpecStructuralCandidate {
	if revision != SunSpecModelsRevisionV2 || wireKey.ModelID != 707 || wireKey.ModelLength > 65534 || len(words) != int(wireKey.ModelLength)+2 || len(words) <= 6 || words[0] != wireKey.ModelID || words[1] != wireKey.ModelLength {
		return nil
	}
	points, curves := words[5], words[6]
	if points == 0xffff || curves == 0xffff {
		return nil
	}
	length := uint64(7) + uint64(curves)*(uint64(4)+9*uint64(points))
	if length > 65534 || length != uint64(wireKey.ModelLength) || !sunSpecStructuralCandidateSpansCover(spans, uint32(len(words))) {
		return nil
	}
	return &sunSpecStructuralCandidate{modelID: wireKey.ModelID}
}

func sunSpecStructuralCandidateSpansCover(spans []SunSpecSourceSpan, expected uint32) bool {
	if len(spans) == 0 {
		return false
	}
	var total uint32
	for _, span := range spans {
		if span.LogicalViewID == 0 || span.WordCount == 0 || uint32(span.PDUOffset)+uint32(span.WordCount) > maxSunSpecOccurrenceWords || uint32(span.WordCount) > expected-total {
			return false
		}
		total += uint32(span.WordCount)
	}
	return total == expected
}

func derTripLVV2NestedTemplate() (sunSpecNestedLayoutTemplate, error) {
	return newSunSpecNestedLayoutTemplate(
		sunSpecNestedTemplateKey{revision: SunSpecModelsRevisionV2, modelID: 707},
		[]sunSpecNestedCountSpec{
			{name: "points", occurrenceWordOffset: 5, unavailable: 0xffff, min: 0, max: 65534},
			{name: "curves", occurrenceWordOffset: 6, unavailable: 0xffff, min: 0, max: 65534},
		},
		sunSpecNestedLayoutGroup{
			fields: []sunSpecNestedLayoutField{
				{name: "ID", wordCount: 1}, {name: "L", wordCount: 1},
				{name: "Ena", wordCount: 1}, {name: "AdptCrvReq", wordCount: 1},
				{name: "AdptCrvRslt", wordCount: 1}, {name: "NPt", wordCount: 1},
				{name: "NCrvSet", wordCount: 1}, {name: "V_SF", wordCount: 1},
				{name: "Tms_SF", wordCount: 1},
			},
			children: []sunSpecNestedLayoutGroup{{
				name: "Crv", repeatCount: "curves", indexBase: 1,
				fields: []sunSpecNestedLayoutField{{name: "ReadOnly", wordCount: 1}},
				children: []sunSpecNestedLayoutGroup{
					{
						name: "MustTrip", fields: []sunSpecNestedLayoutField{{name: "ActPt", wordCount: 1}},
						children: []sunSpecNestedLayoutGroup{{
							name: "Pt", repeatCount: "points", indexBase: 1,
							fields: []sunSpecNestedLayoutField{{name: "V", wordCount: 1, emit: true, hasValueMetadata: true, valueMetadata: sunSpecNestedValueMetadata{pointType: SunSpecTypeUint16, scaleFactor: "V_SF"}}, {name: "Tms", wordCount: 2, emit: true, hasValueMetadata: true, valueMetadata: sunSpecNestedValueMetadata{pointType: SunSpecTypeUint32, scaleFactor: "Tms_SF"}}},
						}},
					},
					{
						name: "MayTrip", fields: []sunSpecNestedLayoutField{{name: "ActPt", wordCount: 1}},
						children: []sunSpecNestedLayoutGroup{{
							name: "Pt", repeatCount: "points", indexBase: 1,
							fields: []sunSpecNestedLayoutField{{name: "V", wordCount: 1, emit: true, hasValueMetadata: true, valueMetadata: sunSpecNestedValueMetadata{pointType: SunSpecTypeUint16, scaleFactor: "V_SF"}}, {name: "Tms", wordCount: 2, emit: true, hasValueMetadata: true, valueMetadata: sunSpecNestedValueMetadata{pointType: SunSpecTypeUint32, scaleFactor: "Tms_SF"}}},
						}},
					},
					{
						name: "MomCess", fields: []sunSpecNestedLayoutField{{name: "ActPt", wordCount: 1}},
						children: []sunSpecNestedLayoutGroup{{
							name: "Pt", repeatCount: "points", indexBase: 1,
							fields: []sunSpecNestedLayoutField{{name: "V", wordCount: 1, emit: true, hasValueMetadata: true, valueMetadata: sunSpecNestedValueMetadata{pointType: SunSpecTypeUint16, scaleFactor: "V_SF"}}, {name: "Tms", wordCount: 2, emit: true, hasValueMetadata: true, valueMetadata: sunSpecNestedValueMetadata{pointType: SunSpecTypeUint32, scaleFactor: "Tms_SF"}}},
						}},
					},
				},
			}},
		},
	)
}

func buildDERTripLVV2NestedLayout(words []uint16, spans []SunSpecSourceSpan) (sunSpecNestedOccurrenceLayout, error) {
	template, err := derTripLVV2NestedTemplate()
	if err != nil {
		return sunSpecNestedOccurrenceLayout{}, err
	}
	return buildSunSpecNestedOccurrenceLayout(template, words, spans)
}

func derDCMeasureV2SunSpecDefinition(length uint16) (sunSpecModelDefinition, error) {
	if length < 18 || (uint32(length)-18)%25 != 0 {
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec V2 Model 714 geometry is invalid")
	}
	ports := (uint32(length) - 18) / 25
	if ports > maxSunSpecDERPorts {
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec V2 Model 714 port count exceeds geometry")
	}
	points := []sunSpecPointDefinition{v2DERPoint("714", "ID", SunSpecTypeUint16, "", "", true), v2DERPoint("714", "L", SunSpecTypeUint16, "", "", true), v2DERPoint("714", "PrtAlrms", SunSpecTypeBitfield32, "", "", false), v2DERPoint("714", "NPrt", SunSpecTypeCount, "", "", false), v2DERPoint("714", "DCA", SunSpecTypeInt16, "A", "DCA_SF", false), v2DERPoint("714", "DCW", SunSpecTypeInt16, "W", "DCW_SF", false), v2DERPoint("714", "DCWhInj", SunSpecTypeUint64, "Wh", "DCWH_SF", false), v2DERPoint("714", "DCWhAbs", SunSpecTypeUint64, "Wh", "DCWH_SF", false), v2DERPoint("714", "DCA_SF", SunSpecTypeScaleFactor, "", "", false), v2DERPoint("714", "DCV_SF", SunSpecTypeScaleFactor, "", "", false), v2DERPoint("714", "DCW_SF", SunSpecTypeScaleFactor, "", "", false), v2DERPoint("714", "DCWH_SF", SunSpecTypeScaleFactor, "", "", false), v2DERPoint("714", "Tmp_SF", SunSpecTypeScaleFactor, "", "", false)}
	for i := uint32(1); i <= ports; i++ {
		for _, p := range []sunSpecPointDefinition{v2DERPoint("714", "PrtTyp", SunSpecTypeEnum16, "", "", false), v2DERPoint("714", "ID", SunSpecTypeUint16, "", "", false), v2DERStringPoint("714", "IDStr", 8), v2DERPoint("714", "DCA", SunSpecTypeInt16, "A", "DCA_SF", false), v2DERPoint("714", "DCV", SunSpecTypeUint16, "V", "DCV_SF", false), v2DERPoint("714", "DCW", SunSpecTypeInt16, "W", "DCW_SF", false), v2DERPoint("714", "DCWhInj", SunSpecTypeUint64, "Wh", "DCWH_SF", false), v2DERPoint("714", "DCWhAbs", SunSpecTypeUint64, "Wh", "DCWH_SF", false), v2DERPoint("714", "Tmp", SunSpecTypeInt16, "C", "Tmp_SF", false), v2DERPoint("714", "DCSta", SunSpecTypeEnum16, "", "", false), v2DERPoint("714", "DCAlrm", SunSpecTypeBitfield32, "", "", false)} {
			p.groupID = "port"
			p.repeatIndex = uint16(i)
			p.repeated = true
			points = append(points, p)
		}
	}
	defs, err := appendSunSpecDefinition(nil, SunSpecModelsRevisionV2, 714, length, SunSpecTopologyNone, false, points)
	if err != nil {
		return sunSpecModelDefinition{}, err
	}
	d := defs[0]
	d.geometry = func(words []uint16) bool {
		return len(words) == int(length)+2 && len(words) > 4 && words[4] != 0xffff && uint32(length) == 18+25*uint32(words[4])
	}
	return d, nil
}

func bessBankV2SunSpecDefinition(length uint16) (sunSpecModelDefinition, error) {
	if length < 26 || (uint32(length)-26)%32 != 0 {
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec V2 Model 803 geometry is invalid")
	}
	strings := (uint32(length) - 26) / 32
	if strings > maxSunSpecBESSStrings {
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec V2 Model 803 string count exceeds geometry")
	}
	points := []sunSpecPointDefinition{
		v2DERPoint("803", "ID", SunSpecTypeUint16, "", "", true),
		v2DERPoint("803", "L", SunSpecTypeUint16, "", "", true),
		v2DERPoint("803", "NStr", SunSpecTypeCount, "", "", false),
		v2DERPoint("803", "NStrCon", SunSpecTypeUint16, "", "", false),
		v2DERPoint("803", "ModTmpMax", SunSpecTypeInt16, "C", "ModTmp_SF", false),
		v2DERPoint("803", "ModTmpMaxStr", SunSpecTypeUint16, "", "", false),
		v2DERPoint("803", "ModTmpMaxMod", SunSpecTypeUint16, "", "", false),
		v2DERPoint("803", "ModTmpMin", SunSpecTypeInt16, "C", "ModTmp_SF", false),
		v2DERPoint("803", "ModTmpMinStr", SunSpecTypeUint16, "", "", false),
		v2DERPoint("803", "ModTmpMinMod", SunSpecTypeUint16, "", "", false),
		v2DERPoint("803", "ModTmpAvg", SunSpecTypeInt16, "C", "ModTmp_SF", false),
		v2DERPoint("803", "StrVMax", SunSpecTypeUint16, "V", "V_SF", false),
		v2DERPoint("803", "StrVMaxStr", SunSpecTypeUint16, "", "", false),
		v2DERPoint("803", "StrVMin", SunSpecTypeUint16, "V", "V_SF", false),
		v2DERPoint("803", "StrVMinStr", SunSpecTypeUint16, "", "", false),
		v2DERPoint("803", "StrVAvg", SunSpecTypeUint16, "V", "V_SF", false),
		v2DERPoint("803", "StrAMax", SunSpecTypeInt16, "A", "A_SF", false),
		v2DERPoint("803", "StrAMaxStr", SunSpecTypeUint16, "", "", false),
		v2DERPoint("803", "StrAMin", SunSpecTypeInt16, "A", "A_SF", false),
		v2DERPoint("803", "StrAMinStr", SunSpecTypeUint16, "", "", false),
		v2DERPoint("803", "StrAAvg", SunSpecTypeInt16, "A", "A_SF", false),
		v2DERPoint("803", "NCellBal", SunSpecTypeUint16, "", "", false),
		v2DERPoint("803", "CellV_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("803", "ModTmp_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("803", "A_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("803", "SoH_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("803", "SoC_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("803", "V_SF", SunSpecTypeScaleFactor, "", "", false),
	}
	for i := uint32(1); i <= strings; i++ {
		for _, point := range []struct {
			name string
			typ  SunSpecPointType
			unit string
			sf   string
		}{
			{"StrNMod", SunSpecTypeUint16, "", ""}, {"StrSt", SunSpecTypeBitfield32, "", ""},
			{"StrConFail", SunSpecTypeEnum16, "", ""}, {"StrSoC", SunSpecTypeUint16, "", "SoC_SF"},
			{"StrSoH", SunSpecTypeUint16, "", "SoH_SF"}, {"StrA", SunSpecTypeInt16, "A", "A_SF"},
			{"StrCellVMax", SunSpecTypeUint16, "V", "CellV_SF"}, {"StrCellVMaxMod", SunSpecTypeUint16, "", ""},
			{"StrCellVMin", SunSpecTypeUint16, "V", "CellV_SF"}, {"StrCellVMinMod", SunSpecTypeUint16, "", ""},
			{"StrCellVAvg", SunSpecTypeUint16, "V", "CellV_SF"}, {"StrModTmpMax", SunSpecTypeInt16, "C", "ModTmp_SF"},
			{"StrModTmpMaxMod", SunSpecTypeUint16, "", ""}, {"StrModTmpMin", SunSpecTypeInt16, "C", "ModTmp_SF"},
			{"StrModTmpMinMod", SunSpecTypeUint16, "", ""}, {"StrModTmpAvg", SunSpecTypeInt16, "C", "ModTmp_SF"},
			{"StrDisRsn", SunSpecTypeEnum16, "", ""}, {"StrConSt", SunSpecTypeBitfield32, "", ""},
			{"StrEvt1", SunSpecTypeBitfield32, "", ""}, {"StrEvt2", SunSpecTypeBitfield32, "", ""},
			{"StrEvtVnd1", SunSpecTypeBitfield32, "", ""}, {"StrEvtVnd2", SunSpecTypeBitfield32, "", ""},
			{"StrSetEna", SunSpecTypeEnum16, "", ""}, {"StrSetCon", SunSpecTypeEnum16, "", ""},
			{"Pad1", SunSpecTypePad, "", ""}, {"Pad2", SunSpecTypePad, "", ""},
		} {
			pointDefinition := v2DERPoint("803", point.name, point.typ, point.unit, point.sf, false)
			pointDefinition.groupID = "string"
			pointDefinition.repeatIndex = uint16(i)
			pointDefinition.repeated = true
			points = append(points, pointDefinition)
		}
	}
	definitions, err := appendSunSpecDefinition(nil, SunSpecModelsRevisionV2, 803, length, SunSpecTopologyNone, false, points)
	if err != nil {
		return sunSpecModelDefinition{}, err
	}
	definition := definitions[0]
	definition.geometry = func(words []uint16) bool {
		return len(words) == int(length)+2 && len(words) > 2 && words[2] != 0xffff && uint32(length) == 26+32*uint32(words[2])
	}
	return definition, nil
}

func bessStringV2SunSpecDefinition(length uint16) (sunSpecModelDefinition, error) {
	if length < 46 || (uint32(length)-46)%16 != 0 {
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec V2 Model 804 geometry is invalid")
	}
	modules := (uint32(length) - 46) / 16
	if modules > maxSunSpecBESSModules {
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec V2 Model 804 module count exceeds geometry")
	}
	points := []sunSpecPointDefinition{
		v2DERPoint("804", "ID", SunSpecTypeUint16, "", "", true),
		v2DERPoint("804", "L", SunSpecTypeUint16, "", "", true),
		v2DERPoint("804", "Idx", SunSpecTypeUint16, "", "", false),
		v2DERPoint("804", "NMod", SunSpecTypeCount, "", "", false),
		v2DERPoint("804", "St", SunSpecTypeBitfield32, "", "", false),
		v2DERPoint("804", "ConFail", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("804", "NCellBal", SunSpecTypeUint16, "", "", false),
		v2DERPoint("804", "SoC", SunSpecTypeUint16, "%", "SoC_SF", false),
		v2DERPoint("804", "DoD", SunSpecTypeUint16, "%", "DoD_SF", false),
		v2DERPoint("804", "NCyc", SunSpecTypeUint32, "", "", false),
		v2DERPoint("804", "SoH", SunSpecTypeUint16, "%", "SoH_SF", false),
		v2DERPoint("804", "A", SunSpecTypeInt16, "A", "A_SF", false),
		v2DERPoint("804", "V", SunSpecTypeUint16, "V", "V_SF", false),
		v2DERPoint("804", "CellVMax", SunSpecTypeUint16, "V", "CellV_SF", false),
		v2DERPoint("804", "CellVMaxMod", SunSpecTypeUint16, "", "", false),
		v2DERPoint("804", "CellVMin", SunSpecTypeUint16, "V", "CellV_SF", false),
		v2DERPoint("804", "CellVMinMod", SunSpecTypeUint16, "", "", false),
		v2DERPoint("804", "CellVAvg", SunSpecTypeUint16, "V", "CellV_SF", false),
		v2DERPoint("804", "ModTmpMax", SunSpecTypeInt16, "C", "ModTmp_SF", false),
		v2DERPoint("804", "ModTmpMaxMod", SunSpecTypeUint16, "", "", false),
		v2DERPoint("804", "ModTmpMin", SunSpecTypeInt16, "C", "ModTmp_SF", false),
		v2DERPoint("804", "ModTmpMinMod", SunSpecTypeUint16, "", "", false),
		v2DERPoint("804", "ModTmpAvg", SunSpecTypeInt16, "C", "ModTmp_SF", false),
		v2DERPoint("804", "Pad1", SunSpecTypePad, "", "", false),
		v2DERPoint("804", "ConSt", SunSpecTypeBitfield32, "", "", false),
		v2DERPoint("804", "Evt1", SunSpecTypeBitfield32, "", "", false),
		v2DERPoint("804", "Evt2", SunSpecTypeBitfield32, "", "", false),
		v2DERPoint("804", "EvtVnd1", SunSpecTypeBitfield32, "", "", false),
		v2DERPoint("804", "EvtVnd2", SunSpecTypeBitfield32, "", "", false),
		v2DERPoint("804", "SetEna", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("804", "SetCon", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("804", "SoC_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("804", "SoH_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("804", "DoD_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("804", "A_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("804", "V_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("804", "CellV_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("804", "ModTmp_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("804", "Pad2", SunSpecTypePad, "", "", false),
		v2DERPoint("804", "Pad3", SunSpecTypePad, "", "", false),
		v2DERPoint("804", "Pad4", SunSpecTypePad, "", "", false),
	}
	for i := uint32(1); i <= modules; i++ {
		for _, point := range []struct {
			name string
			typ  SunSpecPointType
			unit string
			sf   string
		}{
			{"ModNCell", SunSpecTypeUint16, "", ""}, {"ModSoC", SunSpecTypeUint16, "%", "SoC_SF"},
			{"ModSoH", SunSpecTypeUint16, "%", "SoH_SF"}, {"ModCellVMax", SunSpecTypeUint16, "V", "CellV_SF"},
			{"ModCellVMaxCell", SunSpecTypeUint16, "", ""}, {"ModCellVMin", SunSpecTypeUint16, "V", "CellV_SF"},
			{"ModCellVMinCell", SunSpecTypeUint16, "", ""}, {"ModCellVAvg", SunSpecTypeUint16, "V", "CellV_SF"},
			{"ModCellTmpMax", SunSpecTypeInt16, "C", "ModTmp_SF"}, {"ModCellTmpMaxCell", SunSpecTypeUint16, "", ""},
			{"ModCellTmpMin", SunSpecTypeInt16, "C", "ModTmp_SF"}, {"ModCellTmpMinCell", SunSpecTypeUint16, "", ""},
			{"ModCellTmpAvg", SunSpecTypeInt16, "C", "ModTmp_SF"}, {"Pad5", SunSpecTypePad, "", ""},
			{"Pad6", SunSpecTypePad, "", ""}, {"Pad7", SunSpecTypePad, "", ""},
		} {
			pointDefinition := v2DERPoint("804", point.name, point.typ, point.unit, point.sf, false)
			pointDefinition.groupID = "module"
			pointDefinition.repeatIndex = uint16(i)
			pointDefinition.repeated = true
			points = append(points, pointDefinition)
		}
	}
	definitions, err := appendSunSpecDefinition(nil, SunSpecModelsRevisionV2, 804, length, SunSpecTopologyNone, false, points)
	if err != nil {
		return sunSpecModelDefinition{}, err
	}
	definition := definitions[0]
	definition.geometry = func(words []uint16) bool {
		return len(words) == int(length)+2 && len(words) > 3 && words[3] != 0xffff && uint32(length) == 46+16*uint32(words[3])
	}
	return definition, nil
}

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

func derEnterServiceV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("703", "ID", SunSpecTypeUint16, "", "", true),
		v2DERPoint("703", "L", SunSpecTypeUint16, "", "", true),
		v2DERPoint("703", "ES", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("703", "ESVHi", SunSpecTypeUint16, "V", "V_SF", false),
		v2DERPoint("703", "ESVLo", SunSpecTypeUint16, "V", "V_SF", false),
		v2DERPoint("703", "ESHzHi", SunSpecTypeUint32, "Hz", "Hz_SF", false),
		v2DERPoint("703", "ESHzLo", SunSpecTypeUint32, "Hz", "Hz_SF", false),
		v2DERPoint("703", "ESDlyTms", SunSpecTypeUint32, "", "", false),
		v2DERPoint("703", "ESRndTms", SunSpecTypeUint32, "", "", false),
		v2DERPoint("703", "ESRmpTms", SunSpecTypeUint32, "", "", false),
		v2DERPoint("703", "ESDlyRemTms", SunSpecTypeUint32, "", "", false),
		v2DERPoint("703", "V_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("703", "Hz_SF", SunSpecTypeScaleFactor, "", "", false),
	}
}

func derStorageCapacityV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("713", "ID", SunSpecTypeUint16, "", "", true),
		v2DERPoint("713", "L", SunSpecTypeUint16, "", "", true),
		v2DERPoint("713", "WHRtg", SunSpecTypeUint16, "Wh", "WH_SF", false),
		v2DERPoint("713", "WHAvail", SunSpecTypeUint16, "Wh", "WH_SF", false),
		v2DERPoint("713", "SoC", SunSpecTypeUint16, "", "Pct_SF", false),
		v2DERPoint("713", "SoH", SunSpecTypeUint16, "", "Pct_SF", false),
		v2DERPoint("713", "Sta", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("713", "WH_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("713", "Pct_SF", SunSpecTypeScaleFactor, "", "", false),
	}
}

func derControlV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("715", "ID", SunSpecTypeUint16, "", "", true),
		v2DERPoint("715", "L", SunSpecTypeUint16, "", "", true),
		v2DERPoint("715", "LocRemCtl", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("715", "DERHb", SunSpecTypeUint32, "", "", false),
		v2DERPoint("715", "ControllerHb", SunSpecTypeUint32, "", "", false),
		v2DERPoint("715", "AlarmReset", SunSpecTypeUint16, "", "", false),
		v2DERPoint("715", "OpCtl", SunSpecTypeEnum16, "", "", false),
	}
}

func bessBaseV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("802", "ID", SunSpecTypeUint16, "", "", true),
		v2DERPoint("802", "L", SunSpecTypeUint16, "", "", true),
		v2DERPoint("802", "AHRtg", SunSpecTypeUint16, "Ah", "AHRtg_SF", false),
		v2DERPoint("802", "WHRtg", SunSpecTypeUint16, "Wh", "WHRtg_SF", false),
		v2DERPoint("802", "WChaRteMax", SunSpecTypeUint16, "W", "WChaDisChaMax_SF", false),
		v2DERPoint("802", "WDisChaRteMax", SunSpecTypeUint16, "W", "WChaDisChaMax_SF", false),
		v2DERPoint("802", "DisChaRte", SunSpecTypeUint16, "%WHRtg", "DisChaRte_SF", false),
		v2DERPoint("802", "SoCMax", SunSpecTypeUint16, "%WHRtg", "SoC_SF", false),
		v2DERPoint("802", "SoCMin", SunSpecTypeUint16, "%WHRtg", "SoC_SF", false),
		v2DERPoint("802", "SocRsvMax", SunSpecTypeUint16, "%WHRtg", "SoC_SF", false),
		v2DERPoint("802", "SoCRsvMin", SunSpecTypeUint16, "%WHRtg", "SoC_SF", false),
		v2DERPoint("802", "SoC", SunSpecTypeUint16, "%WHRtg", "SoC_SF", false),
		v2DERPoint("802", "DoD", SunSpecTypeUint16, "%", "DoD_SF", false),
		v2DERPoint("802", "SoH", SunSpecTypeUint16, "%", "SoH_SF", false),
		v2DERPoint("802", "NCyc", SunSpecTypeUint32, "", "", false),
		v2DERPoint("802", "ChaSt", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("802", "LocRemCtl", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("802", "Hb", SunSpecTypeUint16, "", "", false),
		v2DERPoint("802", "CtrlHb", SunSpecTypeUint16, "", "", false),
		v2DERPoint("802", "AlmRst", SunSpecTypeUint16, "", "", false),
		v2DERPoint("802", "Typ", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("802", "State", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("802", "StateVnd", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("802", "WarrDt", SunSpecTypeUint32, "", "", false),
		v2DERPoint("802", "Evt1", SunSpecTypeBitfield32, "", "", false),
		v2DERPoint("802", "Evt2", SunSpecTypeBitfield32, "", "", false),
		v2DERPoint("802", "EvtVnd1", SunSpecTypeBitfield32, "", "", false),
		v2DERPoint("802", "EvtVnd2", SunSpecTypeBitfield32, "", "", false),
		v2DERPoint("802", "V", SunSpecTypeUint16, "V", "V_SF", false),
		v2DERPoint("802", "VMax", SunSpecTypeUint16, "V", "V_SF", false),
		v2DERPoint("802", "VMin", SunSpecTypeUint16, "V", "V_SF", false),
		v2DERPoint("802", "CellVMax", SunSpecTypeUint16, "V", "CellV_SF", false),
		v2DERPoint("802", "CellVMaxStr", SunSpecTypeUint16, "", "", false),
		v2DERPoint("802", "CellVMaxMod", SunSpecTypeUint16, "", "", false),
		v2DERPoint("802", "CellVMin", SunSpecTypeUint16, "V", "CellV_SF", false),
		v2DERPoint("802", "CellVMinStr", SunSpecTypeUint16, "", "", false),
		v2DERPoint("802", "CellVMinMod", SunSpecTypeUint16, "", "", false),
		v2DERPoint("802", "CellVAvg", SunSpecTypeUint16, "V", "CellV_SF", false),
		v2DERPoint("802", "A", SunSpecTypeInt16, "A", "A_SF", false),
		v2DERPoint("802", "AChaMax", SunSpecTypeUint16, "A", "AMax_SF", false),
		v2DERPoint("802", "ADisChaMax", SunSpecTypeUint16, "A", "AMax_SF", false),
		v2DERPoint("802", "W", SunSpecTypeInt16, "W", "W_SF", false),
		v2DERPoint("802", "ReqInvState", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("802", "ReqW", SunSpecTypeInt16, "W", "W_SF", false),
		v2DERPoint("802", "SetOp", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("802", "SetInvState", SunSpecTypeEnum16, "", "", false),
		v2DERPoint("802", "AHRtg_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("802", "WHRtg_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("802", "WChaDisChaMax_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("802", "DisChaRte_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("802", "SoC_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("802", "DoD_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("802", "SoH_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("802", "V_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("802", "CellV_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("802", "A_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("802", "AMax_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("802", "W_SF", SunSpecTypeScaleFactor, "", "", false),
	}
}

func bessModuleV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("805", "ID", SunSpecTypeUint16, "", "", true),
		v2DERPoint("805", "L", SunSpecTypeUint16, "", "", true),
		v2DERPoint("805", "StrIdx", SunSpecTypeUint16, "", "", false),
		v2DERPoint("805", "ModIdx", SunSpecTypeUint16, "", "", false),
		v2DERPoint("805", "NCell", SunSpecTypeUint16, "", "", false),
		v2DERPoint("805", "SoC", SunSpecTypeUint16, "%", "SoC_SF", false),
		v2DERPoint("805", "DoD", SunSpecTypeUint16, "%", "DoD_SF", false),
		v2DERPoint("805", "SoH", SunSpecTypeUint16, "%", "SoH_SF", false),
		v2DERPoint("805", "NCyc", SunSpecTypeUint32, "", "", false),
		v2DERPoint("805", "V", SunSpecTypeUint16, "V", "V_SF", false),
		v2DERPoint("805", "CellVMax", SunSpecTypeUint16, "V", "CellV_SF", false),
		v2DERPoint("805", "CellVMaxCell", SunSpecTypeUint16, "", "", false),
		v2DERPoint("805", "CellVMin", SunSpecTypeUint16, "V", "CellV_SF", false),
		v2DERPoint("805", "CellVMinCell", SunSpecTypeUint16, "", "", false),
		v2DERPoint("805", "CellVAvg", SunSpecTypeUint16, "V", "CellV_SF", false),
		v2DERPoint("805", "CellTmpMax", SunSpecTypeInt16, "C", "Tmp_SF", false),
		v2DERPoint("805", "CellTmpMaxCell", SunSpecTypeUint16, "", "", false),
		v2DERPoint("805", "CellTmpMin", SunSpecTypeInt16, "C", "Tmp_SF", false),
		v2DERPoint("805", "CellTmpMinCell", SunSpecTypeUint16, "", "", false),
		v2DERPoint("805", "CellTmpAvg", SunSpecTypeInt16, "C", "Tmp_SF", false),
		v2DERPoint("805", "NCellBal", SunSpecTypeUint16, "", "", false),
		v2DERStringPoint("805", "SN", 16),
		v2DERPoint("805", "SoC_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("805", "SoH_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("805", "DoD_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("805", "V_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("805", "CellV_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("805", "Tmp_SF", SunSpecTypeScaleFactor, "", "", false),
	}
}

func flowBatteryV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("806", "ID", SunSpecTypeUint16, "", "", true),
		v2DERPoint("806", "L", SunSpecTypeUint16, "", "", true),
		v2DERPoint("806", "BatTBD", SunSpecTypeUint16, "", "", false),
	}
}

func flowBatteryStringV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("807", "ID", SunSpecTypeUint16, "", "", true),
		v2DERPoint("807", "L", SunSpecTypeUint16, "", "", true),
		v2DERPoint("807", "Idx", SunSpecTypeUint16, "", "", false),
		v2DERPoint("807", "NMod", SunSpecTypeUint16, "", "", false),
		v2DERPoint("807", "NModCon", SunSpecTypeUint16, "", "", false),
		v2DERPoint("807", "ModVMax", SunSpecTypeUint16, "V", "ModV_SF", false),
		v2DERPoint("807", "ModVMaxMod", SunSpecTypeUint16, "", "", false),
		v2DERPoint("807", "ModVMin", SunSpecTypeUint16, "V", "ModV_SF", false),
		v2DERPoint("807", "ModVMinMod", SunSpecTypeUint16, "", "", false),
		v2DERPoint("807", "ModVAvg", SunSpecTypeUint16, "V", "ModV_SF", false),
		v2DERPoint("807", "CellVMax", SunSpecTypeUint16, "V", "CellV_SF", false),
		v2DERPoint("807", "CellVMaxMod", SunSpecTypeUint16, "", "", false),
		v2DERPoint("807", "CellVMaxStk", SunSpecTypeUint16, "", "", false),
		v2DERPoint("807", "CellVMin", SunSpecTypeUint16, "V", "CellV_SF", false),
		v2DERPoint("807", "CellVMinMod", SunSpecTypeUint16, "", "", false),
		v2DERPoint("807", "CellVMinStk", SunSpecTypeUint16, "", "", false),
		v2DERPoint("807", "CellVAvg", SunSpecTypeUint16, "V", "CellV_SF", false),
		v2DERPoint("807", "TmpMax", SunSpecTypeInt16, "C", "Tmp_SF", false),
		v2DERPoint("807", "TmpMaxMod", SunSpecTypeUint16, "", "", false),
		v2DERPoint("807", "TmpMin", SunSpecTypeInt16, "C", "Tmp_SF", false),
		v2DERPoint("807", "TmpMinMod", SunSpecTypeUint16, "", "", false),
		v2DERPoint("807", "TmpAvg", SunSpecTypeInt16, "C", "Tmp_SF", false),
		v2DERPoint("807", "Evt1", SunSpecTypeBitfield32, "", "", false),
		v2DERPoint("807", "Evt2", SunSpecTypeBitfield32, "", "", false),
		v2DERPoint("807", "EvtVnd1", SunSpecTypeBitfield32, "", "", false),
		v2DERPoint("807", "EvtVnd2", SunSpecTypeBitfield32, "", "", false),
		v2DERPoint("807", "ModV_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("807", "CellV_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("807", "Tmp_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("807", "SoC_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("807", "OCV_SF", SunSpecTypeScaleFactor, "", "", false),
		v2DERPoint("807", "Pad1", SunSpecTypePad, "", "", false),
	}
}

func flowBatteryModuleV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("808", "ID", SunSpecTypeUint16, "", "", true),
		v2DERPoint("808", "L", SunSpecTypeUint16, "", "", true),
		v2DERPoint("808", "ModuleTBD", SunSpecTypeUint16, "", "", false),
	}
}

func flowBatteryStackV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("809", "ID", SunSpecTypeUint16, "", "", true),
		v2DERPoint("809", "L", SunSpecTypeUint16, "", "", true),
		v2DERPoint("809", "StackTBD", SunSpecTypeUint16, "", "", false),
	}
}

func v2DERPoint(model, name string, pointType SunSpecPointType, unit, scale string, mandatory bool) sunSpecPointDefinition {
	symbols := map[uint64]string(nil)
	if model == "701" && name == "ACType" {
		symbols = map[uint64]string{0: "SINGLE_PHASE", 1: "SPLIT_PHASE", 2: "THREE_PHASE"}
	}
	return sunSpecPoint(name, "sunspec.der.v2."+model+"."+name, pointType, v2DERPointWords(pointType), unit, scale, mandatory, symbols, sunSpecKnownMask(symbols))
}

func v2DERStringPoint(model, name string, words uint16) sunSpecPointDefinition {
	point := v2DERPoint(model, name, SunSpecTypeString, "", "", false)
	point.size = words
	return point
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
