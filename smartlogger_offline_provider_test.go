package modbusreg

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func offlineLimits() SmartloggerOfflineInventoryLimits {
	return SmartloggerOfflineInventoryLimits{MaxPages: 2, MaxObjects: 3, MaxBytes: 256}
}

func offlineProvider(t *testing.T, reader SmartloggerOfflineInventoryReader) (*SmartloggerOfflineProvider, *time.Time) {
	t.Helper()
	now := time.Time{}.Add(time.Hour)
	p, err := NewSmartloggerOfflineProvider(smartloggerCommitContext(), func() time.Time { return now }, reader)
	if err != nil {
		t.Fatal(err)
	}
	return p, &now
}

func TestSmartloggerOfflineProviderCompleteDetached(t *testing.T) {
	input := smartloggerCommitFixture()
	calls := 0
	p, now := offlineProvider(t, func(plan SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
		calls++
		if plan.Context != smartloggerCommitContext() || plan.AttemptToken != 1 || !plan.Deadline.Equal(time.Time{}.Add(time.Hour+10*time.Second)) {
			t.Fatal("unfrozen plan")
		}
		plan.Context.OwnerToken = "changed-local-copy"
		return input, nil
	})
	r, err := p.Cycle(context.Background(), now.Add(10*time.Second), offlineLimits())
	if err != nil || r.Outcome != SmartloggerInventoryComplete || !r.DefaultDenied() || calls != 1 {
		t.Fatalf("result %+v %v calls %d", r, err, calls)
	}
	input.Pages[0].Objects[1].Value[0] = 'X'
	input.Pages[1].Objects = nil
	input.Pages[0] = HuaweiSmartLoggerInventoryPage{}
	s, ok := p.LastComplete()
	if !ok || !s.Current || !s.Inventory.DefaultDenied() || s.Inventory.ChildCount() != 1 || string(s.Native.Pages[0].Objects[1].Value) != "1=SmartLogger;2=V300R024C10SPC191" {
		t.Fatal("aliased input")
	}
	s.Native.Pages[0].Objects[1].Value[0] = 'Y'
	s.Native.Pages[1].Objects = nil
	s.Native.Pages[0] = HuaweiSmartLoggerInventoryPage{}
	s.Inventory.children[0] = HuaweiSmartLoggerInventoryChild{}
	again, _ := p.LastComplete()
	if string(again.Native.Pages[0].Objects[1].Value) != "1=SmartLogger;2=V300R024C10SPC191" || again.Inventory.Children()[0].Address() != "1" {
		t.Fatal("aliased output")
	}
}

func TestSmartloggerOfflineProviderInvalidBeforeReader(t *testing.T) {
	for _, name := range []string{"nil context", "cancelled", "deadline", "pages zero", "pages high", "objects zero", "objects high", "bytes zero", "bytes high"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			p, now := offlineProvider(t, func(SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
				calls++
				return smartloggerCommitFixture(), nil
			})
			ctx := context.Background()
			limits := offlineLimits()
			deadline := now.Add(time.Second)
			switch name {
			case "nil context":
				ctx = nil
			case "cancelled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			case "deadline":
				deadline = *now
			case "pages zero":
				limits.MaxPages = 0
			case "pages high":
				limits.MaxPages = 249
			case "objects zero":
				limits.MaxObjects = 0
			case "objects high":
				limits.MaxObjects = 250
			case "bytes zero":
				limits.MaxBytes = 0
			case "bytes high":
				limits.MaxBytes = 65537
			}
			if _, err := p.Cycle(ctx, deadline, limits); err == nil || calls != 0 {
				t.Fatalf("invalid called reader %d: %v", calls, err)
			}
		})
	}
	if _, err := NewSmartloggerOfflineProvider(smartloggerCommitContext(), nil, nil); err == nil {
		t.Fatal("nil reader")
	}
	var zero SmartloggerOfflineProvider
	if _, err := zero.Cycle(context.Background(), time.Now().Add(time.Second), offlineLimits()); err == nil {
		t.Fatal("zero provider")
	}
}

func TestSmartloggerOfflineProviderFailureRetention(t *testing.T) {
	for _, name := range []string{"reader error", "cancel at completion", "deadline", "wrong next", "late more", "pages", "objects", "bytes"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var now *time.Time
			calls := 0
			sentinel := errors.New("synthetic reader failure")
			p, clock := offlineProvider(t, func(plan SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
				calls++
				input := smartloggerCommitFixture()
				if calls == 1 {
					return input, nil
				}
				switch name {
				case "reader error":
					return input, sentinel
				case "cancel at completion":
					cancel()
				case "deadline":
					*now = plan.Deadline
				case "wrong next":
					input.Pages[0].NextObjectID++
				case "late more":
					input.Pages[1].More = true
					input.Pages[1].NextObjectID = 0x8a
				}
				return input, nil
			})
			now = clock
			if r, err := p.Cycle(ctx, now.Add(time.Second), offlineLimits()); err != nil || r.Outcome != SmartloggerInventoryComplete {
				t.Fatal("seed", err)
			}
			before, _ := p.LastComplete()
			*now = now.Add(time.Second)
			limits := offlineLimits()
			switch name {
			case "pages":
				limits.MaxPages = 1
			case "objects":
				limits.MaxObjects = 2
			case "bytes":
				limits.MaxBytes = 1
			}
			r, err := p.Cycle(ctx, now.Add(time.Second), limits)
			want := SmartloggerInventoryFailed
			if name == "cancel at completion" {
				want = SmartloggerInventoryCancelled
			}
			if r.Outcome != want || !r.DefaultDenied() || calls != 2 {
				t.Fatalf("result %+v err %v calls %d", r, err, calls)
			}
			if name == "reader error" && !errors.Is(err, sentinel) {
				t.Fatal("reader error lost")
			}
			after, _ := p.LastComplete()
			if !reflect.DeepEqual(before, after) {
				t.Fatal("failure replaced history or renewed age")
			}
		})
	}
}

func TestSmartloggerOfflineProviderLateCompletion(t *testing.T) {
	for _, event := range []string{"close", "retire", "cancel"} {
		t.Run(event, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			calls := 0
			p, now := offlineProvider(t, func(SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
				calls++
				if calls == 2 {
					close(entered)
					<-release
				}
				return smartloggerCommitFixture(), nil
			})
			if r, err := p.Cycle(context.Background(), now.Add(time.Second), offlineLimits()); err != nil || r.Outcome != SmartloggerInventoryComplete {
				t.Fatalf("seed result %+v: %v", r, err)
			}
			before, _ := p.LastComplete()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan SmartloggerInventoryResult, 1)
			go func() { r, _ := p.Cycle(ctx, now.Add(time.Second), offlineLimits()); done <- r }()
			<-entered
			if _, err := p.Cycle(context.Background(), now.Add(time.Second), offlineLimits()); !errors.Is(err, ErrSmartloggerInventoryBusy) {
				t.Fatal("busy", err)
			}
			want := SmartloggerInventoryCancelled
			switch event {
			case "close":
				p.Close()
				before.Current = false
				want = SmartloggerInventoryClosed
			case "retire":
				c := smartloggerCommitContext()
				c.ConnectionGeneration++
				if err := p.Retire(c); err != nil {
					t.Fatal(err)
				}
				before.Current = false
				want = SmartloggerInventoryRetired
			case "cancel":
				cancel()
			}
			close(release)
			if r := <-done; r.Outcome != want {
				t.Fatalf("late result %+v", r)
			}
			after, _ := p.LastComplete()
			if !reflect.DeepEqual(before, after) {
				t.Fatal("late publication")
			}
			if calls != 2 {
				t.Fatal("unexpected callback", calls)
			}
			if event == "close" {
				if _, err := p.Cycle(context.Background(), now.Add(time.Second), offlineLimits()); !errors.Is(err, ErrSmartloggerInventoryClosed) {
					t.Fatal("closed", err)
				}
			}
		})
	}
}

func TestSmartloggerOfflineProviderEveryFenceAxis(t *testing.T) {
	axes := map[string]func(*SmartloggerInventoryPlan){
		"OwnerToken":           func(p *SmartloggerInventoryPlan) { p.Context.OwnerToken = "other-owner" },
		"ConnectionGeneration": func(p *SmartloggerInventoryPlan) { p.Context.ConnectionGeneration = 2 },
		"RoutingContext":       func(p *SmartloggerInventoryPlan) { p.Context.Identity.RoutingContext = "other-route" },
		"UnitID":               func(p *SmartloggerInventoryPlan) { p.Context.Identity.UnitID = 1 },
		"ParentProfile":        func(p *SmartloggerInventoryPlan) { p.Context.Identity.ParentProfile = "other-parent" },
		"SourceRevision":       func(p *SmartloggerInventoryPlan) { p.Context.Identity.SourceRevision = "other-source" },
		"SelfModel":            func(p *SmartloggerInventoryPlan) { p.Context.Identity.SelfModel = "other-family" },
		"Firmware":             func(p *SmartloggerInventoryPlan) { p.Context.Identity.Firmware = "V300R024C10SPC210" },
		"AttemptToken":         func(p *SmartloggerInventoryPlan) { p.AttemptToken = 99 },
		"Deadline":             func(p *SmartloggerInventoryPlan) { p.Deadline = p.Deadline.Add(-time.Nanosecond) },
	}
	for name, mutate := range axes {
		t.Run(name, func(t *testing.T) {
			p, now := offlineProvider(t, func(SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
				return smartloggerCommitFixture(), nil
			})
			if r, err := p.Cycle(context.Background(), now.Add(time.Second), offlineLimits()); err != nil || r.Outcome != SmartloggerInventoryComplete {
				t.Fatalf("seed result %+v: %v", r, err)
			}
			before, _ := p.LastComplete()
			a, err := p.owner.Begin(now.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			plan := a.Plan()
			mutate(&plan)
			if r := a.Commit(plan, smartloggerCommitFixture()); r.Outcome != SmartloggerInventoryFailed {
				t.Fatal("altered plan accepted", name)
			}
			after, _ := p.LastComplete()
			if !reflect.DeepEqual(before, after) {
				t.Fatal("fence renewed history")
			}
		})
	}
}

func TestSmartloggerOfflineProviderRetiredCannotReleaseSuccessor(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	p, now := offlineProvider(t, func(SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
		close(entered)
		<-release
		return smartloggerCommitFixture(), nil
	})
	done := make(chan SmartloggerInventoryResult, 1)
	go func() { r, _ := p.Cycle(context.Background(), now.Add(time.Second), offlineLimits()); done <- r }()
	<-entered
	next := smartloggerCommitContext()
	next.ConnectionGeneration = 2
	if err := p.Retire(next); err != nil {
		t.Fatal(err)
	}
	successor, err := p.owner.Begin(now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	if r := <-done; r.Outcome != SmartloggerInventoryRetired {
		t.Fatal("late result", r)
	}
	if _, err := p.owner.Begin(now.Add(time.Second)); !errors.Is(err, ErrSmartloggerInventoryBusy) {
		t.Fatal("late callback released successor", err)
	}
	if r := successor.Commit(successor.Plan(), smartloggerCommitFixture()); r.Outcome != SmartloggerInventoryComplete {
		t.Fatal("successor", r)
	}
	s, ok := p.LastComplete()
	if !ok || !s.Current || s.Plan.Context.ConnectionGeneration != 2 {
		t.Fatal("successor context lost")
	}
}

// An already elapsed context deadline must refuse work even when the caller's
// separate plan deadline remains valid under the injected logical clock.
func TestSmartloggerOfflineProviderContextDeadline(t *testing.T) {
	calls := 0
	p, now := offlineProvider(t, func(SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
		calls++
		return smartloggerCommitFixture(), nil
	})
	ctx, cancel := context.WithDeadline(context.Background(), time.Time{})
	defer cancel()
	if _, err := p.Cycle(ctx, now.Add(time.Second), offlineLimits()); !errors.Is(err, context.DeadlineExceeded) || calls != 0 {
		t.Fatal("elapsed context invoked reader", err, calls)
	}
}
