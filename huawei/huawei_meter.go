package huawei

import (
	"encoding/binary"
	"slices"
	"strconv"
)

// MeterPolarity describes a power field's native sign convention. It is not
// the meter's electrical role or evidence of a measured direction of flow.
type MeterPolarity string

// Open and documented native meter sign conventions.
const (
	MeterPolarityUnknown        MeterPolarity = "UNKNOWN"
	MeterPolarityExportPositive MeterPolarity = "EXPORT_POSITIVE"
)

// ProviderMeterFact is a typed native meter field. Value is exact and is valid
// only when Availability is FactValue. Non-power fields have no Polarity.
type ProviderMeterFact struct {
	Key          string
	Offset       uint16
	Type         string
	Unit         string
	Value        Decimal
	Availability FactAvailability
	Grade        EvidenceGrade
	Polarity     MeterPolarity
}

// ProviderMeterGroup carries native words, facts and their source epoch for one
// meter read. A documented refusal has no words and is never sent on the wire.
type ProviderMeterGroup struct {
	UnitID uint8
	ProviderGroupEvidence
	RefusalGrade EvidenceGrade `json:",omitempty"`
	Words        []uint16
	Facts        []ProviderMeterFact
}

func (g ProviderMeterGroup) clone() ProviderMeterGroup {
	g.Words = slices.Clone(g.Words)
	g.Facts = slices.Clone(g.Facts)
	return g
}

// MeterFactsAdmitted reports whether this group decoded any new meter facts.
// Retained values preserve their original epoch and are not new admissions.
func (g ProviderMeterGroup) MeterFactsAdmitted() bool {
	return g.Outcome == SUN2000GroupAnswered && !g.Retained && slices.ContainsFunc(g.Facts, func(fact ProviderMeterFact) bool {
		return fact.Availability == FactValue
	})
}

// MeterFactsAdmitted reports whether this cycle decoded any new meter facts.
func (c ProviderCycle) MeterFactsAdmitted() bool {
	return slices.ContainsFunc(c.Meter, ProviderMeterGroup.MeterFactsAdmitted)
}

type huaweiMeterField struct {
	key      string
	offset   uint16
	typ      string
	unit     string
	exponent int16
}

type huaweiMeterExtent struct {
	offset, quantity uint16
	fields           []huaweiMeterField
}

const huaweiMeterClassLabel = "PowerMeter"

func isLoggerMeter(entry GatewayInventoryChild) bool {
	class := entry.Attribute("product_type")
	return entry.Attribute("model") == huaweiMeterClassLabel && (class == "" || class == huaweiMeterClassLabel)
}

var huaweiLoggerMeter = [...]huaweiMeterExtent{
	{32260, 33, []huaweiMeterField{
		{"phase_a_voltage", 32260, "U32", "V", -2}, {"phase_b_voltage", 32262, "U32", "V", -2}, {"phase_c_voltage", 32264, "U32", "V", -2},
		{"phase_a_current", 32272, "I32", "A", -1}, {"phase_b_current", 32274, "I32", "A", -1}, {"phase_c_current", 32276, "I32", "A", -1},
		{"active_power", 32278, "I32", "kW", -3}, {"power_factor", 32284, "I16", "", -3},
	}},
	{32335, 30, []huaweiMeterField{
		{"phase_a_active_power", 32335, "I32", "kW", -3}, {"phase_b_active_power", 32337, "I32", "kW", -3}, {"phase_c_active_power", 32339, "I32", "kW", -3},
		{"total_active_energy", 32341, "I64", "kWh", -2}, {"negative_active_energy", 32349, "I64", "kWh", -2}, {"positive_active_energy", 32357, "I64", "kWh", -2},
	}},
}

var huaweiEMMAMeter = [...]huaweiMeterExtent{
	{31639, 20, []huaweiMeterField{
		{"phase_a_voltage", 31639, "U32", "V", -2}, {"phase_b_voltage", 31641, "U32", "V", -2}, {"phase_c_voltage", 31643, "U32", "V", -2},
		{"phase_a_current", 31651, "I32", "A", -1}, {"phase_b_current", 31653, "I32", "A", -1}, {"phase_c_current", 31655, "I32", "A", -1},
		{"active_power", 31657, "I32", "kW", -3},
	}},
	{31661, 10, []huaweiMeterField{
		{"power_factor", 31661, "I32", "", -3},
		{"phase_a_active_power", 31665, "I32", "kW", -3}, {"phase_b_active_power", 31667, "I32", "kW", -3}, {"phase_c_active_power", 31669, "I32", "kW", -3},
	}},
	{31671, 4, []huaweiMeterField{{"total_active_energy", 31671, "I64", "kWh", -2}}},
	{31679, 4, []huaweiMeterField{{"negative_active_energy", 31679, "I64", "kWh", -2}}},
	{31687, 4, []huaweiMeterField{{"positive_active_energy", 31687, "I64", "kWh", -2}}},
}

var huaweiSDongleMeter = [...]huaweiMeterExtent{
	{37100, 1, nil},
	{37101, 24, []huaweiMeterField{
		{"phase_a_voltage", 37101, "I32", "V", -1}, {"phase_b_voltage", 37103, "I32", "V", -1}, {"phase_c_voltage", 37105, "I32", "V", -1},
		{"phase_a_current", 37107, "I32", "A", -2}, {"phase_b_current", 37109, "I32", "A", -2}, {"phase_c_current", 37111, "I32", "A", -2},
		{"active_power", 37113, "I32", "W", 0}, {"power_factor", 37117, "I16", "", -3},
		{"export_active_energy", 37119, "I32", "kWh", -2}, {"import_active_energy", 37121, "I32", "kWh", -2},
	}},
	{37125, 1, nil},
	{37126, 13, []huaweiMeterField{
		{"phase_a_active_power", 37132, "I32", "W", 0}, {"phase_b_active_power", 37134, "I32", "W", 0}, {"phase_c_active_power", 37136, "I32", "W", 0},
	}},
}

// EMMA uses the longest of the three meter plans.
type huaweiMeterState [len(huaweiEMMAMeter)]huaweiPlantState

func (e huaweiMeterExtent) observation(unit uint8, kept huaweiPlantState, outcome SUN2000GroupOutcome, code uint8, polarity MeterPolarity) ProviderMeterGroup {
	group := ProviderMeterGroup{UnitID: unit, ProviderGroupEvidence: kept.evidence(e.offset, e.quantity, outcome, code), Words: kept.words}
	for _, field := range e.fields {
		fact := ProviderMeterFact{Key: field.key, Offset: field.offset, Type: field.typ, Unit: field.unit, Availability: FactUnknown, Grade: GradeUnknown}
		if field.unit == "kW" || field.unit == "W" {
			fact.Polarity = polarity
		}
		if len(kept.words) == int(e.quantity) {
			if value, ok := field.decode(kept.words[field.offset-e.offset:]); ok {
				fact.Value, fact.Availability, fact.Grade = value, FactValue, GradeHypothesis
			}
		}
		group.Facts = append(group.Facts, fact)
	}
	return group
}

func (f huaweiMeterField) decode(words []uint16) (Decimal, bool) {
	var raw int64
	switch f.typ {
	case "I16":
		raw = int64(int16(words[0]))
	case "I32":
		raw = int64(int32(uint32(words[0])<<16 | uint32(words[1])))
	case "U32":
		raw = int64(uint32(words[0])<<16 | uint32(words[1]))
	case "I64":
		var buf [8]byte
		for i, word := range words[:4] {
			binary.BigEndian.PutUint16(buf[i*2:], word)
		}
		raw = int64(binary.BigEndian.Uint64(buf[:])) //nolint:gosec // Interpret the stored two's-complement bits.
	default:
		return Decimal{}, false
	}
	return Decimal{Coefficient: raw, Exponent: f.exponent}, true
}

func huaweiLoggerMeterPolarity(build string) MeterPolarity {
	version, ok := parseHuaweiGatewayVersion(build)
	if ok && version.line == 300 && version.branch == 10 {
		switch version.release {
		case 24, 25, 26:
			return MeterPolarityExportPositive
		}
	}
	return MeterPolarityUnknown
}

// A changed listing guard or gateway build cannot refresh retained meter words.
// The refusals are separate evidence and do not acquire a new read epoch.
func (r *huaweiCycleRun) fenceChangedLoggerMeterRoute(guard uint16, build string) {
	s := r.state
	if (len(s.guard) == 0 || s.guard[0] == guard) && (s.gateway.Firmware == "" || s.gateway.Firmware == build) {
		return
	}
	for _, child := range s.children {
		for i, group := range child.meter {
			child.meter[i] = huaweiPlantState{refused: group.refused}
		}
	}
}

func (r *huaweiCycleRun) recordLoggerMeterRefusals(unit uint8) {
	build := r.state.gateway.Firmware
	grade := GradeHypothesis
	if build == huaweiSmartLoggerObservedFirmware && (unit == 11 || unit == 19) {
		grade = GradeProven
	}
	for _, extent := range [][2]uint16{{32293, 6}, {33000, 7}} {
		group := ProviderMeterGroup{UnitID: unit, RefusalGrade: grade, ProviderGroupEvidence: ProviderGroupEvidence{
			Offset: extent[0], Quantity: extent[1], Outcome: SUN2000GroupNotRead, Refused: true, ExceptionCode: huaweiSUN2000RefusalCode,
		}}
		r.out.Meter = append(r.out.Meter, group)
		r.out.Loss = append(r.out.Loss, ProviderLoss{Kind: ProviderLossRefusedGroup, HasUnit: true, UnitID: unit,
			Offset: group.Offset, Quantity: group.Quantity, Detail: "recorded exception 3 refusal; not sent again"})
	}
}

func (r *huaweiCycleRun) readMeterGroups(unit uint8, plan []huaweiMeterExtent, kept []huaweiPlantState, polarity MeterPolarity, read bool) ProviderReason {
	reason := ProviderReasonRead
	for i, extent := range plan {
		outcome, code := SUN2000GroupNotRead, uint8(0)
		if read {
			outcome, code = r.readGatewayGroup(unit, extent.offset, extent.quantity, &kept[i])
		}
		if r.stopped() {
			return ""
		}
		group := extent.observation(unit, kept[i], outcome, code, polarity)
		r.out.Meter = append(r.out.Meter, group)
		r.recordGatewayGroupLoss(unit, group.ProviderGroupEvidence, "meter group did not answer")
		switch {
		case outcome == SUN2000GroupFailed:
			reason = ProviderReasonChildReadFailed
		case (outcome == SUN2000GroupException || group.Refused) && reason == ProviderReasonRead:
			reason = ProviderReasonChildException
		}
	}
	return reason
}

func (r *huaweiCycleRun) readLoggerMeter(child *huaweiChildState, read bool) ProviderReason {
	reason := r.readMeterGroups(child.unit, huaweiLoggerMeter[:], child.meter[:], huaweiLoggerMeterPolarity(r.state.gateway.Firmware), read)
	if !r.stopped() {
		r.recordLoggerMeterRefusals(child.unit)
	}
	return reason
}

func (r *huaweiCycleRun) monitorLoggerMeter(child *huaweiChildState) {
	if child.identity != nil {
		child.fence()
		child.identity, child.catalog = nil, nil
	}
	if child.skipUntil > r.cycle {
		r.readLoggerMeter(child, false)
		r.observeLoggerMeter(child, ProviderEvidenceStale, ProviderReasonChildBackoff)
		return
	}
	reason := r.readLoggerMeter(child, true)
	if r.stopped() {
		return
	}
	state := ProviderEvidenceCurrent
	if reason != ProviderReasonRead {
		state = ProviderEvidenceStale
	}
	if reason == ProviderReasonChildReadFailed {
		child.failures++
		child.skipUntil = huaweiChildSkipUntil(r.cycle, child.failures)
	} else {
		child.failures, child.skipUntil = 0, 0
	}
	r.observeLoggerMeter(child, state, reason)
}

func (r *huaweiCycleRun) observeLoggerMeter(child *huaweiChildState, state ProviderEvidenceState, reason ProviderReason) {
	r.out.Children = append(r.out.Children, ProviderChildObservation{
		UnitID: child.unit, ListedClass: child.listedClass, Evidence: state, Reason: reason, Fenced: child.fenced.clone(),
	})
}

func (r *huaweiCycleRun) readSDongleMeter(child *huaweiChildState, read bool) ProviderReason {
	return r.readMeterGroups(child.unit, huaweiSDongleMeter[:], child.meter[:], MeterPolarityExportPositive, read)
}

func huaweiMeterRefusalKey(index int) string {
	return "meter|" + strconv.Itoa(index)
}

func (p *huaweiProviderCore) carryMeterRefusals(from *huaweiProviderState) {
	for i, group := range from.emmaMeter {
		p.state.emmaMeter[i].refused = p.state.emmaMeter[i].refused || group.refused
	}
	for unit, child := range from.children {
		for i, group := range child.meter {
			if !group.refused {
				continue
			}
			if kept := p.state.children[unit]; kept != nil {
				kept.meter[i].refused = true
				continue
			}
			if p.state.carriedRefusals == nil {
				p.state.carriedRefusals = map[uint8]map[string]bool{}
			}
			if p.state.carriedRefusals[unit] == nil {
				p.state.carriedRefusals[unit] = map[string]bool{}
			}
			p.state.carriedRefusals[unit][huaweiMeterRefusalKey(i)] = true
		}
	}
}
