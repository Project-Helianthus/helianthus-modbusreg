package modbusreg

import (
	"fmt"
	"reflect"
	"testing"
)

func TestSunspecPayloadChunkEdges(t *testing.T) {
	for _, tc := range []struct {
		length uint16
		widths []uint16
	}{
		{124, []uint16{124}},
		{125, []uint16{125}},
		{126, []uint16{125, 1}},
		{250, []uint16{125, 125}},
		{251, []uint16{125, 125, 1}},
	} {
		t.Run(fmt.Sprintf("length_%d", tc.length), func(t *testing.T) {
			plan, err := NewSunSpecChainPlan(SunSpecChainPlanSpec{
				SchemaRevision: "sunspec.r1@1",
				BaseCandidates: []uint16{100},
				Limits:         SunSpecChainLimits{MaxTotalWords: 4096, MaxOccurrences: 16},
			})
			if err != nil {
				t.Fatal(err)
			}
			chain := NewSunSpecChain(plan)
			// This opaque model has no decoder key or structural opt-in.
			model := []uint16{65000, tc.length}
			for i := uint16(0); i < tc.length; i++ {
				model = append(model, 1000+i)
			}
			raw := append([]uint16{0x5375, 0x6e53}, model...)
			raw = append(raw, 0xffff, 0)
			type read struct {
				address, width uint16
				purpose        SunSpecReadPurpose
			}
			want := []read{{100, 2, SunSpecReadSignature}, {102, 2, SunSpecReadHeader}}
			address := uint16(104)
			wantSpans := []SunSpecSourceSpan{{LogicalViewID: 2, PDUOffset: 0, WordCount: 2}}
			for i, width := range tc.widths {
				want = append(want, read{address, width, SunSpecReadPayload})
				wantSpans = append(wantSpans, SunSpecSourceSpan{LogicalViewID: uint64(i + 3), PDUOffset: 0, WordCount: width})
				address += width
			}
			want = append(want, read{104 + tc.length, 2, SunSpecReadHeader})
			var snapshot SunSpecChainSnapshot
			for i, expected := range want {
				requests := chain.NextRequests()
				if len(requests) != 1 {
					t.Fatalf("step %d: requests=%v, want exactly one", i, requests)
				}
				r := requests[0]
				got := read{r.Address(), r.WordCount(), r.Purpose()}
				if got != expected || r.Function() != FunctionReadHoldingRegisters {
					t.Fatalf("step %d: request=%+v function=%v, want %+v FC03", i, got, r.Function(), expected)
				}
				id := uint64(i + 1)
				start := int(expected.address - 100)
				view, err := NewLogicalViewSnapshot(LogicalViewRecord{
					LogicalViewID: id, WireResponseID: id + 100, PhysicalRequestID: id + 200,
					Endpoint: "synthetic", ConnectionID: 4, Transport: TransportTCP,
					TransportGeneration: 5, UnitID: 1, AuthorizationScope: "read",
					PollGeneration: 6, DeadlineIdentity: 7,
					RequestedFunction: FunctionReadHoldingRegisters, ReceivedFunction: FunctionReadHoldingRegisters,
					Table: HoldingRegisters, PhysicalOffset: expected.address, PhysicalWordCount: expected.width,
					LogicalOffset: expected.address, LogicalWordCount: expected.width, SliceWordCount: expected.width,
					Words:             append([]uint16(nil), raw[start:start+int(expected.width)]...),
					WireResponseBytes: []byte{byte(id)},
				})
				if err != nil {
					t.Fatal(err)
				}
				snapshot, err = chain.AdmitReplay(r, view)
				if err != nil {
					t.Fatalf("step %d: %v", i, err)
				}
			}
			if requests := chain.NextRequests(); len(requests) != 0 {
				t.Fatalf("requests after end marker=%v", requests)
			}
			if got := snapshot.RawWords(); !reflect.DeepEqual(got, raw) {
				t.Fatalf("raw words=%v, want %v", got, raw)
			}
			occurrences := snapshot.Occurrences()
			if len(occurrences) != 1 {
				t.Fatalf("occurrences=%d, want 1", len(occurrences))
			}
			occurrence := occurrences[0]
			if occurrence.WireKey != (SunSpecWireKey{ModelID: 65000, ModelLength: tc.length}) || occurrence.HeaderOffset != 102 || occurrence.PayloadOffset != 104 {
				t.Fatalf("occurrence geometry=%+v", occurrence)
			}
			if occurrence.Disposition != SunSpecChainDispositionUnknownModel {
				t.Fatalf("opaque disposition=%v", occurrence.Disposition)
			}
			if _, ok := occurrence.DecoderKey(); ok || occurrence.structuralCandidate != nil {
				t.Fatal("opaque model acquired decoder or structural admission")
			}
			if got := occurrence.Words(); !reflect.DeepEqual(got, model) {
				t.Fatalf("model words=%v, want %v", got, model)
			}
			if got := occurrence.SourceSpans(); !reflect.DeepEqual(got, wantSpans) {
				t.Fatalf("source spans=%+v, want %+v", got, wantSpans)
			}
			views := snapshot.SourceViews()
			if len(views) != len(want) {
				t.Fatalf("source views=%d, want %d", len(views), len(want))
			}
			var reconstructed []uint16
			for _, span := range occurrence.SourceSpans() {
				matches := 0
				for _, view := range views {
					source := view.Record()
					if source.LogicalViewID != span.LogicalViewID {
						continue
					}
					matches++
					if source.SliceOffset != span.PDUOffset || source.SliceWordCount != span.WordCount || len(source.Words) != int(span.WordCount) {
						t.Fatalf("span %+v does not cover source %+v", span, source)
					}
					reconstructed = append(reconstructed, source.Words...)
				}
				if matches != 1 {
					t.Fatalf("span %+v matched %d sources", span, matches)
				}
			}
			if !reflect.DeepEqual(reconstructed, model) {
				t.Fatalf("source reconstruction=%v, want %v", reconstructed, model)
			}
		})
	}
}
