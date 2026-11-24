package huawei

import (
	"sort"
	"strconv"
	"strings"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

const huaweiBatteryFieldKey = "storage."

type huaweiBatteryPlan struct {
	prefix string
	groups []catalogGroup
	fields []catalogField
	states map[uint16]map[uint16]string
}

func huaweiBatteryOffset(offset int) bool {
	return offset >= 37000 && offset <= 37068 || offset >= 37741 && offset <= 37786 ||
		offset >= 37920 && offset <= 37925 || offset >= 38200 && offset <= 38463
}

// Numeric and native-only words come from the selected card. Packing only
// touches contiguous documented words; text, including identity, is not read.
func huaweiBuildBatteryPlan(file catalogRegisterFile, prefix string) huaweiBatteryPlan {
	plan := huaweiBatteryPlan{prefix: prefix, states: huaweiBatteryLabels(file.Card)}
	extents := map[uint16]catalogField{}
	for _, row := range file.Rows {
		if row.Offset == nil || !huaweiBatteryOffset(*row.Offset) || row.Endpoint != "inverter" || row.Access != "RO" {
			continue
		}
		if row.Class != "conditional" && row.Class != "reviewed" && row.Class != "raw-only" {
			continue
		}
		width, signed, ok := catalogWordShape(row.Type)
		if !ok || width != row.Quantity {
			continue
		}
		offset := uint16(*row.Offset)
		field := catalogField{offset: offset, width: uint16(width), signed: signed}
		extents[offset] = field
		if row.Decode == nil || row.Class == "raw-only" || (offset == 37006 && len(plan.states[offset]) == 0) {
			continue
		}
		field.exponent, ok = catalogScale(row.Decode.Gain)
		if ok {
			field.name = huaweiBatteryFieldKey + strconv.Itoa(*row.Offset)
			field.unit = catalogPhysicalUnit(row.Decode.Unit)
			plan.fields = append(plan.fields, field)
		}
	}
	words := make([]catalogField, 0, len(extents))
	for _, field := range extents {
		if field.offset < 38452 {
			words = append(words, field)
		}
	}
	plan.groups = catalogPack(words)
	// A temperature refusal for one pack must not hide the other packs’ rows.
	for offset := uint16(38452); offset <= 38463; offset += 2 {
		words = words[:0]
		for _, address := range []uint16{offset, offset + 1} {
			if field, ok := extents[address]; ok {
				words = append(words, field)
			}
		}
		plan.groups = append(plan.groups, catalogPack(words)...)
	}
	sort.Slice(plan.fields, func(i, j int) bool { return plan.fields[i].offset < plan.fields[j].offset })
	return plan
}

func huaweiBatteryLabels(card string) map[uint16]map[uint16]string {
	idle := map[uint16]string{0: "offline", 1: "idle", 2: "running", 3: "fault", 4: "sleep"}
	switch card {
	case "L-V200", "LB-C00", "LB-C10", "LC", "MA-C00":
		mode := map[uint16]string{0: "inactive_or_not_applicable", 7: "remote_self_consumption"}
		if card != "LB-C10" {
			mode[3] = "fixed_charge_discharge"
		}
		return map[uint16]map[uint16]string{37000: idle, 37006: mode, 37741: idle, 37762: idle}
	case "MA-V200":
		idle[1] = "standby"
		return map[uint16]map[uint16]string{37000: idle, 37741: idle, 37762: idle}
	case "SUN-V300":
		return map[uint16]map[uint16]string{37000: idle}
	}
	return nil
}

func (p huaweiBatteryPlan) appendSteps(steps []SUN2000ReadStep, unit uint8) []SUN2000ReadStep {
	for _, group := range p.groups {
		steps = append(steps, SUN2000ReadStep{Function: uint8(modbusreg.FunctionReadHoldingRegisters), UnitID: unit,
			Offset: group.offset, Quantity: group.quantity})
	}
	return steps
}

func (p huaweiBatteryPlan) appendFacts(facts []SUN2000Fact, groups []SUN2000GroupReading) []SUN2000Fact {
	for _, field := range p.fields {
		facts = append(facts, p.fact(field, groups))
	}
	return facts
}

func (p huaweiBatteryPlan) fact(field catalogField, groups []SUN2000GroupReading) SUN2000Fact {
	fact := SUN2000Fact{Key: p.prefix + field.name, Unit: field.unit, Availability: FactUnknown, Grade: GradeUnknown}
	group, words, ok := catalogFieldWords(field, groups)
	if !ok {
		return fact
	}
	raw, ok := catalogDecodeWord(field, words)
	// With no native presence rule, a zero field is not a current value.
	// Neither a nonzero field nor any running state changes presence.
	if !ok || raw == 0 {
		return fact
	}
	if labels := p.states[field.offset]; labels != nil {
		fact.State = labels[uint16(raw)]
		if fact.State == "" {
			return fact
		}
	}
	fact.Value = Decimal{Coefficient: raw, Exponent: field.exponent}
	fact.Availability, fact.Grade, fact.Cycle, fact.Retained = FactValue, GradeHypothesis, group.Cycle, group.Retained
	return fact
}

func huaweiBatteryPlanFor(decision *SUN2000CatalogDecision) huaweiBatteryPlan {
	row, selected := decision.Row()
	if !selected {
		return huaweiBatteryPlan{}
	}
	profiles, _ := CatalogMonitoringProfiles()
	for _, profile := range profiles {
		if profile.Card() == row.Card && len(profile.battery.fields) != 0 {
			return profile.battery
		}
	}
	return huaweiBatteryPlan{}
}

func huaweiHasBatteryFacts(facts []SUN2000Fact) bool {
	for _, fact := range facts {
		if fact.Availability == FactValue && strings.Contains(fact.Key, "."+huaweiBatteryFieldKey) {
			return true
		}
	}
	return false
}
