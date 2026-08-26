package modbusreg

import (
	"bytes"
	"context"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestTeslaFC100WCLoadSharingStateIsVersionQualifiedAndAtomic(t *testing.T) {
	profile, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1, WCLoadSharingStateOperationVersion: TeslaHSCWCLoadSharingStateCompatibilityV1})
	if err != nil {
		t.Fatal(err)
	}
	request, policy, err := profile.EncodeQualifiedFunction(TeslaTEDAPIOperationWCLoadSharingStateV1)
	if err != nil || request.FunctionCode() != modbus.PrivateFunctionCode(100) || !bytes.Equal(request.Payload(), []byte{0x04, 0x32, 0x02, 0x5a, 0x00}) || policy.MaxAttempts() != 1 {
		t.Fatalf("request/policy = %#v/%#v, %v", request, policy, err)
	}
	registry, err := NewQualifiedFunctionRegistry([]QualifiedFunctionProfile{{Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: TeslaHSCProfileName, Codec: profile}})
	if err != nil {
		t.Fatal(err)
	}
	selector := QualifiedFunctionSelector{Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: TeslaHSCProfileName, Operation: TeslaTEDAPIOperationWCLoadSharingStateV1}
	valid := &qualifiedFunctionTestTransport{responsePayloads: [][]byte{{0x04, 0x32, 0x02, 0x5a, 0x00}, {0x06, 0x32, 0x04, 0x62, 0x02, 0x08, 0x01}}}
	results, err := registry.Dispatch(context.Background(), valid, selector)
	if err != nil || len(results) != 2 || results[0].Replay == nil || results[1].Replay == nil {
		t.Fatalf("valid dispatch = %#v, %v", results, err)
	}
	for _, payloads := range [][][]byte{{{0x04, 0x32, 0x02, 0x5a, 0x00}}, {{0x06, 0x32, 0x04, 0x62, 0x02, 0x08, 0x01}, {0x04, 0x32, 0x02, 0x5a, 0x00}}, {{0x06, 0x32, 0x04, 0x0a, 0x02, 0x08, 0x01}}} {
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
	blockedRegistry, err := NewQualifiedFunctionRegistry([]QualifiedFunctionProfile{{Endpoint: "rtu-a", UnitID: ungated.Node(), VendorProfile: TeslaHSCProfileName, Codec: ungated}})
	if err != nil {
		t.Fatal(err)
	}
	blockedTransport := &qualifiedFunctionTestTransport{}
	if _, err := blockedRegistry.Dispatch(context.Background(), blockedTransport, selector); err == nil || blockedTransport.calls != 0 {
		t.Fatalf("ungated dispatch = %v, calls = %d", err, blockedTransport.calls)
	}
}
