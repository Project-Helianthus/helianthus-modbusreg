package tesla

import (
	"bytes"
	"context"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/internal/qualifiedfunctiontest"
)

func TestTeslaFC100CommonSystemInfoIsExplicitlyVersionQualified(t *testing.T) {
	profile, err := NewHSCProfile(HSCProfileConfig{
		Enabled:                    true,
		Node:                       0x10,
		PassiveCompatible:          true,
		CompatibilityVersion:       HSCCompatibilityV1,
		SystemInfoOperationVersion: HSCSystemInfoCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	admission := profile.OperationAdmissionFor(TEDAPIOperationCommonSystemInfoV1)
	if !admission.OutboundAllowed || admission.State != TEDAPIAdmissionAllowedCommonSystemInfo {
		t.Fatalf("admission = %#v", admission)
	}
	request, policy, err := profile.EncodeQualifiedFunction(TEDAPIOperationCommonSystemInfoV1)
	if err != nil {
		t.Fatal(err)
	}
	if request.FunctionCode() != modbus.PrivateFunctionCode(100) ||
		!bytes.Equal(request.Payload(), []byte{0x04, 0x22, 0x02, 0x12, 0x00}) ||
		policy.MaxAttempts() != 1 {
		t.Fatalf("request/policy = %#v/%#v", request, policy)
	}

	withoutOperationVersion, err := NewHSCProfile(HSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: HSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := &qualifiedfunctiontest.Exchange{}
	registry, err := modbusreg.NewQualifiedFunctionRegistry([]modbusreg.QualifiedFunctionProfile{{
		Endpoint: "rtu-a", UnitID: withoutOperationVersion.Node(), VendorProfile: HSCProfileName, Codec: withoutOperationVersion,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Dispatch(context.Background(), transport, modbusreg.QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: withoutOperationVersion.Node(), VendorProfile: HSCProfileName,
		Operation: TEDAPIOperationCommonSystemInfoV1,
	}); err == nil || transport.Calls != 0 {
		t.Fatalf("unversioned dispatch = %v, calls = %d", err, transport.Calls)
	}
}

func TestTeslaFC100CommonSystemInfoRetainsTerminalOnlyAsRedactedReplay(t *testing.T) {
	profile, err := NewHSCProfile(HSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: HSCCompatibilityV1,
		SystemInfoOperationVersion: HSCSystemInfoCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := modbusreg.NewQualifiedFunctionRegistry([]modbusreg.QualifiedFunctionProfile{{
		Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: HSCProfileName, Codec: profile,
	}})
	if err != nil {
		t.Fatal(err)
	}
	transport := &qualifiedfunctiontest.Exchange{ResponsePayloads: [][]byte{{0x06, 0x22, 0x04, 0x1a, 0x02, 0x08, 0x01}}}
	results, err := registry.Dispatch(context.Background(), transport, modbusreg.QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: HSCProfileName,
		Operation: TEDAPIOperationCommonSystemInfoV1,
	})
	if err != nil || transport.Calls != 1 || len(results) != 1 || len(results[0].Payload) != 0 || results[0].Replay == nil {
		t.Fatalf("terminal dispatch = %#v, %v, calls = %d", results, err, transport.Calls)
	}
	replay := results[0].Replay
	if replay.Kind != string(FC100CommonSystemInfoTerminal) || replay.PayloadLength != 2 ||
		replay.PayloadDigest != "fb8da7eb5b1b399e7321179dac9e9f65773d7331e1e30554e3911e4325e1ef19" {
		t.Fatalf("terminal replay = %#v", replay)
	}
}

func TestTeslaFC100CommonSystemInfoClassifiesExactEchoAndRejectsMalformedTerminal(t *testing.T) {
	echo, err := DecodeTeslaFC100CommonSystemInfoReplay([]byte{0x04, 0x22, 0x02, 0x12, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	if echo.Kind != FC100CommonSystemInfoIntermediate || echo.SnapshotLength != 0 || echo.SnapshotDigest != "" {
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
