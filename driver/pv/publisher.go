package pv

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
	evidenceContract semreg.ContractVersion = "helianthus.modbus.pv/v1"
	namespaceKey     driver.NamespaceID     = "modbus.pv"
)

// SunSpecUnit is the terminal chain of one acquired unit.
type SunSpecUnit struct {
	Unit     uint8
	Snapshot sunspec.ChainSnapshot
}

// Companion is a battery or meter resource of the dongle family that is
// current in the cut and is read through the inverter child at Unit.
type Companion struct {
	Unit uint8
	Node driver.NodeRef
}

// Input is one acquisition: the native evaluations to publish and the candidate
// template the runtime owns.
type Input struct {
	SunSpec    []SunSpecUnit
	Huawei     []huawei.ProviderCycle
	Companions []Companion
	Template   semreg.FactCandidate
}

// Node is a published resource with its durable key and allocated reference.
type Node struct {
	Key driver.NodeKey
	Ref driver.NodeRef
}

// Native is a decoded value in the unit of its source.
type Native struct {
	Source string
	Unit   string
	Value  semreg.Decimal
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

// UnknownLink is a relation the sources do not state, with the missing
// statement named. It is never an absence.
type UnknownLink struct {
	Type   semreg.DefinitionRef
	From   driver.NodeRef
	Reason string
}

// Withheld is a native value that is not published to SemReg, with its reason.
// It stays readable in the driver namespace.
type Withheld struct {
	Path   string
	Reason string
}

// Decision is the capability decision of one SunSpec source: GO when the
// registry admits it, NO_GO with its reason otherwise. Level is the Fronius
// observed flavor recorded beside the decision; it decides nothing.
type Decision struct {
	Unit       uint8
	Common     uint32
	Source     uint32
	Model      uint16
	Capability string
	Go         bool
	Reason     string
	Level      string
}

// Unclaimed is a device without a complete manufacturer and serial pair.
type Unclaimed struct {
	Node   driver.NodeRef
	Reason string
}

// Point is one value of the driver namespace: a decoded SunSpec point or a
// typed dongle-family reading, in the unit of its source.
type Point struct {
	Model    uint16   `json:"model,omitempty"`
	Ordinal  uint32   `json:"ordinal,omitempty"`
	Name     string   `json:"name"`
	Field    string   `json:"field,omitempty"`
	Unit     string   `json:"unit,omitempty"`
	Value    string   `json:"value,omitempty"`
	Raw      []uint16 `json:"raw,omitempty"`
	Validity string   `json:"validity"`
	State    string   `json:"state,omitempty"`
	Retained bool     `json:"retained,omitempty"`
	Reason   string   `json:"reason,omitempty"`
}

// Member is one value of the driver namespace.
type Member struct {
	Key   driver.MemberKey
	Point Point
}

// Publication is everything one input publishes. SunSpec relations and
// dongle-family relations are kept apart: the SunSpec batch carries the
// pending operator links, the dongle-family logger batch does not.
type Publication struct {
	Nodes        []Node
	Links        []driver.LinkClaim
	DongleLinks  []driver.LinkClaim
	Facts        []Fact
	UnknownLinks []UnknownLink
	Withheld     []Withheld
	Decisions    []Decision
	Claims       []driver.ManufacturerSerialClaim
	Unclaimed    []Unclaimed
	Members      []Member
}

// Publisher publishes the PV pack from native evaluations.
type Publisher struct {
	cfg   Config
	alloc Allocator

	mu     sync.RWMutex
	latest map[driver.MemberKey]Point
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
	p          *Publisher
	in         Input
	out        Publication
	nodes      map[string]Node
	registries map[sunspec.SchemaRevision]sunspec.DecoderRegistry
	linked     map[string]bool
}

// Publish maps one input. The result does not depend on the order of the input
// slices.
func (p *Publisher) Publish(in Input) (Publication, error) {
	r := &run{p: p, in: in, nodes: map[string]Node{}, registries: map[sunspec.SchemaRevision]sunspec.DecoderRegistry{}, linked: map[string]bool{}}
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

type basisClass string

const (
	gateRule          basisClass = "gate_rule"
	nativeObservation basisClass = "native_observation"
)

func (r *run) link(dongle bool, typ semreg.DefinitionRef, from, to driver.NodeRef, class basisClass, rule string) {
	if id := fmt.Sprint(dongle, "|", linkOrder(driver.LinkKey{Type: typ, From: from, To: to})); r.linked[id] {
		return
	} else {
		r.linked[id] = true
	}
	claim := driver.LinkClaim{
		Key:   driver.LinkKey{Type: typ, From: from, To: to},
		Basis: []semreg.EvidenceRef{evidence(semreg.DefinitionID("evidence."+string(class)), string(typ.ID), rule)},
	}
	if dongle {
		r.out.DongleLinks = append(r.out.DongleLinks, claim)
		return
	}
	r.out.Links = append(r.out.Links, claim)
}

func relaysRef() semreg.DefinitionRef { return coreDef("relation.relays") }

func (r *run) withheld(path, reason string) {
	r.out.Withheld = append(r.out.Withheld, Withheld{Path: path, Reason: reason})
}

func (r *run) sort() {
	sort.Slice(r.out.Nodes, func(i, j int) bool { return nodeOrder(r.out.Nodes[i].Key) < nodeOrder(r.out.Nodes[j].Key) })
	for _, links := range [][]driver.LinkClaim{r.out.Links, r.out.DongleLinks} {
		links := links
		sort.Slice(links, func(i, j int) bool { return linkOrder(links[i].Key) < linkOrder(links[j].Key) })
	}
	sort.Slice(r.out.Facts, func(i, j int) bool {
		return r.out.Facts[i].Candidate.CandidateID < r.out.Facts[j].Candidate.CandidateID
	})
	sort.Slice(r.out.UnknownLinks, func(i, j int) bool {
		a, b := r.out.UnknownLinks[i], r.out.UnknownLinks[j]
		return fmt.Sprint(a.Type.ID, "|", a.From.ID) < fmt.Sprint(b.Type.ID, "|", b.From.ID)
	})
	sort.Slice(r.out.Withheld, func(i, j int) bool {
		a, b := r.out.Withheld[i], r.out.Withheld[j]
		return a.Path+"\x00"+a.Reason < b.Path+"\x00"+b.Reason
	})
	sort.Slice(r.out.Decisions, func(i, j int) bool {
		a, b := r.out.Decisions[i], r.out.Decisions[j]
		return fmt.Sprintf("%03d|%08d|%08d|%s", a.Unit, a.Common, a.Source, a.Capability) < fmt.Sprintf("%03d|%08d|%08d|%s", b.Unit, b.Common, b.Source, b.Capability)
	})
	sort.Slice(r.out.Claims, func(i, j int) bool { return r.out.Claims[i].Member.ID < r.out.Claims[j].Member.ID })
	sort.Slice(r.out.Unclaimed, func(i, j int) bool { return r.out.Unclaimed[i].Node.ID < r.out.Unclaimed[j].Node.ID })
	sort.Slice(r.out.Members, func(i, j int) bool { return memberOrder(r.out.Members[i].Key) < memberOrder(r.out.Members[j].Key) })
}

func nodeOrder(k driver.NodeKey) string {
	return fmt.Sprint(k.Kind, "|", k.Anchor.Slot, "|", k.Anchor.Parent)
}

func linkOrder(k driver.LinkKey) string {
	return fmt.Sprint(k.Type.ID, "|", k.From.ID, "|", k.To.ID)
}

func memberOrder(k driver.MemberKey) string { return fmt.Sprint(k.Node, "|", k.Local) }
