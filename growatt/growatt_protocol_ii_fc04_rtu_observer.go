package growatt

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

// ProtocolIIFC04RTUObserverSession is an already configured, generic
// correlated FC04 boundary; it owns framing, correlation, deadlines and device access.
type ProtocolIIFC04RTUObserverSession interface {
	ReadInput(context.Context, byte, modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error)
}

// ProtocolIIFC04RTUObserver executes one exact bounded FC04 read after
// its caller has qualified the FC03 identity. It owns neither discovery nor retries.
type ProtocolIIFC04RTUObserver struct {
	identity      ProtocolIIIdentityObservation
	applicability ProtocolIIFC04Applicability
	session       ProtocolIIFC04RTUObserverSession
}

// NewProtocolIIFC04RTUObserver binds an identity and its applicability to
// a session without performing I/O. It fails closed unless the applicability
// admits the identity; no public admitting value exists today.
func NewProtocolIIFC04RTUObserver(identity ProtocolIIIdentityObservation, applicability ProtocolIIFC04Applicability, session ProtocolIIFC04RTUObserverSession) (*ProtocolIIFC04RTUObserver, error) {
	if session == nil || identity.UnitID() == 0 || !validGrowattProtocolIIIdentityProfile(identity.Profile()) || !applicability.matches(identity) {
		return nil, ErrGrowattProtocolIIFC04RTUObserverInvalid
	}
	return &ProtocolIIFC04RTUObserver{identity: identity, applicability: applicability, session: session}, nil
}

// Observe performs one FC04 read of offsets 0 through 58 and decodes it only
// when the response provenance and length match exactly.
func (o *ProtocolIIFC04RTUObserver) Observe(ctx context.Context) (ProtocolIIFC04Telemetry, error) {
	if o == nil || o.session == nil || ctx == nil {
		return ProtocolIIFC04Telemetry{}, ErrGrowattProtocolIIFC04RTUObserverInvalid
	}
	request, err := modbus.NewReadRegistersRequest(modbus.FunctionReadInputRegisters, 0, 59)
	if err != nil {
		return ProtocolIIFC04Telemetry{}, err
	}
	response, err := o.session.ReadInput(ctx, o.identity.UnitID(), request)
	if err != nil {
		return ProtocolIIFC04Telemetry{}, err
	}
	if response.Provenance != (modbus.ReadProvenance{Function: modbus.FunctionReadInputRegisters, Table: modbus.InputRegisters, Offset: 0, Quantity: 59}) || len(response.Words) != 59 {
		return ProtocolIIFC04Telemetry{}, ErrGrowattProtocolIIFC04RTUObserverInvalid
	}
	return DecodeGrowattProtocolIIFC04Telemetry(o.identity, o.applicability, ProtocolIIFC04Slice{Words: append([]uint16(nil), response.Words...)})
}
