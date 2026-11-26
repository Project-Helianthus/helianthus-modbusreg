package reader

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

// syntheticSources is the source descriptor a host declares for the synthetic
// charger; its shape is the descriptor of the gateway's first characterization.
func syntheticSources(t *testing.T, pub ChargerPublication) []semreg.SourceDescriptor {
	t.Helper()
	var golden semreg.Snapshot
	if err := json.Unmarshal(readGolden(t, "sequence2.canonical.json"), &golden); err != nil {
		t.Fatal(err)
	}
	sources := append([]semreg.SourceDescriptor{}, golden.Sources...)
	sources[0].SourceID, sources[0].SourceEpochID = pub.Source, pub.Epoch
	return sources
}

func expiryIn(t *testing.T, cut replayCut) (time.Time, bool) {
	t.Helper()
	for _, c := range candidateByID(cut.snapshot) {
		if string(c.Key.FactID) == ElementAllocatedExpiry {
			ns, err := strconv.ParseInt(string(c.Value.Time.UnixNanoseconds), 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			return time.Unix(0, ns).UTC(), true
		}
	}
	return time.Time{}, false
}

func allocatedIn(cut replayCut) (semreg.FactCandidate, bool) {
	for _, c := range candidateByID(cut.snapshot) {
		if string(c.Key.FactID) == ElementAllocatedCurrent {
			return c, true
		}
	}
	return semreg.FactCandidate{}, false
}

// The mutation's lifetime runs from the correlated set acknowledgement. For a
// 60 s and a 70 s native timeout the committed batches are applied to a kernel
// carrying the EVSE pack one second before, at and one second after the expiry:
// the expiry fact is the same instant throughout and the allocation is present
// only before it.
func TestSetAnchoredExpiryPublishedToTheKernelBeforeAtAndAfterExpiry(t *testing.T) {
	const setWall = 1000
	for _, timeout := range []uint32{60, 70} {
		for name, offset := range map[string]time.Duration{"before": -time.Second, "at": 0, "after": time.Second} {
			t.Run(strconv.Itoa(int(timeout))+"s "+name+" expiry", func(t *testing.T) {
				reader, session, c, clk := startMutation(t)
				pushMutation(c, scalars{limit: u(6), timeout: u(900), inhibit: b(false), configured: u(32)},
					scalars{limit: u(12), timeout: u(timeout), inhibit: b(false), configured: u(32)}, setWall, setWall+1)
				result := settle(t, reader, tesla.ProvisionalCurrentLimitRequest{LimitCurrentMaxAmps: 16, LimitTimeoutSeconds: 900})
				clk.set(time.Unix(setWall, 0).Add(time.Duration(timeout)*time.Second + offset))
				if err := reader.IngestSetResult(context.Background(), result); err != nil {
					t.Fatal(err)
				}
				cut := applyToKernel(t, syntheticPublication().Asset, syntheticSources(t, syntheticPublication()), session, reader)
				expiry, ok := expiryIn(t, cut)
				if want := time.Unix(setWall+int64(timeout), 0).UTC(); !ok || !expiry.Equal(want) {
					t.Fatalf("expiry %v (published %v), want the set acknowledgement plus %d s = %v", expiry, ok, timeout, want)
				}
				allocated, held := allocatedIn(cut)
				if offset < 0 {
					if !held || allocated.Value == nil || allocated.Value.Quantity.Unit != "unit.ampere" || allocated.Value.Quantity.Number.Coefficient != "12" {
						t.Fatalf("the allocation is not published before expiry: %+v", allocated)
					}
					return
				}
				if held || cut.dispositions[ElementAllocatedCurrent] != ReasonProvisionalExpired {
					t.Fatalf("the allocation is not withheld at or after expiry: %+v, %v", allocated, cut.dispositions)
				}
			})
		}
	}
}

// A t28 that reaches the owner 70 s after the set acknowledgement, with a 60 s
// timeout, is withheld in the first batch that carries it and its expiry is the
// set acknowledgement plus 60 s.
func TestReadbackSeventySecondsAfterTheSetIsAlreadyWithheld(t *testing.T) {
	const setWall = 1000
	reader, session, c, clk := startMutation(t)
	pushMutation(c, scalars{limit: u(6), timeout: u(900), inhibit: b(false), configured: u(32)},
		scalars{limit: u(12), timeout: u(60), inhibit: b(false), configured: u(32)}, setWall, setWall+70)
	result := settle(t, reader, tesla.ProvisionalCurrentLimitRequest{LimitCurrentMaxAmps: 16, LimitTimeoutSeconds: 900})
	clk.set(time.Unix(setWall+70, 0))
	if err := reader.IngestSetResult(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	cut := applyToKernel(t, syntheticPublication().Asset, syntheticSources(t, syntheticPublication()), session, reader)
	if expiry, ok := expiryIn(t, cut); !ok || !expiry.Equal(time.Unix(setWall+60, 0).UTC()) {
		t.Fatalf("expiry %v (published %v)", expiry, ok)
	}
	if allocated, held := allocatedIn(cut); held || cut.dispositions[ElementAllocatedCurrent] != ReasonProvisionalExpired {
		t.Fatalf("the allocation is published although the lifetime ended before the readback arrived: %+v", allocated)
	}
}

// A standalone readback anchors its expiry on its own receipt, whatever a
// mutation did before it: the same 60 s and 70 s timeouts, one second before,
// at and after.
func TestStandaloneReadbackExpiryIsAnchoredOnItsOwnReceiptInTheKernel(t *testing.T) {
	const receiptWall = 2000
	for _, timeout := range []uint32{60, 70} {
		for name, offset := range map[string]time.Duration{"before": -time.Second, "at": 0, "after": time.Second} {
			t.Run(strconv.Itoa(int(timeout))+"s "+name+" expiry", func(t *testing.T) {
				reader, session, c, clk := startAfterFirstPoll(t)
				clk.set(time.Unix(receiptWall, 0).Add(time.Duration(timeout)*time.Second + offset))
				ingestReadback(t, reader, c, step{echo: true, wall: receiptWall, epoch: "endpoint-clock", mono: 2 * time.Second,
					terminal: terminal(6, 28, readbackBody(scalars{limit: u(12), timeout: u(timeout), inhibit: b(false)}))})
				cut := applyToKernel(t, syntheticPublication().Asset, syntheticSources(t, syntheticPublication()), session, reader)
				if expiry, ok := expiryIn(t, cut); !ok || !expiry.Equal(time.Unix(receiptWall+int64(timeout), 0).UTC()) {
					t.Fatalf("expiry %v (published %v), want its own receipt plus %d s", expiry, ok, timeout)
				}
				allocated, held := allocatedIn(cut)
				if (offset < 0) != held {
					t.Fatalf("allocation published %v at offset %v: %+v %v", held, offset, allocated, cut.dispositions)
				}
			})
		}
	}
}

// Ampere facts carry unit.ampere and the whole-ampere value the readback
// returned, in the form the kernel accepts for every value including those
// that end in a zero.
func TestAmpereFactsOfEveryWholeValueAreAcceptedByTheKernel(t *testing.T) {
	for _, amps := range []uint32{0, 6, 10, 16, 20, 32, 100} {
		t.Run(strconv.Itoa(int(amps))+" A", func(t *testing.T) {
			reader, session, c, _ := startAfterFirstPoll(t)
			ingestReadback(t, reader, c, step{echo: true, wall: 2000, epoch: "endpoint-clock", mono: 2 * time.Second,
				terminal: terminal(6, 28, readbackBody(scalars{limit: u(amps), timeout: u(600), inhibit: b(amps == 0), configured: u(amps + 10)}))})
			cut := applyToKernel(t, syntheticPublication().Asset, syntheticSources(t, syntheticPublication()), session, reader)
			for element, want := range map[string]uint32{ElementAllocatedCurrent: amps, ElementConfiguredCurrent: amps + 10} {
				var got *semreg.Quantity
				for _, c := range candidateByID(cut.snapshot) {
					if string(c.Key.FactID) == element {
						got = c.Value.Quantity
					}
				}
				if got == nil || got.Unit != "unit.ampere" {
					t.Fatalf("%s: %+v", element, got)
				}
				value, err := strconv.ParseFloat(got.Number.Coefficient, 64)
				if err != nil {
					t.Fatal(err)
				}
				for i := int32(0); i < got.Number.Exponent10; i++ {
					value *= 10
				}
				if uint32(value) != want {
					t.Fatalf("%s is %v A, want %d A", element, value, want)
				}
			}
		})
	}
}
