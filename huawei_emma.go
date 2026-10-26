package modbusreg

import (
	"strings"
)

const (
	HuaweiEMMACanonicalClass    = "EMMA"
	HuaweiEMMAReadOnlyProfileID = "huawei.emma.readonly.v1"
)

// HuaweiEMMAOfflineReason explains one transport-free identity gate result.
type HuaweiEMMAOfflineReason string

const (
	HuaweiEMMAOfflineMatched          HuaweiEMMAOfflineReason = "matched"
	HuaweiEMMAOfflineOfferingMismatch HuaweiEMMAOfflineReason = "offering_mismatch"
	HuaweiEMMAOfflineModelMismatch    HuaweiEMMAOfflineReason = "model_mismatch"
	HuaweiEMMAOfflineFirmwareMismatch HuaweiEMMAOfflineReason = "firmware_mismatch"
)

// HuaweiEMMAOfflineDecision is an immutable, default-denied identity result.
type HuaweiEMMAOfflineDecision struct {
	matched   bool
	model     string
	reason    HuaweiEMMAOfflineReason
	selection HuaweiGatewaySelection
}

func (d HuaweiEMMAOfflineDecision) Matched() bool { return d.matched }

func (d HuaweiEMMAOfflineDecision) CanonicalClass() string {
	if !d.matched {
		return ""
	}
	return HuaweiEMMACanonicalClass
}

func (d HuaweiEMMAOfflineDecision) ModelVariant() string { return d.model }

func (d HuaweiEMMAOfflineDecision) ProfileID() string {
	if !d.matched {
		return ""
	}
	return HuaweiEMMAReadOnlyProfileID
}

func (d HuaweiEMMAOfflineDecision) DefaultDenied() bool { return true }

func (d HuaweiEMMAOfflineDecision) Reason() HuaweiEMMAOfflineReason { return d.reason }

// Selection is the catalogue decision over the software text, recorded for a
// matched tuple and for a tuple refused by its software.
func (d HuaweiEMMAOfflineDecision) Selection() HuaweiGatewaySelection { return d.selection }

// ObservedModel reports whether the model is one of the observed models that
// the registry admitted before the family scope applied.
func (d HuaweiEMMAOfflineDecision) ObservedModel() bool {
	for _, model := range huaweiEMMAObservedModels {
		if d.model == model {
			return true
		}
	}
	return false
}

// huaweiEMMAObservedModels are the model values that the EMMA candidate listed
// before admission followed the catalogue's model rows. They are kept as the
// observed models; the family scope admits every model token of the family.
var huaweiEMMAObservedModels = []string{"EMMA-A01", "EMMA-A02"}

// EvaluateHuaweiEMMAOfflineIdentity classifies a complete documented FC03 EMMA
// identity tuple. The model must be a model token of the EMMA family that the
// catalogue's model rows hold, and the software text must be admitted by the
// catalogue decision: at or above the floor of its own branch, or of a branch
// that no floor documents (grade Hypothesis). It performs no transport work
// and never enables runtime admission.
func EvaluateHuaweiEMMAOfflineIdentity(offering, model, firmware string) HuaweiEMMAOfflineDecision {
	if !isHuaweiEMMAOffering(offering) {
		return HuaweiEMMAOfflineDecision{reason: HuaweiEMMAOfflineOfferingMismatch}
	}
	model = trimHuaweiTerminalPadding(model)
	if !isHuaweiEMMAFamilyModel(model) {
		return HuaweiEMMAOfflineDecision{reason: HuaweiEMMAOfflineModelMismatch}
	}
	selection := DecideHuaweiGatewaySelection(HuaweiGatewaySelectionInput{
		Family:   HuaweiEMMACanonicalClass,
		Scope:    HuaweiGatewayScope{Endpoint: HuaweiGatewayEndpointSelf, Unit: 0},
		Model:    model,
		Software: trimHuaweiTerminalPadding(firmware),
	})
	if !selection.Admitted() || selection.Build() == "" {
		return HuaweiEMMAOfflineDecision{reason: HuaweiEMMAOfflineFirmwareMismatch, selection: selection}
	}
	return HuaweiEMMAOfflineDecision{
		matched:   true,
		model:     model,
		reason:    HuaweiEMMAOfflineMatched,
		selection: selection,
	}
}

// isHuaweiEMMAFamilyModel reports whether a model text is a model token of the
// EMMA family under the catalogue's model rows.
func isHuaweiEMMAFamilyModel(model string) bool {
	return huaweiGatewayFamilyHoldsModel(HuaweiEMMACanonicalClass, model)
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
