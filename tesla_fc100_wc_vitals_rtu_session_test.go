package modbusreg

import (
	"bytes"
	"context"
	"errors"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestTeslaFC100WCVitalsRTUSessionAdapterUsesCorrelatedSession(t *testing.T) {
	session := &teslaFC100WCVitalsSessionFake{responsePayloads: [][]byte{
		{0x04, 0x32, 0x02, 0x0a, 0x00},
		{0x06, 0x32, 0x04, 0x12, 0x02, 0x08, 0x01},
	}}
	adapter, err := NewTeslaFC100WCVitalsRTUSessionAdapter(testTeslaFC100WCVitalsQualifiedProfile(t), session)
	if err != nil {
		t.Fatal(err)
	}

	replays, err := adapter.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if session.calls != 1 || session.unitID != 0x10 {
		t.Fatalf("session calls/unit = %d/%#x", session.calls, session.unitID)
	}
	if got, want := session.request.FunctionCode(), teslaHSCFunction100; got != want {
		t.Fatalf("request function = %d, want %d", got, want)
	}
	if got, want := session.request.Payload(), teslaFC100WCVitalsRequestPDU; !bytes.Equal(got, want) {
		t.Fatalf("request payload = %x, want %x", got, want)
	}
	if len(replays) != 2 || replays[0].Kind != TeslaFC100WCVitalsIntermediate ||
		replays[1].Kind != TeslaFC100WCVitalsTerminal || replays[1].SnapshotDigest == "" {
		t.Fatalf("redacted replays = %#v", replays)
	}
}

func TestTeslaFC100WCVitalsRTUSessionAdapterPropagatesCorrelatedException(t *testing.T) {
	correlatedException := errors.New("correlated exception")
	session := &teslaFC100WCVitalsSessionFake{err: correlatedException}
	adapter, err := NewTeslaFC100WCVitalsRTUSessionAdapter(testTeslaFC100WCVitalsQualifiedProfile(t), session)
	if err != nil {
		t.Fatal(err)
	}
	if replays, err := adapter.Execute(context.Background()); !errors.Is(err, correlatedException) || replays != nil {
		t.Fatalf("exception replays/error = %#v/%v", replays, err)
	}
	if session.calls != 1 {
		t.Fatalf("session calls = %d", session.calls)
	}

	if _, err := NewTeslaFC100WCVitalsRTUSessionAdapter(testTeslaFC100WCVitalsQualifiedProfile(t), nil); !errors.Is(err, ErrQualifiedFunctionNoSend) {
		t.Fatalf("nil session error = %v", err)
	}
}

type teslaFC100WCVitalsSessionFake struct {
	calls            int
	unitID           byte
	request          modbus.PrivateFunctionRequest
	responsePayloads [][]byte
	err              error
}

func (session *teslaFC100WCVitalsSessionFake) Exchange(
	_ context.Context,
	unitID byte,
	request modbus.PrivateFunctionRequest,
	_ modbus.PrivateFunctionResponsePolicy,
) ([]modbus.RTUPrivateFunctionResponseADU, error) {
	session.calls++
	session.unitID = unitID
	session.request = request
	if session.err != nil {
		return nil, session.err
	}
	responses := make([]modbus.RTUPrivateFunctionResponseADU, 0, len(session.responsePayloads))
	for _, payload := range session.responsePayloads {
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
