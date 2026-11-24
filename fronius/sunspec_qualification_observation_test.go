package fronius

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/internal/sunspecfixture"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
)

// The qualification record is deliberately constructed from only a registry and
// a completed capture.  In particular, neither a caller verdict nor a
// caller-selected flavor is an input to the owner record.
func TestSunSpecQualificationObservationDerivesAndRetainsExactV11Evidence(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	snapshot := qualificationSnapshot(t, registry)

	observation, err := NewSunSpecQualificationObservation(registry, snapshot)
	if err != nil {
		t.Fatalf("NewSunSpecQualificationObservation: %v", err)
	}
	if !observation.Capability().Admitted() || observation.Capability().Reason() != sunspec.CapabilityReasonAdmitted {
		t.Fatalf("capability=%#v", observation.Capability())
	}
	if !observation.Flavor().Matched() || observation.Flavor().FlavorID() != SunSpecFroniusObservedFlavorV11ID {
		t.Fatalf("flavor=%#v", observation.Flavor())
	}
	wantChain := []sunspec.WireKey{{ModelID: 1, ModelLength: 65}, {ModelID: 113, ModelLength: 60}, {ModelID: 120, ModelLength: 26}, {ModelID: 121, ModelLength: 30}, {ModelID: 122, ModelLength: 44}, {ModelID: 123, ModelLength: 24}, {ModelID: 160, ModelLength: 88}, {ModelID: 124, ModelLength: 24}, {ModelID: sunspec.EndModelID, ModelLength: 0}}
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
	if model123.WireKey != (sunspec.WireKey{ModelID: 123, ModelLength: 24}) || model123.Disposition != sunspec.ChainDispositionAdmitted {
		t.Fatalf("model 123=%#v", model123)
	}
	key, ok := model123.DecoderKey()
	if !ok || key != (sunspec.DecoderKey{ModelID: 123, ModelLength: 24, SchemaRevision: sunspecfixture.Revision}) || len(model123.SourceSpans()) == 0 || len(model123.Words()) != 26 {
		t.Fatalf("model 123 key=%#v ok=%t spans=%#v raw=%#v", key, ok, model123.SourceSpans(), model123.Words())
	}
	if raw := observation.RawWords(); !reflect.DeepEqual(raw, snapshot.RawWords()) {
		t.Fatalf("raw words were not retained: got=%d want=%d", len(raw), len(snapshot.RawWords()))
	}

	// All getter results must remain detached from the immutable qualification.
	chain, raw := observation.Chain(), observation.RawWords()
	chain[0], raw[0] = sunspec.WireKey{}, 0
	occurrences[5].WireKey = sunspec.WireKey{}
	occurrenceWords := occurrences[5].Words()
	occurrenceWords[0] = 0
	record := views[0].Record()
	record.Words[0] = 0
	if observation.Chain()[0] != (sunspec.WireKey{ModelID: 1, ModelLength: 65}) || observation.RawWords()[0] != sunspecfixture.SignatureFirst || observation.Occurrences()[5].WireKey != (sunspec.WireKey{ModelID: 123, ModelLength: 24}) || observation.Occurrences()[5].Words()[0] != 123 || observation.SourceViews()[0].Record().Words[0] == 0 {
		t.Fatal("qualification getters leaked caller mutation")
	}
}

func TestSunSpecQualificationObservationJSONGoldenAndBoundedReplay(t *testing.T) {
	registry := sunspecfixture.Registry(t)
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
	if bytes.Contains(first, []byte("structural_candidate")) {
		t.Fatal("V1 qualification JSON exposed private structural candidate metadata")
	}
	fixture, err := os.ReadFile("../testdata/sunspec/qualification/fronius_gen24_float_v1_1.json")
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
	if got.Schema != want.Schema || got.CapabilityID != want.CapabilityID || got.CapabilityReason != want.CapabilityReason || got.FlavorID != want.FlavorID || got.FlavorReason != want.FlavorReason || got.SampleID != want.SampleID || got.SampleIdentity != want.SampleIdentity || !reflect.DeepEqual(got.Chain, want.Chain) {
		t.Fatalf("serialized metadata=%+v want=%+v", got.qualificationMetadata(), want)
	}
	if got.CapabilityReason != sunspec.CapabilityReasonAdmitted || got.FlavorReason != SunSpecFroniusFlavorReasonMatched {
		t.Fatalf("serialized reasons capability=%q flavor=%q", got.CapabilityReason, got.FlavorReason)
	}
	if !reflect.DeepEqual(got.RawWords, observation.RawWords()) || len(got.Occurrences) != 8 || len(got.SourceViews) != len(observation.SourceViews()) {
		t.Fatalf("serialized evidence raw=%d occurrences=%d source_views=%d", len(got.RawWords), len(got.Occurrences), len(got.SourceViews))
	}
	for index, gotOccurrence := range got.Occurrences {
		wantOccurrence := observation.Occurrences()[index]
		wantDecoderKey, wantHasDecoderKey := wantOccurrence.DecoderKey()
		if gotOccurrence.Ordinal != wantOccurrence.Ordinal || gotOccurrence.WireKey != wantOccurrence.WireKey || gotOccurrence.SchemaRevision != wantOccurrence.SchemaRevision || gotOccurrence.HeaderOffset != wantOccurrence.HeaderOffset || gotOccurrence.PayloadOffset != wantOccurrence.PayloadOffset || gotOccurrence.Disposition != wantOccurrence.Disposition || !reflect.DeepEqual(gotOccurrence.Words, wantOccurrence.Words()) || !reflect.DeepEqual(gotOccurrence.SourceSpans, wantOccurrence.SourceSpans()) || (gotOccurrence.DecoderKey != nil) != wantHasDecoderKey || wantHasDecoderKey && *gotOccurrence.DecoderKey != wantDecoderKey {
			t.Fatalf("serialized occurrence %d=%#v does not retain source=%#v", index, gotOccurrence, wantOccurrence)
		}
	}
	model123 := got.Occurrences[5]
	if model123.WireKey != (sunspec.WireKey{ModelID: 123, ModelLength: 24}) || model123.DecoderKey == nil || *model123.DecoderKey != (sunspec.DecoderKey{ModelID: 123, ModelLength: 24, SchemaRevision: sunspecfixture.Revision}) {
		t.Fatalf("serialized model 123=%#v", model123)
	}
	for index, gotView := range got.SourceViews {
		wantView := observation.SourceViews()[index].Record()
		if gotView.LogicalViewID != wantView.LogicalViewID || gotView.WireResponseID != wantView.WireResponseID || gotView.PhysicalRequestID != wantView.PhysicalRequestID || gotView.Endpoint != wantView.Endpoint || gotView.ConnectionID != wantView.ConnectionID || gotView.Transport != wantView.Transport || gotView.TransportGeneration != wantView.TransportGeneration || gotView.UnitID != wantView.UnitID || gotView.RequestedFunction != wantView.RequestedFunction || gotView.ReceivedFunction != wantView.ReceivedFunction || gotView.Table != wantView.Table || gotView.PhysicalOffset != wantView.PhysicalOffset || gotView.PhysicalWordCount != wantView.PhysicalWordCount || gotView.AuthorizationScope != wantView.AuthorizationScope || gotView.PollGeneration != wantView.PollGeneration || gotView.DeadlineIdentity != wantView.DeadlineIdentity || gotView.LogicalOffset != wantView.LogicalOffset || gotView.LogicalWordCount != wantView.LogicalWordCount || gotView.SliceOffset != wantView.SliceOffset || gotView.SliceWordCount != wantView.SliceWordCount || !reflect.DeepEqual(gotView.Words, wantView.Words) || !reflect.DeepEqual(gotView.WireResponseBytes, wantView.WireResponseBytes) {
			t.Fatalf("serialized source view %d=%#v does not retain source=%#v", index, gotView, wantView)
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
	if observation.RawWords()[0] != sunspecfixture.SignatureFirst {
		t.Fatal("replay retained caller-owned raw storage")
	}
}

func TestSunSpecQualificationObservationJSONIgnoresPrivateStructuralCandidate(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	baseline, err := NewSunSpecQualificationObservation(registry, qualificationSnapshot(t, registry))
	if err != nil {
		t.Fatal(err)
	}
	baselineJSON, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}

	// The qualification encodes occurrences through their exported fields and
	// accessors only. A V2 Model 707 occurrence walked with structural
	// validation carries the private structural candidate; walked without it,
	// it does not. Every value the encoder reads must be identical for both.
	model707 := []uint16{707, 33, 0, 0, 0, 1, 2, 0, 0, 0, 1, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 1, 0, 1, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 1}
	plain := sunspecfixture.WalkWords(t, sunspec.ChainPlanSpec{SchemaRevision: sunspec.ModelsRevisionV2}, nil, model707).Occurrences()
	selected := sunspecfixture.WalkWords(t, sunspec.ChainPlanSpec{SchemaRevision: sunspec.ModelsRevisionV2, StructuralCandidateIDs: []uint16{707}}, nil, model707).Occurrences()
	if len(plain) != 1 || len(selected) != 1 || reflect.DeepEqual(plain[0], selected[0]) {
		t.Fatal("structural validation attached no private candidate to the Model 707 occurrence")
	}
	plainKey, plainHasKey := plain[0].DecoderKey()
	selectedKey, selectedHasKey := selected[0].DecoderKey()
	if selected[0].Ordinal != plain[0].Ordinal || selected[0].WireKey != plain[0].WireKey || selected[0].SchemaRevision != plain[0].SchemaRevision || selected[0].HeaderOffset != plain[0].HeaderOffset || selected[0].PayloadOffset != plain[0].PayloadOffset || selected[0].Disposition != plain[0].Disposition || !reflect.DeepEqual(selected[0].Words(), plain[0].Words()) || !reflect.DeepEqual(selected[0].SourceSpans(), plain[0].SourceSpans()) || selectedHasKey != plainHasKey || selectedKey != plainKey {
		t.Fatal("private structural candidate metadata reached a value the qualification JSON encodes")
	}
	replayed, err := NewSunSpecQualificationObservation(registry, sunspecfixture.WithRecords(t, qualificationSnapshot(t, registry), nil))
	if err != nil {
		t.Fatal(err)
	}
	replayedJSON, err := json.Marshal(replayed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(replayedJSON, baselineJSON) {
		t.Fatal("qualification JSON is not a function of the exported snapshot evidence")
	}
}

func TestSunSpecQualificationObservationRetainsCoalescedTCPProvenance(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	baseline, err := NewSunSpecQualificationObservation(registry, qualificationSnapshot(t, registry))
	if err != nil {
		t.Fatalf("NewSunSpecQualificationObservation(baseline): %v", err)
	}
	snapshot, expected := qualificationSnapshotWithCoalescedTCPView(t, registry)
	if expected.PhysicalOffset == expected.LogicalOffset ||
		expected.PhysicalWordCount == expected.LogicalWordCount ||
		expected.SliceOffset == 0 ||
		expected.SliceWordCount != expected.LogicalWordCount {
		t.Fatalf("fixture is not a coalesced TCP slice: %#v", expected)
	}
	observation, err := NewSunSpecQualificationObservation(registry, snapshot)
	if err != nil {
		t.Fatalf("NewSunSpecQualificationObservation: %v", err)
	}
	if !observation.Capability().Admitted() ||
		observation.Capability().Reason() != baseline.Capability().Reason() ||
		observation.Capability().ProfileID() != baseline.Capability().ProfileID() ||
		observation.Flavor().FlavorID() != SunSpecFroniusObservedFlavorV11ID ||
		observation.Flavor().Reason() != baseline.Flavor().Reason() ||
		!reflect.DeepEqual(observation.Chain(), baseline.Chain()) {
		t.Fatalf("terminal qualification changed: capability=%#v flavor=%#v", observation.Capability(), observation.Flavor())
	}
	first, err := json.Marshal(observation)
	if err != nil {
		t.Fatalf("json.Marshal(first): %v", err)
	}
	second, err := json.Marshal(observation)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("qualification JSON is not deterministic: %v", err)
	}
	var encoded serializedQualificationObservation
	if err := json.Unmarshal(first, &encoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	var found *serializedLogicalView
	for index := range encoded.SourceViews {
		view := &encoded.SourceViews[index]
		if view.LogicalViewID == expected.LogicalViewID {
			found = view
			break
		}
	}
	if found == nil ||
		found.WireResponseID != expected.WireResponseID ||
		found.PhysicalRequestID != expected.PhysicalRequestID ||
		found.PhysicalOffset != expected.PhysicalOffset ||
		found.PhysicalWordCount != expected.PhysicalWordCount ||
		found.LogicalOffset != expected.LogicalOffset ||
		found.LogicalWordCount != expected.LogicalWordCount ||
		found.SliceOffset != expected.SliceOffset ||
		found.SliceWordCount != expected.SliceWordCount ||
		!bytes.Equal(found.WireResponseBytes, expected.WireResponseBytes) {
		t.Fatalf("serialized coalesced source=%#v want=%#v", found, expected)
	}
	spanFound := false
	for _, occurrence := range observation.Occurrences() {
		for _, span := range occurrence.SourceSpans() {
			if span.LogicalViewID == expected.LogicalViewID {
				spanFound = true
				if span.PDUOffset != expected.SliceOffset {
					t.Fatalf("coalesced occurrence span=%#v want slice offset=%d", span, expected.SliceOffset)
				}
			}
		}
	}
	if !spanFound {
		t.Fatal("coalesced source has no occurrence span")
	}
	replay, err := observation.Replay()
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	replayedFound := false
	for _, view := range replay.SourceViews() {
		record := view.Record()
		if record.LogicalViewID != expected.LogicalViewID {
			continue
		}
		replayedFound = true
		if record.PhysicalOffset != expected.PhysicalOffset ||
			record.PhysicalWordCount != expected.PhysicalWordCount ||
			record.LogicalOffset != expected.LogicalOffset ||
			record.LogicalWordCount != expected.LogicalWordCount ||
			record.SliceOffset != expected.SliceOffset ||
			record.SliceWordCount != expected.SliceWordCount ||
			record.WireResponseID != expected.WireResponseID ||
			record.PhysicalRequestID != expected.PhysicalRequestID ||
			!bytes.Equal(record.WireResponseBytes, expected.WireResponseBytes) {
			t.Fatalf("replayed coalesced source=%#v want=%#v", record, expected)
		}
	}
	if !replayedFound {
		t.Fatal("replay omitted coalesced source")
	}
}

func TestSunSpecQualificationObservationJSONRejectsZeroValue(t *testing.T) {
	if _, err := json.Marshal(SunSpecQualificationObservation{}); err == nil {
		t.Fatal("zero qualification observation unexpectedly marshaled")
	}
}

func TestSunSpecQualificationObservationFailsClosed(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	valid := qualificationSnapshot(t, registry)

	sourceAbsent := qualificationSnapshotWithOccurrences(t, registry, []sunspec.Occurrence{
		sunspecfixture.CommonOccurrence(t, registry, froniusTestManufacturer, froniusTestModel, froniusTestFirmwareV1, 1),
	})
	ambiguousSource := qualificationSnapshotWithOccurrences(t, registry, []sunspec.Occurrence{
		sunspecfixture.CommonOccurrence(t, registry, froniusTestManufacturer, froniusTestModel, froniusTestFirmwareV1, 1),
		sunspecfixture.InverterOccurrence(t, registry, 113, nil, 2),
		sunspecfixture.InverterOccurrence(t, registry, 103, nil, 3),
	})

	// The selector itself must remain fail-closed if future registry definitions overlap.
	if selected := selectSunSpecFroniusFlavor([]SunSpecFroniusFlavorDecision{EvaluateObservedFlavorV11(registry, valid), EvaluateObservedFlavorV11(registry, valid)}); selected.Matched() || selected.Reason() != SunSpecFroniusFlavorSelectionReasonAmbiguousMatch {
		t.Fatalf("ambiguous flavor selection=%#v", selected)
	}
	ambiguous := sunspecfixture.WithRecords(t, valid, func(records []modbusreg.LogicalViewRecord) []modbusreg.LogicalViewRecord {
		records[1].DeadlineIdentity++
		return records
	})

	nonterminal := sunspecfixture.WithRaw(t, valid, func(raw []uint16) []uint16 { return raw[:len(raw)-2] })
	// A source view with a zero poll generation is not a valid logical view, so
	// no snapshot that carries one can be assembled through the SunSpec API.
	malformedRecords := sunspecfixture.Records(valid)
	malformedRecords[0].PollGeneration = 0
	if err := sunspecfixture.AssembleError(valid.RawWords(), valid.Occurrences(), malformedRecords); err == nil {
		t.Fatal("malformed sample identity assembled into a snapshot")
	}
	// An RTU view carries no connection identity; the second view keeps its
	// TCP siblings' poll and deadline identity but not their provenance.
	mixedProvenance := sunspecfixture.WithRecords(t, valid, func(records []modbusreg.LogicalViewRecord) []modbusreg.LogicalViewRecord {
		records[1].Transport = modbusreg.TransportRTU
		records[1].ConnectionID = 0
		return records
	})

	for name, snapshot := range map[string]sunspec.ChainSnapshot{
		"source absent":      sourceAbsent,
		"ambiguous source":   ambiguousSource,
		"ambiguous identity": ambiguous,
		"nonterminal":        nonterminal,
		"mixed provenance":   mixedProvenance,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewSunSpecQualificationObservation(registry, snapshot); err == nil {
				t.Fatal("invalid qualification input unexpectedly admitted")
			}
		})
	}
}

// Each case changes one field of an otherwise matched identity. Poll generation
// and deadline identity are distinct, so a swap of those two fields fails.
// Matching a flavor anyway means the check for the changed field was dropped.
// Rejecting the chain means an admitted chain was still refused because no
// flavor matched.
func TestSunSpecQualificationObservationAdmitsChainWhenFlavorIsNotObserved(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	matched := qualificationSnapshot(t, registry)
	unknownOccurrence := sunspecfixture.Occurrence(t, registry, []uint16{999, 1, 7}, 9)
	if unknownOccurrence.Disposition != sunspec.ChainDispositionUnknownModel {
		t.Fatalf("unknown occurrence disposition=%q", unknownOccurrence.Disposition)
	}
	unsupportedOccurrences := matched.Occurrences()
	unsupportedWords := make([]uint16, 27)
	unsupportedWords[0], unsupportedWords[1] = 123, 25
	unsupportedOccurrences[5] = sunspecfixture.Occurrence(t, registry, unsupportedWords, 6)
	if unsupportedOccurrences[5].Disposition != sunspec.ChainDispositionUnsupportedLength {
		t.Fatalf("unsupported occurrence disposition=%q", unsupportedOccurrences[5].Disposition)
	}
	cases := []struct {
		name           string
		poll, deadline uint64
		snapshot       sunspec.ChainSnapshot
	}{
		{name: "manufacturer", poll: 41, deadline: 42, snapshot: withQualificationIdentity(t, qualificationSnapshotWithCommonIdentity(t, registry, "Synthetic", froniusTestModel, froniusTestFirmwareV1), 41, 42)},
		{name: "model", poll: 43, deadline: 44, snapshot: withQualificationIdentity(t, qualificationSnapshotWithCommonIdentity(t, registry, froniusTestManufacturer, "Public", froniusTestFirmwareV1), 43, 44)},
		{name: "firmware", poll: 45, deadline: 46, snapshot: withQualificationIdentity(t, qualificationSnapshotWithCommonIdentity(t, registry, froniusTestManufacturer, froniusTestModel, "1.41.11-2"), 45, 46)},
		{name: "chain", poll: 47, deadline: 48, snapshot: withQualificationIdentity(t, qualificationSnapshotWithOccurrences(t, registry, []sunspec.Occurrence{
			sunspecfixture.CommonOccurrence(t, registry, froniusTestManufacturer, froniusTestModel, froniusTestFirmwareV1, 1),
			sunspecfixture.InverterOccurrence(t, registry, 113, nil, 2),
		}), 47, 48)},
		{name: "unknown model", poll: 49, deadline: 50, snapshot: withQualificationIdentity(t, qualificationSnapshotWithOccurrences(t, registry, append(matched.Occurrences(), unknownOccurrence)), 49, 50)},
		{name: "unsupported length", poll: 51, deadline: 52, snapshot: withQualificationIdentity(t, qualificationSnapshotWithOccurrences(t, registry, unsupportedOccurrences), 51, 52)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertNotObservedObservation(t, registry, tc.snapshot, tc.poll, tc.deadline)
		})
	}
}

func assertNotObservedObservation(t *testing.T, registry sunspec.DecoderRegistry, snapshot sunspec.ChainSnapshot, poll, deadline uint64) {
	t.Helper()
	base, err := sunspec.NewObservation(registry, snapshot)
	if err != nil {
		t.Fatalf("NewObservation: %v", err)
	}
	observation, err := NewSunSpecQualificationObservation(registry, snapshot)
	if err != nil {
		t.Fatalf("NewSunSpecQualificationObservation: %v", err)
	}
	if !observation.Capability().Admitted() || observation.Capability().Reason() != sunspec.CapabilityReasonAdmitted || observation.Capability().ProfileID() != sunspec.ThreePhaseMonitoringCapabilityID || len(observation.Capability().Facts()) < len(sunspec.ThreePhaseMonitoringFieldIDs()) {
		t.Fatalf("capability=%#v facts=%d", observation.Capability(), len(observation.Capability().Facts()))
	}
	flavor := observation.Flavor()
	if flavor.Matched() || flavor.Reason() != SunSpecFroniusFlavorReason("NOT_OBSERVED") || flavor.FlavorID() != "sunspec.flavor.not_observed" || supportedSunSpecQualificationFlavorID(flavor.FlavorID()) || len(flavor.Chain()) != 0 || len(flavor.SourceViews()) != 0 {
		t.Fatalf("flavor id=%q reason=%q matched=%t chain=%d views=%d", flavor.FlavorID(), flavor.Reason(), flavor.Matched(), len(flavor.Chain()), len(flavor.SourceViews()))
	}
	if observation.SampleID() != base.SampleID() || observation.SampleID() != fmt.Sprintf("sunspec-%d-%d", poll, deadline) {
		t.Fatalf("sample id=%q base=%q", observation.SampleID(), base.SampleID())
	}
	identity := observation.SampleIdentity()
	if identity.PollGeneration() != poll || identity.DeadlineIdentity() != deadline || base.SampleIdentity().PollGeneration() != poll || base.SampleIdentity().DeadlineIdentity() != deadline {
		t.Fatalf("identity poll=%d deadline=%d base poll=%d deadline=%d", identity.PollGeneration(), identity.DeadlineIdentity(), base.SampleIdentity().PollGeneration(), base.SampleIdentity().DeadlineIdentity())
	}
	if !reflect.DeepEqual(observation.Chain(), base.Chain()) || !reflect.DeepEqual(observation.RawWords(), base.RawWords()) || !reflect.DeepEqual(observation.Occurrences(), base.Occurrences()) || !reflect.DeepEqual(observation.SourceViews(), base.SourceViews()) {
		t.Fatal("qualification observation did not retain the vendor-neutral observation")
	}
	assertNotObservedQualificationJSON(t, observation, poll, deadline)
}

func assertNotObservedQualificationJSON(t *testing.T, observation SunSpecQualificationObservation, poll, deadline uint64) {
	t.Helper()
	first, err := json.Marshal(observation)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	second, err := json.Marshal(observation)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("qualification JSON is not deterministic: %v", err)
	}
	var got serializedQualificationObservation
	if err := json.Unmarshal(first, &got); err != nil {
		t.Fatalf("unmarshal qualification JSON: %v", err)
	}
	if got.Schema != sunSpecQualificationObservationSchema || got.CapabilityReason != sunspec.CapabilityReasonAdmitted || got.CapabilityID != observation.Capability().ProfileID() || got.FlavorReason != SunSpecFroniusFlavorReason("NOT_OBSERVED") || got.FlavorID != "sunspec.flavor.not_observed" || got.SampleID != fmt.Sprintf("sunspec-%d-%d", poll, deadline) || got.SampleIdentity.PollGeneration != poll || got.SampleIdentity.DeadlineIdentity != deadline || !reflect.DeepEqual(got.Chain, observation.Chain()) || !reflect.DeepEqual(got.RawWords, observation.RawWords()) {
		t.Fatalf("serialized observation schema=%s capability=%s/%s flavor=%s/%s sample=%s identity=%+v", got.Schema, got.CapabilityID, got.CapabilityReason, got.FlavorID, got.FlavorReason, got.SampleID, got.SampleIdentity)
	}
	if len(got.Occurrences) != len(observation.Occurrences()) || len(got.SourceViews) != len(observation.SourceViews()) || len(got.Occurrences) == 0 || len(got.SourceViews) == 0 {
		t.Fatalf("serialized evidence occurrences=%d source_views=%d", len(got.Occurrences), len(got.SourceViews))
	}
	for index, gotOccurrence := range got.Occurrences {
		wantOccurrence := observation.Occurrences()[index]
		wantDecoderKey, wantHasDecoderKey := wantOccurrence.DecoderKey()
		if gotOccurrence.Ordinal != wantOccurrence.Ordinal || gotOccurrence.WireKey != wantOccurrence.WireKey || gotOccurrence.SchemaRevision != wantOccurrence.SchemaRevision || gotOccurrence.HeaderOffset != wantOccurrence.HeaderOffset || gotOccurrence.PayloadOffset != wantOccurrence.PayloadOffset || gotOccurrence.Disposition != wantOccurrence.Disposition || !reflect.DeepEqual(gotOccurrence.Words, wantOccurrence.Words()) || !reflect.DeepEqual(gotOccurrence.SourceSpans, wantOccurrence.SourceSpans()) || (gotOccurrence.DecoderKey != nil) != wantHasDecoderKey || wantHasDecoderKey && *gotOccurrence.DecoderKey != wantDecoderKey {
			t.Fatalf("serialized occurrence %d=%#v does not retain source=%#v", index, gotOccurrence, wantOccurrence)
		}
	}
	for index, gotView := range got.SourceViews {
		wantView := observation.SourceViews()[index].Record()
		if gotView.LogicalViewID != wantView.LogicalViewID || gotView.WireResponseID != wantView.WireResponseID || gotView.PhysicalRequestID != wantView.PhysicalRequestID || gotView.Endpoint != wantView.Endpoint || gotView.ConnectionID != wantView.ConnectionID || gotView.Transport != wantView.Transport || gotView.TransportGeneration != wantView.TransportGeneration || gotView.UnitID != wantView.UnitID || gotView.RequestedFunction != wantView.RequestedFunction || gotView.ReceivedFunction != wantView.ReceivedFunction || gotView.Table != wantView.Table || gotView.PhysicalOffset != wantView.PhysicalOffset || gotView.PhysicalWordCount != wantView.PhysicalWordCount || gotView.AuthorizationScope != wantView.AuthorizationScope || gotView.PollGeneration != wantView.PollGeneration || gotView.DeadlineIdentity != wantView.DeadlineIdentity || gotView.LogicalOffset != wantView.LogicalOffset || gotView.LogicalWordCount != wantView.LogicalWordCount || gotView.SliceOffset != wantView.SliceOffset || gotView.SliceWordCount != wantView.SliceWordCount || !reflect.DeepEqual(gotView.Words, wantView.Words) || !reflect.DeepEqual(gotView.WireResponseBytes, wantView.WireResponseBytes) {
			t.Fatalf("serialized source view %d does not retain source", index)
		}
		if gotView.PollGeneration != poll || gotView.DeadlineIdentity != deadline {
			t.Fatalf("serialized source view %d identity poll=%d deadline=%d", index, gotView.PollGeneration, gotView.DeadlineIdentity)
		}
	}
}

func qualificationSnapshotWithCommonIdentity(t *testing.T, registry sunspec.DecoderRegistry, manufacturer, model, firmware string) sunspec.ChainSnapshot {
	t.Helper()
	occurrences := qualificationSnapshot(t, registry).Occurrences()
	occurrences[0] = sunspecfixture.CommonOccurrence(t, registry, manufacturer, model, firmware, 1)
	return qualificationSnapshotWithOccurrences(t, registry, occurrences)
}

func withQualificationIdentity(t *testing.T, snapshot sunspec.ChainSnapshot, poll, deadline uint64) sunspec.ChainSnapshot {
	t.Helper()
	return sunspecfixture.WithRecords(t, snapshot, func(records []modbusreg.LogicalViewRecord) []modbusreg.LogicalViewRecord {
		for index := range records {
			records[index].PollGeneration = poll
			records[index].DeadlineIdentity = deadline
		}
		return records
	})
}

func qualificationSnapshot(t *testing.T, registry sunspec.DecoderRegistry) sunspec.ChainSnapshot {
	t.Helper()
	v11 := froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	return qualificationSnapshotWithOccurrences(t, registry, v11.Occurrences())
}

type qualificationObservationManifest struct {
	Schema           string                     `json:"schema"`
	CapabilityID     string                     `json:"capability_id"`
	CapabilityReason sunspec.CapabilityReason   `json:"capability_reason"`
	FlavorID         string                     `json:"flavor_id"`
	FlavorReason     SunSpecFroniusFlavorReason `json:"flavor_reason"`
	SampleID         string                     `json:"sample_id"`
	SampleIdentity   sampleIdentityDTO          `json:"sample_identity"`
	Chain            []sunspec.WireKey          `json:"chain"`
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
	Ordinal        uint32                   `json:"ordinal"`
	WireKey        sunspec.WireKey          `json:"wire_key"`
	SchemaRevision sunspec.SchemaRevision   `json:"schema_revision"`
	HeaderOffset   uint16                   `json:"header_offset"`
	PayloadOffset  uint16                   `json:"payload_offset"`
	Disposition    sunspec.ChainDisposition `json:"disposition"`
	DecoderKey     *sunspec.DecoderKey      `json:"decoder_key"`
	Words          []uint16                 `json:"words"`
	SourceSpans    []sunspec.SourceSpan     `json:"source_spans"`
}

type serializedLogicalView struct {
	LogicalViewID       uint64                    `json:"logical_view_id"`
	WireResponseID      uint64                    `json:"wire_response_id"`
	PhysicalRequestID   uint64                    `json:"physical_request_id"`
	Endpoint            string                    `json:"endpoint"`
	ConnectionID        uint64                    `json:"connection_id"`
	Transport           modbusreg.TransportFamily `json:"transport"`
	TransportGeneration uint64                    `json:"transport_generation"`
	UnitID              byte                      `json:"unit_id"`
	RequestedFunction   modbusreg.FunctionCode    `json:"requested_function"`
	ReceivedFunction    modbusreg.FunctionCode    `json:"received_function"`
	Table               modbusreg.LogicalTable    `json:"table"`
	PhysicalOffset      uint16                    `json:"physical_offset"`
	PhysicalWordCount   uint16                    `json:"physical_word_count"`
	AuthorizationScope  string                    `json:"authorization_scope"`
	PollGeneration      uint64                    `json:"poll_generation"`
	DeadlineIdentity    uint64                    `json:"deadline_identity"`
	LogicalOffset       uint16                    `json:"logical_offset"`
	LogicalWordCount    uint16                    `json:"logical_word_count"`
	SliceOffset         uint16                    `json:"slice_offset"`
	SliceWordCount      uint16                    `json:"slice_word_count"`
	Words               []uint16                  `json:"words"`
	WireResponseBytes   []byte                    `json:"wire_response_bytes"`
}

func qualificationSnapshotWithOccurrences(t *testing.T, registry sunspec.DecoderRegistry, occurrences []sunspec.Occurrence) sunspec.ChainSnapshot {
	t.Helper()
	return sunspecfixture.Walk(t, registry.DecoderKeys(), func(_ int, record *modbusreg.LogicalViewRecord) {
		record.DeadlineIdentity = 700
	}, occurrences...)
}

func qualificationSnapshotWithCoalescedTCPView(t *testing.T, registry sunspec.DecoderRegistry) (sunspec.ChainSnapshot, modbusreg.LogicalViewRecord) {
	t.Helper()
	baseline := qualificationSnapshot(t, registry)
	// The first occurrence span names the view to deliver as a slice of a
	// coalesced physical read; the walk assigns view identifiers by step.
	var target uint64
	for _, occurrence := range baseline.Occurrences() {
		if spans := occurrence.SourceSpans(); len(spans) > 0 {
			target = spans[0].LogicalViewID
			break
		}
	}
	if target < sunspecfixture.FirstLogicalViewID {
		t.Fatal("qualification snapshot has no source view")
	}
	snapshot := sunspecfixture.Walk(t, registry.DecoderKeys(), func(step int, record *modbusreg.LogicalViewRecord) {
		record.DeadlineIdentity = 700
		if uint64(step) != target-sunspecfixture.FirstLogicalViewID || record.LogicalOffset == 0 {
			return
		}
		record.PhysicalOffset = record.LogicalOffset - 1
		record.PhysicalWordCount = record.LogicalWordCount + 2
		record.SliceOffset = 1
		record.SliceWordCount = record.LogicalWordCount
	}, baseline.Occurrences()...)
	for _, record := range sunspecfixture.Records(snapshot) {
		if record.LogicalViewID == target && record.SliceOffset == 1 {
			return snapshot, record
		}
	}
	t.Fatal("qualification snapshot has no coalesced source view")
	return sunspec.ChainSnapshot{}, modbusreg.LogicalViewRecord{}
}
