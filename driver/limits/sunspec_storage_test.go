package limits

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime/life"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

const (
	storageSunS      = 40000
	storageModel     = 40002
	storageModeReg   = storageModel + 5
	storageOutReg    = storageModel + 12
	storageInReg     = storageModel + 13
	storageRevert    = storageModel + 15
	storageRated     = 40030
	storageRatedMode = storageRated + 5
	storageRatedOut  = storageRated + 12
	storageUnrelated = uint16(0x8004)
)

type storageFrame struct {
	unit  uint8
	addr  uint16
	words []uint16
}

type storageBank struct {
	mu      sync.Mutex
	units   map[uint8][]uint16
	frames  []storageFrame
	mode    string
	writes  int
	onWrite func(int)
}

func (b *storageBank) ReadHolding(_ context.Context, unit uint8, address, quantity uint16) ([]uint16, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	cells := b.units[unit]
	if cells == nil || int(address)+int(quantity) > len(cells) {
		return nil, errors.New("synthetic read outside image")
	}
	return append([]uint16(nil), cells[address:address+quantity]...), nil
}

func (b *storageBank) WriteMultiple(_ context.Context, unit uint8, address uint16, registers []uint16, admission modbus.RegisterWriteAdmission) (modbus.WriteMultipleRegistersAcknowledgement, error) {
	qty := uint16(len(registers))
	if admission != nil && !admission(modbus.RegisterWriteAdmissionRequest{UnitID: unit, Function: modbus.FunctionWriteMultipleRegisters, Address: address, Quantity: qty, Registers: registers}) {
		return modbus.WriteMultipleRegistersAcknowledgement{}, errors.New("synthetic admission refused")
	}
	b.mu.Lock()
	b.writes++
	n := b.writes
	mode := b.mode
	cells := b.units[unit]
	b.frames = append(b.frames, storageFrame{unit: unit, addr: address, words: append([]uint16(nil), registers...)})
	fault, apply := storageFault(mode, n)
	if mode == "conflict" && n == 1 && cells != nil && int(address) < len(cells) {
		cells[address] = 0
		apply = false
	}
	if apply && cells != nil && int(address)+len(registers) <= len(cells) {
		copy(cells[address:], registers)
	}
	hook := b.onWrite
	b.mu.Unlock()
	if hook != nil {
		hook(n)
	}
	if fault != nil {
		return modbus.WriteMultipleRegistersAcknowledgement{}, fault
	}
	return modbus.WriteMultipleRegistersAcknowledgement{Address: address, Quantity: qty}, nil
}

func storageFault(mode string, n int) (error, bool) {
	switch {
	case mode == "drop-second" && n == 2:
		return errors.New("synthetic response dropped"), true
	case mode == "reject-second" && n == 2:
		return &modbus.ProtocolError{Kind: modbus.ErrorExceptionResponse, RequestedFunction: modbus.FunctionWriteMultipleRegisters, ReceivedFunction: 0x90, ExceptionCode: 0x02}, false
	default:
		return nil, true
	}
}

func (b *storageBank) written() []storageFrame {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]storageFrame, len(b.frames))
	copy(out, b.frames)
	return out
}

func (b *storageBank) word(unit uint8, addr uint16) uint16 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.units[unit][addr]
}

func storageCells(kind string) []uint16 {
	cells := make([]uint16, 42000)
	cells[storageSunS], cells[storageSunS+1] = 0x5375, 0x6e53
	switch kind {
	case "end":
		cells[storageModel], cells[storageModel+1] = 0xffff, 0
	case "nobound":
		putStorageControls(cells, storageModel, 0xffff, storageUnrelated, 0, 0)
		cells[storageModel+26], cells[storageModel+27] = 0xffff, 0
	case "rated":
		putStorageNameplate(cells, storageModel, 3000)
		putStorageControls(cells, storageRated, 8000, storageUnrelated, 111, 222)
		cells[storageRated+26], cells[storageRated+27] = 0xffff, 0
	default:
		putStorageControls(cells, storageModel, 8000, storageUnrelated, 111, 222)
		cells[storageModel+26], cells[storageModel+27] = 0xffff, 0
	}
	return cells
}

func putStorageControls(cells []uint16, header, chargeMax, mode, out, in uint16) {
	cells[header], cells[header+1] = 124, 24
	cells[header+2] = chargeMax
	cells[header+3], cells[header+4] = 1, 1
	cells[header+5] = mode
	cells[header+6] = 1
	cells[header+11] = 1
	cells[header+12], cells[header+13] = out, in
	cells[header+14], cells[header+15], cells[header+16] = 5, 6, 7
	cells[header+25] = 0xffff
}

func putStorageNameplate(cells []uint16, header, maxDischarge uint16) {
	cells[header], cells[header+1] = 120, 26
	cells[header+2] = 82
	cells[header+3], cells[header+5], cells[header+12] = 1, 1, 1
	cells[header+25] = maxDischarge
	cells[header+27] = 0x8000
}

type storageRecording struct {
	*SunSpecStorage
	startErr error
}

func (r *storageRecording) Start(ctx context.Context, s driver.Session) error {
	r.startErr = r.SunSpecStorage.Start(ctx, s)
	return r.startErr
}

type storageFactory struct{ family *storageRecording }

func (storageFactory) Descriptor() driver.Descriptor { return driver.Descriptor{} }
func (storageFactory) Plan(c driver.Config) ([]driver.InstancePlan, error) {
	return []driver.InstancePlan{{Driver: limitsKind, Instance: driver.InstanceKey(c.Instance)}}, nil
}
func (f storageFactory) Construct(context.Context, driver.InstancePlan) (driver.Instance, error) {
	return f.family, nil
}

func openStorage(t *testing.T, access Registers, batteries []Battery) *SunSpecStorage {
	t.Helper()
	rec := &storageRecording{SunSpecStorage: &SunSpecStorage{cfg: StorageConfig{Access: access, Batteries: batteries}}}
	ctx := context.Background()
	clock := &limitClock{now: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}
	cfg := life.InstanceConfig{Driver: limitsKind, Instance: "limits.storage", Enabled: true, Config: driver.Config{Instance: "limits.storage", Enabled: true}}
	rt, err := runtime.OpenHost(ctx, life.Installation{Key: "installation.sunspec-storage", Bridge: "bridge.test", StateDir: t.TempDir(), Clock: clock, Instances: []life.InstanceConfig{cfg}}, runtime.HostOptions{}, storageFactory{rec})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if err = rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err = rt.WaitInitial(ctx); err != nil {
		t.Fatal(err)
	}
	if rec.startErr != nil {
		status, snapErr := rt.Snapshot(life.Ref{Driver: limitsKind, Instance: "limits.storage"})
		t.Fatalf("family start: %v; snapshot %s %v (%v)", rec.startErr, status.ObservedState, status.Reason, snapErr)
	}
	return rec.SunSpecStorage
}

func storageBattery() Battery { return Battery{Unit: 1, Label: "battery-a", Base: storageSunS} }

func invokeStorage(t *testing.T, family *SunSpecStorage, opID semreg.DefinitionID, arg *semreg.TypedField) driver.OperationResult {
	t.Helper()
	cut, err := family.session.SemanticInput().Capture(context.Background(), driver.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	var route driver.SettingRoute
	found := false
	for _, item := range cut.SettingRoutes {
		if item.Operation.ID == opID {
			route, found = item, true
		}
	}
	if !found {
		t.Fatalf("route %s missing", opID)
	}
	var snap semreg.Snapshot
	var revision semreg.Uint64
	var constraint semreg.Decimal
	have := false
	for _, item := range cut.Snapshots {
		for _, capability := range item.Capabilities {
			if capability.InstanceID != route.Route.CapabilityInstance || capability.AssetID != item.AssetID {
				continue
			}
			if len(capability.Constraints) != 1 || capability.Constraints[0].Value.Quantity == nil {
				t.Fatalf("constraint %+v", capability.Constraints)
			}
			snap, revision, constraint, have = item, capability.Revision, capability.Constraints[0].Value.Quantity.Number, true
		}
	}
	if !have {
		t.Fatal("capability missing")
	}
	field := semreg.TypedField{ID: route.For.Key.FactID, Value: semreg.Value{Kind: semreg.ValueQuantity, Quantity: &semreg.Quantity{Number: constraint, Unit: wattUnit}}}
	if arg != nil {
		field = *arg
	}
	result, err := family.session.ScopedOperations().Invoke(context.Background(), driver.OperationRequest{
		AssetID: snap.AssetID, BindingID: route.Route.BindingID, CapabilityInstanceID: route.Route.CapabilityInstance,
		Operation: route.Operation, Arguments: []semreg.TypedField{field}, Preconditions: []operation.Precondition{}, Timeout: 5 * time.Second,
		ExpectedSemanticRevision: snap.Revisions.Semantic, ExpectedCapabilityRevision: snap.Revisions.Capabilities,
		ExpectedCapabilityInstanceRevision: revision, ExpectedDriverGeneration: route.Route.DriverGeneration,
		ExpectedSourceEpochID: route.Route.SourceEpochID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func ownerForOperation(t *testing.T, family *SunSpecStorage, operation semreg.DefinitionID) storageOwner {
	t.Helper()
	for _, own := range family.owners {
		if own.route.Operation.ID == operation {
			return own
		}
	}
	t.Fatalf("owner %s missing", operation)
	return storageOwner{}
}

func storageDirect(own storageOwner, number semreg.Decimal) driver.Dispatch {
	quantity := semreg.Quantity{Number: number, Unit: wattUnit}
	return driver.Dispatch{
		Request: driver.OperationRequest{Arguments: []semreg.TypedField{{ID: own.argument, Value: semreg.Value{Kind: semreg.ValueQuantity, Quantity: &quantity}}}, Timeout: time.Second},
		Route:   own.route.Route,
		Guard: operation.GuardClaims{
			AssetID: own.asset, BindingID: own.route.Route.BindingID, SourceID: own.route.Route.SourceID, SourceEpochID: own.route.Route.SourceEpochID,
			DriverGeneration: own.route.Route.DriverGeneration, CapabilityInstance: own.route.Route.CapabilityInstance, ServiceInstance: own.route.Route.ServiceInstance,
		},
		AdmittedAt: wallNow(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)),
	}
}

func publishStorageFence(t *testing.T, family *SunSpecStorage) {
	t.Helper()
	own := family.owners[0]
	ctx := context.Background()
	tx, err := family.session.Publisher().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	batch := semreg.PublicationBatch{GenerationFences: []semreg.GenerationFence{{
		SourceID: own.route.Route.SourceID, SourceEpochID: own.route.Route.SourceEpochID, DriverGeneration: own.route.Route.DriverGeneration,
		Reason: "lifecycle.driver_replaced", Evidence: []semreg.EvidenceRef{evidence("evidence.lifecycle", "generation-retired")}, Revision: "1",
	}}}
	if err = tx.Stage(batch); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func requireRateThenMode(t *testing.T, frames []storageFrame, rate, mode uint16, word uint16) {
	t.Helper()
	if len(frames) != 2 || frames[0].addr != rate || frames[1].addr != mode || len(frames[0].words) != 1 || frames[0].words[0] != word || len(frames[1].words) != 1 {
		t.Fatalf("frames %+v, want %d then %d word %d", frames, rate, mode, word)
	}
}

func TestSunSpecStorageChargeAndDischargeWriteRateThenMode(t *testing.T) {
	bank := &storageBank{units: map[uint8][]uint16{1: storageCells("controls")}}
	family := openStorage(t, bank, []Battery{storageBattery()})
	if len(bank.written()) != 0 {
		t.Fatal("publication wrote")
	}
	charge := invokeStorage(t, family, opChargeLimit, nil)
	frames := bank.written()
	if charge.Status != "readback_confirmed" || charge.KernelOutcome != operation.OutcomeApplied {
		t.Fatalf("charge %+v", charge)
	}
	requireRateThenMode(t, frames, storageInReg, storageModeReg, 1000)
	if frames[1].words[0] != storageUnrelated|storageChargeBit {
		t.Fatalf("charge mode word %d", frames[1].words[0])
	}
	if bank.word(1, storageOutReg) != 111 || bank.word(1, storageRevert) != 6 || bank.word(1, storageModeReg) != storageUnrelated|storageChargeBit {
		t.Fatalf("image out %d revert %d mode %d", bank.word(1, storageOutReg), bank.word(1, storageRevert), bank.word(1, storageModeReg))
	}
	discharge := invokeStorage(t, family, opDischargeLimit, nil)
	next := bank.written()[len(frames):]
	if discharge.Status != "readback_confirmed" || discharge.KernelOutcome != operation.OutcomeApplied {
		t.Fatalf("discharge %+v", discharge)
	}
	requireRateThenMode(t, next, storageOutReg, storageModeReg, 1000)
	if bank.word(1, storageModeReg) != storageUnrelated|storageChargeBit|storageDisBit || bank.word(1, storageInReg) != 1000 || bank.word(1, storageRevert) != 6 {
		t.Fatalf("image in %d revert %d mode %d", bank.word(1, storageInReg), bank.word(1, storageRevert), bank.word(1, storageModeReg))
	}
	if charge.CompoundExecution == nil || charge.CompoundExecution.Partial || len(charge.CompoundExecution.Steps) != 2 {
		t.Fatalf("compound %+v", charge.CompoundExecution)
	}
}

func TestSunSpecStorageFirstEchoIsNotConfirmed(t *testing.T) {
	bank := &storageBank{units: map[uint8][]uint16{1: storageCells("controls")}, mode: "drop-second"}
	family := openStorage(t, bank, []Battery{storageBattery()})
	result := invokeStorage(t, family, opChargeLimit, nil)
	frames := bank.written()
	if result.Status == "readback_confirmed" || result.Status != "unknown" || result.KernelOutcome != operation.OutcomeIndeterminate {
		t.Fatalf("result %+v", result)
	}
	if result.Delivery == operation.DeliveryNotSent || result.KernelOutcome == operation.OutcomeFailedNoContact {
		t.Fatalf("lost response looked unsent %+v", result)
	}
	requireRateThenMode(t, frames, storageInReg, storageModeReg, 1000)
	if result.CompoundExecution == nil || !result.CompoundExecution.Partial || len(result.CompoundExecution.Steps) != 2 {
		t.Fatalf("compound %+v", result.CompoundExecution)
	}
	if !result.CompoundExecution.Steps[0].Attempted || result.CompoundExecution.Steps[0].Acknowledgement != operation.AckAccepted || result.CompoundExecution.Steps[1].Delivery != operation.DeliveryUnknown {
		t.Fatalf("steps %+v", result.CompoundExecution.Steps)
	}
	if len(bank.written()) != 2 {
		t.Fatalf("writes %d", len(bank.written()))
	}
}

func TestSunSpecStorageLaterRejectionGuardAndLoss(t *testing.T) {
	t.Run("rejection", func(t *testing.T) {
		bank := &storageBank{units: map[uint8][]uint16{1: storageCells("controls")}, mode: "reject-second"}
		family := openStorage(t, bank, []Battery{storageBattery()})
		result := invokeStorage(t, family, opChargeLimit, nil)
		if result.Status == "readback_confirmed" || result.Status != "failed" || result.KernelOutcome != operation.OutcomeNoEffect || result.Delivery == operation.DeliveryNotSent {
			t.Fatalf("result %+v", result)
		}
		frames := bank.written()
		requireRateThenMode(t, frames, storageInReg, storageModeReg, 1000)
		if bank.word(1, storageModeReg) != storageUnrelated || bank.word(1, storageInReg) != 1000 || bank.word(1, storageRevert) != 6 {
			t.Fatalf("image in %d mode %d revert %d", bank.word(1, storageInReg), bank.word(1, storageModeReg), bank.word(1, storageRevert))
		}
		if result.CompoundExecution == nil || !result.CompoundExecution.Partial || result.CompoundExecution.Steps[1].Acknowledgement != operation.AckRejected {
			t.Fatalf("compound %+v", result.CompoundExecution)
		}
	})
	t.Run("guard", func(t *testing.T) {
		bank := &storageBank{units: map[uint8][]uint16{1: storageCells("controls")}}
		family := openStorage(t, bank, []Battery{storageBattery()})
		bank.onWrite = func(n int) {
			if n == 1 {
				publishStorageFence(t, family)
			}
		}
		result := invokeStorage(t, family, opChargeLimit, nil)
		frames := bank.written()
		if len(frames) != 1 || frames[0].addr != storageInReg || result.Status == "readback_confirmed" || result.Delivery == operation.DeliveryNotSent {
			t.Fatalf("result %+v frames %+v", result, frames)
		}
		if result.CompoundExecution == nil || !result.CompoundExecution.Partial || result.CompoundExecution.Steps[1].Attempted || result.CompoundExecution.Steps[1].Reason != "guard_mismatch" {
			t.Fatalf("compound %+v", result.CompoundExecution)
		}
		if bank.word(1, storageModeReg) != storageUnrelated || bank.word(1, storageRevert) != 6 {
			t.Fatal("mode or revert changed")
		}
	})
	t.Run("loss", func(t *testing.T) {
		bank := &storageBank{units: map[uint8][]uint16{1: storageCells("controls")}, mode: "drop-second"}
		family := openStorage(t, bank, []Battery{storageBattery()})
		result := invokeStorage(t, family, opDischargeLimit, nil)
		if result.Status != "unknown" || result.KernelOutcome == operation.OutcomeFailedNoContact || len(bank.written()) != 2 {
			t.Fatalf("result %+v writes %d", result, len(bank.written()))
		}
		if result.CompoundExecution.Steps[0].Acknowledgement != operation.AckAccepted || result.CompoundExecution.Steps[1].Delivery != operation.DeliveryUnknown {
			t.Fatalf("steps %+v", result.CompoundExecution.Steps)
		}
	})
}

func TestSunSpecStorageConcurrentDirectionsKeepBothBits(t *testing.T) {
	bank := &storageBank{units: map[uint8][]uint16{1: storageCells("controls")}}
	family := openStorage(t, bank, []Battery{storageBattery()})
	charge := storageDirect(ownerForOperation(t, family, opChargeLimit), semreg.Decimal{Coefficient: "8", Exponent10: 3})
	discharge := storageDirect(ownerForOperation(t, family, opDischargeLimit), semreg.Decimal{Coefficient: "8", Exponent10: 3})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := family.Execute(context.Background(), charge); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := family.Execute(context.Background(), discharge); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()
	frames := bank.written()
	if len(frames) != 4 || frames[1].addr != storageModeReg || frames[3].addr != storageModeReg {
		t.Fatalf("frames %+v", frames)
	}
	if frames[0].addr == frames[2].addr || (frames[0].addr != storageInReg && frames[0].addr != storageOutReg) {
		t.Fatalf("rates %+v", frames)
	}
	if bank.word(1, storageModeReg) != storageUnrelated|storageChargeBit|storageDisBit || bank.word(1, storageInReg) != 1000 || bank.word(1, storageOutReg) != 1000 || bank.word(1, storageRevert) != 6 {
		t.Fatalf("in %d out %d mode %d revert %d", bank.word(1, storageInReg), bank.word(1, storageOutReg), bank.word(1, storageModeReg), bank.word(1, storageRevert))
	}
}

func TestSunSpecStorageDischargeUsesChargeMaximum(t *testing.T) {
	bank := &storageBank{units: map[uint8][]uint16{1: storageCells("controls")}}
	family := openStorage(t, bank, []Battery{storageBattery()})
	cut, err := family.session.SemanticInput().Capture(context.Background(), driver.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	caps := map[semreg.DefinitionID]semreg.Decimal{}
	var held *semreg.Snapshot
	for i := range cut.Snapshots {
		for _, capability := range cut.Snapshots[i].Capabilities {
			if len(capability.Constraints) != 1 || capability.Constraints[0].Value.Quantity == nil {
				t.Fatalf("capability %+v", capability)
			}
			caps[capability.Definition.ID] = capability.Constraints[0].Value.Quantity.Number
			held = &cut.Snapshots[i]
		}
	}
	if held == nil {
		t.Fatal("capability snapshot missing")
	}
	services := 0
	for _, service := range held.Services {
		if service.Definition.ID == serviceID {
			services++
			if service.Resource == nil || string(*service.Resource) != storageResourceDigest("resource:sunspec-storage:", "sunspec-storage:battery-a") {
				t.Fatalf("service %+v", service)
			}
		}
	}
	if services != 1 {
		t.Fatalf("services %d", services)
	}
	if len(caps) != 2 || caps[capDischargeLimit].Coefficient != "8" || caps[capChargeLimit].Coefficient != "8" {
		t.Fatalf("capabilities %+v", caps)
	}
	result := invokeStorage(t, family, opDischargeLimit, nil)
	if result.Status != "readback_confirmed" {
		t.Fatalf("result %+v", result)
	}
	requireRateThenMode(t, bank.written(), storageOutReg, storageModeReg, 1000)
	if bank.word(1, storageInReg) != 222 || bank.word(1, storageRevert) != 6 {
		t.Fatal("discharge write touched the charge rate or the revert time")
	}
}

func TestSunSpecStorageSeparateDischargeMaximum(t *testing.T) {
	bank := &storageBank{units: map[uint8][]uint16{1: storageCells("rated")}}
	family := openStorage(t, bank, []Battery{storageBattery()})
	cut, err := family.session.SemanticInput().Capture(context.Background(), driver.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range cut.SettingRoutes {
		if route.Operation.ID == opDischargeLimit && (route.Limits == nil || route.Limits.Max == nil || route.Limits.Max.Number.Coefficient != "3" || route.Limits.Max.Number.Exponent10 != 3) {
			t.Fatalf("discharge limit %+v", route.Limits)
		}
		if route.Operation.ID == opChargeLimit && (route.Limits == nil || route.Limits.Max == nil || route.Limits.Max.Number.Coefficient != "8") {
			t.Fatalf("charge limit %+v", route.Limits)
		}
	}
	result := invokeStorage(t, family, opDischargeLimit, nil)
	if result.Status != "readback_confirmed" {
		t.Fatalf("result %+v", result)
	}
	requireRateThenMode(t, bank.written(), storageRatedOut, storageRatedMode, 1000)
	if bank.written()[0].words[0] == 375 {
		t.Fatal("discharge was encoded from WChaMax")
	}
}

func TestSunSpecStorageContradictingReadbackIsConflict(t *testing.T) {
	bank := &storageBank{units: map[uint8][]uint16{1: storageCells("controls")}, mode: "conflict"}
	family := openStorage(t, bank, []Battery{storageBattery()})
	result := invokeStorage(t, family, opChargeLimit, nil)
	if result.Status != "failed" || result.KernelOutcome != operation.OutcomeConflict || result.KernelOutcome == operation.OutcomeIndeterminate || result.Status == "readback_confirmed" || result.Status == "unknown" {
		t.Fatalf("result %+v", result)
	}
	frames := bank.written()
	requireRateThenMode(t, frames, storageInReg, storageModeReg, 1000)
	if bank.word(1, storageInReg) != 0 {
		t.Fatalf("stored rate %d", bank.word(1, storageInReg))
	}
}

func TestSunSpecStorageRefusalsSendNothing(t *testing.T) {
	bank := &storageBank{units: map[uint8][]uint16{1: storageCells("controls"), 2: storageCells("end"), 3: storageCells("nobound")}}
	family := openStorage(t, bank, []Battery{storageBattery(), {Unit: 2, Label: "battery-b", Base: storageSunS}, {Unit: 3, Label: "battery-c", Base: storageSunS}})
	if len(family.owners) != 2 || len(bank.written()) != 0 {
		t.Fatalf("owners %d writes %d", len(family.owners), len(bank.written()))
	}
	own := ownerForOperation(t, family, opChargeLimit)
	// storageGuard compares the admitted route with the owner route and compares guard asset, binding, source, epoch, generation, capability and service.
	// Each case changes one of those fields. Dropping that comparison from storageGuard lets the case write.
	base := storageDirect(own, semreg.Decimal{Coefficient: "8", Exponent10: 3})
	guards := []struct {
		name string
		edit func(*driver.Dispatch)
	}{
		{"guard asset", func(d *driver.Dispatch) { d.Guard.AssetID = "asset:other" }},
		{"guard binding", func(d *driver.Dispatch) { d.Guard.BindingID = "binding:other" }},
		{"guard source", func(d *driver.Dispatch) { d.Guard.SourceID = "source:other" }},
		{"guard epoch", func(d *driver.Dispatch) { d.Guard.SourceEpochID = "epoch:other" }},
		{"guard generation", func(d *driver.Dispatch) { d.Guard.DriverGeneration = "9" }},
		{"guard capability", func(d *driver.Dispatch) { d.Guard.CapabilityInstance = "capability:other" }},
		{"guard service", func(d *driver.Dispatch) { d.Guard.ServiceInstance = "service:other" }},
		{"route capability", func(d *driver.Dispatch) { d.Route.CapabilityInstance = "capability:other" }},
		{"route service", func(d *driver.Dispatch) { d.Route.ServiceInstance = "service:other" }},
		{"route binding", func(d *driver.Dispatch) { d.Route.BindingID = "binding:other" }},
		{"route source", func(d *driver.Dispatch) { d.Route.SourceID = "source:other" }},
		{"route epoch", func(d *driver.Dispatch) { d.Route.SourceEpochID = "epoch:other" }},
		{"route generation", func(d *driver.Dispatch) { d.Route.DriverGeneration = "9" }},
	}
	for _, tc := range guards {
		dispatch := base
		tc.edit(&dispatch)
		refused, err := family.Execute(context.Background(), dispatch)
		if err != nil || refused.CompoundExecution == nil || refused.CompoundExecution.Steps[0].Reason != "guard_mismatch" || len(bank.written()) != 0 {
			t.Fatalf("%s %+v %v writes %d", tc.name, refused.CompoundExecution, err, len(bank.written()))
		}
	}
	negative, err := family.Execute(context.Background(), storageDirect(own, semreg.Decimal{Coefficient: "-1"}))
	if err != nil || negative.CompoundExecution.Steps[0].Reason != "negative_set" || len(bank.written()) != 0 {
		t.Fatalf("negative %+v %v", negative.CompoundExecution, err)
	}
	above, err := family.Execute(context.Background(), storageDirect(own, semreg.Decimal{Coefficient: "9", Exponent10: 3}))
	if err != nil || above.CompoundExecution.Steps[0].Reason != "out_of_bound" || above.Dispatch.Delivery != operation.DeliveryNotSent || len(bank.written()) != 0 {
		t.Fatalf("above %+v %v writes %d", above, err, len(bank.written()))
	}
	bank.units[1][storageModel], bank.units[1][storageModel+1] = 0xffff, 0
	absent, err := family.Execute(context.Background(), storageDirect(own, semreg.Decimal{Coefficient: "8", Exponent10: 3}))
	if err != nil || absent.CompoundExecution.Steps[0].Reason != "model_absent" || len(bank.written()) != 0 {
		t.Fatalf("absent %+v %v writes %d", absent.CompoundExecution, err, len(bank.written()))
	}
	quantity := semreg.Quantity{Number: semreg.Decimal{Coefficient: "-4", Exponent10: 3}, Unit: wattUnit}
	negativeArg := semreg.TypedField{ID: argChargeLimit, Value: semreg.Value{Kind: semreg.ValueQuantity, Quantity: &quantity}}
	invoked := invokeStorage(t, family, opChargeLimit, &negativeArg)
	if invoked.Reason != "semantic.operation.reason.admission:bounds_exceeded" || invoked.Delivery != operation.DeliveryNotSent || len(bank.written()) != 0 {
		t.Fatalf("invoke %+v writes %d", invoked, len(bank.written()))
	}
}

func TestSunSpecStorageExecutorsExistOnlyWhileRunning(t *testing.T) {
	firstBank := &storageBank{units: map[uint8][]uint16{1: storageCells("controls")}}
	first := openStorage(t, firstBank, []Battery{storageBattery()})
	resource, constraint, limit := storageGraph(t, first)
	if resource != storageResourceDigest("resource:sunspec-storage:", "sunspec-storage:battery-a") || constraint.Coefficient != "8" || limit.Coefficient != "8" {
		t.Fatalf("graph %s %+v %+v", resource, constraint, limit)
	}
	secondBank := &storageBank{units: map[uint8][]uint16{1: storageCells("controls")}}
	second := openStorage(t, secondBank, []Battery{storageBattery()})
	otherResource, otherConstraint, otherLimit := storageGraph(t, second)
	if otherResource != resource || otherConstraint != constraint || otherLimit != limit {
		t.Fatalf("graphs differ %s %+v %+v and %s %+v %+v", resource, constraint, limit, otherResource, otherConstraint, otherLimit)
	}
	if len(second.owners) != 2 {
		t.Fatalf("owners %d", len(second.owners))
	}
	if report := second.Stop(context.Background()); !report.Confirmed {
		t.Fatal("stop was not confirmed")
	}
	if len(second.owners) != 0 {
		t.Fatal("owners remained after stop")
	}
	stopped, err := second.Execute(context.Background(), storageDirect(ownerForOperation(t, first, opChargeLimit), semreg.Decimal{Coefficient: "8", Exponent10: 3}))
	if err != nil || stopped.CompoundExecution.Steps[0].Reason != "stopped" || len(secondBank.written()) != 0 {
		t.Fatalf("stopped %+v %v writes %d", stopped.CompoundExecution, err, len(secondBank.written()))
	}
	result := invokeStorage(t, second, opChargeLimit, nil)
	if result.Reason != "semantic.operation.reason.executor_missing" || len(secondBank.written()) != 0 {
		t.Fatalf("missing %+v writes %d", result, len(secondBank.written()))
	}
}

func storageResourceDigest(prefix, material string) string {
	sum := sha256.Sum256([]byte(material))
	return prefix + hex.EncodeToString(sum[:4])
}

func storageGraph(t *testing.T, family *SunSpecStorage) (string, semreg.Decimal, semreg.Decimal) {
	t.Helper()
	cut, err := family.session.SemanticInput().Capture(context.Background(), driver.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	var resource string
	var constraint semreg.Decimal
	var limit semreg.Decimal
	caps := 0
	for _, snap := range cut.Snapshots {
		for _, item := range snap.Resources {
			if item.Kind == "resource.storage_interface" {
				resource = string(item.ResourceID)
			}
		}
		for _, capability := range snap.Capabilities {
			caps++
			constraint = capability.Constraints[0].Value.Quantity.Number
		}
	}
	if caps != 2 || len(cut.SettingRoutes) != 2 || cut.SettingRoutes[0].Limits == nil || cut.SettingRoutes[0].Limits.Max == nil {
		t.Fatalf("caps %d routes %d", caps, len(cut.SettingRoutes))
	}
	limit = cut.SettingRoutes[0].Limits.Max.Number
	for _, route := range cut.SettingRoutes {
		if route.Limits.Max.Number != limit {
			t.Fatalf("limits differ %+v %+v", limit, route.Limits.Max.Number)
		}
	}
	return resource, constraint, limit
}
