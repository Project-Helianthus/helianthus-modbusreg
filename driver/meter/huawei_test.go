package meter

import (
	"reflect"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/huawei"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

func mfact(key string, offset uint16, typ, unit string, coefficient int64, exponent int16, polarity huawei.MeterPolarity) huawei.ProviderMeterFact {
	return huawei.ProviderMeterFact{Key: key, Offset: offset, Type: typ, Unit: unit, Value: huawei.Decimal{Coefficient: coefficient, Exponent: exponent},
		Availability: huawei.FactValue, Grade: huawei.GradeHypothesis, Polarity: polarity}
}

func answered(offset, quantity uint16) huawei.ProviderGroupEvidence {
	return huawei.ProviderGroupEvidence{Offset: offset, Quantity: quantity, Outcome: huawei.SUN2000GroupAnswered, Cycle: 1, Generation: 1}
}

// dongleMeter is the inverter-side meter group of a routed child unit.
func dongleMeter(unit uint8, power int64, pf int64) huawei.ProviderMeterGroup {
	e := huawei.MeterPolarityExportPositive
	return huawei.ProviderMeterGroup{UnitID: unit, ProviderGroupEvidence: answered(37101, 24), Facts: []huawei.ProviderMeterFact{
		mfact("phase_a_voltage", 37101, "I32", "V", 2300, -1, ""), mfact("phase_b_voltage", 37103, "I32", "V", 2310, -1, ""), mfact("phase_c_voltage", 37105, "I32", "V", 2320, -1, ""),
		mfact("phase_a_current", 37107, "I32", "A", 410, -2, ""), mfact("phase_b_current", 37109, "I32", "A", 420, -2, ""), mfact("phase_c_current", 37111, "I32", "A", 430, -2, ""),
		mfact("active_power", 37113, "I32", "W", power, 0, e), mfact("power_factor", 37117, "I16", "", pf, -3, ""),
		mfact("export_active_energy", 37119, "I32", "kWh", 1234, -2, ""), mfact("import_active_energy", 37121, "I32", "kWh", 567, -2, ""),
	}}
}

func donglePlant(consumptionW, exchangeW int64) huawei.ProviderPlantGroup {
	return huawei.ProviderPlantGroup{UnitID: 100, ProviderGroupEvidence: answered(37498, 8), Facts: []huawei.ProviderPlantFact{
		{Key: "plant_consumption_power", Offset: 37500, Unit: "kW", Availability: huawei.FactValue, Value: huawei.Decimal{Coefficient: consumptionW, Exponent: -3}},
		{Key: "plant_grid_exchange_power", Offset: 37502, Unit: "kW", Availability: huawei.FactValue, Value: huawei.Decimal{Coefficient: exchangeW, Exponent: -3}},
	}}
}

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

func TestSDongleChildMeterPolarityAgainstPlantExchange(t *testing.T) {
	for _, unit := range []uint8{1, 7} {
		cycle := func(meterW, plantW int64, listed bool) HuaweiCycle {
			c := huawei.ProviderCycle{Path: huawei.ProviderPathSDongle, Meter: []huawei.ProviderMeterGroup{dongleMeter(unit, meterW, 980)}, Plant: []huawei.ProviderPlantGroup{donglePlant(1500, plantW)}}
			if listed {
				c.Children = []huawei.ProviderChildObservation{{UnitID: unit, ListedClass: "PowerMeter"}}
			}
			return HuaweiCycle{Path: PathSDongle, Cycle: c}
		}
		plus := publishHuawei(t, dongleConfig(unit), cycle(500, 500, false))
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

		minus := publishHuawei(t, dongleConfig(unit), cycle(-500, -500, false))
		requireFact(t, minus, gc, string(gc.ID), want{"infrastructure.power.import_active", "grid", unitWatt, "500"})
		requireFact(t, minus, gc, string(gc.ID), want{"infrastructure.power.export_active", "grid", unitWatt, "0"})
		requireFact(t, minus, gc, string(gc.ID), want{"infrastructure.power.pcc", "grid", unitWatt, "-500"})
		requireFact(t, minus, plant, string(plant.ID), want{"infrastructure.power.export_active", "grid", unitWatt, "500"})

		// A meter inventory entry for the same unit changes nothing that is modelled.
		listed := publishHuawei(t, dongleConfig(unit), cycle(500, 500, true))
		if !reflect.DeepEqual(plus.Facts, listed.Facts) || !reflect.DeepEqual(plus.Nodes, listed.Nodes) || !reflect.DeepEqual(plus.Links, listed.Links) {
			t.Fatalf("unit %d: a listing entry changed the publication", unit)
		}
	}
}

func TestHuaweiMeterKeepsItsResourceWhenTheRoutedUnitChanges(t *testing.T) {
	at := func(unit uint8) Publication {
		return publishHuawei(t, dongleConfig(unit), HuaweiCycle{Path: PathSDongle, Cycle: huawei.ProviderCycle{Meter: []huawei.ProviderMeterGroup{dongleMeter(unit, 500, 980)}}})
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

func loggerMeter(unit uint8, polarity huawei.MeterPolarity, pf int64, power int64) huawei.ProviderMeterGroup {
	return huawei.ProviderMeterGroup{UnitID: unit, ProviderGroupEvidence: answered(32260, 33), Facts: []huawei.ProviderMeterFact{
		mfact("phase_a_voltage", 32260, "U32", "V", 23001, -2, ""), mfact("phase_a_current", 32272, "I32", "A", 114, -1, ""),
		mfact("active_power", 32278, "I32", "kW", power, -3, polarity), mfact("power_factor", 32284, "I16", "", pf, -3, ""),
		mfact("phase_a_active_power", 32335, "I32", "kW", 250, -3, polarity),
		mfact("total_active_energy", 32341, "I64", "kWh", 100000, -2, ""), mfact("negative_active_energy", 32349, "I64", "kWh", 20000, -2, ""), mfact("positive_active_energy", 32357, "I64", "kWh", 80000, -2, ""),
	}}
}

func loggerConfig(roles map[string]Role) Config {
	cfg := testConfig()
	cfg.Site = Site{Slot: "site-1", GridConnections: []string{"gc-1"}}
	for slot, unit := range map[string]uint8{"grid-meter": 12, "feeder-meter": 13, "plain-meter": 14} {
		cfg.Huawei = append(cfg.Huawei, HuaweiMeter{Path: PathLogger, Slot: slot, Unit: unit, Role: roles[slot], GridConnection: "gc-1"})
	}
	return cfg
}

func TestFeederAndUnassignedMetersContributeNothingToTheGridConnection(t *testing.T) {
	cfg := loggerConfig(map[string]Role{"grid-meter": RoleGridConnection, "feeder-meter": RoleFeeder})
	c := huawei.ProviderCycle{Path: huawei.ProviderPathSmartLogger, Meter: []huawei.ProviderMeterGroup{
		loggerMeter(12, huawei.MeterPolarityExportPositive, 980, 750), loggerMeter(13, huawei.MeterPolarityExportPositive, 720, 600), loggerMeter(14, huawei.MeterPolarityUnknown, 650, 100)}}
	pub := publishHuawei(t, cfg, HuaweiCycle{Path: PathLogger, Cycle: c})
	gc := nodeOf(t, pub, kindGridConnection, "gc-1")
	site := nodeOf(t, pub, kindSite, "site-1")
	// Only the grid-role meter reaches the grid connection: 0.98, never 0.72 or 0.65.
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
	for _, slot := range []string{"feeder-meter", "plain-meter"} {
		m := nodeOf(t, pub, kindMeter, slot)
		requireFact(t, pub, m, string(m.ID)+".phase_a", want{"infrastructure.ac.voltage", "phase", unitVolt, "230.01"})
		requireFact(t, pub, m, string(m.ID)+".phase_a", want{"infrastructure.ac.active_power", "phase", unitWatt, "250"})
		requireNative(t, pub, slot, "power_factor", "", map[string]string{"feeder-meter": "0.72", "plain-meter": "0.65"}[slot])
		requireNative(t, pub, slot, "active_power", "kW", map[string]string{"feeder-meter": "0.6", "plain-meter": "0.1"}[slot])
		requireNative(t, pub, slot, "positive_active_energy", "kWh", "800")
	}
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
	for _, l := range pub.Links {
		if l.Key.Type.ID == "infrastructure.relation.grid_connection_meter" && (l.Key.To == nodeOf(t, pub, kindMeter, "feeder-meter") || l.Key.To == nodeOf(t, pub, kindMeter, "plain-meter")) {
			t.Fatal("a feeder or unassigned meter has a grid-connection relation")
		}
	}
}

func TestLoggerEditionSignDecidesTheSplit(t *testing.T) {
	cfg := loggerConfig(map[string]Role{"grid-meter": RoleGridConnection})
	cfg.Huawei = cfg.Huawei[:0]
	cfg.Huawei = append(cfg.Huawei, HuaweiMeter{Path: PathLogger, Slot: "grid-meter", Unit: 12, Role: RoleGridConnection, GridConnection: "gc-1"})
	stated := publishHuawei(t, cfg, HuaweiCycle{Path: PathLogger, Cycle: huawei.ProviderCycle{Meter: []huawei.ProviderMeterGroup{loggerMeter(12, huawei.MeterPolarityExportPositive, 980, -750)}}})
	gc := nodeOf(t, stated, kindGridConnection, "gc-1")
	requireFact(t, stated, gc, string(gc.ID), want{"infrastructure.power.import_active", "grid", unitWatt, "750"})
	open := publishHuawei(t, cfg, HuaweiCycle{Path: PathLogger, Cycle: huawei.ProviderCycle{Meter: []huawei.ProviderMeterGroup{loggerMeter(12, huawei.MeterPolarityUnknown, 980, -750)}}})
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
	cfg := testConfig()
	cfg.Huawei = []HuaweiMeter{{Path: PathLogger, Slot: "listed-meter", Unit: 12}}
	c := huawei.ProviderCycle{Children: []huawei.ProviderChildObservation{{UnitID: 12, ListedClass: "PowerMeter"}},
		Meter: []huawei.ProviderMeterGroup{{UnitID: 12, ProviderGroupEvidence: huawei.ProviderGroupEvidence{Offset: 32260, Quantity: 33, Outcome: huawei.SUN2000GroupNotRead}}}}
	pub := publishHuawei(t, cfg, HuaweiCycle{Path: PathLogger, Cycle: c})
	if len(pub.Nodes) != 1 || pub.Nodes[0].Key.Kind != kindMeter || pub.Nodes[0].Key.Anchor != (driver.AnchorKey{Slot: "listed-meter"}) {
		t.Fatalf("nodes %+v", pub.Nodes)
	}
	if len(pub.Facts) != 0 || len(pub.Members) != 0 || len(pub.Links) != 0 {
		t.Fatalf("a class label published values or an identity: %+v", pub)
	}
}

func TestRetainedReadingKeepsItsValueAsDegraded(t *testing.T) {
	group := dongleMeter(7, 500, 980)
	group.Retained = true
	pub := publishHuawei(t, dongleConfig(7), HuaweiCycle{Path: PathSDongle, Cycle: huawei.ProviderCycle{Meter: []huawei.ProviderMeterGroup{group}}})
	gc := nodeOf(t, pub, kindGridConnection, "gc-1")
	f := requireFact(t, pub, gc, string(gc.ID), want{"infrastructure.power.pcc", "grid", unitWatt, "500"})
	if f.Candidate.Quality.Availability != semreg.AvailabilityDegraded {
		t.Fatalf("a retained reading was published as %s", f.Candidate.Quality.Availability)
	}
	fresh := publishHuawei(t, dongleConfig(7), HuaweiCycle{Path: PathSDongle, Cycle: huawei.ProviderCycle{Meter: []huawei.ProviderMeterGroup{dongleMeter(7, 500, 980)}}})
	if got := fresh.fact(t, gc, "infrastructure.power.pcc", string(gc.ID)).Candidate.Quality.Availability; got != semreg.AvailabilityAvailable {
		t.Fatalf("a current reading was published as %s", got)
	}
}
