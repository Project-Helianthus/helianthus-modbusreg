package modbusreg

// GrowattDiagnosticCommitNativeIdentity preserves the raw continuity words
// defined by the battery group retention contract. Topology is the full word,
// not only its battery-ID bits. These words confer no acquisition authority.
type GrowattDiagnosticCommitNativeIdentity struct {
	MCUVersion, BMS, Pack, Topology, CellSeries uint16
}

// GrowattDiagnosticCommitContext is caller-asserted same-cycle provenance.
// Record and source tokens are opaque internal correlation, not device IDs.
// Epoch and cycle must be nonzero; allocation and cycle closure belong to the caller.
type GrowattDiagnosticCommitContext struct {
	BaseRecordID, SourceInstance string
	Epoch, Cycle                 uint64
	UnitID                       byte
	Function                     FunctionCode
	Revision                     GrowattBMSRevisionTuple
	NativeIdentity               GrowattDiagnosticCommitNativeIdentity
}

type GrowattDiagnosticCommitRejection string

const (
	GrowattDiagnosticCommitInvalidBase       GrowattDiagnosticCommitRejection = "invalid_base"
	GrowattDiagnosticCommitInvalidContext    GrowattDiagnosticCommitRejection = "invalid_context"
	GrowattDiagnosticCommitContextMismatch   GrowattDiagnosticCommitRejection = "context_mismatch"
	GrowattDiagnosticCommitTopologyChanged   GrowattDiagnosticCommitRejection = "topology_changed"
	GrowattDiagnosticCommitLineageChanged    GrowattDiagnosticCommitRejection = "diagnostic_lineage_changed"
	GrowattDiagnosticCommitInvalidDiagnostic GrowattDiagnosticCommitRejection = "invalid_diagnostic"
)

// GrowattDiagnosticCommitBase freezes a decoded successful base and its
// provenance. Only NewGrowattDiagnosticCommitBase creates a usable reference.
type GrowattDiagnosticCommitBase struct {
	context GrowattDiagnosticCommitContext
	base    GrowattBMSTypedReadOnlyStatus
	valid   bool
}

// NewGrowattDiagnosticCommitBase revalidates and detaches native base evidence.
// Exported typed fields are reconstructed from that evidence, never trusted as
// proof of success. No store, epoch allocation or continuity decision is made.
func NewGrowattDiagnosticCommitBase(base GrowattBMSTypedReadOnlyStatus, context GrowattDiagnosticCommitContext) (GrowattDiagnosticCommitBase, GrowattDiagnosticCommitRejection) {
	observation := base.NativeObservation()
	decoded, err := DecodeGrowattBMSTypedReadOnlyStatus(GrowattBMSReadOnlyInput{
		UnitID: observation.UnitID(), Function: FunctionReadHoldingRegisters,
		Revision: observation.Revision(), Slices: observation.Slices(),
	})
	if err != nil {
		return GrowattDiagnosticCommitBase{}, GrowattDiagnosticCommitInvalidBase
	}
	if context.BaseRecordID == "" || context.SourceInstance == "" || context.Epoch == 0 || context.Cycle == 0 ||
		context.UnitID != observation.UnitID() || context.Function != FunctionReadHoldingRegisters || context.Revision != observation.Revision() {
		return GrowattDiagnosticCommitBase{}, GrowattDiagnosticCommitInvalidContext
	}
	slices := observation.Slices()
	native := GrowattDiagnosticCommitNativeIdentity{
		MCUVersion: slices[0].Words[0], BMS: slices[1].Words[0], Pack: slices[1].Words[1],
		Topology: slices[1].Words[18], CellSeries: slices[1].Words[28],
	}
	if context.NativeIdentity.Topology != native.Topology {
		return GrowattDiagnosticCommitBase{}, GrowattDiagnosticCommitTopologyChanged
	}
	if context.NativeIdentity != native {
		return GrowattDiagnosticCommitBase{}, GrowattDiagnosticCommitContextMismatch
	}
	return GrowattDiagnosticCommitBase{context: context, base: decoded, valid: true}, ""
}

type GrowattDiagnosticCommitIdentity struct {
	BatteryID         uint8
	ReportedGroupWord uint16
}

// GrowattDiagnosticCommitCandidate is one detached all-five-record bundle.
// Its zero value is absent. The caller must serialize and atomically commit the
// entire candidate after rechecking that its frozen context is still active.
type GrowattDiagnosticCommitCandidate struct {
	context    GrowattDiagnosticCommitContext
	identity   GrowattDiagnosticCommitIdentity
	diagnostic GrowattBMSMultiGroupDiagnostic
}

func (candidate GrowattDiagnosticCommitCandidate) Context() GrowattDiagnosticCommitContext {
	return candidate.context
}

func (candidate GrowattDiagnosticCommitCandidate) Identity() GrowattDiagnosticCommitIdentity {
	return candidate.identity
}

// Diagnostic returns a detached view including independent native raw storage.
func (candidate GrowattDiagnosticCommitCandidate) Diagnostic() GrowattBMSMultiGroupDiagnostic {
	diagnostic := candidate.diagnostic
	diagnostic.rawSlices = candidate.diagnostic.RawSlices()
	return diagnostic
}

// CheckGrowattDiagnosticCommit checks caller-asserted provenance against the
// frozen successful base, then runs the existing complete diagnostic decoder.
// expectedReportedGroup is an explicit lineage expectation, including zero;
// a mismatch requires the caller to resolve lineage before retrying. This pure
// check cannot establish acquisition simultaneity, close cycles or commit data.
func CheckGrowattDiagnosticCommit(base GrowattDiagnosticCommitBase, context GrowattDiagnosticCommitContext, expectedReportedGroup uint16, input GrowattBMSMultiGroupDiagnosticInput) (GrowattDiagnosticCommitCandidate, GrowattDiagnosticCommitRejection) {
	if !base.valid {
		return GrowattDiagnosticCommitCandidate{}, GrowattDiagnosticCommitInvalidBase
	}
	if context.NativeIdentity.Topology != base.context.NativeIdentity.Topology {
		return GrowattDiagnosticCommitCandidate{}, GrowattDiagnosticCommitTopologyChanged
	}
	if context != base.context {
		return GrowattDiagnosticCommitCandidate{}, GrowattDiagnosticCommitContextMismatch
	}
	diagnostic, err := DecodeGrowattBMSMultiGroupDiagnostic(base.base, input)
	if err != nil {
		return GrowattDiagnosticCommitCandidate{}, GrowattDiagnosticCommitInvalidDiagnostic
	}
	if diagnostic.ReportedGroupID != expectedReportedGroup {
		return GrowattDiagnosticCommitCandidate{}, GrowattDiagnosticCommitLineageChanged
	}
	return GrowattDiagnosticCommitCandidate{
		context:    base.context,
		identity:   GrowattDiagnosticCommitIdentity{BatteryID: diagnostic.BatteryID, ReportedGroupWord: diagnostic.ReportedGroupID},
		diagnostic: diagnostic,
	}, ""
}
