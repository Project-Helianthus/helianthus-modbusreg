package reader

import "testing"

func TestGraphPlanStatesConfiguredRelationsAndLeavesOthersUnknown(t *testing.T) {
	plan := GraphConfig{Station: "station-a", EVSE: "evse-a", Meter: "meter-a",
		Connectors: []GraphConnector{{"connector-b", "2"}, {"connector-a", "1"}}}.Plan()
	var order []string
	kinds := map[string]bool{}
	for _, r := range plan.Resources {
		kinds[r.Kind] = true
		if r.Kind == KindConn {
			order = append(order, r.Slot+":"+r.Label)
		}
	}
	if len(order) != 2 || order[0] != "1:connector-a" || order[1] != "2:connector-b" {
		t.Fatalf("connector order %v", order)
	}
	for _, kind := range []string{KindStation, KindEVSE, KindConn, KindMeter} {
		if !kinds[kind] {
			t.Fatalf("%s missing", kind)
		}
	}
	if kinds[KindPhase] || kinds[KindSession] {
		t.Fatal("a phase or session resource was published without a public statement")
	}
	defs := map[string]int{}
	for _, r := range plan.Relations {
		defs[r.Definition]++
		if r.Basis != BasisConfiguration {
			t.Fatalf("%+v", r)
		}
	}
	if defs[RelationAssetEVSE] != 1 || defs[RelationEVSEConnector] != 2 || defs[RelationEVSEMeter] != 1 {
		t.Fatalf("relations %v", defs)
	}
	if len(plan.Unknown) != 2 || plan.Unknown[0].Definition != RelationConnectorPhase || plan.Unknown[1].Definition != RelationConnectorSession || plan.Unknown[0].Reason == "" {
		t.Fatalf("unknown %+v", plan.Unknown)
	}
	if without := (GraphConfig{Station: "s", EVSE: "e", Connectors: []GraphConnector{{"c", "1"}}}).Plan(); len(without.Relations) != 2 {
		t.Fatalf("a meter relation was stated without a configured meter: %+v", without.Relations)
	}
}

func TestEveryObservationIsNativeOnlyByOperationAndField(t *testing.T) {
	list := NativeOnlyObservations()
	if len(list) != 5 {
		t.Fatalf("%d operations", len(list))
	}
	seen := map[string]bool{}
	for _, n := range list {
		if n.Operation == "" || n.Field == "" || n.Reason == "" || seen[string(n.Operation)] {
			t.Fatalf("%+v", n)
		}
		seen[string(n.Operation)] = true
	}
}
