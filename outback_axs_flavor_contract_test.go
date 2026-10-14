package modbusreg

import (
	"reflect"
	"testing"
)

func TestOutBackAXSReadOnlyDecoderNeverSelectsUnqualifiedSource(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	decoder, err := NewOutBackAXSReadOnlyDecoder()
	if err != nil {
		t.Fatal(err)
	}
	for name, snapshot := range map[string]SunSpecChainSnapshot{
		"owner manual rev C":     outBackAXSSnapshot(t, registry, outBackAXSOccurrence(64110, 282, 2)),
		"application note rev 5": outBackAXSSnapshot(t, registry, outBackAXSOccurrence(64110, 420, 2)),
		"conflicting revisions":  outBackAXSSnapshot(t, registry, outBackAXSOccurrence(64110, 282, 2), outBackAXSOccurrence(64110, 420, 3)),
		"duplicate interfaces":   outBackAXSSnapshot(t, registry, outBackAXSOccurrence(64110, 282, 2), outBackAXSOccurrence(64110, 282, 3)),
		"wrong length":           outBackAXSSnapshot(t, registry, outBackAXSOccurrence(64110, 281, 2)),
		"missing interface":      outBackAXSSnapshot(t, registry),
		"invalid chain":          {},
	} {
		t.Run(name, func(t *testing.T) {
			before := snapshot.Occurrences()
			for _, d := range []OutBackAXSReadOnlyDecoder{decoder, {}} {
				decision := d.EvaluateSnapshot(snapshot)
				if decision.Matched() || decision.Reason() != OutBackAXSReadOnlyFlavorReasonSourceNotQualified {
					t.Fatal("unqualified source selected an OutBack profile")
				}
				if _, ok := decision.InterfaceModel(); ok {
					t.Fatal("rejected candidate emitted a typed model")
				}
				if _, ok := decision.InterfaceOccurrence(); ok {
					t.Fatal("rejected candidate exposed a selected interface")
				}
			}
			if !reflect.DeepEqual(before, snapshot.Occurrences()) {
				t.Fatal("rejection changed raw snapshot evidence")
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
