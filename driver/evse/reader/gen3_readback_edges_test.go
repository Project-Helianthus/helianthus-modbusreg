package reader

import (
	"context"
	"sync"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
)

// A pause is followed by a readback that reports the inhibit flag false and
// carries no limit current.
//
// The TEDAPI Specification does not state this form. Its reply forms (section
// 6.4.11) always carry the limit in force, and a request that clears the flag
// is answered by a reply with no inhibit member at all (section 7.1.3), so an
// explicit false with no limit is not a stated reply and nothing says what it
// means for the allocation. The published behaviour is therefore the one the
// per-field presence rule gives, and this test records it: the explicit false
// supersedes the disabled supply, and the allocation stays as it was (the 0 A
// of the pause, republished by nothing), because no limit current was carried
// to replace it. The expiry the pause carried also stays.
func TestInhibitFalseReadbackWithoutALimitCurrentKeepsThePausedAllocation(t *testing.T) {
	reader, session, c, _ := startAfterFirstPoll(t)
	ingestReadback(t, reader, c, step{echo: true, wall: 100,
		terminal: terminal(6, 28, readbackBody(scalars{limit: u(16), timeout: u(900), inhibit: b(true), configured: u(32)}))})
	view := reader.View()
	wantAmps(t, view, ElementAllocatedCurrent, 0)
	if s, ok := view.Fact(ElementSupply); !ok || s.Symbol != SymbolSupplyDisabled {
		t.Fatalf("the pause did not publish the disabled supply: %+v", view)
	}
	published := len(session.committed())

	ingestReadback(t, reader, c, step{echo: true, wall: 200,
		terminal: terminal(6, 28, readbackBody(scalars{inhibit: b(false), configured: u(32)}))})
	view = reader.View()
	if _, ok := view.Fact(ElementSupply); ok {
		t.Fatalf("the explicit false did not supersede the disabled supply: %+v", view)
	}
	for _, f := range view.Facts {
		if f.Symbol != "" {
			t.Fatalf("an energized status was made up: %+v", f)
		}
	}
	wantAmps(t, view, ElementAllocatedCurrent, 0)
	if e, ok := view.Fact(ElementAllocatedExpiry); !ok || !e.Time.Equal(time.Unix(100+900, 0).UTC()) {
		t.Fatalf("the expiry did not stay: %+v ok=%v", e, ok)
	}

	batches := session.committed()
	if len(batches) != published+1 {
		t.Fatalf("%d batches, want %d", len(batches), published+1)
	}
	last := batches[len(batches)-1]
	if err := last.Validate(); err != nil {
		t.Fatal(err)
	}
	supply := reader.pub.candidateID(ElementSupply, RoleEVSE)
	if len(last.FactWithdrawals) != 1 || last.FactWithdrawals[0] != supply {
		t.Fatalf("withdrawals %v, want only the supply %s", last.FactWithdrawals, supply)
	}
	for _, f := range last.FactUpserts {
		if string(f.Key.FactID) == ElementAllocatedCurrent || string(f.Key.FactID) == ElementAllocatedExpiry {
			t.Fatalf("the allocation or its expiry was republished: %+v", f)
		}
	}
}

// restartCounter opens the first endpoint and then fails every open, so the
// lifecycle stays unavailable, and records when each open was asked for.
type restartCounter struct {
	mu    sync.Mutex
	opens []time.Time
}

func (r *restartCounter) record() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.opens = append(r.opens, time.Now())
	return len(r.opens)
}

func (r *restartCounter) times() []time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]time.Time(nil), r.opens...)
}

// A lifecycle that stays unavailable is restarted at most once per poll
// interval, as the native acquisition's own run loop does, and the state the
// reader published before stays published unchanged: nothing is republished,
// withdrawn or retimed while the lifecycle is down.
//
// The native acquisition policy is the intended one: the reader loop restarts
// once per interval when a cycle reports the lifecycle unavailable, and adds no
// backoff, count or policy of its own. The lifecycle is taken down by a reply
// that the transport accepts and the operation decoder rejects, which retires
// the endpoint; every later open then fails.
func TestUnavailableLifecycleIsRestartedOncePerIntervalAndPublishedStateStays(t *testing.T) {
	const interval = 30 * time.Millisecond
	const period = 400 * time.Millisecond
	cfg := syntheticConfig(tesla.FC100OperationWCGetVitals)
	cfg.Version, cfg.PollInterval = identifiedBuild, interval
	cfg.Windows[tesla.FC100OperationWCGetVitals] = tesla.Gen3ReadWindow{Freshness: time.Millisecond, Retention: 10 * time.Minute}

	counter := &restartCounter{}
	c := newCharger(t)
	c.deps.Open = func(config modbus.RTUProductionConfig) (tesla.Gen3ReadEndpoint, error) {
		if counter.record() > 1 {
			return nil, context.DeadlineExceeded
		}
		c.endpoint = &scriptedEndpoint{config: config, generation: 1}
		pushVitals(c.endpoint, vitalsReply(flagMember(3, true), flagMember(4, true)))
		pushVitals(c.endpoint, okBody)
		return c.endpoint, nil
	}
	reader, err := NewChargerReader(cfg, syntheticPublication(), c.deps)
	if err != nil {
		t.Fatal(err)
	}
	session := &fakeSession{}
	if err := reader.Start(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Stop(context.Background()) })
	waitFor(t, "the first publication", func() bool { return len(session.committed()) == 1 })
	published := reader.View()
	if f, ok := published.Fact(ElementConnection); !ok || f.Symbol != SymbolConnectionConnected {
		t.Fatalf("connection %+v ok=%v", f, ok)
	}

	started := time.Now()
	time.Sleep(period)
	opens := counter.times()
	restarts := len(opens) - 1
	if bound := int(time.Since(started)/interval) + 2; restarts < 2 || restarts > bound {
		t.Fatalf("%d restarts in about %v at an interval of %v, want between 2 and %d", restarts, period, interval, bound)
	}
	for i := 2; i < len(opens); i++ {
		if gap := opens[i].Sub(opens[i-1]); gap < interval/2 {
			t.Fatalf("restarts %d and %d are %v apart, under the interval %v", i-1, i, gap, interval)
		}
	}
	if state := reader.Acquisition().State(); state != tesla.Gen3ReadLifecycleUnavailable {
		t.Fatalf("lifecycle state %v", state)
	}
	if n := len(session.committed()); n != 1 {
		t.Fatalf("%d batches while the lifecycle was down, want the one published before", n)
	}
	got := reader.View()
	if f, ok := got.Fact(ElementConnection); !ok || f != mustFact(t, published, ElementConnection) || len(got.Facts) != len(published.Facts) || len(got.Dispositions) != 0 {
		t.Fatalf("the published state changed while the lifecycle was down: %+v", got)
	}
	if err := reader.PublicationError(); err != nil {
		t.Fatalf("publication error %v", err)
	}
}

func mustFact(t *testing.T, v View, element string) Fact {
	t.Helper()
	f, ok := v.Fact(element)
	if !ok {
		t.Fatalf("%s is not published", element)
	}
	return f
}

// Two persistent-settings readbacks of one generation are delivered through
// the running entry point out of their receipt order: the one with the older
// receipt never overwrites the newer one, and it publishes nothing.
func TestPersistentEntryDeliveredOutOfOrderKeepsTheNewerReceipt(t *testing.T) {
	for name, newerFirst := range map[string]bool{"newer first": true, "older first": false} {
		t.Run(name, func(t *testing.T) {
			cfg := mutationConfig()
			cfg.Version, cfg.PersistentSettingsQualified = "26.34.0", true
			reader, session, c, _ := runWith(t, cfg, syntheticPublication(), nil)
			waitFor(t, "the first poll", func() bool {
				p, _ := reader.Payload(SlotCommonSystemInfo)
				return p.Present
			})
			exchange := func(identity string, requested int32, returned byte) tesla.PersistentSettingsResult {
				c.endpoint.push(step{terminal: terminal(6, 8, []byte{0x0a, 0x02, 0x08, returned})})
				var result tesla.PersistentSettingsResult
				waitFor(t, "the persistent exchange", func() bool {
					var err error
					result, err = reader.Acquisition().ConfigurePersistentCurrent(context.Background(), tesla.PersistentSettingsRequest{
						MaxOutputCurrentAmps: requested, OperationIdentity: identity, Deadline: time.Now().Add(5 * time.Second)})
					return err == nil
				})
				return result
			}
			older := exchange("persistent-a", 20, 18)
			newer := exchange("persistent-b", 24, 22)
			if !receiptOf(newer.Receipt).After(receiptOf(older.Receipt)) || older.Receipt.Generation != newer.Receipt.Generation {
				t.Fatalf("the exchanges are not two receipts of one generation: %v %v", older.Receipt.ReceivedAt, newer.Receipt.ReceivedAt)
			}
			order := []tesla.PersistentSettingsResult{older, newer}
			if newerFirst {
				order = []tesla.PersistentSettingsResult{newer, older}
			}
			var published []int
			for _, result := range order {
				if err := reader.IngestPersistent(context.Background(), result); err != nil {
					t.Fatal(err)
				}
				published = append(published, len(session.committed()))
			}
			wantAmps(t, reader.View(), ElementConfiguredCurrent, 22)
			if newerFirst && published[1] != published[0] {
				t.Fatalf("the older readback published: %v", published)
			}
			f, _ := reader.View().Fact(ElementConfiguredCurrent)
			if f.Receipt.RequestID != newer.Receipt.RequestID {
				t.Fatalf("the fact carries receipt %d, want %d", f.Receipt.RequestID, newer.Receipt.RequestID)
			}
		})
	}
}
