package modbusreg

import (
	"fmt"
	"sort"
	"sync/atomic"
)

const maxSunSpecReadWords uint16 = 125

var sunSpecPlanNonce atomic.Uint64
var sunSpecChainInstance atomic.Uint64

type SunSpecChainLimits struct {
	MaxTotalWords  uint32
	MaxOccurrences uint32
}
type SunSpecChainPlanSpec struct {
	SchemaRevision         SunSpecSchemaRevision
	BaseCandidates         []uint16
	Limits                 SunSpecChainLimits
	DecoderKeys            []SunSpecDecoderKey
	StructuralCandidateIDs []uint16
}
type SunSpecChainPlan struct {
	revision               SunSpecSchemaRevision
	bases                  []uint16
	limits                 SunSpecChainLimits
	keys                   map[SunSpecWireKey]SunSpecDecoderKey
	known                  map[uint16]struct{}
	structuralCandidateIDs map[uint16]struct{}
	nonce                  uint64
	initial                []SunSpecReadRequest
}
type SunSpecReadRequest struct {
	address, words            uint16
	purpose                   SunSpecReadPurpose
	nonce, instance, sequence uint64
}

func (r SunSpecReadRequest) Function() FunctionCode      { return FunctionReadHoldingRegisters }
func (r SunSpecReadRequest) Address() uint16             { return r.address }
func (r SunSpecReadRequest) WordCount() uint16           { return r.words }
func (r SunSpecReadRequest) Purpose() SunSpecReadPurpose { return r.purpose }
func NewSunSpecChainPlan(s SunSpecChainPlanSpec) (SunSpecChainPlan, error) {
	if !validSunSpecRevision(s.SchemaRevision) || len(s.BaseCandidates) == 0 || s.Limits.MaxTotalWords < 4 || s.Limits.MaxOccurrences == 0 || s.Limits.MaxTotalWords > 65536 || s.Limits.MaxOccurrences > s.Limits.MaxTotalWords/2 {
		return SunSpecChainPlan{}, fmt.Errorf("SunSpec chain plan requires explicit finite bounds")
	}
	p := SunSpecChainPlan{revision: s.SchemaRevision, bases: append([]uint16(nil), s.BaseCandidates...), limits: s.Limits, keys: map[SunSpecWireKey]SunSpecDecoderKey{}, known: map[uint16]struct{}{}, structuralCandidateIDs: map[uint16]struct{}{}, nonce: sunSpecPlanNonce.Add(1)}
	seenBases := make(map[uint16]struct{}, len(p.bases))
	for _, b := range p.bases {
		if _, duplicate := seenBases[b]; duplicate {
			return SunSpecChainPlan{}, fmt.Errorf("SunSpec base candidate duplicated")
		}
		if err := sunSpecEnd(b, 2); err != nil {
			return SunSpecChainPlan{}, err
		}
		seenBases[b] = struct{}{}
	}
	for _, k := range s.DecoderKeys {
		if k.SchemaRevision != s.SchemaRevision || k.ModelLength == 0 {
			return SunSpecChainPlan{}, fmt.Errorf("SunSpec decoder key is not exact")
		}
		wk := SunSpecWireKey{k.ModelID, k.ModelLength}
		if _, ok := p.keys[wk]; ok {
			return SunSpecChainPlan{}, fmt.Errorf("SunSpec decoder key duplicated")
		}
		p.keys[wk] = k
		p.known[k.ModelID] = struct{}{}
	}
	for _, id := range s.StructuralCandidateIDs {
		if _, exists := p.structuralCandidateIDs[id]; exists {
			return SunSpecChainPlan{}, fmt.Errorf("SunSpec structural candidate identifier duplicated")
		}
		p.structuralCandidateIDs[id] = struct{}{}
	}
	for i, b := range p.bases {
		p.initial = append(p.initial, SunSpecReadRequest{address: b, words: 2, purpose: SunSpecReadSignature, nonce: p.nonce, sequence: uint64(i + 1)})
	}
	return p, nil
}
func (p SunSpecChainPlan) Requests() []SunSpecReadRequest {
	return append([]SunSpecReadRequest(nil), p.initial...)
}
func (p SunSpecChainPlan) SchemaRevision() SunSpecSchemaRevision { return p.revision }
func (p SunSpecChainPlan) structuralCandidate(wireKey SunSpecWireKey, words []uint16, spans []SunSpecSourceSpan) *sunSpecStructuralCandidate {
	if _, enabled := p.structuralCandidateIDs[wireKey.ModelID]; !enabled {
		return nil
	}
	return sunSpecV2DERTripLVStructuralCandidate(p.revision, wireKey, words, spans)
}
func sortedRequests(m map[uint64]SunSpecReadRequest) []SunSpecReadRequest {
	ids := make([]uint64, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]SunSpecReadRequest, 0, len(ids))
	for _, id := range ids {
		out = append(out, m[id])
	}
	return out
}
