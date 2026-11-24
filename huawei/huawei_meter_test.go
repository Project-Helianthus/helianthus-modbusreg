package huawei

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestHuaweiMeterReadsFollowEachProviderPath(t *testing.T) {
	t.Run("logger", func(t *testing.T) {
		fixture, start := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "start_identification")
		rig := newH06Rig(t, fixture.Path, []uint8{1, 2, 9, 11, 19, 20})
		script := withSyntheticPlantReads(fixture.Path, scriptWithoutPlantReads(t, rig.sources, fixture.Path, start))
		script = append(script, loggerMeterReplies(11, 101)...)
		script = append(script, loggerMeterReplies(19, 202)...)
		cycle := rig.refresh(t, script...)
		requireMeterReply(t, cycle, 0, loggerMeterReplies(11, 101)[0], loggerACFacts("21474837.49")...)
		requireMeterReply(t, cycle, 1, loggerMeterReplies(11, 101)[1], loggerEnergyFacts...)
		requireMeterReply(t, cycle, 4, loggerMeterReplies(19, 202)[0], loggerACFacts("21474838.50")...)
		requireMeterReply(t, cycle, 5, loggerMeterReplies(19, 202)[1], loggerEnergyFacts...)
		requireLoggerMeterRefusals(t, cycle, 11, GradeProven)
		requireLoggerMeterRefusals(t, cycle, 19, GradeProven)
		if !cycle.MeterFactsAdmitted() || len(cycle.Meter) != 8 {
			t.Fatal("logger meter readings missing")
		}
	})
	t.Run("energy manager", func(t *testing.T) {
		clock := newH06Clock()
		script := append(emmaSiteIdentityScript(), emmaSiteScript(0, 1, 0, 0, 0, 2)...)
		session := newH06Session("emma", clock, &h06Recorder{}, script...)
		cycle := refreshEMMASite(t, startEMMASite(t, clock, session, nil))
		requireEMMAMeter(t, cycle)
		if session.Remaining() != 0 || len(session.Unexpected()) != 0 {
			t.Fatalf("meter read plan: %d reads left, unexpected %v", session.Remaining(), session.Unexpected())
		}
	})
	t.Run("dongle", func(t *testing.T) {
		fixture, start := h06FixtureCycle(t, "sdongle_provider_cycles.json", "start_identification")
		rig := newH06Rig(t, fixture.Path, []uint8{1})
		script := withSyntheticPlantReads(fixture.Path, scriptWithoutPlantReads(t, rig.sources, fixture.Path, start))
		cycle := rig.refresh(t, append(script, dongleMeterReplies()...)...)
		requireDongleMeter(t, cycle)
		requireDongleRawOnly(t, rig, fixture, cycle.Meter[1])
	})
}

func meterReply(unit uint8, offset, quantity uint16, words ...uint16) h06Expected {
	block := make([]uint16, quantity)
	copy(block, words)
	return h06FC03(unit, offset, quantity, h06Answer{words: block})
}

func loggerMeterReplies(unit uint8, seed uint16) []h06Expected {
	return []h06Expected{
		meterReply(unit, 32260, 33, 0x8000, seed, 0, 23001, 0, 23002, 11, 12, 13, 14, 15, 16,
			0xffff, 0xff85, 0, 124, 0, 125, 0xffff, 0xfb2e, 17, 18, 19, 20, 0xff06),
		meterReply(unit, 32335, 30, 0, 1011, 0xffff, 0xf80a, 0, 3033, 1, 2, 3, 4,
			21, 22, 23, 24, 0xfffe, 0x1234, 0x5678, 0x9abc, 25, 26, 27, 28, 0, 0, 9, 29),
	}
}

func emmaMeterReplies() []h06Expected {
	return []h06Expected{
		meterReply(0, 31639, 20, 0, 23101, 0x8000, 23011, 0, 23103, 1, 2, 3, 4, 5, 6,
			0, 321, 0xffff, 0xfedc, 0, 323, 0xffff, 0xef01),
		meterReply(0, 31661, 10, 0xffff, 0xff55, 7, 8, 0, 4123, 0, 4234, 0xffff, 0xeeee),
		meterReply(0, 31671, 4, 2, 3, 4, 5),
		meterReply(0, 31679, 4, 0xffff, 0xfffe, 0xfffd, 0xfffc),
		meterReply(0, 31687, 4, 0, 0, 9, 87),
	}
}

func dongleMeterReplies() []h06Expected {
	return []h06Expected{
		meterReply(1, 37100, 1, 84),
		meterReply(1, 37101, 24, 0, 2311, 0, 2312, 0xffff, 0xf6f5, 0, 3123, 0, 3223, 0xffff, 0xf303,
			0xffff, 0xedcb, 10, 11, 0xff15, 12, 0, 45678, 0, 56789, 13, 14),
		meterReply(1, 37125, 1, 85),
		meterReply(1, 37126, 13, 15, 16, 17, 18, 19, 20, 0, 3211, 0xffff, 0xeedd, 0, 4321, 86),
	}
}

type meterFactWant struct {
	key              string
	offset           uint16
	typ, unit, value string
	polarity         MeterPolarity
}

func loggerACFacts(voltage string) []meterFactWant {
	return []meterFactWant{
		{"phase_a_voltage", 32260, "U32", "V", voltage, ""},
		{"phase_b_voltage", 32262, "U32", "V", "230.01", ""},
		{"phase_c_voltage", 32264, "U32", "V", "230.02", ""},
		{"phase_a_current", 32272, "I32", "A", "-12.3", ""},
		{"phase_b_current", 32274, "I32", "A", "12.4", ""},
		{"phase_c_current", 32276, "I32", "A", "12.5", ""},
		{"active_power", 32278, "I32", "kW", "-1.234", MeterPolarityUnknown},
		{"power_factor", 32284, "I16", "", "-0.250", ""},
	}
}

var loggerEnergyFacts = []meterFactWant{
	{"phase_a_active_power", 32335, "I32", "kW", "1.011", MeterPolarityUnknown},
	{"phase_b_active_power", 32337, "I32", "kW", "-2.038", MeterPolarityUnknown},
	{"phase_c_active_power", 32339, "I32", "kW", "3.033", MeterPolarityUnknown},
	{"total_active_energy", 32341, "I64", "kWh", "2814835668418.60", ""},
	{"negative_active_energy", 32349, "I64", "kWh", "-5429339550774.44", ""},
	{"positive_active_energy", 32357, "I64", "kWh", "5898.53", ""},
}

func requireEMMAMeter(t *testing.T, cycle ProviderCycle) {
	t.Helper()
	if !cycle.MeterFactsAdmitted() || len(cycle.Meter) != 5 {
		t.Fatal("EMMA meter readings missing")
	}
	reads := emmaMeterReplies()
	requireMeterReply(t, cycle, 0, reads[0],
		meterFactWant{"phase_a_voltage", 31639, "U32", "V", "231.01", ""},
		meterFactWant{"phase_b_voltage", 31641, "U32", "V", "21475066.59", ""},
		meterFactWant{"phase_c_voltage", 31643, "U32", "V", "231.03", ""},
		meterFactWant{"phase_a_current", 31651, "I32", "A", "32.1", ""},
		meterFactWant{"phase_b_current", 31653, "I32", "A", "-29.2", ""},
		meterFactWant{"phase_c_current", 31655, "I32", "A", "32.3", ""},
		meterFactWant{"active_power", 31657, "I32", "kW", "-4.351", MeterPolarityUnknown})
	requireMeterReply(t, cycle, 1, reads[1],
		meterFactWant{"power_factor", 31661, "I32", "", "-0.171", ""},
		meterFactWant{"phase_a_active_power", 31665, "I32", "kW", "4.123", MeterPolarityUnknown},
		meterFactWant{"phase_b_active_power", 31667, "I32", "kW", "4.234", MeterPolarityUnknown},
		meterFactWant{"phase_c_active_power", 31669, "I32", "kW", "-4.370", MeterPolarityUnknown})
	requireMeterReply(t, cycle, 2, reads[2], meterFactWant{"total_active_energy", 31671, "I64", "kWh", "5629628385853.49", ""})
	requireMeterReply(t, cycle, 3, reads[3], meterFactWant{"negative_active_energy", 31679, "I64", "kWh", "-42950983.72", ""})
	requireMeterReply(t, cycle, 4, reads[4], meterFactWant{"positive_active_energy", 31687, "I64", "kWh", "5899.11", ""})
}

func requireDongleMeter(t *testing.T, cycle ProviderCycle) {
	t.Helper()
	if !cycle.MeterFactsAdmitted() || len(cycle.Meter) != 4 {
		t.Fatal("dongle meter readings missing")
	}
	reads := dongleMeterReplies()
	requireMeterReply(t, cycle, 0, reads[0])
	requireMeterReply(t, cycle, 1, reads[1],
		meterFactWant{"phase_a_voltage", 37101, "I32", "V", "231.1", ""},
		meterFactWant{"phase_b_voltage", 37103, "I32", "V", "231.2", ""},
		meterFactWant{"phase_c_voltage", 37105, "I32", "V", "-231.5", ""},
		meterFactWant{"phase_a_current", 37107, "I32", "A", "31.23", ""},
		meterFactWant{"phase_b_current", 37109, "I32", "A", "32.23", ""},
		meterFactWant{"phase_c_current", 37111, "I32", "A", "-33.25", ""},
		meterFactWant{"active_power", 37113, "I32", "W", "-4661", MeterPolarityExportPositive},
		meterFactWant{"power_factor", 37117, "I16", "", "-0.235", ""},
		meterFactWant{"export_active_energy", 37119, "I32", "kWh", "456.78", ""},
		meterFactWant{"import_active_energy", 37121, "I32", "kWh", "567.89", ""})
	requireMeterReply(t, cycle, 2, reads[2])
	requireMeterReply(t, cycle, 3, reads[3],
		meterFactWant{"phase_a_active_power", 37132, "I32", "W", "3211", MeterPolarityExportPositive},
		meterFactWant{"phase_b_active_power", 37134, "I32", "W", "-4387", MeterPolarityExportPositive},
		meterFactWant{"phase_c_active_power", 37136, "I32", "W", "4321", MeterPolarityExportPositive})
}

// Both raw-only extents can answer even when the two fact-bearing extents
// refuse. Neither the raw responses nor retained facts count as fresh facts.
func requireDongleRawOnly(t *testing.T, rig *h06Rig, fixture h06Fixture, kept ProviderMeterGroup) {
	t.Helper()
	held := fixture.Cycles[1]
	script := withSyntheticPlantReads(fixture.Path, scriptWithoutPlantReads(t, rig.sources, fixture.Path, held))
	replies := dongleMeterReplies()
	replies[1].answer = h06Answer{err: h06Exception(3, 3)}
	replies[3].answer = h06Answer{err: h06Exception(3, 3)}
	cycle := rig.refresh(t, append(slices.Clone(script), replies...)...)
	if cycle.MeterFactsAdmitted() || !cycle.Meter[1].Refused || !cycle.Meter[3].Refused {
		t.Fatal("raw words or retained facts admitted as freshly decoded")
	}
	requireKeptMeter(t, cycle.Meter[1], kept)
	cycle = rig.refresh(t, append(script, replies[0], replies[2])...)
	requireMeterReply(t, cycle, 0, replies[0])
	requireMeterReply(t, cycle, 2, replies[2])
	if cycle.MeterFactsAdmitted() || cycle.Meter[1].Outcome != SUN2000GroupNotRead || !cycle.Meter[1].Refused {
		t.Fatal("refusal did not carry into the raw-only refresh")
	}
	requireKeptMeter(t, cycle.Meter[1], kept)
}

func requireMeterReply(t *testing.T, cycle ProviderCycle, index int, read h06Expected, wants ...meterFactWant) {
	t.Helper()
	if len(cycle.Meter) <= index {
		t.Fatalf("missing meter reply %d for unit %d", read.offset, read.unit)
	}
	group := cycle.Meter[index]
	if group.UnitID != read.unit || group.Offset != read.offset || group.Quantity != read.quantity ||
		group.Outcome != SUN2000GroupAnswered || group.Retained || group.Refused || group.ExceptionCode != 0 ||
		group.Cycle != cycle.Cycle || group.Generation != cycle.Generation || group.ReadAt.IsZero() ||
		!slices.Equal(group.Words, read.answer.words) || len(group.Facts) != len(wants) || group.MeterFactsAdmitted() != (len(wants) != 0) {
		t.Fatalf("meter group %d: %+v", index, group)
	}
	for i, want := range wants {
		fact := group.Facts[i]
		if fact.Key != want.key || fact.Offset != want.offset || fact.Type != want.typ || fact.Unit != want.unit || fact.Polarity != want.polarity ||
			fact.Availability != FactValue || fact.Grade != GradeHypothesis || fact.Value.String() != want.value {
			t.Errorf("meter fact %+v, want %+v", fact, want)
		}
	}
}

func requireLoggerMeterRefusals(t *testing.T, cycle ProviderCycle, unit uint8, grade EvidenceGrade) {
	t.Helper()
	for _, extent := range [][2]uint16{{32293, 6}, {33000, 7}} {
		if !slices.ContainsFunc(cycle.Meter, func(group ProviderMeterGroup) bool {
			return group.UnitID == unit && group.Offset == extent[0] && group.Quantity == extent[1] && group.Refused &&
				group.Outcome == SUN2000GroupNotRead && group.ExceptionCode == 3 && group.RefusalGrade == grade &&
				len(group.Words) == 0 && len(group.Facts) == 0 && group.Cycle == 0 && !group.MeterFactsAdmitted()
		}) || !slices.ContainsFunc(cycle.Loss, func(loss ProviderLoss) bool {
			return loss.Kind == ProviderLossRefusedGroup && loss.HasUnit && loss.UnitID == unit && loss.Offset == extent[0] && loss.Quantity == extent[1]
		}) {
			t.Errorf("missing refusal at unit %d: %v", unit, extent)
		}
	}
}

func TestHuaweiMeterReadingRetainsPartialEvidenceWithoutFreshAdmission(t *testing.T) {
	clock := newH06Clock()
	identity := emmaSiteIdentityScript()
	site := emmaSiteScript(0, 1, 0, 0, 0, 2)[:3]
	meters := emmaMeterReplies()
	meters[2] = meterReply(0, 31671, 4, 0x8000, 0, 0, 0)
	session := newH06Session("emma", clock, &h06Recorder{}, identity...)
	session.Append(site...)
	session.Append(meters...)
	provider := startEMMASite(t, clock, session, nil)
	first := refreshEMMASite(t, provider)
	if value := first.Meter[2].Facts[0].Value.String(); value != "-92233720368547758.08" {
		t.Fatalf("minimum I64 reading: %s", value)
	}
	kept := first.Meter[0].clone()
	first.Meter[0].Words[0] = 0xffff
	first.Meter[0].Facts[0].Key = "changed"
	if last, ok := provider.LastComplete(); !ok || !slices.Equal(last.Meter[0].Facts, kept.Facts) || !slices.Equal(last.Meter[0].Words, kept.Words) {
		t.Fatal("returned meter reading aliases saved state")
	}

	clock.Advance(time.Minute)
	session.Append(site...)
	short := h06FC03(0, 31639, 20, h06Answer{words: []uint16{1}})
	session.Append(short, short, short, short)
	meters[3] = meterReply(0, 31679, 4, 0, 0, 0, 0)
	session.Append(meters[1:]...)
	second := refreshEMMASite(t, provider)
	if !second.MeterFactsAdmitted() || second.Meter[0].Outcome != SUN2000GroupFailed {
		t.Fatal("malformed group affected a sibling")
	}
	requireKeptMeter(t, second.Meter[0], kept)
	requireMeterReply(t, second, 3, meters[3], meterFactWant{"negative_active_energy", 31679, "I64", "kWh", "0.00", ""})

	clock.Advance(time.Minute)
	session.Append(site...)
	for _, read := range meters {
		read.answer = h06Answer{err: h06Exception(3, 3)}
		session.Append(read)
	}
	third := refreshEMMASite(t, provider)
	if third.MeterFactsAdmitted() || !third.Meter[0].Refused || session.Remaining() != 0 || len(session.Unexpected()) != 0 {
		t.Fatalf("a retained-only cycle still admits meter facts or diverges from the script: %v", session.Unexpected())
	}
	requireKeptMeter(t, third.Meter[0], kept)
}

func requireKeptMeter(t *testing.T, group, kept ProviderMeterGroup) {
	t.Helper()
	if group.MeterFactsAdmitted() || !group.Retained || group.Cycle != kept.Cycle || group.Generation != kept.Generation ||
		group.ReadAt != kept.ReadAt || !slices.Equal(group.Facts, kept.Facts) || !slices.Equal(group.Words, kept.Words) {
		t.Fatalf("last-known-good meter epoch or payload changed: %+v", group)
	}
}

func TestHuaweiLoggerMeterRefusalsCarryAcrossBuilds(t *testing.T) {
	for _, test := range []struct {
		build    string
		polarity MeterPolarity
	}{
		{"V300R023C10SPC621", MeterPolarityUnknown},
		{"V300R024C10SPC210", MeterPolarityExportPositive},
		{"V300R025C10SPC222", MeterPolarityExportPositive},
		{"V300R026C10SPC333", MeterPolarityExportPositive},
	} {
		t.Run(test.build, func(t *testing.T) {
			fixture, start := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "start_identification")
			rig := newH06Rig(t, fixture.Path, []uint8{11, 19})
			steps := h06StepsForUnits(start.Steps, 11)
			script := scriptWithoutPlantReads(t, rig.sources, fixture.Path, h06Cycle{Steps: steps})
			for i := range script {
				target := script[i].answer.page
				if target == nil {
					continue
				}
				for index := range target.Objects {
					object := &target.Objects[index]
					text := strings.ReplaceAll(string(object.Value), "V300R023C10SPC620", test.build)
					target.ResponseBytes += len(text) - len(object.Value)
					object.Value = []byte(text)
				}
			}
			for iteration := range 2 {
				if iteration != 0 {
					script = []h06Expected{h06FC03(0, 65521, 1, h06Answer{words: []uint16{rig.sources.smartLoggerInventory.ChangeCounter.Before}})}
				}
				reads := withLegacyMeterReads(fixture.Path, withSyntheticPlantReads(fixture.Path, script), []uint8{11, 19})
				cycle := rig.refresh(t, reads...)
				if cycle.Gateway.Firmware != test.build || !cycle.MeterFactsAdmitted() {
					t.Fatalf("meter not monitored on admitted build: %s %s", cycle.Gateway.Firmware, cycle.Reason)
				}
				for _, unit := range []uint8{11, 19} {
					requireLoggerMeterRefusals(t, cycle, unit, GradeHypothesis)
				}
				for _, index := range []int{0, 4} {
					if power := cycle.Meter[index].Facts[6]; power.Polarity != test.polarity || power.Value.String() != "-1.234" {
						t.Errorf("documented polarity must preserve native power: %+v", power)
					}
				}
			}
		})
	}
}
