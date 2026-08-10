package modbusreg

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func internalDigestDependency(
	t *testing.T,
	id string,
	version string,
	address uint32,
	evidence string,
) Dependency {
	t.Helper()
	dependency, err := NewDependency(DependencySpec{
		ID:      id,
		Version: MustParseVersion(version),
		Table:   InputRegisters,
		Normalization: AddressNormalizationSpec{
			Version:             MustParseVersion("1.0.0"),
			SourceLocator:       "urn:helianthus:evidence:runtime-digest",
			DocumentaryNotation: "one-based input register",
			DocumentaryBase:     AddressBaseOneBased,
			AddressSpaceLabel:   "input_registers",
			DocumentaryAddress:  address,
			Transformation:      TransformSubtractOne,
			ResolvedPDUOffset:   uint16(address - 1),
		},
		WordCount:          2,
		CodecID:            "u32-energy",
		CodecVersion:       MustParseVersion("1.0.0"),
		CoherenceGroup:     "sample",
		EvidenceReferences: []string{evidence},
		ApplicabilityRefs:  []string{"applicability-v1"},
	})
	if err != nil {
		t.Fatalf("NewDependency(%s): %v", id, err)
	}
	return dependency
}

func internalDigestSet(t *testing.T, dependencies []Dependency) DependencySet {
	t.Helper()
	set, err := NewDependencySet(MustParseVersion("1.0.0"), dependencies)
	if err != nil {
		t.Fatalf("NewDependencySet: %v", err)
	}
	return set
}

func TestRuntimeDependencySetDigestCanonicalGoldenVector(t *testing.T) {
	first := internalDigestDependency(t, "energy-a", "1.0.0", 101, "evidence-a")
	second := internalDigestDependency(t, "energy-b", "2.3.4", 102, "evidence-b")
	set := internalDigestSet(t, []Dependency{first, second})
	encoded, digest, err := encodeRuntimeDependencySet(set, MaxSerializedContractBytes)
	if err != nil {
		t.Fatalf("encodeRuntimeDependencySet: %v", err)
	}
	const expectedEncodingHex = "68656c69616e746875732e6d6f646275737265672e72756e74696d652d646570656e64656e742d6964656e7469746965732f76310000000000000000020000000000000008656e657267792d610000000000000005312e302e300000000000000008656e657267792d620000000000000005322e332e34"
	const expectedDigest = "sha256:880496187aaa1cd891dfae90d23dd9823cb11e866605a3940c99ef5a6374913a"
	if got := hex.EncodeToString(encoded); got != expectedEncodingHex {
		t.Fatalf("canonical encoding = %s", got)
	}
	if digest != expectedDigest {
		t.Fatalf("digest = %q", digest)
	}

	permuted := internalDigestSet(t, []Dependency{second, first})
	_, permutedDigest, err := encodeRuntimeDependencySet(
		permuted,
		MaxSerializedContractBytes,
	)
	if err != nil {
		t.Fatalf("encodeRuntimeDependencySet(permuted): %v", err)
	}
	if permutedDigest == digest {
		t.Fatal("dependency permutation did not change the digest")
	}

	metadataOnly := internalDigestSet(t, []Dependency{
		internalDigestDependency(t, "energy-a", "1.0.0", 101, "replacement-a"),
		internalDigestDependency(t, "energy-b", "2.3.4", 102, "replacement-b"),
	})
	_, metadataDigest, err := encodeRuntimeDependencySet(
		metadataOnly,
		MaxSerializedContractBytes,
	)
	if err != nil {
		t.Fatalf("encodeRuntimeDependencySet(metadata): %v", err)
	}
	if metadataDigest != digest {
		t.Fatalf("metadata-only change altered digest: %q != %q", metadataDigest, digest)
	}
}

func TestLedgerAuditTombstoneStrictJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		tombstone LedgerAuditTombstone
		expected  string
	}{
		{
			name: "attempt",
			tombstone: LedgerAuditTombstone{
				SchemaVersion:    1,
				ObjectKind:       LedgerAuditAttempt,
				TerminalSequence: 7,
				ClaimCount:       1,
				TerminalOutcome:  string(AttemptCancelled),
			},
			expected: `{"schema_version":1,"object_kind":"attempt","terminal_sequence":7,"claim_count":1,"terminal_outcome":"cancelled"}`,
		},
		{
			name: "claim ordinal zero",
			tombstone: LedgerAuditTombstone{
				SchemaVersion:           1,
				ObjectKind:              LedgerAuditClaim,
				TerminalSequence:        8,
				AttemptTerminalSequence: 7,
				ClaimOrdinal:            0,
				TerminalOutcome:         string(ClaimSucceeded),
			},
			expected: `{"schema_version":1,"object_kind":"claim","terminal_sequence":8,"attempt_terminal_sequence":7,"claim_ordinal":0,"terminal_outcome":"claim_succeeded"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.tombstone)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(encoded) != test.expected {
				t.Fatalf("encoding = %s", encoded)
			}
			var decoded LedgerAuditTombstone
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if decoded != test.tombstone {
				t.Fatalf("round trip = %+v", decoded)
			}
		})
	}

	invalid := []string{
		`{"schema_version":1,"object_kind":"attempt","terminal_sequence":7}`,
		`{"schema_version":1,"object_kind":"attempt","terminal_sequence":7,"terminal_outcome":"cancelled","unknown":true}`,
		`{"schema_version":1,"object_kind":"claim","terminal_sequence":8,"attempt_terminal_sequence":7,"terminal_outcome":"claim_succeeded"}`,
		`{"schema_version":1,"object_kind":"attempt","terminal_sequence":7,"terminal_outcome":"` + strings.Repeat("x", 257) + `"}`,
	}
	for index, encoded := range invalid {
		var decoded LedgerAuditTombstone
		if err := json.Unmarshal([]byte(encoded), &decoded); err == nil {
			t.Fatalf("invalid tombstone %d was accepted", index)
		}
	}
}

type internalTerminalCommitter struct{}

func (internalTerminalCommitter) CommitPublication(
	context.Context,
	PublicationCommitRequest,
) (PublicationCommitDecision, error) {
	return PublicationCommitCancelled, errors.New("publication is not used")
}

func (internalTerminalCommitter) CommitTerminalState(
	context.Context,
	TerminalStateCommitRequest,
) error {
	return nil
}

type retryJoinTerminalCommitter struct {
	internalTerminalCommitter
	mu           sync.Mutex
	calls        int
	retryEntered chan struct{}
	retryRelease chan struct{}
}

type terminalVisibilityLocker struct {
	mu       *sync.Mutex
	once     sync.Once
	onLocked func()
}

func (locker *terminalVisibilityLocker) Lock() {
	locker.mu.Lock()
	locker.once.Do(locker.onLocked)
}

func (locker *terminalVisibilityLocker) Unlock() {
	locker.mu.Unlock()
}

type internalRuntimeClock struct{}

func (internalRuntimeClock) Now() time.Duration { return 0 }

func internalRuntimeSource(t *testing.T) *modbus.RuntimeAcquisitionSource {
	t.Helper()
	source, err := modbus.NewRuntimeAcquisitionSource(modbus.RuntimeAcquisitionConfig{
		Limits: modbus.RuntimeAcquisitionLimits{
			MaxLiveCapabilities:                        2,
			MaxAttempts:                                2,
			MaxMembersPerAttempt:                       1,
			AttemptKeyMaxUTF8Bytes:                     64,
			SourceEvidenceIDMaxUTF8Bytes:               64,
			NormalizationRecordMaxEncodedBytes:         256,
			NormalizationRequiredStringMaxUTF8Bytes:    64,
			NormalizationExtensionCountMax:             1,
			NormalizationExtensionKeyMaxUTF8Bytes:      64,
			NormalizationExtensionValueMaxEncodedBytes: 64,
			RetainedDiagnosticCountPerObjectMax:        1,
			RetainedDiagnosticMaxUTF8Bytes:             64,
			CapabilityTombstoneLimit:                   4,
			CapabilityTombstoneMaxEncodedBytes:         128,
		},
		ClaimLifetime: time.Minute,
		Clock:         internalRuntimeClock{},
	})
	if err != nil {
		t.Fatalf("NewRuntimeAcquisitionSource: %v", err)
	}
	return source
}

func (committer *retryJoinTerminalCommitter) CommitTerminalState(
	ctx context.Context,
	_ TerminalStateCommitRequest,
) error {
	committer.mu.Lock()
	committer.calls++
	call := committer.calls
	committer.mu.Unlock()
	switch call {
	case 1:
		return errors.New("forced initial terminal persistence failure")
	case 2:
		close(committer.retryEntered)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-committer.retryRelease:
			return nil
		}
	default:
		return errors.New("duplicate terminal persistence retry")
	}
}

func (committer *retryJoinTerminalCommitter) callCount() int {
	committer.mu.Lock()
	defer committer.mu.Unlock()
	return committer.calls
}

func TestConcurrentCancellationRetriesJoinTerminalPersistence(t *testing.T) {
	limits := DefaultLedgerLimits()
	ledger := &SampleLedger{
		limits:                      limits,
		attempts:                    make(map[string]*runtimeAttemptState),
		commitSerial:                make(chan struct{}, 1),
		nextTerminalSequence:        3,
		durableNextTerminalSequence: 1,
		auditTombstones:             make([]LedgerAuditTombstone, 0, limits.AuditTombstoneLimit),
	}
	ledger.commitSerial <- struct{}{}
	committer := &retryJoinTerminalCommitter{
		retryEntered: make(chan struct{}),
		retryRelease: make(chan struct{}),
	}
	factory := &ObservationFactory{
		profile: ProfileDescriptor{spec: ProfileDescriptorSpec{
			Coherence: CoherencePolicySpec{Mode: CoherenceSingleWireResponse},
			Dependencies: internalDigestSet(t, []Dependency{
				internalDigestDependency(t, "reuse", "1.0.0", 101, "reuse-evidence"),
			}),
		}},
		ledger:                ledger,
		committer:             committer,
		terminalCommitTimeout: DefaultTerminalCommitTimeout,
	}
	state := &runtimeAttemptState{
		factory:          factory,
		key:              "concurrent-terminal-retry",
		phase:            AttemptOpen,
		terminalSequence: 1,
		entries: []runtimeClaimEntry{
			{phase: runtimeClaimUnresolved, terminalSequence: 2},
		},
		admitted:   true,
		cancelOpen: func() error { return nil },
	}
	state.cond = sync.NewCond(&state.mu)
	ledger.attempts[state.key] = state
	ledger.retainedClaims = 1
	attempt := &ObservationAttempt{state: state}

	if result, err := attempt.Cancel(); err == nil || result != CancellationFailed {
		t.Fatalf("initial Cancel=(%q, %v), want retryable persistence failure", result, err)
	}
	reuseSource := internalRuntimeSource(t)
	type visibility struct {
		restart LedgerRestartState
		reused  *ObservationAttempt
		err     error
	}
	visible := make(chan visibility, 1)
	state.cond = sync.NewCond(&terminalVisibilityLocker{
		mu: &state.mu,
		onLocked: func() {
			restart, restartErr := ledger.ExportRestartState()
			if restartErr != nil {
				visible <- visibility{err: restartErr}
				return
			}
			reused, beginErr := factory.BeginRuntimeAttempt(RuntimeAttemptRequest{
				Source:     reuseSource,
				AttemptKey: "concurrent-terminal-retry",
				Identity:   AttemptIdentity{PollGenerationID: 2},
				Observation: RuntimeObservationFacts{
					SourceValidity:          SourceValid,
					SourceTime:              SourceTimeUnavailable(),
					LocalReceiptTime:        time.Unix(1_700_000_000, 0).UTC(),
					LocalReceiptTimePresent: true,
				},
				Dependencies: []RuntimeDependencyFacts{{
					SourceTime: SourceTimeUnavailable(),
				}},
			})
			visible <- visibility{restart: restart, reused: reused, err: beginErr}
		},
	})

	type cancellation struct {
		result CancellationResult
		err    error
	}
	results := make(chan cancellation, 2)
	go func() {
		result, err := attempt.Cancel()
		results <- cancellation{result: result, err: err}
	}()
	<-committer.retryEntered
	go func() {
		result, err := attempt.Cancel()
		results <- cancellation{result: result, err: err}
	}()

	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		state.mu.Lock()
		waiters := state.terminalWaiters
		state.mu.Unlock()
		if waiters == 1 {
			break
		}
		select {
		case got := <-results:
			t.Fatalf("concurrent retry returned before shared commit: (%q, %v)", got.result, got.err)
		case <-deadline.C:
			t.Fatal("concurrent retry did not join terminal persistence")
		default:
			runtime.Gosched()
		}
	}
	close(committer.retryRelease)
	visibilityResult := <-visible

	for index := 0; index < 2; index++ {
		got := <-results
		if got.err != nil || got.result != CancellationCompleted {
			t.Fatalf("retry %d Cancel=(%q, %v)", index, got.result, got.err)
		}
	}
	if calls := committer.callCount(); calls != 2 {
		t.Fatalf("terminal persistence calls=%d, want initial failure plus one retry", calls)
	}
	if visibilityResult.err != nil {
		t.Fatalf("joined retry observed incomplete reclamation: %v", visibilityResult.err)
	}
	if visibilityResult.restart.NextTerminalSequence != 3 {
		t.Fatalf("joined retry restart state=%+v", visibilityResult.restart)
	}
	if visibilityResult.reused == nil {
		t.Fatal("joined retry could not reuse the reclaimed attempt key")
	}
}

func TestUnknownDrainErrorTerminalizesAndWakesCancellationWaiters(t *testing.T) {
	limits := DefaultLedgerLimits()
	ledger := &SampleLedger{
		limits:                      limits,
		attempts:                    make(map[string]*runtimeAttemptState),
		commitSerial:                make(chan struct{}, 1),
		nextTerminalSequence:        3,
		durableNextTerminalSequence: 1,
		auditTombstones:             make([]LedgerAuditTombstone, 0, limits.AuditTombstoneLimit),
	}
	ledger.commitSerial <- struct{}{}
	factory := &ObservationFactory{
		ledger:                ledger,
		committer:             internalTerminalCommitter{},
		terminalCommitTimeout: DefaultTerminalCommitTimeout,
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	forced := errors.New("forced unknown drain failure")
	state := &runtimeAttemptState{
		factory:          factory,
		key:              "forced-drain",
		phase:            AttemptOpen,
		terminalSequence: 1,
		entries: []runtimeClaimEntry{
			{phase: runtimeClaimUnresolved, terminalSequence: 2},
		},
		admitted: true,
		cancelOpen: func() error {
			close(entered)
			<-release
			return forced
		},
	}
	state.cond = sync.NewCond(&state.mu)
	ledger.attempts[state.key] = state
	ledger.retainedClaims = 1
	attempt := &ObservationAttempt{state: state}

	type result struct {
		outcome CancellationResult
		err     error
	}
	results := make(chan result, 2)
	go func() {
		outcome, err := attempt.Cancel()
		results <- result{outcome: outcome, err: err}
	}()
	<-entered
	go func() {
		outcome, err := attempt.Cancel()
		results <- result{outcome: outcome, err: err}
	}()
	close(release)

	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()
	errorCount := 0
	for index := 0; index < 2; index++ {
		select {
		case got := <-results:
			if got.outcome != CancellationFailed {
				t.Fatalf("Cancel outcome = %q", got.outcome)
			}
			if got.err != nil {
				errorCount++
			}
		case <-timeout.C:
			t.Fatal("cancellation waiter remained blocked")
		}
	}
	if errorCount == 0 {
		t.Fatal("forced drain error was not returned")
	}
	if attempt.Phase() != AttemptCancelFailed {
		t.Fatalf("phase = %q", attempt.Phase())
	}
	snapshot := ledger.Snapshot()
	if snapshot.RetainedAttempts != 0 || snapshot.RetainedClaimEntries != 0 {
		t.Fatalf("failed cancellation was not reclaimed: %+v", snapshot)
	}
	foundCancelFailure := false
	for _, tombstone := range snapshot.AuditTombstones {
		if tombstone.ObjectKind == LedgerAuditAttempt &&
			tombstone.TerminalOutcome == string(AttemptCancelFailed) {
			foundCancelFailure = true
		}
	}
	if !foundCancelFailure {
		t.Fatalf("cancel failure audit tombstone is absent: %+v", snapshot.AuditTombstones)
	}
}
