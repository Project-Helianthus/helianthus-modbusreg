package limits

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

const evidenceOwner semreg.DefinitionID = "modbus.sunspec.limit"

const (
	kindRequest semreg.DefinitionID = "native.sunspec.limit.request"
	kindAck     semreg.DefinitionID = "native.sunspec.limit.acknowledgement"
	kindRead    semreg.DefinitionID = "native.sunspec.limit.readback"
	kindRelease semreg.DefinitionID = "native.sunspec.limit.release"
	kindRefusal semreg.DefinitionID = "native.sunspec.limit.refusal"
)

// Moment is one wall time and the monotonic tick beside it.
type Moment struct {
	Wall semreg.TimePoint
	Mono semreg.MonotonicPoint
}

func contextOf(m Moment) semreg.EvaluationContext {
	return semreg.EvaluationContext{EvaluatedAt: m.Wall, EvaluateMonotonic: m.Mono}
}

func wallNow(t time.Time) semreg.TimePoint {
	return semreg.TimePoint{UnixNanoseconds: semreg.Int64(strconv.FormatInt(t.UTC().UnixNano(), 10)), ClockID: "clock.utc", UncertaintyNS: "0"}
}

// Evidence names material by its digest. The digest covers the material bytes.
func Evidence(contract semreg.ContractVersion, kind semreg.DefinitionID, material string) semreg.EvidenceRef {
	sum := sha256.Sum256([]byte(material))
	return semreg.EvidenceRef{Owner: evidenceOwner, Kind: kind, Digest: semreg.Digest("sha256:" + hex.EncodeToString(sum[:])),
		Contract: contract, Access: semreg.EvidenceAccessPublic, Redaction: semreg.RedactionMetadataOnly}
}

func ordered(refs ...semreg.EvidenceRef) []semreg.EvidenceRef {
	out := make([]semreg.EvidenceRef, 0, len(refs))
	seen := map[semreg.EvidenceRef]bool{}
	for _, ref := range refs {
		if ref.Digest != "" && !seen[ref] {
			seen[ref] = true
			out = append(out, ref)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Owner != out[j].Owner {
			return out[i].Owner < out[j].Owner
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Contract != out[j].Contract {
			return out[i].Contract < out[j].Contract
		}
		return out[i].Digest < out[j].Digest
	})
	return out
}

// NotSent is a refusal that put nothing on the wire.
func NotSent(attempt semreg.AttemptID, at Moment, point, reason string) driver.Completion {
	ev := Evidence(operation.ContractOperationV1, kindRefusal, "refusal:"+reason)
	return driver.Completion{
		Outcome: operation.OutcomeFailedNoContact,
		Dispatch: &operation.DispatchEvidence{AttemptID: attempt, Started: contextOf(at),
			Delivery: operation.DeliveryNotSent, Evidence: ordered(ev)},
		OutcomeEvidence:   ordered(ev),
		CompoundExecution: &driver.CompoundExecution{Steps: []driver.CompoundStep{{Point: point, Attempted: false, Reason: reason}}},
	}
}

// Acknowledged is one accepted write with no confirming readback.
func Acknowledged(attempt semreg.AttemptID, started, done Moment, point string, state operation.AckState, ev semreg.EvidenceRef) driver.Completion {
	completed := contextOf(done)
	return driver.Completion{
		Outcome: operation.OutcomeAcknowledgedUnverified,
		Dispatch: &operation.DispatchEvidence{AttemptID: attempt, Started: contextOf(started), Completed: &completed,
			Delivery: operation.DeliverySent, PossibleSideEffect: true, Evidence: ordered(ev)},
		Acknowledgement:   &operation.Acknowledgement{State: state, At: done.Wall, Evidence: ordered(ev)},
		OutcomeEvidence:   ordered(ev),
		CompoundExecution: &driver.CompoundExecution{Steps: []driver.CompoundStep{{Point: point, Attempted: true, Delivery: operation.DeliverySent, Acknowledgement: state}}},
	}
}

// Unknown is a possible transmission with no echo and no retry.
func Unknown(attempt semreg.AttemptID, started Moment, point string, ev semreg.EvidenceRef) driver.Completion {
	return driver.Completion{
		Outcome: operation.OutcomeIndeterminate,
		Dispatch: &operation.DispatchEvidence{AttemptID: attempt, Started: contextOf(started),
			Delivery: operation.DeliveryUnknown, PossibleSideEffect: true, Evidence: ordered(ev)},
		OutcomeEvidence:   ordered(ev),
		CompoundExecution: &driver.CompoundExecution{Steps: []driver.CompoundStep{{Point: point, Attempted: true, Delivery: operation.DeliveryUnknown}}},
	}
}

// Note appends native evidence and does not set a readback.
func Note(c driver.Completion, ev semreg.EvidenceRef) driver.Completion {
	c.OutcomeEvidence = ordered(append(append([]semreg.EvidenceRef{}, c.OutcomeEvidence...), ev)...)
	return c
}

// WithReadback adds the second attempted step and the later snapshot.
// Two attempted steps keep a confirming readback from being reported as acknowledgement only.
func WithReadback(c driver.Completion, snap semreg.Snapshot, candidate semreg.FactCandidate, route operation.Route, relation operation.ReadbackRelation, ev semreg.EvidenceRef) driver.Completion {
	c.ReadbackSnapshot = &snap
	c.Readback = &operation.Readback{
		SnapshotID: snap.SnapshotID, Revisions: snap.Revisions, CandidateID: candidate.CandidateID, CandidateRevision: candidate.Revision,
		BindingID: route.BindingID, SourceID: route.SourceID, SourceEpochID: route.SourceEpochID, DriverGeneration: route.DriverGeneration,
		Relation: relation, Evaluation: semreg.EvaluationContext{EvaluatedAt: snap.EvaluatedAt, EvaluateMonotonic: snap.EvaluateMonotonic},
		Evidence: ordered(ev),
	}
	switch relation {
	case operation.ReadbackConfirms:
		c.Outcome = operation.OutcomeApplied
	case operation.ReadbackContradicts:
		c.Outcome = operation.OutcomeConflict
	}
	if c.CompoundExecution == nil {
		c.CompoundExecution = &driver.CompoundExecution{}
	}
	c.CompoundExecution.Steps = append(c.CompoundExecution.Steps, driver.CompoundStep{Point: "readback", Attempted: true, ReadbackRelation: relation})
	c.OutcomeEvidence = ordered(append(append([]semreg.EvidenceRef{}, c.OutcomeEvidence...), ev)...)
	return c
}

func logicalView(inv Inverter, id uint64, address uint16, words []uint16) (modbusreg.LogicalViewSnapshot, error) {
	if len(words) == 0 || len(words) > 125 {
		return modbusreg.LogicalViewSnapshot{}, errors.New("sunspec read width")
	}
	count := uint16(len(words))
	return modbusreg.NewLogicalViewSnapshot(modbusreg.LogicalViewRecord{
		LogicalViewID: id, WireResponseID: id + 1000, PhysicalRequestID: id + 2000, Endpoint: "sunspec.inverter", ConnectionID: 1,
		Transport: modbusreg.TransportTCP, TransportGeneration: 1, UnitID: inv.Unit, RequestedFunction: modbusreg.FunctionReadHoldingRegisters,
		ReceivedFunction: modbusreg.FunctionReadHoldingRegisters, Table: modbusreg.HoldingRegisters, PhysicalOffset: address, PhysicalWordCount: count,
		AuthorizationScope: "sunspec-limit-read", PollGeneration: 1, DeadlineIdentity: 1, LogicalOffset: address, LogicalWordCount: count, SliceWordCount: count,
		Words: words, WireResponseBytes: []byte{3, byte(count)},
	})
}

func guardMatches(d driver.Dispatch, own owner) bool {
	g, r := d.Guard, own.route.Route
	return d.Route == r && g.AssetID == own.asset && g.BindingID == r.BindingID && g.SourceID == r.SourceID && g.SourceEpochID == r.SourceEpochID &&
		g.DriverGeneration == r.DriverGeneration && g.CapabilityInstance == r.CapabilityInstance && g.ServiceInstance == r.ServiceInstance
}

func classify(d driver.Dispatch) (release bool, watts semreg.Decimal, reason string) {
	args := d.Request.Arguments
	if len(args) == 0 {
		return false, semreg.Decimal{}, "arguments"
	}
	var quantity *semreg.Quantity
	cleared := false
	for _, arg := range args {
		if arg.ID != argID {
			continue
		}
		switch arg.Value.Kind {
		case semreg.ValueBoolean:
			if arg.Value.Boolean == nil {
				return false, semreg.Decimal{}, "arguments"
			}
			if *arg.Value.Boolean {
				return false, semreg.Decimal{}, "release_true"
			}
			cleared = true
		case semreg.ValueQuantity:
			quantity = arg.Value.Quantity
		default:
			return false, semreg.Decimal{}, "arguments"
		}
	}
	if cleared && (len(args) != 1 || quantity != nil) {
		return false, semreg.Decimal{}, "mixed_release"
	}
	if cleared {
		return true, semreg.Decimal{}, ""
	}
	if len(args) != 1 || quantity == nil || quantity.Unit != wattUnit {
		return false, semreg.Decimal{}, "arguments"
	}
	if strings.HasPrefix(quantity.Number.Coefficient, "-") {
		return false, semreg.Decimal{}, "negative_set"
	}
	return false, quantity.Number, ""
}

func mapReason(err error) string {
	var refusal *sunspec.ControlWriteError
	if !errors.As(err, &refusal) {
		return "arguments"
	}
	switch refusal.Reason {
	case sunspec.ControlReasonModelAbsent:
		return "model_absent"
	case sunspec.ControlReasonBoundAbsent:
		return "bound_absent"
	case sunspec.ControlReasonArgumentOutOfBound:
		return "out_of_bound"
	default:
		return "arguments"
	}
}

func (f *SunSpecPv) tick(d driver.Dispatch) Moment {
	f.seq++
	wall := d.AdmittedAt
	if wall.ClockID == "" {
		wall = wallNow(time.Now().UTC())
	}
	return Moment{Wall: wall, Mono: semreg.MonotonicPoint{ClockEpochID: monoEpoch, Nanoseconds: semreg.Uint64(strconv.FormatUint(f.seq, 10))}}
}

func (f *SunSpecPv) attempt() semreg.AttemptID {
	f.seq++
	sum := sha256.Sum256([]byte("attempt:" + strconv.FormatUint(f.seq, 10)))
	return semreg.AttemptID("attempt:sunspec-limit:" + hex.EncodeToString(sum[:4]))
}

// batchStamp is the session ownership a publication must carry before its digest
// is sealed. The session stamps the same ownership again when it accepts the batch.
type batchStamp struct {
	asset      semreg.AssetID
	source     semreg.SourceID
	epoch      semreg.SourceEpochID
	generation semreg.Uint64
}

func fillPublication(b *semreg.PublicationBatch, ready []prepared, stamp batchStamp) {
	ev := Evidence(semreg.ContractKernelV1, kindRequest, "sunspec-limit-source")
	b.SourceUpserts = []semreg.SourceDescriptor{{
		SourceID: stamp.source, SourceEpochID: stamp.epoch, ProtocolID: "modbus.sunspec", ProfileID: "sunspec.inverter.controls", ProfileVersion: "1",
		RegistryEvidence: ev, StartedAt: b.ObservedAt, State: semreg.SourceCurrent, Revision: "1",
	}}
	for _, item := range ready {
		binding, service, capability := localIDs(item.owner.inv.Label)
		slot, resource := item.owner.inv.Label, item.owner.resource
		anchor := ordered(Evidence(semreg.ContractKernelV1, kindRequest, "anchor:"+slot))
		b.ResourceUpserts = append(b.ResourceUpserts, semreg.Resource{
			ResourceID: resource, Kind: kindInverter, Revision: "1", State: semreg.ResourceCurrent,
			Anchor: semreg.ResourceAnchor{Class: semreg.AnchorEquipment, Basis: semreg.AnchorOperatorSlot, Slot: &slot, Evidence: anchor},
			Origin: semreg.OriginRef{OriginID: semreg.OriginID("origin:operator:" + slot), Kind: semreg.OriginOperator, Evidence: anchor},
		})
		b.BindingUpserts = append(b.BindingUpserts, semreg.NativeBinding{
			BindingID: semreg.NativeBindingID(binding), AssetID: stamp.asset, SourceID: stamp.source, SourceEpochID: stamp.epoch,
			DriverGeneration: stamp.generation, NativeResource: anchor[0], State: semreg.BindingCurrent, Revision: "1",
		})
		b.IdentityLinkUpserts = append(b.IdentityLinkUpserts, semreg.IdentityLink{
			AssetID: stamp.asset, BindingID: semreg.NativeBindingID(binding), State: semreg.LinkQualified, Basis: anchor, Revision: "1", Resource: &resource,
		})
		b.ServiceUpserts = append(b.ServiceUpserts, semreg.ServiceInstance{
			InstanceID: semreg.ServiceInstanceID(service), AssetID: stamp.asset, Definition: pvDefinition(serviceDef), BindingID: semreg.NativeBindingID(binding),
			SourceEpochID: stamp.epoch, DriverGeneration: stamp.generation, Qualification: semreg.QualificationQualified, Availability: semreg.AvailabilityAvailable, Revision: "1", Resource: &resource,
		})
		b.CapabilityUpserts = append(b.CapabilityUpserts, semreg.CapabilityInstance{
			InstanceID: semreg.CapabilityInstanceID(capability), AssetID: stamp.asset, ServiceInstance: semreg.ServiceInstanceID(service), Definition: pvDefinition(capDef), BindingID: semreg.NativeBindingID(binding),
			SourceEpochID: stamp.epoch, DriverGeneration: stamp.generation, Qualification: semreg.QualificationQualified, Availability: semreg.AvailabilityAvailable,
			Constraints: []semreg.TypedField{{ID: argID, Value: pvWattsValue(item.constraint)}}, ActivationEvidence: anchor, Revision: "1",
		})
	}
}

func wattsLimits(max semreg.Decimal) *driver.ValueLimits {
	min, top := wattsQuantity(semreg.Decimal{Coefficient: "0", Exponent10: 0}), wattsQuantity(max)
	return &driver.ValueLimits{Min: &min, Max: &top, Origin: driver.LimitDeviceQuery, Evidence: ordered(Evidence(semreg.ContractKernelV1, kindRequest, "wmax"))}
}

func pvDefinition(id semreg.DefinitionID) semreg.DefinitionRef {
	return semreg.DefinitionRef{Pack: semreg.PackRef{ID: pvPackID, Version: pvPackVersion}, ID: id, Version: pvPackVersion}
}

func factKey(resource string) semreg.FactKey {
	return semreg.FactKey{PackID: pvPackID, PackVersion: pvPackVersion, FactID: argID, Dimensions: []semreg.Dimension{{ID: dimID, Value: semreg.Value{Kind: semreg.ValueText, Text: &resource}}}}
}

func pathKey(label string) string { return "sunspec.controls.active_power_limit." + label }

func localIDs(label string) (binding, service, capability string) {
	return "binding:sunspec-limit:" + label, "service:sunspec-inverter:" + label, "capability:sunspec-limit:" + label
}

func localBinding(label string) string { return "binding:sunspec-limit:" + label }

func resourceID(label string) semreg.ResourceID {
	sum := sha256.Sum256([]byte("sunspec-inverter:" + label))
	return semreg.ResourceID("resource:sunspec-inverter:" + hex.EncodeToString(sum[:4]))
}

func stamped(cut driver.SemanticCut, path string) (driver.SettingRoute, bool) {
	for _, route := range cut.SettingRoutes {
		if route.Owner.PathKey == path {
			return route, true
		}
	}
	return driver.SettingRoute{}, false
}

func assetHolding(cut driver.SemanticCut, capability semreg.CapabilityInstanceID) (semreg.AssetID, bool) {
	var found semreg.AssetID
	for _, snap := range cut.Snapshots {
		for _, item := range snap.Capabilities {
			if item.InstanceID != capability || item.AssetID != snap.AssetID {
				continue
			}
			if found != "" && found != snap.AssetID {
				return "", false
			}
			found = snap.AssetID
		}
	}
	return found, found != ""
}

func findFact(cut driver.SemanticCut, local string, key semreg.FactKey) (semreg.Snapshot, semreg.FactCandidate, bool) {
	for _, snap := range cut.Snapshots {
		for _, env := range snap.Facts {
			for _, candidate := range env.Candidates {
				id := string(candidate.CandidateID)
				if (id == local || strings.HasPrefix(id, local+"@")) && sameKey(candidate.Key, key) {
					return snap, candidate, true
				}
			}
		}
	}
	return semreg.Snapshot{}, semreg.FactCandidate{}, false
}

func sameKey(a, b semreg.FactKey) bool {
	if a.PackID != b.PackID || a.PackVersion != b.PackVersion || a.FactID != b.FactID || len(a.Dimensions) != 1 || len(b.Dimensions) != 1 {
		return false
	}
	return a.Dimensions[0].ID == b.Dimensions[0].ID && a.Dimensions[0].Value.Text != nil && b.Dimensions[0].Value.Text != nil && *a.Dimensions[0].Value.Text == *b.Dimensions[0].Value.Text
}

func occurrence(snap sunspec.ChainSnapshot, model uint16, ordinal uint32) (sunspec.Occurrence, bool) {
	for _, occ := range snap.Occurrences() {
		if occ.ModelID() == model && occ.Ordinal == ordinal {
			return occ, true
		}
	}
	return sunspec.Occurrence{}, false
}

func spanWords(occ sunspec.Occurrence, address, count uint16) ([]uint16, bool) {
	words := occ.Words()
	start := int(address) - int(occ.HeaderOffset)
	if start < 0 || start+int(count) > len(words) {
		return nil, false
	}
	return words[start : start+int(count)], true
}

func positiveDecimal(decoded sunspec.DecodedModel, id string) (sunspec.Decimal, bool) {
	fact, ok := decoded.Fact(id)
	if !ok || fact.Value.State() != sunspec.ValueValid {
		return sunspec.Decimal{}, false
	}
	value, ok := fact.Value.Decimal()
	return value, ok && value.Coefficient > 0
}

func enabled(decoded sunspec.DecodedModel) bool {
	n, ok := enumNumber(decoded, fieldEna)
	return ok && n == 1
}

func enumNumber(decoded sunspec.DecodedModel, id string) (uint64, bool) {
	fact, ok := decoded.Fact(id)
	if !ok || fact.Value.State() != sunspec.ValueValid {
		return 0, false
	}
	n, _, ok := fact.Value.Enum()
	return n, ok
}

func modelWatts(decoded sunspec.DecodedModel, wmax sunspec.Decimal) (semreg.Decimal, bool) {
	percent, ok := positiveDecimal(decoded, fieldPct)
	if !ok && !zeroDecimal(decoded, fieldPct) {
		return semreg.Decimal{}, false
	}
	if !ok {
		percent = sunspec.Decimal{}
	}
	return canonical(wattsProduct(percent, wmax)), true
}

func zeroDecimal(decoded sunspec.DecodedModel, id string) bool {
	fact, ok := decoded.Fact(id)
	if !ok || fact.Value.State() != sunspec.ValueValid {
		return false
	}
	value, ok := fact.Value.Decimal()
	return ok && value.Coefficient == 0
}

func wattsProduct(percent, wmax sunspec.Decimal) *big.Int {
	value := sunRat(percent)
	value.Mul(value, sunRat(wmax))
	value.Quo(value, big.NewRat(100, 1))
	return new(big.Int).Quo(value.Num(), value.Denom())
}

func sunRat(d sunspec.Decimal) *big.Rat {
	r := new(big.Rat).SetInt64(d.Coefficient)
	factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(absInt(int64(d.Exponent))), nil)
	if d.Exponent >= 0 {
		return r.Mul(r, new(big.Rat).SetInt(factor))
	}
	return r.Quo(r, new(big.Rat).SetInt(factor))
}

func absInt(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func canonical(n *big.Int) semreg.Decimal {
	if n == nil || n.Sign() == 0 {
		return semreg.Decimal{Coefficient: "0", Exponent10: 0}
	}
	text, exp := n.String(), int32(0)
	for len(text) > 1 && text[len(text)-1] == '0' {
		text, exp = text[:len(text)-1], exp+1
	}
	return semreg.Decimal{Coefficient: text, Exponent10: exp}
}

func nativeDecimal(d sunspec.Decimal) semreg.Decimal {
	r := sunRat(d)
	return canonical(new(big.Int).Quo(r.Num(), r.Denom()))
}

func sameDecimal(a, b semreg.Decimal) bool { return intOf(a).Cmp(intOf(b)) == 0 }

func intOf(d semreg.Decimal) *big.Int {
	n, ok := new(big.Int).SetString(d.Coefficient, 10)
	if !ok {
		return new(big.Int)
	}
	if d.Exponent10 > 0 {
		n.Mul(n, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(d.Exponent10)), nil))
	}
	if d.Exponent10 < 0 {
		n.Quo(n, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-d.Exponent10)), nil))
	}
	return n
}

func wattsQuantity(d semreg.Decimal) semreg.Quantity {
	return semreg.Quantity{Number: d, Unit: wattUnit}
}

func pvWattsValue(d semreg.Decimal) semreg.Value {
	q := wattsQuantity(d)
	return semreg.Value{Kind: semreg.ValueQuantity, Quantity: &q}
}

func sunspecWatts(d semreg.Decimal) (sunspec.Decimal, error) {
	coeff, err := strconv.ParseInt(d.Coefficient, 10, 64)
	if err != nil || d.Exponent10 < -10 || d.Exponent10 > 10 {
		return sunspec.Decimal{}, errors.New("watts")
	}
	return sunspec.Decimal{Coefficient: coeff, Exponent: int16(d.Exponent10)}, nil
}
