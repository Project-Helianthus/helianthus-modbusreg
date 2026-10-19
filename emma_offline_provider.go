package modbusreg

import (
	"errors"
	"sync"
	"time"
)

// EmmaOfflineBounds supplies finite caller ceilings. MaxInputBytes counts the
// decoded strings and raw request/response together. The reader must honor
// MaxReaderWork and the context deadline; the provider never retries it.
type EmmaOfflineBounds struct {
	MaxReaderWork uint32
	MaxInputBytes int
}

// EmmaOfflineIdentityReader returns one already validated FC03 input and an
// independent diagnostic MEI outcome. It performs no inventory acquisition.
// Returned buffers must remain unchanged until Cycle returns.
type EmmaOfflineIdentityReader func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error)

// EmmaOfflineProvider serializes local publication around a supplied reader.
// It is family-local, with no transport, registration or acquisition policy.
// Do not copy a provider after first use.
type EmmaOfflineProvider struct {
	mu      sync.Mutex
	current EmmaIdentityContext
	latest  EmmaIdentityResult
	cycle   uint64
}

func NewEmmaOfflineProvider(current EmmaIdentityContext) *EmmaOfflineProvider {
	return &EmmaOfflineProvider{current: current}
}

// SetContext fences in-flight completion. A different parent starts an empty
// history; revision/connection changes retain only separately labelled history.
func (p *EmmaOfflineProvider) SetContext(current EmmaIdentityContext) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if current.ParentSource != p.current.ParentSource {
		p.latest = EmmaIdentityResult{}
	}
	p.current = current
}

// Latest returns the latest published attempt, with history exposed separately
// by RetainedIdentity. A retained observation is never current admission proof.
func (p *EmmaOfflineProvider) Latest() EmmaIdentityResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.latest
}

// Cycle calls reader once outside the publication lock. The caller supplies a
// logical clock, finite bounds and a cooperative bounded reader. Context and
// deadline are checked before the reader and again at commit. There is no
// goroutine, retry, guard procedure or inventory callback inside this seam.
func (p *EmmaOfflineProvider) Cycle(context EmmaIdentityContext, bounds EmmaOfflineBounds, inventoryRequested bool, reader EmmaOfflineIdentityReader, now func() time.Time) (EmmaIdentityResult, error) {
	if reader == nil || now == nil || bounds.MaxReaderWork == 0 || bounds.MaxInputBytes <= 0 {
		return EmmaIdentityResult{}, errors.New("invalid EMMA offline reader, clock or bounds")
	}
	before := now()
	p.mu.Lock()
	preflight := NewEmmaIdentityAttempt(context, EmmaIdentityInput{}, EmmaMEINotRequested, inventoryRequested).Commit(p.current, before, p.latest)
	if preflight.Reason() == EmmaCommitContextRejected {
		p.mu.Unlock()
		return preflight, errors.New("invalid or stale EMMA offline context")
	}
	p.cycle++
	ticket := p.cycle
	p.mu.Unlock()

	input, mei, readErr := reader(context, bounds)
	if !emmaOfflineInputFits(input, bounds.MaxInputBytes) {
		// Oversized evidence is explicitly unavailable, never silently truncated.
		input = EmmaIdentityInput{Status: EmmaFC03Failed}
		readErr = errors.New("EMMA offline input exceeds caller byte bound")
	} else if readErr != nil {
		input.Status = EmmaFC03Failed
	}
	attempt := NewEmmaIdentityAttempt(context, input, mei, inventoryRequested)
	after := now()
	p.mu.Lock()
	defer p.mu.Unlock()
	current := p.current
	previous := p.latest
	if context.ParentSource != current.ParentSource {
		previous = EmmaIdentityResult{}
	}
	if ticket != p.cycle {
		current.Cancelled = true
	}
	result := attempt.Commit(current, after, previous)
	if ticket == p.cycle && context.ParentSource == p.current.ParentSource {
		p.latest = result
	}
	return result, readErr
}

func emmaOfflineInputFits(input EmmaIdentityInput, remaining int) bool {
	for _, size := range []int{len(input.Offering), len(input.Model), len(input.Firmware), len(input.Request), len(input.Response)} {
		if size > remaining {
			return false
		}
		remaining -= size
	}
	return true
}
