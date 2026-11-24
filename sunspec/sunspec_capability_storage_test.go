package sunspec

import "testing"

func TestStorageStateEmitsNameplateAndNativeStorageControls(t *testing.T) {
	r := mustStandardSunSpecRegistry(t)
	decisions := evaluateFixtureCapabilities(t, snapshotFromOccurrences(commonOccurrence(t, r, "Synthetic maker", "Storage family", "Fixture", 1),
		admittedOccurrence(120, 26, modelWords(t, r, 120, 26, map[string][]uint16{"DERTyp": {4}, "WHRtg": {250}, "WHRtg_SF": {1}, "AhrRtg": {50}}), 2),
		admittedOccurrence(124, 24, modelWords(t, r, 124, 24, map[string][]uint16{"ChaState": {35}, "InWRte": {25}, "OutWRte": {0xffe2}, "InOutWRte_SF": {0xffff}, "InBatV_SF": {0x8000}}), 3)))
	var storage []CapabilityDecision
	for _, d := range decisions {
		if d.ProfileID() == "sunspec.storage.monitoring" {
			storage = append(storage, d)
		}
	}
	if len(storage) != 2 || !storage[0].Admitted() || !storage[1].Admitted() {
		t.Fatalf("storage=%+v", storage)
	}
	requireCapabilityNumber(t, storage[0].Facts(), "storage.rating.energy", "2500", "Wh")
	requireCapabilityNumber(t, storage[0].Facts(), "storage.rating.charge", "50", "AH")
	requireCapabilityNumber(t, storage[1].Facts(), "storage.status.state_of_charge", "35", "% AhrRtg")
	requireCapabilityNumber(t, storage[1].Facts(), "storage.control.charge_rate", "2.5", "% WChaMax")
	requireCapabilityNumber(t, storage[1].Facts(), "storage.control.discharge_rate", "-3", "% WDisChaMax")
	if _, ok := capabilityFactByID(storage[1].Facts(), "storage.status.internal_battery_voltage"); ok {
		t.Fatal("unavailable battery voltage emitted")
	}
}

func TestStorageStateSeparatesBatteryOccurrencesAndRepeatGroups(t *testing.T) {
	r := listedRegistry(t, ModelsRevisionV2)
	plan, err := NewListedChainPlan([]uint16{1, 803, 804, 805}, []uint16{40000}, listedChainLimits())
	if err != nil {
		t.Fatal(err)
	}
	bank := v2DERModelWords(t, r, 803, 90, map[string][]uint16{"NStr": {2}, "NStrCon": {1}, "SoC_SF": {0xffff}})
	bank[32], bank[64] = 125, 250
	str := v2DERModelWords(t, r, 804, 62, map[string][]uint16{"Idx": {9}, "NMod": {1}, "SoC": {74}, "ModSoC": {43}})
	mod := v2DERModelWords(t, r, 805, 42, map[string][]uint16{"StrIdx": {12}, "ModIdx": {7}, "SoC": {54}})
	decisions := evaluateFixtureCapabilities(t, walkListedChain(t, plan, [][]uint16{derCommonFixtureWords(t), bank, str, mod}))
	if len(decisions) != 3 {
		t.Fatalf("storage decisions=%+v", decisions)
	}
	for i, d := range decisions {
		source, ok := d.SourceOccurrence()
		if !ok || source.Ordinal != uint32(i+2) || !d.Admitted() || d.ProfileID() != "sunspec.storage.monitoring" {
			t.Fatalf("storage occurrence %d: %+v", i, d)
		}
	}
	requireCapabilityNumber(t, decisions[0].Facts(), "battery.bank.connected_strings", "1", "none")
	strings := decisions[0].RepeatedEntries()
	modules := decisions[1].RepeatedEntries()
	if len(strings) != 2 || len(modules) != 1 || len(decisions[2].RepeatedEntries()) != 0 {
		t.Fatalf("strings=%+v modules=%+v", strings, modules)
	}
	for i, entry := range strings {
		if entry.GroupID != "string" || entry.Index != uint16(i+1) {
			t.Fatalf("string %d: %+v", i, entry)
		}
		requireCapabilityNumber(t, entry.Facts, "battery.string.state_of_charge", []string{"12.5", "25"}[i], "%")
	}
	if modules[0].GroupID != "module" || modules[0].Index != 1 {
		t.Fatalf("module: %+v", modules[0])
	}
	requireCapabilityNumber(t, decisions[1].Facts(), "battery.string.index", "9", "none")
	requireCapabilityNumber(t, decisions[1].Facts(), "battery.string.state_of_charge", "74", "%")
	requireCapabilityNumber(t, modules[0].Facts, "battery.module.state_of_charge", "43", "%")
	requireCapabilityNumber(t, decisions[2].Facts(), "battery.module.state_of_charge", "54", "%")
	requireCapabilityNumber(t, decisions[2].Facts(), "battery.module.index", "7", "none")
	requireCapabilityNumber(t, decisions[2].Facts(), "battery.module.string_index", "12", "none")
}
