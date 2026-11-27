package storage

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

const (
	sunspecProfile  = "sunspec.storage.monitoring"
	sunspecProtocol = "modbus_tcp"
	sunspecNative   = "native.sunspec.storage.observation"
	sunspecRegistry = "registry.sunspec.storage.mapping"

	// Model 160 reports lifetime DC watt-hours per module and model 714 reports
	// injected and absorbed watt-hours per port. The module role is unknown, and
	// a module label does not establish storage charge or discharge. The port
	// type has no name, so the port is not established as a storage port. Those
	// watt-hours stay in the namespace; the pack energy elements are withheld.
	reasonModuleRoleUnstated = "the model 160 module role is not stated, and a module label does not establish storage charge or discharge"
	reasonPortTypeUnnamed    = "the model 714 port type is not named, so the port is not established as a storage port"
)

// SunSpecStorage configures one storage segment of a chain. Position is the
// native position of the segment's Common model in its chain; Pack is the
// operator slot the segment's pack is named by under the configured battery.
type SunSpecStorage struct {
	Position uint32
	Pack     string
}

// SunSpecObservation is the native evaluation of one terminal chain with the
// receipt of its acquisition. The evaluation is the one the sunspec package
// decides; this producer maps it and sends nothing.
type SunSpecObservation struct {
	Decisions []sunspec.CapabilityDecision
	Receipt   Receipt
}

type sunRow struct {
	Model uint16
	Field string
	Row
}

// sunSpecRows map the native points of the storage models to elements. A point
// whose native unit differs from the row's is not mapped. Model 713 supplies
// the capacity, the state of charge and of health and the available energy;
// Model 802 supplies the electrical observations of the pack. The enumerations of
// models 713 and 802 carry no symbol table in the decoder, so their charge
// state stays native.
var sunSpecRows = []sunRow{
	{120, "storage.rating.energy", Row{Native: "WHRtg", NativeUnit: "Wh", Element: "storage.energy.nominal_capacity", Dimension: dimPack, Unit: unitJoule, Conv: whToJoule}},
	{124, "storage.status.state_of_charge", Row{Native: "ChaState", NativeUnit: "% AhrRtg", Element: "storage.state.soc", Dimension: dimPack, Unit: unitRatio, Conv: percent}},
	{124, "storage.status.internal_battery_voltage", Row{Native: "InBatV", NativeUnit: "V", Element: "storage.pack.voltage", Dimension: dimPack, Unit: unitVolt, Conv: identity}},
	{713, "storage.rating.energy", Row{Native: "WHRtg", NativeUnit: "Wh", Element: "storage.energy.nominal_capacity", Dimension: dimPack, Unit: unitJoule, Conv: whToJoule}},
	{713, "storage.status.available_energy", Row{Native: "WHAvail", NativeUnit: "Wh", Element: "storage.energy.remaining", Dimension: dimPack, Unit: unitJoule, Conv: whToJoule}},
	{713, "storage.status.state_of_charge", Row{Native: "SoC", NativeUnit: "Pct", Element: "storage.state.soc", Dimension: dimPack, Unit: unitRatio, Conv: percent}},
	{713, "storage.status.state_of_health", Row{Native: "SoH", NativeUnit: "Pct", Element: "storage.state.soh", Dimension: dimPack, Unit: unitRatio, Conv: percent}},
	{802, "battery.base.voltage", Row{Native: "V", NativeUnit: "V", Element: "storage.pack.voltage", Dimension: dimPack, Unit: unitVolt, Conv: identity}},
	{802, "battery.base.current", Row{Native: "A", NativeUnit: "A", Element: "storage.pack.current", Dimension: dimPack, Unit: unitAmpere, Conv: identity}},
	{802, "battery.base.power", Row{Native: "W", NativeUnit: "W", Element: "storage.pack.power", Dimension: dimPack, Unit: unitWatt, Conv: identity}},
	{802, "battery.base.cycle_count", Row{Native: "NCyc", NativeUnit: "none", Element: "storage.state.cycle_count", Dimension: dimPack, Unit: unitOne, Conv: identity}},
}

// chargeStates is the symbol table of the Model 124 charge state: the native
// names reduce to the coarser symbols of the element, and the reduction is a
// declared loss. TESTING has no counterpart and stays native.
var chargeStates = map[string]string{
	"CHARGING": "active", "DISCHARGING": "active", "HOLDING": "idle", "FULL": "idle", "OFF": "standby", "EMPTY": "standby",
}

// PublishSunSpec publishes the storage segments of one chain evaluation.
func (p *Publisher) PublishSunSpec(ctx context.Context, obs SunSpecObservation) error {
	return p.publish(ctx, func(st stamp) (plan, error) { return p.sunSpecPlan(obs) })
}

type segment struct {
	pack   int
	has124 bool
	ratio  map[string]*big.Rat
	unit   map[string]string
}

func (p *Publisher) sunSpecPlan(obs SunSpecObservation) (plan, error) {
	pl := plan{profile: sunspecProfile, protocol: sunspecProtocol, nativeKind: sunspecNative, registryKind: sunspecRegistry,
		profileID: sunspecProfile, receipt: obs.Receipt, identity: -1}
	segments := map[uint32]*segment{}
	for _, cfg := range p.cfg.SunSpec {
		pack, err := p.positioned(&pl, cfg.Pack)
		if err != nil {
			return plan{}, err
		}
		if pl.identity < 0 {
			pl.identity = pack
		}
		pl.service(pack, svcPack)
		segments[cfg.Position] = &segment{pack: pack, ratio: map[string]*big.Rat{}, unit: map[string]string{}}
	}
	if pl.identity < 0 {
		return plan{}, fmt.Errorf("storage: no SunSpec storage segment is configured")
	}
	// The reference points of the charge and discharge limits come from the segment's own models.
	for _, d := range obs.Decisions {
		seg, ok := p.segmentOf(d, segments)
		if !ok || !d.Admitted() || d.ProfileID() != sunspec.StorageMonitoringCapabilityID {
			continue
		}
		if occ, has := d.SourceOccurrence(); has && occ.ModelID() == 124 {
			seg.has124 = true
		}
		for _, f := range d.Facts() {
			if n, numeric := f.Value().Number(); numeric {
				if r, err := ratOf(n); err == nil {
					seg.ratio[f.FieldID()], seg.unit[f.FieldID()] = r, f.Unit()
				}
			}
		}
	}
	for _, d := range obs.Decisions {
		seg, ok := p.segmentOf(d, segments)
		if !ok || !d.Admitted() {
			continue
		}
		occ, has := d.SourceOccurrence()
		if !has || !sunSpecAdmitted(d.ProfileID(), occ.ModelID()) {
			continue
		}
		if err := p.sunSpecDecision(&pl, seg, d, occ); err != nil {
			return plan{}, err
		}
	}
	for _, seg := range segments {
		p.sunSpecLimits(&pl, seg)
	}
	return pl, nil
}

// sunSpecAdmitted is the storage segment's own monitoring profile, plus the
// model 160 module monitor and the model 714 port monitor. Model 701 shares
// the port monitor's profile and is not a storage port.
func sunSpecAdmitted(profile string, model uint16) bool {
	switch profile {
	case sunspec.StorageMonitoringCapabilityID:
		return true
	case sunspec.MPPTMonitoringCapabilityID:
		return model == 160
	case sunspec.DERMeasurementCapabilityID:
		return model == 714
	default:
		return false
	}
}

func (p *Publisher) segmentOf(d sunspec.CapabilityDecision, segments map[uint32]*segment) (*segment, bool) {
	common, ok := d.CommonOccurrence()
	if !ok {
		return nil, false
	}
	seg, ok := segments[common.Ordinal]
	return seg, ok
}

// positioned declares a pack under the configured battery at its native
// position, with the battery and system the configuration names.
func (p *Publisher) positioned(pl *plan, slot string) (int, error) {
	if slot == "" {
		return 0, fmt.Errorf("storage: a configured pack slot is required")
	}
	parent := -1
	if p.cfg.Battery != "" {
		parent = pl.node(nodeDecl{kind: kindBattery, slot: p.cfg.Battery, parent: -1, operator: true})
	}
	pack := pl.node(nodeDecl{kind: kindPack, slot: slot, parent: parent})
	if parent >= 0 {
		pl.link(relBatteryPack, parent, pack)
		if p.cfg.System != "" {
			system := pl.node(nodeDecl{kind: kindSystem, slot: p.cfg.System, parent: -1, operator: true})
			pl.link(relSystemBattery, system, parent)
		}
	}
	return pack, nil
}

func (p *Publisher) sunSpecDecision(pl *plan, seg *segment, d sunspec.CapabilityDecision, occ sunspec.Occurrence) error {
	model := occ.ModelID()
	prefix := fmt.Sprintf("model_%d.occurrence_%d.", model, occ.Ordinal)
	for _, f := range d.Facts() {
		if err := p.sunSpecFact(pl, seg, model, prefix, f); err != nil {
			return err
		}
	}
	lifetime := false
	for _, e := range d.RepeatedEntries() {
		group := fmt.Sprintf("%sgroup_%d.", prefix, e.Index)
		for _, f := range e.Facts {
			if err := p.sunSpecMember(pl, seg.pack, group, f); err != nil {
				return err
			}
			if wattHoursUnassigned(model, f) {
				lifetime = true
			}
		}
		pl.members = append(pl.members, memberDecl{seg.pack, group + "raw_span", Native{Source: "raw_span", Text: rawSpan(occ)}})
	}
	if lifetime {
		reason := reasonModuleRoleUnstated
		if model == 714 {
			reason = reasonPortTypeUnnamed
		}
		p.withholdEnergy(pl, seg.pack, reason)
	}
	return nil
}

// wattHoursUnassigned is a lifetime or per-port watt-hour the model states
// without saying whether it is storage charge or storage discharge.
func wattHoursUnassigned(model uint16, f sunspec.CapabilityFact) bool {
	if f.Unit() != "Wh" {
		return false
	}
	if _, ok := f.Value().Number(); !ok {
		return false
	}
	switch model {
	case 160:
		return f.FieldID() == "mppt.module.dc_energy"
	case 714:
		return f.FieldID() == "der.dc.port.energy.injected" || f.FieldID() == "der.dc.port.energy.absorbed"
	default:
		return false
	}
}

// withholdEnergy records both pack energy elements as unknown. A later fact
// for the same element replaces the unknown; a second observation of the same
// gap does not add another record.
func (p *Publisher) withholdEnergy(pl *plan, node int, reason string) {
	for _, element := range []semreg.DefinitionID{"storage.energy.charged", "storage.energy.discharged"} {
		seen := false
		for _, u := range pl.unknown {
			if u.node == node && u.element == element && u.dim == dimPack {
				seen = true
				break
			}
		}
		if !seen {
			pl.unknown = append(pl.unknown, unknownDecl{node, element, dimPack, reason})
		}
	}
}

func rawSpan(o sunspec.Occurrence) string {
	var b strings.Builder
	for i, s := range o.SourceSpans() {
		if i > 0 {
			b.WriteByte(';')
		}
		fmt.Fprintf(&b, "view=%d,pdu_offset=%d,words=%d", s.LogicalViewID, s.PDUOffset, s.WordCount)
	}
	return b.String()
}

func (p *Publisher) sunSpecMember(pl *plan, node int, prefix string, f sunspec.CapabilityFact) error {
	n := Native{Source: f.FieldID(), Unit: f.Unit()}
	if num, ok := f.Value().Number(); ok {
		r, err := ratOf(num)
		if err != nil {
			return err
		}
		n.Value = decimalOf(r, 6)
	} else if _, sym, ok := f.Value().Enum(); ok {
		n.Text = sym
	} else if text, ok := f.Value().Text(); ok {
		n.Text = text
	} else {
		n.Text = fmt.Sprintf("raw_words=%v", f.SourceValue().RawWords())
	}
	pl.members = append(pl.members, memberDecl{node, prefix + f.FieldID(), n})
	return nil
}

func (p *Publisher) sunSpecFact(pl *plan, seg *segment, model uint16, prefix string, f sunspec.CapabilityFact) error {
	if err := p.sunSpecMember(pl, seg.pack, prefix, f); err != nil {
		return err
	}
	if model == 124 && f.FieldID() == "storage.status.charge_state" {
		if _, symbolName, ok := f.Value().Enum(); ok {
			if token, mapped := chargeStates[symbolName]; mapped {
				pl.facts = append(pl.facts, factDecl{seg.pack, elementOperating, dimPack, symbol(elementOperating, token),
					Native{Source: "ChaSt", Unit: "none", Text: symbolName}, lossSymbolCoarse})
			}
		}
		return nil
	}
	for _, row := range sunSpecRows {
		if row.Model != model || row.Field != f.FieldID() || row.NativeUnit != f.Unit() {
			continue
		}
		num, ok := f.Value().Number()
		if !ok {
			return nil
		}
		r, err := ratOf(num)
		if err != nil {
			return err
		}
		return pl.fact(seg.pack, row.Row, r, "")
	}
	return nil
}

// sunSpecLimits publishes the charge and discharge limits of the segment's DC
// interface: the scaled percent of the rate times its own reference. The
// charge limit is InWRte of WChaMax; the discharge limit is OutWRte of the
// maximum discharge rate where the device reports one, else of WChaMax. A
// negative rate or a missing reference states no limit.
func (p *Publisher) sunSpecLimits(pl *plan, seg *segment) {
	if !seg.has124 {
		return
	}
	iface := pl.node(nodeDecl{kind: kindInterface, slot: "dc", parent: seg.pack})
	pl.link(relPackInterface, seg.pack, iface)
	pl.service(iface, svcInterface)
	maxCharge := seg.ratio["storage.control.max_charge_power"]
	if maxCharge == nil || seg.unit["storage.control.max_charge_power"] != "W" {
		return
	}
	maxDischarge, source := seg.ratio["storage.rating.max_discharge_power"], "MaxDisChaRte"
	if maxDischarge == nil || seg.unit["storage.rating.max_discharge_power"] != "W" {
		maxDischarge, source = maxCharge, "WChaMax"
	}
	for _, l := range []struct {
		rate, element, reference string
		max                      *big.Rat
	}{
		{"storage.control.charge_rate", "storage.limit.charge_power", "WChaMax", maxCharge},
		{"storage.control.discharge_rate", "storage.limit.discharge_power", source, maxDischarge},
	} {
		rate := seg.ratio[l.rate]
		if rate == nil || rate.Sign() < 0 {
			continue
		}
		value := new(big.Rat).Mul(rate, l.max)
		value.Quo(value, big.NewRat(100, 1))
		pl.facts = append(pl.facts, factDecl{iface, semreg.DefinitionID(l.element), dimInterface, quantity(decimalOf(value, 6), unitWatt),
			Native{Source: l.rate + " of " + l.reference, Unit: "percent of W", Value: decimalOf(rate, 6), Text: strconv.Quote(l.reference)}, ""})
	}
}
