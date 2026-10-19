package modbusreg

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func i19Fixture() (EmmaIdentityContext, EmmaIdentityInput, time.Time, I19EmmaProviderConfig) {
	i19At := time.Unix(300, 0).UTC()
	i19Context := EmmaIdentityContext{ParentSource: "synthetic-parent", SourceRevision: "synthetic-revision", ConnectionGeneration: 3, Attempt: 4, Deadline: i19At.Add(time.Minute)}
	i19Input := EmmaIdentityInput{Status: EmmaFC03Complete, Offering: "synthetic offering", Model: "EMMA-A01", Firmware: "SmartHEMS V100R024C00SPC100", Request: []byte{3, 0}, Response: []byte{3, 2, 0, 1}}
	return i19Context, i19Input, i19At, I19EmmaProviderConfig{I19Bounds: EmmaOfflineBounds{MaxReaderWork: 1, MaxInputBytes: 1024}, I19Now: func() time.Time { return i19At }, I19Reader: func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
		return i19Input, EmmaMEIFailed, nil
	}}
}

func TestI19Lifecycle(i19T *testing.T) {
	i19Context, _, _, i19Config := i19Fixture()
	i19Port, i19Err := I19NewEmmaProvider(i19Context, i19Config)
	if i19Err != nil {
		i19T.Fatal(i19Err)
	}
	var i19API I19EmmaProviderPort = i19Port
	i19Result, i19Err := i19API.I19Refresh(context.Background(), i19Context, true)
	if i19Err != nil || !i19Result.Committed() || i19Result.MEIStatus() != EmmaMEIFailed || i19Result.InventoryStatus() != EmmaInventoryBlocked || i19Result.InventoryComplete() {
		i19T.Fatal("identity/inventory separation", i19Err)
	}
	if _, i19OK := i19API.I19Identity(); !i19OK {
		i19T.Fatal("current identity missing")
	}
	i19API.I19Retire()
	if _, i19OK := i19API.I19Identity(); i19OK {
		i19T.Fatal("retired identity current")
	}
	if _, i19OK := i19API.I19RetainedIdentity(); !i19OK {
		i19T.Fatal("retired history lost")
	}
}

func TestI19CancellationAndContextAxes(i19T *testing.T) {
	for _, i19Axis := range []string{"ParentSource", "SourceRevision", "ConnectionGeneration", "Attempt", "Deadline", "Cancelled", "refresh-cancel", "retire", "expiry"} {
		i19T.Run(i19Axis, func(i19T *testing.T) {
			i19Context, i19Input, i19At, i19Config := i19Fixture()
			var i19Action func()
			i19Config.I19Now = func() time.Time { return i19At }
			i19Config.I19Reader = func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
				if i19Action != nil {
					i19Action()
				}
				return i19Input, EmmaMEISucceeded, nil
			}
			i19Port, i19Err := I19NewEmmaProvider(i19Context, i19Config)
			if i19Err != nil {
				i19T.Fatal(i19Err)
			}
			i19Seed, i19Err := i19Port.I19Refresh(context.Background(), i19Context, false)
			if i19Err != nil || !i19Seed.Committed() {
				i19T.Fatal("seed")
			}
			i19Next := i19Context
			i19Next.Attempt++
			if i19Err = i19Port.I19SetContext(i19Next); i19Err != nil {
				i19T.Fatal(i19Err)
			}
			i19Refresh, i19Cancel := context.WithCancel(context.Background())
			defer i19Cancel()
			i19At = i19At.Add(time.Second)
			i19Action = func() {
				i19Current := i19Next
				switch i19Axis {
				case "ParentSource":
					i19Current.ParentSource = "other-parent"
				case "SourceRevision":
					i19Current.SourceRevision = "other-revision"
				case "ConnectionGeneration":
					i19Current.ConnectionGeneration++
				case "Attempt":
					i19Current.Attempt++
				case "Deadline":
					i19Current.Deadline = i19Current.Deadline.Add(time.Second)
				case "Cancelled":
					i19Current.Cancelled = true
				case "refresh-cancel":
					i19Cancel()
					return
				case "retire":
					i19Port.I19Retire()
					return
				case "expiry":
					i19At = i19Next.Deadline
					return
				}
				if i19Err := i19Port.I19SetContext(i19Current); i19Err != nil {
					i19T.Fatal(i19Err)
				}
			}
			i19Result, _ := i19Port.I19Refresh(i19Refresh, i19Next, true)
			if i19Result.Committed() || i19Result.IdentityMatch() || i19Result.Reason() != EmmaCommitContextRejected {
				i19T.Fatal("late completion accepted")
			}
			if _, i19OK := i19Port.I19Identity(); i19OK {
				i19T.Fatal("stale identity current")
			}
			i19History, i19OK := i19Port.I19RetainedIdentity()
			if i19Axis == "ParentSource" {
				if i19OK {
					i19T.Fatal("foreign history")
				}
				return
			}
			if !i19OK || i19History.Context() != i19Context || !i19History.ObservedAt().Equal(time.Unix(300, 0).UTC()) || !reflect.DeepEqual(i19History.Input(), i19Input) {
				i19T.Fatal("history erased or redated")
			}
		})
	}
}

func TestI19Refusal(i19T *testing.T) {
	for _, i19Axis := range []string{"work", "bytes", "negative-bytes", "reader", "clock", "parent", "revision", "connection", "attempt", "deadline", "cancelled", "expired", "zero-time", "nil-refresh", "cancelled-refresh", "stale", "retired"} {
		i19T.Run(i19Axis, func(i19T *testing.T) {
			i19Context, i19Input, i19At, i19Config := i19Fixture()
			i19Calls := 0
			i19Config.I19Reader = func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
				i19Calls++
				return i19Input, EmmaMEISucceeded, nil
			}
			switch i19Axis {
			case "work":
				i19Config.I19Bounds.MaxReaderWork = 0
			case "bytes":
				i19Config.I19Bounds.MaxInputBytes = 0
			case "negative-bytes":
				i19Config.I19Bounds.MaxInputBytes = -1
			case "reader":
				i19Config.I19Reader = nil
			case "clock":
				i19Config.I19Now = nil
			case "parent":
				i19Context.ParentSource = ""
			case "revision":
				i19Context.SourceRevision = ""
			case "connection":
				i19Context.ConnectionGeneration = 0
			case "attempt":
				i19Context.Attempt = 0
			case "deadline":
				i19Context.Deadline = time.Time{}
			case "cancelled":
				i19Context.Cancelled = true
			case "expired":
				i19Config.I19Now = func() time.Time { return i19At.Add(time.Minute) }
			case "zero-time":
				i19Config.I19Now = func() time.Time { return time.Time{} }
			}
			i19Port, i19Err := I19NewEmmaProvider(i19Context, i19Config)
			if i19Axis != "nil-refresh" && i19Axis != "cancelled-refresh" && i19Axis != "stale" && i19Axis != "retired" {
				if i19Err == nil || i19Port != nil || i19Calls != 0 {
					i19T.Fatal("constructor refusal")
				}
				return
			}
			if i19Err != nil {
				i19T.Fatal(i19Err)
			}
			i19Refresh := context.Background()
			switch i19Axis {
			case "nil-refresh":
				i19Refresh = nil
			case "cancelled-refresh":
				i19Cancelled, i19Cancel := context.WithCancel(i19Refresh)
				i19Cancel()
				i19Refresh = i19Cancelled
			case "stale":
				i19Context.Attempt++
			case "retired":
				i19Port.I19Retire()
				if i19Port.I19SetContext(i19Context) == nil {
					i19T.Fatal("resurrection")
				}
			}
			i19Result, i19Err := i19Port.I19Refresh(i19Refresh, i19Context, true)
			if i19Err == nil || i19Calls != 0 || i19Result.Committed() {
				i19T.Fatal("refused refresh supplied")
			}
		})
	}
}

func TestI19EvidenceAndDetachment(i19T *testing.T) {
	for _, i19Failure := range []string{"success", "missing", "error", "oversize", "mismatch"} {
		i19T.Run(i19Failure, func(i19T *testing.T) {
			i19Context, i19Input, i19At, i19Config := i19Fixture()
			i19Calls := 0
			i19Config.I19Reader = func(i19Got EmmaIdentityContext, i19Bounds EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
				i19Calls++
				if i19Bounds != (EmmaOfflineBounds{MaxReaderWork: 1, MaxInputBytes: 1024}) {
					i19T.Fatal("configuration not frozen")
				}
				if i19Calls == 1 {
					return i19Input, EmmaMEIFailed, nil
				}
				switch i19Failure {
				case "missing":
					i19Input.Status = EmmaFC03Missing
				case "error":
					return i19Input, EmmaMEISucceeded, errors.New("synthetic failure")
				case "oversize":
					i19Input.Response = make([]byte, 1025)
				case "mismatch":
					i19Input.Model = "unknown-family"
				}
				return i19Input, EmmaMEISucceeded, nil
			}
			i19Config.I19Now = func() time.Time { return i19At }
			i19Port, i19Err := I19NewEmmaProvider(i19Context, i19Config)
			if i19Err != nil {
				i19T.Fatal(i19Err)
			}
			i19Config.I19Bounds.MaxInputBytes = 1
			i19Config.I19Reader = nil
			i19Config.I19Now = nil
			i19Seed, _ := i19Port.I19Refresh(context.Background(), i19Context, true)
			if !i19Seed.Committed() {
				i19T.Fatal("seed")
			}
			i19Context.Attempt++
			i19At = i19At.Add(time.Second)
			_ = i19Port.I19SetContext(i19Context)
			i19Result, i19Err := i19Port.I19Refresh(context.Background(), i19Context, true)
			if i19Calls != 2 || (i19Err != nil) != (i19Failure == "error" || i19Failure == "oversize") {
				i19T.Fatal("supplier error/calls", i19Err)
			}
			if i19Result.Committed() != (i19Failure == "success") || i19Result.MEIStatus() != EmmaMEISucceeded {
				i19T.Fatal("MEI rescued identity")
			}
			if i19Result.AutomaticRuntimeAdmission() || i19Result.LiveQualified() || i19Result.CatalogRegistered() || i19Result.SupportClaim() || i19Result.WriteAuthority() || !i19Result.DefaultDenied() || i19Result.Qualification() != HuaweiQualificationEvidenceBlocked || i19Result.InventoryStatus() != EmmaInventoryBlocked || i19Result.InventoryComplete() {
				i19T.Fatal("documentary promotion")
			}
			i19History, i19OK := i19Port.I19RetainedIdentity()
			if !i19OK {
				i19T.Fatal("history")
			}
			i19WantAt := time.Unix(300, 0).UTC()
			i19WantContext := i19Context
			i19WantContext.Attempt = 4
			if i19Failure == "success" {
				i19WantAt = i19WantAt.Add(time.Second)
				i19WantContext.Attempt = 5
			}
			if i19History.Context() != i19WantContext || !i19History.ObservedAt().Equal(i19WantAt) {
				i19T.Fatal("redated retained evidence")
			}
			i19ExpectedAttempt := EmmaIdentityInput{Status: EmmaFC03Complete, Offering: "synthetic offering", Model: "EMMA-A01", Firmware: "SmartHEMS V100R024C00SPC100", Request: []byte{3, 0}, Response: []byte{3, 2, 0, 1}}
			switch i19Failure {
			case "missing":
				i19ExpectedAttempt.Status = EmmaFC03Missing
			case "error":
				i19ExpectedAttempt.Status = EmmaFC03Failed
			case "mismatch":
				i19ExpectedAttempt.Model = "unknown-family"
			case "oversize":
				i19ExpectedAttempt = EmmaIdentityInput{Status: EmmaFC03Failed}
			}
			i19Input.Request[0] = 99
			i19Input.Response[0] = 99
			i19View := i19Result.AttemptInput()
			if !reflect.DeepEqual(i19View, i19ExpectedAttempt) {
				i19T.Fatal("attempt input aliases supplier or loses fields")
			}
			if !reflect.DeepEqual(i19View.Request, []byte{3, 0}) && i19Failure != "oversize" {
				i19T.Fatal("input alias")
			}
			if len(i19View.Request) > 0 {
				i19View.Request[0] = 88
			}
			if len(i19View.Response) > 0 {
				i19View.Response[0] = 88
			}
			i19Copy := i19History.Input()
			i19Copy.Request[0] = 77
			i19Copy.Response[0] = 77
			if !reflect.DeepEqual(i19Port.I19Latest().AttemptInput(), i19ExpectedAttempt) {
				i19T.Fatal("attempt output aliases publication")
			}
			i19Again, _ := i19Port.I19RetainedIdentity()
			if !reflect.DeepEqual(i19Again.Input().Request, []byte{3, 0}) || !reflect.DeepEqual(i19Again.Input().Response, []byte{3, 2, 0, 1}) {
				i19T.Fatal("output alias")
			}
		})
	}
}

func TestI19CurrentViewAxes(i19T *testing.T) {
	for _, i19Axis := range []string{"ParentSource", "SourceRevision", "ConnectionGeneration", "Attempt", "Deadline", "Cancelled"} {
		i19T.Run(i19Axis, func(i19T *testing.T) {
			i19Context, _, _, i19Config := i19Fixture()
			i19Calls := 0
			i19Reader := i19Config.I19Reader
			i19Config.I19Reader = func(i19Got EmmaIdentityContext, i19Bounds EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
				i19Calls++
				return i19Reader(i19Got, i19Bounds)
			}
			i19Port, i19Err := I19NewEmmaProvider(i19Context, i19Config)
			if i19Err != nil {
				i19T.Fatal(i19Err)
			}
			i19Seed, _ := i19Port.I19Refresh(context.Background(), i19Context, false)
			if !i19Seed.Committed() {
				i19T.Fatal("seed")
			}
			i19Current := i19Context
			switch i19Axis {
			case "ParentSource":
				i19Current.ParentSource = "other-parent"
			case "SourceRevision":
				i19Current.SourceRevision = "other-revision"
			case "ConnectionGeneration":
				i19Current.ConnectionGeneration++
			case "Attempt":
				i19Current.Attempt++
			case "Deadline":
				i19Current.Deadline = i19Current.Deadline.Add(time.Second)
			case "Cancelled":
				i19Current.Cancelled = true
			}
			if i19Err := i19Port.I19SetContext(i19Current); i19Err != nil {
				i19T.Fatal(i19Err)
			}
			if _, i19OK := i19Port.I19Identity(); i19OK {
				i19T.Fatal("old attempt became current")
			}
			i19Result, i19Err := i19Port.I19Refresh(context.Background(), i19Context, true)
			if i19Err == nil || i19Calls != 1 || i19Result.Committed() {
				i19T.Fatal("stale request supplied")
			}
		})
	}
}

func TestI19IdentityReplacementAxes(i19T *testing.T) {
	for _, i19Axis := range []string{"Offering", "Model", "Firmware", "SourceRevision", "ConnectionGeneration", "Attempt", "Deadline"} {
		i19T.Run(i19Axis, func(i19T *testing.T) {
			i19Context, i19Input, i19At, i19Config := i19Fixture()
			i19Config.I19Reader = func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
				return i19Input, EmmaMEIFailed, nil
			}
			i19Config.I19Now = func() time.Time { return i19At }
			i19Port, i19Err := I19NewEmmaProvider(i19Context, i19Config)
			if i19Err != nil {
				i19T.Fatal(i19Err)
			}
			i19Seed, _ := i19Port.I19Refresh(context.Background(), i19Context, true)
			if !i19Seed.Committed() {
				i19T.Fatal("seed")
			}
			switch i19Axis {
			case "Offering":
				i19Input.Offering = "other synthetic offering"
			case "Model":
				i19Input.Model = "EMMA-A02"
			case "Firmware":
				i19Input.Firmware = "SmartHEMS V100R024C00SPC101"
			case "SourceRevision":
				i19Context.SourceRevision = "other-revision"
			case "ConnectionGeneration":
				i19Context.ConnectionGeneration++
			case "Attempt":
				i19Context.Attempt++
			case "Deadline":
				i19Context.Deadline = i19Context.Deadline.Add(time.Second)
			}
			i19At = i19At.Add(time.Second)
			_ = i19Port.I19SetContext(i19Context)
			i19Result, i19Err := i19Port.I19Refresh(context.Background(), i19Context, true)
			if i19Err != nil || !i19Result.Committed() || i19Result.InventoryAssociationInvalidated() != (i19Axis != "Attempt" && i19Axis != "Deadline") {
				i19T.Fatal("identity axis omitted or local token treated as native identity")
			}
			i19Current, i19OK := i19Port.I19Identity()
			if !i19OK || !reflect.DeepEqual(i19Current, i19Input) {
				i19T.Fatal("changed tuple lost")
			}
			i19Current.Request[0] = 99
			i19Current.Response[0] = 99
			i19Again, _ := i19Port.I19Identity()
			if !reflect.DeepEqual(i19Again.Request, []byte{3, 0}) || !reflect.DeepEqual(i19Again.Response, []byte{3, 2, 0, 1}) {
				i19T.Fatal("current output alias")
			}
		})
	}
}

func TestI19LateReplacementAndIsolation(i19T *testing.T) {
	for _, i19ParentChange := range []bool{false, true} {
		i19Context, i19Input, _, i19Config := i19Fixture()
		i19OtherContext := i19Context
		i19OtherContext.ParentSource = "independent-parent"
		i19Other, i19Err := I19NewEmmaProvider(i19OtherContext, i19Config)
		if i19Err != nil {
			i19T.Fatal(i19Err)
		}
		i19OtherSeed, _ := i19Other.I19Refresh(context.Background(), i19OtherContext, true)
		var i19Port *I19EmmaProvider
		var i19New EmmaIdentityResult
		i19Calls := 0
		i19Refresh, i19Cancel := context.WithCancel(context.Background())
		i19Config.I19Reader = func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
			i19Calls++
			if i19Calls == 1 {
				i19Next := i19Context
				i19Next.Attempt++
				if i19ParentChange {
					i19Next.ParentSource = "replacement-parent"
				}
				_ = i19Port.I19SetContext(i19Next)
				i19New, i19Err = i19Port.I19Refresh(context.Background(), i19Next, true)
				// Cancellation of the obsolete refresh must not cancel its replacement.
				i19Cancel()
			}
			return i19Input, EmmaMEISucceeded, nil
		}
		i19Port, i19Err = I19NewEmmaProvider(i19Context, i19Config)
		if i19Err != nil {
			i19T.Fatal(i19Err)
		}
		i19Late, _ := i19Port.I19Refresh(i19Refresh, i19Context, true)
		i19Cancel()
		if i19Err != nil || i19Calls != 2 || !i19New.Committed() || i19Late.Committed() || !reflect.DeepEqual(i19Port.I19Latest(), i19New) {
			i19T.Fatal("late publication replaced new identity")
		}
		if _, i19OK := i19Port.I19Identity(); !i19OK {
			i19T.Fatal("obsolete cancellation leaked")
		}
		if !reflect.DeepEqual(i19Other.I19Latest(), i19OtherSeed) {
			i19T.Fatal("unrelated identity modified")
		}
	}
}

func TestI19BoundedFields(i19T *testing.T) {
	for _, i19Axis := range []string{"Offering", "Model", "Firmware", "Request", "Response"} {
		i19T.Run(i19Axis, func(i19T *testing.T) {
			i19Context, i19Input, _, i19Config := i19Fixture()
			i19Config.I19Bounds.MaxInputBytes = len(i19Input.Offering) + len(i19Input.Model) + len(i19Input.Firmware) + len(i19Input.Request) + len(i19Input.Response)
			i19Config.I19Reader = func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
				return i19Input, EmmaMEISucceeded, nil
			}
			i19Port, i19Err := I19NewEmmaProvider(i19Context, i19Config)
			if i19Err != nil {
				i19T.Fatal(i19Err)
			}
			i19Seed, _ := i19Port.I19Refresh(context.Background(), i19Context, true)
			if !i19Seed.Committed() {
				i19T.Fatal("exact bound refused")
			}
			switch i19Axis {
			case "Offering":
				i19Input.Offering += "x"
			case "Model":
				i19Input.Model += "x"
			case "Firmware":
				i19Input.Firmware += "x"
			case "Request":
				i19Input.Request = append(i19Input.Request, 1)
			case "Response":
				i19Input.Response = append(i19Input.Response, 1)
			}
			i19Result, i19Err := i19Port.I19Refresh(context.Background(), i19Context, true)
			if i19Err == nil || i19Result.Committed() || i19Result.AttemptInput().Status != EmmaFC03Failed || len(i19Result.AttemptInput().Request) != 0 || len(i19Result.AttemptInput().Response) != 0 {
				i19T.Fatal("oversize admitted or retained")
			}
			i19History, i19OK := i19Port.I19RetainedIdentity()
			i19Expected, _ := i19Seed.RetainedIdentity()
			if !i19OK || !reflect.DeepEqual(i19History, i19Expected) {
				i19T.Fatal("oversize replaced history")
			}
		})
	}
}

func TestI19EmptyProvider(i19T *testing.T) {
	for _, i19Port := range []*I19EmmaProvider{nil, {}} {
		if i19Port.I19SetContext(EmmaIdentityContext{}) == nil {
			i19T.Fatal("empty context accepted")
		}
		i19Port.I19Retire()
		if _, i19Err := i19Port.I19Refresh(context.Background(), EmmaIdentityContext{}, false); i19Err == nil {
			i19T.Fatal("empty refresh accepted")
		}
		if _, i19OK := i19Port.I19Identity(); i19OK {
			i19T.Fatal("empty current identity")
		}
		if _, i19OK := i19Port.I19RetainedIdentity(); i19OK {
			i19T.Fatal("empty retained identity")
		}
	}
}

func TestI19ForeignCancellationPreservesCurrent(i19T *testing.T) {
	for _, i19Axis := range []string{"ParentSource", "SourceRevision", "ConnectionGeneration", "Attempt", "Deadline", "Cancelled"} {
		i19T.Run(i19Axis, func(i19T *testing.T) {
			i19Current, _, _, i19Config := i19Fixture()
			i19Calls := 0
			i19Reader := i19Config.I19Reader
			i19Config.I19Reader = func(i19Got EmmaIdentityContext, i19Bounds EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
				i19Calls++
				return i19Reader(i19Got, i19Bounds)
			}
			i19Port, i19Err := I19NewEmmaProvider(i19Current, i19Config)
			if i19Err != nil {
				i19T.Fatal(i19Err)
			}
			i19Seed, _ := i19Port.I19Refresh(context.Background(), i19Current, true)
			if !i19Seed.Committed() {
				i19T.Fatal("seed")
			}
			i19Foreign := i19Current
			switch i19Axis {
			case "ParentSource":
				i19Foreign.ParentSource = "foreign-parent"
			case "SourceRevision":
				i19Foreign.SourceRevision = "foreign-revision"
			case "ConnectionGeneration":
				i19Foreign.ConnectionGeneration++
			case "Attempt":
				i19Foreign.Attempt++
			case "Deadline":
				i19Foreign.Deadline = i19Foreign.Deadline.Add(time.Second)
			case "Cancelled":
				i19Foreign.Cancelled = true
			}
			i19Refresh, i19Cancel := context.WithCancel(context.Background())
			i19Cancel()
			i19Result, i19Err := i19Port.I19Refresh(i19Refresh, i19Foreign, true)
			if i19Err == nil || i19Result.Committed() || i19Calls != 1 {
				i19T.Fatal("foreign refresh supplied")
			}
			if _, i19OK := i19Port.I19Identity(); !i19OK {
				i19T.Fatal("foreign cancellation poisoned current identity")
			}
			if !reflect.DeepEqual(i19Port.I19Latest(), i19Seed) {
				i19T.Fatal("foreign cancellation changed latest")
			}
		})
	}
}
