package fronius

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/internal/sunspecfixture"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
)

func TestFroniusQualificationCardPinsBoundedReadOnlyObservedFlavor(t *testing.T) {
	card := NewSunSpecFroniusQualificationCard()
	if card.ID() != SunSpecFroniusQualificationCardID ||
		card.FlavorID() != SunSpecFroniusObservedFlavorV11ID ||
		card.SchemaRevision() != sunspec.ModelsRevisionV1 ||
		card.Manufacturer() != "Fronius" || card.Model() != "Symo GEN24 10.0" ||
		card.Firmware() != "1.41.11-1" || card.UnitID() != 1 ||
		card.Function() != modbusreg.FunctionReadHoldingRegisters || !card.ReadOnly() {
		t.Fatalf("unexpected qualification card: %#v", card)
	}
	if got := card.BaseCandidates(); !reflect.DeepEqual(got, []uint16{40000}) {
		t.Fatalf("base candidates=%v", got)
	}
	if limits := card.Limits(); limits != (sunspec.ChainLimits{MaxTotalWords: 512, MaxOccurrences: 64}) {
		t.Fatalf("limits=%#v", limits)
	}
	wantFacts := []string{
		"inverter.ac.current.total", "inverter.ac.current.phase_a", "inverter.ac.current.phase_b", "inverter.ac.current.phase_c",
		"inverter.ac.voltage.phase_a", "inverter.ac.voltage.phase_b", "inverter.ac.voltage.phase_c",
		"inverter.ac.power.active", "inverter.ac.frequency", "inverter.ac.energy_lifetime",
		"inverter.temperature.cabinet", "inverter.operating_state", "inverter.events.1", "inverter.events.2",
		"inverter.ac.power.apparent", "inverter.ac.power.reactive",
		"inverter.ac.voltage.line_ab", "inverter.ac.voltage.line_bc", "inverter.ac.voltage.line_ca",
		"mppt.module.dc_energy", "mppt.module.dc_voltage", "mppt.module.dc_current", "mppt.module.dc_power",
	}
	if got := card.SemanticCandidateFieldIDs(); !reflect.DeepEqual(got, wantFacts) {
		t.Fatalf("semantic candidates=%v", got)
	}
	got := card.ExpectedChain()
	got[0] = sunspec.WireKey{}
	if !reflect.DeepEqual(card.ExpectedChain(), froniusObservedChainV11) {
		t.Fatal("expected chain leaked mutation")
	}
	base := card.BaseCandidates()
	base[0] = 0
	if card.BaseCandidates()[0] != 40000 {
		t.Fatal("base candidates leaked mutation")
	}
}

func TestFroniusQualificationCardReplaysCompleteSourceBackedChain(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	card := NewSunSpecFroniusQualificationCard()
	raw := sunspecfixture.RawWords(froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1").Occurrences())
	snapshot, err := replayFroniusQualificationCard(t, card, registry, raw)
	if err != nil {
		t.Fatalf("complete replay: %v", err)
	}
	observation, err := card.Qualify(registry, snapshot)
	if err != nil {
		t.Fatalf("Qualify: %v", err)
	}
	if !observation.Capability().Admitted() || observation.Flavor().FlavorID() != SunSpecFroniusObservedFlavorV11ID {
		t.Fatalf("capability=%#v flavor=%#v", observation.Capability(), observation.Flavor())
	}
	if len(observation.RawWords()) != len(raw) || len(observation.SourceViews()) <= len(observation.Occurrences()) {
		t.Fatalf("raw=%d views=%d occurrences=%d", len(observation.RawWords()), len(observation.SourceViews()), len(observation.Occurrences()))
	}
	for _, occurrence := range observation.Occurrences() {
		if len(occurrence.Words()) == 0 || len(occurrence.SourceSpans()) == 0 {
			t.Fatalf("occurrence lost raw/provenance: %#v", occurrence)
		}
	}
	facts := capabilityFieldIDs(observation.Capability().Facts())
	candidates := card.SemanticCandidateFieldIDs()
	if len(candidates) != len(facts)+len(sunSpecFroniusMPPTCandidateFieldIDs) || !reflect.DeepEqual(candidates[:len(facts)], facts) || !reflect.DeepEqual(candidates[len(facts):], sunSpecFroniusMPPTCandidateFieldIDs) {
		t.Fatalf("typed facts=%v candidates=%v", facts, candidates)
	}
}

func TestFroniusQualificationCardCompleteChainNegativeControls(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	card := NewSunSpecFroniusQualificationCard()
	baseOccurrences := froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1").Occurrences()

	unknown := append([]sunspec.Occurrence(nil), baseOccurrences...)
	unknown = append(unknown[:2], append([]sunspec.Occurrence{rawOccurrence(t, registry, 65000, 2, []uint16{0x1234, 0x5678})}, unknown[2:]...)...)
	duplicate := append([]sunspec.Occurrence(nil), baseOccurrences...)
	duplicate = append(duplicate[:3], append([]sunspec.Occurrence{duplicate[2]}, duplicate[3:]...)...)
	unsupportedLength := append([]sunspec.Occurrence(nil), baseOccurrences...)
	unsupportedLength[2] = rawOccurrence(t, registry, 120, 27, make([]uint16, 27))
	sentinel := append([]sunspec.Occurrence(nil), baseOccurrences...)
	sentinel[1] = sunspecfixture.InverterOccurrence(t, registry, 113, map[string][]uint16{"W": {uint16(math.Float32bits(float32(math.NaN())) >> 16), uint16(math.Float32bits(float32(math.NaN())))}}, 2)
	vendorNegative := froniusObservedSnapshotV11(t, registry, "Other", "Symo GEN24 10.0", "1.41.11-1").Occurrences()
	firmwareNegative := froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-2").Occurrences()

	tests := []struct {
		name                string
		occurrences         []sunspec.Occurrence
		capabilityAdmitted  bool
		retainedDisposition sunspec.ChainDisposition
		retainedModel       uint16
	}{
		{"unknown model retained", unknown, true, sunspec.ChainDispositionUnknownModel, 65000},
		{"duplicate model retained", duplicate, true, sunspec.ChainDispositionAdmitted, 120},
		{"unsupported length retained", unsupportedLength, true, sunspec.ChainDispositionUnsupportedLength, 120},
		{"required sentinel rejected", sentinel, false, "", 0},
		{"vendor negative", vendorNegative, true, "", 0},
		{"firmware negative", firmwareNegative, true, "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := replayFroniusQualificationCard(t, card, registry, sunspecfixture.RawWords(tc.occurrences))
			if err != nil {
				t.Fatalf("replay: %v", err)
			}
			if admitted := registry.EvaluateThreePhaseMonitoring(snapshot).Admitted(); admitted != tc.capabilityAdmitted {
				t.Fatalf("generic capability admitted=%t want=%t", admitted, tc.capabilityAdmitted)
			}
			if _, err := card.Qualify(registry, snapshot); err == nil {
				t.Fatal("negative control qualified exact observed flavor")
			}
			if tc.retainedModel != 0 {
				matches := snapshot.ByModelID(tc.retainedModel)
				if len(matches) == 0 || matches[0].Disposition != tc.retainedDisposition || len(matches[0].Words()) == 0 || len(matches[0].SourceSpans()) == 0 {
					t.Fatalf("retained model=%#v", matches)
				}
				if tc.name == "duplicate model retained" {
					if len(matches) != 2 || matches[0].Ordinal != 3 || matches[1].Ordinal != 4 ||
						!reflect.DeepEqual(matches[0].Words(), matches[1].Words()) ||
						matches[0].HeaderOffset+uint16(len(matches[0].Words())) != matches[1].HeaderOffset {
						t.Fatalf("duplicate occurrences/order/raw=%#v", matches)
					}
					firstSpans, secondSpans := matches[0].SourceSpans(), matches[1].SourceSpans()
					if len(firstSpans) == 0 || len(secondSpans) == 0 || firstSpans[0].LogicalViewID == secondSpans[0].LogicalViewID {
						t.Fatalf("duplicate source spans are not distinct: first=%#v second=%#v", firstSpans, secondSpans)
					}
					raw := snapshot.RawWords()
					firstOffset := int(matches[0].HeaderOffset - card.BaseCandidates()[0])
					secondOffset := int(matches[1].HeaderOffset - card.BaseCandidates()[0])
					if !reflect.DeepEqual(raw[firstOffset:firstOffset+len(matches[0].Words())], matches[0].Words()) ||
						!reflect.DeepEqual(raw[secondOffset:secondOffset+len(matches[1].Words())], matches[1].Words()) {
						t.Fatal("duplicate occurrence raw evidence was not retained in order")
					}
				}
			}
		})
	}
}

func TestFroniusQualificationCardRejectsWrongAndExtraBaseAcquisition(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	card := NewSunSpecFroniusQualificationCard()
	raw := sunspecfixture.RawWords(froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1").Occurrences())

	for _, tc := range []struct {
		name         string
		bases        []uint16
		selectedBase uint16
	}{
		{name: "wrong sole base", bases: []uint16{41000}, selectedBase: 41000},
		{name: "extra scanned base", bases: []uint16{40000, 41000}, selectedBase: 40000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := sunspec.NewChainPlan(sunspec.ChainPlanSpec{
				SchemaRevision: card.SchemaRevision(),
				BaseCandidates: tc.bases,
				Limits:         card.Limits(),
				DecoderKeys:    registry.DecoderKeys(),
			})
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := replayFroniusQualificationPlan(t, plan, raw, tc.selectedBase)
			if err != nil {
				t.Fatalf("custom-plan replay: %v", err)
			}
			if len(snapshot.RawWords()) == 0 || !registry.EvaluateThreePhaseMonitoring(snapshot).Admitted() {
				t.Fatal("negative control did not produce a complete generic SunSpec capture")
			}
			if _, err := card.Qualify(registry, snapshot); err == nil {
				t.Fatal("qualification card accepted acquisition outside its exact base plan")
			}
		})
	}
}

func TestFroniusQualificationCardRejectsMalformedLengthMissingChunkAndWrongUnit(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	card := NewSunSpecFroniusQualificationCard()

	chain, err := card.NewReplayChain(registry)
	if err != nil {
		t.Fatal(err)
	}
	id := uint64(1)
	if _, err := sunspecfixture.AdmitNext(t, chain, &id, []uint16{sunspecfixture.SignatureFirst, sunspecfixture.SignatureSecond}); err != nil {
		t.Fatal(err)
	}
	if _, err := sunspecfixture.AdmitNext(t, chain, &id, []uint16{120, 0}); err == nil || len(chain.NextRequests()) != 0 {
		t.Fatal("malformed zero length did not terminally fail closed")
	}

	raw := sunspecfixture.RawWords(froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1").Occurrences())
	partial := raw[:len(raw)-20]
	snapshot, err := replayFroniusQualificationCard(t, card, registry, partial)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.RawWords()) != 0 {
		t.Fatal("missing chunk produced terminal snapshot")
	}

	complete, err := replayFroniusQualificationCard(t, card, registry, raw)
	if err != nil {
		t.Fatal(err)
	}
	complete = sunspecfixture.WithRecords(t, complete, func(records []modbusreg.LogicalViewRecord) []modbusreg.LogicalViewRecord {
		records[0].UnitID = 2
		return records
	})
	if _, err := card.Qualify(registry, complete); err == nil {
		t.Fatal("wrong unit qualified exact observed card")
	}
}

func TestFroniusQualificationCardSanitizedExpectedResult(t *testing.T) {
	encoded, err := json.Marshal(NewSunSpecFroniusQualificationCard().ExpectedResult())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"live_qualified", "support_claim", "automatic_runtime_admission", "write_authority"} {
		if bytes.Contains(encoded, []byte(`"`+forbidden+`"`)) {
			t.Fatalf("expected result contains %s", forbidden)
		}
	}
	want, err := os.ReadFile("../driverdata/sunspec/fronius_qualification_card_expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(encoded, &gotValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("expected result=%s want=%s", encoded, want)
	}
}

func TestFroniusModel160PointsJoinTheCandidateList(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	card := NewSunSpecFroniusQualificationCard()
	want := []string{
		"mppt.module.dc_energy",
		"mppt.module.dc_voltage",
		"mppt.module.dc_current",
		"mppt.module.dc_power",
	}
	candidates := card.SemanticCandidateFieldIDs()
	if len(candidates) < len(want) {
		t.Fatalf("candidate count=%d", len(candidates))
	}
	got := candidates[len(candidates)-len(want):]
	for index, id := range want {
		if got[index] != id {
			t.Fatalf("model 160 candidate[%d]=%q want %q", index, got[index], id)
		}
	}
	mutated := card.SemanticCandidateFieldIDs()
	mutated[len(mutated)-1] = "mutated"
	fresh := card.SemanticCandidateFieldIDs()
	if fresh[len(fresh)-1] != want[len(want)-1] {
		t.Fatal("candidate list leaked mutation")
	}
	recorded := candidates[:len(candidates)-len(want)]
	if len(recorded) < 14 || recorded[0] != "inverter.ac.current.total" || recorded[13] != "inverter.events.2" {
		t.Fatalf("recorded hardware facts=%v", recorded)
	}
	for _, id := range candidates {
		if id == "meter.energy.import.total" || strings.Contains(strings.ToLower(id), "battery") {
			t.Fatalf("%s is in a qualification list", id)
		}
	}
	if !reflect.DeepEqual(card.ExpectedChain(), froniusObservedChainV11) {
		t.Fatal("expected chain changed")
	}
	raw := sunspecfixture.RawWords(froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1").Occurrences())
	snapshot, err := replayFroniusQualificationCard(t, card, registry, raw)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	observation, err := card.Qualify(registry, snapshot)
	if err != nil {
		t.Fatalf("Qualify: %v", err)
	}
	if got := capabilityFieldIDs(observation.Capability().Facts()); !reflect.DeepEqual(got, recorded) {
		t.Fatalf("typed facts=%v", got)
	}
	var occurrence sunspec.Occurrence
	found := 0
	for _, item := range observation.Occurrences() {
		if item.ModelID() == 160 {
			occurrence = item
			found++
		}
	}
	if found != 1 {
		t.Fatalf("model 160 occurrences=%d", found)
	}
	decoded, err := registry.DecodeOccurrence(occurrence)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !decoded.GeometryValid() {
		t.Fatal("model 160 geometry invalid")
	}
	energy, ok := decoded.Fact("mppt.module.dc_energy")
	if !ok || energy.FieldID != "mppt.module.dc_energy" {
		t.Fatal("model 160 occurrence did not expose mppt.module.dc_energy")
	}
	for _, id := range want {
		if _, ok := decoded.Fact(id); !ok {
			t.Fatalf("model 160 occurrence did not expose %s", id)
		}
	}
	moduleIDs := 0
	for _, fact := range decoded.Facts() {
		if fact.FieldID != "mppt.module.id" {
			continue
		}
		moduleIDs++
		text, isText := fact.Value.Text()
		if isText || text != "" || fact.Value.PointType() != sunspec.TypeUint16 || strings.Contains(strings.ToLower(fact.FieldID), "battery") {
			t.Fatal("module id is a string id or a battery id")
		}
	}
	if moduleIDs == 0 {
		t.Fatal("module id absent")
	}
	label, ok := decoded.Fact("mppt.module.label")
	if !ok || label.Value.PointType() != sunspec.TypeString {
		t.Fatal("module label is not the string point")
	}
}

func capabilityFieldIDs(facts []sunspec.CapabilityFact) []string {
	ids := make([]string, len(facts))
	for index, fact := range facts {
		ids[index] = fact.FieldID()
	}
	return ids
}

// rawOccurrence returns an occurrence whose words are the header of
// modelID/length followed by payload; the replay uses only its words.
func rawOccurrence(t *testing.T, registry sunspec.DecoderRegistry, modelID, length uint16, payload []uint16) sunspec.Occurrence {
	t.Helper()
	words := []uint16{modelID, length}
	words = append(words, payload...)
	return sunspecfixture.Occurrence(t, registry, words, 0)
}

func replayFroniusQualificationCard(t *testing.T, card SunSpecFroniusQualificationCard, registry sunspec.DecoderRegistry, raw []uint16) (sunspec.ChainSnapshot, error) {
	t.Helper()
	chain, err := card.NewReplayChain(registry)
	if err != nil {
		return sunspec.ChainSnapshot{}, err
	}
	return replayFroniusQualificationChain(t, chain, raw, card.BaseCandidates()[0])
}

func replayFroniusQualificationPlan(t *testing.T, plan sunspec.ChainPlan, raw []uint16, selectedBase uint16) (sunspec.ChainSnapshot, error) {
	t.Helper()
	return replayFroniusQualificationChain(t, sunspec.NewChain(plan), raw, selectedBase)
}

func replayFroniusQualificationChain(t *testing.T, chain *sunspec.Chain, raw []uint16, selectedBase uint16) (sunspec.ChainSnapshot, error) {
	t.Helper()
	base := uint32(selectedBase)
	viewID := uint64(1000)
	for len(chain.NextRequests()) != 0 {
		request := chain.NextRequests()[0]
		if request.Purpose() == sunspec.ReadSignature && request.Address() != selectedBase {
			if _, err := chain.AdmitReplay(request, sunspecfixture.View(t, request, viewID, []uint16{0, 0}, "qualification-fixture")); err != nil {
				return sunspec.ChainSnapshot{}, err
			}
			viewID++
			continue
		}
		if uint32(request.Address()) < base {
			return sunspec.ChainSnapshot{}, nil
		}
		start := int(uint32(request.Address()) - base)
		end := start + int(request.WordCount())
		if start < 0 || end > len(raw) {
			return sunspec.ChainSnapshot{}, nil
		}
		snapshot, err := chain.AdmitReplay(request, sunspecfixture.View(t, request, viewID, raw[start:end], "qualification-fixture"))
		if err != nil {
			return sunspec.ChainSnapshot{}, err
		}
		viewID++
		if len(snapshot.RawWords()) != 0 {
			return snapshot, nil
		}
	}
	return sunspec.ChainSnapshot{}, nil
}
