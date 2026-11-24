package sunspec

import "testing"

func TestMPPTInputsKeepAllFourModulesWithoutInferringRoles(t *testing.T) {
	r := mustStandardSunSpecRegistry(t)
	words := modelWords(t, r, 160, 88, map[string][]uint16{"N": {4}, "DCW_SF": {0xffff}, "DCA_SF": {0x8000}})
	labels := []string{"Input 1", "Input 2", "Storage Charge", "Storage Disch"}
	for i, label := range labels {
		offset := 10 + 20*i
		words[offset] = uint16(31 + i)
		copy(words[offset+1:], stringWords(label, 8))
		words[offset+11] = uint16(11 * (i + 1))
		words[offset+17] = 4
	}
	common := commonOccurrence(t, r, "Synthetic maker", "Inverter family", "Fixture", 1)
	d := evaluateFixtureCapabilities(t, snapshotFromOccurrences(common, admittedOccurrence(160, 88, words, 2)))
	if len(d) != 1 || !d[0].Admitted() || d[0].ProfileID() != "sunspec.mppt.monitoring" {
		t.Fatalf("MPPT=%+v", d)
	}
	entries := d[0].RepeatedEntries()
	if len(entries) != 4 {
		t.Fatalf("entries=%+v", entries)
	}
	for i, entry := range entries {
		if entry.Index != uint16(i+1) || entry.Role != "unknown" {
			t.Fatalf("entry %d: %+v", i, entry)
		}
		facts := entry.Facts
		requireCapabilityNumber(t, facts, "mppt.module.id", []string{"31", "32", "33", "34"}[i], "none")
		requireCapabilityNumber(t, facts, "mppt.module.index", []string{"1", "2", "3", "4"}[i], "none")
		power := requireCapabilityNumber(t, facts, "mppt.module.dc_power", []string{"1.1", "2.2", "3.3", "4.4"}[i], "W")
		if exponent, ok := power.ScaleFactor(); !ok || exponent != -1 {
			t.Fatalf("scale=%d %t", exponent, ok)
		}
		field, _ := capabilityFactByID(facts, "mppt.module.label")
		if label, ok := field.Value().Text(); !ok || label != labels[i] {
			t.Fatalf("label=%q %t", label, ok)
		}
		if _, ok := capabilityFactByID(facts, "mppt.module.dc_current"); ok {
			t.Fatal("unavailable current emitted")
		}
	}
	entries[0].Facts[0] = CapabilityFact{}
	if d[0].RepeatedEntries()[0].Facts[0].FieldID() != "mppt.module.index" {
		t.Fatal("entry accessor shares storage")
	}
	words[8] = 0xffff
	unavailable := evaluateFixtureCapabilities(t, snapshotFromOccurrences(common, admittedOccurrence(160, 88, words, 2)))
	if len(unavailable) != 1 || unavailable[0].Admitted() || len(unavailable[0].RepeatedEntries()) != 0 {
		t.Fatalf("unavailable count emitted entries: %+v", unavailable)
	}
}
