package modbusreg

import (
	"reflect"
	"testing"
)

func TestU12BitfieldSentinelResiduals(t *testing.T) {
	for _, tc := range []struct {
		name        string
		pointType   SunSpecPointType
		size        uint16
		words       []uint16
		wantState   SunSpecValueState
		wantBits    uint64
		wantUnknown uint64
		wantPresent bool
		wantSymbols []string
	}{
		{"bitfield16/all_ones", SunSpecTypeBitfield16, 1, []uint16{0xffff}, SunSpecValueNotImplemented, 0, 0, false, nil},
		{"bitfield16/neighbor", SunSpecTypeBitfield16, 1, []uint16{0xfffd}, SunSpecValueValid, 0xfffd, 0xfffc, true, []string{"FLAG_ZERO"}},
		{"bitfield16/last_valid_value", SunSpecTypeBitfield16, 1, []uint16{0xfffe}, SunSpecValueValid, 0xfffe, 0xfffc, true, []string{"FLAG_ONE"}},
		{"bitfield16/low_byte_all_ones", SunSpecTypeBitfield16, 1, []uint16{0x00ff}, SunSpecValueValid, 0x00ff, 0x00fc, true, []string{"FLAG_ZERO", "FLAG_ONE"}},
		{"bitfield32/all_ones", SunSpecTypeBitfield32, 2, []uint16{0xffff, 0xffff}, SunSpecValueNotImplemented, 0, 0, false, nil},
		{"bitfield32/neighbor", SunSpecTypeBitfield32, 2, []uint16{0xffff, 0xfffd}, SunSpecValueValid, 0xfffffffd, 0xfffffffc, true, []string{"FLAG_ZERO"}},
		{"bitfield32/high_word_all_ones_last_valid_value", SunSpecTypeBitfield32, 2, []uint16{0xffff, 0xfffe}, SunSpecValueValid, 0xfffffffe, 0xfffffffc, true, []string{"FLAG_ONE"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Bits zero and one are known; positive controls vary the set bits.
			// The literal unknown masks retain bits two through the width boundary.
			def := sunSpecPointDefinition{
				pointType: tc.pointType, size: tc.size, knownMask: 0x3,
				symbols: map[uint64]string{0: "FLAG_ZERO", 1: "FLAG_ONE", 2: "UNKNOWN_FLAG"},
			}
			input := append([]uint16(nil), tc.words...)
			value := decodeSunSpecValue(def, input, nil)
			assertValue := func() {
				t.Helper()
				if value.State() != tc.wantState {
					t.Errorf("state=%s; want %s", value.State(), tc.wantState)
				}
				bits, unknown, present := value.Bitfield()
				if bits != tc.wantBits || unknown != tc.wantUnknown || present != tc.wantPresent {
					t.Errorf("Bitfield()=(%#x, %#x, %v); want (%#x, %#x, %v)", bits, unknown, present, tc.wantBits, tc.wantUnknown, tc.wantPresent)
				}
				if symbols := value.BitfieldSymbols(); !reflect.DeepEqual(symbols, tc.wantSymbols) {
					t.Errorf("symbols=%v; want %v", symbols, tc.wantSymbols)
				}
				if words := value.RawWords(); !reflect.DeepEqual(words, tc.words) {
					t.Errorf("raw words=%v; want %v", words, tc.words)
				}
			}
			assertValue()

			// Inputs and accessor results must not alias retained native evidence.
			input[0] = 0
			def.symbols[0] = "CHANGED_MAPPING"
			raw := value.RawWords()
			raw[0] = 0
			symbols := value.BitfieldSymbols()
			if len(symbols) > 0 {
				symbols[0] = "CHANGED_RESULT"
			}
			assertValue()
		})
	}
}
