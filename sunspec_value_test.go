package modbusreg

import (
	"math"
	"reflect"
	"testing"
)

func TestSunSpecValueSentinelsAndExactDecimal(t *testing.T) {
	tests := []struct {
		name  string
		def   sunSpecPointDefinition
		words []uint16
		sf    *SunSpecValue
		state SunSpecValueState
	}{
		{name: "int16 sentinel", def: sunSpecPointDefinition{pointType: SunSpecTypeInt16}, words: []uint16{0x8000}, state: SunSpecValueNotImplemented},
		{name: "uint16 sentinel", def: sunSpecPointDefinition{pointType: SunSpecTypeUint16}, words: []uint16{0xffff}, state: SunSpecValueNotImplemented},
		{name: "enum sentinel", def: sunSpecPointDefinition{pointType: SunSpecTypeEnum16}, words: []uint16{0xffff}, state: SunSpecValueNotImplemented},
		{name: "bitfield sentinel", def: sunSpecPointDefinition{pointType: SunSpecTypeBitfield32}, words: []uint16{0xffff, 0xffff}, state: SunSpecValueNotImplemented},
		{name: "accumulator zero", def: sunSpecPointDefinition{pointType: SunSpecTypeAccumulator32}, words: []uint16{0, 0}, state: SunSpecValueNotAccumulated},
		{name: "scale sentinel", def: sunSpecPointDefinition{pointType: SunSpecTypeScaleFactor}, words: []uint16{0x8000}, state: SunSpecValueNotImplemented},
		{name: "scale out of range", def: sunSpecPointDefinition{pointType: SunSpecTypeScaleFactor}, words: []uint16{11}, state: SunSpecValueInvalidEncoding},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value := decodeSunSpecValue(tc.def, tc.words, tc.sf)
			if value.State() != tc.state || !reflect.DeepEqual(value.RawWords(), tc.words) {
				t.Fatalf("value=%#v raw=%v", value, value.RawWords())
			}
		})
	}
	scale := decodeSunSpecValue(sunSpecPointDefinition{pointType: SunSpecTypeScaleFactor}, []uint16{0xfffe}, nil)
	value := decodeSunSpecValue(sunSpecPointDefinition{pointType: SunSpecTypeInt16, scaleFactor: "W_SF"}, []uint16{0xff85}, &scale)
	decimal, ok := value.Decimal()
	if !ok || decimal.Coefficient != -123 || decimal.Exponent != -2 {
		t.Fatalf("decimal=%#v ok=%v", decimal, ok)
	}
	copyWords := value.RawWords()
	copyWords[0] = 0
	if value.RawWords()[0] != 0xff85 {
		t.Fatal("raw words were mutable")
	}
}

func TestSunSpecValueFloatEnumBitfieldAndStringAreLossless(t *testing.T) {
	canonicalNaN := decodeSunSpecValue(sunSpecPointDefinition{pointType: SunSpecTypeFloat32}, []uint16{0x7fc0, 0}, nil)
	if canonicalNaN.State() != SunSpecValueNotImplemented {
		t.Fatalf("canonical NaN=%s", canonicalNaN.State())
	}
	for name, words := range map[string][]uint16{
		"noncanonical NaN":  {0x7fc0, 1},
		"positive infinity": {0x7f80, 0},
		"negative infinity": {0xff80, 0},
	} {
		t.Run(name, func(t *testing.T) {
			if got := decodeSunSpecValue(sunSpecPointDefinition{pointType: SunSpecTypeFloat32}, words, nil); got.State() != SunSpecValueInvalidEncoding {
				t.Fatalf("state=%s", got.State())
			}
		})
	}
	finite := decodeSunSpecValue(sunSpecPointDefinition{pointType: SunSpecTypeFloat32}, []uint16{0x4148, 0}, nil)
	if got, ok := finite.Float32(); !ok || got != 12.5 || math.Float32bits(got) != 0x41480000 {
		t.Fatalf("float=%v ok=%v", got, ok)
	}
	enum := decodeSunSpecValue(sunSpecPointDefinition{pointType: SunSpecTypeEnum16, symbols: map[uint64]string{1: "OFF"}}, []uint16{99}, nil)
	if number, symbol, ok := enum.Enum(); !ok || number != 99 || symbol != "" {
		t.Fatalf("enum=%d/%q/%v", number, symbol, ok)
	}
	bits := decodeSunSpecValue(sunSpecPointDefinition{pointType: SunSpecTypeBitfield32, knownMask: 0x0000ffff}, []uint16{0x0001, 0x0001}, nil)
	if value, unknown, ok := bits.Bitfield(); !ok || value != 0x00010001 || unknown != 0x00010000 {
		t.Fatalf("bitfield=%x unknown=%x ok=%v", value, unknown, ok)
	}
	text := decodeSunSpecValue(sunSpecPointDefinition{pointType: SunSpecTypeString}, []uint16{0x41c4, 0x8300}, nil)
	if got, ok := text.Text(); !ok || got != "Aă" {
		t.Fatalf("text=%q ok=%v state=%s", got, ok, text.State())
	}
	invalidText := decodeSunSpecValue(sunSpecPointDefinition{pointType: SunSpecTypeString}, []uint16{0xc300}, nil)
	if invalidText.State() != SunSpecValueInvalidEncoding {
		t.Fatalf("invalid UTF-8 state=%s", invalidText.State())
	}
}
