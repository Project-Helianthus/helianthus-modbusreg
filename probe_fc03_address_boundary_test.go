package modbusreg_test

import (
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestProbeFc03OverflowAddressBoundary(t *testing.T) {
	limits := reg.DefaultDetectionLimits()
	for _, tc := range []struct {
		name     string
		function reg.ProbeFunction
		address  uint16
		width    uint16
		overflow bool
	}{
		{"fc03_final_register", reg.ProbeReadHoldingRegisters, 65535, 1, false},
		{"fc03_overflow", reg.ProbeReadHoldingRegisters, 65535, 2, true},
		{"fc03_two_words_ending_at_final_register", reg.ProbeReadHoldingRegisters, 65534, 2, false},
		{"fc04_final_register", reg.ProbeReadInputRegisters, 65535, 1, false},
		{"fc04_overflow", reg.ProbeReadInputRegisters, 65535, 2, true},
		{"fc04_two_words_ending_at_final_register", reg.ProbeReadInputRegisters, 65534, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := reg.NewProbePlan(reg.ProbePlanSpec{
				Version: reg.MustParseVersion("1.0.0"),
				Declarations: []reg.ProbeDeclarationSpec{{
					ID:            "synthetic-manufacturer",
					Function:      tc.function,
					Address:       tc.address,
					WordCount:     tc.width,
					IdentityField: reg.ProbeIdentityManufacturer,
					Encoding:      reg.ProbeIdentityASCII,
				}},
			}, limits)
			if tc.overflow {
				if err == nil || err.Error() != "probe declaration range overflows" {
					t.Fatalf("NewProbePlan: expected range overflow, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("NewProbePlan: valid final-register range rejected: %v", err)
			}
		})
	}
}
