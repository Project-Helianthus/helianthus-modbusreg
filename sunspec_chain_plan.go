package modbusreg

import "fmt"

const maxSunSpecReadWords uint16 = 125

// SunSpecChainLimits are caller-supplied finite chain bounds.
type SunSpecChainLimits struct {
	MaxTotalWords  uint32
	MaxOccurrences uint32
}
type SunSpecChainPlanSpec struct {
	SchemaRevision SunSpecSchemaRevision
	BaseCandidates []uint16
	Limits         SunSpecChainLimits
}
type SunSpecChainPlan struct {
	revision SunSpecSchemaRevision
	bases    []uint16
	limits   SunSpecChainLimits
	nonce    uint64
	initial  []SunSpecReadRequest
}
type SunSpecReadRequest struct {
	address, words  uint16
	purpose         SunSpecReadPurpose
	nonce, sequence uint64
}

func (r SunSpecReadRequest) Function() FunctionCode      { return FunctionReadHoldingRegisters }
func (r SunSpecReadRequest) Address() uint16             { return r.address }
func (r SunSpecReadRequest) WordCount() uint16           { return r.words }
func (r SunSpecReadRequest) Purpose() SunSpecReadPurpose { return r.purpose }

func NewSunSpecChainPlan(spec SunSpecChainPlanSpec) (SunSpecChainPlan, error) {
	if !validSunSpecRevision(spec.SchemaRevision) || len(spec.BaseCandidates) == 0 || spec.Limits.MaxTotalWords < 4 || spec.Limits.MaxOccurrences == 0 {
		return SunSpecChainPlan{}, fmt.Errorf("SunSpec chain plan requires explicit revision, bases, and finite bounds")
	}
	if spec.Limits.MaxTotalWords > 65536 || spec.Limits.MaxOccurrences > spec.Limits.MaxTotalWords/2 {
		return SunSpecChainPlan{}, fmt.Errorf("SunSpec chain plan bounds are inconsistent")
	}
	bases := append([]uint16(nil), spec.BaseCandidates...)
	for _, base := range bases {
		if err := sunSpecEnd(base, 2); err != nil {
			return SunSpecChainPlan{}, err
		}
	}
	p := SunSpecChainPlan{revision: spec.SchemaRevision, bases: bases, limits: spec.Limits, nonce: 0x53554e5350454351}
	for i, base := range bases {
		p.initial = append(p.initial, SunSpecReadRequest{address: base, words: 2, purpose: SunSpecReadSignature, nonce: p.nonce, sequence: uint64(i + 1)})
	}
	return p, nil
}
func (p SunSpecChainPlan) Requests() []SunSpecReadRequest {
	return append([]SunSpecReadRequest(nil), p.initial...)
}
func (p SunSpecChainPlan) SchemaRevision() SunSpecSchemaRevision { return p.revision }
