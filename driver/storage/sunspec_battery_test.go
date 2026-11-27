package storage

import (
	"context"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
)

const batteryRevision = sunspec.ModelsRevisionV2

func batteryField(model uint16, name string) string {
	return "sunspec.der.v2." + itoa(model) + "." + name
}

func itoa(n uint16) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func packBlock713(t *testing.T, r sunspec.DecoderRegistry) []uint16 {
	return block(t, r, 713, 7, batteryRevision, map[string][]uint16{
		batteryField(713, "WHRtg"): {10}, batteryField(713, "WHRtg") + "#sf": {3}, batteryField(713, "WHAvail"): {5},
		batteryField(713, "SoC"): {600}, batteryField(713, "SoC") + "#sf": {0xffff}, batteryField(713, "SoH"): {950},
		batteryField(713, "Sta"): {1},
	})
}

func base802(t *testing.T, r sunspec.DecoderRegistry) []uint16 {
	return block(t, r, 802, 62, batteryRevision, map[string][]uint16{
		batteryField(802, "V"): {5120}, batteryField(802, "V") + "#sf": {0xffff},
		batteryField(802, "A"): {0xfff6}, batteryField(802, "A") + "#sf": {0xffff},
		batteryField(802, "W"): {0xff38}, batteryField(802, "W") + "#sf": {0},
		batteryField(802, "NCyc"): {0, 47},
		batteryField(802, "SoC"):  {777}, batteryField(802, "SoC") + "#sf": {0xffff},
	})
}

// bank803 is a bank of two strings: the fixed part is 28 words and each string
// group 32 words, with StrSoC at offset 4 of the group.
func bank803(strSoC ...uint16) []uint16 {
	n := len(strSoC)
	words := make([]uint16, 28+32*n)
	words[0], words[1], words[2] = 803, uint16(26+32*n), uint16(n)
	words[25] = 0 // SoC_SF
	for i, soc := range strSoC {
		words[28+32*i+4] = soc
	}
	return words
}

// string804 is a standalone string with its repeated modules: the fixed part is
// 48 words and each module group 16 words, with ModSoC at offset 1 of the group.
func string804(modSoC ...uint16) []uint16 {
	n := len(modSoC)
	words := make([]uint16, 48+16*n)
	words[0], words[1], words[3] = 804, uint16(46+16*n), uint16(n)
	words[2] = 1 // Idx
	words[38] = 0
	for i, soc := range modSoC {
		words[48+16*i+1] = soc
	}
	return words
}

func module805(t *testing.T, r sunspec.DecoderRegistry, str, mod, soc uint16) []uint16 {
	return block(t, r, 805, 42, batteryRevision, map[string][]uint16{
		batteryField(805, "StrIdx"): {str}, batteryField(805, "ModIdx"): {mod}, batteryField(805, "SoC"): {soc}, batteryField(805, "SoC") + "#sf": {0},
	})
}

func batteryChain(t *testing.T, build func(r sunspec.DecoderRegistry) [][]uint16) sunspec.ChainSnapshot {
	t.Helper()
	r := sunRegistry(t, batteryRevision)
	blocks := append([][]uint16{commonBlock(t, r, batteryRevision)}, build(r)...)
	return chain(t, batteryRevision, r.DecoderKeys(), blocks...)
}

func memberValue(t *testing.T, p *Publisher, local string) Native {
	t.Helper()
	for _, m := range p.Members() {
		if string(m.Key.Local) == local {
			return m.Native
		}
	}
	t.Fatalf("namespace has no %s", local)
	return Native{}
}

func TestSunSpecBatteryModelsPackFactsAndNativeOccurrences(t *testing.T) {
	snap := batteryChain(t, func(r sunspec.DecoderRegistry) [][]uint16 {
		return [][]uint16{packBlock713(t, r), base802(t, r), bank803(70, 80), string804(55, 66), module805(t, r, 1, 1, 41), module805(t, r, 1, 2, 42)}
	})
	s := newSession("asset:sunspec-b", "source-b", "epoch-1", 1)
	p := newPublisher(t, sunConfig(), s, nil)
	if err := p.PublishSunSpec(context.Background(), SunSpecObservation{Decisions: decisions(t, snap), Receipt: receipt(1)}); err != nil {
		t.Fatal(err)
	}
	k := s.k
	// Model 713: the pack's capacity, available energy, state of charge and health.
	requireQuantity(t, k, "storage.energy.nominal_capacity", "36", 6, unitJoule)
	requireQuantity(t, k, "storage.energy.remaining", "18", 6, unitJoule)
	requireQuantity(t, k, "storage.state.soc", "6", -1, unitRatio)
	requireQuantity(t, k, "storage.state.soh", "95", -2, unitRatio)
	// Model 802: the pack's electrical observations; its own state of charge is not the pack's.
	requireQuantity(t, k, "storage.pack.voltage", "512", 0, unitVolt)
	requireQuantity(t, k, "storage.pack.current", "-1", 0, unitAmpere)
	requireQuantity(t, k, "storage.pack.power", "-2", 2, unitWatt)
	requireQuantity(t, k, "storage.state.cycle_count", "47", 0, unitOne)
	if len(factsOf(k, "storage.state.soc")) != 1 {
		t.Fatal("a member or a second pack observation took the pack state of charge")
	}
	// The strings, modules and independent module occurrences stay native, each under its own occurrence and group.
	prefix803, prefix804 := "model_803.occurrence_4.", "model_804.occurrence_5."
	one := memberValue(t, p, prefix803+"group_1.battery.string.state_of_charge")
	two := memberValue(t, p, prefix803+"group_2.battery.string.state_of_charge")
	if whole(one) != "70" || whole(two) != "80" {
		t.Errorf("string members: %+v %+v", one, two)
	}
	firstModule := memberValue(t, p, prefix804+"group_1.battery.module.state_of_charge")
	secondModule := memberValue(t, p, prefix804+"group_2.battery.module.state_of_charge")
	if whole(firstModule) != "55" || whole(secondModule) != "66" {
		t.Errorf("module members: %+v %+v", firstModule, secondModule)
	}
	a, b := memberValue(t, p, "model_805.occurrence_6.battery.module.state_of_charge"), memberValue(t, p, "model_805.occurrence_7.battery.module.state_of_charge")
	if whole(a) != "41" || whole(b) != "42" {
		t.Errorf("805 occurrences: %+v %+v", a, b)
	}
	if memberValue(t, p, prefix803+"group_1.raw_span").Text == "" {
		t.Error("the string group lost its raw span")
	}
	// No relation is drawn from an index or a list position: only the configured structure is.
	for _, def := range []string{"storage.relation.pack_string", "storage.relation.pack_module", "storage.relation.module_cell"} {
		for _, r := range k.relations {
			if string(r.Definition.ID) == def {
				t.Errorf("%s drawn from a reported index", def)
			}
		}
	}
	for _, c := range k.facts {
		if c.Key.Dimensions[0].ID == dimCell {
			t.Errorf("%s published for a cell the pages do not identify", c.Key.FactID)
		}
	}
}

func whole(n Native) string { return decimalRat(n.Value).FloatString(0) }
