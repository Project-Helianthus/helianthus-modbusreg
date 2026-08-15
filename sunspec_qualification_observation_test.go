package modbusreg

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// The qualification record is deliberately constructed from only a registry and
// a completed capture.  In particular, neither a caller verdict nor a
// caller-selected flavor is an input to the owner record.
func TestSunSpecQualificationObservationDerivesAndRetainsExactV11Evidence(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	snapshot := qualificationSnapshot(t, registry)

	observation, err := NewSunSpecQualificationObservation(registry, snapshot)
	if err != nil {
		t.Fatalf("NewSunSpecQualificationObservation: %v", err)
	}
	if !observation.Capability().Admitted() || observation.Capability().Reason() != SunSpecCapabilityReasonAdmitted {
		t.Fatalf("capability=%#v", observation.Capability())
	}
	if !observation.Flavor().Matched() || observation.Flavor().FlavorID() != SunSpecFroniusObservedFlavorV11ID {
		t.Fatalf("flavor=%#v", observation.Flavor())
	}
	wantChain := []SunSpecWireKey{{1, 65}, {113, 60}, {120, 26}, {121, 30}, {122, 44}, {123, 24}, {160, 88}, {124, 24}, {sunSpecEndModel, 0}}
	if got := observation.Chain(); !reflect.DeepEqual(got, wantChain) {
		t.Fatalf("chain=%#v want=%#v", got, wantChain)
	}

	identity := observation.SampleIdentity()
	if identity.PollGeneration() != 6 || identity.DeadlineIdentity() != 700 {
		t.Fatalf("sample identity=%#v", identity)
	}
	if observation.SampleID() != "sunspec-6-700" {
		t.Fatalf("sample id=%q", observation.SampleID())
	}
	occurrences, views := observation.Occurrences(), observation.SourceViews()
	if len(occurrences) != 8 || len(views) != len(snapshot.SourceViews()) {
		t.Fatalf("occurrences=%d views=%d", len(occurrences), len(views))
	}
	for index, view := range views {
		record := view.Record()
		if record.PollGeneration != identity.PollGeneration() || record.DeadlineIdentity != identity.DeadlineIdentity() {
			t.Fatalf("source view %d has detached identity: %#v", index, record)
		}
	}
	model123 := occurrences[5]
	if model123.WireKey != (SunSpecWireKey{ModelID: 123, ModelLength: 24}) || model123.Disposition != SunSpecChainDispositionAdmitted {
		t.Fatalf("model 123=%#v", model123)
	}
	key, ok := model123.DecoderKey()
	if !ok || key != (SunSpecDecoderKey{ModelID: 123, ModelLength: 24, SchemaRevision: testSunSpecModelsRevision}) || len(model123.SourceSpans()) == 0 || len(model123.Words()) != 26 {
		t.Fatalf("model 123 key=%#v ok=%t spans=%#v raw=%#v", key, ok, model123.SourceSpans(), model123.Words())
	}
	if raw := observation.RawWords(); !reflect.DeepEqual(raw, snapshot.RawWords()) {
		t.Fatalf("raw words were not retained: got=%d want=%d", len(raw), len(snapshot.RawWords()))
	}

	// All getter results must remain detached from the immutable qualification.
	chain, raw := observation.Chain(), observation.RawWords()
	chain[0], raw[0] = SunSpecWireKey{}, 0
	occurrences[5].words[0] = 0
	record := views[0].Record()
	record.Words[0] = 0
	if observation.Chain()[0] != (SunSpecWireKey{ModelID: 1, ModelLength: 65}) || observation.RawWords()[0] != sunSpecSignatureFirst || observation.Occurrences()[5].Words()[0] != 123 || observation.SourceViews()[0].Record().Words[0] == 0 {
		t.Fatal("qualification getters leaked caller mutation")
	}
}

func TestSunSpecQualificationObservationJSONGoldenAndBoundedReplay(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	observation, err := NewSunSpecQualificationObservation(registry, qualificationSnapshot(t, registry))
	if err != nil {
		t.Fatal(err)
	}
	first, err := json.Marshal(observation)
	if err != nil {
		t.Fatalf("MarshalJSON(first): %v", err)
	}
	second, err := json.Marshal(observation)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("qualification JSON is not deterministic: %v", err)
	}
	fixture, err := os.ReadFile("testdata/sunspec/qualification/fronius_gen24_float_v1_1.json")
	if err != nil {
		t.Fatal(err)
	}
	var want qualificationObservationManifest
	if err := json.Unmarshal(fixture, &want); err != nil {
		t.Fatalf("unmarshal metadata manifest: %v", err)
	}
	var got serializedQualificationObservation
	if err := json.Unmarshal(first, &got); err != nil {
		t.Fatalf("unmarshal qualification JSON: %v", err)
	}
	if got.Schema != want.Schema || got.CapabilityID != want.CapabilityID || got.FlavorID != want.FlavorID || got.SampleID != want.SampleID || got.SampleIdentity != want.SampleIdentity || !reflect.DeepEqual(got.Chain, want.Chain) {
		t.Fatalf("serialized metadata=%+v want=%+v", got.qualificationMetadata(), want)
	}
	if !reflect.DeepEqual(got.RawWords, observation.RawWords()) || len(got.Occurrences) != 8 || len(got.SourceViews) != len(observation.SourceViews()) {
		t.Fatalf("serialized evidence raw=%d occurrences=%d source_views=%d", len(got.RawWords), len(got.Occurrences), len(got.SourceViews))
	}
	for index, occurrence := range got.Occurrences {
		if len(occurrence.Words) == 0 || len(occurrence.SourceSpans) == 0 || occurrence.DecoderKey == nil {
			t.Fatalf("serialized occurrence %d lost words, spans, or decoder key: %#v", index, occurrence)
		}
	}
	model123 := got.Occurrences[5]
	if model123.WireKey != (SunSpecWireKey{ModelID: 123, ModelLength: 24}) || model123.DecoderKey == nil || *model123.DecoderKey != (SunSpecDecoderKey{ModelID: 123, ModelLength: 24, SchemaRevision: testSunSpecModelsRevision}) {
		t.Fatalf("serialized model 123=%#v", model123)
	}
	for index, view := range got.SourceViews {
		if view.Transport == "" || len(view.WireResponseBytes) == 0 || view.PollGeneration != 6 || view.DeadlineIdentity != 700 {
			t.Fatalf("serialized source view %d lost wire or transport provenance: %#v", index, view)
		}
	}

	replay, err := observation.Replay()
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if !reflect.DeepEqual(replay.RawWords(), observation.RawWords()) || !reflect.DeepEqual(replay.Occurrences(), observation.Occurrences()) || !reflect.DeepEqual(replay.SourceViews(), observation.SourceViews()) {
		t.Fatal("bounded ordered replay lost retained evidence")
	}
	replayed := replay.RawWords()
	replayed[0] = 0
	if observation.RawWords()[0] != sunSpecSignatureFirst {
		t.Fatal("replay retained caller-owned raw storage")
	}
}

func TestSunSpecQualificationObservationFailsClosed(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	valid := qualificationSnapshot(t, registry)

	noMatch := cloneSnapshotForTest(valid)
	noMatch.occurrences[0] = commonOccurrence(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-2", 1)
	noMatch = qualificationRebuildSnapshot(t, registry, noMatch)

	unknown := cloneSnapshotForTest(valid)
	unknown.occurrences = append(unknown.occurrences, SunSpecOccurrence{Ordinal: 9, WireKey: SunSpecWireKey{ModelID: 999, ModelLength: 1}, SchemaRevision: testSunSpecModelsRevision, HeaderOffset: 40000, PayloadOffset: 40002, Disposition: SunSpecChainDispositionUnknownModel, words: []uint16{999, 1, 7}, spans: []SunSpecSourceSpan{{LogicalViewID: 999, PDUOffset: 0, WordCount: 3}}})
	unknown = qualificationRebuildSnapshot(t, registry, unknown)

	unsupported := cloneSnapshotForTest(valid)
	unsupportedWords := make([]uint16, 27)
	unsupportedWords[0], unsupportedWords[1] = 123, 25
	unsupported.occurrences[5] = admittedOccurrence(123, 25, unsupportedWords, 6)
	unsupported.occurrences[5].Disposition = SunSpecChainDispositionUnsupportedLength
	unsupported = qualificationRebuildSnapshot(t, registry, unsupported)

	ambiguous := cloneSnapshotForTest(valid)
	// The selector itself must remain fail-closed if future registry definitions overlap.
	if selected := selectSunSpecFroniusFlavor([]SunSpecFroniusFlavorDecision{registry.EvaluateFroniusObservedFlavorV11(valid), registry.EvaluateFroniusObservedFlavorV11(valid)}); selected.Matched() || selected.Reason() != SunSpecFroniusFlavorSelectionReasonAmbiguousMatch {
		t.Fatalf("ambiguous flavor selection=%#v", selected)
	}
	ambiguous.sources[1].DeadlineIdentity++

	nonterminal := cloneSnapshotForTest(valid)
	nonterminal.raw = nonterminal.raw[:len(nonterminal.raw)-2]
	malformedIdentity := cloneSnapshotForTest(valid)
	malformedIdentity.sources[0].PollGeneration = 0
	mixedProvenance := cloneSnapshotForTest(valid)
	mixedProvenance.sources[1].Transport = TransportRTU

	for name, snapshot := range map[string]SunSpecChainSnapshot{
		"no match":           noMatch,
		"unknown occurrence": unknown,
		"unsupported":        unsupported,
		"ambiguous identity": ambiguous,
		"nonterminal":        nonterminal,
		"malformed identity": malformedIdentity,
		"mixed provenance":   mixedProvenance,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewSunSpecQualificationObservation(registry, snapshot); err == nil {
				t.Fatal("invalid qualification input unexpectedly admitted")
			}
		})
	}
}

func qualificationSnapshot(t *testing.T, registry SunSpecDecoderRegistry) SunSpecChainSnapshot {
	t.Helper()
	v11 := froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	snapshot := completedChainSnapshot(t, registry, v11.Occurrences()...)
	for index := range snapshot.sources {
		snapshot.sources[index].DeadlineIdentity = 700
	}
	return snapshot
}

type qualificationObservationManifest struct {
	Schema         string            `json:"schema"`
	CapabilityID   string            `json:"capability_id"`
	FlavorID       string            `json:"flavor_id"`
	SampleID       string            `json:"sample_id"`
	SampleIdentity sampleIdentityDTO `json:"sample_identity"`
	Chain          []SunSpecWireKey  `json:"chain"`
}

type sampleIdentityDTO struct {
	PollGeneration   uint64 `json:"poll_generation"`
	DeadlineIdentity uint64 `json:"deadline_identity"`
}

type serializedQualificationObservation struct {
	qualificationObservationManifest
	RawWords    []uint16                      `json:"raw_words"`
	Occurrences []serializedSunSpecOccurrence `json:"occurrences"`
	SourceViews []serializedLogicalView       `json:"source_views"`
}

func (value serializedQualificationObservation) qualificationMetadata() qualificationObservationManifest {
	return value.qualificationObservationManifest
}

type serializedSunSpecOccurrence struct {
	WireKey     SunSpecWireKey      `json:"wire_key"`
	DecoderKey  *SunSpecDecoderKey  `json:"decoder_key"`
	Words       []uint16            `json:"words"`
	SourceSpans []SunSpecSourceSpan `json:"source_spans"`
}

type serializedLogicalView struct {
	Transport         string `json:"transport"`
	WireResponseBytes []byte `json:"wire_response_bytes"`
	PollGeneration    uint64 `json:"poll_generation"`
	DeadlineIdentity  uint64 `json:"deadline_identity"`
}

func qualificationRebuildSnapshot(t *testing.T, registry SunSpecDecoderRegistry, snapshot SunSpecChainSnapshot) SunSpecChainSnapshot {
	t.Helper()
	return qualificationSnapshotWithOccurrences(t, registry, snapshot.Occurrences())
}

func qualificationSnapshotWithOccurrences(t *testing.T, registry SunSpecDecoderRegistry, occurrences []SunSpecOccurrence) SunSpecChainSnapshot {
	t.Helper()
	snapshot := completedChainSnapshot(t, registry, occurrences...)
	for index := range snapshot.sources {
		snapshot.sources[index].DeadlineIdentity = 700
	}
	return snapshot
}
