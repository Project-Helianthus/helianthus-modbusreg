package pv

import (
	"testing"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/huawei"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

// dongleChild decodes a SUN2000 child of the S-Dongle profile from register words
// given by their absolute offsets.
func dongleChild(t *testing.T, unit uint8, words map[uint16][]uint16) huawei.ProviderChildObservation {
	t.Helper()
	profile := huawei.SUN2000MonitoringProfiles()[1]
	var groups []huawei.SUN2000GroupReading
	for _, step := range profile.Plan(unit) {
		g := huawei.SUN2000GroupReading{Offset: step.Offset, Quantity: step.Quantity, Outcome: huawei.SUN2000GroupAnswered, Words: make([]uint16, step.Quantity), Cycle: 1}
		for offset, ws := range words {
			if offset >= step.Offset && int(offset)+len(ws) <= int(step.Offset)+int(step.Quantity) {
				copy(g.Words[offset-step.Offset:], ws)
			}
		}
		groups = append(groups, g)
	}
	reading, err := profile.Decode(unit, groups)
	if err != nil {
		t.Fatal(err)
	}
	return huawei.ProviderChildObservation{UnitID: unit, Family: profile.Family(), ProfileID: profile.ID(), Model: "synthetic", Evidence: huawei.ProviderEvidenceCurrent, Reading: reading}
}

func pairWords(v uint32) []uint16 { return []uint16{uint16(v >> 16), uint16(v)} }

func dongleCycle(path huawei.ProviderPath, children ...huawei.ProviderChildObservation) huawei.ProviderCycle {
	return huawei.ProviderCycle{ProviderID: "synthetic", Path: path, Cycle: 1, Generation: 1, CompletedAt: time.Unix(1000, 0), Outcome: "COMPLETE", Children: children}
}

func TestADongleFamilyChildPublishesItsReadings(t *testing.T) {
	child := dongleChild(t, 1, map[uint16][]uint16{
		32016: {3500, 1250}, 32018: {0, 0}, 32020: {3400, 1100},
		32064: pairWords(4500), 32080: pairWords(4200), 32089: {0x0200},
		32106: pairWords(123456), 32112: pairWords(51), 32114: pairWords(1234), 32116: pairWords(23456), 32118: pairWords(345678),
	})
	pub := publish(t, testConfig(), Input{Huawei: []huawei.ProviderCycle{dongleCycle(huawei.ProviderPathSmartLogger, child)}})
	inv := pub.node(t, kindInverter, "instance.synthetic/1#1")
	system := pub.node(t, kindSystem, "instance.synthetic/1")
	id, sys := string(inv.ID), string(system.ID)
	requireQuantity(t, pub, "pv.dc.aggregate_power", id, "unit.watt", "4500")
	requireQuantity(t, pub, "pv.ac.aggregate_active_power", id, "unit.watt", "4200")
	requireSymbol(t, pub, "pv.status.operating", id, "generating")
	requireSymbol(t, pub, "pv.status.derating", id, "clear")
	requireSymbol(t, pub, "pv.status.fault", id, "clear")
	// kWh become joule: 1234.56 kWh = 4444416000 J
	requireQuantity(t, pub, "pv.energy.generated", sys, "unit.joule", "4444416000")
	requireQuantity(t, pub, "pv.energy.yield_day", sys, "unit.joule", "44424000")
	requireQuantity(t, pub, "pv.energy.yield_month", sys, "unit.joule", "844416000")
	requireQuantity(t, pub, "pv.energy.yield_year", sys, "unit.joule", "12444408000")
	// a string with a zero pair is native evidence, not an input
	inputs := map[string]string{}
	for _, n := range pub.Nodes {
		if n.Key.Kind == kindInput {
			inputs[n.Key.Anchor.Slot] = string(n.Ref.ID)
		}
	}
	if len(inputs) != 2 || inputs["1"] == "" || inputs["3"] == "" {
		t.Fatalf("inputs %v", inputs)
	}
	requireQuantity(t, pub, "pv.dc.voltage", inputs["1"], "unit.volt", "350")
	requireQuantity(t, pub, "pv.dc.current", inputs["1"], "unit.ampere", "12.5")
	requireQuantity(t, pub, "pv.dc.voltage", inputs["3"], "unit.volt", "340")
	for _, n := range []string{"L1", "L2", "L3"} {
		pub.node(t, kindPhase, n)
	}
	counts := map[string]int{}
	for _, l := range pub.DongleLinks {
		counts[string(l.Key.Type.ID)]++
	}
	for k, want := range map[string]int{"pv.relation.system_inverter": 1, "pv.relation.inverter_phase": 3, "pv.relation.inverter_input": 2, "relation.relays": 1} {
		if counts[k] != want {
			t.Fatalf("%s: %d, want %d (%v)", k, counts[k], want, counts)
		}
	}
	if len(pub.Links) != 0 {
		t.Fatal("a dongle-family relation was staged with the SunSpec ones")
	}
	// the reading stays in the namespace with its unit
	found := false
	for _, m := range pub.Members {
		if m.Key.Local == "huawei.sun2000.m1.input_power" && m.Point.Unit == "kW" && m.Point.Value == "4.5" {
			found = true
		}
	}
	if !found {
		t.Fatal("the typed reading is not in the namespace")
	}
}

func fact(key, unit, state string, c int64, e int16) huawei.SUN2000Fact {
	return huawei.SUN2000Fact{Key: key, Unit: unit, State: state, Availability: huawei.FactValue, Value: huawei.Decimal{Coefficient: c, Exponent: e}}
}

func childOf(t *testing.T, facts ...huawei.SUN2000Fact) Publication {
	t.Helper()
	r := &run{p: testPublisher(t, testConfig()), in: Input{Template: template()}, nodes: map[string]Node{}, linked: map[string]bool{}}
	if err := r.childFacts(dongleCycle(huawei.ProviderPathSDongle), 1, facts, nil); err != nil {
		t.Fatal(err)
	}
	r.sort()
	return r.out
}

func TestTheRowsOfTheTypedReadings(t *testing.T) {
	const m = "huawei.sun2000.mc."
	pub := childOf(t,
		fact(m+"active_power", "kW", "", -1500, -3), fact(m+"reactive_power", "kvar", "", 250, -3), fact(m+"power_factor", "", "", 987, -3),
		fact(m+"frequency", "Hz", "", 5002, -2), fact(m+"internal_temperature", "℃", "", 355, -1),
		fact(m+"phase_voltage_a", "V", "", 2301, -1), fact(m+"phase_voltage_b", "V", "", 2302, -1), fact(m+"phase_voltage_c", "V", "", 2303, -1),
		fact(m+"phase_current_a", "A", "", 1234, -3), fact(m+"phase_current_c", "A", "", 5, -3),
		fact(m+"line_voltage_ab", "V", "", 4001, -1), fact(m+"line_voltage_bc", "V", "", 4002, -1), fact(m+"line_voltage_ca", "V", "", 4003, -1),
		fact(m+"energy_dc_input", "kWh", "", 98765, -2),
	)
	inv := string(pub.node(t, kindInverter, "instance.synthetic/1#1").ID)
	phase := func(slot string) string { return string(pub.node(t, kindPhase, slot).ID) }
	// the sign of the active power is kept
	requireQuantity(t, pub, "pv.ac.aggregate_active_power", inv, "unit.watt", "-1500")
	requireQuantity(t, pub, "pv.ac.aggregate_reactive_power", inv, "unit.volt_ampere_reactive", "250")
	requireQuantity(t, pub, "pv.ac.power_factor", inv, "unit.ratio", "0.987")
	requireQuantity(t, pub, "pv.ac.frequency", inv, "unit.hertz", "50.02")
	requireQuantity(t, pub, "pv.temperature.inverter", inv, "unit.kelvin", "308.65")
	requireQuantity(t, pub, "pv.ac.voltage", phase("L1"), "unit.volt", "230.1")
	requireQuantity(t, pub, "pv.ac.voltage", phase("L2"), "unit.volt", "230.2")
	requireQuantity(t, pub, "pv.ac.voltage", phase("L3"), "unit.volt", "230.3")
	requireQuantity(t, pub, "pv.ac.current", phase("L1"), "unit.ampere", "1.234")
	requireQuantity(t, pub, "pv.ac.current", phase("L3"), "unit.ampere", "0.005")
	requireQuantity(t, pub, "infrastructure.ac.voltage_line_line", inv+".line_ab", "unit.volt", "400.1")
	requireQuantity(t, pub, "infrastructure.ac.voltage_line_line", inv+".line_ca", "unit.volt", "400.3")
	// phase B current was not read: nothing is published for it
	if _, ok := pub.find("pv.ac.current", phase("L2")); ok {
		t.Fatal("a missing reading was published")
	}
	// the DC-input energy counter has no element in the pinned pack: it is
	// retained natively, listed, and never moved onto another element
	pub.absent(t, "pv.dc.aggregate_energy")
	retained, listed := false, false
	for _, mem := range pub.Members {
		retained = retained || (mem.Point.Field == m+"energy_dc_input" && mem.Point.Unit == "kWh" && mem.Point.Value == "987.65")
	}
	for _, w := range pub.Withheld {
		listed = listed || w.Reason == reasonNoDCInputEnergy
	}
	if !retained || !listed {
		t.Fatalf("counter retained=%v listed=%v", retained, listed)
	}
}

func TestEachStateOfTheDictionaryHasItsRow(t *testing.T) {
	for _, tc := range []struct {
		state, operating, derating, fault string
	}{
		{"grid_connected", "generating", "clear", "clear"}, {"running_power_limited", "generating", "active", "clear"},
		{"standby_grid_check", "standby", "clear", "clear"}, {"starting", "idle", "clear", "clear"}, {"command_shutdown", "idle", "clear", "clear"},
		{"fault_shutdown", "idle", "clear", "present"}, {"power_limit_shutdown", "idle", "active", "clear"},
	} {
		pub := childOf(t, fact("huawei.sun2000.mc.status", "", tc.state, 512, 0))
		inv := string(pub.node(t, kindInverter, "instance.synthetic/1#1").ID)
		requireSymbol(t, pub, "pv.status.operating", inv, tc.operating)
		requireSymbol(t, pub, "pv.status.derating", inv, tc.derating)
		requireSymbol(t, pub, "pv.status.fault", inv, tc.fault)
	}
}

func TestAStateOutsideTheTableIsPublishedRawWithItsReason(t *testing.T) {
	pub := childOf(t, fact("huawei.sun2000.mc.status", "", "iv_scan", 2048, 0))
	pub.absent(t, "pv.status.operating")
	listed := false
	for _, w := range pub.Withheld {
		listed = listed || w.Reason == reasonStateNotTable+":iv_scan"
	}
	raw := false
	for _, m := range pub.Members {
		raw = raw || (m.Point.Field == "huawei.sun2000.mc.status" && m.Point.State == "iv_scan" && m.Point.Value == "2048")
	}
	if !listed || !raw {
		t.Fatalf("listed=%v raw=%v", listed, raw)
	}
}

func TestPlantEnergyAndYieldOnThePVSystem(t *testing.T) {
	logger := dongleCycle(huawei.ProviderPathSmartLogger)
	logger.Plant = []huawei.ProviderPlantGroup{
		{UnitID: 0, Facts: []huawei.ProviderPlantFact{
			{Key: "plant_dc_input_power", Offset: 40521, Unit: "kW", Availability: huawei.FactValue, Value: huawei.Decimal{Coefficient: 8000, Exponent: -3}},
			{Key: "plant_ac_power", Offset: 40525, Unit: "kW", Availability: huawei.FactValue, Value: huawei.Decimal{Coefficient: 7500, Exponent: -3}},
		}},
		{UnitID: 0, Facts: []huawei.ProviderPlantFact{
			{Key: "plant_total_energy", Offset: 40560, Unit: "kWh", Availability: huawei.FactValue, Value: huawei.Decimal{Coefficient: 12345, Exponent: -1}},
			{Key: "plant_daily_energy", Offset: 40562, Unit: "kWh", Availability: huawei.FactValue, Value: huawei.Decimal{Coefficient: 205, Exponent: -1}},
		}},
	}
	emma := dongleCycle(huawei.ProviderPathEMMA)
	emma.EMMASite = []huawei.EMMASiteGroup{{Facts: []huawei.EMMASiteFact{
		{Key: "gateway-self.30346", Offset: 30346, Type: "U32", Unit: "kWh", Exponent: -2, Unsigned: 1500, Availability: huawei.FactValue},
		{Key: "gateway-self.30348", Offset: 30348, Type: "U64", Unit: "kWh", Exponent: -2, Unsigned: 250000, Availability: huawei.FactValue},
		{Key: "gateway-self.30354", Offset: 30354, Type: "U32", Unit: "kW", Exponent: -3, Unsigned: 6000, Availability: huawei.FactValue},
		{Key: "gateway-self.30364", Offset: 30364, Type: "I32", Unit: "kW", Exponent: -3, Signed: 5900, Availability: huawei.FactValue},
	}}}
	dongle := dongleCycle(huawei.ProviderPathSDongle)
	dongle.Plant = []huawei.ProviderPlantGroup{{UnitID: 100, Facts: []huawei.ProviderPlantFact{
		{Key: "plant_dc_input_power", Offset: 37498, Unit: "kW", Availability: huawei.FactValue, Value: huawei.Decimal{Coefficient: 5000, Exponent: -3}},
		{Key: "plant_ac_power", Offset: 37516, Unit: "kW", Availability: huawei.FactValue, Value: huawei.Decimal{Coefficient: 4800, Exponent: -3}},
	}}}

	pub := publish(t, testConfig(), Input{Huawei: []huawei.ProviderCycle{logger, emma}})
	system := string(pub.node(t, kindSystem, "plant-1").ID)
	// logger: 1234.5 kWh and 20.5 kWh; EMMA: 2500 kWh and 15 kWh, on the one PV system
	var generated, day []string
	for _, f := range pub.factsOf("pv.energy.generated") {
		generated = append(generated, f.Candidate.Value.Quantity.Number.Coefficient+"e"+string(rune('0'+f.Candidate.Value.Quantity.Number.Exponent10%10)))
	}
	if len(pub.factsOf("pv.energy.generated")) != 2 || len(pub.factsOf("pv.energy.yield_day")) != 2 {
		t.Fatalf("generated %v day %v", generated, day)
	}
	for _, f := range append(pub.factsOf("pv.energy.generated"), pub.factsOf("pv.energy.yield_day")...) {
		if *f.Candidate.Key.Dimensions[0].Value.Text != system || f.Candidate.Key.Dimensions[0].ID != "pv.dimension.system" {
			t.Fatalf("plant energy on %v", f.Candidate.Key.Dimensions)
		}
	}
	// plant AC and DC power: no element in the pinned pack, so never an inverter's element
	pub.absent(t, "pv.ac.aggregate_active_power")
	pub.absent(t, "pv.dc.aggregate_power")
	gaps := map[string]int{}
	for _, w := range pub.Withheld {
		gaps[w.Reason]++
	}
	if gaps[reasonNoPlantACPower] != 2 || gaps[reasonNoPlantDCPower] != 2 {
		t.Fatalf("gaps %v", gaps)
	}
	// the S-Dongle path supplies neither plant energy nor daily yield: nothing is declared
	pub = publish(t, testConfig(), Input{Huawei: []huawei.ProviderCycle{dongle}})
	pub.absent(t, "pv.energy.generated")
	pub.absent(t, "pv.energy.yield_day")
	listed := false
	for _, w := range pub.Withheld {
		listed = listed || w.Reason == "path_supplies_no_plant_total_energy_or_daily_yield"
	}
	if !listed {
		t.Fatal("the gap is not listed")
	}
}

func TestCompanionsAndTheUnknownRelaysRelation(t *testing.T) {
	child := dongleChild(t, 1, map[uint16][]uint16{32089: {0x0200}})
	battery := driver.NodeRef{Bridge: "bridge.synthetic", ID: "resource:battery.synthetic"}
	with := publish(t, testConfig(), Input{Huawei: []huawei.ProviderCycle{dongleCycle(huawei.ProviderPathSDongle, child)}, Companions: []Companion{{Unit: 1, Node: battery}}})
	inv := with.node(t, kindInverter, "instance.synthetic/1#1")
	found := false
	for _, l := range with.DongleLinks {
		found = found || (l.Key.Type.ID == "relation.relays" && l.Key.From == inv && l.Key.To == battery && l.Basis[0].Kind == "evidence.native_observation")
	}
	if !found || len(with.UnknownLinks) != 0 {
		t.Fatalf("relays to a current battery: found=%v unknown=%v", found, with.UnknownLinks)
	}
	without := publish(t, testConfig(), Input{Huawei: []huawei.ProviderCycle{dongleCycle(huawei.ProviderPathSDongle, child)}, Companions: []Companion{{Unit: 9, Node: battery}}})
	for _, l := range without.DongleLinks {
		if l.Key.Type.ID == "relation.relays" {
			t.Fatal("a relays link was claimed with no current battery or meter for the unit")
		}
	}
	if len(without.UnknownLinks) != 1 || without.UnknownLinks[0].Reason != reasonNoCompanion {
		t.Fatalf("unknown links %+v", without.UnknownLinks)
	}
}

func TestTheLoggerRelaysUseThePlannedSlot(t *testing.T) {
	child := dongleChild(t, 1, map[uint16][]uint16{32089: {0x0200}})
	pub := publish(t, testConfig(), Input{Huawei: []huawei.ProviderCycle{dongleCycle(huawei.ProviderPathSmartLogger, child)}})
	logger := pub.node(t, kindEquipment, "logger-slot-1")
	inv := pub.node(t, kindInverter, "instance.synthetic/1#1")
	var relays int
	for _, l := range pub.DongleLinks {
		if l.Key.Type.ID == "relation.relays" && l.Key.From == logger && l.Key.To == inv {
			relays++
		}
	}
	if relays != 1 {
		t.Fatalf("%d logger relays", relays)
	}
	other := testConfig()
	other.LoggerSlot = "logger-slot-2"
	again := publish(t, other, Input{Huawei: []huawei.ProviderCycle{dongleCycle(huawei.ProviderPathSmartLogger, child)}})
	if again.node(t, kindEquipment, "logger-slot-2").ID == logger.ID {
		t.Fatal("a different planned slot gave the same logger equipment")
	}
}
