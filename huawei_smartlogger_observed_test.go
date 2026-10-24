package modbusreg

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const (
	h03Model    = "Smart Logger"
	h03Firmware = "V300R023C10SPC620"
)

// h03MutatedInventory loads the fixture, lets the caller change it, rebuilds
// the wire responses and decodes them into the card input.
func h03MutatedInventory(t testing.TB, mutate func(*h03InventoryFixture)) HuaweiSmartLoggerObservedQualificationInput {
	t.Helper()
	fixture := h03LoadInventory(t)
	if mutate != nil {
		mutate(&fixture)
	}
	return h03ObservedInputFromFixture(t, fixture)
}

// h03ReplaceInSelfEntry rewrites the self entry text. Lengths may change; the
// object keeps its 254-byte extent.
func h03ReplaceInSelfEntry(old, replacement string) func(*h03InventoryFixture) {
	return func(fixture *h03InventoryFixture) {
		object := &fixture.Pages[1].Objects[0]
		if !strings.Contains(object.Entry, old) {
			panic("self entry does not contain " + old)
		}
		object.Entry = strings.Replace(object.Entry, old, replacement, 1)
	}
}

func h03ValidInput(t testing.TB) HuaweiSmartLoggerObservedQualificationInput {
	t.Helper()
	input := h03MutatedInventory(t, nil)
	input.Basic = h03BasicReplyFromFixture(t, h03LoadBasic(t))
	return input
}

// h03RequireControl keeps every negative test from passing vacuously: the
// unmodified fixture tuple must be admitted by the same card.
func h03RequireControl(t *testing.T) {
	t.Helper()
	if result := NewHuaweiSmartLoggerObservedQualificationCard().Evaluate(h03ValidInput(t)); !result.IdentityMatched() || !result.QualificationTestReady() {
		t.Fatalf("positive control failed: %#v", result)
	}
}

func h03RequireRejected(t *testing.T, label string, result HuaweiQualificationResult) {
	t.Helper()
	if result.IdentityMatched() || result.SelectedClass() != "" || result.Readiness() != HuaweiQualificationInsufficientEvidence ||
		result.NativeFactCount() != 0 || result.AutomaticRequestCount() != 0 || result.FallbackAttempted() || !result.DefaultDenied() ||
		result.QualificationTestReady() {
		t.Fatalf("%s: not rejected as INSUFFICIENT_EVIDENCE with no selection: %#v", label, result)
	}
	if inventory, ok := result.Inventory(); ok || inventory.ChildCount() != 0 {
		t.Fatalf("%s: rejected result retained a partial child list", label)
	}
}

// h03OneCharacterMutants returns the substitution, deletion and insertion
// neighbours of value, never the value itself.
func h03OneCharacterMutants(value string) []string {
	seen := map[string]bool{value: true}
	var mutants []string
	add := func(candidate string) {
		if !seen[candidate] {
			seen[candidate] = true
			mutants = append(mutants, candidate)
		}
	}
	swap := func(c byte) byte {
		switch {
		case c >= 'a' && c <= 'z':
			return c - 'a' + 'A'
		case c >= 'A' && c <= 'Z':
			return c - 'A' + 'a'
		case c >= '0' && c <= '8':
			return c + 1
		case c == '9':
			return '0'
		default:
			return '_'
		}
	}
	for index := 0; index < len(value); index++ {
		for _, replacement := range []byte{swap(value[index]), ' ', '~'} {
			add(value[:index] + string(replacement) + value[index+1:])
		}
		add(value[:index] + value[index+1:])
		add(value[:index] + "Z" + value[index:])
	}
	add(value + "Z")
	add(value + " ")
	add(" " + value)
	return mutants
}

func h03SortedUnits(inventory HuaweiGatewayInventory) []string {
	var units []string
	for _, child := range inventory.Children() {
		units = append(units, child.Address())
	}
	sort.Slice(units, func(i, j int) bool {
		if len(units[i]) != len(units[j]) {
			return len(units[i]) < len(units[j])
		}
		return units[i] < units[j]
	})
	return units
}

func TestH03ObservedCardIdentityAndReplayProcedure(t *testing.T) {
	card := NewHuaweiSmartLoggerObservedQualificationCard()
	if card.ID() != "huawei.qualification.smartlogger.observed-readonly@1.0.0" || card.ID() != HuaweiSmartLoggerObservedQualificationCardID ||
		card.ID() == HuaweiSmartLoggerQualificationCardID || card.CandidateClass() != HuaweiSmartLoggerCanonicalClass ||
		card.UnitID() != 0 || card.Vendor() != "HUAWEI" || card.Model() != h03Model || card.Firmware() != h03Firmware || card.SelfEntryUnit() != "0" ||
		!reflect.DeepEqual(card.MissingEvidence(), []string{"pairwise_negative_overlap_with_emma_fixture"}) {
		t.Fatalf("unexpected observed card: %#v", card)
	}
	if limits := card.Limits(); limits != (HuaweiQualificationInventoryLimits{
		DeadlineMilliseconds: 15000, MaxPages: 248, MaxObjects: 248, MaxBytes: 65536, MaxChildren: 247, MaxResponseBytes: 1024,
	}) {
		t.Fatalf("unexpected observed limits: %#v", limits)
	}
	assertHuaweiQualificationSteps(t, card.Steps(), []HuaweiQualificationStep{
		{operation: "FC03_COUNTER_BEFORE", unitID: 0, offset: 65521, quantity: 1},
		{operation: "FC2B_MEI_0E_READDEV03_INVENTORY", unitID: 0, objectID: 0x87},
		{operation: "FC03_COUNTER_AFTER", unitID: 0, offset: 65521, quantity: 1},
		{operation: "FC2B_MEI_0E_READDEV01_BASIC_OPTIONAL", unitID: 0, objectID: 0x00},
	})
	for _, step := range card.Steps() {
		if !strings.HasPrefix(step.Operation(), "FC03_") && !strings.HasPrefix(step.Operation(), "FC2B_MEI_0E_") {
			t.Fatalf("replay step %q is not one of the two read-only operations", step.Operation())
		}
	}
	// The documentary card keeps its identity, tuples and steps untouched.
	documentary := NewHuaweiSmartLoggerQualificationCard()
	if documentary.ID() != "huawei.qualification.smartlogger.readonly@1.0.0" ||
		!reflect.DeepEqual(documentary.FirmwareTuples(), []string{"V300R024C10SPC191", "V300R024C10SPC210"}) || len(documentary.Steps()) != 3 {
		t.Fatalf("documentary card changed: %#v", documentary)
	}
	if got := documentary.Limits(); got.MaxResponseBytes != 0 {
		t.Fatalf("documentary card gained a response cap: %#v", got)
	}
}

func TestH03ObservedCardAdmitsExactTupleFromFixtures(t *testing.T) {
	card := NewHuaweiSmartLoggerObservedQualificationCard()
	result := card.Evaluate(h03ValidInput(t))
	if !result.IdentityMatched() || result.SelectedClass() != HuaweiSmartLoggerCanonicalClass || result.ModelVariant() != h03Model ||
		result.Readiness() != HuaweiQualificationTestReady || !result.QualificationTestReady() || result.Reason() != HuaweiQualificationMatchedIdentityInventory ||
		!reflect.DeepEqual(result.MissingEvidence(), []string{"pairwise_negative_overlap_with_emma_fixture"}) {
		t.Fatalf("exact observed tuple was not admitted: %#v", result)
	}
	if result.HardwareTestReady() || !result.DefaultDenied() || result.CatalogRegistered() || result.AutomaticRuntimeAdmission() || result.LiveQualified() ||
		result.SupportClaim() || result.WriteAuthority() || result.NativeFactCount() != 0 || result.AutomaticRequestCount() != 0 || result.FallbackAttempted() {
		t.Fatalf("admitted result created authority or telemetry: %#v", result)
	}
	inventory, ok := result.Inventory()
	if !ok || inventory.DeclaredChildren() != 11 || inventory.ChildCount() != 11 || !inventory.DefaultDenied() {
		t.Fatalf("unexpected retained inventory: %#v ok=%t", inventory, ok)
	}
	if got, want := h03SortedUnits(inventory), []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "11", "19"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("routed units=%v want=%v", got, want)
	}
	if anomalies := result.PageAnomalies(); anomalies != (HuaweiInventoryPageAnomalies{OversizeResponses: 12, DeclaredObjectFieldDiffers: 13}) {
		t.Fatalf("page anomalies=%#v", anomalies)
	}
	if !result.BasicCorroborated() || result.BasicOpaqueObjectCount() != 9 {
		t.Fatalf("basic corroboration=%t opaque=%d", result.BasicCorroborated(), result.BasicOpaqueObjectCount())
	}

	// The basic reply is optional: absence changes nothing but the corroboration flag.
	input := h03ValidInput(t)
	input.Basic = nil
	withoutBasic := card.Evaluate(input)
	if !withoutBasic.IdentityMatched() || withoutBasic.BasicCorroborated() || withoutBasic.BasicOpaqueObjectCount() != 0 {
		t.Fatalf("absent basic reply changed the outcome: %#v", withoutBasic)
	}
}

func TestH03ObservedReplayMatchesSanitizedExpectedResult(t *testing.T) {
	card := NewHuaweiSmartLoggerObservedQualificationCard()
	result := card.Evaluate(h03ValidInput(t))
	inventory, _ := result.Inventory()
	labels := map[string]int{}
	for _, child := range inventory.Children() {
		labels[child.Attribute("model")]++
	}
	units := []int{}
	for _, unit := range h03SortedUnits(inventory) {
		var number int
		if _, err := fmt.Sscanf(unit, "%d", &number); err != nil {
			t.Fatal(err)
		}
		units = append(units, number)
	}
	anomalies := result.PageAnomalies()
	actual := map[string]any{
		"schema":         "helianthus-huawei-smartlogger-observed-replay-expected/v1",
		"card_id":        card.ID(),
		"status":         result.Readiness(),
		"reason":         result.Reason(),
		"selected_class": result.SelectedClass(),
		"gateway_unit":   card.UnitID(),
		"self_entry":     map[string]any{"model": card.Model(), "firmware": card.Firmware(), "unit": card.SelfEntryUnit()},
		"pages":          len(h03LoadInventory(t).Pages),
		"entries":        inventory.ChildCount() + 1,
		"children":       inventory.ChildCount(),
		"children_by_label": map[string]any{
			"SUN2000-50KTL-M3": labels["SUN2000-50KTL-M3"], "PLC": labels["PLC"], "PowerMeter": labels["PowerMeter"],
		},
		"child_units":                 units,
		"page_anomalies":              map[string]any{"oversize_responses": anomalies.OversizeResponses, "declared_object_field_differs": anomalies.DeclaredObjectFieldDiffers},
		"basic_corroboration":         map[string]any{"agrees": result.BasicCorroborated(), "opaque_extra_objects": result.BasicOpaqueObjectCount()},
		"missing_evidence":            result.MissingEvidence(),
		"hardware_test_ready":         result.HardwareTestReady(),
		"live_qualified":              result.LiveQualified(),
		"automatic_request_count":     result.AutomaticRequestCount(),
		"native_fact_count":           result.NativeFactCount(),
		"default_denied":              result.DefaultDenied(),
		"catalog_registered":          result.CatalogRegistered(),
		"automatic_runtime_admission": result.AutomaticRuntimeAdmission(),
		"support_claim":               result.SupportClaim(),
		"write_authority":             result.WriteAuthority(),
	}
	encoded, err := json.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/huawei/qualification/smartlogger_observed_replay_expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(encoded, &gotValue); err != nil {
		t.Fatal(err)
	}
	if err := h03Decode(want, &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("replay result=%s\nwant=%s", encoded, want)
	}
}

func TestH03ObservedCardRejectsOneCharacterMutationOfEveryTupleField(t *testing.T) {
	card := NewHuaweiSmartLoggerObservedQualificationCard()
	if !card.Evaluate(h03ValidInput(t)).IdentityMatched() {
		t.Fatal("control did not match")
	}

	fields := []struct {
		name    string
		mutants []string
		mutate  func(string) func(*h03InventoryFixture)
	}{
		{"self-entry model", h03OneCharacterMutants(h03Model), func(m string) func(*h03InventoryFixture) {
			return h03ReplaceInSelfEntry("1="+h03Model+";", "1="+m+";")
		}},
		{"self-entry firmware", h03OneCharacterMutants(h03Firmware), func(m string) func(*h03InventoryFixture) {
			return h03ReplaceInSelfEntry("2="+h03Firmware+";", "2="+m+";")
		}},
		{"self-entry unit", []string{"1", "00", "0 ", "+0", "-0", "a", "10", "247"}, func(m string) func(*h03InventoryFixture) {
			return h03ReplaceInSelfEntry(";5=0;", ";5="+m+";")
		}},
	}
	for _, field := range fields {
		if len(field.mutants) < 8 {
			t.Fatalf("%s: mutant set is too small to cover the field", field.name)
		}
		for _, mutant := range field.mutants {
			input := h03MutatedInventory(t, field.mutate(mutant))
			h03RequireRejected(t, fmt.Sprintf("%s %q", field.name, mutant), card.Evaluate(input))
			input.Basic = h03BasicReplyFromFixture(t, h03LoadBasic(t))
			h03RequireRejected(t, fmt.Sprintf("%s %q with basic reply", field.name, mutant), card.Evaluate(input))
		}
	}

	// Counter bracket: each direction of inequality and a wrap.
	for _, counter := range []h03Counter{{Before: 7, After: 8}, {Before: 7, After: 6}, {Before: 0xffff, After: 0}, {Before: 0, After: 0xffff}} {
		input := h03MutatedInventory(t, func(f *h03InventoryFixture) { f.ChangeCounter = counter })
		h03RequireRejected(t, fmt.Sprintf("counter %+v", counter), card.Evaluate(input))
	}

	// Gateway unit: the card applies to unit 0 only.
	for _, unit := range []uint8{1, 2, 9, 100, 247, 255} {
		input := h03ValidInput(t)
		input.UnitID = unit
		h03RequireRejected(t, fmt.Sprintf("unit %d", unit), card.Evaluate(input))
	}

	// Count object: it includes the self entry, so any other value is a mismatch.
	for _, count := range []uint8{0, 1, 11, 13, 248, 255} {
		count := count
		input := h03MutatedInventory(t, func(f *h03InventoryFixture) { f.Pages[0].Objects[0].Count = &count })
		h03RequireRejected(t, fmt.Sprintf("count %d", count), card.Evaluate(input))
	}
}

func TestH03ObservedCardRejectsOneCharacterMutationOfBasicCorroboration(t *testing.T) {
	h03RequireControl(t)
	card := NewHuaweiSmartLoggerObservedQualificationCard()
	objects := map[uint8]string{0x00: "HUAWEI", 0x01: h03Model, 0x02: h03Firmware}
	for id, text := range objects {
		var mutants []string
		for _, mutant := range h03OneCharacterMutants(text) {
			// Terminal space is padding in a basic object, so that neighbour is the same value.
			if trimHuaweiTerminalPadding(mutant) != text {
				mutants = append(mutants, mutant)
			}
		}
		if len(mutants) < 8 {
			t.Fatalf("object %02x: mutant set is too small", id)
		}
		for _, mutant := range mutants {
			input := h03ValidInput(t)
			input.Basic = h03BasicWith(t, func(basic *h03BasicFixture) {
				for index := range basic.Reply.Objects {
					if uint8(basic.Reply.Objects[index].ObjectID) == id {
						basic.Reply.Objects[index].Text = mutant
					}
				}
			})
			result := card.Evaluate(input)
			h03RequireRejected(t, fmt.Sprintf("basic object %02x %q", id, mutant), result)
			if result.Reason() != HuaweiQualificationBasicIdentityConflict {
				t.Fatalf("basic object %02x %q: reason=%s", id, mutant, result.Reason())
			}
		}
	}
	// A decoded reply that lacks an identity object, or repeats one, cannot corroborate.
	for _, drop := range []uint8{0x00, 0x01, 0x02} {
		input := h03ValidInput(t)
		var kept []HuaweiSmartLoggerInventoryObject
		for _, object := range input.Basic.Objects {
			if object.ObjectID != drop {
				kept = append(kept, object)
			}
		}
		input.Basic = &HuaweiSmartLoggerObservedBasicReply{Objects: kept}
		h03RequireRejected(t, fmt.Sprintf("basic reply without %02x", drop), card.Evaluate(input))
	}
	duplicated := h03ValidInput(t)
	duplicated.Basic.Objects = append(duplicated.Basic.Objects, duplicated.Basic.Objects[0])
	h03RequireRejected(t, "duplicated vendor object", card.Evaluate(duplicated))

	// A basic reply that agrees on product but not on firmware is a conflict even
	// when the extended entry is exact.
	conflicting := h03ValidInput(t)
	conflicting.Basic = h03BasicWith(t, func(basic *h03BasicFixture) { basic.Reply.Objects[2].Text = "V300R024C10SPC191" })
	h03RequireRejected(t, "basic firmware from another tuple", card.Evaluate(conflicting))
}

func h03BasicWith(t testing.TB, mutate func(*h03BasicFixture)) *HuaweiSmartLoggerObservedBasicReply {
	t.Helper()
	fixture := h03LoadBasic(t)
	mutate(&fixture)
	var objects []HuaweiSmartLoggerInventoryObject
	for _, object := range fixture.Reply.Objects {
		value, err := h03BasicObjectValue(object)
		if err != nil {
			t.Fatal(err)
		}
		objects = append(objects, HuaweiSmartLoggerInventoryObject{ObjectID: uint8(object.ObjectID), Value: value})
	}
	return &HuaweiSmartLoggerObservedBasicReply{Objects: objects}
}

func TestH03ObservedCardOutsideAdmittedSetIsInsufficientEvidence(t *testing.T) {
	h03RequireControl(t)
	observed := NewHuaweiSmartLoggerObservedQualificationCard()
	documentary := NewHuaweiSmartLoggerQualificationCard()
	models := []string{"Smart Logger", "SmartLogger"}
	firmwares := []string{"V300R023C10SPC620", "V300R024C10SPC191", "V300R024C10SPC210"}
	for _, model := range models {
		for _, firmware := range firmwares {
			input := h03MutatedInventory(t, func(f *h03InventoryFixture) {
				object := &f.Pages[1].Objects[0]
				object.Entry = strings.Replace(object.Entry, "1="+h03Model+";2="+h03Firmware+";", "1="+model+";2="+firmware+";", 1)
			})
			wantObserved := model == h03Model && firmware == h03Firmware
			if got := observed.Evaluate(input).IdentityMatched(); got != wantObserved {
				t.Fatalf("observed card model=%q firmware=%q matched=%t", model, firmware, got)
			}
			if !wantObserved {
				h03RequireRejected(t, fmt.Sprintf("observed %q %q", model, firmware), observed.Evaluate(input))
			}

			docInput := validHuaweiSmartLoggerQualificationInput()
			docInput.Inventory.Pages[0].Objects[1].Value = []byte(fmt.Sprintf("1=%s;2=%s;5=0", model, firmware))
			wantDocumentary := model == "SmartLogger" && firmware != h03Firmware
			if got := documentary.Evaluate(docInput).IdentityMatched(); got != wantDocumentary {
				t.Fatalf("documentary card model=%q firmware=%q matched=%t", model, firmware, got)
			}
		}
	}

	// Other families' self entries and firmware are outside the admitted set.
	for _, self := range []string{
		"1=EMMA-A02;2=SmartHEMS V100R025C00SPC131;5=0",
		"1=SUN2000-50KTL-M3;2=V200R023C00SPC128;5=0",
		"1=PowerMeter;2=XXXXXXXXXXXXXXXX;5=0",
		"1=S-DongleA-05;2=V200R025C00SPC120;5=0",
		"1=Smart Logger;5=0",
		"1=Smart Logger;2=;5=0",
		"2=" + h03Firmware + ";5=0",
	} {
		input := h03MutatedInventory(t, func(f *h03InventoryFixture) { f.Pages[1].Objects[0].Entry = self })
		h03RequireRejected(t, self, observed.Evaluate(input))
	}

	// Nothing supplied at all.
	h03RequireRejected(t, "empty input", observed.Evaluate(HuaweiSmartLoggerObservedQualificationInput{}))
	h03RequireRejected(t, "empty result accessor", observed.EmptyResult())
}

func TestH03DocumentaryCardStillReturnsInsufficientEvidenceForObservedTuple(t *testing.T) {
	fixture := h03LoadInventory(t)
	pdus, err := h03InventoryPDUs(fixture)
	if err != nil {
		t.Fatal(err)
	}
	documentaryInput := HuaweiSmartLoggerQualificationInput{
		UnitID:    0,
		Inventory: HuaweiSmartLoggerOfflineInventoryInput{ChangeCounterBefore: fixture.ChangeCounter.Before, ChangeCounterAfter: fixture.ChangeCounter.After},
	}
	for _, pdu := range pdus {
		parsed, err := h03ParseMEI(pdu)
		if err != nil {
			t.Fatal(err)
		}
		documentaryInput.Inventory.Pages = append(documentaryInput.Inventory.Pages, HuaweiSmartLoggerInventoryPage{
			More: parsed.more == 0xff, NextObjectID: parsed.next, Objects: parsed.objects,
		})
	}
	result := NewHuaweiSmartLoggerQualificationCard().Evaluate(documentaryInput)
	assertHuaweiQualificationFailureHasNoFallback(t, result)
	if result.Reason() != HuaweiQualificationInventoryInvalid {
		t.Fatalf("documentary card reason=%s", result.Reason())
	}

	// The observed values in the documentary encoding are still not the
	// documentary tuple: the model differs by one space and the firmware is outside both.
	small := validHuaweiSmartLoggerQualificationInput()
	small.Inventory.Pages[0].Objects[1].Value = []byte("1=Smart Logger;2=V300R023C10SPC620;5=0")
	assertHuaweiQualificationFailureHasNoFallback(t, NewHuaweiSmartLoggerQualificationCard().Evaluate(small))
}

func TestH03ChildUnitAnsweringAsLoggerSelectsNothing(t *testing.T) {
	h03RequireControl(t)
	card := NewHuaweiSmartLoggerObservedQualificationCard()
	basic := h03LoadBasic(t)
	if !reflect.DeepEqual(basic.AnsweredForUnits, []int{0, 1, 100}) {
		t.Fatalf("basic reply is answered for units %v", basic.AnsweredForUnits)
	}
	for _, unit := range basic.AnsweredForUnits {
		if unit == 0 {
			continue
		}
		// The gateway's own basic identity arrives for a child unit and for the
		// S-Dongle unit; neither unit is a SmartLogger.
		input := h03ValidInput(t)
		input.UnitID = uint8(unit)
		h03RequireRejected(t, fmt.Sprintf("inventory read on unit %d", unit), card.Evaluate(input))

		basicOnly := HuaweiSmartLoggerObservedQualificationInput{UnitID: uint8(unit), Basic: h03ValidInput(t).Basic}
		h03RequireRejected(t, fmt.Sprintf("basic reply alone on unit %d", unit), card.Evaluate(basicOnly))
	}
	h03RequireRejected(t, "basic reply alone on unit 0", card.Evaluate(HuaweiSmartLoggerObservedQualificationInput{UnitID: 0, Basic: h03ValidInput(t).Basic}))

	// A child that claims the gateway's unit, or a gateway entry that claims a
	// child's unit, breaks the self-entry binding in either direction.
	for _, test := range []struct {
		name   string
		unit   uint8
		mutate func(*h03InventoryFixture)
	}{
		{"self entry claims unit 1 on unit 0", 0, h03ReplaceInSelfEntry(";5=0;", ";5=1;")},
		{"self entry claims unit 1 on unit 1", 1, h03ReplaceInSelfEntry(";5=0;", ";5=1;")},
		{"inverter entry first, read on unit 0", 0, func(f *h03InventoryFixture) {
			f.Pages[1].Objects[0].Entry, f.Pages[2].Objects[0].Entry = f.Pages[2].Objects[0].Entry, f.Pages[1].Objects[0].Entry
		}},
		{"child claims unit 0", 0, func(f *h03InventoryFixture) {
			object := &f.Pages[3].Objects[0]
			object.Entry = strings.Replace(object.Entry, ";5=3;", ";5=0;", 1)
		}},
		{"two entries share a routed unit", 0, func(f *h03InventoryFixture) {
			object := &f.Pages[3].Objects[0]
			object.Entry = strings.Replace(object.Entry, ";5=3;", ";5=2;", 1)
		}},
	} {
		input := h03MutatedInventory(t, test.mutate)
		input.UnitID = test.unit
		h03RequireRejected(t, test.name, card.Evaluate(input))
	}
}

func TestH03ChildListingAloneSelectsNothing(t *testing.T) {
	h03RequireControl(t)
	card := NewHuaweiSmartLoggerObservedQualificationCard()
	// A complete, valid listing whose first entry is not the gateway: every
	// child is intact and none of them selects a class.
	for _, replacement := range []string{
		"1=SUN2000-50KTL-M3;2=V200R023C00SPC128;3=X0.00-X0.0;4=000000000000;5=0;6=0.0;8=SUN2000;9=0",
		"1=PLC;2=XXXXXXXXXXXXXXXXX;5=0",
		"1=PowerMeter;2=XXXXXXXXXXXXXXXX;5=0",
		"1=EMMA-A02;2=SmartHEMS V100R025C00SPC131;5=0",
	} {
		input := h03MutatedInventory(t, func(f *h03InventoryFixture) { f.Pages[1].Objects[0].Entry = replacement })
		result := card.Evaluate(input)
		h03RequireRejected(t, replacement, result)
		resolution := ResolveHuaweiQualification(result)
		if resolution.SelectedClass() != "" || len(resolution.CandidateClasses()) != 0 || resolution.Readiness() != HuaweiQualificationInsufficientEvidence {
			t.Fatalf("listing alone selected %#v", resolution)
		}
	}

	// The children of the observed listing are retained listing evidence only:
	// no child class appears among the candidate or selected classes.
	result := card.Evaluate(h03ValidInput(t))
	inventory, _ := result.Inventory()
	for _, child := range inventory.Children() {
		if child.Attribute("model") == result.SelectedClass() || child.Attribute("product_type") == result.SelectedClass() {
			t.Fatalf("child %q was read as the selected class", child.Attribute("model"))
		}
	}
	if result.SelectedClass() != HuaweiSmartLoggerCanonicalClass || len(result.CandidateClasses()) != 0 {
		t.Fatalf("unexpected classes: selected=%q candidates=%v", result.SelectedClass(), result.CandidateClasses())
	}
	resolution := ResolveHuaweiQualification(result)
	if resolution.SelectedClass() != HuaweiSmartLoggerCanonicalClass || !reflect.DeepEqual(resolution.CandidateClasses(), []string{HuaweiSmartLoggerCanonicalClass}) {
		t.Fatalf("single observed result resolved to %#v", resolution)
	}
}

func TestH03SerialEsnReadabilityAndPrefixNeverSelect(t *testing.T) {
	h03RequireControl(t)
	card := NewHuaweiSmartLoggerObservedQualificationCard()

	// The sensitive attributes are never a predicate: any valid value in any
	// entry leaves the positive outcome and the retained listing unchanged.
	reference := card.Evaluate(h03ValidInput(t))
	referenceInventory, _ := reference.Inventory()
	for _, value := range []string{"A", "000000000000", "AAAAAAAAAAAAAAAAAAAA", "synthetic-private-input", "1", "XX_XXX_XXXXXX"} {
		input := h03MutatedInventory(t, func(f *h03InventoryFixture) {
			for pageIndex := 1; pageIndex < len(f.Pages); pageIndex++ {
				object := &f.Pages[pageIndex].Objects[0]
				attributes := h03EntryAttributes(object.Entry)
				for index := range attributes {
					if attributes[index][0] == "4" || attributes[index][0] == "10" {
						attributes[index][1] = value
					}
				}
				var parts []string
				for _, pair := range attributes {
					parts = append(parts, pair[0]+"="+pair[1])
				}
				object.Entry = strings.Join(parts, ";")
			}
		})
		result := card.Evaluate(input)
		inventory, _ := result.Inventory()
		if !result.IdentityMatched() || !reflect.DeepEqual(inventory.Children(), referenceInventory.Children()) {
			t.Fatalf("sensitive attribute value %q changed the outcome: %#v", value, result)
		}
	}

	// Matching sensitive attributes cannot stand in for the tuple.
	input := h03MutatedInventory(t, h03ReplaceInSelfEntry("1="+h03Model+";", "1=Other Logger;"))
	h03RequireRejected(t, "serial-bearing entry with another model", card.Evaluate(input))

	// Readability alone: counters that agree, no inventory.
	h03RequireRejected(t, "counter readable, no listing", card.Evaluate(HuaweiSmartLoggerObservedQualificationInput{
		UnitID:    0,
		Inventory: HuaweiSmartLoggerObservedInventoryInput{ChangeCounterBefore: 7, ChangeCounterAfter: 7},
	}))
	h03RequireRejected(t, "count object only", card.Evaluate(func() HuaweiSmartLoggerObservedQualificationInput {
		input := h03ValidInput(t)
		input.Inventory.Pages = input.Inventory.Pages[:1]
		input.Inventory.Pages[0].More = false
		input.Inventory.Pages[0].NextObjectID = 0
		return input
	}()))

	// Prefix, range and ordering are not matching rules.
	for _, mutant := range []string{
		"V300R023C10SPC62", "V300R023C10SPC6200", "V300R023C10", "V300R023", "V300R023C10SPC621", "V300R023C10SPC619",
		"V300R024C10SPC620", "V300R023C10SPC6", strings.ToLower(h03Firmware), "v300r023c10spc620",
	} {
		input := h03MutatedInventory(t, h03ReplaceInSelfEntry("2="+h03Firmware+";", "2="+mutant+";"))
		h03RequireRejected(t, "firmware "+mutant, card.Evaluate(input))
	}
	for _, mutant := range []string{"Smart", "Smart Logger 2", "Smart Logger  ", "smart logger", "SMART LOGGER", "Smart_Logger", "SmartLogger", "Smart-Logger", "Logger"} {
		input := h03MutatedInventory(t, h03ReplaceInSelfEntry("1="+h03Model+";", "1="+mutant+";"))
		h03RequireRejected(t, "model "+mutant, card.Evaluate(input))
	}
}

func TestH03TwoPositiveCandidatesProduceNoWinner(t *testing.T) {
	observed := NewHuaweiSmartLoggerObservedQualificationCard().Evaluate(h03ValidInput(t))
	emma := NewHuaweiEMMAQualificationCard().Evaluate(HuaweiEMMAQualificationInput{
		UnitID: 0, Offering: "SmartHEMS", Model: "EMMA-A02", Firmware: "SmartHEMS V100R025C00SPC131",
	})
	if !observed.IdentityMatched() || !emma.IdentityMatched() {
		t.Fatal("both controls must be positive")
	}
	for _, results := range [][]HuaweiQualificationResult{{observed, emma}, {emma, observed}} {
		resolution := ResolveHuaweiQualification(results...)
		if resolution.Readiness() != HuaweiQualificationAmbiguous || resolution.Reason() != HuaweiQualificationMultipleClasses ||
			resolution.SelectedClass() != "" || resolution.IdentityMatched() ||
			!reflect.DeepEqual(resolution.CandidateClasses(), []string{HuaweiEMMACanonicalClass, HuaweiSmartLoggerCanonicalClass}) ||
			resolution.NativeFactCount() != 0 || resolution.AutomaticRequestCount() != 0 || resolution.FallbackAttempted() {
			t.Fatalf("two positive candidates produced a winner: %#v", resolution)
		}
	}

	// One positive and one failed candidate of another class stays insufficient.
	failed := NewHuaweiEMMAQualificationCard().Evaluate(HuaweiEMMAQualificationInput{})
	resolution := ResolveHuaweiQualification(observed, failed)
	if resolution.Readiness() != HuaweiQualificationInsufficientEvidence || resolution.SelectedClass() != "" || resolution.FallbackAttempted() {
		t.Fatalf("mixed positive and failed candidates selected: %#v", resolution)
	}
}

func TestH03SDongleHardStopIsPreservedWithoutRespondingGatewayUnitTuple(t *testing.T) {
	h03RequireControl(t)
	observed := NewHuaweiSmartLoggerObservedQualificationCard().Evaluate(h03ValidInput(t))
	emma := NewHuaweiEMMAQualificationCard().Evaluate(HuaweiEMMAQualificationInput{
		UnitID: 0, Offering: "SmartHEMS", Model: "EMMA-A02", Firmware: "SmartHEMS V100R025C00SPC131",
	})
	blocked := NewHuaweiSDongleQualificationCard().Result()
	sets := [][]HuaweiQualificationResult{
		{observed, blocked}, {blocked, observed}, {observed, emma, blocked}, {blocked, observed, emma}, {emma, blocked, observed},
	}
	for index, results := range sets {
		resolution := ResolveHuaweiQualification(results...)
		if resolution.Readiness() != HuaweiQualificationEvidenceBlocked || resolution.Reason() != HuaweiQualificationPersistentNonResponse ||
			!reflect.DeepEqual(resolution.MissingEvidence(), blocked.MissingEvidence()) || resolution.SelectedClass() != "" ||
			len(resolution.CandidateClasses()) != 0 || resolution.NativeFactCount() != 0 || resolution.FallbackAttempted() {
			t.Fatalf("set %d lost the S-Dongle hard stop: %#v", index, resolution)
		}
	}

	// The only responder at logical unit 100 in the fixture set is the
	// SmartLogger's own basic identity. It is not an S-Dongle tuple.
	basic := h03LoadBasic(t)
	var product string
	for _, object := range basic.Reply.Objects {
		if object.ObjectID == 0x01 {
			product = object.Text
		}
	}
	decision := EvaluateHuaweiSDongleOfflineObservation(HuaweiSDongleOfflineObservation{
		UnitID: 100, Model: product, Firmware: h03Firmware, ProtocolMajor: 5, ProtocolMinor: 0, SearchState: 0,
		ChangeSequenceBefore: 7, ChangeSequenceAfter: 7, Capacity: 2, ChildCount: 1,
	})
	if decision.Matched() || decision.Reason() != HuaweiSDongleOfflineModelMismatch {
		t.Fatalf("gateway-unit reply from another family matched the S-Dongle gate: %#v", decision)
	}
	card := NewHuaweiSDongleQualificationCard()
	if !card.HardStop() || len(card.Steps()) != 0 || card.Status() != HuaweiQualificationEvidenceBlocked || card.UnitID() != 100 {
		t.Fatalf("S-Dongle card lost its hard stop: %#v", card)
	}
	disposition := decodeHuaweiObject(t, "profiles/vendor/huawei/sdongle-disposition.json")
	plan := huaweiObject(t, disposition["qualification_plan"])
	evidence := huaweiObject(t, disposition["evidence"])
	live := huaweiObject(t, evidence["live_qualification"])
	if disposition["outcome"] != "PRE_LIVE_INSUFFICIENT_EVIDENCE" || plan["further_live_io"] != "HARD_STOP" ||
		live["status"] != "LIVE_STOPPED_PERSISTENT_NON_RESPONSE" || huaweiInt(t, live["subsequent_modbus_requests_sent"]) != 0 ||
		len(huaweiStringSlice(t, evidence["missing_evidence"])) != 3 {
		t.Fatalf("S-Dongle disposition changed: %#v", disposition)
	}
}

func TestH03ObservedCardCarriesNoAuthority(t *testing.T) {
	card := NewHuaweiSmartLoggerObservedQualificationCard()
	results := []HuaweiQualificationResult{card.EmptyResult(), card.Evaluate(h03ValidInput(t)), card.Evaluate(HuaweiSmartLoggerObservedQualificationInput{})}
	for _, result := range results {
		if !result.DefaultDenied() || result.CatalogRegistered() || result.AutomaticRuntimeAdmission() || result.LiveQualified() ||
			result.SupportClaim() || result.WriteAuthority() || result.NativeFactCount() != 0 || result.AutomaticRequestCount() != 0 ||
			result.FallbackAttempted() || result.HardwareTestReady() {
			t.Fatalf("result created authority or telemetry: %#v", result)
		}
	}
	for _, name := range []string{"smartlogger-disposition.json", "emma-disposition.json", "sdongle-disposition.json", "sun2000-disposition.json", "luna2000-disposition.json"} {
		record := decodeHuaweiObject(t, "profiles/vendor/huawei/"+name)
		if record["catalog_registered"] != false || record["automatic_runtime_admission"] != false || record["support_claim"] != false {
			t.Fatalf("%s grants authority: %#v", name, record)
		}
		if name != "emma-disposition.json" {
			private := huaweiObject(t, record["private_functions"])
			if private["FC0x41"] != "NO_SEND" || private["FC0x17"] != "NO_SEND" {
				t.Fatalf("%s opens a private function: %#v", name, private)
			}
		}
		if name == "smartlogger-disposition.json" && record["write_authority"] != false {
			t.Fatalf("%s lacks the explicit write-authority denial", name)
		}
	}
}

func TestH03SameClassSiblingCardsAreAlternativesInSharedResolution(t *testing.T) {
	h03RequireControl(t)
	observed := NewHuaweiSmartLoggerObservedQualificationCard().Evaluate(h03ValidInput(t))
	observedFailed := NewHuaweiSmartLoggerObservedQualificationCard().Evaluate(HuaweiSmartLoggerObservedQualificationInput{})
	documentary := NewHuaweiSmartLoggerQualificationCard().Evaluate(validHuaweiSmartLoggerQualificationInput())
	documentaryFailed := NewHuaweiSmartLoggerQualificationCard().Evaluate(HuaweiSmartLoggerQualificationInput{})
	emma := NewHuaweiEMMAQualificationCard().Evaluate(HuaweiEMMAQualificationInput{
		UnitID: 0, Offering: "SmartHEMS", Model: "EMMA-A02", Firmware: "SmartHEMS V100R025C00SPC131",
	})
	emmaFailed := NewHuaweiEMMAQualificationCard().Evaluate(HuaweiEMMAQualificationInput{})
	blocked := NewHuaweiSDongleQualificationCard().Result()
	if !observed.IdentityMatched() || !documentary.IdentityMatched() || observedFailed.IdentityMatched() || documentaryFailed.IdentityMatched() {
		t.Fatal("controls are not positive and negative as required")
	}

	selects := func(name string, results ...HuaweiQualificationResult) {
		t.Helper()
		resolution := ResolveHuaweiQualification(results...)
		if !resolution.IdentityMatched() || resolution.SelectedClass() != HuaweiSmartLoggerCanonicalClass || resolution.Readiness() != HuaweiQualificationTestReady ||
			!reflect.DeepEqual(resolution.CandidateClasses(), []string{HuaweiSmartLoggerCanonicalClass}) || resolution.FallbackAttempted() {
			t.Fatalf("%s: %#v", name, resolution)
		}
	}
	selects("observed with failed documentary sibling", observed, documentaryFailed)
	selects("failed documentary sibling first", documentaryFailed, observed)
	selects("documentary with failed observed sibling", documentary, observedFailed)

	for name, results := range map[string][]HuaweiQualificationResult{
		"both siblings failed":                                    {observedFailed, documentaryFailed},
		"positive with a failed other class":                      {observed, emmaFailed},
		"positive with a failed sibling and a failed other class": {observed, documentaryFailed, emmaFailed},
	} {
		resolution := ResolveHuaweiQualification(results...)
		if resolution.Readiness() != HuaweiQualificationInsufficientEvidence || resolution.SelectedClass() != "" || resolution.IdentityMatched() || resolution.FallbackAttempted() {
			t.Fatalf("%s selected: %#v", name, resolution)
		}
	}

	ambiguous := ResolveHuaweiQualification(observed, documentaryFailed, emma)
	if ambiguous.Readiness() != HuaweiQualificationAmbiguous || ambiguous.SelectedClass() != "" ||
		!reflect.DeepEqual(ambiguous.CandidateClasses(), []string{HuaweiEMMACanonicalClass, HuaweiSmartLoggerCanonicalClass}) {
		t.Fatalf("sibling and another positive class: %#v", ambiguous)
	}
	hardStop := ResolveHuaweiQualification(observed, documentaryFailed, blocked)
	if hardStop.Readiness() != HuaweiQualificationEvidenceBlocked || hardStop.Reason() != HuaweiQualificationPersistentNonResponse || hardStop.SelectedClass() != "" {
		t.Fatalf("hard stop lost behind a sibling: %#v", hardStop)
	}

	// Two positive results of one class would be two exact tuples at once; that
	// is a contradiction and selects nothing.
	second := observed
	second.modelVariant = "contradictory second tuple"
	contradiction := ResolveHuaweiQualification(observed, second)
	if contradiction.Readiness() != HuaweiQualificationAmbiguous || contradiction.SelectedClass() != "" ||
		!reflect.DeepEqual(contradiction.CandidateClasses(), []string{HuaweiSmartLoggerCanonicalClass}) {
		t.Fatalf("two positives of one class selected: %#v", contradiction)
	}
}
