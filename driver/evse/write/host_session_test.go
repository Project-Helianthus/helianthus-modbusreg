package write

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/driver/evse/reader"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

// hostStamp rewrites what a driver declares, registers and publishes the way
// the SemReg host session does: its own source, epoch and generation replace
// the driver's, and every identifier the driver chose gets a suffix of the
// digest of the epoch.
type hostStamp struct {
	source     semreg.SourceID
	epoch      semreg.SourceEpochID
	generation semreg.Uint64
	suffix     string
}

func newHostStamp() *hostStamp {
	return &hostStamp{source: "source:session", epoch: "epoch:session", generation: "9", suffix: "@1a2b3c4d"}
}

func (h *hostStamp) route(r driver.SettingRoute) driver.SettingRoute {
	r.Route.CapabilityInstance += semreg.CapabilityInstanceID(h.suffix)
	r.Route.ServiceInstance += semreg.ServiceInstanceID(h.suffix)
	r.Route.BindingID += semreg.NativeBindingID(h.suffix)
	r.Route.SourceID, r.Route.SourceEpochID, r.Route.DriverGeneration = h.source, h.epoch, h.generation
	return r
}

func (h *hostStamp) stamp(b semreg.PublicationBatch) (semreg.PublicationBatch, error) {
	b.SourceID, b.SourceEpochID, b.DriverGeneration = h.source, h.epoch, h.generation
	binding := func(id semreg.NativeBindingID) semreg.NativeBindingID { return id + semreg.NativeBindingID(h.suffix) }
	b.SourceUpserts = append([]semreg.SourceDescriptor{}, b.SourceUpserts...)
	for i := range b.SourceUpserts {
		b.SourceUpserts[i].SourceID, b.SourceUpserts[i].SourceEpochID = h.source, h.epoch
	}
	b.BindingUpserts = append([]semreg.NativeBinding{}, b.BindingUpserts...)
	for i := range b.BindingUpserts {
		n := &b.BindingUpserts[i]
		n.SourceID, n.SourceEpochID, n.DriverGeneration, n.BindingID = h.source, h.epoch, h.generation, binding(n.BindingID)
	}
	b.IdentityLinkUpserts = append([]semreg.IdentityLink{}, b.IdentityLinkUpserts...)
	for i := range b.IdentityLinkUpserts {
		b.IdentityLinkUpserts[i].BindingID = binding(b.IdentityLinkUpserts[i].BindingID)
	}
	b.ServiceUpserts = append([]semreg.ServiceInstance{}, b.ServiceUpserts...)
	for i := range b.ServiceUpserts {
		n := &b.ServiceUpserts[i]
		n.SourceEpochID, n.DriverGeneration = h.epoch, h.generation
		n.InstanceID += semreg.ServiceInstanceID(h.suffix)
		n.BindingID = binding(n.BindingID)
	}
	b.CapabilityUpserts = append([]semreg.CapabilityInstance{}, b.CapabilityUpserts...)
	for i := range b.CapabilityUpserts {
		n := &b.CapabilityUpserts[i]
		n.SourceEpochID, n.DriverGeneration = h.epoch, h.generation
		n.InstanceID += semreg.CapabilityInstanceID(h.suffix)
		n.ServiceInstance += semreg.ServiceInstanceID(h.suffix)
		n.BindingID = binding(n.BindingID)
	}
	b.FactUpserts = append([]semreg.FactCandidate{}, b.FactUpserts...)
	for i := range b.FactUpserts {
		f := &b.FactUpserts[i]
		f.CandidateID += semreg.CandidateID(h.suffix)
		if f.BindingID != nil {
			id := binding(*f.BindingID)
			f.BindingID = &id
		}
		if f.Origin.BindingID != nil {
			id := binding(*f.Origin.BindingID)
			f.Origin.BindingID = &id
		}
		if f.SourceEpochID != nil {
			epoch := h.epoch
			f.SourceEpochID = &epoch
		}
		if f.DriverGeneration != nil {
			generation := h.generation
			f.DriverGeneration = &generation
		}
		if f.Origin.SourceID != nil {
			source := h.source
			f.Origin.SourceID = &source
		}
		if f.Origin.SourceEpochID != nil {
			epoch := h.epoch
			f.Origin.SourceEpochID = &epoch
		}
	}
	sort.SliceStable(b.BindingUpserts, func(i, j int) bool { return b.BindingUpserts[i].BindingID < b.BindingUpserts[j].BindingID })
	sort.SliceStable(b.IdentityLinkUpserts, func(i, j int) bool { return b.IdentityLinkUpserts[i].BindingID < b.IdentityLinkUpserts[j].BindingID })
	sort.SliceStable(b.FactUpserts, func(i, j int) bool { return b.FactUpserts[i].CandidateID < b.FactUpserts[j].CandidateID })
	sort.SliceStable(b.ServiceUpserts, func(i, j int) bool { return b.ServiceUpserts[i].InstanceID < b.ServiceUpserts[j].InstanceID })
	sort.SliceStable(b.CapabilityUpserts, func(i, j int) bool { return b.CapabilityUpserts[i].InstanceID < b.CapabilityUpserts[j].InstanceID })
	digest, err := b.ComputedDigest()
	if err != nil {
		return b, err
	}
	b.BatchDigest = digest
	return b, nil
}

// hostDispatch is the dispatch the router builds for the route the session
// registered: the session's own identifiers, the route it admitted and the
// guard drawn from it.
func (r *rig) hostDispatch(args ...semreg.TypedField) driver.Dispatch {
	r.t.Helper()
	live := r.session.live()
	if len(live) != 1 {
		r.t.Fatalf("executors %d", len(live))
	}
	route := live[0].route.Route
	return driver.Dispatch{
		Request: driver.OperationRequest{AssetID: r.pub.Asset, BindingID: route.BindingID, CapabilityInstanceID: route.CapabilityInstance, Operation: definition(OperationID), Arguments: args},
		Route:   route,
		Guard: operation.GuardClaims{AssetID: r.pub.Asset, ExpectedCapabilityRevision: "1", CapabilityInstance: route.CapabilityInstance, ExpectedCapabilityInstanceRevision: "1",
			ServiceInstance: route.ServiceInstance, BindingID: route.BindingID, SourceID: route.SourceID, SourceEpochID: route.SourceEpochID, DriverGeneration: route.DriverGeneration},
	}
}

func (r *rig) hostExecute(args ...semreg.TypedField) driver.Completion {
	r.t.Helper()
	r.clk.set(unix(1001))
	completion, err := r.live().exec.Execute(ctxb(), r.hostDispatch(args...))
	if err != nil {
		r.t.Fatal(err)
	}
	if err := completion.Validate(); err != nil {
		r.t.Fatalf("the completion is not valid: %v", err)
	}
	return completion
}

func (r *rig) live() *registered { return r.session.live()[0] }

// A dispatch the way the host session hands it over, with the identifiers it
// scoped and the ownership it stamped, is executed: the set is sent and read
// back, and the readback names the candidate the session published.
func TestAHostShapedDispatchIsExecutedAndItsCandidateFound(t *testing.T) {
	r := newRigOn(t, newHostStamp())
	route := r.live().route.Route
	if route.SourceEpochID != "epoch:session" || route.CapabilityInstance == r.pub.CapabilityInstanceIdentifier(reader.SetAllocated) {
		t.Fatalf("the session did not rewrite the route: %+v", route)
	}
	r.setFor(32, device(16, 900, false))
	c := r.hostExecute(quantityArg(amperes("16", 0)))
	if c.Outcome != operation.OutcomeApplied || c.Readback == nil || c.ReadbackSnapshot == nil || c.Readback.Relation != operation.ReadbackConfirms {
		t.Fatalf("outcome %s readback %+v", c.Outcome, c.Readback)
	}
	r.wantLog("t27", "t25(16,900,false)", "t27")
	candidate, ok := r.session.candidate(reader.ElementAllocatedCurrent)
	if !ok || c.Readback.CandidateID != candidate.CandidateID || candidate.CandidateID == r.pub.ConnectorCandidateIdentifier(reader.ElementAllocatedCurrent) {
		t.Fatalf("the readback names %s, the session published %s", c.Readback.CandidateID, candidate.CandidateID)
	}
}

func TestAHostShapedReleaseIsExecuted(t *testing.T) {
	r := newRigOn(t, newHostStamp())
	r.setFor(32, device(32, 900, false))
	c := r.hostExecute(booleanArg(false))
	if c.Outcome != operation.OutcomeAcknowledgedUnverified || c.Acknowledgement == nil {
		t.Fatalf("outcome %s", c.Outcome)
	}
	r.wantLog("t27", "t25(32,900,false)", "t27")
}

// The guard and the route are the session's: ownership or an identifier of
// another epoch than the session's, and a suffix that is not the session's
// digest form, send nothing.
func TestAHostShapedDispatchOfAnotherOwnerSendsNothing(t *testing.T) {
	cases := map[string]func(*driver.Dispatch){
		"another epoch":      func(d *driver.Dispatch) { d.Guard.SourceEpochID = "epoch:synthetic-charger" },
		"another generation": func(d *driver.Dispatch) { d.Guard.DriverGeneration = "7" },
		"another source":     func(d *driver.Dispatch) { d.Guard.SourceID = "source:other" },
		"another asset":      func(d *driver.Dispatch) { d.Guard.AssetID = "asset:other" },
		"another binding":    func(d *driver.Dispatch) { d.Guard.BindingID += "x" },
		"a malformed suffix": func(d *driver.Dispatch) { d.Guard.CapabilityInstance += "@zz" },
		"another capability": func(d *driver.Dispatch) { d.Route.CapabilityInstance = "capability:other@1a2b3c4d" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRigOn(t, newHostStamp())
			r.seed(idle(32), 990)
			d := r.hostDispatch(quantityArg(amperes("16", 0)))
			mutate(&d)
			c, err := r.live().exec.Execute(ctxb(), d)
			if err != nil || c.Outcome != operation.OutcomeFailedNoContact {
				t.Fatalf("outcome %s err %v", c.Outcome, err)
			}
			r.wantLog()
		})
	}
}

// A writer whose start failed registers nothing and sends nothing.
func TestAWriterWhoseStartFailedSendsNothing(t *testing.T) {
	r := newRig(t)
	failing := &fakeSession{t: t, pub: r.pub, routeErr: errors.New("begin failed")}
	w, err := NewWriter(Config{Reader: r.reader, Publication: r.pub, ClockEpoch: "clock:writer-failed", Clock: r.clk})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Stop(context.Background()) })
	if err := w.Start(context.Background(), failing); err == nil {
		t.Fatal("start succeeded")
	}
	r.seed(idle(32), 990)
	r.clk.set(unix(1001))
	if result := w.ApplyAllocatedCurrent(ctxb(), r.guard(), amperes("16", 0)); result.Level != LevelFailedNoContact || result.Reason != ReasonStopped {
		t.Fatalf("%s %q", result.Level, result.Reason)
	}
	r.wantLog()
	if len(failing.live()) != 0 {
		t.Fatal("an executor is registered")
	}
}
