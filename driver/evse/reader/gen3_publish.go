package reader

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

var evseRefPack = semreg.PackRef{ID: "helianthus.pack.evse", Version: "1.0.0"}

func defRef(id semreg.DefinitionID) semreg.DefinitionRef {
	return semreg.DefinitionRef{Pack: evseRefPack, ID: id, Version: "1.0.0"}
}

// Gen3Publication is the identity a host supplies for one configured charger.
// The dimension values are the configured subject labels; none is taken from
// an asset identifier, an index, a role string or an address.
type Gen3Publication struct {
	Asset      semreg.AssetID
	Source     semreg.SourceID
	Epoch      semreg.SourceEpochID
	Generation uint64
	ClockEpoch semreg.ClockEpochID
	EVSE       string
	Connector  string
	Policy     semreg.FreshnessPolicy
}

// Capability binds one read or setter capability to the service the pack names.
type Capability struct {
	Capability semreg.DefinitionID
	Service    semreg.DefinitionID
}

// The read bindings of the five subjects. The charger's five observations give
// the EVSE and the connector facts; the phase, session and meter bindings apply
// only where a subject exists for them.
var (
	ReadEVSE      = Capability{"evse.capability.read.evse", "evse.service.evse"}
	ReadConnector = Capability{"evse.capability.read.connector", "evse.service.connector"}
	ReadPhase     = Capability{"evse.capability.read.phase", "evse.service.phase"}
	ReadSession   = Capability{"evse.capability.read.session", "evse.service.session"}
	ReadMeter     = Capability{"evse.capability.read.meter", "evse.service.meter"}
	SetAllocated  = Capability{"evse.capability.set_allocated_current", "evse.service.connector"}
)

// Capabilities lists the capabilities published for a charger. The connector
// carries the setter capability only while the t25 operation is qualified.
func Capabilities(writable, meter bool) []Capability {
	out := []Capability{ReadEVSE, ReadConnector}
	if meter {
		out = append(out, ReadMeter)
	}
	if writable {
		out = append(out, SetAllocated)
	}
	return out
}

func digestOf(parts ...string) semreg.Digest {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return semreg.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func opaque(prefix string, parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return prefix + ":" + hex.EncodeToString(sum[:16])
}

func (p Gen3Publication) scope() []string {
	return []string{string(p.Asset), string(p.Source), string(p.Epoch), strconv.FormatUint(p.Generation, 10)}
}

func (p Gen3Publication) bindingID() semreg.NativeBindingID {
	return semreg.NativeBindingID(opaque("binding:tesla-wc3", p.scope()...))
}

func (p Gen3Publication) uint64(n uint64) semreg.Uint64 {
	return semreg.Uint64(strconv.FormatUint(n, 10))
}

func wall(t time.Time) semreg.TimePoint {
	return semreg.TimePoint{UnixNanoseconds: semreg.Int64(strconv.FormatInt(t.UTC().UnixNano(), 10)), ClockID: "wall.utc", UncertaintyNS: "0"}
}

func (p Gen3Publication) mono(d time.Duration) semreg.MonotonicPoint {
	return semreg.MonotonicPoint{ClockEpochID: p.ClockEpoch, Nanoseconds: semreg.Uint64(strconv.FormatInt(d.Nanoseconds(), 10))}
}

func (p Gen3Publication) subject(role Role) (semreg.DefinitionID, string) {
	if role == RoleConnector {
		return "evse.dimension.connector", p.Connector
	}
	return "evse.dimension.evse", p.EVSE
}

func (p Gen3Publication) factKey(element string, role Role) semreg.FactKey {
	dim, label := p.subject(role)
	text := label
	return semreg.FactKey{PackID: evseRefPack.ID, PackVersion: evseRefPack.Version, FactID: semreg.DefinitionID(element),
		Dimensions: []semreg.Dimension{{ID: dim, Value: semreg.Value{Kind: semreg.ValueText, Text: &text}}}}
}

func (p Gen3Publication) candidateID(element string, role Role) semreg.CandidateID {
	_, label := p.subject(role)
	return semreg.CandidateID(opaque("candidate:tesla-wc3", append(p.scope(), element, label)...))
}

func (p Gen3Publication) evidence(kind semreg.DefinitionID, parts ...string) semreg.EvidenceRef {
	return semreg.EvidenceRef{Owner: evseRefPack.ID, Kind: kind, Digest: digestOf(parts...), Contract: semreg.ContractKernelV1,
		Access: semreg.EvidenceAccessAuthorized, Redaction: semreg.RedactionMetadataOnly}
}

func evidenceKind(f Fact) semreg.DefinitionID {
	if f.Element == ElementConfiguredCurrent {
		return "native.tesla.wc3.current_limit.persistent"
	}
	return "native.tesla.wc3.current_limit.provisional"
}

func (p Gen3Publication) value(f Fact) semreg.Value {
	switch {
	case f.Element == ElementAllocatedExpiry:
		tp := wall(f.Time)
		return semreg.Value{Kind: semreg.ValueTime, Time: &tp}
	case f.Symbol != "":
		i := strings.LastIndex(f.Symbol, ".")
		return semreg.Value{Kind: semreg.ValueSymbol, Symbol: &semreg.Symbol{Namespace: semreg.DefinitionID(f.Symbol[:i]), Token: f.Symbol[i+1:], Known: true}}
	}
	return semreg.Value{Kind: semreg.ValueQuantity, Quantity: &semreg.Quantity{Number: semreg.Decimal{Coefficient: strconv.FormatUint(uint64(f.Amps), 10)}, Unit: "unit.ampere"}}
}

func (p Gen3Publication) candidate(f Fact, evaluated At, revision uint64) semreg.FactCandidate {
	binding, epoch, generation := p.bindingID(), p.Epoch, p.uint64(p.Generation)
	source := p.Source
	ev := p.evidence(evidenceKind(f), string(p.Asset), f.Element, strconv.FormatUint(f.Receipt.Lifecycle, 10), strconv.FormatUint(f.Receipt.Generation, 10), strconv.FormatUint(f.Receipt.RequestID, 10))
	value := p.value(f)
	key := p.factKey(f.Element, f.Role)
	id := p.candidateID(f.Element, f.Role)
	return semreg.FactCandidate{
		CandidateID: id, Key: key, Value: &value,
		Quality: semreg.Quality{Assertion: semreg.AssertionObserved, Qualification: semreg.QualificationQualified, Promotion: semreg.PromotionPromoted,
			Validity: semreg.ValidityGood, Availability: semreg.AvailabilityAvailable, Freshness: semreg.FreshnessFresh, Reasons: []semreg.DefinitionID{}},
		Times: semreg.Times{ReceivedAt: wall(f.Receipt.Wall), ReceiptMonotonic: p.mono(f.Receipt.Mono),
			EvaluatedAt: wall(evaluated.Wall), EvaluateMonotonic: p.mono(evaluated.Mono)},
		FreshnessPolicy: p.Policy, BindingID: &binding, SourceEpochID: &epoch, DriverGeneration: &generation,
		Origin: semreg.OriginRef{OriginID: semreg.OriginID(opaque("origin:tesla-wc3", string(id), strconv.FormatUint(revision, 10))), Kind: semreg.OriginNativeObservation,
			SourceID: &source, SourceEpochID: &epoch, BindingID: &binding, Evidence: []semreg.EvidenceRef{ev}},
		Evidence: []semreg.EvidenceRef{ev}, Revision: p.uint64(revision),
	}
}

func (p Gen3Publication) binding(version string, node byte, endpoint string) semreg.NativeBinding {
	return semreg.NativeBinding{BindingID: p.bindingID(), AssetID: p.Asset, SourceID: p.Source, SourceEpochID: p.Epoch,
		DriverGeneration: p.uint64(p.Generation),
		NativeResource:   p.evidence(semreg.DefinitionID("registry.tesla.wc3_"+strings.ReplaceAll(version, ".", "_")), endpoint, version, strconv.Itoa(int(node))),
		State:            semreg.BindingCurrent, Revision: "1"}
}

func (p Gen3Publication) service(c Capability) semreg.ServiceInstance {
	return semreg.ServiceInstance{
		InstanceID: semreg.ServiceInstanceID(opaque("service:tesla-wc3", append(p.scope(), string(c.Service))...)),
		AssetID:    p.Asset, Definition: defRef(c.Service), BindingID: p.bindingID(), SourceEpochID: p.Epoch,
		DriverGeneration: p.uint64(p.Generation), Qualification: semreg.QualificationQualified, Availability: semreg.AvailabilityAvailable, Revision: "1",
	}
}

func (p Gen3Publication) capability(c Capability, service semreg.ServiceInstanceID) semreg.CapabilityInstance {
	return semreg.CapabilityInstance{
		InstanceID: semreg.CapabilityInstanceID(opaque("capability:tesla-wc3", append(p.scope(), string(c.Capability))...)),
		AssetID:    p.Asset, ServiceInstance: service, Definition: defRef(c.Capability), BindingID: p.bindingID(), SourceEpochID: p.Epoch,
		DriverGeneration: p.uint64(p.Generation), Qualification: semreg.QualificationQualified, Availability: semreg.AvailabilityAvailable,
		Constraints:        []semreg.TypedField{},
		ActivationEvidence: []semreg.EvidenceRef{p.evidence("native.tesla.wc3.capability", string(p.Asset), string(c.Capability))},
		Revision:           "1",
	}
}
