package modbusreg

import (
	"context"
	"errors"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestGrowattBMSRS485RTUObserverReadsExactSlicesInOrder(t *testing.T) {
	session := &growattBMSRTUSessionFake{wordsByOffset: growattBMSRTUFixtureWords(), failAt: -1, mismatchAt: -1}
	observer, err := NewGrowattBMSRS485RTUObserver(
		GrowattBMSRevisionTuple{Family: "1xSxxP ESS", FileRevision: "Rev2.01", HeaderVersion: "V2.0", CumulativeRevision: "2.02"},
		7,
		session,
	)
	if err != nil {
		t.Fatal(err)
	}

	status, err := observer.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.SOCPercent != 75 || status.OperatingState != GrowattBMSStateCharging || status.OutboundAllowed() {
		t.Fatalf("status=%#v", status)
	}
	if got, want := session.calls, [][2]uint16{{0x0001, 7}, {0x000d, 29}, {0x0100, 12}, {0x010d, 2}}; len(got) != len(want) {
		t.Fatalf("calls=%#v", got)
	} else {
		for index := range want {
			if got[index] != want[index] {
				t.Fatalf("calls=%#v", got)
			}
			if session.functions[index] != modbus.FunctionReadHoldingRegisters {
				t.Fatalf("functions=%#v", session.functions)
			}
		}
	}
	if session.unitID != 7 {
		t.Fatalf("unit_id=%d", session.unitID)
	}
}

func TestGrowattBMSRS485RTUObserverStopsWithoutPartialStatus(t *testing.T) {
	session := &growattBMSRTUSessionFake{wordsByOffset: growattBMSRTUFixtureWords(), failAt: 1, mismatchAt: -1}
	observer, err := NewGrowattBMSRS485RTUObserver(
		GrowattBMSRevisionTuple{Family: "1xSxxP ESS", FileRevision: "Rev2.01", HeaderVersion: "V2.0", CumulativeRevision: "2.02"},
		7,
		session,
	)
	if err != nil {
		t.Fatal(err)
	}

	status, err := observer.Observe(context.Background())
	if err == nil || status != (GrowattBMSTypedReadOnlyStatus{}) || len(session.calls) != 2 {
		t.Fatalf("status/error/calls=%#v/%v/%#v", status, err, session.calls)
	}
}

func TestGrowattBMSRS485RTUObserverRejectsMismatchedResponse(t *testing.T) {
	session := &growattBMSRTUSessionFake{wordsByOffset: growattBMSRTUFixtureWords(), failAt: -1, mismatchAt: 2}
	observer, err := NewGrowattBMSRS485RTUObserver(
		GrowattBMSRevisionTuple{Family: "1xSxxP ESS", FileRevision: "Rev2.01", HeaderVersion: "V2.0", CumulativeRevision: "2.02"},
		7,
		session,
	)
	if err != nil {
		t.Fatal(err)
	}

	status, err := observer.Observe(context.Background())
	if err == nil || status != (GrowattBMSTypedReadOnlyStatus{}) || len(session.calls) != 3 {
		t.Fatalf("status/error/calls=%#v/%v/%#v", status, err, session.calls)
	}
}

func TestGrowattBMSRS485RTUObserverRejectsInvalidCallerSelectionBeforeRead(t *testing.T) {
	session := &growattBMSRTUSessionFake{wordsByOffset: growattBMSRTUFixtureWords(), failAt: -1, mismatchAt: -1}
	if observer, err := NewGrowattBMSRS485RTUObserver(GrowattBMSRevisionTuple{}, 0, session); err == nil || observer != nil || len(session.calls) != 0 {
		t.Fatalf("observer/error/calls=%#v/%v/%#v", observer, err, session.calls)
	}
}

type growattBMSRTUSessionFake struct {
	wordsByOffset map[uint16][]uint16
	calls         [][2]uint16
	functions     []modbus.FunctionCode
	unitID        byte
	failAt        int
	mismatchAt    int
}

func (session *growattBMSRTUSessionFake) ReadHolding(
	_ context.Context,
	unitID byte,
	request modbus.ReadRegistersRequest,
) (modbus.ReadRegistersResponse, error) {
	session.unitID = unitID
	session.calls = append(session.calls, [2]uint16{request.Offset(), request.Quantity()})
	session.functions = append(session.functions, request.Function())
	index := len(session.calls) - 1
	if index == session.failAt {
		return modbus.ReadRegistersResponse{}, errors.New("correlated read failed")
	}
	words := session.wordsByOffset[request.Offset()]
	response, err := modbus.DecodeReadRegistersResponse(request, growattBMSRTUReadPDU(words))
	if err != nil {
		return modbus.ReadRegistersResponse{}, err
	}
	if index == session.mismatchAt {
		response.Provenance.Offset++
	}
	return response, nil
}

func growattBMSRTUReadPDU(words []uint16) []byte {
	pdu := make([]byte, 2+len(words)*2)
	pdu[0] = byte(modbus.FunctionReadHoldingRegisters)
	pdu[1] = byte(len(words) * 2)
	for index, word := range words {
		pdu[2+index*2], pdu[3+index*2] = byte(word>>8), byte(word)
	}
	return pdu
}

func growattBMSRTUFixtureWords() map[uint16][]uint16 {
	identity := make([]uint16, 7)
	identity[0], identity[1] = 0x0102, 0x0304
	status := make([]uint16, 29)
	status[0], status[1] = 0x0204, 0x0301
	status[6], status[8], status[9], status[10], status[11] = 2, 75, 5200, 0xff9c, 25
	status[13], status[14], status[17] = 3200, 5000, 110
	extension := make([]uint16, 12)
	extension[0], extension[1], extension[2], extension[4], extension[5], extension[6] = 100, 123, 3300, 512, 5, 6
	return map[uint16][]uint16{0x0001: identity, 0x000d: status, 0x0100: extension, 0x010d: {0, 0}}
}
