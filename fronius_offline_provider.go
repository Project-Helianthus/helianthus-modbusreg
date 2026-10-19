package modbusreg

import (
	"context"
	"fmt"
)

// FroniusOfflineRequest bounds one explicitly invoked offline cycle. Source
// identities are opaque provenance; they are never addresses to contact.
type FroniusOfflineRequest struct {
	Key              FroniusRetainedKey
	PollGeneration   uint64
	DeadlineIdentity uint64
	MaxWords         uint32
	MaxOccurrences   uint32
	MaxSourceViews   uint32
}

// FroniusOfflineReadRequest binds the reader to the original caller attempt.
type FroniusOfflineReadRequest struct {
	Attempt        FroniusRetainedAttempt
	MaxWords       uint32
	MaxOccurrences uint32
	MaxSourceViews uint32
}

// FroniusOfflineReadResult must echo the supplied attempt without rewriting it.
// The observation is native evidence, never a caller qualification verdict.
type FroniusOfflineReadResult struct {
	Attempt     FroniusRetainedAttempt
	Observation SunSpecQualificationObservation
}

// FroniusOfflineReader is called synchronously exactly once for valid input.
// It must honor the supplied context and work bounds, perform no live I/O, and
// keep the returned observation immutable until Run returns. No retry occurs.
type FroniusOfflineReader func(context.Context, FroniusOfflineReadRequest) (FroniusOfflineReadResult, error)

// FroniusOfflineProvider composes one family-local offline cycle with the
// existing retained owner. Its zero value is ready; do not copy after use.
// It owns no transport, scheduler, persistence or gateway registration.
type FroniusOfflineProvider struct {
	retained FroniusRetainedCommit
}

// Run begins a new attempt, obtains supplied native evidence and commits only
// an exact-card result. Invalid input never invokes the reader. Begin still
// fences incompatible history on invalid attempts. Cancellation is cooperative;
// Run creates no goroutine and cannot interrupt an uncooperative callback.
func (p *FroniusOfflineProvider) Run(ctx context.Context, registry SunSpecDecoderRegistry, request FroniusOfflineRequest, reader FroniusOfflineReader) error {
	token, err := p.retained.Begin(ctx, request.Key, request.PollGeneration, request.DeadlineIdentity)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		_ = p.retained.Fail(token)
		return err
	}
	card := NewSunSpecFroniusQualificationCard()
	k := request.Key
	if reader == nil || request.MaxWords == 0 || request.MaxWords > card.Limits().MaxTotalWords ||
		request.MaxOccurrences == 0 || request.MaxOccurrences > card.Limits().MaxOccurrences ||
		request.MaxSourceViews == 0 || request.MaxSourceViews > MaxSunSpecPhaseOneChainWords ||
		k.CardID != card.ID() || k.Schema != card.SchemaRevision() || registry.revision != card.SchemaRevision() || len(registry.definitions) == 0 ||
		k.RequestedFunction != card.Function() || k.ReceivedFunction != card.Function() || k.Table != HoldingRegisters || k.UnitID != card.UnitID() ||
		(k.Transport != TransportTCP && k.Transport != TransportRTU) {
		return fail(fmt.Errorf("invalid offline reader, bounds or native card input"))
	}
	deadline, _ := ctx.Deadline()
	attempt := FroniusRetainedAttempt{Token: token, Key: k, PollGeneration: request.PollGeneration, DeadlineIdentity: request.DeadlineIdentity, Deadline: deadline}
	result, readErr := reader(ctx, FroniusOfflineReadRequest{Attempt: attempt, MaxWords: request.MaxWords, MaxOccurrences: request.MaxOccurrences, MaxSourceViews: request.MaxSourceViews})
	// Observe cancellation before considering even a successful reader result.
	if ctx.Err() != nil {
		_ = p.retained.Cancel(token)
		return fmt.Errorf("offline caller context ended: %w", ctx.Err())
	}
	if readErr != nil {
		return fail(fmt.Errorf("offline reader failed"))
	}
	if result.Attempt != attempt {
		return fail(fmt.Errorf("offline reader attempt mismatch"))
	}
	// Check the supplied sizes before replay/cloning. The retained owner applies
	// native validation and rechecks token/context at the publication boundary.
	o := result.Observation
	if len(o.snapshot.raw) > int(request.MaxWords) || len(o.snapshot.occurrences) > int(request.MaxOccurrences) || len(o.snapshot.sources) > int(request.MaxSourceViews) {
		return fail(fmt.Errorf("offline result exceeds caller bounds"))
	}
	return p.retained.Commit(token, registry, o)
}

// Cancel closes only the specified active cycle; late callbacks cannot publish.
func (p *FroniusOfflineProvider) Cancel(token FroniusRetainedToken) error {
	return p.retained.Cancel(token)
}

// View returns the existing owner's detached atomic native envelope and status.
func (p *FroniusOfflineProvider) View() FroniusRetainedView { return p.retained.View() }
