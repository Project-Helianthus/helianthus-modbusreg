package modbusreg

import "fmt"

// SunSpecChain incrementally admits planner-issued FC03 logical views.
type SunSpecChain struct {
	plan       SunSpecChainPlan
	seen       map[uint64]struct{}
	provenance *LogicalViewRecord
	pending    map[uint64]SunSpecReadRequest
	next       uint64
	words      []uint16
	spans      []SunSpecSourceSpan
	selected   *uint16
	complete   bool
}

func NewSunSpecChain(plan SunSpecChainPlan) *SunSpecChain {
	pending := make(map[uint64]SunSpecReadRequest)
	for _, r := range plan.initial {
		pending[r.sequence] = r
	}
	return &SunSpecChain{plan: plan, seen: make(map[uint64]struct{}), pending: pending, next: uint64(len(plan.initial))}
}
func (c *SunSpecChain) Admit(request SunSpecReadRequest, view LogicalViewSnapshot) (SunSpecChainSnapshot, error) {
	if c == nil || c.complete || request.nonce != c.plan.nonce {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec request is detached or replayed")
	}
	expected, ok := c.pending[request.sequence]
	if !ok || expected != request {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec request was not planned")
	}
	record := view.Record()
	if _, duplicate := c.seen[record.LogicalViewID]; duplicate {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec logical view is duplicated")
	}
	if record.RequestedFunction != FunctionReadHoldingRegisters || record.ReceivedFunction != FunctionReadHoldingRegisters || record.LogicalOffset != request.address || record.LogicalWordCount != request.words || len(record.Words) != int(request.words) {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec view does not match request")
	}
	if c.provenance == nil {
		copy := record
		c.provenance = &copy
	} else if !sameSunSpecProvenance(*c.provenance, record) {
		return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec views have mixed provenance")
	}
	c.seen[record.LogicalViewID] = struct{}{}
	delete(c.pending, request.sequence)
	c.words = append(c.words, record.Words...)
	c.spans = append(c.spans, SunSpecSourceSpan{LogicalViewID: record.LogicalViewID, PDUOffset: record.LogicalOffset, WordCount: record.LogicalWordCount})
	if request.purpose == SunSpecReadSignature {
		if record.Words[0] != sunSpecSignatureFirst || record.Words[1] != sunSpecSignatureSecond {
			return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec signature is invalid")
		}
		base := request.address
		c.selected = &base
		c.queue(base+2, 2, SunSpecReadHeader)
		return SunSpecChainSnapshot{}, nil
	}
	if request.purpose == SunSpecReadHeader {
		id, length := record.Words[0], record.Words[1]
		if id == sunSpecEndModel {
			if length != 0 {
				return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec end marker is nonzero")
			}
			c.complete = true
			return c.snapshot(), nil
		}
		if length == 0 {
			return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec non-end model has zero length")
		}
		if uint32(len(c.words))+uint32(length)+2 > c.plan.limits.MaxTotalWords {
			return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec total word bound exceeded")
		}
		c.queue(request.address+2, length, SunSpecReadPayload)
		return SunSpecChainSnapshot{}, nil
	}
	if request.purpose == SunSpecReadPayload {
		c.queue(request.address+request.words, 2, SunSpecReadHeader)
		return SunSpecChainSnapshot{}, nil
	}
	return SunSpecChainSnapshot{}, fmt.Errorf("SunSpec request purpose is invalid")
}
func (c *SunSpecChain) queue(address, words uint16, purpose SunSpecReadPurpose) {
	if words > maxSunSpecReadWords {
		return
	}
	if sunSpecEnd(address, words) != nil {
		return
	}
	c.next++
	c.pending[c.next] = SunSpecReadRequest{address: address, words: words, purpose: purpose, nonce: c.plan.nonce, sequence: c.next}
}
func (c *SunSpecChain) NextRequests() []SunSpecReadRequest {
	out := make([]SunSpecReadRequest, 0, len(c.pending))
	for _, r := range c.pending {
		out = append(out, r)
	}
	return out
}
func sameSunSpecProvenance(a, b LogicalViewRecord) bool {
	return a.Endpoint == b.Endpoint && a.UnitID == b.UnitID && a.ConnectionID == b.ConnectionID && a.Transport == b.Transport && a.TransportGeneration == b.TransportGeneration && a.PollGeneration == b.PollGeneration && a.DeadlineIdentity == b.DeadlineIdentity && a.AuthorizationScope == b.AuthorizationScope
}
func (c *SunSpecChain) snapshot() SunSpecChainSnapshot {
	return SunSpecChainSnapshot{raw: append([]uint16(nil), c.words...)}
}
