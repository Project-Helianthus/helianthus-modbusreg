package modbusreg

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestFroniusObservedFlavorMatchesOnlyExactEvidenceTuple(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	snapshot := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	decision := registry.EvaluateFroniusObservedFlavor(snapshot)
	if !decision.Matched() || decision.Reason() != SunSpecFroniusFlavorReasonMatched || decision.FlavorID() != SunSpecFroniusObservedFlavorID {
		t.Fatalf("matched=%t reason=%q flavor=%q", decision.Matched(), decision.Reason(), decision.FlavorID())
	}
	if !decision.Capability().Admitted() || len(decision.Chain()) != 8 || len(decision.SourceViews()) != len(snapshot.SourceViews()) {
		t.Fatalf("capability=%t chain=%v views=%d", decision.Capability().Admitted(), decision.Chain(), len(decision.SourceViews()))
	}
	chain := decision.Chain()
	chain[0] = SunSpecWireKey{}
	if decision.Chain()[0] != (SunSpecWireKey{ModelID: 1, ModelLength: 65}) {
		t.Fatal("flavor chain mutated through accessor")
	}
}

func TestFroniusObservedFlavorAcceptsCompletedPublicChainSnapshot(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	fixture := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	snapshot := completedChainSnapshot(t, registry, fixture.Occurrences()...)
	decision := registry.EvaluateFroniusObservedFlavor(snapshot)
	if !decision.Matched() || !decision.Capability().Admitted() {
		t.Fatalf("matched=%t flavor_reason=%q capability_reason=%q", decision.Matched(), decision.Reason(), decision.Capability().Reason())
	}
	if len(snapshot.SourceViews()) <= len(snapshot.Occurrences()) {
		t.Fatalf("production snapshot did not retain split signature/header/payload/terminal views: views=%d occurrences=%d", len(snapshot.SourceViews()), len(snapshot.Occurrences()))
	}
}

func TestCapabilityAndFlavorRejectInvalidCommonAndAdmittedGeometry(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	valid := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	invalidCommon := valid.Occurrences()
	invalidCommon[0] = commonOccurrence(t, registry, "", "Symo GEN24 10.0", "1.41.11-1", 1)
	invalidGeometry := valid.Occurrences()
	invalidGeometry[5] = admittedOccurrence(160, 88, modelWords(t, registry, 160, 88, map[string][]uint16{"N": {3}}), 6)

	for name, occurrences := range map[string][]SunSpecOccurrence{
		"invalid mandatory Common":   invalidCommon,
		"invalid Model 160 geometry": invalidGeometry,
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := completedChainSnapshot(t, registry, occurrences...)
			capability := registry.EvaluateThreePhaseMonitoring(snapshot)
			if capability.Admitted() || capability.Reason() != SunSpecCapabilityReasonInvalidChain {
				t.Fatalf("capability admitted=%t reason=%q", capability.Admitted(), capability.Reason())
			}
			flavor := registry.EvaluateFroniusObservedFlavor(snapshot)
			if flavor.Matched() || flavor.Reason() != SunSpecFroniusFlavorReasonCapabilityNotAdmitted {
				t.Fatalf("flavor matched=%t reason=%q", flavor.Matched(), flavor.Reason())
			}
		})
	}
}

func TestCapabilityAndFlavorRejectRetainedUnadmittedModel160Geometry(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	fixture := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	occurrences := fixture.Occurrences()
	occurrences[5] = admittedOccurrence(160, 88, modelWords(t, registry, 160, 88, map[string][]uint16{"N": {3}}), 6)
	keys := registry.DecoderKeys()
	withoutExactMPPT := keys[:0]
	for _, key := range keys {
		if key.ModelID != 160 || key.ModelLength != 88 {
			withoutExactMPPT = append(withoutExactMPPT, key)
		}
	}
	snapshot := completedChainSnapshotWithKeys(t, withoutExactMPPT, occurrences...)
	if model160 := snapshot.ByModelID(160); len(model160) != 1 || model160[0].Disposition != SunSpecChainDispositionUnsupportedLength {
		t.Fatalf("retained Model 160=%#v", model160)
	}
	capability := registry.EvaluateThreePhaseMonitoring(snapshot)
	if capability.Admitted() || capability.Reason() != SunSpecCapabilityReasonInvalidChain {
		t.Fatalf("capability admitted=%t reason=%q", capability.Admitted(), capability.Reason())
	}
	flavor := registry.EvaluateFroniusObservedFlavor(snapshot)
	if flavor.Matched() || flavor.Reason() != SunSpecFroniusFlavorReasonCapabilityNotAdmitted {
		t.Fatalf("flavor matched=%t reason=%q", flavor.Matched(), flavor.Reason())
	}
}

func TestFroniusObservedFlavorReasonsAreClosedAndFailClosed(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	valid := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	identity := froniusObservedSnapshot(t, registry, "fronius", "Symo GEN24 10.0", "1.41.11-1")
	firmware := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-2")
	chainOccurrences := valid.Occurrences()
	chainOccurrences[2], chainOccurrences[3] = chainOccurrences[3], chainOccurrences[2]
	for index := range chainOccurrences {
		chainOccurrences[index].Ordinal = uint32(index + 1)
	}
	chain := snapshotFromOccurrences(chainOccurrences...)
	invalidCapability := cloneSnapshotForTest(valid)
	invalidCapability.occurrences[1] = inverterOccurrence(t, registry, 113, map[string][]uint16{"St": {99}}, 2)
	invalidCapability = snapshotFromOccurrences(invalidCapability.occurrences...)
	ambiguousOccurrences := valid.Occurrences()
	ambiguousOccurrences = append(ambiguousOccurrences[:2], append([]SunSpecOccurrence{inverterOccurrence(t, registry, 103, nil, 3)}, ambiguousOccurrences[2:]...)...)
	for index := range ambiguousOccurrences {
		ambiguousOccurrences[index].Ordinal = uint32(index + 1)
	}
	ambiguous := snapshotFromOccurrences(ambiguousOccurrences...)

	tests := map[string]struct {
		snapshot SunSpecChainSnapshot
		reason   SunSpecFroniusFlavorReason
	}{
		"identity":   {identity, SunSpecFroniusFlavorReasonCommonIdentityMismatch},
		"firmware":   {firmware, SunSpecFroniusFlavorReasonFirmwareMismatch},
		"chain":      {chain, SunSpecFroniusFlavorReasonChainMismatch},
		"capability": {invalidCapability, SunSpecFroniusFlavorReasonCapabilityNotAdmitted},
		"ambiguous":  {ambiguous, SunSpecFroniusFlavorReasonAmbiguousSource},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			decision := registry.EvaluateFroniusObservedFlavor(tc.snapshot)
			if decision.Matched() || decision.Reason() != tc.reason {
				t.Fatalf("matched=%t reason=%q want=%q", decision.Matched(), decision.Reason(), tc.reason)
			}
		})
	}
}

func TestFroniusObservedFlavorMapsMalformedSnapshotToCapabilityFailure(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	valid := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	missingTerminal := cloneSnapshotForTest(valid)
	missingTerminal.raw = missingTerminal.raw[:len(missingTerminal.raw)-2]
	trailing := cloneSnapshotForTest(valid)
	trailing.raw = append(trailing.raw, 0)

	for name, snapshot := range map[string]SunSpecChainSnapshot{
		"missing terminal": missingTerminal,
		"trailing words":   trailing,
	} {
		t.Run(name, func(t *testing.T) {
			decision := registry.EvaluateFroniusObservedFlavor(snapshot)
			if decision.Matched() || decision.Reason() != SunSpecFroniusFlavorReasonCapabilityNotAdmitted {
				t.Fatalf("matched=%t reason=%q", decision.Matched(), decision.Reason())
			}
		})
	}
}

func TestFroniusObservedFlavorFixtureIsExactAndNonActionable(t *testing.T) {
	raw, err := os.ReadFile("testdata/sunspec/chains/fronius_gen24_float_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Schema       string           `json:"schema"`
		FlavorID     string           `json:"flavor_id"`
		Manufacturer string           `json:"manufacturer"`
		Model        string           `json:"model"`
		Firmware     string           `json:"firmware"`
		Chain        []SunSpecWireKey `json:"chain"`
		Observation  struct {
			Function  string `json:"function"`
			UnitID    uint16 `json:"unit_id"`
			PDUOffset uint16 `json:"pdu_offset"`
			Authority string `json:"authority"`
		} `json:"sanitized_observation"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	wantChain := []SunSpecWireKey{{1, 65}, {113, 60}, {120, 26}, {121, 30}, {122, 44}, {160, 88}, {124, 24}, {0xffff, 0}}
	if fixture.Schema != "helianthus-sunspec-fronius-observed-flavor/v1" || fixture.FlavorID != SunSpecFroniusObservedFlavorID || fixture.Manufacturer != "Fronius" || fixture.Model != "Symo GEN24 10.0" || fixture.Firmware != "1.41.11-1" || !reflect.DeepEqual(fixture.Chain, wantChain) {
		t.Fatalf("fixture=%+v", fixture)
	}
	if fixture.Observation.Function != "FC03" || fixture.Observation.UnitID != 1 || fixture.Observation.PDUOffset != 40000 || fixture.Observation.Authority != "non_actionable_provenance_only" {
		t.Fatalf("observation=%+v", fixture.Observation)
	}
}

func TestFroniusObservedFlavorV11MatchesOnlyTheModel123Chain(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	v1Snapshot := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	v11Snapshot := froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")

	v1 := registry.EvaluateFroniusObservedFlavor(v11Snapshot)
	if v1.Matched() || v1.Reason() != SunSpecFroniusFlavorReasonChainMismatch || v1.FlavorID() != SunSpecFroniusObservedFlavorID {
		t.Fatalf("V1 on V1.1 chain matched=%t reason=%q id=%q", v1.Matched(), v1.Reason(), v1.FlavorID())
	}
	v11OnV1 := registry.EvaluateFroniusObservedFlavorV11(v1Snapshot)
	if v11OnV1.Matched() || v11OnV1.Reason() != SunSpecFroniusFlavorReasonChainMismatch || v11OnV1.FlavorID() != SunSpecFroniusObservedFlavorV11ID {
		t.Fatalf("V1.1 on V1 chain matched=%t reason=%q id=%q", v11OnV1.Matched(), v11OnV1.Reason(), v11OnV1.FlavorID())
	}
	v11 := registry.EvaluateFroniusObservedFlavorV11(v11Snapshot)
	if !v11.Matched() || v11.Reason() != SunSpecFroniusFlavorReasonMatched || v11.FlavorID() != SunSpecFroniusObservedFlavorV11ID {
		t.Fatalf("V1.1 matched=%t reason=%q id=%q", v11.Matched(), v11.Reason(), v11.FlavorID())
	}
	if !v11.Capability().Admitted() || len(v11.Chain()) != 9 || len(v11.SourceViews()) != len(v11Snapshot.SourceViews()) {
		t.Fatalf("V1.1 capability=%t chain=%v views=%d", v11.Capability().Admitted(), v11.Chain(), len(v11.SourceViews()))
	}
}

func TestFroniusObservedFlavorSelectorRequiresExactlyOneMatch(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	for name, snapshot := range map[string]SunSpecChainSnapshot{
		"v1":   froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1"),
		"v1.1": froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1"),
	} {
		t.Run(name, func(t *testing.T) {
			selection := registry.SelectFroniusObservedFlavor(snapshot)
			if !selection.Matched() || selection.Reason() != SunSpecFroniusFlavorSelectionReasonMatched {
				t.Fatalf("selection matched=%t reason=%q", selection.Matched(), selection.Reason())
			}
			decision, ok := selection.Decision()
			if !ok || !decision.Matched() {
				t.Fatalf("selected decision=%#v ok=%v", decision, ok)
			}
			wantID := SunSpecFroniusObservedFlavorID
			if name == "v1.1" {
				wantID = SunSpecFroniusObservedFlavorV11ID
			}
			if decision.FlavorID() != wantID || len(selection.Evaluations()) != 2 {
				t.Fatalf("selected id=%q evaluations=%d", decision.FlavorID(), len(selection.Evaluations()))
			}
		})
	}

	noMatch := registry.SelectFroniusObservedFlavor(froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-2"))
	if noMatch.Matched() || noMatch.Reason() != SunSpecFroniusFlavorSelectionReasonNoMatch {
		t.Fatalf("no-match matched=%t reason=%q", noMatch.Matched(), noMatch.Reason())
	}
	if _, ok := noMatch.Decision(); ok {
		t.Fatal("no-match selection exposed a decision")
	}

	matched := registry.EvaluateFroniusObservedFlavor(froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1"))
	ambiguous := selectSunSpecFroniusFlavor([]SunSpecFroniusFlavorDecision{matched, matched})
	if ambiguous.Matched() || ambiguous.Reason() != SunSpecFroniusFlavorSelectionReasonAmbiguousMatch {
		t.Fatalf("ambiguous matched=%t reason=%q", ambiguous.Matched(), ambiguous.Reason())
	}
}

func TestFroniusObservedFlavorV11FixtureIsExactAndNonActionable(t *testing.T) {
	raw, err := os.ReadFile("testdata/sunspec/chains/fronius_gen24_float_v1_1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Schema       string           `json:"schema"`
		FlavorID     string           `json:"flavor_id"`
		Manufacturer string           `json:"manufacturer"`
		Model        string           `json:"model"`
		Firmware     string           `json:"firmware"`
		Chain        []SunSpecWireKey `json:"chain"`
		Observation  struct {
			Function  string `json:"function"`
			UnitID    uint16 `json:"unit_id"`
			PDUOffset uint16 `json:"pdu_offset"`
			Authority string `json:"authority"`
		} `json:"sanitized_observation"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	wantChain := []SunSpecWireKey{{1, 65}, {113, 60}, {120, 26}, {121, 30}, {122, 44}, {123, 24}, {160, 88}, {124, 24}, {0xffff, 0}}
	if fixture.Schema != "helianthus-sunspec-fronius-observed-flavor/v1.1" || fixture.FlavorID != SunSpecFroniusObservedFlavorV11ID || fixture.Manufacturer != "Fronius" || fixture.Model != "Symo GEN24 10.0" || fixture.Firmware != "1.41.11-1" || !reflect.DeepEqual(fixture.Chain, wantChain) {
		t.Fatalf("fixture=%+v", fixture)
	}
	if fixture.Observation.Function != "FC03" || fixture.Observation.UnitID != 1 || fixture.Observation.PDUOffset != 40000 || fixture.Observation.Authority != "non_actionable_provenance_only" {
		t.Fatalf("observation=%+v", fixture.Observation)
	}
}

func froniusObservedSnapshot(t *testing.T, registry SunSpecDecoderRegistry, manufacturer, model, firmware string) SunSpecChainSnapshot {
	t.Helper()
	occurrences := []SunSpecOccurrence{
		commonOccurrence(t, registry, manufacturer, model, firmware, 1),
		inverterOccurrence(t, registry, 113, nil, 2),
		admittedOccurrence(120, 26, modelWords(t, registry, 120, 26, nil), 3),
		admittedOccurrence(121, 30, modelWords(t, registry, 121, 30, nil), 4),
		admittedOccurrence(122, 44, modelWords(t, registry, 122, 44, nil), 5),
		admittedOccurrence(160, 88, modelWords(t, registry, 160, 88, map[string][]uint16{"N": {4}}), 6),
		admittedOccurrence(124, 24, modelWords(t, registry, 124, 24, nil), 7),
	}
	return snapshotFromOccurrences(occurrences...)
}

func froniusObservedSnapshotV11(t *testing.T, registry SunSpecDecoderRegistry, manufacturer, model, firmware string) SunSpecChainSnapshot {
	t.Helper()
	occurrences := []SunSpecOccurrence{
		commonOccurrence(t, registry, manufacturer, model, firmware, 1),
		inverterOccurrence(t, registry, 113, nil, 2),
		admittedOccurrence(120, 26, modelWords(t, registry, 120, 26, nil), 3),
		admittedOccurrence(121, 30, modelWords(t, registry, 121, 30, nil), 4),
		admittedOccurrence(122, 44, modelWords(t, registry, 122, 44, nil), 5),
		admittedOccurrence(123, 24, modelWords(t, registry, 123, 24, validModel123Values()), 6),
		admittedOccurrence(160, 88, modelWords(t, registry, 160, 88, map[string][]uint16{"N": {4}}), 7),
		admittedOccurrence(124, 24, modelWords(t, registry, 124, 24, nil), 8),
	}
	return snapshotFromOccurrences(occurrences...)
}
