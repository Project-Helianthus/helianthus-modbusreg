package sunspec

import (
	"fmt"
	"math"
	"strings"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// PointWrite is a native writable point of one walked occurrence. It uses the
// span and scale from that snapshot; it grants no runtime admission. The zero
// value does not encode a request.
type PointWrite struct {
	point   sunSpecPointDefinition
	path    FactPath
	address uint16
	scale   *Value
}

// Point identifies the point by its decoded field ID, group and repeat index.
// Model 707 instead uses NestedPath for its repeat identity.
func (p PointWrite) Point() (fieldID, groupID string, repeatIndex uint16) {
	if len(p.path.segments) != 0 {
		return p.point.fieldID, "", 0
	}
	return p.point.fieldID, p.point.groupID, p.point.repeatIndex
}

// NestedPath returns the point's structural path for model 707.
func (p PointWrite) NestedPath() (FactPath, bool) { return p.path, len(p.path.segments) != 0 }

// PointType is the point's native SunSpec type.
func (p PointWrite) PointType() PointType { return p.point.pointType }

// Unit is the native unit expected by Encode, or empty when unitless.
func (p PointWrite) Unit() string { return p.point.unit }

// PointWriteRequest is one contiguous FC16 request. Address is the zero-based
// PDU register address. Words is owned by the caller of Encode.
type PointWriteRequest struct {
	Function  modbusreg.FunctionCode
	Address   uint16
	WordCount uint16
	Words     []uint16
}

// PointWrites lists the native write operations of the 1-based occurrence in a
// consistent terminal snapshot. A known read-only point, a read-only curve
// instance or a scaled point without a usable scale offers no write. Model 707
// uses its validated structural layout, not a decoder key.
func PointWrites(r DecoderRegistry, snapshot ChainSnapshot, ordinal uint32) ([]PointWrite, error) {
	if ordinal == 0 || uint64(ordinal) > uint64(len(snapshot.occurrences)) {
		return nil, fmt.Errorf("SunSpec write occurrence is absent")
	}
	if _, ok := snapshot.WireChain(); !ok {
		return nil, fmt.Errorf("SunSpec write requires a consistent terminal chain")
	}
	occurrence := snapshot.occurrences[ordinal-1]
	if occurrence.SchemaRevision != r.revision || !sunSpecWriteSpansMatch(snapshot, occurrence) {
		return nil, fmt.Errorf("SunSpec write occurrence contradicts registry or source spans")
	}
	points, paths, err := r.pointWriteDefinitions(occurrence)
	if err != nil {
		return nil, err
	}
	return sunSpecOccurrenceWrites(occurrence, points, paths), nil
}

func sunSpecWriteSpansMatch(snapshot ChainSnapshot, occurrence Occurrence) bool {
	addresses := make(map[uint64]uint16, len(snapshot.sources))
	var first modbusreg.LogicalViewRecord
	for i, source := range snapshot.sources {
		record := source.Record()
		if i == 0 {
			first = record
		}
		if record.RequestedFunction != modbusreg.FunctionReadHoldingRegisters || !SameProvenance(first, record) {
			return false
		}
		addresses[record.LogicalViewID] = record.LogicalOffset
	}
	address := uint32(occurrence.HeaderOffset)
	for _, span := range occurrence.spans {
		start, ok := addresses[span.LogicalViewID]
		if !ok || uint32(start) != address {
			return false
		}
		address += uint32(span.WordCount)
	}
	return address <= 65536
}

func (r DecoderRegistry) pointWriteDefinitions(o Occurrence) ([]sunSpecPointDefinition, map[uint16]FactPath, error) {
	id := o.ModelID()
	if id == 707 {
		return sunSpecStructuralWriteDefinitions(o)
	}
	if (id < 120 || id > 145) && id != 703 && id != 708 && id != 709 {
		return nil, nil, nil
	}
	key, ok := o.DecoderKey()
	if !ok || o.Disposition != ChainDispositionAdmitted || key != (DecoderKey{id, o.ModelLength(), o.SchemaRevision}) {
		return nil, nil, fmt.Errorf("SunSpec write occurrence lacks an exact admitted decoder key")
	}
	definition, ok := r.definition(key)
	if !ok {
		return nil, nil, fmt.Errorf("SunSpec write decoder key is unsupported")
	}
	if definition.layout != nil {
		points, valid := definition.layout(o.words)
		if !valid {
			return nil, nil, fmt.Errorf("SunSpec write occurrence has invalid geometry")
		}
		return points, nil, nil
	}
	if definition.geometry != nil && !definition.geometry(o.words) {
		return nil, nil, fmt.Errorf("SunSpec write occurrence has invalid geometry")
	}
	return definition.points, nil, nil
}

func sunSpecStructuralWriteDefinitions(o Occurrence) ([]sunSpecPointDefinition, map[uint16]FactPath, error) {
	candidate := o.structuralCandidate
	if candidate == nil || candidate.modelID != 707 || o.SchemaRevision != ModelsRevisionV2 || o.Disposition != ChainDispositionUnknownModel || o.decoderKey != nil || candidate.layout.key != (sunSpecNestedTemplateKey{revision: ModelsRevisionV2, modelID: 707}) {
		return nil, nil, fmt.Errorf("SunSpec write occurrence lacks a validated structural layout")
	}
	entries := candidate.layout.Entries()
	points := make([]sunSpecPointDefinition, 0, len(entries))
	paths := make(map[uint16]FactPath, len(entries))
	for _, entry := range entries {
		point, _, ok := sunSpecStructuralEntryDefinition(o.words, entry)
		if !ok {
			return nil, nil, fmt.Errorf("SunSpec write structural span is invalid")
		}
		point.offset = uint16(entry.SourceRange().OccurrenceOffset())
		path := entry.Path()
		point.repeated = len(path.segments) != 0 && path.segments[0].Name == "Crv"
		if point.repeated {
			point.repeatIndex = uint16(path.segments[0].Index)
		}
		points = append(points, point)
		paths[point.offset] = path
	}
	return points, paths, nil
}

func sunSpecOccurrenceWrites(o Occurrence, points []sunSpecPointDefinition, paths map[uint16]FactPath) []PointWrite {
	scales := make(map[sunSpecScaleKey]Value)
	writableCurve := make(map[uint16]bool)
	for _, p := range points {
		if p.pointType == TypeScaleFactor {
			scales[sunSpecScaleKey{name: p.name, group: p.groupID, index: p.repeatIndex}] = decodeSunSpecValue(p, o.words[p.offset:int(p.offset)+int(p.size)], nil)
		}
		if p.name == "ReadOnly" && p.repeated {
			writableCurve[p.repeatIndex] = o.words[p.offset] == 0
		}
	}
	var result []PointWrite
	var derCurve uint16
	for _, p := range points {
		if p.name == "ReadOnly" && o.ModelID() >= 707 {
			// DER nested children follow their owning curve's access word.
			derCurve = p.repeatIndex
		}
		if !sunSpecPointHasWriteAccess(o.ModelID(), p) {
			continue
		}
		if p.repeated && p.groupID != "repeating" {
			index := p.repeatIndex
			if o.ModelID() >= 707 {
				index = derCurve
			}
			if !writableCurve[index] {
				continue
			}
		}
		var scale *Value
		if p.scaleFactor != "" {
			value, ok := sunSpecScaleValue(scales, p)
			if !ok || value.State() != ValueValid {
				continue
			}
			scale = &value
		}
		if o.ModelID() == 703 {
			p = enterServiceWritePoint(p)
		}
		result = append(result, PointWrite{point: p, path: paths[p.offset], address: o.HeaderOffset + p.offset, scale: scale})
	}
	return result
}

func enterServiceWritePoint(p sunSpecPointDefinition) sunSpecPointDefinition {
	switch p.name {
	case "ES":
		p.symbols = gridSymbolSet().ena
	case "ESDlyTms", "ESRndTms", "ESRmpTms":
		p.unit = "Secs"
	}
	return p
}

// Encode builds, but never sends, one FC16 request. Scaled points take an
// exact Decimal in Unit; a nonrepresentable value is rejected, not rounded.
// Unscaled points take int16 (int16 or sunssf), int32, uint16 (uint16, enum16
// or bitfield16), uint32 (uint32 or bitfield32), or string, by PointType.
// A rejected encoding returns the zero request.
func (p PointWrite) Encode(native any) (PointWriteRequest, error) {
	def := p.point
	if def.size == 0 {
		return PointWriteRequest{}, fmt.Errorf("SunSpec write point is absent")
	}
	words, ok := p.encodeWords(native)
	if !ok || decodeSunSpecValue(def, words, p.scale).State() != ValueValid {
		return PointWriteRequest{}, fmt.Errorf("value cannot be encoded for SunSpec point %s", def.name)
	}
	return PointWriteRequest{Function: 16, Address: p.address, WordCount: def.size, Words: words}, nil
}

func (p PointWrite) encodeWords(native any) ([]uint16, bool) {
	def := p.point
	if def.pointType == TypeString {
		text, ok := native.(string)
		if !ok || strings.ContainsRune(text, 0) || len(text) > int(def.size)*2 {
			return nil, false
		}
		words := make([]uint16, def.size)
		if text == "" {
			words[0] = 0x0080 // Encoded empty string, not the all-zero sentinel.
		}
		for i := 0; i < len(text); i++ {
			words[i/2] |= uint16(text[i]) << (8 * (1 - i%2))
		}
		return words, true
	}
	var number int64
	var valid bool
	if p.scale != nil {
		if decimal, ok := native.(Decimal); ok {
			number, valid = sunSpecWriteCoefficient(decimal, int16(p.scale.signed))
		}
	} else {
		number, valid = sunSpecWriteInteger(def, native)
	}
	if !valid || !sunSpecWriteIntegerFits(def.pointType, number) {
		return nil, false
	}
	// SunSpec Device Information Model, Data Encoding: most significant register
	// first, and most significant byte first within each register.
	if def.size == 2 {
		return []uint16{uint16(uint32(number) >> 16), uint16(number)}, true
	}
	return []uint16{uint16(number)}, true
}

func sunSpecWriteInteger(def sunSpecPointDefinition, native any) (int64, bool) {
	switch def.pointType {
	case TypeInt16, TypeScaleFactor:
		value, ok := native.(int16)
		return int64(value), ok
	case TypeInt32:
		value, ok := native.(int32)
		return int64(value), ok
	case TypeUint16, TypeEnum16, TypeBitfield16:
		value, ok := native.(uint16)
		if def.pointType == TypeEnum16 && len(def.symbols) != 0 {
			ok = ok && def.symbols[uint64(value)] != ""
		}
		return int64(value), ok
	case TypeUint32, TypeBitfield32:
		value, ok := native.(uint32)
		return int64(value), ok
	default:
		return 0, false
	}
}

func sunSpecWriteIntegerFits(pointType PointType, value int64) bool {
	switch pointType {
	case TypeInt16, TypeScaleFactor:
		return value >= math.MinInt16 && value <= math.MaxInt16
	case TypeInt32:
		return value >= math.MinInt32 && value <= math.MaxInt32
	case TypeUint16, TypeEnum16, TypeBitfield16:
		return value >= 0 && value <= math.MaxUint16
	case TypeUint32, TypeBitfield32:
		return value >= 0 && value <= math.MaxUint32
	default:
		return false
	}
}

func sunSpecWriteCoefficient(decimal Decimal, scale int16) (int64, bool) {
	if decimal.Exponent < -10 || decimal.Exponent > 10 {
		return 0, false
	}
	coefficient := decimal.Coefficient
	for exponent := decimal.Exponent; exponent < scale; exponent++ {
		if coefficient%10 != 0 {
			return 0, false
		}
		coefficient /= 10
	}
	for exponent := decimal.Exponent; exponent > scale; exponent-- {
		if coefficient > math.MaxInt64/10 || coefficient < math.MinInt64/10 {
			return 0, false
		}
		coefficient *= 10
	}
	return coefficient, true
}

const sunSpecCurveWrites = "ActCrv ModEna WinTms RvrtTms RmpTms"

// The hand-callable controls and grid-code access tables select writable rows.
// Definition metadata is shared with the decoder; unlisted points stay reads.
var sunSpecFixedWrites = map[uint16]string{
	123: "Conn_WinTms Conn_RvrtTms Conn WMaxLimPct WMaxLimPct_WinTms WMaxLimPct_RvrtTms WMaxLimPct_RmpTms WMaxLim_Ena OutPFSet_WinTms OutPFSet_RvrtTms OutPFSet_RmpTms VArPct_WinTms VArPct_RvrtTms VArPct_RmpTms VArPct_Mod",
	124: "WChaGra WDisChaGra StorCtl_Mod OutWRte InWRte InOutWRte_WinTms InOutWRte_RvrtTms InOutWRte_RmpTms ChaGriSet",
	125: "ModEna SigType Sig WinTms RvtTms RmpTms",
	126: sunSpecCurveWrites,
	127: "WGra HzStr HzStop HysEna ModEna HzStopWGra",
	128: "ArGraMod ArGraSag ArGraSwell ModEna FilTms DbVMin DbVMax BlkZnV HysBlkZnV BlkZnTmms HoldTmms",
	129: sunSpecCurveWrites, 130: sunSpecCurveWrites, 131: sunSpecCurveWrites, 132: sunSpecCurveWrites,
	133: "ActSchd ModEna",
	134: sunSpecCurveWrites, 135: sunSpecCurveWrites, 136: sunSpecCurveWrites, 137: sunSpecCurveWrites,
	138: sunSpecCurveWrites, 139: sunSpecCurveWrites, 140: sunSpecCurveWrites, 141: sunSpecCurveWrites,
	142: sunSpecCurveWrites, 143: sunSpecCurveWrites, 144: sunSpecCurveWrites,
	145: "NomRmpUpRte NomRmpDnRte EmgRmpUpRte EmgRmpDnRte ConnRmpUpRte ConnRmpDnRte AGra",
	703: "ES ESVHi ESVLo ESHzHi ESHzLo ESDlyTms ESRndTms ESRmpTms",
	707: "Ena AdptCrvReq", 708: "Ena AdptCrvReq", 709: "Ena AdptCrvReq",
}

var sunSpecRepeatWrites = map[uint16]map[string]struct{}{
	126: sunSpecRepeatedWriteNames("ActPt DeptRef CrvNam RmpTms RmpDecTmm RmpIncTmm", 20, "V", "VAr"),
	129: sunSpecRepeatedWriteNames("ActPt CrvNam", 20, "Tms", "V"),
	130: sunSpecRepeatedWriteNames("ActPt CrvNam", 20, "Tms", "V"),
	131: sunSpecRepeatedWriteNames("ActPt CrvNam RmpPT1Tms RmpDecTmm RmpIncTmm", 20, "W", "PF"),
	132: sunSpecRepeatedWriteNames("ActPt DeptRef CrvNam RmpPt1Tms RmpDecTmm RmpIncTmm", 20, "V", "W"),
	133: sunSpecRepeatedWriteNames("ActPts StrTms RepPer IntvTyp XTyp X_SF YTyp Y_SF Nam WinTms RmpTms", 10, "X", "Y"),
	134: sunSpecRepeatedWriteNames("ActPt CrvNam RmpPT1Tms RmpDecTmm RmpIncTmm RmpRsUp SnptW WRef WRefStrHz WRefStopHz", 20, "Hz", "W"),
	135: sunSpecRepeatedWriteNames("ActPt CrvNam", 20, "Tms", "Hz"),
	136: sunSpecRepeatedWriteNames("ActPt CrvNam", 20, "Tms", "Hz"),
	137: sunSpecRepeatedWriteNames("ActPt CrvNam", 20, "Tms", "V"),
	138: sunSpecRepeatedWriteNames("ActPt CrvNam", 20, "Tms", "V"),
	139: sunSpecRepeatedWriteNames("ActPt CrvNam", 20, "Tms", "V"),
	140: sunSpecRepeatedWriteNames("ActPt CrvNam", 20, "Tms", "V"),
	141: sunSpecRepeatedWriteNames("ActPt CrvNam", 20, "Tms", "Hz"),
	142: sunSpecRepeatedWriteNames("ActPt CrvNam", 20, "Tms", "Hz"),
	143: sunSpecRepeatedWriteNames("ActPt CrvNam", 20, "Tms", "Hz"),
	144: sunSpecRepeatedWriteNames("ActPt CrvNam", 20, "Tms", "Hz"),
	707: sunSpecRepeatedWriteNames("ActPt V Tms", 0),
	708: sunSpecRepeatedWriteNames("MustTrip.ActPt MustTrip.V MustTrip.Tms MayTrip.ActPt MayTrip.V MayTrip.Tms MomCess.ActPt MomCess.V MomCess.Tms", 0),
	709: sunSpecRepeatedWriteNames("MustTrip.ActPt MustTrip.Hz MustTrip.Tms MayTrip.ActPt MayTrip.Hz MayTrip.Tms MomCess.ActPt MomCess.Hz MomCess.Tms", 0),
}

func sunSpecRepeatedWriteNames(names string, count int, prefixes ...string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, name := range strings.Fields(names) {
		out[name] = struct{}{}
	}
	for _, prefix := range prefixes {
		for i := 1; i <= count; i++ {
			out[fmt.Sprintf("%s%d", prefix, i)] = struct{}{}
		}
	}
	return out
}

func sunSpecPointHasWriteAccess(id uint16, p sunSpecPointDefinition) bool {
	if p.repeated {
		_, ok := sunSpecRepeatWrites[id][p.name]
		return ok
	}
	for _, name := range strings.Fields(sunSpecFixedWrites[id]) {
		if name == p.name {
			return true
		}
	}
	return false
}
