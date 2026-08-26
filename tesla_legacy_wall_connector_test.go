package modbusreg

import (
	"bytes"
	"testing"
)

func TestTeslaLegacyWallConnectorCodecPreservesNativeFrames(t *testing.T) {
	profile, err := NewTeslaLegacyWallConnectorProfile(TeslaLegacyWallConnectorProfileConfig{
		Enabled: true, Compatibility: TeslaLegacyWallConnectorFamilyCompatible,
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Generation() != TeslaLegacyWallConnectorGenerationCandidate {
		t.Fatalf("generation=%q", profile.Generation())
	}

	request, err := EncodeTeslaLegacyWallConnectorFrame(
		TeslaLegacyCommand{Prefix: 0xfc, Opcode: 0xa1}, []byte{0xc0, 0xdb},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantRequest := []byte{0xc0, 0xfc, 0xa1, 0xdb, 0xdc, 0xdb, 0xdd, 0x3c, 0xc0}
	if !bytes.Equal(request, wantRequest) {
		t.Fatalf("request=%x want=%x", request, wantRequest)
	}

	decoded, err := DecodeTeslaLegacyWallConnectorFrame(request)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Command() != (TeslaLegacyCommand{Prefix: 0xfc, Opcode: 0xa1}) ||
		!bytes.Equal(decoded.Payload(), []byte{0xc0, 0xdb}) ||
		decoded.Operation() != TeslaLegacyOperationUnknown {
		t.Fatalf("decoded=%#v", decoded)
	}

	record, err := profile.NativeRecord(TeslaLegacyWallConnectorResponse, decoded)
	if err != nil {
		t.Fatal(err)
	}
	if record.Compatibility() != TeslaLegacyWallConnectorFamilyCompatible ||
		!bytes.Equal(record.Payload(), []byte{0xc0, 0xdb}) {
		t.Fatalf("record=%#v", record)
	}
}

func TestTeslaLegacyWallConnectorRejectsMalformedFramesBeforeReplay(t *testing.T) {
	for _, wire := range [][]byte{
		{0xc0, 0xfb, 0xe0, 0x01, 0xe0, 0xc0}, // bad checksum
		{0xc0, 0xfb, 0xe0, 0xdb, 0xc0},       // incomplete escape
		{0xfb, 0xe0, 0x01, 0xe1, 0xc0},       // missing opening delimiter
	} {
		if _, err := DecodeTeslaLegacyWallConnectorFrame(wire); err == nil {
			t.Fatalf("wire=%x accepted", wire)
		}
	}
}

