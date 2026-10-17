package modbusreg

import (
	"fmt"
	"testing"
)

func TestS28ProbeASCIIBoundaryVectors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		words   []uint16
		maximum int
		want    string
		invalid bool
	}{
		{name: "printable endpoints high then low", words: []uint16{0x207e}, maximum: 2, want: " ~"},
		{name: "printable endpoints low then high", words: []uint16{0x7e20}, maximum: 2, want: "~ "},
		{name: "asymmetric word order", words: []uint16{0x4162, 0x4331}, maximum: 4, want: "AbC1"},
		{name: "low byte suffix padding", words: []uint16{0x4100}, maximum: 1, want: "A"},
		{name: "whole word suffix padding", words: []uint16{0x4142, 0x0000}, maximum: 2, want: "AB"},
		{name: "suffix spans low and high bytes", words: []uint16{0x4100, 0x0000}, maximum: 1, want: "A"},
		{name: "leading NUL is not padding", words: []uint16{0x0041}, maximum: 2, invalid: true},
		{name: "interior low NUL before suffix", words: []uint16{0x4100, 0x4200}, maximum: 4, invalid: true},
		{name: "interior high NUL before suffix", words: []uint16{0x4142, 0x0043, 0x0000}, maximum: 6, invalid: true},
		{name: "one all padding word", words: []uint16{0x0000}, maximum: 2, invalid: true},
		{name: "multiple all padding words", words: []uint16{0x0000, 0x0000}, maximum: 4, invalid: true},
		{name: "valid UTF8 is still not ASCII", words: []uint16{0x41c2, 0xa242}, maximum: 4, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeProbeASCII(tc.words, tc.maximum)
			if tc.invalid {
				if err == nil || got != "" {
					t.Fatalf("decodeProbeASCII(%04x, %d) = %q, %v; want empty result and error", tc.words, tc.maximum, got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("decodeProbeASCII(%04x, %d) = %q, %v; want %q, nil", tc.words, tc.maximum, got, err, tc.want)
			}
		})
	}
}

func TestS28ProbeASCIIInteriorByteSweep(t *testing.T) {
	for _, position := range []string{"low", "high"} {
		for value := 0; value <= 255; value++ {
			t.Run(fmt.Sprintf("%s/%02x", position, value), func(t *testing.T) {
				words := []uint16{0x4142, 0x4344}
				want := []byte{'A', 'B', 'C', 'D'}
				if position == "low" {
					words[0] = 0x4100 | uint16(value)
					want[1] = byte(value)
				} else {
					words[1] = uint16(value)<<8 | 0x0044
					want[2] = byte(value)
				}
				got, err := decodeProbeASCII(words, 4)
				if value < 0x20 || value > 0x7e {
					if err == nil || got != "" {
						t.Fatalf("interior byte %02x: got %q, %v; want empty result and error", value, got, err)
					}
					return
				}
				if err != nil || got != string(want) {
					t.Fatalf("interior byte %02x: got %q, %v; want %q, nil", value, got, err, want)
				}
			})
		}
	}
}

func TestS28ProbeASCIIDecodedLength(t *testing.T) {
	for _, tc := range []struct {
		name    string
		words   []uint16
		maximum int
		want    string
		invalid bool
	}{
		{name: "even exact maximum", words: []uint16{0x4162, 0x4331}, maximum: 4, want: "AbC1"},
		{name: "even one over maximum", words: []uint16{0x4162, 0x4331}, maximum: 3, invalid: true},
		{name: "odd exact maximum with padding", words: []uint16{0x4162, 0x4300, 0x0000}, maximum: 3, want: "AbC"},
		{name: "odd one over maximum with padding", words: []uint16{0x4162, 0x4300, 0x0000}, maximum: 2, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeProbeASCII(tc.words, tc.maximum)
			if tc.invalid {
				if err == nil || got != "" {
					t.Fatalf("got %q, %v; want empty result and error above maximum %d", got, err, tc.maximum)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q, nil at maximum %d", got, err, tc.want, tc.maximum)
			}
		})
	}
}
