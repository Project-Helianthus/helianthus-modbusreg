package modbusreg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestH06ProviderCycleFixturesDecodeStrictlyRoundTripAndAreSanitized(t *testing.T) {
	entries, err := os.ReadDir(h06FixtureDirectory)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if !reflect.DeepEqual(names, h06FixtureNames()) {
		t.Fatalf("provider fixtures=%v want=%v", names, h06FixtureNames())
	}
	walked := map[string]bool{}
	for _, path := range h03CommittedHuaweiTestdata(t) {
		walked[path] = true
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			fixture, data := h06Load(t, name)
			encoded, err := json.MarshalIndent(fixture, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if string(append(encoded, '\n')) != string(data) {
				t.Fatal("fixture does not round-trip byte for byte")
			}
			if fixture.Schema != "helianthus-huawei-provider-cycle-fixture/v1" || fixture.Kind != "scripted_provider_cycles" ||
				fixture.License != "CC0-1.0" || !strings.HasPrefix(fixture.Provenance, "synthetic") || fixture.Sanitization.Rule == "" {
				t.Fatalf("unexpected header: %+v", fixture)
			}
			wantID := map[string]string{"SmartLogger": HuaweiSmartLoggerReadOnlyProviderID, "S-Dongle": HuaweiSDongleReadOnlyProviderID}[fixture.Path]
			if wantID == "" || fixture.ProviderID != wantID {
				t.Fatalf("fixture %s names provider %q for path %q", name, fixture.ProviderID, fixture.Path)
			}
			if !walked[h06FixtureDirectory+"/"+name] {
				t.Fatalf("%s is outside the committed shape scan", name)
			}
			if findings := h03ShapeFindings(string(data)); len(findings) != 0 {
				t.Fatalf("%s holds serial-, address- or time-shaped values: %v", name, findings)
			}
			for _, source := range fixture.Sources {
				if _, err := os.Stat("testdata/huawei/" + source); err != nil {
					t.Fatalf("source %s: %v", source, err)
				}
			}
			// Every literal reply is marked synthetic; every other reply comes from
			// a source fixture or is a timeout or an exception. A reply taken from
			// a case of the catalogue decision table is a source reply of an
			// identity read, and the table is one of the named sources.
			decisionTable := false
			for _, source := range fixture.Sources {
				decisionTable = decisionTable || source == h06DecisionTableSource
			}
			for _, cycle := range fixture.Cycles {
				for _, step := range cycle.Steps {
					if step.Reply.Case != "" && (step.Reply.Kind != "source" || !decisionTable || step.Function != "FC03" ||
						(step.Offset != huaweiSUN2000IdentifierOffset && step.Offset != huaweiSUN2000SoftwareOffset)) {
						t.Fatalf("%s: a decision-table reply outside an identity read: %+v", cycle.Name, step)
					}
					switch step.Reply.Kind {
					case "source", "timeout", "exception", "transport":
						if len(step.Reply.Words) != 0 || step.Reply.Synthetic {
							t.Fatalf("%s: %s reply carries words: %+v", cycle.Name, step.Reply.Kind, step)
						}
					case "words":
						if !step.Reply.Synthetic {
							t.Fatalf("%s: literal words are not marked synthetic: %+v", cycle.Name, step)
						}
					default:
						t.Fatalf("%s: unknown reply %q", cycle.Name, step.Reply.Kind)
					}
				}
			}
		})
	}
}

func TestH06ACompleteCycleYieldsTypedPerChildResults(t *testing.T) {
	for _, test := range []struct {
		fixture string
		profile string
		units   []uint8
	}{
		{"smartlogger_provider_cycles.json", HuaweiSUN2000M3MonitoringProfileID, []uint8{1, 2}},
		{"sdongle_provider_cycles.json", HuaweiSUN2000M1MonitoringProfileID, []uint8{1}},
	} {
		t.Run(test.fixture, func(t *testing.T) {
			_, cycles := h06RunFixture(t, test.fixture)
			first := cycles[0]
			monitoring, _ := h05Load(t, h05FixtureFor(test.profile))
			if first.Outcome != HuaweiProviderCycleComplete || first.Gateway.State != HuaweiProviderGatewayIdentified ||
				first.Source != "endpoint-a" || first.StartedAt.IsZero() || !first.CompletedAt.After(first.StartedAt) {
				t.Fatalf("first cycle: %+v", first)
			}
			for _, unit := range test.units {
				child, ok := first.Child(unit)
				if !ok || child.Evidence != HuaweiProviderEvidenceCurrent || child.ProfileID != test.profile ||
					child.Family != h05Profile(t, test.profile).Family() || child.ListedClass != huaweiSUN2000ClassLabel {
					t.Fatalf("unit %d: %+v", unit, child)
				}
				var expected []h05Expected
				for _, reading := range monitoring.Readings {
					if reading.Unit == unit {
						expected = reading.Expected
					}
				}
				if len(expected) == 0 || len(child.Reading.Facts()) != len(expected) {
					t.Fatalf("unit %d: %d facts, want %d", unit, len(child.Reading.Facts()), len(expected))
				}
				for _, want := range expected {
					fact := h05FactValue(t, child.Reading, want.Key, want.Index)
					if fact.Value.String() != want.Value || fact.Unit != want.Unit || string(fact.Grade) != want.Grade ||
						fact.State != want.State || fact.Retained || fact.Cycle != first.Cycle {
						t.Fatalf("unit %d %s[%d]: %+v want %+v", unit, want.Key, want.Index, fact, want)
					}
				}
				for _, group := range child.Groups {
					if group.Outcome != HuaweiSUN2000GroupAnswered || group.Retained || group.Cycle != first.Cycle ||
						group.Generation != first.Generation || group.ReadAt.IsZero() {
						t.Fatalf("unit %d group %d: %+v", unit, group.Offset, group)
					}
				}
			}
			if first.SupportClaim() || first.WriteAuthority() || first.LiveQualified() || first.CatalogRegistered() || first.AutomaticRuntimeAdmission() {
				t.Fatal("a cycle carries authority")
			}
		})
	}
}

func TestH06AChildTimeoutMakesOnlyThatChildStale(t *testing.T) {
	_, cycles := h06RunFixture(t, "smartlogger_provider_cycles.json")
	first, second := cycles[0], cycles[1]
	sibling, _ := second.Child(1)
	stale, _ := second.Child(2)
	if second.Outcome != HuaweiProviderCycleComplete || sibling.Evidence != HuaweiProviderEvidenceCurrent ||
		stale.Evidence != HuaweiProviderEvidenceStale || stale.Reason != HuaweiProviderReasonChildTimeout {
		t.Fatalf("second cycle: %v", h06ChildSummary(second))
	}
	before, _ := first.Child(2)
	for _, fact := range stale.Reading.Facts() {
		earlier, ok := before.Reading.Fact(fact.Key, fact.Index)
		if !ok || fact.Value != earlier.Value || !fact.Retained || fact.Cycle != first.Cycle {
			t.Fatalf("stale fact %s[%d] = %+v, earlier %+v", fact.Key, fact.Index, fact, earlier)
		}
	}
	for _, group := range stale.Groups {
		if !group.Retained || group.Cycle != first.Cycle || group.ReadAt.After(first.CompletedAt) || group.Generation != first.Generation {
			t.Fatalf("stale group %d: %+v", group.Offset, group)
		}
	}
	// Four attempts at the child's first group, then nothing more at that
	// child, and the sibling's groups were read before it.
	attempts := 0
	for _, exchange := range second.Exchanges {
		if exchange.UnitID == 2 {
			attempts++
			if exchange.Offset != 32000 || exchange.Outcome != HuaweiProviderExchangeTimeout || exchange.Elapsed != HuaweiProviderAtDeadline {
				t.Fatalf("child 2 exchange: %+v", exchange)
			}
		}
	}
	if attempts != 4 {
		t.Fatalf("child 2 attempts=%d want 4", attempts)
	}

	_, sdongle := h06RunFixture(t, "sdongle_provider_cycles.json")
	child, _ := sdongle[2].Child(1)
	if child.Evidence != HuaweiProviderEvidenceStale || child.Reason != HuaweiProviderReasonChildTimeout || sdongle[2].Gateway.State != HuaweiProviderGatewayIdentified {
		t.Fatalf("S-Dongle child timeout: %+v", child)
	}
	fresh, ok := child.Reading.Fact("huawei.sun2000.m1.pv_string_voltage", 1)
	if ok && fresh.Availability == HuaweiFactValue && !fresh.Retained {
		t.Fatalf("a string fact of the failed group is fresh: %+v", fresh)
	}
}

func TestH06ACounterChangeInMidCycleKeepsThePreviousCompleteSnapshot(t *testing.T) {
	_, cycles := h06RunFixture(t, "smartlogger_provider_cycles.json")
	changed, unstable := cycles[2], cycles[3]
	for _, cycle := range []HuaweiProviderCycle{changed, unstable} {
		if cycle.Outcome != HuaweiProviderCycleIncomplete {
			t.Fatalf("cycle %d outcome %s", cycle.Cycle, cycle.Outcome)
		}
		for _, exchange := range cycle.Exchanges {
			if exchange.UnitID != 0 {
				t.Fatalf("cycle %d sent a routed read after the counter changed: %+v", cycle.Cycle, exchange)
			}
		}
	}
	// Replay the same cycles and stop after the unstable bracket: the last
	// complete snapshot is still the cycle before the change.
	fixture, _ := h06Load(t, "smartlogger_provider_cycles.json")
	replay := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
	var kept HuaweiProviderCycle
	for index, cycle := range fixture.Cycles[:4] {
		out := replay.refresh(t, h06Script(t, replay.sources, fixture.Path, cycle)...)
		if index == 1 {
			kept = out
		}
		last, ok := replay.provider.LastComplete()
		if !ok {
			t.Fatalf("after %s there is no complete cycle", cycle.Name)
		}
		if index >= 1 && (last.Cycle != kept.Cycle || !reflect.DeepEqual(last, kept)) {
			t.Fatalf("after %s the last complete cycle is %d, want the unchanged cycle %d", cycle.Name, last.Cycle, kept.Cycle)
		}
	}
	// The next cycle enumerates once under the full bracket and identifies every
	// child again before it reads it.
	reenumerated := cycles[4]
	var units []string
	for _, exchange := range reenumerated.Exchanges {
		units = append(units, fmt.Sprintf("%d:%d", exchange.UnitID, exchange.Offset))
	}
	for _, unit := range []string{"1", "2"} {
		admission := strings.ReplaceAll("u:30000 u:30070 u:30050 u:30035 u:32000", "u", unit)
		if !strings.Contains(strings.Join(units, " "), admission) {
			t.Fatalf("re-enumeration did not identify child %s again first: %v", unit, units)
		}
	}
}

func TestH06ThreeUnstableBracketsBackOffTenCycles(t *testing.T) {
	fixture, _ := h06Load(t, "smartlogger_provider_cycles.json")
	rig := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
	rig.refresh(t, h06Script(t, rig.sources, fixture.Path, fixture.Cycles[0])...)
	rig.refresh(t, h06Script(t, rig.sources, fixture.Path, fixture.Cycles[2])...)
	unstable := fixture.Cycles[3]
	for round := 1; round <= 3; round++ {
		out := rig.refresh(t, h06Script(t, rig.sources, fixture.Path, unstable)...)
		want := HuaweiProviderReasonListingUnstable
		if round == 3 {
			want = HuaweiProviderReasonListingBackoff
			if wait := rig.provider.NextRefreshAt().Sub(out.StartedAt); wait < 11*time.Minute {
				t.Fatalf("listing back-off is %s, want at least eleven cycles", wait)
			}
		}
		if out.Reason != want {
			t.Fatalf("round %d reason %s want %s", round, out.Reason, want)
		}
	}
}

func TestH06AnUnknownChildClassIsWithheld(t *testing.T) {
	sources := h06LoadSources(t)
	// The listing names unit 3 with a product type that has no class contract
	// and unit 4 with a SUN2000 model of another family; unit 4's routed model
	// read agrees with its listing.
	const other = "SUN2000-OTHER-X"
	inventory := sources.smartLoggerInventory
	for pageIndex := range inventory.Pages {
		for objectIndex := range inventory.Pages[pageIndex].Objects {
			object := &inventory.Pages[pageIndex].Objects[objectIndex]
			switch {
			case strings.Contains(object.Entry, ";5=3;"):
				object.Entry = strings.Replace(object.Entry, "8=SUN2000", "8=BACKUP0", 1)
			case strings.Contains(object.Entry, ";5=4;"):
				object.Entry = strings.Replace(object.Entry, "1=SUN2000-50KTL-M3", "1="+other, 1)
			}
		}
	}
	sources.smartLoggerInventory = inventory
	rig := newH06Rig(t, "SmartLogger", []uint8{3, 4})
	rig.sources = sources
	var script []h06Expected
	for _, step := range h06IdentificationSteps() {
		script = append(script, h06Script(t, sources, "SmartLogger", h06Cycle{Steps: []h06Step{step}})...)
	}
	model := make([]uint16, 15)
	copy(model, []uint16{0x5355, 0x4e32, 0x3030, 0x302d, 0x4f54, 0x4845, 0x522d, 0x5800})
	script = append(script, h06FC03(4, 30000, 15, h06Answer{words: model}))
	script = append(script, h06AdmissionWords(t, sources, "SmartLogger", 4, "m3_series_identifier_selects_the_mc_card")...)
	cycle := rig.refresh(t, script...)
	unknown, _ := cycle.Child(3)
	otherFamily, _ := cycle.Child(4)
	if unknown.Evidence != HuaweiProviderEvidenceWithheld || unknown.Reason != HuaweiProviderReasonNoClassContract || unknown.ListedClass != "BACKUP0" ||
		len(unknown.Reading.Facts()) != 0 {
		t.Fatalf("unknown class: %+v", unknown)
	}
	// A model token that no catalogue row carries is not selected, with the
	// closed reason recorded.
	if otherFamily.Evidence != HuaweiProviderEvidenceWithheld || otherFamily.Reason != HuaweiProviderReasonCatalogueNotSelected ||
		h06CatalogueSummary(otherFamily.Catalogue) != "NOT_SELECTED no_catalogue_row" || len(otherFamily.Reading.Facts()) != 0 {
		t.Fatalf("other family: %+v", otherFamily)
	}
	for _, exchange := range cycle.Exchanges {
		if exchange.UnitID == 3 || (exchange.UnitID == 4 && exchange.Offset >= 32000) {
			t.Fatalf("a withheld child was read: %+v", exchange)
		}
	}
	// The withheld child is not read again in the next cycle either.
	next := rig.refresh(t, h06FC03(0, huaweiSmartLoggerCounterOffset, 1, h06Answer{words: []uint16{sources.smartLoggerInventory.ChangeCounter.Before}}))
	if child, _ := next.Child(4); child.Reason != HuaweiProviderReasonCatalogueNotSelected || h06CatalogueSummary(child.Catalogue) != "NOT_SELECTED no_catalogue_row" {
		t.Fatalf("next cycle: %+v", child)
	}
}

// h06IdentificationSteps are the SmartLogger identification reads at unit 0.
func h06IdentificationSteps() []h06Step {
	source := h06Reply{Kind: "source"}
	steps := []h06Step{{Function: "FC03", Unit: 0, Offset: huaweiSmartLoggerCounterOffset, Quantity: 1, Reply: source}}
	for object := 0x87; object <= 0x93; object++ {
		id := h03Hex(object)
		steps = append(steps, h06Step{Function: "FC2B", Unit: 0, Code: 3, Object: &id, Reply: source})
	}
	zero := h03Hex(0)
	return append(steps,
		h06Step{Function: "FC03", Unit: 0, Offset: huaweiSmartLoggerCounterOffset, Quantity: 1, Reply: source},
		h06Step{Function: "FC2B", Unit: 0, Code: 1, Object: &zero, Reply: source},
	)
}

func TestH06ProvidersReadOnlyAllowlistedPDUs(t *testing.T) {
	type allowed struct {
		unit             uint8
		function         uint8
		offset, quantity uint16
		code             uint8
	}
	sets := map[string]map[allowed]bool{
		"SmartLogger": {
			{0, 0x03, 65521, 1, 0}: true, {0, 0x2b, 0, 0, 3}: true, {0, 0x2b, 0, 0, 1}: true,
		},
		"S-Dongle": {
			{100, 0x03, 37410, 3, 0}: true, {100, 0x03, 37411, 1, 0}: true, {100, 0x03, 30068, 2, 0}: true, {100, 0x03, 37429, 1, 0}: true,
			{100, 0x2b, 0, 0, 1}: true, {1, 0x03, 30000, 15, 0}: true, {1, 0x2b, 0, 0, 1}: true, {1, 0x2b, 0, 0, 3}: true,
		},
	}
	for unit := uint8(1); unit <= 8; unit++ {
		sets["SmartLogger"][allowed{unit, 0x03, 30000, 15, 0}] = true
	}
	// The identity words of a child's admission (issue 365): the model
	// identifier, the software text and the firmware text.
	for _, read := range []allowed{{0, 0x03, 30070, 1, 0}, {0, 0x03, 30050, 15, 0}, {0, 0x03, 30035, 15, 0}} {
		for unit := uint8(1); unit <= 8; unit++ {
			read.unit = unit
			sets["SmartLogger"][read] = true
		}
		read.unit = 1
		sets["S-Dongle"][read] = true
	}
	for _, profile := range HuaweiSUN2000MonitoringProfiles() {
		path := map[string]string{HuaweiSmartLoggerCanonicalClass: "SmartLogger", HuaweiSDongleCanonicalClass: "S-Dongle"}[profile.GatewayClass()]
		for unit := uint8(1); unit <= 8; unit++ {
			for _, step := range profile.Plan(unit) {
				sets[path][allowed{unit, step.Function, step.Offset, step.Quantity, 0}] = true
			}
		}
	}
	for _, name := range h06FixtureNames() {
		t.Run(name, func(t *testing.T) {
			rig, _ := h06RunFixture(t, name)
			calls := rig.recorder.Calls()
			if len(calls) == 0 {
				t.Fatal("no request recorded")
			}
			for _, call := range calls {
				pdu := call.pdu
				if len(pdu) == 0 {
					t.Fatal("empty PDU")
				}
				switch pdu[0] {
				case 0x03:
					if len(pdu) != 5 {
						t.Fatalf("FC03 PDU %x", pdu)
					}
					offset, quantity := uint16(pdu[1])<<8|uint16(pdu[2]), uint16(pdu[3])<<8|uint16(pdu[4])
					if !sets[rig.path][allowed{call.unit, 0x03, offset, quantity, 0}] {
						t.Fatalf("FC03 at unit %d %d/%d is not a published read of the path", call.unit, offset, quantity)
					}
				case 0x2b:
					if len(pdu) != 4 || pdu[1] != 0x0e || !sets[rig.path][allowed{call.unit, 0x2b, 0, 0, pdu[2]}] {
						t.Fatalf("FC2B PDU %x at unit %d is not an allowed identification read", pdu, call.unit)
					}
					if pdu[2] == 1 && pdu[3] != 0 {
						t.Fatalf("basic identification starts at object %#x", pdu[3])
					}
				default:
					t.Fatalf("function %#x was sent", pdu[0])
				}
				for _, refused := range []struct{ offset, quantity uint16 }{{32064, 32}, {37100, 38}, {37000, 1}, {47000, 1}} {
					if rig.path == "SmartLogger" && pdu[0] == 0x03 && uint16(pdu[1])<<8|uint16(pdu[2]) == refused.offset {
						t.Fatalf("the refused or battery read %d/%d was sent", refused.offset, refused.quantity)
					}
				}
				if rig.path == "S-Dongle" && call.unit != 100 && call.unit != 1 {
					t.Fatalf("unit %d was addressed on the S-Dongle path", call.unit)
				}
				if rig.path == "SmartLogger" && (call.unit == 9 || call.unit == 11 || call.unit == 19) {
					t.Fatalf("a PLC or meter unit %d was read", call.unit)
				}
			}
		})
	}
	// The session type offers reads and the close of its own connection only.
	kind := reflect.TypeOf((*HuaweiProviderSession)(nil)).Elem()
	var methods []string
	for index := 0; index < kind.NumMethod(); index++ {
		methods = append(methods, kind.Method(index).Name)
	}
	if !reflect.DeepEqual(methods, []string{"Close", "ReadDeviceIdentification", "ReadHolding"}) {
		t.Fatalf("session methods=%v", methods)
	}
}

func TestH06PacingIsEnforcedAgainstTheClock(t *testing.T) {
	for _, name := range h06FixtureNames() {
		t.Run(name, func(t *testing.T) {
			rig, cycles := h06RunFixture(t, name)
			pacing := rig.provider.Pacing()
			// The envelope is the contract's: 750 ms and 5 s gaps, a 60 s cycle,
			// a 5 s deadline and at most four attempts per read.
			wantGap, wantBackoff := 750*time.Millisecond, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
			if rig.path == "S-Dongle" {
				wantGap, wantBackoff = 5*time.Second, []time.Duration{5 * time.Second, 10 * time.Second, 15 * time.Second}
			}
			if pacing.Gap != wantGap || pacing.Cycle != time.Minute || pacing.AttemptDeadline != 5*time.Second || pacing.MaxAttempts != 4 ||
				!reflect.DeepEqual(pacing.Backoff, wantBackoff) || pacing.FirstRequestExtraAttempt != (rig.path == "S-Dongle") {
				t.Fatalf("pacing %+v is not the contract's", pacing)
			}
			calls := rig.recorder.Calls()
			// Between two requests on the connection lies at least the gap, or the
			// backoff before a repeat of a failed attempt.
			exchanges := []HuaweiProviderExchange{}
			for _, cycle := range cycles {
				exchanges = append(exchanges, cycle.Exchanges...)
			}
			if len(exchanges) != len(calls) {
				t.Fatalf("%d exchanges for %d calls", len(exchanges), len(calls))
			}
			for index := 1; index < len(calls); index++ {
				spacing := pacing.Gap
				attempt := exchanges[index].Attempt
				if attempt > 1 && pacing.Backoff[attempt-2] > spacing {
					spacing = pacing.Backoff[attempt-2]
				}
				if got := calls[index].sentAt.Sub(calls[index-1].endAt); got < spacing {
					t.Fatalf("request %d (%s) followed the previous reply after %s, want %s", index, calls[index].h06Expected, got, spacing)
				}
				if !exchanges[index].SentAt.Equal(calls[index].sentAt) {
					t.Fatalf("exchange %d time differs from the request time", index)
				}
			}
			// One connection, one request in flight: the scripted session saw the
			// requests strictly one after the other.
			for index := 1; index < len(calls); index++ {
				if calls[index].sentAt.Before(calls[index-1].endAt) {
					t.Fatalf("request %d overlapped the previous one", index)
				}
			}
			// Cycles start no earlier than the cycle interval after each other,
			// and a cycle that read for more than three quarters of it lengthened
			// the interval.
			for index := 1; index < len(cycles); index++ {
				previous := cycles[index-1]
				interval := pacing.Cycle
				if elapsed := previous.CompletedAt.Sub(previous.StartedAt); elapsed*4 > pacing.Cycle*3 {
					interval = (elapsed*4 + 2) / 3
				}
				if cycles[index].StartedAt.Sub(previous.StartedAt) < interval {
					t.Fatalf("cycle %d started %s after cycle %d, want %s", index, cycles[index].StartedAt.Sub(previous.StartedAt), index-1, interval)
				}
			}
			for _, asked := range rig.waits.Asked() {
				if asked <= 0 || asked > pacing.Backoff[len(pacing.Backoff)-1] {
					t.Fatalf("wait asked for %s", asked)
				}
			}
		})
	}
}

func TestH06APacingViolationOrAnEarlyRefreshSendsNothing(t *testing.T) {
	fixture, _ := h06Load(t, "smartlogger_provider_cycles.json")
	rig := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
	// A refresh before the cycle interval sends nothing.
	rig.refresh(t, h06Script(t, rig.sources, fixture.Path, fixture.Cycles[0])...)
	sent := len(rig.recorder.Calls())
	if _, err := rig.provider.Refresh(context.Background()); !errors.Is(err, ErrHuaweiProviderEarly) {
		t.Fatalf("early refresh: %v", err)
	}
	if len(rig.recorder.Calls()) != sent {
		t.Fatal("an early refresh sent a request")
	}
	// A wait that returns after half the gap stops the cycle before the next
	// request and commits nothing.
	rig.clock.Advance(rig.provider.NextRefreshAt().Sub(rig.clock.Now()))
	rig.waits.mu.Lock()
	rig.waits.num, rig.waits.den = 1, 2
	rig.waits.mu.Unlock()
	rig.session.Append(h06Script(t, rig.sources, fixture.Path, fixture.Cycles[1])...)
	before, _ := rig.provider.LastComplete()
	if _, err := rig.provider.Refresh(context.Background()); !errors.Is(err, ErrHuaweiProviderPacing) {
		t.Fatalf("lazy wait: %v", err)
	}
	if got := len(rig.recorder.Calls()); got != sent+1 {
		t.Fatalf("%d requests after the lazy wait, want only the guard read", got-sent)
	}
	if after, _ := rig.provider.LastComplete(); !reflect.DeepEqual(before, after) {
		t.Fatal("a cycle stopped by a pacing violation changed the last complete snapshot")
	}
	// A provider never constructs without the caller's wait.
	config := h06Config(rig.clock, rig.waits, nil)
	config.Wait = nil
	if _, err := NewHuaweiSmartLoggerReadOnlyProvider(config); !errors.Is(err, ErrHuaweiProviderConfig) {
		t.Fatalf("no wait: %v", err)
	}
}

func TestH06ConcurrentRefreshIsRefusedAndCloseFencesLateResults(t *testing.T) {
	for _, name := range h06FixtureNames() {
		t.Run(name, func(t *testing.T) {
			fixture, _ := h06Load(t, name)
			rig := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
			rig.refresh(t, h06Script(t, rig.sources, fixture.Path, fixture.Cycles[0])...)
			kept, _ := rig.provider.LastComplete()

			script := h06Script(t, rig.sources, fixture.Path, fixture.Cycles[1])
			entered, release := make(chan struct{}), make(chan struct{})
			script[0].answer.entered, script[0].answer.release = entered, release
			rig.session.Append(script...)
			rig.clock.Advance(rig.provider.NextRefreshAt().Sub(rig.clock.Now()))
			done := make(chan error, 1)
			go func() {
				_, err := rig.provider.Refresh(context.Background())
				done <- err
			}()
			<-entered
			if _, err := rig.provider.Refresh(context.Background()); !errors.Is(err, ErrHuaweiProviderBusy) {
				t.Errorf("concurrent refresh: %v", err)
			}
			if err := rig.provider.Close(); err != nil {
				t.Errorf("close: %v", err)
			}
			close(release)
			if err := <-done; !errors.Is(err, ErrHuaweiProviderClosed) {
				t.Fatalf("late refresh result: %v", err)
			}
			if got := rig.session.Closes(); got != 1 {
				t.Fatalf("session closed %d times", got)
			}
			if unexpected := rig.session.Unexpected(); len(unexpected) != 0 {
				t.Fatalf("requests after close: %v", unexpected)
			}
			if rig.session.Remaining() != len(script)-1 {
				t.Fatalf("%d requests were sent after the fenced one", len(script)-1-rig.session.Remaining())
			}
			if after, _ := rig.provider.LastComplete(); !reflect.DeepEqual(kept, after) {
				t.Fatal("a fenced cycle changed the last complete snapshot")
			}
			if _, err := rig.provider.Refresh(context.Background()); !errors.Is(err, ErrHuaweiProviderClosed) {
				t.Fatalf("refresh after close: %v", err)
			}
			if err := rig.provider.Start(newH06Session("late", rig.clock, rig.recorder)); !errors.Is(err, ErrHuaweiProviderClosed) {
				t.Fatalf("start after close: %v", err)
			}
		})
	}
}

func TestH06LastCompleteIsDetached(t *testing.T) {
	rig, _ := h06RunFixture(t, "smartlogger_provider_cycles.json")
	first, _ := rig.provider.LastComplete()
	first.Children[0].Groups[0].Offset = 1
	first.Exchanges[0].UnitID = 99
	first.Loss[0].Detail = "changed"
	again, _ := rig.provider.LastComplete()
	if again.Children[0].Groups[0].Offset == 1 || again.Exchanges[0].UnitID == 99 || again.Loss[0].Detail == "changed" {
		t.Fatal("the last complete snapshot is not detached")
	}
}

func TestH06CancelledCycleCommitsNoReading(t *testing.T) {
	fixture, _ := h06Load(t, "sdongle_provider_cycles.json")
	rig := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
	rig.refresh(t, h06Script(t, rig.sources, fixture.Path, fixture.Cycles[0])...)
	kept, _ := rig.provider.LastComplete()
	ctx, cancel := context.WithCancel(context.Background())
	script := h06Script(t, rig.sources, fixture.Path, fixture.Cycles[1])
	entered, release := make(chan struct{}), make(chan struct{})
	script[1].answer.entered, script[1].answer.release = entered, release
	rig.session.Append(script...)
	rig.clock.Advance(rig.provider.NextRefreshAt().Sub(rig.clock.Now()))
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
	if after, _ := rig.provider.LastComplete(); !reflect.DeepEqual(kept, after) {
		t.Fatal("a cancelled cycle changed the last complete snapshot")
	}
	if rig.session.Remaining() != len(script)-2 {
		t.Fatalf("requests were sent after the cancellation: %d left of %d", rig.session.Remaining(), len(script))
	}
	// A cancelled cycle still counts against the cycle interval.
	if _, err := rig.provider.Refresh(context.Background()); !errors.Is(err, ErrHuaweiProviderEarly) {
		t.Fatalf("refresh right after a cancelled cycle: %v", err)
	}
}
