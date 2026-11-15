package fronius

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/internal/sunspecfixture"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
)

func offlineRequest(k RetainedKey, o SunSpecQualificationObservation) OfflineRequest {
	return OfflineRequest{Key: k, PollGeneration: o.SampleIdentity().PollGeneration(), DeadlineIdentity: o.SampleIdentity().DeadlineIdentity(), MaxWords: 512, MaxOccurrences: 64, MaxSourceViews: 512}
}

func offlineContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func offlineReader(o SunSpecQualificationObservation) OfflineReader {
	return func(_ context.Context, request OfflineReadRequest) (OfflineReadResult, error) {
		return OfflineReadResult{Attempt: request.Attempt, Observation: o}, nil
	}
}

func TestFroniusOfflineProviderCycle(t *testing.T) {
	r, o, k := retainedFixture(t)
	p := new(OfflineProvider)
	ctx := offlineContext(t)
	req := offlineRequest(k, o)
	calls := 0
	err := p.Run(ctx, r, req, func(got context.Context, read OfflineReadRequest) (OfflineReadResult, error) {
		calls++
		deadline, _ := ctx.Deadline()
		if got != ctx || read.Attempt.Key != k || read.Attempt.PollGeneration != req.PollGeneration || read.Attempt.DeadlineIdentity != req.DeadlineIdentity || read.Attempt.Deadline != deadline || read.Attempt.Token == (RetainedToken{}) || read.MaxWords != 512 || read.MaxOccurrences != 64 || read.MaxSourceViews != 512 {
			t.Fatal("reader did not receive exact caller identity and bounds")
		}
		return OfflineReadResult{Attempt: read.Attempt, Observation: o}, nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("Run: %v, calls %d", err, calls)
	}
	before := p.View()
	if before.Envelope == nil || before.Stale || before.Outcome != RetainedSucceeded || !reflect.DeepEqual(before.Envelope.Observation, o) {
		t.Fatal("complete native observation was not committed")
	}
	for _, mode := range []string{"error with valid result", "partial", "rejected", "words bound", "occurrences bound", "sources bound"} {
		t.Run(mode, func(t *testing.T) {
			candidate := cloneFroniusRetainedObservation(o)
			request := req
			var readErr error
			switch mode {
			case "error with valid result":
				readErr = errors.New("synthetic failure")
			case "partial":
				candidate.snapshot = sunspecfixture.KeepSourceViews(t, candidate.snapshot, candidate.snapshot.SourceViewCount()-1)
			case "rejected":
				candidate = SunSpecQualificationObservation{}
			case "words bound":
				request.MaxWords = 1
			case "occurrences bound":
				request.MaxOccurrences = 1
			case "sources bound":
				request.MaxSourceViews = 1
			}
			if err := p.Run(ctx, r, request, func(_ context.Context, read OfflineReadRequest) (OfflineReadResult, error) {
				return OfflineReadResult{Attempt: read.Attempt, Observation: candidate}, readErr
			}); err == nil {
				t.Fatal("failed refresh accepted")
			}
			got := p.View()
			if !got.Stale || got.Outcome != RetainedFailed || !reflect.DeepEqual(got.Envelope, before.Envelope) {
				t.Fatal("failure changed retained native evidence, original attempt or age")
			}
		})
	}
	view := p.View()
	view.Envelope.Observation.snapshot = mutatedSnapshot(t, view.Envelope.Observation.snapshot)
	o.snapshot = mutatedSnapshot(t, o.snapshot)
	if !reflect.DeepEqual(p.View().Envelope, before.Envelope) {
		t.Fatal("caller observation or view aliases retained slices")
	}
}

func offlineAttemptAxes() map[string]func(*RetainedAttempt) {
	return map[string]func(*RetainedAttempt){
		"RequestedFunction":   func(a *RetainedAttempt) { a.Key.RequestedFunction++ },
		"ReceivedFunction":    func(a *RetainedAttempt) { a.Key.ReceivedFunction++ },
		"Table":               func(a *RetainedAttempt) { a.Key.Table = modbusreg.InputRegisters },
		"UnitID":              func(a *RetainedAttempt) { a.Key.UnitID++ },
		"Source":              func(a *RetainedAttempt) { a.Key.Source = "different-synthetic-source" },
		"ConnectionID":        func(a *RetainedAttempt) { a.Key.ConnectionID++ },
		"Transport":           func(a *RetainedAttempt) { a.Key.Transport = modbusreg.TransportRTU },
		"TransportGeneration": func(a *RetainedAttempt) { a.Key.TransportGeneration++ },
		"CardID":              func(a *RetainedAttempt) { a.Key.CardID = "different-card" },
		"Schema":              func(a *RetainedAttempt) { a.Key.Schema = "different-schema" },
		"PollGeneration":      func(a *RetainedAttempt) { a.PollGeneration++ },
		"DeadlineIdentity":    func(a *RetainedAttempt) { a.DeadlineIdentity++ },
		"Deadline":            func(a *RetainedAttempt) { a.Deadline = a.Deadline.Add(time.Second) },
		"Token":               func(a *RetainedAttempt) { a.Token = RetainedToken{} },
	}
}

func TestFroniusOfflineProviderCallbackIdentity(t *testing.T) {
	r, o, k := retainedFixture(t)
	for name, change := range offlineAttemptAxes() {
		t.Run(name, func(t *testing.T) {
			p := new(OfflineProvider)
			var foreign RetainedToken
			if name == "Token" {
				owner := new(RetainedCommit)
				foreign = retainedBegin(t, owner, k, o)
			}
			err := p.Run(offlineContext(t), r, offlineRequest(k, o), func(_ context.Context, read OfflineReadRequest) (OfflineReadResult, error) {
				// Only the callback identity changes; valid native evidence stays fixed.
				attempt := read.Attempt
				change(&attempt)
				if name == "Token" {
					attempt.Token = foreign
				}
				return OfflineReadResult{Attempt: attempt, Observation: o}, nil
			})
			if err == nil || p.View().Envelope != nil || p.View().Outcome != RetainedFailed {
				t.Fatal("mismatched callback identity accepted")
			}
		})
	}
}

func TestFroniusOfflineProviderKeyChange(t *testing.T) {
	r, o, k := retainedFixture(t)
	for name, change := range offlineAttemptAxes() {
		if name == "PollGeneration" || name == "DeadlineIdentity" || name == "Deadline" || name == "Token" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			p := new(OfflineProvider)
			ctx := offlineContext(t)
			if err := p.Run(ctx, r, offlineRequest(k, o), offlineReader(o)); err != nil {
				t.Fatal(err)
			}
			a := RetainedAttempt{Key: k}
			change(&a)
			if err := p.Run(ctx, r, offlineRequest(a.Key, o), offlineReader(o)); err == nil {
				t.Fatal("old evidence accepted under new key")
			}
			if p.View().Envelope != nil {
				t.Fatal("equal values preserved incompatible history")
			}
			if err := p.Run(ctx, r, offlineRequest(k, o), func(context.Context, OfflineReadRequest) (OfflineReadResult, error) {
				return OfflineReadResult{}, errors.New("synthetic failure")
			}); err == nil {
				t.Fatal("failure accepted")
			}
			if p.View().Envelope != nil {
				t.Fatal("return to original key resurrected history")
			}
		})
	}
}

func TestFroniusOfflineProviderNativeAttempt(t *testing.T) {
	r, o, k := retainedFixture(t)
	for _, axis := range []string{"PollGeneration", "DeadlineIdentity"} {
		t.Run(axis, func(t *testing.T) {
			p := new(OfflineProvider)
			req := offlineRequest(k, o)
			if axis == "PollGeneration" {
				req.PollGeneration++
			} else {
				req.DeadlineIdentity++
			}
			// Echo the correct local attempt, but leave native sample identity old.
			if err := p.Run(offlineContext(t), r, req, offlineReader(o)); err == nil || p.View().Envelope != nil {
				t.Fatal("local attempt rewrote native sample identity")
			}
		})
	}
}

func TestFroniusOfflineProviderReplacement(t *testing.T) {
	r, o, k := retainedFixture(t)
	p := new(OfflineProvider)
	ctx := offlineContext(t)
	if err := p.Run(ctx, r, offlineRequest(k, o), offlineReader(o)); err != nil {
		t.Fatal(err)
	}
	first := p.View().Envelope
	snapshot := sunspecfixture.ShiftSampleIdentity(t, o.snapshot, 1, 1)
	next, err := NewSunSpecFroniusQualificationCard().Qualify(r, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Run(ctx, r, offlineRequest(k, next), offlineReader(next)); err != nil {
		t.Fatal(err)
	}
	got := p.View()
	if got.Stale || got.Outcome != RetainedSucceeded || got.Envelope.Attempt.Token == first.Attempt.Token || got.Envelope.Attempt.PollGeneration != o.identity.pollGeneration+1 || got.Envelope.Attempt.DeadlineIdentity != o.identity.deadlineIdentity+1 || !reflect.DeepEqual(got.Envelope.Observation, next) {
		t.Fatal("complete replacement mixed envelopes or attempt provenance")
	}
}

func TestFroniusOfflineProviderInvalidBeforeReader(t *testing.T) {
	r, o, k := retainedFixture(t)
	for _, mode := range []string{"nil context", "no deadline", "cancelled", "expired", "poll", "deadline identity", "source", "connection", "generation", "card", "schema", "function", "received", "table", "unit", "transport", "words zero", "words high", "occurrences zero", "occurrences high", "sources zero", "sources high", "registry", "nil reader"} {
		t.Run(mode, func(t *testing.T) {
			ctx := offlineContext(t)
			req := offlineRequest(k, o)
			registry := r
			calls := 0
			reader := OfflineReader(func(context.Context, OfflineReadRequest) (OfflineReadResult, error) {
				calls++
				return OfflineReadResult{}, nil
			})
			switch mode {
			case "nil context":
				ctx = nil
			case "no deadline":
				ctx = context.Background()
			case "cancelled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			case "expired":
				c, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer cancel()
				ctx = c
			case "poll":
				req.PollGeneration = 0
			case "deadline identity":
				req.DeadlineIdentity = 0
			case "source":
				req.Key.Source = ""
			case "connection":
				req.Key.ConnectionID = 0
			case "generation":
				req.Key.TransportGeneration = 0
			case "card":
				req.Key.CardID = "unsupported-card"
			case "schema":
				req.Key.Schema = "unsupported-schema"
			case "function":
				req.Key.RequestedFunction++
			case "received":
				req.Key.ReceivedFunction++
			case "table":
				req.Key.Table = modbusreg.InputRegisters
			case "unit":
				req.Key.UnitID++
			case "transport":
				req.Key.Transport = ""
			case "words zero":
				req.MaxWords = 0
			case "words high":
				req.MaxWords = 513
			case "occurrences zero":
				req.MaxOccurrences = 0
			case "occurrences high":
				req.MaxOccurrences = 65
			case "sources zero":
				req.MaxSourceViews = 0
			case "sources high":
				req.MaxSourceViews = 513
			case "registry":
				registry = sunspec.DecoderRegistry{}
			case "nil reader":
				reader = nil
			}
			p := new(OfflineProvider)
			if err := p.Run(ctx, registry, req, reader); err == nil || calls != 0 || p.View().Envelope != nil {
				t.Fatalf("invalid input: err %v, calls %d", err, calls)
			}
		})
	}
}

func TestFroniusOfflineProviderCancellationAndSupersession(t *testing.T) {
	r, o, k := retainedFixture(t)
	for _, mode := range []string{"cancel", "context", "supersede", "changed key"} {
		t.Run(mode, func(t *testing.T) {
			p := new(OfflineProvider)
			ctx, cancel := context.WithCancel(offlineContext(t))
			defer cancel()
			req := offlineRequest(k, o)
			if err := p.Run(ctx, r, req, offlineReader(o)); err != nil {
				t.Fatal(err)
			}
			original := p.View().Envelope
			entered := make(chan RetainedAttempt, 1)
			release := make(chan struct{})
			done := make(chan error, 1)
			go func() {
				done <- p.Run(ctx, r, req, func(_ context.Context, read OfflineReadRequest) (OfflineReadResult, error) {
					entered <- read.Attempt
					<-release
					return OfflineReadResult{Attempt: read.Attempt, Observation: o}, nil
				})
			}()
			attempt := <-entered
			switch mode {
			case "cancel":
				if err := p.Cancel(attempt.Token); err != nil {
					t.Fatal(err)
				}
			case "context":
				cancel()
			case "supersede":
				if err := p.Run(ctx, r, req, offlineReader(o)); err != nil {
					t.Fatal(err)
				}
			case "changed key":
				req.Key.ConnectionID++
				_ = p.Run(ctx, r, req, offlineReader(o))
			}
			winner := p.View()
			close(release)
			if err := <-done; err == nil {
				t.Fatal("late completion accepted")
			} else if mode == "context" && !errors.Is(err, context.Canceled) {
				t.Fatalf("post-reader context fence did not report cancellation: %v", err)
			}
			got := p.View()
			if !reflect.DeepEqual(got, winner) {
				t.Fatal("late completion changed winner")
			}
			if mode == "cancel" || mode == "context" {
				if got.Outcome != RetainedCancelled || !got.Stale || !reflect.DeepEqual(got.Envelope, original) {
					t.Fatal("cancel renewed retained evidence")
				}
			}
			if mode == "changed key" && got.Envelope != nil {
				t.Fatal("key change retained incompatible evidence")
			}
		})
	}
}
