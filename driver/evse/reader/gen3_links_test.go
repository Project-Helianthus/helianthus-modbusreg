package reader

import (
	"bytes"
	"context"
	modbus "github.com/Project-Helianthus/helianthus-modbus"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

func sixPollConfig() Gen3Config {
	cfg := syntheticConfig(append(append([]tesla.FC100Operation(nil), fiveOperations...), tesla.FC100OperationWCGetProvisional)...)
	cfg.ProvisionalReadbackQualified = true
	return cfg
}

func firstBatch(t *testing.T, cfg Gen3Config, pub Gen3Publication, links func([]semreg.NativeBinding) []semreg.IdentityLink) (*Gen3Reader, *fakeSession) {
	t.Helper()
	c := newCharger(t)
	c.deps.Now = func() time.Time { return time.Unix(61, 0) }
	open := c.deps.Open
	c.deps.Open = func(config modbus.RTUProductionConfig) (tesla.Gen3ReadEndpoint, error) {
		endpoint, err := open(config)
		pushPolls(c.endpoint, cfg.Operations, scalars{limit: u(16), timeout: u(600), inhibit: b(false), configured: u(32)})
		return endpoint, err
	}
	reader, err := NewGen3Reader(cfg, pub, c.deps)
	if err != nil {
		t.Fatal(err)
	}
	session := &fakeSession{links: links}
	if err := reader.Start(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Stop(context.Background()) })
	waitFor(t, "the first batch", func() bool { return len(session.committed()) == 1 })
	return reader, session
}

func operatorLink(pub Gen3Publication, state semreg.LinkState, revision semreg.Uint64) semreg.IdentityLink {
	resource := semreg.ResourceID("resource:operator-declared")
	return semreg.IdentityLink{AssetID: pub.Asset, BindingID: pub.bindingID(), State: state, Revision: revision, Resource: &resource,
		Basis: []semreg.EvidenceRef{pub.evidence("operator.declaration", "one")}}
}

// pendingOnce returns the links until a transaction has carried them, as the
// session does once they are committed.
func pendingOnce(links ...semreg.IdentityLink) func([]semreg.NativeBinding) []semreg.IdentityLink {
	var mu sync.Mutex
	done := false
	return func([]semreg.NativeBinding) []semreg.IdentityLink {
		mu.Lock()
		defer mu.Unlock()
		if done {
			return nil
		}
		done = true
		return links
	}
}

func TestOperatorLinkIsAppliedToTheBindingOfTheBatchAndNothingElse(t *testing.T) {
	pub := syntheticPublication()
	foreign := operatorLink(pub, semreg.LinkQualified, "2")
	foreign.BindingID = "binding:another-source"
	own := operatorLink(pub, semreg.LinkQualified, "2")
	cfg := sixPollConfig()
	_, with := firstBatch(t, cfg, pub, pendingOnce(foreign, own))
	batch := with.committed()[0]
	if len(batch.IdentityLinkUpserts) != 1 || batch.IdentityLinkUpserts[0].BindingID != pub.bindingID() || batch.IdentityLinkUpserts[0].Revision != "2" ||
		batch.IdentityLinkUpserts[0].State != semreg.LinkQualified {
		t.Fatalf("links %+v", batch.IdentityLinkUpserts)
	}
	calls := with.operatorLinkCalls()
	if len(calls[0]) != 1 || calls[0][0].BindingID != pub.bindingID() {
		t.Fatalf("OperatorLinks was handed %+v", calls[0])
	}
	if err := batch.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestNoPendingLinkLeavesTheBatchByteEqual(t *testing.T) {
	pub := syntheticPublication()
	_, plain := firstBatch(t, sixPollConfig(), pub, nil)
	_, none := firstBatch(t, sixPollConfig(), pub, func([]semreg.NativeBinding) []semreg.IdentityLink { return nil })
	if !bytes.Equal(mustJSON(t, plain.committed()[0]), mustJSON(t, none.committed()[0])) {
		t.Fatal("an empty OperatorLinks result changed the batch")
	}
	_, linked := firstBatch(t, sixPollConfig(), pub, pendingOnce(operatorLink(pub, semreg.LinkQualified, "2")))
	if bytes.Equal(mustJSON(t, plain.committed()[0]), mustJSON(t, linked.committed()[0])) {
		t.Fatal("a pending link did not change the batch")
	}
	for _, fact := range linked.committed()[0].FactUpserts {
		var found bool
		for _, other := range plain.committed()[0].FactUpserts {
			found = found || bytes.Equal(mustJSON(t, fact), mustJSON(t, other))
		}
		if !found {
			t.Fatalf("the family's own facts changed: %s", fact.Key.FactID)
		}
	}
}

func TestFinalDeclarationWithdrawalOnAnAlreadyCurrentBinding(t *testing.T) {
	pub := syntheticPublication()
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
	if len(committed) != before+1 {
		t.Fatalf("the withdrawal was not carried by the next owner batch")
	}
	batch := committed[len(committed)-1]
	if len(batch.BindingUpserts) != 0 || len(batch.IdentityLinkUpserts) != 1 || batch.IdentityLinkUpserts[0].State != semreg.LinkWithdrawn ||
		batch.IdentityLinkUpserts[0].Revision != "4" || len(batch.FactUpserts) != 0 || len(batch.FactWithdrawals) != 0 {
		t.Fatalf("batch %+v", batch)
	}
	calls := session.operatorLinkCalls()
	if last := calls[len(calls)-1]; len(last) != 0 {
		t.Fatalf("OperatorLinks was not called with the empty binding set: %+v", last)
	}
	if err := batch.Validate(); err != nil {
		t.Fatal(err)
	}
	withdraw = false
	count := len(session.committed())
	if err := reader.refresh(context.Background()); err != nil || len(session.committed()) != count {
		t.Fatalf("the withdrawal repeated: %v", err)
	}
}

func idsOf(t *testing.T, batch semreg.PublicationBatch) []string {
	t.Helper()
	var ids []string
	for _, b := range batch.BindingUpserts {
		ids = append(ids, string(b.BindingID))
	}
	for _, s := range batch.ServiceUpserts {
		ids = append(ids, string(s.InstanceID))
	}
	for _, c := range batch.CapabilityUpserts {
		ids = append(ids, string(c.InstanceID))
	}
	for _, f := range batch.FactUpserts {
		ids = append(ids, string(f.CandidateID), string(f.Origin.OriginID))
	}
	sort.Strings(ids)
	return ids
}

func TestIdentifiersAreStableAcrossRestartAndDiscoveryOrder(t *testing.T) {
	a, b := syntheticPublication(), syntheticPublication()
	b.Asset, b.Source, b.Epoch, b.EVSE, b.Connector = "asset:second-charger", "source:second-charger", "epoch:second-charger", "evse-b", "connector-b"
	one := func(pub Gen3Publication) []string {
		_, s := firstBatch(t, sixPollConfig(), pub, nil)
		return idsOf(t, s.committed()[0])
	}
	firstA, firstB := one(a), one(b)
	secondB, secondA := one(b), one(a)
	if len(firstA) == 0 || len(firstA) != len(secondA) {
		t.Fatal("no identifiers")
	}
	for i := range firstA {
		if firstA[i] != secondA[i] {
			t.Fatalf("charger A changed across restart and reverse discovery: %s != %s", firstA[i], secondA[i])
		}
	}
	for i := range firstB {
		if firstB[i] != secondB[i] {
			t.Fatalf("charger B changed: %s != %s", firstB[i], secondB[i])
		}
	}
	seen := map[string]bool{}
	for _, id := range firstA {
		seen[id] = true
	}
	for _, id := range firstB {
		if seen[id] {
			t.Fatalf("two chargers share the identifier %s", id)
		}
	}
}
