package modbusreg

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// FixtureReplayer validates offline evidence without creating a runtime
// capability, production attempt, seal, publication, or sample identity.
type FixtureReplayer struct {
	profile ProfileDescriptor
}

// NewFixtureReplayer binds offline validation to one immutable profile.
func NewFixtureReplayer(profile ProfileDescriptor) (*FixtureReplayer, error) {
	copy, err := NewProfileDescriptor(profile.Spec())
	if err != nil {
		return nil, fmt.Errorf("fixture replay profile: %w", err)
	}
	if copy.spec.Kind != ProfileStandardFamily {
		return nil, fmt.Errorf("vendor overlay requires M3 resolution")
	}
	return &FixtureReplayer{profile: copy}, nil
}

// FixtureReplay is a nonpublishable evidence result. It intentionally has no
// capability, claim, seal, publish, or production SampleID method.
type FixtureReplay struct {
	fixtureID string
	spec      ObservationSpec
	replayed  []ReplayedDependency
}

// Replay strictly decodes and validates one bounded fixture record.
func (replayer *FixtureReplayer) Replay(data []byte) (FixtureReplay, error) {
	if replayer == nil || replayer.profile.ID() == "" {
		return FixtureReplay{}, fmt.Errorf("fixture replayer is invalid")
	}
	var record observationDTO
	if err := decodeStrict(data, &record); err != nil {
		return FixtureReplay{}, err
	}
	spec, err := observationSpecFromDTO(record)
	if err != nil {
		return FixtureReplay{}, err
	}
	if spec.SampleID != "" {
		return FixtureReplay{}, fmt.Errorf("fixture cannot contain a production sample ID")
	}
	validated, err := buildObservation(replayer.profile, spec)
	if err != nil {
		return FixtureReplay{}, err
	}
	digest := sha256.Sum256(append(
		[]byte("helianthus.modbusreg.fixture-replay/v1\x00"),
		data...,
	))
	return FixtureReplay{
		fixtureID: "fixture:sha256:" + hex.EncodeToString(digest[:]),
		spec:      validated.Spec(),
		replayed:  validated.Replay(),
	}, nil
}

// FixtureID returns the evidence-scoped content identity.
func (replay FixtureReplay) FixtureID() string {
	return replay.fixtureID
}

// Spec returns an independent fixture envelope with no production sample ID.
func (replay FixtureReplay) Spec() ObservationSpec {
	return cloneObservationSpec(replay.spec)
}

// Replay returns independent validated fixture dependencies.
func (replay FixtureReplay) Replay() []ReplayedDependency {
	result := make([]ReplayedDependency, len(replay.replayed))
	copy(result, replay.replayed)
	return result
}
