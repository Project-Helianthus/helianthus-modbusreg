package sunspec

import (
	"reflect"
	"testing"
)

func TestS27ScaleBoundaryDependencyMatrix(t *testing.T) {
	coefficients := []struct {
		name     string
		point    PointType
		words    []uint16
		signed   int64
		unsigned uint64
	}{
		{"int16", TypeInt16, []uint16{0xff85}, -123, 0},
		{"int32", TypeInt32, []uint16{0xfffe, 0x1dc0}, -123456, 0},
		{"uint16", TypeUint16, []uint16{123}, 0, 123},
		{"uint32", TypeUint32, []uint16{0x0001, 0xe240}, 0, 123456},
		{"uint64", TypeUint64, []uint16{0x9000, 0x0001, 0x0002, 0x0003}, 0, 10376293545756721155},
	}
	dependencies := []struct {
		name     string
		words    []uint16
		missing  bool
		state    ValueState
		exponent int16
	}{
		{"minus11", []uint16{0xfff5}, false, ValueInvalidEncoding, 0},
		{"minus10", []uint16{0xfff6}, false, ValueValid, -10},
		{"minus1", []uint16{0xffff}, false, ValueValid, -1},
		{"zero", []uint16{0}, false, ValueValid, 0},
		{"plus1", []uint16{1}, false, ValueValid, 1},
		{"plus10", []uint16{10}, false, ValueValid, 10},
		{"plus11", []uint16{11}, false, ValueInvalidEncoding, 0},
		{"missing", nil, true, ValueInvalidEncoding, 0},
		{"invalid_width", []uint16{0, 1}, false, ValueInvalidEncoding, 0},
		{"not_implemented", []uint16{0x8000}, false, ValueNotImplemented, 0},
		{"max_int16", []uint16{0x7fff}, false, ValueInvalidEncoding, 0},
		{"min_plus_one", []uint16{0x8001}, false, ValueInvalidEncoding, 0},
	}
	for _, coefficient := range coefficients {
		for _, dependency := range dependencies {
			t.Run(coefficient.name+"/"+dependency.name, func(t *testing.T) {
				var scale *Value
				if !dependency.missing {
					decoded := decodeSunSpecValue(sunSpecPointDefinition{pointType: TypeScaleFactor, size: 1}, dependency.words, nil)
					scale = &decoded
					if scale.State() != dependency.state || scale.PointType() != TypeScaleFactor || !reflect.DeepEqual(scale.RawWords(), dependency.words) {
						t.Errorf("scale state=%s type=%s raw=%v; want state=%s raw=%v", scale.State(), scale.PointType(), scale.RawWords(), dependency.state, dependency.words)
					}
					got, ok := scale.Signed()
					if wantOK := dependency.state == ValueValid; ok != wantOK || (wantOK && got != int64(dependency.exponent)) {
						t.Errorf("scale signed=%d available=%v; want exponent=%d available=%v", got, ok, dependency.exponent, wantOK)
					}
				}
				definition := sunSpecPointDefinition{pointType: coefficient.point, size: uint16(len(coefficient.words)), scaleFactor: "SF"}
				value := decodeSunSpecValue(definition, coefficient.words, scale)
				if value.State() != dependency.state || value.PointType() != coefficient.point || !reflect.DeepEqual(value.RawWords(), coefficient.words) {
					t.Errorf("value state=%s type=%s raw=%v; want state=%s type=%s raw=%v", value.State(), value.PointType(), value.RawWords(), dependency.state, coefficient.point, coefficient.words)
				}
				wantSigned := coefficient.point == TypeInt16 || coefficient.point == TypeInt32
				if got, ok := value.Signed(); ok != wantSigned || got != coefficient.signed {
					t.Errorf("signed=%d available=%v; want %d available=%v", got, ok, coefficient.signed, wantSigned)
				}
				if got, ok := value.Unsigned(); ok == wantSigned || got != coefficient.unsigned {
					t.Errorf("unsigned=%d available=%v; want %d available=%v", got, ok, coefficient.unsigned, !wantSigned)
				}
				validScale := dependency.state == ValueValid
				wideUnsigned := coefficient.point == TypeUint64
				wantDecimal := Decimal{}
				if validScale && !wideUnsigned {
					wantCoefficient := coefficient.signed
					if !wantSigned {
						wantCoefficient = int64(coefficient.unsigned)
					}
					wantDecimal = Decimal{Coefficient: wantCoefficient, Exponent: dependency.exponent}
				}
				if got, ok := value.Decimal(); ok != (validScale && !wideUnsigned) || got != wantDecimal {
					t.Errorf("decimal=%+v available=%v; want %+v available=%v", got, ok, wantDecimal, validScale && !wideUnsigned)
				}
				wantUnsignedDecimal := UnsignedDecimal{}
				if validScale && wideUnsigned {
					wantUnsignedDecimal = UnsignedDecimal{Coefficient: coefficient.unsigned, Exponent: dependency.exponent}
				}
				if got, ok := value.UnsignedDecimal(); ok != (validScale && wideUnsigned) || got != wantUnsignedDecimal {
					t.Errorf("unsigned decimal=%+v available=%v; want %+v available=%v", got, ok, wantUnsignedDecimal, validScale && wideUnsigned)
				}
				if got, ok := value.Float32(); ok || got != 0 {
					t.Errorf("integer scaling published float=%v available=%v", got, ok)
				}
			})
		}
	}
}
