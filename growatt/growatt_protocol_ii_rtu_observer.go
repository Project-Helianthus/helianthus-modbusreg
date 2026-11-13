package growatt

import (
	"context"
	"errors"
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// ErrGrowattProtocolIIIdentityRTUObserverInvalid reports a refused construction
// or call (nil session or context, unit outside 1-247, invalid profile) or a
// response whose provenance or word count differs from the requested slice.
// Session errors are returned unchanged.
var ErrGrowattProtocolIIIdentityRTUObserverInvalid = errors.New("growatt Protocol II identity RTU observer is invalid")

// ProtocolIIIdentityRTUObserverSession is an already-configured,
// generic correlated FC03 boundary. It owns serial configuration, framing,
// correlation, deadlines, and real-device access.
type ProtocolIIIdentityRTUObserverSession interface {
	ReadHolding(context.Context, byte, modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error)
}

// ProtocolIIIdentityRTUObserver composes one caller-selected native
// identity tuple from its five exact FC03 reads. It does not discover units,
// retry, broadcast, configure a transport, or execute a control operation.
type ProtocolIIIdentityRTUObserver struct {
	profile ProtocolIIIdentityProfile
	unitID  byte
	session ProtocolIIIdentityRTUObserverSession
}

// NewProtocolIIIdentityRTUObserver binds a caller-selected identity
// profile and unicast unit to a session without performing I/O.
func NewProtocolIIIdentityRTUObserver(
	profile ProtocolIIIdentityProfile,
	unitID byte,
	session ProtocolIIIdentityRTUObserverSession,
) (*ProtocolIIIdentityRTUObserver, error) {
	if session == nil || unitID == 0 || unitID > 247 || !validGrowattProtocolIIIdentityProfile(profile) {
		return nil, ErrGrowattProtocolIIIdentityRTUObserverInvalid
	}
	return &ProtocolIIIdentityRTUObserver{profile: profile, unitID: unitID, session: session}, nil
}

// Observe performs the five fixed FC03 identity reads in order, stops at the
// first failure, and decodes only after every response matches its request.
func (observer *ProtocolIIIdentityRTUObserver) Observe(ctx context.Context) (ProtocolIIIdentityObservation, error) {
	if observer == nil || observer.session == nil || ctx == nil {
		return ProtocolIIIdentityObservation{}, ErrGrowattProtocolIIIdentityRTUObserverInvalid
	}
	requests := [...]struct{ offset, quantity uint16 }{{9, 6}, {23, 5}, {43, 1}, {82, 2}, {88, 1}}
	slices := make([]ProtocolIIIdentitySlice, 0, len(requests))
	for _, spec := range requests {
		request, err := modbus.NewReadRegistersRequest(modbus.FunctionReadHoldingRegisters, spec.offset, spec.quantity)
		if err != nil {
			return ProtocolIIIdentityObservation{}, fmt.Errorf("growatt Protocol II identity request: %w", err)
		}
		response, err := observer.session.ReadHolding(ctx, observer.unitID, request)
		if err != nil {
			return ProtocolIIIdentityObservation{}, err
		}
		if response.Provenance != (modbus.ReadProvenance{Function: modbus.FunctionReadHoldingRegisters, Table: modbus.HoldingRegisters, Offset: spec.offset, Quantity: spec.quantity}) || len(response.Words) != int(spec.quantity) {
			return ProtocolIIIdentityObservation{}, ErrGrowattProtocolIIIdentityRTUObserverInvalid
		}
		slices = append(slices, ProtocolIIIdentitySlice{Offset: spec.offset, Words: append([]uint16(nil), response.Words...)})
	}
	return DecodeGrowattProtocolIIIdentity(ProtocolIIIdentityInput{UnitID: observer.unitID, Function: modbusreg.FunctionReadHoldingRegisters, Profile: observer.profile, Slices: slices})
}
