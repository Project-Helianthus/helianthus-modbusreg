package modbusreg

import (
	"bytes"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
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

func TestTeslaHSCNativeRequestAndResponseBounds(t *testing.T) {
	maxRequest := bytes.Repeat([]byte{0xa5}, 251)
	request, err := BuildTeslaHSCNativeRequest(teslaHSCFunction102, maxRequest)
	if err != nil {
		t.Fatal(err)
	}
	if got := request.Payload(); len(got) != 252 || got[0] != 251 || !bytes.Equal(got[1:], maxRequest) {
		t.Fatalf("maximum request = %x", got)
	}
	if _, err := BuildTeslaHSCNativeRequest(teslaHSCFunction101, append(maxRequest, 0)); err == nil {
		t.Fatal("accepted 252-byte native request message")
	}
	if request, err := BuildTeslaHSCNativeRequest(teslaHSCFunction101, nil); err != nil || !bytes.Equal(request.Payload(), []byte{0}) {
		t.Fatalf("empty request = %x, %v", request.Payload(), err)
	}

	maximumResponse := bytes.Repeat([]byte{0x5a}, 252)
	record, err := DecodeTeslaHSCNativeRecord(TeslaHSCCompatibilityV1, teslaHSCFunction101, maximumResponse)
	if err != nil || !bytes.Equal(record.Payload(), maximumResponse) {
		t.Fatalf("maximum response = %x, %v", record.Payload(), err)
	}
	if _, err := DecodeTeslaHSCNativeRecord(TeslaHSCCompatibilityV1, teslaHSCFunction102, append(maximumResponse, 0)); err == nil {
		t.Fatal("accepted oversized native response")
	}
}

func TestTeslaHSCNativeRecordRejectsUnsupportedAndExceptionFunctions(t *testing.T) {
	if _, err := BuildTeslaHSCNativeRequest(teslaHSCFunction100, nil); err == nil {
		t.Fatal("accepted FC100 as a native request")
	}
	if _, err := DecodeTeslaHSCNativeRecord("other", teslaHSCFunction101, nil); err == nil {
		t.Fatal("accepted unsupported compatibility")
	}
	exceptionFunction := modbus.PrivateFunctionCode(byte(teslaHSCFunction101) | 0x80)
	if _, err := DecodeTeslaHSCNativeRecord(TeslaHSCCompatibilityV1, exceptionFunction, []byte{4}); err == nil {
		t.Fatal("decoded transport exception as native record")
	}
}
