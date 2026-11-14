package tesla

import (
	"bytes"
	"context"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/internal/qualifiedfunctiontest"
)

func TestTeslaFC100WCSystemInfoIsVersionQualifiedAndSequenceBounded(t *testing.T) {
	profile, err := NewHSCProfile(HSCProfileConfig{Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: HSCCompatibilityV1, WCSystemInfoOperationVersion: HSCWCSystemInfoCompatibilityV1})
	if err != nil {
		t.Fatal(err)
	}
	request, policy, err := profile.EncodeQualifiedFunction(TEDAPIOperationWCSystemInfoV1)
	if err != nil || request.FunctionCode() != modbus.PrivateFunctionCode(100) || !bytes.Equal(request.Payload(), []byte{0x04, 0x32, 0x02, 0x4a, 0x00}) || policy.MaxAttempts() != 1 {
		t.Fatalf("request/policy = %#v/%#v, %v", request, policy, err)
	}
	results, err := DecodeTeslaFC100WCSystemInfoReplaySequence([][]byte{{0x04, 0x32, 0x02, 0x4a, 0x00}, {0x06, 0x32, 0x04, 0x52, 0x02, 0x08, 0x01}})
	if err != nil || len(results) != 2 || results[0].Kind != FC100WCSystemInfoIntermediate || results[1].Kind != FC100WCSystemInfoTerminal || results[1].SnapshotLength != 2 || results[1].SnapshotDigest == "" {
		t.Fatalf("results = %#v, %v", results, err)
	}
	for _, invalid := range [][][]byte{{{0x04, 0x32, 0x02, 0x4a, 0x00}}, {{0x06, 0x32, 0x04, 0x52, 0x02, 0x08, 0x01}, {0x04, 0x32, 0x02, 0x4a, 0x00}}, {{0x06, 0x32, 0x04, 0x0a, 0x02, 0x08, 0x01}}} {
		if _, err := DecodeTeslaFC100WCSystemInfoReplaySequence(invalid); err == nil {
			t.Fatalf("invalid sequence accepted: %x", invalid)
		}
	}
}

func TestTeslaFC100WCSystemInfoDispatchIsGatedAndAtomic(t *testing.T) {
	profile, err := NewHSCProfile(HSCProfileConfig{Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: HSCCompatibilityV1, WCSystemInfoOperationVersion: HSCWCSystemInfoCompatibilityV1})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := modbusreg.NewQualifiedFunctionRegistry([]modbusreg.QualifiedFunctionProfile{{Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: HSCProfileName, Codec: profile}})
	if err != nil {
		t.Fatal(err)
	}
	selector := modbusreg.QualifiedFunctionSelector{Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: HSCProfileName, Operation: TEDAPIOperationWCSystemInfoV1}
	valid := &qualifiedfunctiontest.Exchange{ResponsePayloads: [][]byte{{0x04, 0x32, 0x02, 0x4a, 0x00}, {0x06, 0x32, 0x04, 0x52, 0x02, 0x08, 0x01}}}
	results, err := registry.Dispatch(context.Background(), valid, selector)
	if err != nil || len(results) != 2 || results[0].Replay == nil || results[1].Replay == nil {
		t.Fatalf("valid dispatch = %#v, %v", results, err)
	}
	for _, payloads := range [][][]byte{{{0x04, 0x32, 0x02, 0x4a, 0x00}}, {{0x06, 0x32, 0x04, 0x52, 0x02, 0x08, 0x01}, {0x04, 0x32, 0x02, 0x4a, 0x00}}, {{0x06, 0x32, 0x04, 0x0a, 0x02, 0x08, 0x01}}} {
		transport := &qualifiedfunctiontest.Exchange{ResponsePayloads: payloads}
		results, err := registry.Dispatch(context.Background(), transport, selector)
		if err == nil || len(results) != 0 {
			t.Fatalf("invalid dispatch = %#v, %v", results, err)
		}
	}
	ungated, err := NewHSCProfile(HSCProfileConfig{Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: HSCCompatibilityV1})
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := modbusreg.NewQualifiedFunctionRegistry([]modbusreg.QualifiedFunctionProfile{{Endpoint: "rtu-a", UnitID: ungated.Node(), VendorProfile: HSCProfileName, Codec: ungated}})
	if err != nil {
		t.Fatal(err)
	}
	transport := &qualifiedfunctiontest.Exchange{}
	if _, err := blocked.Dispatch(context.Background(), transport, selector); err == nil || transport.Calls != 0 {
		t.Fatalf("ungated dispatch = %v, calls=%d", err, transport.Calls)
	}
}
