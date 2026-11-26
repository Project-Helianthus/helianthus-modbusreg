package reader

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

// ingestReadback has the reader acquire one provisional readback scripted as
// given and feed it, returning when it was taken.
func ingestReadback(t *testing.T, reader *ChargerReader, c *charger, s step) {
	t.Helper()
	c.endpoint.push(s)
	waitFor(t, "the readback", func() bool {
		record, err := reader.Acquisition().Acquire(context.Background(), tesla.FC100OperationWCGetProvisional)
		return err == nil && record.State == tesla.Gen3ReadEvidenceAck && reader.IngestPolled(context.Background(), record) == nil
	})
}

func startAfterFirstPoll(t *testing.T) (*ChargerReader, *fakeSession, *charger, *clock) {
	t.Helper()
	reader, session, c, clk := runWith(t, mutationConfig(), syntheticPublication(), nil)
	waitFor(t, "the first poll", func() bool {
		p, _ := reader.Payload(SlotCommonSystemInfo)
		return p.Present
	})
	return reader, session, c, clk
}

func nanos(t *testing.T, v semreg.Uint64) int64 {
	t.Helper()
	n, err := strconv.ParseInt(string(v), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// The evaluation instant on the monotonic axis is the reader's clock at the
// evaluation. The clock is advanced and the reader evaluates again: the
// instant moves by exactly what the clock moved, whatever the receipts hold,
// and the elapsed time of a record on the reader's own clock epoch is read on
// the monotonic axis, not on the wall axis.
func TestEvaluationInstantFollowsTheReadersClockOnItsOwnEpoch(t *testing.T) {
	reader, session, c, clk := startAfterFirstPoll(t)
	// The reader's monotonic axis starts at 5 s on its clock, so 105 s is 100 s on it.
	clk.set(time.Unix(105, 0))
	// The receipt is 70 s on the reader's own epoch, and 100 s on the wall: 30 s elapsed on
	// the monotonic axis now, 5 s on the wall axis.
	ingestReadback(t, reader, c, step{echo: true, terminal: terminal(6, 28, readbackBody(scalars{limit: u(12), timeout: u(60), inhibit: b(false)})),
		wall: 100, epoch: "clock:synthetic", mono: 70 * time.Second})
	first := candidateOf(t, session.committed(), ElementAllocatedCurrent)
	if first.Value == nil || first.Quality.Availability != semreg.AvailabilityAvailable {
		t.Fatalf("the allocation is not published before the lifetime ends: %+v", first)
	}
	if first.Times.EvaluateMonotonic.ClockEpochID != "clock:synthetic" || nanos(t, first.Times.EvaluateMonotonic.Nanoseconds) != 100*int64(time.Second) {
		t.Fatalf("evaluation instant %+v, want the reader's clock at 100 s (a receipt gives 70 s)", first.Times.EvaluateMonotonic)
	}

	// 40 s later on the clock the monotonic elapsed time is 70 s, the wall elapsed time 45 s.
	clk.set(time.Unix(145, 0))
	if err := reader.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := candidateOf(t, session.committed(), ElementAllocatedCurrent)
	wantWithheld(t, second, ReasonProvisionalExpired)
	if got := nanos(t, second.Times.EvaluateMonotonic.Nanoseconds) - nanos(t, first.Times.EvaluateMonotonic.Nanoseconds); got != 40*int64(time.Second) {
		t.Fatalf("the evaluation instant moved by %d ns, the clock by 40 s", got)
	}
	elapsed, err := second.Times.ElapsedMonotonicNS()
	if err != nil || elapsed != "70000000000" {
		t.Fatalf("elapsed %q err %v", elapsed, err)
	}
	if second.Times.ReceiptMonotonic.ClockEpochID != "clock:synthetic" {
		t.Fatalf("receipt epoch %q", second.Times.ReceiptMonotonic.ClockEpochID)
	}
}

// A record from another clock epoch is published with its own epoch and its
// own monotonic receipt; the two axes are never compared, so its lifetime runs
// on the wall receipt, and the evaluation instant still follows the reader's
// clock rather than the largest receipt.
func TestRecordFromAnotherClockEpochIsPublishedWithItsOwnEpochAndAgesOnTheWall(t *testing.T) {
	reader, session, c, clk := startAfterFirstPoll(t)
	clk.set(time.Unix(105, 0))
	// The receipt's monotonic offset (170 s) is ahead of the reader's clock (100 s): comparing the axes
	// would give a negative age and the allocation would never expire.
	ingestReadback(t, reader, c, step{echo: true, terminal: terminal(6, 28, readbackBody(scalars{limit: u(12), timeout: u(60), inhibit: b(false)})),
		wall: 100, epoch: "another-epoch", mono: 170 * time.Second})
	first := candidateOf(t, session.committed(), ElementAllocatedCurrent)
	if first.Value == nil {
		t.Fatalf("the allocation is not published: %+v", first)
	}
	if first.Times.ReceiptMonotonic.ClockEpochID != "another-epoch" || nanos(t, first.Times.ReceiptMonotonic.Nanoseconds) != 170*int64(time.Second) {
		t.Fatalf("receipt %+v is not the record's own", first.Times.ReceiptMonotonic)
	}
	if first.Times.EvaluateMonotonic.ClockEpochID != "clock:synthetic" || nanos(t, first.Times.EvaluateMonotonic.Nanoseconds) != 100*int64(time.Second) {
		t.Fatalf("evaluation instant %+v, want the reader's clock at 100 s (a receipt gives 170 s)", first.Times.EvaluateMonotonic)
	}
	if _, err := first.Times.ElapsedMonotonicNS(); semreg.ErrorIdentifier(err) != semreg.IncomparableClockEpoch {
		t.Fatalf("elapsed across epochs: %v", err)
	}

	clk.set(time.Unix(159, 0))
	if err := reader.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	wantAmps(t, reader.View(), ElementAllocatedCurrent, 12)
	clk.set(time.Unix(160, 0))
	if err := reader.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := candidateOf(t, session.committed(), ElementAllocatedCurrent)
	wantWithheld(t, second, ReasonProvisionalExpired)
	if got := nanos(t, second.Times.EvaluateMonotonic.Nanoseconds) - nanos(t, first.Times.EvaluateMonotonic.Nanoseconds); got != 55*int64(time.Second) {
		t.Fatalf("the evaluation instant moved by %d ns, the clock by 55 s", got)
	}
	if second.Times.ReceiptMonotonic != first.Times.ReceiptMonotonic {
		t.Fatalf("the receipt changed: %+v", second.Times.ReceiptMonotonic)
	}
}
