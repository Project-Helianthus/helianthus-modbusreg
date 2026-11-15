package sunspecfixture

import (
	"math"
	"slices"
	"sync"
	"testing"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
)

// Revision is the schema revision of every fixture registry and chain.
const Revision = sunspec.ModelsRevisionV1

// SignatureFirst and SignatureSecond are the two words of the SunSpec
// signature "SunS" that open every chain image.
const (
	SignatureFirst  uint16 = 0x5375
	SignatureSecond uint16 = 0x6e53
)

// Base is the zero-based PDU register address every fixture chain starts at.
const Base uint16 = 40000

// FirstLogicalViewID is the identifier of the first logical view a fixture
// walk delivers; each later view takes the next identifier.
const FirstLogicalViewID uint64 = 1000

// Registry returns the standard decoder registry of Revision.
func Registry(t testing.TB) sunspec.DecoderRegistry {
	t.Helper()
	registry, err := sunspec.NewStandardSunSpecDecoderRegistry(Revision)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

// RevisionRegistry returns the standard decoder registry of revision.
func RevisionRegistry(t testing.TB, revision sunspec.SchemaRevision) sunspec.DecoderRegistry {
	t.Helper()
	registry, err := sunspec.NewStandardSunSpecDecoderRegistry(revision)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

// StringWords packs value into words registers, two bytes per register, high
// byte first, padded with zero bytes.
func StringWords(value string, words int) []uint16 {
	raw := make([]byte, words*2)
	copy(raw, []byte(value))
	out := make([]uint16, words)
	for index := range out {
		out[index] = uint16(raw[index*2])<<8 | uint16(raw[index*2+1])
	}
	return out
}

// Float32Words returns the two registers of an IEEE 754 single-precision
// value, high word first.
func Float32Words(value float32) []uint16 {
	bits := math.Float32bits(value)
	return []uint16{uint16(bits >> 16), uint16(bits)}
}

// Model123Values returns valid point values for the immediate-controls model
// 123 with length 24.
func Model123Values() map[string][]uint16 {
	return map[string][]uint16{
		"Conn":          {1},
		"WMaxLimPct":    {500},
		"WMaxLim_Ena":   {1},
		"OutPFSet":      {950},
		"OutPFSet_Ena":  {1},
		"VArPct_Mod":    {3},
		"VArPct_Ena":    {0},
		"WMaxLimPct_SF": {0xffff},
		"OutPFSet_SF":   {0xfffd},
		"VArPct_SF":     {0xffff},
	}
}

// CommonOccurrence returns an admitted Common Model occurrence with the given
// identity strings and a sanitized serial number.
func CommonOccurrence(t testing.TB, registry sunspec.DecoderRegistry, manufacturer, model, firmware string, ordinal uint32) sunspec.Occurrence {
	t.Helper()
	return Occurrence(t, registry, ModelWords(t, registry, 1, 65, map[string][]uint16{
		"Mn": StringWords(manufacturer, 16), "Md": StringWords(model, 16), "Vr": StringWords(firmware, 8), "SN": StringWords("sanitized", 16),
	}), ordinal)
}

// InverterOccurrence returns an admitted three-phase-capable inverter
// occurrence of modelID with valid monitoring values, each overridden by
// override. Model 113 uses its float layout of length 60; every other model
// uses the integer layout of length 50.
func InverterOccurrence(t testing.TB, registry sunspec.DecoderRegistry, modelID uint16, override map[string][]uint16, ordinal uint32) sunspec.Occurrence {
	t.Helper()
	length := uint16(50)
	values := map[string][]uint16{
		"A": {1}, "AphA": {1}, "AphB": {1}, "AphC": {1}, "A_SF": {0},
		"PhVphA": {1}, "PhVphB": {1}, "PhVphC": {1}, "V_SF": {0},
		"W": {1}, "W_SF": {0}, "Hz": {1}, "Hz_SF": {0}, "WH": {0, 1}, "WH_SF": {0},
		"TmpCab": {1}, "Tmp_SF": {0}, "St": {4}, "Evt1": {0, 0}, "Evt2": {0, 0},
	}
	if modelID == 113 {
		length = 60
		for _, name := range []string{"A", "AphA", "AphB", "AphC", "PhVphA", "PhVphB", "PhVphC", "W", "Hz", "WH", "TmpCab"} {
			values[name] = Float32Words(1)
		}
		for _, name := range []string{"A_SF", "V_SF", "W_SF", "Hz_SF", "WH_SF", "Tmp_SF"} {
			delete(values, name)
		}
	}
	for name, value := range override {
		values[name] = value
	}
	return Occurrence(t, registry, ModelWords(t, registry, modelID, length, values), ordinal)
}

// Occurrence returns the occurrence that a chain walk under every decoder key
// of registry produces for words, a complete model block beginning with its
// two header words, with Ordinal set to ordinal. The disposition is admitted
// when registry holds the exact key, unsupported length when it holds the
// model with another length, and unknown model otherwise. Snapshot and Walk
// use only the words of an occurrence.
func Occurrence(t testing.TB, registry sunspec.DecoderRegistry, words []uint16, ordinal uint32) sunspec.Occurrence {
	t.Helper()
	if len(words) < 2 || len(words) != int(words[1])+2 {
		t.Fatalf("occurrence words=%d do not hold a complete model block", len(words))
	}
	walked := WalkWords(t, sunspec.ChainPlanSpec{DecoderKeys: registry.DecoderKeys()}, nil, words)
	occurrences := walked.Occurrences()
	if len(occurrences) != 1 {
		t.Fatalf("occurrence walk produced %d occurrences", len(occurrences))
	}
	occurrence := occurrences[0]
	occurrence.Ordinal = ordinal
	return occurrence
}

// ModelWords returns the words of a model block id/length of registry: the
// two header words, every point named in values at its catalog offset in every
// repetition, and zero elsewhere. A value whose length differs from its
// point's size, or a model the registry does not decode, fails t.
func ModelWords(t testing.TB, registry sunspec.DecoderRegistry, id, length uint16, values map[string][]uint16) []uint16 {
	t.Helper()
	words := make([]uint16, int(length)+2)
	words[0], words[1] = id, length
	for _, point := range pointPlacements(t, registry, id, length) {
		value, exists := values[point.name]
		if !exists {
			continue
		}
		if len(value) != point.size {
			t.Fatalf("point %s words=%d want=%d", point.name, len(value), point.size)
		}
		copy(words[point.offset:point.offset+point.size], value)
	}
	model := decode(t, registry, words)
	for _, fact := range model.Facts() {
		if value, exists := values[fact.PointName]; exists && !slices.Equal(fact.Value.RawWords(), value) {
			t.Fatalf("model %d/%d point %s decodes words %v, want %v", id, length, fact.PointName, fact.Value.RawWords(), value)
		}
	}
	return words
}

// placement is the catalog position of one decoded point repetition.
type placement struct {
	name         string
	offset, size int
}

// placementKey identifies one catalog layout.
type placementKey struct {
	revision   sunspec.SchemaRevision
	id, length uint16
}

var (
	placementsMu    sync.Mutex
	placementsCache = map[placementKey][]placement{}
)

// pointPlacements returns the catalog position of every point repetition of
// id/length in registry, computed once per layout.
func pointPlacements(t testing.TB, registry sunspec.DecoderRegistry, id, length uint16) []placement {
	t.Helper()
	key := placementKey{registry.SchemaRevision(), id, length}
	placementsMu.Lock()
	cached, ok := placementsCache[key]
	placementsMu.Unlock()
	if ok {
		return cached
	}
	placements := probePlacements(t, registry, id, length)
	placementsMu.Lock()
	placementsCache[key] = placements
	placementsMu.Unlock()
	return placements
}

// probePlacements recovers the catalog offset of every point of id/length by
// decoding probe words. Each payload word first holds its own offset, so a
// point's raw words name its offset directly. A model whose geometry rejects
// that probe, such as a repeating model whose count word must match its
// length, is probed with every payload word set to one value from 0 to 255
// until the geometry accepts it; the offsets then follow from the
// catalog-ordered point sizes, which must cover the payload exactly.
func probePlacements(t testing.TB, registry sunspec.DecoderRegistry, id, length uint16) []placement {
	t.Helper()
	probe := make([]uint16, int(length)+2)
	probe[0], probe[1] = id, length
	for index := 2; index < len(probe); index++ {
		probe[index] = uint16(index)
	}
	if model := decode(t, registry, probe); model.GeometryValid() {
		var placements []placement
		for _, fact := range model.Facts() {
			raw := fact.Value.RawWords()
			for index, word := range raw {
				if int(word) != int(raw[0])+index {
					t.Fatalf("model %d/%d point %s probe words %v are not contiguous", id, length, fact.PointName, raw)
				}
			}
			if len(raw) > 0 {
				placements = append(placements, placement{name: fact.PointName, offset: int(raw[0]), size: len(raw)})
			}
		}
		return placements
	}
	for fill := uint16(0); fill <= 0xff; fill++ {
		for index := 2; index < len(probe); index++ {
			probe[index] = fill
		}
		model := decode(t, registry, probe)
		if !model.GeometryValid() {
			continue
		}
		var placements []placement
		cursor := 2
		for _, fact := range model.Facts() {
			size := len(fact.Value.RawWords())
			placements = append(placements, placement{name: fact.PointName, offset: cursor, size: size})
			cursor += size
		}
		if cursor != len(probe) {
			t.Fatalf("model %d/%d points cover %d of %d words", id, length, cursor, len(probe))
		}
		return placements
	}
	t.Fatalf("model %d/%d has no geometry-valid probe", id, length)
	return nil
}

func decode(t testing.TB, registry sunspec.DecoderRegistry, words []uint16) sunspec.DecodedModel {
	t.Helper()
	model, err := registry.DecodeOccurrence(Occurrence(t, registry, words, 1))
	if err != nil {
		t.Fatalf("model %d/%d: %v", words[0], words[1], err)
	}
	return model
}

// RawWords returns the chain image of occurrences: the signature, the words of
// every occurrence in order and the end marker.
func RawWords(occurrences []sunspec.Occurrence) []uint16 {
	raw := []uint16{SignatureFirst, SignatureSecond}
	for _, occurrence := range occurrences {
		raw = append(raw, occurrence.Words()...)
	}
	return append(raw, sunspec.EndModelID, 0)
}

// Snapshot returns the terminal snapshot of a chain walk over the words of
// occurrences under every decoder key of registry.
func Snapshot(t testing.TB, registry sunspec.DecoderRegistry, occurrences ...sunspec.Occurrence) sunspec.ChainSnapshot {
	t.Helper()
	return Walk(t, registry.DecoderKeys(), nil, occurrences...)
}

// SnapshotWithKeys returns the terminal snapshot of a chain walk over the
// words of occurrences under decoderKeys.
func SnapshotWithKeys(t testing.TB, decoderKeys []sunspec.DecoderKey, occurrences ...sunspec.Occurrence) sunspec.ChainSnapshot {
	t.Helper()
	return Walk(t, decoderKeys, nil, occurrences...)
}

// Walk plans a chain at Base under decoderKeys and admits one synthetic
// logical view per planned request over RawWords(occurrences) until the chain
// is terminal. Before each view is validated, edit, when not nil, may change
// its record; step counts the views from zero. It returns the terminal
// snapshot and fails t if any admission fails.
func Walk(t testing.TB, decoderKeys []sunspec.DecoderKey, edit func(step int, record *modbusreg.LogicalViewRecord), occurrences ...sunspec.Occurrence) sunspec.ChainSnapshot {
	t.Helper()
	blocks := make([][]uint16, len(occurrences))
	for index, occurrence := range occurrences {
		blocks[index] = occurrence.Words()
	}
	return WalkWords(t, sunspec.ChainPlanSpec{DecoderKeys: decoderKeys}, edit, blocks...)
}

// WalkWords walks a chain as Walk does over the model blocks given as words,
// under spec. An empty schema revision becomes Revision, empty base
// candidates become Base, and zero limits become the exact size of the chain
// image and the number of blocks.
func WalkWords(t testing.TB, spec sunspec.ChainPlanSpec, edit func(step int, record *modbusreg.LogicalViewRecord), blocks ...[]uint16) sunspec.ChainSnapshot {
	t.Helper()
	raw := []uint16{SignatureFirst, SignatureSecond}
	for _, block := range blocks {
		raw = append(raw, block...)
	}
	raw = append(raw, sunspec.EndModelID, 0)
	if spec.SchemaRevision == "" {
		spec.SchemaRevision = Revision
	}
	if len(spec.BaseCandidates) == 0 {
		spec.BaseCandidates = []uint16{Base}
	}
	if len(spec.BaseCandidates) != 1 {
		t.Fatalf("fixture walk needs exactly one base candidate, got %d", len(spec.BaseCandidates))
	}
	if spec.Limits == (sunspec.ChainLimits{}) {
		spec.Limits = sunspec.ChainLimits{MaxTotalWords: uint32(len(raw)), MaxOccurrences: uint32(max(len(blocks), 1))}
	}
	plan, err := sunspec.NewChainPlan(spec)
	if err != nil {
		t.Fatal(err)
	}
	chain := sunspec.NewChain(plan)
	logicalViewID := FirstLogicalViewID
	for step := 0; step < len(raw); step++ {
		requests := chain.NextRequests()
		if len(requests) != 1 {
			t.Fatalf("pending requests=%d", len(requests))
		}
		request := requests[0]
		start := int(request.Address()) - int(spec.BaseCandidates[0])
		end := start + int(request.WordCount())
		if start < 0 || end > len(raw) {
			t.Fatalf("request [%d,%d) exceeds raw words %d", start, end, len(raw))
		}
		record := Record(request, logicalViewID, raw[start:end], "fixture")
		if edit != nil {
			edit(step, &record)
		}
		view, err := modbusreg.NewLogicalViewSnapshot(record)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := chain.AdmitReplay(request, view)
		if err != nil {
			t.Fatal(err)
		}
		logicalViewID++
		if snapshot.RawWordCount() != 0 {
			return snapshot
		}
	}
	t.Fatal("SunSpec chain did not reach its terminal snapshot")
	return sunspec.ChainSnapshot{}
}

// Record returns the synthetic FC03 TCP logical-view record that answers
// request with words: one physical read equal to the logical read, with
// response and physical-request identifiers derived from id.
func Record(request sunspec.ReadRequest, id uint64, words []uint16, endpoint string) modbusreg.LogicalViewRecord {
	return modbusreg.LogicalViewRecord{
		LogicalViewID: id, WireResponseID: id + 100, PhysicalRequestID: id + 200,
		Endpoint: endpoint, ConnectionID: 4, Transport: modbusreg.TransportTCP, TransportGeneration: 5, UnitID: 1,
		RequestedFunction: modbusreg.FunctionReadHoldingRegisters, ReceivedFunction: modbusreg.FunctionReadHoldingRegisters, Table: modbusreg.HoldingRegisters,
		PhysicalOffset: request.Address(), PhysicalWordCount: request.WordCount(), AuthorizationScope: "read",
		PollGeneration: 6, DeadlineIdentity: 7, LogicalOffset: request.Address(), LogicalWordCount: request.WordCount(),
		SliceOffset: 0, SliceWordCount: request.WordCount(), Words: append([]uint16(nil), words...), WireResponseBytes: []byte{byte(id)},
	}
}

// View returns the validated logical view of Record(request, id, words,
// endpoint).
func View(t testing.TB, request sunspec.ReadRequest, id uint64, words []uint16, endpoint string) modbusreg.LogicalViewSnapshot {
	t.Helper()
	view, err := modbusreg.NewLogicalViewSnapshot(Record(request, id, words, endpoint))
	if err != nil {
		t.Fatal(err)
	}
	return view
}

// AdmitNext answers the first pending request of chain with words in a view
// identified by *id, increments *id and returns the admission result.
func AdmitNext(t testing.TB, chain *sunspec.Chain, id *uint64, words []uint16) (sunspec.ChainSnapshot, error) {
	t.Helper()
	request := chain.NextRequests()[0]
	view := View(t, request, *id, words, "fixture")
	*id++
	return chain.AdmitReplay(request, view)
}

// Records returns copies of the source-view records of snapshot in admission
// order.
func Records(snapshot sunspec.ChainSnapshot) []modbusreg.LogicalViewRecord {
	views := snapshot.SourceViews()
	records := make([]modbusreg.LogicalViewRecord, len(views))
	for index, view := range views {
		records[index] = view.Record()
	}
	return records
}

// Assemble returns sunspec.NewChainSnapshot of raw, occurrences and
// the validated views of records, failing t on any error. It assembles
// snapshots that no chain walk produces, such as an inconsistent chain image.
func Assemble(t testing.TB, raw []uint16, occurrences []sunspec.Occurrence, records []modbusreg.LogicalViewRecord) sunspec.ChainSnapshot {
	t.Helper()
	snapshot, err := assemble(raw, occurrences, records)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

// WithRaw returns Assemble of the raw words edit returns for a copy of the
// raw words of snapshot, with the occurrences and source views of snapshot.
// A nil edit keeps the raw words.
func WithRaw(t testing.TB, snapshot sunspec.ChainSnapshot, edit func(raw []uint16) []uint16) sunspec.ChainSnapshot {
	t.Helper()
	raw := snapshot.RawWords()
	if edit != nil {
		raw = edit(raw)
	}
	return Assemble(t, raw, snapshot.Occurrences(), Records(snapshot))
}

// WithRecords returns Assemble of the raw words and occurrences of snapshot
// with the source-view records edit returns for copies of the records of
// snapshot. A nil edit keeps the records.
func WithRecords(t testing.TB, snapshot sunspec.ChainSnapshot, edit func(records []modbusreg.LogicalViewRecord) []modbusreg.LogicalViewRecord) sunspec.ChainSnapshot {
	t.Helper()
	records := Records(snapshot)
	if edit != nil {
		records = edit(records)
	}
	return Assemble(t, snapshot.RawWords(), snapshot.Occurrences(), records)
}

// KeepSourceViews returns Assemble of the raw words and occurrences of
// snapshot with only its first count source views.
func KeepSourceViews(t testing.TB, snapshot sunspec.ChainSnapshot, count int) sunspec.ChainSnapshot {
	t.Helper()
	records := Records(snapshot)
	if count < 0 || count > len(records) {
		t.Fatalf("keep %d of %d source views", count, len(records))
	}
	return Assemble(t, snapshot.RawWords(), snapshot.Occurrences(), records[:count])
}

// ShiftSampleIdentity returns Assemble of snapshot with poll added to the
// poll generation and deadline added to the deadline identity of every source
// view.
func ShiftSampleIdentity(t testing.TB, snapshot sunspec.ChainSnapshot, poll, deadline uint64) sunspec.ChainSnapshot {
	t.Helper()
	records := Records(snapshot)
	for index := range records {
		records[index].PollGeneration += poll
		records[index].DeadlineIdentity += deadline
	}
	return Assemble(t, snapshot.RawWords(), snapshot.Occurrences(), records)
}

// AssembleError returns the error sunspec.NewChainSnapshot reports
// for raw, occurrences and records, or the error of the first record that is
// not a valid logical view.
func AssembleError(raw []uint16, occurrences []sunspec.Occurrence, records []modbusreg.LogicalViewRecord) error {
	_, err := assemble(raw, occurrences, records)
	return err
}

func assemble(raw []uint16, occurrences []sunspec.Occurrence, records []modbusreg.LogicalViewRecord) (sunspec.ChainSnapshot, error) {
	views := make([]modbusreg.LogicalViewSnapshot, len(records))
	for index, record := range records {
		view, err := modbusreg.NewLogicalViewSnapshot(record)
		if err != nil {
			return sunspec.ChainSnapshot{}, err
		}
		views[index] = view
	}
	return sunspec.NewChainSnapshot(raw, occurrences, views)
}
