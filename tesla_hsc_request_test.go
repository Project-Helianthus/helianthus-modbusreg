package modbusreg

import (
	"bytes"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestNewTeslaHSCSyntacticRequestBuildsExactLengthEnvelope(t *testing.T) {
	maximum := bytes.Repeat([]byte{0xa5}, maxTeslaHSCPayload-1)
	for _, test := range []struct {
		name     string
		function modbus.PrivateFunctionCode
		nested   []byte
	}{
		{name: "fc100_empty", function: teslaHSCFunction100},
		{name: "fc101_nested", function: teslaHSCFunction101, nested: []byte{0x0a, 0x00}},
		{name: "fc102_maximum", function: teslaHSCFunction102, nested: maximum},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, err := NewTeslaHSCSyntacticRequest(test.function, test.nested)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := request.Function(), test.function; got != want {
				t.Fatalf("function = %d, want %d", got, want)
			}
			payload := request.Payload()
			if got, want := len(payload), len(test.nested)+1; got != want {
				t.Fatalf("payload length = %d, want %d", got, want)
			}
			if got, want := payload[0], byte(len(test.nested)); got != want {
				t.Fatalf("length prefix = %d, want %d", got, want)
			}
			if got, want := payload[1:], test.nested; !bytes.Equal(got, want) {
				t.Fatalf("nested payload = %x, want %x", got, want)
			}
			envelope, err := DecodeTeslaHSCRequestEnvelope(test.function, payload)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := envelope.Payload(), test.nested; !bytes.Equal(got, want) {
				t.Fatalf("decoded nested payload = %x, want %x", got, want)
			}
		})
	}
}

func TestNewTeslaHSCSyntacticRequestFailsClosedAndDoesNotAdmit(t *testing.T) {
	if _, err := NewTeslaHSCSyntacticRequest(modbus.PrivateFunctionCode(99), nil); err == nil {
		t.Fatal("unsupported function accepted")
	}
	if _, err := NewTeslaHSCSyntacticRequest(teslaHSCFunction101, make([]byte, maxTeslaHSCPayload)); err == nil {
		t.Fatal("oversized nested payload accepted")
	}

	request, err := NewTeslaHSCSyntacticRequest(teslaHSCFunction101, []byte{0xff})
	if err != nil {
		t.Fatal(err)
	}
	payload := request.Payload()
	payload[0] = 0
	if got, want := request.Payload(), []byte{1, 0xff}; !bytes.Equal(got, want) {
		t.Fatalf("request payload was mutated through copy: %x", got)
	}

	profile := testTeslaDirectionalProfile(t)
	if profile.OutboundAllowed() {
		t.Fatal("syntactic request construction enabled outbound transport")
	}
	if _, _, err := profile.EncodeQualifiedFunction("tesla.hsc.fc101.syntactic.v1"); err == nil {
		t.Fatal("syntactic request construction admitted an operation")
	}
}
