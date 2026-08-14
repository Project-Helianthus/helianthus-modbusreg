package modbusreg

import "fmt"

type sunSpecCurrent struct {
	id, length, header uint16
	spans              []SunSpecSourceSpan
	words              []uint16
}
type SunSpecChain struct {
	plan       SunSpecChainPlan
	instance   uint64
	seen       map[uint64]struct{}
	provenance *LogicalViewRecord
	pending    map[uint64]SunSpecReadRequest
	next       uint64
	selected   *uint16
	raw        []uint16
	occ        []SunSpecOccurrence
	sources    []LogicalViewRecord
	current    *sunSpecCurrent
	complete   bool
	failed     bool
}

func NewSunSpecChain(p SunSpecChainPlan) *SunSpecChain {
	instance := nextSunSpecChainInstance()
	if instance == 0 {
		return &SunSpecChain{failed: true}
	}
	q := map[uint64]SunSpecReadRequest{}
	for _, r := range p.initial {
		r.instance = instance
		q[r.sequence] = r
	}
	return &SunSpecChain{plan: p, instance: instance, seen: map[uint64]struct{}{}, pending: q, next: uint64(len(p.initial))}
}
func nextSunSpecChainInstance() uint64 {
	for {
		current := sunSpecChainInstance.Load()
		if current == ^uint64(0) {
			return 0
		}
		if sunSpecChainInstance.CompareAndSwap(current, current+1) {
			return current + 1
		}
	}
}
func (c *SunSpecChain) NextRequests() []SunSpecReadRequest { return sortedRequests(c.pending) }
func (c *SunSpecChain) AdmitReplay(r SunSpecReadRequest, v LogicalViewSnapshot) (SunSpecChainSnapshot, error) {
	return c.admitReplay(r, v)
}
func (c *SunSpecChain) admitReplay(r SunSpecReadRequest, v LogicalViewSnapshot) (SunSpecChainSnapshot, error) {
	if c == nil || c.complete || c.failed || c.instance == 0 || r.nonce != c.plan.nonce || r.instance != c.instance {
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
	c.sources = append(c.sources, cloneSunSpecLogicalViewRecord(x))
}
func (c *SunSpecChain) signature(r SunSpecReadRequest, x LogicalViewRecord) (SunSpecChainSnapshot, error) {
	if x.Words[0] == sunSpecSignatureFirst && x.Words[1] == sunSpecSignatureSecond {
		if c.selected != nil {
			c.failed = true
			clear(c.pending)
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
	return SunSpecChainSnapshot{}, c.queue(uint32(*c.selected)+2, 2, SunSpecReadHeader)
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
	payloadAddress := uint32(r.address) + 2
	payloadWords := minSunSpec(n)
	if err := sunSpecQueueAddress(payloadAddress, payloadWords); err != nil {
		return SunSpecChainSnapshot{}, err
	}
	c.accept(r, x)
	c.raw = append(c.raw, id, n)
	c.current = &sunSpecCurrent{id: id, length: n, header: r.address, words: []uint16{id, n}, spans: []SunSpecSourceSpan{{x.LogicalViewID, x.SliceOffset, 2}}}
	return SunSpecChainSnapshot{}, c.queue(payloadAddress, payloadWords, SunSpecReadPayload)
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
	nextAddress := uint32(r.address) + uint32(r.words)
	completed := uint16(len(c.current.words)-2)+r.words == c.current.length
	nextPurpose, nextWords := SunSpecReadPayload, uint16(0)
	if completed {
		nextPurpose, nextWords = SunSpecReadHeader, 2
	} else {
		nextWords = minSunSpec(c.current.length - (uint16(len(c.current.words)-2) + r.words))
	}
	if err := sunSpecQueueAddress(nextAddress, nextWords); err != nil {
		return SunSpecChainSnapshot{}, err
	}
	c.accept(r, x)
	c.current.words = append(c.current.words, x.Words...)
	c.current.spans = append(c.current.spans, SunSpecSourceSpan{x.LogicalViewID, x.SliceOffset, r.words})
	c.raw = append(c.raw, x.Words...)
	if !completed {
		return SunSpecChainSnapshot{}, c.queue(nextAddress, nextWords, nextPurpose)
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
	return SunSpecChainSnapshot{}, c.queue(nextAddress, nextWords, nextPurpose)
}
func sunSpecQueueAddress(address uint32, words uint16) error {
	if address > 65535 {
		return fmt.Errorf("SunSpec successor address exceeds address space")
	}
	return sunSpecEnd(uint16(address), words)
}
func (c *SunSpecChain) queue(address uint32, n uint16, p SunSpecReadPurpose) error {
	if n == 0 || n > maxSunSpecReadWords {
		return fmt.Errorf("SunSpec request bound invalid")
	}
	if err := sunSpecQueueAddress(address, n); err != nil {
		return err
	}
	c.next++
	c.pending[c.next] = SunSpecReadRequest{uint16(address), n, p, c.plan.nonce, c.instance, c.next}
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
	for _, source := range c.sources {
		out.sources = append(out.sources, cloneSunSpecLogicalViewRecord(source))
	}
	return out
}
