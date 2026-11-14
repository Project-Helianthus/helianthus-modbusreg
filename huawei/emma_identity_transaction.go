package huawei

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

// EmmaFC03Status is the caller-reported acquisition status of the FC03
// identity tuple. Only EmmaFC03Complete lets Commit evaluate the identity;
// missing and failed tuples yield EmmaCommitIdentityUnavailable.
type EmmaFC03Status string

// FC03 acquisition statuses.
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

// EmmaMEIStatus records the optional MEI device-identification read as
// diagnostics only. A successful MEI read never replaces a missing or failed
// FC03 tuple.
type EmmaMEIStatus string

// MEI diagnostic statuses.
const (
	EmmaMEINotRequested EmmaMEIStatus = "not_requested"
	EmmaMEIFailed       EmmaMEIStatus = "failed"
	EmmaMEISucceeded    EmmaMEIStatus = "succeeded"
)

// EmmaInventoryStatus reports whether the caller asked for the child
// inventory. The identity transaction cannot acquire inventory, so a requested
// inventory is always EmmaInventoryBlocked.
type EmmaInventoryStatus string

// Inventory statuses.
const (
	EmmaInventoryNotRequested EmmaInventoryStatus = "not_requested"
	EmmaInventoryBlocked      EmmaInventoryStatus = "blocked"
)

// EmmaCommitReason explains the outcome of one Commit. Only
// EmmaCommitIdentityCommitted replaces the retained identity.
// EmmaCommitContextRejected covers a zero, cancelled, expired or stale context;
// EmmaCommitIdentityUnavailable an FC03 tuple that is not complete; and
// EmmaCommitIdentityMismatch a complete tuple that
// EvaluateHuaweiEMMAOfflineIdentity does not match.
type EmmaCommitReason string

// Commit reasons.
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

// NewEmmaIdentityAttempt freezes one attempt with detached copies of the
// native evidence. It validates nothing; Commit does. The lifecycle is
// described in
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/emma-identity-transaction-implementation.md.
func NewEmmaIdentityAttempt(context EmmaIdentityContext, input EmmaIdentityInput, mei EmmaMEIStatus, inventoryRequested bool) EmmaIdentityAttempt {
	return EmmaIdentityAttempt{context: context, input: cloneEmmaIdentityInput(input), mei: mei, inventoryRequested: inventoryRequested}
}

// EmmaRetainedIdentity keeps the originating context and observation time.
type EmmaRetainedIdentity struct {
	context    EmmaIdentityContext
	input      EmmaIdentityInput
	observedAt time.Time
}

// Context returns the context of the attempt that committed this identity.
func (identity EmmaRetainedIdentity) Context() EmmaIdentityContext { return identity.context }

// Input returns a detached copy of the committed FC03 tuple and its native
// request and response bytes.
func (identity EmmaRetainedIdentity) Input() EmmaIdentityInput {
	return cloneEmmaIdentityInput(identity.input)
}

// ObservedAt returns the caller-supplied commit time. Later failed attempts do
// not refresh it.
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

// AttemptContext returns this attempt's context, also when it was rejected.
func (result EmmaIdentityResult) AttemptContext() EmmaIdentityContext { return result.context }

// AttemptInput returns a detached copy of this attempt's input, also when it
// was rejected, for diagnostics.
func (result EmmaIdentityResult) AttemptInput() EmmaIdentityInput {
	return cloneEmmaIdentityInput(result.input)
}

// IdentityMatch reports that this attempt's complete tuple matched within the
// current context fence.
func (result EmmaIdentityResult) IdentityMatch() bool { return result.matched }

// Committed reports that this call replaced the retained identity.
func (result EmmaIdentityResult) Committed() bool { return result.committed }

// Reason returns the commit outcome; the zero result reports
// EmmaCommitContextRejected.
func (result EmmaIdentityResult) Reason() EmmaCommitReason {
	if result.reason == "" {
		return EmmaCommitContextRejected
	}
	return result.reason
}

// MEIStatus returns the optional MEI diagnostic status, EmmaMEINotRequested
// when none was given.
func (result EmmaIdentityResult) MEIStatus() EmmaMEIStatus {
	if result.mei == "" {
		return EmmaMEINotRequested
	}
	return result.mei
}

// InventoryStatus returns EmmaInventoryBlocked when the attempt requested the
// inventory and EmmaInventoryNotRequested otherwise.
func (result EmmaIdentityResult) InventoryStatus() EmmaInventoryStatus {
	if result.inventoryRequested {
		return EmmaInventoryBlocked
	}
	return EmmaInventoryNotRequested
}

// InventoryComplete is always false: this transaction never acquires the
// child inventory.
func (EmmaIdentityResult) InventoryComplete() bool { return false }

// InventoryAssociationInvalidated reports that a successful commit replaced a
// retained identity with a different offering, model, firmware, parent
// source, source revision or connection generation, so any inventory tied to
// the old identity must be retired by its owner.
func (result EmmaIdentityResult) InventoryAssociationInvalidated() bool { return result.invalidated }

// RetainedIdentity returns a detached copy of the last committed identity and
// whether one exists. It is history, not proof of current availability.
func (result EmmaIdentityResult) RetainedIdentity() (EmmaRetainedIdentity, bool) {
	identity := result.retained
	identity.input = cloneEmmaIdentityInput(identity.input)
	return identity, result.hasRetained
}

// Qualification is always QualificationEvidenceBlocked: an identity
// match is not qualification evidence.
func (EmmaIdentityResult) Qualification() QualificationReadiness {
	return QualificationEvidenceBlocked
}

// DefaultDenied is always true: identity matching never admits the device.
func (EmmaIdentityResult) DefaultDenied() bool { return true }

// AutomaticRuntimeAdmission is always false.
func (EmmaIdentityResult) AutomaticRuntimeAdmission() bool { return false }

// LiveQualified is always false.
func (EmmaIdentityResult) LiveQualified() bool { return false }

// CatalogRegistered is always false.
func (EmmaIdentityResult) CatalogRegistered() bool { return false }

// SupportClaim is always false.
func (EmmaIdentityResult) SupportClaim() bool { return false }

// WriteAuthority is always false.
func (EmmaIdentityResult) WriteAuthority() bool { return false }

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
