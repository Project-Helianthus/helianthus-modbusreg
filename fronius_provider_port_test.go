package modbusreg

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func i25Context(i25T *testing.T) context.Context {
	i25T.Helper()
	i25Ctx, i25Cancel := context.WithTimeout(context.Background(), time.Minute)
	i25T.Cleanup(i25Cancel)
	return i25Ctx
}

func i25Request(i25Key FroniusRetainedKey, i25Observation SunSpecQualificationObservation) FroniusOfflineRequest {
	return FroniusOfflineRequest{Key: i25Key, PollGeneration: i25Observation.SampleIdentity().PollGeneration(), DeadlineIdentity: i25Observation.SampleIdentity().DeadlineIdentity(), MaxWords: 512, MaxOccurrences: 64, MaxSourceViews: 512}
}

func i25Reader(i25Observation SunSpecQualificationObservation) FroniusOfflineReader {
	return func(_ context.Context, i25Read FroniusOfflineReadRequest) (FroniusOfflineReadResult, error) {
		return FroniusOfflineReadResult{Attempt: i25Read.Attempt, Observation: i25Observation}, nil
	}
}

func i25Port(i25T *testing.T, i25Registry SunSpecDecoderRegistry, i25Reader FroniusOfflineReader) *I25FroniusAdapter {
	i25T.Helper()
	i25Port, i25Err := I25NewFroniusAdapter(i25Registry, 512, 64, 512, i25Reader)
	if i25Err != nil {
		i25T.Fatal(i25Err)
	}
	return i25Port
}

func TestI25RefreshAndDetachment(i25T *testing.T) {
	i25Registry, i25Observation, i25Key := retainedFixture(i25T)
	i25Candidate := cloneFroniusRetainedObservation(i25Observation)
	var i25ReadErr error
	i25Calls := 0
	i25RequestValue := i25Request(i25Key, i25Observation)
	i25Ctx := i25Context(i25T)
	i25Port := i25Port(i25T, i25Registry, func(i25Got context.Context, i25Read FroniusOfflineReadRequest) (FroniusOfflineReadResult, error) {
		i25Calls++
		i25Deadline, _ := i25Ctx.Deadline()
		if i25Got != i25Ctx || i25Read.Attempt.Key != i25Key || i25Read.Attempt.PollGeneration != i25RequestValue.PollGeneration || i25Read.Attempt.DeadlineIdentity != i25RequestValue.DeadlineIdentity || i25Read.Attempt.Deadline != i25Deadline || i25Read.Attempt.Token == (FroniusRetainedToken{}) || i25Read.MaxWords != 512 || i25Read.MaxOccurrences != 64 || i25Read.MaxSourceViews != 512 {
			i25T.Fatal("identity or bounds changed")
		}
		return FroniusOfflineReadResult{Attempt: i25Read.Attempt, Observation: i25Candidate}, i25ReadErr
	})
	var i25Interface I25FroniusProvider = i25Port
	i25Life := i25Interface.I25View().I25Lifetime
	if i25Err := i25Interface.I25Run(i25Ctx, i25Life, i25RequestValue); i25Err != nil {
		i25T.Fatal(i25Err)
	}
	i25Original := i25Port.I25View().I25Native.Envelope
	if i25Original == nil || !reflect.DeepEqual(i25Original.Observation, i25Observation) {
		i25T.Fatal("native evidence lost")
	}
	for _, i25Mode := range []string{"partial", "failed", "rejected"} {
		i25T.Run(i25Mode, func(i25T *testing.T) {
			i25Candidate = cloneFroniusRetainedObservation(i25Observation)
			i25ReadErr = nil
			switch i25Mode {
			case "partial":
				i25Candidate.snapshot.sources = i25Candidate.snapshot.sources[:len(i25Candidate.snapshot.sources)-1]
			case "failed":
				i25ReadErr = errors.New("synthetic failure")
			case "rejected":
				i25Candidate = SunSpecQualificationObservation{}
			}
			if i25Port.I25Run(i25Ctx, i25Life, i25RequestValue) == nil {
				i25T.Fatal("failed refresh accepted")
			}
			i25View := i25Port.I25View().I25Native
			if !i25View.Stale || i25View.Outcome != FroniusRetainedFailed || !reflect.DeepEqual(i25View.Envelope, i25Original) {
				i25T.Fatal("original receipt renewed or changed")
			}
		})
	}
	if i25Calls != 4 {
		i25T.Fatalf("calls %d", i25Calls)
	}
	i25View := i25Port.I25View()
	i25View.I25Native.Envelope.Observation.snapshot.raw[0]++
	i25View.I25Native.Envelope.Observation.snapshot.sources[0].Words[0]++
	i25Observation.snapshot.raw[0]++
	i25Candidate.snapshot.raw = nil
	// Mutate supplied registry internals after construction; the adapter owns a copy.
	for i25Key := range i25Registry.definitions {
		delete(i25Registry.definitions, i25Key)
	}
	if !reflect.DeepEqual(i25Port.I25View().I25Native.Envelope, i25Original) {
		i25T.Fatal("input/output aliases receipt")
	}
	i25Card := NewSunSpecFroniusQualificationCard()
	if i25Card.LiveQualified() || i25Card.AutomaticRuntimeAdmission() || i25Card.WriteAuthority() {
		i25T.Fatal("documentary success promoted")
	}
}

func i25Axes() map[string]func(*FroniusRetainedAttempt) {
	return map[string]func(*FroniusRetainedAttempt){
		"RequestedFunction":   func(i25A *FroniusRetainedAttempt) { i25A.Key.RequestedFunction++ },
		"ReceivedFunction":    func(i25A *FroniusRetainedAttempt) { i25A.Key.ReceivedFunction++ },
		"Table":               func(i25A *FroniusRetainedAttempt) { i25A.Key.Table = InputRegisters },
		"UnitID":              func(i25A *FroniusRetainedAttempt) { i25A.Key.UnitID++ },
		"Source":              func(i25A *FroniusRetainedAttempt) { i25A.Key.Source = "other-synthetic-source" },
		"ConnectionID":        func(i25A *FroniusRetainedAttempt) { i25A.Key.ConnectionID++ },
		"Transport":           func(i25A *FroniusRetainedAttempt) { i25A.Key.Transport = TransportRTU },
		"TransportGeneration": func(i25A *FroniusRetainedAttempt) { i25A.Key.TransportGeneration++ },
		"CardID":              func(i25A *FroniusRetainedAttempt) { i25A.Key.CardID = "other-card" },
		"Schema":              func(i25A *FroniusRetainedAttempt) { i25A.Key.Schema = "other-schema" },
		"PollGeneration":      func(i25A *FroniusRetainedAttempt) { i25A.PollGeneration++ },
		"DeadlineIdentity":    func(i25A *FroniusRetainedAttempt) { i25A.DeadlineIdentity++ },
		"Deadline":            func(i25A *FroniusRetainedAttempt) { i25A.Deadline = i25A.Deadline.Add(time.Second) },
		"Token":               func(i25A *FroniusRetainedAttempt) { i25A.Token = FroniusRetainedToken{} },
	}
}

func TestI25Identity(i25T *testing.T) {
	i25Registry, i25Observation, i25Key := retainedFixture(i25T)
	for i25Name, i25Change := range i25Axes() {
		i25T.Run(i25Name, func(i25T *testing.T) {
			i25Port := i25Port(i25T, i25Registry, func(_ context.Context, i25Read FroniusOfflineReadRequest) (FroniusOfflineReadResult, error) {
				i25Attempt := i25Read.Attempt
				i25Change(&i25Attempt)
				return FroniusOfflineReadResult{Attempt: i25Attempt, Observation: i25Observation}, nil
			})
			if i25Port.I25Run(i25Context(i25T), i25Port.I25View().I25Lifetime, i25Request(i25Key, i25Observation)) == nil || i25Port.I25View().I25Native.Envelope != nil {
				i25T.Fatal("mismatched echo accepted")
			}
		})
	}
}

func TestI25KeyFences(i25T *testing.T) {
	i25Registry, i25Observation, i25Key := retainedFixture(i25T)
	for i25Name, i25Change := range i25Axes() {
		if i25Name == "PollGeneration" || i25Name == "DeadlineIdentity" || i25Name == "Deadline" || i25Name == "Token" {
			continue
		}
		i25T.Run(i25Name, func(i25T *testing.T) {
			i25Port := i25Port(i25T, i25Registry, i25Reader(i25Observation))
			i25Life := i25Port.I25View().I25Lifetime
			i25RequestValue := i25Request(i25Key, i25Observation)
			if i25Port.I25Run(i25Context(i25T), i25Life, i25RequestValue) != nil {
				i25T.Fatal("initial success failed")
			}
			i25Attempt := FroniusRetainedAttempt{Key: i25Key}
			i25Change(&i25Attempt)
			i25RequestValue.Key = i25Attempt.Key
			if i25Port.I25Run(i25Context(i25T), i25Life, i25RequestValue) == nil || i25Port.I25View().I25Native.Envelope != nil {
				i25T.Fatal("incompatible history survived")
			}
			i25RequestValue.Key = i25Key
			i25RequestValue.MaxWords = 0
			_ = i25Port.I25Run(i25Context(i25T), i25Life, i25RequestValue)
			if i25Port.I25View().I25Native.Envelope != nil {
				i25T.Fatal("history resurrected")
			}
		})
	}
}

func TestI25Refusals(i25T *testing.T) {
	i25Registry, i25Observation, i25Key := retainedFixture(i25T)
	i25Calls := 0
	i25ReaderValue := FroniusOfflineReader(func(_ context.Context, i25Read FroniusOfflineReadRequest) (FroniusOfflineReadResult, error) {
		i25Calls++
		return FroniusOfflineReadResult{Attempt: i25Read.Attempt, Observation: i25Observation}, nil
	})
	for _, i25Mode := range []string{"registry", "schema", "reader", "words zero", "words high", "occurrences zero", "occurrences high", "sources zero", "sources high"} {
		i25T.Run("constructor/"+i25Mode, func(i25T *testing.T) {
			i25R := i25Registry
			i25W, i25O, i25S := uint32(512), uint32(64), uint32(512)
			i25Callback := i25ReaderValue
			switch i25Mode {
			case "registry":
				i25R = SunSpecDecoderRegistry{}
			case "schema":
				i25R.revision = "other-schema"
			case "reader":
				i25Callback = nil
			case "words zero":
				i25W = 0
			case "words high":
				i25W = 513
			case "occurrences zero":
				i25O = 0
			case "occurrences high":
				i25O = 65
			case "sources zero":
				i25S = 0
			case "sources high":
				i25S = 513
			}
			if i25P, i25Err := I25NewFroniusAdapter(i25R, i25W, i25O, i25S, i25Callback); i25Err == nil || i25P != nil || i25Calls != 0 {
				i25T.Fatal("invalid configuration accepted or supplier called")
			}
		})
	}
	for _, i25Mode := range []string{"nil context", "no deadline", "cancelled", "expired", "poll", "deadline identity", "source", "connection", "generation", "card", "schema", "function", "received", "table", "unit", "transport", "words zero", "words high", "occurrences zero", "occurrences high", "sources zero", "sources high", "lifetime", "foreign lifetime", "retired", "zero adapter", "nil adapter"} {
		i25T.Run("run/"+i25Mode, func(i25T *testing.T) {
			i25P := i25Port(i25T, i25Registry, i25ReaderValue)
			i25Life := i25P.I25View().I25Lifetime
			i25Req := i25Request(i25Key, i25Observation)
			i25Ctx := i25Context(i25T)
			switch i25Mode {
			case "nil context":
				i25Ctx = nil
			case "no deadline":
				i25Ctx = context.Background()
			case "cancelled":
				i25C, i25Cancel := context.WithCancel(i25Ctx)
				i25Cancel()
				i25Ctx = i25C
			case "expired":
				i25C, i25Cancel := context.WithDeadline(i25Ctx, time.Now().Add(-time.Second))
				defer i25Cancel()
				i25Ctx = i25C
			case "poll":
				i25Req.PollGeneration = 0
			case "deadline identity":
				i25Req.DeadlineIdentity = 0
			case "source":
				i25Req.Key.Source = ""
			case "connection":
				i25Req.Key.ConnectionID = 0
			case "generation":
				i25Req.Key.TransportGeneration = 0
			case "card":
				i25Req.Key.CardID = "other-card"
			case "schema":
				i25Req.Key.Schema = "other-schema"
			case "function":
				i25Req.Key.RequestedFunction++
			case "received":
				i25Req.Key.ReceivedFunction++
			case "table":
				i25Req.Key.Table = InputRegisters
			case "unit":
				i25Req.Key.UnitID++
			case "transport":
				i25Req.Key.Transport = ""
			case "words zero":
				i25Req.MaxWords = 0
			case "words high":
				i25Req.MaxWords = 513
			case "occurrences zero":
				i25Req.MaxOccurrences = 0
			case "occurrences high":
				i25Req.MaxOccurrences = 65
			case "sources zero":
				i25Req.MaxSourceViews = 0
			case "sources high":
				i25Req.MaxSourceViews = 513
			case "lifetime":
				i25Life = I25FroniusLifetime{}
			case "foreign lifetime":
				i25Life = i25Port(i25T, i25Registry, i25ReaderValue).I25View().I25Lifetime
			case "retired":
				if i25P.I25Retire(i25Life) != nil {
					i25T.Fatal("retire failed")
				}
			case "zero adapter":
				i25P = new(I25FroniusAdapter)
			case "nil adapter":
				i25P = nil
			}
			if i25P.I25Run(i25Ctx, i25Life, i25Req) == nil || i25Calls != 0 || i25P.I25View().I25Native.Envelope != nil {
				i25T.Fatal("refused run invoked supplier or published")
			}
		})
	}
	for _, i25Mode := range []string{"words", "occurrences", "sources"} {
		i25T.Run("configured ceiling/"+i25Mode, func(i25T *testing.T) {
			i25W, i25O, i25S := uint32(512), uint32(64), uint32(512)
			switch i25Mode {
			case "words":
				i25W--
			case "occurrences":
				i25O--
			case "sources":
				i25S--
			}
			i25P, i25Err := I25NewFroniusAdapter(i25Registry, i25W, i25O, i25S, i25ReaderValue)
			if i25Err != nil {
				i25T.Fatal(i25Err)
			}
			if i25P.I25Run(i25Context(i25T), i25P.I25View().I25Lifetime, i25Request(i25Key, i25Observation)) == nil || i25Calls != 0 {
				i25T.Fatal("configured ceiling ignored")
			}
		})
	}
}

func TestI25Lifecycle(i25T *testing.T) {
	i25Registry, i25Observation, i25Key := retainedFixture(i25T)
	for _, i25Mode := range []string{"cancel", "context before return", "context at return", "retire", "supersede", "supersede success", "foreign cancel", "foreign retire"} {
		i25T.Run(i25Mode, func(i25T *testing.T) {
			i25Entered := make(chan FroniusRetainedAttempt, 1)
			i25Release := make(chan struct{})
			i25Done := make(chan error, 1)
			i25Block := false
			i25Ctx, i25Cancel := context.WithCancel(i25Context(i25T))
			defer i25Cancel()
			i25P := i25Port(i25T, i25Registry, func(_ context.Context, i25Read FroniusOfflineReadRequest) (FroniusOfflineReadResult, error) {
				if i25Block {
					i25Entered <- i25Read.Attempt
					<-i25Release
					if i25Mode == "context at return" {
						i25Cancel()
					}
				}
				return FroniusOfflineReadResult{Attempt: i25Read.Attempt, Observation: i25Observation}, nil
			})
			i25Life := i25P.I25View().I25Lifetime
			i25Req := i25Request(i25Key, i25Observation)
			if i25P.I25Run(i25Ctx, i25Life, i25Req) != nil {
				i25T.Fatal("initial success failed")
			}
			i25Original := i25P.I25View().I25Native.Envelope
			i25Block = true
			go func() { i25Done <- i25P.I25Run(i25Ctx, i25Life, i25Req) }()
			i25Attempt := <-i25Entered
			i25Pending := i25P.I25View()
			if i25Pending.I25Lifetime != i25Life || i25Pending.I25Native.Attempt.Token != i25Attempt.Token || !i25Pending.I25Native.Stale || !reflect.DeepEqual(i25Pending.I25Native.Envelope, i25Original) {
				i25T.Fatal("pending view lost lifetime/receipt")
			}
			switch i25Mode {
			case "cancel":
				if i25P.I25Cancel(i25Life, i25Attempt.Token) != nil {
					i25T.Fatal("cancel failed")
				}
			case "context before return":
				i25Cancel()
			case "retire":
				if i25P.I25Retire(i25Life) != nil {
					i25T.Fatal("retire failed")
				}
			case "supersede":
				// An invalid newer cycle supersedes the blocked token without a second callback.
				i25New := i25Req
				i25New.MaxWords = 0
				if i25P.I25Run(i25Ctx, i25Life, i25New) == nil {
					i25T.Fatal("invalid newer run passed")
				}
			case "supersede success":
				i25Block = false
				if i25P.I25Run(i25Ctx, i25Life, i25Req) != nil {
					i25T.Fatal("newer success failed")
				}
			case "foreign cancel":
				if i25P.I25Cancel(I25FroniusLifetime{}, i25Attempt.Token) == nil {
					i25T.Fatal("foreign cancel accepted")
				}
			case "foreign retire":
				if i25P.I25Retire(I25FroniusLifetime{}) == nil {
					i25T.Fatal("foreign retire accepted")
				}
			}
			i25Winner := i25P.I25View()
			close(i25Release)
			i25Err := <-i25Done
			i25Got := i25P.I25View()
			if i25Mode == "foreign cancel" || i25Mode == "foreign retire" {
				if i25Err != nil || i25Got.I25Native.Outcome != FroniusRetainedSucceeded || i25Got.I25Retired {
					i25T.Fatal("foreign operation affected current run")
				}
				return
			}
			if i25Err == nil {
				i25T.Fatal("late completion accepted")
			}
			if i25Mode != "context at return" && !reflect.DeepEqual(i25Got, i25Winner) {
				i25T.Fatal("late completion changed winner")
			}
			if i25Mode == "context at return" && (!errors.Is(i25Err, context.Canceled) || i25Got.I25Native.Outcome != FroniusRetainedCancelled) {
				i25T.Fatal("post-return cancellation lost")
			}
			if i25Mode == "supersede success" {
				return
			}
			if !reflect.DeepEqual(i25Got.I25Native.Envelope, i25Original) || !i25Got.I25Native.Stale {
				i25T.Fatal("invalidated completion renewed receipt")
			}
			if i25Mode == "retire" {
				if !i25Got.I25Retired || i25P.I25Run(i25Ctx, i25Life, i25Req) == nil || i25P.I25Retire(i25Life) == nil || i25P.I25Cancel(i25Life, i25Attempt.Token) == nil {
					i25T.Fatal("retired lifetime reusable")
				}
			}
		})
	}
}

func TestI25ReplacementAndNativeRevision(i25T *testing.T) {
	i25Registry, i25Observation, i25Key := retainedFixture(i25T)
	i25Candidate := cloneFroniusRetainedObservation(i25Observation)
	i25P := i25Port(i25T, i25Registry, func(_ context.Context, i25Read FroniusOfflineReadRequest) (FroniusOfflineReadResult, error) {
		return FroniusOfflineReadResult{Attempt: i25Read.Attempt, Observation: i25Candidate}, nil
	})
	i25Life := i25P.I25View().I25Lifetime
	i25Ctx := i25Context(i25T)
	if i25P.I25Run(i25Ctx, i25Life, i25Request(i25Key, i25Observation)) != nil {
		i25T.Fatal("initial run failed")
	}
	i25Original := i25P.I25View().I25Native.Envelope
	for _, i25Axis := range []string{"PollGeneration", "DeadlineIdentity"} {
		i25T.Run(i25Axis, func(i25T *testing.T) {
			i25Req := i25Request(i25Key, i25Observation)
			if i25Axis == "PollGeneration" {
				i25Req.PollGeneration++
			} else {
				i25Req.DeadlineIdentity++
			}
			if i25P.I25Run(i25Ctx, i25Life, i25Req) == nil || !reflect.DeepEqual(i25P.I25View().I25Native.Envelope, i25Original) {
				i25T.Fatal("echo rewrote native identity")
			}
		})
	}
	for _, i25Axis := range []string{"occurrence revision", "decoder revision"} {
		i25T.Run(i25Axis, func(i25T *testing.T) {
			i25Candidate = cloneFroniusRetainedObservation(i25Observation)
			for i25Index := range i25Candidate.snapshot.occurrences {
				if i25Candidate.snapshot.occurrences[i25Index].decoderKey != nil {
					if i25Axis == "occurrence revision" {
						i25Candidate.snapshot.occurrences[i25Index].SchemaRevision = "other-schema"
					} else {
						i25Candidate.snapshot.occurrences[i25Index].decoderKey.SchemaRevision = "other-schema"
					}
					break
				}
			}
			if i25P.I25Run(i25Ctx, i25Life, i25Request(i25Key, i25Observation)) == nil || !reflect.DeepEqual(i25P.I25View().I25Native.Envelope, i25Original) {
				i25T.Fatal("changed native chain revision accepted")
			}
		})
	}
	i25Snapshot := cloneSunSpecQualificationSnapshot(i25Observation.snapshot)
	for i25Index := range i25Snapshot.sources {
		i25Snapshot.sources[i25Index].PollGeneration++
		i25Snapshot.sources[i25Index].DeadlineIdentity++
	}
	var i25Err error
	i25Candidate, i25Err = NewSunSpecFroniusQualificationCard().Qualify(i25Registry, i25Snapshot)
	if i25Err != nil {
		i25T.Fatal(i25Err)
	}
	i25Expected := cloneFroniusRetainedObservation(i25Candidate)
	// Detach the constructor from registry input before another successful replay.
	for i25Key, i25Definition := range i25Registry.definitions {
		if len(i25Definition.points) > 0 {
			i25Definition.points[0].fieldID = "changed-input"
		}
		delete(i25Registry.definitions, i25Key)
	}
	if i25P.I25Run(i25Ctx, i25Life, i25Request(i25Key, i25Candidate)) != nil {
		i25T.Fatal("replacement or registry detachment failed")
	}
	i25Got := i25P.I25View()
	if i25Got.I25Lifetime != i25Life || i25Got.I25Native.Stale || i25Got.I25Native.Outcome != FroniusRetainedSucceeded || i25Got.I25Native.Envelope.Attempt.Token == i25Original.Attempt.Token || i25Got.I25Native.Envelope.Attempt.PollGeneration != i25Observation.SampleIdentity().PollGeneration()+1 || i25Got.I25Native.Envelope.Attempt.DeadlineIdentity != i25Observation.SampleIdentity().DeadlineIdentity()+1 || !reflect.DeepEqual(i25Got.I25Native.Envelope.Observation, i25Expected) {
		i25T.Fatal("replacement mixed provenance or changed lifetime")
	}
	i25Candidate.snapshot.raw[0]++
	i25Candidate.snapshot.sources[0].Words[0]++
	i25Candidate.snapshot.occurrences[0].words[0]++
	if !reflect.DeepEqual(i25P.I25View().I25Native.Envelope.Observation, i25Expected) {
		i25T.Fatal("successful input not detached")
	}
	if i25P.I25Cancel(i25Life, i25Original.Attempt.Token) == nil {
		i25T.Fatal("closed token reusable")
	}
	i25BeforeRetire := i25P.I25View().I25Native
	if i25P.I25Retire(i25Life) != nil || !reflect.DeepEqual(i25P.I25View().I25Native, i25BeforeRetire) {
		i25T.Fatal("retirement after success relabeled receipt")
	}
}
