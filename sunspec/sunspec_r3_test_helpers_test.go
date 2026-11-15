package sunspec

import (
	"math"
	"reflect"
	"testing"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func mustStandardSunSpecRegistry(t *testing.T) DecoderRegistry {
	t.Helper()
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func commonOccurrence(t *testing.T, registry DecoderRegistry, manufacturer, model, firmware string, ordinal uint32) Occurrence {
	t.Helper()
	return admittedOccurrence(1, 65, modelWords(t, registry, 1, 65, map[string][]uint16{
		"Mn": stringWords(manufacturer, 16), "Md": stringWords(model, 16), "Vr": stringWords(firmware, 8), "SN": stringWords("sanitized", 16),
	}), ordinal)
}

func inverterOccurrence(t *testing.T, registry DecoderRegistry, modelID uint16, override map[string][]uint16, ordinal uint32) Occurrence {
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

func capabilitySnapshot(t *testing.T, registry DecoderRegistry, modelID uint16, override map[string][]uint16) ChainSnapshot {
	t.Helper()
	return snapshotFromOccurrences(
		commonOccurrence(t, registry, "Maker", "Model", "1.0", 1),
		inverterOccurrence(t, registry, modelID, override, 2),
	)
}

func snapshotFromOccurrences(occurrences ...Occurrence) ChainSnapshot {
	cloned := make([]Occurrence, len(occurrences))
	sources := make([]modbusreg.LogicalViewSnapshot, len(occurrences))
	header := uint16(40002)
	for index, occurrence := range occurrences {
		occurrence = cloneOccurrence(occurrence)
		occurrence.HeaderOffset = header
		occurrence.PayloadOffset = header + 2
		logicalViewID := uint64(index + 1)
		occurrence.spans = []SourceSpan{{LogicalViewID: logicalViewID, PDUOffset: 0, WordCount: uint16(len(occurrence.words))}}
		cloned[index] = occurrence
		source, err := modbusreg.NewLogicalViewSnapshot(modbusreg.LogicalViewRecord{
			LogicalViewID: logicalViewID, WireResponseID: uint64(index + 101), PhysicalRequestID: uint64(index + 201),
			Endpoint: "fixture-endpoint", ConnectionID: 4, Transport: modbusreg.TransportTCP, TransportGeneration: 5, UnitID: 1,
			RequestedFunction: modbusreg.FunctionReadHoldingRegisters, ReceivedFunction: modbusreg.FunctionReadHoldingRegisters, Table: modbusreg.HoldingRegisters,
			PhysicalOffset: header, PhysicalWordCount: uint16(len(occurrence.words)), AuthorizationScope: "fixture-read-only",
			PollGeneration: 6, DeadlineIdentity: uint64(index + 301), LogicalOffset: header,
			LogicalWordCount: uint16(len(occurrence.words)), SliceWordCount: uint16(len(occurrence.words)),
			Words: occurrence.Words(), WireResponseBytes: []byte{0, byte(index + 1)},
		})
		if err != nil {
			panic(err)
		}
		sources[index] = source
		header += uint16(len(occurrence.words))
	}
	return ChainSnapshot{
		occurrences: cloned,
		raw:         rawWordsForOccurrences(cloned),
		sources:     sources,
	}
}

func cloneSnapshotForTest(snapshot ChainSnapshot) ChainSnapshot {
	sources := make([]modbusreg.LogicalViewSnapshot, len(snapshot.sources))
	for index, source := range snapshot.sources {
		sources[index] = source.Clone()
	}
	return ChainSnapshot{
		occurrences: snapshot.Occurrences(),
		raw:         snapshot.RawWords(),
		sources:     sources,
	}
}

// sharesViewStorage reports whether two source views hold the same word or
// wire-byte storage. Views are immutable through their accessors, so the test
// compares the storage addresses instead of mutating one view.
func sharesViewStorage(a, b modbusreg.LogicalViewSnapshot) bool {
	storage := func(view modbusreg.LogicalViewSnapshot) (uintptr, uintptr) {
		record := reflect.ValueOf(view).FieldByName("record")
		return record.FieldByName("Words").Pointer(), record.FieldByName("WireResponseBytes").Pointer()
	}
	aWords, aBytes := storage(a)
	bWords, bBytes := storage(b)
	return aWords == bWords || aBytes == bBytes
}

func completedChainSnapshot(t *testing.T, registry DecoderRegistry, occurrences ...Occurrence) ChainSnapshot {
	t.Helper()
	return completedChainSnapshotWithKeys(t, registry.DecoderKeys(), occurrences...)
}

func completedChainSnapshotWithKeys(t *testing.T, decoderKeys []DecoderKey, occurrences ...Occurrence) ChainSnapshot {
	t.Helper()
	const base = uint16(40000)
	raw := rawWordsForOccurrences(occurrences)
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision: testSunSpecModelsRevision,
		BaseCandidates: []uint16{base},
		Limits: ChainLimits{
			MaxTotalWords:  uint32(len(raw)),
			MaxOccurrences: uint32(len(occurrences)),
		},
		DecoderKeys: decoderKeys,
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	logicalViewID := uint64(1000)
	for step := 0; step < len(raw); step++ {
		requests := chain.NextRequests()
		if len(requests) != 1 {
			t.Fatalf("pending requests=%d", len(requests))
		}
		request := requests[0]
		start := int(uint32(request.Address()) - uint32(base))
		end := start + int(request.WordCount())
		if start < 0 || end > len(raw) {
			t.Fatalf("request [%d,%d) exceeds raw words %d", start, end, len(raw))
		}
		snapshot, err := chain.AdmitReplay(request, chainView(t, request, logicalViewID, raw[start:end], "fixture"))
		if err != nil {
			t.Fatal(err)
		}
		logicalViewID++
		if len(snapshot.RawWords()) != 0 {
			return snapshot
		}
	}
	t.Fatal("SunSpec chain did not reach its terminal snapshot")
	return ChainSnapshot{}
}

func rawWordsForOccurrences(occurrences []Occurrence) []uint16 {
	raw := []uint16{sunSpecSignatureFirst, sunSpecSignatureSecond}
	for _, occurrence := range occurrences {
		raw = append(raw, occurrence.Words()...)
	}
	return append(raw, EndModelID, 0)
}

func float32Words(value float32) []uint16 {
	bits := math.Float32bits(value)
	return []uint16{uint16(bits >> 16), uint16(bits)}
}
