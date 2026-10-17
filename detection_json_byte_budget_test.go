package modbusreg_test

import (
	"bytes"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestQ30DetectionDecisionSerializedByteBudget(t *testing.T) {
	// Every vector has the same small decision; only legal JSON whitespace changes.
	valid := []byte(`{"schema_version":"1.0.0","outcome":"no_match","reason":"invalid_identity","selected_profile_id":"","selected_profile_version":"","evidence":[{"profile_id":"synthetic.family","profile_version":"1.0.0","score":1,"reason":"invalid_identity","matched_gates":[],"probe_evidence_ids":[],"detector_version":"1.0.0","qualification_version":"1.0.0"}]}`)
	assertDecision := func(t *testing.T, data []byte) {
		t.Helper()
		decision, err := reg.UnmarshalDetectionDecision(data)
		if err != nil {
			t.Fatalf("valid decision (%d bytes) rejected: %v", len(data), err)
		}
		if decision.Outcome() != reg.DetectionNoMatch || decision.Reason() != reg.DetectionReasonInvalidIdentity {
			t.Fatal("decoded decision changed outcome or reason")
		}
		canonical, err := reg.MarshalDetectionDecision(decision)
		if err != nil || !bytes.Equal(canonical, valid) {
			t.Fatalf("decoded decision changed canonical content: %v", err)
		}
	}
	t.Run("unpadded_control", func(t *testing.T) { assertDecision(t, valid) })
	if len(valid) >= reg.MaxSerializedContractBytes {
		t.Fatal("control must leave room for whitespace padding")
	}

	// One reusable limit+1 buffer bounds test-owned large allocations. Subtests
	// run sequentially; no corpus or retained copies of padded inputs are built.
	buffer := make([]byte, reg.MaxSerializedContractBytes+1)
	for _, placement := range []string{"leading", "trailing"} {
		t.Run(placement, func(t *testing.T) {
			for _, extra := range []int{0, 1} {
				name := "exact_limit"
				if extra != 0 {
					name = "one_byte_over"
				}
				t.Run(name, func(t *testing.T) {
					data := buffer[:reg.MaxSerializedContractBytes+extra]
					for index := range data {
						data[index] = " \t\r\n"[index%4]
					}
					offset := 0
					if placement == "leading" {
						offset = len(data) - len(valid)
					}
					copy(data[offset:], valid)
					if len(data) != reg.MaxSerializedContractBytes+extra ||
						!bytes.Equal(bytes.TrimSpace(data), valid) {
						t.Fatal("vector must have the exact byte length and unchanged decision")
					}
					if extra == 0 {
						assertDecision(t, data)
						return
					}
					if _, err := reg.UnmarshalDetectionDecision(data); err == nil || err.Error() != "serialized contract exceeds the byte boundary" {
						t.Fatalf("one byte over limit must fail at byte preflight: %v", err)
					}
				})
			}
		})
	}
	t.Run("second_value_within_budget", func(t *testing.T) {
		data := buffer[:2*len(valid)+1]
		copy(data, valid)
		data[len(valid)] = '\n'
		copy(data[len(valid)+1:], valid)
		if len(data) > reg.MaxSerializedContractBytes ||
			!bytes.Equal(data[:len(valid)], valid) ||
			!bytes.Equal(data[len(valid)+1:], valid) {
			t.Fatal("multiple-value vector must contain two valid decisions within budget")
		}
		// Pin rejection in preflight, before the second strict decoder also checks EOF.
		if _, err := reg.UnmarshalDetectionDecision(data); err == nil || err.Error() != "serialized contract contains multiple values" {
			t.Fatalf("second JSON value must fail at trailing-token preflight: %v", err)
		}
	})
}
