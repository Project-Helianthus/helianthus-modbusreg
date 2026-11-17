package sunspec

import (
	"strconv"
	"strings"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// ThreePhaseMonitoringCapabilityID identifies the vendor-neutral
// three-phase inverter monitoring capability that EvaluateThreePhaseMonitoring
// decides.
const ThreePhaseMonitoringCapabilityID = "sunspec.inverter.three_phase.monitoring@1.0.0"

// CapabilityReason is the stable reason code of a
// CapabilityDecision.
type CapabilityReason string

// Capability reasons. InvalidChain: the snapshot is not a consistent terminal
// chain or does not decode. SourceAbsent and AmbiguousSource: no, or more than
// one, three-phase inverter occurrence. SourceUnsupported: the source is not
// admitted under the registry revision or is not three-phase.
// InvalidRequiredFact: a required fact is missing, repeated, has another unit,
// or is not a valid and fully named value.
const (
	CapabilityReasonAdmitted            CapabilityReason = "ADMITTED"
	CapabilityReasonInvalidChain        CapabilityReason = "INVALID_CHAIN"
	CapabilityReasonSourceAbsent        CapabilityReason = "SOURCE_ABSENT"
	CapabilityReasonAmbiguousSource     CapabilityReason = "AMBIGUOUS_SOURCE"
	CapabilityReasonSourceUnsupported   CapabilityReason = "SOURCE_UNSUPPORTED"
	CapabilityReasonInvalidRequiredFact CapabilityReason = "INVALID_REQUIRED_FACT"
)

// CanonicalValueKind selects which CanonicalValue accessor holds
// the value.
type CanonicalValueKind string

// Canonical value kinds: an exact decimal number, an enum with its symbol, or
// a bitfield with its symbols.
const (
	CanonicalNumber   CanonicalValueKind = "number"
	CanonicalEnum     CanonicalValueKind = "enum"
	CanonicalBitfield CanonicalValueKind = "bitfield"
)

// CanonicalValue is a value in transport-neutral form. Only the
// accessor that matches Kind reports true.
type CanonicalValue struct {
	kind       CanonicalValueKind
	number     string
	enumNumber uint64
	enumSymbol string
	bits       uint64
	bitSymbols []string
}

// Kind returns which accessor holds the value.
func (v CanonicalValue) Kind() CanonicalValueKind { return v.kind }

// Number returns the value as a plain decimal string, without exponent or
// trailing zeros and with zero as "0", and true for a number.
func (v CanonicalValue) Number() (string, bool) {
	return v.number, v.kind == CanonicalNumber
}

// Enum returns the enum number, its non-empty symbol and true for an enum.
func (v CanonicalValue) Enum() (uint64, string, bool) {
	return v.enumNumber, v.enumSymbol, v.kind == CanonicalEnum
}

// Bitfield returns the raw bits, a copy of the set bits' symbols and true for
// a bitfield. An admitted bitfield has no unknown bits.
func (v CanonicalValue) Bitfield() (uint64, []string, bool) {
	return v.bits, append([]string(nil), v.bitSymbols...), v.kind == CanonicalBitfield
}

// CapabilityFact is one admitted fact of an admitted capability: field
// identifier, capability unit, canonical value and the decoded source value it
// was derived from.
type CapabilityFact struct {
	fieldID     string
	unit        string
	value       CanonicalValue
	sourceValue Value
}

// FieldID returns the stable field identifier.
func (f CapabilityFact) FieldID() string { return f.fieldID }

// Unit returns the capability unit, or "none" for enum and bitfield facts.
func (f CapabilityFact) Unit() string { return f.unit }

// Value returns a copy of the canonical value.
func (f CapabilityFact) Value() CanonicalValue {
	return cloneSunSpecCanonicalValue(f.value)
}

// SourceValue returns a copy of the decoded source value, raw words included.
func (f CapabilityFact) SourceValue() Value {
	return cloneSunSpecValue(f.sourceValue)
}

// CapabilityDecision is the immutable result of
// EvaluateThreePhaseMonitoring. A rejected decision carries only its reason;
// an admitted one also carries the source occurrence, the admitted facts in a
// fixed order and the source views. Admission is not a product or firmware
// qualification.
type CapabilityDecision struct {
	admitted bool
	reason   CapabilityReason
	source   *Occurrence
	facts    []CapabilityFact
	views    []modbusreg.LogicalViewSnapshot
}

// Admitted reports whether the capability was admitted.
func (d CapabilityDecision) Admitted() bool { return d.admitted }

// Reason returns the admission or rejection reason.
func (d CapabilityDecision) Reason() CapabilityReason { return d.reason }

// ProfileID returns ThreePhaseMonitoringCapabilityID, also for a
// rejected decision.
func (d CapabilityDecision) ProfileID() string { return ThreePhaseMonitoringCapabilityID }

// SourceOccurrence returns a copy of the source occurrence and true when
// admitted.
func (d CapabilityDecision) SourceOccurrence() (Occurrence, bool) {
	if d.source == nil {
		return Occurrence{}, false
	}
	return cloneOccurrence(*d.source), true
}

// Facts returns copies of the admitted facts in the capability's fixed order;
// it is empty when rejected. Required facts come first. Optional apparent
// power, reactive power, and line-to-line voltages follow when they are
// admitted.
func (d CapabilityDecision) Facts() []CapabilityFact {
	return cloneSunSpecCapabilityFacts(d.facts)
}

// SourceViews returns copies of the snapshot's source views; it is empty when
// rejected.
func (d CapabilityDecision) SourceViews() []modbusreg.LogicalViewSnapshot {
	return CloneSourceViews(d.views)
}

// Clone returns a copy of the decision that shares no evidence with it: the
// source occurrence, the facts and the source views are copied. A rejected
// decision's clone carries empty, non-nil facts and source views.
func (d CapabilityDecision) Clone() CapabilityDecision {
	out := d
	if d.source != nil {
		source := cloneOccurrence(*d.source)
		out.source = &source
	}
	out.facts = cloneSunSpecCapabilityFacts(d.facts)
	out.views = CloneSourceViews(d.views)
	return out
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

var threePhaseMonitoringOptionalFields = []sunSpecCapabilityField{
	{"inverter.ac.power.apparent", "VA", "VA"},
	{"inverter.ac.power.reactive", "var", "var"},
	{"inverter.ac.voltage.line_ab", "V", "V"},
	{"inverter.ac.voltage.line_bc", "V", "V"},
	{"inverter.ac.voltage.line_ca", "V", "V"},
}

// ThreePhaseMonitoringFieldIDs returns a new slice of the field
// identifiers a ThreePhaseMonitoringCapabilityID decision requires, in
// their fixed order. Admitted facts start with this list.
func ThreePhaseMonitoringFieldIDs() []string {
	return sunSpecCapabilityFieldIDs(threePhaseMonitoringFields)
}

// ThreePhaseMonitoringOptionalFieldIDs returns a new slice of the optional
// apparent-power, reactive-power, and line-to-line voltage identifiers, in
// the order an admitted decision appends the points that qualify.
func ThreePhaseMonitoringOptionalFieldIDs() []string {
	return sunSpecCapabilityFieldIDs(threePhaseMonitoringOptionalFields)
}

func sunSpecCapabilityFieldIDs(fields []sunSpecCapabilityField) []string {
	ids := make([]string, len(fields))
	for index, field := range fields {
		ids[index] = field.id
	}
	return ids
}

// EvaluateThreePhaseMonitoring decides the three-phase monitoring capability
// from a completed chain snapshot. The snapshot must be a consistent terminal
// chain that DecodeChain accepts, with a qualifying Common Model and valid
// geometry throughout; exactly one three-phase inverter occurrence must be
// present and admitted under this registry's revision; and every required fact
// must occur once, with its catalog unit and a valid, fully named value. An
// apparent-power, reactive-power, or line-to-line voltage point is appended
// after those required facts, in that order, only when it occurs once with its
// catalog unit and a valid value. An absent or unimplemented point is omitted:
// it does not reject the capability and it is not reported as zero. Any
// failure of a required check yields a rejected decision with its reason. It
// never returns an error and never reads from a device.
func (r DecoderRegistry) EvaluateThreePhaseMonitoring(snapshot ChainSnapshot) CapabilityDecision {
	rejected := func(reason CapabilityReason) CapabilityDecision {
		return CapabilityDecision{reason: reason}
	}
	if _, ok := snapshot.WireChain(); !ok {
		return rejected(CapabilityReasonInvalidChain)
	}
	exactSources := make([]Occurrence, 0, 2)
	for _, occurrence := range snapshot.Occurrences() {
		if !validRetainedSunSpecGeometry(occurrence) {
			return rejected(CapabilityReasonInvalidChain)
		}
		if occurrence.WireKey == (WireKey{ModelID: 103, ModelLength: 50}) || occurrence.WireKey == (WireKey{ModelID: 113, ModelLength: 60}) {
			exactSources = append(exactSources, occurrence)
		}
	}
	decoded, err := r.DecodeChain(snapshot)
	if err != nil {
		return rejected(CapabilityReasonInvalidChain)
	}
	for _, candidate := range decoded.Models() {
		if !candidate.GeometryValid() || (candidate.Key().ModelID == 1 && !candidate.Qualifies()) {
			return rejected(CapabilityReasonInvalidChain)
		}
	}
	if len(exactSources) > 1 {
		return rejected(CapabilityReasonAmbiguousSource)
	}
	if len(exactSources) == 0 {
		return rejected(CapabilityReasonSourceAbsent)
	}
	exactSource := exactSources[0]
	key, admitted := exactSource.DecoderKey()
	if !admitted || exactSource.Disposition != ChainDispositionAdmitted || key.SchemaRevision != r.revision {
		return rejected(CapabilityReasonSourceUnsupported)
	}
	var model DecodedModel
	found := false
	for _, candidate := range decoded.Models() {
		if candidate.Ordinal() == exactSource.Ordinal && candidate.Key() == key {
			model, found = candidate, true
			break
		}
	}
	if !found || model.Topology() != TopologyThreePhase {
		return rejected(CapabilityReasonSourceUnsupported)
	}
	factsByID := make(map[string][]Fact, len(model.Facts()))
	for _, fact := range model.Facts() {
		factsByID[fact.FieldID] = append(factsByID[fact.FieldID], fact)
	}
	canonical := make([]CapabilityFact, 0, len(threePhaseMonitoringFields)+len(threePhaseMonitoringOptionalFields))
	for _, required := range threePhaseMonitoringFields {
		matches := factsByID[required.id]
		if len(matches) != 1 || matches[0].Unit != required.sourceUnit || !matches[0].Required {
			return rejected(CapabilityReasonInvalidRequiredFact)
		}
		value, ok := canonicalSunSpecValue(matches[0].Value)
		if !ok {
			return rejected(CapabilityReasonInvalidRequiredFact)
		}
		canonical = append(canonical, CapabilityFact{
			fieldID: required.id, unit: required.unit, value: value,
			sourceValue: cloneSunSpecValue(matches[0].Value),
		})
	}
	for _, optional := range threePhaseMonitoringOptionalFields {
		matches := factsByID[optional.id]
		if len(matches) != 1 || matches[0].Unit != optional.sourceUnit {
			continue
		}
		value, ok := canonicalSunSpecValue(matches[0].Value)
		if !ok {
			continue
		}
		canonical = append(canonical, CapabilityFact{
			fieldID: optional.id, unit: optional.unit, value: value,
			sourceValue: cloneSunSpecValue(matches[0].Value),
		})
	}
	var source *Occurrence
	for _, occurrence := range snapshot.Occurrences() {
		if occurrence.Ordinal == model.Ordinal() && occurrence.WireKey == (WireKey{ModelID: model.Key().ModelID, ModelLength: model.Key().ModelLength}) {
			copy := cloneOccurrence(occurrence)
			source = &copy
			break
		}
	}
	if source == nil {
		return rejected(CapabilityReasonInvalidChain)
	}
	return CapabilityDecision{
		admitted: true,
		reason:   CapabilityReasonAdmitted,
		source:   source,
		facts:    canonical,
		views:    snapshot.SourceViews(),
	}
}

func validRetainedSunSpecGeometry(occurrence Occurrence) bool {
	if occurrence.ModelID() != 160 {
		return true
	}
	words := occurrence.Words()
	if len(words) != int(occurrence.ModelLength())+2 || len(words) <= 8 || words[8] == 0xffff {
		return false
	}
	return uint32(occurrence.ModelLength()) == 8+20*uint32(words[8])
}

func canonicalSunSpecValue(value Value) (CanonicalValue, bool) {
	if value.State() != ValueValid {
		return CanonicalValue{}, false
	}
	if decimal, ok := value.Decimal(); ok {
		return CanonicalValue{kind: CanonicalNumber, number: formatSunSpecDecimal(decimal)}, true
	}
	if number, ok := value.Float32(); ok {
		formatted := strconv.FormatFloat(float64(number), 'f', -1, 32)
		if formatted == "-0" {
			formatted = "0"
		}
		return CanonicalValue{kind: CanonicalNumber, number: formatted}, true
	}
	if number, symbol, ok := value.Enum(); ok {
		if symbol == "" {
			return CanonicalValue{}, false
		}
		return CanonicalValue{kind: CanonicalEnum, enumNumber: number, enumSymbol: symbol}, true
	}
	if bits, unknown, ok := value.Bitfield(); ok {
		if unknown != 0 {
			return CanonicalValue{}, false
		}
		symbols := value.BitfieldSymbols()
		return CanonicalValue{kind: CanonicalBitfield, bits: bits, bitSymbols: symbols}, true
	}
	return CanonicalValue{}, false
}

func formatSunSpecDecimal(decimal Decimal) string {
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

func cloneSunSpecCanonicalValue(value CanonicalValue) CanonicalValue {
	value.bitSymbols = append([]string(nil), value.bitSymbols...)
	return value
}

func cloneSunSpecCapabilityFacts(facts []CapabilityFact) []CapabilityFact {
	out := make([]CapabilityFact, len(facts))
	for index, fact := range facts {
		fact.value = cloneSunSpecCanonicalValue(fact.value)
		fact.sourceValue = cloneSunSpecValue(fact.sourceValue)
		out[index] = fact
	}
	return out
}

func cloneSunSpecValue(value Value) Value {
	value.raw = append([]uint16(nil), value.raw...)
	value.bitSymbols = append([]string(nil), value.bitSymbols...)
	return value
}

// CloneSourceViews returns copies of source views that share no
// storage with them; the result is empty and non-nil for no views. A copy is
// valid only when its view is: an invalid view, such as the zero value, stays
// invalid.
func CloneSourceViews(views []modbusreg.LogicalViewSnapshot) []modbusreg.LogicalViewSnapshot {
	out := make([]modbusreg.LogicalViewSnapshot, len(views))
	for index, view := range views {
		out[index] = view.Clone()
	}
	return out
}
