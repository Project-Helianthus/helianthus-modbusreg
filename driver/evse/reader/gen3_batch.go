package reader

import (
	"sort"
	"strconv"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

func batchEmpty(b semreg.PublicationBatch) bool {
	return len(b.BindingUpserts) == 0 && len(b.IdentityLinkUpserts) == 0 && len(b.FactUpserts) == 0 && len(b.FactWithdrawals) == 0 &&
		len(b.ServiceUpserts) == 0 && len(b.CapabilityUpserts) == 0
}

// batch stages the difference between the view and what is published: the
// binding, services and capabilities on the first batch, a candidate for every
// fact that is new or changed, and a withdrawal for every fact that is gone.
func (r *Gen3Reader) batch(view View, at At) (semreg.PublicationBatch, map[semreg.CandidateID]uint64, []Fact) {
	p := r.pub
	b := semreg.PublicationBatch{
		Contract: semreg.ContractKernelV1,
		BatchID:  semreg.BatchID(opaque("batch:tesla-wc3", append(p.scope(), strconv.FormatUint(r.seq+1, 10))...)),
		AssetID:  p.Asset, SourceID: p.Source, SourceEpochID: p.Epoch, DriverGeneration: p.uint64(p.Generation),
		Sequence: p.uint64(r.seq + 1), ExpectedSemanticRevision: p.uint64(r.semantic), ObservedAt: wall(at.Wall),
		SourceUpserts: []semreg.SourceDescriptor{}, SourceRetirements: []semreg.SourceEpochID{}, BindingUpserts: []semreg.NativeBinding{},
		IdentityLinkUpserts: []semreg.IdentityLink{}, FactUpserts: []semreg.FactCandidate{}, FactWithdrawals: []semreg.CandidateID{},
		ServiceUpserts: []semreg.ServiceInstance{}, ServiceWithdrawals: []semreg.ServiceInstanceID{},
		CapabilityUpserts: []semreg.CapabilityInstance{}, CapabilityWithdrawals: []semreg.CapabilityInstanceID{},
		GenerationFences: []semreg.GenerationFence{},
	}
	if r.seq == 0 {
		b.BindingUpserts = append(b.BindingUpserts, p.binding(r.cfg.Version, r.cfg.Node, r.cfg.Endpoint))
		services := map[semreg.DefinitionID]semreg.ServiceInstanceID{}
		for _, c := range Capabilities(r.cfg.CurrentLimitWritable(), false) {
			service, ok := services[c.Service]
			if !ok {
				s := p.service(c)
				b.ServiceUpserts = append(b.ServiceUpserts, s)
				service = s.InstanceID
				services[c.Service] = service
			}
			b.CapabilityUpserts = append(b.CapabilityUpserts, p.capability(c, service))
		}
	}
	// A withheld or unresolved element is published as a candidate without a
	// value that states its reason, so the cut can tell expiry, a persistent-
	// only withdrawal and an explicit zero apart.
	items := append([]Fact(nil), view.Facts...)
	for _, d := range view.Dispositions {
		items = append(items, Fact{Element: d.Element, Role: d.Role, Receipt: d.Receipt, Reason: d.Reason})
	}
	for _, f := range items {
		if f.Receipt.Mono > at.Mono {
			at.Mono = f.Receipt.Mono
		}
	}
	revisions := map[semreg.CandidateID]uint64{}
	for id, rev := range r.revisions {
		revisions[id] = rev
	}
	present := map[semreg.CandidateID]bool{}
	for _, f := range items {
		id := p.candidateID(f.Element, f.Role)
		present[id] = true
		if prior, ok := r.published[id]; ok && prior.fact == f {
			continue
		}
		revisions[id]++
		b.FactUpserts = append(b.FactUpserts, p.candidate(f, at, revisions[id]))
	}
	for id := range r.published {
		if !present[id] {
			b.FactWithdrawals = append(b.FactWithdrawals, id)
		}
	}
	sortWithdrawals(b.FactWithdrawals)
	sort.Slice(b.FactUpserts, func(i, j int) bool { return b.FactUpserts[i].CandidateID < b.FactUpserts[j].CandidateID })
	sort.Slice(b.ServiceUpserts, func(i, j int) bool { return b.ServiceUpserts[i].InstanceID < b.ServiceUpserts[j].InstanceID })
	sort.Slice(b.CapabilityUpserts, func(i, j int) bool { return b.CapabilityUpserts[i].InstanceID < b.CapabilityUpserts[j].InstanceID })
	return b, revisions, items
}

func sortWithdrawals(ids []semreg.CandidateID) {
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
}
