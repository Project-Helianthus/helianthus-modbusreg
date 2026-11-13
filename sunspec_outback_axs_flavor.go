package modbusreg

// OutBackAXSReadOnlyFlavorID identifies the OutBack AXS read-only SunSpec
// flavor. Every decision reports it, matched or not. See
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/outback/axs-port-sunspec-readonly-v1.md.
const OutBackAXSReadOnlyFlavorID = "sunspec.flavor.outback.axs.readonly@1.0.0"

// OutBackAXSReadOnlyFlavorReason is the stable reason code of an
// OutBackAXSReadOnlyFlavorDecision.
type OutBackAXSReadOnlyFlavorReason string

// OutBack flavor reasons. While the source selector is NO_GO, EvaluateSnapshot
// returns only OutBackAXSReadOnlyFlavorReasonSourceNotQualified; the other
// codes name the structural outcomes of the public boundary contract and are
// not produced.
const (
	OutBackAXSReadOnlyFlavorReasonSourceNotQualified   OutBackAXSReadOnlyFlavorReason = "NO_GO"
	OutBackAXSReadOnlyFlavorReasonInvalidChain         OutBackAXSReadOnlyFlavorReason = "INVALID_CHAIN"
	OutBackAXSReadOnlyFlavorReasonCommonNotAdmitted    OutBackAXSReadOnlyFlavorReason = "COMMON_NOT_ADMITTED"
	OutBackAXSReadOnlyFlavorReasonInterfaceAbsent      OutBackAXSReadOnlyFlavorReason = "INTERFACE_ABSENT"
	OutBackAXSReadOnlyFlavorReasonInterfaceWrongLength OutBackAXSReadOnlyFlavorReason = "INTERFACE_WRONG_LENGTH"
	OutBackAXSReadOnlyFlavorReasonInterfaceAmbiguous   OutBackAXSReadOnlyFlavorReason = "INTERFACE_AMBIGUOUS"
	OutBackAXSReadOnlyFlavorReasonMatched              OutBackAXSReadOnlyFlavorReason = "MATCHED"
)

// OutBackAXSReadOnlyFlavorDecision is an immutable result of an explicitly
// requested OutBack selection. It neither alters the standard registry nor
// authorizes acquisition, runtime activation, or control.
type OutBackAXSReadOnlyFlavorDecision struct {
	reason OutBackAXSReadOnlyFlavorReason
}

// Matched always reports false: no snapshot is matched while the source
// selector is NO_GO.
func (OutBackAXSReadOnlyFlavorDecision) Matched() bool { return false }

// Reason returns the rejection reason.
func (d OutBackAXSReadOnlyFlavorDecision) Reason() OutBackAXSReadOnlyFlavorReason {
	return d.reason
}

// FlavorID returns OutBackAXSReadOnlyFlavorID.
func (OutBackAXSReadOnlyFlavorDecision) FlavorID() string { return OutBackAXSReadOnlyFlavorID }

// Chain returns nil: a rejected decision carries no matched chain.
func (OutBackAXSReadOnlyFlavorDecision) Chain() []SunSpecWireKey { return nil }

// InterfaceOccurrence reports false: a rejected decision carries no interface
// occurrence.
func (OutBackAXSReadOnlyFlavorDecision) InterfaceOccurrence() (SunSpecOccurrence, bool) {
	return SunSpecOccurrence{}, false
}

// InterfaceModel reports false: a rejected decision carries no decoded
// interface model.
func (OutBackAXSReadOnlyFlavorDecision) InterfaceModel() (SunSpecDecodedModel, bool) {
	return SunSpecDecodedModel{}, false
}

// EvaluateSnapshot rejects every snapshot while the source selector is NO_GO.
// A complete Common Model and either documented length are structural evidence,
// not product/firmware qualification or an admitted read operation. The caller's
// immutable raw snapshot remains available; no acquisition request is created.
func (OutBackAXSReadOnlyDecoder) EvaluateSnapshot(_ SunSpecChainSnapshot) OutBackAXSReadOnlyFlavorDecision {
	return OutBackAXSReadOnlyFlavorDecision{reason: OutBackAXSReadOnlyFlavorReasonSourceNotQualified}
}
