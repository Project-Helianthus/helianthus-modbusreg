package fronius

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// RetainedKey identifies compatible native evidence, not an admission.
// Source is an opaque caller identity, never a transport to open.
type RetainedKey struct {
	RequestedFunction   modbusreg.FunctionCode
	ReceivedFunction    modbusreg.FunctionCode
	Table               modbusreg.LogicalTable
	UnitID              byte
	Source              string
	ConnectionID        uint64
	Transport           modbusreg.TransportFamily
	TransportGeneration uint64
	CardID              string
	Schema              modbusreg.SunSpecSchemaRevision
}

// RetainedToken is an opaque store-local, single-use cycle identity.
type RetainedToken struct{ cycle *froniusRetainedCycle }
type froniusRetainedCycle struct {
	ctx context.Context
}

// RetainedAttempt preserves the original finite caller context identity.
type RetainedAttempt struct {
	Token            RetainedToken
	Key              RetainedKey
	PollGeneration   uint64
	DeadlineIdentity uint64
	Deadline         time.Time
}

// RetainedOutcome is the latest-attempt diagnostic of a
// RetainedCommit. It never carries caller text, and only Succeeded
// replaces the retained envelope; see
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/fronius-retained-commit-implementation.md.
type RetainedOutcome string

// Latest-attempt outcomes. Pending is the only state in which a token can
// still commit; Failed also records a rejected Begin or invalid candidate, and
// Cancelled also records an expired or cancelled caller context.
const (
	RetainedPending   RetainedOutcome = "pending"
	RetainedSucceeded RetainedOutcome = "succeeded"
	RetainedFailed    RetainedOutcome = "failed"
	RetainedCancelled RetainedOutcome = "cancelled"
)

// RetainedEnvelope contains only a complete exact-card offline result.
// CommittedAt is local commit age, not a device capture timestamp.
type RetainedEnvelope struct {
	Observation SunSpecQualificationObservation
	Attempt     RetainedAttempt
	CommittedAt time.Time
}

// RetainedView is detached. Attempt/Outcome describe the latest attempt;
// Envelope keeps its own original attempt. Stale never implies live admission.
type RetainedView struct {
	Envelope *RetainedEnvelope
	Attempt  RetainedAttempt
	Outcome  RetainedOutcome
	Stale    bool
}

// RetainedCommit is an explicitly called in-memory boundary. Its zero
// value is ready. Do not copy after use. It has no I/O, scheduler or persistence.
type RetainedCommit struct {
	mu       sync.Mutex
	active   *froniusRetainedCycle
	key      RetainedKey
	envelope *RetainedEnvelope
	attempt  RetainedAttempt
	outcome  RetainedOutcome
}

// Begin supersedes the active cycle and irreversibly drops incompatible history.
// A changed key fences history even if the new attempt context is invalid.
func (s *RetainedCommit) Begin(ctx context.Context, key RetainedKey, pollGeneration, deadlineIdentity uint64) (RetainedToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key != key {
		s.envelope = nil
	}
	s.key = key
	s.active = nil
	s.attempt = RetainedAttempt{Key: key, PollGeneration: pollGeneration, DeadlineIdentity: deadlineIdentity}
	s.outcome = RetainedFailed
	if ctx == nil {
		return RetainedToken{}, fmt.Errorf("finite caller deadline required")
	}
	deadline, ok := ctx.Deadline()
	if !ok || deadline.IsZero() || !time.Now().Before(deadline) || ctx.Err() != nil || pollGeneration == 0 || deadlineIdentity == 0 {
		return RetainedToken{}, fmt.Errorf("finite active deadline and nonzero attempt identity required")
	}
	if key.Source == "" || key.ConnectionID == 0 || key.TransportGeneration == 0 || key.CardID == "" || key.Schema == "" {
		s.envelope = nil
		return RetainedToken{}, fmt.Errorf("known source compatibility required")
	}
	cycle := &froniusRetainedCycle{ctx: ctx}
	token := RetainedToken{cycle: cycle}
	s.attempt = RetainedAttempt{Token: token, Key: key, PollGeneration: pollGeneration, DeadlineIdentity: deadlineIdentity, Deadline: deadline}
	s.active = cycle
	s.outcome = RetainedPending
	return token, nil
}

// Commit replays the complete supplied observation through the existing card.
// Validation and publication serialize with Begin, Fail, Cancel and View.
// Invalid active candidates close the attempt without replacing prior evidence.
func (s *RetainedCommit) Commit(token RetainedToken, registry modbusreg.SunSpecDecoderRegistry, observation SunSpecQualificationObservation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkActive(token); err != nil {
		return err
	}
	reject := func(err error) error { s.active = nil; s.outcome = RetainedFailed; return err }
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
		key := RetainedKey{RequestedFunction: v.RequestedFunction, ReceivedFunction: v.ReceivedFunction, Table: v.Table, UnitID: v.UnitID, Source: v.Endpoint, ConnectionID: v.ConnectionID, Transport: v.Transport, TransportGeneration: v.TransportGeneration, CardID: card.ID(), Schema: card.SchemaRevision()}
		if key != s.key || v.PollGeneration != s.attempt.PollGeneration || v.DeadlineIdentity != s.attempt.DeadlineIdentity {
			return reject(fmt.Errorf("source or attempt mismatch"))
		}
	}
	// Replay may consume the remaining caller deadline; check again at publication.
	if err := s.checkActive(token); err != nil {
		return err
	}
	s.envelope = &RetainedEnvelope{Observation: validated, Attempt: s.attempt, CommittedAt: time.Now()}
	s.active = nil
	s.outcome = RetainedSucceeded
	return nil
}

func (s *RetainedCommit) checkActive(token RetainedToken) error {
	if token.cycle == nil || s.active != token.cycle {
		return fmt.Errorf("inactive retained cycle")
	}
	if token.cycle.ctx.Err() != nil || !time.Now().Before(s.attempt.Deadline) {
		s.active = nil
		s.outcome = RetainedCancelled
		return fmt.Errorf("retained cycle cancelled or expired")
	}
	return nil
}

// Fail closes the attempt with a bounded diagnostic outcome, without caller text.
func (s *RetainedCommit) Fail(token RetainedToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkActive(token); err != nil {
		return err
	}
	s.active = nil
	s.outcome = RetainedFailed
	return nil
}

// Cancel wins against every subsequent completion for this token.
func (s *RetainedCommit) Cancel(token RetainedToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkActive(token); err != nil {
		return err
	}
	s.active = nil
	s.outcome = RetainedCancelled
	return nil
}

// View returns a detached copy of the retained envelope and the latest attempt.
// An active attempt whose context has ended is closed as cancelled first. Stale
// is true when an envelope exists but the latest attempt did not succeed.
func (s *RetainedCommit) View() RetainedView {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != nil {
		_ = s.checkActive(RetainedToken{cycle: s.active})
	}
	result := RetainedView{Attempt: s.attempt, Outcome: s.outcome, Stale: s.envelope != nil && s.outcome != RetainedSucceeded}
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
