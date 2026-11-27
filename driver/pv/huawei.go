package pv

import (
	"math/big"
	"sort"
	"strconv"
	"strings"

	hw "github.com/Project-Helianthus/helianthus-modbusreg/huawei"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

const (
	offsetEMMAYieldDay   = 30346
	offsetEMMAYieldTotal = 30348
	offsetEMMAPVPower    = 30354
	offsetEMMAInverterAC = 30364
	reasonNoPlantDCPower = "no_pv_system_dc_power_definition_in_pinned_pack"
	reasonLoggerSlot     = "logger_link_node_slot_not_planned"
	reasonNoCompanion    = "no_battery_or_meter_resource_current_in_the_cut"
)

func normalUnit(u string) string {
	switch strings.ToLower(strings.TrimSpace(u)) {
	case "℃", "°c", "c":
		return "℃"
	case "kvar":
		return "kvar"
	}
	return strings.TrimSpace(u)
}

// huawei publishes every child of every cycle the registry admits, and the
// plant values of the cycle's gateway path.
func (r *run) huawei() error {
	cycles := append([]hw.ProviderCycle(nil), r.in.Huawei...)
	sort.SliceStable(cycles, func(i, j int) bool { return cycles[i].Path < cycles[j].Path })
	for _, c := range cycles {
		if err := r.cycle(c); err != nil {
			return err
		}
	}
	return nil
}

func (r *run) plantSystem() (driver.NodeRef, error) {
	slot := r.p.cfg.System
	if slot == "" {
		slot = string(r.p.cfg.Instance)
	}
	return r.node(kindSystem, slot, nil)
}

func (r *run) cycle(c hw.ProviderCycle) error {
	var logger *driver.NodeRef
	if c.Path == hw.ProviderPathSmartLogger && len(c.Children) > 0 {
		if r.p.cfg.LoggerSlot == "" {
			r.withheld(string(c.Path)+".logger", reasonLoggerSlot)
		} else {
			n, err := r.node(kindEquipment, r.p.cfg.LoggerSlot, nil)
			if err != nil {
				return err
			}
			logger = &n
		}
	}
	children := append([]hw.ProviderChildObservation(nil), c.Children...)
	sort.Slice(children, func(i, j int) bool { return children[i].UnitID < children[j].UnitID })
	for _, child := range children {
		if child.Evidence == hw.ProviderEvidenceWithheld || child.Reading.ProfileID() == "" {
			continue
		}
		if err := r.child(c, child, logger); err != nil {
			return err
		}
	}
	return r.plant(c)
}

func (r *run) child(c hw.ProviderCycle, child hw.ProviderChildObservation, logger *driver.NodeRef) error {
	return r.childFacts(c, child.UnitID, child.Reading.Facts(), logger)
}

// childFacts publishes the typed readings of one admitted child.
func (r *run) childFacts(c hw.ProviderCycle, unit uint8, facts []hw.SUN2000Fact, logger *driver.NodeRef) error {
	slot := string(r.p.cfg.Instance) + "/" + strconv.Itoa(int(unit))
	system, err := r.node(kindSystem, slot, nil)
	if err != nil {
		return err
	}
	inv, err := r.node(kindInverter, slot+"#1", nil)
	if err != nil {
		return err
	}
	r.link(true, pvDef("pv.relation.system_inverter"), system, inv, gateRule, "one_pv_system_per_admitted_inverter_child")
	if logger != nil {
		r.link(true, relaysRef(), *logger, inv, nativeObservation, "each_admitted_child_is_read_through_the_gateway_path_its_profile_names")
	}
	phases := map[string]driver.NodeRef{}
	for i, suffix := range []string{"a", "b", "c"} {
		n, err := r.node(kindPhase, "L"+strconv.Itoa(i+1), &inv)
		if err != nil {
			return err
		}
		phases[suffix] = n
		r.link(true, pvDef("pv.relation.inverter_phase"), inv, n, gateRule, "each_dongle_family_inverter_child_has_three_phase_components")
	}
	if err := r.companions(unit, inv); err != nil {
		return err
	}
	inputs := huaweiInputs(facts)
	for _, f := range facts {
		if err := r.reading(slot, f, system, inv, phases, inputs); err != nil {
			return err
		}
	}
	return nil
}

// companions states the relays relation from the inverter to a battery or meter
// of its own family that is current in the cut. When none is current the
// relation stays unknown, with the missing resource named.
func (r *run) companions(unit uint8, inv driver.NodeRef) error {
	found := false
	for _, co := range r.in.Companions {
		if co.Unit == unit {
			found = true
			r.link(true, relaysRef(), inv, co.Node, nativeObservation, "the_inverter_child_reports_the_battery_or_meter_of_its_own_family")
		}
	}
	if !found {
		r.out.UnknownLinks = append(r.out.UnknownLinks, UnknownLink{Type: relaysRef(), From: inv, Reason: reasonNoCompanion})
	}
	return nil
}

// huaweiInputs lists the native string indexes whose voltage and current words
// show a present input. A zero pair is native evidence, not membership.
func huaweiInputs(facts []hw.SUN2000Fact) map[int]bool {
	type pair struct{ v, a *hw.SUN2000Fact }
	pairs := map[int]*pair{}
	for i := range facts {
		f := &facts[i]
		if f.Index <= 0 || f.Availability != hw.FactValue {
			continue
		}
		p := pairs[f.Index]
		if p == nil {
			p = &pair{}
			pairs[f.Index] = p
		}
		switch nameOf(f.Key) {
		case "pv_string_voltage":
			p.v = f
		case "pv_string_current":
			p.a = f
		}
	}
	out := map[int]bool{}
	for index, p := range pairs {
		if p.v != nil && p.a != nil && (p.v.Value.Coefficient != 0 || p.a.Value.Coefficient != 0) {
			out[index] = true
		}
	}
	return out
}

func nameOf(key string) string { return key[strings.LastIndex(key, ".")+1:] }

func (r *run) reading(slot string, f hw.SUN2000Fact, system, inv driver.NodeRef, phases map[string]driver.NodeRef, inputs map[int]bool) error {
	name := nameOf(f.Key)
	local := f.Key
	if f.Index > 0 {
		local += "." + strconv.Itoa(f.Index)
	}
	point := Point{Name: local, Field: f.Key, Unit: f.Unit, Validity: string(f.Availability), State: f.State, Retained: f.Retained}
	if f.Availability != hw.FactValue {
		r.member(slot, local, point)
		return nil
	}
	rat := decimalRat(f.Value.Coefficient, f.Value.Exponent)
	d, err := decimalOf(rat)
	if err != nil {
		return err
	}
	point.Value = plain(strconv.FormatInt(f.Value.Coefficient, 10), int(f.Value.Exponent))
	r.member(slot, local, point)
	path := joinPath(slot, local)
	native := Native{Source: f.Key, Unit: f.Unit, Value: d}
	src := source{path: path, native: native, retained: f.Retained}
	switch name {
	case "status":
		return r.status(slot, f, inv, src)
	}
	rw, ok := huaweiRows[name]
	if !ok {
		r.withheld(path, reasonNoElement)
		return nil
	}
	if rw.native != "" && normalUnit(f.Unit) != normalUnit(rw.native) {
		r.withheld(path, reasonUnit)
		return nil
	}
	switch rw.scope {
	case onSystem:
		return r.publish(system, rw, string(system.ID), rat, src)
	case onPhase:
		n := phases[rw.suffix]
		return r.publish(n, rw, string(n.ID), rat, src)
	case onLinePair:
		return r.publish(inv, rw, string(inv.ID)+".line_"+rw.suffix, rat, src)
	case onInput:
		if !inputPresent(inputs, f.Index) {
			r.withheld(path, "dc_input_not_present_zero_pair_is_not_membership")
			return nil
		}
		n, err := r.node(kindInput, strconv.Itoa(f.Index), &inv)
		if err != nil {
			return err
		}
		r.link(true, pvDef("pv.relation.inverter_input"), inv, n, gateRule, "native_string_index_with_a_nonzero_voltage_or_current_word")
		return r.publish(n, rw, string(n.ID), rat, src)
	}
	return r.publish(inv, rw, string(inv.ID), rat, src)
}

func inputPresent(inputs map[int]bool, index int) bool { return index > 0 && inputs[index] }

// status publishes the operating state, the derating and the fault of a child
// from the registry's state dictionary. A name absent from the table is
// published raw in the namespace with its reason and listed.
func (r *run) status(slot string, f hw.SUN2000Fact, inv driver.NodeRef, src source) error {
	st, ok := huaweiStates[f.State]
	if !ok {
		r.withheld(src.path, reasonStateNotTable+":"+f.State)
		return nil
	}
	id := string(inv.ID)
	if err := r.emit(inv, operatingEl, id, symbol(operatingEl, st.operating), src); err != nil {
		return err
	}
	derating := "clear"
	if st.derating {
		derating = "active"
	}
	fault := "clear"
	if st.fault {
		fault = "present"
	}
	d, fl := src, src
	d.path, fl.path = src.path+".derating", src.path+".fault"
	if err := r.emit(inv, deratingEl, id, symbol(deratingEl, derating), d); err != nil {
		return err
	}
	return r.emit(inv, faultEl, id, symbol(faultEl, fault), fl)
}

type plantValue struct {
	key, unit string
	offset    int
	rat       *big.Rat
	retained  bool
	native    semreg.Decimal
}

// plantValues lists the plant values of a cycle's gateway groups.
func plantValues(c hw.ProviderCycle) ([]plantValue, error) {
	var values []plantValue
	add := func(key string, offset uint16, unit string, rat *big.Rat, retained bool) error {
		d, err := decimalOf(rat)
		if err != nil {
			return err
		}
		values = append(values, plantValue{key: key, unit: unit, offset: int(offset), rat: rat, retained: retained, native: d})
		return nil
	}
	for _, g := range c.Plant {
		for _, f := range g.Facts {
			if f.Availability == hw.FactValue {
				if err := add(f.Key, f.Offset, f.Unit, decimalRat(f.Value.Coefficient, f.Value.Exponent), g.Retained); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, g := range c.EMMASite {
		for _, f := range g.Facts {
			if f.Availability != hw.FactValue {
				continue
			}
			rat := scaled(new(big.Rat).SetInt(new(big.Int).SetUint64(f.Unsigned)), int64(f.Exponent))
			if strings.HasPrefix(f.Type, "I") {
				rat = scaled(new(big.Rat).SetInt64(int64(f.Signed)), int64(f.Exponent))
			}
			if err := add(f.Key, f.Offset, f.Unit, rat, g.Retained); err != nil {
				return nil, err
			}
		}
	}
	return values, nil
}

// plant publishes the plant values of a path on the PV system: the plant energy,
// the plant daily yield and the plant AC power (sign kept). The plant DC power
// has no system-scoped element in the pinned pack and is published in the
// namespace only. A value the path does not supply is declared nowhere.
func (r *run) plant(c hw.ProviderCycle) error {
	values, err := plantValues(c)
	if err != nil {
		return err
	}
	var system *driver.NodeRef
	for _, v := range values {
		kind, rw := plantElement(c.Path, v.key, v.offset)
		if kind == plantOther {
			continue
		}
		slot := string(r.p.cfg.Instance)
		path := joinPath(string(c.Path), "plant", strconv.Itoa(v.offset))
		r.member(slot, "plant."+v.key, Point{Name: "plant." + v.key, Field: v.key, Unit: v.unit, Value: plain(v.native.Coefficient, int(v.native.Exponent10)), Validity: string(hw.FactValue), Retained: v.retained})
		switch kind {
		case plantDC:
			r.withheld(path, reasonNoPlantDCPower)
		case plantEnergy, plantACPower:
			if v.unit != rw.native {
				r.withheld(path, reasonUnit)
				continue
			}
			if system == nil {
				n, err := r.plantSystem()
				if err != nil {
					return err
				}
				system = &n
			}
			if err := r.publish(*system, rw, string(system.ID), v.rat, source{path: path, native: Native{Source: v.key, Unit: v.unit, Value: v.native}, retained: v.retained}); err != nil {
				return err
			}
		}
	}
	if c.Path == hw.ProviderPathSDongle {
		r.withheld(string(c.Path)+".plant", "path_supplies_no_plant_total_energy_or_daily_yield")
	}
	return nil
}

type plantKind int

const (
	plantOther plantKind = iota
	plantEnergy
	plantACPower
	plantDC
)

// plantElement classifies a plant value of a gateway path by its documented key
// or register offset.
func plantElement(path hw.ProviderPath, key string, offset int) (plantKind, row) {
	switch {
	case key == plantTotalEnergy || offset == offsetEMMAYieldTotal:
		return plantEnergy, row{el: pvEl("pv.energy.generated", dimSystem, unitJoule), scope: onSystem, native: "kWh", convert: by(kwhToJoule)}
	case key == plantDailyEnergy || offset == offsetEMMAYieldDay:
		return plantEnergy, row{el: pvEl("pv.energy.yield_day", dimSystem, unitJoule), scope: onSystem, native: "kWh", convert: by(kwhToJoule)}
	case key == plantACKey || offset == offsetEMMAInverterAC:
		return plantACPower, row{el: pvEl("pv.system.active_power", dimSystem, unitWatt), scope: onSystem, native: "kW", convert: by(kiloToBase)}
	case key == "plant_dc_input_power" || offset == offsetEMMAPVPower:
		return plantDC, row{}
	}
	return plantOther, row{}
}
