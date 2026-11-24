package huawei

import "strconv"

// EMMASiteFact is one native gateway-self register. The coefficient is Signed
// for I32 and Unsigned otherwise; the physical value is coefficient × 10^Exponent.
// Availability distinguishes an unknown reading from a valid zero.
type EMMASiteFact struct {
	Key             string
	Offset          uint16
	Type            string
	Unit            string
	Exponent        int16
	Unsigned        uint64
	Signed          int32
	Availability    FactAvailability
	Grade           EvidenceGrade
	PolarityUnknown bool
}

// EMMASiteGroup carries the native words and typed facts of one unit 0 read.
// ProviderGroupEvidence is the source validity and epoch of all its facts.
type EMMASiteGroup struct {
	UnitID uint8
	ProviderGroupEvidence
	Words []uint16
	Facts []EMMASiteFact
}

func (g EMMASiteGroup) clone() EMMASiteGroup {
	g.Words = append([]uint16(nil), g.Words...)
	g.Facts = append([]EMMASiteFact(nil), g.Facts...)
	return g
}

// EMMASiteFact finds a named EMMA site reading, including Unknown readings.
// Its origin and freshness are in the enclosing group in EMMASite.
func (c ProviderCycle) EMMASiteFact(key string) (EMMASiteFact, bool) {
	for _, group := range c.EMMASite {
		for _, fact := range group.Facts {
			if fact.Key == key {
				return fact, true
			}
		}
	}
	return EMMASiteFact{}, false
}

type huaweiEMMASiteField struct {
	offset   uint16
	typ      string
	unit     string
	exponent int16
}

type huaweiEMMASiteExtent struct {
	offset, quantity uint16
	fields           []huaweiEMMASiteField
}

var huaweiEMMASite = [...]huaweiEMMASiteExtent{
	{30306, 46, []huaweiEMMASiteField{
		{30306, "U32", "kWh", -2}, {30308, "U64", "kWh", -2}, // battery charge: day and total
		{30312, "U32", "kWh", -2}, {30314, "U64", "kWh", -2}, // battery discharge: day and total
		{30322, "U32", "kWh", -3},                            // rated storage capacity
		{30324, "U32", "kWh", -2}, {30326, "U64", "kWh", -2}, // household consumption: day and total
		{30330, "U32", "kWh", -2}, {30332, "U64", "kWh", -2}, // grid export: day and total
		{30336, "U32", "kWh", -2}, {30338, "U64", "kWh", -2}, // grid import: day and total
		{30346, "U32", "kWh", -2}, {30348, "U64", "kWh", -2}, // PV energy: day and total
	}},
	{30354, 12, []huaweiEMMASiteField{
		{30354, "U32", "kW", -3}, // PV power
		{30356, "U32", "kW", -3}, // consumption power
		{30358, "I32", "kW", -3}, // grid exchange power
		{30360, "I32", "kW", -3}, // battery power
		{30364, "I32", "kW", -3}, // inverter AC power
	}},
	{30368, 1, []huaweiEMMASiteField{{30368, "U16", "%", -2}}}, // battery state of charge
}

func (g huaweiEMMASiteExtent) observation(kept huaweiPlantState, outcome SUN2000GroupOutcome, code uint8) EMMASiteGroup {
	out := EMMASiteGroup{
		UnitID:                huaweiEMMAObservedGatewayUnit,
		ProviderGroupEvidence: kept.evidence(g.offset, g.quantity, outcome, code),
		Words:                 kept.words,
		Facts:                 make([]EMMASiteFact, 0, len(g.fields)),
	}
	for _, field := range g.fields {
		fact := EMMASiteFact{Key: "gateway-self." + strconv.Itoa(int(field.offset)), Offset: field.offset,
			Type: field.typ, Unit: field.unit, Exponent: field.exponent, PolarityUnknown: field.typ == "I32",
			Availability: FactUnknown, Grade: GradeUnknown}
		if len(kept.words) == int(g.quantity) {
			fact.decode(kept.words[field.offset-g.offset:])
		}
		out.Facts = append(out.Facts, fact)
	}
	return out
}

func (f *EMMASiteFact) decode(words []uint16) {
	switch f.Type {
	case "U16":
		f.Unsigned = uint64(words[0])
	case "U32":
		f.Unsigned = uint64(words[0])<<16 | uint64(words[1])
	case "U64":
		f.Unsigned = uint64(words[0])<<48 | uint64(words[1])<<32 | uint64(words[2])<<16 | uint64(words[3])
	case "I32":
		f.Signed = int32(uint32(words[0])<<16 | uint32(words[1])) //nolint:gosec // interpret native two's-complement bits
	default:
		return
	}
	f.Availability, f.Grade = FactValue, GradeHypothesis
}
