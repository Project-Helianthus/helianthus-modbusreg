package huawei

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestEMMASiteCyclePublishesIndependentHouseholdCounters(t *testing.T) {
	clock := newH06Clock()
	session := newH06Session("emma", clock, &h06Recorder{}, emmaSiteIdentityScript()...)
	session.Append(emmaSiteScript(0x8123, 0x4567, 0x8234, 0x5678, 0x9abc, 0xdef0)...)
	session.Append(emmaUncoveredChildIdentityScript()...)
	session.Append(emmaUncoveredChildGroups()...)
	session.Append(emmaSiteScript(0, 113, 0x8234, 0x5678, 0x9abc, 0xe019)...)
	session.Append(emmaUncoveredChildGroups()...)
	provider, err := NewEMMAReadOnlyProvider(h06Config(clock, newH06Waits(clock), []uint8{3, 4}))
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Start(session); err != nil {
		t.Fatal(err)
	}

	first := refreshEMMASite(t, provider)
	if first.PlannedReads != 19 || first.WireAttempts != 19 {
		t.Fatalf("initial request count: %d/%d", first.PlannedReads, first.WireAttempts)
	}
	firstEpoch := first.StartedAt.Add(4*750*time.Millisecond + 5*20*time.Millisecond)
	requireEMMAEnergy(t, first, "gateway-self.30324", 30324, "U32", 0x81234567, 1, firstEpoch)
	requireEMMAEnergy(t, first, "gateway-self.30326", 30326, "U64", 0x823456789abcdef0, 1, firstEpoch)
	requireEMMAOtherSiteFacts(t, first)
	requireEMMANativeChild(t, first)

	clock.Advance(24 * time.Hour)
	second := refreshEMMASite(t, provider)
	if second.PlannedReads != 11 || second.WireAttempts != 11 {
		t.Fatalf("rollover request count: %d/%d", second.PlannedReads, second.WireAttempts)
	}
	requireEMMAEnergy(t, second, "gateway-self.30324", 30324, "U32", 113, 2, second.StartedAt.Add(20*time.Millisecond))
	requireEMMAEnergy(t, second, "gateway-self.30326", 30326, "U64", 0x823456789abce019, 2, second.StartedAt.Add(20*time.Millisecond))
	requireEMMANativeChild(t, second)
	if session.Remaining() != 0 || len(session.Unexpected()) != 0 {
		t.Fatalf("requests: %d not sent, unexpected %v", session.Remaining(), session.Unexpected())
	}
}

func emmaSiteIdentityScript() []h06Expected {
	return emmaSiteIdentityWithChild("SUN2000-SYNTHETIC")
}

func emmaSiteIdentityWithChild(model string) []h06Expected {
	self := "1=EMMA-A01;2=V100R025C00SPC102;5=0;8=HEMS"
	child := "1=" + model + ";5=3;8=SUN2000"
	return []h06Expected{
		emmaTextReply(0, 30000, 15, "SmartHEMS"),
		emmaTextReply(0, 30222, 20, "EMMA-A01"),
		emmaTextReply(0, 30035, 15, "SmartHEMS V100R025C00SPC102"),
		h06FC2B(0, 3, 0x87, h06Answer{page: &IdentificationPage{
			ReadDeviceIDCode: 3, Conformity: 3, DeclaredObjects: 3,
			ResponseBytes: 7 + 7 + len(self) + len(child),
			Objects: []GatewayInventoryObject{{ObjectID: 0x87, Value: []byte{2}},
				{ObjectID: 0x88, Value: []byte(self)}, {ObjectID: 0x89, Value: []byte(child)}},
		}}),
	}
}

func emmaTextReply(unit uint8, offset, quantity uint16, text string) h06Expected {
	block := make([]byte, 2*int(quantity))
	copy(block, text)
	return h06FC03(unit, offset, quantity, h06Answer{words: h06BlockWords(block)})
}

func emmaSiteScript(dayHigh, dayLow, total0, total1, total2, total3 uint16) []h06Expected {
	return append([]h06Expected{
		h06FC03(0, 30306, 46, h06Answer{words: []uint16{
			0, 201, 0, 1, 2, 202, 0, 203, 0, 3, 4, 204, 41, 53, 67, 79, 0, 4321,
			dayHigh, dayLow, total0, total1, total2, total3,
			0, 205, 0, 5, 6, 206, 0, 207, 0, 7, 8, 208, 83, 97, 101, 103, 0, 209, 0, 9, 10, 210,
		}}),
		h06FC03(0, 30354, 12, h06Answer{words: []uint16{0, 1234, 0, 2345, 0xffff, 0xf16c, 0, 4567, 4, 99, 0xffff, 0xe9d2}}),
		h06FC03(0, 30368, 1, h06Answer{words: []uint16{7890}}),
	}, emmaMeterReplies()...)
}

func emmaUncoveredChildIdentityScript() []h06Expected {
	return []h06Expected{
		emmaTextReply(3, 30000, 15, "SUN2000-SYNTHETIC"),
		h06FC03(3, 30070, 1, h06Answer{words: []uint16{65000}}),
		emmaTextReply(3, 30050, 15, "V100R001C00SPC100"),
		emmaTextReply(3, 30035, 15, "V100R002C00SPC100"),
	}
}

func emmaUncoveredChildGroups() []h06Expected {
	status, strings, energy := make([]uint16, 16), make([]uint16, 24), make([]uint16, 10)
	status[0], strings[1], energy[9] = 0x0200, 456, 789
	return []h06Expected{
		h06FC03(3, 32000, 16, h06Answer{words: status}),
		h06FC03(3, 32016, 24, h06Answer{words: strings}),
		h06FC03(3, 32106, 10, h06Answer{words: energy}),
	}
}

func refreshEMMASite(t *testing.T, provider *EMMAReadOnlyProvider) ProviderCycle {
	t.Helper()
	cycle, err := provider.Refresh(context.Background())
	if err != nil || cycle.Outcome != ProviderCycleComplete {
		t.Fatalf("cycle outcome %s (%s): %v", cycle.Outcome, cycle.Reason, err)
	}
	return cycle
}

func requireEMMAEnergy(t *testing.T, cycle ProviderCycle, key string, offset uint16, typ string, coefficient uint64, epoch uint64, readAt time.Time) {
	t.Helper()
	fact, ok := cycle.EMMASiteFact(key)
	group := cycle.EMMASite[0]
	if !ok || fact.Key != key || fact.Offset != offset || fact.Unit != "kWh" || fact.Type != typ || fact.Unsigned != coefficient ||
		fact.Signed != 0 || fact.Exponent != -2 || fact.Availability != FactValue || fact.Grade != GradeHypothesis || fact.PolarityUnknown ||
		group.Outcome != SUN2000GroupAnswered || group.Retained || group.Cycle != epoch || !group.ReadAt.Equal(readAt) ||
		group.Generation != 1 || group.Offset != 30306 || group.Quantity != 46 || group.UnitID != 0 {
		t.Fatalf("%s: %+v, epoch/validity: %+v", key, fact, group.ProviderGroupEvidence)
	}
}

func requireEMMAOtherSiteFacts(t *testing.T, cycle ProviderCycle) {
	t.Helper()
	wants := []EMMASiteFact{
		{Key: "gateway-self.30306", Offset: 30306, Type: "U32", Unit: "kWh", Exponent: -2, Unsigned: 201},
		{Key: "gateway-self.30308", Offset: 30308, Type: "U64", Unit: "kWh", Exponent: -2, Unsigned: 0x1000200ca},
		{Key: "gateway-self.30312", Offset: 30312, Type: "U32", Unit: "kWh", Exponent: -2, Unsigned: 203},
		{Key: "gateway-self.30314", Offset: 30314, Type: "U64", Unit: "kWh", Exponent: -2, Unsigned: 0x3000400cc},
		{Key: "gateway-self.30322", Offset: 30322, Type: "U32", Unit: "kWh", Exponent: -3, Unsigned: 4321},
		{Key: "gateway-self.30330", Offset: 30330, Type: "U32", Unit: "kWh", Exponent: -2, Unsigned: 205},
		{Key: "gateway-self.30332", Offset: 30332, Type: "U64", Unit: "kWh", Exponent: -2, Unsigned: 0x5000600ce},
		{Key: "gateway-self.30336", Offset: 30336, Type: "U32", Unit: "kWh", Exponent: -2, Unsigned: 207},
		{Key: "gateway-self.30338", Offset: 30338, Type: "U64", Unit: "kWh", Exponent: -2, Unsigned: 0x7000800d0},
		{Key: "gateway-self.30346", Offset: 30346, Type: "U32", Unit: "kWh", Exponent: -2, Unsigned: 209},
		{Key: "gateway-self.30348", Offset: 30348, Type: "U64", Unit: "kWh", Exponent: -2, Unsigned: 0x9000a00d2},
		{Key: "gateway-self.30354", Offset: 30354, Type: "U32", Unit: "kW", Exponent: -3, Unsigned: 1234},
		{Key: "gateway-self.30356", Offset: 30356, Type: "U32", Unit: "kW", Exponent: -3, Unsigned: 2345},
		{Key: "gateway-self.30358", Offset: 30358, Type: "I32", Unit: "kW", Exponent: -3, Signed: -3732, PolarityUnknown: true},
		{Key: "gateway-self.30360", Offset: 30360, Type: "I32", Unit: "kW", Exponent: -3, Signed: 4567, PolarityUnknown: true},
		{Key: "gateway-self.30364", Offset: 30364, Type: "I32", Unit: "kW", Exponent: -3, Signed: -5678, PolarityUnknown: true},
		{Key: "gateway-self.30368", Offset: 30368, Type: "U16", Unit: "%", Exponent: -2, Unsigned: 7890},
	}
	for _, want := range wants {
		want.Availability, want.Grade = FactValue, GradeHypothesis
		if got, ok := cycle.EMMASiteFact(want.Key); !ok || got != want {
			t.Errorf("%s: got %+v, want %+v", want.Key, got, want)
		}
	}
	if len(cycle.EMMASite) != 3 || len(cycle.EMMASite[0].Facts)+len(cycle.EMMASite[1].Facts)+len(cycle.EMMASite[2].Facts) != len(wants)+2 {
		t.Fatal("unexpected site group or fact count")
	}
}

func requireEMMANativeChild(t *testing.T, cycle ProviderCycle) {
	t.Helper()
	child, ok := cycle.Child(3)
	if !ok || child.Catalog == nil || child.Catalog.Reason() != SUN2000CatalogNoCatalogRow || child.ProfileID != "" ||
		child.IdentifiedGeneration != 1 || child.Model != "SUN2000-SYNTHETIC" || child.Evidence != ProviderEvidenceCurrent ||
		child.Reason != ProviderReasonNoProfile || len(child.Reading.Facts()) != 0 {
		t.Fatalf("uncovered child: %+v", child)
	}
	groups := child.Reading.Groups()
	for i, want := range emmaUncoveredChildGroups() {
		if len(groups) != 3 || groups[i].Offset != want.offset || groups[i].Quantity != want.quantity ||
			groups[i].Outcome != SUN2000GroupAnswered || groups[i].Cycle != cycle.Cycle || !reflect.DeepEqual(groups[i].Words, want.answer.words) {
			t.Fatalf("uncovered child native groups: %+v", groups)
		}
	}
	unlisted, ok := cycle.Child(4)
	if !ok || unlisted.Reason != ProviderReasonNotListed || unlisted.Evidence != ProviderEvidenceWithheld {
		t.Fatalf("unlisted child: %+v", unlisted)
	}
}

func TestEMMASiteDispositionDeclaresFamilyMonitoring(t *testing.T) {
	record := decodeHuaweiObject(t, "../profiles/vendors/huawei/emma-disposition.json")
	monitoring := huaweiObject(t, record["monitoring"])
	if monitoring["scope"] != "EVERY_DECLARED_FAMILY" || monitoring["status"] != "MONITORING_BY_CONTRACT" ||
		monitoring["provider_id"] != EMMAReadOnlyProviderID || monitoring["uncovered_child"] != "NATIVE_ONLY" || monitoring["battery_fallback"] != "SITE_LEVEL_ONLY" {
		t.Fatalf("EMMA monitoring disposition: %#v", monitoring)
	}
}

func TestEMMASiteCycleKeepsEpochOfMalformedGroup(t *testing.T) {
	clock := newH06Clock()
	session := newH06Session("emma", clock, &h06Recorder{}, emmaSiteIdentityScript()...)
	session.Append(emmaSiteScript(0, 551, 1, 2, 3, 4)...)
	failed := h06FC03(0, 30306, 46, h06Answer{words: []uint16{17}})
	session.Append(failed, failed, failed, failed)
	session.Append(h06FC03(0, 30354, 12, h06Answer{words: []uint16{0, 987, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}))
	session.Append(h06FC03(0, 30368, 1, h06Answer{err: h06Exception(3, 3)}))
	session.Append(emmaMeterReplies()...)
	provider := startEMMASite(t, clock, session, nil)
	first := refreshEMMASite(t, provider)
	want := first.EMMASite[0].clone()
	first.EMMASite[0].Words[0] = 0xffff
	first.EMMASite[0].Facts[0].Unsigned = 0xffff
	last, ok := provider.LastComplete()
	if !ok || !reflect.DeepEqual(last.EMMASite[0], want) {
		t.Fatal("published site data aliases the retained cycle")
	}
	clock.Advance(time.Minute)
	second := refreshEMMASite(t, provider)
	kept := second.EMMASite[0]
	if kept.Outcome != SUN2000GroupFailed || !kept.Retained || kept.Cycle != want.Cycle || kept.Generation != want.Generation ||
		kept.ReadAt != want.ReadAt || !reflect.DeepEqual(kept.Words, want.Words) || !reflect.DeepEqual(kept.Facts, want.Facts) {
		t.Fatalf("malformed read replaced or refreshed the retained group: %+v", kept)
	}
	power, _ := second.EMMASiteFact("gateway-self.30354")
	if power.Unsigned != 987 || second.EMMASite[1].Retained || second.EMMASite[1].Cycle != 2 ||
		!second.EMMASite[2].Refused || second.EMMASite[2].ExceptionCode != 3 || second.EMMASite[2].Cycle != 1 {
		t.Fatalf("site sibling outcomes: %+v", second.EMMASite[1:])
	}
	if len(second.Loss) < 2 || second.Loss[0].Kind != ProviderLossFailedGroup || second.Loss[0].UnitID != 0 || second.Loss[0].Offset != 30306 ||
		second.Loss[1].Kind != ProviderLossRefusedGroup || second.Loss[1].Offset != 30368 || session.Remaining() != 0 || len(session.Unexpected()) != 0 {
		t.Fatalf("losses: %+v, unexpected requests: %v", second.Loss, session.Unexpected())
	}
}

func TestEMMASiteCycleRejectsConflictingGatewayIdentity(t *testing.T) {
	for _, tc := range []struct {
		name  string
		index int
		text  string
	}{
		{"offering", 0, "Synthetic other family"},
		{"model", 1, "Synthetic other model"},
		{"software", 2, "SmartHEMS V100R025C00SPC001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clock := newH06Clock()
			script := emmaSiteIdentityScript()[:3]
			step := script[tc.index]
			script[tc.index] = emmaTextReply(step.unit, step.offset, step.quantity, tc.text)
			session := newH06Session("emma", clock, &h06Recorder{}, script...)
			provider := startEMMASite(t, clock, session, []uint8{3})
			cycle, err := provider.Refresh(context.Background())
			if err != nil || cycle.Outcome != ProviderCycleInsufficient || cycle.Reason != ProviderReasonGatewayMismatch ||
				session.Remaining() != 0 || len(session.Unexpected()) != 0 {
				t.Fatalf("monitoring followed a gateway mismatch: %s, %s, %v, %v", cycle.Outcome, cycle.Reason, err, session.Unexpected())
			}
			day, ok := cycle.EMMASiteFact("gateway-self.30324")
			if !ok || day.Availability != FactUnknown || cycle.EMMASite[0].Cycle != 0 || len(cycle.EMMASite[0].Words) != 0 {
				t.Fatalf("gateway mismatch emitted a reading: %+v", cycle.EMMASite[0])
			}
		})
	}
}

func TestEMMASiteInventoryCorroboratesEachSuppliedField(t *testing.T) {
	const model, software, product = "family-variant", "build", "HEMS"
	for _, tc := range []struct {
		name     string
		self     GatewayInventoryChild
		conflict bool
	}{
		{"match", GatewayInventoryChild{model: model, softwareVersion: software, productType: product}, false},
		{"model", GatewayInventoryChild{model: "other", softwareVersion: software, productType: product}, true},
		{"software", GatewayInventoryChild{model: model, softwareVersion: "other", productType: product}, true},
		{"product", GatewayInventoryChild{model: model, softwareVersion: software, productType: "other"}, true},
		{"software omitted", GatewayInventoryChild{model: model, productType: product}, false},
		{"product omitted", GatewayInventoryChild{model: model, softwareVersion: software}, false},
	} {
		if got := emmaSelfConflicts(tc.self, model, software, product); got != tc.conflict {
			t.Errorf("%s: conflict %t, want %t", tc.name, got, tc.conflict)
		}
	}
}

func TestEMMASiteCycleSelectsChildByCatalogIdentifier(t *testing.T) {
	for _, tc := range []struct {
		name       string
		identifier uint16
		want       ProviderReason
	}{
		{"declared identifier", 471, ProviderReasonRead},
		{"different identifier", 472, ProviderReasonCatalogNotSelected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clock := newH06Clock()
			model := huaweiSUN2000M3Model
			session := newH06Session("emma", clock, &h06Recorder{}, emmaSiteIdentityWithChild(model)...)
			session.Append(emmaSiteScript(0, 1, 0, 0, 1, 2)...)
			session.Append(emmaTextReply(3, 30000, 15, model), h06FC03(3, 30070, 1, h06Answer{words: []uint16{tc.identifier}}),
				emmaTextReply(3, 30050, 15, "V200R023C00SPC128"), emmaTextReply(3, 30035, 15, "V200R023C00SPC100"))
			if tc.want == ProviderReasonRead {
				for _, step := range catalogProfileByCard(t, "MC").Plan(3) {
					words := make([]uint16, step.Quantity)
					if step.Offset == 32106 {
						words[1] = 123
					}
					session.Append(h06FC03(3, step.Offset, step.Quantity, h06Answer{words: words}))
				}
			}
			cycle := refreshEMMASite(t, startEMMASite(t, clock, session, []uint8{3}))
			child, ok := cycle.Child(3)
			if !ok || child.Reason != tc.want || child.Catalog == nil || session.Remaining() != 0 || len(session.Unexpected()) != 0 {
				t.Fatalf("child: %s, expected %s, unexpected requests %v", child.Reason, tc.want, session.Unexpected())
			}
			if tc.want == ProviderReasonRead {
				fact, present := child.Reading.Fact("huawei.sun2000.m3.energy_accumulated", 0)
				if child.ProfileID != SUN2000M3MonitoringProfileID || !present || fact.Value != (Decimal{Coefficient: 123, Exponent: -2}) || fact.Availability != FactValue {
					t.Fatalf("catalogue-selected energy fact: %+v", fact)
				}
			} else if child.Catalog.Reason() != SUN2000CatalogIdentifierMismatch || len(child.Reading.Groups()) != 0 {
				t.Fatalf("conflicting identifier allowed reads: %+v", child)
			}
		})
	}
}

func startEMMASite(t *testing.T, clock *h06Clock, session *h06Session, units []uint8) *EMMAReadOnlyProvider {
	t.Helper()
	provider, err := NewEMMAReadOnlyProvider(h06Config(clock, newH06Waits(clock), units))
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Start(session); err != nil {
		t.Fatal(err)
	}
	return provider
}
