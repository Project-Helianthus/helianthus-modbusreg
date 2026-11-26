package reader

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbus/session"
	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
)

// Construct the wire fixture independently: the checksum excludes the first
// command byte, and the two escaped octets must be encoded before delimiting.
func legacyFixture(prefix, opcode byte, src, dst uint16, actual byte) []byte {
	message := []byte{prefix, opcode, byte(src >> 8), byte(src), byte(dst >> 8), byte(dst), 9, 0x04, 0xb0, 0x02, actual, 0xc0, 0xdb}
	var sum byte
	for _, octet := range message[1:] {
		sum += octet
	}
	wire := []byte{0xc0}
	for _, octet := range append(message, sum) {
		switch octet {
		case 0xc0:
			wire = append(wire, 0xdb, 0xdc)
		case 0xdb:
			wire = append(wire, 0xdb, 0xdd)
		default:
			wire = append(wire, octet)
		}
	}
	return append(wire, 0xc0)
}

type legacyScript struct {
	bytes  chan byte
	closed bool
}

func (s *legacyScript) ReceiveByte(ctx context.Context) (byte, error) {
	select {
	case b, ok := <-s.bytes:
		if !ok {
			return 0, io.EOF
		}
		return b, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (s *legacyScript) Close() error { s.closed = true; return nil }

func (s *legacyScript) feed(wire []byte) {
	for _, b := range wire {
		s.bytes <- b
	}
}

func TestLegacyMonitorReadsBoundHeartbeats(t *testing.T) {
	const primary, secondary = 0xdb01, 0x01c0
	s := &legacyScript{bytes: make(chan byte)}
	profile, err := tesla.NewLegacyWallConnectorProfile(tesla.LegacyWallConnectorProfileConfig{Enabled: true, Compatibility: tesla.LegacyWallConnectorFamilyCompatible})
	if err != nil {
		t.Fatal(err)
	}
	cfg := LegacyConfig{SerialPath: "synthetic-device", Source: "source:legacy-a", Profile: profile,
		Binding: &LegacyBinding{EVSE: "resource:evse-a", Connector: "resource:connector-a", PrimaryID: primary, SecondaryID: secondary}}
	opens := 0
	m := NewLegacyMonitor(cfg, func(path string, rate uint32) (session.ByteSource, error) {
		opens++
		if path != cfg.SerialPath || rate != 9600 {
			t.Fatalf("opener: %q %d", path, rate)
		}
		return s, nil
	})
	ctx := context.Background()
	if err := m.Start(ctx, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Stop(ctx) })
	fbe0 := legacyFixture(0xfb, 0xe0, primary, secondary, 0)
	fde0 := legacyFixture(0xfd, 0xe0, secondary, primary, 0x58)
	s.feed([]byte{0x01, 0x02})
	s.feed(fbe0[:6])
	s.feed(fbe0[6:])
	s.feed(fde0)
	badChecksum := legacyFixture(0xfd, 0xe0, secondary, primary, 0x12)
	badChecksum[len(badChecksum)-2] ^= 1
	s.feed(badChecksum)
	s.feed(legacyFixture(0xfd, 0xe0, secondary+1, primary, 0x34))
	s.feed(legacyFixture(0xfd, 0xe0, secondary, primary+1, 0x56))
	s.feed(legacyFixture(0xfb, 0xe0, primary+1, secondary, 0x9a))
	s.feed(legacyFixture(0xfb, 0xe0, primary, secondary+1, 0xbc))
	s.feed(legacyFixture(0xfd, 0xe0, primary, secondary, 0x78))
	s.feed([]byte{0xc0, 0xdb, 0x01, 0xc0})
	s.feed(bytes.Repeat([]byte{0x42}, 515))
	s.feed([]byte{0xc0})
	s.feed(legacyFixture(0xfd, 0xe2, secondary, primary, 0))
	s.feed([]byte{0xc0}) // the next read starts after the unrelated frame has been processed
	got := m.Snapshot()
	if got.Primary == nil || got.Secondary == nil {
		t.Fatalf("missing typed heartbeat: %+v", got)
	}
	for _, record := range []*LegacyObservation{got.Primary, got.Secondary} {
		if record.Source != cfg.Source || record.EVSE != cfg.Binding.EVSE || record.Connector != cfg.Binding.Connector || record.Disposition != LegacyAttributed {
			t.Fatalf("unexpected binding: %+v", record)
		}
	}
	if req := got.Primary.Primary; req == nil || req.SourceID() != primary || req.DestinationID() != secondary || req.AllocatedMaxCA() != 1200 || req.Qualification() != tesla.LegacyFamilyCompatible || !bytes.Equal(req.Raw(), fbe0) {
		t.Fatalf("unexpected request: %+v", got.Primary)
	}
	if reply := got.Secondary.Secondary; reply == nil || reply.SourceID() != secondary || reply.DestinationID() != primary || reply.AllocatedMaxCA() != 1200 || reply.ActualCurrentCA() != 600 || reply.State() != tesla.LegacyFDE0StateCharging || !bytes.Equal(reply.Raw(), fde0) {
		t.Fatalf("unexpected response: %+v", got.Secondary)
	}
	got.Secondary.Wire[0] = 0
	*got.Secondary.Secondary = tesla.LegacyFDE0Observation{}
	if again := m.Snapshot().Secondary; !bytes.Equal(again.Wire, fde0) || again.Secondary.ActualCurrentCA() != 600 {
		t.Fatal("snapshot aliases retained frame")
	}
	if result := m.Stop(ctx); opens != 1 || !s.closed || result.Err != nil {
		t.Fatalf("opens=%d closed=%t stop=%+v", opens, s.closed, result)
	}
	if got := m.Snapshot(); got.Secondary != nil || got.Primary != nil {
		t.Fatalf("stopped snapshot remains visible: %+v", got)
	}
}

func TestLegacyMonitorNativeOnlyDispositions(t *testing.T) {
	for _, tc := range []struct {
		profile tesla.LegacyWallConnectorCompatibility
		want    LegacyDisposition
		typed   bool
	}{
		{tesla.LegacyWallConnectorBuildConfirmed, LegacyUnattributed, true},
		{tesla.LegacyWallConnectorUnknown, LegacyRawOnly, false},
	} {
		t.Run(string(tc.profile), func(t *testing.T) {
			s := &legacyScript{bytes: make(chan byte)}
			profile, err := tesla.NewLegacyWallConnectorProfile(tesla.LegacyWallConnectorProfileConfig{Enabled: true, Compatibility: tc.profile})
			if err != nil {
				t.Fatal(err)
			}
			m := NewLegacyMonitor(LegacyConfig{SerialPath: "synthetic-device", Source: "source:legacy-b", Profile: profile}, func(string, uint32) (session.ByteSource, error) { return s, nil })
			ctx := context.Background()
			if err := m.Start(ctx, nil); err != nil {
				t.Fatal(err)
			}
			defer m.Stop(ctx)
			frame := legacyFixture(0xfd, 0xe0, 0x0102, 0x0304, 0)
			s.feed(frame)
			s.feed([]byte{0xc0})
			got := m.Snapshot().Secondary
			if got == nil || got.Disposition != tc.want || got.Connector != "" || got.EVSE != "" || (got.Secondary != nil) != tc.typed || got.Source != "source:legacy-b" || got.Native.Compatibility() != tc.profile || !bytes.Equal(got.Wire, frame) {
				t.Fatalf("native observation: %+v", got)
			}
		})
	}
}
