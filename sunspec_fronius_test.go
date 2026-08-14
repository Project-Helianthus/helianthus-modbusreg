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

func TestFroniusObservedFlavorReasonsAreClosedAndFailClosed(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	valid := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-1")
	identity := froniusObservedSnapshot(t, registry, "fronius", "Symo GEN24 10.0", "1.41.11-1")
	firmware := froniusObservedSnapshot(t, registry, "Fronius", "Symo GEN24 10.0", "1.41.11-2")
	chain := valid
	chain.occurrences[2], chain.occurrences[3] = chain.occurrences[3], chain.occurrences[2]
	chain.raw = rawWordsForOccurrences(chain.occurrences)
	invalidCapability := valid
	invalidCapability.occurrences[1] = inverterOccurrence(t, registry, 113, map[string][]uint16{"St": {99}}, 2)
	invalidCapability.raw = rawWordsForOccurrences(invalidCapability.occurrences)
	ambiguous := valid
	ambiguous.occurrences = append(ambiguous.occurrences[:2], append([]SunSpecOccurrence{inverterOccurrence(t, registry, 103, nil, 3)}, ambiguous.occurrences[2:]...)...)
	for index := range ambiguous.occurrences {
		ambiguous.occurrences[index].Ordinal = uint32(index + 1)
	}
	ambiguous.raw = rawWordsForOccurrences(ambiguous.occurrences)

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
