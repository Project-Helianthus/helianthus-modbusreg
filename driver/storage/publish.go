package storage

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

// Config is the configuration of one producer instance. System and Battery are
// the configured slots of the storage system and its battery; an empty System
// states no system. First selects the identifiers of the first incarnation of
// the RS-485 family: a successor leaves it unset and scopes the binding by its
// epoch and generation.
type Config struct {
	Installation string
	Driver       driver.DriverKind
	Instance     driver.InstanceKey
	Bridge       driver.BridgeID
	System       string
	Battery      string
	SunSpec      []SunSpecStorage
	Huawei       []HuaweiBattery
	EMMASite     string
	Freshness    semreg.FreshnessPolicy
	First        bool
}

// Publisher publishes the storage pack through one driver session.
type Publisher struct {
	cfg     Config
	alloc   Allocator
	session driver.Session

	mu        sync.Mutex
	epochs    map[string]bool
	bindings  map[semreg.NativeBindingID]bool
	services  map[semreg.ServiceInstanceID]bool
	resources map[semreg.ResourceID]string
	resRev    map[semreg.ResourceID]uint64
	relations map[string]relState
	held      held
	seeded    bool
	published map[semreg.CandidateID]semreg.DefinitionID
	members   map[driver.MemberKey]Native
	unknown   map[string]Unknown
}

// New returns a publisher for one configured instance.
func New(cfg Config, alloc Allocator, session driver.Session) (*Publisher, error) {
	if err := cfg.Freshness.Validate(); err != nil {
		return nil, err
	}
	if err := (driver.InstancePlan{Driver: cfg.Driver, Instance: cfg.Instance}).Validate(); err != nil {
		return nil, err
	}
	if err := cfg.Bridge.Validate(); err != nil {
		return nil, err
	}
	if alloc == nil || session == nil {
		return nil, errors.New("storage: a publisher needs an allocator and a session")
	}
	return &Publisher{cfg: cfg, alloc: alloc, session: session,
		epochs: map[string]bool{}, bindings: map[semreg.NativeBindingID]bool{}, services: map[semreg.ServiceInstanceID]bool{},
		resources: map[semreg.ResourceID]string{}, resRev: map[semreg.ResourceID]uint64{},
		relations: map[string]relState{}, published: map[semreg.CandidateID]semreg.DefinitionID{}, members: map[driver.MemberKey]Native{}, unknown: map[string]Unknown{}}, nil
}

type relState struct {
	rev   uint64
	scope string
}

// stamp is the identity the session gives one transaction.
type stamp struct {
	asset      semreg.AssetID
	source     semreg.SourceID
	epoch      semreg.SourceEpochID
	generation semreg.Uint64
	sequence   semreg.Uint64
	observed   semreg.TimePoint
}

func stampOf(b semreg.PublicationBatch) stamp {
	return stamp{b.AssetID, b.SourceID, b.SourceEpochID, b.DriverGeneration, b.Sequence, b.ObservedAt}
}

// allocated is a node with its allocated reference.
type allocated struct {
	decl nodeDecl
	key  driver.NodeKey
	ref  driver.NodeRef
}

// outcome is what one committed transaction adds to the producer's memory.
type outcome struct {
	epoch     string
	bindings  []semreg.NativeBindingID
	services  []semreg.ServiceInstanceID
	resources map[semreg.ResourceID]string
	resRev    map[semreg.ResourceID]uint64
	relations map[string]relState
	upserted  map[semreg.CandidateID]semreg.DefinitionID
	withdrawn []semreg.CandidateID
}

// Publish stages one plan in one session transaction: the batch, then every
// link claim, then one commit. A refused stage or commit aborts the
// transaction, so neither the batch's relations nor the claims are kept, and
// the producer's memory is not advanced.
func (p *Publisher) publish(ctx context.Context, build func(stamp) (plan, error)) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	probe, err := p.session.Prepare(nil)
	if err != nil {
		return err
	}
	if err := p.seed(ctx, probe.AssetID); err != nil {
		return err
	}
	pl, err := build(stampOf(probe))
	if err != nil {
		return err
	}
	if pl.identity < 0 {
		return nil
	}
	tx, err := p.session.Publisher().Begin(ctx)
	if err != nil {
		return err
	}
	var nodes []allocated
	var claims []driver.LinkClaim
	var out outcome
	var fillErr error
	batch, err := p.session.Prepare(func(b *semreg.PublicationBatch) {
		nodes, claims, out, fillErr = p.fill(b, tx, pl)
	})
	if err == nil {
		err = fillErr
	}
	if err == nil && batchEmpty(batch) {
		p.remember(out, nodes, pl)
		return tx.Abort(ctx)
	}
	if err == nil {
		err = tx.Stage(batch)
	}
	for i := 0; err == nil && i < len(claims); i++ {
		err = tx.StageLink(claims[i])
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		return errors.Join(err, tx.Abort(ctx))
	}
	p.remember(out, nodes, pl)
	return nil
}

func (p *Publisher) remember(out outcome, nodes []allocated, pl plan) {
	p.epochs[out.epoch] = true
	for _, id := range out.bindings {
		p.bindings[id] = true
	}
	for _, id := range out.services {
		p.services[id] = true
	}
	for id, rev := range out.resRev {
		p.resRev[id] = rev
		p.resources[id] = out.resources[id]
	}
	for key, state := range out.relations {
		p.relations[key] = state
	}
	for id, element := range out.upserted {
		p.published[id] = element
	}
	for _, id := range out.withdrawn {
		delete(p.published, id)
	}
	for _, m := range pl.members {
		key := p.memberKey(nodes[m.node], m.local)
		p.members[key] = m.native
	}
	for _, u := range pl.unknown {
		key := factKey(u.dim, u.element, nodes[u.node])
		p.unknown[unknownKey(key)] = Unknown{Node: nodes[u.node].ref, Key: key, Reason: u.reason}
	}
	for _, f := range pl.facts {
		delete(p.unknown, unknownKey(factKey(f.dim, f.element, nodes[f.node])))
	}
}

func (p *Publisher) memberKey(n allocated, local string) driver.MemberKey {
	return driver.MemberKey{Namespace: namespaceKey, Instance: p.cfg.Instance, Node: driver.NodeID(n.ref.ID), Local: driver.MemberID(local)}
}

func (p *Publisher) allocate(st stamp, pl plan) ([]allocated, error) {
	out := make([]allocated, len(pl.nodes))
	for i, d := range pl.nodes {
		key := driver.NodeKey{Installation: p.cfg.Installation, Driver: p.cfg.Driver, Instance: p.cfg.Instance, Kind: d.kind}
		switch {
		case d.legacy:
			key.Anchor = driver.AnchorKey{Slot: packSlot(st.asset)}
		case d.parent >= 0:
			key.Anchor = driver.AnchorKey{Parent: out[d.parent].ref.ID, Slot: d.slot}
		default:
			key.Anchor = driver.AnchorKey{Slot: d.slot}
		}
		id, err := p.alloc.Allocate(key)
		if err != nil {
			return nil, err
		}
		out[i] = allocated{decl: d, key: key, ref: driver.NodeRef{Bridge: p.cfg.Bridge, ID: id}}
	}
	return out, nil
}

// bindingID is the binding of one pack. The host accepts an observed fact only
// when the identity link of its binding names the fact's subject or an anchor
// parent of it, up to the first equipment resource, and one link names one
// resource; so every pack is the equipment of its own binding. The first
// incarnation of the RS-485 family has exactly one pack and keeps the frozen
// recipe; every other family scopes the binding by the pack's resource.
func (p *Publisher) bindingID(st stamp, pl plan, owner allocated) semreg.NativeBindingID {
	if pl.legacyIDs {
		if p.cfg.First {
			return semreg.NativeBindingID("binding:semantic-storage:" + hashHex(string(st.asset), string(st.source), pl.profile)[:32])
		}
		return semreg.NativeBindingID("binding:semantic-storage:" + hashHex(string(st.asset), string(st.source), string(st.epoch), string(st.generation))[:32])
	}
	return semreg.NativeBindingID("binding:modbus-storage:" + hashHex(string(st.asset), string(st.source), pl.profile, string(st.epoch), string(st.generation), string(owner.ref.ID))[:32])
}

func (p *Publisher) serviceID(st stamp, pl plan, binding semreg.NativeBindingID, n allocated, svc semreg.DefinitionID) semreg.ServiceInstanceID {
	name := string(svc[len("storage.service."):])
	if pl.legacyIDs && n.decl.legacy {
		return semreg.ServiceInstanceID("service:semantic-storage:" + hashHex(string(st.asset), string(binding), name)[:32])
	}
	return semreg.ServiceInstanceID("service:modbus-storage:" + hashHex(string(st.asset), string(binding), name, string(n.ref.ID))[:32])
}

func (p *Publisher) candidateHash(st stamp, pl plan, binding semreg.NativeBindingID, n allocated, element semreg.DefinitionID) (string, string) {
	if pl.legacyIDs {
		if n.decl.legacy {
			return "semantic-storage", hashHex(string(st.asset), string(binding), string(element))[:32]
		}
		return "semantic-storage", hashHex(string(st.asset), string(binding), string(element), string(n.ref.ID))[:32]
	}
	return "modbus-storage", hashHex(string(st.asset), string(binding), string(element), string(n.ref.ID))[:32]
}

func (p *Publisher) candidateID(st stamp, pl plan, binding semreg.NativeBindingID, n allocated, element semreg.DefinitionID) semreg.CandidateID {
	family, h := p.candidateHash(st, pl, binding, n, element)
	return semreg.CandidateID("candidate:" + family + ":" + h)
}

func wall(t time.Time) semreg.TimePoint {
	return semreg.TimePoint{UnixNanoseconds: semreg.Int64(strconv.FormatInt(t.UTC().UnixNano(), 10)), ClockID: "wall.utc", UncertaintyNS: "0"}
}

func factKey(dim semreg.DefinitionID, element semreg.DefinitionID, node allocated) semreg.FactKey {
	return semreg.FactKey{PackID: packID, PackVersion: packVersion, FactID: element,
		Dimensions: []semreg.Dimension{{ID: dim, Value: text(string(node.ref.ID))}}}
}

func evidenceKey(e semreg.EvidenceRef) string {
	return string(e.Owner) + "\x00" + string(e.Kind) + "\x00" + string(e.Contract) + "\x00" + string(e.Digest)
}

func sortedEvidence(in ...semreg.EvidenceRef) []semreg.EvidenceRef {
	out := append([]semreg.EvidenceRef(nil), in...)
	sort.Slice(out, func(i, j int) bool { return evidenceKey(out[i]) < evidenceKey(out[j]) })
	return out
}

func (p *Publisher) candidate(st stamp, pl plan, binding semreg.NativeBindingID, n allocated, f factDecl) semreg.FactCandidate {
	family, h := p.candidateHash(st, pl, binding, n, f.element)
	id := semreg.CandidateID("candidate:" + family + ":" + h)
	observation := evidence(pl.nativeKind, pl.receipt.Evidence...)
	native := evidence("evidence.native", f.native.Source, f.native.Unit, f.native.Value.Coefficient, strconv.Itoa(int(f.native.Value.Exponent10)), f.native.Text)
	refs := sortedEvidence(observation, native)
	mono := semreg.MonotonicPoint{ClockEpochID: semreg.ClockEpochID(pl.receipt.ClockEpoch), Nanoseconds: semreg.Uint64(strconv.FormatInt(pl.receipt.Mono.Nanoseconds(), 10))}
	value := f.value
	source := st.source
	epoch := st.epoch
	generation := st.generation
	bound := binding
	return semreg.FactCandidate{
		CandidateID: id, Key: factKey(f.dim, f.element, n), Value: &value,
		Quality: semreg.Quality{Assertion: semreg.AssertionObserved, Qualification: semreg.QualificationQualified, Promotion: semreg.PromotionPromoted,
			Validity: semreg.ValidityGood, Availability: semreg.AvailabilityAvailable, Freshness: semreg.FreshnessFresh, Reasons: []semreg.DefinitionID{}},
		Times:           semreg.Times{ReceivedAt: wall(pl.receipt.Wall), ReceiptMonotonic: mono, EvaluatedAt: wall(pl.receipt.Wall), EvaluateMonotonic: mono},
		FreshnessPolicy: p.cfg.Freshness, BindingID: &bound, SourceEpochID: &epoch, DriverGeneration: &generation,
		Origin: semreg.OriginRef{OriginID: semreg.OriginID("origin:" + family + ":" + h), Kind: semreg.OriginNativeObservation,
			SourceID: &source, SourceEpochID: &epoch, BindingID: &bound, Evidence: refs},
		Evidence: refs, Revision: st.sequence,
	}
}

func resourceClass(kind semreg.DefinitionID) semreg.AnchorClass {
	switch kind {
	case kindSystem:
		return semreg.AnchorFunction
	case kindInterface, kindCell:
		return semreg.AnchorComponent
	}
	return semreg.AnchorEquipment
}

func next(rev uint64, seen bool) uint64 {
	if !seen {
		return 1
	}
	return rev + 1
}

func (p *Publisher) resource(st stamp, binding semreg.NativeBindingID, all []allocated, i int, origin semreg.OriginRef, slot string, basis semreg.AnchorBasis, rev uint64) semreg.Resource {
	n := all[i]
	s := slot
	anchor := semreg.ResourceAnchor{Class: resourceClass(n.decl.kind), Basis: basis, Slot: &s, Evidence: origin.Evidence}
	if anchor.Class == semreg.AnchorComponent && n.decl.parent >= 0 {
		parent := all[n.decl.parent].ref.ID
		anchor.Parent = &parent
	}
	return semreg.Resource{ResourceID: n.ref.ID, Kind: n.decl.kind, Anchor: anchor, State: semreg.ResourceCurrent, Origin: origin, Revision: semreg.Uint64(strconv.FormatUint(rev, 10))}
}

func (p *Publisher) operatorOrigin(parts ...string) semreg.OriginRef {
	ev := evidence("evidence.configuration", parts...)
	return semreg.OriginRef{OriginID: semreg.OriginID("origin:modbus-storage-operator:" + hashHex(parts...)[:32]), Kind: semreg.OriginOperator, Evidence: []semreg.EvidenceRef{ev}}
}

func (p *Publisher) nativeOrigin(st stamp, binding semreg.NativeBindingID, parts ...string) semreg.OriginRef {
	ev := evidence("evidence.gate_rule", parts...)
	source, epoch, bound := st.source, st.epoch, binding
	return semreg.OriginRef{OriginID: semreg.OriginID("origin:modbus-storage-native:" + hashHex(parts...)[:32]), Kind: semreg.OriginNativeObservation,
		SourceID: &source, SourceEpochID: &epoch, BindingID: &bound, Evidence: []semreg.EvidenceRef{ev}}
}

// carryLinks replaces the link of each pending binding and keeps every other
// link of the batch.
func carryLinks(built, pending []semreg.IdentityLink) []semreg.IdentityLink {
	if len(pending) == 0 {
		return built
	}
	replaced := map[semreg.NativeBindingID]bool{}
	for _, link := range pending {
		replaced[link.BindingID] = true
	}
	out := []semreg.IdentityLink{}
	for _, link := range built {
		if !replaced[link.BindingID] {
			out = append(out, link)
		}
	}
	return append(out, pending...)
}

// ownerOf is the pack that carries node i: the node itself or its nearest
// ancestor that is a pack. A node under no pack (the battery, the system) is
// carried by the plan's first pack.
func ownerOf(all []allocated, i, primary int) int {
	for n := i; n >= 0; n = all[n].decl.parent {
		if all[n].decl.kind == kindPack {
			return n
		}
	}
	return primary
}

func (p *Publisher) fill(b *semreg.PublicationBatch, tx driver.PublicationTransaction, pl plan) ([]allocated, []driver.LinkClaim, outcome, error) {
	st := stampOf(*b)
	out := outcome{epoch: string(st.source) + "\x00" + string(st.epoch), resources: map[semreg.ResourceID]string{}, resRev: map[semreg.ResourceID]uint64{}, relations: map[string]relState{}, upserted: map[semreg.CandidateID]semreg.DefinitionID{}}
	all, err := p.allocate(st, pl)
	if err != nil {
		return nil, nil, out, err
	}
	owned := map[int]semreg.NativeBindingID{}
	var packs []int
	for i, n := range all {
		if n.decl.kind == kindPack {
			owned[i] = p.bindingID(st, pl, n)
			packs = append(packs, i)
		}
	}
	bindingOf := func(node int) semreg.NativeBindingID { return owned[ownerOf(all, node, pl.identity)] }
	registry := evidence(pl.registryKind, pl.profile)
	if !p.epochs[out.epoch] {
		b.SourceUpserts = append(b.SourceUpserts, semreg.SourceDescriptor{SourceID: st.source, SourceEpochID: st.epoch, ProtocolID: semreg.DefinitionID(pl.protocol),
			ProfileID: semreg.DefinitionID(pl.profileID), ProfileVersion: semreg.VersionLabel("1"), RegistryEvidence: registry, StartedAt: wall(pl.receipt.Wall), State: semreg.SourceCurrent, Revision: st.sequence})
	}
	for _, i := range packs {
		binding := owned[i]
		out.bindings = append(out.bindings, binding)
		if p.bindings[binding] {
			continue
		}
		b.BindingUpserts = append(b.BindingUpserts, semreg.NativeBinding{BindingID: binding, AssetID: st.asset, SourceID: st.source, SourceEpochID: st.epoch,
			DriverGeneration: st.generation, NativeResource: evidence(pl.nativeKind+".binding", string(binding)), State: semreg.BindingCurrent, Revision: st.sequence})
		resource := all[i].ref.ID
		b.IdentityLinkUpserts = append(b.IdentityLinkUpserts, semreg.IdentityLink{AssetID: st.asset, BindingID: binding, State: semreg.LinkQualified,
			Basis: []semreg.EvidenceRef{evidence(pl.nativeKind, pl.receipt.Evidence...)}, Revision: st.sequence, Resource: &resource})
	}
	var claims []driver.LinkClaim
	for i, n := range all {
		binding := bindingOf(i)
		prior, seen := p.resources[n.ref.ID]
		origin, basis, slot := p.operatorOrigin(string(n.decl.kind), n.decl.slot), semreg.AnchorOperatorSlot, n.decl.slot
		if n.decl.legacy {
			slot = packSlot(st.asset)
			origin = semreg.OriginRef{OriginID: semreg.OriginID("origin:semantic-storage-pack:" + hashHex(string(kindPack), slot)[:32]), Kind: semreg.OriginOperator,
				Evidence: []semreg.EvidenceRef{evidence("operator.storage.pack_label", slot)}}
		} else if !n.decl.operator {
			origin, basis = p.nativeOrigin(st, binding, string(n.decl.kind), string(n.ref.ID)), semreg.AnchorNativeSlot
		}
		scope := string(origin.Kind)
		if origin.Kind == semreg.OriginNativeObservation {
			scope = string(binding)
		}
		if n.decl.parent >= 0 && resourceClass(n.decl.kind) != semreg.AnchorComponent {
			slot = all[n.decl.parent].decl.slot + "/" + slot
		}
		if seen && prior == scope {
			continue
		}
		rev := next(p.resRev[n.ref.ID], seen)
		r := p.resource(st, binding, all, i, origin, slot, basis, rev)
		if held, ok := p.held.resources[n.ref.ID]; ok && !seen {
			var heldRev uint64
			heldRev, r.Revision = revisionOf(held.Revision), held.Revision
			if sameRecord(r, held) {
				out.resources[n.ref.ID], out.resRev[n.ref.ID] = scope, heldRev
				continue
			}
			rev = heldRev + 1
			r.Revision = semreg.Uint64(strconv.FormatUint(rev, 10))
		}
		b.ResourceUpserts = append(b.ResourceUpserts, r)
		out.resources[n.ref.ID], out.resRev[n.ref.ID] = scope, rev
	}
	for _, l := range pl.links {
		from, to := all[l.from], all[l.to]
		key := string(l.def) + "\x00" + string(from.ref.ID) + "\x00" + string(to.ref.ID)
		class, origin := semreg.RelationGateRule, p.nativeOrigin(st, bindingOf(l.to), string(l.def), string(from.ref.ID), string(to.ref.ID))
		scope := string(bindingOf(l.to))
		if l.def == relSystemBattery {
			class, origin, scope = semreg.RelationConfiguration, p.operatorOrigin(string(l.def), string(from.ref.ID), string(to.ref.ID)), "configuration"
		}
		held, seen := p.relations[key]
		if seen && held.scope == scope {
			continue
		}
		rev := next(held.rev, seen)
		relation := semreg.RelationInstance{Definition: def(l.def), From: from.ref.ID, To: to.ref.ID, State: semreg.LinkQualified,
			BasisClass: class, Basis: origin.Evidence, Origin: origin, Revision: semreg.Uint64(strconv.FormatUint(rev, 10))}
		if old, ok := p.held.relations[key]; ok && !seen {
			var heldRev uint64
			heldRev, relation.Revision = revisionOf(old.Revision), old.Revision
			if sameRecord(relation, old) {
				out.relations[key] = relState{rev: heldRev, scope: scope}
				continue
			}
			rev = heldRev + 1
			relation.Revision = semreg.Uint64(strconv.FormatUint(rev, 10))
		}
		b.RelationUpserts = append(b.RelationUpserts, relation)
		out.relations[key] = relState{rev: rev, scope: scope}
		claims = append(claims, driver.LinkClaim{Key: driver.LinkKey{Type: def(l.def), From: from.ref, To: to.ref}, Basis: origin.Evidence})
	}
	for _, s := range pl.services {
		n := all[s.node]
		binding := bindingOf(s.node)
		id := p.serviceID(st, pl, binding, n, s.svc)
		if p.services[id] {
			continue
		}
		resource := n.ref.ID
		b.ServiceUpserts = append(b.ServiceUpserts, semreg.ServiceInstance{InstanceID: id, AssetID: st.asset, Definition: def(s.svc), BindingID: binding,
			SourceEpochID: st.epoch, DriverGeneration: st.generation, Qualification: semreg.QualificationQualified, Availability: semreg.AvailabilityAvailable,
			Revision: st.sequence, Resource: &resource})
		out.services = append(out.services, id)
	}
	upserted := map[semreg.CandidateID]bool{}
	for _, f := range pl.facts {
		c := p.candidate(st, pl, bindingOf(f.node), all[f.node], f)
		b.FactUpserts = append(b.FactUpserts, c)
		out.upserted[c.CandidateID] = f.element
		upserted[c.CandidateID] = true
	}
	withdrawn := map[semreg.CandidateID]bool{}
	for _, w := range pl.withdrawals {
		id := p.candidateID(st, pl, bindingOf(w.node), all[w.node], w.element)
		if _, held := p.published[id]; held && !upserted[id] && !withdrawn[id] {
			b.FactWithdrawals = append(b.FactWithdrawals, id)
			out.withdrawn = append(out.withdrawn, id)
			withdrawn[id] = true
		}
	}
	for id, element := range p.published {
		for _, replaced := range pl.supersede {
			if element == replaced && !upserted[id] && !withdrawn[id] {
				b.FactWithdrawals = append(b.FactWithdrawals, id)
				out.withdrawn = append(out.withdrawn, id)
				withdrawn[id] = true
			}
		}
	}
	sortBatch(b, claims)
	b.IdentityLinkUpserts = carryLinks(b.IdentityLinkUpserts, pendingLinks(tx, b, out.bindings))
	if out.withdrawn == nil {
		out.withdrawn = []semreg.CandidateID{}
	}
	return all, claims, out, nil
}

// sortBatch orders the batch's records and the claims so that a replay of the
// same cut yields the same bytes.
func sortBatch(b *semreg.PublicationBatch, claims []driver.LinkClaim) {
	sort.Slice(b.FactUpserts, func(i, j int) bool { return b.FactUpserts[i].CandidateID < b.FactUpserts[j].CandidateID })
	sort.Slice(b.FactWithdrawals, func(i, j int) bool { return b.FactWithdrawals[i] < b.FactWithdrawals[j] })
	sort.Slice(b.ServiceUpserts, func(i, j int) bool { return b.ServiceUpserts[i].InstanceID < b.ServiceUpserts[j].InstanceID })
	sort.Slice(b.ResourceUpserts, func(i, j int) bool { return b.ResourceUpserts[i].ResourceID < b.ResourceUpserts[j].ResourceID })
	sort.Slice(b.RelationUpserts, func(i, j int) bool { return relationOrder(b.RelationUpserts[i]) < relationOrder(b.RelationUpserts[j]) })
	sort.Slice(b.BindingUpserts, func(i, j int) bool { return b.BindingUpserts[i].BindingID < b.BindingUpserts[j].BindingID })
	sort.Slice(b.IdentityLinkUpserts, func(i, j int) bool { return b.IdentityLinkUpserts[i].BindingID < b.IdentityLinkUpserts[j].BindingID })
	sort.Slice(claims, func(i, j int) bool { return claimOrder(claims[i]) < claimOrder(claims[j]) })
}

// pendingLinks returns the operator links the host holds for this batch's own
// bindings only.
func pendingLinks(tx driver.PublicationTransaction, b *semreg.PublicationBatch, bindings []semreg.NativeBindingID) []semreg.IdentityLink {
	mine := map[semreg.NativeBindingID]bool{}
	for _, id := range bindings {
		mine[id] = true
	}
	var pending []semreg.IdentityLink
	for _, link := range tx.OperatorLinks(b.BindingUpserts) {
		if mine[link.BindingID] {
			pending = append(pending, link)
		}
	}
	return pending
}

// held is what the host kept of this producer's resources and relations
// before this process published; a successor revises them from there.
type held struct {
	resources map[semreg.ResourceID]semreg.Resource
	relations map[string]semreg.RelationInstance
}

// seed reads the resources and relations the host already holds for the asset,
// once, so that a restarted producer upserts a changed record at the held
// revision plus one and leaves an identical one alone, as the host requires.
func (p *Publisher) seed(ctx context.Context, asset semreg.AssetID) error {
	if p.seeded {
		return nil
	}
	cut, err := p.session.SemanticInput().Capture(ctx, driver.Filter{Assets: []semreg.AssetID{asset}})
	if err != nil {
		return err
	}
	p.held = held{resources: map[semreg.ResourceID]semreg.Resource{}, relations: map[string]semreg.RelationInstance{}}
	for _, snapshot := range cut.Snapshots {
		if snapshot.AssetID != asset {
			continue
		}
		for _, r := range snapshot.Resources {
			p.held.resources[r.ResourceID] = r
		}
		for _, r := range snapshot.Relations {
			p.held.relations[string(r.Definition.ID)+"\x00"+string(r.From)+"\x00"+string(r.To)] = r
		}
	}
	p.seeded = true
	return nil
}

func revisionOf(r semreg.Uint64) uint64 {
	n, _ := strconv.ParseUint(string(r), 10, 64)
	return n
}

// sameRecord compares two records of the same revision by their canonical form.
func sameRecord(a, b semreg.Record) bool {
	x, errA := semreg.CanonicalJSON(a)
	y, errB := semreg.CanonicalJSON(b)
	return errA == nil && errB == nil && string(x) == string(y)
}

func unknownKey(k semreg.FactKey) string {
	canonical, _ := semreg.CanonicalJSON(k)
	return string(canonical)
}

// Unknown lists the elements the producer withholds, each with the reason
// naming the missing statement.
func (p *Publisher) Unknown() []Unknown {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Unknown, 0, len(p.unknown))
	for _, u := range p.unknown {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return unknownKey(out[i].Key) < unknownKey(out[j].Key) })
	return out
}

func batchEmpty(b semreg.PublicationBatch) bool {
	return len(b.SourceUpserts) == 0 && len(b.BindingUpserts) == 0 && len(b.IdentityLinkUpserts) == 0 && len(b.FactUpserts) == 0 && len(b.FactWithdrawals) == 0 &&
		len(b.ServiceUpserts) == 0 && len(b.ResourceUpserts) == 0 && len(b.RelationUpserts) == 0
}

func relationOrder(r semreg.RelationInstance) string {
	return string(r.Definition.ID) + "\x00" + string(r.From) + "\x00" + string(r.To)
}

func claimOrder(c driver.LinkClaim) string {
	return string(c.Key.Type.ID) + "\x00" + string(c.Key.From.ID) + "\x00" + string(c.Key.To.ID)
}
