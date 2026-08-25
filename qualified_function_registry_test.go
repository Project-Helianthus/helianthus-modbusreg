package modbusreg

import (
	"context"
	"fmt"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestQualifiedFunctionRegistryScopesSameCodeToItsSelectedProfile(t *testing.T) {
	code, err := modbus.NewPrivateFunctionCode(0x64)
	if err != nil {
		t.Fatal(err)
	}
	first := &qualifiedFunctionTestCodec{code: code, request: []byte{0xa1}, response: []byte{0xa1}}
	second := &qualifiedFunctionTestCodec{code: code, request: []byte{0xb2}, response: []byte{0xb2}}
	registry, err := NewQualifiedFunctionRegistry([]QualifiedFunctionProfile{
		{Endpoint: "rtu-a", UnitID: 0x10, VendorProfile: "test-a", Codec: first},
		{Endpoint: "rtu-b", UnitID: 0x10, VendorProfile: "test-b", Codec: second},
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := &qualifiedFunctionTestTransport{}
	for _, selector := range []QualifiedFunctionSelector{
		{Endpoint: "rtu-a", UnitID: 0x10, VendorProfile: "test-a", Operation: "read"},
		{Endpoint: "rtu-b", UnitID: 0x10, VendorProfile: "test-b", Operation: "read"},
	} {
		if _, err := registry.Dispatch(context.Background(), transport, selector); err != nil {
			t.Fatal(err)
		}
	}
	if first.decodeCalls != 1 || second.decodeCalls != 1 {
		t.Fatalf("decode calls = %d, %d", first.decodeCalls, second.decodeCalls)
	}
	if got, want := transport.requests[0].Payload(), []byte{0xa1}; string(got) != string(want) {
		t.Fatalf("first payload = %x, want %x", got, want)
	}
	if got, want := transport.requests[1].Payload(), []byte{0xb2}; string(got) != string(want) {
		t.Fatalf("second payload = %x, want %x", got, want)
	}
}

func TestQualifiedFunctionRegistryDeniesAmbiguousOrUnqualifiedSelectionWithoutSend(t *testing.T) {
	code, err := modbus.NewPrivateFunctionCode(0x64)
	if err != nil {
		t.Fatal(err)
	}
	codec := &qualifiedFunctionTestCodec{code: code, request: []byte{1}, response: []byte{1}}
	transport := &qualifiedFunctionTestTransport{}
	ambiguous, err := NewQualifiedFunctionRegistry([]QualifiedFunctionProfile{
		{Endpoint: "rtu-a", UnitID: 0x10, VendorProfile: "test-a", Codec: codec},
		{Endpoint: "rtu-a", UnitID: 0x10, VendorProfile: "test-b", Codec: codec},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ambiguous.Dispatch(context.Background(), transport, QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: 0x10, VendorProfile: "test-a", Operation: "read",
	}); err == nil {
		t.Fatal("ambiguous selection sent")
	}
	if transport.calls != 0 {
		t.Fatalf("ambiguous sends = %d", transport.calls)
	}

	qualified, err := NewQualifiedFunctionRegistry([]QualifiedFunctionProfile{
		{Endpoint: "rtu-a", UnitID: 0x10, VendorProfile: "test-a", Codec: codec},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := qualified.Dispatch(context.Background(), transport, QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: 0x10, VendorProfile: "wrong", Operation: "read",
	}); err == nil {
		t.Fatal("mismatched profile sent")
	}
	if transport.calls != 0 {
		t.Fatalf("unqualified sends = %d", transport.calls)
	}
}

func TestTeslaQualifiedFunctionProfileKeepsUnknownOperationNoSend(t *testing.T) {
	profile, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewQualifiedFunctionRegistry([]QualifiedFunctionProfile{
		{Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: TeslaHSCProfileName, Codec: profile},
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := &qualifiedFunctionTestTransport{}
	if _, err := registry.Dispatch(context.Background(), transport, QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: TeslaHSCProfileName, Operation: "unknown",
	}); err == nil {
		t.Fatal("unknown Tesla operation sent")
	}
	if transport.calls != 0 {
		t.Fatalf("Tesla no-send calls = %d", transport.calls)
	}
}

type qualifiedFunctionTestCodec struct {
	code        modbus.PrivateFunctionCode
	request     []byte
	response    []byte
	decodeCalls int
}

func (codec *qualifiedFunctionTestCodec) EncodeQualifiedFunction(operation string) (modbus.PrivateFunctionRequest, modbus.PrivateFunctionResponsePolicy, error) {
	if operation != "read" {
		return modbus.PrivateFunctionRequest{}, modbus.PrivateFunctionResponsePolicy{}, fmt.Errorf("operation is not admitted")
	}
	request, err := modbus.NewPrivateFunctionRequest(codec.code, codec.request)
	if err != nil {
		return modbus.PrivateFunctionRequest{}, modbus.PrivateFunctionResponsePolicy{}, err
	}
	return request, modbus.DefaultPrivateFunctionResponsePolicy(), nil
}

func (codec *qualifiedFunctionTestCodec) DecodeQualifiedFunction(operation string, function modbus.PrivateFunctionCode, payload []byte) (QualifiedFunctionResult, error) {
	if operation != "read" || function != codec.code || string(payload) != string(codec.response) {
		return QualifiedFunctionResult{}, fmt.Errorf("replay belongs to another codec")
	}
	codec.decodeCalls++
	return QualifiedFunctionResult{Payload: append([]byte(nil), payload...)}, nil
}

type qualifiedFunctionTestTransport struct {
	calls            int
	requests         []modbus.PrivateFunctionRequest
	responsePayloads [][]byte
}

func (transport *qualifiedFunctionTestTransport) Exchange(_ context.Context, unitID byte, request modbus.PrivateFunctionRequest, _ modbus.PrivateFunctionResponsePolicy) ([]modbus.RTUPrivateFunctionResponseADU, error) {
	transport.calls++
	transport.requests = append(transport.requests, request)
	payloads := transport.responsePayloads
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
