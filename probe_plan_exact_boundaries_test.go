package modbusreg_test

import (
	"reflect"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestU23ProbePlanAggregateWordBoundary(t *testing.T) {
	spec := reg.ProbePlanSpec{
		Version: reg.MustParseVersion("1.0.0"),
		Declarations: []reg.ProbeDeclarationSpec{
			{ID: "u23-manufacturer", Function: reg.ProbeReadHoldingRegisters, Address: 100, WordCount: 2, IdentityField: reg.ProbeIdentityManufacturer, Encoding: reg.ProbeIdentityASCII},
			{ID: "u23-model", Function: reg.ProbeReadHoldingRegisters, Address: 110, WordCount: 3, IdentityField: reg.ProbeIdentityModel, Encoding: reg.ProbeIdentityASCII},
			{ID: "u23-firmware", Function: reg.ProbeReadHoldingRegisters, Address: 120, WordCount: 4, IdentityField: reg.ProbeIdentityFirmware, Encoding: reg.ProbeIdentityASCII},
		},
	}
	for _, tc := range []struct {
		name    string
		limit   int
		wantErr string
	}{
		{name: "exact_nine_words", limit: 9},
		{name: "third_declaration_crosses_eight", limit: 8, wantErr: "probe plan exceeds the aggregate word bound"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := reg.DefaultDetectionLimits()
			limits.MaxTotalWords = tc.limit
			plan, err := reg.NewProbePlan(spec, limits)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("NewProbePlan aggregate limit %d: error = %v, want %q", tc.limit, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewProbePlan at exact aggregate limit: %v", err)
			}
			if !reflect.DeepEqual(plan.Spec(), spec) {
				t.Fatalf("accepted plan = %+v, want %+v", plan.Spec(), spec)
			}
		})
	}
}

func TestU23ProbePlanLastAddressBoundary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		address uint16
		wantErr string
	}{
		{name: "ends_at_65535", address: 65534},
		{name: "forged_start_ends_at_65536", address: 65535, wantErr: "probe declaration range overflows"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := reg.ProbePlanSpec{
				Version: reg.MustParseVersion("1.0.0"),
				Declarations: []reg.ProbeDeclarationSpec{
					{ID: "u23-last-address", Function: reg.ProbeReadInputRegisters, Address: tc.address, WordCount: 2, IdentityField: reg.ProbeIdentityManufacturer, Encoding: reg.ProbeIdentityASCII},
				},
			}
			plan, err := reg.NewProbePlan(spec, reg.DefaultDetectionLimits())
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("NewProbePlan start %d, width 2: error = %v, want %q", tc.address, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewProbePlan at exact final address: %v", err)
			}
			if !reflect.DeepEqual(plan.Spec(), spec) {
				t.Fatalf("accepted plan = %+v, want %+v", plan.Spec(), spec)
			}
		})
	}
}
