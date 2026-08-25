package modbusreg

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestTeslaHSCOpaqueTerminalProvenanceRetainsOnlySelectedFC101AndFC102ResponseMetadata(t *testing.T) {
	for _, test := range []struct {
		name     string
		function modbus.PrivateFunctionCode
		payload  []byte
	}{
		{name: "fc101", function: teslaHSCFunction101, payload: []byte{0x01}},
		{name: "fc102", function: teslaHSCFunction102, payload: []byte{0x02, 0x00}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := DecodeTeslaHSCOpaqueTerminalProvenance(test.function, [][]byte{test.payload})
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(test.payload)
			if got.Function() != test.function || got.PayloadLength() != len(test.payload) ||
				got.PayloadDigest() != hex.EncodeToString(digest[:]) {
				t.Fatalf("redacted opaque terminal provenance = %#v", got)
			}
		})
	}
}

func TestTeslaHSCOpaqueTerminalProvenanceRejectsNonterminalOrUnselectedExchange(t *testing.T) {
	if _, err := DecodeTeslaHSCOpaqueTerminalProvenance(teslaHSCFunction100, [][]byte{{0}}); err == nil {
		t.Fatal("FC100 intermediate-capable exchange accepted as opaque terminal provenance")
	}
	if _, err := DecodeTeslaHSCOpaqueTerminalProvenance(teslaHSCFunction101, [][]byte{{0x01}, {0x02}}); err == nil {
		t.Fatal("multiple FC101 responses accepted as one terminal response")
	}
}
