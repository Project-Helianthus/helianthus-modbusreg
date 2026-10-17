package modbusreg

import (
	"reflect"
	"testing"
)

func TestU13ExtremeScaleResiduals(t *testing.T) {
	for _, coefficient := range []struct {
		name      string
		pointType SunSpecPointType
		words     []uint16
	}{
		{name: "int16", pointType: SunSpecTypeInt16, words: []uint16{123}},
		{name: "uint64", pointType: SunSpecTypeUint64, words: []uint16{0x8000, 0, 0, 1}},
	} {
		t.Run(coefficient.name, func(t *testing.T) {
			definition := sunSpecPointDefinition{
				pointType: coefficient.pointType, size: uint16(len(coefficient.words)), scaleFactor: "U13_SF",
			}
			for _, tc := range []struct {
				name  string
				word  uint16
				state SunSpecValueState
			}{
				{name: "positive_extreme", word: 0x7fff, state: SunSpecValueInvalidEncoding},
				{name: "negative_extreme", word: 0x8001, state: SunSpecValueInvalidEncoding},
				{name: "exact_sentinel", word: 0x8000, state: SunSpecValueNotImplemented},
				{name: "zero_scale", word: 0, state: SunSpecValueValid},
			} {
				t.Run(tc.name, func(t *testing.T) {
					scaleWords := []uint16{tc.word}
					scale := decodeSunSpecValue(sunSpecPointDefinition{pointType: SunSpecTypeScaleFactor, size: 1}, scaleWords, nil)
					value := decodeSunSpecValue(definition, coefficient.words, &scale)
					if value.State() != tc.state {
						t.Errorf("scaled state=%s, want %s", value.State(), tc.state)
					}
					if scale.State() != tc.state {
						t.Errorf("scale state=%s, want %s", scale.State(), tc.state)
					}
					if !reflect.DeepEqual(value.RawWords(), coefficient.words) || !reflect.DeepEqual(scale.RawWords(), scaleWords) {
						t.Errorf("raw coefficient=%v scale=%v, want %v and %v", value.RawWords(), scale.RawWords(), coefficient.words, scaleWords)
					}
					decimal, hasDecimal := value.Decimal()
					unsignedDecimal, hasUnsignedDecimal := value.UnsignedDecimal()
					if tc.state != SunSpecValueValid {
						if hasDecimal || hasUnsignedDecimal {
							t.Errorf("unusable scale exposed decimal: signed=%+v/%v unsigned=%+v/%v", decimal, hasDecimal, unsignedDecimal, hasUnsignedDecimal)
						}
						return
					}
					if coefficient.pointType == SunSpecTypeInt16 {
						if !hasDecimal || decimal != (SunSpecDecimal{Coefficient: 123, Exponent: 0}) || hasUnsignedDecimal {
							t.Errorf("zero scale: signed=%+v/%v unsigned=%+v/%v", decimal, hasDecimal, unsignedDecimal, hasUnsignedDecimal)
						}
					} else if !hasUnsignedDecimal || unsignedDecimal != (SunSpecUnsignedDecimal{Coefficient: 9223372036854775809, Exponent: 0}) || hasDecimal {
						t.Errorf("zero scale: signed=%+v/%v unsigned=%+v/%v", decimal, hasDecimal, unsignedDecimal, hasUnsignedDecimal)
					}
				})
			}
		})
	}
}
