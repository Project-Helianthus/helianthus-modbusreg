package modbusreg

import (
	"bytes"
	"testing"
)

func TestTeslaFC100RequestMatrixRecognizesFixedAndStructuredRequests(t *testing.T) {
	matrix, err := ValidateTeslaFC100RequestMatrix(TeslaHSCFC100OperationCompatibilityV1, TeslaFC100OperationWCGetConfig, nil)
	if err != nil {
		t.Fatal(err)
	}
	if matrix.RequestBody != nil || len(matrix.Entries) != 0 {
		t.Fatalf("fixed matrix = %#v", matrix)
	}
	if _, err := ValidateTeslaFC100RequestMatrix(TeslaHSCFC100OperationCompatibilityV1, TeslaFC100OperationCommonWifiScan, nil); err == nil {
		t.Fatal("empty C10 accepted")
	}
	if status, ok := TeslaFC100DeterministicApplicationStatus(TeslaFC100OperationWCPushPPUAuthorization); !ok || status != 7 {
		t.Fatalf("W35 status = %d, %v", status, ok)
	}
}

func TestTeslaFC100RequestMatrixValidatesStructuredAndOpaqueBodies(t *testing.T) {
	for _, test := range []struct {
		name      string
		operation TeslaFC100Operation
		body      []byte
		wantError bool
	}{
		{"C10 known varint", TeslaFC100OperationCommonWifiScan, []byte{0x08, 0x01}, false},
		{"C10 wrong wire", TeslaFC100OperationCommonWifiScan, []byte{0x0a, 0x00}, true},
		{"C12 enabled", TeslaFC100OperationCommonConfigureWifi, []byte{0x08, 0x01}, false},
		{"C12 wifi string", TeslaFC100OperationCommonConfigureWifi, []byte{0x12, 0x03, 0x0a, 0x01, 'x'}, false},
		{"C12 malformed nested", TeslaFC100OperationCommonConfigureWifi, []byte{0x12, 0x01, 0x0a}, true},
		{"C12 unknown group retained", TeslaFC100OperationCommonConfigureWifi, []byte{0x08, 0x01, 0x1b, 0x20, 0x01, 0x1c}, false},
		{"opaque malformed", TeslaFC100OperationWCConfigureSettings, []byte{0x80}, true},
		{"opaque unknown retained", TeslaFC100OperationWCConfigureSettings, []byte{0x7a, 0x02, 0xaa, 0xbb}, false},
		{"opaque over FC100 bound", TeslaFC100OperationWCConfigureSettings, bytes.Repeat([]byte{0x08, 0x01}, 126), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			matrix, err := ValidateTeslaFC100RequestMatrix(TeslaHSCFC100OperationCompatibilityV1, test.operation, test.body)
			if (err != nil) != test.wantError {
				t.Fatalf("ValidateTeslaFC100RequestMatrix() error = %v, wantError %v", err, test.wantError)
			}
			if err == nil && !bytes.Equal(matrix.RequestBody, test.body) {
				t.Fatalf("body = %x, want %x", matrix.RequestBody, test.body)
			}
		})
	}
}
