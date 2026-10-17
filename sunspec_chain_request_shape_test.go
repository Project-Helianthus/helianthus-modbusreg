package modbusreg

import (
	"reflect"
	"testing"
)

func TestSunspecRequestShapeRejectsForgeryWithoutConsumingHeader(t *testing.T) {
	for _, field := range []string{"address", "words", "purpose"} {
		t.Run(field, func(t *testing.T) {
			plan, err := NewSunSpecChainPlan(SunSpecChainPlanSpec{
				SchemaRevision: "sunspec.r1@1",
				BaseCandidates: []uint16{40000},
				Limits:         SunSpecChainLimits{MaxTotalWords: 8, MaxOccurrences: 1},
			})
			if err != nil {
				t.Fatal(err)
			}
			chain := NewSunSpecChain(plan)
			signatures := chain.NextRequests()
			if len(signatures) != 1 {
				t.Fatalf("signature requests: got %d, want 1", len(signatures))
			}
			if _, err := chain.AdmitReplay(signatures[0], requestShapeView(t, signatures[0], 1, []uint16{0x5375, 0x6e53})); err != nil {
				t.Fatalf("admit signature: %v", err)
			}

			pending := append([]SunSpecReadRequest(nil), chain.NextRequests()...)
			if len(pending) != 1 {
				t.Fatalf("header requests: got %d, want 1", len(pending))
			}
			header := pending[0]
			if header.Address() != 40002 || header.WordCount() != 2 || header.Purpose() != SunSpecReadHeader {
				t.Fatalf("unexpected pending header: %+v", header)
			}
			forged := header
			words := []uint16{0xffff, 0}
			switch field {
			case "address":
				forged.address++
			case "words":
				forged.words = 3
				words = append(words, 0)
			case "purpose":
				forged.purpose = SunSpecReadPayload
			}
			if forged.nonce != header.nonce || forged.instance != header.instance || forged.sequence != header.sequence {
				t.Fatal("shape forgery changed request tokens")
			}
			view := requestShapeView(t, forged, 2, words)
			_, err = chain.AdmitReplay(forged, view)
			if err == nil || err.Error() != "SunSpec request was not planned" {
				t.Errorf("forged %s: got %v, want SunSpec request was not planned", field, err)
			}
			if got := chain.NextRequests(); !reflect.DeepEqual(got, pending) {
				t.Errorf("forged %s consumed or changed pending work: got %+v, want %+v", field, got, pending)
			}
			snapshot, err := chain.AdmitReplay(header, requestShapeView(t, header, 2, []uint16{0xffff, 0}))
			if err != nil {
				t.Fatalf("original header after rejected %s forgery: %v", field, err)
			}
			if got := snapshot.RawWords(); !reflect.DeepEqual(got, []uint16{0x5375, 0x6e53, 0xffff, 0}) {
				t.Errorf("original header result: got %v, want signature and end marker", got)
			}
			if got := chain.NextRequests(); len(got) != 0 {
				t.Errorf("original end header left pending work: %+v", got)
			}
		})
	}
}

func requestShapeView(t *testing.T, request SunSpecReadRequest, id uint64, words []uint16) LogicalViewSnapshot {
	t.Helper()
	view, err := NewLogicalViewSnapshot(LogicalViewRecord{
		LogicalViewID: id, WireResponseID: id + 10, PhysicalRequestID: id + 20,
		Endpoint: "synthetic", ConnectionID: 1, Transport: TransportTCP,
		TransportGeneration: 1, UnitID: 1,
		RequestedFunction: FunctionReadHoldingRegisters, ReceivedFunction: FunctionReadHoldingRegisters,
		Table: HoldingRegisters, PhysicalOffset: request.Address(), PhysicalWordCount: request.WordCount(),
		AuthorizationScope: "read", PollGeneration: 1, DeadlineIdentity: 1,
		LogicalOffset: request.Address(), LogicalWordCount: request.WordCount(),
		SliceWordCount: request.WordCount(), Words: words, WireResponseBytes: []byte{byte(id)},
	})
	if err != nil {
		t.Fatalf("construct matching synthetic view: %v", err)
	}
	return view
}
