package sunspec

import (
	"bytes"
	"fmt"
	"sync"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

type sunSpecCurrent struct {
	id, length, header uint16
	spans              []SourceSpan
	words              []uint16
}
type sunSpecPhysicalSource struct {
	wireResponseID, connectionID, transportGeneration uint64
	function                                          modbusreg.FunctionCode
	table                                             modbusreg.LogicalTable
	offset, count                                     uint16
}
type sunSpecWireSource struct {
	physicalRequestID uint64
	responseBytes     []byte
}

// Chain walks one SunSpec chain from planned reads to a
// ChainSnapshot. The caller performs each request from NextRequests and
// passes the logical view that answered it to AdmitReplay; the chain does no
// I/O itself. All views must share one provenance. The walk ends at the end
// marker or at the first structural failure, after which nothing is accepted.
// Methods are safe for concurrent use. See
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/sunspec/read-only-core-v1.md.
type Chain struct {
	mu         sync.Mutex
	plan       ChainPlan
	instance   uint64
	seen       map[uint64]struct{}
	physical   map[uint64]sunSpecPhysicalSource
	wire       map[uint64]sunSpecWireSource
	provenance *modbusreg.LogicalViewRecord
	pending    map[uint64]ReadRequest
	next       uint64
	selected   *uint16
	raw        []uint16
	occ        []Occurrence
	sources    []modbusreg.LogicalViewSnapshot
	current    *sunSpecCurrent
	complete   bool
	failed     bool
}

// NewChain starts a walk of plan p under a process-unique instance
// number. If instance numbers are exhausted it returns a chain that rejects
// every view.
func NewChain(p ChainPlan) *Chain {
	instance := nextSunSpecChainInstance()
	if instance == 0 {
		return &Chain{failed: true}
	}
	q := map[uint64]ReadRequest{}
	for _, r := range p.initial {
		r.instance = instance
		q[r.sequence] = r
	}
	return &Chain{
		plan:     p,
		instance: instance,
		seen:     map[uint64]struct{}{},
		physical: map[uint64]sunSpecPhysicalSource{},
		wire:     map[uint64]sunSpecWireSource{},
		pending:  q,
		next:     uint64(len(p.initial)),
	}
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

// NextRequests returns the outstanding requests in issue order: all signature
// probes first, then one header or payload request at a time. It returns none
// once the walk is complete or failed, and nil for a nil chain.
func (c *Chain) NextRequests() []ReadRequest {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return sortedRequests(c.pending)
}

// AdmitReplay admits the logical view v that answered request r and advances
// the walk. It returns a snapshot with raw words only when v completes the
// chain at the end marker; a zero snapshot with a nil error means more
// requests are pending. A request or view that does not belong to this walk
// (another chain's, replayed, unplanned, a duplicated view, a mismatched range
// or mixed provenance) is rejected and leaves the walk unchanged. An invalid
// chain (ambiguous or absent signature, bad header, exceeded limit, address
// overflow or contradictory source identity) fails the walk for good.
func (c *Chain) AdmitReplay(r ReadRequest, v modbusreg.LogicalViewSnapshot) (ChainSnapshot, error) {
	if c == nil {
		return ChainSnapshot{}, fmt.Errorf("SunSpec chain is unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.admitReplay(r, v)
}
func (c *Chain) admitReplay(r ReadRequest, v modbusreg.LogicalViewSnapshot) (ChainSnapshot, error) {
	if c == nil || c.complete || c.failed || c.instance == 0 || r.nonce != c.plan.nonce || r.instance != c.instance {
		return ChainSnapshot{}, fmt.Errorf("SunSpec request is detached or replayed")
	}
	want, ok := c.pending[r.sequence]
	if !ok || want != r {
		return ChainSnapshot{}, fmt.Errorf("SunSpec request was not planned")
	}
	x := v.Record()
	if _, ok := c.seen[x.LogicalViewID]; ok {
		return ChainSnapshot{}, fmt.Errorf("SunSpec logical view is duplicated")
	}
	if x.RequestedFunction != modbusreg.FunctionReadHoldingRegisters || x.ReceivedFunction != modbusreg.FunctionReadHoldingRegisters || x.LogicalOffset != r.address || x.LogicalWordCount != r.words || len(x.Words) != int(r.words) {
		return ChainSnapshot{}, fmt.Errorf("SunSpec view does not match request")
	}
	if c.provenance != nil && !SameProvenance(*c.provenance, x) {
		return ChainSnapshot{}, fmt.Errorf("SunSpec views have mixed provenance")
	}
	if err := c.validateSourceIdentity(x); err != nil {
		return ChainSnapshot{}, c.fail(err)
	}
	if r.purpose == ReadSignature {
		snapshot, err := c.signature(r, v, x)
		if err != nil {
			return snapshot, c.fail(err)
		}
		return snapshot, nil
	}
	if c.selected == nil {
		return ChainSnapshot{}, fmt.Errorf("SunSpec base is not selected")
	}
	if r.purpose == ReadHeader {
		snapshot, err := c.header(r, v, x)
		if err != nil {
			return snapshot, c.fail(err)
		}
		return snapshot, nil
	}
	if r.purpose == ReadPayload {
		snapshot, err := c.payload(r, v, x)
		if err != nil {
			return snapshot, c.fail(err)
		}
		return snapshot, nil
	}
	return ChainSnapshot{}, fmt.Errorf("SunSpec request purpose invalid")
}
func (c *Chain) validateSourceIdentity(x modbusreg.LogicalViewRecord) error {
	physical := sunSpecPhysicalSource{
		wireResponseID:      x.WireResponseID,
		connectionID:        x.ConnectionID,
		transportGeneration: x.TransportGeneration,
		function:            x.RequestedFunction,
		table:               x.Table,
		offset:              x.PhysicalOffset,
		count:               x.PhysicalWordCount,
	}
	if prior, ok := c.physical[x.PhysicalRequestID]; ok && prior != physical {
		return fmt.Errorf("SunSpec physical-request identity is contradictory")
	}
	if prior, ok := c.wire[x.WireResponseID]; ok &&
		(prior.physicalRequestID != x.PhysicalRequestID || !bytes.Equal(prior.responseBytes, x.WireResponseBytes)) {
		return fmt.Errorf("SunSpec wire-response identity is contradictory")
	}
	c.physical[x.PhysicalRequestID] = physical
	c.wire[x.WireResponseID] = sunSpecWireSource{
		physicalRequestID: x.PhysicalRequestID,
		responseBytes:     append([]byte(nil), x.WireResponseBytes...),
	}
	return nil
}
func (c *Chain) fail(err error) error {
	c.failed = true
	clear(c.pending)
	c.current = nil
	return err
}
func (c *Chain) accept(r ReadRequest, v modbusreg.LogicalViewSnapshot, x modbusreg.LogicalViewRecord) {
	if c.provenance == nil {
		y := x
		c.provenance = &y
	}
	c.seen[x.LogicalViewID] = struct{}{}
	delete(c.pending, r.sequence)
	c.sources = append(c.sources, v.Clone())
}
func (c *Chain) signature(r ReadRequest, v modbusreg.LogicalViewSnapshot, x modbusreg.LogicalViewRecord) (ChainSnapshot, error) {
	if x.Words[0] == sunSpecSignatureFirst && x.Words[1] == sunSpecSignatureSecond {
		if c.selected != nil {
			c.failed = true
			clear(c.pending)
			return ChainSnapshot{}, fmt.Errorf("SunSpec base candidates are ambiguous")
		}
		b := r.address
		c.selected = &b
	}
	c.accept(r, v, x)
	if len(c.pending) != 0 {
		return ChainSnapshot{}, nil
	}
	if c.selected == nil {
		return ChainSnapshot{}, fmt.Errorf("SunSpec signature absent from candidates")
	}
	c.raw = []uint16{sunSpecSignatureFirst, sunSpecSignatureSecond}
	return ChainSnapshot{}, c.queue(uint32(*c.selected)+2, 2, ReadHeader)
}
func (c *Chain) header(r ReadRequest, v modbusreg.LogicalViewSnapshot, x modbusreg.LogicalViewRecord) (ChainSnapshot, error) {
	id, n := x.Words[0], x.Words[1]
	if id == EndModelID {
		if n != 0 {
			return ChainSnapshot{}, fmt.Errorf("SunSpec end marker nonzero")
		}
		c.accept(r, v, x)
		c.raw = append(c.raw, id, n)
		c.complete = true
		return c.snapshot(), nil
	}
	if n == 0 {
		return ChainSnapshot{}, fmt.Errorf("SunSpec non-end model has zero length")
	}
	if uint32(len(c.raw))+2+uint32(n)+2 > c.plan.limits.MaxTotalWords {
		return ChainSnapshot{}, fmt.Errorf("SunSpec total word bound exceeded")
	}
	if uint32(len(c.occ))+1 > c.plan.limits.MaxOccurrences {
		return ChainSnapshot{}, fmt.Errorf("SunSpec occurrence bound exceeded")
	}
	if uint32(r.address)+2+uint32(n) > 65536 {
		return ChainSnapshot{}, fmt.Errorf("SunSpec model range overflows")
	}
	payloadAddress := uint32(r.address) + 2
	payloadWords := minSunSpec(n)
	if err := sunSpecQueueAddress(payloadAddress, payloadWords); err != nil {
		return ChainSnapshot{}, err
	}
	c.accept(r, v, x)
	c.raw = append(c.raw, id, n)
	c.current = &sunSpecCurrent{id: id, length: n, header: r.address, words: []uint16{id, n}, spans: []SourceSpan{{x.LogicalViewID, x.SliceOffset, 2}}}
	return ChainSnapshot{}, c.queue(payloadAddress, payloadWords, ReadPayload)
}
func minSunSpec(n uint16) uint16 {
	if n > maxSunSpecReadWords {
		return maxSunSpecReadWords
	}
	return n
}
func (c *Chain) payload(r ReadRequest, v modbusreg.LogicalViewSnapshot, x modbusreg.LogicalViewRecord) (ChainSnapshot, error) {
	if c.current == nil {
		return ChainSnapshot{}, fmt.Errorf("SunSpec payload lacks header")
	}
	nextAddress := uint32(r.address) + uint32(r.words)
	completed := uint16(len(c.current.words)-2)+r.words == c.current.length
	nextPurpose, nextWords := ReadPayload, uint16(0)
	if completed {
		nextPurpose, nextWords = ReadHeader, 2
	} else {
		nextWords = minSunSpec(c.current.length - (uint16(len(c.current.words)-2) + r.words))
	}
	if err := sunSpecQueueAddress(nextAddress, nextWords); err != nil {
		return ChainSnapshot{}, err
	}
	c.accept(r, v, x)
	c.current.words = append(c.current.words, x.Words...)
	c.current.spans = append(c.current.spans, SourceSpan{x.LogicalViewID, x.SliceOffset, r.words})
	c.raw = append(c.raw, x.Words...)
	if !completed {
		return ChainSnapshot{}, c.queue(nextAddress, nextWords, nextPurpose)
	}
	wk := WireKey{c.current.id, c.current.length}
	d := ChainDispositionUnknownModel
	var key *DecoderKey
	if k, ok := c.plan.keys[wk]; ok {
		d = ChainDispositionAdmitted
		z := k
		key = &z
	} else if _, ok := c.plan.known[c.current.id]; ok {
		d = ChainDispositionUnsupportedLength
	}
	c.occ = append(c.occ, Occurrence{
		Ordinal:             uint32(len(c.occ) + 1),
		WireKey:             wk,
		SchemaRevision:      c.plan.revision,
		HeaderOffset:        c.current.header,
		PayloadOffset:       c.current.header + 2,
		Disposition:         d,
		decoderKey:          key,
		structuralCandidate: c.plan.structuralCandidate(wk, c.current.words, c.current.spans),
		words:               append([]uint16(nil), c.current.words...),
		spans:               append([]SourceSpan(nil), c.current.spans...),
	})
	c.current = nil
	return ChainSnapshot{}, c.queue(nextAddress, nextWords, nextPurpose)
}
func sunSpecQueueAddress(address uint32, words uint16) error {
	if address > 65535 {
		return fmt.Errorf("SunSpec successor address exceeds address space")
	}
	return sunSpecEnd(uint16(address), words)
}
func (c *Chain) queue(address uint32, n uint16, p ReadPurpose) error {
	if n == 0 || n > maxSunSpecReadWords {
		return fmt.Errorf("SunSpec request bound invalid")
	}
	if err := sunSpecQueueAddress(address, n); err != nil {
		return err
	}
	c.next++
	c.pending[c.next] = ReadRequest{uint16(address), n, p, c.plan.nonce, c.instance, c.next}
	return nil
}

// SameProvenance reports whether two logical view records carry the
// same acquisition provenance: endpoint, unit, connection, transport and its
// generation, poll generation, deadline identity and authorization scope. Every
// source view of one SunSpec chain must share it.
func SameProvenance(a, b modbusreg.LogicalViewRecord) bool {
	return a.Endpoint == b.Endpoint && a.UnitID == b.UnitID && a.ConnectionID == b.ConnectionID && a.Transport == b.Transport && a.TransportGeneration == b.TransportGeneration && a.PollGeneration == b.PollGeneration && a.DeadlineIdentity == b.DeadlineIdentity && a.AuthorizationScope == b.AuthorizationScope
}
func (c *Chain) snapshot() ChainSnapshot {
	out := ChainSnapshot{raw: append([]uint16(nil), c.raw...)}
	for _, o := range c.occ {
		out.occurrences = append(out.occurrences, cloneOccurrence(o))
	}
	for _, source := range c.sources {
		out.sources = append(out.sources, source.Clone())
	}
	return out
}
