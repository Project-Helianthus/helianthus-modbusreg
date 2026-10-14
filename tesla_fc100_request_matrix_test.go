package modbusreg

import (
	"bytes"
	"testing"
)

func TestTeslaFC100RequestMatrixRecognizesRetainedEmptyRequests(t *testing.T) {
	for _, operation := range []TeslaFC100Operation{
		TeslaFC100OperationCommonSystemInfo,
		TeslaFC100OperationWCGetVitals,
		TeslaFC100OperationWCGetLifetimeStats,
		TeslaFC100OperationWCGetSystemInfo,
		TeslaFC100OperationWCGetLoadSharingState,
		TeslaFC100OperationWCGetProvisional,
	} {
		t.Run(string(operation), func(t *testing.T) {
			matrix, err := ValidateTeslaFC100RequestMatrix(TeslaHSCFC100OperationCompatibilityV1, operation, nil)
			if err != nil {
				t.Fatal(err)
			}
			if matrix.Operation != operation || matrix.RequestBody != nil || len(matrix.Entries) != 0 {
				t.Fatalf("fixed matrix = %#v", matrix)
			}
			if _, err := ValidateTeslaFC100RequestMatrix(TeslaHSCFC100OperationCompatibilityV1, operation, []byte{0x08, 0x01}); err == nil {
				t.Fatal("fixed request with a body was accepted")
			}
		})
	}
}

func TestTeslaFC100RequestMatrixValidatesRetainedStructuredBodies(t *testing.T) {
	for _, operation := range []TeslaFC100Operation{TeslaFC100OperationWCConfigureSettings, TeslaFC100OperationWCSetProvisional} {
		t.Run(string(operation), func(t *testing.T) {
			for _, test := range []struct {
				name      string
				body      []byte
				wantError bool
			}{
				{"empty", nil, true},
				{"malformed", []byte{0x80}, true},
				{"nested values", []byte{0x0a, 0x02, 0x08, 0x10}, false},
				{"unknown retained", []byte{0x7a, 0x02, 0xaa, 0xbb}, false},
				{"unknown group retained", []byte{0x1b, 0x20, 0x01, 0x1c}, false},
				{"over FC100 bound", bytes.Repeat([]byte{0x08, 0x01}, 126), true},
			} {
				t.Run(test.name, func(t *testing.T) {
					matrix, err := ValidateTeslaFC100RequestMatrix(TeslaHSCFC100OperationCompatibilityV1, operation, test.body)
					if (err != nil) != test.wantError {
						t.Fatalf("ValidateTeslaFC100RequestMatrix() error = %v, wantError %v", err, test.wantError)
					}
					if err == nil {
						if matrix.Operation != operation || !bytes.Equal(matrix.RequestBody, test.body) || len(matrix.Entries) == 0 {
							t.Fatalf("matrix = %#v, want retained body %x", matrix, test.body)
						}
						test.body[0] ^= 0xff
						if matrix.RequestBody[0] == test.body[0] {
							t.Fatal("matrix retained caller-owned body")
						}
					}
				})
			}
		})
	}
}

func TestTeslaFC100RequestMatrixRejectsUnknownVersion(t *testing.T) {
	if _, err := ValidateTeslaFC100RequestMatrix("unknown", TeslaFC100OperationWCGetVitals, nil); err == nil {
		t.Fatal("unknown operation version was accepted")
	}
}
