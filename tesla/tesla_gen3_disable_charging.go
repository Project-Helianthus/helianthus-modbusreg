package tesla

import (
	"context"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// EncodeTeslaDisableCharging builds the stop-charging request
// (TEDAPI Specification §§6.4.17 and 7.2).
func EncodeTeslaDisableCharging(firmware string) (modbus.PrivateFunctionRequest, error) {
	return encodeTeslaChargeCommand(firmware, 2)
}

// DecodeTeslaDisableCharging decodes one normal or application-error reply.
// A successful command receipt does not establish that charging stopped.
func DecodeTeslaDisableCharging(firmware string, payload []byte) (FC100OperationResult, error) {
	request, err := EncodeTeslaDisableCharging(firmware)
	return decodeTeslaChargeCommand(request, payload, err)
}

// DisableCharging sends one explicit stop command. The receipt is not a
// readback or proof of a change in charging state.
func (acquisition *Gen3ReadAcquisition) DisableCharging(ctx context.Context) (Gen3ReadRecord, error) {
	request, err := EncodeTeslaDisableCharging(acquisition.config.Gen3.Version())
	return acquisition.runNativeCommand(ctx, teslaNativeCommand{name: "wc.disable_charging", spec: teslaChargeCommandSpec}, request, err)
}
