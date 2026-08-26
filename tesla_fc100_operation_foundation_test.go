package modbusreg

import (
	"bytes"
	"testing"
)

func TestTeslaFC100OperationFoundationBuildsFixedAndOpaqueRequests(t *testing.T) {
	fixed, err := BuildTeslaFC100OperationRequest(
		TeslaHSCFC100OperationCompatibilityV1,
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
		TeslaHSCFC100OperationCompatibilityV1,
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
	request, err := BuildTeslaFC100OperationRequest(TeslaHSCFC100OperationCompatibilityV1, TeslaFC100OperationWCGetConfig, nil)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := DecodeTeslaFC100OperationSequence(
		TeslaHSCFC100OperationCompatibilityV1,
		TeslaFC100OperationWCGetConfig,
		request.Payload(),
		[][]byte{{0x04, 0x32, 0x02, 0x2a, 0x00}, {0x07, 0x32, 0x05, 0x32, 0x03, 0x0a, 0x01, 0xff}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(terminal) != 2 || terminal[0].Kind != TeslaFC100OperationIntermediate || terminal[1].Kind != TeslaFC100OperationTerminal || !bytes.Equal(terminal[1].Body, []byte{0x0a, 0x01, 0xff}) {
		t.Fatalf("terminal = %#v", terminal)
	}

	applicationError, err := DecodeTeslaFC100OperationSequence(
		TeslaHSCFC100OperationCompatibilityV1,
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

func TestTeslaFC100OperationFoundationRejectsInvalidTerminalSequences(t *testing.T) {
	request, err := BuildTeslaFC100OperationRequest(TeslaHSCFC100OperationCompatibilityV1, TeslaFC100OperationWCGetConfig, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, sequence := range [][][]byte{nil, {{0x04, 0x32, 0x02, 0x2a, 0x00}}, {{0x04, 0x32, 0x02, 0x32, 0x00}, {0x04, 0x32, 0x02, 0x32, 0x00}, {0x04, 0x32, 0x02, 0x32, 0x00}}} {
		if _, err := DecodeTeslaFC100OperationSequence(TeslaHSCFC100OperationCompatibilityV1, TeslaFC100OperationWCGetConfig, request.Payload(), sequence); err == nil {
			t.Fatalf("accepted sequence %#v", sequence)
		}
	}
}

func TestTeslaFC100RecoveredNamesCoverAccessVehiclesAndOCPP(t *testing.T) {
	for _, operation := range []TeslaFC100Operation{TeslaFC100OperationWCGetAccessControl, TeslaFC100OperationWCConfigureAccessControl, TeslaFC100OperationWCGetRecentVehicles, TeslaFC100OperationWCGetOCPPSecurity, TeslaFC100OperationNeurioConfigureCTs} {
		names, ok := TeslaFC100RecoveredNames(operation)
		if !ok || names.Request == "" || names.Response == "" || len(names.Fields) == 0 {
			t.Fatalf("missing recovered names for %s: %#v", operation, names)
		}
	}
}
