package modbusreg

import (
	"bytes"
	"context"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestTeslaFC100CommonSystemInfoIsExplicitlyVersionQualified(t *testing.T) {
	profile, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled:                    true,
		Node:                       0x10,
		PassiveCompatible:          true,
		CompatibilityVersion:       TeslaHSCCompatibilityV1,
		SystemInfoOperationVersion: TeslaHSCSystemInfoCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	admission := profile.OperationAdmissionFor(TeslaTEDAPIOperationCommonSystemInfoV1)
	if !admission.OutboundAllowed || admission.State != TeslaTEDAPIAdmissionAllowedCommonSystemInfo {
		t.Fatalf("admission = %#v", admission)
	}
	request, policy, err := profile.EncodeQualifiedFunction(TeslaTEDAPIOperationCommonSystemInfoV1)
	if err != nil {
		t.Fatal(err)
	}
	if request.FunctionCode() != modbus.PrivateFunctionCode(100) ||
		!bytes.Equal(request.Payload(), []byte{0x04, 0x22, 0x02, 0x12, 0x00}) ||
		policy.MaxAttempts() != 1 {
		t.Fatalf("request/policy = %#v/%#v", request, policy)
	}

	withoutOperationVersion, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := &qualifiedFunctionTestTransport{}
	registry, err := NewQualifiedFunctionRegistry([]QualifiedFunctionProfile{{
		Endpoint: "rtu-a", UnitID: withoutOperationVersion.Node(), VendorProfile: TeslaHSCProfileName, Codec: withoutOperationVersion,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Dispatch(context.Background(), transport, QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: withoutOperationVersion.Node(), VendorProfile: TeslaHSCProfileName,
		Operation: TeslaTEDAPIOperationCommonSystemInfoV1,
	}); err == nil || transport.calls != 0 {
		t.Fatalf("unversioned dispatch = %v, calls = %d", err, transport.calls)
	}
}

func TestTeslaFC100CommonSystemInfoRetainsTerminalOnlyAsRedactedReplay(t *testing.T) {
	profile, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1,
		SystemInfoOperationVersion: TeslaHSCSystemInfoCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewQualifiedFunctionRegistry([]QualifiedFunctionProfile{{
		Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: TeslaHSCProfileName, Codec: profile,
	}})
	if err != nil {
		t.Fatal(err)
	}
	transport := &qualifiedFunctionTestTransport{responsePayloads: [][]byte{{0x06, 0x22, 0x04, 0x1a, 0x02, 0x08, 0x01}}}
	results, err := registry.Dispatch(context.Background(), transport, QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: TeslaHSCProfileName,
		Operation: TeslaTEDAPIOperationCommonSystemInfoV1,
	})
	if err != nil || transport.calls != 1 || len(results) != 1 || len(results[0].Payload) != 0 || results[0].Replay == nil {
		t.Fatalf("terminal dispatch = %#v, %v, calls = %d", results, err, transport.calls)
	}
	replay := results[0].Replay
	if replay.Kind != string(TeslaFC100CommonSystemInfoTerminal) || replay.PayloadLength != 2 ||
		replay.PayloadDigest != "fb8da7eb5b1b399e7321179dac9e9f65773d7331e1e30554e3911e4325e1ef19" {
		t.Fatalf("terminal replay = %#v", replay)
	}
}

func TestTeslaFC100CommonSystemInfoClassifiesExactEchoAndRejectsMalformedTerminal(t *testing.T) {
	echo, err := DecodeTeslaFC100CommonSystemInfoReplay([]byte{0x04, 0x22, 0x02, 0x12, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	if echo.Kind != TeslaFC100CommonSystemInfoIntermediate || echo.SnapshotLength != 0 || echo.SnapshotDigest != "" {
		t.Fatalf("echo = %#v", echo)
	}

	for _, payload := range [][]byte{
		{0x06, 0x22, 0x04, 0x12, 0x02, 0x08, 0x01},
		{0x07, 0x22, 0x04, 0x1a, 0x02, 0x08, 0x01, 0x00},
		{0x06, 0x22, 0x04, 0x1a, 0x02, 0x08},
	} {
		if _, err := DecodeTeslaFC100CommonSystemInfoReplay(payload); err == nil {
			t.Fatalf("malformed terminal accepted: %x", payload)
		}
	}
}
