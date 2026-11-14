package tesla

import (
	"bytes"
	"testing"
)

func TestTeslaLegacyWallConnectorCodecPreservesNativeFrames(t *testing.T) {
	profile, err := NewLegacyWallConnectorProfile(LegacyWallConnectorProfileConfig{
		Enabled: true, Compatibility: LegacyWallConnectorFamilyCompatible,
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Generation() != LegacyWallConnectorGenerationCandidate {
		t.Fatalf("generation=%q", profile.Generation())
	}

	request, err := EncodeTeslaLegacyWallConnectorFrame(
		LegacyCommand{Prefix: 0xfc, Opcode: 0xa1}, []byte{0xc0, 0xdb},
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
	if decoded.Command() != (LegacyCommand{Prefix: 0xfc, Opcode: 0xa1}) ||
		!bytes.Equal(decoded.Payload(), []byte{0xc0, 0xdb}) ||
		decoded.Operation() != LegacyOperationUnknown {
		t.Fatalf("decoded=%#v", decoded)
	}

	record, err := profile.NativeRecord(LegacyWallConnectorResponse, decoded)
	if err != nil {
		t.Fatal(err)
	}
	if record.Compatibility() != LegacyWallConnectorFamilyCompatible ||
		!bytes.Equal(record.Payload(), []byte{0xc0, 0xdb}) {
		t.Fatalf("record=%#v", record)
	}
	payload := record.Payload()
	payload[0] = 0
	if !bytes.Equal(record.Payload(), []byte{0xc0, 0xdb}) {
		t.Fatalf("record payload was not copied: %x", record.Payload())
	}
}

func TestTeslaLegacyWallConnectorNamesOnlyDocumentedLinkCommands(t *testing.T) {
	tests := []struct {
		command LegacyCommand
		want    LegacyOperation
	}{
		{LegacyCommand{Prefix: 0xfc, Opcode: 0xe1}, LegacyOperationPrimaryDiscovery},
		{LegacyCommand{Prefix: 0xfb, Opcode: 0xe2}, LegacyOperationSecondaryDiscovery},
		{LegacyCommand{Prefix: 0xfd, Opcode: 0xe2}, LegacyOperationSecondaryDiscovery},
		{LegacyCommand{Prefix: 0xfb, Opcode: 0xe0}, LegacyOperationPrimaryHeartbeat},
		{LegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, LegacyOperationSecondaryHeartbeat},
		{LegacyCommand{Prefix: 0xfc, Opcode: 0xa3}, LegacyOperationUnknown},
	}
	for _, test := range tests {
		wire, err := EncodeTeslaLegacyWallConnectorFrame(test.command, nil)
		if err != nil {
			t.Fatal(err)
		}
		frame, err := DecodeTeslaLegacyWallConnectorFrame(wire)
		if err != nil {
			t.Fatal(err)
		}
		if frame.Operation() != test.want {
			t.Fatalf("command=%#v operation=%q want=%q", test.command, frame.Operation(), test.want)
		}
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

func TestTeslaLegacyWallConnectorRoundTripsMaximumEscapedMessage(t *testing.T) {
	payload := bytes.Repeat([]byte{0xc0}, maxTeslaLegacyWallConnectorMessage-3)
	wire, err := EncodeTeslaLegacyWallConnectorFrame(
		LegacyCommand{Prefix: 0xfc, Opcode: 0xa1}, payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) <= maxTeslaLegacyWallConnectorMessage+2 {
		t.Fatalf("wire length=%d does not exercise escape expansion", len(wire))
	}
	frame, err := DecodeTeslaLegacyWallConnectorFrame(wire)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frame.Payload(), payload) {
		t.Fatalf("payload=%x want=%x", frame.Payload(), payload)
	}
}
