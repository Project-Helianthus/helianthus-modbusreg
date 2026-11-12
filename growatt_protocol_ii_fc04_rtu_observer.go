package modbusreg

import (
	"context"
	"errors"
	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// ErrGrowattProtocolIIFC04RTUObserverInvalid reports a refused construction or
// call, including an identity without matching applicability, or a response
// whose provenance or word count differs from the single 59-word FC04 read.
// Session errors are returned unchanged.
var ErrGrowattProtocolIIFC04RTUObserverInvalid = errors.New("growatt Protocol II FC04 RTU observer is invalid")

// GrowattProtocolIIFC04RTUObserverSession is an already configured, generic
// correlated FC04 boundary; it owns framing, correlation, deadlines and device access.
type GrowattProtocolIIFC04RTUObserverSession interface {
	ReadInput(context.Context, byte, modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error)
}

// GrowattProtocolIIFC04RTUObserver executes one exact bounded FC04 read after
// its caller has qualified the FC03 identity. It owns neither discovery nor retries.
type GrowattProtocolIIFC04RTUObserver struct {
	identity      GrowattProtocolIIIdentityObservation
	applicability GrowattProtocolIIFC04Applicability
	session       GrowattProtocolIIFC04RTUObserverSession
}

// NewGrowattProtocolIIFC04RTUObserver binds an identity and its applicability to
// a session without performing I/O. It fails closed unless the applicability
// admits the identity; no public admitting value exists today.
func NewGrowattProtocolIIFC04RTUObserver(identity GrowattProtocolIIIdentityObservation, applicability GrowattProtocolIIFC04Applicability, session GrowattProtocolIIFC04RTUObserverSession) (*GrowattProtocolIIFC04RTUObserver, error) {
	if session == nil || identity.UnitID() == 0 || !validGrowattProtocolIIIdentityProfile(identity.Profile()) || !applicability.matches(identity) {
		return nil, ErrGrowattProtocolIIFC04RTUObserverInvalid
	}
	return &GrowattProtocolIIFC04RTUObserver{identity: identity, applicability: applicability, session: session}, nil
}

// Observe performs one FC04 read of offsets 0 through 58 and decodes it only
// when the response provenance and length match exactly.
func (o *GrowattProtocolIIFC04RTUObserver) Observe(ctx context.Context) (GrowattProtocolIIFC04Telemetry, error) {
	if o == nil || o.session == nil || ctx == nil {
		return GrowattProtocolIIFC04Telemetry{}, ErrGrowattProtocolIIFC04RTUObserverInvalid
	}
	request, err := modbus.NewReadRegistersRequest(modbus.FunctionReadInputRegisters, 0, 59)
	if err != nil {
		return GrowattProtocolIIFC04Telemetry{}, err
	}
	response, err := o.session.ReadInput(ctx, o.identity.UnitID(), request)
	if err != nil {
		return GrowattProtocolIIFC04Telemetry{}, err
	}
	if response.Provenance != (modbus.ReadProvenance{Function: modbus.FunctionReadInputRegisters, Table: modbus.InputRegisters, Offset: 0, Quantity: 59}) || len(response.Words) != 59 {
		return GrowattProtocolIIFC04Telemetry{}, ErrGrowattProtocolIIFC04RTUObserverInvalid
	}
	return DecodeGrowattProtocolIIFC04Telemetry(o.identity, o.applicability, GrowattProtocolIIFC04Slice{Words: append([]uint16(nil), response.Words...)})
}
