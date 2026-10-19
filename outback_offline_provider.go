package modbusreg

import (
	"context"
	"errors"
	"sync"
	"time"
)

// OutBackOfflineAttempt separates local publication context from native identity.
// Source is the frozen caller declaration, without verification or admission.
type OutBackOfflineAttempt struct {
	Generation, Number uint64
	Source             OutBackSourceAssociation
}

// OutBackDiagnosticSupplier imports supplied records; it must honor ctx and its
// deadline and bound its own work and allocations. It performs no acquisition.
// Returned buffers must remain untouched until Cycle returns.
type OutBackDiagnosticSupplier func(context.Context, OutBackOfflineAttempt) (OutBackDetachedRecordInput, error)

type OutBackOfflineStatus string

const (
	OutBackOfflineIdle       OutBackOfflineStatus = "idle"
	OutBackOfflineRunning    OutBackOfflineStatus = "running"
	OutBackOfflineImported   OutBackOfflineStatus = "imported"
	OutBackOfflineIncomplete OutBackOfflineStatus = "incomplete"
	OutBackOfflineFailed     OutBackOfflineStatus = "failed"
	OutBackOfflineCanceled   OutBackOfflineStatus = "canceled"
	OutBackOfflineRetired    OutBackOfflineStatus = "retired"
)

var (
	ErrOutBackOfflineInput       = errors.New("OutBack offline provider: invalid bounded input")
	ErrOutBackOfflineAssociation = errors.New("OutBack offline provider: supplied association differs from frozen source")
	ErrOutBackOfflineSuperseded  = errors.New("OutBack offline provider: superseded attempt")
	ErrOutBackOfflineRetired     = errors.New("OutBack offline provider: retired")
)

// OutBackOfflineSnapshot contains one immutable retained record and the latest
// attempt status. An incomplete attempt may leave an earlier complete record.
// RetainedAt is caller-supplied receipt metadata, never qualification/freshness.
type OutBackOfflineSnapshot struct {
	Record                         *OutBackDetachedRecord
	RetainedAt                     time.Time
	RetainedAttempt, LatestAttempt OutBackOfflineAttempt
	Status                         OutBackOfflineStatus
	Association                    OutBackAssociationDisposition
}

// OutBackOfflineProvider is a family-local offline holder, not a gateway
// Provider registration or acquisition owner. Its zero value fails closed.
// Do not copy a provider after first use.
type OutBackOfflineProvider struct {
	mu                  sync.Mutex
	maxBytes            uint64
	maxWork             time.Duration
	supplier            OutBackDiagnosticSupplier
	source              OutBackSourceAssociation
	generation, attempt uint64
	retired             bool
	cancel              context.CancelFunc
	state               OutBackOfflineSnapshot
}

// NewOutBackOfflineProvider requires positive practical record/work bounds and
// a nonnil cooperative supplier. Source metadata is byte-checked and detached.
func NewOutBackOfflineProvider(source OutBackSourceAssociation, maxBytes uint64, maxWork time.Duration, supplier OutBackDiagnosticSupplier) (*OutBackOfflineProvider, error) {
	if maxBytes == 0 || maxWork <= 0 || supplier == nil {
		return nil, ErrOutBackOfflineInput
	}
	r, err := NewOutBackDetachedRecord(OutBackDetachedRecordInput{Source: source}, maxBytes)
	if err != nil {
		return nil, err
	}
	return &OutBackOfflineProvider{maxBytes: maxBytes, maxWork: maxWork, supplier: supplier, source: r.SourceAssociation(), generation: 1, state: OutBackOfflineSnapshot{Status: OutBackOfflineIdle}}, nil
}

// Replace advances local generation even for equal native metadata, fences all
// in-flight work and retains evidence under its original association and age.
func (p *OutBackOfflineProvider) Replace(source OutBackSourceAssociation) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.retired {
		return ErrOutBackOfflineRetired
	}
	if p.supplier == nil || p.generation == ^uint64(0) {
		return ErrOutBackOfflineInput
	}
	r, err := NewOutBackDetachedRecord(OutBackDetachedRecordInput{Source: source}, p.maxBytes)
	if err != nil {
		return err
	}
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.generation++
	p.source = r.SourceAssociation()
	p.state.Status = OutBackOfflineIdle
	return nil
}

// Retire permanently fences this holder; historical evidence remains available.
func (p *OutBackOfflineProvider) Retire() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.retired = true
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.state.Status = OutBackOfflineRetired
}

// Cycle calls the supplier synchronously once, under a deadline. There are no
// background workers, retries, reads or inferred requests. A newer attempt,
// replacement, retirement or cancellation fences publication after return.
func (p *OutBackOfflineProvider) Cycle(ctx context.Context, receivedAt time.Time) error {
	if ctx == nil || receivedAt.IsZero() {
		return ErrOutBackOfflineInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	if p.retired {
		p.mu.Unlock()
		return ErrOutBackOfflineRetired
	}
	if p.supplier == nil || p.maxBytes == 0 || p.maxWork <= 0 || p.attempt == ^uint64(0) {
		p.mu.Unlock()
		return ErrOutBackOfflineInput
	}
	if p.cancel != nil {
		p.cancel()
	}
	p.attempt++
	frozen := OutBackOfflineAttempt{Generation: p.generation, Number: p.attempt, Source: p.source}
	work, cancel := context.WithTimeout(ctx, p.maxWork)
	p.cancel = cancel
	p.state.LatestAttempt = frozen
	p.state.Status = OutBackOfflineRunning
	p.mu.Unlock()
	defer cancel()
	input, err := p.supplier(work, frozen)
	// Check both contexts: a supplier cannot convert cancellation into success.
	if canceled := ctx.Err(); canceled != nil {
		err = canceled
	} else if canceled = work.Err(); canceled != nil {
		err = canceled
	}
	var record *OutBackDetachedRecord
	if err == nil {
		if !outBackOfflineSameSource(input.Source, frozen.Source) {
			err = ErrOutBackOfflineAssociation
		} else {
			record, err = NewOutBackDetachedRecord(input, p.maxBytes)
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.retired || p.generation != frozen.Generation || p.attempt != frozen.Number {
		return ErrOutBackOfflineSuperseded
	}
	// Cancellation may happen while the record is being validated and copied.
	if canceled := ctx.Err(); canceled != nil {
		err = canceled
	} else if canceled = work.Err(); canceled != nil {
		err = canceled
	}
	p.cancel = nil
	if err != nil {
		p.state.Status = OutBackOfflineFailed
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.state.Status = OutBackOfflineCanceled
		}
		return err
	}
	p.state.Status = OutBackOfflineImported
	if !record.Complete() {
		p.state.Status = OutBackOfflineIncomplete
		if p.state.Record != nil && p.state.Record.Complete() {
			return nil
		}
	}
	p.state.Record = record
	p.state.RetainedAt = receivedAt
	p.state.RetainedAttempt = frozen
	return nil
}

// Snapshot compares retained provenance with a caller's current descriptor. A
// source also must match the holder's current generation descriptor to be current.
// Missing/conflicting metadata is historical; the record is never relabeled.
func (p *OutBackOfflineProvider) Snapshot(current OutBackSourceAssociation) OutBackOfflineSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.state
	s.Association = OutBackAssociationHistorical
	if !p.retired && s.Record != nil && outBackOfflineSameSource(current, p.source) {
		s.Association = s.Record.Association(current)
	}
	return s
}

// Availability here compares declarations; current association separately
// requires availability on both the original and current records.
func outBackOfflineSameSource(a, b OutBackSourceAssociation) bool {
	return a.Key == b.Key && a.Revision == b.Revision &&
		a.PublicLocator == b.PublicLocator && a.ContentIdentity == b.ContentIdentity &&
		a.DeclaredLayout == b.DeclaredLayout && a.SourceSetRevision == b.SourceSetRevision &&
		a.SelectorInputs == b.SelectorInputs && a.Available == b.Available
}
