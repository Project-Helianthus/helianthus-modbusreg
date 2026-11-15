package detection

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// FixtureReplayer validates offline evidence without creating a runtime
// capability, production attempt, seal, publication, or sample identity.
type FixtureReplayer struct {
	profile modbusreg.ProfileDescriptor
}

// NewFixtureReplayer binds offline validation to one immutable profile.
func NewFixtureReplayer(profile modbusreg.ProfileDescriptor) (*FixtureReplayer, error) {
	copy, err := modbusreg.NewProfileDescriptor(profile.Spec())
	if err != nil {
		return nil, fmt.Errorf("fixture replay profile: %w", err)
	}
	if copy.Kind() != modbusreg.ProfileStandardFamily {
		return nil, fmt.Errorf("vendor overlay requires M3 resolution")
	}
	return &FixtureReplayer{profile: copy}, nil
}

// FixtureReplay is a nonpublishable evidence result. It intentionally has no
// capability, claim, seal, publish, or production SampleID method.
type FixtureReplay struct {
	fixtureID string
	spec      modbusreg.ObservationSpec
	replayed  []modbusreg.ReplayedDependency
}

// Replay strictly decodes and validates one bounded fixture record.
func (replayer *FixtureReplayer) Replay(data []byte) (FixtureReplay, error) {
	if replayer == nil || replayer.profile.ID() == "" {
		return FixtureReplay{}, fmt.Errorf("fixture replayer is invalid")
	}
	var spec modbusreg.ObservationSpec
	if err := spec.UnmarshalJSON(data); err != nil {
		return FixtureReplay{}, err
	}
	if spec.SampleID != "" {
		return FixtureReplay{}, fmt.Errorf("fixture cannot contain a production sample ID")
	}
	validated, err := modbusreg.NewObservation(replayer.profile, spec)
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
func (replay FixtureReplay) Spec() modbusreg.ObservationSpec {
	return modbusreg.CloneObservationSpec(replay.spec)
}

// Replay returns independent validated fixture dependencies.
func (replay FixtureReplay) Replay() []modbusreg.ReplayedDependency {
	result := make([]modbusreg.ReplayedDependency, len(replay.replayed))
	copy(result, replay.replayed)
	return result
}
