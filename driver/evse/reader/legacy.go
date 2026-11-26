// Package reader receives charger observations for driver hosts.
package reader

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"sync/atomic"

	"github.com/Project-Helianthus/helianthus-modbus/session"
	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

// LegacyBinding supplies the configured subjects and the two endpoints of the
// heartbeat exchange. The IDs qualify a frame for this binding, not a device.
type LegacyBinding struct {
	EVSE, Connector        semreg.ResourceID
	PrimaryID, SecondaryID uint16
}

// LegacyConfig identifies the receive-only source, profile and optional binding.
type LegacyConfig struct {
	SerialPath string
	Source     semreg.SourceID
	Profile    tesla.LegacyWallConnectorProfile
	Binding    *LegacyBinding
}

// LegacyDisposition describes whether a validated frame belongs to a configured connector.
type LegacyDisposition string

// These dispositions separate connector attribution from raw-only decoding.
const (
	LegacyAttributed   LegacyDisposition = "attributed"
	LegacyUnattributed LegacyDisposition = "connector_unattributed"
	LegacyRawOnly      LegacyDisposition = "raw_only"
)

// LegacyObservation retains a validated frame even when it cannot be fed to
// the connector's fact rows. Wire and both typed records are detached copies.
type LegacyObservation struct {
	Source          semreg.SourceID
	EVSE, Connector semreg.ResourceID
	Disposition     LegacyDisposition
	Native          tesla.LegacyWallConnectorRecord
	Wire            []byte
	Primary         *tesla.LegacyFBE0Observation
	Secondary       *tesla.LegacyFDE0Observation
}

// LegacySnapshot holds the latest retained heartbeats in each direction.
type LegacySnapshot struct{ Primary, Secondary *LegacyObservation }

// LegacyMonitor owns one receive-only legacy source and its retained heartbeats.
type LegacyMonitor struct {
	cfg       LegacyConfig
	open      func(string, uint32) (session.ByteSource, error)
	fenced    atomic.Bool
	mu        sync.Mutex
	source    session.ByteSource
	cancel    context.CancelFunc
	done      chan struct{}
	last      LegacySnapshot
	closeOnce sync.Once
	closeErr  error
}

var _ driver.Instance = (*LegacyMonitor)(nil)

// NewLegacyMonitor constructs one receive-only instance for the configured
// legacy family. The optional opener is for callers providing a ByteSource.
func NewLegacyMonitor(cfg LegacyConfig, open func(string, uint32) (session.ByteSource, error)) *LegacyMonitor {
	if open == nil {
		open = session.OpenPassiveSerial
	}
	if cfg.Binding != nil {
		binding := *cfg.Binding
		cfg.Binding = &binding
	}
	return &LegacyMonitor{cfg: cfg, open: open}
}

// Start opens the configured receive-only source and begins reading on the instance lifetime.
func (m *LegacyMonitor) Start(ctx context.Context, _ driver.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fenced.Load() || m.source != nil {
		return errors.New("legacy monitor already stopped or started")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.cfg.SerialPath == "" {
		return errors.New("legacy monitor SerialPath is required")
	}
	if err := m.cfg.Source.Validate(); err != nil {
		return fmt.Errorf("legacy monitor Source: %w", err)
	}
	if m.cfg.Profile.Compatibility() == "" {
		return errors.New("legacy monitor Profile is required")
	}
	if m.cfg.Binding != nil {
		if err := m.cfg.Binding.EVSE.Validate(); err != nil {
			return fmt.Errorf("legacy monitor Binding.EVSE: %w", err)
		}
		if err := m.cfg.Binding.Connector.Validate(); err != nil {
			return fmt.Errorf("legacy monitor Binding.Connector: %w", err)
		}
	}
	source, err := m.open(m.cfg.SerialPath, 9600)
	if err != nil {
		return legacySerialError(err)
	}
	if source == nil {
		return errors.New("legacy monitor opener returned no source")
	}
	lifetime, cancel := context.WithCancel(context.WithoutCancel(ctx))
	m.source, m.cancel, m.done = source, cancel, make(chan struct{})
	go m.receive(lifetime, source)
	return nil
}

// FenceWithdrawal prevents further exposure of observations.
func (m *LegacyMonitor) FenceWithdrawal() { m.fenced.Store(true) }

// Stop fences observations and closes the read source.
func (m *LegacyMonitor) Stop(ctx context.Context) driver.CloseReport {
	m.FenceWithdrawal()
	m.mu.Lock()
	source, cancel, done := m.source, m.cancel, m.done
	m.mu.Unlock()
	if source == nil {
		return driver.CloseReport{Confirmed: true}
	}
	m.closeOnce.Do(func() { cancel(); m.closeErr = legacySerialError(source.Close()) })
	select {
	case <-done:
		return driver.CloseReport{Confirmed: m.closeErr == nil, Err: m.closeErr}
	case <-ctx.Done():
		return driver.CloseReport{Err: errors.Join(m.closeErr, ctx.Err())}
	}
}

func legacySerialError(err error) error {
	var path *fs.PathError
	if errors.As(err, &path) {
		return fmt.Errorf("serial %s: %w", path.Op, path.Err)
	}
	return err
}

// Snapshot returns detached copies of the retained heartbeats while unfenced.
func (m *LegacyMonitor) Snapshot() LegacySnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fenced.Load() {
		return LegacySnapshot{}
	}
	return LegacySnapshot{Primary: cloneLegacyObservation(m.last.Primary), Secondary: cloneLegacyObservation(m.last.Secondary)}
}

func cloneLegacyObservation(record *LegacyObservation) *LegacyObservation {
	if record == nil {
		return nil
	}
	out := *record
	out.Wire = bytes.Clone(out.Wire)
	if out.Primary != nil {
		value := *out.Primary
		out.Primary = &value
	}
	if out.Secondary != nil {
		value := *out.Secondary
		out.Secondary = &value
	}
	return &out
}

func (m *LegacyMonitor) receive(ctx context.Context, source session.ByteSource) {
	defer close(m.done)
	const delimiter byte = 0xc0
	// The decoder accepts up to 256 message bytes, each possibly escaped,
	// with two framing delimiters.
	var bounded [2*256 + 2]byte
	n := 0
	for !m.fenced.Load() {
		b, err := source.ReceiveByte(ctx)
		if err != nil || m.fenced.Load() {
			return
		}
		if b == delimiter {
			if n > 1 {
				bounded[n] = b
				m.retain(bounded[:n+1])
			}
			bounded[0], n = b, 1
		} else if n == len(bounded)-1 {
			n = 0
		} else if n > 0 {
			bounded[n] = b
			n++
		}
	}
}

func (m *LegacyMonitor) retain(wire []byte) {
	frame, err := tesla.DecodeTeslaLegacyWallConnectorFrame(wire)
	if err != nil {
		return
	}
	direction := tesla.LegacyWallConnectorRequest
	switch frame.Operation() {
	case tesla.LegacyOperationPrimaryHeartbeat:
	case tesla.LegacyOperationSecondaryHeartbeat:
		direction = tesla.LegacyWallConnectorResponse
	default:
		return
	}
	native, err := m.cfg.Profile.NativeRecord(direction, frame)
	if err != nil {
		return
	}
	record := &LegacyObservation{Native: native, Disposition: LegacyRawOnly, Source: m.cfg.Source, Wire: bytes.Clone(wire)}
	if native.Compatibility() != tesla.LegacyWallConnectorUnknown {
		if !m.attributeHeartbeat(record, direction) {
			return
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fenced.Load() {
		return
	}
	if direction == tesla.LegacyWallConnectorRequest {
		m.last.Primary = record
	} else {
		m.last.Secondary = record
	}
}

func (m *LegacyMonitor) attributeHeartbeat(record *LegacyObservation, direction tesla.LegacyDirection) bool {
	p := record.Native.Payload()
	if len(p) != 11 && len(p) != 13 {
		return false
	}
	qualification := tesla.LegacyQualification(record.Native.Compatibility())
	state, allocation := tesla.LegacyFDE0State(p[4]), binary.BigEndian.Uint16(p[5:7])
	src, dst := binary.BigEndian.Uint16(p[:2]), binary.BigEndian.Uint16(p[2:4])
	binding := m.cfg.Binding
	if binding != nil {
		wantSource, wantDestination := binding.PrimaryID, binding.SecondaryID
		if direction == tesla.LegacyWallConnectorResponse {
			wantSource, wantDestination = wantDestination, wantSource
		}
		if src != wantSource || dst != wantDestination {
			return false
		}
	}
	if direction == tesla.LegacyWallConnectorRequest {
		obs, err := tesla.NewLegacyFBE0Observation(tesla.LegacyFBE0ObservationSpec{Profile: m.cfg.Profile, Direction: direction, Qualification: qualification, State: state, AllocatedMaxCA: allocation, Raw: record.Wire})
		if err != nil {
			return false
		}
		record.Primary = &obs
	} else {
		obs, err := tesla.NewLegacyFDE0Observation(tesla.LegacyFDE0ObservationSpec{Profile: m.cfg.Profile, Direction: direction, Qualification: qualification, State: state, AllocatedMaxCA: allocation, ActualCurrentCA: binary.BigEndian.Uint16(p[7:9]), Raw: record.Wire})
		if err != nil {
			return false
		}
		record.Secondary = &obs
	}
	record.Disposition = LegacyUnattributed
	if binding != nil {
		record.Disposition, record.EVSE, record.Connector = LegacyAttributed, binding.EVSE, binding.Connector
	}
	return true
}
