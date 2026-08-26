package modbusreg

import (
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// TeslaGen3HSCNativeOperationDirection identifies the retained PDU direction.
type TeslaGen3HSCNativeOperationDirection string

const (
	TeslaGen3HSCRequestDirection  TeslaGen3HSCNativeOperationDirection = "request"
	TeslaGen3HSCResponseDirection TeslaGen3HSCNativeOperationDirection = "response"
)

// TeslaGen3HSCNativeOperationOutcome identifies the normal or exception path.
type TeslaGen3HSCNativeOperationOutcome string

const (
	TeslaGen3HSCNormalOutcome    TeslaGen3HSCNativeOperationOutcome = "normal"
	TeslaGen3HSCExceptionOutcome TeslaGen3HSCNativeOperationOutcome = "exception"
)

// TeslaGen3HSCNativeOperationRecordConfig supplies one bounded Gen3 native record.
type TeslaGen3HSCNativeOperationRecordConfig struct {
	Profile          string
	ProfileVersion   string
	OperationVersion string
	Operation        TeslaFC100Operation
	Function         modbus.PrivateFunctionCode
	Direction        TeslaGen3HSCNativeOperationDirection
	Outcome          TeslaGen3HSCNativeOperationOutcome
	Payload          []byte
}

// TeslaGen3HSCNativeOperationRecord retains a profile-scoped native operation PDU.
type TeslaGen3HSCNativeOperationRecord struct {
	profile          string
	profileVersion   string
	operationVersion string
	operation        TeslaFC100Operation
	function         modbus.PrivateFunctionCode
	direction        TeslaGen3HSCNativeOperationDirection
	outcome          TeslaGen3HSCNativeOperationOutcome
	payload          []byte
}

// NewTeslaGen3HSCNativeOperationRecord validates and defensively retains one
// bounded Gen3 request, normal response, or transport-correlated exception.
func NewTeslaGen3HSCNativeOperationRecord(config TeslaGen3HSCNativeOperationRecordConfig) (TeslaGen3HSCNativeOperationRecord, error) {
	if config.Profile != TeslaHSCProfileName || config.ProfileVersion != TeslaHSCCompatibilityV1 {
		return TeslaGen3HSCNativeOperationRecord{}, fmt.Errorf("tesla Gen3 HSC profile is unsupported")
	}
	if !isTeslaHSCFunction(config.Function) {
		return TeslaGen3HSCNativeOperationRecord{}, fmt.Errorf("tesla Gen3 HSC function is unsupported")
	}
	if config.Direction != TeslaGen3HSCRequestDirection && config.Direction != TeslaGen3HSCResponseDirection {
		return TeslaGen3HSCNativeOperationRecord{}, fmt.Errorf("tesla Gen3 HSC direction is invalid")
	}
	if config.Outcome != TeslaGen3HSCNormalOutcome && config.Outcome != TeslaGen3HSCExceptionOutcome {
		return TeslaGen3HSCNativeOperationRecord{}, fmt.Errorf("tesla Gen3 HSC outcome is invalid")
	}
	if len(config.Payload) > maxTeslaHSCPayload {
		return TeslaGen3HSCNativeOperationRecord{}, fmt.Errorf("tesla Gen3 HSC native payload exceeds bound")
	}
	if config.Function == teslaHSCFunction100 {
		if _, ok := teslaFC100OperationSpecs[config.Operation]; !ok || config.OperationVersion != TeslaHSCFC100OperationCompatibilityV1 {
			return TeslaGen3HSCNativeOperationRecord{}, fmt.Errorf("tesla Gen3 FC100 operation context is invalid")
		}
	} else if config.Operation != "" || config.OperationVersion != "" {
		return TeslaGen3HSCNativeOperationRecord{}, fmt.Errorf("tesla Gen3 FC101 or FC102 operation is opaque")
	}
	return TeslaGen3HSCNativeOperationRecord{
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

func (record TeslaGen3HSCNativeOperationRecord) Profile() string { return record.profile }

func (record TeslaGen3HSCNativeOperationRecord) ProfileVersion() string { return record.profileVersion }

func (record TeslaGen3HSCNativeOperationRecord) OperationVersion() string {
	return record.operationVersion
}

func (record TeslaGen3HSCNativeOperationRecord) Operation() TeslaFC100Operation {
	return record.operation
}

func (record TeslaGen3HSCNativeOperationRecord) Function() modbus.PrivateFunctionCode {
	return record.function
}

func (record TeslaGen3HSCNativeOperationRecord) Direction() TeslaGen3HSCNativeOperationDirection {
	return record.direction
}

func (record TeslaGen3HSCNativeOperationRecord) Outcome() TeslaGen3HSCNativeOperationOutcome {
	return record.outcome
}

// Payload returns a defensive copy of the retained bounded native PDU payload.
func (record TeslaGen3HSCNativeOperationRecord) Payload() []byte {
	return append([]byte(nil), record.payload...)
}
