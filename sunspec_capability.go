package modbusreg

import (
	"strconv"
	"strings"
)

const SunSpecThreePhaseMonitoringCapabilityID = "sunspec.inverter.three_phase.monitoring@1.0.0"

type SunSpecCapabilityReason string

const (
	SunSpecCapabilityReasonAdmitted            SunSpecCapabilityReason = "ADMITTED"
	SunSpecCapabilityReasonInvalidChain        SunSpecCapabilityReason = "INVALID_CHAIN"
	SunSpecCapabilityReasonSourceAbsent        SunSpecCapabilityReason = "SOURCE_ABSENT"
	SunSpecCapabilityReasonAmbiguousSource     SunSpecCapabilityReason = "AMBIGUOUS_SOURCE"
	SunSpecCapabilityReasonSourceUnsupported   SunSpecCapabilityReason = "SOURCE_UNSUPPORTED"
	SunSpecCapabilityReasonInvalidRequiredFact SunSpecCapabilityReason = "INVALID_REQUIRED_FACT"
)

type SunSpecCanonicalValueKind string

const (
	SunSpecCanonicalNumber   SunSpecCanonicalValueKind = "number"
	SunSpecCanonicalEnum     SunSpecCanonicalValueKind = "enum"
	SunSpecCanonicalBitfield SunSpecCanonicalValueKind = "bitfield"
)

type SunSpecCanonicalValue struct {
	kind       SunSpecCanonicalValueKind
	number     string
	enumNumber uint64
	enumSymbol string
	bits       uint64
	bitSymbols []string
}

func (v SunSpecCanonicalValue) Kind() SunSpecCanonicalValueKind { return v.kind }
func (v SunSpecCanonicalValue) Number() (string, bool) {
	return v.number, v.kind == SunSpecCanonicalNumber
}
func (v SunSpecCanonicalValue) Enum() (uint64, string, bool) {
	return v.enumNumber, v.enumSymbol, v.kind == SunSpecCanonicalEnum
}
func (v SunSpecCanonicalValue) Bitfield() (uint64, []string, bool) {
	return v.bits, append([]string(nil), v.bitSymbols...), v.kind == SunSpecCanonicalBitfield
}

type SunSpecCapabilityFact struct {
	fieldID     string
	unit        string
	value       SunSpecCanonicalValue
	sourceValue SunSpecValue
}

func (f SunSpecCapabilityFact) FieldID() string { return f.fieldID }
func (f SunSpecCapabilityFact) Unit() string    { return f.unit }
func (f SunSpecCapabilityFact) Value() SunSpecCanonicalValue {
	return cloneSunSpecCanonicalValue(f.value)
}
func (f SunSpecCapabilityFact) SourceValue() SunSpecValue {
	return cloneSunSpecValue(f.sourceValue)
}

type SunSpecCapabilityDecision struct {
	admitted bool
	reason   SunSpecCapabilityReason
	source   *SunSpecOccurrence
	facts    []SunSpecCapabilityFact
	views    []LogicalViewSnapshot
}

func (d SunSpecCapabilityDecision) Admitted() bool                  { return d.admitted }
func (d SunSpecCapabilityDecision) Reason() SunSpecCapabilityReason { return d.reason }
func (d SunSpecCapabilityDecision) ProfileID() string               { return SunSpecThreePhaseMonitoringCapabilityID }
func (d SunSpecCapabilityDecision) SourceOccurrence() (SunSpecOccurrence, bool) {
	if d.source == nil {
		return SunSpecOccurrence{}, false
	}
	return cloneOccurrence(*d.source), true
}
func (d SunSpecCapabilityDecision) Facts() []SunSpecCapabilityFact {
	return cloneSunSpecCapabilityFacts(d.facts)
}
func (d SunSpecCapabilityDecision) SourceViews() []LogicalViewSnapshot {
	return cloneSunSpecLogicalViewSnapshots(d.views)
}

type sunSpecCapabilityField struct{ id, sourceUnit, unit string }

var threePhaseMonitoringFields = []sunSpecCapabilityField{
	{"inverter.ac.current.total", "A", "A"},
	{"inverter.ac.current.phase_a", "A", "A"},
	{"inverter.ac.current.phase_b", "A", "A"},
	{"inverter.ac.current.phase_c", "A", "A"},
	{"inverter.ac.voltage.phase_a", "V", "V"},
	{"inverter.ac.voltage.phase_b", "V", "V"},
	{"inverter.ac.voltage.phase_c", "V", "V"},
	{"inverter.ac.power.active", "W", "W"},
	{"inverter.ac.frequency", "Hz", "Hz"},
	{"inverter.ac.energy_lifetime", "Wh", "Wh"},
	{"inverter.temperature.cabinet", "C", "C"},
	{"inverter.operating_state", "", "none"},
	{"inverter.events.1", "", "none"},
	{"inverter.events.2", "", "none"},
}

func (r SunSpecDecoderRegistry) EvaluateThreePhaseMonitoring(snapshot SunSpecChainSnapshot) SunSpecCapabilityDecision {
	rejected := func(reason SunSpecCapabilityReason) SunSpecCapabilityDecision {
		return SunSpecCapabilityDecision{reason: reason}
	}
	if _, ok := sunSpecSnapshotWireChain(snapshot); !ok {
		return rejected(SunSpecCapabilityReasonInvalidChain)
	}
	exactSources := make([]SunSpecOccurrence, 0, 2)
	for _, occurrence := range snapshot.Occurrences() {
		if occurrence.WireKey == (SunSpecWireKey{ModelID: 103, ModelLength: 50}) || occurrence.WireKey == (SunSpecWireKey{ModelID: 113, ModelLength: 60}) {
			exactSources = append(exactSources, occurrence)
		}
	}
	decoded, err := r.DecodeChain(snapshot)
	if err != nil {
		return rejected(SunSpecCapabilityReasonInvalidChain)
	}
	if len(exactSources) > 1 {
		return rejected(SunSpecCapabilityReasonAmbiguousSource)
	}
	if len(exactSources) == 0 {
		return rejected(SunSpecCapabilityReasonSourceAbsent)
	}
	exactSource := exactSources[0]
	key, admitted := exactSource.DecoderKey()
	if !admitted || exactSource.Disposition != SunSpecChainDispositionAdmitted || key.SchemaRevision != r.revision {
		return rejected(SunSpecCapabilityReasonSourceUnsupported)
	}
	var model SunSpecDecodedModel
	found := false
	for _, candidate := range decoded.Models() {
		if candidate.Ordinal() == exactSource.Ordinal && candidate.Key() == key {
			model, found = candidate, true
			break
		}
	}
	if !found || model.Topology() != SunSpecTopologyThreePhase {
		return rejected(SunSpecCapabilityReasonSourceUnsupported)
	}
	factsByID := make(map[string][]SunSpecFact, len(threePhaseMonitoringFields))
	for _, fact := range model.Facts() {
		factsByID[fact.FieldID] = append(factsByID[fact.FieldID], fact)
	}
	canonical := make([]SunSpecCapabilityFact, 0, len(threePhaseMonitoringFields))
	for _, required := range threePhaseMonitoringFields {
		matches := factsByID[required.id]
		if len(matches) != 1 || matches[0].Unit != required.sourceUnit || !matches[0].Required {
			return rejected(SunSpecCapabilityReasonInvalidRequiredFact)
		}
		value, ok := canonicalSunSpecValue(matches[0].Value)
		if !ok {
			return rejected(SunSpecCapabilityReasonInvalidRequiredFact)
		}
		canonical = append(canonical, SunSpecCapabilityFact{
			fieldID: required.id, unit: required.unit, value: value,
			sourceValue: cloneSunSpecValue(matches[0].Value),
		})
	}
	var source *SunSpecOccurrence
	for _, occurrence := range snapshot.Occurrences() {
		if occurrence.Ordinal == model.Ordinal() && occurrence.WireKey == (SunSpecWireKey{ModelID: model.Key().ModelID, ModelLength: model.Key().ModelLength}) {
			copy := cloneOccurrence(occurrence)
			source = &copy
			break
		}
	}
	if source == nil {
		return rejected(SunSpecCapabilityReasonInvalidChain)
	}
	return SunSpecCapabilityDecision{
		admitted: true,
		reason:   SunSpecCapabilityReasonAdmitted,
		source:   source,
		facts:    canonical,
		views:    snapshot.SourceViews(),
	}
}

func canonicalSunSpecValue(value SunSpecValue) (SunSpecCanonicalValue, bool) {
	if value.State() != SunSpecValueValid {
		return SunSpecCanonicalValue{}, false
	}
	if decimal, ok := value.Decimal(); ok {
		return SunSpecCanonicalValue{kind: SunSpecCanonicalNumber, number: formatSunSpecDecimal(decimal)}, true
	}
	if number, ok := value.Float32(); ok {
		formatted := strconv.FormatFloat(float64(number), 'f', -1, 32)
		if formatted == "-0" {
			formatted = "0"
		}
		return SunSpecCanonicalValue{kind: SunSpecCanonicalNumber, number: formatted}, true
	}
	if number, symbol, ok := value.Enum(); ok {
		if symbol == "" {
			return SunSpecCanonicalValue{}, false
		}
		return SunSpecCanonicalValue{kind: SunSpecCanonicalEnum, enumNumber: number, enumSymbol: symbol}, true
	}
	if bits, unknown, ok := value.Bitfield(); ok {
		if unknown != 0 {
			return SunSpecCanonicalValue{}, false
		}
		symbols := value.BitfieldSymbols()
		return SunSpecCanonicalValue{kind: SunSpecCanonicalBitfield, bits: bits, bitSymbols: symbols}, true
	}
	return SunSpecCanonicalValue{}, false
}

func formatSunSpecDecimal(decimal SunSpecDecimal) string {
	if decimal.Coefficient == 0 {
		return "0"
	}
	negative := decimal.Coefficient < 0
	digits := strconv.FormatInt(decimal.Coefficient, 10)
	if negative {
		digits = digits[1:]
	}
	if decimal.Exponent >= 0 {
		digits += strings.Repeat("0", int(decimal.Exponent))
	} else {
		point := len(digits) + int(decimal.Exponent)
		if point <= 0 {
			digits = "0." + strings.Repeat("0", -point) + digits
		} else {
			digits = digits[:point] + "." + digits[point:]
		}
		digits = strings.TrimRight(strings.TrimRight(digits, "0"), ".")
	}
	if negative {
		return "-" + digits
	}
	return digits
}

func cloneSunSpecCanonicalValue(value SunSpecCanonicalValue) SunSpecCanonicalValue {
	value.bitSymbols = append([]string(nil), value.bitSymbols...)
	return value
}

func cloneSunSpecCapabilityFacts(facts []SunSpecCapabilityFact) []SunSpecCapabilityFact {
	out := make([]SunSpecCapabilityFact, len(facts))
	for index, fact := range facts {
		fact.value = cloneSunSpecCanonicalValue(fact.value)
		fact.sourceValue = cloneSunSpecValue(fact.sourceValue)
		out[index] = fact
	}
	return out
}

func cloneSunSpecValue(value SunSpecValue) SunSpecValue {
	value.raw = append([]uint16(nil), value.raw...)
	value.bitSymbols = append([]string(nil), value.bitSymbols...)
	return value
}

func cloneSunSpecLogicalViewSnapshots(views []LogicalViewSnapshot) []LogicalViewSnapshot {
	out := make([]LogicalViewSnapshot, len(views))
	for index, view := range views {
		record := view.Record()
		out[index] = LogicalViewSnapshot{record: cloneSunSpecLogicalViewRecord(record), valid: true}
	}
	return out
}
