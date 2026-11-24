package fronius

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/internal/sunspecfixture"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
)

func TestFroniusHardwareTestLevelRecordsEachObservedFlavorField(t *testing.T) {
	level := froniusObservedHardwareTestLevel()
	want := []struct {
		id       string
		firmware string
		chain    []sunspec.WireKey
	}{
		{SunSpecFroniusObservedFlavorID, "1.41.11-1", froniusObservedChainV1},
		{SunSpecFroniusObservedFlavorV11ID, "1.41.11-1", froniusObservedChainV11},
		{SunSpecFroniusObservedFlavorV12ID, "1.42.9-1", froniusObservedChainV12},
	}
	if len(level) != len(want) {
		t.Fatalf("hardware test level count=%d", len(level))
	}
	for index, entry := range want {
		got := level[index]
		if got.FlavorID != entry.id {
			t.Fatalf("flavor_id[%d]=%q want %q", index, got.FlavorID, entry.id)
		}
		if got.Firmware != entry.firmware {
			t.Fatalf("firmware[%d]=%q want %q", index, got.Firmware, entry.firmware)
		}
		if !reflect.DeepEqual(got.Chain, entry.chain) {
			t.Fatalf("chain[%d]=%v want %v", index, got.Chain, entry.chain)
		}
	}
	if level[0].Firmware != level[1].Firmware || reflect.DeepEqual(level[0].Chain, level[1].Chain) || level[0].FlavorID == level[1].FlavorID {
		t.Fatal("versions 1.0.0 and 1.1.0 do not differ by chain and flavor id at the same firmware")
	}
	if level[1].Firmware == level[2].Firmware || !reflect.DeepEqual(level[1].Chain, level[2].Chain) || level[1].FlavorID == level[2].FlavorID {
		t.Fatal("versions 1.1.0 and 1.2.0 do not differ by firmware and flavor id on the same chain")
	}
	mutated := froniusObservedHardwareTestLevel()
	mutated[0].FlavorID = "mutated-id"
	mutated[0].Firmware = "mutated-firmware"
	mutated[0].Chain[0].ModelID++
	mutated[1].Chain[0].ModelLength++
	fresh := froniusObservedHardwareTestLevel()
	if fresh[0].FlavorID != want[0].id || fresh[0].Firmware != want[0].firmware || fresh[0].Chain[0].ModelID != want[0].chain[0].ModelID || fresh[1].Chain[0].ModelLength != want[1].chain[0].ModelLength {
		t.Fatal("hardware test level leaked a field mutation")
	}
}

func TestFroniusObservedFlavorMatchesOnlyExactEvidenceTuple(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	snapshot := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	decision := EvaluateObservedFlavor(registry, snapshot)
	if !decision.Matched() || decision.Reason() != SunSpecFroniusFlavorReasonMatched || decision.FlavorID() != SunSpecFroniusObservedFlavorID {
		t.Fatalf("matched=%t reason=%q flavor=%q", decision.Matched(), decision.Reason(), decision.FlavorID())
	}
	if !decision.Capability().Admitted() || len(decision.Chain()) != 8 || len(decision.SourceViews()) != len(snapshot.SourceViews()) {
		t.Fatalf("capability=%t chain=%v views=%d", decision.Capability().Admitted(), decision.Chain(), len(decision.SourceViews()))
	}
	chain := decision.Chain()
	chain[0] = sunspec.WireKey{}
	if decision.Chain()[0] != (sunspec.WireKey{ModelID: 1, ModelLength: 65}) {
		t.Fatal("flavor chain mutated through accessor")
	}
}

func TestFroniusFlavorDecisionZeroValuePreservesV1Identity(t *testing.T) {
	var decision SunSpecFroniusFlavorDecision
	if decision.FlavorID() != SunSpecFroniusObservedFlavorID {
		t.Fatalf("zero-value flavor id=%q want=%q", decision.FlavorID(), SunSpecFroniusObservedFlavorID)
	}
}

func TestFroniusObservedFlavorAcceptsCompletedPublicChainSnapshot(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	fixture := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	snapshot := sunspecfixture.Snapshot(t, registry, fixture.Occurrences()...)
	decision := EvaluateObservedFlavor(registry, snapshot)
	if !decision.Matched() || !decision.Capability().Admitted() {
		t.Fatalf("matched=%t flavor_reason=%q capability_reason=%q", decision.Matched(), decision.Reason(), decision.Capability().Reason())
	}
	if len(snapshot.SourceViews()) <= len(snapshot.Occurrences()) {
		t.Fatalf("production snapshot did not retain split signature/header/payload/terminal views: views=%d occurrences=%d", len(snapshot.SourceViews()), len(snapshot.Occurrences()))
	}
}

func TestCapabilityAndFlavorRejectInvalidCommonAndAdmittedGeometry(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	valid := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	invalidCommon := valid.Occurrences()
	invalidCommon[0] = sunspecfixture.CommonOccurrence(t, registry, "", "Symo GEN24 10.0", "1.41.11-1", 1)
	invalidGeometry := valid.Occurrences()
	invalidGeometry[5] = sunspecfixture.Occurrence(t, registry, sunspecfixture.ModelWords(t, registry, 160, 88, map[string][]uint16{"N": {3}}), 6)

	for name, occurrences := range map[string][]sunspec.Occurrence{
		"invalid mandatory Common":   invalidCommon,
		"invalid Model 160 geometry": invalidGeometry,
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := sunspecfixture.Snapshot(t, registry, occurrences...)
			capability := registry.EvaluateThreePhaseMonitoring(snapshot)
			if capability.Admitted() || capability.Reason() != sunspec.CapabilityReasonInvalidChain {
				t.Fatalf("capability admitted=%t reason=%q", capability.Admitted(), capability.Reason())
			}
			flavor := EvaluateObservedFlavor(registry, snapshot)
			if flavor.Matched() || flavor.Reason() != SunSpecFroniusFlavorReasonCapabilityNotAdmitted {
				t.Fatalf("flavor matched=%t reason=%q", flavor.Matched(), flavor.Reason())
			}
		})
	}
}

func TestCapabilityAndFlavorRejectRetainedUnadmittedModel160Geometry(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	fixture := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	occurrences := fixture.Occurrences()
	occurrences[5] = sunspecfixture.Occurrence(t, registry, sunspecfixture.ModelWords(t, registry, 160, 88, map[string][]uint16{"N": {3}}), 6)
	keys := registry.DecoderKeys()
	withoutExactMPPT := keys[:0]
	for _, key := range keys {
		if key.ModelID != 160 || key.ModelLength != 88 {
			withoutExactMPPT = append(withoutExactMPPT, key)
		}
	}
	snapshot := sunspecfixture.SnapshotWithKeys(t, withoutExactMPPT, occurrences...)
	if model160 := snapshot.ByModelID(160); len(model160) != 1 || model160[0].Disposition != sunspec.ChainDispositionUnsupportedLength {
		t.Fatalf("retained Model 160=%#v", model160)
	}
	capability := registry.EvaluateThreePhaseMonitoring(snapshot)
	if capability.Admitted() || capability.Reason() != sunspec.CapabilityReasonInvalidChain {
		t.Fatalf("capability admitted=%t reason=%q", capability.Admitted(), capability.Reason())
	}
	flavor := EvaluateObservedFlavor(registry, snapshot)
	if flavor.Matched() || flavor.Reason() != SunSpecFroniusFlavorReasonCapabilityNotAdmitted {
		t.Fatalf("flavor matched=%t reason=%q", flavor.Matched(), flavor.Reason())
	}
}

func TestFroniusObservedFlavorReasonsAreClosedAndFailClosed(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	valid := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	identity := froniusObservedSnapshot(t, registry, "fronius", "Symo GEN24 10.0", "1.41.11-1")
	firmware := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-2")
	chainOccurrences := valid.Occurrences()
	chainOccurrences[2], chainOccurrences[3] = chainOccurrences[3], chainOccurrences[2]
	for index := range chainOccurrences {
		chainOccurrences[index].Ordinal = uint32(index + 1)
	}
	chain := sunspecfixture.Snapshot(t, registry, chainOccurrences...)
	invalidCapabilityOccurrences := valid.Occurrences()
	invalidCapabilityOccurrences[1] = sunspecfixture.InverterOccurrence(t, registry, 113, map[string][]uint16{"St": {99}}, 2)
	invalidCapability := sunspecfixture.Snapshot(t, registry, invalidCapabilityOccurrences...)
	ambiguousOccurrences := valid.Occurrences()
	ambiguousOccurrences = append(ambiguousOccurrences[:2], append([]sunspec.Occurrence{sunspecfixture.InverterOccurrence(t, registry, 103, nil, 3)}, ambiguousOccurrences[2:]...)...)
	for index := range ambiguousOccurrences {
		ambiguousOccurrences[index].Ordinal = uint32(index + 1)
	}
	ambiguous := sunspecfixture.Snapshot(t, registry, ambiguousOccurrences...)

	tests := map[string]struct {
		snapshot sunspec.ChainSnapshot
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
			decision := EvaluateObservedFlavor(registry, tc.snapshot)
			if decision.Matched() || decision.Reason() != tc.reason {
				t.Fatalf("matched=%t reason=%q want=%q", decision.Matched(), decision.Reason(), tc.reason)
			}
		})
	}
}

func TestFroniusObservedFlavorMapsMalformedSnapshotToCapabilityFailure(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	valid := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	missingTerminal := sunspecfixture.WithRaw(t, valid, func(raw []uint16) []uint16 { return raw[:len(raw)-2] })
	trailing := sunspecfixture.WithRaw(t, valid, func(raw []uint16) []uint16 { return append(raw, 0) })

	for name, snapshot := range map[string]sunspec.ChainSnapshot{
		"missing terminal": missingTerminal,
		"trailing words":   trailing,
	} {
		t.Run(name, func(t *testing.T) {
			decision := EvaluateObservedFlavor(registry, snapshot)
			if decision.Matched() || decision.Reason() != SunSpecFroniusFlavorReasonCapabilityNotAdmitted {
				t.Fatalf("matched=%t reason=%q", decision.Matched(), decision.Reason())
			}
		})
	}
}

func TestFroniusObservedFlavorFixtureIsExactAndNonActionable(t *testing.T) {
	raw, err := os.ReadFile("../testdata/sunspec/chains/fronius_gen24_float_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Schema       string            `json:"schema"`
		FlavorID     string            `json:"flavor_id"`
		Manufacturer string            `json:"manufacturer"`
		Model        string            `json:"model"`
		Firmware     string            `json:"firmware"`
		Chain        []sunspec.WireKey `json:"chain"`
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
	wantChain := []sunspec.WireKey{{ModelID: 1, ModelLength: 65}, {ModelID: 113, ModelLength: 60}, {ModelID: 120, ModelLength: 26}, {ModelID: 121, ModelLength: 30}, {ModelID: 122, ModelLength: 44}, {ModelID: 160, ModelLength: 88}, {ModelID: 124, ModelLength: 24}, {ModelID: 0xffff, ModelLength: 0}}
	if fixture.Schema != "helianthus-sunspec-fronius-observed-flavor/v1" || fixture.FlavorID != SunSpecFroniusObservedFlavorID || fixture.Manufacturer != "Fronius" || fixture.Model != "Symo GEN24 10.0" || fixture.Firmware != "1.41.11-1" || !reflect.DeepEqual(fixture.Chain, wantChain) {
		t.Fatalf("fixture=%+v", fixture)
	}
	if fixture.Observation.Function != "FC03" || fixture.Observation.UnitID != 1 || fixture.Observation.PDUOffset != 40000 || fixture.Observation.Authority != "non_actionable_provenance_only" {
		t.Fatalf("observation=%+v", fixture.Observation)
	}
}

func TestFroniusObservedFlavorV11MatchesOnlyTheModel123Chain(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	v1Snapshot := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	v11Snapshot := froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")

	v1 := EvaluateObservedFlavor(registry, v11Snapshot)
	if v1.Matched() || v1.Reason() != SunSpecFroniusFlavorReasonChainMismatch || v1.FlavorID() != SunSpecFroniusObservedFlavorID {
		t.Fatalf("V1 on V1.1 chain matched=%t reason=%q id=%q", v1.Matched(), v1.Reason(), v1.FlavorID())
	}
	v11OnV1 := EvaluateObservedFlavorV11(registry, v1Snapshot)
	if v11OnV1.Matched() || v11OnV1.Reason() != SunSpecFroniusFlavorReasonChainMismatch || v11OnV1.FlavorID() != SunSpecFroniusObservedFlavorV11ID {
		t.Fatalf("V1.1 on V1 chain matched=%t reason=%q id=%q", v11OnV1.Matched(), v11OnV1.Reason(), v11OnV1.FlavorID())
	}
	v11 := EvaluateObservedFlavorV11(registry, v11Snapshot)
	if !v11.Matched() || v11.Reason() != SunSpecFroniusFlavorReasonMatched || v11.FlavorID() != SunSpecFroniusObservedFlavorV11ID {
		t.Fatalf("V1.1 matched=%t reason=%q id=%q", v11.Matched(), v11.Reason(), v11.FlavorID())
	}
	if !v11.Capability().Admitted() || len(v11.Chain()) != 9 || len(v11.SourceViews()) != len(v11Snapshot.SourceViews()) {
		t.Fatalf("V1.1 capability=%t chain=%v views=%d", v11.Capability().Admitted(), v11.Chain(), len(v11.SourceViews()))
	}
}

func TestFroniusObservedFlavorSelectorRequiresExactlyOneMatch(t *testing.T) {
	registry := sunspecfixture.Registry(t)
	for name, snapshot := range map[string]sunspec.ChainSnapshot{
		"v1":   froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1"),
		"v1.1": froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1"),
	} {
		t.Run(name, func(t *testing.T) {
			selection := SelectObservedFlavor(registry, snapshot)
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
			if decision.FlavorID() != wantID || len(selection.Evaluations()) != 3 {
				t.Fatalf("selected id=%q evaluations=%d", decision.FlavorID(), len(selection.Evaluations()))
			}
		})
	}

	noMatch := SelectObservedFlavor(registry, froniusObservedSnapshotV11(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-2"))
	if noMatch.Matched() || noMatch.Reason() != SunSpecFroniusFlavorSelectionReasonNoMatch {
		t.Fatalf("no-match matched=%t reason=%q", noMatch.Matched(), noMatch.Reason())
	}
	if _, ok := noMatch.Decision(); ok {
		t.Fatal("no-match selection exposed a decision")
	}

	matched := EvaluateObservedFlavor(registry, froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1"))
	ambiguous := selectSunSpecFroniusFlavor([]SunSpecFroniusFlavorDecision{matched, matched})
	if ambiguous.Matched() || ambiguous.Reason() != SunSpecFroniusFlavorSelectionReasonAmbiguousMatch {
		t.Fatalf("ambiguous matched=%t reason=%q", ambiguous.Matched(), ambiguous.Reason())
	}
}

func TestFroniusObservedFlavorV11FixtureIsExactAndNonActionable(t *testing.T) {
	raw, err := os.ReadFile("../testdata/sunspec/chains/fronius_gen24_float_v1_1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Schema       string            `json:"schema"`
		FlavorID     string            `json:"flavor_id"`
		Manufacturer string            `json:"manufacturer"`
		Model        string            `json:"model"`
		Firmware     string            `json:"firmware"`
		Chain        []sunspec.WireKey `json:"chain"`
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
	wantChain := []sunspec.WireKey{{ModelID: 1, ModelLength: 65}, {ModelID: 113, ModelLength: 60}, {ModelID: 120, ModelLength: 26}, {ModelID: 121, ModelLength: 30}, {ModelID: 122, ModelLength: 44}, {ModelID: 123, ModelLength: 24}, {ModelID: 160, ModelLength: 88}, {ModelID: 124, ModelLength: 24}, {ModelID: 0xffff, ModelLength: 0}}
	if fixture.Schema != "helianthus-sunspec-fronius-observed-flavor/v1.1" || fixture.FlavorID != SunSpecFroniusObservedFlavorV11ID || fixture.Manufacturer != "Fronius" || fixture.Model != "Symo GEN24 10.0" || fixture.Firmware != "1.41.11-1" || !reflect.DeepEqual(fixture.Chain, wantChain) {
		t.Fatalf("fixture=%+v", fixture)
	}
	if fixture.Observation.Function != "FC03" || fixture.Observation.UnitID != 1 || fixture.Observation.PDUOffset != 40000 || fixture.Observation.Authority != "non_actionable_provenance_only" {
		t.Fatalf("observation=%+v", fixture.Observation)
	}
}

func froniusObservedSnapshot(t *testing.T, registry sunspec.DecoderRegistry, manufacturer, model, firmware string) sunspec.ChainSnapshot {
	t.Helper()
	occurrences := []sunspec.Occurrence{
		sunspecfixture.CommonOccurrence(t, registry, manufacturer, model, firmware, 1),
		sunspecfixture.InverterOccurrence(t, registry, 113, nil, 2),
		sunspecfixture.Occurrence(t, registry, sunspecfixture.ModelWords(t, registry, 120, 26, nil), 3),
		sunspecfixture.Occurrence(t, registry, sunspecfixture.ModelWords(t, registry, 121, 30, nil), 4),
		sunspecfixture.Occurrence(t, registry, sunspecfixture.ModelWords(t, registry, 122, 44, nil), 5),
		sunspecfixture.Occurrence(t, registry, sunspecfixture.ModelWords(t, registry, 160, 88, map[string][]uint16{"N": {4}}), 6),
		sunspecfixture.Occurrence(t, registry, sunspecfixture.ModelWords(t, registry, 124, 24, nil), 7),
	}
	return sunspecfixture.Snapshot(t, registry, occurrences...)
}

func froniusObservedSnapshotV11(t *testing.T, registry sunspec.DecoderRegistry, manufacturer, model, firmware string) sunspec.ChainSnapshot {
	t.Helper()
	// This synthetic replay represents only the inverter points on the card.
	// Absent optional float points must not use the fixture builder's zero fill.
	unsupported := map[string][]uint16{
		"StVnd": {0xffff}, "EvtVnd1": {0xffff, 0xffff}, "EvtVnd2": {0xffff, 0xffff},
		"EvtVnd3": {0xffff, 0xffff}, "EvtVnd4": {0xffff, 0xffff},
	}
	for _, point := range []string{"PF", "DCA", "DCV", "DCW", "TmpSnk", "TmpTrns", "TmpOt"} {
		unsupported[point] = []uint16{0x7fc0, 0}
	}
	occurrences := []sunspec.Occurrence{
		sunspecfixture.CommonOccurrence(t, registry, manufacturer, model, firmware, 1),
		sunspecfixture.InverterOccurrence(t, registry, 113, unsupported, 2),
		sunspecfixture.Occurrence(t, registry, sunspecfixture.ModelWords(t, registry, 120, 26, nil), 3),
		sunspecfixture.Occurrence(t, registry, sunspecfixture.ModelWords(t, registry, 121, 30, nil), 4),
		sunspecfixture.Occurrence(t, registry, sunspecfixture.ModelWords(t, registry, 122, 44, nil), 5),
		sunspecfixture.Occurrence(t, registry, sunspecfixture.ModelWords(t, registry, 123, 24, sunspecfixture.Model123Values()), 6),
		sunspecfixture.Occurrence(t, registry, sunspecfixture.ModelWords(t, registry, 160, 88, map[string][]uint16{"N": {4}}), 7),
		sunspecfixture.Occurrence(t, registry, sunspecfixture.ModelWords(t, registry, 124, 24, nil), 8),
	}
	return sunspecfixture.Snapshot(t, registry, occurrences...)
}
