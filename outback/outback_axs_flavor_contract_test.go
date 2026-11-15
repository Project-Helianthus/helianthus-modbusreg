package outback

import (
	"reflect"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/internal/sunspecfixture"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
)

func TestOutBackAXSReadOnlyDecoderNeverSelectsUnqualifiedSource(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	decoder, err := NewAXSReadOnlyDecoder()
	if err != nil {
		t.Fatal(err)
	}
	for name, snapshot := range map[string]sunspec.ChainSnapshot{
		"owner manual rev C":     outBackAXSSnapshot(t, registry, outBackAXSOccurrence(t, 64110, 282, 2)),
		"application note rev 5": outBackAXSSnapshot(t, registry, outBackAXSOccurrence(t, 64110, 420, 2)),
		"conflicting revisions":  outBackAXSSnapshot(t, registry, outBackAXSOccurrence(t, 64110, 282, 2), outBackAXSOccurrence(t, 64110, 420, 3)),
		"duplicate interfaces":   outBackAXSSnapshot(t, registry, outBackAXSOccurrence(t, 64110, 282, 2), outBackAXSOccurrence(t, 64110, 282, 3)),
		"wrong length":           outBackAXSSnapshot(t, registry, outBackAXSOccurrence(t, 64110, 281, 2)),
		"missing interface":      outBackAXSSnapshot(t, registry),
		"invalid chain":          {},
	} {
		t.Run(name, func(t *testing.T) {
			before := snapshot.Occurrences()
			for _, d := range []AXSReadOnlyDecoder{decoder, {}} {
				decision := d.EvaluateSnapshot(snapshot)
				if decision.Matched() || decision.Reason() != AXSReadOnlyFlavorReasonSourceNotQualified {
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

func outBackAXSSnapshot(t *testing.T, registry sunspec.DecoderRegistry, vendor ...sunspec.Occurrence) sunspec.ChainSnapshot {
	t.Helper()
	occurrences := []sunspec.Occurrence{sunspecfixture.CommonOccurrence(t, registry, "Maker", "Model", "1.0", 1)}
	occurrences = append(occurrences, vendor...)
	return sunspecfixture.Snapshot(t, registry, occurrences...)
}

// outBackAXSOccurrence returns a zero-payload vendor occurrence, which the
// standard registry leaves an unknown model.
func outBackAXSOccurrence(t *testing.T, modelID, length uint16, ordinal uint32) sunspec.Occurrence {
	t.Helper()
	words := make([]uint16, int(length)+2)
	words[0], words[1] = modelID, length
	occurrence := sunspecfixture.Occurrence(t, sunspecfixture.Registry(t), words, ordinal)
	if occurrence.Disposition != sunspec.ChainDispositionUnknownModel {
		t.Fatalf("vendor occurrence %d/%d disposition=%q", modelID, length, occurrence.Disposition)
	}
	return occurrence
}
