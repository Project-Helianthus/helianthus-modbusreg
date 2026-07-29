package modbusreg_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
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

func round6RuntimeRTUView(t *testing.T) modbus.LogicalReadView {
	t.Helper()
	timing, err := modbus.NewRTUTiming(modbus.RTUTimingConfig{
		Baud:               9600,
		DataBits:           8,
		Parity:             modbus.RTUParityEven,
		StopBits:           1,
		MaxResponseLatency: 20 * time.Millisecond,
		MaxQuiescence:      50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewRTUTiming: %v", err)
	}
	line, err := modbus.NewRTUFixtureLine("round6-memory-line")
	if err != nil {
		t.Fatalf("NewRTUFixtureLine: %v", err)
	}
	clock := &round5RTUClock{}
	endpoint, err := modbus.NewRTUFixtureEndpoint(
		modbus.RTUFixtureEndpointConfig{
			Endpoint:           "round6-memory-endpoint",
			Line:               line,
			Timing:             timing,
			Clock:              clock,
			FixtureOptIn:       true,
			MaxDiscardedFrames: 4,
		},
	)
	if err != nil {
		t.Fatalf("NewRTUFixtureEndpoint: %v", err)
	}
	defer func() { _ = endpoint.Close() }()
	request, err := modbus.NewReadRegistersRequest(
		modbus.FunctionReadInputRegisters,
		100,
		2,
	)
	if err != nil {
		t.Fatalf("NewReadRegistersRequest: %v", err)
	}
	handle, _, err := endpoint.BeginRead(
		context.Background(),
		modbus.RTUReadPlan{
			UnitID:             1,
			AuthorizationScope: "round6-read-only",
			PollGeneration:     41,
			DeadlineIdentity:   12,
			LogicalViewID:      1001,
			Timeout:            20 * time.Millisecond,
			Request:            request,
		},
	)
	if err != nil {
		t.Fatalf("BeginRead: %v", err)
	}
	if err := endpoint.CompleteTransmit(
		handle,
		modbus.TransmitComplete,
	); err != nil {
		t.Fatalf("CompleteTransmit: %v", err)
	}
	responseFrame := []byte{0x01, 0x04, 0x04, 0x01, 0x02, 0x03, 0x04, 0x5a, 0x8b}
	for index, value := range responseFrame {
		if index != 0 {
			clock.now += timing.CharacterTime()
		}
		if err := endpoint.FeedByte(value); err != nil {
			t.Fatalf("FeedByte(%d): %v", index, err)
		}
	}
	clock.now += timing.InterFrame()
	result, err := endpoint.EndFrame()
	if err != nil {
		t.Fatalf("EndFrame: %v", err)
	}
	view, ok := result.LogicalView()
	if !ok || !result.Deliverable() {
		t.Fatal("runtime fixture did not produce a logical view")
	}
	return view
}

func round6RuntimeObservationSpec(
	profile reg.ProfileDescriptor,
	result reg.DependencyResult,
) reg.ObservationSpec {
	observed := time.Unix(1_700_000_200, 0).UTC()
	return reg.ObservationSpec{
		SchemaVersion:          reg.CurrentSchemaVersion(),
		RuntimeContractVersion: profile.RuntimeContractVersion(),
		ProfileID:              profile.ID(),
		ProfileVersion:         profile.Version(),
		CodecContractVersion:   profile.CodecContractVersion(),
		DetectorVersion:        profile.DetectorVersion(),
		NormalizationVersion:   profile.NormalizationVersion(),
		CoherenceVersion:       profile.CoherenceVersion(),
		QualificationVersion:   profile.QualificationVersion(),
		PollGenerationID:       41,
		RetryOrdinal:           1,
		DependencySetID:        profile.Dependencies().ID(),
		DependencySetVersion:   profile.Dependencies().Version(),
		SourceValidity:         reg.SourceValid,
		SourceTime:             reg.SourceTimeObserved(observed),
		LocalReceiptTime:       observed,
		Endpoint:               "round6-memory-endpoint",
		UnitID:                 1,
		Dependencies:           []reg.DependencyResult{result},
	}
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

func TestRound6RuntimeCaptureIsAttemptOwnedAndSingleUse(t *testing.T) {
	profile := round6BoundedSingleProfile(t)
	state := round3State(t, profile, "round6-runtime")
	store := &round3MemoryCAS{state: state}
	factory := round3Factory(t, profile, state, store)
	first, err := factory.BeginObservationAttempt(reg.AttemptIdentity{
		PollGenerationID: 41,
		RetryOrdinal:     1,
	})
	if err != nil {
		t.Fatalf("BeginObservationAttempt(first): %v", err)
	}
	second, err := factory.BeginObservationAttempt(reg.AttemptIdentity{
		PollGenerationID: 41,
		RetryOrdinal:     2,
	})
	if err != nil {
		t.Fatalf("BeginObservationAttempt(second): %v", err)
	}
	capture, err := reg.CaptureLogicalView(round6RuntimeRTUView(t))
	if err != nil {
		t.Fatalf("CaptureLogicalView: %v", err)
	}
	retained := capture
	facts := reg.RuntimeDependencyFacts{
		SourceTime:                   reg.SourceTimeObserved(time.Unix(1_700_000_200, 0).UTC()),
		LocalReceiptTime:             time.Unix(1_700_000_200, 0).UTC(),
		DocumentaryConsistencyMarker: "round6-sequence",
		AcquisitionOrdinal:           1,
	}
	result, err := first.CaptureDependency("energy-a", capture, facts)
	if err != nil {
		t.Fatalf("CaptureDependency(first): %v", err)
	}
	if result.RetryOrdinal != 1 {
		t.Fatalf("runtime retry ordinal = %d", result.RetryOrdinal)
	}
	if _, err := second.CaptureDependency(
		"energy-a",
		retained,
		facts,
	); err == nil {
		t.Fatal("retained runtime view was relabelled with retry ordinal 2")
	}
	observation, err := first.Publish(round6RuntimeObservationSpec(profile, result))
	if err != nil {
		t.Fatalf("Publish(runtime capture): %v", err)
	}
	replay := observation.Replay()
	if len(replay) != 1 ||
		!reflect.DeepEqual(replay[0].RawWords(), []uint16{0x0102, 0x0304}) {
		t.Fatalf("runtime replay = %#v", replay)
	}
}

func TestRound6SyntheticCaptureRequiresFixtureDecode(t *testing.T) {
	profile, spec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	state := round3State(t, profile, "round6-fixture")
	store := &round3MemoryCAS{state: state}
	factory := round3Factory(t, profile, state, store)
	direct, err := factory.BeginObservationAttempt(reg.AttemptIdentity{
		PollGenerationID: spec.PollGenerationID,
		RetryOrdinal:     spec.RetryOrdinal,
	})
	if err != nil {
		t.Fatalf("BeginObservationAttempt(direct): %v", err)
	}
	if _, err := direct.BindDependency(spec.Dependencies[0]); err == nil {
		t.Fatal("synthetic dependency entered the direct runtime path")
	}

	encoded, err := reg.MarshalFixtureSpec(spec)
	if err != nil {
		t.Fatalf("MarshalFixtureSpec: %v", err)
	}
	replay, err := factory.BeginObservationAttempt(reg.AttemptIdentity{
		PollGenerationID: spec.PollGenerationID,
		RetryOrdinal:     spec.RetryOrdinal,
	})
	if err != nil {
		t.Fatalf("BeginObservationAttempt(replay): %v", err)
	}
	decoded, err := replay.DecodeSpec(encoded)
	if err != nil {
		t.Fatalf("DecodeSpec: %v", err)
	}
	if _, err := replay.Publish(decoded); err != nil {
		t.Fatalf("Publish(decoded fixture): %v", err)
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
				&round3MemoryCAS{state: state},
			); err == nil {
				t.Fatal("factory accepted mode-incompatible persisted attempt")
			}
		})
	}
}

func TestRound6SuccessfulPollIsTerminalAcrossRetries(t *testing.T) {
	profile, spec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	state := round3State(t, profile, "round6-terminal-poll")
	store := &round3MemoryCAS{state: state}
	factory := round3Factory(t, profile, state, store)
	encoded, err := reg.MarshalFixtureSpec(spec)
	if err != nil {
		t.Fatalf("MarshalFixtureSpec: %v", err)
	}
	first, err := factory.BeginObservationAttempt(reg.AttemptIdentity{
		PollGenerationID: spec.PollGenerationID,
		RetryOrdinal:     spec.RetryOrdinal,
	})
	if err != nil {
		t.Fatalf("BeginObservationAttempt(first): %v", err)
	}
	decoded, err := first.DecodeSpec(encoded)
	if err != nil {
		t.Fatalf("DecodeSpec: %v", err)
	}
	if _, err := first.Publish(decoded); err != nil {
		t.Fatalf("Publish(first): %v", err)
	}
	if _, err := factory.BeginObservationAttempt(reg.AttemptIdentity{
		PollGenerationID: spec.PollGenerationID,
		RetryOrdinal:     spec.RetryOrdinal + 1,
	}); err == nil {
		t.Fatal("successful poll accepted a later retry ordinal")
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
	serialized, err := reg.MarshalObservation(observation)
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
