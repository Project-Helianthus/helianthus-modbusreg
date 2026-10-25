package modbusreg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// The H06 harness drives the providers through scripted sessions. A script is
// the exact request order of one or more cycles; every reply is taken from the
// sanitized identity and monitoring fixtures, or is a timeout, an exception or
// a value the provider-cycle fixture marks synthetic.

const h06FixtureDirectory = "testdata/huawei/provider"

// ---- clock and wait ---------------------------------------------------------

type h06Clock struct {
	mu  sync.Mutex
	now time.Time
}

func newH06Clock() *h06Clock { return &h06Clock{now: time.Unix(1_000_000, 0).UTC()} }

func (c *h06Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *h06Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// h06Waits is the caller cadence: it advances the clock by the requested
// duration times num/den and records every request.
type h06Waits struct {
	mu       sync.Mutex
	clock    *h06Clock
	num, den int64
	asked    []time.Duration
}

func newH06Waits(clock *h06Clock) *h06Waits { return &h06Waits{clock: clock, num: 1, den: 1} }

func (w *h06Waits) Wait(ctx context.Context, d time.Duration) error {
	w.mu.Lock()
	w.asked = append(w.asked, d)
	num, den := w.num, w.den
	w.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	w.clock.Advance(time.Duration(int64(d) * num / den))
	return nil
}

func (w *h06Waits) Asked() []time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]time.Duration(nil), w.asked...)
}

// ---- scripted session -------------------------------------------------------

type h06Answer struct {
	words   []uint16
	page    *HuaweiIdentificationPage
	err     error
	elapsed time.Duration
	// entered is closed when the call starts; release must be closed before
	// the call answers.
	entered chan struct{}
	release chan struct{}
}

type h06Expected struct {
	function uint8
	unit     uint8
	offset   uint16
	quantity uint16
	code     uint8
	object   uint8
	answer   h06Answer
}

func (e h06Expected) String() string {
	if e.function == 0x2b {
		return fmt.Sprintf("FC2B unit %d code %d object 0x%02x", e.unit, e.code, e.object)
	}
	return fmt.Sprintf("FC03 unit %d %d/%d", e.unit, e.offset, e.quantity)
}

// h06Call is one request the provider handed to a session.
type h06Call struct {
	session string
	h06Expected
	pdu    []byte
	sentAt time.Time
	endAt  time.Time
}

type h06Recorder struct {
	mu    sync.Mutex
	calls []h06Call
}

func (r *h06Recorder) add(call h06Call) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
}

func (r *h06Recorder) Calls() []h06Call {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]h06Call(nil), r.calls...)
}

type h06Session struct {
	name     string
	clock    *h06Clock
	recorder *h06Recorder

	mu         sync.Mutex
	script     []h06Expected
	next       int
	closes     int
	unexpected []string
}

func newH06Session(name string, clock *h06Clock, recorder *h06Recorder, script ...h06Expected) *h06Session {
	return &h06Session{name: name, clock: clock, recorder: recorder, script: script}
}

func (s *h06Session) Append(script ...h06Expected) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.script = append(s.script, script...)
}

func (s *h06Session) Remaining() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.script) - s.next
}

func (s *h06Session) Closes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closes
}

func (s *h06Session) Unexpected() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.unexpected...)
}

func (s *h06Session) take(ctx context.Context, got h06Expected, pdu []byte) (h06Answer, bool) {
	s.mu.Lock()
	sentAt := s.clock.Now()
	if s.closes != 0 {
		s.unexpected = append(s.unexpected, "after close: "+got.String())
		s.mu.Unlock()
		return h06Answer{}, false
	}
	if s.next >= len(s.script) {
		s.unexpected = append(s.unexpected, "beyond the script: "+got.String())
		s.mu.Unlock()
		return h06Answer{}, false
	}
	want := s.script[s.next]
	s.next++
	if want.function != got.function || want.unit != got.unit || want.offset != got.offset || want.quantity != got.quantity ||
		want.code != got.code || want.object != got.object {
		s.unexpected = append(s.unexpected, fmt.Sprintf("request %d: got %s want %s", s.next, got, want))
		s.mu.Unlock()
		return h06Answer{}, false
	}
	s.mu.Unlock()
	answer := want.answer
	if answer.entered != nil {
		close(answer.entered)
	}
	if answer.release != nil {
		select {
		case <-answer.release:
		case <-ctx.Done():
		}
	}
	elapsed := answer.elapsed
	if elapsed == 0 {
		elapsed = 20 * time.Millisecond
		if errors.Is(answer.err, context.DeadlineExceeded) {
			elapsed = 5 * time.Second
		}
	}
	s.clock.Advance(elapsed)
	s.recorder.add(h06Call{session: s.name, h06Expected: got, pdu: pdu, sentAt: sentAt, endAt: s.clock.Now()})
	return answer, true
}

func (s *h06Session) ReadHolding(ctx context.Context, unit byte, request modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error) {
	pdu, err := request.EncodePDU()
	if err != nil {
		return modbus.ReadRegistersResponse{}, err
	}
	got := h06Expected{function: byte(request.Function()), unit: unit, offset: request.Offset(), quantity: request.Quantity()}
	answer, ok := s.take(ctx, got, pdu)
	if !ok {
		return modbus.ReadRegistersResponse{}, errors.New("scripted session: unexpected request")
	}
	if answer.err != nil {
		return modbus.ReadRegistersResponse{}, answer.err
	}
	return modbus.ReadRegistersResponse{
		Words:      append([]uint16(nil), answer.words...),
		Provenance: modbus.ReadProvenance{Function: request.Function(), Table: request.Table(), Offset: request.Offset(), Quantity: request.Quantity()},
	}, nil
}

func (s *h06Session) ReadDeviceIdentification(ctx context.Context, unit byte, request HuaweiIdentificationRequest) (HuaweiIdentificationPage, error) {
	pdu := []byte{0x2b, 0x0e, request.ReadDeviceIDCode, request.ObjectID}
	got := h06Expected{function: 0x2b, unit: unit, code: request.ReadDeviceIDCode, object: request.ObjectID}
	answer, ok := s.take(ctx, got, pdu)
	if !ok {
		return HuaweiIdentificationPage{}, errors.New("scripted session: unexpected request")
	}
	if answer.err != nil {
		return HuaweiIdentificationPage{}, answer.err
	}
	return answer.page.clone(), nil
}

func (s *h06Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closes++
	return nil
}

func h06Exception(function uint8, code uint8) error {
	return &modbus.ProtocolError{Kind: modbus.ErrorExceptionResponse, RequestedFunction: modbus.FunctionCode(function),
		ReceivedFunction: modbus.FunctionCode(function | 0x80), ExceptionCode: code, Offset: -1}
}

func h06Timeout() error { return fmt.Errorf("scripted attempt: %w", context.DeadlineExceeded) }

func h06FC03(unit uint8, offset, quantity uint16, answer h06Answer) h06Expected {
	return h06Expected{function: 0x03, unit: unit, offset: offset, quantity: quantity, answer: answer}
}

func h06FC2B(unit uint8, code, object uint8, answer h06Answer) h06Expected {
	return h06Expected{function: 0x2b, unit: unit, code: code, object: object, answer: answer}
}

// ---- provider-cycle fixtures -------------------------------------------------

type h06Reply struct {
	Kind         string   `json:"kind"`
	Words        []uint16 `json:"words,omitempty"`
	Code         uint8    `json:"code,omitempty"`
	Synthetic    bool     `json:"synthetic,omitempty"`
	ElapsedClass string   `json:"elapsed_class,omitempty"`
}

type h06Step struct {
	Function string   `json:"function"`
	Unit     uint8    `json:"unit"`
	Offset   uint16   `json:"offset,omitempty"`
	Quantity uint16   `json:"quantity,omitempty"`
	Code     uint8    `json:"code,omitempty"`
	Object   *h03Hex  `json:"object,omitempty"`
	Reply    h06Reply `json:"reply"`
}

type h06ExpectChild struct {
	Unit     uint8    `json:"unit"`
	Evidence string   `json:"evidence"`
	Reason   string   `json:"reason"`
	Retained []uint16 `json:"retained,omitempty"`
}

type h06Expect struct {
	Outcome  string           `json:"outcome"`
	Reason   string           `json:"reason"`
	Children []h06ExpectChild `json:"children"`
}

type h06Cycle struct {
	Name   string    `json:"name"`
	Steps  []h06Step `json:"steps"`
	Expect h06Expect `json:"expect"`
}

type h06Fixture struct {
	Schema       string `json:"schema"`
	Kind         string `json:"kind"`
	Path         string `json:"path"`
	ProviderID   string `json:"provider_id"`
	License      string `json:"license"`
	Provenance   string `json:"provenance"`
	Sanitization struct {
		Rule string `json:"rule"`
	} `json:"sanitization"`
	Sources      []string   `json:"sources"`
	AllowedUnits []int      `json:"allowed_units"`
	Cycles       []h06Cycle `json:"cycles"`
}

func h06FixtureNames() []string {
	return []string{"sdongle_provider_cycles.json", "smartlogger_provider_cycles.json"}
}

func h06Load(t testing.TB, name string) (h06Fixture, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h06FixtureDirectory, name))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var fixture h06Fixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("%s: trailing content", name)
	}
	return fixture, data
}

// h06Sources resolves "source" replies from the committed fixtures.
type h06Sources struct {
	smartLoggerInventory h03InventoryFixture
	smartLoggerBasic     h03BasicFixture
	routed               h03ProbeFixture
	sdongle              h15Fixtures
	m3, m1               h05Fixture
}

func h06LoadSources(t testing.TB) h06Sources {
	t.Helper()
	m3, _ := h05Load(t, "sun2000_m3_smartlogger_monitoring.json")
	m1, _ := h05Load(t, "sun2000_m1_sdongle_monitoring.json")
	return h06Sources{
		smartLoggerInventory: h03LoadInventory(t),
		smartLoggerBasic:     h03LoadBasic(t),
		routed:               h03LoadProbes(t, "sun2000_routed_identity.json"),
		sdongle:              h15LoadAll(t),
		m3:                   m3,
		m1:                   m1,
	}
}

func h06PageFromPDU(t testing.TB, pdu []byte) *HuaweiIdentificationPage {
	t.Helper()
	parsed, err := h03ParseMEI(pdu)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.more != 0x00 && parsed.more != 0xff {
		t.Fatalf("unexpected continuation byte %#x", parsed.more)
	}
	return &HuaweiIdentificationPage{
		ReadDeviceIDCode: parsed.readDeviceIDCode, Conformity: parsed.conformity, ResponseBytes: parsed.length,
		DeclaredObjects: int(parsed.declared), More: parsed.more == 0xff, NextObjectID: parsed.next, Objects: parsed.objects,
	}
}

func h06ProbeAnswer(t testing.TB, probe h03Probe) h06Answer {
	t.Helper()
	switch probe.Outcome.Type {
	case "exception":
		return h06Answer{err: h06Exception(0x03, probe.Outcome.Code)}
	case "no_response":
		return h06Answer{err: h06Timeout()}
	}
	return h06Answer{words: h15Words(t, probe)}
}

func h06MonitoringAnswer(t testing.TB, fixture h05Fixture, unit uint8, offset, quantity uint16) h06Answer {
	t.Helper()
	for _, reading := range fixture.Readings {
		if reading.Unit != unit {
			continue
		}
		for _, read := range reading.Reads {
			if read.Offset == offset && read.Quantity == quantity {
				if read.Outcome.Type == "exception" {
					return h06Answer{err: h06Exception(0x03, read.Outcome.Code)}
				}
				return h06Answer{words: append([]uint16(nil), read.Outcome.Words...)}
			}
		}
	}
	t.Fatalf("%s: no read at unit %d %d/%d", fixture.ProfileID, unit, offset, quantity)
	return h06Answer{}
}

func (s h06Sources) resolve(t testing.TB, path string, step h06Step) h06Answer {
	t.Helper()
	var object uint8
	if step.Object != nil {
		object = uint8(*step.Object)
	}
	switch path {
	case "SmartLogger":
		switch {
		case step.Function == "FC03" && step.Unit == 0 && step.Offset == huaweiSmartLoggerCounterOffset:
			return h06Answer{words: []uint16{s.smartLoggerInventory.ChangeCounter.Before}}
		case step.Function == "FC2B" && step.Unit == 0 && step.Code == 3:
			for _, page := range s.smartLoggerInventory.Pages {
				if uint8(page.RequestObjectID) == object {
					pdu, err := h03InventoryPDU(s.smartLoggerInventory.Read, page)
					if err != nil {
						t.Fatal(err)
					}
					return h06Answer{page: h06PageFromPDU(t, pdu)}
				}
			}
		case step.Function == "FC2B" && step.Unit == 0 && step.Code == 1:
			pdu, err := h03BasicPDU(s.smartLoggerBasic.Read, s.smartLoggerBasic.Reply)
			if err != nil {
				t.Fatal(err)
			}
			return h06Answer{page: h06PageFromPDU(t, pdu)}
		case step.Function == "FC03" && step.Offset == huaweiSUN2000ModelOffset:
			return h06ProbeAnswer(t, h15ProbeAt(t, s.routed, int(step.Unit), step.Offset))
		case step.Function == "FC03":
			return h06MonitoringAnswer(t, s.m3, step.Unit, step.Offset, step.Quantity)
		}
	case "S-Dongle":
		f := s.sdongle
		switch {
		case step.Function == "FC03" && step.Unit == 100:
			return h06ProbeAnswer(t, h15ProbeAt(t, f.Gateway, 100, step.Offset))
		case step.Function == "FC2B" && step.Unit == 100 && step.Code == 1:
			pdu, err := h15MEIExceptionPDU(f.GatewayBasic.Reply)
			if err != nil {
				t.Fatal(err)
			}
			code, err := h15ParseMEIException(pdu)
			if err != nil {
				t.Fatal(err)
			}
			return h06Answer{err: h06Exception(0x2b, code)}
		case step.Function == "FC03" && step.Unit == 1 && step.Offset == huaweiSUN2000ModelOffset:
			return h06ProbeAnswer(t, h15ProbeAt(t, f.ChildBlocks, 1, step.Offset))
		case step.Function == "FC2B" && step.Unit == 1 && step.Code == 1:
			pdu, err := h03BasicPDU(f.ChildBasic.Read, f.ChildBasic.Reply)
			if err != nil {
				t.Fatal(err)
			}
			return h06Answer{page: h06PageFromPDU(t, pdu)}
		case step.Function == "FC2B" && step.Unit == 1 && step.Code == 3:
			pdu, err := h03InventoryPDU(f.ChildInventory.Read, f.ChildInventory.Pages[0])
			if err != nil {
				t.Fatal(err)
			}
			return h06Answer{page: h06PageFromPDU(t, pdu)}
		case step.Function == "FC03" && step.Unit == 1:
			return h06MonitoringAnswer(t, s.m1, step.Unit, step.Offset, step.Quantity)
		}
	}
	t.Fatalf("%s: no source for %+v", path, step)
	return h06Answer{}
}

func h06ElapsedOf(class string) time.Duration {
	switch class {
	case "at_deadline":
		return 5 * time.Second
	case "within_deadline":
		return 400 * time.Millisecond
	}
	return 20 * time.Millisecond
}

// h06Script turns one fixture cycle into a session script.
func h06Script(t testing.TB, sources h06Sources, path string, cycle h06Cycle) []h06Expected {
	t.Helper()
	script := make([]h06Expected, 0, len(cycle.Steps))
	for _, step := range cycle.Steps {
		var answer h06Answer
		switch step.Reply.Kind {
		case "source":
			answer = sources.resolve(t, path, step)
		case "words":
			if !step.Reply.Synthetic {
				t.Fatalf("%s: a literal reply must be marked synthetic: %+v", cycle.Name, step)
			}
			answer = h06Answer{words: append([]uint16(nil), step.Reply.Words...)}
		case "exception":
			function := uint8(0x03)
			if step.Function == "FC2B" {
				function = 0x2b
			}
			answer = h06Answer{err: h06Exception(function, step.Reply.Code)}
		case "timeout":
			answer = h06Answer{err: h06Timeout()}
		case "transport":
			answer = h06Answer{err: errors.New("scripted peer reset after the request was written")}
		default:
			t.Fatalf("%s: unknown reply kind %q", cycle.Name, step.Reply.Kind)
		}
		if step.Reply.ElapsedClass != "" || answer.err == nil {
			answer.elapsed = h06ElapsedOf(step.Reply.ElapsedClass)
		}
		expected := h06Expected{unit: step.Unit, answer: answer}
		switch step.Function {
		case "FC03":
			expected.function, expected.offset, expected.quantity = 0x03, step.Offset, step.Quantity
		case "FC2B":
			expected.function, expected.code = 0x2b, step.Code
			if step.Object != nil {
				expected.object = uint8(*step.Object)
			}
		default:
			t.Fatalf("%s: unknown function %q", cycle.Name, step.Function)
		}
		script = append(script, expected)
	}
	return script
}

// ---- running a provider -------------------------------------------------------

type h06Provider interface {
	Start(HuaweiProviderSession) error
	Stop() error
	Reconnect(HuaweiProviderSession) error
	Close() error
	Refresh(context.Context) (HuaweiProviderCycle, error)
	LastComplete() (HuaweiProviderCycle, bool)
	NextRefreshAt() time.Time
	Pacing() HuaweiProviderPacing
}

var (
	_ h06Provider = (*HuaweiSmartLoggerReadOnlyProvider)(nil)
	_ h06Provider = (*HuaweiSDongleReadOnlyProvider)(nil)
)

type h06Rig struct {
	clock    *h06Clock
	waits    *h06Waits
	recorder *h06Recorder
	provider h06Provider
	session  *h06Session
	sources  h06Sources
	path     string
}

func h06Config(clock *h06Clock, waits *h06Waits, allowed []uint8) HuaweiProviderConfig {
	return HuaweiProviderConfig{Source: "endpoint-a", AllowedUnits: allowed, Clock: clock.Now, Wait: waits.Wait}
}

func newH06Rig(t testing.TB, path string, allowed []uint8) *h06Rig {
	t.Helper()
	rig := &h06Rig{clock: newH06Clock(), recorder: &h06Recorder{}, sources: h06LoadSources(t), path: path}
	rig.waits = newH06Waits(rig.clock)
	config := h06Config(rig.clock, rig.waits, allowed)
	var err error
	switch path {
	case "SmartLogger":
		rig.provider, err = NewHuaweiSmartLoggerReadOnlyProvider(config)
	case "S-Dongle":
		rig.provider, err = NewHuaweiSDongleReadOnlyProvider(config)
	default:
		t.Fatalf("unknown path %s", path)
	}
	if err != nil {
		t.Fatalf("construct %s provider: %v", path, err)
	}
	rig.session = newH06Session("generation-1", rig.clock, rig.recorder)
	if err := rig.provider.Start(rig.session); err != nil {
		t.Fatal(err)
	}
	return rig
}

func h06Units(values []int) []uint8 {
	out := make([]uint8, len(values))
	for index, value := range values {
		out[index] = uint8(value)
	}
	return out
}

// refresh advances the clock to the next permitted cycle start, as a caller's
// cadence would, and runs one cycle that must consume the whole script.
func (rig *h06Rig) refresh(t testing.TB, script ...h06Expected) HuaweiProviderCycle {
	t.Helper()
	rig.session.Append(script...)
	if next := rig.provider.NextRefreshAt(); !next.IsZero() {
		if now := rig.clock.Now(); now.Before(next) {
			rig.clock.Advance(next.Sub(now))
		}
	}
	cycle, err := rig.provider.Refresh(context.Background())
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if unexpected := rig.session.Unexpected(); len(unexpected) != 0 {
		t.Fatalf("unexpected requests: %v", unexpected)
	}
	if remaining := rig.session.Remaining(); remaining != 0 {
		t.Fatalf("%d scripted requests were not sent", remaining)
	}
	return cycle
}

func h06RequireExpect(t testing.TB, name string, cycle HuaweiProviderCycle, expect h06Expect) {
	t.Helper()
	if string(cycle.Outcome) != expect.Outcome || string(cycle.Reason) != expect.Reason {
		t.Fatalf("%s: outcome=%s reason=%s want %s %s", name, cycle.Outcome, cycle.Reason, expect.Outcome, expect.Reason)
	}
	if len(cycle.Children) != len(expect.Children) {
		t.Fatalf("%s: %d children, want %d: %+v", name, len(cycle.Children), len(expect.Children), h06ChildSummary(cycle))
	}
	for index, want := range expect.Children {
		got := cycle.Children[index]
		if got.UnitID != want.Unit || string(got.Evidence) != want.Evidence || string(got.Reason) != want.Reason {
			t.Fatalf("%s: child %d = unit %d %s %s, want unit %d %s %s", name, index, got.UnitID, got.Evidence, got.Reason, want.Unit, want.Evidence, want.Reason)
		}
		var retained []uint16
		for _, group := range got.Groups {
			if group.Retained {
				retained = append(retained, group.Offset)
			}
		}
		if fmt.Sprint(retained) != fmt.Sprint(append([]uint16(nil), want.Retained...)) {
			t.Fatalf("%s: unit %d retained groups=%v want %v", name, got.UnitID, retained, want.Retained)
		}
	}
}

func h06ChildSummary(cycle HuaweiProviderCycle) []string {
	var out []string
	for _, child := range cycle.Children {
		out = append(out, fmt.Sprintf("%d:%s:%s", child.UnitID, child.Evidence, child.Reason))
	}
	return out
}

// h06RunFixture replays every cycle of a provider-cycle fixture and returns
// the cycles and the rig.
func h06RunFixture(t testing.TB, name string) (*h06Rig, []HuaweiProviderCycle) {
	t.Helper()
	fixture, _ := h06Load(t, name)
	rig := newH06Rig(t, fixture.Path, h06Units(fixture.AllowedUnits))
	var cycles []HuaweiProviderCycle
	for _, cycle := range fixture.Cycles {
		out := rig.refresh(t, h06Script(t, rig.sources, fixture.Path, cycle)...)
		h06RequireExpect(t, fixture.Path+"/"+cycle.Name, out, cycle.Expect)
		cycles = append(cycles, out)
	}
	return rig, cycles
}
