package modbusreg

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func offlineRequest(k FroniusRetainedKey, o SunSpecQualificationObservation) FroniusOfflineRequest {
	return FroniusOfflineRequest{Key: k, PollGeneration: o.SampleIdentity().PollGeneration(), DeadlineIdentity: o.SampleIdentity().DeadlineIdentity(), MaxWords: 512, MaxOccurrences: 64, MaxSourceViews: 512}
}

func offlineContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func offlineReader(o SunSpecQualificationObservation) FroniusOfflineReader {
	return func(_ context.Context, request FroniusOfflineReadRequest) (FroniusOfflineReadResult, error) {
		return FroniusOfflineReadResult{Attempt: request.Attempt, Observation: o}, nil
	}
}

func TestFroniusOfflineProviderCycle(t *testing.T) {
	r, o, k := retainedFixture(t)
	p := new(FroniusOfflineProvider)
	ctx := offlineContext(t)
	req := offlineRequest(k, o)
	calls := 0
	err := p.Run(ctx, r, req, func(got context.Context, read FroniusOfflineReadRequest) (FroniusOfflineReadResult, error) {
		calls++
		deadline, _ := ctx.Deadline()
		if got != ctx || read.Attempt.Key != k || read.Attempt.PollGeneration != req.PollGeneration || read.Attempt.DeadlineIdentity != req.DeadlineIdentity || read.Attempt.Deadline != deadline || read.Attempt.Token == (FroniusRetainedToken{}) || read.MaxWords != 512 || read.MaxOccurrences != 64 || read.MaxSourceViews != 512 {
			t.Fatal("reader did not receive exact caller identity and bounds")
		}
		return FroniusOfflineReadResult{Attempt: read.Attempt, Observation: o}, nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("Run: %v, calls %d", err, calls)
	}
	before := p.View()
	if before.Envelope == nil || before.Stale || before.Outcome != FroniusRetainedSucceeded || !reflect.DeepEqual(before.Envelope.Observation, o) {
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
				candidate.snapshot.sources = candidate.snapshot.sources[:len(candidate.snapshot.sources)-1]
			case "rejected":
				candidate = SunSpecQualificationObservation{}
			case "words bound":
				request.MaxWords = 1
			case "occurrences bound":
				request.MaxOccurrences = 1
			case "sources bound":
				request.MaxSourceViews = 1
			}
			if err := p.Run(ctx, r, request, func(_ context.Context, read FroniusOfflineReadRequest) (FroniusOfflineReadResult, error) {
				return FroniusOfflineReadResult{Attempt: read.Attempt, Observation: candidate}, readErr
			}); err == nil {
				t.Fatal("failed refresh accepted")
			}
			got := p.View()
			if !got.Stale || got.Outcome != FroniusRetainedFailed || !reflect.DeepEqual(got.Envelope, before.Envelope) {
				t.Fatal("failure changed retained native evidence, original attempt or age")
			}
		})
	}
	view := p.View()
	view.Envelope.Observation.snapshot.raw[0]++
	view.Envelope.Observation.snapshot.sources[0].Words[0]++
	o.snapshot.raw[0]++
	o.snapshot.sources[0].Words[0]++
	if !reflect.DeepEqual(p.View().Envelope, before.Envelope) {
		t.Fatal("caller observation or view aliases retained slices")
	}
}

func offlineAttemptAxes() map[string]func(*FroniusRetainedAttempt) {
	return map[string]func(*FroniusRetainedAttempt){
		"RequestedFunction":   func(a *FroniusRetainedAttempt) { a.Key.RequestedFunction++ },
		"ReceivedFunction":    func(a *FroniusRetainedAttempt) { a.Key.ReceivedFunction++ },
		"Table":               func(a *FroniusRetainedAttempt) { a.Key.Table = InputRegisters },
		"UnitID":              func(a *FroniusRetainedAttempt) { a.Key.UnitID++ },
		"Source":              func(a *FroniusRetainedAttempt) { a.Key.Source = "different-synthetic-source" },
		"ConnectionID":        func(a *FroniusRetainedAttempt) { a.Key.ConnectionID++ },
		"Transport":           func(a *FroniusRetainedAttempt) { a.Key.Transport = TransportRTU },
		"TransportGeneration": func(a *FroniusRetainedAttempt) { a.Key.TransportGeneration++ },
		"CardID":              func(a *FroniusRetainedAttempt) { a.Key.CardID = "different-card" },
		"Schema":              func(a *FroniusRetainedAttempt) { a.Key.Schema = "different-schema" },
		"PollGeneration":      func(a *FroniusRetainedAttempt) { a.PollGeneration++ },
		"DeadlineIdentity":    func(a *FroniusRetainedAttempt) { a.DeadlineIdentity++ },
		"Deadline":            func(a *FroniusRetainedAttempt) { a.Deadline = a.Deadline.Add(time.Second) },
		"Token":               func(a *FroniusRetainedAttempt) { a.Token = FroniusRetainedToken{} },
	}
}

func TestFroniusOfflineProviderCallbackIdentity(t *testing.T) {
	r, o, k := retainedFixture(t)
	for name, change := range offlineAttemptAxes() {
		t.Run(name, func(t *testing.T) {
			p := new(FroniusOfflineProvider)
			var foreign FroniusRetainedToken
			if name == "Token" {
				owner := new(FroniusRetainedCommit)
				foreign = retainedBegin(t, owner, k, o)
			}
			err := p.Run(offlineContext(t), r, offlineRequest(k, o), func(_ context.Context, read FroniusOfflineReadRequest) (FroniusOfflineReadResult, error) {
				// Only the callback identity changes; valid native evidence stays fixed.
				attempt := read.Attempt
				change(&attempt)
				if name == "Token" {
					attempt.Token = foreign
				}
				return FroniusOfflineReadResult{Attempt: attempt, Observation: o}, nil
			})
			if err == nil || p.View().Envelope != nil || p.View().Outcome != FroniusRetainedFailed {
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
			p := new(FroniusOfflineProvider)
			ctx := offlineContext(t)
			if err := p.Run(ctx, r, offlineRequest(k, o), offlineReader(o)); err != nil {
				t.Fatal(err)
			}
			a := FroniusRetainedAttempt{Key: k}
			change(&a)
			if err := p.Run(ctx, r, offlineRequest(a.Key, o), offlineReader(o)); err == nil {
				t.Fatal("old evidence accepted under new key")
			}
			if p.View().Envelope != nil {
				t.Fatal("equal values preserved incompatible history")
			}
			if err := p.Run(ctx, r, offlineRequest(k, o), func(context.Context, FroniusOfflineReadRequest) (FroniusOfflineReadResult, error) {
				return FroniusOfflineReadResult{}, errors.New("synthetic failure")
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
			p := new(FroniusOfflineProvider)
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
	p := new(FroniusOfflineProvider)
	ctx := offlineContext(t)
	if err := p.Run(ctx, r, offlineRequest(k, o), offlineReader(o)); err != nil {
		t.Fatal(err)
	}
	first := p.View().Envelope
	snapshot := cloneSunSpecQualificationSnapshot(o.snapshot)
	for i := range snapshot.sources {
		snapshot.sources[i].PollGeneration++
		snapshot.sources[i].DeadlineIdentity++
	}
	next, err := NewSunSpecFroniusQualificationCard().Qualify(r, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Run(ctx, r, offlineRequest(k, next), offlineReader(next)); err != nil {
		t.Fatal(err)
	}
	got := p.View()
	if got.Stale || got.Outcome != FroniusRetainedSucceeded || got.Envelope.Attempt.Token == first.Attempt.Token || got.Envelope.Attempt.PollGeneration != o.identity.pollGeneration+1 || got.Envelope.Attempt.DeadlineIdentity != o.identity.deadlineIdentity+1 || !reflect.DeepEqual(got.Envelope.Observation, next) {
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
			reader := FroniusOfflineReader(func(context.Context, FroniusOfflineReadRequest) (FroniusOfflineReadResult, error) {
				calls++
				return FroniusOfflineReadResult{}, nil
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
				req.Key.Table = InputRegisters
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
				registry = SunSpecDecoderRegistry{}
			case "nil reader":
				reader = nil
			}
			p := new(FroniusOfflineProvider)
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
			p := new(FroniusOfflineProvider)
			ctx, cancel := context.WithCancel(offlineContext(t))
			defer cancel()
			req := offlineRequest(k, o)
			if err := p.Run(ctx, r, req, offlineReader(o)); err != nil {
				t.Fatal(err)
			}
			original := p.View().Envelope
			entered := make(chan FroniusRetainedAttempt, 1)
			release := make(chan struct{})
			done := make(chan error, 1)
			go func() {
				done <- p.Run(ctx, r, req, func(_ context.Context, read FroniusOfflineReadRequest) (FroniusOfflineReadResult, error) {
					entered <- read.Attempt
					<-release
					return FroniusOfflineReadResult{Attempt: read.Attempt, Observation: o}, nil
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
				if got.Outcome != FroniusRetainedCancelled || !got.Stale || !reflect.DeepEqual(got.Envelope, original) {
					t.Fatal("cancel renewed retained evidence")
				}
			}
			if mode == "changed key" && got.Envelope != nil {
				t.Fatal("key change retained incompatible evidence")
			}
		})
	}
}
