package storage

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

func relationsOf(k *kernel, def semreg.DefinitionID) []semreg.RelationInstance {
	var out []semreg.RelationInstance
	for _, r := range k.relations {
		if r.Definition.ID == def {
			out = append(out, r)
		}
	}
	return out
}

func claimsOf(k *kernel, def semreg.DefinitionID) []driver.LinkClaim {
	var out []driver.LinkClaim
	for _, c := range k.claims {
		if c.Key.Type.ID == def {
			out = append(out, c)
		}
	}
	return out
}

func TestRelationsAreRelationsAndClaimsOfOneTransaction(t *testing.T) {
	f := loadFixture(t)
	s := f.session()
	p := newPublisher(t, f.config(), s, f.packAllocator(t))
	if err := p.PublishGrowatt(context.Background(), f.observe(t, 2, 1, false)); err != nil {
		t.Fatal(err)
	}
	k := s.k
	pack := semreg.ResourceID(f.Configuration.Asset)
	sb, bp := relationsOf(k, relSystemBattery), relationsOf(k, relBatteryPack)
	if len(sb) != 1 || len(bp) != 1 || len(relationsOf(k, relPackInterface)) != 0 {
		t.Fatalf("relations: %d system_battery %d battery_pack %d pack_interface", len(sb), len(bp), len(relationsOf(k, relPackInterface)))
	}
	if sb[0].BasisClass != semreg.RelationConfiguration || sb[0].Origin.Kind != semreg.OriginOperator {
		t.Errorf("system_battery basis %s origin %s", sb[0].BasisClass, sb[0].Origin.Kind)
	}
	if bp[0].BasisClass != semreg.RelationGateRule || bp[0].Origin.Kind != semreg.OriginNativeObservation || bp[0].To != pack || bp[0].From != sb[0].To {
		t.Errorf("battery_pack: %+v", bp[0])
	}
	if k.resources[sb[0].From].Kind != kindSystem || k.resources[sb[0].To].Kind != kindBattery || k.resources[pack].Kind != kindPack {
		t.Errorf("endpoint kinds: %s %s %s", k.resources[sb[0].From].Kind, k.resources[sb[0].To].Kind, k.resources[pack].Kind)
	}
	if len(k.claims) != 2 {
		t.Fatalf("claims %d", len(k.claims))
	}
	for _, r := range k.relations {
		var found bool
		for _, c := range k.claims {
			found = found || (c.Key.Type == r.Definition && c.Key.From.ID == r.From && c.Key.To.ID == r.To && c.Key.From.Bridge == "bridge.test")
		}
		if !found {
			t.Errorf("relation %s has no claim", r.Definition.ID)
		}
	}
	// A refresh stages neither the relations nor the claims again.
	if err := p.PublishGrowatt(context.Background(), f.observe(t, 2, 2, false)); err != nil {
		t.Fatal(err)
	}
	if len(k.relations) != 2 || len(k.claims) != 2 {
		t.Errorf("refresh repeated relations %d claims %d", len(k.relations), len(k.claims))
	}
}

func TestRelationsWithoutConfiguredEndsStayUnknown(t *testing.T) {
	f := loadFixture(t)
	cfg := f.config()
	cfg.Battery, cfg.System = "", ""
	s := f.session()
	p := newPublisher(t, cfg, s, f.packAllocator(t))
	if err := p.PublishGrowatt(context.Background(), f.observe(t, 2, 1, false)); err != nil {
		t.Fatal(err)
	}
	if len(s.k.relations) != 0 || len(s.k.claims) != 0 {
		t.Fatalf("relations %d claims %d with nothing configured", len(s.k.relations), len(s.k.claims))
	}
	cfg.Battery = "battery-a"
	second := f.session()
	q := newPublisher(t, cfg, second, f.packAllocator(t))
	if err := q.PublishGrowatt(context.Background(), f.observe(t, 2, 1, false)); err != nil {
		t.Fatal(err)
	}
	if len(relationsOf(second.k, relSystemBattery)) != 0 || len(relationsOf(second.k, relBatteryPack)) != 1 {
		t.Fatal("a battery without a configured system was grouped")
	}
}

func TestRejectedCommitKeepsNeitherRelationsNorClaims(t *testing.T) {
	f := loadFixture(t)
	s := f.session()
	s.refuse = errors.New("refused")
	p := newPublisher(t, f.config(), s, f.packAllocator(t))
	if err := p.PublishGrowatt(context.Background(), f.observe(t, 2, 1, false)); err == nil {
		t.Fatal("the refused commit succeeded")
	}
	k := s.k
	if len(k.relations) != 0 || len(k.claims) != 0 || len(k.facts) != 0 || len(k.resources) != 0 || s.aborted != 1 {
		t.Fatalf("relations %d claims %d facts %d resources %d aborted %d", len(k.relations), len(k.claims), len(k.facts), len(k.resources), s.aborted)
	}
	// The producer's memory did not advance: the next accepted commit carries the whole first batch.
	s.refuse = nil
	if err := p.PublishGrowatt(context.Background(), f.observe(t, 2, 2, false)); err != nil {
		t.Fatal(err)
	}
	if len(k.relations) != 2 || len(k.claims) != 2 || len(k.batches[0].BindingUpserts) != 1 || len(k.batches[0].SourceUpserts) != 1 {
		t.Fatalf("retry: relations %d claims %d", len(k.relations), len(k.claims))
	}
}

func TestSuccessorGenerationHasItsOwnBindingAndServiceAndKeepsTheResource(t *testing.T) {
	f, frozen := loadFixture(t), loadIDs(t)
	s := f.session()
	a := f.packAllocator(t)
	p := newPublisher(t, f.config(), s, a)
	if err := p.PublishGrowatt(context.Background(), f.observe(t, 2, 1, false)); err != nil {
		t.Fatal(err)
	}
	next := s.successor("source-epoch-2", 2)
	cfg := f.config()
	cfg.First = false
	q := newPublisher(t, cfg, next, a)
	if err := q.PublishGrowatt(context.Background(), f.observe(t, 2, 2, false)); err != nil {
		t.Fatal(err)
	}
	if len(next.k.bindings) != 2 || len(next.k.services) != 2 {
		t.Fatalf("bindings %d services %d", len(next.k.bindings), len(next.k.services))
	}
	var succeeding semreg.NativeBindingID
	for id := range next.k.bindings {
		if id != semreg.NativeBindingID(frozen.Binding) {
			succeeding = id
		}
	}
	for _, sv := range next.k.services {
		if sv.BindingID == succeeding && sv.DriverGeneration != "2" {
			t.Errorf("successor service generation %s", sv.DriverGeneration)
		}
	}
	// The pack keeps its identifier across the generation: the same node key.
	last := next.k.batches[len(next.k.batches)-1]
	for _, c := range last.FactUpserts {
		if *c.Key.Dimensions[0].Value.Text != frozen.PackDimensionText {
			t.Errorf("%s: the successor named another pack %s", c.Key.FactID, *c.Key.Dimensions[0].Value.Text)
		}
		if c.BindingID == nil || *c.BindingID != succeeding {
			t.Errorf("%s: binding %v", c.Key.FactID, c.BindingID)
		}
	}
}

// Pending operator identity links on the storage batch.

func pendingLink(binding semreg.NativeBindingID, asset semreg.AssetID, resource semreg.ResourceID, state semreg.LinkState) semreg.IdentityLink {
	return semreg.IdentityLink{AssetID: asset, BindingID: binding, State: state, Basis: []semreg.EvidenceRef{evidence("evidence.configuration", "operator-selected", string(binding))},
		Revision: "7", Resource: &resource}
}

func TestPendingOperatorLinkReplacesTheLinkOfItsBindingOnly(t *testing.T) {
	f, frozen := loadFixture(t), loadIDs(t)
	own := semreg.NativeBindingID(frozen.Binding)
	selected := semreg.ResourceID("resource:operator-selected")
	other := semreg.NativeBindingID("binding:other:00000000000000000000000000000000")

	s := f.session()
	s.pending = []semreg.IdentityLink{pendingLink(own, s.asset, selected, semreg.LinkQualified), pendingLink(other, s.asset, "resource:elsewhere", semreg.LinkQualified)}
	p := newPublisher(t, f.config(), s, f.packAllocator(t))
	if err := p.PublishGrowatt(context.Background(), f.observe(t, 2, 1, false)); err != nil {
		t.Fatal(err)
	}
	links := s.k.batches[0].IdentityLinkUpserts
	if len(links) != 1 || links[0].BindingID != own || links[0].Resource == nil || *links[0].Resource != selected || links[0].Revision != "7" {
		t.Fatalf("carried links: %+v", links)
	}
	for _, l := range links {
		if l.BindingID == other {
			t.Fatal("the pending link of another binding was carried")
		}
	}
	// The batch that does not carry it leaves it pending, and the next batch for that binding carries it.
	second := f.session()
	secondPublisher := newPublisher(t, f.config(), second, f.packAllocator(t))
	if err := secondPublisher.PublishGrowatt(context.Background(), f.observe(t, 2, 1, false)); err != nil {
		t.Fatal(err)
	}
	if got := second.k.batches[0].IdentityLinkUpserts; len(got) != 1 || *got[0].Resource != semreg.ResourceID(f.Configuration.Asset) {
		t.Fatalf("built link: %+v", got)
	}
	second.pending = []semreg.IdentityLink{pendingLink(own, second.asset, selected, semreg.LinkQualified)}
	if err := secondPublisher.PublishGrowatt(context.Background(), f.observe(t, 2, 2, false)); err != nil {
		t.Fatal(err)
	}
	carried := second.k.batches[1].IdentityLinkUpserts
	if len(carried) != 1 || *carried[0].Resource != selected {
		t.Fatalf("a committed binding was not carried: %+v", carried)
	}
	// The pack record, relations and facts are intact beside the carried link.
	if len(s.k.relations) != 2 || len(s.k.facts) == 0 || len(s.k.batches[0].ResourceUpserts) != 3 {
		t.Fatalf("relations %d facts %d resources %d", len(s.k.relations), len(s.k.facts), len(s.k.batches[0].ResourceUpserts))
	}
}

func TestPendingOperatorWithdrawalGoesInTheSameBatch(t *testing.T) {
	f, frozen := loadFixture(t), loadIDs(t)
	own := semreg.NativeBindingID(frozen.Binding)
	s := f.session()
	p := newPublisher(t, f.config(), s, f.packAllocator(t))
	if err := p.PublishGrowatt(context.Background(), f.observe(t, 2, 1, false)); err != nil {
		t.Fatal(err)
	}
	s.pending = []semreg.IdentityLink{pendingLink(own, s.asset, "resource:operator-selected", semreg.LinkWithdrawn)}
	if err := p.PublishGrowatt(context.Background(), f.observe(t, 2, 2, false)); err != nil {
		t.Fatal(err)
	}
	b := s.k.batches[1]
	if len(b.IdentityLinkUpserts) != 1 || b.IdentityLinkUpserts[0].State != semreg.LinkWithdrawn || len(b.FactUpserts) == 0 {
		t.Fatalf("withdrawal batch: %+v", b.IdentityLinkUpserts)
	}
}

func TestEmptyPendingSetLeavesTheBatchByteForByte(t *testing.T) {
	f, frozen := loadFixture(t), loadIDs(t)
	run := func(pending []semreg.IdentityLink) []byte {
		s := f.session()
		s.pending = pending
		p := newPublisher(t, f.config(), s, f.packAllocator(t))
		if err := p.PublishGrowatt(context.Background(), f.observe(t, 2, 1, true)); err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(s.k.batches[0])
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	empty := run(nil)
	if string(run([]semreg.IdentityLink{})) != string(empty) {
		t.Fatal("an empty set differs from no set")
	}
	foreign := pendingLink("binding:other:00000000000000000000000000000000", "asset:growatt-bms-a", "resource:elsewhere", semreg.LinkQualified)
	if string(run([]semreg.IdentityLink{foreign})) != string(empty) {
		t.Fatal("a pending link of another binding changed the batch")
	}
	carried := run([]semreg.IdentityLink{pendingLink(semreg.NativeBindingID(frozen.Binding), "asset:growatt-bms-a", "resource:operator-selected", semreg.LinkQualified)})
	if string(carried) == string(empty) {
		t.Fatal("the pending link of its own binding was not carried")
	}
}
