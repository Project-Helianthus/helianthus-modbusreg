package modbusreg_test

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func round5StoreState(store *round3MemoryCAS) reg.SampleLedgerState {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.state
}

func round5SingleDependencyProfile(
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

func round5ObservationSpec(
	t *testing.T,
	profile reg.ProfileDescriptor,
	snapshot reg.LogicalViewSnapshot,
	pollGeneration uint64,
	endpoint string,
	unitID byte,
) reg.ObservationSpec {
	t.Helper()
	dependency := profile.Dependencies().Dependencies()[0]
	result, err := reg.NewDependencyResult(reg.DependencyResult{
		DependencyID:         dependency.ID(),
		DependencyVersion:    dependency.Version(),
		CodecID:              dependency.CodecID(),
		CodecVersion:         dependency.CodecVersion(),
		NormalizationVersion: dependency.Normalization().Spec().Version,
		Status:               reg.DependencyReadSuccessful,
		View:                 snapshot,
		SourceTime:           reg.SourceTimeUnavailable(),
	})
	if err != nil {
		t.Fatalf("NewDependencyResult: %v", err)
	}
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
		PollGenerationID:       pollGeneration,
		DependencySetID:        profile.Dependencies().ID(),
		DependencySetVersion:   profile.Dependencies().Version(),
		SourceValidity:         reg.SourceValid,
		SourceTime:             reg.SourceTimeUnavailable(),
		LocalReceiptTime:       time.Unix(1_700_000_000, 0).UTC(),
		Endpoint:               endpoint,
		UnitID:                 unitID,
		Dependencies:           []reg.DependencyResult{result},
	}
}

func TestRound5LedgerRejectsRestartReplayAndOrdersCommittedAttempts(
	t *testing.T,
) {
	profile, firstSpec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	initial := round3State(t, profile, "round5-attempt-order")
	firstStore := &round3MemoryCAS{state: initial}
	firstFactory := round3Factory(t, profile, initial, firstStore)
	firstAttempt := round4Attempt(t, firstFactory, firstSpec)
	firstBound := round4Bind(t, firstAttempt, firstSpec)
	serializedAttempt, err := firstAttempt.MarshalSpec(firstBound)
	if err != nil {
		t.Fatalf("MarshalSpec(first): %v", err)
	}
	if _, err := firstAttempt.Publish(firstBound); err != nil {
		t.Fatalf("Publish(first): %v", err)
	}
	committed := round5StoreState(firstStore)
	if committed.LastCommittedAttempt != (reg.AttemptIdentity{
		PollGenerationID: firstSpec.PollGenerationID,
		RetryOrdinal:     firstSpec.RetryOrdinal,
	}) {
		t.Fatalf("last committed attempt = %#v", committed.LastCommittedAttempt)
	}
	encodedState, err := reg.MarshalSampleLedgerState(committed)
	if err != nil {
		t.Fatalf("MarshalSampleLedgerState: %v", err)
	}
	if !bytes.Contains(encodedState, []byte(`"last_committed_attempt"`)) {
		t.Fatal("serialized ledger omitted the committed attempt identity")
	}
	restored, err := reg.UnmarshalSampleLedgerState(encodedState)
	if err != nil {
		t.Fatalf("UnmarshalSampleLedgerState: %v", err)
	}
	restartStore := &round3MemoryCAS{state: restored}
	restartFactory := round3Factory(t, profile, restored, restartStore)
	replayAttempt := round4Attempt(t, restartFactory, firstSpec)
	replayed, err := replayAttempt.DecodeSpec(serializedAttempt)
	if err != nil {
		t.Fatalf("DecodeSpec(restart): %v", err)
	}
	if observation, err := replayAttempt.Publish(replayed); err == nil ||
		observation.SampleID() != "" {
		t.Fatal("restart replay issued sample :2")
	}
	if got := round5StoreState(restartStore); got != restored {
		t.Fatal("restart replay advanced persisted state")
	}

	_, higherRetry := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	higherRetry.RetryOrdinal = 2
	for index := range higherRetry.Dependencies {
		higherRetry.Dependencies[index].RetryOrdinal = 2
	}
	higherAttempt := round4Attempt(t, restartFactory, higherRetry)
	if _, err := higherAttempt.Publish(
		round4Bind(t, higherAttempt, higherRetry),
	); err != nil {
		t.Fatalf("higher retry ordinal rejected: %v", err)
	}

	_, lower := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	lowerAttempt := round4Attempt(t, restartFactory, lower)
	if observation, err := lowerAttempt.Publish(
		round4Bind(t, lowerAttempt, lower),
	); err == nil || observation.SampleID() != "" {
		t.Fatal("lower retry ordinal was committed")
	}

	_, higherPoll := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	higherPoll.PollGenerationID++
	for index := range higherPoll.Dependencies {
		record := higherPoll.Dependencies[index].View.Record()
		record.PollGeneration = higherPoll.PollGenerationID
		higherPoll.Dependencies[index].View = snapshotFromRecord(t, record)
	}
	higherPollAttempt := round4Attempt(t, restartFactory, higherPoll)
	if _, err := higherPollAttempt.Publish(
		round4Bind(t, higherPollAttempt, higherPoll),
	); err != nil {
		t.Fatalf("higher poll generation rejected: %v", err)
	}
}

func TestRound5UTCRangeValidationPrecedesCAS(t *testing.T) {
	profile := profileFixture(t)
	cases := []struct {
		name   string
		mutate func(*reg.ObservationSpec)
	}{
		{
			name: "lower bound crosses into UTC year zero",
			mutate: func(spec *reg.ObservationSpec) {
				location := time.FixedZone("UTC+14", 14*60*60)
				spec.SourceTime = reg.SourceTimeObserved(
					time.Date(1, time.January, 1, 0, 0, 0, 0, location),
				)
			},
		},
		{
			name: "upper bound crosses into UTC year 10000",
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
			initial := round3State(t, profile, "round5-time-"+test.name)
			store := &round3MemoryCAS{state: initial}
			factory := round3Factory(t, profile, initial, store)
			marshalAttempt := round4Attempt(t, factory, spec)
			if _, err := marshalAttempt.MarshalSpec(
				round4Bind(t, marshalAttempt, spec),
			); err == nil {
				t.Fatal("out-of-range UTC timestamp serialized")
			}

			publishSpec := successfulObservationSpec(t, profile)
			test.mutate(&publishSpec)
			publishAttempt := round4Attempt(t, factory, publishSpec)
			observation, err := publishAttempt.Publish(
				round4Bind(t, publishAttempt, publishSpec),
			)
			if err == nil || observation.SampleID() != "" {
				t.Fatal("out-of-range UTC timestamp published")
			}
			if store.commits != 0 || round5StoreState(store) != initial {
				t.Fatal("timestamp failure consumed CAS state")
			}
		})
	}
}

func TestRound5RTUPhysicalResponseHasOneLogicalViewInBoundedMode(
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
	if _, err := round3Publish(t, profile, spec); err == nil {
		t.Fatal("bounded coherence admitted two RTU views from one response")
	}
}

type round5RTUClock struct {
	now time.Duration
}

func (clock *round5RTUClock) ContractVersion() string {
	return "round5-fixture-clock/v1"
}

func (clock *round5RTUClock) Now() time.Duration {
	return clock.now
}

func TestRound5RuntimeRTUFixtureViewAdmitsAndReplaysExactly(t *testing.T) {
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
	line, err := modbus.NewRTUFixtureLine("round5-memory-line")
	if err != nil {
		t.Fatalf("NewRTUFixtureLine: %v", err)
	}
	clock := &round5RTUClock{}
	endpoint, err := modbus.NewRTUFixtureEndpoint(
		modbus.RTUFixtureEndpointConfig{
			Endpoint:           "round5-memory-endpoint",
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
			AuthorizationScope: "round5-read-only",
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
	runtimeView, ok := result.LogicalView()
	if !ok || !result.Deliverable() {
		t.Fatal("runtime fixture did not produce a logical view")
	}
	snapshot, err := reg.CaptureLogicalView(runtimeView)
	if err != nil {
		t.Fatalf("CaptureLogicalView: %v", err)
	}
	profile := round5SingleDependencyProfile(t)
	spec := round5ObservationSpec(
		t,
		profile,
		snapshot,
		41,
		"round5-memory-endpoint",
		1,
	)
	observation, err := round3Publish(t, profile, spec)
	if err != nil {
		t.Fatalf("Publish(runtime fixture): %v", err)
	}
	replay := observation.Replay()
	if len(replay) != 1 ||
		!reflect.DeepEqual(replay[0].LogicalViewRecord(), snapshot.Record()) ||
		!reflect.DeepEqual(replay[0].RawWords(), []uint16{0x0102, 0x0304}) {
		t.Fatalf("runtime fixture replay = %#v", replay)
	}
}

func TestRound5TCPDisjointSurvivingViewsRemainValid(t *testing.T) {
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
	profile, err := reg.NewProfileDescriptor(profileSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor: %v", err)
	}
	spec := successfulObservationSpec(t, profile)
	first := spec.Dependencies[0].View.Record()
	first.PhysicalOffset = 100
	first.PhysicalWordCount = 6
	first.LogicalOffset = 100
	first.SliceOffset = 0
	first.Words = []uint16{1, 2}
	secondRecord := spec.Dependencies[1].View.Record()
	secondRecord.WireResponseID = first.WireResponseID
	secondRecord.PhysicalRequestID = first.PhysicalRequestID
	secondRecord.PhysicalOffset = 100
	secondRecord.PhysicalWordCount = 6
	secondRecord.LogicalOffset = 104
	secondRecord.SliceOffset = 4
	secondRecord.Words = []uint16{5, 6}
	spec.Dependencies[0].View = snapshotFromRecord(t, first)
	spec.Dependencies[1].View = snapshotFromRecord(t, secondRecord)

	// Pinned M1 ReplaySuccessfulResponse skips dependents cancelled after write,
	// so the active survivors may occupy disjoint slices of one valid response.
	observation, err := round3Publish(t, profile, spec)
	if err != nil {
		t.Fatalf("disjoint surviving TCP views rejected: %v", err)
	}
	if len(observation.Replay()) != 2 {
		t.Fatal("disjoint surviving TCP views were not retained")
	}
}
