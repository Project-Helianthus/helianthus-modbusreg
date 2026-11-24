package sunspec

import "testing"

func TestMeterMonitoringPreservesSignScaleAndOccurrence(t *testing.T) {
	r := mustStandardSunSpecRegistry(t)
	snapshot := completedChainSnapshot(t, r,
		commonOccurrence(t, r, "Synthetic maker", "Meter family", "Fixture", 1),
		meterCapabilityFixture(t, r, 203, 2, map[string][]uint16{"W": {0xffe2}, "W_SF": {0xffff}, "VAR": {12}, "PF": {0xfff6}, "TotVArhExpQ3PhA": {0, 9}, "WphC": {0x8000}}),
		meterCapabilityFixture(t, r, 213, 3, map[string][]uint16{"W": float32Words(4), "VAR": float32Words(-6), "PF": float32Words(0.5), "TotWhExp": float32Words(0), "WphC": {0x7fc0, 0}, "TotVArhExpQ3phA": float32Words(8)}))
	decisions := evaluateFixtureCapabilities(t, snapshot)
	if len(decisions) != 2 {
		t.Fatalf("decisions=%+v", decisions)
	}
	for i, want := range []struct {
		power, reactive, pf, pfUnit, counter string
		scaled                               bool
	}{{"-3", "12", "-10", "Pct", "9", true}, {"4", "-6", "0.5", "PF", "8", false}} {
		d := decisions[i]
		source, ok := d.SourceOccurrence()
		if !ok || source.Ordinal != uint32(i+2) || !d.Admitted() || d.ProfileID() != "sunspec.meter.monitoring" {
			t.Fatalf("meter %d: %+v", i, d)
		}
		facts := d.Facts()
		w := requireCapabilityNumber(t, facts, "meter.ac.power.active.total", want.power, "W")
		if exponent, ok := w.ScaleFactor(); ok != want.scaled || ok && exponent != -1 || w.SignConvention() != "positive_toward_load" {
			t.Fatalf("active power scale/sign: %d %t %q", exponent, ok, w.SignConvention())
		}
		if got := requireCapabilityNumber(t, facts, "meter.ac.power.reactive.total", want.reactive, "var").SignConvention(); got != "positive_inductive" {
			t.Fatalf("reactive sign=%q", got)
		}
		if got := requireCapabilityNumber(t, facts, "meter.ac.power_factor.total", want.pf, want.pfUnit).SignConvention(); got != "positive_lagging" {
			t.Fatalf("power factor sign=%q", got)
		}
		if got := requireCapabilityNumber(t, facts, "meter.energy.reactive_export.q3.phase_a", want.counter, "varh").SignConvention(); got != "export_counter" {
			t.Fatalf("counter sign=%q", got)
		}
		if got := requireCapabilityNumber(t, facts, "meter.ac.power.apparent.total", "0", "VA").SignConvention(); got != "open" {
			t.Fatalf("apparent sign=%q", got)
		}
		if _, ok := capabilityFactByID(facts, "meter.ac.power.active.phase_c"); ok {
			t.Fatal("unimplemented phase power was emitted")
		}
	}
	requireCapabilityNumber(t, decisions[1].Facts(), "meter.energy.export.total", "0", "Wh")
}

func meterCapabilityFixture(t *testing.T, r DecoderRegistry, id uint16, ordinal uint32, overrides map[string][]uint16) Occurrence {
	t.Helper()
	length := uint16(105)
	values := map[string][]uint16{"TotWhExp": {0, 1}, "TotWhImp": {0, 2}}
	if id >= 211 {
		length = 124
		values["TotWhExp"], values["TotWhImp"] = float32Words(1), float32Words(2)
	}
	for name, words := range overrides {
		values[name] = words
	}
	return admittedOccurrence(id, length, modelWords(t, r, id, length, values), ordinal)
}
