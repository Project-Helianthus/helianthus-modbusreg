package modbusreg

import (
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func chainPlan(t *testing.T, bases []uint16) SunSpecChainPlan {
	t.Helper()
	p, err := NewSunSpecChainPlan(SunSpecChainPlanSpec{SchemaRevision: "sunspec.r1@1", BaseCandidates: bases, Limits: SunSpecChainLimits{MaxTotalWords: 64, MaxOccurrences: 8}, DecoderKeys: []SunSpecDecoderKey{{ModelID: 103, ModelLength: 2, SchemaRevision: "sunspec.r1@1"}, {ModelID: 113, ModelLength: 1, SchemaRevision: "sunspec.r1@1"}}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func chainView(t *testing.T, r SunSpecReadRequest, id uint64, words []uint16, endpoint string) LogicalViewSnapshot {
	t.Helper()
	v, err := NewLogicalViewSnapshot(LogicalViewRecord{LogicalViewID: id, WireResponseID: id + 100, PhysicalRequestID: id + 200, Endpoint: endpoint, ConnectionID: 4, Transport: TransportTCP, TransportGeneration: 5, UnitID: 1, RequestedFunction: FunctionReadHoldingRegisters, ReceivedFunction: FunctionReadHoldingRegisters, Table: HoldingRegisters, PhysicalOffset: r.Address(), PhysicalWordCount: r.WordCount(), AuthorizationScope: "read", PollGeneration: 6, DeadlineIdentity: 7, LogicalOffset: r.Address(), LogicalWordCount: r.WordCount(), SliceOffset: 0, SliceWordCount: r.WordCount(), Words: words, WireResponseBytes: []byte{byte(id)}})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func chainSlicedView(t *testing.T, r SunSpecReadRequest, id uint64, words []uint16, sliceOffset uint16) LogicalViewSnapshot {
	t.Helper()
	physicalOffset := r.Address() - sliceOffset
	physicalWords := r.WordCount() + sliceOffset + 1
	v, err := NewLogicalViewSnapshot(LogicalViewRecord{LogicalViewID: id, WireResponseID: id + 100, PhysicalRequestID: id + 200, Endpoint: "fixture", ConnectionID: 4, Transport: TransportTCP, TransportGeneration: 5, UnitID: 1, RequestedFunction: FunctionReadHoldingRegisters, ReceivedFunction: FunctionReadHoldingRegisters, Table: HoldingRegisters, PhysicalOffset: physicalOffset, PhysicalWordCount: physicalWords, AuthorizationScope: "read", PollGeneration: 6, DeadlineIdentity: 7, LogicalOffset: r.Address(), LogicalWordCount: r.WordCount(), SliceOffset: sliceOffset, SliceWordCount: r.WordCount(), Words: words, WireResponseBytes: []byte{byte(id)}})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func admitNext(t *testing.T, c *SunSpecChain, id *uint64, words []uint16) (SunSpecChainSnapshot, error) {
	t.Helper()
	r := c.NextRequests()[0]
	v := chainView(t, r, *id, words, "fixture")
	*id++
	return c.AdmitReplay(r, v)
}

func TestSunSpecChainRejectsCrossInstanceReplay(t *testing.T) {
	plan := chainPlan(t, []uint16{40000})
	c1 := NewSunSpecChain(plan)
	c2 := NewSunSpecChain(plan)
	r := c1.NextRequests()[0]
	v := chainView(t, r, 1, []uint16{0x5375, 0x6e53}, "fixture")
	if _, err := c1.AdmitReplay(r, v); err != nil {
		t.Fatalf("first chain replay admission: %v", err)
	}
	if _, err := c2.AdmitReplay(r, v); err == nil {
		t.Fatal("stale request/view from a different chain instance was admitted")
	}
}

func TestSunSpecChainRetainsOrderedDuplicatesUnknownAndWrongLength(t *testing.T) {
	c := NewSunSpecChain(chainPlan(t, []uint16{40000}))
	id := uint64(1)
	words := orderedChainWords(t)
	for len(words) != 0 {
		r := c.NextRequests()[0]
		if len(words) < int(r.WordCount()) {
			t.Fatalf("fixture ended before %v request", r.Purpose())
		}
		chunk := words[:r.WordCount()]
		words = words[r.WordCount():]
		if _, err := admitNext(t, c, &id, chunk); err != nil {
			t.Fatalf("admit %v: %v", chunk, err)
		}
	}
	s, err := admitNext(t, c, &id, []uint16{0xffff, 0})
	if err != nil {
		t.Fatal(err)
	}
	o := s.Occurrences()
	if len(o) != 4 {
		t.Fatalf("occurrences=%d", len(o))
	}
	want := []uint16{1, 103, 65000, 103}
	for i, id := range want {
		if o[i].ModelID() != id || o[i].Ordinal != uint32(i+1) {
			t.Fatalf("occurrence %d=%#v", i, o[i])
		}
	}
	if o[1].Disposition != SunSpecChainDispositionUnsupportedLength || o[2].Disposition != SunSpecChainDispositionUnknownModel || o[3].Disposition != SunSpecChainDispositionAdmitted {
		t.Fatalf("dispositions=%v/%v/%v", o[1].Disposition, o[2].Disposition, o[3].Disposition)
	}
	if _, ok := o[1].DecoderKey(); ok {
		t.Fatal("wrong length has decoder key")
	}
	if k, ok := o[3].DecoderKey(); !ok || k.ModelLength != 2 {
		t.Fatal("exact shape key absent")
	}
	if got := s.ByModelID(103); len(got) != 2 || got[0].Ordinal != 2 || got[1].Ordinal != 4 {
		t.Fatalf("duplicates lost: %#v", got)
	}
}

func orderedChainWords(t *testing.T) []uint16 {
	t.Helper()
	b, err := os.ReadFile("testdata/sunspec/chain/ordered-chain.words")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(b))
	words := make([]uint16, 0, len(fields))
	for _, field := range fields {
		word, err := strconv.ParseUint(field, 16, 16)
		if err != nil {
			t.Fatalf("parse fixture word %q: %v", field, err)
		}
		words = append(words, uint16(word))
	}
	return words
}
func TestSunSpecChainSourceSpansReconstructAndCopiesAreImmutable(t *testing.T) {
	c := NewSunSpecChain(chainPlan(t, []uint16{40000}))
	id := uint64(1)
	for _, w := range [][]uint16{{0x5375, 0x6e53}, {65000, 2}, {3, 4}} {
		if _, e := admitNext(t, c, &id, w); e != nil {
			t.Fatal(e)
		}
	}
	s, e := admitNext(t, c, &id, []uint16{0xffff, 0})
	if e != nil {
		t.Fatal(e)
	}
	o := s.Occurrences()[0]
	if o.HeaderOffset != 40002 || o.PayloadOffset != 40004 || len(o.SourceSpans()) != 2 {
		t.Fatalf("bad spans %#v", o)
	}
	if got := o.Words(); len(got) != 4 || got[0] != 65000 || got[3] != 4 {
		t.Fatalf("raw=%v", got)
	}
	raw := s.RawWords()
	raw[0] = 0
	words := o.Words()
	words[0] = 0
	spans := o.SourceSpans()
	spans[0].PDUOffset = 0
	out := s.Occurrences()
	out[0].words[0] = 0
	method := reflect.ValueOf(s).MethodByName("SourceViews")
	if !method.IsValid() {
		t.Fatal("completed chain snapshot does not expose source views")
	}
	views, ok := method.Call(nil)[0].Interface().([]LogicalViewSnapshot)
	if !ok {
		t.Fatal("source views have the wrong type")
	}
	if len(views) != 4 {
		t.Fatalf("source views=%d", len(views))
	}
	for _, span := range s.Occurrences()[0].SourceSpans() {
		found := false
		for _, view := range views {
			if view.Record().LogicalViewID == span.LogicalViewID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("span %#v is detached from source views", span)
		}
	}
	if got := views[0].Record(); got.Words[0] != 0x5375 || got.WireResponseBytes[0] != 1 {
		t.Fatalf("signature source cannot reconstruct bytes: %#v", got)
	}
	views[0].record.Words[0] = 0
	views[0].record.WireResponseBytes[0] = 0
	stored := method.Call(nil)[0].Interface().([]LogicalViewSnapshot)
	if s.RawWords()[0] != 0x5375 || s.Occurrences()[0].Words()[0] != 65000 || s.Occurrences()[0].SourceSpans()[0].PDUOffset != 0 || stored[0].Record().WireResponseBytes[0] != 1 {
		t.Fatal("returned copy mutated snapshot")
	}
}

func TestSunSpecChainSourceSpansUseSourcePDUCoordinates(t *testing.T) {
	c := NewSunSpecChain(chainPlan(t, []uint16{40000}))
	id := uint64(1)
	for _, words := range [][]uint16{{0x5375, 0x6e53}, {65000, 2}, {3, 4}, {0xffff, 0}} {
		r := c.NextRequests()[0]
		snapshot, err := c.AdmitReplay(r, chainSlicedView(t, r, id, words, 3))
		if err != nil {
			t.Fatal(err)
		}
		id++
		if len(snapshot.Occurrences()) == 0 {
			continue
		}

		occurrence := snapshot.Occurrences()[0]
		views := snapshot.SourceViews()
		var reconstructed []uint16
		for _, span := range occurrence.SourceSpans() {
			var source *LogicalViewRecord
			for _, view := range views {
				record := view.Record()
				if record.LogicalViewID == span.LogicalViewID {
					source = &record
					break
				}
			}
			if source == nil {
				t.Fatalf("source view %d is missing", span.LogicalViewID)
			}
			if span.PDUOffset != source.SliceOffset || span.WordCount != source.SliceWordCount || uint32(span.PDUOffset)+uint32(span.WordCount) > uint32(source.PhysicalWordCount) {
				t.Fatalf("span %#v does not identify source PDU slice %#v", span, *source)
			}
			reconstructed = append(reconstructed, source.Words...)
		}
		if !reflect.DeepEqual(reconstructed, occurrence.Words()) {
			t.Fatalf("reconstructed=%v occurrence=%v", reconstructed, occurrence.Words())
		}
	}
}
func TestSunSpecChainAmbiguityTerminallyPoisonsBuilder(t *testing.T) {
	c := NewSunSpecChain(chainPlan(t, []uint16{40000, 41000}))
	rs := c.NextRequests()
	if _, e := c.AdmitReplay(rs[0], chainView(t, rs[0], 1, []uint16{0x5375, 0x6e53}, "fixture")); e != nil {
		t.Fatal(e)
	}
	if _, e := c.AdmitReplay(rs[1], chainView(t, rs[1], 2, []uint16{0x5375, 0x6e53}, "fixture")); e == nil {
		t.Fatal("ambiguous bases admitted")
	}
	if got := c.NextRequests(); len(got) != 0 {
		t.Fatalf("ambiguous builder retained retryable requests: %#v", got)
	}
	if _, e := c.AdmitReplay(rs[1], chainView(t, rs[1], 3, []uint16{0, 0}, "fixture")); e == nil {
		t.Fatal("ambiguous builder recovered after resubmission")
	}
}

func TestSunSpecChainRejectsProvenanceReplayAndTerminalErrors(t *testing.T) {
	c := NewSunSpecChain(chainPlan(t, []uint16{40000}))
	r := c.NextRequests()[0]
	v := chainView(t, r, 1, []uint16{0x5375, 0x6e53}, "fixture")
	if _, e := c.AdmitReplay(r, v); e != nil {
		t.Fatal(e)
	}
	if _, e := c.AdmitReplay(r, v); e == nil {
		t.Fatal("replay admitted")
	}
	h := c.NextRequests()[0]
	if _, e := c.AdmitReplay(h, chainView(t, h, 2, []uint16{0xffff, 1}, "fixture")); e == nil {
		t.Fatal("nonzero terminal admitted")
	}
}
func TestSunSpecChainRejectsDetachedRangeAndMixedProvenance(t *testing.T) {
	c := NewSunSpecChain(chainPlan(t, []uint16{40000}))
	r := c.NextRequests()[0]
	if _, e := c.AdmitReplay(SunSpecReadRequest{}, chainView(t, r, 1, []uint16{0x5375, 0x6e53}, "fixture")); e == nil {
		t.Fatal("detached request admitted")
	}
	if _, e := c.AdmitReplay(r, chainView(t, r, 1, []uint16{0, 0}, "fixture")); e == nil {
		t.Fatal("bad signature admitted")
	}
	c = NewSunSpecChain(chainPlan(t, []uint16{40000}))
	r = c.NextRequests()[0]
	if _, e := c.AdmitReplay(r, chainView(t, r, 1, []uint16{0x5375, 0x6e53}, "fixture")); e != nil {
		t.Fatal(e)
	}
	h := c.NextRequests()[0]
	if _, e := c.AdmitReplay(h, chainView(t, h, 2, []uint16{0xffff, 0}, "other")); e == nil {
		t.Fatal("mixed provenance admitted")
	}
}

func TestSunSpecChainRejectsZeroLengthAndBoundedOverrun(t *testing.T) {
	c := NewSunSpecChain(chainPlan(t, []uint16{40000}))
	id := uint64(1)
	if _, err := admitNext(t, c, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	if _, err := admitNext(t, c, &id, []uint16{7, 0}); err == nil {
		t.Fatal("zero non-end length admitted")
	}
	p, err := NewSunSpecChainPlan(SunSpecChainPlanSpec{SchemaRevision: "sunspec.r1@1", BaseCandidates: []uint16{65500}, Limits: SunSpecChainLimits{MaxTotalWords: 512, MaxOccurrences: 2}})
	if err != nil {
		t.Fatal(err)
	}
	c = NewSunSpecChain(p)
	id = 20
	if _, err := admitNext(t, c, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	if _, err := admitNext(t, c, &id, []uint16{7, 125}); err == nil {
		t.Fatal("address-space overrun admitted")
	}
}

func TestSunSpecChainRejectsUnrepresentableSuccessorRequests(t *testing.T) {
	p, err := NewSunSpecChainPlan(SunSpecChainPlanSpec{SchemaRevision: "sunspec.r1@1", BaseCandidates: []uint16{65534}, Limits: SunSpecChainLimits{MaxTotalWords: 8, MaxOccurrences: 1}})
	if err != nil {
		t.Fatal(err)
	}
	c := NewSunSpecChain(p)
	id := uint64(1)
	if _, err := admitNext(t, c, &id, []uint16{0x5375, 0x6e53}); err == nil {
		t.Fatal("signature at 65534 scheduled an unrepresentable header")
	}
	p, err = NewSunSpecChainPlan(SunSpecChainPlanSpec{SchemaRevision: "sunspec.r1@1", BaseCandidates: []uint16{65531}, Limits: SunSpecChainLimits{MaxTotalWords: 8, MaxOccurrences: 1}})
	if err != nil {
		t.Fatal(err)
	}
	c = NewSunSpecChain(p)
	id = 10
	if _, err := admitNext(t, c, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	if _, err := admitNext(t, c, &id, []uint16{1, 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := admitNext(t, c, &id, []uint16{9}); err == nil {
		t.Fatal("payload ending at 65536 scheduled an unrepresentable terminal header")
	}
}
