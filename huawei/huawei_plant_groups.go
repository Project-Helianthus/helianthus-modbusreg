package huawei

// ProviderPlantFact is a gateway-native measurement at a two-word register
// offset. Its read and freshness are described by the containing group.
type ProviderPlantFact struct {
	Key          string
	Offset       uint16
	Unit         string
	Availability FactAvailability
	Value        Decimal
	Grade        EvidenceGrade
}

// ProviderPlantGroup is the evidence and decoded facts of one gateway plant
// read. Words include pairs that are not published as facts.
type ProviderPlantGroup struct {
	UnitID uint8
	ProviderGroupEvidence
	Words []uint16
	Facts []ProviderPlantFact
}

func (g ProviderPlantGroup) clone() ProviderPlantGroup {
	g.Words = append([]uint16(nil), g.Words...)
	g.Facts = append([]ProviderPlantFact(nil), g.Facts...)
	return g
}

type huaweiPlantField struct {
	key      string
	offset   uint16
	wordType huaweiSUN2000WordType
	unit     string
	exponent int16
}

type huaweiPlantExtent struct {
	offset, quantity uint16
	fields           []huaweiPlantField
}

type huaweiPlantPlan struct {
	unit   uint8
	groups [2]huaweiPlantExtent
}

var huaweiSmartLoggerPlant = huaweiPlantPlan{unit: 0, groups: [2]huaweiPlantExtent{
	{40521, 6, []huaweiPlantField{
		{"plant_dc_input_power", 40521, huaweiSUN2000U32, "kW", -3},
		// 40523 is the aggregate CO2 counter, not a plant power fact.
		{"plant_ac_power", 40525, huaweiSUN2000I32, "kW", -3},
	}},
	{40560, 4, []huaweiPlantField{
		{"plant_total_energy", 40560, huaweiSUN2000U32, "kWh", -1},
		{"plant_daily_energy", 40562, huaweiSUN2000U32, "kWh", -1},
	}},
}}

var huaweiSDonglePlant = huaweiPlantPlan{unit: 100, groups: [2]huaweiPlantExtent{
	{37498, 8, []huaweiPlantField{
		{"plant_dc_input_power", 37498, huaweiSUN2000U32, "kW", -3},
		{"plant_consumption_power", 37500, huaweiSUN2000U32, "kW", -3},
		{"plant_grid_exchange_power", 37502, huaweiSUN2000I32, "kW", -3}, // import-positive
		{"plant_battery_power", 37504, huaweiSUN2000I32, "kW", -3},       // charging-positive
	}},
	{37516, 2, []huaweiPlantField{
		{"plant_ac_power", 37516, huaweiSUN2000I32, "kW", -3},
	}},
}}

func (g huaweiPlantExtent) decode(words []uint16) []ProviderPlantFact {
	facts := make([]ProviderPlantFact, 0, len(g.fields))
	for _, field := range g.fields {
		fact := ProviderPlantFact{Key: field.key, Offset: field.offset, Unit: field.unit, Availability: FactUnknown, Grade: GradeUnknown}
		if len(words) == int(g.quantity) {
			decoder := huaweiSUN2000Field{wordType: field.wordType, exponent: field.exponent, kind: huaweiSUN2000Measurement}
			if value, _, ok := decodeHuaweiSUN2000Field(decoder, words[field.offset-g.offset:]); ok {
				fact.Value, fact.Availability, fact.Grade = value, FactValue, GradeHypothesis
			}
		}
		facts = append(facts, fact)
	}
	return facts
}

// Kept words are immutable and are replaced only by an entire answered read.
type huaweiPlantState struct {
	words   []uint16
	meta    huaweiGroupMeta
	refused bool
}

func (g huaweiPlantState) observation(unit uint8, extent huaweiPlantExtent, outcome SUN2000GroupOutcome, code uint8) ProviderPlantGroup {
	return ProviderPlantGroup{
		UnitID: unit,
		ProviderGroupEvidence: ProviderGroupEvidence{
			Offset: extent.offset, Quantity: extent.quantity, Outcome: outcome, ExceptionCode: code, Refused: g.refused,
			Retained: len(g.words) != 0 && outcome != SUN2000GroupAnswered,
			Cycle:    g.meta.cycle, ReadAt: g.meta.readAt, Generation: g.meta.generation,
		},
		Words: g.words, Facts: extent.decode(g.words),
	}
}
