package meter

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/roweval"
)

type element struct {
	id   semreg.DefinitionID
	dim  semreg.DefinitionID
	unit semreg.DefinitionID
}

type service struct{ service, capability semreg.DefinitionID }

var services = map[semreg.DefinitionID]service{
	dimSite:           {"infrastructure.service.site", "infrastructure.capability.read.site"},
	dimGridConnection: {"infrastructure.service.grid_connection", "infrastructure.capability.read.grid_connection"},
	dimMeter:          {"infrastructure.service.meter", "infrastructure.capability.read.meter"},
	dimPhase:          {"infrastructure.service.phase", "infrastructure.capability.read.phase"},
	dimLinePair:       {"infrastructure.service.line_pair", ""},
	dimLoad:           {"infrastructure.service.load", ""},
}

type derivation struct {
	inputs []string
}

// source says how a value came to be.
type source struct {
	path     string
	native   Native
	retained bool
	derived  *derivation
}

var errNoSourcePath = errors.New("the candidate template carries no native source path")

func (r *run) subject(node driver.NodeRef, dim semreg.DefinitionID) roweval.Subject {
	s := services[dim]
	instance := semreg.CapabilityInstanceID("capability." + string(node.ID) + "." + string(dim))
	sub := roweval.Subject{Role: string(dim), Service: infra(s.service), ServiceInstance: semreg.ServiceInstanceID("service." + string(node.ID) + "." + string(dim))}
	if s.capability != "" {
		sub.Capability, sub.CapabilityInstance = infra(s.capability), &instance
	}
	return sub
}

func candidateID(node driver.NodeRef, key semreg.FactKey, path string) semreg.CandidateID {
	canonical, _ := semreg.CanonicalJSON(key)
	return semreg.CandidateID("candidate." + hex.EncodeToString([]byte(digestOf(string(node.ID), string(canonical), path))[7:23]))
}

func text(s string) semreg.Value { return semreg.Value{Kind: semreg.ValueText, Text: &s} }

// emit publishes one fact, or records the reason it is withheld. dimText names
// the subject of the dimension.
func (r *run) emit(node driver.NodeRef, el element, dimText string, value semreg.Value, src source) (semreg.CandidateID, error) {
	key := semreg.FactKey{PackID: infraID, PackVersion: infraVersion, FactID: el.id, Dimensions: []semreg.Dimension{{ID: el.dim, Value: text(dimText)}}}
	c := r.in.Template
	c.CandidateID, c.Key, c.Value = candidateID(node, key, src.path), key, &value
	c.FreshnessPolicy = r.p.cfg.Freshness
	native := evidence("evidence.native", src.path, src.native.Unit, src.native.Value.Coefficient, fmt.Sprint(src.native.Value.Exponent10))
	c.Evidence = withEvidence(c.Evidence, native)
	c.Quality.Reasons = []semreg.DefinitionID{}
	kind := driver.OriginMeasured
	if src.retained {
		c.Quality.Availability = semreg.AvailabilityDegraded
	}
	if src.derived != nil {
		kind = driver.OriginDerived
		if err := r.derive(&c, src, native); err != nil {
			return "", err
		}
	}
	if err := c.Validate(); err != nil {
		return "", err
	}
	r.out.Facts = append(r.out.Facts, Fact{Node: node, Subject: r.subject(node, el.dim), Origin: kind, Path: src.path, Native: src.native, Candidate: c})
	return c.CandidateID, nil
}

func (r *run) derive(c *semreg.FactCandidate, src source, native semreg.EvidenceRef) error {
	if c.BindingID == nil || c.SourceEpochID == nil || c.DriverGeneration == nil || c.Origin.SourceID == nil {
		return errNoSourcePath
	}
	path := semreg.SourcePathRef{BindingID: *c.BindingID, SourceID: *c.Origin.SourceID, SourceEpochID: *c.SourceEpochID, DriverGeneration: *c.DriverGeneration}
	d := &semreg.Derivation{Algorithm: derivationID, Version: derivationVersion, Evidence: []semreg.EvidenceRef{native}}
	for _, id := range src.derived.inputs {
		for _, f := range r.out.Facts {
			if string(f.Candidate.CandidateID) == id {
				d.Inputs = append(d.Inputs, semreg.DerivationInput{CandidateID: f.Candidate.CandidateID, CandidateRevision: f.Candidate.Revision, SourcePaths: []semreg.SourcePathRef{path}})
			}
		}
	}
	if len(d.Inputs) != len(src.derived.inputs) {
		return fmt.Errorf("a derived value needs every input published")
	}
	sort.Slice(d.Inputs, func(i, j int) bool { return d.Inputs[i].CandidateID < d.Inputs[j].CandidateID })
	c.Quality.Assertion = semreg.AssertionInferred
	c.BindingID, c.SourceEpochID, c.DriverGeneration = nil, nil, nil
	c.Origin = semreg.OriginRef{OriginID: semreg.OriginID("origin." + string(c.CandidateID)), Kind: semreg.OriginDerived, Evidence: c.Evidence}
	c.Derivation = d
	return nil
}

func evidenceKey(e semreg.EvidenceRef) string {
	return string(e.Owner) + "\x00" + string(e.Kind) + "\x00" + string(e.Contract) + "\x00" + string(e.Digest)
}

func withEvidence(base []semreg.EvidenceRef, more ...semreg.EvidenceRef) []semreg.EvidenceRef {
	out := append(append([]semreg.EvidenceRef(nil), base...), more...)
	sort.Slice(out, func(i, j int) bool { return evidenceKey(out[i]) < evidenceKey(out[j]) })
	uniq := out[:0]
	for i, e := range out {
		if i == 0 || evidenceKey(e) != evidenceKey(out[i-1]) {
			uniq = append(uniq, e)
		}
	}
	return uniq
}

// unknown records an element whose value cannot be stated.
func (r *run) unknown(node driver.NodeRef, el element, dimText, reason string) {
	key := semreg.FactKey{PackID: infraID, PackVersion: infraVersion, FactID: el.id, Dimensions: []semreg.Dimension{{ID: el.dim, Value: text(dimText)}}}
	r.out.Unknown = append(r.out.Unknown, Unknown{Node: node, Key: key, Reason: reason})
}

func (r *run) member(node, local string, n Native) {
	r.out.Members = append(r.out.Members, Member{
		Key:    driver.MemberKey{Namespace: namespaceKey, Instance: r.p.cfg.Instance, Node: driver.NodeID(node), Local: driver.MemberID(local)},
		Native: n,
	})
}
