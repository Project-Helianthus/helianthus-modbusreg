package modbusreg

import (
	"strconv"
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
	HuaweiEMMAOfflineModelMismatch    HuaweiEMMAOfflineReason = "model_mismatch"
	HuaweiEMMAOfflineFirmwareMismatch HuaweiEMMAOfflineReason = "firmware_mismatch"
)

// HuaweiEMMAOfflineDecision is an immutable, default-denied identity result.
type HuaweiEMMAOfflineDecision struct {
	matched bool
	model   string
	reason  HuaweiEMMAOfflineReason
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

// EvaluateHuaweiEMMAOfflineIdentity classifies only documented EMMA model and
// firmware strings. It performs no transport work and never enables runtime
// admission.
func EvaluateHuaweiEMMAOfflineIdentity(model, firmware string) HuaweiEMMAOfflineDecision {
	model = trimHuaweiTerminalPadding(model)
	if model != "EMMA-A01" && model != "EMMA-A02" {
		return HuaweiEMMAOfflineDecision{reason: HuaweiEMMAOfflineModelMismatch}
	}
	if !matchesHuaweiEMMAFirmwareFloor(trimHuaweiTerminalPadding(firmware)) {
		return HuaweiEMMAOfflineDecision{reason: HuaweiEMMAOfflineFirmwareMismatch}
	}
	return HuaweiEMMAOfflineDecision{
		matched: true,
		model:   model,
		reason:  HuaweiEMMAOfflineMatched,
	}
}

func trimHuaweiTerminalPadding(value string) string {
	return strings.TrimRight(value, "\x00 ")
}

func matchesHuaweiEMMAFirmwareFloor(firmware string) bool {
	var prefix string
	var floor int
	switch {
	case strings.HasPrefix(firmware, "SmartHEMS V100R024C00SPC"):
		prefix, floor = "SmartHEMS V100R024C00SPC", 100
	case strings.HasPrefix(firmware, "SmartHEMS V100R025C00SPC"):
		prefix, floor = "SmartHEMS V100R025C00SPC", 102
	default:
		return false
	}
	patch := strings.TrimPrefix(firmware, prefix)
	if len(patch) != 3 {
		return false
	}
	value, err := strconv.Atoi(patch)
	return err == nil && value >= floor
}
