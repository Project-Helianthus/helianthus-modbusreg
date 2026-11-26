package reader

import (
	"fmt"
	"sort"
	"strconv"

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

// PlannedRelation is one relation stated from configuration. From and To are
// the labels of its ends, each within the kind its definition names.
type PlannedRelation struct {
	Definition       string
	From, To         string
	Basis            string
	FromKind, ToKind string
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

// IsZero reports a graph with nothing configured.
func (g GraphConfig) IsZero() bool {
	return g.Station == "" && g.EVSE == "" && g.Meter == "" && len(g.Connectors) == 0
}

// contains checks that the graph names the EVSE and the connector the reader
// publishes facts for.
func (g GraphConfig) contains(evse, connector string) error {
	if g.EVSE != evse {
		return fmt.Errorf("graph evse %q differs from the publication evse", g.EVSE)
	}
	for _, c := range g.Connectors {
		if c.Label == connector {
			return nil
		}
	}
	return fmt.Errorf("graph has no connector %q", connector)
}

// slotLess orders configured slots by their numeric value, so slot 10 follows
// slot 9. A slot that is not a number follows every number and orders as text.
func slotLess(a, b string) bool {
	na, errA := strconv.ParseUint(a, 10, 64)
	nb, errB := strconv.ParseUint(b, 10, 64)
	switch {
	case errA == nil && errB == nil && na != nb:
		return na < nb
	case errA == nil && errB != nil:
		return true
	case errA != nil && errB == nil:
		return false
	}
	return a < b
}

// Validate reports the first configured value that cannot state a graph: an
// empty label, a slot that is not a number, or a repeated label or slot.
func (g GraphConfig) Validate() error {
	if g.Station == "" || g.EVSE == "" {
		return fmt.Errorf("graph station and evse labels are required")
	}
	if len(g.Connectors) == 0 {
		return fmt.Errorf("graph needs at least one connector")
	}
	labels, slots := map[string]bool{}, map[uint64]bool{}
	for _, c := range g.Connectors {
		n, err := strconv.ParseUint(c.Slot, 10, 64)
		switch {
		case c.Label == "":
			return fmt.Errorf("graph connector label is required")
		case err != nil:
			return fmt.Errorf("graph connector slot %q is not a number", c.Slot)
		case labels[c.Label]:
			return fmt.Errorf("graph connector label %q is repeated", c.Label)
		case slots[n]:
			return fmt.Errorf("graph connector slot %q is repeated", c.Slot)
		}
		labels[c.Label], slots[n] = true, true
	}
	return nil
}

// Plan derives the graph. Connectors are ordered by the numeric value of their
// configured slot, so a host ordering them by slot keeps its output order; no
// dimension value is taken from an identifier, an index, a role string or an
// address.
func (g GraphConfig) Plan() GraphPlan {
	plan := GraphPlan{Resources: []PlannedResource{{Kind: KindStation, Label: g.Station}, {Kind: KindEVSE, Label: g.EVSE}}}
	plan.Relations = append(plan.Relations, PlannedRelation{RelationAssetEVSE, g.Station, g.EVSE, BasisConfiguration, KindStation, KindEVSE})
	connectors := append([]GraphConnector(nil), g.Connectors...)
	sort.SliceStable(connectors, func(i, j int) bool { return slotLess(connectors[i].Slot, connectors[j].Slot) })
	for _, c := range connectors {
		plan.Resources = append(plan.Resources, PlannedResource{Kind: KindConn, Label: c.Label, Slot: c.Slot})
		plan.Relations = append(plan.Relations, PlannedRelation{RelationEVSEConnector, g.EVSE, c.Label, BasisConfiguration, KindEVSE, KindConn})
	}
	if g.Meter != "" {
		plan.Resources = append(plan.Resources, PlannedResource{Kind: KindMeter, Label: g.Meter})
		plan.Relations = append(plan.Relations, PlannedRelation{RelationEVSEMeter, g.EVSE, g.Meter, BasisConfiguration, KindEVSE, KindMeter})
	}
	plan.Unknown = []UnknownRelation{
		{RelationConnectorPhase, UnknownReasonPhase},
		{RelationConnectorSession, UnknownReasonSession},
	}
	return plan
}

// The reasons a relation stays unknown: the specification does not state it.
const (
	UnknownReasonPhase   = "the specification states no phase component of the connector"
	UnknownReasonSession = "the specification states no start or end of a charging session"
)

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
	const vitalsReason = "typed vitals are served in the native record; the members without an admitted mapping stay native"
	return []NativeOnly{
		{tesla.FC100OperationCommonSystemInfo, "terminal body", reason},
		{tesla.FC100OperationWCGetVitals, "terminal body", vitalsReason},
		{tesla.FC100OperationWCGetLifetimeStats, "terminal body", reason},
		{tesla.FC100OperationWCGetSystemInfo, "terminal body", reason},
		{tesla.FC100OperationWCGetLoadSharingState, "terminal body", reason},
	}
}
