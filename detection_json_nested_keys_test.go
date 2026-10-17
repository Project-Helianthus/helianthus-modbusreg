package modbusreg_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestQ25DetectionDecisionNestedKeyCollisions(t *testing.T) {
	profile := detectionProfile(t, "example.standard.json-keys", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	catalog, err := reg.NewCatalog(profile)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	detector := newDetector(t, catalog, detectionCandidate(t, profile, 100, true, false))
	decision, err := detector.Detect(context.Background(), detectionReader(t), reg.DetectionOptions{})
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if decision.Outcome() != reg.DetectionMatched || decision.SelectedProfileID() != profile.ID() ||
		len(decision.Evidence()) != 1 || len(decision.Evidence()[0].ProbeEvidenceIDs) != 3 {
		t.Fatalf("expected a matched decision with nonempty evidence: %+v", decision)
	}
	encoded, err := reg.MarshalDetectionDecision(decision)
	if err != nil {
		t.Fatalf("MarshalDetectionDecision: %v", err)
	}
	assertAccepted := func(t *testing.T, data []byte) {
		t.Helper()
		restored, err := reg.UnmarshalDetectionDecision(data)
		if err != nil {
			t.Fatalf("valid decision rejected: %v", err)
		}
		reencoded, err := reg.MarshalDetectionDecision(restored)
		if err != nil || !bytes.Equal(reencoded, encoded) {
			t.Fatalf("valid decision changed during round trip: %v", err)
		}
	}
	assertAccepted(t, encoded)

	// Extract the original evidence text without rebuilding it through a map.
	var raw struct {
		Evidence []json.RawMessage `json:"evidence"`
	}
	if err := json.Unmarshal(encoded, &raw); err != nil || len(raw.Evidence) != 1 {
		t.Fatalf("extract evidence: count=%d error=%v", len(raw.Evidence), err)
	}
	base := string(encoded)
	for _, scope := range []struct {
		name   string
		object string
	}{
		{"root", base},
		{"evidence", string(raw.Evidence[0])},
	} {
		t.Run(scope.name, func(t *testing.T) {
			const member = `"reason":"selected"`
			object := scope.object
			if !strings.HasPrefix(object, "{") || !strings.HasSuffix(object, "}") ||
				strings.Count(object, member) < 1 || strings.Count(base, object) != 1 {
				t.Fatal("nonempty raw JSON mutation target is absent or ambiguous")
			}
			// At the root the first reason belongs to the decision; inside the
			// extracted object it belongs to the sole evidence record.
			keyOffset := strings.Index(object, member)
			escaped := object[:keyOffset] + strings.Replace(object[keyOffset:], `"reason"`, `"\u0072eason"`, 1)
			t.Run("single_escaped_canonical_key", func(t *testing.T) {
				assertAccepted(t, []byte(strings.Replace(base, object, escaped, 1)))
			})

			for _, spelling := range []struct {
				name string
				key  string
			}{
				{"duplicate", `"reason"`},
				{"case_variant", `"Reason"`},
				{"escape_equivalent", `"\u0072eason"`},
			} {
				for _, position := range []string{"first", "adjacent", "last"} {
					t.Run(spelling.name+"/"+position, func(t *testing.T) {
						// Equal values keep the decoded decision valid if duplicate
						// checking is removed, regardless of overwrite order.
						extra := spelling.key + `:"selected"`
						var mutated string
						switch position {
						case "first":
							mutated = "{" + extra + "," + object[1:]
						case "adjacent":
							mutated = object[:keyOffset] + extra + "," + object[keyOffset:]
						case "last":
							mutated = object[:len(object)-1] + "," + extra + "}"
						}
						candidate := []byte(strings.Replace(base, object, mutated, 1))
						if bytes.Equal(candidate, encoded) || !json.Valid(candidate) {
							t.Fatal("collision mutation did not produce changed, syntactically valid JSON")
						}
						if _, err := reg.UnmarshalDetectionDecision(candidate); err == nil {
							t.Fatal("decision with a key collision was accepted")
						}
					})
				}
			}
		})
	}
}
