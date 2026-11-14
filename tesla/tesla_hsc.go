package tesla

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

const (
	// HSCCompatibilityV1 is the initial public profile compatibility gate.
	HSCCompatibilityV1 = "tesla_hsc_modbus_v1"
	// HSCWCVitalsCompatibilityV1 is the exact operation contract gate for
	// the bounded WC vitals snapshot. It is not a general firmware claim.
	HSCWCVitalsCompatibilityV1 = "wc3_24_44_3"
	// HSCSystemInfoCompatibilityV1 is the exact operation contract gate
	// for the bounded Common system-information snapshot.
	HSCSystemInfoCompatibilityV1 = "wc3_24_44_3"
	// HSCWCLifetimeCompatibilityV1 is the exact operation contract gate for
	// the bounded WC lifetime snapshot.
	HSCWCLifetimeCompatibilityV1 = "wc3_24_44_3"
	// HSCWCSystemInfoCompatibilityV1 is the exact operation contract gate
	// for the bounded WC system-information snapshot.
	HSCWCSystemInfoCompatibilityV1 = "wc3_24_44_3"
	// HSCWCLoadSharingStateCompatibilityV1 is the exact operation
	// contract gate for the bounded WC load-sharing-state replay.
	HSCWCLoadSharingStateCompatibilityV1 = "wc3_24_44_3"
)

// HSCProfileName identifies the profile only inside registry selection.
const HSCProfileName = "tesla_hsc"

const (
	teslaHSCFunction100 modbus.PrivateFunctionCode = 100
	teslaHSCFunction101 modbus.PrivateFunctionCode = 101
	teslaHSCFunction102 modbus.PrivateFunctionCode = 102
	maxTeslaHSCPayload                             = modbus.MaxPDUSize - 1
)

// HSCDisposition records whether a configured profile can do more than
// parse bounded frames. Qualification never grants opaque outbound admission.
type HSCDisposition string

// Dispositions. Disabled: the configuration is not enabled. FramingOnly:
// enabled, but not passive-compatible under HSCCompatibilityV1; frames are
// parsed and no operation is admitted. QualifiedReadOnly: enabled,
// passive-compatible and on HSCCompatibilityV1; operation admission is
// still decided per operation.
const (
	HSCDisabled          HSCDisposition = "disabled"
	HSCFramingOnly       HSCDisposition = "framing_only"
	HSCQualifiedReadOnly HSCDisposition = "qualified_read_only"
)

// TEDAPIAdmissionState distinguishes profile qualification from the
// separate, per-operation wire-admission decision.
type TEDAPIAdmissionState string

const (
	// TEDAPIAdmissionBlockedProfile denies an unqualified profile.
	TEDAPIAdmissionBlockedProfile TEDAPIAdmissionState = "blocked_profile"
	// TEDAPIAdmissionBlockedNoAdmissibleOperation denies a qualified
	// profile until a later contract proves one particular operation safe.
	TEDAPIAdmissionBlockedNoAdmissibleOperation TEDAPIAdmissionState = "blocked_no_admissible_operation"
	// TEDAPIAdmissionAllowedWCVitals admits only the explicit bounded WC
	// vitals operation after all profile and operation gates pass.
	TEDAPIAdmissionAllowedWCVitals TEDAPIAdmissionState = "allowed_wc_vitals"
	// TEDAPIAdmissionAllowedCommonSystemInfo admits only the explicit
	// bounded Common system-information operation after all gates pass.
	TEDAPIAdmissionAllowedCommonSystemInfo TEDAPIAdmissionState = "allowed_common_system_info"
	// TEDAPIAdmissionAllowedWCLifetime admits only the explicit bounded WC
	// lifetime operation after all gates pass.
	TEDAPIAdmissionAllowedWCLifetime TEDAPIAdmissionState = "allowed_wc_lifetime"
	// TEDAPIAdmissionAllowedWCSystemInfo admits only the explicit bounded
	// WC system-information operation after all gates pass.
	TEDAPIAdmissionAllowedWCSystemInfo TEDAPIAdmissionState = "allowed_wc_system_info"
	// TEDAPIAdmissionAllowedWCLoadSharingState admits only the explicit
	// bounded WC load-sharing-state operation after all gates pass.
	TEDAPIAdmissionAllowedWCLoadSharingState TEDAPIAdmissionState = "allowed_wc_load_sharing_state"
)

// TEDAPIOperationAdmission is the redacted result of local admission
// policy. It deliberately carries no request bytes or inferred operation name.
type TEDAPIOperationAdmission struct {
	State           TEDAPIAdmissionState
	OutboundAllowed bool
}

// HSCProfileConfig is an explicit local flavor configuration. A matching
// node or a readable frame is not a substitute for this configuration.
type HSCProfileConfig struct {
	Enabled                            bool
	Node                               byte
	PassiveCompatible                  bool
	CompatibilityVersion               string
	WCVitalsOperationVersion           string
	SystemInfoOperationVersion         string
	WCLifetimeOperationVersion         string
	WCSystemInfoOperationVersion       string
	WCLoadSharingStateOperationVersion string
}

// HSCProfile retains the immutable profile gate decision.
type HSCProfile struct {
	config      HSCProfileConfig
	disposition HSCDisposition
}

// NewHSCProfile validates explicit configuration and calculates its
// fail-closed disposition without transmitting any vendor request.
func NewHSCProfile(config HSCProfileConfig) (HSCProfile, error) {
	if config.Node == 0 || config.Node > 247 {
		return HSCProfile{}, fmt.Errorf("tesla HSC node is invalid")
	}
	if config.CompatibilityVersion == "" {
		return HSCProfile{}, fmt.Errorf("tesla HSC compatibility version is required")
	}
	disposition := HSCDisabled
	if config.Enabled {
		disposition = HSCFramingOnly
		if config.PassiveCompatible &&
			config.CompatibilityVersion == HSCCompatibilityV1 {
			disposition = HSCQualifiedReadOnly
		}
	}
	return HSCProfile{config: config, disposition: disposition}, nil
}

// Node returns the explicitly configured RTU node.
func (profile HSCProfile) Node() byte {
	return profile.config.Node
}

// Disposition returns the fail-closed profile qualification result.
func (profile HSCProfile) Disposition() HSCDisposition {
	return profile.disposition
}

// OutboundAllowed always denies wire transmission in the opaque initial
// profile. A later typed, read-only operation contract must opt in separately.
func (HSCProfile) OutboundAllowed() bool {
	return false
}

// OperationAdmission reports that initial profile qualification never itself
// authorizes an opaque vendor request. A later contract must add a typed,
// allowlisted operation before this result can become sendable.
func (profile HSCProfile) OperationAdmission() TEDAPIOperationAdmission {
	if profile.Disposition() != HSCQualifiedReadOnly {
		return TEDAPIOperationAdmission{State: TEDAPIAdmissionBlockedProfile}
	}
	return TEDAPIOperationAdmission{
		State: TEDAPIAdmissionBlockedNoAdmissibleOperation,
	}
}

// HSCEnvelope is a decoded exact-length request or FC100 data envelope.
type HSCEnvelope struct {
	function modbus.PrivateFunctionCode
	payload  []byte
}

// DecodeTeslaHSCEnvelope validates the FC100 data envelope. FC101 and FC102
// use the exact-length envelope only in the request direction.
func DecodeTeslaHSCEnvelope(
	function modbus.PrivateFunctionCode,
	payload []byte,
) (HSCEnvelope, error) {
	if function != teslaHSCFunction100 {
		return HSCEnvelope{}, fmt.Errorf("tesla HSC function is unsupported")
	}
	return decodeTeslaHSCExactEnvelope(function, payload)
}

// DecodeTeslaHSCRequestEnvelope validates the exact-length request envelope
// used by FC100, FC101, and FC102 without granting outbound admission.
func DecodeTeslaHSCRequestEnvelope(
	function modbus.PrivateFunctionCode,
	payload []byte,
) (HSCEnvelope, error) {
	if !isTeslaHSCFunction(function) {
		return HSCEnvelope{}, fmt.Errorf("tesla HSC function is unsupported")
	}
	return decodeTeslaHSCExactEnvelope(function, payload)
}

func decodeTeslaHSCExactEnvelope(
	function modbus.PrivateFunctionCode,
	payload []byte,
) (HSCEnvelope, error) {
	if len(payload) > maxTeslaHSCPayload {
		return HSCEnvelope{}, fmt.Errorf("tesla HSC payload exceeds bound")
	}
	if len(payload) == 0 {
		return HSCEnvelope{}, fmt.Errorf("tesla HSC payload has no length prefix")
	}
	if int(payload[0]) != len(payload)-1 {
		return HSCEnvelope{}, fmt.Errorf("tesla HSC length prefix is inexact")
	}
	return HSCEnvelope{
		function: function,
		payload:  append([]byte(nil), payload[1:]...),
	}, nil
}

// HSCResponse is one bounded direction-aware normal response. FC100
// retains decoded data bytes; FC101 and FC102 retain raw bytes opaquely.
type HSCResponse struct {
	function modbus.PrivateFunctionCode
	payload  []byte
}

// DecodeTeslaHSCResponse decodes FC100 data envelopes while retaining FC101
// and FC102 normal response payloads as bounded opaque bytes.
func DecodeTeslaHSCResponse(
	function modbus.PrivateFunctionCode,
	payload []byte,
) (HSCResponse, error) {
	if !isTeslaHSCFunction(function) {
		return HSCResponse{}, fmt.Errorf("tesla HSC function is unsupported")
	}
	if len(payload) > maxTeslaHSCPayload {
		return HSCResponse{}, fmt.Errorf("tesla HSC response exceeds bound")
	}
	if function == teslaHSCFunction100 {
		envelope, err := DecodeTeslaHSCEnvelope(function, payload)
		if err != nil {
			return HSCResponse{}, err
		}
		return HSCResponse{function: function, payload: envelope.Payload()}, nil
	}
	return HSCResponse{function: function, payload: append([]byte(nil), payload...)}, nil
}

// DecodeTeslaHSCExchange applies the selected Tesla response contract to one
// completed generic RTU exchange. It consumes only already-correlated normal
// payloads and never constructs or admits an outbound operation. FC100 may
// retain its bounded intermediate/result sequence; FC101 and FC102 retain one
// opaque terminal normal response.
func DecodeTeslaHSCExchange(
	function modbus.PrivateFunctionCode,
	payloads [][]byte,
) ([]HSCResponse, error) {
	if !isTeslaHSCFunction(function) {
		return nil, fmt.Errorf("tesla HSC function is unsupported")
	}
	if len(payloads) == 0 || len(payloads) > 8 ||
		(function != teslaHSCFunction100 && len(payloads) != 1) {
		return nil, fmt.Errorf("tesla HSC response count is invalid")
	}
	responses := make([]HSCResponse, 0, len(payloads))
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
func (response HSCResponse) Function() modbus.PrivateFunctionCode {
	return response.function
}

// Payload returns independent normal-response bytes under the selected
// direction contract.
func (response HSCResponse) Payload() []byte {
	return append([]byte(nil), response.payload...)
}

// Function returns the vendor function identified by this envelope.
func (envelope HSCEnvelope) Function() modbus.PrivateFunctionCode {
	return envelope.function
}

// Payload returns an independent copy of opaque inner bytes.
func (envelope HSCEnvelope) Payload() []byte {
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

// HSCException classifies only the documented opaque response outcomes.
type HSCException string

// Exception classes. UnknownFunction: status 1 on any HSC function.
// CodecFailure: status 4 on FC101 or FC102. Unknown: every other status, or a
// function outside FC100 to FC102.
const (
	HSCExceptionUnknown         HSCException = "unknown"
	HSCExceptionUnknownFunction HSCException = "unknown_function"
	HSCExceptionCodecFailure    HSCException = "codec_failure"
)

// ClassifyTeslaHSCException classifies exception status without decoding any
// opaque vendor payload or inventing field semantics.
func ClassifyTeslaHSCException(
	function modbus.PrivateFunctionCode,
	status byte,
) HSCException {
	if !isTeslaHSCFunction(function) {
		return HSCExceptionUnknown
	}
	if status == 1 {
		return HSCExceptionUnknownFunction
	}
	if status == 4 &&
		(function == teslaHSCFunction101 || function == teslaHSCFunction102) {
		return HSCExceptionCodecFailure
	}
	return HSCExceptionUnknown
}

// HSCProvenance retains redacted, deterministic payload identity only.
type HSCProvenance struct {
	compatibilityVersion string
	node                 byte
	function             modbus.PrivateFunctionCode
	payloadLength        int
	payloadDigest        string
}

// HSCOpaqueTerminalProvenance is redacted metadata for one already-
// correlated FC101 or FC102 normal terminal response. It never retains or
// exposes raw response bytes.
type HSCOpaqueTerminalProvenance struct {
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
) (HSCOpaqueTerminalProvenance, error) {
	if function != teslaHSCFunction101 && function != teslaHSCFunction102 {
		return HSCOpaqueTerminalProvenance{}, fmt.Errorf("tesla HSC opaque terminal function is unsupported")
	}
	responses, err := DecodeTeslaHSCExchange(function, payloads)
	if err != nil {
		return HSCOpaqueTerminalProvenance{}, err
	}
	payload := responses[0].Payload()
	digest := sha256.Sum256(payload)
	return HSCOpaqueTerminalProvenance{
		function:      function,
		payloadLength: len(payload),
		payloadDigest: hex.EncodeToString(digest[:]),
	}, nil
}

// Function returns the selected opaque terminal-response function.
func (provenance HSCOpaqueTerminalProvenance) Function() modbus.PrivateFunctionCode {
	return provenance.function
}

// PayloadLength returns the bounded terminal-response byte count.
func (provenance HSCOpaqueTerminalProvenance) PayloadLength() int {
	return provenance.payloadLength
}

// PayloadDigest returns the deterministic redacted terminal-response digest.
func (provenance HSCOpaqueTerminalProvenance) PayloadDigest() string {
	return provenance.payloadDigest
}

// NewHSCProvenance creates provenance that contains no raw payload bytes.
func NewHSCProvenance(
	compatibilityVersion string,
	node byte,
	function modbus.PrivateFunctionCode,
	payload []byte,
) HSCProvenance {
	digest := sha256.Sum256(payload)
	return HSCProvenance{
		compatibilityVersion: compatibilityVersion,
		node:                 node,
		function:             function,
		payloadLength:        len(payload),
		payloadDigest:        hex.EncodeToString(digest[:]),
	}
}

// EncodeQualifiedFunction constructs only an explicitly qualified, bounded
// read-only operation. Every unknown operation remains no-send.
func (profile HSCProfile) EncodeQualifiedFunction(operation string) (modbus.PrivateFunctionRequest, modbus.PrivateFunctionResponsePolicy, error) {
	if admission := profile.OperationAdmissionFor(operation); !admission.OutboundAllowed {
		return modbus.PrivateFunctionRequest{}, modbus.PrivateFunctionResponsePolicy{}, fmt.Errorf("tesla HSC operation is not admitted")
	}
	var payload []byte
	switch operation {
	case TEDAPIOperationWCVitalsV1:
		payload = teslaFC100WCVitalsRequestPDU
	case TEDAPIOperationCommonSystemInfoV1:
		payload = teslaFC100CommonSystemInfoRequestPDU
	case TEDAPIOperationWCLifetimeV1:
		payload = teslaFC100WCLifetimeRequestPDU
	case TEDAPIOperationWCSystemInfoV1:
		payload = teslaFC100WCSystemInfoRequestPDU
	case TEDAPIOperationWCLoadSharingStateV1:
		payload = teslaFC100WCLoadSharingStateRequestPDU
	default:
		return modbus.PrivateFunctionRequest{}, modbus.PrivateFunctionResponsePolicy{}, fmt.Errorf("tesla HSC operation is not admitted")
	}
	request, err := modbus.NewPrivateFunctionRequest(teslaHSCFunction100, payload)
	if err != nil {
		return modbus.PrivateFunctionRequest{}, modbus.PrivateFunctionResponsePolicy{}, err
	}
	return request, modbus.DefaultPrivateFunctionResponsePolicy(), nil
}

// DecodeQualifiedFunction validates only the Tesla profile normal-response
// contract. It is available to an already selected codec and does not itself
// grant outbound admission.
func (profile HSCProfile) DecodeQualifiedFunction(operation string, function modbus.PrivateFunctionCode, payload []byte) (modbusreg.QualifiedFunctionResult, error) {
	if operation == TEDAPIOperationWCVitalsV1 {
		if function != teslaHSCFunction100 {
			return modbusreg.QualifiedFunctionResult{}, fmt.Errorf("tesla HSC operation response function is invalid")
		}
		replay, err := DecodeTeslaFC100WCVitalsReplay(payload)
		if err != nil {
			return modbusreg.QualifiedFunctionResult{}, err
		}
		return modbusreg.QualifiedFunctionResult{Replay: &modbusreg.QualifiedFunctionReplay{
			Kind: string(replay.Kind), PayloadLength: replay.SnapshotLength, PayloadDigest: replay.SnapshotDigest,
		}}, nil
	}
	if operation == TEDAPIOperationCommonSystemInfoV1 {
		if function != teslaHSCFunction100 {
			return modbusreg.QualifiedFunctionResult{}, fmt.Errorf("tesla HSC operation response function is invalid")
		}
		replay, err := DecodeTeslaFC100CommonSystemInfoReplay(payload)
		if err != nil {
			return modbusreg.QualifiedFunctionResult{}, err
		}
		return modbusreg.QualifiedFunctionResult{Replay: &modbusreg.QualifiedFunctionReplay{
			Kind: string(replay.Kind), PayloadLength: replay.SnapshotLength, PayloadDigest: replay.SnapshotDigest,
		}}, nil
	}
	if operation == TEDAPIOperationWCLifetimeV1 {
		if function != teslaHSCFunction100 {
			return modbusreg.QualifiedFunctionResult{}, fmt.Errorf("tesla HSC operation response function is invalid")
		}
		replay, err := DecodeTeslaFC100WCLifetimeReplay(payload)
		if err != nil {
			return modbusreg.QualifiedFunctionResult{}, err
		}
		return modbusreg.QualifiedFunctionResult{Replay: &modbusreg.QualifiedFunctionReplay{Kind: string(replay.Kind), PayloadLength: replay.SnapshotLength, PayloadDigest: replay.SnapshotDigest}}, nil
	}
	if operation == TEDAPIOperationWCSystemInfoV1 {
		if function != teslaHSCFunction100 {
			return modbusreg.QualifiedFunctionResult{}, fmt.Errorf("tesla HSC operation response function is invalid")
		}
		replay, err := DecodeTeslaFC100WCSystemInfoReplay(payload)
		if err != nil {
			return modbusreg.QualifiedFunctionResult{}, err
		}
		return modbusreg.QualifiedFunctionResult{Replay: &modbusreg.QualifiedFunctionReplay{Kind: string(replay.Kind), PayloadLength: replay.SnapshotLength, PayloadDigest: replay.SnapshotDigest}}, nil
	}
	if operation == TEDAPIOperationWCLoadSharingStateV1 {
		if function != teslaHSCFunction100 {
			return modbusreg.QualifiedFunctionResult{}, fmt.Errorf("tesla HSC operation response function is invalid")
		}
		replay, err := DecodeTeslaFC100WCLoadSharingStateReplay(payload)
		if err != nil {
			return modbusreg.QualifiedFunctionResult{}, err
		}
		return modbusreg.QualifiedFunctionResult{Replay: &modbusreg.QualifiedFunctionReplay{Kind: string(replay.Kind), PayloadLength: replay.SnapshotLength, PayloadDigest: replay.SnapshotDigest}}, nil
	}
	response, err := DecodeTeslaHSCResponse(function, payload)
	if err != nil {
		return modbusreg.QualifiedFunctionResult{}, err
	}
	return modbusreg.QualifiedFunctionResult{Payload: response.Payload()}, nil
}

// DecodeQualifiedFunctionSequence decodes a complete FC100 response sequence
// for the WC lifetime, WC system-information and WC load-sharing-state
// operations into redacted replays (kind, length, digest). Any other function
// or operation, or an invalid sequence, returns an error.
func (profile HSCProfile) DecodeQualifiedFunctionSequence(operation string, function modbus.PrivateFunctionCode, payloads [][]byte) ([]modbusreg.QualifiedFunctionResult, error) {
	if function != teslaHSCFunction100 {
		return nil, fmt.Errorf("tesla HSC operation response sequence is invalid")
	}
	var replays []FC100WCLifetimeReplay
	var err error
	switch operation {
	case TEDAPIOperationWCLifetimeV1:
		replays, err = DecodeTeslaFC100WCLifetimeReplaySequence(payloads)
	case TEDAPIOperationWCSystemInfoV1:
		return decodeTeslaFC100WCSystemInfoSequence(payloads)
	case TEDAPIOperationWCLoadSharingStateV1:
		return decodeTeslaFC100WCLoadSharingStateSequence(payloads)
	default:
		return nil, fmt.Errorf("tesla HSC operation response sequence is invalid")
	}
	if err != nil {
		return nil, err
	}
	results := make([]modbusreg.QualifiedFunctionResult, len(replays))
	for i, replay := range replays {
		results[i] = modbusreg.QualifiedFunctionResult{Replay: &modbusreg.QualifiedFunctionReplay{Kind: string(replay.Kind), PayloadLength: replay.SnapshotLength, PayloadDigest: replay.SnapshotDigest}}
	}
	return results, nil
}

// HandlesQualifiedFunctionSequence reports whether an operation is decoded as a
// whole sequence by DecodeQualifiedFunctionSequence rather than per response.
func (HSCProfile) HandlesQualifiedFunctionSequence(operation string) bool {
	return operation == TEDAPIOperationWCLifetimeV1 ||
		operation == TEDAPIOperationWCSystemInfoV1 ||
		operation == TEDAPIOperationWCLoadSharingStateV1
}

// PayloadLength returns the retained opaque payload length.
func (provenance HSCProvenance) PayloadLength() int {
	return provenance.payloadLength
}

// PayloadDigest returns the redacted deterministic payload digest.
func (provenance HSCProvenance) PayloadDigest() string {
	return provenance.payloadDigest
}
