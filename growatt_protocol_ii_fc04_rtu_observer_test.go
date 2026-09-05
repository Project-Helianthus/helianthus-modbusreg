package modbusreg

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
	admission, err := NewGrowattProtocolIIFC04Applicability(validGrowattProtocolIIIdentityInput().Profile, "test-exact-mapping")
	if err != nil {
		t.Fatal(err)
	}
	s := &growattProtocolIIFC04Fake{words: make([]uint16, 59)}
	s.words[0] = 1
	o, err := NewGrowattProtocolIIFC04RTUObserver(identity, admission, s)
	if err != nil {
		t.Fatal(err)
	}
	status, err := o.Observe(context.Background())
	if err != nil || status.InverterState != GrowattProtocolIIStateNormal || s.offset != 0 || s.quantity != 59 || s.function != modbus.FunctionReadInputRegisters {
		t.Fatalf("status/err/session=%#v/%v/%#v", status, err, s)
	}
}
func TestGrowattProtocolIIFC04RTUObserverRejectsShortOrFailedRead(t *testing.T) {
	identity, err := DecodeGrowattProtocolIIIdentity(validGrowattProtocolIIIdentityInput())
	if err != nil {
		t.Fatal(err)
	}
	admission, err := NewGrowattProtocolIIFC04Applicability(validGrowattProtocolIIIdentityInput().Profile, "test-exact-mapping")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []*growattProtocolIIFC04Fake{{words: make([]uint16, 58)}, {err: errors.New("timeout")}} {
		o, _ := NewGrowattProtocolIIFC04RTUObserver(identity, admission, s)
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
	admission, err := NewGrowattProtocolIIFC04Applicability(mismatch, "test-exact-mapping")
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []GrowattProtocolIIFC04Applicability{{}, admission} {
		if observer, err := NewGrowattProtocolIIFC04RTUObserver(identity, candidate, &growattProtocolIIFC04Fake{}); err == nil || observer != nil {
			t.Fatalf("observer/err=%#v/%v", observer, err)
		}
	}
}

type growattProtocolIIFC04Fake struct {
	words            []uint16
	err              error
	offset, quantity uint16
	function         modbus.FunctionCode
}

func (s *growattProtocolIIFC04Fake) ReadInput(_ context.Context, _ byte, r modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error) {
	s.offset, s.quantity, s.function = r.Offset(), r.Quantity(), r.Function()
	if s.err != nil {
		return modbus.ReadRegistersResponse{}, s.err
	}
	p := make([]byte, 2+len(s.words)*2)
	p[0] = byte(modbus.FunctionReadInputRegisters)
	p[1] = byte(len(s.words) * 2)
	for i, w := range s.words {
		p[2+i*2], p[3+i*2] = byte(w>>8), byte(w)
	}
	return modbus.DecodeReadRegistersResponse(r, p)
}
