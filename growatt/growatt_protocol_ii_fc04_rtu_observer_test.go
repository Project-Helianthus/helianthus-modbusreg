package growatt

import (
	"context"
	"errors"
	modbus "github.com/Project-Helianthus/helianthus-modbus"
	"testing"
)

func TestGrowattProtocolIIFC04RTUObserverUsesOneExactInputRead(t *testing.T) {
	identity, err := DecodeGrowattProtocolIIIdentity(validGrowattProtocolIIIdentityInput())
	if err != nil {
		t.Fatal(err)
	}
	admission := syntheticGrowattProtocolIIFC04FixtureAdmission(validGrowattProtocolIIIdentityInput().Profile)
	s := &growattProtocolIIFC04Fake{words: make([]uint16, protocolIIInputWidth)}
	s.words[0] = 1
	o, err := NewProtocolIIFC04RTUObserver(identity, admission, s)
	if err != nil {
		t.Fatal(err)
	}
	status, err := o.Observe(context.Background())
	if err != nil || status.InverterState != ProtocolIIStateNormal || len(s.calls) != 2 || len(status.RawSlice().Words) != protocolIIInputWidth {
		t.Fatalf("status/err/calls=%#v/%v/%#v", status, err, s.calls)
	}
	if s.calls[0].offset != 0 || s.calls[0].quantity != modbus.MaxReadRegisters || s.calls[1].offset != modbus.MaxReadRegisters || s.calls[1].quantity != modbus.MaxReadRegisters || s.calls[0].function != modbus.FunctionReadInputRegisters || s.calls[1].function != modbus.FunctionReadInputRegisters {
		t.Fatalf("calls=%#v", s.calls)
	}
}
func TestGrowattProtocolIIFC04RTUObserverRejectsShortOrFailedRead(t *testing.T) {
	identity, err := DecodeGrowattProtocolIIIdentity(validGrowattProtocolIIIdentityInput())
	if err != nil {
		t.Fatal(err)
	}
	admission := syntheticGrowattProtocolIIFC04FixtureAdmission(validGrowattProtocolIIIdentityInput().Profile)
	for _, s := range []*growattProtocolIIFC04Fake{{words: make([]uint16, 58)}, {err: errors.New("timeout")}} {
		o, _ := NewProtocolIIFC04RTUObserver(identity, admission, s)
		if status, err := o.Observe(context.Background()); err == nil || status.Identity().UnitID() != 0 {
			t.Fatalf("status/err=%#v/%v", status, err)
		}
	}
}

func TestGrowattProtocolIIFC04RTUObserverRejectsMissingOrMismatchedApplicability(t *testing.T) {
	identity, err := DecodeGrowattProtocolIIIdentity(validGrowattProtocolIIIdentityInput())
	if err != nil {
		t.Fatal(err)
	}
	mismatch := validGrowattProtocolIIIdentityInput().Profile
	mismatch.ModelBuild = [2]uint16{0x1111, 0x2222}
	admission := syntheticGrowattProtocolIIFC04FixtureAdmission(mismatch)
	for _, candidate := range []ProtocolIIFC04Applicability{{}, admission} {
		if observer, err := NewProtocolIIFC04RTUObserver(identity, candidate, &growattProtocolIIFC04Fake{}); err == nil || observer != nil {
			t.Fatalf("observer/err=%#v/%v", observer, err)
		}
	}
}

type protocolIIReadCall struct {
	unit             byte
	offset, quantity uint16
	function         modbus.FunctionCode
}

type growattProtocolIIFC04Fake struct {
	words []uint16
	err   error
	calls []protocolIIReadCall
}

func (s *growattProtocolIIFC04Fake) ReadInput(_ context.Context, unit byte, r modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error) {
	s.calls = append(s.calls, protocolIIReadCall{unit: unit, offset: r.Offset(), quantity: r.Quantity(), function: r.Function()})
	if s.err != nil {
		return modbus.ReadRegistersResponse{}, s.err
	}
	words := s.words
	start, end := int(r.Offset()), int(r.Offset())+int(r.Quantity())
	if start >= 0 && end <= len(s.words) {
		words = s.words[start:end]
	}
	p := make([]byte, 2+len(words)*2)
	p[0] = byte(modbus.FunctionReadInputRegisters)
	p[1] = byte(len(words) * 2)
	for i, w := range words {
		p[2+i*2], p[3+i*2] = byte(w>>8), byte(w)
	}
	return modbus.DecodeReadRegistersResponse(r, p)
}
