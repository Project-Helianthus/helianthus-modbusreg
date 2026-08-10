package modbusreg_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func round6BoundedSingleProfile(t *testing.T) reg.ProfileDescriptor {
	t.Helper()
	base := round5SingleDependencyProfile(t)
	spec := base.Spec()
	spec.Coherence = reg.CoherencePolicySpec{
		Version:                      base.CoherenceVersion(),
		Mode:                         reg.CoherenceBoundedMultiResponse,
		MaximumSourceSkew:            time.Second,
		MaximumReceiptSkew:           time.Second,
		RequireGenerationEquality:    true,
		AcquisitionOrder:             reg.AcquisitionOrderDependencyDeclaration,
		RetrySetBehavior:             reg.RetryWholeSet,
		DocumentaryConsistencyMarker: "round6-sequence",
	}
	profile, err := reg.NewProfileDescriptor(spec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(bounded single): %v", err)
	}
	return profile
}

func TestRound6M1CommonIntersectionParity(t *testing.T) {
	base := profileFixture(t)
	if _, err := reg.NewProfileDescriptor(base.Spec()); err != nil {
		t.Fatalf("overlapping M1 profile rejected: %v", err)
	}

	dependencies := base.Dependencies().Dependencies()
	adjacentSpec := dependencies[1].Spec()
	adjacentSpec.Normalization = normalizationSpec(t, 103)
	adjacent, err := reg.NewDependency(adjacentSpec)
	if err != nil {
		t.Fatalf("NewDependency(adjacent): %v", err)
	}
	set, err := reg.NewDependencySet(
		base.Dependencies().Version(),
		[]reg.Dependency{dependencies[0], adjacent},
	)
	if err != nil {
		t.Fatalf("NewDependencySet(adjacent): %v", err)
	}
	profileSpec := base.Spec()
	profileSpec.Dependencies = set
	if _, err := reg.NewProfileDescriptor(profileSpec); err == nil {
		t.Fatal("single-wire profile accepted max-start equal to min-end")
	}

	profileSpec.Coherence = reg.CoherencePolicySpec{
		Version:                      base.CoherenceVersion(),
		Mode:                         reg.CoherenceBoundedMultiResponse,
		MaximumSourceSkew:            2 * time.Second,
		MaximumReceiptSkew:           2 * time.Second,
		RequireGenerationEquality:    true,
		AcquisitionOrder:             reg.AcquisitionOrderDependencyDeclaration,
		RetrySetBehavior:             reg.RetryWholeSet,
		DocumentaryConsistencyMarker: "round6-intersection",
	}
	bounded, err := reg.NewProfileDescriptor(profileSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(bounded adjacent): %v", err)
	}
	observation := successfulObservationSpec(t, bounded)
	observation.RetryOrdinal = 1
	source := time.Unix(1_700_000_300, 0).UTC()
	for index := range observation.Dependencies {
		record := observation.Dependencies[index].View.Record()
		record.PhysicalOffset = 100
		record.PhysicalWordCount = 4
		record.WireResponseID = 77
		record.PhysicalRequestID = 55
		record.LogicalOffset = uint16(100 + index*2)
		record.SliceOffset = uint16(index * 2)
		record.Words = []uint16{uint16(index*2 + 1), uint16(index*2 + 2)}
		observation.Dependencies[index].View = snapshotFromRecord(t, record)
		observation.Dependencies[index].SourceTime =
			reg.SourceTimeObserved(source)
		observation.Dependencies[index].LocalReceiptTime = source
		observation.Dependencies[index].DocumentaryConsistencyMarker =
			"round6-intersection"
		observation.Dependencies[index].AcquisitionOrdinal = uint32(index + 1)
		observation.Dependencies[index].RetryOrdinal = 1
	}
	observation.SourceTime = reg.SourceTimeObserved(source)
	observation.LocalReceiptTime = source
	if _, err := round3Publish(t, bounded, observation); err == nil {
		t.Fatal("one TCP physical response accepted views without common intersection")
	}
}

func TestRound6PersistedAttemptMustMatchProfileMode(t *testing.T) {
	tests := []struct {
		name    string
		profile reg.ProfileDescriptor
		attempt reg.AttemptIdentity
	}{
		{
			name:    "single wire cannot restore retry ordinal",
			profile: profileFixture(t),
			attempt: reg.AttemptIdentity{PollGenerationID: 41, RetryOrdinal: 1},
		},
		{
			name:    "bounded mode requires retry ordinal",
			profile: round6BoundedSingleProfile(t),
			attempt: reg.AttemptIdentity{PollGenerationID: 41},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := round3State(t, test.profile, "round6-mode-state")
			state.Revision = 1
			state.HighWater = 1
			state.LastCommittedAttempt = test.attempt
			ledger, err := reg.NewSampleLedger(state, 1)
			if err != nil {
				t.Fatalf("NewSampleLedger: %v", err)
			}
			if _, err := reg.NewObservationFactory(
				test.profile,
				ledger,
				&memoryPublicationCommitter{},
			); err == nil {
				t.Fatal("factory accepted mode-incompatible persisted attempt")
			}
		})
	}
}

func TestRound6CoherenceMarkerIsBoundedBeforeConstruction(t *testing.T) {
	base, _ := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	spec := base.Spec()
	spec.Coherence.DocumentaryConsistencyMarker = strings.Repeat(
		"x",
		reg.MaxContractStringBytes+1,
	)
	if _, err := reg.NewProfileDescriptor(spec); err == nil {
		t.Fatal("4097-byte coherence marker was admitted")
	}

	spec = base.Spec()
	spec.Coherence.DocumentaryConsistencyMarker = strings.Repeat(
		"x",
		reg.MaxContractStringBytes,
	)
	maximum, err := reg.NewProfileDescriptor(spec)
	if err != nil {
		t.Fatalf("maximum coherence marker rejected: %v", err)
	}
	encoded, err := reg.MarshalProfileDescriptor(maximum)
	if err != nil {
		t.Fatalf("MarshalProfileDescriptor(maximum marker): %v", err)
	}
	decoded, err := reg.UnmarshalProfileDescriptor(encoded)
	if err != nil {
		t.Fatalf("UnmarshalProfileDescriptor(maximum marker): %v", err)
	}
	reencoded, err := reg.MarshalProfileDescriptor(decoded)
	if err != nil {
		t.Fatalf("MarshalProfileDescriptor(round trip): %v", err)
	}
	if !reflect.DeepEqual(encoded, reencoded) {
		t.Fatal("maximum coherence marker is not byte-stable")
	}
}

func TestRound6ExplicitObservedUTCYearOneRoundTrips(t *testing.T) {
	profile := profileFixture(t)
	spec := successfulObservationSpec(t, profile)
	spec.SourceTime = reg.SourceTimeObserved(time.Time{})
	encoded, err := reg.MarshalFixtureSpec(spec)
	if err != nil {
		t.Fatalf("MarshalFixtureSpec(year one): %v", err)
	}
	state := round3State(t, profile, "round6-year-one")
	factory := round3Factory(
		t,
		profile,
		state,
		&round3MemoryCAS{state: state},
	)
	attempt, err := factory.BeginObservationAttempt(reg.AttemptIdentity{
		PollGenerationID: spec.PollGenerationID,
	})
	if err != nil {
		t.Fatalf("BeginObservationAttempt: %v", err)
	}
	decoded, err := attempt.DecodeSpec(encoded)
	if err != nil {
		t.Fatalf("DecodeSpec(year one): %v", err)
	}
	observation, err := attempt.Publish(decoded)
	if err != nil {
		t.Fatalf("Publish(year one): %v", err)
	}
	if got := observation.Spec().SourceTime; got.State != reg.SourceTimeObservedState ||
		!got.Time.Equal(time.Time{}) {
		t.Fatalf("year-one source time = %#v", got)
	}
	serialized, err := reg.MarshalFixtureSpec(observation.Spec())
	if err != nil {
		t.Fatalf("MarshalObservation(year one): %v", err)
	}
	if !strings.Contains(string(serialized), "0001-01-01T00:00:00Z") {
		t.Fatal("serialized observation lost explicit UTC year one")
	}
}

func TestRound6MissingJSONFieldErrorIsDeterministic(t *testing.T) {
	const expected = `serialized object is missing required key "schema_version"`
	for iteration := 0; iteration < 100; iteration++ {
		_, err := reg.UnmarshalSampleLedgerState([]byte(`{}`))
		if err == nil || err.Error() != expected {
			t.Fatalf("iteration %d missing-key error = %v", iteration, err)
		}
	}
}

func TestRound7FixturePublishUsesDecodedImmutableSnapshot(t *testing.T) {
	profile, spec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	encoded, err := reg.MarshalFixtureSpec(spec)
	if err != nil {
		t.Fatalf("MarshalFixtureSpec: %v", err)
	}
	replayer, err := reg.NewFixtureReplayer(profile)
	if err != nil {
		t.Fatalf("NewFixtureReplayer: %v", err)
	}
	replay, err := replayer.Replay(encoded)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	decoded := replay.Spec()
	admitted := replay.Replay()[0].LogicalViewRecord()
	mutated := admitted
	mutated.LogicalViewID += 100
	mutated.WireResponseID += 100
	mutated.PhysicalRequestID += 100
	mutated.Words = []uint16{0x9999, 0x8888}
	decoded.Dependencies[0].View = snapshotFromRecord(t, mutated)

	if got := replay.Replay()[0].LogicalViewRecord(); !reflect.DeepEqual(got, admitted) {
		t.Fatalf("fixture replay used caller-mutated DTO: %#v", got)
	}
}

func TestRound7DistinctFixtureViewsWithEqualWordsRemainAdmissible(t *testing.T) {
	profile, spec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	firstWords := spec.Dependencies[0].View.Record().Words
	second := spec.Dependencies[1].View.Record()
	second.Words = append([]uint16(nil), firstWords...)
	spec.Dependencies[1].View = snapshotFromRecord(t, second)
	state := round3State(t, profile, "round7-fixture-equal-words")
	factory := round3Factory(t, profile, state, &round3MemoryCAS{state: state})
	if _, decoded := round3Attempt(t, factory, spec); !reflect.DeepEqual(
		decoded.Dependencies[0].View.Record().Words,
		decoded.Dependencies[1].View.Record().Words,
	) {
		t.Fatal("fixture setup did not retain equal raw words")
	}
}

func TestRound7FixturePhysicalResponseOwnsChronologyFacts(t *testing.T) {
	profile, valid := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	makeRepeatedWireGroup(t, &valid)
	observed := time.Unix(1_700_000_600, 0).UTC()
	receipt := observed.Add(time.Second)
	for index := range valid.Dependencies {
		valid.Dependencies[index].SourceTime = reg.SourceTimeObserved(observed)
		valid.Dependencies[index].LocalReceiptTime = receipt
		valid.Dependencies[index].AcquisitionOrdinal = 1
	}
	valid.SourceTime = reg.SourceTimeObserved(observed)
	valid.LocalReceiptTime = receipt
	if _, err := round3Publish(t, profile, valid); err != nil {
		t.Fatalf("fixture shared physical chronology rejected: %v", err)
	}

	_, contradictory := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	makeRepeatedWireGroup(t, &contradictory)
	contradictory.Dependencies[1].SourceTime = reg.SourceTimeObserved(
		contradictory.Dependencies[0].SourceTime.Time.Add(time.Second),
	)
	contradictory.Dependencies[1].LocalReceiptTime =
		contradictory.Dependencies[0].LocalReceiptTime.Add(time.Second)
	contradictory.Dependencies[1].AcquisitionOrdinal = 2
	if _, err := round3Publish(t, profile, contradictory); err == nil {
		t.Fatal("fixture physical response accepted contradictory chronology facts")
	}
}

func TestRound7FixtureBoundedSourceTimeStatesUseReceiptSkew(t *testing.T) {
	t.Run("explicit unavailable", func(t *testing.T) {
		profile, spec := boundedFixture(
			t,
			reg.AcquisitionOrderDependencyDeclaration,
		)
		for index := range spec.Dependencies {
			spec.Dependencies[index].SourceTime = reg.SourceTimeUnavailable()
		}
		spec.SourceTime = reg.SourceTimeUnavailable()
		if _, err := round3Publish(t, profile, spec); err != nil {
			t.Fatalf("fixture unavailable source time rejected: %v", err)
		}
	})

	t.Run("receipt skew still enforced", func(t *testing.T) {
		profile, spec := boundedFixture(
			t,
			reg.AcquisitionOrderDependencyDeclaration,
		)
		for index := range spec.Dependencies {
			spec.Dependencies[index].SourceTime = reg.SourceTimeUnavailable()
		}
		spec.Dependencies[1].LocalReceiptTime =
			spec.Dependencies[0].LocalReceiptTime.Add(4 * time.Second)
		spec.SourceTime = reg.SourceTimeUnavailable()
		spec.LocalReceiptTime = spec.Dependencies[1].LocalReceiptTime
		if _, err := round3Publish(t, profile, spec); err == nil {
			t.Fatal("fixture unavailable source time bypassed receipt skew")
		}
	})

	t.Run("explicit observed UTC year one", func(t *testing.T) {
		profile, spec := boundedFixture(
			t,
			reg.AcquisitionOrderDependencyDeclaration,
		)
		for index := range spec.Dependencies {
			spec.Dependencies[index].SourceTime =
				reg.SourceTimeObserved(time.Time{})
		}
		spec.SourceTime = reg.SourceTimeObserved(time.Time{})
		observation, err := round3Publish(t, profile, spec)
		if err != nil {
			t.Fatalf("fixture year-one source time rejected: %v", err)
		}
		if got := observation.Spec().SourceTime; got.State !=
			reg.SourceTimeObservedState || !got.Time.Equal(time.Time{}) {
			t.Fatalf("fixture year-one source time = %#v", got)
		}
	})
}

func round7FixtureJSONWithReceipts(
	t *testing.T,
	spec reg.ObservationSpec,
	value any,
) []byte {
	t.Helper()
	encoded, err := reg.MarshalFixtureSpec(spec)
	if err != nil {
		t.Fatalf("MarshalFixtureSpec: %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(encoded, &record); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	record["local_receipt_time"] = value
	dependencies, ok := record["dependencies"].([]any)
	if !ok {
		t.Fatal("fixture dependencies have an unexpected JSON shape")
	}
	for _, item := range dependencies {
		dependency, ok := item.(map[string]any)
		if !ok {
			t.Fatal("fixture dependency has an unexpected JSON shape")
		}
		dependency["local_receipt_time"] = value
	}
	mutated, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return mutated
}

func TestRound7FixtureJSONRetainsRequiredYearOneReceiptPresence(t *testing.T) {
	profile, spec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	const yearOne = "0001-01-01T00:00:00Z"
	encoded := round7FixtureJSONWithReceipts(t, spec, yearOne)
	state := round3State(t, profile, "round7-json-year-one")
	factory := round3Factory(t, profile, state, &round3MemoryCAS{state: state})
	attempt, err := factory.BeginObservationAttempt(reg.AttemptIdentity{
		PollGenerationID: spec.PollGenerationID,
		RetryOrdinal:     spec.RetryOrdinal,
	})
	if err != nil {
		t.Fatalf("BeginObservationAttempt: %v", err)
	}
	decoded, err := attempt.DecodeSpec(encoded)
	if err != nil {
		t.Fatalf("DecodeSpec(year one receipt): %v", err)
	}
	observation, err := attempt.Publish(decoded)
	if err != nil {
		t.Fatalf("Publish(year one receipt): %v", err)
	}
	if !observation.Spec().LocalReceiptTime.Equal(time.Time{}) {
		t.Fatalf(
			"year-one receipt = %s",
			observation.Spec().LocalReceiptTime.Format(time.RFC3339Nano),
		)
	}
	reencoded, err := reg.MarshalFixtureSpec(observation.Spec())
	if err != nil {
		t.Fatalf("MarshalObservation: %v", err)
	}
	if strings.Count(string(reencoded), yearOne) < len(spec.Dependencies)+1 {
		t.Fatal("year-one receipt presence was lost during replay serialization")
	}

	for _, test := range []struct {
		name     string
		mutate   func(map[string]any)
		expected string
	}{
		{
			name: "absent",
			mutate: func(record map[string]any) {
				delete(record, "local_receipt_time")
			},
			expected: `serialized object is missing required key "local_receipt_time"`,
		},
		{
			name: "null",
			mutate: func(record map[string]any) {
				record["local_receipt_time"] = nil
			},
			expected: `serialized key "local_receipt_time": serialized contract contains a non-canonical null`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var record map[string]any
			if err := json.Unmarshal(encoded, &record); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			test.mutate(record)
			candidate, err := json.Marshal(record)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			candidateState := round3State(
				t,
				profile,
				"round7-json-"+test.name,
			)
			candidateFactory := round3Factory(
				t,
				profile,
				candidateState,
				&round3MemoryCAS{state: candidateState},
			)
			candidateAttempt, err := candidateFactory.BeginObservationAttempt(
				reg.AttemptIdentity{
					PollGenerationID: spec.PollGenerationID,
					RetryOrdinal:     spec.RetryOrdinal,
				},
			)
			if err != nil {
				t.Fatalf("BeginObservationAttempt: %v", err)
			}
			_, err = candidateAttempt.DecodeSpec(candidate)
			if err == nil || err.Error() != test.expected {
				t.Fatalf("DecodeSpec error = %v, want %q", err, test.expected)
			}
		})
	}
}

func TestRound7BoundedCoherenceRejectsMixedTCPConnections(t *testing.T) {
	profile, spec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	second := spec.Dependencies[1].View.Record()
	second.ConnectionID++
	spec.Dependencies[1].View = snapshotFromRecord(t, second)
	if _, err := round3Publish(t, profile, spec); err == nil {
		t.Fatal("bounded sample mixed TCP connection identities in one generation")
	}
}
