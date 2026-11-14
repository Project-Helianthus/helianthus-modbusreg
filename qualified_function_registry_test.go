package modbusreg_test

import (
	"context"
	"fmt"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/internal/qualifiedfunctiontest"
)

func TestQualifiedFunctionRegistryScopesSameCodeToItsSelectedProfile(t *testing.T) {
	code, err := modbus.NewPrivateFunctionCode(0x64)
	if err != nil {
		t.Fatal(err)
	}
	first := &qualifiedFunctionTestCodec{code: code, request: []byte{0xa1}, response: []byte{0xa1}}
	second := &qualifiedFunctionTestCodec{code: code, request: []byte{0xb2}, response: []byte{0xb2}}
	registry, err := modbusreg.NewQualifiedFunctionRegistry([]modbusreg.QualifiedFunctionProfile{
		{Endpoint: "rtu-a", UnitID: 0x10, VendorProfile: "test-a", Codec: first},
		{Endpoint: "rtu-b", UnitID: 0x10, VendorProfile: "test-b", Codec: second},
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := &qualifiedfunctiontest.Exchange{}
	for _, selector := range []modbusreg.QualifiedFunctionSelector{
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
	if got, want := transport.Requests[0].Payload(), []byte{0xa1}; string(got) != string(want) {
		t.Fatalf("first payload = %x, want %x", got, want)
	}
	if got, want := transport.Requests[1].Payload(), []byte{0xb2}; string(got) != string(want) {
		t.Fatalf("second payload = %x, want %x", got, want)
	}
}

func TestQualifiedFunctionRegistryDeniesAmbiguousOrUnqualifiedSelectionWithoutSend(t *testing.T) {
	code, err := modbus.NewPrivateFunctionCode(0x64)
	if err != nil {
		t.Fatal(err)
	}
	codec := &qualifiedFunctionTestCodec{code: code, request: []byte{1}, response: []byte{1}}
	transport := &qualifiedfunctiontest.Exchange{}
	ambiguous, err := modbusreg.NewQualifiedFunctionRegistry([]modbusreg.QualifiedFunctionProfile{
		{Endpoint: "rtu-a", UnitID: 0x10, VendorProfile: "test-a", Codec: codec},
		{Endpoint: "rtu-a", UnitID: 0x10, VendorProfile: "test-b", Codec: codec},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ambiguous.Dispatch(context.Background(), transport, modbusreg.QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: 0x10, VendorProfile: "test-a", Operation: "read",
	}); err == nil {
		t.Fatal("ambiguous selection sent")
	}
	if transport.Calls != 0 {
		t.Fatalf("ambiguous sends = %d", transport.Calls)
	}

	qualified, err := modbusreg.NewQualifiedFunctionRegistry([]modbusreg.QualifiedFunctionProfile{
		{Endpoint: "rtu-a", UnitID: 0x10, VendorProfile: "test-a", Codec: codec},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := qualified.Dispatch(context.Background(), transport, modbusreg.QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: 0x10, VendorProfile: "wrong", Operation: "read",
	}); err == nil {
		t.Fatal("mismatched profile sent")
	}
	if transport.Calls != 0 {
		t.Fatalf("unqualified sends = %d", transport.Calls)
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

func (codec *qualifiedFunctionTestCodec) DecodeQualifiedFunction(operation string, function modbus.PrivateFunctionCode, payload []byte) (modbusreg.QualifiedFunctionResult, error) {
	if operation != "read" || function != codec.code || string(payload) != string(codec.response) {
		return modbusreg.QualifiedFunctionResult{}, fmt.Errorf("replay belongs to another codec")
	}
	codec.decodeCalls++
	return modbusreg.QualifiedFunctionResult{Payload: append([]byte(nil), payload...)}, nil
}
