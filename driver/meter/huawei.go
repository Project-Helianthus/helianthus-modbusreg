package meter

import (
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/Project-Helianthus/helianthus-modbusreg/huawei"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

const (
	offsetPlantConsumption  = 37500
	offsetPlantGridExchange = 37502
	offsetEMMAConsumption   = 30356
	offsetEMMAEnergyUse     = 30326
	offsetEMMAFeedIn        = 30332
	offsetEMMASupply        = 30338
	offsetEMMAGridExchange  = 30358
)

var (
	loadPower  = element{"infrastructure.load.active_power", dimLoad, unitWatt}
	loadEnergy = element{"infrastructure.load.energy_consumed", dimLoad, unitJoule}
	energyIn   = element{"infrastructure.energy.import", dimMeter, unitJoule}
	energyOut  = element{"infrastructure.energy.export", dimMeter, unitJoule}
	pccEl      = element{"infrastructure.power.pcc", dimGridConnection, unitWatt}
)

var huaweiKinds = buildHuaweiKinds()

func buildHuaweiKinds() map[string]string {
	t := map[string]string{
		"active_power": "power.active.total", "power_factor": "power_factor.total",
		"export_active_energy": "energy.export.total", "import_active_energy": "energy.import.total",
	}
	for _, p := range []string{"a", "b", "c"} {
		t["phase_"+p+"_voltage"] = "voltage.phase." + p
		t["phase_"+p+"_current"] = "current.phase." + p
		t["phase_"+p+"_active_power"] = "power.active.phase." + p
	}
	return t
}

var undirectedCounters = map[string]bool{"total_active_energy": true, "negative_active_energy": true, "positive_active_energy": true}

func (r *run) huawei() error {
	cycles := append([]HuaweiCycle(nil), r.in.Huawei...)
	sort.SliceStable(cycles, func(i, j int) bool { return cycles[i].Path < cycles[j].Path })
	for _, m := range r.p.cfg.Huawei {
		for _, c := range cycles {
			if c.Path == m.Path {
				if err := r.huaweiMeter(m, c.Cycle); err != nil {
					return err
				}
			}
		}
	}
	for _, gp := range r.p.cfg.GridPoints {
		for _, c := range cycles {
			if c.Path == gp.Path {
				if err := r.gridPoint(gp, c.Cycle); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (r *run) huaweiMeter(m HuaweiMeter, c huawei.ProviderCycle) error {
	var groups []huawei.ProviderMeterGroup
	for _, g := range c.Meter {
		if g.UnitID == m.Unit && !g.Refused {
			groups = append(groups, g)
		}
	}
	if len(groups) == 0 {
		return nil
	}
	f, err := r.meter(m.Slot, m.Role, m.GridConnection)
	if err != nil {
		return err
	}
	for _, g := range groups {
		for _, fact := range g.Facts {
			if fact.Availability != huawei.FactValue {
				continue
			}
			if err := f.feed(meterReading(fact, g.Retained, string(m.Path))); err != nil {
				return err
			}
		}
	}
	return nil
}

func meterReading(f huawei.ProviderMeterFact, retained bool, path string) reading {
	rd := reading{kind: huaweiKinds[f.Key], field: f.Key, unit: f.Unit, value: decimalRat(f.Value.Coefficient, f.Value.Exponent), retained: retained, path: joinPath(path, strconv.Itoa(int(f.Offset)))}
	if f.Unit == "kW" || f.Unit == "W" {
		rd.polarity, rd.open = signOpen, openSign
		if f.Polarity == huawei.MeterPolarityExportPositive {
			rd.polarity, rd.open = signExportPositive, ""
		}
	}
	if undirectedCounters[f.Key] {
		rd.open = openCounter
	}
	return rd
}

func decimalRat(coefficient int64, exponent int16) *big.Rat {
	return scaled(new(big.Rat).SetInt64(coefficient), int64(exponent))
}

// gridPoint publishes the plant and site groups of one path.
func (r *run) gridPoint(gp HuaweiGridPoint, c huawei.ProviderCycle) error {
	var gc *driver.NodeRef
	if gp.GridConnection != "" {
		node, ok, err := r.gridConnection(gp.GridConnection)
		if err != nil {
			return err
		}
		if ok {
			gc = &node
		}
	}
	f := &feeder{r: r, slot: gp.Slot, grid: gc}
	for _, g := range c.Plant {
		for _, fact := range g.Facts {
			if fact.Availability != huawei.FactValue {
				continue
			}
			if err := r.plantFact(f, gp, fact, g.Retained); err != nil {
				return err
			}
		}
	}
	return r.siteGroups(f, gp, c)
}

func (r *run) plantFact(f *feeder, gp HuaweiGridPoint, fact huawei.ProviderPlantFact, retained bool) error {
	rd := reading{field: fact.Key, unit: fact.Unit, value: decimalRat(fact.Value.Coefficient, fact.Value.Exponent), retained: retained,
		path: joinPath(string(gp.Path), "plant", strconv.Itoa(int(fact.Offset)))}
	d, err := decimalOf(rd.value)
	if err != nil {
		return err
	}
	r.member(gp.Slot, "plant."+fact.Key, rd.native(d))
	switch fact.Offset {
	case offsetPlantConsumption:
		return r.load(f, gp, loadPower, rd, kiloToBase)
	case offsetPlantGridExchange:
		if f.grid == nil {
			return nil
		}
		rd.kind, rd.polarity = "power.active.total", signImportPositive
		return f.activePower(rd)
	}
	return nil
}

func (r *run) load(f *feeder, gp HuaweiGridPoint, el element, rd reading, conv factor) error {
	node, err := r.node(kindLoad, gp.Slot, nil)
	if err != nil {
		return err
	}
	v, err := quantity(conv.apply(rd.value), el.unit)
	if err != nil {
		return err
	}
	d, _ := decimalOf(rd.value)
	_, err = r.emit(node, el, string(node.ID), v, source{path: joinPath(gp.Slot, rd.path), native: rd.native(d), retained: rd.retained})
	return err
}

func (r *run) siteGroups(f *feeder, gp HuaweiGridPoint, c huawei.ProviderCycle) error {
	ids := map[uint16]semreg.CandidateID{}
	for _, g := range c.EMMASite {
		for _, fact := range g.Facts {
			if err := r.siteFact(f, gp, fact, g.Retained, ids); err != nil {
				return err
			}
		}
	}
	for _, reading := range c.EMMASitePower {
		if reading.Offset == offsetEMMAGridExchange {
			return r.derivedExchange(f, gp, reading, ids)
		}
	}
	return nil
}

func siteRat(f huawei.EMMASiteFact) *big.Rat {
	if strings.HasPrefix(f.Type, "U") {
		return scaled(new(big.Rat).SetInt(new(big.Int).SetUint64(f.Unsigned)), int64(f.Exponent))
	}
	return scaled(new(big.Rat).SetInt64(int64(f.Signed)), int64(f.Exponent))
}

func (r *run) siteFact(f *feeder, gp HuaweiGridPoint, fact huawei.EMMASiteFact, retained bool, ids map[uint16]semreg.CandidateID) error {
	if fact.Availability != huawei.FactValue {
		return nil
	}
	rd := reading{field: fact.Key, unit: fact.Unit, value: siteRat(fact), retained: retained, path: joinPath(string(gp.Path), "site", strconv.Itoa(int(fact.Offset)))}
	if fact.PolarityUnknown {
		rd.polarity, rd.open = signOpen, openSign
	}
	d, err := decimalOf(rd.value)
	if err != nil {
		return err
	}
	r.member(gp.Slot, "site."+fact.Key, rd.native(d))
	switch fact.Offset {
	case offsetEMMAConsumption:
		return r.load(f, gp, loadPower, rd, kiloToBase)
	case offsetEMMAEnergyUse:
		return r.load(f, gp, loadEnergy, rd, kwhToJoule)
	case offsetEMMAFeedIn, offsetEMMASupply:
		el := energyOut
		if fact.Offset == offsetEMMASupply {
			el = energyIn
		}
		node, err := r.node(kindMeter, gp.Slot, nil)
		if err != nil {
			return err
		}
		if f.grid != nil {
			r.link("infrastructure.relation.grid_connection_meter", *f.grid, node, gp.GridConnection, gp.Slot, string(RoleGridConnection))
		}
		f.node = node
		v, err := quantity(kwhToJoule.apply(rd.value), el.unit)
		if err != nil {
			return err
		}
		id, err := r.emit(node, el, string(node.ID), v, source{path: joinPath(gp.Slot, rd.path), native: rd.native(d), retained: retained})
		ids[fact.Offset] = id
		return err
	}
	return nil
}

// derivedExchange publishes the grid exchange power at offset 30358 with the
// sign the provider derived from the import and export counters, or records
// that the cycle leaves the sign unknown.
func (r *run) derivedExchange(f *feeder, gp HuaweiGridPoint, p huawei.EMMASitePowerReading, ids map[uint16]semreg.CandidateID) error {
	if f.grid == nil {
		return nil
	}
	gc := *f.grid
	if !p.Derived {
		r.unknown(gc, pccEl, string(gc.ID), p.Reason)
		return nil
	}
	rat := decimalRat(p.Value.Coefficient, p.Value.Exponent)
	v, err := quantity(kiloToBase.apply(rat), unitWatt)
	if err != nil {
		return err
	}
	d, err := decimalOf(rat)
	if err != nil {
		return err
	}
	in, out := ids[offsetEMMASupply], ids[offsetEMMAFeedIn]
	if in == "" || out == "" {
		r.unknown(gc, pccEl, string(gc.ID), "counter_facts_not_published")
		return nil
	}
	n := Native{Source: fmt.Sprint("gateway-self.", p.Offset), Unit: p.Unit, Value: d, Sign: "positive_import", Open: ""}
	_, err = r.emit(gc, pccEl, string(gc.ID), v, source{path: joinPath(gp.Slot, string(gp.Path), "site", strconv.Itoa(int(p.Offset)), "derived"), native: n, derived: &derivation{inputs: []string{string(in), string(out)}}})
	return err
}
