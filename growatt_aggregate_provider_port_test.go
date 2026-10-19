package modbusreg

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func i23Input() GrowattBMSReadOnlyInput {
	return GrowattBMSReadOnlyInput{UnitID: 1, Function: FunctionReadHoldingRegisters,
		Revision: GrowattBMSRevisionTuple{Family: "1xSxxP ESS", FileRevision: "Rev2.01", HeaderVersion: "V2.0", CumulativeRevision: "2.02"},
		Slices: []GrowattBMSReadOnlySlice{
			{Offset: 1, Words: []uint16{0x0102, 0, 0, 0, 0, 0, 0}},
			{Offset: 13, Words: []uint16{0x0204, 0x0301, 0, 0, 0, 0, 2, 0, 75, 5200, 100, 25, 0, 3200, 5000, 0, 0, 0, 0x0300, 0, 0, 0, 0, 0, 0, 0, 0, 0, 32}},
			{Offset: 256, Words: make([]uint16, 12)}, {Offset: 269, Words: make([]uint16, 2)}}}
}

func i23Request() I23GrowattRequest {
	i23Cycle := GrowattAggregateOfflineCycle{Context: GrowattAggregateContext{SourceInstance: "synthetic-source", OwnerEpoch: 1, UnitID: 1, Function: FunctionReadHoldingRegisters,
		Revision: i23Input().Revision, IdentityKnown: true, Identity: GrowattAggregateIdentity{MCUSoftwareVersion: "1.2", BMSCompany: 4, BMSGeneration: 2, PackCompany: 1, PackGeneration: 3, Topology: 0x0300, CellSeriesCount: 32}}, Cycle: 1, Deadline: 10}
	i23Link := I23GrowattLink{I23Context: i23Cycle.Context, I23Cycle: 1, I23Generation: 1, I23Receipt: "synthetic-receipt", I23Revision: 1}
	i23Diagnostic := GrowattBMSMultiGroupDiagnosticInput{UnitID: 1, Function: FunctionReadHoldingRegisters, Revision: i23Input().Revision,
		Slices: []GrowattBMSReadOnlySlice{{Offset: 0x70, Words: []uint16{9}}, {Offset: 0x71, Words: make([]uint16, 16)}, {Offset: 0x81, Words: make([]uint16, 16)}}}
	for i23Index := 0; i23Index < 16; i23Index++ {
		i23Diagnostic.Slices[1].Words[i23Index] = uint16(3201 + i23Index)
		i23Diagnostic.Slices[2].Words[i23Index] = uint16(3301 + i23Index)
	}
	return I23GrowattRequest{I23Cycle: i23Cycle, I23Generation: 1, I23Receipt: "synthetic-receipt", I23Diagnostic: &I23GrowattDiagnostic{I23Link: i23Link, I23Context: GrowattDiagnosticCommitContext{
		BaseRecordID: "synthetic-receipt", SourceInstance: "synthetic-source", Epoch: 1, Cycle: 1, UnitID: 1, Function: FunctionReadHoldingRegisters, Revision: i23Input().Revision,
		NativeIdentity: GrowattDiagnosticCommitNativeIdentity{MCUVersion: 0x0102, BMS: 0x0204, Pack: 0x0301, Topology: 0x0300, CellSeries: 32}}, I23ExpectedGroup: 9, I23Input: i23Diagnostic}}
}

func i23Port(i23T *testing.T, i23Reader GrowattAggregateOfflineReader) *I23GrowattAdapter {
	i23T.Helper()
	i23Port, i23Err := I23NewGrowattProvider(10, 4, 50, i23Reader)
	if i23Err != nil {
		i23T.Fatal(i23Err)
	}
	return i23Port
}

func TestI23SuccessDetached(i23T *testing.T) {
	i23InputValue := i23Input()
	i23RequestValue := i23Request()
	var i23Provider I23GrowattProvider = i23Port(i23T, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
		return i23InputValue, nil
	})
	i23Result := i23Provider.I23Refresh(context.Background(), i23RequestValue, func() uint64 { return 1 })
	if i23Result.I23AggregateError != nil || i23Result.I23DiagnosticError != nil || i23Result.I23DiagnosticRejection != "" || i23Result.I23Snapshot.I23Diagnostic == nil {
		i23T.Fatalf("%+v", i23Result)
	}
	i23View := i23Result.I23Snapshot
	if i23View.I23Aggregate.State != "current" || i23View.I23Aggregate.SuccessTick != 1 || i23View.I23Aggregate.SuccessCycle != 1 || i23View.I23Aggregate.SuccessRevision != 1 || i23View.I23Aggregate.Value.SOCPercent != 75 || i23View.I23Aggregate.Value.OutboundAllowed() || i23View.I23Diagnostic.Diagnostic().OutboundAllowed() {
		i23T.Fatal("wrong native outcome")
	}
	if i23View.I23Link != i23RequestValue.I23Diagnostic.I23Link || i23View.I23Diagnostic.Context() != i23RequestValue.I23Diagnostic.I23Context || i23View.I23Diagnostic.Diagnostic().CellVoltageGroups[0].Millivolts[0] != 3201 {
		i23T.Fatal("lost association")
	}
	i23Before := i23Provider.I23Snapshot()
	i23InputValue.Slices[1].Words[8] = 4
	i23RequestValue.I23Diagnostic.I23Input.Slices[1].Words[0] = 5
	i23View.I23Aggregate.Value.nativeObservation.slices[1].Words[8] = 6
	i23View.I23Diagnostic.diagnostic.rawSlices[2].Words[0] = 7
	if !reflect.DeepEqual(i23Before, i23Provider.I23Snapshot()) {
		i23T.Fatal("input/output aliases stored evidence")
	}
}

func TestI23DiagnosticLinkAxes(i23T *testing.T) {
	for i23Name, i23Mutate := range i23LinkAxes() {
		i23T.Run(i23Name, func(i23T *testing.T) {
			i23RequestValue := i23Request()
			i23Mutate(&i23RequestValue.I23Diagnostic.I23Link)
			i23Provider := i23Port(i23T, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
				return i23Input(), nil
			})
			i23Result := i23Provider.I23Refresh(context.Background(), i23RequestValue, func() uint64 { return 1 })
			if i23Result.I23AggregateError != nil || i23Result.I23DiagnosticError == nil || i23Result.I23Snapshot.I23Diagnostic != nil || i23Result.I23Snapshot.I23Aggregate.Value == nil || i23Result.I23Snapshot.I23Aggregate.SuccessTick != 1 {
				i23T.Fatalf("foreign link accepted or base lost: %+v", i23Result)
			}
		})
	}
}

func i23LinkAxes() map[string]func(*I23GrowattLink) {
	i23Axes := map[string]func(*I23GrowattLink){
		"cycle": func(i23L *I23GrowattLink) { i23L.I23Cycle++ }, "generation": func(i23L *I23GrowattLink) { i23L.I23Generation++ }, "receipt": func(i23L *I23GrowattLink) { i23L.I23Receipt += "-other" }, "success_revision": func(i23L *I23GrowattLink) { i23L.I23Revision++ },
	}
	for i23Name, i23Mutate := range i23ContextAxes() {
		i23Axes[i23Name] = func(i23L *I23GrowattLink) { i23Mutate(&i23L.I23Context) }
	}
	return i23Axes
}

func i23ContextAxes() map[string]func(*GrowattAggregateContext) {
	return map[string]func(*GrowattAggregateContext){
		"source": func(i23C *GrowattAggregateContext) { i23C.SourceInstance += "-other" }, "epoch": func(i23C *GrowattAggregateContext) { i23C.OwnerEpoch++ }, "unit": func(i23C *GrowattAggregateContext) { i23C.UnitID++ }, "function": func(i23C *GrowattAggregateContext) { i23C.Function = FunctionReadInputRegisters },
		"family": func(i23C *GrowattAggregateContext) { i23C.Revision.Family += "-other" }, "file_revision": func(i23C *GrowattAggregateContext) { i23C.Revision.FileRevision += "-other" }, "header_version": func(i23C *GrowattAggregateContext) { i23C.Revision.HeaderVersion += "-other" }, "cumulative_revision": func(i23C *GrowattAggregateContext) { i23C.Revision.CumulativeRevision += "-other" },
		"identity_known": func(i23C *GrowattAggregateContext) { i23C.IdentityKnown = false }, "mcu": func(i23C *GrowattAggregateContext) { i23C.Identity.MCUSoftwareVersion = "9.9" }, "bms_company": func(i23C *GrowattAggregateContext) { i23C.Identity.BMSCompany++ }, "bms_generation": func(i23C *GrowattAggregateContext) { i23C.Identity.BMSGeneration++ }, "pack_company": func(i23C *GrowattAggregateContext) { i23C.Identity.PackCompany++ }, "pack_generation": func(i23C *GrowattAggregateContext) { i23C.Identity.PackGeneration++ }, "topology": func(i23C *GrowattAggregateContext) { i23C.Identity.Topology++ }, "cell_series": func(i23C *GrowattAggregateContext) { i23C.Identity.CellSeriesCount++ },
	}
}

func TestI23Retention(i23T *testing.T) {
	i23Fail := false
	i23Provider := i23Port(i23T, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
		if i23Fail {
			return GrowattBMSReadOnlyInput{}, errors.New("synthetic failure")
		}
		return i23Input(), nil
	})
	i23RequestValue := i23Request()
	i23First := i23Provider.I23Refresh(context.Background(), i23RequestValue, func() uint64 { return 1 })
	if i23First.I23AggregateError != nil {
		i23T.Fatal(i23First.I23AggregateError)
	}
	i23Fail = true
	i23RequestValue.I23Cycle.Cycle = 2
	i23RequestValue.I23Generation = 2
	i23RequestValue.I23Receipt = "synthetic-next"
	i23Result := i23Provider.I23Refresh(context.Background(), i23RequestValue, func() uint64 { return 2 })
	if i23Result.I23AggregateError == nil || i23Result.I23Snapshot.I23Aggregate.State != "retained" || i23Result.I23Snapshot.I23Aggregate.SuccessTick != 1 || i23Result.I23Snapshot.I23Aggregate.SuccessCycle != 1 || i23Result.I23Snapshot.I23Aggregate.SuccessRevision != 1 || i23Result.I23Snapshot.I23Aggregate.Value.SOCPercent != 75 || i23Result.I23Snapshot.I23Diagnostic != nil || i23Result.I23Snapshot.I23Link != i23First.I23Snapshot.I23Link {
		i23T.Fatalf("retained evidence redated: %+v", i23Result)
	}
}

func i23DiagnosticAxes() map[string]func(*GrowattDiagnosticCommitContext) {
	return map[string]func(*GrowattDiagnosticCommitContext){
		"base_record":         func(i23C *GrowattDiagnosticCommitContext) { i23C.BaseRecordID += "-other" },
		"source":              func(i23C *GrowattDiagnosticCommitContext) { i23C.SourceInstance += "-other" },
		"epoch":               func(i23C *GrowattDiagnosticCommitContext) { i23C.Epoch++ },
		"cycle":               func(i23C *GrowattDiagnosticCommitContext) { i23C.Cycle++ },
		"unit":                func(i23C *GrowattDiagnosticCommitContext) { i23C.UnitID++ },
		"function":            func(i23C *GrowattDiagnosticCommitContext) { i23C.Function = FunctionReadInputRegisters },
		"family":              func(i23C *GrowattDiagnosticCommitContext) { i23C.Revision.Family += "-other" },
		"file_revision":       func(i23C *GrowattDiagnosticCommitContext) { i23C.Revision.FileRevision += "-other" },
		"header_version":      func(i23C *GrowattDiagnosticCommitContext) { i23C.Revision.HeaderVersion += "-other" },
		"cumulative_revision": func(i23C *GrowattDiagnosticCommitContext) { i23C.Revision.CumulativeRevision += "-other" },
		"mcu":                 func(i23C *GrowattDiagnosticCommitContext) { i23C.NativeIdentity.MCUVersion++ },
		"bms":                 func(i23C *GrowattDiagnosticCommitContext) { i23C.NativeIdentity.BMS++ },
		"pack":                func(i23C *GrowattDiagnosticCommitContext) { i23C.NativeIdentity.Pack++ },
		"topology":            func(i23C *GrowattDiagnosticCommitContext) { i23C.NativeIdentity.Topology++ },
		"cell_series":         func(i23C *GrowattDiagnosticCommitContext) { i23C.NativeIdentity.CellSeries++ },
	}
}

func TestI23DiagnosticContextAxes(i23T *testing.T) {
	for i23Name, i23Mutate := range i23DiagnosticAxes() {
		i23T.Run(i23Name, func(i23T *testing.T) {
			i23RequestValue := i23Request()
			i23Mutate(&i23RequestValue.I23Diagnostic.I23Context)
			i23Provider := i23Port(i23T, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
				return i23Input(), nil
			})
			i23Result := i23Provider.I23Refresh(context.Background(), i23RequestValue, func() uint64 { return 1 })
			i23Want := GrowattDiagnosticCommitContextMismatch
			if i23Name == "topology" {
				i23Want = GrowattDiagnosticCommitTopologyChanged
			}
			if i23Result.I23AggregateError != nil || i23Result.I23DiagnosticRejection != i23Want || i23Result.I23Snapshot.I23Diagnostic != nil || i23Result.I23Snapshot.I23Aggregate.Value.SOCPercent != 75 {
				i23T.Fatalf("foreign diagnostic context: %+v", i23Result)
			}
		})
	}
}

func TestI23DiagnosticFailuresPreserveBase(i23T *testing.T) {
	for _, i23Mode := range []string{"absent", "partial", "extra", "oversize", "unit", "function", "family", "file_revision", "header_version", "cumulative_revision", "group"} {
		i23T.Run(i23Mode, func(i23T *testing.T) {
			i23RequestValue := i23Request()
			switch i23Mode {
			case "absent":
				i23RequestValue.I23Diagnostic = nil
			case "partial":
				i23RequestValue.I23Diagnostic.I23Input.Slices = i23RequestValue.I23Diagnostic.I23Input.Slices[:2]
			case "extra":
				i23RequestValue.I23Diagnostic.I23Input.Slices = append(i23RequestValue.I23Diagnostic.I23Input.Slices, GrowattBMSReadOnlySlice{})
			case "oversize":
				i23RequestValue.I23Diagnostic.I23Input.Slices[1].Words = make([]uint16, 17)
			case "unit":
				i23RequestValue.I23Diagnostic.I23Input.UnitID++
			case "function":
				i23RequestValue.I23Diagnostic.I23Input.Function = FunctionReadInputRegisters
			case "family":
				i23RequestValue.I23Diagnostic.I23Input.Revision.Family += "-other"
			case "file_revision":
				i23RequestValue.I23Diagnostic.I23Input.Revision.FileRevision += "-other"
			case "header_version":
				i23RequestValue.I23Diagnostic.I23Input.Revision.HeaderVersion += "-other"
			case "cumulative_revision":
				i23RequestValue.I23Diagnostic.I23Input.Revision.CumulativeRevision += "-other"
			case "group":
				i23RequestValue.I23Diagnostic.I23ExpectedGroup = 10
			}
			i23Provider := i23Port(i23T, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
				return i23Input(), nil
			})
			i23Result := i23Provider.I23Refresh(context.Background(), i23RequestValue, func() uint64 { return 1 })
			i23Want := GrowattDiagnosticCommitInvalidDiagnostic
			if i23Mode == "group" {
				i23Want = GrowattDiagnosticCommitLineageChanged
			}
			if i23Mode == "absent" {
				i23Want = ""
			}
			if i23Result.I23AggregateError != nil || i23Result.I23DiagnosticRejection != i23Want || i23Result.I23Snapshot.I23Diagnostic != nil || i23Result.I23Snapshot.I23Aggregate.SuccessTick != 1 || i23Result.I23Snapshot.I23Aggregate.SuccessRevision != 1 || i23Result.I23Snapshot.I23Aggregate.Value.SOCPercent != 75 {
				i23T.Fatalf("diagnostic failure lost base: %+v", i23Result)
			}
		})
	}
}

func TestI23Refusals(i23T *testing.T) {
	i23Calls := 0
	i23Reader := func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
		i23Calls++
		return i23Input(), nil
	}
	for _, i23Bounds := range [][3]uint64{{0, 4, 50}, {10, 0, 50}, {10, 3, 50}, {10, 4, 49}, {10, 4, 0}} {
		if i23PortValue, i23Err := I23NewGrowattProvider(i23Bounds[0], i23Bounds[1], i23Bounds[2], i23Reader); i23Err == nil || i23PortValue != nil {
			i23T.Fatal("invalid construction")
		}
	}
	if i23PortValue, i23Err := I23NewGrowattProvider(10, 4, 50, nil); i23Err == nil || i23PortValue != nil {
		i23T.Fatal("nil reader")
	}
	for _, i23Mode := range []string{"generation", "receipt", "long_receipt", "nil_context", "cancelled", "nil_clock", "deadline", "source", "epoch", "unit", "fc04", "revision", "unknown", "retired", "zero_provider", "nil_provider"} {
		i23T.Run(i23Mode, func(i23T *testing.T) {
			i23Provider := i23Port(i23T, i23Reader)
			i23RequestValue := i23Request()
			i23Ctx := context.Background()
			i23Clock := func() uint64 { return 1 }
			switch i23Mode {
			case "generation":
				i23RequestValue.I23Generation = 0
			case "receipt":
				i23RequestValue.I23Receipt = ""
			case "long_receipt":
				i23RequestValue.I23Receipt = string(make([]byte, 129))
			case "nil_context":
				i23Ctx = nil
			case "cancelled":
				i23Work, i23Cancel := context.WithCancel(i23Ctx)
				i23Cancel()
				i23Ctx = i23Work
			case "nil_clock":
				i23Clock = nil
			case "deadline":
				i23RequestValue.I23Cycle.Deadline = 1
			case "source":
				i23RequestValue.I23Cycle.Context.SourceInstance = ""
			case "epoch":
				i23RequestValue.I23Cycle.Context.OwnerEpoch = 0
			case "unit":
				i23RequestValue.I23Cycle.Context.UnitID = 0
			case "fc04":
				i23RequestValue.I23Cycle.Context.Function = FunctionReadInputRegisters
			case "revision":
				i23RequestValue.I23Cycle.Context.Revision.FileRevision = "unsupported"
			case "unknown":
				i23RequestValue.I23Cycle.Context.IdentityKnown = false
			case "retired":
				i23Provider.I23Retire()
			case "zero_provider":
				i23Provider = &I23GrowattAdapter{}
			case "nil_provider":
				i23Provider = nil
			}
			if i23Result := i23Provider.I23Refresh(i23Ctx, i23RequestValue, i23Clock); i23Result.I23AggregateError == nil {
				i23T.Fatal("refusal accepted")
			}
			if i23Calls != 0 {
				i23T.Fatal("refusal invoked supplier")
			}
		})
	}
}

func TestI23Lifecycle(i23T *testing.T) {
	for _, i23Mode := range []string{"cancel", "retire", "expire", "overlap"} {
		i23T.Run(i23Mode, func(i23T *testing.T) {
			i23Ctx, i23Cancel := context.WithCancel(context.Background())
			defer i23Cancel()
			i23Tick := uint64(1)
			i23Calls := 0
			var i23Provider *I23GrowattAdapter
			i23Provider = i23Port(i23T, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
				i23Calls++
				if i23Calls == 2 {
					switch i23Mode {
					case "cancel":
						i23Cancel()
					case "retire":
						i23Provider.I23Retire()
					case "expire":
						i23Tick = 10
						if i23Err := i23Provider.I23Advance(10); i23Err != nil {
							i23T.Fatal(i23Err)
						}
					case "overlap":
						i23Other := i23Request()
						i23Other.I23Generation = 3
						i23Other.I23Cycle.Cycle = 3
						i23Other.I23Receipt = "synthetic-overlap"
						if i23Provider.I23Refresh(context.Background(), i23Other, func() uint64 { return i23Tick }).I23AggregateError == nil {
							i23T.Fatal("overlap accepted")
						}
					}
				}
				i23InputValue := i23Input()
				if i23Calls == 2 {
					i23InputValue.Slices[1].Words[8] = 22
				}
				return i23InputValue, nil
			})
			i23First := i23Provider.I23Refresh(context.Background(), i23Request(), func() uint64 { return i23Tick })
			if i23First.I23AggregateError != nil {
				i23T.Fatal(i23First.I23AggregateError)
			}
			i23Next := i23Request()
			i23Next.I23Generation = 2
			i23Next.I23Cycle.Cycle = 2
			i23Next.I23Receipt = "synthetic-next"
			i23Result := i23Provider.I23Refresh(i23Ctx, i23Next, func() uint64 { return i23Tick })
			if i23Calls != 2 {
				i23T.Fatal("wrong supplier count")
			}
			if i23Mode == "overlap" {
				if i23Result.I23AggregateError != nil || i23Result.I23Snapshot.I23Aggregate.Value.SOCPercent != 22 {
					i23T.Fatal("outer refresh failed")
				}
				return
			}
			if i23Result.I23AggregateError == nil || i23Result.I23Snapshot.I23Aggregate.SuccessTick != 1 || i23Result.I23Snapshot.I23Aggregate.SuccessRevision != 1 || i23Result.I23Snapshot.I23Aggregate.Value.SOCPercent != 75 || i23Result.I23Snapshot.I23Diagnostic != nil {
				i23T.Fatalf("late work published: %+v", i23Result)
			}
			if i23Mode == "expire" && i23Result.I23Snapshot.I23Aggregate.Attempt.Outcome != "incomplete" {
				i23T.Fatal("expiry not observed")
			}
			if i23Mode == "retire" {
				i23Next.I23Generation = 3
				if i23Provider.I23Refresh(context.Background(), i23Next, func() uint64 { return i23Tick }).I23AggregateError == nil || i23Calls != 2 || !i23Provider.I23Snapshot().I23Retired {
					i23T.Fatal("retired provider reused")
				}
			}
		})
	}
}

func TestI23DiagnosticExpiryAndFreeze(i23T *testing.T) {
	i23RequestValue := i23Request()
	i23Provider := i23Port(i23T, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
		i23RequestValue.I23Diagnostic.I23Input.Slices[1].Words[0] = 99
		return i23Input(), nil
	})
	i23Result := i23Provider.I23Refresh(context.Background(), i23RequestValue, func() uint64 { return 1 })
	if i23Result.I23Snapshot.I23Diagnostic == nil || i23Result.I23Snapshot.I23Diagnostic.Diagnostic().CellVoltageGroups[0].Millivolts[0] != 3201 {
		i23T.Fatal("optional input not frozen before supplier")
	}
	if i23Err := i23Provider.I23Advance(10); i23Err != nil {
		i23T.Fatal(i23Err)
	}
	i23View := i23Provider.I23Snapshot()
	if i23View.I23Diagnostic != nil || i23View.I23Aggregate.SuccessTick != 1 || i23View.I23Aggregate.Value.SOCPercent != 75 {
		i23T.Fatal("expiry redated or removed aggregate")
	}
	if i23Provider.I23Advance(9) == nil {
		i23T.Fatal("backward tick accepted")
	}
}

func TestI23PostNativeCancellation(i23T *testing.T) {
	i23Ctx, i23Cancel := context.WithCancel(context.Background())
	defer i23Cancel()
	i23Calls := 0
	i23ClockCalls := 0
	i23Provider := i23Port(i23T, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
		i23Calls++
		return i23Input(), nil
	})
	i23Result := i23Provider.I23Refresh(i23Ctx, i23Request(), func() uint64 {
		i23ClockCalls++
		if i23ClockCalls == 3 {
			i23Cancel()
		}
		return 1
	})
	if i23Result.I23AggregateError == nil || i23Result.I23Snapshot.I23Aggregate.Value != nil || !i23Result.I23Snapshot.I23Retired {
		i23T.Fatal("post-native cancellation published")
	}
	i23Next := i23Request()
	i23Next.I23Generation = 2
	i23Next.I23Cycle.Cycle = 2
	i23Next.I23Receipt = "synthetic-next"
	if i23Provider.I23Refresh(context.Background(), i23Next, func() uint64 { return 2 }).I23AggregateError == nil || i23Calls != 1 {
		i23T.Fatal("hidden native success resurrected")
	}
}

func TestI23AggregateContinuityAxes(i23T *testing.T) {
	for i23Name, i23Mutate := range i23ContextAxes() {
		i23T.Run(i23Name, func(i23T *testing.T) {
			i23Fail := false
			i23Provider := i23Port(i23T, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
				if i23Fail {
					return GrowattBMSReadOnlyInput{}, errors.New("synthetic failure")
				}
				return i23Input(), nil
			})
			i23First := i23Provider.I23Refresh(context.Background(), i23Request(), func() uint64 { return 1 })
			if i23First.I23AggregateError != nil {
				i23T.Fatal(i23First.I23AggregateError)
			}
			i23RequestValue := i23Request()
			i23RequestValue.I23Generation = 2
			i23RequestValue.I23Receipt = "synthetic-next"
			i23RequestValue.I23Cycle.Cycle = 2
			i23Fail = true
			i23Mutate(&i23RequestValue.I23Cycle.Context)
			i23Result := i23Provider.I23Refresh(context.Background(), i23RequestValue, func() uint64 { return 2 })
			if i23Result.I23AggregateError == nil || i23Result.I23Snapshot.I23Aggregate.State != "invalidated" || i23Result.I23Snapshot.I23Aggregate.Value != nil || i23Result.I23Snapshot.I23Aggregate.SuccessTick != 1 || i23Result.I23Snapshot.I23Aggregate.SuccessRevision != 1 || i23Result.I23Snapshot.I23Diagnostic != nil {
				i23T.Fatalf("changed continuity revived base: %+v", i23Result)
			}
		})
	}
}

func TestI23ReplayRefusals(i23T *testing.T) {
	for _, i23Mode := range []string{"generation", "receipt", "cycle", "epoch"} {
		i23T.Run(i23Mode, func(i23T *testing.T) {
			i23Calls := 0
			i23Provider := i23Port(i23T, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
				i23Calls++
				return i23Input(), nil
			})
			if i23Provider.I23Refresh(context.Background(), i23Request(), func() uint64 { return 1 }).I23AggregateError != nil {
				i23T.Fatal("seed failed")
			}
			i23RequestValue := i23Request()
			i23RequestValue.I23Generation = 2
			i23RequestValue.I23Receipt = "synthetic-next"
			i23RequestValue.I23Cycle.Cycle = 2
			switch i23Mode {
			case "generation":
				i23RequestValue.I23Generation = 1
			case "receipt":
				i23RequestValue.I23Receipt = "synthetic-receipt"
			case "cycle":
				i23RequestValue.I23Cycle.Cycle = 1
			case "epoch":
				i23RequestValue.I23Cycle.Context.OwnerEpoch = 0
			}
			i23Result := i23Provider.I23Refresh(context.Background(), i23RequestValue, func() uint64 { return 2 })
			if i23Result.I23AggregateError == nil || i23Calls != 1 || i23Result.I23Snapshot.I23Aggregate.SuccessTick != 1 || i23Result.I23Snapshot.I23Aggregate.SuccessRevision != 1 {
				i23T.Fatal("replay called supplier or renewed base")
			}
		})
	}
}

func TestI23DiagnosticCompletionDeadline(i23T *testing.T) {
	i23Provider := i23Port(i23T, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
		return i23Input(), nil
	})
	i23Calls := 0
	i23Result := i23Provider.I23Refresh(context.Background(), i23Request(), func() uint64 {
		i23Calls++
		if i23Calls >= 3 {
			return 10
		}
		return 1
	})
	if i23Result.I23AggregateError != nil || i23Result.I23DiagnosticError == nil || i23Result.I23Snapshot.I23Diagnostic != nil || i23Result.I23Snapshot.I23Aggregate.SuccessTick != 1 || i23Result.I23Snapshot.I23Aggregate.Value.SOCPercent != 75 {
		i23T.Fatal("diagnostic expiry erased timely base or admitted late diagnostic")
	}
}

func TestI23DocumentarySuccessDeniesFC04Admission(i23T *testing.T) {
	// Parsing a documentary record is a success; native identity remains Unknown.
	i23Bytes, i23Err := json.Marshal(GrowattFC04Record{Family: "MAX", DeviceType: 17, ModelBuildWord82: 23, ModelBuildWord83: 41, ReportedProtocolWord: 256, SourceSchemaRef: "synthetic:source", Function: 4, Offset: 0, WidthWords: 59, FieldSchemaRef: "synthetic:fields", TupleEvidenceRef: "synthetic:tuple"})
	if i23Err != nil {
		i23T.Fatal(i23Err)
	}
	if _, i23Err = ParseGrowattFC04Record(i23Bytes); i23Err != nil {
		i23T.Fatal(i23Err)
	}
	i23Admission, i23Err := ValidateGrowattFC04RecordAdmission(i23Bytes, GrowattProtocolIIIdentityObservation{})
	if i23Admission != "NO_ADMISSIBLE_PROFILE" || i23Err == nil {
		i23T.Fatal("documentary success became admission")
	}
}

func TestI23PartialAndZeroReplacement(i23T *testing.T) {
	i23Mode := "seed"
	i23Provider := i23Port(i23T, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
		i23Value := i23Input()
		if i23Mode == "partial" {
			i23Value.Slices = i23Value.Slices[:3]
		}
		if i23Mode == "zero" {
			i23Value.Slices[1].Words[8] = 0
		}
		return i23Value, nil
	})
	if i23Provider.I23Refresh(context.Background(), i23Request(), func() uint64 { return 1 }).I23AggregateError != nil {
		i23T.Fatal("seed failed")
	}
	for i23Index, i23NextMode := range []string{"partial", "zero"} {
		i23Mode = i23NextMode
		i23RequestValue := i23Request()
		i23RequestValue.I23Cycle.Cycle = uint64(i23Index + 2)
		i23RequestValue.I23Generation = uint64(i23Index + 2)
		i23RequestValue.I23Receipt = "synthetic-" + i23Mode
		i23RequestValue.I23Diagnostic = nil
		i23Result := i23Provider.I23Refresh(context.Background(), i23RequestValue, func() uint64 { return uint64(i23Index + 2) })
		if i23Mode == "partial" {
			if i23Result.I23AggregateError == nil || i23Result.I23Snapshot.I23Aggregate.State != "retained" || i23Result.I23Snapshot.I23Aggregate.SuccessTick != 1 || i23Result.I23Snapshot.I23Aggregate.SuccessRevision != 1 || i23Result.I23Snapshot.I23Aggregate.Value.SOCPercent != 75 {
				i23T.Fatal("partial base replaced last good")
			}
		} else {
			if i23Result.I23AggregateError != nil || i23Result.I23Snapshot.I23Aggregate.State != "current" || i23Result.I23Snapshot.I23Aggregate.SuccessTick != 3 || i23Result.I23Snapshot.I23Aggregate.SuccessRevision != 2 || i23Result.I23Snapshot.I23Aggregate.Value.SOCPercent != 0 {
				i23T.Fatal("valid zero lost")
			}
		}
	}
}

func TestI23PostNativeClockRegression(i23T *testing.T) {
	i23ClockCalls := 0
	i23Provider := i23Port(i23T, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
		return i23Input(), nil
	})
	i23Result := i23Provider.I23Refresh(context.Background(), i23Request(), func() uint64 {
		i23ClockCalls++
		if i23ClockCalls == 3 {
			return 1
		}
		return 2
	})
	if i23Result.I23AggregateError == nil || i23Result.I23Snapshot.I23Aggregate.Value != nil || !i23Result.I23Snapshot.I23Retired {
		i23T.Fatal("regressing completion clock published hidden success")
	}
}
