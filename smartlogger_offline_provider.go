package modbusreg

import (
	"context"
	"errors"
	"time"
)

var ErrSmartloggerOfflineProviderInput = errors.New("invalid SmartLogger offline provider bounds or reader")

// SmartloggerOfflineInventoryReader supplies already-read native records for one
// frozen plan. It must finish within the plan deadline and caller input limits;
// it must not perform transport I/O or mutate returned buffers during Cycle.
// Errors must be safe for the caller to retain; they are returned unchanged.
type SmartloggerOfflineInventoryReader func(SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error)

// SmartloggerOfflineInventoryLimits bounds supplied pages, all objects (including
// the count object), and native object bytes including two framing bytes each.
// Limits may narrow, but never widen, the existing native parser ceilings.
type SmartloggerOfflineInventoryLimits struct {
	MaxPages   int
	MaxObjects int
	MaxBytes   int
}

func (l SmartloggerOfflineInventoryLimits) valid() bool {
	return l.MaxPages > 0 && l.MaxPages <= huaweiSmartLoggerInventoryMaxPages &&
		l.MaxObjects > 0 && l.MaxObjects <= huaweiSmartLoggerInventoryMaxObjects+1 &&
		l.MaxBytes > 0 && l.MaxBytes <= huaweiSmartLoggerInventoryMaxBytes
}

func (l SmartloggerOfflineInventoryLimits) accepts(input HuaweiSmartLoggerOfflineInventoryInput) bool {
	if len(input.Pages) == 0 || len(input.Pages) > l.MaxPages {
		return false
	}
	objects, bytes := 0, 0
	for _, page := range input.Pages {
		if len(page.Objects) > l.MaxObjects-objects {
			return false
		}
		objects += len(page.Objects)
		for _, object := range page.Objects {
			if l.MaxBytes-bytes < 2 || len(object.Value) > l.MaxBytes-bytes-2 {
				return false
			}
			bytes += len(object.Value) + 2
		}
	}
	return true
}

// SmartloggerOfflineProvider composes one explicitly invoked offline cycle over
// the existing commit owner. Do not copy it. It is not a gateway provider registry
// or acquisition owner, and it grants no admission. Its zero value refuses work.
type SmartloggerOfflineProvider struct {
	owner  *SmartloggerInventoryCommitOwner
	reader SmartloggerOfflineInventoryReader
}

func NewSmartloggerOfflineProvider(identity SmartloggerInventoryContext, clock func() time.Time, reader SmartloggerOfflineInventoryReader) (*SmartloggerOfflineProvider, error) {
	if reader == nil {
		return nil, ErrSmartloggerOfflineProviderInput
	}
	owner, err := NewSmartloggerInventoryCommitOwner(identity, clock)
	if err != nil {
		return nil, err
	}
	return &SmartloggerOfflineProvider{owner: owner, reader: reader}, nil
}

// Cycle invokes the reader exactly once after validating its finite caller
// bounds. The owner enforces a deadline at most fifteen seconds from begin;
// an earlier context deadline narrows it. A callback is synchronous and must
// cooperate with that deadline: this API cannot forcibly interrupt caller code.
// Refusals return a zero result and an error; accepted attempts return the
// owner's immutable terminal result. Failed validation has a coarse error.
func (p *SmartloggerOfflineProvider) Cycle(ctx context.Context, deadline time.Time, limits SmartloggerOfflineInventoryLimits) (SmartloggerInventoryResult, error) {
	if p == nil || p.owner == nil || p.reader == nil || ctx == nil || !limits.valid() {
		return SmartloggerInventoryResult{}, ErrSmartloggerOfflineProviderInput
	}
	if err := ctx.Err(); err != nil {
		return SmartloggerInventoryResult{}, err
	}
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	attempt, err := p.owner.Begin(deadline)
	if err != nil {
		return SmartloggerInventoryResult{}, err
	}
	plan := attempt.Plan()
	input, readErr := p.reader(plan)
	// Recheck cancellation after caller code, including cancellation immediately
	// before return. No goroutine or scheduler is required by this offline seam.
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			return attempt.Cancel(), err
		}
		return attempt.Commit(plan, HuaweiSmartLoggerOfflineInventoryInput{}), err
	}
	if readErr != nil {
		return attempt.Commit(plan, HuaweiSmartLoggerOfflineInventoryInput{}), readErr
	}
	if !limits.accepts(input) {
		return attempt.Commit(plan, HuaweiSmartLoggerOfflineInventoryInput{}), ErrSmartloggerOfflineProviderInput
	}
	result := attempt.Commit(plan, input)
	if result.Outcome != SmartloggerInventoryComplete {
		return result, ErrSmartloggerInventoryContext
	}
	return result, nil
}

func (p *SmartloggerOfflineProvider) Close() {
	if p != nil && p.owner != nil {
		p.owner.Close()
	}
}

func (p *SmartloggerOfflineProvider) Retire(identity SmartloggerInventoryContext) error {
	if p == nil || p.owner == nil {
		return ErrSmartloggerOfflineProviderInput
	}
	return p.owner.Retire(identity)
}

func (p *SmartloggerOfflineProvider) LastComplete() (SmartloggerInventorySnapshot, bool) {
	if p == nil || p.owner == nil {
		return SmartloggerInventorySnapshot{}, false
	}
	return p.owner.LastComplete()
}
