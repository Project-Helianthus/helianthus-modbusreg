package modbusreg

import (
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestTeslaHSCExchangeDecodesSelectedFunctionWithoutAdmission(t *testing.T) {
	for _, test := range []struct {
		function modbus.PrivateFunctionCode
		frames   [][]byte
		want     [][]byte
	}{
		{teslaHSCFunction100, [][]byte{{0}, {1, 0xaa}}, [][]byte{{}, {0xaa}}},
		{teslaHSCFunction101, [][]byte{{1, 0}}, [][]byte{{1, 0}}},
		{teslaHSCFunction102, [][]byte{{2, 0, 1}}, [][]byte{{2, 0, 1}}},
	} {
		t.Run(string(rune(test.function)), func(t *testing.T) {
			responses, err := DecodeTeslaHSCExchange(test.function, test.frames)
			if err != nil {
				t.Fatal(err)
			}
			if len(responses) != len(test.want) {
				t.Fatalf("responses = %d, want %d", len(responses), len(test.want))
			}
			for index := range test.want {
				if got := responses[index].Payload(); string(got) != string(test.want[index]) {
					t.Fatalf("payload[%d] = %x, want %x", index, got, test.want[index])
				}
			}
		})
	}
}

func TestTeslaHSCExchangeRejectsWrongResponseMultiplicity(t *testing.T) {
	if _, err := DecodeTeslaHSCExchange(teslaHSCFunction101, [][]byte{{1}, {2}}); err == nil {
		t.Fatal("FC101 accepted multiple normal responses")
	}
	if _, err := DecodeTeslaHSCExchange(teslaHSCFunction102, nil); err == nil {
		t.Fatal("FC102 accepted no normal response")
	}
}
