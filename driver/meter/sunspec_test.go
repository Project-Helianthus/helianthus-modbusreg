package meter

import (
	"math/big"
	"testing"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/packs/infrastructure"
)

type want struct {
	element semreg.DefinitionID
	scope   string
	unit    semreg.DefinitionID
	value   string
}

func rat(t *testing.T, text string) *big.Rat {
	t.Helper()
	r, ok := new(big.Rat).SetString(text)
	if !ok {
		t.Fatalf("bad number %q", text)
	}
	return r
}

func requireFact(t *testing.T, p Publication, node driver.NodeRef, dim string, w want) Fact {
	t.Helper()
	f := p.fact(t, node, w.element, dim)
	q := f.Candidate.Value.Quantity
	if q == nil || q.Unit != w.unit {
		t.Fatalf("%s: value %+v, want unit %s", w.element, f.Candidate.Value, w.unit)
	}
	got, err := ratOf(q.Number)
	if err != nil || got.Cmp(rat(t, w.value)) != 0 {
		t.Fatalf("%s %s: got %s, want %s", w.element, dim, got.RatString(), w.value)
	}
	if err := infrastructure.New().ValidateFact(f.Candidate.Key, f.Candidate.Value); err != nil {
		t.Fatalf("%s is not a fact of the pinned pack: %v", w.element, err)
	}
	return f
}

func requireNative(t *testing.T, p Publication, slot, field, unit, value string) {
	t.Helper()
	for _, m := range p.Members {
		if string(m.Key.Node) == slot && string(m.Key.Local) == field {
			got, err := ratOf(m.Native.Value)
			if err != nil || m.Native.Unit != unit || got.Cmp(rat(t, value)) != 0 {
				t.Fatalf("native %s: %+v, want %s %s", field, m.Native, value, unit)
			}
			return
		}
	}
	t.Fatalf("native %s of %s is not in the namespace", field, slot)
}

func sunSpecConfig(roles ...SunSpecMeter) Config {
	cfg := testConfig()
	cfg.Site = Site{Slot: "site-1", GridConnections: []string{"gc-1", "gc-2"}}
	cfg.SunSpec = roles
	return cfg
}

func TestSunSpecIntegerAndFloatMetersPublishEveryElement(t *testing.T) {
	r := registry(t)
	integerBlock := meterBlock(t, r, 203, map[string][]uint16{
		"meter.ac.current.total": int16Word(330), "meter.ac.voltage.line_neutral.average": int16Word(2310), "meter.ac.voltage.line_line.average": int16Word(3990),
		"meter.ac.current.phase_a": int16Word(100), "meter.ac.current.phase_b": int16Word(110), "meter.ac.current.phase_c": int16Word(120), "meter.scale.current": int16Word(-1),
		"meter.ac.voltage.line_neutral.phase_a": int16Word(2300), "meter.ac.voltage.line_neutral.phase_b": int16Word(2310), "meter.ac.voltage.line_neutral.phase_c": int16Word(2320),
		"meter.ac.voltage.line_line.ab": int16Word(3980), "meter.ac.voltage.line_line.bc": int16Word(3990), "meter.ac.voltage.line_line.ca": int16Word(4000), "meter.scale.voltage": int16Word(-1),
		"meter.ac.frequency": int16Word(5002), "meter.scale.frequency": int16Word(-2),
		"meter.ac.power.active.total": int16Word(150), "meter.ac.power.active.phase_a": int16Word(51), "meter.ac.power.active.phase_b": int16Word(52), "meter.ac.power.active.phase_c": int16Word(47), "meter.scale.active_power": int16Word(1),
		"meter.ac.power.apparent.total": int16Word(160), "meter.ac.power.apparent.phase_a": int16Word(53), "meter.ac.power.apparent.phase_b": int16Word(54), "meter.ac.power.apparent.phase_c": int16Word(53), "meter.scale.apparent_power": int16Word(1),
		"meter.ac.power.reactive.total": int16Word(-20), "meter.ac.power.reactive.phase_a": int16Word(5), "meter.ac.power.reactive.phase_b": int16Word(-6), "meter.ac.power.reactive.phase_c": int16Word(-19), "meter.scale.reactive_power": int16Word(0),
		"meter.ac.power_factor.total": int16Word(-90), "meter.ac.power_factor.phase_a": int16Word(-95), "meter.ac.power_factor.phase_b": int16Word(80), "meter.ac.power_factor.phase_c": int16Word(-70), "meter.scale.power_factor": int16Word(0),
		"meter.energy.export.total": accumulator(1000), "meter.energy.import.total": accumulator(2000),
		"meter.energy.import.phase_a": accumulator(300), "meter.energy.import.phase_b": accumulator(400), "meter.energy.import.phase_c": accumulator(500),
		"meter.energy.export.phase_a": accumulator(10), "meter.energy.export.phase_b": accumulator(20), "meter.energy.export.phase_c": accumulator(30), "meter.scale.active_energy": int16Word(1),
		"meter.energy.apparent_import.total": accumulator(7000), "meter.energy.apparent_export.total": accumulator(3500), "meter.scale.apparent_energy": int16Word(-1),
		"meter.energy.reactive_import.q1.total": accumulator(11), "meter.energy.reactive_import.q2.total": accumulator(12), "meter.energy.reactive_export.q3.total": accumulator(13), "meter.energy.reactive_export.q4.total": accumulator(14), "meter.scale.reactive_energy": int16Word(2),
		"meter.events": accumulator(4),
	})
	floatBlock := meterBlock(t, r, 213, map[string][]uint16{
		"meter.ac.current.total": floatWords(34.5), "meter.ac.voltage.line_neutral.average": floatWords(231.5), "meter.ac.voltage.line_line.average": floatWords(400.5),
		"meter.ac.current.phase_a": floatWords(10.5), "meter.ac.current.phase_b": floatWords(11.5), "meter.ac.current.phase_c": floatWords(12.5),
		"meter.ac.voltage.line_neutral.phase_a": floatWords(230.5), "meter.ac.voltage.line_neutral.phase_b": floatWords(231.5), "meter.ac.voltage.line_neutral.phase_c": floatWords(232.5),
		"meter.ac.voltage.line_line.ab": floatWords(399.5), "meter.ac.voltage.line_line.bc": floatWords(400.5), "meter.ac.voltage.line_line.ca": floatWords(401.5),
		"meter.ac.frequency":          floatWords(50.25),
		"meter.ac.power.active.total": floatWords(-1234.5), "meter.ac.power.active.phase_a": floatWords(-400.5), "meter.ac.power.active.phase_b": floatWords(-410.5), "meter.ac.power.active.phase_c": floatWords(-423.5),
		"meter.ac.power.apparent.total": floatWords(1300.5), "meter.ac.power.apparent.phase_a": floatWords(430.5), "meter.ac.power.apparent.phase_b": floatWords(433.5), "meter.ac.power.apparent.phase_c": floatWords(436.5),
		"meter.ac.power.reactive.total": floatWords(40.5), "meter.ac.power.reactive.phase_a": floatWords(10.5), "meter.ac.power.reactive.phase_b": floatWords(-20.5), "meter.ac.power.reactive.phase_c": floatWords(50.5),
		"meter.ac.power_factor.total": floatWords(0.5), "meter.ac.power_factor.phase_a": floatWords(-0.75), "meter.ac.power_factor.phase_b": floatWords(0.25), "meter.ac.power_factor.phase_c": floatWords(-0.625),
		"meter.energy.export.total": floatWords(12000), "meter.energy.import.total": floatWords(24000),
		"meter.energy.import.phase_a": floatWords(1000), "meter.energy.import.phase_b": floatWords(2000), "meter.energy.import.phase_c": floatWords(3000),
		"meter.energy.export.phase_a": floatWords(100), "meter.energy.export.phase_b": floatWords(200), "meter.energy.export.phase_c": floatWords(300),
		"meter.energy.apparent_import.total": floatWords(500), "meter.energy.apparent_export.total": floatWords(250),
		"meter.energy.reactive_import.q1.total": floatWords(21), "meter.energy.reactive_import.q2.total": floatWords(22), "meter.energy.reactive_export.q3.total": floatWords(23), "meter.energy.reactive_export.q4.total": floatWords(24),
		"meter.events": accumulator(0),
	})
	cfg := sunSpecConfig(SunSpecMeter{Unit: 7, Position: 2, Role: RoleGridConnection, GridConnection: "gc-1"}, SunSpecMeter{Unit: 8, Position: 2, Role: RoleGridConnection, GridConnection: "gc-2"})
	p := testPublisher(t, cfg)
	pub, err := p.Publish(Input{Template: template(), SunSpec: []SunSpecUnit{
		{Unit: 7, Snapshot: chain(t, r, 7, commonBlock(), integerBlock)}, {Unit: 8, Snapshot: chain(t, r, 8, commonBlock(), floatBlock)}}})
	if err != nil {
		t.Fatal(err)
	}
	nodes := map[string]driver.NodeRef{}
	for _, n := range pub.Nodes {
		nodes[string(n.Key.Kind)+"/"+n.Key.Anchor.Slot] = n.Ref
	}
	for _, c := range []struct {
		slot, gc string
		wants    []want
		native   [][3]string
	}{
		{"unit-7.position-2", "gc-1", []want{
			{"infrastructure.ac.current", "phase_a", unitAmpere, "10"}, {"infrastructure.ac.current", "phase_b", unitAmpere, "11"}, {"infrastructure.ac.current", "phase_c", unitAmpere, "12"},
			{"infrastructure.ac.voltage", "phase_a", unitVolt, "230"}, {"infrastructure.ac.voltage", "phase_b", unitVolt, "231"}, {"infrastructure.ac.voltage", "phase_c", unitVolt, "232"},
			{"infrastructure.ac.voltage_line_line", "line_ab", unitVolt, "398"}, {"infrastructure.ac.voltage_line_line", "line_bc", unitVolt, "399"}, {"infrastructure.ac.voltage_line_line", "line_ca", unitVolt, "400"},
			{"infrastructure.ac.active_power", "phase_a", unitWatt, "510"}, {"infrastructure.ac.active_power", "phase_b", unitWatt, "520"}, {"infrastructure.ac.active_power", "phase_c", unitWatt, "470"},
			{"infrastructure.ac.apparent_power", "phase_a", unitVA, "530"}, {"infrastructure.ac.apparent_power", "phase_b", unitVA, "540"}, {"infrastructure.ac.apparent_power", "phase_c", unitVA, "530"},
			{"infrastructure.ac.reactive_power", "phase_a", unitVar, "5"}, {"infrastructure.ac.reactive_power", "phase_b", unitVar, "-6"}, {"infrastructure.ac.reactive_power", "phase_c", unitVar, "-19"},
			{"infrastructure.energy.phase_import", "phase_a", unitJoule, "10800000"}, {"infrastructure.energy.phase_import", "phase_b", unitJoule, "14400000"}, {"infrastructure.energy.phase_import", "phase_c", unitJoule, "18000000"},
			{"infrastructure.energy.phase_export", "phase_a", unitJoule, "360000"}, {"infrastructure.energy.phase_export", "phase_b", unitJoule, "720000"}, {"infrastructure.energy.phase_export", "phase_c", unitJoule, "1080000"},
			{"infrastructure.ac.phase_power_factor", "phase_a", unitRatio, "-0.95"}, {"infrastructure.ac.phase_power_factor", "phase_b", unitRatio, "0.8"}, {"infrastructure.ac.phase_power_factor", "phase_c", unitRatio, "-0.7"},
			{"infrastructure.ac.frequency", "grid", unitHertz, "50.02"}, {"infrastructure.ac.power_factor", "grid", unitRatio, "-0.9"},
			{"infrastructure.power.pcc", "grid", unitWatt, "1500"}, {"infrastructure.power.import_active", "grid", unitWatt, "1500"}, {"infrastructure.power.export_active", "grid", unitWatt, "0"},
			{"infrastructure.power.apparent", "grid", unitVA, "1600"}, {"infrastructure.power.import_reactive", "grid", unitVar, "0"}, {"infrastructure.power.export_reactive", "grid", unitVar, "20"},
			{"infrastructure.energy.export", "meter", unitJoule, "36000000"}, {"infrastructure.energy.import", "meter", unitJoule, "72000000"},
			{"infrastructure.energy.apparent_import", "meter", unitVAs, "2520000"}, {"infrastructure.energy.apparent_export", "meter", unitVAs, "1260000"},
		}, [][3]string{{"meter.ac.power_factor.phase_a", "Pct", "-95"}, {"meter.ac.power_factor.phase_b", "Pct", "80"}, {"meter.ac.power_factor.phase_c", "Pct", "-70"},
			{"meter.energy.reactive_import.q1.total", "varh", "1100"}, {"meter.energy.reactive_import.q2.total", "varh", "1200"}, {"meter.energy.reactive_export.q3.total", "varh", "1300"}, {"meter.energy.reactive_export.q4.total", "varh", "1400"}}},
		{"unit-8.position-2", "gc-2", []want{
			{"infrastructure.ac.current", "phase_a", unitAmpere, "10.5"}, {"infrastructure.ac.voltage", "phase_c", unitVolt, "232.5"}, {"infrastructure.ac.voltage_line_line", "line_ca", unitVolt, "401.5"},
			{"infrastructure.ac.active_power", "phase_c", unitWatt, "-423.5"}, {"infrastructure.ac.apparent_power", "phase_a", unitVA, "430.5"}, {"infrastructure.ac.reactive_power", "phase_b", unitVar, "-20.5"},
			{"infrastructure.energy.phase_import", "phase_a", unitJoule, "3600000"}, {"infrastructure.energy.phase_import", "phase_c", unitJoule, "10800000"},
			{"infrastructure.energy.phase_export", "phase_b", unitJoule, "720000"}, {"infrastructure.energy.phase_export", "phase_c", unitJoule, "1080000"},
			{"infrastructure.ac.phase_power_factor", "phase_a", unitRatio, "-0.75"}, {"infrastructure.ac.phase_power_factor", "phase_b", unitRatio, "0.25"}, {"infrastructure.ac.phase_power_factor", "phase_c", unitRatio, "-0.625"},
			{"infrastructure.ac.frequency", "grid", unitHertz, "50.25"}, {"infrastructure.ac.power_factor", "grid", unitRatio, "0.5"},
			{"infrastructure.power.pcc", "grid", unitWatt, "-1234.5"}, {"infrastructure.power.import_active", "grid", unitWatt, "0"}, {"infrastructure.power.export_active", "grid", unitWatt, "1234.5"},
			{"infrastructure.power.apparent", "grid", unitVA, "1300.5"}, {"infrastructure.power.import_reactive", "grid", unitVar, "40.5"}, {"infrastructure.power.export_reactive", "grid", unitVar, "0"},
			{"infrastructure.energy.export", "meter", unitJoule, "43200000"}, {"infrastructure.energy.import", "meter", unitJoule, "86400000"},
			{"infrastructure.energy.apparent_import", "meter", unitVAs, "1800000"}, {"infrastructure.energy.apparent_export", "meter", unitVAs, "900000"},
		}, [][3]string{{"meter.ac.power_factor.phase_a", "PF", "-0.75"}, {"meter.ac.power_factor.phase_b", "PF", "0.25"}, {"meter.ac.power_factor.phase_c", "PF", "-0.625"},
			{"meter.energy.reactive_import.q1.total", "varh", "21"}, {"meter.energy.reactive_import.q2.total", "varh", "22"}, {"meter.energy.reactive_export.q3.total", "varh", "23"}, {"meter.energy.reactive_export.q4.total", "varh", "24"}}},
	} {
		meter, gc := nodes["resource.meter/"+c.slot], nodes["resource.grid_connection/"+c.gc]
		for _, w := range c.wants {
			dim := string(meter.ID)
			switch {
			case w.scope == "grid":
				dim = string(gc.ID)
			case w.scope != "meter":
				dim = string(meter.ID) + "." + w.scope
			}
			node := meter
			if w.scope == "grid" {
				node = gc
			}
			requireFact(t, pub, node, dim, w)
		}
		for _, n := range c.native {
			requireNative(t, pub, c.slot, n[0], n[1], n[2])
		}
	}
	for _, f := range pub.Facts {
		if f.Candidate.Key.FactID == "infrastructure.energy.reactive_quadrant" {
			t.Fatalf("%s was published although the pack declares no usable element", f.Candidate.Key.FactID)
		}
	}
}
