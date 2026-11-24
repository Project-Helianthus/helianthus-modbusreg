package huawei

import (
	"slices"
	"testing"
)

type plantFactWant struct {
	key    string
	offset uint16
	unit   string
	value  string
}

func requirePlantRead(t *testing.T, cycle ProviderCycle, index int, read h06Expected, wants ...plantFactWant) {
	t.Helper()
	group := cycle.Plant[index]
	if group.UnitID != read.unit || group.Offset != read.offset || group.Quantity != read.quantity ||
		group.Outcome != SUN2000GroupAnswered || group.Retained || group.Refused || group.ExceptionCode != 0 ||
		group.Cycle != cycle.Cycle || group.Generation != cycle.Generation || group.ReadAt.IsZero() ||
		!slices.Equal(group.Words, read.answer.words) {
		t.Fatalf("plant group %d: %+v", index, group)
	}
	if len(group.Facts) != len(wants) {
		t.Fatalf("plant group %d facts: %+v", index, group.Facts)
	}
	for i, want := range wants {
		fact := group.Facts[i]
		if fact.Key != want.key || fact.Offset != want.offset || fact.Unit != want.unit ||
			fact.Availability != FactValue || fact.Grade != GradeHypothesis || fact.Value.String() != want.value {
			t.Errorf("plant fact: %+v, want %+v", fact, want)
		}
	}
}

func TestHuaweiPlantGroupsReachProviderCycle(t *testing.T) {
	// Exact scripts vary each read key: FC03 (not identification), both units
	// (not the routed child), all offsets (not a neighbor), and quantities
	// 6, 4, 8 and 2 (not a shortened, split or enlarged request).
	for _, test := range []struct {
		fixture string
		reads   []h06Expected
		facts   [][]plantFactWant
	}{
		{"smartlogger_provider_cycles.json", []h06Expected{
			h06FC03(0, 40521, 6, h06Answer{words: []uint16{0x8000, 11, 0x5678, 22, 0xffff, 0xfc0d}}),
			h06FC03(0, 40560, 4, h06Answer{words: []uint16{0x9000, 33, 2, 44}}),
		}, [][]plantFactWant{
			{{"plant_dc_input_power", 40521, "kW", "2147483.659"}, {"plant_ac_power", 40525, "kW", "-1.011"}},
			{{"plant_total_energy", 40560, "kWh", "241591913.7"}, {"plant_daily_energy", 40562, "kWh", "13111.6"}},
		}},
		{"sdongle_provider_cycles.json", []h06Expected{
			h06FC03(100, 37498, 8, h06Answer{words: []uint16{0x8000, 11, 0x9000, 22, 0, 3333, 0xffff, 0xeea4}}),
			h06FC03(100, 37516, 2, h06Answer{words: []uint16{0xffff, 0xea4d}}),
		}, [][]plantFactWant{
			{
				{"plant_dc_input_power", 37498, "kW", "2147483.659"},
				{"plant_consumption_power", 37500, "kW", "2415919.126"},
				{"plant_grid_exchange_power", 37502, "kW", "3.333"},
				{"plant_battery_power", 37504, "kW", "-4.444"},
			},
			{{"plant_ac_power", 37516, "kW", "-5.555"}},
		}},
	} {
		t.Run(test.fixture, func(t *testing.T) {
			fixture, _ := h06Load(t, test.fixture)
			rig := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
			script := scriptWithoutPlantReads(t, rig.sources, fixture.Path, fixture.Cycles[0])
			cycle := rig.refresh(t, insertPlantReads(script, test.reads)...)
			h06RequireExpect(t, fixture.Path, cycle, fixture.Cycles[0].Expect)
			if len(cycle.Plant) != len(test.reads) {
				t.Fatalf("plant groups: %+v", cycle.Plant)
			}
			for i, read := range test.reads {
				requirePlantRead(t, cycle, i, read, test.facts[i]...)
			}
			for _, loss := range cycle.Loss {
				if loss.Kind == ProviderLossPlannedGroup {
					t.Fatalf("plant group is still planned: %+v", loss)
				}
			}
			if fixture.Path == "S-Dongle" {
				checkDonglePlantReversal(t, rig, fixture, test.reads[1], test.facts[1])
			}
		})
	}
}

func checkDonglePlantReversal(t *testing.T, rig *h06Rig, fixture h06Fixture, ac h06Expected, acFacts []plantFactWant) {
	t.Helper()
	// A held guard also reads both groups. Reverse the grid and battery
	// directions without changing the DC and consumption words.
	power := h06FC03(100, 37498, 8, h06Answer{words: []uint16{0x8000, 11, 0x9000, 22, 0xffff, 0xe5f6, 0, 7777}})
	step := fixture.Cycles[1]
	script := scriptWithoutPlantReads(t, rig.sources, fixture.Path, step)
	cycle := rig.refresh(t, insertPlantReads(script, []h06Expected{power, ac})...)
	h06RequireExpect(t, fixture.Path, cycle, step.Expect)
	requirePlantRead(t, cycle, 0, power,
		plantFactWant{"plant_dc_input_power", 37498, "kW", "2147483.659"},
		plantFactWant{"plant_consumption_power", 37500, "kW", "2415919.126"},
		plantFactWant{"plant_grid_exchange_power", 37502, "kW", "-6.666"},
		plantFactWant{"plant_battery_power", 37504, "kW", "7.777"},
	)
	requirePlantRead(t, cycle, 1, ac, acFacts...)
}

func TestHuaweiPlantGroupRefusalIsLocal(t *testing.T) {
	for _, havePrior := range []bool{false, true} {
		name := "no prior reading"
		if havePrior {
			name = "with prior reading"
		}
		t.Run(name, func(t *testing.T) {
			fixture, _ := h06Load(t, "smartlogger_provider_cycles.json")
			rig := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
			step := fixture.Cycles[0]
			var kept ProviderPlantGroup
			if havePrior {
				prior := rig.refresh(t, h06Script(t, rig.sources, fixture.Path, step)...)
				last, ok := rig.provider.LastComplete()
				if !ok {
					t.Fatal("missing complete plant snapshot")
				}
				kept = last.Plant[0]
				// The returned snapshots must not alias provider state.
				prior.Plant[0].Words[0] = 1
				prior.Plant[0].Facts[0].Value = Decimal{}
				step = fixture.Cycles[1]
			}
			script := scriptWithoutPlantReads(t, rig.sources, fixture.Path, step)
			energy := h06FC03(0, 40560, 4, h06Answer{words: []uint16{0, 111, 0, 222}})
			refusal := h06FC03(0, 40521, 6, h06Answer{err: h06Exception(3, 3)})
			cycle := rig.refresh(t, insertPlantReads(script, []h06Expected{refusal, energy})...)
			requireLoggerPlantRefusal(t, cycle, kept, havePrior, SUN2000GroupException, 3)
			requirePlantRead(t, cycle, 1, energy,
				plantFactWant{"plant_total_energy", 40560, "kWh", "11.1"},
				plantFactWant{"plant_daily_energy", 40562, "kWh", "22.2"},
			)
			if !havePrior {
				h06RequireExpect(t, fixture.Path, cycle, step.Expect)
			}
			guard := scriptWithoutPlantReads(t, rig.sources, fixture.Path, fixture.Cycles[1])
			next := rig.refresh(t, insertPlantReads(guard, []h06Expected{energy})...)
			requireLoggerPlantRefusal(t, next, kept, havePrior, SUN2000GroupNotRead, 0)
		})
	}
}

func requireLoggerPlantRefusal(t *testing.T, cycle ProviderCycle, kept ProviderPlantGroup, havePrior bool, outcome SUN2000GroupOutcome, code uint8) {
	t.Helper()
	if cycle.Outcome != ProviderCycleComplete || len(cycle.Plant) != 2 {
		t.Fatalf("partial plant cycle: %s %+v", cycle.Outcome, cycle.Plant)
	}
	group := cycle.Plant[0]
	if !group.Refused || group.Outcome != outcome || group.ExceptionCode != code || group.Retained != havePrior {
		t.Fatalf("refusal evidence: %+v", group)
	}
	if havePrior {
		if group.Cycle != kept.Cycle || group.Generation != kept.Generation || !group.ReadAt.Equal(kept.ReadAt) ||
			!slices.Equal(group.Words, kept.Words) || !slices.Equal(group.Facts, kept.Facts) {
			t.Fatalf("refusal replaced the last-known-good group: %+v, want %+v", group, kept)
		}
	} else if len(group.Words) != 0 || len(group.Facts) != 2 || group.Facts[0].Availability != FactUnknown || group.Facts[1].Availability != FactUnknown {
		t.Fatalf("refusal invented plant power facts: %+v", group)
	}
	for _, loss := range cycle.Loss {
		if loss.Kind == ProviderLossRefusedGroup && loss.HasUnit && loss.UnitID == 0 && loss.Offset == 40521 && loss.Quantity == 6 {
			return
		}
	}
	t.Fatalf("no unit-scoped plant loss: %+v", cycle.Loss)
}
