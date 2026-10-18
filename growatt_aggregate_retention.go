package modbusreg

import (
	"fmt"
	"slices"
	"sync"
)

// GrowattAggregateHistoryLimit is the number of last-success records retained.
// Replacement discards the preceding record; there is no historical log.
const GrowattAggregateHistoryLimit = 1

// GrowattAggregateIdentity is native continuity evidence, not a device selector.
// Topology preserves the whole word at 0x001F; CellSeriesCount is at 0x0029.
type GrowattAggregateIdentity struct {
	MCUSoftwareVersion                                     string
	BMSCompany, BMSGeneration, PackCompany, PackGeneration uint8
	Topology, CellSeriesCount                              uint16
}

// GrowattAggregateContext carries caller-established continuity for a cycle.
// IdentityKnown must mean all identity fields have coherent evidence, including
// for failed acquisitions. Zero native values alone do not mean unknown.
// SourceInstance is an opaque internal token, never a network or device ID.
type GrowattAggregateContext struct {
	SourceInstance string
	OwnerEpoch     uint64
	UnitID         byte
	Function       FunctionCode
	Revision       GrowattBMSRevisionTuple
	IdentityKnown  bool
	Identity       GrowattAggregateIdentity
}

type GrowattAggregateAttempt struct {
	Context         GrowattAggregateContext
	Cycle, Deadline uint64
	// Outcome is pending, success, failed, or incomplete.
	Outcome string
}

// GrowattAggregateSnapshot separates applicability from last-success metadata.
// Value is absent for absent/invalidated states. Ticks are supplied logical
// counters, not capture times or evidence of wall-clock freshness.
type GrowattAggregateSnapshot struct {
	// State is absent, current, retained, or invalidated.
	State                                      string
	InvalidationReason                         string
	Attempt                                    GrowattAggregateAttempt
	SuccessRevision, SuccessCycle, SuccessTick uint64
	SuccessContext                             GrowattAggregateContext
	Value                                      *GrowattBMSTypedReadOnlyStatus
}

// GrowattAggregateRetention is an offline, in-memory aggregate lifecycle owner.
// Calls are serialized internally. No method acquires data or schedules work.
// Storage is bounded to one last success and one latest attempt.
type GrowattAggregateRetention struct {
	mu                                  sync.Mutex
	maxCycleTicks, tick                 uint64
	attempt                             GrowattAggregateAttempt
	started, applicable                 bool
	invalidationReason                  string
	revision, successCycle, successTick uint64
	successContext                      GrowattAggregateContext
	value                               *GrowattBMSTypedReadOnlyStatus
}

// NewGrowattAggregateRetention requires a finite positive maximum cycle span.
// The caller chooses logical tick units and explicitly advances them.
func NewGrowattAggregateRetention(maxCycleTicks uint64) (*GrowattAggregateRetention, error) {
	if maxCycleTicks == 0 {
		return nil, fmt.Errorf("aggregate completion bound must be positive")
	}
	return &GrowattAggregateRetention{maxCycleTicks: maxCycleTicks}, nil
}

// Begin replaces the latest attempt. A pending earlier attempt becomes
// incomplete and cannot commit. Cycles increase within an epoch; epochs increase
// across restarts. Neither a returning key nor failure restores invalidated data.
func (r *GrowattAggregateRetention) Begin(c GrowattAggregateContext, cycle, now, deadline uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.maxCycleTicks == 0 || c.SourceInstance == "" || len(c.SourceInstance) > 128 || c.OwnerEpoch == 0 || cycle == 0 ||
		len(c.Identity.MCUSoftwareVersion) > 32 || !growattAggregateBoundedRevision(c.Revision) ||
		deadline <= now || deadline-now > r.maxCycleTicks || now < r.tick {
		return fmt.Errorf("invalid aggregate cycle context or deadline")
	}
	if r.started && (c.OwnerEpoch < r.attempt.Context.OwnerEpoch ||
		(c.OwnerEpoch == r.attempt.Context.OwnerEpoch && cycle <= r.attempt.Cycle)) {
		return fmt.Errorf("aggregate cycle is not newer")
	}
	r.tick = now
	r.started = true
	r.attempt = GrowattAggregateAttempt{Context: c, Cycle: cycle, Deadline: deadline, Outcome: "pending"}
	if !c.IdentityKnown {
		r.invalidate("continuity_unknown")
	} else if r.value != nil && !growattAggregateSameContext(c, r.successContext) {
		r.invalidate("continuity_changed")
	}
	return nil
}

// Complete admits only one complete existing typed-decoder success. The exact
// cycle context and raw identity must agree. Identical successful retransmission
// is idempotent; conflicting duplicates never alter the accepted record.
func (r *GrowattAggregateRetention) Complete(c GrowattAggregateContext, cycle, now uint64, input GrowattBMSReadOnlyInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.callback(c, cycle, now); err != nil {
		return err
	}
	if r.attempt.Outcome == "success" {
		if r.sameInput(input) {
			return nil
		}
		return fmt.Errorf("conflicting aggregate completion")
	}
	if r.attempt.Outcome != "pending" {
		return fmt.Errorf("aggregate cycle is closed")
	}
	r.attempt.Outcome = "failed"
	status, err := DecodeGrowattBMSTypedReadOnlyStatus(input)
	if !c.IdentityKnown {
		r.invalidate("continuity_unknown")
		return fmt.Errorf("aggregate identity is unknown")
	}
	// An empty or partial payload cannot replace data. Declared known cycle
	// continuity still permits retention on acquisition/encoding failure.
	if len(input.Slices) > 0 && (input.UnitID != c.UnitID || input.Function != c.Function || input.Revision != c.Revision) {
		r.invalidate("identity_conflict")
		return fmt.Errorf("aggregate native context contradicts cycle")
	}
	// Check complete raw identity even if a typed telemetry encoding is malformed.
	native, nativeErr := DecodeGrowattBMSReadOnlyObservation(input)
	if nativeErr == nil && !growattAggregateSameIdentity(growattAggregateNativeIdentity(native), c.Identity) {
		r.invalidate("identity_conflict")
		return fmt.Errorf("aggregate native identity contradicts cycle")
	}
	if err != nil {
		return err
	}
	if r.revision == ^uint64(0) {
		return fmt.Errorf("aggregate replacement revision exhausted")
	}
	r.value = &status
	r.revision++
	r.successCycle = cycle
	r.successTick = now
	r.successContext = c
	r.applicable = true
	r.invalidationReason = ""
	r.attempt.Outcome = "success"
	return nil
}

// Fail closes the active attempt without replacing any last-success content.
// The caller's known continuity evidence is still required to retain values.
func (r *GrowattAggregateRetention) Fail(c GrowattAggregateContext, cycle, now uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.callback(c, cycle, now); err != nil {
		return err
	}
	if r.attempt.Outcome == "failed" {
		return nil
	}
	if r.attempt.Outcome != "pending" {
		return fmt.Errorf("aggregate cycle is closed")
	}
	r.attempt.Outcome = "failed"
	return nil
}

// Advance explicitly closes a pending cycle at its deadline. There is no timer;
// an owner must call Advance or another lifecycle operation to observe expiry.
func (r *GrowattAggregateRetention) Advance(now uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.advance(now)
}

func (r *GrowattAggregateRetention) advance(now uint64) error {
	if now < r.tick {
		return fmt.Errorf("aggregate logical tick moved backwards")
	}
	r.tick = now
	if r.started && r.attempt.Outcome == "pending" && now >= r.attempt.Deadline {
		r.attempt.Outcome = "incomplete"
	}
	return nil
}

func (r *GrowattAggregateRetention) callback(c GrowattAggregateContext, cycle, now uint64) error {
	if !r.started || cycle != r.attempt.Cycle || !growattAggregateSameContext(c, r.attempt.Context) {
		return fmt.Errorf("aggregate completion is not for the active cycle")
	}
	return r.advance(now)
}

func (r *GrowattAggregateRetention) invalidate(reason string) {
	r.applicable = false
	r.invalidationReason = reason
}

// Snapshot returns detached typed and raw storage. Even current describes
// cycle membership only and grants no acquisition or outbound authority.
func (r *GrowattAggregateRetention) Snapshot() GrowattAggregateSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := GrowattAggregateSnapshot{State: "absent", Attempt: r.attempt, SuccessRevision: r.revision,
		SuccessCycle: r.successCycle, SuccessTick: r.successTick, SuccessContext: r.successContext, InvalidationReason: r.invalidationReason}
	if r.value == nil {
		return s
	}
	if !r.applicable {
		s.State = "invalidated"
		return s
	}
	s.State = "retained"
	if r.attempt.Outcome == "success" {
		s.State = "current"
	}
	status := *r.value
	native := r.value.NativeObservation()
	status.nativeObservation = &native
	s.Value = &status
	return s
}

func (r *GrowattAggregateRetention) sameInput(input GrowattBMSReadOnlyInput) bool {
	if r.value == nil || input.UnitID != r.successContext.UnitID || input.Function != r.successContext.Function || input.Revision != r.successContext.Revision {
		return false
	}
	raw := r.value.nativeObservation.slices
	if len(input.Slices) != len(raw) {
		return false
	}
	for i, s := range raw {
		if s.Offset != input.Slices[i].Offset || !slices.Equal(s.Words, input.Slices[i].Words) {
			return false
		}
	}
	return true
}

func growattAggregateNativeIdentity(o GrowattBMSReadOnlyObservation) GrowattAggregateIdentity {
	raw := o.slices
	return GrowattAggregateIdentity{MCUSoftwareVersion: growattBMSByteVersion(raw[0].Words[0]),
		BMSCompany: uint8(raw[1].Words[0]), BMSGeneration: uint8(raw[1].Words[0] >> 8),
		PackCompany: uint8(raw[1].Words[1]), PackGeneration: uint8(raw[1].Words[1] >> 8),
		Topology: raw[1].Words[18], CellSeriesCount: raw[1].Words[28]}
}

func growattAggregateBoundedRevision(v GrowattBMSRevisionTuple) bool {
	return len(v.Family) <= 64 && len(v.FileRevision) <= 64 && len(v.HeaderVersion) <= 64 && len(v.CumulativeRevision) <= 64
}

func growattAggregateSameContext(a, b GrowattAggregateContext) bool {
	return a.SourceInstance == b.SourceInstance && a.OwnerEpoch == b.OwnerEpoch &&
		a.UnitID == b.UnitID && a.Function == b.Function &&
		a.Revision.Family == b.Revision.Family && a.Revision.FileRevision == b.Revision.FileRevision &&
		a.Revision.HeaderVersion == b.Revision.HeaderVersion && a.Revision.CumulativeRevision == b.Revision.CumulativeRevision &&
		a.IdentityKnown == b.IdentityKnown && growattAggregateSameIdentity(a.Identity, b.Identity)
}

func growattAggregateSameIdentity(a, b GrowattAggregateIdentity) bool {
	return a.MCUSoftwareVersion == b.MCUSoftwareVersion && a.BMSCompany == b.BMSCompany &&
		a.BMSGeneration == b.BMSGeneration && a.PackCompany == b.PackCompany && a.PackGeneration == b.PackGeneration &&
		a.Topology == b.Topology && a.CellSeriesCount == b.CellSeriesCount
}
