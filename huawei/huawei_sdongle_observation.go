package huawei

import "strings"

// SDongleOfflineReason explains a transport-free gateway-observation
// result. A matched observation remains default denied.
type SDongleOfflineReason string

// S-Dongle observation reasons, checked in this order: the unit is not 100,
// the model is not a listed S-Dongle model, the catalog decision does not
// admit the firmware text, the protocol main revision is not 5, the search
// state is not 0, the change sequence differs before and after the reads, or
// the child count exceeds the capacity.
const (
	SDongleOfflineMatched          SDongleOfflineReason = "matched"
	SDongleOfflineUnitMismatch     SDongleOfflineReason = "unit_mismatch"
	SDongleOfflineModelMismatch    SDongleOfflineReason = "model_mismatch"
	SDongleOfflineFirmwareMismatch SDongleOfflineReason = "firmware_mismatch"
	SDongleOfflineProtocolMismatch SDongleOfflineReason = "protocol_mismatch"
	SDongleOfflineSearchIncomplete SDongleOfflineReason = "search_incomplete"
	SDongleOfflineSequenceMismatch SDongleOfflineReason = "sequence_mismatch"
	SDongleOfflineCapacityMismatch SDongleOfflineReason = "capacity_mismatch"
)

// S-Dongle family identity. SDongleCanonicalClass is the family class a
// matched decision reports, and SDongleReadOnlyProfileID the read-only
// profile it names. The native identity contract is
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/gateway-readonly-v1.md.
const (
	SDongleCanonicalClass    = "S-Dongle"
	SDongleReadOnlyProfileID = "huawei.sdongle.readonly.v1"
)

// SDongleOfflineObservation is an already-decoded unit-100 snapshot.
// It intentionally carries no endpoint, socket, or request details.
type SDongleOfflineObservation struct {
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

// SDongleOfflineDecision is a default-denied result. It never enables
// runtime admission, child inventory, or an operation.
type SDongleOfflineDecision struct {
	matched   bool
	model     string
	reason    SDongleOfflineReason
	selection GatewaySelection
}

// Matched reports that every gate of the observation passed.
func (d SDongleOfflineDecision) Matched() bool { return d.matched }

// CanonicalClass returns SDongleCanonicalClass for a matched decision
// and the empty string otherwise.
func (d SDongleOfflineDecision) CanonicalClass() string {
	if !d.matched {
		return ""
	}
	return SDongleCanonicalClass
}

// ModelVariant returns the matched wire model with terminal NUL and space
// padding removed, and the empty string when the decision did not match.
func (d SDongleOfflineDecision) ModelVariant() string { return d.model }

// ProfileID returns SDongleReadOnlyProfileID for a matched decision and
// the empty string otherwise.
func (d SDongleOfflineDecision) ProfileID() string {
	if !d.matched {
		return ""
	}
	return SDongleReadOnlyProfileID
}

// DefaultDenied is always true: a match never admits the device at runtime.
func (d SDongleOfflineDecision) DefaultDenied() bool { return true }

// Reason returns the first gate that failed, or SDongleOfflineMatched.
func (d SDongleOfflineDecision) Reason() SDongleOfflineReason { return d.reason }

// Selection is the catalog decision over the firmware text, recorded once
// the model is admitted.
func (d SDongleOfflineDecision) Selection() GatewaySelection { return d.selection }

// huaweiSDongleProtocolMainRevision is the incompatible main revision that the
// consulted editions declare compatible. Any release revision of it is
// accepted: the editions declare the release revisions of one main revision
// compatible from 0 onward.
const huaweiSDongleProtocolMainRevision = 5

// EvaluateHuaweiSDongleOfflineObservation validates a complete, already-read
// gateway observation. The firmware must be admitted by the catalog decision
// over the S-Dongle floors (V200R022C10 for the listed models, compared within
// the V200 line, whose order the editions document); a build of a line that no
// floor documents is admitted with grade Hypothesis. The protocol version must
// carry main revision 5, with any release revision. It performs no Modbus I/O
// and remains non-actionable even when the tuple matches.
func EvaluateHuaweiSDongleOfflineObservation(observation SDongleOfflineObservation) SDongleOfflineDecision {
	if observation.UnitID != 100 {
		return SDongleOfflineDecision{reason: SDongleOfflineUnitMismatch}
	}
	model := trimHuaweiTerminalPadding(observation.Model)
	if !isHuaweiSDongleModel(model) {
		return SDongleOfflineDecision{reason: SDongleOfflineModelMismatch}
	}
	selection := DecideHuaweiGatewaySelection(GatewaySelectionInput{
		Family:   SDongleCanonicalClass,
		Scope:    GatewayScope{Endpoint: GatewayEndpointSelf, Unit: observation.UnitID},
		Model:    huaweiSDongleCatalogModel(model),
		Software: trimHuaweiTerminalPadding(observation.Firmware),
	})
	if !selection.Admitted() || selection.Build() == "" {
		return SDongleOfflineDecision{reason: SDongleOfflineFirmwareMismatch, selection: selection}
	}
	refused := func(reason SDongleOfflineReason) SDongleOfflineDecision {
		return SDongleOfflineDecision{reason: reason, selection: selection}
	}
	if observation.ProtocolMajor != huaweiSDongleProtocolMainRevision {
		return refused(SDongleOfflineProtocolMismatch)
	}
	if observation.SearchState != 0 {
		return refused(SDongleOfflineSearchIncomplete)
	}
	if observation.ChangeSequenceBefore != observation.ChangeSequenceAfter {
		return refused(SDongleOfflineSequenceMismatch)
	}
	if observation.ChildCount > observation.Capacity {
		return refused(SDongleOfflineCapacityMismatch)
	}
	return SDongleOfflineDecision{
		matched:   true,
		model:     model,
		reason:    SDongleOfflineMatched,
		selection: selection,
	}
}

// huaweiSDongleCatalogModel is the spelling of a wire model in the catalog
// model rows, which write the family name without the hyphen after the S.
func huaweiSDongleCatalogModel(model string) string {
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
