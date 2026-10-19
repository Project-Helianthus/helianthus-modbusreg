package modbusreg

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func emmaProviderFixture() (EmmaIdentityContext, EmmaIdentityInput, time.Time, EmmaOfflineBounds) {
	at := time.Unix(200, 0).UTC()
	return EmmaIdentityContext{ParentSource: "synthetic-parent", SourceRevision: "synthetic-revision", ConnectionGeneration: 3, Attempt: 4, Deadline: at.Add(time.Minute)}, EmmaIdentityInput{Status: EmmaFC03Complete, Offering: "synthetic offering", Model: "EMMA-A01", Firmware: "SmartHEMS V100R024C00SPC100", Request: []byte{3, 0}, Response: []byte{3, 2, 0, 1}}, at, EmmaOfflineBounds{MaxReaderWork: 1, MaxInputBytes: 1024}
}

func TestEmmaOfflineProviderCycle(t *testing.T) {
	for _, mei := range []EmmaMEIStatus{EmmaMEIFailed, EmmaMEISucceeded, EmmaMEINotRequested} {
		for _, inventory := range []bool{false, true} {
			ctx, input, at, bounds := emmaProviderFixture()
			p := NewEmmaOfflineProvider(ctx)
			calls := 0
			result, err := p.Cycle(ctx, bounds, inventory, func(got EmmaIdentityContext, b EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
				calls++
				if got != ctx || b != bounds {
					t.Fatal("reader context/bounds changed")
				}
				return input, mei, nil
			}, func() time.Time { return at })
			wantInventory := EmmaInventoryNotRequested
			if inventory {
				wantInventory = EmmaInventoryBlocked
			}
			if err != nil || calls != 1 || !result.Committed() || !result.IdentityMatch() || result.MEIStatus() != mei || result.InventoryStatus() != wantInventory || result.InventoryComplete() {
				t.Fatalf("cycle: %+v %v", result, err)
			}
			if result.AutomaticRuntimeAdmission() || result.LiveQualified() || result.CatalogRegistered() || result.SupportClaim() || result.WriteAuthority() || !result.DefaultDenied() || result.Qualification() != HuaweiQualificationEvidenceBlocked {
				t.Fatal("promotion")
			}
			if !reflect.DeepEqual(p.Latest(), result) {
				t.Fatal("latest differs")
			}
		}
	}
}

func TestEmmaOfflineProviderUnavailableAndRetention(t *testing.T) {
	for _, failure := range []string{"missing", "failed", "reader-error", "oversized", "mismatch"} {
		t.Run(failure, func(t *testing.T) {
			ctx, input, at, bounds := emmaProviderFixture()
			p := NewEmmaOfflineProvider(ctx)
			read := func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
				return input, EmmaMEISucceeded, nil
			}
			first, err := p.Cycle(ctx, bounds, true, read, func() time.Time { return at })
			if err != nil || !first.Committed() {
				t.Fatal("seed failed")
			}
			original, _ := first.RetainedIdentity()
			expectedContext, expectedInput := ctx, input
			ctx.Attempt++
			p.SetContext(ctx)
			var readErr error
			switch failure {
			case "missing":
				input.Status = EmmaFC03Missing
			case "failed":
				input.Status = EmmaFC03Failed
			case "reader-error":
				readErr = errors.New("synthetic failure")
			case "oversized":
				bounds.MaxInputBytes = 1
			case "mismatch":
				input.Model = "unknown"
			}
			result, err := p.Cycle(ctx, bounds, true, func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
				return input, EmmaMEISucceeded, readErr
			}, func() time.Time { return at.Add(time.Second) })
			if (failure == "reader-error" || failure == "oversized") && err == nil {
				t.Fatal("error missing")
			}
			if result.Committed() || result.IdentityMatch() || result.MEIStatus() != EmmaMEISucceeded {
				t.Fatal("failed FC03 rescued")
			}
			retained, ok := result.RetainedIdentity()
			if !ok || !reflect.DeepEqual(retained, original) {
				t.Fatal("history erased or age renewed")
			}
			if retained.Context() != expectedContext || retained.ObservedAt() != at || !reflect.DeepEqual(retained.Input(), expectedInput) {
				t.Fatal("retained history differs from independent fixture")
			}
			if !reflect.DeepEqual(p.Latest(), result) {
				t.Fatal("failure not latest")
			}
		})
	}
}

func TestEmmaOfflineProviderContextFences(t *testing.T) {
	for _, axis := range []string{"ParentSource", "SourceRevision", "ConnectionGeneration", "Attempt", "Deadline", "Cancelled"} {
		t.Run(axis, func(t *testing.T) {
			ctx, input, at, bounds := emmaProviderFixture()
			p := NewEmmaOfflineProvider(ctx)
			entered := make(chan struct{})
			release := make(chan struct{})
			done := make(chan EmmaIdentityResult, 1)
			go func() {
				result, _ := p.Cycle(ctx, bounds, true, func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
					close(entered)
					<-release
					return input, EmmaMEISucceeded, nil
				}, func() time.Time { return at })
				done <- result
			}()
			<-entered
			current := ctx
			switch axis {
			case "ParentSource":
				current.ParentSource = "replacement-parent"
			case "SourceRevision":
				current.SourceRevision = "replacement-revision"
			case "ConnectionGeneration":
				current.ConnectionGeneration = 7
			case "Attempt":
				current.Attempt = 8
			case "Deadline":
				current.Deadline = at.Add(2 * time.Minute)
			case "Cancelled":
				current.Cancelled = true
			}
			p.SetContext(current)
			close(release)
			result := <-done
			if result.Committed() || result.IdentityMatch() || result.Reason() != EmmaCommitContextRejected {
				t.Fatal("post-reader fence bypassed")
			}
		})
	}
}

func TestEmmaOfflineProviderIdentityAxes(t *testing.T) {
	for _, axis := range []string{"Offering", "Model", "Firmware", "SourceRevision", "ConnectionGeneration"} {
		t.Run(axis, func(t *testing.T) {
			ctx, input, at, bounds := emmaProviderFixture()
			p := NewEmmaOfflineProvider(ctx)
			read := func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
				return input, EmmaMEIFailed, nil
			}
			first, err := p.Cycle(ctx, bounds, false, read, func() time.Time { return at })
			if err != nil || !first.Committed() {
				t.Fatal("seed")
			}
			ctx.Attempt++
			switch axis {
			case "Offering":
				input.Offering = "another synthetic offering"
			case "Model":
				input.Model = "EMMA-A02"
			case "Firmware":
				input.Firmware = "SmartHEMS V100R024C00SPC101"
			case "SourceRevision":
				ctx.SourceRevision = "another synthetic revision"
			case "ConnectionGeneration":
				ctx.ConnectionGeneration = 9
			}
			p.SetContext(ctx)
			result, err := p.Cycle(ctx, bounds, true, read, func() time.Time { return at.Add(time.Second) })
			if err != nil || !result.Committed() || !result.InventoryAssociationInvalidated() {
				t.Fatal("identity association comparison missing")
			}
		})
	}
}

func TestEmmaOfflineProviderDetached(t *testing.T) {
	ctx, input, at, bounds := emmaProviderFixture()
	p := NewEmmaOfflineProvider(ctx)
	result, err := p.Cycle(ctx, bounds, false, func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
		return input, EmmaMEIFailed, nil
	}, func() time.Time { return at })
	if err != nil {
		t.Fatal(err)
	}
	input.Request[0] = 99
	input.Response[0] = 99
	view := result.AttemptInput()
	if !reflect.DeepEqual(view.Request, []byte{3, 0}) || !reflect.DeepEqual(view.Response, []byte{3, 2, 0, 1}) {
		t.Fatal("reader slice alias")
	}
	view.Request[0] = 88
	view.Response[0] = 88
	retained, _ := p.Latest().RetainedIdentity()
	copy := retained.Input()
	copy.Request[0] = 77
	copy.Response[0] = 77
	again := p.Latest()
	saved, _ := again.RetainedIdentity()
	if !reflect.DeepEqual(again.AttemptInput().Request, []byte{3, 0}) || !reflect.DeepEqual(saved.Input().Response, []byte{3, 2, 0, 1}) {
		t.Fatal("getter alias")
	}
}

func TestEmmaOfflineProviderPreflight(t *testing.T) {
	for _, axis := range []string{"parent", "revision", "connection", "attempt", "deadline", "cancelled", "expired", "time", "work", "bytes", "negative-bytes", "reader", "clock", "stale"} {
		t.Run(axis, func(t *testing.T) {
			ctx, input, at, bounds := emmaProviderFixture()
			p := NewEmmaOfflineProvider(ctx)
			calls := 0
			var reader EmmaOfflineIdentityReader = func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
				calls++
				return input, EmmaMEIFailed, nil
			}
			clock := func() time.Time { return at }
			switch axis {
			case "parent":
				ctx.ParentSource = ""
			case "revision":
				ctx.SourceRevision = ""
			case "connection":
				ctx.ConnectionGeneration = 0
			case "attempt":
				ctx.Attempt = 0
			case "deadline":
				ctx.Deadline = time.Time{}
			case "cancelled":
				ctx.Cancelled = true
			case "expired":
				at = ctx.Deadline
			case "time":
				at = time.Time{}
			case "work":
				bounds.MaxReaderWork = 0
			case "bytes":
				bounds.MaxInputBytes = 0
			case "negative-bytes":
				bounds.MaxInputBytes = -1
			case "reader":
				reader = nil
			case "clock":
				clock = nil
			case "stale":
				ctx.Attempt++
			}
			result, err := p.Cycle(ctx, bounds, true, reader, clock)
			if err == nil || calls != 0 || result.Committed() {
				t.Fatal("invalid request invoked reader")
			}
		})
	}
}

func TestEmmaOfflineProviderDeadlineAfterReader(t *testing.T) {
	ctx, input, at, bounds := emmaProviderFixture()
	p := NewEmmaOfflineProvider(ctx)
	result, _ := p.Cycle(ctx, bounds, false, func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
		at = ctx.Deadline
		return input, EmmaMEIFailed, nil
	}, func() time.Time { return at })
	if result.Committed() || result.Reason() != EmmaCommitContextRejected {
		t.Fatal("expired completion")
	}
}

func TestEmmaOfflineProviderReplacement(t *testing.T) {
	ctx, input, at, bounds := emmaProviderFixture()
	p := NewEmmaOfflineProvider(ctx)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan EmmaIdentityResult, 1)
	go func() {
		r, _ := p.Cycle(ctx, bounds, false, func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
			close(entered)
			<-release
			return input, EmmaMEISucceeded, nil
		}, func() time.Time { return at })
		done <- r
	}()
	<-entered
	next := ctx
	next.Attempt++
	p.SetContext(next)
	latest, err := p.Cycle(next, bounds, false, func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
		return input, EmmaMEIFailed, nil
	}, func() time.Time { return at.Add(time.Second) })
	close(release)
	late := <-done
	if err != nil || !latest.Committed() || late.Committed() || !reflect.DeepEqual(p.Latest(), latest) {
		t.Fatal("late completion replaced latest attempt")
	}
}

func TestEmmaOfflineProviderBoundedEvidence(t *testing.T) {
	for _, axis := range []string{"Offering", "Model", "Firmware", "Request", "Response"} {
		t.Run(axis, func(t *testing.T) {
			ctx, input, at, bounds := emmaProviderFixture()
			size := len(input.Offering) + len(input.Model) + len(input.Firmware) + len(input.Request) + len(input.Response)
			bounds.MaxInputBytes = size
			p := NewEmmaOfflineProvider(ctx)
			read := func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
				return input, EmmaMEISucceeded, nil
			}
			control, err := p.Cycle(ctx, bounds, false, read, func() time.Time { return at })
			if err != nil || !control.Committed() {
				t.Fatal("exact bound rejected")
			}
			switch axis {
			case "Offering":
				input.Offering += "x"
			case "Model":
				input.Model += "x"
			case "Firmware":
				input.Firmware += "x"
			case "Request":
				input.Request = append(input.Request, 0)
			case "Response":
				input.Response = append(input.Response, 0)
			}
			result, err := p.Cycle(ctx, bounds, false, read, func() time.Time { return at })
			if err == nil || result.Committed() || result.AttemptInput().Status != EmmaFC03Failed || len(result.AttemptInput().Request) != 0 || len(result.AttemptInput().Response) != 0 {
				t.Fatal("oversized evidence retained")
			}
		})
	}
}

func TestEmmaOfflineProviderParentHistory(t *testing.T) {
	ctx, input, at, bounds := emmaProviderFixture()
	p := NewEmmaOfflineProvider(ctx)
	read := func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
		return input, EmmaMEISucceeded, nil
	}
	first, err := p.Cycle(ctx, bounds, false, read, func() time.Time { return at })
	if err != nil || !first.Committed() {
		t.Fatal("seed")
	}
	ctx.ParentSource = "replacement-parent"
	p.SetContext(ctx)
	input.Status = EmmaFC03Missing
	result, err := p.Cycle(ctx, bounds, false, read, func() time.Time { return at.Add(time.Second) })
	if err != nil || result.Committed() {
		t.Fatal("replacement")
	}
	if _, ok := result.RetainedIdentity(); ok {
		t.Fatal("cross-parent retained history")
	}
}

func TestEmmaOfflineProviderSameContextSuperseded(t *testing.T) {
	ctx, input, at, bounds := emmaProviderFixture()
	p := NewEmmaOfflineProvider(ctx)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan EmmaIdentityResult, 1)
	go func() {
		r, _ := p.Cycle(ctx, bounds, false, func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
			close(entered)
			<-release
			return input, EmmaMEISucceeded, nil
		}, func() time.Time { return at })
		done <- r
	}()
	<-entered
	latest, err := p.Cycle(ctx, bounds, false, func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
		return input, EmmaMEIFailed, nil
	}, func() time.Time { return at.Add(time.Second) })
	close(release)
	late := <-done
	if err != nil || !latest.Committed() || late.Committed() || !reflect.DeepEqual(p.Latest(), latest) {
		t.Fatal("superseded cycle committed")
	}
}

func TestEmmaOfflineProviderLocalContextOnly(t *testing.T) {
	ctx, input, at, bounds := emmaProviderFixture()
	p := NewEmmaOfflineProvider(ctx)
	read := func(EmmaIdentityContext, EmmaOfflineBounds) (EmmaIdentityInput, EmmaMEIStatus, error) {
		return input, EmmaMEIFailed, nil
	}
	first, err := p.Cycle(ctx, bounds, false, read, func() time.Time { return at })
	if err != nil || !first.Committed() {
		t.Fatal("seed")
	}
	ctx.Attempt++
	ctx.Deadline = at.Add(2 * time.Minute)
	p.SetContext(ctx)
	result, err := p.Cycle(ctx, bounds, false, read, func() time.Time { return at.Add(time.Second) })
	if err != nil || !result.Committed() || result.InventoryAssociationInvalidated() {
		t.Fatal("local context confused with native identity")
	}
}
