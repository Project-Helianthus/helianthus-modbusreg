package modbusreg

import (
	"fmt"
	"reflect"
	"testing"
)

func TestS26BitfieldOneHotAttribution(t *testing.T) {
	for _, tc := range []struct {
		pointType SunSpecPointType
		width     uint64
		size      uint16
		masks     [2]uint64
	}{
		{SunSpecTypeBitfield16, 16, 1, [2]uint64{0x5555, 0xaaaa}},
		{SunSpecTypeBitfield32, 32, 2, [2]uint64{0x55555555, 0xaaaaaaaa}},
	} {
		for parity, knownMask := range tc.masks {
			for bit := uint64(0); bit < tc.width; bit++ {
				t.Run(fmt.Sprintf("%s/parity_%d/bit_%02d", tc.pointType, parity, bit), func(t *testing.T) {
					def := sunSpecPointDefinition{pointType: tc.pointType, size: tc.size, knownMask: knownMask}
					def.symbols = make(map[uint64]string)
					for position := uint64(0); position < tc.width; position++ {
						if position != 6 {
							def.symbols[position] = fmt.Sprintf("FLAG_%02d", position)
						}
					}
					def.symbols[4] = ""
					raw := uint64(1) << bit
					words := []uint16{uint16(raw)}
					if tc.size == 2 {
						words = []uint16{uint16(raw >> 16), uint16(raw)}
					}
					// Membership comes from parity, independently of mask arithmetic.
					wantUnknown := raw
					var wantSymbols []string
					if bit%2 == uint64(parity) {
						wantUnknown = 0
						if bit != 4 && bit != 6 {
							wantSymbols = []string{fmt.Sprintf("FLAG_%02d", bit)}
						}
					}
					value := decodeSunSpecValue(def, words, nil)
					bits, unknown, ok := value.Bitfield()
					if value.State() != SunSpecValueValid || !ok || bits != raw || unknown != wantUnknown {
						t.Fatalf("state=%s bits=%#x unknown=%#x ok=%v; want bits=%#x unknown=%#x", value.State(), bits, unknown, ok, raw, wantUnknown)
					}
					if !reflect.DeepEqual(value.RawWords(), words) || !reflect.DeepEqual(value.BitfieldSymbols(), wantSymbols) {
						t.Fatalf("words=%v symbols=%v; want words=%v symbols=%v", value.RawWords(), value.BitfieldSymbols(), words, wantSymbols)
					}
				})
			}
		}
	}
}

func TestS26BitfieldSymbolOrderAndOwnership(t *testing.T) {
	for _, tc := range []struct {
		pointType   SunSpecPointType
		width       int
		size        uint16
		knownMask   uint64
		words       []uint16
		wantBits    uint64
		wantUnknown uint64
		wantSymbols []string
	}{
		{SunSpecTypeBitfield16, 16, 1, 0x5555, []uint16{0x9255}, 0x9255, 0x8200, []string{"FLAG_00", "FLAG_02", "FLAG_12"}},
		{SunSpecTypeBitfield32, 32, 2, 0x55555555, []uint16{0x9200, 0x9255}, 0x92009255, 0x82008200, []string{"FLAG_00", "FLAG_02", "FLAG_12", "FLAG_28"}},
	} {
		for _, order := range []string{"ascending", "descending", "odd_then_even"} {
			t.Run(fmt.Sprintf("%s/%s", tc.pointType, order), func(t *testing.T) {
				positions := make([]int, 0, tc.width)
				switch order {
				case "ascending":
					for bit := 0; bit < tc.width; bit++ {
						positions = append(positions, bit)
					}
				case "descending":
					for bit := tc.width - 1; bit >= 0; bit-- {
						positions = append(positions, bit)
					}
				case "odd_then_even":
					for _, parity := range []int{1, 0} {
						for bit := parity; bit < tc.width; bit += 2 {
							positions = append(positions, bit)
						}
					}
				}
				def := sunSpecPointDefinition{pointType: tc.pointType, size: tc.size, knownMask: tc.knownMask, symbols: make(map[uint64]string)}
				for _, bit := range positions {
					if bit != 6 {
						def.symbols[uint64(bit)] = fmt.Sprintf("FLAG_%02d", bit)
					}
				}
				def.symbols[4] = ""
				value := decodeSunSpecValue(def, tc.words, nil)
				bits, unknown, ok := value.Bitfield()
				if value.State() != SunSpecValueValid || !ok || bits != tc.wantBits || unknown != tc.wantUnknown {
					t.Fatalf("state=%s bits=%#x unknown=%#x ok=%v; want bits=%#x unknown=%#x", value.State(), bits, unknown, ok, tc.wantBits, tc.wantUnknown)
				}
				if !reflect.DeepEqual(value.RawWords(), tc.words) {
					t.Fatalf("raw words=%v; want %v", value.RawWords(), tc.words)
				}
				first, second := value.BitfieldSymbols(), value.BitfieldSymbols()
				if !reflect.DeepEqual(first, tc.wantSymbols) || !reflect.DeepEqual(second, tc.wantSymbols) {
					t.Fatalf("symbol reads=%v/%v; want %v", first, second, tc.wantSymbols)
				}
				for index := range first {
					first[index] = "CHANGED"
				}
				if !reflect.DeepEqual(second, tc.wantSymbols) || !reflect.DeepEqual(value.BitfieldSymbols(), tc.wantSymbols) {
					t.Fatalf("mutation affected another read: retained=%v fresh=%v; want %v", second, value.BitfieldSymbols(), tc.wantSymbols)
				}
			})
		}
	}
}
