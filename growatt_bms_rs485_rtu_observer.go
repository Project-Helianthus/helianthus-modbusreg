package modbusreg

import (
	"context"
	"errors"
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

var ErrGrowattBMSRS485RTUObserverInvalid = errors.New("growatt BMS RTU observer is invalid")

// GrowattBMSRS485RTUObserverSession is an already configured, generic RTU
// read boundary. It owns framing, CRC, response correlation, deadlines, and
// serialization without carrying Growatt-specific semantics.
type GrowattBMSRS485RTUObserverSession interface {
	ReadHolding(context.Context, byte, modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error)
}

// GrowattBMSRS485RTUObserver performs the four fixed, read-only FC03 slices
// for one caller-selected BMS revision and unicast unit. It owns neither
// discovery, retries, session configuration, nor publication.
type GrowattBMSRS485RTUObserver struct {
	revision GrowattBMSRevisionTuple
	unitID   byte
	session  GrowattBMSRS485RTUObserverSession
}

// NewGrowattBMSRS485RTUObserver validates the exact caller-selected tuple and
// binds it to a generic correlated RTU read session without opening a device.
func NewGrowattBMSRS485RTUObserver(
	revision GrowattBMSRevisionTuple,
	unitID byte,
	session GrowattBMSRS485RTUObserverSession,
) (*GrowattBMSRS485RTUObserver, error) {
	if session == nil || unitID == 0 || unitID > 247 ||
		revision != (GrowattBMSRevisionTuple{
			Family: growattBMSFamily, FileRevision: growattBMSFileRevision,
			HeaderVersion: growattBMSHeaderVersion, CumulativeRevision: growattBMSCumulativeRevision,
		}) {
		return nil, ErrGrowattBMSRS485RTUObserverInvalid
	}
	return &GrowattBMSRS485RTUObserver{revision: revision, unitID: unitID, session: session}, nil
}

// Observe executes the exact contract slices in order and decodes them only
// after every generic exchange is correlated and complete.
func (observer *GrowattBMSRS485RTUObserver) Observe(ctx context.Context) (GrowattBMSTypedReadOnlyStatus, error) {
	if observer == nil || observer.session == nil || ctx == nil {
		return GrowattBMSTypedReadOnlyStatus{}, ErrGrowattBMSRS485RTUObserverInvalid
	}
	requests := [...]struct {
		offset   uint16
		quantity uint16
	}{
		{offset: 0x0001, quantity: 7},
		{offset: 0x000d, quantity: 29},
		{offset: 0x0100, quantity: 12},
		{offset: 0x010d, quantity: 2},
	}
	slices := make([]GrowattBMSReadOnlySlice, 0, len(requests))
	for _, spec := range requests {
		request, err := modbus.NewReadRegistersRequest(modbus.FunctionReadHoldingRegisters, spec.offset, spec.quantity)
		if err != nil {
			return GrowattBMSTypedReadOnlyStatus{}, fmt.Errorf("growatt BMS RTU request: %w", err)
		}
		response, err := observer.session.ReadHolding(ctx, observer.unitID, request)
		if err != nil {
			return GrowattBMSTypedReadOnlyStatus{}, err
		}
		if response.Provenance != (modbus.ReadProvenance{
			Function: modbus.FunctionReadHoldingRegisters,
			Table:    modbus.HoldingRegisters,
			Offset:   spec.offset,
			Quantity: spec.quantity,
		}) || len(response.Words) != int(spec.quantity) {
			return GrowattBMSTypedReadOnlyStatus{}, ErrGrowattBMSRS485RTUObserverInvalid
		}
		slices = append(slices, GrowattBMSReadOnlySlice{Offset: spec.offset, Words: append([]uint16(nil), response.Words...)})
	}
	return DecodeGrowattBMSTypedReadOnlyStatus(GrowattBMSReadOnlyInput{
		UnitID: observer.unitID, Function: FunctionReadHoldingRegisters, Revision: observer.revision, Slices: slices,
	})
}
