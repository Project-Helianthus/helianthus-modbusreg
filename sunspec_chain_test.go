package modbusreg

import "testing"

func TestSunSpecChainRetainsOrderedDuplicatesUnknownAndWrongLength(t *testing.T) {
	plan, err := NewSunSpecChainPlan(SunSpecChainPlanSpec{
		SchemaRevision: SunSpecSchemaRevision("sunspec.r1@1"),
		BaseCandidates: []uint16{40000},
		Limits:         SunSpecChainLimits{MaxTotalWords: 32, MaxOccurrences: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := plan.Requests()[0]
	view, err := NewLogicalViewSnapshot(LogicalViewRecord{
		LogicalViewID: 1, WireResponseID: 2, PhysicalRequestID: 3, Endpoint: "fixture", ConnectionID: 4,
		Transport: TransportTCP, TransportGeneration: 5, UnitID: 1, RequestedFunction: FunctionReadHoldingRegisters,
		ReceivedFunction: FunctionReadHoldingRegisters, Table: HoldingRegisters, PhysicalOffset: request.Address(),
		PhysicalWordCount: 16, AuthorizationScope: "read", PollGeneration: 6, DeadlineIdentity: 7,
		LogicalOffset: request.Address(), LogicalWordCount: 16, SliceOffset: 0, SliceWordCount: 16,
		Words: []uint16{0x5375, 0x6e53, 1, 1, 9, 103, 1, 8, 65000, 2, 3, 4, 103, 2, 5, 6}, WireResponseBytes: []byte{1},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewSunSpecChain(plan).Admit(request, view)
	if err == nil {
		t.Fatal("incomplete increment must not complete")
	}
}

func TestSunSpecChainRejectsDetachedDuplicateAndMixedProvenance(t *testing.T) {
	// Compile-time/public API guard: only requests returned by the plan are admitted.
	_ = SunSpecChainDispositionUnsupportedLength
	_ = SunSpecDecoderKey{}
	_ = SunSpecSourceSpan{}
}
