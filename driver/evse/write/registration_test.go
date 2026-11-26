package write

import (
	"context"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/driver/evse/reader"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

func TestTheExecutorExistsWhileTheWriterRunsAndIsGoneAfterStop(t *testing.T) {
	r := newRig(t)
	live := r.session.live()
	if len(live) != 1 || len(r.session.routes) != 1 || len(r.session.sources) != 1 {
		t.Fatalf("executors %d routes %d sources %d", len(live), len(r.session.routes), len(r.session.sources))
	}
	route := r.session.routes[0]
	want := r.dispatch().Route
	if route.Route != want || route.Operation != definition(OperationID) || live[0].route.Route != want {
		t.Fatalf("route %+v", route)
	}
	if route.Owner.PathKey != PathKey || r.session.sources[0].PathKey != PathKey || route.For.Key.FactID != ArgumentID {
		t.Fatalf("owner %+v source %+v", route.Owner, r.session.sources[0])
	}
	if text := route.For.Key.Dimensions[0]; text.ID != DimensionID || *text.Value.Text != r.pub.Connector {
		t.Fatalf("the route is not on the connector: %+v", text)
	}
	if route.Limits != nil || r.session.sources[0].Limits != nil || r.session.sources[0].Priority != 0 {
		t.Fatal("the route declares a limit or a priority of its own")
	}
	if report := r.writer.Stop(context.Background()); !report.Confirmed {
		t.Fatalf("%+v", report)
	}
	if len(r.session.live()) != 0 {
		t.Fatal("the executor is still registered after stop")
	}
	r.seed(idle(32), 990)
	if result := r.apply(amperes("16", 0)); result.Reason != ReasonStopped {
		t.Fatalf("%q", result.Reason)
	}
	r.wantLog()
}

// A build admitted for reads with no qualified t25 setter keeps its reads, has
// no route and no executor, and sends nothing.
func TestABuildWithoutAQualifiedSetterRegistersNoExecutor(t *testing.T) {
	r := newRig(t, func(c *reader.ChargerConfig) { c.ProvisionalSetQualified = false })
	if r.reader.CurrentLimitWritable() {
		t.Fatal("the build is writable")
	}
	if len(r.session.live()) != 0 || len(r.session.routes) != 0 || len(r.session.sources) != 0 {
		t.Fatalf("executors %d routes %d sources %d", len(r.session.live()), len(r.session.routes), len(r.session.sources))
	}
	waitFor(t, "the reads to be published", func() bool { r.session.mu.Lock(); defer r.session.mu.Unlock(); return len(r.session.batches) > 0 })
	result := r.apply(amperes("16", 0))
	if result.Level != LevelFailedNoContact || result.Reason != ReasonNotWritable {
		t.Fatalf("%s %q", result.Level, result.Reason)
	}
	r.wantLog()
}

// The reader does not publish the setter capability, so the writer declares a
// route to a capability instance that the reader has not published. That is the
// named gap of this row, kept as a test of the published batches.
func TestTheReaderStillPublishesNoSetterCapability(t *testing.T) {
	r := newRig(t)
	r.seed(idle(32), 990)
	r.apply(amperes("16", 0))
	r.session.mu.Lock()
	defer r.session.mu.Unlock()
	for _, batch := range r.session.batches {
		for _, c := range batch.CapabilityUpserts {
			if c.Definition.ID == CapabilityID {
				t.Fatal("the setter capability is published")
			}
		}
	}
}

func TestAFailedRouteCommitFailsTheStartAndRegistersNothing(t *testing.T) {
	r := newRig(t)
	w, err := NewWriter(Config{Reader: r.reader, Publication: r.pub, ClockEpoch: "clock:writer", Clock: r.clk})
	if err != nil {
		t.Fatal(err)
	}
	r.session.mu.Lock()
	r.session.routeErr = context.Canceled
	r.session.mu.Unlock()
	before := len(r.session.live())
	if err := w.Start(context.Background(), r.session); err == nil {
		t.Fatal("the start succeeded with no route")
	}
	if len(r.session.live()) != before {
		t.Fatal("an executor was registered without a route")
	}
	var _ driver.Instance = w
}
