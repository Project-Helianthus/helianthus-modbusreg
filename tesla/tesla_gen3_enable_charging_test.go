package tesla

import (
	"bytes"
	"context"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestTeslaNativeCommandEnableCharging(t *testing.T) {
	testTeslaNativeChargeCommand(t, 1, "wc.enable_charging", EncodeTeslaEnableCharging, DecodeTeslaEnableCharging, (*Gen3ReadAcquisition).EnableCharging)
}

func testTeslaNativeChargeCommand(t *testing.T, command byte, name string,
	encode func(string) (modbus.PrivateFunctionRequest, error),
	decode func(string, []byte) (FC100OperationResult, error),
	call func(*Gen3ReadAcquisition, context.Context) (Gen3ReadRecord, error),
) {
	t.Helper()
	empty := []byte{5, 0x32, 3, 0xc2, 2, 0}
	for _, firmware := range []string{"26.34.0", "26.42.0", "24.28.3", "24.44.3", "MID 25.2.2", "26.18.0", "26.42.1"} {
		t.Run(firmware, func(t *testing.T) {
			if firmware == "24.28.3" {
				if _, err := encode(firmware); err == nil {
					t.Fatal("undocumented layout was encoded")
				}
				return
			}
			h := newTeslaNativeCommandHarness(t, firmware)
			ep := h.endpoint()
			if firmware != "26.34.0" && firmware != "26.42.0" {
				receipt, err := call(h.acquisition, context.Background())
				if err == nil || receipt.State != Gen3ReadEvidenceNoSend || ep.exchanges() != 0 {
					t.Fatalf("unsupported firmware: %+v %v", receipt, err)
				}
				if _, err := decode(firmware, empty); err == nil {
					t.Fatal("undocumented layout was decoded")
				}
				return
			}
			want := []byte{9, 0x32, 7, 0xba, 2, 4, 10, 2, 8, command}
			for _, tc := range []struct {
				name          string
				payload, body []byte
				kind          FC100OperationResultKind
				status        uint64
			}{
				{"empty success", empty, nil, FC100OperationTerminal, 0},
				{"unknown field", []byte{7, 0x32, 5, 0xc2, 2, 2, 0x38, 9}, []byte{0x38, 9}, FC100OperationTerminal, 0},
				{"error status", []byte{8, 0x22, 6, 0x0a, 4, 0x0a, 2, 0x08, 9}, nil, FC100OperationApplicationError, 9},
			} {
				t.Run(tc.name, func(t *testing.T) {
					count := ep.exchanges()
					ep.push(teslaGen3FakeStep{terminal: tc.payload})
					receipt, err := call(h.acquisition, context.Background())
					if err != nil || receipt.NativeCommand != name || receipt.RequestTag != 39 || receipt.TerminalTag != 40 || ep.exchanges() != count+1 {
						t.Fatalf("receipt: %+v %v", receipt, err)
					}
					request := ep.requests[count]
					if request.FunctionCode().Byte() != 100 || !bytes.Equal(request.Payload(), want) {
						t.Fatalf("sent %x, want %x", request.Payload(), want)
					}
					results := receipt.Results()
					if len(results) != 1 || results[0].Kind != tc.kind || results[0].Status != tc.status || !bytes.Equal(results[0].Body, tc.body) {
						t.Fatalf("results: %+v", results)
					}
					if tc.kind == FC100OperationApplicationError {
						if receipt.State != Gen3ReadEvidenceRejected || !receipt.HasApplicationStatus || receipt.ApplicationStatus != tc.status {
							t.Fatalf("status: %+v", receipt)
						}
					} else if receipt.State != Gen3ReadEvidenceAck || receipt.HasApplicationStatus {
						t.Fatalf("success: %+v", receipt)
					}
					standalone, err := decode(firmware, tc.payload)
					if err != nil || standalone.Kind != tc.kind || standalone.Status != tc.status || !bytes.Equal(standalone.Body, tc.body) {
						t.Fatalf("standalone: %+v %v", standalone, err)
					}
				})
			}
		})
	}
}
