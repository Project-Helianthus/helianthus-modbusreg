package sunspec

import (
	"encoding/binary"
	"math"
	"unicode/utf8"
)

// PointType is the SunSpec type of a catalog point. It fixes the word
// width, the not-implemented sentinel and the Value accessor that
// carries the reading. See
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/sunspec/read-only-core-v1.md.
type PointType string

// SunSpec point types. Integer types carry their bit width; acc32 and acc64
// are unsigned accumulators, sunssf is a scale-factor exponent and pad is a
// reserved word.
const (
	TypeInt16         PointType = "int16"
	TypeInt32         PointType = "int32"
	TypeUint16        PointType = "uint16"
	TypeUint32        PointType = "uint32"
	TypeUint64        PointType = "uint64"
	TypeAccumulator32 PointType = "acc32"
	TypeAccumulator64 PointType = "acc64"
	TypeCount         PointType = "count"
	TypeEnum16        PointType = "enum16"
	TypeBitfield16    PointType = "bitfield16"
	TypeBitfield32    PointType = "bitfield32"
	TypeString        PointType = "string"
	TypeFloat32       PointType = "float32"
	TypeScaleFactor   PointType = "sunssf"
	TypePad           PointType = "pad"
)

// ValueState classifies a decoded point. Every state keeps the raw
// words, so evidence survives a value that cannot be used.
type ValueState string

// Value states. NotImplemented: the point, or the scale factor it depends on,
// held the not-implemented sentinel of its type. NotAccumulated: an
// accumulator read zero. InvalidEncoding: the words could not be decoded, for
// example a wrong width, a scale factor outside -10 to 10, a NaN or infinite
// float, a malformed string, or a missing or invalid scale factor.
const (
	ValueValid           ValueState = "valid"
	ValueNotImplemented  ValueState = "not_implemented"
	ValueNotAccumulated  ValueState = "not_accumulated"
	ValueInvalidEncoding ValueState = "invalid_encoding"
)

// Decimal is an exact scaled reading, Coefficient * 10^Exponent.
// Exponent is the point's scale factor (-10 to 10), so no floating-point
// rounding occurs; the unit is the unit of the fact that holds it.
type Decimal struct {
	Coefficient int64
	Exponent    int16
}

// UnsignedDecimal preserves an unsigned coefficient without narrowing it
// through a signed or floating-point representation.
type UnsignedDecimal struct {
	Coefficient uint64
	Exponent    int16
}

// Value is one decoded SunSpec point: its type, state, raw words and
// the typed readings its type defines. Each accessor's bool reports whether
// that reading is present; absent readings are zero. A value is immutable and
// its slice accessors return copies.
type Value struct {
	pointType          PointType
	state              ValueState
	raw                []uint16
	decimal            Decimal
	hasDecimal         bool
	unsignedDecimal    UnsignedDecimal
	hasUnsignedDecimal bool
	float32            float32
	hasFloat           bool
	signed             int64
	hasSigned          bool
	unsigned           uint64
	hasUnsigned        bool
	text               string
	hasText            bool
	enumNumber         uint64
	enumSymbol         string
	hasEnum            bool
	bits               uint64
	unknown            uint64
	bitSymbols         []string
	hasBits            bool
}

// PointType returns the catalog point type the words were decoded as.
func (v Value) PointType() PointType { return v.pointType }

// State returns the decode state. Check it before using a typed reading: a
// scaled point whose scale factor is unusable keeps its unscaled integer
// reading but has no decimal reading.
func (v Value) State() ValueState { return v.state }

// RawWords returns a copy of the point's words exactly as read, in every
// state.
func (v Value) RawWords() []uint16 { return append([]uint16(nil), v.raw...) }

// Decimal returns the exact scaled reading of a point with a scale factor and
// true when the scale factor was valid. Scaled uint64 points use
// UnsignedDecimal instead.
func (v Value) Decimal() (Decimal, bool) { return v.decimal, v.hasDecimal }

// UnsignedDecimal returns the scaled reading of a scaled uint64 point, whose
// coefficient may exceed the int64 range, and true when present.
func (v Value) UnsignedDecimal() (UnsignedDecimal, bool) {
	return v.unsignedDecimal, v.hasUnsignedDecimal
}

// Float32 returns the reading of a float32 point and true. A NaN or infinite
// word pair never yields a reading.
func (v Value) Float32() (float32, bool) { return v.float32, v.hasFloat }

// Signed returns the unscaled two's-complement reading of an int16, int32 or
// sunssf point and true.
func (v Value) Signed() (int64, bool) { return v.signed, v.hasSigned }

// Unsigned returns the unscaled reading of a uint16, uint32, uint64, count or
// accumulator point and true.
func (v Value) Unsigned() (uint64, bool) { return v.unsigned, v.hasUnsigned }

// Text returns the UTF-8 text of a string point without its NUL padding and
// true. A string that is not valid UTF-8 or has bytes after its padding is
// invalid.
func (v Value) Text() (string, bool) { return v.text, v.hasText }

// Enum returns the raw enum16 number, its catalog symbol and true. The symbol
// is empty for a number the catalog does not name; the number is kept.
func (v Value) Enum() (uint64, string, bool) { return v.enumNumber, v.enumSymbol, v.hasEnum }

// Bitfield returns the raw bits, the set bits the catalog does not name, and
// true for a bitfield point.
func (v Value) Bitfield() (uint64, uint64, bool) { return v.bits, v.unknown, v.hasBits }

// BitfieldSymbols returns the catalog symbols of the set, named bits, from the
// least significant bit up, as a new slice.
func (v Value) BitfieldSymbols() []string {
	return append([]string(nil), v.bitSymbols...)
}

func invalidSunSpecValue(pointType PointType, words []uint16, state ValueState) Value {
	return Value{pointType: pointType, state: state, raw: append([]uint16(nil), words...)}
}

func decodeSunSpecValue(def sunSpecPointDefinition, words []uint16, scale *Value) Value {
	value := invalidSunSpecValue(def.pointType, words, ValueInvalidEncoding)
	if len(words) != int(def.size) {
		return value
	}
	switch def.pointType {
	case TypeInt16:
		if words[0] == 0x8000 {
			return invalidSunSpecValue(def.pointType, words, ValueNotImplemented)
		}
		value.signed, value.hasSigned = int64(int16(words[0])), true
	case TypeInt32:
		raw := uint32(words[0])<<16 | uint32(words[1])
		if raw == 0x80000000 {
			return invalidSunSpecValue(def.pointType, words, ValueNotImplemented)
		}
		value.signed, value.hasSigned = int64(int32(raw)), true
	case TypeUint16, TypeCount:
		if words[0] == 0xffff {
			return invalidSunSpecValue(def.pointType, words, ValueNotImplemented)
		}
		value.unsigned, value.hasUnsigned = uint64(words[0]), true
	case TypeUint32:
		raw := uint64(uint32(words[0])<<16 | uint32(words[1]))
		if raw == math.MaxUint32 {
			return invalidSunSpecValue(def.pointType, words, ValueNotImplemented)
		}
		value.unsigned, value.hasUnsigned = raw, true
	case TypeUint64:
		raw := uint64(words[0])<<48 | uint64(words[1])<<32 | uint64(words[2])<<16 | uint64(words[3])
		if raw == math.MaxUint64 {
			return invalidSunSpecValue(def.pointType, words, ValueNotImplemented)
		}
		value.unsigned, value.hasUnsigned = raw, true
	case TypeAccumulator32:
		raw := uint64(uint32(words[0])<<16 | uint32(words[1]))
		if raw == 0 {
			return invalidSunSpecValue(def.pointType, words, ValueNotAccumulated)
		}
		value.unsigned, value.hasUnsigned = raw, true
	case TypeAccumulator64:
		raw := uint64(words[0])<<48 | uint64(words[1])<<32 | uint64(words[2])<<16 | uint64(words[3])
		if raw == 0 {
			return invalidSunSpecValue(def.pointType, words, ValueNotAccumulated)
		}
		value.unsigned, value.hasUnsigned = raw, true
	case TypeScaleFactor:
		if words[0] == 0x8000 {
			return invalidSunSpecValue(def.pointType, words, ValueNotImplemented)
		}
		sf := int16(words[0])
		if sf < -10 || sf > 10 {
			return value
		}
		value.signed, value.hasSigned = int64(sf), true
	case TypeFloat32:
		bits := uint32(words[0])<<16 | uint32(words[1])
		if bits == 0x7fc00000 {
			return invalidSunSpecValue(def.pointType, words, ValueNotImplemented)
		}
		decoded := math.Float32frombits(bits)
		if math.IsNaN(float64(decoded)) || math.IsInf(float64(decoded), 0) {
			return value
		}
		value.float32, value.hasFloat = decoded, true
	case TypeEnum16:
		if words[0] == 0xffff {
			return invalidSunSpecValue(def.pointType, words, ValueNotImplemented)
		}
		value.enumNumber, value.enumSymbol, value.hasEnum = uint64(words[0]), def.symbols[uint64(words[0])], true
	case TypeBitfield16:
		bits := uint64(words[0])
		if bits == math.MaxUint16 {
			return invalidSunSpecValue(def.pointType, words, ValueNotImplemented)
		}
		setSunSpecBitfield(&value, bits, def, 15)
	case TypeBitfield32:
		bits := uint64(uint32(words[0])<<16 | uint32(words[1]))
		if bits == math.MaxUint32 {
			return invalidSunSpecValue(def.pointType, words, ValueNotImplemented)
		}
		setSunSpecBitfield(&value, bits, def, 31)
	case TypeString:
		return decodeSunSpecString(def, words)
	case TypePad:
		if words[0] != 0x8000 {
			return value
		}
	default:
		return value
	}
	value.state = ValueValid
	if def.scaleFactor != "" || def.fixedScale != nil {
		return applySunSpecScale(value, scale)
	}
	return value
}

func setSunSpecBitfield(value *Value, bits uint64, def sunSpecPointDefinition, width uint64) {
	value.bits, value.unknown, value.hasBits = bits, bits&^def.knownMask, true
	for bit := uint64(0); bit <= width; bit++ {
		mask := uint64(1) << bit
		if bits&mask != 0 && def.knownMask&mask != 0 && def.symbols[bit] != "" {
			value.bitSymbols = append(value.bitSymbols, def.symbols[bit])
		}
	}
}

func applySunSpecScale(value Value, scale *Value) Value {
	if scale == nil || scale.state == ValueInvalidEncoding {
		value.state = ValueInvalidEncoding
		value.hasDecimal = false
		return value
	}
	if scale.state != ValueValid || !scale.hasSigned {
		value.state = ValueNotImplemented
		value.hasDecimal = false
		return value
	}
	coefficient := value.signed
	if !value.hasSigned {
		if value.pointType == TypeUint64 && value.hasUnsigned {
			value.unsignedDecimal = UnsignedDecimal{Coefficient: value.unsigned, Exponent: int16(scale.signed)}
			value.hasUnsignedDecimal = true
			return value
		}
		if !value.hasUnsigned || value.unsigned > math.MaxInt64 {
			value.state = ValueInvalidEncoding
			return value
		}
		coefficient = int64(value.unsigned)
	}
	value.decimal = Decimal{Coefficient: coefficient, Exponent: int16(scale.signed)}
	value.hasDecimal = true
	return value
}

func decodeSunSpecString(def sunSpecPointDefinition, words []uint16) Value {
	value := invalidSunSpecValue(def.pointType, words, ValueInvalidEncoding)
	raw := make([]byte, len(words)*2)
	for index, word := range words {
		binary.BigEndian.PutUint16(raw[index*2:], word)
	}
	allZero := true
	for _, b := range raw {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return invalidSunSpecValue(def.pointType, words, ValueNotImplemented)
	}
	if len(raw) >= 2 && raw[0] == 0 && raw[1] == 0x80 {
		for _, b := range raw[2:] {
			if b != 0 {
				return value
			}
		}
		value.state, value.text, value.hasText = ValueValid, "", true
		return value
	}
	end := len(raw)
	for index, b := range raw {
		if b == 0 {
			end = index
			break
		}
	}
	for _, b := range raw[end:] {
		if b != 0 {
			return value
		}
	}
	if !utf8.Valid(raw[:end]) {
		return value
	}
	value.state, value.text, value.hasText = ValueValid, string(raw[:end]), true
	return value
}
