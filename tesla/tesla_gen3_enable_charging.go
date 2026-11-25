package tesla

import (
	"context"
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// EncodeTeslaEnableCharging builds the start-charging request
// (TEDAPI Specification §§6.4.17 and 7.2).
func EncodeTeslaEnableCharging(firmware string) (modbus.PrivateFunctionRequest, error) {
	return encodeTeslaChargeCommand(firmware, 1)
}

// DecodeTeslaEnableCharging decodes one normal or application-error reply.
// A successful command receipt does not establish that charging started.
func DecodeTeslaEnableCharging(firmware string, payload []byte) (FC100OperationResult, error) {
	request, err := EncodeTeslaEnableCharging(firmware)
	return decodeTeslaChargeCommand(request, payload, err)
}

// EnableCharging sends one explicit start command. The receipt is not a
// readback or proof of a change in charging state.
func (acquisition *Gen3ReadAcquisition) EnableCharging(ctx context.Context) (Gen3ReadRecord, error) {
	request, err := EncodeTeslaEnableCharging(acquisition.config.Gen3.Version())
	return acquisition.runNativeCommand(ctx, teslaNativeCommand{name: "wc.enable_charging", spec: teslaChargeCommandSpec}, request, err)
}

var teslaChargeCommandSpec = teslaFC100OperationSpec{6, 39, 40}

func encodeTeslaChargeCommand(firmware string, value byte) (modbus.PrivateFunctionRequest, error) {
	version, ok := teslaReadFirmware(firmware)
	if !ok || (version != [3]uint64{26, 34, 0} && version != [3]uint64{26, 42, 0}) {
		return modbus.PrivateFunctionRequest{}, fmt.Errorf("RS-485 charge command layout not specified for firmware %q", firmware)
	}
	body := []byte{10, 2, 8, value}
	var operation FC100Operation
	if _, err := validateTeslaStructuredRequestBody(operation, body); err != nil {
		return modbus.PrivateFunctionRequest{}, err
	}
	return buildTeslaOperationRequest(teslaChargeCommandSpec, body)
}

func decodeTeslaChargeCommand(request modbus.PrivateFunctionRequest, payload []byte, encodeErr error) (FC100OperationResult, error) {
	if encodeErr != nil {
		return FC100OperationResult{}, encodeErr
	}
	results, err := decodeTeslaOperationSequence(teslaChargeCommandSpec, request.Payload(), [][]byte{payload})
	if err != nil {
		return FC100OperationResult{}, err
	}
	reply := results[0]
	if reply.Kind == FC100OperationTerminal {
		if err := decodeTeslaChargeCommandBody(reply.Body); err != nil {
			return FC100OperationResult{}, err
		}
	}
	return reply, nil
}

// The successful reply has no defined fields. Well-formed unknown fields
// remain in the complete body rather than becoming charging state.
func decodeTeslaChargeCommandBody(body []byte) error { return decodeTeslaCommandFields(body) }
