package modbusreg

import (
	"context"
	"errors"
	"sync"
	"time"
)

// I19EmmaProviderPort is a family-local port over supplied native evidence.
// It is not a production gateway Provider or an acquisition interface.
type I19EmmaProviderPort interface {
	I19Refresh(context.Context, EmmaIdentityContext, bool) (EmmaIdentityResult, error)
	I19SetContext(EmmaIdentityContext) error
	I19Retire()
	I19Latest() EmmaIdentityResult
	I19Identity() (EmmaIdentityInput, bool)
	I19RetainedIdentity() (EmmaRetainedIdentity, bool)
}

// I19EmmaProviderConfig is copied at construction. The supplier and logical
// clock must be cooperative, bounded and safe for the caller's concurrency.
// Supplied buffers remain immutable until refresh returns, as required by Cycle.
type I19EmmaProviderConfig struct {
	I19Bounds EmmaOfflineBounds
	I19Reader EmmaOfflineIdentityReader
	I19Now    func() time.Time
}

// I19EmmaProvider adapts lifecycle changes to the native publication fence.
// Do not copy it after construction. Retirement is terminal; history survives.
type I19EmmaProvider struct {
	i19Mu       sync.Mutex
	i19Native   *EmmaOfflineProvider
	i19Current  EmmaIdentityContext
	i19Config   I19EmmaProviderConfig
	i19Retired  bool
	i19Sequence uint64
}

func I19NewEmmaProvider(i19Current EmmaIdentityContext, i19Config I19EmmaProviderConfig) (*I19EmmaProvider, error) {
	if i19Config.I19Reader == nil || i19Config.I19Now == nil || i19Config.I19Bounds.MaxReaderWork == 0 || i19Config.I19Bounds.MaxInputBytes <= 0 {
		return nil, errors.New("invalid I19 EMMA configuration")
	}
	if NewEmmaIdentityAttempt(i19Current, EmmaIdentityInput{}, EmmaMEINotRequested, false).Commit(i19Current, i19Config.I19Now(), EmmaIdentityResult{}).Reason() == EmmaCommitContextRejected {
		return nil, errors.New("invalid I19 EMMA initial context")
	}
	return &I19EmmaProvider{i19Native: NewEmmaOfflineProvider(i19Current), i19Current: i19Current, i19Config: i19Config}, nil
}

// I19SetContext fences pending work. Use fresh attempt/connection tokens and
// never restore a retired context; a parent change discards foreign history.
func (i19Port *I19EmmaProvider) I19SetContext(i19Current EmmaIdentityContext) error {
	if i19Port == nil {
		return errors.New("nil I19 EMMA provider")
	}
	i19Port.i19Mu.Lock()
	defer i19Port.i19Mu.Unlock()
	if i19Port.i19Native == nil || i19Port.i19Retired {
		return errors.New("retired or uninitialized I19 EMMA provider")
	}
	i19Port.i19Sequence++
	i19Port.i19Current = i19Current
	i19Port.i19Native.SetContext(i19Current)
	return nil
}

func (i19Port *I19EmmaProvider) I19Retire() {
	if i19Port == nil {
		return
	}
	i19Port.i19Mu.Lock()
	defer i19Port.i19Mu.Unlock()
	i19Port.i19Retired = true
	i19Port.i19Sequence++
	i19Port.i19Current.Cancelled = true
	if i19Port.i19Native != nil {
		i19Port.i19Native.SetContext(i19Port.i19Current)
	}
}

// I19Refresh delegates to the actual native Cycle without a lock around the
// supplier. Explicit refresh cancellation is sampled at each native clock fence
// and becomes the existing Cancelled context before commit. It cannot forcibly
// interrupt a supplier. Cancellation after that sample is a subsequent event.
func (i19Port *I19EmmaProvider) I19Refresh(i19Refresh context.Context, i19Expected EmmaIdentityContext, i19InventoryRequested bool) (EmmaIdentityResult, error) {
	if i19Port == nil || i19Refresh == nil {
		return EmmaIdentityResult{}, errors.New("nil I19 EMMA provider or refresh context")
	}
	i19Port.i19Mu.Lock()
	if i19Port.i19Native == nil || i19Port.i19Retired {
		i19Port.i19Mu.Unlock()
		return EmmaIdentityResult{}, errors.New("retired or uninitialized I19 EMMA provider")
	}
	// A stale/refused refresh must not supersede or cancel unrelated valid work.
	if !i19SameContext(i19Expected, i19Port.i19Current) {
		i19Port.i19Mu.Unlock()
		return EmmaIdentityResult{}, errors.New("stale I19 EMMA context")
	}
	i19Port.i19Sequence++
	i19Ticket := i19Port.i19Sequence
	i19Config := i19Port.i19Config
	i19Port.i19Mu.Unlock()
	i19Clock := func() time.Time {
		i19At := i19Config.I19Now()
		if i19Refresh.Err() != nil {
			i19Port.i19Mu.Lock()
			if i19Ticket == i19Port.i19Sequence && i19SameContext(i19Expected, i19Port.i19Current) {
				i19Port.i19Current.Cancelled = true
				i19Port.i19Native.SetContext(i19Port.i19Current)
			}
			i19Port.i19Mu.Unlock()
		}
		return i19At
	}
	i19Result, i19Err := i19Port.i19Native.Cycle(i19Expected, i19Config.I19Bounds, i19InventoryRequested, i19Config.I19Reader, i19Clock)
	if i19Err == nil && i19Refresh.Err() != nil && !i19Result.Committed() {
		i19Err = i19Refresh.Err()
	}
	return i19Result, i19Err
}

func (i19Port *I19EmmaProvider) I19Latest() EmmaIdentityResult {
	if i19Port == nil {
		return EmmaIdentityResult{}
	}
	i19Port.i19Mu.Lock()
	defer i19Port.i19Mu.Unlock()
	if i19Port.i19Native == nil {
		return EmmaIdentityResult{}
	}
	return i19Port.i19Native.Latest()
}

// I19Identity exposes only a committed tuple still matching the active context
// and deadline. Missing current identity does not erase retained history.
func (i19Port *I19EmmaProvider) I19Identity() (EmmaIdentityInput, bool) {
	if i19Port == nil || i19Port.i19Config.I19Now == nil {
		return EmmaIdentityInput{}, false
	}
	i19At := i19Port.i19Config.I19Now()
	i19Port.i19Mu.Lock()
	defer i19Port.i19Mu.Unlock()
	if i19Port.i19Native == nil || i19Port.i19Retired || i19Port.i19Current.Cancelled || i19At.IsZero() || !i19At.Before(i19Port.i19Current.Deadline) {
		return EmmaIdentityInput{}, false
	}
	i19Latest := i19Port.i19Native.Latest()
	if !i19Latest.Committed() || !i19SameContext(i19Latest.AttemptContext(), i19Port.i19Current) {
		return EmmaIdentityInput{}, false
	}
	return i19Latest.AttemptInput(), true
}

func (i19Port *I19EmmaProvider) I19RetainedIdentity() (EmmaRetainedIdentity, bool) {
	return i19Port.I19Latest().RetainedIdentity()
}

func i19SameContext(i19Left, i19Right EmmaIdentityContext) bool {
	return i19Left.ParentSource == i19Right.ParentSource && i19Left.SourceRevision == i19Right.SourceRevision && i19Left.ConnectionGeneration == i19Right.ConnectionGeneration && i19Left.Attempt == i19Right.Attempt && i19Left.Deadline.Equal(i19Right.Deadline) && i19Left.Cancelled == i19Right.Cancelled
}
