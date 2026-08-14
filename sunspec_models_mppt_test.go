package modbusreg

import (
	"reflect"
	"testing"
)

func TestSunSpecMPPTExactKeyGeometryIsFiniteAndLazy(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	var keys []SunSpecDecoderKey
	for _, key := range registry.DecoderKeys() {
		if key.ModelID == 160 {
			keys = append(keys, key)
		}
	}
	if len(keys) != 3277 || keys[0].ModelLength != 8 || keys[len(keys)-1].ModelLength != 65528 {
		t.Fatalf("MPPT key count=%d first=%#v last=%#v", len(keys), keys[0], keys[len(keys)-1])
	}
	for n, length := range map[uint16]uint16{0: 8, 1: 28, 4: 88, 3276: 65528} {
		definition, ok := registry.definition(SunSpecDecoderKey{160, length, testSunSpecModelsRevision})
		if !ok || len(definition.points) != 9+10*int(n) {
			t.Fatalf("N=%d L=%d points=%d ok=%v", n, length, len(definition.points), ok)
		}
	}
	if _, ok := registry.definition(SunSpecDecoderKey{160, 87, testSunSpecModelsRevision}); ok {
		t.Fatal("non-geometric MPPT length resolved")
	}
}

func TestSunSpecMPPTDecodesIndexedModulesWithSharedScale(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.definition(SunSpecDecoderKey{160, 88, testSunSpecModelsRevision})
	if !ok {
		t.Fatal("model 160/88 absent")
	}
	words := make([]uint16, 90)
	words[0], words[1] = 160, 88
	for _, point := range definition.points {
		switch {
		case point.name == "DCA_SF":
			words[point.offset] = 0xfffe
		case point.name == "N":
			words[point.offset] = 4
		case point.repeated && point.name == "ID":
			words[point.offset] = point.repeatIndex
		case point.repeated && point.name == "DCA":
			words[point.offset] = 100 + point.repeatIndex
		case point.repeated && point.name == "DCWH":
			words[point.offset+1] = uint16(point.repeatIndex)
		}
	}
	decoded, err := registry.DecodeOccurrence(admittedOccurrence(160, 88, words, 3))
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.GeometryValid() || !decoded.Qualifies() {
		t.Fatalf("geometry=%v qualifies=%v", decoded.GeometryValid(), decoded.Qualifies())
	}
	var currents []SunSpecFact
	for _, fact := range decoded.Facts() {
		if fact.FieldID == "mppt.module.dc_current" {
			currents = append(currents, fact)
		}
	}
	if len(currents) != 4 {
		t.Fatalf("module currents=%d", len(currents))
	}
	for index, fact := range currents {
		if !fact.Repeated || fact.GroupID != "module" || fact.RepeatIndex != uint16(index+1) {
			t.Fatalf("fact[%d]=%#v", index, fact)
		}
		decimal, ok := fact.Value.Decimal()
		if !ok || decimal != (SunSpecDecimal{Coefficient: int64(101 + index), Exponent: -2}) {
			t.Fatalf("fact[%d] decimal=%#v ok=%v", index, decimal, ok)
		}
	}
	facts := decoded.Facts()
	facts[0].GroupID = "mutated"
	if reflect.DeepEqual(facts, decoded.Facts()) {
		t.Fatal("repeated fact metadata was mutable")
	}
}

func TestSunSpecMPPTCountMustMatchExactLength(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(testSunSpecModelsRevision)
	if err != nil {
		t.Fatal(err)
	}
	for name, count := range map[string]uint16{"mismatch": 3, "sentinel": 0xffff} {
		t.Run(name, func(t *testing.T) {
			words := modelWords(t, registry, 160, 88, map[string][]uint16{"N": {count}})
			decoded, err := registry.DecodeOccurrence(admittedOccurrence(160, 88, words, 1))
			if err != nil {
				t.Fatal(err)
			}
			if decoded.GeometryValid() || decoded.Qualifies() || !reflect.DeepEqual(decoded.RawWords(), words) {
				t.Fatalf("geometry=%v qualifies=%v", decoded.GeometryValid(), decoded.Qualifies())
			}
		})
	}
}
