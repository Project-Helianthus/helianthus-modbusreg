package write

import (
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/driver/evse/reader"
	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

// setFor scripts the t28 that learns the configured current, then the exchange.
func (r *rig) setFor(configured uint32, returned scalars) {
	r.ep.push(r.readback(idle(configured), 990))
	r.exchange(returned, 1000, 1002)
}

func TestSixteenAmpsSetIsConfirmedByTheReturnedReadback(t *testing.T) {
	r := newRig(t)
	r.setFor(32, device(16, 900, false))
	result := r.apply(amperes("16", 0))
	if result.Level != LevelReadbackConfirmed {
		t.Fatalf("level %s (%s): %+v", result.Level, result.Reason, result.Native)
	}
	r.wantLog("t27", "t25(16,900,false)", "t27")
	if !result.HasSent || result.Sent.LimitCurrentMaxAmps != 16 || result.Sent.LimitTimeoutSeconds != 900 || result.Sent.InhibitCharging {
		t.Fatalf("sent %+v", result.Sent)
	}
	if !result.HasPrerequisite || !result.HasConfigured || result.Configured != 32 {
		t.Fatalf("the configured current is not from a t28 read first: %+v", result)
	}
	if !result.HasReturned || result.Returned.LimitCurrentMaxAmps != 16 || result.Returned.LimitTimeoutSeconds != 900 || result.Returned.InhibitCharging {
		t.Fatalf("returned %+v", result.Returned)
	}
	c, ok := r.session.candidate(reader.ElementAllocatedCurrent)
	if !ok || ampsOf(t, c) != 16 {
		t.Fatalf("the published allocated current is not the returned 16 A: %v %+v", ok, c.Value)
	}
}

// A returned value that differs from the request stays evidence: the allocated
// current follows the returned t28, and the route is never readback_confirmed.
func TestReturnedValuesThatDifferAreEvidenceAndNotConfirmation(t *testing.T) {
	rows := []struct {
		name      string
		returned  scalars
		level     Level
		published int64
		known     bool
	}{
		{"another current", device(12, 900, false), LevelConflict, 12, true},
		{"another timeout", device(16, 600, false), LevelProtocolAck, 16, true},
		{"inhibit set", device(16, 900, true), LevelConflict, 0, true},
		{"timeout absent", scalars{limit: u(16), inhibit: b(false), configured: u(32)}, LevelProtocolAck, 16, true},
		{"zero current without inhibit", device(0, 900, false), LevelProtocolAck, 0, false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			r := newRig(t)
			r.setFor(32, row.returned)
			result := r.apply(amperes("16", 0))
			if result.Level != row.level || result.Level == LevelReadbackConfirmed {
				t.Fatalf("level %s (%s), want %s", result.Level, result.Reason, row.level)
			}
			if result.Matches {
				t.Fatal("a differing readback matches")
			}
			rb := row.returned
			if !result.HasReturned || result.Returned.LimitCurrentMaxAmps != *rb.limit {
				t.Fatalf("the native values are not kept: %+v", result.Returned)
			}
			if result.HasPublished != row.known || (row.known && int64(result.Published) != row.published) {
				t.Fatalf("published %d known %t, want %d known %t", result.Published, result.HasPublished, row.published, row.known)
			}
			c, ok := r.session.candidate(reader.ElementAllocatedCurrent)
			if ok != row.known || (ok && ampsOf(t, c) != row.published) {
				t.Fatalf("candidate present %t, want %t", ok, row.known)
			}
			if !row.known && r.reader.View().Reason(reader.ElementAllocatedCurrent) != reader.ReasonZeroCurrent {
				t.Fatalf("the zero current is not left unknown: %q", r.reader.View().Reason(reader.ElementAllocatedCurrent))
			}
			if result.Sent.LimitCurrentMaxAmps != 16 {
				t.Fatalf("the requested value was replaced: %+v", result.Sent)
			}
		})
	}
}

func TestPauseSendsSixAmpsInhibitAndPublishesZero(t *testing.T) {
	r := newRig(t)
	r.setFor(32, device(6, 900, true))
	result := r.apply(amperes("0", 0))
	if result.Level != LevelReadbackConfirmed {
		t.Fatalf("level %s (%s)", result.Level, result.Reason)
	}
	r.wantLog("t27", "t25(6,900,true)", "t27")
	if result.Sent.LimitCurrentMaxAmps != 6 || result.Sent.LimitTimeoutSeconds != 900 || !result.Sent.InhibitCharging {
		t.Fatalf("sent %+v", result.Sent)
	}
	if result.Returned.LimitCurrentMaxAmps != 6 || !result.HasPublished || result.Published != 0 {
		t.Fatalf("returned %+v published %d/%t", result.Returned, result.Published, result.HasPublished)
	}
	if c, ok := r.session.candidate(reader.ElementAllocatedCurrent); !ok || ampsOf(t, c) != 0 {
		t.Fatalf("the allocated current is not 0 A: %v", ok)
	}
}

func TestValuesTheDeviceAndItsStandardDoNotAdmitSendNothing(t *testing.T) {
	t.Run("below the pilot minimum", func(t *testing.T) {
		r := newRig(t)
		for _, n := range []string{"1", "2", "3", "4", "5"} {
			result := r.apply(amperes(n, 0))
			if result.Level != LevelFailedNoContact || result.Reason != ReasonBelowFloor {
				t.Fatalf("%s A: %s %q", n, result.Level, result.Reason)
			}
		}
		r.wantLog()
	})
	t.Run("not a whole number of amperes", func(t *testing.T) {
		r := newRig(t)
		r.seed(idle(16), 990)
		for _, q := range []semreg.Quantity{amperes("65", -1), amperes("1601", -2), amperes("5", -1)} {
			result := r.apply(q)
			if result.Level != LevelFailedNoContact || result.Reason != ReasonNotWhole {
				t.Fatalf("%+v: %s %q", q.Number, result.Level, result.Reason)
			}
		}
		r.wantLog()
	})
	t.Run("not in amperes or negative", func(t *testing.T) {
		r := newRig(t)
		if result := r.apply(semreg.Quantity{Number: semreg.Decimal{Coefficient: "16"}, Unit: "unit.watt"}); result.Reason != ReasonUnit {
			t.Fatalf("%q", result.Reason)
		}
		if result := r.apply(amperes("-3", 0)); result.Reason != ReasonNegative {
			t.Fatalf("%q", result.Reason)
		}
		r.wantLog()
	})
	t.Run("above the configured current with a t28 retained", func(t *testing.T) {
		r := newRig(t)
		r.seed(idle(16), 990)
		result := r.apply(amperes("20", 0))
		if result.Level != LevelFailedNoContact || result.Reason != ReasonAboveConfig || result.HasPrerequisite {
			t.Fatalf("%s %q %+v", result.Level, result.Reason, result)
		}
		r.wantLog()
	})
	t.Run("above the configured current with no t28 reads t27 and sends no t25", func(t *testing.T) {
		r := newRig(t)
		r.ep.push(r.readback(idle(16), 990))
		result := r.apply(amperes("20", 0))
		if result.Level != LevelFailedNoContact || result.Reason != ReasonAboveConfig || !result.HasPrerequisite {
			t.Fatalf("%s %q", result.Level, result.Reason)
		}
		r.wantLog("t27")
		retained, ok := r.reader.Acquisition().Retained(tesla.FC100OperationWCGetProvisional)
		if !ok || !retained.HasLastGood {
			t.Fatal("the t28 is not retained")
		}
		if last, ok := r.reader.Feed().LastProvisional(); !ok || !last.Readback.HasConfiguredCurrent || last.Readback.ConfiguredCurrentAmps != 16 {
			t.Fatalf("the t28 did not reach the reader: %+v", last)
		}
	})
	t.Run("a t28 without a usable configured current", func(t *testing.T) {
		for name, s := range map[string]scalars{"zero": {limit: u(32), configured: u(0)}, "omitted": {limit: u(32)}} {
			r := newRig(t)
			r.seed(s, 990)
			if result := r.apply(amperes("16", 0)); result.Reason != ReasonNoConfigured || result.Level != LevelFailedNoContact {
				t.Fatalf("%s: %s %q", name, result.Level, result.Reason)
			}
			r.wantLog()
		}
	})
}

// Each field of the guard is varied alone; the mutation each row catches is the
// comparison of that field dropped from the guard check.
func TestAGuardOfAnotherRouteSendsNothing(t *testing.T) {
	rows := []struct {
		field  string
		mutate func(g *guardClaims)
	}{
		{"asset (asset comparison dropped)", func(g *guardClaims) { g.AssetID = "asset:other" }},
		{"binding (binding comparison dropped)", func(g *guardClaims) { g.BindingID = "binding:other" }},
		{"source (source comparison dropped)", func(g *guardClaims) { g.SourceID = "source:other" }},
		{"epoch (epoch comparison dropped)", func(g *guardClaims) { g.SourceEpochID = "epoch:other" }},
		{"generation (generation comparison dropped)", func(g *guardClaims) { g.DriverGeneration = "8" }},
		{"service (service comparison dropped)", func(g *guardClaims) { g.ServiceInstance = "service:other" }},
		{"capability (capability comparison dropped)", func(g *guardClaims) { g.CapabilityInstance = "capability:other" }},
	}
	for _, row := range rows {
		t.Run(row.field, func(t *testing.T) {
			r := newRig(t)
			r.seed(idle(32), 990)
			guard := r.guard()
			row.mutate(&guard)
			r.clk.set(unix(1001))
			result := r.writer.ApplyAllocatedCurrent(ctxb(), guard, amperes("16", 0))
			if result.Level != LevelFailedNoContact || result.Reason != ReasonGuard {
				t.Fatalf("%s %q", result.Level, result.Reason)
			}
			r.wantLog()
		})
	}
}

func TestALifecycleThatIsNotReadySendsNothing(t *testing.T) {
	r := newRig(t)
	r.seed(idle(32), 990)
	r.reader.Stop(ctxb())
	result := r.apply(amperes("16", 0))
	if result.Level != LevelFailedNoContact || result.Reason != ReasonNotReady {
		t.Fatalf("%s %q", result.Level, result.Reason)
	}
	r.wantLog()
}

func TestATimeoutAfterPossibleTransmissionIsNotResent(t *testing.T) {
	r := newRig(t)
	r.ep.push(r.readback(idle(32), 990), step{timeout: true})
	result := r.apply(amperes("16", 0))
	if result.Level != LevelUnknown || result.Reason != ReasonNoAcknowledge {
		t.Fatalf("%s %q", result.Level, result.Reason)
	}
	again := r.apply(amperes("16", 0))
	if again.Level != LevelFailedNoContact {
		t.Fatalf("the second command: %s %q", again.Level, again.Reason)
	}
	r.wantLog("t27", "t25(16,900,false)")
}

func TestAnAcknowledgementWithoutReadbackIsProtocolAcknowledgement(t *testing.T) {
	r := newRig(t)
	r.ep.push(r.readback(idle(32), 990), r.ack(1000), step{timeout: true})
	result := r.apply(amperes("16", 0))
	if result.Level != LevelProtocolAck || result.Reason != ReasonNoReadback || result.HasReturned {
		t.Fatalf("%s %q", result.Level, result.Reason)
	}
	r.wantLog("t27", "t25(16,900,false)", "t27")
}
