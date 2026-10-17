package modbusreg

import (
	"reflect"
	"strings"
	"testing"
)

func TestSunspecExactChainBudget(t *testing.T) {
	for _, tc := range []struct {
		name        string
		words       uint32
		occurrences uint32
		wantError   string
	}{
		{"exact_words_and_occurrences", 10, 2, ""},
		{"one_word_short_reserves_terminator", 9, 2, "total word bound exceeded"},
		{"one_occurrence_short", 10, 1, "occurrence bound exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := NewSunSpecChainPlan(SunSpecChainPlanSpec{
				SchemaRevision: "sunspec.r1@1",
				BaseCandidates: []uint16{0},
				Limits: SunSpecChainLimits{
					MaxTotalWords: tc.words, MaxOccurrences: tc.occurrences,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			chain := NewSunSpecChain(plan)
			// Synthetic stream: signature + two opaque one-word occurrences + terminator.
			steps := []struct {
				address uint16
				purpose SunSpecReadPurpose
				words   []uint16
			}{
				{0, SunSpecReadSignature, []uint16{0x5375, 0x6e53}},
				{2, SunSpecReadHeader, []uint16{65000, 1}},
				{4, SunSpecReadPayload, []uint16{0x1234}},
				{5, SunSpecReadHeader, []uint16{65000, 1}},
				{7, SunSpecReadPayload, []uint16{0xabcd}},
				{8, SunSpecReadHeader, []uint16{0xffff, 0}},
			}
			var snapshot SunSpecChainSnapshot
			for index, step := range steps {
				requests := chain.NextRequests()
				if len(requests) != 1 {
					t.Fatalf("step %d requests=%v, want exactly one", index, requests)
				}
				request := requests[0]
				if request.Address() != step.address || request.WordCount() != uint16(len(step.words)) || request.Purpose() != step.purpose {
					t.Fatalf("step %d unexpected request: %#v", index, request)
				}
				view := exactChainBudgetView(t, request, uint64(index+1), step.words)
				snapshot, err = chain.AdmitReplay(request, view)
				if index == 3 && tc.wantError != "" {
					if err == nil || !strings.Contains(err.Error(), tc.wantError) {
						t.Fatalf("second header error=%v, want %q", err, tc.wantError)
					}
					if !chain.failed || chain.complete || chain.current != nil || len(chain.NextRequests()) != 0 {
						t.Fatalf("rejection is not terminal and empty: failed=%v complete=%v current=%v requests=%v", chain.failed, chain.complete, chain.current, chain.NextRequests())
					}
					if len(snapshot.RawWords()) != 0 || len(snapshot.Occurrences()) != 0 {
						t.Fatal("rejection published a completed snapshot")
					}
					if !reflect.DeepEqual(chain.raw, []uint16{0x5375, 0x6e53, 65000, 1, 0x1234}) || len(chain.occ) != 1 || len(chain.sources) != 3 {
						t.Fatal("rejection changed previously accepted evidence")
					}
					if !reflect.DeepEqual(chain.occ[0].Words(), []uint16{65000, 1, 0x1234}) {
						t.Fatal("rejection changed the accepted occurrence words")
					}
					if _, retryErr := chain.AdmitReplay(request, view); retryErr == nil {
						t.Fatal("failed chain accepted the rejected request again")
					}
					if len(chain.NextRequests()) != 0 {
						t.Fatal("retry repopulated terminal requests")
					}
					return
				}
				if err != nil {
					t.Fatalf("step %d: %v", index, err)
				}
				if index < len(steps)-1 && (chain.complete || len(snapshot.RawWords()) != 0 || len(snapshot.Occurrences()) != 0) {
					t.Fatalf("step %d completed before the terminator", index)
				}
			}
			if !chain.complete || chain.failed || len(chain.NextRequests()) != 0 {
				t.Fatal("exact-budget chain did not complete with no pending requests")
			}
			wantRaw := []uint16{0x5375, 0x6e53, 65000, 1, 0x1234, 65000, 1, 0xabcd, 0xffff, 0}
			if !reflect.DeepEqual(snapshot.RawWords(), wantRaw) || len(snapshot.SourceViews()) != 6 {
				t.Fatalf("exact-budget evidence lost: words=%v sources=%d", snapshot.RawWords(), len(snapshot.SourceViews()))
			}
			occurrences := snapshot.Occurrences()
			if len(occurrences) != 2 {
				t.Fatalf("occurrences=%d, want two", len(occurrences))
			}
			for index, payload := range []uint16{0x1234, 0xabcd} {
				occurrence := occurrences[index]
				if !reflect.DeepEqual(occurrence.Words(), []uint16{65000, 1, payload}) || occurrence.Ordinal != uint32(index+1) {
					t.Fatalf("occurrence %d lost order or words: %#v", index, occurrence)
				}
				if occurrence.Disposition != SunSpecChainDispositionUnknownModel || occurrence.structuralCandidate != nil {
					t.Fatalf("opaque occurrence %d activated: %#v", index, occurrence)
				}
				if _, ok := occurrence.DecoderKey(); ok {
					t.Fatalf("opaque occurrence %d acquired a decoder", index)
				}
			}
		})
	}
}

func exactChainBudgetView(t *testing.T, request SunSpecReadRequest, id uint64, words []uint16) LogicalViewSnapshot {
	t.Helper()
	view, err := NewLogicalViewSnapshot(LogicalViewRecord{
		LogicalViewID: id, WireResponseID: id + 100, PhysicalRequestID: id + 200,
		Endpoint: "synthetic", ConnectionID: 1, Transport: TransportTCP,
		TransportGeneration: 1, UnitID: 1,
		RequestedFunction: FunctionReadHoldingRegisters, ReceivedFunction: FunctionReadHoldingRegisters,
		Table: HoldingRegisters, PhysicalOffset: request.Address(), PhysicalWordCount: request.WordCount(),
		AuthorizationScope: "read", PollGeneration: 1, DeadlineIdentity: 1,
		LogicalOffset: request.Address(), LogicalWordCount: request.WordCount(),
		SliceWordCount: request.WordCount(), Words: words, WireResponseBytes: []byte{byte(id)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return view
}
