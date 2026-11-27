package limits

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	"github.com/Project-Helianthus/helianthus-modbusreg/huawei"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime/life"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/catalog"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

const huaweiActiveKind driver.DriverKind = "modbus.limits.huawei"

const (
	activeAddress = 40126
	ratedAddress  = 30073
)

type activePeer struct {
	mu     sync.Mutex
	units  map[byte][]uint16
	frames [][]byte
	mode   string
	wrote  bool
}

func (p *activePeer) writtenFrames() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out [][]byte
	for _, frame := range p.frames {
		if len(frame) > 7 && frame[7] == 16 {
			out = append(out, append([]byte(nil), frame...))
		}
	}
	return out
}

func (p *activePeer) serve(conn net.Conn) error {
	for {
		var header [7]byte
		if _, err := io.ReadFull(conn, header[:]); err != nil {
			return err
		}
		n := int(binary.BigEndian.Uint16(header[4:6])) - 1
		if n < 1 || n > 253 {
			return errors.New("synthetic exchange length")
		}
		request := make([]byte, n)
		if _, err := io.ReadFull(conn, request); err != nil {
			return err
		}
		reply, drop, err := p.answer(header[6], append(header[:], request...))
		if err != nil || drop {
			_ = conn.Close()
			return err
		}
		binary.BigEndian.PutUint16(header[4:6], uint16(len(reply)+1))
		if _, err = conn.Write(append(header[:], reply...)); err != nil {
			return err
		}
	}
}

func (p *activePeer) answer(unit byte, frame []byte) ([]byte, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.frames = append(p.frames, append([]byte(nil), frame...))
	request := frame[7:]
	cells := p.units[unit]
	switch request[0] {
	case 3:
		if p.mode == "noread" && p.wrote {
			return nil, true, errors.New("synthetic read dropped")
		}
		addr, qty := binary.BigEndian.Uint16(request[1:3]), binary.BigEndian.Uint16(request[3:5])
		if cells == nil || int(addr)+int(qty) > len(cells) {
			return nil, false, errors.New("synthetic read outside image")
		}
		body := []byte{3, byte(qty * 2)}
		for i := uint16(0); i < qty; i++ {
			word := cells[addr+i]
			if p.mode == "conflict" && p.wrote && addr == activeAddress && i == 0 {
				word = 0x0000
			}
			if p.mode == "conflict" && p.wrote && addr == activeAddress && i == 1 {
				word = 0x0FA0
			}
			body = binary.BigEndian.AppendUint16(body, word)
		}
		return body, false, nil
	case 16:
		addr, qty := binary.BigEndian.Uint16(request[1:3]), binary.BigEndian.Uint16(request[3:5])
		if int(request[5]) != int(qty)*2 || len(request) < 6+int(qty)*2 {
			return nil, false, errors.New("synthetic write width")
		}
		if cells != nil && int(addr)+int(qty) <= len(cells) && p.mode != "exception" {
			for i := uint16(0); i < qty; i++ {
				cells[addr+i] = binary.BigEndian.Uint16(request[6+2*i:])
			}
		}
		p.wrote = true
		if p.mode == "drop" {
			return nil, true, errors.New("synthetic response dropped")
		}
		if p.mode == "exception" {
			return []byte{0x90, 0x02}, false, nil
		}
		body := []byte{16}
		body = binary.BigEndian.AppendUint16(body, addr)
		body = binary.BigEndian.AppendUint16(body, qty)
		return body, false, nil
	default:
		return nil, false, errors.New("synthetic function")
	}
}

type activeLink struct {
	mu    sync.Mutex
	conn  net.Conn
	calls int
	peer  *activePeer
	txn   uint16
}

func (s *activeLink) ReadHolding(ctx context.Context, unit uint8, address, quantity uint16) ([]uint16, error) {
	body := []byte{3}
	body = binary.BigEndian.AppendUint16(body, address)
	body = binary.BigEndian.AppendUint16(body, quantity)
	reply, err := s.exchange(ctx, unit, body)
	if err != nil {
		return nil, err
	}
	if len(reply) < 2 || reply[0] != 3 || int(reply[1]) != len(reply)-2 || int(reply[1]) != int(quantity)*2 {
		return nil, errors.New("synthetic read reply")
	}
	words := make([]uint16, quantity)
	for i := range words {
		words[i] = binary.BigEndian.Uint16(reply[2+2*i:])
	}
	return words, nil
}

func (s *activeLink) WriteMultiple(ctx context.Context, unit uint8, address uint16, registers []uint16, admission modbus.RegisterWriteAdmission) (modbus.WriteMultipleRegistersAcknowledgement, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	qty := uint16(len(registers))
	if admission != nil && !admission(modbus.RegisterWriteAdmissionRequest{UnitID: unit, Function: modbus.FunctionWriteMultipleRegisters, Address: address, Quantity: qty, Registers: registers}) {
		return modbus.WriteMultipleRegistersAcknowledgement{}, errors.New("synthetic admission refused")
	}
	body := []byte{16}
	body = binary.BigEndian.AppendUint16(body, address)
	body = binary.BigEndian.AppendUint16(body, qty)
	body = append(body, byte(qty*2))
	for _, word := range registers {
		body = binary.BigEndian.AppendUint16(body, word)
	}
	reply, err := s.exchange(ctx, unit, body)
	if err != nil {
		return modbus.WriteMultipleRegistersAcknowledgement{}, err
	}
	if len(reply) >= 1 && reply[0]&0x80 != 0 {
		code := byte(0)
		if len(reply) > 1 {
			code = reply[1]
		}
		return modbus.WriteMultipleRegistersAcknowledgement{}, &modbus.ProtocolError{
			Kind: modbus.ErrorExceptionResponse, RequestedFunction: modbus.FunctionWriteMultipleRegisters,
			ReceivedFunction: modbus.FunctionCode(reply[0]), ExceptionCode: code, Offset: -1,
		}
	}
	if len(reply) != 5 || reply[0] != 16 {
		return modbus.WriteMultipleRegistersAcknowledgement{}, errors.New("synthetic write reply")
	}
	return modbus.WriteMultipleRegistersAcknowledgement{Address: binary.BigEndian.Uint16(reply[1:3]), Quantity: binary.BigEndian.Uint16(reply[3:5])}, nil
}

func (s *activeLink) exchange(ctx context.Context, unit uint8, pdu []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.txn++
	header := make([]byte, 7)
	binary.BigEndian.PutUint16(header[0:2], s.txn)
	binary.BigEndian.PutUint16(header[4:6], uint16(len(pdu)+1))
	header[6] = unit
	if deadline, ok := ctx.Deadline(); ok {
		_ = s.conn.SetDeadline(deadline)
	}
	if _, err := s.conn.Write(append(header, pdu...)); err != nil {
		return nil, err
	}
	var response [7]byte
	if _, err := io.ReadFull(s.conn, response[:]); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint16(response[4:6])) - 1
	if n < 1 || n > 253 {
		return nil, errors.New("synthetic response length")
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(s.conn, body); err != nil {
		return nil, err
	}
	return body, nil
}

func activeImage(watts uint32) []uint16 {
	cells := make([]uint16, 40128)
	cells[activeAddress] = uint16(watts >> 16)
	cells[activeAddress+1] = uint16(watts)
	cells[ratedAddress] = 0x0001
	cells[ratedAddress+1] = 0x86A0
	return cells
}

func listenActive(t *testing.T, mode string, watts uint32) *activeLink {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	peer := &activePeer{units: map[byte][]uint16{1: activeImage(watts)}, mode: mode}
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		accepted <- conn
		_ = peer.serve(conn)
	}()
	client, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var server net.Conn
	select {
	case server = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("synthetic peer did not accept")
	}
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	return &activeLink{conn: client, peer: peer}
}

type activeRecording struct {
	*HuaweiActive
	startErr error
}

func (r *activeRecording) Start(ctx context.Context, s driver.Session) error {
	r.startErr = r.HuaweiActive.Start(ctx, s)
	return r.startErr
}

type activeFactory struct{ family *activeRecording }

func (activeFactory) Descriptor() driver.Descriptor { return driver.Descriptor{} }
func (activeFactory) Plan(c driver.Config) ([]driver.InstancePlan, error) {
	return []driver.InstancePlan{{Driver: huaweiActiveKind, Instance: driver.InstanceKey(c.Instance)}}, nil
}
func (f activeFactory) Construct(context.Context, driver.InstancePlan) (driver.Instance, error) {
	return f.family, nil
}

func openActive(t *testing.T, access Registers, inverters []ActiveInverter) (*runtime.Runtime, *HuaweiActive) {
	t.Helper()
	rec := &activeRecording{HuaweiActive: &HuaweiActive{Access: access, Inverters: inverters}}
	ctx := context.Background()
	clock := &limitClock{now: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}
	cfg := life.InstanceConfig{Driver: huaweiActiveKind, Instance: "huawei-active.one", Enabled: true, Config: driver.Config{Instance: "huawei-active.one", Enabled: true}}
	rt, err := runtime.OpenHost(ctx, life.Installation{Key: "installation.huawei-active", Bridge: "bridge.test", StateDir: t.TempDir(), Clock: clock, Instances: []life.InstanceConfig{cfg}}, runtime.HostOptions{}, activeFactory{rec})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if err = rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err = rt.WaitInitial(ctx); err != nil {
		t.Fatal(err)
	}
	if rec.startErr != nil {
		status, snapErr := rt.Snapshot(life.Ref{Driver: huaweiActiveKind, Instance: "huawei-active.one"})
		t.Fatalf("family start: %v; snapshot %s %v (%v)", rec.startErr, status.ObservedState, status.Reason, snapErr)
	}
	return rt, rec.HuaweiActive
}

func capturedActive(t *testing.T, family *HuaweiActive) driver.SemanticCut {
	t.Helper()
	cut, err := family.session.SemanticInput().Capture(context.Background(), driver.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	return cut
}

func invokeActive(t *testing.T, family *HuaweiActive, args ...semreg.TypedField) driver.OperationResult {
	t.Helper()
	return invokeActivePath(t, family, "", args...)
}

func invokeActivePath(t *testing.T, family *HuaweiActive, label string, args ...semreg.TypedField) driver.OperationResult {
	t.Helper()
	cut := capturedActive(t, family)
	if len(cut.SettingRoutes) == 0 {
		t.Fatal("no route")
	}
	route := cut.SettingRoutes[0]
	if label != "" {
		found := false
		for _, item := range cut.SettingRoutes {
			if item.Owner.PathKey == activePath(label) {
				route, found = item, true
			}
		}
		if !found {
			t.Fatalf("path %s missing", label)
		}
	}
	snap := activeSnapshot(t, cut)
	var revision semreg.Uint64
	for _, capability := range snap.Capabilities {
		if capability.InstanceID == route.Route.CapabilityInstance {
			revision = capability.Revision
		}
	}
	if revision == "" {
		t.Fatalf("capability revision missing for %s", route.Route.CapabilityInstance)
	}
	result, err := family.session.ScopedOperations().Invoke(context.Background(), driver.OperationRequest{
		AssetID: snap.AssetID, BindingID: route.Route.BindingID, CapabilityInstanceID: route.Route.CapabilityInstance,
		Operation: pvDefinition(opID), Arguments: args, Preconditions: []operation.Precondition{}, Timeout: 5 * time.Second,
		ExpectedSemanticRevision: snap.Revisions.Semantic, ExpectedCapabilityRevision: snap.Revisions.Capabilities,
		ExpectedCapabilityInstanceRevision: revision, ExpectedDriverGeneration: route.Route.DriverGeneration,
		ExpectedSourceEpochID: route.Route.SourceEpochID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func activeSnapshot(t *testing.T, cut driver.SemanticCut) semreg.Snapshot {
	t.Helper()
	var found *semreg.Snapshot
	for i := range cut.Snapshots {
		for _, capability := range cut.Snapshots[i].Capabilities {
			if capability.Definition.ID != capDef || capability.AssetID != cut.Snapshots[i].AssetID {
				continue
			}
			if found != nil && found.AssetID != cut.Snapshots[i].AssetID {
				t.Fatal("limit capability on two assets")
			}
			found = &cut.Snapshots[i]
		}
	}
	if found == nil {
		t.Fatal("limit capability missing")
	}
	return *found
}

func activeDispatch(family *HuaweiActive, args ...semreg.TypedField) driver.Dispatch {
	own := family.owners[0]
	route := own.route.Route
	return driver.Dispatch{
		Request: driver.OperationRequest{Arguments: args, Timeout: time.Second},
		Route:   route,
		Guard: operation.GuardClaims{
			AssetID: own.asset, BindingID: route.BindingID, SourceID: route.SourceID, SourceEpochID: route.SourceEpochID,
			DriverGeneration: route.DriverGeneration, CapabilityInstance: route.CapabilityInstance, ServiceInstance: route.ServiceInstance,
		},
		AdmittedAt: wallNow(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)),
	}
}

func watts10000() *huawei.Watts {
	return &huawei.Watts{Coefficient: "1", Exponent10: 4}
}

func TestHuaweiActiveCapability(t *testing.T) {
	link := listenActive(t, "", 5000)
	_, family := openActive(t, link, []ActiveInverter{
		{Unit: 1, Label: "bound", Card: "MA-V200", Path: "S-Dongle"},
		{Unit: 2, Label: "unbound", Card: "COMMERCIAL", Path: "S-Dongle"},
	})
	cut := capturedActive(t, family)
	if len(cut.SettingRoutes) != 1 || cut.SettingRoutes[0].Owner.PathKey != activePath("bound") {
		t.Fatalf("routes %+v", cut.SettingRoutes)
	}
	if cut.SettingRoutes[0].Limits == nil || cut.SettingRoutes[0].Limits.Min == nil || cut.SettingRoutes[0].Limits.Max != nil || cut.SettingRoutes[0].Limits.Origin != driver.LimitSpecification {
		t.Fatalf("limits %#v", cut.SettingRoutes[0].Limits)
	}
	if cut.SettingRoutes[0].Limits.Min.Number.Coefficient != "0" || cut.SettingRoutes[0].Limits.Min.Number.Exponent10 != 0 {
		t.Fatalf("lower %#v", cut.SettingRoutes[0].Limits.Min)
	}
	snap := activeSnapshot(t, cut)
	caps := 0
	for _, capability := range snap.Capabilities {
		if capability.Definition.ID != capDef {
			continue
		}
		caps++
		if len(capability.Constraints) != 1 || capability.Constraints[0].Value.Quantity == nil {
			t.Fatalf("constraint %+v", capability.Constraints)
		}
		if capability.Constraints[0].Value.Quantity.Number.Coefficient != "5" || capability.Constraints[0].Value.Quantity.Number.Exponent10 != 3 {
			t.Fatalf("constraint %+v", capability.Constraints[0].Value.Quantity.Number)
		}
		seen := false
		for _, ev := range capability.ActivationEvidence {
			if ev.Kind == semreg.DefinitionID(VerificationLevel) {
				seen = true
			}
		}
		if !seen || VerificationLevel != catalog.VerificationNotTestedOnHardware {
			t.Fatalf("verification %q evidence %+v", VerificationLevel, capability.ActivationEvidence)
		}
	}
	if caps != 1 {
		t.Fatalf("capabilities %d", caps)
	}
	if len(link.peer.writtenFrames()) != 0 {
		t.Fatal("publication wrote")
	}
}

func TestHuaweiActiveUnboundHasNoRoute(t *testing.T) {
	link := listenActive(t, "", 5000)
	_, family := openActive(t, link, []ActiveInverter{{Unit: 1, Label: "commercial", Card: "COMMERCIAL", Path: "EMMA"}})
	cut := capturedActive(t, family)
	if len(cut.SettingRoutes) != 0 || len(family.owners) != 0 {
		t.Fatalf("routes %d owners %d", len(cut.SettingRoutes), len(family.owners))
	}
	for _, snap := range cut.Snapshots {
		for _, capability := range snap.Capabilities {
			if capability.Definition.ID == capDef {
				t.Fatal("unbound card published the capability")
			}
		}
	}
	if len(link.peer.writtenFrames()) != 0 || link.calls != 0 {
		t.Fatal("unbound card wrote")
	}
}

func TestHuaweiActiveSetConfirmsReadback(t *testing.T) {
	link := listenActive(t, "", 5000)
	_, family := openActive(t, link, []ActiveInverter{{Unit: 1, Label: "mb02", Card: "MB02", Path: "SmartLogger"}})
	cut := capturedActive(t, family)
	if len(cut.SettingRoutes) != 1 || cut.SettingRoutes[0].Limits != nil {
		t.Fatalf("MB02 limits %#v", cut.SettingRoutes[0].Limits)
	}
	result := invokeActive(t, family, wattsArg("5", 3))
	frames := link.peer.writtenFrames()
	if result.Status != "readback_confirmed" || result.KernelOutcome != operation.OutcomeApplied || result.ReadbackRelation != operation.ReadbackConfirms {
		t.Fatalf("result %+v", result)
	}
	if len(frames) != 1 || link.calls != 1 {
		t.Fatalf("writes %d calls %d", len(frames), link.calls)
	}
	addr, qty, words := wordsOf(t, frames[0])
	if addr != activeAddress || qty != 2 || len(words) != 2 || words[0] != 0x0000 || words[1] != 0x1388 {
		t.Fatalf("write %d x %d %v", addr, qty, words)
	}
}

func TestHuaweiActiveContradictingReadback(t *testing.T) {
	link := listenActive(t, "conflict", 5000)
	_, family := openActive(t, link, []ActiveInverter{{Unit: 1, Label: "ma", Card: "MA-V200", Path: "S-Dongle"}})
	result := invokeActive(t, family, wattsArg("5", 3))
	if result.Status != "failed" || result.KernelOutcome != operation.OutcomeConflict || result.ReadbackRelation != operation.ReadbackContradicts || result.Status == "readback_confirmed" {
		t.Fatalf("result %+v", result)
	}
	if len(link.peer.writtenFrames()) != 1 {
		t.Fatalf("writes %d", len(link.peer.writtenFrames()))
	}
	cut := capturedActive(t, family)
	found := false
	for _, snap := range cut.Snapshots {
		for _, env := range snap.Facts {
			for _, candidate := range env.Candidates {
				if candidate.Value == nil || candidate.Value.Quantity == nil || candidate.Key.FactID != argID {
					continue
				}
				number := candidate.Value.Quantity.Number
				if number.Coefficient == "4" && number.Exponent10 == 3 {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("later candidate missing")
	}
}

func TestHuaweiActiveExceptionIsNoEffect(t *testing.T) {
	link := listenActive(t, "exception", 5000)
	_, family := openActive(t, link, []ActiveInverter{{Unit: 1, Label: "ma", Card: "MC", Path: "EMMA"}})
	result := invokeActive(t, family, wattsArg("5", 3))
	if result.Status != "failed" || result.KernelOutcome != operation.OutcomeNoEffect || result.Delivery != operation.DeliverySent || result.Acknowledgement != operation.AckRejected {
		t.Fatalf("result %+v", result)
	}
	if result.ReadbackRelation == operation.ReadbackConfirms || result.Status == "readback_confirmed" {
		t.Fatalf("exception confirmed %+v", result)
	}
	if result.CompoundExecution == nil || len(result.CompoundExecution.Steps) != 1 || result.CompoundExecution.Steps[0].Reason != "exception_response:2" {
		t.Fatalf("cause %+v", result.CompoundExecution)
	}
	if len(link.peer.writtenFrames()) != 1 || link.calls != 1 {
		t.Fatalf("writes %d calls %d", len(link.peer.writtenFrames()), link.calls)
	}
}

func TestHuaweiActiveReadFailureIsProtocolAck(t *testing.T) {
	link := listenActive(t, "noread", 5000)
	_, family := openActive(t, link, []ActiveInverter{{Unit: 1, Label: "ma", Card: "MA-V200", Path: "S-Dongle"}})
	result := invokeActive(t, family, wattsArg("5", 3))
	if result.Status != "protocol_ack" || result.KernelOutcome != operation.OutcomeAcknowledgedUnverified || result.Status == "readback_confirmed" {
		t.Fatalf("result %+v", result)
	}
	if len(link.peer.writtenFrames()) != 1 {
		t.Fatalf("writes %d", len(link.peer.writtenFrames()))
	}
}

func TestHuaweiActiveLostResponseIsUnknown(t *testing.T) {
	link := listenActive(t, "drop", 5000)
	_, family := openActive(t, link, []ActiveInverter{{Unit: 1, Label: "ma", Card: "MA-V200", Path: "S-Dongle"}})
	result := invokeActive(t, family, wattsArg("5", 3))
	if result.Status != "unknown" || result.KernelOutcome != operation.OutcomeIndeterminate {
		t.Fatalf("result %+v", result)
	}
	if len(link.peer.writtenFrames()) != 1 || link.calls != 1 {
		t.Fatalf("writes %d calls %d", len(link.peer.writtenFrames()), link.calls)
	}
}

func TestHuaweiActiveBelowZeroSendsNothing(t *testing.T) {
	for _, card := range []string{"MA-V200", "MC"} {
		t.Run(card, func(t *testing.T) {
			link := listenActive(t, "", 5000)
			_, family := openActive(t, link, []ActiveInverter{{Unit: 1, Label: "ma", Card: card, Path: "S-Dongle"}})
			result := invokeActive(t, family, wattsArg("-1", 0))
			if result.Reason != "semantic.operation.reason.negative_set" || result.Delivery != operation.DeliveryNotSent || link.calls != 0 || len(link.peer.writtenFrames()) != 0 {
				t.Fatalf("%s %+v calls %d", card, result, link.calls)
			}
		})
	}
}

func TestHuaweiActiveGuardChangeSendsNothing(t *testing.T) {
	link := listenActive(t, "", 5000)
	_, family := openActive(t, link, []ActiveInverter{{Unit: 1, Label: "ma", Card: "MA-V200", Path: "S-Dongle"}})
	dispatch := activeDispatch(family, wattsArg("5", 3))
	dispatch.Guard.DriverGeneration = "99"
	completion, err := family.Execute(context.Background(), dispatch)
	if err != nil || completion.Outcome != operation.OutcomeFailedNoContact || completion.Dispatch == nil || completion.Dispatch.Delivery != operation.DeliveryNotSent {
		t.Fatalf("completion %+v %v", completion, err)
	}
	if completion.CompoundExecution == nil || completion.CompoundExecution.Steps[0].Reason != "guard_mismatch" {
		t.Fatalf("reason %+v", completion.CompoundExecution)
	}
	if link.calls != 0 || len(link.peer.writtenFrames()) != 0 {
		t.Fatalf("writes %d calls %d", len(link.peer.writtenFrames()), link.calls)
	}
}

func TestHuaweiActiveReleaseUnknownSendsNothing(t *testing.T) {
	for _, card := range []string{"MA-V200", "MC", "MB02"} {
		t.Run(card, func(t *testing.T) {
			link := listenActive(t, "", 5000)
			_, family := openActive(t, link, []ActiveInverter{{Unit: 1, Label: "ma", Card: card, Path: "SmartLogger"}})
			result := invokeActive(t, family, boolArg(false))
			if result.Status == "readback_confirmed" || result.KernelOutcome == operation.OutcomeApplied {
				t.Fatalf("%s confirmed %+v", card, result)
			}
			if result.Delivery != operation.DeliveryNotSent || result.Reason != "semantic.operation.reason.no_contact" {
				t.Fatalf("%s %+v", card, result)
			}
			if result.CompoundExecution == nil || result.CompoundExecution.Steps[0].Reason != "release_unknown" {
				t.Fatalf("%s cause %+v", card, result.CompoundExecution)
			}
			if link.calls != 0 || len(link.peer.writtenFrames()) != 0 {
				t.Fatalf("%s writes %d", card, link.calls)
			}
			assertNoEnabledFact(t, family)
		})
	}
}

func TestHuaweiActiveQueriedReleaseAndSet(t *testing.T) {
	queried := watts10000()
	link := listenActive(t, "", 5000)
	_, family := openActive(t, link, []ActiveInverter{{Unit: 1, Label: "ma", Card: "MA-V200", Path: "S-Dongle", QueriedMaximum: queried}})
	cut := capturedActive(t, family)
	if cut.SettingRoutes[0].Limits == nil || cut.SettingRoutes[0].Limits.Max == nil || cut.SettingRoutes[0].Limits.Origin != driver.LimitDeviceQuery {
		t.Fatalf("queried limits %#v", cut.SettingRoutes[0].Limits)
	}
	if cut.SettingRoutes[0].Limits.Max.Number.Coefficient != "1" || cut.SettingRoutes[0].Limits.Max.Number.Exponent10 != 4 {
		t.Fatalf("max %#v", cut.SettingRoutes[0].Limits.Max)
	}
	if cut.SettingRoutes[0].Limits.Min == nil || cut.SettingRoutes[0].Limits.Min.Number.Coefficient != "0" {
		t.Fatalf("min %#v", cut.SettingRoutes[0].Limits.Min)
	}
	released := invokeActive(t, family, boolArg(false))
	frames := link.peer.writtenFrames()
	if released.Status != "protocol_ack" || released.KernelOutcome != operation.OutcomeAcknowledgedUnverified || released.Status == "readback_confirmed" {
		t.Fatalf("release %+v", released)
	}
	if len(frames) != 1 {
		t.Fatalf("release writes %d", len(frames))
	}
	addr, qty, words := wordsOf(t, frames[0])
	if addr != activeAddress || qty != 2 || words[0] != 0x0000 || words[1] != 0x2710 {
		t.Fatalf("release write %d x %d %v", addr, qty, words)
	}
	assertNoEnabledFact(t, family)

	setLink := listenActive(t, "", 10000)
	_, setFamily := openActive(t, setLink, []ActiveInverter{{Unit: 1, Label: "ma", Card: "MA-V200", Path: "EMMA", QueriedMaximum: queried}})
	set := invokeActive(t, setFamily, wattsArg("1", 4))
	if set.Status != "readback_confirmed" || set.KernelOutcome != operation.OutcomeApplied {
		t.Fatalf("set %+v", set)
	}
	setFrames := setLink.peer.writtenFrames()
	if len(setFrames) != 1 {
		t.Fatalf("set writes %d", len(setFrames))
	}
	addr, qty, words = wordsOf(t, setFrames[0])
	if addr != activeAddress || qty != 2 || words[0] != 0x0000 || words[1] != 0x2710 {
		t.Fatalf("set write %d x %d %v", addr, qty, words)
	}
}

func TestHuaweiActiveRefusalsSendNothing(t *testing.T) {
	link := listenActive(t, "", 5000)
	_, family := openActive(t, link, []ActiveInverter{{Unit: 1, Label: "ma", Card: "MA-V200", Path: "S-Dongle"}})
	other := semreg.TypedField{ID: "pv.limit.other", Value: wattsArg("5", 3).Value}
	cases := []struct {
		name string
		args []semreg.TypedField
		want string
	}{
		{"true", []semreg.TypedField{boolArg(true)}, "semantic.operation.reason.release_true"},
		{"mixed", []semreg.TypedField{boolArg(false), other}, "semantic.operation.reason.mixed_release"},
		{"negative", []semreg.TypedField{wattsArg("-5", 0)}, "semantic.operation.reason.negative_set"},
		{"fraction", []semreg.TypedField{wattsArg("1", -1)}, "semantic.operation.reason.admission:"},
		{"overflow", []semreg.TypedField{wattsArg("4294967296", 0)}, "semantic.operation.reason.admission:"},
		{"encoder-limit", []semreg.TypedField{wattsArg("4294967295", 0)}, "semantic.operation.reason.admission:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := invokeActive(t, family, tc.args...)
			if result.Delivery != operation.DeliveryNotSent || link.calls != 0 || len(link.peer.writtenFrames()) != 0 {
				t.Fatalf("%s %+v calls %d", tc.name, result, link.calls)
			}
			if tc.want != "" && !hasPrefix(result.Reason, tc.want) {
				t.Fatalf("%s reason %q", tc.name, result.Reason)
			}
		})
	}
	dispatch := activeDispatch(family, wattsArg("1", -1))
	completion, err := family.Execute(context.Background(), dispatch)
	if err != nil || completion.Outcome != operation.OutcomeFailedNoContact || completion.CompoundExecution == nil || completion.CompoundExecution.Steps[0].Reason != "unrepresentable" {
		t.Fatalf("fraction completion %+v %v", completion, err)
	}
	if link.calls != 0 || len(link.peer.writtenFrames()) != 0 {
		t.Fatal("fraction wrote")
	}
	if report := family.Stop(context.Background()); !report.Confirmed {
		t.Fatal("stop was not confirmed")
	}
	result := invokeActive(t, family, wattsArg("5", 3))
	if result.Reason != "semantic.operation.reason.executor_missing" || link.calls != 0 || len(link.peer.writtenFrames()) != 0 {
		t.Fatalf("missing owner %+v calls %d", result, link.calls)
	}
}

func TestHuaweiActiveMCLowerBound(t *testing.T) {
	link := listenActive(t, "", 5000)
	_, family := openActive(t, link, []ActiveInverter{{Unit: 1, Label: "mc", Card: "MC", Path: "S-Dongle"}})
	cut := capturedActive(t, family)
	limits := cut.SettingRoutes[0].Limits
	if limits == nil || limits.Min == nil || limits.Max != nil || limits.Min.Number.Coefficient != "0" || limits.Origin != driver.LimitSpecification {
		t.Fatalf("MC limits %#v", limits)
	}
	result := invokeActive(t, family, wattsArg("5", 3))
	if result.Status != "readback_confirmed" || len(link.peer.writtenFrames()) != 1 {
		t.Fatalf("MC set %+v writes %d", result, len(link.peer.writtenFrames()))
	}
}

func assertNoEnabledFact(t *testing.T, family *HuaweiActive) {
	t.Helper()
	cut := capturedActive(t, family)
	for _, snap := range cut.Snapshots {
		for _, env := range snap.Facts {
			for _, candidate := range env.Candidates {
				if candidate.Key.FactID == "pv.limit.active_power_enabled" {
					t.Fatal("enabled state was published")
				}
			}
		}
	}
}

func hasPrefix(value, prefix string) bool {
	return len(value) >= len(prefix) && value[:len(prefix)] == prefix
}
