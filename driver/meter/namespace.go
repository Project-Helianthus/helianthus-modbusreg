package meter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

const (
	nodeContract driver.SchemaRef  = "modbus.meter/v1"
	memberSchema driver.SchemaRef  = "modbus.meter.native/v1"
	memberMode   driver.AccessMode = "read"
	memberScope  driver.ScopeID    = "diagnostic"
)

func (p *Publisher) retain(pub Publication) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.latest == nil {
		p.latest = map[driver.MemberKey]Native{}
	}
	for _, m := range pub.Members {
		p.latest[m.Key] = m.Native
	}
}

func (p *Publisher) read(key driver.MemberKey) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	n, ok := p.latest[key]
	if !ok {
		return nil, fmt.Errorf("no retained observation for %s", key.Local)
	}
	return json.Marshal(n)
}

// Register declares the nodes and members of a publication in the driver
// namespace. A read answers from the last retained observation and sends no
// request.
func (p *Publisher) Register(reg driver.NativeRegistry, pub Publication) error {
	nodes := map[driver.NodeID]driver.NodeRef{}
	for _, m := range pub.Members {
		ref, ok := nodes[m.Key.Node]
		if !ok {
			var err error
			ref, err = reg.Node(driver.NodeSpec{Kind: kindMeter, Local: m.Key.Node, Contract: nodeContract})
			if err != nil {
				return err
			}
			nodes[m.Key.Node] = ref
		}
		key := m.Key
		spec := driver.MemberSpec{Local: key.Local, Schema: memberSchema, Mode: memberMode, Scope: memberScope}
		if err := reg.ReadMember(ref, spec, func(context.Context) ([]byte, error) { return p.read(key) }); err != nil {
			return err
		}
	}
	return nil
}
