package modbusreg

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// The M89 tests (helianthus-modbusreg issue 365) cover the child admission of
// the read-only providers: the identity words read at the routed unit, the
// catalogue decision over them, and what the provider records and reads for
// each outcome.

// m89IdentityOffsets are the admission reads in the order the providers send
// them; the model text is first on the SmartLogger path only.
var m89IdentityOffsets = map[uint16]bool{huaweiSUN2000ModelOffset: true, huaweiSUN2000IdentifierOffset: true, huaweiSUN2000SoftwareOffset: true, huaweiSUN2000FirmwareOffset: true}

func m89Text(value string) []uint16 { return h06BlockWords(h05RoutedModelBlock(value)) }

// m89FixtureText decodes the text of a probe of an identity fixture.
func m89FixtureText(t testing.TB, set h03ProbeFixture, unit int, offset uint16) string {
	t.Helper()
	text, ok := parseHuaweiSUN2000TextBlock(huaweiWordsToBytes(h15Words(t, h15ProbeAt(t, set, unit, offset))), huaweiSUN2000TextBlockBytes)
	if !ok {
		t.Fatalf("probe at unit %d offset %d is not a text block", unit, offset)
	}
	return text
}

// TestM89OwnedFamiliesAdmitThroughTheCatalogueSelection replays the scripted
// sessions of the two owned families: each child is admitted on the catalogue
// row its identity words select, and the observation records the row, its
// edition, page, floor and grade, the identifier and the texts as read.
func TestM89OwnedFamiliesAdmitThroughTheCatalogueSelection(t *testing.T) {
	sources := h06LoadSources(t)
	for _, test := range []struct {
		fixture, profile, decisionCase string
		units                          []uint8
		card                           string
		identifier                     uint16
		firmware                       string
	}{
		{"smartlogger_provider_cycles.json", HuaweiSUN2000M3MonitoringProfileID, "m3_series_identifier_selects_the_mc_card", []uint8{1, 2}, "MC", 471,
			m89FixtureText(t, sources.routed, 1, huaweiSUN2000FirmwareOffset)},
		{"sdongle_provider_cycles.json", HuaweiSUN2000M1MonitoringProfileID, "m1_series_identifier_selects_the_ma_c00_card", []uint8{1}, "MA-C00", 428,
			m89FixtureText(t, sources.sdongle.ChildBlocks, 1, huaweiSUN2000FirmwareOffset)},
	} {
		t.Run(test.fixture, func(t *testing.T) {
			_, cycles := h06RunFixture(t, test.fixture)
			software := m85Case(t, test.decisionCase).Input.SoftwareText
			for _, cycle := range cycles {
				for _, unit := range test.units {
					child, ok := cycle.Child(unit)
					if !ok || child.Catalogue == nil {
						t.Fatalf("cycle %d unit %d carries no catalogue decision: %+v", cycle.Cycle, unit, child)
					}
					decision := *child.Catalogue
					row, selected := decision.Row()
					identifier, read := decision.Identifier()
					build, from := decision.Build()
					if !selected || decision.Outcome() != HuaweiSUN2000CatalogueSelected || decision.Reason() != HuaweiSUN2000CatalogueAtOrAboveFloor ||
						decision.Grade() != HuaweiGradeHypothesis || row.Card != test.card || row.Identifier != int(test.identifier) ||
						row.MatchedBy != HuaweiSUN2000CatalogueMatchToken || row.FloorRelation != HuaweiSUN2000FloorAtOrAbove ||
						!read || identifier != test.identifier || build != software || from != HuaweiSUN2000BuildSoftwareText ||
						decision.FirmwareText() != test.firmware || decision.Scope() != HuaweiSUN2000ScopeInverterMap ||
						!reflect.DeepEqual(m85Rows(decision.Candidates()), []m85Row{{test.card, int(test.identifier)}}) {
						t.Fatalf("cycle %d unit %d decision %s %s row %+v build %q %s firmware %q", cycle.Cycle, unit, decision.Outcome(), decision.Reason(),
							row, build, from, decision.FirmwareText())
					}
					m85RequireRowFacts(t, row)
					if child.ProfileID != "" && child.ProfileID != test.profile {
						t.Fatalf("unit %d profile %s", unit, child.ProfileID)
					}
				}
			}
			first, _ := cycles[0].Child(test.units[0])
			if first.Evidence != HuaweiProviderEvidenceCurrent || first.Reason != HuaweiProviderReasonRead || first.ProfileID != test.profile {
				t.Fatalf("first cycle: %+v", first)
			}
		})
	}
}

// TestM89IdentityWordsAreReadAtAdmissionOnly counts the identity reads of every
// scripted session: one admission per child per cycle, in the documented
// order, three identity reads besides the model text, none for a child that a
// guard-held cycle finds admitted, and again after a guard change.
func TestM89IdentityWordsAreReadAtAdmissionOnly(t *testing.T) {
	for _, name := range h06FixtureNames() {
		t.Run(name, func(t *testing.T) {
			fixture, _ := h06Load(t, name)
			_, cycles := h06RunFixture(t, name)
			order := []uint16{huaweiSUN2000ModelOffset, huaweiSUN2000IdentifierOffset, huaweiSUN2000SoftwareOffset, huaweiSUN2000FirmwareOffset}
			if fixture.Path == "S-Dongle" {
				// The model text at the child is part of the card tuple.
				order = order[1:]
			}
			admitted := map[uint8]bool{}
			admissions, steady := 0, 0
			for _, cycle := range cycles {
				firsts := map[uint8][]uint16{}
				for _, exchange := range cycle.Exchanges {
					if exchange.UnitID == 0 || exchange.UnitID == 100 || exchange.Function != 0x03 || !m89IdentityOffsets[exchange.Offset] || exchange.Attempt != 1 {
						continue
					}
					if fixture.Path == "S-Dongle" && exchange.Offset == huaweiSUN2000ModelOffset {
						continue
					}
					firsts[exchange.UnitID] = append(firsts[exchange.UnitID], exchange.Offset)
				}
				for unit, offsets := range firsts {
					if len(offsets) > len(order) || !reflect.DeepEqual(offsets, order[:len(offsets)]) {
						t.Fatalf("cycle %d unit %d identity reads %v, want a prefix of %v", cycle.Cycle, unit, offsets, order)
					}
					if cycle.Reason == HuaweiProviderReasonGuardHeld && admitted[unit] {
						t.Fatalf("cycle %d read the identity words of admitted unit %d", cycle.Cycle, unit)
					}
					if len(offsets) == len(order) {
						admissions++
					}
				}
				for _, child := range cycle.Children {
					if cycle.Reason == HuaweiProviderReasonGuardHeld && admitted[child.UnitID] && len(firsts[child.UnitID]) == 0 {
						steady++
					}
					if cycle.Reason == HuaweiProviderReasonIdentified && child.Evidence == HuaweiProviderEvidenceCurrent && len(firsts[child.UnitID]) != len(order) {
						t.Fatalf("cycle %d unit %d read without a full admission: %v", cycle.Cycle, child.UnitID, firsts[child.UnitID])
					}
					admitted[child.UnitID] = child.ProfileID != "" && child.IdentifiedGeneration == cycle.Generation && child.Evidence != HuaweiProviderEvidenceWithheld
				}
				if cycle.Reason == HuaweiProviderReasonGuardChanged && len(firsts) != 0 {
					t.Fatalf("cycle %d read identity words after a guard change", cycle.Cycle)
				}
			}
			if admissions == 0 || steady == 0 {
				t.Fatalf("admissions=%d steady child cycles=%d", admissions, steady)
			}
		})
	}
}

// TestM89AdmissionOutcomesAreRecordedPerChild checks the admission sessions:
// an identity read that times out leaves the child unadmitted with nothing
// guessed and is retried on the next cycle; a build of an unrelated branch is
// admitted as Hypothesis; an identifier of another row is withheld with the
// closed reason, and an ambiguous decision is withheld naming its candidates.
// No monitoring group is read at a withheld child.
func TestM89AdmissionOutcomesAreRecordedPerChild(t *testing.T) {
	withheldDetail := func(cycle HuaweiProviderCycle, unit uint8) string {
		for _, loss := range cycle.Loss {
			if loss.Kind == HuaweiProviderLossWithheldChild && loss.HasUnit && loss.UnitID == unit {
				return loss.Detail
			}
		}
		return ""
	}
	groupsRead := func(cycle HuaweiProviderCycle, unit uint8) bool {
		for _, exchange := range cycle.Exchanges {
			if exchange.UnitID == unit && exchange.Offset >= 32000 {
				return true
			}
		}
		return false
	}

	t.Run("SmartLogger", func(t *testing.T) {
		_, cycles := h06RunFixture(t, "smartlogger_provider_admission_cycles.json")
		first, retried := cycles[0], cycles[1]

		timedOut, _ := first.Child(1)
		var attempts []string
		for _, exchange := range first.Exchanges {
			if exchange.UnitID == 1 {
				attempts = append(attempts, fmt.Sprintf("%d:%d:%s", exchange.Offset, exchange.Attempt, exchange.Outcome))
			}
		}
		if timedOut.Catalogue != nil || timedOut.ProfileID != "" || timedOut.Reason != HuaweiProviderReasonChildTimeout ||
			!reflect.DeepEqual(attempts, []string{"30000:1:ANSWERED", "30070:1:TIMEOUT", "30070:2:TIMEOUT", "30070:3:TIMEOUT", "30070:4:TIMEOUT"}) ||
			withheldDetail(first, 1) != string(HuaweiProviderReasonChildTimeout) {
			t.Fatalf("identifier timeout: %+v attempts %v", timedOut, attempts)
		}
		if again, _ := retried.Child(1); again.Evidence != HuaweiProviderEvidenceCurrent || again.Catalogue == nil || again.IdentifiedGeneration != retried.Generation {
			t.Fatalf("the admission was not retried: %+v", again)
		}

		unrelated, _ := first.Child(2)
		row, _ := unrelated.Catalogue.Row()
		if unrelated.Evidence != HuaweiProviderEvidenceCurrent || unrelated.Catalogue.Reason() != HuaweiSUN2000CatalogueNoDocumentedRelation ||
			unrelated.Catalogue.Grade() != HuaweiGradeHypothesis || row.FloorRelation != HuaweiSUN2000FloorNoDocumentedRelation || row.Card != "MC" {
			t.Fatalf("unrelated branch: %+v %+v", unrelated, row)
		}
		m85RequireRowFacts(t, row)

		for _, cycle := range cycles {
			mismatch, _ := cycle.Child(3)
			ambiguous, _ := cycle.Child(4)
			if mismatch.Catalogue == nil || mismatch.Catalogue.Reason() != HuaweiSUN2000CatalogueIdentifierMismatch || len(mismatch.Reading.Facts()) != 0 ||
				ambiguous.Catalogue == nil || ambiguous.Catalogue.Outcome() != HuaweiSUN2000CatalogueAmbiguous || len(ambiguous.Reading.Facts()) != 0 {
				t.Fatalf("cycle %d: mismatch %+v ambiguous %+v", cycle.Cycle, mismatch, ambiguous)
			}
			if groupsRead(cycle, 3) || groupsRead(cycle, 4) {
				t.Fatalf("cycle %d read a withheld child", cycle.Cycle)
			}
			if cycle.Reason == HuaweiProviderReasonGuardChanged {
				continue
			}
			if withheldDetail(cycle, 3) != "catalogue_not_selected: identifier_mismatch" ||
				withheldDetail(cycle, 4) != "catalogue_ambiguous: MA-C00 449, MA-C20 449" {
				t.Fatalf("cycle %d withheld details %q %q", cycle.Cycle, withheldDetail(cycle, 3), withheldDetail(cycle, 4))
			}
			if _, selected := ambiguous.Catalogue.Row(); selected {
				t.Fatalf("cycle %d: an ambiguous decision selected a row", cycle.Cycle)
			}
		}
	})

	t.Run("S-Dongle", func(t *testing.T) {
		_, cycles := h06RunFixture(t, "sdongle_provider_admission_cycles.json")
		timedOut, _ := cycles[0].Child(1)
		if timedOut.Catalogue != nil || timedOut.ProfileID != "" || timedOut.Reason != HuaweiProviderReasonChildTimeout || groupsRead(cycles[0], 1) {
			t.Fatalf("identifier timeout: %+v", timedOut)
		}
		for _, cycle := range cycles[1:4] {
			child, _ := cycle.Child(1)
			if child.Catalogue == nil || child.Catalogue.Reason() != HuaweiSUN2000CatalogueIdentifierMismatch || groupsRead(cycle, 1) || len(child.Reading.Facts()) != 0 {
				t.Fatalf("cycle %d mismatch: %+v", cycle.Cycle, child)
			}
		}
		if len(cycles[2].Exchanges) != 1 {
			t.Fatalf("a withheld child was read in a guard-held cycle: %+v", cycles[2].Exchanges)
		}
		admitted, _ := cycles[4].Child(1)
		if admitted.Evidence != HuaweiProviderEvidenceCurrent || admitted.Catalogue == nil || admitted.Catalogue.Outcome() != HuaweiSUN2000CatalogueSelected {
			t.Fatalf("re-identification: %+v", admitted)
		}
	})
}

// TestM89BuildBelowTheFloorIsWithheld runs a SmartLogger child whose listing
// and routed model name a catalogue row with a build floor. The two owned rows
// state their floor as a branch without a build number, so no build of their
// branch is below it; the rows with a build floor are on the LB-C10 card. A
// build below the floor is withheld with the closed reason and is not read
// again until the next identification event; the build at the floor selects
// the row, which has no monitoring profile on this path.
func TestM89BuildBelowTheFloorIsWithheld(t *testing.T) {
	for _, owned := range []struct {
		card       string
		identifier int
	}{{"MC", 471}, {"MA-C00", 428}} {
		floor, ok := parseHuaweiSUN2000Version(m85CatalogueRowOf(t, owned.card, owned.identifier).Floor)
		if !ok || floor.hasBuild {
			t.Fatalf("%s %d: the floor carries a build number; a below-floor build of its branch exists", owned.card, owned.identifier)
		}
	}
	const unit = 3
	for _, test := range []struct {
		decisionCase string
		reason       HuaweiProviderReason
		summary      string
		detail       string
	}{
		{"build_below_the_floor_is_not_selected", HuaweiProviderReasonCatalogueNotSelected, "NOT_SELECTED below_floor LB-C10/380", "catalogue_not_selected: below_floor"},
		{"build_at_the_floor_is_selected", HuaweiProviderReasonNoProfile, "SELECTED at_or_above_floor LB-C10/380", string(HuaweiProviderReasonNoProfile)},
	} {
		t.Run(test.decisionCase, func(t *testing.T) {
			model := m85Case(t, test.decisionCase).Input.ModelText
			sources := h06LoadSources(t)
			inventory := sources.smartLoggerInventory
			inventory.Pages = append([]h03InventoryPage(nil), inventory.Pages...)
			rewritten := 0
			for pageIndex := range inventory.Pages {
				inventory.Pages[pageIndex].Objects = append([]h03Object(nil), inventory.Pages[pageIndex].Objects...)
				for objectIndex := range inventory.Pages[pageIndex].Objects {
					object := &inventory.Pages[pageIndex].Objects[objectIndex]
					if strings.Contains(object.Entry, fmt.Sprintf(";5=%d;", unit)) {
						object.Entry = strings.Replace(object.Entry, "1="+huaweiSUN2000M3Model, "1="+model, 1)
						rewritten++
					}
				}
			}
			if rewritten != 1 {
				t.Fatalf("rewrote %d listing entries", rewritten)
			}
			sources.smartLoggerInventory = inventory
			rig := newH06Rig(t, "SmartLogger", []uint8{unit})
			rig.sources = sources
			var script []h06Expected
			for _, step := range h06IdentificationSteps() {
				script = append(script, h06Script(t, sources, "SmartLogger", h06Cycle{Steps: []h06Step{step}})...)
			}
			script = append(script, h06Script(t, sources, "SmartLogger", h06Cycle{Steps: []h06Step{
				{Function: "FC03", Unit: unit, Offset: huaweiSUN2000ModelOffset, Quantity: huaweiSUN2000ModelQuantity, Reply: h06Reply{Kind: "source", Case: test.decisionCase}},
			}})...)
			script = append(script, h06AdmissionWords(t, sources, "SmartLogger", unit, test.decisionCase)...)
			cycle := rig.refresh(t, script...)
			child, _ := cycle.Child(unit)
			if child.Evidence != HuaweiProviderEvidenceWithheld || child.Reason != test.reason || h06CatalogueSummary(child.Catalogue) != test.summary ||
				child.ProfileID != "" || len(child.Reading.Facts()) != 0 {
				t.Fatalf("child: %s %s %q", child.Evidence, child.Reason, h06CatalogueSummary(child.Catalogue))
			}
			candidates := child.Catalogue.Candidates()
			if len(candidates) != 1 || candidates[0].Floor != "V300R024C10SPC100" {
				t.Fatalf("candidates %+v", candidates)
			}
			if test.reason == HuaweiProviderReasonCatalogueNotSelected && candidates[0].FloorRelation != HuaweiSUN2000FloorBelow {
				t.Fatalf("floor relation %s", candidates[0].FloorRelation)
			}
			found := false
			for _, loss := range cycle.Loss {
				found = found || (loss.Kind == HuaweiProviderLossWithheldChild && loss.UnitID == unit && loss.Detail == test.detail)
			}
			if !found {
				t.Fatalf("no withheld loss %q: %+v", test.detail, cycle.Loss)
			}
			// The guard holds: the withheld child is neither read nor admitted.
			next := rig.refresh(t, h06FC03(0, huaweiSmartLoggerCounterOffset, 1, h06Answer{words: []uint16{sources.smartLoggerInventory.ChangeCounter.Before}}))
			if again, _ := next.Child(unit); again.Reason != test.reason || h06CatalogueSummary(again.Catalogue) != test.summary || len(next.Exchanges) != 1 {
				t.Fatalf("next cycle: %+v, %d exchanges", again, len(next.Exchanges))
			}
		})
	}
}

// TestM89AdmissionVariesEveryIdentityWord varies each word of the admission at
// one SmartLogger child, one at a time, from the case that selects the MC row.
// The fields and the production mutation each one catches:
//   - model text (30000): a model that disagrees with the listing, or no text,
//     withholds; catches a selection fed the listed model instead of the read.
//   - model identifier (30070): another row's identifier, an identifier shared
//     by two rows, or none of a row; catches the identifier not passed to the
//     selection (the baseline then turns ambiguous) and an ambiguous decision
//     admitted.
//   - software text (30050): an unrelated branch, another build of the floor's
//     branch, a malformed text and no text; catches the software text not
//     passed (the firmware text would then decide on its carried parts).
//   - firmware text (30035): another text and no text; catches the firmware
//     text not recorded with the decision.
func TestM89AdmissionVariesEveryIdentityWord(t *testing.T) {
	const unit = 1
	base := map[uint16][]uint16{
		huaweiSUN2000ModelOffset:      m89Text(huaweiSUN2000M3Model),
		huaweiSUN2000IdentifierOffset: {471},
		huaweiSUN2000SoftwareOffset:   m89Text("V200R023C00SPC100"),
		huaweiSUN2000FirmwareOffset:   m89Text("V200R023"),
	}
	blank := make([]uint16, huaweiSUN2000TextQuantity)
	for _, tc := range []struct {
		name     string
		offset   uint16
		words    []uint16
		reason   HuaweiProviderReason
		summary  string
		firmware string
	}{
		{"baseline", 0, nil, HuaweiProviderReasonRead, "SELECTED at_or_above_floor MC/471", "V200R023"},
		{"model of another family", huaweiSUN2000ModelOffset, m89Text("SUN2000-50KTL-M4"), HuaweiProviderReasonIdentityMismatch, "", ""},
		{"model without text", huaweiSUN2000ModelOffset, blank, HuaweiProviderReasonIdentityInsuff, "", ""},
		{"identifier of another row", huaweiSUN2000IdentifierOffset, []uint16{470}, HuaweiProviderReasonCatalogueNotSelected,
			"NOT_SELECTED identifier_mismatch MA-C00/449,MA-C20/449,MC/471", ""},
		{"identifier of two rows", huaweiSUN2000IdentifierOffset, []uint16{449}, HuaweiProviderReasonCatalogueAmbiguous,
			"AMBIGUOUS multiple_candidate_rows MA-C00/449,MA-C20/449", ""},
		{"identifier of no row", huaweiSUN2000IdentifierOffset, []uint16{0}, HuaweiProviderReasonCatalogueNotSelected,
			"NOT_SELECTED identifier_mismatch MA-C00/449,MA-C20/449,MC/471", ""},
		{"software of an unrelated branch", huaweiSUN2000SoftwareOffset, m89Text("V200R025C00SPC100"), HuaweiProviderReasonRead,
			"SELECTED no_documented_branch_relation MC/471", "V200R023"},
		{"software of another build", huaweiSUN2000SoftwareOffset, m89Text("V200R023C00SPC001"), HuaweiProviderReasonRead,
			"SELECTED at_or_above_floor MC/471", "V200R023"},
		{"software outside the branch form", huaweiSUN2000SoftwareOffset, m89Text("V200R023C0"), HuaweiProviderReasonCatalogueNotSelected,
			"NOT_SELECTED software_malformed", ""},
		{"software without text", huaweiSUN2000SoftwareOffset, blank, HuaweiProviderReasonIdentityInsuff, "", ""},
		{"another firmware text", huaweiSUN2000FirmwareOffset, m89Text("V200R024"), HuaweiProviderReasonRead, "SELECTED at_or_above_floor MC/471", "V200R024"},
		{"firmware without text", huaweiSUN2000FirmwareOffset, blank, HuaweiProviderReasonIdentityInsuff, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := newH06Rig(t, "SmartLogger", []uint8{unit})
			var script []h06Expected
			for _, step := range h06IdentificationSteps() {
				script = append(script, h06Script(t, rig.sources, "SmartLogger", h06Cycle{Steps: []h06Step{step}})...)
			}
			for _, offset := range []uint16{huaweiSUN2000ModelOffset, huaweiSUN2000IdentifierOffset, huaweiSUN2000SoftwareOffset, huaweiSUN2000FirmwareOffset} {
				words := base[offset]
				if offset == tc.offset {
					words = tc.words
				}
				script = append(script, h06FC03(unit, offset, uint16(len(base[offset])), h06Answer{words: words}))
			}
			if tc.reason == HuaweiProviderReasonRead {
				for _, step := range h05Profile(t, HuaweiSUN2000M3MonitoringProfileID).Plan(unit) {
					script = append(script, h06FC03(unit, step.Offset, step.Quantity, h06MonitoringAnswer(t, rig.sources.m3, unit, step.Offset, step.Quantity)))
				}
			}
			cycle := rig.refresh(t, script...)
			child, _ := cycle.Child(unit)
			if child.Reason != tc.reason || h06CatalogueSummary(child.Catalogue) != tc.summary {
				t.Fatalf("child %s %s %q, want %s %q", child.Evidence, child.Reason, h06CatalogueSummary(child.Catalogue), tc.reason, tc.summary)
			}
			if (child.Evidence == HuaweiProviderEvidenceCurrent) != (tc.reason == HuaweiProviderReasonRead) || (child.ProfileID != "") != (tc.reason == HuaweiProviderReasonRead) {
				t.Fatalf("admission %s profile %q", child.Evidence, child.ProfileID)
			}
			if tc.reason != HuaweiProviderReasonRead {
				return
			}
			identifier, read := child.Catalogue.Identifier()
			build, from := child.Catalogue.Build()
			software, _ := parseHuaweiSUN2000TextBlock(huaweiWordsToBytes(base[huaweiSUN2000SoftwareOffset]), huaweiSUN2000TextBlockBytes)
			if tc.offset == huaweiSUN2000SoftwareOffset {
				software, _ = parseHuaweiSUN2000TextBlock(huaweiWordsToBytes(tc.words), huaweiSUN2000TextBlockBytes)
			}
			if !read || identifier != 471 || build != software || from != HuaweiSUN2000BuildSoftwareText || child.Catalogue.FirmwareText() != tc.firmware ||
				child.Catalogue.Grade() != HuaweiGradeHypothesis {
				t.Fatalf("recorded identifier %d %v build %q %s firmware %q", identifier, read, build, from, child.Catalogue.FirmwareText())
			}
		})
	}
}

// TestM89AProfileIsBoundOnlyToASelectedRow hands the binding a selection that
// names a profile without a selected catalogue row: the provider withholds the
// child instead of trusting the selection outcome alone.
func TestM89AProfileIsBoundOnlyToASelectedRow(t *testing.T) {
	profile := h05Profile(t, HuaweiSUN2000M3MonitoringProfileID)
	ambiguous := DecideHuaweiSUN2000CatalogueSelection(m85Case(t, "m3_series_token_without_identifier_is_ambiguous").Input.catalogueInput())
	selected := DecideHuaweiSUN2000CatalogueSelection(m85Case(t, "m3_series_identifier_selects_the_mc_card").Input.catalogueInput())
	for name, tc := range map[string]struct {
		decision *HuaweiSUN2000CatalogueDecision
		bound    bool
	}{
		"selected row":       {&selected, true},
		"ambiguous decision": {&ambiguous, false},
		"no decision":        {nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			run := &huaweiCycleRun{state: &huaweiProviderState{children: map[uint8]*huaweiChildState{}}, generation: 1}
			child := run.state.child(1)
			selection := HuaweiSUN2000CapabilitySelection{outcome: HuaweiSUN2000ProfileSelected, reason: HuaweiSUN2000ReasonMatched, unitID: 1, profile: &profile, catalogue: tc.decision}
			if bound := run.bindChildSelection(child, selection); bound != tc.bound || (child.identity != nil) != tc.bound {
				t.Fatalf("bound=%v identity=%v", bound, child.identity)
			}
			if !tc.bound && (child.definitive != HuaweiProviderReasonIdentityInsuff || len(run.out.Children) != 1) {
				t.Fatalf("withheld as %q with %d observations", child.definitive, len(run.out.Children))
			}
		})
	}
}

// TestM89AnIdentificationEventDropsTheDecisionOfAWithheldChild withholds a
// child on its decision, changes the guard and times out the identifier read
// of the next admission: the child is unadmitted with no decision, because the
// earlier decision no longer describes it.
func TestM89AnIdentificationEventDropsTheDecisionOfAWithheldChild(t *testing.T) {
	fixture, _ := h06Load(t, "smartlogger_provider_admission_cycles.json")
	rig := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
	for _, cycle := range fixture.Cycles[:3] {
		rig.refresh(t, h06Script(t, rig.sources, fixture.Path, cycle)...)
	}
	var script []h06Expected
	for _, step := range h06IdentificationSteps() {
		script = append(script, h06Script(t, rig.sources, fixture.Path, h06Cycle{Steps: []h06Step{step}})...)
	}
	for _, unit := range []uint8{1, 2} {
		script = append(script, h06Script(t, rig.sources, fixture.Path, h06Cycle{Steps: []h06Step{
			{Function: "FC03", Unit: unit, Offset: huaweiSUN2000ModelOffset, Quantity: huaweiSUN2000ModelQuantity, Reply: h06Reply{Kind: "source"}},
		}})...)
		script = append(script, h06AdmissionWords(t, rig.sources, fixture.Path, unit, "m3_series_identifier_selects_the_mc_card")...)
		for _, step := range h05Profile(t, HuaweiSUN2000M3MonitoringProfileID).Plan(unit) {
			script = append(script, h06FC03(unit, step.Offset, step.Quantity, h06MonitoringAnswer(t, rig.sources.m3, unit, step.Offset, step.Quantity)))
		}
	}
	script = append(script, h06Script(t, rig.sources, fixture.Path, h06Cycle{Steps: []h06Step{
		{Function: "FC03", Unit: 3, Offset: huaweiSUN2000ModelOffset, Quantity: huaweiSUN2000ModelQuantity, Reply: h06Reply{Kind: "source"}},
	}})...)
	for attempt := 0; attempt < 4; attempt++ {
		script = append(script, h06FC03(3, huaweiSUN2000IdentifierOffset, 1, h06Answer{err: h06Timeout()}))
	}
	script = append(script, h06Script(t, rig.sources, fixture.Path, h06Cycle{Steps: []h06Step{
		{Function: "FC03", Unit: 4, Offset: huaweiSUN2000ModelOffset, Quantity: huaweiSUN2000ModelQuantity, Reply: h06Reply{Kind: "source"}},
	}})...)
	script = append(script, h06AdmissionWords(t, rig.sources, fixture.Path, 4, "m3_series_token_with_identifier_449_stays_ambiguous_between_two_cards")...)
	cycle := rig.refresh(t, script...)
	child, _ := cycle.Child(3)
	if child.Evidence != HuaweiProviderEvidenceWithheld || child.Reason != HuaweiProviderReasonChildTimeout || child.Catalogue != nil {
		t.Fatalf("child 3 after the timed-out admission: %s %s %q", child.Evidence, child.Reason, h06CatalogueSummary(child.Catalogue))
	}
	if admitted, _ := cycle.Child(2); admitted.Catalogue == nil || admitted.Catalogue.Reason() != HuaweiSUN2000CatalogueAtOrAboveFloor {
		t.Fatalf("child 2 keeps the decision of its earlier admission: %q", h06CatalogueSummary(admitted.Catalogue))
	}
}

// TestM89AGuardChangeMakesTheAdmissionRunAgainAfterAFailedRead changes the
// S-Dongle guard after the child was admitted and times out the identifier
// read of the re-identification: the child keeps its reading as stale history,
// and the next cycle with the guard held runs the admission again before any
// group is read.
func TestM89AGuardChangeMakesTheAdmissionRunAgainAfterAFailedRead(t *testing.T) {
	fixture, start := h06FixtureCycle(t, "sdongle_provider_cycles.json", "start_identification")
	_, changed := h06FixtureCycle(t, "sdongle_provider_cycles.json", "guard_changed")
	_, reidentify := h06FixtureCycle(t, "sdongle_provider_cycles.json", "reidentification")
	rig := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
	rig.refresh(t, h06Script(t, rig.sources, fixture.Path, start)...)
	rig.refresh(t, h06Script(t, rig.sources, fixture.Path, changed)...)
	var tuple []h06Step
	for _, step := range reidentify.Steps {
		if step.Unit == 1 && step.Offset >= huaweiSUN2000FirmwareOffset {
			continue
		}
		tuple = append(tuple, step)
	}
	script := h06Script(t, rig.sources, fixture.Path, h06Cycle{Steps: tuple})
	for attempt := 0; attempt < 4; attempt++ {
		script = append(script, h06FC03(1, huaweiSUN2000IdentifierOffset, 1, h06Answer{err: h06Timeout()}))
	}
	failed := rig.refresh(t, script...)
	if child, _ := failed.Child(1); child.Evidence != HuaweiProviderEvidenceStale || child.Reason != HuaweiProviderReasonChildTimeout {
		t.Fatalf("re-identification with a timed-out identifier: %s %s", child.Evidence, child.Reason)
	}
	guard := h06Script(t, rig.sources, fixture.Path, h06Cycle{Steps: start.Steps[1:2]})
	retry := append(guard, h06AdmissionWords(t, rig.sources, fixture.Path, 1, "m1_series_identifier_selects_the_ma_c00_card")...)
	for _, step := range h05Profile(t, HuaweiSUN2000M1MonitoringProfileID).Plan(1) {
		retry = append(retry, h06FC03(1, step.Offset, step.Quantity, h06MonitoringAnswer(t, rig.sources.m1, 1, step.Offset, step.Quantity)))
	}
	again := rig.refresh(t, retry...)
	if child, _ := again.Child(1); again.Reason != HuaweiProviderReasonGuardHeld || child.Evidence != HuaweiProviderEvidenceCurrent ||
		child.IdentifiedGeneration != again.Generation || child.Catalogue == nil {
		t.Fatalf("retried admission: %s %+v", again.Reason, child)
	}
}
