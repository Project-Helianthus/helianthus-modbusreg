package modbusreg_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

type mixedCatalogClosure struct {
	Schema       string `json:"schema"`
	Milestone    string `json:"milestone"`
	Dependencies struct {
		SunSpecExpansionMerge    string `json:"sunspec_expansion_merge"`
		GrowattDispositionMerge  string `json:"growatt_disposition_merge"`
		HuaweiDispositionMerge   string `json:"huawei_disposition_merge"`
		VendorDocumentationMerge string `json:"vendor_docs_merge"`
		MixedCatalogDocsMerge    string `json:"mixed_catalog_docs_merge"`
	} `json:"dependencies"`
	Selection struct {
		SelectionMode                     string   `json:"selection_mode"`
		MaxSelectedPrimaries              int      `json:"max_selected_primaries"`
		NoPositiveOutcome                 string   `json:"no_positive_outcome"`
		MultiplePositiveOutcome           string   `json:"multiple_positive_outcome"`
		SerializedMultiplePositiveOutcome string   `json:"serialized_multiple_positive_outcome"`
		SerializedMultiplePositiveReason  string   `json:"serialized_multiple_positive_reason"`
		ImplicitPriority                  bool     `json:"implicit_priority"`
		SelectionIsStateless              bool     `json:"selection_is_stateless"`
		ActivationMutation                bool     `json:"activation_mutation"`
		EligibleProfileRequirements       []string `json:"eligible_profile_requirements"`
	} `json:"selection"`
	Participants []struct {
		ID              string `json:"id"`
		Role            string `json:"role"`
		Disposition     string `json:"disposition"`
		RuntimeIdentity string `json:"runtime_identity"`
		DispositionFile string `json:"disposition_file"`
	} `json:"participants"`
}

func readMixedCatalogClosure(t *testing.T) mixedCatalogClosure {
	t.Helper()
	data, err := os.ReadFile("profiles/mixed-catalog-closure-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var closure mixedCatalogClosure
	if err := decoder.Decode(&closure); err != nil {
		t.Fatal(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("mixed-catalog closure has trailing JSON: %v", err)
	}
	return closure
}

func TestMixedCatalogClosurePinsParticipantsAndFailClosedPolicy(t *testing.T) {
	closure := readMixedCatalogClosure(t)
	if closure.Schema != "helianthus-modbusreg-mixed-catalog-closure/v1" || closure.Milestone != "FMV3-M7-05" {
		t.Fatalf("unexpected closure identity: %#v", closure)
	}
	if closure.Dependencies.SunSpecExpansionMerge != "7137bf4e3cb5cf84dc581b490fa9c9ddf8ea49f6" ||
		closure.Dependencies.GrowattDispositionMerge != "5c76899f6e15c52110e14e33e0cc25fe2f3a452b" ||
		closure.Dependencies.HuaweiDispositionMerge != "110fe6417b511d8cacfdf22fce5d3d5581672c32" ||
		closure.Dependencies.VendorDocumentationMerge != "aa67e0c2a7c2042c7c1dccad6ebe3c4900dab04f" ||
		closure.Dependencies.MixedCatalogDocsMerge != "736fd599cf0128b32257c178b454114893b5dc57" {
		t.Fatalf("unexpected dependency closure: %#v", closure.Dependencies)
	}
	wantRequirements := []string{"qualified", "active", "default_enabled", "candidate_enabled"}
	if closure.Selection.SelectionMode != "exclusive" ||
		closure.Selection.MaxSelectedPrimaries != 1 ||
		closure.Selection.NoPositiveOutcome != "NO_MATCH" ||
		closure.Selection.MultiplePositiveOutcome != "INSUFFICIENT_EVIDENCE" ||
		closure.Selection.SerializedMultiplePositiveOutcome != string(reg.DetectionAmbiguous) ||
		closure.Selection.SerializedMultiplePositiveReason != string(reg.DetectionReasonMultipleMatches) ||
		closure.Selection.ImplicitPriority || !closure.Selection.SelectionIsStateless ||
		closure.Selection.ActivationMutation ||
		!reflect.DeepEqual(closure.Selection.EligibleProfileRequirements, wantRequirements) {
		t.Fatalf("unexpected selection policy: %#v", closure.Selection)
	}

	wantParticipants := []struct {
		id, role, disposition, runtimeIdentity, dispositionFile string
	}{
		{"sunspec.direct-inverter", "PRIMARY", "CONDITIONAL_CAPABILITY", reg.SunSpecThreePhaseMonitoringCapabilityID, ""},
		{"fronius.gen24", "POST_PRIMARY_FLAVOR", "QUALIFIED_FLAVOR_CLASSIFICATION", reg.SunSpecFroniusObservedFlavorID + "," + reg.SunSpecFroniusObservedFlavorV11ID, ""},
		{"growatt.direct-inverter", "PRIMARY_CANDIDATE", "NO_ADMISSIBLE_PROFILE", "", "profiles/vendor/growatt/disposition.json"},
		{"huawei.smartlogger", "PRIMARY_CANDIDATE", "NO_ADMISSIBLE_PROFILE", "", "profiles/vendor/huawei/smartlogger-disposition.json"},
		{"huawei.sdongle", "PRIMARY_CANDIDATE", "PRE_LIVE_INSUFFICIENT_EVIDENCE", "", "profiles/vendor/huawei/sdongle-disposition.json"},
		{"huawei.emma", "PRIMARY_CANDIDATE", "OFFLINE_IDENTITY_ADMITTED", "", "profiles/vendor/huawei/emma-disposition.json"},
	}
	if len(closure.Participants) != len(wantParticipants) {
		t.Fatalf("participants=%d want=%d", len(closure.Participants), len(wantParticipants))
	}
	for index, want := range wantParticipants {
		got := closure.Participants[index]
		if got.ID != want.id || got.Role != want.role || got.Disposition != want.disposition ||
			got.RuntimeIdentity != want.runtimeIdentity || got.DispositionFile != want.dispositionFile {
			t.Fatalf("participant %d=%#v want=%#v", index, got, want)
		}
		if got.DispositionFile == "" {
			continue
		}
		data, err := os.ReadFile(got.DispositionFile)
		if err != nil {
			t.Fatal(err)
		}
		var disposition struct {
			Outcome           string `json:"outcome"`
			CatalogRegistered bool   `json:"catalog_registered"`
			SupportClaim      bool   `json:"support_claim"`
		}
		if err := json.Unmarshal(data, &disposition); err != nil {
			t.Fatal(err)
		}
		if disposition.Outcome != got.Disposition || disposition.CatalogRegistered || disposition.SupportClaim {
			t.Fatalf("participant %s became actionable: %#v", got.ID, disposition)
		}
	}
}

func TestMixedCatalogExclusiveSelectionRejectsUnequalScoreOverlap(t *testing.T) {
	profiles := []reg.ProfileDescriptor{
		detectionProfile(t, "fixture.sunspec.direct", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true),
		detectionProfile(t, "fixture.huawei.smartlogger", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true),
		detectionProfile(t, "fixture.huawei.sdongle", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true),
		detectionProfile(t, "fixture.huawei.emma", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true),
	}
	scores := []uint32{100, 80, 60, 40}

	for _, order := range [][]int{{0, 1, 2, 3}, {3, 1, 0, 2}} {
		catalogProfiles := make([]reg.ProfileDescriptor, len(order))
		candidates := make([]reg.DetectionCandidate, len(order))
		for index, source := range order {
			catalogProfiles[index] = profiles[source]
			candidates[index] = detectionCandidate(t, profiles[source], scores[source], true, true)
		}
		catalog, err := reg.NewCatalog(catalogProfiles...)
		if err != nil {
			t.Fatalf("NewCatalog: %v", err)
		}
		decision, err := newDetector(t, catalog, candidates...).Detect(
			context.Background(),
			detectionReader(t),
			reg.DetectionOptions{AllowFixtureOnly: true},
		)
		if err != nil || decision.Outcome() != reg.DetectionAmbiguous ||
			decision.Reason() != reg.DetectionReasonMultipleMatches ||
			decision.SelectedProfileID() != "" {
			t.Fatalf("exclusive overlap decision=(%+v,%v)", decision, err)
		}
		for _, evidence := range decision.Evidence() {
			if evidence.Reason != reg.DetectionReasonMultipleMatches {
				t.Fatalf("overlap evidence for %s=%q", evidence.ProfileID, evidence.Reason)
			}
		}
		encoded, err := reg.MarshalDetectionDecision(decision)
		if err != nil {
			t.Fatalf("MarshalDetectionDecision: %v", err)
		}
		roundTrip, err := reg.UnmarshalDetectionDecision(encoded)
		if err != nil || roundTrip.Outcome() != reg.DetectionAmbiguous || roundTrip.Reason() != reg.DetectionReasonMultipleMatches {
			t.Fatalf("round-trip decision=(%+v,%v)", roundTrip, err)
		}
	}
}

func TestMixedCatalogExclusiveSelectionPreservesEligibilityLifecycle(t *testing.T) {
	eligible := detectionProfile(t, "fixture.sunspec.eligible", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	unqualified := detectionProfile(t, "fixture.huawei.emma", "1.0.0", reg.MaturityExperimental, reg.ProfileActive, false)
	revoked := detectionProfile(t, "fixture.huawei.smartlogger", "1.0.0", reg.MaturityQualified, reg.ProfileRevoked, false)
	replacement := detectionProfile(t, "fixture.huawei.sdongle.v2", "2.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	supersededSpec := detectionProfile(t, "fixture.huawei.sdongle", "1.0.0", reg.MaturityQualified, reg.ProfileActive, false).Spec()
	supersededSpec.State = reg.ProfileSuperseded
	supersededSpec.SupersededByID = replacement.ID()
	supersededSpec.SupersededByVersion = replacement.Version()
	superseded, err := reg.NewProfileDescriptor(supersededSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(superseded): %v", err)
	}
	catalog, err := reg.NewCatalog(eligible, unqualified, revoked, superseded, replacement)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	detector := newDetector(t, catalog,
		detectionCandidate(t, unqualified, 400, true, true),
		detectionCandidate(t, revoked, 300, true, true),
		detectionCandidate(t, superseded, 200, true, true),
		detectionCandidate(t, eligible, 1, true, true),
	)
	decision, err := detector.Detect(
		context.Background(),
		detectionReader(t),
		reg.DetectionOptions{AllowFixtureOnly: true},
	)
	if err != nil || decision.Outcome() != reg.DetectionMatched || decision.SelectedProfileID() != eligible.ID() {
		t.Fatalf("lifecycle decision=(%+v,%v)", decision, err)
	}
	reasons := make(map[string]reg.DetectionReason)
	for _, evidence := range decision.Evidence() {
		reasons[evidence.ProfileID] = evidence.Reason
	}
	want := map[string]reg.DetectionReason{
		unqualified.ID(): reg.DetectionReasonProfileUnqualified,
		revoked.ID():     reg.DetectionReasonProfileRevoked,
		superseded.ID():  reg.DetectionReasonProfileSuperseded,
	}
	for profileID, reason := range want {
		if reasons[profileID] != reason {
			t.Fatalf("reason[%s]=%q want=%q", profileID, reasons[profileID], reason)
		}
	}
}
