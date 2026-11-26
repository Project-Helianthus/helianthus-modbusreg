package write

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	"github.com/Project-Helianthus/helianthus-modbusreg/driver/evse/reader"
	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

var okBody = []byte{0x08, 0x01, 0x7a, 0x02, 0xaa, 0xbb}

// rig is one configured charger: the scripted endpoint, the fake clock, the
// reader and the writer on one session.
type rig struct {
	t       *testing.T
	clk     *fakeClock
	ep      *scriptedEndpoint
	reader  *reader.ChargerReader
	writer  *Writer
	session *fakeSession
	pub     reader.ChargerPublication
	base    int
}

func newRig(t *testing.T, mutate ...func(*reader.ChargerConfig)) *rig {
	t.Helper()
	return newRigOn(t, nil, mutate...)
}

// newRigOn is a rig whose session stamps like the SemReg host session when host
// is set.
func newRigOn(t *testing.T, host *hostStamp, mutate ...func(*reader.ChargerConfig)) *rig {
	t.Helper()
	r := &rig{t: t, clk: &fakeClock{at: time.Unix(5, 0)}, pub: syntheticPublication()}
	cfg := syntheticConfig()
	for _, m := range mutate {
		m(&cfg)
	}
	deps := reader.ChargerDeps{Activation: activation{}, Now: r.clk.Now,
		Open: func(config modbus.RTUProductionConfig) (tesla.Gen3ReadEndpoint, error) {
			r.ep = &scriptedEndpoint{config: config, generation: 1}
			r.ep.push(step{echo: true, terminal: terminal(4, 3, okBody)})
			return r.ep, nil
		}}
	rd, err := reader.NewChargerReader(cfg, r.pub, deps)
	if err != nil || rd == nil {
		t.Fatalf("reader %v err %v", rd, err)
	}
	r.reader = rd
	r.session = &fakeSession{t: t, pub: r.pub, host: host}
	if err := rd.Start(context.Background(), r.session); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rd.Stop(context.Background()) })
	w, err := NewWriter(Config{Reader: rd, Publication: r.pub, ClockEpoch: "clock:writer", Clock: r.clk})
	if err != nil {
		t.Fatal(err)
	}
	r.writer = w
	if err := w.Start(context.Background(), r.session); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Stop(context.Background()) })
	waitFor(t, "the first poll", func() bool { p, _ := rd.Payload(reader.SlotCommonSystemInfo); return p.Present })
	r.base = len(r.ep.sent())
	return r
}

func (r *rig) guard() operation.GuardClaims {
	p := r.pub
	return operation.GuardClaims{
		AssetID: p.Asset, ExpectedCapabilityRevision: "1", CapabilityInstance: p.CapabilityInstanceIdentifier(reader.SetAllocated),
		ExpectedCapabilityInstanceRevision: "1", ServiceInstance: p.ServiceInstanceIdentifier(reader.SetAllocated),
		BindingID: p.BindingIdentifier(), SourceID: p.Source, SourceEpochID: p.Epoch, DriverGeneration: "7",
	}
}

func payloadOf(t *testing.T, op tesla.FC100Operation, body []byte) string {
	t.Helper()
	built, err := tesla.BuildTeslaFC100OperationRequest(tesla.HSCFC100OperationCompatibilityV1, op, body)
	if err != nil {
		t.Fatal(err)
	}
	return string(built.Payload())
}

func (r *rig) setPayload(limit, timeout uint32, inhibit bool) string {
	r.t.Helper()
	body, err := tesla.EncodeTeslaProvisionalCurrentLimitBody(tesla.ProvisionalCurrentLimitRequest{LimitCurrentMaxAmps: limit, LimitTimeoutSeconds: timeout, InhibitCharging: inhibit})
	if err != nil {
		r.t.Fatal(err)
	}
	return payloadOf(r.t, tesla.FC100OperationWCSetProvisional, body)
}

// log names every request the peer was handed since the rig started.
func (r *rig) log() []string {
	var out []string
	known := map[string]string{
		payloadOf(r.t, tesla.FC100OperationWCGetProvisional, nil): "t27",
		payloadOf(r.t, tesla.FC100OperationCommonSystemInfo, nil): "poll",
	}
	for _, limit := range []uint32{6, 8, 12, 16, 20, 24, 32} {
		for _, inhibit := range []bool{false, true} {
			known[r.setPayload(limit, 900, inhibit)] = fmt.Sprintf("t25(%d,900,%t)", limit, inhibit)
		}
	}
	for _, payload := range r.ep.sent()[r.base:] {
		name, ok := known[string(payload)]
		if !ok {
			name = fmt.Sprintf("other:%x", payload)
		}
		out = append(out, name)
	}
	return out
}

func (r *rig) wantLog(want ...string) {
	r.t.Helper()
	got := r.log()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		r.t.Fatalf("the peer was handed %v, want %v", got, want)
	}
}

func (r *rig) readback(s scalars, wall int64) step {
	return step{echo: true, terminal: terminal(6, 28, readbackBody(s)), wall: wall}
}

func (r *rig) ack(wall int64) step { return step{terminal: terminal(6, 26, nil), wall: wall} }

// idle is a t28 of a device with no limit set: the default limit of its
// variant and the configured current, without a timeout or an inhibit.
func idle(configured uint32) scalars {
	return scalars{limit: u(32), configured: u(configured)}
}

// configuredAs is the t28 with another configured current.
func configuredAs(s scalars, configured uint32) scalars {
	s.configured = u(configured)
	return s
}

// device is a t28 of a device configured for 32 A with the given limit.
func device(limit, timeout uint32, inhibit bool) scalars {
	return scalars{limit: u(limit), timeout: u(timeout), inhibit: b(inhibit), configured: u(32)}
}

// exchange scripts one t25 → t26, t27 → t28 with the returned state.
func (r *rig) exchange(returned scalars, setWall, readWall int64) {
	r.ep.push(r.ack(setWall), r.readback(returned, readWall))
}

// seed has the reader read and feed one t28, so the generation has one.
func (r *rig) seed(s scalars, wall int64) {
	r.t.Helper()
	r.ep.push(r.readback(s, wall))
	record, err := r.reader.Acquisition().Acquire(context.Background(), tesla.FC100OperationWCGetProvisional)
	if err != nil || record.State != tesla.Gen3ReadEvidenceAck {
		r.t.Fatalf("seed: %v %+v", err, record.State)
	}
	if err := r.reader.IngestPolled(context.Background(), record); err != nil {
		r.t.Fatal(err)
	}
	r.base = len(r.ep.sent())
}

func amperes(coefficient string, exponent int32) semreg.Quantity {
	return semreg.Quantity{Number: semreg.Decimal{Coefficient: coefficient, Exponent10: exponent}, Unit: UnitAmpere}
}

func (r *rig) apply(q semreg.Quantity) Result {
	r.t.Helper()
	r.clk.set(time.Unix(1001, 0))
	return r.writer.ApplyAllocatedCurrent(context.Background(), r.guard(), q)
}

func (r *rig) dispatch(args ...semreg.TypedField) driver.Dispatch {
	p := r.pub
	op := definition(OperationID)
	return driver.Dispatch{
		Request: driver.OperationRequest{AssetID: p.Asset, BindingID: p.BindingIdentifier(), CapabilityInstanceID: p.CapabilityInstanceIdentifier(reader.SetAllocated), Operation: op, Arguments: args},
		Route: operation.Route{CapabilityInstance: p.CapabilityInstanceIdentifier(reader.SetAllocated), ServiceInstance: p.ServiceInstanceIdentifier(reader.SetAllocated),
			BindingID: p.BindingIdentifier(), SourceID: p.Source, SourceEpochID: p.Epoch, DriverGeneration: "7"},
		Guard: r.guard(),
	}
}

func quantityArg(q semreg.Quantity) semreg.TypedField {
	return semreg.TypedField{ID: ArgumentID, Value: semreg.Value{Kind: semreg.ValueQuantity, Quantity: &q}}
}

func booleanArg(v bool) semreg.TypedField {
	return semreg.TypedField{ID: ArgumentID, Value: semreg.Value{Kind: semreg.ValueBoolean, Boolean: &v}}
}

type guardClaims = operation.GuardClaims

func unix(sec int64) time.Time { return time.Unix(sec, 0) }

func ctxb() context.Context { return context.Background() }

var errCapture = errors.New("no later snapshot")
