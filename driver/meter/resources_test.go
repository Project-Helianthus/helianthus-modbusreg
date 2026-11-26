package meter

import (
	"reflect"
	"testing"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

// integerMeterPoints holds the mandatory points of a three-phase integer meter.
func integerMeterPoints(extra map[string][]uint16) map[string][]uint16 {
	v := map[string][]uint16{
		"meter.ac.current.total": int16Word(30), "meter.ac.current.phase_a": int16Word(10), "meter.ac.current.phase_b": int16Word(10), "meter.ac.current.phase_c": int16Word(10), "meter.scale.current": int16Word(0),
		"meter.ac.voltage.line_neutral.average": int16Word(230), "meter.ac.voltage.line_neutral.phase_a": int16Word(230), "meter.ac.voltage.line_neutral.phase_b": int16Word(230), "meter.ac.voltage.line_neutral.phase_c": int16Word(230),
		"meter.ac.voltage.line_line.average": int16Word(400), "meter.ac.voltage.line_line.ab": int16Word(400), "meter.ac.voltage.line_line.bc": int16Word(400), "meter.ac.voltage.line_line.ca": int16Word(400), "meter.scale.voltage": int16Word(0),
		"meter.ac.frequency": int16Word(50), "meter.scale.frequency": int16Word(0),
		"meter.ac.power.active.total": int16Word(100), "meter.scale.active_power": int16Word(0),
		"meter.energy.export.total": accumulator(1), "meter.energy.import.total": accumulator(2), "meter.scale.active_energy": int16Word(0),
		"meter.events": accumulator(0),
	}
	for k, w := range extra {
		v[k] = w
	}
	return v
}

func inverterModel() []uint16 {
	b := make([]uint16, 52)
	b[0], b[1] = 103, 50
	return b
}

func meterNodes(p Publication) []string {
	var out []string
	for _, n := range p.Nodes {
		if n.Key.Kind == kindMeter {
			out = append(out, n.Key.Anchor.Slot)
		}
	}
	return out
}

func resourceInput(t *testing.T) Input {
	r := registry(t)
	integerBlock, floatBlock := meterBlock(t, r, 203, integerMeterPoints(nil)), meterBlock(t, r, 213, map[string][]uint16{
		"meter.ac.current.total": floatWords(3), "meter.ac.current.phase_a": floatWords(1), "meter.ac.current.phase_b": floatWords(1), "meter.ac.current.phase_c": floatWords(1),
		"meter.ac.voltage.line_neutral.average": floatWords(230), "meter.ac.voltage.line_neutral.phase_a": floatWords(230), "meter.ac.voltage.line_neutral.phase_b": floatWords(230), "meter.ac.voltage.line_neutral.phase_c": floatWords(230),
		"meter.ac.voltage.line_line.average": floatWords(400), "meter.ac.voltage.line_line.ab": floatWords(400), "meter.ac.voltage.line_line.bc": floatWords(400), "meter.ac.voltage.line_line.ca": floatWords(400),
		"meter.ac.frequency": floatWords(50), "meter.ac.power.active.total": floatWords(100), "meter.energy.export.total": floatWords(1), "meter.energy.import.total": floatWords(2), "meter.events": accumulator(0)})
	return Input{Template: template(), SunSpec: []SunSpecUnit{
		{Unit: 7, Snapshot: chain(t, r, 7, commonBlock(), integerBlock)},
		{Unit: 8, Snapshot: chain(t, r, 8, commonBlock(), floatBlock)},
		{Unit: 17, Snapshot: chain(t, r, 17, commonBlock(), inverterModel())},
		{Unit: 20, Snapshot: chain(t, r, 20, commonBlock(), inverterModel(), commonBlock(), integerBlock, commonBlock(), floatBlock)},
	}}
}

func TestEveryMeterOfEveryUnitAndSegmentIsOneResource(t *testing.T) {
	cfg := sunSpecConfig(
		SunSpecMeter{Unit: 7, Position: 2, Role: RoleGridConnection, GridConnection: "gc-1"},
		SunSpecMeter{Unit: 8, Position: 2, Role: RoleFeeder, GridConnection: "gc-1"},
		SunSpecMeter{Unit: 20, Position: 6, Role: RoleGridConnection, GridConnection: "gc-2"},
	)
	in := resourceInput(t)
	pub, err := testPublisher(t, cfg).Publish(in)
	if err != nil {
		t.Fatal(err)
	}
	got := meterNodes(pub)
	wantSlots := []string{"unit-20.position-4", "unit-20.position-6", "unit-7.position-2", "unit-8.position-2"}
	if len(got) != 4 || !sameSet(got, wantSlots) {
		t.Fatalf("meter resources %v, want %v (the inverter unit has none)", got, wantSlots)
	}
	site := nodeOf(t, pub, kindSite, "site-1")
	gridOne, gridTwo := nodeOf(t, pub, kindGridConnection, "gc-1"), nodeOf(t, pub, kindGridConnection, "gc-2")
	edges := map[string]bool{}
	for _, l := range pub.Links {
		switch l.Key.Type.ID {
		case "infrastructure.relation.site_grid_connection":
			if l.Key.From != site {
				t.Fatalf("site edge from %v", l.Key.From)
			}
			edges["site>"+string(l.Key.To.ID)] = true
		case "infrastructure.relation.grid_connection_meter":
			edges[string(l.Key.From.ID)+">"+string(l.Key.To.ID)] = true
		}
	}
	wantEdges := map[string]bool{
		"site>" + string(gridOne.ID): true, "site>" + string(gridTwo.ID): true,
		string(gridOne.ID) + ">" + string(nodeOf(t, pub, kindMeter, "unit-7.position-2").ID):  true,
		string(gridTwo.ID) + ">" + string(nodeOf(t, pub, kindMeter, "unit-20.position-6").ID): true,
	}
	if !reflect.DeepEqual(edges, wantEdges) {
		t.Fatalf("edges %v, want %v: a feeder or unassigned meter must not be a grid-connection meter", edges, wantEdges)
	}
	// The feeder and the unassigned meters keep their facts, without a grid-connection value.
	for _, slot := range []string{"unit-8.position-2", "unit-20.position-4"} {
		m := nodeOf(t, pub, kindMeter, slot)
		requireFact(t, pub, m, string(m.ID), want{"infrastructure.energy.import", "meter", unitJoule, "7200"})
	}
	for _, f := range pub.Facts {
		if f.Node == gridTwo && f.Candidate.Key.FactID == "infrastructure.power.pcc" {
			if f.Path != "unit-20.position-6.meter.ac.power.active.total" {
				t.Fatalf("a meter without the grid role reached the grid connection: %s", f.Path)
			}
		}
	}

	// Two lifecycles that discover the units in opposite order publish the same ids and keys.
	rev := in
	rev.SunSpec = []SunSpecUnit{in.SunSpec[3], in.SunSpec[2], in.SunSpec[1], in.SunSpec[0]}
	again, err := testPublisher(t, cfg).Publish(rev)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pub.Nodes, again.Nodes) || !reflect.DeepEqual(pub.Links, again.Links) || !reflect.DeepEqual(pub.Facts, again.Facts) || !reflect.DeepEqual(pub.Members, again.Members) {
		t.Fatal("discovery order changed a node id, a link key or a candidate")
	}
}

func sameSet(a, b []string) bool {
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

func TestUnimplementedPointsPublishNothingAndTheEventWordMapsToFault(t *testing.T) {
	r := registry(t)
	block := func(events uint32) []uint16 {
		return meterBlock(t, r, 203, integerMeterPoints(map[string][]uint16{"meter.ac.power.active.phase_a": int16Word(1), "meter.scale.active_power": int16Word(0), "meter.events": accumulator(events)}))
	}
	cfg := sunSpecConfig(SunSpecMeter{Unit: 7, Position: 2, Role: RoleGridConnection, GridConnection: "gc-1"})
	for _, c := range []struct {
		events uint32
		token  string
	}{{0, "clear"}, {1 << 2, "present"}, {1 << 6, "present"}, {1 << 16, "clear"}} {
		pub, err := testPublisher(t, cfg).Publish(Input{Template: template(), SunSpec: []SunSpecUnit{{Unit: 7, Snapshot: chain(t, r, 7, commonBlock(), block(c.events))}}})
		if err != nil {
			t.Fatal(err)
		}
		site := nodeOf(t, pub, kindSite, "site-1")
		f := pub.fact(t, site, "infrastructure.status.fault", string(site.ID))
		if f.Candidate.Value.Symbol.Token != c.token {
			t.Fatalf("event word %#x: %s, want %s", c.events, f.Candidate.Value.Symbol.Token, c.token)
		}
		m := nodeOf(t, pub, kindMeter, "unit-7.position-2")
		for _, absent := range []struct {
			el  string
			dim string
		}{{"infrastructure.ac.active_power", ".phase_b"}, {"infrastructure.ac.active_power", ".phase_c"}, {"infrastructure.ac.reactive_power", ".phase_a"}, {"infrastructure.energy.phase_import", ".phase_a"}} {
			for _, fact := range pub.Facts {
				if string(fact.Candidate.Key.FactID) == absent.el && *fact.Candidate.Key.Dimensions[0].Value.Text == string(m.ID)+absent.dim {
					t.Fatalf("an unimplemented point was published: %s%s", absent.el, absent.dim)
				}
			}
		}
		for _, mem := range pub.Members {
			if mem.Key.Local == "meter.ac.power.active.phase_b" || mem.Key.Local == "meter.ac.power.apparent.total" {
				t.Fatalf("an unimplemented point reached the namespace: %s", mem.Key.Local)
			}
		}
		for _, id := range []string{"infrastructure.power.apparent", "infrastructure.power.import_reactive", "infrastructure.ac.power_factor"} {
			if hasFact(pub, nodeOf(t, pub, kindGridConnection, "gc-1"), semreg.DefinitionID(id)) {
				t.Fatalf("an unimplemented point was published as %s", id)
			}
		}
	}
}
