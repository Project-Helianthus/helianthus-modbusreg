package tesla

import (
	"bytes"
	"context"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/internal/qualifiedfunctiontest"
)

func TestTeslaFC100WCLoadSharingStateIsVersionQualifiedAndAtomic(t *testing.T) {
	profile, err := NewHSCProfile(HSCProfileConfig{Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: HSCCompatibilityV1, WCLoadSharingStateOperationVersion: HSCWCLoadSharingStateCompatibilityV1})
	if err != nil {
		t.Fatal(err)
	}
	request, policy, err := profile.EncodeQualifiedFunction(TEDAPIOperationWCLoadSharingStateV1)
	if err != nil || request.FunctionCode() != modbus.PrivateFunctionCode(100) || !bytes.Equal(request.Payload(), []byte{0x04, 0x32, 0x02, 0x5a, 0x00}) || policy.MaxAttempts() != 1 {
		t.Fatalf("request/policy = %#v/%#v, %v", request, policy, err)
	}
	registry, err := modbusreg.NewQualifiedFunctionRegistry([]modbusreg.QualifiedFunctionProfile{{Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: HSCProfileName, Codec: profile}})
	if err != nil {
		t.Fatal(err)
	}
	selector := modbusreg.QualifiedFunctionSelector{Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: HSCProfileName, Operation: TEDAPIOperationWCLoadSharingStateV1}
	valid := &qualifiedfunctiontest.Exchange{ResponsePayloads: [][]byte{{0x04, 0x32, 0x02, 0x5a, 0x00}, {0x06, 0x32, 0x04, 0x62, 0x02, 0x08, 0x01}}}
	results, err := registry.Dispatch(context.Background(), valid, selector)
	if err != nil || len(results) != 2 || results[0].Replay == nil || results[1].Replay == nil {
		t.Fatalf("valid dispatch = %#v, %v", results, err)
	}
	for _, payloads := range [][][]byte{{{0x04, 0x32, 0x02, 0x5a, 0x00}}, {{0x06, 0x32, 0x04, 0x62, 0x02, 0x08, 0x01}, {0x04, 0x32, 0x02, 0x5a, 0x00}}, {{0x06, 0x32, 0x04, 0x0a, 0x02, 0x08, 0x01}}} {
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
	blockedRegistry, err := modbusreg.NewQualifiedFunctionRegistry([]modbusreg.QualifiedFunctionProfile{{Endpoint: "rtu-a", UnitID: ungated.Node(), VendorProfile: HSCProfileName, Codec: ungated}})
	if err != nil {
		t.Fatal(err)
	}
	blockedTransport := &qualifiedfunctiontest.Exchange{}
	if _, err := blockedRegistry.Dispatch(context.Background(), blockedTransport, selector); err == nil || blockedTransport.Calls != 0 {
		t.Fatalf("ungated dispatch = %v, calls = %d", err, blockedTransport.Calls)
	}
}
