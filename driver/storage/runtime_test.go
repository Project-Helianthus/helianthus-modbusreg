package storage

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime/life"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/graph"
)

const storageKind driver.DriverKind = "modbus"

type runtimeClock struct {
	mu   sync.Mutex
	now  time.Time
	mono uint64
}

func (c *runtimeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }

func (c *runtimeClock) Sample() (uint64, error) { c.mu.Lock(); defer c.mu.Unlock(); return c.mono, nil }

func (c *runtimeClock) Anchor(sample uint64) ([]byte, error) { return []byte{byte(sample)}, nil }

func (c *runtimeClock) Elapsed(_ []byte, sample uint64) (uint64, error) { return sample, nil }

// instance publishes through the host's own session the way a started driver
// does, and records what the session answered.
type instance struct {
	run func(context.Context, driver.Session) error
	err error
}

func (i *instance) FenceWithdrawal() {}

func (i *instance) Stop(context.Context) driver.CloseReport {
	return driver.CloseReport{Confirmed: true}
}

func (i *instance) Start(ctx context.Context, s driver.Session) error {
	i.err = i.run(ctx, s)
	return nil
}

type hostFactory struct{ inst *instance }

func (hostFactory) Descriptor() driver.Descriptor { return driver.Descriptor{} }

func (hostFactory) Plan(c driver.Config) ([]driver.InstancePlan, error) {
	return []driver.InstancePlan{{Driver: storageKind, Instance: driver.InstanceKey(c.Instance)}}, nil
}

func (f hostFactory) Construct(context.Context, driver.InstancePlan) (driver.Instance, error) {
	return f.inst, nil
}

func openHost(t *testing.T, inst *instance) *runtime.Runtime {
	t.Helper()
	ctx := context.Background()
	clock := &runtimeClock{now: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}
	cfg := life.InstanceConfig{Driver: storageKind, Instance: "storage.one", Enabled: true, Config: driver.Config{Instance: "storage.one", Enabled: true}}
	rt, err := runtime.OpenHost(ctx, life.Installation{Key: "installation.test", Bridge: "bridge.test", StateDir: t.TempDir(), Clock: clock, Instances: []life.InstanceConfig{cfg}},
		runtime.HostOptions{}, hostFactory{inst})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if err := rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := rt.WaitInitial(ctx); err != nil {
		t.Fatal(err)
	}
	return rt
}

func hostResources(t *testing.T, rt *runtime.Runtime) int {
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
		switch r.Kind {
		case kindSystem, kindBattery, kindPack, kindInterface, kindCell:
			n++
		}
	}
	return n
}

func hostClaims(t *testing.T, rt *runtime.Runtime) int {
	t.Helper()
	cut, err := rt.Cut(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cut.Close(); err != nil {
			t.Error(err)
		}
	}()
	n := 0
	for _, l := range cut.Links {
		if l.Key.Type.Pack.ID == packID {
			n += len(l.Claims)
		}
	}
	return n
}

// TestSessionTransactionCommitsAndRejectsRelationsAndClaimsTogether drives the
// producer through the host's own driver.Session: an accepted commit keeps the
// resources and the claims, a rejected one keeps neither.
func TestSessionTransactionCommitsAndRejectsRelationsAndClaimsTogether(t *testing.T) {
	f := loadFixture(t)
	observe := func(ctx context.Context, s driver.Session, poison bool) error {
		p, err := New(f.config(), &allocator{}, s)
		if err != nil {
			return err
		}
		if !poison {
			return p.PublishGrowatt(ctx, f.observe(t, 2, 1, false))
		}
		// The producer believes it published a direction the host never held; a standby
		// withdraws it, and the host refuses the withdrawal of an unpublished candidate.
		return p.publish(ctx, func(st stamp) (plan, error) {
			pl, err := p.growattPlan(f.observe(t, 1, 1, false))
			if err != nil {
				return plan{}, err
			}
			all, err := p.allocate(st, pl)
			if err != nil {
				return plan{}, err
			}
			p.published[p.candidateID(st, pl, p.bindingID(st, pl), all[pl.identity], elementDirection)] = elementDirection
			return pl, nil
		})
	}
	accepted := &instance{run: func(ctx context.Context, s driver.Session) error { return observe(ctx, s, false) }}
	rt := openHost(t, accepted)
	if accepted.err != nil {
		t.Fatalf("the host refused the producer's first batch: %v", accepted.err)
	}
	if hostResources(t, rt) != 3 || hostClaims(t, rt) != 2 {
		t.Fatalf("accepted commit kept %d resources and %d claims, want 3 and 2", hostResources(t, rt), hostClaims(t, rt))
	}

	// A withdrawal of a candidate the host never published makes the host refuse the whole batch.
	rejected := &instance{run: func(ctx context.Context, s driver.Session) error {
		return observe(ctx, s, true)
	}}
	rt = openHost(t, rejected)
	if rejected.err == nil {
		t.Fatal("the host accepted a batch that withdrew an unpublished candidate")
	}
	if hostResources(t, rt) != 0 || hostClaims(t, rt) != 0 {
		t.Fatalf("a rejected transaction kept %d resources and %d claims", hostResources(t, rt), hostClaims(t, rt))
	}
}
