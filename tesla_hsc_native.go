package modbusreg

import (
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// TeslaHSCNativeRecord retains one bounded FC101 or FC102 native message.
// Its payload is deliberately anonymous: numeric protobuf structure may be
// inspected by callers without assigning field names or units.
type TeslaHSCNativeRecord struct {
	function modbus.PrivateFunctionCode
	payload  []byte
}

// BuildTeslaHSCNativeRequest constructs a syntactically valid native FC101 or
// FC102 request. It performs no exchange and accepts locally mutating message
// shapes as opaque caller-supplied bytes.
func BuildTeslaHSCNativeRequest(function modbus.PrivateFunctionCode, message []byte) (modbus.PrivateFunctionRequest, error) {
	if function != teslaHSCFunction101 && function != teslaHSCFunction102 {
		return modbus.PrivateFunctionRequest{}, fmt.Errorf("tesla HSC native function is unsupported")
	}
	if len(message) > 251 {
		return modbus.PrivateFunctionRequest{}, fmt.Errorf("tesla HSC native message exceeds bound")
	}
	payload := append([]byte{byte(len(message))}, message...)
	return modbus.NewPrivateFunctionRequest(function, payload)
}

// DecodeTeslaHSCNativeRecord retains a normal FC101 or FC102 terminal exactly
// as received. Generic Modbus exception frames remain owned by transport.
func DecodeTeslaHSCNativeRecord(version string, function modbus.PrivateFunctionCode, payload []byte) (TeslaHSCNativeRecord, error) {
	if version != TeslaHSCCompatibilityV1 {
		return TeslaHSCNativeRecord{}, fmt.Errorf("tesla HSC native compatibility is unsupported")
	}
	if function != teslaHSCFunction101 && function != teslaHSCFunction102 {
		return TeslaHSCNativeRecord{}, fmt.Errorf("tesla HSC native function is unsupported")
	}
	response, err := DecodeTeslaHSCResponse(function, payload)
	if err != nil {
		return TeslaHSCNativeRecord{}, err
	}
	return TeslaHSCNativeRecord{function: function, payload: response.Payload()}, nil
}

func (record TeslaHSCNativeRecord) Function() modbus.PrivateFunctionCode { return record.function }
func (record TeslaHSCNativeRecord) Payload() []byte                      { return append([]byte(nil), record.payload...) }
