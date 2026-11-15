package sunspec

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
		sf    *Value
		state ValueState
	}{
		{name: "int16 sentinel", def: sizedSunSpecPoint(TypeInt16, 1), words: []uint16{0x8000}, state: ValueNotImplemented},
		{name: "uint16 sentinel", def: sizedSunSpecPoint(TypeUint16, 1), words: []uint16{0xffff}, state: ValueNotImplemented},
		{name: "enum sentinel", def: sizedSunSpecPoint(TypeEnum16, 1), words: []uint16{0xffff}, state: ValueNotImplemented},
		{name: "bitfield sentinel", def: sizedSunSpecPoint(TypeBitfield32, 2), words: []uint16{0xffff, 0xffff}, state: ValueNotImplemented},
		{name: "accumulator zero", def: sizedSunSpecPoint(TypeAccumulator32, 2), words: []uint16{0, 0}, state: ValueNotAccumulated},
		{name: "scale sentinel", def: sizedSunSpecPoint(TypeScaleFactor, 1), words: []uint16{0x8000}, state: ValueNotImplemented},
		{name: "scale out of range", def: sizedSunSpecPoint(TypeScaleFactor, 1), words: []uint16{11}, state: ValueInvalidEncoding},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value := decodeSunSpecValue(tc.def, tc.words, tc.sf)
			if value.State() != tc.state || !reflect.DeepEqual(value.RawWords(), tc.words) {
				t.Fatalf("value=%#v raw=%v", value, value.RawWords())
			}
		})
	}
	scale := decodeSunSpecValue(sizedSunSpecPoint(TypeScaleFactor, 1), []uint16{0xfffe}, nil)
	definition := sizedSunSpecPoint(TypeInt16, 1)
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
	canonicalNaN := decodeSunSpecValue(sizedSunSpecPoint(TypeFloat32, 2), []uint16{0x7fc0, 0}, nil)
	if canonicalNaN.State() != ValueNotImplemented {
		t.Fatalf("canonical NaN=%s", canonicalNaN.State())
	}
	for name, words := range map[string][]uint16{
		"noncanonical NaN":  {0x7fc0, 1},
		"positive infinity": {0x7f80, 0},
		"negative infinity": {0xff80, 0},
	} {
		t.Run(name, func(t *testing.T) {
			if got := decodeSunSpecValue(sizedSunSpecPoint(TypeFloat32, 2), words, nil); got.State() != ValueInvalidEncoding {
				t.Fatalf("state=%s", got.State())
			}
		})
	}
	finite := decodeSunSpecValue(sizedSunSpecPoint(TypeFloat32, 2), []uint16{0x4148, 0}, nil)
	if got, ok := finite.Float32(); !ok || got != 12.5 || math.Float32bits(got) != 0x41480000 {
		t.Fatalf("float=%v ok=%v", got, ok)
	}
	enumDefinition := sizedSunSpecPoint(TypeEnum16, 1)
	enumDefinition.symbols = map[uint64]string{1: "OFF"}
	enum := decodeSunSpecValue(enumDefinition, []uint16{99}, nil)
	if number, symbol, ok := enum.Enum(); !ok || number != 99 || symbol != "" {
		t.Fatalf("enum=%d/%q/%v", number, symbol, ok)
	}
	bitfieldDefinition := sizedSunSpecPoint(TypeBitfield32, 2)
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
	text := decodeSunSpecValue(sizedSunSpecPoint(TypeString, 2), []uint16{0x41c4, 0x8300}, nil)
	if got, ok := text.Text(); !ok || got != "Aă" {
		t.Fatalf("text=%q ok=%v state=%s", got, ok, text.State())
	}
	invalidText := decodeSunSpecValue(sizedSunSpecPoint(TypeString, 1), []uint16{0xc300}, nil)
	if invalidText.State() != ValueInvalidEncoding {
		t.Fatalf("invalid UTF-8 state=%s", invalidText.State())
	}
}

func TestSunSpecBitfield32DomainBoundary(t *testing.T) {
	definition := sizedSunSpecPoint(TypeBitfield32, 2)
	definition.knownMask = uint64(1) << 31
	definition.symbols = map[uint64]string{31: "HIGH_FLAG"}
	valid := decodeSunSpecValue(definition, []uint16{0x7fff, 0xffff}, nil)
	if bits, unknown, ok := valid.Bitfield(); valid.State() != ValueValid || !ok || bits != 0x7fffffff || unknown != 0x7fffffff {
		t.Fatalf("valid boundary state=%s bits=%x unknown=%x ok=%v", valid.State(), bits, unknown, ok)
	}
	high := decodeSunSpecValue(definition, []uint16{0x8000, 0}, nil)
	if bits, unknown, ok := high.Bitfield(); high.State() != ValueValid || !ok || bits != 0x80000000 || unknown != 0 || !reflect.DeepEqual(high.BitfieldSymbols(), []string{"HIGH_FLAG"}) {
		t.Fatalf("high boundary state=%s bits=%x unknown=%x symbols=%v ok=%v", high.State(), bits, unknown, high.BitfieldSymbols(), ok)
	}
	sentinel := decodeSunSpecValue(definition, []uint16{0xffff, 0xffff}, nil)
	if sentinel.State() != ValueNotImplemented || !reflect.DeepEqual(sentinel.RawWords(), []uint16{0xffff, 0xffff}) {
		t.Fatalf("sentinel=%#v", sentinel)
	}
}

func TestSunSpecExtendedValueTypesRetainExactState(t *testing.T) {
	accumulator := decodeSunSpecValue(sizedSunSpecPoint(TypeAccumulator64, 4), []uint16{0x0001, 0x0002, 0x0003, 0x0004}, nil)
	if value, ok := accumulator.Unsigned(); accumulator.State() != ValueValid || !ok || value != 0x0001000200030004 {
		t.Fatalf("acc64 state=%s value=%x ok=%v", accumulator.State(), value, ok)
	}
	notAccumulated := decodeSunSpecValue(sizedSunSpecPoint(TypeAccumulator64, 4), []uint16{0, 0, 0, 0}, nil)
	if notAccumulated.State() != ValueNotAccumulated {
		t.Fatalf("zero acc64 state=%s", notAccumulated.State())
	}
	count := decodeSunSpecValue(sizedSunSpecPoint(TypeCount, 1), []uint16{4}, nil)
	if value, ok := count.Unsigned(); count.State() != ValueValid || !ok || value != 4 {
		t.Fatalf("count state=%s value=%d ok=%v", count.State(), value, ok)
	}
	if got := decodeSunSpecValue(sizedSunSpecPoint(TypeCount, 1), []uint16{0xffff}, nil); got.State() != ValueNotImplemented {
		t.Fatalf("count sentinel state=%s", got.State())
	}
	definition := sizedSunSpecPoint(TypeBitfield16, 1)
	definition.knownMask = 0x800f
	definition.symbols = map[uint64]string{15: "HIGH_FLAG"}
	valid := decodeSunSpecValue(definition, []uint16{0x4011}, nil)
	if bits, unknown, ok := valid.Bitfield(); valid.State() != ValueValid || !ok || bits != 0x4011 || unknown != 0x4010 {
		t.Fatalf("bitfield16 state=%s bits=%x unknown=%x ok=%v", valid.State(), bits, unknown, ok)
	}
	high := decodeSunSpecValue(definition, []uint16{0x8000}, nil)
	if bits, unknown, ok := high.Bitfield(); high.State() != ValueValid || !ok || bits != 0x8000 || unknown != 0 || !reflect.DeepEqual(high.BitfieldSymbols(), []string{"HIGH_FLAG"}) {
		t.Fatalf("bitfield16 high boundary state=%s bits=%x unknown=%x symbols=%v ok=%v", high.State(), bits, unknown, high.BitfieldSymbols(), ok)
	}
	sentinel := decodeSunSpecValue(definition, []uint16{0xffff}, nil)
	if sentinel.State() != ValueNotImplemented {
		t.Fatalf("bitfield16 sentinel state=%s", sentinel.State())
	}
}

func TestSunSpecUint64PreservesExactUnsignedScaling(t *testing.T) {
	scale := decodeSunSpecValue(sizedSunSpecPoint(TypeScaleFactor, 1), []uint16{0xfffe}, nil)
	definition := sizedSunSpecPoint(TypeUint64, 4)
	definition.scaleFactor = "TotWh_SF"

	value := decodeSunSpecValue(definition, []uint16{0x8000, 0, 0, 0}, &scale)
	if value.State() != ValueValid {
		t.Fatalf("state=%s", value.State())
	}
	if raw, ok := value.Unsigned(); !ok || raw != math.MaxInt64+1 {
		t.Fatalf("unsigned=%d ok=%v", raw, ok)
	}
	if _, ok := value.Decimal(); ok {
		t.Fatal("uint64 scaling was narrowed through signed decimal")
	}
	decimal, ok := value.UnsignedDecimal()
	if !ok || decimal != (UnsignedDecimal{Coefficient: math.MaxInt64 + 1, Exponent: -2}) {
		t.Fatalf("unsigned decimal=%#v ok=%v", decimal, ok)
	}

	zero := decodeSunSpecValue(definition, []uint16{0, 0, 0, 0}, &scale)
	if raw, ok := zero.Unsigned(); zero.State() != ValueValid || !ok || raw != 0 {
		t.Fatalf("zero=%#v raw=%d ok=%v", zero, raw, ok)
	}
	if decimal, ok := zero.UnsignedDecimal(); !ok || decimal != (UnsignedDecimal{Coefficient: 0, Exponent: -2}) {
		t.Fatalf("zero decimal=%#v ok=%v", decimal, ok)
	}

	allOnes := decodeSunSpecValue(definition, []uint16{0xffff, 0xffff, 0xffff, 0xffff}, &scale)
	if allOnes.State() != ValueNotImplemented {
		t.Fatalf("all-ones state=%s", allOnes.State())
	}
	if _, ok := allOnes.UnsignedDecimal(); ok {
		t.Fatal("all-ones uint64 produced a scaled value")
	}

	for name, invalidScale := range map[string]*Value{
		"missing":     nil,
		"unavailable": &Value{pointType: TypeScaleFactor, state: ValueNotImplemented},
		"invalid":     &Value{pointType: TypeScaleFactor, state: ValueInvalidEncoding},
	} {
		t.Run(name, func(t *testing.T) {
			got := decodeSunSpecValue(definition, []uint16{0, 0, 0, 1}, invalidScale)
			if _, ok := got.UnsignedDecimal(); ok {
				t.Fatalf("invalid scale produced unsigned decimal: %#v", got)
			}
		})
	}

	accumulator := decodeSunSpecValue(sizedSunSpecPoint(TypeAccumulator64, 4), []uint16{0, 0, 0, 0}, nil)
	if accumulator.State() != ValueNotAccumulated {
		t.Fatalf("acc64 zero state=%s", accumulator.State())
	}
}

func sizedSunSpecPoint(pointType PointType, size uint16) sunSpecPointDefinition {
	return sunSpecPointDefinition{pointType: pointType, size: size}
}
