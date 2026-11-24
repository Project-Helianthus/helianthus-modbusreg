package sunspec

import (
	"fmt"
	"reflect"
	"testing"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// PollGeneration and DeadlineIdentity are varied independently, including a
// swap of the two values. Reading one field into the other, or formatting the
// sample identifier from the opposite order, fails one of the cases.
func TestObservationAdmitsSyntheticChain(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	for _, identity := range []ObservationSampleIdentity{{pollGeneration: 11, deadlineIdentity: 22}, {pollGeneration: 22, deadlineIdentity: 11}} {
		t.Run(fmt.Sprintf("poll-%d-deadline-%d", identity.pollGeneration, identity.deadlineIdentity), func(t *testing.T) {
			snapshot := observationSnapshotWithIdentity(t, syntheticAdmittedSnapshot(t, registry), identity.pollGeneration, identity.deadlineIdentity)
			observation, err := NewObservation(registry, snapshot)
			if err != nil {
				t.Fatalf("NewObservation: %v", err)
			}
			if !observation.Capability().Admitted() || observation.Capability().Reason() != CapabilityReasonAdmitted || observation.Capability().ProfileID() != ThreePhaseMonitoringCapabilityID || len(observation.Capability().Facts()) < len(ThreePhaseMonitoringFieldIDs()) {
				t.Fatalf("capability=%#v facts=%d", observation.Capability(), len(observation.Capability().Facts()))
			}
			wantChain := []WireKey{{ModelID: 1, ModelLength: 65}, {ModelID: 113, ModelLength: 60}, {ModelID: EndModelID, ModelLength: 0}}
			if got := observation.Chain(); !reflect.DeepEqual(got, wantChain) {
				t.Fatalf("chain=%#v", got)
			}
			if observation.SampleIdentity() != identity || observation.SampleID() != fmt.Sprintf("sunspec-%d-%d", identity.pollGeneration, identity.deadlineIdentity) {
				t.Fatalf("sample id=%q identity=%#v", observation.SampleID(), observation.SampleIdentity())
			}
			if len(observation.Occurrences()) != 2 || len(observation.SourceViews()) == 0 || len(observation.RawWords()) != len(snapshot.RawWords()) {
				t.Fatalf("occurrences=%d views=%d raw=%d", len(observation.Occurrences()), len(observation.SourceViews()), len(observation.RawWords()))
			}
			for index, view := range observation.SourceViews() {
				record := view.Record()
				if record.PollGeneration != identity.pollGeneration || record.DeadlineIdentity != identity.deadlineIdentity {
					t.Fatalf("source view %d identity poll=%d deadline=%d", index, record.PollGeneration, record.DeadlineIdentity)
				}
			}
			chain, raw := observation.Chain(), observation.RawWords()
			chain[0], raw[0] = WireKey{}, 0
			occurrences := observation.Occurrences()
			occurrences[0].WireKey = WireKey{}
			words := occurrences[0].Words()
			words[0] = 0
			record := observation.SourceViews()[0].Record()
			record.Words[0] = 0
			if observation.Chain()[0] != (WireKey{ModelID: 1, ModelLength: 65}) || observation.RawWords()[0] != snapshot.RawWords()[0] || observation.Occurrences()[0].WireKey != (WireKey{ModelID: 1, ModelLength: 65}) || observation.Occurrences()[0].Words()[0] == 0 || observation.SourceViews()[0].Record().Words[0] == 0 {
				t.Fatal("observation getters leaked caller mutation")
			}
			validated, gotIdentity, err := ValidateObservationSnapshot(observation.Snapshot())
			if err != nil || gotIdentity != identity || !reflect.DeepEqual(validated.RawWords(), observation.RawWords()) {
				t.Fatalf("ValidateObservationSnapshot: %v identity=%#v", err, gotIdentity)
			}
		})
	}
}

func TestObservationRejectsChainThatIsNotAdmitted(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	admitted := syntheticAdmittedSnapshot(t, registry)
	if _, err := NewObservation(registry, ChainSnapshot{}); err == nil {
		t.Fatal("zero snapshot produced an observation")
	}
	if _, err := NewObservation(registry, completedChainSnapshot(t, registry, commonOccurrence(t, registry, "Synthetic", "Public", "1.0", 1))); err == nil {
		t.Fatal("chain without a three-phase source produced an observation")
	}
	mixed := observationSnapshotWithIdentity(t, admitted, 11, 22)
	views := mixed.SourceViews()
	record := views[1].Record()
	record.DeadlineIdentity++
	replacement, err := modbusreg.NewLogicalViewSnapshot(record)
	if err != nil {
		t.Fatal(err)
	}
	views[1] = replacement
	rebuilt, err := NewChainSnapshot(mixed.RawWords(), mixed.Occurrences(), views)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewObservation(registry, rebuilt); err == nil {
		t.Fatal("mixed sample identity produced an observation")
	}
	if _, _, err := ValidateObservationSnapshot(ChainSnapshot{}); err == nil {
		t.Fatal("zero snapshot validated")
	}
}

func syntheticAdmittedSnapshot(t *testing.T, registry DecoderRegistry) ChainSnapshot {
	t.Helper()
	return completedChainSnapshot(t, registry,
		commonOccurrence(t, registry, "Synthetic", "Public", "1.0", 1),
		inverterOccurrence(t, registry, 113, nil, 2),
	)
}

func observationSnapshotWithIdentity(t *testing.T, snapshot ChainSnapshot, poll, deadline uint64) ChainSnapshot {
	t.Helper()
	views := snapshot.SourceViews()
	sources := make([]modbusreg.LogicalViewSnapshot, len(views))
	for index, view := range views {
		record := view.Record()
		record.PollGeneration = poll
		record.DeadlineIdentity = deadline
		validated, err := modbusreg.NewLogicalViewSnapshot(record)
		if err != nil {
			t.Fatal(err)
		}
		sources[index] = validated
	}
	rebuilt, err := NewChainSnapshot(snapshot.RawWords(), snapshot.Occurrences(), sources)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rebuilt.WireChain(); !ok {
		t.Fatal("identity rewrite broke the terminal chain")
	}
	return rebuilt
}
