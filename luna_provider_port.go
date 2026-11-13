package modbusreg

import (
	"context"
	"errors"
	"sync"
)

// I15LunaSupplier is the caller-owned evidence callback of one cycle. It
// receives a copy of the plan with Started and Now set to the cycle start and
// returns the observed input; it must honor cancellation and return bounded
// data. The result must keep the plan's binding, policy and Started, and pass
// the same validation as the plan, or the cycle is refused.
type I15LunaSupplier func(context.Context, LunaOptionalInput) (LunaOptionalInput, error)

// I15LunaProviderPort is the provider surface for the optional LUNA2000
// group: run one supplied cycle, read the latest view, and retire. It opens no
// connection and grants no admission.
type I15LunaProviderPort interface {
	// I15Cycle runs one supplied cycle that starts at the given tick.
	I15Cycle(context.Context, int64) error
	// I15Latest returns a detached copy of the current view.
	I15Latest() I15LunaView
	// I15Retire permanently stops further cycles.
	I15Retire()
}

// I15LunaView is a detached snapshot of a provider: its generation, the
// reduced state and the last accepted input. Until a cycle is accepted,
// I15HasReceipt is false and I15Receipt is the zero input.
type I15LunaView struct {
	I15Generation string
	I15State      LunaOptionalState
	I15Receipt    LunaOptionalInput
	I15HasReceipt bool
}

// I15LunaProvider runs supplied cycles for one generation and binding and
// reduces them into a LunaOptionalState. It is safe for concurrent use; at
// most one cycle runs at a time, and the supplier is called without the lock
// held. Create it with I15NewLunaProvider.
type I15LunaProvider struct {
	i15Mu                              sync.Mutex
	i15Generation                      string
	i15Plan                            LunaOptionalInput
	i15Supplier                        I15LunaSupplier
	i15State                           LunaOptionalState
	i15Receipt                         LunaOptionalInput
	i15HasReceipt, i15Busy, i15Retired bool
}

var errI15Refused = errors.New("I15 supplied cycle refused")

// I15NewLunaProvider validates the plan and returns a provider for the
// generation. A refused plan, empty generation or nil supplier returns an
// error. The plan's evidence is validated and then dropped, and its event is
// reset to LunaNoObservation, so construction never establishes presence.
func I15NewLunaProvider(i15Generation string, i15Plan LunaOptionalInput, i15Supplier I15LunaSupplier) (*I15LunaProvider, error) {
	if i15Generation == "" || i15Supplier == nil || !i15ValidInput(i15Plan) {
		return nil, errI15Refused
	}
	// Constructor evidence is validated but never published or used as presence.
	i15Plan.Evidence = nil
	i15Plan.Event = LunaNoObservation
	return &I15LunaProvider{i15Generation: i15Generation, i15Plan: i15Plan, i15Supplier: i15Supplier}, nil
}

func i15CopyInput(i15In LunaOptionalInput) LunaOptionalInput {
	i15In.Evidence = append([]LunaOptionalEvidence(nil), i15In.Evidence...)
	return i15In
}

func i15ValidInput(i15In LunaOptionalInput) bool {
	if !i15In.Binding.valid() || !i15In.Policy.valid() || !i15In.ParentIdentified || !i15In.EpochsAssociated || i15In.ObservationParentEpoch != i15In.Binding.ParentEpoch || i15In.ObservationGroupEpoch != i15In.Binding.GroupEpoch || i15In.Started < 0 || i15In.Now < i15In.Started || i15In.Now >= i15In.Policy.Deadline || len(i15In.Evidence) > i15In.Policy.MaxRecords {
		return false
	}
	i15Bytes := 0
	if lunaRecordBytes(i15In.Binding, LunaOptionalEvidence{}) > i15In.Policy.MaxBytes {
		return false
	}
	for _, i15Evidence := range i15In.Evidence {
		i15Size := lunaRecordBytes(i15In.Binding, i15Evidence)
		if i15Evidence.Reference == "" || i15Evidence.NativeOutcome == "" || i15Evidence.ObservedAt < 0 || i15Evidence.ObservedAt > i15In.Now || i15Size > i15In.Policy.MaxBytes-i15Bytes {
			return false
		}
		i15Bytes += i15Size
	}
	return true
}

// I15Cycle calls the supplier exactly once and reduces its result into the
// state. It refuses, without calling the supplier, a nil context, a cancelled
// context, a retired or busy provider, and a start tick that is negative,
// earlier than the last evaluation or not before the policy deadline. A
// supplier error is returned unchanged and leaves the state as it was; a
// result that arrives after retirement or cancellation, or that fails
// validation, is refused and not committed. No goroutine, retry or timer is
// started.
func (i15P *I15LunaProvider) I15Cycle(i15Ctx context.Context, i15Started int64) error {
	if i15P == nil || i15Ctx == nil {
		return errI15Refused
	}
	i15P.i15Mu.Lock()
	if i15P.i15Supplier == nil || i15P.i15Retired || i15P.i15Busy || i15Ctx.Err() != nil || i15Started < 0 || i15Started < i15P.i15State.evaluatedAt || i15Started >= i15P.i15Plan.Policy.Deadline {
		i15P.i15Mu.Unlock()
		return errI15Refused
	}
	i15P.i15Busy = true
	i15Request := i15CopyInput(i15P.i15Plan)
	i15Request.Started = i15Started
	i15Request.Now = i15Started
	i15P.i15Mu.Unlock()
	defer func() { i15P.i15Mu.Lock(); i15P.i15Busy = false; i15P.i15Mu.Unlock() }()
	// Exactly one synchronous supplied callback; it must honor cancellation and
	// return bounded data. No goroutine, retry, timer, or acquisition is started.
	i15Result, i15Err := i15P.i15Supplier(i15Ctx, i15Request)
	i15P.i15Mu.Lock()
	defer i15P.i15Mu.Unlock()
	if i15Err != nil {
		return i15Err
	}
	if i15P.i15Retired || i15Ctx.Err() != nil || i15Result.Binding != i15P.i15Plan.Binding || i15Result.Policy != i15P.i15Plan.Policy || i15Result.Started != i15Started || !i15ValidInput(i15Result) {
		return errI15Refused
	}
	i15Result = i15CopyInput(i15Result)
	i15P.i15State = ReduceLunaOptionalState(i15P.i15State, i15Result)
	i15P.i15Receipt = i15Result
	i15P.i15HasReceipt = true
	return nil
}

// I15Latest returns a detached copy of the current view; a nil provider
// returns the zero view.
func (i15P *I15LunaProvider) I15Latest() I15LunaView {
	if i15P == nil {
		return I15LunaView{}
	}
	i15P.i15Mu.Lock()
	defer i15P.i15Mu.Unlock()
	i15State := i15P.i15State
	i15State.history = i15State.History()
	return I15LunaView{I15Generation: i15P.i15Generation, I15State: i15State, I15Receipt: i15CopyInput(i15P.i15Receipt), I15HasReceipt: i15P.i15HasReceipt}
}

// I15Retire marks the provider retired. Later cycles are refused, a cycle in
// flight cannot commit, and the last view stays readable. It is idempotent.
func (i15P *I15LunaProvider) I15Retire() {
	if i15P == nil {
		return
	}
	i15P.i15Mu.Lock()
	defer i15P.i15Mu.Unlock()
	i15P.i15Retired = true
}
