package modbusreg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func h06FixtureCycle(t testing.TB, name, cycle string) (h06Fixture, h06Cycle) {
	t.Helper()
	fixture, _ := h06Load(t, name)
	for _, candidate := range fixture.Cycles {
		if candidate.Name == cycle {
			return fixture, candidate
		}
	}
	t.Fatalf("%s has no cycle %s", name, cycle)
	return fixture, h06Cycle{}
}

func TestH06SmartLoggerReconnectIdentifiesAgainAndKeepsRetainedHistory(t *testing.T) {
	fixture, start := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "start_identification")
	rig := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
	first := rig.refresh(t, h06Script(t, rig.sources, fixture.Path, start)...)

	// Stop closes the connection; a refresh then has no session.
	if err := rig.provider.Stop(); err != nil {
		t.Fatal(err)
	}
	if rig.session.Closes() != 1 {
		t.Fatalf("stop closed the session %d times", rig.session.Closes())
	}
	if _, err := rig.provider.Refresh(context.Background()); !errors.Is(err, ErrHuaweiProviderNotStarted) {
		t.Fatalf("refresh without a session: %v", err)
	}
	second := newH06Session("generation-2", rig.clock, rig.recorder)
	if err := rig.provider.Start(second); err != nil {
		t.Fatal(err)
	}
	if err := rig.provider.Start(second); !errors.Is(err, ErrHuaweiProviderStarted) {
		t.Fatalf("second start: %v", err)
	}
	// On the new generation the guard alone is not enough: the whole
	// identification runs again before any child group is read.
	rig.session = second
	cycle := rig.refresh(t, h06Script(t, rig.sources, fixture.Path, start)...)
	if cycle.Generation != first.Generation+2 || cycle.Reason != HuaweiProviderReasonIdentified || cycle.Gateway.Generation != cycle.Generation {
		t.Fatalf("generation %d reason %s gateway %+v", cycle.Generation, cycle.Reason, cycle.Gateway)
	}
	child, _ := cycle.Child(1)
	if child.IdentifiedGeneration != cycle.Generation {
		t.Fatalf("child identified on generation %d", child.IdentifiedGeneration)
	}
	for _, group := range child.Groups {
		if group.Generation != cycle.Generation || group.Retained {
			t.Fatalf("group %d after the new identification: %+v", group.Offset, group)
		}
	}

	// Reconnect closes the bound connection first and starts another generation.
	third := newH06Session("generation-3", rig.clock, rig.recorder)
	if err := rig.provider.Reconnect(third); err != nil {
		t.Fatal(err)
	}
	if second.Closes() != 1 || third.Closes() != 0 {
		t.Fatalf("reconnect closes: second=%d third=%d", second.Closes(), third.Closes())
	}
	// A failed gateway read on the new generation keeps the retained readings
	// as stale history with their original generation and age.
	rig.session = third
	failed := rig.refresh(t,
		h06FC03(0, huaweiSmartLoggerCounterOffset, 1, h06Answer{err: h06Timeout()}),
		h06FC03(0, huaweiSmartLoggerCounterOffset, 1, h06Answer{err: h06Timeout()}),
		h06FC03(0, huaweiSmartLoggerCounterOffset, 1, h06Answer{err: h06Timeout()}),
		h06FC03(0, huaweiSmartLoggerCounterOffset, 1, h06Answer{err: h06Timeout()}),
	)
	if failed.Outcome != HuaweiProviderCycleInsufficient || failed.Reason != HuaweiProviderReasonGatewayReadFail {
		t.Fatalf("failed cycle: %s %s", failed.Outcome, failed.Reason)
	}
	stale, _ := failed.Child(1)
	if stale.Evidence != HuaweiProviderEvidenceStale {
		t.Fatalf("child after the failed gateway read: %+v", stale)
	}
	for _, group := range stale.Groups {
		if !group.Retained || group.Generation != cycle.Generation || group.Cycle != cycle.Cycle {
			t.Fatalf("retained group %d: %+v", group.Offset, group)
		}
	}
	// The endpoint backs off for at least one cycle after a gateway that stops
	// answering, doubling up to ten cycles.
	if wait := rig.provider.NextRefreshAt().Sub(failed.StartedAt); wait < 2*time.Minute {
		t.Fatalf("endpoint back-off %s", wait)
	}
}

func TestH06SmartLoggerGatewayMismatchWithholdsTheInstance(t *testing.T) {
	sources := h06LoadSources(t)
	for name, mutate := range map[string]func(*h03InventoryFixture){
		"firmware": func(f *h03InventoryFixture) {
			object := &f.Pages[1].Objects[0]
			object.Entry = strings.Replace(object.Entry, "V300R023C10SPC620", "V300R023C10SPC621", 1)
		},
		"self model": func(f *h03InventoryFixture) {
			object := &f.Pages[1].Objects[0]
			object.Entry = strings.Replace(object.Entry, "1=Smart Logger", "1=Smart Loggex", 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			rig := newH06Rig(t, "SmartLogger", []uint8{1, 2})
			// First establish the instance with the observed tuple.
			_, start := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "start_identification")
			rig.refresh(t, h06Script(t, rig.sources, "SmartLogger", start)...)
			// The counter moves and the next enumeration lists another tuple.
			rig.refresh(t, h06FC03(0, huaweiSmartLoggerCounterOffset, 1, h06Answer{words: []uint16{8}}))
			mutated := sources
			inventory := sources.smartLoggerInventory
			inventory.Pages = append([]h03InventoryPage(nil), inventory.Pages...)
			for index := range inventory.Pages {
				inventory.Pages[index].Objects = append([]h03Object(nil), inventory.Pages[index].Objects...)
			}
			mutate(&inventory)
			mutated.smartLoggerInventory = inventory
			var script []h06Expected
			for _, step := range h06IdentificationSteps() {
				script = append(script, h06Script(t, mutated, "SmartLogger", h06Cycle{Steps: []h06Step{step}})...)
			}
			cycle := rig.refresh(t, script...)
			if cycle.Outcome != HuaweiProviderCycleInsufficient || cycle.Reason != HuaweiProviderReasonGatewayMismatch ||
				cycle.Gateway.State != HuaweiProviderGatewayInsufficient {
				t.Fatalf("mismatch cycle: %s %s %+v", cycle.Outcome, cycle.Reason, cycle.Gateway)
			}
			for _, child := range cycle.Children {
				if child.Evidence != HuaweiProviderEvidenceWithheld || len(child.Reading.Facts()) != 0 {
					t.Fatalf("child %d emits facts under a gateway mismatch: %+v", child.UnitID, child)
				}
			}
			for _, exchange := range cycle.Exchanges {
				if exchange.UnitID != 0 {
					t.Fatalf("a child was read under a gateway mismatch: %+v", exchange)
				}
			}
		})
	}
	// An EMMA at the endpoint refuses the counter with exception 3: one
	// definitive reply, no repeat, no further read.
	rig := newH06Rig(t, "SmartLogger", []uint8{1})
	cycle := rig.refresh(t, h06FC03(0, huaweiSmartLoggerCounterOffset, 1, h06Answer{err: h06Exception(0x03, 3)}))
	if cycle.Outcome != HuaweiProviderCycleInsufficient || cycle.Reason != HuaweiProviderReasonGatewayReadFail || len(cycle.Exchanges) != 1 {
		t.Fatalf("EMMA-like endpoint: %s %s %d exchanges", cycle.Outcome, cycle.Reason, len(cycle.Exchanges))
	}
}

func TestH06SmartLoggerChildIdentityChangeFencesItsReading(t *testing.T) {
	fixture, start := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "start_identification")
	rig := newH06Rig(t, fixture.Path, []uint8{1})
	first := rig.refresh(t, h06Script(t, rig.sources, fixture.Path, h06Cycle{Steps: h06StepsForUnits(start.Steps, 1)})...)
	if child, _ := first.Child(1); child.Evidence != HuaweiProviderEvidenceCurrent {
		t.Fatalf("first: %+v", child)
	}
	// The counter changes; the re-enumeration finds the same listing, but the
	// routed model read at unit 1 now disagrees with the listing.
	rig.refresh(t, h06FC03(0, huaweiSmartLoggerCounterOffset, 1, h06Answer{words: []uint16{8}}))
	var script []h06Expected
	for _, step := range h06IdentificationSteps() {
		script = append(script, h06Script(t, rig.sources, fixture.Path, h06Cycle{Steps: []h06Step{step}})...)
	}
	model := make([]uint16, 15)
	copy(model, []uint16{0x5355, 0x4e32, 0x3030, 0x302d, 0x4f54, 0x4845, 0x522d, 0x5800})
	script = append(script, h06FC03(1, 30000, 15, h06Answer{words: model}))
	cycle := rig.refresh(t, script...)
	child, _ := cycle.Child(1)
	if child.Evidence != HuaweiProviderEvidenceWithheld || child.Reason != HuaweiProviderReasonIdentityMismatch || len(child.Reading.Facts()) != 0 ||
		child.Catalogue != nil {
		t.Fatalf("mismatched child: %+v", child)
	}
	if child.Fenced == nil || child.Fenced.ProfileID != HuaweiSUN2000M3MonitoringProfileID {
		t.Fatalf("the earlier reading is not kept as fenced history: %+v", child.Fenced)
	}
	before, _ := first.Child(1)
	for _, group := range child.Fenced.Groups {
		if group.Cycle != first.Cycle || group.Generation != first.Generation {
			t.Fatalf("fenced group %d lost its age: %+v", group.Offset, group)
		}
	}
	for _, fact := range child.Fenced.Reading.Facts() {
		if earlier, _ := before.Reading.Fact(fact.Key, fact.Index); earlier.Value != fact.Value {
			t.Fatalf("fenced fact %s changed", fact.Key)
		}
	}
	if cycle.Outcome != HuaweiProviderCycleComplete {
		t.Fatalf("a child mismatch failed the cycle: %s", cycle.Outcome)
	}
}

// h06StepsForUnits keeps the gateway steps and the steps of the named units.
func h06StepsForUnits(steps []h06Step, units ...uint8) []h06Step {
	keep := map[uint8]bool{0: true}
	for _, unit := range units {
		keep[unit] = true
	}
	var out []h06Step
	for _, step := range steps {
		if keep[step.Unit] {
			out = append(out, step)
		}
	}
	return out
}

func TestH06SmartLoggerEmptyAllowlistReadsNoChild(t *testing.T) {
	rig := newH06Rig(t, "SmartLogger", nil)
	var script []h06Expected
	for _, step := range h06IdentificationSteps() {
		script = append(script, h06Script(t, rig.sources, "SmartLogger", h06Cycle{Steps: []h06Step{step}})...)
	}
	cycle := rig.refresh(t, script...)
	if cycle.Outcome != HuaweiProviderCycleComplete || len(cycle.Children) != 11 {
		t.Fatalf("empty allowlist: %s %v", cycle.Outcome, h06ChildSummary(cycle))
	}
	for _, child := range cycle.Children {
		if child.Evidence != HuaweiProviderEvidenceWithheld || child.Reason != HuaweiProviderReasonNotAllowlisted {
			t.Fatalf("child %d: %s %s", child.UnitID, child.Evidence, child.Reason)
		}
	}
}

func TestH06SDongleFirstRequestTimeoutIsATimingClass(t *testing.T) {
	_, cycles := h06RunFixture(t, "sdongle_provider_cycles.json")
	first := cycles[0]
	if len(first.Exchanges) < 2 {
		t.Fatal("no exchanges")
	}
	tolerated, repeat := first.Exchanges[0], first.Exchanges[1]
	if tolerated.UnitID != 100 || tolerated.Offset != huaweiSDongleSearchOffset || tolerated.Outcome != HuaweiProviderExchangeTimeout ||
		tolerated.Elapsed != HuaweiProviderFirstRequestTolerated || tolerated.Attempt != 1 {
		t.Fatalf("first exchange: %+v", tolerated)
	}
	if repeat.Offset != huaweiSDongleSearchOffset || repeat.Outcome != HuaweiProviderExchangeAnswered || repeat.Attempt != 1 ||
		repeat.Elapsed != HuaweiProviderWithinPromptBound {
		t.Fatalf("repeat: %+v", repeat)
	}
	basic := first.Exchanges[2]
	if basic.UnitID != 100 || basic.Function != 0x2b || basic.Outcome != HuaweiProviderExchangeException || basic.ExceptionCode != 1 || basic.Attempt != 1 {
		t.Fatalf("basic identification at the gateway unit: %+v", basic)
	}
	for index, exchange := range first.Exchanges[3:] {
		if exchange.Elapsed == HuaweiProviderFirstRequestTolerated || exchange.UnitID == 0 {
			t.Fatalf("exchange %d: %+v", index+3, exchange)
		}
	}
	// A later timeout of the same request on the same generation is a failed
	// attempt, not a tolerated one.
	for _, cycle := range cycles[1:] {
		for _, exchange := range cycle.Exchanges {
			if exchange.Elapsed == HuaweiProviderFirstRequestTolerated {
				t.Fatalf("cycle %d tolerated a timeout that is not the first request: %+v", cycle.Cycle, exchange)
			}
		}
	}
	// After a reconnect the first request is tolerated once more.
	fixture, start := h06FixtureCycle(t, "sdongle_provider_cycles.json", "start_identification")
	rig := newH06Rig(t, fixture.Path, []uint8{1})
	rig.refresh(t, h06Script(t, rig.sources, fixture.Path, start)...)
	next := newH06Session("generation-2", rig.clock, rig.recorder)
	if err := rig.provider.Reconnect(next); err != nil {
		t.Fatal(err)
	}
	rig.session = next
	again := rig.refresh(t, h06Script(t, rig.sources, fixture.Path, start)...)
	if again.Exchanges[0].Elapsed != HuaweiProviderFirstRequestTolerated || again.Reason != HuaweiProviderReasonIdentified {
		t.Fatalf("after reconnect: %+v %s", again.Exchanges[0], again.Reason)
	}
	// Two timeouts in a row at the first request: the second is a failed
	// attempt, and four failed attempts end the read.
	rig = newH06Rig(t, fixture.Path, []uint8{1})
	var timeouts []h06Expected
	for attempt := 0; attempt < 5; attempt++ {
		timeouts = append(timeouts, h06FC03(100, huaweiSDongleSearchOffset, huaweiSDongleSearchQuantity, h06Answer{err: h06Timeout()}))
	}
	cycle := rig.refresh(t, timeouts...)
	if cycle.Outcome != HuaweiProviderCycleInsufficient || cycle.Reason != HuaweiProviderReasonGuardFailed || len(cycle.Exchanges) != 5 ||
		cycle.Exchanges[1].Elapsed != HuaweiProviderAtDeadline || cycle.Exchanges[4].Attempt != 4 {
		t.Fatalf("five timeouts: %s %s %+v", cycle.Outcome, cycle.Reason, cycle.Exchanges)
	}
}

func TestH06SDongleGatewayIdentityGateRequiresTheBasicRefusal(t *testing.T) {
	fixture, start := h06FixtureCycle(t, "sdongle_provider_cycles.json", "start_identification")
	sources := h06LoadSources(t)
	childBasic := h06Script(t, sources, fixture.Path, h06Cycle{Steps: []h06Step{start.Steps[7]}})[0].answer
	for name, answer := range map[string]h06Answer{
		"basic answered":    childBasic,
		"exception 2":       {err: h06Exception(0x2b, 2)},
		"exception 4":       {err: h06Exception(0x2b, 4)},
		"reset after write": {err: errors.New("scripted peer reset after the request was written")},
	} {
		t.Run(name, func(t *testing.T) {
			rig := newH06Rig(t, fixture.Path, []uint8{1})
			script := h06Script(t, rig.sources, fixture.Path, start)
			// Keep the steps up to and including the gateway basic read.
			script = script[:3]
			script[2].answer = answer
			if name == "reset after write" {
				for attempt := 1; attempt < 4; attempt++ {
					script = append(script, h06FC2B(100, 1, 0, answer))
				}
			} else {
				rest := h06Script(t, rig.sources, fixture.Path, start)[3:10]
				script = append(script, rest...)
			}
			cycle := rig.refresh(t, script...)
			if cycle.Outcome != HuaweiProviderCycleInsufficient || cycle.Gateway.State != HuaweiProviderGatewayInsufficient {
				t.Fatalf("outcome %s gateway %+v", cycle.Outcome, cycle.Gateway)
			}
			for _, exchange := range cycle.Exchanges {
				if exchange.UnitID == 1 && exchange.Offset >= 32000 {
					t.Fatalf("a child group was read without the gateway tuple: %+v", exchange)
				}
			}
			if len(cycle.Children) != 0 && cycle.Children[0].Evidence != HuaweiProviderEvidenceWithheld {
				t.Fatalf("child: %+v", cycle.Children[0])
			}
		})
	}
}

func TestH06SDongleGuardFailureBacksOffAndIdentifiesAgain(t *testing.T) {
	fixture, start := h06FixtureCycle(t, "sdongle_provider_cycles.json", "start_identification")
	_, reidentify := h06FixtureCycle(t, "sdongle_provider_cycles.json", "reidentification")
	rig := newH06Rig(t, fixture.Path, []uint8{1})
	rig.refresh(t, h06Script(t, rig.sources, fixture.Path, start)...)
	var timeouts []h06Expected
	for attempt := 0; attempt < 4; attempt++ {
		timeouts = append(timeouts, h06FC03(100, huaweiSDongleSearchOffset, huaweiSDongleSearchQuantity, h06Answer{err: h06Timeout()}))
	}
	failed := rig.refresh(t, timeouts...)
	child, _ := failed.Child(1)
	if failed.Outcome != HuaweiProviderCycleIncomplete || failed.Reason != HuaweiProviderReasonGuardFailed ||
		failed.Gateway.State != HuaweiProviderGatewayUnconfirmed || child.Evidence != HuaweiProviderEvidenceStale {
		t.Fatalf("guard failure: %s %s %+v %+v", failed.Outcome, failed.Reason, failed.Gateway, child)
	}
	if wait := rig.provider.NextRefreshAt().Sub(failed.StartedAt); wait < 2*time.Minute {
		t.Fatalf("back-off %s", wait)
	}
	// The next cycle identifies the child again at its unit.
	again := rig.refresh(t, h06Script(t, rig.sources, fixture.Path, reidentify)...)
	if again.Reason != HuaweiProviderReasonIdentified || again.Outcome != HuaweiProviderCycleComplete {
		t.Fatalf("after the guard failure: %s %s", again.Outcome, again.Reason)
	}
}

func TestH06ChildBackoffSkipsTheDocumentedCycles(t *testing.T) {
	for failures := 1; failures <= 8; failures++ {
		wait := huaweiChildBackoffCycles(failures)
		skipUntil := huaweiChildSkipUntil(100, failures)
		var skipped uint64
		for cycle := uint64(101); cycle < 200 && skipUntil > cycle; cycle++ {
			skipped++
		}
		if skipped != wait {
			t.Fatalf("%d failures: backoff %d cycles but %d skipped", failures, wait, skipped)
		}
	}
}

func TestH06SmartLoggerGuardFailureIdentifiesAgain(t *testing.T) {
	fixture, start := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "start_identification")
	rig := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
	rig.refresh(t, h06Script(t, rig.sources, fixture.Path, start)...)
	var timeouts []h06Expected
	for attempt := 0; attempt < 4; attempt++ {
		timeouts = append(timeouts, h06FC03(0, huaweiSmartLoggerCounterOffset, 1, h06Answer{err: h06Timeout()}))
	}
	failed := rig.refresh(t, timeouts...)
	if failed.Reason != HuaweiProviderReasonGuardFailed {
		t.Fatalf("guard failure: %s %s", failed.Outcome, failed.Reason)
	}
	// A matching counter alone must not resume the cached children.
	again := rig.refresh(t, h06Script(t, rig.sources, fixture.Path, start)...)
	if again.Reason != HuaweiProviderReasonIdentified || again.Outcome != HuaweiProviderCycleComplete {
		t.Fatalf("after the guard failure: %s %s", again.Outcome, again.Reason)
	}
}

func TestH06SmartLoggerInventoryDeadlineStopsPagination(t *testing.T) {
	fixture, start := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "start_identification")
	rig := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
	script := h06Script(t, rig.sources, fixture.Path, start)
	if len(script) < 3 || script[1].function != 0x2b {
		t.Fatalf("unexpected start script shape")
	}
	script[1].answer.elapsed = 16 * time.Second
	cycle := rig.refresh(t, script[:2]...)
	if cycle.Outcome != HuaweiProviderCycleIncomplete || cycle.Reason != HuaweiProviderReasonListingUnstable {
		t.Fatalf("slow inventory: %s %s", cycle.Outcome, cycle.Reason)
	}
}

// h06ReentrantSession calls back into the provider from Close, as a session
// that reports its final state would.
type h06ReentrantSession struct {
	*h06Session
	provider h06Provider
}

func (s *h06ReentrantSession) Close() error {
	s.provider.LastComplete()
	s.provider.NextRefreshAt()
	return s.h06Session.Close()
}

func TestH06CloseOfTheSessionRunsOutsideTheProviderLock(t *testing.T) {
	for name, act := range map[string]func(h06Provider, HuaweiProviderSession) error{
		"stop":      func(p h06Provider, _ HuaweiProviderSession) error { return p.Stop() },
		"close":     func(p h06Provider, _ HuaweiProviderSession) error { return p.Close() },
		"reconnect": func(p h06Provider, next HuaweiProviderSession) error { return p.Reconnect(next) },
	} {
		t.Run(name, func(t *testing.T) {
			rig := newH06Rig(t, "SmartLogger", []uint8{1})
			if err := rig.provider.Stop(); err != nil {
				t.Fatal(err)
			}
			session := &h06ReentrantSession{h06Session: newH06Session("reentrant", rig.clock, rig.recorder), provider: rig.provider}
			if err := rig.provider.Start(session); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- act(rig.provider, newH06Session("next", rig.clock, rig.recorder)) }()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("deadlock: the session was closed under the provider lock")
			}
			if session.Closes() != 1 {
				t.Fatalf("closes=%d", session.Closes())
			}
		})
	}
}

func TestH06InventoryRetriesStayWithinTheEnumerationDeadline(t *testing.T) {
	rig := newH06Rig(t, "SmartLogger", []uint8{1})
	script := []h06Expected{h06FC03(0, huaweiSmartLoggerCounterOffset, 1, h06Answer{words: []uint16{1}})}
	for i := 0; i < 3; i++ {
		script = append(script, h06FC2B(0, 3, huaweiSmartLoggerInventoryStartObject, h06Answer{err: h06Timeout()}))
	}
	cycle := rig.refresh(t, script...)
	if cycle.Outcome != HuaweiProviderCycleIncomplete || cycle.Reason != HuaweiProviderReasonListingUnstable {
		t.Fatalf("cycle: %s %s", cycle.Outcome, cycle.Reason)
	}
	calls := rig.recorder.Calls()
	start := calls[0].endAt
	limit := time.Duration(NewHuaweiSmartLoggerObservedQualificationCard().Limits().DeadlineMilliseconds) * time.Millisecond
	for _, call := range calls[1:] {
		if call.sentAt.Sub(start) >= limit {
			t.Fatalf("request sent %s into the enumeration, deadline %s", call.sentAt.Sub(start), limit)
		}
	}
}

func h06InventoryJump(next uint8) h06Answer {
	return h06Answer{page: &HuaweiIdentificationPage{
		ReadDeviceIDCode: 3, ResponseBytes: 10, DeclaredObjects: 1, More: true, NextObjectID: next,
		Objects: []HuaweiGatewayInventoryObject{{ObjectID: huaweiSmartLoggerInventoryStartObject, Value: []byte{3}}},
	}}
}

// The continuation rule is the observed inventory parser's: one bounded wrap
// of the object identifier is valid, a second wrap or a repeated cursor is
// unstable and is not requested.
func TestH06InventoryContinuationAllowsOneBoundedWrap(t *testing.T) {
	start := uint8(huaweiSmartLoggerInventoryStartObject)
	requested := func(rig *h06Rig, object uint8) bool {
		for _, call := range rig.recorder.Calls() {
			if call.function == 0x2b && call.object == object {
				return true
			}
		}
		return false
	}
	run := func(steps ...h06Expected) (*h06Rig, HuaweiProviderCycle) {
		rig := newH06Rig(t, "SmartLogger", []uint8{1})
		rig.session.Append(h06FC03(0, huaweiSmartLoggerCounterOffset, 1, h06Answer{words: []uint16{1}}))
		rig.session.Append(steps...)
		cycle, err := rig.provider.Refresh(context.Background())
		if err != nil {
			t.Fatalf("refresh: %v", err)
		}
		return rig, cycle
	}
	wrapTo := func(next uint8) h06Answer {
		answer := h06InventoryJump(next)
		answer.page.Objects[0].ObjectID = 0xff
		return answer
	}

	t.Run("a single wrap is followed", func(t *testing.T) {
		rig, _ := run(h06FC2B(0, 3, start, h06InventoryJump(0xff)), h06FC2B(0, 3, 0xff, wrapTo(0x00)),
			h06FC2B(0, 3, 0x00, h06Answer{err: h06Timeout()}))
		if !requested(rig, 0x00) {
			t.Fatal("the continuation after one wrap was not requested")
		}
	})
	t.Run("a second wrap is unstable", func(t *testing.T) {
		rig, cycle := run(h06FC2B(0, 3, start, h06InventoryJump(0xff)), h06FC2B(0, 3, 0xff, wrapTo(0x10)),
			h06FC2B(0, 3, 0x10, wrapTo(0x05)))
		if requested(rig, 0x05) || cycle.Reason != HuaweiProviderReasonListingUnstable {
			t.Fatalf("second wrap: requested=%v reason=%s", requested(rig, 0x05), cycle.Reason)
		}
	})
	t.Run("a repeated cursor is unstable", func(t *testing.T) {
		rig, cycle := run(h06FC2B(0, 3, start, h06InventoryJump(start)))
		if cycle.Reason != HuaweiProviderReasonListingUnstable || rig.session.Remaining() != 0 {
			t.Fatalf("repeated cursor: %s", cycle.Reason)
		}
	})
}

// A reply of any kind that arrives after the attempt deadline is not current.
func TestH06LateRepliesOfEveryKindAreNotCurrent(t *testing.T) {
	fixture, start := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "start_identification")
	for _, kind := range []struct {
		name string
		err  error
	}{
		{"answered", nil},
		{"exception 3", h06Exception(0x03, 3)},
		{"exception 2", h06Exception(0x03, 2)},
	} {
		for _, late := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s late=%v", kind.name, late), func(t *testing.T) {
				rig := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
				script := h06Script(t, rig.sources, fixture.Path, start)
				first := script[0]
				first.answer.err = kind.err
				if late {
					first.answer.elapsed = 6 * time.Second
				}
				rig.session.Append(first)
				cycle, err := rig.provider.Refresh(context.Background())
				if err != nil {
					t.Fatalf("refresh: %v", err)
				}
				got := cycle.Exchanges[0]
				if late && (got.Outcome != HuaweiProviderExchangeTimeout || got.Elapsed != HuaweiProviderAtDeadline || got.ExceptionCode != 0) {
					t.Fatalf("late reply recorded as %v %v code %d", got.Outcome, got.Elapsed, got.ExceptionCode)
				}
				if !late && kind.err != nil && got.Outcome != HuaweiProviderExchangeException {
					t.Fatalf("timely exception recorded as %v", got.Outcome)
				}
			})
		}
	}
}

// h06BlockingCloseSession holds Close until released.
type h06BlockingCloseSession struct {
	*h06Session
	entered chan struct{}
	release chan struct{}
}

func (s *h06BlockingCloseSession) Close() error {
	close(s.entered)
	<-s.release
	return s.h06Session.Close()
}

func TestH06ReconnectBindsTheNewSessionOnlyAfterTheOldCloseReturns(t *testing.T) {
	fixture, start := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "start_identification")
	rig := newH06Rig(t, "SmartLogger", h06Units(fixture.AllowedUnits))
	if err := rig.provider.Stop(); err != nil {
		t.Fatal(err)
	}
	old := &h06BlockingCloseSession{
		h06Session: newH06Session("old", rig.clock, rig.recorder),
		entered:    make(chan struct{}), release: make(chan struct{}),
	}
	if err := rig.provider.Start(old); err != nil {
		t.Fatal(err)
	}
	next := newH06Session("next", rig.clock, rig.recorder)
	done := make(chan error, 1)
	go func() { done <- rig.provider.Reconnect(next) }()
	<-old.entered
	// The old Close has not returned: nothing may be read through the new session.
	if _, err := rig.provider.Refresh(context.Background()); !errors.Is(err, ErrHuaweiProviderNotStarted) {
		t.Fatalf("refresh while the old session closes: %v", err)
	}
	if calls := rig.recorder.Calls(); len(calls) != 0 {
		t.Fatalf("requests sent while the old session closes: %d", len(calls))
	}
	close(old.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if old.Closes() != 1 {
		t.Fatalf("old session closes: %d", old.Closes())
	}
	rig.session = next
	cycle := rig.refresh(t, h06Script(t, rig.sources, "SmartLogger", start)...)
	if cycle.Reason != HuaweiProviderReasonIdentified {
		t.Fatalf("after reconnect: %s", cycle.Reason)
	}
}

func TestH06CancelledCycleKeepsAnExceptionThreeRefusal(t *testing.T) {
	fixtureFile, _ := h06Load(t, "smartlogger_provider_cycles.json")
	_, start := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "start_identification")
	_, refusal := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "runtime_refusal")
	_, notSent := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "refused_group_not_sent")
	rig := newH06Rig(t, fixtureFile.Path, h06Units(fixtureFile.AllowedUnits))
	rig.refresh(t, h06Script(t, rig.sources, fixtureFile.Path, start)...)

	script := h06Script(t, rig.sources, fixtureFile.Path, refusal)
	entered, release := make(chan struct{}), make(chan struct{})
	// The refusal is answered at unit 1; the cycle is cancelled at unit 2.
	script[4].answer.entered, script[4].answer.release = entered, release
	rig.session.Append(script[:5]...)
	rig.clock.Advance(rig.provider.NextRefreshAt().Sub(rig.clock.Now()))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := rig.provider.Refresh(ctx)
		done <- err
	}()
	<-entered
	cancel()
	close(release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled refresh: %v", err)
	}
	// The next cycle must not send the refused group again.
	cycle := rig.refresh(t, h06Script(t, rig.sources, fixtureFile.Path, notSent)...)
	if cycle.Outcome != HuaweiProviderCycleComplete {
		t.Fatalf("cycle after the cancellation: %s %s", cycle.Outcome, cycle.Reason)
	}
}

func TestH06FencedCycleKeepsAnExceptionThreeRefusal(t *testing.T) {
	fixtureFile, _ := h06Load(t, "smartlogger_provider_cycles.json")
	_, start := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "start_identification")
	_, refusal := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "runtime_refusal")
	rig := newH06Rig(t, fixtureFile.Path, h06Units(fixtureFile.AllowedUnits))
	rig.refresh(t, h06Script(t, rig.sources, fixtureFile.Path, start)...)

	script := h06Script(t, rig.sources, fixtureFile.Path, refusal)
	entered, release := make(chan struct{}), make(chan struct{})
	// The refusal is answered at unit 1; the cycle is fenced at unit 2.
	script[4].answer.entered, script[4].answer.release = entered, release
	rig.session.Append(script[:5]...)
	rig.clock.Advance(rig.provider.NextRefreshAt().Sub(rig.clock.Now()))
	done := make(chan error, 1)
	go func() {
		_, err := rig.provider.Refresh(context.Background())
		done <- err
	}()
	<-entered
	if err := rig.provider.Stop(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, ErrHuaweiProviderFenced) {
		t.Fatalf("fenced refresh: %v", err)
	}
	next := newH06Session("next", rig.clock, rig.recorder)
	if err := rig.provider.Start(next); err != nil {
		t.Fatal(err)
	}
	rig.session = next
	// The refused group (unit 1, 32016) is not requested again.
	var kept []h06Expected
	for _, step := range h06Script(t, rig.sources, fixtureFile.Path, start) {
		if step.function == 0x03 && step.unit == 1 && step.offset == 32016 {
			continue
		}
		kept = append(kept, step)
	}
	rig.refresh(t, kept...)
}

func TestH06TransitionsDuringABlockingCloseAreRefusedAsClosing(t *testing.T) {
	for name, transition := range map[string]func(p h06Provider, s HuaweiProviderSession) error{
		"stop":      func(p h06Provider, _ HuaweiProviderSession) error { return p.Stop() },
		"reconnect": func(p h06Provider, s HuaweiProviderSession) error { return p.Reconnect(s) },
	} {
		t.Run(name, func(t *testing.T) {
			rig := newH06Rig(t, "SmartLogger", []uint8{1})
			if err := rig.provider.Stop(); err != nil {
				t.Fatal(err)
			}
			old := &h06BlockingCloseSession{
				h06Session: newH06Session("old", rig.clock, rig.recorder),
				entered:    make(chan struct{}), release: make(chan struct{}),
			}
			if err := rig.provider.Start(old); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- transition(rig.provider, newH06Session("next", rig.clock, rig.recorder)) }()
			<-old.entered
			rival := newH06Session("rival", rig.clock, rig.recorder)
			if err := rig.provider.Start(rival); !errors.Is(err, ErrHuaweiProviderClosing) {
				t.Fatalf("start during the close: %v", err)
			}
			if err := rig.provider.Reconnect(rival); !errors.Is(err, ErrHuaweiProviderClosing) {
				t.Fatalf("reconnect during the close: %v", err)
			}
			if err := rig.provider.Stop(); !errors.Is(err, ErrHuaweiProviderClosing) {
				t.Fatalf("stop during the close: %v", err)
			}
			if _, err := rig.provider.Refresh(context.Background()); !errors.Is(err, ErrHuaweiProviderNotStarted) {
				t.Fatalf("refresh during the close: %v", err)
			}
			if calls := rig.recorder.Calls(); len(calls) != 0 {
				t.Fatalf("requests sent during the close: %d", len(calls))
			}
			close(old.release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if rig.session != nil && old.Closes() != 1 {
				t.Fatalf("old session closes: %d", old.Closes())
			}
			if err := rig.provider.Start(rival); name == "stop" && err != nil {
				t.Fatalf("start after the close: %v", err)
			}
		})
	}
}

func TestH06WithheldChildStaysWithheldAcrossAGatewayFailure(t *testing.T) {
	fixture, start := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "start_identification")
	rig := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
	first := rig.refresh(t, h06Script(t, rig.sources, fixture.Path, start)...)
	if child, _ := first.Child(1); child.Evidence != HuaweiProviderEvidenceCurrent {
		t.Fatalf("first cycle child: %+v", child)
	}
	// The next enumeration lists every child with a class that has no contract.
	sources := rig.sources
	inventory := sources.smartLoggerInventory
	for pageIndex := range inventory.Pages {
		for objectIndex := range inventory.Pages[pageIndex].Objects {
			object := &inventory.Pages[pageIndex].Objects[objectIndex]
			object.Entry = strings.Replace(object.Entry, "8=SUN2000", "8=BACKUP0", 1)
		}
	}
	sources.smartLoggerInventory = inventory
	rig.sources = sources
	next := newH06Session("generation-2", rig.clock, rig.recorder)
	if err := rig.provider.Reconnect(next); err != nil {
		t.Fatal(err)
	}
	rig.session = next
	var script []h06Expected
	for _, step := range h06IdentificationSteps() {
		script = append(script, h06Script(t, sources, fixture.Path, h06Cycle{Steps: []h06Step{step}})...)
	}
	withheld := rig.refresh(t, script...)
	if child, _ := withheld.Child(1); child.Evidence != HuaweiProviderEvidenceWithheld || child.Reason != HuaweiProviderReasonNoClassContract {
		t.Fatalf("re-enumeration: %+v", child)
	}
	var timeouts []h06Expected
	for attempt := 0; attempt < 4; attempt++ {
		timeouts = append(timeouts, h06FC03(0, huaweiSmartLoggerCounterOffset, 1, h06Answer{err: h06Timeout()}))
	}
	failed := rig.refresh(t, timeouts...)
	if failed.Reason != HuaweiProviderReasonGuardFailed {
		t.Fatalf("failed cycle: %s %s", failed.Outcome, failed.Reason)
	}
	child, _ := failed.Child(1)
	if child.Evidence != HuaweiProviderEvidenceWithheld || child.Reason != HuaweiProviderReasonNoClassContract || len(child.Reading.Facts()) != 0 {
		t.Fatalf("child after the gateway failure: %+v", child)
	}
}

func TestH06SmartLoggerMismatchFenceSurvivesAGatewayReadFailure(t *testing.T) {
	fixture, start := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "start_identification")
	rig := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
	first := rig.refresh(t, h06Script(t, rig.sources, fixture.Path, start)...)
	if child, _ := first.Child(1); child.Evidence != HuaweiProviderEvidenceCurrent {
		t.Fatalf("first cycle child: %+v", child)
	}
	rig.refresh(t, h06FC03(0, huaweiSmartLoggerCounterOffset, 1, h06Answer{words: []uint16{8}}))
	sources := rig.sources
	inventory := sources.smartLoggerInventory
	inventory.Pages = append([]h03InventoryPage(nil), inventory.Pages...)
	for index := range inventory.Pages {
		inventory.Pages[index].Objects = append([]h03Object(nil), inventory.Pages[index].Objects...)
	}
	object := &inventory.Pages[1].Objects[0]
	object.Entry = strings.Replace(object.Entry, "V300R023C10SPC620", "V300R023C10SPC621", 1)
	sources.smartLoggerInventory = inventory
	var script []h06Expected
	for _, step := range h06IdentificationSteps() {
		script = append(script, h06Script(t, sources, fixture.Path, h06Cycle{Steps: []h06Step{step}})...)
	}
	if mismatch := rig.refresh(t, script...); mismatch.Reason != HuaweiProviderReasonGatewayMismatch {
		t.Fatalf("mismatch cycle: %s %s", mismatch.Outcome, mismatch.Reason)
	}
	var timeouts []h06Expected
	for attempt := 0; attempt < 4; attempt++ {
		timeouts = append(timeouts, h06FC03(0, huaweiSmartLoggerCounterOffset, 1, h06Answer{err: h06Timeout()}))
	}
	failed := rig.refresh(t, timeouts...)
	if failed.Reason != HuaweiProviderReasonGatewayReadFail {
		t.Fatalf("failed cycle: %s %s", failed.Outcome, failed.Reason)
	}
	if len(failed.Children) == 0 {
		t.Fatal("no child rendered")
	}
	for _, child := range failed.Children {
		if child.Evidence != HuaweiProviderEvidenceWithheld || child.Reason != HuaweiProviderReasonGatewayMismatch || len(child.Reading.Facts()) != 0 {
			t.Fatalf("child %d released by a read failure after a mismatch: %+v", child.UnitID, child)
		}
	}
}

// The refusal is recorded where the reply is decoded, so a cancellation or a
// fence that coincides with the exception 3 reply of the same exchange still
// keeps the group from being sent again.
func TestH06RefusalCoincidingWithStopIsRecorded(t *testing.T) {
	for _, name := range []string{"cancel", "stop"} {
		t.Run(name, func(t *testing.T) {
			fixtureFile, _ := h06Load(t, "smartlogger_provider_cycles.json")
			_, start := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "start_identification")
			_, refusal := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "runtime_refusal")
			_, notSent := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "refused_group_not_sent")
			rig := newH06Rig(t, fixtureFile.Path, h06Units(fixtureFile.AllowedUnits))
			rig.refresh(t, h06Script(t, rig.sources, fixtureFile.Path, start)...)

			script := h06Script(t, rig.sources, fixtureFile.Path, refusal)
			index := -1
			for i, step := range script {
				var protocol *modbus.ProtocolError
				if step.function == 0x03 && errors.As(step.answer.err, &protocol) && protocol.ExceptionCode == huaweiSUN2000RefusalCode {
					index = i
					break
				}
			}
			if index < 0 {
				t.Fatal("no exception 3 step in the refusal cycle")
			}
			entered, release := make(chan struct{}), make(chan struct{})
			script[index].answer.entered, script[index].answer.release = entered, release
			rig.session.Append(script[:index+1]...)
			rig.clock.Advance(rig.provider.NextRefreshAt().Sub(rig.clock.Now()))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := rig.provider.Refresh(ctx)
				done <- err
			}()
			<-entered
			want := context.Canceled
			if name == "cancel" {
				cancel()
			} else {
				if err := rig.provider.Stop(); err != nil {
					t.Fatal(err)
				}
				want = ErrHuaweiProviderFenced
			}
			close(release)
			if err := <-done; !errors.Is(err, want) {
				t.Fatalf("refresh: %v", err)
			}
			if name == "stop" {
				next := newH06Session("next", rig.clock, rig.recorder)
				if err := rig.provider.Start(next); err != nil {
					t.Fatal(err)
				}
				rig.session = next
				var kept []h06Expected
				for _, step := range h06Script(t, rig.sources, fixtureFile.Path, start) {
					if step.function == 0x03 && step.unit == 1 && step.offset == 32016 {
						continue
					}
					kept = append(kept, step)
				}
				rig.refresh(t, kept...)
				return
			}
			cycle := rig.refresh(t, h06Script(t, rig.sources, fixtureFile.Path, notSent)...)
			if cycle.Outcome != HuaweiProviderCycleComplete {
				t.Fatalf("cycle after the cancellation: %s %s", cycle.Outcome, cycle.Reason)
			}
		})
	}
}

func TestH06LateSuccessAfterTheAttemptDeadlineIsNotCurrent(t *testing.T) {
	fixture, start := h06FixtureCycle(t, "smartlogger_provider_cycles.json", "start_identification")
	rig := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
	script := h06Script(t, rig.sources, fixture.Path, start)
	if len(script) == 0 || script[0].answer.err != nil {
		t.Fatalf("unexpected start script shape")
	}
	script[0].answer.elapsed = 6 * time.Second
	rig.session.Append(script[:1]...)
	cycle, err := rig.provider.Refresh(context.Background())
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(cycle.Exchanges) == 0 {
		t.Fatal("no exchange recorded")
	}
	first := cycle.Exchanges[0]
	if first.Outcome != HuaweiProviderExchangeTimeout || first.Elapsed != HuaweiProviderAtDeadline {
		t.Fatalf("late reply recorded as %v %v", first.Outcome, first.Elapsed)
	}
	if cycle.Outcome == HuaweiProviderCycleComplete {
		t.Fatalf("a cycle with only a late reply completed")
	}
}
