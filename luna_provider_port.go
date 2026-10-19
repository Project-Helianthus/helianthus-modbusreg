package modbusreg

import (
	"context"
	"errors"
	"sync"
)

type I15LunaSupplier func(context.Context, LunaOptionalInput) (LunaOptionalInput, error)

type I15LunaProviderPort interface {
	I15Cycle(context.Context, int64) error
	I15Latest() I15LunaView
	I15Retire()
}

type I15LunaView struct {
	I15Generation string
	I15State      LunaOptionalState
	I15Receipt    LunaOptionalInput
	I15HasReceipt bool
}

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

func (i15P *I15LunaProvider) I15Retire() {
	if i15P == nil {
		return
	}
	i15P.i15Mu.Lock()
	defer i15P.i15Mu.Unlock()
	i15P.i15Retired = true
}
