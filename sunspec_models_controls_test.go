package modbusreg

import (
	"fmt"
	"reflect"
	"testing"
)

func TestSunSpecModel123CatalogMatchesPinnedImmediateControls(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	key := SunSpecDecoderKey{ModelID: 123, ModelLength: 24, SchemaRevision: testSunSpecModelsRevision}
	definition, ok := registry.definition(key)
	if !ok {
		t.Fatal("standard Model 123/L24 decoder key absent")
	}
	if _, wrongLength := registry.definition(SunSpecDecoderKey{ModelID: 123, ModelLength: 23, SchemaRevision: testSunSpecModelsRevision}); wrongLength {
		t.Fatal("Model 123 admitted by ID without exact length")
	}

	want := "ID:uint16:1,L:uint16:1,Conn_WinTms:uint16:1,Conn_RvrtTms:uint16:1,Conn:enum16:1,WMaxLimPct:uint16:1,WMaxLimPct_WinTms:uint16:1,WMaxLimPct_RvrtTms:uint16:1,WMaxLimPct_RmpTms:uint16:1,WMaxLim_Ena:enum16:1,OutPFSet:int16:1,OutPFSet_WinTms:uint16:1,OutPFSet_RvrtTms:uint16:1,OutPFSet_RmpTms:uint16:1,OutPFSet_Ena:enum16:1,VArWMaxPct:int16:1,VArMaxPct:int16:1,VArAvalPct:int16:1,VArPct_WinTms:uint16:1,VArPct_RvrtTms:uint16:1,VArPct_RmpTms:uint16:1,VArPct_Mod:enum16:1,VArPct_Ena:enum16:1,WMaxLimPct_SF:sunssf:1,OutPFSet_SF:sunssf:1,VArPct_SF:sunssf:1"
	got := make([]string, len(definition.points))
	var extent uint16
	for index, point := range definition.points {
		got[index] = fmt.Sprintf("%s:%s:%d", point.name, point.pointType, point.size)
		if point.offset != extent {
			t.Fatalf("point %s offset=%d want=%d", point.name, point.offset, extent)
		}
		extent += point.size
	}
	if joined := joinSunSpecCatalog(got); joined != want {
		t.Fatalf("Model 123 catalog\n%s\nwant\n%s", joined, want)
	}
	if extent != 26 {
		t.Fatalf("Model 123 extent=%d want=26 header+payload words", extent)
	}
}

func TestSunSpecModel123DecodesReadOnlyTypedFacts(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	words := modelWords(t, registry, 123, 24, validModel123Values())
	occurrence := admittedOccurrence(123, 24, words, 6)
	occurrence.spans = []SunSpecSourceSpan{{LogicalViewID: 77, PDUOffset: 4, WordCount: 26}}
	decoded, err := registry.DecodeOccurrence(occurrence)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.Qualifies() || decoded.Key() != (SunSpecDecoderKey{123, 24, testSunSpecModelsRevision}) || decoded.Topology() != SunSpecTopologyNone {
		t.Fatalf("decoded qualifies=%t key=%#v topology=%q", decoded.Qualifies(), decoded.Key(), decoded.Topology())
	}
	if !reflect.DeepEqual(decoded.RawWords(), words) || !reflect.DeepEqual(decoded.SourceSpans(), occurrence.SourceSpans()) {
		t.Fatal("Model 123 raw words or provenance were not retained")
	}

	connection := mustSunSpecFact(t, decoded, "der.control.connection.command")
	if number, symbol, ok := connection.Value.Enum(); !ok || number != 1 || symbol != "CONNECT" {
		t.Fatalf("connection=%d/%q/%v", number, symbol, ok)
	}
	activeLimit := mustSunSpecFact(t, decoded, "der.control.active_power_limit.percent")
	if decimal, ok := activeLimit.Value.Decimal(); !ok || decimal != (SunSpecDecimal{Coefficient: 500, Exponent: -1}) {
		t.Fatalf("active limit=%#v/%v", decimal, ok)
	}
	powerFactor := mustSunSpecFact(t, decoded, "der.control.power_factor.setpoint")
	if decimal, ok := powerFactor.Value.Decimal(); !ok || decimal != (SunSpecDecimal{Coefficient: 950, Exponent: -3}) {
		t.Fatalf("power factor=%#v/%v", decimal, ok)
	}
	mode := mustSunSpecFact(t, decoded, "der.control.reactive_power.mode")
	if number, symbol, ok := mode.Value.Enum(); !ok || number != 3 || symbol != "VAR_AVAIL" {
		t.Fatalf("reactive mode=%d/%q/%v", number, symbol, ok)
	}
}

func TestSunSpecModel123MandatoryUnknownAndSentinelFailClosed(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	tests := map[string]struct {
		point string
		words []uint16
		state SunSpecValueState
	}{
		"unknown connection enum": {point: "Conn", words: []uint16{9}, state: SunSpecValueValid},
		"missing active limit":    {point: "WMaxLimPct", words: []uint16{0xffff}, state: SunSpecValueNotImplemented},
		"missing scale factor":    {point: "OutPFSet_SF", words: []uint16{0x8000}, state: SunSpecValueNotImplemented},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			values := validModel123Values()
			values[tc.point] = tc.words
			words := modelWords(t, registry, 123, 24, values)
			decoded, err := registry.DecodeOccurrence(admittedOccurrence(123, 24, words, 1))
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Qualifies() {
				t.Fatal("invalid mandatory Model 123 value qualified")
			}
			var pointFact SunSpecFact
			for _, fact := range decoded.Facts() {
				if fact.PointName == tc.point {
					pointFact = fact
					break
				}
			}
			if pointFact.PointName == "" || pointFact.Value.State() != tc.state || !reflect.DeepEqual(pointFact.Value.RawWords(), tc.words) {
				t.Fatalf("point=%q state=%q raw=%v", pointFact.PointName, pointFact.Value.State(), pointFact.Value.RawWords())
			}
		})
	}
}

func validModel123Values() map[string][]uint16 {
	return map[string][]uint16{
		"Conn":          {1},
		"WMaxLimPct":    {500},
		"WMaxLim_Ena":   {1},
		"OutPFSet":      {950},
		"OutPFSet_Ena":  {1},
		"VArPct_Mod":    {3},
		"VArPct_Ena":    {0},
		"WMaxLimPct_SF": {0xffff},
		"OutPFSet_SF":   {0xfffd},
		"VArPct_SF":     {0xffff},
	}
}

func mustSunSpecFact(t *testing.T, model SunSpecDecodedModel, fieldID string) SunSpecFact {
	t.Helper()
	fact, ok := model.Fact(fieldID)
	if !ok {
		t.Fatalf("fact %q absent", fieldID)
	}
	return fact
}
