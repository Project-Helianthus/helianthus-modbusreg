package modbusreg

import (
	"bytes"
	"context"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestTeslaFC100WCSystemInfoIsVersionQualifiedAndSequenceBounded(t *testing.T) {
	profile, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1, WCSystemInfoOperationVersion: TeslaHSCWCSystemInfoCompatibilityV1})
	if err != nil {
		t.Fatal(err)
	}
	request, policy, err := profile.EncodeQualifiedFunction(TeslaTEDAPIOperationWCSystemInfoV1)
	if err != nil || request.FunctionCode() != modbus.PrivateFunctionCode(100) || !bytes.Equal(request.Payload(), []byte{0x04, 0x32, 0x02, 0x4a, 0x00}) || policy.MaxAttempts() != 1 {
		t.Fatalf("request/policy = %#v/%#v, %v", request, policy, err)
	}
	results, err := DecodeTeslaFC100WCSystemInfoReplaySequence([][]byte{{0x04, 0x32, 0x02, 0x4a, 0x00}, {0x06, 0x32, 0x04, 0x52, 0x02, 0x08, 0x01}})
	if err != nil || len(results) != 2 || results[0].Kind != TeslaFC100WCSystemInfoIntermediate || results[1].Kind != TeslaFC100WCSystemInfoTerminal || results[1].SnapshotLength != 2 || results[1].SnapshotDigest == "" {
		t.Fatalf("results = %#v, %v", results, err)
	}
	for _, invalid := range [][][]byte{{{0x04, 0x32, 0x02, 0x4a, 0x00}}, {{0x06, 0x32, 0x04, 0x52, 0x02, 0x08, 0x01}, {0x04, 0x32, 0x02, 0x4a, 0x00}}, {{0x06, 0x32, 0x04, 0x0a, 0x02, 0x08, 0x01}}} {
		if _, err := DecodeTeslaFC100WCSystemInfoReplaySequence(invalid); err == nil {
			t.Fatalf("invalid sequence accepted: %x", invalid)
		}
	}
}

func TestTeslaFC100WCSystemInfoDispatchIsGatedAndAtomic(t *testing.T) {
	profile, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1, WCSystemInfoOperationVersion: TeslaHSCWCSystemInfoCompatibilityV1})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewQualifiedFunctionRegistry([]QualifiedFunctionProfile{{Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: TeslaHSCProfileName, Codec: profile}})
	if err != nil {
		t.Fatal(err)
	}
	selector := QualifiedFunctionSelector{Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: TeslaHSCProfileName, Operation: TeslaTEDAPIOperationWCSystemInfoV1}
	valid := &qualifiedFunctionTestTransport{responsePayloads: [][]byte{{0x04, 0x32, 0x02, 0x4a, 0x00}, {0x06, 0x32, 0x04, 0x52, 0x02, 0x08, 0x01}}}
	results, err := registry.Dispatch(context.Background(), valid, selector)
	if err != nil || len(results) != 2 || results[0].Replay == nil || results[1].Replay == nil {
		t.Fatalf("valid dispatch = %#v, %v", results, err)
	}
	for _, payloads := range [][][]byte{{{0x04, 0x32, 0x02, 0x4a, 0x00}}, {{0x06, 0x32, 0x04, 0x52, 0x02, 0x08, 0x01}, {0x04, 0x32, 0x02, 0x4a, 0x00}}, {{0x06, 0x32, 0x04, 0x0a, 0x02, 0x08, 0x01}}} {
		transport := &qualifiedFunctionTestTransport{responsePayloads: payloads}
		results, err := registry.Dispatch(context.Background(), transport, selector)
		if err == nil || len(results) != 0 {
			t.Fatalf("invalid dispatch = %#v, %v", results, err)
		}
	}
	ungated, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1})
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := NewQualifiedFunctionRegistry([]QualifiedFunctionProfile{{Endpoint: "rtu-a", UnitID: ungated.Node(), VendorProfile: TeslaHSCProfileName, Codec: ungated}})
	if err != nil {
		t.Fatal(err)
	}
	transport := &qualifiedFunctionTestTransport{}
	if _, err := blocked.Dispatch(context.Background(), transport, selector); err == nil || transport.calls != 0 {
		t.Fatalf("ungated dispatch = %v, calls=%d", err, transport.calls)
	}
}
