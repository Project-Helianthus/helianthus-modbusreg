package modbusreg_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func q27Decision(t *testing.T, outcome reg.DetectionOutcome) []byte {
	t.Helper()
	alpha := detectionProfile(t, "fixture.alpha", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	beta := detectionProfile(t, "fixture.beta", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	catalog, err := reg.NewCatalog(alpha, beta)
	if err != nil {
		t.Fatal(err)
	}
	candidates := []reg.DetectionCandidate{detectionCandidate(t, beta, 20, outcome != reg.DetectionNoMatch, true)}
	wantReason := reg.DetectionReasonSelected
	wantID, wantVersion := "fixture.beta", "1.0.0"
	if outcome == reg.DetectionAmbiguous {
		candidates = append(candidates, detectionCandidate(t, alpha, 10, true, true))
		wantReason = reg.DetectionReasonMultipleMatches
	}
	if outcome == reg.DetectionNoMatch {
		wantReason = reg.DetectionReasonCandidateDisabled
	}
	if outcome != reg.DetectionMatched {
		wantID, wantVersion = "", ""
	}
	decision, err := newDetector(t, catalog, candidates...).Detect(context.Background(), detectionReader(t), reg.DetectionOptions{AllowFixtureOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Outcome() != outcome || decision.Reason() != wantReason || decision.SelectedProfileID() != wantID || decision.SelectedProfileVersion().String() != wantVersion {
		t.Fatalf("unexpected baseline: %+v", decision)
	}
	if outcome == reg.DetectionAmbiguous {
		evidence := decision.Evidence()
		if len(evidence) != 2 || evidence[0].Score != 10 || evidence[1].Score != 20 || evidence[0].Reason != reg.DetectionReasonMultipleMatches || evidence[1].Reason != reg.DetectionReasonMultipleMatches {
			t.Fatalf("unequal-score overlap lost exclusive evidence: %+v", evidence)
		}
	}
	encoded, err := reg.MarshalDetectionDecision(decision)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := reg.UnmarshalDetectionDecision(encoded)
	if err != nil {
		t.Fatal(err)
	}
	reencoded, err := reg.MarshalDetectionDecision(restored)
	if err != nil || !bytes.Equal(encoded, reencoded) {
		t.Fatalf("unmodified decision did not round-trip: %v", err)
	}
	return encoded
}

func q27Replace(t *testing.T, data []byte, old, replacement string) []byte {
	t.Helper()
	if old == "" || old == replacement || bytes.Count(data, []byte(old)) != 1 {
		t.Fatalf("mutation must have exactly one nonempty changed target: %q", old)
	}
	return bytes.Replace(data, []byte(old), []byte(replacement), 1)
}

func q27Reject(t *testing.T, data []byte, want string) {
	t.Helper()
	if !json.Valid(data) {
		t.Fatal("mutation broke JSON syntax")
	}
	// These errors originate in validateDetectionDecision, after strict decoding,
	// version parsing, and canonical evidence ordering have succeeded.
	if _, err := reg.UnmarshalDetectionDecision(data); err == nil || err.Error() != want {
		t.Fatalf("decode error = %v; want semantic validator error %q", err, want)
	}
}

func TestQ27MatchedSelectionConsistency(t *testing.T) {
	baseline := q27Decision(t, reg.DetectionMatched)
	for _, tc := range []struct{ name, old, replacement, want string }{
		{"selected_profile_id", `"selected_profile_id":"fixture.beta"`, `"selected_profile_id":"fixture.alpha"`, "selected profile is absent from decision evidence"},
		{"selected_profile_version", `"selected_profile_version":"1.0.0"`, `"selected_profile_version":"2.0.0"`, "selected profile is absent from decision evidence"},
		{"selected_evidence_reason_zero_selected", `"score":20,"reason":"selected"`, `"score":20,"reason":"lower_score"`, "matched detection decision is inconsistent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q27Reject(t, q27Replace(t, baseline, tc.old, tc.replacement), tc.want)
		})
	}
	t.Run("two_selected_evidence_records", func(t *testing.T) {
		var record struct {
			Evidence []json.RawMessage `json:"evidence"`
		}
		if err := json.Unmarshal(baseline, &record); err != nil || len(record.Evidence) != 1 {
			t.Fatalf("baseline evidence: %v", err)
		}
		// Insert one complete, independently valid record before beta. The last
		// selected record still joins the selected profile, isolating the count guard.
		alpha := q27Replace(t, record.Evidence[0], `"profile_id":"fixture.beta"`, `"profile_id":"fixture.alpha"`)
		mutated := q27Replace(t, baseline, `"evidence":[`, `"evidence":[`+string(alpha)+`,`)
		q27Reject(t, mutated, "matched detection decision is inconsistent")
	})
}

func TestQ27NoMatchSelectionConsistency(t *testing.T) {
	baseline := q27Decision(t, reg.DetectionNoMatch)
	for _, field := range []struct{ name, value string }{
		{"selected_profile_id", "fixture.beta"},
		{"selected_profile_version", "1.0.0"},
	} {
		t.Run(field.name, func(t *testing.T) {
			q27Reject(t, q27Replace(t, baseline, `"`+field.name+`":""`, `"`+field.name+`":"`+field.value+`"`), "no-match detection decision is inconsistent")
		})
	}
}

func TestQ27AmbiguousSelectionConsistency(t *testing.T) {
	baseline := q27Decision(t, reg.DetectionAmbiguous)
	for _, tc := range []struct{ name, old, replacement, want string }{
		{"selected_profile_id", `"selected_profile_id":""`, `"selected_profile_id":"fixture.beta"`, "ambiguous detection decision is inconsistent"},
		{"selected_profile_version", `"selected_profile_version":""`, `"selected_profile_version":"1.0.0"`, "ambiguous detection decision is inconsistent"},
		{"selected_evidence_reason", `"score":20,"reason":"multiple_matches"`, `"score":20,"reason":"selected"`, "ambiguous detection decision is inconsistent"},
		{"legacy_lower_score_reason", `"score":10,"reason":"multiple_matches"`, `"score":10,"reason":"lower_score"`, "exclusive detection decision is inconsistent"},
		{"legacy_equal_best_decision_reason", `"outcome":"ambiguous","reason":"multiple_matches"`, `"outcome":"ambiguous","reason":"equal_best"`, "ambiguous detection decision is inconsistent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q27Reject(t, q27Replace(t, baseline, tc.old, tc.replacement), tc.want)
		})
	}
}
