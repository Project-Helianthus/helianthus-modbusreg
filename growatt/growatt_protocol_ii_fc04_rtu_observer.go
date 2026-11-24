package growatt

import (
	"context"
	"errors"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// ErrGrowattProtocolIIFC04RTUObserverInvalid reports a refused construction or
// call, including an identity without matching applicability, or a response
// whose provenance or word count differs from its FC04 read.
// Session errors are returned unchanged.
var ErrGrowattProtocolIIFC04RTUObserverInvalid = errors.New("growatt Protocol II FC04 RTU observer is invalid")

// ProtocolIIFC04RTUObserverSession is an already configured, generic
// correlated FC04 boundary; it owns framing, correlation, deadlines and device access.
type ProtocolIIFC04RTUObserverSession interface {
	ReadInput(context.Context, byte, modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error)
}

// ProtocolIIFC04RTUObserver executes the two bounded FC04 reads after its
// caller has qualified the endpoint. It owns neither discovery nor retries.
type ProtocolIIFC04RTUObserver struct {
	identity      ProtocolIIIdentityObservation
	applicability ProtocolIIFC04Applicability
	session       ProtocolIIFC04RTUObserverSession
	unit          byte
}

// NewProtocolIIFC04RTUObserver binds applicability to a session without I/O.
// A configured endpoint uses that endpoint's unit. An FC03 observation from
// another unit is not evidence and does not retarget the read. The synthetic
// fixture path still requires an exact identity match.
func NewProtocolIIFC04RTUObserver(identity ProtocolIIIdentityObservation, applicability ProtocolIIFC04Applicability, session ProtocolIIFC04RTUObserverSession) (*ProtocolIIFC04RTUObserver, error) {
	if session == nil {
		return nil, ErrGrowattProtocolIIFC04RTUObserverInvalid
	}
	if applicability.configured {
		if !applicability.admits(identity) {
			return nil, ErrGrowattProtocolIIFC04RTUObserverInvalid
		}
		if identity.UnitID() != 0 && identity.UnitID() != applicability.executionUnit {
			identity = ProtocolIIIdentityObservation{}
		}
		if identity.UnitID() == applicability.executionUnit {
			family := identity.Profile().Family
			if family != "" && family != applicability.profile.Family {
				return nil, ErrGrowattProtocolIIFC04RTUObserverInvalid
			}
		}
		return &ProtocolIIFC04RTUObserver{identity: identity, applicability: applicability, session: session, unit: applicability.executionUnit}, nil
	}
	if identity.UnitID() == 0 || !validGrowattProtocolIIIdentityProfile(identity.Profile()) || !applicability.matches(identity) {
		return nil, ErrGrowattProtocolIIFC04RTUObserverInvalid
	}
	return &ProtocolIIFC04RTUObserver{identity: identity, applicability: applicability, session: session, unit: identity.UnitID()}, nil
}

// Observe reads input words 0 through 249 as two FC04 reads and decodes them
// only when each response provenance and length match that read.
// The first read is offset 0, quantity modbus.MaxReadRegisters (125).
// The second read is offset 125, quantity modbus.MaxReadRegisters (125).
// MODBUS Application Protocol Specification V1.1b3 §6.4 limits one Read Input
// Registers quantity to 1..125.
func (o *ProtocolIIFC04RTUObserver) Observe(ctx context.Context) (ProtocolIIFC04Telemetry, error) {
	if o == nil || o.session == nil || ctx == nil {
		return ProtocolIIFC04Telemetry{}, ErrGrowattProtocolIIFC04RTUObserverInvalid
	}
	words := make([]uint16, 0, protocolIIInputWidth)
	for _, part := range []struct{ offset, quantity uint16 }{{0, modbus.MaxReadRegisters}, {modbus.MaxReadRegisters, modbus.MaxReadRegisters}} {
		request, err := modbus.NewReadRegistersRequest(modbus.FunctionReadInputRegisters, part.offset, part.quantity)
		if err != nil {
			return ProtocolIIFC04Telemetry{}, err
		}
		response, err := o.session.ReadInput(ctx, o.unit, request)
		if err != nil {
			return ProtocolIIFC04Telemetry{}, err
		}
		want := modbus.ReadProvenance{Function: modbus.FunctionReadInputRegisters, Table: modbus.InputRegisters, Offset: part.offset, Quantity: part.quantity}
		if response.Provenance != want || len(response.Words) != int(part.quantity) {
			return ProtocolIIFC04Telemetry{}, ErrGrowattProtocolIIFC04RTUObserverInvalid
		}
		words = append(words, response.Words...)
	}
	return DecodeGrowattProtocolIIFC04Telemetry(o.identity, o.applicability, ProtocolIIFC04Slice{Words: words})
}
