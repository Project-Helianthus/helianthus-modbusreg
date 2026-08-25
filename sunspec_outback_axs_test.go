package modbusreg

import (
	"strings"
	"testing"
)

func TestOutBackAXSReadOnlyDecoderIsExplicitAndStandardRegistryStaysIsolated(t *testing.T) {
	standard := mustStandardSunSpecRegistry(t)
	for _, key := range []SunSpecDecoderKey{
		{ModelID: 64110, ModelLength: 282, SchemaRevision: SunSpecModelsRevisionV1},
		{ModelID: 64111, ModelLength: 23, SchemaRevision: SunSpecModelsRevisionV1},
	} {
		if _, ok := standard.definition(key); ok {
			t.Fatalf("standard registry must not expose vendor key %#v", key)
		}
	}

	decoder, err := NewOutBackAXSReadOnlyDecoder()
	if err != nil {
		t.Fatalf("NewOutBackAXSReadOnlyDecoder() error = %v", err)
	}
	words := make([]uint16, 284)
	words[0], words[1] = 64110, 282
	words[2], words[3], words[4] = 1, 2, 3
	words[278], words[279], words[280] = 25, 20, 0
	words[281], words[282] = 0x8000, 0x0001
	decoded, err := decoder.Decode(words)
	if err != nil {
		t.Fatalf("decoder.Decode() error = %v", err)
	}
	if decoded.Key() != (SunSpecDecoderKey{ModelID: 64110, ModelLength: 282, SchemaRevision: SunSpecModelsRevisionV1}) {
		t.Fatalf("unexpected decoder key: %#v", decoded.Key())
	}
	for _, fact := range decoded.Facts() {
		for _, forbidden := range []string{"network", "password", "mac", "config", "control"} {
			if strings.Contains(fact.FieldID, forbidden) {
				t.Fatalf("excluded fact leaked: %#v", fact)
			}
		}
	}
	if _, ok := decoded.Fact("outback.axs.firmware.major"); !ok {
		t.Fatal("firmware fact missing")
	}

	charge := make([]uint16, 25)
	charge[0], charge[1], charge[2] = 64111, 23, 7
	decoded, err = decoder.Decode(charge)
	if err != nil {
		t.Fatalf("charge decoder.Decode() error = %v", err)
	}
	if len(decoded.Facts()) != 0 {
		t.Fatalf("vendor-scoped raw charge block emitted facts: %#v", decoded.Facts())
	}
	if got := decoded.RawWords(); got[2] != 7 {
		t.Fatalf("vendor-scoped raw charge block was not retained: %#v", got)
	}
}

func TestOutBackAXSReadOnlyDecoderFailsClosed(t *testing.T) {
	decoder, err := NewOutBackAXSReadOnlyDecoder()
	if err != nil {
		t.Fatal(err)
	}
	for _, words := range [][]uint16{{64110, 281}, {64110, 282}, {64112, 64}, {64111, 22}} {
		if _, err := decoder.Decode(words); err == nil {
			t.Fatalf("decoder accepted unsupported or malformed words: %#v", words)
		}
	}
}
