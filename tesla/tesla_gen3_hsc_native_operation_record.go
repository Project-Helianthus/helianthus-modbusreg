package tesla

import (
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// Gen3HSCNativeOperationDirection identifies the retained PDU direction.
type Gen3HSCNativeOperationDirection string

// Directions. An exception record must use the response direction.
const (
	Gen3HSCRequestDirection  Gen3HSCNativeOperationDirection = "request"
	Gen3HSCResponseDirection Gen3HSCNativeOperationDirection = "response"
)

// Gen3HSCNativeOperationOutcome identifies the normal or exception path.
type Gen3HSCNativeOperationOutcome string

// Outcomes: Normal for a request or normal response, Exception for a
// transport-correlated exception response.
const (
	Gen3HSCNormalOutcome    Gen3HSCNativeOperationOutcome = "normal"
	Gen3HSCExceptionOutcome Gen3HSCNativeOperationOutcome = "exception"
)

// Gen3HSCNativeOperationRecordConfig supplies one bounded Gen3 native record.
type Gen3HSCNativeOperationRecordConfig struct {
	Profile          string
	ProfileVersion   string
	OperationVersion string
	Operation        FC100Operation
	Function         modbus.PrivateFunctionCode
	Direction        Gen3HSCNativeOperationDirection
	Outcome          Gen3HSCNativeOperationOutcome
	Payload          []byte
}

// Gen3HSCNativeOperationRecord retains a profile-scoped native operation PDU.
type Gen3HSCNativeOperationRecord struct {
	profile          string
	profileVersion   string
	operationVersion string
	operation        FC100Operation
	function         modbus.PrivateFunctionCode
	direction        Gen3HSCNativeOperationDirection
	outcome          Gen3HSCNativeOperationOutcome
	payload          []byte
}

// NewGen3HSCNativeOperationRecord validates and defensively retains one
// bounded Gen3 request, normal response, or transport-correlated exception.
func NewGen3HSCNativeOperationRecord(config Gen3HSCNativeOperationRecordConfig) (Gen3HSCNativeOperationRecord, error) {
	if config.Profile != HSCProfileName || config.ProfileVersion != HSCCompatibilityV1 {
		return Gen3HSCNativeOperationRecord{}, fmt.Errorf("tesla Gen3 HSC profile is unsupported")
	}
	if !isTeslaHSCFunction(config.Function) {
		return Gen3HSCNativeOperationRecord{}, fmt.Errorf("tesla Gen3 HSC function is unsupported")
	}
	if config.Direction != Gen3HSCRequestDirection && config.Direction != Gen3HSCResponseDirection {
		return Gen3HSCNativeOperationRecord{}, fmt.Errorf("tesla Gen3 HSC direction is invalid")
	}
	if config.Outcome != Gen3HSCNormalOutcome && config.Outcome != Gen3HSCExceptionOutcome {
		return Gen3HSCNativeOperationRecord{}, fmt.Errorf("tesla Gen3 HSC outcome is invalid")
	}
	if config.Direction == Gen3HSCRequestDirection && config.Outcome == Gen3HSCExceptionOutcome {
		return Gen3HSCNativeOperationRecord{}, fmt.Errorf("tesla Gen3 HSC exception must be a response")
	}
	if len(config.Payload) > maxTeslaHSCPayload {
		return Gen3HSCNativeOperationRecord{}, fmt.Errorf("tesla Gen3 HSC native payload exceeds bound")
	}
	if config.Function == teslaHSCFunction100 {
		if _, ok := teslaFC100OperationSpecs[config.Operation]; !ok || config.OperationVersion != HSCFC100OperationCompatibilityV1 {
			return Gen3HSCNativeOperationRecord{}, fmt.Errorf("tesla Gen3 FC100 operation context is invalid")
		}
	} else if config.Operation != "" || config.OperationVersion != "" {
		return Gen3HSCNativeOperationRecord{}, fmt.Errorf("tesla Gen3 FC101 or FC102 operation is opaque")
	}
	return Gen3HSCNativeOperationRecord{
		profile:          config.Profile,
		profileVersion:   config.ProfileVersion,
		operationVersion: config.OperationVersion,
		operation:        config.Operation,
		function:         config.Function,
		direction:        config.Direction,
		outcome:          config.Outcome,
		payload:          append([]byte(nil), config.Payload...),
	}, nil
}

// Profile returns the validated HSC profile name.
func (record Gen3HSCNativeOperationRecord) Profile() string { return record.profile }

// ProfileVersion returns the validated HSC profile compatibility identifier.
func (record Gen3HSCNativeOperationRecord) ProfileVersion() string { return record.profileVersion }

// OperationVersion returns the FC100 operation compatibility identifier, or the
// empty string for an opaque FC101 or FC102 record.
func (record Gen3HSCNativeOperationRecord) OperationVersion() string {
	return record.operationVersion
}

// Operation returns the FC100 operation, or the empty string for FC101 and FC102.
func (record Gen3HSCNativeOperationRecord) Operation() FC100Operation {
	return record.operation
}

// Function returns the HSC private function code of the retained PDU.
func (record Gen3HSCNativeOperationRecord) Function() modbus.PrivateFunctionCode {
	return record.function
}

// Direction returns whether the payload is a request or a response.
func (record Gen3HSCNativeOperationRecord) Direction() Gen3HSCNativeOperationDirection {
	return record.direction
}

// Outcome returns the normal or exception path of the retained PDU.
func (record Gen3HSCNativeOperationRecord) Outcome() Gen3HSCNativeOperationOutcome {
	return record.outcome
}

// Payload returns a defensive copy of the retained bounded native PDU payload.
func (record Gen3HSCNativeOperationRecord) Payload() []byte {
	return append([]byte(nil), record.payload...)
}
