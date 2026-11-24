package sunspec

import (
	"fmt"
	"sync"
)

type gridColumn struct {
	name, unit, scale string
	pointType         PointType
	size              uint16
	symbols           map[uint64]string
}

type gridSymbols struct {
	enable, enabled, readOnly, deptRef, wattDept, edge, sigType, cease, access, ena, adopt map[uint64]string
	schedule, interval, axis, dependent                                                    map[uint64]string
}

type gridBody uint8

const (
	gridBodyTime gridBody = iota
	gridBodyVoltVar
	gridBodyWattPF
	gridBodyVoltWatt
	gridBodySchedule
	gridBodyFreqWatt
)

type gridRepeatSpec struct {
	id, base, stride, countOffset uint16
	group, countName, second      string
	body                          gridBody
	cease                         bool
}

var (
	gridSymbolOnce  sync.Once
	gridSymbolValue gridSymbols
	derTripOnce     sync.Once
	derTripHigh     map[uint16]struct{}
	derTripLow      map[uint16]struct{}
)

func gridSymbolSet() gridSymbols {
	gridSymbolOnce.Do(func() {
		schedule := map[uint64]string{}
		for bit := uint64(0); bit <= 9; bit++ {
			schedule[bit] = fmt.Sprintf("SCHED%d", bit+1)
		}
		schedule[10] = "SCHED12"
		for bit := uint64(11); bit <= 19; bit++ {
			schedule[bit] = fmt.Sprintf("SCHED%d", bit+2)
		}
		for bit := uint64(21); bit <= 31; bit++ {
			schedule[bit] = fmt.Sprintf("SCHED%d", bit+1)
		}
		gridSymbolValue = gridSymbols{
			enable:    map[uint64]string{0: "ENABLE"},
			enabled:   map[uint64]string{0: "ENABLED"},
			readOnly:  map[uint64]string{0: "READWRITE", 1: "READONLY"},
			deptRef:   map[uint64]string{1: "WMax", 2: "VArMax", 3: "VArAval"},
			wattDept:  map[uint64]string{1: "%WMax", 2: "%WAval"},
			edge:      map[uint64]string{0: "EDGE", 1: "CENTER"},
			sigType:   map[uint64]string{0: "UNKNOWN", 1: "ABSOLUTE", 2: "RELATIVE", 3: "MULTIPLIER", 4: "LEVEL"},
			cease:     map[uint64]string{1: "CEASE_TO_ENERGIZE"},
			access:    map[uint64]string{0: "RW", 1: "R"},
			ena:       map[uint64]string{0: "DISABLED", 1: "ENABLED"},
			adopt:     map[uint64]string{0: "IN_PROGRESS", 1: "COMPLETED", 2: "FAILED"},
			schedule:  schedule,
			interval:  map[uint64]string{0: "ONETIME", 1: "DAILY", 2: "WEEKLY", 3: "MONTHLY", 4: "WEEKDAY", 5: "HOLIDAY", 6: "WEEKEND", 7: "YEARLY"},
			axis:      map[uint64]string{0: "UNSET", 1: "TIME", 2: "TEMP", 3: "PRICE", 99: "OTHER"},
			dependent: map[uint64]string{0: "UNSET", 1: "WMax", 2: "RSRVD2", 3: "PF", 4: "RSRVD4", 5: "WATT_PRICE", 6: "VAR_PRICE", 7: "RSRVD7", 8: "VOLT_VAR_ARRAY", 9: "WChaGra", 10: "WDisChaGra", 11: "VArAval", 12: "Schedule", 99: "OTHER"},
		}
	})
	return gridSymbolValue
}

func gridColumnPoint(model uint16, column gridColumn) sunSpecPointDefinition {
	size := column.size
	if size == 0 {
		size = 1
	}
	known := uint64(0)
	if column.pointType == TypeBitfield16 || column.pointType == TypeBitfield32 {
		known = sunSpecKnownMask(column.symbols)
	}
	field := ""
	if column.name != "ID" && column.name != "L" {
		field = fmt.Sprintf("sunspec.grid.%d.%s", model, column.name)
	}
	return sunSpecPoint(column.name, field, column.pointType, size, column.unit, column.scale, false, cloneSunSpecSymbols(column.symbols), known)
}

func gridColumns(model uint16, columns []gridColumn) []sunSpecPointDefinition {
	points := make([]sunSpecPointDefinition, len(columns))
	for index, column := range columns {
		points[index] = gridColumnPoint(model, column)
	}
	return points
}

func gridNumberedPairs(count int, leftName, rightName string, left, right gridColumn) []gridColumn {
	out := make([]gridColumn, 0, count*2)
	for index := 1; index <= count; index++ {
		leftPoint, rightPoint := left, right
		leftPoint.name = fmt.Sprintf("%s%d", leftName, index)
		rightPoint.name = fmt.Sprintf("%s%d", rightName, index)
		out = append(out, leftPoint, rightPoint)
	}
	return out
}

func gridCurveLead(symbols gridSymbols) []gridColumn {
	return []gridColumn{
		{name: "ID", pointType: TypeUint16, size: 1},
		{name: "L", pointType: TypeUint16, size: 1},
		{name: "ActCrv", pointType: TypeUint16, size: 1},
		{name: "ModEna", pointType: TypeBitfield16, size: 1, symbols: symbols.enabled},
		{name: "WinTms", pointType: TypeUint16, size: 1, unit: "Secs"},
		{name: "RvrtTms", pointType: TypeUint16, size: 1, unit: "Secs"},
		{name: "RmpTms", pointType: TypeUint16, size: 1, unit: "Secs"},
		{name: "NCrv", pointType: TypeUint16, size: 1},
		{name: "NPt", pointType: TypeUint16, size: 1},
	}
}

func gridScale(name, unit string) gridColumn {
	return gridColumn{name: name, pointType: TypeScaleFactor, size: 1, unit: unit}
}

func gridSupportFixedDefinitions(revision SchemaRevision, definitions []sunSpecModelDefinition) ([]sunSpecModelDefinition, error) {
	symbols := gridSymbolSet()
	fixed := []struct {
		id, length uint16
		columns    []gridColumn
	}{
		{125, 8, []gridColumn{
			{name: "ID", pointType: TypeUint16, size: 1}, {name: "L", pointType: TypeUint16, size: 1},
			{name: "ModEna", pointType: TypeBitfield16, size: 1, symbols: symbols.enable},
			{name: "SigType", pointType: TypeEnum16, size: 1, symbols: symbols.sigType},
			{name: "Sig", pointType: TypeInt16, size: 1, scale: "Sig_SF"},
			{name: "WinTms", pointType: TypeUint16, size: 1, unit: "Secs"},
			{name: "RvtTms", pointType: TypeUint16, size: 1, unit: "Secs"},
			{name: "RmpTms", pointType: TypeUint16, size: 1, unit: "Secs"},
			gridScale("Sig_SF", ""), {name: "Pad", pointType: TypePad, size: 1},
		}},
		{127, 10, []gridColumn{
			{name: "ID", pointType: TypeUint16, size: 1}, {name: "L", pointType: TypeUint16, size: 1},
			{name: "WGra", pointType: TypeUint16, size: 1, unit: "% PM/Hz", scale: "WGra_SF"},
			{name: "HzStr", pointType: TypeInt16, size: 1, unit: "Hz", scale: "HzStrStop_SF"},
			{name: "HzStop", pointType: TypeInt16, size: 1, unit: "Hz", scale: "HzStrStop_SF"},
			{name: "HysEna", pointType: TypeBitfield16, size: 1, symbols: symbols.enabled},
			{name: "ModEna", pointType: TypeBitfield16, size: 1, symbols: symbols.enabled},
			{name: "HzStopWGra", pointType: TypeUint16, size: 1, unit: "% WMax/min", scale: "RmpIncDec_SF"},
			gridScale("WGra_SF", ""), gridScale("HzStrStop_SF", ""), gridScale("RmpIncDec_SF", ""),
			{name: "Pad", pointType: TypePad, size: 1},
		}},
		{128, 14, []gridColumn{
			{name: "ID", pointType: TypeUint16, size: 1}, {name: "L", pointType: TypeUint16, size: 1},
			{name: "ArGraMod", pointType: TypeEnum16, size: 1, symbols: symbols.edge},
			{name: "ArGraSag", pointType: TypeUint16, size: 1, unit: "%ARtg/%dV", scale: "ArGra_SF"},
			{name: "ArGraSwell", pointType: TypeUint16, size: 1, unit: "%ARtg/%dV", scale: "ArGra_SF"},
			{name: "ModEna", pointType: TypeBitfield16, size: 1, symbols: symbols.enabled},
			{name: "FilTms", pointType: TypeUint16, size: 1, unit: "Secs"},
			{name: "DbVMin", pointType: TypeUint16, size: 1, unit: "% VRef", scale: "VRefPct_SF"},
			{name: "DbVMax", pointType: TypeUint16, size: 1, unit: "% VRef", scale: "VRefPct_SF"},
			{name: "BlkZnV", pointType: TypeUint16, size: 1, unit: "% VRef", scale: "VRefPct_SF"},
			{name: "HysBlkZnV", pointType: TypeUint16, size: 1, unit: "% VRef", scale: "VRefPct_SF"},
			{name: "BlkZnTmms", pointType: TypeUint16, size: 1, unit: "mSecs"},
			{name: "HoldTmms", pointType: TypeUint16, size: 1, unit: "mSecs"},
			gridScale("ArGra_SF", ""), gridScale("VRefPct_SF", ""),
			{name: "Pad", pointType: TypePad, size: 1},
		}},
		{145, 8, []gridColumn{
			{name: "ID", pointType: TypeUint16, size: 1}, {name: "L", pointType: TypeUint16, size: 1},
			{name: "NomRmpUpRte", pointType: TypeUint16, size: 1, unit: "Pct", scale: "Rmp_SF"},
			{name: "NomRmpDnRte", pointType: TypeUint16, size: 1, unit: "Pct", scale: "Rmp_SF"},
			{name: "EmgRmpUpRte", pointType: TypeUint16, size: 1, unit: "Pct", scale: "Rmp_SF"},
			{name: "EmgRmpDnRte", pointType: TypeUint16, size: 1, unit: "Pct", scale: "Rmp_SF"},
			{name: "ConnRmpUpRte", pointType: TypeUint16, size: 1, unit: "Pct", scale: "Rmp_SF"},
			{name: "ConnRmpDnRte", pointType: TypeUint16, size: 1, unit: "Pct", scale: "Rmp_SF"},
			{name: "AGra", pointType: TypeUint16, size: 1, unit: "Pct", scale: "Rmp_SF"},
			gridScale("Rmp_SF", ""),
		}},
	}
	var err error
	for _, model := range fixed {
		definitions, err = appendSunSpecDefinition(definitions, revision, model.id, model.length, TopologyNone, false, gridColumns(model.id, model.columns))
		if err != nil {
			return nil, err
		}
	}
	return definitions, nil
}

func gridRepeatModels() []gridRepeatSpec {
	time := func(id uint16, second string, cease bool) gridRepeatSpec {
		return gridRepeatSpec{id: id, base: 10, stride: 50, countOffset: 7, group: "curve", countName: "NCrv", second: second, body: gridBodyTime, cease: cease}
	}
	return []gridRepeatSpec{
		{id: 126, base: 10, stride: 54, countOffset: 7, group: "curve", countName: "NCrv", body: gridBodyVoltVar},
		time(129, "V", false), time(130, "V", false),
		{id: 131, base: 10, stride: 54, countOffset: 7, group: "curve", countName: "NCrv", body: gridBodyWattPF},
		{id: 132, base: 10, stride: 54, countOffset: 7, group: "curve", countName: "NCrv", body: gridBodyVoltWatt},
		{id: 133, base: 6, stride: 60, countOffset: 5, group: "repeating", countName: "NSchd", body: gridBodySchedule},
		{id: 134, base: 10, stride: 58, countOffset: 7, group: "curve", countName: "NCrv", body: gridBodyFreqWatt},
		time(135, "Hz", false), time(136, "Hz", false), time(137, "V", false), time(138, "V", false),
		time(139, "V", true), time(140, "V", true), time(141, "Hz", false), time(142, "Hz", false),
		time(143, "Hz", true), time(144, "Hz", true),
	}
}

func gridSupportRepeatKeyCount() int {
	total := 0
	for _, spec := range gridRepeatModels() {
		total += (65534-int(spec.base))/int(spec.stride) + 1
	}
	return total
}

func gridSupportRepeatKeys(revision SchemaRevision) []DecoderKey {
	keys := make([]DecoderKey, 0, gridSupportRepeatKeyCount())
	for _, spec := range gridRepeatModels() {
		span := (65534 - int(spec.base)) / int(spec.stride)
		for count := 0; count <= span; count++ {
			keys = append(keys, DecoderKey{ModelID: spec.id, ModelLength: uint16(int(spec.base) + count*int(spec.stride)), SchemaRevision: revision})
		}
	}
	return keys
}

func gridRepeatByID(id uint16) (gridRepeatSpec, bool) {
	for _, spec := range gridRepeatModels() {
		if spec.id == id {
			return spec, true
		}
	}
	return gridRepeatSpec{}, false
}

func gridTimeHeader(secondScale string, cease bool, symbols gridSymbols) []gridColumn {
	columns := append(gridCurveLead(symbols), gridScale("Tms_SF", ""), gridScale(secondScale, ""))
	if cease {
		return append(columns, gridColumn{name: "CrvType", pointType: TypeEnum16, size: 1, symbols: symbols.cease})
	}
	return append(columns, gridColumn{name: "Pad", pointType: TypePad, size: 1})
}

func gridTimeRepeat(secondName, secondUnit, secondScale string, symbols gridSymbols) []gridColumn {
	columns := []gridColumn{{name: "ActPt", pointType: TypeUint16, size: 1}}
	columns = append(columns, gridNumberedPairs(20, "Tms", secondName,
		gridColumn{pointType: TypeUint16, size: 1, unit: "Secs", scale: "Tms_SF"},
		gridColumn{pointType: TypeUint16, size: 1, unit: secondUnit, scale: secondScale},
	)...)
	return append(columns,
		gridColumn{name: "CrvNam", pointType: TypeString, size: 8},
		gridColumn{name: "ReadOnly", pointType: TypeEnum16, size: 1, symbols: symbols.readOnly},
	)
}

func gridVoltVarColumns(symbols gridSymbols) (header, repeat []gridColumn) {
	header = append(gridCurveLead(symbols), gridScale("V_SF", ""), gridScale("DeptRef_SF", ""), gridScale("RmpIncDec_SF", ""))
	repeat = []gridColumn{
		{name: "ActPt", pointType: TypeUint16, size: 1},
		{name: "DeptRef", pointType: TypeEnum16, size: 1, symbols: symbols.deptRef},
	}
	repeat = append(repeat, gridNumberedPairs(20, "V", "VAr",
		gridColumn{pointType: TypeUint16, size: 1, unit: "% VRef", scale: "V_SF"},
		gridColumn{pointType: TypeInt16, size: 1, scale: "DeptRef_SF"},
	)...)
	repeat = append(repeat,
		gridColumn{name: "CrvNam", pointType: TypeString, size: 8},
		gridColumn{name: "RmpTms", pointType: TypeUint16, size: 1, unit: "Secs"},
		gridColumn{name: "RmpDecTmm", pointType: TypeUint16, size: 1, unit: "% ref_value/min", scale: "RmpIncDec_SF"},
		gridColumn{name: "RmpIncTmm", pointType: TypeUint16, size: 1, unit: "% ref_value/min", scale: "RmpIncDec_SF"},
		gridColumn{name: "ReadOnly", pointType: TypeEnum16, size: 1, symbols: symbols.readOnly},
	)
	return header, repeat
}

func gridWattPFColumns(symbols gridSymbols) (header, repeat []gridColumn) {
	header = append(gridCurveLead(symbols), gridScale("W_SF", ""), gridScale("PF_SF", ""), gridScale("RmpIncDec_SF", ""))
	repeat = []gridColumn{{name: "ActPt", pointType: TypeUint16, size: 1}}
	repeat = append(repeat, gridNumberedPairs(20, "W", "PF",
		gridColumn{pointType: TypeInt16, size: 1, unit: "% WMax", scale: "W_SF"},
		gridColumn{pointType: TypeInt16, size: 1, unit: "cos()", scale: "PF_SF"},
	)...)
	repeat = append(repeat,
		gridColumn{name: "CrvNam", pointType: TypeString, size: 8},
		gridColumn{name: "RmpPT1Tms", pointType: TypeUint16, size: 1, unit: "Secs"},
		gridColumn{name: "RmpDecTmm", pointType: TypeUint16, size: 1, unit: "% PF/min", scale: "RmpIncDec_SF"},
		gridColumn{name: "RmpIncTmm", pointType: TypeUint16, size: 1, unit: "% PF/min", scale: "RmpIncDec_SF"},
		gridColumn{name: "ReadOnly", pointType: TypeEnum16, size: 1, symbols: symbols.readOnly},
		gridColumn{name: "Pad", pointType: TypePad, size: 1},
	)
	return header, repeat
}

func gridVoltWattColumns(symbols gridSymbols) (header, repeat []gridColumn) {
	header = append(gridCurveLead(symbols), gridScale("V_SF", ""), gridScale("DeptRef_SF", ""), gridScale("RmpIncDec_SF", ""))
	repeat = []gridColumn{
		{name: "ActPt", pointType: TypeUint16, size: 1},
		{name: "DeptRef", pointType: TypeEnum16, size: 1, symbols: symbols.wattDept},
	}
	repeat = append(repeat, gridNumberedPairs(20, "V", "W",
		gridColumn{pointType: TypeUint16, size: 1, unit: "% VRef", scale: "V_SF"},
		gridColumn{pointType: TypeInt16, size: 1, unit: "% VRef", scale: "DeptRef_SF"},
	)...)
	repeat = append(repeat,
		gridColumn{name: "CrvNam", pointType: TypeString, size: 8},
		gridColumn{name: "RmpPt1Tms", pointType: TypeUint16, size: 1, unit: "Secs"},
		gridColumn{name: "RmpDecTmm", pointType: TypeUint16, size: 1, unit: "% WMax/min", scale: "RmpIncDec_SF"},
		gridColumn{name: "RmpIncTmm", pointType: TypeUint16, size: 1, unit: "% WMax/min", scale: "RmpIncDec_SF"},
		gridColumn{name: "ReadOnly", pointType: TypeEnum16, size: 1, symbols: symbols.readOnly},
	)
	return header, repeat
}

func gridScheduleColumns(symbols gridSymbols) (header, repeat []gridColumn) {
	header = []gridColumn{
		{name: "ID", pointType: TypeUint16, size: 1}, {name: "L", pointType: TypeUint16, size: 1},
		{name: "ActSchd", pointType: TypeBitfield32, size: 2, symbols: symbols.schedule},
		{name: "ModEna", pointType: TypeBitfield16, size: 1, symbols: symbols.enabled},
		{name: "NSchd", pointType: TypeUint16, size: 1},
		{name: "NPts", pointType: TypeUint16, size: 1},
		{name: "Pad", pointType: TypePad, size: 1},
	}
	repeat = []gridColumn{
		{name: "ActPts", pointType: TypeUint16, size: 1},
		{name: "StrTms", pointType: TypeUint32, size: 2, unit: "Secs"},
		{name: "RepPer", pointType: TypeUint16, size: 1},
		{name: "IntvTyp", pointType: TypeEnum16, size: 1, symbols: symbols.interval},
		{name: "XTyp", pointType: TypeEnum16, size: 1, symbols: symbols.axis},
		gridScale("X_SF", ""),
		{name: "YTyp", pointType: TypeEnum16, size: 1, symbols: symbols.dependent},
		gridScale("Y_SF", ""),
	}
	repeat = append(repeat, gridNumberedPairs(10, "X", "Y",
		gridColumn{pointType: TypeInt32, size: 2, scale: "X_SF"},
		gridColumn{pointType: TypeInt32, size: 2, scale: "Y_SF"},
	)...)
	repeat = append(repeat,
		gridColumn{name: "Nam", pointType: TypeString, size: 8},
		gridColumn{name: "WinTms", pointType: TypeUint16, size: 1, unit: "Secs"},
		gridColumn{name: "RmpTms", pointType: TypeUint16, size: 1, unit: "Secs"},
		gridColumn{name: "ActIndx", pointType: TypeUint16, size: 1},
	)
	return header, repeat
}

func gridFreqWattColumns(symbols gridSymbols) (header, repeat []gridColumn) {
	header = append(gridCurveLead(symbols), gridScale("Hz_SF", "SF"), gridScale("W_SF", "SF"), gridScale("RmpIncDec_SF", "SF"))
	repeat = []gridColumn{{name: "ActPt", pointType: TypeUint16, size: 1}}
	repeat = append(repeat, gridNumberedPairs(20, "Hz", "W",
		gridColumn{pointType: TypeUint16, size: 1, unit: "Hz", scale: "Hz_SF"},
		gridColumn{pointType: TypeInt16, size: 1, unit: "% WRef", scale: "W_SF"},
	)...)
	repeat = append(repeat,
		gridColumn{name: "CrvNam", pointType: TypeString, size: 8},
		gridColumn{name: "RmpPT1Tms", pointType: TypeUint16, size: 1, unit: "Secs"},
		gridColumn{name: "RmpDecTmm", pointType: TypeUint16, size: 1, unit: "% WMax/min", scale: "RmpIncDec_SF"},
		gridColumn{name: "RmpIncTmm", pointType: TypeUint16, size: 1, unit: "% WMax/min", scale: "RmpIncDec_SF"},
		gridColumn{name: "RmpRsUp", pointType: TypeUint16, size: 1, unit: "% WMax/min", scale: "RmpIncDec_SF"},
		gridColumn{name: "SnptW", pointType: TypeBitfield16, size: 1},
		gridColumn{name: "WRef", pointType: TypeUint16, size: 1, unit: "W", scale: "W_SF"},
		gridColumn{name: "WRefStrHz", pointType: TypeUint16, size: 1, unit: "Hz", scale: "Hz_SF"},
		gridColumn{name: "WRefStopHz", pointType: TypeUint16, size: 1, unit: "Hz", scale: "Hz_SF"},
		gridColumn{name: "ReadOnly", pointType: TypeEnum16, size: 1, symbols: symbols.readOnly},
	)
	return header, repeat
}

func gridRepeatColumns(spec gridRepeatSpec) (header, repeat []gridColumn) {
	symbols := gridSymbolSet()
	switch spec.body {
	case gridBodyVoltVar:
		return gridVoltVarColumns(symbols)
	case gridBodyWattPF:
		return gridWattPFColumns(symbols)
	case gridBodyVoltWatt:
		return gridVoltWattColumns(symbols)
	case gridBodySchedule:
		return gridScheduleColumns(symbols)
	case gridBodyFreqWatt:
		return gridFreqWattColumns(symbols)
	default:
		secondScale := "V_SF"
		secondUnit := "% VRef"
		if spec.second == "Hz" {
			secondScale = "Hz_SF"
			secondUnit = "Hz"
		}
		return gridTimeHeader(secondScale, spec.cease, symbols), gridTimeRepeat(spec.second, secondUnit, secondScale, symbols)
	}
}

func gridSupportSunSpecDefinition(revision SchemaRevision, id, length uint16) (sunSpecModelDefinition, error) {
	if id == 708 || id == 709 {
		return derTripCurveSunSpecDefinition(revision, id, length)
	}
	if revision != ModelsRevisionV1 {
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec grid model is unsupported")
	}
	spec, ok := gridRepeatByID(id)
	if !ok || length < spec.base || int(length-spec.base)%int(spec.stride) != 0 {
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec grid model is unsupported")
	}
	count := int(length-spec.base) / int(spec.stride)
	header, repeat := gridRepeatColumns(spec)
	points := gridColumns(id, header)
	repeated := gridColumns(id, repeat)
	for index := 1; index <= count; index++ {
		for _, point := range repeated {
			point.groupID = spec.group
			point.repeatIndex = uint16(index)
			point.repeated = true
			points = append(points, point)
		}
	}
	definitions, err := appendSunSpecDefinition(nil, revision, id, length, TopologyNone, false, points)
	if err != nil {
		return sunSpecModelDefinition{}, err
	}
	definition := definitions[0]
	countOffset, want := spec.countOffset, uint16(count)
	definition.geometry = func(words []uint16) bool {
		return len(words) > int(countOffset) && words[countOffset] == want
	}
	return definition, nil
}

func buildDerTripCurveLengths(pointBlock uint16) map[uint16]struct{} {
	const maxLength = 65534
	out := map[uint16]struct{}{7: {}}
	for points := uint32(0); ; points++ {
		stride := uint32(4) + uint32(pointBlock)*points
		if stride > maxLength-7 {
			break
		}
		for curves := uint32(1); ; curves++ {
			total := uint32(7) + curves*stride
			if total > maxLength {
				break
			}
			out[uint16(total)] = struct{}{}
		}
	}
	return out
}

func derTripCurveLengthSet(pointBlock uint16) map[uint16]struct{} {
	derTripOnce.Do(func() {
		derTripHigh = buildDerTripCurveLengths(9)
		derTripLow = buildDerTripCurveLengths(12)
	})
	if pointBlock == 12 {
		return derTripLow
	}
	return derTripHigh
}

func derTripCurveKeys(revision SchemaRevision, modelID, pointBlock uint16) []DecoderKey {
	lengths := derTripCurveLengthSet(pointBlock)
	keys := make([]DecoderKey, 0, len(lengths))
	for length := range lengths {
		keys = append(keys, DecoderKey{ModelID: modelID, ModelLength: length, SchemaRevision: revision})
	}
	return keys
}

type derTripShape struct {
	id, pointBlock, valueSize        uint16
	valueName, valueUnit, valueScale string
	valueType                        PointType
}

func derTripShapeFor(id uint16) (derTripShape, bool) {
	switch id {
	case 708:
		return derTripShape{id: 708, pointBlock: 9, valueSize: 1, valueName: "V", valueUnit: "VNomPct", valueScale: "V_SF", valueType: TypeUint16}, true
	case 709:
		return derTripShape{id: 709, pointBlock: 12, valueSize: 2, valueName: "Hz", valueUnit: "Hz", valueScale: "Hz_SF", valueType: TypeUint32}, true
	default:
		return derTripShape{}, false
	}
}

func derTripCurveSunSpecDefinition(revision SchemaRevision, id, length uint16) (sunSpecModelDefinition, error) {
	shape, ok := derTripShapeFor(id)
	if !ok || revision != ModelsRevisionV2 {
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec trip curve is unsupported")
	}
	if _, admitted := derTripCurveLengthSet(shape.pointBlock)[length]; !admitted {
		return sunSpecModelDefinition{}, fmt.Errorf("SunSpec trip curve length is unsupported")
	}
	return sunSpecModelDefinition{
		key: DecoderKey{ModelID: id, ModelLength: length, SchemaRevision: revision},
		layout: func(words []uint16) ([]sunSpecPointDefinition, bool) {
			return derTripCurvePoints(shape, length, words)
		},
	}, nil
}

func derTripPoint(model uint16, name, field, unit, scale string, pointType PointType, size uint16, symbols map[uint64]string) sunSpecPointDefinition {
	fieldID := ""
	if field != "" {
		fieldID = fmt.Sprintf("sunspec.der.v2.%d.%s", model, field)
	}
	return sunSpecPoint(name, fieldID, pointType, size, unit, scale, false, cloneSunSpecSymbols(symbols), sunSpecKnownMask(symbols))
}

func derTripCurvePoints(shape derTripShape, length uint16, words []uint16) ([]sunSpecPointDefinition, bool) {
	if len(words) != int(length)+2 || len(words) < 9 {
		return nil, false
	}
	pointsPerCurve := uint64(words[5])
	curves := uint64(words[6])
	if curves == 0 {
		if length != 7 {
			return nil, false
		}
	} else {
		stride := uint64(4) + uint64(shape.pointBlock)*pointsPerCurve
		total := uint64(7) + curves*stride
		if total != uint64(length) || total > 65534 {
			return nil, false
		}
	}
	symbols := gridSymbolSet()
	points := []sunSpecPointDefinition{
		derTripPoint(shape.id, "ID", "", "", "", TypeUint16, 1, nil),
		derTripPoint(shape.id, "L", "", "", "", TypeUint16, 1, nil),
		derTripPoint(shape.id, "Ena", "Ena", "", "", TypeEnum16, 1, symbols.ena),
		derTripPoint(shape.id, "AdptCrvReq", "AdptCrvReq", "", "", TypeUint16, 1, nil),
		derTripPoint(shape.id, "AdptCrvRslt", "AdptCrvRslt", "", "", TypeEnum16, 1, symbols.adopt),
		derTripPoint(shape.id, "NPt", "NPt", "", "", TypeUint16, 1, nil),
		derTripPoint(shape.id, "NCrvSet", "NCrvSet", "", "", TypeUint16, 1, nil),
		derTripPoint(shape.id, shape.valueScale, shape.valueScale, "", "", TypeScaleFactor, 1, nil),
		derTripPoint(shape.id, "Tms_SF", "Tms_SF", "", "", TypeScaleFactor, 1, nil),
	}
	for curve := uint16(1); curve <= uint16(curves); curve++ {
		readOnly := derTripPoint(shape.id, "ReadOnly", "Crv.ReadOnly", "", "", TypeEnum16, 1, symbols.access)
		readOnly.groupID, readOnly.repeatIndex, readOnly.repeated = "curve", curve, true
		points = append(points, readOnly)
		for _, kind := range []string{"MustTrip", "MayTrip", "MomCess"} {
			active := derTripPoint(shape.id, kind+".ActPt", "Crv."+kind+".ActPt", "", "", TypeUint16, 1, nil)
			active.groupID, active.repeatIndex, active.repeated = "curve", curve, true
			points = append(points, active)
			for point := uint16(1); point <= uint16(pointsPerCurve); point++ {
				group := fmt.Sprintf("curve.%d.%s", curve, kind)
				sample := derTripPoint(shape.id, kind+"."+shape.valueName, "Crv."+kind+".Pt."+shape.valueName, shape.valueUnit, shape.valueScale, shape.valueType, shape.valueSize, nil)
				sample.groupID, sample.repeatIndex, sample.repeated = group, point, true
				elapsed := derTripPoint(shape.id, kind+".Tms", "Crv."+kind+".Pt.Tms", "Secs", "Tms_SF", TypeUint32, 2, nil)
				elapsed.groupID, elapsed.repeatIndex, elapsed.repeated = group, point, true
				points = append(points, sample, elapsed)
			}
		}
	}
	return assignSunSpecPointOffsets(points, length)
}

func assignSunSpecPointOffsets(points []sunSpecPointDefinition, length uint16) ([]sunSpecPointDefinition, bool) {
	var extent uint32
	for index := range points {
		if points[index].size == 0 || extent > 65535 || extent+uint32(points[index].size) > 65536 {
			return nil, false
		}
		points[index].offset = uint16(extent)
		extent += uint32(points[index].size)
	}
	if extent != uint32(length)+2 {
		return nil, false
	}
	return points, true
}
