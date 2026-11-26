package reader

import (
	"testing"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

type routeIdentifiers struct {
	binding    semreg.NativeBindingID
	service    semreg.ServiceInstanceID
	capability semreg.CapabilityInstanceID
	candidate  semreg.CandidateID
}

func identifiersOf(p ChargerPublication) routeIdentifiers {
	return routeIdentifiers{
		binding:    p.BindingIdentifier(),
		service:    p.ServiceInstanceIdentifier(SetAllocated),
		capability: p.CapabilityInstanceIdentifier(SetAllocated),
		candidate:  p.ConnectorCandidateIdentifier(ElementAllocatedCurrent),
	}
}

// The identifiers are the ones the provider publishes in its first batch.
func TestRouteIdentifiersAreTheOnesTheProviderPublishes(t *testing.T) {
	pub := syntheticPublication()
	cfg := sixPollConfig()
	_, session := firstBatch(t, cfg, pub, nil)
	batch := session.committed()[0]
	if got, want := pub.BindingIdentifier(), batch.BindingUpserts[0].BindingID; got != want {
		t.Fatalf("binding %s, published %s", got, want)
	}
	connector := batch.ServiceUpserts[0]
	for _, s := range batch.ServiceUpserts {
		if s.Definition.ID == ReadConnector.Service {
			connector = s
		}
	}
	if got := pub.ServiceInstanceIdentifier(ReadConnector); got != connector.InstanceID {
		t.Fatalf("service %s, published %s", got, connector.InstanceID)
	}
	if got, want := pub.ServiceInstanceIdentifier(SetAllocated), connector.InstanceID; got != want {
		t.Fatalf("the setter capability binds to the connector service: %s, published %s", got, want)
	}
	for _, c := range batch.CapabilityUpserts {
		if c.Definition.ID == ReadConnector.Capability {
			if got := pub.CapabilityInstanceIdentifier(ReadConnector); got != c.InstanceID {
				t.Fatalf("capability %s, published %s", got, c.InstanceID)
			}
		}
	}
	var candidate semreg.CandidateID
	for _, f := range batch.FactUpserts {
		if string(f.Key.FactID) == ElementAllocatedCurrent {
			candidate = f.CandidateID
		}
	}
	if candidate == "" {
		t.Fatal("the first batch carries no allocated current")
	}
	if got := pub.ConnectorCandidateIdentifier(ElementAllocatedCurrent); got != candidate {
		t.Fatalf("candidate %s, published %s", got, candidate)
	}
}

// Every field of the publication is varied in both incarnations; the identifier
// that takes the field into its material changes, and one that does not stays.
// Each row names the production mutation (dropping the field from the
// identifier's material) that the row catches.
func TestRouteIdentifiersFollowEachFieldOfTheirMaterial(t *testing.T) {
	type vary struct {
		name   string
		mutate func(*ChargerPublication)
		// changes[mode] lists the identifiers that must differ in that mode.
		first, successor [4]bool // binding, service, capability, candidate
	}
	rows := []vary{
		{"asset", func(p *ChargerPublication) { p.Asset = "asset:other" }, [4]bool{true, true, true, true}, [4]bool{true, true, true, true}},
		{"source", func(p *ChargerPublication) { p.Source = "source:other" }, [4]bool{true, false, false, true}, [4]bool{true, true, true, true}},
		{"epoch", func(p *ChargerPublication) { p.Epoch = "epoch:other" }, [4]bool{false, false, false, false}, [4]bool{true, true, true, true}},
		{"generation", func(p *ChargerPublication) { p.Generation++ }, [4]bool{false, false, false, false}, [4]bool{true, true, true, true}},
		{"connector", func(p *ChargerPublication) { p.Connector = "connector-other" }, [4]bool{false, false, false, false}, [4]bool{false, false, false, true}},
		{"evse", func(p *ChargerPublication) { p.EVSE = "evse-other" }, [4]bool{false, false, false, false}, [4]bool{false, false, false, false}},
	}
	for _, incarnation := range []struct {
		name  string
		first bool
	}{{"first", true}, {"successor", false}} {
		for _, row := range rows {
			t.Run(incarnation.name+"/"+row.name, func(t *testing.T) {
				base := syntheticPublication()
				base.FirstIncarnation = incarnation.first
				varied := base
				row.mutate(&varied)
				a, b := identifiersOf(base), identifiersOf(varied)
				want := row.successor
				if incarnation.first {
					want = row.first
				}
				got := [4]bool{a.binding != b.binding, a.service != b.service, a.capability != b.capability, a.candidate != b.candidate}
				if got != want {
					t.Fatalf("changed [binding service capability candidate] = %v, want %v", got, want)
				}
			})
		}
	}
	t.Run("incarnation", func(t *testing.T) {
		base := syntheticPublication()
		varied := base
		varied.FirstIncarnation = true
		a, b := identifiersOf(base), identifiersOf(varied)
		if a.binding == b.binding || a.service == b.service || a.capability == b.capability || a.candidate == b.candidate {
			t.Fatalf("the incarnation does not enter every identifier: %+v %+v", a, b)
		}
	})
	t.Run("definition and element", func(t *testing.T) {
		p := syntheticPublication()
		if p.CapabilityInstanceIdentifier(SetAllocated) == p.CapabilityInstanceIdentifier(ReadConnector) {
			t.Fatal("two capabilities share an identifier")
		}
		if p.ServiceInstanceIdentifier(ReadConnector) == p.ServiceInstanceIdentifier(ReadEVSE) {
			t.Fatal("two services share an identifier")
		}
		if p.ConnectorCandidateIdentifier(ElementAllocatedCurrent) == p.ConnectorCandidateIdentifier(ElementAllocatedExpiry) {
			t.Fatal("two elements share a candidate identifier")
		}
	})
}
