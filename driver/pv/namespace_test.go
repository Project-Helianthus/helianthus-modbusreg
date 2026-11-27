package pv

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

type fakeRegistry struct {
	nodes   map[driver.NodeID]driver.NodeRef
	readers map[driver.MemberID]driver.NativeReader
	local   map[driver.NodeID][]driver.MemberID
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{nodes: map[driver.NodeID]driver.NodeRef{}, readers: map[driver.MemberID]driver.NativeReader{}, local: map[driver.NodeID][]driver.MemberID{}}
}

func (f *fakeRegistry) Root() driver.NodeRef {
	return driver.NodeRef{Bridge: "bridge.synthetic", ID: "resource:root"}
}
func (f *fakeRegistry) Node(s driver.NodeSpec) (driver.NodeRef, error) {
	ref := driver.NodeRef{Bridge: "bridge.synthetic", ID: semreg.ResourceID("resource:ns." + string(s.Local))}
	f.nodes[s.Local] = ref
	return ref, nil
}
func (f *fakeRegistry) ReadMember(n driver.NodeRef, s driver.MemberSpec, r driver.NativeReader) error {
	f.readers[s.Local+"@"+driver.MemberID(n.ID)] = r
	f.local[driver.NodeID(n.ID)] = append(f.local[driver.NodeID(n.ID)], s.Local)
	return nil
}
func (f *fakeRegistry) Method(driver.NodeRef, driver.MethodSpec, driver.NativeMethod) error {
	return nil
}
func (f *fakeRegistry) View(driver.ViewSpec) error { return nil }

func TestTheNamespaceServesEveryDecodedPointWithoutARequest(t *testing.T) {
	blocks := [][]uint16{
		derCommonBlock("Maker", "Model", "SN-1"), inverterBlock(t, 103, integerValues(103, nil)), zeros(120, 26), zeros(121, 30),
		zeros(203, 105), zeros(701, 153),
	}
	snapshot := chain(t, 1, blocks...)
	cfg := testConfig()
	p := testPublisher(t, cfg)
	in := Input{Template: template(), SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: snapshot}}}
	pub, err := p.Publish(in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := p.Publish(in)
	if err != nil || !reflect.DeepEqual(pub.Members, again.Members) {
		t.Fatalf("the order of the members is not stable: %v", err)
	}
	// every decoded point of every occurrence is a member: the expected set is
	// built with the decoder alone
	want := 0
	for _, occ := range snapshot.Occurrences() {
		key, ok := occ.DecoderKey()
		if !ok {
			continue
		}
		r, _ := sunspec.NewStandardSunSpecDecoderRegistry(key.SchemaRevision)
		m, err := r.DecodeOccurrence(occ)
		if err != nil {
			t.Fatal(err)
		}
		want += len(m.Facts())
	}
	if len(pub.Members) != want || want == 0 {
		t.Fatalf("%d members for %d decoded points", len(pub.Members), want)
	}
	byKey := map[string]Member{}
	for _, m := range pub.Members {
		byKey[fmt.Sprint(m.Key.Node, "/", m.Key.Local)] = m
	}
	w, ok := byKey["unit-1.segment-1/m103.o2.W"]
	if !ok || w.Point.Model != 103 || w.Point.Name != "W" || w.Point.Unit != "W" || w.Point.Value != "2500" || !reflect.DeepEqual(w.Point.Raw, []uint16{2500}) || w.Point.Validity != "valid" {
		t.Fatalf("W: %+v", w)
	}
	for _, id := range []string{"m1.o1.Mn", "m120.o3.WRtg", "m121.o4.WMax", "m203.o5.A", "m701.o6.W"} {
		if _, ok := byKey["unit-1.segment-1/"+id]; !ok {
			t.Fatalf("%s is not served", id)
		}
	}
	reg := newFakeRegistry()
	if err := p.Register(reg, pub); err != nil {
		t.Fatal(err)
	}
	node := reg.nodes["unit-1.segment-1"]
	read := reg.readers["m103.o2.W@"+driver.MemberID(node.ID)]
	if read == nil {
		t.Fatal("member not registered")
	}
	raw, err := read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got Point
	if err := json.Unmarshal(raw, &got); err != nil || got.Value != "2500" {
		t.Fatalf("read %s: %v", raw, err)
	}
}
