package pv

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"sort"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
)

type recordedFact struct {
	Element, Dimension, Unit, Coefficient, Token string
	Exponent                                     int32
}

func recordedFacts(t *testing.T, name string) []recordedFact {
	t.Helper()
	raw, err := os.ReadFile("testdata/recorded_facts.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct{ Recorded map[string][]recordedFact }
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Recorded[name]) == 0 {
		t.Fatalf("no recorded facts %q", name)
	}
	return f.Recorded[name]
}

// recordedChain is the observed three-phase float chain the earlier publisher
// recorded: a Common model, model 113 with its state at MPPT, the nameplate,
// settings and status models, four MPPT modules and the storage controls.
func recordedChain(t *testing.T, power float32) []uint16 {
	t.Helper()
	inverter := zeros(113, 60)
	inverter[48] = 4
	if power != 0 {
		p, _, ok := registry(t).FixedPointLayout(sunspec.DecoderKey{ModelID: 113, ModelLength: 60, SchemaRevision: sunspec.ModelsRevisionV1}, "inverter.ac.power.active")
		if !ok {
			t.Fatal("no power point")
		}
		copy(inverter[p.WordOffset:], floatWords(power))
	}
	mppt := zeros(160, 88)
	mppt[8] = 4
	common := commonBlock("Fronius", "Symo GEN24 10.0", "synthetic")
	copy(common[42:50], stringWords("1.41.11-1", 8))
	var words []uint16
	for _, b := range [][]uint16{common, inverter, zeros(120, 26), zeros(121, 30), zeros(122, 44), mppt, zeros(124, 24)} {
		words = append(words, b...)
	}
	return words
}

func splitBlocks(t *testing.T, words []uint16) [][]uint16 {
	t.Helper()
	var out [][]uint16
	for len(words) > 0 {
		n := int(words[1]) + 2
		out = append(out, words[:n])
		words = words[n:]
	}
	return out
}

// repaired is what a recorded fact becomes after the stated unit repairs: a
// Celsius temperature in kelvin and a kilowatt-hour counter in joule.
func repaired(t *testing.T, r recordedFact) (unit string, value *big.Rat) {
	t.Helper()
	if r.Token != "" {
		return "", nil
	}
	n, ok := new(big.Rat).SetString(r.Coefficient)
	if !ok {
		t.Fatal("bad coefficient")
	}
	n = scaled(n, int64(r.Exponent))
	switch r.Unit {
	case "unit.celsius":
		return "unit.kelvin", celsiusToKelvin(n)
	case "unit.kilowatt_hour":
		return "unit.joule", kwhToJoule.apply(n)
	}
	return r.Unit, n
}

func describe(t *testing.T, f Fact) string {
	t.Helper()
	k := f.Candidate.Key
	v := f.Candidate.Value
	if v.Symbol != nil {
		return fmt.Sprint(k.FactID, "|", k.Dimensions[0].ID, "|", v.Symbol.Token)
	}
	n, err := ratOf(v.Quantity.Number)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprint(k.FactID, "|", k.Dimensions[0].ID, "|", v.Quantity.Unit, "|", n.RatString())
}

// The facts the earlier publisher recorded for the observed chain are all
// published again, with the stated unit repairs; every other fact is one the
// new rows add, listed below with its count.
func TestTheRecordedFactsAreTranscribed(t *testing.T) {
	added := map[string]int{
		"pv.ac.aggregate_current": 1, "pv.ac.power_factor": 1, "pv.ac.aggregate_apparent_power": 1, "pv.ac.aggregate_reactive_power": 1,
		"infrastructure.ac.voltage_line_line": 3, "pv.dc.aggregate_current": 1, "pv.dc.aggregate_voltage": 1, "pv.dc.aggregate_power": 1,
		"pv.status.derating": 1, "pv.status.fault": 1, "pv.dc.current": 4, "pv.dc.voltage": 4, "pv.dc.power": 4,
	}
	for _, tc := range []struct {
		name  string
		power float32
	}{{"first_publication", 0}, {"refresh", 4321.5}, {"replacement", 0}} {
		pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, splitBlocks(t, recordedChain(t, tc.power))...)}}})
		have := map[string]int{}
		for _, f := range pub.Facts {
			have[describe(t, f)]++
		}
		for _, r := range recordedFacts(t, tc.name) {
			var key string
			if r.Token != "" {
				key = fmt.Sprint(r.Element, "|", r.Dimension, "|", r.Token)
			} else {
				unit, value := repaired(t, r)
				key = fmt.Sprint(r.Element, "|", r.Dimension, "|", unit, "|", value.RatString())
			}
			if have[key] == 0 {
				t.Fatalf("%s: recorded fact %s is not published again; have %v", tc.name, key, have)
			}
			have[key]--
		}
		left := map[string]int{}
		for key, n := range have {
			for ; n > 0; n-- {
				left[key[:indexOfBar(key)]]++
			}
		}
		if fmt.Sprint(sortedCounts(left)) != fmt.Sprint(sortedCounts(added)) {
			t.Fatalf("%s: added facts %v, want %v", tc.name, sortedCounts(left), sortedCounts(added))
		}
	}
}

func indexOfBar(s string) int {
	for i := range s {
		if s[i] == '|' {
			return i
		}
	}
	return len(s)
}

func sortedCounts(m map[string]int) []string {
	var out []string
	for k, v := range m {
		out = append(out, fmt.Sprint(k, "=", v))
	}
	sort.Strings(out)
	return out
}
