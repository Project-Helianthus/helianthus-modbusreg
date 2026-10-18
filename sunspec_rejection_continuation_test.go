package modbusreg

import (
	"reflect"
	"testing"
)

func TestSunSpecRejectedShapeContinuation(t *testing.T) {
	for _, field := range []string{"address", "quantity", "purpose"} {
		t.Run(field, func(t *testing.T) {
			plan, err := NewSunSpecChainPlan(SunSpecChainPlanSpec{
				SchemaRevision: "sunspec.r1@1",
				BaseCandidates: []uint16{40000},
				Limits:         SunSpecChainLimits{MaxTotalWords: 8, MaxOccurrences: 1},
			})
			if err != nil {
				t.Fatal(err)
			}
			control, subject := NewSunSpecChain(plan), NewSunSpecChain(plan)
			if control.instance == subject.instance {
				t.Fatal("control must be a detached chain instance")
			}
			steps := []struct {
				address, quantity uint16
				purpose           SunSpecReadPurpose
				words             []uint16
			}{
				{40000, 2, SunSpecReadSignature, []uint16{0x5375, 0x6e53}},
				{40002, 2, SunSpecReadHeader, []uint16{65000, 2}},
				{40004, 2, SunSpecReadPayload, []uint16{17, 23}},
				{40006, 2, SunSpecReadHeader, []uint16{0xffff, 0}},
			}
			for i, step := range steps {
				pending := control.NextRequests()
				if len(pending) != 1 {
					t.Fatalf("step %d control pending count: got %d, want 1", i, len(pending))
				}
				request := pending[0]
				if request.address != step.address || request.words != step.quantity || request.purpose != step.purpose || request.sequence != uint64(i+1) {
					t.Fatalf("step %d control request disagrees with independent trace: %+v", i, request)
				}
				// Only the instance differs between detached chains sharing a plan.
				original := request
				original.instance = subject.instance
				wantPending := []SunSpecReadRequest{original}
				if got := subject.NextRequests(); !reflect.DeepEqual(got, wantPending) {
					t.Errorf("step %d subject pending differs from control: got %+v, want %+v", i, got, wantPending)
				}
				id := uint64(i + 1)
				if i == 1 {
					forged := original
					words := append([]uint16(nil), step.words...)
					switch field {
					case "address":
						forged.address++
					case "quantity":
						forged.words++
						words = append(words, 0)
					case "purpose":
						forged.purpose = SunSpecReadPayload
					}
					if forged.nonce != original.nonce || forged.instance != original.instance || forged.sequence != original.sequence {
						t.Fatal("shape forgery changed plan, instance or sequence tokens")
					}
					rejected, err := subject.AdmitReplay(forged, requestShapeView(t, forged, id, words))
					if err == nil || err.Error() != "SunSpec request was not planned" {
						t.Errorf("forged %s: got %v, want SunSpec request was not planned", field, err)
					}
					if !reflect.DeepEqual(rejected, SunSpecChainSnapshot{}) {
						t.Errorf("forged %s returned nonzero snapshot: %+v", field, rejected)
					}
					if got := subject.NextRequests(); !reflect.DeepEqual(got, wantPending) {
						t.Errorf("forged %s changed pending work: got %+v, want %+v", field, got, wantPending)
					}
				}
				want, err := control.AdmitReplay(request, requestShapeView(t, request, id, step.words))
				if err != nil {
					t.Fatalf("step %d control continuation: %v", i, err)
				}
				got, err := subject.AdmitReplay(original, requestShapeView(t, original, id, step.words))
				if err != nil {
					t.Fatalf("step %d continuation after rejected %s (control succeeded): %v", i, field, err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("step %d snapshot differs from untouched control: got %+v, want %+v", i, got, want)
				}
				if i == len(steps)-1 {
					if raw := got.RawWords(); !reflect.DeepEqual(raw, []uint16{0x5375, 0x6e53, 65000, 2, 17, 23, 0xffff, 0}) {
						t.Errorf("completed raw words: %v", raw)
					}
					if len(got.Occurrences()) != 1 || len(got.SourceViews()) != 4 {
						t.Errorf("completed evidence: want one occurrence and four source views, got %+v", got)
					}
				}
			}
			if got, want := subject.NextRequests(), control.NextRequests(); len(got) != 0 || len(want) != 0 {
				t.Errorf("completed pending work: subject %+v, control %+v", got, want)
			}
		})
	}
}
