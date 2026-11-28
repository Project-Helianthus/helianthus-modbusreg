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
	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime/life"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/catalog"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

const huaweiExportKind driver.DriverKind = "modbus.limits.huawei.export"

const (
	exportModeAddress  = 40100
	exportScopeAddress = 40101
	exportPowerAddress = 40107
	exportPlantMode    = 47415
	exportPlantPower   = 47416
	exportOutsidePlant = 47419
)

type exportPeer struct {
	mu      sync.Mutex
	units   map[byte][]uint16
	frames  [][]byte
	reads   []uint16
	mode    string
	wrote   bool
	modeAt  uint16
	valueAt uint16
	scopeAt int
}

func (p *exportPeer) writtenFrames() [][]byte {
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

func (p *exportPeer) readList() []uint16 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]uint16(nil), p.reads...)
}

func (p *exportPeer) cell(unit byte, address uint16) uint16 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.units[unit][address]
}

func (p *exportPeer) serve(conn net.Conn) error {
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

func (p *exportPeer) answer(unit byte, frame []byte) ([]byte, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.frames = append(p.frames, append([]byte(nil), frame...))
	request := frame[7:]
	cells := p.units[unit]
	switch request[0] {
	case 3:
		return p.read(cells, request)
	case 16:
		return p.write(cells, request)
	default:
		return nil, false, errors.New("synthetic function")
	}
}

func (p *exportPeer) read(cells []uint16, request []byte) ([]byte, bool, error) {
	addr, qty := binary.BigEndian.Uint16(request[1:3]), binary.BigEndian.Uint16(request[3:5])
	p.reads = append(p.reads, addr)
	if p.mode == "noread" && p.wrote {
		return nil, true, errors.New("synthetic read dropped")
	}
	if p.mode == "scope-drop" && p.wrote && p.scopeAt >= 0 && addr == uint16(p.scopeAt) {
		return nil, true, errors.New("synthetic scope dropped")
	}
	if cells == nil || int(addr)+int(qty) > len(cells) {
		return nil, false, errors.New("synthetic read outside image")
	}
	body := []byte{3, byte(qty * 2)}
	for i := uint16(0); i < qty; i++ {
		body = binary.BigEndian.AppendUint16(body, p.word(addr, i, cells[addr+i]))
	}
	return body, false, nil
}

func (p *exportPeer) word(addr, index, stored uint16) uint16 {
	switch {
	case p.mode == "conflict" && p.wrote && addr == p.valueAt && index == 0:
		return 0x0000
	case p.mode == "conflict" && p.wrote && addr == p.valueAt && index == 1:
		return 0x0FA0
	case p.mode == "mode" && p.wrote && addr == p.modeAt:
		return 0
	case p.mode == "scope-phase" && p.wrote && p.scopeAt >= 0 && addr == uint16(p.scopeAt):
		return 1
	case p.mode == "scope-other" && p.wrote && p.scopeAt >= 0 && addr == uint16(p.scopeAt):
		return 2
	default:
		return stored
	}
}

func (p *exportPeer) write(cells []uint16, request []byte) ([]byte, bool, error) {
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
}

type exportLink struct {
	mu    sync.Mutex
	conn  net.Conn
	calls int
	mode  string
	txn   uint16
}

func (s *exportLink) ReadHolding(ctx context.Context, unit uint8, address, quantity uint16) ([]uint16, error) {
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

func (s *exportLink) WriteMultiple(ctx context.Context, unit uint8, address uint16, registers []uint16, admission modbus.RegisterWriteAdmission) (modbus.WriteMultipleRegistersAcknowledgement, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()
	if s.mode == "second-fail" && call >= 2 {
		return modbus.WriteMultipleRegistersAcknowledgement{}, errors.New("synthetic value write refused before send")
	}
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

func (s *exportLink) exchange(ctx context.Context, unit uint8, pdu []byte) ([]byte, error) {
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

func emmaImage(scope uint16, watts uint32) []uint16 {
	cells := make([]uint16, exportPowerAddress+3)
	cells[exportScopeAddress] = scope
	cells[exportPowerAddress] = uint16(watts >> 16)
	cells[exportPowerAddress+1] = uint16(watts)
	return cells
}

func plantImage(watts uint32) []uint16 {
	cells := make([]uint16, exportOutsidePlant)
	cells[exportPlantPower] = uint16(watts >> 16)
	cells[exportPlantPower+1] = uint16(watts)
	return cells
}

func listenExport(t *testing.T, mode string, units map[byte][]uint16, modeAt, valueAt uint16, scopeAt int) (*exportLink, *exportPeer) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	peer := &exportPeer{units: units, mode: mode, modeAt: modeAt, valueAt: valueAt, scopeAt: scopeAt}
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
	return &exportLink{conn: client, mode: mode}, peer
}

type exportRecording struct {
	*HuaweiExport
	startErr error
}

func (r *exportRecording) Start(ctx context.Context, s driver.Session) error {
	r.startErr = r.HuaweiExport.Start(ctx, s)
	return r.startErr
}

type exportFactory struct{ family *exportRecording }

func (exportFactory) Descriptor() driver.Descriptor { return driver.Descriptor{} }
func (exportFactory) Plan(c driver.Config) ([]driver.InstancePlan, error) {
	return []driver.InstancePlan{{Driver: huaweiExportKind, Instance: driver.InstanceKey(c.Instance)}}, nil
}
func (f exportFactory) Construct(context.Context, driver.InstancePlan) (driver.Instance, error) {
	return f.family, nil
}

func openExport(t *testing.T, access Registers, systems []ExportSystem) *HuaweiExport {
	t.Helper()
	rec := &exportRecording{HuaweiExport: &HuaweiExport{Access: access, Systems: systems}}
	ctx := context.Background()
	clock := &limitClock{now: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}
	cfg := life.InstanceConfig{Driver: huaweiExportKind, Instance: "huawei-export.one", Enabled: true, Config: driver.Config{Instance: "huawei-export.one", Enabled: true}}
	rt, err := runtime.OpenHost(ctx, life.Installation{Key: "installation.huawei-export", Bridge: "bridge.test", StateDir: t.TempDir(), Clock: clock, Instances: []life.InstanceConfig{cfg}}, runtime.HostOptions{}, exportFactory{rec})
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
		status, snapErr := rt.Snapshot(life.Ref{Driver: huaweiExportKind, Instance: "huawei-export.one"})
		t.Fatalf("family start: %v; snapshot %s %v (%v)", rec.startErr, status.ObservedState, status.Reason, snapErr)
	}
	return rec.HuaweiExport
}

func capturedExport(t *testing.T, family *HuaweiExport) driver.SemanticCut {
	t.Helper()
	cut, err := family.session.SemanticInput().Capture(context.Background(), driver.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	return cut
}

func exportSnapshot(t *testing.T, cut driver.SemanticCut) semreg.Snapshot {
	t.Helper()
	var found *semreg.Snapshot
	for i := range cut.Snapshots {
		for _, capability := range cut.Snapshots[i].Capabilities {
			if capability.Definition.ID != exportCap || capability.AssetID != cut.Snapshots[i].AssetID {
				continue
			}
			if found != nil && found.AssetID != cut.Snapshots[i].AssetID {
				t.Fatal("export capability on two assets")
			}
			found = &cut.Snapshots[i]
		}
	}
	if found == nil {
		t.Fatal("export capability missing")
	}
	return *found
}

func invokeExport(t *testing.T, family *HuaweiExport, tweak func(*driver.OperationRequest), args ...semreg.TypedField) driver.OperationResult {
	t.Helper()
	cut := capturedExport(t, family)
	if len(cut.SettingRoutes) == 0 {
		t.Fatal("no route")
	}
	route := cut.SettingRoutes[0]
	snap := exportSnapshot(t, cut)
	var revision semreg.Uint64
	for _, capability := range snap.Capabilities {
		if capability.InstanceID == route.Route.CapabilityInstance {
			revision = capability.Revision
		}
	}
	if revision == "" {
		t.Fatalf("capability revision missing for %s", route.Route.CapabilityInstance)
	}
	request := driver.OperationRequest{
		AssetID: snap.AssetID, BindingID: route.Route.BindingID, CapabilityInstanceID: route.Route.CapabilityInstance,
		Operation: pvDefinition(exportOp), Arguments: args, Preconditions: []operation.Precondition{}, Timeout: 5 * time.Second,
		ExpectedSemanticRevision: snap.Revisions.Semantic, ExpectedCapabilityRevision: snap.Revisions.Capabilities,
		ExpectedCapabilityInstanceRevision: revision, ExpectedDriverGeneration: route.Route.DriverGeneration,
		ExpectedSourceEpochID: route.Route.SourceEpochID,
	}
	if tweak != nil {
		tweak(&request)
	}
	result, err := family.session.ScopedOperations().Invoke(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func exportWattsArg(coefficient string, exponent int32) semreg.TypedField {
	quantity := semreg.Quantity{Number: semreg.Decimal{Coefficient: coefficient, Exponent10: exponent}, Unit: wattUnit}
	return semreg.TypedField{ID: exportArg, Value: semreg.Value{Kind: semreg.ValueQuantity, Quantity: &quantity}}
}

func assertExportStatus(t *testing.T, result driver.OperationResult, status string, outcome operation.Outcome, delivery operation.DeliveryState) {
	t.Helper()
	if string(result.Status) != status || result.KernelOutcome != outcome || result.Delivery != delivery || result.Status == "readback_confirmed" && status != "readback_confirmed" {
		t.Fatalf("result %+v", result)
	}
}

func assertNoExportWrites(t *testing.T, link *exportLink, peer *exportPeer) {
	t.Helper()
	if link.calls != 0 || len(peer.writtenFrames()) != 0 {
		t.Fatalf("calls %d frames %d", link.calls, len(peer.writtenFrames()))
	}
}

func assertScopeUnwritten(t *testing.T, peer *exportPeer) {
	t.Helper()
	for _, frame := range peer.writtenFrames() {
		addr, _, _ := wordsOf(t, frame)
		if addr == exportScopeAddress {
			t.Fatal("scope row was written")
		}
	}
}

func assertFreshExport(t *testing.T, family *HuaweiExport, coefficient string, exponent int32) {
	t.Helper()
	cut := capturedExport(t, family)
	if len(cut.SettingRoutes) != 1 {
		t.Fatalf("routes %d", len(cut.SettingRoutes))
	}
	route := cut.SettingRoutes[0].Route
	found := false
	for _, snap := range cut.Snapshots {
		for _, env := range snap.Facts {
			for _, candidate := range env.Candidates {
				if candidate.Key.FactID != exportArg || candidate.Value == nil || candidate.Value.Quantity == nil {
					continue
				}
				number := candidate.Value.Quantity.Number
				if number.Coefficient != coefficient || number.Exponent10 != exponent {
					t.Fatalf("candidate %+v", number)
				}
				if candidate.Quality.Assertion != semreg.AssertionObserved || candidate.BindingID == nil || *candidate.BindingID != route.BindingID {
					t.Fatalf("candidate binding %+v", candidate)
				}
				if candidate.SourceEpochID == nil || *candidate.SourceEpochID != route.SourceEpochID || candidate.DriverGeneration == nil || *candidate.DriverGeneration != route.DriverGeneration {
					t.Fatalf("candidate source %+v", candidate)
				}
				if candidate.Times.ReceiptMonotonic.ClockEpochID == "" || candidate.Times.ReceivedAt.ClockID == "" {
					t.Fatalf("candidate receipt %+v", candidate.Times)
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatal("observed export candidate missing")
	}
}

func assertNoExportFact(t *testing.T, family *HuaweiExport) {
	t.Helper()
	cut := capturedExport(t, family)
	for _, snap := range cut.Snapshots {
		for _, env := range snap.Facts {
			for _, candidate := range env.Candidates {
				if candidate.Key.FactID == exportArg {
					t.Fatalf("export fact published %+v", candidate.Value)
				}
			}
		}
	}
}

func TestHuaweiExportCapability(t *testing.T) {
	link, peer := listenExport(t, "", map[byte][]uint16{1: emmaImage(0, 5000)}, exportModeAddress, exportPowerAddress, exportScopeAddress)
	family := openExport(t, link, []ExportSystem{
		{Unit: 1, Label: "plant", Card: "emma-v100r025c00spc102-2025-08-14", Path: "EMMA"},
		{Unit: 2, Label: "other", Card: "COMMERCIAL", Path: "S-Dongle"},
	})
	cut := capturedExport(t, family)
	if len(cut.SettingRoutes) != 1 || cut.SettingRoutes[0].Limits != nil || cut.SettingRoutes[0].Owner.PathKey != exportPath("plant") {
		t.Fatalf("routes %+v", cut.SettingRoutes)
	}
	snap := exportSnapshot(t, cut)
	caps, systems, kind := 0, 0, false
	for _, capability := range snap.Capabilities {
		if capability.Definition.ID != exportCap {
			continue
		}
		caps++
		if len(capability.Constraints) != 1 || capability.Constraints[0].ID != exportArg || capability.Constraints[0].Value.Quantity == nil {
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
	for _, service := range snap.Services {
		if service.Definition.ID == exportService {
			systems++
		}
	}
	for _, resource := range snap.Resources {
		if resource.Kind == exportKind {
			kind = true
		}
	}
	if caps != 1 || systems != 1 || !kind {
		t.Fatalf("capabilities %d services %d kind %v", caps, systems, kind)
	}
	if !sameAddresses(peer.readList(), []uint16{exportPowerAddress}) || len(peer.writtenFrames()) != 0 {
		t.Fatalf("start reads %v", peer.readList())
	}
}

func TestHuaweiExportUnboundHasNoRoute(t *testing.T) {
	link, peer := listenExport(t, "", map[byte][]uint16{1: plantImage(0)}, exportPlantMode, exportPlantPower, -1)
	family := openExport(t, link, []ExportSystem{{Unit: 1, Label: "commercial", Card: "COMMERCIAL", Path: "EMMA"}})
	cut := capturedExport(t, family)
	if len(cut.SettingRoutes) != 0 || len(family.owners) != 0 {
		t.Fatalf("routes %d owners %d", len(cut.SettingRoutes), len(family.owners))
	}
	for _, snap := range cut.Snapshots {
		for _, capability := range snap.Capabilities {
			if capability.Definition.ID == exportCap {
				t.Fatal("unbound card published the capability")
			}
		}
	}
	assertNoExportWrites(t, link, peer)
	if len(peer.readList()) != 0 {
		t.Fatalf("unbound card read %v", peer.readList())
	}
}

func TestHuaweiExportEmmaConfirmsTotal(t *testing.T) {
	link, peer := listenExport(t, "", map[byte][]uint16{1: emmaImage(0, 5000)}, exportModeAddress, exportPowerAddress, exportScopeAddress)
	family := openExport(t, link, []ExportSystem{{Unit: 1, Label: "plant", Card: "emma-v100r024c00-01-2024-07-15", Path: "SmartLogger"}})
	result := invokeExport(t, family, nil, exportWattsArg("5", 3))
	assertExportStatus(t, result, "readback_confirmed", operation.OutcomeApplied, operation.DeliverySent)
	if result.ReadbackRelation != operation.ReadbackConfirms {
		t.Fatalf("relation %s", result.ReadbackRelation)
	}
	frames := peer.writtenFrames()
	if len(frames) != 2 || link.calls != 2 {
		t.Fatalf("writes %d calls %d", len(frames), link.calls)
	}
	modeAddr, modeQty, modeWords := wordsOf(t, frames[0])
	valueAddr, valueQty, valueWords := wordsOf(t, frames[1])
	if modeAddr != exportModeAddress || modeQty != 1 || len(modeWords) != 1 || modeWords[0] != 6 {
		t.Fatalf("mode %d x %d %v", modeAddr, modeQty, modeWords)
	}
	if valueAddr != exportPowerAddress || valueQty != 2 || len(valueWords) != 2 || valueWords[0] != 0x0000 || valueWords[1] != 0x1388 {
		t.Fatalf("value %d x %d %v", valueAddr, valueQty, valueWords)
	}
	assertScopeUnwritten(t, peer)
	if peer.cell(1, exportScopeAddress) != 0 {
		t.Fatal("scope changed")
	}
	want := []uint16{exportPowerAddress, exportScopeAddress, exportModeAddress, exportPowerAddress, exportScopeAddress}
	if !sameAddresses(peer.readList(), want) {
		t.Fatalf("reads %v", peer.readList())
	}
	assertFreshExport(t, family, "5", 3)
}

func TestHuaweiExportInverterConfirms(t *testing.T) {
	for _, card := range []string{"MA-V200", "MC"} {
		t.Run(card, func(t *testing.T) {
			link, peer := listenExport(t, "", map[byte][]uint16{1: plantImage(5000)}, exportPlantMode, exportPlantPower, -1)
			family := openExport(t, link, []ExportSystem{{Unit: 1, Label: "inverter", Card: card, Path: "S-Dongle"}})
			cut := capturedExport(t, family)
			limits := cut.SettingRoutes[0].Limits
			if limits == nil || limits.Min == nil || limits.Max != nil || limits.Origin != driver.LimitSpecification {
				t.Fatalf("limits %#v", limits)
			}
			if limits.Min.Number.Coefficient != "-1" || limits.Min.Number.Exponent10 != 6 {
				t.Fatalf("lower %#v", limits.Min.Number)
			}
			result := invokeExport(t, family, nil, exportWattsArg("5", 3))
			assertExportStatus(t, result, "readback_confirmed", operation.OutcomeApplied, operation.DeliverySent)
			frames := peer.writtenFrames()
			if len(frames) != 2 || link.calls != 2 {
				t.Fatalf("writes %d calls %d", len(frames), link.calls)
			}
			modeAddr, modeQty, modeWords := wordsOf(t, frames[0])
			valueAddr, valueQty, valueWords := wordsOf(t, frames[1])
			if modeAddr != exportPlantMode || modeQty != 1 || modeWords[0] != 6 || valueAddr != exportPlantPower || valueQty != 2 || valueWords[0] != 0x0000 || valueWords[1] != 0x1388 {
				t.Fatalf("mode %d %v value %d %v", modeAddr, modeWords, valueAddr, valueWords)
			}
			want := []uint16{exportPlantPower, exportPlantMode, exportPlantPower}
			if !sameAddresses(peer.readList(), want) {
				t.Fatalf("reads %v", peer.readList())
			}
			for _, addr := range peer.readList() {
				if addr == exportOutsidePlant || addr == exportScopeAddress {
					t.Fatalf("scope read %d", addr)
				}
			}
			assertFreshExport(t, family, "5", 3)
		})
	}
}

func TestHuaweiExportValueMismatch(t *testing.T) {
	for _, card := range []string{"MA-V200", "MC"} {
		t.Run(card, func(t *testing.T) {
			link, peer := listenExport(t, "conflict", map[byte][]uint16{1: plantImage(5000)}, exportPlantMode, exportPlantPower, -1)
			family := openExport(t, link, []ExportSystem{{Unit: 1, Label: "inverter", Card: card, Path: "EMMA"}})
			result := invokeExport(t, family, nil, exportWattsArg("5", 3))
			assertExportStatus(t, result, "failed", operation.OutcomeConflict, operation.DeliverySent)
			if result.ReadbackRelation != operation.ReadbackContradicts || result.Status == "readback_confirmed" {
				t.Fatalf("result %+v", result)
			}
			if len(peer.writtenFrames()) != 2 || link.calls != 2 {
				t.Fatalf("writes %d calls %d", len(peer.writtenFrames()), link.calls)
			}
			assertFreshExport(t, family, "4", 3)
		})
	}
}

func TestHuaweiExportModeMismatch(t *testing.T) {
	link, peer := listenExport(t, "mode", map[byte][]uint16{1: plantImage(5000)}, exportPlantMode, exportPlantPower, -1)
	family := openExport(t, link, []ExportSystem{{Unit: 1, Label: "inverter", Card: "MA-V200", Path: "S-Dongle"}})
	result := invokeExport(t, family, nil, exportWattsArg("5", 3))
	// The power row equals the request, so no second watts reading exists and no
	// pv.limit.export_power candidate is stored. The contradicting mode read is
	// still conflict.
	assertExportStatus(t, result, "failed", operation.OutcomeConflict, operation.DeliverySent)
	if result.ReadbackRelation != operation.ReadbackContradicts || result.Status == "readback_confirmed" || result.Status == "unknown" || result.KernelOutcome == operation.OutcomeIndeterminate || result.KernelOutcome == operation.OutcomeApplied {
		t.Fatalf("result %+v", result)
	}
	if len(peer.writtenFrames()) != 2 {
		t.Fatalf("writes %d", len(peer.writtenFrames()))
	}
	assertNoExportFact(t, family)
}

func TestHuaweiExportScopeOutsidePairAfterSend(t *testing.T) {
	link, peer := listenExport(t, "scope-other", map[byte][]uint16{1: emmaImage(0, 5000)}, exportModeAddress, exportPowerAddress, exportScopeAddress)
	family := openExport(t, link, []ExportSystem{{Unit: 1, Label: "plant", Card: "emma-v100r024c00-01-2024-07-15", Path: "EMMA"}})
	result := invokeExport(t, family, nil, exportWattsArg("5", 3))
	// Mode and watts match. Scope word 2 is neither total nor individual phase,
	// so the reread is not a contradiction and confirms nothing.
	assertExportStatus(t, result, "protocol_ack", operation.OutcomeAcknowledgedUnverified, operation.DeliverySent)
	if result.ReadbackRelation != operation.ReadbackInconclusive || result.KernelOutcome == operation.OutcomeConflict || result.Status == "failed" || result.Status == "unknown" {
		t.Fatalf("result %+v", result)
	}
	frames := peer.writtenFrames()
	if len(frames) != 2 || link.calls != 2 {
		t.Fatalf("writes %d calls %d", len(frames), link.calls)
	}
	assertScopeUnwritten(t, peer)
	assertNoExportFact(t, family)
}

type exportCommitFailSession struct {
	driver.Session
	armed  bool
	stage  bool
	cancel context.CancelFunc
}

func (s *exportCommitFailSession) Publisher() driver.Publisher {
	return exportCommitFailPublisher{Publisher: s.Session.Publisher(), armed: &s.armed, stage: s.stage, cancel: s.cancel}
}

type exportCommitFailPublisher struct {
	driver.Publisher
	armed  *bool
	stage  bool
	cancel context.CancelFunc
}

func (p exportCommitFailPublisher) Begin(ctx context.Context) (driver.PublicationTransaction, error) {
	tx, err := p.Publisher.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return exportCommitFailTx{PublicationTransaction: tx, armed: p.armed, stage: p.stage, cancel: p.cancel}, nil
}

type exportCommitFailTx struct {
	driver.PublicationTransaction
	armed  *bool
	stage  bool
	cancel context.CancelFunc
}

// Stage cancels the operation and refuses the contradiction batch. It does not
// abort, so a caller that leaves the transaction open is visible to the next Begin.
func (t exportCommitFailTx) Stage(batch semreg.PublicationBatch) error {
	if t.stage && t.armed != nil && *t.armed {
		*t.armed = false
		if t.cancel != nil {
			t.cancel()
		}
		return errors.New("huawei export contradiction stage")
	}
	return t.PublicationTransaction.Stage(batch)
}

// Commit cancels the operation after the contradiction batch is staged and
// then calls the real commit. It does not abort, so a caller that leaves the
// transaction open is visible to the next Begin.
func (t exportCommitFailTx) Commit(ctx context.Context) error {
	if !t.stage && t.armed != nil && *t.armed {
		*t.armed = false
		if t.cancel != nil {
			t.cancel()
		}
		return t.PublicationTransaction.Commit(ctx)
	}
	return t.PublicationTransaction.Commit(ctx)
}

func exportDispatch(family *HuaweiExport, args ...semreg.TypedField) driver.Dispatch {
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

func TestHuaweiExportModeMismatchSnapshotCommit(t *testing.T) {
	for _, stage := range []bool{false, true} {
		name := "commit"
		if stage {
			name = "stage"
		}
		t.Run(name, func(t *testing.T) {
			testHuaweiExportModeMismatchSnapshotRelease(t, stage)
		})
	}
}

func testHuaweiExportModeMismatchSnapshotRelease(t *testing.T, stage bool) {
	t.Helper()
	link, peer := listenExport(t, "mode", map[byte][]uint16{1: plantImage(5000)}, exportPlantMode, exportPlantPower, -1)
	family := openExport(t, link, []ExportSystem{{Unit: 1, Label: "inverter", Card: "MA-V200", Path: "S-Dongle"}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	family.session = &exportCommitFailSession{Session: family.session, armed: true, stage: stage, cancel: cancel}
	completion, err := family.Execute(ctx, exportDispatch(family, exportWattsArg("5", 3)))
	if err == nil {
		t.Fatal("release failure returned no error")
	}
	fresh, beginErr := family.session.Publisher().Begin(context.Background())
	if beginErr != nil {
		t.Fatalf("begin after canceled release: %v", beginErr)
	}
	if abortErr := fresh.Abort(context.Background()); abortErr != nil {
		t.Fatal(abortErr)
	}
	if completion.Outcome != operation.OutcomeIndeterminate || completion.Acknowledgement != nil || completion.Outcome == operation.OutcomeAcknowledgedUnverified {
		t.Fatalf("completion %+v", completion)
	}
	if completion.Dispatch == nil || completion.Dispatch.Delivery != operation.DeliverySent {
		t.Fatalf("dispatch %+v", completion.Dispatch)
	}
	want := Evidence(operation.ContractOperationV1, kindRead, "mode=mismatch value=match")
	if completion.Readback == nil || completion.Readback.Relation != operation.ReadbackContradicts || completion.Readback.CandidateID != "" || completion.Readback.CandidateRevision != "" || len(completion.Readback.Evidence) != 1 || completion.Readback.Evidence[0] != want {
		t.Fatalf("readback %+v", completion.Readback)
	}
	found := false
	if completion.CompoundExecution != nil {
		for _, step := range completion.CompoundExecution.Steps {
			if step.Point == "readback" && step.ReadbackRelation == operation.ReadbackContradicts {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("steps %+v", completion.CompoundExecution)
	}
	if len(peer.writtenFrames()) != 2 || link.calls != 2 {
		t.Fatalf("writes %d calls %d", len(peer.writtenFrames()), link.calls)
	}
	assertNoExportFact(t, family)
}

func TestHuaweiExportModeMismatchPublicationThroughRouter(t *testing.T) {
	for _, stage := range []bool{false, true} {
		name := "commit"
		if stage {
			name = "stage"
		}
		t.Run(name, func(t *testing.T) {
			testHuaweiExportModeMismatchPublicationThroughRouter(t, stage)
		})
	}
}

// testHuaweiExportModeMismatchPublicationThroughRouter dispatches through the
// pinned router. The mode reread contradicts and the power row matches, so the
// contradiction has no candidate. Publication of the later snapshot then fails.
// The router drops that error. Without the snapshot the kernel cannot record
// conflict, so the top-level result stays unknown, indeterminate, sent, with
// an inconclusive relation. The compound readback step keeps the contradiction.
func testHuaweiExportModeMismatchPublicationThroughRouter(t *testing.T, stage bool) {
	t.Helper()
	link, peer := listenExport(t, "mode", map[byte][]uint16{1: plantImage(5000)}, exportPlantMode, exportPlantPower, -1)
	family := openExport(t, link, []ExportSystem{{Unit: 1, Label: "inverter", Card: "MA-V200", Path: "S-Dongle"}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	family.session = &exportCommitFailSession{Session: family.session, armed: true, stage: stage, cancel: cancel}
	result, err := invokeExportContext(t, ctx, family, nil, exportWattsArg("5", 3))
	if err != nil {
		t.Fatalf("router returned the publication error: %v", err)
	}
	assertExportStatus(t, result, "unknown", operation.OutcomeIndeterminate, operation.DeliverySent)
	if result.ReadbackRelation != operation.ReadbackInconclusive || result.Acknowledgement != "" || result.Reason != "" || result.Status == "failed" || result.Status == "protocol_ack" || result.KernelOutcome == operation.OutcomeConflict || result.KernelOutcome == operation.OutcomeAcknowledgedUnverified || result.KernelOutcome == operation.OutcomeApplied {
		t.Fatalf("result %+v", result)
	}
	if result.CompoundExecution == nil {
		t.Fatal("compound execution missing")
	}
	var readback *driver.CompoundStep
	for i := range result.CompoundExecution.Steps {
		step := &result.CompoundExecution.Steps[i]
		if step.Point == "readback" {
			readback = step
		}
	}
	want := driver.CompoundStep{Point: "readback", Attempted: true, ReadbackRelation: operation.ReadbackContradicts}
	if readback == nil || *readback != want {
		t.Fatalf("steps %+v", result.CompoundExecution.Steps)
	}
	if len(peer.writtenFrames()) != 2 || link.calls != 2 {
		t.Fatalf("writes %d calls %d", len(peer.writtenFrames()), link.calls)
	}
	assertNoExportFact(t, family)
}

func invokeExportContext(t *testing.T, ctx context.Context, family *HuaweiExport, tweak func(*driver.OperationRequest), args ...semreg.TypedField) (driver.OperationResult, error) {
	t.Helper()
	cut := capturedExport(t, family)
	if len(cut.SettingRoutes) == 0 {
		t.Fatal("no route")
	}
	route := cut.SettingRoutes[0]
	snap := exportSnapshot(t, cut)
	var revision semreg.Uint64
	for _, capability := range snap.Capabilities {
		if capability.InstanceID == route.Route.CapabilityInstance {
			revision = capability.Revision
		}
	}
	if revision == "" {
		t.Fatalf("capability revision missing for %s", route.Route.CapabilityInstance)
	}
	request := driver.OperationRequest{
		AssetID: snap.AssetID, BindingID: route.Route.BindingID, CapabilityInstanceID: route.Route.CapabilityInstance,
		Operation: pvDefinition(exportOp), Arguments: args, Preconditions: []operation.Precondition{}, Timeout: 5 * time.Second,
		ExpectedSemanticRevision: snap.Revisions.Semantic, ExpectedCapabilityRevision: snap.Revisions.Capabilities,
		ExpectedCapabilityInstanceRevision: revision, ExpectedDriverGeneration: route.Route.DriverGeneration,
		ExpectedSourceEpochID: route.Route.SourceEpochID,
	}
	if tweak != nil {
		tweak(&request)
	}
	return family.session.ScopedOperations().Invoke(ctx, request)
}

func TestHuaweiExportScopePhaseBeforeSend(t *testing.T) {
	link, peer := listenExport(t, "", map[byte][]uint16{1: emmaImage(1, 5000)}, exportModeAddress, exportPowerAddress, exportScopeAddress)
	family := openExport(t, link, []ExportSystem{{Unit: 1, Label: "plant", Card: "emma-v100r025c00spc102-2025-08-14", Path: "EMMA"}})
	result := invokeExport(t, family, nil, exportWattsArg("5", 3))
	assertExportStatus(t, result, "failed", operation.OutcomeFailedNoContact, operation.DeliveryNotSent)
	if result.Reason != "semantic.operation.reason.no_contact" || result.KernelOutcome == operation.OutcomeRejected {
		t.Fatalf("result %+v", result)
	}
	if result.CompoundExecution == nil || result.CompoundExecution.Steps[0].Reason != "scope_not_total" {
		t.Fatalf("cause %+v", result.CompoundExecution)
	}
	assertNoExportWrites(t, link, peer)
	assertScopeUnwritten(t, peer)
	if peer.cell(1, exportScopeAddress) != 1 || peer.cell(1, exportModeAddress) != 0 || peer.cell(1, exportPowerAddress) != 0 || peer.cell(1, exportPowerAddress+1) != 5000 {
		t.Fatal("image changed")
	}
	if !sameAddresses(peer.readList(), []uint16{exportPowerAddress, exportScopeAddress}) {
		t.Fatalf("reads %v", peer.readList())
	}
}

func TestHuaweiExportScopePhaseAfterSend(t *testing.T) {
	link, peer := listenExport(t, "scope-phase", map[byte][]uint16{1: emmaImage(0, 5000)}, exportModeAddress, exportPowerAddress, exportScopeAddress)
	family := openExport(t, link, []ExportSystem{{Unit: 1, Label: "plant", Card: "emma-v100r024c00-01-2024-07-15", Path: "EMMA"}})
	result := invokeExport(t, family, nil, exportWattsArg("5", 3))
	// The power row equals the request, so no pv.limit.export_power candidate is
	// stored. The contradicting scope read is still conflict.
	assertExportStatus(t, result, "failed", operation.OutcomeConflict, operation.DeliverySent)
	if result.ReadbackRelation != operation.ReadbackContradicts || result.Status == "readback_confirmed" || result.Status == "unknown" || result.KernelOutcome == operation.OutcomeIndeterminate || result.KernelOutcome == operation.OutcomeApplied {
		t.Fatalf("result %+v", result)
	}
	frames := peer.writtenFrames()
	if len(frames) != 2 || link.calls != 2 {
		t.Fatalf("writes %d calls %d", len(frames), link.calls)
	}
	assertScopeUnwritten(t, peer)
	assertNoExportFact(t, family)
}

func TestHuaweiExportScopeRereadDropped(t *testing.T) {
	link, peer := listenExport(t, "scope-drop", map[byte][]uint16{1: emmaImage(0, 5000)}, exportModeAddress, exportPowerAddress, exportScopeAddress)
	family := openExport(t, link, []ExportSystem{{Unit: 1, Label: "plant", Card: "emma-v100r025c00spc102-2025-08-14", Path: "EMMA"}})
	result := invokeExport(t, family, nil, exportWattsArg("5", 3))
	assertExportStatus(t, result, "protocol_ack", operation.OutcomeAcknowledgedUnverified, operation.DeliverySent)
	if result.ReadbackRelation != operation.ReadbackInconclusive || result.Status == "readback_confirmed" || result.KernelOutcome == operation.OutcomeApplied {
		t.Fatalf("result %+v", result)
	}
	if len(peer.writtenFrames()) != 2 || link.calls != 2 {
		t.Fatalf("writes %d calls %d", len(peer.writtenFrames()), link.calls)
	}
	assertNoExportFact(t, family)
}

func TestHuaweiExportPartialSend(t *testing.T) {
	link, peer := listenExport(t, "second-fail", map[byte][]uint16{1: emmaImage(0, 5000)}, exportModeAddress, exportPowerAddress, exportScopeAddress)
	family := openExport(t, link, []ExportSystem{{Unit: 1, Label: "plant", Card: "emma-v100r024c00-01-2024-07-15", Path: "EMMA"}})
	result := invokeExport(t, family, nil, exportWattsArg("5", 3))
	assertExportStatus(t, result, "unknown", operation.OutcomeIndeterminate, operation.DeliverySent)
	if result.KernelOutcome == operation.OutcomeAcknowledgedUnverified || result.Status == "protocol_ack" || result.KernelOutcome == operation.OutcomeFailedNoContact {
		t.Fatalf("result %+v", result)
	}
	frames := peer.writtenFrames()
	if len(frames) != 1 || link.calls != 2 {
		t.Fatalf("writes %d calls %d", len(frames), link.calls)
	}
	addr, qty, words := wordsOf(t, frames[0])
	if addr != exportModeAddress || qty != 1 || words[0] != 6 {
		t.Fatalf("first write %d x %d %v", addr, qty, words)
	}
	assertScopeUnwritten(t, peer)
	if peer.cell(1, exportPowerAddress) != 0 || peer.cell(1, exportPowerAddress+1) != 0x1388 {
		t.Fatal("value row changed")
	}
}

func TestHuaweiExportLostFirstWrite(t *testing.T) {
	link, peer := listenExport(t, "drop", map[byte][]uint16{1: plantImage(5000)}, exportPlantMode, exportPlantPower, -1)
	family := openExport(t, link, []ExportSystem{{Unit: 1, Label: "inverter", Card: "MC", Path: "S-Dongle"}})
	result := invokeExport(t, family, nil, exportWattsArg("5", 3))
	assertExportStatus(t, result, "unknown", operation.OutcomeIndeterminate, operation.DeliveryUnknown)
	if result.Status == "readback_confirmed" || result.KernelOutcome == operation.OutcomeApplied {
		t.Fatalf("result %+v", result)
	}
	frames := peer.writtenFrames()
	if len(frames) != 1 || link.calls != 1 {
		t.Fatalf("writes %d calls %d", len(frames), link.calls)
	}
	addr, _, _ := wordsOf(t, frames[0])
	if addr != exportPlantMode {
		t.Fatalf("write %d", addr)
	}
}

func TestHuaweiExportException(t *testing.T) {
	link, peer := listenExport(t, "exception", map[byte][]uint16{1: plantImage(5000)}, exportPlantMode, exportPlantPower, -1)
	family := openExport(t, link, []ExportSystem{{Unit: 1, Label: "inverter", Card: "MA-V200", Path: "SmartLogger"}})
	result := invokeExport(t, family, nil, exportWattsArg("5", 3))
	assertExportStatus(t, result, "failed", operation.OutcomeNoEffect, operation.DeliverySent)
	if result.Acknowledgement != operation.AckRejected || result.Status == "readback_confirmed" {
		t.Fatalf("result %+v", result)
	}
	if len(peer.writtenFrames()) != 1 || link.calls != 1 {
		t.Fatalf("writes %d calls %d", len(peer.writtenFrames()), link.calls)
	}
}

func TestHuaweiExportGuardAndRange(t *testing.T) {
	link, peer := listenExport(t, "", map[byte][]uint16{1: emmaImage(0, 0)}, exportModeAddress, exportPowerAddress, exportScopeAddress)
	family := openExport(t, link, []ExportSystem{{Unit: 1, Label: "plant", Card: "emma-v100r025c00spc102-2025-08-14", Path: "EMMA"}})
	stale := invokeExport(t, family, func(req *driver.OperationRequest) { req.ExpectedDriverGeneration = "99" }, exportWattsArg("5", 3))
	assertExportStatus(t, stale, "failed", operation.OutcomeRejected, operation.DeliveryNotSent)
	if stale.Reason != "semantic.operation.reason.admission:stale_driver_generation" {
		t.Fatalf("guard %+v", stale)
	}
	assertNoExportWrites(t, link, peer)
	negative := invokeExport(t, family, nil, exportWattsArg("-1", 0))
	assertExportStatus(t, negative, "failed", operation.OutcomeRejected, operation.DeliveryNotSent)
	if negative.Reason != "semantic.operation.reason.admission:bounds_exceeded" {
		t.Fatalf("negative %+v", negative)
	}
	over := invokeExport(t, family, nil, exportWattsArg("1", 8))
	assertExportStatus(t, over, "failed", operation.OutcomeRejected, operation.DeliveryNotSent)
	if over.Reason != "semantic.operation.reason.admission:bounds_exceeded" {
		t.Fatalf("over %+v", over)
	}
	assertNoExportWrites(t, link, peer)
}

func sameAddresses(got, want []uint16) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
