package modbusreg

import (
	"math"
	"reflect"
	"testing"
)

func TestThreePhaseMonitoringAdmitsExactlyOneCompleteSource(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	for _, modelID := range []uint16{103, 113} {
		t.Run(string(rune(modelID)), func(t *testing.T) {
			snapshot := capabilitySnapshot(t, registry, modelID, nil)
			decision := registry.EvaluateThreePhaseMonitoring(snapshot)
			if !decision.Admitted() || decision.Reason() != SunSpecCapabilityReasonAdmitted || decision.ProfileID() != SunSpecThreePhaseMonitoringCapabilityID {
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

	for name, occurrences := range map[string][]SunSpecOccurrence{
		"duplicate": {common, integer, inverterOccurrence(t, registry, 103, nil, 3)},
		"both":      {common, integer, floating},
	} {
		t.Run(name, func(t *testing.T) {
			decision := registry.EvaluateThreePhaseMonitoring(snapshotFromOccurrences(occurrences...))
			if decision.Admitted() || decision.Reason() != SunSpecCapabilityReasonAmbiguousSource || len(decision.Facts()) != 0 {
				t.Fatalf("decision admitted=%t reason=%q facts=%d", decision.Admitted(), decision.Reason(), len(decision.Facts()))
			}
		})
	}
}

func TestThreePhaseMonitoringRejectsMissingSourceAndInvalidCommon(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	common := commonOccurrence(t, registry, "Maker", "Model", "1.0", 1)
	if decision := registry.EvaluateThreePhaseMonitoring(snapshotFromOccurrences(common)); decision.Admitted() || decision.Reason() != SunSpecCapabilityReasonSourceAbsent {
		t.Fatalf("missing source admitted=%t reason=%q", decision.Admitted(), decision.Reason())
	}
	if decision := registry.EvaluateThreePhaseMonitoring(snapshotFromOccurrences(inverterOccurrence(t, registry, 113, nil, 1))); decision.Admitted() || decision.Reason() != SunSpecCapabilityReasonInvalidChain {
		t.Fatalf("missing common admitted=%t reason=%q", decision.Admitted(), decision.Reason())
	}
}

func TestThreePhaseMonitoringDistinguishesUnsupportedExactSource(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	common := commonOccurrence(t, registry, "Maker", "Model", "1.0", 1)
	unsupported := inverterOccurrence(t, registry, 113, nil, 2)
	unsupported.Disposition = SunSpecChainDispositionUnknownModel
	unsupported.decoderKey = nil
	decision := registry.EvaluateThreePhaseMonitoring(snapshotFromOccurrences(common, unsupported))
	if decision.Admitted() || decision.Reason() != SunSpecCapabilityReasonSourceUnsupported {
		t.Fatalf("unsupported admitted=%t reason=%q", decision.Admitted(), decision.Reason())
	}

	wrongLength := SunSpecOccurrence{
		Ordinal: 2, WireKey: SunSpecWireKey{ModelID: 103, ModelLength: 49},
		SchemaRevision: testSunSpecModelsRevision, Disposition: SunSpecChainDispositionUnsupportedLength,
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
	if decision.Admitted() || decision.Reason() != SunSpecCapabilityReasonAmbiguousSource {
		t.Fatalf("unsupported exact source was ignored: admitted=%t reason=%q", decision.Admitted(), decision.Reason())
	}
}

func TestCanonicalSunSpecNumbersUseClosedPlainDecimalGrammar(t *testing.T) {
	tests := []struct {
		decimal SunSpecDecimal
		want    string
	}{
		{SunSpecDecimal{Coefficient: 0, Exponent: -4}, "0"},
		{SunSpecDecimal{Coefficient: 12, Exponent: -1}, "1.2"},
		{SunSpecDecimal{Coefficient: 120, Exponent: -2}, "1.2"},
		{SunSpecDecimal{Coefficient: -5, Exponent: 0}, "-5"},
		{SunSpecDecimal{Coefficient: 1, Exponent: -3}, "0.001"},
		{SunSpecDecimal{Coefficient: -10, Exponent: 2}, "-1000"},
	}
	for _, tc := range tests {
		if got := formatSunSpecDecimal(tc.decimal); got != tc.want {
			t.Fatalf("formatSunSpecDecimal(%+v)=%q want=%q", tc.decimal, got, tc.want)
		}
	}
	value, ok := canonicalSunSpecValue(SunSpecValue{
		pointType: SunSpecTypeFloat32,
		state:     SunSpecValueValid,
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

	for name, snapshot := range map[string]SunSpecChainSnapshot{
		"missing terminal":  missingTerminal,
		"bad offset":        badOffset,
		"bad span":          badSpan,
		"raw contradiction": rawContradiction,
	} {
		t.Run(name, func(t *testing.T) {
			decision := registry.EvaluateThreePhaseMonitoring(snapshot)
			if decision.Admitted() || decision.Reason() != SunSpecCapabilityReasonInvalidChain {
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
			if decision.Admitted() || decision.Reason() != SunSpecCapabilityReasonInvalidRequiredFact || len(decision.Facts()) != 0 {
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
	facts[0] = SunSpecCapabilityFact{}
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

func capabilityFactShape(facts []SunSpecCapabilityFact) []string {
	out := make([]string, len(facts))
	for index, fact := range facts {
		out[index] = fact.FieldID() + "|" + fact.Unit() + "|" + string(fact.Value().Kind())
	}
	return out
}

func capabilityFactByID(facts []SunSpecCapabilityFact, fieldID string) (SunSpecCapabilityFact, bool) {
	for _, fact := range facts {
		if fact.FieldID() == fieldID {
			return fact, true
		}
	}
	return SunSpecCapabilityFact{}, false
}
