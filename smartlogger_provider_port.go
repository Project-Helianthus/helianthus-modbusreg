package modbusreg

import (
	"context"
	"sync"
	"time"
)

// I17SmartloggerProvider is a family-local port over supplied native evidence.
// Completion and Current never grant qualification or production admission.
type I17SmartloggerProvider interface {
	I17Refresh(context.Context, time.Time) (SmartloggerInventoryResult, error)
	I17NativeView() (SmartloggerInventorySnapshot, bool)
	I17Retire(SmartloggerInventoryContext) error
	I17Close()
}

// I17SmartloggerProviderAdapter delegates all inventory mechanics to the native
// offline provider. Do not copy it. Its zero value refuses work.
type I17SmartloggerProviderAdapter struct {
	i17Mu       sync.Mutex
	i17Provider *SmartloggerOfflineProvider
	i17Limits   SmartloggerOfflineInventoryLimits
	i17Busy     bool
	i17Closed   bool
}

var _ I17SmartloggerProvider = (*I17SmartloggerProviderAdapter)(nil)

// I17NewSmartloggerProvider freezes value inputs and the chosen clock function.
// The clock and supplier retain the native offline provider's finite, nonblocking
// clock and cooperative synchronous supplier contracts. Nil clock uses time.Now.
func I17NewSmartloggerProvider(i17Identity SmartloggerInventoryContext, i17Limits SmartloggerOfflineInventoryLimits, i17Clock func() time.Time, i17Reader SmartloggerOfflineInventoryReader) (*I17SmartloggerProviderAdapter, error) {
	if !i17Limits.valid() || i17Reader == nil {
		return nil, ErrSmartloggerOfflineProviderInput
	}
	i17P := &I17SmartloggerProviderAdapter{i17Limits: i17Limits}
	i17Native, i17Err := NewSmartloggerOfflineProvider(i17Identity, i17Clock, func(i17Plan SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
		// A refresh may have reserved its slot just before retirement. In that
		// case do not invoke supplier code under the successor native context.
		i17P.i17Mu.Lock()
		i17Closed := i17P.i17Closed
		i17P.i17Mu.Unlock()
		if i17Closed {
			return HuaweiSmartLoggerOfflineInventoryInput{}, ErrSmartloggerInventoryClosed
		}
		return i17Reader(i17Plan)
	})
	if i17Err != nil {
		return nil, i17Err
	}
	i17P.i17Provider = i17Native
	return i17P, nil
}

// I17Refresh executes one local cycle with constructor-frozen bounds. Concurrent
// refreshes refuse rather than queue. No lock is held across supplier code.
func (i17P *I17SmartloggerProviderAdapter) I17Refresh(i17Context context.Context, i17Deadline time.Time) (SmartloggerInventoryResult, error) {
	if i17P == nil {
		return SmartloggerInventoryResult{}, ErrSmartloggerOfflineProviderInput
	}
	i17P.i17Mu.Lock()
	if i17P.i17Provider == nil {
		i17P.i17Mu.Unlock()
		return SmartloggerInventoryResult{}, ErrSmartloggerOfflineProviderInput
	}
	if i17P.i17Closed {
		i17P.i17Mu.Unlock()
		return SmartloggerInventoryResult{}, ErrSmartloggerInventoryClosed
	}
	if i17P.i17Busy {
		i17P.i17Mu.Unlock()
		return SmartloggerInventoryResult{}, ErrSmartloggerInventoryBusy
	}
	i17P.i17Busy = true
	i17P.i17Mu.Unlock()
	defer func() {
		i17P.i17Mu.Lock()
		i17P.i17Busy = false
		i17P.i17Mu.Unlock()
	}()
	return i17P.i17Provider.Cycle(i17Context, i17Deadline, i17P.i17Limits)
}

// I17NativeView returns the native owner's detached last-complete history.
func (i17P *I17SmartloggerProviderAdapter) I17NativeView() (SmartloggerInventorySnapshot, bool) {
	if i17P == nil || i17P.i17Provider == nil {
		return SmartloggerInventorySnapshot{}, false
	}
	return i17P.i17Provider.LastComplete()
}

// I17Retire validates a strictly newer native context and permanently retires
// this port. A successor needs a new port; retained history is never relabeled.
// An active native attempt retains its retired outcome, including on late return.
func (i17P *I17SmartloggerProviderAdapter) I17Retire(i17Next SmartloggerInventoryContext) error {
	if i17P == nil {
		return ErrSmartloggerOfflineProviderInput
	}
	i17P.i17Mu.Lock()
	defer i17P.i17Mu.Unlock()
	if i17P.i17Provider == nil {
		return ErrSmartloggerOfflineProviderInput
	}
	if i17P.i17Closed {
		return ErrSmartloggerInventoryClosed
	}
	if i17Err := i17P.i17Provider.Retire(i17Next); i17Err != nil {
		return i17Err
	}
	i17P.i17Closed = true
	i17P.i17Provider.Close()
	return nil
}

// I17Close permanently fences publication without waiting for supplier code.
func (i17P *I17SmartloggerProviderAdapter) I17Close() {
	if i17P == nil {
		return
	}
	i17P.i17Mu.Lock()
	defer i17P.i17Mu.Unlock()
	i17P.i17Closed = true
	if i17P.i17Provider != nil {
		i17P.i17Provider.Close()
	}
}
