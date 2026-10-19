package modbusreg

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func sdongleOfflineRequest(p SDongleRoutePlan) SDongleOfflineRequest {
	return SDongleOfflineRequest{Key: p.Key, AttemptedAt: time.Unix(100, 0), Bounds: SDongleOfflineBounds{LookupTimeout: time.Second, MaxInputBytes: 8192, MaxReferences: 16}}
}

func TestSdongleOfflineProviderReady(t *testing.T) {
	for _, p := range []SDongleRoutePlan{routePlan(), childRoutePlan()} {
		calls := 0
		req := sdongleOfflineRequest(p)
		r, err := RunSDongleOfflineProvider(context.Background(), req, func(ctx context.Context, k SDongleRouteKey) (SDongleRoutePlan, error) {
			calls++
			if k != req.Key {
				t.Fatal("lookup key changed")
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("missing lookup deadline")
			}
			return p, nil
		})
		if err != nil || calls != 1 || r.State != SDongleOfflineReady || r.Decision == nil || r.Decision.Outcome != SDonglePlanned || r.Retained == nil || r.Retained.EvaluatedAt != req.AttemptedAt || !r.DefaultDenied() {
			t.Fatalf("%+v %v calls=%d", r, err, calls)
		}
		if !reflect.DeepEqual(r.Decision.Evidence, p.Evidence) {
			t.Fatal("native evidence lost")
		}
		p.Evidence.References[0] = "caller-change"
		if r.Decision.Evidence.References[0] != "synthetic-native-observation" {
			t.Fatal("reader slice alias")
		}
		r.Decision.Evidence.References[0] = "decision-change"
		if r.Retained.Decision.Evidence.References[0] != "synthetic-native-observation" {
			t.Fatal("retained slice alias")
		}
	}
	if !(SDongleOfflineResult{}).DefaultDenied() {
		t.Fatal("zero result admitted")
	}
}

// Change the request, leaving an internally valid returned plan untouched. This
// isolates the wrapper association fence from the route evaluator's own checks.
func TestSdongleOfflineProviderKeyAxes(t *testing.T) {
	var walk func(reflect.Type, []int, string)
	walk = func(typ reflect.Type, path []int, prefix string) {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			indices := append(append([]int{}, path...), i)
			name := prefix + field.Name
			if field.Type.Kind() == reflect.Struct {
				walk(field.Type, indices, name+".")
				continue
			}
			t.Run(name, func(t *testing.T) {
				p := childRoutePlan()
				req := sdongleOfflineRequest(p)
				v := reflect.ValueOf(&req.Key).Elem().FieldByIndex(indices)
				if v.Kind() == reflect.String {
					v.SetString(v.String() + "-other")
				} else {
					v.SetUint(v.Uint() + 1)
				}
				calls := 0
				r, err := RunSDongleOfflineProvider(context.Background(), req, func(context.Context, SDongleRouteKey) (SDongleRoutePlan, error) { calls++; return p, nil })
				if !errors.Is(err, ErrSDongleOfflineAssociation) || calls != 1 || r.State != SDongleOfflineBlocked || r.Decision != nil || r.Retained != nil {
					t.Fatalf("%+v %v calls=%d", r, err, calls)
				}
			})
		}
	}
	walk(reflect.TypeOf(SDongleRouteKey{}), nil, "")
}

func TestSdongleOfflineProviderControls(t *testing.T) {
	for name, tc := range map[string]struct {
		change  func(*SDongleRoutePlan)
		state   SDongleOfflineState
		outcome SDongleRouteOutcome
	}{
		"stale":        {func(p *SDongleRoutePlan) { p.Evidence.Current = false }, SDongleOfflineBlocked, SDongleWithheld},
		"conflict":     {func(p *SDongleRoutePlan) { p.Evidence.Conflict = true }, SDongleOfflineBlocked, SDongleWithheld},
		"scope":        {func(p *SDongleRoutePlan) { p.Key.Scope = "other"; p.Evidence.Key = p.Key }, SDongleOfflineDenied, SDongleNoSend},
		"guard":        {func(p *SDongleRoutePlan) { p.Key.Child.GuardAfter++; p.Evidence.Key = p.Key }, SDongleOfflineBlocked, SDongleWithheld},
		"private":      {func(p *SDongleRoutePlan) { p.Key.Request = SDonglePrivateFunction; p.Evidence.Key = p.Key }, SDongleOfflineDenied, SDongleNoSend},
		"stopped":      {func(p *SDongleRoutePlan) { p.QualificationStopped = true }, SDongleOfflineDenied, SDongleNoSend},
		"timeout":      {func(p *SDongleRoutePlan) { p.TimedOut = true }, SDongleOfflineDenied, SDongleNoSend},
		"evidence-key": {func(p *SDongleRoutePlan) { p.Evidence.Key.Unit++ }, SDongleOfflineBlocked, SDongleWithheld},
	} {
		t.Run(name, func(t *testing.T) {
			p := childRoutePlan()
			tc.change(&p)
			r, err := RunSDongleOfflineProvider(context.Background(), sdongleOfflineRequest(p), func(context.Context, SDongleRouteKey) (SDongleRoutePlan, error) { return p, nil })
			if err != nil || r.State != tc.state || r.Decision == nil || r.Decision.Outcome != tc.outcome || r.Retained != nil || !r.DefaultDenied() {
				t.Fatalf("%+v %v", r, err)
			}
		})
	}
}

func TestSdongleOfflineProviderInvalid(t *testing.T) {
	for name, change := range map[string]func(*SDongleOfflineRequest){
		"timeout-zero":     func(r *SDongleOfflineRequest) { r.Bounds.LookupTimeout = 0 },
		"timeout-negative": func(r *SDongleOfflineRequest) { r.Bounds.LookupTimeout = -1 },
		"timeout-excess":   func(r *SDongleOfflineRequest) { r.Bounds.LookupTimeout = 16 * time.Second },
		"bytes-zero":       func(r *SDongleOfflineRequest) { r.Bounds.MaxInputBytes = 0 },
		"bytes-excess":     func(r *SDongleOfflineRequest) { r.Bounds.MaxInputBytes = 1 << 21 },
		"refs-zero":        func(r *SDongleOfflineRequest) { r.Bounds.MaxReferences = 0 },
		"refs-excess":      func(r *SDongleOfflineRequest) { r.Bounds.MaxReferences = 257 },
		"key-excess":       func(r *SDongleOfflineRequest) { r.Key.Model = strings.Repeat("x", 8193) },
		"attempt-zero":     func(r *SDongleOfflineRequest) { r.AttemptedAt = time.Time{} },
	} {
		t.Run(name, func(t *testing.T) {
			req := sdongleOfflineRequest(routePlan())
			change(&req)
			calls := 0
			r, err := RunSDongleOfflineProvider(context.Background(), req, func(context.Context, SDongleRouteKey) (SDongleRoutePlan, error) { calls++; return routePlan(), nil })
			if !errors.Is(err, ErrSDongleOfflineInput) || calls != 0 || r.Decision != nil {
				t.Fatalf("%+v %v calls=%d", r, err, calls)
			}
		})
	}
	_, err := RunSDongleOfflineProvider(context.Background(), sdongleOfflineRequest(routePlan()), nil)
	if !errors.Is(err, ErrSDongleOfflineInput) {
		t.Fatal(err)
	}
	_, err = RunSDongleOfflineProvider(nil, sdongleOfflineRequest(routePlan()), func(context.Context, SDongleRouteKey) (SDongleRoutePlan, error) { //nolint:staticcheck // Intentional nil context verifies rejection before lookup.
		t.Fatal("nil context lookup")
		return SDongleRoutePlan{}, nil
	})
	if !errors.Is(err, ErrSDongleOfflineInput) {
		t.Fatal(err)
	}
}

func TestSdongleOfflineProviderRetentionAndFences(t *testing.T) {
	p := routePlan()
	old := SDongleOfflineRecord{Decision: DecideSDongleRoute(p), EvaluatedAt: time.Unix(10, 0)}
	for _, mode := range []string{"error", "cancel-before", "cancel-after", "deadline-before", "deadline-after", "blocked", "denied", "association", "oversize", "references"} {
		t.Run(mode, func(t *testing.T) {
			req := sdongleOfflineRequest(p)
			req.Previous = &old
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel-before" {
				cancel()
			}
			if mode == "deadline-before" {
				var stop context.CancelFunc
				ctx, stop = context.WithDeadline(context.Background(), time.Unix(1, 0))
				defer stop()
			}
			calls := 0
			boom := errors.New("synthetic lookup failure")
			r, err := RunSDongleOfflineProvider(ctx, req, func(c context.Context, _ SDongleRouteKey) (SDongleRoutePlan, error) {
				calls++
				candidate := routePlan()
				switch mode {
				case "error":
					return candidate, boom
				case "cancel-after":
					cancel()
				case "deadline-after":
					<-c.Done()
				case "blocked":
					candidate.Evidence.Current = false
				case "denied":
					candidate.TimedOut = true
				case "association":
					candidate.Key.Unit++
				case "oversize":
					candidate.Observation.Model = strings.Repeat("x", 8193)
				case "references":
					candidate.Evidence.References = make([]string, 17)
				}
				return candidate, nil
			})
			wantCalls := 1
			if mode == "cancel-before" || mode == "deadline-before" {
				wantCalls = 0
			}
			wantErr := mode != "blocked" && mode != "denied"
			if calls != wantCalls || (err != nil) != wantErr || r.State == SDongleOfflineReady || !reflect.DeepEqual(r.Retained, &old) {
				t.Fatalf("%+v %v calls=%d", r, err, calls)
			}
			if wantErr && r.Decision != nil {
				t.Fatal("failed attempt committed decision")
			}
			if mode == "error" && !errors.Is(err, boom) {
				t.Fatal("lookup error lost")
			}
			if strings.HasPrefix(mode, "cancel") && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if strings.HasPrefix(mode, "deadline") && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
			r.Retained.Decision.Evidence.References[0] = "returned-change"
			if old.Decision.Evidence.References[0] != "synthetic-native-observation" {
				t.Fatal("previous slice alias")
			}
		})
	}
}

func TestSdongleOfflineProviderCancellationBarrier(t *testing.T) {
	p := childRoutePlan()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan SDongleOfflineResult)
	go func() {
		r, err := RunSDongleOfflineProvider(ctx, sdongleOfflineRequest(p), func(context.Context, SDongleRouteKey) (SDongleRoutePlan, error) {
			close(entered)
			<-release
			return p, nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("post-reader fence: %v", err)
		}
		finished <- r
	}()
	<-entered
	cancel()
	close(release)
	r := <-finished
	if r.State != SDongleOfflineBlocked || r.Decision != nil || r.Retained != nil {
		t.Fatalf("late record committed: %+v", r)
	}
}

func TestSdongleOfflineProviderPreviousValidation(t *testing.T) {
	for name, change := range map[string]func(*SDongleOfflineRecord){
		"not-planned":  func(p *SDongleOfflineRecord) { p.Decision.Outcome = SDongleWithheld },
		"time-absent":  func(p *SDongleOfflineRecord) { p.EvaluatedAt = time.Time{} },
		"bytes-excess": func(p *SDongleOfflineRecord) { p.Decision.Evidence.Identity.Reference = strings.Repeat("x", 8193) },
		"refs-excess":  func(p *SDongleOfflineRecord) { p.Decision.Evidence.References = make([]string, 17) },
	} {
		t.Run(name, func(t *testing.T) {
			old := SDongleOfflineRecord{Decision: DecideSDongleRoute(routePlan()), EvaluatedAt: time.Unix(10, 0)}
			change(&old)
			req := sdongleOfflineRequest(routePlan())
			req.Previous = &old
			r, err := RunSDongleOfflineProvider(context.Background(), req, func(context.Context, SDongleRouteKey) (SDongleRoutePlan, error) {
				t.Fatal("invalid previous reached lookup")
				return routePlan(), nil
			})
			if !errors.Is(err, ErrSDongleOfflineInput) || r.Decision != nil || r.Retained != nil {
				t.Fatalf("%+v %v", r, err)
			}
		})
	}
}

func TestSdongleOfflineProviderHistoryIsolation(t *testing.T) {
	p := childRoutePlan()
	old := SDongleOfflineRecord{Decision: DecideSDongleRoute(routePlan()), EvaluatedAt: time.Unix(10, 0)}
	expected := SDongleOfflineRecord{Decision: DecideSDongleRoute(routePlan()), EvaluatedAt: time.Unix(10, 0)}
	req := sdongleOfflineRequest(p)
	req.Previous = &old
	r, err := RunSDongleOfflineProvider(context.Background(), req, func(context.Context, SDongleRouteKey) (SDongleRoutePlan, error) {
		old.Decision.Evidence.References[0] = "changed during lookup"
		p.Evidence.Current = false
		return p, nil
	})
	if err != nil || r.State != SDongleOfflineBlocked || !reflect.DeepEqual(r.Retained, &expected) || r.Retained.Decision.Key.Role != SDongleGateway || r.Decision.Key.Role != SDongleChild {
		t.Fatalf("history relabeled: %+v %v", r, err)
	}
}

func TestSdongleOfflineProviderEmptyKey(t *testing.T) {
	for _, name := range []string{"Family", "Model", "Firmware", "Role", "Request", "Scope", "Revision"} {
		t.Run(name, func(t *testing.T) {
			req := sdongleOfflineRequest(routePlan())
			reflect.ValueOf(&req.Key).Elem().FieldByName(name).SetString("")
			_, err := RunSDongleOfflineProvider(context.Background(), req, func(context.Context, SDongleRouteKey) (SDongleRoutePlan, error) {
				t.Fatal("incomplete key reached lookup")
				return SDongleRoutePlan{}, nil
			})
			if !errors.Is(err, ErrSDongleOfflineInput) {
				t.Fatal(err)
			}
		})
	}
}
