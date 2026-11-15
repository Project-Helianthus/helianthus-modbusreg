package modbusreg_test

import (
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/detection"
)

func TestProbeFc03OverflowAddressBoundary(t *testing.T) {
	limits := detection.DefaultDetectionLimits()
	for _, tc := range []struct {
		name     string
		function detection.ProbeFunction
		address  uint16
		width    uint16
		overflow bool
	}{
		{"fc03_final_register", detection.ProbeReadHoldingRegisters, 65535, 1, false},
		{"fc03_overflow", detection.ProbeReadHoldingRegisters, 65535, 2, true},
		{"fc03_two_words_ending_at_final_register", detection.ProbeReadHoldingRegisters, 65534, 2, false},
		{"fc04_final_register", detection.ProbeReadInputRegisters, 65535, 1, false},
		{"fc04_overflow", detection.ProbeReadInputRegisters, 65535, 2, true},
		{"fc04_two_words_ending_at_final_register", detection.ProbeReadInputRegisters, 65534, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := detection.NewProbePlan(detection.ProbePlanSpec{
				Version: reg.MustParseVersion("1.0.0"),
				Declarations: []detection.ProbeDeclarationSpec{{
					ID:            "synthetic-manufacturer",
					Function:      tc.function,
					Address:       tc.address,
					WordCount:     tc.width,
					IdentityField: detection.ProbeIdentityManufacturer,
					Encoding:      detection.ProbeIdentityASCII,
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
