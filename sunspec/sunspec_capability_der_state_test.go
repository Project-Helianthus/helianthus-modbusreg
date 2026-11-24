package sunspec

import "testing"

func TestDERObservedStateEmitsRatingsSettingsStatusAndControls(t *testing.T) {
	r := mustStandardSunSpecRegistry(t)
	snapshot := snapshotFromOccurrences(commonOccurrence(t, r, "Synthetic maker", "DER family", "Fixture", 1),
		admittedOccurrence(120, 26, modelWords(t, r, 120, 26, map[string][]uint16{"DERTyp": {4}, "WRtg": {250}, "WRtg_SF": {1}}), 2),
		admittedOccurrence(121, 30, modelWords(t, r, 121, 30, map[string][]uint16{"VRef": {230}, "VMax": {250}, "VMinMax_SF": {0x8000}}), 3),
		admittedOccurrence(122, 44, modelWords(t, r, 122, 44, map[string][]uint16{"StActCtl": {0, 1}, "WAval": {32}, "TmSrc": stringWords("fixture", 4)}), 4),
		admittedOccurrence(123, 24, modelWords(t, r, 123, 24, map[string][]uint16{"Conn": {1}, "WMaxLimPct": {500}, "WMaxLimPct_SF": {0xffff}, "WMaxLim_Ena": {1}, "OutPFSet": {98}, "OutPFSet_SF": {0xfffe}, "VArPct_Ena": {1}, "VArWMaxPct": {3}}), 5))
	var facts []CapabilityFact
	var seen int
	for _, d := range evaluateFixtureCapabilities(t, snapshot) {
		if d.ProfileID() == "sunspec.der.state" {
			if !d.Admitted() {
				t.Fatalf("state rejected: %s", d.Reason())
			}
			seen++
			facts = append(facts, d.Facts()...)
		}
	}
	if seen != 4 {
		t.Fatalf("state occurrences=%d", seen)
	}
	requireCapabilityNumber(t, facts, "der.rating.active_power", "2500", "W")
	requireCapabilityNumber(t, facts, "der.settings.reference_voltage", "230", "V")
	requireCapabilityNumber(t, facts, "der.status.available_active_power", "32", "W")
	requireCapabilityNumber(t, facts, "der.control.active_power_limit.percent", "50", "% WMax")
	requireCapabilityNumber(t, facts, "der.control.power_factor.setpoint", "0.98", "cos()")
	requireCapabilityNumber(t, facts, "der.control.reactive_power.percent_wmax", "3", "% WMax")
	active, _ := capabilityFactByID(facts, "der.status.active_controls")
	if bits, _, ok := active.Value().Bitfield(); !ok || bits != 1 {
		t.Fatalf("active controls=%d %t", bits, ok)
	}
	limit, _ := capabilityFactByID(facts, "der.control.active_power_limit.enabled")
	if n, _, ok := limit.Value().Enum(); !ok || n != 1 {
		t.Fatalf("limit enable=%d %t", n, ok)
	}
	source, _ := capabilityFactByID(facts, "der.status.time_source")
	if label, ok := source.Value().Text(); !ok || label != "fixture" {
		t.Fatalf("time source=%q %t", label, ok)
	}
	if _, ok := capabilityFactByID(facts, "der.settings.max_voltage"); ok {
		t.Fatal("invalid scale was ignored")
	}
}
