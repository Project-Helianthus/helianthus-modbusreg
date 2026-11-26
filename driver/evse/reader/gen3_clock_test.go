package reader

import (
	"context"
	"errors"
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

func wallNanos(t *testing.T, v semreg.Int64) int64 {
	t.Helper()
	n, err := strconv.ParseInt(string(v), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n
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
// instant moves by exactly what the clock moved, whatever the receipts hold.
// The receipt is on the endpoint's own epoch, which differs from the reader's,
// so its lifetime runs on the wall axis and the two monotonic origins are
// never compared.
func TestEvaluationInstantFollowsTheReadersClockOnTheReadersEpoch(t *testing.T) {
	reader, session, c, clk := startAfterFirstPoll(t)
	// The reader's monotonic axis starts at 5 s on its clock, so 105 s is 100 s on it.
	clk.set(time.Unix(105, 0))
	// The receipt is 70 s on the endpoint's epoch and 100 s on the wall.
	ingestReadback(t, reader, c, step{echo: true, terminal: terminal(6, 28, readbackBody(scalars{limit: u(12), timeout: u(60), inhibit: b(false)})),
		wall: 100, epoch: "endpoint-clock", mono: 70 * time.Second})
	first := candidateOf(t, session.committed(), ElementAllocatedCurrent)
	if first.Value == nil || first.Quality.Availability != semreg.AvailabilityAvailable {
		t.Fatalf("the allocation is not published before the lifetime ends: %+v", first)
	}
	if first.Times.EvaluateMonotonic.ClockEpochID != "clock:synthetic" || nanos(t, first.Times.EvaluateMonotonic.Nanoseconds) != 100*int64(time.Second) {
		t.Fatalf("evaluation instant %+v, want the reader's clock at 100 s (a receipt gives 70 s)", first.Times.EvaluateMonotonic)
	}

	// 55 s later the wall elapsed time is 60 s: the lifetime has ended.
	clk.set(time.Unix(160, 0))
	if err := reader.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The expired allocation is no candidate: its candidate is withdrawn in a
	// batch observed 55 s later, and the disposition keeps the receipt it rests on.
	expired := wantDisposition(t, reader, ElementAllocatedCurrent, ReasonProvisionalExpired)
	committed := session.committed()
	last := committed[len(committed)-1]
	if len(last.FactWithdrawals) != 1 || last.FactWithdrawals[0] != first.CandidateID || len(last.FactUpserts) != 0 {
		t.Fatalf("the expiry batch withdraws %v and upserts %d, want only %s", last.FactWithdrawals, len(last.FactUpserts), first.CandidateID)
	}
	if got := wallNanos(t, last.ObservedAt.UnixNanoseconds) - wallNanos(t, first.Times.EvaluatedAt.UnixNanoseconds); got != 55*int64(time.Second) {
		t.Fatalf("the evaluation instant moved by %d ns, the clock by 55 s", got)
	}
	if expired.Receipt.ClockEpoch != "endpoint-clock" {
		t.Fatalf("receipt epoch %q", expired.Receipt.ClockEpoch)
	}
}

// The reader's clock epoch label means the reader's own monotonic origin. A
// record that carries the same label would make two origins comparable, so it
// is not taken: it has no effect on the published state or the retained
// payloads, and the entry reports it.
func TestRecordOnTheReadersOwnClockEpochIsRefused(t *testing.T) {
	reader, session, c, clk := startAfterFirstPoll(t)
	clk.set(time.Unix(105, 0))
	c.endpoint.push(step{echo: true, terminal: terminal(6, 28, readbackBody(scalars{limit: u(12), timeout: u(60), inhibit: b(false)})),
		wall: 100, epoch: "clock:synthetic", mono: 70 * time.Second})
	var record tesla.Gen3ReadRecord
	waitFor(t, "the readback", func() bool {
		var err error
		record, err = reader.Acquisition().Acquire(context.Background(), tesla.FC100OperationWCGetProvisional)
		return err == nil && record.State == tesla.Gen3ReadEvidenceAck
	})
	if err := reader.IngestPolled(context.Background(), record); !errors.Is(err, ErrSharedClockEpoch) {
		t.Fatalf("polled record on the reader's epoch: %v", err)
	}
	if p, _ := reader.Payload(SlotProvisionalReadback); p.Present {
		t.Fatalf("a refused record was retained: %+v", p)
	}
	if _, ok := reader.View().Fact(ElementAllocatedCurrent); ok {
		t.Fatal("a refused record changed the published state")
	}
	for _, batch := range session.committed() {
		for _, f := range batch.FactUpserts {
			if string(f.Key.FactID) == ElementAllocatedCurrent {
				t.Fatalf("a refused record published %s", f.Key.FactID)
			}
		}
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
	expired := wantDisposition(t, reader, ElementAllocatedCurrent, ReasonProvisionalExpired)
	committed := session.committed()
	last := committed[len(committed)-1]
	if len(last.FactWithdrawals) != 1 || last.FactWithdrawals[0] != first.CandidateID || len(last.FactUpserts) != 0 {
		t.Fatalf("the expiry batch withdraws %v and upserts %d, want only %s", last.FactWithdrawals, len(last.FactUpserts), first.CandidateID)
	}
	if got := wallNanos(t, last.ObservedAt.UnixNanoseconds) - wallNanos(t, first.Times.EvaluatedAt.UnixNanoseconds); got != 55*int64(time.Second) {
		t.Fatalf("the evaluation instant moved by %d ns, the clock by 55 s", got)
	}
	if expired.Receipt.ClockEpoch != "another-epoch" || expired.Receipt.Mono != 170*time.Second || !expired.Receipt.Wall.Equal(time.Unix(100, 0)) {
		t.Fatalf("the receipt changed: %+v", expired.Receipt)
	}
}
