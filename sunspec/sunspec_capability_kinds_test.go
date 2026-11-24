package sunspec

import "testing"

func TestInverterMonitoringUsesOnlyItsModelPhases(t *testing.T) {
	r := mustStandardSunSpecRegistry(t)
	for _, tc := range []struct {
		id       uint16
		topology Topology
		phases   int
	}{
		{101, TopologySinglePhase, 1}, {102, TopologySplitPhase, 2}, {103, TopologyThreePhase, 3},
		{111, TopologySinglePhase, 1}, {112, TopologySplitPhase, 2}, {113, TopologyThreePhase, 3},
	} {
		common := commonOccurrence(t, r, "Synthetic maker", "Inverter family", "Fixture", 1)
		decisions := evaluateFixtureCapabilities(t, snapshotFromOccurrences(common, phaseInverterFixture(t, r, tc.id, 2)))
		if len(decisions) != 1 || !decisions[0].Admitted() || decisions[0].ProfileID() != "sunspec.inverter.monitoring" || decisions[0].Topology() != tc.topology {
			t.Fatalf("model %d decisions=%+v", tc.id, decisions)
		}
		facts := decisions[0].Facts()
		for i, suffix := range []string{"a", "b", "c"} {
			for _, prefix := range []string{"inverter.ac.current.phase_", "inverter.ac.voltage.phase_"} {
				if _, ok := capabilityFactByID(facts, prefix+suffix); ok != (i < tc.phases) {
					t.Fatalf("model %d %s%s present=%v", tc.id, prefix, suffix, ok)
				}
			}
		}
		for i, suffix := range []string{"ab", "bc", "ca"} {
			if _, ok := capabilityFactByID(facts, "inverter.ac.voltage.line_"+suffix); ok != (tc.phases == 3 || (tc.phases == 2 && i == 0)) {
				t.Fatalf("model %d line %s present=%v", tc.id, suffix, ok)
			}
		}
		requireCapabilityNumber(t, facts, "inverter.ac.current.total", "1", "A")
	}
}

func TestSunSpecCapabilityAttributesModelOccurrencesToTheirNearestCommon(t *testing.T) {
	r := mustStandardSunSpecRegistry(t)
	common := func(i uint32) Occurrence {
		return commonOccurrence(t, r, "Synthetic maker", "Device family", "Fixture", i)
	}
	snapshot := snapshotFromOccurrences(phaseInverterFixture(t, r, 101, 1), common(2), phaseInverterFixture(t, r, 103, 3), phaseInverterFixture(t, r, 103, 4), common(5), meterCapabilityFixture(t, r, 203, 6, nil))
	decisions := evaluateFixtureCapabilities(t, snapshot)
	if len(decisions) != 4 || decisions[0].Admitted() || decisions[0].Reason() != CapabilityReasonAmbiguousSource || len(decisions[0].Facts()) != 0 {
		t.Fatalf("unattributed prefix: %+v", decisions)
	}
	for i, want := range []struct {
		source, common uint32
		profile        string
	}{
		{3, 2, "sunspec.inverter.monitoring"}, {4, 2, "sunspec.inverter.monitoring"}, {6, 5, "sunspec.meter.monitoring"},
	} {
		d := decisions[i+1]
		occurrence, ok := d.SourceOccurrence()
		anchor, anchored := d.CommonOccurrence()
		if !d.Admitted() || d.ProfileID() != want.profile || !ok || occurrence.Ordinal != want.source || !anchored || anchor.Ordinal != want.common {
			t.Fatalf("decision %d: %+v source=%d common=%d", i, d, occurrence.Ordinal, anchor.Ordinal)
		}
		if len(d.SourceViews()) != snapshot.SourceViewCount() || occurrence.Words()[0] != snapshot.occurrences[want.source-1].ModelID() {
			t.Fatal("source evidence was not retained")
		}
	}

	clean := completedChainSnapshot(t, r, common(1), phaseInverterFixture(t, r, 103, 2), common(3), meterCapabilityFixture(t, r, 213, 4, nil))
	decisions = evaluateFixtureCapabilities(t, clean)
	if len(decisions) != 2 || !decisions[0].Admitted() || !decisions[1].Admitted() || decisions[0].ProfileID() != "sunspec.inverter.monitoring" || decisions[1].ProfileID() != "sunspec.meter.monitoring" {
		t.Fatalf("two-Common replay: %+v", decisions)
	}
	requireCapabilityNumber(t, decisions[0].Facts(), "inverter.ac.current.total", "1", "A")
	requireCapabilityNumber(t, decisions[1].Facts(), "meter.energy.export.total", "1", "Wh")
}

func TestSunSpecCapabilityRejectsInvalidSourcesWithoutErasingValidSiblings(t *testing.T) {
	r := mustStandardSunSpecRegistry(t)
	wrong := admittedOccurrence(103, 49, append([]uint16{103, 49}, make([]uint16, 49)...), 2)
	wrong.Disposition, wrong.decoderKey = ChainDispositionUnsupportedLength, nil
	invalid := inverterOccurrence(t, r, 103, map[string][]uint16{"A_SF": {0x8000}}, 3)
	bad := snapshotFromOccurrences(commonOccurrence(t, r, "Synthetic maker", "Device family", "Fixture", 1), wrong, invalid, meterCapabilityFixture(t, r, 203, 4, nil))
	decisions := evaluateFixtureCapabilities(t, bad)
	if len(decisions) != 3 || decisions[0].Reason() != CapabilityReasonSourceUnsupported || decisions[1].Reason() != CapabilityReasonInvalidRequiredFact || !decisions[2].Admitted() || len(decisions[0].Facts())+len(decisions[1].Facts()) != 0 {
		t.Fatalf("decisions=%+v", decisions)
	}
	bad.raw = bad.raw[:len(bad.raw)-2]
	if decisions, err := EvaluateCapabilities(bad); err == nil || len(decisions) != 0 {
		t.Fatal("missing terminal was accepted")
	}
}

func phaseInverterFixture(t *testing.T, r DecoderRegistry, id uint16, ordinal uint32) Occurrence {
	t.Helper()
	if id < 110 {
		return inverterOccurrence(t, r, id, nil, ordinal)
	}
	wire := inverterOccurrence(t, r, 113, nil, ordinal).Words()
	wire[0] = id
	return admittedOccurrence(id, 60, wire, ordinal)
}

func evaluateFixtureCapabilities(t *testing.T, snapshot ChainSnapshot) []CapabilityDecision {
	t.Helper()
	decisions, err := EvaluateCapabilities(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return decisions
}

func requireCapabilityNumber(t *testing.T, facts []CapabilityFact, id, number, unit string) CapabilityFact {
	t.Helper()
	f, ok := capabilityFactByID(facts, id)
	value, numeric := f.Value().Number()
	if !ok || !numeric || value != number || f.Unit() != unit {
		t.Fatalf("%s = %q %q (present=%v numeric=%v), want %q %q", id, value, f.Unit(), ok, numeric, number, unit)
	}
	return f
}

func TestSunSpecCapabilityRequiresTheRetainedDecoderKey(t *testing.T) {
	r := mustStandardSunSpecRegistry(t)
	valid := capabilitySnapshot(t, r, 103, nil)
	for _, tc := range []struct {
		name   string
		mutate func(*Occurrence)
	}{
		{"model", func(o *Occurrence) { o.decoderKey.ModelID = 102 }},
		{"length", func(o *Occurrence) { o.decoderKey.ModelLength = 49 }},
		{"key revision", func(o *Occurrence) { o.decoderKey.SchemaRevision = ModelsRevisionV2 }},
		{"occurrence revision", func(o *Occurrence) { o.SchemaRevision = ModelsRevisionV2 }},
		{"missing key", func(o *Occurrence) { o.decoderKey = nil }},
		{"unsupported disposition", func(o *Occurrence) { o.Disposition = ChainDispositionUnknownModel }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := cloneSnapshotForTest(valid)
			tc.mutate(&input.occurrences[1])
			decisions := evaluateFixtureCapabilities(t, input)
			if len(decisions) != 1 || decisions[0].Admitted() || decisions[0].Reason() != CapabilityReasonSourceUnsupported || len(decisions[0].Facts()) != 0 {
				t.Fatalf("decisions=%+v", decisions)
			}
		})
	}
	if got := evaluateFixtureCapabilities(t, valid); len(got) != 1 || !got[0].Admitted() {
		t.Fatalf("valid key rejected: %+v", got)
	}
}

func TestDERMeasurementReusesOnlyEquivalentCoreFacts(t *testing.T) {
	r := listedRegistry(t, ModelsRevisionV2)
	plan, err := NewListedChainPlan([]uint16{1, 103, 701, 713, 714, 802}, []uint16{40000}, ChainLimits{MaxTotalWords: 512, MaxOccurrences: 8})
	if err != nil {
		t.Fatal(err)
	}
	inverter := inverterOccurrence(t, mustStandardSunSpecRegistry(t), 103, nil, 2).Words()
	ac := v2DERModelWords(t, r, 701, 153, map[string][]uint16{"ACType": {2}, "W": {0xffe2}, "W_SF": {0xffff}, "AL1": {0xfff4}, "VL1": {230}, "Hz": {0, 50}, "PF": {95}, "TotWhInj": {0x8000, 0, 0, 1}, "TotWh_SF": {0xffff}})
	capacity := v2DERModelWords(t, r, 713, 7, map[string][]uint16{"WHRtg": {400}, "SoC": {65}})
	port := v2DERModelWords(t, r, 714, 68, map[string][]uint16{"NPrt": {2}})
	port[5], port[30], port[55] = 2, 6, 7
	port[20], port[21], port[31] = 1, 21, 15
	port[45], port[46], port[56] = 2, 22, 25
	battery := v2DERModelWords(t, r, 802, 62, map[string][]uint16{"AHRtg": {80}, "WHRtg": {350}, "SoC": {75}, "WChaRteMax": {280}, "W": {125}})
	decisions := evaluateFixtureCapabilities(t, walkListedChain(t, plan, [][]uint16{derCommonFixtureWords(t), inverter, ac, capacity, port, battery}))
	if len(decisions) != 5 {
		t.Fatalf("decisions=%+v", decisions)
	}
	for i, id := range []string{"sunspec.inverter.monitoring", "sunspec.der.measurement", "sunspec.storage.monitoring", "sunspec.der.measurement", "sunspec.storage.monitoring"} {
		if !decisions[i].Admitted() || decisions[i].ProfileID() != id {
			t.Fatalf("decision %d: %+v", i, decisions[i])
		}
	}
	facts := decisions[1].Facts()
	requireCapabilityNumber(t, facts, "inverter.ac.power.active", "-3", "W")
	requireCapabilityNumber(t, facts, "inverter.ac.voltage.phase_a", "230", "V")
	requireCapabilityNumber(t, facts, "inverter.ac.frequency", "50", "Hz")
	requireCapabilityNumber(t, facts, "inverter.ac.power_factor", "95", "Pct")
	requireCapabilityNumber(t, facts, "der.ac.current.phase_a", "-12", "A")
	requireCapabilityNumber(t, facts, "der.ac.energy.injected", "922337203685477580.9", "Wh")
	for _, id := range []string{"inverter.ac.current.phase_a", "meter.ac.power.active.total", "inverter.ac.energy_lifetime"} {
		if _, ok := capabilityFactByID(facts, id); ok {
			t.Fatalf("nonmatching identifier reused: %s", id)
		}
	}
	requireCapabilityNumber(t, decisions[2].Facts(), "storage.status.state_of_charge", "65", "Pct")
	requireCapabilityNumber(t, decisions[2].Facts(), "storage.rating.energy", "400", "Wh")
	requireCapabilityNumber(t, decisions[4].Facts(), "storage.status.state_of_charge", "75", "%WHRtg")
	requireCapabilityNumber(t, decisions[4].Facts(), "storage.rating.energy", "350", "Wh")
	requireCapabilityNumber(t, decisions[4].Facts(), "storage.rating.charge", "80", "Ah")
	requireCapabilityNumber(t, decisions[4].Facts(), "storage.rating.max_charge_power", "280", "W")
	requireCapabilityNumber(t, decisions[4].Facts(), "battery.base.power", "125", "W")
	entries := decisions[3].RepeatedEntries()
	if len(entries) != 2 {
		t.Fatalf("ports=%+v", entries)
	}
	requireCapabilityNumber(t, decisions[3].Facts(), "der.dc.current", "2", "A")
	for i, entry := range entries {
		if entry.Index != uint16(i+1) || entry.GroupID != "port" {
			t.Fatalf("port %d: %+v", i, entry)
		}
		requireCapabilityNumber(t, entry.Facts, "der.dc.port.id", []string{"21", "22"}[i], "none")
		requireCapabilityNumber(t, entry.Facts, "der.dc.port.current", []string{"6", "7"}[i], "A")
		requireCapabilityNumber(t, entry.Facts, "der.dc.port.voltage", []string{"15", "25"}[i], "V")
		typ, _ := capabilityFactByID(entry.Facts, "der.dc.port.type")
		number, _, ok := typ.SourceValue().Enum()
		if !ok || number != uint64(i+1) {
			t.Fatalf("port type %d: %d %t", i, number, ok)
		}
	}
}

func derCommonFixtureWords(t *testing.T) []uint16 {
	t.Helper()
	r := listedRegistry(t, ModelsRevisionV2)
	return v2DERModelWords(t, r, 1, 66, map[string][]uint16{"Mn": stringWords("Synthetic maker", 16), "Md": stringWords("Device family", 16), "SN": stringWords("fixture", 16), "Pad": {0x8000}})
}
