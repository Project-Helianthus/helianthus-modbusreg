package limits

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/huawei"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime/life"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/catalog"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

const driverKind driver.DriverKind = "modbus"

type stepClock struct {
	mu   sync.Mutex
	now  time.Time
	mono uint64
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *stepClock) Sample() (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mono++
	return c.mono, nil
}

func (c *stepClock) Anchor(sample uint64) ([]byte, error) { return []byte{byte(sample)}, nil }

func (c *stepClock) Elapsed(anchor []byte, sample uint64) (uint64, error) { return sample, nil }

type recordedWrite struct {
	write huawei.ControlWrite
	words []uint16
}

type scriptImage struct {
	mu     sync.Mutex
	writes []recordedWrite
	bytes  int
	mode   string
}

func (s *scriptImage) Write(_ context.Context, write huawei.ControlWrite, words []uint16) ([]uint16, error) {
	if !huawei.AdmitControlWrite(write) {
		return nil, errRefused
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bytes += len(words) * 2
	copied := append([]uint16(nil), words...)
	s.writes = append(s.writes, recordedWrite{write: write, words: copied})
	switch s.mode {
	case "error":
		return nil, errNoRead
	case "mismatch":
		return []uint16{0, 1}, nil
	default:
		return copied, nil
	}
}

type imageError string

func (e imageError) Error() string { return string(e) }

const (
	errRefused imageError = "refused"
	errNoRead  imageError = "no read"
)

type familyFactory struct{ family *Family }

func (familyFactory) Descriptor() driver.Descriptor { return driver.Descriptor{} }

func (familyFactory) Plan(c driver.Config) ([]driver.InstancePlan, error) {
	return []driver.InstancePlan{{Driver: driverKind, Instance: driver.InstanceKey(c.Instance)}}, nil
}

func (f familyFactory) Construct(context.Context, driver.InstancePlan) (driver.Instance, error) {
	return f.family, nil
}

func openFamily(t *testing.T, family *Family) {
	t.Helper()
	ctx := context.Background()
	clock := &stepClock{now: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}
	cfg := life.InstanceConfig{Driver: driverKind, Instance: "limits.dongle", Enabled: true, Config: driver.Config{Instance: "limits.dongle", Enabled: true}}
	rt, err := runtime.OpenHost(ctx, life.Installation{
		Key: "installation.dongle", Bridge: "bridge.test", StateDir: t.TempDir(), Clock: clock,
		Instances: []life.InstanceConfig{cfg},
	}, runtime.HostOptions{}, familyFactory{family})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close(context.Background()) })
	if err := rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := rt.WaitInitial(ctx); err != nil {
		t.Fatal(err)
	}
	family.mu.Lock()
	err = family.startErr
	family.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
}

type operationSpec struct {
	id     string
	offset uint16
	words  []uint16
	limit  string
}

func operations() []operationSpec {
	return []operationSpec{
		{id: huawei.OpSetChargeLimit, offset: 47247, words: []uint16{0, 0x09C4}, limit: "2500"},
		{id: huawei.OpSetDischargeLimit, offset: 47249, words: []uint16{0, 0x0708}, limit: "1800"},
	}
}

func newFamily(image *scriptImage) *Family {
	return &Family{
		Card: "MA-V200", Unit: 1, Path: "S-Dongle",
		ChargeConstraint: "2500", DischargeConstraint: "1800", Image: image,
	}
}

type dispatchCase struct {
	name   string
	mode   string
	hold   bool
	prior  bool
	retire bool
	mutate func(*driver.OperationRequest)
	want   operation.Outcome
	writes int
}

func TestDongleStorageLimitDispatch(t *testing.T) {
	if VerificationLevel != catalog.VerificationNotTestedOnHardware {
		t.Fatalf("verification %q", VerificationLevel)
	}
	cases := []dispatchCase{
		{name: "applied", want: operation.OutcomeApplied, writes: 1},
		{name: "local-comparison", hold: true, want: operation.OutcomeAcknowledgedUnverified, writes: 1},
		{name: "retired-generation", prior: true, want: operation.OutcomeAcknowledgedUnverified, writes: 1},
		{name: "acknowledgement", mode: "error", want: operation.OutcomeAcknowledgedUnverified, writes: 1},
		{name: "conflict", mode: "mismatch", want: operation.OutcomeConflict, writes: 1},
		{name: "outside-limit", mutate: negativeWatts, want: operation.OutcomeRejected, writes: 0},
		{name: "stale-guard", mutate: staleGeneration, want: operation.OutcomeRejected, writes: 0},
		{name: "undeclared-predicate", mutate: extraDuration, want: operation.OutcomeRejected, writes: 0},
		{name: "retired-route", retire: true, want: operation.OutcomeFailedNoContact, writes: 0},
	}
	for _, op := range operations() {
		op := op
		for _, tc := range cases {
			tc := tc
			t.Run(op.id+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				image := &scriptImage{mode: tc.mode}
				family := newFamily(image)
				openFamily(t, family)
				if tc.hold || tc.prior || tc.retire {
					family.mu.Lock()
					family.hold, family.prior, family.retired = tc.hold, tc.prior, tc.retire
					family.mu.Unlock()
				}
				result := invoke(t, family, op.id, tc.mutate)
				if result.KernelOutcome != tc.want {
					t.Fatalf("outcome %s delivery %s ack %s relation %s reason %s", result.KernelOutcome, result.Delivery, result.Acknowledgement, result.ReadbackRelation, result.Reason)
				}
				if tc.want == operation.OutcomeApplied && (result.Status != "readback_confirmed" || result.ReadbackRelation != operation.ReadbackConfirms) {
					t.Fatalf("applied status %s relation %s", result.Status, result.ReadbackRelation)
				}
				if tc.want == operation.OutcomeAcknowledgedUnverified && result.KernelOutcome == operation.OutcomeApplied {
					t.Fatal("local evidence confirmed")
				}
				if tc.want == operation.OutcomeConflict && result.ReadbackRelation != operation.ReadbackContradicts {
					t.Fatalf("conflict relation %s", result.ReadbackRelation)
				}
				if (tc.want == operation.OutcomeRejected || tc.want == operation.OutcomeFailedNoContact) && result.Delivery != operation.DeliveryNotSent {
					t.Fatalf("delivery %s", result.Delivery)
				}
				if tc.want == operation.OutcomeFailedNoContact && result.KernelOutcome == operation.OutcomeRejected {
					t.Fatal("retired route rejected")
				}
				image.mu.Lock()
				defer image.mu.Unlock()
				if len(image.writes) != tc.writes || (tc.writes == 0 && image.bytes != 0) {
					t.Fatalf("writes %d bytes %d", len(image.writes), image.bytes)
				}
				if tc.writes == 1 {
					assertWrite(t, image.writes[0], op)
				}
			})
		}
	}
}

// assertWrite checks every field of the bound write. A production change of
// card, path, unit, function, address or quantity fails that field's comparison.
func assertWrite(t *testing.T, got recordedWrite, op operationSpec) {
	t.Helper()
	if got.write.Card != "MA-V200" || got.write.Path != "S-Dongle" || got.write.Unit != 1 || got.write.IdentifiedUnit != 1 {
		t.Fatalf("identity %#v", got.write)
	}
	if got.write.Function != huawei.FunctionWriteMultipleRegisters || got.write.Address != op.offset || got.write.Quantity != 2 {
		t.Fatalf("register %#v", got.write)
	}
	if len(got.words) != 2 || got.words[0] != op.words[0] || got.words[1] != op.words[1] {
		t.Fatalf("words %v", got.words)
	}
}

func negativeWatts(req *driver.OperationRequest) {
	req.Arguments[0].Value = semreg.Value{Kind: semreg.ValueQuantity, Quantity: &semreg.Quantity{Number: semreg.Decimal{Coefficient: "-1"}, Unit: unitWatt}}
}

func staleGeneration(req *driver.OperationRequest) {
	req.ExpectedDriverGeneration = "99"
}

func extraDuration(req *driver.OperationRequest) {
	req.Arguments = append(req.Arguments, semreg.TypedField{
		ID: "storage.setpoint.duration", Value: semreg.Value{Kind: semreg.ValueQuantity, Quantity: &semreg.Quantity{Number: semreg.Decimal{Coefficient: "1"}, Unit: "unit.second"}},
	})
}

func invoke(t *testing.T, family *Family, operationID string, mutate func(*driver.OperationRequest)) driver.OperationResult {
	t.Helper()
	ctx := context.Background()
	cut, err := family.session.SemanticInput().Capture(ctx, driver.Filter{})
	if err != nil || len(cut.Snapshots) == 0 {
		t.Fatalf("capture %v", err)
	}
	req := requestFor(t, cut, operationID)
	if mutate != nil {
		mutate(&req)
	}
	result, err := family.session.ScopedOperations().Invoke(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func requestFor(t *testing.T, cut driver.SemanticCut, operationID string) driver.OperationRequest {
	t.Helper()
	var route driver.SettingRoute
	found := false
	for _, item := range cut.SettingRoutes {
		if string(item.Operation.ID) == operationID {
			route = item
			found = true
		}
	}
	if !found {
		t.Fatalf("no route %s", operationID)
	}
	var snap semreg.Snapshot
	var capability semreg.CapabilityInstance
	for _, item := range cut.Snapshots {
		for _, candidate := range item.Capabilities {
			if candidate.InstanceID == route.Route.CapabilityInstance {
				snap = item
				capability = candidate
			}
		}
	}
	if capability.InstanceID == "" || len(capability.Constraints) != 1 {
		t.Fatalf("capability %#v", capability)
	}
	if route.Limits == nil || route.Limits.Min == nil || route.Limits.Max != nil || route.Limits.Origin != driver.LimitSpecification {
		t.Fatalf("limits %#v", route.Limits)
	}
	if route.Limits.Min.Number.Coefficient != "0" || route.Limits.Min.Unit != unitWatt {
		t.Fatalf("min %#v", route.Limits.Min)
	}
	return driver.OperationRequest{
		AssetID: snap.AssetID, BindingID: route.Route.BindingID, CapabilityInstanceID: capability.InstanceID,
		Operation: route.Operation, Arguments: []semreg.TypedField{{ID: route.For.Key.FactID, Value: capability.Constraints[0].Value}},
		ExpectedSemanticRevision: snap.Revisions.Semantic, ExpectedCapabilityRevision: snap.Revisions.Capabilities,
		ExpectedCapabilityInstanceRevision: capability.Revision, ExpectedDriverGeneration: route.Route.DriverGeneration,
		ExpectedSourceEpochID: route.Route.SourceEpochID, Preconditions: []operation.Precondition{}, Timeout: time.Second,
	}
}

func TestDongleStorageLimitGraphsMatchAcrossLifecycles(t *testing.T) {
	first := graphOf(t)
	second := graphOf(t)
	if len(first) != 2 || len(first) != len(second) {
		t.Fatalf("graphs %d %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("graph %s != %s", first[i], second[i])
		}
	}
}

type graphShape struct {
	operation, capability, argument, coefficient string
	exponent                                     int32
	qualified, available                         bool
	evidence                                     string
	min                                          string
	maxAbsent                                    bool
	resource                                     string
}

func (g graphShape) String() string {
	return g.operation + " " + g.capability + " " + g.argument + " " + g.coefficient
}

func graphOf(t *testing.T) []string {
	t.Helper()
	family := newFamily(&scriptImage{})
	openFamily(t, family)
	ctx := context.Background()
	cut, err := family.session.SemanticInput().Capture(ctx, driver.Filter{})
	if err != nil || len(cut.Snapshots) == 0 {
		t.Fatalf("capture %v", err)
	}
	var out []string
	for _, snap := range cut.Snapshots {
		kinds := map[semreg.ResourceID]semreg.DefinitionID{}
		for _, resource := range snap.Resources {
			kinds[resource.ResourceID] = resource.Kind
		}
		for _, capability := range snap.Capabilities {
			if capability.Qualification != semreg.QualificationQualified || capability.Availability != semreg.AvailabilityAvailable {
				t.Fatalf("capability publication %#v", capability)
			}
			if len(capability.ActivationEvidence) != 1 || capability.ActivationEvidence[0].Kind != verificationEvidence {
				t.Fatalf("verification evidence %#v", capability.ActivationEvidence)
			}
			var service semreg.ServiceInstance
			for _, item := range snap.Services {
				if item.InstanceID == capability.ServiceInstance {
					service = item
				}
			}
			if service.Resource == nil || kinds[*service.Resource] != "resource.storage_interface" {
				t.Fatalf("service resource %#v", service)
			}
			var route driver.SettingRoute
			for _, item := range cut.SettingRoutes {
				if item.Route.CapabilityInstance == capability.InstanceID {
					route = item
				}
			}
			if route.Limits == nil || route.Limits.Max != nil || route.Limits.Min == nil {
				t.Fatalf("route limits %#v", route.Limits)
			}
			shape := graphShape{
				operation: string(route.Operation.ID), capability: string(capability.Definition.ID),
				argument: string(capability.Constraints[0].ID), coefficient: capability.Constraints[0].Value.Quantity.Number.Coefficient,
				exponent:  capability.Constraints[0].Value.Quantity.Number.Exponent10,
				qualified: true, available: true, evidence: string(capability.ActivationEvidence[0].Kind),
				min: route.Limits.Min.Number.Coefficient, maxAbsent: true, resource: string(kinds[*service.Resource]),
			}
			out = append(out, shape.operation+" "+shape.capability+" "+shape.argument+" "+shape.coefficient+" "+itoa(shape.exponent)+" "+shape.min+" "+shape.evidence+" "+shape.resource)
		}
	}
	if len(out) == 2 && out[0] > out[1] {
		out[0], out[1] = out[1], out[0]
	}
	if family.Image.(*scriptImage).bytes != 0 {
		t.Fatal("graph publication wrote")
	}
	if len(out) != 2 {
		t.Fatalf("capabilities %d start %v", len(out), family.startErr)
	}
	return out
}

func itoa(v int32) string {
	return strconvItoa(v)
}

func strconvItoa(v int32) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [12]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestUnboundCardsPublishNoRoute(t *testing.T) {
	for _, card := range []string{"HA", "COMMERCIAL", "L-V200"} {
		card := card
		t.Run(card, func(t *testing.T) {
			image := &scriptImage{}
			family := &Family{Card: card, Unit: 1, Path: "S-Dongle", Image: image}
			openFamily(t, family)
			cut, err := family.session.SemanticInput().Capture(context.Background(), driver.Filter{})
			if err != nil {
				t.Fatal(err)
			}
			for _, snap := range cut.Snapshots {
				if len(snap.Capabilities) != 0 {
					t.Fatalf("%s published %#v", card, snap.Capabilities)
				}
			}
			if len(cut.SettingRoutes) != 0 || image.bytes != 0 || len(image.writes) != 0 {
				t.Fatalf("%s routes %d bytes %d", card, len(cut.SettingRoutes), image.bytes)
			}
		})
	}
}
