package modbusreg

// HuaweiSDongleOfflineReason explains a transport-free gateway-observation
// result. A matched observation remains default denied.
type HuaweiSDongleOfflineReason string

const (
	HuaweiSDongleOfflineMatched          HuaweiSDongleOfflineReason = "matched"
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
	matched bool
	model   string
	reason  HuaweiSDongleOfflineReason
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

// EvaluateHuaweiSDongleOfflineObservation validates a complete, already-read
// gateway observation. It performs no Modbus I/O and remains non-actionable
// even when the documented tuple matches.
func EvaluateHuaweiSDongleOfflineObservation(observation HuaweiSDongleOfflineObservation) HuaweiSDongleOfflineDecision {
	model := trimHuaweiTerminalPadding(observation.Model)
	if !isHuaweiSDongleModel(model) {
		return HuaweiSDongleOfflineDecision{reason: HuaweiSDongleOfflineModelMismatch}
	}
	if !isHuaweiSDongleFirmwareTuple(model, trimHuaweiTerminalPadding(observation.Firmware)) {
		return HuaweiSDongleOfflineDecision{reason: HuaweiSDongleOfflineFirmwareMismatch}
	}
	if observation.ProtocolMajor != 5 || observation.ProtocolMinor != 0 {
		return HuaweiSDongleOfflineDecision{reason: HuaweiSDongleOfflineProtocolMismatch}
	}
	if observation.SearchState != 0 {
		return HuaweiSDongleOfflineDecision{reason: HuaweiSDongleOfflineSearchIncomplete}
	}
	if observation.ChangeSequenceBefore != observation.ChangeSequenceAfter {
		return HuaweiSDongleOfflineDecision{reason: HuaweiSDongleOfflineSequenceMismatch}
	}
	if observation.ChildCount > observation.Capacity {
		return HuaweiSDongleOfflineDecision{reason: HuaweiSDongleOfflineCapacityMismatch}
	}
	return HuaweiSDongleOfflineDecision{
		matched: true,
		model:   model,
		reason:  HuaweiSDongleOfflineMatched,
	}
}

func isHuaweiSDongleModel(model string) bool {
	switch model {
	case "S-DongleA-05", "S-DongleB-03", "S-DongleB-06":
		return true
	default:
		return false
	}
}

func isHuaweiSDongleFirmwareTuple(model, firmware string) bool {
	if firmware == "V200R025C00SPC120" {
		return true
	}
	return model == "S-DongleA-05" && firmware == "V200R022C10SPC312"
}
