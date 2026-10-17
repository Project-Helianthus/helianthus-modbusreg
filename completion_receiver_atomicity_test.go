package modbusreg_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestS30CompletionReceiverRetainsAcceptedRecordAfterDecodeFailures(t *testing.T) {
	receiver, err := reg.NewCurrentFMV3M303CompletionRecord()
	if err != nil {
		t.Fatalf("construct accepted record: %v", err)
	}
	baseline := receiver.Spec()
	baseline.Applicability = append([]string(nil), baseline.Applicability...)
	baseline.Limitations = append([]string(nil), baseline.Limitations...)
	baseline.InvalidationRollback = append([]string(nil), baseline.InvalidationRollback...)
	if reflect.DeepEqual(baseline, reg.CompletionRecordSpec{}) {
		t.Fatal("accepted receiver must be nonzero")
	}
	encoded, err := reg.MarshalFMV3M303CompletionRecord(receiver)
	if err != nil {
		t.Fatalf("marshal accepted record: %v", err)
	}
	canonical := append([]byte(nil), encoded...)

	mutateField := func(key string, value any) []byte {
		t.Helper()
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(canonical, &fields); err != nil {
			t.Fatalf("decode constructor output for mutation: %v", err)
		}
		field, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("encode mutation: %v", err)
		}
		fields[key] = field
		candidate, err := json.Marshal(fields)
		if err != nil {
			t.Fatalf("encode mutated constructor output: %v", err)
		}
		return candidate
	}
	malformed := append([]byte(nil), canonical...)
	malformed[len(malformed)-1] = '!'
	conclusion := append([]string(nil), baseline.Limitations...)
	if len(conclusion) == 0 {
		t.Fatal("constructor output needs a limitation to mutate")
	}
	conclusion[0] = "synthetic changed conclusion: UNKNOWN"
	cases := []struct {
		name      string
		candidate []byte
		wantError string
	}{
		{name: "malformed JSON", candidate: malformed},
		{
			name: "unknown field", candidate: mutateField("synthetic_unknown", true),
			wantError: `serialized object key "synthetic_unknown" is not canonical`,
		},
		{
			name: "incompatible schema", candidate: mutateField("schema_version", baseline.SchemaVersion+1),
			wantError: "unsupported completion record schema",
		},
		{
			name: "invalid conclusion", candidate: mutateField("limitations", conclusion),
			wantError: "completion record conclusion content is not canonical",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if bytes.Equal(test.candidate, canonical) {
				t.Fatal("corrupt input must differ from the accepted record")
			}
			if valid := json.Valid(test.candidate); valid != (test.wantError != "") {
				t.Fatal("input does not exercise the intended lexical or semantic path")
			}
			err := receiver.UnmarshalJSON(test.candidate)
			if err == nil {
				t.Error("corrupt input was accepted")
			} else if test.wantError == "" {
				var syntaxError *json.SyntaxError
				if !errors.As(err, &syntaxError) {
					t.Errorf("malformed input failed for an unintended reason: %v", err)
				}
			} else if err.Error() != test.wantError {
				t.Errorf("decode error = %q; want %q", err, test.wantError)
			}
			if !reflect.DeepEqual(receiver.Spec(), baseline) {
				t.Error("failed decode changed the accepted receiver Spec")
			}
			retained, err := reg.MarshalFMV3M303CompletionRecord(receiver)
			if err != nil {
				t.Errorf("failed decode left an unusable receiver: %v", err)
			}
			if !bytes.Equal(retained, canonical) {
				t.Error("failed decode changed the accepted receiver canonical bytes")
			}
		})
	}
	t.Run("valid decode after failures", func(t *testing.T) {
		if err := receiver.UnmarshalJSON(canonical); err != nil {
			t.Fatalf("valid decode after failures: %v", err)
		}
		if !reflect.DeepEqual(receiver.Spec(), baseline) {
			t.Error("valid decode did not restore the expected Spec")
		}
		got, err := reg.MarshalFMV3M303CompletionRecord(receiver)
		if err != nil {
			t.Fatalf("marshal after valid decode: %v", err)
		}
		if !bytes.Equal(got, canonical) {
			t.Error("valid decode did not preserve canonical bytes")
		}
	})
}
