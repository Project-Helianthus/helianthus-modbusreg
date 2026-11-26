package storage

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"testing"
	"time"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	packstorage "github.com/Project-Helianthus/helianthus-semreg/semreg/v1/packs/storage"
)

// kernel is a stand-in for the semantic kernel of one asset: it applies a
// committed transaction as one unit and keeps nothing of a refused one.
type kernel struct {
	sequence  uint64
	facts     map[semreg.CandidateID]semreg.FactCandidate
	services  map[semreg.ServiceInstanceID]semreg.ServiceInstance
	resources map[semreg.ResourceID]semreg.Resource
	relations []semreg.RelationInstance
	claims    []driver.LinkClaim
	links     map[semreg.NativeBindingID]semreg.IdentityLink
	bindings  map[semreg.NativeBindingID]semreg.NativeBinding
	batches   []semreg.PublicationBatch
}

func newKernel() *kernel {
	return &kernel{facts: map[semreg.CandidateID]semreg.FactCandidate{}, services: map[semreg.ServiceInstanceID]semreg.ServiceInstance{},
		resources: map[semreg.ResourceID]semreg.Resource{}, links: map[semreg.NativeBindingID]semreg.IdentityLink{}, bindings: map[semreg.NativeBindingID]semreg.NativeBinding{}}
}

type session struct {
	asset      semreg.AssetID
	source     semreg.SourceID
	epoch      semreg.SourceEpochID
	generation uint64
	k          *kernel
	pending    []semreg.IdentityLink
	refuse     error
	opened     int
	aborted    int
}

func newSession(asset, source, epoch string, generation uint64) *session {
	return &session{asset: semreg.AssetID(asset), source: semreg.SourceID(source), epoch: semreg.SourceEpochID(epoch), generation: generation, k: newKernel()}
}

// successor is the next generation of the same session on the same kernel.
func (s *session) successor(epoch string, generation uint64) *session {
	n := *s
	n.epoch, n.generation = semreg.SourceEpochID(epoch), generation
	n.k.sequence = 0
	return &n
}

type fakeSession struct {
	driver.Session
	s *session
}

func (f fakeSession) Publisher() driver.Publisher { return f }

func (f fakeSession) Begin(context.Context) (driver.PublicationTransaction, error) {
	f.s.opened++
	return &tx{s: f.s}, nil
}

func (f fakeSession) Prepare(mutate func(*semreg.PublicationBatch)) (semreg.PublicationBatch, error) {
	s := f.s
	b := semreg.PublicationBatch{Contract: semreg.ContractKernelV1,
		BatchID: semreg.BatchID(fmt.Sprintf("batch:%s:%d:%d", s.source, s.generation, s.k.sequence+1)),
		AssetID: s.asset, SourceID: s.source, SourceEpochID: s.epoch, DriverGeneration: semreg.Uint64(strconv.FormatUint(s.generation, 10)),
		Sequence: semreg.Uint64(strconv.FormatUint(s.k.sequence+1, 10)), ExpectedSemanticRevision: semreg.Uint64(strconv.FormatUint(s.k.sequence, 10)),
		ObservedAt:    semreg.TimePoint{UnixNanoseconds: "1800000000000000000", ClockID: "wall.utc", UncertaintyNS: "0"},
		SourceUpserts: []semreg.SourceDescriptor{}, SourceRetirements: []semreg.SourceEpochID{}, BindingUpserts: []semreg.NativeBinding{},
		IdentityLinkUpserts: []semreg.IdentityLink{}, FactUpserts: []semreg.FactCandidate{}, FactWithdrawals: []semreg.CandidateID{},
		ServiceUpserts: []semreg.ServiceInstance{}, ServiceWithdrawals: []semreg.ServiceInstanceID{},
		CapabilityUpserts: []semreg.CapabilityInstance{}, CapabilityWithdrawals: []semreg.CapabilityInstanceID{}, GenerationFences: []semreg.GenerationFence{}}
	if mutate != nil {
		mutate(&b)
	}
	d, err := b.ComputedDigest()
	if err != nil {
		return semreg.PublicationBatch{}, err
	}
	b.BatchDigest = d
	return b, nil
}

type tx struct {
	driver.PublicationTransaction
	s      *session
	batch  *semreg.PublicationBatch
	claims []driver.LinkClaim
	closed bool
}

func (t *tx) OperatorLinks(upserts []semreg.NativeBinding) []semreg.IdentityLink {
	return append([]semreg.IdentityLink(nil), t.s.pending...)
}

var validator = packstorage.New()

func (t *tx) Stage(b semreg.PublicationBatch) error {
	if err := b.Validate(); err != nil {
		return err
	}
	for _, c := range b.FactUpserts {
		if err := c.Validate(); err != nil {
			return err
		}
		if err := validator.ValidateFact(c.Key, c.Value); err != nil {
			return fmt.Errorf("%s: %w", c.Key.FactID, err)
		}
	}
	for _, r := range b.ResourceUpserts {
		if err := r.Validate(); err != nil {
			return err
		}
	}
	for _, r := range b.RelationUpserts {
		if err := r.Validate(); err != nil {
			return err
		}
	}
	t.batch = &b
	return nil
}

func (t *tx) StageLink(c driver.LinkClaim) error {
	if err := c.Validate(); err != nil {
		return err
	}
	t.claims = append(t.claims, c)
	return nil
}

func (t *tx) Commit(context.Context) error {
	if t.closed || t.batch == nil {
		return errors.New("no staged batch")
	}
	t.closed = true
	k, b := t.s.k, *t.batch
	if t.s.refuse != nil {
		return t.s.refuse
	}
	kinds := map[semreg.DefinitionID]semreg.DefinitionID{dimPack: kindPack, dimInterface: kindInterface, dimCell: kindCell}
	known := map[semreg.ResourceID]semreg.DefinitionID{}
	for id, r := range k.resources {
		known[id] = r.Kind
	}
	for _, r := range b.ResourceUpserts {
		known[r.ResourceID] = r.Kind
	}
	for _, c := range b.FactUpserts {
		d := c.Key.Dimensions[0]
		if got := known[semreg.ResourceID(*d.Value.Text)]; got != kinds[d.ID] {
			return fmt.Errorf("%s: the subject %s of dimension %s is a %q, not a %s", c.Key.FactID, *d.Value.Text, d.ID, got, kinds[d.ID])
		}
	}
	for _, sv := range b.ServiceUpserts {
		if sv.Resource == nil || known[*sv.Resource] == "" {
			return fmt.Errorf("service %s names no published resource", sv.InstanceID)
		}
	}
	for _, id := range b.FactWithdrawals {
		if _, ok := k.facts[id]; !ok {
			return fmt.Errorf("withdrawal of unpublished candidate %s", id)
		}
	}
	for _, id := range b.FactWithdrawals {
		delete(k.facts, id)
	}
	for _, c := range b.FactUpserts {
		k.facts[c.CandidateID] = c
	}
	for _, sv := range b.ServiceUpserts {
		k.services[sv.InstanceID] = sv
	}
	for _, r := range b.ResourceUpserts {
		k.resources[r.ResourceID] = r
	}
	k.relations = append(k.relations, b.RelationUpserts...)
	k.claims = append(k.claims, t.claims...)
	for _, l := range b.IdentityLinkUpserts {
		k.links[l.BindingID] = l
	}
	for _, nb := range b.BindingUpserts {
		k.bindings[nb.BindingID] = nb
	}
	k.sequence++
	k.batches = append(k.batches, b)
	return nil
}

func (t *tx) Abort(context.Context) error {
	t.closed = true
	t.s.aborted++
	return nil
}

// allocator gives every node key a resource identifier from its kind, parent
// and slot, the same for any order of discovery.
type allocator struct {
	fixed map[string]semreg.ResourceID
	seen  []driver.NodeKey
}

func (a *allocator) Allocate(k driver.NodeKey) (semreg.ResourceID, error) {
	if err := k.Validate(); err != nil {
		return "", err
	}
	a.seen = append(a.seen, k)
	name := string(k.Kind) + "|" + string(k.Anchor.Parent) + "|" + k.Anchor.Slot
	if id, ok := a.fixed[name]; ok {
		return id, nil
	}
	return semreg.ResourceID("resource:" + hashHex(name)[:24]), nil
}

var testPolicy = semreg.FreshnessPolicy{PolicyID: "policy:gateway-storage-native-receipt", Version: "1.0.0", FreshForNS: "60000000000", RetainForNS: "300000000000", MaxWallUncertaintyNS: "0"}

func testConfig() Config {
	return Config{Installation: "installation.test", Driver: "modbus", Instance: "storage.one", Bridge: "bridge.test", Freshness: testPolicy, First: true}
}

func newPublisher(t *testing.T, cfg Config, s *session, a Allocator) *Publisher {
	t.Helper()
	if a == nil {
		a = &allocator{}
	}
	p, err := New(cfg, a, fakeSession{s: s})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func receipt(n int) Receipt {
	return Receipt{ObservationID: "observation-" + strconv.Itoa(n), Wall: time.Unix(1800000000, int64(n)), Mono: time.Duration(n) * time.Millisecond, ClockEpoch: "system-monotonic-v1",
		Evidence: []string{"observation-" + strconv.Itoa(n)}}
}

// factsOf lists the published facts of one element, by dimension value.
func factsOf(k *kernel, element semreg.DefinitionID) []semreg.FactCandidate {
	var out []semreg.FactCandidate
	for _, c := range k.facts {
		if c.Key.FactID == element {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CandidateID < out[j].CandidateID })
	return out
}

func only(t *testing.T, k *kernel, element semreg.DefinitionID) semreg.FactCandidate {
	t.Helper()
	got := factsOf(k, element)
	if len(got) != 1 {
		t.Fatalf("%s: %d facts, want 1", element, len(got))
	}
	return got[0]
}

func quantityOf(t *testing.T, c semreg.FactCandidate) (string, int32, semreg.DefinitionID) {
	t.Helper()
	if c.Value == nil || c.Value.Quantity == nil {
		t.Fatalf("%s: no quantity", c.Key.FactID)
	}
	q := c.Value.Quantity
	return q.Number.Coefficient, q.Number.Exponent10, q.Unit
}

func symbolOf(t *testing.T, c semreg.FactCandidate) string {
	t.Helper()
	if c.Value == nil || c.Value.Symbol == nil {
		t.Fatalf("%s: no symbol", c.Key.FactID)
	}
	return string(c.Value.Symbol.Namespace) + "." + c.Value.Symbol.Token
}

func requireQuantity(t *testing.T, k *kernel, element semreg.DefinitionID, coefficient string, exponent int32, unit semreg.DefinitionID) {
	t.Helper()
	c, e, u := quantityOf(t, only(t, k, element))
	if c != coefficient || e != exponent || u != unit {
		t.Errorf("%s = %se%d %s, want %se%d %s", element, c, e, u, coefficient, exponent, unit)
	}
}
