package storage

import (
	"context"
	"math/big"
	"strconv"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/huawei"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

func catalogProfile(t *testing.T, card string) huawei.CatalogMonitoringProfile {
	t.Helper()
	profiles, err := huawei.CatalogMonitoringProfiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range profiles {
		if p.Card() == card {
			return p
		}
	}
	t.Fatalf("no catalogue card %s", card)
	return huawei.CatalogMonitoringProfile{}
}

// readings answers every planned group of a unit from the words by offset;
// a word that is not named is zero.
func readings(profile huawei.CatalogMonitoringProfile, unit uint8, words map[uint16]uint16) []huawei.SUN2000GroupReading {
	var out []huawei.SUN2000GroupReading
	for _, step := range profile.Plan(unit) {
		w := make([]uint16, step.Quantity)
		for i := range w {
			w[i] = words[step.Offset+uint16(i)]
		}
		out = append(out, huawei.SUN2000GroupReading{Offset: step.Offset, Quantity: step.Quantity, Outcome: huawei.SUN2000GroupAnswered, Words: w})
	}
	return out
}

func childOf(t *testing.T, card string, unit uint8, words map[uint16]uint16) HuaweiChild {
	t.Helper()
	profile := catalogProfile(t, card)
	reading, err := profile.Decode(unit, readings(profile, unit, words))
	if err != nil {
		t.Fatal(err)
	}
	return HuaweiChild{Unit: unit, Facts: reading.Facts(), BatteryAdmitted: true}
}

func withWords(words map[uint16]uint16) map[uint16]uint16 {
	base := map[uint16]uint16{
		37000: 2, 37001: 0, 37002: 1500, 37003: 5120, 37004: 507, 37007: 0, 37008: 5000, 37009: 0, 37010: 4000, 37021: 0xffdd, 37022: 245,
		37066: 0, 37067: 12345, 37068: 0, 37069: 6789,
		// pack 1 and pack 2 of unit 1
		38229: 606, 38235: 3000, 38236: 20, 38238: 0, 38239: 100, 38240: 0, 38241: 50,
		38271: 707, 38277: 3100, 38278: 30, 38280: 0, 38281: 200, 38282: 0, 38283: 80,
	}
	for k, v := range words {
		base[k] = v
	}
	return base
}

func huaweiConfig() Config {
	c := testConfig()
	c.Battery, c.System = "battery-a", "system-a"
	c.Huawei = []HuaweiBattery{{Unit: 1, Battery: "battery-a"}}
	return c
}

func publishHuawei(t *testing.T, p *Publisher, obs HuaweiObservation) {
	t.Helper()
	if err := p.PublishHuawei(context.Background(), obs); err != nil {
		t.Fatal(err)
	}
}

func TestHuaweiBatteryFacts(t *testing.T) {
	s := newSession("asset:inverter-a", "source-a", "epoch-1", 1)
	p := newPublisher(t, huaweiConfig(), s, nil)
	publishHuawei(t, p, HuaweiObservation{Children: []HuaweiChild{childOf(t, "LB-C00", 1, withWords(nil))}, Receipt: receipt(1)})
	k := s.k
	byDimension := map[string]map[semreg.DefinitionID]semreg.FactCandidate{}
	for _, c := range k.facts {
		node := *c.Key.Dimensions[0].Value.Text
		if byDimension[node] == nil {
			byDimension[node] = map[semreg.DefinitionID]semreg.FactCandidate{}
		}
		byDimension[node][c.Key.FactID] = c
	}
	// Three pack-dimension subjects: the storage unit and its two documented packs; one interface.
	packs, interfaces := 0, 0
	for node := range byDimension {
		switch k.resources[semreg.ResourceID(node)].Kind {
		case kindPack:
			packs++
		case kindInterface:
			interfaces++
		}
	}
	if packs != 3 || interfaces != 1 {
		t.Fatalf("%d packs and %d interfaces carry facts", packs, interfaces)
	}
	unit := factsOf(k, "storage.pack.voltage")
	if len(unit) != 3 {
		t.Fatalf("voltage facts %d", len(unit))
	}
	requireOne := func(element semreg.DefinitionID, coefficient string, exponent int32, unitID semreg.DefinitionID) {
		t.Helper()
		for _, c := range factsOf(k, element) {
			co, e, u := quantityOf(t, c)
			if co == coefficient && e == exponent && u == unitID {
				return
			}
		}
		t.Errorf("no %s = %se%d %s", element, coefficient, exponent, unitID)
	}
	requireOne("storage.pack.voltage", "512", 0, unitVolt)
	requireOne("storage.state.soc", "507", -3, unitRatio)
	requireOne("storage.pack.current", "-35", -1, unitAmpere)
	requireOne("storage.temperature.pack", "29765", -2, unitKelvin)
	requireOne("storage.energy.charged", "44442", 4, unitJoule)
	requireOne("storage.energy.discharged", "244404", 3, unitJoule)
	requireOne("storage.limit.charge_power", "5", 3, unitWatt)
	requireOne("storage.limit.discharge_power", "4", 3, unitWatt)
	// The two documented packs under their own pack dimensions.
	requireOne("storage.state.soc", "606", -3, unitRatio)
	requireOne("storage.state.soc", "707", -3, unitRatio)
	requireOne("storage.energy.charged", "36", 5, unitJoule)
	if got := symbolOf(t, only(t, k, elementOperating)); got != "storage.status.operating.active" {
		t.Errorf("operating %s", got)
	}
	// The power words document no sign: no fact states one, the native value stays readable.
	if len(factsOf(k, "storage.pack.power")) != 0 {
		t.Fatal("a power fact was published with no documented sign")
	}
	var open bool
	for _, m := range p.Members() {
		open = open || (m.Native.Open == reasonSignNotStated && m.Key.Local == "unit_1_power")
	}
	if !open {
		t.Error("the native power value is not in the namespace with its open sign")
	}
	for _, def := range []semreg.DefinitionID{relSystemBattery, relBatteryPack, relPackInterface} {
		if len(relationsOf(k, def)) == 0 || len(claimsOf(k, def)) != len(relationsOf(k, def)) {
			t.Errorf("%s: %d relations %d claims", def, len(relationsOf(k, def)), len(claimsOf(k, def)))
		}
	}
	if len(relationsOf(k, relBatteryPack)) != 3 {
		t.Errorf("battery_pack relations %d", len(relationsOf(k, relBatteryPack)))
	}
}

func TestHuaweiZeroPlaceholderIsNotAValueAndAnAbsentBatteryPublishesNothing(t *testing.T) {
	s := newSession("asset:inverter-a", "source-a", "epoch-1", 1)
	p := newPublisher(t, huaweiConfig(), s, nil)
	zero := map[uint16]uint16{}
	for _, o := range []uint16{37000, 37002, 37003, 37004, 37008, 37010, 37021, 37022, 37067, 37069, 38229, 38235, 38236, 38239, 38241, 38271, 38277, 38278, 38281, 38283} {
		zero[o] = 0
	}
	words := withWords(zero)
	for o := range words {
		words[o] = 0
	}
	publishHuawei(t, p, HuaweiObservation{Children: []HuaweiChild{childOf(t, "LB-C00", 1, words)}, Receipt: receipt(1)})
	if len(s.k.facts) != 0 {
		t.Fatalf("a zero placeholder published %d facts", len(s.k.facts))
	}
	// A pack of unit 1 that reads all zero is no pack: only the others publish.
	second := newSession("asset:inverter-a", "source-a", "epoch-1", 1)
	q := newPublisher(t, huaweiConfig(), second, nil)
	pack2 := withWords(map[uint16]uint16{38271: 0, 38277: 0, 38278: 0, 38281: 0, 38283: 0})
	publishHuawei(t, q, HuaweiObservation{Children: []HuaweiChild{childOf(t, "LB-C00", 1, pack2)}, Receipt: receipt(1)})
	if got := len(factsOf(second.k, "storage.pack.voltage")); got != 2 {
		t.Fatalf("voltage facts %d, want the unit and one pack", got)
	}
}

func TestHuaweiRetainedFactsKeepTheOriginalReceipt(t *testing.T) {
	s := newSession("asset:inverter-a", "source-a", "epoch-1", 1)
	p := newPublisher(t, huaweiConfig(), s, nil)
	child := childOf(t, "LB-C00", 1, withWords(nil))
	publishHuawei(t, p, HuaweiObservation{Children: []HuaweiChild{child}, Receipt: receipt(1)})
	before := map[semreg.CandidateID]semreg.FactCandidate{}
	for id, c := range s.k.facts {
		before[id] = c
	}
	retained := child
	retained.Facts = append([]huawei.SUN2000Fact(nil), child.Facts...)
	for i := range retained.Facts {
		retained.Facts[i].Retained = true
	}
	// The optional group failed this cycle: DD05 hands back the last-known-good facts as retained.
	publishHuawei(t, p, HuaweiObservation{Children: []HuaweiChild{retained}, Receipt: receipt(9)})
	if len(s.k.batches) != 1 || len(s.k.facts) != len(before) {
		t.Fatalf("batches %d facts %d: a retained reading republished", len(s.k.batches), len(s.k.facts))
	}
	for id, c := range s.k.facts {
		if c.Times.ReceivedAt != before[id].Times.ReceivedAt || *c.BindingID != *before[id].BindingID || c.Revision != before[id].Revision {
			t.Errorf("%s lost its original receipt, binding or revision", c.Key.FactID)
		}
	}
	// A new generation cannot borrow the old samples.
	next := s.successor("epoch-2", 2)
	q := newPublisher(t, huaweiConfig(), next, nil)
	publishHuawei(t, q, HuaweiObservation{Children: []HuaweiChild{retained}, Receipt: receipt(10)})
	for _, c := range next.k.facts {
		if c.DriverGeneration != nil && *c.DriverGeneration == "2" {
			t.Errorf("%s: the new generation published a retained sample", c.Key.FactID)
		}
	}
}

func emmaGroup(retained bool, facts ...huawei.EMMASiteFact) huawei.EMMASiteGroup {
	return huawei.EMMASiteGroup{UnitID: 0, ProviderGroupEvidence: huawei.ProviderGroupEvidence{Outcome: huawei.SUN2000GroupAnswered, Retained: retained}, Facts: facts}
}

func siteFact(offset uint16, typ, unit string, exponent int16, unsigned uint64, signed int32) huawei.EMMASiteFact {
	return huawei.EMMASiteFact{Offset: offset, Type: typ, Unit: unit, Exponent: exponent, Unsigned: unsigned, Signed: signed, Availability: huawei.FactValue}
}

func emmaConfig() Config {
	c := huaweiConfig()
	c.EMMASite = "battery-site"
	return c
}

func energyGroup(daily, total uint64) huawei.EMMASiteGroup {
	return emmaGroup(false,
		siteFact(30306, "U32", "kWh", -2, daily, 0), siteFact(30308, "U64", "kWh", -2, 123456789, 0),
		siteFact(30312, "U32", "kWh", -2, daily+1, 0), siteFact(30314, "U64", "kWh", -2, total, 0))
}

func powerGroup(signed int32) huawei.EMMASiteGroup {
	return emmaGroup(false, siteFact(30360, "I32", "kW", -3, 0, signed), siteFact(30368, "U16", "%", -2, 7250, 0))
}

func TestEMMASiteBatteryPromotesTheTotalsAndDerivedPower(t *testing.T) {
	s := newSession("asset:emma-a", "source-a", "epoch-1", 1)
	p := newPublisher(t, emmaConfig(), s, nil)
	obs := HuaweiObservation{
		Site: []huawei.EMMASiteGroup{energyGroup(10, 987654321), powerGroup(-800)},
		SitePower: []huawei.EMMASitePowerReading{
			{Offset: 30358, Unit: "kW", Availability: huawei.FactValue, Direction: huawei.EMMASitePowerImport, Value: huawei.Decimal{Coefficient: 2500, Exponent: -3}, Derived: true},
			{Offset: 30360, Unit: "kW", Availability: huawei.FactValue, Direction: huawei.EMMASitePowerDischarge, Value: huawei.Decimal{Coefficient: -800, Exponent: -3}, Derived: true,
				CounterOffsets: [2]uint16{30308, 30314},
				Previous:       huawei.EMMASitePowerCounterSample{Totals: [2]uint64{123456789, 987654000}}, Current: huawei.EMMASitePowerCounterSample{Totals: [2]uint64{123456789, 987654321}}},
		},
		Receipt: receipt(1),
	}
	publishHuawei(t, p, obs)
	k := s.k
	// Total charge 1234567.89 kWh and total discharge 9876543.21 kWh, in joules, from the totals alone.
	requireQuantity(t, k, "storage.energy.charged", "4444444404", 3, unitJoule)
	requireQuantity(t, k, "storage.energy.discharged", "35555555556", 3, unitJoule)
	requireQuantity(t, k, "storage.state.soc", "725", -3, unitRatio)
	// The battery's power takes its sign from the battery's counters: grid import beside it changes nothing.
	requireQuantity(t, k, "storage.pack.power", "-8", 2, unitWatt)
	// The four native counters stay native, daily and total each under its own identity.
	for _, local := range []string{"site_30306", "site_30308", "site_30312", "site_30314", "site_30360"} {
		var found bool
		for _, m := range p.Members() {
			found = found || string(m.Key.Local) == local
		}
		if !found {
			t.Errorf("%s is not in the namespace", local)
		}
	}
	// A daily rollover with unchanged totals leaves the promoted energies unchanged.
	before := map[semreg.DefinitionID]string{}
	for _, e := range []semreg.DefinitionID{"storage.energy.charged", "storage.energy.discharged"} {
		c, ex, _ := quantityOf(t, only(t, k, e))
		before[e] = c + "e" + itoa(uint16(ex+100))
	}
	obs.Site = []huawei.EMMASiteGroup{energyGroup(0, 987654321), powerGroup(-800)}
	obs.Receipt = receipt(2)
	publishHuawei(t, p, obs)
	for _, e := range []semreg.DefinitionID{"storage.energy.charged", "storage.energy.discharged"} {
		c, ex, _ := quantityOf(t, only(t, k, e))
		if before[e] != c+"e"+itoa(uint16(ex+100)) {
			t.Errorf("%s moved with the daily counter", e)
		}
	}
}

func TestEMMAAmbiguousBatteryCycleLeavesPowerUnknown(t *testing.T) {
	s := newSession("asset:emma-a", "source-a", "epoch-1", 1)
	p := newPublisher(t, emmaConfig(), s, nil)
	publishHuawei(t, p, HuaweiObservation{
		Site: []huawei.EMMASiteGroup{energyGroup(10, 987654321), powerGroup(-800)},
		SitePower: []huawei.EMMASitePowerReading{
			{Offset: 30358, Unit: "kW", Direction: huawei.EMMASitePowerExport, Derived: true, Value: huawei.Decimal{Coefficient: -2500, Exponent: -3}, Availability: huawei.FactValue},
			{Offset: 30360, Unit: "kW", Derived: false, Reason: "both_counters_advanced"},
		},
		Receipt: receipt(1)})
	if len(factsOf(s.k, "storage.pack.power")) != 0 {
		t.Fatal("an ambiguous battery cycle published a power")
	}
	unknown := p.Unknown()
	if len(unknown) != 1 || unknown[0].Reason != "both_counters_advanced" {
		t.Fatalf("unknown %+v", unknown)
	}
}

func TestHuaweiChildWithoutDocumentedBatteryGroupsIsSiteOnly(t *testing.T) {
	s := newSession("asset:emma-a", "source-a", "epoch-1", 1)
	p := newPublisher(t, emmaConfig(), s, nil)
	child := childOf(t, "LB-C00", 1, withWords(nil))
	child.BatteryAdmitted = false
	publishHuawei(t, p, HuaweiObservation{Children: []HuaweiChild{child}, Site: []huawei.EMMASiteGroup{energyGroup(10, 987654321), powerGroup(-800)}, Receipt: receipt(1)})
	if got := len(factsOf(s.k, "storage.state.soc")); got != 1 {
		t.Fatalf("state of charge facts %d: the child published a pack value", got)
	}
	var reason bool
	for _, m := range p.Members() {
		reason = reason || m.Native.Open == reasonNoInverterBattery
	}
	if !reason {
		t.Error("the site-only reason is not recorded")
	}
}

func TestHuaweiRunningStateTable(t *testing.T) {
	for _, c := range []struct {
		code  uint16
		token string
	}{{2, "active"}, {1, "idle"}, {4, "standby"}, {3, ""}} {
		s := newSession("asset:inverter-a", "source-a", "epoch-1", 1)
		p := newPublisher(t, huaweiConfig(), s, nil)
		publishHuawei(t, p, HuaweiObservation{Children: []HuaweiChild{childOf(t, "LB-C00", 1, withWords(map[uint16]uint16{37000: c.code}))}, Receipt: receipt(1)})
		got := factsOf(s.k, elementOperating)
		switch {
		case c.token == "" && len(got) != 0:
			t.Errorf("state %d published %s", c.code, symbolOf(t, got[0]))
		case c.token != "" && (len(got) != 1 || symbolOf(t, got[0]) != "storage.status.operating."+c.token):
			t.Errorf("state %d: %+v", c.code, got)
		}
	}
}

func TestHuaweiOfflineStateWordIsNoBatteryWhateverTheOtherWordsSay(t *testing.T) {
	s := newSession("asset:inverter-a", "source-a", "epoch-1", 1)
	p := newPublisher(t, huaweiConfig(), s, nil)
	publishHuawei(t, p, HuaweiObservation{Children: []HuaweiChild{childOf(t, "LB-C00", 1, withWords(map[uint16]uint16{37000: 0}))}, Receipt: receipt(1)})
	if len(s.k.facts) != 0 {
		t.Fatalf("an offline unit published %d facts", len(s.k.facts))
	}
}

func TestHuaweiWordsWithoutAnElementStayInTheNamespaceAndAForeignUnitIsNotMapped(t *testing.T) {
	s := newSession("asset:inverter-a", "source-a", "epoch-1", 1)
	p := newPublisher(t, huaweiConfig(), s, nil)
	child := childOf(t, "LB-C00", 1, withWords(map[uint16]uint16{37758: 0, 37759: 10000, 37760: 800}))
	publishHuawei(t, p, HuaweiObservation{Children: []HuaweiChild{child}, Receipt: receipt(1)})
	var system bool
	for _, m := range p.Members() {
		system = system || string(m.Key.Local) == "storage_37760"
	}
	if !system {
		t.Fatal("the system group state of charge is not in the namespace")
	}
	// A fact whose unit is not the row's unit is not mapped.
	wrong := child
	wrong.Facts = append([]huawei.SUN2000Fact(nil), child.Facts...)
	for i := range wrong.Facts {
		if wrong.Facts[i].Unit == "V" {
			wrong.Facts[i].Unit = "kV"
		}
	}
	other := newSession("asset:inverter-a", "source-a", "epoch-1", 1)
	q := newPublisher(t, huaweiConfig(), other, nil)
	publishHuawei(t, q, HuaweiObservation{Children: []HuaweiChild{wrong}, Receipt: receipt(1)})
	if len(factsOf(other.k, "storage.pack.voltage")) != 0 {
		t.Fatal("a voltage in another unit was published")
	}
}

// The rated capacity of the storage unit is published in joule on the pack
// dimension, with the native watt hours in the evidence; a zero word is no
// capacity.
func TestHuaweiRatedCapacityIsPublishedInJouleAndAZeroWordIsNot(t *testing.T) {
	s := newSession("asset:inverter-a", "source-a", "epoch-1", 1)
	p := newPublisher(t, huaweiConfig(), s, nil)
	child := childOf(t, "LB-C00", 1, withWords(map[uint16]uint16{37758: 0, 37759: 10000}))
	publishHuawei(t, p, HuaweiObservation{Children: []HuaweiChild{child}, Receipt: receipt(1)})
	facts := factsOf(s.k, "storage.energy.nominal_capacity")
	if len(facts) != 1 {
		t.Fatalf("%d nominal capacity facts", len(facts))
	}
	if co, e, u := quantityOf(t, facts[0]); co != "36" || e != 6 || u != unitJoule {
		t.Fatalf("nominal capacity %se%d %s", co, e, u)
	}
	if got := s.k.resources[semreg.ResourceID(*facts[0].Key.Dimensions[0].Value.Text)].Kind; got != kindPack {
		t.Fatalf("nominal capacity subject kind %s", got)
	}
	wh := decimalOf(big.NewRat(10000, 1), 6)
	var native bool
	for _, ref := range facts[0].Evidence {
		native = native || ref.Digest == evidence("evidence.native", "unit_1_rated_capacity", "Wh", wh.Coefficient, strconv.Itoa(int(wh.Exponent10)), "37758").Digest
	}
	if !native {
		t.Error("the native watt hours are not in the evidence")
	}
	for _, m := range p.Members() {
		if string(m.Key.Local) == "storage_37758" {
			t.Error("the published capacity is also in the namespace")
		}
	}

	zero := newSession("asset:inverter-a", "source-a", "epoch-1", 1)
	q := newPublisher(t, huaweiConfig(), zero, nil)
	publishHuawei(t, q, HuaweiObservation{Children: []HuaweiChild{childOf(t, "LB-C00", 1, withWords(map[uint16]uint16{37758: 0, 37759: 0}))}, Receipt: receipt(1)})
	if len(factsOf(zero.k, "storage.energy.nominal_capacity")) != 0 {
		t.Fatal("a zero placeholder word was published as a capacity")
	}
}
