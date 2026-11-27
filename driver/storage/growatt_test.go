package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/growatt"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

type window struct {
	Offset   uint16   `json:"offset"`
	Quantity uint16   `json:"quantity"`
	Words    []uint16 `json:"words"`
}

type directionCase struct {
	Key         string  `json:"key"`
	Raw         uint16  `json:"raw_0x0013"`
	NativeLabel string  `json:"native_label"`
	Coarse      *string `json:"coarse"`
	Direction   *string `json:"direction"`
}

type fixture struct {
	Configuration struct {
		Asset      string `json:"asset"`
		Source     string `json:"source"`
		Epoch      string `json:"epoch"`
		Generation uint64 `json:"generation"`
		Unit       byte   `json:"unit"`
		Battery    string `json:"battery"`
		System     string `json:"system"`
	} `json:"configuration"`
	Clock struct {
		Wall  int64  `json:"wall_unix_nanos"`
		Step  int64  `json:"step_nanos"`
		Epoch string `json:"epoch"`
	} `json:"clock"`
	Revision struct {
		Family, FileRevision, HeaderVersion, CumulativeRevision string
	} `json:"-"`
	BaseRequests []struct {
		Function         int
		Offset, Quantity uint16
	} `json:"base_requests"`
	DiagnosticRequests []struct {
		Function         int
		Offset, Quantity uint16
	} `json:"diagnostic_requests"`
	DiagnosticReplies []window        `json:"diagnostic_replies"`
	Template          []window        `json:"base_template"`
	Cases             []directionCase `json:"cases"`
	OrderedPair       []string        `json:"ordered_pair"`
	Unresolved        struct {
		Key string `json:"key"`
		Raw uint16 `json:"raw_0x0013"`
	} `json:"unresolved"`
	ExpectedCell struct {
		Minimum struct {
			Index      int
			Millivolts uint16
			Volts      string
		}
		Maximum struct {
			Index      int
			Millivolts uint16
			Volts      string
		}
	} `json:"expected_cell"`
	ExpectedCycleCount int `json:"expected_cycle_count"`
}

func loadFixture(t *testing.T) fixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/growatt_rs485_direction.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

var exactRevision = growatt.BMSRevisionTuple{Family: "1xSxxP ESS", FileRevision: "Rev2.01", HeaderVersion: "V2.0", CumulativeRevision: "2.02"}

// windowsFor is the base template with the native state word set.
func (f fixture) windowsFor(raw uint16) []window {
	out := make([]window, len(f.Template))
	for i, w := range f.Template {
		out[i] = window{w.Offset, w.Quantity, append([]uint16(nil), w.Words...)}
		if w.Offset == 0x000d {
			out[i].Words[6] = raw
		}
	}
	return out
}

func (f fixture) status(t *testing.T, raw uint16) growatt.BMSTypedReadOnlyStatus {
	t.Helper()
	var slices []growatt.BMSReadOnlySlice
	for _, w := range f.windowsFor(raw) {
		slices = append(slices, growatt.BMSReadOnlySlice{Offset: w.Offset, Words: w.Words})
	}
	status, err := growatt.DecodeGrowattBMSTypedReadOnlyStatus(growatt.BMSReadOnlyInput{UnitID: f.Configuration.Unit, Function: modbusreg.FunctionReadHoldingRegisters, Revision: exactRevision, Slices: slices})
	if err != nil {
		t.Fatalf("status %d: %v", raw, err)
	}
	return status
}

func (f fixture) diagnostic(t *testing.T, status growatt.BMSTypedReadOnlyStatus) growatt.BMSMultiGroupDiagnostic {
	t.Helper()
	var slices []growatt.BMSReadOnlySlice
	for _, w := range f.DiagnosticReplies {
		slices = append(slices, growatt.BMSReadOnlySlice{Offset: w.Offset, Words: w.Words})
	}
	d, err := growatt.DecodeGrowattBMSMultiGroupDiagnostic(status, growatt.BMSMultiGroupDiagnosticInput{UnitID: f.Configuration.Unit, Function: modbusreg.FunctionReadHoldingRegisters, Revision: exactRevision, Slices: slices})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (f fixture) receipt(n int) Receipt {
	return Receipt{ObservationID: "observation-" + string(rune('a'+n)), Wall: time.Unix(0, f.Clock.Wall+int64(n)*f.Clock.Step), Mono: time.Duration(int64(n) * f.Clock.Step),
		ClockEpoch: f.Clock.Epoch, Evidence: []string{"observation", string(rune('a' + n))}}
}

type call struct {
	unit             byte
	offset, quantity uint16
}

// rtu is a scripted read session that answers from the fixture and records
// every request in order.
type rtu struct {
	replies map[uint16][]uint16
	calls   []call
	fail    map[uint16]error
}

func (f fixture) rtu(raw uint16) *rtu {
	r := &rtu{replies: map[uint16][]uint16{}, fail: map[uint16]error{}}
	for _, w := range f.windowsFor(raw) {
		r.replies[w.Offset] = w.Words
	}
	for _, w := range f.DiagnosticReplies {
		r.replies[w.Offset] = w.Words
	}
	return r
}

func (r *rtu) ReadHolding(_ context.Context, unit byte, req modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error) {
	r.calls = append(r.calls, call{unit, req.Offset(), req.Quantity()})
	if err := r.fail[req.Offset()]; err != nil {
		return modbus.ReadRegistersResponse{}, err
	}
	return modbus.ReadRegistersResponse{
		Provenance: modbus.ReadProvenance{Function: modbus.FunctionReadHoldingRegisters, Table: modbus.HoldingRegisters, Offset: req.Offset(), Quantity: req.Quantity()},
		Words:      append([]uint16(nil), r.replies[req.Offset()]...)}, nil
}

func (f fixture) config() Config {
	c := testConfig()
	c.Battery, c.System = f.Configuration.Battery, f.Configuration.System
	return c
}

func (f fixture) session() *session {
	c := f.Configuration
	return newSession(c.Asset, c.Source, c.Epoch, c.Generation)
}

// packAllocator allocates the pack of the configured asset label at the
// identifier the first incarnation recorded.
func (f fixture) packAllocator(t *testing.T) *allocator {
	t.Helper()
	slot := hashHex("helianthus.gateway.storage.pack_slot/v1", f.Configuration.Asset)[:32]
	return &allocator{fixed: map[string]semreg.ResourceID{"resource.battery_pack||" + slot: semreg.ResourceID(f.Configuration.Asset)}}
}

type ids struct {
	Binding           string                    `json:"binding"`
	Service           string                    `json:"service"`
	PackDimensionText string                    `json:"pack_dimension_text"`
	Candidates        map[string]string         `json:"candidates"`
	RecordedValues    map[string]map[string]any `json:"recorded_values"`
	RepairedValues    map[string]map[string]any `json:"repaired_values"`
}

type incarnation struct {
	Configuration struct {
		Asset      string `json:"asset"`
		Source     string `json:"source"`
		Epoch      string `json:"epoch"`
		Generation uint64 `json:"generation"`
		Unit       byte   `json:"unit"`
		Battery    string `json:"battery"`
		System     string `json:"system"`
	} `json:"configuration"`
	Clock struct {
		Wall  int64  `json:"wall_unix_nanos"`
		Step  int64  `json:"step_nanos"`
		Epoch string `json:"epoch"`
	} `json:"clock"`
	Windows []window `json:"windows"`
	Service struct {
		InstanceID        string `json:"instance_id"`
		AssetID           string `json:"asset_id"`
		PackID            string `json:"pack_id"`
		PackVersion       string `json:"pack_version"`
		DefinitionID      string `json:"definition_id"`
		DefinitionVersion string `json:"definition_version"`
		BindingID         string `json:"binding_id"`
		SourceEpochID     string `json:"source_epoch_id"`
		DriverGeneration  string `json:"driver_generation"`
		Qualification     string `json:"qualification"`
		Availability      string `json:"availability"`
		Revision          string `json:"revision"`
	} `json:"mcp_service"`
}

func loadIncarnation(t *testing.T) incarnation {
	t.Helper()
	raw, err := os.ReadFile("testdata/growatt_first_incarnation_inputs.json")
	if err != nil {
		t.Fatal(err)
	}
	var in incarnation
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func (in incarnation) status(t *testing.T) growatt.BMSTypedReadOnlyStatus {
	t.Helper()
	var slices []growatt.BMSReadOnlySlice
	for _, w := range in.Windows {
		slices = append(slices, growatt.BMSReadOnlySlice{Offset: w.Offset, Words: append([]uint16(nil), w.Words...)})
	}
	status, err := growatt.DecodeGrowattBMSTypedReadOnlyStatus(growatt.BMSReadOnlyInput{UnitID: in.Configuration.Unit, Function: modbusreg.FunctionReadHoldingRegisters, Revision: exactRevision, Slices: slices})
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func (in incarnation) observe(t *testing.T, n int) GrowattObservation {
	t.Helper()
	return GrowattObservation{Status: in.status(t), Receipt: Receipt{ObservationID: "observation-" + string(rune('a'+n)), Wall: time.Unix(0, in.Clock.Wall+int64(n)*in.Clock.Step), Mono: time.Duration(int64(n) * in.Clock.Step),
		ClockEpoch: in.Clock.Epoch, Evidence: []string{"observation", string(rune('a' + n))}}}
}

func loadIDs(t *testing.T) ids {
	t.Helper()
	raw, err := os.ReadFile("testdata/growatt_first_incarnation_ids.json")
	if err != nil {
		t.Fatal(err)
	}
	var v ids
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func (f fixture) observe(t *testing.T, raw uint16, n int, diag bool) GrowattObservation {
	t.Helper()
	status := f.status(t, raw)
	obs := GrowattObservation{Status: status, Receipt: f.receipt(n)}
	if diag {
		d := f.diagnostic(t, status)
		obs.Diagnostic = &d
	}
	return obs
}

// Rows: wrong unit, wrong sign, wrong subject and an invented identity must fail.

func TestFirstIncarnationTranscription(t *testing.T) {
	in, frozen := loadIncarnation(t), loadIDs(t)
	// The donor windows are not the direction template: identity word 1 is 772
	// there and 515 in the template, and the cycle word is 110 there and 47 in
	// the template. Replaying the template fails both checks.
	if len(in.Windows) != 4 || in.Windows[0].Offset != 1 || in.Windows[0].Quantity != 7 || in.Windows[0].Words[1] != 772 ||
		in.Windows[1].Offset != 13 || in.Windows[1].Quantity != 29 || in.Windows[1].Words[6] != 2 || in.Windows[1].Words[17] != 110 ||
		in.Windows[2].Offset != 256 || in.Windows[2].Quantity != 12 || in.Windows[3].Offset != 269 || in.Windows[3].Quantity != 2 {
		t.Fatalf("donor windows %+v", in.Windows)
	}
	if in.Service.InstanceID != frozen.Service || in.Service.BindingID != frozen.Binding {
		t.Fatal("the service comparison does not name the frozen instance")
	}
	if got := in.status(t); got.CycleCount != 110 || got.OperatingState != "charging" {
		t.Fatalf("donor status cycle %d state %s", got.CycleCount, got.OperatingState)
	}
	s := newSession(in.Configuration.Asset, in.Configuration.Source, in.Configuration.Epoch, in.Configuration.Generation)
	cfg := testConfig()
	cfg.Battery, cfg.System = in.Configuration.Battery, in.Configuration.System
	slot := hashHex("helianthus.gateway.storage.pack_slot/v1", in.Configuration.Asset)[:32]
	p := newPublisher(t, cfg, s, &allocator{fixed: map[string]semreg.ResourceID{"resource.battery_pack||" + slot: semreg.ResourceID(in.Configuration.Asset)}})
	ctx := context.Background()
	for n := range []int{1, 2} {
		if err := p.PublishGrowatt(ctx, in.observe(t, n+1)); err != nil {
			t.Fatal(err)
		}
	}
	k := s.k
	if len(k.bindings) != 1 || len(k.services) != 1 || len(k.batches) != 2 {
		t.Fatalf("bindings %d services %d batches %d", len(k.bindings), len(k.services), len(k.batches))
	}
	if _, ok := k.bindings[semreg.NativeBindingID(frozen.Binding)]; !ok {
		t.Fatalf("binding is not the frozen first-incarnation binding: %v", k.bindings)
	}
	service, ok := k.services[semreg.ServiceInstanceID(frozen.Service)]
	if !ok {
		t.Fatalf("service is not the frozen instance: %v", k.services)
	}
	pack := semreg.ResourceID(frozen.PackDimensionText)
	svc := in.Service
	if service.Definition.ID != semreg.DefinitionID(svc.DefinitionID) || service.Definition.Version != semreg.SemanticVersion(svc.DefinitionVersion) ||
		service.Definition.Pack.ID != semreg.DefinitionID(svc.PackID) || service.Definition.Pack.Version != semreg.SemanticVersion(svc.PackVersion) ||
		service.Resource == nil || *service.Resource != pack || service.BindingID != semreg.NativeBindingID(svc.BindingID) ||
		service.AssetID != semreg.AssetID(svc.AssetID) || service.SourceEpochID != semreg.SourceEpochID(svc.SourceEpochID) ||
		service.DriverGeneration != semreg.Uint64(svc.DriverGeneration) || string(service.Revision) != svc.Revision ||
		service.Qualification != semreg.Qualification(svc.Qualification) || service.Availability != semreg.Availability(svc.Availability) {
		t.Fatalf("service record: %+v", service)
	}
	for element, id := range frozen.Candidates {
		c, ok := k.facts[semreg.CandidateID(id)]
		if !ok || c.Key.FactID != semreg.DefinitionID(element) {
			t.Errorf("%s: the frozen candidate %s is not published", element, id)
			continue
		}
		if len(c.Key.Dimensions) != 1 || c.Key.Dimensions[0].ID != dimPack || *c.Key.Dimensions[0].Value.Text != frozen.PackDimensionText {
			t.Errorf("%s: pack dimension key %+v", element, c.Key.Dimensions)
		}
		if c.Origin.OriginID != semreg.OriginID("origin:semantic-storage:"+id[len("candidate:semantic-storage:"):]) {
			t.Errorf("%s: origin %s", element, c.Origin.OriginID)
		}
		if c.Revision != "2" {
			t.Errorf("%s: revision %s, want the second sequence", element, c.Revision)
		}
	}
	// Unchanged values: the first incarnation's own values.
	requireQuantity(t, k, "storage.pack.voltage", "52", 0, unitVolt)
	requireQuantity(t, k, "storage.pack.current", "-1", 0, unitAmpere)
	requireQuantity(t, k, "storage.capacity.charge", "5", -1, unitAmpereHour)
	requireQuantity(t, k, "storage.capacity.discharge", "6", -1, unitAmpereHour)
	// Donor cycle 110, remaining 32 Ah and full charge 50 Ah. The direction
	// template publishes cycle 47, remaining 41.5 Ah and full charge 100 Ah.
	requireQuantity(t, k, "storage.state.cycle_count", "11", 1, unitOne)
	requireQuantity(t, k, "storage.capacity.remaining", "32", 0, unitAmpereHour)
	requireQuantity(t, k, "storage.capacity.full_charge", "5", 1, unitAmpereHour)
	// The two stated unit repairs.
	requireQuantity(t, k, "storage.state.soc", "75", -2, unitRatio)
	requireQuantity(t, k, "storage.temperature.pack", "29815", -2, unitKelvin)
	if got := symbolOf(t, only(t, k, elementOperating)); got != "storage.status.operating.active" {
		t.Errorf("coarse operating %s", got)
	}
	if got := symbolOf(t, only(t, k, elementDirection)); got != "storage.status.charge_direction.charging" {
		t.Errorf("direction %s", got)
	}
	for _, c := range k.facts {
		if c.Key.FactID == elementDirection {
			if _, recorded := frozen.Candidates[string(elementDirection)]; recorded {
				t.Error("the first incarnation recorded no direction")
			}
		}
	}
	// The pack record keeps the first incarnation's anchor: operator slot, digest of the label.
	r := k.resources[pack]
	if r.Kind != kindPack || r.Anchor.Basis != semreg.AnchorOperatorSlot || r.Anchor.Class != semreg.AnchorEquipment || r.Anchor.Slot == nil || *r.Anchor.Slot != slot || r.Anchor.Parent != nil {
		t.Fatalf("pack record: %+v", r)
	}
	// A refresh does not upsert the service, binding or source again.
	second := k.batches[1]
	if len(second.ServiceUpserts) != 0 || len(second.BindingUpserts) != 0 || len(second.SourceUpserts) != 0 || len(second.ResourceUpserts) != 0 {
		t.Fatalf("refresh re-upserted: %d services %d bindings %d sources %d resources", len(second.ServiceUpserts), len(second.BindingUpserts), len(second.SourceUpserts), len(second.ResourceUpserts))
	}
}

func TestSoftStartRemovesTheCoarseCandidateAndPublishesStarting(t *testing.T) {
	f, frozen := loadFixture(t), loadIDs(t)
	s := f.session()
	p := newPublisher(t, f.config(), s, f.packAllocator(t))
	ctx := context.Background()
	if err := p.PublishGrowatt(ctx, f.observe(t, 2, 1, false)); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.k.facts[semreg.CandidateID(frozen.Candidates["storage.status.operating"])]; !ok {
		t.Fatal("the coarse candidate was not published")
	}
	if err := p.PublishGrowatt(ctx, f.observe(t, 0, 2, false)); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.k.facts[semreg.CandidateID(frozen.Candidates["storage.status.operating"])]; ok {
		t.Fatal("the soft start kept the coarse candidate")
	}
	if got := symbolOf(t, only(t, s.k, elementDirection)); got != "storage.status.charge_direction.starting" {
		t.Fatalf("direction %s", got)
	}
	requireQuantity(t, s.k, "storage.pack.voltage", "52", 0, unitVolt)
}

func TestDirectionCasesOnOneBinding(t *testing.T) {
	f := loadFixture(t)
	for _, c := range f.Cases {
		t.Run(c.Key, func(t *testing.T) {
			s := f.session()
			p := newPublisher(t, f.config(), s, f.packAllocator(t))
			if err := p.PublishGrowatt(context.Background(), f.observe(t, c.Raw, 1, false)); err != nil {
				t.Fatal(err)
			}
			operating, direction := factsOf(s.k, elementOperating), factsOf(s.k, elementDirection)
			if c.Coarse == nil && len(operating) != 0 {
				t.Errorf("%s published a coarse state", c.Key)
			}
			if c.Coarse != nil {
				if len(operating) != 1 || symbolOf(t, operating[0]) != "storage.status.operating."+*c.Coarse {
					t.Errorf("coarse %+v", operating)
				}
			}
			if c.Direction == nil && len(direction) != 0 {
				t.Errorf("%s published a direction", c.Key)
			}
			if c.Direction != nil {
				if len(direction) != 1 || symbolOf(t, direction[0]) != "storage.status.charge_direction."+*c.Direction {
					t.Fatalf("direction %+v", direction)
				}
				got := direction[0]
				if *got.Key.Dimensions[0].Value.Text != string(f.Configuration.Asset) {
					t.Errorf("direction subject %s", *got.Key.Dimensions[0].Value.Text)
				}
				var native string
				for _, e := range got.Evidence {
					if e.Kind == "evidence.native" {
						native = string(e.Digest)
					}
				}
				want := evidence("evidence.native", "operating_state", "none", "", "0", c.NativeLabel)
				if native != string(want.Digest) {
					t.Errorf("the native state %q is not in the evidence", c.NativeLabel)
				}
			}
		})
	}
}

func TestChargingThenStandbyWithdrawsOnlyTheDirection(t *testing.T) {
	f := loadFixture(t)
	s := f.session()
	p := newPublisher(t, f.config(), s, f.packAllocator(t))
	ctx := context.Background()
	raws := map[string]uint16{}
	for _, c := range f.Cases {
		raws[c.Key] = c.Raw
	}
	for n, key := range f.OrderedPair {
		if err := p.PublishGrowatt(ctx, f.observe(t, raws[key], n+1, false)); err != nil {
			t.Fatal(err)
		}
	}
	if len(factsOf(s.k, elementDirection)) != 0 {
		t.Fatal("a valid standby left a current direction")
	}
	if got := symbolOf(t, only(t, s.k, elementOperating)); got != "storage.status.operating.standby" {
		t.Fatalf("coarse %s", got)
	}
	requireQuantity(t, s.k, "storage.pack.voltage", "52", 0, unitVolt)
	last := s.k.batches[len(s.k.batches)-1]
	if len(last.FactWithdrawals) != 1 {
		t.Fatalf("withdrawals %v", last.FactWithdrawals)
	}
}

func TestUnresolvedNativeStatePublishesNothing(t *testing.T) {
	f := loadFixture(t)
	s := f.session()
	p := newPublisher(t, f.config(), s, f.packAllocator(t))
	if err := p.PublishGrowatt(context.Background(), f.observe(t, 2, 1, false)); err != nil {
		t.Fatal(err)
	}
	var slices []growatt.BMSReadOnlySlice
	for _, w := range f.windowsFor(f.Unresolved.Raw) {
		slices = append(slices, growatt.BMSReadOnlySlice{Offset: w.Offset, Words: w.Words})
	}
	if _, err := growatt.DecodeGrowattBMSTypedReadOnlyStatus(growatt.BMSReadOnlyInput{UnitID: 1, Function: modbusreg.FunctionReadHoldingRegisters, Revision: exactRevision, Slices: slices}); err == nil {
		t.Fatal("the decoder admitted an unresolved state")
	}
	if len(s.k.batches) != 1 || len(factsOf(s.k, elementDirection)) != 1 {
		t.Fatal("an unresolved observation changed the published state")
	}
}

func TestAddedRowsCapacityCycleCountAndCells(t *testing.T) {
	f := loadFixture(t)
	s := f.session()
	p := newPublisher(t, f.config(), s, f.packAllocator(t))
	if err := p.PublishGrowatt(context.Background(), f.observe(t, 2, 1, true)); err != nil {
		t.Fatal(err)
	}
	requireQuantity(t, s.k, "storage.capacity.remaining", "415", -1, unitAmpereHour)
	requireQuantity(t, s.k, "storage.capacity.full_charge", "1", 2, unitAmpereHour)
	requireQuantity(t, s.k, "storage.state.cycle_count", "47", 0, unitOne)
	if f.ExpectedCycleCount != 47 {
		t.Fatal("the fixture states another cycle count")
	}
	// The cumulative counters stay the counters: the added capacities do not take their elements.
	requireQuantity(t, s.k, "storage.capacity.charge", "5", -1, unitAmpereHour)
	min, max := only(t, s.k, "storage.cell.voltage_minimum"), only(t, s.k, "storage.cell.voltage_maximum")
	if c, e, u := quantityOf(t, min); c != "3295" || e != -3 || u != unitVolt {
		t.Errorf("minimum %s e%d %s", c, e, u)
	}
	if c, e, u := quantityOf(t, max); c != "335" || e != -2 || u != unitVolt {
		t.Errorf("maximum %s e%d %s", c, e, u)
	}
	if min.Key.Dimensions[0].ID != dimCell || max.Key.Dimensions[0].ID != dimCell || *min.Key.Dimensions[0].Value.Text == *max.Key.Dimensions[0].Value.Text {
		t.Errorf("cell subjects: %+v %+v", min.Key.Dimensions, max.Key.Dimensions)
	}
	if f.ExpectedCell.Minimum.Index != 17 || f.ExpectedCell.Maximum.Index != 20 {
		t.Fatal("fixture cells")
	}
}

func TestFailedDiagnosticLeavesTheBasePublicationIdentical(t *testing.T) {
	f := loadFixture(t)
	withBase := f.session()
	p := newPublisher(t, f.config(), withBase, f.packAllocator(t))
	if err := p.PublishGrowatt(context.Background(), f.observe(t, 2, 1, false)); err != nil {
		t.Fatal(err)
	}
	r := f.rtu(2)
	r.fail[0x0071] = errors.New("window failed")
	reader := GrowattReader{Session: r, Unit: 1, Revision: exactRevision, Receipt: func() Receipt { return f.receipt(1) }}
	failing := f.session()
	q := newPublisher(t, f.config(), failing, f.packAllocator(t))
	if err := q.ObserveGrowatt(context.Background(), reader); err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(withBase.k.batches[0])
	b, _ := json.Marshal(failing.k.batches[0])
	if string(a) != string(b) {
		t.Fatalf("a failed diagnostic changed the base publication:\n%s\n%s", a, b)
	}
	if len(factsOf(failing.k, "storage.cell.voltage_minimum")) != 0 {
		t.Fatal("a failed diagnostic published a cell voltage")
	}
}

func readerFor(f fixture, r *rtu, n int) GrowattReader {
	return GrowattReader{Session: r, Unit: f.Configuration.Unit, Revision: exactRevision, Receipt: func() Receipt { return f.receipt(n) }}
}

func TestNativeStatusReadsInOrderServesTheViewAndPublishesNothing(t *testing.T) {
	f := loadFixture(t)
	s := f.session()
	p := newPublisher(t, f.config(), s, f.packAllocator(t))
	ctx := context.Background()
	if err := p.ObserveGrowatt(ctx, readerFor(f, f.rtu(2), 1)); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(s.k.facts)
	revision := s.k.sequence
	// The device reports other values for the native read than the last cut.
	other := f.rtu(3)
	other.replies[0x000d] = append([]uint16(nil), other.replies[0x000d]...)
	other.replies[0x000d][9] = 4800
	got, err := p.NativeStatus(ctx, readerFor(f, other, 5))
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := []call{{1, 1, 7}, {1, 13, 29}, {1, 256, 12}, {1, 269, 2}, {1, 0x70, 1}, {1, 0x71, 16}, {1, 0x81, 16}}
	if !reflect.DeepEqual(other.calls, wantCalls) {
		t.Fatalf("calls %v, want %v", other.calls, wantCalls)
	}
	if got.Status.PackVoltageVolts != 48 || got.Status.OperatingState != growatt.BMSStateDischarging || got.Receipt.ObservationID != f.receipt(5).ObservationID || len(got.Slices) != 4 {
		t.Fatalf("native status: %+v", got.Status)
	}
	if got.Diagnostic == nil || got.Diagnostic.ReportedGroupWord != 7 || len(got.Diagnostic.Windows) != 3 {
		t.Fatalf("diagnostic view: %+v", got.Diagnostic)
	}
	for i, w := range got.Diagnostic.Windows {
		want := f.DiagnosticReplies[i]
		if w.Offset != want.Offset || !reflect.DeepEqual(w.Words, want.Words) {
			t.Errorf("window %d: %+v, want %+v", i, w, want)
		}
	}
	raw := got.Diagnostic.Typed.RawSlices()
	if len(raw) != 5 || raw[1].Offset != 0x70 || raw[2].Offset != 0x71 || len(raw[2].Words) != 16 || raw[3].Offset != 0x81 {
		t.Fatalf("raw slices %+v", raw)
	}
	after, _ := json.Marshal(s.k.facts)
	if string(before) != string(after) || s.k.sequence != revision {
		t.Fatal("the native read changed the semantic cut")
	}
	// The next ordinary observation advances the cut once with its own values.
	if err := p.ObserveGrowatt(ctx, readerFor(f, other, 6)); err != nil {
		t.Fatal(err)
	}
	if s.k.sequence != revision+1 {
		t.Fatalf("revision %d, want %d", s.k.sequence, revision+1)
	}
	requireQuantity(t, s.k, "storage.pack.voltage", "48", 0, unitVolt)
}

func TestNativeStatusOptionalWindowFailureKeepsTheBase(t *testing.T) {
	f := loadFixture(t)
	s := f.session()
	p := newPublisher(t, f.config(), s, f.packAllocator(t))
	ctx := context.Background()
	if err := p.ObserveGrowatt(ctx, readerFor(f, f.rtu(2), 1)); err != nil {
		t.Fatal(err)
	}
	r := f.rtu(3)
	r.fail[0x0081] = errors.New("window failed")
	revision := s.k.sequence
	got, err := p.NativeStatus(ctx, readerFor(f, r, 2))
	if err != nil {
		t.Fatalf("an optional window failure failed the call: %v", err)
	}
	if got.Diagnostic != nil || len(got.Slices) != 4 || got.Status.OperatingState != growatt.BMSStateDischarging || got.Receipt.ObservationID != f.receipt(2).ObservationID {
		t.Fatalf("native status: %+v", got)
	}
	if len(r.calls) != 7 || s.k.sequence != revision {
		t.Fatalf("calls %d revision %d", len(r.calls), s.k.sequence)
	}
}

func TestDiagnosticWindowOrderIsNotAConcatenation(t *testing.T) {
	f := loadFixture(t)
	r := f.rtu(2)
	swapped := append([]uint16(nil), r.replies[0x71]...)
	r.replies[0x71], r.replies[0x81] = r.replies[0x81], swapped
	got, err := GrowattReader{Session: r, Unit: 1, Revision: exactRevision, Receipt: func() Receipt { return f.receipt(1) }}.Acquire(context.Background())
	if err != nil || got.Diagnostic == nil {
		t.Fatalf("%v %+v", err, got)
	}
	want := f.DiagnosticReplies[1].Words
	if reflect.DeepEqual(got.Diagnostic.Windows[1].Words, want) {
		t.Fatal("reordered windows compared equal to the fixture")
	}
}

func TestWrongMappingsAreRefused(t *testing.T) {
	f := loadFixture(t)
	s := f.session()
	p := newPublisher(t, f.config(), s, f.packAllocator(t))
	if err := p.PublishGrowatt(context.Background(), f.observe(t, 2, 1, true)); err != nil {
		t.Fatal(err)
	}
	// A percent where the pack states a ratio, Celsius where it states kelvin, and
	// the native current sign dropped must each be caught by the pack's own types.
	soc := only(t, s.k, "storage.state.soc")
	wrongUnit := *soc.Value
	wrongUnit.Quantity = &semreg.Quantity{Number: wrongUnit.Quantity.Number, Unit: "unit.percent"}
	if err := validator.ValidateFact(soc.Key, &wrongUnit); err == nil {
		t.Error("a percent state of charge was accepted")
	}
	temperature := only(t, s.k, "storage.temperature.pack")
	wrong := *temperature.Value
	wrong.Quantity = &semreg.Quantity{Number: wrong.Quantity.Number, Unit: "unit.celsius"}
	if err := validator.ValidateFact(temperature.Key, &wrong); err == nil {
		t.Error("a Celsius temperature was accepted")
	}
	current := only(t, s.k, "storage.pack.current")
	if c, _, _ := quantityOf(t, current); c != "-1" {
		t.Errorf("the native current sign was lost: %s", c)
	}
	direction := only(t, s.k, elementDirection)
	if direction.Value.Symbol.Token == "idle" {
		t.Error("a native state published the canonical idle")
	}
	// A fact on another subject than the pack is not published for the pack elements.
	for _, c := range s.k.facts {
		if c.Key.Dimensions[0].ID == dimPack && *c.Key.Dimensions[0].Value.Text != f.Configuration.Asset {
			t.Errorf("%s names another pack: %s", c.Key.FactID, *c.Key.Dimensions[0].Value.Text)
		}
	}
}
