package modbusreg

import "time"

// EmmaIdentityContext identifies one caller-owned, per-parent attempt. Tokens
// are local correlation metadata, not native wire identifiers or credentials.
type EmmaIdentityContext struct {
	ParentSource         string
	SourceRevision       string
	ConnectionGeneration uint64
	Attempt              uint64
	Deadline             time.Time
	Cancelled            bool
}

type EmmaFC03Status string

const (
	EmmaFC03Missing  EmmaFC03Status = "missing"
	EmmaFC03Failed   EmmaFC03Status = "failed"
	EmmaFC03Complete EmmaFC03Status = "complete"
)

// EmmaIdentityInput contains one complete, already validated native FC03 tuple.
// The caller owns target, extent and encoding validation. Request and Response
// retain caller-supplied native evidence; absent replies must not be invented.
type EmmaIdentityInput struct {
	Status   EmmaFC03Status
	Offering string
	Model    string
	Firmware string
	Request  []byte
	Response []byte
}

type EmmaMEIStatus string

const (
	EmmaMEINotRequested EmmaMEIStatus = "not_requested"
	EmmaMEIFailed       EmmaMEIStatus = "failed"
	EmmaMEISucceeded    EmmaMEIStatus = "succeeded"
)

type EmmaInventoryStatus string

const (
	EmmaInventoryNotRequested EmmaInventoryStatus = "not_requested"
	EmmaInventoryBlocked      EmmaInventoryStatus = "blocked"
)

type EmmaCommitReason string

const (
	EmmaCommitContextRejected     EmmaCommitReason = "context_rejected"
	EmmaCommitIdentityUnavailable EmmaCommitReason = "identity_unavailable"
	EmmaCommitIdentityMismatch    EmmaCommitReason = "identity_mismatch"
	EmmaCommitIdentityCommitted   EmmaCommitReason = "identity_committed"
)

// EmmaIdentityAttempt owns a frozen, detached candidate. It has no guard
// qualification input and cannot acquire or validate inventory.
type EmmaIdentityAttempt struct {
	context            EmmaIdentityContext
	input              EmmaIdentityInput
	mei                EmmaMEIStatus
	inventoryRequested bool
}

func NewEmmaIdentityAttempt(context EmmaIdentityContext, input EmmaIdentityInput, mei EmmaMEIStatus, inventoryRequested bool) EmmaIdentityAttempt {
	return EmmaIdentityAttempt{context: context, input: cloneEmmaIdentityInput(input), mei: mei, inventoryRequested: inventoryRequested}
}

// EmmaRetainedIdentity keeps the originating context and observation time.
type EmmaRetainedIdentity struct {
	context    EmmaIdentityContext
	input      EmmaIdentityInput
	observedAt time.Time
}

func (identity EmmaRetainedIdentity) Context() EmmaIdentityContext { return identity.context }
func (identity EmmaRetainedIdentity) Input() EmmaIdentityInput {
	return cloneEmmaIdentityInput(identity.input)
}
func (identity EmmaRetainedIdentity) ObservedAt() time.Time { return identity.observedAt }

// EmmaIdentityResult separates this attempt's outcome from retained history.
type EmmaIdentityResult struct {
	context            EmmaIdentityContext
	input              EmmaIdentityInput
	matched            bool
	committed          bool
	retained           EmmaRetainedIdentity
	hasRetained        bool
	mei                EmmaMEIStatus
	inventoryRequested bool
	invalidated        bool
	reason             EmmaCommitReason
}

func (result EmmaIdentityResult) AttemptContext() EmmaIdentityContext { return result.context }
func (result EmmaIdentityResult) AttemptInput() EmmaIdentityInput {
	return cloneEmmaIdentityInput(result.input)
}
func (result EmmaIdentityResult) IdentityMatch() bool { return result.matched }
func (result EmmaIdentityResult) Committed() bool     { return result.committed }
func (result EmmaIdentityResult) Reason() EmmaCommitReason {
	if result.reason == "" {
		return EmmaCommitContextRejected
	}
	return result.reason
}
func (result EmmaIdentityResult) MEIStatus() EmmaMEIStatus {
	if result.mei == "" {
		return EmmaMEINotRequested
	}
	return result.mei
}
func (result EmmaIdentityResult) InventoryStatus() EmmaInventoryStatus {
	if result.inventoryRequested {
		return EmmaInventoryBlocked
	}
	return EmmaInventoryNotRequested
}
func (EmmaIdentityResult) InventoryComplete() bool                      { return false }
func (result EmmaIdentityResult) InventoryAssociationInvalidated() bool { return result.invalidated }
func (result EmmaIdentityResult) RetainedIdentity() (EmmaRetainedIdentity, bool) {
	identity := result.retained
	identity.input = cloneEmmaIdentityInput(identity.input)
	return identity, result.hasRetained
}
func (EmmaIdentityResult) Qualification() HuaweiQualificationReadiness {
	return HuaweiQualificationEvidenceBlocked
}
func (EmmaIdentityResult) DefaultDenied() bool             { return true }
func (EmmaIdentityResult) AutomaticRuntimeAdmission() bool { return false }
func (EmmaIdentityResult) LiveQualified() bool             { return false }
func (EmmaIdentityResult) CatalogRegistered() bool         { return false }
func (EmmaIdentityResult) SupportClaim() bool              { return false }
func (EmmaIdentityResult) WriteAuthority() bool            { return false }

// Commit is a pure transition. The owner must serialize reading current and
// publishing the returned value against changes to that parent's context.
// A rejected completion never refreshes or erases the retained observation.
func (attempt EmmaIdentityAttempt) Commit(current EmmaIdentityContext, now time.Time, previous EmmaIdentityResult) EmmaIdentityResult {
	result := EmmaIdentityResult{context: attempt.context, input: cloneEmmaIdentityInput(attempt.input), retained: previous.retained, hasRetained: previous.hasRetained, mei: attempt.mei, inventoryRequested: attempt.inventoryRequested, reason: EmmaCommitContextRejected}
	ctx := attempt.context
	if ctx.ParentSource == "" || ctx.SourceRevision == "" || ctx.ConnectionGeneration == 0 || ctx.Attempt == 0 || ctx.Deadline.IsZero() || ctx.Cancelled || current.Cancelled || now.IsZero() || !now.Before(ctx.Deadline) ||
		ctx.ParentSource != current.ParentSource || ctx.SourceRevision != current.SourceRevision || ctx.ConnectionGeneration != current.ConnectionGeneration || ctx.Attempt != current.Attempt || !ctx.Deadline.Equal(current.Deadline) {
		return result
	}
	if attempt.input.Status != EmmaFC03Complete {
		result.reason = EmmaCommitIdentityUnavailable
		return result
	}
	identity := EvaluateHuaweiEMMAOfflineIdentity(attempt.input.Offering, attempt.input.Model, attempt.input.Firmware)
	result.matched = identity.Matched()
	if !result.matched {
		result.reason = EmmaCommitIdentityMismatch
		return result
	}
	if previous.hasRetained {
		old := previous.retained
		result.invalidated = old.context.ParentSource != ctx.ParentSource || old.context.SourceRevision != ctx.SourceRevision || old.context.ConnectionGeneration != ctx.ConnectionGeneration || old.input.Offering != attempt.input.Offering || old.input.Model != attempt.input.Model || old.input.Firmware != attempt.input.Firmware
	}
	result.retained = EmmaRetainedIdentity{context: ctx, input: cloneEmmaIdentityInput(attempt.input), observedAt: now}
	result.hasRetained = true
	result.committed = true
	result.reason = EmmaCommitIdentityCommitted
	return result
}

func cloneEmmaIdentityInput(input EmmaIdentityInput) EmmaIdentityInput {
	input.Request = append([]byte(nil), input.Request...)
	input.Response = append([]byte(nil), input.Response...)
	return input
}
