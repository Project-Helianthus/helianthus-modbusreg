package modbusreg

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrI21Association = errors.New("i21 sdongle: refresh key differs from frozen route")
	ErrI21Retired     = errors.New("i21 sdongle: provider retired")
	ErrI21Busy        = errors.New("i21 sdongle: refresh already in flight")
)

// I21SDongleProvider is a family-local documentary port, with no acquisition,
// registration, native qualification or sending authority.
type I21SDongleProvider interface {
	I21Refresh(context.Context, SDongleRouteKey, time.Time) (SDongleOfflineResult, error)
	I21Snapshot() SDongleOfflineResult
	I21Retire()
}

// I21SDonglePort owns one frozen route, one latest result and at most one
// retained complete success. Refreshes are synchronous and cannot overlap.
// The supplier must cooperate with context cancellation and native bounds.
// Do not copy a port after construction.
type I21SDonglePort struct {
	i21mu      sync.Mutex
	i21key     SDongleRouteKey
	i21bounds  SDongleOfflineBounds
	i21lookup  SDongleOfflineLookup
	i21latest  SDongleOfflineResult
	i21retired bool
	i21cancel  context.CancelFunc
}

// I21NewSDonglePort validates using the native cycle with an already canceled
// context: construction never invokes the supplier. Previous is optional
// caller-supplied history for this exact route, never an admission credential.
func I21NewSDonglePort(i21req SDongleOfflineRequest, i21lookup SDongleOfflineLookup) (*I21SDonglePort, error) {
	if i21req.Previous != nil && (i21req.Previous.Decision.Key != i21req.Key || i21req.Previous.Decision.Evidence.Key != i21req.Key) {
		return nil, ErrI21Association
	}
	i21ctx, i21cancel := context.WithCancel(context.Background())
	i21cancel()
	i21result, i21err := RunSDongleOfflineProvider(i21ctx, i21req, i21lookup)
	if !errors.Is(i21err, context.Canceled) {
		return nil, i21err
	}
	return &I21SDonglePort{i21key: i21req.Key, i21bounds: i21req.Bounds, i21lookup: i21lookup, i21latest: i21Detach(i21result)}, nil
}

// I21Refresh makes one bounded supplied-record lookup after refusal checks.
// Attempt time is local evaluation context; it never renews native source age.
// Retirement and cancellation fence publication even after supplier completion.
func (i21p *I21SDonglePort) I21Refresh(i21ctx context.Context, i21key SDongleRouteKey, i21at time.Time) (SDongleOfflineResult, error) {
	if i21p == nil {
		return SDongleOfflineResult{State: SDongleOfflineBlocked}, ErrSDongleOfflineInput
	}
	i21p.i21mu.Lock()
	i21refused := i21p.i21Blocked()
	if i21p.i21retired {
		i21p.i21mu.Unlock()
		return i21refused, ErrI21Retired
	}
	if i21p.i21lookup == nil || i21ctx == nil || i21at.IsZero() {
		i21p.i21mu.Unlock()
		return i21refused, ErrSDongleOfflineInput
	}
	if i21key != i21p.i21key {
		i21p.i21mu.Unlock()
		return i21refused, ErrI21Association
	}
	if i21err := i21ctx.Err(); i21err != nil {
		i21p.i21mu.Unlock()
		return i21refused, i21err
	}
	if i21p.i21cancel != nil {
		i21p.i21mu.Unlock()
		return i21refused, ErrI21Busy
	}
	i21work, i21cancel := context.WithCancel(i21ctx)
	i21p.i21cancel = i21cancel
	i21req := SDongleOfflineRequest{Key: i21p.i21key, Bounds: i21p.i21bounds, AttemptedAt: i21at, Previous: i21refused.Retained}
	// A native success can exceed the stricter Previous decision budget.
	// Keep that history locally without making it input to the next cycle.
	if i21req.Previous != nil && !sdongleOfflineDecisionFits(i21req.Bounds, i21req.Previous.Decision) {
		i21req.Previous = nil
	}
	i21lookup := i21p.i21lookup
	i21p.i21mu.Unlock()
	defer func() {
		i21cancel()
		i21p.i21mu.Lock()
		i21p.i21cancel = nil
		i21p.i21mu.Unlock()
	}()

	i21result, i21err := RunSDongleOfflineProvider(i21work, i21req, i21lookup)
	i21p.i21mu.Lock()
	defer i21p.i21mu.Unlock()
	if i21p.i21retired {
		return i21p.i21Blocked(), ErrI21Retired
	}
	if i21fence := i21work.Err(); i21fence != nil {
		i21result, i21err = i21p.i21Blocked(), i21fence
	}
	if i21result.Retained == nil {
		i21result.Retained = i21refused.Retained
	}
	i21p.i21latest = i21Detach(i21result)
	return i21Detach(i21result), i21err
}

// I21Snapshot returns independent copies of both the latest decision and its
// historical record. A retained Current flag describes its original attempt.
func (i21p *I21SDonglePort) I21Snapshot() SDongleOfflineResult {
	if i21p == nil {
		return SDongleOfflineResult{State: SDongleOfflineBlocked}
	}
	i21p.i21mu.Lock()
	defer i21p.i21mu.Unlock()
	return i21Detach(i21p.i21latest)
}

// I21Retire is idempotent. It blocks the current documentary result, preserves
// the original retained record and cancels any cooperative in-flight lookup.
func (i21p *I21SDonglePort) I21Retire() {
	if i21p == nil {
		return
	}
	i21p.i21mu.Lock()
	defer i21p.i21mu.Unlock()
	i21p.i21retired = true
	i21p.i21latest = i21p.i21Blocked()
	if i21p.i21cancel != nil {
		i21p.i21cancel()
	}
}

// i21Blocked requires the owner's lock and never substitutes retained readiness.
func (i21p *I21SDonglePort) i21Blocked() SDongleOfflineResult {
	return i21Detach(SDongleOfflineResult{State: SDongleOfflineBlocked, Retained: i21p.i21latest.Retained})
}

func i21Detach(i21result SDongleOfflineResult) SDongleOfflineResult {
	if i21result.Decision != nil {
		i21decision := *i21result.Decision
		i21decision.Evidence.References = append([]string(nil), i21decision.Evidence.References...)
		i21result.Decision = &i21decision
	}
	if i21result.Retained != nil {
		i21record := *i21result.Retained
		i21record.Decision.Evidence.References = append([]string(nil), i21record.Decision.Evidence.References...)
		i21result.Retained = &i21record
	}
	return i21result
}

var _ I21SDongleProvider = (*I21SDonglePort)(nil)
