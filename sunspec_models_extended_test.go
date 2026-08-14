package modbusreg

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

func TestSunSpecExtendedModelCatalogMatchesPinnedShapes(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id, length uint16
		want       string
	}{
		{120, 26, "ID:uint16:1,L:uint16:1,DERTyp:enum16:1,WRtg:uint16:1,WRtg_SF:sunssf:1,VARtg:uint16:1,VARtg_SF:sunssf:1,VArRtgQ1:int16:1,VArRtgQ2:int16:1,VArRtgQ3:int16:1,VArRtgQ4:int16:1,VArRtg_SF:sunssf:1,ARtg:uint16:1,ARtg_SF:sunssf:1,PFRtgQ1:int16:1,PFRtgQ2:int16:1,PFRtgQ3:int16:1,PFRtgQ4:int16:1,PFRtg_SF:sunssf:1,WHRtg:uint16:1,WHRtg_SF:sunssf:1,AhrRtg:uint16:1,AhrRtg_SF:sunssf:1,MaxChaRte:uint16:1,MaxChaRte_SF:sunssf:1,MaxDisChaRte:uint16:1,MaxDisChaRte_SF:sunssf:1,Pad:pad:1"},
		{121, 30, "ID:uint16:1,L:uint16:1,WMax:uint16:1,VRef:uint16:1,VRefOfs:int16:1,VMax:uint16:1,VMin:uint16:1,VAMax:uint16:1,VArMaxQ1:int16:1,VArMaxQ2:int16:1,VArMaxQ3:int16:1,VArMaxQ4:int16:1,WGra:uint16:1,PFMinQ1:int16:1,PFMinQ2:int16:1,PFMinQ3:int16:1,PFMinQ4:int16:1,VArAct:enum16:1,ClcTotVA:enum16:1,MaxRmpRte:uint16:1,ECPNomHz:uint16:1,ConnPh:enum16:1,WMax_SF:sunssf:1,VRef_SF:sunssf:1,VRefOfs_SF:sunssf:1,VMinMax_SF:sunssf:1,VAMax_SF:sunssf:1,VArMax_SF:sunssf:1,WGra_SF:sunssf:1,PFMin_SF:sunssf:1,MaxRmpRte_SF:sunssf:1,ECPNomHz_SF:sunssf:1"},
		{122, 44, "ID:uint16:1,L:uint16:1,PVConn:bitfield16:1,StorConn:bitfield16:1,ECPConn:bitfield16:1,ActWh:acc64:4,ActVAh:acc64:4,ActVArhQ1:acc64:4,ActVArhQ2:acc64:4,ActVArhQ3:acc64:4,ActVArhQ4:acc64:4,VArAval:int16:1,VArAval_SF:sunssf:1,WAval:uint16:1,WAval_SF:sunssf:1,StSetLimMsk:bitfield32:2,StActCtl:bitfield32:2,TmSrc:string:4,Tms:uint32:2,RtSt:bitfield16:1,Ris:uint16:1,Ris_SF:sunssf:1"},
		{124, 24, "ID:uint16:1,L:uint16:1,WChaMax:uint16:1,WChaGra:uint16:1,WDisChaGra:uint16:1,StorCtl_Mod:bitfield16:1,VAChaMax:uint16:1,MinRsvPct:uint16:1,ChaState:uint16:1,StorAval:uint16:1,InBatV:uint16:1,ChaSt:enum16:1,OutWRte:int16:1,InWRte:int16:1,InOutWRte_WinTms:uint16:1,InOutWRte_RvrtTms:uint16:1,InOutWRte_RmpTms:uint16:1,ChaGriSet:enum16:1,WChaMax_SF:sunssf:1,WChaDisChaGra_SF:sunssf:1,VAChaMax_SF:sunssf:1,MinRsvPct_SF:sunssf:1,ChaState_SF:sunssf:1,StorAval_SF:sunssf:1,InBatV_SF:sunssf:1,InOutWRte_SF:sunssf:1"},
	} {
		definition, ok := registry.definition(SunSpecDecoderKey{tc.id, tc.length, testSunSpecModelsRevision})
		if !ok {
			t.Fatalf("model %d absent", tc.id)
		}
		got := make([]string, len(definition.points))
		for index, point := range definition.points {
			got[index] = fmt.Sprintf("%s:%s:%d", point.name, point.pointType, point.size)
		}
		if joined := joinSunSpecCatalog(got); joined != tc.want {
			t.Fatalf("model %d catalog\n%s\nwant\n%s", tc.id, joined, tc.want)
		}
	}
}

func TestSunSpecExtendedGoldenMetadataMatchesCatalog(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"120", "121", "122", "124", "160_n4"} {
		data, err := os.ReadFile("testdata/sunspec/models/v1/model_" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var fixture struct {
			SourceCommit     string `json:"source_commit"`
			ID, Length       uint16
			PointCount       int      `json:"point_count"`
			Mandatory        []string `json:"mandatory"`
			ModuleCount      uint16   `json:"module_count"`
			ModulePointCount int      `json:"module_point_count"`
		}
		if err := json.Unmarshal(data, &fixture); err != nil {
			t.Fatal(err)
		}
		if fixture.SourceCommit != "7abdf8982d5364f8ae916deee18aac86c11be36d" {
			t.Fatalf("fixture %s source=%s", name, fixture.SourceCommit)
		}
		definition, ok := registry.definition(SunSpecDecoderKey{fixture.ID, fixture.Length, testSunSpecModelsRevision})
		if !ok || len(definition.points) != fixture.PointCount {
			t.Fatalf("fixture %s points=%d ok=%v", name, len(definition.points), ok)
		}
		var mandatory []string
		var repeated int
		for _, point := range definition.points {
			if point.mandatory {
				mandatory = append(mandatory, point.name)
			}
			if point.repeated {
				repeated++
			}
		}
		if !reflect.DeepEqual(mandatory, fixture.Mandatory) {
			t.Fatalf("fixture %s mandatory=%v", name, mandatory)
		}
		if fixture.ModuleCount > 0 && repeated != int(fixture.ModuleCount)*fixture.ModulePointCount {
			t.Fatalf("fixture %s repeated=%d", name, repeated)
		}
	}
}

func TestSunSpecExtendedModelsDecodeTypedFactsAndFailClosed(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	nameplate, err := registry.DecodeOccurrence(admittedOccurrence(120, 26, modelWords(t, registry, 120, 26, map[string][]uint16{
		"DERTyp": {1}, "WRtg": {1234}, "WRtg_SF": {0x8000}, "VARtg": {1400}, "VARtg_SF": {0},
		"VArRtgQ1": {1}, "VArRtgQ2": {1}, "VArRtgQ3": {1}, "VArRtgQ4": {1}, "VArRtg_SF": {0},
		"ARtg": {100}, "ARtg_SF": {0}, "PFRtgQ1": {1}, "PFRtgQ2": {1}, "PFRtgQ3": {1}, "PFRtgQ4": {1}, "PFRtg_SF": {0}, "Pad": {0x8000},
	}), 1))
	if err != nil {
		t.Fatal(err)
	}
	if nameplate.Qualifies() {
		t.Fatal("required value with missing scale factor qualified")
	}
	status, err := registry.DecodeOccurrence(admittedOccurrence(122, 44, modelWords(t, registry, 122, 44, map[string][]uint16{
		"PVConn": {9}, "StorConn": {1}, "ECPConn": {2}, "ActWh": {0, 0, 0, 42},
	}), 1))
	if err != nil {
		t.Fatal(err)
	}
	energy, ok := status.Fact("der.status.active_energy")
	if !ok {
		t.Fatal("active energy fact absent")
	}
	if value, ok := energy.Value.Unsigned(); !ok || value != 42 {
		t.Fatalf("active energy=%d/%v", value, ok)
	}
	connection, ok := status.Fact("der.status.pv_connection")
	if !ok {
		t.Fatal("PV connection fact absent")
	}
	if bits, unknown, ok := connection.Value.Bitfield(); !ok || bits != 9 || unknown != 0 {
		t.Fatalf("PV connection=%x unknown=%x ok=%v", bits, unknown, ok)
	}
}
