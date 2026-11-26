package reader

import (
	"context"
	"io"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbus/session"
	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
)

// A delimiter can have been read by the transport just as the instance stops.
// This source returns that last byte only after Close, even after cancellation.
type lastByteOnClose struct {
	prefix          []byte
	waiting, closed chan struct{}
}

func (s *lastByteOnClose) ReceiveByte(context.Context) (byte, error) {
	if len(s.prefix) > 0 {
		value := s.prefix[0]
		s.prefix = s.prefix[1:]
		return value, nil
	}
	select {
	case <-s.waiting:
		return 0, io.EOF
	default:
		close(s.waiting)
		<-s.closed
		return 0xc0, nil
	}
}

func (s *lastByteOnClose) Close() error { close(s.closed); return nil }

func TestLegacyMonitorFencesLateFrameOnStop(t *testing.T) {
	wire := legacyFixture(0xfd, 0xe0, 0x0102, 0x0304, 0)
	s := &lastByteOnClose{wire[:len(wire)-1], make(chan struct{}), make(chan struct{})}
	profile, err := tesla.NewLegacyWallConnectorProfile(tesla.LegacyWallConnectorProfileConfig{
		Enabled: true, Compatibility: tesla.LegacyWallConnectorFamilyCompatible,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := NewLegacyMonitor(LegacyConfig{SerialPath: "synthetic-device", Source: "source:legacy-a", Profile: profile}, func(string, uint32) (session.ByteSource, error) { return s, nil })
	ctx := context.Background()
	if err := m.Start(ctx, nil); err != nil {
		t.Fatal(err)
	}
	<-s.waiting
	if report := m.Stop(ctx); report.Err != nil || !report.Confirmed {
		t.Fatalf("stop: %+v", report)
	}
	if m.last.Secondary != nil {
		t.Fatal("late delimiter updated retained evidence")
	}
	if report := m.Stop(ctx); report.Err != nil || !report.Confirmed {
		t.Fatalf("second stop: %+v", report)
	}
	if err := m.Start(ctx, nil); err == nil {
		t.Fatal("reopened retired monitor")
	}
}

func TestLegacyMonitorUsesConfiguredIdentity(t *testing.T) {
	profile, err := tesla.NewLegacyWallConnectorProfile(tesla.LegacyWallConnectorProfileConfig{
		Enabled: true, Compatibility: tesla.LegacyWallConnectorFamilyCompatible,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := LegacyBinding{EVSE: "resource:legacy-evse", Connector: "resource:legacy-connector", PrimaryID: 0x0102, SecondaryID: 0x0304}
	const source = "source:legacy-a"
	for _, axis := range []string{"Source", "EVSE", "Connector"} {
		t.Run(axis, func(t *testing.T) {
			binding := base
			s := &legacyScript{bytes: make(chan byte)}
			cfg := LegacyConfig{SerialPath: "synthetic-device", Source: source, Profile: profile, Binding: &binding}
			switch axis {
			case "Source":
				cfg.Source = "source:legacy-b"
			case "EVSE":
				binding.EVSE = "resource:legacy-other-evse"
			case "Connector":
				binding.Connector = "resource:legacy-other-connector"
			}
			m := NewLegacyMonitor(cfg, func(string, uint32) (session.ByteSource, error) { return s, nil })
			want := binding
			binding.EVSE, binding.Connector = "resource:unowned-evse", "resource:unowned-connector"
			binding.PrimaryID, binding.SecondaryID = 0x0506, 0x0708
			ctx := context.Background()
			if err := m.Start(ctx, nil); err != nil {
				t.Fatal(err)
			}
			defer m.Stop(ctx)
			s.feed(legacyFixture(0xfd, 0xe0, want.SecondaryID, want.PrimaryID, 0))
			s.feed([]byte{0xc0})
			got := m.Snapshot().Secondary
			if got == nil || got.Source != cfg.Source || got.EVSE != want.EVSE || got.Connector != want.Connector || got.Disposition != LegacyAttributed {
				t.Fatalf("got %+v, want source %s and binding %+v", got, cfg.Source, want)
			}
		})
	}
}
