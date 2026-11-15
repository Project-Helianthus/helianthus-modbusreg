package sunspec

import "fmt"

const maxSunSpecDERPorts uint32 = (65535 - 18) / 25
const maxSunSpecBESSStrings uint32 = (65535 - 26) / 32
const maxSunSpecBESSModules uint32 = (65535 - 46) / 16

func sunSpecV2DERTripLVStructuralCandidate(revision SchemaRevision, wireKey WireKey, words []uint16, spans []SourceSpan) *sunSpecStructuralCandidate {
	if revision != ModelsRevisionV2 || wireKey.ModelID != 707 || wireKey.ModelLength > 65534 || len(words) != int(wireKey.ModelLength)+2 || len(words) <= 6 || words[0] != wireKey.ModelID || words[1] != wireKey.ModelLength {
		return nil
	}
	points, curves := words[5], words[6]
	if points == 0xffff || curves == 0xffff {
		return nil
	}
	length := uint64(7) + uint64(curves)*(uint64(4)+9*uint64(points))
	if length > 65534 || length != uint64(wireKey.ModelLength) {
		return nil
	}
	layout, err := buildDERTripLVV2StructuralProjectionLayout(words, spans)
	if err != nil {
		return nil
	}
	return &sunSpecStructuralCandidate{modelID: wireKey.ModelID, layout: layout}
}

func derTripLVV2NestedTemplate() (sunSpecNestedLayoutTemplate, error) {
	return newSunSpecNestedLayoutTemplate(
		sunSpecNestedTemplateKey{revision: ModelsRevisionV2, modelID: 707},
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
							fields: []sunSpecNestedLayoutField{{name: "V", wordCount: 1, emit: true, hasValueMetadata: true, valueMetadata: sunSpecNestedValueMetadata{pointType: TypeUint16, scaleFactor: "V_SF"}}, {name: "Tms", wordCount: 2, emit: true, hasValueMetadata: true, valueMetadata: sunSpecNestedValueMetadata{pointType: TypeUint32, scaleFactor: "Tms_SF"}}},
						}},
					},
					{
						name: "MayTrip", fields: []sunSpecNestedLayoutField{{name: "ActPt", wordCount: 1}},
						children: []sunSpecNestedLayoutGroup{{
							name: "Pt", repeatCount: "points", indexBase: 1,
							fields: []sunSpecNestedLayoutField{{name: "V", wordCount: 1, emit: true, hasValueMetadata: true, valueMetadata: sunSpecNestedValueMetadata{pointType: TypeUint16, scaleFactor: "V_SF"}}, {name: "Tms", wordCount: 2, emit: true, hasValueMetadata: true, valueMetadata: sunSpecNestedValueMetadata{pointType: TypeUint32, scaleFactor: "Tms_SF"}}},
						}},
					},
					{
						name: "MomCess", fields: []sunSpecNestedLayoutField{{name: "ActPt", wordCount: 1}},
						children: []sunSpecNestedLayoutGroup{{
							name: "Pt", repeatCount: "points", indexBase: 1,
							fields: []sunSpecNestedLayoutField{{name: "V", wordCount: 1, emit: true, hasValueMetadata: true, valueMetadata: sunSpecNestedValueMetadata{pointType: TypeUint16, scaleFactor: "V_SF"}}, {name: "Tms", wordCount: 2, emit: true, hasValueMetadata: true, valueMetadata: sunSpecNestedValueMetadata{pointType: TypeUint32, scaleFactor: "Tms_SF"}}},
						}},
					},
				},
			}},
		},
	)
}

func buildDERTripLVV2NestedLayout(words []uint16, spans []SourceSpan) (sunSpecNestedOccurrenceLayout, error) {
	template, err := derTripLVV2NestedTemplate()
	if err != nil {
		return sunSpecNestedOccurrenceLayout{}, err
	}
	return buildSunSpecNestedOccurrenceLayout(template, words, spans)
}

func buildDERTripLVV2StructuralProjectionLayout(words []uint16, spans []SourceSpan) (sunSpecNestedOccurrenceLayout, error) {
	template, err := derTripLVV2StructuralProjectionTemplate()
	if err != nil {
		return sunSpecNestedOccurrenceLayout{}, err
	}
	return buildSunSpecNestedOccurrenceLayout(template, words, spans)
}

func derTripLVV2StructuralProjectionTemplate() (sunSpecNestedLayoutTemplate, error) {
	field := func(fieldID, name string, pointType PointType, wordCount uint32, unit, scaleFactor string, symbols map[uint64]string) sunSpecNestedLayoutField {
		return sunSpecNestedLayoutField{
			name: name, wordCount: wordCount, emit: true, hasValueMetadata: true,
			valueMetadata: sunSpecNestedValueMetadata{fieldID: fieldID, pointName: name, unit: unit, pointType: pointType, scaleFactor: scaleFactor, symbols: cloneSunSpecSymbols(symbols)},
		}
	}
	enaSymbols := map[uint64]string{0: "DISABLED", 1: "ENABLED"}
	resultSymbols := map[uint64]string{0: "IN_PROGRESS", 1: "COMPLETED", 2: "FAILED"}
	readOnlySymbols := map[uint64]string{0: "RW", 1: "R"}
	return newSunSpecNestedLayoutTemplate(
		sunSpecNestedTemplateKey{revision: ModelsRevisionV2, modelID: 707},
		[]sunSpecNestedCountSpec{
			{name: "points", occurrenceWordOffset: 5, unavailable: 0xffff, min: 0, max: 65534},
			{name: "curves", occurrenceWordOffset: 6, unavailable: 0xffff, min: 0, max: 65534},
		},
		sunSpecNestedLayoutGroup{
			fields: []sunSpecNestedLayoutField{
				field("sunspec.der.v2.707.ID", "ID", TypeUint16, 1, "", "", nil),
				field("sunspec.der.v2.707.L", "L", TypeUint16, 1, "", "", nil),
				field("sunspec.der.v2.707.Ena", "Ena", TypeEnum16, 1, "", "", enaSymbols),
				field("sunspec.der.v2.707.AdptCrvReq", "AdptCrvReq", TypeUint16, 1, "", "", nil),
				field("sunspec.der.v2.707.AdptCrvRslt", "AdptCrvRslt", TypeEnum16, 1, "", "", resultSymbols),
				field("sunspec.der.v2.707.NPt", "NPt", TypeUint16, 1, "", "", nil),
				field("sunspec.der.v2.707.NCrvSet", "NCrvSet", TypeUint16, 1, "", "", nil),
				field("sunspec.der.v2.707.V_SF", "V_SF", TypeScaleFactor, 1, "", "", nil),
				field("sunspec.der.v2.707.Tms_SF", "Tms_SF", TypeScaleFactor, 1, "", "", nil),
			},
			children: []sunSpecNestedLayoutGroup{{
				name: "Crv", repeatCount: "curves", indexBase: 1,
				fields: []sunSpecNestedLayoutField{field("sunspec.der.v2.707.Crv.ReadOnly", "ReadOnly", TypeEnum16, 1, "", "", readOnlySymbols)},
				children: []sunSpecNestedLayoutGroup{
					{name: "MustTrip", fields: []sunSpecNestedLayoutField{field("sunspec.der.v2.707.Crv.MustTrip.ActPt", "ActPt", TypeUint16, 1, "", "", nil)}, children: []sunSpecNestedLayoutGroup{{name: "Pt", repeatCount: "points", indexBase: 1, fields: []sunSpecNestedLayoutField{field("sunspec.der.v2.707.Crv.MustTrip.Pt.V", "V", TypeUint16, 1, "VNomPct", "V_SF", nil), field("sunspec.der.v2.707.Crv.MustTrip.Pt.Tms", "Tms", TypeUint32, 2, "Secs", "Tms_SF", nil)}}}},
					{name: "MayTrip", fields: []sunSpecNestedLayoutField{field("sunspec.der.v2.707.Crv.MayTrip.ActPt", "ActPt", TypeUint16, 1, "", "", nil)}, children: []sunSpecNestedLayoutGroup{{name: "Pt", repeatCount: "points", indexBase: 1, fields: []sunSpecNestedLayoutField{field("sunspec.der.v2.707.Crv.MayTrip.Pt.V", "V", TypeUint16, 1, "VNomPct", "V_SF", nil), field("sunspec.der.v2.707.Crv.MayTrip.Pt.Tms", "Tms", TypeUint32, 2, "Secs", "Tms_SF", nil)}}}},
					{name: "MomCess", fields: []sunSpecNestedLayoutField{field("sunspec.der.v2.707.Crv.MomCess.ActPt", "ActPt", TypeUint16, 1, "", "", nil)}, children: []sunSpecNestedLayoutGroup{{name: "Pt", repeatCount: "points", indexBase: 1, fields: []sunSpecNestedLayoutField{field("sunspec.der.v2.707.Crv.MomCess.Pt.V", "V", TypeUint16, 1, "VNomPct", "V_SF", nil), field("sunspec.der.v2.707.Crv.MomCess.Pt.Tms", "Tms", TypeUint32, 2, "Secs", "Tms_SF", nil)}}}},
				},
			}},
		},
	)
}

func derDCMeasureV2SunSpecDefinition(length uint16) (sunSpecModelDefinition, error) {
	if length < 18 || (uint32(length)-18)%25 != 0 {
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec V2 Model 714 geometry is invalid")
	}
	ports := (uint32(length) - 18) / 25
	if ports > maxSunSpecDERPorts {
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec V2 Model 714 port count exceeds geometry")
	}
	points := []sunSpecPointDefinition{v2DERPoint("714", "ID", TypeUint16, "", "", true), v2DERPoint("714", "L", TypeUint16, "", "", true), v2DERPoint("714", "PrtAlrms", TypeBitfield32, "", "", false), v2DERPoint("714", "NPrt", TypeCount, "", "", false), v2DERPoint("714", "DCA", TypeInt16, "A", "DCA_SF", false), v2DERPoint("714", "DCW", TypeInt16, "W", "DCW_SF", false), v2DERPoint("714", "DCWhInj", TypeUint64, "Wh", "DCWH_SF", false), v2DERPoint("714", "DCWhAbs", TypeUint64, "Wh", "DCWH_SF", false), v2DERPoint("714", "DCA_SF", TypeScaleFactor, "", "", false), v2DERPoint("714", "DCV_SF", TypeScaleFactor, "", "", false), v2DERPoint("714", "DCW_SF", TypeScaleFactor, "", "", false), v2DERPoint("714", "DCWH_SF", TypeScaleFactor, "", "", false), v2DERPoint("714", "Tmp_SF", TypeScaleFactor, "", "", false)}
	for i := uint32(1); i <= ports; i++ {
		for _, p := range []sunSpecPointDefinition{v2DERPoint("714", "PrtTyp", TypeEnum16, "", "", false), v2DERPoint("714", "ID", TypeUint16, "", "", false), v2DERStringPoint("714", "IDStr", 8), v2DERPoint("714", "DCA", TypeInt16, "A", "DCA_SF", false), v2DERPoint("714", "DCV", TypeUint16, "V", "DCV_SF", false), v2DERPoint("714", "DCW", TypeInt16, "W", "DCW_SF", false), v2DERPoint("714", "DCWhInj", TypeUint64, "Wh", "DCWH_SF", false), v2DERPoint("714", "DCWhAbs", TypeUint64, "Wh", "DCWH_SF", false), v2DERPoint("714", "Tmp", TypeInt16, "C", "Tmp_SF", false), v2DERPoint("714", "DCSta", TypeEnum16, "", "", false), v2DERPoint("714", "DCAlrm", TypeBitfield32, "", "", false)} {
			p.groupID = "port"
			p.repeatIndex = uint16(i)
			p.repeated = true
			points = append(points, p)
		}
	}
	defs, err := appendSunSpecDefinition(nil, ModelsRevisionV2, 714, length, TopologyNone, false, points)
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
		v2DERPoint("803", "ID", TypeUint16, "", "", true),
		v2DERPoint("803", "L", TypeUint16, "", "", true),
		v2DERPoint("803", "NStr", TypeCount, "", "", false),
		v2DERPoint("803", "NStrCon", TypeUint16, "", "", false),
		v2DERPoint("803", "ModTmpMax", TypeInt16, "C", "ModTmp_SF", false),
		v2DERPoint("803", "ModTmpMaxStr", TypeUint16, "", "", false),
		v2DERPoint("803", "ModTmpMaxMod", TypeUint16, "", "", false),
		v2DERPoint("803", "ModTmpMin", TypeInt16, "C", "ModTmp_SF", false),
		v2DERPoint("803", "ModTmpMinStr", TypeUint16, "", "", false),
		v2DERPoint("803", "ModTmpMinMod", TypeUint16, "", "", false),
		v2DERPoint("803", "ModTmpAvg", TypeInt16, "C", "ModTmp_SF", false),
		v2DERPoint("803", "StrVMax", TypeUint16, "V", "V_SF", false),
		v2DERPoint("803", "StrVMaxStr", TypeUint16, "", "", false),
		v2DERPoint("803", "StrVMin", TypeUint16, "V", "V_SF", false),
		v2DERPoint("803", "StrVMinStr", TypeUint16, "", "", false),
		v2DERPoint("803", "StrVAvg", TypeUint16, "V", "V_SF", false),
		v2DERPoint("803", "StrAMax", TypeInt16, "A", "A_SF", false),
		v2DERPoint("803", "StrAMaxStr", TypeUint16, "", "", false),
		v2DERPoint("803", "StrAMin", TypeInt16, "A", "A_SF", false),
		v2DERPoint("803", "StrAMinStr", TypeUint16, "", "", false),
		v2DERPoint("803", "StrAAvg", TypeInt16, "A", "A_SF", false),
		v2DERPoint("803", "NCellBal", TypeUint16, "", "", false),
		v2DERPoint("803", "CellV_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("803", "ModTmp_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("803", "A_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("803", "SoH_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("803", "SoC_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("803", "V_SF", TypeScaleFactor, "", "", false),
	}
	for i := uint32(1); i <= strings; i++ {
		for _, point := range []struct {
			name string
			typ  PointType
			unit string
			sf   string
		}{
			{"StrNMod", TypeUint16, "", ""}, {"StrSt", TypeBitfield32, "", ""},
			{"StrConFail", TypeEnum16, "", ""}, {"StrSoC", TypeUint16, "", "SoC_SF"},
			{"StrSoH", TypeUint16, "", "SoH_SF"}, {"StrA", TypeInt16, "A", "A_SF"},
			{"StrCellVMax", TypeUint16, "V", "CellV_SF"}, {"StrCellVMaxMod", TypeUint16, "", ""},
			{"StrCellVMin", TypeUint16, "V", "CellV_SF"}, {"StrCellVMinMod", TypeUint16, "", ""},
			{"StrCellVAvg", TypeUint16, "V", "CellV_SF"}, {"StrModTmpMax", TypeInt16, "C", "ModTmp_SF"},
			{"StrModTmpMaxMod", TypeUint16, "", ""}, {"StrModTmpMin", TypeInt16, "C", "ModTmp_SF"},
			{"StrModTmpMinMod", TypeUint16, "", ""}, {"StrModTmpAvg", TypeInt16, "C", "ModTmp_SF"},
			{"StrDisRsn", TypeEnum16, "", ""}, {"StrConSt", TypeBitfield32, "", ""},
			{"StrEvt1", TypeBitfield32, "", ""}, {"StrEvt2", TypeBitfield32, "", ""},
			{"StrEvtVnd1", TypeBitfield32, "", ""}, {"StrEvtVnd2", TypeBitfield32, "", ""},
			{"StrSetEna", TypeEnum16, "", ""}, {"StrSetCon", TypeEnum16, "", ""},
			{"Pad1", TypePad, "", ""}, {"Pad2", TypePad, "", ""},
		} {
			pointDefinition := v2DERPoint("803", point.name, point.typ, point.unit, point.sf, false)
			pointDefinition.groupID = "string"
			pointDefinition.repeatIndex = uint16(i)
			pointDefinition.repeated = true
			points = append(points, pointDefinition)
		}
	}
	definitions, err := appendSunSpecDefinition(nil, ModelsRevisionV2, 803, length, TopologyNone, false, points)
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
		v2DERPoint("804", "ID", TypeUint16, "", "", true),
		v2DERPoint("804", "L", TypeUint16, "", "", true),
		v2DERPoint("804", "Idx", TypeUint16, "", "", false),
		v2DERPoint("804", "NMod", TypeCount, "", "", false),
		v2DERPoint("804", "St", TypeBitfield32, "", "", false),
		v2DERPoint("804", "ConFail", TypeEnum16, "", "", false),
		v2DERPoint("804", "NCellBal", TypeUint16, "", "", false),
		v2DERPoint("804", "SoC", TypeUint16, "%", "SoC_SF", false),
		v2DERPoint("804", "DoD", TypeUint16, "%", "DoD_SF", false),
		v2DERPoint("804", "NCyc", TypeUint32, "", "", false),
		v2DERPoint("804", "SoH", TypeUint16, "%", "SoH_SF", false),
		v2DERPoint("804", "A", TypeInt16, "A", "A_SF", false),
		v2DERPoint("804", "V", TypeUint16, "V", "V_SF", false),
		v2DERPoint("804", "CellVMax", TypeUint16, "V", "CellV_SF", false),
		v2DERPoint("804", "CellVMaxMod", TypeUint16, "", "", false),
		v2DERPoint("804", "CellVMin", TypeUint16, "V", "CellV_SF", false),
		v2DERPoint("804", "CellVMinMod", TypeUint16, "", "", false),
		v2DERPoint("804", "CellVAvg", TypeUint16, "V", "CellV_SF", false),
		v2DERPoint("804", "ModTmpMax", TypeInt16, "C", "ModTmp_SF", false),
		v2DERPoint("804", "ModTmpMaxMod", TypeUint16, "", "", false),
		v2DERPoint("804", "ModTmpMin", TypeInt16, "C", "ModTmp_SF", false),
		v2DERPoint("804", "ModTmpMinMod", TypeUint16, "", "", false),
		v2DERPoint("804", "ModTmpAvg", TypeInt16, "C", "ModTmp_SF", false),
		v2DERPoint("804", "Pad1", TypePad, "", "", false),
		v2DERPoint("804", "ConSt", TypeBitfield32, "", "", false),
		v2DERPoint("804", "Evt1", TypeBitfield32, "", "", false),
		v2DERPoint("804", "Evt2", TypeBitfield32, "", "", false),
		v2DERPoint("804", "EvtVnd1", TypeBitfield32, "", "", false),
		v2DERPoint("804", "EvtVnd2", TypeBitfield32, "", "", false),
		v2DERPoint("804", "SetEna", TypeEnum16, "", "", false),
		v2DERPoint("804", "SetCon", TypeEnum16, "", "", false),
		v2DERPoint("804", "SoC_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("804", "SoH_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("804", "DoD_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("804", "A_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("804", "V_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("804", "CellV_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("804", "ModTmp_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("804", "Pad2", TypePad, "", "", false),
		v2DERPoint("804", "Pad3", TypePad, "", "", false),
		v2DERPoint("804", "Pad4", TypePad, "", "", false),
	}
	for i := uint32(1); i <= modules; i++ {
		for _, point := range []struct {
			name string
			typ  PointType
			unit string
			sf   string
		}{
			{"ModNCell", TypeUint16, "", ""}, {"ModSoC", TypeUint16, "%", "SoC_SF"},
			{"ModSoH", TypeUint16, "%", "SoH_SF"}, {"ModCellVMax", TypeUint16, "V", "CellV_SF"},
			{"ModCellVMaxCell", TypeUint16, "", ""}, {"ModCellVMin", TypeUint16, "V", "CellV_SF"},
			{"ModCellVMinCell", TypeUint16, "", ""}, {"ModCellVAvg", TypeUint16, "V", "CellV_SF"},
			{"ModCellTmpMax", TypeInt16, "C", "ModTmp_SF"}, {"ModCellTmpMaxCell", TypeUint16, "", ""},
			{"ModCellTmpMin", TypeInt16, "C", "ModTmp_SF"}, {"ModCellTmpMinCell", TypeUint16, "", ""},
			{"ModCellTmpAvg", TypeInt16, "C", "ModTmp_SF"}, {"Pad5", TypePad, "", ""},
			{"Pad6", TypePad, "", ""}, {"Pad7", TypePad, "", ""},
		} {
			pointDefinition := v2DERPoint("804", point.name, point.typ, point.unit, point.sf, false)
			pointDefinition.groupID = "module"
			pointDefinition.repeatIndex = uint16(i)
			pointDefinition.repeated = true
			points = append(points, pointDefinition)
		}
	}
	definitions, err := appendSunSpecDefinition(nil, ModelsRevisionV2, 804, length, TopologyNone, false, points)
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
		v2DERPoint("701", "ID", TypeUint16, "", "", true),
		v2DERPoint("701", "L", TypeUint16, "", "", true),
		v2DERPoint("701", "ACType", TypeEnum16, "", "", true),
		v2DERPoint("701", "St", TypeEnum16, "", "", false),
		v2DERPoint("701", "InvSt", TypeEnum16, "", "", false),
		v2DERPoint("701", "ConnSt", TypeEnum16, "", "", false),
		v2DERPoint("701", "Alrm", TypeBitfield32, "", "", false),
		v2DERPoint("701", "DERMode", TypeBitfield32, "", "", false),
	}
	for _, point := range []struct {
		name string
		typ  PointType
		unit string
		sf   string
	}{
		{"W", TypeInt16, "W", "W_SF"}, {"VA", TypeInt16, "VA", "VA_SF"},
		{"Var", TypeInt16, "Var", "Var_SF"}, {"PF", TypeInt16, "", "PF_SF"},
		{"A", TypeInt16, "A", "A_SF"}, {"LLV", TypeUint16, "V", "V_SF"},
		{"LNV", TypeUint16, "V", "V_SF"}, {"Hz", TypeUint32, "Hz", "Hz_SF"},
		{"TotWhInj", TypeUint64, "Wh", "TotWh_SF"}, {"TotWhAbs", TypeUint64, "Wh", "TotWh_SF"},
		{"TotVarhInj", TypeUint64, "Varh", "TotVarh_SF"}, {"TotVarhAbs", TypeUint64, "Varh", "TotVarh_SF"},
		{"TmpAmb", TypeInt16, "C", "Tmp_SF"}, {"TmpCab", TypeInt16, "C", "Tmp_SF"},
		{"TmpSnk", TypeInt16, "C", "Tmp_SF"}, {"TmpTrns", TypeInt16, "C", "Tmp_SF"},
		{"TmpSw", TypeInt16, "C", "Tmp_SF"}, {"TmpOt", TypeInt16, "C", "Tmp_SF"},
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
			typ  PointType
			unit string
			sf   string
		}{
			{"W" + phase.suffix, TypeInt16, "W", "W_SF"}, {"VA" + phase.suffix, TypeInt16, "VA", "VA_SF"},
			{"Var" + phase.suffix, TypeInt16, "Var", "Var_SF"}, {"PF" + phase.suffix, TypeInt16, "", "PF_SF"},
			{"A" + phase.suffix, TypeInt16, "A", "A_SF"}, {phase.line, TypeUint16, "V", "V_SF"},
			{phase.voltage, TypeUint16, "V", "V_SF"}, {"TotWhInj" + phase.suffix, TypeUint64, "Wh", "TotWh_SF"},
			{"TotWhAbs" + phase.suffix, TypeUint64, "Wh", "TotWh_SF"}, {"TotVarhInj" + phase.suffix, TypeUint64, "Varh", "TotVarh_SF"},
			{"TotVarhAbs" + phase.suffix, TypeUint64, "Varh", "TotVarh_SF"},
		} {
			points = append(points, v2DERPoint("701", point.name, point.typ, point.unit, point.sf, false))
		}
	}
	for _, point := range []struct {
		name string
		typ  PointType
	}{
		{"ThrotPct", TypeUint16}, {"ThrotSrc", TypeBitfield32},
		{"A_SF", TypeScaleFactor}, {"V_SF", TypeScaleFactor}, {"Hz_SF", TypeScaleFactor},
		{"W_SF", TypeScaleFactor}, {"PF_SF", TypeScaleFactor}, {"VA_SF", TypeScaleFactor},
		{"Var_SF", TypeScaleFactor}, {"TotWh_SF", TypeScaleFactor}, {"TotVarh_SF", TypeScaleFactor},
		{"Tmp_SF", TypeScaleFactor}, {"MnAlrmInfo", TypeString},
	} {
		points = append(points, v2DERPoint("701", point.name, point.typ, "", "", false))
	}
	return points
}

func derCapacityV2SunSpecPoints() []sunSpecPointDefinition {
	points := []sunSpecPointDefinition{
		v2DERPoint("702", "ID", TypeUint16, "", "", true),
		v2DERPoint("702", "L", TypeUint16, "", "", true),
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
		points = append(points, v2DERPoint("702", point.name, TypeUint16, point.unit, point.sf, false))
	}
	for _, point := range []struct {
		name string
		typ  PointType
	}{
		{"NorOpCatRtg", TypeEnum16}, {"AbnOpCatRtg", TypeEnum16}, {"CtrlModes", TypeBitfield32},
		{"IntIslandCatRtg", TypeBitfield16},
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
		points = append(points, v2DERPoint("702", point.name, TypeUint16, point.unit, point.sf, false))
	}
	points = append(points, v2DERPoint("702", "IntIslandCat", TypeBitfield16, "", "", false))
	for _, name := range []string{"W_SF", "PF_SF", "VA_SF", "Var_SF", "V_SF", "A_SF", "S_SF"} {
		points = append(points, v2DERPoint("702", name, TypeScaleFactor, "", "", false))
	}
	return points
}

func derEnterServiceV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("703", "ID", TypeUint16, "", "", true),
		v2DERPoint("703", "L", TypeUint16, "", "", true),
		v2DERPoint("703", "ES", TypeEnum16, "", "", false),
		v2DERPoint("703", "ESVHi", TypeUint16, "V", "V_SF", false),
		v2DERPoint("703", "ESVLo", TypeUint16, "V", "V_SF", false),
		v2DERPoint("703", "ESHzHi", TypeUint32, "Hz", "Hz_SF", false),
		v2DERPoint("703", "ESHzLo", TypeUint32, "Hz", "Hz_SF", false),
		v2DERPoint("703", "ESDlyTms", TypeUint32, "", "", false),
		v2DERPoint("703", "ESRndTms", TypeUint32, "", "", false),
		v2DERPoint("703", "ESRmpTms", TypeUint32, "", "", false),
		v2DERPoint("703", "ESDlyRemTms", TypeUint32, "", "", false),
		v2DERPoint("703", "V_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("703", "Hz_SF", TypeScaleFactor, "", "", false),
	}
}

func derStorageCapacityV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("713", "ID", TypeUint16, "", "", true),
		v2DERPoint("713", "L", TypeUint16, "", "", true),
		v2DERPoint("713", "WHRtg", TypeUint16, "Wh", "WH_SF", false),
		v2DERPoint("713", "WHAvail", TypeUint16, "Wh", "WH_SF", false),
		v2DERPoint("713", "SoC", TypeUint16, "", "Pct_SF", false),
		v2DERPoint("713", "SoH", TypeUint16, "", "Pct_SF", false),
		v2DERPoint("713", "Sta", TypeEnum16, "", "", false),
		v2DERPoint("713", "WH_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("713", "Pct_SF", TypeScaleFactor, "", "", false),
	}
}

func derControlV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("715", "ID", TypeUint16, "", "", true),
		v2DERPoint("715", "L", TypeUint16, "", "", true),
		v2DERPoint("715", "LocRemCtl", TypeEnum16, "", "", false),
		v2DERPoint("715", "DERHb", TypeUint32, "", "", false),
		v2DERPoint("715", "ControllerHb", TypeUint32, "", "", false),
		v2DERPoint("715", "AlarmReset", TypeUint16, "", "", false),
		v2DERPoint("715", "OpCtl", TypeEnum16, "", "", false),
	}
}

func bessBaseV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("802", "ID", TypeUint16, "", "", true),
		v2DERPoint("802", "L", TypeUint16, "", "", true),
		v2DERPoint("802", "AHRtg", TypeUint16, "Ah", "AHRtg_SF", false),
		v2DERPoint("802", "WHRtg", TypeUint16, "Wh", "WHRtg_SF", false),
		v2DERPoint("802", "WChaRteMax", TypeUint16, "W", "WChaDisChaMax_SF", false),
		v2DERPoint("802", "WDisChaRteMax", TypeUint16, "W", "WChaDisChaMax_SF", false),
		v2DERPoint("802", "DisChaRte", TypeUint16, "%WHRtg", "DisChaRte_SF", false),
		v2DERPoint("802", "SoCMax", TypeUint16, "%WHRtg", "SoC_SF", false),
		v2DERPoint("802", "SoCMin", TypeUint16, "%WHRtg", "SoC_SF", false),
		v2DERPoint("802", "SocRsvMax", TypeUint16, "%WHRtg", "SoC_SF", false),
		v2DERPoint("802", "SoCRsvMin", TypeUint16, "%WHRtg", "SoC_SF", false),
		v2DERPoint("802", "SoC", TypeUint16, "%WHRtg", "SoC_SF", false),
		v2DERPoint("802", "DoD", TypeUint16, "%", "DoD_SF", false),
		v2DERPoint("802", "SoH", TypeUint16, "%", "SoH_SF", false),
		v2DERPoint("802", "NCyc", TypeUint32, "", "", false),
		v2DERPoint("802", "ChaSt", TypeEnum16, "", "", false),
		v2DERPoint("802", "LocRemCtl", TypeEnum16, "", "", false),
		v2DERPoint("802", "Hb", TypeUint16, "", "", false),
		v2DERPoint("802", "CtrlHb", TypeUint16, "", "", false),
		v2DERPoint("802", "AlmRst", TypeUint16, "", "", false),
		v2DERPoint("802", "Typ", TypeEnum16, "", "", false),
		v2DERPoint("802", "State", TypeEnum16, "", "", false),
		v2DERPoint("802", "StateVnd", TypeEnum16, "", "", false),
		v2DERPoint("802", "WarrDt", TypeUint32, "", "", false),
		v2DERPoint("802", "Evt1", TypeBitfield32, "", "", false),
		v2DERPoint("802", "Evt2", TypeBitfield32, "", "", false),
		v2DERPoint("802", "EvtVnd1", TypeBitfield32, "", "", false),
		v2DERPoint("802", "EvtVnd2", TypeBitfield32, "", "", false),
		v2DERPoint("802", "V", TypeUint16, "V", "V_SF", false),
		v2DERPoint("802", "VMax", TypeUint16, "V", "V_SF", false),
		v2DERPoint("802", "VMin", TypeUint16, "V", "V_SF", false),
		v2DERPoint("802", "CellVMax", TypeUint16, "V", "CellV_SF", false),
		v2DERPoint("802", "CellVMaxStr", TypeUint16, "", "", false),
		v2DERPoint("802", "CellVMaxMod", TypeUint16, "", "", false),
		v2DERPoint("802", "CellVMin", TypeUint16, "V", "CellV_SF", false),
		v2DERPoint("802", "CellVMinStr", TypeUint16, "", "", false),
		v2DERPoint("802", "CellVMinMod", TypeUint16, "", "", false),
		v2DERPoint("802", "CellVAvg", TypeUint16, "V", "CellV_SF", false),
		v2DERPoint("802", "A", TypeInt16, "A", "A_SF", false),
		v2DERPoint("802", "AChaMax", TypeUint16, "A", "AMax_SF", false),
		v2DERPoint("802", "ADisChaMax", TypeUint16, "A", "AMax_SF", false),
		v2DERPoint("802", "W", TypeInt16, "W", "W_SF", false),
		v2DERPoint("802", "ReqInvState", TypeEnum16, "", "", false),
		v2DERPoint("802", "ReqW", TypeInt16, "W", "W_SF", false),
		v2DERPoint("802", "SetOp", TypeEnum16, "", "", false),
		v2DERPoint("802", "SetInvState", TypeEnum16, "", "", false),
		v2DERPoint("802", "AHRtg_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("802", "WHRtg_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("802", "WChaDisChaMax_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("802", "DisChaRte_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("802", "SoC_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("802", "DoD_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("802", "SoH_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("802", "V_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("802", "CellV_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("802", "A_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("802", "AMax_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("802", "W_SF", TypeScaleFactor, "", "", false),
	}
}

func bessModuleV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("805", "ID", TypeUint16, "", "", true),
		v2DERPoint("805", "L", TypeUint16, "", "", true),
		v2DERPoint("805", "StrIdx", TypeUint16, "", "", false),
		v2DERPoint("805", "ModIdx", TypeUint16, "", "", false),
		v2DERPoint("805", "NCell", TypeUint16, "", "", false),
		v2DERPoint("805", "SoC", TypeUint16, "%", "SoC_SF", false),
		v2DERPoint("805", "DoD", TypeUint16, "%", "DoD_SF", false),
		v2DERPoint("805", "SoH", TypeUint16, "%", "SoH_SF", false),
		v2DERPoint("805", "NCyc", TypeUint32, "", "", false),
		v2DERPoint("805", "V", TypeUint16, "V", "V_SF", false),
		v2DERPoint("805", "CellVMax", TypeUint16, "V", "CellV_SF", false),
		v2DERPoint("805", "CellVMaxCell", TypeUint16, "", "", false),
		v2DERPoint("805", "CellVMin", TypeUint16, "V", "CellV_SF", false),
		v2DERPoint("805", "CellVMinCell", TypeUint16, "", "", false),
		v2DERPoint("805", "CellVAvg", TypeUint16, "V", "CellV_SF", false),
		v2DERPoint("805", "CellTmpMax", TypeInt16, "C", "Tmp_SF", false),
		v2DERPoint("805", "CellTmpMaxCell", TypeUint16, "", "", false),
		v2DERPoint("805", "CellTmpMin", TypeInt16, "C", "Tmp_SF", false),
		v2DERPoint("805", "CellTmpMinCell", TypeUint16, "", "", false),
		v2DERPoint("805", "CellTmpAvg", TypeInt16, "C", "Tmp_SF", false),
		v2DERPoint("805", "NCellBal", TypeUint16, "", "", false),
		v2DERStringPoint("805", "SN", 16),
		v2DERPoint("805", "SoC_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("805", "SoH_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("805", "DoD_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("805", "V_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("805", "CellV_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("805", "Tmp_SF", TypeScaleFactor, "", "", false),
	}
}

func flowBatteryV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("806", "ID", TypeUint16, "", "", true),
		v2DERPoint("806", "L", TypeUint16, "", "", true),
		v2DERPoint("806", "BatTBD", TypeUint16, "", "", false),
	}
}

func flowBatteryStringV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("807", "ID", TypeUint16, "", "", true),
		v2DERPoint("807", "L", TypeUint16, "", "", true),
		v2DERPoint("807", "Idx", TypeUint16, "", "", false),
		v2DERPoint("807", "NMod", TypeUint16, "", "", false),
		v2DERPoint("807", "NModCon", TypeUint16, "", "", false),
		v2DERPoint("807", "ModVMax", TypeUint16, "V", "ModV_SF", false),
		v2DERPoint("807", "ModVMaxMod", TypeUint16, "", "", false),
		v2DERPoint("807", "ModVMin", TypeUint16, "V", "ModV_SF", false),
		v2DERPoint("807", "ModVMinMod", TypeUint16, "", "", false),
		v2DERPoint("807", "ModVAvg", TypeUint16, "V", "ModV_SF", false),
		v2DERPoint("807", "CellVMax", TypeUint16, "V", "CellV_SF", false),
		v2DERPoint("807", "CellVMaxMod", TypeUint16, "", "", false),
		v2DERPoint("807", "CellVMaxStk", TypeUint16, "", "", false),
		v2DERPoint("807", "CellVMin", TypeUint16, "V", "CellV_SF", false),
		v2DERPoint("807", "CellVMinMod", TypeUint16, "", "", false),
		v2DERPoint("807", "CellVMinStk", TypeUint16, "", "", false),
		v2DERPoint("807", "CellVAvg", TypeUint16, "V", "CellV_SF", false),
		v2DERPoint("807", "TmpMax", TypeInt16, "C", "Tmp_SF", false),
		v2DERPoint("807", "TmpMaxMod", TypeUint16, "", "", false),
		v2DERPoint("807", "TmpMin", TypeInt16, "C", "Tmp_SF", false),
		v2DERPoint("807", "TmpMinMod", TypeUint16, "", "", false),
		v2DERPoint("807", "TmpAvg", TypeInt16, "C", "Tmp_SF", false),
		v2DERPoint("807", "Evt1", TypeBitfield32, "", "", false),
		v2DERPoint("807", "Evt2", TypeBitfield32, "", "", false),
		v2DERPoint("807", "EvtVnd1", TypeBitfield32, "", "", false),
		v2DERPoint("807", "EvtVnd2", TypeBitfield32, "", "", false),
		v2DERPoint("807", "ModV_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("807", "CellV_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("807", "Tmp_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("807", "SoC_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("807", "OCV_SF", TypeScaleFactor, "", "", false),
		v2DERPoint("807", "Pad1", TypePad, "", "", false),
	}
}

func flowBatteryModuleV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("808", "ID", TypeUint16, "", "", true),
		v2DERPoint("808", "L", TypeUint16, "", "", true),
		v2DERPoint("808", "ModuleTBD", TypeUint16, "", "", false),
	}
}

func flowBatteryStackV2SunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		v2DERPoint("809", "ID", TypeUint16, "", "", true),
		v2DERPoint("809", "L", TypeUint16, "", "", true),
		v2DERPoint("809", "StackTBD", TypeUint16, "", "", false),
	}
}

func v2DERPoint(model, name string, pointType PointType, unit, scale string, mandatory bool) sunSpecPointDefinition {
	symbols := map[uint64]string(nil)
	if model == "701" && name == "ACType" {
		symbols = map[uint64]string{0: "SINGLE_PHASE", 1: "SPLIT_PHASE", 2: "THREE_PHASE"}
	}
	return sunSpecPoint(name, "sunspec.der.v2."+model+"."+name, pointType, v2DERPointWords(pointType), unit, scale, mandatory, symbols, sunSpecKnownMask(symbols))
}

func v2DERStringPoint(model, name string, words uint16) sunSpecPointDefinition {
	point := v2DERPoint(model, name, TypeString, "", "", false)
	point.size = words
	return point
}

func v2DERPointWords(pointType PointType) uint16 {
	switch pointType {
	case TypeUint32, TypeBitfield32:
		return 2
	case TypeUint64:
		return 4
	case TypeString:
		return 32
	default:
		return 1
	}
}
