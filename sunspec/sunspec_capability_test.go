package sunspec

import (
	"math"
	"reflect"
	"testing"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestThreePhaseMonitoringAdmitsExactlyOneCompleteSource(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	for _, modelID := range []uint16{103, 113} {
		t.Run(string(rune(modelID)), func(t *testing.T) {
			snapshot := capabilitySnapshot(t, registry, modelID, nil)
			decision := registry.EvaluateThreePhaseMonitoring(snapshot)
			if !decision.Admitted() || decision.Reason() != CapabilityReasonAdmitted || decision.ProfileID() != ThreePhaseMonitoringCapabilityID {
				t.Fatalf("decision admitted=%t reason=%q profile=%q", decision.Admitted(), decision.Reason(), decision.ProfileID())
			}
			facts := decision.Facts()
			if len(facts) != 14 {
				t.Fatalf("facts=%d", len(facts))
			}
			if source, ok := decision.SourceOccurrence(); !ok || source.ModelID() != modelID || source.ModelLength() != map[bool]uint16{true: 60, false: 50}[modelID == 113] {
				t.Fatalf("source=%#v present=%t", source, ok)
			}
			if len(decision.SourceViews()) != len(snapshot.SourceViews()) {
				t.Fatalf("source views=%d want=%d", len(decision.SourceViews()), len(snapshot.SourceViews()))
			}
		})
	}
}

func TestThreePhaseMonitoringFailsClosedOnSourceAmbiguity(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	common := commonOccurrence(t, registry, "Maker", "Model", "1.0", 1)
	integer := inverterOccurrence(t, registry, 103, nil, 2)
	floating := inverterOccurrence(t, registry, 113, nil, 3)

	for name, occurrences := range map[string][]Occurrence{
		"duplicate": {common, integer, inverterOccurrence(t, registry, 103, nil, 3)},
		"both":      {common, integer, floating},
	} {
		t.Run(name, func(t *testing.T) {
			decision := registry.EvaluateThreePhaseMonitoring(snapshotFromOccurrences(occurrences...))
			if decision.Admitted() || decision.Reason() != CapabilityReasonAmbiguousSource || len(decision.Facts()) != 0 {
				t.Fatalf("decision admitted=%t reason=%q facts=%d", decision.Admitted(), decision.Reason(), len(decision.Facts()))
			}
		})
	}
}

func TestThreePhaseMonitoringRejectsMissingSourceAndInvalidCommon(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	common := commonOccurrence(t, registry, "Maker", "Model", "1.0", 1)
	if decision := registry.EvaluateThreePhaseMonitoring(snapshotFromOccurrences(common)); decision.Admitted() || decision.Reason() != CapabilityReasonSourceAbsent {
		t.Fatalf("missing source admitted=%t reason=%q", decision.Admitted(), decision.Reason())
	}
	if decision := registry.EvaluateThreePhaseMonitoring(snapshotFromOccurrences(inverterOccurrence(t, registry, 113, nil, 1))); decision.Admitted() || decision.Reason() != CapabilityReasonInvalidChain {
		t.Fatalf("missing common admitted=%t reason=%q", decision.Admitted(), decision.Reason())
	}
}

func TestThreePhaseMonitoringDistinguishesUnsupportedExactSource(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	common := commonOccurrence(t, registry, "Maker", "Model", "1.0", 1)
	unsupported := inverterOccurrence(t, registry, 113, nil, 2)
	unsupported.Disposition = ChainDispositionUnknownModel
	unsupported.decoderKey = nil
	decision := registry.EvaluateThreePhaseMonitoring(snapshotFromOccurrences(common, unsupported))
	if decision.Admitted() || decision.Reason() != CapabilityReasonSourceUnsupported {
		t.Fatalf("unsupported admitted=%t reason=%q", decision.Admitted(), decision.Reason())
	}

	wrongLength := Occurrence{
		Ordinal: 2, WireKey: WireKey{ModelID: 103, ModelLength: 49},
		SchemaRevision: testSunSpecModelsRevision, Disposition: ChainDispositionUnsupportedLength,
		words: append([]uint16{103, 49}, make([]uint16, 49)...),
	}
	valid := inverterOccurrence(t, registry, 113, nil, 3)
	decision = registry.EvaluateThreePhaseMonitoring(snapshotFromOccurrences(common, wrongLength, valid))
	if !decision.Admitted() {
		t.Fatalf("wrong-length lookalike blocked exact source: %q", decision.Reason())
	}

	unsupported.Ordinal = 2
	valid.Ordinal = 3
	decision = registry.EvaluateThreePhaseMonitoring(snapshotFromOccurrences(common, unsupported, valid))
	if decision.Admitted() || decision.Reason() != CapabilityReasonAmbiguousSource {
		t.Fatalf("unsupported exact source was ignored: admitted=%t reason=%q", decision.Admitted(), decision.Reason())
	}
}

func TestCanonicalSunSpecNumbersUseClosedPlainDecimalGrammar(t *testing.T) {
	tests := []struct {
		decimal Decimal
		want    string
	}{
		{Decimal{Coefficient: 0, Exponent: -4}, "0"},
		{Decimal{Coefficient: 12, Exponent: -1}, "1.2"},
		{Decimal{Coefficient: 120, Exponent: -2}, "1.2"},
		{Decimal{Coefficient: -5, Exponent: 0}, "-5"},
		{Decimal{Coefficient: 1, Exponent: -3}, "0.001"},
		{Decimal{Coefficient: -10, Exponent: 2}, "-1000"},
	}
	for _, tc := range tests {
		if got := formatSunSpecDecimal(tc.decimal); got != tc.want {
			t.Fatalf("formatSunSpecDecimal(%+v)=%q want=%q", tc.decimal, got, tc.want)
		}
	}
	value, ok := canonicalSunSpecValue(Value{
		pointType: TypeFloat32,
		state:     ValueValid,
		float32:   math.Float32frombits(0x80000000),
		hasFloat:  true,
	})
	if number, numberOK := value.Number(); !ok || !numberOK || number != "0" {
		t.Fatalf("negative zero canonical=%q value_ok=%t number_ok=%t", number, ok, numberOK)
	}
}

func TestThreePhaseMonitoringRequiresVerifiedTerminalOffsetsAndSpans(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	valid := capabilitySnapshot(t, registry, 113, nil)
	missingTerminal := cloneSnapshotForTest(valid)
	missingTerminal.raw = missingTerminal.raw[:len(missingTerminal.raw)-2]
	badOffset := cloneSnapshotForTest(valid)
	badOffset.occurrences[1].HeaderOffset++
	badSpan := cloneSnapshotForTest(valid)
	badSpan.occurrences[1].spans[0].WordCount--
	rawContradiction := cloneSnapshotForTest(valid)
	rawContradiction.raw[2] = 2

	for name, snapshot := range map[string]ChainSnapshot{
		"missing terminal":  missingTerminal,
		"bad offset":        badOffset,
		"bad span":          badSpan,
		"raw contradiction": rawContradiction,
	} {
		t.Run(name, func(t *testing.T) {
			decision := registry.EvaluateThreePhaseMonitoring(snapshot)
			if decision.Admitted() || decision.Reason() != CapabilityReasonInvalidChain {
				t.Fatalf("admitted=%t reason=%q", decision.Admitted(), decision.Reason())
			}
		})
	}
}

func TestThreePhaseMonitoringRejectsInvalidRequiredFacts(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	tests := map[string]map[string][]uint16{
		"sentinel":           {"A": {0x7fc0, 0}},
		"nonfinite":          {"A": {0x7f80, 0}},
		"unknown enum":       {"St": {99}},
		"unknown event bits": {"Evt1": {0x0001, 0}},
	}
	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			decision := registry.EvaluateThreePhaseMonitoring(capabilitySnapshot(t, registry, 113, values))
			if decision.Admitted() || decision.Reason() != CapabilityReasonInvalidRequiredFact || len(decision.Facts()) != 0 {
				t.Fatalf("decision admitted=%t reason=%q facts=%d", decision.Admitted(), decision.Reason(), len(decision.Facts()))
			}
		})
	}
}

func TestThreePhaseMonitoringNormalizesEquivalent103And113Values(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	integer := registry.EvaluateThreePhaseMonitoring(capabilitySnapshot(t, registry, 103, map[string][]uint16{
		"A": {125}, "AphA": {125}, "AphB": {125}, "AphC": {125}, "A_SF": {0xfffe},
		"PhVphA": {2300}, "PhVphB": {2300}, "PhVphC": {2300}, "V_SF": {0xffff},
		"W": {125}, "W_SF": {0xfffe}, "Hz": {5000}, "Hz_SF": {0xfffe},
		"WH": {0, 125}, "WH_SF": {0xfffe}, "TmpCab": {250}, "Tmp_SF": {0xffff},
		"St": {4}, "Evt1": {0, 1}, "Evt2": {0, 0},
	}))
	floating := registry.EvaluateThreePhaseMonitoring(capabilitySnapshot(t, registry, 113, map[string][]uint16{
		"A": float32Words(1.25), "AphA": float32Words(1.25), "AphB": float32Words(1.25), "AphC": float32Words(1.25),
		"PhVphA": float32Words(230), "PhVphB": float32Words(230), "PhVphC": float32Words(230),
		"W": float32Words(1.25), "Hz": float32Words(50), "WH": float32Words(1.25), "TmpCab": float32Words(25),
		"St": {4}, "Evt1": {0, 1}, "Evt2": {0, 0},
	}))
	if !integer.Admitted() || !floating.Admitted() {
		t.Fatalf("integer=%q floating=%q", integer.Reason(), floating.Reason())
	}
	if !reflect.DeepEqual(capabilityFactShape(integer.Facts()), capabilityFactShape(floating.Facts())) {
		t.Fatalf("103=%v\n113=%v", capabilityFactShape(integer.Facts()), capabilityFactShape(floating.Facts()))
	}
	for _, field := range []string{"inverter.ac.current.total", "inverter.ac.voltage.phase_a", "inverter.ac.power.active", "inverter.ac.frequency", "inverter.ac.energy_lifetime", "inverter.temperature.cabinet"} {
		left, lok := capabilityFactByID(integer.Facts(), field)
		right, rok := capabilityFactByID(floating.Facts(), field)
		leftNumber, leftOK := left.Value().Number()
		rightNumber, rightOK := right.Value().Number()
		if !lok || !rok || !leftOK || !rightOK || leftNumber != rightNumber {
			t.Fatalf("%s integer=%q/%t floating=%q/%t", field, leftNumber, leftOK, rightNumber, rightOK)
		}
	}
	for _, field := range []string{"inverter.operating_state", "inverter.events.1", "inverter.events.2"} {
		fact, ok := capabilityFactByID(floating.Facts(), field)
		if !ok || fact.Unit() != "none" {
			t.Fatalf("%s unit=%q present=%t", field, fact.Unit(), ok)
		}
	}
}

func TestThreePhaseMonitoringDecisionIsDefensiveAndRetainsEvidence(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	snapshot := capabilitySnapshot(t, registry, 113, nil)
	decision := registry.EvaluateThreePhaseMonitoring(snapshot)
	facts := decision.Facts()
	facts[0] = CapabilityFact{}
	if decision.Facts()[0].FieldID() == "" {
		t.Fatal("facts mutated through accessor")
	}
	original := decision.Facts()[0].SourceValue()
	words := original.RawWords()
	words[0] = 0
	if decision.Facts()[0].SourceValue().RawWords()[0] == 0 {
		t.Fatal("original typed value mutated through accessor")
	}
	source, ok := decision.SourceOccurrence()
	if !ok {
		t.Fatal("source absent")
	}
	raw := source.Words()
	raw[0] = 0
	again, _ := decision.SourceOccurrence()
	if again.Words()[0] != 113 {
		t.Fatal("source raw evidence mutated")
	}
	views := decision.SourceViews()
	if len(views) == 0 {
		t.Fatal("source provenance absent")
	}
}

func capabilityFactShape(facts []CapabilityFact) []string {
	out := make([]string, len(facts))
	for index, fact := range facts {
		out[index] = fact.FieldID() + "|" + fact.Unit() + "|" + string(fact.Value().Kind())
	}
	return out
}

func capabilityFactByID(facts []CapabilityFact, fieldID string) (CapabilityFact, bool) {
	for _, fact := range facts {
		if fact.FieldID() == fieldID {
			return fact, true
		}
	}
	return CapabilityFact{}, false
}

func TestSunSpecCapabilityDecisionCloneSharesNoEvidence(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	decision := registry.EvaluateThreePhaseMonitoring(capabilitySnapshot(t, registry, 113, nil))
	if !decision.Admitted() {
		t.Fatalf("decision reason=%q", decision.Reason())
	}
	clone := decision.Clone()
	if !reflect.DeepEqual(clone, decision) {
		t.Fatal("clone differs from its original")
	}
	clone.source.words[0]++
	clone.facts[0].sourceValue.raw[0]++
	clone.facts[11].value.enumSymbol = "changed-clone"
	source, _ := decision.SourceOccurrence()
	if source.Words()[0] != 113 || decision.facts[0].sourceValue.raw[0] == clone.facts[0].sourceValue.raw[0] ||
		decision.facts[11].value.enumSymbol == "changed-clone" || sharesViewStorage(decision.views[0], clone.views[0]) {
		t.Fatal("clone shares evidence with its original")
	}
	rejected := registry.EvaluateThreePhaseMonitoring(ChainSnapshot{})
	if got := rejected.Clone(); got.Admitted() || got.Reason() != CapabilityReasonInvalidChain || got.source != nil || got.facts == nil || len(got.facts) != 0 || got.views == nil || len(got.views) != 0 {
		t.Fatalf("rejected clone=%#v", got)
	}
}

func TestSunSpecThreePhaseMonitoringFieldIDsFollowTheCapabilityOrder(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	decision := registry.EvaluateThreePhaseMonitoring(capabilitySnapshot(t, registry, 113, nil))
	want := make([]string, 0, len(decision.Facts()))
	for _, fact := range decision.Facts() {
		want = append(want, fact.FieldID())
	}
	got := ThreePhaseMonitoringFieldIDs()
	if len(want) != 14 || !reflect.DeepEqual(got, want) {
		t.Fatalf("field IDs=%v want %v", got, want)
	}
	got[0] = "changed-caller"
	if ThreePhaseMonitoringFieldIDs()[0] != want[0] {
		t.Fatal("field IDs share storage between calls")
	}
}

func TestSunSpecCloneSourceViewsSharesNoStorage(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	snapshot := completedChainSnapshot(t, registry,
		commonOccurrence(t, registry, "Maker", "Model", "1.0", 1),
		inverterOccurrence(t, registry, 113, nil, 2),
	)
	views := snapshot.SourceViews()
	clone := CloneSourceViews(views)
	if !reflect.DeepEqual(clone, views) {
		t.Fatal("cloned views differ")
	}
	if sharesViewStorage(clone[0], views[0]) {
		t.Fatal("cloned views share storage")
	}
	if got := CloneSourceViews(nil); got == nil || len(got) != 0 {
		t.Fatalf("nil views clone=%#v", got)
	}
	if got := CloneSourceViews([]modbusreg.LogicalViewSnapshot{{}, views[1]}); got[0].Valid() || !reflect.DeepEqual(got[0], modbusreg.LogicalViewSnapshot{}) || !got[1].Valid() {
		t.Fatalf("clone changed view validity: %#v", got)
	}
}
