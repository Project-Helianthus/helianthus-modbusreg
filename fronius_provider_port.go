package modbusreg

import (
	"context"
	"fmt"
	"sync"
)

// I25FroniusProvider is a family-local port over supplied offline evidence.
// It is not a production gateway Provider, qualification or admission API.
type I25FroniusProvider interface {
	I25Run(context.Context, I25FroniusLifetime, FroniusOfflineRequest) error
	I25Cancel(I25FroniusLifetime, FroniusRetainedToken) error
	I25Retire(I25FroniusLifetime) error
	I25View() I25FroniusView
}

// I25FroniusLifetime identifies one configured adapter, independently of the
// native attempt token. A retired lifetime cannot be restarted or reused.
type I25FroniusLifetime struct{ i25Identity *i25FroniusIdentity }
type i25FroniusIdentity struct{ i25Present bool }

// I25FroniusView separates adapter lifetime from the detached native view.
// Historical evidence after retirement grants no runtime admission.
type I25FroniusView struct {
	I25Lifetime I25FroniusLifetime
	I25Retired  bool
	I25Native   FroniusRetainedView
}

// I25FroniusAdapter owns one native cycle and configured offline callback.
// Construct with I25NewFroniusAdapter; the zero value refuses work. Do not copy.
type I25FroniusAdapter struct {
	i25Mu                                             sync.Mutex
	i25Lifetime                                       I25FroniusLifetime
	i25Retired                                        bool
	i25Registry                                       SunSpecDecoderRegistry
	i25MaxWords, i25MaxOccurrences, i25MaxSourceViews uint32
	i25Reader                                         FroniusOfflineReader
	i25Native                                         FroniusOfflineProvider
}

// I25NewFroniusAdapter accepts an existing registry and finite per-run ceilings.
// It copies the registry and invokes no supplier. Native validation still owns
// exact-card acceptance; these ceilings confer no acquisition authority.
func I25NewFroniusAdapter(i25Registry SunSpecDecoderRegistry, i25MaxWords, i25MaxOccurrences, i25MaxSourceViews uint32, i25Reader FroniusOfflineReader) (*I25FroniusAdapter, error) {
	i25Card := NewSunSpecFroniusQualificationCard()
	if i25Reader == nil || i25Registry.revision != i25Card.SchemaRevision() || len(i25Registry.definitions) == 0 || i25MaxWords == 0 || i25MaxWords > i25Card.Limits().MaxTotalWords || i25MaxOccurrences == 0 || i25MaxOccurrences > i25Card.Limits().MaxOccurrences || i25MaxSourceViews == 0 || i25MaxSourceViews > MaxSunSpecPhaseOneChainWords {
		return nil, fmt.Errorf("invalid I25 registry, reader or finite bounds")
	}
	i25Copy := SunSpecDecoderRegistry{revision: i25Registry.revision, keys: append([]SunSpecDecoderKey(nil), i25Registry.keys...), definitions: make(map[SunSpecDecoderKey]sunSpecModelDefinition, len(i25Registry.definitions))}
	for i25Key, i25Definition := range i25Registry.definitions {
		i25Definition.points = cloneSunSpecPointDefinitions(i25Definition.points)
		for i25Index := range i25Definition.points {
			if i25Definition.points[i25Index].fixedScale != nil {
				i25Scale := *i25Definition.points[i25Index].fixedScale
				i25Definition.points[i25Index].fixedScale = &i25Scale
			}
		}
		i25Copy.definitions[i25Key] = i25Definition
	}
	return &I25FroniusAdapter{i25Lifetime: I25FroniusLifetime{&i25FroniusIdentity{true}}, i25Registry: i25Copy, i25MaxWords: i25MaxWords, i25MaxOccurrences: i25MaxOccurrences, i25MaxSourceViews: i25MaxSourceViews, i25Reader: i25Reader}, nil
}

func (i25P *I25FroniusAdapter) i25Check(i25Lifetime I25FroniusLifetime) error {
	if i25P.i25Lifetime.i25Identity == nil || i25P.i25Lifetime != i25Lifetime || i25P.i25Retired {
		return fmt.Errorf("inactive I25 adapter lifetime")
	}
	return nil
}

// I25Run synchronously delegates to the existing cycle. Only the offline reader
// executes outside the adapter lock. Native publication serializes with retire
// and cancel, and the native token resolves overlapping runs. No goroutine,
// scheduler, transport or second retained store is created by the adapter.
func (i25P *I25FroniusAdapter) I25Run(i25Ctx context.Context, i25Lifetime I25FroniusLifetime, i25Request FroniusOfflineRequest) error {
	if i25P == nil {
		return fmt.Errorf("nil I25 adapter")
	}
	i25P.i25Mu.Lock()
	defer i25P.i25Mu.Unlock()
	if i25Err := i25P.i25Check(i25Lifetime); i25Err != nil {
		return i25Err
	}
	var i25Reader FroniusOfflineReader
	if i25Request.MaxWords <= i25P.i25MaxWords && i25Request.MaxOccurrences <= i25P.i25MaxOccurrences && i25Request.MaxSourceViews <= i25P.i25MaxSourceViews {
		i25Reader = func(i25Ctx context.Context, i25Read FroniusOfflineReadRequest) (FroniusOfflineReadResult, error) {
			i25P.i25Mu.Unlock()
			// Reacquire even when the callback panics so the outer unlock is balanced.
			defer i25P.i25Mu.Lock()
			return i25P.i25Reader(i25Ctx, i25Read)
		}
	}
	// A refused ceiling still passes through native Begin's compatibility fence;
	// nil reader causes native Fail without invoking the configured supplier.
	return i25P.i25Native.Run(i25Ctx, i25P.i25Registry, i25Request, i25Reader)
}

// I25Cancel closes one native attempt without retiring its adapter lifetime.
func (i25P *I25FroniusAdapter) I25Cancel(i25Lifetime I25FroniusLifetime, i25Token FroniusRetainedToken) error {
	if i25P == nil {
		return fmt.Errorf("nil I25 adapter")
	}
	i25P.i25Mu.Lock()
	defer i25P.i25Mu.Unlock()
	if i25Err := i25P.i25Check(i25Lifetime); i25Err != nil {
		return i25Err
	}
	return i25P.i25Native.Cancel(i25Token)
}

// I25Retire permanently refuses new work and cancels the active native token.
// A success that published first remains detached historical evidence.
func (i25P *I25FroniusAdapter) I25Retire(i25Lifetime I25FroniusLifetime) error {
	if i25P == nil {
		return fmt.Errorf("nil I25 adapter")
	}
	i25P.i25Mu.Lock()
	defer i25P.i25Mu.Unlock()
	if i25Err := i25P.i25Check(i25Lifetime); i25Err != nil {
		return i25Err
	}
	i25P.i25Retired = true
	i25View := i25P.i25Native.View()
	if i25View.Outcome == FroniusRetainedPending {
		_ = i25P.i25Native.Cancel(i25View.Attempt.Token)
	}
	return nil
}

// I25View returns current native status and retained evidence as independent
// copies. The current native attempt may be newer than a returning Run call.
func (i25P *I25FroniusAdapter) I25View() I25FroniusView {
	if i25P == nil {
		return I25FroniusView{I25Retired: true}
	}
	i25P.i25Mu.Lock()
	defer i25P.i25Mu.Unlock()
	return I25FroniusView{I25Lifetime: i25P.i25Lifetime, I25Retired: i25P.i25Retired || i25P.i25Lifetime.i25Identity == nil, I25Native: i25P.i25Native.View()}
}
