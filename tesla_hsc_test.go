package modbusreg

import (
	"bytes"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestTeslaHSCProfileFailsClosedUntilExplicitlyQualified(t *testing.T) {
	framing, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled:              true,
		Node:                 0x10,
		PassiveCompatible:    true,
		CompatibilityVersion: "unknown",
	})
	if err != nil {
		t.Fatal(err)
	}
	if framing.Disposition() != TeslaHSCFramingOnly || framing.OutboundAllowed() {
		t.Fatalf("framing profile = %#v", framing)
	}
	qualified, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled:              true,
		Node:                 0x10,
		PassiveCompatible:    true,
		CompatibilityVersion: TeslaHSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if qualified.Disposition() != TeslaHSCQualifiedReadOnly {
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
	if classification != TeslaHSCExceptionCodecFailure {
		t.Fatalf("exception classification = %q", classification)
	}
	provenance := NewTeslaHSCProvenance(
		TeslaHSCCompatibilityV1,
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
