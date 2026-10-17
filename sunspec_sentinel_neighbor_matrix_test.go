package modbusreg

import (
	"reflect"
	"testing"
)

func TestQ29SunSpecSentinelNeighborMatrix(t *testing.T) {
	type vector struct {
		name     string
		words    []uint16
		state    SunSpecValueState
		signed   int64
		unsigned uint64
	}
	valid, absent, empty, invalid := SunSpecValueValid, SunSpecValueNotImplemented, SunSpecValueNotAccumulated, SunSpecValueInvalidEncoding
	for _, family := range []struct {
		pointType SunSpecPointType
		signed    bool
		vectors   []vector
	}{
		{SunSpecTypeInt16, true, []vector{
			{"below", []uint16{0x7fff}, valid, (1 << 15) - 1, 0},
			{"sentinel", []uint16{0x8000}, absent, 0, 0},
			{"above", []uint16{0x8001}, valid, -(1 << 15) + 1, 0},
			{"zero", []uint16{0}, valid, 0, 0},
		}},
		{SunSpecTypeInt32, true, []vector{
			{"below", []uint16{0x7fff, 0xffff}, valid, (1 << 31) - 1, 0},
			{"sentinel", []uint16{0x8000, 0}, absent, 0, 0},
			{"above", []uint16{0x8000, 1}, valid, -(1 << 31) + 1, 0},
			{"zero", []uint16{0, 0}, valid, 0, 0},
		}},
		{SunSpecTypeUint16, false, []vector{
			{"below", []uint16{0xfffe}, valid, 0, (1 << 16) - 2},
			{"sentinel", []uint16{0xffff}, absent, 0, 0},
			{"zero", []uint16{0}, valid, 0, 0},
		}},
		{SunSpecTypeCount, false, []vector{
			{"below", []uint16{0xfffe}, valid, 0, (1 << 16) - 2},
			{"sentinel", []uint16{0xffff}, absent, 0, 0},
			{"zero", []uint16{0}, valid, 0, 0},
		}},
		{SunSpecTypeUint32, false, []vector{
			{"below", []uint16{0xffff, 0xfffe}, valid, 0, (1 << 32) - 2},
			{"sentinel", []uint16{0xffff, 0xffff}, absent, 0, 0},
			{"zero", []uint16{0, 0}, valid, 0, 0},
		}},
		{SunSpecTypeAccumulator32, false, []vector{
			{"zero_sentinel", []uint16{0, 0}, empty, 0, 0},
			{"above", []uint16{0, 1}, valid, 0, 1},
			{"maximum", []uint16{0xffff, 0xffff}, valid, 0, (1 << 32) - 1},
		}},
		{SunSpecTypeAccumulator64, false, []vector{
			{"zero_sentinel", []uint16{0, 0, 0, 0}, empty, 0, 0},
			{"above", []uint16{0, 0, 0, 1}, valid, 0, 1},
			{"word_order", []uint16{1, 2, 3, 4}, valid, 0, (1 << 48) + 2*(1<<32) + 3*(1<<16) + 4},
		}},
		{SunSpecTypeScaleFactor, true, []vector{
			// Sentinel neighbors are invalid encodings under the bounded scale contract.
			{"below_sentinel", []uint16{0x7fff}, invalid, 0, 0},
			{"sentinel", []uint16{0x8000}, absent, 0, 0},
			{"above_sentinel", []uint16{0x8001}, invalid, 0, 0},
			{"below_minimum", []uint16{0xfff5}, invalid, 0, 0},
			{"minimum", []uint16{0xfff6}, valid, -10, 0},
			{"above_minimum", []uint16{0xfff7}, valid, -9, 0},
			{"zero", []uint16{0}, valid, 0, 0},
			{"below_maximum", []uint16{9}, valid, 9, 0},
			{"maximum", []uint16{10}, valid, 10, 0},
			{"above_maximum", []uint16{11}, invalid, 0, 0},
		}},
	} {
		t.Run(string(family.pointType), func(t *testing.T) {
			for _, tc := range family.vectors {
				t.Run(tc.name, func(t *testing.T) {
					for _, mode := range []string{"unscaled", "scale_negative", "scale_zero", "scale_positive", "scale_missing", "scale_sentinel"} {
						// A scale factor is itself unscaled, never linked to another factor.
						if family.pointType == SunSpecTypeScaleFactor && mode != "unscaled" {
							continue
						}
						t.Run(mode, func(t *testing.T) {
							def := sizedSunSpecPoint(family.pointType, uint16(len(tc.words)))
							state := tc.state
							var scale *SunSpecValue
							var exponent int16
							decimalAvailable := false
							if mode != "unscaled" {
								def.scaleFactor = "Synthetic_SF"
								var scaleWord uint16
								switch mode {
								case "scale_negative":
									scaleWord, exponent = 0xfffe, -2
								case "scale_positive":
									scaleWord, exponent = 2, 2
								case "scale_sentinel":
									scaleWord = 0x8000
								}
								if mode != "scale_missing" {
									sf := decodeSunSpecValue(sizedSunSpecPoint(SunSpecTypeScaleFactor, 1), []uint16{scaleWord}, nil)
									scale = &sf
								}
								if state == valid {
									switch mode {
									case "scale_missing":
										state = invalid
									case "scale_sentinel":
										state = absent
									default:
										decimalAvailable = true
									}
								}
							}
							got := decodeSunSpecValue(def, tc.words, scale)
							if got.PointType() != family.pointType || got.State() != state || !reflect.DeepEqual(got.RawWords(), tc.words) {
								t.Fatalf("type=%s state=%s raw=%v; want type=%s state=%s raw=%v", got.PointType(), got.State(), got.RawWords(), family.pointType, state, tc.words)
							}
							// Missing/unavailable scales retain the decoded native integer.
							nativeAvailable := tc.state == valid
							if n, ok := got.Signed(); ok != (nativeAvailable && family.signed) || (ok && n != tc.signed) {
								t.Fatalf("signed=%d/%v want=%d/%v", n, ok, tc.signed, nativeAvailable && family.signed)
							}
							if n, ok := got.Unsigned(); ok != (nativeAvailable && !family.signed) || (ok && n != tc.unsigned) {
								t.Fatalf("unsigned=%d/%v want=%d/%v", n, ok, tc.unsigned, nativeAvailable && !family.signed)
							}
							coefficient := tc.signed
							if !family.signed {
								coefficient = int64(tc.unsigned)
							}
							wantDecimal := SunSpecDecimal{Coefficient: coefficient, Exponent: exponent}
							if d, ok := got.Decimal(); ok != decimalAvailable || (ok && d != wantDecimal) {
								t.Fatalf("decimal=%+v/%v want=%+v/%v", d, ok, wantDecimal, decimalAvailable)
							}
							if d, ok := got.UnsignedDecimal(); ok {
								t.Fatalf("unexpected unsigned decimal=%+v", d)
							}
							if _, ok := got.Float32(); ok {
								t.Fatal("integer produced a floating-point accessor")
							}
							if _, ok := got.Text(); ok {
								t.Fatal("integer produced a text accessor")
							}
							if _, _, ok := got.Enum(); ok {
								t.Fatal("integer produced an enum accessor")
							}
							if _, _, ok := got.Bitfield(); ok {
								t.Fatal("integer produced a bitfield accessor")
							}
						})
					}
				})
			}
		})
	}
}
