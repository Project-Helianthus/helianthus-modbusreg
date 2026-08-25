package modbusreg

import "testing"

func TestOutBackAXSReadOnlyDecoderSelectsOneExplicitExactInterface(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	decoder, err := NewOutBackAXSReadOnlyDecoder()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := outBackAXSSnapshot(t, registry, outBackAXSOccurrence(OutBackAXSModelInterface, OutBackAXSInterfaceModelWords, 2))

	decision := decoder.EvaluateSnapshot(snapshot)
	if !decision.Matched() || decision.Reason() != OutBackAXSReadOnlyFlavorReasonMatched {
		t.Fatalf("matched=%t reason=%q", decision.Matched(), decision.Reason())
	}
	occurrence, ok := decision.InterfaceOccurrence()
	if !ok || occurrence.WireKey != (SunSpecWireKey{ModelID: OutBackAXSModelInterface, ModelLength: OutBackAXSInterfaceModelWords}) {
		t.Fatalf("interface occurrence=%#v found=%t", occurrence, ok)
	}
	if chain := decision.Chain(); len(chain) != 3 || chain[1] != occurrence.WireKey {
		t.Fatalf("selected chain=%#v", chain)
	}
	model, ok := decision.InterfaceModel()
	if !ok || model.Key() != (SunSpecDecoderKey{ModelID: OutBackAXSModelInterface, ModelLength: OutBackAXSInterfaceModelWords, SchemaRevision: SunSpecModelsRevisionV1}) {
		t.Fatalf("interface model=%#v found=%t", model, ok)
	}
	model.raw[2] = 99
	again, ok := decision.InterfaceModel()
	if !ok || again.RawWords()[2] != 0 {
		t.Fatal("interface model aliases decision state")
	}
	if _, standard := registry.definition(SunSpecDecoderKey{ModelID: OutBackAXSModelInterface, ModelLength: OutBackAXSInterfaceModelWords, SchemaRevision: SunSpecModelsRevisionV1}); standard {
		t.Fatal("explicit vendor selection extended the standard registry")
	}
}

func TestOutBackAXSReadOnlyDecoderFailsClosedOnInvalidOrAmbiguousSnapshot(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	decoder, err := NewOutBackAXSReadOnlyDecoder()
	if err != nil {
		t.Fatal(err)
	}
	validCommon := commonOccurrence(t, registry, "Maker", "Model", "1.0", 1)
	invalidCommon := cloneOccurrence(validCommon)
	invalidCommon.words[2] = 0

	for name, tc := range map[string]struct {
		snapshot SunSpecChainSnapshot
		reason   OutBackAXSReadOnlyFlavorReason
	}{
		"missing interface": {
			snapshot: outBackAXSSnapshot(t, registry),
			reason:   OutBackAXSReadOnlyFlavorReasonInterfaceAbsent,
		},
		"wrong interface length": {
			snapshot: outBackAXSSnapshot(t, registry, outBackAXSOccurrence(OutBackAXSModelInterface, OutBackAXSInterfaceModelWords-1, 2)),
			reason:   OutBackAXSReadOnlyFlavorReasonInterfaceWrongLength,
		},
		"duplicate interfaces": {
			snapshot: outBackAXSSnapshot(t, registry,
				outBackAXSOccurrence(OutBackAXSModelInterface, OutBackAXSInterfaceModelWords, 2),
				outBackAXSOccurrence(OutBackAXSModelInterface, OutBackAXSInterfaceModelWords, 3),
			),
			reason: OutBackAXSReadOnlyFlavorReasonInterfaceAmbiguous,
		},
		"invalid common": {
			snapshot: snapshotFromOccurrences(invalidCommon, outBackAXSOccurrence(OutBackAXSModelInterface, OutBackAXSInterfaceModelWords, 2)),
			reason:   OutBackAXSReadOnlyFlavorReasonCommonNotAdmitted,
		},
	} {
		t.Run(name, func(t *testing.T) {
			decision := decoder.EvaluateSnapshot(tc.snapshot)
			if decision.Matched() || decision.Reason() != tc.reason {
				t.Fatalf("matched=%t reason=%q want=%q", decision.Matched(), decision.Reason(), tc.reason)
			}
			if _, ok := decision.InterfaceOccurrence(); ok {
				t.Fatal("rejected decision exposed vendor occurrence")
			}
		})
	}
}

func outBackAXSSnapshot(t *testing.T, registry SunSpecDecoderRegistry, vendor ...SunSpecOccurrence) SunSpecChainSnapshot {
	t.Helper()
	occurrences := []SunSpecOccurrence{commonOccurrence(t, registry, "Maker", "Model", "1.0", 1)}
	occurrences = append(occurrences, vendor...)
	return snapshotFromOccurrences(occurrences...)
}

func outBackAXSOccurrence(modelID, length uint16, ordinal uint32) SunSpecOccurrence {
	words := make([]uint16, int(length)+2)
	words[0], words[1] = modelID, length
	return SunSpecOccurrence{
		Ordinal:        ordinal,
		WireKey:        SunSpecWireKey{ModelID: modelID, ModelLength: length},
		SchemaRevision: testSunSpecModelsRevision,
		Disposition:    SunSpecChainDispositionUnknownModel,
		words:          words,
	}
}
