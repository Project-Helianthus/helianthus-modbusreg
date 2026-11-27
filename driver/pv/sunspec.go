package pv

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/Project-Helianthus/helianthus-modbusreg/fronius"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

const (
	reasonNoElement     = "no_element_in_pinned_pack"
	reasonUnit          = "native_unit_not_the_row_unit"
	reasonNotNumeric    = "native_value_not_numeric"
	reasonStateNotTable = "state_not_in_table"
)

func (r *run) registry(rev sunspec.SchemaRevision) (sunspec.DecoderRegistry, error) {
	if reg, ok := r.registries[rev]; ok {
		return reg, nil
	}
	reg, err := sunspec.NewStandardSunSpecDecoderRegistry(rev)
	if err != nil {
		return sunspec.DecoderRegistry{}, err
	}
	r.registries[rev] = reg
	return reg, nil
}

func (r *run) decode(occ sunspec.Occurrence) (sunspec.DecodedModel, error) {
	key, ok := occ.DecoderKey()
	if !ok {
		return sunspec.DecodedModel{}, fmt.Errorf("model %d at %d has no decoder key", occ.ModelID(), occ.Ordinal)
	}
	reg, err := r.registry(key.SchemaRevision)
	if err != nil {
		return sunspec.DecodedModel{}, err
	}
	return reg.DecodeOccurrence(occ)
}

// sunSpec publishes every inverter device of every acquired unit. A chain the
// registry does not admit is NO_GO with its reason and publishes nothing; the
// admitted devices of the same chain still publish.
func (r *run) sunSpec() error {
	units := append([]SunSpecUnit(nil), r.in.SunSpec...)
	sort.Slice(units, func(i, j int) bool { return units[i].Unit < units[j].Unit })
	for _, u := range units {
		if err := r.unit(u); err != nil {
			return fmt.Errorf("unit %d: %w", u.Unit, err)
		}
	}
	return nil
}

func (r *run) level(snapshot sunspec.ChainSnapshot) string {
	reg, err := r.registry(sunspec.ModelsRevisionV1)
	if err != nil {
		return ""
	}
	if decision, ok := fronius.SelectObservedFlavor(reg, snapshot).Decision(); ok {
		return decision.FlavorID()
	}
	return ""
}

func (r *run) unit(u SunSpecUnit) error {
	if err := r.members(u); err != nil {
		return err
	}
	decisions, err := sunspec.EvaluateCapabilities(u.Snapshot)
	if err != nil {
		r.out.Decisions = append(r.out.Decisions, Decision{Unit: u.Unit, Capability: "chain", Reason: "invalid_chain"})
		return nil
	}
	level := r.level(u.Snapshot)
	bySegment := map[uint32][]sunspec.CapabilityDecision{}
	var order []uint32
	for _, d := range decisions {
		common := uint32(0)
		if c, ok := d.CommonOccurrence(); ok {
			common = c.Ordinal
		}
		if _, seen := bySegment[common]; !seen {
			order = append(order, common)
		}
		bySegment[common] = append(bySegment[common], d)
		if isPVProfile(d) {
			r.decision(u.Unit, common, d, level)
		}
	}
	for _, common := range order {
		if common == 0 {
			continue
		}
		if err := r.device(u.Unit, bySegment[common]); err != nil {
			return err
		}
	}
	return nil
}

func isPVProfile(d sunspec.CapabilityDecision) bool {
	switch d.ProfileID() {
	case sunspec.InverterMonitoringCapabilityID, sunspec.MPPTMonitoringCapabilityID, sunspec.DERMeasurementCapabilityID:
		return true
	}
	return false
}

// decision records the capability decision of one source. A block before the
// first Common model has no device to attribute to and says so.
func (r *run) decision(unit uint8, common uint32, d sunspec.CapabilityDecision, level string) {
	rec := Decision{Unit: unit, Common: common, Capability: d.ProfileID(), Go: d.Admitted(), Reason: string(d.Reason()), Level: level}
	if src, ok := d.SourceOccurrence(); ok {
		rec.Source, rec.Model = src.Ordinal, src.ModelID()
	}
	if common == 0 {
		rec.Go, rec.Reason = false, "no_common_model_for_block"
	}
	r.out.Decisions = append(r.out.Decisions, rec)
}

func (r *run) identity(common sunspec.Occurrence) (manufacturer, model, serial string) {
	decoded, err := r.decode(common)
	if err != nil {
		return "", "", ""
	}
	manufacturer, _ = decoded.TextFact("device.manufacturer")
	model, _ = decoded.TextFact("device.model")
	serial, _ = decoded.TextFact("device.serial")
	return manufacturer, model, serial
}

// identitySlot is the operator slot of a device that states a complete Common
// identity: the digest of its manufacturer, model and serial. It is the slot
// the earlier publisher anchored the inverter and its PV system on, so the
// allocator names them as before.
func identitySlot(manufacturer, model, serial string) string {
	sum := sha256.Sum256([]byte("pv-operator-slot\x00" + manufacturer + "\x00" + model + "\x00" + serial))
	return hex.EncodeToString(sum[:])[:32]
}

type device struct {
	r        *run
	unit     uint8
	slot     string
	inverter driver.NodeRef
	system   driver.NodeRef
	phases   map[string]driver.NodeRef
	topology int
}

func factMap(ds []sunspec.CapabilityDecision, id string) map[string]sunspec.CapabilityFact {
	out := map[string]sunspec.CapabilityFact{}
	for _, d := range ds {
		if d.ProfileID() != id || !d.Admitted() {
			continue
		}
		for _, f := range d.Facts() {
			if _, dup := out[f.FieldID()]; !dup {
				out[f.FieldID()] = f
			}
		}
	}
	return out
}

func sourceModel(d sunspec.CapabilityDecision) uint16 {
	src, _ := d.SourceOccurrence()
	return src.ModelID()
}

func (r *run) device(unit uint8, ds []sunspec.CapabilityDecision) error {
	common, _ := ds[0].CommonOccurrence()
	var inverters, ders []sunspec.CapabilityDecision
	pvDevice := false
	for _, d := range ds {
		if !d.Admitted() {
			continue
		}
		switch d.ProfileID() {
		case sunspec.InverterMonitoringCapabilityID:
			inverters, pvDevice = append(inverters, d), true
		case sunspec.DERMeasurementCapabilityID:
			ders = append(ders, d)
			if sourceModel(d) == 701 || hasPVPort(d) {
				pvDevice = true
			}
		}
	}
	if !pvDevice {
		return nil
	}
	mn, md, sn := r.identity(common)
	slot := fmt.Sprintf("unit-%d.position-%d", unit, common.Ordinal)
	if strings.TrimSpace(mn) != "" && strings.TrimSpace(md) != "" && strings.TrimSpace(sn) != "" {
		slot = identitySlot(mn, md, sn)
	}
	inv, err := r.node(kindInverter, slot, nil)
	if err != nil {
		return err
	}
	systemSlot := slot
	if r.p.cfg.System != "" {
		systemSlot = r.p.cfg.System
	}
	system, err := r.node(kindSystem, systemSlot, nil)
	if err != nil {
		return err
	}
	dev := &device{r: r, unit: unit, slot: slot, inverter: inv, system: system, phases: map[string]driver.NodeRef{}}
	r.link(false, pvDef("pv.relation.system_inverter"), system, inv, gateRule, "one_pv_system_groups_each_inverter_device_of_a_common_model")
	if err := r.endpointRelays(inv); err != nil {
		return err
	}
	r.claim(inv, common, strings.TrimSpace(mn), strings.TrimSpace(sn))
	for _, d := range inverters {
		dev.topologyOf(d)
	}
	for _, d := range ders {
		dev.topologyOf(d)
	}
	for i := 1; i <= dev.topology; i++ {
		if _, err := dev.phase(string(rune('a' + i - 1))); err != nil {
			return err
		}
	}
	state := factMap(ds, sunspec.DERStateCapabilityID)
	for _, d := range inverters {
		if err := dev.inverterModel(d, state); err != nil {
			return err
		}
	}
	for _, d := range ders {
		if err := dev.derMeasurement(d); err != nil {
			return err
		}
	}
	for _, d := range ds {
		if d.Admitted() && d.ProfileID() == sunspec.MPPTMonitoringCapabilityID {
			if err := dev.mppt(d); err != nil {
				return err
			}
		}
	}
	return nil
}

// endpointRelays states the relays relation from the endpoint to a device of the
// chain, or leaves it unknown when no endpoint is configured.
func (r *run) endpointRelays(inv driver.NodeRef) error {
	if r.p.cfg.Endpoint == "" {
		r.out.UnknownLinks = append(r.out.UnknownLinks, UnknownLink{Type: relaysRef(), From: inv, Reason: "endpoint_not_configured"})
		return nil
	}
	endpoint, err := r.node(kindEquipment, r.p.cfg.Endpoint, nil)
	if err != nil {
		return err
	}
	r.link(false, relaysRef(), endpoint, inv, nativeObservation, "each_common_model_of_the_chain_is_read_through_its_endpoint")
	return nil
}

func (r *run) claim(inv driver.NodeRef, common sunspec.Occurrence, manufacturer, serial string) {
	switch {
	case manufacturer != "" && serial != "":
		words := make([]string, 0, len(common.Words()))
		for _, w := range common.Words() {
			words = append(words, strconv.Itoa(int(w)))
		}
		r.out.Claims = append(r.out.Claims, driver.ManufacturerSerialClaim{
			Member: inv, Manufacturer: manufacturer, Serial: serial,
			Proof: []semreg.EvidenceRef{evidence("evidence.common_model", words...)},
		})
	case manufacturer == "" && serial == "":
		r.out.Unclaimed = append(r.out.Unclaimed, Unclaimed{Node: inv, Reason: "manufacturer_and_serial_absent"})
	case manufacturer == "":
		r.out.Unclaimed = append(r.out.Unclaimed, Unclaimed{Node: inv, Reason: "manufacturer_absent"})
	default:
		r.out.Unclaimed = append(r.out.Unclaimed, Unclaimed{Node: inv, Reason: "serial_absent"})
	}
}

func (dev *device) topologyOf(d sunspec.CapabilityDecision) {
	n := 0
	switch d.Topology() {
	case sunspec.TopologySinglePhase:
		n = 1
	case sunspec.TopologySplitPhase:
		n = 2
	case sunspec.TopologyThreePhase:
		n = 3
	}
	for _, f := range d.Facts() {
		for i, p := range []string{"_a", "_b", "_c"} {
			if strings.HasSuffix(f.FieldID(), p) && strings.Contains(f.FieldID(), "phase") && i+1 > n {
				n = i + 1
			}
		}
	}
	if n > dev.topology {
		dev.topology = n
	}
}

func hasPVPort(d sunspec.CapabilityDecision) bool {
	for _, e := range d.RepeatedEntries() {
		if portIsPV(e) {
			return true
		}
	}
	return false
}

// portIsPV reads the DC port type of model 714: the value 0 is a PV port. A
// port that does not state its type is not attributed.
func portIsPV(e sunspec.CapabilityEntry) bool {
	for _, f := range e.Facts {
		if f.FieldID() == "der.dc.port.type" {
			n, _, ok := f.SourceValue().Enum()
			return ok && n == 0
		}
	}
	return false
}

func (dev *device) phase(suffix string) (driver.NodeRef, error) {
	if n, ok := dev.phases[suffix]; ok {
		return n, nil
	}
	slot := "L" + string(rune('1'+strings.Index("abc", suffix)))
	n, err := dev.r.node(kindPhase, slot, &dev.inverter)
	if err != nil {
		return driver.NodeRef{}, err
	}
	dev.phases[suffix] = n
	dev.r.link(false, pvDef("pv.relation.inverter_phase"), dev.inverter, n, gateRule, "each_phase_the_inverter_model_reads_is_a_phase_of_the_inverter")
	return n, nil
}

func (dev *device) path(d sunspec.CapabilityDecision, id string) string {
	src, _ := d.SourceOccurrence()
	return fmt.Sprintf("%s.m%d.o%d.%s", dev.slot, src.ModelID(), src.Ordinal, id)
}

// sameUnit compares unit tokens the way the models spell them: V1 states var,
// the DER models state Var.
func sameUnit(a, b string) bool { return strings.EqualFold(a, b) }

func nativeOf(f sunspec.CapabilityFact, v *big.Rat) Native {
	d, _ := decimalOf(v)
	return Native{Source: f.FieldID(), Unit: f.Unit(), Value: d}
}

func numberOf(f sunspec.CapabilityFact) (*big.Rat, bool) {
	text, ok := f.Value().Number()
	if !ok {
		return nil, false
	}
	v, err := parseNumber(text)
	return v, err == nil
}

// target resolves the node and the dimension text of a row for this device.
func (dev *device) target(rw row) (driver.NodeRef, string, error) {
	switch rw.scope {
	case onSystem:
		return dev.system, string(dev.system.ID), nil
	case onPhase:
		n, err := dev.phase(rw.suffix)
		return n, string(n.ID), err
	case onLinePair:
		return dev.inverter, string(dev.inverter.ID) + ".line_" + rw.suffix, nil
	}
	return dev.inverter, string(dev.inverter.ID), nil
}

func (dev *device) feed(d sunspec.CapabilityDecision, f sunspec.CapabilityFact, rw row) error {
	v, ok := numberOf(f)
	path := dev.path(d, f.FieldID())
	if !ok {
		dev.r.withheld(path, reasonNotNumeric)
		return nil
	}
	if !sameUnit(f.Unit(), rw.native) {
		dev.r.withheld(path, reasonUnit)
		return nil
	}
	node, dim, err := dev.target(rw)
	if err != nil {
		return err
	}
	return dev.r.publish(node, rw, dim, v, source{path: path, native: nativeOf(f, v)})
}
