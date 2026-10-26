package modbusreg

import (
	"errors"
	"strconv"
	"strings"
)

// The two SUN2000 monitoring profiles of the docs-modbus Huawei read-only
// monitoring contract: the M3 series behind the observed SmartLogger and the M1
// series behind the observed S-Dongle. Each profile serves one family behind
// one observed gateway identity and is routed by the catalogue row of that
// family (see SelectHuaweiSUN2000MonitoringProfile); its read plan and decoder
// follow the consulted edition the contract checked. The MAP0 series behind
// the EMMA has a catalogue row and no profile.
const (
	HuaweiSUN2000M3MonitoringProfileID = "huawei.sun2000.m3.monitoring-readonly@1.0.0"
	HuaweiSUN2000M1MonitoringProfileID = "huawei.sun2000.m1.monitoring-readonly@1.0.0"

	huaweiSUN2000M3Family = "SUN2000 M3 series"
	huaweiSUN2000M1Family = "SUN2000 M1 series"

	huaweiSUN2000M3Model = "SUN2000-50KTL-M3"

	huaweiSUN2000ModelOffset   = 30000
	huaweiSUN2000ModelQuantity = 15
	huaweiSUN2000MaxReadWords  = 125
	huaweiSUN2000StringCount   = 8
	huaweiSUN2000MaxUnit       = 247
	huaweiSUN2000RefusalCode   = 3
)

// HuaweiVendorDocumentIdentity is the identity of one consulted vendor
// document edition: the title, the version and the date, each as the document
// states it. It carries no excerpt, link, locator or digest.
type HuaweiVendorDocumentIdentity struct {
	Title   string
	Version string
	Date    string
}

// huaweiSUN2000MCEdition is the one consulted edition that binds the SUN2000
// monitoring groups. Its supported-model table lists the M3 series model; the
// monitoring contract checked its layout against the M1 series child reading.
var huaweiSUN2000MCEdition = HuaweiVendorDocumentIdentity{
	Title:   "SUN2000MC V200R023C00 Modbus Interface Definitions",
	Version: "Issue 01",
	Date:    "2023-03-17",
}

// HuaweiEditionBinding states how the consulted edition binds a family.
type HuaweiEditionBinding string

const (
	// HuaweiEditionBindingSupportedModelTable: the edition's supported-model
	// table names the routed model string.
	HuaweiEditionBindingSupportedModelTable HuaweiEditionBinding = "EDITION_SUPPORTED_MODEL_TABLE"
	// HuaweiEditionBindingLiveCheckedLayout: the edition does not list the
	// model; the monitoring contract checked the edition's layout against an
	// independent reading of the observed unit.
	HuaweiEditionBindingLiveCheckedLayout HuaweiEditionBinding = "EDITION_LAYOUT_LIVE_CHECKED"
)

// HuaweiEvidenceGrade is the grade of a fact. It is separate from admission,
// qualification and support.
type HuaweiEvidenceGrade string

const (
	HuaweiGradeProven     HuaweiEvidenceGrade = "PROVEN"
	HuaweiGradeHypothesis HuaweiEvidenceGrade = "HYPOTHESIS"
	HuaweiGradeUnknown    HuaweiEvidenceGrade = "UNKNOWN"
)

// HuaweiSUN2000RefusedGroup is a group that the family answered with an
// exception. It is never part of the family's read plan.
type HuaweiSUN2000RefusedGroup struct {
	Offset        uint16
	Quantity      uint16
	ExceptionCode uint8
	// Cause is the grade of any statement about why the group is refused.
	Cause HuaweiEvidenceGrade
}

type huaweiSUN2000Group struct {
	offset, quantity uint16
}

var (
	huaweiSUN2000StatusGroup  = huaweiSUN2000Group{offset: 32000, quantity: 16}
	huaweiSUN2000StringsGroup = huaweiSUN2000Group{offset: 32016, quantity: 24}
	huaweiSUN2000PowerGroup   = huaweiSUN2000Group{offset: 32064, quantity: 32}
	huaweiSUN2000EnergyGroup  = huaweiSUN2000Group{offset: 32106, quantity: 10}
)

type huaweiSUN2000WordType uint8

const (
	huaweiSUN2000I16 huaweiSUN2000WordType = iota + 1
	huaweiSUN2000U16
	huaweiSUN2000I32
	huaweiSUN2000U32
)

type huaweiSUN2000FieldKind uint8

const (
	huaweiSUN2000Measurement huaweiSUN2000FieldKind = iota + 1
	// huaweiSUN2000PositiveOnly decodes only a value of zero or more; a negative
	// value is outside the observed range and stays Unknown.
	huaweiSUN2000PositiveOnly
	// huaweiSUN2000StateCode publishes only the observed code.
	huaweiSUN2000StateCode
)

type huaweiSUN2000Field struct {
	name     string
	index    int
	group    huaweiSUN2000Group
	offset   uint16
	wordType huaweiSUN2000WordType
	exponent int16
	unit     string
	kind     huaweiSUN2000FieldKind
	// grade applies at the units the contract checked; elsewhere a fact is at
	// most Hypothesis.
	grade HuaweiEvidenceGrade
}

// huaweiSUN2000ObservedState is the one device status code observed on the
// M1 series child, with the neutral name of the state that the consulted
// edition assigns to it. Every other code is Unknown and withheld.
const (
	huaweiSUN2000ObservedStateCode = 0x0200
	huaweiSUN2000ObservedStateName = "grid_connected"
)

// HuaweiSUN2000MonitoringProfile is an inert, exact monitoring profile for
// one SUN2000 family behind one observed gateway identity. It sends nothing and
// grants no authority.
type HuaweiSUN2000MonitoringProfile struct {
	id            string
	family        string
	gatewayClass  string
	identityCard  string
	model         string
	document      HuaweiVendorDocumentIdentity
	binding       HuaweiEditionBinding
	groups        []huaweiSUN2000Group
	refused       []HuaweiSUN2000RefusedGroup
	observedUnits []uint8
	fields        []huaweiSUN2000Field
}

func (p HuaweiSUN2000MonitoringProfile) ID() string           { return p.id }
func (p HuaweiSUN2000MonitoringProfile) Family() string       { return p.family }
func (p HuaweiSUN2000MonitoringProfile) GatewayClass() string { return p.gatewayClass }

// IdentityCard is the observed gateway qualification card whose exact tuple
// the profile requires.
func (p HuaweiSUN2000MonitoringProfile) IdentityCard() string { return p.identityCard }
func (p HuaweiSUN2000MonitoringProfile) Model() string        { return p.model }
func (p HuaweiSUN2000MonitoringProfile) Document() HuaweiVendorDocumentIdentity {
	return p.document
}
func (p HuaweiSUN2000MonitoringProfile) EditionBinding() HuaweiEditionBinding { return p.binding }
func (p HuaweiSUN2000MonitoringProfile) ObservedUnits() []uint8 {
	return append([]uint8(nil), p.observedUnits...)
}
func (p HuaweiSUN2000MonitoringProfile) RefusedGroups() []HuaweiSUN2000RefusedGroup {
	return append([]HuaweiSUN2000RefusedGroup(nil), p.refused...)
}

// DecoderKeys lists the keys of the fields this profile decodes, once each.
func (p HuaweiSUN2000MonitoringProfile) DecoderKeys() []string {
	var keys []string
	seen := make(map[string]bool, len(p.fields))
	for _, field := range p.fields {
		key := p.decoderKey(field)
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	return keys
}

func (p HuaweiSUN2000MonitoringProfile) decoderKey(field huaweiSUN2000Field) string {
	switch p.id {
	case HuaweiSUN2000M3MonitoringProfileID:
		return "huawei.sun2000.m3." + field.name
	case HuaweiSUN2000M1MonitoringProfileID:
		return "huawei.sun2000.m1." + field.name
	}
	return ""
}

func (HuaweiSUN2000MonitoringProfile) CatalogRegistered() bool         { return false }
func (HuaweiSUN2000MonitoringProfile) AutomaticRuntimeAdmission() bool { return false }
func (HuaweiSUN2000MonitoringProfile) LiveQualified() bool             { return false }
func (HuaweiSUN2000MonitoringProfile) SupportClaim() bool              { return false }
func (HuaweiSUN2000MonitoringProfile) WriteAuthority() bool            { return false }

// BatteryFactsAdmitted is false: the battery groups are present on the MAP0
// series and answer zero words elsewhere, and no battery fact is admitted.
func (HuaweiSUN2000MonitoringProfile) BatteryFactsAdmitted() bool { return false }

// MeterFactsAdmitted is false: no meter register was read on any path.
func (HuaweiSUN2000MonitoringProfile) MeterFactsAdmitted() bool { return false }

// HuaweiSUN2000MonitoringProfiles returns the two exact profiles.
func HuaweiSUN2000MonitoringProfiles() []HuaweiSUN2000MonitoringProfile {
	return []HuaweiSUN2000MonitoringProfile{huaweiSUN2000M3Profile(), huaweiSUN2000M1Profile()}
}

func huaweiSUN2000StringFields(voltage, current HuaweiEvidenceGrade) []huaweiSUN2000Field {
	fields := make([]huaweiSUN2000Field, 0, 2*huaweiSUN2000StringCount)
	for n := 1; n <= huaweiSUN2000StringCount; n++ {
		offset := huaweiSUN2000StringsGroup.offset + uint16(2*(n-1))
		fields = append(fields,
			huaweiSUN2000Field{name: "pv_string_voltage", index: n, group: huaweiSUN2000StringsGroup, offset: offset,
				wordType: huaweiSUN2000I16, exponent: -1, unit: "V", kind: huaweiSUN2000Measurement, grade: voltage},
			huaweiSUN2000Field{name: "pv_string_current", index: n, group: huaweiSUN2000StringsGroup, offset: offset + 1,
				wordType: huaweiSUN2000I16, exponent: -2, unit: "A", kind: huaweiSUN2000Measurement, grade: current},
		)
	}
	return fields
}

func huaweiSUN2000EnergyFields(accumulated, counters HuaweiEvidenceGrade) []huaweiSUN2000Field {
	group := huaweiSUN2000EnergyGroup
	return []huaweiSUN2000Field{
		{name: "energy_accumulated", group: group, offset: 32106, wordType: huaweiSUN2000U32, exponent: -2, unit: "kWh",
			kind: huaweiSUN2000Measurement, grade: accumulated},
		{name: "energy_current_hour", group: group, offset: 32112, wordType: huaweiSUN2000U32, exponent: -2, unit: "kWh",
			kind: huaweiSUN2000Measurement, grade: counters},
		{name: "energy_current_day", group: group, offset: 32114, wordType: huaweiSUN2000U32, exponent: -2, unit: "kWh",
			kind: huaweiSUN2000Measurement, grade: counters},
	}
}

func huaweiSUN2000M3Profile() HuaweiSUN2000MonitoringProfile {
	fields := huaweiSUN2000StringFields(HuaweiGradeProven, HuaweiGradeHypothesis)
	fields = append(fields, huaweiSUN2000EnergyFields(HuaweiGradeProven, HuaweiGradeHypothesis)...)
	return HuaweiSUN2000MonitoringProfile{
		id:           HuaweiSUN2000M3MonitoringProfileID,
		family:       huaweiSUN2000M3Family,
		gatewayClass: HuaweiSmartLoggerCanonicalClass,
		identityCard: HuaweiSmartLoggerObservedQualificationCardID,
		model:        huaweiSUN2000M3Model,
		document:     huaweiSUN2000MCEdition,
		binding:      HuaweiEditionBindingSupportedModelTable,
		groups:       []huaweiSUN2000Group{huaweiSUN2000StatusGroup, huaweiSUN2000StringsGroup, huaweiSUN2000EnergyGroup},
		refused: []HuaweiSUN2000RefusedGroup{
			{Offset: 32064, Quantity: 32, ExceptionCode: huaweiSUN2000RefusalCode, Cause: HuaweiGradeUnknown},
			{Offset: 37100, Quantity: 38, ExceptionCode: huaweiSUN2000RefusalCode, Cause: HuaweiGradeHypothesis},
		},
		observedUnits: []uint8{1, 2},
		fields:        fields,
	}
}

func huaweiSUN2000M1Profile() HuaweiSUN2000MonitoringProfile {
	power := huaweiSUN2000PowerGroup
	fields := huaweiSUN2000StringFields(HuaweiGradeProven, HuaweiGradeProven)
	fields = append(fields,
		huaweiSUN2000Field{name: "input_power", group: power, offset: 32064, wordType: huaweiSUN2000I32, exponent: -3, unit: "kW",
			kind: huaweiSUN2000Measurement, grade: HuaweiGradeProven},
		huaweiSUN2000Field{name: "active_power", group: power, offset: 32080, wordType: huaweiSUN2000I32, exponent: -3, unit: "kW",
			kind: huaweiSUN2000PositiveOnly, grade: HuaweiGradeProven},
		huaweiSUN2000Field{name: "device_status", group: power, offset: 32089, wordType: huaweiSUN2000U16,
			kind: huaweiSUN2000StateCode, grade: HuaweiGradeHypothesis},
	)
	fields = append(fields, huaweiSUN2000EnergyFields(HuaweiGradeHypothesis, HuaweiGradeHypothesis)...)
	return HuaweiSUN2000MonitoringProfile{
		id:            HuaweiSUN2000M1MonitoringProfileID,
		family:        huaweiSUN2000M1Family,
		gatewayClass:  HuaweiSDongleCanonicalClass,
		identityCard:  HuaweiSDongleObservedQualificationCardID,
		model:         huaweiSDongleObservedChildModel,
		document:      huaweiSUN2000MCEdition,
		binding:       HuaweiEditionBindingLiveCheckedLayout,
		groups:        []huaweiSUN2000Group{huaweiSUN2000StatusGroup, huaweiSUN2000StringsGroup, huaweiSUN2000PowerGroup, huaweiSUN2000EnergyGroup},
		observedUnits: []uint8{huaweiSDongleObservedChildUnit},
		fields:        fields,
	}
}

// HuaweiSUN2000ReadStep is one bounded FC03 read of a monitoring group.
type HuaweiSUN2000ReadStep struct {
	Function uint8
	UnitID   uint8
	Offset   uint16
	Quantity uint16
}

// Plan returns the bounded read plan at one unit, in the order of the
// monitoring contract. A refused group is never part of it.
func (p HuaweiSUN2000MonitoringProfile) Plan(unit uint8) []HuaweiSUN2000ReadStep {
	steps := make([]HuaweiSUN2000ReadStep, 0, len(p.groups))
	for _, group := range p.groups {
		steps = append(steps, HuaweiSUN2000ReadStep{Function: uint8(FunctionReadHoldingRegisters), UnitID: unit, Offset: group.offset, Quantity: group.quantity})
	}
	return steps
}

// HuaweiSUN2000GroupOutcome is the outcome of one group read.
type HuaweiSUN2000GroupOutcome string

const (
	HuaweiSUN2000GroupAnswered  HuaweiSUN2000GroupOutcome = "ANSWERED"
	HuaweiSUN2000GroupException HuaweiSUN2000GroupOutcome = "EXCEPTION"
	HuaweiSUN2000GroupFailed    HuaweiSUN2000GroupOutcome = "FAILED"
	HuaweiSUN2000GroupNotRead   HuaweiSUN2000GroupOutcome = "NOT_READ"
)

// HuaweiSUN2000GroupReading is the native evidence of one group: every word
// of an answered group is retained, including the words that are not facts.
type HuaweiSUN2000GroupReading struct {
	Offset        uint16
	Quantity      uint16
	Outcome       HuaweiSUN2000GroupOutcome
	ExceptionCode uint8
	Words         []uint16
	// Cycle is the caller's cycle number of the read that carried the words.
	Cycle uint64
	// Retained is true when the words are a last complete reading kept after
	// a later read of the group failed.
	Retained bool
}

func (reading HuaweiSUN2000GroupReading) clone() HuaweiSUN2000GroupReading {
	reading.Words = append([]uint16(nil), reading.Words...)
	return reading
}

// HuaweiDecimal is an exact decimal: Coefficient times ten to the Exponent.
type HuaweiDecimal struct {
	Coefficient int64
	Exponent    int16
}

// String renders the exact decimal with as many fractional digits as the
// exponent states.
func (d HuaweiDecimal) String() string {
	if d.Exponent >= 0 {
		return strconv.FormatInt(d.Coefficient, 10) + strings.Repeat("0", int(d.Exponent))
	}
	negative := d.Coefficient < 0
	magnitude := d.Coefficient
	if negative {
		magnitude = -magnitude
	}
	digits := strconv.FormatInt(magnitude, 10)
	places := int(-d.Exponent)
	if len(digits) <= places {
		digits = strings.Repeat("0", places-len(digits)+1) + digits
	}
	text := digits[:len(digits)-places] + "." + digits[len(digits)-places:]
	if negative {
		text = "-" + text
	}
	return text
}

// HuaweiFactAvailability states whether a fact carries a value.
type HuaweiFactAvailability string

const (
	HuaweiFactValue   HuaweiFactAvailability = "VALUE"
	HuaweiFactUnknown HuaweiFactAvailability = "UNKNOWN"
)

// HuaweiSUN2000Fact is one typed monitoring fact. An Unknown fact carries no
// value, never a zero.
type HuaweiSUN2000Fact struct {
	Key          string
	Index        int
	Unit         string
	Availability HuaweiFactAvailability
	Value        HuaweiDecimal
	// State is the neutral name of an observed state code.
	State string
	Grade HuaweiEvidenceGrade
	Cycle uint64
	// Retained is true when the fact comes from a retained group reading.
	Retained bool
}

// HuaweiSUN2000MonitoringReading is the decoded monitoring state of one unit.
type HuaweiSUN2000MonitoringReading struct {
	profileID string
	unitID    uint8
	groups    []HuaweiSUN2000GroupReading
	facts     []HuaweiSUN2000Fact
}

func (r HuaweiSUN2000MonitoringReading) ProfileID() string { return r.profileID }
func (r HuaweiSUN2000MonitoringReading) UnitID() uint8     { return r.unitID }
func (r HuaweiSUN2000MonitoringReading) Groups() []HuaweiSUN2000GroupReading {
	out := make([]HuaweiSUN2000GroupReading, len(r.groups))
	for index, group := range r.groups {
		out[index] = group.clone()
	}
	return out
}
func (r HuaweiSUN2000MonitoringReading) Facts() []HuaweiSUN2000Fact {
	return append([]HuaweiSUN2000Fact(nil), r.facts...)
}

// Fact returns the fact with the decoder key and string index (0 for a
// field that is not per string).
func (r HuaweiSUN2000MonitoringReading) Fact(key string, index int) (HuaweiSUN2000Fact, bool) {
	for _, fact := range r.facts {
		if fact.Key == key && fact.Index == index {
			return fact, true
		}
	}
	return HuaweiSUN2000Fact{}, false
}

func (HuaweiSUN2000MonitoringReading) BatteryFactsAdmitted() bool { return false }
func (HuaweiSUN2000MonitoringReading) MeterFactsAdmitted() bool   { return false }
func (HuaweiSUN2000MonitoringReading) SupportClaim() bool         { return false }
func (HuaweiSUN2000MonitoringReading) WriteAuthority() bool       { return false }

var errHuaweiSUN2000ReadingInvalid = errors.New("invalid SUN2000 monitoring reading")

// Decode validates the group readings of one unit against the read plan and
// decodes the consumed fields. A group outside the plan, a repeated group, or
// words that do not fill the group are refused as a whole. A group that did
// not answer contributes no fact: its fields stay Unknown.
func (p HuaweiSUN2000MonitoringProfile) Decode(unit uint8, readings []HuaweiSUN2000GroupReading) (HuaweiSUN2000MonitoringReading, error) {
	if p.id == "" || unit == 0 || unit > huaweiSUN2000MaxUnit {
		return HuaweiSUN2000MonitoringReading{}, errHuaweiSUN2000ReadingInvalid
	}
	groups := make([]HuaweiSUN2000GroupReading, len(p.groups))
	for index, group := range p.groups {
		groups[index] = HuaweiSUN2000GroupReading{Offset: group.offset, Quantity: group.quantity, Outcome: HuaweiSUN2000GroupNotRead}
	}
	seen := make([]bool, len(p.groups))
	for _, reading := range readings {
		position := -1
		for index, group := range p.groups {
			if reading.Offset == group.offset && reading.Quantity == group.quantity {
				position = index
			}
		}
		if position < 0 || seen[position] || !validHuaweiSUN2000GroupReading(reading) {
			return HuaweiSUN2000MonitoringReading{}, errHuaweiSUN2000ReadingInvalid
		}
		seen[position] = true
		groups[position] = reading.clone()
	}
	return HuaweiSUN2000MonitoringReading{profileID: p.id, unitID: unit, groups: groups, facts: p.decodeFacts(unit, groups)}, nil
}

func validHuaweiSUN2000GroupReading(reading HuaweiSUN2000GroupReading) bool {
	switch reading.Outcome {
	case HuaweiSUN2000GroupAnswered:
		return len(reading.Words) == int(reading.Quantity) && reading.ExceptionCode == 0
	case HuaweiSUN2000GroupException:
		return len(reading.Words) == 0 && reading.ExceptionCode != 0 && !reading.Retained
	case HuaweiSUN2000GroupFailed, HuaweiSUN2000GroupNotRead:
		return len(reading.Words) == 0 && reading.ExceptionCode == 0 && !reading.Retained
	}
	return false
}

func (p HuaweiSUN2000MonitoringProfile) observedUnit(unit uint8) bool {
	for _, observed := range p.observedUnits {
		if observed == unit {
			return true
		}
	}
	return false
}

func (p HuaweiSUN2000MonitoringProfile) decodeFacts(unit uint8, groups []HuaweiSUN2000GroupReading) []HuaweiSUN2000Fact {
	facts := make([]HuaweiSUN2000Fact, 0, len(p.fields))
	for _, field := range p.fields {
		fact := HuaweiSUN2000Fact{Key: p.decoderKey(field), Index: field.index, Unit: field.unit, Availability: HuaweiFactUnknown, Grade: HuaweiGradeUnknown}
		for _, group := range groups {
			if group.Offset != field.group.offset || group.Outcome != HuaweiSUN2000GroupAnswered {
				continue
			}
			value, state, ok := decodeHuaweiSUN2000Field(field, group.Words[field.offset-group.Offset:])
			if !ok {
				break
			}
			fact.Availability, fact.Value, fact.State = HuaweiFactValue, value, state
			fact.Grade = field.grade
			if !p.observedUnit(unit) && fact.Grade == HuaweiGradeProven {
				fact.Grade = HuaweiGradeHypothesis
			}
			fact.Cycle, fact.Retained = group.Cycle, group.Retained
		}
		facts = append(facts, fact)
	}
	return facts
}

func decodeHuaweiSUN2000Field(field huaweiSUN2000Field, words []uint16) (HuaweiDecimal, string, bool) {
	var raw int64
	switch field.wordType {
	case huaweiSUN2000I16:
		raw = int64(int16(words[0]))
	case huaweiSUN2000U16:
		raw = int64(words[0])
	case huaweiSUN2000I32:
		raw = int64(int32(uint32(words[0])<<16 | uint32(words[1])))
	case huaweiSUN2000U32:
		raw = int64(uint32(words[0])<<16 | uint32(words[1]))
	default:
		return HuaweiDecimal{}, "", false
	}
	switch field.kind {
	case huaweiSUN2000Measurement:
		return HuaweiDecimal{Coefficient: raw, Exponent: field.exponent}, "", true
	case huaweiSUN2000PositiveOnly:
		if raw < 0 {
			return HuaweiDecimal{}, "", false
		}
		return HuaweiDecimal{Coefficient: raw, Exponent: field.exponent}, "", true
	case huaweiSUN2000StateCode:
		if raw != huaweiSUN2000ObservedStateCode {
			return HuaweiDecimal{}, "", false
		}
		return HuaweiDecimal{Coefficient: raw}, huaweiSUN2000ObservedStateName, true
	}
	return HuaweiDecimal{}, "", false
}

// RetainHuaweiSUN2000Monitoring keeps, for each group that did not answer in
// the current reading, the last complete reading of that group from the
// previous state, with its original cycle. A reading of another profile or
// unit is fenced: nothing migrates from it.
func RetainHuaweiSUN2000Monitoring(previous, current HuaweiSUN2000MonitoringReading) HuaweiSUN2000MonitoringReading {
	if previous.profileID != current.profileID || previous.unitID != current.unitID || current.profileID == "" {
		return current
	}
	var profile HuaweiSUN2000MonitoringProfile
	for _, candidate := range HuaweiSUN2000MonitoringProfiles() {
		if candidate.id == current.profileID {
			profile = candidate
		}
	}
	if profile.id == "" || len(previous.groups) != len(current.groups) {
		return current
	}
	groups := current.Groups()
	for index := range groups {
		before := previous.groups[index]
		if groups[index].Outcome != HuaweiSUN2000GroupAnswered && before.Outcome == HuaweiSUN2000GroupAnswered &&
			before.Offset == groups[index].Offset && before.Quantity == groups[index].Quantity {
			kept := before.clone()
			kept.Retained = true
			groups[index] = kept
		}
	}
	return HuaweiSUN2000MonitoringReading{profileID: current.profileID, unitID: current.unitID, groups: groups, facts: profile.decodeFacts(current.unitID, groups)}
}
