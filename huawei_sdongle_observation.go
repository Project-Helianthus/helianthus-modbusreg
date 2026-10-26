package modbusreg

import "strings"

// HuaweiSDongleOfflineReason explains a transport-free gateway-observation
// result. A matched observation remains default denied.
type HuaweiSDongleOfflineReason string

const (
	HuaweiSDongleOfflineMatched          HuaweiSDongleOfflineReason = "matched"
	HuaweiSDongleOfflineUnitMismatch     HuaweiSDongleOfflineReason = "unit_mismatch"
	HuaweiSDongleOfflineModelMismatch    HuaweiSDongleOfflineReason = "model_mismatch"
	HuaweiSDongleOfflineFirmwareMismatch HuaweiSDongleOfflineReason = "firmware_mismatch"
	HuaweiSDongleOfflineProtocolMismatch HuaweiSDongleOfflineReason = "protocol_mismatch"
	HuaweiSDongleOfflineSearchIncomplete HuaweiSDongleOfflineReason = "search_incomplete"
	HuaweiSDongleOfflineSequenceMismatch HuaweiSDongleOfflineReason = "sequence_mismatch"
	HuaweiSDongleOfflineCapacityMismatch HuaweiSDongleOfflineReason = "capacity_mismatch"
)

const (
	HuaweiSDongleCanonicalClass    = "S-Dongle"
	HuaweiSDongleReadOnlyProfileID = "huawei.sdongle.readonly.v1"
)

// HuaweiSDongleOfflineObservation is an already-decoded unit-100 snapshot.
// It intentionally carries no endpoint, socket, or request details.
type HuaweiSDongleOfflineObservation struct {
	UnitID               uint8
	Model                string
	Firmware             string
	ProtocolMajor        uint8
	ProtocolMinor        uint8
	SearchState          uint16
	ChangeSequenceBefore uint16
	ChangeSequenceAfter  uint16
	Capacity             uint16
	ChildCount           uint16
}

// HuaweiSDongleOfflineDecision is a default-denied result. It never enables
// runtime admission, child inventory, or an operation.
type HuaweiSDongleOfflineDecision struct {
	matched   bool
	model     string
	reason    HuaweiSDongleOfflineReason
	selection HuaweiGatewaySelection
}

func (d HuaweiSDongleOfflineDecision) Matched() bool { return d.matched }

func (d HuaweiSDongleOfflineDecision) CanonicalClass() string {
	if !d.matched {
		return ""
	}
	return HuaweiSDongleCanonicalClass
}

func (d HuaweiSDongleOfflineDecision) ModelVariant() string { return d.model }

func (d HuaweiSDongleOfflineDecision) ProfileID() string {
	if !d.matched {
		return ""
	}
	return HuaweiSDongleReadOnlyProfileID
}

func (d HuaweiSDongleOfflineDecision) DefaultDenied() bool { return true }

func (d HuaweiSDongleOfflineDecision) Reason() HuaweiSDongleOfflineReason { return d.reason }

// Selection is the catalogue decision over the firmware text, recorded once
// the model is admitted.
func (d HuaweiSDongleOfflineDecision) Selection() HuaweiGatewaySelection { return d.selection }

// huaweiSDongleProtocolMainRevision is the incompatible main revision that the
// consulted editions declare compatible. Any release revision of it is
// accepted: the editions declare the release revisions of one main revision
// compatible from 0 onward.
const huaweiSDongleProtocolMainRevision = 5

// EvaluateHuaweiSDongleOfflineObservation validates a complete, already-read
// gateway observation. The firmware must be admitted by the catalogue decision
// over the S-Dongle floors (V200R022C10 for the listed models, compared within
// the V200 line, whose order the editions document); a build of a line that no
// floor documents is admitted with grade Hypothesis. The protocol version must
// carry main revision 5, with any release revision. It performs no Modbus I/O
// and remains non-actionable even when the tuple matches.
func EvaluateHuaweiSDongleOfflineObservation(observation HuaweiSDongleOfflineObservation) HuaweiSDongleOfflineDecision {
	if observation.UnitID != 100 {
		return HuaweiSDongleOfflineDecision{reason: HuaweiSDongleOfflineUnitMismatch}
	}
	model := trimHuaweiTerminalPadding(observation.Model)
	if !isHuaweiSDongleModel(model) {
		return HuaweiSDongleOfflineDecision{reason: HuaweiSDongleOfflineModelMismatch}
	}
	selection := DecideHuaweiGatewaySelection(HuaweiGatewaySelectionInput{
		Family:   HuaweiSDongleCanonicalClass,
		Scope:    HuaweiGatewayScope{Endpoint: HuaweiGatewayEndpointSelf, Unit: observation.UnitID},
		Model:    huaweiSDongleCatalogueModel(model),
		Software: trimHuaweiTerminalPadding(observation.Firmware),
	})
	if !selection.Admitted() || selection.Build() == "" {
		return HuaweiSDongleOfflineDecision{reason: HuaweiSDongleOfflineFirmwareMismatch, selection: selection}
	}
	refused := func(reason HuaweiSDongleOfflineReason) HuaweiSDongleOfflineDecision {
		return HuaweiSDongleOfflineDecision{reason: reason, selection: selection}
	}
	if observation.ProtocolMajor != huaweiSDongleProtocolMainRevision {
		return refused(HuaweiSDongleOfflineProtocolMismatch)
	}
	if observation.SearchState != 0 {
		return refused(HuaweiSDongleOfflineSearchIncomplete)
	}
	if observation.ChangeSequenceBefore != observation.ChangeSequenceAfter {
		return refused(HuaweiSDongleOfflineSequenceMismatch)
	}
	if observation.ChildCount > observation.Capacity {
		return refused(HuaweiSDongleOfflineCapacityMismatch)
	}
	return HuaweiSDongleOfflineDecision{
		matched:   true,
		model:     model,
		reason:    HuaweiSDongleOfflineMatched,
		selection: selection,
	}
}

// huaweiSDongleCatalogueModel is the spelling of a wire model in the catalogue
// model rows, which write the family name without the hyphen after the S.
func huaweiSDongleCatalogueModel(model string) string {
	return strings.Replace(model, "S-Dongle", "SDongle", 1)
}

func isHuaweiSDongleModel(model string) bool {
	switch model {
	case "S-DongleA-05", "S-DongleB-03", "S-DongleB-06":
		return true
	default:
		return false
	}
}
