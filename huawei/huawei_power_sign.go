package huawei

// EMMASitePowerDirection is a direction inferred for one EMMA site interval.
// An empty direction is unknown.
type EMMASitePowerDirection string

// The two opposite directions for grid exchange and for storage.
const (
	EMMASitePowerImport    EMMASitePowerDirection = "import"
	EMMASitePowerExport    EMMASitePowerDirection = "export"
	EMMASitePowerCharge    EMMASitePowerDirection = "charge"
	EMMASitePowerDischarge EMMASitePowerDirection = "discharge"
)

// EMMASitePowerCounterSample identifies one unit 0 energy read and the exact
// U64 coefficients of the two totals (kWh, gain 100), in CounterOffsets order.
// Availability describes the coefficients; Evidence distinguishes retained
// readings from current ones.
type EMMASitePowerCounterSample struct {
	Evidence     ProviderGroupEvidence
	Totals       [2]uint64
	Availability FactAvailability
}

// EMMASitePowerReading is separate from the signed native fact at Offset.
// Value uses the native power magnitude with import/charge positive and
// export/discharge negative. Direction and Value are only set when Derived is
// true; otherwise Reason describes why the reading is unknown.
type EMMASitePowerReading struct {
	Offset         uint16
	PowerEvidence  ProviderGroupEvidence
	Unit           string
	Availability   FactAvailability
	Direction      EMMASitePowerDirection
	Value          Decimal
	Derived        bool
	Reason         string
	CounterOffsets [2]uint16
	Previous       EMMASitePowerCounterSample
	Current        EMMASitePowerCounterSample
}

// The site read plan emits energy first and power second, including Unknown
// groups after a failed admission. No meter or daily counters enter this path.
func (c *ProviderCycle) deriveEMMASitePower(kept huaweiPlantState) {
	energy, power := c.EMMASite[0], c.EMMASite[1]
	var previous EMMASiteGroup
	if c.Cycle > 1 && kept.meta.cycle == c.Cycle-1 && kept.meta.generation == c.Generation {
		previous = huaweiEMMASite[0].observation(kept, SUN2000GroupAnswered, 0)
	}
	c.EMMASitePower = []EMMASitePowerReading{
		c.deriveEMMAPower(power, energy, previous, 30358, [2]uint16{30338, 30332}, EMMASitePowerImport, EMMASitePowerExport),
		c.deriveEMMAPower(power, energy, previous, 30360, [2]uint16{30308, 30314}, EMMASitePowerCharge, EMMASitePowerDischarge),
	}
}

func (c ProviderCycle) deriveEMMAPower(power, energy, previous EMMASiteGroup, offset uint16, counters [2]uint16, positive, negative EMMASitePowerDirection) EMMASitePowerReading {
	native := emmaPowerSourceFact(power, offset)
	out := EMMASitePowerReading{
		Offset: offset, PowerEvidence: power.ProviderGroupEvidence, Unit: native.Unit, Availability: FactUnknown,
		CounterOffsets: counters, Previous: emmaPowerCounterSample(previous, counters), Current: emmaPowerCounterSample(energy, counters),
	}
	before, now := out.Previous.Totals, out.Current.Totals
	switch {
	case !c.currentEMMASiteGroup(energy) || out.Current.Availability != FactValue:
		out.Reason = "current_counters_unavailable"
	case !c.currentEMMASiteGroup(power) || native.Availability != FactValue:
		out.Reason = "current_power_unavailable"
	case out.Previous.Availability != FactValue:
		out.Reason = "missing_previous_counters"
	case now[0] < before[0] || now[1] < before[1]:
		out.Reason = "counter_regressed"
	case now[0] > before[0] && now[1] > before[1]:
		out.Reason = "both_counters_advanced"
	case now == before:
		out.Reason = "neither_counter_advanced"
	default:
		magnitude := int64(native.Signed)
		if magnitude < 0 {
			magnitude = -magnitude
		}
		out.Direction = positive
		if now[1] > before[1] {
			out.Direction, magnitude = negative, -magnitude
		}
		out.Value = Decimal{Coefficient: magnitude, Exponent: native.Exponent}
		out.Availability, out.Derived = FactValue, true
	}
	return out
}

func (c ProviderCycle) currentEMMASiteGroup(group EMMASiteGroup) bool {
	return group.Outcome == SUN2000GroupAnswered && !group.Retained && group.Cycle == c.Cycle && group.Generation == c.Generation
}

func emmaPowerCounterSample(group EMMASiteGroup, offsets [2]uint16) EMMASitePowerCounterSample {
	sample := EMMASitePowerCounterSample{Evidence: group.ProviderGroupEvidence, Availability: FactUnknown}
	first, second := emmaPowerSourceFact(group, offsets[0]), emmaPowerSourceFact(group, offsets[1])
	if first.Availability == FactValue && second.Availability == FactValue {
		sample.Totals, sample.Availability = [2]uint64{first.Unsigned, second.Unsigned}, FactValue
	}
	return sample
}

func emmaPowerSourceFact(group EMMASiteGroup, offset uint16) EMMASiteFact {
	for _, fact := range group.Facts {
		if fact.Offset == offset {
			return fact
		}
	}
	return EMMASiteFact{}
}
