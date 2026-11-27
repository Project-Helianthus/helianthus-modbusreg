package pv

import (
	"crypto/sha256"
	"fmt"
	"math"
	"math/big"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime/identity"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/graph"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/packs/infrastructure"
	pvpack "github.com/Project-Helianthus/helianthus-semreg/semreg/v1/packs/pv"
)

const testPolicy semreg.PolicyID = "pv.telemetry.fast.v1"

func testConfig() Config {
	return Config{
		Installation: "installation.synthetic", Driver: "modbus", Instance: "instance.synthetic", Bridge: "bridge.synthetic",
		System: "plant-1", Endpoint: "endpoint-1", LoggerSlot: "logger-slot-1",
		Freshness: semreg.FreshnessPolicy{PolicyID: testPolicy, Version: "1.0.0", FreshForNS: "1000", RetainForNS: "2000", MaxWallUncertaintyNS: "0"},
	}
}

func testAllocator(t *testing.T) Allocator {
	t.Helper()
	catalog, err := graph.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	a, err := identity.NewAllocator(identity.NewMemoryStore(), catalog)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func testPublisher(t *testing.T, cfg Config) *Publisher {
	t.Helper()
	p, err := New(cfg, testAllocator(t))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func template() semreg.FactCandidate {
	binding, source, epoch, gen := semreg.NativeBindingID("binding.synthetic"), semreg.SourceID("source.synthetic"), semreg.SourceEpochID("epoch.synthetic"), semreg.Uint64("2")
	raw := []byte("synthetic receipt")
	ev := []semreg.EvidenceRef{{Owner: "test.synthetic", Kind: "evidence.native", Digest: semreg.Digest(fmt.Sprintf("sha256:%x", sha256.Sum256(raw))), Contract: "test.native/v1", Access: semreg.EvidenceAccessPublic, Redaction: semreg.RedactionNone}}
	return semreg.FactCandidate{CandidateID: "candidate.template", Revision: "7", BindingID: &binding, SourceEpochID: &epoch, DriverGeneration: &gen,
		Quality: semreg.Quality{Assertion: semreg.AssertionObserved, Qualification: semreg.QualificationQualified, Promotion: semreg.PromotionPromoted, Validity: semreg.ValidityGood, Availability: semreg.AvailabilityAvailable, Freshness: semreg.FreshnessFresh, Reasons: []semreg.DefinitionID{}},
		Times: semreg.Times{ReceivedAt: semreg.TimePoint{UnixNanoseconds: "100", ClockID: "clock.synthetic", UncertaintyNS: "0"}, ReceiptMonotonic: semreg.MonotonicPoint{ClockEpochID: "clock:synthetic", Nanoseconds: "100"},
			EvaluatedAt: semreg.TimePoint{UnixNanoseconds: "300", ClockID: "clock.synthetic", UncertaintyNS: "0"}, EvaluateMonotonic: semreg.MonotonicPoint{ClockEpochID: "clock:synthetic", Nanoseconds: "200"}},
		FreshnessPolicy: semreg.FreshnessPolicy{PolicyID: testPolicy, Version: "1.0.0", FreshForNS: "1000", RetainForNS: "2000", MaxWallUncertaintyNS: "0"},
		Origin:          semreg.OriginRef{OriginID: "origin.synthetic", Kind: semreg.OriginNativeObservation, SourceID: &source, SourceEpochID: &epoch, BindingID: &binding, Evidence: ev}, Evidence: ev}
}

func stringWords(text string, words int) []uint16 {
	out := make([]uint16, words)
	for i := 0; i < len(text) && i/2 < words; i++ {
		out[i/2] |= uint16(text[i]) << (8 * (1 - uint(i%2)))
	}
	return out
}

// commonBlock is a V1 Common model. An empty manufacturer or serial leaves that
// string absent.
func commonBlock(manufacturer, model, serial string) []uint16 {
	b := make([]uint16, 67)
	b[0], b[1] = 1, 65
	copy(b[2:], stringWords(manufacturer, 16))
	copy(b[18:], stringWords(model, 16))
	copy(b[42:], stringWords("1.0", 8))
	copy(b[50:], stringWords(serial, 16))
	return b
}

// derCommonBlock is a Common model of a chain that carries V2 models: its length
// is 66.
func derCommonBlock(manufacturer, model, serial string) []uint16 {
	b := make([]uint16, 68)
	b[0], b[1] = 1, 66
	copy(b[2:], stringWords(manufacturer, 16))
	copy(b[18:], stringWords(model, 16))
	copy(b[42:], stringWords("1.0", 8))
	copy(b[50:], stringWords(serial, 16))
	b[66] = 0x8000
	return b
}

func floatWords(v float32) []uint16 {
	b := math.Float32bits(v)
	return []uint16{uint16(b >> 16), uint16(b)}
}

func accumulator(v uint32) []uint16 { return []uint16{uint16(v >> 16), uint16(v)} }

func word(v int16) []uint16 { return []uint16{uint16(v)} }

func registry(t *testing.T) sunspec.DecoderRegistry {
	t.Helper()
	r, err := sunspec.NewStandardSunSpecDecoderRegistry(sunspec.ModelsRevisionV1)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

var inverterScaleIDs = []string{"inverter.scale.current", "inverter.scale.voltage", "inverter.scale.active_power", "inverter.scale.frequency", "inverter.scale.apparent_power",
	"inverter.scale.reactive_power", "inverter.scale.power_factor", "inverter.scale.energy", "inverter.scale.dc_current", "inverter.scale.dc_voltage", "inverter.scale.dc_power", "inverter.scale.temperature"}

var inverterFieldIDs = []string{"inverter.ac.current.total", "inverter.ac.current.phase_a", "inverter.ac.current.phase_b", "inverter.ac.current.phase_c",
	"inverter.ac.voltage.line_ab", "inverter.ac.voltage.line_bc", "inverter.ac.voltage.line_ca", "inverter.ac.voltage.phase_a", "inverter.ac.voltage.phase_b", "inverter.ac.voltage.phase_c",
	"inverter.ac.power.active", "inverter.ac.frequency", "inverter.ac.power.apparent", "inverter.ac.power.reactive", "inverter.ac.power_factor", "inverter.ac.energy_lifetime",
	"inverter.dc.current", "inverter.dc.voltage", "inverter.dc.power", "inverter.temperature.cabinet", "inverter.temperature.heatsink", "inverter.temperature.transformer", "inverter.temperature.other",
	"inverter.operating_state", "inverter.vendor_state", "inverter.events.1", "inverter.events.2", "inverter.vendor_events.1", "inverter.vendor_events.2", "inverter.vendor_events.3", "inverter.vendor_events.4"}

// inverterBlock builds an inverter model block of the V1 catalog. A point that
// is not given is not implemented; the scale factors default to zero.
func inverterBlock(t *testing.T, model uint16, values map[string][]uint16) []uint16 {
	t.Helper()
	r := registry(t)
	length := uint16(50)
	if model >= 111 {
		length = 60
	}
	key := sunspec.DecoderKey{ModelID: model, ModelLength: length, SchemaRevision: sunspec.ModelsRevisionV1}
	words := make([]uint16, int(length)+2)
	words[0], words[1] = model, length
	for _, id := range inverterFieldIDs {
		p, _, ok := r.FixedPointLayout(key, id)
		if !ok {
			continue
		}
		switch p.Type {
		case sunspec.TypeFloat32:
			copy(words[p.WordOffset:], []uint16{0x7fc0, 0})
		case sunspec.TypeAccumulator32, sunspec.TypeBitfield32:
		case sunspec.TypeUint16, sunspec.TypeEnum16:
			words[p.WordOffset] = 0xffff
		default:
			words[p.WordOffset] = 0x8000
		}
	}
	for _, id := range inverterScaleIDs {
		if p, _, ok := r.FixedPointLayout(key, id); ok {
			words[p.WordOffset] = 0
		}
	}
	for id, v := range values {
		p, _, ok := r.FixedPointLayout(key, id)
		if !ok || int(p.WordWidth) != len(v) {
			t.Fatalf("model %d point %s: layout %v ok=%v words=%d", model, id, p, ok, len(v))
		}
		copy(words[p.WordOffset:], v)
	}
	return words
}

func merge(a, b map[string][]uint16) map[string][]uint16 {
	out := map[string][]uint16{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// integerValues are the required points of an integer inverter model with the
// scale factors that make 100 a value of 10.0.
func integerValues(model uint16, extra map[string][]uint16) map[string][]uint16 {
	v := map[string][]uint16{
		"inverter.scale.current": word(-1), "inverter.scale.voltage": word(-1), "inverter.scale.frequency": word(-2), "inverter.scale.temperature": word(-1),
		"inverter.ac.current.total": word(100), "inverter.ac.current.phase_a": word(30), "inverter.ac.voltage.phase_a": word(2300),
		"inverter.ac.power.active": word(2500), "inverter.ac.frequency": word(5001), "inverter.ac.energy_lifetime": accumulator(123456),
		"inverter.temperature.cabinet": word(350), "inverter.operating_state": {4}, "inverter.events.1": accumulator(0), "inverter.events.2": accumulator(0),
	}
	if model == 102 || model == 103 {
		v["inverter.ac.current.phase_b"], v["inverter.ac.voltage.phase_b"] = word(40), word(2310)
	}
	if model == 103 {
		v["inverter.ac.current.phase_c"], v["inverter.ac.voltage.phase_c"] = word(50), word(2320)
	}
	return merge(v, extra)
}

func floatValues(model uint16, extra map[string][]uint16) map[string][]uint16 {
	v := map[string][]uint16{
		"inverter.ac.current.total": floatWords(10.5), "inverter.ac.current.phase_a": floatWords(3.5), "inverter.ac.voltage.phase_a": floatWords(230.5),
		"inverter.ac.power.active": floatWords(2500.5), "inverter.ac.frequency": floatWords(50.25), "inverter.ac.energy_lifetime": floatWords(123456),
		"inverter.temperature.cabinet": floatWords(35.5), "inverter.operating_state": {4}, "inverter.events.1": accumulator(0), "inverter.events.2": accumulator(0),
	}
	if model == 112 || model == 113 {
		v["inverter.ac.current.phase_b"], v["inverter.ac.voltage.phase_b"] = floatWords(4.5), floatWords(231.5)
	}
	if model == 113 {
		v["inverter.ac.current.phase_c"], v["inverter.ac.voltage.phase_c"] = floatWords(5.5), floatWords(232.5)
	}
	return merge(v, extra)
}

// chain walks a synthetic chain image of the given blocks on one unit.
func chain(t *testing.T, unit uint8, blocks ...[]uint16) sunspec.ChainSnapshot {
	t.Helper()
	const base = uint16(40000)
	raw := []uint16{0x5375, 0x6e53}
	ids := make([]uint16, 0, len(blocks))
	for _, b := range blocks {
		raw = append(raw, b...)
		ids = append(ids, b[0])
	}
	raw = append(raw, 0xffff, 0)
	plan, err := sunspec.NewListedChainPlan(ids, []uint16{base}, sunspec.ChainLimits{MaxTotalWords: uint32(len(raw)), MaxOccurrences: uint32(len(blocks))})
	if err != nil {
		t.Fatal(err)
	}
	c := sunspec.NewChain(plan)
	id := uint64(1000)
	for step := 0; step < len(raw)+2; step++ {
		next := c.NextRequests()
		if len(next) != 1 {
			t.Fatalf("pending requests=%d", len(next))
		}
		q := next[0]
		start := int(q.Address()) - int(base)
		view, err := reg.NewLogicalViewSnapshot(reg.LogicalViewRecord{LogicalViewID: id, WireResponseID: id + 100, PhysicalRequestID: id + 200, Endpoint: "fixture-endpoint", ConnectionID: 4,
			Transport: reg.TransportTCP, TransportGeneration: 5, UnitID: unit, RequestedFunction: reg.FunctionReadHoldingRegisters, ReceivedFunction: reg.FunctionReadHoldingRegisters, Table: reg.HoldingRegisters,
			PhysicalOffset: q.Address(), PhysicalWordCount: q.WordCount(), AuthorizationScope: "fixture-read-only", PollGeneration: 6, DeadlineIdentity: 7,
			LogicalOffset: q.Address(), LogicalWordCount: q.WordCount(), SliceWordCount: q.WordCount(), Words: raw[start : start+int(q.WordCount())], WireResponseBytes: []byte{3, byte(q.WordCount() * 2)}})
		if err != nil {
			t.Fatal(err)
		}
		snap, err := c.AdmitReplay(q, view)
		if err != nil {
			t.Fatal(err)
		}
		id++
		if len(snap.RawWords()) != 0 {
			return snap
		}
	}
	t.Fatal("chain did not terminate")
	return sunspec.ChainSnapshot{}
}

func rat(t *testing.T, text string) *big.Rat {
	t.Helper()
	r, ok := new(big.Rat).SetString(text)
	if !ok {
		t.Fatalf("bad number %q", text)
	}
	return r
}

// find returns the fact of an element on a dimension text.
func (p Publication) find(element semreg.DefinitionID, dim string) (Fact, bool) {
	for _, f := range p.Facts {
		if f.Candidate.Key.FactID == element && *f.Candidate.Key.Dimensions[0].Value.Text == dim {
			return f, true
		}
	}
	return Fact{}, false
}

func (p Publication) factsOf(element semreg.DefinitionID) []Fact {
	var out []Fact
	for _, f := range p.Facts {
		if f.Candidate.Key.FactID == element {
			out = append(out, f)
		}
	}
	return out
}

func (p Publication) node(t *testing.T, kind semreg.DefinitionID, slot string) driver.NodeRef {
	t.Helper()
	for _, n := range p.Nodes {
		if n.Key.Kind == kind && n.Key.Anchor.Slot == slot {
			return n.Ref
		}
	}
	t.Fatalf("no %s node at %s", kind, slot)
	return driver.NodeRef{}
}

func (p Publication) inverters() []Node {
	var out []Node
	for _, n := range p.Nodes {
		if n.Key.Kind == kindInverter {
			out = append(out, n)
		}
	}
	return out
}

// requireQuantity checks a fact of the pinned PV pack (or the infrastructure
// pack) by its element, dimension text, unit and exact value.
func requireQuantity(t *testing.T, p Publication, element semreg.DefinitionID, dim string, unit semreg.DefinitionID, value string) {
	t.Helper()
	f, ok := p.find(element, dim)
	if !ok {
		t.Fatalf("no %s on %s", element, dim)
	}
	q := f.Candidate.Value.Quantity
	if q == nil || q.Unit != unit {
		t.Fatalf("%s: value %+v, want unit %s", element, f.Candidate.Value, unit)
	}
	got, err := ratOf(q.Number)
	if err != nil || got.Cmp(rat(t, value)) != 0 {
		t.Fatalf("%s %s: got %s, want %s", element, dim, got.RatString(), value)
	}
	requireInPack(t, f)
}

func requireSymbol(t *testing.T, p Publication, element semreg.DefinitionID, dim, token string) {
	t.Helper()
	f, ok := p.find(element, dim)
	if !ok {
		t.Fatalf("no %s on %s", element, dim)
	}
	s := f.Candidate.Value.Symbol
	if s == nil || s.Token != token || s.Namespace != element {
		t.Fatalf("%s: value %+v, want %s", element, f.Candidate.Value, token)
	}
	requireInPack(t, f)
}

func requireInPack(t *testing.T, f Fact) {
	t.Helper()
	var err error
	if f.Candidate.Key.PackID == infraID {
		err = infrastructure.New().ValidateFact(f.Candidate.Key, f.Candidate.Value)
	} else {
		err = pvpack.New().ValidateFact(f.Candidate.Key, f.Candidate.Value)
	}
	if err != nil {
		t.Fatalf("%s is not a fact of the pinned pack: %v", f.Candidate.Key.FactID, err)
	}
}

func (p Publication) absent(t *testing.T, element semreg.DefinitionID) {
	t.Helper()
	if got := p.factsOf(element); len(got) != 0 {
		t.Fatalf("%s was published %d times, want none", element, len(got))
	}
}
