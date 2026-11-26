package meter

import (
	"reflect"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/huawei"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

func dongleConfig(unit uint8) Config {
	cfg := testConfig()
	cfg.Site = Site{Slot: "site-1", GridConnections: []string{"gc-1", "gc-2"}}
	cfg.Huawei = []HuaweiMeter{{Path: PathSDongle, Slot: "dongle-meter", Unit: unit, Role: RoleGridConnection, GridConnection: "gc-1"}}
	cfg.GridPoints = []HuaweiGridPoint{{Path: PathSDongle, Slot: "dongle-site", GridConnection: "gc-2"}}
	return cfg
}

func publishHuawei(t *testing.T, cfg Config, cycles ...HuaweiCycle) Publication {
	t.Helper()
	out, err := testPublisher(t, cfg).Publish(Input{Template: template(), Huawei: cycles})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// sdongleCycle runs one cycle of the real S-Dongle provider with the plant
// consumption and exchange and the child meter power set, and checks that the
// provider placed the meter group at the routed child unit.
func sdongleCycle(t *testing.T, g *deviceRig, meterW, plantW int32) huawei.ProviderCycle {
	t.Helper()
	g.plant(1500, plantW)
	g.meter(meterW, 980)
	c := g.cycle()
	var groups int
	for _, m := range c.Meter {
		if m.UnitID != g.unit {
			t.Fatalf("meter group at unit %d, want the routed child unit %d", m.UnitID, g.unit)
		}
		groups++
	}
	if groups != 4 || len(c.Children) != 1 || c.Children[0].UnitID != g.unit {
		t.Fatalf("provider cycle: %d meter groups, children %+v", groups, c.Children)
	}
	return c
}

func TestSDongleChildMeterPolarityAgainstPlantExchange(t *testing.T) {
	for _, unit := range []uint8{1, 7} {
		g := newSDongleRig(t, unit)
		plusCycle := sdongleCycle(t, g, 500, 500)
		plus := publishHuawei(t, dongleConfig(unit), HuaweiCycle{Path: PathSDongle, Cycle: plusCycle})
		gc, plant := nodeOf(t, plus, kindGridConnection, "gc-1"), nodeOf(t, plus, kindGridConnection, "gc-2")
		meter, load := nodeOf(t, plus, kindMeter, "dongle-meter"), nodeOf(t, plus, kindLoad, "dongle-site")
		// The meter states positive as fed to the grid; the plant register states positive as imported.
		requireFact(t, plus, gc, string(gc.ID), want{"infrastructure.power.export_active", "grid", unitWatt, "500"})
		requireFact(t, plus, gc, string(gc.ID), want{"infrastructure.power.import_active", "grid", unitWatt, "0"})
		requireFact(t, plus, gc, string(gc.ID), want{"infrastructure.power.pcc", "grid", unitWatt, "500"})
		requireFact(t, plus, plant, string(plant.ID), want{"infrastructure.power.import_active", "grid", unitWatt, "500"})
		requireFact(t, plus, plant, string(plant.ID), want{"infrastructure.power.export_active", "grid", unitWatt, "0"})
		requireFact(t, plus, plant, string(plant.ID), want{"infrastructure.power.pcc", "grid", unitWatt, "500"})
		requireFact(t, plus, load, string(load.ID), want{"infrastructure.load.active_power", "load", unitWatt, "1500"})
		requireFact(t, plus, gc, string(gc.ID), want{"infrastructure.ac.power_factor", "grid", unitRatio, "0.98"})
		requireFact(t, plus, meter, string(meter.ID), want{"infrastructure.energy.export", "meter", unitJoule, "44424000"})
		requireFact(t, plus, meter, string(meter.ID), want{"infrastructure.energy.import", "meter", unitJoule, "20412000"})
		requireFact(t, plus, meter, string(meter.ID)+".phase_b", want{"infrastructure.ac.voltage", "phase", unitVolt, "231"})
		requireFact(t, plus, meter, string(meter.ID)+".phase_c", want{"infrastructure.ac.current", "phase", unitAmpere, "4.3"})
		requireFact(t, plus, meter, string(meter.ID)+".phase_a", want{"infrastructure.ac.active_power", "phase", unitWatt, "200"})

		minus := publishHuawei(t, dongleConfig(unit), HuaweiCycle{Path: PathSDongle, Cycle: sdongleCycle(t, g, -500, -500)})
		requireFact(t, minus, gc, string(gc.ID), want{"infrastructure.power.import_active", "grid", unitWatt, "500"})
		requireFact(t, minus, gc, string(gc.ID), want{"infrastructure.power.export_active", "grid", unitWatt, "0"})
		requireFact(t, minus, gc, string(gc.ID), want{"infrastructure.power.pcc", "grid", unitWatt, "-500"})
		requireFact(t, minus, plant, string(plant.ID), want{"infrastructure.power.export_active", "grid", unitWatt, "500"})

		// A meter inventory entry for the same unit, added to the provider's own cycle, changes nothing that is modelled.
		listedCycle := plusCycle
		listedCycle.Children = append([]huawei.ProviderChildObservation(nil), plusCycle.Children...)
		listedCycle.Children[0].ListedClass = "PowerMeter"
		listed := publishHuawei(t, dongleConfig(unit), HuaweiCycle{Path: PathSDongle, Cycle: listedCycle})
		if !reflect.DeepEqual(plus.Facts, listed.Facts) || !reflect.DeepEqual(plus.Nodes, listed.Nodes) || !reflect.DeepEqual(plus.Links, listed.Links) {
			t.Fatalf("unit %d: a listing entry changed the publication", unit)
		}
	}
}

func TestHuaweiMeterKeepsItsResourceWhenTheRoutedUnitChanges(t *testing.T) {
	at := func(unit uint8) Publication {
		return publishHuawei(t, dongleConfig(unit), HuaweiCycle{Path: PathSDongle, Cycle: sdongleCycle(t, newSDongleRig(t, unit), 500, 500)})
	}
	a, b := at(7), at(9)
	if !reflect.DeepEqual(a.Nodes, b.Nodes) || !reflect.DeepEqual(a.Links, b.Links) {
		t.Fatalf("routed unit changed the resource or edge keys:\n%+v\n%+v", a.Nodes, b.Nodes)
	}
	meter := nodeOf(t, a, kindMeter, "dongle-meter")
	var toMeter int
	for _, l := range a.Links {
		if l.Key.Type.ID == "infrastructure.relation.grid_connection_meter" && l.Key.To == meter {
			toMeter++
		}
	}
	if len(a.Links) != 3 || toMeter != 1 {
		t.Fatalf("links %+v", a.Links)
	}
	for _, n := range a.Nodes {
		if n.Key.Kind == kindMeter && (n.Key.Anchor.Slot != "dongle-meter" || n.Key.Anchor.Key != "" || n.Key.Anchor.Scheme != "") {
			t.Fatalf("meter anchored by something other than the configured slot: %+v", n.Key.Anchor)
		}
	}
}

// loggerConfig declares the grid-role meter at routed unit 11 and a second meter
// of the given role at routed unit 19, the two meters the logger listing carries.
func loggerConfig(other Role) Config {
	cfg := testConfig()
	cfg.Site = Site{Slot: "site-1", GridConnections: []string{"gc-1"}}
	cfg.Huawei = []HuaweiMeter{
		{Path: PathLogger, Slot: "grid-meter", Unit: 11, Role: RoleGridConnection, GridConnection: "gc-1"},
		{Path: PathLogger, Slot: "other-meter", Unit: 19, Role: other, GridConnection: "gc-1"},
	}
	return cfg
}

func TestFeederAndUnassignedMetersContributeNothingToTheGridConnection(t *testing.T) {
	for _, tc := range []struct {
		name, pf, power, kw string
		role                Role
		pfRaw               uint16
		powerW              int32
	}{
		{"feeder-meter", "0.72", "600", "0.6", RoleFeeder, 720, 600},
		{"plain-meter", "0.65", "100", "0.1", "", 650, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newLoggerRig(t, loggerSignedBuild)
			g.loggerMeter(11, 750, 980)
			g.loggerMeter(19, tc.powerW, tc.pfRaw)
			c := g.cycle()
			if got := loggerPolarity(t, c, 11); got != huawei.MeterPolarityExportPositive {
				t.Fatalf("the provider reported polarity %q for the stated edition", got)
			}
			cfg := loggerConfig(tc.role)
			cfg.Huawei[1].Slot = tc.name
			pub := publishHuawei(t, cfg, HuaweiCycle{Path: PathLogger, Cycle: c})
			gc := nodeOf(t, pub, kindGridConnection, "gc-1")
			site := nodeOf(t, pub, kindSite, "site-1")
			// Only the grid-role meter reaches the grid connection: 0.98, never the other meter's value.
			requireFact(t, pub, gc, string(gc.ID), want{"infrastructure.ac.power_factor", "grid", unitRatio, "0.98"})
			requireFact(t, pub, gc, string(gc.ID), want{"infrastructure.power.pcc", "grid", unitWatt, "750"})
			requireFact(t, pub, gc, string(gc.ID), want{"infrastructure.power.export_active", "grid", unitWatt, "750"})
			count := map[semreg.DefinitionID]int{}
			for _, f := range pub.Facts {
				if f.Node == gc {
					count[f.Candidate.Key.FactID]++
				}
			}
			if count["infrastructure.ac.power_factor"] != 1 || count["infrastructure.power.pcc"] != 1 || count["infrastructure.power.import_active"] != 1 {
				t.Fatalf("grid connection facts: %v", count)
			}
			m := nodeOf(t, pub, kindMeter, tc.name)
			requireFact(t, pub, m, string(m.ID)+".phase_a", want{"infrastructure.ac.voltage", "phase", unitVolt, "230.01"})
			requireFact(t, pub, m, string(m.ID)+".phase_a", want{"infrastructure.ac.active_power", "phase", unitWatt, "250"})
			requireNative(t, pub, tc.name, "power_factor", "", tc.pf)
			requireNative(t, pub, tc.name, "active_power", "kW", tc.kw)
			requireNative(t, pub, tc.name, "positive_active_energy", "kWh", "800")
			// Edges: the declared site edge stays, and only the grid-role meter has a grid-connection edge.
			var siteEdges, meterEdges []driver.LinkKey
			for _, l := range pub.Links {
				switch l.Key.Type.ID {
				case "infrastructure.relation.site_grid_connection":
					siteEdges = append(siteEdges, l.Key)
				case "infrastructure.relation.grid_connection_meter":
					meterEdges = append(meterEdges, l.Key)
				}
			}
			if len(siteEdges) != 1 || siteEdges[0].From != site || siteEdges[0].To != gc || len(meterEdges) != 1 || meterEdges[0].To != nodeOf(t, pub, kindMeter, "grid-meter") {
				t.Fatalf("edges: %+v %+v", siteEdges, meterEdges)
			}
		})
	}
}

// loggerPolarity is the polarity the provider reported for the active power of a meter unit.
func loggerPolarity(t *testing.T, c huawei.ProviderCycle, unit uint8) huawei.MeterPolarity {
	t.Helper()
	for _, g := range c.Meter {
		for _, f := range g.Facts {
			if g.UnitID == unit && f.Key == "active_power" {
				return f.Polarity
			}
		}
	}
	t.Fatalf("no active power fact of unit %d in the provider cycle", unit)
	return ""
}

func TestLoggerEditionSignDecidesTheSplit(t *testing.T) {
	cfg := loggerConfig("")
	cfg.Huawei = cfg.Huawei[:1]
	stated := newLoggerRig(t, loggerSignedBuild)
	stated.loggerMeter(11, -750, 980)
	statedCycle := stated.cycle()
	if got := loggerPolarity(t, statedCycle, 11); got != huawei.MeterPolarityExportPositive {
		t.Fatalf("the provider reported polarity %q for the stated edition", got)
	}
	pub := publishHuawei(t, cfg, HuaweiCycle{Path: PathLogger, Cycle: statedCycle})
	gc := nodeOf(t, pub, kindGridConnection, "gc-1")
	requireFact(t, pub, gc, string(gc.ID), want{"infrastructure.power.import_active", "grid", unitWatt, "750"})

	// The observed edition states no sign, so the provider reports none.
	unstated := newLoggerRig(t, loggerObservedBuild)
	unstated.loggerMeter(11, -750, 980)
	openCycle := unstated.cycle()
	if got := loggerPolarity(t, openCycle, 11); got != huawei.MeterPolarityUnknown {
		t.Fatalf("the provider reported polarity %q for an edition that states none", got)
	}
	open := publishHuawei(t, cfg, HuaweiCycle{Path: PathLogger, Cycle: openCycle})
	pcc := requireFact(t, open, gc, string(gc.ID), want{"infrastructure.power.pcc", "grid", unitWatt, "-750"})
	if pcc.Native.Open != openSign || hasFact(open, gc, "infrastructure.power.import_active") || hasFact(open, gc, "infrastructure.power.export_active") {
		t.Fatalf("an open sign was split or not named: %+v", pcc.Native)
	}
	// Counters of the logger state no direction.
	m := nodeOf(t, open, kindMeter, "grid-meter")
	if hasFact(open, m, "infrastructure.energy.import") || hasFact(open, m, "infrastructure.energy.export") {
		t.Fatal("logger counters were published as import or export")
	}
	requireNative(t, open, "grid-meter", "negative_active_energy", "kWh", "200")
}

func TestLoggerListingWithOnlyAClassLabelPublishesNoPhysicalIdentity(t *testing.T) {
	// The listing carries the class label and routing of the meter; the
	// groups are not answered, so no value and no identity is published.
	g := newLoggerRig(t, loggerObservedBuild)
	c := g.cycle()
	var listed bool
	for _, child := range c.Children {
		listed = listed || (child.UnitID == 11 && child.ListedClass == "PowerMeter")
	}
	if !listed {
		t.Fatalf("the provider listing lacks the meter class label: %+v", c.Children)
	}
	cfg := testConfig()
	cfg.Huawei = []HuaweiMeter{{Path: PathLogger, Slot: "listed-meter", Unit: 11}}
	pub := publishHuawei(t, cfg, HuaweiCycle{Path: PathLogger, Cycle: c})
	if len(pub.Nodes) != 1 || pub.Nodes[0].Key.Kind != kindMeter || pub.Nodes[0].Key.Anchor != (driver.AnchorKey{Slot: "listed-meter"}) {
		t.Fatalf("nodes %+v", pub.Nodes)
	}
	if len(pub.Facts) != 0 || len(pub.Members) != 0 || len(pub.Links) != 0 {
		t.Fatalf("a class label published values or an identity: %+v", pub)
	}
}

func TestRetainedReadingKeepsItsValueAsDegraded(t *testing.T) {
	g := newSDongleRig(t, 7)
	fresh := publishHuawei(t, dongleConfig(7), HuaweiCycle{Path: PathSDongle, Cycle: sdongleCycle(t, g, 500, 500)})
	gc := nodeOf(t, fresh, kindGridConnection, "gc-1")
	if got := fresh.fact(t, gc, "infrastructure.power.pcc", string(gc.ID)).Candidate.Quality.Availability; got != semreg.AvailabilityAvailable {
		t.Fatalf("a current reading was published as %s", got)
	}
	// The next cycle's meter reads fail; the provider keeps the last words as retained groups.
	for _, extent := range [][2]uint16{{37100, 1}, {37101, 24}, {37125, 1}, {37126, 13}} {
		g.m.exclude[addr(7, extent[0], extent[1])] = true
	}
	c := g.cycle()
	var retained bool
	for _, m := range c.Meter {
		retained = retained || (m.Retained && m.Offset == 37101)
	}
	if !retained {
		t.Fatalf("the provider did not retain the meter group: %+v", c.Meter)
	}
	pub := publishHuawei(t, dongleConfig(7), HuaweiCycle{Path: PathSDongle, Cycle: c})
	f := requireFact(t, pub, gc, string(gc.ID), want{"infrastructure.power.pcc", "grid", unitWatt, "500"})
	if f.Candidate.Quality.Availability != semreg.AvailabilityDegraded {
		t.Fatalf("a retained reading was published as %s", f.Candidate.Quality.Availability)
	}
}
