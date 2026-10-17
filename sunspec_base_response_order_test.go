package modbusreg

import (
	"fmt"
	"strings"
	"testing"
)

func TestSunspecBaseResponseOrder(t *testing.T) {
	bases := [3]uint16{100, 200, 300}
	orders := [6][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	sets := []struct {
		name    string
		matches [3]bool
		header  uint16
	}{
		{"only_100", [3]bool{true, false, false}, 102},
		{"only_200", [3]bool{false, true, false}, 202},
		{"only_300", [3]bool{false, false, true}, 302},
		{"ambiguous_100_200", [3]bool{true, true, false}, 0},
		{"ambiguous_100_300", [3]bool{true, false, true}, 0},
		{"ambiguous_200_300", [3]bool{false, true, true}, 0},
		{"absent", [3]bool{}, 0},
	}
	for _, set := range sets {
		for _, order := range orders {
			t.Run(fmt.Sprintf("%s/order_%d_%d_%d", set.name, bases[order[0]], bases[order[1]], bases[order[2]]), func(t *testing.T) {
				plan, err := NewSunSpecChainPlan(SunSpecChainPlanSpec{
					SchemaRevision: "sunspec.r1@1",
					BaseCandidates: bases[:],
					Limits:         SunSpecChainLimits{MaxTotalWords: 4, MaxOccurrences: 1},
				})
				if err != nil {
					t.Fatal(err)
				}
				chain := NewSunSpecChain(plan)
				initial := chain.NextRequests()
				if len(initial) != 3 {
					t.Fatalf("signature requests = %d, want 3", len(initial))
				}
				var requests [3]SunSpecReadRequest
				var views [3]LogicalViewSnapshot
				for i, base := range bases {
					found := false
					for _, request := range initial {
						if request.Address() == base {
							if found {
								t.Fatalf("duplicate signature request at %d", base)
							}
							found = true
							requests[i] = request
						}
					}
					if !found || requests[i].Purpose() != SunSpecReadSignature || requests[i].WordCount() != 2 || requests[i].Function() != FunctionReadHoldingRegisters {
						t.Fatalf("missing two-word holding-register signature probe at %d: %+v", base, requests[i])
					}
					words := []uint16{0, 0}
					if set.matches[i] {
						words = []uint16{0x5375, 0x6e53}
					}
					views[i] = sunspecBaseOrderView(t, base, uint64(i+1), words)
				}
				var resolved [3]bool
				matches := 0
				for step, index := range order {
					if set.matches[index] {
						matches++
					}
					snapshot, err := chain.AdmitReplay(requests[index], views[index])
					if len(snapshot.RawWords()) != 0 || len(snapshot.Occurrences()) != 0 || len(snapshot.SourceViews()) != 0 {
						t.Fatal("signature probing published a chain snapshot")
					}
					failure := ""
					if matches == 2 {
						failure = "ambiguous"
					} else if step == 2 && matches == 0 {
						failure = "signature absent"
					}
					if failure != "" {
						if err == nil || !strings.Contains(err.Error(), failure) {
							t.Fatalf("step %d: error = %v, want %q", step+1, err, failure)
						}
						if pending := chain.NextRequests(); len(pending) != 0 {
							t.Fatalf("terminal failure retained requests: %+v", pending)
						}
						for i, request := range requests {
							if _, err := chain.AdmitReplay(request, views[i]); err == nil {
								t.Fatalf("terminal failure accepted probe at %d", request.Address())
							}
						}
						if pending := chain.NextRequests(); len(pending) != 0 {
							t.Fatalf("terminal failure recovered requests: %+v", pending)
						}
						return
					}
					if err != nil {
						t.Fatalf("step %d: %v", step+1, err)
					}
					resolved[index] = true
					pending := chain.NextRequests()
					if step < 2 {
						if len(pending) != 2-step {
							t.Fatalf("step %d: pending = %+v, want only %d unresolved signatures", step+1, pending, 2-step)
						}
						for i, request := range requests {
							count := 0
							for _, next := range pending {
								if next == request {
									count++
								}
							}
							want := 1
							if resolved[i] {
								want = 0
							}
							if count != want {
								t.Fatalf("step %d: pending multiplicity at %d = %d, want %d", step+1, bases[i], count, want)
							}
						}
					} else if len(pending) != 1 || pending[0].Purpose() != SunSpecReadHeader || pending[0].Address() != set.header || pending[0].WordCount() != 2 || pending[0].Function() != FunctionReadHoldingRegisters {
						t.Fatalf("resolved signatures queued %+v, want exactly a two-word header at %d", pending, set.header)
					}
				}
			})
		}
	}
}

func sunspecBaseOrderView(t *testing.T, base uint16, id uint64, words []uint16) LogicalViewSnapshot {
	t.Helper()
	view, err := NewLogicalViewSnapshot(LogicalViewRecord{
		LogicalViewID: id, WireResponseID: id + 10, PhysicalRequestID: id + 20,
		Endpoint: "synthetic-sunspec", ConnectionID: 1, Transport: TransportTCP,
		TransportGeneration: 2, UnitID: 1, PollGeneration: 3, DeadlineIdentity: 4,
		AuthorizationScope: "read", RequestedFunction: FunctionReadHoldingRegisters,
		ReceivedFunction: FunctionReadHoldingRegisters, Table: HoldingRegisters,
		PhysicalOffset: base, PhysicalWordCount: 2, LogicalOffset: base,
		LogicalWordCount: 2, SliceOffset: 0, SliceWordCount: 2, Words: words,
		WireResponseBytes: []byte{3, 4, byte(words[0] >> 8), byte(words[0]), byte(words[1] >> 8), byte(words[1])},
	})
	if err != nil {
		t.Fatal(err)
	}
	return view
}
