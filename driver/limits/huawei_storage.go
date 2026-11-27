package limits

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/huawei"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/catalog"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

const (
	packID      semreg.DefinitionID    = "helianthus.pack.storage"
	packVersion semreg.SemanticVersion = "1.1.0"

	serviceID   semreg.DefinitionID = "storage.service.interface"
	dimensionID semreg.DefinitionID = "storage.dimension.interface"
	unitWatt    semreg.DefinitionID = "unit.watt"

	interfaceResource = "resource:storage-interface"
	systemResource    = "resource:storage-system"
	bindingName       = "binding:dongle-storage"
	serviceName       = "service:dongle-storage"

	// VerificationLevel is the recorded hardware-test level of this executor.
	// Publication does not branch on it.
	VerificationLevel = catalog.VerificationNotTestedOnHardware

	verificationEvidence = "verification.not_tested_on_hardware"
)

// RegisterImage is one register write and, where the card documents a read, the re-read.
// A nil error returns that re-read. A nil error and a nil re-read mean the write was
// accepted and no row could be read. Any other error means delivery of the write is
// unknown: the call did not establish that the device accepted it.
// The family sends nothing when Image is nil.
type RegisterImage interface {
	Write(context.Context, huawei.ControlWrite, []uint16) ([]uint16, error)
}

// Family is the dongle-family battery charge and discharge limit owner.
// It registers one executor per operation the identified card binds, while it runs.
type Family struct {
	Card                string
	Unit                byte
	Path                string
	ChargeConstraint    string
	DischargeConstraint string
	Image               RegisterImage

	mu         sync.Mutex
	idle       sync.Cond
	session    driver.Session
	running    bool
	fenced     atomic.Bool
	inflight   int
	hold       bool
	prior      bool
	startErr   error
	unregister []func()
	execs      []*storageExecutor
	seq        atomic.Uint64
}

type storageExecutor struct {
	family *Family
	row    huawei.StorageRow
	route  driver.SettingRoute
	owner  generationOwner
}

// generationOwner is the session generation recorded when the family started.
// A later guard is compared with this, not with the route carried on the same dispatch.
type generationOwner struct {
	asset      semreg.AssetID
	source     semreg.SourceID
	epoch      semreg.SourceEpochID
	generation semreg.Uint64
	binding    semreg.NativeBindingID
	service    semreg.ServiceInstanceID
	capability semreg.CapabilityInstanceID
}

// Start publishes the bound operations and registers their executors.
// A card that binds neither operation publishes no capability and registers no executor.
func (f *Family) Start(ctx context.Context, session driver.Session) (err error) {
	if ctx == nil || session == nil {
		return errors.New("limits: a context and a session are required")
	}
	f.mu.Lock()
	if f.idle.L == nil {
		f.idle.L = &f.mu
	}
	if f.running {
		f.mu.Unlock()
		return errors.New("limits: the family is already running")
	}
	// A start is a new run. The fence belongs to the run that Stop ended.
	f.fenced.Store(false)
	f.session = session
	f.running = true
	f.mu.Unlock()
	defer func() {
		if err != nil {
			f.mu.Lock()
			f.startErr = err
			f.mu.Unlock()
			f.Stop(ctx)
		}
	}()
	rows, err := huawei.Bound(f.Card)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	producer := session.Native().Root()
	var routes []driver.SettingRoute
	var capabilities []semreg.CapabilityInstance
	var executors []*storageExecutor
	for _, row := range rows {
		constraint, ok := f.constraint(row.Operation)
		if !ok {
			return errors.New("limits: a bound operation has no watt constraint")
		}
		number, err := canonicalDecimal(constraint)
		if err != nil {
			return err
		}
		route := f.route(producer, row)
		routes = append(routes, route)
		capabilities = append(capabilities, f.capability(row, number))
		executors = append(executors, &storageExecutor{family: f, row: row, route: route})
	}
	for _, exec := range executors {
		unregister, err := session.RegisterExecutor(exec.route, exec)
		if err != nil {
			return err
		}
		f.mu.Lock()
		f.unregister = append(f.unregister, unregister)
		f.mu.Unlock()
	}
	if err := f.commit(ctx, f.batch(capabilities), routes); err != nil {
		return err
	}
	return f.bindOwners(ctx, executors)
}

// FenceWithdrawal ends later sends without blocking and without calling the supervisor.
func (f *Family) FenceWithdrawal() {
	f.fenced.Store(true)
}

// Stop fences later sends, drops the executors, and waits until a send that
// already passed the gate has returned. A cancelled context leaves the close unconfirmed.
func (f *Family) Stop(ctx context.Context) driver.CloseReport {
	f.FenceWithdrawal()
	if ctx == nil {
		ctx = context.Background()
	}
	done := make(chan struct{})
	go func() {
		f.mu.Lock()
		f.running = false
		unreg := f.unregister
		f.unregister = nil
		for f.inflight > 0 {
			f.idle.Wait()
		}
		f.mu.Unlock()
		for _, unregister := range unreg {
			unregister()
		}
		close(done)
	}()
	select {
	case <-done:
		return driver.CloseReport{Confirmed: true}
	case <-ctx.Done():
		return driver.CloseReport{Err: ctx.Err()}
	}
}

// bindOwners records the session generation the guard must still name.
func (f *Family) bindOwners(ctx context.Context, execs []*storageExecutor) error {
	if len(execs) == 0 {
		return nil
	}
	stamp, err := f.session.Prepare(nil)
	if err != nil {
		return err
	}
	cut, err := f.session.SemanticInput().Capture(ctx, driver.Filter{})
	if err != nil {
		return err
	}
	for _, exec := range execs {
		var route driver.SettingRoute
		found := false
		for _, item := range cut.SettingRoutes {
			if string(item.Operation.ID) == exec.row.Operation {
				route = item
				found = true
			}
		}
		if !found {
			return errors.New("limits: the bound route is not published")
		}
		if route.Route.SourceID != stamp.SourceID || route.Route.SourceEpochID != stamp.SourceEpochID || route.Route.DriverGeneration != stamp.DriverGeneration {
			return errors.New("limits: the bound route is not this session generation")
		}
		asset, ok := assetOfBinding(cut, route.Route.BindingID)
		if !ok || asset != stamp.AssetID {
			return errors.New("limits: the bound route has no session asset")
		}
		owner := generationOwner{
			asset: asset, source: route.Route.SourceID, epoch: route.Route.SourceEpochID,
			generation: route.Route.DriverGeneration, binding: route.Route.BindingID,
			service: route.Route.ServiceInstance, capability: route.Route.CapabilityInstance,
		}
		f.mu.Lock()
		exec.owner = owner
		f.mu.Unlock()
	}
	f.mu.Lock()
	f.execs = execs
	f.mu.Unlock()
	return nil
}

func (f *Family) constraint(operation string) (string, bool) {
	switch operation {
	case huawei.OpSetChargeLimit:
		return f.ChargeConstraint, f.ChargeConstraint != ""
	case huawei.OpSetDischargeLimit:
		return f.DischargeConstraint, f.DischargeConstraint != ""
	default:
		return "", false
	}
}

func (f *Family) commit(ctx context.Context, batch semreg.PublicationBatch, routes []driver.SettingRoute) error {
	tx, err := f.session.Publisher().Begin(ctx)
	if err != nil {
		return err
	}
	if err := tx.Stage(batch); err != nil {
		_ = tx.Abort(ctx)
		return err
	}
	for _, route := range routes {
		if err := tx.DeclareSource(driver.Source{For: route.For, Producer: route.Owner.Producer, PathKey: route.Owner.PathKey}); err != nil {
			_ = tx.Abort(ctx)
			return err
		}
		if err := tx.DeclareSettingRoute(route); err != nil {
			_ = tx.Abort(ctx)
			return err
		}
	}
	return tx.Commit(ctx)
}

func (f *Family) batch(capabilities []semreg.CapabilityInstance) semreg.PublicationBatch {
	binding := semreg.NativeBindingID(bindingName)
	interfaceID := semreg.ResourceID(interfaceResource)
	systemID := semreg.ResourceID(systemResource)
	return semreg.PublicationBatch{
		SourceUpserts: []semreg.SourceDescriptor{{
			ProtocolID: "protocol.modbus", ProfileID: "profile.dongle-storage", ProfileVersion: "1",
			RegistryEvidence: evidence("evidence.catalogue", "dongle-storage-source"),
			StartedAt:        timePoint(time.Unix(1, 0).UTC()), State: semreg.SourceCurrent, Revision: "1",
		}},
		BindingUpserts: []semreg.NativeBinding{{
			BindingID: binding, NativeResource: evidence("evidence.binding", "dongle-storage-binding"),
			State: semreg.BindingCurrent, Revision: "1",
		}},
		IdentityLinkUpserts: []semreg.IdentityLink{{
			BindingID: binding, Resource: &interfaceID, State: semreg.LinkQualified,
			Basis: []semreg.EvidenceRef{evidence("evidence.identity", "dongle-storage-link")}, Revision: "1",
		}},
		ResourceUpserts: []semreg.Resource{
			{
				ResourceID: interfaceID, Kind: "resource.storage_interface", State: semreg.ResourceCurrent, Revision: "1",
				Anchor: semreg.ResourceAnchor{Class: semreg.AnchorComponent, Basis: semreg.AnchorOperatorSlot, Parent: &systemID, Slot: slot("storage-interface"), Evidence: []semreg.EvidenceRef{evidence("evidence.anchor", "storage-interface")}},
				Origin: semreg.OriginRef{OriginID: "origin:operator:storage-interface", Kind: semreg.OriginOperator, Evidence: []semreg.EvidenceRef{evidence("evidence.origin", "storage-interface")}},
			},
			{
				ResourceID: systemID, Kind: "resource.storage_system", State: semreg.ResourceCurrent, Revision: "1",
				Anchor: semreg.ResourceAnchor{Class: semreg.AnchorFunction, Basis: semreg.AnchorOperatorSlot, Slot: slot("storage-system"), Evidence: []semreg.EvidenceRef{evidence("evidence.anchor", "storage-system")}},
				Origin: semreg.OriginRef{OriginID: "origin:operator:storage-system", Kind: semreg.OriginOperator, Evidence: []semreg.EvidenceRef{evidence("evidence.origin", "storage-system")}},
			},
		},
		ServiceUpserts:    []semreg.ServiceInstance{f.service()},
		CapabilityUpserts: capabilities,
	}
}

func (f *Family) service() semreg.ServiceInstance {
	interfaceID := semreg.ResourceID(interfaceResource)
	return semreg.ServiceInstance{
		InstanceID: serviceName, Definition: definition(serviceID), BindingID: bindingName,
		Qualification: semreg.QualificationQualified, Availability: semreg.AvailabilityAvailable,
		Revision: "1", Resource: &interfaceID,
	}
}

func (f *Family) capability(row huawei.StorageRow, number semreg.Decimal) semreg.CapabilityInstance {
	argument := semreg.DefinitionID(argumentID(row.Operation))
	return semreg.CapabilityInstance{
		InstanceID: semreg.CapabilityInstanceID("capability:" + row.Operation), ServiceInstance: serviceName,
		Definition: definition(semreg.DefinitionID(capabilityID(row.Operation))), BindingID: bindingName,
		Qualification: semreg.QualificationQualified, Availability: semreg.AvailabilityAvailable,
		Constraints:        []semreg.TypedField{{ID: argument, Value: wattsValue(number)}},
		ActivationEvidence: []semreg.EvidenceRef{evidence(verificationEvidence, row.Operation)},
		Revision:           "1",
	}
}

func (f *Family) route(producer driver.NodeRef, row huawei.StorageRow) driver.SettingRoute {
	element := f.element(producer, row.Operation)
	limits := &driver.ValueLimits{
		Min:      &semreg.Quantity{Number: semreg.Decimal{Coefficient: "0"}, Unit: unitWatt},
		Origin:   driver.LimitSpecification,
		Evidence: []semreg.EvidenceRef{evidence("evidence.specification", row.Operation+"-lower")},
	}
	return driver.SettingRoute{
		For: element, Operation: definition(semreg.DefinitionID(row.Operation)),
		Owner: driver.SourceRef{For: element, Producer: producer, PathKey: "dongle-" + row.Operation},
		Route: operation.Route{
			CapabilityInstance: semreg.CapabilityInstanceID("capability:" + row.Operation),
			ServiceInstance:    serviceName, BindingID: bindingName,
		},
		Limits: limits,
	}
}

func (f *Family) element(producer driver.NodeRef, operation string) driver.Element {
	text := interfaceResource
	return driver.Element{Subject: producer, Key: semreg.FactKey{
		PackID: packID, PackVersion: packVersion, FactID: semreg.DefinitionID(argumentID(operation)),
		Dimensions: []semreg.Dimension{{ID: dimensionID, Value: semreg.Value{Kind: semreg.ValueText, Text: &text}}},
	}}
}

// Execute rechecks the admitted guard, writes the bound row once, and re-reads it
// where the card documents a read. The register comparison is native evidence.
// SS08 decides the pack outcome from the completion.
func (e *storageExecutor) Execute(ctx context.Context, dispatch driver.Dispatch) (driver.Completion, error) {
	if ctx.Err() != nil {
		return driver.Completion{}, ctx.Err()
	}
	watts, err := wattsArgument(dispatch.Request.Arguments, semreg.DefinitionID(argumentID(e.row.Operation)))
	if err != nil {
		return notSent(dispatch), nil
	}
	words, err := huawei.EncodeStorageControl(e.family.Card, e.row.Offset, watts)
	if err != nil {
		return notSent(dispatch), nil
	}
	write := huawei.ControlWrite{
		Card: e.family.Card, Path: e.family.Path, IdentifiedUnit: e.family.Unit, Unit: e.family.Unit,
		Function: huawei.FunctionWriteMultipleRegisters, Address: uint16(e.row.Offset), Quantity: uint16(len(words)),
	}
	if !huawei.AdmitControlWrite(write) || e.family.Image == nil {
		return notSent(dispatch), nil
	}
	read, err, sent := e.family.invokeNative(ctx, e, dispatch, write, words)
	if !sent {
		return notSent(dispatch), nil
	}
	if err != nil {
		return unknownDelivery(dispatch), nil
	}
	switch huawei.CompareStorageReadback(e.family.Card, e.row.Offset, words, read) {
	case "match":
		return e.matched(ctx, dispatch, watts)
	case "mismatch":
		return e.conflicted(ctx, dispatch, read)
	default:
		return sentAck(dispatch), nil
	}
}

// invokeNative rechecks the guard and performs the register write as one hold of
// the lifecycle lock, so a withdrawal cannot pass between the decision and the send.
func (f *Family) invokeNative(ctx context.Context, exec *storageExecutor, dispatch driver.Dispatch, write huawei.ControlWrite, words []uint16) ([]uint16, error, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.admitNative(ctx, exec, dispatch) {
		return nil, nil, false
	}
	f.inflight++
	read, err := f.Image.Write(ctx, write, words)
	f.inflight--
	if f.inflight == 0 {
		f.idle.Broadcast()
	}
	return read, err, true
}

// admitNative is the last check before the native write. The caller holds the lifecycle lock.
// The guard is matched to the generation recorded at start. The published source epoch,
// binding and generation fence are what a retirement changes while the family is still running.
func (f *Family) admitNative(ctx context.Context, exec *storageExecutor, dispatch driver.Dispatch) bool {
	if f.fenced.Load() || !f.running || f.session == nil || !exec.owns(dispatch.Guard) {
		return false
	}
	if !f.publishedLive(ctx, dispatch.Guard) {
		return false
	}
	tx, err := f.session.Publisher().Begin(ctx)
	if err != nil {
		return false
	}
	if err := tx.Abort(context.Background()); err != nil {
		return false
	}
	return !f.fenced.Load() && f.running
}

func (e *storageExecutor) owns(guard operation.GuardClaims) bool {
	owner := e.owner
	return guard.AssetID == owner.asset && guard.SourceID == owner.source && guard.SourceEpochID == owner.epoch &&
		guard.DriverGeneration == owner.generation && guard.BindingID == owner.binding &&
		guard.ServiceInstance == owner.service && guard.CapabilityInstance == owner.capability
}

func (f *Family) publishedLive(ctx context.Context, guard operation.GuardClaims) bool {
	cut, err := f.session.SemanticInput().Capture(ctx, driver.Filter{})
	if err != nil {
		return false
	}
	snap, ok := snapshotWithBinding(cut, guard.BindingID)
	if !ok {
		return false
	}
	return sourceEpochLive(snap, guard) && bindingLive(snap, guard) && !generationFenced(snap, guard)
}

func assetOfBinding(cut driver.SemanticCut, binding semreg.NativeBindingID) (semreg.AssetID, bool) {
	snap, ok := snapshotWithBinding(cut, binding)
	if !ok {
		return "", false
	}
	return snap.AssetID, true
}

func snapshotWithBinding(cut driver.SemanticCut, binding semreg.NativeBindingID) (semreg.Snapshot, bool) {
	for _, snap := range cut.Snapshots {
		for _, item := range snap.Bindings {
			if item.BindingID == binding {
				return snap, true
			}
		}
	}
	return semreg.Snapshot{}, false
}

func sourceEpochLive(snap semreg.Snapshot, guard operation.GuardClaims) bool {
	seen, live := false, false
	for _, source := range snap.Sources {
		if source.SourceID != guard.SourceID || source.SourceEpochID != guard.SourceEpochID {
			continue
		}
		seen = true
		if source.State == semreg.SourceRetired {
			return false
		}
		if source.State == semreg.SourceCurrent {
			live = true
		}
	}
	return seen && live
}

func bindingLive(snap semreg.Snapshot, guard operation.GuardClaims) bool {
	for _, binding := range snap.Bindings {
		if binding.BindingID != guard.BindingID {
			continue
		}
		return binding.State == semreg.BindingCurrent && binding.SourceID == guard.SourceID &&
			binding.SourceEpochID == guard.SourceEpochID && binding.DriverGeneration == guard.DriverGeneration
	}
	return false
}

func generationFenced(snap semreg.Snapshot, guard operation.GuardClaims) bool {
	for _, fence := range snap.Fences {
		if fence.SourceID == guard.SourceID && fence.SourceEpochID == guard.SourceEpochID && fence.DriverGeneration == guard.DriverGeneration {
			return true
		}
	}
	return false
}

func (e *storageExecutor) matched(ctx context.Context, dispatch driver.Dispatch, watts huawei.Watts) (driver.Completion, error) {
	e.family.mu.Lock()
	hold, prior := e.family.hold, e.family.prior
	e.family.mu.Unlock()
	if hold {
		return sentAck(dispatch), nil
	}
	snap, candidate, err := e.publish(ctx, dispatch, watts)
	if err != nil {
		return driver.Completion{}, err
	}
	if prior {
		snap = shiftedGeneration(snap, candidate.CandidateID, "9")
	}
	return confirming(dispatch, snap, candidate), nil
}

func (e *storageExecutor) conflicted(ctx context.Context, dispatch driver.Dispatch, read []uint16) (driver.Completion, error) {
	decoded, err := huawei.DecodeStorageWatts(e.family.Card, e.row.Offset, read)
	if err != nil {
		decoded = huawei.Watts{Coefficient: "1"}
	}
	snap, candidate, err := e.publish(ctx, dispatch, decoded)
	if err != nil {
		return driver.Completion{}, err
	}
	done := confirming(dispatch, snap, candidate)
	done.Readback.Relation = operation.ReadbackContradicts
	done.Outcome = operation.OutcomeConflict
	return done, nil
}

func (e *storageExecutor) publish(ctx context.Context, dispatch driver.Dispatch, watts huawei.Watts) (semreg.Snapshot, semreg.FactCandidate, error) {
	fact := e.effect(watts)
	tx, err := e.family.session.Publisher().Begin(ctx)
	if err != nil {
		return semreg.Snapshot{}, semreg.FactCandidate{}, err
	}
	if err := tx.Stage(semreg.PublicationBatch{FactUpserts: []semreg.FactCandidate{fact}}); err != nil {
		_ = tx.Abort(ctx)
		return semreg.Snapshot{}, semreg.FactCandidate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return semreg.Snapshot{}, semreg.FactCandidate{}, err
	}
	cut, err := e.family.session.SemanticInput().Capture(ctx, driver.Filter{})
	if err != nil || len(cut.Snapshots) == 0 {
		return semreg.Snapshot{}, semreg.FactCandidate{}, errors.New("limits: the later snapshot is absent")
	}
	for _, snap := range cut.Snapshots {
		candidate, ok := findEffect(snap, dispatch.ExpectedEffect.Fact)
		if ok {
			return snap, candidate, nil
		}
	}
	return semreg.Snapshot{}, semreg.FactCandidate{}, errors.New("limits: the later effect candidate is absent")
}

func (e *storageExecutor) effect(watts huawei.Watts) semreg.FactCandidate {
	text := interfaceResource
	binding := semreg.NativeBindingID(bindingName)
	epoch := semreg.SourceEpochID("epoch:placeholder")
	generation := semreg.Uint64("1")
	source := semreg.SourceID("source:placeholder")
	now := timePoint(time.Now().UTC())
	value := wattsValue(semreg.Decimal{Coefficient: watts.Coefficient, Exponent10: watts.Exponent10})
	return semreg.FactCandidate{
		CandidateID: semreg.CandidateID("candidate:dongle-limit-" + strconv.FormatUint(e.family.seq.Add(1), 10)),
		Key: semreg.FactKey{
			PackID: packID, PackVersion: packVersion, FactID: semreg.DefinitionID(argumentID(e.row.Operation)),
			Dimensions: []semreg.Dimension{{ID: dimensionID, Value: semreg.Value{Kind: semreg.ValueText, Text: &text}}},
		},
		Value: &value,
		Quality: semreg.Quality{
			Assertion: semreg.AssertionObserved, Qualification: semreg.QualificationQualified,
			Promotion: semreg.PromotionPromoted, Validity: semreg.ValidityGood,
			Availability: semreg.AvailabilityAvailable, Freshness: semreg.FreshnessFresh,
			Reasons: []semreg.DefinitionID{},
		},
		Times: semreg.Times{ReceivedAt: now, EvaluatedAt: now},
		FreshnessPolicy: semreg.FreshnessPolicy{
			PolicyID: "policy:storage-limit", Version: "1.0.0",
			FreshForNS: "3600000000000", RetainForNS: "7200000000000", MaxWallUncertaintyNS: "0",
		},
		BindingID: &binding, SourceEpochID: &epoch, DriverGeneration: &generation,
		Origin: semreg.OriginRef{
			OriginID: "origin:native:dongle-limit", Kind: semreg.OriginNativeObservation,
			SourceID: &source, SourceEpochID: &epoch, BindingID: &binding,
			Evidence: []semreg.EvidenceRef{evidence("evidence.origin", "dongle-limit-fact")},
		},
		Evidence: []semreg.EvidenceRef{evidence("evidence.fact", "dongle-limit-fact")},
		Revision: "1",
	}
}

func confirming(dispatch driver.Dispatch, snap semreg.Snapshot, candidate semreg.FactCandidate) driver.Completion {
	started, completed, ok := around(candidate.Times.ReceiptMonotonic, dispatch.AdmittedAt)
	if !ok {
		return sentAck(dispatch)
	}
	return driver.Completion{
		Outcome: operation.OutcomeApplied,
		Dispatch: &operation.DispatchEvidence{
			AttemptID: nextAttempt(), Started: started, Completed: &completed,
			Delivery: operation.DeliverySent, PossibleSideEffect: true,
			Evidence: []semreg.EvidenceRef{evidence("evidence.dispatch", "sent")},
		},
		Acknowledgement: &operation.Acknowledgement{
			State: operation.AckAccepted, At: dispatch.AdmittedAt,
			Evidence: []semreg.EvidenceRef{evidence("evidence.ack", "accepted")},
		},
		Readback: &operation.Readback{
			SnapshotID: snap.SnapshotID, Revisions: snap.Revisions,
			CandidateID: candidate.CandidateID, CandidateRevision: candidate.Revision,
			BindingID: dispatch.Route.BindingID, SourceID: dispatch.Route.SourceID,
			SourceEpochID: dispatch.Route.SourceEpochID, DriverGeneration: dispatch.Route.DriverGeneration,
			Relation:   operation.ReadbackConfirms,
			Evaluation: semreg.EvaluationContext{EvaluatedAt: snap.EvaluatedAt, EvaluateMonotonic: snap.EvaluateMonotonic},
			Evidence:   []semreg.EvidenceRef{evidence("evidence.readback", "later")},
		},
		ReadbackSnapshot: &snap,
		OutcomeEvidence:  []semreg.EvidenceRef{evidence("evidence.outcome", "readback")},
	}
}

// unknownDelivery is a write whose delivery cannot be established.
// It carries no acknowledgement: a failed contact is not an accepted write.
func unknownDelivery(dispatch driver.Dispatch) driver.Completion {
	started := semreg.EvaluationContext{
		EvaluatedAt:       dispatch.AdmittedAt,
		EvaluateMonotonic: semreg.MonotonicPoint{ClockEpochID: "clock-epoch:dispatch", Nanoseconds: "1"},
	}
	completed := started
	completed.EvaluateMonotonic.Nanoseconds = "2"
	return driver.Completion{
		Outcome: operation.OutcomeIndeterminate,
		Dispatch: &operation.DispatchEvidence{
			AttemptID: nextAttempt(), Started: started, Completed: &completed,
			Delivery: operation.DeliveryUnknown, PossibleSideEffect: true,
			Evidence: []semreg.EvidenceRef{evidence("evidence.dispatch", "unknown")},
		},
		OutcomeEvidence: []semreg.EvidenceRef{evidence("evidence.outcome", "unknown")},
	}
}

func sentAck(dispatch driver.Dispatch) driver.Completion {
	started := semreg.EvaluationContext{
		EvaluatedAt:       dispatch.AdmittedAt,
		EvaluateMonotonic: semreg.MonotonicPoint{ClockEpochID: "clock-epoch:dispatch", Nanoseconds: "1"},
	}
	completed := started
	completed.EvaluateMonotonic.Nanoseconds = "2"
	return driver.Completion{
		Outcome: operation.OutcomeAcknowledgedUnverified,
		Dispatch: &operation.DispatchEvidence{
			AttemptID: nextAttempt(), Started: started, Completed: &completed,
			Delivery: operation.DeliverySent, PossibleSideEffect: true,
			Evidence: []semreg.EvidenceRef{evidence("evidence.dispatch", "ack")},
		},
		Acknowledgement: &operation.Acknowledgement{
			State: operation.AckAccepted, At: dispatch.AdmittedAt,
			Evidence: []semreg.EvidenceRef{evidence("evidence.ack", "ack-only")},
		},
		OutcomeEvidence: []semreg.EvidenceRef{evidence("evidence.outcome", "ack-only")},
	}
}

func notSent(dispatch driver.Dispatch) driver.Completion {
	return driver.Completion{
		Outcome: operation.OutcomeFailedNoContact,
		Dispatch: &operation.DispatchEvidence{
			AttemptID: nextAttempt(),
			Started: semreg.EvaluationContext{
				EvaluatedAt:       dispatch.AdmittedAt,
				EvaluateMonotonic: semreg.MonotonicPoint{ClockEpochID: "clock-epoch:dispatch", Nanoseconds: "1"},
			},
			Delivery: operation.DeliveryNotSent, PossibleSideEffect: false,
			Evidence: []semreg.EvidenceRef{evidence("evidence.dispatch", "not-sent")},
		},
		OutcomeEvidence: []semreg.EvidenceRef{evidence("evidence.outcome", "not-sent")},
	}
}

func around(receipt semreg.MonotonicPoint, wall semreg.TimePoint) (semreg.EvaluationContext, semreg.EvaluationContext, bool) {
	completedNS, ok := subUint(receipt.Nanoseconds, 1)
	if !ok {
		return semreg.EvaluationContext{}, semreg.EvaluationContext{}, false
	}
	startedNS, ok := subUint(receipt.Nanoseconds, 2)
	if !ok {
		return semreg.EvaluationContext{}, semreg.EvaluationContext{}, false
	}
	started := semreg.EvaluationContext{EvaluatedAt: wall, EvaluateMonotonic: semreg.MonotonicPoint{ClockEpochID: receipt.ClockEpochID, Nanoseconds: startedNS}}
	completed := semreg.EvaluationContext{EvaluatedAt: wall, EvaluateMonotonic: semreg.MonotonicPoint{ClockEpochID: receipt.ClockEpochID, Nanoseconds: completedNS}}
	return started, completed, true
}

func subUint(value semreg.Uint64, n uint64) (semreg.Uint64, bool) {
	parsed, ok := new(big.Int).SetString(string(value), 10)
	if !ok {
		return "", false
	}
	parsed.Sub(parsed, new(big.Int).SetUint64(n))
	if parsed.Sign() <= 0 {
		return "", false
	}
	return semreg.Uint64(parsed.String()), true
}

func shiftedGeneration(snap semreg.Snapshot, id semreg.CandidateID, generation semreg.Uint64) semreg.Snapshot {
	snap.Facts = append([]semreg.FactEnvelope(nil), snap.Facts...)
	for i := range snap.Facts {
		snap.Facts[i].Candidates = append([]semreg.FactCandidate(nil), snap.Facts[i].Candidates...)
		for j := range snap.Facts[i].Candidates {
			if snap.Facts[i].Candidates[j].CandidateID != id {
				continue
			}
			g := generation
			snap.Facts[i].Candidates[j].DriverGeneration = &g
		}
	}
	return snap
}

func findEffect(snap semreg.Snapshot, key semreg.FactKey) (semreg.FactCandidate, bool) {
	for _, envelope := range snap.Facts {
		if envelope.Key.FactID != key.FactID {
			continue
		}
		for _, candidate := range envelope.Candidates {
			return candidate, true
		}
	}
	return semreg.FactCandidate{}, false
}

func wattsArgument(fields []semreg.TypedField, id semreg.DefinitionID) (huawei.Watts, error) {
	for _, field := range fields {
		if field.ID == id && field.Value.Kind == semreg.ValueQuantity && field.Value.Quantity != nil {
			return huawei.Watts{Coefficient: field.Value.Quantity.Number.Coefficient, Exponent10: field.Value.Quantity.Number.Exponent10}, nil
		}
	}
	return huawei.Watts{}, errors.New("limits: the request has no watt argument")
}

func canonicalDecimal(raw string) (semreg.Decimal, error) {
	parsed, ok := new(big.Int).SetString(raw, 10)
	if !ok || parsed.Sign() < 0 {
		return semreg.Decimal{}, errors.New("limits: the watt constraint is not a non-negative integer")
	}
	if parsed.Sign() == 0 {
		return semreg.Decimal{Coefficient: "0"}, nil
	}
	exp := int32(0)
	ten := big.NewInt(10)
	for new(big.Int).Rem(parsed, ten).Sign() == 0 {
		parsed.Quo(parsed, ten)
		exp++
	}
	return semreg.Decimal{Coefficient: parsed.String(), Exponent10: exp}, nil
}

func wattsValue(number semreg.Decimal) semreg.Value {
	return semreg.Value{Kind: semreg.ValueQuantity, Quantity: &semreg.Quantity{Number: number, Unit: unitWatt}}
}

func definition(id semreg.DefinitionID) semreg.DefinitionRef {
	return semreg.DefinitionRef{Pack: semreg.PackRef{ID: packID, Version: packVersion}, ID: id, Version: packVersion}
}

func argumentID(operation string) string {
	switch operation {
	case huawei.OpSetChargeLimit:
		return "storage.limit.charge_power"
	case huawei.OpSetDischargeLimit:
		return "storage.limit.discharge_power"
	default:
		return ""
	}
}

func capabilityID(operation string) string {
	switch operation {
	case huawei.OpSetChargeLimit:
		return "storage.capability.set_charge_limit"
	case huawei.OpSetDischargeLimit:
		return "storage.capability.set_discharge_limit"
	default:
		return ""
	}
}

func slot(text string) *string { return &text }

func timePoint(at time.Time) semreg.TimePoint {
	return semreg.TimePoint{UnixNanoseconds: semreg.Int64(strconv.FormatInt(at.UnixNano(), 10)), ClockID: "clock.utc", UncertaintyNS: "0"}
}

func evidence(kind, label string) semreg.EvidenceRef {
	sum := sha256.Sum256([]byte(label))
	return semreg.EvidenceRef{
		Owner: "helianthus.modbus", Kind: semreg.DefinitionID(kind), Digest: semreg.Digest("sha256:" + hex.EncodeToString(sum[:])),
		Contract: "modbus.catalogue/v1", Access: semreg.EvidenceAccessPublic, Redaction: semreg.RedactionNone,
	}
}

var attempts atomic.Uint64

func nextAttempt() semreg.AttemptID {
	return semreg.AttemptID("attempt:dongle-limit-" + strconv.FormatUint(attempts.Add(1), 10))
}
