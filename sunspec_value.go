package modbusreg

import (
	"encoding/binary"
	"math"
	"unicode/utf8"
)

type SunSpecPointType string

const (
	SunSpecTypeInt16         SunSpecPointType = "int16"
	SunSpecTypeUint16        SunSpecPointType = "uint16"
	SunSpecTypeUint32        SunSpecPointType = "uint32"
	SunSpecTypeAccumulator32 SunSpecPointType = "acc32"
	SunSpecTypeEnum16        SunSpecPointType = "enum16"
	SunSpecTypeBitfield32    SunSpecPointType = "bitfield32"
	SunSpecTypeString        SunSpecPointType = "string"
	SunSpecTypeFloat32       SunSpecPointType = "float32"
	SunSpecTypeScaleFactor   SunSpecPointType = "sunssf"
	SunSpecTypePad           SunSpecPointType = "pad"
)

type SunSpecValueState string

const (
	SunSpecValueValid           SunSpecValueState = "valid"
	SunSpecValueNotImplemented  SunSpecValueState = "not_implemented"
	SunSpecValueNotAccumulated  SunSpecValueState = "not_accumulated"
	SunSpecValueInvalidEncoding SunSpecValueState = "invalid_encoding"
)

type SunSpecDecimal struct {
	Coefficient int64
	Exponent    int16
}

type SunSpecValue struct {
	pointType   SunSpecPointType
	state       SunSpecValueState
	raw         []uint16
	decimal     SunSpecDecimal
	hasDecimal  bool
	float32     float32
	hasFloat    bool
	signed      int64
	hasSigned   bool
	unsigned    uint64
	hasUnsigned bool
	text        string
	hasText     bool
	enumNumber  uint64
	enumSymbol  string
	hasEnum     bool
	bits        uint64
	unknown     uint64
	hasBits     bool
}

func (v SunSpecValue) PointType() SunSpecPointType      { return v.pointType }
func (v SunSpecValue) State() SunSpecValueState         { return v.state }
func (v SunSpecValue) RawWords() []uint16               { return append([]uint16(nil), v.raw...) }
func (v SunSpecValue) Decimal() (SunSpecDecimal, bool)  { return v.decimal, v.hasDecimal }
func (v SunSpecValue) Float32() (float32, bool)         { return v.float32, v.hasFloat }
func (v SunSpecValue) Signed() (int64, bool)            { return v.signed, v.hasSigned }
func (v SunSpecValue) Unsigned() (uint64, bool)         { return v.unsigned, v.hasUnsigned }
func (v SunSpecValue) Text() (string, bool)             { return v.text, v.hasText }
func (v SunSpecValue) Enum() (uint64, string, bool)     { return v.enumNumber, v.enumSymbol, v.hasEnum }
func (v SunSpecValue) Bitfield() (uint64, uint64, bool) { return v.bits, v.unknown, v.hasBits }

func invalidSunSpecValue(pointType SunSpecPointType, words []uint16, state SunSpecValueState) SunSpecValue {
	return SunSpecValue{pointType: pointType, state: state, raw: append([]uint16(nil), words...)}
}

func decodeSunSpecValue(def sunSpecPointDefinition, words []uint16, scale *SunSpecValue) SunSpecValue {
	value := invalidSunSpecValue(def.pointType, words, SunSpecValueInvalidEncoding)
	if len(words) != int(def.size) {
		return value
	}
	switch def.pointType {
	case SunSpecTypeInt16:
		if words[0] == 0x8000 {
			return invalidSunSpecValue(def.pointType, words, SunSpecValueNotImplemented)
		}
		value.signed, value.hasSigned = int64(int16(words[0])), true
	case SunSpecTypeUint16:
		if words[0] == 0xffff {
			return invalidSunSpecValue(def.pointType, words, SunSpecValueNotImplemented)
		}
		value.unsigned, value.hasUnsigned = uint64(words[0]), true
	case SunSpecTypeUint32:
		raw := uint64(uint32(words[0])<<16 | uint32(words[1]))
		if raw == math.MaxUint32 {
			return invalidSunSpecValue(def.pointType, words, SunSpecValueNotImplemented)
		}
		value.unsigned, value.hasUnsigned = raw, true
	case SunSpecTypeAccumulator32:
		raw := uint64(uint32(words[0])<<16 | uint32(words[1]))
		if raw == 0 {
			return invalidSunSpecValue(def.pointType, words, SunSpecValueNotAccumulated)
		}
		value.unsigned, value.hasUnsigned = raw, true
	case SunSpecTypeScaleFactor:
		if words[0] == 0x8000 {
			return invalidSunSpecValue(def.pointType, words, SunSpecValueNotImplemented)
		}
		sf := int16(words[0])
		if sf < -10 || sf > 10 {
			return value
		}
		value.signed, value.hasSigned = int64(sf), true
	case SunSpecTypeFloat32:
		bits := uint32(words[0])<<16 | uint32(words[1])
		if bits == 0x7fc00000 {
			return invalidSunSpecValue(def.pointType, words, SunSpecValueNotImplemented)
		}
		decoded := math.Float32frombits(bits)
		if math.IsNaN(float64(decoded)) || math.IsInf(float64(decoded), 0) {
			return value
		}
		value.float32, value.hasFloat = decoded, true
	case SunSpecTypeEnum16:
		if words[0] == 0xffff {
			return invalidSunSpecValue(def.pointType, words, SunSpecValueNotImplemented)
		}
		value.enumNumber, value.enumSymbol, value.hasEnum = uint64(words[0]), def.symbols[uint64(words[0])], true
	case SunSpecTypeBitfield32:
		bits := uint64(uint32(words[0])<<16 | uint32(words[1]))
		if bits == math.MaxUint32 {
			return invalidSunSpecValue(def.pointType, words, SunSpecValueNotImplemented)
		}
		if bits > math.MaxInt32 {
			return value
		}
		value.bits, value.unknown, value.hasBits = bits, bits&^def.knownMask, true
	case SunSpecTypeString:
		return decodeSunSpecString(def, words)
	case SunSpecTypePad:
		if words[0] != 0x8000 {
			return value
		}
	default:
		return value
	}
	value.state = SunSpecValueValid
	if def.scaleFactor != "" {
		return applySunSpecScale(value, scale)
	}
	return value
}

func applySunSpecScale(value SunSpecValue, scale *SunSpecValue) SunSpecValue {
	if scale == nil || scale.state == SunSpecValueInvalidEncoding {
		value.state = SunSpecValueInvalidEncoding
		value.hasDecimal = false
		return value
	}
	if scale.state != SunSpecValueValid || !scale.hasSigned {
		value.state = SunSpecValueNotImplemented
		value.hasDecimal = false
		return value
	}
	coefficient := value.signed
	if !value.hasSigned {
		if !value.hasUnsigned || value.unsigned > math.MaxInt64 {
			value.state = SunSpecValueInvalidEncoding
			return value
		}
		coefficient = int64(value.unsigned)
	}
	value.decimal = SunSpecDecimal{Coefficient: coefficient, Exponent: int16(scale.signed)}
	value.hasDecimal = true
	return value
}

func decodeSunSpecString(def sunSpecPointDefinition, words []uint16) SunSpecValue {
	value := invalidSunSpecValue(def.pointType, words, SunSpecValueInvalidEncoding)
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
		return invalidSunSpecValue(def.pointType, words, SunSpecValueNotImplemented)
	}
	if len(raw) >= 2 && raw[0] == 0 && raw[1] == 0x80 {
		for _, b := range raw[2:] {
			if b != 0 {
				return value
			}
		}
		value.state, value.text, value.hasText = SunSpecValueValid, "", true
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
	value.state, value.text, value.hasText = SunSpecValueValid, string(raw[:end]), true
	return value
}
