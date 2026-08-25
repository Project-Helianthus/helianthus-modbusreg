package modbusreg

import "testing"

func TestTeslaFC100TEDAPIParsesBoundedFirstVarintAndRedactsReplay(t *testing.T) {
	decoded, err := DecodeTeslaFC100TEDAPI([]byte{3, 0x08, 0x96, 0x01})
	if err != nil {
		t.Fatal(err)
	}
	if decoded.MessageLength() != 3 || decoded.FirstFieldNumber() != 1 || decoded.FirstWireType() != 0 {
		t.Fatalf("decoded = %#v", decoded)
	}
	if decoded.PayloadDigest() == "" || decoded.Payload() != nil {
		t.Fatalf("replay projection leaked or lost identity: %#v", decoded)
	}
}

func TestTeslaFC100TEDAPIRejectsMalformedOrOverBoundedFirstVarint(t *testing.T) {
	for _, payload := range [][]byte{
		{},
		{2, 0x08},
		{1, 0x80},
		{5, 0x80, 0x80, 0x80, 0x80, 0x10},
		{10, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x02},
	} {
		if _, err := DecodeTeslaFC100TEDAPI(payload); err == nil {
			t.Fatalf("invalid FC100 TEDAPI payload accepted: %x", payload)
		}
	}
}
