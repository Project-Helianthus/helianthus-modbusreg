package modbusreg

import (
	"math"
	"testing"
)

func mustStandardSunSpecRegistry(t *testing.T) SunSpecDecoderRegistry {
	t.Helper()
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func commonOccurrence(t *testing.T, registry SunSpecDecoderRegistry, manufacturer, model, firmware string, ordinal uint32) SunSpecOccurrence {
	t.Helper()
	return admittedOccurrence(1, 65, modelWords(t, registry, 1, 65, map[string][]uint16{
		"Mn": stringWords(manufacturer, 16), "Md": stringWords(model, 16), "Vr": stringWords(firmware, 8), "SN": stringWords("sanitized", 16),
	}), ordinal)
}

func inverterOccurrence(t *testing.T, registry SunSpecDecoderRegistry, modelID uint16, override map[string][]uint16, ordinal uint32) SunSpecOccurrence {
	t.Helper()
	length := uint16(50)
	values := map[string][]uint16{
		"A": {1}, "AphA": {1}, "AphB": {1}, "AphC": {1}, "A_SF": {0},
		"PhVphA": {1}, "PhVphB": {1}, "PhVphC": {1}, "V_SF": {0},
		"W": {1}, "W_SF": {0}, "Hz": {1}, "Hz_SF": {0}, "WH": {0, 1}, "WH_SF": {0},
		"TmpCab": {1}, "Tmp_SF": {0}, "St": {4}, "Evt1": {0, 0}, "Evt2": {0, 0},
	}
	if modelID == 113 {
		length = 60
		for _, name := range []string{"A", "AphA", "AphB", "AphC", "PhVphA", "PhVphB", "PhVphC", "W", "Hz", "WH", "TmpCab"} {
			values[name] = float32Words(1)
		}
		delete(values, "A_SF")
		delete(values, "V_SF")
		delete(values, "W_SF")
		delete(values, "Hz_SF")
		delete(values, "WH_SF")
		delete(values, "Tmp_SF")
	}
	for name, value := range override {
		values[name] = value
	}
	return admittedOccurrence(modelID, length, modelWords(t, registry, modelID, length, values), ordinal)
}

func capabilitySnapshot(t *testing.T, registry SunSpecDecoderRegistry, modelID uint16, override map[string][]uint16) SunSpecChainSnapshot {
	t.Helper()
	return snapshotFromOccurrences(
		commonOccurrence(t, registry, "Maker", "Model", "1.0", 1),
		inverterOccurrence(t, registry, modelID, override, 2),
	)
}

func snapshotFromOccurrences(occurrences ...SunSpecOccurrence) SunSpecChainSnapshot {
	return SunSpecChainSnapshot{
		occurrences: occurrences,
		raw:         rawWordsForOccurrences(occurrences),
		sources: []LogicalViewRecord{{
			LogicalViewID: 1, WireResponseID: 2, PhysicalRequestID: 3, ConnectionID: 4,
			TransportGeneration: 5, PollGeneration: 6, Words: []uint16{1}, WireResponseBytes: []byte{0, 1},
		}},
	}
}

func rawWordsForOccurrences(occurrences []SunSpecOccurrence) []uint16 {
	raw := []uint16{sunSpecSignatureFirst, sunSpecSignatureSecond}
	for _, occurrence := range occurrences {
		raw = append(raw, occurrence.Words()...)
	}
	return append(raw, sunSpecEndModel, 0)
}

func float32Words(value float32) []uint16 {
	bits := math.Float32bits(value)
	return []uint16{uint16(bits >> 16), uint16(bits)}
}
