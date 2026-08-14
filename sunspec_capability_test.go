package modbusreg

import (
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
