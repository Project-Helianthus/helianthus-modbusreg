package tesla

import (
	"bytes"
	"context"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/internal/qualifiedfunctiontest"
)

func TestTeslaFC100WCLifetimeIsVersionQualifiedAndNoSendWhenUngated(t *testing.T) {
	profile, err := NewHSCProfile(HSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: HSCCompatibilityV1,
		WCLifetimeOperationVersion: HSCWCLifetimeCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, policy, err := profile.EncodeQualifiedFunction(TEDAPIOperationWCLifetimeV1)
	if err != nil || request.FunctionCode() != modbus.PrivateFunctionCode(100) ||
		!bytes.Equal(request.Payload(), []byte{0x04, 0x32, 0x02, 0x1a, 0x00}) || policy.MaxAttempts() != 1 {
		t.Fatalf("request/policy/error = %#v/%#v/%v", request, policy, err)
	}
	blocked, err := NewHSCProfile(HSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: HSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := modbusreg.NewQualifiedFunctionRegistry([]modbusreg.QualifiedFunctionProfile{{
		Endpoint: "rtu-a", UnitID: blocked.Node(), VendorProfile: HSCProfileName, Codec: blocked,
	}})
	if err != nil {
		t.Fatal(err)
	}
	transport := &qualifiedfunctiontest.Exchange{}
	if _, err := registry.Dispatch(context.Background(), transport, modbusreg.QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: blocked.Node(), VendorProfile: HSCProfileName, Operation: TEDAPIOperationWCLifetimeV1,
	}); err == nil || transport.Calls != 0 {
		t.Fatalf("ungated dispatch = %v, calls = %d", err, transport.Calls)
	}
}

func TestTeslaFC100WCLifetimeReplaySequenceIsBoundedAndOpaque(t *testing.T) {
	results, err := DecodeTeslaFC100WCLifetimeReplaySequence([][]byte{
		{0x04, 0x32, 0x02, 0x1a, 0x00},
		{0x06, 0x32, 0x04, 0x22, 0x02, 0x08, 0x01},
	})
	if err != nil || len(results) != 2 || results[0].Kind != FC100WCLifetimeIntermediate ||
		results[1].Kind != FC100WCLifetimeTerminal || results[1].SnapshotLength != 2 || results[1].SnapshotDigest == "" {
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
	profile, err := NewHSCProfile(HSCProfileConfig{Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: HSCCompatibilityV1, WCLifetimeOperationVersion: HSCWCLifetimeCompatibilityV1})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := modbusreg.NewQualifiedFunctionRegistry([]modbusreg.QualifiedFunctionProfile{{Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: HSCProfileName, Codec: profile}})
	if err != nil {
		t.Fatal(err)
	}
	for _, payloads := range [][][]byte{{{0x06, 0x32, 0x04, 0x22, 0x02, 0x08, 0x01}, {0x04, 0x32, 0x02, 0x1a, 0x00}}, {{0x06, 0x32, 0x04, 0x1a, 0x02, 0x08, 0x01}}} {
		transport := &qualifiedfunctiontest.Exchange{ResponsePayloads: payloads}
		results, err := registry.Dispatch(context.Background(), transport, modbusreg.QualifiedFunctionSelector{Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: HSCProfileName, Operation: TEDAPIOperationWCLifetimeV1})
		if err == nil || len(results) != 0 {
			t.Fatalf("invalid dispatch = %#v, %v", results, err)
		}
	}
	transport := &qualifiedfunctiontest.Exchange{ResponsePayloads: [][]byte{{0x04, 0x32, 0x02, 0x1a, 0x00}}}
	results, err := registry.Dispatch(context.Background(), transport, modbusreg.QualifiedFunctionSelector{Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: HSCProfileName, Operation: TEDAPIOperationWCLifetimeV1})
	if err == nil || len(results) != 0 {
		t.Fatalf("echo-only dispatch = %#v, %v", results, err)
	}
}
