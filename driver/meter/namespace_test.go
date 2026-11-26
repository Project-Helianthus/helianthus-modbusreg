package meter

import (
	"context"
	"encoding/json"
	"testing"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

type recordingRegistry struct {
	nodes   []driver.NodeSpec
	readers map[driver.MemberID]driver.NativeReader
}

func (r *recordingRegistry) Root() driver.NodeRef {
	return driver.NodeRef{Bridge: "bridge.synthetic", ID: semreg.ResourceID("node.root")}
}

func (r *recordingRegistry) Node(s driver.NodeSpec) (driver.NodeRef, error) {
	r.nodes = append(r.nodes, s)
	return driver.NodeRef{Bridge: "bridge.synthetic", ID: semreg.ResourceID("node." + string(s.Local))}, nil
}

func (r *recordingRegistry) ReadMember(_ driver.NodeRef, s driver.MemberSpec, read driver.NativeReader) error {
	if r.readers == nil {
		r.readers = map[driver.MemberID]driver.NativeReader{}
	}
	r.readers[s.Local] = read
	return nil
}

func (r *recordingRegistry) View(driver.ViewSpec) error { return nil }

func (r *recordingRegistry) Method(driver.NodeRef, driver.MethodSpec, driver.NativeMethod) error {
	return nil
}

func TestNamespaceServesRetainedObservationsWithoutARequest(t *testing.T) {
	cfg := dongleConfig(7)
	p := testPublisher(t, cfg)
	g := newSDongleRig(t, 7)
	cycle := func(w int32) []HuaweiCycle {
		return []HuaweiCycle{{Path: PathSDongle, Cycle: sdongleCycle(t, g, w, 0)}}
	}
	first, err := p.Publish(Input{Template: template(), Huawei: cycle(500)})
	if err != nil {
		t.Fatal(err)
	}
	reg := &recordingRegistry{}
	if err := p.Register(reg, first); err != nil {
		t.Fatal(err)
	}
	read := reg.readers["active_power"]
	// The provider's own cycle also carries the plant groups of the gateway unit, so the
	// grid point's node is registered beside the meter.
	var meterNodes int
	for _, n := range reg.nodes {
		if n.Local == "dongle-meter" {
			meterNodes++
		}
	}
	if read == nil || len(reg.nodes) != 2 || meterNodes != 1 {
		t.Fatalf("registered %+v %v", reg.nodes, reg.readers)
	}
	var n Native
	raw, err := read(context.Background())
	if err != nil || json.Unmarshal(raw, &n) != nil || n.Unit != "W" || n.Value.Coefficient != "5" || n.Sign != "positive_export" {
		t.Fatalf("read %s: %v", raw, err)
	}
	// A later cycle is served by the same reader; no session exists at all.
	if _, err := p.Publish(Input{Template: template(), Huawei: cycle(-300)}); err != nil {
		t.Fatal(err)
	}
	raw, err = read(context.Background())
	if err != nil || json.Unmarshal(raw, &n) != nil || n.Value.Coefficient != "-3" {
		t.Fatalf("retained read %s: %v", raw, err)
	}
}
