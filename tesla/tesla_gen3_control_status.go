package tesla

import (
	"context"
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// ControlStatusRequest selects at most one request form. Nil selects neither.
// Even the status poll replaces the stored control request (RS-485 Site Controller Protocol §6).
type ControlStatusRequest struct {
	StatusPoll    *bool
	ControlUpdate *ControlUpdate
}

// ControlUpdate carries the specified fields; nil scalars are omitted.
// ControllerModeHint and RequestExternalPowerRecheck are Provisional.
type ControlUpdate struct {
	ControlFlagOne              *bool
	ControllerModeHint          *uint32
	RequestExternalPowerRecheck *bool
}

// ControlStatusReply preserves the exact PDU payload and typed wire presence.
type ControlStatusReply struct {
	Snapshot *ControlStatusSnapshot
	native   HSCNativeRecord
}

// Payload returns a copy of the native response payload, including unknown fields.
func (reply ControlStatusReply) Payload() []byte { return reply.native.Payload() }

// ControlStatusSnapshot carries native values, not a charging confirmation.
// Current units and the energy direction associations are Provisional.
type ControlStatusSnapshot struct {
	OperatingStateCode                   *uint32
	StatusFlagTwo                        *bool
	CurrentLimitInput                    *float32
	DifferentialRMS                      *float32
	LocalReadings                        *ControlStatusReadings
	ObservedCurrent                      *float32
	StatusFlagSeven                      *bool
	ThresholdEventMaskEncoded            *uint32
	StatusMaskTwo                        *uint32
	VoltageFrequencyEventCountdownActive *bool
	LifetimeExportEnergy                 *float64
	LifetimeImportEnergy                 *float64
	StatusMaskThirteen                   *uint32
	StatusCodeFourteen                   *uint32
	StatusMaskOne                        *uint32
	StatusCodeSixteen                    *uint32
	StatusCodeSeventeen                  *uint32
	StatusCodeEighteen                   *uint32
	StatusMaskThree                      *uint32
	StatusCodeTwenty                     *uint32
}

// ControlStatusReadings retains the four native records of snapshot field 5.
type ControlStatusReadings struct {
	ReadingOne      *ControlStatusReading
	ReadingTwo      *ControlStatusReading
	VoltageEstimate *ControlStatusReading // Voltage unit Provisional; voltage_estimate_b on 26.x.
	Reserved        *NativeValueAndCode
}

// ControlStatusReading carries a float (fixed32 on the wire) and a reserved code.
type ControlStatusReading struct {
	Value *float32
	Code  *uint32
}

// EncodeTeslaControlStatus builds a typed HscControlStatus request (§6 and Appendix A).
func EncodeTeslaControlStatus(firmware string, input ControlStatusRequest) (modbus.PrivateFunctionRequest, error) {
	mid, _, supported := teslaNativeHSCLayout(firmware)
	if !supported {
		return modbus.PrivateFunctionRequest{}, fmt.Errorf("HscControlStatus layout not specified for firmware %q", firmware)
	}
	if input.StatusPoll != nil && input.ControlUpdate != nil {
		return modbus.PrivateFunctionRequest{}, fmt.Errorf("HscControlStatus request selects both oneof members")
	}
	body := appendTeslaCommandBool(nil, 8, input.StatusPoll)
	if update := input.ControlUpdate; update != nil {
		if mid && (update.ControllerModeHint != nil || update.RequestExternalPowerRecheck != nil) {
			return modbus.PrivateFunctionRequest{}, fmt.Errorf("HscControlStatus update fields not specified for firmware %q", firmware)
		}
		if update.ControllerModeHint != nil && *update.ControllerModeHint > 255 {
			return modbus.PrivateFunctionRequest{}, fmt.Errorf("HscControlStatus mode hint is outside 0–255")
		}
		fields := appendTeslaCommandBool(nil, 8, update.ControlFlagOne)
		fields = appendTeslaCommandUint(fields, 16, update.ControllerModeHint)
		fields = appendTeslaCommandBool(fields, 24, update.RequestExternalPowerRecheck)
		body = appendTeslaCommandMessage(body, 18, fields)
	}
	return BuildTeslaHSCNativeRequest(teslaHSCFunction101, body)
}

// DecodeTeslaControlStatus decodes one normal terminal; the transport owns exceptions.
func DecodeTeslaControlStatus(firmware string, payload []byte) (ControlStatusReply, error) {
	mid, _, supported := teslaNativeHSCLayout(firmware)
	if !supported {
		return ControlStatusReply{}, fmt.Errorf("HscControlStatus layout not specified for firmware %q", firmware)
	}
	envelope, err := decodeTeslaHSCExactEnvelope(teslaHSCFunction101, payload)
	if err != nil {
		return ControlStatusReply{}, err
	}
	reply := ControlStatusReply{native: HSCNativeRecord{function: teslaHSCFunction101, payload: cloneTeslaGen3Bytes(payload)}}
	err = decodeTeslaCommandFields(envelope.Payload(), teslaCommandNested(&reply.Snapshot, func(data []byte, snapshot *ControlStatusSnapshot) error {
		return decodeTeslaControlSnapshot(data, snapshot, mid)
	}))
	if err != nil {
		return ControlStatusReply{}, err
	}
	return reply, nil
}

func decodeTeslaControlSnapshot(data []byte, snapshot *ControlStatusSnapshot, mid bool) error {
	fields := []teslaCommandField{
		teslaCommandUint(&snapshot.OperatingStateCode), teslaCommandBool(&snapshot.StatusFlagTwo),
		teslaCommandFloat(&snapshot.CurrentLimitInput), teslaCommandFloat(&snapshot.DifferentialRMS),
		teslaCommandNested(&snapshot.LocalReadings, decodeTeslaControlReadings), teslaCommandFloat(&snapshot.ObservedCurrent),
		teslaCommandBool(&snapshot.StatusFlagSeven),
	}
	if !mid {
		fields = append(fields, teslaCommandUint(&snapshot.ThresholdEventMaskEncoded), teslaCommandUint(&snapshot.StatusMaskTwo),
			teslaCommandBool(&snapshot.VoltageFrequencyEventCountdownActive), teslaCommandDouble(&snapshot.LifetimeExportEnergy),
			teslaCommandDouble(&snapshot.LifetimeImportEnergy), teslaCommandUint(&snapshot.StatusMaskThirteen),
			teslaCommandUint(&snapshot.StatusCodeFourteen), teslaCommandUint(&snapshot.StatusMaskOne),
			teslaCommandUint(&snapshot.StatusCodeSixteen), teslaCommandUint(&snapshot.StatusCodeSeventeen),
			teslaCommandUint(&snapshot.StatusCodeEighteen), teslaCommandUint(&snapshot.StatusMaskThree), teslaCommandUint(&snapshot.StatusCodeTwenty))
	}
	return decodeTeslaCommandFields(data, fields...)
}

func decodeTeslaControlReadings(data []byte, readings *ControlStatusReadings) error {
	decodeReading := func(data []byte, reading *ControlStatusReading) error {
		return decodeTeslaCommandFields(data, teslaCommandFloat(&reading.Value), teslaCommandUint(&reading.Code))
	}
	return decodeTeslaCommandFields(data, teslaCommandNested(&readings.ReadingOne, decodeReading),
		teslaCommandNested(&readings.ReadingTwo, decodeReading), teslaCommandNested(&readings.VoltageEstimate, decodeReading),
		teslaCommandNested(&readings.Reserved, decodeTeslaNativeValueAndCode))
}

// ExchangeControlStatus sends exactly one explicit HscControlStatus request.
func (acquisition *Gen3ReadAcquisition) ExchangeControlStatus(ctx context.Context, input ControlStatusRequest) (Gen3ReadRecord, ControlStatusReply, error) {
	firmware := acquisition.config.Gen3.Version()
	request, err := EncodeTeslaControlStatus(firmware, input)
	var reply ControlStatusReply
	receipt, err := acquisition.runNativeCommand(ctx, teslaNativeCommand{name: "hsc.control_status", decodeHSC: func(payload []byte) error {
		var decodeErr error
		reply, decodeErr = DecodeTeslaControlStatus(firmware, payload)
		return decodeErr
	}}, request, err)
	if receipt.State != Gen3ReadEvidenceAck {
		reply = ControlStatusReply{}
	}
	return receipt, reply, err
}
