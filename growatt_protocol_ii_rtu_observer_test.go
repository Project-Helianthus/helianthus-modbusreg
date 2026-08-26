package modbusreg

import (
	"context"
	"errors"
	"reflect"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestGrowattProtocolIIIdentityRTUObserverRetainsCompleteNativeTuple(t *testing.T) {
	session := &growattProtocolIIIdentityRTUSessionFake{words: growattProtocolIIIdentityRTUWords(), failAt: -1}
	profile := GrowattProtocolIIIdentityProfile{
		Schema:          growattProtocolIIIdentitySchema,
		Family:          "MAX",
		DeviceType:      0x1234,
		ModelBuild:      [2]uint16{0x4d41, 0x5831},
		ProtocolVersion: 0x0124,
	}
	observer, err := NewGrowattProtocolIIIdentityRTUObserver(profile, 7, session)
	if err != nil {
		t.Fatal(err)
	}

	observation, err := observer.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observation.UnitID() != 7 || observation.FirmwareText() != "FW-1" || observation.SerialText() != "SN-0001" ||
		observation.DeviceType() != profile.DeviceType || observation.ModelBuild() != profile.ModelBuild ||
		observation.ProtocolVersion() != profile.ProtocolVersion {
		t.Fatalf("observation=%#v", observation)
	}
	wantCalls := [][2]uint16{{9, 6}, {23, 5}, {43, 1}, {82, 2}, {88, 1}}
	if !reflect.DeepEqual(session.calls, wantCalls) {
		t.Fatalf("calls=%#v want=%#v", session.calls, wantCalls)
	}
	if len(observation.Slices()) != len(wantCalls) || observation.Slices()[1].Offset != 23 ||
		!reflect.DeepEqual(observation.Slices()[1].Words, session.words[23]) {
		t.Fatalf("slices=%#v", observation.Slices())
	}
}

func TestGrowattProtocolIIIdentityRTUObserverStopsOnFirstFailure(t *testing.T) {
	session := &growattProtocolIIIdentityRTUSessionFake{words: growattProtocolIIIdentityRTUWords(), failAt: 2}
	profile := GrowattProtocolIIIdentityProfile{
		Schema: growattProtocolIIIdentitySchema, Family: "MAX", DeviceType: 0x1234,
		ModelBuild: [2]uint16{0x4d41, 0x5831}, ProtocolVersion: 0x0124,
	}
	observer, err := NewGrowattProtocolIIIdentityRTUObserver(profile, 7, session)
	if err != nil {
		t.Fatal(err)
	}
	if observation, err := observer.Observe(context.Background()); err == nil || observation != (GrowattProtocolIIIdentityObservation{}) || len(session.calls) != 3 {
		t.Fatalf("observation/err/calls=%#v/%v/%#v", observation, err, session.calls)
	}
}

type growattProtocolIIIdentityRTUSessionFake struct {
	words  map[uint16][]uint16
	calls  [][2]uint16
	failAt int
}

func (session *growattProtocolIIIdentityRTUSessionFake) ReadHolding(
	_ context.Context,
	unitID byte,
	request modbus.ReadRegistersRequest,
) (modbus.ReadRegistersResponse, error) {
	if unitID != 7 || request.Function() != modbus.FunctionReadHoldingRegisters {
		return modbus.ReadRegistersResponse{}, errors.New("unexpected native request")
	}
	session.calls = append(session.calls, [2]uint16{request.Offset(), request.Quantity()})
	if len(session.calls)-1 == session.failAt {
		return modbus.ReadRegistersResponse{}, errors.New("correlated read failed")
	}
	words := session.words[request.Offset()]
	pdu := make([]byte, 2+len(words)*2)
	pdu[0], pdu[1] = byte(modbus.FunctionReadHoldingRegisters), byte(len(words)*2)
	for index, word := range words {
		pdu[2+index*2], pdu[3+index*2] = byte(word>>8), byte(word)
	}
	return modbus.DecodeReadRegistersResponse(request, pdu)
}

func growattProtocolIIIdentityRTUWords() map[uint16][]uint16 {
	return map[uint16][]uint16{
		9:  {0x4657, 0x2d31, 0, 0, 0, 0},
		23: {0x534e, 0x2d30, 0x3030, 0x3100, 0},
		43: {0x1234},
		82: {0x4d41, 0x5831},
		88: {0x0124},
	}
}
