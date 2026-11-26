package write

import (
	"context"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

// release sends the t25 that returns the limit to the configured current of the
// latest t28 with the lifetime of the operation and inhibit false, then stops
// the renewal. It is the typed release case of the operation and is reached
// only from the executor. A configured current that is missing, zero or not
// usable cannot form the triple: nothing is sent, the t28 that was read is kept
// in the result and the renewal of a limit in force is left as it was. The
// release is acknowledged and its t28 kept beside the request; it is not a
// readback confirmation.
func (w *Writer) release(ctx context.Context, guard operation.GuardClaims) Result {
	w.mu.Lock()
	defer w.mu.Unlock()
	if ctx == nil {
		return refused(ReasonCancelled)
	}
	if reason := w.admit(ctx, guard); reason != "" {
		return Result{Level: LevelFailedNoContact, Reason: reason, Release: true}
	}
	configured, pre := w.configuredCurrent(ctx, guard)
	pre.Release = true
	if pre.Reason != "" {
		return pre
	}
	r := w.send(ctx, guard, tesla.ProvisionalCurrentLimitRequest{LimitCurrentMaxAmps: configured, LimitTimeoutSeconds: LimitTimeoutSeconds}, configured, pre, false)
	r.Release = true
	if r.LimitRequestSent() {
		w.disarm()
	}
	return r
}
