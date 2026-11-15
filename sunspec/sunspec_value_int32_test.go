package sunspec

import "testing"

func TestSunSpecInt32UsesNetworkWordOrderAndExactSentinel(t *testing.T) {
	definition := sizedSunSpecPoint(TypeInt32, 2)
	for _, tc := range []struct {
		name  string
		words []uint16
		state ValueState
		value int64
	}{
		{"positive", []uint16{0x0001, 0x0002}, ValueValid, 65538},
		{"negative", []uint16{0xffff, 0xfffe}, ValueValid, -2},
		{"sentinel", []uint16{0x8000, 0x0000}, ValueNotImplemented, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoded := decodeSunSpecValue(definition, tc.words, nil)
			if decoded.State() != tc.state {
				t.Fatalf("state=%s want=%s", decoded.State(), tc.state)
			}
			if tc.state == ValueValid {
				value, ok := decoded.Signed()
				if !ok || value != tc.value {
					t.Fatalf("value=%d ok=%v", value, ok)
				}
			}
		})
	}
}
