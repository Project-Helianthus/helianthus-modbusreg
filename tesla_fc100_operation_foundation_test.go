package modbusreg

import (
	"bytes"
	"testing"
)

func TestTeslaFC100OperationFoundationBuildsFixedAndOpaqueRequests(t *testing.T) {
	fixed, err := BuildTeslaFC100OperationRequest(
		TeslaHSCCompatibilityV1,
		TeslaFC100OperationWCGetConfig,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fixed.Payload(), []byte{0x04, 0x32, 0x02, 0x2a, 0x00}; !bytes.Equal(got, want) {
		t.Fatalf("GetConfig payload = %x, want %x", got, want)
	}

	opaque, err := BuildTeslaFC100OperationRequest(
		TeslaHSCCompatibilityV1,
		TeslaFC100OperationWCConfigureSettings,
		[]byte{0x08, 0x01},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := opaque.Payload(), []byte{0x06, 0x32, 0x04, 0x3a, 0x02, 0x08, 0x01}; !bytes.Equal(got, want) {
		t.Fatalf("ConfigureSettings payload = %x, want %x", got, want)
	}
}

func TestTeslaFC100OperationFoundationRetainsTerminalAndApplicationError(t *testing.T) {
	request, err := BuildTeslaFC100OperationRequest(TeslaHSCCompatibilityV1, TeslaFC100OperationWCGetConfig, nil)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := DecodeTeslaFC100OperationSequence(
		TeslaHSCCompatibilityV1,
		TeslaFC100OperationWCGetConfig,
		request.Payload(),
		[][]byte{{0x04, 0x32, 0x02, 0x32, 0x00}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(terminal) != 1 || terminal[0].Kind != TeslaFC100OperationTerminal || !bytes.Equal(terminal[0].Body, []byte{}) {
		t.Fatalf("terminal = %#v", terminal)
	}

	applicationError, err := DecodeTeslaFC100OperationSequence(
		TeslaHSCCompatibilityV1,
		TeslaFC100OperationWCGetConfig,
		request.Payload(),
		[][]byte{{0x06, 0x22, 0x04, 0x0a, 0x02, 0x08, 0x0e}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(applicationError) != 1 || applicationError[0].Kind != TeslaFC100OperationApplicationError || applicationError[0].Status != 14 {
		t.Fatalf("application error = %#v", applicationError)
	}
}
