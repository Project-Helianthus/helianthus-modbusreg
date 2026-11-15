package sunspec

import (
	"fmt"
	"sort"
	"sync/atomic"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

const maxSunSpecReadWords uint16 = 125

var sunSpecPlanNonce atomic.Uint64
var sunSpecChainInstance atomic.Uint64

// ChainLimits bounds one chain walk. MaxTotalWords counts every word of
// the raw chain image, signature and end marker included; MaxOccurrences
// counts complete model blocks. A plan requires MaxTotalWords from 4 to 65536
// and MaxOccurrences from 1 to MaxTotalWords/2.
type ChainLimits struct {
	MaxTotalWords  uint32
	MaxOccurrences uint32
}

// ChainPlanSpec is the input to NewChainPlan. BaseCandidates are
// distinct zero-based PDU register addresses probed for the signature.
// DecoderKeys are the exact keys to admit, all of SchemaRevision, usually
// DecoderRegistry.DecoderKeys. StructuralCandidateIDs opts model
// identifiers into post-payload structural validation. The plan copies every
// slice. See
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/sunspec/dynamic-structural-selection-v2.md.
type ChainPlanSpec struct {
	SchemaRevision         SchemaRevision
	BaseCandidates         []uint16
	Limits                 ChainLimits
	DecoderKeys            []DecoderKey
	StructuralCandidateIDs []uint16
}

// ChainPlan is an immutable, validated plan for walking a SunSpec
// chain. Each plan carries a process-unique nonce, so a chain never accepts a
// request of another plan. One plan may start any number of independent chains
// through NewChain.
type ChainPlan struct {
	revision               SchemaRevision
	bases                  []uint16
	limits                 ChainLimits
	keys                   map[WireKey]DecoderKey
	known                  map[uint16]struct{}
	structuralCandidateIDs map[uint16]struct{}
	nonce                  uint64
	initial                []ReadRequest
}

// ReadRequest is one planned FC03 read. Its fields are private so it
// cannot be forged or altered; a chain admits one view per request, and only
// for requests that its own NextRequests returned.
type ReadRequest struct {
	address, words            uint16
	purpose                   ReadPurpose
	nonce, instance, sequence uint64
}

// Function returns FunctionReadHoldingRegisters; a chain reads no other table.
func (r ReadRequest) Function() modbusreg.FunctionCode { return modbusreg.FunctionReadHoldingRegisters }

// Address returns the zero-based PDU register address of the read.
func (r ReadRequest) Address() uint16 { return r.address }

// WordCount returns the number of registers to read, 1 to 125.
func (r ReadRequest) WordCount() uint16 { return r.words }

// Purpose returns the chain step the read serves.
func (r ReadRequest) Purpose() ReadPurpose { return r.purpose }

// NewChainPlan validates s and returns a plan whose initial requests
// probe each base candidate, in order, for the two-word signature. It returns
// an error for an invalid revision, no base candidates, limits out of bounds,
// a duplicated base or one too close to the end of the address space, a
// decoder key of another revision or of zero length, a duplicated decoder wire
// key, or a duplicated structural candidate identifier.
func NewChainPlan(s ChainPlanSpec) (ChainPlan, error) {
	if !validSunSpecRevision(s.SchemaRevision) || len(s.BaseCandidates) == 0 || s.Limits.MaxTotalWords < 4 || s.Limits.MaxOccurrences == 0 || s.Limits.MaxTotalWords > 65536 || s.Limits.MaxOccurrences > s.Limits.MaxTotalWords/2 {
		return ChainPlan{}, fmt.Errorf("SunSpec chain plan requires explicit finite bounds")
	}
	p := ChainPlan{revision: s.SchemaRevision, bases: append([]uint16(nil), s.BaseCandidates...), limits: s.Limits, keys: map[WireKey]DecoderKey{}, known: map[uint16]struct{}{}, structuralCandidateIDs: map[uint16]struct{}{}, nonce: sunSpecPlanNonce.Add(1)}
	seenBases := make(map[uint16]struct{}, len(p.bases))
	for _, b := range p.bases {
		if _, duplicate := seenBases[b]; duplicate {
			return ChainPlan{}, fmt.Errorf("SunSpec base candidate duplicated")
		}
		if err := sunSpecEnd(b, 2); err != nil {
			return ChainPlan{}, err
		}
		seenBases[b] = struct{}{}
	}
	for _, k := range s.DecoderKeys {
		if k.SchemaRevision != s.SchemaRevision || k.ModelLength == 0 {
			return ChainPlan{}, fmt.Errorf("SunSpec decoder key is not exact")
		}
		wk := WireKey{k.ModelID, k.ModelLength}
		if _, ok := p.keys[wk]; ok {
			return ChainPlan{}, fmt.Errorf("SunSpec decoder key duplicated")
		}
		p.keys[wk] = k
		p.known[k.ModelID] = struct{}{}
	}
	for _, id := range s.StructuralCandidateIDs {
		if _, exists := p.structuralCandidateIDs[id]; exists {
			return ChainPlan{}, fmt.Errorf("SunSpec structural candidate identifier duplicated")
		}
		p.structuralCandidateIDs[id] = struct{}{}
	}
	for i, b := range p.bases {
		p.initial = append(p.initial, ReadRequest{address: b, words: 2, purpose: ReadSignature, nonce: p.nonce, sequence: uint64(i + 1)})
	}
	return p, nil
}

// Requests returns copies of the signature-probe requests in base order. They
// are not bound to a chain: Chain.AdmitReplay accepts only requests
// returned by that chain's NextRequests.
func (p ChainPlan) Requests() []ReadRequest {
	return append([]ReadRequest(nil), p.initial...)
}

// SchemaRevision returns the revision the plan admits decoder keys under.
func (p ChainPlan) SchemaRevision() SchemaRevision { return p.revision }
func (p ChainPlan) structuralCandidate(wireKey WireKey, words []uint16, spans []SourceSpan) *sunSpecStructuralCandidate {
	if _, enabled := p.structuralCandidateIDs[wireKey.ModelID]; !enabled {
		return nil
	}
	return sunSpecV2DERTripLVStructuralCandidate(p.revision, wireKey, words, spans)
}
func sortedRequests(m map[uint64]ReadRequest) []ReadRequest {
	ids := make([]uint64, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]ReadRequest, 0, len(ids))
	for _, id := range ids {
		out = append(out, m[id])
	}
	return out
}
