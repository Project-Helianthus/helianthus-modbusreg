package modbusreg

import (
	"reflect"
	"testing"
	"time"
)

func smartloggerCommitFixture() HuaweiSmartLoggerOfflineInventoryInput {
	return HuaweiSmartLoggerOfflineInventoryInput{ChangeCounterBefore: 12, ChangeCounterAfter: 12,
		Pages: []HuaweiSmartLoggerInventoryPage{
			{More: true, NextObjectID: 0x89, Objects: []HuaweiSmartLoggerInventoryObject{
				{ObjectID: 0x87, Value: []byte{1}},
				{ObjectID: 0x88, Value: []byte("1=SmartLogger;2=V300R024C10SPC191")},
			}},
			{Objects: []HuaweiSmartLoggerInventoryObject{{ObjectID: 0x89, Value: []byte("1=synthetic-child;5=1;8=synthetic-type")}}},
		}}
}

func smartloggerCommitContext() SmartloggerInventoryContext {
	return SmartloggerInventoryContext{OwnerToken: "synthetic-owner", ConnectionGeneration: 1,
		Identity: SmartloggerInventoryIdentity{RoutingContext: "synthetic-route", UnitID: 0,
			ParentProfile: "synthetic-parent", SourceRevision: "synthetic-source",
			SelfModel: "SmartLogger", Firmware: "V300R024C10SPC191"}}
}

func smartloggerCommitOwner(t *testing.T) (*SmartloggerInventoryCommitOwner, *time.Time) {
	t.Helper()
	now := time.Time{}.Add(time.Hour)
	owner, err := NewSmartloggerInventoryCommitOwner(smartloggerCommitContext(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return owner, &now
}

func smartloggerCommitBegin(t *testing.T, owner *SmartloggerInventoryCommitOwner, now time.Time) *SmartloggerInventoryAttempt {
	t.Helper()
	a, err := owner.Begin(now.Add(10 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func smartloggerCommitSeed(t *testing.T, owner *SmartloggerInventoryCommitOwner, now time.Time) SmartloggerInventorySnapshot {
	t.Helper()
	a := smartloggerCommitBegin(t, owner, now)
	if r := a.Commit(a.Plan(), smartloggerCommitFixture()); r.Outcome != SmartloggerInventoryComplete {
		t.Fatalf("seed: %+v", r)
	}
	s, ok := owner.LastComplete()
	if !ok || !s.Current || !s.Inventory.DefaultDenied() || s.Inventory.ChildCount() != 1 || s.Inventory.Children()[0].Attribute("product_type") != "synthetic-type" {
		t.Fatal("invalid seed")
	}
	return s
}

func TestSmartloggerInventoryCommitSuccessDetached(t *testing.T) {
	o, now := smartloggerCommitOwner(t)
	if _, ok := o.LastComplete(); ok {
		t.Fatal("unexpected history")
	}
	a := smartloggerCommitBegin(t, o, *now)
	if _, err := o.Begin(now.Add(time.Second)); err != ErrSmartloggerInventoryBusy {
		t.Fatalf("busy: %v", err)
	}
	input := smartloggerCommitFixture()
	r := a.Commit(a.Plan(), input)
	if r.Outcome != SmartloggerInventoryComplete || !r.DefaultDenied() {
		t.Fatalf("result: %+v", r)
	}
	input.Pages[0].Objects[1].Value[0] = 'X'
	s, ok := o.LastComplete()
	if !ok || !s.ObservedAt.Equal(*now) || !s.CompletedAt.Equal(*now) || s.Native.ChangeCounterBefore != 12 || s.Native.ChangeCounterAfter != 12 || s.Inventory.DeclaredChildren() != 1 {
		t.Fatal("snapshot metadata")
	}
	s.Native.Pages[0].Objects[1].Value[0] = 'Y'
	s.Inventory.children[0] = HuaweiSmartLoggerInventoryChild{}
	again, _ := o.LastComplete()
	if string(again.Native.Pages[0].Objects[1].Value) != "1=SmartLogger;2=V300R024C10SPC191" || again.Inventory.Children()[0].Address() != "1" {
		t.Fatal("aliased snapshot")
	}
	r.Outcome = SmartloggerInventoryFailed
	if got := a.Cancel(); got.Outcome != SmartloggerInventoryComplete {
		t.Fatal("terminal success changed")
	}
	if got := a.Commit(a.Plan(), HuaweiSmartLoggerOfflineInventoryInput{}); got.Outcome != SmartloggerInventoryComplete {
		t.Fatal("repeat changed terminal result")
	}
}

func TestSmartloggerInventoryCommitEveryFenceAxis(t *testing.T) {
	mutations := map[string]func(*SmartloggerInventoryPlan){
		"owner":           func(p *SmartloggerInventoryPlan) { p.Context.OwnerToken += "-other" },
		"attempt":         func(p *SmartloggerInventoryPlan) { p.AttemptToken++ },
		"generation":      func(p *SmartloggerInventoryPlan) { p.Context.ConnectionGeneration++ },
		"routing context": func(p *SmartloggerInventoryPlan) { p.Context.Identity.RoutingContext += "-other" },
		"unit":            func(p *SmartloggerInventoryPlan) { p.Context.Identity.UnitID++ },
		"parent profile":  func(p *SmartloggerInventoryPlan) { p.Context.Identity.ParentProfile += "-other" },
		"source revision": func(p *SmartloggerInventoryPlan) { p.Context.Identity.SourceRevision += "-other" },
		"self model":      func(p *SmartloggerInventoryPlan) { p.Context.Identity.SelfModel += "-other" },
		"firmware":        func(p *SmartloggerInventoryPlan) { p.Context.Identity.Firmware = "V300R024C10SPC210" },
		"deadline":        func(p *SmartloggerInventoryPlan) { p.Deadline = p.Deadline.Add(-time.Second) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			o, now := smartloggerCommitOwner(t)
			before := smartloggerCommitSeed(t, o, *now)
			*now = now.Add(time.Second)
			a := smartloggerCommitBegin(t, o, *now)
			p := a.Plan()
			mutate(&p)
			if got := a.Commit(p, smartloggerCommitFixture()); got.Outcome != SmartloggerInventoryFailed {
				t.Fatalf("accepted altered %s", name)
			}
			after, _ := o.LastComplete()
			if !reflect.DeepEqual(before, after) {
				t.Fatal("failed fence changed inventory or age")
			}
		})
	}
}

func TestSmartloggerInventoryCommitValidationRetainsAge(t *testing.T) {
	for _, name := range []string{"counter", "late malformed", "deadline exceeded", "deadline equal", "missing firmware", "wrong firmware", "other exact firmware", "wrong self", "page limit"} {
		t.Run(name, func(t *testing.T) {
			o, now := smartloggerCommitOwner(t)
			before := smartloggerCommitSeed(t, o, *now)
			*now = now.Add(time.Second)
			a := smartloggerCommitBegin(t, o, *now)
			input := smartloggerCommitFixture()
			switch name {
			case "counter":
				input.ChangeCounterAfter++
			case "late malformed":
				input.Pages[1].Objects[0].Value = []byte("1=synthetic-child;5")
			case "deadline exceeded":
				*now = a.Plan().Deadline.Add(time.Nanosecond)
			case "deadline equal":
				*now = a.Plan().Deadline
			case "missing firmware":
				input.Pages[0].Objects[1].Value = []byte("1=SmartLogger")
			case "wrong firmware":
				input.Pages[0].Objects[1].Value = []byte("1=SmartLogger;2=V300R024C10SPC191-extra")
			case "other exact firmware":
				input.Pages[0].Objects[1].Value = []byte("1=SmartLogger;2=V300R024C10SPC210")
			case "wrong self":
				input.Pages[0].Objects[1].Value = []byte("1=synthetic-other;2=V300R024C10SPC191")
			case "page limit":
				input.Pages = make([]HuaweiSmartLoggerInventoryPage, 249)
			}
			if got := a.Commit(a.Plan(), input); got.Outcome != SmartloggerInventoryFailed {
				t.Fatalf("accepted %s", name)
			}
			after, _ := o.LastComplete()
			if !reflect.DeepEqual(before, after) {
				t.Fatal("failure changed last complete or observation age")
			}
			if got := a.Cancel(); got.Outcome != SmartloggerInventoryFailed {
				t.Fatal("failure was mutable")
			}
		})
	}
}

func TestSmartloggerInventoryCommitRetirementAndSuccessor(t *testing.T) {
	for _, event := range []string{"cancel", "close", "retire"} {
		t.Run(event, func(t *testing.T) {
			o, now := smartloggerCommitOwner(t)
			before := smartloggerCommitSeed(t, o, *now)
			*now = now.Add(time.Second)
			a := smartloggerCommitBegin(t, o, *now)
			want := SmartloggerInventoryCancelled
			switch event {
			case "cancel":
				a.Cancel()
			case "close":
				o.Close()
				want = SmartloggerInventoryClosed
			case "retire":
				c := smartloggerCommitContext()
				c.ConnectionGeneration++
				if err := o.Retire(c); err != nil {
					t.Fatal(err)
				}
				want = SmartloggerInventoryRetired
			}
			after, _ := o.LastComplete()
			if event != "cancel" {
				before.Current = false
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("retirement changed retained history")
			}
			if event == "close" {
				if _, err := o.Begin(now.Add(time.Second)); err != ErrSmartloggerInventoryClosed {
					t.Fatal("closed owner accepted begin")
				}
			} else {
				b := smartloggerCommitBegin(t, o, *now)
				if b.Plan().AttemptToken == a.Plan().AttemptToken {
					t.Fatal("reused attempt token")
				}
				if r := a.Commit(a.Plan(), smartloggerCommitFixture()); r.Outcome != want {
					t.Fatal("old callback changed terminal result")
				}
				if _, err := o.Begin(now.Add(time.Second)); err != ErrSmartloggerInventoryBusy {
					t.Fatal("old callback released successor slot")
				}
				if r := b.Commit(b.Plan(), smartloggerCommitFixture()); r.Outcome != SmartloggerInventoryComplete {
					t.Fatal("successor failed")
				}
				current, _ := o.LastComplete()
				if !current.Current || !current.ObservedAt.Equal(*now) {
					t.Fatal("new bracket not current")
				}
			}
			if r := a.Commit(a.Plan(), smartloggerCommitFixture()); r.Outcome != want {
				t.Fatal("late completion changed outcome")
			}
		})
	}
}

func TestSmartloggerInventoryCommitBeginBounds(t *testing.T) {
	o, now := smartloggerCommitOwner(t)
	for _, deadline := range []time.Time{{}, *now, now.Add(16 * time.Second)} {
		if _, err := o.Begin(deadline); err == nil {
			t.Fatal("accepted invalid deadline")
		}
	}
	a, err := o.Begin(now.Add(15 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	a.Cancel()
	for _, mutate := range []func(*SmartloggerInventoryContext){
		func(c *SmartloggerInventoryContext) { c.OwnerToken = "" },
		func(c *SmartloggerInventoryContext) { c.ConnectionGeneration = 0 },
		func(c *SmartloggerInventoryContext) { c.Identity.RoutingContext = "" },
		func(c *SmartloggerInventoryContext) { c.Identity.UnitID = 1 },
		func(c *SmartloggerInventoryContext) { c.Identity.ParentProfile = "" },
		func(c *SmartloggerInventoryContext) { c.Identity.SourceRevision = "" },
		func(c *SmartloggerInventoryContext) { c.Identity.SelfModel = "other" },
		func(c *SmartloggerInventoryContext) { c.Identity.Firmware += "-later" },
	} {
		c := smartloggerCommitContext()
		mutate(&c)
		if _, err := NewSmartloggerInventoryCommitOwner(c, time.Now); err == nil {
			t.Fatal("invalid context accepted")
		}
	}
	var zero SmartloggerInventoryCommitOwner
	if _, err := zero.Begin(now.Add(time.Second)); err == nil {
		t.Fatal("zero owner accepted begin")
	}
}

func TestSmartloggerInventoryCommitCancelOrdering(t *testing.T) {
	// Channels establish both orderings without sleeps or timing assumptions.
	for _, commitFirst := range []bool{false, true} {
		o, now := smartloggerCommitOwner(t)
		a := smartloggerCommitBegin(t, o, *now)
		first, done := make(chan struct{}), make(chan SmartloggerInventoryResult)
		go func() {
			<-first
			if commitFirst {
				done <- a.Cancel()
			} else {
				done <- a.Commit(a.Plan(), smartloggerCommitFixture())
			}
		}()
		want := SmartloggerInventoryCancelled
		if commitFirst {
			a.Commit(a.Plan(), smartloggerCommitFixture())
			want = SmartloggerInventoryComplete
		} else {
			a.Cancel()
		}
		close(first)
		if got := <-done; got.Outcome != want {
			t.Fatal("serialized outcome changed")
		}
		_, ok := o.LastComplete()
		if ok != commitFirst {
			t.Fatal("cancel publication order")
		}
	}
}

func TestSmartloggerInventoryCommitFinalDeadline(t *testing.T) {
	o, now := smartloggerCommitOwner(t)
	before := smartloggerCommitSeed(t, o, *now)
	a := smartloggerCommitBegin(t, o, *now)
	calls := 0
	o.now = func() time.Time {
		calls++
		if calls == 1 {
			return *now
		}
		return a.Plan().Deadline
	}
	if r := a.Commit(a.Plan(), smartloggerCommitFixture()); r.Outcome != SmartloggerInventoryFailed {
		t.Fatal("deadline was checked only before validation")
	}
	after, _ := o.LastComplete()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("final deadline failure refreshed inventory")
	}
}

func TestSmartloggerInventoryCommitBothExactFirmwareControls(t *testing.T) {
	for _, firmware := range []string{"V300R024C10SPC191", "V300R024C10SPC210"} {
		c := smartloggerCommitContext()
		c.Identity.Firmware = firmware
		now := time.Time{}.Add(time.Hour)
		o, err := NewSmartloggerInventoryCommitOwner(c, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		a := smartloggerCommitBegin(t, o, now)
		input := smartloggerCommitFixture()
		input.Pages[0].Objects[1].Value = []byte("1=SmartLogger;2=" + firmware)
		if r := a.Commit(a.Plan(), input); r.Outcome != SmartloggerInventoryComplete || !r.DefaultDenied() {
			t.Fatal("exact firmware control denied mechanics")
		}
		o.Close()
		if r := a.Cancel(); r.Outcome != SmartloggerInventoryComplete {
			t.Fatal("close rewrote success")
		}
	}
}

func TestSmartloggerInventoryCommitRetirementWaitsForPublication(t *testing.T) {
	// Block at the final clock read while Commit holds the publication lock.
	// Retirement is attempted from another goroutine and must follow that commit.
	o, now := smartloggerCommitOwner(t)
	a := smartloggerCommitBegin(t, o, *now)
	entered, release := make(chan struct{}), make(chan struct{})
	calls := 0
	o.now = func() time.Time {
		calls++
		if calls == 2 {
			close(entered)
			<-release
		}
		return *now
	}
	committed := make(chan SmartloggerInventoryResult)
	go func() { committed <- a.Commit(a.Plan(), smartloggerCommitFixture()) }()
	<-entered
	started, retired := make(chan struct{}), make(chan error)
	go func() {
		close(started)
		c := smartloggerCommitContext()
		c.ConnectionGeneration++
		retired <- o.Retire(c)
	}()
	<-started
	close(release)
	if r := <-committed; r.Outcome != SmartloggerInventoryComplete {
		t.Fatal("commit did not win")
	}
	if err := <-retired; err != nil {
		t.Fatal(err)
	}
	s, ok := o.LastComplete()
	if !ok || s.Current || s.Plan.Context.ConnectionGeneration != 1 {
		t.Fatal("retirement relabeled committed history")
	}
	if r := a.Cancel(); r.Outcome != SmartloggerInventoryComplete {
		t.Fatal("retirement rewrote terminal success")
	}
}
