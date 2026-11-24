package sunspec

import (
	"fmt"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// ObservationSampleIdentity is the capture identity shared by every retained
// source view of one observation. Neither field is zero for a constructed
// observation.
type ObservationSampleIdentity struct {
	pollGeneration   uint64
	deadlineIdentity uint64
}

// PollGeneration returns the poll generation shared by every source view.
func (identity ObservationSampleIdentity) PollGeneration() uint64 {
	return identity.pollGeneration
}

// DeadlineIdentity returns the deadline identity shared by every source view.
func (identity ObservationSampleIdentity) DeadlineIdentity() uint64 {
	return identity.deadlineIdentity
}

// Observation is the vendor-neutral evidence record for one chain whose
// monitoring capability is admitted. It retains the sample identity, raw
// words, occurrences, source views, terminal chain and capability decision.
// It carries no vendor flavor.
type Observation struct {
	capability CapabilityDecision
	chain      []WireKey
	sampleID   string
	identity   ObservationSampleIdentity
	snapshot   ChainSnapshot
}

// NewObservation builds an observation from a chain the three-phase monitoring
// capability admits. The snapshot must be a bounded terminal chain with one
// non-zero sample identity. Any other snapshot returns an error and no
// observation. The caller supplies no verdict and no sample identity.
func NewObservation(registry DecoderRegistry, input ChainSnapshot) (Observation, error) {
	snapshot, identity, err := ValidateObservationSnapshot(input)
	if err != nil {
		return Observation{}, err
	}
	chain, valid := snapshot.WireChain()
	if !valid {
		return Observation{}, fmt.Errorf("SunSpec observation chain is not terminal verified")
	}
	capability := registry.EvaluateThreePhaseMonitoring(snapshot)
	if !capability.Admitted() || capability.Reason() != CapabilityReasonAdmitted {
		return Observation{}, fmt.Errorf("SunSpec observation capability is not admitted")
	}
	return Observation{
		capability: capability.Clone(),
		chain:      append([]WireKey(nil), chain...),
		sampleID:   fmt.Sprintf("sunspec-%d-%d", identity.pollGeneration, identity.deadlineIdentity),
		identity:   identity,
		snapshot:   snapshot.Clone(),
	}, nil
}

// Capability returns a copy of the admitted monitoring decision.
func (observation Observation) Capability() CapabilityDecision {
	return observation.capability.Clone()
}

// Chain returns a copy of the terminal wire chain, end marker included.
func (observation Observation) Chain() []WireKey {
	return append([]WireKey(nil), observation.chain...)
}

// SampleID returns the sample identifier derived from the sample identity, in
// the form sunspec-<poll generation>-<deadline identity>.
func (observation Observation) SampleID() string { return observation.sampleID }

// SampleIdentity returns the capture identity shared by the source views.
func (observation Observation) SampleIdentity() ObservationSampleIdentity {
	return observation.identity
}

// RawWords returns a copy of the retained raw chain image.
func (observation Observation) RawWords() []uint16 {
	return observation.snapshot.RawWords()
}

// Occurrences returns copies of the retained occurrences in chain order.
func (observation Observation) Occurrences() []Occurrence {
	return observation.snapshot.Occurrences()
}

// SourceViews returns copies of the retained source views in admission order.
func (observation Observation) SourceViews() []modbusreg.LogicalViewSnapshot {
	return observation.snapshot.SourceViews()
}

// Snapshot returns a detached copy of the retained chain snapshot.
func (observation Observation) Snapshot() ChainSnapshot {
	return observation.snapshot.Clone()
}

// ValidateObservationSnapshot checks the observation bounds, one non-zero
// sample identity and a terminal chain. It returns detached copies and does
// not evaluate a capability.
func ValidateObservationSnapshot(input ChainSnapshot) (ChainSnapshot, ObservationSampleIdentity, error) {
	if input.RawWordCount() < 4 || input.RawWordCount() > MaxSunSpecPhaseOneChainWords || input.OccurrenceCount() == 0 || input.OccurrenceCount() > MaxSunSpecPhaseOneChainWords/2 || input.SourceViewCount() == 0 || input.SourceViewCount() > MaxSunSpecPhaseOneChainWords {
		return ChainSnapshot{}, ObservationSampleIdentity{}, fmt.Errorf("SunSpec observation bounds are invalid")
	}
	inputViews := input.SourceViews()
	sources := make([]modbusreg.LogicalViewSnapshot, len(inputViews))
	first := inputViews[0].Record()
	if first.PollGeneration == 0 || first.DeadlineIdentity == 0 {
		return ChainSnapshot{}, ObservationSampleIdentity{}, fmt.Errorf("SunSpec observation sample identity is invalid")
	}
	for index, view := range inputViews {
		validated, err := modbusreg.NewLogicalViewSnapshot(view.Record())
		if err != nil {
			return ChainSnapshot{}, ObservationSampleIdentity{}, fmt.Errorf("SunSpec observation source view %d: %w", index, err)
		}
		record := validated.Record()
		if !SameProvenance(first, record) || record.PollGeneration == 0 || record.DeadlineIdentity == 0 {
			return ChainSnapshot{}, ObservationSampleIdentity{}, fmt.Errorf("SunSpec observation source provenance is mixed")
		}
		sources[index] = validated
	}
	snapshot, err := NewChainSnapshot(input.RawWords(), input.Occurrences(), sources)
	if err != nil {
		return ChainSnapshot{}, ObservationSampleIdentity{}, fmt.Errorf("SunSpec observation retained chain is invalid")
	}
	if _, valid := snapshot.WireChain(); !valid {
		return ChainSnapshot{}, ObservationSampleIdentity{}, fmt.Errorf("SunSpec observation retained chain is invalid")
	}
	return snapshot, ObservationSampleIdentity{pollGeneration: first.PollGeneration, deadlineIdentity: first.DeadlineIdentity}, nil
}
