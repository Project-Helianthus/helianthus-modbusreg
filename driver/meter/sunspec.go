package meter

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
)

var sunSpecKinds = buildSunSpecKinds()

func buildSunSpecKinds() map[string]string {
	t := map[string]string{
		"meter.ac.frequency": "frequency", "meter.ac.power.active.total": "power.active.total",
		"meter.ac.power.apparent.total": "power.apparent.total", "meter.ac.power.reactive.total": "power.reactive.total",
		"meter.ac.power_factor.total": "power_factor.total", "meter.energy.import.total": "energy.import.total",
		"meter.energy.export.total": "energy.export.total", "meter.energy.apparent_import.total": "energy.apparent_import.total",
		"meter.energy.apparent_export.total": "energy.apparent_export.total",
	}
	for _, p := range []string{"a", "b", "c"} {
		t["meter.ac.current.phase_"+p] = "current.phase." + p
		t["meter.ac.voltage.line_neutral.phase_"+p] = "voltage.phase." + p
		t["meter.ac.power.active.phase_"+p] = "power.active.phase." + p
		t["meter.ac.power.apparent.phase_"+p] = "power.apparent.phase." + p
		t["meter.ac.power.reactive.phase_"+p] = "power.reactive.phase." + p
		t["meter.ac.power_factor.phase_"+p] = "power_factor.phase." + p
		t["meter.energy.import.phase_"+p] = "energy.import.phase." + p
		t["meter.energy.export.phase_"+p] = "energy.export.phase." + p
	}
	for _, p := range []string{"ab", "bc", "ca"} {
		t["meter.ac.voltage.line_line."+p] = "voltage.pair." + p
	}
	return t
}

// faultBits are the documented meter event conditions, bits 2 to 7 of the
// event word: power failure, under voltage, low power factor, over current,
// over voltage and missing sensor.
const faultBits = 0xfc

func (r *run) sunSpec() error {
	units := append([]SunSpecUnit(nil), r.in.SunSpec...)
	sort.Slice(units, func(i, j int) bool { return units[i].Unit < units[j].Unit })
	for _, u := range units {
		decisions, err := sunspec.EvaluateCapabilities(u.Snapshot)
		if err != nil {
			return fmt.Errorf("unit %d: %w", u.Unit, err)
		}
		for _, d := range decisions {
			if d.ProfileID() != sunspec.MeterMonitoringCapabilityID || !d.Admitted() {
				continue
			}
			if err := r.sunSpecMeter(u.Unit, d); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *run) sunSpecMeter(unit uint8, d sunspec.CapabilityDecision) error {
	source, ok := d.SourceOccurrence()
	if !ok {
		return nil
	}
	slot := fmt.Sprintf("unit-%d.position-%d", unit, source.Ordinal)
	role, gc := Role(""), ""
	for _, c := range r.p.cfg.SunSpec {
		if c.Unit == unit && c.Position == source.Ordinal {
			role, gc = c.Role, c.GridConnection
		}
	}
	f, err := r.meter(slot, role, gc)
	if err != nil {
		return err
	}
	for _, fact := range d.Facts() {
		if err := f.sunSpecFact(fact); err != nil {
			return err
		}
	}
	return nil
}

func (f *feeder) sunSpecFact(fact sunspec.CapabilityFact) error {
	id := fact.FieldID()
	if id == "meter.events" {
		bits, _, ok := fact.Value().Bitfield()
		if !ok {
			return nil
		}
		return f.fault(bits&faultBits != 0, reading{field: id, path: id, value: bitsRat(bits)})
	}
	number, ok := fact.Value().Number()
	if !ok {
		return nil
	}
	value, err := parseNumber(number)
	if err != nil {
		return err
	}
	rd := reading{kind: sunSpecKinds[id], field: id, unit: fact.Unit(), value: value, path: id}
	switch fact.SignConvention() {
	case sunspec.MeterSignTowardLoad:
		rd.polarity = signImportPositive
	case sunspec.MeterSignOpen:
		rd.polarity, rd.open = signOpen, openSign
	}
	if rd.kind == "" && strings.HasPrefix(id, "meter.energy.reactive_") && strings.Contains(id, ".q") {
		rd.open = openNoQuad
	}
	if rd.kind == "power.active.total" && rd.polarity == signNone {
		rd.polarity, rd.open = signOpen, openSign
	}
	return f.feed(rd)
}
