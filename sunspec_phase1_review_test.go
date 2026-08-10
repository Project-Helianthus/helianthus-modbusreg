package modbusreg_test

import (
	"reflect"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestSunSpecPhaseOneActivationBindsRawChainAndObservation(t *testing.T) {
	decoder, profile := reviewDecoder(t)
	raw := reviewRawChain(101)
	chain, err := decoder.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	observation := sunSpecObservation(t, profile)
	activated, err := decoder.Activate(reg.SunSpecPhaseOneActivation{
		Chain:       chain,
		RawWords:    raw,
		Observation: observation,
	})
	if err != nil {
		t.Fatalf("Activate(valid): %v", err)
	}
	if !reflect.DeepEqual(activated.RawWords(), raw) ||
		!reflect.DeepEqual(activated.Spec(), observation) {
		t.Fatal("activation did not preserve exact raw chain and observation facts")
	}

	mutations := []struct {
		name   string
		mutate func(*reg.SunSpecPhaseOneActivation)
	}{
		{"unrelated raw words", func(a *reg.SunSpecPhaseOneActivation) { a.RawWords = reviewRawChain(102) }},
		{"endpoint", func(a *reg.SunSpecPhaseOneActivation) { a.Observation.Endpoint = "fixture:other" }},
		{"unit", func(a *reg.SunSpecPhaseOneActivation) { a.Observation.UnitID++ }},
		{"poll generation", func(a *reg.SunSpecPhaseOneActivation) { a.Observation.PollGenerationID++ }},
		{"dependency identity", func(a *reg.SunSpecPhaseOneActivation) { a.Observation.Dependencies[0].DependencyID = "other" }},
		{"wire identity", func(a *reg.SunSpecPhaseOneActivation) {
			record := a.Observation.Dependencies[0].View.Record()
			record.WireResponseID++
			a.Observation.Dependencies[0].View = snapshotFromRecord(t, record)
		}},
		{"logical identity", func(a *reg.SunSpecPhaseOneActivation) {
			record := a.Observation.Dependencies[0].View.Record()
			record.LogicalViewID++
			a.Observation.Dependencies[0].View = snapshotFromRecord(t, record)
		}},
		{"torn generation", func(a *reg.SunSpecPhaseOneActivation) {
			record := a.Observation.Dependencies[0].View.Record()
			record.TransportGeneration++
			a.Observation.Dependencies[0].View = snapshotFromRecord(t, record)
		}},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			activation := reg.SunSpecPhaseOneActivation{Chain: chain, RawWords: raw, Observation: observation}
			test.mutate(&activation)
			if _, err := decoder.Activate(activation); err == nil {
				t.Fatal("Activate accepted a mismatched chain/observation binding")
			}
		})
	}
}

func TestSunSpecPhaseOneParsesFixtureSemanticsFromRawModels(t *testing.T) {
	decoder, _ := reviewDecoder(t)
	// Values are the public M3-01 synthetic FSS-P-002 and FSS-P-003 examples.
	chain, err := decoder.Parse(reviewRawChain(101, 102, 103))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	common, ok := chain.Common()
	if !ok || common.Manufacturer() != "FixtureCo" || common.Model() != "Fixture-1" ||
		common.SerialNumber() != "PLACEHOLDER" || common.Version() != "VERSION123456789" {
		t.Fatalf("Common model 1 = %#v, present=%t", common, ok)
	}
	for _, id := range []uint16{101, 102, 103} {
		inverter, ok := chain.Inverter(id)
		if !ok {
			t.Fatalf("model %d was not semantically exposed", id)
		}
		if power := inverter.Power(); power.Raw() != -123 || power.ScaleFactor() != -1 || power.Value() != -12.3 {
			t.Fatalf("model %d power = %#v", id, power)
		}
		if energy := inverter.Energy(); energy.Raw() != 120000 || energy.ScaleFactor() != 0 || energy.Value() != 120000 {
			t.Fatalf("model %d energy = %#v", id, energy)
		}
	}
}

func TestSunSpecPhaseOneUsesExactNormalizationLengthsAndDeferredSet(t *testing.T) {
	decoder, profile := reviewDecoder(t)
	dependency := profile.Dependencies().Dependencies()[0]
	normalization := dependency.Normalization().Spec()
	if normalization.DocumentaryAddress != 40001 || normalization.DocumentaryBase != reg.AddressBaseOneBased ||
		normalization.Transformation != reg.TransformSubtractOne || normalization.ResolvedPDUOffset != 40000 {
		t.Fatalf("SunSpec normalization = %#v", normalization)
	}
	if dependency.Normalization().ResolvedPDUOffset() != 40000 {
		t.Fatal("runtime normalization differs from documentary record")
	}
	for _, test := range []struct{ id, length uint16 }{
		{1, 64}, {1, 66}, {101, 49}, {101, 51}, {102, 49}, {102, 51}, {103, 49}, {103, 51},
	} {
		if _, err := decoder.Parse(reviewRawChainWithLength(test.id, test.length)); err == nil {
			t.Fatalf("model %d length %d was accepted", test.id, test.length)
		}
	}
	for _, id := range []uint16{200, 219, 777} {
		if _, err := decoder.Parse(reviewRawChainWithLength(id, 3)); err == nil {
			t.Fatalf("deferred model %d was accepted", id)
		}
	}
	chain, err := decoder.Parse(reviewRawChainWithLength(220, 3))
	if err != nil || !reflect.DeepEqual(modelIDs(chain.SkippedModels()), []uint16{220}) {
		t.Fatalf("model 220 was not structurally skipped: %#v, %v", chain, err)
	}
}

func reviewDecoder(t *testing.T) (reg.SunSpecPhaseOneDecoder, reg.ProfileDescriptor) {
	t.Helper()
	profile, err := reg.NewSunSpecPhaseOneProfile(reg.SunSpecPhaseOneVersions{Profile: version(t, "1.0.0"), Codec: version(t, "1.0.0")})
	if err != nil {
		t.Fatalf("NewSunSpecPhaseOneProfile: %v", err)
	}
	decoder, err := reg.NewSunSpecPhaseOneDecoder(profile)
	if err != nil {
		t.Fatalf("NewSunSpecPhaseOneDecoder: %v", err)
	}
	return decoder, profile
}

func reviewRawChain(ids ...uint16) []uint16 {
	words := []uint16{0x5375, 0x6e53}
	words = append(words, 1, 65)
	common := make([]uint16, 65)
	copy(common[0:], reviewStringWords("FixtureCo", 16))
	copy(common[16:], reviewStringWords("Fixture-1", 16))
	copy(common[32:], reviewStringWords("PLACEHOLDER", 16))
	copy(common[48:], reviewStringWords("VERSION123456789", 8))
	words = append(words, common...)
	for _, id := range ids {
		words = append(words, id, 50)
		payload := make([]uint16, 50)
		payload[8], payload[9] = 0xff85, 0xffff
		payload[16], payload[17], payload[18] = 1, 0xd4c0, 0
		words = append(words, payload...)
	}
	return append(words, 0xffff, 0)
}

func reviewRawChainWithLength(id, length uint16) []uint16 {
	return append(append([]uint16{0x5375, 0x6e53, id, length}, make([]uint16, length)...), 0xffff, 0)
}

func reviewStringWords(value string, width int) []uint16 {
	bytes := make([]byte, width*2)
	copy(bytes, value)
	words := make([]uint16, width)
	for index := range words {
		words[index] = uint16(bytes[index*2])<<8 | uint16(bytes[index*2+1])
	}
	return words
}
