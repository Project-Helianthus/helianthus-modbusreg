package limits

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

const (
	pvPackID      semreg.DefinitionID    = "helianthus.pack.pv"
	pvPackVersion semreg.SemanticVersion = "1.0.0"
	opID                                 = "pv.operation.set_active_power_limit"
	argID                                = "pv.limit.active_power"
	serviceDef                           = "pv.service.inverter"
	capDef                               = "pv.capability.set_active_power_limit"
	dimID                                = "pv.dimension.inverter"
	kindInverter                         = "resource.inverter"
	fieldWMax                            = "der.settings.max_active_power"
	fieldPct                             = "der.control.active_power_limit.percent"
	fieldEna                             = "der.control.active_power_limit.enabled"
	wattUnit                             = "unit.watt"
	monoEpoch     semreg.ClockEpochID    = "clock:sunspec-limit"
)

// Registers is the holding-register access the family uses. The family does not open a connection.
type Registers interface {
	ReadHolding(ctx context.Context, unit uint8, address, quantity uint16) ([]uint16, error)
	WriteMultiple(ctx context.Context, unit uint8, address uint16, registers []uint16, admission modbus.RegisterWriteAdmission) (modbus.WriteMultipleRegistersAcknowledgement, error)
}

// Inverter is one SunSpec chain the family may limit.
type Inverter struct {
	Unit  uint8
	Label string
	Base  uint16
}

// Config is the inverters and the register access of one family.
type Config struct {
	Inverters []Inverter
	Access    Registers
}

type owner struct {
	inv      Inverter
	resource semreg.ResourceID
	route    driver.SettingRoute
	asset    semreg.AssetID
}

// SunSpecPv is the setting owner of pv.operation.set_active_power_limit while it runs.
// The pack admits a watts request only when it equals the published capability constraint,
// so a limited inverter publishes the applied watts and any other inverter publishes WMax.
type SunSpecPv struct {
	cfg     Config
	mu      sync.Mutex
	fenced  atomic.Bool
	running bool
	session driver.Session
	owners  []owner
	unreg   []func()
	reg     sunspec.DecoderRegistry
	ready   bool
	seq     uint64
}

type prepared struct {
	owner      owner
	local      driver.SettingRoute
	constraint semreg.Decimal
}

// FenceWithdrawal ends later sends without blocking.
func (f *SunSpecPv) FenceWithdrawal() { f.fenced.Store(true) }

// Stop removes the executors. A later start of this family may run again.
func (f *SunSpecPv) Stop(context.Context) driver.CloseReport {
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

// Start publishes one capability and setting route for each inverter whose chain carries models 121 and 123 and a positive WMax, and registers the executor.
func (f *SunSpecPv) Start(ctx context.Context, session driver.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ctx == nil || session == nil || f.cfg.Access == nil {
		return errors.New("sunspec limit family requires a context, a session and register access")
	}
	if f.running {
		return errors.New("sunspec limit family is already running")
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

func (f *SunSpecPv) collect(ctx context.Context, session driver.Session) ([]prepared, error) {
	var ready []prepared
	root := session.Native().Root()
	for _, inv := range f.cfg.Inverters {
		snap, err := f.readChain(ctx, inv)
		if err != nil {
			return nil, err
		}
		_, _, wmax, applied, ok := f.eligibility(snap)
		if !ok {
			continue
		}
		resource := resourceID(inv.Label)
		element := driver.Element{Subject: root, Key: factKey(string(resource))}
		binding, service, capability := localIDs(inv.Label)
		route := driver.SettingRoute{
			For: element, Operation: pvDefinition(opID),
			Owner:  driver.SourceRef{For: element, Producer: root, PathKey: pathKey(inv.Label)},
			Route:  operation.Route{CapabilityInstance: semreg.CapabilityInstanceID(capability), ServiceInstance: semreg.ServiceInstanceID(service), BindingID: semreg.NativeBindingID(binding), SourceID: "source:local", SourceEpochID: "epoch:local", DriverGeneration: "1"},
			Limits: wattsLimits(nativeDecimal(wmax)),
		}
		constraint := nativeDecimal(wmax)
		if applied.Coefficient != "" {
			constraint = applied
		}
		ready = append(ready, prepared{owner: owner{inv: inv, resource: resource}, local: route, constraint: constraint})
	}
	return ready, nil
}

func (f *SunSpecPv) commit(ctx context.Context, session driver.Session, ready []prepared) error {
	probe, err := session.Prepare(nil)
	if err != nil {
		return err
	}
	stamp := batchStamp{asset: probe.AssetID, source: probe.SourceID, epoch: probe.SourceEpochID, generation: probe.DriverGeneration}
	batch, err := session.Prepare(func(b *semreg.PublicationBatch) { fillPublication(b, ready, stamp) })
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
			return errors.New("sunspec limit route was not committed")
		}
		item.owner.route = found
		held, heldOK := assetHolding(cut, found.Route.CapabilityInstance)
		if !heldOK {
			return errors.New("sunspec limit capability was not captured")
		}
		item.owner.asset = held
		f.owners = append(f.owners, item.owner)
	}
	return nil
}

func (f *SunSpecPv) drop(extra func()) {
	if extra != nil {
		f.unreg = append(f.unreg, extra)
	}
	for _, fn := range f.unreg {
		fn()
	}
	f.unreg, f.owners = nil, nil
}

// Execute is the setting-owner callback. It writes one admitted span or nothing.
func (f *SunSpecPv) Execute(ctx context.Context, d driver.Dispatch) (driver.Completion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	release, watts, reason := classify(d)
	point := "WMaxLimPct"
	if release {
		point = "WMaxLim_Ena"
	}
	if reason == "" {
		reason = f.precheck(ctx, d)
	}
	if reason != "" {
		return NotSent(f.attempt(), f.tick(d), point, reason), nil
	}
	own, _ := f.ownerFor(d)
	return f.dispatch(ctx, d, own, release, watts, point)
}

func (f *SunSpecPv) precheck(ctx context.Context, d driver.Dispatch) string {
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
	if !guardMatches(d, own) {
		return "guard_mismatch"
	}
	return ""
}

func (f *SunSpecPv) dispatch(ctx context.Context, d driver.Dispatch, own owner, release bool, watts semreg.Decimal, point string) (driver.Completion, error) {
	snap, err := f.readChain(ctx, own.inv)
	if err != nil {
		return NotSent(f.attempt(), f.tick(d), point, "model_absent"), nil
	}
	settings, controls, _, _, ok := f.eligibility(snap)
	if !ok {
		return NotSent(f.attempt(), f.tick(d), point, "model_absent"), nil
	}
	reg, err := f.registry()
	if err != nil {
		return NotSent(f.attempt(), f.tick(d), point, "model_absent"), nil
	}
	step, err := f.encode(reg, snap, settings, controls, release, watts)
	if err != nil {
		return NotSent(f.attempt(), f.tick(d), point, mapReason(err)), nil
	}
	if reason := f.beforeSend(ctx, d, own); reason != "" {
		return NotSent(f.attempt(), f.tick(d), point, reason), nil
	}
	return f.finish(ctx, d, own, step, controls, release, watts, point)
}

func (f *SunSpecPv) encode(reg sunspec.DecoderRegistry, snap sunspec.ChainSnapshot, settings, controls uint32, release bool, watts semreg.Decimal) (sunspec.ControlWriteStep, error) {
	if release {
		return sunspec.EncodeActivePowerRelease(reg, snap, controls)
	}
	native, err := sunspecWatts(watts)
	if err != nil {
		return sunspec.ControlWriteStep{}, err
	}
	return sunspec.EncodeActivePowerLimit(reg, snap, sunspec.ActivePowerLimitRequest{SettingsOrdinal: settings, ControlsOrdinal: controls, Watts: native})
}

func (f *SunSpecPv) beforeSend(ctx context.Context, d driver.Dispatch, own owner) string {
	if f.fenced.Load() || !guardMatches(d, own) {
		return "guard_mismatch"
	}
	if ctx != nil && ctx.Err() != nil {
		return "cancelled"
	}
	return ""
}

func (f *SunSpecPv) finish(ctx context.Context, d driver.Dispatch, own owner, step sunspec.ControlWriteStep, controls uint32, release bool, watts semreg.Decimal, point string) (driver.Completion, error) {
	started, attempt := f.tick(d), f.attempt()
	ev := Evidence(operation.ContractOperationV1, kindRequest, point+":"+string(attempt))
	ack, err := f.cfg.Access.WriteMultiple(ctx, own.inv.Unit, step.Request.Address, step.Request.Words, step.Admission())
	if modbus.IsRegisterWriteUnsent(err) {
		return NotSent(attempt, started, point, "arguments"), nil
	}
	if err != nil || ack.Address != step.Request.Address || ack.Quantity != step.Request.WordCount {
		return Unknown(attempt, started, point, ev), nil
	}
	completed := Acknowledged(attempt, started, f.tick(d), point, operation.AckAccepted, ev)
	if release {
		return f.noteRelease(ctx, own, controls, completed)
	}
	return f.confirm(ctx, d, own, step, completed, watts)
}

// A release is acknowledgement only. The enable re-read is native evidence beside the acknowledgement and is not a readback.
func (f *SunSpecPv) noteRelease(ctx context.Context, own owner, controls uint32, completed driver.Completion) (driver.Completion, error) {
	snap, err := f.readChain(ctx, own.inv)
	if err != nil {
		return completed, nil
	}
	occ, ok := occurrence(snap, 123, controls)
	if !ok {
		return completed, nil
	}
	decoded, err := f.decode(occ)
	if err != nil {
		return completed, nil
	}
	n, ok := enumNumber(decoded, fieldEna)
	if !ok {
		return completed, nil
	}
	return Note(completed, Evidence(operation.ContractOperationV1, kindRelease, "enable="+strconv.FormatUint(n, 10))), nil
}

func (f *SunSpecPv) confirm(ctx context.Context, d driver.Dispatch, own owner, step sunspec.ControlWriteStep, completed driver.Completion, watts semreg.Decimal) (driver.Completion, error) {
	snap, err := f.readChain(ctx, own.inv)
	if err != nil {
		return completed, nil
	}
	_, controls, wmax, _, ok := f.eligibility(snap)
	if !ok {
		return completed, nil
	}
	occ, ok := occurrence(snap, 123, controls)
	if !ok {
		return completed, nil
	}
	words, ok := spanWords(occ, step.Request.Address, step.Request.WordCount)
	if !ok || step.CompareReadback(words) == sunspec.ReadbackUnknown {
		return completed, nil
	}
	decoded, err := f.decode(occ)
	if err != nil {
		return completed, nil
	}
	got, ok := modelWatts(decoded, wmax)
	if !ok {
		return completed, nil
	}
	relation := operation.ReadbackContradicts
	if sameDecimal(got, watts) {
		relation = operation.ReadbackConfirms
	}
	id := "candidate:sunspec-limit:" + own.inv.Label + ":" + strconv.FormatUint(f.seq+1, 10)
	if err = f.publishFact(ctx, own, id, got, f.tick(d)); err != nil {
		return completed, nil
	}
	cut, err := f.session.SemanticInput().Capture(ctx, driver.Filter{})
	if err != nil {
		return completed, nil
	}
	shot, candidate, ok := findFact(cut, id, own.route.For.Key)
	if !ok {
		return completed, nil
	}
	return WithReadback(completed, shot, candidate, own.route.Route, relation, Evidence(operation.ContractOperationV1, kindRead, id)), nil
}

func (f *SunSpecPv) publishFact(ctx context.Context, own owner, id string, watts semreg.Decimal, at Moment) error {
	binding := semreg.NativeBindingID(localBinding(own.inv.Label))
	gen, epoch, source := semreg.Uint64("1"), semreg.SourceEpochID("epoch:local"), semreg.SourceID("source:local")
	value := pvWattsValue(watts)
	ev := Evidence(semreg.ContractKernelV1, kindRead, id)
	fact := semreg.FactCandidate{
		CandidateID: semreg.CandidateID(id), Key: own.route.For.Key, Value: &value, Revision: "1",
		Quality:         semreg.Quality{Assertion: semreg.AssertionObserved, Qualification: semreg.QualificationQualified, Promotion: semreg.PromotionPromoted, Validity: semreg.ValidityGood, Availability: semreg.AvailabilityAvailable, Freshness: semreg.FreshnessFresh, Reasons: []semreg.DefinitionID{}},
		Times:           semreg.Times{ReceivedAt: at.Wall, ReceiptMonotonic: at.Mono, EvaluatedAt: at.Wall, EvaluateMonotonic: at.Mono},
		FreshnessPolicy: semreg.FreshnessPolicy{PolicyID: "policy:sunspec-limit", Version: "1.0.0", FreshForNS: "3600000000000", RetainForNS: "7200000000000", MaxWallUncertaintyNS: "0"},
		BindingID:       &binding, SourceEpochID: &epoch, DriverGeneration: &gen,
		Origin:   semreg.OriginRef{OriginID: semreg.OriginID("origin:sunspec-limit:" + own.inv.Label), Kind: semreg.OriginNativeObservation, SourceID: &source, SourceEpochID: &epoch, BindingID: &binding, Evidence: ordered(ev)},
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

func (f *SunSpecPv) eligibility(snap sunspec.ChainSnapshot) (settings, controls uint32, wmax sunspec.Decimal, applied semreg.Decimal, ok bool) {
	var controlsModel sunspec.DecodedModel
	var haveControls bool
	for _, occ := range snap.Occurrences() {
		decoded, err := f.decode(occ)
		if err != nil {
			continue
		}
		switch occ.ModelID() {
		case 121:
			if value, good := positiveDecimal(decoded, fieldWMax); good {
				settings, wmax = occ.Ordinal, value
			}
		case 123:
			controls, controlsModel, haveControls = occ.Ordinal, decoded, true
		}
	}
	if settings == 0 || !haveControls || wmax.Coefficient <= 0 {
		return 0, 0, sunspec.Decimal{}, semreg.Decimal{}, false
	}
	if enabled(controlsModel) {
		applied, _ = modelWatts(controlsModel, wmax)
	}
	return settings, controls, wmax, applied, true
}

func (f *SunSpecPv) decode(occ sunspec.Occurrence) (sunspec.DecodedModel, error) {
	reg, err := f.registry()
	if err != nil {
		return sunspec.DecodedModel{}, err
	}
	return reg.DecodeOccurrence(occ)
}

func (f *SunSpecPv) registry() (sunspec.DecoderRegistry, error) {
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

func (f *SunSpecPv) readChain(ctx context.Context, inv Inverter) (sunspec.ChainSnapshot, error) {
	plan, err := sunspec.NewListedChainPlan(nil, []uint16{inv.Base}, sunspec.ChainLimits{MaxTotalWords: 4096, MaxOccurrences: 16})
	if err != nil {
		return sunspec.ChainSnapshot{}, err
	}
	chain := sunspec.NewChain(plan)
	for step := 0; step < 64; step++ {
		next := chain.NextRequests()
		if len(next) == 0 {
			return sunspec.ChainSnapshot{}, errors.New("sunspec chain ended before the end marker")
		}
		req := next[0]
		words, err := f.cfg.Access.ReadHolding(ctx, inv.Unit, req.Address(), req.WordCount())
		if err != nil {
			return sunspec.ChainSnapshot{}, err
		}
		view, err := logicalView(inv, uint64(step+1), req.Address(), words)
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

func (f *SunSpecPv) ownerFor(d driver.Dispatch) (owner, bool) {
	for _, own := range f.owners {
		if own.route.Route.BindingID == d.Guard.BindingID || own.route.Route.BindingID == d.Route.BindingID {
			return own, true
		}
	}
	return owner{}, false
}
