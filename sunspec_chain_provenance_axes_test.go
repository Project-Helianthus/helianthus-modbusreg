package modbusreg

import (
	"reflect"
	"strings"
	"testing"
)

func TestSunspecProvenanceAxes(t *testing.T) {
	cases := []struct {
		name   string
		change func(*LogicalViewRecord)
	}{
		{"Endpoint", func(r *LogicalViewRecord) { r.Endpoint = "other-fixture" }},
		{"UnitID", func(r *LogicalViewRecord) { r.UnitID++ }},
		{"ConnectionID", func(r *LogicalViewRecord) { r.ConnectionID++ }},
		{"Transport", func(r *LogicalViewRecord) { r.Transport = TransportRTU }},
		{"TransportGeneration", func(r *LogicalViewRecord) { r.TransportGeneration++ }},
		{"PollGeneration", func(r *LogicalViewRecord) { r.PollGeneration++ }},
		{"DeadlineIdentity", func(r *LogicalViewRecord) { r.DeadlineIdentity++ }},
		{"AuthorizationScope", func(r *LogicalViewRecord) { r.AuthorizationScope = "other-opaque-scope" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := NewSunSpecChainPlan(SunSpecChainPlanSpec{
				SchemaRevision: "sunspec.r1@1",
				BaseCandidates: []uint16{40000},
				Limits:         SunSpecChainLimits{MaxTotalWords: 8, MaxOccurrences: 1},
			})
			if err != nil {
				t.Fatal(err)
			}
			chain := NewSunSpecChain(plan)
			signature := chain.NextRequests()
			if len(signature) != 1 || signature[0].Purpose() != SunSpecReadSignature {
				t.Fatalf("expected one signature request, got %#v", signature)
			}
			view := provenanceAxesView(t, signature[0], 1, []uint16{0x5375, 0x6e53})
			if _, err := chain.AdmitReplay(signature[0], view); err != nil {
				t.Fatalf("valid signature: %v", err)
			}
			pending := append([]SunSpecReadRequest(nil), chain.NextRequests()...)
			if len(pending) != 1 || pending[0].Purpose() != SunSpecReadHeader {
				t.Fatalf("expected one pending header, got %#v", pending)
			}
			original := provenanceAxesView(t, pending[0], 2, []uint16{0xffff, 0})
			changed := original.Record()
			tc.change(&changed)
			mixed, err := NewLogicalViewSnapshot(changed)
			if tc.name == "Transport" {
				// RTU forbids the unchanged TCP connection identity, so only
				// the comparator can isolate this axis without a second change.
				if err == nil || err.Error() != "RTU logical view contradicts runtime shape" {
					t.Fatalf("expected isolated transport to fail upstream validation, got %v", err)
				}
				if !sameSunSpecProvenance(view.Record(), original.Record()) {
					t.Fatal("equal provenance rejected by comparator")
				}
				if sameSunSpecProvenance(original.Record(), changed) {
					t.Fatal("comparator accepted isolated Transport mismatch")
				}
			} else {
				if err != nil {
					t.Fatalf("isolated %s must remain a valid view: %v", tc.name, err)
				}
				if _, err := chain.AdmitReplay(pending[0], mixed); err == nil || !strings.Contains(err.Error(), "mixed provenance") {
					t.Fatalf("expected mixed provenance rejection for %s, got %v", tc.name, err)
				}
			}
			if got := chain.NextRequests(); !reflect.DeepEqual(got, pending) {
				t.Fatalf("pending requests changed: got %#v, want %#v", got, pending)
			}
			snapshot, err := chain.AdmitReplay(pending[0], original)
			if err != nil {
				t.Fatalf("original header after isolated %s: %v", tc.name, err)
			}
			if len(chain.NextRequests()) != 0 || !reflect.DeepEqual(snapshot.RawWords(), []uint16{0x5375, 0x6e53, 0xffff, 0}) {
				t.Fatal("original header did not complete the chain with retained raw signature and header")
			}
		})
	}
}

func provenanceAxesView(t *testing.T, request SunSpecReadRequest, id uint64, words []uint16) LogicalViewSnapshot {
	t.Helper()
	view, err := NewLogicalViewSnapshot(LogicalViewRecord{
		LogicalViewID: id, WireResponseID: id + 100, PhysicalRequestID: id + 200,
		Endpoint: "fixture", UnitID: 1, ConnectionID: 4, Transport: TransportTCP,
		TransportGeneration: 5, PollGeneration: 6, DeadlineIdentity: 7, AuthorizationScope: "opaque-scope",
		RequestedFunction: FunctionReadHoldingRegisters, ReceivedFunction: FunctionReadHoldingRegisters,
		Table: HoldingRegisters, PhysicalOffset: request.Address(), PhysicalWordCount: request.WordCount(),
		LogicalOffset: request.Address(), LogicalWordCount: request.WordCount(), SliceWordCount: request.WordCount(),
		Words: words, WireResponseBytes: []byte{byte(id)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return view
}
