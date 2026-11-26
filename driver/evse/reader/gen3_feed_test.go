package reader

import (
	"testing"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
)

var feedBase = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func rcpt(id uint64, offset time.Duration) Receipt {
	return Receipt{ClockEpoch: "synthetic", Wall: feedBase.Add(offset), Mono: offset, Lifecycle: 1, Generation: 1, RequestID: id}
}

func at(offset time.Duration) At {
	return At{ClockEpoch: "synthetic", Wall: feedBase.Add(offset), Mono: offset}
}

type scalars struct {
	limit, timeout, configured *uint32
	inhibit                    *bool
}

func u(v uint32) *uint32 { return &v }
func b(v bool) *bool     { return &v }

func readback(s scalars) tesla.ProvisionalCurrentLimitReadback {
	var r tesla.ProvisionalCurrentLimitReadback
	if s.limit != nil {
		r.LimitCurrentMaxAmps, r.HasLimitCurrent = *s.limit, true
	}
	if s.timeout != nil {
		r.LimitTimeoutSeconds, r.HasLimitTimeout = *s.timeout, true
	}
	if s.inhibit != nil {
		r.InhibitCharging, r.HasInhibit = *s.inhibit, true
	}
	if s.configured != nil {
		r.ConfiguredCurrentAmps, r.HasConfiguredCurrent = *s.configured, true
	}
	return r
}

func standalone(id uint64, offset time.Duration, s scalars) ProvisionalObservation {
	r := rcpt(id, offset)
	return ProvisionalObservation{Readback: readback(s), Receipt: r, Anchor: r}
}

func wantAmps(t *testing.T, v View, element string, want uint32) {
	t.Helper()
	f, ok := v.Fact(element)
	if !ok || f.Amps != want {
		t.Fatalf("%s: got %+v ok=%v, want %d A (view %+v)", element, f, ok, want, v)
	}
}

func wantNone(t *testing.T, v View, element string) {
	t.Helper()
	if f, ok := v.Fact(element); ok {
		t.Fatalf("%s published as %+v", element, f)
	}
}

func TestFeedKnownCurrentSurvivesZeroTimeout(t *testing.T) {
	f := NewFeed()
	f.Provisional(standalone(1, time.Second, scalars{limit: u(16), timeout: u(0), inhibit: b(false)}))
	v := f.View(at(2 * time.Second))
	wantAmps(t, v, ElementAllocatedCurrent, 16)
	wantNone(t, v, ElementAllocatedExpiry)
	if v.Reason(ElementAllocatedExpiry) != ReasonZeroTimeout {
		t.Fatalf("zero timeout not reported: %+v", v)
	}
}

func TestFeedZeroCurrentWithoutInhibitStaysUnknown(t *testing.T) {
	f := NewFeed()
	f.Provisional(standalone(1, time.Second, scalars{limit: u(0), timeout: u(900), inhibit: b(false)}))
	v := f.View(at(2 * time.Second))
	wantNone(t, v, ElementAllocatedCurrent)
	if v.Reason(ElementAllocatedCurrent) != ReasonZeroCurrent {
		t.Fatalf("view %+v", v)
	}
}

func TestFeedInhibitTruePublishesZeroAmpsAndDisabledSupply(t *testing.T) {
	f := NewFeed()
	f.Provisional(standalone(1, time.Second, scalars{limit: u(16), timeout: u(900), inhibit: b(true)}))
	v := f.View(at(2 * time.Second))
	wantAmps(t, v, ElementAllocatedCurrent, 0)
	if s, ok := v.Fact(ElementSupply); !ok || s.Symbol != SymbolSupplyDisabled || s.Role != RoleEVSE {
		t.Fatalf("supply %+v ok=%v", s, ok)
	}
	if e, ok := v.Fact(ElementAllocatedExpiry); !ok || !e.Time.Equal(feedBase.Add(time.Second+900*time.Second)) || e.Role != RoleConnector {
		t.Fatalf("expiry %+v ok=%v", e, ok)
	}
}

func TestFeedInhibitFalseSupersedesDisabledWithoutEnergizedStatus(t *testing.T) {
	f := NewFeed()
	f.Provisional(standalone(1, time.Second, scalars{inhibit: b(true)}))
	if _, ok := f.View(at(2 * time.Second)).Fact(ElementSupply); !ok {
		t.Fatal("disabled supply not published")
	}
	f.Provisional(standalone(2, 3*time.Second, scalars{inhibit: b(false)}))
	v := f.View(at(4 * time.Second))
	wantNone(t, v, ElementSupply)
	for _, fact := range v.Facts {
		if fact.Symbol != "" {
			t.Fatalf("a symbol was fabricated: %+v", fact)
		}
	}
}

func TestFeedOmittedInhibitDoesNotClearOrCreateSupply(t *testing.T) {
	f := NewFeed()
	f.Provisional(standalone(1, time.Second, scalars{inhibit: b(true)}))
	f.Provisional(standalone(2, 3*time.Second, scalars{limit: u(16), configured: u(32)}))
	v := f.View(at(4 * time.Second))
	if _, ok := v.Fact(ElementSupply); !ok {
		t.Fatal("an omitted inhibit cleared the disabled supply")
	}
	g := NewFeed()
	g.Provisional(standalone(1, time.Second, scalars{limit: u(16), configured: u(32), timeout: u(900)}))
	w := g.View(at(2 * time.Second))
	wantAmps(t, w, ElementConfiguredCurrent, 32)
	wantNone(t, w, ElementAllocatedCurrent)
	wantNone(t, w, ElementSupply)
	if e, ok := w.Fact(ElementAllocatedExpiry); !ok || !e.Time.Equal(feedBase.Add(time.Second+900*time.Second)) {
		t.Fatalf("expiry %+v ok=%v", e, ok)
	}
}

func TestFeedAbsentLimitIsNotZeroAndAbsentTimeoutIsNoExpiry(t *testing.T) {
	f := NewFeed()
	f.Provisional(standalone(1, time.Second, scalars{inhibit: b(false)}))
	v := f.View(at(2 * time.Second))
	wantNone(t, v, ElementAllocatedCurrent)
	wantNone(t, v, ElementAllocatedExpiry)
	if v.Reason(ElementAllocatedCurrent) != "" {
		t.Fatalf("disposition %+v", v)
	}
	f.Provisional(standalone(2, 3*time.Second, scalars{limit: u(16), inhibit: b(false)}))
	f.Provisional(standalone(3, 4*time.Second, scalars{inhibit: b(false)}))
	wantAmps(t, f.View(at(5*time.Second)), ElementAllocatedCurrent, 16)
}

func TestFeedConfiguredCurrentPrecedence(t *testing.T) {
	f := NewFeed()
	f.Persistent(PersistentObservation{MaxOutputCurrentAmps: 32, Receipt: rcpt(1, time.Second)})
	f.Provisional(standalone(2, 2*time.Second, scalars{limit: u(12), inhibit: b(false)}))
	wantAmps(t, f.View(at(3*time.Second)), ElementConfiguredCurrent, 32)
	f.Provisional(standalone(3, 4*time.Second, scalars{configured: u(16)}))
	wantAmps(t, f.View(at(5*time.Second)), ElementConfiguredCurrent, 16)
	f.Persistent(PersistentObservation{MaxOutputCurrentAmps: 18, Receipt: rcpt(4, 6*time.Second)})
	wantAmps(t, f.View(at(7*time.Second)), ElementConfiguredCurrent, 18)
}

func TestFeedExplicitZeroConfiguredIsUnknownNotFallback(t *testing.T) {
	f := NewFeed()
	f.Persistent(PersistentObservation{MaxOutputCurrentAmps: 32, Receipt: rcpt(1, time.Second)})
	f.Provisional(standalone(2, 2*time.Second, scalars{configured: u(0)}))
	v := f.View(at(3 * time.Second))
	wantNone(t, v, ElementConfiguredCurrent)
	if v.Reason(ElementConfiguredCurrent) != ReasonZeroCurrent {
		t.Fatalf("view %+v", v)
	}
	f.Persistent(PersistentObservation{MaxOutputCurrentAmps: 18, Receipt: rcpt(3, 4*time.Second)})
	wantAmps(t, f.View(at(5*time.Second)), ElementConfiguredCurrent, 18)

	g := NewFeed()
	g.Persistent(PersistentObservation{MaxOutputCurrentAmps: 32, Receipt: rcpt(1, time.Second)})
	g.Provisional(standalone(2, 2*time.Second, scalars{limit: u(12), inhibit: b(false)}))
	wantAmps(t, g.View(at(3*time.Second)), ElementConfiguredCurrent, 32)
}

func TestFeedMutationExpiryIsAnchoredOnTheSetAcknowledgement(t *testing.T) {
	f := NewFeed()
	set := rcpt(1, 10*time.Second)
	readback := rcpt(2, 80*time.Second)
	f.Provisional(ProvisionalObservation{Readback: readback0(scalars{limit: u(12), timeout: u(60), inhibit: b(false)}), Receipt: readback, Anchor: set})
	expiry := feedBase.Add(70 * time.Second)
	for name, tc := range map[string]struct {
		offset time.Duration
		held   bool
	}{"before": {69 * time.Second, false}, "at": {70 * time.Second, true}, "after": {80 * time.Second, true}} {
		v := f.View(at(tc.offset))
		if e, ok := v.Fact(ElementAllocatedExpiry); !ok || !e.Time.Equal(expiry) {
			t.Fatalf("%s: expiry %+v ok=%v, want the set receipt plus 60 s", name, e, ok)
		}
		if tc.held {
			wantNone(t, v, ElementAllocatedCurrent)
			if v.Reason(ElementAllocatedCurrent) != ReasonProvisionalExpired {
				t.Fatalf("%s: %+v", name, v)
			}
		} else {
			wantAmps(t, v, ElementAllocatedCurrent, 12)
		}
	}
}

func readback0(s scalars) tesla.ProvisionalCurrentLimitReadback { return readback(s) }

func TestFeedStandaloneReadbackUsesItsOwnReceiptNotTheSetAcknowledgement(t *testing.T) {
	f := NewFeed()
	f.Provisional(standalone(2, 80*time.Second, scalars{limit: u(12), timeout: u(60), inhibit: b(false)}))
	v := f.View(at(100 * time.Second))
	wantAmps(t, v, ElementAllocatedCurrent, 12)
	if e, _ := v.Fact(ElementAllocatedExpiry); !e.Time.Equal(feedBase.Add(140 * time.Second)) {
		t.Fatalf("expiry %v", e.Time)
	}
	wantNone(t, f.View(at(140*time.Second)), ElementAllocatedCurrent)
}

func TestFeedDuplicatesAndObsoleteGenerationsHaveNoEffect(t *testing.T) {
	f := NewFeed()
	first := standalone(1, time.Second, scalars{limit: u(16), inhibit: b(false)})
	if !f.Provisional(first) || f.Provisional(first) {
		t.Fatal("duplicate accepted or first refused")
	}
	older := standalone(9, 2*time.Second, scalars{limit: u(6), inhibit: b(false)})
	older.Receipt.Generation, older.Anchor.Generation = 0, 0
	if f.Provisional(older) {
		t.Fatal("an obsolete generation was accepted")
	}
	wantAmps(t, f.View(at(3*time.Second)), ElementAllocatedCurrent, 16)
	if !f.Persistent(PersistentObservation{MaxOutputCurrentAmps: 18, Receipt: rcpt(5, 4*time.Second)}) || f.Persistent(PersistentObservation{MaxOutputCurrentAmps: 18, Receipt: rcpt(5, 4*time.Second)}) {
		t.Fatal("persistent duplicate handling")
	}
}

func TestFeedPersistentOnlyPublicationWithdrawsTheProvisionalAllocation(t *testing.T) {
	f := NewFeed()
	f.Persistent(PersistentObservation{MaxOutputCurrentAmps: 32, Receipt: rcpt(1, time.Second)})
	f.Provisional(standalone(2, 2*time.Second, scalars{limit: u(12), timeout: u(600), inhibit: b(false)}))
	wantAmps(t, f.View(at(3*time.Second)), ElementAllocatedCurrent, 12)
	f.Published()
	f.Persistent(PersistentObservation{MaxOutputCurrentAmps: 18, Receipt: rcpt(3, 4*time.Second)})
	v := f.View(at(5 * time.Second))
	wantAmps(t, v, ElementConfiguredCurrent, 18)
	wantNone(t, v, ElementAllocatedCurrent)
	if v.Reason(ElementAllocatedCurrent) != ReasonProvisionalMissing {
		t.Fatalf("view %+v", v)
	}
}

func TestFeedPersistentOnlyPublicationKeepsInhibitSupplyAndExpiry(t *testing.T) {
	f := NewFeed()
	f.Provisional(standalone(1, time.Second, scalars{limit: u(16), timeout: u(600), inhibit: b(true)}))
	v := f.View(at(2 * time.Second))
	if _, ok := v.Fact(ElementSupply); !ok {
		t.Fatalf("supply before the t8: %+v", v)
	}
	f.Published()
	f.Persistent(PersistentObservation{MaxOutputCurrentAmps: 18, Receipt: rcpt(2, 3*time.Second)})
	v = f.View(at(4 * time.Second))
	wantAmps(t, v, ElementConfiguredCurrent, 18)
	wantNone(t, v, ElementAllocatedCurrent)
	if v.Reason(ElementAllocatedCurrent) != ReasonProvisionalMissing {
		t.Fatalf("view %+v", v)
	}
	if s, ok := v.Fact(ElementSupply); !ok || s.Symbol != SymbolSupplyDisabled {
		t.Fatalf("a persistent t8 carries no inhibit flag and must not clear the disabled supply: %+v", v)
	}
	if e, ok := v.Fact(ElementAllocatedExpiry); !ok || !e.Time.Equal(feedBase.Add(time.Second+600*time.Second)) {
		t.Fatalf("a persistent t8 must keep the expiry fact on its original anchor: %+v", v)
	}
}

func TestFeedProvisionalBeforeFirstPublicationIsKept(t *testing.T) {
	f := NewFeed()
	f.Provisional(standalone(1, time.Second, scalars{limit: u(12), timeout: u(600), inhibit: b(false)}))
	f.Persistent(PersistentObservation{MaxOutputCurrentAmps: 32, Receipt: rcpt(2, 2*time.Second)})
	v := f.View(at(3 * time.Second))
	wantAmps(t, v, ElementConfiguredCurrent, 32)
	wantAmps(t, v, ElementAllocatedCurrent, 12)
}
