package modbusreg_test

import (
	"testing"
	"time"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func singleDependencyProfile(
	t *testing.T,
) reg.ProfileDescriptor {
	t.Helper()
	base := profileFixture(t)
	dependencies := base.Dependencies().Dependencies()
	set, err := reg.NewDependencySet(
		base.Dependencies().Version(),
		dependencies[:1],
	)
	if err != nil {
		t.Fatalf("NewDependencySet(single): %v", err)
	}
	spec := base.Spec()
	spec.Dependencies = set
	profile, err := reg.NewProfileDescriptor(spec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(single): %v", err)
	}
	return profile
}

func TestUTCRangeValidationPrecedesCAS(t *testing.T) {
	profile := profileFixture(t)
	cases := []struct {
		name   string
		issuer string
		mutate func(*reg.ObservationSpec)
	}{
		{
			name:   "lower bound crosses into UTC year zero",
			issuer: "fixture-time-lower",
			mutate: func(spec *reg.ObservationSpec) {
				location := time.FixedZone("UTC+14", 14*60*60)
				spec.SourceTime = reg.SourceTimeObserved(
					time.Date(1, time.January, 1, 0, 0, 0, 0, location),
				)
			},
		},
		{
			name:   "upper bound crosses into UTC year 10000",
			issuer: "fixture-time-upper",
			mutate: func(spec *reg.ObservationSpec) {
				location := time.FixedZone("UTC-14", -14*60*60)
				spec.LocalReceiptTime = time.Date(
					9999,
					time.December,
					31,
					23,
					59,
					59,
					999_999_999,
					location,
				)
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			spec := successfulObservationSpec(t, profile)
			test.mutate(&spec)
			initial := fixtureLedgerState(t, profile, test.issuer)
			factory := fixtureFactoryFromState(t, profile, initial)
			if _, err := reg.MarshalFixtureSpec(spec); err == nil {
				t.Fatal("out-of-range UTC timestamp serialized")
			}

			publishSpec := successfulObservationSpec(t, profile)
			test.mutate(&publishSpec)
			observation, err := replayWithFactory(factory, publishSpec)
			if err == nil || observation.FixtureID() != "" {
				t.Fatal("out-of-range UTC timestamp published")
			}
		})
	}
}

func TestRTUPhysicalResponseHasOneLogicalViewInBoundedMode(
	t *testing.T,
) {
	base := profileFixture(t)
	dependencies := base.Dependencies().Dependencies()
	secondSpec := dependencies[1].Spec()
	secondSpec.Normalization = normalizationSpec(t, 101)
	second, err := reg.NewDependency(secondSpec)
	if err != nil {
		t.Fatalf("NewDependency(second): %v", err)
	}
	set, err := reg.NewDependencySet(
		base.Dependencies().Version(),
		[]reg.Dependency{dependencies[0], second},
	)
	if err != nil {
		t.Fatalf("NewDependencySet: %v", err)
	}
	profileSpec := base.Spec()
	profileSpec.Dependencies = set
	profileSpec.Coherence = reg.CoherencePolicySpec{
		Version:                      base.CoherenceVersion(),
		Mode:                         reg.CoherenceBoundedMultiResponse,
		MaximumSourceSkew:            2 * time.Second,
		MaximumReceiptSkew:           3 * time.Second,
		RequireGenerationEquality:    true,
		AcquisitionOrder:             reg.AcquisitionOrderDependencyDeclaration,
		RetrySetBehavior:             reg.RetryWholeSet,
		DocumentaryConsistencyMarker: "sequence-7",
	}
	profile, err := reg.NewProfileDescriptor(profileSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor: %v", err)
	}
	spec := successfulObservationSpec(t, profile)
	spec.RetryOrdinal = 1
	source := time.Unix(1_700_000_100, 0).UTC()
	for index := range spec.Dependencies {
		record := spec.Dependencies[index].View.Record()
		record.Transport = reg.TransportRTU
		record.ConnectionID = 0
		record.WireResponseID = 77
		record.PhysicalRequestID = 55
		record.PhysicalOffset = 100
		record.PhysicalWordCount = 2
		record.LogicalOffset = 100
		record.LogicalWordCount = 2
		record.SliceOffset = 0
		record.SliceWordCount = 2
		record.Words = []uint16{0x0102, 0x0304}
		spec.Dependencies[index].View = snapshotFromRecord(t, record)
		spec.Dependencies[index].SourceTime = reg.SourceTimeObserved(
			source.Add(time.Duration(index) * time.Second),
		)
		spec.Dependencies[index].LocalReceiptTime = source.Add(
			time.Duration(index+1) * time.Second,
		)
		spec.Dependencies[index].DocumentaryConsistencyMarker = "sequence-7"
		spec.Dependencies[index].AcquisitionOrdinal = uint32(index + 1)
		spec.Dependencies[index].RetryOrdinal = 1
	}
	spec.SourceTime = reg.SourceTimeObserved(source.Add(time.Second))
	spec.LocalReceiptTime = source.Add(2 * time.Second)
	if _, err := validateFixtureReplay(t, profile, spec); err == nil {
		t.Fatal("bounded coherence admitted two RTU views from one response")
	}
}

func TestTCPDisjointProfileIsRejectedByM1(t *testing.T) {
	base := profileFixture(t)
	dependencies := base.Dependencies().Dependencies()
	secondSpec := dependencies[1].Spec()
	secondSpec.Normalization = normalizationSpec(t, 105)
	second, err := reg.NewDependency(secondSpec)
	if err != nil {
		t.Fatalf("NewDependency(second): %v", err)
	}
	set, err := reg.NewDependencySet(
		base.Dependencies().Version(),
		[]reg.Dependency{dependencies[0], second},
	)
	if err != nil {
		t.Fatalf("NewDependencySet: %v", err)
	}
	profileSpec := base.Spec()
	profileSpec.Dependencies = set
	if _, err := reg.NewProfileDescriptor(profileSpec); err == nil {
		t.Fatal("M1 single-wire profile accepted disjoint logical ranges")
	}
}
