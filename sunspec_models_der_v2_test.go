package modbusreg

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestSunSpecV2DERDefinitionsAndApacheNotice(t *testing.T) {
	t.Run("Apache attribution", func(t *testing.T) {
		contents, err := os.ReadFile("THIRD_PARTY_NOTICES.md")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{
			"SunSpec/models",
			"https://github.com/sunspec/models",
			"90b4a331dcca1d6eac69c1bead952fddcc5852e0",
			"Models 701/153, 702/50, 703/17, 707 variable geometry, 713/7,",
			"714 variable geometry, 715/7, 802/62, 803 variable geometry, 804 variable",
			"geometry, 805/42, 806/1, 807/34, 808/1, and 809/1",
			"modified by Helianthus",
			"Apache License",
			"Version 2.0, January 2004",
		} {
			if !strings.Contains(string(contents), want) {
				t.Fatalf("third-party notice lacks %q", want)
			}
		}
	})

	t.Run("exact V2 shapes", func(t *testing.T) {
		registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []struct {
			id, length uint16
			points     int
			first      []string
			last       string
		}{
			{701, 153, 72, []string{"ID", "L", "ACType"}, "MnAlrmInfo"},
			{702, 50, 51, []string{"ID", "L", "WMaxRtg"}, "S_SF"},
			{703, 17, 13, []string{"ID", "L", "ES"}, "Hz_SF"},
			{713, 7, 9, []string{"ID", "L", "WHRtg"}, "Pct_SF"},
			{715, 7, 7, []string{"ID", "L", "LocRemCtl"}, "OpCtl"},
			{802, 62, 58, []string{"ID", "L", "AHRtg"}, "W_SF"},
			{805, 42, 28, []string{"ID", "L", "StrIdx"}, "Tmp_SF"},
			{806, 1, 3, []string{"ID", "L", "BatTBD"}, "BatTBD"},
			{807, 34, 32, []string{"ID", "L", "Idx"}, "Pad1"},
			{808, 1, 3, []string{"ID", "L", "ModuleTBD"}, "ModuleTBD"},
			{809, 1, 3, []string{"ID", "L", "StackTBD"}, "StackTBD"},
		} {
			definition, ok := registry.definition(SunSpecDecoderKey{ModelID: want.id, ModelLength: want.length, SchemaRevision: SunSpecModelsRevisionV2})
			if !ok || len(definition.points) != want.points {
				t.Fatalf("Model %d/%d definition=%v points=%d", want.id, want.length, ok, len(definition.points))
			}
			for index, name := range want.first {
				if definition.points[index].name != name {
					t.Fatalf("Model %d point %d=%q want=%q", want.id, index, definition.points[index].name, name)
				}
			}
			if definition.points[len(definition.points)-1].name != want.last {
				t.Fatalf("Model %d last=%q want=%q", want.id, definition.points[len(definition.points)-1].name, want.last)
			}
		}
	})
}

func TestSunSpecV2BESSBaseRetainsPinnedPointOrderTypesAndScales(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.definition(SunSpecDecoderKey{ModelID: 802, ModelLength: 62, SchemaRevision: SunSpecModelsRevisionV2})
	if !ok {
		t.Fatal("Model 802/62 definition absent")
	}
	want := strings.Split("ID:uint16:1:,L:uint16:1:,AHRtg:uint16:1:AHRtg_SF,WHRtg:uint16:1:WHRtg_SF,WChaRteMax:uint16:1:WChaDisChaMax_SF,WDisChaRteMax:uint16:1:WChaDisChaMax_SF,DisChaRte:uint16:1:DisChaRte_SF,SoCMax:uint16:1:SoC_SF,SoCMin:uint16:1:SoC_SF,SocRsvMax:uint16:1:SoC_SF,SoCRsvMin:uint16:1:SoC_SF,SoC:uint16:1:SoC_SF,DoD:uint16:1:DoD_SF,SoH:uint16:1:SoH_SF,NCyc:uint32:2:,ChaSt:enum16:1:,LocRemCtl:enum16:1:,Hb:uint16:1:,CtrlHb:uint16:1:,AlmRst:uint16:1:,Typ:enum16:1:,State:enum16:1:,StateVnd:enum16:1:,WarrDt:uint32:2:,Evt1:bitfield32:2:,Evt2:bitfield32:2:,EvtVnd1:bitfield32:2:,EvtVnd2:bitfield32:2:,V:uint16:1:V_SF,VMax:uint16:1:V_SF,VMin:uint16:1:V_SF,CellVMax:uint16:1:CellV_SF,CellVMaxStr:uint16:1:,CellVMaxMod:uint16:1:,CellVMin:uint16:1:CellV_SF,CellVMinStr:uint16:1:,CellVMinMod:uint16:1:,CellVAvg:uint16:1:CellV_SF,A:int16:1:A_SF,AChaMax:uint16:1:AMax_SF,ADisChaMax:uint16:1:AMax_SF,W:int16:1:W_SF,ReqInvState:enum16:1:,ReqW:int16:1:W_SF,SetOp:enum16:1:,SetInvState:enum16:1:,AHRtg_SF:sunssf:1:,WHRtg_SF:sunssf:1:,WChaDisChaMax_SF:sunssf:1:,DisChaRte_SF:sunssf:1:,SoC_SF:sunssf:1:,DoD_SF:sunssf:1:,SoH_SF:sunssf:1:,V_SF:sunssf:1:,CellV_SF:sunssf:1:,A_SF:sunssf:1:,AMax_SF:sunssf:1:,W_SF:sunssf:1:", ",")
	got := make([]string, len(definition.points))
	for index, point := range definition.points {
		got[index] = fmt.Sprintf("%s:%s:%d:%s", point.name, point.pointType, point.size, point.scaleFactor)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Model 802 signature=%q want=%q", got, want)
	}
}

func TestSunSpecV2BESSModuleRetainsPinnedPointOrderTypesAndScales(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.definition(SunSpecDecoderKey{ModelID: 805, ModelLength: 42, SchemaRevision: SunSpecModelsRevisionV2})
	if !ok {
		t.Fatal("Model 805/42 definition absent")
	}
	want := strings.Split("ID:uint16:1:,L:uint16:1:,StrIdx:uint16:1:,ModIdx:uint16:1:,NCell:uint16:1:,SoC:uint16:1:SoC_SF,DoD:uint16:1:DoD_SF,SoH:uint16:1:SoH_SF,NCyc:uint32:2:,V:uint16:1:V_SF,CellVMax:uint16:1:CellV_SF,CellVMaxCell:uint16:1:,CellVMin:uint16:1:CellV_SF,CellVMinCell:uint16:1:,CellVAvg:uint16:1:CellV_SF,CellTmpMax:int16:1:Tmp_SF,CellTmpMaxCell:uint16:1:,CellTmpMin:int16:1:Tmp_SF,CellTmpMinCell:uint16:1:,CellTmpAvg:int16:1:Tmp_SF,NCellBal:uint16:1:,SN:string:16:,SoC_SF:sunssf:1:,SoH_SF:sunssf:1:,DoD_SF:sunssf:1:,V_SF:sunssf:1:,CellV_SF:sunssf:1:,Tmp_SF:sunssf:1:", ",")
	got := make([]string, len(definition.points))
	for index, point := range definition.points {
		got[index] = fmt.Sprintf("%s:%s:%d:%s", point.name, point.pointType, point.size, point.scaleFactor)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Model 805 signature=%q want=%q", got, want)
	}
	words := v2DERModelWords(t, registry, 805, 42, map[string][]uint16{
		"StrIdx": {1}, "ModIdx": {2}, "NCell": {12}, "SoC": {74}, "SoC_SF": {0},
		"SN": stringWords("synthetic", 16),
	})
	key := SunSpecDecoderKey{ModelID: 805, ModelLength: 42, SchemaRevision: SunSpecModelsRevisionV2}
	decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 805, ModelLength: 42}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words})
	if err != nil || !decoded.GeometryValid() || !decoded.Qualifies() {
		t.Fatalf("Model 805 observed state geometry=%t qualifies=%t err=%v", decoded.GeometryValid(), decoded.Qualifies(), err)
	}
	for _, fieldID := range []string{"sunspec.der.v2.805.StrIdx", "sunspec.der.v2.805.ModIdx", "sunspec.der.v2.805.SN"} {
		if _, ok := decoded.Fact(fieldID); !ok {
			t.Fatalf("Model 805 observed fact %q absent", fieldID)
		}
	}
}

func TestSunSpecV2FlowBatteryRetainsStructuralObservedState(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.definition(SunSpecDecoderKey{ModelID: 806, ModelLength: 1, SchemaRevision: SunSpecModelsRevisionV2})
	if !ok {
		t.Fatal("Model 806/1 definition absent")
	}
	want := []string{"ID:uint16:1::0:false", "L:uint16:1::0:false", "BatTBD:uint16:1::0:false"}
	got := make([]string, len(definition.points))
	for index, point := range definition.points {
		got[index] = fmt.Sprintf("%s:%s:%d:%s:%d:%t", point.name, point.pointType, point.size, point.scaleFactor, point.repeatIndex, point.repeated)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Model 806 signature=%q want=%q", got, want)
	}
	words := v2DERModelWords(t, registry, 806, 1, map[string][]uint16{"BatTBD": {7}})
	key := SunSpecDecoderKey{ModelID: 806, ModelLength: 1, SchemaRevision: SunSpecModelsRevisionV2}
	decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 806, ModelLength: 1}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words})
	if err != nil || !decoded.GeometryValid() || !decoded.Qualifies() {
		t.Fatalf("Model 806 observed state geometry=%t qualifies=%t err=%v", decoded.GeometryValid(), decoded.Qualifies(), err)
	}
	if _, ok := decoded.Fact("sunspec.der.v2.806.BatTBD"); !ok {
		t.Fatal("Model 806 structural observed fact absent")
	}
}

func TestSunSpecV2FlowBatteryStringRetainsObservedStaticBase(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.definition(SunSpecDecoderKey{ModelID: 807, ModelLength: 34, SchemaRevision: SunSpecModelsRevisionV2})
	if !ok {
		t.Fatal("Model 807/34 definition absent")
	}
	if len(definition.points) != 32 {
		t.Fatalf("Model 807 points=%d", len(definition.points))
	}
	want := strings.Split("ID:uint16:1:,L:uint16:1:,Idx:uint16:1:,NMod:uint16:1:,NModCon:uint16:1:,ModVMax:uint16:1:ModV_SF,ModVMaxMod:uint16:1:,ModVMin:uint16:1:ModV_SF,ModVMinMod:uint16:1:,ModVAvg:uint16:1:ModV_SF,CellVMax:uint16:1:CellV_SF,CellVMaxMod:uint16:1:,CellVMaxStk:uint16:1:,CellVMin:uint16:1:CellV_SF,CellVMinMod:uint16:1:,CellVMinStk:uint16:1:,CellVAvg:uint16:1:CellV_SF,TmpMax:int16:1:Tmp_SF,TmpMaxMod:uint16:1:,TmpMin:int16:1:Tmp_SF,TmpMinMod:uint16:1:,TmpAvg:int16:1:Tmp_SF,Evt1:bitfield32:2:,Evt2:bitfield32:2:,EvtVnd1:bitfield32:2:,EvtVnd2:bitfield32:2:,ModV_SF:sunssf:1:,CellV_SF:sunssf:1:,Tmp_SF:sunssf:1:,SoC_SF:sunssf:1:,OCV_SF:sunssf:1:,Pad1:pad:1:", ",")
	got := make([]string, len(definition.points))
	for index, point := range definition.points {
		got[index] = fmt.Sprintf("%s:%s:%d:%s", point.name, point.pointType, point.size, point.scaleFactor)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Model 807 signature=%q want=%q", got, want)
	}
	for _, point := range definition.points {
		if point.repeated || point.repeatIndex != 0 {
			t.Fatalf("Model 807 materialized group point=%#v", point)
		}
	}
	words := v2DERModelWords(t, registry, 807, 34, map[string][]uint16{
		"Idx": {1}, "NMod": {2}, "NModCon": {1}, "ModVMax": {600}, "ModVMin": {500},
		"TmpMax": {25}, "TmpMin": {10}, "TmpAvg": {18}, "Evt1": {0x8000, 0},
		"ModV_SF": {0}, "CellV_SF": {0}, "Tmp_SF": {0}, "SoC_SF": {0}, "OCV_SF": {0},
	})
	key := SunSpecDecoderKey{ModelID: 807, ModelLength: 34, SchemaRevision: SunSpecModelsRevisionV2}
	decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 807, ModelLength: 34}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words})
	if err != nil || !decoded.GeometryValid() || !decoded.Qualifies() {
		t.Fatalf("Model 807 observed state geometry=%t qualifies=%t err=%v", decoded.GeometryValid(), decoded.Qualifies(), err)
	}
	for _, fieldID := range []string{"sunspec.der.v2.807.Idx", "sunspec.der.v2.807.NMod", "sunspec.der.v2.807.ModVMax", "sunspec.der.v2.807.Evt1"} {
		if _, ok := decoded.Fact(fieldID); !ok {
			t.Fatalf("Model 807 observed fact %q absent", fieldID)
		}
	}
	for _, fieldID := range []string{"sunspec.der.v2.807.ModSetEna", "sunspec.der.v2.807.ModSetCon"} {
		if _, ok := decoded.Fact(fieldID); ok {
			t.Fatalf("Model 807 zero-count module field %q materialized", fieldID)
		}
	}
}

func TestSunSpecV2FlowBatteryModuleRetainsObservedStaticBase(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.definition(SunSpecDecoderKey{ModelID: 808, ModelLength: 1, SchemaRevision: SunSpecModelsRevisionV2})
	if !ok {
		t.Fatal("Model 808/1 definition absent")
	}
	want := []string{"ID:uint16:1::0:false", "L:uint16:1::0:false", "ModuleTBD:uint16:1::0:false"}
	got := make([]string, len(definition.points))
	for index, point := range definition.points {
		got[index] = fmt.Sprintf("%s:%s:%d:%s:%d:%t", point.name, point.pointType, point.size, point.scaleFactor, point.repeatIndex, point.repeated)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Model 808 signature=%q want=%q", got, want)
	}
	words := v2DERModelWords(t, registry, 808, 1, map[string][]uint16{"ModuleTBD": {9}})
	key := SunSpecDecoderKey{ModelID: 808, ModelLength: 1, SchemaRevision: SunSpecModelsRevisionV2}
	decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 808, ModelLength: 1}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words})
	if err != nil || !decoded.GeometryValid() || !decoded.Qualifies() {
		t.Fatalf("Model 808 observed state geometry=%t qualifies=%t err=%v", decoded.GeometryValid(), decoded.Qualifies(), err)
	}
	if _, ok := decoded.Fact("sunspec.der.v2.808.ModuleTBD"); !ok {
		t.Fatal("Model 808 structural observed fact absent")
	}
	if _, ok := decoded.Fact("sunspec.der.v2.808.StackTBD"); ok {
		t.Fatal("Model 808 zero-count stack field materialized")
	}
}

func TestSunSpecV2FlowBatteryStackRetainsObservedStaticBase(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.definition(SunSpecDecoderKey{ModelID: 809, ModelLength: 1, SchemaRevision: SunSpecModelsRevisionV2})
	if !ok {
		t.Fatal("Model 809/1 definition absent")
	}
	want := []string{"ID:uint16:1::0:false", "L:uint16:1::0:false", "StackTBD:uint16:1::0:false"}
	got := make([]string, len(definition.points))
	for index, point := range definition.points {
		got[index] = fmt.Sprintf("%s:%s:%d:%s:%d:%t", point.name, point.pointType, point.size, point.scaleFactor, point.repeatIndex, point.repeated)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Model 809 signature=%q want=%q", got, want)
	}
	words := v2DERModelWords(t, registry, 809, 1, map[string][]uint16{"StackTBD": {11}})
	key := SunSpecDecoderKey{ModelID: 809, ModelLength: 1, SchemaRevision: SunSpecModelsRevisionV2}
	decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 809, ModelLength: 1}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words})
	if err != nil || !decoded.GeometryValid() || !decoded.Qualifies() {
		t.Fatalf("Model 809 observed state geometry=%t qualifies=%t err=%v", decoded.GeometryValid(), decoded.Qualifies(), err)
	}
	if _, ok := decoded.Fact("sunspec.der.v2.809.StackTBD"); !ok {
		t.Fatal("Model 809 structural observed fact absent")
	}
	if _, ok := decoded.Fact("sunspec.der.v2.809.CellTBD"); ok {
		t.Fatal("Model 809 zero-count cell field materialized")
	}
}

func TestSunSpecV2BESSBankRequiresExactDynamicDefinitions(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		strings, length uint16
		points          int
	}{
		{0, 26, 28},
		{2, 90, 80},
		{2047, 65530, 28 + 26*2047},
	} {
		key := SunSpecDecoderKey{ModelID: 803, ModelLength: want.length, SchemaRevision: SunSpecModelsRevisionV2}
		definition, ok := registry.definition(key)
		if !ok || len(definition.points) != want.points {
			t.Fatalf("Model 803/%d dynamic definition=%t points=%d", want.length, ok, len(definition.points))
		}
		if want.strings == 2047 {
			continue
		}
		words := make([]uint16, int(want.length)+2)
		words[0], words[1], words[2] = 803, want.length, want.strings
		spans := []SunSpecSourceSpan{{LogicalViewID: 11, PDUOffset: 0, WordCount: want.length + 2}}
		decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 803, ModelLength: want.length}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words, spans: spans})
		if err != nil || !decoded.GeometryValid() || !decoded.Qualifies() {
			t.Fatalf("Model 803/%d geometry=%t qualifies=%t err=%v", want.length, decoded.GeometryValid(), decoded.Qualifies(), err)
		}
		if want.strings == 2 {
			counts := map[uint16]int{}
			for _, fact := range decoded.Facts() {
				if fact.Repeated && fact.GroupID == "string" {
					counts[fact.RepeatIndex]++
				}
			}
			if counts[1] != 26 || counts[2] != 26 || len(counts) != 2 || !reflect.DeepEqual(decoded.SourceSpans(), spans) {
				t.Fatalf("Model 803 repeat facts=%v spans=%#v", counts, decoded.SourceSpans())
			}
		}
	}
	for _, want := range []struct {
		name          string
		length, count uint16
	}{
		{"count mismatch", 90, 1},
		{"sentinel", 90, 0xffff},
	} {
		t.Run(want.name, func(t *testing.T) {
			key := SunSpecDecoderKey{ModelID: 803, ModelLength: want.length, SchemaRevision: SunSpecModelsRevisionV2}
			words := make([]uint16, int(want.length)+2)
			words[0], words[1], words[2] = 803, want.length, want.count
			decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 803, ModelLength: want.length}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words})
			if err != nil || decoded.GeometryValid() || decoded.Qualifies() || len(decoded.Facts()) != 0 || !reflect.DeepEqual(decoded.RawWords(), words) {
				t.Fatalf("Model 803 %s geometry=%t qualifies=%t facts=%d err=%v", want.name, decoded.GeometryValid(), decoded.Qualifies(), len(decoded.Facts()), err)
			}
		})
	}
}

func TestSunSpecV2BESSStringRequiresExactDynamicDefinitions(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		modules, length uint16
	}{
		{0, 46},
		{2, 78},
	} {
		key := SunSpecDecoderKey{ModelID: 804, ModelLength: want.length, SchemaRevision: SunSpecModelsRevisionV2}
		definition, ok := registry.definition(key)
		if !ok || len(definition.points) == 0 {
			t.Fatalf("Model 804/%d dynamic definition=%t points=%d", want.length, ok, len(definition.points))
		}
		words := make([]uint16, int(want.length)+2)
		words[0], words[1], words[3] = 804, want.length, want.modules
		spans := []SunSpecSourceSpan{{LogicalViewID: 12, PDUOffset: 0, WordCount: want.length + 2}}
		decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 804, ModelLength: want.length}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words, spans: spans})
		if err != nil || !decoded.GeometryValid() || !decoded.Qualifies() {
			t.Fatalf("Model 804/%d geometry=%t qualifies=%t err=%v", want.length, decoded.GeometryValid(), decoded.Qualifies(), err)
		}
		if want.modules == 2 {
			counts := map[uint16]int{}
			for _, fact := range decoded.Facts() {
				if fact.Repeated && fact.GroupID == "module" {
					counts[fact.RepeatIndex]++
				}
			}
			if counts[1] != 16 || counts[2] != 16 || len(counts) != 2 || !reflect.DeepEqual(decoded.SourceSpans(), spans) {
				t.Fatalf("Model 804 repeat facts=%v spans=%#v", counts, decoded.SourceSpans())
			}
		}
	}
	for _, want := range []struct {
		name          string
		length, count uint16
	}{
		{"count mismatch", 78, 1},
		{"sentinel", 78, 0xffff},
	} {
		t.Run(want.name, func(t *testing.T) {
			key := SunSpecDecoderKey{ModelID: 804, ModelLength: want.length, SchemaRevision: SunSpecModelsRevisionV2}
			words := make([]uint16, int(want.length)+2)
			words[0], words[1], words[3] = 804, want.length, want.count
			decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 804, ModelLength: want.length}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words})
			if err != nil || decoded.GeometryValid() || decoded.Qualifies() || len(decoded.Facts()) != 0 || !reflect.DeepEqual(decoded.RawWords(), words) {
				t.Fatalf("Model 804 %s geometry=%t qualifies=%t facts=%d err=%v", want.name, decoded.GeometryValid(), decoded.Qualifies(), len(decoded.Facts()), err)
			}
		})
	}
}

func TestSunSpecV2BESSStringDecodesFullMaximumOfflineOccurrence(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	key := SunSpecDecoderKey{ModelID: 804, ModelLength: 65534, SchemaRevision: SunSpecModelsRevisionV2}
	if _, ok := registry.definition(key); !ok {
		t.Fatal("Model 804/65534 definition absent")
	}
	words := make([]uint16, 65536)
	words[0], words[1], words[3] = 804, 65534, 4093
	spans := make([]SunSpecSourceSpan, 0, (len(words)+124)/125)
	for remaining := len(words); remaining > 0; {
		count := min(remaining, 125)
		spans = append(spans, SunSpecSourceSpan{LogicalViewID: uint64(len(spans) + 1), PDUOffset: 0, WordCount: uint16(count)})
		remaining -= count
	}
	decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 804, ModelLength: 65534}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words, spans: spans})
	if err != nil || !decoded.GeometryValid() || !decoded.Qualifies() {
		t.Fatalf("Model 804 maximum geometry=%t qualifies=%t err=%v", decoded.GeometryValid(), decoded.Qualifies(), err)
	}
	var extent uint32
	for _, span := range decoded.SourceSpans() {
		if span.WordCount == 0 || span.WordCount > 125 {
			t.Fatalf("maximum span=%#v", span)
		}
		extent += uint32(span.WordCount)
	}
	if len(decoded.SourceSpans()) < 2 || extent != 65536 {
		t.Fatalf("maximum spans=%d extent=%d", len(decoded.SourceSpans()), extent)
	}
	moduleFacts := map[uint16]int{}
	for _, fact := range decoded.Facts() {
		if fact.Repeated && fact.GroupID == "module" {
			moduleFacts[fact.RepeatIndex]++
		}
	}
	if moduleFacts[1] != 16 || moduleFacts[4093] != 16 || len(moduleFacts) != 4093 {
		t.Fatalf("maximum module facts first=%d last=%d groups=%d", moduleFacts[1], moduleFacts[4093], len(moduleFacts))
	}
	if _, ok := registry.definition(SunSpecDecoderKey{ModelID: 803, ModelLength: 26, SchemaRevision: SunSpecModelsRevisionV2}); !ok {
		t.Fatal("Model 803 lost independent V2 definition")
	}
}

func TestSunSpecV2BESSStringRetainsPinnedPointOrderTypesAndScales(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.definition(SunSpecDecoderKey{ModelID: 804, ModelLength: 62, SchemaRevision: SunSpecModelsRevisionV2})
	if !ok {
		t.Fatal("Model 804/62 definition absent")
	}
	want := strings.Split("ID:uint16:1::0:false,L:uint16:1::0:false,Idx:uint16:1::0:false,NMod:count:1::0:false,St:bitfield32:2::0:false,ConFail:enum16:1::0:false,NCellBal:uint16:1::0:false,SoC:uint16:1:SoC_SF:0:false,DoD:uint16:1:DoD_SF:0:false,NCyc:uint32:2::0:false,SoH:uint16:1:SoH_SF:0:false,A:int16:1:A_SF:0:false,V:uint16:1:V_SF:0:false,CellVMax:uint16:1:CellV_SF:0:false,CellVMaxMod:uint16:1::0:false,CellVMin:uint16:1:CellV_SF:0:false,CellVMinMod:uint16:1::0:false,CellVAvg:uint16:1:CellV_SF:0:false,ModTmpMax:int16:1:ModTmp_SF:0:false,ModTmpMaxMod:uint16:1::0:false,ModTmpMin:int16:1:ModTmp_SF:0:false,ModTmpMinMod:uint16:1::0:false,ModTmpAvg:int16:1:ModTmp_SF:0:false,Pad1:pad:1::0:false,ConSt:bitfield32:2::0:false,Evt1:bitfield32:2::0:false,Evt2:bitfield32:2::0:false,EvtVnd1:bitfield32:2::0:false,EvtVnd2:bitfield32:2::0:false,SetEna:enum16:1::0:false,SetCon:enum16:1::0:false,SoC_SF:sunssf:1::0:false,SoH_SF:sunssf:1::0:false,DoD_SF:sunssf:1::0:false,A_SF:sunssf:1::0:false,V_SF:sunssf:1::0:false,CellV_SF:sunssf:1::0:false,ModTmp_SF:sunssf:1::0:false,Pad2:pad:1::0:false,Pad3:pad:1::0:false,Pad4:pad:1::0:false,ModNCell:uint16:1::1:true,ModSoC:uint16:1:SoC_SF:1:true,ModSoH:uint16:1:SoH_SF:1:true,ModCellVMax:uint16:1:CellV_SF:1:true,ModCellVMaxCell:uint16:1::1:true,ModCellVMin:uint16:1:CellV_SF:1:true,ModCellVMinCell:uint16:1::1:true,ModCellVAvg:uint16:1:CellV_SF:1:true,ModCellTmpMax:int16:1:ModTmp_SF:1:true,ModCellTmpMaxCell:uint16:1::1:true,ModCellTmpMin:int16:1:ModTmp_SF:1:true,ModCellTmpMinCell:uint16:1::1:true,ModCellTmpAvg:int16:1:ModTmp_SF:1:true,Pad5:pad:1::1:true,Pad6:pad:1::1:true,Pad7:pad:1::1:true", ",")
	got := make([]string, len(definition.points))
	for index, point := range definition.points {
		got[index] = fmt.Sprintf("%s:%s:%d:%s:%d:%t", point.name, point.pointType, point.size, point.scaleFactor, point.repeatIndex, point.repeated)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Model 804 signature=%q want=%q", got, want)
	}
	words := v2DERModelWords(t, registry, 804, 62, map[string][]uint16{
		"NMod":   {1},
		"SetEna": {1},
		"SetCon": {2},
	})
	key := SunSpecDecoderKey{ModelID: 804, ModelLength: 62, SchemaRevision: SunSpecModelsRevisionV2}
	decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 804, ModelLength: 62}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words})
	if err != nil || !decoded.GeometryValid() || !decoded.Qualifies() {
		t.Fatalf("Model 804 observed state geometry=%t qualifies=%t err=%v", decoded.GeometryValid(), decoded.Qualifies(), err)
	}
	for _, fieldID := range []string{"sunspec.der.v2.804.Idx", "sunspec.der.v2.804.SetEna", "sunspec.der.v2.804.SetCon"} {
		if _, ok := decoded.Fact(fieldID); !ok {
			t.Fatalf("Model 804 observed fact %q absent", fieldID)
		}
	}
}

func TestSunSpecV2BESSBankRetainsPinnedPointOrderTypesAndScales(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.definition(SunSpecDecoderKey{ModelID: 803, ModelLength: 58, SchemaRevision: SunSpecModelsRevisionV2})
	if !ok {
		t.Fatal("Model 803/58 definition absent")
	}
	want := strings.Split("ID:uint16:1::0:false,L:uint16:1::0:false,NStr:count:1::0:false,NStrCon:uint16:1::0:false,ModTmpMax:int16:1:ModTmp_SF:0:false,ModTmpMaxStr:uint16:1::0:false,ModTmpMaxMod:uint16:1::0:false,ModTmpMin:int16:1:ModTmp_SF:0:false,ModTmpMinStr:uint16:1::0:false,ModTmpMinMod:uint16:1::0:false,ModTmpAvg:int16:1:ModTmp_SF:0:false,StrVMax:uint16:1:V_SF:0:false,StrVMaxStr:uint16:1::0:false,StrVMin:uint16:1:V_SF:0:false,StrVMinStr:uint16:1::0:false,StrVAvg:uint16:1:V_SF:0:false,StrAMax:int16:1:A_SF:0:false,StrAMaxStr:uint16:1::0:false,StrAMin:int16:1:A_SF:0:false,StrAMinStr:uint16:1::0:false,StrAAvg:int16:1:A_SF:0:false,NCellBal:uint16:1::0:false,CellV_SF:sunssf:1::0:false,ModTmp_SF:sunssf:1::0:false,A_SF:sunssf:1::0:false,SoH_SF:sunssf:1::0:false,SoC_SF:sunssf:1::0:false,V_SF:sunssf:1::0:false,StrNMod:uint16:1::1:true,StrSt:bitfield32:2::1:true,StrConFail:enum16:1::1:true,StrSoC:uint16:1:SoC_SF:1:true,StrSoH:uint16:1:SoH_SF:1:true,StrA:int16:1:A_SF:1:true,StrCellVMax:uint16:1:CellV_SF:1:true,StrCellVMaxMod:uint16:1::1:true,StrCellVMin:uint16:1:CellV_SF:1:true,StrCellVMinMod:uint16:1::1:true,StrCellVAvg:uint16:1:CellV_SF:1:true,StrModTmpMax:int16:1:ModTmp_SF:1:true,StrModTmpMaxMod:uint16:1::1:true,StrModTmpMin:int16:1:ModTmp_SF:1:true,StrModTmpMinMod:uint16:1::1:true,StrModTmpAvg:int16:1:ModTmp_SF:1:true,StrDisRsn:enum16:1::1:true,StrConSt:bitfield32:2::1:true,StrEvt1:bitfield32:2::1:true,StrEvt2:bitfield32:2::1:true,StrEvtVnd1:bitfield32:2::1:true,StrEvtVnd2:bitfield32:2::1:true,StrSetEna:enum16:1::1:true,StrSetCon:enum16:1::1:true,Pad1:pad:1::1:true,Pad2:pad:1::1:true", ",")
	got := make([]string, len(definition.points))
	for index, point := range definition.points {
		got[index] = fmt.Sprintf("%s:%s:%d:%s:%d:%t", point.name, point.pointType, point.size, point.scaleFactor, point.repeatIndex, point.repeated)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Model 803 signature=%q want=%q", got, want)
	}
}

func TestSunSpecV2ControlObservabilityDecodesStateOnly(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		id, length uint16
		values     map[string][]uint16
		field      string
	}{
		{703, 17, map[string][]uint16{"ES": {1}, "ESVHi": {2300}, "V_SF": {0}, "ESHzHi": {5000}, "Hz_SF": {0}}, "sunspec.der.v2.703.ES"},
		{715, 7, map[string][]uint16{"LocRemCtl": {1}, "DERHb": {0, 7}, "AlarmReset": {1}, "OpCtl": {2}}, "sunspec.der.v2.715.AlarmReset"},
		{802, 62, map[string][]uint16{"AHRtg": {100}, "AHRtg_SF": {0}, "AlmRst": {1}, "SetOp": {2}, "SetInvState": {3}}, "sunspec.der.v2.802.AlmRst"},
	} {
		words := v2DERModelWords(t, registry, want.id, want.length, want.values)
		key := SunSpecDecoderKey{ModelID: want.id, ModelLength: want.length, SchemaRevision: SunSpecModelsRevisionV2}
		decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: want.id, ModelLength: want.length}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words})
		if err != nil || !decoded.GeometryValid() || !decoded.Qualifies() {
			t.Fatalf("Model %d/%d decode geometry=%v qualifies=%v err=%v", want.id, want.length, decoded.GeometryValid(), decoded.Qualifies(), err)
		}
		if _, ok := decoded.Fact(want.field); !ok {
			t.Fatalf("Model %d missing observed fact %q", want.id, want.field)
		}
	}
}

func TestSunSpecV2DERMeasurePreservesScaledUnsignedCounter(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	words := v2DERModelWords(t, registry, 701, 153, map[string][]uint16{
		"ACType":     {2},
		"TotWhInj":   {0x8000, 0, 0, 1},
		"TotWh_SF":   {0xffff},
		"MnAlrmInfo": {0x0080, 0},
	})
	key := SunSpecDecoderKey{ModelID: 701, ModelLength: 153, SchemaRevision: SunSpecModelsRevisionV2}
	decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{
		Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 701, ModelLength: 153}, SchemaRevision: SunSpecModelsRevisionV2,
		Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words,
	})
	if err != nil {
		t.Fatal(err)
	}
	fact, ok := decoded.Fact("sunspec.der.v2.701.TotWhInj")
	if !ok {
		t.Fatal("scaled unsigned counter absent")
	}
	decimal, ok := fact.Value.UnsignedDecimal()
	if !ok || decimal != (SunSpecUnsignedDecimal{Coefficient: 0x8000000000000001, Exponent: -1}) {
		t.Fatalf("unsigned decimal=%#v present=%v", decimal, ok)
	}
}

func TestSunSpecV2DERStoragePreservesValueOrderAndScale(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	words := v2DERModelWords(t, registry, 713, 7, map[string][]uint16{
		"WHRtg":   {100},
		"WHAvail": {50},
		"SoC":     {75},
		"SoH":     {95},
		"Sta":     {3},
		"WH_SF":   {0},
		"Pct_SF":  {0xffff},
	})
	key := SunSpecDecoderKey{ModelID: 713, ModelLength: 7, SchemaRevision: SunSpecModelsRevisionV2}
	decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{
		Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 713, ModelLength: 7}, SchemaRevision: SunSpecModelsRevisionV2,
		Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		field       string
		coefficient int64
		exponent    int16
	}{
		{"sunspec.der.v2.713.WHRtg", 100, 0},
		{"sunspec.der.v2.713.WHAvail", 50, 0},
		{"sunspec.der.v2.713.SoC", 75, -1},
		{"sunspec.der.v2.713.SoH", 95, -1},
	} {
		fact, ok := decoded.Fact(want.field)
		if !ok {
			t.Fatalf("fact %q absent", want.field)
		}
		value, ok := fact.Value.Decimal()
		if !ok || value != (SunSpecDecimal{Coefficient: want.coefficient, Exponent: want.exponent}) {
			t.Fatalf("fact %q decimal=%#v present=%v", want.field, value, ok)
		}
	}
}

func TestSunSpecV2DERPortGeometryIsBoundedAndRawOnlyOnMismatch(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		ports, length uint16
		points, facts int
	}{
		{0, 18, 13, 11},
		{2, 68, 35, 33},
		{uint16(maxSunSpecDERPorts), 65518, 13 + 11*int(maxSunSpecDERPorts), 11 + 11*int(maxSunSpecDERPorts)},
	} {
		definition, ok := registry.definition(SunSpecDecoderKey{ModelID: 714, ModelLength: want.length, SchemaRevision: SunSpecModelsRevisionV2})
		if !ok || len(definition.points) != want.points {
			t.Fatalf("N=%d definition=%v points=%d", want.ports, ok, len(definition.points))
		}
		if uint32(want.ports) == maxSunSpecDERPorts {
			continue
		}
		words := make([]uint16, int(want.length)+2)
		words[0], words[1], words[4] = 714, want.length, want.ports
		key := SunSpecDecoderKey{ModelID: 714, ModelLength: want.length, SchemaRevision: SunSpecModelsRevisionV2}
		decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 714, ModelLength: want.length}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words, spans: []SunSpecSourceSpan{{LogicalViewID: 7, PDUOffset: 0, WordCount: want.length + 2}}})
		if err != nil || !decoded.GeometryValid() || len(decoded.Facts()) != want.facts {
			t.Fatalf("N=%d geometry=%v facts=%d err=%v", want.ports, decoded.GeometryValid(), len(decoded.Facts()), err)
		}
		if want.ports == 2 {
			ports := 0
			for _, fact := range decoded.Facts() {
				if fact.Repeated && fact.GroupID == "port" && fact.RepeatIndex >= 1 && fact.RepeatIndex <= 2 {
					ports++
				}
			}
			if ports != 22 || len(decoded.SourceSpans()) != 1 {
				t.Fatalf("repeated facts=%d spans=%#v", ports, decoded.SourceSpans())
			}
		}
	}
	for _, want := range []struct {
		name          string
		length, ports uint16
	}{
		{name: "count mismatch", length: 68, ports: 1},
		{name: "sentinel", length: 68, ports: 0xffff},
		{name: "partial port group", length: 43, ports: 2},
	} {
		t.Run(want.name, func(t *testing.T) {
			words := make([]uint16, int(want.length)+2)
			words[0], words[1], words[4] = 714, want.length, want.ports
			key := SunSpecDecoderKey{ModelID: 714, ModelLength: want.length, SchemaRevision: SunSpecModelsRevisionV2}
			decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 714, ModelLength: want.length}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words})
			if err != nil || decoded.GeometryValid() || decoded.Qualifies() || len(decoded.Facts()) != 0 || !reflect.DeepEqual(decoded.RawWords(), words) {
				t.Fatalf("geometry=%v qualifies=%v facts=%d err=%v", decoded.GeometryValid(), decoded.Qualifies(), len(decoded.Facts()), err)
			}
		})
	}
}

func TestSunSpecV2DERTripLVGeometryIsBoundedAndOfflineOnly(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		curves, length uint16
		points, facts  int
	}{
		{0, 7, 9, 7},
		{2, 9, 11, 9},
		{uint16(maxSunSpecDERTripLVCurves), 65534, 65536, 65534},
	} {
		key := SunSpecDecoderKey{ModelID: 707, ModelLength: want.length, SchemaRevision: SunSpecModelsRevisionV2}
		definition, ok := registry.definition(key)
		if !ok || len(definition.points) != want.points {
			t.Fatalf("N=%d definition=%v points=%d", want.curves, ok, len(definition.points))
		}
		if uint32(want.curves) == maxSunSpecDERTripLVCurves {
			continue
		}
		words := make([]uint16, int(want.length)+2)
		words[0], words[1], words[6] = 707, want.length, want.curves
		decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 707, ModelLength: want.length}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words, spans: []SunSpecSourceSpan{{LogicalViewID: 7, PDUOffset: 0, WordCount: want.length + 2}}})
		if err != nil || !decoded.GeometryValid() || len(decoded.Facts()) != want.facts {
			t.Fatalf("N=%d geometry=%v facts=%d err=%v", want.curves, decoded.GeometryValid(), len(decoded.Facts()), err)
		}
	}

	for _, want := range []struct {
		name          string
		length, count uint16
	}{
		{name: "count mismatch", length: 9, count: 1},
		{name: "unavailable count", length: 9, count: 0xffff},
		{name: "partial curve group", length: 8, count: 2},
	} {
		t.Run(want.name, func(t *testing.T) {
			key := SunSpecDecoderKey{ModelID: 707, ModelLength: want.length, SchemaRevision: SunSpecModelsRevisionV2}
			words := make([]uint16, int(want.length)+2)
			words[0], words[1], words[6] = 707, want.length, want.count
			decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 707, ModelLength: want.length}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &key, words: words})
			if err != nil || decoded.GeometryValid() || decoded.Qualifies() || len(decoded.Facts()) != 0 || !reflect.DeepEqual(decoded.RawWords(), words) {
				t.Fatalf("geometry=%v qualifies=%v facts=%d err=%v", decoded.GeometryValid(), decoded.Qualifies(), len(decoded.Facts()), err)
			}
		})
	}

	maxKey := SunSpecDecoderKey{ModelID: 707, ModelLength: 65534, SchemaRevision: SunSpecModelsRevisionV2}
	words := make([]uint16, 65536)
	words[0], words[1], words[6] = 707, 65534, uint16(maxSunSpecDERTripLVCurves)
	spans := make([]SunSpecSourceSpan, 0, (len(words)+124)/125)
	for remaining := len(words); remaining > 0; {
		count := min(remaining, 125)
		spans = append(spans, SunSpecSourceSpan{LogicalViewID: uint64(len(spans) + 1), PDUOffset: 0, WordCount: uint16(count)})
		remaining -= count
	}
	decoded, err := registry.DecodeOccurrence(SunSpecOccurrence{Ordinal: 1, WireKey: SunSpecWireKey{ModelID: 707, ModelLength: 65534}, SchemaRevision: SunSpecModelsRevisionV2, Disposition: SunSpecChainDispositionAdmitted, decoderKey: &maxKey, words: words, spans: spans})
	if err != nil || !decoded.GeometryValid() || !decoded.Qualifies() || len(decoded.Facts()) != 65534 {
		t.Fatalf("maximum geometry=%t qualifies=%t facts=%d err=%v", decoded.GeometryValid(), decoded.Qualifies(), len(decoded.Facts()), err)
	}
	var extent uint32
	for _, span := range decoded.SourceSpans() {
		if span.WordCount == 0 || span.WordCount > 125 {
			t.Fatalf("maximum span=%#v", span)
		}
		extent += uint32(span.WordCount)
	}
	if extent != 65536 {
		t.Fatalf("maximum span extent=%d", extent)
	}
	if _, ok := registry.definition(SunSpecDecoderKey{ModelID: 707, ModelLength: 65535, SchemaRevision: SunSpecModelsRevisionV2}); ok {
		t.Fatal("over-boundary Model 707 must remain opaque")
	}
}

func v2DERModelWords(t *testing.T, registry SunSpecDecoderRegistry, id, length uint16, values map[string][]uint16) []uint16 {
	t.Helper()
	key := SunSpecDecoderKey{ModelID: id, ModelLength: length, SchemaRevision: SunSpecModelsRevisionV2}
	definition, ok := registry.definition(key)
	if !ok {
		t.Fatalf("definition %d/%d absent", id, length)
	}
	words := make([]uint16, int(length)+2)
	words[0], words[1] = id, length
	for _, point := range definition.points {
		value, exists := values[point.name]
		if !exists || point.name == "ID" || point.name == "L" {
			continue
		}
		if len(value) > int(point.size) {
			t.Fatalf("point %s words=%d exceeds=%d", point.name, len(value), point.size)
		}
		copy(words[point.offset:point.offset+point.size], value)
	}
	return words
}
