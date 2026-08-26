package modbusreg

import (
	"bytes"
	"context"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestTeslaFC100WCLifetimeIsVersionQualifiedAndNoSendWhenUngated(t *testing.T) {
	profile, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1,
		WCLifetimeOperationVersion: TeslaHSCWCLifetimeCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, policy, err := profile.EncodeQualifiedFunction(TeslaTEDAPIOperationWCLifetimeV1)
	if err != nil || request.FunctionCode() != modbus.PrivateFunctionCode(100) ||
		!bytes.Equal(request.Payload(), []byte{0x04, 0x32, 0x02, 0x1a, 0x00}) || policy.MaxAttempts() != 1 {
		t.Fatalf("request/policy/error = %#v/%#v/%v", request, policy, err)
	}
	blocked, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewQualifiedFunctionRegistry([]QualifiedFunctionProfile{{
		Endpoint: "rtu-a", UnitID: blocked.Node(), VendorProfile: TeslaHSCProfileName, Codec: blocked,
	}})
	if err != nil {
		t.Fatal(err)
	}
	transport := &qualifiedFunctionTestTransport{}
	if _, err := registry.Dispatch(context.Background(), transport, QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: blocked.Node(), VendorProfile: TeslaHSCProfileName, Operation: TeslaTEDAPIOperationWCLifetimeV1,
	}); err == nil || transport.calls != 0 {
		t.Fatalf("ungated dispatch = %v, calls = %d", err, transport.calls)
	}
}

func TestTeslaFC100WCLifetimeReplaySequenceIsBoundedAndOpaque(t *testing.T) {
	results, err := DecodeTeslaFC100WCLifetimeReplaySequence([][]byte{
		{0x04, 0x32, 0x02, 0x1a, 0x00},
		{0x06, 0x32, 0x04, 0x22, 0x02, 0x08, 0x01},
	})
	if err != nil || len(results) != 2 || results[0].Kind != TeslaFC100WCLifetimeIntermediate ||
		results[1].Kind != TeslaFC100WCLifetimeTerminal || results[1].SnapshotLength != 2 || results[1].SnapshotDigest == "" {
		t.Fatalf("sequence = %#v, %v", results, err)
	}
	for _, invalid := range [][][]byte{
		{{0x04, 0x32, 0x02, 0x1a, 0x00}, {0x04, 0x32, 0x02, 0x1a, 0x00}},
		{{0x06, 0x32, 0x04, 0x22, 0x02, 0x08, 0x01}, {0x06, 0x32, 0x04, 0x22, 0x02, 0x08, 0x01}},
		{{0x06, 0x32, 0x04, 0x22, 0x02, 0x08, 0x01}, {0x04, 0x32, 0x02, 0x1a, 0x00}},
		{{0x06, 0x32, 0x04, 0x1a, 0x02, 0x08, 0x01}},
	} {
		if _, err := DecodeTeslaFC100WCLifetimeReplaySequence(invalid); err == nil {
			t.Fatalf("invalid sequence accepted: %x", invalid)
		}
	}
}

func TestTeslaFC100WCLifetimeDispatchRejectsInvalidCompleteSequence(t *testing.T) {
	profile, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1, WCLifetimeOperationVersion: TeslaHSCWCLifetimeCompatibilityV1})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewQualifiedFunctionRegistry([]QualifiedFunctionProfile{{Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: TeslaHSCProfileName, Codec: profile}})
	if err != nil {
		t.Fatal(err)
	}
	for _, payloads := range [][][]byte{{{0x06, 0x32, 0x04, 0x22, 0x02, 0x08, 0x01}, {0x04, 0x32, 0x02, 0x1a, 0x00}}, {{0x06, 0x32, 0x04, 0x1a, 0x02, 0x08, 0x01}}} {
		transport := &qualifiedFunctionTestTransport{responsePayloads: payloads}
		results, err := registry.Dispatch(context.Background(), transport, QualifiedFunctionSelector{Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: TeslaHSCProfileName, Operation: TeslaTEDAPIOperationWCLifetimeV1})
		if err == nil || len(results) != 0 {
			t.Fatalf("invalid dispatch = %#v, %v", results, err)
		}
	}
	transport := &qualifiedFunctionTestTransport{responsePayloads: [][]byte{{0x04, 0x32, 0x02, 0x1a, 0x00}}}
	results, err := registry.Dispatch(context.Background(), transport, QualifiedFunctionSelector{Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: TeslaHSCProfileName, Operation: TeslaTEDAPIOperationWCLifetimeV1})
	if err == nil || len(results) != 0 {
		t.Fatalf("echo-only dispatch = %#v, %v", results, err)
	}
}
