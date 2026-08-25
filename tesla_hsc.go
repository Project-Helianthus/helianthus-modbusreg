package modbusreg

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

const (
	// TeslaHSCCompatibilityV1 is the initial public profile compatibility gate.
	TeslaHSCCompatibilityV1 = "tesla_hsc_modbus_v1"
	// TeslaHSCWCVitalsCompatibilityV1 is the exact operation contract gate for
	// the bounded WC vitals snapshot. It is not a general firmware claim.
	TeslaHSCWCVitalsCompatibilityV1 = "wc3_24_44_3"
)

// TeslaHSCProfileName identifies the profile only inside registry selection.
const TeslaHSCProfileName = "tesla_hsc"

const (
	teslaHSCFunction100 modbus.PrivateFunctionCode = 100
	teslaHSCFunction101 modbus.PrivateFunctionCode = 101
	teslaHSCFunction102 modbus.PrivateFunctionCode = 102
	maxTeslaHSCPayload                             = modbus.MaxPDUSize - 1
)

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
	// TeslaTEDAPIAdmissionAllowedWCVitals admits only the explicit bounded WC
	// vitals operation after all profile and operation gates pass.
	TeslaTEDAPIAdmissionAllowedWCVitals TeslaTEDAPIAdmissionState = "allowed_wc_vitals"
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
	Enabled                  bool
	Node                     byte
	PassiveCompatible        bool
	CompatibilityVersion     string
	WCVitalsOperationVersion string
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

// TeslaHSCEnvelope is a decoded exact-length request or FC100 data envelope.
type TeslaHSCEnvelope struct {
	function modbus.PrivateFunctionCode
	payload  []byte
}

// DecodeTeslaHSCEnvelope validates the FC100 data envelope. FC101 and FC102
// use the exact-length envelope only in the request direction.
func DecodeTeslaHSCEnvelope(
	function modbus.PrivateFunctionCode,
	payload []byte,
) (TeslaHSCEnvelope, error) {
	if function != teslaHSCFunction100 {
		return TeslaHSCEnvelope{}, fmt.Errorf("tesla HSC function is unsupported")
	}
	return decodeTeslaHSCExactEnvelope(function, payload)
}

// DecodeTeslaHSCRequestEnvelope validates the exact-length request envelope
// used by FC100, FC101, and FC102 without granting outbound admission.
func DecodeTeslaHSCRequestEnvelope(
	function modbus.PrivateFunctionCode,
	payload []byte,
) (TeslaHSCEnvelope, error) {
	if !isTeslaHSCFunction(function) {
		return TeslaHSCEnvelope{}, fmt.Errorf("tesla HSC function is unsupported")
	}
	return decodeTeslaHSCExactEnvelope(function, payload)
}

func decodeTeslaHSCExactEnvelope(
	function modbus.PrivateFunctionCode,
	payload []byte,
) (TeslaHSCEnvelope, error) {
	if len(payload) > maxTeslaHSCPayload {
		return TeslaHSCEnvelope{}, fmt.Errorf("tesla HSC payload exceeds bound")
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

// TeslaHSCResponse is one bounded direction-aware normal response. FC100
// retains decoded data bytes; FC101 and FC102 retain raw bytes opaquely.
type TeslaHSCResponse struct {
	function modbus.PrivateFunctionCode
	payload  []byte
}

// DecodeTeslaHSCResponse decodes FC100 data envelopes while retaining FC101
// and FC102 normal response payloads as bounded opaque bytes.
func DecodeTeslaHSCResponse(
	function modbus.PrivateFunctionCode,
	payload []byte,
) (TeslaHSCResponse, error) {
	if !isTeslaHSCFunction(function) {
		return TeslaHSCResponse{}, fmt.Errorf("tesla HSC function is unsupported")
	}
	if len(payload) > maxTeslaHSCPayload {
		return TeslaHSCResponse{}, fmt.Errorf("tesla HSC response exceeds bound")
	}
	if function == teslaHSCFunction100 {
		envelope, err := DecodeTeslaHSCEnvelope(function, payload)
		if err != nil {
			return TeslaHSCResponse{}, err
		}
		return TeslaHSCResponse{function: function, payload: envelope.Payload()}, nil
	}
	return TeslaHSCResponse{function: function, payload: append([]byte(nil), payload...)}, nil
}

// DecodeTeslaHSCExchange applies the selected Tesla response contract to one
// completed generic RTU exchange. It consumes only already-correlated normal
// payloads and never constructs or admits an outbound operation. FC100 may
// retain its bounded intermediate/result sequence; FC101 and FC102 retain one
// opaque terminal normal response.
func DecodeTeslaHSCExchange(
	function modbus.PrivateFunctionCode,
	payloads [][]byte,
) ([]TeslaHSCResponse, error) {
	if !isTeslaHSCFunction(function) {
		return nil, fmt.Errorf("tesla HSC function is unsupported")
	}
	if len(payloads) == 0 || len(payloads) > 8 ||
		(function != teslaHSCFunction100 && len(payloads) != 1) {
		return nil, fmt.Errorf("tesla HSC response count is invalid")
	}
	responses := make([]TeslaHSCResponse, 0, len(payloads))
	for _, payload := range payloads {
		response, err := DecodeTeslaHSCResponse(function, payload)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}

// Function returns the vendor function identified by this response.
func (response TeslaHSCResponse) Function() modbus.PrivateFunctionCode {
	return response.function
}

// Payload returns independent normal-response bytes under the selected
// direction contract.
func (response TeslaHSCResponse) Payload() []byte {
	return append([]byte(nil), response.payload...)
}

// Function returns the vendor function identified by this envelope.
func (envelope TeslaHSCEnvelope) Function() modbus.PrivateFunctionCode {
	return envelope.function
}

// Payload returns an independent copy of opaque inner bytes.
func (envelope TeslaHSCEnvelope) Payload() []byte {
	return append([]byte(nil), envelope.payload...)
}

func isTeslaHSCFunction(function modbus.PrivateFunctionCode) bool {
	switch function {
	case teslaHSCFunction100, teslaHSCFunction101, teslaHSCFunction102:
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
	function modbus.PrivateFunctionCode,
	status byte,
) TeslaHSCException {
	if !isTeslaHSCFunction(function) {
		return TeslaHSCExceptionUnknown
	}
	if status == 1 {
		return TeslaHSCExceptionUnknownFunction
	}
	if status == 4 &&
		(function == teslaHSCFunction101 || function == teslaHSCFunction102) {
		return TeslaHSCExceptionCodecFailure
	}
	return TeslaHSCExceptionUnknown
}

// TeslaHSCProvenance retains redacted, deterministic payload identity only.
type TeslaHSCProvenance struct {
	compatibilityVersion string
	node                 byte
	function             modbus.PrivateFunctionCode
	payloadLength        int
	payloadDigest        string
}

// TeslaHSCOpaqueTerminalProvenance is redacted metadata for one already-
// correlated FC101 or FC102 normal terminal response. It never retains or
// exposes raw response bytes.
type TeslaHSCOpaqueTerminalProvenance struct {
	function      modbus.PrivateFunctionCode
	payloadLength int
	payloadDigest string
}

// DecodeTeslaHSCOpaqueTerminalProvenance validates one selected opaque terminal
// exchange and retains only deterministic redacted response metadata. It does
// not construct a request or grant any outbound admission.
func DecodeTeslaHSCOpaqueTerminalProvenance(
	function modbus.PrivateFunctionCode,
	payloads [][]byte,
) (TeslaHSCOpaqueTerminalProvenance, error) {
	if function != teslaHSCFunction101 && function != teslaHSCFunction102 {
		return TeslaHSCOpaqueTerminalProvenance{}, fmt.Errorf("tesla HSC opaque terminal function is unsupported")
	}
	responses, err := DecodeTeslaHSCExchange(function, payloads)
	if err != nil {
		return TeslaHSCOpaqueTerminalProvenance{}, err
	}
	payload := responses[0].Payload()
	digest := sha256.Sum256(payload)
	return TeslaHSCOpaqueTerminalProvenance{
		function:      function,
		payloadLength: len(payload),
		payloadDigest: hex.EncodeToString(digest[:]),
	}, nil
}

// Function returns the selected opaque terminal-response function.
func (provenance TeslaHSCOpaqueTerminalProvenance) Function() modbus.PrivateFunctionCode {
	return provenance.function
}

// PayloadLength returns the bounded terminal-response byte count.
func (provenance TeslaHSCOpaqueTerminalProvenance) PayloadLength() int {
	return provenance.payloadLength
}

// PayloadDigest returns the deterministic redacted terminal-response digest.
func (provenance TeslaHSCOpaqueTerminalProvenance) PayloadDigest() string {
	return provenance.payloadDigest
}

// NewTeslaHSCProvenance creates provenance that contains no raw payload bytes.
func NewTeslaHSCProvenance(
	compatibilityVersion string,
	node byte,
	function modbus.PrivateFunctionCode,
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

// EncodeQualifiedFunction denies every outbound Tesla operation until a later
// typed, proven read-only contract adds an explicit operation allowlist.
func (profile TeslaHSCProfile) EncodeQualifiedFunction(operation string) (modbus.PrivateFunctionRequest, modbus.PrivateFunctionResponsePolicy, error) {
	if admission := profile.OperationAdmissionFor(operation); !admission.OutboundAllowed {
		return modbus.PrivateFunctionRequest{}, modbus.PrivateFunctionResponsePolicy{}, fmt.Errorf("tesla HSC operation is not admitted")
	}
	request, err := modbus.NewPrivateFunctionRequest(teslaHSCFunction100, teslaFC100WCVitalsRequestPDU)
	if err != nil {
		return modbus.PrivateFunctionRequest{}, modbus.PrivateFunctionResponsePolicy{}, err
	}
	return request, modbus.DefaultPrivateFunctionResponsePolicy(), nil
}

// DecodeQualifiedFunction validates only the Tesla profile normal-response
// contract. It is available to an already selected codec and does not itself
// grant outbound admission.
func (profile TeslaHSCProfile) DecodeQualifiedFunction(operation string, function modbus.PrivateFunctionCode, payload []byte) (QualifiedFunctionResult, error) {
	if operation == TeslaTEDAPIOperationWCVitalsV1 {
		if function != teslaHSCFunction100 {
			return QualifiedFunctionResult{}, fmt.Errorf("tesla HSC operation response function is invalid")
		}
		if _, err := DecodeTeslaFC100WCVitalsReplay(payload); err != nil {
			return QualifiedFunctionResult{}, err
		}
	}
	response, err := DecodeTeslaHSCResponse(function, payload)
	if err != nil {
		return QualifiedFunctionResult{}, err
	}
	return QualifiedFunctionResult{Payload: response.Payload()}, nil
}

// PayloadLength returns the retained opaque payload length.
func (provenance TeslaHSCProvenance) PayloadLength() int {
	return provenance.payloadLength
}

// PayloadDigest returns the redacted deterministic payload digest.
func (provenance TeslaHSCProvenance) PayloadDigest() string {
	return provenance.payloadDigest
}
