package modbusreg

import (
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// TeslaHSCSyntacticRequest is one bounded HSC request PDU. It is only a
// syntax value: it carries no operation admission or transport authority.
type TeslaHSCSyntacticRequest struct {
	function modbus.PrivateFunctionCode
	payload  []byte
}

// NewTeslaHSCSyntacticRequest constructs the exact-length L|nested request
// PDU used by FC100, FC101, and FC102. It does not select an operation,
// construct a generic transport request, or permit wire transmission.
func NewTeslaHSCSyntacticRequest(
	function modbus.PrivateFunctionCode,
	nested []byte,
) (TeslaHSCSyntacticRequest, error) {
	if !isTeslaHSCFunction(function) {
		return TeslaHSCSyntacticRequest{}, fmt.Errorf("tesla HSC function is unsupported")
	}
	if len(nested) >= maxTeslaHSCPayload {
		return TeslaHSCSyntacticRequest{}, fmt.Errorf("tesla HSC nested payload exceeds bound")
	}
	payload := make([]byte, len(nested)+1)
	payload[0] = byte(len(nested))
	copy(payload[1:], nested)
	if _, err := DecodeTeslaHSCRequestEnvelope(function, payload); err != nil {
		return TeslaHSCSyntacticRequest{}, err
	}
	return TeslaHSCSyntacticRequest{function: function, payload: payload}, nil
}

// Function returns the vendor function selected for this syntactic PDU.
func (request TeslaHSCSyntacticRequest) Function() modbus.PrivateFunctionCode {
	return request.function
}

// Payload returns an independent copy of the exact-length L|nested PDU.
func (request TeslaHSCSyntacticRequest) Payload() []byte {
	return append([]byte(nil), request.payload...)
}
