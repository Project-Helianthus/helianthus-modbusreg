package modbusreg

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func i17Context() SmartloggerInventoryContext {
	return SmartloggerInventoryContext{OwnerToken: "i17-owner", ConnectionGeneration: 7,
		Identity: SmartloggerInventoryIdentity{RoutingContext: "i17-route", UnitID: 0,
			ParentProfile: "i17-parent", SourceRevision: "i17-source", SelfModel: "SmartLogger", Firmware: "V300R024C10SPC191"}}
}

func i17Limits() SmartloggerOfflineInventoryLimits {
	return SmartloggerOfflineInventoryLimits{MaxPages: 2, MaxObjects: 3, MaxBytes: 256}
}

func i17Input() HuaweiSmartLoggerOfflineInventoryInput {
	return HuaweiSmartLoggerOfflineInventoryInput{ChangeCounterBefore: 24, ChangeCounterAfter: 24,
		Pages: []HuaweiSmartLoggerInventoryPage{
			{More: true, NextObjectID: 0x89, Objects: []HuaweiSmartLoggerInventoryObject{
				{ObjectID: 0x87, Value: []byte{1}},
				{ObjectID: 0x88, Value: []byte("1=SmartLogger;2=V300R024C10SPC191")},
			}},
			{Objects: []HuaweiSmartLoggerInventoryObject{{ObjectID: 0x89, Value: []byte("1=i17-child-family;5=2;8=i17-type")}}},
		}}
}

func i17Port(i17T *testing.T, i17Reader SmartloggerOfflineInventoryReader) (I17SmartloggerProvider, *time.Time) {
	i17T.Helper()
	i17Now := time.Time{}.Add(time.Hour)
	i17P, i17Err := I17NewSmartloggerProvider(i17Context(), i17Limits(), func() time.Time { return i17Now }, i17Reader)
	if i17Err != nil {
		i17T.Fatal(i17Err)
	}
	return i17P, &i17Now
}

func TestI17FrozenInputsAndDetachedViews(i17T *testing.T) {
	i17Identity, i17Bounds := i17Context(), i17Limits()
	i17Now := time.Time{}.Add(time.Hour)
	i17Clock := func() time.Time { return i17Now }
	i17Data, i17Expected := i17Input(), i17Input()
	i17Calls := 0
	i17P, i17Err := I17NewSmartloggerProvider(i17Identity, i17Bounds, i17Clock, func(i17Plan SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
		i17Calls++
		i17Want := SmartloggerInventoryPlan{Context: i17Context(), AttemptToken: 1, Deadline: time.Time{}.Add(time.Hour + time.Second)}
		if i17Plan != i17Want {
			i17T.Fatalf("plan = %+v, want %+v", i17Plan, i17Want)
		}
		i17Plan.Context.OwnerToken = "i17-local-copy"
		return i17Data, nil
	})
	if i17Err != nil {
		i17T.Fatal(i17Err)
	}
	i17Identity = SmartloggerInventoryContext{}
	i17Bounds = SmartloggerOfflineInventoryLimits{}
	i17Clock = func() time.Time { return time.Time{} }
	if i17Identity == i17Context() || i17Bounds.valid() || !i17Clock().IsZero() {
		i17T.Fatal("caller inputs must be invalidated before checking constructor isolation")
	}
	if _, i17OK := i17P.I17NativeView(); i17OK {
		i17T.Fatal("unobserved inventory must remain unknown")
	}
	i17R, i17Err := i17P.I17Refresh(context.Background(), i17Now.Add(time.Second))
	if i17Err != nil || i17R.Outcome != SmartloggerInventoryComplete || !i17R.DefaultDenied() || i17Calls != 1 {
		i17T.Fatalf("refresh %+v, %v, calls %d", i17R, i17Err, i17Calls)
	}
	i17Data.Pages[0].Objects[1].Value[0] = 'X'
	i17Data.Pages[1].Objects = nil
	i17Data.Pages[0] = HuaweiSmartLoggerInventoryPage{}
	i17S, i17OK := i17P.I17NativeView()
	if !i17OK || !i17S.Current || !i17S.Inventory.DefaultDenied() || i17S.Inventory.ChildCount() != 1 ||
		!reflect.DeepEqual(i17S.Native, i17Expected) || i17S.Inventory.Children()[0].Address() != "2" ||
		!i17S.ObservedAt.Equal(i17Now) || !i17S.CompletedAt.Equal(i17Now) || i17S.Plan != i17R.Plan {
		i17T.Fatal("native evidence, markers, identity or denied admission lost")
	}
	i17S.Native.Pages[0].Objects[1].Value[0] = 'Y'
	i17S.Native.Pages[1].Objects = nil
	i17S.Native.Pages[0] = HuaweiSmartLoggerInventoryPage{}
	i17S.Inventory.children[0] = HuaweiSmartLoggerInventoryChild{}
	i17S.Plan.Context.OwnerToken = "i17-output-copy"
	i17Again, _ := i17P.I17NativeView()
	if !reflect.DeepEqual(i17Again.Native, i17Expected) || i17Again.Inventory.Children()[0].Address() != "2" || i17Again.Plan != i17R.Plan {
		i17T.Fatal("view aliases owner history")
	}
}

func TestI17ConstructorIdentityAxes(i17T *testing.T) {
	i17Axes := map[string]func(*SmartloggerInventoryContext){
		"OwnerToken":           func(i17C *SmartloggerInventoryContext) { i17C.OwnerToken = "" },
		"ConnectionGeneration": func(i17C *SmartloggerInventoryContext) { i17C.ConnectionGeneration = 0 },
		"RoutingContext":       func(i17C *SmartloggerInventoryContext) { i17C.Identity.RoutingContext = "" },
		"UnitID":               func(i17C *SmartloggerInventoryContext) { i17C.Identity.UnitID = 1 },
		"ParentProfile":        func(i17C *SmartloggerInventoryContext) { i17C.Identity.ParentProfile = "" },
		"SourceRevision":       func(i17C *SmartloggerInventoryContext) { i17C.Identity.SourceRevision = "" },
		"SelfModel":            func(i17C *SmartloggerInventoryContext) { i17C.Identity.SelfModel = "i17-other-family" },
		"Firmware":             func(i17C *SmartloggerInventoryContext) { i17C.Identity.Firmware = "i17-unknown" },
	}
	for i17Name, i17Mutate := range i17Axes {
		i17T.Run(i17Name, func(i17T *testing.T) {
			i17C, i17Calls := i17Context(), 0
			i17Mutate(&i17C)
			i17P, i17Err := I17NewSmartloggerProvider(i17C, i17Limits(), nil, func(SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
				i17Calls++
				return i17Input(), nil
			})
			if i17P != nil || !errors.Is(i17Err, ErrSmartloggerInventoryContext) || i17Calls != 0 {
				i17T.Fatalf("invalid %s accepted: %v calls %d", i17Name, i17Err, i17Calls)
			}
		})
	}
}

func TestI17DistinctValidContextAxes(i17T *testing.T) {
	i17Axes := map[string]func(*SmartloggerInventoryContext){
		"OwnerToken":           func(i17C *SmartloggerInventoryContext) { i17C.OwnerToken = "i17-second-owner" },
		"ConnectionGeneration": func(i17C *SmartloggerInventoryContext) { i17C.ConnectionGeneration = 9 },
		"RoutingContext":       func(i17C *SmartloggerInventoryContext) { i17C.Identity.RoutingContext = "i17-second-route" },
		"ParentProfile":        func(i17C *SmartloggerInventoryContext) { i17C.Identity.ParentProfile = "i17-second-parent" },
		"SourceRevision":       func(i17C *SmartloggerInventoryContext) { i17C.Identity.SourceRevision = "i17-second-source" },
		"Firmware":             func(i17C *SmartloggerInventoryContext) { i17C.Identity.Firmware = "V300R024C10SPC210" },
	}
	for i17Name, i17Mutate := range i17Axes {
		i17T.Run(i17Name, func(i17T *testing.T) {
			i17Expected := i17Context()
			i17Mutate(&i17Expected)
			i17Supplied, i17Now := i17Expected, time.Time{}.Add(time.Hour)
			i17Calls := 0
			i17P, i17Err := I17NewSmartloggerProvider(i17Supplied, i17Limits(), func() time.Time { return i17Now }, func(i17Plan SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
				i17Calls++
				if i17Plan.Context != i17Expected {
					i17T.Fatalf("context collapsed: %+v, want %+v", i17Plan.Context, i17Expected)
				}
				i17Data := i17Input()
				if i17Name == "Firmware" {
					i17Data.Pages[0].Objects[1].Value = []byte("1=SmartLogger;2=V300R024C10SPC210")
				}
				return i17Data, nil
			})
			if i17Err != nil {
				i17T.Fatal(i17Err)
			}
			i17Supplied = SmartloggerInventoryContext{}
			if i17Supplied == i17Expected {
				i17T.Fatal("caller context must differ before checking constructor isolation")
			}
			i17R, i17Err := i17P.I17Refresh(context.Background(), i17Now.Add(time.Second))
			i17S, i17OK := i17P.I17NativeView()
			if i17Err != nil || i17Calls != 1 || !i17OK || i17R.Plan.Context != i17Expected || i17S.Plan.Context != i17Expected || !i17R.DefaultDenied() || !i17S.Inventory.DefaultDenied() {
				i17T.Fatal("distinct context lost or admitted", i17R, i17Err)
			}
		})
	}
}

func TestI17ConstructorBoundsAndRefusals(i17T *testing.T) {
	for _, i17Name := range []string{"pages-zero", "pages-high", "objects-zero", "objects-high", "bytes-zero", "bytes-high", "reader"} {
		i17T.Run(i17Name, func(i17T *testing.T) {
			i17Bounds, i17Calls := i17Limits(), 0
			var i17Reader SmartloggerOfflineInventoryReader = func(SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
				i17Calls++
				return i17Input(), nil
			}
			switch i17Name {
			case "pages-zero":
				i17Bounds.MaxPages = 0
			case "pages-high":
				i17Bounds.MaxPages = 249
			case "objects-zero":
				i17Bounds.MaxObjects = 0
			case "objects-high":
				i17Bounds.MaxObjects = 250
			case "bytes-zero":
				i17Bounds.MaxBytes = 0
			case "bytes-high":
				i17Bounds.MaxBytes = 65537
			case "reader":
				i17Reader = nil
			}
			i17P, i17Err := I17NewSmartloggerProvider(i17Context(), i17Bounds, nil, i17Reader)
			if i17P != nil || i17Err == nil || i17Calls != 0 {
				i17T.Fatal("invalid constructor called supplier")
			}
		})
	}
	i17Calls := 0
	i17P, i17Now := i17Port(i17T, func(SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
		i17Calls++
		return i17Input(), nil
	})
	i17Cancelled, i17Cancel := context.WithCancel(context.Background())
	i17Cancel()
	i17Expired, i17Stop := context.WithDeadline(context.Background(), time.Time{})
	defer i17Stop()
	for _, i17Ctx := range []context.Context{nil, i17Cancelled, i17Expired} {
		if i17R, i17Err := i17P.I17Refresh(i17Ctx, i17Now.Add(time.Second)); i17Err == nil || i17R != (SmartloggerInventoryResult{}) {
			i17T.Fatal("invalid context accepted")
		}
	}
	for _, i17Deadline := range []time.Time{time.Time{}, *i17Now, i17Now.Add(16 * time.Second)} {
		if _, i17Err := i17P.I17Refresh(context.Background(), i17Deadline); i17Err == nil {
			i17T.Fatal("invalid deadline accepted")
		}
	}
	if i17Calls != 0 {
		i17T.Fatal("refusal invoked supplier", i17Calls)
	}
	var i17Zero I17SmartloggerProviderAdapter
	var i17Nil *I17SmartloggerProviderAdapter
	for _, i17Invalid := range []*I17SmartloggerProviderAdapter{&i17Zero, i17Nil} {
		if _, i17Err := i17Invalid.I17Refresh(context.Background(), i17Now.Add(time.Second)); i17Err == nil {
			i17T.Fatal("zero/nil refresh accepted")
		}
		if _, i17OK := i17Invalid.I17NativeView(); i17OK {
			i17T.Fatal("zero/nil invented history")
		}
		if i17Err := i17Invalid.I17Retire(i17Context()); i17Err == nil {
			i17T.Fatal("zero/nil retirement accepted")
		}
		i17Invalid.I17Close()
	}
}

func TestI17FailedRefreshRetainsHistory(i17T *testing.T) {
	for _, i17Name := range []string{"incomplete", "reader-error", "cancelled", "expired", "counter", "wrong-next", "firmware", "pages", "objects", "bytes"} {
		i17T.Run(i17Name, func(i17T *testing.T) {
			i17Ctx, i17Cancel := context.WithCancel(context.Background())
			defer i17Cancel()
			i17Calls := 0
			i17Sentinel := errors.New("i17 synthetic failure")
			var i17Now *time.Time
			i17P, i17Clock := i17Port(i17T, func(i17Plan SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
				i17Calls++
				i17Data := i17Input()
				if i17Calls == 1 {
					return i17Data, nil
				}
				switch i17Name {
				case "incomplete":
					i17Data.Pages = i17Data.Pages[:1]
				case "reader-error":
					return i17Data, i17Sentinel
				case "cancelled":
					i17Cancel()
				case "expired":
					*i17Now = i17Plan.Deadline
				case "counter":
					i17Data.ChangeCounterAfter++
				case "wrong-next":
					i17Data.Pages[0].NextObjectID++
				case "firmware":
					i17Data.Pages[0].Objects[1].Value = []byte("1=SmartLogger;2=V300R024C10SPC210")
				case "pages":
					i17Data.Pages = append(i17Data.Pages, HuaweiSmartLoggerInventoryPage{})
				case "objects":
					i17Data.Pages[1].Objects = append(i17Data.Pages[1].Objects, HuaweiSmartLoggerInventoryObject{})
				case "bytes":
					i17Data.Pages[1].Objects[0].Value = make([]byte, 257)
				}
				return i17Data, nil
			})
			i17Now = i17Clock
			if _, i17Err := i17P.I17Refresh(i17Ctx, i17Now.Add(time.Second)); i17Err != nil {
				i17T.Fatal(i17Err)
			}
			i17Before, _ := i17P.I17NativeView()
			*i17Now = i17Now.Add(2 * time.Second)
			i17R, i17Err := i17P.I17Refresh(i17Ctx, i17Now.Add(time.Second))
			i17Want := SmartloggerInventoryFailed
			if i17Name == "cancelled" {
				i17Want = SmartloggerInventoryCancelled
			}
			if i17Err == nil || i17R.Outcome != i17Want || !i17R.DefaultDenied() || i17Calls != 2 {
				i17T.Fatalf("result %+v error %v calls %d", i17R, i17Err, i17Calls)
			}
			if i17Name == "reader-error" && !errors.Is(i17Err, i17Sentinel) {
				i17T.Fatal("reader error lost")
			}
			i17After, _ := i17P.I17NativeView()
			if !reflect.DeepEqual(i17After, i17Before) || !i17After.ObservedAt.Equal(time.Time{}.Add(time.Hour)) {
				i17T.Fatal("failure replaced or redated history")
			}
		})
	}
}

func TestI17FrozenNarrowBounds(i17T *testing.T) {
	for _, i17Axis := range []string{"MaxPages", "MaxObjects", "MaxBytes"} {
		i17T.Run(i17Axis, func(i17T *testing.T) {
			i17Bounds := i17Limits()
			switch i17Axis {
			case "MaxPages":
				i17Bounds.MaxPages = 1
			case "MaxObjects":
				i17Bounds.MaxObjects = 2
			case "MaxBytes":
				i17Bounds.MaxBytes = 40
			}
			i17Calls, i17Now := 0, time.Time{}.Add(time.Hour)
			i17P, i17Err := I17NewSmartloggerProvider(i17Context(), i17Bounds, func() time.Time { return i17Now }, func(SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
				i17Calls++
				return i17Input(), nil
			})
			if i17Err != nil {
				i17T.Fatal(i17Err)
			}
			i17Bounds = i17Limits()
			i17R, i17Err := i17P.I17Refresh(context.Background(), i17Now.Add(time.Second))
			if !errors.Is(i17Err, ErrSmartloggerOfflineProviderInput) || i17R.Outcome != SmartloggerInventoryFailed || i17Calls != 1 || !i17R.DefaultDenied() {
				i17T.Fatal("frozen narrow bound ignored", i17R, i17Err, i17Calls)
			}
			if _, i17OK := i17P.I17NativeView(); i17OK {
				i17T.Fatal("out-of-bounds evidence published")
			}
		})
	}
}

func i17Wait(i17T *testing.T, i17Signal <-chan struct{}) {
	i17T.Helper()
	select {
	case <-i17Signal:
	case <-time.After(3 * time.Second):
		i17T.Fatal("i17 watchdog: callback did not enter")
	}
}

func TestI17LifecycleFences(i17T *testing.T) {
	for _, i17Event := range []string{"close", "retire", "cancel"} {
		i17T.Run(i17Event, func(i17T *testing.T) {
			i17Entered, i17Release := make(chan struct{}), make(chan struct{}, 1)
			defer close(i17Release)
			var i17Calls atomic.Int32
			i17P, i17Now := i17Port(i17T, func(SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
				if i17Calls.Add(1) == 2 {
					close(i17Entered)
					<-i17Release
				}
				return i17Input(), nil
			})
			if _, i17Err := i17P.I17Refresh(context.Background(), i17Now.Add(time.Second)); i17Err != nil {
				i17T.Fatal(i17Err)
			}
			i17Before, _ := i17P.I17NativeView()
			i17Ctx, i17Cancel := context.WithCancel(context.Background())
			defer i17Cancel()
			i17Done := make(chan SmartloggerInventoryResult, 1)
			go func() { i17R, _ := i17P.I17Refresh(i17Ctx, i17Now.Add(time.Second)); i17Done <- i17R }()
			i17Wait(i17T, i17Entered)
			if _, i17Err := i17P.I17Refresh(context.Background(), i17Now.Add(time.Second)); !errors.Is(i17Err, ErrSmartloggerInventoryBusy) || i17Calls.Load() != 2 {
				i17T.Fatal("concurrent refresh not refused", i17Err)
			}
			i17Want := SmartloggerInventoryCancelled
			switch i17Event {
			case "close":
				i17P.I17Close()
				i17Want = SmartloggerInventoryClosed
				i17Before.Current = false
			case "retire":
				i17Next := i17Context()
				i17Next.ConnectionGeneration++
				if i17Err := i17P.I17Retire(i17Next); i17Err != nil {
					i17T.Fatal(i17Err)
				}
				i17Want = SmartloggerInventoryRetired
				i17Before.Current = false
			case "cancel":
				i17Cancel()
			}
			if i17Event != "cancel" {
				if _, i17Err := i17P.I17Refresh(context.Background(), i17Now.Add(time.Second)); i17Err == nil || i17Calls.Load() != 2 {
					i17T.Fatal("terminal port accepted work")
				}
			}
			i17Release <- struct{}{}
			select {
			case i17R := <-i17Done:
				if i17R.Outcome != i17Want || !i17R.DefaultDenied() {
					i17T.Fatalf("late result %+v", i17R)
				}
			case <-time.After(3 * time.Second):
				i17T.Fatal("i17 watchdog: refresh did not finish")
			}
			i17After, _ := i17P.I17NativeView()
			if !reflect.DeepEqual(i17Before, i17After) {
				i17T.Fatal("late publication changed history")
			}
			if i17Event == "cancel" {
				if _, i17Err := i17P.I17Refresh(context.Background(), i17Now.Add(time.Second)); i17Err != nil || i17Calls.Load() != 3 {
					i17T.Fatal("cancel did not release slot", i17Err)
				}
			} else {
				i17P.I17Close()
				if i17Err := i17P.I17Retire(i17Context()); i17Err == nil {
					i17T.Fatal("terminal retirement accepted")
				}
				if _, i17Err := i17P.I17Refresh(context.Background(), i17Now.Add(time.Second)); i17Err == nil || i17Calls.Load() != 2 {
					i17T.Fatal("terminal port reopened")
				}
			}
		})
	}
}

func TestI17RetirementContextValidation(i17T *testing.T) {
	i17Calls := 0
	i17P, i17Now := i17Port(i17T, func(SmartloggerInventoryPlan) (HuaweiSmartLoggerOfflineInventoryInput, error) {
		i17Calls++
		return i17Input(), nil
	})
	for _, i17C := range []SmartloggerInventoryContext{i17Context(), {}} {
		if i17Err := i17P.I17Retire(i17C); !errors.Is(i17Err, ErrSmartloggerInventoryContext) || i17Calls != 0 {
			i17T.Fatal("invalid retirement accepted", i17Err)
		}
	}
	if _, i17Err := i17P.I17Refresh(context.Background(), i17Now.Add(time.Second)); i17Err != nil || i17Calls != 1 {
		i17T.Fatal("invalid retirement closed port", i17Err)
	}
	i17Next := i17Context()
	i17Next.ConnectionGeneration++
	if i17Err := i17P.I17Retire(i17Next); i17Err != nil {
		i17T.Fatal(i17Err)
	}
	if _, i17Err := i17P.I17Refresh(context.Background(), i17Now.Add(time.Second)); i17Err == nil || i17Calls != 1 {
		i17T.Fatal("retired idle port invoked supplier")
	}
}
