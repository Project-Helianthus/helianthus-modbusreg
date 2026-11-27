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

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	"github.com/Project-Helianthus/helianthus-modbusreg/huawei"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

const huaweiMono semreg.ClockEpochID = "clock:huawei-active-limit"

// ActiveInverter is one dongle-family inverter the active-power executor may limit.
// QueriedMaximum is the device-query maximum in watts. Nil means that query is absent.
type ActiveInverter struct {
	Unit           uint8
	Label          string
	Card           string
	Path           string
	QueriedMaximum *huawei.Watts
}

// HuaweiActive is the setting owner of pv.operation.set_active_power_limit for dongle-family inverters while the family runs.
type HuaweiActive struct {
	Inverters []ActiveInverter
	Access    Registers

	mu      sync.Mutex
	fenced  atomic.Bool
	running bool
	session driver.Session
	owners  []activeOwner
	unreg   []func()
	seq     uint64
}

type activeOwner struct {
	inv      ActiveInverter
	resource semreg.ResourceID
	route    driver.SettingRoute
	asset    semreg.AssetID
}

type activePrepared struct {
	owner      activeOwner
	local      driver.SettingRoute
	constraint semreg.Decimal
}

// FenceWithdrawal ends later sends without blocking.
func (f *HuaweiActive) FenceWithdrawal() { f.fenced.Store(true) }

// Stop removes the executors. A later start of this family may run again.
func (f *HuaweiActive) Stop(context.Context) driver.CloseReport {
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

// Start publishes one capability and setting route for each inverter whose card binds the fixed active-power row, and registers the executor.
func (f *HuaweiActive) Start(ctx context.Context, session driver.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ctx == nil || session == nil || f.Access == nil {
		return errors.New("huawei active-power family requires a context, a session and register access")
	}
	if f.running {
		return errors.New("huawei active-power family is already running")
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

func (f *HuaweiActive) collect(ctx context.Context, session driver.Session) ([]activePrepared, error) {
	var ready []activePrepared
	root := session.Native().Root()
	for _, inv := range f.Inverters {
		row, ok, err := huawei.BoundActivePower(inv.Card)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if !row.WayBack {
			return nil, errors.New("huawei active-power row has no documented read")
		}
		words, err := f.Access.ReadHolding(ctx, inv.Unit, uint16(row.Offset), uint16(row.Quantity))
		if err != nil {
			return nil, err
		}
		decoded, err := huawei.DecodeActivePowerWatts(inv.Card, words)
		if err != nil {
			return nil, err
		}
		constraint := wattsDecimal(decoded)
		resource := huaweiResourceID(inv.Label)
		element := driver.Element{Subject: root, Key: factKey(string(resource))}
		binding, service, capability := activeIDs(inv.Label)
		route := driver.SettingRoute{
			For: element, Operation: pvDefinition(opID),
			Owner:  driver.SourceRef{For: element, Producer: root, PathKey: activePath(inv.Label)},
			Route:  operation.Route{CapabilityInstance: semreg.CapabilityInstanceID(capability), ServiceInstance: semreg.ServiceInstanceID(service), BindingID: semreg.NativeBindingID(binding), SourceID: "source:local", SourceEpochID: "epoch:local", DriverGeneration: "1"},
			Limits: activeLimits(row, inv.QueriedMaximum),
		}
		ready = append(ready, activePrepared{owner: activeOwner{inv: inv, resource: resource}, local: route, constraint: constraint})
	}
	return ready, nil
}

func (f *HuaweiActive) commit(ctx context.Context, session driver.Session, ready []activePrepared) error {
	probe, err := session.Prepare(nil)
	if err != nil {
		return err
	}
	stamp := batchStamp{asset: probe.AssetID, source: probe.SourceID, epoch: probe.SourceEpochID, generation: probe.DriverGeneration}
	batch, err := session.Prepare(func(b *semreg.PublicationBatch) { fillActive(b, ready, stamp) })
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
			return errors.New("huawei active-power route was not committed")
		}
		item.owner.route = found
		held, heldOK := assetHolding(cut, found.Route.CapabilityInstance)
		if !heldOK {
			return errors.New("huawei active-power capability was not captured")
		}
		item.owner.asset = held
		f.owners = append(f.owners, item.owner)
	}
	return nil
}

func (f *HuaweiActive) drop(extra func()) {
	if extra != nil {
		f.unreg = append(f.unreg, extra)
	}
	for _, fn := range f.unreg {
		fn()
	}
	f.unreg, f.owners = nil, nil
}

// Execute writes the bound active-power row once, or sends nothing.
func (f *HuaweiActive) Execute(ctx context.Context, d driver.Dispatch) (driver.Completion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	const point = "fixed_active_limit_watts"
	release, watts, reason := classify(d)
	if reason == "" {
		reason = f.precheck(ctx, d)
	}
	if reason != "" {
		return NotSent(f.attempt(), f.tick(d), point, reason), nil
	}
	own, _ := f.ownerFor(d)
	row, ok, err := huawei.BoundActivePower(own.inv.Card)
	if err != nil || !ok {
		return NotSent(f.attempt(), f.tick(d), point, "unbound"), nil
	}
	words, err := f.encode(own, release, watts)
	if err != nil {
		return NotSent(f.attempt(), f.tick(d), point, encodeReason(err)), nil
	}
	if reason = f.beforeSend(ctx, d, own); reason != "" {
		return NotSent(f.attempt(), f.tick(d), point, reason), nil
	}
	write := huawei.ControlWrite{
		Card: own.inv.Card, Path: own.inv.Path, IdentifiedUnit: own.inv.Unit, Unit: own.inv.Unit,
		Function: huawei.FunctionWriteMultipleRegisters, Address: uint16(row.Offset), Quantity: uint16(len(words)),
	}
	if !huawei.AdmitActivePowerWrite(write) {
		return NotSent(f.attempt(), f.tick(d), point, "admission"), nil
	}
	return f.finish(ctx, d, own, row, words, write, release, watts, point)
}

func (f *HuaweiActive) encode(own activeOwner, release bool, watts semreg.Decimal) ([]uint16, error) {
	if release {
		return huawei.EncodeActivePowerRelease(own.inv.Card, own.inv.QueriedMaximum, nil)
	}
	return huawei.EncodeActivePowerLimit(own.inv.Card, huawei.Watts{Coefficient: watts.Coefficient, Exponent10: watts.Exponent10})
}

func (f *HuaweiActive) precheck(ctx context.Context, d driver.Dispatch) string {
	switch {
	case f.fenced.Load() || !f.running:
		return "stopped"
	case ctx != nil && ctx.Err() != nil:
		return "cancelled"
	}
	own, ok := f.ownerFor(d)
	if !ok {
		return "owner"
	}
	if !activeGuardMatches(d, own) {
		return "guard_mismatch"
	}
	return ""
}

func (f *HuaweiActive) beforeSend(ctx context.Context, d driver.Dispatch, own activeOwner) string {
	if f.fenced.Load() || !f.running || !activeGuardMatches(d, own) {
		return "guard_mismatch"
	}
	if ctx != nil && ctx.Err() != nil {
		return "cancelled"
	}
	return ""
}

func (f *HuaweiActive) finish(ctx context.Context, d driver.Dispatch, own activeOwner, row huawei.ActivePowerControl, words []uint16, write huawei.ControlWrite, release bool, watts semreg.Decimal, point string) (driver.Completion, error) {
	started, attempt := f.tick(d), f.attempt()
	ev := Evidence(operation.ContractOperationV1, kindRequest, point+":"+string(attempt))
	ack, err := f.Access.WriteMultiple(ctx, own.inv.Unit, write.Address, words, f.admission(own.inv))
	if modbus.IsRegisterWriteUnsent(err) {
		return NotSent(attempt, started, point, "unsent"), nil
	}
	if cause, ok := exceptionCause(err); ok {
		rejected := Acknowledged(attempt, started, f.tick(d), point, operation.AckRejected, Evidence(operation.ContractOperationV1, kindAck, cause))
		if rejected.CompoundExecution != nil && len(rejected.CompoundExecution.Steps) == 1 {
			rejected.CompoundExecution.Steps[0].Reason = cause
		}
		return rejected, nil
	}
	if err != nil || ack.Address != write.Address || ack.Quantity != uint16(len(words)) {
		return Unknown(attempt, started, point, ev), nil
	}
	completed := Acknowledged(attempt, started, f.tick(d), point, operation.AckAccepted, ev)
	if release {
		return f.noteRelease(ctx, own, row, words, completed)
	}
	return f.confirm(ctx, d, own, words, completed, watts)
}

// A release is acknowledgement only. The bound-row re-read is native evidence beside that acknowledgement.
// It publishes no enabled-state candidate and no snapshot, and it is not a confirmed readback.
func (f *HuaweiActive) noteRelease(ctx context.Context, own activeOwner, row huawei.ActivePowerControl, wrote []uint16, completed driver.Completion) (driver.Completion, error) {
	read, err := f.Access.ReadHolding(ctx, own.inv.Unit, uint16(row.Offset), uint16(row.Quantity))
	if err != nil {
		return completed, nil
	}
	material := "read=" + formatWords(read) + " compare=" + huawei.CompareActivePowerReadback(own.inv.Card, wrote, read)
	return Note(completed, Evidence(operation.ContractOperationV1, kindRelease, material)), nil
}

func (f *HuaweiActive) confirm(ctx context.Context, d driver.Dispatch, own activeOwner, wrote []uint16, completed driver.Completion, watts semreg.Decimal) (driver.Completion, error) {
	row, ok, err := huawei.BoundActivePower(own.inv.Card)
	if err != nil || !ok || !row.WayBack {
		return completed, nil
	}
	read, err := f.Access.ReadHolding(ctx, own.inv.Unit, uint16(row.Offset), uint16(row.Quantity))
	if err != nil || huawei.CompareActivePowerReadback(own.inv.Card, wrote, read) == "unknown" {
		return completed, nil
	}
	gotWatts, err := huawei.DecodeActivePowerWatts(own.inv.Card, read)
	if err != nil {
		return completed, nil
	}
	got := wattsDecimal(gotWatts)
	relation := operation.ReadbackContradicts
	if sameDecimal(got, watts) {
		relation = operation.ReadbackConfirms
	}
	id := "candidate:huawei-active:" + own.inv.Label + ":" + strconv.FormatUint(f.seq+1, 10)
	if err = f.publishFact(ctx, own, id, got, f.tick(d)); err != nil {
		return completed, nil
	}
	cut, err := f.session.SemanticInput().Capture(ctx, driver.Filter{})
	if err != nil {
		return completed, nil
	}
	shot, candidate, found := findFact(cut, id, own.route.For.Key)
	if !found {
		return completed, nil
	}
	return WithReadback(completed, shot, candidate, own.route.Route, relation, Evidence(operation.ContractOperationV1, kindRead, id)), nil
}

func (f *HuaweiActive) admission(inv ActiveInverter) modbus.RegisterWriteAdmission {
	return func(req modbus.RegisterWriteAdmissionRequest) bool {
		return huawei.AdmitActivePowerWrite(huawei.ControlWrite{
			Card: inv.Card, Path: inv.Path, IdentifiedUnit: inv.Unit, Unit: req.UnitID,
			Function: uint8(req.Function), Address: req.Address, Quantity: req.Quantity,
		})
	}
}

func (f *HuaweiActive) ownerFor(d driver.Dispatch) (activeOwner, bool) {
	for _, own := range f.owners {
		if own.route.Route.BindingID == d.Guard.BindingID || own.route.Route.BindingID == d.Route.BindingID {
			return own, true
		}
	}
	return activeOwner{}, false
}

func (f *HuaweiActive) tick(d driver.Dispatch) Moment {
	f.seq++
	wall := d.AdmittedAt
	if wall.ClockID == "" {
		wall = wallNow(time.Now().UTC())
	}
	return Moment{Wall: wall, Mono: semreg.MonotonicPoint{ClockEpochID: huaweiMono, Nanoseconds: semreg.Uint64(strconv.FormatUint(f.seq, 10))}}
}

func (f *HuaweiActive) attempt() semreg.AttemptID {
	f.seq++
	sum := sha256.Sum256([]byte("attempt:" + strconv.FormatUint(f.seq, 10)))
	return semreg.AttemptID("attempt:huawei-active:" + hex.EncodeToString(sum[:4]))
}

func activeGuardMatches(d driver.Dispatch, own activeOwner) bool {
	g, r := d.Guard, own.route.Route
	return d.Route == r && g.AssetID == own.asset && g.BindingID == r.BindingID && g.SourceID == r.SourceID && g.SourceEpochID == r.SourceEpochID &&
		g.DriverGeneration == r.DriverGeneration && g.CapabilityInstance == r.CapabilityInstance && g.ServiceInstance == r.ServiceInstance
}

func activeLimits(row huawei.ActivePowerControl, queried *huawei.Watts) *driver.ValueLimits {
	if queried != nil {
		max := wattsQuantity(wattsDecimal(*queried))
		limits := &driver.ValueLimits{Max: &max, Origin: driver.LimitDeviceQuery, Evidence: ordered(Evidence(semreg.ContractKernelV1, kindRequest, "queried-maximum:"+row.Key))}
		if row.LowerKnown {
			min := wattsQuantity(semreg.Decimal{Coefficient: row.LowerWatts, Exponent10: 0})
			limits.Min = &min
		}
		return limits
	}
	if !row.LowerKnown && !row.UpperKnown {
		return nil
	}
	limits := &driver.ValueLimits{Origin: driver.LimitSpecification, Evidence: ordered(Evidence(semreg.ContractKernelV1, kindRequest, "card-range:"+row.Key))}
	if row.LowerKnown {
		min := wattsQuantity(semreg.Decimal{Coefficient: row.LowerWatts, Exponent10: 0})
		limits.Min = &min
	}
	if row.UpperKnown {
		max := wattsQuantity(wattsDecimal(huawei.Watts{Coefficient: row.UpperWatts}))
		limits.Max = &max
	}
	return limits
}

func fillActive(b *semreg.PublicationBatch, ready []activePrepared, stamp batchStamp) {
	ev := Evidence(semreg.ContractKernelV1, kindRequest, "huawei-active-source")
	b.SourceUpserts = []semreg.SourceDescriptor{{
		SourceID: stamp.source, SourceEpochID: stamp.epoch, ProtocolID: "protocol.modbus", ProfileID: "profile.dongle-inverter", ProfileVersion: "1",
		RegistryEvidence: ev, StartedAt: b.ObservedAt, State: semreg.SourceCurrent, Revision: "1",
	}}
	for _, item := range ready {
		binding, service, capability := activeIDs(item.owner.inv.Label)
		slot, resource := item.owner.inv.Label, item.owner.resource
		anchor := ordered(Evidence(semreg.ContractKernelV1, kindRequest, "anchor:"+slot))
		recorded := Evidence(semreg.ContractKernelV1, semreg.DefinitionID(VerificationLevel), "verification:"+slot)
		b.ResourceUpserts = append(b.ResourceUpserts, semreg.Resource{
			ResourceID: resource, Kind: kindInverter, Revision: "1", State: semreg.ResourceCurrent,
			Anchor: semreg.ResourceAnchor{Class: semreg.AnchorEquipment, Basis: semreg.AnchorOperatorSlot, Slot: &slot, Evidence: anchor},
			Origin: semreg.OriginRef{OriginID: semreg.OriginID("origin:operator:" + slot), Kind: semreg.OriginOperator, Evidence: anchor},
		})
		b.BindingUpserts = append(b.BindingUpserts, semreg.NativeBinding{
			BindingID: semreg.NativeBindingID(binding), AssetID: stamp.asset, SourceID: stamp.source, SourceEpochID: stamp.epoch,
			DriverGeneration: stamp.generation, NativeResource: anchor[0], State: semreg.BindingCurrent, Revision: "1",
		})
		b.IdentityLinkUpserts = append(b.IdentityLinkUpserts, semreg.IdentityLink{
			AssetID: stamp.asset, BindingID: semreg.NativeBindingID(binding), State: semreg.LinkQualified, Basis: anchor, Revision: "1", Resource: &resource,
		})
		b.ServiceUpserts = append(b.ServiceUpserts, semreg.ServiceInstance{
			InstanceID: semreg.ServiceInstanceID(service), AssetID: stamp.asset, Definition: pvDefinition(serviceDef), BindingID: semreg.NativeBindingID(binding),
			SourceEpochID: stamp.epoch, DriverGeneration: stamp.generation, Qualification: semreg.QualificationQualified, Availability: semreg.AvailabilityAvailable, Revision: "1", Resource: &resource,
		})
		b.CapabilityUpserts = append(b.CapabilityUpserts, semreg.CapabilityInstance{
			InstanceID: semreg.CapabilityInstanceID(capability), AssetID: stamp.asset, ServiceInstance: semreg.ServiceInstanceID(service), Definition: pvDefinition(capDef), BindingID: semreg.NativeBindingID(binding),
			SourceEpochID: stamp.epoch, DriverGeneration: stamp.generation, Qualification: semreg.QualificationQualified, Availability: semreg.AvailabilityAvailable,
			Constraints: []semreg.TypedField{{ID: argID, Value: pvWattsValue(item.constraint)}}, ActivationEvidence: ordered(anchor[0], recorded), Revision: "1",
		})
	}
}

func (f *HuaweiActive) publishFact(ctx context.Context, own activeOwner, id string, watts semreg.Decimal, at Moment) error {
	binding := semreg.NativeBindingID(activeBinding(own.inv.Label))
	gen, epoch, source := semreg.Uint64("1"), semreg.SourceEpochID("epoch:local"), semreg.SourceID("source:local")
	value := pvWattsValue(watts)
	ev := Evidence(semreg.ContractKernelV1, kindRead, id)
	fact := semreg.FactCandidate{
		CandidateID: semreg.CandidateID(id), Key: own.route.For.Key, Value: &value, Revision: "1",
		Quality:         semreg.Quality{Assertion: semreg.AssertionObserved, Qualification: semreg.QualificationQualified, Promotion: semreg.PromotionPromoted, Validity: semreg.ValidityGood, Availability: semreg.AvailabilityAvailable, Freshness: semreg.FreshnessFresh, Reasons: []semreg.DefinitionID{}},
		Times:           semreg.Times{ReceivedAt: at.Wall, ReceiptMonotonic: at.Mono, EvaluatedAt: at.Wall, EvaluateMonotonic: at.Mono},
		FreshnessPolicy: semreg.FreshnessPolicy{PolicyID: "policy:huawei-active-limit", Version: "1.0.0", FreshForNS: "3600000000000", RetainForNS: "7200000000000", MaxWallUncertaintyNS: "0"},
		BindingID:       &binding, SourceEpochID: &epoch, DriverGeneration: &gen,
		Origin:   semreg.OriginRef{OriginID: semreg.OriginID("origin:huawei-active:" + own.inv.Label), Kind: semreg.OriginNativeObservation, SourceID: &source, SourceEpochID: &epoch, BindingID: &binding, Evidence: ordered(ev)},
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

func wattsDecimal(w huawei.Watts) semreg.Decimal {
	coeff, ok := new(big.Int).SetString(w.Coefficient, 10)
	if !ok {
		return semreg.Decimal{}
	}
	switch {
	case w.Exponent10 > 0:
		coeff.Mul(coeff, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(w.Exponent10)), nil))
	case w.Exponent10 < 0:
		den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-w.Exponent10)), nil)
		if new(big.Int).Rem(coeff, den).Sign() != 0 {
			return semreg.Decimal{}
		}
		coeff.Quo(coeff, den)
	}
	return canonical(coeff)
}

func encodeReason(err error) string {
	switch {
	case errors.Is(err, huawei.ErrReleaseUnknown):
		return "release_unknown"
	case errors.Is(err, huawei.ErrUnrepresentable):
		return "unrepresentable"
	default:
		return "arguments"
	}
}

func exceptionCause(err error) (string, bool) {
	var protocol *modbus.ProtocolError
	if !errors.As(err, &protocol) || protocol == nil || protocol.Kind != modbus.ErrorExceptionResponse {
		return "", false
	}
	return "exception_response:" + strconv.Itoa(int(protocol.ExceptionCode)), true
}

func formatWords(words []uint16) string {
	text := ""
	for i, word := range words {
		if i > 0 {
			text += ","
		}
		text += strconv.FormatUint(uint64(word), 10)
	}
	return text
}

func activeIDs(label string) (binding, service, capability string) {
	return "binding:huawei-active:" + label, "service:huawei-inverter:" + label, "capability:huawei-active:" + label
}

func activeBinding(label string) string { return "binding:huawei-active:" + label }

func activePath(label string) string { return "huawei.controls.active_power_limit." + label }

func huaweiResourceID(label string) semreg.ResourceID {
	sum := sha256.Sum256([]byte("dongle-inverter:" + label))
	return semreg.ResourceID("resource:dongle-inverter:" + hex.EncodeToString(sum[:4]))
}
