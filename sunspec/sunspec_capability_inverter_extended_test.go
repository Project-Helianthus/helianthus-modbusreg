package sunspec

import (
	"reflect"
	"testing"
)

func TestThreePhaseMonitoringRetainsValidZeroPowerFactor(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	snapshot := capabilitySnapshot(t, registry, 113, map[string][]uint16{"PF": float32Words(0)})
	decision := registry.EvaluateThreePhaseMonitoring(snapshot)
	if !decision.Admitted() || decision.ProfileID() != ThreePhaseMonitoringCapabilityID {
		t.Fatalf("monitoring decision: %s (%s)", decision.Reason(), decision.ProfileID())
	}
	requireCapabilityNumber(t, decision.Facts(), "inverter.ac.power_factor", "0", "Pct")
}

func TestThreePhaseMonitoringAppendsExtendedInverterFactsInOrder(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	snapshot := capabilitySnapshot(t, registry, 113, map[string][]uint16{
		"PF": float32Words(91), "DCA": float32Words(6), "DCV": float32Words(300), "DCW": float32Words(1800),
		"TmpSnk": float32Words(32), "TmpTrns": float32Words(35), "TmpOt": float32Words(-5),
		"StVnd": {11}, "EvtVnd1": {0, 1}, "EvtVnd2": {0, 2}, "EvtVnd3": {0, 4}, "EvtVnd4": {0, 8},
	})
	old := registry.EvaluateThreePhaseMonitoring(snapshot)
	segment := evaluateFixtureCapabilities(t, snapshot)
	if !old.Admitted() || old.ProfileID() != "sunspec.inverter.three_phase.monitoring@1.0.0" || len(ThreePhaseMonitoringFieldIDs()) != 14 || len(segment) != 1 || !segment[0].Admitted() || !reflect.DeepEqual(segment[0].Facts(), old.Facts()) {
		t.Fatalf("legacy=%s segment=%+v", old.Reason(), segment)
	}
	facts := old.Facts()
	if len(facts) != 31 {
		t.Fatalf("facts=%d", len(facts))
	}
	facts = facts[19:]
	for i, want := range []struct{ id, value, unit string }{
		{"inverter.ac.power_factor", "91", "Pct"}, {"inverter.dc.current", "6", "A"}, {"inverter.dc.voltage", "300", "V"}, {"inverter.dc.power", "1800", "W"},
		{"inverter.temperature.heatsink", "32", "C"}, {"inverter.temperature.transformer", "35", "C"}, {"inverter.temperature.other", "-5", "C"},
	} {
		if facts[i].FieldID() != want.id {
			t.Fatalf("optional fact %d=%s", i, facts[i].FieldID())
		}
		requireCapabilityNumber(t, facts, want.id, want.value, want.unit)
	}
	state := facts[7]
	n, symbol, ok := state.SourceValue().Enum()
	if state.FieldID() != "inverter.vendor_state" || !ok || n != 11 || symbol != "" || state.Value().Kind() != "" {
		t.Fatalf("vendor state=%+v", state)
	}
	for i, id := range []string{"inverter.vendor_events.1", "inverter.vendor_events.2", "inverter.vendor_events.3", "inverter.vendor_events.4"} {
		f := facts[8+i]
		bits, unknown, ok := f.SourceValue().Bitfield()
		if f.FieldID() != id || !ok || bits != 1<<i || unknown != bits || f.Value().Kind() != "" {
			t.Fatalf("vendor event %d=%+v", i, f)
		}
	}
}

func TestThreePhaseMonitoringOmitsExtendedInverterPointsWithAbsentScale(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	snapshot := legacyOptionalCapabilitySnapshot(t, registry, 103, map[string][]uint16{
		"PF": {42}, "PF_SF": {0x8000}, "DCA": {42}, "DCA_SF": {0xffff},
	})
	decision := registry.EvaluateThreePhaseMonitoring(snapshot)
	if !decision.Admitted() {
		t.Fatalf("monitoring decision: %s", decision.Reason())
	}
	if _, ok := capabilityFactByID(decision.Facts(), "inverter.ac.power_factor"); ok {
		t.Fatal("unimplemented PF scale emitted a fact")
	}
	f := requireCapabilityNumber(t, decision.Facts(), "inverter.dc.current", "4.2", "A")
	if exponent, ok := f.ScaleFactor(); !ok || exponent != -1 {
		t.Fatalf("DCA scale=%d %t", exponent, ok)
	}
}
