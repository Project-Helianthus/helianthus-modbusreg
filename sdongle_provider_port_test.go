package modbusreg

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func i21Plan(i21child bool) SDongleRoutePlan {
	i21key := SDongleRouteKey{Family: "S-Dongle", Model: "S-DongleB-03", Firmware: "V200R025C00SPC120", ProtocolMajor: 5, Role: SDongleGateway, Unit: 100, Request: SDongleBasicMEI, Scope: "basic-identity", Revision: "synthetic-rA"}
	i21obs := HuaweiSDongleOfflineObservation{UnitID: 100, Model: i21key.Model, Firmware: i21key.Firmware, ProtocolMajor: 5, Capacity: 2, ChildCount: 1, ChangeSequenceBefore: 4, ChangeSequenceAfter: 4}
	if i21child {
		i21key.Child = SDongleChildRoute{Parent: SDongleRouteSelection{Family: i21key.Family, Model: i21key.Model, Firmware: i21key.Firmware, ProtocolMajor: 5, Role: SDongleGateway, Unit: 100, Request: SDongleBasicMEI, Scope: "basic-identity", Revision: "synthetic-rA"}, EvidenceRevision: "synthetic-rA", Qualification: "qualified", GuardBefore: 4, GuardAfter: 4}
		i21key.Model, i21key.Firmware, i21key.Role, i21key.Unit, i21key.Request, i21key.Scope = "synthetic-child", "synthetic-firmware", SDongleChild, 7, SDongleExtendedMEI, "extended-inventory"
	}
	i21ref := SDongleRouteReference{Revision: "synthetic-rA", Reference: "synthetic-evidence"}
	return SDongleRoutePlan{Key: i21key, Observation: i21obs, Evidence: SDongleRouteEvidence{Key: i21key, Current: true, Identity: i21ref, Protocol: i21ref, Allowlist: i21ref, Route: SDongleRouteReference{Revision: "synthetic-rA", Reference: "synthetic-child-route"}, References: []string{"synthetic-native-record"}}, Bounds: SDongleRouteBounds{Deadline: time.Second, RetryBounded: true, MaxPages: 1, MaxObjects: 1, MaxBytes: 128}}
}

func i21Request(i21plan SDongleRoutePlan) SDongleOfflineRequest {
	return SDongleOfflineRequest{Key: i21plan.Key, AttemptedAt: time.Unix(10, 0), Bounds: SDongleOfflineBounds{LookupTimeout: time.Second, MaxInputBytes: 8192, MaxReferences: 16}}
}

func i21Axes(i21visit func(string, []int)) {
	var i21walk func(reflect.Type, []int, string)
	i21walk = func(i21typ reflect.Type, i21path []int, i21prefix string) {
		for i21index := 0; i21index < i21typ.NumField(); i21index++ {
			i21field := i21typ.Field(i21index)
			i21next := append(append([]int{}, i21path...), i21index)
			if i21field.Type.Kind() == reflect.Struct {
				i21walk(i21field.Type, i21next, i21prefix+i21field.Name+".")
			} else {
				i21visit(i21prefix+i21field.Name, i21next)
			}
		}
	}
	i21walk(reflect.TypeOf(SDongleRouteKey{}), nil, "")
}

func i21Change(i21key *SDongleRouteKey, i21path []int) {
	i21value := reflect.ValueOf(i21key).Elem().FieldByIndex(i21path)
	if i21value.Kind() == reflect.String {
		i21value.SetString(i21value.String() + "-other")
	} else {
		i21value.SetUint(i21value.Uint() + 1)
	}
}

func TestI21ExactKey(i21t *testing.T) {
	i21Axes(func(i21name string, i21path []int) {
		i21t.Run(i21name, func(i21t *testing.T) {
			i21plan := i21Plan(true)
			i21calls := 0
			i21owner, i21err := I21NewSDonglePort(i21Request(i21plan), func(context.Context, SDongleRouteKey) (SDongleRoutePlan, error) { i21calls++; return i21plan, nil })
			if i21err != nil || i21calls != 0 {
				i21t.Fatalf("constructor: %v calls=%d", i21err, i21calls)
			}
			i21key := i21plan.Key
			i21Change(&i21key, i21path)
			i21result, i21err := i21owner.I21Refresh(context.Background(), i21key, time.Unix(20, 0))
			if !errors.Is(i21err, ErrI21Association) || i21calls != 0 || i21result.Decision != nil || i21result.Retained != nil || i21result.State != SDongleOfflineBlocked || !i21result.DefaultDenied() {
				i21t.Fatalf("key accepted: %+v %v calls=%d", i21result, i21err, i21calls)
			}
			i21result, i21err = i21owner.I21Refresh(context.Background(), i21plan.Key, time.Unix(30, 0))
			if i21err != nil || i21calls != 1 || i21result.State != SDongleOfflineReady || !i21result.DefaultDenied() || !i21result.Decision.DefaultDenied() {
				i21t.Fatalf("frozen route lost: %+v %v", i21result, i21err)
			}
		})
	})
}

func TestI21ReturnedAssociations(i21t *testing.T) {
	i21Axes(func(i21name string, i21path []int) {
		for _, i21evidence := range []bool{false, true} {
			i21label := "plan/"
			if i21evidence {
				i21label = "evidence/"
			}
			i21t.Run(i21label+i21name, func(i21t *testing.T) {
				i21plan := i21Plan(true)
				i21req := i21Request(i21plan)
				if i21evidence {
					i21Change(&i21plan.Evidence.Key, i21path)
				} else {
					i21Change(&i21plan.Key, i21path)
				}
				i21calls := 0
				i21owner, i21err := I21NewSDonglePort(i21req, func(context.Context, SDongleRouteKey) (SDongleRoutePlan, error) { i21calls++; return i21plan, nil })
				if i21err != nil {
					i21t.Fatal(i21err)
				}
				i21result, i21err := i21owner.I21Refresh(context.Background(), i21req.Key, time.Unix(20, 0))
				if i21calls != 1 || i21result.State != SDongleOfflineBlocked || i21result.Retained != nil || !i21result.DefaultDenied() {
					i21t.Fatalf("association promoted: %+v %v calls=%d", i21result, i21err, i21calls)
				}
				if i21evidence {
					if i21err != nil || i21result.Decision == nil || i21result.Decision.Outcome != SDongleWithheld {
						i21t.Fatalf("native validation lost: %+v %v", i21result, i21err)
					}
				} else if !errors.Is(i21err, ErrSDongleOfflineAssociation) || i21result.Decision != nil {
					i21t.Fatalf("plan accepted: %+v %v", i21result, i21err)
				}
			})
		}
	})
}

func TestI21Refusals(i21t *testing.T) {
	for i21name, i21change := range map[string]func(*SDongleOfflineRequest){
		"empty-key":      func(i21r *SDongleOfflineRequest) { i21r.Key.Family = "" },
		"zero-time":      func(i21r *SDongleOfflineRequest) { i21r.AttemptedAt = time.Time{} },
		"timeout-zero":   func(i21r *SDongleOfflineRequest) { i21r.Bounds.LookupTimeout = 0 },
		"timeout-excess": func(i21r *SDongleOfflineRequest) { i21r.Bounds.LookupTimeout = 16 * time.Second },
		"bytes-zero":     func(i21r *SDongleOfflineRequest) { i21r.Bounds.MaxInputBytes = 0 },
		"bytes-excess":   func(i21r *SDongleOfflineRequest) { i21r.Bounds.MaxInputBytes = 1 << 21 },
		"refs-zero":      func(i21r *SDongleOfflineRequest) { i21r.Bounds.MaxReferences = 0 },
		"refs-excess":    func(i21r *SDongleOfflineRequest) { i21r.Bounds.MaxReferences = 257 },
		"key-excess":     func(i21r *SDongleOfflineRequest) { i21r.Key.Model = strings.Repeat("x", 8193) },
		"previous-key": func(i21r *SDongleOfflineRequest) {
			i21r.Previous = &SDongleOfflineRecord{Decision: DecideSDongleRoute(i21Plan(true)), EvaluatedAt: time.Unix(1, 0)}
		},
		"previous-outcome": func(i21r *SDongleOfflineRequest) {
			i21r.Previous = &SDongleOfflineRecord{Decision: SDongleRouteDecision{Key: i21r.Key}, EvaluatedAt: time.Unix(1, 0)}
		},
	} {
		i21t.Run(i21name, func(i21t *testing.T) {
			i21calls := 0
			i21req := i21Request(i21Plan(false))
			i21change(&i21req)
			i21owner, i21err := I21NewSDonglePort(i21req, func(context.Context, SDongleRouteKey) (SDongleRoutePlan, error) {
				i21calls++
				return i21Plan(false), nil
			})
			if i21err == nil || i21owner != nil || i21calls != 0 {
				i21t.Fatalf("accepted: %v calls=%d", i21err, i21calls)
			}
		})
	}
	if i21owner, i21err := I21NewSDonglePort(i21Request(i21Plan(false)), nil); i21err == nil || i21owner != nil {
		i21t.Fatal("nil supplier accepted")
	}
	i21calls := 0
	i21req := i21Request(i21Plan(false))
	i21owner, _ := I21NewSDonglePort(i21req, func(context.Context, SDongleRouteKey) (SDongleRoutePlan, error) {
		i21calls++
		return i21Plan(false), nil
	})
	i21ctx, i21cancel := context.WithCancel(context.Background())
	i21cancel()
	if _, i21err := i21owner.I21Refresh(i21ctx, i21req.Key, time.Unix(20, 0)); !errors.Is(i21err, context.Canceled) {
		i21t.Fatal(i21err)
	}
	if _, i21err := i21owner.I21Refresh(context.Background(), i21req.Key, time.Time{}); !errors.Is(i21err, ErrSDongleOfflineInput) {
		i21t.Fatal(i21err)
	}
	i21owner.I21Retire()
	i21owner.I21Retire()
	if _, i21err := i21owner.I21Refresh(context.Background(), i21req.Key, time.Unix(20, 0)); !errors.Is(i21err, ErrI21Retired) {
		i21t.Fatal(i21err)
	}
	if i21calls != 0 {
		i21t.Fatal("refusal called supplier")
	}
}

func TestI21RetentionAndDetachment(i21t *testing.T) {
	for _, i21child := range []bool{false, true} {
		i21plan := i21Plan(i21child)
		i21req := i21Request(i21plan)
		i21req.Previous = &SDongleOfflineRecord{Decision: DecideSDongleRoute(i21Plan(i21child)), EvaluatedAt: time.Unix(1, 0)}
		i21calls := 0
		i21mode := "ready"
		i21owner, i21err := I21NewSDonglePort(i21req, func(i21ctx context.Context, i21key SDongleRouteKey) (SDongleRoutePlan, error) {
			i21calls++
			if i21key != i21Request(i21Plan(i21child)).Key {
				i21t.Fatal("frozen input mutated")
			}
			if _, i21ok := i21ctx.Deadline(); !i21ok {
				i21t.Fatal("missing bound")
			}
			i21candidate := i21Plan(i21child)
			switch i21mode {
			case "error":
				return i21candidate, errors.New("synthetic failure")
			case "blocked":
				i21candidate.Evidence.Current = false
			case "denied":
				i21candidate.QualificationStopped = true
			}
			i21plan = i21candidate
			return i21candidate, nil
		})
		if i21err != nil {
			i21t.Fatal(i21err)
		}
		i21req.Bounds = SDongleOfflineBounds{}
		i21req.Key.Model = "changed"
		i21req.Previous.Decision.Evidence.References[0] = "changed"
		if i21owner.I21Snapshot().Retained.Decision.Evidence.References[0] != "synthetic-native-record" {
			i21t.Fatal("input alias")
		}
		i21ready, i21err := i21owner.I21Refresh(context.Background(), i21Plan(i21child).Key, time.Unix(20, 0))
		if i21err != nil || i21ready.State != SDongleOfflineReady || i21ready.Retained.EvaluatedAt != time.Unix(20, 0) || !i21ready.DefaultDenied() {
			i21t.Fatalf("%+v %v", i21ready, i21err)
		}
		if !reflect.DeepEqual(i21ready.Decision.Evidence, i21Plan(i21child).Evidence) || i21ready.Decision.Observation != i21Plan(i21child).Observation || i21ready.Decision.Bounds != i21Plan(i21child).Bounds {
			i21t.Fatal("native fields lost")
		}
		i21plan.Evidence.References[0] = "supplier-change"
		i21ready.Decision.Evidence.References[0] = "output-change"
		if i21ready.Retained.Decision.Evidence.References[0] != "synthetic-native-record" {
			i21t.Fatal("decision/history alias")
		}
		i21ready.Retained.Decision.Evidence.References[0] = "history-change"
		i21snapshot := i21owner.I21Snapshot()
		if i21snapshot.Decision.Evidence.References[0] != "synthetic-native-record" || i21snapshot.Retained.Decision.Evidence.References[0] != "synthetic-native-record" {
			i21t.Fatal("stored alias")
		}
		i21snapshot.Decision.Evidence.References[0] = "snapshot-change"
		i21snapshot.Retained.Decision.Evidence.References[0] = "snapshot-history-change"
		for _, i21next := range []string{"error", "blocked", "denied"} {
			i21mode = i21next
			i21result, i21failure := i21owner.I21Refresh(context.Background(), i21Plan(i21child).Key, time.Unix(30, 0))
			if (i21failure != nil) != (i21next == "error") || i21result.State == SDongleOfflineReady || !i21result.DefaultDenied() || i21result.Retained.EvaluatedAt != time.Unix(20, 0) || i21result.Retained.Decision.Evidence.References[0] != "synthetic-native-record" || i21result.Retained.Decision.Key != i21Plan(i21child).Key {
				i21t.Fatalf("history changed: %+v %v", i21result, i21failure)
			}
			if i21next == "error" && i21result.Decision != nil {
				i21t.Fatal("failed current decision")
			}
		}
		if i21calls != 4 {
			i21t.Fatal(i21calls)
		}
		i21owner.I21Retire()
		i21snapshot = i21owner.I21Snapshot()
		if i21snapshot.State != SDongleOfflineBlocked || i21snapshot.Decision != nil || i21snapshot.Retained.EvaluatedAt != time.Unix(20, 0) {
			i21t.Fatal("retirement lost history")
		}
	}
}

func TestI21InFlightFences(i21t *testing.T) {
	for _, i21mode := range []string{"retire", "cancel", "deadline"} {
		i21t.Run(i21mode, func(i21t *testing.T) {
			i21plan := i21Plan(true)
			i21req := i21Request(i21plan)
			if i21mode == "deadline" {
				i21req.Bounds.LookupTimeout = 20 * time.Millisecond
			}
			i21req.Previous = &SDongleOfflineRecord{Decision: DecideSDongleRoute(i21Plan(true)), EvaluatedAt: time.Unix(1, 0)}
			i21entered := make(chan context.Context, 1)
			i21release := make(chan struct{})
			i21done := make(chan error, 1)
			i21calls := 0
			i21owner, _ := I21NewSDonglePort(i21req, func(i21ctx context.Context, _ SDongleRouteKey) (SDongleRoutePlan, error) {
				i21calls++
				i21entered <- i21ctx
				<-i21release
				return i21plan, nil
			})
			i21ctx, i21cancel := context.WithCancel(context.Background())
			defer i21cancel()
			go func() {
				i21result, i21err := i21owner.I21Refresh(i21ctx, i21plan.Key, time.Unix(20, 0))
				if i21result.State != SDongleOfflineBlocked || i21result.Decision != nil || i21result.Retained.EvaluatedAt != time.Unix(1, 0) {
					i21t.Errorf("late completion published: %+v", i21result)
				}
				i21done <- i21err
			}()
			i21work := <-i21entered
			if _, i21err := i21owner.I21Refresh(context.Background(), i21plan.Key, time.Unix(30, 0)); !errors.Is(i21err, ErrI21Busy) {
				i21t.Fatal(i21err)
			}
			i21want := context.Canceled
			switch i21mode {
			case "retire":
				i21owner.I21Retire()
				i21want = ErrI21Retired
			case "cancel":
				i21cancel()
			case "deadline":
				<-i21work.Done()
				i21want = context.DeadlineExceeded
			}
			close(i21release)
			if i21err := <-i21done; !errors.Is(i21err, i21want) {
				i21t.Fatalf("got %v want %v", i21err, i21want)
			}
			i21snapshot := i21owner.I21Snapshot()
			if i21calls != 1 || i21snapshot.Decision != nil || i21snapshot.State != SDongleOfflineBlocked || i21snapshot.Retained.EvaluatedAt != time.Unix(1, 0) || !i21snapshot.DefaultDenied() {
				i21t.Fatalf("published: %+v calls=%d", i21snapshot, i21calls)
			}
		})
	}
}

func TestI21NativeControls(i21t *testing.T) {
	for i21name, i21change := range map[string]func(*SDongleRoutePlan){
		"missing-child-qualification": func(i21p *SDongleRoutePlan) { i21p.Key.Child.Qualification = ""; i21p.Evidence.Key = i21p.Key },
		"missing-child-route":         func(i21p *SDongleRoutePlan) { i21p.Evidence.Route = SDongleRouteReference{} },
		"wrong-parent-observation":    func(i21p *SDongleRoutePlan) { i21p.Observation.Model = "synthetic-other" },
		"conflict":                    func(i21p *SDongleRoutePlan) { i21p.Evidence.Conflict = true },
		"native-bounds":               func(i21p *SDongleRoutePlan) { i21p.Bounds.MaxBytes = 0 },
	} {
		i21t.Run(i21name, func(i21t *testing.T) {
			i21plan := i21Plan(true)
			i21change(&i21plan)
			i21calls := 0
			i21owner, i21err := I21NewSDonglePort(i21Request(i21plan), func(context.Context, SDongleRouteKey) (SDongleRoutePlan, error) { i21calls++; return i21plan, nil })
			if i21err != nil {
				i21t.Fatal(i21err)
			}
			i21result, i21err := i21owner.I21Refresh(context.Background(), i21plan.Key, time.Unix(20, 0))
			if i21err != nil || i21calls != 1 || i21result.State != SDongleOfflineBlocked || i21result.Decision == nil || i21result.Decision.Outcome != SDongleWithheld || i21result.Retained != nil || !i21result.DefaultDenied() {
				i21t.Fatalf("native validation bypassed: %+v %v", i21result, i21err)
			}
		})
	}
}

func TestI21ReplacementAndZero(i21t *testing.T) {
	i21plan := i21Plan(false)
	i21calls := 0
	i21owner, _ := I21NewSDonglePort(i21Request(i21plan), func(context.Context, SDongleRouteKey) (SDongleRoutePlan, error) { i21calls++; return i21plan, nil })
	if i21snapshot := i21owner.I21Snapshot(); i21snapshot.State != SDongleOfflineBlocked || i21snapshot.Decision != nil || i21snapshot.Retained != nil {
		i21t.Fatal(i21snapshot)
	}
	for _, i21at := range []int64{20, 30, 40} {
		i21plan.Evidence.References = []string{"synthetic-replacement"}
		i21result, i21err := i21owner.I21Refresh(context.Background(), i21plan.Key, time.Unix(i21at, 0))
		if i21err != nil || i21result.State != SDongleOfflineReady || i21result.Retained.EvaluatedAt != time.Unix(i21at, 0) || i21result.Retained.Decision.Evidence.References[0] != "synthetic-replacement" || !i21result.DefaultDenied() {
			i21t.Fatalf("replacement: %+v %v", i21result, i21err)
		}
	}
	if i21calls != 3 {
		i21t.Fatal(i21calls)
	}
	var i21nilctx context.Context
	if _, i21err := i21owner.I21Refresh(i21nilctx, i21plan.Key, time.Unix(50, 0)); !errors.Is(i21err, ErrSDongleOfflineInput) || i21calls != 3 {
		i21t.Fatal("nil context reached supplier")
	}
	for _, i21zero := range []*I21SDonglePort{nil, {}} {
		if _, i21err := i21zero.I21Refresh(context.Background(), i21plan.Key, time.Unix(20, 0)); !errors.Is(i21err, ErrSDongleOfflineInput) {
			i21t.Fatal(i21err)
		}
		if i21zero.I21Snapshot().Decision != nil {
			i21t.Fatal("zero decision")
		}
		i21zero.I21Retire()
	}
}
