package reader

import (
	"sort"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

const evidenceGraph semreg.DefinitionID = "operator.configuration.charger_graph"

// graphID names one resource from the asset, its kind, its parent and its
// configured slot, so it is the same across a restart and any discovery order
// and differs between two chargers.
func (p ChargerPublication) graphID(kind string, parent semreg.ResourceID, slot string) semreg.ResourceID {
	return semreg.ResourceID(opaque("resource:tesla-wc3", string(p.Asset), kind, string(parent), slot))
}

func (p ChargerPublication) graphOrigin(parts ...string) semreg.OriginRef {
	return semreg.OriginRef{OriginID: semreg.OriginID(opaque("origin:tesla-wc3-graph", append([]string{string(p.Asset)}, parts...)...)),
		Kind: semreg.OriginOperator, Evidence: []semreg.EvidenceRef{p.evidence(evidenceGraph, append([]string{string(p.Asset)}, parts...)...)}}
}

type graphRecords struct {
	resources []semreg.Resource
	relations []semreg.RelationInstance
}

func (g graphRecords) empty() bool { return len(g.resources) == 0 && len(g.relations) == 0 }

// graphRecords states the configured graph as resource and relation upserts.
// Every anchor is an operator slot: the configured label for the station, the
// EVSE and the meter, and the configured slot for a connector. No native
// identity is proven for any of them. A graph that was not configured states
// nothing.
func (p ChargerPublication) graphRecords(plan GraphPlan) graphRecords {
	if len(plan.Resources) == 0 {
		return graphRecords{}
	}
	type endKey struct{ kind, label string }
	ids := map[endKey]semreg.ResourceID{}
	var out graphRecords
	var station, evse semreg.ResourceID
	for _, r := range plan.Resources {
		var anchor semreg.ResourceAnchor
		slot := r.Label
		class := semreg.AnchorComponent
		var parent *semreg.ResourceID
		switch r.Kind {
		case KindStation, KindMeter:
			class = semreg.AnchorEquipment
		case KindEVSE:
			parent = &station
		case KindConn:
			parent, slot = &evse, r.Slot
		}
		parentID := semreg.ResourceID("")
		if parent != nil {
			copied := *parent
			parent, parentID = &copied, copied
		}
		id := p.graphID(r.Kind, parentID, slot)
		origin := p.graphOrigin(r.Kind, string(parentID), slot)
		text := slot
		anchor = semreg.ResourceAnchor{Class: class, Basis: semreg.AnchorOperatorSlot, Parent: parent, Slot: &text, Evidence: origin.Evidence}
		out.resources = append(out.resources, semreg.Resource{ResourceID: id, Kind: semreg.DefinitionID(r.Kind), Anchor: anchor,
			State: semreg.ResourceCurrent, Origin: origin, Revision: "1"})
		ids[endKey{r.Kind, r.Label}] = id
		switch r.Kind {
		case KindStation:
			station = id
		case KindEVSE:
			evse = id
		}
	}
	for _, r := range plan.Relations {
		from, to := ids[endKey{r.FromKind, r.From}], ids[endKey{r.ToKind, r.To}]
		origin := p.graphOrigin(r.Definition, string(from), string(to))
		out.relations = append(out.relations, semreg.RelationInstance{Definition: defRef(semreg.DefinitionID(r.Definition)), From: from, To: to,
			State: semreg.LinkQualified, BasisClass: semreg.RelationConfiguration, Basis: origin.Evidence, Origin: origin, Revision: "1"})
	}
	sort.Slice(out.resources, func(i, j int) bool { return out.resources[i].ResourceID < out.resources[j].ResourceID })
	keys := make([]string, len(out.relations))
	for i, r := range out.relations {
		keys[i] = relationSortKey(r)
	}
	sort.Sort(relationOrder{out.relations, keys})
	return out
}

func relationSortKey(r semreg.RelationInstance) string {
	definition, _ := semreg.CanonicalJSON(r.Definition)
	return string(definition) + "\x00" + string(r.From) + "\x00" + string(r.To)
}

type relationOrder struct {
	relations []semreg.RelationInstance
	keys      []string
}

func (o relationOrder) Len() int           { return len(o.relations) }
func (o relationOrder) Less(i, j int) bool { return o.keys[i] < o.keys[j] }
func (o relationOrder) Swap(i, j int) {
	o.relations[i], o.relations[j] = o.relations[j], o.relations[i]
	o.keys[i], o.keys[j] = o.keys[j], o.keys[i]
}
