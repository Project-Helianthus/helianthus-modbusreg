package write

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	"github.com/Project-Helianthus/helianthus-modbusreg/driver/evse/reader"
	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/packs/evse"
)

const syntheticSerialPath = "/synthetic/charger-line"

func varint(v uint64) []byte {
	var out []byte
	for v >= 0x80 {
		out = append(out, byte(v)|0x80)
		v >>= 7
	}
	return append(out, byte(v))
}

func terminal(family, tag uint64, body []byte) []byte {
	inner := append(varint(tag<<3|2), varint(uint64(len(body)))...)
	inner = append(inner, body...)
	message := append(varint(family<<3|2), varint(uint64(len(inner)))...)
	message = append(message, inner...)
	return append([]byte{byte(len(message))}, message...)
}

// scalars are the members of a t28 body; a nil member is absent.
type scalars struct {
	limit, timeout, configured *uint32
	inhibit                    *bool
}

func u(v uint32) *uint32 { return &v }
func b(v bool) *bool     { return &v }

func readbackBody(s scalars) []byte {
	var inner []byte
	if s.limit != nil {
		inner = append(append(inner, 0x08), varint(uint64(*s.limit))...)
	}
	if s.timeout != nil {
		inner = append(append(inner, 0x10), varint(uint64(*s.timeout))...)
	}
	if s.inhibit != nil {
		inner = append(inner, 0x18, map[bool]byte{false: 0, true: 1}[*s.inhibit])
	}
	body := append([]byte{0x0a, byte(len(inner))}, inner...)
	if s.configured != nil {
		body = append(append(body, 0x10), varint(uint64(*s.configured))...)
	}
	return body
}

type step struct {
	echo     bool
	terminal []byte
	timeout  bool
	wall     int64
}

// scriptedEndpoint answers each private-function request with the next step and
// records every request it was handed; with no step left, or on a timeout step,
// it fences its generation.
type scriptedEndpoint struct {
	mu         sync.Mutex
	config     modbus.RTUProductionConfig
	generation uint64
	nextID     uint64
	steps      []step
	requests   []modbus.PrivateFunctionRequest
	fenced     bool
	closed     bool
}

func (e *scriptedEndpoint) push(s ...step) {
	e.mu.Lock()
	e.steps = append(e.steps, s...)
	e.mu.Unlock()
}

func (e *scriptedEndpoint) sent() [][]byte {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out [][]byte
	for _, r := range e.requests {
		out = append(out, r.Payload())
	}
	return out
}

func (e *scriptedEndpoint) ExchangePrivateFunction(ctx context.Context, unit byte, request modbus.PrivateFunctionRequest, policy modbus.PrivateFunctionResponsePolicy, classifier modbus.RTUPrivateFunctionResponseClassifier) ([]modbus.RTUPrivateFunctionResponseADU, modbus.RTUPrivateFunctionEvidence, error) {
	e.mu.Lock()
	if e.closed || e.fenced {
		e.mu.Unlock()
		return nil, modbus.RTUPrivateFunctionEvidence{}, errors.New("endpoint unavailable")
	}
	e.requests = append(e.requests, request)
	var s step
	if len(e.steps) > 0 {
		s, e.steps = e.steps[0], e.steps[1:]
	} else {
		s = step{timeout: true}
	}
	candidate := modbus.RTUPrivateFunctionAdmissionRequest{Endpoint: e.config.Endpoint, Generation: e.generation, UnitID: unit, Function: request.FunctionCode(), Payload: request.Payload()}
	admission := e.config.PrivateFunction.Admission
	e.mu.Unlock()
	if !admission.AdmitRTUPrivateFunction(candidate) {
		return nil, modbus.RTUPrivateFunctionEvidence{}, errors.New("not admitted")
	}
	requestADU, err := modbus.EncodeRTUPrivateFunctionADU(unit, request)
	if err != nil {
		return nil, modbus.RTUPrivateFunctionEvidence{}, err
	}
	e.mu.Lock()
	e.nextID++
	evidence := modbus.RTUPrivateFunctionEvidence{Endpoint: e.config.Endpoint, Generation: e.generation, RequestID: e.nextID, UnitID: unit,
		Function: request.FunctionCode(), RequestADU: requestADU, SentAt: time.Millisecond, ReceivedAt: time.Duration(e.nextID) * time.Second,
		ReceiptWall: time.Unix(int64(e.nextID)*10, 0), ClockEpoch: "scripted"}
	if s.wall != 0 {
		evidence.ReceiptWall = time.Unix(s.wall, 0)
	}
	e.mu.Unlock()
	if s.timeout {
		e.mu.Lock()
		e.fenced = true
		e.mu.Unlock()
		evidence.TerminalOutcome = "transport_fault"
		return nil, evidence, context.DeadlineExceeded
	}
	var responses []modbus.RTUPrivateFunctionResponseADU
	frame := func(payload []byte) error {
		response, err := modbus.NewPrivateFunctionRequest(request.FunctionCode(), payload)
		if err != nil {
			return err
		}
		adu, err := modbus.EncodeRTUPrivateFunctionADU(unit, response)
		if err != nil {
			return err
		}
		decoded, err := modbus.DecodeRTUPrivateFunctionResponseADU(unit, request, adu)
		if err != nil {
			return err
		}
		evidence.ResponseADUs = append(evidence.ResponseADUs, adu)
		responses = append(responses, decoded)
		return nil
	}
	if s.echo {
		if err := frame(request.Payload()); err != nil {
			return nil, evidence, err
		}
		if _, err := classifier.ClassifyRTUPrivateFunctionResponse(responses[0]); err != nil {
			return nil, evidence, err
		}
	}
	if err := frame(s.terminal); err != nil {
		return nil, evidence, err
	}
	if _, err := classifier.ClassifyRTUPrivateFunctionResponse(responses[len(responses)-1]); err != nil {
		return nil, evidence, err
	}
	e.mu.Lock()
	evidence.Current = !e.closed && !e.fenced
	e.mu.Unlock()
	evidence.IntegrityValid, evidence.TerminalOutcome = true, "success"
	return responses, evidence, nil
}

func (e *scriptedEndpoint) Recover(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.generation++
	e.fenced = false
	return nil
}

func (e *scriptedEndpoint) Close() error {
	e.mu.Lock()
	e.closed = true
	e.mu.Unlock()
	return nil
}

type activation struct{}

func (activation) DriveTeslaHSCActivation(ctx context.Context, request tesla.HSCActivationRequest) (tesla.HSCActivationReport, error) {
	event := func(kind tesla.HSCActivationEventKind, payload []byte) tesla.HSCActivationEvent {
		e := tesla.HSCActivationEvent{Kind: kind, Bytes: append([]byte(nil), payload...), LastByteOffset: time.Millisecond}
		if kind == tesla.HSCActivationEventEcho {
			e.Turnaround, e.WriteDuration = time.Millisecond, time.Millisecond
		}
		return e
	}
	greeting := tesla.HSCActivationGreeting()
	return tesla.HSCActivationReport{ClockEpoch: "synthetic", Events: []tesla.HSCActivationEvent{
		event(tesla.HSCActivationEventGreeting, greeting), event(tesla.HSCActivationEventEcho, greeting),
		event(tesla.HSCActivationEventConfirmation, tesla.HSCActivationConfirmation()),
	}}, nil
}

// fakeClock is the clock of the reader and of the writer; After fires when the
// clock is advanced past it.
type fakeClock struct {
	mu      sync.Mutex
	at      time.Time
	waiters []waiter
}

type waiter struct {
	due time.Time
	ch  chan time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.at }

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	c.waiters = append(c.waiters, waiter{c.at.Add(d), ch})
	return ch
}

func (c *fakeClock) set(at time.Time) { c.mu.Lock(); c.at = at; c.mu.Unlock() }

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.at = c.at.Add(d)
	var keep []waiter
	for _, w := range c.waiters {
		if !w.due.After(c.at) {
			w.ch <- c.at
		} else {
			keep = append(keep, w)
		}
	}
	c.waiters = keep
	c.mu.Unlock()
}

func (c *fakeClock) waiting() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.waiters) }

// recordingRegistry is a synthetic native registry.
type recordingRegistry struct{}

func (recordingRegistry) Root() driver.NodeRef {
	return driver.NodeRef{Bridge: "bridge.synthetic", ID: semreg.ResourceID("node.root")}
}
func (recordingRegistry) Node(s driver.NodeSpec) (driver.NodeRef, error) {
	return driver.NodeRef{Bridge: "bridge.synthetic", ID: semreg.ResourceID("node." + string(s.Local))}, nil
}
func (recordingRegistry) ReadMember(driver.NodeRef, driver.MemberSpec, driver.NativeReader) error {
	return nil
}
func (recordingRegistry) View(driver.ViewSpec) error { return nil }
func (recordingRegistry) Method(driver.NodeRef, driver.MethodSpec, driver.NativeMethod) error {
	return nil
}

type registered struct {
	route   driver.SettingRoute
	exec    driver.Executor
	removed bool
}

// fakeSession applies every batch the reader commits to a publication kernel
// that carries the EVSE pack, serves the latest snapshot as the scoped input,
// and records the route, the source and the executor of the writer.
type fakeSession struct {
	driver.Session
	t   *testing.T
	pub reader.ChargerPublication

	mu         sync.Mutex
	kernel     *semreg.PublicationKernel
	snapshot   semreg.Snapshot
	batches    []semreg.PublicationBatch
	sources    []driver.Source
	routes     []driver.SettingRoute
	executors  []*registered
	captureErr error
	routeErr   error
}

func (s *fakeSession) Native() driver.NativeRegistry       { return recordingRegistry{} }
func (s *fakeSession) Publisher() driver.Publisher         { return fakePublisher{s} }
func (s *fakeSession) SemanticInput() driver.SemanticInput { return captureInput{s: s} }

func (s *fakeSession) RegisterExecutor(route driver.SettingRoute, exec driver.Executor) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := &registered{route: route, exec: exec}
	s.executors = append(s.executors, entry)
	return func() { s.mu.Lock(); entry.removed = true; s.mu.Unlock() }, nil
}

func (s *fakeSession) live() []*registered {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*registered
	for _, e := range s.executors {
		if !e.removed {
			out = append(out, e)
		}
	}
	return out
}

type captureInput struct {
	driver.SemanticInput
	s *fakeSession
}

func (c captureInput) Capture(context.Context, driver.Filter) (driver.SemanticCut, error) {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	if c.s.captureErr != nil {
		return driver.SemanticCut{}, c.s.captureErr
	}
	return driver.SemanticCut{Snapshots: []semreg.Snapshot{c.s.snapshot}}, nil
}

type fakePublisher struct{ s *fakeSession }

func (p fakePublisher) Begin(context.Context) (driver.PublicationTransaction, error) {
	p.s.mu.Lock()
	err := p.s.routeErr
	p.s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return &fakeTx{s: p.s}, nil
}

type fakeTx struct {
	s       *fakeSession
	staged  []semreg.PublicationBatch
	sources []driver.Source
	routes  []driver.SettingRoute
}

func (tx *fakeTx) OperatorLinks([]semreg.NativeBinding) []semreg.IdentityLink   { return nil }
func (tx *fakeTx) AdmitManufacturerSerial(driver.ManufacturerSerialClaim) error { return nil }
func (tx *fakeTx) Stage(batch semreg.PublicationBatch) error {
	tx.staged = append(tx.staged, batch)
	return nil
}
func (tx *fakeTx) StageLink(driver.LinkClaim) error { return nil }
func (tx *fakeTx) StageSample(driver.Sample) error  { return nil }
func (tx *fakeTx) Abort(context.Context) error      { return nil }
func (tx *fakeTx) DeclareSource(s driver.Source) error {
	tx.sources = append(tx.sources, s)
	return nil
}
func (tx *fakeTx) DeclareSettingRoute(r driver.SettingRoute) error {
	tx.routes = append(tx.routes, r)
	return r.Validate()
}

func (tx *fakeTx) Commit(context.Context) error {
	s := tx.s
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, batch := range tx.staged {
		if s.kernel == nil {
			kernel, err := semreg.NewPublicationKernel(s.pub.Asset, evse.New())
			if err != nil {
				return err
			}
			s.kernel = kernel
			batch.SourceUpserts = []semreg.SourceDescriptor{sourceDescriptor(s.pub)}
			digest, err := batch.ComputedDigest()
			if err != nil {
				return err
			}
			batch.BatchDigest = digest
		}
		wallNS, _ := strconv.ParseInt(string(batch.ObservedAt.UnixNanoseconds), 10, 64)
		snapshot, _, err := s.kernel.Apply(batch, semreg.MonotonicPoint{ClockEpochID: "clock:charger-reader", Nanoseconds: semreg.Uint64(strconv.FormatInt(wallNS, 10))})
		if err != nil {
			return err
		}
		s.snapshot = snapshot
		s.batches = append(s.batches, batch)
	}
	s.sources = append(s.sources, tx.sources...)
	s.routes = append(s.routes, tx.routes...)
	return nil
}

func evidenceRef(material string) semreg.EvidenceRef {
	return evidence(KindRefusal, material)
}

func sourceDescriptor(pub reader.ChargerPublication) semreg.SourceDescriptor {
	return semreg.SourceDescriptor{SourceID: pub.Source, SourceEpochID: pub.Epoch, ProtocolID: "protocol.synthetic", ProfileID: "profile.synthetic",
		ProfileVersion: "1", RegistryEvidence: evidenceRef("registry"), StartedAt: semreg.TimePoint{UnixNanoseconds: "1000000000", ClockID: "clock.utc", UncertaintyNS: "0"},
		State: semreg.SourceCurrent, Revision: "1"}
}

func (s *fakeSession) candidate(element string) (semreg.FactCandidate, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.pub.ConnectorCandidateIdentifier(element)
	for _, envelope := range s.snapshot.Facts {
		for _, c := range envelope.Candidates {
			if c.CandidateID == id {
				return c, true
			}
		}
	}
	return semreg.FactCandidate{}, false
}

// ampsOf reads the amperes of a published quantity.
func ampsOf(t *testing.T, c semreg.FactCandidate) int64 {
	t.Helper()
	if c.Value == nil || c.Value.Quantity == nil {
		t.Fatalf("candidate carries no quantity: %+v", c.Value)
	}
	n, err := strconv.ParseInt(c.Value.Quantity.Number.Coefficient, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	for e := c.Value.Quantity.Number.Exponent10; e > 0; e-- {
		n *= 10
	}
	return n
}

func syntheticConfig() reader.ChargerConfig {
	windows := map[tesla.FC100Operation]tesla.Gen3ReadWindow{
		tesla.FC100OperationWCGetProvisional:    {Freshness: time.Hour, Retention: 2 * time.Hour},
		tesla.FC100OperationWCConfigureSettings: {Freshness: time.Second, Retention: 2 * time.Second},
		tesla.FC100OperationCommonSystemInfo:    {Freshness: time.Hour, Retention: 2 * time.Hour},
	}
	return reader.ChargerConfig{
		Endpoint: "synthetic-charger", SerialPath: syntheticSerialPath, Node: 0x10, Version: "wc3_24_44_3",
		ResponseTimeout: time.Second, MaxResponseDelay: 500 * time.Millisecond, MaxQuiescence: 900 * time.Millisecond,
		PollInterval: time.Hour, AcquisitionDeadline: 10 * time.Second, ActivationTimeout: time.Second, MaxAttempts: 1,
		PersistentIdentityCapacity: 3, Operations: []tesla.FC100Operation{tesla.FC100OperationCommonSystemInfo}, Windows: windows,
		ProvisionalReadbackQualified: true, ProvisionalSetQualified: true,
	}
}

func syntheticPublication() reader.ChargerPublication {
	return reader.ChargerPublication{
		Asset: "asset:synthetic-charger", Source: "source:synthetic-charger", Epoch: "epoch:synthetic-charger", Generation: 7,
		ClockEpoch: "clock:synthetic", EVSE: "evse-a", Connector: "connector-a",
		Policy: semreg.FreshnessPolicy{PolicyID: "policy:synthetic", Version: "1.0.0", FreshForNS: "30000000000", RetainForNS: "600000000000", MaxWallUncertaintyNS: "1"},
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

var _ = bytes.Equal
