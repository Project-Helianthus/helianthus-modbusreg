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

// sunSpecDERTripStructuralCandidate is the model identifier a V2 plan opts
// into post-payload structural validation. The occurrence stays an unknown
// model; the existing projection reads the retained candidate.
const sunSpecDERTripStructuralCandidate uint16 = 707

// ChainUnitKind is the configured role of one allowlisted unit.
type ChainUnitKind string

// Allowlisted unit roles. An inverter and a meter on one endpoint are read
// as separate chains.
const (
	ChainUnitInverter ChainUnitKind = "inverter"
	ChainUnitMeter    ChainUnitKind = "meter"
)

// AllowlistedUnit is one configured unit whose SunSpec chain is read.
// ListedModels are the model identifiers the chain lists; an empty list
// selects the whole V1 registry.
type AllowlistedUnit struct {
	UnitID       byte
	Kind         ChainUnitKind
	ListedModels []uint16
}

// UnitAllowlist is the configured set of units read on one endpoint.
type UnitAllowlist struct {
	Endpoint       string
	Units          []AllowlistedUnit
	BaseCandidates []uint16
	Limits         ChainLimits
}

// UnitChain is one allowlisted unit's chain walk and its own plan state.
type UnitChain struct {
	unitID   byte
	kind     ChainUnitKind
	endpoint string
	chain    *Chain
}

// UnitID returns the configured unit identifier.
func (u UnitChain) UnitID() byte { return u.unitID }

// Kind returns the configured role of the unit.
func (u UnitChain) Kind() ChainUnitKind { return u.kind }

// Endpoint returns the endpoint the unit is read on.
func (u UnitChain) Endpoint() string { return u.endpoint }

// Plan returns the chain plan that walks this unit.
func (u UnitChain) Plan() ChainPlan {
	if u.chain == nil {
		return ChainPlan{}
	}
	return u.chain.plan
}

// NextRequests returns the outstanding reads of this unit's chain.
func (u UnitChain) NextRequests() []ReadRequest {
	if u.chain == nil {
		return nil
	}
	return u.chain.NextRequests()
}

// AdmitReplay admits the logical view that answered request on this unit's
// chain. A view for another unit or endpoint is rejected and leaves the walk
// unchanged.
func (u UnitChain) AdmitReplay(r ReadRequest, v modbusreg.LogicalViewSnapshot) (ChainSnapshot, error) {
	if u.chain == nil {
		return ChainSnapshot{}, fmt.Errorf("SunSpec chain is unavailable")
	}
	return u.chain.AdmitReplay(r, v)
}

// AllowlistPlan holds one independent chain per allowlisted unit.
type AllowlistPlan struct {
	units []UnitChain
}

// Units returns the planned chains in allowlist order.
func (p AllowlistPlan) Units() []UnitChain {
	return append([]UnitChain(nil), p.units...)
}

// PlanAllowlistedUnits plans one chain walk per configured unit, a meter unit
// on the same endpoint included. Each chain has its own plan state. The plan
// for a unit uses NewListedChainPlan on that unit's listed models.
func PlanAllowlistedUnits(list UnitAllowlist) (AllowlistPlan, error) {
	if err := validateUnitAllowlist(list); err != nil {
		return AllowlistPlan{}, err
	}
	units := make([]UnitChain, 0, len(list.Units))
	for _, unit := range list.Units {
		plan, err := NewListedChainPlan(unit.ListedModels, list.BaseCandidates, list.Limits)
		if err != nil {
			return AllowlistPlan{}, err
		}
		chain := NewChain(plan)
		chain.bindAllowlistedUnit(list.Endpoint, unit.UnitID)
		units = append(units, UnitChain{unitID: unit.UnitID, kind: unit.Kind, endpoint: list.Endpoint, chain: chain})
	}
	return AllowlistPlan{units: units}, nil
}

func validateUnitAllowlist(list UnitAllowlist) error {
	if list.Endpoint == "" || len(list.Units) == 0 {
		return fmt.Errorf("SunSpec allowlist requires an endpoint and a unit")
	}
	seen := make(map[byte]struct{}, len(list.Units))
	for _, unit := range list.Units {
		if unit.UnitID == 0 || unit.UnitID > 247 || (unit.Kind != ChainUnitInverter && unit.Kind != ChainUnitMeter) {
			return fmt.Errorf("SunSpec allowlisted unit is invalid")
		}
		if _, duplicate := seen[unit.UnitID]; duplicate {
			return fmt.Errorf("SunSpec allowlisted unit is duplicated")
		}
		seen[unit.UnitID] = struct{}{}
	}
	return nil
}

// NewListedChainPlan builds the read plan for a chain that lists modelIDs.
// An identifier in the 700s or 800s selects the V2 registry and the model 707
// structural-candidate selection. Every other chain uses the V1 registry.
// Decoder keys are the registry keys of the listed identifiers. An empty list
// carries the whole V1 registry. A listed model 120 keeps its V1 nameplate key.
func NewListedChainPlan(modelIDs, bases []uint16, limits ChainLimits) (ChainPlan, error) {
	revision := sunSpecChainRevision(modelIDs)
	registry, err := NewStandardSunSpecDecoderRegistry(revision)
	if err != nil {
		return ChainPlan{}, err
	}
	keys := sunSpecListedDecoderKeys(registry, modelIDs)
	if err := sunSpecRequireListedNameplate(keys, modelIDs, revision); err != nil {
		return ChainPlan{}, err
	}
	spec := ChainPlanSpec{SchemaRevision: revision, BaseCandidates: bases, Limits: limits, DecoderKeys: keys}
	if revision == ModelsRevisionV2 {
		spec.StructuralCandidateIDs = []uint16{sunSpecDERTripStructuralCandidate}
	}
	return NewChainPlan(spec)
}

func sunSpecChainRevision(modelIDs []uint16) SchemaRevision {
	for _, id := range modelIDs {
		if id >= 700 && id <= 899 {
			return ModelsRevisionV2
		}
	}
	return ModelsRevisionV1
}

func sunSpecListedDecoderKeys(registry DecoderRegistry, modelIDs []uint16) []DecoderKey {
	if len(modelIDs) == 0 {
		return registry.DecoderKeys()
	}
	listed := make(map[uint16]struct{}, len(modelIDs))
	for _, id := range modelIDs {
		listed[id] = struct{}{}
	}
	keys := make([]DecoderKey, 0, len(modelIDs))
	for _, key := range registry.DecoderKeys() {
		if _, ok := listed[key.ModelID]; ok {
			keys = append(keys, key)
		}
	}
	return keys
}

func sunSpecRequireListedNameplate(keys []DecoderKey, modelIDs []uint16, revision SchemaRevision) error {
	if revision != ModelsRevisionV1 || !sunSpecModelListed(modelIDs, 120) {
		return nil
	}
	for _, key := range keys {
		if key.ModelID == 120 && key.ModelLength == 26 && key.SchemaRevision == revision {
			return nil
		}
	}
	return fmt.Errorf("SunSpec nameplate model is absent from the read plan")
}

func sunSpecModelListed(modelIDs []uint16, modelID uint16) bool {
	for _, id := range modelIDs {
		if id == modelID {
			return true
		}
	}
	return false
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
