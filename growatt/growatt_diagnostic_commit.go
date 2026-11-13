package growatt

import modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"

// DiagnosticCommitNativeIdentity preserves the raw continuity words
// defined by the battery group retention contract. Topology is the full word,
// not only its battery-ID bits. These words confer no acquisition authority.
type DiagnosticCommitNativeIdentity struct {
	MCUVersion, BMS, Pack, Topology, CellSeries uint16
}

// DiagnosticCommitContext is caller-asserted same-cycle provenance.
// Record and source tokens are opaque internal correlation, not device IDs.
// Epoch and cycle must be nonzero; allocation and cycle closure belong to the caller.
type DiagnosticCommitContext struct {
	BaseRecordID, SourceInstance string
	Epoch, Cycle                 uint64
	UnitID                       byte
	Function                     modbusreg.FunctionCode
	Revision                     BMSRevisionTuple
	NativeIdentity               DiagnosticCommitNativeIdentity
}

// DiagnosticCommitRejection is the bounded reason a base or diagnostic
// candidate is refused; the empty string means accepted. See
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/growatt-diagnostic-commit-check-implementation.md.
type DiagnosticCommitRejection string

// Rejection reasons. InvalidBase: the base evidence does not decode or the base
// was not built by NewDiagnosticCommitBase. InvalidContext: missing
// tokens, zero epoch or cycle, or a unit, function or revision that differs from
// the base. ContextMismatch: other provenance or continuity words differ.
// TopologyChanged: the full topology word differs (checked before ContextMismatch).
// LineageChanged: the decoded reported-group word differs from the expectation.
// InvalidDiagnostic: the diagnostic decoder refused the input.
const (
	DiagnosticCommitInvalidBase       DiagnosticCommitRejection = "invalid_base"
	DiagnosticCommitInvalidContext    DiagnosticCommitRejection = "invalid_context"
	DiagnosticCommitContextMismatch   DiagnosticCommitRejection = "context_mismatch"
	DiagnosticCommitTopologyChanged   DiagnosticCommitRejection = "topology_changed"
	DiagnosticCommitLineageChanged    DiagnosticCommitRejection = "diagnostic_lineage_changed"
	DiagnosticCommitInvalidDiagnostic DiagnosticCommitRejection = "invalid_diagnostic"
)

// DiagnosticCommitBase freezes a decoded successful base and its
// provenance. Only NewDiagnosticCommitBase creates a usable reference.
type DiagnosticCommitBase struct {
	context DiagnosticCommitContext
	base    BMSTypedReadOnlyStatus
	valid   bool
}

// NewDiagnosticCommitBase revalidates and detaches native base evidence.
// Exported typed fields are reconstructed from that evidence, never trusted as
// proof of success. No store, epoch allocation or continuity decision is made.
func NewDiagnosticCommitBase(base BMSTypedReadOnlyStatus, context DiagnosticCommitContext) (DiagnosticCommitBase, DiagnosticCommitRejection) {
	observation := base.NativeObservation()
	decoded, err := DecodeGrowattBMSTypedReadOnlyStatus(BMSReadOnlyInput{
		UnitID: observation.UnitID(), Function: modbusreg.FunctionReadHoldingRegisters,
		Revision: observation.Revision(), Slices: observation.Slices(),
	})
	if err != nil {
		return DiagnosticCommitBase{}, DiagnosticCommitInvalidBase
	}
	if context.BaseRecordID == "" || context.SourceInstance == "" || context.Epoch == 0 || context.Cycle == 0 ||
		context.UnitID != observation.UnitID() || context.Function != modbusreg.FunctionReadHoldingRegisters || context.Revision != observation.Revision() {
		return DiagnosticCommitBase{}, DiagnosticCommitInvalidContext
	}
	slices := observation.Slices()
	native := DiagnosticCommitNativeIdentity{
		MCUVersion: slices[0].Words[0], BMS: slices[1].Words[0], Pack: slices[1].Words[1],
		Topology: slices[1].Words[18], CellSeries: slices[1].Words[28],
	}
	if context.NativeIdentity.Topology != native.Topology {
		return DiagnosticCommitBase{}, DiagnosticCommitTopologyChanged
	}
	if context.NativeIdentity != native {
		return DiagnosticCommitBase{}, DiagnosticCommitContextMismatch
	}
	return DiagnosticCommitBase{context: context, base: decoded, valid: true}, ""
}

// DiagnosticCommitIdentity is the decoded battery ID and full
// reported-group word of an accepted diagnostic candidate.
type DiagnosticCommitIdentity struct {
	BatteryID         uint8
	ReportedGroupWord uint16
}

// DiagnosticCommitCandidate is one detached all-five-record bundle.
// Its zero value is absent. The caller must serialize and atomically commit the
// entire candidate after rechecking that its frozen context is still active.
type DiagnosticCommitCandidate struct {
	context    DiagnosticCommitContext
	identity   DiagnosticCommitIdentity
	diagnostic BMSMultiGroupDiagnostic
}

// Context returns the frozen base context the candidate was checked against.
func (candidate DiagnosticCommitCandidate) Context() DiagnosticCommitContext {
	return candidate.context
}

// Identity returns the decoded diagnostic identity.
func (candidate DiagnosticCommitCandidate) Identity() DiagnosticCommitIdentity {
	return candidate.identity
}

// Diagnostic returns a detached view including independent native raw storage.
func (candidate DiagnosticCommitCandidate) Diagnostic() BMSMultiGroupDiagnostic {
	diagnostic := candidate.diagnostic
	diagnostic.rawSlices = candidate.diagnostic.RawSlices()
	return diagnostic
}

// CheckGrowattDiagnosticCommit checks caller-asserted provenance against the
// frozen successful base, then runs the existing complete diagnostic decoder.
// expectedReportedGroup is an explicit lineage expectation, including zero;
// a mismatch requires the caller to resolve lineage before retrying. This pure
// check cannot establish acquisition simultaneity, close cycles or commit data.
func CheckGrowattDiagnosticCommit(base DiagnosticCommitBase, context DiagnosticCommitContext, expectedReportedGroup uint16, input BMSMultiGroupDiagnosticInput) (DiagnosticCommitCandidate, DiagnosticCommitRejection) {
	if !base.valid {
		return DiagnosticCommitCandidate{}, DiagnosticCommitInvalidBase
	}
	if context.NativeIdentity.Topology != base.context.NativeIdentity.Topology {
		return DiagnosticCommitCandidate{}, DiagnosticCommitTopologyChanged
	}
	if context != base.context {
		return DiagnosticCommitCandidate{}, DiagnosticCommitContextMismatch
	}
	diagnostic, err := DecodeGrowattBMSMultiGroupDiagnostic(base.base, input)
	if err != nil {
		return DiagnosticCommitCandidate{}, DiagnosticCommitInvalidDiagnostic
	}
	if diagnostic.ReportedGroupID != expectedReportedGroup {
		return DiagnosticCommitCandidate{}, DiagnosticCommitLineageChanged
	}
	return DiagnosticCommitCandidate{
		context:    base.context,
		identity:   DiagnosticCommitIdentity{BatteryID: diagnostic.BatteryID, ReportedGroupWord: diagnostic.ReportedGroupID},
		diagnostic: diagnostic,
	}, ""
}
