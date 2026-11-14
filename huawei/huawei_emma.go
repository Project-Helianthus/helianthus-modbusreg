package huawei

import (
	"strings"
)

// EMMA family identity. EMMACanonicalClass is the family class a
// matched decision reports, and EMMAReadOnlyProfileID the read-only
// profile it names. The native identity contract is
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/gateway-readonly-v1.md.
const (
	EMMACanonicalClass    = "EMMA"
	EMMAReadOnlyProfileID = "huawei.emma.readonly.v1"
)

// EMMAOfflineReason explains one transport-free identity gate result.
type EMMAOfflineReason string

// EMMA identity gate reasons, checked in this order: the offering text is not
// bounded printable ASCII, the model is not a model token of the EMMA family,
// or the catalog decision does not admit the software text.
const (
	EMMAOfflineMatched          EMMAOfflineReason = "matched"
	EMMAOfflineOfferingMismatch EMMAOfflineReason = "offering_mismatch"
	EMMAOfflineModelMismatch    EMMAOfflineReason = "model_mismatch"
	EMMAOfflineFirmwareMismatch EMMAOfflineReason = "firmware_mismatch"
)

// EMMAOfflineDecision is an immutable, default-denied identity result.
type EMMAOfflineDecision struct {
	matched   bool
	model     string
	reason    EMMAOfflineReason
	selection GatewaySelection
}

// Matched reports that the offering, model and software gates all passed.
func (d EMMAOfflineDecision) Matched() bool { return d.matched }

// CanonicalClass returns EMMACanonicalClass for a matched decision and
// the empty string otherwise.
func (d EMMAOfflineDecision) CanonicalClass() string {
	if !d.matched {
		return ""
	}
	return EMMACanonicalClass
}

// ModelVariant returns the matched model text with terminal NUL and space
// padding removed, and the empty string when the decision did not match.
func (d EMMAOfflineDecision) ModelVariant() string { return d.model }

// ProfileID returns EMMAReadOnlyProfileID for a matched decision and the
// empty string otherwise.
func (d EMMAOfflineDecision) ProfileID() string {
	if !d.matched {
		return ""
	}
	return EMMAReadOnlyProfileID
}

// DefaultDenied is always true: a match never admits the device at runtime.
func (d EMMAOfflineDecision) DefaultDenied() bool { return true }

// Reason returns the first gate that failed, or EMMAOfflineMatched.
func (d EMMAOfflineDecision) Reason() EMMAOfflineReason { return d.reason }

// Selection is the catalog decision over the software text, recorded for a
// matched tuple and for a tuple refused by its software.
func (d EMMAOfflineDecision) Selection() GatewaySelection { return d.selection }

// ObservedModel reports whether the model is one of the observed models that
// the registry admitted before the family scope applied.
func (d EMMAOfflineDecision) ObservedModel() bool {
	for _, model := range huaweiEMMAObservedModels {
		if d.model == model {
			return true
		}
	}
	return false
}

// huaweiEMMAObservedModels are the model values that the EMMA candidate listed
// before admission followed the catalog's model rows. They are kept as the
// observed models; the family scope admits every model token of the family.
var huaweiEMMAObservedModels = []string{"EMMA-A01", "EMMA-A02"}

// EvaluateHuaweiEMMAOfflineIdentity classifies a complete documented FC03 EMMA
// identity tuple. The model must be a model token of the EMMA family that the
// catalog's model rows hold, and the software text must be admitted by the
// catalog decision: at or above the floor of its own branch, or of a branch
// that no floor documents (grade Hypothesis). It performs no transport work
// and never enables runtime admission.
func EvaluateHuaweiEMMAOfflineIdentity(offering, model, firmware string) EMMAOfflineDecision {
	if !isHuaweiEMMAOffering(offering) {
		return EMMAOfflineDecision{reason: EMMAOfflineOfferingMismatch}
	}
	model = trimHuaweiTerminalPadding(model)
	if !isHuaweiEMMAFamilyModel(model) {
		return EMMAOfflineDecision{reason: EMMAOfflineModelMismatch}
	}
	selection := DecideHuaweiGatewaySelection(GatewaySelectionInput{
		Family:   EMMACanonicalClass,
		Scope:    GatewayScope{Endpoint: GatewayEndpointSelf, Unit: 0},
		Model:    model,
		Software: trimHuaweiTerminalPadding(firmware),
	})
	if !selection.Admitted() || selection.Build() == "" {
		return EMMAOfflineDecision{reason: EMMAOfflineFirmwareMismatch, selection: selection}
	}
	return EMMAOfflineDecision{
		matched:   true,
		model:     model,
		reason:    EMMAOfflineMatched,
		selection: selection,
	}
}

// isHuaweiEMMAFamilyModel reports whether a model text is a model token of the
// EMMA family under the catalog's model rows.
func isHuaweiEMMAFamilyModel(model string) bool {
	return huaweiGatewayFamilyHoldsModel(EMMACanonicalClass, model)
}

func isHuaweiEMMAOffering(offering string) bool {
	offering = trimHuaweiTerminalPadding(offering)
	if len(offering) == 0 || len(offering) > 30 {
		return false
	}
	for _, character := range offering {
		if character < 0x20 || character > 0x7e {
			return false
		}
	}
	return true
}

func trimHuaweiTerminalPadding(value string) string {
	return strings.TrimRight(value, "\x00 ")
}
