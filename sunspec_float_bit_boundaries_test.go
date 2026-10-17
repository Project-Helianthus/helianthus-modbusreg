package modbusreg

import (
	"math"
	"testing"
)

func TestU22Float32FiniteBoundaryBits(t *testing.T) {
	tests := []struct {
		name     string
		input    uint32
		wantBits uint32
	}{
		{name: "positive_zero", input: 0x00000000, wantBits: 0x00000000},
		{name: "negative_zero", input: 0x80000000, wantBits: 0x80000000},
		{name: "smallest_positive_subnormal", input: 0x00000001, wantBits: 0x00000001},
		{name: "smallest_negative_subnormal", input: 0x80000001, wantBits: 0x80000001},
		{name: "largest_positive_finite", input: 0x7f7fffff, wantBits: 0x7f7fffff},
		{name: "largest_negative_finite", input: 0xff7fffff, wantBits: 0xff7fffff},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			words := []uint16{uint16(tc.input >> 16), uint16(tc.input)}
			value := decodeSunSpecValue(sunSpecPointDefinition{pointType: SunSpecTypeFloat32, size: 2}, words, nil)
			if value.State() != SunSpecValueValid {
				t.Fatalf("state=%s, want %s", value.State(), SunSpecValueValid)
			}
			got, ok := value.Float32()
			if !ok || math.Float32bits(got) != tc.wantBits {
				t.Fatalf("Float32 bits=%08x present=%v, want bits=%08x present=true", math.Float32bits(got), ok, tc.wantBits)
			}
			if raw := value.RawWords(); len(raw) != 2 || raw[0] != words[0] || raw[1] != words[1] {
				t.Fatalf("raw=%04x, want %04x", raw, words)
			}
		})
	}
}

func TestU22Float32NonfiniteStates(t *testing.T) {
	tests := []struct {
		name  string
		input uint32
		state SunSpecValueState
	}{
		{name: "canonical_missing_nan", input: 0x7fc00000, state: SunSpecValueNotImplemented},
		{name: "positive_quiet_nan_payload", input: 0x7fc00001, state: SunSpecValueInvalidEncoding},
		{name: "negative_quiet_nan_payload", input: 0xffc00001, state: SunSpecValueInvalidEncoding},
		{name: "negative_canonical_shaped_nan", input: 0xffc00000, state: SunSpecValueInvalidEncoding},
		{name: "positive_signaling_nan", input: 0x7f800001, state: SunSpecValueInvalidEncoding},
		{name: "negative_signaling_nan", input: 0xff800001, state: SunSpecValueInvalidEncoding},
		{name: "positive_infinity", input: 0x7f800000, state: SunSpecValueInvalidEncoding},
		{name: "negative_infinity", input: 0xff800000, state: SunSpecValueInvalidEncoding},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			words := []uint16{uint16(tc.input >> 16), uint16(tc.input)}
			value := decodeSunSpecValue(sunSpecPointDefinition{pointType: SunSpecTypeFloat32, size: 2}, words, nil)
			if value.State() != tc.state {
				t.Fatalf("state=%s, want %s", value.State(), tc.state)
			}
			if got, ok := value.Float32(); ok {
				t.Fatalf("nonfinite encoding exposes Float32 bits=%08x", math.Float32bits(got))
			}
			if raw := value.RawWords(); len(raw) != 2 || raw[0] != words[0] || raw[1] != words[1] {
				t.Fatalf("raw=%04x, want %04x", raw, words)
			}
		})
	}
}
