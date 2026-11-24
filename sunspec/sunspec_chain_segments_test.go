package sunspec

import (
	"testing"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestSegmentChainSplitsEachCommonModelIntoItsOwnDevice(t *testing.T) {
	leading := segmentFixtureOccurrence(1, 160, 8)
	inverterCommon := segmentFixtureOccurrence(2, 1, 66)
	inverter := segmentFixtureOccurrence(3, 103, 50)
	nameplate := segmentFixtureOccurrence(4, 120, 26)
	meterCommon := segmentFixtureOccurrence(5, 1, 65)
	meter := segmentFixtureOccurrence(6, 203, 105)
	snapshot, err := NewChainSnapshot(nil, []Occurrence{leading, inverterCommon, inverter, nameplate, meterCommon, meter}, nil)
	if err != nil {
		t.Fatal(err)
	}

	segments := SegmentChain(snapshot)
	if len(segments) != 2 {
		t.Fatalf("segments=%d", len(segments))
	}
	assertSegmentOccurrences(t, segments[0], []segmentIdentity{
		{2, 1, 66},
		{3, 103, 50},
		{4, 120, 26},
	})
	assertSegmentOccurrences(t, segments[1], []segmentIdentity{
		{5, 1, 65},
		{6, 203, 105},
	})

	returned := segments[0].Occurrences()
	returned[1].words[0] = 0
	returned[1].WireKey.ModelID = 1
	if got := segments[0].Occurrences()[1]; got.ModelID() != 103 || got.ModelLength() != 50 || got.Ordinal != 3 || got.words[0] != 103 {
		t.Fatalf("segment leaked a caller mutation: %#v", got)
	}
	if got := snapshot.Occurrences()[2]; got.ModelID() != 103 || got.words[0] != 103 {
		t.Fatalf("segment aliased the snapshot: %#v", got)
	}
}

func TestSegmentChainOmitsAChainWithNoCommonModel(t *testing.T) {
	snapshot, err := NewChainSnapshot(nil, []Occurrence{segmentFixtureOccurrence(1, 103, 50)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := SegmentChain(snapshot); len(got) != 0 {
		t.Fatalf("segments=%d", len(got))
	}
}

func TestAllowlistPlanReadsInverterAndMeterOnOneEndpoint(t *testing.T) {
	const (
		endpoint = "synthetic-bus"
		base     = uint16(40000)
		inverter = byte(17)
		meter    = byte(29)
	)
	plan, err := PlanAllowlistedUnits(UnitAllowlist{
		Endpoint:       endpoint,
		BaseCandidates: []uint16{base},
		Limits:         listedChainLimits(),
		Units: []AllowlistedUnit{
			{UnitID: inverter, Kind: ChainUnitInverter, ListedModels: []uint16{1, 701}},
			{UnitID: meter, Kind: ChainUnitMeter, ListedModels: []uint16{1, 203}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	units := plan.Units()
	if len(units) != 2 {
		t.Fatalf("units=%d", len(units))
	}
	assertAllowlistedUnit(t, units[0], endpoint, inverter, ChainUnitInverter, base)
	assertAllowlistedUnit(t, units[1], endpoint, meter, ChainUnitMeter, base)
	if units[0].chain == nil || units[1].chain == nil || units[0].chain == units[1].chain || units[0].chain.instance == units[1].chain.instance || units[0].chain.plan.nonce == units[1].chain.plan.nonce {
		t.Fatal("allowlisted units share one chain plan")
	}
	assertPlanKey(t, units[0].Plan(), DecoderKey{ModelID: 701, ModelLength: 153, SchemaRevision: ModelsRevisionV2})
	if _, ok := units[0].Plan().structuralCandidateIDs[707]; !ok || len(units[0].Plan().structuralCandidateIDs) != 1 {
		t.Fatalf("inverter structural candidates=%v", units[0].Plan().structuralCandidateIDs)
	}
	assertPlanKey(t, units[1].Plan(), DecoderKey{ModelID: 203, ModelLength: 105, SchemaRevision: ModelsRevisionV1})
	if len(units[1].Plan().structuralCandidateIDs) != 0 || units[1].Plan().SchemaRevision() != ModelsRevisionV1 {
		t.Fatalf("meter plan revision=%q candidates=%v", units[1].Plan().SchemaRevision(), units[1].Plan().structuralCandidateIDs)
	}

	request := units[0].NextRequests()[0]
	foreignUnit := allowlistedView(t, request, 1, []uint16{sunSpecSignatureFirst, sunSpecSignatureSecond}, endpoint, meter)
	if _, err := units[0].AdmitReplay(request, foreignUnit); err == nil {
		t.Fatal("a meter unit view was admitted on the inverter chain")
	}
	if units[0].NextRequests()[0] != request {
		t.Fatal("a foreign unit changed the inverter chain")
	}
	foreignEndpoint := allowlistedView(t, request, 2, []uint16{sunSpecSignatureFirst, sunSpecSignatureSecond}, "other-bus", inverter)
	if _, err := units[0].AdmitReplay(request, foreignEndpoint); err == nil {
		t.Fatal("another endpoint was admitted on the inverter chain")
	}
	if units[0].NextRequests()[0] != request {
		t.Fatal("a foreign endpoint changed the inverter chain")
	}
	matching := allowlistedView(t, request, 3, []uint16{sunSpecSignatureFirst, sunSpecSignatureSecond}, endpoint, inverter)
	if _, err := units[0].AdmitReplay(request, matching); err != nil {
		t.Fatal(err)
	}
	if got := units[0].NextRequests(); len(got) != 1 || got[0].Purpose() != ReadHeader {
		t.Fatalf("inverter chain did not advance: %#v", got)
	}
	meterRequest := units[1].NextRequests()[0]
	if _, err := units[1].AdmitReplay(request, matching); err == nil {
		t.Fatal("the inverter chain request was admitted on the meter chain")
	}
	meterView := allowlistedView(t, meterRequest, 4, []uint16{sunSpecSignatureFirst, sunSpecSignatureSecond}, endpoint, meter)
	if _, err := units[1].AdmitReplay(meterRequest, meterView); err != nil {
		t.Fatal(err)
	}
	if got := units[1].NextRequests(); len(got) != 1 || got[0].Purpose() != ReadHeader || got[0].Address() != base+2 {
		t.Fatalf("meter chain did not advance: %#v", got)
	}
}

func TestListedDERChainDecodesWithTheDERRegistry(t *testing.T) {
	plan := listedDERPlan(t)
	if plan.SchemaRevision() != ModelsRevisionV2 {
		t.Fatalf("revision=%q", plan.SchemaRevision())
	}
	for _, key := range []DecoderKey{
		{ModelID: 1, ModelLength: 66, SchemaRevision: ModelsRevisionV2},
		{ModelID: 701, ModelLength: 153, SchemaRevision: ModelsRevisionV2},
		{ModelID: 713, ModelLength: 7, SchemaRevision: ModelsRevisionV2},
		{ModelID: 802, ModelLength: 62, SchemaRevision: ModelsRevisionV2},
	} {
		assertPlanKey(t, plan, key)
	}
	snapshot := walkListedChain(t, plan, [][]uint16{
		modelBlock(1, 66),
		modelBlock(701, 153),
		modelBlock(713, 7),
		modelBlock(802, 62),
	})
	registry, err := NewStandardSunSpecDecoderRegistry(plan.SchemaRevision())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := registry.DecodeChain(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	models := decoded.Models()
	if len(models) != 4 {
		t.Fatalf("models=%d", len(models))
	}
	for index, key := range []DecoderKey{
		{ModelID: 1, ModelLength: 66, SchemaRevision: ModelsRevisionV2},
		{ModelID: 701, ModelLength: 153, SchemaRevision: ModelsRevisionV2},
		{ModelID: 713, ModelLength: 7, SchemaRevision: ModelsRevisionV2},
		{ModelID: 802, ModelLength: 62, SchemaRevision: ModelsRevisionV2},
	} {
		if models[index].Key() != key || models[index].Ordinal() != uint32(index+1) {
			t.Fatalf("model %d key=%#v ordinal=%d", index, models[index].Key(), models[index].Ordinal())
		}
	}
}

func TestListedChainKeepsCoreModelsBesideTheDERRegistry(t *testing.T) {
	plan, err := NewListedChainPlan([]uint16{1, 103, 120, 701}, []uint16{40000}, listedChainLimits())
	if err != nil {
		t.Fatal(err)
	}
	if plan.SchemaRevision() != ModelsRevisionV2 {
		t.Fatalf("revision=%q", plan.SchemaRevision())
	}
	if _, ok := plan.structuralCandidateIDs[707]; !ok || len(plan.structuralCandidateIDs) != 1 {
		t.Fatalf("structural candidates=%v", plan.structuralCandidateIDs)
	}
	want := []DecoderKey{
		{ModelID: 1, ModelLength: 66, SchemaRevision: ModelsRevisionV2},
		{ModelID: 103, ModelLength: 50, SchemaRevision: ModelsRevisionV1},
		{ModelID: 120, ModelLength: 26, SchemaRevision: ModelsRevisionV1},
		{ModelID: 701, ModelLength: 153, SchemaRevision: ModelsRevisionV2},
	}
	for _, key := range want {
		assertPlanKey(t, plan, key)
	}
	if got := plan.keys[WireKey{ModelID: 1, ModelLength: 66}]; got.SchemaRevision != ModelsRevisionV2 {
		t.Fatalf("common model key=%#v", got)
	}

	snapshot := walkListedChain(t, plan, [][]uint16{
		modelBlock(1, 66),
		modelBlock(103, 50),
		modelBlock(120, 26),
		modelBlock(701, 153),
	})
	occurrences := snapshot.Occurrences()
	if len(occurrences) != len(want) {
		t.Fatalf("occurrences=%d", len(occurrences))
	}
	core := listedRegistry(t, ModelsRevisionV1)
	der := listedRegistry(t, ModelsRevisionV2)
	for index, key := range want {
		occurrence := occurrences[index]
		got, ok := occurrence.DecoderKey()
		if !ok || got != key || occurrence.SchemaRevision != key.SchemaRevision || occurrence.Disposition != ChainDispositionAdmitted {
			t.Fatalf("occurrence %d key=%#v revision=%q disposition=%q", index, got, occurrence.SchemaRevision, occurrence.Disposition)
		}
		registry := der
		if key.SchemaRevision == ModelsRevisionV1 {
			registry = core
		}
		decoded, err := registry.DecodeOccurrence(occurrence)
		if err != nil {
			t.Fatal(err)
		}
		if decoded.Key() != key || decoded.Ordinal() != uint32(index+1) || len(decoded.Facts()) == 0 {
			t.Fatalf("model %d key=%#v ordinal=%d facts=%d", index, decoded.Key(), decoded.Ordinal(), len(decoded.Facts()))
		}
	}
}

func TestListedNameplateModelStaysInTheReadPlan(t *testing.T) {
	withNameplate, err := NewListedChainPlan([]uint16{1, 103, 120}, []uint16{40000}, listedChainLimits())
	if err != nil {
		t.Fatal(err)
	}
	assertPlanKey(t, withNameplate, DecoderKey{ModelID: 120, ModelLength: 26, SchemaRevision: ModelsRevisionV1})
	if withNameplate.SchemaRevision() != ModelsRevisionV1 {
		t.Fatalf("revision=%q", withNameplate.SchemaRevision())
	}
	var nameplateKeys int
	for _, key := range withNameplate.keys {
		if key.ModelID == 120 {
			nameplateKeys++
			if key.ModelLength != 26 || key.SchemaRevision != ModelsRevisionV1 {
				t.Fatalf("nameplate key=%#v", key)
			}
		}
	}
	if nameplateKeys != 1 {
		t.Fatalf("nameplate keys=%d", nameplateKeys)
	}

	withoutNameplate, err := NewListedChainPlan([]uint16{1, 103}, []uint16{40000}, listedChainLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range withoutNameplate.keys {
		if key.ModelID == 120 {
			t.Fatalf("unlisted nameplate key=%#v", key)
		}
	}
}

func TestListedDERPlanRetainsStructuralProjection(t *testing.T) {
	plan := listedDERPlan(t)
	if _, ok := plan.structuralCandidateIDs[707]; !ok {
		t.Fatal("DER plan omitted the structural candidate")
	}
	valid := []uint16{707, 33, 0, 0, 0, 1, 2, 0, 0, 0, 1, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 1, 0, 1, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 1}
	snapshot := walkListedChain(t, plan, [][]uint16{modelBlock(1, 66), valid})
	occurrences := snapshot.ByModelID(707)
	if len(occurrences) != 1 {
		t.Fatalf("occurrences=%d", len(occurrences))
	}
	occurrence := occurrences[0]
	if occurrence.Disposition != ChainDispositionUnknownModel || occurrence.SchemaRevision != ModelsRevisionV2 {
		t.Fatalf("occurrence disposition=%q revision=%q", occurrence.Disposition, occurrence.SchemaRevision)
	}
	if _, ok := occurrence.DecoderKey(); ok || occurrence.structuralCandidate == nil {
		t.Fatal("structural candidate was admitted as a decoder or dropped")
	}
	projections := ProjectSunSpecStructuralFacts(snapshot)
	if len(projections) != 1 {
		t.Fatalf("projections=%d", len(projections))
	}
	projection := projections[0]
	if projection.WireKey() != (WireKey{ModelID: 707, ModelLength: 33}) || projection.SchemaRevision() != ModelsRevisionV2 || len(projection.Facts()) != 29 {
		t.Fatalf("projection key=%#v revision=%q facts=%d", projection.WireKey(), projection.SchemaRevision(), len(projection.Facts()))
	}
	registry, err := NewStandardSunSpecDecoderRegistry(plan.SchemaRevision())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := registry.DecodeChain(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range decoded.Models() {
		if model.Key().ModelID == 707 {
			t.Fatal("structural candidate was decoded")
		}
	}
}

type segmentIdentity struct {
	ordinal uint32
	modelID uint16
	length  uint16
}

func assertSegmentOccurrences(t *testing.T, segment ChainSegment, want []segmentIdentity) {
	t.Helper()
	got := segment.Occurrences()
	if len(got) != len(want) {
		t.Fatalf("occurrences=%d want=%d", len(got), len(want))
	}
	for index, identity := range want {
		if got[index].Ordinal != identity.ordinal || got[index].ModelID() != identity.modelID || got[index].ModelLength() != identity.length {
			t.Fatalf("occurrence %d=%#v want=%#v", index, got[index].WireKey, identity)
		}
	}
}

func segmentFixtureOccurrence(ordinal uint32, modelID, length uint16) Occurrence {
	return Occurrence{
		Ordinal:        ordinal,
		WireKey:        WireKey{ModelID: modelID, ModelLength: length},
		SchemaRevision: ModelsRevisionV1,
		Disposition:    ChainDispositionUnknownModel,
		words:          []uint16{modelID, length},
	}
}

func listedChainLimits() ChainLimits {
	return ChainLimits{MaxTotalWords: 512, MaxOccurrences: 8}
}

func listedRegistry(t *testing.T, revision SchemaRevision) DecoderRegistry {
	t.Helper()
	registry, err := NewStandardSunSpecDecoderRegistry(revision)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func listedDERPlan(t *testing.T) ChainPlan {
	t.Helper()
	plan, err := NewListedChainPlan([]uint16{1, 701, 713, 802}, []uint16{40000}, listedChainLimits())
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func assertPlanKey(t *testing.T, plan ChainPlan, want DecoderKey) {
	t.Helper()
	for _, key := range plan.keys {
		if key == want {
			return
		}
	}
	t.Fatalf("plan revision=%q lacks key %#v", plan.SchemaRevision(), want)
}

func assertAllowlistedUnit(t *testing.T, unit UnitChain, endpoint string, unitID byte, kind ChainUnitKind, base uint16) {
	t.Helper()
	if unit.UnitID() != unitID || unit.Kind() != kind || unit.Endpoint() != endpoint {
		t.Fatalf("unit id=%d kind=%q endpoint=%q", unit.UnitID(), unit.Kind(), unit.Endpoint())
	}
	requests := unit.NextRequests()
	if len(requests) != 1 || requests[0].Function() != modbusreg.FunctionReadHoldingRegisters || requests[0].Address() != base || requests[0].WordCount() != 2 || requests[0].Purpose() != ReadSignature {
		t.Fatalf("initial read=%#v", requests)
	}
}

func allowlistedView(t *testing.T, request ReadRequest, id uint64, words []uint16, endpoint string, unitID byte) modbusreg.LogicalViewSnapshot {
	t.Helper()
	view, err := modbusreg.NewLogicalViewSnapshot(modbusreg.LogicalViewRecord{
		LogicalViewID: id, WireResponseID: id + 100, PhysicalRequestID: id + 200,
		Endpoint: endpoint, ConnectionID: 4, Transport: modbusreg.TransportTCP, TransportGeneration: 5,
		UnitID: unitID, RequestedFunction: modbusreg.FunctionReadHoldingRegisters, ReceivedFunction: modbusreg.FunctionReadHoldingRegisters,
		Table: modbusreg.HoldingRegisters, PhysicalOffset: request.Address(), PhysicalWordCount: request.WordCount(),
		AuthorizationScope: "read", PollGeneration: 6, DeadlineIdentity: 7,
		LogicalOffset: request.Address(), LogicalWordCount: request.WordCount(), SliceOffset: 0, SliceWordCount: request.WordCount(),
		Words: words, WireResponseBytes: []byte{byte(id)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func modelBlock(modelID, length uint16) []uint16 {
	block := make([]uint16, int(length)+2)
	block[0] = modelID
	block[1] = length
	return block
}

func walkListedChain(t *testing.T, plan ChainPlan, blocks [][]uint16) ChainSnapshot {
	t.Helper()
	chain := NewChain(plan)
	id := uint64(1)
	if _, err := admitNext(t, chain, &id, []uint16{sunSpecSignatureFirst, sunSpecSignatureSecond}); err != nil {
		t.Fatal(err)
	}
	var stream []uint16
	for _, block := range blocks {
		stream = append(stream, block...)
	}
	for len(stream) > 0 {
		requests := chain.NextRequests()
		if len(requests) == 0 || len(stream) < int(requests[0].WordCount()) {
			t.Fatalf("chain stopped with %d words left", len(stream))
		}
		chunk := stream[:requests[0].WordCount()]
		stream = stream[requests[0].WordCount():]
		if _, err := admitNext(t, chain, &id, chunk); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := admitNext(t, chain, &id, []uint16{EndModelID, 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.RawWords()) == 0 {
		t.Fatal("listed chain did not complete")
	}
	return snapshot
}
