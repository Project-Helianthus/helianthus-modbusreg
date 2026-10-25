package modbusreg

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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
	if child.Evidence != HuaweiProviderEvidenceWithheld || child.Reason != HuaweiProviderReasonIdentityMismatch || len(child.Reading.Facts()) != 0 {
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
