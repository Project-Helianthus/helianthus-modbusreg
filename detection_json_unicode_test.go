package modbusreg

import (
	"bytes"
	"encoding/json"
	"testing"
)

// Decision strings are identities, versions, or enums, so Unicode controls
// exercise the real DTO before semantic validation rather than relaxing it.
const q26ASCIIDecision = `{"schema_version":"1.0.0","outcome":"no_match","reason":"invalid_identity","selected_profile_id":"","selected_profile_version":"","evidence":[{"profile_id":"example.standard.unicode","profile_version":"1.0.0","score":1,"reason":"invalid_identity","matched_gates":[],"probe_evidence_ids":[],"detector_version":"1.0.0","qualification_version":"1.0.0"}]}`

func q26DecisionWithProfile(t *testing.T, value []byte) []byte {
	t.Helper()
	target := []byte(`"profile_id":"example.standard.unicode"`)
	data := []byte(q26ASCIIDecision)
	if bytes.Count(data, target) != 1 {
		t.Fatal("expected exactly one nonempty profile mutation target")
	}
	replacement := append([]byte(`"profile_id":"`), value...)
	replacement = append(replacement, '"')
	return bytes.Replace(data, target, replacement, 1)
}

func TestQ26DecisionASCIIControl(t *testing.T) {
	decision, err := UnmarshalDetectionDecision([]byte(q26ASCIIDecision))
	if err != nil {
		t.Fatalf("legal ASCII decision: %v", err)
	}
	evidence := decision.Evidence()
	if decision.Outcome() != DetectionNoMatch || decision.Reason() != DetectionReasonInvalidIdentity ||
		decision.SelectedProfileID() != "" || len(evidence) != 1 ||
		evidence[0].ProfileID != "example.standard.unicode" || evidence[0].Score != 1 {
		t.Fatalf("unexpected ASCII decision: %+v, evidence: %+v", decision, evidence)
	}
	encoded, err := MarshalDetectionDecision(decision)
	if err != nil || !bytes.Equal(encoded, []byte(q26ASCIIDecision)) {
		t.Fatalf("ASCII decision round trip: %s, %v", encoded, err)
	}
}

func TestQ26DecisionRejectsUnicodeBeforeNormalization(t *testing.T) {
	tests := []struct {
		name       string
		value      []byte
		wantError  string
		normalized string
	}{
		{"raw invalid UTF-8", []byte{'x', 0xff}, "serialized contract is not valid UTF-8", "x\ufffd"},
		{"unpaired high surrogate", []byte(`x\ud800`), "serialized string has an unpaired high surrogate", "x\ufffd"},
		{"unpaired low surrogate", []byte(`x\udc00`), "serialized string has an unpaired low surrogate", "x\ufffd"},
		{"truncated Unicode zero digits", []byte(`x\u`), "serialized string has an invalid Unicode escape", ""},
		{"truncated Unicode one digit", []byte(`x\u1`), "serialized string has an invalid Unicode escape", ""},
		{"truncated Unicode two digits", []byte(`x\u12`), "serialized string has an invalid Unicode escape", ""},
		{"truncated Unicode three digits", []byte(`x\u123`), "serialized string has an invalid Unicode escape", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := q26DecisionWithProfile(t, test.value)
			// Show the normalization hazard using the production DTO, without
			// preflight or semantic validation masking encoding/json behavior.
			var record detectionDecisionDTO
			decodeErr := json.Unmarshal(data, &record)
			if test.normalized != "" {
				if decodeErr != nil || len(record.Evidence) != 1 || record.Evidence[0].ProfileID != test.normalized {
					t.Fatalf("encoding/json normalization: %+v, %v; want %q", record, decodeErr, test.normalized)
				}
			} else if decodeErr == nil {
				t.Fatal("encoding/json unexpectedly accepted a truncated Unicode escape")
			}
			if _, err := UnmarshalDetectionDecision(data); err == nil || err.Error() != test.wantError {
				t.Fatalf("public decoder error = %v; want lexical error %q", err, test.wantError)
			}
		})
	}
}

func TestQ26DecisionUnicodeLexicalControls(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"valid surrogate pair", `x\ud83d\ude00`, "x\U0001f600"},
		{"escaped backslash then literal u text", `x\\ud800`, `x\ud800`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := q26DecisionWithProfile(t, []byte(test.value))
			var record detectionDecisionDTO
			if err := decodeStrict(data, &record); err != nil {
				t.Fatalf("lexically valid production DTO rejected: %v", err)
			}
			if len(record.Evidence) != 1 || record.Evidence[0].ProfileID != test.want {
				t.Fatalf("decoded evidence = %+v; want profile %q", record.Evidence, test.want)
			}
			if _, err := UnmarshalDetectionDecision(data); err == nil || err.Error() != "detection candidate evidence is invalid" {
				t.Fatalf("identity constraint error = %v; want detection candidate evidence is invalid", err)
			}
		})
	}
}
