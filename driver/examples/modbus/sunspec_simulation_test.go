package modbus

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	wire "github.com/Project-Helianthus/helianthus-modbus"
	reg "github.com/Project-Helianthus/helianthus-modbusreg"
	"io"
	"net"
	"slices"
	"sync"
	"testing"
	"time"
)

type zeroJitter struct{}

func (zeroJitter) Next(time.Duration) time.Duration { return 0 }

type framePeer struct {
	mu       sync.Mutex
	frames   [][]byte
	cells    []uint16
	target   uint16
	mismatch *uint16
}

func (p *framePeer) requests() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.frames)
}

func (p *framePeer) answer(header, request []byte) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.frames = append(p.frames, append(bytes.Clone(header), request...))
	if len(request) != 5 {
		return nil, fmt.Errorf("unsupported synthetic PDU size %d", len(request))
	}
	addr, operand := binary.BigEndian.Uint16(request[1:]), binary.BigEndian.Uint16(request[3:])
	switch request[0] {
	case 3:
		if int(addr)+int(operand) > len(p.cells) {
			return nil, fmt.Errorf("synthetic address/count outside image")
		}
		return append([]byte{3, byte(operand * 2)}, wordBytes(p.cells[int(addr):int(addr)+int(operand)])...), nil
	case 6:
		if addr != p.target {
			return nil, fmt.Errorf("unexpected synthetic write target")
		}
		p.cells[addr] = operand
		if p.mismatch != nil {
			p.cells[addr] = *p.mismatch
		}
		return request, nil
	default:
		return nil, fmt.Errorf("unexpected request function %d", request[0])
	}
}

func (p *framePeer) serve(c net.Conn) error {
	for {
		var header [7]byte
		if _, err := io.ReadFull(c, header[:]); err != nil {
			return err
		}
		n := int(binary.BigEndian.Uint16(header[4:6])) - 1
		if n < 1 || n > 253 {
			return fmt.Errorf("invalid synthetic exchange length %d", n)
		}
		request := make([]byte, n)
		if _, err := io.ReadFull(c, request); err != nil {
			return err
		}
		reply, err := p.answer(header[:], request)
		if err != nil {
			return err
		}
		binary.BigEndian.PutUint16(header[4:6], uint16(len(reply)+1))
		if _, err = c.Write(append(header[:], reply...)); err != nil {
			return err
		}
	}
}

type simulated struct {
	*framePeer
	endpoint   *wire.TCPEndpoint
	connection wire.TCPConnectionHandle
	name       string
	id         uint64
}

func openSimulation(t *testing.T, cells []uint16) *simulated {
	t.Helper()
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	name := "tcp://" + listener.Addr().String()
	p := &framePeer{cells: cells}
	endpoint, err := wire.NewTCPEndpoint(wire.TCPEndpointConfig{Endpoint: name, Clock: wire.NewRealTCPMonotonicClock(), MaxBufferedBytes: 260, MaxRequestDeadline: time.Second, MaxResponseDeadline: time.Second,
		PoolLimits:      wire.EndpointPoolLimits{MaxConnections: 1, Connection: wire.ConnectionLimits{MaxInFlight: 2, MaxTombstones: 4}},
		SchedulerLimits: wire.SchedulerLimits{MaxActiveAdmissionKeys: 1, ProtectedSlotsPerKey: 1, SharedBurstSlots: 1, TotalQueued: 2, MaxQueuedPerKey: 2, MaxQueuedPerAuthorizationScope: 2, MaxCoalescedDependentsPerKey: 2, MaxRetryAttempts: 1, MaxInFlightRequests: 1},
		Backoff:         wire.BackoffConfig{Floor: time.Millisecond, Ceiling: time.Millisecond, MaxAttempts: 1, Jitter: zeroJitter{}, JitterAlgorithmID: "zero", JitterVersion: "v1", JitterEvidence: "synthetic"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = endpoint.Close() })
	client, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	peer, err := listener.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}
	handle, err := endpoint.OpenConnection(client)
	if err != nil {
		_ = peer.Close()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- p.serve(peer) }()
	t.Cleanup(func() {
		_ = client.Close()
		_ = peer.Close()
		err := <-done
		if err != io.EOF && err != nil {
			if !errors.Is(err, net.ErrClosed) {
				t.Errorf("synthetic peer: %v", err)
			}
		}
	})
	return &simulated{framePeer: p, endpoint: endpoint, connection: handle, name: name}
}

func (s *simulated) read(t *testing.T, address, count uint16) (wire.LogicalReadView, []byte) {
	t.Helper()
	request, err := wire.NewReadRegistersRequest(wire.FunctionReadHoldingRegisters, address, count)
	if err != nil {
		t.Fatal(err)
	}
	s.id++
	_, err = s.endpoint.EnqueueRead(wire.TCPReadPlan{Connection: s.connection, UnitID: fixtureUnit, AuthorizationScope: "synthetic", PollGeneration: 1, DeadlineIdentity: 1, Timeout: time.Second, Reads: []wire.TCPLogicalRead{{LogicalViewID: s.id, Request: request}}})
	if err != nil {
		t.Fatal(err)
	}
	dispatch, ok := s.endpoint.Dispatch()
	if !ok {
		t.Fatal("no scheduled read")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err = s.endpoint.Write(ctx, dispatch); err != nil {
		t.Fatal(err)
	}
	batch, err := s.endpoint.Read(ctx, s.connection)
	if err != nil || len(batch.Views) != 1 {
		t.Fatalf("correlated read: %+v %v", batch, err)
	}
	return batch.Views[0], batch.Responses[0].Bytes()
}

func replaySnapshot(t *testing.T, v wire.LogicalReadView, raw []byte) reg.LogicalViewSnapshot {
	t.Helper()
	p := v.Provenance()
	record := reg.LogicalViewRecord{LogicalViewID: v.LogicalViewID(), WireResponseID: v.WireResponseID(), PhysicalRequestID: p.PhysicalRequestID, Endpoint: p.Wire.Endpoint, ConnectionID: p.Wire.ConnectionID, Transport: p.Wire.Transport, TransportGeneration: p.Wire.TransportGeneration, UnitID: p.Wire.UnitID, RequestedFunction: p.Wire.RequestedFunction, ReceivedFunction: p.Wire.ReceivedFunction, Table: p.Wire.Table, PhysicalOffset: p.Wire.Offset, PhysicalWordCount: p.Wire.Quantity, AuthorizationScope: p.AuthorizationScope, PollGeneration: p.PollGeneration, DeadlineIdentity: p.DeadlineIdentity, LogicalOffset: p.LogicalOffset, LogicalWordCount: p.LogicalWordCount, SliceOffset: p.SliceOffset, SliceWordCount: p.SliceWordCount, Words: v.Words(), WireResponseBytes: raw}
	snap, err := reg.NewLogicalViewSnapshot(record)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}
