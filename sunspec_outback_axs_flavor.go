package modbusreg

const OutBackAXSReadOnlyFlavorID = "sunspec.flavor.outback.axs.readonly@1.0.0"

type OutBackAXSReadOnlyFlavorReason string

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

func (OutBackAXSReadOnlyFlavorDecision) Matched() bool { return false }
func (d OutBackAXSReadOnlyFlavorDecision) Reason() OutBackAXSReadOnlyFlavorReason {
	return d.reason
}
func (OutBackAXSReadOnlyFlavorDecision) FlavorID() string        { return OutBackAXSReadOnlyFlavorID }
func (OutBackAXSReadOnlyFlavorDecision) Chain() []SunSpecWireKey { return nil }
func (OutBackAXSReadOnlyFlavorDecision) InterfaceOccurrence() (SunSpecOccurrence, bool) {
	return SunSpecOccurrence{}, false
}
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
