package tesla

import (
	"bytes"
	"context"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestTeslaNativeCommandControlStatus(t *testing.T) {
	for _, firmware := range []string{"MID 25.2.2", "26.18.0", "26.34.0", "24.28.3", "24.44.3", "26.42.0", "26.34.1"} {
		t.Run(firmware, func(t *testing.T) {
			if firmware == "24.28.3" {
				if _, err := EncodeTeslaControlStatus(firmware, ControlStatusRequest{}); err == nil {
					t.Fatal("undocumented layout was encoded")
				}
				return
			}
			h := newTeslaNativeCommandHarness(t, firmware)
			ep := h.endpoint()
			poll := false
			input := ControlStatusRequest{StatusPoll: &poll}
			switch firmware {
			case "MID 25.2.2", "26.18.0", "26.34.0":
			default:
				receipt, reply, err := h.acquisition.ExchangeControlStatus(context.Background(), input)
				if err == nil || receipt.State != Gen3ReadEvidenceNoSend || reply.Snapshot != nil || ep.exchanges() != 0 {
					t.Fatalf("unsupported firmware: %+v %+v %v", receipt, reply, err)
				}
				return
			}
			terminal := []byte{8, 10, 6, 8, 7, 64, 5, 120, 3}
			ep.push(teslaGen3FakeStep{terminal: terminal})
			receipt, reply, err := h.acquisition.ExchangeControlStatus(context.Background(), input)
			if err != nil || receipt.State != Gen3ReadEvidenceAck || reply.Snapshot == nil || reply.Snapshot.OperatingStateCode == nil || *reply.Snapshot.OperatingStateCode != 7 || !bytes.Equal(reply.Payload(), terminal) {
				t.Fatalf("reply: %+v %+v %v", receipt, reply, err)
			}
			if (reply.Snapshot.ThresholdEventMaskEncoded != nil) != (firmware != "MID 25.2.2") {
				t.Fatalf("firmware snapshot: %+v", reply.Snapshot)
			}
			if ep.exchanges() != 1 || ep.requests[0].FunctionCode().Byte() != 101 || !bytes.Equal(ep.requests[0].Payload(), []byte{2, 8, 0}) {
				t.Fatalf("request: %+v", ep.requests)
			}
			ep.push(teslaGen3FakeStep{outcome: "exception", status: 4})
			receipt, reply, err = h.acquisition.ExchangeControlStatus(context.Background(), input)
			if err != nil || receipt.State != Gen3ReadEvidenceRejected || !receipt.HasExceptionStatus || receipt.ExceptionStatus != 4 || reply.Snapshot != nil || ep.exchanges() != 2 {
				t.Fatalf("exception: %+v %+v %v", receipt, reply, err)
			}
		})
	}
}

func newTeslaNativeCommandHarness(t *testing.T, firmware string) *teslaGen3ReadHarness {
	t.Helper()
	return newTeslaGen3ReadHarness(t, func(config *Gen3ReadAcquisitionConfig) {
		profile := config.Gen3.config
		profile.Version = firmware
		config.Gen3 = mustTeslaGen3Profile(t, profile)
	})
}

func TestTeslaNativeCommandControlUpdateLayout(t *testing.T) {
	flag := true
	mode := uint32(129)
	recheck := false
	for _, test := range []struct {
		firmware string
		input    ControlStatusRequest
		want     []byte
	}{
		{"MID 25.2.2", ControlStatusRequest{}, []byte{0}},
		{"MID 25.2.2", ControlStatusRequest{ControlUpdate: &ControlUpdate{ControlFlagOne: &flag}}, []byte{4, 18, 2, 8, 1}},
		{"26.18.0", ControlStatusRequest{ControlUpdate: &ControlUpdate{ControlFlagOne: &flag, ControllerModeHint: &mode, RequestExternalPowerRecheck: &recheck}}, []byte{9, 18, 7, 8, 1, 16, 129, 1, 24, 0}},
		{"26.34.0", ControlStatusRequest{ControlUpdate: &ControlUpdate{}}, []byte{2, 18, 0}},
		{"26.34.0", ControlStatusRequest{StatusPoll: &flag, ControlUpdate: &ControlUpdate{}}, nil},
		{"MID 25.2.2", ControlStatusRequest{ControlUpdate: &ControlUpdate{ControllerModeHint: &mode}}, nil},
		{"MID 25.2.2", ControlStatusRequest{ControlUpdate: &ControlUpdate{RequestExternalPowerRecheck: &recheck}}, nil},
	} {
		got, err := EncodeTeslaControlStatus(test.firmware, test.input)
		if (test.want == nil) != (err != nil) || !bytes.Equal(got.Payload(), test.want) {
			t.Fatalf("%s, %+v: payload %x, want %x, err=%v", test.firmware, test.input, got.Payload(), test.want, err)
		}
	}
}

func TestTeslaNativeCommandAdmissionBindsExactExchange(t *testing.T) {
	h := newTeslaNativeCommandHarness(t, "26.34.0")
	ep := h.endpoint()
	ep.onAdmission = func(candidate modbus.RTUPrivateFunctionAdmissionRequest) {
		admission := ep.config.PrivateFunction.Admission
		if !admission.AdmitRTUPrivateFunction(candidate) {
			t.Fatal("exact command not admitted")
		}
		for _, test := range []struct {
			field string
			alter func(*modbus.RTUPrivateFunctionAdmissionRequest)
		}{
			{"endpoint", func(c *modbus.RTUPrivateFunctionAdmissionRequest) { c.Endpoint += "-other" }},
			{"unit", func(c *modbus.RTUPrivateFunctionAdmissionRequest) { c.UnitID++ }},
			{"function", func(c *modbus.RTUPrivateFunctionAdmissionRequest) { c.Function = teslaHSCFunction100 }},
			{"payload", func(c *modbus.RTUPrivateFunctionAdmissionRequest) { c.Payload = []byte{2, 8, 1} }},
			{"generation", func(c *modbus.RTUPrivateFunctionAdmissionRequest) { c.Generation++ }},
		} {
			modified := candidate
			test.alter(&modified)
			if admission.AdmitRTUPrivateFunction(modified) {
				t.Errorf("admitted different %s", test.field)
			}
		}
		if h.acquisition.admit(h.acquisition.instance+1, candidate) {
			t.Error("admitted different endpoint instance")
		}
	}
	ep.push(teslaGen3FakeStep{terminal: []byte{4, 10, 2, 8, 1}, receiptGeneration: 2})
	receipt, reply, err := h.acquisition.ExchangeControlStatus(context.Background(), ControlStatusRequest{StatusPoll: teslaNativeTestPointer(false)})
	if err != nil || receipt.State != Gen3ReadEvidenceIndeterminate || reply.Snapshot != nil || len(receipt.ResponseADUs()) != 1 {
		t.Fatalf("old-generation receipt: %+v, %+v, %v", receipt, reply, err)
	}
	if ep.config.PrivateFunction.Admission.AdmitRTUPrivateFunction(modbus.RTUPrivateFunctionAdmissionRequest{
		Endpoint: ep.config.Endpoint, UnitID: h.profile.Node(), Function: teslaHSCFunction101, Payload: []byte{2, 8, 0}, Generation: 1,
	}) {
		t.Fatal("command admitted without a plan")
	}
}
