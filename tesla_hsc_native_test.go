package modbusreg

import (
	"bytes"
	"testing"
)

func TestTeslaHSCNativeRecordsRetainFC101AndFC102Payloads(t *testing.T) {
	request, err := BuildTeslaHSCNativeRequest(teslaHSCFunction101, []byte{0x0a, 0x02, 0x08, 0x01})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := request.Payload(), []byte{0x04, 0x0a, 0x02, 0x08, 0x01}; !bytes.Equal(got, want) {
		t.Fatalf("request=%x want=%x", got, want)
	}
	record, err := DecodeTeslaHSCNativeRecord(TeslaHSCCompatibilityV1, teslaHSCFunction102, []byte{0x0a, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	if got := record.Payload(); !bytes.Equal(got, []byte{0x0a, 0x00}) {
		t.Fatalf("payload=%x", got)
	}
}
