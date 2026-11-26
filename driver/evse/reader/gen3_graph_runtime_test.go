package reader

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"testing"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

func graphPublication() ChargerPublication {
	pub := syntheticPublication()
	pub.Graph = GraphConfig{Station: "station-a", EVSE: pub.EVSE, Meter: "meter-a",
		Connectors: []GraphConnector{{pub.Connector, "1"}, {"connector-b", "2"}}}
	return pub
}

func graphBatch(t *testing.T, pub ChargerPublication) (*ChargerReader, semreg.PublicationBatch) {
	t.Helper()
	reader, session := firstBatch(t, sixPollConfig(), pub, nil)
	return reader, session.committed()[0]
}

func kindCount(batch semreg.PublicationBatch) map[string]int {
	counts := map[string]int{}
	for _, r := range batch.ResourceUpserts {
		counts[string(r.Kind)]++
	}
	return counts
}

func relationCount(batch semreg.PublicationBatch) map[string]int {
	counts := map[string]int{}
	for _, r := range batch.RelationUpserts {
		counts[string(r.Definition.ID)]++
	}
	return counts
}

func TestRunningReaderBatchCarriesResourcesAndRelationsOfTheGraph(t *testing.T) {
	_, batch := graphBatch(t, graphPublication())
	counts := kindCount(batch)
	for kind, want := range map[string]int{KindStation: 1, KindEVSE: 1, KindConn: 2, KindMeter: 1} {
		if counts[kind] != want {
			t.Fatalf("%s upserts %d, want %d (%v)", kind, counts[kind], want, counts)
		}
	}
	if len(batch.ResourceUpserts) != 5 || counts[KindPhase] != 0 || counts[KindSession] != 0 {
		t.Fatalf("resources %v", counts)
	}
	relations := relationCount(batch)
	if relations[RelationAssetEVSE] != 1 || relations[RelationEVSEConnector] != 2 || relations[RelationEVSEMeter] != 1 || len(batch.RelationUpserts) != 4 {
		t.Fatalf("relations %v", relations)
	}
	kinds := map[semreg.ResourceID]string{}
	for _, r := range batch.ResourceUpserts {
		if err := r.Validate(); err != nil {
			t.Fatal(err)
		}
		kinds[r.ResourceID] = string(r.Kind)
	}
	ends := map[string][2]string{RelationAssetEVSE: {KindStation, KindEVSE}, RelationEVSEConnector: {KindEVSE, KindConn}, RelationEVSEMeter: {KindEVSE, KindMeter}}
	for _, r := range batch.RelationUpserts {
		want := ends[string(r.Definition.ID)]
		if kinds[r.From] != want[0] || kinds[r.To] != want[1] || r.BasisClass != semreg.RelationConfiguration || r.State != semreg.LinkQualified ||
			r.Origin.Kind != semreg.OriginOperator {
			t.Fatalf("relation %+v", r)
		}
	}
	if err := batch.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(batch.AbsenceUpserts) != 0 || len(batch.RelationWithdrawals) != 0 {
		t.Fatalf("absence or withdrawal staged: %+v", batch)
	}
}

func TestConnectorAnchorsFollowTheConfiguredParent(t *testing.T) {
	_, batch := graphBatch(t, graphPublication())
	byID := map[semreg.ResourceID]semreg.Resource{}
	for _, r := range batch.ResourceUpserts {
		byID[r.ResourceID] = r
	}
	for _, r := range batch.ResourceUpserts {
		switch string(r.Kind) {
		case KindStation, KindMeter:
			if r.Anchor.Class != semreg.AnchorEquipment || r.Anchor.Parent != nil {
				t.Fatalf("%+v", r.Anchor)
			}
		case KindEVSE:
			if r.Anchor.Parent == nil || byID[*r.Anchor.Parent].Kind != KindStation {
				t.Fatalf("the EVSE is not under the station: %+v", r.Anchor)
			}
		case KindConn:
			if r.Anchor.Parent == nil || byID[*r.Anchor.Parent].Kind != KindEVSE || r.Anchor.Slot == nil || (*r.Anchor.Slot != "1" && *r.Anchor.Slot != "2") {
				t.Fatalf("the connector is not under the EVSE at its configured slot: %+v", r.Anchor)
			}
		}
	}
}

func TestMeterIsPublishedOnlyWhenConfigured(t *testing.T) {
	pub := graphPublication()
	pub.Graph.Meter = ""
	_, batch := graphBatch(t, pub)
	if counts := kindCount(batch); counts[KindMeter] != 0 || len(batch.ResourceUpserts) != 4 {
		t.Fatalf("a meter resource was published without its configuration: %v", counts)
	}
	if relations := relationCount(batch); relations[RelationEVSEMeter] != 0 || len(batch.RelationUpserts) != 3 {
		t.Fatalf("a meter relation was published without its configuration: %v", relations)
	}
}

func TestConnectorSlotsAreOrderedByTheirNumericValue(t *testing.T) {
	pub := syntheticPublication()
	pub.Graph = GraphConfig{Station: "station-a", EVSE: pub.EVSE,
		Connectors: []GraphConnector{{"connector-two", "2"}, {pub.Connector, "10"}, {"connector-nine", "9"}}}
	reader, batch := graphBatch(t, pub)
	var order []string
	for _, r := range reader.Graph().Resources {
		if r.Kind == KindConn {
			order = append(order, r.Slot)
		}
	}
	if strings.Join(order, ",") != "2,9,10" {
		t.Fatalf("connector order %v", order)
	}
	var published []string
	for _, r := range batch.ResourceUpserts {
		if r.Kind == KindConn {
			published = append(published, *r.Anchor.Slot)
		}
	}
	sort.Slice(published, func(i, j int) bool {
		a, _ := strconv.Atoi(published[i])
		b, _ := strconv.Atoi(published[j])
		return a < b
	})
	if strings.Join(published, ",") != "2,9,10" {
		t.Fatalf("published slots %v", published)
	}
}

func TestSlotOrderIsNumericInThePlan(t *testing.T) {
	plan := GraphConfig{Station: "s", EVSE: "e", Connectors: []GraphConnector{{"a", "10"}, {"b", "2"}, {"c", "9"}, {"d", "x"}}}.Plan()
	var order []string
	for _, r := range plan.Resources {
		if r.Kind == KindConn {
			order = append(order, r.Slot)
		}
	}
	if strings.Join(order, ",") != "2,9,10,x" {
		t.Fatalf("order %v", order)
	}
}

func TestUnsupportedRelationsAreUnknownWithTheirReason(t *testing.T) {
	reader, batch := graphBatch(t, graphPublication())
	reasons := map[string]string{}
	for _, u := range reader.UnknownRelations() {
		reasons[u.Definition] = u.Reason
	}
	if len(reasons) != 2 || reasons[RelationConnectorPhase] != "the specification states no phase component of the connector" ||
		reasons[RelationConnectorSession] != "the specification states no start or end of a charging session" {
		t.Fatalf("reasons %v", reasons)
	}
	for definition := range reasons {
		if relationCount(batch)[definition] != 0 {
			t.Fatalf("%s was published as an instance", definition)
		}
	}
	if len(batch.AbsenceUpserts) != 0 {
		t.Fatalf("an absence statement was staged where the relation is unknown: %+v", batch.AbsenceUpserts)
	}
	for _, r := range batch.ResourceUpserts {
		if r.Kind == KindPhase || r.Kind == KindSession {
			t.Fatalf("%s resource published", r.Kind)
		}
	}
}

type graphRoles struct {
	resources map[string]semreg.ResourceID
	relations map[string]string
}

func rolesOf(t *testing.T, batch semreg.PublicationBatch) graphRoles {
	t.Helper()
	out := graphRoles{resources: map[string]semreg.ResourceID{}, relations: map[string]string{}}
	var connectors []semreg.Resource
	for _, r := range batch.ResourceUpserts {
		if r.Kind == KindConn {
			connectors = append(connectors, r)
			continue
		}
		out.resources[string(r.Kind)] = r.ResourceID
	}
	sort.Slice(connectors, func(i, j int) bool {
		a, _ := strconv.Atoi(*connectors[i].Anchor.Slot)
		b, _ := strconv.Atoi(*connectors[j].Anchor.Slot)
		return a < b
	})
	for i, r := range connectors {
		out.resources["connector"+strconv.Itoa(i)] = r.ResourceID
	}
	names := map[semreg.ResourceID]string{}
	for name, id := range out.resources {
		names[id] = name
	}
	for _, r := range batch.RelationUpserts {
		out.relations[string(r.Definition.ID)+"|"+names[r.From]+"|"+names[r.To]] = relationSortKey(r)
	}
	return out
}

func TestGraphIdentifiersOfTheRunningProducerFollowTheirIdentityFields(t *testing.T) {
	_, base := graphBatch(t, graphPublication())
	want := rolesOf(t, base)
	if len(want.resources) != 5 || len(want.relations) != 4 {
		t.Fatalf("roles %v %v", want.resources, want.relations)
	}
	reversed := func(p ChargerPublication) ChargerPublication {
		c := p.Graph.Connectors
		p.Graph.Connectors = []GraphConnector{c[1], c[0]}
		return p
	}
	cases := []struct {
		name    string
		mutate  func(*ChargerPublication)
		changed []string
	}{
		{"restart: epoch, generation, clock epoch and source", func(p *ChargerPublication) {
			p.Epoch, p.Generation, p.ClockEpoch, p.Source = "epoch:restarted", 8, "clock:restarted", "source:restarted"
		}, nil},
		{"reversed discovery order", func(p *ChargerPublication) { *p = reversed(*p) }, nil},
		{"connector label with the same slot", func(p *ChargerPublication) { p.Graph.Connectors[1].Label = "connector-renamed" }, nil},
		{"asset", func(p *ChargerPublication) { p.Asset = "asset:second-charger" },
			[]string{KindStation, KindEVSE, KindMeter, "connector0", "connector1"}},
		{"station label", func(p *ChargerPublication) { p.Graph.Station = "station-b" },
			[]string{KindStation, KindEVSE, "connector0", "connector1"}},
		{"evse label", func(p *ChargerPublication) { p.EVSE, p.Graph.EVSE = "evse-b", "evse-b" },
			[]string{KindEVSE, "connector0", "connector1"}},
		{"connector slot", func(p *ChargerPublication) { p.Graph.Connectors[1].Slot = "3" }, []string{"connector1"}},
		{"meter label", func(p *ChargerPublication) { p.Graph.Meter = "meter-b" }, []string{KindMeter}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pub := graphPublication()
			pub.Graph.Connectors = append([]GraphConnector(nil), pub.Graph.Connectors...)
			tc.mutate(&pub)
			_, batch := graphBatch(t, pub)
			got := rolesOf(t, batch)
			changed := map[string]bool{}
			for _, name := range tc.changed {
				changed[name] = true
			}
			for name, id := range want.resources {
				if (got.resources[name] != id) != changed[name] {
					t.Fatalf("resource %s: changed=%v, want %v", name, got.resources[name] != id, changed[name])
				}
			}
			for name, key := range want.relations {
				parts := strings.Split(name, "|")
				wantChange := changed[parts[1]] || changed[parts[2]]
				if (got.relations[name] != key) != wantChange {
					t.Fatalf("relation %s: changed=%v, want %v", name, got.relations[name] != key, wantChange)
				}
			}
		})
	}
}

func TestResourceIdentifiersAreStableAcrossTwoRuns(t *testing.T) {
	_, first := graphBatch(t, graphPublication())
	_, second := graphBatch(t, graphPublication())
	if !bytes.Equal(mustJSON(t, first.ResourceUpserts), mustJSON(t, second.ResourceUpserts)) ||
		!bytes.Equal(mustJSON(t, first.RelationUpserts), mustJSON(t, second.RelationUpserts)) {
		t.Fatal("two runs of one configuration published different records")
	}
}

func TestNothingPendingLeavesTheBatchWithoutGraphMembers(t *testing.T) {
	zero := syntheticPublication()
	_, plain := graphBatch(t, zero)
	text := string(mustJSON(t, plain))
	if strings.Contains(text, "resource_upserts") || strings.Contains(text, "relation_upserts") {
		t.Fatal("a batch of a reader without a graph carries an empty upsert list")
	}
	_, withGraph := graphBatch(t, graphPublication())
	stripped := withGraph
	stripped.ResourceUpserts, stripped.RelationUpserts, stripped.BatchDigest = nil, nil, ""
	baseline := plain
	baseline.BatchDigest = ""
	if !bytes.Equal(mustJSON(t, stripped), mustJSON(t, baseline)) {
		t.Fatal("the graph changed more than its own members")
	}

	pub := graphPublication()
	var withdraw bool
	reader, session := firstBatch(t, sixPollConfig(), pub, func([]semreg.NativeBinding) []semreg.IdentityLink {
		if withdraw {
			return []semreg.IdentityLink{operatorLink(pub, semreg.LinkWithdrawn, "4")}
		}
		return nil
	})
	before := len(session.committed())
	if err := reader.refresh(context.Background()); err != nil || len(session.committed()) != before {
		t.Fatalf("an unchanged state published a batch: %v", err)
	}
	withdraw = true
	if err := reader.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	committed := session.committed()
	later := committed[len(committed)-1]
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(mustJSON(t, later), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"resource_upserts", "relation_upserts", "relation_withdrawals", "absence_upserts", "absence_withdrawals"} {
		if _, present := raw[key]; present {
			t.Fatalf("a later batch carries %s", key)
		}
	}
}

func TestGraphThatNamesAnotherEVSEOrConnectorIsRefused(t *testing.T) {
	for name, mutate := range map[string]func(*ChargerPublication){
		"evse":            func(p *ChargerPublication) { p.Graph.EVSE = "evse-other" },
		"connector":       func(p *ChargerPublication) { p.Graph.Connectors[0].Label = "connector-other" },
		"slot not number": func(p *ChargerPublication) { p.Graph.Connectors[0].Slot = "left" },
		"slot repeated":   func(p *ChargerPublication) { p.Graph.Connectors[1].Slot = "01" },
		"label repeated":  func(p *ChargerPublication) { p.Graph.Connectors[1].Label = p.Graph.Connectors[0].Label },
		"no station":      func(p *ChargerPublication) { p.Graph.Station = "" },
		"no connector":    func(p *ChargerPublication) { p.Graph.Connectors = nil },
	} {
		t.Run(name, func(t *testing.T) {
			pub := graphPublication()
			pub.Graph.Connectors = append([]GraphConnector(nil), pub.Graph.Connectors...)
			mutate(&pub)
			c := newCharger(t)
			_, err := NewChargerReader(sixPollConfig(), pub, c.deps)
			if ce, ok := err.(*ConfigError); !ok || ce.Field != "publication.graph" {
				t.Fatalf("err %v", err)
			}
			if c.opens != 0 {
				t.Fatal("a refused graph opened the endpoint")
			}
		})
	}
}
