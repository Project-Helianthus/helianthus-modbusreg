package modbus

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	wire "github.com/Project-Helianthus/helianthus-modbus"
	reg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/roweval"
)

const fixtureUnit = 1
const fixtureBase = 0

type readVector struct {
	Name, Revision, NativeDisposition, NativeState, Value, Reason string
	Resource, Received, Clock, State                              string
	Qualified, Derived                                            bool
	Frozen                                                        *frozenRecord
	Repair                                                        *repairRecord
	Function                                                      wire.FunctionCode
	Model, Offset, Count                                          uint16
	Occurrence                                                    uint32
	Block                                                         []string
}

// frozenRecord restates the indexed native recorder output for one case. The
// record holds no producer outcome or receipt context, so nothing is asserted
// about the producer. The current catalog emits more facts than the record
// lists for an admitted case, so only the recorded count is a lower bound.
type frozenRecord struct {
	Case, Reason, Number, State string
	Model, Length               uint16
	W, SF                       *uint16
	Facts                       int
}

// repairRecord restates the separate source-backed operand of the native
// omission: the signed operands and the unknown expectation.
type repairRecord struct {
	Case, Value string
	W, SF       int16
}

type inverseVector struct {
	Name, Outcome, Received, Resource, Clock, State, Value string
	Qualified                                              bool
	Rating, Control                                        []string
	Watts, EncodedWatts                                    int64
	Word                                                   uint16
	Readback                                               *uint16
}

type vectors struct {
	Common  []string
	Read    []readVector
	Inverse []inverseVector
}

func readVectors(t *testing.T) vectors {
	t.Helper()
	b, err := os.ReadFile("testdata/sunspec_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var f vectors
	if err = json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func words(t *testing.T, hex []string) []uint16 {
	t.Helper()
	var out []uint16
	for _, s := range hex {
		n, err := strconv.ParseUint(s, 16, 16)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, uint16(n))
	}
	return out
}

func candidate(policy semreg.PolicyID, received string, raw []byte) semreg.FactCandidate {
	binding, source, epoch, gen := semreg.NativeBindingID("binding.synthetic"), semreg.SourceID("source.synthetic"), semreg.SourceEpochID("epoch.synthetic"), semreg.Uint64("2")
	ev := []semreg.EvidenceRef{{Owner: "test.synthetic", Kind: "evidence.native", Digest: semreg.Digest(fmt.Sprintf("sha256:%x", sha256.Sum256(raw))), Contract: "test.native/v1", Access: semreg.EvidenceAccessPublic, Redaction: semreg.RedactionNone}}
	return semreg.FactCandidate{CandidateID: "candidate.synthetic.1", Revision: "7", BindingID: &binding, SourceEpochID: &epoch, DriverGeneration: &gen,
		Quality:         semreg.Quality{Assertion: semreg.AssertionObserved, Qualification: semreg.QualificationQualified, Promotion: semreg.PromotionPromoted, Validity: semreg.ValidityGood, Availability: semreg.AvailabilityAvailable, Freshness: semreg.FreshnessFresh, Reasons: []semreg.DefinitionID{}},
		Times:           semreg.Times{ReceivedAt: semreg.TimePoint{UnixNanoseconds: semreg.Int64(received), ClockID: "clock.synthetic", UncertaintyNS: "0"}, ReceiptMonotonic: semreg.MonotonicPoint{ClockEpochID: "clock:synthetic", Nanoseconds: "100"}, EvaluatedAt: semreg.TimePoint{UnixNanoseconds: "300", ClockID: "clock.synthetic", UncertaintyNS: "0"}, EvaluateMonotonic: semreg.MonotonicPoint{ClockEpochID: "clock:synthetic", Nanoseconds: "200"}, SourceAt: &semreg.TimePoint{UnixNanoseconds: "90", ClockID: "clock.synthetic", UncertaintyNS: "5"}},
		FreshnessPolicy: semreg.FreshnessPolicy{PolicyID: policy, Version: "1.0.0", FreshForNS: "1000", RetainForNS: "2000", MaxWallUncertaintyNS: "0"},
		Origin:          semreg.OriginRef{OriginID: "origin.synthetic", Kind: semreg.OriginNativeObservation, SourceID: &source, SourceEpochID: &epoch, BindingID: &binding, Evidence: ev}, Evidence: ev}
}

// The inferred operand exercises non-nil derivation preservation separately
// from the observed monitoring comparisons.
func setDerivedTemplate(tpl *semreg.FactCandidate) {
	path := semreg.SourcePathRef{BindingID: *tpl.BindingID, SourceID: *tpl.Origin.SourceID, SourceEpochID: *tpl.SourceEpochID, DriverGeneration: *tpl.DriverGeneration}
	tpl.BindingID, tpl.SourceEpochID, tpl.DriverGeneration = nil, nil, nil
	tpl.Quality.Assertion = semreg.AssertionInferred
	tpl.Origin = semreg.OriginRef{OriginID: "origin.synthetic.derived", Kind: semreg.OriginDerived, Evidence: tpl.Evidence}
	tpl.Derivation = &semreg.Derivation{Algorithm: "derivation.synthetic", Version: "2.0.0", Evidence: tpl.Evidence, Inputs: []semreg.DerivationInput{{CandidateID: "candidate.synthetic.input", CandidateRevision: "4", SourcePaths: []semreg.SourcePathRef{path}}}}
}

func expectRef(pack semreg.DefinitionID, id semreg.DefinitionID) semreg.DefinitionRef {
	return semreg.DefinitionRef{Pack: semreg.PackRef{ID: pack, Version: "1.0.0"}, ID: id, Version: "1.0.0"}
}

func roles(want roweval.Subject, facts, operations []semreg.DefinitionRef) roweval.Roles {
	return roweval.Roles{Subjects: map[string]roweval.RoleBinding{want.Role: {ServiceInstance: want.ServiceInstance, CapabilityInstance: want.CapabilityInstance}}, Catalog: []semreg.DefinitionIndex{{Pack: want.Service.Pack, Services: []semreg.DefinitionRef{want.Service}, Capabilities: []semreg.DefinitionRef{want.Capability}, Fields: facts, Operations: operations}}}
}

func subject(role string, pack, service, capability semreg.DefinitionID, resource string) roweval.Subject {
	instance := semreg.CapabilityInstanceID("capability." + resource)
	return roweval.Subject{Role: role, Service: expectRef(pack, service), Capability: expectRef(pack, capability), ServiceInstance: semreg.ServiceInstanceID(resource), CapabilityInstance: &instance}
}

func assertCandidate(t *testing.T, got roweval.Evaluation, tpl semreg.FactCandidate, sub roweval.Subject, fact semreg.DefinitionRef, dim semreg.DefinitionID, resource string, value semreg.Value) {
	t.Helper()
	key := semreg.FactKey{PackID: fact.Pack.ID, PackVersion: fact.Pack.Version, FactID: fact.ID, Dimensions: []semreg.Dimension{{ID: dim, Value: semreg.Value{Kind: semreg.ValueText, Text: &resource}}}}
	if got.Candidate == nil || !reflect.DeepEqual(got.Subject, &sub) || !reflect.DeepEqual(got.Candidate.Key, key) {
		t.Fatalf("candidate key/subject = %+v / %+v; need %+v / %+v", got.Candidate, got.Subject, key, sub)
	}
	if value.Quantity != nil {
		assertNumber(t, got.Candidate.Value, value.Quantity.Unit, rational(value.Quantity.Number))
		value = *got.Candidate.Value
	}
	tpl.Key, tpl.Value = key, &value
	if !reflect.DeepEqual(got.Candidate, &tpl) {
		t.Fatalf("candidate metadata/value changed\ngot  %+v\nwant %+v", got.Candidate, &tpl)
	}
}

func assertNumber(t *testing.T, value *semreg.Value, unit semreg.DefinitionID, expected *big.Rat) {
	t.Helper()
	if value == nil || value.Quantity == nil || value.Quantity.Unit != unit {
		t.Fatalf("value=%+v, want %s %s", value, expected, unit)
	}
	actual := rational(value.Quantity.Number)
	if actual == nil || actual.Cmp(expected) != 0 {
		t.Fatalf("value=%+v, want %s %s", value, expected, unit)
	}
}

func registry(t *testing.T) sunspec.DecoderRegistry {
	t.Helper()
	r, err := sunspec.NewStandardSunSpecDecoderRegistry(sunspec.ModelsRevisionV1)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSunSpecSelectorAndScale(t *testing.T) {
	f := readVectors(t)
	r := registry(t)
	rows, err := makeSunSpecRows(r, inverterKey, 2)
	if err != nil {
		t.Fatal(err)
	}
	if rows.table.Bindings[0].Write != nil {
		t.Fatal("monitoring power has a write")
	}
	for _, vec := range f.Read {
		t.Run(vec.Name, func(t *testing.T) {
			if vec.Name == "inconsistent-length" {
				checkInconsistentLength(t, r, rows, words(t, f.Common), vec)
				return
			}
			checkReadVector(t, r, rows, f.Common, vec)
		})
	}
}

func checkReadVector(t *testing.T, r sunspec.DecoderRegistry, rows sunSpecRows, common []string, vec readVector) {
	t.Helper()
	block := words(t, vec.Block)
	snap, _ := acquire(t, r, vec.Revision, words(t, common), block)
	occ := snap.Occurrences()[1]
	if vec.Frozen != nil {
		checkFrozen(t, r, snap, vec, block)
	}
	checkReadResult(t, r, rows, snap, occ, vec, block)
}

func checkReadResult(t *testing.T, r sunspec.DecoderRegistry, rows sunSpecRows, snap sunspec.ChainSnapshot, occ sunspec.Occurrence, vec readVector, block []uint16) {
	t.Helper()
	if !slices.Equal(occ.Words(), block) {
		t.Fatalf("acquired words %04x want %04x", occ.Words(), block)
	}
	sel := SunSpecSelector{Function: vec.Function, ModelID: vec.Model, Occurrence: vec.Occurrence, WordOffset: vec.Offset, WordCount: vec.Count}
	if vec.Revision == "v2" {
		k, ok := occ.DecoderKey()
		if !ok || k != (sunspec.DecoderKey{ModelID: 103, ModelLength: 50, SchemaRevision: sunspec.ModelsRevisionV2}) || occ.Disposition != sunspec.ChainDispositionAdmitted || occ.Ordinal != 2 || sel != (SunSpecSelector{3, 103, 2, 14, 1}) {
			t.Fatalf("revision case lacks admitted matching geometry: %+v %+v", occ, sel)
		}
		for _, view := range snap.SourceViews() {
			if view.Record().RequestedFunction != 3 {
				t.Fatal("wrong acquired function")
			}
		}
	}
	if vec.NativeDisposition != "" {
		checkNative(t, r, snap, occ, vec, block)
	}
	sub := subject("inverter", "helianthus.pack.pv", "pv.service.inverter", "pv.capability.read.inverter", vec.Resource)
	fact := expectRef("helianthus.pack.pv", "pv.ac.aggregate_active_power")
	a := roweval.Acquired[SunSpecSelector]{Selector: sel, Template: candidate("pv.telemetry.fast.v1", vec.Received, wordBytes(block)), Roles: roles(sub, []semreg.DefinitionRef{fact}, nil), Coordinates: map[string]string{"inverter-resource": vec.Resource}}
	a.Template.Times.ReceivedAt.ClockID = semreg.DefinitionID(vec.Clock)
	if vec.Derived {
		setDerivedTemplate(&a.Template)
	}
	a.Receipt = roweval.Receipt{ReceivedAt: a.Template.Times.ReceivedAt, Monotonic: a.Template.Times.ReceiptMonotonic}
	a.Qualifiers = roweval.Qualifiers{Values: map[string]string{"state": vec.State}, Features: map[string]bool{"qualified": vec.Qualified}}
	if err := roweval.ValidateTable(rows.table, descriptors[SunSpecSelector](), a.Roles); err != nil {
		t.Fatal(err)
	}
	got, err := rows.evaluate(r, occ, a)
	if vec.Revision == "v2" {
		if err == nil || err.Error() != "SunSpec occurrence lacks an exact admitted decoder key" || got.Candidate != nil || got.Subject != nil || got.Native != nil || !slices.Equal(occ.Words(), block) {
			t.Fatalf("wrong revision: result=%+v error=%v", got, err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if vec.Reason != "" {
		if vec.Reason == "no_match" && got.Native != nil {
			t.Fatalf("selector miss decoded a point: %+v", got)
		}
		if vec.Reason != "no_match" && (got.Native == nil || got.Native.Raw != uint64(block[14])) {
			t.Fatalf("unknown value lost its native word: %+v", got)
		}
		if string(got.Reason) != vec.Reason || got.Candidate != nil || got.Subject != nil || got.Native != nil && got.Native.Value != nil {
			t.Fatalf("no value expected: %+v", got)
		}
		if vec.Repair != nil {
			checkRepair(t, got, vec, block)
		}
		return
	}
	want, ok := new(big.Rat).SetString(vec.Value)
	if !ok {
		t.Fatalf("invalid vector value %q", vec.Value)
	}
	if got.Candidate == nil {
		t.Fatalf("missing candidate: %+v", got)
	}
	assertNumber(t, got.Candidate.Value, "unit.watt", want)
	assertCandidate(t, got, a.Template, sub, fact, "pv.dimension.inverter", vec.Resource, quantity(got.Candidate.Value.Quantity.Number, "unit.watt"))
	if got.Native == nil || got.Native.Raw != uint64(block[14]) {
		t.Fatalf("unscaled evidence not retained: %+v", got.Native)
	}
	assertNumber(t, got.Native.Value, "W", want)
}

func checkFrozen(t *testing.T, r sunspec.DecoderRegistry, snap sunspec.ChainSnapshot, vec readVector, block []uint16) {
	t.Helper()
	f := vec.Frozen
	if block[0] != f.Model || f.Length != 0 && block[1] != f.Length || f.W != nil && block[14] != *f.W || f.SF != nil && block[15] != *f.SF {
		t.Fatalf("block %04x does not replay the indexed operands of %s", block[:2], f.Case)
	}
	d := r.EvaluateThreePhaseMonitoring(snap)
	if string(d.Reason()) != f.Reason || d.Reason() != sunspec.CapabilityReason(vec.NativeDisposition) || len(d.Facts()) < f.Facts || f.Facts == 0 && len(d.Facts()) != 0 || d.Admitted() != (f.Reason == "ADMITTED") {
		t.Fatalf("native disposition %v with %d facts; indexed %s has %s with at least %d", d.Reason(), len(d.Facts()), f.Case, f.Reason, f.Facts)
	}
	if f.Number == "" {
		return
	}
	want, ok := new(big.Rat).SetString(f.Number)
	if !ok || f.Number != vec.Value {
		t.Fatalf("indexed number %q differs from the binding expectation %q", f.Number, vec.Value)
	}
	seen := 0
	for _, fact := range d.Facts() {
		switch fact.FieldID() {
		case "inverter.ac.power.active":
			n, has := fact.Value().Number()
			got, parsed := new(big.Rat).SetString(n)
			if !has || !parsed || got.Cmp(want) != 0 {
				t.Fatalf("recorded W %q, indexed %s", n, f.Number)
			}
			seen++
		case "inverter.operating_state":
			if _, state, ok := fact.Value().Enum(); !ok || state != f.State {
				t.Fatalf("recorded state %s, indexed %s", state, f.State)
			}
			seen++
		}
	}
	if seen != 2 {
		t.Fatalf("recorded W and state facts missing from %d facts", len(d.Facts()))
	}
}

func checkRepair(t *testing.T, got roweval.Evaluation, vec readVector, block []uint16) {
	t.Helper()
	rp := vec.Repair
	if int16(block[14]) != rp.W || int16(block[15]) != rp.SF || rp.Value != "unknown" {
		t.Fatalf("block %04x does not replay the signed repair operands of %s", block[14:16], rp.Case)
	}
	if got.Candidate != nil || got.Native == nil || got.Native.Value != nil || got.Native.Raw != uint64(block[14]) {
		t.Fatalf("repair %s must be unknown with its raw word retained: %+v", rp.Case, got)
	}
}

func checkNative(t *testing.T, r sunspec.DecoderRegistry, snap sunspec.ChainSnapshot, occ sunspec.Occurrence, vec readVector, block []uint16) {
	t.Helper()
	disposition := r.EvaluateThreePhaseMonitoring(snap)
	if string(disposition.Reason()) != vec.NativeDisposition || disposition.Admitted() != (vec.NativeDisposition == "ADMITTED") {
		t.Fatalf("native disposition %v; expected %s", disposition.Reason(), vec.NativeDisposition)
	}
	if vec.Name == "wrong-length" && (occ.Disposition != sunspec.ChainDispositionUnsupportedLength || occ.WireKey != (sunspec.WireKey{ModelID: 103, ModelLength: 30})) {
		t.Fatalf("wrong-length occurrence lost native evidence: %+v", occ)
	}
	if vec.NativeState == "" {
		return
	}
	model, err := r.DecodeOccurrence(occ)
	if err != nil {
		t.Fatal(err)
	}
	w, ok := model.Fact("inverter.ac.power.active")
	sf, scaleOK := model.Fact("inverter.scale.active_power")
	if !ok || !scaleOK || w.Unit != "W" || string(w.Value.State()) != vec.NativeState || !slices.Equal(w.Value.RawWords(), block[14:15]) || !slices.Equal(sf.Value.RawWords(), block[15:16]) {
		t.Fatalf("native W/SF = %+v / %+v", w, sf)
	}
	d, has := w.Value.Decimal()
	if vec.Value == "" {
		if has {
			t.Fatalf("unexpected native value %+v", d)
		}
		return
	}
	want, ok := new(big.Rat).SetString(vec.Value)
	if !ok {
		t.Fatalf("invalid vector value %q", vec.Value)
	}
	for _, fact := range disposition.Facts() {
		if fact.FieldID() == "inverter.operating_state" {
			n, state, ok := fact.Value().Enum()
			if !ok || n != 4 || state != vec.State {
				t.Fatalf("native operating state %d %s", n, state)
			}
		}
	}
	if !has || rational(decimal(d.Coefficient, int32(d.Exponent))).Cmp(want) != 0 {
		t.Fatalf("native value %+v", d)
	}
}

func acquire(t *testing.T, r sunspec.DecoderRegistry, revision string, blocks ...[]uint16) (sunspec.ChainSnapshot, *simulated) {
	t.Helper()
	cells := chainWords(blocks)
	plan := testChainPlan(t, r, revision, len(cells), len(blocks))
	s := openSimulation(t, cells)
	snap, err := walkChain(plan, func(request sunspec.ReadRequest) reg.LogicalViewSnapshot {
		view, raw := s.read(t, request.Address(), request.WordCount())
		return replaySnapshot(t, view, raw)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(snap.RawWords(), cells) || len(snap.Occurrences()) != len(blocks) {
		t.Fatalf("acquired chain evidence incomplete: %04x", snap.RawWords())
	}
	return snap, s
}

func chainWords(blocks [][]uint16) []uint16 {
	cells := []uint16{0x5375, 0x6e53}
	for _, block := range blocks {
		cells = append(cells, block...)
	}
	return append(cells, 0xffff, 0)
}

func testChainPlan(t *testing.T, r sunspec.DecoderRegistry, revision string, total, occurrences int) sunspec.ChainPlan {
	t.Helper()
	keys, rev := r.DecoderKeys(), sunspec.ModelsRevisionV1
	if revision == "v2" {
		rev = sunspec.ModelsRevisionV2
		keys = []sunspec.DecoderKey{{ModelID: 1, ModelLength: 65, SchemaRevision: rev}, {ModelID: 103, ModelLength: 50, SchemaRevision: rev}}
	}
	plan, err := sunspec.NewChainPlan(sunspec.ChainPlanSpec{BaseCandidates: []uint16{fixtureBase}, SchemaRevision: rev, DecoderKeys: keys, Limits: sunspec.ChainLimits{MaxTotalWords: uint32(total), MaxOccurrences: uint32(occurrences)}})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func walkChain(plan sunspec.ChainPlan, read func(sunspec.ReadRequest) reg.LogicalViewSnapshot) (sunspec.ChainSnapshot, error) {
	chain := sunspec.NewChain(plan)
	var snap sunspec.ChainSnapshot
	for next := chain.NextRequests(); len(next) > 0; next = chain.NextRequests() {
		for _, request := range next {
			var err error
			snap, err = chain.AdmitReplay(request, read(request))
			if err != nil {
				return snap, err
			}
		}
	}
	return snap, nil
}

func checkInconsistentLength(t *testing.T, r sunspec.DecoderRegistry, rows sunSpecRows, common []uint16, vec readVector) {
	t.Helper()
	block := words(t, vec.Block)
	cells := append(chainWords([][]uint16{common, block}), 0, 0)
	s := openSimulation(t, cells)
	view, raw := s.read(t, uint16(2+len(common)), uint16(len(block)))
	if !slices.Equal(view.Words(), block) || len(raw) == 0 {
		t.Fatal("malformed block evidence was lost")
	}
	snap, err := walkChain(testChainPlan(t, r, "v1", len(cells), 2), func(q sunspec.ReadRequest) reg.LogicalViewSnapshot {
		v, b := s.read(t, q.Address(), q.WordCount())
		return replaySnapshot(t, v, b)
	})
	if err == nil || err.Error() != "SunSpec non-end model has zero length" {
		t.Fatalf("chain error: %v", err)
	}
	checkNative(t, r, snap, sunspec.Occurrence{}, vec, block)
	checkFrozen(t, r, snap, vec, block)
	got, err := rows.evaluate(r, sunspec.Occurrence{}, roweval.Acquired[SunSpecSelector]{Selector: SunSpecSelector{3, 103, 2, 14, 1}})
	if err != nil || got.Reason != roweval.ReasonNoMatch || got.Native != nil || got.Candidate != nil || got.Subject != nil {
		t.Fatalf("invalid chain reached a row: %+v (%v)", got, err)
	}
}

func TestSunSpecInverseNegatives(t *testing.T) {
	f := readVectors(t)
	r := registry(t)
	for _, v := range f.Inverse {
		t.Run(v.Name, func(t *testing.T) { checkInverse(t, r, f.Common, v) })
	}
}

func checkInverse(t *testing.T, r sunspec.DecoderRegistry, common []string, v inverseVector) {
	t.Helper()
	blocks := [][]uint16{words(t, common)}
	if len(v.Rating) > 0 {
		blocks = append(blocks, words(t, v.Rating))
	}
	blocks = append(blocks, words(t, v.Control))
	snap, s := acquire(t, r, "v1", blocks...)
	occurrences := snap.Occurrences()
	control := occurrences[len(occurrences)-1]
	rows, err := makeSunSpecRows(r, controlKey, control.Ordinal)
	if err != nil {
		t.Fatal(err)
	}
	point, scale, ok := r.FixedPointLayout(controlKey, limitField)
	if !ok {
		t.Fatal("missing limit layout")
	}
	ctrl, err := r.DecodeOccurrence(control)
	if err != nil {
		t.Fatal(err)
	}
	enabled, _ := ctrl.Fact("der.control.active_power_limit.enabled")
	if n, state, ok := enabled.Value.Enum(); !ok || n != 1 || state != v.State || !v.Qualified {
		t.Fatal("synthetic executor is not bound to an enabled target")
	}
	target := control.HeaderOffset + point.WordOffset
	s.mu.Lock()
	s.target, s.mismatch = target, v.Readback
	s.mu.Unlock()
	if v.Name == "ratio-0.5" {
		checkRatio(t, r, rows, control, v)
		return
	}
	request := inverseRequest(t, r, occurrences, v)
	before := len(s.requests())
	got, targetWatts, encoded := invokeSynthetic(t, s, rows, request, target, control.Words()[scale.WordOffset], v.Outcome != "absent_executor")
	frames := s.requests()[before:]
	if got != v.Outcome {
		t.Fatalf("outcome=%s want %s (frames=%x)", got, v.Outcome, frames)
	}
	if v.Outcome != "applied" && v.Outcome != "contradicted" {
		if len(frames) != 0 {
			t.Fatalf("unexpected outbound frames: %x", frames)
		}
		t.Log("0 additional outbound frames (all functions)")
		return
	}
	if encoded != v.Word || targetWatts.Cmp(big.NewRat(v.EncodedWatts, 1)) != 0 {
		t.Fatalf("encoded=%d target watts=%s", encoded, targetWatts)
	}
	wantWrite := []byte{fixtureUnit, 6, byte(target >> 8), byte(target), byte(v.Word >> 8), byte(v.Word)}
	wantRead := []byte{fixtureUnit, 3, byte(target >> 8), byte(target), 0, 1}
	if len(frames) != 2 || !bytes.Equal(frames[0][6:], wantWrite) || !bytes.Equal(frames[1][6:], wantRead) || bytes.Equal(frames[0][:2], frames[1][:2]) {
		t.Fatalf("outbound frames=%x, expected a write followed by a distinct correlated read", frames)
	}
	t.Logf("1 exact write + 1 correlated readback; %s (requested %d W, encoded %s W)", got, v.Watts, targetWatts)
}

func inverseRequest(t *testing.T, r sunspec.DecoderRegistry, occurrences []sunspec.Occurrence, v inverseVector) roweval.Writing[SunSpecSelector] {
	t.Helper()
	w := roweval.Writing[SunSpecSelector]{Coordinates: map[string]string{"inverter-resource": v.Resource}, Qualifiers: roweval.Qualifiers{Values: map[string]string{"state": v.State}, Features: map[string]bool{"qualified": v.Qualified}}, NativeID: limitField, SubjectRole: "inverter", Operation: expectRef("helianthus.pack.pv", "pv.operation.set_active_power_limit"), Argument: semreg.TypedField{ID: "pv.limit.active_power", Value: quantity(decimal(v.Watts, 0), "unit.watt")}, Descriptors: descriptors[SunSpecSelector]()}
	if v.Outcome == "denied_operation" {
		w.Operation.ID = "pv.operation.other"
	}
	for _, occ := range occurrences[1:] {
		w.Inputs = append(w.Inputs, roweval.Input{Payload: wordBytes(occ.Words()), Receipt: roweval.Receipt{ReceivedAt: semreg.TimePoint{UnixNanoseconds: semreg.Int64(v.Received), ClockID: semreg.DefinitionID(v.Clock), UncertaintyNS: "0"}, Monotonic: semreg.MonotonicPoint{ClockEpochID: "clock:synthetic", Nanoseconds: "100"}}})
		if occ.WireKey.ModelID != 121 {
			continue
		}
		model, err := r.DecodeOccurrence(occ)
		if err != nil {
			t.Fatal(err)
		}
		rating, _ := model.Fact("der.settings.max_active_power")
		if d, ok := rating.Value.Decimal(); ok && d.Coefficient > 0 {
			w.Limits = &driver.ValueLimits{Min: quantity(decimal(0, 0), "unit.watt").Quantity, Max: quantity(decimal(d.Coefficient, int32(d.Exponent)), "unit.watt").Quantity}
		}
	}
	return w
}

func invokeSynthetic(t *testing.T, s *simulated, rows sunSpecRows, w roweval.Writing[SunSpecSelector], target, sf uint16, haveExecutor bool) (string, *big.Rat, uint16) {
	t.Helper()
	prepared, err := roweval.Invert(rows.table, w)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Reason != "" {
		if prepared.Payload != nil {
			t.Fatalf("denial produced payload: %+v", prepared)
		}
		if prepared.Reason == roweval.ReasonOutsideDomain && prepared.Detail != "published limit: above the maximum" {
			t.Fatalf("watt bound was not checked before encoding: %+v", prepared)
		}
		if prepared.Reason == roweval.ReasonNoMatch {
			return "denied_operation", nil, 0
		}
		return string(prepared.Reason), nil, 0
	}
	if !haveExecutor {
		return "absent_executor", nil, 0
	}
	if len(prepared.Payload) != 2 || prepared.Target != limitField || prepared.Readback != limitField {
		t.Fatalf("inverse=%+v", prepared)
	}
	raw := binary.BigEndian.Uint16(prepared.Payload)
	admit := func(r wire.RegisterWriteAdmissionRequest) bool {
		return r.Endpoint == s.name && r.Generation == s.connection.Generation() && r.UnitID == fixtureUnit && r.Function == 6 && r.Address == target && r.Value == raw && r.Quantity == 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ack, err := s.endpoint.WriteSingleRegister(ctx, s.connection, fixtureUnit, target, raw, admit)
	if err != nil || ack.Address != target || ack.Value != raw {
		t.Fatalf("write acknowledgement %+v, %v", ack, err)
	}
	view, _ := s.read(t, target, 1)
	readback := view.Words()
	max := rational(w.Limits.Max.Number)
	targetWatts := encodedWatts(raw, sf, max)
	actualWatts := encodedWatts(readback[0], sf, max)
	if readback[0] != raw || actualWatts.Cmp(targetWatts) != 0 {
		return "contradicted", targetWatts, raw
	}
	return "applied", targetWatts, raw
}

func encodedWatts(raw, sf uint16, max *big.Rat) *big.Rat {
	return new(big.Rat).Mul(new(big.Rat).Mul(big.NewRat(int64(raw), 100), power10(int64(int16(sf)))), max)
}

func checkRatio(t *testing.T, r sunspec.DecoderRegistry, rows sunSpecRows, occ sunspec.Occurrence, v inverseVector) {
	t.Helper()
	sub := subject("inverter", "helianthus.pack.pv", "pv.service.inverter", "pv.capability.set_active_power_limit", v.Resource)
	fact := expectRef("helianthus.pack.pv", "pv.limit.active_power_ratio")
	op := expectRef("helianthus.pack.pv", "pv.operation.set_active_power_limit")
	tpl := candidate("pv.telemetry.fast.v1", v.Received, wordBytes(occ.Words()))
	tpl.Times.ReceivedAt.ClockID = semreg.DefinitionID(v.Clock)
	got, err := rows.evaluate(r, occ, roweval.Acquired[SunSpecSelector]{Selector: SunSpecSelector{3, 123, occ.Ordinal, 5, 1}, Template: tpl, Receipt: roweval.Receipt{ReceivedAt: tpl.Times.ReceivedAt, Monotonic: tpl.Times.ReceiptMonotonic}, Qualifiers: roweval.Qualifiers{Values: map[string]string{"state": v.State}, Features: map[string]bool{"qualified": v.Qualified}}, Roles: roles(sub, []semreg.DefinitionRef{fact}, []semreg.DefinitionRef{op}), Coordinates: map[string]string{"inverter-resource": v.Resource}})
	if err != nil {
		t.Fatal(err)
	}
	expected, ok := new(big.Rat).SetString(v.Value)
	if !ok {
		t.Fatal("invalid ratio operand")
	}
	if got.Candidate == nil {
		t.Fatalf("missing ratio candidate: %+v", got)
	}
	assertNumber(t, got.Candidate.Value, "unit.ratio", expected)
	assertCandidate(t, got, tpl, sub, fact, "pv.dimension.inverter", v.Resource, quantity(decimal(5, -1), "unit.ratio"))
	assertNumber(t, got.Native.Value, "% WMax", big.NewRat(50, 1))
}
