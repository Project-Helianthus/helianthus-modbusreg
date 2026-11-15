package modbusreg_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/detection"
)

func TestProbeResultStatusExclusionSuccess(t *testing.T) {
	result, err := detection.NewProbeReadResult(detection.ProbeReadResultSpec{
		Status:     detection.ProbeReadSucceeded,
		Words:      []uint16{0x1234},
		EvidenceID: "synthetic-probe-result",
	})
	if err != nil {
		t.Fatalf("one-word success rejected: %v", err)
	}
	if result.Status() != detection.ProbeReadSucceeded ||
		!reflect.DeepEqual(result.Words(), []uint16{0x1234}) ||
		result.ExceptionCode() != 0 {
		t.Fatalf("success payload changed: %+v", result)
	}
}

func TestProbeResultStatusExclusionExceptionCodes(t *testing.T) {
	for code := 1; code <= 255; code++ {
		t.Run(fmt.Sprintf("code_%03d", code), func(t *testing.T) {
			result, err := detection.NewProbeReadResult(detection.ProbeReadResultSpec{
				Status:        detection.ProbeReadException,
				EvidenceID:    "synthetic-probe-result",
				ExceptionCode: uint8(code),
			})
			if err != nil {
				t.Fatalf("exception code %d rejected: %v", code, err)
			}
			if result.Status() != detection.ProbeReadException ||
				len(result.Words()) != 0 || result.ExceptionCode() != uint8(code) {
				t.Fatalf("exception code %d payload changed: %+v", code, result)
			}
		})
	}
}

func TestProbeResultStatusExclusionContradictions(t *testing.T) {
	tests := []struct {
		name string
		spec detection.ProbeReadResultSpec
	}{
		{
			name: "success_with_exception_code",
			spec: detection.ProbeReadResultSpec{Status: detection.ProbeReadSucceeded, Words: []uint16{0x1234}, ExceptionCode: 1},
		},
		{
			name: "exception_with_words",
			spec: detection.ProbeReadResultSpec{Status: detection.ProbeReadException, Words: []uint16{0x1234}, ExceptionCode: 1},
		},
		{
			name: "exception_with_zero_code",
			spec: detection.ProbeReadResultSpec{Status: detection.ProbeReadException},
		},
		{
			name: "unknown_status",
			spec: detection.ProbeReadResultSpec{Status: detection.ProbeReadStatus("unknown"), Words: []uint16{0x1234}},
		},
		{
			name: "empty_success",
			spec: detection.ProbeReadResultSpec{Status: detection.ProbeReadSucceeded},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.spec.EvidenceID = "synthetic-probe-result"
			result, err := detection.NewProbeReadResult(test.spec)
			if err == nil {
				t.Error("contradictory result accepted")
			}
			if !reflect.DeepEqual(result, detection.ProbeReadResult{}) {
				t.Errorf("constructor error must return the zero result: %+v", result)
			}
		})
	}
}
