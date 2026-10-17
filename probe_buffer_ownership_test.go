package modbusreg_test

import (
	"reflect"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestU24ProbePlanBufferOwnership(t *testing.T) {
	for _, accessor := range []struct {
		name string
		read func(reg.ProbePlan) []reg.ProbeDeclarationSpec
	}{
		{"Spec", func(plan reg.ProbePlan) []reg.ProbeDeclarationSpec { return plan.Spec().Declarations }},
		{"Declarations", func(plan reg.ProbePlan) []reg.ProbeDeclarationSpec { return plan.Declarations() }},
	} {
		t.Run(accessor.name, func(t *testing.T) {
			expected := [2]reg.ProbeDeclarationSpec{
				{ID: "synthetic-manufacturer", Function: reg.ProbeReadHoldingRegisters, Address: 10, WordCount: 2, IdentityField: reg.ProbeIdentityManufacturer, Encoding: reg.ProbeIdentityASCII},
				{ID: "synthetic-firmware", Function: reg.ProbeReadInputRegisters, Address: 30, WordCount: 1, IdentityField: reg.ProbeIdentityFirmware, Encoding: reg.ProbeIdentityASCII},
			}
			input := append([]reg.ProbeDeclarationSpec(nil), expected[:]...)
			version := reg.MustParseVersion("0.7.0")
			plan, err := reg.NewProbePlan(reg.ProbePlanSpec{
				Version: version, Declarations: input,
			}, reg.DefaultDetectionLimits())
			if err != nil {
				t.Fatalf("NewProbePlan: %v", err)
			}
			assertPlan := func(phase string) {
				t.Helper()
				spec := plan.Spec()
				if spec.Version != version || !reflect.DeepEqual(spec.Declarations, expected[:]) {
					t.Fatalf("%s: Spec = %+v, want version %v and ordered declarations %+v", phase, spec, version, expected)
				}
				if got := plan.Declarations(); !reflect.DeepEqual(got, expected[:]) {
					t.Fatalf("%s: Declarations = %+v, want %+v", phase, got, expected)
				}
			}

			input[0], input[1] = input[1], input[0]
			input[0].Address = 90
			assertPlan("caller input mutation")

			first := accessor.read(plan)
			if !reflect.DeepEqual(first, expected[:]) {
				t.Fatalf("first output = %+v, want %+v", first, expected)
			}
			first[0].ID = "synthetic-output"
			first[1].Address = 70
			assertPlan("first output mutation")

			second := accessor.read(plan)
			if !reflect.DeepEqual(second, expected[:]) {
				t.Fatalf("second output = %+v, want %+v", second, expected)
			}
			second[0].ID = "synthetic-second-output"
			second[1].Address = 80
			assertPlan("second output mutation")
			firstExpected := expected
			firstExpected[0].ID = "synthetic-output"
			firstExpected[1].Address = 70
			if !reflect.DeepEqual(first, firstExpected[:]) {
				t.Fatalf("second output mutation changed first output: got %+v, want %+v", first, firstExpected)
			}
		})
	}
}

func TestU24ProbeReadResultBufferOwnership(t *testing.T) {
	expected := [2]uint16{0x1234, 0xabcd}
	input := append([]uint16(nil), expected[:]...)
	result, err := reg.NewProbeReadResult(reg.ProbeReadResultSpec{
		Status: reg.ProbeReadSucceeded, Words: input, EvidenceID: "synthetic-u24-result",
	})
	if err != nil {
		t.Fatalf("NewProbeReadResult: %v", err)
	}
	assertResult := func(phase string) {
		t.Helper()
		if got := result.Words(); !reflect.DeepEqual(got, expected[:]) {
			t.Fatalf("%s: Words = %04x, want %04x", phase, got, expected)
		}
		if result.Status() != reg.ProbeReadSucceeded || result.EvidenceID() != "synthetic-u24-result" || result.ExceptionCode() != 0 {
			t.Fatalf("%s: successful result metadata changed", phase)
		}
	}

	input[0], input[1] = 0x5678, 0xef01
	assertResult("caller input mutation")
	first := result.Words()
	if !reflect.DeepEqual(first, expected[:]) {
		t.Fatalf("first output = %04x, want %04x", first, expected)
	}
	first[0], first[1] = 0x2345, 0xbcde
	assertResult("first output mutation")
	second := result.Words()
	if !reflect.DeepEqual(second, expected[:]) {
		t.Fatalf("second output = %04x, want %04x", second, expected)
	}
	second[0], second[1] = 0x3456, 0xcdef
	assertResult("second output mutation")
	if want := []uint16{0x2345, 0xbcde}; !reflect.DeepEqual(first, want) {
		t.Fatalf("second output mutation changed first output: got %04x, want %04x", first, want)
	}
}
