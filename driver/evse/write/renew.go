package write

import (
	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

type renewal struct{ stop chan struct{} }

// held follows a command that sent a t25. A limit that the native comparison
// establishes to be in force is sent again at the renewal period with the same
// request; a sent request that does not establish one ends the renewal, and a
// command that sent nothing leaves it as it was. The caller holds the lifecycle
// lock.
func (w *Writer) held(guard operation.GuardClaims, request tesla.ProvisionalCurrentLimitRequest, r Result) Result {
	switch {
	case !r.Native.PossiblyTransmitted:
	case r.Matches:
		w.disarm()
		w.renewal = &renewal{stop: make(chan struct{})}
		w.wg.Add(1)
		go w.renew(w.renewal, guard, request)
	default:
		w.disarm()
	}
	return r
}

func (w *Writer) disarm() {
	if w.renewal != nil {
		close(w.renewal.stop)
		w.renewal = nil
	}
}

func (w *Writer) renew(r *renewal, guard operation.GuardClaims, request tesla.ProvisionalCurrentLimitRequest) {
	defer w.wg.Done()
	for {
		select {
		case <-r.stop:
			return
		case <-w.runCtx.Done():
			return
		case <-w.clock.After(RenewPeriod):
		}
		if !w.renewOnce(r, guard, request) {
			return
		}
	}
}

// renewOnce sends the request of the limit again. The exchange is the one of a
// command and is never repeated: a renewal that is not acknowledged and read
// back as the request ends the renewal and keeps its result.
func (w *Writer) renewOnce(r *renewal, guard operation.GuardClaims, request tesla.ProvisionalCurrentLimitRequest) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.renewal != r {
		return false
	}
	if w.admit(w.runCtx, guard) != "" {
		w.disarm()
		return false
	}
	result := w.send(w.runCtx, guard, request, 0, Result{}, false)
	if !result.Matches {
		w.disarm()
		return false
	}
	return true
}
