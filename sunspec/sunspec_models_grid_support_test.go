package sunspec

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func mustSunSpecRegistry(t *testing.T, revision SchemaRevision) DecoderRegistry {
	t.Helper()
	registry, err := NewStandardSunSpecDecoderRegistry(revision)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func decoderKeySet(registry DecoderRegistry) map[DecoderKey]bool {
	keys := registry.DecoderKeys()
	out := make(map[DecoderKey]bool, len(keys))
	for _, key := range keys {
		out[key] = true
	}
	return out
}

func registeredDecoderKey(t *testing.T, registry DecoderRegistry, id, length uint16) DecoderKey {
	t.Helper()
	key := DecoderKey{ModelID: id, ModelLength: length, SchemaRevision: registry.SchemaRevision()}
	if !decoderKeySet(registry)[key] {
		t.Fatalf("decoder key absent: model %d length %d", id, length)
	}
	return key
}

func rejectDecoderKey(t *testing.T, registry DecoderRegistry, id, length uint16) {
	t.Helper()
	key := DecoderKey{ModelID: id, ModelLength: length, SchemaRevision: registry.SchemaRevision()}
	if decoderKeySet(registry)[key] {
		t.Fatalf("decoder key present: model %d length %d", id, length)
	}
	if _, ok := registry.definition(key); ok {
		t.Fatalf("definition present: model %d length %d", id, length)
	}
}

func putUint32(words []uint16, offset int, value uint32) {
	words[offset] = uint16(value >> 16)
	words[offset+1] = uint16(value)
}

func gridField(id uint16, name string) string {
	return fmt.Sprintf("sunspec.grid.%d.%s", id, name)
}

func tripField(id uint16, name string) string {
	return fmt.Sprintf("sunspec.der.v2.%d.%s", id, name)
}

func decodeRegisteredChain(t *testing.T, registry DecoderRegistry, blocks ...[]uint16) DecodedChain {
	t.Helper()
	decoded, err := registry.DecodeChain(walkRegisteredGridChain(t, registry, blocks...))
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func walkRegisteredGridChain(t *testing.T, registry DecoderRegistry, blocks ...[]uint16) ChainSnapshot {
	t.Helper()
	keys := make([]DecoderKey, 0, len(blocks))
	seen := make(map[DecoderKey]bool, len(blocks))
	occurrences := make([]Occurrence, len(blocks))
	for index, block := range blocks {
		if len(block) < 2 || len(block) != int(block[1])+2 {
			t.Fatalf("block %d length does not match its header", index)
		}
		key := registeredDecoderKey(t, registry, block[0], block[1])
		if !seen[key] {
			keys = append(keys, key)
			seen[key] = true
		}
		occurrences[index] = Occurrence{words: append([]uint16(nil), block...)}
	}
	const base = uint16(40000)
	raw := rawWordsForOccurrences(occurrences)
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: registry.SchemaRevision(),
		BaseCandidates: []uint16{base},
		Limits:         ChainLimits{MaxTotalWords: uint32(len(raw)), MaxOccurrences: uint32(len(occurrences))},
		DecoderKeys:    keys,
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	logicalViewID := uint64(1000)
	var snapshot ChainSnapshot
	for step := 0; step < len(raw); step++ {
		requests := chain.NextRequests()
		if len(requests) != 1 {
			t.Fatalf("pending requests=%d", len(requests))
		}
		request := requests[0]
		start := int(uint32(request.Address()) - uint32(base))
		end := start + int(request.WordCount())
		if start < 0 || end > len(raw) {
			t.Fatalf("request [%d,%d) exceeds raw words %d", start, end, len(raw))
		}
		snapshot, err = chain.AdmitReplay(request, chainView(t, request, logicalViewID, raw[start:end], "fixture"))
		if err != nil {
			t.Fatal(err)
		}
		logicalViewID++
		if snapshot.OccurrenceCount() != 0 {
			break
		}
	}
	if snapshot.OccurrenceCount() == 0 {
		t.Fatal("chain did not complete")
	}
	for _, occurrence := range snapshot.Occurrences() {
		if occurrence.Disposition != ChainDispositionAdmitted {
			t.Fatalf("model %d disposition %q", occurrence.ModelID(), occurrence.Disposition)
		}
	}
	return snapshot
}

func requireDecodedModel(t *testing.T, decoded DecodedChain, id, length uint16, revision SchemaRevision) DecodedModel {
	t.Helper()
	want := DecoderKey{ModelID: id, ModelLength: length, SchemaRevision: revision}
	var found []DecodedModel
	for _, model := range decoded.Models() {
		if model.Key() == want {
			found = append(found, model)
		}
	}
	if len(found) != 1 || !found[0].GeometryValid() || !found[0].Qualifies() {
		t.Fatalf("model %d/%d count=%d", id, length, len(found))
	}
	return found[0]
}

func requireInactiveGeometry(t *testing.T, decoded DecodedChain, id, length uint16, revision SchemaRevision) {
	t.Helper()
	want := DecoderKey{ModelID: id, ModelLength: length, SchemaRevision: revision}
	var found []DecodedModel
	for _, model := range decoded.Models() {
		if model.Key() == want {
			found = append(found, model)
		}
	}
	if len(found) != 1 || found[0].GeometryValid() || found[0].Qualifies() || len(found[0].Facts()) != 0 {
		t.Fatalf("model %d/%d geometry was accepted", id, length)
	}
}

func oneFact(t *testing.T, model DecodedModel, field, name, group string, index uint16) Fact {
	t.Helper()
	var found []Fact
	for _, fact := range model.Facts() {
		if fact.FieldID == field && fact.PointName == name && fact.GroupID == group && fact.RepeatIndex == index {
			found = append(found, fact)
		}
	}
	if len(found) != 1 {
		t.Fatalf("fact %s %s group %q index %d count=%d", field, name, group, index, len(found))
	}
	return found[0]
}

func requireShape(t *testing.T, fact Fact, unit string, pointType PointType, repeated bool) {
	t.Helper()
	if fact.Required || fact.Unit != unit || fact.Value.PointType() != pointType || fact.Repeated != repeated || fact.Value.State() != ValueValid {
		t.Fatalf("fact %s unit %q type %q repeated %v state %q required %v", fact.FieldID, fact.Unit, fact.Value.PointType(), fact.Repeated, fact.Value.State(), fact.Required)
	}
}

func requireUnsignedFact(t *testing.T, model DecodedModel, field, name, group, unit string, index uint16, pointType PointType, repeated bool, number uint64) {
	t.Helper()
	fact := oneFact(t, model, field, name, group, index)
	requireShape(t, fact, unit, pointType, repeated)
	got, ok := fact.Value.Unsigned()
	if !ok || got != number {
		t.Fatalf("fact %s unsigned %d ok %v", field, got, ok)
	}
}

func requireDecimalFact(t *testing.T, model DecodedModel, field, name, group, unit string, index uint16, pointType PointType, repeated bool, coefficient int64, exponent int16) {
	t.Helper()
	fact := oneFact(t, model, field, name, group, index)
	requireShape(t, fact, unit, pointType, repeated)
	got, ok := fact.Value.Decimal()
	if !ok || got.Coefficient != coefficient || got.Exponent != exponent {
		t.Fatalf("fact %s decimal %#v", field, got)
	}
}

func requireScaledRaw(t *testing.T, model DecodedModel, field, name, group, unit string, index uint16, pointType PointType, raw []uint16, coefficient int64, exponent int16) {
	t.Helper()
	requireDecimalFact(t, model, field, name, group, unit, index, pointType, true, coefficient, exponent)
	fact := oneFact(t, model, field, name, group, index)
	unsigned, ok := fact.Value.Unsigned()
	if !ok || unsigned != uint64(coefficient) || !reflect.DeepEqual(fact.Value.RawWords(), raw) {
		t.Fatalf("fact %s raw %#v unsigned %d", field, fact.Value.RawWords(), unsigned)
	}
}

func requireSymbolFact(t *testing.T, model DecodedModel, field, name, group, unit, symbol string, index uint16, pointType PointType, repeated bool, number uint64) {
	t.Helper()
	fact := oneFact(t, model, field, name, group, index)
	requireShape(t, fact, unit, pointType, repeated)
	if pointType == TypeBitfield16 || pointType == TypeBitfield32 {
		symbols := fact.Value.BitfieldSymbols()
		bits, unknown, ok := fact.Value.Bitfield()
		if !ok || bits != number || unknown != 0 || len(symbols) != 1 || symbols[0] != symbol {
			t.Fatalf("fact %s bits %d unknown %d symbols %#v", field, bits, unknown, symbols)
		}
		return
	}
	got, gotSymbol, ok := fact.Value.Enum()
	if !ok || got != number || gotSymbol != symbol {
		t.Fatalf("fact %s enum %d %q", field, got, gotSymbol)
	}
}

func requireNoPoint(t *testing.T, model DecodedModel, name string) {
	t.Helper()
	for _, fact := range model.Facts() {
		if fact.PointName == name || strings.Contains(fact.FieldID, ".707.") {
			t.Fatalf("unexpected fact %s %s", fact.FieldID, fact.PointName)
		}
	}
}

func requireNoRepeated(t *testing.T, model DecodedModel) {
	t.Helper()
	for _, fact := range model.Facts() {
		if fact.Repeated || fact.GroupID != "" || fact.RepeatIndex != 0 {
			t.Fatalf("unexpected repeated fact %s group %q index %d", fact.FieldID, fact.GroupID, fact.RepeatIndex)
		}
	}
}

func countNamed(model DecodedModel, name, group string) int {
	count := 0
	for _, fact := range model.Facts() {
		if fact.PointName == name && fact.GroupID == group {
			count++
		}
	}
	return count
}

func TestGridSupportFixedModelsDecode(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV1)
	other := mustSunSpecRegistry(t, ModelsRevisionV2)
	pricing := modelBlock(125, 8)
	pricing[2], pricing[3], pricing[4], pricing[6], pricing[8] = 1, 2, 20, 11, 0xFFFF
	frequency := modelBlock(127, 10)
	frequency[2], frequency[5], frequency[7], frequency[8] = 15, 1, 6, 0xFFFF
	reactive := modelBlock(128, 14)
	reactive[2], reactive[3], reactive[7], reactive[13] = 1, 8, 12, 0
	settings := modelBlock(145, 8)
	settings[2], settings[8], settings[9] = 7, 3, 0xFFFF
	decoded := decodeRegisteredChain(t, registry, modelBlock(1, 66), pricing, frequency, reactive, settings)
	price := requireDecodedModel(t, decoded, 125, 8, ModelsRevisionV1)
	requireSymbolFact(t, price, gridField(125, "ModEna"), "ModEna", "", "", "ENABLE", 0, TypeBitfield16, false, 1)
	requireSymbolFact(t, price, gridField(125, "SigType"), "SigType", "", "", "RELATIVE", 0, TypeEnum16, false, 2)
	requireDecimalFact(t, price, gridField(125, "Sig"), "Sig", "", "", 0, TypeInt16, false, 20, -1)
	requireUnsignedFact(t, price, gridField(125, "RvtTms"), "RvtTms", "", "Secs", 0, TypeUint16, false, 11)
	gradient := requireDecodedModel(t, decoded, 127, 10, ModelsRevisionV1)
	requireDecimalFact(t, gradient, gridField(127, "WGra"), "WGra", "", "% PM/Hz", 0, TypeUint16, false, 15, -1)
	requireSymbolFact(t, gradient, gridField(127, "HysEna"), "HysEna", "", "", "ENABLED", 0, TypeBitfield16, false, 1)
	requireDecimalFact(t, gradient, gridField(127, "HzStopWGra"), "HzStopWGra", "", "% WMax/min", 0, TypeUint16, false, 6, 0)
	current := requireDecodedModel(t, decoded, 128, 14, ModelsRevisionV1)
	requireSymbolFact(t, current, gridField(128, "ArGraMod"), "ArGraMod", "", "", "CENTER", 0, TypeEnum16, false, 1)
	requireDecimalFact(t, current, gridField(128, "ArGraSag"), "ArGraSag", "", "%ARtg/%dV", 0, TypeUint16, false, 8, 0)
	requireDecimalFact(t, current, gridField(128, "DbVMin"), "DbVMin", "", "% VRef", 0, TypeUint16, false, 12, 0)
	extended := requireDecodedModel(t, decoded, 145, 8, ModelsRevisionV1)
	requireDecimalFact(t, extended, gridField(145, "NomRmpUpRte"), "NomRmpUpRte", "", "Pct", 0, TypeUint16, false, 7, -1)
	requireDecimalFact(t, extended, gridField(145, "AGra"), "AGra", "", "Pct", 0, TypeUint16, false, 3, -1)
	for _, model := range []struct{ id, length uint16 }{{125, 8}, {127, 10}, {128, 14}, {145, 8}} {
		rejectDecoderKey(t, registry, model.id, model.length+1)
		rejectDecoderKey(t, other, model.id, model.length)
	}
}

func TestGridSupportVoltVarCurvesDecode(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV1)
	one := modelBlock(126, 64)
	one[7], one[9], one[10], one[13], one[14], one[15] = 1, 0xFFFF, 0, 2, 30, 7
	two := modelBlock(126, 118)
	two[7], two[13], two[67] = 2, 1, 3
	none := modelBlock(126, 10)
	mismatch := modelBlock(126, 64)
	mismatch[7] = 2
	decoded := decodeRegisteredChain(t, registry, modelBlock(1, 66), one, two, none)
	first := requireDecodedModel(t, decoded, 126, 64, ModelsRevisionV1)
	requireSymbolFact(t, first, gridField(126, "DeptRef"), "DeptRef", "curve", "", "VArMax", 1, TypeEnum16, true, 2)
	requireDecimalFact(t, first, gridField(126, "V1"), "V1", "curve", "% VRef", 1, TypeUint16, true, 30, -1)
	requireDecimalFact(t, first, gridField(126, "VAr1"), "VAr1", "curve", "", 1, TypeInt16, true, 7, 0)
	second := requireDecodedModel(t, decoded, 126, 118, ModelsRevisionV1)
	requireSymbolFact(t, second, gridField(126, "DeptRef"), "DeptRef", "curve", "", "WMax", 1, TypeEnum16, true, 1)
	requireSymbolFact(t, second, gridField(126, "DeptRef"), "DeptRef", "curve", "", "VArAval", 2, TypeEnum16, true, 3)
	empty := requireDecodedModel(t, decoded, 126, 10, ModelsRevisionV1)
	requireUnsignedFact(t, empty, gridField(126, "NCrv"), "NCrv", "", "", 0, TypeUint16, false, 0)
	requireNoRepeated(t, empty)
	requireInactiveGeometry(t, decodeRegisteredChain(t, registry, modelBlock(1, 66), mismatch), 126, 64, ModelsRevisionV1)
}

func TestGridSupportWattCurvesDecode(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV1)
	wattPF := modelBlock(131, 64)
	wattPF[7], wattPF[9], wattPF[13], wattPF[14], wattPF[61] = 1, 0, 12, 9, 3
	voltWatt := modelBlock(132, 64)
	voltWatt[7], voltWatt[13], voltWatt[15], voltWatt[62] = 1, 1, 6, 4
	freqWatt := modelBlock(134, 68)
	freqWatt[7], freqWatt[13], freqWatt[14], freqWatt[64] = 1, 500, 11, 2
	decoded := decodeRegisteredChain(t, registry, modelBlock(1, 66), wattPF, voltWatt, freqWatt)
	powerFactor := requireDecodedModel(t, decoded, 131, 64, ModelsRevisionV1)
	requireDecimalFact(t, powerFactor, gridField(131, "W1"), "W1", "curve", "% WMax", 1, TypeInt16, true, 12, 0)
	requireDecimalFact(t, powerFactor, gridField(131, "PF1"), "PF1", "curve", "cos()", 1, TypeInt16, true, 9, 0)
	requireUnsignedFact(t, powerFactor, gridField(131, "RmpPT1Tms"), "RmpPT1Tms", "curve", "Secs", 1, TypeUint16, true, 3)
	voltage := requireDecodedModel(t, decoded, 132, 64, ModelsRevisionV1)
	requireSymbolFact(t, voltage, gridField(132, "DeptRef"), "DeptRef", "curve", "", "%WMax", 1, TypeEnum16, true, 1)
	requireDecimalFact(t, voltage, gridField(132, "W1"), "W1", "curve", "% VRef", 1, TypeInt16, true, 6, 0)
	requireUnsignedFact(t, voltage, gridField(132, "RmpPt1Tms"), "RmpPt1Tms", "curve", "Secs", 1, TypeUint16, true, 4)
	frequency := requireDecodedModel(t, decoded, 134, 68, ModelsRevisionV1)
	requireDecimalFact(t, frequency, gridField(134, "Hz1"), "Hz1", "curve", "Hz", 1, TypeUint16, true, 500, 0)
	requireDecimalFact(t, frequency, gridField(134, "W1"), "W1", "curve", "% WRef", 1, TypeInt16, true, 11, 0)
	requireUnsignedFact(t, frequency, gridField(134, "RmpRsUp"), "RmpRsUp", "curve", "% WMax/min", 1, TypeUint16, true, 2)
	scale := oneFact(t, frequency, gridField(134, "Hz_SF"), "Hz_SF", "", 0)
	requireShape(t, scale, "SF", TypeScaleFactor, false)
}

func TestGridSupportScheduleScalesStayWithTheirInstance(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV1)
	words := modelBlock(133, 126)
	words[2], words[3], words[5] = 16, 1024, 2
	words[12], words[14], words[15] = 6, 0, 99
	putUint32(words, 17, 10)
	words[72], words[74] = 1, 0xFFFF
	putUint32(words, 77, 10)
	decoded := decodeRegisteredChain(t, registry, modelBlock(1, 66), words)
	model := requireDecodedModel(t, decoded, 133, 126, ModelsRevisionV1)
	active := oneFact(t, model, gridField(133, "ActSchd"), "ActSchd", "", 0)
	requireShape(t, active, "", TypeBitfield32, false)
	bits, unknown, ok := active.Value.Bitfield()
	symbols := active.Value.BitfieldSymbols()
	if !ok || bits != (1<<10)|(1<<20) || unknown == 0 || len(symbols) != 1 || symbols[0] != "SCHED12" {
		t.Fatalf("active schedule bits %d unknown %d symbols %#v", bits, unknown, symbols)
	}
	requireSymbolFact(t, model, gridField(133, "IntvTyp"), "IntvTyp", "repeating", "", "WEEKEND", 1, TypeEnum16, true, 6)
	requireSymbolFact(t, model, gridField(133, "IntvTyp"), "IntvTyp", "repeating", "", "DAILY", 2, TypeEnum16, true, 1)
	requireSymbolFact(t, model, gridField(133, "YTyp"), "YTyp", "repeating", "", "OTHER", 1, TypeEnum16, true, 99)
	requireDecimalFact(t, model, gridField(133, "X1"), "X1", "repeating", "", 1, TypeInt32, true, 10, 0)
	requireDecimalFact(t, model, gridField(133, "X1"), "X1", "repeating", "", 2, TypeInt32, true, 10, -1)
}

func TestGridSupportRideThroughCurvesDecode(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV1)
	type ride struct {
		id     uint16
		axis   string
		unit   string
		sample uint16
		cease  bool
	}
	cases := []ride{
		{129, "V", "% VRef", 4, false}, {130, "V", "% VRef", 5, false},
		{135, "Hz", "Hz", 499, false}, {136, "Hz", "Hz", 501, false},
		{137, "V", "% VRef", 40, false}, {138, "V", "% VRef", 41, false},
		{139, "V", "% VRef", 42, true}, {140, "V", "% VRef", 43, true},
		{141, "Hz", "Hz", 480, false}, {142, "Hz", "Hz", 481, false},
		{143, "Hz", "Hz", 482, true}, {144, "Hz", "Hz", 483, true},
	}
	blocks := [][]uint16{modelBlock(1, 66)}
	for _, item := range cases {
		words := modelBlock(item.id, 60)
		words[7], words[13], words[14], words[61] = 1, 9, item.sample, 1
		if item.cease {
			words[11] = 1
		}
		blocks = append(blocks, words)
	}
	decoded := decodeRegisteredChain(t, registry, blocks...)
	for _, item := range cases {
		model := requireDecodedModel(t, decoded, item.id, 60, ModelsRevisionV1)
		requireDecimalFact(t, model, gridField(item.id, "Tms1"), "Tms1", "curve", "Secs", 1, TypeUint16, true, 9, 0)
		requireDecimalFact(t, model, gridField(item.id, item.axis+"1"), item.axis+"1", "curve", item.unit, 1, TypeUint16, true, int64(item.sample), 0)
		requireSymbolFact(t, model, gridField(item.id, "ReadOnly"), "ReadOnly", "curve", "", "READONLY", 1, TypeEnum16, true, 1)
		if item.cease {
			requireSymbolFact(t, model, gridField(item.id, "CrvType"), "CrvType", "", "", "CEASE_TO_ENERGIZE", 0, TypeEnum16, false, 1)
		}
	}
}

func TestGridSupportHighTripCurveDecode(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV2)
	words := modelBlock(708, 20)
	words[2], words[3], words[4], words[5], words[6], words[7] = 1, 4, 1, 1, 1, 0xFFFF
	words[9], words[10], words[11] = 1, 1, 20
	putUint32(words, 12, 0x00010002)
	words[14], words[15] = 2, 21
	putUint32(words, 16, 0x00030004)
	words[18], words[19] = 3, 22
	putUint32(words, 20, 0x00050006)
	second := modelBlock(708, 33)
	second[5], second[6], second[9], second[11], second[22], second[24] = 1, 2, 0, 20, 1, 80
	decoded := decodeRegisteredChain(t, registry, modelBlock(1, 66), words, second)
	model := requireDecodedModel(t, decoded, 708, 20, ModelsRevisionV2)
	requireNoPoint(t, model, "absent")
	requireSymbolFact(t, model, tripField(708, "Ena"), "Ena", "", "", "ENABLED", 0, TypeEnum16, false, 1)
	requireUnsignedFact(t, model, tripField(708, "AdptCrvReq"), "AdptCrvReq", "", "", 0, TypeUint16, false, 4)
	requireSymbolFact(t, model, tripField(708, "AdptCrvRslt"), "AdptCrvRslt", "", "", "COMPLETED", 0, TypeEnum16, false, 1)
	requireSymbolFact(t, model, tripField(708, "Crv.ReadOnly"), "ReadOnly", "curve", "", "R", 1, TypeEnum16, true, 1)
	requireUnsignedFact(t, model, tripField(708, "Crv.MustTrip.ActPt"), "MustTrip.ActPt", "curve", "", 1, TypeUint16, true, 1)
	requireDecimalFact(t, model, tripField(708, "Crv.MustTrip.Pt.V"), "MustTrip.V", "curve.1.MustTrip", "VNomPct", 1, TypeUint16, true, 20, -1)
	requireScaledRaw(t, model, tripField(708, "Crv.MustTrip.Pt.Tms"), "MustTrip.Tms", "curve.1.MustTrip", "Secs", 1, TypeUint32, []uint16{0x0001, 0x0002}, 0x00010002, 0)
	requireUnsignedFact(t, model, tripField(708, "Crv.MayTrip.ActPt"), "MayTrip.ActPt", "curve", "", 1, TypeUint16, true, 2)
	requireDecimalFact(t, model, tripField(708, "Crv.MayTrip.Pt.V"), "MayTrip.V", "curve.1.MayTrip", "VNomPct", 1, TypeUint16, true, 21, -1)
	requireScaledRaw(t, model, tripField(708, "Crv.MayTrip.Pt.Tms"), "MayTrip.Tms", "curve.1.MayTrip", "Secs", 1, TypeUint32, []uint16{0x0003, 0x0004}, 0x00030004, 0)
	requireUnsignedFact(t, model, tripField(708, "Crv.MomCess.ActPt"), "MomCess.ActPt", "curve", "", 1, TypeUint16, true, 3)
	requireDecimalFact(t, model, tripField(708, "Crv.MomCess.Pt.V"), "MomCess.V", "curve.1.MomCess", "VNomPct", 1, TypeUint16, true, 22, -1)
	requireScaledRaw(t, model, tripField(708, "Crv.MomCess.Pt.Tms"), "MomCess.Tms", "curve.1.MomCess", "Secs", 1, TypeUint32, []uint16{0x0005, 0x0006}, 0x00050006, 0)
	other := requireDecodedModel(t, decoded, 708, 33, ModelsRevisionV2)
	requireSymbolFact(t, other, tripField(708, "Crv.ReadOnly"), "ReadOnly", "curve", "", "RW", 1, TypeEnum16, true, 0)
	requireSymbolFact(t, other, tripField(708, "Crv.ReadOnly"), "ReadOnly", "curve", "", "R", 2, TypeEnum16, true, 1)
	requireDecimalFact(t, other, tripField(708, "Crv.MustTrip.Pt.V"), "MustTrip.V", "curve.2.MustTrip", "VNomPct", 1, TypeUint16, true, 80, 0)
}

func TestGridSupportLowTripCurveDecode(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV2)
	words := modelBlock(709, 23)
	words[2], words[3], words[4], words[5], words[6], words[7] = 1, 5, 2, 1, 1, 0xFFFF
	words[9], words[10] = 0, 1
	putUint32(words, 11, 0x00010002)
	putUint32(words, 13, 0x00030004)
	words[15] = 2
	putUint32(words, 16, 0x00050006)
	putUint32(words, 18, 0x00070008)
	words[20] = 3
	putUint32(words, 21, 0x0009000A)
	putUint32(words, 23, 0x000B000C)
	second := modelBlock(709, 39)
	second[5], second[6] = 1, 2
	putUint32(second, 11, 0x00010002)
	putUint32(second, 27, 0x000A000B)
	decoded := decodeRegisteredChain(t, registry, modelBlock(1, 66), words, second)
	model := requireDecodedModel(t, decoded, 709, 23, ModelsRevisionV2)
	requireNoPoint(t, model, "MustTrip.V")
	requireSymbolFact(t, model, tripField(709, "Ena"), "Ena", "", "", "ENABLED", 0, TypeEnum16, false, 1)
	requireSymbolFact(t, model, tripField(709, "AdptCrvRslt"), "AdptCrvRslt", "", "", "FAILED", 0, TypeEnum16, false, 2)
	requireSymbolFact(t, model, tripField(709, "Crv.ReadOnly"), "ReadOnly", "curve", "", "RW", 1, TypeEnum16, true, 0)
	requireScaledRaw(t, model, tripField(709, "Crv.MustTrip.Pt.Hz"), "MustTrip.Hz", "curve.1.MustTrip", "Hz", 1, TypeUint32, []uint16{0x0001, 0x0002}, 0x00010002, -1)
	requireScaledRaw(t, model, tripField(709, "Crv.MustTrip.Pt.Tms"), "MustTrip.Tms", "curve.1.MustTrip", "Secs", 1, TypeUint32, []uint16{0x0003, 0x0004}, 0x00030004, 0)
	requireScaledRaw(t, model, tripField(709, "Crv.MayTrip.Pt.Hz"), "MayTrip.Hz", "curve.1.MayTrip", "Hz", 1, TypeUint32, []uint16{0x0005, 0x0006}, 0x00050006, -1)
	requireScaledRaw(t, model, tripField(709, "Crv.MayTrip.Pt.Tms"), "MayTrip.Tms", "curve.1.MayTrip", "Secs", 1, TypeUint32, []uint16{0x0007, 0x0008}, 0x00070008, 0)
	requireScaledRaw(t, model, tripField(709, "Crv.MomCess.Pt.Hz"), "MomCess.Hz", "curve.1.MomCess", "Hz", 1, TypeUint32, []uint16{0x0009, 0x000A}, 0x0009000A, -1)
	requireScaledRaw(t, model, tripField(709, "Crv.MomCess.Pt.Tms"), "MomCess.Tms", "curve.1.MomCess", "Secs", 1, TypeUint32, []uint16{0x000B, 0x000C}, 0x000B000C, 0)
	other := requireDecodedModel(t, decoded, 709, 39, ModelsRevisionV2)
	requireScaledRaw(t, other, tripField(709, "Crv.MustTrip.Pt.Hz"), "MustTrip.Hz", "curve.2.MustTrip", "Hz", 1, TypeUint32, []uint16{0x000A, 0x000B}, 0x000A000B, 0)
}

func TestGridSupportTripCurveLengthFollowsCountWords(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV2)
	other := mustSunSpecRegistry(t, ModelsRevisionV1)
	highEmpty := modelBlock(708, 7)
	highEmpty[2], highEmpty[5] = 1, 5
	lowEmpty := modelBlock(709, 7)
	lowEmpty[2], lowEmpty[5] = 1, 6
	highFlat := modelBlock(708, 59)
	highFlat[6], highFlat[57] = 13, 1
	highPoints := modelBlock(708, 59)
	highPoints[5], highPoints[6], highPoints[50] = 1, 4, 77
	lowFlat := modelBlock(709, 23)
	lowFlat[6], lowFlat[21] = 4, 1
	highMismatch := modelBlock(708, 20)
	highMismatch[5], highMismatch[6] = 2, 1
	lowMismatch := modelBlock(709, 23)
	lowMismatch[5], lowMismatch[6] = 2, 1
	empty := decodeRegisteredChain(t, registry, modelBlock(1, 66), highEmpty, lowEmpty)
	flatChain := decodeRegisteredChain(t, registry, modelBlock(1, 66), highFlat)
	pointChain := decodeRegisteredChain(t, registry, modelBlock(1, 66), highPoints)
	lowChain := decodeRegisteredChain(t, registry, modelBlock(1, 66), lowFlat)
	inactive := decodeRegisteredChain(t, registry, modelBlock(1, 66), highMismatch, lowMismatch)
	highNone := requireDecodedModel(t, empty, 708, 7, ModelsRevisionV2)
	requireSymbolFact(t, highNone, tripField(708, "Ena"), "Ena", "", "", "ENABLED", 0, TypeEnum16, false, 1)
	requireUnsignedFact(t, highNone, tripField(708, "NPt"), "NPt", "", "", 0, TypeUint16, false, 5)
	requireNoRepeated(t, highNone)
	requireNoPoint(t, highNone, "MustTrip.V")
	lowNone := requireDecodedModel(t, empty, 709, 7, ModelsRevisionV2)
	requireUnsignedFact(t, lowNone, tripField(709, "NPt"), "NPt", "", "", 0, TypeUint16, false, 6)
	requireNoPoint(t, lowNone, "MustTrip.Hz")
	flat := requireDecodedModel(t, flatChain, 708, 59, ModelsRevisionV2)
	if countNamed(flat, "ReadOnly", "curve") != 13 {
		t.Fatalf("flat curves=%d", countNamed(flat, "ReadOnly", "curve"))
	}
	requireSymbolFact(t, flat, tripField(708, "Crv.ReadOnly"), "ReadOnly", "curve", "", "R", 13, TypeEnum16, true, 1)
	requireNoPoint(t, flat, "MustTrip.V")
	points := requireDecodedModel(t, pointChain, 708, 59, ModelsRevisionV2)
	if countNamed(points, "ReadOnly", "curve") != 4 {
		t.Fatalf("point curves=%d", countNamed(points, "ReadOnly", "curve"))
	}
	requireDecimalFact(t, points, tripField(708, "Crv.MustTrip.Pt.V"), "MustTrip.V", "curve.4.MustTrip", "VNomPct", 1, TypeUint16, true, 77, 0)
	lowCurves := requireDecodedModel(t, lowChain, 709, 23, ModelsRevisionV2)
	if countNamed(lowCurves, "ReadOnly", "curve") != 4 {
		t.Fatalf("low flat curves=%d", countNamed(lowCurves, "ReadOnly", "curve"))
	}
	requireSymbolFact(t, lowCurves, tripField(709, "Crv.ReadOnly"), "ReadOnly", "curve", "", "R", 4, TypeEnum16, true, 1)
	requireNoPoint(t, lowCurves, "MustTrip.Hz")
	requireInactiveGeometry(t, inactive, 708, 20, ModelsRevisionV2)
	requireInactiveGeometry(t, inactive, 709, 23, ModelsRevisionV2)
	for _, model := range []struct{ id, length uint16 }{{708, 8}, {709, 8}, {708, 20}, {709, 23}} {
		rejectDecoderKey(t, other, model.id, model.length)
	}
	rejectDecoderKey(t, registry, 708, 8)
	rejectDecoderKey(t, registry, 709, 8)
}

func TestGridSupportRepeatKeysFollowPublishedStrides(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV1)
	other := mustSunSpecRegistry(t, ModelsRevisionV2)
	present := decoderKeySet(registry)
	for _, span := range []struct{ id, base, stride uint16 }{
		{126, 10, 54}, {129, 10, 50}, {130, 10, 50}, {131, 10, 54}, {132, 10, 54},
		{133, 6, 60}, {134, 10, 58}, {135, 10, 50}, {136, 10, 50}, {137, 10, 50},
		{138, 10, 50}, {139, 10, 50}, {140, 10, 50}, {141, 10, 50}, {142, 10, 50},
		{143, 10, 50}, {144, 10, 50},
	} {
		for _, length := range []uint16{span.base, span.base + span.stride} {
			key := DecoderKey{ModelID: span.id, ModelLength: length, SchemaRevision: ModelsRevisionV1}
			if !present[key] {
				t.Fatalf("decoder key absent: model %d length %d", span.id, length)
			}
		}
		absent := DecoderKey{ModelID: span.id, ModelLength: span.base + 1, SchemaRevision: ModelsRevisionV1}
		if present[absent] {
			t.Fatalf("decoder key present: model %d length %d", span.id, span.base+1)
		}
		if _, ok := registry.definition(absent); ok {
			t.Fatalf("definition present: model %d length %d", span.id, span.base+1)
		}
		if _, ok := other.definition(DecoderKey{ModelID: span.id, ModelLength: span.base, SchemaRevision: ModelsRevisionV2}); ok {
			t.Fatalf("model %d resolved on the other revision", span.id)
		}
	}
}

func TestEnterServiceKeepsItsDecoder(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV2)
	words := modelBlock(703, 17)
	words[2], words[3], words[4], words[17] = 1, 1100, 900, 0xffff
	putUint32(words, 5, 5000)
	decoded := decodeRegisteredChain(t, registry, modelBlock(1, 66), words)
	model := requireDecodedModel(t, decoded, 703, 17, ModelsRevisionV2)
	enter := oneFact(t, model, "sunspec.der.v2.703.ES", "ES", "", 0)
	requireShape(t, enter, "", TypeEnum16, false)
	number, symbol, ok := enter.Value.Enum()
	if !ok || number != 1 || symbol != "" {
		t.Fatalf("enter service enum %d %q", number, symbol)
	}
	requireDecimalFact(t, model, "sunspec.der.v2.703.ESVHi", "ESVHi", "", "Pct", 0, TypeUint16, false, 1100, -1)
	requireDecimalFact(t, model, "sunspec.der.v2.703.ESVLo", "ESVLo", "", "Pct", 0, TypeUint16, false, 900, -1)
	delay := oneFact(t, model, "sunspec.der.v2.703.ESDlyTms", "ESDlyTms", "", 0)
	requireShape(t, delay, "", TypeUint32, false)
	rejectDecoderKey(t, mustSunSpecRegistry(t, ModelsRevisionV1), 703, 17)
}

func TestTripLVStaysWithoutDecoderKey(t *testing.T) {
	for _, revision := range []SchemaRevision{ModelsRevisionV1, ModelsRevisionV2} {
		registry := mustSunSpecRegistry(t, revision)
		for _, key := range registry.DecoderKeys() {
			if key.ModelID == 707 || key.SchemaRevision != revision {
				t.Fatalf("decoder key %#v", key)
			}
		}
		rejectDecoderKey(t, registry, 707, 33)
		rejectDecoderKey(t, registry, 707, 7)
	}
}
