package huawei

import (
	"reflect"
	"testing"
)

// TestHuaweiMonthAndYearYieldsStayInsideTheEnergyRead covers the month and year
// counters on both profiles. Each assertion names the production mutation it
// catches:
//   - decoder key absent: the field is removed from a profile
//   - key namespace swapped: the family prefix is dropped or shared
//   - index 1 accepted: the counter is stored as a string channel
//   - offset, group, word type, exponent, unit or grade changed: that field of the counter is edited
//   - plan quantity other than 14: the read no longer covers both counters and the three existing yields
//   - decoded text changed: word type or exponent no longer matches a U32 with exponent -2
//   - hole word changes a fact: the four unnamed words are decoded
//   - short word count publishes: a partial group is zero-filled
//   - failed group publishes a value: an unanswered counter becomes zero
//   - retention drops the previous counter: a later failure replaces the last complete counters with zero
func TestHuaweiMonthAndYearYieldsStayInsideTheEnergyRead(t *testing.T) {
	words := []uint16{
		0x0000, 1000,
		0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF,
		0x0000, 11,
		0x0000, 22,
		0x8000, 0,
		0x4000, 1,
	}
	wantText := map[string]string{
		"energy_accumulated":   "10.00",
		"energy_current_hour":  "0.11",
		"energy_current_day":   "0.22",
		"energy_current_month": "21474836.48",
		"energy_current_year":  "10737418.25",
	}
	for _, profile := range SUN2000MonitoringProfiles() {
		profile := profile
		t.Run(profile.ID(), func(t *testing.T) {
			for _, name := range []string{"energy_current_month", "energy_current_year", "energy_accumulated", "energy_current_hour", "energy_current_day"} {
				field, ok := periodField(profile, name)
				if !ok {
					t.Fatalf("decoder field %s absent", name)
				}
				key := profile.decoderKey(field)
				if key == "" || !containsString(profile.DecoderKeys(), key) {
					t.Fatalf("decoder key %s absent", name)
				}
				other := SUN2000M1MonitoringProfileID
				if profile.ID() == other {
					other = SUN2000M3MonitoringProfileID
				}
				swapped := otherKey(profile, other, field)
				if swapped == key || containsString(profile.DecoderKeys(), swapped) {
					t.Fatalf("namespace collision %s", swapped)
				}
				want := field
				want.name = name
				want.group = huaweiSUN2000Group{offset: 32106, quantity: 14}
				want.wordType = huaweiSUN2000U32
				want.exponent = -2
				want.unit = "kWh"
				want.kind = huaweiSUN2000Measurement
				want.index = 0
				switch name {
				case "energy_accumulated":
					want.offset = 32106
					want.grade = field.grade
				case "energy_current_hour":
					want.offset = 32112
					want.grade = field.grade
				case "energy_current_day":
					want.offset = 32114
					want.grade = field.grade
				case "energy_current_month":
					want.offset = 32116
					want.grade = GradeHypothesis
				case "energy_current_year":
					want.offset = 32118
					want.grade = GradeHypothesis
				}
				if !reflect.DeepEqual(field, want) {
					t.Fatalf("%s field %+v", name, field)
				}
			}
			var energy SUN2000ReadStep
			for _, step := range profile.Plan(1) {
				if step.Offset == 32106 {
					energy = step
				}
			}
			if energy.Function != 3 || energy.Quantity != 14 || energy.Offset != 32106 {
				t.Fatalf("energy read %+v", energy)
			}
			if profile.MeterFactsAdmitted() || profile.BatteryFactsAdmitted() {
				t.Fatal("profile admits a meter or a battery")
			}
			decoded := periodDecode(t, profile, words)
			if decoded.MeterFactsAdmitted() || decoded.BatteryFactsAdmitted() {
				t.Fatal("reading admits a meter or a battery")
			}
			if len(decoded.Facts()) != len(profile.fields) {
				t.Fatalf("facts %d fields %d", len(decoded.Facts()), len(profile.fields))
			}
			for name, text := range wantText {
				field, _ := periodField(profile, name)
				fact, ok := decoded.Fact(profile.decoderKey(field), 0)
				if !ok || fact.Availability != FactValue || fact.Value.String() != text || fact.Unit != "kWh" || fact.State != "" {
					t.Fatalf("%s fact %+v", name, fact)
				}
				if _, ok := decoded.Fact(profile.decoderKey(field), 1); ok {
					t.Fatalf("%s accepted a string index", name)
				}
				if name == "energy_current_month" || name == "energy_current_year" {
					if fact.Grade != GradeHypothesis {
						t.Fatalf("%s grade %s", name, fact.Grade)
					}
				}
			}
			hole := append([]uint16(nil), words...)
			hole[2] ^= 0x00FF
			if !reflect.DeepEqual(decoded.Facts(), periodDecode(t, profile, hole).Facts()) {
				t.Fatal("an unnamed word changed a fact")
			}
			partial := SUN2000GroupReading{Offset: 32106, Quantity: 14, Outcome: SUN2000GroupAnswered, Words: append([]uint16(nil), words[:10]...)}
			if _, err := profile.Decode(1, []SUN2000GroupReading{partial}); err == nil {
				t.Fatal("a partial energy group published counters")
			}
			failed, err := profile.Decode(1, []SUN2000GroupReading{{Offset: 32106, Quantity: 14, Outcome: SUN2000GroupFailed}})
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"energy_current_month", "energy_current_year"} {
				field, _ := periodField(profile, name)
				fact, ok := failed.Fact(profile.decoderKey(field), 0)
				if !ok || fact.Availability != FactUnknown || fact.Value != (Decimal{}) || fact.Grade != GradeUnknown || fact.State != "" {
					t.Fatalf("unanswered %s published %+v", name, fact)
				}
			}
			kept := RetainHuaweiSUN2000Monitoring(decoded, failed)
			for _, name := range []string{"energy_current_month", "energy_current_year"} {
				field, _ := periodField(profile, name)
				fact, _ := kept.Fact(profile.decoderKey(field), 0)
				if fact.Availability != FactValue || fact.Value.String() != wantText[name] || !fact.Retained {
					t.Fatalf("retained %s %+v", name, fact)
				}
			}
			fenced := decoded
			fenced.unitID = 2
			cleared := RetainHuaweiSUN2000Monitoring(fenced, failed)
			for _, name := range []string{"energy_current_month", "energy_current_year"} {
				field, _ := periodField(profile, name)
				fact, _ := cleared.Fact(profile.decoderKey(field), 0)
				if fact.Availability != FactUnknown || fact.Retained {
					t.Fatalf("unit collision retained %s %+v", name, fact)
				}
			}
		})
	}
}

// TestHuaweiUnnamedEnergyWordsStayUnconsumed covers the loss at 32108 quantity 4.
// Each assertion names the production mutation it catches:
//   - detail text: the loss again names a DC input energy counter, a scale, a sentinel or a statistic
//   - offset or quantity: the hole moves or changes width
//   - kind: the hole becomes a refused group or a consumed fact
//   - MeterFactsAdmitted: an inverter profile admits a meter value
//   - a zero word at a battery offset decodes: the word becomes a battery value
func TestHuaweiUnnamedEnergyWordsStayUnconsumed(t *testing.T) {
	for _, profile := range SUN2000MonitoringProfiles() {
		profile := profile
		t.Run(profile.ID(), func(t *testing.T) {
			var hole ProviderLoss
			for _, loss := range huaweiChildProfileLoss(profile) {
				if loss.Offset == 32108 {
					hole = loss
				}
			}
			want := ProviderLoss{Kind: ProviderLossUnconsumedWords, Offset: 32108, Quantity: 4, Detail: "no numeric source field detected in interval"}
			if hole != want {
				t.Fatalf("hole loss %+v", hole)
			}
			if profile.MeterFactsAdmitted() || profile.BatteryFactsAdmitted() {
				t.Fatal("profile admits a meter or a battery")
			}
			if _, err := profile.Decode(1, []SUN2000GroupReading{{Offset: 37000, Quantity: 1, Outcome: SUN2000GroupAnswered, Words: []uint16{0}}}); err == nil {
				t.Fatal("a zero battery word decoded")
			}
		})
	}
}

func periodField(profile SUN2000MonitoringProfile, name string) (huaweiSUN2000Field, bool) {
	for _, field := range profile.fields {
		if field.name == name {
			return field, true
		}
	}
	return huaweiSUN2000Field{}, false
}

func otherKey(profile SUN2000MonitoringProfile, otherID string, field huaweiSUN2000Field) string {
	saved := profile.id
	profile.id = otherID
	key := profile.decoderKey(field)
	profile.id = saved
	return key
}

func periodDecode(t *testing.T, profile SUN2000MonitoringProfile, words []uint16) SUN2000MonitoringReading {
	t.Helper()
	decoded, err := profile.Decode(1, []SUN2000GroupReading{{
		Offset: 32106, Quantity: uint16(len(words)), Outcome: SUN2000GroupAnswered, Words: append([]uint16(nil), words...),
	}})
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}
