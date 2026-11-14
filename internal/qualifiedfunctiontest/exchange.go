package qualifiedfunctiontest

import (
	"context"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// Exchange is an in-memory qualified-function exchanger. It records every
// request it receives and answers with one RTU response ADU per configured
// payload, echoing the request payload when ResponsePayloads is nil.
type Exchange struct {
	// Calls counts the exchanges performed.
	Calls int
	// Requests holds the requests in the order they were exchanged.
	Requests []modbus.PrivateFunctionRequest
	// ResponsePayloads are the payloads of the response ADUs returned for
	// every request; nil echoes the request payload once.
	ResponsePayloads [][]byte
}

// Exchange records request and returns the configured response ADUs for
// unitID, encoded and decoded through the RTU private-function codec.
func (exchange *Exchange) Exchange(_ context.Context, unitID byte, request modbus.PrivateFunctionRequest, _ modbus.PrivateFunctionResponsePolicy) ([]modbus.RTUPrivateFunctionResponseADU, error) {
	exchange.Calls++
	exchange.Requests = append(exchange.Requests, request)
	payloads := exchange.ResponsePayloads
	if payloads == nil {
		payloads = [][]byte{request.Payload()}
	}
	responses := make([]modbus.RTUPrivateFunctionResponseADU, 0, len(payloads))
	for _, payload := range payloads {
		responseRequest, err := modbus.NewPrivateFunctionRequest(request.FunctionCode(), payload)
		if err != nil {
			return nil, err
		}
		frame, err := modbus.EncodeRTUPrivateFunctionADU(unitID, responseRequest)
		if err != nil {
			return nil, err
		}
		response, err := modbus.DecodeRTUPrivateFunctionResponseADU(unitID, request, frame)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}
