package modbusreg

import (
	"encoding/json"
	"fmt"
	"reflect"
)

const sunSpecQualificationObservationSchema = "helianthus-sunspec-qualification-observation/v1"

// SunSpecQualificationSampleIdentity is the capture identity shared by every
// retained source view of one qualification observation.
type SunSpecQualificationSampleIdentity struct {
	pollGeneration   uint64
	deadlineIdentity uint64
}

func (identity SunSpecQualificationSampleIdentity) PollGeneration() uint64 {
	return identity.pollGeneration
}

func (identity SunSpecQualificationSampleIdentity) DeadlineIdentity() uint64 {
	return identity.deadlineIdentity
}

// SunSpecQualificationReplay is an immutable detached replay image.
type SunSpecQualificationReplay struct{ snapshot SunSpecChainSnapshot }

func (replay SunSpecQualificationReplay) RawWords() []uint16 {
	return replay.snapshot.RawWords()
}

func (replay SunSpecQualificationReplay) Occurrences() []SunSpecOccurrence {
	return replay.snapshot.Occurrences()
}

func (replay SunSpecQualificationReplay) SourceViews() []LogicalViewSnapshot {
	return replay.snapshot.SourceViews()
}

// SunSpecQualificationObservation is the registry-owned, transport-neutral
// evidence record for one terminal-qualified SunSpec capture.
type SunSpecQualificationObservation struct {
	capability SunSpecCapabilityDecision
	flavor     SunSpecFroniusFlavorDecision
	chain      []SunSpecWireKey
	sampleID   string
	identity   SunSpecQualificationSampleIdentity
	snapshot   SunSpecChainSnapshot
}

// NewSunSpecQualificationObservation derives qualification only from the
// supplied registry and complete source-backed snapshot. It never accepts a
// caller verdict, flavor selection, or sample identity.
func NewSunSpecQualificationObservation(registry SunSpecDecoderRegistry, input SunSpecChainSnapshot) (SunSpecQualificationObservation, error) {
	snapshot, identity, err := validateSunSpecQualificationSnapshot(input)
	if err != nil {
		return SunSpecQualificationObservation{}, err
	}
	chain, valid := sunSpecSnapshotWireChain(snapshot)
	if !valid {
		return SunSpecQualificationObservation{}, fmt.Errorf("SunSpec qualification chain is not terminal verified")
	}
	capability := registry.EvaluateThreePhaseMonitoring(snapshot)
	if !capability.Admitted() || capability.Reason() != SunSpecCapabilityReasonAdmitted {
		return SunSpecQualificationObservation{}, fmt.Errorf("SunSpec qualification capability is not admitted")
	}
	selection := registry.SelectFroniusObservedFlavor(snapshot)
	if !selection.Matched() || selection.Reason() != SunSpecFroniusFlavorSelectionReasonMatched {
		return SunSpecQualificationObservation{}, fmt.Errorf("SunSpec qualification flavor is not uniquely matched")
	}
	flavor, ok := selection.Decision()
	if !ok || !flavor.Matched() || flavor.Reason() != SunSpecFroniusFlavorReasonMatched {
		return SunSpecQualificationObservation{}, fmt.Errorf("SunSpec qualification flavor decision is invalid")
	}
	return SunSpecQualificationObservation{
		capability: cloneSunSpecCapabilityDecision(capability),
		flavor:     cloneSunSpecFroniusFlavorDecision(flavor),
		chain:      append([]SunSpecWireKey(nil), chain...),
		sampleID:   fmt.Sprintf("sunspec-%d-%d", identity.pollGeneration, identity.deadlineIdentity),
		identity:   identity,
		snapshot:   cloneSunSpecQualificationSnapshot(snapshot),
	}, nil
}

func (observation SunSpecQualificationObservation) Capability() SunSpecCapabilityDecision {
	return cloneSunSpecCapabilityDecision(observation.capability)
}

func (observation SunSpecQualificationObservation) Flavor() SunSpecFroniusFlavorDecision {
	return cloneSunSpecFroniusFlavorDecision(observation.flavor)
}

func (observation SunSpecQualificationObservation) Chain() []SunSpecWireKey {
	return append([]SunSpecWireKey(nil), observation.chain...)
}

func (observation SunSpecQualificationObservation) SampleID() string { return observation.sampleID }

func (observation SunSpecQualificationObservation) SampleIdentity() SunSpecQualificationSampleIdentity {
	return observation.identity
}

func (observation SunSpecQualificationObservation) RawWords() []uint16 {
	return observation.snapshot.RawWords()
}

func (observation SunSpecQualificationObservation) Occurrences() []SunSpecOccurrence {
	return observation.snapshot.Occurrences()
}

func (observation SunSpecQualificationObservation) SourceViews() []LogicalViewSnapshot {
	return observation.snapshot.SourceViews()
}

func (observation SunSpecQualificationObservation) Replay() (SunSpecQualificationReplay, error) {
	if _, _, err := validateSunSpecQualificationSnapshot(observation.snapshot); err != nil {
		return SunSpecQualificationReplay{}, err
	}
	return SunSpecQualificationReplay{snapshot: cloneSunSpecQualificationSnapshot(observation.snapshot)}, nil
}

func validateSunSpecQualificationSnapshot(input SunSpecChainSnapshot) (SunSpecChainSnapshot, SunSpecQualificationSampleIdentity, error) {
	if len(input.raw) < 4 || len(input.raw) > MaxSunSpecPhaseOneChainWords || len(input.occurrences) == 0 || len(input.occurrences) > MaxSunSpecPhaseOneChainWords/2 || len(input.sources) == 0 || len(input.sources) > MaxSunSpecPhaseOneChainWords {
		return SunSpecChainSnapshot{}, SunSpecQualificationSampleIdentity{}, fmt.Errorf("SunSpec qualification bounds are invalid")
	}
	sources := make([]LogicalViewRecord, len(input.sources))
	first := input.sources[0]
	if first.PollGeneration == 0 || first.DeadlineIdentity == 0 {
		return SunSpecChainSnapshot{}, SunSpecQualificationSampleIdentity{}, fmt.Errorf("SunSpec qualification sample identity is invalid")
	}
	for index, source := range input.sources {
		validated, err := NewLogicalViewSnapshot(source)
		if err != nil {
			return SunSpecChainSnapshot{}, SunSpecQualificationSampleIdentity{}, fmt.Errorf("SunSpec qualification source view %d: %w", index, err)
		}
		record := validated.Record()
		if !sameSunSpecProvenance(first, record) || record.PollGeneration == 0 || record.DeadlineIdentity == 0 {
			return SunSpecChainSnapshot{}, SunSpecQualificationSampleIdentity{}, fmt.Errorf("SunSpec qualification source provenance is mixed")
		}
		sources[index] = record
	}
	snapshot := SunSpecChainSnapshot{
		raw:         append([]uint16(nil), input.raw...),
		occurrences: input.Occurrences(),
		sources:     sources,
	}
	if _, valid := sunSpecSnapshotWireChain(snapshot); !valid {
		return SunSpecChainSnapshot{}, SunSpecQualificationSampleIdentity{}, fmt.Errorf("SunSpec qualification retained chain is invalid")
	}
	return snapshot, SunSpecQualificationSampleIdentity{pollGeneration: first.PollGeneration, deadlineIdentity: first.DeadlineIdentity}, nil
}

func cloneSunSpecQualificationSnapshot(snapshot SunSpecChainSnapshot) SunSpecChainSnapshot {
	cloned := SunSpecChainSnapshot{raw: snapshot.RawWords(), occurrences: snapshot.Occurrences()}
	for _, source := range snapshot.SourceViews() {
		cloned.sources = append(cloned.sources, source.Record())
	}
	return cloned
}

func (observation SunSpecQualificationObservation) MarshalJSON() ([]byte, error) {
	snapshot, identity, err := validateSunSpecQualificationSnapshot(observation.snapshot)
	if err != nil {
		return nil, fmt.Errorf("SunSpec qualification observation cannot serialize invalid retained snapshot: %w", err)
	}
	chain, terminal := sunSpecSnapshotWireChain(snapshot)
	if !terminal || !reflect.DeepEqual(observation.chain, chain) {
		return nil, fmt.Errorf("SunSpec qualification observation cannot serialize inconsistent chain")
	}
	expectedSampleID := fmt.Sprintf("sunspec-%d-%d", identity.pollGeneration, identity.deadlineIdentity)
	if observation.sampleID == "" || observation.sampleID != expectedSampleID || observation.identity != identity {
		return nil, fmt.Errorf("SunSpec qualification observation cannot serialize inconsistent sample identity")
	}
	capability := observation.Capability()
	if !capability.Admitted() || capability.Reason() != SunSpecCapabilityReasonAdmitted || capability.ProfileID() != SunSpecThreePhaseMonitoringCapabilityID {
		return nil, fmt.Errorf("SunSpec qualification observation cannot serialize non-admitted capability")
	}
	flavor := observation.Flavor()
	if !flavor.Matched() || flavor.Reason() != SunSpecFroniusFlavorReasonMatched || !supportedSunSpecQualificationFlavorID(flavor.FlavorID()) || !reflect.DeepEqual(flavor.Chain(), chain) || !reflect.DeepEqual(flavor.SourceViews(), snapshot.SourceViews()) {
		return nil, fmt.Errorf("SunSpec qualification observation cannot serialize inconsistent flavor")
	}

	occurrences := snapshot.Occurrences()
	encodedOccurrences := make([]sunSpecQualificationOccurrenceJSON, len(occurrences))
	for index, occurrence := range occurrences {
		key, hasKey := occurrence.DecoderKey()
		encodedOccurrences[index] = sunSpecQualificationOccurrenceJSON{
			Ordinal:        occurrence.Ordinal,
			WireKey:        occurrence.WireKey,
			SchemaRevision: occurrence.SchemaRevision,
			HeaderOffset:   occurrence.HeaderOffset,
			PayloadOffset:  occurrence.PayloadOffset,
			Disposition:    occurrence.Disposition,
			Words:          occurrence.Words(),
			SourceSpans:    occurrence.SourceSpans(),
		}
		if hasKey {
			keyCopy := key
			encodedOccurrences[index].DecoderKey = &keyCopy
		}
	}
	views := snapshot.SourceViews()
	encodedViews := make([]sunSpecQualificationLogicalViewJSON, len(views))
	for index, view := range views {
		encodedViews[index] = newSunSpecQualificationLogicalViewJSON(view.Record())
	}
	return json.Marshal(sunSpecQualificationObservationJSON{
		Schema:           sunSpecQualificationObservationSchema,
		CapabilityID:     capability.ProfileID(),
		CapabilityReason: capability.Reason(),
		FlavorID:         flavor.FlavorID(),
		FlavorReason:     flavor.Reason(),
		SampleID:         observation.SampleID(),
		SampleIdentity: sunSpecQualificationSampleIdentityJSON{
			PollGeneration: identity.pollGeneration, DeadlineIdentity: identity.deadlineIdentity,
		},
		Chain:       append([]SunSpecWireKey(nil), chain...),
		RawWords:    snapshot.RawWords(),
		Occurrences: encodedOccurrences,
		SourceViews: encodedViews,
	})
}

func supportedSunSpecQualificationFlavorID(flavorID string) bool {
	return flavorID == SunSpecFroniusObservedFlavorID || flavorID == SunSpecFroniusObservedFlavorV11ID
}

type sunSpecQualificationObservationJSON struct {
	Schema           string                                 `json:"schema"`
	CapabilityID     string                                 `json:"capability_id"`
	CapabilityReason SunSpecCapabilityReason                `json:"capability_reason"`
	FlavorID         string                                 `json:"flavor_id"`
	FlavorReason     SunSpecFroniusFlavorReason             `json:"flavor_reason"`
	SampleID         string                                 `json:"sample_id"`
	SampleIdentity   sunSpecQualificationSampleIdentityJSON `json:"sample_identity"`
	Chain            []SunSpecWireKey                       `json:"chain"`
	RawWords         []uint16                               `json:"raw_words"`
	Occurrences      []sunSpecQualificationOccurrenceJSON   `json:"occurrences"`
	SourceViews      []sunSpecQualificationLogicalViewJSON  `json:"source_views"`
}

type sunSpecQualificationSampleIdentityJSON struct {
	PollGeneration   uint64 `json:"poll_generation"`
	DeadlineIdentity uint64 `json:"deadline_identity"`
}

type sunSpecQualificationOccurrenceJSON struct {
	Ordinal        uint32                  `json:"ordinal"`
	WireKey        SunSpecWireKey          `json:"wire_key"`
	SchemaRevision SunSpecSchemaRevision   `json:"schema_revision"`
	HeaderOffset   uint16                  `json:"header_offset"`
	PayloadOffset  uint16                  `json:"payload_offset"`
	Disposition    SunSpecChainDisposition `json:"disposition"`
	DecoderKey     *SunSpecDecoderKey      `json:"decoder_key,omitempty"`
	Words          []uint16                `json:"words"`
	SourceSpans    []SunSpecSourceSpan     `json:"source_spans"`
}

type sunSpecQualificationLogicalViewJSON struct {
	LogicalViewID       uint64          `json:"logical_view_id"`
	WireResponseID      uint64          `json:"wire_response_id"`
	PhysicalRequestID   uint64          `json:"physical_request_id"`
	Endpoint            string          `json:"endpoint"`
	ConnectionID        uint64          `json:"connection_id"`
	Transport           TransportFamily `json:"transport"`
	TransportGeneration uint64          `json:"transport_generation"`
	UnitID              byte            `json:"unit_id"`
	RequestedFunction   FunctionCode    `json:"requested_function"`
	ReceivedFunction    FunctionCode    `json:"received_function"`
	Table               LogicalTable    `json:"table"`
	PhysicalOffset      uint16          `json:"physical_offset"`
	PhysicalWordCount   uint16          `json:"physical_word_count"`
	AuthorizationScope  string          `json:"authorization_scope"`
	PollGeneration      uint64          `json:"poll_generation"`
	DeadlineIdentity    uint64          `json:"deadline_identity"`
	LogicalOffset       uint16          `json:"logical_offset"`
	LogicalWordCount    uint16          `json:"logical_word_count"`
	SliceOffset         uint16          `json:"slice_offset"`
	SliceWordCount      uint16          `json:"slice_word_count"`
	Words               []uint16        `json:"words"`
	WireResponseBytes   []byte          `json:"wire_response_bytes"`
}

func newSunSpecQualificationLogicalViewJSON(record LogicalViewRecord) sunSpecQualificationLogicalViewJSON {
	return sunSpecQualificationLogicalViewJSON{
		LogicalViewID: record.LogicalViewID, WireResponseID: record.WireResponseID, PhysicalRequestID: record.PhysicalRequestID,
		Endpoint: record.Endpoint, ConnectionID: record.ConnectionID, Transport: record.Transport, TransportGeneration: record.TransportGeneration,
		UnitID: record.UnitID, RequestedFunction: record.RequestedFunction, ReceivedFunction: record.ReceivedFunction, Table: record.Table,
		PhysicalOffset: record.PhysicalOffset, PhysicalWordCount: record.PhysicalWordCount, AuthorizationScope: record.AuthorizationScope,
		PollGeneration: record.PollGeneration, DeadlineIdentity: record.DeadlineIdentity, LogicalOffset: record.LogicalOffset,
		LogicalWordCount: record.LogicalWordCount, SliceOffset: record.SliceOffset, SliceWordCount: record.SliceWordCount,
		Words: append([]uint16(nil), record.Words...), WireResponseBytes: append([]byte(nil), record.WireResponseBytes...),
	}
}
