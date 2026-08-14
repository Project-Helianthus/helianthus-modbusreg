package modbusreg

import "fmt"

type sunSpecCurrent struct {
	id, length, header uint16
	spans              []SunSpecSourceSpan
	words              []uint16
}
type SunSpecChain struct {
	plan       SunSpecChainPlan
	seen       map[uint64]struct{}
	provenance *LogicalViewRecord
	pending    map[uint64]SunSpecReadRequest
	next       uint64
	selected   *uint16
	raw        []uint16
	occ        []SunSpecOccurrence
	current    *sunSpecCurrent
	complete   bool
}

func NewSunSpecChain(p SunSpecChainPlan) *SunSpecChain {
	q := map[uint64]SunSpecReadRequest{}
	for _, r := range p.initial {
		q[r.sequence] = r
	}
	return &SunSpecChain{plan: p, seen: map[uint64]struct{}{}, pending: q, next: uint64(len(p.initial))}
}
func (c *SunSpecChain) NextRequests() []SunSpecReadRequest { return sortedRequests(c.pending) }
func (c *SunSpecChain) Admit(r SunSpecReadRequest, v LogicalViewSnapshot) (SunSpecChainSnapshot, error) {
	if c == nil || c.complete || r.nonce != c.plan.nonce {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec request is detached or replayed")
	}
	want, ok := c.pending[r.sequence]
	if !ok || want != r {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec request was not planned")
	}
	x := v.Record()
	if _, ok := c.seen[x.LogicalViewID]; ok {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec logical view is duplicated")
	}
	if x.RequestedFunction != FunctionReadHoldingRegisters || x.ReceivedFunction != FunctionReadHoldingRegisters || x.LogicalOffset != r.address || x.LogicalWordCount != r.words || len(x.Words) != int(r.words) {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec view does not match request")
	}
	if c.provenance != nil && !sameSunSpecProvenance(*c.provenance, x) {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec views have mixed provenance")
	}
	if r.purpose == SunSpecReadSignature {
		return c.signature(r, x)
	}
	if c.selected == nil {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec base is not selected")
	}
	if r.purpose == SunSpecReadHeader {
		return c.header(r, x)
	}
	if r.purpose == SunSpecReadPayload {
		return c.payload(r, x)
	}
	return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec request purpose invalid")
}
func (c *SunSpecChain) accept(r SunSpecReadRequest, x LogicalViewRecord) {
	if c.provenance == nil {
		y := x
		c.provenance = &y
	}
	c.seen[x.LogicalViewID] = struct{}{}
	delete(c.pending, r.sequence)
}
func (c *SunSpecChain) signature(r SunSpecReadRequest, x LogicalViewRecord) (SunSpecChainSnapshot, error) {
	if x.Words[0] == sunSpecSignatureFirst && x.Words[1] == sunSpecSignatureSecond {
		if c.selected != nil {
			return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec base candidates are ambiguous")
		}
		b := r.address
		c.selected = &b
	}
	c.accept(r, x)
	if len(c.pending) != 0 {
		return SunSpecChainSnapshot{}, nil
	}
	if c.selected == nil {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec signature absent from candidates")
	}
	c.raw = []uint16{sunSpecSignatureFirst, sunSpecSignatureSecond}
	return SunSpecChainSnapshot{}, c.queue(*c.selected+2, 2, SunSpecReadHeader)
}
func (c *SunSpecChain) header(r SunSpecReadRequest, x LogicalViewRecord) (SunSpecChainSnapshot, error) {
	id, n := x.Words[0], x.Words[1]
	if id == sunSpecEndModel {
		if n != 0 {
			return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec end marker nonzero")
		}
		c.accept(r, x)
		c.raw = append(c.raw, id, n)
		c.complete = true
		return c.snapshot(), nil
	}
	if n == 0 {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec non-end model has zero length")
	}
	if uint32(len(c.raw))+2+uint32(n)+2 > c.plan.limits.MaxTotalWords {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec total word bound exceeded")
	}
	if uint32(len(c.occ))+1 > c.plan.limits.MaxOccurrences {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec occurrence bound exceeded")
	}
	if uint32(r.address)+2+uint32(n) > 65536 {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec model range overflows")
	}
	c.accept(r, x)
	c.raw = append(c.raw, id, n)
	c.current = &sunSpecCurrent{id: id, length: n, header: r.address, words: []uint16{id, n}, spans: []SunSpecSourceSpan{{x.LogicalViewID, r.address, 2}}}
	return SunSpecChainSnapshot{}, c.queue(r.address+2, minSunSpec(n), SunSpecReadPayload)
}
func minSunSpec(n uint16) uint16 {
	if n > maxSunSpecReadWords {
		return maxSunSpecReadWords
	}
	return n
}
func (c *SunSpecChain) payload(r SunSpecReadRequest, x LogicalViewRecord) (SunSpecChainSnapshot, error) {
	if c.current == nil {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec payload lacks header")
	}
	c.accept(r, x)
	c.current.words = append(c.current.words, x.Words...)
	c.current.spans = append(c.current.spans, SunSpecSourceSpan{x.LogicalViewID, r.address, r.words})
	c.raw = append(c.raw, x.Words...)
	if uint16(len(c.current.words)-2) < c.current.length {
		return SunSpecChainSnapshot{}, c.queue(r.address+r.words, minSunSpec(c.current.length-uint16(len(c.current.words)-2)), SunSpecReadPayload)
	}
	wk := SunSpecWireKey{c.current.id, c.current.length}
	d := SunSpecChainDispositionUnknownModel
	var key *SunSpecDecoderKey
	if k, ok := c.plan.keys[wk]; ok {
		d = SunSpecChainDispositionAdmitted
		z := k
		key = &z
	} else if _, ok := c.plan.known[c.current.id]; ok {
		d = SunSpecChainDispositionUnsupportedLength
	}
	c.occ = append(c.occ, SunSpecOccurrence{uint32(len(c.occ) + 1), wk, c.current.header, c.current.header + 2, d, key, append([]uint16(nil), c.current.words...), append([]SunSpecSourceSpan(nil), c.current.spans...)})
	c.current = nil
	return SunSpecChainSnapshot{}, c.queue(r.address+r.words, 2, SunSpecReadHeader)
}
func (c *SunSpecChain) queue(a, n uint16, p SunSpecReadPurpose) error {
	if n == 0 || n > maxSunSpecReadWords {
		return fmt.Errorf("SunSpec request bound invalid")
	}
	if err := sunSpecEnd(a, n); err != nil {
		return err
	}
	c.next++
	c.pending[c.next] = SunSpecReadRequest{a, n, p, c.plan.nonce, c.next}
	return nil
}
func sameSunSpecProvenance(a, b LogicalViewRecord) bool {
	return a.Endpoint == b.Endpoint && a.UnitID == b.UnitID && a.ConnectionID == b.ConnectionID && a.Transport == b.Transport && a.TransportGeneration == b.TransportGeneration && a.PollGeneration == b.PollGeneration && a.DeadlineIdentity == b.DeadlineIdentity && a.AuthorizationScope == b.AuthorizationScope
}
func (c *SunSpecChain) snapshot() SunSpecChainSnapshot {
	out := SunSpecChainSnapshot{raw: append([]uint16(nil), c.raw...)}
	for _, o := range c.occ {
		out.occurrences = append(out.occurrences, cloneOccurrence(o))
	}
	return out
}
