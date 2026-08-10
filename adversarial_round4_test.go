package modbusreg_test

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func round4Attempt(
	t *testing.T,
	factory *fixtureValidationFactory,
	spec reg.ObservationSpec,
) *fixtureValidationAttempt {
	t.Helper()
	attempt, err := factory.BeginObservationAttempt(reg.AttemptIdentity{
		PollGenerationID: spec.PollGenerationID,
		RetryOrdinal:     spec.RetryOrdinal,
	})
	if err != nil {
		t.Fatalf("BeginObservationAttempt: %v", err)
	}
	return attempt
}

func round4Bind(
	t *testing.T,
	attempt *fixtureValidationAttempt,
	spec reg.ObservationSpec,
) reg.ObservationSpec {
	t.Helper()
	encoded, err := reg.MarshalFixtureSpec(spec)
	if err != nil {
		t.Fatalf("MarshalFixtureSpec: %v", err)
	}
	decoded, err := attempt.DecodeSpec(encoded)
	if err != nil {
		t.Fatalf("DecodeSpec: %v", err)
	}
	return decoded
}

func TestRound4DeterministicReplayAcrossFreshFactories(t *testing.T) {
	profile, spec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	firstState := round3State(t, profile, "round4-replay-first")
	firstStore := &round3MemoryCAS{state: firstState}
	firstFactory := round3Factory(t, profile, firstState, firstStore)
	firstAttempt := round4Attempt(t, firstFactory, spec)
	bound := round4Bind(t, firstAttempt, spec)
	encoded, err := firstAttempt.MarshalSpec(bound)
	if err != nil {
		t.Fatalf("MarshalSpec(first): %v", err)
	}
	if bytes.Contains(encoded, []byte("retry_attempt_token")) ||
		!bytes.Contains(encoded, []byte(`"retry_ordinal":1`)) {
		t.Fatal("serialized attempt identity is random or incomplete")
	}
	if _, err := firstAttempt.DecodeSpec(encoded); err != nil {
		t.Fatalf("offline fixture was not idempotently replayable: %v", err)
	}

	secondState := round3State(t, profile, "round4-replay-second")
	secondStore := &round3MemoryCAS{state: secondState}
	secondFactory := round3Factory(t, profile, secondState, secondStore)
	secondAttempt := round4Attempt(t, secondFactory, spec)
	decoded, err := secondAttempt.DecodeSpec(encoded)
	if err != nil {
		t.Fatalf("DecodeSpec(fresh factory): %v", err)
	}
	_, additional := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	if _, err := secondAttempt.BindDependency(
		additional.Dependencies[0],
	); err == nil {
		t.Fatal("a serialized-capture attempt accepted a direct dependency")
	}
	reencoded, err := secondAttempt.MarshalSpec(decoded)
	if err != nil {
		t.Fatalf("MarshalSpec(fresh factory): %v", err)
	}
	if !bytes.Equal(encoded, reencoded) {
		t.Fatal("fresh-process-equivalent replay is not byte deterministic")
	}
	if _, err := secondAttempt.Publish(decoded); err != nil {
		t.Fatalf("Publish(fresh replay): %v", err)
	}
}

func TestRound4PhysicalRequestIDHasOneCanonicalTuple(t *testing.T) {
	profile, spec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	first := spec.Dependencies[0].View.Record()
	second := spec.Dependencies[1].View.Record()
	second.PhysicalRequestID = first.PhysicalRequestID
	spec.Dependencies[1].View = snapshotFromRecord(t, second)
	if _, err := round3Publish(t, profile, spec); err == nil {
		t.Fatal("one physical request ID mapped to different ranges and wires")
	}
}

func TestRound4JSONRejectsNullAndMissingRequiredMembers(t *testing.T) {
	profile := profileFixture(t)
	profileBytes, err := reg.MarshalProfileDescriptor(profile)
	if err != nil {
		t.Fatalf("MarshalProfileDescriptor: %v", err)
	}
	profileCases := [][]byte{
		bytes.Replace(
			profileBytes,
			[]byte(`,"default_enabled":false`),
			nil,
			1,
		),
		bytes.Replace(
			profileBytes,
			[]byte(`"default_enabled":false`),
			[]byte(`"default_enabled":null`),
			1,
		),
		bytes.Replace(
			profileBytes,
			[]byte(`"profile_id":"example.standard.energy"`),
			[]byte(`"profile_id":null`),
			1,
		),
		bytes.Replace(
			profileBytes,
			[]byte(`"require_generation_equality":false`),
			[]byte(`"require_generation_equality":null`),
			1,
		),
	}
	for index, candidate := range profileCases {
		if bytes.Equal(candidate, profileBytes) {
			t.Fatalf("profile mutation %d did not apply", index)
		}
		if _, err := reg.UnmarshalProfileDescriptor(candidate); err == nil {
			t.Fatalf("profile null/missing case %d was accepted", index)
		}
	}

	state := round3State(t, profile, "round4-json-ledger")
	ledgerBytes, err := reg.MarshalSampleLedgerState(state)
	if err != nil {
		t.Fatalf("MarshalSampleLedgerState: %v", err)
	}
	ledgerCases := [][]byte{
		bytes.Replace(ledgerBytes, []byte(`"revision":0,`), nil, 1),
		bytes.Replace(
			ledgerBytes,
			[]byte(`"high_water":0`),
			[]byte(`"high_water":null`),
			1,
		),
	}
	for index, candidate := range ledgerCases {
		if bytes.Equal(candidate, ledgerBytes) {
			t.Fatalf("ledger mutation %d did not apply", index)
		}
		if _, err := reg.UnmarshalSampleLedgerState(candidate); err == nil {
			t.Fatalf("ledger null/missing case %d was accepted", index)
		}
	}

	boundedProfile, observationSpec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	observationState := round3State(t, boundedProfile, "round4-json-observation")
	observationFactory := round3Factory(
		t,
		boundedProfile,
		observationState,
		&round3MemoryCAS{state: observationState},
	)
	observationAttempt := round4Attempt(
		t,
		observationFactory,
		observationSpec,
	)
	observationBytes, err := observationAttempt.MarshalSpec(
		round4Bind(t, observationAttempt, observationSpec),
	)
	if err != nil {
		t.Fatalf("MarshalSpec: %v", err)
	}
	observationCases := [][]byte{
		bytes.Replace(
			observationBytes,
			[]byte(`"retry_ordinal":1,`),
			nil,
			1,
		),
		bytes.Replace(
			observationBytes,
			[]byte(`"PollGeneration":41`),
			[]byte(`"PollGeneration":null`),
			1,
		),
		bytes.Replace(
			observationBytes,
			[]byte(`"state":"observed"`),
			[]byte(`"state":null`),
			1,
		),
	}
	for index, candidate := range observationCases {
		if bytes.Equal(candidate, observationBytes) {
			t.Fatalf("observation mutation %d did not apply", index)
		}
		freshState := round3State(
			t,
			boundedProfile,
			"round4-json-observation-decode",
		)
		freshFactory := round3Factory(
			t,
			boundedProfile,
			freshState,
			&round3MemoryCAS{state: freshState},
		)
		freshAttempt := round4Attempt(t, freshFactory, observationSpec)
		if _, err := freshAttempt.DecodeSpec(candidate); err == nil {
			t.Fatalf("observation null/missing case %d was accepted", index)
		}
	}
}

func TestRound4SerializationNeverMutatesObservationInput(t *testing.T) {
	profile, spec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	state := round3State(t, profile, "round4-immutable")
	factory := round3Factory(
		t,
		profile,
		state,
		&round3MemoryCAS{state: state},
	)
	attempt := round4Attempt(t, factory, spec)
	bound := round4Bind(t, attempt, spec)
	location := time.FixedZone("round4-offset", 2*60*60)
	source := time.Date(2026, time.July, 29, 21, 0, 0, 123, location)
	bound.SourceTime = reg.SourceTimeObserved(source.Add(time.Second))
	bound.LocalReceiptTime = source.Add(2 * time.Second)
	for index := range bound.Dependencies {
		bound.Dependencies[index].SourceTime = reg.SourceTimeObserved(
			source.Add(time.Duration(index) * time.Second),
		)
		bound.Dependencies[index].LocalReceiptTime = source.Add(
			time.Duration(index+1) * time.Second,
		)
	}
	bound.SourceValidity = reg.SourceValidity("invalid-test-value")
	before := bound
	before.Dependencies = append(
		[]reg.DependencyResult(nil),
		bound.Dependencies...,
	)
	if _, err := attempt.MarshalSpec(bound); err == nil {
		t.Fatal("invalid observation unexpectedly serialized")
	}
	if !reflect.DeepEqual(bound, before) {
		t.Fatal("failed serialization canonicalized caller-owned storage")
	}
}

func TestRound4SingleWireDependencyTimeStateIsExplicit(t *testing.T) {
	profile := profileFixture(t)
	invalid := successfulObservationSpec(t, profile)
	invalid.Dependencies[0].SourceTime = reg.SourceTimeSpec{}
	if _, err := round3Publish(t, profile, invalid); err == nil {
		t.Fatal("single-wire dependency accepted an implicit time state")
	}

	valid := successfulObservationSpec(t, profile)
	observation, err := round3Publish(t, profile, valid)
	if err != nil {
		t.Fatalf("valid single-wire observation rejected: %v", err)
	}
	if _, err := reg.MarshalFixtureSpec(observation.Spec()); err != nil {
		t.Fatalf("admitted single-wire observation cannot serialize: %v", err)
	}
}

func TestRound4NormalizationSourceLocatorRequiresIdentifier(t *testing.T) {
	for _, locator := range []string{
		"https://",
		"https://example.com",
		"urn:helianthus:evidence:",
	} {
		spec := normalizationSpec(t, 101)
		spec.SourceLocator = locator
		if _, err := reg.NewAddressNormalization(spec); err == nil {
			t.Fatalf("bare normalization source locator %q was accepted", locator)
		}
	}
	valid := normalizationSpec(t, 101)
	valid.SourceLocator = "https://example.com/registers/map-v1"
	if _, err := reg.NewAddressNormalization(valid); err != nil {
		t.Fatalf("identified HTTPS source locator rejected: %v", err)
	}
}
