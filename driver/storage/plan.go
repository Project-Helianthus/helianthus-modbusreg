package storage

import (
	"time"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

// Receipt is the evidence of one native observation: the clock points of its
// receipt and the parts its digest covers.
type Receipt struct {
	ObservationID string
	Wall          time.Time
	Mono          time.Duration
	ClockEpoch    string
	Evidence      []string
}

// Native is a decoded value in the unit of the source, kept beside the
// converted fact. Open names a point the documentation leaves open.
type Native struct {
	Source string
	Unit   string
	Value  semreg.Decimal
	Text   string
	Open   string
}

// Member is one value of the driver namespace.
type Member struct {
	Key    driver.MemberKey
	Native Native
}

// Unknown is an element whose value is withheld, with the reason naming the
// missing statement (rule 14).
type Unknown struct {
	Node   driver.NodeRef
	Key    semreg.FactKey
	Reason string
}

// nodeDecl declares one resource of the producer. Slot is the configured slot
// or the native position under the parent; legacy marks the RS-485 pack, whose
// anchor is the operator slot of the configured asset label.
type nodeDecl struct {
	kind     semreg.DefinitionID
	slot     string
	parent   int
	legacy   bool
	operator bool
}

type linkDecl struct {
	def      semreg.DefinitionID
	from, to int
}

type factDecl struct {
	node    int
	element semreg.DefinitionID
	dim     semreg.DefinitionID
	value   semreg.Value
	native  Native
	loss    string
}

// withdrawal names the fact of a node this producer published earlier and
// withdraws now because its case states no value.
type withdrawal struct {
	node    int
	element semreg.DefinitionID
	dim     semreg.DefinitionID
}

type serviceDecl struct {
	node int
	svc  semreg.DefinitionID
}

type unknownDecl struct {
	node    int
	element semreg.DefinitionID
	dim     semreg.DefinitionID
	reason  string
}

type memberDecl struct {
	node   int
	local  string
	native Native
}

// plan is everything one native observation publishes, before any identifier
// of the session is known.
type plan struct {
	profile      string
	protocol     string
	nativeKind   semreg.DefinitionID
	registryKind semreg.DefinitionID
	profileID    string
	receipt      Receipt
	nodes        []nodeDecl
	links        []linkDecl
	facts        []factDecl
	withdrawals  []withdrawal
	supersede    []semreg.DefinitionID
	services     []serviceDecl
	unknown      []unknownDecl
	members      []memberDecl
	identity     int
	legacyIDs    bool
}

func (pl *plan) node(n nodeDecl) int {
	for i, have := range pl.nodes {
		if have.kind == n.kind && have.slot == n.slot && have.parent == n.parent && have.legacy == n.legacy {
			return i
		}
	}
	pl.nodes = append(pl.nodes, n)
	return len(pl.nodes) - 1
}

func (pl *plan) link(def semreg.DefinitionID, from, to int) {
	for _, have := range pl.links {
		if have.def == def && have.from == from && have.to == to {
			return
		}
	}
	pl.links = append(pl.links, linkDecl{def, from, to})
}

func (pl *plan) service(node int, svc semreg.DefinitionID) {
	for _, have := range pl.services {
		if have.node == node && have.svc == svc {
			return
		}
	}
	pl.services = append(pl.services, serviceDecl{node, svc})
}
