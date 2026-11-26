package storage

import "sort"

// Members lists every native value the producer retained for the driver
// namespace, in key order: the values that have no element of the pack, and
// the native value beside each fact. It sends no request.
func (p *Publisher) Members() []Member {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Member, 0, len(p.members))
	for key, native := range p.members {
		out = append(out, Member{Key: key, Native: native})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Key, out[j].Key
		if a.Node != b.Node {
			return a.Node < b.Node
		}
		return a.Local < b.Local
	})
	return out
}
