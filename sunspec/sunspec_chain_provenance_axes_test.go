package sunspec

import (
	"reflect"
	"strings"
	"testing"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestSunspecProvenanceAxes(t *testing.T) {
	cases := []struct {
		name   string
		change func(*modbusreg.LogicalViewRecord)
	}{
		{"Endpoint", func(r *modbusreg.LogicalViewRecord) { r.Endpoint = "other-fixture" }},
		{"UnitID", func(r *modbusreg.LogicalViewRecord) { r.UnitID++ }},
		{"ConnectionID", func(r *modbusreg.LogicalViewRecord) { r.ConnectionID++ }},
		{"Transport", func(r *modbusreg.LogicalViewRecord) { r.Transport = modbusreg.TransportRTU }},
		{"TransportGeneration", func(r *modbusreg.LogicalViewRecord) { r.TransportGeneration++ }},
		{"PollGeneration", func(r *modbusreg.LogicalViewRecord) { r.PollGeneration++ }},
		{"DeadlineIdentity", func(r *modbusreg.LogicalViewRecord) { r.DeadlineIdentity++ }},
		{"AuthorizationScope", func(r *modbusreg.LogicalViewRecord) { r.AuthorizationScope = "other-opaque-scope" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := NewChainPlan(ChainPlanSpec{
				SchemaRevision: "sunspec.r1@1",
				BaseCandidates: []uint16{40000},
				Limits:         ChainLimits{MaxTotalWords: 8, MaxOccurrences: 1},
			})
			if err != nil {
				t.Fatal(err)
			}
			chain := NewChain(plan)
			signature := chain.NextRequests()
			if len(signature) != 1 || signature[0].Purpose() != ReadSignature {
				t.Fatalf("expected one signature request, got %#v", signature)
			}
			view := provenanceAxesView(t, signature[0], 1, []uint16{0x5375, 0x6e53})
			if _, err := chain.AdmitReplay(signature[0], view); err != nil {
				t.Fatalf("valid signature: %v", err)
			}
			pending := append([]ReadRequest(nil), chain.NextRequests()...)
			if len(pending) != 1 || pending[0].Purpose() != ReadHeader {
				t.Fatalf("expected one pending header, got %#v", pending)
			}
			original := provenanceAxesView(t, pending[0], 2, []uint16{0xffff, 0})
			changed := original.Record()
			tc.change(&changed)
			mixed, err := modbusreg.NewLogicalViewSnapshot(changed)
			if tc.name == "Transport" {
				// RTU forbids the unchanged TCP connection identity, so only
				// the comparator can isolate this axis without a second change.
				if err == nil || err.Error() != "RTU logical view contradicts runtime shape" {
					t.Fatalf("expected isolated transport to fail upstream validation, got %v", err)
				}
				if !SameProvenance(view.Record(), original.Record()) {
					t.Fatal("equal provenance rejected by comparator")
				}
				if SameProvenance(original.Record(), changed) {
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

func provenanceAxesView(t *testing.T, request ReadRequest, id uint64, words []uint16) modbusreg.LogicalViewSnapshot {
	t.Helper()
	view, err := modbusreg.NewLogicalViewSnapshot(modbusreg.LogicalViewRecord{
		LogicalViewID: id, WireResponseID: id + 100, PhysicalRequestID: id + 200,
		Endpoint: "fixture", UnitID: 1, ConnectionID: 4, Transport: modbusreg.TransportTCP,
		TransportGeneration: 5, PollGeneration: 6, DeadlineIdentity: 7, AuthorizationScope: "opaque-scope",
		RequestedFunction: modbusreg.FunctionReadHoldingRegisters, ReceivedFunction: modbusreg.FunctionReadHoldingRegisters,
		Table: modbusreg.HoldingRegisters, PhysicalOffset: request.Address(), PhysicalWordCount: request.WordCount(),
		LogicalOffset: request.Address(), LogicalWordCount: request.WordCount(), SliceWordCount: request.WordCount(),
		Words: words, WireResponseBytes: []byte{byte(id)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return view
}

// TestSunSpecSameProvenanceComparesEveryAxis varies each compared field of a
// valid record once and one field outside the comparison: the comparator must
// reject each provenance change and ignore the rest.
func TestSunSpecSameProvenanceComparesEveryAxis(t *testing.T) {
	base := modbusreg.LogicalViewRecord{
		LogicalViewID: 1, WireResponseID: 2, PhysicalRequestID: 3, Endpoint: "fixture-endpoint", ConnectionID: 4,
		Transport: modbusreg.TransportTCP, TransportGeneration: 5, UnitID: 1, AuthorizationScope: "fixture-read-only",
		PollGeneration: 6, DeadlineIdentity: 7, Words: []uint16{1}, WireResponseBytes: []byte{1},
	}
	if !SameProvenance(base, base) {
		t.Fatal("identical provenance rejected")
	}
	for name, change := range map[string]func(*modbusreg.LogicalViewRecord){
		"Endpoint":            func(r *modbusreg.LogicalViewRecord) { r.Endpoint = "other-fixture" },
		"UnitID":              func(r *modbusreg.LogicalViewRecord) { r.UnitID++ },
		"ConnectionID":        func(r *modbusreg.LogicalViewRecord) { r.ConnectionID++ },
		"Transport":           func(r *modbusreg.LogicalViewRecord) { r.Transport = modbusreg.TransportRTU },
		"TransportGeneration": func(r *modbusreg.LogicalViewRecord) { r.TransportGeneration++ },
		"PollGeneration":      func(r *modbusreg.LogicalViewRecord) { r.PollGeneration++ },
		"DeadlineIdentity":    func(r *modbusreg.LogicalViewRecord) { r.DeadlineIdentity++ },
		"AuthorizationScope":  func(r *modbusreg.LogicalViewRecord) { r.AuthorizationScope = "other-opaque-scope" },
	} {
		changed := base
		change(&changed)
		if SameProvenance(base, changed) || SameProvenance(changed, base) {
			t.Fatalf("comparator accepted a %s change", name)
		}
	}
	other := base
	other.LogicalViewID, other.WireResponseID, other.PhysicalRequestID, other.Words = 9, 9, 9, []uint16{9}
	if !SameProvenance(base, other) {
		t.Fatal("comparator rejected a change outside the provenance axes")
	}
}
