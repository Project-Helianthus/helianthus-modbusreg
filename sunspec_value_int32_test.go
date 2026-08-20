package modbusreg

import "testing"

func TestSunSpecInt32UsesNetworkWordOrderAndExactSentinel(t *testing.T) {
	definition := sizedSunSpecPoint(SunSpecTypeInt32, 2)
	for _, tc := range []struct {
		name  string
		words []uint16
		state SunSpecValueState
		value int64
	}{
		{"positive", []uint16{0x0001, 0x0002}, SunSpecValueValid, 65538},
		{"negative", []uint16{0xffff, 0xfffe}, SunSpecValueValid, -2},
		{"sentinel", []uint16{0x8000, 0x0000}, SunSpecValueNotImplemented, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoded := decodeSunSpecValue(definition, tc.words, nil)
			if decoded.State() != tc.state {
				t.Fatalf("state=%s want=%s", decoded.State(), tc.state)
			}
			if tc.state == SunSpecValueValid {
				value, ok := decoded.Signed()
				if !ok || value != tc.value {
					t.Fatalf("value=%d ok=%v", value, ok)
				}
			}
		})
	}
}
