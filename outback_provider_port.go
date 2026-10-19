package modbusreg

import (
	"context"
	"errors"
	"sync"
	"time"
)

// I27OutBackProvider is a family-local lifecycle port over supplied diagnostics.
// It is not a common production Provider, acquisition owner or registration.
type I27OutBackProvider interface {
	I27Replace(i27Source OutBackSourceAssociation) error
	I27Retire()
	I27Cycle(i27Ctx context.Context, i27ReceivedAt time.Time) error
	I27Snapshot(i27Current OutBackSourceAssociation) OutBackOfflineSnapshot
}

// I27OutBackConfig requires explicit finite bounds and a cooperative supplier.
// The supplier imports opaque evidence only and must honor context cancellation
// and bound its own allocations. No default source, layout or read is inferred.
type I27OutBackConfig struct {
	I27Source   OutBackSourceAssociation
	I27MaxBytes uint64
	I27MaxWork  time.Duration
	I27Supplier OutBackDiagnosticSupplier
}

var ErrI27OutBackBusy = errors.New("I27 OutBack provider: diagnostic callback still active")

// I27OutBackAdapter composes native lifecycle boundaries without decoding.
// Do not copy it after first use. Its zero value and nil receivers fail closed.
type I27OutBackAdapter struct {
	i27Mu         sync.Mutex
	i27CycleMu    sync.Mutex
	i27Native     *OutBackOfflineProvider
	i27Generation uint64
}

var _ I27OutBackProvider = (*I27OutBackAdapter)(nil)

// I27NewOutBackAdapter validates configuration without invoking the supplier.
func I27NewOutBackAdapter(i27Config I27OutBackConfig) (*I27OutBackAdapter, error) {
	i27Native, i27Err := NewOutBackOfflineProvider(i27Config.I27Source, i27Config.I27MaxBytes, i27Config.I27MaxWork, i27Config.I27Supplier)
	if i27Err != nil {
		return nil, i27Err
	}
	return &I27OutBackAdapter{i27Native: i27Native, i27Generation: 1}, nil
}

// I27Replace fences native publication even for equal source metadata. Retained
// evidence stays historical across replacements until a new import is retained.
func (i27P *I27OutBackAdapter) I27Replace(i27Source OutBackSourceAssociation) error {
	if i27P == nil || i27P.i27Native == nil {
		return ErrOutBackOfflineInput
	}
	i27P.i27Mu.Lock()
	defer i27P.i27Mu.Unlock()
	if i27Err := i27P.i27Native.Replace(i27Source); i27Err != nil {
		return i27Err
	}
	i27P.i27Generation++
	return nil
}

// I27Retire permanently closes publication while retaining original evidence.
func (i27P *I27OutBackAdapter) I27Retire() {
	if i27P == nil || i27P.i27Native == nil {
		return
	}
	i27P.i27Mu.Lock()
	defer i27P.i27Mu.Unlock()
	i27P.i27Native.Retire()
}

// I27Cycle calls the native synchronous cycle once. Active callbacks occupy
// capacity until return, including after replacement, retirement or cancellation.
// Busy calls refuse immediately without a callback, queue or automatic retry.
func (i27P *I27OutBackAdapter) I27Cycle(i27Ctx context.Context, i27ReceivedAt time.Time) error {
	if i27P == nil || i27P.i27Native == nil {
		return ErrOutBackOfflineInput
	}
	if !i27P.i27CycleMu.TryLock() {
		return ErrI27OutBackBusy
	}
	defer i27P.i27CycleMu.Unlock()
	return i27P.i27Native.Cycle(i27Ctx, i27ReceivedAt)
}

// I27Snapshot preserves native source comparison and additionally requires the
// retained publication generation to match this adapter's current generation.
// Association and structural completeness never confer admission or freshness.
func (i27P *I27OutBackAdapter) I27Snapshot(i27Current OutBackSourceAssociation) OutBackOfflineSnapshot {
	if i27P == nil || i27P.i27Native == nil {
		return OutBackOfflineSnapshot{Association: OutBackAssociationHistorical}
	}
	i27P.i27Mu.Lock()
	defer i27P.i27Mu.Unlock()
	i27Snapshot := i27P.i27Native.Snapshot(i27Current)
	if i27Snapshot.RetainedAttempt.Generation != i27P.i27Generation {
		i27Snapshot.Association = OutBackAssociationHistorical
	}
	return i27Snapshot
}
