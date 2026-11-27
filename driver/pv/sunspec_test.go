package pv

import (
	"testing"
)

func publish(t *testing.T, cfg Config, in Input) Publication {
	t.Helper()
	in.Template = template()
	pub, err := testPublisher(t, cfg).Publish(in)
	if err != nil {
		t.Fatal(err)
	}
	return pub
}

func TestInverterKindsPublishTheElementsOfTheirPhaseSet(t *testing.T) {
	for _, tc := range []struct {
		model   uint16
		phases  string
		float   bool
		current string
	}{
		{101, "a", false, "3"}, {102, "ab", false, "3"}, {103, "abc", false, "3"},
		{111, "a", true, "3.5"}, {112, "ab", true, "3.5"}, {113, "abc", true, "3.5"},
	} {
		var block []uint16
		if tc.float {
			block = inverterBlock(t, tc.model, floatValues(tc.model, nil))
		} else {
			block = inverterBlock(t, tc.model, integerValues(tc.model, nil))
		}
		pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, commonBlock("Maker", "Model", "SN-1"), block)}}})
		invs := pub.inverters()
		if len(invs) != 1 {
			t.Fatalf("model %d: %d inverters", tc.model, len(invs))
		}
		inv := string(invs[0].Ref.ID)
		phases := map[string]string{}
		for _, n := range pub.Nodes {
			if n.Key.Kind == kindPhase {
				phases[n.Key.Anchor.Slot] = string(n.Ref.ID)
			}
		}
		if len(phases) != len(tc.phases) {
			t.Fatalf("model %d: phase nodes %v, want %q", tc.model, phases, tc.phases)
		}
		for i, p := range tc.phases {
			slot := "L" + string(rune('1'+i))
			if _, ok := phases[slot]; !ok {
				t.Fatalf("model %d: no phase %s (%c)", tc.model, slot, p)
			}
		}
		if got := len(pub.factsOf("pv.ac.current")); got != len(tc.phases) {
			t.Fatalf("model %d: %d phase currents, want %d", tc.model, got, len(tc.phases))
		}
		if got := len(pub.factsOf("pv.ac.voltage")); got != len(tc.phases) {
			t.Fatalf("model %d: %d phase voltages, want %d", tc.model, got, len(tc.phases))
		}
		wantW, wantHz, wantA := "2500", "50.01", "10"
		wantJ, wantK := "444441600", "308.15"
		if tc.float {
			wantW, wantHz, wantA, wantJ, wantK = "2500.5", "50.25", "10.5", "444441600", "308.65"
		}
		requireQuantity(t, pub, "pv.ac.current", phases["L1"], "unit.ampere", tc.current)
		requireQuantity(t, pub, "pv.ac.aggregate_active_power", inv, "unit.watt", wantW)
		requireQuantity(t, pub, "pv.ac.frequency", inv, "unit.hertz", wantHz)
		requireQuantity(t, pub, "pv.ac.aggregate_current", inv, "unit.ampere", wantA)
		requireQuantity(t, pub, "pv.temperature.inverter", inv, "unit.kelvin", wantK)
		system := string(pub.node(t, kindSystem, "plant-1").ID)
		requireQuantity(t, pub, "pv.energy.generated", system, "unit.joule", wantJ)
		requireSymbol(t, pub, "pv.status.operating", inv, "generating")
		requireSymbol(t, pub, "pv.status.derating", inv, "clear")
		requireSymbol(t, pub, "pv.status.fault", inv, "clear")
	}
}

func TestUnitsAndAbsence(t *testing.T) {
	values := integerValues(103, map[string][]uint16{
		"inverter.scale.power_factor": word(-1), "inverter.ac.power_factor": word(950), "inverter.ac.power.apparent": word(2600), "inverter.ac.power.reactive": word(-300),
		"inverter.ac.voltage.line_ab": word(4000), "inverter.dc.current": word(7), "inverter.dc.voltage": word(6000), "inverter.dc.power": word(1900),
		"inverter.scale.dc_current": word(0), "inverter.scale.dc_voltage": word(-1), "inverter.scale.dc_power": word(0), "inverter.scale.voltage": word(-1),
	})
	pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, commonBlock("Maker", "Model", "SN-1"), inverterBlock(t, 103, values))}}})
	inv := string(pub.inverters()[0].Ref.ID)
	requireQuantity(t, pub, "pv.ac.power_factor", inv, "unit.ratio", "0.95")
	requireQuantity(t, pub, "pv.ac.aggregate_apparent_power", inv, "unit.volt_ampere", "2600")
	requireQuantity(t, pub, "pv.ac.aggregate_reactive_power", inv, "unit.volt_ampere_reactive", "-300")
	requireQuantity(t, pub, "infrastructure.ac.voltage_line_line", inv+".line_ab", "unit.volt", "400")
	requireQuantity(t, pub, "pv.dc.aggregate_current", inv, "unit.ampere", "7")
	requireQuantity(t, pub, "pv.dc.aggregate_voltage", inv, "unit.volt", "600")
	requireQuantity(t, pub, "pv.dc.aggregate_power", inv, "unit.watt", "1900")
	// a point the device does not implement is absent, never zero
	if _, ok := pub.find("infrastructure.ac.voltage_line_line", inv+".line_bc"); ok {
		t.Fatal("an unimplemented line voltage was published")
	}
	for _, f := range pub.Facts {
		if f.Candidate.Key.FactID == "pv.ac.aggregate_active_power" && f.Native.Unit != "W" {
			t.Fatalf("native unit lost: %+v", f.Native)
		}
	}
}

func TestEveryDocumentedOperatingStateIsPublished(t *testing.T) {
	for _, tc := range []struct {
		state             uint16
		operating, derate string
		fault             string
	}{
		{1, "idle", "clear", "clear"}, {2, "idle", "clear", "clear"}, {3, "idle", "clear", "clear"}, {4, "generating", "clear", "clear"},
		{5, "generating", "active", "clear"}, {6, "idle", "clear", "clear"}, {7, "", "clear", "present"}, {8, "standby", "clear", "clear"},
	} {
		for _, model := range []uint16{103, 111} {
			var block []uint16
			if model == 103 {
				block = inverterBlock(t, 103, integerValues(103, map[string][]uint16{"inverter.operating_state": {tc.state}}))
			} else {
				block = inverterBlock(t, 111, floatValues(111, map[string][]uint16{"inverter.operating_state": {tc.state}}))
			}
			pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, commonBlock("Maker", "Model", "SN-1"), block)}}})
			inv := string(pub.inverters()[0].Ref.ID)
			if tc.operating == "" {
				pub.absent(t, "pv.status.operating")
			} else {
				requireSymbol(t, pub, "pv.status.operating", inv, tc.operating)
			}
			requireSymbol(t, pub, "pv.status.derating", inv, tc.derate)
			requireSymbol(t, pub, "pv.status.fault", inv, tc.fault)
		}
	}
}

func TestEventWordsRaiseTheFaultElement(t *testing.T) {
	for _, bit := range []uint32{0, 1, 7, 8, 9, 10, 11, 15} {
		block := inverterBlock(t, 103, integerValues(103, map[string][]uint16{"inverter.events.1": accumulator(1 << bit)}))
		pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, commonBlock("Maker", "Model", "SN-1"), block)}}})
		requireSymbol(t, pub, "pv.status.fault", string(pub.inverters()[0].Ref.ID), "present")
	}
	// a bit that is not a fault condition leaves the element clear
	block := inverterBlock(t, 103, integerValues(103, map[string][]uint16{"inverter.events.1": accumulator(1 << 5)}))
	pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, commonBlock("Maker", "Model", "SN-1"), block)}}})
	requireSymbol(t, pub, "pv.status.fault", string(pub.inverters()[0].Ref.ID), "clear")
}
