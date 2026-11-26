package meter

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"

	"github.com/Project-Helianthus/helianthus-modbusreg/huawei"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/roweval"
)

const (
	evidenceOwner    semreg.DefinitionID    = "helianthus.modbus"
	evidenceContract semreg.ContractVersion = "helianthus.modbus.meter/v1"
	namespaceKey     driver.NamespaceID     = "modbus.meter"

	derivationID      semreg.DefinitionID    = "derivation.modbus.grid_exchange_sign"
	derivationVersion semreg.SemanticVersion = "1.0.0"
)

// SunSpecUnit is the terminal chain of one configured unit.
type SunSpecUnit struct {
	Unit     uint8
	Snapshot sunspec.ChainSnapshot
}

// HuaweiCycle is one typed provider cycle of a path.
type HuaweiCycle struct {
	Path  Path
	Cycle huawei.ProviderCycle
}

// Input is one acquisition: the native evaluations to publish and the
// candidate template the runtime owns.
type Input struct {
	SunSpec  []SunSpecUnit
	Huawei   []HuaweiCycle
	Template semreg.FactCandidate
}

// Node is a published resource with its durable key and allocated reference.
type Node struct {
	Key driver.NodeKey
	Ref driver.NodeRef
}

// Native is a decoded value in the unit of the source. Open names a point the
// documentation leaves open.
type Native struct {
	Source string
	Unit   string
	Value  semreg.Decimal
	Sign   string
	Open   string
}

// Fact is a candidate with the subject it is about.
type Fact struct {
	Node      driver.NodeRef
	Subject   roweval.Subject
	Origin    driver.OriginKind
	Path      string
	Native    Native
	Candidate semreg.FactCandidate
}

// Unknown is an element whose value is withheld, with the reason.
type Unknown struct {
	Node   driver.NodeRef
	Key    semreg.FactKey
	Reason string
}

// Member is one value of the driver namespace.
type Member struct {
	Key    driver.MemberKey
	Native Native
}

// Publication is everything one input publishes.
type Publication struct {
	Nodes   []Node
	Links   []driver.LinkClaim
	Facts   []Fact
	Unknown []Unknown
	Members []Member
}

// Publisher publishes the infrastructure pack from native evaluations.
type Publisher struct {
	cfg   Config
	alloc Allocator

	mu     sync.RWMutex
	latest map[driver.MemberKey]Native
}

// New returns a publisher for one configured instance.
func New(cfg Config, alloc Allocator) (*Publisher, error) {
	if err := cfg.Freshness.Validate(); err != nil {
		return nil, err
	}
	if err := (driver.InstancePlan{Driver: cfg.Driver, Instance: cfg.Instance}).Validate(); err != nil {
		return nil, err
	}
	if err := cfg.Bridge.Validate(); err != nil {
		return nil, err
	}
	return &Publisher{cfg: cfg, alloc: alloc}, nil
}

type run struct {
	p     *Publisher
	in    Input
	out   Publication
	nodes map[string]Node
}

// Publish maps one input. The result does not depend on the order of the input
// slices.
func (p *Publisher) Publish(in Input) (Publication, error) {
	r := &run{p: p, in: in, nodes: map[string]Node{}}
	if err := r.declared(); err != nil {
		return Publication{}, err
	}
	if err := r.sunSpec(); err != nil {
		return Publication{}, err
	}
	if err := r.huawei(); err != nil {
		return Publication{}, err
	}
	r.sort()
	p.retain(r.out)
	return r.out, nil
}

func (r *run) node(kind semreg.DefinitionID, slot string, parent *driver.NodeRef) (driver.NodeRef, error) {
	key := driver.NodeKey{Installation: r.p.cfg.Installation, Driver: r.p.cfg.Driver, Instance: r.p.cfg.Instance, Kind: kind, Anchor: driver.AnchorKey{Slot: slot}}
	if parent != nil {
		key.Anchor.Parent = parent.ID
	}
	id := string(kind) + "|" + slot + "|" + string(key.Anchor.Parent)
	if n, ok := r.nodes[id]; ok {
		return n.Ref, nil
	}
	rid, err := r.p.alloc.Allocate(key)
	if err != nil {
		return driver.NodeRef{}, err
	}
	n := Node{Key: key, Ref: driver.NodeRef{Bridge: r.p.cfg.Bridge, ID: rid}}
	r.nodes[id] = n
	r.out.Nodes = append(r.out.Nodes, n)
	return n.Ref, nil
}

func digestOf(parts ...string) semreg.Digest {
	h := sha256.New()
	for _, s := range parts {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return semreg.Digest("sha256:" + hex.EncodeToString(h.Sum(nil)))
}

func evidence(kind semreg.DefinitionID, parts ...string) semreg.EvidenceRef {
	return semreg.EvidenceRef{Owner: evidenceOwner, Kind: kind, Digest: digestOf(parts...), Contract: evidenceContract, Access: semreg.EvidenceAccessPublic, Redaction: semreg.RedactionNone}
}

func (r *run) link(def semreg.DefinitionID, from, to driver.NodeRef, basis ...string) {
	r.out.Links = append(r.out.Links, driver.LinkClaim{
		Key:   driver.LinkKey{Type: infra(def), From: from, To: to},
		Basis: []semreg.EvidenceRef{evidence("evidence.configuration", append([]string{string(def)}, basis...)...)},
	})
}

// declared publishes the configured site and its grid connections.
func (r *run) declared() error {
	cfg := r.p.cfg
	if cfg.Site.Slot == "" {
		return nil
	}
	site, err := r.node(kindSite, cfg.Site.Slot, nil)
	if err != nil {
		return err
	}
	for _, slot := range cfg.Site.GridConnections {
		gc, err := r.node(kindGridConnection, slot, &site)
		if err != nil {
			return err
		}
		r.link("infrastructure.relation.site_grid_connection", site, gc, cfg.Site.Slot, slot)
	}
	return nil
}

func (r *run) gridConnection(slot string) (driver.NodeRef, bool, error) {
	cfg := r.p.cfg
	for _, declared := range cfg.Site.GridConnections {
		if declared == slot && cfg.Site.Slot != "" {
			site, err := r.node(kindSite, cfg.Site.Slot, nil)
			if err != nil {
				return driver.NodeRef{}, false, err
			}
			gc, err := r.node(kindGridConnection, slot, &site)
			return gc, err == nil, err
		}
	}
	return driver.NodeRef{}, false, nil
}

func (r *run) sort() {
	sort.Slice(r.out.Nodes, func(i, j int) bool { return nodeOrder(r.out.Nodes[i].Key) < nodeOrder(r.out.Nodes[j].Key) })
	sort.Slice(r.out.Links, func(i, j int) bool { return linkOrder(r.out.Links[i].Key) < linkOrder(r.out.Links[j].Key) })
	sort.Slice(r.out.Facts, func(i, j int) bool {
		return r.out.Facts[i].Candidate.CandidateID < r.out.Facts[j].Candidate.CandidateID
	})
	sort.Slice(r.out.Unknown, func(i, j int) bool { return unknownOrder(r.out.Unknown[i]) < unknownOrder(r.out.Unknown[j]) })
	sort.Slice(r.out.Members, func(i, j int) bool { return memberOrder(r.out.Members[i].Key) < memberOrder(r.out.Members[j].Key) })
}

func nodeOrder(k driver.NodeKey) string {
	return fmt.Sprint(k.Kind, "|", k.Anchor.Slot, "|", k.Anchor.Parent)
}

func linkOrder(k driver.LinkKey) string {
	return fmt.Sprint(k.Type.ID, "|", k.From.ID, "|", k.To.ID)
}

func unknownOrder(u Unknown) string {
	key, _ := semreg.CanonicalJSON(u.Key)
	return fmt.Sprint(u.Node.ID, "|", string(key))
}

func memberOrder(k driver.MemberKey) string {
	return fmt.Sprint(k.Node, "|", k.Local)
}
