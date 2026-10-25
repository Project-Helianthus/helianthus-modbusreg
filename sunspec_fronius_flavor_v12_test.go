package modbusreg

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

const (
	froniusTestManufacturer = "Fronius"
	froniusTestModel        = "Symo GEN24 10.0"
	froniusTestFirmwareV1   = "1.41.11-1"
	froniusTestFirmwareV12  = "1.42.9-1"
)

// Identity coverage for flavor 1.2.0. Each field of the exact tuple is varied
// independently and each test names the production mutation it rejects:
//
//   - manufacturer: removing or case-folding the Common manufacturer gate;
//   - model: removing or prefix-matching the Common model gate;
//   - firmware: comparing against a shared literal instead of the definition,
//     comparing against another definition, trimming, prefix or range matching,
//     or accepting an empty definition firmware;
//   - ordered model chain: removing the exact chain comparison or comparing
//     against another definition's chain.
//
// Options, serial number and device address are not identity fields; varying
// them must not change the decision.

func TestFroniusObservedFlavorV12AdmitsTheCurrentExactTuple(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	snapshot := froniusObservedSnapshotV12(t, registry, froniusTestManufacturer, froniusTestModel, froniusTestFirmwareV12)

	decision := registry.EvaluateFroniusObservedFlavorV12(snapshot)
	if !decision.Matched() || decision.Reason() != SunSpecFroniusFlavorReasonMatched || decision.FlavorID() != SunSpecFroniusObservedFlavorV12ID {
		t.Fatalf("V1.2 matched=%t reason=%q id=%q", decision.Matched(), decision.Reason(), decision.FlavorID())
	}
	if !decision.Capability().Admitted() || !reflect.DeepEqual(decision.Chain(), froniusObservedChainV12) || len(decision.SourceViews()) != len(snapshot.SourceViews()) {
		t.Fatalf("V1.2 capability=%t chain=%v views=%d", decision.Capability().Admitted(), decision.Chain(), len(decision.SourceViews()))
	}

	selection := registry.SelectFroniusObservedFlavor(snapshot)
	selected, ok := selection.Decision()
	if !selection.Matched() || selection.Reason() != SunSpecFroniusFlavorSelectionReasonMatched || !ok || selected.FlavorID() != SunSpecFroniusObservedFlavorV12ID {
		t.Fatalf("selection matched=%t reason=%q decision=%q ok=%t", selection.Matched(), selection.Reason(), selected.FlavorID(), ok)
	}
	evaluations := selection.Evaluations()
	wantIDs := []string{SunSpecFroniusObservedFlavorID, SunSpecFroniusObservedFlavorV11ID, SunSpecFroniusObservedFlavorV12ID}
	wantReasons := []SunSpecFroniusFlavorReason{SunSpecFroniusFlavorReasonFirmwareMismatch, SunSpecFroniusFlavorReasonFirmwareMismatch, SunSpecFroniusFlavorReasonMatched}
	if len(evaluations) != len(wantIDs) {
		t.Fatalf("evaluations=%d want=%d", len(evaluations), len(wantIDs))
	}
	for index, evaluation := range evaluations {
		if evaluation.FlavorID() != wantIDs[index] || evaluation.Reason() != wantReasons[index] {
			t.Fatalf("evaluation %d id=%q reason=%q want id=%q reason=%q", index, evaluation.FlavorID(), evaluation.Reason(), wantIDs[index], wantReasons[index])
		}
	}
}

func TestFroniusObservedFlavorFirmwareIsExactPerDefinition(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	flavors := []struct {
		name     string
		firmware string
		snapshot func(*testing.T, SunSpecDecoderRegistry, string, string, string) SunSpecChainSnapshot
		evaluate func(SunSpecChainSnapshot) SunSpecFroniusFlavorDecision
		id       string
	}{
		{"v1.0.0", froniusTestFirmwareV1, froniusObservedSnapshot, registry.EvaluateFroniusObservedFlavor, SunSpecFroniusObservedFlavorID},
		{"v1.1.0", froniusTestFirmwareV1, froniusObservedSnapshotV11, registry.EvaluateFroniusObservedFlavorV11, SunSpecFroniusObservedFlavorV11ID},
		{"v1.2.0", froniusTestFirmwareV12, froniusObservedSnapshotV12, registry.EvaluateFroniusObservedFlavorV12, SunSpecFroniusObservedFlavorV12ID},
	}
	candidates := []string{
		froniusTestFirmwareV1, froniusTestFirmwareV12,
		"1.41.11-2", "1.42.9-2", "1.42.9", "1.42.9-", "1.42.9-10", "1.42.9-1-1",
		"1.42.10-1", "1.43.0-1", "1.42.9-1 ", " 1.42.9-1", "V1.42.9-1", "1.41.11-1 ",
	}
	for _, flavor := range flavors {
		t.Run(flavor.name+"/own firmware", func(t *testing.T) {
			decision := flavor.evaluate(flavor.snapshot(t, registry, froniusTestManufacturer, froniusTestModel, flavor.firmware))
			if !decision.Matched() || decision.Reason() != SunSpecFroniusFlavorReasonMatched || decision.FlavorID() != flavor.id {
				t.Fatalf("matched=%t reason=%q id=%q", decision.Matched(), decision.Reason(), decision.FlavorID())
			}
		})
		for _, firmware := range candidates {
			if firmware == flavor.firmware {
				continue
			}
			t.Run(flavor.name+"/"+firmware, func(t *testing.T) {
				decision := flavor.evaluate(flavor.snapshot(t, registry, froniusTestManufacturer, froniusTestModel, firmware))
				if decision.Matched() || decision.Reason() != SunSpecFroniusFlavorReasonFirmwareMismatch || decision.FlavorID() != flavor.id {
					t.Fatalf("firmware %q matched=%t reason=%q id=%q", firmware, decision.Matched(), decision.Reason(), decision.FlavorID())
				}
			})
		}
		for name, version := range map[string][]uint16{
			"unimplemented version": make([]uint16, 8),
			"empty version marker":  append([]uint16{0x0080}, make([]uint16, 7)...),
			"bytes after NUL":       append(stringWords(flavor.firmware, 7), 0x0058),
		} {
			t.Run(flavor.name+"/"+name, func(t *testing.T) {
				snapshot := froniusSnapshotForChain(t, registry, froniusChainForFlavor(flavor.id), map[string][]uint16{
					"Mn": stringWords(froniusTestManufacturer, 16), "Md": stringWords(froniusTestModel, 16), "Vr": version, "SN": stringWords("sanitized", 16),
				})
				decision := flavor.evaluate(snapshot)
				if decision.Matched() || decision.Reason() != SunSpecFroniusFlavorReasonFirmwareMismatch {
					t.Fatalf("matched=%t reason=%q", decision.Matched(), decision.Reason())
				}
			})
		}
	}

	// On the shared 1.1.0/1.2.0 chain the firmware is the only distinguishing
	// gate, so each other firmware must reach every flavor as FIRMWARE_MISMATCH
	// and the selector must report NO_MATCH with that common reason.
	for _, firmware := range []string{froniusTestFirmwareV1 + "x", "1.42.9-2", "1.43.0-1"} {
		t.Run("selection/"+firmware, func(t *testing.T) {
			selection := registry.SelectFroniusObservedFlavor(froniusObservedSnapshotV12(t, registry, froniusTestManufacturer, froniusTestModel, firmware))
			if selection.Matched() || selection.Reason() != SunSpecFroniusFlavorSelectionReasonNoMatch {
				t.Fatalf("selection matched=%t reason=%q", selection.Matched(), selection.Reason())
			}
			if _, ok := selection.Decision(); ok {
				t.Fatal("no-match selection exposed a decision")
			}
			evaluations := selection.Evaluations()
			if len(evaluations) != 3 {
				t.Fatalf("evaluations=%d", len(evaluations))
			}
			for _, evaluation := range evaluations {
				if evaluation.Matched() || evaluation.Reason() != SunSpecFroniusFlavorReasonFirmwareMismatch {
					t.Fatalf("evaluation id=%q matched=%t reason=%q", evaluation.FlavorID(), evaluation.Matched(), evaluation.Reason())
				}
			}
		})
	}
}

func TestFroniusObservedFlavorV12VariesEveryIdentityField(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	tests := []struct {
		name     string
		snapshot SunSpecChainSnapshot
		reason   SunSpecFroniusFlavorReason
	}{
		{"manufacturer lower case", froniusObservedSnapshotV12(t, registry, "fronius", froniusTestModel, froniusTestFirmwareV12), SunSpecFroniusFlavorReasonCommonIdentityMismatch},
		{"manufacturer upper case", froniusObservedSnapshotV12(t, registry, "FRONIUS", froniusTestModel, froniusTestFirmwareV12), SunSpecFroniusFlavorReasonCommonIdentityMismatch},
		{"manufacturer suffix", froniusObservedSnapshotV12(t, registry, "Fronius International", froniusTestModel, froniusTestFirmwareV12), SunSpecFroniusFlavorReasonCommonIdentityMismatch},
		{"model other rating", froniusObservedSnapshotV12(t, registry, froniusTestManufacturer, "Symo GEN24 8.0", froniusTestFirmwareV12), SunSpecFroniusFlavorReasonCommonIdentityMismatch},
		{"model suffix", froniusObservedSnapshotV12(t, registry, froniusTestManufacturer, "Symo GEN24 10.0 Plus", froniusTestFirmwareV12), SunSpecFroniusFlavorReasonCommonIdentityMismatch},
		{"model prefix", froniusObservedSnapshotV12(t, registry, froniusTestManufacturer, "Symo GEN24", froniusTestFirmwareV12), SunSpecFroniusFlavorReasonCommonIdentityMismatch},
		{"model case", froniusObservedSnapshotV12(t, registry, froniusTestManufacturer, "symo gen24 10.0", froniusTestFirmwareV12), SunSpecFroniusFlavorReasonCommonIdentityMismatch},
		{"firmware of 1.0.0 and 1.1.0", froniusObservedSnapshotV12(t, registry, froniusTestManufacturer, froniusTestModel, froniusTestFirmwareV1), SunSpecFroniusFlavorReasonFirmwareMismatch},
		{"firmware build", froniusObservedSnapshotV12(t, registry, froniusTestManufacturer, froniusTestModel, "1.42.9-2"), SunSpecFroniusFlavorReasonFirmwareMismatch},
		{"chain of 1.0.0", froniusObservedSnapshot(t, registry, froniusTestManufacturer, froniusTestModel, froniusTestFirmwareV12), SunSpecFroniusFlavorReasonChainMismatch},
		{"chain reordered", froniusSnapshotForChain(t, registry, []SunSpecWireKey{{1, 65}, {113, 60}, {120, 26}, {121, 30}, {122, 44}, {160, 88}, {123, 24}, {124, 24}, {0xffff, 0}}, froniusCommonValues(froniusTestManufacturer, froniusTestModel, froniusTestFirmwareV12)), SunSpecFroniusFlavorReasonChainMismatch},
		{"chain duplicate model", froniusSnapshotForChain(t, registry, []SunSpecWireKey{{1, 65}, {113, 60}, {120, 26}, {120, 26}, {121, 30}, {122, 44}, {123, 24}, {160, 88}, {124, 24}, {0xffff, 0}}, froniusCommonValues(froniusTestManufacturer, froniusTestModel, froniusTestFirmwareV12)), SunSpecFroniusFlavorReasonChainMismatch},
		{"chain missing trailing model", froniusSnapshotForChain(t, registry, []SunSpecWireKey{{1, 65}, {113, 60}, {120, 26}, {121, 30}, {122, 44}, {123, 24}, {160, 88}, {0xffff, 0}}, froniusCommonValues(froniusTestManufacturer, froniusTestModel, froniusTestFirmwareV12)), SunSpecFroniusFlavorReasonChainMismatch},
	}
	// The 1.1.0 tuple on the shared chain is the only variation that selects
	// another flavor; every other variation selects nothing.
	otherSelection := map[string]string{"firmware of 1.0.0 and 1.1.0": SunSpecFroniusObservedFlavorV11ID}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !registry.EvaluateThreePhaseMonitoring(tc.snapshot).Admitted() {
				t.Fatal("identity variation must keep the generic capability admitted")
			}
			decision := registry.EvaluateFroniusObservedFlavorV12(tc.snapshot)
			if decision.Matched() || decision.Reason() != tc.reason || decision.FlavorID() != SunSpecFroniusObservedFlavorV12ID {
				t.Fatalf("matched=%t reason=%q want=%q id=%q", decision.Matched(), decision.Reason(), tc.reason, decision.FlavorID())
			}
			selection := registry.SelectFroniusObservedFlavor(tc.snapshot)
			selected, ok := selection.Decision()
			if want := otherSelection[tc.name]; want != "" {
				if !selection.Matched() || !ok || selected.FlavorID() != want {
					t.Fatalf("selection matched=%t reason=%q decision=%q want=%q", selection.Matched(), selection.Reason(), selected.FlavorID(), want)
				}
				return
			}
			if selection.Matched() || ok || selection.Reason() != SunSpecFroniusFlavorSelectionReasonNoMatch {
				t.Fatalf("selection matched=%t reason=%q decision=%q", selection.Matched(), selection.Reason(), selected.FlavorID())
			}
		})
	}
}

func TestFroniusObservedFlavorV12IgnoresOptionsSerialAndDeviceAddress(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	for name, override := range map[string]map[string][]uint16{
		"options present":    {"Opt": stringWords("opt", 8)},
		"other serial":       {"SN": stringWords("sanitized-other", 16)},
		"device address 2":   {"DA": {2}},
		"device address 247": {"DA": {247}},
	} {
		t.Run(name, func(t *testing.T) {
			values := froniusCommonValues(froniusTestManufacturer, froniusTestModel, froniusTestFirmwareV12)
			for point, words := range override {
				values[point] = words
			}
			selection := registry.SelectFroniusObservedFlavor(froniusSnapshotForChain(t, registry, froniusObservedChainV12, values))
			decision, ok := selection.Decision()
			if !selection.Matched() || !ok || decision.FlavorID() != SunSpecFroniusObservedFlavorV12ID {
				t.Fatalf("selection matched=%t reason=%q decision=%q", selection.Matched(), selection.Reason(), decision.FlavorID())
			}
		})
	}
}

func TestFroniusObservedFlavorDefinitionsCarryTheirOwnExactTuple(t *testing.T) {
	definitions := []struct {
		definition sunSpecFroniusFlavorDefinition
		id         string
		firmware   string
		chain      []SunSpecWireKey
	}{
		{froniusObservedFlavorV1, SunSpecFroniusObservedFlavorID, froniusTestFirmwareV1, []SunSpecWireKey{{1, 65}, {113, 60}, {120, 26}, {121, 30}, {122, 44}, {160, 88}, {124, 24}, {0xffff, 0}}},
		{froniusObservedFlavorV11, SunSpecFroniusObservedFlavorV11ID, froniusTestFirmwareV1, []SunSpecWireKey{{1, 65}, {113, 60}, {120, 26}, {121, 30}, {122, 44}, {123, 24}, {160, 88}, {124, 24}, {0xffff, 0}}},
		{froniusObservedFlavorV12, SunSpecFroniusObservedFlavorV12ID, froniusTestFirmwareV12, []SunSpecWireKey{{1, 65}, {113, 60}, {120, 26}, {121, 30}, {122, 44}, {123, 24}, {160, 88}, {124, 24}, {0xffff, 0}}},
	}
	for index, want := range definitions {
		got := want.definition
		if got.id != want.id || got.firmware != want.firmware || !slices.Equal(got.chain, want.chain) {
			t.Fatalf("definition %d=%+v want id=%q firmware=%q chain=%v", index, got, want.id, want.firmware, want.chain)
		}
		for _, other := range definitions[index+1:] {
			if got.id == other.definition.id || got.firmware == other.definition.firmware && slices.Equal(got.chain, other.definition.chain) {
				t.Fatalf("definitions %q and %q do not admit disjoint exact tuples", got.id, other.definition.id)
			}
		}
	}

	registry := mustStandardSunSpecRegistry(t)
	emptyVersion := froniusCommonValues(froniusTestManufacturer, froniusTestModel, "")
	emptyVersion["Vr"] = append([]uint16{0x0080}, make([]uint16, 7)...)
	snapshot := froniusSnapshotForChain(t, registry, froniusObservedChainV12, emptyVersion)
	unpinned := froniusObservedFlavorV12
	unpinned.firmware = ""
	if decision := registry.evaluateFroniusObservedFlavor(snapshot, unpinned); decision.Matched() || decision.Reason() != SunSpecFroniusFlavorReasonFirmwareMismatch {
		t.Fatalf("definition without firmware matched=%t reason=%q", decision.Matched(), decision.Reason())
	}

	card := NewSunSpecFroniusQualificationCard()
	if card.FlavorID() != froniusObservedFlavorV11.id || card.Firmware() != froniusObservedFlavorV11.firmware || !slices.Equal(card.ExpectedChain(), froniusObservedFlavorV11.chain) {
		t.Fatalf("card id=%q firmware=%q chain=%v drifted from its flavor definition", card.FlavorID(), card.Firmware(), card.ExpectedChain())
	}
}

func TestFroniusObservedFlavorV12FixtureIsExactAndNonActionable(t *testing.T) {
	fixture := readFroniusV11Fixture(t, "testdata/sunspec/chains/fronius_gen24_float_v1_2.json")
	if fixture.Schema != "helianthus-sunspec-fronius-observed-flavor/v1.2" || fixture.FlavorID != SunSpecFroniusObservedFlavorV12ID ||
		fixture.Manufacturer != froniusTestManufacturer || fixture.Model != froniusTestModel || fixture.Firmware != froniusTestFirmwareV12 ||
		!reflect.DeepEqual(fixture.Chain, froniusObservedChainV12) {
		t.Fatalf("fixture=%+v", fixture)
	}
	if fixture.Observation.Function != "FC03" || fixture.Observation.UnitID != 1 || fixture.Observation.PDUOffset != 40000 || fixture.Observation.Authority != "non_actionable_provenance_only" {
		t.Fatalf("observation=%+v", fixture.Observation)
	}

	registry := mustStandardSunSpecRegistry(t)
	snapshot := froniusSnapshotForChain(t, registry, fixture.Chain, froniusCommonValues(fixture.Manufacturer, fixture.Model, fixture.Firmware))
	selection := registry.SelectFroniusObservedFlavor(snapshot)
	decision, ok := selection.Decision()
	if !selection.Matched() || !ok || decision.FlavorID() != fixture.FlavorID || !reflect.DeepEqual(decision.Chain(), fixture.Chain) {
		t.Fatalf("classifier does not substantiate fixture: matched=%t reason=%q decision=%q", selection.Matched(), selection.Reason(), decision.FlavorID())
	}
}

func TestSunSpecQualificationObservationAdmitsAndSerializesV12(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	occurrences := froniusObservedSnapshotV12(t, registry, froniusTestManufacturer, froniusTestModel, froniusTestFirmwareV12).Occurrences()
	observation, err := NewSunSpecQualificationObservation(registry, qualificationSnapshotWithOccurrences(t, registry, occurrences))
	if err != nil {
		t.Fatalf("NewSunSpecQualificationObservation: %v", err)
	}
	if observation.Flavor().FlavorID() != SunSpecFroniusObservedFlavorV12ID || !reflect.DeepEqual(observation.Chain(), froniusObservedChainV12) {
		t.Fatalf("flavor=%q chain=%v", observation.Flavor().FlavorID(), observation.Chain())
	}
	encoded, err := json.Marshal(observation)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var serialized serializedQualificationObservation
	if err := json.Unmarshal(encoded, &serialized); err != nil {
		t.Fatal(err)
	}
	if serialized.FlavorID != SunSpecFroniusObservedFlavorV12ID || serialized.FlavorReason != SunSpecFroniusFlavorReasonMatched || !reflect.DeepEqual(serialized.Chain, froniusObservedChainV12) {
		t.Fatalf("serialized flavor=%q reason=%q chain=%v", serialized.FlavorID, serialized.FlavorReason, serialized.Chain)
	}

	unsupported := observation
	unsupported.flavor.flavorID = "sunspec.flavor.fronius.gen24.float.observed@1.3.0"
	if _, err := json.Marshal(unsupported); err == nil {
		t.Fatal("observation with an unregistered flavor id serialized")
	}

	for _, evaluation := range registry.SelectFroniusObservedFlavor(qualificationSnapshotWithOccurrences(t, registry, occurrences)).Evaluations() {
		if !supportedSunSpecQualificationFlavorID(evaluation.FlavorID()) {
			t.Fatalf("selector flavor %q is not a supported qualification flavor", evaluation.FlavorID())
		}
	}
	for _, id := range []string{"", "sunspec.flavor.fronius.gen24.float.observed@1.3.0", SunSpecFroniusObservedFlavorV12ID + " ", "sunspec.flavor.fronius.gen24.float.observed@1.2"} {
		if supportedSunSpecQualificationFlavorID(id) {
			t.Fatalf("unexpected supported qualification flavor %q", id)
		}
	}
}

func TestFroniusQualificationCardRemainsBoundToV11ForTheCurrentTuple(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	card := NewSunSpecFroniusQualificationCard()
	raw := rawWordsForOccurrences(froniusObservedSnapshotV12(t, registry, froniusTestManufacturer, froniusTestModel, froniusTestFirmwareV12).Occurrences())
	snapshot, err := replayFroniusQualificationCard(t, card, registry, raw)
	if err != nil || len(snapshot.RawWords()) == 0 {
		t.Fatalf("complete replay raw=%d err=%v", len(snapshot.RawWords()), err)
	}
	if _, err := card.Qualify(registry, snapshot); err == nil {
		t.Fatal("card bound to flavor 1.1.0 qualified the 1.2.0 tuple")
	}
	observation, err := NewSunSpecQualificationObservation(registry, snapshot)
	if err != nil || observation.Flavor().FlavorID() != SunSpecFroniusObservedFlavorV12ID {
		t.Fatalf("registry qualification observation err=%v flavor=%q", err, observation.Flavor().FlavorID())
	}
}

func froniusObservedSnapshotV12(t *testing.T, registry SunSpecDecoderRegistry, manufacturer, model, firmware string) SunSpecChainSnapshot {
	t.Helper()
	return froniusSnapshotForChain(t, registry, froniusObservedChainV12, froniusCommonValues(manufacturer, model, firmware))
}

func froniusCommonValues(manufacturer, model, firmware string) map[string][]uint16 {
	return map[string][]uint16{
		"Mn": stringWords(manufacturer, 16), "Md": stringWords(model, 16), "Vr": stringWords(firmware, 8),
		"SN": stringWords("sanitized", 16), "DA": {1},
	}
}

func froniusChainForFlavor(id string) []SunSpecWireKey {
	switch id {
	case SunSpecFroniusObservedFlavorID:
		return froniusObservedChainV1
	case SunSpecFroniusObservedFlavorV11ID:
		return froniusObservedChainV11
	default:
		return froniusObservedChainV12
	}
}

// froniusSnapshotForChain builds a synthetic snapshot whose ordered wire chain
// is exactly chain. The terminal marker must be the last key.
func froniusSnapshotForChain(t *testing.T, registry SunSpecDecoderRegistry, chain []SunSpecWireKey, common map[string][]uint16) SunSpecChainSnapshot {
	t.Helper()
	if len(chain) == 0 || chain[len(chain)-1] != (SunSpecWireKey{ModelID: sunSpecEndModel}) {
		t.Fatalf("chain %v does not end in the terminal marker", chain)
	}
	occurrences := make([]SunSpecOccurrence, 0, len(chain)-1)
	for index, key := range chain[:len(chain)-1] {
		ordinal := uint32(index + 1)
		switch key {
		case SunSpecWireKey{ModelID: 1, ModelLength: 65}:
			occurrences = append(occurrences, admittedOccurrence(1, 65, modelWords(t, registry, 1, 65, common), ordinal))
		case SunSpecWireKey{ModelID: 113, ModelLength: 60}:
			occurrences = append(occurrences, inverterOccurrence(t, registry, 113, nil, ordinal))
		case SunSpecWireKey{ModelID: 123, ModelLength: 24}:
			occurrences = append(occurrences, admittedOccurrence(123, 24, modelWords(t, registry, 123, 24, validModel123Values()), ordinal))
		case SunSpecWireKey{ModelID: 160, ModelLength: 88}:
			occurrences = append(occurrences, admittedOccurrence(160, 88, modelWords(t, registry, 160, 88, map[string][]uint16{"N": {4}}), ordinal))
		default:
			occurrences = append(occurrences, admittedOccurrence(key.ModelID, key.ModelLength, modelWords(t, registry, key.ModelID, key.ModelLength, nil), ordinal))
		}
	}
	return snapshotFromOccurrences(occurrences...)
}
