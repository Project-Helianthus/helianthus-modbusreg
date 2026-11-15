package fronius

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/internal/sunspecfixture"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
)

func retainedFixture(t *testing.T) (sunspec.DecoderRegistry, SunSpecQualificationObservation, RetainedKey) {
	t.Helper()
	r := sunspecfixture.Registry(t)
	card := NewSunSpecFroniusQualificationCard()
	raw := sunspecfixture.RawWords(froniusObservedSnapshotV11(t, r, card.Manufacturer(), card.Model(), card.Firmware()).Occurrences())
	s, err := replayFroniusQualificationCard(t, card, r, raw)
	if err != nil {
		t.Fatal(err)
	}
	o, err := card.Qualify(r, s)
	if err != nil {
		t.Fatal(err)
	}
	v := o.SourceViews()[0].Record()
	return r, o, RetainedKey{RequestedFunction: v.RequestedFunction, ReceivedFunction: v.ReceivedFunction, Table: v.Table, UnitID: v.UnitID, Source: v.Endpoint, ConnectionID: v.ConnectionID, Transport: v.Transport, TransportGeneration: v.TransportGeneration, CardID: card.ID(), Schema: card.SchemaRevision()}
}

// mutatedSnapshot returns snapshot rebuilt from its exported evidence with
// its first raw word and the first word of its first source view incremented.
// The SunSpec snapshot is immutable outside its package, so a caller can alter
// retained evidence only by replacing the snapshot value.
func mutatedSnapshot(t *testing.T, snapshot sunspec.ChainSnapshot) sunspec.ChainSnapshot {
	t.Helper()
	raw, records := snapshot.RawWords(), sunspecfixture.Records(snapshot)
	raw[0]++
	records[0].Words[0]++
	return sunspecfixture.Assemble(t, raw, snapshot.Occurrences(), records)
}

func retainedBegin(t *testing.T, s *RetainedCommit, k RetainedKey, o SunSpecQualificationObservation) RetainedToken {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	token, err := s.Begin(ctx, k, o.SampleIdentity().PollGeneration(), o.SampleIdentity().DeadlineIdentity())
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestFroniusRetainedCommitLifecycle(t *testing.T) {
	r, o, k := retainedFixture(t)
	s := new(RetainedCommit)
	first := retainedBegin(t, s, k, o)
	if s.View().Envelope != nil {
		t.Fatal("observation before first success")
	}
	if err := s.Commit(first, r, o); err != nil {
		t.Fatal(err)
	}
	original := s.View()
	if original.Envelope == nil || original.Stale || original.Outcome != RetainedSucceeded {
		t.Fatal("missing fresh success")
	}
	if !reflect.DeepEqual(original.Envelope.Observation, o) {
		t.Fatal("native observation changed")
	}
	token := retainedBegin(t, s, k, o)
	if !s.View().Stale {
		t.Fatal("refresh made history fresh")
	}
	if err := s.Fail(token); err != nil {
		t.Fatal(err)
	}
	got := s.View()
	if !got.Stale || got.Outcome != RetainedFailed || !reflect.DeepEqual(got.Envelope, original.Envelope) || got.Attempt.Token != token {
		t.Fatal("failed refresh changed original evidence/age/attempt")
	}
	got.Envelope.Observation.snapshot = mutatedSnapshot(t, got.Envelope.Observation.snapshot)
	if !reflect.DeepEqual(s.View().Envelope, original.Envelope) {
		t.Fatal("view aliases storage")
	}
	o.snapshot = mutatedSnapshot(t, o.snapshot)
	if !reflect.DeepEqual(s.View().Envelope, original.Envelope) {
		t.Fatal("commit aliases input")
	}
}

func TestFroniusRetainedCommitKeyAxes(t *testing.T) {
	r, o, k := retainedFixture(t)
	axes := map[string]func(*RetainedKey){
		"requested function":   func(k *RetainedKey) { k.RequestedFunction++ },
		"received function":    func(k *RetainedKey) { k.ReceivedFunction++ },
		"table":                func(k *RetainedKey) { k.Table = modbusreg.InputRegisters },
		"unit":                 func(k *RetainedKey) { k.UnitID++ },
		"source":               func(k *RetainedKey) { k.Source = "other-synthetic-source" },
		"connection":           func(k *RetainedKey) { k.ConnectionID++ },
		"transport":            func(k *RetainedKey) { k.Transport = modbusreg.TransportRTU },
		"transport generation": func(k *RetainedKey) { k.TransportGeneration++ },
		"card":                 func(k *RetainedKey) { k.CardID = "other-card" },
		"schema":               func(k *RetainedKey) { k.Schema = "other-schema" },
	}
	for name, change := range axes {
		t.Run(name, func(t *testing.T) {
			s := new(RetainedCommit)
			token := retainedBegin(t, s, k, o)
			if err := s.Commit(token, r, o); err != nil {
				t.Fatal(err)
			}
			changed := k
			change(&changed)
			token = retainedBegin(t, s, changed, o)
			if s.View().Envelope != nil {
				t.Fatal("changed key reused old history")
			}
			if err := s.Commit(token, r, o); err == nil {
				t.Fatal("candidate outside active key accepted")
			}
			token = retainedBegin(t, s, k, o)
			if s.View().Envelope != nil {
				t.Fatal("returning key resurrected history")
			}
			if err := s.Fail(token); err != nil {
				t.Fatal(err)
			}
			if s.View().Envelope != nil {
				t.Fatal("failure resurrected history")
			}
		})
	}
}

func TestFroniusRetainedCommitAttemptAxes(t *testing.T) {
	r, o, k := retainedFixture(t)
	for _, axis := range []string{"cycle", "poll", "deadline"} {
		t.Run(axis, func(t *testing.T) {
			s := new(RetainedCommit)
			token := retainedBegin(t, s, k, o)
			candidate := o
			candidate.snapshot = cloneSunSpecQualificationSnapshot(o.snapshot)
			switch axis {
			case "cycle":
				retainedBegin(t, s, k, o)
			case "poll", "deadline":
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				poll, deadline := o.identity.pollGeneration, o.identity.deadlineIdentity
				if axis == "poll" {
					poll++
				} else {
					deadline++
				}
				var err error
				token, err = s.Begin(ctx, k, poll, deadline)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Commit(token, r, candidate); err == nil {
				t.Fatal("mixed attempt committed")
			}
			if s.View().Envelope != nil {
				t.Fatal("mixed attempt published")
			}
		})
	}
}

func TestFroniusRetainedCommitRejectsIncompleteAndClosed(t *testing.T) {
	r, o, k := retainedFixture(t)
	for _, mode := range []string{"partial", "zero", "cancel", "context cancel", "expired", "expires while active", "superseded", "foreign"} {
		t.Run(mode, func(t *testing.T) {
			s := new(RetainedCommit)
			token := retainedBegin(t, s, k, o)
			candidate := o
			switch mode {
			case "partial":
				candidate.snapshot = sunspecfixture.KeepSourceViews(t, o.snapshot, o.snapshot.SourceViewCount()-1)
			case "zero":
				candidate = SunSpecQualificationObservation{}
			case "cancel":
				if err := s.Cancel(token); err != nil {
					t.Fatal(err)
				}
			case "context cancel":
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				var err error
				token, err = s.Begin(ctx, k, o.identity.pollGeneration, o.identity.deadlineIdentity)
				if err != nil {
					t.Fatal(err)
				}
				cancel()
			case "expired":
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				defer cancel()
				if _, err := s.Begin(ctx, k, 1, 1); err == nil {
					t.Fatal("expired begin accepted")
				}
			case "expires while active":
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
				defer cancel()
				var err error
				token, err = s.Begin(ctx, k, o.identity.pollGeneration, o.identity.deadlineIdentity)
				if err != nil {
					t.Fatal(err)
				}
				<-ctx.Done()
			case "superseded":
				retainedBegin(t, s, k, o)
			case "foreign":
				token = retainedBegin(t, new(RetainedCommit), k, o)
			}
			if err := s.Commit(token, r, candidate); err == nil {
				t.Fatal("invalid candidate published")
			}
			if s.View().Envelope != nil {
				t.Fatal("observation after rejected commit")
			}
		})
	}
	s := new(RetainedCommit)
	if _, err := s.Begin(context.Background(), k, 1, 1); err == nil {
		t.Fatal("unbounded context accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, ids := range [][2]uint64{{0, 1}, {1, 0}} {
		if _, err := s.Begin(ctx, k, ids[0], ids[1]); err == nil {
			t.Fatal("unknown attempt accepted")
		}
	}
}

func TestFroniusRetainedCommitCancellationOrdering(t *testing.T) {
	r, o, k := retainedFixture(t)
	for i := 0; i < 20; i++ {
		s := new(RetainedCommit)
		token := retainedBegin(t, s, k, o)
		var wg sync.WaitGroup
		wg.Add(2)
		start := make(chan struct{})
		go func() { defer wg.Done(); <-start; _ = s.Commit(token, r, o) }()
		go func() { defer wg.Done(); <-start; _ = s.Cancel(token) }()
		close(start)
		wg.Wait()
		v := s.View()
		if (v.Envelope == nil && v.Outcome != RetainedCancelled) || (v.Envelope != nil && v.Outcome != RetainedSucceeded) {
			t.Fatal("torn envelope/outcome")
		}
		if err := s.Commit(token, r, o); err == nil {
			t.Fatal("closed token reused")
		}
	}
}

func TestFroniusRetainedCommitReplacementAndFailureControls(t *testing.T) {
	r, o, k := retainedFixture(t)
	s := new(RetainedCommit)
	first := retainedBegin(t, s, k, o)
	if err := s.Commit(first, r, o); err != nil {
		t.Fatal(err)
	}
	before := s.View()
	// Detached replay with a new poll only; deadline identity stays unchanged.
	snapshot := sunspecfixture.ShiftSampleIdentity(t, o.snapshot, 1, 0)
	next, err := NewSunSpecFroniusQualificationCard().Qualify(r, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	token := retainedBegin(t, s, k, next)
	if err := s.Fail(first); err == nil {
		t.Fatal("old fail closed replacement")
	}
	if err := s.Cancel(first); err == nil {
		t.Fatal("old cancel closed replacement")
	}
	if err := s.Commit(token, r, next); err != nil {
		t.Fatal(err)
	}
	after := s.View()
	if !reflect.DeepEqual(after.Envelope.Observation, next) || after.Envelope.Attempt.Token != token || after.Stale {
		t.Fatal("replacement did not publish coherent envelope")
	}
	if !reflect.DeepEqual(before.Envelope.Observation, o) {
		t.Fatal("detached old view changed")
	}
	// Reject a partial refresh while preserving the new complete envelope.
	token = retainedBegin(t, s, k, next)
	partial := next
	partial.snapshot = sunspecfixture.KeepSourceViews(t, next.snapshot, 1)
	if err := s.Commit(token, r, partial); err == nil {
		t.Fatal("partial accepted")
	}
	if v := s.View(); !v.Stale || v.Outcome != RetainedFailed || !reflect.DeepEqual(v.Envelope, after.Envelope) {
		t.Fatal("partial replaced history")
	}
	token = retainedBegin(t, s, k, next)
	if err := s.Cancel(token); err != nil {
		t.Fatal(err)
	}
	if err := s.Commit(token, r, next); err == nil {
		t.Fatal("cancelled refresh committed")
	}
	if v := s.View(); !v.Stale || !reflect.DeepEqual(v.Envelope, after.Envelope) {
		t.Fatal("cancel changed history age or evidence")
	}
}

func TestFroniusRetainedCommitMixedViews(t *testing.T) {
	r, o, k := retainedFixture(t)
	for _, axis := range []string{"poll", "deadline", "connection", "generation"} {
		t.Run(axis, func(t *testing.T) {
			s := new(RetainedCommit)
			token := retainedBegin(t, s, k, o)
			bad := o
			bad.snapshot = sunspecfixture.WithRecords(t, o.snapshot, func(records []modbusreg.LogicalViewRecord) []modbusreg.LogicalViewRecord {
				v := &records[len(records)-1]
				switch axis {
				case "poll":
					v.PollGeneration++
				case "deadline":
					v.DeadlineIdentity++
				case "connection":
					v.ConnectionID++
				case "generation":
					v.TransportGeneration++
				}
				return records
			})
			if err := s.Commit(token, r, bad); err == nil {
				t.Fatal("mixed native views accepted")
			}
			if s.View().Envelope != nil {
				t.Fatal("mixed native views published")
			}
		})
	}
}
