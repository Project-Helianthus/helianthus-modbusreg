package modbusreg

import "testing"

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
func admitNext(t *testing.T, c *SunSpecChain, id *uint64, words []uint16) (SunSpecChainSnapshot, error) {
	t.Helper()
	r := c.NextRequests()[0]
	v := chainView(t, r, *id, words, "fixture")
	*id++
	return c.Admit(r, v)
}

func TestSunSpecChainRetainsOrderedDuplicatesUnknownAndWrongLength(t *testing.T) {
	c := NewSunSpecChain(chainPlan(t, []uint16{40000}))
	id := uint64(1)
	for _, words := range [][]uint16{{0x5375, 0x6e53}, {1, 1}, {9}, {103, 1}, {8}, {65000, 2}, {3, 4}, {103, 2}, {5, 6}, {113, 1}, {7}} {
		if _, err := admitNext(t, c, &id, words); err != nil {
			t.Fatalf("admit %v: %v", words, err)
		}
	}
	s, err := admitNext(t, c, &id, []uint16{0xffff, 0})
	if err != nil {
		t.Fatal(err)
	}
	o := s.Occurrences()
	if len(o) != 5 {
		t.Fatalf("occurrences=%d", len(o))
	}
	want := []uint16{1, 103, 65000, 103, 113}
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
	if s.RawWords()[0] != 0x5375 || s.Occurrences()[0].Words()[0] != 65000 || s.Occurrences()[0].SourceSpans()[0].PDUOffset != 40002 {
		t.Fatal("returned copy mutated snapshot")
	}
}
func TestSunSpecChainRejectsAmbiguityAndProvenanceReplayAndTerminalErrors(t *testing.T) {
	c := NewSunSpecChain(chainPlan(t, []uint16{40000, 41000}))
	rs := c.NextRequests()
	if _, e := c.Admit(rs[0], chainView(t, rs[0], 1, []uint16{0x5375, 0x6e53}, "fixture")); e != nil {
		t.Fatal(e)
	}
	if _, e := c.Admit(rs[1], chainView(t, rs[1], 2, []uint16{0x5375, 0x6e53}, "fixture")); e == nil {
		t.Fatal("ambiguous bases admitted")
	}
	c = NewSunSpecChain(chainPlan(t, []uint16{40000}))
	r := c.NextRequests()[0]
	v := chainView(t, r, 1, []uint16{0x5375, 0x6e53}, "fixture")
	if _, e := c.Admit(r, v); e != nil {
		t.Fatal(e)
	}
	if _, e := c.Admit(r, v); e == nil {
		t.Fatal("replay admitted")
	}
	h := c.NextRequests()[0]
	if _, e := c.Admit(h, chainView(t, h, 2, []uint16{0xffff, 1}, "fixture")); e == nil {
		t.Fatal("nonzero terminal admitted")
	}
}
func TestSunSpecChainRejectsDetachedRangeAndMixedProvenance(t *testing.T) {
	c := NewSunSpecChain(chainPlan(t, []uint16{40000}))
	r := c.NextRequests()[0]
	if _, e := c.Admit(SunSpecReadRequest{}, chainView(t, r, 1, []uint16{0x5375, 0x6e53}, "fixture")); e == nil {
		t.Fatal("detached request admitted")
	}
	if _, e := c.Admit(r, chainView(t, r, 1, []uint16{0, 0}, "fixture")); e == nil {
		t.Fatal("bad signature admitted")
	}
	c = NewSunSpecChain(chainPlan(t, []uint16{40000}))
	r = c.NextRequests()[0]
	if _, e := c.Admit(r, chainView(t, r, 1, []uint16{0x5375, 0x6e53}, "fixture")); e != nil {
		t.Fatal(e)
	}
	h := c.NextRequests()[0]
	if _, e := c.Admit(h, chainView(t, h, 2, []uint16{0xffff, 0}, "other")); e == nil {
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
