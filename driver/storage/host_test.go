package storage

import (
	"context"
	"testing"
	"time"

	huawei "github.com/Project-Helianthus/helianthus-modbusreg/huawei"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime/life"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/graph"
)

// openHostIn opens the host on a state directory the caller keeps, so that a
// second open continues what the first one retained.
func openHostIn(t *testing.T, dir string, inst *instance) *runtime.Runtime {
	t.Helper()
	ctx := context.Background()
	clock := &runtimeClock{now: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}
	cfg := life.InstanceConfig{Driver: storageKind, Instance: "storage.one", Enabled: true, Config: driver.Config{Instance: "storage.one", Enabled: true}}
	rt, err := runtime.OpenHost(ctx, life.Installation{Key: "installation.test", Bridge: "bridge.test", StateDir: dir, Clock: clock, Instances: []life.InstanceConfig{cfg}},
		runtime.HostOptions{}, hostFactory{inst})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := rt.WaitInitial(ctx); err != nil {
		t.Fatal(err)
	}
	return rt
}

func hostFacts(t *testing.T, rt *runtime.Runtime) int {
	t.Helper()
	ctx := context.Background()
	cut, err := rt.Cut(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cut.Close(); err != nil {
			t.Error(err)
		}
	}()
	result, err := cut.ListResources(ctx, graph.ListRequest{PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range result.Page.Resources {
		if r.Kind == kindPack {
			n++
		}
	}
	return n
}

// publishOnHost runs one publication through the host's own session and
// reports what the host answered.
func publishOnHost(t *testing.T, publish func(context.Context, *Publisher) error, cfg Config) (*runtime.Runtime, error) {
	t.Helper()
	inst := &instance{run: func(ctx context.Context, s driver.Session) error {
		p, err := New(cfg, &allocator{}, s)
		if err != nil {
			return err
		}
		return publish(ctx, p)
	}}
	rt := openHost(t, inst)
	return rt, inst.err
}

// TestHostAcceptsEveryPackOfADongleReading: the host accepts an observed fact
// only when its binding names the fact's subject, so each pack of a reading
// has its own binding and link.
func TestHostAcceptsEveryPackOfADongleReading(t *testing.T) {
	obs := HuaweiObservation{Children: []HuaweiChild{childOf(t, "LB-C00", 1, withWords(nil))}, Receipt: receipt(1)}
	rt, err := publishOnHost(t, func(ctx context.Context, p *Publisher) error { return p.PublishHuawei(ctx, obs) }, huaweiConfig())
	if err != nil {
		t.Fatalf("the host refused a reading with three packs: %v", err)
	}
	if got := hostFacts(t, rt); got != 3 {
		t.Fatalf("the host keeps %d packs, want the unit and its two packs", got)
	}
}

func TestHostAcceptsTheSiteBatteryBesideAChildUnit(t *testing.T) {
	obs := HuaweiObservation{Children: []HuaweiChild{childOf(t, "LB-C00", 1, withWords(nil))},
		Site: []huawei.EMMASiteGroup{energyGroup(10, 987654321), powerGroup(-800)}, Receipt: receipt(1)}
	rt, err := publishOnHost(t, func(ctx context.Context, p *Publisher) error { return p.PublishHuawei(ctx, obs) }, emmaConfig())
	if err != nil {
		t.Fatalf("the host refused the site battery beside a child unit: %v", err)
	}
	if got := hostFacts(t, rt); got != 4 {
		t.Fatalf("the host keeps %d packs, want the site battery, the unit and its two packs", got)
	}
}

func twoSegmentChain(t *testing.T) (sunspec.ChainSnapshot, Config) {
	t.Helper()
	r := sunRegistry(t, sunspec.ModelsRevisionV1)
	common := commonBlock(t, r, sunspec.ModelsRevisionV1)
	snap := chain(t, sunspec.ModelsRevisionV1, r.DecoderKeys(), common, controlsBlock(t, r, 500, 400), common, controlsBlock(t, r, 300, 200))
	cfg := sunConfig()
	cfg.SunSpec = []SunSpecStorage{{Position: 1, Pack: "pack-1"}, {Position: 3, Pack: "pack-2"}}
	return snap, cfg
}

func TestHostAcceptsATwoSegmentSunSpecChain(t *testing.T) {
	snap, cfg := twoSegmentChain(t)
	obs := SunSpecObservation{Decisions: decisions(t, snap), Receipt: receipt(1)}
	rt, err := publishOnHost(t, func(ctx context.Context, p *Publisher) error { return p.PublishSunSpec(ctx, obs) }, cfg)
	if err != nil {
		t.Fatalf("the host refused a chain with two segments: %v", err)
	}
	if got := hostFacts(t, rt); got != 2 {
		t.Fatalf("the host keeps %d packs, want one per segment", got)
	}
}

// restart publishes through two host lifetimes on one state directory, the
// second with a new producer, as a restarted driver does.
func restart(t *testing.T, cfg func(first bool) Config, publish func(context.Context, *Publisher, int) error) {
	t.Helper()
	dir := t.TempDir()
	for n := 1; n <= 2; n++ {
		inst := &instance{run: func(ctx context.Context, s driver.Session) error {
			p, err := New(cfg(n == 1), &allocator{}, s)
			if err != nil {
				return err
			}
			return publish(ctx, p, n)
		}}
		rt := openHostIn(t, dir, inst)
		if inst.err != nil {
			t.Fatalf("start %d: the host refused the producer's batch: %v", n, inst.err)
		}
		if err := rt.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSuccessorOfASunSpecProducerIsAcceptedByTheHost(t *testing.T) {
	snap := basicChain(t,
		func(r sunspec.DecoderRegistry) []uint16 { return ratingsBlock(t, r) },
		func(r sunspec.DecoderRegistry) []uint16 { return controlsBlock(t, r, 500, 400) })
	restart(t, func(bool) Config { return sunConfig() }, func(ctx context.Context, p *Publisher, n int) error {
		return p.PublishSunSpec(ctx, SunSpecObservation{Decisions: decisions(t, snap), Receipt: receipt(n)})
	})
}

func TestSuccessorOfAGrowattProducerWithCellsIsAcceptedByTheHost(t *testing.T) {
	f := loadFixture(t)
	restart(t, func(first bool) Config { c := f.config(); c.First = first; return c }, func(ctx context.Context, p *Publisher, n int) error {
		return p.PublishGrowatt(ctx, f.observe(t, 2, n, true))
	})
}

func TestSuccessorOfADongleProducerIsAcceptedByTheHost(t *testing.T) {
	restart(t, func(bool) Config { return huaweiConfig() }, func(ctx context.Context, p *Publisher, n int) error {
		return p.PublishHuawei(ctx, HuaweiObservation{Children: []HuaweiChild{childOf(t, "LB-C00", 1, withWords(nil))}, Receipt: receipt(n)})
	})
}
