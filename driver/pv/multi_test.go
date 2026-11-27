package pv

import (
	"fmt"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
)

func derBlock(t *testing.T, model, length uint16, values map[string][]uint16) []uint16 {
	t.Helper()
	r, err := sunspec.NewStandardSunSpecDecoderRegistry(sunspec.ModelsRevisionV2)
	if err != nil {
		t.Fatal(err)
	}
	key := sunspec.DecoderKey{ModelID: model, ModelLength: length, SchemaRevision: sunspec.ModelsRevisionV2}
	words := make([]uint16, int(length)+2)
	words[0], words[1] = model, length
	for name, v := range values {
		p, _, ok := r.FixedPointLayout(key, fmt.Sprintf("sunspec.der.v2.%d.%s", model, name))
		if !ok || int(p.WordWidth) != len(v) {
			t.Fatalf("model %d point %s: layout %v ok=%v words=%d", model, name, p, ok, len(v))
		}
		copy(words[p.WordOffset:], v)
	}
	return words
}

func TestTwoCommonModelsAreTwoInvertersUnderOneSystem(t *testing.T) {
	a := inverterBlock(t, 103, integerValues(103, map[string][]uint16{"inverter.ac.power.active": word(1000)}))
	b := inverterBlock(t, 103, integerValues(103, map[string][]uint16{"inverter.ac.power.active": word(2000)}))
	pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1,
		commonBlock("Maker", "Model", "SN-A"), a, commonBlock("Maker", "Model", "SN-B"), b)}}})
	invs := pub.inverters()
	if len(invs) != 2 {
		t.Fatalf("%d inverters", len(invs))
	}
	got := map[string]bool{}
	for _, f := range pub.factsOf("pv.ac.aggregate_active_power") {
		got[f.Candidate.Value.Quantity.Number.Coefficient+"e"+fmt.Sprint(f.Candidate.Value.Quantity.Number.Exponent10)] = true
	}
	if !got["1e3"] || !got["2e3"] {
		t.Fatalf("each inverter keeps its own power: %v", got)
	}
	system := pub.node(t, kindSystem, "plant-1")
	for _, n := range invs {
		found := false
		for _, l := range pub.Links {
			if l.Key.Type.ID == "pv.relation.system_inverter" && l.Key.From == system && l.Key.To == n.Ref {
				found = true
			}
		}
		if !found {
			t.Fatalf("inverter %s is not grouped under the configured system", n.Ref.ID)
		}
	}
	if len(pub.Claims) != 2 {
		t.Fatalf("%d claims, want one per device", len(pub.Claims))
	}
}

func TestOneCommonModelWithManyBlocksIsOneDevice(t *testing.T) {
	pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1,
		commonBlock("Maker", "Model", "SN-A"), inverterBlock(t, 103, integerValues(103, nil)), inverterBlock(t, 103, integerValues(103, nil)))}}})
	if got := len(pub.inverters()); got != 1 {
		t.Fatalf("%d inverters for one Common model", got)
	}
}

func TestAnUnattributableBlockCarriesItsReasonWhileTheOthersPublish(t *testing.T) {
	stray := inverterBlock(t, 103, integerValues(103, nil))
	pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1,
		stray, commonBlock("Maker", "Model", "SN-A"), inverterBlock(t, 103, integerValues(103, nil)))}}})
	if len(pub.inverters()) != 1 {
		t.Fatalf("%d inverters, want the attributable one", len(pub.inverters()))
	}
	var noGo, goDecision int
	for _, d := range pub.Decisions {
		switch {
		case !d.Go && d.Common == 0 && d.Reason == "no_common_model_for_block":
			noGo++
		case d.Go:
			goDecision++
		}
	}
	if noGo != 1 || goDecision != 1 {
		t.Fatalf("decisions %+v", pub.Decisions)
	}
}

func TestADERChainPublishesTheModel701AndPVPortValues(t *testing.T) {
	ac := derBlock(t, 701, 153, map[string][]uint16{
		"ACType": {2}, "W": word(-30), "W_SF": word(0), "VA": word(50), "VA_SF": word(0), "Var": word(7), "Var_SF": word(0),
		"WL1": word(11), "WL2": word(12), "WL3": word(13), "VAL1": word(21), "VAL2": word(22), "VAL3": word(23), "VarL1": word(31), "VarL2": word(32), "VarL3": word(33),
		"VL1": word(230), "V_SF": word(0), "AL1": word(4), "A_SF": word(0), "Hz": {0, 50}, "Hz_SF": word(0), "PF": word(95), "PF_SF": word(0),
	})
	port := make([]uint16, 70)
	port[0], port[1], port[4], port[5] = 714, 68, 2, 2
	port[20], port[45] = 0, 1 // port 1 is a PV port, port 2 a storage port
	port[21], port[46] = 21, 22
	port[30], port[55] = 6, 7
	port[31], port[56] = 15, 25
	battery := derBlock(t, 713, 7, map[string][]uint16{"WHRtg": {400}, "SoC": {65}})
	storage := derBlock(t, 802, 62, map[string][]uint16{"AHRtg": {80}, "WHRtg": {350}, "SoC": {75}})
	pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1,
		derCommonBlock("Maker", "Model", "SN-D"), ac, battery, port, storage)}}})
	inv := string(pub.inverters()[0].Ref.ID)
	phases := map[string]string{}
	for _, n := range pub.Nodes {
		if n.Key.Kind == kindPhase {
			phases[n.Key.Anchor.Slot] = string(n.Ref.ID)
		}
	}
	requireQuantity(t, pub, "pv.ac.aggregate_active_power", inv, "unit.watt", "-30")
	requireQuantity(t, pub, "pv.ac.aggregate_apparent_power", inv, "unit.volt_ampere", "50")
	requireQuantity(t, pub, "pv.ac.aggregate_reactive_power", inv, "unit.volt_ampere_reactive", "7")
	for i, slot := range []string{"L1", "L2", "L3"} {
		requireQuantity(t, pub, "pv.ac.active_power", phases[slot], "unit.watt", fmt.Sprint(11+i))
		requireQuantity(t, pub, "pv.ac.apparent_power", phases[slot], "unit.volt_ampere", fmt.Sprint(21+i))
		requireQuantity(t, pub, "pv.ac.reactive_power", phases[slot], "unit.volt_ampere_reactive", fmt.Sprint(31+i))
	}
	requireQuantity(t, pub, "pv.ac.voltage", phases["L1"], "unit.volt", "230")
	requireQuantity(t, pub, "pv.ac.current", phases["L1"], "unit.ampere", "4")
	requireQuantity(t, pub, "pv.ac.frequency", inv, "unit.hertz", "50")
	requireQuantity(t, pub, "pv.ac.power_factor", inv, "unit.ratio", "0.95")
	inputs := 0
	for _, n := range pub.Nodes {
		if n.Key.Kind == kindInput {
			inputs++
			requireQuantity(t, pub, "pv.dc.current", string(n.Ref.ID), "unit.ampere", "6")
			requireQuantity(t, pub, "pv.dc.voltage", string(n.Ref.ID), "unit.volt", "15")
		}
	}
	if inputs != 1 {
		t.Fatalf("%d inputs, want the PV port only", inputs)
	}
}
