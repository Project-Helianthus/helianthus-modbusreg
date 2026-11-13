package growatt

import (
	"context"
	"errors"
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// ErrGrowattBMSRS485RTUObserverInvalid reports a refused observer construction
// or call (nil session or context, unit outside 1-247, unsupported revision) or
// a response whose provenance or word count differs from the requested slice.
// Session errors are returned unchanged.
var ErrGrowattBMSRS485RTUObserverInvalid = errors.New("growatt BMS RTU observer is invalid")

// BMSRS485RTUObserverSession is an already configured, generic RTU
// read boundary. It owns framing, CRC, response correlation, deadlines, and
// serialization without carrying Growatt-specific semantics.
type BMSRS485RTUObserverSession interface {
	ReadHolding(context.Context, byte, modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error)
}

// BMSRS485RTUObserver performs the four fixed, read-only FC03 slices
// for one caller-selected BMS revision and unicast unit. It owns neither
// discovery, retries, session configuration, nor publication.
type BMSRS485RTUObserver struct {
	revision BMSRevisionTuple
	unitID   byte
	session  BMSRS485RTUObserverSession
}

// NewBMSRS485RTUObserver validates the exact caller-selected tuple and
// binds it to a generic correlated RTU read session without opening a device.
func NewBMSRS485RTUObserver(
	revision BMSRevisionTuple,
	unitID byte,
	session BMSRS485RTUObserverSession,
) (*BMSRS485RTUObserver, error) {
	if session == nil || unitID == 0 || unitID > 247 ||
		revision != (BMSRevisionTuple{
			Family: growattBMSFamily, FileRevision: growattBMSFileRevision,
			HeaderVersion: growattBMSHeaderVersion, CumulativeRevision: growattBMSCumulativeRevision,
		}) {
		return nil, ErrGrowattBMSRS485RTUObserverInvalid
	}
	return &BMSRS485RTUObserver{revision: revision, unitID: unitID, session: session}, nil
}

// Observe executes the exact contract slices in order and decodes them only
// after every generic exchange is correlated and complete.
func (observer *BMSRS485RTUObserver) Observe(ctx context.Context) (BMSTypedReadOnlyStatus, error) {
	if observer == nil || observer.session == nil || ctx == nil {
		return BMSTypedReadOnlyStatus{}, ErrGrowattBMSRS485RTUObserverInvalid
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
	slices := make([]BMSReadOnlySlice, 0, len(requests))
	for _, spec := range requests {
		request, err := modbus.NewReadRegistersRequest(modbus.FunctionReadHoldingRegisters, spec.offset, spec.quantity)
		if err != nil {
			return BMSTypedReadOnlyStatus{}, fmt.Errorf("growatt BMS RTU request: %w", err)
		}
		response, err := observer.session.ReadHolding(ctx, observer.unitID, request)
		if err != nil {
			return BMSTypedReadOnlyStatus{}, err
		}
		if response.Provenance != (modbus.ReadProvenance{
			Function: modbus.FunctionReadHoldingRegisters,
			Table:    modbus.HoldingRegisters,
			Offset:   spec.offset,
			Quantity: spec.quantity,
		}) || len(response.Words) != int(spec.quantity) {
			return BMSTypedReadOnlyStatus{}, ErrGrowattBMSRS485RTUObserverInvalid
		}
		slices = append(slices, BMSReadOnlySlice{Offset: spec.offset, Words: append([]uint16(nil), response.Words...)})
	}
	return DecodeGrowattBMSTypedReadOnlyStatus(BMSReadOnlyInput{
		UnitID: observer.unitID, Function: modbusreg.FunctionReadHoldingRegisters, Revision: observer.revision, Slices: slices,
	})
}
