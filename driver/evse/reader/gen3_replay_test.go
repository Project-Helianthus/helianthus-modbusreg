package reader

import (
	"testing"
	"time"
)

// Receipt inputs of the first characterized sequence: wall anchor 1_800_000_000 s; t8 (16 A) at +1 s;
// the mutation's set acknowledgement at +2 s and its t28 readback at +3 s
// (12 A, 600 s, inhibit false, configured 32 A); a later t8 (18 A) at +6 s.
// The expected values are the previously published values plus the listed
// differences: the t28 configured current is published on the EVSE and the
// later receipt wins.
var anchor = time.Unix(1_800_000_000, 0).UTC()

func sequenceReceipt(correlation uint64, wallOffset time.Duration) Receipt {
	return Receipt{ClockEpoch: "sequence", Wall: anchor.Add(wallOffset), Mono: time.Duration(correlation) * time.Second,
		Lifecycle: 1, Generation: 7, RequestID: correlation}
}

func TestSequenceTwoAndThreeKeepTheirReceipts(t *testing.T) {
	feed := NewFeed()
	feed.Persistent(PersistentObservation{MaxOutputCurrentAmps: 16, Receipt: sequenceReceipt(1, time.Second)})
	feed.Published()
	set, readback := sequenceReceipt(2, 2*time.Second), sequenceReceipt(3, 3*time.Second)
	feed.Provisional(ProvisionalObservation{
		Readback: readback0(scalars{limit: u(12), timeout: u(600), inhibit: b(false), configured: u(32)}),
		Receipt:  readback, Anchor: set,
	})
	sequence2 := feed.View(At{ClockEpoch: "sequence", Wall: anchor.Add(5 * time.Second), Mono: 3 * time.Second})
	wantAmps(t, sequence2, ElementConfiguredCurrent, 32)
	wantAmps(t, sequence2, ElementAllocatedCurrent, 12)
	if f, _ := sequence2.Fact(ElementAllocatedCurrent); f.Receipt.Wall.UnixNano() != 1800000002000000000 || f.Role != RoleConnector {
		t.Fatalf("allocation receipt %+v", f)
	}
	if f, _ := sequence2.Fact(ElementConfiguredCurrent); f.Receipt.Wall.UnixNano() != 1800000003000000000 || f.Role != RoleEVSE {
		t.Fatalf("configured receipt %+v", f)
	}
	if f, ok := sequence2.Fact(ElementAllocatedExpiry); !ok || f.Time.UnixNano() != 1800000602000000000 {
		t.Fatalf("expiry %+v ok=%v", f, ok)
	}
	original, _ := feed.LastProvisional()

	feed.Published()
	feed.Persistent(PersistentObservation{MaxOutputCurrentAmps: 18, Receipt: sequenceReceipt(4, 6*time.Second)})
	sequence3 := feed.View(At{ClockEpoch: "sequence", Wall: anchor.Add(8 * time.Second), Mono: 5 * time.Second})
	wantAmps(t, sequence3, ElementConfiguredCurrent, 18)
	if f, _ := sequence3.Fact(ElementConfiguredCurrent); f.Receipt.Wall.UnixNano() != 1800000006000000000 {
		t.Fatalf("configured receipt %+v", f)
	}
	wantNone(t, sequence3, ElementAllocatedCurrent)
	if sequence3.Reason(ElementAllocatedCurrent) != "withheld_provisional_missing" {
		t.Fatalf("view %+v", sequence3)
	}
	if retained, ok := feed.LastProvisional(); !ok || retained != original {
		t.Fatalf("the native t28 or its timing changed: %+v", retained)
	}
}
