package sunspec

import (
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func chainPlan(t *testing.T, bases []uint16) ChainPlan {
	t.Helper()
	p, err := NewChainPlan(ChainPlanSpec{SchemaRevision: "sunspec.r1@1", BaseCandidates: bases, Limits: ChainLimits{MaxTotalWords: 64, MaxOccurrences: 8}, DecoderKeys: []DecoderKey{{ModelID: 103, ModelLength: 2, SchemaRevision: "sunspec.r1@1"}, {ModelID: 113, ModelLength: 1, SchemaRevision: "sunspec.r1@1"}}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func chainView(t *testing.T, r ReadRequest, id uint64, words []uint16, endpoint string) modbusreg.LogicalViewSnapshot {
	t.Helper()
	v, err := modbusreg.NewLogicalViewSnapshot(modbusreg.LogicalViewRecord{LogicalViewID: id, WireResponseID: id + 100, PhysicalRequestID: id + 200, Endpoint: endpoint, ConnectionID: 4, Transport: modbusreg.TransportTCP, TransportGeneration: 5, UnitID: 1, RequestedFunction: modbusreg.FunctionReadHoldingRegisters, ReceivedFunction: modbusreg.FunctionReadHoldingRegisters, Table: modbusreg.HoldingRegisters, PhysicalOffset: r.Address(), PhysicalWordCount: r.WordCount(), AuthorizationScope: "read", PollGeneration: 6, DeadlineIdentity: 7, LogicalOffset: r.Address(), LogicalWordCount: r.WordCount(), SliceOffset: 0, SliceWordCount: r.WordCount(), Words: words, WireResponseBytes: []byte{byte(id)}})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func chainSlicedView(t *testing.T, r ReadRequest, id uint64, words []uint16, sliceOffset uint16) modbusreg.LogicalViewSnapshot {
	t.Helper()
	physicalOffset := r.Address() - sliceOffset
	physicalWords := r.WordCount() + sliceOffset + 1
	v, err := modbusreg.NewLogicalViewSnapshot(modbusreg.LogicalViewRecord{LogicalViewID: id, WireResponseID: id + 100, PhysicalRequestID: id + 200, Endpoint: "fixture", ConnectionID: 4, Transport: modbusreg.TransportTCP, TransportGeneration: 5, UnitID: 1, RequestedFunction: modbusreg.FunctionReadHoldingRegisters, ReceivedFunction: modbusreg.FunctionReadHoldingRegisters, Table: modbusreg.HoldingRegisters, PhysicalOffset: physicalOffset, PhysicalWordCount: physicalWords, AuthorizationScope: "read", PollGeneration: 6, DeadlineIdentity: 7, LogicalOffset: r.Address(), LogicalWordCount: r.WordCount(), SliceOffset: sliceOffset, SliceWordCount: r.WordCount(), Words: words, WireResponseBytes: []byte{byte(id)}})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func admitNext(t *testing.T, c *Chain, id *uint64, words []uint16) (ChainSnapshot, error) {
	t.Helper()
	r := c.NextRequests()[0]
	v := chainView(t, r, *id, words, "fixture")
	*id++
	return c.AdmitReplay(r, v)
}

func TestSunSpecChainRejectsCrossInstanceReplay(t *testing.T) {
	plan := chainPlan(t, []uint16{40000})
	c1 := NewChain(plan)
	c2 := NewChain(plan)
	r := c1.NextRequests()[0]
	v := chainView(t, r, 1, []uint16{0x5375, 0x6e53}, "fixture")
	if _, err := c1.AdmitReplay(r, v); err != nil {
		t.Fatalf("first chain replay admission: %v", err)
	}
	if _, err := c2.AdmitReplay(r, v); err == nil {
		t.Fatal("stale request/view from a different chain instance was admitted")
	}
}

type sunSpecReadTrace struct {
	address uint16
	words   uint16
	purpose ReadPurpose
}

func runV2StructuralCandidateChain(t *testing.T, candidateIDs []uint16) (ChainSnapshot, []sunSpecReadTrace) {
	t.Helper()
	plan, err := NewChainPlan(ChainPlanSpec{
		SchemaRevision:         ModelsRevisionV2,
		BaseCandidates:         []uint16{40000},
		Limits:                 ChainLimits{MaxTotalWords: 256, MaxOccurrences: 4},
		StructuralCandidateIDs: candidateIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	id := uint64(1)
	trace := make([]sunSpecReadTrace, 0, 8)
	admit := func(words []uint16) (ChainSnapshot, error) {
		r := chain.NextRequests()[0]
		trace = append(trace, sunSpecReadTrace{r.Address(), r.WordCount(), r.Purpose()})
		snapshot, err := chain.AdmitReplay(r, chainView(t, r, id, words, "fixture"))
		id++
		return snapshot, err
	}
	if _, err := admit([]uint16{sunSpecSignatureFirst, sunSpecSignatureSecond}); err != nil {
		t.Fatal(err)
	}
	first707 := []uint16{707, 33, 0, 0, 0, 1, 2, 0, 0, 0, 1, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 1, 0, 1, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 1}
	second707 := append([]uint16(nil), first707...)
	stream := append([]uint16(nil), first707...)
	stream = append(stream, second707...)
	stream = append(stream, 708, 33)
	stream = append(stream, make([]uint16, 33)...)
	stream = append(stream, 709, 39)
	stream = append(stream, make([]uint16, 39)...)
	for len(stream) > 0 {
		r := chain.NextRequests()[0]
		if len(stream) < int(r.WordCount()) {
			t.Fatalf("fixture ended before request %#v", r)
		}
		chunk := stream[:r.WordCount()]
		stream = stream[r.WordCount():]
		if _, err := admit(chunk); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := admit([]uint16{EndModelID, 0})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, trace
}

func TestSunSpecV2StructuralCandidateDoesNotChangeChainBehaviorOrActivateModels(t *testing.T) {
	plain, plainTrace := runV2StructuralCandidateChain(t, nil)
	selected, selectedTrace := runV2StructuralCandidateChain(t, []uint16{707})
	if !reflect.DeepEqual(selectedTrace, plainTrace) {
		t.Fatalf("candidate changed read trace=%#v plain=%#v", selectedTrace, plainTrace)
	}
	if !reflect.DeepEqual(selected.RawWords(), plain.RawWords()) || len(selected.SourceViews()) != len(plain.SourceViews()) {
		t.Fatal("candidate changed retained raw acquisition evidence")
	}
	plainOccurrences, selectedOccurrences := plain.Occurrences(), selected.Occurrences()
	if len(selectedOccurrences) != 4 || len(plainOccurrences) != len(selectedOccurrences) {
		t.Fatalf("occurrences plain=%#v selected=%#v", plainOccurrences, selectedOccurrences)
	}
	for index, occurrence := range selectedOccurrences {
		if occurrence.Disposition != ChainDispositionUnknownModel {
			t.Fatalf("occurrence %d disposition=%q", index, occurrence.Disposition)
		}
		if _, ok := occurrence.DecoderKey(); ok {
			t.Fatalf("occurrence %d unexpectedly has decoder key", index)
		}
		if occurrence.WireKey.ModelID == 707 {
			if occurrence.structuralCandidate == nil {
				t.Fatalf("Model 707 occurrence %d lacks candidate", index)
			}
			if plainOccurrences[index].structuralCandidate != nil {
				t.Fatalf("non-opt-in Model 707 occurrence %d has candidate", index)
			}
		} else if occurrence.structuralCandidate != nil {
			t.Fatalf("non-707 model %d became candidate", occurrence.WireKey.ModelID)
		}
	}
	if selectedOccurrences[0].structuralCandidate == selectedOccurrences[1].structuralCandidate {
		t.Fatal("repeated candidates share sidecar storage")
	}
	copy := selected.Occurrences()
	if copy[0].structuralCandidate == selectedOccurrences[0].structuralCandidate {
		t.Fatal("candidate sidecar aliases snapshot storage")
	}
	copy[0].structuralCandidate.layout.entries[0].path.segments[0].Name = "mutated"
	again := selected.Occurrences()
	if again[0].structuralCandidate.layout.entries[0].path.segments[0].Name == "mutated" {
		t.Fatal("candidate layout aliases snapshot storage")
	}
}

func TestSunSpecChainRetainsOrderedDuplicatesUnknownAndWrongLength(t *testing.T) {
	c := NewChain(chainPlan(t, []uint16{40000}))
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
	if o[1].Disposition != ChainDispositionUnsupportedLength || o[2].Disposition != ChainDispositionUnknownModel || o[3].Disposition != ChainDispositionAdmitted {
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
	for _, occurrence := range o {
		if occurrence.SchemaRevision != "sunspec.r1@1" {
			t.Fatalf("occurrence %d lost schema revision: %#v", occurrence.Ordinal, occurrence)
		}
	}
}

func orderedChainWords(t *testing.T) []uint16 {
	t.Helper()
	b, err := os.ReadFile("../testdata/sunspec/chain/ordered-chain.words")
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
	c := NewChain(chainPlan(t, []uint16{40000}))
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
	views, ok := method.Call(nil)[0].Interface().([]modbusreg.LogicalViewSnapshot)
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
	stored := method.Call(nil)[0].Interface().([]modbusreg.LogicalViewSnapshot)
	if sharesViewStorage(views[0], stored[0]) || s.RawWords()[0] != 0x5375 || s.Occurrences()[0].Words()[0] != 65000 || s.Occurrences()[0].SourceSpans()[0].PDUOffset != 0 || stored[0].Record().WireResponseBytes[0] != 1 {
		t.Fatal("returned copy mutated snapshot")
	}
}

// sourceViewRecord returns the record of the source view with the given identifier by value,
// so the span checks below never dereference a pointer that an analyzer may judge nilable.
func sourceViewRecord(views []modbusreg.LogicalViewSnapshot, id uint64) (modbusreg.LogicalViewRecord, bool) {
	for _, view := range views {
		record := view.Record()
		if record.LogicalViewID == id {
			return record, true
		}
	}
	return modbusreg.LogicalViewRecord{}, false
}

func TestSunSpecChainSourceSpansUseSourcePDUCoordinates(t *testing.T) {
	c := NewChain(chainPlan(t, []uint16{40000}))
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
			source, found := sourceViewRecord(views, span.LogicalViewID)
			if !found {
				t.Fatalf("source view %d is missing", span.LogicalViewID)
			}
			if span.PDUOffset != source.SliceOffset || span.WordCount != source.SliceWordCount || uint32(span.PDUOffset)+uint32(span.WordCount) > uint32(source.PhysicalWordCount) {
				t.Fatalf("span %#v does not identify source PDU slice %#v", span, source)
			}
			reconstructed = append(reconstructed, source.Words...)
		}
		if !reflect.DeepEqual(reconstructed, occurrence.Words()) {
			t.Fatalf("reconstructed=%v occurrence=%v", reconstructed, occurrence.Words())
		}
	}
}
func TestSunSpecChainAmbiguityTerminallyPoisonsBuilder(t *testing.T) {
	c := NewChain(chainPlan(t, []uint16{40000, 41000}))
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
	c := NewChain(chainPlan(t, []uint16{40000}))
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
	c := NewChain(chainPlan(t, []uint16{40000}))
	r := c.NextRequests()[0]
	if _, e := c.AdmitReplay(ReadRequest{}, chainView(t, r, 1, []uint16{0x5375, 0x6e53}, "fixture")); e == nil {
		t.Fatal("detached request admitted")
	}
	if _, e := c.AdmitReplay(r, chainView(t, r, 1, []uint16{0, 0}, "fixture")); e == nil {
		t.Fatal("bad signature admitted")
	}
	c = NewChain(chainPlan(t, []uint16{40000}))
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
	c := NewChain(chainPlan(t, []uint16{40000}))
	id := uint64(1)
	if _, err := admitNext(t, c, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	if _, err := admitNext(t, c, &id, []uint16{7, 0}); err == nil {
		t.Fatal("zero non-end length admitted")
	}
	p, err := NewChainPlan(ChainPlanSpec{SchemaRevision: "sunspec.r1@1", BaseCandidates: []uint16{65500}, Limits: ChainLimits{MaxTotalWords: 512, MaxOccurrences: 2}})
	if err != nil {
		t.Fatal(err)
	}
	c = NewChain(p)
	id = 20
	if _, err := admitNext(t, c, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	if _, err := admitNext(t, c, &id, []uint16{7, 125}); err == nil {
		t.Fatal("address-space overrun admitted")
	}
}

func TestSunSpecChainStructuralErrorsTerminallyPoisonBuilder(t *testing.T) {
	tests := []struct {
		name      string
		base      uint16
		malformed []uint16
	}{
		{name: "zero length", base: 40000, malformed: []uint16{7, 0}},
		{name: "nonzero terminal", base: 40000, malformed: []uint16{0xffff, 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := NewChainPlan(ChainPlanSpec{SchemaRevision: "sunspec.r1@1", BaseCandidates: []uint16{tc.base}, Limits: ChainLimits{MaxTotalWords: 8, MaxOccurrences: 1}})
			if err != nil {
				t.Fatal(err)
			}
			chain := NewChain(plan)
			id := uint64(1)
			if _, err := admitNext(t, chain, &id, []uint16{0x5375, 0x6e53}); err != nil {
				t.Fatal(err)
			}
			request := chain.NextRequests()[0]
			if _, err := chain.AdmitReplay(request, chainView(t, request, id, tc.malformed, "fixture")); err == nil {
				t.Fatal("malformed structure was admitted")
			}
			if pending := chain.NextRequests(); len(pending) != 0 {
				t.Fatalf("failed chain retained requests: %#v", pending)
			}
			id++
			if _, err := chain.AdmitReplay(request, chainView(t, request, id, []uint16{0xffff, 0}, "fixture")); err == nil {
				t.Fatal("failed chain recovered after structural error")
			}
		})
	}

	plan, err := NewChainPlan(ChainPlanSpec{SchemaRevision: "sunspec.r1@1", BaseCandidates: []uint16{65531}, Limits: ChainLimits{MaxTotalWords: 8, MaxOccurrences: 1}})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(plan)
	id := uint64(20)
	if _, err := admitNext(t, chain, &id, []uint16{0x5375, 0x6e53}); err != nil {
		t.Fatal(err)
	}
	if _, err := admitNext(t, chain, &id, []uint16{1, 1}); err != nil {
		t.Fatal(err)
	}
	request := chain.NextRequests()[0]
	if _, err := chain.AdmitReplay(request, chainView(t, request, id, []uint16{9}, "fixture")); err == nil {
		t.Fatal("unrepresentable payload successor was admitted")
	}
	if pending := chain.NextRequests(); len(pending) != 0 {
		t.Fatalf("failed payload chain retained requests: %#v", pending)
	}
	id++
	if _, err := chain.AdmitReplay(request, chainView(t, request, id, []uint16{8}, "fixture")); err == nil {
		t.Fatal("failed payload chain recovered after structural error")
	}
}

func TestSunSpecChainSerializesConcurrentAdmissions(t *testing.T) {
	chain := NewChain(chainPlan(t, []uint16{40000, 41000}))
	requests := chain.NextRequests()
	start := make(chan struct{})
	errs := make(chan error, len(requests))
	var wait sync.WaitGroup
	for index, request := range requests {
		wait.Add(1)
		go func(index int, request ReadRequest) {
			defer wait.Done()
			<-start
			words := []uint16{0, 0}
			if index == 0 {
				words = []uint16{0x5375, 0x6e53}
			}
			_, err := chain.AdmitReplay(request, chainView(t, request, uint64(index+1), words, "fixture"))
			errs <- err
		}(index, request)
	}
	wait.Add(1)
	go func() {
		defer wait.Done()
		<-start
		for range 100 {
			_ = chain.NextRequests()
		}
	}()
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	pending := chain.NextRequests()
	if len(pending) != 1 || pending[0].Purpose() != ReadHeader || pending[0].Address() != 40002 {
		t.Fatalf("concurrent admission produced %#v", pending)
	}
}

func TestSunSpecChainRejectsContradictorySourceIdentities(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*modbusreg.LogicalViewRecord, modbusreg.LogicalViewRecord)
	}{
		{
			name: "wire response maps to another physical request",
			mutate: func(record *modbusreg.LogicalViewRecord, first modbusreg.LogicalViewRecord) {
				record.WireResponseID = first.WireResponseID
			},
		},
		{
			name: "physical request maps to another wire response",
			mutate: func(record *modbusreg.LogicalViewRecord, first modbusreg.LogicalViewRecord) {
				record.PhysicalRequestID = first.PhysicalRequestID
			},
		},
		{
			name: "wire response maps to contradictory exact bytes",
			mutate: func(record *modbusreg.LogicalViewRecord, first modbusreg.LogicalViewRecord) {
				record.WireResponseID = first.WireResponseID
				record.PhysicalRequestID = first.PhysicalRequestID
				record.WireResponseBytes = []byte{0xff}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chain := NewChain(chainPlan(t, []uint16{40000}))
			signatureRequest := chain.NextRequests()[0]
			signatureView := chainView(t, signatureRequest, 1, []uint16{0x5375, 0x6e53}, "fixture")
			if _, err := chain.AdmitReplay(signatureRequest, signatureView); err != nil {
				t.Fatal(err)
			}
			headerRequest := chain.NextRequests()[0]
			record := chainView(t, headerRequest, 2, []uint16{0xffff, 0}, "fixture").Record()
			tc.mutate(&record, signatureView.Record())
			contradictory, err := modbusreg.NewLogicalViewSnapshot(record)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := chain.AdmitReplay(headerRequest, contradictory); err == nil {
				t.Fatal("contradictory source identity completed the chain")
			}
			if pending := chain.NextRequests(); len(pending) != 0 {
				t.Fatalf("contradictory provenance did not poison chain: %#v", pending)
			}
		})
	}
}

func TestSunSpecChainRejectsUnrepresentableSuccessorRequests(t *testing.T) {
	p, err := NewChainPlan(ChainPlanSpec{SchemaRevision: "sunspec.r1@1", BaseCandidates: []uint16{65534}, Limits: ChainLimits{MaxTotalWords: 8, MaxOccurrences: 1}})
	if err != nil {
		t.Fatal(err)
	}
	c := NewChain(p)
	id := uint64(1)
	if _, err := admitNext(t, c, &id, []uint16{0x5375, 0x6e53}); err == nil {
		t.Fatal("signature at 65534 scheduled an unrepresentable header")
	}
	p, err = NewChainPlan(ChainPlanSpec{SchemaRevision: "sunspec.r1@1", BaseCandidates: []uint16{65531}, Limits: ChainLimits{MaxTotalWords: 8, MaxOccurrences: 1}})
	if err != nil {
		t.Fatal(err)
	}
	c = NewChain(p)
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

func TestSunSpecChainSnapshotReadSurfaceMatchesRetainedEvidence(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	snapshot := completedChainSnapshot(t, registry,
		commonOccurrence(t, registry, "Maker", "Model", "1.0", 1),
		inverterOccurrence(t, registry, 113, nil, 2),
	)
	if snapshot.RawWordCount() != len(snapshot.raw) || snapshot.OccurrenceCount() != len(snapshot.occurrences) || snapshot.SourceViewCount() != len(snapshot.sources) {
		t.Fatalf("counts=%d/%d/%d want %d/%d/%d", snapshot.RawWordCount(), snapshot.OccurrenceCount(), snapshot.SourceViewCount(), len(snapshot.raw), len(snapshot.occurrences), len(snapshot.sources))
	}
	if snapshot.RawWordCount() == 0 || snapshot.OccurrenceCount() != 2 || snapshot.SourceViewCount() == 0 {
		t.Fatalf("completed snapshot counts=%d/%d/%d", snapshot.RawWordCount(), snapshot.OccurrenceCount(), snapshot.SourceViewCount())
	}
	var zero ChainSnapshot
	if zero.RawWordCount() != 0 || zero.OccurrenceCount() != 0 || zero.SourceViewCount() != 0 {
		t.Fatal("zero snapshot reports retained evidence")
	}

	chain, ok := snapshot.WireChain()
	want := []WireKey{{ModelID: 1, ModelLength: 65}, {ModelID: 113, ModelLength: 60}, {ModelID: EndModelID, ModelLength: 0}}
	if !ok || !reflect.DeepEqual(chain, want) || EndModelID != 0xffff {
		t.Fatalf("wire chain=%v ok=%t want %v", chain, ok, want)
	}
	truncated := snapshot.Clone()
	truncated.raw = truncated.raw[:len(truncated.raw)-2]
	if chain, ok := truncated.WireChain(); ok || chain != nil {
		t.Fatalf("non-terminal snapshot wire chain=%v ok=%t", chain, ok)
	}
	if chain, ok := zero.WireChain(); ok || chain != nil {
		t.Fatalf("zero snapshot wire chain=%v ok=%t", chain, ok)
	}
}

func TestSunSpecChainSnapshotCloneSharesNoStorage(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	snapshot := completedChainSnapshot(t, registry,
		commonOccurrence(t, registry, "Maker", "Model", "1.0", 1),
		inverterOccurrence(t, registry, 113, nil, 2),
	)
	clone := snapshot.Clone()
	if !reflect.DeepEqual(clone, snapshot) {
		t.Fatal("clone differs from its original")
	}
	snapshot.raw[0]++
	snapshot.occurrences[0].words[0]++
	snapshot.occurrences[0].spans[0].WordCount++
	if clone.raw[0] == snapshot.raw[0] || clone.occurrences[0].words[0] == snapshot.occurrences[0].words[0] ||
		clone.occurrences[0].spans[0].WordCount == snapshot.occurrences[0].spans[0].WordCount ||
		sharesViewStorage(clone.sources[0], snapshot.sources[0]) {
		t.Fatal("clone shares storage with its original")
	}
	if _, ok := clone.WireChain(); !ok {
		t.Fatal("clone of a terminal snapshot is not terminal")
	}
	var zero ChainSnapshot
	if got := zero.Clone(); got.raw != nil || got.occurrences == nil || len(got.occurrences) != 0 || got.sources != nil {
		t.Fatalf("zero clone=%#v", got)
	}
}

func TestNewSunSpecChainSnapshotCopiesPartsAndRevalidatesSourceViews(t *testing.T) {
	registry := mustStandardSunSpecRegistry(t)
	snapshot := completedChainSnapshot(t, registry,
		commonOccurrence(t, registry, "Maker", "Model", "1.0", 1),
		inverterOccurrence(t, registry, 113, nil, 2),
	)
	raw, occurrences, views := snapshot.RawWords(), snapshot.Occurrences(), snapshot.SourceViews()
	assembled, err := NewChainSnapshot(raw, occurrences, views)
	if err != nil || !reflect.DeepEqual(assembled, snapshot) {
		t.Fatalf("assembled=%#v err=%v", assembled, err)
	}
	raw[0]++
	occurrences[0].words[0]++
	if !reflect.DeepEqual(assembled, snapshot) || sharesViewStorage(assembled.sources[0], views[0]) {
		t.Fatal("assembled snapshot shares storage with its inputs")
	}

	invalid := append([]modbusreg.LogicalViewSnapshot(nil), snapshot.SourceViews()...)
	invalid[1] = modbusreg.LogicalViewSnapshot{}
	if got, err := NewChainSnapshot(snapshot.RawWords(), snapshot.Occurrences(), invalid); err == nil || !strings.Contains(err.Error(), "source view 1") || !reflect.DeepEqual(got, ChainSnapshot{}) {
		t.Fatalf("invalid source view snapshot=%#v err=%v", got, err)
	}
	empty, err := NewChainSnapshot(nil, nil, nil)
	if err != nil || empty.RawWordCount() != 0 || empty.OccurrenceCount() != 0 || empty.SourceViewCount() != 0 {
		t.Fatalf("empty parts snapshot=%#v err=%v", empty, err)
	}
	if _, ok := empty.WireChain(); ok {
		t.Fatal("assembly of empty parts is a terminal chain")
	}
}
