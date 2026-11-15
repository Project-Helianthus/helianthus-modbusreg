package outback

import modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"

// AXSReadOnlyFlavorID identifies the OutBack AXS read-only SunSpec
// flavor. Every decision reports it, matched or not. See
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/outback/axs-port-sunspec-readonly-v1.md.
const AXSReadOnlyFlavorID = "sunspec.flavor.outback.axs.readonly@1.0.0"

// AXSReadOnlyFlavorReason is the stable reason code of an
// AXSReadOnlyFlavorDecision.
type AXSReadOnlyFlavorReason string

// OutBack flavor reasons. While the source selector is NO_GO, EvaluateSnapshot
// returns only AXSReadOnlyFlavorReasonSourceNotQualified; the other
// codes name the structural outcomes of the public boundary contract and are
// not produced.
const (
	AXSReadOnlyFlavorReasonSourceNotQualified   AXSReadOnlyFlavorReason = "NO_GO"
	AXSReadOnlyFlavorReasonInvalidChain         AXSReadOnlyFlavorReason = "INVALID_CHAIN"
	AXSReadOnlyFlavorReasonCommonNotAdmitted    AXSReadOnlyFlavorReason = "COMMON_NOT_ADMITTED"
	AXSReadOnlyFlavorReasonInterfaceAbsent      AXSReadOnlyFlavorReason = "INTERFACE_ABSENT"
	AXSReadOnlyFlavorReasonInterfaceWrongLength AXSReadOnlyFlavorReason = "INTERFACE_WRONG_LENGTH"
	AXSReadOnlyFlavorReasonInterfaceAmbiguous   AXSReadOnlyFlavorReason = "INTERFACE_AMBIGUOUS"
	AXSReadOnlyFlavorReasonMatched              AXSReadOnlyFlavorReason = "MATCHED"
)

// AXSReadOnlyFlavorDecision is an immutable result of an explicitly
// requested OutBack selection. It neither alters the standard registry nor
// authorizes acquisition, runtime activation, or control.
type AXSReadOnlyFlavorDecision struct {
	reason AXSReadOnlyFlavorReason
}

// Matched always reports false: no snapshot is matched while the source
// selector is NO_GO.
func (AXSReadOnlyFlavorDecision) Matched() bool { return false }

// Reason returns the rejection reason.
func (d AXSReadOnlyFlavorDecision) Reason() AXSReadOnlyFlavorReason {
	return d.reason
}

// FlavorID returns AXSReadOnlyFlavorID.
func (AXSReadOnlyFlavorDecision) FlavorID() string { return AXSReadOnlyFlavorID }

// Chain returns nil: a rejected decision carries no matched chain.
func (AXSReadOnlyFlavorDecision) Chain() []modbusreg.SunSpecWireKey { return nil }

// InterfaceOccurrence reports false: a rejected decision carries no interface
// occurrence.
func (AXSReadOnlyFlavorDecision) InterfaceOccurrence() (modbusreg.SunSpecOccurrence, bool) {
	return modbusreg.SunSpecOccurrence{}, false
}

// InterfaceModel reports false: a rejected decision carries no decoded
// interface model.
func (AXSReadOnlyFlavorDecision) InterfaceModel() (modbusreg.SunSpecDecodedModel, bool) {
	return modbusreg.SunSpecDecodedModel{}, false
}

// EvaluateSnapshot rejects every snapshot while the source selector is NO_GO.
// A complete Common Model and either documented length are structural evidence,
// not product/firmware qualification or an admitted read operation. The caller's
// immutable raw snapshot remains available; no acquisition request is created.
func (AXSReadOnlyDecoder) EvaluateSnapshot(_ modbusreg.SunSpecChainSnapshot) AXSReadOnlyFlavorDecision {
	return AXSReadOnlyFlavorDecision{reason: AXSReadOnlyFlavorReasonSourceNotQualified}
}
