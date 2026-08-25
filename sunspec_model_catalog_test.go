package modbusreg

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
)

func TestSunSpecModelCatalogMatchesPinnedAuthoritativeShapes(t *testing.T) {
	data, err := os.ReadFile("testdata/sunspec/models/v1/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SourceCommit   string                `json:"source_commit"`
		SchemaRevision SunSpecSchemaRevision `json:"schema_revision"`
		Models         []struct {
			ID, Length, Points uint16
			Compatibility      bool `json:"compatibility"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SourceCommit != "7abdf8982d5364f8ae916deee18aac86c11be36d" || fixture.SchemaRevision != testSunSpecModelsRevision {
		t.Fatalf("unpinned fixture: %#v", fixture)
	}
	registry, err := NewStandardSunSpecDecoderRegistry(fixture.SchemaRevision)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range fixture.Models {
		definition, ok := registry.definition(SunSpecDecoderKey{expected.ID, expected.Length, fixture.SchemaRevision})
		if !ok {
			t.Fatalf("model %d/%d absent", expected.ID, expected.Length)
		}
		if len(definition.points) != int(expected.Points) || definition.compatibility != expected.Compatibility {
			t.Fatalf("model %d/%d points=%d compatibility=%v", expected.ID, expected.Length, len(definition.points), definition.compatibility)
		}
		var end uint16
		for _, point := range definition.points {
			if point.offset != end || point.size == 0 {
				t.Fatalf("model %d point %#v is not contiguous", expected.ID, point)
			}
			end += point.size
		}
		if end != expected.Length+2 {
			t.Fatalf("model %d/%d extent=%d", expected.ID, expected.Length, end)
		}
	}
}

func TestSunSpecV2CatalogContainsOnlyCurrentAdmittedModels(t *testing.T) {
	definitions, err := standardSunSpecModelDefinitions(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatalf("V2 catalog: %v", err)
	}
	if len(definitions) != 7 {
		t.Fatalf("V2 definitions=%d", len(definitions))
	}
	want := []SunSpecDecoderKey{
		{ModelID: 1, ModelLength: 66, SchemaRevision: SunSpecModelsRevisionV2},
		{ModelID: 701, ModelLength: 153, SchemaRevision: SunSpecModelsRevisionV2},
		{ModelID: 702, ModelLength: 50, SchemaRevision: SunSpecModelsRevisionV2},
		{ModelID: 703, ModelLength: 17, SchemaRevision: SunSpecModelsRevisionV2},
		{ModelID: 713, ModelLength: 7, SchemaRevision: SunSpecModelsRevisionV2},
		{ModelID: 715, ModelLength: 7, SchemaRevision: SunSpecModelsRevisionV2},
		{ModelID: 802, ModelLength: 62, SchemaRevision: SunSpecModelsRevisionV2},
	}
	for index, key := range want {
		if definitions[index].key != key || definitions[index].compatibility {
			t.Fatalf("unexpected V2 definition %d: %#v", index, definitions[index].key)
		}
	}
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []SunSpecDecoderKey{
		{ModelID: 1, ModelLength: 65, SchemaRevision: SunSpecModelsRevisionV2},
		{ModelID: 703, ModelLength: 16, SchemaRevision: SunSpecModelsRevisionV2},
		{ModelID: 715, ModelLength: 8, SchemaRevision: SunSpecModelsRevisionV2},
		{ModelID: 801, ModelLength: 1, SchemaRevision: SunSpecModelsRevisionV2},
		{ModelID: 802, ModelLength: 61, SchemaRevision: SunSpecModelsRevisionV2},
		{ModelID: 803, ModelLength: 27, SchemaRevision: SunSpecModelsRevisionV2},
		{ModelID: 704, ModelLength: 65, SchemaRevision: SunSpecModelsRevisionV2},
		{ModelID: 712, ModelLength: 14, SchemaRevision: SunSpecModelsRevisionV2},
	} {
		if _, ok := registry.definition(key); ok {
			t.Fatalf("V2 admitted excluded tuple %#v", key)
		}
	}
}

func TestSunSpecPinnedPointOrderAndTypes(t *testing.T) {
	common := "ID:uint16:1,L:uint16:1,Mn:string:16,Md:string:16,Opt:string:8,Vr:string:8,SN:string:16,DA:uint16:1,Pad:pad:1"
	integer := "ID:uint16:1,L:uint16:1,A:uint16:1,AphA:uint16:1,AphB:uint16:1,AphC:uint16:1,A_SF:sunssf:1,PPVphAB:uint16:1,PPVphBC:uint16:1,PPVphCA:uint16:1,PhVphA:uint16:1,PhVphB:uint16:1,PhVphC:uint16:1,V_SF:sunssf:1,W:int16:1,W_SF:sunssf:1,Hz:uint16:1,Hz_SF:sunssf:1,VA:int16:1,VA_SF:sunssf:1,VAr:int16:1,VAr_SF:sunssf:1,PF:int16:1,PF_SF:sunssf:1,WH:acc32:2,WH_SF:sunssf:1,DCA:uint16:1,DCA_SF:sunssf:1,DCV:uint16:1,DCV_SF:sunssf:1,DCW:int16:1,DCW_SF:sunssf:1,TmpCab:int16:1,TmpSnk:int16:1,TmpTrns:int16:1,TmpOt:int16:1,Tmp_SF:sunssf:1,St:enum16:1,StVnd:enum16:1,Evt1:bitfield32:2,Evt2:bitfield32:2,EvtVnd1:bitfield32:2,EvtVnd2:bitfield32:2,EvtVnd3:bitfield32:2,EvtVnd4:bitfield32:2"
	float := "ID:uint16:1,L:uint16:1,A:float32:2,AphA:float32:2,AphB:float32:2,AphC:float32:2,PPVphAB:float32:2,PPVphBC:float32:2,PPVphCA:float32:2,PhVphA:float32:2,PhVphB:float32:2,PhVphC:float32:2,W:float32:2,Hz:float32:2,VA:float32:2,VAr:float32:2,PF:float32:2,WH:float32:2,DCA:float32:2,DCV:float32:2,DCW:float32:2,TmpCab:float32:2,TmpSnk:float32:2,TmpTrns:float32:2,TmpOt:float32:2,St:enum16:1,StVnd:enum16:1,Evt1:bitfield32:2,Evt2:bitfield32:2,EvtVnd1:bitfield32:2,EvtVnd2:bitfield32:2,EvtVnd3:bitfield32:2,EvtVnd4:bitfield32:2"
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id, length uint16
		want       string
	}{
		{1, 66, common}, {101, 50, integer}, {102, 50, integer}, {103, 50, integer},
		{111, 60, float}, {112, 60, float}, {113, 60, float},
	} {
		definition, ok := registry.definition(SunSpecDecoderKey{tc.id, tc.length, testSunSpecModelsRevision})
		if !ok {
			t.Fatalf("definition %d absent", tc.id)
		}
		got := make([]string, len(definition.points))
		for index, point := range definition.points {
			got[index] = fmt.Sprintf("%s:%s:%d", point.name, point.pointType, point.size)
		}
		if joined := joinSunSpecCatalog(got); joined != tc.want {
			t.Fatalf("model %d catalog\n%s\nwant\n%s", tc.id, joined, tc.want)
		}
	}
	l65, _ := registry.definition(SunSpecDecoderKey{1, 65, testSunSpecModelsRevision})
	l66, _ := registry.definition(SunSpecDecoderKey{1, 66, testSunSpecModelsRevision})
	if !reflect.DeepEqual(l65.points, l66.points[:len(l66.points)-1]) || l66.points[len(l66.points)-1].name != "Pad" {
		t.Fatal("Model 1 L65 compatibility is not exactly L66 without Pad")
	}
}

func joinSunSpecCatalog(values []string) string {
	if len(values) == 0 {
		return ""
	}
	result := values[0]
	for _, value := range values[1:] {
		result += "," + value
	}
	return result
}

func TestSunSpecExpandedCatalogMatchesEveryPinnedPoint(t *testing.T) {
	data, err := os.ReadFile("testdata/sunspec/models/v1/expanded_catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	type fixtureSymbol struct {
		Name  string
		Value uint64
	}
	type fixturePoint struct {
		Name, Type, Unit, SF string
		Size                 uint16
		Mandatory            bool
		Symbols              []fixtureSymbol
	}
	type fixtureGroup struct {
		Name       string
		WordLength uint16 `json:"word_length"`
		Points     []fixturePoint
	}
	var fixture struct {
		SourceCommit   string                `json:"source_commit"`
		SchemaRevision SunSpecSchemaRevision `json:"schema_revision"`
		Models         []struct {
			ID             uint16
			Points         []fixturePoint
			RepeatingGroup *fixtureGroup `json:"repeating_group"`
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SourceCommit != "7abdf8982d5364f8ae916deee18aac86c11be36d" || fixture.SchemaRevision != testSunSpecModelsRevision || len(fixture.Models) != 15 {
		t.Fatalf("expanded fixture provenance=%q/%q models=%d", fixture.SourceCommit, fixture.SchemaRevision, len(fixture.Models))
	}
	registry, err := NewStandardSunSpecDecoderRegistry(fixture.SchemaRevision)
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range fixture.Models {
		expected := append([]fixturePoint(nil), model.Points...)
		var length uint16
		for _, point := range model.Points {
			length += point.Size
		}
		if model.RepeatingGroup != nil {
			length = model.RepeatingGroup.WordLength
			expected = append(expected, model.RepeatingGroup.Points...)
		} else {
			length -= 2
		}
		definition, ok := registry.definition(SunSpecDecoderKey{model.ID, length, fixture.SchemaRevision})
		if !ok || len(definition.points) != len(expected) {
			t.Fatalf("model %d/%d points=%d ok=%v want=%d", model.ID, length, len(definition.points), ok, len(expected))
		}
		for index, want := range expected {
			got := definition.points[index]
			scale := got.scaleFactor
			if got.fixedScale != nil {
				scale = strconv.FormatInt(int64(*got.fixedScale), 10)
			}
			if got.name != want.Name || string(got.pointType) != want.Type || got.size != want.Size || got.unit != want.Unit || scale != want.SF || got.mandatory != want.Mandatory {
				t.Fatalf("model %d point %d=%#v scale=%q want=%#v", model.ID, index, got, scale, want)
			}
			symbols := make(map[uint64]string, len(want.Symbols))
			for _, symbol := range want.Symbols {
				symbols[symbol.Value] = symbol.Name
			}
			if !reflect.DeepEqual(got.symbols, symbols) && (len(got.symbols) != 0 || len(symbols) != 0) {
				t.Fatalf("model %d point %s symbols=%v want=%v", model.ID, got.name, got.symbols, symbols)
			}
			if model.RepeatingGroup != nil && index >= len(model.Points) {
				if !got.repeated || got.repeatIndex != 1 || got.groupID != model.RepeatingGroup.Name {
					t.Fatalf("model %d repeated point=%#v", model.ID, got)
				}
			} else if got.repeated || got.repeatIndex != 0 || got.groupID != "" {
				t.Fatalf("model %d fixed point=%#v", model.ID, got)
			}
		}
	}
}
