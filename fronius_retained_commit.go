package modbusreg

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// FroniusRetainedKey identifies compatible native evidence, not an admission.
// Source is an opaque caller identity, never a transport to open.
type FroniusRetainedKey struct {
	RequestedFunction   FunctionCode
	ReceivedFunction    FunctionCode
	Table               LogicalTable
	UnitID              byte
	Source              string
	ConnectionID        uint64
	Transport           TransportFamily
	TransportGeneration uint64
	CardID              string
	Schema              SunSpecSchemaRevision
}

// FroniusRetainedToken is an opaque store-local, single-use cycle identity.
type FroniusRetainedToken struct{ cycle *froniusRetainedCycle }
type froniusRetainedCycle struct {
	ctx context.Context
}

// FroniusRetainedAttempt preserves the original finite caller context identity.
type FroniusRetainedAttempt struct {
	Token            FroniusRetainedToken
	Key              FroniusRetainedKey
	PollGeneration   uint64
	DeadlineIdentity uint64
	Deadline         time.Time
}

type FroniusRetainedOutcome string

const (
	FroniusRetainedPending   FroniusRetainedOutcome = "pending"
	FroniusRetainedSucceeded FroniusRetainedOutcome = "succeeded"
	FroniusRetainedFailed    FroniusRetainedOutcome = "failed"
	FroniusRetainedCancelled FroniusRetainedOutcome = "cancelled"
)

// FroniusRetainedEnvelope contains only a complete exact-card offline result.
// CommittedAt is local commit age, not a device capture timestamp.
type FroniusRetainedEnvelope struct {
	Observation SunSpecQualificationObservation
	Attempt     FroniusRetainedAttempt
	CommittedAt time.Time
}

// FroniusRetainedView is detached. Attempt/Outcome describe the latest attempt;
// Envelope keeps its own original attempt. Stale never implies live admission.
type FroniusRetainedView struct {
	Envelope *FroniusRetainedEnvelope
	Attempt  FroniusRetainedAttempt
	Outcome  FroniusRetainedOutcome
	Stale    bool
}

// FroniusRetainedCommit is an explicitly called in-memory boundary. Its zero
// value is ready. Do not copy after use. It has no I/O, scheduler or persistence.
type FroniusRetainedCommit struct {
	mu       sync.Mutex
	active   *froniusRetainedCycle
	key      FroniusRetainedKey
	envelope *FroniusRetainedEnvelope
	attempt  FroniusRetainedAttempt
	outcome  FroniusRetainedOutcome
}

// Begin supersedes the active cycle and irreversibly drops incompatible history.
// A changed key fences history even if the new attempt context is invalid.
func (s *FroniusRetainedCommit) Begin(ctx context.Context, key FroniusRetainedKey, pollGeneration, deadlineIdentity uint64) (FroniusRetainedToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key != key {
		s.envelope = nil
	}
	s.key = key
	s.active = nil
	s.attempt = FroniusRetainedAttempt{Key: key, PollGeneration: pollGeneration, DeadlineIdentity: deadlineIdentity}
	s.outcome = FroniusRetainedFailed
	if ctx == nil {
		return FroniusRetainedToken{}, fmt.Errorf("finite caller deadline required")
	}
	deadline, ok := ctx.Deadline()
	if !ok || deadline.IsZero() || !time.Now().Before(deadline) || ctx.Err() != nil || pollGeneration == 0 || deadlineIdentity == 0 {
		return FroniusRetainedToken{}, fmt.Errorf("finite active deadline and nonzero attempt identity required")
	}
	if key.Source == "" || key.ConnectionID == 0 || key.TransportGeneration == 0 || key.CardID == "" || key.Schema == "" {
		s.envelope = nil
		return FroniusRetainedToken{}, fmt.Errorf("known source compatibility required")
	}
	cycle := &froniusRetainedCycle{ctx: ctx}
	token := FroniusRetainedToken{cycle: cycle}
	s.attempt = FroniusRetainedAttempt{Token: token, Key: key, PollGeneration: pollGeneration, DeadlineIdentity: deadlineIdentity, Deadline: deadline}
	s.active = cycle
	s.outcome = FroniusRetainedPending
	return token, nil
}

// Commit replays the complete supplied observation through the existing card.
// Validation and publication serialize with Begin, Fail, Cancel and View.
// Invalid active candidates close the attempt without replacing prior evidence.
func (s *FroniusRetainedCommit) Commit(token FroniusRetainedToken, registry SunSpecDecoderRegistry, observation SunSpecQualificationObservation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkActive(token); err != nil {
		return err
	}
	reject := func(err error) error { s.active = nil; s.outcome = FroniusRetainedFailed; return err }
	card := NewSunSpecFroniusQualificationCard()
	if s.key.CardID != card.ID() || s.key.Schema != card.SchemaRevision() {
		return reject(fmt.Errorf("card/schema mismatch"))
	}
	replay, err := observation.Replay()
	if err != nil {
		return reject(err)
	}
	validated, err := card.Qualify(registry, replay.snapshot)
	if err != nil {
		return reject(err)
	}
	if !reflect.DeepEqual(validated, observation) {
		return reject(fmt.Errorf("observation differs from card replay"))
	}
	for _, view := range validated.SourceViews() {
		v := view.Record()
		key := FroniusRetainedKey{RequestedFunction: v.RequestedFunction, ReceivedFunction: v.ReceivedFunction, Table: v.Table, UnitID: v.UnitID, Source: v.Endpoint, ConnectionID: v.ConnectionID, Transport: v.Transport, TransportGeneration: v.TransportGeneration, CardID: card.ID(), Schema: card.SchemaRevision()}
		if key != s.key || v.PollGeneration != s.attempt.PollGeneration || v.DeadlineIdentity != s.attempt.DeadlineIdentity {
			return reject(fmt.Errorf("source or attempt mismatch"))
		}
	}
	// Replay may consume the remaining caller deadline; check again at publication.
	if err := s.checkActive(token); err != nil {
		return err
	}
	s.envelope = &FroniusRetainedEnvelope{Observation: validated, Attempt: s.attempt, CommittedAt: time.Now()}
	s.active = nil
	s.outcome = FroniusRetainedSucceeded
	return nil
}

func (s *FroniusRetainedCommit) checkActive(token FroniusRetainedToken) error {
	if token.cycle == nil || s.active != token.cycle {
		return fmt.Errorf("inactive retained cycle")
	}
	if token.cycle.ctx.Err() != nil || !time.Now().Before(s.attempt.Deadline) {
		s.active = nil
		s.outcome = FroniusRetainedCancelled
		return fmt.Errorf("retained cycle cancelled or expired")
	}
	return nil
}

// Fail closes the attempt with a bounded diagnostic outcome, without caller text.
func (s *FroniusRetainedCommit) Fail(token FroniusRetainedToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkActive(token); err != nil {
		return err
	}
	s.active = nil
	s.outcome = FroniusRetainedFailed
	return nil
}

// Cancel wins against every subsequent completion for this token.
func (s *FroniusRetainedCommit) Cancel(token FroniusRetainedToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkActive(token); err != nil {
		return err
	}
	s.active = nil
	s.outcome = FroniusRetainedCancelled
	return nil
}

func (s *FroniusRetainedCommit) View() FroniusRetainedView {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != nil {
		_ = s.checkActive(FroniusRetainedToken{cycle: s.active})
	}
	result := FroniusRetainedView{Attempt: s.attempt, Outcome: s.outcome, Stale: s.envelope != nil && s.outcome != FroniusRetainedSucceeded}
	if s.envelope != nil {
		envelope := *s.envelope
		envelope.Observation = cloneFroniusRetainedObservation(envelope.Observation)
		result.Envelope = &envelope
	}
	return result
}

func cloneFroniusRetainedObservation(o SunSpecQualificationObservation) SunSpecQualificationObservation {
	o.capability = o.Capability()
	o.flavor = o.Flavor()
	o.chain = o.Chain()
	o.snapshot = cloneSunSpecQualificationSnapshot(o.snapshot)
	return o
}
