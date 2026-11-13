package modbusreg

import (
	"strconv"
	"strings"
)

// SunSpecThreePhaseMonitoringCapabilityID identifies the vendor-neutral
// three-phase inverter monitoring capability that EvaluateThreePhaseMonitoring
// decides.
const SunSpecThreePhaseMonitoringCapabilityID = "sunspec.inverter.three_phase.monitoring@1.0.0"

// SunSpecCapabilityReason is the stable reason code of a
// SunSpecCapabilityDecision.
type SunSpecCapabilityReason string

// Capability reasons. InvalidChain: the snapshot is not a consistent terminal
// chain or does not decode. SourceAbsent and AmbiguousSource: no, or more than
// one, three-phase inverter occurrence. SourceUnsupported: the source is not
// admitted under the registry revision or is not three-phase.
// InvalidRequiredFact: a required fact is missing, repeated, has another unit,
// or is not a valid and fully named value.
const (
	SunSpecCapabilityReasonAdmitted            SunSpecCapabilityReason = "ADMITTED"
	SunSpecCapabilityReasonInvalidChain        SunSpecCapabilityReason = "INVALID_CHAIN"
	SunSpecCapabilityReasonSourceAbsent        SunSpecCapabilityReason = "SOURCE_ABSENT"
	SunSpecCapabilityReasonAmbiguousSource     SunSpecCapabilityReason = "AMBIGUOUS_SOURCE"
	SunSpecCapabilityReasonSourceUnsupported   SunSpecCapabilityReason = "SOURCE_UNSUPPORTED"
	SunSpecCapabilityReasonInvalidRequiredFact SunSpecCapabilityReason = "INVALID_REQUIRED_FACT"
)

// SunSpecCanonicalValueKind selects which SunSpecCanonicalValue accessor holds
// the value.
type SunSpecCanonicalValueKind string

// Canonical value kinds: an exact decimal number, an enum with its symbol, or
// a bitfield with its symbols.
const (
	SunSpecCanonicalNumber   SunSpecCanonicalValueKind = "number"
	SunSpecCanonicalEnum     SunSpecCanonicalValueKind = "enum"
	SunSpecCanonicalBitfield SunSpecCanonicalValueKind = "bitfield"
)

// SunSpecCanonicalValue is a value in transport-neutral form. Only the
// accessor that matches Kind reports true.
type SunSpecCanonicalValue struct {
	kind       SunSpecCanonicalValueKind
	number     string
	enumNumber uint64
	enumSymbol string
	bits       uint64
	bitSymbols []string
}

// Kind returns which accessor holds the value.
func (v SunSpecCanonicalValue) Kind() SunSpecCanonicalValueKind { return v.kind }

// Number returns the value as a plain decimal string, without exponent or
// trailing zeros and with zero as "0", and true for a number.
func (v SunSpecCanonicalValue) Number() (string, bool) {
	return v.number, v.kind == SunSpecCanonicalNumber
}

// Enum returns the enum number, its non-empty symbol and true for an enum.
func (v SunSpecCanonicalValue) Enum() (uint64, string, bool) {
	return v.enumNumber, v.enumSymbol, v.kind == SunSpecCanonicalEnum
}

// Bitfield returns the raw bits, a copy of the set bits' symbols and true for
// a bitfield. An admitted bitfield has no unknown bits.
func (v SunSpecCanonicalValue) Bitfield() (uint64, []string, bool) {
	return v.bits, append([]string(nil), v.bitSymbols...), v.kind == SunSpecCanonicalBitfield
}

// SunSpecCapabilityFact is one required fact of an admitted capability: field
// identifier, capability unit, canonical value and the decoded source value it
// was derived from.
type SunSpecCapabilityFact struct {
	fieldID     string
	unit        string
	value       SunSpecCanonicalValue
	sourceValue SunSpecValue
}

// FieldID returns the stable field identifier.
func (f SunSpecCapabilityFact) FieldID() string { return f.fieldID }

// Unit returns the capability unit, or "none" for enum and bitfield facts.
func (f SunSpecCapabilityFact) Unit() string { return f.unit }

// Value returns a copy of the canonical value.
func (f SunSpecCapabilityFact) Value() SunSpecCanonicalValue {
	return cloneSunSpecCanonicalValue(f.value)
}

// SourceValue returns a copy of the decoded source value, raw words included.
func (f SunSpecCapabilityFact) SourceValue() SunSpecValue {
	return cloneSunSpecValue(f.sourceValue)
}

// SunSpecCapabilityDecision is the immutable result of
// EvaluateThreePhaseMonitoring. A rejected decision carries only its reason;
// an admitted one also carries the source occurrence, the required facts in a
// fixed order and the source views. Admission is not a product or firmware
// qualification.
type SunSpecCapabilityDecision struct {
	admitted bool
	reason   SunSpecCapabilityReason
	source   *SunSpecOccurrence
	facts    []SunSpecCapabilityFact
	views    []LogicalViewSnapshot
}

// Admitted reports whether the capability was admitted.
func (d SunSpecCapabilityDecision) Admitted() bool { return d.admitted }

// Reason returns the admission or rejection reason.
func (d SunSpecCapabilityDecision) Reason() SunSpecCapabilityReason { return d.reason }

// ProfileID returns SunSpecThreePhaseMonitoringCapabilityID, also for a
// rejected decision.
func (d SunSpecCapabilityDecision) ProfileID() string { return SunSpecThreePhaseMonitoringCapabilityID }

// SourceOccurrence returns a copy of the source occurrence and true when
// admitted.
func (d SunSpecCapabilityDecision) SourceOccurrence() (SunSpecOccurrence, bool) {
	if d.source == nil {
		return SunSpecOccurrence{}, false
	}
	return cloneOccurrence(*d.source), true
}

// Facts returns copies of the required facts in the capability's fixed order;
// it is empty when rejected.
func (d SunSpecCapabilityDecision) Facts() []SunSpecCapabilityFact {
	return cloneSunSpecCapabilityFacts(d.facts)
}

// SourceViews returns copies of the snapshot's source views; it is empty when
// rejected.
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

// EvaluateThreePhaseMonitoring decides the three-phase monitoring capability
// from a completed chain snapshot. The snapshot must be a consistent terminal
// chain that DecodeChain accepts, with a qualifying Common Model and valid
// geometry throughout; exactly one three-phase inverter occurrence must be
// present and admitted under this registry's revision; and every required fact
// must occur once, with its catalog unit and a valid, fully named value. Any
// failure yields a rejected decision with its reason. It never returns an
// error and never reads from a device.
func (r SunSpecDecoderRegistry) EvaluateThreePhaseMonitoring(snapshot SunSpecChainSnapshot) SunSpecCapabilityDecision {
	rejected := func(reason SunSpecCapabilityReason) SunSpecCapabilityDecision {
		return SunSpecCapabilityDecision{reason: reason}
	}
	if _, ok := sunSpecSnapshotWireChain(snapshot); !ok {
		return rejected(SunSpecCapabilityReasonInvalidChain)
	}
	exactSources := make([]SunSpecOccurrence, 0, 2)
	for _, occurrence := range snapshot.Occurrences() {
		if !validRetainedSunSpecGeometry(occurrence) {
			return rejected(SunSpecCapabilityReasonInvalidChain)
		}
		if occurrence.WireKey == (SunSpecWireKey{ModelID: 103, ModelLength: 50}) || occurrence.WireKey == (SunSpecWireKey{ModelID: 113, ModelLength: 60}) {
			exactSources = append(exactSources, occurrence)
		}
	}
	decoded, err := r.DecodeChain(snapshot)
	if err != nil {
		return rejected(SunSpecCapabilityReasonInvalidChain)
	}
	for _, candidate := range decoded.Models() {
		if !candidate.GeometryValid() || (candidate.Key().ModelID == 1 && !candidate.Qualifies()) {
			return rejected(SunSpecCapabilityReasonInvalidChain)
		}
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

func validRetainedSunSpecGeometry(occurrence SunSpecOccurrence) bool {
	if occurrence.ModelID() != 160 {
		return true
	}
	words := occurrence.Words()
	if len(words) != int(occurrence.ModelLength())+2 || len(words) <= 8 || words[8] == 0xffff {
		return false
	}
	return uint32(occurrence.ModelLength()) == 8+20*uint32(words[8])
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
