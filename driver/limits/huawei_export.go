package limits

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

const (
	exportOp      = "pv.operation.set_export_limit"
	exportArg     = "pv.limit.export_power"
	exportService = "pv.service.system"
	exportCap     = "pv.capability.set_export_limit"
	exportDim     = "pv.dimension.system"
	exportKind    = "resource.pv_system"
	exportMono    = "clock:huawei-export-limit"
)

// ExportSystem is one dongle-family PV system whose card may bind the feed-in limit.
type ExportSystem struct {
	Unit  uint8
	Label string
	Card  string
	Path  string
}

// HuaweiExport is the setting owner of pv.operation.set_export_limit while the family runs.
type HuaweiExport struct {
	Systems []ExportSystem
	Access  Registers

	mu      sync.Mutex
	fenced  atomic.Bool
	running bool
	session driver.Session
	owners  []exportOwner
	unreg   []func()
	seq     uint64
}

type exportOwner struct {
	sys      ExportSystem
	resource semreg.ResourceID
	route    driver.SettingRoute
	asset    semreg.AssetID
}

type exportPrepared struct {
	owner      exportOwner
	local      driver.SettingRoute
	constraint semreg.Decimal
}

// FenceWithdrawal ends later sends without blocking.
func (f *HuaweiExport) FenceWithdrawal() { f.fenced.Store(true) }

// Stop removes the executors. A later start of this family may run again.
func (f *HuaweiExport) Stop(context.Context) driver.CloseReport {
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

// Start publishes pv.capability.set_export_limit for each system whose card binds the feed-in rows, and registers the executor.
func (f *HuaweiExport) Start(ctx context.Context, session driver.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ctx == nil || session == nil || f.Access == nil {
		return errors.New("huawei export-limit family requires a context, a session and register access")
	}
	if f.running {
		return errors.New("huawei export-limit family is already running")
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

func (f *HuaweiExport) collect(ctx context.Context, session driver.Session) ([]exportPrepared, error) {
	var ready []exportPrepared
	root := session.Native().Root()
	for _, sys := range f.Systems {
		lim, ok, err := huawei.BoundExportLimit(sys.Card)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if !lim.Power.WayBack || !lim.Mode.WayBack {
			return nil, errors.New("huawei export-limit row has no documented read")
		}
		words, err := f.Access.ReadHolding(ctx, sys.Unit, uint16(lim.Power.Offset), uint16(lim.Power.Quantity))
		if err != nil {
			return nil, err
		}
		decoded, err := huawei.DecodeExportWatts(sys.Card, words)
		if err != nil {
			return nil, err
		}
		resource := exportResourceID(sys.Label)
		element := driver.Element{Subject: root, Key: exportFactKey(string(resource))}
		binding, service, capability := exportIDs(sys.Label)
		route := driver.SettingRoute{
			For: element, Operation: pvDefinition(exportOp),
			Owner:  driver.SourceRef{For: element, Producer: root, PathKey: exportPath(sys.Label)},
			Route:  operation.Route{CapabilityInstance: semreg.CapabilityInstanceID(capability), ServiceInstance: semreg.ServiceInstanceID(service), BindingID: semreg.NativeBindingID(binding), SourceID: "source:local", SourceEpochID: "epoch:local", DriverGeneration: "1"},
			Limits: exportLimits(lim),
		}
		ready = append(ready, exportPrepared{owner: exportOwner{sys: sys, resource: resource}, local: route, constraint: wattsDecimal(decoded)})
	}
	return ready, nil
}

func (f *HuaweiExport) commit(ctx context.Context, session driver.Session, ready []exportPrepared) error {
	probe, err := session.Prepare(nil)
	if err != nil {
		return err
	}
	stamp := batchStamp{asset: probe.AssetID, source: probe.SourceID, epoch: probe.SourceEpochID, generation: probe.DriverGeneration}
	batch, err := session.Prepare(func(b *semreg.PublicationBatch) { fillExport(b, ready, stamp) })
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
			return errors.New("huawei export-limit route was not committed")
		}
		item.owner.route = found
		held, heldOK := assetHolding(cut, found.Route.CapabilityInstance)
		if !heldOK {
			return errors.New("huawei export-limit capability was not captured")
		}
		item.owner.asset = held
		f.owners = append(f.owners, item.owner)
	}
	return nil
}

func (f *HuaweiExport) drop(extra func()) {
	if extra != nil {
		f.unreg = append(f.unreg, extra)
	}
	for _, fn := range f.unreg {
		fn()
	}
	f.unreg, f.owners = nil, nil
}

// Execute writes the bound mode row and the bound power row, or sends nothing.
// The scope row is read and never written. The percent row is not written.
func (f *HuaweiExport) Execute(ctx context.Context, d driver.Dispatch) (driver.Completion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	const point = "export_limit"
	watts, reason := exportArgument(d)
	if reason == "" {
		reason = f.precheck(ctx, d)
	}
	if reason != "" {
		return NotSent(f.attempt(), f.tick(d), point, reason), nil
	}
	own, _ := f.ownerFor(d)
	lim, ok, err := huawei.BoundExportLimit(own.sys.Card)
	if err != nil || !ok {
		return NotSent(f.attempt(), f.tick(d), point, "unbound"), nil
	}
	mode, value, err := huawei.EncodeExportLimit(own.sys.Card, huawei.Watts{Coefficient: watts.Coefficient, Exponent10: watts.Exponent10})
	if err != nil {
		return NotSent(f.attempt(), f.tick(d), point, encodeReason(err)), nil
	}
	if reason = outsideCard(lim, watts); reason != "" {
		return NotSent(f.attempt(), f.tick(d), point, reason), nil
	}
	if !admitBoth(own.sys, lim, mode, value) {
		return NotSent(f.attempt(), f.tick(d), point, "admission"), nil
	}
	if reason = f.beforeSend(ctx, d, own); reason != "" {
		return NotSent(f.attempt(), f.tick(d), point, reason), nil
	}
	if reason = f.scopeAllows(ctx, own.sys, lim); reason != "" {
		return NotSent(f.attempt(), f.tick(d), point, reason), nil
	}
	started, attempt := f.tick(d), f.attempt()
	ev := Evidence(operation.ContractOperationV1, kindRequest, point+":"+string(attempt))
	if completion, failed := f.writePair(ctx, own, lim, mode, value, attempt, started, ev); failed {
		return completion, nil
	}
	return f.readBack(ctx, d, own, lim, mode, value, watts, f.accepted(attempt, started, ev, d))
}

func (f *HuaweiExport) writePair(ctx context.Context, own exportOwner, lim huawei.ExportLimit, mode, value []uint16, attempt semreg.AttemptID, started Moment, ev semreg.EvidenceRef) (driver.Completion, bool) {
	kind, cause := f.send(ctx, own.sys, uint16(lim.Mode.Offset), mode)
	if kind != writeOK {
		return f.firstFailure(attempt, started, ev, kind, cause), true
	}
	kind, cause = f.send(ctx, own.sys, uint16(lim.Power.Offset), value)
	if kind != writeOK {
		return f.secondFailure(attempt, started, ev, kind, cause), true
	}
	return driver.Completion{}, false
}

func (f *HuaweiExport) readBack(ctx context.Context, d driver.Dispatch, own exportOwner, lim huawei.ExportLimit, mode, value []uint16, watts semreg.Decimal, completed driver.Completion) (driver.Completion, error) {
	modeRead, modeErr := f.Access.ReadHolding(ctx, own.sys.Unit, uint16(lim.Mode.Offset), uint16(lim.Mode.Quantity))
	valueRead, valueErr := f.Access.ReadHolding(ctx, own.sys.Unit, uint16(lim.Power.Offset), uint16(lim.Power.Quantity))
	var scopeWord uint16
	var scopeErr error
	var scopeRead bool
	if lim.Scope != nil {
		words, err := f.Access.ReadHolding(ctx, own.sys.Unit, uint16(lim.Scope.Offset), uint16(lim.Scope.Quantity))
		scopeErr, scopeRead = err, true
		if err == nil && len(words) == 1 {
			scopeWord = words[0]
		} else if err == nil {
			scopeErr = errors.New("huawei export scope width")
		}
	}
	if modeErr != nil || valueErr != nil || scopeErr != nil {
		return inconclusiveRead(completed, "read_unobtained"), nil
	}
	modeCmp := huawei.CompareExportReadback(own.sys.Card, "mode", mode, modeRead)
	valueCmp := huawei.CompareExportReadback(own.sys.Card, "power", value, valueRead)
	if modeCmp == "unknown" || valueCmp == "unknown" {
		return inconclusiveRead(completed, "read_unobtained"), nil
	}
	decoded, decErr := huawei.DecodeExportWatts(own.sys.Card, valueRead)
	wattsMatch := decErr == nil && sameDecimal(wattsDecimal(decoded), watts)
	scopeTotal := !scopeRead || scopeWord == lim.Total
	if modeCmp == "match" && valueCmp == "match" && wattsMatch && scopeTotal {
		return f.confirm(ctx, d, own, wattsDecimal(decoded), completed)
	}
	scopePhase := scopeRead && scopeWord == lim.Phase
	if modeCmp == "mismatch" || valueCmp == "mismatch" || (decErr == nil && !wattsMatch) || scopePhase {
		if got, ok := exportConflictWatts(decoded, decErr == nil, watts); ok {
			return f.contradict(ctx, d, own, got, completed)
		}
	}
	// The scope row states total and individual phase. Another obtained word is
	// neither the system effect nor that contradiction, so it confirms nothing.
	if modeCmp == "match" && valueCmp == "match" && wattsMatch && scopeRead && scopeWord != lim.Total && !scopePhase {
		return inconclusiveRead(completed, "scope="+strconv.FormatUint(uint64(scopeWord), 10)), nil
	}
	material := "mode=" + modeCmp + " value=" + valueCmp
	if scopeRead {
		material += " scope=" + strconv.FormatUint(uint64(scopeWord), 10)
	}
	return f.contradictUnobserved(ctx, own, completed, material)
}

// contradictUnobserved records a contradicting read that is not a second watts
// value. The kernel keeps that conflict only on a later snapshot, so the cut is
// advanced with a commit that stores no fact. When that snapshot cannot be
// taken, the acknowledged completion is not returned as success.
func (f *HuaweiExport) contradictUnobserved(ctx context.Context, own exportOwner, completed driver.Completion, material string) (driver.Completion, error) {
	snap, err := f.laterSnapshot(ctx, own.asset)
	if err != nil {
		return contradictionUnrecorded(completed, own.route.Route, material), err
	}
	return contradictWithoutCandidate(completed, snap, own.route.Route, material), nil
}

func (f *HuaweiExport) laterSnapshot(ctx context.Context, asset semreg.AssetID) (semreg.Snapshot, error) {
	batch, err := f.session.Prepare(func(*semreg.PublicationBatch) {})
	if err != nil {
		return semreg.Snapshot{}, err
	}
	tx, err := f.session.Publisher().Begin(ctx)
	if err != nil {
		return semreg.Snapshot{}, err
	}
	if err = tx.Stage(batch); err != nil {
		return semreg.Snapshot{}, errors.Join(err, tx.Abort(ctx))
	}
	if err = tx.Commit(ctx); err != nil {
		return semreg.Snapshot{}, err
	}
	cut, err := f.session.SemanticInput().Capture(ctx, driver.Filter{})
	if err != nil {
		return semreg.Snapshot{}, err
	}
	for _, snap := range cut.Snapshots {
		if snap.AssetID == asset {
			return snap, nil
		}
	}
	return semreg.Snapshot{}, errors.New("huawei export contradiction snapshot")
}

func (f *HuaweiExport) confirm(ctx context.Context, d driver.Dispatch, own exportOwner, got semreg.Decimal, completed driver.Completion) (driver.Completion, error) {
	return f.withObserved(ctx, d, own, got, completed, operation.ReadbackConfirms)
}

func (f *HuaweiExport) contradict(ctx context.Context, d driver.Dispatch, own exportOwner, got semreg.Decimal, completed driver.Completion) (driver.Completion, error) {
	return f.withObserved(ctx, d, own, got, completed, operation.ReadbackContradicts)
}

func (f *HuaweiExport) withObserved(ctx context.Context, d driver.Dispatch, own exportOwner, got semreg.Decimal, completed driver.Completion, relation operation.ReadbackRelation) (driver.Completion, error) {
	id := "candidate:huawei-export:" + own.sys.Label + ":" + strconv.FormatUint(f.seq+1, 10)
	if err := f.publishFact(ctx, own, id, got, f.tick(d)); err != nil {
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

func (f *HuaweiExport) send(ctx context.Context, sys ExportSystem, address uint16, words []uint16) (writeKind, string) {
	ack, err := f.Access.WriteMultiple(ctx, sys.Unit, address, words, f.admission(sys))
	switch {
	case err == nil && ack.Address == address && ack.Quantity == uint16(len(words)):
		return writeOK, ""
	case modbus.IsRegisterWriteUnsent(err):
		return writeUnsent, "unsent"
	default:
		if cause, ok := exceptionCause(err); ok {
			return writeException, cause
		}
		return writeLost, "lost"
	}
}

func (f *HuaweiExport) firstFailure(attempt semreg.AttemptID, started Moment, ev semreg.EvidenceRef, kind writeKind, cause string) driver.Completion {
	switch kind {
	case writeUnsent:
		return NotSent(attempt, started, "export_mode", cause)
	case writeException:
		rejected := Acknowledged(attempt, started, f.tick(driver.Dispatch{}), "export_mode", operation.AckRejected, Evidence(operation.ContractOperationV1, kindAck, cause))
		if rejected.CompoundExecution != nil && len(rejected.CompoundExecution.Steps) == 1 {
			rejected.CompoundExecution.Steps[0].Reason = cause
		}
		return rejected
	default:
		return Unknown(attempt, started, "export_mode", ev)
	}
}

func (f *HuaweiExport) secondFailure(attempt semreg.AttemptID, started Moment, ev semreg.EvidenceRef, kind writeKind, cause string) driver.Completion {
	second := driver.CompoundStep{Point: "export_value", Reason: cause}
	switch kind {
	case writeUnsent:
		second.Attempted = false
	case writeException:
		second.Attempted, second.Delivery, second.Acknowledgement = true, operation.DeliverySent, operation.AckRejected
	default:
		second.Attempted, second.Delivery = true, operation.DeliveryUnknown
	}
	if !second.Attempted {
		second.Delivery, second.Acknowledgement = "", ""
	}
	return driver.Completion{
		Outcome: operation.OutcomeIndeterminate,
		Dispatch: &operation.DispatchEvidence{AttemptID: attempt, Started: contextOf(started),
			Delivery: operation.DeliverySent, PossibleSideEffect: true, Evidence: ordered(ev)},
		OutcomeEvidence: ordered(ev),
		CompoundExecution: &driver.CompoundExecution{Steps: []driver.CompoundStep{
			{Point: "export_mode", Attempted: true, Delivery: operation.DeliverySent, Acknowledgement: operation.AckAccepted},
			second,
		}},
	}
}

func (f *HuaweiExport) accepted(attempt semreg.AttemptID, started Moment, ev semreg.EvidenceRef, d driver.Dispatch) driver.Completion {
	done := f.tick(d)
	completed := contextOf(done)
	return driver.Completion{
		Outcome: operation.OutcomeAcknowledgedUnverified,
		Dispatch: &operation.DispatchEvidence{AttemptID: attempt, Started: contextOf(started), Completed: &completed,
			Delivery: operation.DeliverySent, PossibleSideEffect: true, Evidence: ordered(ev)},
		Acknowledgement: &operation.Acknowledgement{State: operation.AckAccepted, At: done.Wall, Evidence: ordered(ev)},
		OutcomeEvidence: ordered(ev),
		CompoundExecution: &driver.CompoundExecution{Steps: []driver.CompoundStep{
			{Point: "export_mode", Attempted: true, Delivery: operation.DeliverySent, Acknowledgement: operation.AckAccepted},
			{Point: "export_value", Attempted: true, Delivery: operation.DeliverySent, Acknowledgement: operation.AckAccepted},
		}},
	}
}

func (f *HuaweiExport) scopeAllows(ctx context.Context, sys ExportSystem, lim huawei.ExportLimit) string {
	if lim.Scope == nil {
		return ""
	}
	words, err := f.Access.ReadHolding(ctx, sys.Unit, uint16(lim.Scope.Offset), uint16(lim.Scope.Quantity))
	if err != nil || len(words) != lim.Scope.Quantity || len(words) != 1 || words[0] != lim.Total {
		return "scope_not_total"
	}
	return ""
}

func (f *HuaweiExport) admission(sys ExportSystem) modbus.RegisterWriteAdmission {
	return func(req modbus.RegisterWriteAdmissionRequest) bool {
		return huawei.AdmitExportWrite(huawei.ControlWrite{
			Card: sys.Card, Path: sys.Path, IdentifiedUnit: sys.Unit, Unit: req.UnitID,
			Function: uint8(req.Function), Address: req.Address, Quantity: req.Quantity,
		})
	}
}

func (f *HuaweiExport) precheck(ctx context.Context, d driver.Dispatch) string {
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
	if !exportGuardMatches(d, own) {
		return "guard_mismatch"
	}
	return ""
}

func (f *HuaweiExport) beforeSend(ctx context.Context, d driver.Dispatch, own exportOwner) string {
	if f.fenced.Load() || !f.running || !exportGuardMatches(d, own) {
		return "guard_mismatch"
	}
	if ctx != nil && ctx.Err() != nil {
		return "cancelled"
	}
	return ""
}

func (f *HuaweiExport) ownerFor(d driver.Dispatch) (exportOwner, bool) {
	for _, own := range f.owners {
		if own.route.Route.BindingID == d.Guard.BindingID || own.route.Route.BindingID == d.Route.BindingID {
			return own, true
		}
	}
	return exportOwner{}, false
}

func (f *HuaweiExport) tick(d driver.Dispatch) Moment {
	f.seq++
	wall := d.AdmittedAt
	if wall.ClockID == "" {
		wall = wallNow(time.Now().UTC())
	}
	return Moment{Wall: wall, Mono: semreg.MonotonicPoint{ClockEpochID: exportMono, Nanoseconds: semreg.Uint64(strconv.FormatUint(f.seq, 10))}}
}

func (f *HuaweiExport) attempt() semreg.AttemptID {
	f.seq++
	sum := sha256.Sum256([]byte("attempt:" + strconv.FormatUint(f.seq, 10)))
	return semreg.AttemptID("attempt:huawei-export:" + hex.EncodeToString(sum[:4]))
}

func (f *HuaweiExport) publishFact(ctx context.Context, own exportOwner, id string, watts semreg.Decimal, at Moment) error {
	binding := semreg.NativeBindingID(exportBinding(own.sys.Label))
	gen, epoch, source := semreg.Uint64("1"), semreg.SourceEpochID("epoch:local"), semreg.SourceID("source:local")
	value := pvWattsValue(watts)
	ev := Evidence(semreg.ContractKernelV1, kindRead, id)
	fact := semreg.FactCandidate{
		CandidateID: semreg.CandidateID(id), Key: own.route.For.Key, Value: &value, Revision: "1",
		Quality:         semreg.Quality{Assertion: semreg.AssertionObserved, Qualification: semreg.QualificationQualified, Promotion: semreg.PromotionPromoted, Validity: semreg.ValidityGood, Availability: semreg.AvailabilityAvailable, Freshness: semreg.FreshnessFresh, Reasons: []semreg.DefinitionID{}},
		Times:           semreg.Times{ReceivedAt: at.Wall, ReceiptMonotonic: at.Mono, EvaluatedAt: at.Wall, EvaluateMonotonic: at.Mono},
		FreshnessPolicy: semreg.FreshnessPolicy{PolicyID: "policy:huawei-export-limit", Version: "1.0.0", FreshForNS: "3600000000000", RetainForNS: "7200000000000", MaxWallUncertaintyNS: "0"},
		BindingID:       &binding, SourceEpochID: &epoch, DriverGeneration: &gen,
		Origin:   semreg.OriginRef{OriginID: semreg.OriginID("origin:huawei-export:" + own.sys.Label), Kind: semreg.OriginNativeObservation, SourceID: &source, SourceEpochID: &epoch, BindingID: &binding, Evidence: ordered(ev)},
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

type writeKind int

const (
	writeOK writeKind = iota
	writeUnsent
	writeException
	writeLost
)

func exportArgument(d driver.Dispatch) (semreg.Decimal, string) {
	if len(d.Request.Arguments) != 1 {
		return semreg.Decimal{}, "arguments"
	}
	arg := d.Request.Arguments[0]
	if arg.ID != exportArg || arg.Value.Kind != semreg.ValueQuantity || arg.Value.Quantity == nil || arg.Value.Quantity.Unit != wattUnit {
		return semreg.Decimal{}, "arguments"
	}
	return arg.Value.Quantity.Number, ""
}

func outsideCard(lim huawei.ExportLimit, watts semreg.Decimal) string {
	got := intOf(watts)
	if lim.LowerKnown && got.Cmp(intOf(semreg.Decimal{Coefficient: lim.LowerWatts})) < 0 {
		return "outside_limit"
	}
	if lim.UpperKnown && got.Cmp(intOf(semreg.Decimal{Coefficient: lim.UpperWatts})) > 0 {
		return "outside_limit"
	}
	return ""
}

func admitBoth(sys ExportSystem, lim huawei.ExportLimit, mode, value []uint16) bool {
	modeWrite := huawei.ControlWrite{Card: sys.Card, Path: sys.Path, IdentifiedUnit: sys.Unit, Unit: sys.Unit, Function: huawei.FunctionWriteMultipleRegisters, Address: uint16(lim.Mode.Offset), Quantity: uint16(len(mode))}
	valueWrite := modeWrite
	valueWrite.Address, valueWrite.Quantity = uint16(lim.Power.Offset), uint16(len(value))
	return huawei.AdmitExportWrite(modeWrite) && huawei.AdmitExportWrite(valueWrite)
}

func exportGuardMatches(d driver.Dispatch, own exportOwner) bool {
	g, r := d.Guard, own.route.Route
	return d.Route == r && g.AssetID == own.asset && g.BindingID == r.BindingID && g.SourceID == r.SourceID && g.SourceEpochID == r.SourceEpochID &&
		g.DriverGeneration == r.DriverGeneration && g.CapabilityInstance == r.CapabilityInstance && g.ServiceInstance == r.ServiceInstance
}

func exportLimits(lim huawei.ExportLimit) *driver.ValueLimits {
	if !lim.LowerKnown && !lim.UpperKnown {
		return nil
	}
	limits := &driver.ValueLimits{Origin: driver.LimitSpecification, Evidence: ordered(Evidence(semreg.ContractKernelV1, kindRequest, "card-range:"+lim.Power.Key))}
	if lim.LowerKnown {
		min := wattsQuantity(wattsDecimal(huawei.Watts{Coefficient: lim.LowerWatts}))
		limits.Min = &min
	}
	if lim.UpperKnown {
		max := wattsQuantity(wattsDecimal(huawei.Watts{Coefficient: lim.UpperWatts}))
		limits.Max = &max
	}
	return limits
}

func fillExport(b *semreg.PublicationBatch, ready []exportPrepared, stamp batchStamp) {
	ev := Evidence(semreg.ContractKernelV1, kindRequest, "huawei-export-source")
	b.SourceUpserts = []semreg.SourceDescriptor{{
		SourceID: stamp.source, SourceEpochID: stamp.epoch, ProtocolID: "protocol.modbus", ProfileID: "profile.dongle-system", ProfileVersion: "1",
		RegistryEvidence: ev, StartedAt: b.ObservedAt, State: semreg.SourceCurrent, Revision: "1",
	}}
	for _, item := range ready {
		binding, service, capability := exportIDs(item.owner.sys.Label)
		slot := item.owner.sys.Label
		resource := item.owner.resource
		anchor := ordered(Evidence(semreg.ContractKernelV1, kindRequest, "anchor:"+slot))
		recorded := Evidence(semreg.ContractKernelV1, semreg.DefinitionID(VerificationLevel), "verification:"+slot)
		b.ResourceUpserts = append(b.ResourceUpserts, semreg.Resource{
			ResourceID: resource, Kind: exportKind, Revision: "1", State: semreg.ResourceCurrent,
			Anchor: semreg.ResourceAnchor{Class: semreg.AnchorFunction, Basis: semreg.AnchorOperatorSlot, Slot: &slot, Evidence: anchor},
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
			InstanceID: semreg.ServiceInstanceID(service), AssetID: stamp.asset, Definition: pvDefinition(exportService), BindingID: semreg.NativeBindingID(binding),
			SourceEpochID: stamp.epoch, DriverGeneration: stamp.generation, Qualification: semreg.QualificationQualified, Availability: semreg.AvailabilityAvailable, Revision: "1", Resource: &resource,
		})
		b.CapabilityUpserts = append(b.CapabilityUpserts, semreg.CapabilityInstance{
			InstanceID: semreg.CapabilityInstanceID(capability), AssetID: stamp.asset, ServiceInstance: semreg.ServiceInstanceID(service), Definition: pvDefinition(exportCap), BindingID: semreg.NativeBindingID(binding),
			SourceEpochID: stamp.epoch, DriverGeneration: stamp.generation, Qualification: semreg.QualificationQualified, Availability: semreg.AvailabilityAvailable,
			Constraints: []semreg.TypedField{{ID: exportArg, Value: pvWattsValue(item.constraint)}}, ActivationEvidence: ordered(anchor[0], recorded), Revision: "1",
		})
	}
}

func inconclusiveRead(c driver.Completion, material string) driver.Completion {
	ev := Evidence(operation.ContractOperationV1, kindRead, material)
	c.Readback = &operation.Readback{Relation: operation.ReadbackInconclusive, Evidence: ordered(ev)}
	c.OutcomeEvidence = ordered(append(append([]semreg.EvidenceRef{}, c.OutcomeEvidence...), ev)...)
	return c
}

// exportConflictWatts is the quantity correlated after a re-read that contradicts a written point.
// Only a power row decoded from this dispatch's reply that is not the request qualifies. When the
// power row equals the request there is no second watts reading, so no candidate is published.
func exportConflictWatts(decoded huawei.Watts, decodedOK bool, requested semreg.Decimal) (semreg.Decimal, bool) {
	if decodedOK {
		if got := wattsDecimal(decoded); !sameDecimal(got, requested) {
			return got, true
		}
	}
	return semreg.Decimal{}, false
}

// contradictionUnrecorded keeps a verified contradicting read when the later
// snapshot cannot be published. The acknowledgement is cleared so the send is
// not reported as accepted, and no candidate is invented.
func contradictionUnrecorded(c driver.Completion, route operation.Route, material string) driver.Completion {
	ev := Evidence(operation.ContractOperationV1, kindRead, material)
	c.Outcome = operation.OutcomeIndeterminate
	c.Acknowledgement = nil
	c.ReadbackSnapshot = nil
	c.Readback = &operation.Readback{
		BindingID: route.BindingID, SourceID: route.SourceID, SourceEpochID: route.SourceEpochID, DriverGeneration: route.DriverGeneration,
		Relation: operation.ReadbackContradicts, Evidence: ordered(ev),
	}
	if c.CompoundExecution == nil {
		c.CompoundExecution = &driver.CompoundExecution{}
	}
	c.CompoundExecution.Steps = append(c.CompoundExecution.Steps, driver.CompoundStep{Point: "readback", Attempted: true, ReadbackRelation: operation.ReadbackContradicts})
	c.OutcomeEvidence = ordered(append(append([]semreg.EvidenceRef{}, c.OutcomeEvidence...), ev)...)
	return c
}

func contradictWithoutCandidate(c driver.Completion, snap semreg.Snapshot, route operation.Route, material string) driver.Completion {
	ev := Evidence(operation.ContractOperationV1, kindRead, material)
	c.Outcome = operation.OutcomeConflict
	c.ReadbackSnapshot = &snap
	c.Readback = &operation.Readback{
		SnapshotID: snap.SnapshotID, Revisions: snap.Revisions,
		BindingID: route.BindingID, SourceID: route.SourceID, SourceEpochID: route.SourceEpochID, DriverGeneration: route.DriverGeneration,
		Relation: operation.ReadbackContradicts, Evaluation: semreg.EvaluationContext{EvaluatedAt: snap.EvaluatedAt, EvaluateMonotonic: snap.EvaluateMonotonic},
		Evidence: ordered(ev),
	}
	if c.CompoundExecution == nil {
		c.CompoundExecution = &driver.CompoundExecution{}
	}
	c.CompoundExecution.Steps = append(c.CompoundExecution.Steps, driver.CompoundStep{Point: "readback", Attempted: true, ReadbackRelation: operation.ReadbackContradicts})
	c.OutcomeEvidence = ordered(append(append([]semreg.EvidenceRef{}, c.OutcomeEvidence...), ev)...)
	return c
}

func exportIDs(label string) (binding, service, capability string) {
	return "binding:huawei-export:" + label, "service:huawei-system:" + label, "capability:huawei-export:" + label
}

func exportBinding(label string) string { return "binding:huawei-export:" + label }

func exportPath(label string) string { return "huawei.controls.export_limit." + label }

func exportFactKey(resource string) semreg.FactKey {
	return semreg.FactKey{PackID: pvPackID, PackVersion: pvPackVersion, FactID: exportArg, Dimensions: []semreg.Dimension{{ID: exportDim, Value: semreg.Value{Kind: semreg.ValueText, Text: &resource}}}}
}

func exportResourceID(label string) semreg.ResourceID {
	sum := sha256.Sum256([]byte("dongle-system:" + label))
	return semreg.ResourceID("resource:dongle-system:" + hex.EncodeToString(sum[:4]))
}
