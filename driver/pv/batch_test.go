package pv

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/huawei"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

type fakeTx struct {
	calls     []string
	pending   map[semreg.NativeBindingID]semreg.IdentityLink
	committed map[semreg.NativeBindingID]bool
	asked     []semreg.NativeBinding
	claims    []driver.ManufacturerSerialClaim
	links     []driver.LinkClaim
	staged    []semreg.PublicationBatch
}

// OperatorLinks answers the pending set of the transaction's scope for the
// bindings of the batch and for the bindings already committed.
func (f *fakeTx) OperatorLinks(bindings []semreg.NativeBinding) []semreg.IdentityLink {
	f.calls = append(f.calls, "operator_links")
	f.asked = bindings
	in := map[semreg.NativeBindingID]bool{}
	for _, b := range bindings {
		in[b.BindingID] = true
	}
	var out []semreg.IdentityLink
	for id, l := range f.pending {
		if in[id] || f.committed[id] {
			out = append(out, l)
		}
	}
	sortLinks(out)
	return out
}

func sortLinks(l []semreg.IdentityLink) {
	for i := 1; i < len(l); i++ {
		for j := i; j > 0 && l[j].BindingID < l[j-1].BindingID; j-- {
			l[j], l[j-1] = l[j-1], l[j]
		}
	}
}

func (f *fakeTx) AdmitManufacturerSerial(c driver.ManufacturerSerialClaim) error {
	f.calls = append(f.calls, "claim")
	f.claims = append(f.claims, c)
	return c.Validate()
}
func (f *fakeTx) Stage(b semreg.PublicationBatch) error {
	f.calls = append(f.calls, "stage")
	f.staged = append(f.staged, b)
	return nil
}
func (f *fakeTx) DeclareSource(driver.Source) error             { return nil }
func (f *fakeTx) DeclareSettingRoute(driver.SettingRoute) error { return nil }
func (f *fakeTx) StageSample(driver.Sample) error               { return nil }
func (f *fakeTx) StageLink(l driver.LinkClaim) error {
	f.calls = append(f.calls, "link")
	f.links = append(f.links, l)
	return l.Validate()
}
func (f *fakeTx) Commit(context.Context) error { f.calls = append(f.calls, "commit"); return nil }
func (f *fakeTx) Abort(context.Context) error  { f.calls = append(f.calls, "abort"); return nil }

func evidenceFor(tag string) []semreg.EvidenceRef {
	return []semreg.EvidenceRef{evidence("evidence.configuration", tag)}
}

func link(asset, binding string, state semreg.LinkState, tag string) semreg.IdentityLink {
	return semreg.IdentityLink{AssetID: semreg.AssetID(asset), BindingID: semreg.NativeBindingID(binding), State: state, Basis: evidenceFor(tag), Revision: "1"}
}

func binding(asset, id string) semreg.NativeBinding {
	return semreg.NativeBinding{BindingID: semreg.NativeBindingID(id), AssetID: semreg.AssetID(asset), State: semreg.BindingCurrent, Revision: "1"}
}

func ownerBatch() semreg.PublicationBatch {
	return semreg.PublicationBatch{
		Contract: semreg.ContractKernelV1, BatchID: "batch.synthetic", AssetID: "asset.a", Sequence: "3",
		BindingUpserts:      []semreg.NativeBinding{binding("asset.a", "binding.a"), binding("asset.a", "binding.b")},
		IdentityLinkUpserts: []semreg.IdentityLink{link("asset.a", "binding.a", semreg.LinkQualified, "own-a"), link("asset.a", "binding.z", semreg.LinkQualified, "own-z")},
		FactUpserts:         []semreg.FactCandidate{}, FactWithdrawals: []semreg.CandidateID{},
	}
}

func canonical(t *testing.T, b semreg.PublicationBatch) string {
	t.Helper()
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func linkByBinding(b semreg.PublicationBatch) map[semreg.NativeBindingID]semreg.IdentityLink {
	out := map[semreg.NativeBindingID]semreg.IdentityLink{}
	for _, l := range b.IdentityLinkUpserts {
		out[l.BindingID] = l
	}
	return out
}

func TestThePendingOperatorLinkIsCarriedOnTheOwnerBatch(t *testing.T) {
	p := testPublisher(t, testConfig())
	pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, commonBlock("Maker", "Model", "SN-1"), inverterBlock(t, 103, integerValues(103, nil)))}}})
	tx := &fakeTx{
		pending: map[semreg.NativeBindingID]semreg.IdentityLink{
			"binding.a": link("asset.a", "binding.a", semreg.LinkCandidate, "operator-a"), // replaces the batch's own link of binding a
			"binding.c": link("asset.a", "binding.c", semreg.LinkCandidate, "operator-c"), // a committed binding, not in the batch
			"binding.d": link("asset.d", "binding.d", semreg.LinkCandidate, "operator-d"), // a binding this transaction does not carry
		},
		committed: map[semreg.NativeBindingID]bool{"binding.c": true},
	}
	batch := ownerBatch()
	if err := p.StageSunSpec(context.Background(), tx, batch, pub); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tx.asked, batch.BindingUpserts) {
		t.Fatal("the pending links were asked for bindings other than the batch's upserts")
	}
	got := linkByBinding(tx.staged[0])
	if len(got) != 3 || len(tx.staged[0].IdentityLinkUpserts) != 3 {
		t.Fatalf("links %v", got)
	}
	if l := got["binding.a"]; l.State != semreg.LinkCandidate || !reflect.DeepEqual(l.Basis, evidenceFor("operator-a")) {
		t.Fatalf("the link of the matched binding was not replaced: %+v", l)
	}
	if l := got["binding.z"]; l.State != semreg.LinkQualified || !reflect.DeepEqual(l.Basis, evidenceFor("own-z")) {
		t.Fatalf("another link of the batch was changed: %+v", l)
	}
	if _, ok := got["binding.c"]; !ok {
		t.Fatal("the pending link of a committed binding was not carried")
	}
	if _, ok := got["binding.d"]; ok {
		t.Fatal("a pending link of a binding the transaction does not carry was staged")
	}
	// it stays pending for the batch that does carry it
	if _, ok := tx.pending["binding.d"]; !ok {
		t.Fatal("the pending set was consumed")
	}
	// the claim, the relations and the batch go in one transaction, in order
	want := []string{"claim"}
	for range pub.Links {
		want = append(want, "link")
	}
	want = append(want, "operator_links", "stage")
	if !reflect.DeepEqual(tx.calls, want) {
		t.Fatalf("calls %v, want %v", tx.calls, want)
	}
	if len(tx.claims) != 1 || len(tx.links) != len(pub.Links) {
		t.Fatalf("claims %d links %d", len(tx.claims), len(tx.links))
	}
}

func TestAnOperatorWithdrawalGoesThroughTheSameBatch(t *testing.T) {
	p := testPublisher(t, testConfig())
	tx := &fakeTx{pending: map[semreg.NativeBindingID]semreg.IdentityLink{"binding.b": link("asset.a", "binding.b", semreg.LinkWithdrawn, "withdrawn")}}
	if err := p.StageSunSpec(context.Background(), tx, ownerBatch(), Publication{}); err != nil {
		t.Fatal(err)
	}
	if l := linkByBinding(tx.staged[0])["binding.b"]; l.State != semreg.LinkWithdrawn {
		t.Fatalf("withdrawal not carried: %+v", l)
	}
}

func TestAnEmptyPendingSetLeavesTheBatchByteForByte(t *testing.T) {
	p := testPublisher(t, testConfig())
	batch := ownerBatch()
	before := canonical(t, batch)
	tx := &fakeTx{}
	if err := p.StageSunSpec(context.Background(), tx, batch, Publication{}); err != nil {
		t.Fatal(err)
	}
	if got := canonical(t, tx.staged[0]); got != before {
		t.Fatalf("the batch changed:\n%s\n%s", before, got)
	}
}

func TestTheDongleFamilyLoggerBatchIsUnchanged(t *testing.T) {
	p := testPublisher(t, testConfig())
	child := dongleChild(t, 1, map[uint16][]uint16{32089: {0x0200}})
	pub := publish(t, testConfig(), Input{Huawei: []huawei.ProviderCycle{dongleCycle(huawei.ProviderPathSmartLogger, child)}})
	batch := ownerBatch()
	before := canonical(t, batch)
	tx := &fakeTx{pending: map[semreg.NativeBindingID]semreg.IdentityLink{"binding.a": link("asset.a", "binding.a", semreg.LinkCandidate, "operator-a")}}
	if err := p.StageDongle(context.Background(), tx, batch, pub); err != nil {
		t.Fatal(err)
	}
	if canonical(t, tx.staged[0]) != before {
		t.Fatal("the logger batch carried a pending link or changed")
	}
	for _, c := range tx.calls {
		if c == "operator_links" || c == "claim" {
			t.Fatalf("the logger batch called %s", c)
		}
	}
	if len(tx.links) != len(pub.DongleLinks) || len(pub.DongleLinks) == 0 {
		t.Fatalf("%d links staged of %d", len(tx.links), len(pub.DongleLinks))
	}
}
