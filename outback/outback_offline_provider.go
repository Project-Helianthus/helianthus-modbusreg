package outback

import (
	"context"
	"errors"
	"sync"
	"time"
)

// OfflineAttempt separates local publication context from native identity.
// Source is the frozen caller declaration, without verification or admission.
type OfflineAttempt struct {
	Generation, Number uint64
	Source             SourceAssociation
}

// DiagnosticSupplier imports supplied records; it must honor ctx and its
// deadline and bound its own work and allocations. It performs no acquisition.
// Returned buffers must remain untouched until Cycle returns.
type DiagnosticSupplier func(context.Context, OfflineAttempt) (DetachedRecordInput, error)

// OfflineStatus is the status of the latest Cycle attempt; it is
// independent of the retained record. See
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/outback-offline-provider-cycle.md.
type OfflineStatus string

// Attempt statuses. Idle: no attempt since construction or Replace. Running: a
// supplier call is in progress. Imported: a complete record was retained.
// Incomplete: the record was not complete; it replaces the retained record only
// when no complete record is retained. Failed: supplier, association or bound
// error. Canceled: the caller context or work deadline ended. Retired: Retire was
// called.
const (
	OfflineIdle       OfflineStatus = "idle"
	OfflineRunning    OfflineStatus = "running"
	OfflineImported   OfflineStatus = "imported"
	OfflineIncomplete OfflineStatus = "incomplete"
	OfflineFailed     OfflineStatus = "failed"
	OfflineCanceled   OfflineStatus = "canceled"
	OfflineRetired    OfflineStatus = "retired"
)

// OutBack offline provider errors. ErrOutBackOfflineInput: invalid construction
// or call arguments, or an exhausted generation or attempt counter.
// ErrOutBackOfflineAssociation: the supplied record names a source other than
// the frozen attempt source. ErrOutBackOfflineSuperseded: a newer attempt,
// Replace or Retire happened before publication; nothing was published.
// ErrOutBackOfflineRetired: the provider is retired.
var (
	ErrOutBackOfflineInput       = errors.New("OutBack offline provider: invalid bounded input")
	ErrOutBackOfflineAssociation = errors.New("OutBack offline provider: supplied association differs from frozen source")
	ErrOutBackOfflineSuperseded  = errors.New("OutBack offline provider: superseded attempt")
	ErrOutBackOfflineRetired     = errors.New("OutBack offline provider: retired")
)

// OfflineSnapshot contains one immutable retained record and the latest
// attempt status. An incomplete attempt may leave an earlier complete record.
// RetainedAt is caller-supplied receipt metadata, never qualification/freshness.
type OfflineSnapshot struct {
	Record                         *DetachedRecord
	RetainedAt                     time.Time
	RetainedAttempt, LatestAttempt OfflineAttempt
	Status                         OfflineStatus
	Association                    AssociationDisposition
}

// OfflineProvider is a family-local offline holder, not a gateway
// Provider registration or acquisition owner. Its zero value fails closed.
// Do not copy a provider after first use.
type OfflineProvider struct {
	mu                  sync.Mutex
	maxBytes            uint64
	maxWork             time.Duration
	supplier            DiagnosticSupplier
	source              SourceAssociation
	generation, attempt uint64
	retired             bool
	cancel              context.CancelFunc
	state               OfflineSnapshot
}

// NewOfflineProvider requires positive practical record/work bounds and
// a nonnil cooperative supplier. Source metadata is byte-checked and detached.
func NewOfflineProvider(source SourceAssociation, maxBytes uint64, maxWork time.Duration, supplier DiagnosticSupplier) (*OfflineProvider, error) {
	if maxBytes == 0 || maxWork <= 0 || supplier == nil {
		return nil, ErrOutBackOfflineInput
	}
	r, err := NewDetachedRecord(DetachedRecordInput{Source: source}, maxBytes)
	if err != nil {
		return nil, err
	}
	return &OfflineProvider{maxBytes: maxBytes, maxWork: maxWork, supplier: supplier, source: r.SourceAssociation(), generation: 1, state: OfflineSnapshot{Status: OfflineIdle}}, nil
}

// Replace advances local generation even for equal native metadata, fences all
// in-flight work and retains evidence under its original association and age.
func (p *OfflineProvider) Replace(source SourceAssociation) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.retired {
		return ErrOutBackOfflineRetired
	}
	if p.supplier == nil || p.generation == ^uint64(0) {
		return ErrOutBackOfflineInput
	}
	r, err := NewDetachedRecord(DetachedRecordInput{Source: source}, p.maxBytes)
	if err != nil {
		return err
	}
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.generation++
	p.source = r.SourceAssociation()
	p.state.Status = OfflineIdle
	return nil
}

// Retire permanently fences this holder; historical evidence remains available.
func (p *OfflineProvider) Retire() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.retired = true
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.state.Status = OfflineRetired
}

// Cycle calls the supplier synchronously once, under a deadline. There are no
// background workers, retries, reads or inferred requests. A newer attempt,
// replacement, retirement or cancellation fences publication after return.
func (p *OfflineProvider) Cycle(ctx context.Context, receivedAt time.Time) error {
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
	frozen := OfflineAttempt{Generation: p.generation, Number: p.attempt, Source: p.source}
	work, cancel := context.WithTimeout(ctx, p.maxWork)
	p.cancel = cancel
	p.state.LatestAttempt = frozen
	p.state.Status = OfflineRunning
	p.mu.Unlock()
	defer cancel()
	input, err := p.supplier(work, frozen)
	// Check both contexts: a supplier cannot convert cancellation into success.
	if canceled := ctx.Err(); canceled != nil {
		err = canceled
	} else if canceled = work.Err(); canceled != nil {
		err = canceled
	}
	var record *DetachedRecord
	if err == nil {
		if !outBackOfflineSameSource(input.Source, frozen.Source) {
			err = ErrOutBackOfflineAssociation
		} else {
			record, err = NewDetachedRecord(input, p.maxBytes)
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
		p.state.Status = OfflineFailed
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.state.Status = OfflineCanceled
		}
		return err
	}
	p.state.Status = OfflineImported
	if !record.Complete() {
		p.state.Status = OfflineIncomplete
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
func (p *OfflineProvider) Snapshot(current SourceAssociation) OfflineSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.state
	s.Association = AssociationHistorical
	if !p.retired && s.Record != nil && outBackOfflineSameSource(current, p.source) {
		s.Association = s.Record.Association(current)
	}
	return s
}

// Availability here compares declarations; current association separately
// requires availability on both the original and current records.
func outBackOfflineSameSource(a, b SourceAssociation) bool {
	return a.Key == b.Key && a.Revision == b.Revision &&
		a.PublicLocator == b.PublicLocator && a.ContentIdentity == b.ContentIdentity &&
		a.DeclaredLayout == b.DeclaredLayout && a.SourceSetRevision == b.SourceSetRevision &&
		a.SelectorInputs == b.SelectorInputs && a.Available == b.Available
}
