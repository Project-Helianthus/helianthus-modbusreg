package modbusreg

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func offlineInput() GrowattBMSReadOnlyInput {
	return GrowattBMSReadOnlyInput{UnitID: 1, Function: FunctionReadHoldingRegisters,
		Revision: GrowattBMSRevisionTuple{Family: "1xSxxP ESS", FileRevision: "Rev2.01", HeaderVersion: "V2.0", CumulativeRevision: "2.02"},
		Slices: []GrowattBMSReadOnlySlice{
			{Offset: 1, Words: []uint16{0x0102, 0, 0, 0, 0, 0, 0}},
			{Offset: 13, Words: []uint16{0x0204, 0x0301, 0, 0, 0, 0, 2, 0, 75, 5200, 100, 25, 0, 3200, 5000, 0, 0, 0, 0x0300, 0, 0, 0, 0, 0, 0, 0, 0, 0, 32}},
			{Offset: 256, Words: make([]uint16, 12)},
			{Offset: 269, Words: make([]uint16, 2)},
		}}
}

func offlineRequest() GrowattAggregateOfflineCycle {
	return GrowattAggregateOfflineCycle{Context: GrowattAggregateContext{
		SourceInstance: "synthetic-source", OwnerEpoch: 1, UnitID: 1, Function: FunctionReadHoldingRegisters,
		Revision: GrowattBMSRevisionTuple{Family: "1xSxxP ESS", FileRevision: "Rev2.01", HeaderVersion: "V2.0", CumulativeRevision: "2.02"}, IdentityKnown: true,
		Identity: GrowattAggregateIdentity{MCUSoftwareVersion: "1.2", BMSCompany: 4, BMSGeneration: 2,
			PackCompany: 1, PackGeneration: 3, Topology: 0x0300, CellSeriesCount: 32}}, Cycle: 1, Deadline: 10}
}

func offlineProvider(t *testing.T, reader GrowattAggregateOfflineReader) *GrowattAggregateOfflineProvider {
	t.Helper()
	p, err := NewGrowattAggregateOfflineProvider(10, 4, 50, reader)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestGrowattAggregateOfflineProviderTransitions(t *testing.T) {
	in := offlineInput()
	var readErr error
	calls := 0
	p := offlineProvider(t, func(_ context.Context, got GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
		calls++
		if got.Context != offlineRequest().Context {
			t.Fatal("reader context changed")
		}
		return in, readErr
	})
	r := offlineRequest()
	tick := uint64(1)
	run := func() error { return p.Cycle(context.Background(), r, func() uint64 { return tick }) }
	if err := run(); err != nil {
		t.Fatal(err)
	}
	s := p.Snapshot()
	if s.State != "current" || s.SuccessTick != 1 || s.SuccessRevision != 1 || s.Value.SOCPercent != 75 || s.Value.OutboundAllowed() {
		t.Fatalf("success: %+v", s)
	}
	// Mutating both reader-owned and snapshot-owned nested slices cannot alter storage.
	in.Slices[1].Words[8] = 42
	raw := s.Value.NativeObservation().Slices()
	raw[1].Words[8] = 13
	s.Value.nativeObservation.slices[1].Words[8] = 14
	if p.Snapshot().Value.SOCPercent != 75 || p.Snapshot().Value.NativeObservation().Slices()[1].Words[8] != 75 {
		t.Fatal("aliased evidence")
	}
	for _, mode := range []string{"partial", "error", "zero", "replacement"} {
		r.Cycle++
		tick++
		r.Deadline = tick + 5
		in = offlineInput()
		readErr = nil
		switch mode {
		case "partial":
			in.Slices = in.Slices[:3]
		case "error":
			readErr = errors.New("synthetic failure")
		case "zero":
			in.Slices[1].Words[8] = 0
		case "replacement":
			in.Slices[1].Words[8] = 33
		}
		err := run()
		got := p.Snapshot()
		if mode == "partial" || mode == "error" {
			if err == nil || got.State != "retained" || got.Attempt.Outcome != "failed" || got.SuccessTick != 1 || got.SuccessRevision != 1 || got.Value.SOCPercent != 75 {
				t.Fatalf("%s: %+v %v", mode, got, err)
			}
		} else {
			want := uint8(0)
			if mode == "replacement" {
				want = 33
			}
			if err != nil || got.Value.SOCPercent != want || got.SuccessTick != tick || got.SuccessRevision != r.Cycle-2 {
				t.Fatalf("%s: %+v %v", mode, got, err)
			}
		}
	}
	if calls != 5 {
		t.Fatalf("reader called %d times", calls)
	}
}

func offlineContextAxes() map[string]func(*GrowattAggregateContext) {
	return map[string]func(*GrowattAggregateContext){
		"SourceInstance":              func(c *GrowattAggregateContext) { c.SourceInstance += "-other" },
		"OwnerEpoch":                  func(c *GrowattAggregateContext) { c.OwnerEpoch++ },
		"UnitID":                      func(c *GrowattAggregateContext) { c.UnitID++ },
		"Function":                    func(c *GrowattAggregateContext) { c.Function = FunctionReadInputRegisters },
		"Revision.Family":             func(c *GrowattAggregateContext) { c.Revision.Family += "-other" },
		"Revision.FileRevision":       func(c *GrowattAggregateContext) { c.Revision.FileRevision += "-other" },
		"Revision.HeaderVersion":      func(c *GrowattAggregateContext) { c.Revision.HeaderVersion += "-other" },
		"Revision.CumulativeRevision": func(c *GrowattAggregateContext) { c.Revision.CumulativeRevision += "-other" },
		"IdentityKnown":               func(c *GrowattAggregateContext) { c.IdentityKnown = false },
		"MCUSoftwareVersion":          func(c *GrowattAggregateContext) { c.Identity.MCUSoftwareVersion = "9.9" },
		"BMSCompany":                  func(c *GrowattAggregateContext) { c.Identity.BMSCompany++ },
		"BMSGeneration":               func(c *GrowattAggregateContext) { c.Identity.BMSGeneration++ },
		"PackCompany":                 func(c *GrowattAggregateContext) { c.Identity.PackCompany++ },
		"PackGeneration":              func(c *GrowattAggregateContext) { c.Identity.PackGeneration++ },
		"Topology":                    func(c *GrowattAggregateContext) { c.Identity.Topology++ },
		"CellSeriesCount":             func(c *GrowattAggregateContext) { c.Identity.CellSeriesCount++ },
	}
}

func TestGrowattAggregateOfflineProviderContinuity(t *testing.T) {
	for name, mutate := range offlineContextAxes() {
		t.Run(name, func(t *testing.T) {
			p := offlineProvider(t, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
				return offlineInput(), nil
			})
			r := offlineRequest()
			if err := p.Cycle(context.Background(), r, func() uint64 { return 1 }); err != nil {
				t.Fatal(err)
			}
			mutate(&r.Context)
			r.Cycle++
			// Fail after Begin so only the changed continuity axis can invalidate the prior success.
			p.reader = func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
				return GrowattBMSReadOnlyInput{}, errors.New("synthetic failure")
			}
			_ = p.Cycle(context.Background(), r, func() uint64 { return 2 })
			s := p.Snapshot()
			if s.State != "invalidated" || s.Value != nil || s.SuccessTick != 1 || s.SuccessRevision != 1 {
				t.Fatalf("%s: %+v", name, s)
			}
		})
	}
}

func TestGrowattAggregateOfflineProviderCallbackContext(t *testing.T) {
	for name, mutate := range offlineContextAxes() {
		t.Run(name, func(t *testing.T) {
			var p *GrowattAggregateOfflineProvider
			p = offlineProvider(t, func(_ context.Context, request GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
				wrong := request.Context
				mutate(&wrong)
				if err := p.retention.Fail(wrong, request.Cycle, 1); err == nil || p.Snapshot().Attempt.Outcome != "pending" {
					t.Fatal("foreign callback closed active attempt")
				}
				return offlineInput(), nil
			})
			if err := p.Cycle(context.Background(), offlineRequest(), func() uint64 { return 1 }); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGrowattAggregateOfflineProviderNativeIdentity(t *testing.T) {
	for name, mutate := range map[string]func(*GrowattBMSReadOnlyInput){
		"UnitID":                      func(in *GrowattBMSReadOnlyInput) { in.UnitID++ },
		"Function":                    func(in *GrowattBMSReadOnlyInput) { in.Function = FunctionReadInputRegisters },
		"Revision.Family":             func(in *GrowattBMSReadOnlyInput) { in.Revision.Family += "other" },
		"Revision.FileRevision":       func(in *GrowattBMSReadOnlyInput) { in.Revision.FileRevision += "other" },
		"Revision.HeaderVersion":      func(in *GrowattBMSReadOnlyInput) { in.Revision.HeaderVersion += "other" },
		"Revision.CumulativeRevision": func(in *GrowattBMSReadOnlyInput) { in.Revision.CumulativeRevision += "other" },
		"MCUSoftwareVersion":          func(in *GrowattBMSReadOnlyInput) { in.Slices[0].Words[0]++ },
		"BMSCompany":                  func(in *GrowattBMSReadOnlyInput) { in.Slices[1].Words[0]++ },
		"BMSGeneration":               func(in *GrowattBMSReadOnlyInput) { in.Slices[1].Words[0] += 256 },
		"PackCompany":                 func(in *GrowattBMSReadOnlyInput) { in.Slices[1].Words[1]++ },
		"PackGeneration":              func(in *GrowattBMSReadOnlyInput) { in.Slices[1].Words[1] += 256 },
		"Topology":                    func(in *GrowattBMSReadOnlyInput) { in.Slices[1].Words[18]++ },
		"CellSeriesCount":             func(in *GrowattBMSReadOnlyInput) { in.Slices[1].Words[28]++ },
	} {
		t.Run(name, func(t *testing.T) {
			in := offlineInput()
			mutate(&in)
			p := offlineProvider(t, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) { return in, nil })
			if err := p.Cycle(context.Background(), offlineRequest(), func() uint64 { return 1 }); err == nil || p.Snapshot().Value != nil {
				t.Fatal("native conflict committed")
			}
		})
	}
}

func TestGrowattAggregateOfflineProviderCancelExpirySupersession(t *testing.T) {
	for _, mode := range []string{"cancel", "deadline", "advance", "cycle", "epoch"} {
		t.Run(mode, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			p := offlineProvider(t, func(_ context.Context, r GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
				if r.Cycle == 2 {
					close(entered)
					<-release
				}
				return offlineInput(), nil
			})
			r := offlineRequest()
			if err := p.Cycle(context.Background(), r, func() uint64 { return 1 }); err != nil {
				t.Fatal(err)
			}
			r.Cycle = 2
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tick := uint64(2)
			done := make(chan error, 1)
			go func() { done <- p.Cycle(ctx, r, func() uint64 { return tick }) }()
			<-entered
			switch mode {
			case "cancel":
				cancel()
			case "deadline":
				tick = 10
			case "advance":
				if err := p.Advance(10); err != nil {
					t.Fatal(err)
				}
				tick = 10
			case "cycle", "epoch":
				next := r
				next.Cycle = 3
				if mode == "epoch" {
					next.Context.OwnerEpoch++
					next.Cycle = 1
				}
				if err := p.Cycle(context.Background(), next, func() uint64 { return 3 }); err != nil {
					t.Fatal(err)
				}
				tick = 4
			}
			before := p.Snapshot()
			close(release)
			if err := <-done; err == nil {
				t.Fatal("late/cancelled cycle succeeded")
			}
			s := p.Snapshot()
			if s.SuccessTick != before.SuccessTick || s.SuccessRevision != before.SuccessRevision || !reflect.DeepEqual(s.Value, before.Value) {
				t.Fatalf("late completion replaced success: %+v", s)
			}
			if mode == "cancel" && s.Attempt.Outcome != "failed" {
				t.Fatal(s.Attempt)
			}
			if (mode == "deadline" || mode == "advance") && s.Attempt.Outcome != "incomplete" {
				t.Fatal(s.Attempt)
			}
		})
	}
}

func TestGrowattAggregateOfflineProviderBounds(t *testing.T) {
	reader := func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
		return offlineInput(), nil
	}
	for _, b := range [][3]uint64{{0, 4, 50}, {10, 0, 50}, {10, 4, 0}} {
		if _, err := NewGrowattAggregateOfflineProvider(b[0], b[1], b[2], reader); err == nil {
			t.Fatal("unbounded constructor")
		}
	}
	if _, err := NewGrowattAggregateOfflineProvider(10, 4, 50, nil); err == nil {
		t.Fatal("nil reader")
	}
	for _, mode := range []string{"nil-context", "nil-clock", "cancelled", "cycle", "deadline", "span", "source", "epoch", "FC04", "revision", "unit", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			p := offlineProvider(t, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
				calls++
				return offlineInput(), nil
			})
			r := offlineRequest()
			ctx := context.Background()
			clock := func() uint64 { return 1 }
			switch mode {
			case "nil-context":
				ctx = nil
			case "nil-clock":
				clock = nil
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "cycle":
				r.Cycle = 0
			case "deadline":
				r.Deadline = 1
			case "span":
				r.Deadline = 12
			case "source":
				r.Context.SourceInstance = ""
			case "epoch":
				r.Context.OwnerEpoch = 0
			case "FC04":
				r.Context.Function = FunctionReadInputRegisters
			case "revision":
				r.Context.Revision.Family = "other"
			case "unit":
				r.Context.UnitID = 0
			case "unknown":
				r.Context.IdentityKnown = false
			}
			if err := p.Cycle(ctx, r, clock); err == nil || calls != 0 {
				t.Fatalf("invalid request called reader: %d %v", calls, err)
			}
		})
	}
	for _, mode := range []string{"slices", "words", "words-total", "FC04", "diagnostic", "empty"} {
		t.Run(mode, func(t *testing.T) {
			in := offlineInput()
			switch mode {
			case "slices":
				in.Slices = append(in.Slices, GrowattBMSReadOnlySlice{})
			case "words":
				in.Slices[0].Words = make([]uint16, 51)
			case "words-total":
				in.Slices[0].Words = make([]uint16, 8)
			case "FC04":
				in.Function = FunctionReadInputRegisters
			case "diagnostic":
				in.Slices[0].Offset = 0x0070
			case "empty":
				in = GrowattBMSReadOnlyInput{}
			}
			p := offlineProvider(t, func(context.Context, GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) { return in, nil })
			err := p.Cycle(context.Background(), offlineRequest(), func() uint64 { return 1 })
			if err == nil || p.Snapshot().Value != nil {
				t.Fatalf("admitted %s", mode)
			}
			if mode == "slices" && !strings.Contains(err.Error(), "slice bound") {
				t.Fatal(err)
			}
			if strings.HasPrefix(mode, "words") && !strings.Contains(err.Error(), "word bound") {
				t.Fatal(err)
			}
		})
	}
}

func TestGrowattAggregateOfflineProviderRequest(t *testing.T) {
	r := offlineRequest()
	r.Cycle = 7
	r.Deadline = 8
	calls := 0
	p := offlineProvider(t, func(_ context.Context, got GrowattAggregateOfflineCycle) (GrowattBMSReadOnlyInput, error) {
		calls++
		if got != r {
			t.Fatalf("request lost context: %+v", got)
		}
		return offlineInput(), nil
	})
	if err := p.Cycle(context.Background(), r, func() uint64 { return 2 }); err != nil {
		t.Fatal(err)
	}
	if err := p.Cycle(context.Background(), r, func() uint64 { return 3 }); err == nil || calls != 1 {
		t.Fatalf("repeated cycle: %d %v", calls, err)
	}
	s := p.Snapshot()
	if s.Attempt.Cycle != 7 || s.Attempt.Deadline != 8 || s.SuccessTick != 2 {
		t.Fatal(s)
	}
}
