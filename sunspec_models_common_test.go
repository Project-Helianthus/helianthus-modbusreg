package modbusreg

import "testing"

func TestSunSpecCommonL66AndL65DecodeOnlyByExactKey(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string][]uint16{
		"Mn": stringWords("Fronius", 16), "Md": stringWords("GEN24", 16),
		"Opt": stringWords("", 8), "Vr": stringWords("1.2.3", 8), "SN": stringWords("SERIAL", 16),
		"DA": {1}, "Pad": {0x8000},
	}
	for _, length := range []uint16{65, 66} {
		decoded, err := registry.DecodeOccurrence(admittedOccurrence(1, length, modelWords(t, registry, 1, length, values), 1))
		if err != nil {
			t.Fatalf("L%d: %v", length, err)
		}
		if decoded.Key().ModelLength != length || decoded.Topology() != SunSpecTopologyNone {
			t.Fatalf("L%d decoded=%#v", length, decoded)
		}
		fact, ok := decoded.Fact("device.manufacturer")
		if !ok {
			t.Fatalf("L%d manufacturer absent", length)
		}
		if text, ok := fact.Value.Text(); !ok || text != "Fronius" {
			t.Fatalf("L%d manufacturer=%q/%v", length, text, ok)
		}
	}
	unsupported := admittedOccurrence(1, 64, make([]uint16, 66), 1)
	unsupported.decoderKey = nil
	unsupported.Disposition = SunSpecChainDispositionUnsupportedLength
	if _, err := registry.DecodeOccurrence(unsupported); err == nil {
		t.Fatal("unsupported Common length decoded")
	}
}

func TestSunSpecChainDecodeRequiresExactlyOneSupportedCommonFirst(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	common := admittedOccurrence(1, 65, modelWords(t, registry, 1, 65, map[string][]uint16{"Mn": stringWords("Maker", 16)}), 1)
	inverter := admittedOccurrence(103, 50, modelWords(t, registry, 103, 50, nil), 2)
	for name, occurrences := range map[string][]SunSpecOccurrence{
		"missing": {inverter}, "out of order": {inverter, common}, "repeated": {common, common},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := registry.decodeOccurrences(occurrences); err == nil {
				t.Fatal("invalid Common structure decoded")
			}
		})
	}
	decoded, err := registry.decodeOccurrences([]SunSpecOccurrence{common, inverter})
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Models()) != 2 {
		t.Fatalf("models=%d", len(decoded.Models()))
	}
}
