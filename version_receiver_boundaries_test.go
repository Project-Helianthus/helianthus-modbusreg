package modbusreg_test

import (
	"strings"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestU28VersionReceiverPreservesAcceptedValue(t *testing.T) {
	var receiver reg.Version
	if err := receiver.UnmarshalJSON([]byte(`"7.8.9"`)); err != nil {
		t.Fatalf("initial decode: %v", err)
	}
	if got := receiver.String(); got != "7.8.9" {
		t.Fatalf("initial value = %q, want 7.8.9", got)
	}
	original := receiver
	for _, tc := range []struct {
		name string
		json string
	}{
		{"null", `null`},
		{"numeric_json", `123`},
		{"malformed_string", `"7.8"`},
		{"malformed_json", `"7.8.9`},
		{"leading_zero", `"07.8.9"`},
		{"overlength_valid_syntax", `"` + strings.Repeat("9", reg.MaxContractStringBytes-3) + `.0.0"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := receiver.UnmarshalJSON([]byte(tc.json)); err == nil {
				t.Error("invalid update succeeded")
			}
			if receiver != original {
				t.Errorf("failed update changed receiver: got %q, want %q", receiver.String(), original.String())
			}
		})
	}
	if err := receiver.UnmarshalJSON([]byte(`"10.11.12"`)); err != nil {
		t.Fatalf("later valid update: %v", err)
	}
	if got := receiver.String(); got != "10.11.12" {
		t.Fatalf("later value = %q, want 10.11.12", got)
	}
}

func TestU28VersionExactStringBoundary(t *testing.T) {
	// ASCII digits keep byte length explicit without converting a component to an integer.
	exact := strings.Repeat("9", reg.MaxContractStringBytes-len(".0.0")) + ".0.0"
	over := "9" + exact
	for _, tc := range []struct {
		name  string
		value string
		valid bool
	}{
		{"exact_limit", exact, true},
		{"one_byte_over", over, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := reg.ParseVersion(tc.value)
			if tc.valid {
				if err != nil {
					t.Fatalf("ParseVersion exact limit: %v", err)
				}
				if parsed.String() != tc.value {
					t.Error("ParseVersion changed the large numeric component")
				}
			} else if err == nil {
				t.Error("ParseVersion accepted one byte over the limit")
			}

			var receiver reg.Version
			if err := receiver.UnmarshalJSON([]byte(`"4.5.6"`)); err != nil {
				t.Fatalf("seed decode: %v", err)
			}
			original := receiver
			err = receiver.UnmarshalJSON([]byte(`"` + tc.value + `"`))
			if tc.valid {
				if err != nil {
					t.Fatalf("decode exact limit: %v", err)
				}
				if receiver.String() != tc.value {
					t.Error("decode changed the large numeric component")
				}
			} else {
				if err == nil {
					t.Error("decode accepted one byte over the limit")
				}
				if receiver != original {
					t.Error("overlimit decode changed the accepted receiver")
				}
			}
		})
	}
}
