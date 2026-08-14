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
		{name: "int16 sentinel", def: sizedSunSpecPoint(SunSpecTypeInt16, 1), words: []uint16{0x8000}, state: SunSpecValueNotImplemented},
		{name: "uint16 sentinel", def: sizedSunSpecPoint(SunSpecTypeUint16, 1), words: []uint16{0xffff}, state: SunSpecValueNotImplemented},
		{name: "enum sentinel", def: sizedSunSpecPoint(SunSpecTypeEnum16, 1), words: []uint16{0xffff}, state: SunSpecValueNotImplemented},
		{name: "bitfield sentinel", def: sizedSunSpecPoint(SunSpecTypeBitfield32, 2), words: []uint16{0xffff, 0xffff}, state: SunSpecValueNotImplemented},
		{name: "accumulator zero", def: sizedSunSpecPoint(SunSpecTypeAccumulator32, 2), words: []uint16{0, 0}, state: SunSpecValueNotAccumulated},
		{name: "scale sentinel", def: sizedSunSpecPoint(SunSpecTypeScaleFactor, 1), words: []uint16{0x8000}, state: SunSpecValueNotImplemented},
		{name: "scale out of range", def: sizedSunSpecPoint(SunSpecTypeScaleFactor, 1), words: []uint16{11}, state: SunSpecValueInvalidEncoding},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value := decodeSunSpecValue(tc.def, tc.words, tc.sf)
			if value.State() != tc.state || !reflect.DeepEqual(value.RawWords(), tc.words) {
				t.Fatalf("value=%#v raw=%v", value, value.RawWords())
			}
		})
	}
	scale := decodeSunSpecValue(sizedSunSpecPoint(SunSpecTypeScaleFactor, 1), []uint16{0xfffe}, nil)
	definition := sizedSunSpecPoint(SunSpecTypeInt16, 1)
	definition.scaleFactor = "W_SF"
	value := decodeSunSpecValue(definition, []uint16{0xff85}, &scale)
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
	canonicalNaN := decodeSunSpecValue(sizedSunSpecPoint(SunSpecTypeFloat32, 2), []uint16{0x7fc0, 0}, nil)
	if canonicalNaN.State() != SunSpecValueNotImplemented {
		t.Fatalf("canonical NaN=%s", canonicalNaN.State())
	}
	for name, words := range map[string][]uint16{
		"noncanonical NaN":  {0x7fc0, 1},
		"positive infinity": {0x7f80, 0},
		"negative infinity": {0xff80, 0},
	} {
		t.Run(name, func(t *testing.T) {
			if got := decodeSunSpecValue(sizedSunSpecPoint(SunSpecTypeFloat32, 2), words, nil); got.State() != SunSpecValueInvalidEncoding {
				t.Fatalf("state=%s", got.State())
			}
		})
	}
	finite := decodeSunSpecValue(sizedSunSpecPoint(SunSpecTypeFloat32, 2), []uint16{0x4148, 0}, nil)
	if got, ok := finite.Float32(); !ok || got != 12.5 || math.Float32bits(got) != 0x41480000 {
		t.Fatalf("float=%v ok=%v", got, ok)
	}
	enumDefinition := sizedSunSpecPoint(SunSpecTypeEnum16, 1)
	enumDefinition.symbols = map[uint64]string{1: "OFF"}
	enum := decodeSunSpecValue(enumDefinition, []uint16{99}, nil)
	if number, symbol, ok := enum.Enum(); !ok || number != 99 || symbol != "" {
		t.Fatalf("enum=%d/%q/%v", number, symbol, ok)
	}
	bitfieldDefinition := sizedSunSpecPoint(SunSpecTypeBitfield32, 2)
	bitfieldDefinition.knownMask = 0x0000ffff
	bitfieldDefinition.symbols = map[uint64]string{0: "GROUND_FAULT", 16: "VENDOR_EXTENSION"}
	bits := decodeSunSpecValue(bitfieldDefinition, []uint16{0x0001, 0x0001}, nil)
	if value, unknown, ok := bits.Bitfield(); !ok || value != 0x00010001 || unknown != 0x00010000 {
		t.Fatalf("bitfield=%x unknown=%x ok=%v", value, unknown, ok)
	}
	if symbols := bits.BitfieldSymbols(); !reflect.DeepEqual(symbols, []string{"GROUND_FAULT"}) {
		t.Fatalf("bitfield symbols=%v", symbols)
	}
	symbols := bits.BitfieldSymbols()
	symbols[0] = "MUTATED"
	if bits.BitfieldSymbols()[0] != "GROUND_FAULT" {
		t.Fatal("bitfield symbols were mutable")
	}
	text := decodeSunSpecValue(sizedSunSpecPoint(SunSpecTypeString, 2), []uint16{0x41c4, 0x8300}, nil)
	if got, ok := text.Text(); !ok || got != "Aă" {
		t.Fatalf("text=%q ok=%v state=%s", got, ok, text.State())
	}
	invalidText := decodeSunSpecValue(sizedSunSpecPoint(SunSpecTypeString, 1), []uint16{0xc300}, nil)
	if invalidText.State() != SunSpecValueInvalidEncoding {
		t.Fatalf("invalid UTF-8 state=%s", invalidText.State())
	}
}

func TestSunSpecBitfield32DomainBoundary(t *testing.T) {
	definition := sizedSunSpecPoint(SunSpecTypeBitfield32, 2)
	valid := decodeSunSpecValue(definition, []uint16{0x7fff, 0xffff}, nil)
	if bits, unknown, ok := valid.Bitfield(); valid.State() != SunSpecValueValid || !ok || bits != 0x7fffffff || unknown != 0x7fffffff {
		t.Fatalf("valid boundary state=%s bits=%x unknown=%x ok=%v", valid.State(), bits, unknown, ok)
	}
	invalid := decodeSunSpecValue(definition, []uint16{0x8000, 0}, nil)
	if _, _, ok := invalid.Bitfield(); invalid.State() != SunSpecValueInvalidEncoding || ok || !reflect.DeepEqual(invalid.RawWords(), []uint16{0x8000, 0}) {
		t.Fatalf("invalid boundary=%#v", invalid)
	}
	sentinel := decodeSunSpecValue(definition, []uint16{0xffff, 0xffff}, nil)
	if sentinel.State() != SunSpecValueNotImplemented || !reflect.DeepEqual(sentinel.RawWords(), []uint16{0xffff, 0xffff}) {
		t.Fatalf("sentinel=%#v", sentinel)
	}
}

func TestSunSpecExtendedValueTypesRetainExactState(t *testing.T) {
	accumulator := decodeSunSpecValue(sizedSunSpecPoint(SunSpecTypeAccumulator64, 4), []uint16{0x0001, 0x0002, 0x0003, 0x0004}, nil)
	if value, ok := accumulator.Unsigned(); accumulator.State() != SunSpecValueValid || !ok || value != 0x0001000200030004 {
		t.Fatalf("acc64 state=%s value=%x ok=%v", accumulator.State(), value, ok)
	}
	notAccumulated := decodeSunSpecValue(sizedSunSpecPoint(SunSpecTypeAccumulator64, 4), []uint16{0, 0, 0, 0}, nil)
	if notAccumulated.State() != SunSpecValueNotAccumulated {
		t.Fatalf("zero acc64 state=%s", notAccumulated.State())
	}
	count := decodeSunSpecValue(sizedSunSpecPoint(SunSpecTypeCount, 1), []uint16{4}, nil)
	if value, ok := count.Unsigned(); count.State() != SunSpecValueValid || !ok || value != 4 {
		t.Fatalf("count state=%s value=%d ok=%v", count.State(), value, ok)
	}
	if got := decodeSunSpecValue(sizedSunSpecPoint(SunSpecTypeCount, 1), []uint16{0xffff}, nil); got.State() != SunSpecValueNotImplemented {
		t.Fatalf("count sentinel state=%s", got.State())
	}
	definition := sizedSunSpecPoint(SunSpecTypeBitfield16, 1)
	definition.knownMask = 0x000f
	valid := decodeSunSpecValue(definition, []uint16{0x4011}, nil)
	if bits, unknown, ok := valid.Bitfield(); valid.State() != SunSpecValueValid || !ok || bits != 0x4011 || unknown != 0x4010 {
		t.Fatalf("bitfield16 state=%s bits=%x unknown=%x ok=%v", valid.State(), bits, unknown, ok)
	}
	invalid := decodeSunSpecValue(definition, []uint16{0x8000}, nil)
	if invalid.State() != SunSpecValueInvalidEncoding || !reflect.DeepEqual(invalid.RawWords(), []uint16{0x8000}) {
		t.Fatalf("bitfield16 invalid boundary=%#v", invalid)
	}
	sentinel := decodeSunSpecValue(definition, []uint16{0xffff}, nil)
	if sentinel.State() != SunSpecValueNotImplemented {
		t.Fatalf("bitfield16 sentinel state=%s", sentinel.State())
	}
}

func sizedSunSpecPoint(pointType SunSpecPointType, size uint16) sunSpecPointDefinition {
	return sunSpecPointDefinition{pointType: pointType, size: size}
}
