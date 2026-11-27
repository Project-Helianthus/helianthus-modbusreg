package pv

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

const (
	nodeContract driver.SchemaRef  = "modbus.pv/v1"
	memberSchema driver.SchemaRef  = "modbus.pv.native/v1"
	memberMode   driver.AccessMode = "read"
	memberScope  driver.ScopeID    = "diagnostic"
	segmentKind                    = "modbus.pv.native"
)

// members declares, for every occurrence of the chain, every decoded point with
// its model, name, unit, scale-applied value, raw words and validity. An
// occurrence the decoder does not admit keeps its raw words.
func (r *run) members(u SunSpecUnit) error {
	segment := uint32(0)
	for _, occ := range u.Snapshot.Occurrences() {
		if occ.ModelID() == 1 {
			segment = occ.Ordinal
		}
		node := fmt.Sprintf("unit-%d.segment-%d", u.Unit, segment)
		prefix := fmt.Sprintf("m%d.o%d.", occ.ModelID(), occ.Ordinal)
		decoded, err := r.decode(occ)
		if err != nil {
			r.member(node, prefix+"raw", Point{Model: occ.ModelID(), Ordinal: occ.Ordinal, Name: "raw", Raw: occ.Words(), Validity: "undecoded", Reason: err.Error()})
			continue
		}
		for _, f := range decoded.Facts() {
			name := f.PointName
			if f.Repeated {
				name += "." + strconv.Itoa(int(f.RepeatIndex))
			}
			r.member(node, prefix+name, Point{Model: occ.ModelID(), Ordinal: occ.Ordinal, Name: name, Field: f.FieldID, Unit: f.Unit,
				Value: valueText(f.Value), Raw: f.Value.RawWords(), Validity: string(f.Value.State())})
		}
	}
	return nil
}

// valueText renders the scale-applied value of a decoded point.
func valueText(v sunspec.Value) string {
	switch {
	case v.State() != sunspec.ValueValid:
		return ""
	}
	if d, ok := v.Decimal(); ok {
		return plain(strconv.FormatInt(d.Coefficient, 10), int(d.Exponent))
	}
	if d, ok := v.UnsignedDecimal(); ok {
		return plain(strconv.FormatUint(d.Coefficient, 10), int(d.Exponent))
	}
	if f, ok := v.Float32(); ok {
		return strconv.FormatFloat(float64(f), 'f', -1, 32)
	}
	if n, symbol, ok := v.Enum(); ok {
		if symbol != "" {
			return symbol
		}
		return strconv.FormatUint(n, 10)
	}
	if bits, _, ok := v.Bitfield(); ok {
		return strconv.FormatUint(bits, 10)
	}
	if n, ok := v.Unsigned(); ok {
		return strconv.FormatUint(n, 10)
	}
	if n, ok := v.Signed(); ok {
		return strconv.FormatInt(n, 10)
	}
	if t, ok := v.Text(); ok {
		return t
	}
	return ""
}

func (p *Publisher) retain(pub Publication) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.latest == nil {
		p.latest = map[driver.MemberKey]Point{}
	}
	for _, m := range pub.Members {
		p.latest[m.Key] = m.Point
	}
}

func (p *Publisher) read(key driver.MemberKey) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	pt, ok := p.latest[key]
	if !ok {
		return nil, fmt.Errorf("no retained observation for %s", key.Local)
	}
	return json.Marshal(pt)
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
			ref, err = reg.Node(driver.NodeSpec{Kind: segmentKind, Local: m.Key.Node, Contract: nodeContract})
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
