package modbusreg

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestS25SunSpecStringPaddingMatrix(t *testing.T) {
	tests := []struct {
		name    string
		words   []uint16
		raw     []byte
		state   SunSpecValueState
		text    string
		hasText bool
	}{
		{name: "all_zero_single_word", words: []uint16{0}, raw: []byte{0, 0}, state: SunSpecValueNotImplemented},
		{name: "all_zero_multiple_words", words: []uint16{0, 0, 0}, raw: []byte{0, 0, 0, 0, 0, 0}, state: SunSpecValueNotImplemented},
		{name: "explicit_empty_single_word", words: []uint16{0x0080}, raw: []byte{0, 0x80}, state: SunSpecValueValid, hasText: true},
		{name: "explicit_empty_zero_tail", words: []uint16{0x0080, 0, 0}, raw: []byte{0, 0x80, 0, 0, 0, 0}, state: SunSpecValueValid, hasText: true},
		{name: "explicit_empty_nonzero_high_tail", words: []uint16{0x0080, 0x4100, 0}, raw: []byte{0, 0x80, 'A', 0, 0, 0}, state: SunSpecValueInvalidEncoding},
		{name: "explicit_empty_nonzero_low_tail", words: []uint16{0x0080, 0, 0x0041}, raw: []byte{0, 0x80, 0, 0, 0, 'A'}, state: SunSpecValueInvalidEncoding},
		{name: "utf8_ends_low_byte_full_width", words: []uint16{0xc483}, raw: []byte{0xc4, 0x83}, state: SunSpecValueValid, text: "ă", hasText: true},
		{name: "utf8_ends_low_byte_zero_tail", words: []uint16{0xc483, 0}, raw: []byte{0xc4, 0x83, 0, 0}, state: SunSpecValueValid, text: "ă", hasText: true},
		{name: "utf8_ends_high_byte", words: []uint16{0x41c4, 0x8300, 0}, raw: []byte{'A', 0xc4, 0x83, 0, 0, 0}, state: SunSpecValueValid, text: "Aă", hasText: true},
		{name: "three_byte_utf8_ends_high_byte", words: []uint16{0xe282, 0xac00}, raw: []byte{0xe2, 0x82, 0xac, 0}, state: SunSpecValueValid, text: "€", hasText: true},
		{name: "nul_low_byte_nonzero_tail", words: []uint16{0x4100, 0x4200}, raw: []byte{'A', 0, 'B', 0}, state: SunSpecValueInvalidEncoding},
		{name: "nul_high_byte_nonzero_tail", words: []uint16{0x4142, 0x0043}, raw: []byte{'A', 'B', 0, 'C'}, state: SunSpecValueInvalidEncoding},
		{name: "truncated_utf8_before_nul", words: []uint16{0x41e2, 0x8200}, raw: []byte{'A', 0xe2, 0x82, 0}, state: SunSpecValueInvalidEncoding},
		{name: "truncated_utf8_at_field_end", words: []uint16{0xe282}, raw: []byte{0xe2, 0x82}, state: SunSpecValueInvalidEncoding},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := append([]uint16(nil), tc.words...)
			value := decodeSunSpecValue(sunSpecPointDefinition{pointType: SunSpecTypeString, size: uint16(len(input))}, input, nil)
			assertValue := func(stage string) {
				t.Helper()
				if got := value.State(); got != tc.state {
					t.Errorf("%s: State() = %q, want %q", stage, got, tc.state)
				}
				text, hasText := value.Text()
				if text != tc.text {
					t.Errorf("%s: Text() = %q, want %q", stage, text, tc.text)
				}
				if hasText != tc.hasText {
					t.Errorf("%s: text presence = %v, want %v", stage, hasText, tc.hasText)
				}
				words := value.RawWords()
				raw := make([]byte, len(words)*2)
				for i, word := range words {
					binary.BigEndian.PutUint16(raw[i*2:], word)
				}
				if !bytes.Equal(raw, tc.raw) {
					t.Errorf("%s: raw bytes = %x, want %x", stage, raw, tc.raw)
				}
			}
			assertValue("decoded")
			for i := range input {
				input[i] = ^input[i]
			}
			assertValue("caller input mutated")
			returned := value.RawWords()
			for i := range returned {
				returned[i] = ^returned[i]
			}
			assertValue("returned raw words mutated")
		})
	}
}
