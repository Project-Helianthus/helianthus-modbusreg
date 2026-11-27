package limits

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

const (
	opChargeLimit      semreg.DefinitionID = "storage.operation.set_charge_limit"
	opDischargeLimit   semreg.DefinitionID = "storage.operation.set_discharge_limit"
	argChargeLimit     semreg.DefinitionID = "storage.limit.charge_power"
	argDischargeLimit  semreg.DefinitionID = "storage.limit.discharge_power"
	capChargeLimit     semreg.DefinitionID = "storage.capability.set_charge_limit"
	capDischargeLimit  semreg.DefinitionID = "storage.capability.set_discharge_limit"
	fieldStorMode                          = "storage.control.mode"
	fieldStorCharge                        = "storage.control.charge_rate"
	fieldStorDischarge                     = "storage.control.discharge_rate"
	fieldStorChargeMax                     = "storage.control.max_charge_power"
	fieldStorDisMax                        = "storage.rating.max_discharge_power"
	storageClock       semreg.ClockEpochID = "clock:sunspec-storage"
	storageChargeBit   uint16              = 1 << 0
	storageDisBit      uint16              = 1 << 1
)

// Battery is one SunSpec chain the family may limit.
type Battery struct {
	Unit  uint8
	Label string
	Base  uint16
}

// StorageConfig is the batteries and the register access of one family.
type StorageConfig struct {
	Batteries []Battery
	Access    Registers
}

type storageOwner struct {
	battery   Battery
	iface     semreg.ResourceID
	route     driver.SettingRoute
	asset     semreg.AssetID
	direction sunspec.StorageDirection
	argument  semreg.DefinitionID
	rateField string
	bit       uint16
}

type storagePrepared struct {
	owner      storageOwner
	local      driver.SettingRoute
	constraint semreg.Decimal
	batteryID  semreg.ResourceID
}

type storageBounds struct {
	ordinal     uint32
	charge      sunspec.Decimal
	discharge   sunspec.Decimal
	separate    bool
	mode        uint16
	modeKnown   bool
	chargePct   sunspec.Decimal
	disPct      sunspec.Decimal
	chargePctOK bool
	disPctOK    bool
}

// SunSpecStorage is the setting owner of the model 124 charge and discharge limits while it runs.
// The storage operations declare no duration, so the executor writes no InOutWRte_RvrtTms.
// One lock covers the mode read and both writes, so the two directions cannot drop each other's bit.
type SunSpecStorage struct {
	cfg     StorageConfig
	mu      sync.Mutex
	fenced  atomic.Bool
	running bool
	session driver.Session
	owners  []storageOwner
	unreg   []func()
	reg     sunspec.DecoderRegistry
	ready   bool
	seq     uint64
}

// FenceWithdrawal ends later sends without blocking.
func (f *SunSpecStorage) FenceWithdrawal() { f.fenced.Store(true) }

// Stop removes the executors. A later start of this family may run again.
func (f *SunSpecStorage) Stop(context.Context) driver.CloseReport {
	f.FenceWithdrawal()
	f.mu.Lock()
	f.running, f.owners = false, nil
	unreg := f.unreg
	f.unreg = nil
	f.mu.Unlock()
	for _, fn := range unreg {
		fn()
	}
	return driver.CloseReport{Confirmed: true}
}

// Start publishes the charge and discharge capabilities on each battery whose chain carries model 124 and a positive bound, and registers the executor.
func (f *SunSpecStorage) Start(ctx context.Context, session driver.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ctx == nil || session == nil || f.cfg.Access == nil {
		return errors.New("sunspec storage family requires a context, a session and register access")
	}
	if f.running {
		return errors.New("sunspec storage family is already running")
	}
	f.fenced.Store(false)
	f.session = session
	ready, err := f.collect(ctx, session)
	if err != nil || len(ready) == 0 {
		if err != nil {
			f.session = nil
		}
		f.running = err == nil
		return err
	}
	if err = f.commit(ctx, session, ready); err != nil {
		f.drop(nil)
		f.session, f.running = nil, false
		return err
	}
	f.running = true
	return nil
}

func (f *SunSpecStorage) collect(ctx context.Context, session driver.Session) ([]storagePrepared, error) {
	var ready []storagePrepared
	root := session.Native().Root()
	for _, battery := range f.cfg.Batteries {
		snap, err := f.readChain(ctx, battery)
		if err != nil {
			return nil, err
		}
		bounds, ok := f.bounds(snap)
		if !ok {
			continue
		}
		batteryID, iface := storageBatteryID(battery.Label), storageInterfaceID(battery.Label)
		binding, service := storageBinding(battery.Label), storageService(battery.Label)
		for _, op := range storageOps(bounds) {
			element := driver.Element{Subject: root, Key: storageFactKey(op.argument, string(iface))}
			route := driver.SettingRoute{
				For: element, Operation: definition(op.id),
				Owner:  driver.SourceRef{For: element, Producer: root, PathKey: op.path + battery.Label},
				Route:  operation.Route{CapabilityInstance: semreg.CapabilityInstanceID(op.capability + battery.Label), ServiceInstance: semreg.ServiceInstanceID(service), BindingID: semreg.NativeBindingID(binding), SourceID: "source:local", SourceEpochID: "epoch:local", DriverGeneration: "1"},
				Limits: storageLimits(nativeDecimal(op.bound)),
			}
			ready = append(ready, storagePrepared{
				owner: storageOwner{battery: battery, iface: iface, direction: op.direction, argument: op.argument, rateField: op.rate, bit: op.bit},
				local: route, constraint: storageConstraint(op, bounds), batteryID: batteryID,
			})
		}
	}
	return ready, nil
}

type storageOpSpec struct {
	id, argument semreg.DefinitionID
	capability   string
	direction    sunspec.StorageDirection
	bit          uint16
	rate, path   string
	bound        sunspec.Decimal
	pct          sunspec.Decimal
	pctOK        bool
}

func storageOps(b storageBounds) []storageOpSpec {
	var ops []storageOpSpec
	if b.charge.Coefficient > 0 {
		ops = append(ops, storageOpSpec{id: opChargeLimit, argument: argChargeLimit, capability: "capability:sunspec-storage:charge:", direction: sunspec.StorageCharge, bit: storageChargeBit, rate: fieldStorCharge, path: "sunspec.storage.charge_limit.", bound: b.charge, pct: b.chargePct, pctOK: b.chargePctOK})
	}
	if b.discharge.Coefficient > 0 {
		ops = append(ops, storageOpSpec{id: opDischargeLimit, argument: argDischargeLimit, capability: "capability:sunspec-storage:discharge:", direction: sunspec.StorageDischarge, bit: storageDisBit, rate: fieldStorDischarge, path: "sunspec.storage.discharge_limit.", bound: b.discharge, pct: b.disPct, pctOK: b.disPctOK})
	}
	return ops
}

func storageConstraint(op storageOpSpec, b storageBounds) semreg.Decimal {
	if b.modeKnown && b.mode&op.bit != 0 && op.pctOK {
		return canonical(wattsProduct(op.pct, op.bound))
	}
	return nativeDecimal(op.bound)
}

func (f *SunSpecStorage) commit(ctx context.Context, session driver.Session, ready []storagePrepared) error {
	probe, err := session.Prepare(nil)
	if err != nil {
		return err
	}
	stamp := batchStamp{asset: probe.AssetID, source: probe.SourceID, epoch: probe.SourceEpochID, generation: probe.DriverGeneration}
	batch, err := session.Prepare(func(b *semreg.PublicationBatch) { fillStorage(b, ready, stamp) })
	if err != nil {
		return err
	}
	tx, err := session.Publisher().Begin(ctx)
	if err != nil {
		return err
	}
	if err = tx.Stage(batch); err != nil {
		return errors.Join(err, tx.Abort(ctx))
	}
	for _, item := range ready {
		src := driver.Source{For: item.local.For, Producer: item.local.Owner.Producer, PathKey: item.local.Owner.PathKey, Limits: item.local.Limits}
		if err = tx.DeclareSource(src); err != nil {
			return errors.Join(err, tx.Abort(ctx))
		}
		if err = tx.DeclareSettingRoute(item.local); err != nil {
			return errors.Join(err, tx.Abort(ctx))
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	for _, item := range ready {
		unreg, regErr := session.RegisterExecutor(item.local, f)
		if regErr != nil {
			f.drop(unreg)
			return regErr
		}
		f.unreg = append(f.unreg, unreg)
	}
	cut, err := session.SemanticInput().Capture(ctx, driver.Filter{})
	if err != nil {
		return err
	}
	for _, item := range ready {
		found, ok := stamped(cut, item.local.Owner.PathKey)
		if !ok {
			return errors.New("sunspec storage route was not committed")
		}
		item.owner.route = found
		held, heldOK := assetHolding(cut, found.Route.CapabilityInstance)
		if !heldOK {
			return errors.New("sunspec storage capability was not captured")
		}
		item.owner.asset = held
		f.owners = append(f.owners, item.owner)
	}
	return nil
}

func (f *SunSpecStorage) drop(extra func()) {
	if extra != nil {
		f.unreg = append(f.unreg, extra)
	}
	for _, fn := range f.unreg {
		fn()
	}
	f.unreg, f.owners = nil, nil
}

func fillStorage(b *semreg.PublicationBatch, ready []storagePrepared, stamp batchStamp) {
	ev := Evidence(semreg.ContractKernelV1, kindRequest, "sunspec-storage-source")
	b.SourceUpserts = []semreg.SourceDescriptor{{
		SourceID: stamp.source, SourceEpochID: stamp.epoch, ProtocolID: "modbus.sunspec", ProfileID: "sunspec.storage.controls", ProfileVersion: "1",
		RegistryEvidence: ev, StartedAt: b.ObservedAt, State: semreg.SourceCurrent, Revision: "1",
	}}
	seen := map[string]bool{}
	for _, item := range ready {
		label := item.owner.battery.Label
		if seen[label] {
			continue
		}
		seen[label] = true
		binding, service := storageBinding(label), storageService(label)
		iface, batteryID := item.owner.iface, item.batteryID
		anchor := ordered(Evidence(semreg.ContractKernelV1, kindRequest, "anchor:"+label))
		b.ResourceUpserts = append(b.ResourceUpserts,
			semreg.Resource{ResourceID: batteryID, Kind: "resource.battery", Revision: "1", State: semreg.ResourceCurrent,
				Anchor: semreg.ResourceAnchor{Class: semreg.AnchorEquipment, Basis: semreg.AnchorOperatorSlot, Slot: slot(label), Evidence: anchor},
				Origin: semreg.OriginRef{OriginID: semreg.OriginID("origin:operator:" + label), Kind: semreg.OriginOperator, Evidence: anchor}},
			semreg.Resource{ResourceID: iface, Kind: "resource.storage_interface", Revision: "1", State: semreg.ResourceCurrent,
				Anchor: semreg.ResourceAnchor{Class: semreg.AnchorComponent, Basis: semreg.AnchorOperatorSlot, Parent: idPtr(batteryID), Slot: slot("dc"), Evidence: anchor},
				Origin: semreg.OriginRef{OriginID: semreg.OriginID("origin:operator:interface:" + label), Kind: semreg.OriginOperator, Evidence: anchor}},
		)
		b.BindingUpserts = append(b.BindingUpserts, semreg.NativeBinding{
			BindingID: semreg.NativeBindingID(binding), AssetID: stamp.asset, SourceID: stamp.source, SourceEpochID: stamp.epoch,
			DriverGeneration: stamp.generation, NativeResource: anchor[0], State: semreg.BindingCurrent, Revision: "1",
		})
		b.IdentityLinkUpserts = append(b.IdentityLinkUpserts, semreg.IdentityLink{
			AssetID: stamp.asset, BindingID: semreg.NativeBindingID(binding), State: semreg.LinkQualified, Basis: anchor, Revision: "1", Resource: idPtr(iface),
		})
		b.ServiceUpserts = append(b.ServiceUpserts, semreg.ServiceInstance{
			InstanceID: semreg.ServiceInstanceID(service), AssetID: stamp.asset, Definition: definition(serviceID), BindingID: semreg.NativeBindingID(binding),
			SourceEpochID: stamp.epoch, DriverGeneration: stamp.generation, Qualification: semreg.QualificationQualified, Availability: semreg.AvailabilityAvailable, Revision: "1", Resource: idPtr(iface),
		})
	}
	for _, item := range ready {
		label := item.owner.battery.Label
		binding, service := storageBinding(label), storageService(label)
		anchor := ordered(Evidence(semreg.ContractKernelV1, kindRequest, "anchor:"+label))
		cap := capChargeLimit
		if item.owner.direction == sunspec.StorageDischarge {
			cap = capDischargeLimit
		}
		b.CapabilityUpserts = append(b.CapabilityUpserts, semreg.CapabilityInstance{
			InstanceID: semreg.CapabilityInstanceID(item.local.Route.CapabilityInstance), AssetID: stamp.asset, ServiceInstance: semreg.ServiceInstanceID(service), Definition: definition(cap), BindingID: semreg.NativeBindingID(binding),
			SourceEpochID: stamp.epoch, DriverGeneration: stamp.generation, Qualification: semreg.QualificationQualified, Availability: semreg.AvailabilityAvailable,
			Constraints: []semreg.TypedField{{ID: item.owner.argument, Value: wattsValue(item.constraint)}}, ActivationEvidence: anchor, Revision: "1",
		})
	}
}

func idPtr(id semreg.ResourceID) *semreg.ResourceID { return &id }

func storageFactKey(arg semreg.DefinitionID, resource string) semreg.FactKey {
	return semreg.FactKey{PackID: packID, PackVersion: packVersion, FactID: arg, Dimensions: []semreg.Dimension{{ID: dimensionID, Value: semreg.Value{Kind: semreg.ValueText, Text: slot(resource)}}}}
}

func storageBinding(label string) string { return "binding:sunspec-storage:" + label }
func storageService(label string) string { return "service:sunspec-storage:" + label }

func storageBatteryID(label string) semreg.ResourceID {
	return storageHashID("resource:sunspec-battery:", "sunspec-battery:"+label)
}

func storageInterfaceID(label string) semreg.ResourceID {
	return storageHashID("resource:sunspec-storage:", "sunspec-storage:"+label)
}

func storageHashID(prefix, material string) semreg.ResourceID {
	sum := sha256.Sum256([]byte(material))
	return semreg.ResourceID(prefix + hex.EncodeToString(sum[:4]))
}

func storageLimits(max semreg.Decimal) *driver.ValueLimits {
	min, top := wattsQuantity(semreg.Decimal{Coefficient: "0"}), wattsQuantity(max)
	return &driver.ValueLimits{Min: &min, Max: &top, Origin: driver.LimitDeviceQuery, Evidence: ordered(Evidence(semreg.ContractKernelV1, kindRequest, "storage-bound"))}
}

// Execute is the setting-owner callback for one charge or discharge limit.
func (f *SunSpecStorage) Execute(ctx context.Context, d driver.Dispatch) (driver.Completion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	own, found := f.ownerFor(d)
	point := "limit"
	if found {
		point = own.rateField
	}
	if reason := f.precheck(ctx, d, found); reason != "" {
		return storageRefused(f.attempt(), f.tick(d), point, reason), nil
	}
	watts, reason := storageWatts(d, own.argument)
	if reason != "" {
		return storageRefused(f.attempt(), f.tick(d), point, reason), nil
	}
	return f.dispatch(ctx, d, own, watts)
}

func (f *SunSpecStorage) precheck(ctx context.Context, d driver.Dispatch, found bool) string {
	switch {
	case f.fenced.Load() || !f.running:
		return "stopped"
	case ctx != nil && ctx.Err() != nil:
		return "cancelled"
	case !found:
		return "owner"
	case !storageGuard(d, f.must(d)):
		return "guard_mismatch"
	}
	return ""
}

func (f *SunSpecStorage) must(d driver.Dispatch) storageOwner {
	own, _ := f.ownerFor(d)
	return own
}

func (f *SunSpecStorage) dispatch(ctx context.Context, d driver.Dispatch, own storageOwner, watts semreg.Decimal) (driver.Completion, error) {
	snap, err := f.readChain(ctx, own.battery)
	if err != nil {
		return storageRefused(f.attempt(), f.tick(d), own.rateField, "model_absent"), nil
	}
	bounds, ok := f.bounds(snap)
	if !ok {
		return storageRefused(f.attempt(), f.tick(d), own.rateField, "model_absent"), nil
	}
	if !bounds.allows(own.direction) {
		return storageRefused(f.attempt(), f.tick(d), own.rateField, "bound_absent"), nil
	}
	plan, err := f.encode(snap, bounds, own, watts)
	if err != nil {
		return storageRefused(f.attempt(), f.tick(d), own.rateField, mapReason(err)), nil
	}
	started, attempt := f.tick(d), f.attempt()
	rate := plan.Steps[0]
	ev := Evidence(operation.ContractOperationV1, kindRequest, rate.Fields[0]+"="+storageWordText(rate.Request.Words))
	if reason := f.before(ctx, d, own); reason != "" {
		return storageRefused(attempt, started, rate.Fields[0], reason), nil
	}
	return f.run(ctx, d, own, plan, rate, bounds, watts, attempt, started, ev)
}

func (f *SunSpecStorage) encode(snap sunspec.ChainSnapshot, bounds storageBounds, own storageOwner, watts semreg.Decimal) (sunspec.StorageLimitWrite, error) {
	native, err := sunspecWatts(watts)
	if err != nil {
		return sunspec.StorageLimitWrite{}, err
	}
	reg, err := f.registry()
	if err != nil {
		return sunspec.StorageLimitWrite{}, err
	}
	q := sunspec.StorageLimitRequest{Ordinal: bounds.ordinal, Direction: own.direction, Watts: native}
	if own.direction == sunspec.StorageDischarge && bounds.separate {
		max := bounds.discharge
		q.MaxDischarge = &max
	}
	plan, err := sunspec.EncodeStorageLimit(reg, snap, q)
	if err != nil {
		return sunspec.StorageLimitWrite{}, err
	}
	if len(plan.Steps) != 1 {
		return sunspec.StorageLimitWrite{}, errors.New("duration")
	}
	return plan, nil
}

func (f *SunSpecStorage) run(ctx context.Context, d driver.Dispatch, own storageOwner, plan sunspec.StorageLimitWrite, rate sunspec.ControlWriteStep, bounds storageBounds, watts semreg.Decimal, attempt semreg.AttemptID, started Moment, ev semreg.EvidenceRef) (driver.Completion, error) {
	class := f.classify(ctx, own, rate)
	switch class {
	case "unsent":
		return storageRefused(attempt, started, rate.Fields[0], "arguments"), nil
	case "rejected":
		return storagePartial(attempt, started, f.tick(d), ev, true, operation.DeliverySent, operation.AckRejected, operation.OutcomeNoEffect, rate.Fields[0], "rejected", "skipped"), nil
	case "unknown":
		return storagePartial(attempt, started, started, ev, true, operation.DeliveryUnknown, "", operation.OutcomeIndeterminate, rate.Fields[0], "unknown", "skipped"), nil
	}
	if reason := f.before(ctx, d, own); reason != "" {
		return storagePartial(attempt, started, f.tick(d), ev, true, operation.DeliverySent, operation.AckAccepted, operation.OutcomeAcknowledgedUnverified, rate.Fields[0], "accepted", reason), nil
	}
	modeStep, reason := f.modeStep(ctx, own, plan)
	if reason != "" {
		return storagePartial(attempt, started, f.tick(d), ev, true, operation.DeliverySent, operation.AckAccepted, operation.OutcomeAcknowledgedUnverified, rate.Fields[0], "accepted", reason), nil
	}
	if reason = f.before(ctx, d, own); reason != "" {
		return storagePartial(attempt, started, f.tick(d), ev, true, operation.DeliverySent, operation.AckAccepted, operation.OutcomeAcknowledgedUnverified, rate.Fields[0], "accepted", reason), nil
	}
	modeEv := Evidence(operation.ContractOperationV1, kindAck, fieldStorMode+"="+storageWordText(modeStep.Request.Words))
	switch f.classify(ctx, own, modeStep) {
	case "unsent":
		return storagePartial(attempt, started, f.tick(d), ev, true, operation.DeliverySent, operation.AckAccepted, operation.OutcomeAcknowledgedUnverified, rate.Fields[0], "accepted", "arguments"), nil
	case "rejected":
		return storagePartial(attempt, started, f.tick(d), modeEv, true, operation.DeliverySent, operation.AckRejected, operation.OutcomeNoEffect, rate.Fields[0], "accepted", "rejected-sent"), nil
	case "unknown":
		return storagePartial(attempt, started, started, modeEv, true, operation.DeliveryUnknown, "", operation.OutcomeIndeterminate, rate.Fields[0], "accepted", "unknown-sent"), nil
	}
	return f.confirm(ctx, d, own, plan, modeStep, bounds, watts, attempt, started, ev)
}

func (f *SunSpecStorage) classify(ctx context.Context, own storageOwner, step sunspec.ControlWriteStep) string {
	ack, err := f.cfg.Access.WriteMultiple(ctx, own.battery.Unit, step.Request.Address, step.Request.Words, step.Admission())
	if modbus.IsRegisterWriteUnsent(err) {
		return "unsent"
	}
	var protocol *modbus.ProtocolError
	if errors.As(err, &protocol) && protocol.Kind == modbus.ErrorExceptionResponse {
		return "rejected"
	}
	if err != nil || ack.Address != step.Request.Address || ack.Quantity != step.Request.WordCount {
		return "unknown"
	}
	return "accepted"
}

func (f *SunSpecStorage) modeStep(ctx context.Context, own storageOwner, plan sunspec.StorageLimitWrite) (sunspec.ControlWriteStep, string) {
	span := plan.Mode.Span()
	words, err := f.cfg.Access.ReadHolding(ctx, own.battery.Unit, span.Address, span.WordCount)
	if err != nil || len(words) != 1 {
		return sunspec.ControlWriteStep{}, "mode"
	}
	step, err := plan.Mode.Request(words[0])
	if err != nil {
		return sunspec.ControlWriteStep{}, mapReason(err)
	}
	return step, ""
}

func (f *SunSpecStorage) before(ctx context.Context, d driver.Dispatch, own storageOwner) string {
	switch {
	case f.fenced.Load() || !f.running:
		return "stopped"
	case ctx != nil && ctx.Err() != nil:
		return "cancelled"
	case !storageGuard(d, own) || !f.live(ctx, own):
		return "guard_mismatch"
	}
	return ""
}

func (f *SunSpecStorage) live(ctx context.Context, own storageOwner) bool {
	if f.session == nil {
		return false
	}
	cut, err := f.session.SemanticInput().Capture(ctx, driver.Filter{})
	if err != nil {
		return false
	}
	snap, ok := snapshotWithBinding(cut, own.route.Route.BindingID)
	if !ok {
		return false
	}
	guard := operation.GuardClaims{SourceID: own.route.Route.SourceID, SourceEpochID: own.route.Route.SourceEpochID, DriverGeneration: own.route.Route.DriverGeneration, BindingID: own.route.Route.BindingID}
	return sourceEpochLive(snap, guard) && bindingLive(snap, guard) && !generationFenced(snap, guard)
}

func (f *SunSpecStorage) confirm(ctx context.Context, d driver.Dispatch, own storageOwner, plan sunspec.StorageLimitWrite, mode sunspec.ControlWriteStep, bounds storageBounds, watts semreg.Decimal, attempt semreg.AttemptID, started Moment, ev semreg.EvidenceRef) (driver.Completion, error) {
	done := storagePartial(attempt, started, f.tick(d), ev, false, operation.DeliverySent, operation.AckAccepted, operation.OutcomeAcknowledgedUnverified, plan.Steps[0].Fields[0], "accepted", "accepted-sent")
	snap, err := f.readChain(ctx, own.battery)
	if err != nil {
		return done, nil
	}
	again, ok := f.bounds(snap)
	if !ok {
		return done, nil
	}
	occ, ok := occurrence(snap, 124, again.ordinal)
	if !ok {
		return done, nil
	}
	rateWords, rateOK := spanWords(occ, plan.Steps[0].Request.Address, plan.Steps[0].Request.WordCount)
	modeWords, modeOK := spanWords(occ, mode.Request.Address, mode.Request.WordCount)
	if !rateOK || !modeOK {
		return done, nil
	}
	state := plan.CompareReadback(mode, rateWords, modeWords)
	decoded, err := f.decode(occ)
	if err != nil {
		return done, nil
	}
	bound := again.charge
	if own.direction == sunspec.StorageDischarge {
		bound = again.discharge
	}
	got, gotOK := appliedStorageWatts(decoded, own.rateField, bound)
	switch {
	case state == sunspec.ReadbackMatch && gotOK && sameDecimal(got, watts):
		return f.readback(ctx, d, own, done, got, operation.ReadbackConfirms)
	case state == sunspec.ReadbackMismatch:
		return f.readback(ctx, d, own, done, conflictWatts(got, gotOK, watts, modeWords, own.bit, bound), operation.ReadbackContradicts)
	default:
		return done, nil
	}
}

// conflictWatts is the quantity correlated after a re-read that contradicts a written point.
// The rate is used when it is not the request. A clear direction bit leaves that rate unselected,
// so the bound is used when the bound is not the request. Zero remains when the request equals
// every watt figure the re-read can show: the mode word still contradicts, and a matching quantity would be confirmed.
func conflictWatts(got semreg.Decimal, gotOK bool, requested semreg.Decimal, modeWords []uint16, bit uint16, bound sunspec.Decimal) semreg.Decimal {
	if gotOK && !sameDecimal(got, requested) {
		return got
	}
	ceiling := nativeDecimal(bound)
	if len(modeWords) == 1 && modeWords[0]&bit == 0 && !sameDecimal(ceiling, requested) {
		return ceiling
	}
	return semreg.Decimal{Coefficient: "0"}
}

func (f *SunSpecStorage) readback(ctx context.Context, d driver.Dispatch, own storageOwner, done driver.Completion, watts semreg.Decimal, relation operation.ReadbackRelation) (driver.Completion, error) {
	id := "candidate:sunspec-storage:" + own.battery.Label + ":" + strconv.FormatUint(f.seq+1, 10)
	if err := f.publishFact(ctx, own, id, watts, f.tick(d)); err != nil {
		return done, nil
	}
	cut, err := f.session.SemanticInput().Capture(ctx, driver.Filter{})
	if err != nil {
		return done, nil
	}
	shot, candidate, ok := findFact(cut, id, own.route.For.Key)
	if !ok {
		return done, nil
	}
	for i := range done.CompoundExecution.Steps {
		done.CompoundExecution.Steps[i].ReadbackRelation = relation
	}
	ev := Evidence(operation.ContractOperationV1, kindRead, id)
	done.ReadbackSnapshot = &shot
	done.Readback = &operation.Readback{
		SnapshotID: shot.SnapshotID, Revisions: shot.Revisions, CandidateID: candidate.CandidateID, CandidateRevision: candidate.Revision,
		BindingID: own.route.Route.BindingID, SourceID: own.route.Route.SourceID, SourceEpochID: own.route.Route.SourceEpochID, DriverGeneration: own.route.Route.DriverGeneration,
		Relation: relation, Evaluation: semreg.EvaluationContext{EvaluatedAt: shot.EvaluatedAt, EvaluateMonotonic: shot.EvaluateMonotonic}, Evidence: ordered(ev),
	}
	done.OutcomeEvidence = ordered(append(append([]semreg.EvidenceRef{}, done.OutcomeEvidence...), ev)...)
	if relation == operation.ReadbackConfirms {
		done.Outcome = operation.OutcomeApplied
	} else {
		done.Outcome = operation.OutcomeConflict
	}
	return done, nil
}

func (f *SunSpecStorage) publishFact(ctx context.Context, own storageOwner, id string, watts semreg.Decimal, at Moment) error {
	binding := semreg.NativeBindingID(storageBinding(own.battery.Label))
	gen, epoch, source := semreg.Uint64("1"), semreg.SourceEpochID("epoch:local"), semreg.SourceID("source:local")
	value := wattsValue(watts)
	ev := Evidence(semreg.ContractKernelV1, kindRead, id)
	fact := semreg.FactCandidate{
		CandidateID: semreg.CandidateID(id), Key: own.route.For.Key, Value: &value, Revision: "1",
		Quality:         semreg.Quality{Assertion: semreg.AssertionObserved, Qualification: semreg.QualificationQualified, Promotion: semreg.PromotionPromoted, Validity: semreg.ValidityGood, Availability: semreg.AvailabilityAvailable, Freshness: semreg.FreshnessFresh, Reasons: []semreg.DefinitionID{}},
		Times:           semreg.Times{ReceivedAt: at.Wall, ReceiptMonotonic: at.Mono, EvaluatedAt: at.Wall, EvaluateMonotonic: at.Mono},
		FreshnessPolicy: semreg.FreshnessPolicy{PolicyID: "policy:sunspec-storage", Version: "1.0.0", FreshForNS: "3600000000000", RetainForNS: "7200000000000", MaxWallUncertaintyNS: "0"},
		BindingID:       &binding, SourceEpochID: &epoch, DriverGeneration: &gen,
		Origin:   semreg.OriginRef{OriginID: semreg.OriginID("origin:sunspec-storage:" + own.battery.Label), Kind: semreg.OriginNativeObservation, SourceID: &source, SourceEpochID: &epoch, BindingID: &binding, Evidence: ordered(ev)},
		Evidence: ordered(ev),
	}
	batch, err := f.session.Prepare(func(b *semreg.PublicationBatch) { b.FactUpserts = []semreg.FactCandidate{fact} })
	if err != nil {
		return err
	}
	tx, err := f.session.Publisher().Begin(ctx)
	if err != nil {
		return err
	}
	if err = tx.Stage(batch); err != nil {
		return errors.Join(err, tx.Abort(ctx))
	}
	return tx.Commit(ctx)
}

func (b storageBounds) allows(direction sunspec.StorageDirection) bool {
	if direction == sunspec.StorageCharge {
		return b.charge.Coefficient > 0
	}
	return b.discharge.Coefficient > 0
}

func (f *SunSpecStorage) bounds(snap sunspec.ChainSnapshot) (storageBounds, bool) {
	var b storageBounds
	var have bool
	for _, occ := range snap.Occurrences() {
		decoded, err := f.decode(occ)
		if err != nil {
			continue
		}
		switch occ.ModelID() {
		case 120:
			if value, ok := positiveDecimal(decoded, fieldStorDisMax); ok {
				b.discharge, b.separate = value, true
			}
		case 124:
			have, b.ordinal = true, occ.Ordinal
			if value, ok := positiveDecimal(decoded, fieldStorChargeMax); ok {
				b.charge = value
			}
			if bits, ok := modeBits(decoded); ok {
				b.mode, b.modeKnown = bits, true
			}
			b.chargePct, b.chargePctOK = scaledRate(decoded, fieldStorCharge)
			b.disPct, b.disPctOK = scaledRate(decoded, fieldStorDischarge)
		}
	}
	if !have {
		return storageBounds{}, false
	}
	if !b.separate && b.charge.Coefficient > 0 {
		b.discharge = b.charge
	}
	if b.charge.Coefficient <= 0 && b.discharge.Coefficient <= 0 {
		return storageBounds{}, false
	}
	return b, true
}

func modeBits(decoded sunspec.DecodedModel) (uint16, bool) {
	fact, ok := decoded.Fact(fieldStorMode)
	if !ok || fact.Value.State() != sunspec.ValueValid {
		return 0, false
	}
	bits, _, ok := fact.Value.Bitfield()
	return uint16(bits), ok
}

func scaledRate(decoded sunspec.DecodedModel, id string) (sunspec.Decimal, bool) {
	if value, ok := positiveDecimal(decoded, id); ok {
		return value, true
	}
	if zeroDecimal(decoded, id) {
		return sunspec.Decimal{}, true
	}
	return sunspec.Decimal{}, false
}

func appliedStorageWatts(decoded sunspec.DecodedModel, field string, bound sunspec.Decimal) (semreg.Decimal, bool) {
	percent, ok := scaledRate(decoded, field)
	if !ok || bound.Coefficient <= 0 {
		return semreg.Decimal{}, false
	}
	return canonical(wattsProduct(percent, bound)), true
}

func (f *SunSpecStorage) decode(occ sunspec.Occurrence) (sunspec.DecodedModel, error) {
	reg, err := f.registry()
	if err != nil {
		return sunspec.DecodedModel{}, err
	}
	return reg.DecodeOccurrence(occ)
}

func (f *SunSpecStorage) registry() (sunspec.DecoderRegistry, error) {
	if f.ready {
		return f.reg, nil
	}
	reg, err := sunspec.NewStandardSunSpecDecoderRegistry(sunspec.ModelsRevisionV1)
	if err != nil {
		return sunspec.DecoderRegistry{}, err
	}
	f.reg, f.ready = reg, true
	return reg, nil
}

func (f *SunSpecStorage) readChain(ctx context.Context, battery Battery) (sunspec.ChainSnapshot, error) {
	plan, err := sunspec.NewListedChainPlan(nil, []uint16{battery.Base}, sunspec.ChainLimits{MaxTotalWords: 4096, MaxOccurrences: 16})
	if err != nil {
		return sunspec.ChainSnapshot{}, err
	}
	chain := sunspec.NewChain(plan)
	viewOf := Inverter{Unit: battery.Unit}
	for step := 0; step < 64; step++ {
		next := chain.NextRequests()
		if len(next) == 0 {
			return sunspec.ChainSnapshot{}, errors.New("sunspec chain ended before the end marker")
		}
		req := next[0]
		words, err := f.cfg.Access.ReadHolding(ctx, battery.Unit, req.Address(), req.WordCount())
		if err != nil {
			return sunspec.ChainSnapshot{}, err
		}
		view, err := logicalView(viewOf, uint64(step+1), req.Address(), words)
		if err != nil {
			return sunspec.ChainSnapshot{}, err
		}
		snap, err := chain.AdmitReplay(req, view)
		if err != nil {
			return sunspec.ChainSnapshot{}, err
		}
		if len(snap.RawWords()) != 0 {
			return snap, nil
		}
	}
	return sunspec.ChainSnapshot{}, errors.New("sunspec chain walk did not finish")
}

func (f *SunSpecStorage) ownerFor(d driver.Dispatch) (storageOwner, bool) {
	for _, own := range f.owners {
		if own.route.Route.CapabilityInstance == d.Route.CapabilityInstance || own.route.Route.CapabilityInstance == d.Guard.CapabilityInstance {
			return own, true
		}
	}
	return storageOwner{}, false
}

func storageGuard(d driver.Dispatch, own storageOwner) bool {
	g, r := d.Guard, own.route.Route
	return d.Route == r && g.AssetID == own.asset && g.BindingID == r.BindingID && g.SourceID == r.SourceID && g.SourceEpochID == r.SourceEpochID &&
		g.DriverGeneration == r.DriverGeneration && g.CapabilityInstance == r.CapabilityInstance && g.ServiceInstance == r.ServiceInstance
}

func storageWatts(d driver.Dispatch, arg semreg.DefinitionID) (semreg.Decimal, string) {
	args := d.Request.Arguments
	if len(args) != 1 || args[0].ID != arg || args[0].Value.Kind != semreg.ValueQuantity || args[0].Value.Quantity == nil || args[0].Value.Quantity.Unit != wattUnit {
		return semreg.Decimal{}, "arguments"
	}
	number := args[0].Value.Quantity.Number
	if strings.HasPrefix(number.Coefficient, "-") {
		return semreg.Decimal{}, "negative_set"
	}
	return number, ""
}

func storageWordText(words []uint16) string {
	parts := make([]string, len(words))
	for i, word := range words {
		parts[i] = strconv.FormatUint(uint64(word), 10)
	}
	return strings.Join(parts, ",")
}

func storageRefused(attempt semreg.AttemptID, at Moment, rate, reason string) driver.Completion {
	done := NotSent(attempt, at, rate, reason)
	done.CompoundExecution.Steps = []driver.CompoundStep{{Point: rate, Attempted: false, Reason: reason}, {Point: fieldStorMode, Attempted: false, Reason: reason}}
	return done
}

func storagePartial(attempt semreg.AttemptID, started, done Moment, ev semreg.EvidenceRef, partial bool, delivery operation.DeliveryState, ack operation.AckState, outcome operation.Outcome, rate, rateKind, modeKind string) driver.Completion {
	completed := contextOf(done)
	dispatch := &operation.DispatchEvidence{AttemptID: attempt, Started: contextOf(started), Delivery: delivery, PossibleSideEffect: delivery != operation.DeliveryNotSent, Evidence: ordered(ev)}
	if delivery != operation.DeliveryUnknown {
		dispatch.Completed = &completed
	}
	result := driver.Completion{Outcome: outcome, Dispatch: dispatch, OutcomeEvidence: ordered(ev), CompoundExecution: &driver.CompoundExecution{Partial: partial, Steps: []driver.CompoundStep{storageStep(rate, rateKind), storageStep(fieldStorMode, modeKind)}}}
	if ack != "" {
		result.Acknowledgement = &operation.Acknowledgement{State: ack, At: done.Wall, Evidence: ordered(ev)}
	}
	return result
}

func storageStep(point, kind string) driver.CompoundStep {
	switch kind {
	case "accepted", "accepted-sent":
		return driver.CompoundStep{Point: point, Attempted: true, Delivery: operation.DeliverySent, Acknowledgement: operation.AckAccepted}
	case "rejected":
		return driver.CompoundStep{Point: point, Attempted: true, Delivery: operation.DeliverySent, Acknowledgement: operation.AckRejected}
	case "rejected-sent":
		return driver.CompoundStep{Point: point, Attempted: true, Delivery: operation.DeliverySent, Acknowledgement: operation.AckRejected}
	case "unknown", "unknown-sent":
		return driver.CompoundStep{Point: point, Attempted: true, Delivery: operation.DeliveryUnknown}
	default:
		return driver.CompoundStep{Point: point, Attempted: false, Reason: kind}
	}
}

func (f *SunSpecStorage) tick(d driver.Dispatch) Moment {
	f.seq++
	wall := d.AdmittedAt
	if wall.ClockID == "" {
		wall = wallNow(time.Now().UTC())
	}
	return Moment{Wall: wall, Mono: semreg.MonotonicPoint{ClockEpochID: storageClock, Nanoseconds: semreg.Uint64(strconv.FormatUint(f.seq, 10))}}
}

func (f *SunSpecStorage) attempt() semreg.AttemptID {
	f.seq++
	sum := sha256.Sum256([]byte("attempt:" + strconv.FormatUint(f.seq, 10)))
	return semreg.AttemptID("attempt:sunspec-storage:" + hex.EncodeToString(sum[:4]))
}
