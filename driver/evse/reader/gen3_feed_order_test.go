package reader

import (
	"testing"
	"time"
)

// A persistent-settings readback whose receipt is older than the one held does
// not overwrite it inside one generation, whatever order the two arrive in.
// The receipt order is the monotonic receipt on one clock epoch and the wall
// receipt across epochs; each case below changes only what orders the receipts.
// A production mutation that replaces the held readback on every arrival makes
// each case fail.
func TestFeedPersistentReadbackOlderReceiptNeverOverwritesNewer(t *testing.T) {
	older := PersistentObservation{MaxOutputCurrentAmps: 16, Receipt: rcpt(1, 2*time.Second)}
	newer := PersistentObservation{MaxOutputCurrentAmps: 20, Receipt: rcpt(2, 5*time.Second)}
	crossEpochOlder := older
	crossEpochOlder.Receipt.ClockEpoch = "another"
	crossEpochOlder.Receipt.Mono = 500 * time.Second
	disagree := older
	disagree.Receipt.Wall = feedBase.Add(90 * time.Second)

	for name, tc := range map[string]struct {
		older PersistentObservation
	}{
		"monotonic receipt on one epoch":                  {older},
		"wall receipt across clock epochs":                {crossEpochOlder},
		"monotonic receipt decides when the wall differs": {disagree},
	} {
		t.Run(name, func(t *testing.T) {
			for order, deliver := range map[string][]PersistentObservation{
				"newer first": {newer, tc.older}, "older first": {tc.older, newer}} {
				f := NewFeed()
				var taken []bool
				for _, o := range deliver {
					taken = append(taken, f.Persistent(o))
				}
				wantAmps(t, f.View(at(10*time.Second)), ElementConfiguredCurrent, 20)
				if order == "newer first" && (!taken[0] || taken[1]) {
					t.Fatalf("%s: the older readback was taken: %v", order, taken)
				}
			}
		})
	}
}

// An older persistent readback arriving after a publication must not withdraw
// the provisional allocation: it changed nothing.
func TestFeedOlderPersistentReadbackDoesNotWithdrawTheAllocation(t *testing.T) {
	f := NewFeed()
	f.Persistent(PersistentObservation{MaxOutputCurrentAmps: 20, Receipt: rcpt(2, 5*time.Second)})
	f.Provisional(standalone(3, 6*time.Second, scalars{limit: u(12), inhibit: b(false)}))
	f.Published()
	if f.Persistent(PersistentObservation{MaxOutputCurrentAmps: 16, Receipt: rcpt(1, 2*time.Second)}) {
		t.Fatal("the older readback was taken")
	}
	v := f.View(at(7 * time.Second))
	wantAmps(t, v, ElementAllocatedCurrent, 12)
	wantAmps(t, v, ElementConfiguredCurrent, 20)
}

// The same holds for the limit readback: each scalar is taken from the later
// readback, so an older one arriving late fills only what the newer one did not
// carry.
func TestFeedLimitReadbackOlderReceiptNeverOverwritesNewer(t *testing.T) {
	newer := standalone(2, 20*time.Second, scalars{limit: u(12), timeout: u(900), inhibit: b(false)})
	older := standalone(1, 10*time.Second, scalars{limit: u(16), timeout: u(60), inhibit: b(true), configured: u(32)})
	for order, deliver := range map[string][]ProvisionalObservation{"newer first": {newer, older}, "older first": {older, newer}} {
		f := NewFeed()
		for _, o := range deliver {
			if !f.Provisional(o) {
				t.Fatalf("%s: a readback of a new receipt was refused", order)
			}
		}
		v := f.View(at(30 * time.Second))
		wantAmps(t, v, ElementAllocatedCurrent, 12)
		wantNone(t, v, ElementSupply)
		if e, ok := v.Fact(ElementAllocatedExpiry); !ok || !e.Time.Equal(feedBase.Add(20*time.Second+900*time.Second)) {
			t.Fatalf("%s: expiry %+v ok=%v", order, e, ok)
		}
		wantAmps(t, v, ElementConfiguredCurrent, 32)
		if last, _ := f.LastProvisional(); last.Receipt.RequestID != 2 {
			t.Fatalf("%s: the last readback is %d", order, last.Receipt.RequestID)
		}
	}
}

// An older readback arriving after a persistent-only withdrawal does not
// bring the allocation back.
func TestFeedOlderLimitReadbackDoesNotLiftAWithdrawal(t *testing.T) {
	f := NewFeed()
	f.Provisional(standalone(1, 2*time.Second, scalars{limit: u(12), inhibit: b(false)}))
	f.Published()
	f.Persistent(PersistentObservation{MaxOutputCurrentAmps: 18, Receipt: rcpt(3, 8*time.Second)})
	f.Provisional(standalone(2, 5*time.Second, scalars{limit: u(16), inhibit: b(false)}))
	v := f.View(at(9 * time.Second))
	wantNone(t, v, ElementAllocatedCurrent)
	if v.Reason(ElementAllocatedCurrent) != ReasonProvisionalMissing {
		t.Fatalf("view %+v", v)
	}
	f.Provisional(standalone(4, 10*time.Second, scalars{limit: u(16), inhibit: b(false)}))
	wantAmps(t, f.View(at(11*time.Second)), ElementAllocatedCurrent, 16)
}

// The expiry of an earlier readback gates only the allocation that readback
// carried. A later allocation whose own readback carried no timeout is not
// hidden by it, and the earlier expiry fact stays published as it was.
//
// The later readback differs from the earlier one in exactly one field of the
// readback identity (lifecycle, endpoint instance, generation, request
// number), so a production mutation that leaves a field out of the identity
// comparison, or that gates every allocation on the held expiry, makes the
// matching case fail.
func TestFeedExpiryOfAnEarlierReadbackDoesNotHideALaterAllocation(t *testing.T) {
	first := rcpt(1, time.Second)
	for field, vary := range map[string]func(Receipt) Receipt{
		"lifecycle":         func(r Receipt) Receipt { r.Lifecycle++; r.RequestID = 1; return r },
		"endpoint instance": func(r Receipt) Receipt { r.Instance++; return r },
		"generation":        func(r Receipt) Receipt { r.Generation++; return r },
		"request number": func(r Receipt) Receipt {
			r.RequestID++
			r.Mono, r.Wall = 10*time.Second, feedBase.Add(10*time.Second)
			return r
		},
	} {
		t.Run(field, func(t *testing.T) {
			second := vary(first)
			second.Mono, second.Wall = 10*time.Second, feedBase.Add(10*time.Second)
			f := NewFeed()
			f.Provisional(ProvisionalObservation{Readback: readback(scalars{limit: u(16), timeout: u(60), inhibit: b(false)}), Receipt: first, Anchor: first})
			if v := f.View(at(30 * time.Second)); wantFact(t, v, ElementAllocatedCurrent) != 16 {
				t.Fatalf("before expiry: %+v", v)
			}
			if v := f.View(at(100 * time.Second)); v.Reason(ElementAllocatedCurrent) != ReasonProvisionalExpired {
				t.Fatalf("the allocation of the readback that carried the timeout is not withheld: %+v", v)
			}
			if !f.Provisional(ProvisionalObservation{Readback: readback(scalars{limit: u(12), inhibit: b(false)}), Receipt: second, Anchor: second}) {
				t.Fatal("the later readback was refused")
			}
			v := f.View(at(100 * time.Second))
			wantAmps(t, v, ElementAllocatedCurrent, 12)
			if e, ok := v.Fact(ElementAllocatedExpiry); !ok || !e.Time.Equal(feedBase.Add(time.Second+60*time.Second)) || e.Receipt != first {
				t.Fatalf("the earlier expiry did not stay as it was: %+v ok=%v", e, ok)
			}
			if v.Reason(ElementAllocatedCurrent) != "" {
				t.Fatalf("view %+v", v)
			}
		})
	}
}

// A later readback that carries its own timeout is gated by that timeout.
func TestFeedLaterReadbackIsGatedByItsOwnTimeout(t *testing.T) {
	f := NewFeed()
	f.Provisional(standalone(1, time.Second, scalars{limit: u(16), timeout: u(600), inhibit: b(false)}))
	f.Provisional(standalone(2, 10*time.Second, scalars{limit: u(12), timeout: u(30), inhibit: b(false)}))
	wantAmps(t, f.View(at(39*time.Second)), ElementAllocatedCurrent, 12)
	v := f.View(at(40 * time.Second))
	wantNone(t, v, ElementAllocatedCurrent)
	if v.Reason(ElementAllocatedCurrent) != ReasonProvisionalExpired {
		t.Fatalf("view %+v", v)
	}
}

// An allocation whose own lifetime has passed is never published as available
// again because a later readback carrying only a limit and a timeout replaced
// the held expiry: that readback carries no allocation, so it neither lifts
// nor replaces the lifetime of the earlier one. Pause, expiry, then the
// ordinary limit-and-timeout reply.
func TestFeedExpiredAllocationStaysWithheldAfterALaterTimeoutOnlyReadback(t *testing.T) {
	for name, first := range map[string]scalars{
		"pause":       {limit: u(16), timeout: u(60), inhibit: b(true)},
		"nonzero cap": {limit: u(32), timeout: u(60), inhibit: b(false)},
	} {
		t.Run(name, func(t *testing.T) {
			f := NewFeed()
			f.Provisional(standalone(1, 0, first))
			wantNone(t, f.View(at(70*time.Second)), ElementAllocatedCurrent)
			f.Provisional(standalone(2, 100*time.Second, scalars{limit: u(16), timeout: u(60)}))
			for _, when := range []time.Duration{110 * time.Second, 3600 * time.Second} {
				v := f.View(at(when))
				wantNone(t, v, ElementAllocatedCurrent)
				if v.Reason(ElementAllocatedCurrent) != ReasonProvisionalExpired {
					t.Fatalf("at %s: view %+v", when, v)
				}
				if _, ok := v.Fact(ElementAllocatedExpiry); !ok {
					t.Fatalf("at %s: the expiry fact is not published: %+v", when, v)
				}
			}
		})
	}
}

func wantFact(t *testing.T, v View, element string) uint32 {
	t.Helper()
	f, ok := v.Fact(element)
	if !ok {
		t.Fatalf("%s is not published: %+v", element, v)
	}
	return f.Amps
}
