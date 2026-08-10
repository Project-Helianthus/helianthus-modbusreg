package modbusreg_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

type runtimeIntegrationTimer struct {
	timer *time.Timer
}

func (timer runtimeIntegrationTimer) Stop() bool {
	return timer.timer.Stop()
}

type runtimeIntegrationClock struct {
	started time.Time
}

func newRuntimeIntegrationClock() *runtimeIntegrationClock {
	return &runtimeIntegrationClock{started: time.Now()}
}

func (*runtimeIntegrationClock) ContractVersion() string {
	return "modbusreg-m1-06-integration/v1"
}

func (clock *runtimeIntegrationClock) Now() time.Duration {
	return time.Since(clock.started)
}

func (*runtimeIntegrationClock) AfterFunc(
	delay time.Duration,
	callback func(),
) modbus.TCPTransportTimer {
	return runtimeIntegrationTimer{timer: time.AfterFunc(delay, callback)}
}

type runtimeZeroJitter struct{}

func (runtimeZeroJitter) Next(time.Duration) time.Duration { return 0 }

func runtimeSourceConfig(clock modbus.RuntimeAcquisitionClock) modbus.RuntimeAcquisitionConfig {
	return modbus.RuntimeAcquisitionConfig{
		Limits: modbus.RuntimeAcquisitionLimits{
			MaxLiveCapabilities:                        16,
			MaxAttempts:                                16,
			MaxMembersPerAttempt:                       8,
			AttemptKeyMaxUTF8Bytes:                     64,
			SourceEvidenceIDMaxUTF8Bytes:               256,
			NormalizationRecordMaxEncodedBytes:         4096,
			NormalizationRequiredStringMaxUTF8Bytes:    256,
			NormalizationExtensionCountMax:             8,
			NormalizationExtensionKeyMaxUTF8Bytes:      128,
			NormalizationExtensionValueMaxEncodedBytes: 1024,
			RetainedDiagnosticCountPerObjectMax:        8,
			RetainedDiagnosticMaxUTF8Bytes:             256,
			CapabilityTombstoneLimit:                   32,
			CapabilityTombstoneMaxEncodedBytes:         128,
		},
		ClaimLifetime: time.Minute,
		Clock:         clock,
	}
}

func runtimeEndpointConfig(
	endpoint string,
	clock modbus.TCPMonotonicClock,
	source *modbus.RuntimeAcquisitionSource,
) modbus.TCPEndpointConfig {
	return modbus.TCPEndpointConfig{
		Endpoint: endpoint,
		PoolLimits: modbus.EndpointPoolLimits{
			MaxConnections: 1,
			Connection: modbus.ConnectionLimits{
				MaxInFlight:   2,
				MaxTombstones: 4,
			},
		},
		SchedulerLimits: modbus.SchedulerLimits{
			MaxActiveAdmissionKeys:         1,
			ProtectedSlotsPerKey:           1,
			SharedBurstSlots:               1,
			TotalQueued:                    2,
			MaxQueuedPerKey:                2,
			MaxQueuedPerAuthorizationScope: 2,
			MaxCoalescedDependentsPerKey:   2,
			MaxRetryAttempts:               1,
			MaxInFlightRequests:            1,
		},
		Backoff: modbus.BackoffConfig{
			Floor:             time.Millisecond,
			Ceiling:           time.Millisecond,
			MaxAttempts:       1,
			Jitter:            runtimeZeroJitter{},
			JitterAlgorithmID: "zero",
			JitterVersion:     "v1",
			JitterEvidence:    "deterministic-test",
		},
		MaxBufferedBytes:         260,
		MaxRequestDeadline:       time.Second,
		MaxResponseDeadline:      time.Second,
		Clock:                    clock,
		RuntimeAcquisitionSource: source,
	}
}

func serveOneModbusResponse(
	t *testing.T,
	listener net.Listener,
	words []uint16,
) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer func() { _ = connection.Close() }()
		header := make([]byte, 7)
		if _, err := io.ReadFull(connection, header); err != nil {
			done <- err
			return
		}
		length := int(binary.BigEndian.Uint16(header[4:6]))
		if length < 2 {
			done <- fmt.Errorf("invalid request length %d", length)
			return
		}
		body := make([]byte, length-1)
		if _, err := io.ReadFull(connection, body); err != nil {
			done <- err
			return
		}
		response := []byte{
			header[0], header[1], 0, 0,
			0, byte(3 + len(words)*2), header[6],
			body[0], byte(len(words) * 2),
		}
		for _, word := range words {
			response = append(response, byte(word>>8), byte(word))
		}
		_, err = connection.Write(response)
		done <- err
	}()
	return done
}

func runtimeSourceAndViews(
	t *testing.T,
) (*modbus.RuntimeAcquisitionSource, []modbus.LogicalReadView) {
	return runtimeSourceAndViewsForGeneration(t, 41)
}

func runtimeSourceAndViewsForGeneration(
	t *testing.T,
	pollGeneration uint64,
) (*modbus.RuntimeAcquisitionSource, []modbus.LogicalReadView) {
	t.Helper()
	clock := newRuntimeIntegrationClock()
	source, err := modbus.NewRuntimeAcquisitionSource(runtimeSourceConfig(clock))
	if err != nil {
		t.Fatalf("NewRuntimeAcquisitionSource: %v", err)
	}
	return source, runtimeViewsForSourceGeneration(t, source, pollGeneration)
}

func runtimeViewsForSource(
	t *testing.T,
	source *modbus.RuntimeAcquisitionSource,
) []modbus.LogicalReadView {
	return runtimeViewsForSourceGeneration(t, source, 41)
}

func runtimeViewsForSourceGeneration(
	t *testing.T,
	source *modbus.RuntimeAcquisitionSource,
	pollGeneration uint64,
) []modbus.LogicalReadView {
	t.Helper()
	clock := newRuntimeIntegrationClock()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	done := serveOneModbusResponse(t, listener, []uint16{0x0102, 0x0304, 0x0506})
	endpoint, err := modbus.NewTCPEndpoint(runtimeEndpointConfig(
		"tcp://"+listener.Addr().String(),
		clock,
		source,
	))
	if err != nil {
		t.Fatalf("NewTCPEndpoint: %v", err)
	}
	defer func() { _ = endpoint.Close() }()
	connection, err := net.Dial("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	handle, err := endpoint.OpenConnection(connection)
	if err != nil {
		t.Fatalf("OpenConnection: %v", err)
	}
	first, err := modbus.NewReadRegistersRequest(
		modbus.FunctionReadInputRegisters,
		100,
		2,
	)
	if err != nil {
		t.Fatalf("NewReadRegistersRequest(first): %v", err)
	}
	second, err := modbus.NewReadRegistersRequest(
		modbus.FunctionReadInputRegisters,
		101,
		2,
	)
	if err != nil {
		t.Fatalf("NewReadRegistersRequest(second): %v", err)
	}
	if _, err := endpoint.EnqueueRead(modbus.TCPReadPlan{
		Connection:         handle,
		UnitID:             1,
		AuthorizationScope: "read-only",
		PollGeneration:     pollGeneration,
		DeadlineIdentity:   12,
		Timeout:            time.Second,
		Reads: []modbus.TCPLogicalRead{
			{LogicalViewID: 1001, Request: first},
			{LogicalViewID: 1002, Request: second},
		},
	}); err != nil {
		t.Fatalf("EnqueueRead: %v", err)
	}
	dispatch, ok := endpoint.Dispatch()
	if !ok {
		t.Fatal("runtime read was not dispatched")
	}
	if _, err := endpoint.Write(context.Background(), dispatch); err != nil {
		t.Fatalf("Write: %v", err)
	}
	batch, err := endpoint.Read(context.Background(), handle)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("server: %v", err)
	}
	if len(batch.Views) != 2 {
		t.Fatalf("runtime views=%d want=2", len(batch.Views))
	}
	return batch.Views
}

func runtimeNormalizations(
	t *testing.T,
	source *modbus.RuntimeAcquisitionSource,
	views []modbus.LogicalReadView,
) ([]modbus.RuntimeNormalizationRecord, [][]byte) {
	t.Helper()
	normalizations := make([]modbus.RuntimeNormalizationRecord, len(views))
	normalizationBytes := make([][]byte, len(views))
	for index := range views {
		normalizationBytes[index] = []byte(fmt.Sprintf(
			`{"schema_version":1,"source_kind":"runtime","source_evidence_id":"urn:helianthus:evidence:example-register-map","documentary_notation":"one-based input register","documentary_address":%d,"documentary_address_base":"one_based_register","function_code":4,"logical_table":"input_registers","normalized_zero_based_pdu_offset":%d,"word_count":2,"future_extension":{"canary":"normalization-secret-%d","order":[3,1,2]}}`,
			101+index,
			100+index,
			index,
		))
		var err error
		normalizations[index], err = source.ParseNormalizationRecord(normalizationBytes[index])
		if err != nil {
			t.Fatalf("ParseNormalizationRecord(%d): %v", index, err)
		}
	}
	return normalizations, normalizationBytes
}

func issueRuntimeDependencies(
	t *testing.T,
	source *modbus.RuntimeAcquisitionSource,
	attempt *reg.ObservationAttempt,
	views []modbus.LogicalReadView,
) [][]byte {
	t.Helper()
	normalizations, normalizationBytes := runtimeNormalizations(t, source, views)
	for index, view := range views {
		err := attempt.Issue(uint32(index), view, normalizations[index])
		if err != nil {
			t.Fatalf("Issue(%d): %v", index, err)
		}
	}
	return normalizationBytes

}

func issueDirectRuntimeAcquisitions(
	t *testing.T,
	source *modbus.RuntimeAcquisitionSource,
	producerAttempt *modbus.RuntimeAttempt,
	views []modbus.LogicalReadView,
) []modbus.RuntimeAcquisition {
	t.Helper()
	normalizations, _ := runtimeNormalizations(t, source, views)
	acquisitions := make([]modbus.RuntimeAcquisition, len(views))
	for index, view := range views {
		var err error
		acquisitions[index], err = source.Issue(
			producerAttempt,
			uint32(index),
			view,
			normalizations[index],
		)
		if err != nil {
			t.Fatalf("direct Issue(%d): %v", index, err)
		}
	}
	return acquisitions
}

func runtimeAttemptRequestForTest(
	source *modbus.RuntimeAcquisitionSource,
	key string,
	pollGeneration uint64,
	dependencyCount int,
) reg.RuntimeAttemptRequest {
	dependencies := make([]reg.RuntimeDependencyFacts, dependencyCount)
	for index := range dependencies {
		dependencies[index].SourceTime = reg.SourceTimeUnavailable()
	}
	return reg.RuntimeAttemptRequest{
		Source:     source,
		AttemptKey: key,
		Identity: reg.AttemptIdentity{
			PollGenerationID: pollGeneration,
		},
		Observation: reg.RuntimeObservationFacts{
			SourceValidity:          reg.SourceValid,
			SourceTime:              reg.SourceTimeUnavailable(),
			LocalReceiptTime:        time.Unix(1_700_000_000, 0).UTC(),
			LocalReceiptTimePresent: true,
		},
		Dependencies: dependencies,
	}
}

func runtimeLedgerForTest(
	t *testing.T,
	profile reg.ProfileDescriptor,
	limits reg.LedgerLimits,
) (reg.SampleLedgerState, *reg.SampleLedger) {
	t.Helper()
	initial, err := reg.EmptySampleLedgerState("runtime-ledger", profile)
	if err != nil {
		t.Fatalf("EmptySampleLedgerState: %v", err)
	}
	ledger, err := reg.NewSampleLedger(initial, 0, limits)
	if err != nil {
		t.Fatalf("NewSampleLedger: %v", err)
	}
	return initial, ledger
}

func beginAdmittedRuntimeAttempt(
	t *testing.T,
	factory *reg.ObservationFactory,
	source *modbus.RuntimeAcquisitionSource,
	views []modbus.LogicalReadView,
	key string,
	pollGeneration uint64,
) *reg.ObservationAttempt {
	t.Helper()
	attempt, err := factory.BeginRuntimeAttempt(runtimeAttemptRequestForTest(
		source,
		key,
		pollGeneration,
		len(views),
	))
	if err != nil {
		t.Fatalf("BeginRuntimeAttempt: %v", err)
	}
	issueRuntimeDependencies(t, source, attempt, views)
	if err := attempt.Admit(); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	return attempt
}

type memoryPublicationCommitter struct {
	mu            sync.Mutex
	state         reg.SampleLedgerState
	effects       []reg.PublishedAttemptV1
	requests      []reg.PublicationCommitRequest
	terminalCalls int
	restart       reg.LedgerRestartState
	terminal      []reg.TerminalStateCommitRequest
}

type arbitrationPublicationCommitter struct {
	mu        sync.Mutex
	state     reg.SampleLedgerState
	entered   chan struct{}
	release   chan struct{}
	enterOnce sync.Once
	effects   int
	restart   reg.LedgerRestartState
}

func newArbitrationPublicationCommitter(
	state reg.SampleLedgerState,
) *arbitrationPublicationCommitter {
	return &arbitrationPublicationCommitter{
		state:   state,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (committer *arbitrationPublicationCommitter) CommitPublication(
	ctx context.Context,
	request reg.PublicationCommitRequest,
) (reg.PublicationCommitDecision, error) {
	committer.enterOnce.Do(func() { close(committer.entered) })
	select {
	case <-ctx.Done():
		return reg.PublicationCommitCancelled, nil
	case <-committer.release:
		committer.mu.Lock()
		defer committer.mu.Unlock()
		if committer.state != request.ExpectedState {
			return "", fmt.Errorf("publication state conflict")
		}
		if committer.restart.SchemaVersion == 0 {
			committer.restart = request.ExpectedRestartState
		}
		if !reflect.DeepEqual(committer.restart, request.ExpectedRestartState) &&
			!committer.restart.CoversTerminalWatermark(request.PublishedRestartState) {
			return "", fmt.Errorf("publication restart state conflict")
		}
		committer.state = request.PublishedState
		if !committer.restart.CoversTerminalWatermark(request.PublishedRestartState) {
			committer.restart = request.PublishedRestartState
		}
		committer.effects++
		return reg.PublicationCommitCommitted, nil
	}
}

func (committer *arbitrationPublicationCommitter) effectCount() int {
	committer.mu.Lock()
	defer committer.mu.Unlock()
	return committer.effects
}

func (committer *arbitrationPublicationCommitter) CommitTerminalState(
	ctx context.Context,
	request reg.TerminalStateCommitRequest,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	committer.mu.Lock()
	defer committer.mu.Unlock()
	if committer.restart.SchemaVersion == 0 {
		committer.restart = request.ExpectedRestartState
	}
	if !reflect.DeepEqual(committer.restart, request.ExpectedRestartState) &&
		!committer.restart.CoversTerminalWatermark(request.TerminalRestartState) {
		return fmt.Errorf("terminal restart state conflict")
	}
	if !committer.restart.CoversTerminalWatermark(request.TerminalRestartState) {
		committer.restart = request.TerminalRestartState
	}
	return nil
}

func (committer *memoryPublicationCommitter) CommitPublication(
	ctx context.Context,
	request reg.PublicationCommitRequest,
) (reg.PublicationCommitDecision, error) {
	if err := ctx.Err(); err != nil {
		return reg.PublicationCommitCancelled, nil
	}
	committer.mu.Lock()
	defer committer.mu.Unlock()
	if committer.state != request.ExpectedState {
		return "", fmt.Errorf("publication state conflict")
	}
	if committer.restart.SchemaVersion == 0 {
		committer.restart = request.ExpectedRestartState
	}
	if !reflect.DeepEqual(committer.restart, request.ExpectedRestartState) &&
		!committer.restart.CoversTerminalWatermark(request.PublishedRestartState) {
		return "", fmt.Errorf("publication restart state conflict")
	}
	if err := ctx.Err(); err != nil {
		return reg.PublicationCommitCancelled, nil
	}
	committer.state = request.PublishedState
	committer.effects = append(committer.effects, request.Attempt)
	committer.requests = append(committer.requests, request)
	if !committer.restart.CoversTerminalWatermark(request.PublishedRestartState) {
		committer.restart = request.PublishedRestartState
	}
	return reg.PublicationCommitCommitted, nil
}

func (committer *memoryPublicationCommitter) CommitTerminalState(
	ctx context.Context,
	request reg.TerminalStateCommitRequest,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	committer.mu.Lock()
	defer committer.mu.Unlock()
	if committer.restart.SchemaVersion == 0 {
		committer.restart = request.ExpectedRestartState
	}
	if !reflect.DeepEqual(committer.restart, request.ExpectedRestartState) &&
		!committer.restart.CoversTerminalWatermark(request.TerminalRestartState) {
		return fmt.Errorf("terminal restart state conflict")
	}
	if !committer.restart.CoversTerminalWatermark(request.TerminalRestartState) {
		committer.restart = request.TerminalRestartState
	}
	committer.terminalCalls++
	committer.terminal = append(committer.terminal, request)
	return nil
}

func TestPublicationCommitCarriesExactFinalObservationAndRestartState(t *testing.T) {
	profile := profileFixture(t)
	source, views := runtimeSourceAndViews(t)
	initial, ledger := runtimeLedgerForTest(t, profile, reg.DefaultLedgerLimits())
	committer := &memoryPublicationCommitter{state: initial}
	factory, err := reg.NewObservationFactory(profile, ledger, committer)
	if err != nil {
		t.Fatalf("NewObservationFactory: %v", err)
	}
	attempt, err := factory.BeginRuntimeAttempt(runtimeAttemptRequestForTest(
		source,
		"publication-crash-boundary",
		41,
		len(views),
	))
	if err != nil {
		t.Fatalf("BeginRuntimeAttempt: %v", err)
	}
	normalizationBytes := issueRuntimeDependencies(t, source, attempt, views)
	if err := attempt.Admit(); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	for ordinal := range views {
		if outcome, err := attempt.Claim(uint64(ordinal)); err != nil ||
			outcome != reg.ClaimSucceeded {
			t.Fatalf("Claim(%d)=(%q, %v)", ordinal, outcome, err)
		}
	}
	if err := attempt.Seal(); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	published, err := attempt.Publish(context.Background())
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	committer.mu.Lock()
	if len(committer.requests) != 1 {
		committer.mu.Unlock()
		t.Fatalf("publication requests=%d", len(committer.requests))
	}
	request := committer.requests[0]
	committer.mu.Unlock()
	committedObservation := request.Observation
	if committedObservation.SampleID() != published.SampleID() ||
		!reflect.DeepEqual(committedObservation.Spec(), published.Spec()) {
		t.Fatal("committed final observation envelope differs from the published result")
	}
	committedReplay := committedObservation.Replay()
	publishedReplay := published.Replay()
	if len(committedReplay) != len(publishedReplay) ||
		len(committedReplay) != len(normalizationBytes) {
		t.Fatal("committed observation dependency cardinality changed")
	}
	for index := range committedReplay {
		if !reflect.DeepEqual(
			committedReplay[index].LogicalViewRecord(),
			publishedReplay[index].LogicalViewRecord(),
		) || !bytes.Equal(
			committedReplay[index].RuntimeNormalizationBytes(),
			normalizationBytes[index],
		) {
			t.Fatalf("committed dependency %d lost exact provenance", index)
		}
	}
	if request.PublishedRestartState.NextTerminalSequence == 0 {
		t.Fatal("atomic publication request omits terminal restart state")
	}
	restored, err := reg.NewSampleLedgerFromRestart(
		request.PublishedState,
		request.PublishedState.Revision,
		reg.DefaultLedgerLimits(),
		request.PublishedRestartState,
	)
	if err != nil {
		t.Fatalf("crash-boundary publication restart: %v", err)
	}
	if got := restored.Snapshot().NextTerminalSequence; got != request.PublishedRestartState.NextTerminalSequence {
		t.Fatalf("restored terminal watermark=%d", got)
	}
	restartCommitter := &memoryPublicationCommitter{
		state:   request.PublishedState,
		restart: request.PublishedRestartState,
	}
	restoredFactory, err := reg.NewObservationFactory(profile, restored, restartCommitter)
	if err != nil {
		t.Fatalf("NewObservationFactory(restored): %v", err)
	}
	restartSource, restartViews := runtimeSourceAndViews(t)
	restartedAttempt, err := restoredFactory.BeginRuntimeAttempt(
		runtimeAttemptRequestForTest(
			restartSource,
			"post-publication-crash",
			42,
			len(restartViews),
		),
	)
	if err != nil {
		t.Fatalf("BeginRuntimeAttempt(restored): %v", err)
	}
	if result, err := restartedAttempt.Cancel(); err != nil ||
		result != reg.CancellationCompleted {
		t.Fatalf("Cancel(restored)=(%q, %v)", result, err)
	}
	if got := restartCommitter.terminal[0].Attempt.AttemptTerminalSequence; got != request.PublishedRestartState.NextTerminalSequence {
		t.Fatalf("post-crash terminal sequence=%d reused committed history", got)
	}
}

func TestCancellationCrossesDurableTerminalBoundary(t *testing.T) {
	profile := profileFixture(t)
	source, views := runtimeSourceAndViews(t)
	initial, ledger := runtimeLedgerForTest(t, profile, reg.DefaultLedgerLimits())
	committer := &memoryPublicationCommitter{state: initial}
	factory, err := reg.NewObservationFactory(profile, ledger, committer)
	if err != nil {
		t.Fatalf("NewObservationFactory: %v", err)
	}
	attempt, err := factory.BeginRuntimeAttempt(runtimeAttemptRequestForTest(
		source,
		"cancellation-crash-boundary",
		41,
		len(views),
	))
	if err != nil {
		t.Fatalf("BeginRuntimeAttempt: %v", err)
	}
	if result, err := attempt.Cancel(); err != nil || result != reg.CancellationCompleted {
		t.Fatalf("Cancel=(%q, %v)", result, err)
	}
	committer.mu.Lock()
	terminalCalls := committer.terminalCalls
	terminalRequests := append([]reg.TerminalStateCommitRequest(nil), committer.terminal...)
	committer.mu.Unlock()
	if terminalCalls != 1 {
		t.Fatalf("durable cancellation terminal commits=%d, want 1", terminalCalls)
	}
	terminal := terminalRequests[0]
	if terminal.Outcome != reg.AttemptCancelled {
		t.Fatalf("durable cancellation outcome=%q", terminal.Outcome)
	}
	restored, err := reg.NewSampleLedgerFromRestart(
		initial,
		0,
		reg.DefaultLedgerLimits(),
		terminal.TerminalRestartState,
	)
	if err != nil {
		t.Fatalf("crash-boundary cancellation restart: %v", err)
	}
	if got := restored.Snapshot().NextTerminalSequence; got != terminal.TerminalRestartState.NextTerminalSequence {
		t.Fatalf("restored cancellation watermark=%d", got)
	}
	restartCommitter := &memoryPublicationCommitter{
		state:   initial,
		restart: terminal.TerminalRestartState,
	}
	restoredFactory, err := reg.NewObservationFactory(profile, restored, restartCommitter)
	if err != nil {
		t.Fatalf("NewObservationFactory(restored): %v", err)
	}
	restartSource, restartViews := runtimeSourceAndViews(t)
	restartedAttempt, err := restoredFactory.BeginRuntimeAttempt(
		runtimeAttemptRequestForTest(
			restartSource,
			"post-cancellation-crash",
			42,
			len(restartViews),
		),
	)
	if err != nil {
		t.Fatalf("BeginRuntimeAttempt(restored): %v", err)
	}
	if result, err := restartedAttempt.Cancel(); err != nil ||
		result != reg.CancellationCompleted {
		t.Fatalf("Cancel(restored)=(%q, %v)", result, err)
	}
	if got := restartCommitter.terminal[0].Attempt.AttemptTerminalSequence; got != terminal.TerminalRestartState.NextTerminalSequence {
		t.Fatalf("post-crash terminal sequence=%d reused cancelled history", got)
	}
}

func TestRuntimeAttemptClaimsProducerCapabilitiesAndRetainsExactNormalization(
	t *testing.T,
) {
	profile := profileFixture(t)
	source, views := runtimeSourceAndViews(t)
	initial, err := reg.EmptySampleLedgerState("runtime-ledger", profile)
	if err != nil {
		t.Fatalf("EmptySampleLedgerState: %v", err)
	}
	ledger, err := reg.NewSampleLedger(initial, 0, reg.DefaultLedgerLimits())
	if err != nil {
		t.Fatalf("NewSampleLedger: %v", err)
	}
	committer := &memoryPublicationCommitter{state: initial}
	factory, err := reg.NewObservationFactory(profile, ledger, committer)
	if err != nil {
		t.Fatalf("NewObservationFactory: %v", err)
	}
	attempt, err := factory.BeginRuntimeAttempt(reg.RuntimeAttemptRequest{
		Source:     source,
		AttemptKey: "poll-41",
		Identity:   reg.AttemptIdentity{PollGenerationID: 41},
		Observation: reg.RuntimeObservationFacts{
			SourceValidity:          reg.SourceValid,
			SourceTime:              reg.SourceTimeUnavailable(),
			LocalReceiptTime:        time.Unix(1_700_000_000, 0).UTC(),
			LocalReceiptTimePresent: true,
		},
		Dependencies: []reg.RuntimeDependencyFacts{
			{SourceTime: reg.SourceTimeUnavailable()},
			{SourceTime: reg.SourceTimeUnavailable()},
		},
		Diagnostics: []string{"runtime admission validated"},
	})
	if err != nil {
		t.Fatalf("BeginRuntimeAttempt: %v", err)
	}
	copied := *attempt
	if duplicate, err := factory.BeginRuntimeAttempt(reg.RuntimeAttemptRequest{
		Source:       source,
		AttemptKey:   "poll-41",
		Identity:     reg.AttemptIdentity{PollGenerationID: 42},
		Observation:  reg.RuntimeObservationFacts{SourceValidity: reg.SourceValid},
		Dependencies: make([]reg.RuntimeDependencyFacts, 2),
	}); err == nil || duplicate != nil {
		t.Fatal("duplicate retained AttemptKey was admitted")
	}
	normalizationBytes := issueRuntimeDependencies(t, source, attempt, views)
	normalizations, _ := runtimeNormalizations(t, source, views)
	if err := copied.Issue(0, views[0], normalizations[0]); err == nil {
		t.Fatal("copied attempt issued the same dependency ordinal twice")
	}
	if err := attempt.Admit(); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	if err := copied.Admit(); err == nil {
		t.Fatal("copied attempt admitted the same private set twice")
	}
	for ordinal := uint64(0); ordinal < 2; ordinal++ {
		outcome, err := copied.Claim(ordinal)
		if err != nil || outcome != reg.ClaimSucceeded {
			t.Fatalf("Claim(%d)=(%q, %v)", ordinal, outcome, err)
		}
	}
	if outcome, err := attempt.Claim(0); err == nil || outcome != reg.ClaimSucceeded {
		t.Fatalf("second claim=(%q, %v), want immutable first outcome plus rejection", outcome, err)
	}
	if err := attempt.Seal(); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	observation, err := attempt.Publish(context.Background())
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if observation.SampleID() != "runtime-ledger:1" {
		t.Fatalf("SampleID=%q", observation.SampleID())
	}
	replayed := observation.Replay()
	if len(replayed) != len(normalizationBytes) {
		t.Fatalf("replayed dependencies=%d", len(replayed))
	}
	for index := range replayed {
		retained := replayed[index].RuntimeNormalizationBytes()
		if !bytes.Equal(retained, normalizationBytes[index]) {
			t.Fatalf("normalization %d changed:\n got %s\nwant %s", index, replayed[index].RuntimeNormalizationBytes(), normalizationBytes[index])
		}
		retained[0] ^= 0xff
		if !bytes.Equal(replayed[index].RuntimeNormalizationBytes(), normalizationBytes[index]) {
			t.Fatalf("normalization %d exposed mutable retained bytes", index)
		}
		if len(replayed[index].LogicalViewRecord().WireResponseBytes) == 0 {
			t.Fatalf("dependency %d lost exact wire response bytes", index)
		}
	}
	if len(committer.effects) != 1 {
		t.Fatalf("external effects=%d want=1", len(committer.effects))
	}
	encoded, err := json.Marshal(committer.effects[0])
	if err != nil {
		t.Fatalf("Marshal(PublishedAttemptV1): %v", err)
	}
	if bytes.Contains(encoded, []byte("normalization-secret-0")) {
		t.Fatalf("published projection leaked normalization canary: %s", encoded)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("Unmarshal projection: %v", err)
	}
	if len(fields) != 5 {
		t.Fatalf("published projection fields=%d want=5: %s", len(fields), encoded)
	}
	snapshot := source.Snapshot()
	if snapshot.LiveCapabilities != 0 || snapshot.ActiveAttempts != 0 {
		t.Fatalf("producer state was not synchronously drained: %+v", snapshot)
	}
}

func TestRuntimeAdmissionRejectsForeignMembershipAndEmptyAttempts(t *testing.T) {
	t.Run("different source", func(t *testing.T) {
		profile := profileFixture(t)
		initial, ledger := runtimeLedgerForTest(t, profile, reg.DefaultLedgerLimits())
		factory, err := reg.NewObservationFactory(
			profile,
			ledger,
			&memoryPublicationCommitter{state: initial},
		)
		if err != nil {
			t.Fatalf("NewObservationFactory: %v", err)
		}
		ownerSource, ownerViews := runtimeSourceAndViews(t)
		foreignSource, foreignViews := runtimeSourceAndViews(t)
		owner, err := factory.BeginRuntimeAttempt(runtimeAttemptRequestForTest(
			ownerSource,
			"owner-source",
			41,
			len(ownerViews),
		))
		if err != nil {
			t.Fatalf("BeginRuntimeAttempt(owner): %v", err)
		}
		foreignNormalizations, _ := runtimeNormalizations(
			t,
			foreignSource,
			foreignViews,
		)
		if err := owner.Issue(0, foreignViews[0], foreignNormalizations[0]); err == nil {
			t.Fatal("foreign source view and normalization entered retained issuance")
		}
		issueRuntimeDependencies(t, ownerSource, owner, ownerViews)
		if err := owner.Admit(); err != nil {
			t.Fatalf("Admit(owner): %v", err)
		}
		if result, err := owner.Cancel(); err != nil || result != reg.CancellationCompleted {
			t.Fatalf("Cancel(owner)=(%q, %v)", result, err)
		}
		for name, source := range map[string]*modbus.RuntimeAcquisitionSource{
			"owner":   ownerSource,
			"foreign": foreignSource,
		} {
			snapshot := source.Snapshot()
			if snapshot.LiveCapabilities != 0 || snapshot.ActiveAttempts != 0 {
				t.Fatalf("%s source was not drained: %+v", name, snapshot)
			}
		}
		if snapshot := ledger.Snapshot(); snapshot.RetainedAttempts != 0 ||
			snapshot.RetainedClaimEntries != 0 {
			t.Fatalf("failed membership was retained: %+v", snapshot)
		}
	})

	t.Run("different attempt on same source", func(t *testing.T) {
		profile := profileFixture(t)
		initial, ledger := runtimeLedgerForTest(t, profile, reg.DefaultLedgerLimits())
		factory, err := reg.NewObservationFactory(
			profile,
			ledger,
			&memoryPublicationCommitter{state: initial},
		)
		if err != nil {
			t.Fatalf("NewObservationFactory: %v", err)
		}
		source, views := runtimeSourceAndViews(t)
		owner, err := factory.BeginRuntimeAttempt(runtimeAttemptRequestForTest(
			source,
			"owner-attempt",
			41,
			len(views),
		))
		if err != nil {
			t.Fatalf("BeginRuntimeAttempt(owner): %v", err)
		}
		foreignAttempt, err := source.BeginAttempt("foreign-attempt")
		if err != nil {
			t.Fatalf("BeginAttempt(foreign): %v", err)
		}
		foreignViews := runtimeViewsForSource(t, source)
		foreign := issueDirectRuntimeAcquisitions(t, source, foreignAttempt, foreignViews)
		if err := owner.Admit(); err == nil {
			t.Fatal("a separately issued attempt populated the owner's private set")
		}
		foreignInstance, err := foreignAttempt.Close(foreign)
		if err != nil {
			t.Fatalf("Close(foreign): %v", err)
		}
		if err := source.CancelOpen(foreignInstance); err != nil {
			t.Fatalf("CancelOpen(foreign): %v", err)
		}
		issueRuntimeDependencies(t, source, owner, views)
		if err := owner.Admit(); err != nil {
			t.Fatalf("Admit(owner): %v", err)
		}
		if result, err := owner.Cancel(); err != nil || result != reg.CancellationCompleted {
			t.Fatalf("Cancel(owner)=(%q, %v)", result, err)
		}
		snapshot := source.Snapshot()
		if snapshot.LiveCapabilities != 0 || snapshot.ActiveAttempts != 0 {
			t.Fatalf("source was not drained: %+v", snapshot)
		}
		if snapshot := ledger.Snapshot(); snapshot.RetainedAttempts != 0 ||
			snapshot.RetainedClaimEntries != 0 {
			t.Fatalf("ledger retained separate attempts: %+v", snapshot)
		}
	})

	t.Run("empty admission", func(t *testing.T) {
		profile := profileFixture(t)
		initial, ledger := runtimeLedgerForTest(t, profile, reg.DefaultLedgerLimits())
		factory, err := reg.NewObservationFactory(
			profile,
			ledger,
			&memoryPublicationCommitter{state: initial},
		)
		if err != nil {
			t.Fatalf("NewObservationFactory: %v", err)
		}
		source, _ := runtimeSourceAndViews(t)
		attempt, err := factory.BeginRuntimeAttempt(runtimeAttemptRequestForTest(
			source,
			"empty-attempt",
			41,
			2,
		))
		if err != nil {
			t.Fatalf("BeginRuntimeAttempt: %v", err)
		}
		if err := attempt.Admit(); err == nil {
			t.Fatal("empty producer attempt entered production")
		}
		if attempt.Phase() != reg.AttemptOpen {
			t.Fatalf("incomplete admission changed phase to %q", attempt.Phase())
		}
		if result, err := attempt.Cancel(); err != nil || result != reg.CancellationCompleted {
			t.Fatalf("Cancel(empty)=(%q, %v)", result, err)
		}
		if attempt.Phase() != reg.AttemptCancelled {
			t.Fatalf("empty attempt terminal phase = %q", attempt.Phase())
		}
		snapshot := source.Snapshot()
		if snapshot.LiveCapabilities != 0 || snapshot.ActiveAttempts != 0 {
			t.Fatalf("empty producer attempt was not drained: %+v", snapshot)
		}
	})
}

func TestRuntimeLedgerCapacityReclaimsReusesAndRestartsDeterministically(
	t *testing.T,
) {
	profile := profileFixture(t)
	limits := reg.DefaultLedgerLimits()
	limits.MaxRetainedAttempts = 1
	limits.MaxClaimEntriesPerAttempt = 2
	limits.MaxRetainedClaimEntries = 2
	limits.AuditTombstoneLimit = 3
	initial, ledger := runtimeLedgerForTest(t, profile, limits)
	committer := &memoryPublicationCommitter{state: initial}
	factory, err := reg.NewObservationFactory(profile, ledger, committer)
	if err != nil {
		t.Fatalf("NewObservationFactory: %v", err)
	}
	source, _ := runtimeSourceAndViews(t)
	first, err := factory.BeginRuntimeAttempt(runtimeAttemptRequestForTest(
		source,
		"capacity-key",
		41,
		2,
	))
	if err != nil {
		t.Fatalf("BeginRuntimeAttempt(first): %v", err)
	}
	if duplicate, err := factory.BeginRuntimeAttempt(runtimeAttemptRequestForTest(
		source,
		"capacity-key",
		42,
		2,
	)); err == nil || duplicate != nil {
		t.Fatal("duplicate live AttemptKey was admitted")
	}
	if overflow, err := factory.BeginRuntimeAttempt(runtimeAttemptRequestForTest(
		source,
		"capacity-overflow",
		42,
		2,
	)); err == nil || overflow != nil {
		t.Fatal("retained-attempt capacity was exceeded")
	}
	if snapshot := source.Snapshot(); snapshot.ActiveAttempts != 1 {
		t.Fatalf("rejected ledger admission began an M1 attempt: %+v", snapshot)
	}
	if _, err := ledger.ExportRestartState(); err == nil {
		t.Fatal("live capabilities were exported as restartable state")
	}
	if result, err := first.Cancel(); err != nil || result != reg.CancellationCompleted {
		t.Fatalf("Cancel(first)=(%q, %v)", result, err)
	}
	if snapshot := ledger.Snapshot(); snapshot.RetainedAttempts != 0 ||
		snapshot.RetainedClaimEntries != 0 || len(snapshot.AuditTombstones) != 3 {
		t.Fatalf("first terminal state was not synchronously reclaimed: %+v", snapshot)
	}

	reused, err := factory.BeginRuntimeAttempt(runtimeAttemptRequestForTest(
		source,
		"capacity-key",
		42,
		2,
	))
	if err != nil {
		t.Fatalf("reused reclaimed AttemptKey: %v", err)
	}
	if result, err := reused.Cancel(); err != nil || result != reg.CancellationCompleted {
		t.Fatalf("Cancel(reused)=(%q, %v)", result, err)
	}
	restart, err := ledger.ExportRestartState()
	if err != nil {
		t.Fatalf("ExportRestartState: %v", err)
	}
	if len(restart.AuditTombstones) != limits.AuditTombstoneLimit ||
		restart.NextTerminalSequence != 7 {
		t.Fatalf("bounded restart state = %+v", restart)
	}
	restored, err := reg.NewSampleLedgerFromRestart(initial, 0, limits, restart)
	if err != nil {
		t.Fatalf("NewSampleLedgerFromRestart: %v", err)
	}
	restoredState, err := restored.ExportRestartState()
	if err != nil {
		t.Fatalf("restored ExportRestartState: %v", err)
	}
	if restoredState.NextTerminalSequence != restart.NextTerminalSequence ||
		restoredState.SequenceExhausted != restart.SequenceExhausted ||
		len(restoredState.AuditTombstones) != len(restart.AuditTombstones) {
		t.Fatalf("restart state changed: got %+v want %+v", restoredState, restart)
	}
	for index := range restart.AuditTombstones {
		if restoredState.AuditTombstones[index] != restart.AuditTombstones[index] {
			t.Fatalf("restart tombstone %d changed", index)
		}
	}

	exhaustion := reg.LedgerRestartState{
		SchemaVersion:            1,
		NextTerminalSequence:     math.MaxUint64 - 1,
		TruncatedThroughSequence: math.MaxUint64 - 2,
	}
	exhaustedLedger, err := reg.NewSampleLedgerFromRestart(
		initial,
		0,
		limits,
		exhaustion,
	)
	if err != nil {
		t.Fatalf("NewSampleLedgerFromRestart(exhaustion): %v", err)
	}
	exhaustedFactory, err := reg.NewObservationFactory(
		profile,
		exhaustedLedger,
		&memoryPublicationCommitter{state: initial},
	)
	if err != nil {
		t.Fatalf("NewObservationFactory(exhaustion): %v", err)
	}
	exhaustionSource, _ := runtimeSourceAndViews(t)
	if attempt, err := exhaustedFactory.BeginRuntimeAttempt(
		runtimeAttemptRequestForTest(exhaustionSource, "sequence-exhaustion", 43, 2),
	); err == nil || attempt != nil {
		t.Fatal("partial terminal-sequence batch was reserved")
	}
	exhaustedSnapshot := exhaustedLedger.Snapshot()
	if exhaustedSnapshot.NextTerminalSequence != math.MaxUint64-1 ||
		exhaustedSnapshot.SequenceExhausted ||
		exhaustedSnapshot.RetainedAttempts != 0 {
		t.Fatalf("failed sequence reservation mutated ledger: %+v", exhaustedSnapshot)
	}
	if snapshot := exhaustionSource.Snapshot(); snapshot.ActiveAttempts != 0 {
		t.Fatalf("sequence exhaustion began an M1 attempt: %+v", snapshot)
	}
}

func TestLedgerRestartRejectsMalformedClaimSequenceReservations(t *testing.T) {
	profile := profileFixture(t)
	state := emptyLedgerState(t, profile)
	limits := reg.DefaultLedgerLimits()
	limits.MaxClaimEntriesPerAttempt = 2
	limits.MaxRetainedClaimEntries = 2 * limits.MaxRetainedAttempts

	attempt := func(sequence uint64) reg.LedgerAuditTombstone {
		return reg.LedgerAuditTombstone{
			SchemaVersion:    1,
			ObjectKind:       reg.LedgerAuditAttempt,
			TerminalSequence: sequence,
			TerminalOutcome:  string(reg.AttemptCancelled),
		}
	}
	claim := func(sequence, attemptSequence, ordinal uint64) reg.LedgerAuditTombstone {
		return reg.LedgerAuditTombstone{
			SchemaVersion:           1,
			ObjectKind:              reg.LedgerAuditClaim,
			TerminalSequence:        sequence,
			AttemptTerminalSequence: attemptSequence,
			ClaimOrdinal:            ordinal,
			TerminalOutcome:         string(reg.ClaimAttemptCancelled),
		}
	}

	tests := []struct {
		name       string
		tombstones []reg.LedgerAuditTombstone
		next       uint64
		exhausted  bool
		truncated  uint64
	}{
		{
			name:       "claim sequence does not match reservation",
			tombstones: []reg.LedgerAuditTombstone{attempt(7), claim(9, 7, 0)},
			next:       10,
			truncated:  6,
		},
		{
			name:       "claim ordinal reaches configured bound",
			tombstones: []reg.LedgerAuditTombstone{attempt(7), claim(10, 7, 2)},
			next:       11,
			truncated:  6,
		},
		{
			name:       "claim reservation addition overflows",
			tombstones: []reg.LedgerAuditTombstone{claim(math.MaxUint64, math.MaxUint64, 0)},
			exhausted:  true,
			truncated:  math.MaxUint64 - 1,
		},
		{
			name: "claim reservation crosses future attempt",
			tombstones: []reg.LedgerAuditTombstone{
				attempt(7),
				attempt(8),
				claim(9, 7, 1),
			},
			next:      10,
			truncated: 6,
		},
		{
			name:       "retained history has a sequence gap",
			tombstones: []reg.LedgerAuditTombstone{attempt(7), claim(9, 7, 1)},
			next:       10,
			truncated:  6,
		},
		{
			name: "claim links to a retained claim instead of an attempt",
			tombstones: []reg.LedgerAuditTombstone{
				claim(7, 6, 0),
				claim(8, 7, 0),
			},
			next:      9,
			truncated: 6,
		},
		{
			name:       "empty history has an unexplained watermark",
			tombstones: nil,
			next:       7,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			restart := reg.LedgerRestartState{
				SchemaVersion:            1,
				NextTerminalSequence:     test.next,
				SequenceExhausted:        test.exhausted,
				TruncatedThroughSequence: test.truncated,
				AuditTombstones:          test.tombstones,
			}
			if _, err := reg.NewSampleLedgerFromRestart(
				state,
				0,
				limits,
				restart,
			); err == nil {
				t.Fatal("malformed restart claim reservation was accepted")
			}
		})
	}
}

func TestLedgerRestartClaimSequenceReservationRoundTrip(t *testing.T) {
	profile := profileFixture(t)
	state := emptyLedgerState(t, profile)
	limits := reg.DefaultLedgerLimits()
	limits.MaxClaimEntriesPerAttempt = 2
	limits.MaxRetainedClaimEntries = 2 * limits.MaxRetainedAttempts
	restart := reg.LedgerRestartState{
		SchemaVersion:            1,
		NextTerminalSequence:     10,
		TruncatedThroughSequence: 6,
		AuditTombstones: []reg.LedgerAuditTombstone{
			{
				SchemaVersion:    1,
				ObjectKind:       reg.LedgerAuditAttempt,
				TerminalSequence: 7,
				TerminalOutcome:  string(reg.AttemptCancelled),
			},
			{
				SchemaVersion:           1,
				ObjectKind:              reg.LedgerAuditClaim,
				TerminalSequence:        8,
				AttemptTerminalSequence: 7,
				ClaimOrdinal:            0,
				TerminalOutcome:         string(reg.ClaimAttemptCancelled),
			},
			{
				SchemaVersion:           1,
				ObjectKind:              reg.LedgerAuditClaim,
				TerminalSequence:        9,
				AttemptTerminalSequence: 7,
				ClaimOrdinal:            1,
				TerminalOutcome:         string(reg.ClaimAttemptCancelled),
			},
		},
	}
	ledger, err := reg.NewSampleLedgerFromRestart(state, 0, limits, restart)
	if err != nil {
		t.Fatalf("NewSampleLedgerFromRestart(valid): %v", err)
	}
	got, err := ledger.ExportRestartState()
	if err != nil {
		t.Fatalf("ExportRestartState: %v", err)
	}
	encodedWant, err := json.Marshal(restart)
	if err != nil {
		t.Fatalf("Marshal(want): %v", err)
	}
	encodedGot, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal(got): %v", err)
	}
	if !bytes.Equal(encodedGot, encodedWant) {
		t.Fatalf("restart round trip = %s, want %s", encodedGot, encodedWant)
	}
}

func TestRuntimeAttemptRejectsUnboundedFactsBeforeProducerRetention(t *testing.T) {
	profile := profileFixture(t)
	initial, ledger := runtimeLedgerForTest(t, profile, reg.DefaultLedgerLimits())
	factory, err := reg.NewObservationFactory(
		profile,
		ledger,
		&memoryPublicationCommitter{state: initial},
	)
	if err != nil {
		t.Fatalf("NewObservationFactory: %v", err)
	}
	source, _ := runtimeSourceAndViews(t)
	request := runtimeAttemptRequestForTest(source, "unbounded-facts", 41, 2)
	request.Dependencies[0].DocumentaryConsistencyMarker = strings.Repeat(
		"x",
		reg.MaxContractStringBytes+1,
	)
	if attempt, err := factory.BeginRuntimeAttempt(request); err == nil || attempt != nil {
		t.Fatal("unbounded runtime facts were retained")
	}
	if snapshot := ledger.Snapshot(); snapshot.RetainedAttempts != 0 ||
		snapshot.RetainedClaimEntries != 0 {
		t.Fatalf("invalid facts consumed ledger capacity: %+v", snapshot)
	}
	if snapshot := source.Snapshot(); snapshot.ActiveAttempts != 0 {
		t.Fatalf("invalid facts began an M1 attempt: %+v", snapshot)
	}
}

func TestRuntimeClaimAndCancelRaceHasOneDeterministicDrain(t *testing.T) {
	profile := profileFixture(t)
	initial, ledger := runtimeLedgerForTest(t, profile, reg.DefaultLedgerLimits())
	factory, err := reg.NewObservationFactory(
		profile,
		ledger,
		&memoryPublicationCommitter{state: initial},
	)
	if err != nil {
		t.Fatalf("NewObservationFactory: %v", err)
	}
	source, views := runtimeSourceAndViews(t)
	attempt := beginAdmittedRuntimeAttempt(
		t,
		factory,
		source,
		views,
		"claim-cancel-race",
		41,
	)
	start := make(chan struct{})
	var claimOutcome reg.ClaimOutcome
	var claimErr error
	var cancelResult reg.CancellationResult
	var cancelErr error
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		claimOutcome, claimErr = attempt.Claim(0)
	}()
	go func() {
		defer wait.Done()
		<-start
		cancelResult, cancelErr = attempt.Cancel()
	}()
	close(start)
	wait.Wait()
	if cancelErr != nil || cancelResult != reg.CancellationCompleted {
		t.Fatalf("Cancel=(%q, %v)", cancelResult, cancelErr)
	}
	switch claimOutcome {
	case reg.ClaimSucceeded:
		if claimErr != nil {
			t.Fatalf("successful claim returned %v", claimErr)
		}
	case reg.ClaimAttemptCancelled, "":
		if claimErr == nil {
			t.Fatal("cancelled claim did not report closed admission")
		}
	default:
		t.Fatalf("claim race outcome=(%q, %v)", claimOutcome, claimErr)
	}
	if attempt.Phase() != reg.AttemptCancelled {
		t.Fatalf("phase=%q want cancelled", attempt.Phase())
	}
	snapshot := ledger.Snapshot()
	if snapshot.RetainedAttempts != 0 || snapshot.RetainedClaimEntries != 0 ||
		len(snapshot.AuditTombstones) != 3 {
		t.Fatalf("claim/cancel terminal state was not reclaimed: %+v", snapshot)
	}
	claimOutcomes := make(map[uint64]string)
	for _, tombstone := range snapshot.AuditTombstones {
		if tombstone.ObjectKind == reg.LedgerAuditClaim {
			claimOutcomes[tombstone.ClaimOrdinal] = tombstone.TerminalOutcome
		}
	}
	if got := claimOutcomes[0]; got != string(reg.ClaimSucceeded) &&
		got != string(reg.ClaimAttemptCancelled) {
		t.Fatalf("claim 0 tombstone=%q", got)
	}
	if got := claimOutcomes[1]; got != string(reg.ClaimAttemptCancelled) {
		t.Fatalf("claim 1 tombstone=%q", got)
	}
	producer := source.Snapshot()
	if producer.LiveCapabilities != 0 || producer.ActiveAttempts != 0 {
		t.Fatalf("producer was not drained once: %+v", producer)
	}
}

func sealedRuntimeAttemptForTest(
	t *testing.T,
	committer reg.PublicationCommitter,
) (*reg.ObservationAttempt, *reg.SampleLedger, *modbus.RuntimeAcquisitionSource) {
	t.Helper()
	profile := profileFixture(t)
	initial, ledger := runtimeLedgerForTest(t, profile, reg.DefaultLedgerLimits())
	if memory, ok := committer.(*arbitrationPublicationCommitter); ok {
		memory.mu.Lock()
		memory.state = initial
		memory.mu.Unlock()
	}
	factory, err := reg.NewObservationFactory(profile, ledger, committer)
	if err != nil {
		t.Fatalf("NewObservationFactory: %v", err)
	}
	source, views := runtimeSourceAndViews(t)
	attempt := beginAdmittedRuntimeAttempt(
		t,
		factory,
		source,
		views,
		"publish-cancel",
		41,
	)
	for ordinal := uint64(0); ordinal < uint64(len(views)); ordinal++ {
		outcome, err := attempt.Claim(ordinal)
		if err != nil || outcome != reg.ClaimSucceeded {
			t.Fatalf("Claim(%d)=(%q, %v)", ordinal, outcome, err)
		}
	}
	if err := attempt.Seal(); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	return attempt, ledger, source
}

type publicationResult struct {
	observation reg.Observation
	err         error
}

type cancellationOutcome struct {
	result reg.CancellationResult
	err    error
}

func TestRuntimePublishAndCancelShareOneTransactionalDecision(t *testing.T) {
	t.Run("cancellation wins", func(t *testing.T) {
		committer := newArbitrationPublicationCommitter(reg.SampleLedgerState{})
		attempt, ledger, source := sealedRuntimeAttemptForTest(t, committer)
		published := make(chan publicationResult, 1)
		go func() {
			observation, err := attempt.Publish(context.Background())
			published <- publicationResult{observation: observation, err: err}
		}()
		<-committer.entered
		cancelResult, cancelErr := attempt.Cancel()
		result := <-published
		if result.err == nil || result.observation.SampleID() != "" {
			t.Fatal("cancelled transaction published an observation")
		}
		if cancelErr != nil || cancelResult != reg.CancellationPublishFailed {
			t.Fatalf("Cancel=(%q, %v)", cancelResult, cancelErr)
		}
		if committer.effectCount() != 0 || ledger.ExportState().Revision != 0 ||
			attempt.Phase() != reg.AttemptPublishFailed {
			t.Fatalf("cancelled transaction crossed its commit boundary")
		}
		producer := source.Snapshot()
		if producer.LiveCapabilities != 0 || producer.ActiveAttempts != 0 {
			t.Fatalf("cancelled publication did not drain producer: %+v", producer)
		}
	})

	t.Run("commit wins", func(t *testing.T) {
		committer := newArbitrationPublicationCommitter(reg.SampleLedgerState{})
		attempt, ledger, source := sealedRuntimeAttemptForTest(t, committer)
		published := make(chan publicationResult, 1)
		go func() {
			observation, err := attempt.Publish(context.Background())
			published <- publicationResult{observation: observation, err: err}
		}()
		<-committer.entered
		close(committer.release)
		result := <-published
		cancelResult, cancelErr := attempt.Cancel()
		if result.err != nil || result.observation.SampleID() != "runtime-ledger:1" {
			t.Fatalf("committed publication=(%q, %v)", result.observation.SampleID(), result.err)
		}
		if cancelErr != nil || cancelResult != reg.CancellationAlreadyPublished {
			t.Fatalf("Cancel=(%q, %v)", cancelResult, cancelErr)
		}
		if committer.effectCount() != 1 || ledger.ExportState().Revision != 1 ||
			attempt.Phase() != reg.AttemptPublished {
			t.Fatal("committed transaction did not make one terminal decision")
		}
		producer := source.Snapshot()
		if producer.LiveCapabilities != 0 || producer.ActiveAttempts != 0 {
			t.Fatalf("committed publication did not drain producer: %+v", producer)
		}
	})

	t.Run("simultaneous arbitration", func(t *testing.T) {
		committer := newArbitrationPublicationCommitter(reg.SampleLedgerState{})
		attempt, ledger, _ := sealedRuntimeAttemptForTest(t, committer)
		published := make(chan publicationResult, 1)
		go func() {
			observation, err := attempt.Publish(context.Background())
			published <- publicationResult{observation: observation, err: err}
		}()
		<-committer.entered
		start := make(chan struct{})
		cancelled := make(chan cancellationOutcome, 1)
		go func() {
			<-start
			result, err := attempt.Cancel()
			cancelled <- cancellationOutcome{result: result, err: err}
		}()
		go func() {
			<-start
			close(committer.release)
		}()
		close(start)
		publishResult := <-published
		cancelResult := <-cancelled
		if cancelResult.err != nil {
			t.Fatalf("Cancel: %v", cancelResult.err)
		}
		switch committer.effectCount() {
		case 0:
			if publishResult.err == nil || publishResult.observation.SampleID() != "" ||
				cancelResult.result != reg.CancellationPublishFailed ||
				ledger.ExportState().Revision != 0 ||
				attempt.Phase() != reg.AttemptPublishFailed {
				t.Fatal("cancelled arbitration had a partial commit")
			}
		case 1:
			if publishResult.err != nil || publishResult.observation.SampleID() != "runtime-ledger:1" ||
				cancelResult.result != reg.CancellationAlreadyPublished ||
				ledger.ExportState().Revision != 1 ||
				attempt.Phase() != reg.AttemptPublished {
				t.Fatal("committed arbitration had a partial decision")
			}
		default:
			t.Fatalf("external effects=%d want zero or one", committer.effectCount())
		}
		if _, err := attempt.Publish(context.Background()); err == nil {
			t.Fatal("terminal attempt published twice")
		}
		if committer.effectCount() > 1 {
			t.Fatal("terminal retry repeated the external effect")
		}
	})
}
