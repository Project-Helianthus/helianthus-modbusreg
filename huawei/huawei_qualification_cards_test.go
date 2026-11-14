package huawei

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestHuaweiQualificationCardsAreBoundedDefaultDeniedAndClassLocal(t *testing.T) {
	smartLogger := NewSmartLoggerQualificationCard()
	if smartLogger.ID() != SmartLoggerQualificationCardID || smartLogger.CandidateClass() != SmartLoggerCanonicalClass ||
		smartLogger.UnitID() != 0 ||
		smartLogger.Limits() != (QualificationInventoryLimits{DeadlineMilliseconds: 15000, MaxPages: 248, MaxObjects: 248, MaxBytes: 65536, MaxChildren: 247}) {
		t.Fatalf("unexpected SmartLogger card: %#v", smartLogger)
	}
	assertHuaweiQualificationSteps(t, smartLogger.Steps(), []QualificationStep{
		{operation: "FC03_COUNTER_BEFORE", unitID: 0, offset: 65521, quantity: 1},
		{operation: "FC2B_MEI_0E_READDEV03_INVENTORY", unitID: 0, objectID: 0x87},
		{operation: "FC03_COUNTER_AFTER", unitID: 0, offset: 65521, quantity: 1},
	})

	emma := NewEMMAQualificationCard()
	if emma.ID() != EMMAQualificationCardID || emma.CandidateClass() != EMMACanonicalClass || emma.UnitID() != 0 ||
		!reflect.DeepEqual(emma.ModelVariants(), []string{"EMMA-A01", "EMMA-A02"}) ||
		!reflect.DeepEqual(emma.MissingCapabilityEvidence(), []string{"sanitized_readonly_capability_fixture", "negative_overlap_with_smartlogger", "model_specific_capability_fixture"}) {
		t.Fatalf("unexpected EMMA card: %#v", emma)
	}
	assertHuaweiQualificationSteps(t, emma.Steps(), []QualificationStep{
		{operation: "FC03_OFFERING", unitID: 0, offset: 30000, quantity: 15},
		{operation: "FC03_MODEL", unitID: 0, offset: 30222, quantity: 20},
		{operation: "FC03_FIRMWARE", unitID: 0, offset: 30035, quantity: 15},
	})

	sdongle := NewSDongleQualificationCard()
	if sdongle.ID() != SDongleQualificationCardID || sdongle.CandidateClass() != SDongleCanonicalClass || sdongle.UnitID() != 100 ||
		!sdongle.HardStop() || sdongle.Status() != QualificationEvidenceBlocked || len(sdongle.Steps()) != 0 ||
		!reflect.DeepEqual(sdongle.RequiredConnectionContext(), []string{"endpoint", "port", "unit_id_100", "gateway_child_topology"}) {
		t.Fatalf("unexpected S-Dongle card: %#v", sdongle)
	}

	for _, result := range []QualificationResult{smartLogger.EmptyResult(), emma.EmptyResult(), sdongle.Result()} {
		if !result.DefaultDenied() || result.CatalogRegistered() || result.AutomaticRuntimeAdmission() || result.LiveQualified() ||
			result.SupportClaim() || result.WriteAuthority() || result.NativeFactCount() != 0 || result.AutomaticRequestCount() != 0 || result.FallbackAttempted() || result.HardwareTestReady() {
			t.Fatalf("qualification result created authority or telemetry: %#v", result)
		}
	}
}

func TestHuaweiSmartLoggerQualificationCardUsesStableSelfBackedInventory(t *testing.T) {
	card := NewSmartLoggerQualificationCard()
	input := validHuaweiSmartLoggerQualificationInput()
	result := card.Evaluate(input)
	if !result.IdentityMatched() || result.SelectedClass() != SmartLoggerCanonicalClass ||
		result.Readiness() != QualificationTestReady || !result.QualificationTestReady() || result.Reason() != QualificationMatchedIdentityInventory {
		t.Fatalf("unexpected SmartLogger result: %#v", result)
	}
	inventory, ok := result.Inventory()
	if !ok || inventory.DeclaredChildren() != 1 || inventory.ChildCount() != 1 || inventory.Children()[0].Address() != "1" ||
		inventory.Children()[0].Attribute("model") != "SUN2000" || inventory.Children()[0].Attribute("4") != "" {
		t.Fatalf("unexpected retained inventory: %#v ok=%t", inventory, ok)
	}

	tests := []struct {
		name   string
		mutate func(*SmartLoggerQualificationInput)
	}{
		{"wrong unit", func(input *SmartLoggerQualificationInput) { input.UnitID = 1 }},
		{"firmware below the V300 floor", func(input *SmartLoggerQualificationInput) {
			input.Inventory.Pages[0].Objects[1].Value = []byte("1=SmartLogger;2=V300R000C10SPC999;5=0")
		}},
		{"malformed firmware", func(input *SmartLoggerQualificationInput) {
			input.Inventory.Pages[0].Objects[1].Value = []byte("1=SmartLogger;2=V300R024C10SPC99;5=0")
		}},
		{"EMMA self is not SmartLogger", func(input *SmartLoggerQualificationInput) {
			input.Inventory.Pages[0].Objects[1].Value = []byte("1=EMMA-A02;2=SmartHEMS V100R025C00SPC131;5=0")
		}},
		{"counter changed", func(input *SmartLoggerQualificationInput) { input.Inventory.ChangeCounterAfter++ }},
		{"malformed child", func(input *SmartLoggerQualificationInput) {
			input.Inventory.Pages[0].Objects[2].Value = []byte("1=SUN2000;5")
		}},
		{"child attribute key that is not a number", func(input *SmartLoggerQualificationInput) {
			input.Inventory.Pages[0].Objects[2].Value = []byte("1=SUN2000;5=1;x9=unknown")
		}},
		{"cursor loop", func(input *SmartLoggerQualificationInput) {
			input.Inventory.Pages[0].More = true
			input.Inventory.Pages[0].NextObjectID = 0x87
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := validHuaweiSmartLoggerQualificationInput()
			tc.mutate(&input)
			result := card.Evaluate(input)
			assertHuaweiQualificationFailureHasNoFallback(t, result)
		})
	}
}

func TestHuaweiEMMAQualificationCardPinsModelFirmwareAndMissingCapabilities(t *testing.T) {
	card := NewEMMAQualificationCard()
	for _, input := range []EMMAQualificationInput{
		{UnitID: 0, Offering: "SmartHEMS", Model: "EMMA-A01", Firmware: "SmartHEMS V100R024C00SPC100"},
		{UnitID: 0, Offering: "SmartHEMS", Model: "EMMA-A02", Firmware: "SmartHEMS V100R025C00SPC131"},
	} {
		result := card.Evaluate(input)
		if !result.IdentityMatched() || result.SelectedClass() != EMMACanonicalClass || result.ModelVariant() != input.Model ||
			result.Readiness() != QualificationEvidenceBlocked || result.QualificationTestReady() || result.Reason() != QualificationIdentityMatchedCapabilitiesMissing ||
			!reflect.DeepEqual(result.MissingEvidence(), card.MissingCapabilityEvidence()) || result.NativeFactCount() != 0 {
			t.Fatalf("unexpected EMMA result for %#v: %#v", input, result)
		}
	}

	for _, input := range []EMMAQualificationInput{
		{UnitID: 1, Offering: "SmartHEMS", Model: "EMMA-A02", Firmware: "SmartHEMS V100R025C00SPC131"},
		{UnitID: 0, Offering: "SmartHEMS", Model: "SmartLogger", Firmware: "SmartHEMS V100R025C00SPC131"},
		{UnitID: 0, Offering: "SmartHEMS", Model: "EMMA-A02", Firmware: "SmartHEMS V100R025C00SPC101"},
	} {
		assertHuaweiQualificationFailureHasNoFallback(t, card.Evaluate(input))
	}
}

func TestHuaweiQualificationResolutionRejectsAmbiguityAndNeverFallsBack(t *testing.T) {
	smartLogger := NewSmartLoggerQualificationCard().Evaluate(validHuaweiSmartLoggerQualificationInput())
	emma := NewEMMAQualificationCard().Evaluate(EMMAQualificationInput{
		UnitID: 0, Offering: "SmartHEMS", Model: "EMMA-A02", Firmware: "SmartHEMS V100R025C00SPC131",
	})
	resolution := ResolveHuaweiQualification(smartLogger, emma)
	if resolution.Readiness() != QualificationAmbiguous || resolution.SelectedClass() != "" ||
		!reflect.DeepEqual(resolution.CandidateClasses(), []string{EMMACanonicalClass, SmartLoggerCanonicalClass}) ||
		resolution.NativeFactCount() != 0 || resolution.AutomaticRequestCount() != 0 || resolution.FallbackAttempted() {
		t.Fatalf("ambiguous resolution leaked a selection: %#v", resolution)
	}

	failed := NewSmartLoggerQualificationCard().Evaluate(SmartLoggerQualificationInput{})
	resolution = ResolveHuaweiQualification(failed)
	if resolution.Readiness() != QualificationInsufficientEvidence || resolution.SelectedClass() != "" ||
		len(resolution.CandidateClasses()) != 0 || resolution.NativeFactCount() != 0 || resolution.AutomaticRequestCount() != 0 || resolution.FallbackAttempted() {
		t.Fatalf("failed resolution created fallback: %#v", resolution)
	}

	resolution = ResolveHuaweiQualification(failed, emma)
	if resolution.Readiness() != QualificationInsufficientEvidence || resolution.SelectedClass() != "" || resolution.FallbackAttempted() {
		t.Fatalf("mixed failed/positive classifications created cross-class fallback: %#v", resolution)
	}

	blocked := NewSDongleQualificationCard().Result()
	resolution = ResolveHuaweiQualification(blocked)
	if resolution.Readiness() != QualificationEvidenceBlocked || resolution.Reason() != QualificationPersistentNonResponse || resolution.SelectedClass() != "" {
		t.Fatalf("S-Dongle hard stop was downgraded by shared resolution: %#v", resolution)
	}
}

func TestHuaweiQualificationPersistentNonResponseDominatesEveryClassOrder(t *testing.T) {
	smartLogger := NewSmartLoggerQualificationCard().Evaluate(validHuaweiSmartLoggerQualificationInput())
	emma := NewEMMAQualificationCard().Evaluate(EMMAQualificationInput{
		UnitID: 0, Offering: "SmartHEMS", Model: "EMMA-A02", Firmware: "SmartHEMS V100R025C00SPC131",
	})
	blocked := NewSDongleQualificationCard().Result()
	permutations := [][]QualificationResult{
		{smartLogger, emma, blocked},
		{smartLogger, blocked, emma},
		{emma, smartLogger, blocked},
		{emma, blocked, smartLogger},
		{blocked, smartLogger, emma},
		{blocked, emma, smartLogger},
	}
	for index, results := range permutations {
		resolution := ResolveHuaweiQualification(results...)
		if resolution.Readiness() != QualificationEvidenceBlocked ||
			resolution.Reason() != QualificationPersistentNonResponse ||
			!reflect.DeepEqual(resolution.MissingEvidence(), blocked.MissingEvidence()) ||
			resolution.SelectedClass() != "" || len(resolution.CandidateClasses()) != 0 ||
			resolution.NativeFactCount() != 0 || resolution.AutomaticRequestCount() != 0 || resolution.FallbackAttempted() {
			t.Fatalf("permutation %d lost the complete S-Dongle hard stop: %#v", index, resolution)
		}
	}
}

func TestHuaweiSDongleQualificationCardPreservesPersistentNonResponseHardStop(t *testing.T) {
	card := NewSDongleQualificationCard()
	result := card.Result()
	wantMissing := []string{
		"confirmed_gateway_connection_context",
		"gateway_unit_100_topology",
		"sanitized_basic_mei_product_model_fixture",
		"exact_protocol_version_encoding_fixture",
		"completed_search_stable_sequence_capacity_fixture",
		"separately_qualified_child_unit_inventory_fixture",
	}
	if result.Readiness() != QualificationEvidenceBlocked || result.Reason() != QualificationPersistentNonResponse ||
		result.IdentityMatched() || result.SelectedClass() != "" || !reflect.DeepEqual(result.MissingEvidence(), wantMissing) ||
		result.QualificationTestReady() || result.AutomaticRequestCount() != 0 || result.FallbackAttempted() {
		t.Fatalf("unexpected S-Dongle hard-stop result: %#v", result)
	}

	// A synthetic candidate tuple exercises only the existing offline decoder;
	// it cannot overwrite the retained live evidence outcome of this card.
	candidate := EvaluateHuaweiSDongleOfflineObservation(SDongleOfflineObservation{
		UnitID: 100, Model: "S-DongleA-05", Firmware: "V200R025C00SPC120", ProtocolMajor: 5, ProtocolMinor: 0,
		SearchState: 0, ChangeSequenceBefore: 7, ChangeSequenceAfter: 7, Capacity: 2, ChildCount: 1,
	})
	if !candidate.Matched() || card.Result().Readiness() != QualificationEvidenceBlocked || card.Result().IdentityMatched() {
		t.Fatal("synthetic offline candidate overrode the persistent non-response hard stop")
	}
}

func TestHuaweiQualificationPackageSanitizedExpectedResult(t *testing.T) {
	encoded, err := json.Marshal(NewQualificationPackage().ExpectedResult())
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../testdata/huawei/qualification/readiness_expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(encoded, &gotValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("expected result=%s want=%s", encoded, want)
	}
}

func validHuaweiSmartLoggerQualificationInput() SmartLoggerQualificationInput {
	return SmartLoggerQualificationInput{
		UnitID: 0,
		Inventory: SmartLoggerOfflineInventoryInput{
			ChangeCounterBefore: 7,
			ChangeCounterAfter:  7,
			Pages: []SmartLoggerInventoryPage{{
				Objects: []SmartLoggerInventoryObject{
					{ObjectID: 0x87, Value: []byte{1}},
					{ObjectID: 0x88, Value: []byte("1=SmartLogger;2=V300R024C10SPC191;5=0")},
					{ObjectID: 0x89, Value: []byte("1=SUN2000;2=V300R024C10SPC191;3=V1;4=synthetic-private-input;5=1;6=F1;8=PV")},
				},
			}},
		},
	}
}

func assertHuaweiQualificationFailureHasNoFallback(t *testing.T, result QualificationResult) {
	t.Helper()
	if result.IdentityMatched() || result.SelectedClass() != "" || result.Readiness() != QualificationInsufficientEvidence ||
		result.NativeFactCount() != 0 || result.AutomaticRequestCount() != 0 || result.FallbackAttempted() || !result.DefaultDenied() {
		t.Fatalf("failed classification created fallback, telemetry, or request: %#v", result)
	}
}

func assertHuaweiQualificationSteps(t *testing.T, got, want []QualificationStep) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("qualification steps=%#v want=%#v", got, want)
	}
	for _, step := range got {
		if step.Operation() == "" || step.UnitID() != step.unitID || step.Offset() != step.offset || step.Quantity() != step.quantity || step.ObjectID() != step.objectID {
			t.Fatalf("qualification step accessors disagree: %#v", step)
		}
	}
}
