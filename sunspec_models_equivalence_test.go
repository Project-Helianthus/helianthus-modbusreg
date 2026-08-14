package modbusreg

import (
	"reflect"
	"sort"
	"testing"
)

func TestSunSpecModels103And113CanonicalEquivalence(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	integer := modelWords(t, registry, 103, 50, map[string][]uint16{
		"A": {125}, "AphA": {40}, "AphB": {41}, "AphC": {44}, "A_SF": {0xfffe},
		"W": {0x04d2}, "W_SF": {0}, "Hz": {5000}, "Hz_SF": {0xfffe},
		"WH": {0, 100}, "WH_SF": {0}, "St": {4},
	})
	floating := modelWords(t, registry, 113, 60, map[string][]uint16{
		"A": {0x3fa0, 0}, "AphA": {0x3e4c, 0xcccd}, "AphB": {0x3e51, 0xeb85}, "AphC": {0x3e61, 0x47ae},
		"W": {0x449a, 0x4000}, "Hz": {0x4248, 0}, "WH": {0x42c8, 0}, "St": {4},
	})
	left, err := registry.DecodeOccurrence(admittedOccurrence(103, 50, integer, 1))
	if err != nil {
		t.Fatal(err)
	}
	right, err := registry.DecodeOccurrence(admittedOccurrence(113, 60, floating, 1))
	if err != nil {
		t.Fatal(err)
	}
	if left.Topology() != SunSpecTopologyThreePhase || right.Topology() != SunSpecTopologyThreePhase {
		t.Fatal("three-phase topology lost")
	}
	leftShape, rightShape := requiredFactShape(left), requiredFactShape(right)
	if !reflect.DeepEqual(leftShape, rightShape) {
		t.Fatalf("103=%v\n113=%v", leftShape, rightShape)
	}
	if reflect.DeepEqual(left.RawWords(), right.RawWords()) {
		t.Fatal("distinct encodings collapsed")
	}
}

func requiredFactShape(model SunSpecDecodedModel) []string {
	var shape []string
	for _, fact := range model.Facts() {
		if fact.Required && fact.Value.State() == SunSpecValueValid {
			shape = append(shape, fact.FieldID+"|"+fact.Unit)
		}
	}
	sort.Strings(shape)
	return shape
}
