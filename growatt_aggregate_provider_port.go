package modbusreg

import (
	"context"
	"fmt"
	"sync"
)

// I23GrowattProvider is a local supplied-evidence port, not a gateway provider.
// All callbacks are synchronous and must cooperate with cancellation and bounds.
type I23GrowattProvider interface {
	I23Refresh(context.Context, I23GrowattRequest, func() uint64) I23GrowattResult
	I23Advance(uint64) error
	I23Snapshot() I23GrowattSnapshot
	I23Retire()
}

// I23GrowattLink binds caller correlation to an actual aggregate success.
// Receipt is an opaque base-record token, never a device identifier.
type I23GrowattLink struct {
	I23Context                           GrowattAggregateContext
	I23Cycle, I23Generation, I23Revision uint64
	I23Receipt                           string
}

type I23GrowattDiagnostic struct {
	I23Link          I23GrowattLink
	I23Context       GrowattDiagnosticCommitContext
	I23ExpectedGroup uint16
	I23Input         GrowattBMSMultiGroupDiagnosticInput
}

type I23GrowattRequest struct {
	I23Cycle      GrowattAggregateOfflineCycle
	I23Generation uint64
	I23Receipt    string
	I23Diagnostic *I23GrowattDiagnostic
}

// I23GrowattSnapshot never presents retained evidence as a new successful base.
// Diagnostics are optional and apply only to I23Link, never a later attempt.
type I23GrowattSnapshot struct {
	I23Aggregate  GrowattAggregateSnapshot
	I23Link       I23GrowattLink
	I23Diagnostic *GrowattDiagnosticCommitCandidate
	I23Retired    bool
}

type I23GrowattResult struct {
	I23Snapshot                           I23GrowattSnapshot
	I23AggregateError, I23DiagnosticError error
	I23DiagnosticRejection                GrowattDiagnosticCommitRejection
}

// I23GrowattAdapter owns one native provider and an atomic published envelope.
// Overlapping refreshes are refused without invoking the supplier. Advance and
// retirement may run during a cooperative supplier callback. No timer is owned.
type I23GrowattAdapter struct {
	i23Mu                               sync.Mutex
	i23Native                           *GrowattAggregateOfflineProvider
	i23Busy, i23Retired                 bool
	i23Generation, i23Deadline, i23Tick uint64
	i23Cancel                           context.CancelFunc
	i23Published                        I23GrowattSnapshot
}

func I23NewGrowattProvider(i23MaxTicks, i23MaxSlices, i23MaxWords uint64, i23Reader GrowattAggregateOfflineReader) (*I23GrowattAdapter, error) {
	// Refuse capacities that cannot hold even the canonical complete base.
	if i23MaxSlices < 4 || i23MaxWords < 50 {
		return nil, fmt.Errorf("I23 incomplete base capacity")
	}
	i23Native, i23Err := NewGrowattAggregateOfflineProvider(i23MaxTicks, i23MaxSlices, i23MaxWords, i23Reader)
	if i23Err != nil {
		return nil, i23Err
	}
	return &I23GrowattAdapter{i23Native: i23Native, i23Published: I23GrowattSnapshot{I23Aggregate: i23Native.Snapshot()}}, nil
}

func (i23P *I23GrowattAdapter) I23Refresh(i23Ctx context.Context, i23Request I23GrowattRequest, i23Clock func() uint64) I23GrowattResult {
	if i23P == nil {
		return I23GrowattResult{I23AggregateError: fmt.Errorf("I23 nil provider")}
	}
	i23P.i23Mu.Lock()
	if i23P.i23Native == nil || i23P.i23Retired || i23P.i23Busy || i23Ctx == nil || i23Clock == nil || i23Request.I23Generation == 0 || i23Request.I23Generation <= i23P.i23Generation || i23Request.I23Receipt == "" || i23Request.I23Receipt == i23P.i23Published.I23Link.I23Receipt || len(i23Request.I23Receipt) > 128 {
		i23Result := I23GrowattResult{I23Snapshot: i23CloneSnapshot(i23P.i23Published), I23AggregateError: fmt.Errorf("I23 refused lifecycle request")}
		i23P.i23Mu.Unlock()
		return i23Result
	}
	if i23Err := i23Ctx.Err(); i23Err != nil {
		i23Result := I23GrowattResult{I23Snapshot: i23CloneSnapshot(i23P.i23Published), I23AggregateError: i23Err}
		i23P.i23Mu.Unlock()
		return i23Result
	}
	i23Work, i23Cancel := context.WithCancel(i23Ctx)
	i23P.i23Busy = true
	i23P.i23Generation = i23Request.I23Generation
	i23P.i23Cancel = i23Cancel
	i23P.i23Mu.Unlock()
	defer i23Cancel()
	// Freeze optional caller storage before invoking the aggregate supplier.
	if i23Request.I23Diagnostic != nil {
		i23Copy := *i23Request.I23Diagnostic
		// The merged decoder accepts exactly three slices with 1/16/16 words.
		// Bound before copying; malformed extents still reach its native validation.
		if len(i23Copy.I23Input.Slices) > 3 {
			i23Copy.I23Input.Slices = nil
		} else {
			i23Copy.I23Input.Slices = append([]GrowattBMSReadOnlySlice(nil), i23Copy.I23Input.Slices...)
			for i23Index := range i23Copy.I23Input.Slices {
				if len(i23Copy.I23Input.Slices[i23Index].Words) > 16 {
					i23Copy.I23Input.Slices[i23Index].Words = nil
				} else {
					i23Copy.I23Input.Slices[i23Index].Words = append([]uint16(nil), i23Copy.I23Input.Slices[i23Index].Words...)
				}
			}
		}
		i23Request.I23Diagnostic = &i23Copy
	}
	i23AggregateErr := i23P.i23Native.Cycle(i23Work, i23Request.I23Cycle, i23Clock)
	i23Now := i23Clock()
	i23P.i23Mu.Lock()
	defer i23P.i23Mu.Unlock()
	i23P.i23Busy = false
	i23P.i23Cancel = nil
	if i23Work.Err() != nil && i23AggregateErr == nil {
		// A cancellation in the narrow interval after native commit cannot be
		// rolled back by the merged API. Retire to prevent later resurrection.
		i23P.i23Retired = true
		i23P.i23Published.I23Retired = true
		i23P.i23Published.I23Diagnostic = nil
	}
	if i23P.i23Retired {
		// Native work is private: cancellation/retirement cannot publish a late base.
		return I23GrowattResult{I23Snapshot: i23CloneSnapshot(i23P.i23Published), I23AggregateError: fmt.Errorf("I23 completion cancelled or retired")}
	}
	if i23Err := i23P.i23Native.Advance(i23Now); i23Err != nil {
		// Do not expose a privately committed success under a regressing clock.
		i23P.i23Retired = true
		i23P.i23Published.I23Retired = true
		i23P.i23Published.I23Diagnostic = nil
		return I23GrowattResult{I23Snapshot: i23CloneSnapshot(i23P.i23Published), I23AggregateError: i23Err}
	}
	i23View := i23P.i23Native.Snapshot()
	i23Next := I23GrowattSnapshot{I23Aggregate: i23View, I23Link: i23P.i23Published.I23Link}
	i23Result := I23GrowattResult{I23AggregateError: i23AggregateErr}
	if i23AggregateErr == nil && i23View.Value != nil && i23View.SuccessCycle == i23Request.I23Cycle.Cycle && i23View.SuccessContext == i23Request.I23Cycle.Context && i23View.Attempt.Outcome == "success" {
		i23Next.I23Link = I23GrowattLink{I23Context: i23View.SuccessContext, I23Cycle: i23View.SuccessCycle, I23Generation: i23Request.I23Generation, I23Revision: i23View.SuccessRevision, I23Receipt: i23Request.I23Receipt}
		if i23Request.I23Diagnostic != nil {
			if i23Now >= i23Request.I23Cycle.Deadline || i23P.i23Tick >= i23Request.I23Cycle.Deadline || i23Now < i23P.i23Tick {
				i23Result.I23DiagnosticError = fmt.Errorf("I23 diagnostic deadline or clock rejected")
			} else if i23Request.I23Diagnostic.I23Link != i23Next.I23Link {
				i23Result.I23DiagnosticError = fmt.Errorf("I23 diagnostic base association mismatch")
			} else {
				i23Raw := i23View.Value.NativeObservation().Slices()
				i23Context := GrowattDiagnosticCommitContext{BaseRecordID: i23Request.I23Receipt, SourceInstance: i23View.SuccessContext.SourceInstance, Epoch: i23View.SuccessContext.OwnerEpoch, Cycle: i23View.SuccessCycle, UnitID: i23View.SuccessContext.UnitID, Function: i23View.SuccessContext.Function, Revision: i23View.SuccessContext.Revision,
					NativeIdentity: GrowattDiagnosticCommitNativeIdentity{MCUVersion: i23Raw[0].Words[0], BMS: i23Raw[1].Words[0], Pack: i23Raw[1].Words[1], Topology: i23Raw[1].Words[18], CellSeries: i23Raw[1].Words[28]}}
				i23Base, i23Reason := NewGrowattDiagnosticCommitBase(*i23View.Value, i23Context)
				if i23Reason == "" {
					i23Candidate, i23Rejected := CheckGrowattDiagnosticCommit(i23Base, i23Request.I23Diagnostic.I23Context, i23Request.I23Diagnostic.I23ExpectedGroup, i23Request.I23Diagnostic.I23Input)
					i23Reason = i23Rejected
					if i23Reason == "" {
						i23Next.I23Diagnostic = &i23Candidate
					}
				}
				i23Result.I23DiagnosticRejection = i23Reason
			}
		}
	} else if i23Request.I23Diagnostic != nil {
		i23Result.I23DiagnosticError = fmt.Errorf("I23 no successful base for this cycle")
	}
	if i23Work.Err() != nil && i23AggregateErr == nil {
		i23P.i23Retired = true
		i23P.i23Published.I23Retired = true
		i23P.i23Published.I23Diagnostic = nil
		return I23GrowattResult{I23Snapshot: i23CloneSnapshot(i23P.i23Published), I23AggregateError: i23Work.Err()}
	}
	if i23Now > i23P.i23Tick {
		i23P.i23Tick = i23Now
	}
	i23P.i23Deadline = i23Request.I23Cycle.Deadline
	i23P.i23Published = i23Next
	i23Result.I23Snapshot = i23CloneSnapshot(i23Next)
	return i23Result
}

func (i23P *I23GrowattAdapter) I23Advance(i23Now uint64) error {
	if i23P == nil {
		return fmt.Errorf("I23 nil provider")
	}
	i23P.i23Mu.Lock()
	defer i23P.i23Mu.Unlock()
	if i23P.i23Native == nil || i23P.i23Retired || i23Now < i23P.i23Tick {
		return fmt.Errorf("I23 refused advance")
	}
	if i23Err := i23P.i23Native.Advance(i23Now); i23Err != nil {
		return i23Err
	}
	i23P.i23Tick = i23Now
	if i23Now >= i23P.i23Deadline {
		i23P.i23Published.I23Diagnostic = nil
	}
	return nil
}

func (i23P *I23GrowattAdapter) I23Retire() {
	if i23P == nil {
		return
	}
	i23P.i23Mu.Lock()
	defer i23P.i23Mu.Unlock()
	i23P.i23Retired = true
	i23P.i23Published.I23Retired = true
	i23P.i23Published.I23Diagnostic = nil
	if i23P.i23Cancel != nil {
		i23P.i23Cancel()
	}
}

func (i23P *I23GrowattAdapter) I23Snapshot() I23GrowattSnapshot {
	if i23P == nil {
		return I23GrowattSnapshot{I23Aggregate: GrowattAggregateSnapshot{State: "absent"}}
	}
	i23P.i23Mu.Lock()
	defer i23P.i23Mu.Unlock()
	return i23CloneSnapshot(i23P.i23Published)
}

func i23CloneSnapshot(i23View I23GrowattSnapshot) I23GrowattSnapshot {
	if i23View.I23Aggregate.Value != nil {
		i23Value := *i23View.I23Aggregate.Value
		i23Native := i23Value.NativeObservation()
		i23Value.nativeObservation = &i23Native
		i23View.I23Aggregate.Value = &i23Value
	}
	if i23View.I23Diagnostic != nil {
		i23Candidate := *i23View.I23Diagnostic
		i23Candidate.diagnostic = i23Candidate.Diagnostic()
		i23View.I23Diagnostic = &i23Candidate
	}
	return i23View
}
