package modbusreg

import (
	"context"
	"errors"
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

var ErrGrowattProtocolIIIdentityRTUObserverInvalid = errors.New("growatt Protocol II identity RTU observer is invalid")

// GrowattProtocolIIIdentityRTUObserverSession is an already-configured,
// generic correlated FC03 boundary. It owns serial configuration, framing,
// correlation, deadlines, and real-device access.
type GrowattProtocolIIIdentityRTUObserverSession interface {
	ReadHolding(context.Context, byte, modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error)
}

// GrowattProtocolIIIdentityRTUObserver composes one caller-selected native
// identity tuple from its five exact FC03 reads. It does not discover units,
// retry, broadcast, configure a transport, or execute a control operation.
type GrowattProtocolIIIdentityRTUObserver struct {
	profile GrowattProtocolIIIdentityProfile
	unitID  byte
	session GrowattProtocolIIIdentityRTUObserverSession
}

func NewGrowattProtocolIIIdentityRTUObserver(
	profile GrowattProtocolIIIdentityProfile,
	unitID byte,
	session GrowattProtocolIIIdentityRTUObserverSession,
) (*GrowattProtocolIIIdentityRTUObserver, error) {
	if session == nil || unitID == 0 || unitID > 247 || !validGrowattProtocolIIIdentityProfile(profile) {
		return nil, ErrGrowattProtocolIIIdentityRTUObserverInvalid
	}
	return &GrowattProtocolIIIdentityRTUObserver{profile: profile, unitID: unitID, session: session}, nil
}

func (observer *GrowattProtocolIIIdentityRTUObserver) Observe(ctx context.Context) (GrowattProtocolIIIdentityObservation, error) {
	if observer == nil || observer.session == nil || ctx == nil {
		return GrowattProtocolIIIdentityObservation{}, ErrGrowattProtocolIIIdentityRTUObserverInvalid
	}
	requests := [...]struct{ offset, quantity uint16 }{{9, 6}, {23, 5}, {43, 1}, {82, 2}, {88, 1}}
	slices := make([]GrowattProtocolIIIdentitySlice, 0, len(requests))
	for _, spec := range requests {
		request, err := modbus.NewReadRegistersRequest(modbus.FunctionReadHoldingRegisters, spec.offset, spec.quantity)
		if err != nil {
			return GrowattProtocolIIIdentityObservation{}, fmt.Errorf("growatt Protocol II identity request: %w", err)
		}
		response, err := observer.session.ReadHolding(ctx, observer.unitID, request)
		if err != nil {
			return GrowattProtocolIIIdentityObservation{}, err
		}
		if response.Provenance != (modbus.ReadProvenance{Function: modbus.FunctionReadHoldingRegisters, Table: modbus.HoldingRegisters, Offset: spec.offset, Quantity: spec.quantity}) || len(response.Words) != int(spec.quantity) {
			return GrowattProtocolIIIdentityObservation{}, ErrGrowattProtocolIIIdentityRTUObserverInvalid
		}
		slices = append(slices, GrowattProtocolIIIdentitySlice{Offset: spec.offset, Words: append([]uint16(nil), response.Words...)})
	}
	return DecodeGrowattProtocolIIIdentity(GrowattProtocolIIIdentityInput{UnitID: observer.unitID, Function: FunctionReadHoldingRegisters, Profile: observer.profile, Slices: slices})
}
