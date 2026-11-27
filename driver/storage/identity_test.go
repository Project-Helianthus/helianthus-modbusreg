package storage

import (
	"context"
	"sort"
	"testing"
)

// shape is the set of resource identifiers with their kinds and the set of
// link keys a lifecycle published.
func shape(k *kernel) ([]string, []string) {
	var nodes, links []string
	for id, r := range k.resources {
		nodes = append(nodes, string(id)+"="+string(r.Kind))
	}
	for _, r := range k.relations {
		links = append(links, string(r.Definition.ID)+"|"+string(r.From)+"|"+string(r.To))
	}
	for _, c := range k.claims {
		links = append(links, "claim|"+string(c.Key.Type.ID)+"|"+string(c.Key.From.ID)+"|"+string(c.Key.To.ID))
	}
	sort.Strings(nodes)
	sort.Strings(links)
	return nodes, links
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTwoLifecyclesInReverseDiscoveryOrderGiveTheSameNodesAndLinks(t *testing.T) {
	cfg := testConfig()
	cfg.System = "system-a"
	cfg.Huawei = []HuaweiBattery{{Unit: 1, Battery: "battery-a"}, {Unit: 2, Battery: "battery-b"}}
	one := childOf(t, "LB-C00", 1, withWords(nil))
	two := childOf(t, "LB-C00", 2, withWords(map[uint16]uint16{37004: 111, 37003: 4000}))
	run := func(children ...HuaweiChild) *kernel {
		s := newSession("asset:inverter-a", "source-a", "epoch-1", 1)
		p := newPublisher(t, cfg, s, nil)
		publishHuawei(t, p, HuaweiObservation{Children: children, Receipt: receipt(1)})
		return s.k
	}
	forward, reverse := run(one, two), run(two, one)
	fn, fl := shape(forward)
	rn, rl := shape(reverse)
	if len(fn) == 0 || !equalStrings(fn, rn) || !equalStrings(fl, rl) {
		t.Fatalf("reverse discovery order changed the graph:\n%v\n%v\n%v\n%v", fn, rn, fl, rl)
	}
	// And the same fact keys.
	keys := func(k *kernel) []string {
		var out []string
		for _, c := range k.facts {
			out = append(out, string(c.Key.FactID)+"|"+*c.Key.Dimensions[0].Value.Text)
		}
		sort.Strings(out)
		return out
	}
	if !equalStrings(keys(forward), keys(reverse)) {
		t.Fatal("reverse discovery order changed the fact keys")
	}
}

func TestIdentityDoesNotDependOnAddressGenerationOrClock(t *testing.T) {
	f := loadFixture(t)
	ids := func(epoch string, generation uint64, n int) []string {
		s := newSession(f.Configuration.Asset, f.Configuration.Source, epoch, generation)
		a := &allocator{}
		p := newPublisher(t, f.config(), s, a)
		if err := p.PublishGrowatt(context.Background(), f.observe(t, 2, n, false)); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, key := range a.seen {
			out = append(out, string(key.Kind)+"|"+string(key.Anchor.Parent)+"|"+key.Anchor.Slot)
		}
		sort.Strings(out)
		return out
	}
	if !equalStrings(ids("epoch-1", 1, 1), ids("epoch-9", 7, 5)) {
		t.Fatal("the node keys depend on the epoch, generation or clock")
	}
}
