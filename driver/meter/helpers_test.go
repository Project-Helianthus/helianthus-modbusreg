package meter

import (
	"crypto/sha256"
	"fmt"
	"math"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/runtime/identity"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/graph"
)

const testPolicy semreg.PolicyID = "infrastructure.telemetry.v1"

func testConfig() Config {
	return Config{
		Installation: "installation.synthetic", Driver: "modbus", Instance: "instance.synthetic", Bridge: "bridge.synthetic",
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

func commonBlock() []uint16 {
	b := make([]uint16, 67)
	b[0], b[1] = 1, 65
	copy(b[2:], stringWords("Maker", 16))
	copy(b[18:], stringWords("Model", 16))
	copy(b[42:], stringWords("1.0", 8))
	copy(b[50:], stringWords("TEST", 16))
	return b
}

func floatWords(v float32) []uint16 {
	b := math.Float32bits(v)
	return []uint16{uint16(b >> 16), uint16(b)}
}

func accumulator(v uint32) []uint16 { return []uint16{uint16(v >> 16), uint16(v)} }

func int16Word(v int16) []uint16 { return []uint16{uint16(v)} }

var meterFieldIDs = func() []string {
	ids := []string{"meter.ac.current.total", "meter.scale.current", "meter.ac.voltage.line_neutral.average", "meter.ac.voltage.line_line.average", "meter.scale.voltage",
		"meter.ac.frequency", "meter.scale.frequency", "meter.ac.power.active.total", "meter.scale.active_power", "meter.ac.power.apparent.total", "meter.scale.apparent_power",
		"meter.ac.power.reactive.total", "meter.scale.reactive_power", "meter.ac.power_factor.total", "meter.scale.power_factor",
		"meter.energy.export.total", "meter.energy.import.total", "meter.scale.active_energy", "meter.energy.apparent_export.total", "meter.energy.apparent_import.total", "meter.scale.apparent_energy",
		"meter.energy.reactive_import.q1.total", "meter.energy.reactive_import.q2.total", "meter.energy.reactive_export.q3.total", "meter.energy.reactive_export.q4.total", "meter.scale.reactive_energy", "meter.events"}
	for _, p := range []string{"a", "b", "c"} {
		for _, f := range []string{"current", "voltage.line_neutral", "power.active", "power.apparent", "power.reactive", "power_factor"} {
			ids = append(ids, "meter.ac."+f+".phase_"+p)
		}
		for _, f := range []string{"export", "import", "apparent_export", "apparent_import"} {
			ids = append(ids, "meter.energy."+f+".phase_"+p)
		}
	}
	for _, p := range []string{"ab", "bc", "ca"} {
		ids = append(ids, "meter.ac.voltage.line_line."+p)
	}
	return ids
}()

// meterBlock builds a meter model block. Points that are not given are not
// implemented.
func meterBlock(t *testing.T, r sunspec.DecoderRegistry, model uint16, values map[string][]uint16) []uint16 {
	t.Helper()
	length := uint16(105)
	if model >= 211 {
		length = 124
	}
	key := sunspec.DecoderKey{ModelID: model, ModelLength: length, SchemaRevision: sunspec.ModelsRevisionV1}
	words := make([]uint16, int(length)+2)
	words[0], words[1] = model, length
	for _, id := range meterFieldIDs {
		p, _, ok := r.FixedPointLayout(key, id)
		if !ok {
			continue
		}
		switch p.Type {
		case sunspec.TypeFloat32:
			copy(words[p.WordOffset:], []uint16{0x7fc0, 0})
		case sunspec.TypeAccumulator32, sunspec.TypeBitfield32:
		case sunspec.TypeUint16:
			words[p.WordOffset] = 0xffff
		default:
			words[p.WordOffset] = 0x8000
		}
	}
	for id, v := range values {
		p, _, ok := r.FixedPointLayout(key, id)
		if !ok || int(p.WordWidth) != len(v) {
			t.Fatalf("point %s: layout %v ok=%v words=%d", id, p, ok, len(v))
		}
		copy(words[p.WordOffset:], v)
	}
	return words
}

func registry(t *testing.T) sunspec.DecoderRegistry {
	t.Helper()
	r, err := sunspec.NewStandardSunSpecDecoderRegistry(sunspec.ModelsRevisionV1)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// chain walks a synthetic chain image of the given blocks on one unit.
func chain(t *testing.T, r sunspec.DecoderRegistry, unit uint8, blocks ...[]uint16) sunspec.ChainSnapshot {
	t.Helper()
	const base = uint16(40000)
	raw := []uint16{0x5375, 0x6e53}
	for _, b := range blocks {
		raw = append(raw, b...)
	}
	raw = append(raw, 0xffff, 0)
	plan, err := sunspec.NewChainPlan(sunspec.ChainPlanSpec{SchemaRevision: sunspec.ModelsRevisionV1, BaseCandidates: []uint16{base}, DecoderKeys: r.DecoderKeys(),
		Limits: sunspec.ChainLimits{MaxTotalWords: uint32(len(raw)), MaxOccurrences: uint32(len(blocks))}})
	if err != nil {
		t.Fatal(err)
	}
	c := sunspec.NewChain(plan)
	id := uint64(1000)
	for step := 0; step < len(raw); step++ {
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

func (p *Publication) fact(t *testing.T, node driver.NodeRef, element semreg.DefinitionID, dim string) Fact {
	t.Helper()
	for _, f := range p.Facts {
		if f.Node == node && f.Candidate.Key.FactID == element && *f.Candidate.Key.Dimensions[0].Value.Text == dim {
			return f
		}
	}
	t.Fatalf("no fact %s on %s/%s", element, node.ID, dim)
	return Fact{}
}
