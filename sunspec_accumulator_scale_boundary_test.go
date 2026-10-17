package modbusreg

import (
	"math"
	"reflect"
	"testing"
)

func TestU26Accumulator64SignedScaleBoundary(t *testing.T) {
	scale := decodeSunSpecValue(sunSpecPointDefinition{
		pointType: SunSpecTypeScaleFactor, size: 1,
	}, []uint16{0xffff}, nil)
	if signed, ok := scale.Signed(); scale.State() != SunSpecValueValid || !ok || signed != -1 {
		t.Fatalf("scale state=%s signed=%d present=%v", scale.State(), signed, ok)
	}

	for _, tc := range []struct {
		name        string
		words       []uint16
		coefficient uint64
		accState    SunSpecValueState
		uintState   SunSpecValueState
	}{
		{"MaxInt64", []uint16{0x7fff, 0xffff, 0xffff, 0xffff}, math.MaxInt64, SunSpecValueValid, SunSpecValueValid},
		{"MaxInt64PlusOne", []uint16{0x8000, 0, 0, 0}, uint64(math.MaxInt64) + 1, SunSpecValueInvalidEncoding, SunSpecValueValid},
		{"MaxUint64", []uint16{0xffff, 0xffff, 0xffff, 0xffff}, math.MaxUint64, SunSpecValueInvalidEncoding, SunSpecValueNotImplemented},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, point := range []struct {
				name      string
				pointType SunSpecPointType
				state     SunSpecValueState
			}{
				{"acc64", SunSpecTypeAccumulator64, tc.accState},
				{"uint64", SunSpecTypeUint64, tc.uintState},
			} {
				t.Run(point.name, func(t *testing.T) {
					value := decodeSunSpecValue(sunSpecPointDefinition{
						pointType: point.pointType, size: 4, scaleFactor: "U26_SF",
					}, tc.words, &scale)
					if value.State() != point.state || value.PointType() != point.pointType {
						t.Errorf("state/type=%s/%s want=%s/%s", value.State(), value.PointType(), point.state, point.pointType)
					}
					if got := value.RawWords(); !reflect.DeepEqual(got, tc.words) {
						t.Errorf("raw words=%v want=%v", got, tc.words)
					}
					wantUnsigned := point.state != SunSpecValueNotImplemented
					if got, ok := value.Unsigned(); ok != wantUnsigned || (ok && got != tc.coefficient) {
						t.Errorf("unsigned=%d present=%v want=%d present=%v", got, ok, tc.coefficient, wantUnsigned)
					}
					wantDecimal := point.pointType == SunSpecTypeAccumulator64 && tc.coefficient == math.MaxInt64
					if got, ok := value.Decimal(); ok != wantDecimal || (ok && got != (SunSpecDecimal{Coefficient: math.MaxInt64, Exponent: -1})) {
						t.Errorf("decimal=%+v present=%v want present=%v", got, ok, wantDecimal)
					}
					wantUnsignedDecimal := point.pointType == SunSpecTypeUint64 && point.state == SunSpecValueValid
					if got, ok := value.UnsignedDecimal(); ok != wantUnsignedDecimal || (ok && got != (SunSpecUnsignedDecimal{Coefficient: tc.coefficient, Exponent: -1})) {
						t.Errorf("unsigned decimal=%+v present=%v want coefficient=%d exponent=-1 present=%v", got, ok, tc.coefficient, wantUnsignedDecimal)
					}
				})
			}
		})
	}
}
