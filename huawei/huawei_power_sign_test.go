package huawei

import (
	"testing"
	"time"
)

// The counter order of this script is import, export, charge, discharge.
func emmaPowerSignScript(totals [4]uint64) []h06Expected {
	script := emmaSiteScript(0, 1, 0, 0, 0, 2)
	for i, offset := range [...]uint16{30338, 30332, 30308, 30314} {
		for word := range 4 {
			script[0].answer.words[int(offset-30306)+word] = uint16(totals[i] >> (48 - 16*word))
		}
	}
	return script
}

func requireEMMAPowerPair(t *testing.T, cycle ProviderCycle, grid, battery EMMASitePowerReading) {
	t.Helper()
	if len(cycle.EMMASitePower) != 2 || cycle.EMMASitePower[0] != grid || cycle.EMMASitePower[1] != battery {
		t.Fatalf("cycle %d: derived power %+v; want grid %+v, battery %+v", cycle.Cycle, cycle.EMMASitePower, grid, battery)
	}
}

func expectedEMMAPower(cycle ProviderCycle, power uint16, counters [2]uint16, current [2]uint64) EMMASitePowerReading {
	return EMMASitePowerReading{
		Offset: power, PowerEvidence: cycle.EMMASite[1].ProviderGroupEvidence, Unit: "kW", Availability: FactUnknown,
		CounterOffsets: counters, Previous: EMMASitePowerCounterSample{Availability: FactUnknown}, Current: EMMASitePowerCounterSample{
			Evidence: cycle.EMMASite[0].ProviderGroupEvidence, Totals: current, Availability: FactValue,
		},
	}
}

func TestEMMASitePowerDerivesIndependentCounterDirections(t *testing.T) {
	clock := newH06Clock()
	session := newH06Session("emma", clock, &h06Recorder{}, emmaSiteIdentityScript()...)
	provider := startEMMASite(t, clock, session, nil)
	// Cross the signed 64-bit boundary, then exercise each source and polarity,
	// both-advanced, neither-advanced, and regression even when its peer advances.
	const initialImport = 1<<63 - 1
	var prev ProviderCycle
	var last [4]uint64
	for _, tc := range []struct {
		totals                  [4]uint64
		grid, battery           EMMASitePowerDirection
		gridWhy, batteryWhy     string
		gridPower, batteryPower int64
	}{
		{[4]uint64{initialImport, 10, 20, 30}, "", "", "missing_previous_counters", "missing_previous_counters", 0, 0},
		{[4]uint64{initialImport + 1, 10, 20, 31}, "import", "discharge", "", "", 1 << 31, -4567},
		{[4]uint64{initialImport + 1, 12, 23, 31}, "export", "charge", "", "", -3732, 4567},
		{[4]uint64{initialImport + 2, 13, 24, 31}, "", "charge", "both_counters_advanced", "", 0, 4567},
		{[4]uint64{initialImport + 4, 13, 24, 31}, "import", "", "", "neither_counter_advanced", 3732, 0},
		{[4]uint64{initialImport + 3, 14, 25, 29}, "", "", "counter_regressed", "counter_regressed", 0, 0},
	} {
		clock.Advance(time.Minute)
		script := emmaPowerSignScript(tc.totals)
		// Daily counters change (and can roll over) independently of totals.
		for _, index := range []int{1, 7, 25, 31} {
			script[0].answer.words[index] = uint16(tc.totals[1] % 3)
		}
		gridNative := int32(-3732)
		if tc.gridPower == 1<<31 {
			gridNative = -1 << 31
			script[1].answer.words[4], script[1].answer.words[5] = 0x8000, 0
		}
		session.Append(script...)
		cycle := refreshEMMASite(t, provider)
		grid := expectedEMMAPower(cycle, 30358, [2]uint16{30338, 30332}, [2]uint64{tc.totals[0], tc.totals[1]})
		battery := expectedEMMAPower(cycle, 30360, [2]uint16{30308, 30314}, [2]uint64{tc.totals[2], tc.totals[3]})
		grid.Direction, grid.Reason = tc.grid, tc.gridWhy
		battery.Direction, battery.Reason = tc.battery, tc.batteryWhy
		if prev.Cycle != 0 {
			grid.Previous = EMMASitePowerCounterSample{Evidence: prev.EMMASite[0].ProviderGroupEvidence, Totals: [2]uint64{last[0], last[1]}, Availability: FactValue}
			battery.Previous = EMMASitePowerCounterSample{Evidence: prev.EMMASite[0].ProviderGroupEvidence, Totals: [2]uint64{last[2], last[3]}, Availability: FactValue}
		}
		if tc.grid != "" {
			grid.Availability, grid.Derived, grid.Value = FactValue, true, Decimal{Coefficient: tc.gridPower, Exponent: -3}
		}
		if tc.battery != "" {
			battery.Availability, battery.Derived, battery.Value = FactValue, true, Decimal{Coefficient: tc.batteryPower, Exponent: -3}
		}
		requireEMMAPowerPair(t, cycle, grid, battery)
		for key, signed := range map[string]int32{"gateway-self.30358": gridNative, "gateway-self.30360": 4567} {
			fact, ok := cycle.EMMASiteFact(key)
			if !ok || fact.Availability != FactValue || !fact.PolarityUnknown || fact.Signed != signed {
				t.Fatalf("native fact %s was replaced: %+v", key, fact)
			}
		}
		cycle.EMMASitePower[0].Offset = 0
		saved, ok := provider.LastComplete()
		if !ok || saved.EMMASitePower[0] != grid {
			t.Fatal("returned derived reading aliases LastComplete")
		}
		saved.EMMASitePower[0].Offset = 0
		if again, _ := provider.LastComplete(); again.EMMASitePower[0] != grid {
			t.Fatal("LastComplete exposed its stored derived reading")
		}
		prev, last = cycle, tc.totals
	}
	if session.Remaining() != 0 || len(session.Unexpected()) != 0 {
		t.Fatalf("requests: %d left, unexpected %v", session.Remaining(), session.Unexpected())
	}
}

func TestEMMASitePowerRequiresCurrentGroupsAndFreshBaseline(t *testing.T) {
	clock := newH06Clock()
	session := newH06Session("emma", clock, &h06Recorder{}, emmaSiteIdentityScript()...)
	provider := startEMMASite(t, clock, session, nil)
	var prev ProviderCycle
	for i, tc := range []struct {
		failed      uint16
		reason      string
		counterKept bool
		powerKept   bool
	}{
		{0, "missing_previous_counters", false, false},
		{30306, "current_counters_unavailable", true, false},
		{0, "missing_previous_counters", false, false},
		{30354, "current_power_unavailable", false, true},
		{0, "", false, false},
		{0, "missing_previous_counters", false, false}, // new connection generation
	} {
		clock.Advance(time.Minute)
		script := emmaPowerSignScript([4]uint64{uint64(i + 1), 100, 200, uint64(i + 300)})
		if i == 5 {
			session = newH06Session("reconnected", clock, &h06Recorder{}, emmaSiteIdentityScript()...)
			if err := provider.Reconnect(session); err != nil {
				t.Fatal(err)
			}
		}
		for _, read := range script {
			if read.offset == tc.failed {
				read.answer.words = []uint16{17}
				session.Append(read, read, read, read)
			} else {
				session.Append(read)
			}
		}
		cycle := refreshEMMASite(t, provider)
		if len(cycle.EMMASitePower) != 2 {
			t.Fatalf("cycle %d has no derived power entries", cycle.Cycle)
		}
		for _, sign := range cycle.EMMASitePower {
			if sign.Reason != tc.reason || sign.Derived != (tc.reason == "") || sign.Current.Evidence.Retained != tc.counterKept ||
				sign.PowerEvidence.Retained != tc.powerKept || (tc.reason != "" && (sign.Availability != FactUnknown || sign.Direction != "" || sign.Value != (Decimal{}))) {
				t.Fatalf("cycle %d power evidence: %+v", cycle.Cycle, sign)
			}
			if tc.reason == "missing_previous_counters" && sign.Previous.Availability == FactValue {
				t.Fatalf("cycle %d used an old baseline: %+v", cycle.Cycle, sign.Previous)
			}
			if tc.counterKept {
				requireEMMAPowerRetainedEpoch(t, sign.Current.Evidence, prev.EMMASite[0].ProviderGroupEvidence)
			}
			if tc.powerKept {
				requireEMMAPowerRetainedEpoch(t, sign.PowerEvidence, prev.EMMASite[1].ProviderGroupEvidence)
			}
		}
		if session.Remaining() != 0 || len(session.Unexpected()) != 0 {
			t.Fatalf("requests: %d left, unexpected %v", session.Remaining(), session.Unexpected())
		}
		prev = cycle
	}
}

func requireEMMAPowerRetainedEpoch(t *testing.T, got, original ProviderGroupEvidence) {
	t.Helper()
	original.Outcome, original.Retained = SUN2000GroupFailed, true
	if got != original {
		t.Fatalf("retained evidence lost its original epoch: got %+v, want %+v", got, original)
	}
}

func TestEMMASitePowerRejectsIneligibleSourceEpoch(t *testing.T) {
	cycle := ProviderCycle{Cycle: 7, Generation: 3}
	fresh := ProviderGroupEvidence{Outcome: SUN2000GroupAnswered, Cycle: 7, Generation: 3}
	if !cycle.currentEMMASiteGroup(EMMASiteGroup{ProviderGroupEvidence: fresh}) {
		t.Fatal("answered source from the current epoch was rejected")
	}
	for name, change := range map[string]func(*ProviderGroupEvidence){
		"outcome":    func(e *ProviderGroupEvidence) { e.Outcome = SUN2000GroupFailed },
		"retained":   func(e *ProviderGroupEvidence) { e.Retained = true },
		"cycle":      func(e *ProviderGroupEvidence) { e.Cycle-- },
		"generation": func(e *ProviderGroupEvidence) { e.Generation-- },
	} {
		t.Run(name, func(t *testing.T) {
			stale := fresh
			change(&stale)
			if cycle.currentEMMASiteGroup(EMMASiteGroup{ProviderGroupEvidence: stale}) {
				t.Fatal("ineligible source was accepted")
			}
		})
	}
}

func TestHuaweiUndirectedMeterCountersDoNotSupplyPowerDirections(t *testing.T) {
	t.Run("older SmartLogger", func(t *testing.T) {
		fixture, start := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "start_identification")
		rig := newH06Rig(t, fixture.Path, []uint8{1, 2, 9, 11, 19, 20})
		for i, step := range []h06Cycle{start, fixture.Cycles[1]} {
			script := withSyntheticPlantReads(fixture.Path, scriptWithoutPlantReads(t, rig.sources, fixture.Path, step))
			meter := loggerMeterReplies(11, 101)
			meter[1].answer.words[25] = uint16(29 + 3*i) // positive active energy
			script = append(script, meter...)
			script = append(script, loggerMeterReplies(19, 202)...)
			cycle := rig.refresh(t, script...)
			if len(cycle.EMMASitePower) != 0 {
				t.Fatalf("unexpected derived SmartLogger readings: %+v", cycle.EMMASitePower)
			}
			requireUndirectedMeterPower(t, cycle, 11, 32278, -1234)
			wants := append([]meterFactWant(nil), loggerEnergyFacts...)
			if i == 1 {
				wants[5].value = "5898.56"
			}
			requireMeterReply(t, cycle, 1, meter[1], wants...)
		}
	})
	t.Run("EMMA built-in sensor", func(t *testing.T) {
		clock := newH06Clock()
		session := newH06Session("emma", clock, &h06Recorder{}, emmaSiteIdentityScript()...)
		provider := startEMMASite(t, clock, session, nil)
		for i := range 3 {
			clock.Advance(time.Minute)
			imported := uint64(10)
			if i == 2 {
				imported++ // site grid direction can be known while the meter remains open
			}
			script := emmaPowerSignScript([4]uint64{imported, 20, 30, 40})
			script[7].answer.words[3] = uint16(87 + min(i, 1))
			session.Append(script...)
			cycle := refreshEMMASite(t, provider)
			requireUndirectedMeterPower(t, cycle, 0, 31657, -4351)
			if len(cycle.EMMASitePower) != 2 ||
				i == 1 && cycle.EMMASitePower[0].Reason != "neither_counter_advanced" ||
				i == 2 && cycle.EMMASitePower[0].Direction != "import" {
				t.Fatalf("meter and site directions were not independent: %+v", cycle.EMMASitePower)
			}
			want := [...]string{"5899.11", "5899.12", "5899.12"}[i]
			requireMeterReply(t, cycle, 4, script[7], meterFactWant{"positive_active_energy", 31687, "I64", "kWh", want, ""})
		}
		if session.Remaining() != 0 || len(session.Unexpected()) != 0 {
			t.Fatalf("requests: %d left, unexpected %v", session.Remaining(), session.Unexpected())
		}
	})
}

func requireUndirectedMeterPower(t *testing.T, cycle ProviderCycle, unit uint8, offset uint16, native int64) {
	t.Helper()
	for _, group := range cycle.Meter {
		if group.UnitID != unit {
			continue
		}
		for _, fact := range group.Facts {
			if fact.Offset == offset {
				if fact.Key != "active_power" || fact.Availability != FactValue || fact.Polarity != MeterPolarityUnknown || fact.Value != (Decimal{Coefficient: native, Exponent: -3}) {
					t.Fatalf("meter power must retain the open native sign: %+v", fact)
				}
				return
			}
		}
	}
	t.Fatalf("meter power was not published at unit %d offset %d", unit, offset)
}
