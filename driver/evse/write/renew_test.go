package write

import (
	"testing"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/driver/evse/reader"
	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
)

// setAndHold performs a 16 A set that the device returns as requested.
func setAndHold(t *testing.T, r *rig, configured uint32) Result {
	t.Helper()
	r.setFor(configured, configuredAs(device(16, 900, false), configured))
	result := r.apply(amperes("16", 0))
	if result.Level != LevelReadbackConfirmed {
		t.Fatalf("level %s (%s)", result.Level, result.Reason)
	}
	return result
}

func (r *rig) wantRequests(n int) {
	r.t.Helper()
	waitFor(r.t, "the requests", func() bool { return len(r.log()) >= n })
}

func TestALimitInForceIsSentAgainEveryThreeHundredSeconds(t *testing.T) {
	r := newRig(t)
	setAndHold(t, r, 32)
	waitFor(t, "the renewal to wait", func() bool { return r.clk.waiting() == 1 })
	r.exchange(device(16, 900, false), 1300, 1302)
	r.clk.advance(299 * time.Second)
	time.Sleep(20 * time.Millisecond)
	r.wantLog("t27", "t25(16,900,false)", "t27")
	r.clk.advance(time.Second)
	r.wantRequests(5)
	r.wantLog("t27", "t25(16,900,false)", "t27", "t25(16,900,false)", "t27")
	waitFor(t, "the next renewal to wait", func() bool { return r.clk.waiting() == 1 })
	r.exchange(device(16, 900, false), 1600, 1602)
	r.clk.advance(300 * time.Second)
	r.wantRequests(7)
	r.wantLog("t27", "t25(16,900,false)", "t27", "t25(16,900,false)", "t27", "t25(16,900,false)", "t27")
}

// The renewal follows the native comparison of the returned t28, not the label
// of the result: a set whose later snapshot is unavailable is still renewed.
func TestRenewalDoesNotWaitForTheResultLabel(t *testing.T) {
	r := newRig(t)
	r.session.mu.Lock()
	r.session.captureErr = errCapture
	r.session.mu.Unlock()
	r.setFor(32, device(16, 900, false))
	result := r.apply(amperes("16", 0))
	if result.Level != LevelProtocolAck || !result.Matches {
		t.Fatalf("level %s matches %t", result.Level, result.Matches)
	}
	waitFor(t, "the renewal to wait", func() bool { return r.clk.waiting() == 1 })
}

func TestARequestThatIsNotEstablishedIsNotRenewed(t *testing.T) {
	rows := map[string]scalars{"another current": device(12, 900, false), "another timeout": device(16, 600, false), "inhibit set": device(16, 900, true)}
	for name, returned := range rows {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.setFor(32, returned)
			r.apply(amperes("16", 0))
			if r.clk.waiting() != 0 {
				t.Fatal("a limit that was not established is renewed")
			}
			r.clk.advance(time.Hour)
			time.Sleep(20 * time.Millisecond)
			r.wantLog("t27", "t25(16,900,false)", "t27")
		})
	}
	t.Run("a request that was refused leaves a limit in force renewed", func(t *testing.T) {
		r := newRig(t)
		setAndHold(t, r, 32)
		waitFor(t, "the renewal to wait", func() bool { return r.clk.waiting() == 1 })
		if result := r.apply(amperes("3", 0)); result.Level != LevelFailedNoContact {
			t.Fatalf("%s", result.Level)
		}
		if r.clk.waiting() != 1 {
			t.Fatal("a refusal ended the renewal")
		}
	})
}

func TestAPauseIsRenewedWithItsOwnTriple(t *testing.T) {
	r := newRig(t)
	r.setFor(32, device(6, 900, true))
	if result := r.apply(amperes("0", 0)); result.Level != LevelReadbackConfirmed {
		t.Fatalf("%s %q", result.Level, result.Reason)
	}
	waitFor(t, "the renewal to wait", func() bool { return r.clk.waiting() == 1 })
	r.exchange(device(6, 900, true), 1300, 1302)
	r.clk.advance(RenewPeriod)
	r.wantRequests(5)
	r.wantLog("t27", "t25(6,900,true)", "t27", "t25(6,900,true)", "t27")
}

// A renewal that is not read back as requested ends the renewal and keeps its
// result; it is not sent a second time.
func TestARenewalThatIsNotConfirmedEndsTheRenewalWithoutRetry(t *testing.T) {
	r := newRig(t)
	setAndHold(t, r, 32)
	waitFor(t, "the renewal to wait", func() bool { return r.clk.waiting() == 1 })
	r.exchange(device(12, 900, false), 1300, 1302)
	r.clk.advance(RenewPeriod)
	r.wantRequests(5)
	waitFor(t, "the renewal to end", func() bool { return r.clk.waiting() == 0 })
	if v, ok := r.reader.View().Fact(reader.ElementAllocatedCurrent); !ok || v.Amps != 12 {
		t.Fatalf("the returned state is not published: %+v", v)
	}
	r.clk.advance(time.Hour)
	time.Sleep(20 * time.Millisecond)
	r.wantLog("t27", "t25(16,900,false)", "t27", "t25(16,900,false)", "t27")
}

// A tick that meets a read holding the reservation sends no t25; the limit is
// still in force and the next tick sends it again.
func TestATickThatSendsNothingLeavesTheRenewalArmed(t *testing.T) {
	r := newRig(t)
	setAndHold(t, r, 32)
	waitFor(t, "the renewal to wait", func() bool { return r.clk.waiting() == 1 })
	hold, release := make(chan struct{}), make(chan struct{})
	r.ep.push(step{echo: true, terminal: terminal(4, 3, okBody), hold: hold, release: release})
	done := make(chan error, 1)
	go func() {
		_, err := r.reader.Acquisition().Acquire(ctxb(), tesla.FC100OperationCommonSystemInfo)
		done <- err
	}()
	<-hold
	r.clk.advance(RenewPeriod)
	waitFor(t, "the renewal to wait again", func() bool { return r.clk.waiting() == 1 })
	r.wantLog("t27", "t25(16,900,false)", "t27", "poll")
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	r.exchange(device(16, 900, false), 1300, 1302)
	r.clk.advance(RenewPeriod)
	r.wantRequests(6)
	r.wantLog("t27", "t25(16,900,false)", "t27", "poll", "t25(16,900,false)", "t27")
}

func TestStoppingTheWriterEndsTheRenewal(t *testing.T) {
	r := newRig(t)
	setAndHold(t, r, 32)
	waitFor(t, "the renewal to wait", func() bool { return r.clk.waiting() == 1 })
	if report := r.writer.Stop(ctxb()); !report.Confirmed {
		t.Fatalf("%+v", report)
	}
	r.exchange(device(16, 900, false), 1300, 1302)
	r.clk.advance(time.Hour)
	time.Sleep(20 * time.Millisecond)
	r.wantLog("t27", "t25(16,900,false)", "t27")
}
