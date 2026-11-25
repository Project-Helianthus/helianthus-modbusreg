package tesla

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

type teslaNativeCommand struct {
	name      string
	spec      teslaFC100OperationSpec
	decodeHSC func([]byte) error
}

func (acquisition *Gen3ReadAcquisition) runNativeCommand(parent context.Context, command teslaNativeCommand, request modbus.PrivateFunctionRequest, encodeErr error) (Gen3ReadRecord, error) {
	noSend := func(err error) (Gen3ReadRecord, error) {
		return Gen3ReadRecord{NativeCommand: command.name, State: Gen3ReadEvidenceNoSend, Cause: err.Error()}, err
	}
	if encodeErr != nil {
		return noSend(fmt.Errorf("%w: %w", modbusreg.ErrQualifiedFunctionNoSend, encodeErr))
	}
	if parent == nil || parent.Err() != nil {
		return noSend(fmt.Errorf("%w: missing or canceled context", modbusreg.ErrQualifiedFunctionNoSend))
	}
	if err := acquisition.reserve(); err != nil {
		return noSend(err)
	}
	defer acquisition.release()
	ctx, cancel := context.WithTimeout(parent, acquisition.config.AcquisitionDeadline)
	defer cancel()
	var operation FC100Operation
	return acquisition.mutationExchangeWithCommand(ctx, operation, request, modbus.DefaultPrivateFunctionResponsePolicy(), &command), nil
}

// The RS-485 Site Controller Protocol §§4.2, 6.8 and 7.7 and Appendix A
// define exact layouts, not a version range.
func teslaNativeHSCLayout(firmware string) (mid, latest, supported bool) {
	if strings.TrimSpace(firmware) == "MID 25.2.2" {
		return true, false, true
	}
	version, ok := teslaReadFirmware(firmware)
	latest = ok && version == [3]uint64{26, 34, 0}
	return false, latest, latest || (ok && version == [3]uint64{26, 18, 0})
}

func appendTeslaCommandMessage(out []byte, key byte, body []byte) []byte {
	out = appendTeslaFC100Varint(append(out, key), uint64(len(body)))
	return append(out, body...)
}

func appendTeslaCommandUint(out []byte, key byte, value *uint32) []byte {
	if value == nil {
		return out
	}
	return appendTeslaFC100Varint(append(out, key), uint64(*value))
}

func appendTeslaCommandBool(out []byte, key byte, value *bool) []byte {
	if value == nil {
		return out
	}
	var bit byte
	if *value {
		bit = 1
	}
	return append(out, key, bit)
}

func appendTeslaCommandFixed(out []byte, key byte, value *uint32) []byte {
	if value == nil {
		return out
	}
	return binary.LittleEndian.AppendUint32(append(out, key), *value)
}

type teslaCommandField struct {
	wire uint8
	set  func([]byte) error
}

// Fields are indexed by wire number minus one; zero entries are unprojected.
// The exact native payload is retained separately, including unknown fields.
func decodeTeslaCommandFields(body []byte, fields ...teslaCommandField) error {
	var seen uint32
	for len(body) != 0 {
		field, size, err := decodeTeslaFC100RequestField(body)
		if err != nil {
			return err
		}
		if field.number <= uint64(len(fields)) {
			target := fields[field.number-1]
			if target.set != nil {
				bit := uint32(1) << (field.number - 1)
				if target.wire != field.wireType || seen&bit != 0 {
					return fmt.Errorf("native command field %d has wrong wire type or is duplicated", field.number)
				}
				seen |= bit
				value := field.value
				if target.wire != 2 {
					_, keySize, _ := decodeTeslaFC100Varint(body)
					value = body[keySize:size]
				}
				if err := target.set(value); err != nil {
					return err
				}
			}
		}
		body = body[size:]
	}
	return nil
}

func teslaCommandUint(target **uint32) teslaCommandField {
	return teslaCommandField{0, func(data []byte) error {
		value, _, _ := decodeTeslaFC100Varint(data)
		if value > math.MaxUint32 {
			return fmt.Errorf("native command field exceeds uint32")
		}
		n := uint32(value)
		*target = &n
		return nil
	}}
}

func teslaCommandBool(target **bool) teslaCommandField {
	return teslaCommandField{0, func(data []byte) error {
		value, _, _ := decodeTeslaFC100Varint(data)
		flag := value != 0
		*target = &flag
		return nil
	}}
}

func teslaCommandFixed(target **uint32) teslaCommandField {
	return teslaCommandField{5, func(data []byte) error {
		value := binary.LittleEndian.Uint32(data)
		*target = &value
		return nil
	}}
}

func teslaCommandFloat(target **float32) teslaCommandField {
	return teslaCommandField{5, func(data []byte) error {
		value := math.Float32frombits(binary.LittleEndian.Uint32(data))
		*target = &value
		return nil
	}}
}

func teslaCommandDouble(target **float64) teslaCommandField {
	return teslaCommandField{1, func(data []byte) error {
		value := math.Float64frombits(binary.LittleEndian.Uint64(data))
		*target = &value
		return nil
	}}
}

func teslaCommandNested[T any](target **T, decode func([]byte, *T) error) teslaCommandField {
	return teslaCommandField{2, func(data []byte) error {
		*target = new(T)
		return decode(data, *target)
	}}
}

func decodeTeslaNativeHSCTerminal(record Gen3ReadRecord, responses []modbus.RTUPrivateFunctionResponseADU, decode func([]byte) error) (Gen3ReadRecord, teslaGen3ReadFollowUp) {
	if len(responses) != 1 {
		record.State, record.Cause = Gen3ReadEvidenceIndeterminate, "native HSC command has no single terminal"
		return record, teslaGen3ReadFollowUpRetire
	}
	if err := decode(responses[0].Payload()); err != nil {
		record.State, record.Cause = Gen3ReadEvidenceIndeterminate, err.Error()
		return record, teslaGen3ReadFollowUpRetire
	}
	record.State = Gen3ReadEvidenceAck
	return record, teslaGen3ReadFollowUpNone
}
