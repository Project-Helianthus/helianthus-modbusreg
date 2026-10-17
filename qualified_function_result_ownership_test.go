package modbusreg

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestU29OrdinaryDecodeResultOwnership(t *testing.T) {
	u29CheckResultOwnership(t, false)
}

func TestU29SequenceDecodeResultOwnership(t *testing.T) {
	u29CheckResultOwnership(t, true)
}

func u29CheckResultOwnership(t *testing.T, sequence bool) {
	t.Helper()
	for _, field := range []string{"Payload", "Replay"} {
		t.Run(field, func(t *testing.T) {
			code, err := modbus.NewPrivateFunctionCode(0x64)
			if err != nil {
				t.Fatal(err)
			}
			codec := &u29RetainingCodec{code: code, retained: u29ExpectedResults()}
			var selected QualifiedFunctionCodec = codec
			var sequenceCodec *u29SequenceCodec
			if sequence {
				sequenceCodec = &u29SequenceCodec{u29RetainingCodec: codec}
				selected = sequenceCodec
			}
			registry, err := NewQualifiedFunctionRegistry([]QualifiedFunctionProfile{{
				Endpoint: "synthetic", UnitID: 1, VendorProfile: "synthetic-family", Codec: selected,
			}})
			if err != nil {
				t.Fatal(err)
			}
			exchanger := &u29MemoryExchanger{}
			dispatch := func() []QualifiedFunctionResult {
				t.Helper()
				results, err := registry.Dispatch(context.Background(), exchanger, QualifiedFunctionSelector{
					Endpoint: "synthetic", UnitID: 1, VendorProfile: "synthetic-family", Operation: "read",
				})
				if err != nil {
					t.Fatal(err)
				}
				return results
			}
			assertResults := func(label string, got, want []QualifiedFunctionResult) {
				t.Helper()
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("%s: got %+v, want %+v", label, got, want)
				}
			}

			first := dispatch()
			assertResults("first success control", first, u29ExpectedResults())
			u29MutateResult(first, field, false)
			assertResults("caller mutation must not reach codec", codec.retained, u29ExpectedResults())
			second := dispatch()
			assertResults("reused codec storage success control", second, u29ExpectedResults())
			callerExpected := u29ExpectedResults()
			u29MutateResult(callerExpected, field, false)
			assertResults("second dispatch must not change prior caller storage", first, callerExpected)

			u29MutateResult(codec.retained, field, true)
			assertResults("codec mutation must not reach prior result", first, callerExpected)
			assertResults("codec mutation must not reach latest result", second, u29ExpectedResults())
			third := dispatch()
			codecExpected := u29ExpectedResults()
			u29MutateResult(codecExpected, field, true)
			assertResults("third success control observes retained codec mutation", third, codecExpected)
			assertResults("later dispatch must preserve prior caller storage", first, callerExpected)
			assertResults("later dispatch must preserve detached second result", second, u29ExpectedResults())
			if exchanger.calls != 3 || codec.encodeCalls != 3 {
				t.Fatalf("exchange/encode calls = %d/%d, want 3/3", exchanger.calls, codec.encodeCalls)
			}
			if sequence {
				if sequenceCodec.handlesCalls != 3 || sequenceCodec.decodeCalls != 3 || codec.decodeCalls != 0 {
					t.Fatalf("sequence branch calls: handles=%d sequence=%d ordinary=%d", sequenceCodec.handlesCalls, sequenceCodec.decodeCalls, codec.decodeCalls)
				}
			} else if codec.decodeCalls != 6 {
				t.Fatalf("ordinary decode calls = %d, want 6", codec.decodeCalls)
			}
		})
	}
}

func u29ExpectedResults() []QualifiedFunctionResult {
	return []QualifiedFunctionResult{
		{Payload: []byte{0x00, 0x81, 0xff}, Replay: &QualifiedFunctionReplay{Kind: "synthetic", PayloadLength: 3, PayloadDigest: "synthetic-marker"}},
		{Payload: []byte{0x42, 0x00}},
	}
}

func u29MutateResult(results []QualifiedFunctionResult, field string, retained bool) {
	if field == "Payload" {
		if retained {
			results[0].Payload[1] = 0x33
			results[1].Payload[0] = 0x44
		} else {
			results[0].Payload[0] = 0x55
			results[1].Payload[1] = 0x66
		}
		return
	}
	if retained {
		*results[0].Replay = QualifiedFunctionReplay{Kind: "codec-mutated", PayloadLength: 7, PayloadDigest: "codec-marker"}
	} else {
		*results[0].Replay = QualifiedFunctionReplay{Kind: "caller-mutated", PayloadLength: 9, PayloadDigest: "caller-marker"}
	}
}

type u29RetainingCodec struct {
	code        modbus.PrivateFunctionCode
	retained    []QualifiedFunctionResult
	encodeCalls int
	decodeCalls int
}

func (codec *u29RetainingCodec) EncodeQualifiedFunction(operation string) (modbus.PrivateFunctionRequest, modbus.PrivateFunctionResponsePolicy, error) {
	if operation != "read" {
		return modbus.PrivateFunctionRequest{}, modbus.PrivateFunctionResponsePolicy{}, fmt.Errorf("unexpected synthetic operation")
	}
	codec.encodeCalls++
	request, err := modbus.NewPrivateFunctionRequest(codec.code, []byte{0x01})
	return request, modbus.DefaultPrivateFunctionResponsePolicy(), err
}

func (codec *u29RetainingCodec) DecodeQualifiedFunction(operation string, code modbus.PrivateFunctionCode, payload []byte) (QualifiedFunctionResult, error) {
	if operation != "read" || code != codec.code || len(payload) != 1 || payload[0] > 1 {
		return QualifiedFunctionResult{}, fmt.Errorf("unexpected synthetic response")
	}
	codec.decodeCalls++
	return codec.retained[int(payload[0])], nil
}

type u29SequenceCodec struct {
	*u29RetainingCodec
	handlesCalls int
	decodeCalls  int
}

func (codec *u29SequenceCodec) HandlesQualifiedFunctionSequence(operation string) bool {
	codec.handlesCalls++
	return operation == "read"
}

func (codec *u29SequenceCodec) DecodeQualifiedFunctionSequence(operation string, code modbus.PrivateFunctionCode, payloads [][]byte) ([]QualifiedFunctionResult, error) {
	if operation != "read" || code != codec.code || !reflect.DeepEqual(payloads, [][]byte{{0}, {1}}) {
		return nil, fmt.Errorf("unexpected synthetic sequence")
	}
	codec.decodeCalls++
	return codec.retained, nil
}

type u29MemoryExchanger struct{ calls int }

func (exchanger *u29MemoryExchanger) Exchange(_ context.Context, unit byte, request modbus.PrivateFunctionRequest, _ modbus.PrivateFunctionResponsePolicy) ([]modbus.RTUPrivateFunctionResponseADU, error) {
	exchanger.calls++
	if unit != 1 || !reflect.DeepEqual(request.Payload(), []byte{0x01}) {
		return nil, fmt.Errorf("unexpected synthetic request")
	}
	responses := make([]modbus.RTUPrivateFunctionResponseADU, 0, 2)
	for _, index := range []byte{0, 1} {
		responseRequest, err := modbus.NewPrivateFunctionRequest(request.FunctionCode(), []byte{index})
		if err != nil {
			return nil, err
		}
		frame, err := modbus.EncodeRTUPrivateFunctionADU(unit, responseRequest)
		if err != nil {
			return nil, err
		}
		response, err := modbus.DecodeRTUPrivateFunctionResponseADU(unit, request, frame)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}
