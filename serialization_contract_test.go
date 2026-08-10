package modbusreg_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestLedgerRestartStateStrictTopLevelJSON(t *testing.T) {
	profile := profileFixture(t)
	initial := emptyLedgerState(t, profile)
	limits := reg.DefaultLedgerLimits()
	validTruncated := []byte(`{"schema_version":1,"next_terminal_sequence":7,"sequence_exhausted":false,"truncated_through_sequence":6,"audit_tombstones":[]}`)
	var decoded reg.LedgerRestartState
	if err := json.Unmarshal(validTruncated, &decoded); err != nil {
		t.Fatalf("Unmarshal(valid truncated restart): %v", err)
	}
	if _, err := reg.NewSampleLedgerFromRestart(initial, 0, limits, decoded); err != nil {
		t.Fatalf("NewSampleLedgerFromRestart(valid truncated restart): %v", err)
	}
	roundTrip, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal(valid truncated restart): %v", err)
	}
	if !bytes.Equal(roundTrip, validTruncated) {
		t.Fatalf("restart JSON round trip=%s", roundTrip)
	}

	invalid := [][]byte{
		[]byte(`{"schema_version":1,"next_terminal_sequence":1,"sequence_exhausted":false,"truncated_through_sequence":0,"audit_tombstones":[],"unknown":true}`),
		[]byte(`{"schema_version":1,"next_terminal_sequence":1,"sequence_exhausted":false,"audit_tombstones":[]}`),
		[]byte(`{"SchemaVersion":1,"NextTerminalSequence":1,"SequenceExhausted":false,"TruncatedThroughSequence":0,"AuditTombstones":[]}`),
		[]byte(`{"schema_version":1,"schema_version":1,"next_terminal_sequence":1,"sequence_exhausted":false,"truncated_through_sequence":0,"audit_tombstones":[]}`),
	}
	for index, encoded := range invalid {
		var restart reg.LedgerRestartState
		if err := json.Unmarshal(encoded, &restart); err == nil {
			t.Fatalf("invalid top-level restart JSON %d was accepted", index)
		}
	}
}

func beginFixtureReplay(
	t *testing.T,
	factory *fixtureValidationFactory,
	spec reg.ObservationSpec,
) *fixtureReplaySession {
	t.Helper()
	attempt, err := factory.BeginFixtureReplay(reg.AttemptIdentity{
		PollGenerationID: spec.PollGenerationID,
		RetryOrdinal:     spec.RetryOrdinal,
	})
	if err != nil {
		t.Fatalf("BeginFixtureReplay: %v", err)
	}
	return attempt
}

func decodeFixtureSpec(
	t *testing.T,
	attempt *fixtureReplaySession,
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

func TestDeterministicReplayAcrossFreshFactories(t *testing.T) {
	profile, spec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	firstState := fixtureLedgerState(t, profile, "fixture-replay-first")
	firstFactory := fixtureFactoryFromState(t, profile, firstState)
	firstAttempt := beginFixtureReplay(t, firstFactory, spec)
	bound := decodeFixtureSpec(t, firstAttempt, spec)
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

	secondState := fixtureLedgerState(t, profile, "fixture-replay-second")
	secondFactory := fixtureFactoryFromState(t, profile, secondState)
	secondAttempt := beginFixtureReplay(t, secondFactory, spec)
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
	if _, err := secondAttempt.Replay(decoded); err != nil {
		t.Fatalf("Replay(fresh replay): %v", err)
	}
}

func TestPhysicalRequestIDHasOneCanonicalTuple(t *testing.T) {
	profile, spec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	first := spec.Dependencies[0].View.Record()
	second := spec.Dependencies[1].View.Record()
	second.PhysicalRequestID = first.PhysicalRequestID
	spec.Dependencies[1].View = snapshotFromRecord(t, second)
	if _, err := validateFixtureReplay(t, profile, spec); err == nil {
		t.Fatal("one physical request ID mapped to different ranges and wires")
	}
}

func TestJSONRejectsNullAndMissingRequiredMembers(t *testing.T) {
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

	state := fixtureLedgerState(t, profile, "fixture-json-ledger")
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
	observationState := fixtureLedgerState(t, boundedProfile, "fixture-json-observation")
	observationFactory := fixtureFactoryFromState(
		t,
		boundedProfile,
		observationState,
	)
	observationAttempt := beginFixtureReplay(
		t,
		observationFactory,
		observationSpec,
	)
	observationBytes, err := observationAttempt.MarshalSpec(
		decodeFixtureSpec(t, observationAttempt, observationSpec),
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
		freshState := fixtureLedgerState(
			t,
			boundedProfile,
			"fixture-json-observation-decode",
		)
		freshFactory := fixtureFactoryFromState(
			t,
			boundedProfile,
			freshState,
		)
		freshAttempt := beginFixtureReplay(t, freshFactory, observationSpec)
		if _, err := freshAttempt.DecodeSpec(candidate); err == nil {
			t.Fatalf("observation null/missing case %d was accepted", index)
		}
	}
}

func TestSerializationNeverMutatesObservationInput(t *testing.T) {
	profile, spec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	state := fixtureLedgerState(t, profile, "fixture-immutable")
	factory := fixtureFactoryFromState(
		t,
		profile,
		state,
	)
	attempt := beginFixtureReplay(t, factory, spec)
	bound := decodeFixtureSpec(t, attempt, spec)
	location := time.FixedZone("fixture-offset", 2*60*60)
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

func TestSingleWireDependencyTimeStateIsExplicit(t *testing.T) {
	profile := profileFixture(t)
	invalid := successfulObservationSpec(t, profile)
	invalid.Dependencies[0].SourceTime = reg.SourceTimeSpec{}
	if _, err := validateFixtureReplay(t, profile, invalid); err == nil {
		t.Fatal("single-wire dependency accepted an implicit time state")
	}

	valid := successfulObservationSpec(t, profile)
	observation, err := validateFixtureReplay(t, profile, valid)
	if err != nil {
		t.Fatalf("valid single-wire observation rejected: %v", err)
	}
	if _, err := reg.MarshalFixtureSpec(observation.Spec()); err != nil {
		t.Fatalf("admitted single-wire observation cannot serialize: %v", err)
	}
}

func TestNormalizationSourceLocatorRequiresIdentifier(t *testing.T) {
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
