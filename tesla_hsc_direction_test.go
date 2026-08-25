package modbusreg

import (
	"bytes"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestTeslaHSCRequestEnvelopeIsDirectionalAndBounded(t *testing.T) {
	maximum := make([]byte, 252)
	maximum[0] = 251
	overflow := make([]byte, 253)
	overflow[0] = 252

	for _, test := range []struct {
		name     string
		function modbus.PrivateFunctionCode
		payload  []byte
		valid    bool
	}{
		{name: "fc100_empty", function: teslaHSCFunction100, payload: []byte{0}, valid: true},
		{name: "fc101_empty", function: teslaHSCFunction101, payload: []byte{0}, valid: true},
		{name: "fc102_empty", function: teslaHSCFunction102, payload: []byte{0}, valid: true},
		{name: "maximum", function: teslaHSCFunction101, payload: maximum, valid: true},
		{name: "missing_prefix", function: teslaHSCFunction100, payload: nil},
		{name: "underflow", function: teslaHSCFunction102, payload: []byte{1}},
		{name: "trailing", function: teslaHSCFunction101, payload: []byte{0, 0}},
		{name: "overflow", function: teslaHSCFunction100, payload: overflow},
	} {
		t.Run(test.name, func(t *testing.T) {
			envelope, err := DecodeTeslaHSCRequestEnvelope(test.function, test.payload)
			if test.valid {
				if err != nil {
					t.Fatal(err)
				}
				if got, want := envelope.Payload(), test.payload[1:]; !bytes.Equal(got, want) {
					t.Fatalf("payload = %x, want %x", got, want)
				}
				return
			}
			if err == nil {
				t.Fatalf("invalid request envelope accepted: %x", test.payload)
			}
		})
	}
}

func TestTeslaHSCResponseDecodingKeepsFC101And102Raw(t *testing.T) {
	maximum := make([]byte, 252)
	overflow := make([]byte, 253)
	for _, function := range []modbus.PrivateFunctionCode{
		teslaHSCFunction101,
		teslaHSCFunction102,
	} {
		for _, payload := range [][]byte{nil, {0}, {1}, {2, 0}, maximum} {
			response, err := DecodeTeslaHSCResponse(function, payload)
			if err != nil {
				t.Fatalf("function %d payload %x: %v", function, payload, err)
			}
			if got := response.Payload(); !bytes.Equal(got, payload) {
				t.Fatalf("function %d payload = %x, want %x", function, got, payload)
			}
		}
		if _, err := DecodeTeslaHSCResponse(function, overflow); err == nil {
			t.Fatalf("function %d accepted oversized response", function)
		}
	}
	if _, err := DecodeTeslaHSCResponse(teslaHSCFunction100, []byte{0}); err != nil {
		t.Fatal(err)
	}
	for _, payload := range [][]byte{nil, {1}, {0, 0}} {
		if _, err := DecodeTeslaHSCResponse(teslaHSCFunction100, payload); err == nil {
			t.Fatalf("FC100 invalid response accepted: %x", payload)
		}
	}
}

func TestTeslaTEDAPIRegistryRequiresDirectionAndPreservesRawResponse(t *testing.T) {
	profile := testTeslaDirectionalProfile(t)
	registry := NewTeslaTEDAPISemanticRegistry()
	if err := registry.Retain(TeslaTEDAPIObservationSpec{
		ID: "missing-direction", Profile: profile, Function: teslaHSCFunction101, Payload: []byte{0},
	}); err == nil {
		t.Fatal("observation without direction retained")
	}
	if err := registry.Retain(TeslaTEDAPIObservationSpec{
		ID: "request", Profile: profile, Direction: TeslaTEDAPIRequest,
		Function: teslaHSCFunction101, Payload: []byte{0},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Retain(TeslaTEDAPIObservationSpec{
		ID: "response", Profile: profile, Direction: TeslaTEDAPIResponse,
		Function: teslaHSCFunction101, Payload: []byte{1},
	}); err != nil {
		t.Fatal(err)
	}
	request, ok := registry.Lookup("request")
	if !ok || request.Direction != TeslaTEDAPIRequest || request.PayloadLength != 0 || request.OutboundAllowed {
		t.Fatalf("request record = %#v", request)
	}
	response, ok := registry.Lookup("response")
	if !ok || response.Direction != TeslaTEDAPIResponse || response.PayloadLength != 1 || response.OutboundAllowed {
		t.Fatalf("response record = %#v", response)
	}
}

func TestTeslaHSCNoSendSurvivesDirectionalValidation(t *testing.T) {
	profile := testTeslaDirectionalProfile(t)
	for _, operation := range []string{"fc100", "fc101", "fc102"} {
		if _, _, err := profile.EncodeQualifiedFunction(operation); err == nil {
			t.Fatalf("operation %q was admitted", operation)
		}
	}
}

func testTeslaDirectionalProfile(t *testing.T) TeslaHSCProfile {
	t.Helper()
	profile, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return profile
}
