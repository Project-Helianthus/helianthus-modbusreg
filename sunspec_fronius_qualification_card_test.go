package modbusreg

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"
)

func TestFroniusQualificationCardPinsBoundedReadOnlyObservedFlavor(t *testing.T) {
	card := NewSunSpecFroniusQualificationCard()
	if card.ID() != SunSpecFroniusQualificationCardID ||
		card.FlavorID() != SunSpecFroniusObservedFlavorV11ID ||
		card.SchemaRevision() != SunSpecModelsRevisionV1 ||
		card.Manufacturer() != "Fronius" || card.Model() != "Symo GEN24 10.0" ||
		card.Firmware() != "1.41.11-1" || card.UnitID() != 1 ||
		card.Function() != FunctionReadHoldingRegisters || !card.ReadOnly() {
		t.Fatalf("unexpected qualification card: %#v", card)
	}
	if card.LiveQualified() || card.AutomaticRuntimeAdmission() || card.WriteAuthority() {
		t.Fatal("offline qualification card created live or write authority")
	}
	if got := card.BaseCandidates(); !reflect.DeepEqual(got, []uint16{40000}) {
		t.Fatalf("base candidates=%v", got)
	}
	if limits := card.Limits(); limits != (SunSpecChainLimits{MaxTotalWords: 512, MaxOccurrences: 64}) {
		t.Fatalf("limits=%#v", limits)
	}
	wantFacts := []string{
		"inverter.ac.current.total", "inverter.ac.current.phase_a", "inverter.ac.current.phase_b", "inverter.ac.current.phase_c",
		"inverter.ac.voltage.phase_a", "inverter.ac.voltage.phase_b", "inverter.ac.voltage.phase_c",
		"inverter.ac.power.active", "inverter.ac.frequency", "inverter.ac.energy_lifetime",
		"inverter.temperature.cabinet", "inverter.operating_state", "inverter.events.1", "inverter.events.2",
	}
	if got := card.SemanticCandidateFieldIDs(); !reflect.DeepEqual(got, wantFacts) {
		t.Fatalf("semantic candidates=%v", got)
	}
	got := card.ExpectedChain()
	got[0] = SunSpecWireKey{}
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
	registry := mustStandardSunSpecRegistry(t)
	card := NewSunSpecFroniusQualificationCard()
	raw := rawWordsForOccurrences(froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1").Occurrences())
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
	if got := len(observation.Capability().Facts()); got != len(card.SemanticCandidateFieldIDs()) {
		t.Fatalf("typed facts=%d", got)
	}
}

func TestFroniusQualificationCardCompleteChainNegativeControls(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	card := NewSunSpecFroniusQualificationCard()
	baseOccurrences := froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1").Occurrences()

	unknown := append([]SunSpecOccurrence(nil), baseOccurrences...)
	unknown = append(unknown[:2], append([]SunSpecOccurrence{rawOccurrence(65000, 2, []uint16{0x1234, 0x5678})}, unknown[2:]...)...)
	duplicate := append([]SunSpecOccurrence(nil), baseOccurrences...)
	duplicate = append(duplicate[:3], append([]SunSpecOccurrence{duplicate[2]}, duplicate[3:]...)...)
	unsupportedLength := append([]SunSpecOccurrence(nil), baseOccurrences...)
	unsupportedLength[2] = rawOccurrence(120, 27, make([]uint16, 27))
	sentinel := append([]SunSpecOccurrence(nil), baseOccurrences...)
	sentinel[1] = inverterOccurrence(t, registry, 113, map[string][]uint16{"W": {uint16(math.Float32bits(float32(math.NaN())) >> 16), uint16(math.Float32bits(float32(math.NaN())))}}, 2)
	vendorNegative := froniusObservedSnapshotV11(t, registry, "Other", "Symo GEN24 10.0", "1.41.11-1").Occurrences()
	firmwareNegative := froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-2").Occurrences()

	tests := []struct {
		name                string
		occurrences         []SunSpecOccurrence
		capabilityAdmitted  bool
		retainedDisposition SunSpecChainDisposition
		retainedModel       uint16
	}{
		{"unknown model retained", unknown, true, SunSpecChainDispositionUnknownModel, 65000},
		{"duplicate model retained", duplicate, true, SunSpecChainDispositionAdmitted, 120},
		{"unsupported length retained", unsupportedLength, true, SunSpecChainDispositionUnsupportedLength, 120},
		{"required sentinel rejected", sentinel, false, "", 0},
		{"vendor negative", vendorNegative, true, "", 0},
		{"firmware negative", firmwareNegative, true, "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := replayFroniusQualificationCard(t, card, registry, rawWordsForOccurrences(tc.occurrences))
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
			}
		})
	}
}

func TestFroniusQualificationCardRejectsMalformedLengthMissingChunkAndWrongUnit(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	card := NewSunSpecFroniusQualificationCard()

	chain, err := card.NewReplayChain(registry)
	if err != nil {
		t.Fatal(err)
	}
	id := uint64(1)
	if _, err := admitNext(t, chain, &id, []uint16{sunSpecSignatureFirst, sunSpecSignatureSecond}); err != nil {
		t.Fatal(err)
	}
	if _, err := admitNext(t, chain, &id, []uint16{120, 0}); err == nil || len(chain.NextRequests()) != 0 {
		t.Fatal("malformed zero length did not terminally fail closed")
	}

	raw := rawWordsForOccurrences(froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1").Occurrences())
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
	complete.sources[0].UnitID = 2
	if _, err := card.Qualify(registry, complete); err == nil {
		t.Fatal("wrong unit qualified exact observed card")
	}
}

func TestFroniusQualificationCardSanitizedExpectedResult(t *testing.T) {
	encoded, err := json.Marshal(NewSunSpecFroniusQualificationCard().ExpectedResult())
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/sunspec/qualification/fronius_gen24_float_v1_1_expected.json")
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

func rawOccurrence(modelID, length uint16, payload []uint16) SunSpecOccurrence {
	words := []uint16{modelID, length}
	words = append(words, payload...)
	return SunSpecOccurrence{words: words}
}

func replayFroniusQualificationCard(t *testing.T, card SunSpecFroniusQualificationCard, registry SunSpecDecoderRegistry, raw []uint16) (SunSpecChainSnapshot, error) {
	t.Helper()
	chain, err := card.NewReplayChain(registry)
	if err != nil {
		return SunSpecChainSnapshot{}, err
	}
	base := uint32(card.BaseCandidates()[0])
	viewID := uint64(1000)
	for len(chain.NextRequests()) != 0 {
		request := chain.NextRequests()[0]
		start := int(uint32(request.Address()) - base)
		end := start + int(request.WordCount())
		if start < 0 || end > len(raw) {
			return SunSpecChainSnapshot{}, nil
		}
		snapshot, err := chain.AdmitReplay(request, chainView(t, request, viewID, raw[start:end], "qualification-fixture"))
		if err != nil {
			return SunSpecChainSnapshot{}, err
		}
		viewID++
		if len(snapshot.RawWords()) != 0 {
			return snapshot, nil
		}
	}
	return SunSpecChainSnapshot{}, nil
}
