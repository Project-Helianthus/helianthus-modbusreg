package tesla

import (
	"bytes"
	"context"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/internal/qualifiedfunctiontest"
)

func TestTeslaHSCProfileFailsClosedUntilExplicitlyQualified(t *testing.T) {
	framing, err := NewHSCProfile(HSCProfileConfig{
		Enabled:              true,
		Node:                 0x10,
		PassiveCompatible:    true,
		CompatibilityVersion: "unknown",
	})
	if err != nil {
		t.Fatal(err)
	}
	if framing.Disposition() != HSCFramingOnly || framing.OutboundAllowed() {
		t.Fatalf("framing profile = %#v", framing)
	}
	qualified, err := NewHSCProfile(HSCProfileConfig{
		Enabled:              true,
		Node:                 0x10,
		PassiveCompatible:    true,
		CompatibilityVersion: HSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if qualified.Disposition() != HSCQualifiedReadOnly {
		t.Fatalf("qualified disposition = %q", qualified.Disposition())
	}
	if qualified.OutboundAllowed() {
		t.Fatal("opaque profile permitted live outbound operation")
	}
}

func TestTeslaHSCEnvelopesValidateAndRetainOpaquePayload(t *testing.T) {
	for _, function := range []modbus.PrivateFunctionCode{
		teslaHSCFunction100,
		teslaHSCFunction101,
		teslaHSCFunction102,
	} {
		envelope, err := DecodeTeslaHSCRequestEnvelope(function, []byte{2, 0xaa, 0xbb})
		if err != nil {
			t.Fatalf("function %d: %v", function, err)
		}
		if got, want := envelope.Payload(), []byte{0xaa, 0xbb}; !bytes.Equal(got, want) {
			t.Fatalf("function %d payload = %x, want %x", function, got, want)
		}
	}
	for _, payload := range [][]byte{nil, {1}, {1, 0xaa, 0xbb}} {
		if _, err := DecodeTeslaHSCEnvelope(teslaHSCFunction100, payload); err == nil {
			t.Fatalf("FC100 invalid payload accepted: %x", payload)
		}
	}
	if _, err := DecodeTeslaHSCEnvelope(teslaHSCFunction101, []byte{0}); err == nil {
		t.Fatal("FC101 response envelope accepted as FC100 data")
	}
}

func TestTeslaHSCExceptionAndRedactedProvenance(t *testing.T) {
	classification := ClassifyTeslaHSCException(teslaHSCFunction101, 4)
	if classification != HSCExceptionCodecFailure {
		t.Fatalf("exception classification = %q", classification)
	}
	provenance := NewHSCProvenance(
		HSCCompatibilityV1,
		0x10,
		teslaHSCFunction102,
		[]byte("sensitive-fixture-identifier"),
	)
	if provenance.PayloadLength() != len("sensitive-fixture-identifier") ||
		provenance.PayloadDigest() == "" {
		t.Fatalf("provenance = %#v", provenance)
	}
	if bytes.Contains([]byte(provenance.PayloadDigest()), []byte("sensitive")) {
		t.Fatal("provenance leaked raw payload")
	}
}

func TestTeslaQualifiedFunctionProfileKeepsUnknownOperationNoSend(t *testing.T) {
	profile, err := NewHSCProfile(HSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: HSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := modbusreg.NewQualifiedFunctionRegistry([]modbusreg.QualifiedFunctionProfile{
		{Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: HSCProfileName, Codec: profile},
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := &qualifiedfunctiontest.Exchange{}
	if _, err := registry.Dispatch(context.Background(), transport, modbusreg.QualifiedFunctionSelector{
		Endpoint: "rtu-a", UnitID: profile.Node(), VendorProfile: HSCProfileName, Operation: "unknown",
	}); err == nil {
		t.Fatal("unknown Tesla operation sent")
	}
	if transport.Calls != 0 {
		t.Fatalf("Tesla no-send calls = %d", transport.Calls)
	}
}
