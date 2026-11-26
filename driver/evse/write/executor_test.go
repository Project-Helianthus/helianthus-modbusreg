package write

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/driver/evse/reader"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

func TestExecutorAnswersASetWithDeliveryAcknowledgementAndReadback(t *testing.T) {
	r := newRig(t)
	r.setFor(32, device(16, 900, false))
	c := r.execute(quantityArg(amperes("16", 0)))
	if c.Outcome != operation.OutcomeApplied {
		t.Fatalf("outcome %s", c.Outcome)
	}
	if c.Dispatch.Delivery != operation.DeliverySent || !c.Dispatch.PossibleSideEffect || c.Dispatch.Completed == nil {
		t.Fatalf("dispatch %+v", c.Dispatch)
	}
	if c.Acknowledgement == nil || c.Acknowledgement.State != operation.AckAccepted {
		t.Fatalf("acknowledgement %+v", c.Acknowledgement)
	}
	if c.Readback == nil || c.ReadbackSnapshot == nil || c.Readback.Relation != operation.ReadbackConfirms {
		t.Fatalf("readback %+v", c.Readback)
	}
	if want := r.pub.ConnectorCandidateIdentifier(reader.ElementAllocatedCurrent); c.Readback.CandidateID != want {
		t.Fatalf("the readback names %s, want %s", c.Readback.CandidateID, want)
	}
	if c.Readback.SnapshotID != c.ReadbackSnapshot.SnapshotID || c.Readback.Revisions != c.ReadbackSnapshot.Revisions {
		t.Fatal("the readback does not name its snapshot")
	}
	completed, received := wallOf(t, c.Dispatch.Completed.EvaluatedAt), receivedOf(t, c)
	if !received.After(completed) {
		t.Fatalf("the readback candidate (%v) is not strictly after the dispatch completion (%v)", received, completed)
	}
	if c.Dispatch.Evidence[0].Kind != KindRequest || c.Readback.Evidence[0].Kind != KindReturned {
		t.Fatalf("the sent triple and the returned t28 are not distinct evidence: %v %v", c.Dispatch.Evidence[0].Kind, c.Readback.Evidence[0].Kind)
	}
	if c.Dispatch.Evidence[0].Digest == c.Readback.Evidence[0].Digest {
		t.Fatal("the sent triple and the returned t28 share evidence")
	}
}

func TestExecutorAnswersAPauseAsAnAmperesSet(t *testing.T) {
	r := newRig(t)
	r.setFor(32, device(6, 900, true))
	c := r.execute(quantityArg(amperes("0", 0)))
	if c.Outcome != operation.OutcomeApplied || c.Readback == nil {
		t.Fatalf("outcome %s", c.Outcome)
	}
	r.wantLog("t27", "t25(6,900,true)", "t27")
}

func TestExecutorOutcomesOfAMismatchAndOfMissingEvidence(t *testing.T) {
	t.Run("another current is a conflict", func(t *testing.T) {
		r := newRig(t)
		r.setFor(32, device(12, 900, false))
		c := r.execute(quantityArg(amperes("16", 0)))
		if c.Outcome != operation.OutcomeConflict || c.Readback == nil || c.Readback.Relation != operation.ReadbackContradicts {
			t.Fatalf("outcome %s", c.Outcome)
		}
	})
	t.Run("another timeout is acknowledged and unverified", func(t *testing.T) {
		r := newRig(t)
		r.setFor(32, device(16, 600, false))
		c := r.execute(quantityArg(amperes("16", 0)))
		if c.Outcome != operation.OutcomeAcknowledgedUnverified || c.Readback != nil || c.ReadbackSnapshot != nil {
			t.Fatalf("outcome %s", c.Outcome)
		}
	})
	t.Run("a zero current without inhibit is acknowledged and unverified", func(t *testing.T) {
		r := newRig(t)
		r.setFor(32, device(0, 900, false))
		c := r.execute(quantityArg(amperes("16", 0)))
		if c.Outcome != operation.OutcomeAcknowledgedUnverified || c.Readback != nil {
			t.Fatalf("outcome %s", c.Outcome)
		}
	})
	t.Run("an acknowledgement without readback", func(t *testing.T) {
		r := newRig(t)
		r.ep.push(r.readback(idle(32), 990), r.ack(1000), step{timeout: true})
		c := r.execute(quantityArg(amperes("16", 0)))
		if c.Outcome != operation.OutcomeAcknowledgedUnverified || c.Acknowledgement == nil || c.Readback != nil {
			t.Fatalf("outcome %s", c.Outcome)
		}
	})
	t.Run("possible transmission without acknowledgement", func(t *testing.T) {
		r := newRig(t)
		r.ep.push(r.readback(idle(32), 990), step{timeout: true})
		c := r.execute(quantityArg(amperes("16", 0)))
		if c.Outcome != operation.OutcomeIndeterminate || c.Dispatch.Delivery != operation.DeliveryUnknown || !c.Dispatch.PossibleSideEffect ||
			c.Dispatch.Completed != nil || c.Acknowledgement != nil {
			t.Fatalf("outcome %s dispatch %+v", c.Outcome, c.Dispatch)
		}
		r.wantLog("t27", "t25(16,900,false)")
	})
}

// A pause whose prerequisite t27 times out encoded no t25: the completion
// carries no t25 request evidence and no t25 delivery state.
func TestAPauseWhoseConfiguredCurrentReadTimesOutEncodedNoLimitRequest(t *testing.T) {
	r := newRig(t)
	r.ep.push(step{timeout: true})
	c := r.execute(quantityArg(amperes("0", 0)))
	if c.Outcome != operation.OutcomeFailedNoContact {
		t.Fatalf("outcome %s", c.Outcome)
	}
	if c.Dispatch.Delivery != operation.DeliveryNotSent || c.Dispatch.PossibleSideEffect {
		t.Fatalf("a t25 is stated: %+v", c.Dispatch)
	}
	for _, ref := range c.OutcomeEvidence {
		if ref.Kind == KindRequest {
			t.Fatal("request evidence for a t25 that was never encoded")
		}
	}
	r.wantLog("t27")
}

// Every command the row does not admit is answered before anything is sent.
func TestExecutorRefusesBeforeAnySend(t *testing.T) {
	other := "connector-b"
	otherConnector := semreg.FactKey{PackID: evsePack.ID, PackVersion: evsePack.Version, FactID: ArgumentID,
		Dimensions: []semreg.Dimension{{ID: DimensionID, Value: semreg.Value{Kind: semreg.ValueText, Text: &other}}}}
	enabled := semreg.TypedField{ID: "evse.limit.allocated_current_enabled", Value: semreg.Value{Kind: semreg.ValueBoolean, Boolean: new(bool)}}
	rows := []struct {
		name   string
		args   []semreg.TypedField
		reason string
		mutate func(r *rig, d *dispatchT)
	}{
		{"3 A", []semreg.TypedField{quantityArg(amperes("3", 0))}, ReasonBelowFloor, nil},
		{"6.5 A", []semreg.TypedField{quantityArg(amperes("65", -1))}, ReasonNotWhole, nil},
		{"-3 A", []semreg.TypedField{quantityArg(amperes("-3", 0))}, ReasonNegative, nil},
		{"20 A above 16 A", []semreg.TypedField{quantityArg(amperes("20", 0))}, ReasonAboveConfig, nil},
		{"the boolean true", []semreg.TypedField{booleanArg(true)}, ReasonReleaseTrue, nil},
		{"a set and a release", []semreg.TypedField{quantityArg(amperes("16", 0)), enabled}, ReasonArguments, nil},
		{"no argument", nil, ReasonArguments, nil},
		{"an argument of another element", []semreg.TypedField{{ID: "evse.limit.configured_current", Value: quantityArg(amperes("16", 0)).Value}}, ReasonArguments, nil},
		{"another connector", []semreg.TypedField{quantityArg(amperes("16", 0))}, ReasonForeignRoute, func(r *rig, d *dispatchT) { d.ExpectedEffect.Fact = otherConnector }},
		{"another capability instance", []semreg.TypedField{quantityArg(amperes("16", 0))}, ReasonForeignRoute, func(r *rig, d *dispatchT) {
			d.Request.CapabilityInstanceID = "capability:other"
		}},
		{"another operation", []semreg.TypedField{quantityArg(amperes("16", 0))}, ReasonForeignRoute, func(r *rig, d *dispatchT) {
			d.Request.Operation.ID = "evse.operation.set_allocated_current_phase"
		}},
		{"another generation", []semreg.TypedField{quantityArg(amperes("16", 0))}, ReasonGuard, func(r *rig, d *dispatchT) { d.Guard.DriverGeneration = "8" }},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			r := newRig(t)
			r.seed(idle(16), 990)
			d := r.dispatch(row.args...)
			if row.mutate != nil {
				row.mutate(r, &d)
			}
			r.clk.set(unix(1001))
			c, err := r.writer.Execute(ctxb(), d)
			if err != nil || c.Validate() != nil {
				t.Fatalf("err %v validate %v", err, c.Validate())
			}
			if c.Outcome != operation.OutcomeFailedNoContact || c.Dispatch == nil || c.Dispatch.Delivery != operation.DeliveryNotSent || c.Dispatch.PossibleSideEffect {
				t.Fatalf("outcome %s dispatch %+v", c.Outcome, c.Dispatch)
			}
			if c.Acknowledgement != nil || c.Readback != nil || c.ReadbackSnapshot != nil {
				t.Fatal("a refusal carries acknowledgement or readback")
			}
			steps := c.CompoundExecution.Steps
			if last := steps[len(steps)-1]; last.Attempted || last.Reason != row.reason {
				t.Fatalf("the reason is %q, want %q", last.Reason, row.reason)
			}
			r.wantLog()
		})
	}
}

func TestEveryRequestCarriesANonZeroTimeout(t *testing.T) {
	for name, amps := range map[string]string{"set": "16", "pause": "0"} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.setFor(32, device(6, 900, true))
			result := r.apply(amperes(amps, 0))
			if !result.HasSent || result.Sent.LimitTimeoutSeconds != LimitTimeoutSeconds {
				t.Fatalf("sent %+v", result.Sent)
			}
			for _, entry := range r.log() {
				if strings.HasPrefix(entry, "other:") || strings.Contains(entry, ",0,") {
					t.Fatalf("a request with another timeout: %s", entry)
				}
			}
		})
	}
}

type dispatchT = driver.Dispatch

func wallOf(t *testing.T, p semreg.TimePoint) time.Time {
	t.Helper()
	ns, err := strconv.ParseInt(string(p.UnixNanoseconds), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return time.Unix(0, ns).UTC()
}

func receivedOf(t *testing.T, c driver.Completion) time.Time {
	t.Helper()
	for _, envelope := range c.ReadbackSnapshot.Facts {
		for _, candidate := range envelope.Candidates {
			if candidate.CandidateID == c.Readback.CandidateID {
				return wallOf(t, candidate.Times.ReceivedAt)
			}
		}
	}
	t.Fatal("the readback candidate is not in its snapshot")
	return time.Time{}
}
