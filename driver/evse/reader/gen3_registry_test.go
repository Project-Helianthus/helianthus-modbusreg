package reader

import (
	"context"
	"sync"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

// recordingRegistry is a synthetic native registry: it records the nodes and
// members a reader declares and answers a member read from the reader.
type recordingRegistry struct {
	mu      sync.Mutex
	nodes   []driver.NodeSpec
	members []driver.MemberSpec
	readers map[driver.MemberID]driver.NativeReader
}

func (r *recordingRegistry) Root() driver.NodeRef {
	return driver.NodeRef{Bridge: "bridge.synthetic", ID: semreg.ResourceID("node.root")}
}

func (r *recordingRegistry) Node(s driver.NodeSpec) (driver.NodeRef, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodes = append(r.nodes, s)
	return driver.NodeRef{Bridge: "bridge.synthetic", ID: semreg.ResourceID("node." + string(s.Local))}, nil
}

func (r *recordingRegistry) ReadMember(_ driver.NodeRef, s driver.MemberSpec, read driver.NativeReader) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.readers == nil {
		r.readers = map[driver.MemberID]driver.NativeReader{}
	}
	r.members = append(r.members, s)
	r.readers[s.Local] = read
	return nil
}

func (r *recordingRegistry) View(driver.ViewSpec) error { return nil }

func (r *recordingRegistry) Method(driver.NodeRef, driver.MethodSpec, driver.NativeMethod) error {
	return nil
}

func (r *recordingRegistry) read(member driver.MemberID) ([]byte, error) {
	r.mu.Lock()
	read := r.readers[member]
	r.mu.Unlock()
	return read(context.Background())
}
