package modbusreg_test

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestRuntimeAttemptPublicSurfaceKeepsProducerAuthorityPrivate(t *testing.T) {
	typeOfAttempt := reflect.TypeOf((*reg.ObservationAttempt)(nil))
	for _, forbidden := range []string{"ProducerAttempt", "Acquisition", "Capability"} {
		if _, exists := typeOfAttempt.MethodByName(forbidden); exists {
			t.Fatalf("runtime attempt exposes producer authority through %s", forbidden)
		}
	}
	issue, exists := typeOfAttempt.MethodByName("Issue")
	if !exists || issue.Type.NumIn() != 4 || issue.Type.NumOut() != 1 {
		t.Fatalf("Issue API shape = %#v", issue)
	}
	admit, exists := typeOfAttempt.MethodByName("Admit")
	if !exists || admit.Type.NumIn() != 1 || admit.Type.NumOut() != 1 {
		t.Fatalf("Admit API shape = %#v", admit)
	}
}

type serialPublicationCommitter struct {
	mu            sync.Mutex
	state         reg.SampleLedgerState
	calls         int
	effects       int
	firstEntered  chan struct{}
	secondEntered chan struct{}
	releaseFirst  chan struct{}
}

func newSerialPublicationCommitter(
	state reg.SampleLedgerState,
) *serialPublicationCommitter {
	return &serialPublicationCommitter{
		state:         state,
		firstEntered:  make(chan struct{}),
		secondEntered: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
}

func (committer *serialPublicationCommitter) CommitPublication(
	ctx context.Context,
	request reg.PublicationCommitRequest,
) (reg.PublicationCommitDecision, error) {
	committer.mu.Lock()
	committer.calls++
	call := committer.calls
	committer.mu.Unlock()
	if call == 1 {
		close(committer.firstEntered)
		select {
		case <-ctx.Done():
			return reg.PublicationCommitCancelled, nil
		case <-committer.releaseFirst:
		}
	} else {
		if call == 2 {
			close(committer.secondEntered)
		}
		if err := ctx.Err(); err != nil {
			return reg.PublicationCommitCancelled, nil
		}
	}
	committer.mu.Lock()
	defer committer.mu.Unlock()
	if committer.state != request.ExpectedState {
		return "", fmt.Errorf("publication state conflict")
	}
	committer.state = request.PublishedState
	committer.effects++
	return reg.PublicationCommitCommitted, nil
}

func (committer *serialPublicationCommitter) counts() (int, int) {
	committer.mu.Lock()
	defer committer.mu.Unlock()
	return committer.calls, committer.effects
}

func twoSealedRuntimeAttempts(
	t *testing.T,
) (*serialPublicationCommitter, *reg.ObservationAttempt, *reg.ObservationAttempt, *reg.SampleLedger) {
	t.Helper()
	profile := profileFixture(t)
	initial, ledger := runtimeLedgerForTest(t, profile, reg.DefaultLedgerLimits())
	committer := newSerialPublicationCommitter(initial)
	factory, err := reg.NewObservationFactory(profile, ledger, committer)
	if err != nil {
		t.Fatalf("NewObservationFactory: %v", err)
	}
	seal := func(key string, poll uint64) *reg.ObservationAttempt {
		source, views := runtimeSourceAndViewsForGeneration(t, poll)
		attempt := beginAdmittedRuntimeAttempt(t, factory, source, views, key, poll)
		for ordinal := uint64(0); ordinal < uint64(len(views)); ordinal++ {
			outcome, err := attempt.Claim(ordinal)
			if err != nil || outcome != reg.ClaimSucceeded {
				t.Fatalf("Claim(%d)=(%q, %v)", ordinal, outcome, err)
			}
		}
		if err := attempt.Seal(); err != nil {
			t.Fatalf("Seal: %v", err)
		}
		return attempt
	}
	return committer, seal("serial-a", 41), seal("serial-b", 42), ledger
}

func waitPublicationBeforeRelease(
	t *testing.T,
	result <-chan publicationResult,
	release chan struct{},
) publicationResult {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case published := <-result:
		return published
	case <-timer.C:
		close(release)
		published := <-result
		t.Fatalf("queued publication ignored cancellation before commit admission: %v", published.err)
		return publicationResult{}
	}
}

func waitAttemptPhase(
	t *testing.T,
	attempt *reg.ObservationAttempt,
	want reg.AttemptPhase,
) {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if got := attempt.Phase(); got == want {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("attempt phase did not reach %q; got %q", want, attempt.Phase())
		case <-ticker.C:
		}
	}
}

func TestQueuedPublicationCancellationDoesNotWaitForCommitSlot(t *testing.T) {
	t.Run("attempt cancellation", func(t *testing.T) {
		committer, first, second, ledger := twoSealedRuntimeAttempts(t)
		firstResult := make(chan publicationResult, 1)
		go func() {
			observation, err := first.Publish(context.Background())
			firstResult <- publicationResult{observation: observation, err: err}
		}()
		<-committer.firstEntered
		secondResult := make(chan publicationResult, 1)
		go func() {
			observation, err := second.Publish(context.Background())
			secondResult <- publicationResult{observation: observation, err: err}
		}()
		waitAttemptPhase(t, second, reg.AttemptPublishing)
		cancelled := make(chan cancellationOutcome, 1)
		go func() {
			result, err := second.Cancel()
			cancelled <- cancellationOutcome{result: result, err: err}
		}()
		published := waitPublicationBeforeRelease(t, secondResult, committer.releaseFirst)
		cancelResult := <-cancelled
		if published.err == nil || cancelResult.err != nil ||
			cancelResult.result != reg.CancellationPublishFailed ||
			second.Phase() != reg.AttemptPublishFailed {
			t.Fatalf("queued cancellation = (%v, %+v, %q)", published.err, cancelResult, second.Phase())
		}
		close(committer.releaseFirst)
		if result := <-firstResult; result.err != nil {
			t.Fatalf("first Publish: %v", result.err)
		}
		calls, effects := committer.counts()
		snapshot := ledger.Snapshot()
		if calls != 1 || effects != 1 || ledger.ExportState().Revision != 1 ||
			snapshot.RetainedAttempts != 0 || snapshot.RetainedClaimEntries != 0 {
			t.Fatalf("committer calls=%d effects=%d state=%+v", calls, effects, ledger.ExportState())
		}
	})

	t.Run("parent context", func(t *testing.T) {
		committer, first, second, ledger := twoSealedRuntimeAttempts(t)
		firstResult := make(chan publicationResult, 1)
		go func() {
			observation, err := first.Publish(context.Background())
			firstResult <- publicationResult{observation: observation, err: err}
		}()
		<-committer.firstEntered
		ctx, cancel := context.WithCancel(context.Background())
		secondResult := make(chan publicationResult, 1)
		go func() {
			observation, err := second.Publish(ctx)
			secondResult <- publicationResult{observation: observation, err: err}
		}()
		waitAttemptPhase(t, second, reg.AttemptPublishing)
		cancel()
		published := waitPublicationBeforeRelease(t, secondResult, committer.releaseFirst)
		if published.err == nil || second.Phase() != reg.AttemptPublishFailed {
			t.Fatalf("context cancellation = (%v, %q)", published.err, second.Phase())
		}
		close(committer.releaseFirst)
		if result := <-firstResult; result.err != nil {
			t.Fatalf("first Publish: %v", result.err)
		}
		calls, effects := committer.counts()
		snapshot := ledger.Snapshot()
		if calls != 1 || effects != 1 || ledger.ExportState().Revision != 1 ||
			snapshot.RetainedAttempts != 0 || snapshot.RetainedClaimEntries != 0 {
			t.Fatalf("committer calls=%d effects=%d state=%+v", calls, effects, ledger.ExportState())
		}
	})
}

func TestProducerRestartRetirementRacingCancellationIsBenign(t *testing.T) {
	for iteration := 0; iteration < 10; iteration++ {
		committer := newArbitrationPublicationCommitter(reg.SampleLedgerState{})
		attempt, ledger, source := sealedRuntimeAttemptForTest(t, committer)
		start := make(chan struct{})
		exportResult := make(chan error, 1)
		cancelResult := make(chan cancellationOutcome, 1)
		go func() {
			<-start
			_, err := source.ExportRestartState()
			exportResult <- err
		}()
		go func() {
			<-start
			result, err := attempt.Cancel()
			cancelResult <- cancellationOutcome{result: result, err: err}
		}()
		close(start)
		if err := <-exportResult; err != nil {
			t.Fatalf("iteration %d ExportRestartState: %v", iteration, err)
		}
		cancelled := <-cancelResult
		if cancelled.err != nil || cancelled.result != reg.CancellationCompleted ||
			attempt.Phase() != reg.AttemptCancelled {
			t.Fatalf("iteration %d Cancel=(%q, %v), phase=%q", iteration, cancelled.result, cancelled.err, attempt.Phase())
		}
		if snapshot := ledger.Snapshot(); snapshot.RetainedAttempts != 0 ||
			snapshot.RetainedClaimEntries != 0 {
			t.Fatalf("iteration %d retained state: %+v", iteration, snapshot)
		}
	}
}
