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

func TestTeslaFC100TEDAPISummarizesOrderedBoundedWireEntries(t *testing.T) {
	message := []byte{
		0x08, 0x96, 0x01, // field 1, varint
		0x12, 0x02, 0xaa, 0xbb, // field 2, length-delimited
		0x1d, 0x01, 0x02, 0x03, 0x04, // field 3, fixed32
		0x23,       // field 4, start group
		0x28, 0x01, // field 5, varint inside group
		0x24, // field 4, end group
	}
	decoded, err := DecodeTeslaFC100TEDAPI(append([]byte{byte(len(message))}, message...))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.EnvelopeLength() != len(message)+1 || decoded.WireEntryCount() != 6 || decoded.Payload() != nil {
		t.Fatalf("wire summary metadata = %#v", decoded)
	}
	entries := decoded.WireEntries()
	want := []struct {
		field uint64
		kind  uint8
	}{{1, 0}, {2, 2}, {3, 5}, {4, 3}, {5, 0}, {4, 4}}
	if len(entries) != len(want) {
		t.Fatalf("wire entries = %#v", entries)
	}
	for index, entry := range entries {
		if entry.FieldNumber() != want[index].field || entry.WireType() != want[index].kind {
			t.Fatalf("wire entry[%d] = %#v, want field=%d kind=%d", index, entry, want[index].field, want[index].kind)
		}
	}
}

func TestTeslaFC100TEDAPIRejectsMalformedOrOverCountedWireSummary(t *testing.T) {
	overCountMessage := make([]byte, 0, 130)
	for range 65 {
		overCountMessage = append(overCountMessage, 0x08, 0x00)
	}
	for _, message := range [][]byte{
		{0x12, 0x02, 0xaa}, // truncated length-delimited value
		{0x09, 0x01},       // truncated fixed64 value
		{0x0c},             // unpaired end group
		{0x0b, 0x14},       // mismatched group boundaries
		overCountMessage,
	} {
		payload := append([]byte{byte(len(message))}, message...)
		if _, err := DecodeTeslaFC100TEDAPI(payload); err == nil {
			t.Fatalf("invalid wire summary accepted: %x", payload)
		}
	}
}
