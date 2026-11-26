package write

import (
	"testing"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/driver/evse/reader"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

func (r *rig) execute(args ...semreg.TypedField) driver.Completion {
	r.t.Helper()
	r.clk.set(unix(1001))
	completion, err := r.writer.Execute(ctxb(), r.dispatch(args...))
	if err != nil {
		r.t.Fatal(err)
	}
	if err := completion.Validate(); err != nil {
		r.t.Fatalf("the completion is not valid: %v", err)
	}
	return completion
}

// A release sends one t25 with the configured current of the latest t28, 900
// seconds and inhibit false, then the t26 and the t27 → t28, and no further
// renewal occurs.
func TestReleaseSendsTheConfiguredCurrentOnceAndStopsRenewal(t *testing.T) {
	r := newRig(t)
	setAndHold(t, r, 20)
	waitFor(t, "the renewal to wait", func() bool { return r.clk.waiting() == 1 })
	r.exchange(configuredAs(device(20, 900, false), 20), 1100, 1102)
	completion := r.execute(booleanArg(false))
	r.wantLog("t27", "t25(16,900,false)", "t27", "t25(20,900,false)", "t27")
	if completion.Outcome != operation.OutcomeAcknowledgedUnverified || completion.Readback != nil || completion.ReadbackSnapshot != nil {
		t.Fatalf("a release is acknowledged and never read back: %+v", completion.Outcome)
	}
	if r.armed() {
		t.Fatal("the renewal was not stopped")
	}
	if got, ok := r.reader.View().Fact(reader.ElementAllocatedCurrent); !ok || got.Amps != 20 {
		t.Fatalf("the next readback does not publish the returned device state: %+v", got)
	}
	r.exchange(device(16, 900, false), 1400, 1402)
	r.clk.advance(time.Hour)
	time.Sleep(20 * time.Millisecond)
	r.wantLog("t27", "t25(16,900,false)", "t27", "t25(20,900,false)", "t27")
	var kinds []semreg.DefinitionID
	for _, e := range completion.OutcomeEvidence {
		kinds = append(kinds, e.Kind)
	}
	if !hasKind(kinds, KindRequest) || !hasKind(kinds, KindRelease) {
		t.Fatalf("the sent triple and the returned t28 are not both kept: %v", kinds)
	}
}

func hasKind(kinds []semreg.DefinitionID, kind semreg.DefinitionID) bool {
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func TestAFirstReleaseReadsTheConfiguredCurrentFirst(t *testing.T) {
	r := newRig(t)
	r.ep.push(r.readback(idle(24), 990))
	r.exchange(configuredAs(device(24, 900, false), 24), 1000, 1002)
	completion := r.execute(booleanArg(false))
	r.wantLog("t27", "t25(24,900,false)", "t27")
	if completion.Outcome != operation.OutcomeAcknowledgedUnverified {
		t.Fatalf("%s", completion.Outcome)
	}
}

func TestAReleaseIsNeverReadbackConfirmed(t *testing.T) {
	for name, returned := range map[string]scalars{"equal": device(20, 900, false), "another current": device(12, 900, false), "another timeout": device(20, 600, false)} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.ep.push(r.readback(idle(20), 990))
			r.exchange(returned, 1000, 1002)
			r.clk.set(unix(1001))
			result := r.writer.release(ctxb(), r.guard())
			if result.Level != LevelProtocolAck || !result.Release || result.Snapshot != nil || result.Candidate != nil {
				t.Fatalf("%s release=%t", result.Level, result.Release)
			}
			if result.Matches != (name == "equal") {
				t.Fatalf("the native comparison of the release readback is %t", result.Matches)
			}
			if !result.HasReturned || result.Returned.LimitCurrentMaxAmps != *returned.limit {
				t.Fatalf("returned %+v", result.Returned)
			}
		})
	}
}

// A configured current that is zero or omitted cannot form the release triple:
// nothing is sent, the t28 is kept, and the limit in force keeps its renewal.
func TestAReleaseWithoutAUsableConfiguredCurrentSendsNothing(t *testing.T) {
	for name, configured := range map[string]*uint32{"zero": u(0), "omitted": nil} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.ep.push(r.readback(idle(32), 990))
			r.exchange(scalars{limit: u(16), timeout: u(900), inhibit: b(false), configured: configured}, 1000, 1002)
			if result := r.apply(amperes("16", 0)); result.Level != LevelReadbackConfirmed {
				t.Fatalf("%s %q", result.Level, result.Reason)
			}
			waitFor(t, "the renewal to wait", func() bool { return r.clk.waiting() == 1 })
			before := r.log()
			completion := r.execute(booleanArg(false))
			if completion.Outcome != operation.OutcomeFailedNoContact || completion.Dispatch.Delivery != operation.DeliveryNotSent {
				t.Fatalf("%s", completion.Outcome)
			}
			if len(r.log()) != len(before) {
				t.Fatalf("something was sent: %v", r.log())
			}
			if !r.armed() {
				t.Fatal("the refusal changed the renewal of the limit in force")
			}
			last, ok := r.reader.Feed().LastProvisional()
			if !ok || last.Readback.HasConfiguredCurrent != (configured != nil) {
				t.Fatalf("the matching t28 is not kept: %+v", last)
			}
		})
	}
}

// A request for the configured current as amperes is an ordinary set: it is
// recorded as one and renewed.
func TestSettingTheConfiguredCurrentIsNotARelease(t *testing.T) {
	r := newRig(t)
	r.ep.push(r.readback(idle(16), 990))
	r.exchange(device(16, 900, false), 1000, 1002)
	completion := r.execute(quantityArg(amperes("16", 0)))
	if completion.Outcome != operation.OutcomeApplied || completion.Readback == nil {
		t.Fatalf("%s", completion.Outcome)
	}
	for _, e := range completion.OutcomeEvidence {
		if e.Kind == KindRelease {
			t.Fatal("a set is recorded as a release")
		}
	}
	waitFor(t, "the renewal to wait", func() bool { return r.clk.waiting() == 1 })
}

func (r *rig) armed() bool {
	r.writer.mu.Lock()
	defer r.writer.mu.Unlock()
	return r.writer.renewal != nil
}
