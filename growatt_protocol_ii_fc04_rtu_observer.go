package modbusreg

import (
	"context"
	"errors"
	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

var ErrGrowattProtocolIIFC04RTUObserverInvalid = errors.New("growatt Protocol II FC04 RTU observer is invalid")

type GrowattProtocolIIFC04RTUObserverSession interface {
	ReadInput(context.Context, byte, modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error)
}

// GrowattProtocolIIFC04RTUObserver executes one exact bounded FC04 read after
// its caller has qualified the FC03 identity. It owns neither discovery nor retries.
type GrowattProtocolIIFC04RTUObserver struct {
	identity GrowattProtocolIIIdentityObservation
	session  GrowattProtocolIIFC04RTUObserverSession
}

func NewGrowattProtocolIIFC04RTUObserver(identity GrowattProtocolIIIdentityObservation, session GrowattProtocolIIFC04RTUObserverSession) (*GrowattProtocolIIFC04RTUObserver, error) {
	if session == nil || identity.UnitID() == 0 || !validGrowattProtocolIIIdentityProfile(identity.Profile()) {
		return nil, ErrGrowattProtocolIIFC04RTUObserverInvalid
	}
	return &GrowattProtocolIIFC04RTUObserver{identity: identity, session: session}, nil
}
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
	return DecodeGrowattProtocolIIFC04Telemetry(o.identity, GrowattProtocolIIFC04Slice{Words: append([]uint16(nil), response.Words...)})
}
