package pv

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

// inverter_ publishes the facts of an inverter model (101 to 103, 111 to 113).
func (dev *device) inverter_(d sunspec.CapabilityDecision, state map[string]sunspec.CapabilityFact) error {
	facts := d.Facts()
	byID := map[string]sunspec.CapabilityFact{}
	for _, f := range facts {
		byID[f.FieldID()] = f
	}
	for _, f := range facts {
		if rw, ok := sunSpecRows[f.FieldID()]; ok {
			if err := dev.feed(d, f, rw); err != nil {
				return err
			}
			continue
		}
		switch f.FieldID() {
		case "inverter.operating_state", "inverter.events.1":
		default:
			dev.r.withheld(dev.path(d, f.FieldID()), reasonNoElement)
		}
	}
	if err := dev.status(d, byID["inverter.operating_state"], byID["inverter.events.1"], state); err != nil {
		return err
	}
	return dev.limits(d, state)
}

// status publishes the operating state, the derating and the fault of the
// inverter from the state enum, the first event word and the model 122 active
// controls. A value no source states is not published, never zero.
func (dev *device) status(d sunspec.CapabilityDecision, st, evt sunspec.CapabilityFact, state map[string]sunspec.CapabilityFact) error {
	r := dev.r
	stKnown, stFault, stThrottled := false, false, false
	if _, symbol, ok := st.Value().Enum(); ok {
		name := stripVendorPrefix(symbol)
		path := dev.path(d, "inverter.operating_state")
		native := Native{Source: st.FieldID(), Unit: st.Unit()}
		if n, _, ok := st.Value().Enum(); ok {
			native.Value = semreg.Decimal{Coefficient: strconv.FormatUint(n, 10)}
		}
		switch row, listed := sunSpecStates[name]; {
		case name == "FAULT":
			stKnown, stFault = true, true
			r.withheld(path, "fault_state_published_on_pv.status.fault")
		case listed:
			stKnown, stThrottled = true, name == "THROTTLED"
			if err := r.emit(dev.inverter, operatingEl, string(dev.inverter.ID), symbol_(operatingEl, row.token), source{path: path, native: native}); err != nil {
				return err
			}
			if row.loss != "" {
				r.withheld(path, "symbol_reduced:"+row.loss)
			}
		default:
			r.withheld(path, reasonStateNotTable)
		}
	}
	ctlKnown, ctlActive := false, false
	if f, ok := state["der.status.active_controls"]; ok {
		if bits, _, ok := f.Value().Bitfield(); ok {
			ctlKnown, ctlActive = true, bits != 0
		}
	}
	if stKnown || ctlKnown {
		token := "clear"
		if stThrottled || ctlActive {
			token = "active"
		}
		if err := r.emit(dev.inverter, deratingEl, string(dev.inverter.ID), symbol_(deratingEl, token), source{path: dev.path(d, "inverter.derating"), native: Native{Source: "inverter.operating_state"}}); err != nil {
			return err
		}
	}
	evtKnown, evtFault := false, false
	if bits, _, ok := evt.Value().Bitfield(); ok {
		evtKnown, evtFault = true, bits&sunSpecFaultEvents != 0
	}
	if stKnown || evtKnown {
		token := "clear"
		if stFault || evtFault {
			token = "present"
		}
		return r.emit(dev.inverter, faultEl, string(dev.inverter.ID), symbol_(faultEl, token), source{path: dev.path(d, "inverter.fault"), native: Native{Source: "inverter.events.1"}})
	}
	return nil
}

func symbol_(el element, token string) semreg.Value { return symbol(el, token) }

// limits publishes the model 123 active power limit when it is enabled: as a
// ratio, and as watts of the decoded maximum active power.
func (dev *device) limits(d sunspec.CapabilityDecision, state map[string]sunspec.CapabilityFact) error {
	pct, ok := state["der.control.active_power_limit.percent"]
	if !ok {
		return nil
	}
	path := dev.path(d, "der.control.active_power_limit.percent")
	_, symbol, enum := state["der.control.active_power_limit.enabled"].Value().Enum()
	if !enum || symbol != "ENABLED" {
		dev.r.withheld(path, "limit_not_enabled")
		return nil
	}
	v, ok := numberOf(pct)
	if !ok || pct.Unit() != "% WMax" {
		dev.r.withheld(path, reasonNotNumeric)
		return nil
	}
	ratio := limitRatio
	rw := row{el: ratio, native: "% WMax", convert: by(perHundred)}
	if err := dev.r.publish(dev.inverter, rw, string(dev.inverter.ID), v, source{path: path, native: nativeOf(pct, v)}); err != nil {
		return err
	}
	max, ok := state["der.settings.max_active_power"]
	if !ok {
		return nil
	}
	m, ok := numberOf(max)
	if !ok || max.Unit() != "W" {
		return nil
	}
	watts := new(big.Rat).Mul(perHundred.apply(v), m)
	q, err := quantity(watts, limitPower.unit)
	if err != nil {
		return err
	}
	return dev.r.emit(dev.inverter, limitPower, string(dev.inverter.ID), q, source{path: dev.path(d, "der.control.active_power_limit.watts"), native: nativeOf(pct, v)})
}

// derMeasurement publishes the models 701 and 714 values.
func (dev *device) derMeasurement(d sunspec.CapabilityDecision) error {
	if sourceModel(d) == 714 {
		return dev.ports(d)
	}
	for _, f := range d.Facts() {
		if rw, ok := sunSpecRows[f.FieldID()]; ok {
			if err := dev.feed(d, f, rw); err != nil {
				return err
			}
			continue
		}
		dev.r.withheld(dev.path(d, f.FieldID()), reasonNoElement)
	}
	return nil
}

func (dev *device) ports(d sunspec.CapabilityDecision) error {
	for _, f := range d.Facts() {
		dev.r.withheld(dev.path(d, f.FieldID()), reasonNoElement)
	}
	src, _ := d.SourceOccurrence()
	for _, e := range d.RepeatedEntries() {
		if !portIsPV(e) {
			dev.r.withheld(fmt.Sprintf("%s.m714.o%d.port-%d", dev.slot, src.Ordinal, e.Index), "dc_port_not_of_type_pv")
			continue
		}
		if err := dev.input(d, fmt.Sprintf("m714.o%d.port-%d", src.Ordinal, e.Index), e, portRows, "der.dc.port.energy.injected"); err != nil {
			return err
		}
	}
	return nil
}

func (dev *device) mppt(d sunspec.CapabilityDecision) error {
	src, _ := d.SourceOccurrence()
	for _, f := range d.Facts() {
		dev.r.withheld(dev.path(d, f.FieldID()), reasonNoElement)
	}
	for _, e := range d.RepeatedEntries() {
		if e.Role == "storage" {
			continue
		}
		if err := dev.input(d, fmt.Sprintf("m160.o%d.module-%d", src.Ordinal, e.Index), e, mpptRows, "mppt.module.dc_energy"); err != nil {
			return err
		}
	}
	return nil
}

// input publishes the values of one DC input and its relation to the inverter.
// The lifetime DC energy has no element whose dimension admits an input, so it
// is reported and stays in the namespace.
func (dev *device) input(d sunspec.CapabilityDecision, slot string, e sunspec.CapabilityEntry, rows map[string]row, energy string) error {
	node, err := dev.r.node(kindInput, slot, &dev.inverter)
	if err != nil {
		return err
	}
	dev.r.link(false, pvDef("pv.relation.inverter_input"), dev.inverter, node, gateRule, "each_module_or_pv_port_of_the_model_is_a_dc_input_of_the_inverter")
	for _, f := range e.Facts {
		path := dev.slot + "." + slot + "." + f.FieldID()
		if rw, ok := rows[f.FieldID()]; ok {
			v, ok := numberOf(f)
			switch {
			case !ok:
				dev.r.withheld(path, reasonNotNumeric)
			case !sameUnit(f.Unit(), rw.native):
				dev.r.withheld(path, reasonUnit)
			default:
				if err := dev.r.publish(node, rw, string(node.ID), v, source{path: path, native: nativeOf(f, v)}); err != nil {
					return err
				}
			}
			continue
		}
		if strings.HasSuffix(f.FieldID(), ".index") {
			continue
		}
		if f.FieldID() == energy {
			dev.r.withheld(path, "pv.dc.energy_dimension_is_string_not_input")
			continue
		}
		dev.r.withheld(path, reasonNoElement)
	}
	return nil
}
