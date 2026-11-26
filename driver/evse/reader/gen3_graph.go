package reader

import (
	"sort"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
)

// Catalog resource kinds of the charger's graph.
const (
	KindStation = "resource.charging_station"
	KindEVSE    = "resource.evse"
	KindConn    = "resource.connector"
	KindPhase   = "resource.phase"
	KindSession = "resource.charging_session"
	KindMeter   = "resource.meter"
)

// Relation definitions of the charger's graph.
const (
	RelationAssetEVSE        = "evse.relation.asset_evse"
	RelationEVSEConnector    = "evse.relation.evse_connector"
	RelationConnectorPhase   = "evse.relation.connector_phase"
	RelationConnectorSession = "evse.relation.connector_session"
	RelationEVSEMeter        = "evse.relation.evse_meter"
)

// BasisConfiguration is the basis of every relation the graph states.
const BasisConfiguration = "configuration"

// GraphConnector is one configured connector with its configured slot.
type GraphConnector struct {
	Label string
	Slot  string
}

// GraphConfig is the configured shape of one charger: its labels, its
// connectors in their configured slots and, when configured, its meter.
type GraphConfig struct {
	Station    string
	EVSE       string
	Connectors []GraphConnector
	Meter      string
}

// PlannedResource is one published resource of the graph.
type PlannedResource struct {
	Kind  string
	Label string
	Slot  string
}

// PlannedRelation is one relation stated from configuration.
type PlannedRelation struct {
	Definition string
	From, To   string
	Basis      string
}

// UnknownRelation is a relation the specification does not state; it stays
// unknown and names the missing statement.
type UnknownRelation struct {
	Definition string
	Reason     string
}

// GraphPlan is the runtime graph of a configured charger.
type GraphPlan struct {
	Resources []PlannedResource
	Relations []PlannedRelation
	Unknown   []UnknownRelation
}

// Plan derives the graph. Connectors are ordered by their configured slot, so
// a host ordering them by slot keeps its output order; no dimension value is
// taken from an identifier, an index, a role string or an address.
func (g GraphConfig) Plan() GraphPlan {
	plan := GraphPlan{Resources: []PlannedResource{{Kind: KindStation, Label: g.Station}, {Kind: KindEVSE, Label: g.EVSE}}}
	plan.Relations = append(plan.Relations, PlannedRelation{RelationAssetEVSE, g.Station, g.EVSE, BasisConfiguration})
	connectors := append([]GraphConnector(nil), g.Connectors...)
	sort.SliceStable(connectors, func(i, j int) bool { return connectors[i].Slot < connectors[j].Slot })
	for _, c := range connectors {
		plan.Resources = append(plan.Resources, PlannedResource{Kind: KindConn, Label: c.Label, Slot: c.Slot})
		plan.Relations = append(plan.Relations, PlannedRelation{RelationEVSEConnector, g.EVSE, c.Label, BasisConfiguration})
	}
	if g.Meter != "" {
		plan.Resources = append(plan.Resources, PlannedResource{Kind: KindMeter, Label: g.Meter})
		plan.Relations = append(plan.Relations, PlannedRelation{RelationEVSEMeter, g.EVSE, g.Meter, BasisConfiguration})
	}
	plan.Unknown = []UnknownRelation{
		{RelationConnectorPhase, "the specification states no phase component of the connector"},
		{RelationConnectorSession, "the specification states no start or end of a charging session"},
	}
	return plan
}

// NativeOnly names an observation field that is served only as the native
// record: the specification gives no field layout, unit, phase or episode
// boundary for it, so nothing is interpreted from its bytes.
type NativeOnly struct {
	Operation tesla.FC100Operation
	Field     string
	Reason    string
}

// NativeOnlyObservations lists the five observation operations by operation
// and field.
func NativeOnlyObservations() []NativeOnly {
	const reason = "the specification states no field layout, unit, phase or episode boundary"
	return []NativeOnly{
		{tesla.FC100OperationCommonSystemInfo, "terminal body", reason},
		{tesla.FC100OperationWCGetVitals, "terminal body", reason},
		{tesla.FC100OperationWCGetLifetimeStats, "terminal body", reason},
		{tesla.FC100OperationWCGetSystemInfo, "terminal body", reason},
		{tesla.FC100OperationWCGetLoadSharingState, "terminal body", reason},
	}
}
