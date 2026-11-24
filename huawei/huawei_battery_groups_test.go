package huawei

import (
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"
)

// Independent FC03 extents for a complete storage card. No request spans an
// undocumented gap or includes battery identity or firmware text.
var batteryTestExtents = [][2]uint16{
	{37000, 5}, {37006, 5}, {37014, 5}, {37021, 2}, {37025, 1}, {37046, 4}, {37066, 4},
	{37741, 1}, {37743, 2}, {37746, 11}, {37758, 3}, {37762, 5}, {37780, 8}, {37920, 6},
	{38228, 2}, {38233, 4}, {38238, 4}, {38270, 2}, {38275, 4}, {38280, 4},
	{38312, 2}, {38317, 4}, {38322, 4}, {38354, 2}, {38359, 4}, {38364, 4},
	{38396, 2}, {38401, 4}, {38406, 4}, {38438, 2}, {38443, 4}, {38448, 4},
	{38452, 2}, {38454, 2}, {38456, 2}, {38458, 2}, {38460, 2}, {38462, 2},
}

func TestHuaweiBatteryCataloguePlans(t *testing.T) {
	plans := map[string][][2]uint16{
		"L-V200": batteryTestExtents, "LB-C00": batteryTestExtents, "LB-C10": batteryTestExtents,
		"LC": batteryTestExtents, "MA-C00": batteryTestExtents, "MA-V200": batteryTestExtents,
		"L-LEGACY": {{37001, 2}, {37004, 1}, {37015, 4}},
		"SUN-V300": {{37000, 3}, {37015, 4}},
		"MC":       {{37000, 1}}, "MA-C20": {{37000, 1}},
	}
	profiles, err := CatalogMonitoringProfiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range profiles {
		for _, unit := range []uint8{3, 7} {
			var got [][2]uint16
			for _, step := range profile.Plan(unit) {
				if step.Offset >= 37000 && step.Offset <= 38463 {
					if step.Function != 3 || step.UnitID != unit {
						t.Fatalf("%s: incorrect routed FC03: %+v", profile.Card(), step)
					}
					got = append(got, [2]uint16{step.Offset, step.Quantity})
				}
			}
			if want := plans[profile.Card()]; !reflect.DeepEqual(got, want) {
				t.Errorf("%s: extents %v, want %v", profile.Card(), got, want)
			}
		}
	}
	requireBatteryCatalogueVariants(t, profiles)
}

// Compare all named storage codes, including codes that stay native at runtime.
func requireBatteryCatalogueVariants(t *testing.T, profiles []CatalogMonitoringProfile) {
	t.Helper()
	data, err := os.ReadFile("../profiles/vendors/huawei/catalog/dictionaries.json")
	if err != nil {
		t.Fatal(err)
	}
	var labels catalogDictFile
	if err := decodeHuaweiSUN2000CatalogFile(data, &labels); err != nil {
		t.Fatal(err)
	}
	for _, profile := range profiles {
		want := map[uint16]map[uint16]string{}
		for _, item := range labels.Dictionaries {
			if item.Card != profile.Card() || item.Kind != "value_enum" {
				continue
			}
			for _, offset := range item.Offsets {
				if offset < 37000 || offset > 37786 || offset > 37068 && offset < 37741 {
					continue
				}
				codes := map[uint16]string{}
				for _, value := range item.Values {
					code, err := strconv.ParseUint(value.Code, 10, 16)
					if err != nil {
						t.Fatal(err)
					}
					codes[uint16(code)] = value.Meaning
				}
				want[uint16(offset)] = codes
			}
		}
		if len(want) != len(profile.battery.states) || len(want) != 0 && !reflect.DeepEqual(want, profile.battery.states) {
			t.Errorf("%s: storage labels %v, want %v", profile.Card(), profile.battery.states, want)
		}
	}

	profile := catalogProfileByCard(t, "MA-V200")
	reading, err := profile.Decode(7, []SUN2000GroupReading{
		{Offset: 37000, Quantity: 5, Outcome: SUN2000GroupAnswered, Words: []uint16{1, 0, 801, 101, 524}},
		{Offset: 37006, Quantity: 5, Outcome: SUN2000GroupAnswered, Words: []uint16{7, 0, 401, 0, 303}},
		{Offset: 37014, Quantity: 5, Outcome: SUN2000GroupAnswered, Words: []uint16{9, 0, 12, 0, 13}},
		{Offset: 38438, Quantity: 2, Outcome: SUN2000GroupAnswered, Words: []uint16{3, 451}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fact, ok := reading.Fact("huawei.sun2000.ma-v200.storage.37000", 0); !ok || fact.State != "standby" || fact.Availability != FactValue {
		t.Fatalf("MA-V200 running state: %+v", fact)
	}
	if fact, ok := reading.Fact("huawei.sun2000.ma-v200.storage.38439", 0); !ok || fact.Availability != FactValue || fact.Value.String() != "45.1" {
		t.Fatalf("MA-V200 unit 2, pack 3: %+v", fact)
	}
	for _, offset := range []string{"37006", "37014", "38438"} {
		if fact, ok := reading.Fact("huawei.sun2000.ma-v200.storage."+offset, 0); ok {
			t.Errorf("MA-V200 native-only row promoted: %+v", fact)
		}
	}
	if _, ok := reading.Fact("huawei.sun2000.ma-v200.storage.38439", 1); ok {
		t.Fatal("fact lookup ignored the scalar index")
	}
}

func TestHuaweiBatteryChildCycle(t *testing.T) {
	t.Run("dongle", requireDongleBatteryChild)
	t.Run("energy manager", requireEMMABatteryChild)
}

func requireEMMABatteryChild(t *testing.T) {
	t.Helper()
	clock := newH06Clock()
	model := huaweiSDongleObservedChildModel
	session := newH06Session("emma", clock, &h06Recorder{}, emmaSiteIdentityWithChild(model)...)
	session.Append(emmaSiteScript(0, 1, 0, 0, 1, 2)...)
	session.Append(emmaTextReply(3, 30000, 15, model), h06FC03(3, 30070, 1, h06Answer{words: []uint16{428}}),
		emmaTextReply(3, 30050, 15, "V100R001C00SPC100"), emmaTextReply(3, 30035, 15, "V100R001C00SPC100"))
	profile := catalogProfileByCard(t, "MA-C00")
	ordinary := batteryTestOrdinaryReplies(profile)
	session.Append(ordinary...)
	session.Append(batteryTestReplies(3, map[uint16]uint16{
		37000: 2, 37001: 0xffff, 37002: 0x4321, 37003: 3812, 37004: 713,
		37006: 7, 37007: 1, 37008: 0x1234, 37021: 0xff9c,
		37068: 1, 37069: 0x3456, 37741: 3, 37758: 1, 37759: 0x1235,
		37760: 729, 37762: 1, 37786: 2, 37787: 0x6543,
		38228: 9, 38229: 688, 38233: 0xffff, 38234: 0x7fff,
		38271: 645, 38313: 619, 38355: 578, 38397: 541, 38439: 501, 38463: 0xfff8,
	}, 0)...)
	provider := startEMMASite(t, clock, session, []uint8{3})
	first := refreshEMMASite(t, provider)
	child, ok := first.Child(3)
	if !ok || child.ProfileID != SUN2000M1MonitoringProfileID || child.Reason != ProviderReasonRead || !child.Reading.BatteryFactsAdmitted() {
		t.Fatalf("qualifying child did not receive battery facts: %s %s", child.ProfileID, child.Reason)
	}
	requireBatteryTestValues(t, child.Reading, first.Cycle)
	for _, offset := range []uint16{37920, 38228} {
		if fact, ok := child.Reading.Fact(batteryTestKey(offset), 0); ok {
			t.Fatalf("raw-only row promoted: %+v", fact)
		}
	}
	for _, group := range child.Reading.Groups() {
		if group.Offset == 38228 && (len(group.Words) != 2 || group.Words[0] != 9) {
			t.Fatalf("raw pack status changed: %+v", group)
		}
	}
	running := batteryTestGroup(t, child, 37000)
	clock.Advance(time.Minute)
	session.Append(emmaSiteScript(0, 2, 0, 0, 2, 3)...)
	session.Append(ordinary...)
	session.Append(batteryTestReplies(3, map[uint16]uint16{37760: 811}, 37000)...)
	second := refreshEMMASite(t, provider)
	child, _ = second.Child(3)
	kept := batteryTestGroup(t, child, 37000)
	if !kept.Retained || !kept.Refused || kept.ExceptionCode != 3 || kept.Cycle != running.Cycle || kept.ReadAt != running.ReadAt || kept.Generation != running.Generation {
		t.Fatalf("unit refusal lost the retained epoch: before %+v, after %+v", running, kept)
	}
	old, _ := child.Reading.Fact(batteryTestKey(37004), 0)
	sibling, _ := child.Reading.Fact(batteryTestKey(37760), 0)
	zero, _ := child.Reading.Fact(batteryTestKey(38229), 0)
	if child.Evidence != ProviderEvidenceStale || !old.Retained || old.Cycle != first.Cycle || old.Value.Coefficient != 713 ||
		sibling.Retained || sibling.Cycle != second.Cycle || sibling.Value.Coefficient != 811 || zero.Availability != FactUnknown {
		t.Fatalf("partial cycle: retained %+v, current %+v, zero %+v", old, sibling, zero)
	}
	clock.Advance(time.Minute)
	session.Append(emmaSiteScript(0, 3, 0, 0, 3, 4)...)
	session.Append(ordinary...)
	session.Append(batteryTestReplies(3, nil, 0)[1:]...)
	third := refreshEMMASite(t, provider)
	child, _ = third.Child(3)
	if group := batteryTestGroup(t, child, 37000); group.Outcome != SUN2000GroupNotRead || !group.Refused || !group.Retained || group.ReadAt != running.ReadAt {
		t.Fatalf("refused group was re-read or lost its original age: %+v", group)
	}
	if session.Remaining() != 0 || len(session.Unexpected()) != 0 {
		t.Fatalf("script: %d remaining, unexpected %v", session.Remaining(), session.Unexpected())
	}
}

func requireDongleBatteryChild(t *testing.T) {
	t.Helper()
	fixture, start := h06FixtureCycle(t, "sdongle_provider_cycles.json", "start_identification")
	rig := newH06Rig(t, fixture.Path, []uint8{1})
	script := withSyntheticPlantReads(fixture.Path, scriptWithoutPlantReads(t, rig.sources, fixture.Path, start))
	script = append(script, batteryTestReplies(1, map[uint16]uint16{37004: 507, 37760: 812, 38229: 606}, 0)...)
	cycle := rig.refresh(t, append(script, dongleMeterReplies()...)...)
	child, ok := cycle.Child(1)
	if !ok || child.Reason != ProviderReasonRead || !child.Reading.BatteryFactsAdmitted() {
		t.Fatalf("dongle did not read child storage: %s", child.Reason)
	}
	for offset, coefficient := range map[uint16]int64{37004: 507, 37760: 812, 38229: 606} {
		fact, ok := child.Reading.Fact(batteryTestKey(offset), 0)
		if !ok || fact.Availability != FactValue || fact.Value != (Decimal{Coefficient: coefficient, Exponent: -1}) {
			t.Errorf("child row %d: %+v", offset, fact)
		}
	}
}

func TestHuaweiBatteryZeroAndSiteOnly(t *testing.T) {
	clock := newH06Clock()
	model := huaweiSDongleObservedChildModel
	session := newH06Session("emma", clock, &h06Recorder{}, emmaSiteIdentityWithChild(model)...)
	session.Append(emmaSiteScript(0, 1, 0, 0, 1, 2)...)
	session.Append(emmaTextReply(3, 30000, 15, model), h06FC03(3, 30070, 1, h06Answer{words: []uint16{428}}),
		emmaTextReply(3, 30050, 15, "V100R001C00SPC100"), emmaTextReply(3, 30035, 15, "V100R001C00SPC100"))
	session.Append(batteryTestOrdinaryReplies(catalogProfileByCard(t, "MA-C00"))...)
	session.Append(batteryTestReplies(3, nil, 0)...)
	cycle := refreshEMMASite(t, startEMMASite(t, clock, session, []uint8{3}))
	child, ok := cycle.Child(3)
	if !ok || child.Reason != ProviderReasonRead || child.Reading.BatteryFactsAdmitted() || session.Remaining() != 0 || len(session.Unexpected()) != 0 {
		t.Fatalf("zero-only qualified child: %s, unexpected %v", child.Reason, session.Unexpected())
	}
	for _, offset := range []uint16{37000, 37001, 37004, 37760, 38229, 38463} {
		fact, ok := child.Reading.Fact(batteryTestKey(offset), 0)
		if !ok || fact.Availability != FactUnknown || fact.Grade != GradeUnknown || fact.State != "" {
			t.Fatalf("zero row %d: %+v", offset, fact)
		}
	}
	if group := child.Reading.Groups()[len(child.Groups)-len(batteryTestExtents)]; group.Offset != 37000 || group.Outcome != SUN2000GroupAnswered || group.Words[0] != 0 {
		t.Fatalf("zero raw response not preserved: %+v", group)
	}

	clock = newH06Clock()
	session = newH06Session("emma", clock, &h06Recorder{}, emmaSiteIdentityScript()...)
	session.Append(emmaSiteScript(0, 1, 0, 0, 1, 2)...)
	session.Append(emmaUncoveredChildIdentityScript()...)
	session.Append(emmaUncoveredChildGroups()...)
	cycle = refreshEMMASite(t, startEMMASite(t, clock, session, []uint8{3, 4}))
	requireEMMANativeChild(t, cycle)
	for _, key := range []string{"gateway-self.30322", "gateway-self.30360", "gateway-self.30368"} {
		if fact, ok := cycle.EMMASiteFact(key); !ok || fact.Availability != FactValue {
			t.Fatalf("missing site battery fact %s", key)
		}
	}
	var found bool
	for _, loss := range cycle.Loss {
		if loss.Kind == ProviderLossBattery && loss.HasUnit && loss.UnitID == 3 && loss.Detail == "site level only: no documented inverter-side battery decoder for the child" {
			found = true
		}
	}
	if !found || session.Remaining() != 0 || len(session.Unexpected()) != 0 {
		t.Fatalf("site-only loss or native request plan missing: %+v, unexpected %v", cycle.Loss, session.Unexpected())
	}
}

func batteryTestReplies(unit uint8, values map[uint16]uint16, refuse uint16) []h06Expected {
	var replies []h06Expected
	for _, extent := range batteryTestExtents {
		words := make([]uint16, extent[1])
		for index := range words {
			words[index] = values[extent[0]+uint16(index)]
		}
		answer := h06Answer{words: words}
		if extent[0] == refuse {
			answer = h06Answer{err: h06Exception(3, 3)}
		}
		replies = append(replies, h06FC03(unit, extent[0], extent[1], answer))
	}
	return replies
}

func batteryTestOrdinaryReplies(profile CatalogMonitoringProfile) []h06Expected {
	var replies []h06Expected
	for _, step := range profile.Plan(3) {
		if step.Offset < 37000 || step.Offset > 38463 {
			replies = append(replies, h06FC03(3, step.Offset, step.Quantity, h06Answer{words: make([]uint16, step.Quantity)}))
		}
	}
	return replies
}

func batteryTestKey(offset uint16) string {
	return "huawei.sun2000.m1.storage." + strconv.Itoa(int(offset))
}

func batteryTestGroup(t *testing.T, child ProviderChildObservation, offset uint16) ProviderGroupEvidence {
	t.Helper()
	for _, group := range child.Groups {
		if group.Offset == offset {
			return group
		}
	}
	t.Fatalf("missing child group %d", offset)
	return ProviderGroupEvidence{}
}

func requireBatteryTestValues(t *testing.T, reading SUN2000MonitoringReading, cycle uint64) {
	t.Helper()
	for _, want := range []struct {
		offset      uint16
		coefficient int64
		exponent    int16
		unit, state string
	}{
		{37000, 2, 0, "", "running"}, {37001, -48351, 0, "W", ""}, {37003, 3812, -1, "V", ""},
		{37004, 713, -1, "%", ""}, {37006, 7, 0, "", "remote_self_consumption"},
		{37007, 0x11234, 0, "W", ""}, {37021, -100, -1, "A", ""}, {37068, 0x13456, -2, "kWh", ""},
		{37741, 3, 0, "", "fault"}, {37758, 0x11235, 0, "Wh", ""}, {37760, 729, -1, "%", ""},
		{37762, 1, 0, "", "idle"}, {37786, 0x26543, -2, "kWh", ""},
		{38229, 688, -1, "%", ""}, {38233, -32769, -3, "kW", ""}, {38271, 645, -1, "%", ""},
		{38313, 619, -1, "%", ""}, {38355, 578, -1, "%", ""}, {38397, 541, -1, "%", ""},
		{38439, 501, -1, "%", ""}, {38463, -8, -1, "℃", ""},
	} {
		key := batteryTestKey(want.offset)
		fact, ok := reading.Fact(key, 0)
		expected := SUN2000Fact{Key: key, Unit: want.unit, Availability: FactValue,
			Value: Decimal{Coefficient: want.coefficient, Exponent: want.exponent}, State: want.state, Grade: GradeHypothesis, Cycle: cycle}
		if !ok || fact != expected {
			t.Errorf("row %d: got %+v, want %+v", want.offset, fact, expected)
		}
	}
}
