package reader

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

const syntheticSerialPath = "/synthetic/charger-line"

// varint and terminal build the response envelope:
// family 6, the operation's response tag, then the body.
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

// readbackBody encodes the readback body: field 1 holds limit (1), timeout (2) and
// inhibit (3); field 2 holds the configured current.
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
	// wall, when set, is the receipt's wall second; otherwise it follows the request number.
	wall int64
	// epoch and mono, when set, are the receipt's clock epoch and monotonic offset.
	epoch string
	mono  time.Duration
}

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

func (e *scriptedEndpoint) sent() []modbus.PrivateFunctionRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]modbus.PrivateFunctionRequest(nil), e.requests...)
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
	if s.epoch != "" {
		evidence.ClockEpoch = s.epoch
	}
	if s.mono != 0 {
		evidence.ReceivedAt = s.mono
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

type activation struct {
	mu    sync.Mutex
	calls int
}

func (a *activation) DriveTeslaHSCActivation(ctx context.Context, request tesla.HSCActivationRequest) (tesla.HSCActivationReport, error) {
	a.mu.Lock()
	a.calls++
	a.mu.Unlock()
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

type charger struct {
	t        *testing.T
	opens    int
	endpoint *scriptedEndpoint
	act      *activation
	deps     ChargerDeps
}

func newCharger(t *testing.T) *charger {
	c := &charger{t: t, act: &activation{}}
	c.deps = ChargerDeps{Activation: c.act, Open: func(config modbus.RTUProductionConfig) (tesla.Gen3ReadEndpoint, error) {
		c.opens++
		c.endpoint = &scriptedEndpoint{config: config, generation: 1}
		return c.endpoint, nil
	}}
	return c
}

var fiveOperations = []tesla.FC100Operation{
	tesla.FC100OperationCommonSystemInfo, tesla.FC100OperationWCGetVitals, tesla.FC100OperationWCGetLifetimeStats,
	tesla.FC100OperationWCGetSystemInfo, tesla.FC100OperationWCGetLoadSharingState,
}

func syntheticConfig(operations ...tesla.FC100Operation) ChargerConfig {
	windows := map[tesla.FC100Operation]tesla.Gen3ReadWindow{}
	for _, op := range []tesla.FC100Operation{tesla.FC100OperationWCGetProvisional, tesla.FC100OperationWCConfigureSettings} {
		windows[op] = tesla.Gen3ReadWindow{Freshness: time.Second, Retention: 2 * time.Second}
	}
	for _, op := range operations {
		windows[op] = tesla.Gen3ReadWindow{Freshness: 30 * time.Second, Retention: 10 * time.Minute}
	}
	return ChargerConfig{
		Endpoint: "synthetic-charger", SerialPath: syntheticSerialPath, Node: 0x10, Version: "wc3_24_44_3",
		ResponseTimeout: time.Second, MaxResponseDelay: 500 * time.Millisecond, MaxQuiescence: 900 * time.Millisecond,
		PollInterval: time.Hour, AcquisitionDeadline: 10 * time.Second, ActivationTimeout: time.Second, MaxAttempts: 2,
		PersistentIdentityCapacity: 3, Operations: operations, Windows: windows,
	}
}

func syntheticPublication() ChargerPublication {
	return ChargerPublication{
		Asset: "asset:synthetic-charger", Source: "source:synthetic-charger", Epoch: "epoch:synthetic-charger", Generation: 7,
		ClockEpoch: "clock:synthetic", EVSE: "evse-a", Connector: "connector-a",
		Policy: semreg.FreshnessPolicy{PolicyID: "policy:synthetic", Version: "1.0.0", FreshForNS: "30000000000", RetainForNS: "600000000000", MaxWallUncertaintyNS: "1"},
	}
}

// okBody is an opaque body the five observation reads accept.
var okBody = []byte{0x08, 0x01, 0x7a, 0x02, 0xaa, 0xbb}

// pollTerminal is the terminal frame payload a scripted poll of op answers with.
func pollTerminal(op tesla.FC100Operation, readback scalars) []byte {
	if op == tesla.FC100OperationWCGetProvisional {
		return terminal(6, 28, readbackBody(readback))
	}
	family, tag := uint64(6), map[tesla.FC100Operation]uint64{
		tesla.FC100OperationCommonSystemInfo: 3, tesla.FC100OperationWCGetVitals: 2, tesla.FC100OperationWCGetLifetimeStats: 4,
		tesla.FC100OperationWCGetSystemInfo: 10, tesla.FC100OperationWCGetLoadSharingState: 12}[op]
	if op == tesla.FC100OperationCommonSystemInfo {
		family = 4
	}
	return terminal(family, tag, okBody)
}

func pushPolls(e *scriptedEndpoint, operations []tesla.FC100Operation, readback scalars) {
	for _, op := range operations {
		e.push(step{echo: true, terminal: pollTerminal(op, readback)})
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

type fakeTx struct {
	s      *fakeSession
	staged []semreg.PublicationBatch
}

func (tx *fakeTx) OperatorLinks(upserts []semreg.NativeBinding) []semreg.IdentityLink {
	tx.s.mu.Lock()
	defer tx.s.mu.Unlock()
	tx.s.linkCalls = append(tx.s.linkCalls, append([]semreg.NativeBinding(nil), upserts...))
	if tx.s.links == nil {
		return nil
	}
	return tx.s.links(upserts)
}
func (tx *fakeTx) AdmitManufacturerSerial(driver.ManufacturerSerialClaim) error { return nil }
func (tx *fakeTx) Stage(b semreg.PublicationBatch) error {
	tx.staged = append(tx.staged, b)
	return nil
}
func (tx *fakeTx) DeclareSource(driver.Source) error             { return nil }
func (tx *fakeTx) DeclareSettingRoute(driver.SettingRoute) error { return nil }
func (tx *fakeTx) StageLink(driver.LinkClaim) error              { return nil }
func (tx *fakeTx) StageSample(driver.Sample) error               { return nil }
func (tx *fakeTx) Commit(context.Context) error {
	tx.s.mu.Lock()
	if tx.s.commitErr != nil {
		defer tx.s.mu.Unlock()
		return tx.s.commitErr
	}
	tx.s.batches = append(tx.s.batches, tx.staged...)
	tx.s.mu.Unlock()
	return nil
}
func (tx *fakeTx) Abort(context.Context) error {
	tx.s.mu.Lock()
	tx.s.aborts++
	tx.s.mu.Unlock()
	return nil
}

type fakeSession struct {
	driver.Session
	native    recordingRegistry
	mu        sync.Mutex
	batches   []semreg.PublicationBatch
	linkCalls [][]semreg.NativeBinding
	aborts    int
	commitErr error
	links     func([]semreg.NativeBinding) []semreg.IdentityLink
}

func (s *fakeSession) Publisher() driver.Publisher { return fakePublisher{s} }

func (s *fakeSession) Native() driver.NativeRegistry { return &s.native }

type fakePublisher struct{ s *fakeSession }

func (p fakePublisher) Begin(context.Context) (driver.PublicationTransaction, error) {
	return &fakeTx{s: p.s}, nil
}

func (s *fakeSession) committed() []semreg.PublicationBatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]semreg.PublicationBatch(nil), s.batches...)
}

func (s *fakeSession) operatorLinkCalls() [][]semreg.NativeBinding {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]semreg.NativeBinding(nil), s.linkCalls...)
}
