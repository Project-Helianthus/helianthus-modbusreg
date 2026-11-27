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
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

const limitsKind driver.DriverKind = "modbus.limits"

const (
	sunSpecBase = 40000
	model121At  = 40002
	model123At  = 40034
	setAddress  = 40039
	enaAddress  = 40043
)

type limitClock struct {
	mu   sync.Mutex
	now  time.Time
	mono uint64
}

func (c *limitClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *limitClock) Sample() (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mono, nil
}
func (c *limitClock) Anchor(sample uint64) ([]byte, error) { return []byte{byte(sample)}, nil }
func (c *limitClock) Elapsed(_ []byte, sample uint64) (uint64, error) {
	return sample, nil
}

type scriptedPeer struct {
	mu     sync.Mutex
	units  map[byte][]uint16
	frames [][]byte
	mode   string
}

func (p *scriptedPeer) writtenFrames() [][]byte {
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

func (p *scriptedPeer) serve(conn net.Conn) error {
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

func (p *scriptedPeer) answer(unit byte, frame []byte) ([]byte, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.frames = append(p.frames, append([]byte(nil), frame...))
	request := frame[7:]
	cells := p.units[unit]
	switch request[0] {
	case 3:
		addr, qty := binary.BigEndian.Uint16(request[1:3]), binary.BigEndian.Uint16(request[3:5])
		if int(addr)+int(qty) > len(cells) {
			return nil, false, errors.New("synthetic read outside image")
		}
		body := []byte{3, byte(qty * 2)}
		for _, word := range cells[addr : addr+qty] {
			body = binary.BigEndian.AppendUint16(body, word)
		}
		return body, false, nil
	case 16:
		if p.mode == "drop" {
			return nil, true, errors.New("synthetic response dropped")
		}
		addr, qty := binary.BigEndian.Uint16(request[1:3]), binary.BigEndian.Uint16(request[3:5])
		if int(request[5]) != int(qty)*2 || len(request) < 6+int(qty)*2 {
			return nil, false, errors.New("synthetic write width")
		}
		for i := uint16(0); i < qty; i++ {
			word := binary.BigEndian.Uint16(request[6+2*i:])
			if p.mode == "conflict" && i == 0 && qty == 5 {
				word = 0
			}
			cells[addr+i] = word
		}
		body := []byte{16}
		body = binary.BigEndian.AppendUint16(body, addr)
		body = binary.BigEndian.AppendUint16(body, qty)
		return body, false, nil
	default:
		return nil, false, errors.New("synthetic function")
	}
}

type scriptedLink struct {
	mu    sync.Mutex
	conn  net.Conn
	calls int
	peer  *scriptedPeer
	txn   uint16
}

func (s *scriptedLink) ReadHolding(ctx context.Context, unit uint8, address, quantity uint16) ([]uint16, error) {
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

func (s *scriptedLink) WriteMultiple(ctx context.Context, unit uint8, address uint16, registers []uint16, admission modbus.RegisterWriteAdmission) (modbus.WriteMultipleRegistersAcknowledgement, error) {
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
	if len(reply) != 5 || reply[0] != 16 {
		return modbus.WriteMultipleRegistersAcknowledgement{}, errors.New("synthetic write reply")
	}
	return modbus.WriteMultipleRegistersAcknowledgement{Address: binary.BigEndian.Uint16(reply[1:3]), Quantity: binary.BigEndian.Uint16(reply[3:5])}, nil
}

func (s *scriptedLink) exchange(ctx context.Context, unit uint8, pdu []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.txn++
	header := make([]byte, 7)
	binary.BigEndian.PutUint16(header[0:2], s.txn)
	binary.BigEndian.PutUint16(header[4:6], uint16(len(pdu)+1))
	header[6] = unit
	deadline, ok := ctx.Deadline()
	if ok {
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

func sunSpecImage(withControls bool) []uint16 {
	cells := make([]uint16, 42000)
	cells[sunSpecBase], cells[sunSpecBase+1] = 0x5375, 0x6e53
	cells[model121At], cells[model121At+1] = 121, 30
	cells[model121At+2] = 8000
	if !withControls {
		cells[model123At], cells[model123At+1] = 0xffff, 0
		return cells
	}
	cells[model123At], cells[model123At+1] = 123, 24
	cells[setAddress], cells[setAddress+1], cells[setAddress+2], cells[setAddress+3], cells[setAddress+4] = 500, 7, 300, 11, 1
	cells[model123At+23] = 0xffff
	cells[model123At+26], cells[model123At+27] = 0xffff, 0
	return cells
}

func listenPair(t *testing.T, mode string) *scriptedLink {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	peer := &scriptedPeer{units: map[byte][]uint16{1: sunSpecImage(true), 2: sunSpecImage(false)}, mode: mode}
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
	return &scriptedLink{conn: client, peer: peer}
}

type recordingFamily struct {
	*SunSpecPv
	startErr error
}

func (r *recordingFamily) Start(ctx context.Context, s driver.Session) error {
	r.startErr = r.SunSpecPv.Start(ctx, s)
	return r.startErr
}

type limitFactory struct{ family *recordingFamily }

func (limitFactory) Descriptor() driver.Descriptor { return driver.Descriptor{} }
func (limitFactory) Plan(c driver.Config) ([]driver.InstancePlan, error) {
	return []driver.InstancePlan{{Driver: limitsKind, Instance: driver.InstanceKey(c.Instance)}}, nil
}
func (f limitFactory) Construct(context.Context, driver.InstancePlan) (driver.Instance, error) {
	return f.family, nil
}

func openSunSpecPv(t *testing.T, access Registers) (*runtime.Runtime, *SunSpecPv) {
	t.Helper()
	rec := &recordingFamily{SunSpecPv: &SunSpecPv{cfg: Config{Access: access, Inverters: []Inverter{
		{Unit: 1, Label: "inverter-a", Base: sunSpecBase},
		{Unit: 2, Label: "inverter-b", Base: sunSpecBase},
	}}}}
	ctx := context.Background()
	clock := &limitClock{now: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}
	cfg := life.InstanceConfig{Driver: limitsKind, Instance: "limits.one", Enabled: true, Config: driver.Config{Instance: "limits.one", Enabled: true}}
	rt, err := runtime.OpenHost(ctx, life.Installation{Key: "installation.sunspec-limit", Bridge: "bridge.test", StateDir: t.TempDir(), Clock: clock, Instances: []life.InstanceConfig{cfg}}, runtime.HostOptions{}, limitFactory{rec})
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
		status, snapErr := rt.Snapshot(life.Ref{Driver: limitsKind, Instance: "limits.one"})
		t.Fatalf("family start: %v; snapshot %s %v (%v)", rec.startErr, status.ObservedState, status.Reason, snapErr)
	}
	if rec.session == nil {
		t.Fatal("family started without a session")
	}
	return rt, rec.SunSpecPv
}

func wattsArg(coefficient string, exponent int32) semreg.TypedField {
	quantity := semreg.Quantity{Number: semreg.Decimal{Coefficient: coefficient, Exponent10: exponent}, Unit: wattUnit}
	return semreg.TypedField{ID: argID, Value: semreg.Value{Kind: semreg.ValueQuantity, Quantity: &quantity}}
}

func boolArg(value bool) semreg.TypedField {
	bit := value
	return semreg.TypedField{ID: argID, Value: semreg.Value{Kind: semreg.ValueBoolean, Boolean: &bit}}
}

func captured(t *testing.T, family *SunSpecPv) driver.SemanticCut {
	t.Helper()
	cut, err := family.session.SemanticInput().Capture(context.Background(), driver.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	return cut
}

func limitSnapshot(t *testing.T, cut driver.SemanticCut) semreg.Snapshot {
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

func invokeSunSpec(t *testing.T, family *SunSpecPv, args ...semreg.TypedField) driver.OperationResult {
	t.Helper()
	cut := captured(t, family)
	if len(cut.SettingRoutes) != 1 {
		t.Fatalf("routes %d", len(cut.SettingRoutes))
	}
	snap, route := limitSnapshot(t, cut), cut.SettingRoutes[0]
	var revision semreg.Uint64
	for _, capability := range snap.Capabilities {
		if capability.InstanceID == route.Route.CapabilityInstance {
			revision = capability.Revision
		}
	}
	if revision == "" {
		t.Fatalf("capability revision missing for %s", route.Route.CapabilityInstance)
	}
	ctx := context.Background()
	result, err := family.session.ScopedOperations().Invoke(ctx, driver.OperationRequest{
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

func stableGraph(t *testing.T, family *SunSpecPv) (string, semreg.Decimal, semreg.Decimal) {
	t.Helper()
	cut := captured(t, family)
	snap := limitSnapshot(t, cut)
	var resource string
	for _, item := range snap.Resources {
		if item.Kind == kindInverter {
			if resource != "" {
				t.Fatalf("two inverter resources")
			}
			resource = string(item.ResourceID)
		}
	}
	var constraint semreg.Decimal
	caps := 0
	for _, capability := range snap.Capabilities {
		if capability.Definition.ID != capDef {
			continue
		}
		caps++
		if capability.Definition.ID != capDef || len(capability.Constraints) != 1 || capability.Constraints[0].Value.Quantity == nil {
			t.Fatalf("capability %+v", capability)
		}
		constraint = capability.Constraints[0].Value.Quantity.Number
	}
	if caps != 1 {
		t.Fatalf("capabilities %d", caps)
	}
	services := 0
	for _, service := range snap.Services {
		if service.Definition.ID == serviceDef {
			services++
		}
	}
	if services != 1 || len(cut.SettingRoutes) != 1 || cut.SettingRoutes[0].Limits == nil || cut.SettingRoutes[0].Limits.Max == nil {
		t.Fatalf("services %d routes %+v", services, cut.SettingRoutes)
	}
	return resource, constraint, cut.SettingRoutes[0].Limits.Max.Number
}

func wordsOf(t *testing.T, frame []byte) (addr, qty uint16, words []uint16) {
	t.Helper()
	pdu := frame[7:]
	addr, qty = binary.BigEndian.Uint16(pdu[1:3]), binary.BigEndian.Uint16(pdu[3:5])
	for i := uint16(0); i < qty; i++ {
		words = append(words, binary.BigEndian.Uint16(pdu[6+2*i:]))
	}
	return addr, qty, words
}

func TestSunSpecPvCapabilityAndEqualGraphs(t *testing.T) {
	first := listenPair(t, "")
	_, family := openSunSpecPv(t, first)
	resource, constraint, limit := stableGraph(t, family)
	if resource != string(resourceID("inverter-a")) {
		t.Fatalf("resource %s", resource)
	}
	if constraint.Coefficient != "4" || constraint.Exponent10 != 3 {
		t.Fatalf("constraint %+v", constraint)
	}
	if limit.Coefficient != "8" || limit.Exponent10 != 3 {
		t.Fatalf("limit %+v", limit)
	}
	if len(first.peer.writtenFrames()) != 0 {
		t.Fatal("publication wrote")
	}
	second := listenPair(t, "")
	_, again := openSunSpecPv(t, second)
	otherResource, otherConstraint, otherLimit := stableGraph(t, again)
	if otherResource != resource || otherConstraint != constraint || otherLimit != limit {
		t.Fatalf("graphs (%s %+v %+v) and (%s %+v %+v)", resource, constraint, limit, otherResource, otherConstraint, otherLimit)
	}
}

func TestSunSpecPvSetConfirmsReadback(t *testing.T) {
	link := listenPair(t, "")
	_, family := openSunSpecPv(t, link)
	result := invokeSunSpec(t, family, wattsArg("4", 3))
	frames := link.peer.writtenFrames()
	if result.Status != "readback_confirmed" || result.KernelOutcome != operation.OutcomeApplied {
		t.Fatalf("result %+v frames %d", result, len(frames))
	}
	if len(frames) != 1 || link.calls != 1 {
		t.Fatalf("writes %d calls %d", len(frames), link.calls)
	}
	addr, qty, words := wordsOf(t, frames[0])
	if addr != setAddress || qty != 5 || words[0] != 500 || words[1] != 7 || words[2] != 300 || words[3] != 11 || words[4] != 1 {
		t.Fatalf("write %d x %d %v", addr, qty, words)
	}
}

func TestSunSpecPvContradictingReadback(t *testing.T) {
	link := listenPair(t, "conflict")
	_, family := openSunSpecPv(t, link)
	result := invokeSunSpec(t, family, wattsArg("4", 3))
	if result.Status != "failed" || result.KernelOutcome != operation.OutcomeConflict || result.Status == "unknown" || result.KernelOutcome == operation.OutcomeIndeterminate {
		t.Fatalf("result %+v", result)
	}
	if len(link.peer.writtenFrames()) != 1 {
		t.Fatalf("writes %d", len(link.peer.writtenFrames()))
	}
}

func TestSunSpecPvGuardMismatchSendsNothing(t *testing.T) {
	link := listenPair(t, "")
	_, family := openSunSpecPv(t, link)
	cut := captured(t, family)
	route := cut.SettingRoutes[0]
	dispatch := driver.Dispatch{Request: driver.OperationRequest{Arguments: []semreg.TypedField{wattsArg("4", 3)}, Timeout: time.Second}, Route: route.Route, Guard: operation.GuardClaims{BindingID: "binding:other"}, AdmittedAt: wallNow(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC))}
	completion, err := family.Execute(context.Background(), dispatch)
	if err != nil || completion.CompoundExecution == nil || completion.CompoundExecution.Steps[0].Reason != "guard_mismatch" {
		t.Fatalf("completion %+v %v", completion, err)
	}
	if len(link.peer.writtenFrames()) != 0 || link.calls != 0 {
		t.Fatalf("writes %d calls %d", len(link.peer.writtenFrames()), link.calls)
	}
}

func TestSunSpecPvReleaseIsAcknowledgementOnly(t *testing.T) {
	link := listenPair(t, "")
	_, family := openSunSpecPv(t, link)
	result := invokeSunSpec(t, family, boolArg(false))
	frames := link.peer.writtenFrames()
	if result.Status != "protocol_ack" || result.KernelOutcome != operation.OutcomeAcknowledgedUnverified || result.Status == "readback_confirmed" {
		t.Fatalf("result %+v", result)
	}
	if len(frames) != 1 {
		t.Fatalf("writes %d", len(frames))
	}
	addr, qty, words := wordsOf(t, frames[0])
	if addr != enaAddress || qty != 1 || words[0] != 0 {
		t.Fatalf("release %d x %d %v", addr, qty, words)
	}
}

func TestSunSpecPvRefusalsSendNothing(t *testing.T) {
	link := listenPair(t, "")
	_, family := openSunSpecPv(t, link)
	other := semreg.TypedField{ID: "pv.limit.other", Value: wattsArg("4", 3).Value}
	cases := []struct {
		name string
		args []semreg.TypedField
		want string
	}{
		{"true", []semreg.TypedField{boolArg(true)}, "semantic.operation.reason.release_true"},
		{"mixed", []semreg.TypedField{boolArg(false), other}, "semantic.operation.reason.mixed_release"},
		{"negative", []semreg.TypedField{wattsArg("-4", 3)}, "semantic.operation.reason.negative_set"},
	}
	for _, tc := range cases {
		result := invokeSunSpec(t, family, tc.args...)
		if result.Reason != tc.want || result.Delivery != operation.DeliveryNotSent || len(link.peer.writtenFrames()) != 0 || link.calls != 0 {
			t.Fatalf("%s result %+v writes %d calls %d", tc.name, result, len(link.peer.writtenFrames()), link.calls)
		}
	}
	if report := family.Stop(context.Background()); !report.Confirmed {
		t.Fatal("stop was not confirmed")
	}
	result := invokeSunSpec(t, family, wattsArg("4", 3))
	if result.Reason != "semantic.operation.reason.executor_missing" || len(link.peer.writtenFrames()) != 0 || link.calls != 0 {
		t.Fatalf("missing owner %+v writes %d", result, link.calls)
	}
}

func TestSunSpecPvLostResponseIsUnknown(t *testing.T) {
	link := listenPair(t, "drop")
	_, family := openSunSpecPv(t, link)
	result := invokeSunSpec(t, family, wattsArg("4", 3))
	if result.Status != "unknown" || result.KernelOutcome != operation.OutcomeIndeterminate {
		t.Fatalf("result %+v", result)
	}
	if len(link.peer.writtenFrames()) != 1 || link.calls != 1 {
		t.Fatalf("writes %d calls %d", len(link.peer.writtenFrames()), link.calls)
	}
}
