package pv

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"

	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/roweval"
)

type service struct{ service, capability semreg.DefinitionID }

var pvServices = map[semreg.DefinitionID]service{
	dimSystem:   {"pv.service.system", "pv.capability.read.system"},
	dimInverter: {"pv.service.inverter", "pv.capability.read.inverter"},
	dimPhase:    {"pv.service.phase", "pv.capability.read.phase"},
	dimInput:    {"pv.service.input", "pv.capability.read.input"},
}

func (r *run) subject(node driver.NodeRef, el element) roweval.Subject {
	if el.pack == infraID {
		return roweval.Subject{Role: string(el.dim), Service: infraDef("infrastructure.service.line_pair"), ServiceInstance: semreg.ServiceInstanceID("service." + string(node.ID) + "." + string(el.dim))}
	}
	s := pvServices[el.dim]
	instance := semreg.CapabilityInstanceID("capability." + string(node.ID) + "." + string(el.dim))
	return roweval.Subject{Role: string(el.dim), Service: pvDef(s.service), Capability: pvDef(s.capability), CapabilityInstance: &instance,
		ServiceInstance: semreg.ServiceInstanceID("service." + string(node.ID) + "." + string(el.dim))}
}

func candidateID(node driver.NodeRef, key semreg.FactKey, path string) semreg.CandidateID {
	canonical, _ := semreg.CanonicalJSON(key)
	return semreg.CandidateID("candidate." + hex.EncodeToString([]byte(digestOf(string(node.ID), string(canonical), path))[7:23]))
}

func text(s string) semreg.Value { return semreg.Value{Kind: semreg.ValueText, Text: &s} }

func symbol(el element, token string) semreg.Value {
	return semreg.Value{Kind: semreg.ValueSymbol, Symbol: &semreg.Symbol{Namespace: el.id, Token: token, Known: true}}
}

// source says where a value came from: the path of its native reading, the
// native value and unit, and whether the reading is a retained last-known-good.
type source struct {
	path     string
	native   Native
	retained bool
}

// emit publishes one fact. dimText names the subject of the dimension.
func (r *run) emit(node driver.NodeRef, el element, dimText string, value semreg.Value, src source) error {
	key := semreg.FactKey{PackID: el.pack, PackVersion: packVer, FactID: el.id, Dimensions: []semreg.Dimension{{ID: el.dim, Value: text(dimText)}}}
	c := r.in.Template
	c.CandidateID, c.Key, c.Value = candidateID(node, key, src.path), key, &value
	c.FreshnessPolicy = r.p.cfg.Freshness
	native := evidence("evidence.native", src.path, src.native.Unit, src.native.Value.Coefficient, fmt.Sprint(src.native.Value.Exponent10))
	c.Evidence = withEvidence(c.Evidence, native)
	c.Quality.Reasons = []semreg.DefinitionID{}
	if src.retained {
		c.Quality.Availability = semreg.AvailabilityDegraded
	}
	if err := c.Validate(); err != nil {
		return err
	}
	r.out.Facts = append(r.out.Facts, Fact{Node: node, Subject: r.subject(node, el), Origin: driver.OriginMeasured, Path: src.path, Native: src.native, Candidate: c})
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

// publish converts a native value with its row and publishes it on the subject
// the row names. dim is the text of the dimension.
func (r *run) publish(node driver.NodeRef, rw row, dim string, value *big.Rat, src source) error {
	v, err := quantity(rw.convert(value), rw.el.unit)
	if err != nil {
		return err
	}
	return r.emit(node, rw.el, dim, v, src)
}

func (r *run) member(node, local string, p Point) {
	r.out.Members = append(r.out.Members, Member{
		Key:   driver.MemberKey{Namespace: namespaceKey, Instance: r.p.cfg.Instance, Node: driver.NodeID(node), Local: driver.MemberID(local)},
		Point: p,
	})
}

func joinPath(parts ...string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "."
		}
		out += p
	}
	return out
}
