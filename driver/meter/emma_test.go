package meter

import (
	"reflect"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/huawei"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

func emmaConfig() Config {
	cfg := testConfig()
	cfg.Site = Site{Slot: "site-1", GridConnections: []string{"gc-1"}}
	cfg.Huawei = []HuaweiMeter{{Path: PathEMMA, Slot: "emma-meter", Unit: 0, Role: RoleGridConnection, GridConnection: "gc-1"}}
	cfg.GridPoints = []HuaweiGridPoint{{Path: PathEMMA, Slot: "emma-site", GridConnection: "gc-1"}}
	return cfg
}

func nodeOf(t *testing.T, p Publication, kind semreg.DefinitionID, slot string) driver.NodeRef {
	t.Helper()
	for _, n := range p.Nodes {
		if n.Key.Kind == kind && n.Key.Anchor.Slot == slot {
			return n.Ref
		}
	}
	t.Fatalf("no %s %s", kind, slot)
	return driver.NodeRef{}
}

func hasFact(p Publication, node driver.NodeRef, element semreg.DefinitionID) bool {
	for _, f := range p.Facts {
		if f.Node == node && f.Candidate.Key.FactID == element {
			return true
		}
	}
	return false
}

func unknownReason(t *testing.T, p Publication, node driver.NodeRef, element semreg.DefinitionID) string {
	t.Helper()
	for _, u := range p.Unknown {
		if u.Node == node && u.Key.FactID == element {
			return u.Reason
		}
	}
	t.Fatalf("%s is not recorded unknown", element)
	return ""
}

func loadKey(fact semreg.DefinitionID, load driver.NodeRef) semreg.FactKey {
	id := string(load.ID)
	return semreg.FactKey{PackID: infraID, PackVersion: infraVersion, FactID: fact, Dimensions: []semreg.Dimension{{ID: dimLoad, Value: semreg.Value{Kind: semreg.ValueText, Text: &id}}}}
}

func TestEMMAReplayPublishesSiteMeterAndConsumption(t *testing.T) {
	g := newEMMARig(t)
	pub := testPublisher(t, emmaConfig())
	publish := func(c huawei.ProviderCycle) Publication {
		out, err := pub.Publish(Input{Template: template(), Huawei: []HuaweiCycle{{Path: PathEMMA, Cycle: c}}})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	// A household load of 2.345 kW with a PV generation of 5 kW, in one cycle.
	g.site([3]uint64{1234, 56, 78}, [3]uint64{987654, 4500, 6000}, 5000, 2345, -500)
	g.meter(-750)
	first := publish(g.cycle())
	load, site, meter, gc := nodeOf(t, first, kindLoad, "emma-site"), nodeOf(t, first, kindMeter, "emma-site"), nodeOf(t, first, kindMeter, "emma-meter"), nodeOf(t, first, kindGridConnection, "gc-1")
	power := requireFact(t, first, load, string(load.ID), want{"infrastructure.load.active_power", "load", unitWatt, "2345"})
	if !reflect.DeepEqual(power.Candidate.Key, loadKey("infrastructure.load.active_power", load)) || power.Origin != driver.OriginMeasured {
		t.Fatalf("load power key/origin: %+v %s", power.Candidate.Key, power.Origin)
	}
	requireFact(t, first, load, string(load.ID), want{"infrastructure.load.energy_consumed", "load", unitJoule, "35555544000"})
	requireFact(t, first, site, string(site.ID), want{"infrastructure.energy.export", "meter", unitJoule, "162000000"})
	requireFact(t, first, site, string(site.ID), want{"infrastructure.energy.import", "meter", unitJoule, "216000000"})
	for _, n := range [][3]string{{"site.gateway-self.30324", "kWh", "12.34"}, {"site.gateway-self.30330", "kWh", "0.56"}, {"site.gateway-self.30336", "kWh", "0.78"}, {"site.gateway-self.30354", "kW", "5"}} {
		requireNative(t, first, "emma-site", n[0], n[1], n[2])
	}
	if hasFact(first, load, "infrastructure.load.energy_produced") {
		t.Fatal("a day counter or a PV value was published as a load fact")
	}
	if got := unknownReason(t, first, gc, "infrastructure.power.pcc"); got != "missing_previous_counters" {
		t.Fatalf("first cycle exchange: %s", got)
	}
	// The built-in sensor: no sign is stated, so the signed power is published
	// with the open point named and no split; the counters stay native.
	pcc := requireFact(t, first, gc, string(gc.ID), want{"infrastructure.power.pcc", "grid", unitWatt, "-750"})
	if pcc.Native.Open != openSign || unknownReason(t, first, gc, "infrastructure.power.import_active") != openSign || unknownReason(t, first, gc, "infrastructure.power.export_active") != openSign {
		t.Fatalf("open sign not named: %+v", pcc.Native)
	}
	requireFact(t, first, gc, string(gc.ID), want{"infrastructure.ac.power_factor", "grid", unitRatio, "0.98"})
	requireFact(t, first, meter, string(meter.ID)+".phase_a", want{"infrastructure.ac.voltage", "phase", unitVolt, "230.1"})
	requireFact(t, first, meter, string(meter.ID)+".phase_c", want{"infrastructure.ac.current", "phase", unitAmpere, "13.5"})
	requireFact(t, first, meter, string(meter.ID)+".phase_b", want{"infrastructure.ac.active_power", "phase", unitWatt, "200"})
	if hasFact(first, meter, "infrastructure.energy.import") || hasFact(first, meter, "infrastructure.energy.export") {
		t.Fatal("undirected built-in counters were published as import or export")
	}
	for _, n := range [][3]string{{"total_active_energy", "kWh", "50"}, {"negative_active_energy", "kWh", "10"}, {"positive_active_energy", "kWh", "40"}} {
		requireNative(t, first, "emma-meter", n[0], n[1], n[2])
	}
	for _, m := range first.Members {
		if m.Key.Local == "total_active_energy" && m.Native.Open != openCounter {
			t.Fatalf("counter direction open point not named: %+v", m.Native)
		}
	}

	// Day boundary: the daily counters reset, the totals advance independently;
	// supply advanced alone, so the derived exchange is an import.
	g.site([3]uint64{3, 0, 5}, [3]uint64{987700, 4500, 6010}, 5000, 2345, -500)
	second := publish(g.cycle())
	requireFact(t, second, load, string(load.ID), want{"infrastructure.load.energy_consumed", "load", unitJoule, "35557200000"})
	requireFact(t, second, site, string(site.ID), want{"infrastructure.energy.import", "meter", unitJoule, "216360000"})
	requireNative(t, second, "emma-site", "site.gateway-self.30324", "kWh", "0.03")
	derived := requireFact(t, second, gc, string(gc.ID), want{"infrastructure.power.pcc", "grid", unitWatt, "500"})
	var exchange Fact
	for _, f := range second.Facts {
		if f.Node == gc && f.Candidate.Key.FactID == "infrastructure.power.pcc" && f.Origin == driver.OriginDerived {
			exchange = f
		}
	}
	d := exchange.Candidate.Derivation
	if derived.Candidate.CandidateID == "" || d == nil || exchange.Candidate.Quality.Assertion != semreg.AssertionInferred || exchange.Candidate.Origin.Kind != semreg.OriginDerived || len(d.Inputs) != 2 {
		t.Fatalf("derived exchange without lineage: %+v", exchange.Candidate)
	}
	inputs := map[semreg.CandidateID]bool{}
	for _, in := range d.Inputs {
		inputs[in.CandidateID] = true
	}
	for _, e := range []semreg.DefinitionID{"infrastructure.energy.import", "infrastructure.energy.export"} {
		if !inputs[second.fact(t, site, e, string(site.ID)).Candidate.CandidateID] {
			t.Fatalf("lineage lacks the %s counter", e)
		}
	}

	g.site([3]uint64{3, 4, 5}, [3]uint64{987700, 4510, 6010}, 5000, 2345, 500)
	third := publish(g.cycle())
	requireFact(t, third, gc, string(gc.ID), want{"infrastructure.power.pcc", "grid", unitWatt, "-500"})

	g.site([3]uint64{3, 4, 5}, [3]uint64{987700, 4520, 6020}, 5000, 2345, 500)
	fourth := publish(g.cycle())
	if got := unknownReason(t, fourth, gc, "infrastructure.power.pcc"); got != "both_counters_advanced" {
		t.Fatalf("ambiguous cycle: %s", got)
	}
	for _, f := range fourth.Facts {
		if f.Origin == driver.OriginDerived {
			t.Fatalf("an ambiguous cycle published a derived value: %+v", f.Candidate.Key)
		}
	}
}

func TestEMMAListingWithAMeterEntryChangesNoModelledPublication(t *testing.T) {
	g := newEMMARig(t)
	g.site([3]uint64{1234, 56, 78}, [3]uint64{987654, 4500, 6000}, 5000, 2345, -500)
	g.meter(-750)
	bare := g.cycle()
	listed := bare
	listed.Children = append(append([]huawei.ProviderChildObservation(nil), bare.Children...), huawei.ProviderChildObservation{UnitID: 0, ListedClass: "PowerMeter"})
	a := publishHuawei(t, emmaConfig(), HuaweiCycle{Path: PathEMMA, Cycle: bare})
	b := publishHuawei(t, emmaConfig(), HuaweiCycle{Path: PathEMMA, Cycle: listed})
	if len(a.Facts) == 0 || !reflect.DeepEqual(a, b) {
		t.Fatalf("a meter listing entry changed the publication: %d facts", len(a.Facts))
	}
}
