package modbusreg

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// TeslaHSCCompatibilityV1 is the initial public profile compatibility gate.
const TeslaHSCCompatibilityV1 = "tesla_hsc_modbus_v1"

// TeslaHSCDisposition records whether a configured profile can do more than
// parse bounded frames. Qualification never grants opaque outbound admission.
type TeslaHSCDisposition string

const (
	TeslaHSCDisabled          TeslaHSCDisposition = "disabled"
	TeslaHSCFramingOnly       TeslaHSCDisposition = "framing_only"
	TeslaHSCQualifiedReadOnly TeslaHSCDisposition = "qualified_read_only"
)

// TeslaTEDAPIAdmissionState distinguishes profile qualification from the
// separate, per-operation wire-admission decision.
type TeslaTEDAPIAdmissionState string

const (
	// TeslaTEDAPIAdmissionBlockedProfile denies an unqualified profile.
	TeslaTEDAPIAdmissionBlockedProfile TeslaTEDAPIAdmissionState = "blocked_profile"
	// TeslaTEDAPIAdmissionBlockedNoAdmissibleOperation denies a qualified
	// profile until a later contract proves one particular operation safe.
	TeslaTEDAPIAdmissionBlockedNoAdmissibleOperation TeslaTEDAPIAdmissionState = "blocked_no_admissible_operation"
)

// TeslaTEDAPIOperationAdmission is the redacted result of local admission
// policy. It deliberately carries no request bytes or inferred operation name.
type TeslaTEDAPIOperationAdmission struct {
	State           TeslaTEDAPIAdmissionState
	OutboundAllowed bool
}

// TeslaHSCProfileConfig is an explicit local flavor configuration. A matching
// node or a readable frame is not a substitute for this configuration.
type TeslaHSCProfileConfig struct {
	Enabled              bool
	Node                 byte
	PassiveCompatible    bool
	CompatibilityVersion string
}

// TeslaHSCProfile retains the immutable profile gate decision.
type TeslaHSCProfile struct {
	config      TeslaHSCProfileConfig
	disposition TeslaHSCDisposition
}

// NewTeslaHSCProfile validates explicit configuration and calculates its
// fail-closed disposition without transmitting any vendor request.
func NewTeslaHSCProfile(config TeslaHSCProfileConfig) (TeslaHSCProfile, error) {
	if config.Node == 0 || config.Node > 247 {
		return TeslaHSCProfile{}, fmt.Errorf("tesla HSC node is invalid")
	}
	if config.CompatibilityVersion == "" {
		return TeslaHSCProfile{}, fmt.Errorf("tesla HSC compatibility version is required")
	}
	disposition := TeslaHSCDisabled
	if config.Enabled {
		disposition = TeslaHSCFramingOnly
		if config.PassiveCompatible &&
			config.CompatibilityVersion == TeslaHSCCompatibilityV1 {
			disposition = TeslaHSCQualifiedReadOnly
		}
	}
	return TeslaHSCProfile{config: config, disposition: disposition}, nil
}

// Node returns the explicitly configured RTU node.
func (profile TeslaHSCProfile) Node() byte {
	return profile.config.Node
}

// Disposition returns the fail-closed profile qualification result.
func (profile TeslaHSCProfile) Disposition() TeslaHSCDisposition {
	return profile.disposition
}

// OutboundAllowed always denies wire transmission in the opaque initial
// profile. A later typed, read-only operation contract must opt in separately.
func (TeslaHSCProfile) OutboundAllowed() bool {
	return false
}

// OperationAdmission reports that initial profile qualification never itself
// authorizes an opaque vendor request. A later contract must add a typed,
// allowlisted operation before this result can become sendable.
func (profile TeslaHSCProfile) OperationAdmission() TeslaTEDAPIOperationAdmission {
	if profile.Disposition() != TeslaHSCQualifiedReadOnly {
		return TeslaTEDAPIOperationAdmission{State: TeslaTEDAPIAdmissionBlockedProfile}
	}
	return TeslaTEDAPIOperationAdmission{
		State: TeslaTEDAPIAdmissionBlockedNoAdmissibleOperation,
	}
}

// TeslaHSCEnvelope is a decoded length envelope with uninterpreted inner bytes.
type TeslaHSCEnvelope struct {
	function modbus.FunctionCode
	payload  []byte
}

// DecodeTeslaHSCEnvelope validates the one-byte exact length envelope used by
// all supported HSC vendor functions and retains the nested bytes opaquely.
func DecodeTeslaHSCEnvelope(
	function modbus.FunctionCode,
	payload []byte,
) (TeslaHSCEnvelope, error) {
	if !isTeslaHSCFunction(function) {
		return TeslaHSCEnvelope{}, fmt.Errorf("tesla HSC function is unsupported")
	}
	if len(payload) == 0 {
		return TeslaHSCEnvelope{}, fmt.Errorf("tesla HSC payload has no length prefix")
	}
	if int(payload[0]) != len(payload)-1 {
		return TeslaHSCEnvelope{}, fmt.Errorf("tesla HSC length prefix is inexact")
	}
	return TeslaHSCEnvelope{
		function: function,
		payload:  append([]byte(nil), payload[1:]...),
	}, nil
}

// Function returns the vendor function identified by this envelope.
func (envelope TeslaHSCEnvelope) Function() modbus.FunctionCode {
	return envelope.function
}

// Payload returns an independent copy of opaque inner bytes.
func (envelope TeslaHSCEnvelope) Payload() []byte {
	return append([]byte(nil), envelope.payload...)
}

func isTeslaHSCFunction(function modbus.FunctionCode) bool {
	switch function {
	case modbus.FunctionVendor100,
		modbus.FunctionVendor101,
		modbus.FunctionVendor102:
		return true
	default:
		return false
	}
}

// TeslaHSCException classifies only the documented opaque response outcomes.
type TeslaHSCException string

const (
	TeslaHSCExceptionUnknown         TeslaHSCException = "unknown"
	TeslaHSCExceptionUnknownFunction TeslaHSCException = "unknown_function"
	TeslaHSCExceptionCodecFailure    TeslaHSCException = "codec_failure"
)

// ClassifyTeslaHSCException classifies exception status without decoding any
// opaque vendor payload or inventing field semantics.
func ClassifyTeslaHSCException(
	function modbus.FunctionCode,
	status byte,
) TeslaHSCException {
	if !isTeslaHSCFunction(function) {
		return TeslaHSCExceptionUnknown
	}
	if status == 1 {
		return TeslaHSCExceptionUnknownFunction
	}
	if status == 4 &&
		(function == modbus.FunctionVendor101 || function == modbus.FunctionVendor102) {
		return TeslaHSCExceptionCodecFailure
	}
	return TeslaHSCExceptionUnknown
}

// TeslaHSCProvenance retains redacted, deterministic payload identity only.
type TeslaHSCProvenance struct {
	compatibilityVersion string
	node                 byte
	function             modbus.FunctionCode
	payloadLength        int
	payloadDigest        string
}

// NewTeslaHSCProvenance creates provenance that contains no raw payload bytes.
func NewTeslaHSCProvenance(
	compatibilityVersion string,
	node byte,
	function modbus.FunctionCode,
	payload []byte,
) TeslaHSCProvenance {
	digest := sha256.Sum256(payload)
	return TeslaHSCProvenance{
		compatibilityVersion: compatibilityVersion,
		node:                 node,
		function:             function,
		payloadLength:        len(payload),
		payloadDigest:        hex.EncodeToString(digest[:]),
	}
}

// PayloadLength returns the retained opaque payload length.
func (provenance TeslaHSCProvenance) PayloadLength() int {
	return provenance.payloadLength
}

// PayloadDigest returns the redacted deterministic payload digest.
func (provenance TeslaHSCProvenance) PayloadDigest() string {
	return provenance.payloadDigest
}
