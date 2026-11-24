package fronius

import (
	"encoding/json"
	"fmt"
	"reflect"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
)

const sunSpecQualificationObservationSchema = "helianthus-sunspec-qualification-observation/v1"

// SunSpecQualificationFlavorNotObserved is the flavor reason recorded when a
// chain is admitted and no Fronius flavor was observed.
const SunSpecQualificationFlavorNotObserved SunSpecFroniusFlavorReason = "NOT_OBSERVED"

// sunSpecQualificationFlavorNotObservedID is the flavor identifier encoded for
// SunSpecQualificationFlavorNotObserved. It is not an observed flavor.
const sunSpecQualificationFlavorNotObservedID = "sunspec.flavor.not_observed"

// SunSpecQualificationSampleIdentity is the capture identity shared by every
// retained source view of one qualification observation.
type SunSpecQualificationSampleIdentity struct {
	pollGeneration   uint64
	deadlineIdentity uint64
}

// PollGeneration returns the poll generation shared by every source view; it
// is never zero for a constructed observation.
func (identity SunSpecQualificationSampleIdentity) PollGeneration() uint64 {
	return identity.pollGeneration
}

// DeadlineIdentity returns the deadline identity shared by every source view;
// it is never zero for a constructed observation.
func (identity SunSpecQualificationSampleIdentity) DeadlineIdentity() uint64 {
	return identity.deadlineIdentity
}

// SunSpecQualificationReplay is an immutable detached replay image.
type SunSpecQualificationReplay struct {
	snapshot sunspec.ChainSnapshot
}

// RawWords returns a copy of the retained raw chain image.
func (replay SunSpecQualificationReplay) RawWords() []uint16 {
	return replay.snapshot.RawWords()
}

// Occurrences returns copies of the retained occurrences in chain order.
func (replay SunSpecQualificationReplay) Occurrences() []sunspec.Occurrence {
	return replay.snapshot.Occurrences()
}

// SourceViews returns copies of the retained source views in admission order.
func (replay SunSpecQualificationReplay) SourceViews() []modbusreg.LogicalViewSnapshot {
	return replay.snapshot.SourceViews()
}

// SunSpecQualificationObservation is the registry-owned, transport-neutral
// evidence record for one admitted SunSpec capture. A uniquely matched Fronius
// flavor is recorded on it. When no flavor matches, the flavor reason is
// SunSpecQualificationFlavorNotObserved.
type SunSpecQualificationObservation struct {
	capability sunspec.CapabilityDecision
	flavor     SunSpecFroniusFlavorDecision
	chain      []sunspec.WireKey
	sampleID   string
	identity   SunSpecQualificationSampleIdentity
	snapshot   sunspec.ChainSnapshot
}

// NewSunSpecQualificationObservation builds on sunspec.NewObservation. When one
// Fronius flavor matches, the observation records that flavor. When the chain
// is admitted and no flavor matches, the observation is returned with the
// flavor recorded as not observed. An ambiguous flavor match is rejected. It
// never accepts a caller verdict, flavor selection, or sample identity.
func NewSunSpecQualificationObservation(registry sunspec.DecoderRegistry, input sunspec.ChainSnapshot) (SunSpecQualificationObservation, error) {
	base, err := sunspec.NewObservation(registry, input)
	if err != nil {
		return SunSpecQualificationObservation{}, err
	}
	selection := SelectObservedFlavor(registry, base.Snapshot())
	var flavor SunSpecFroniusFlavorDecision
	if selection.Matched() {
		if selection.Reason() != SunSpecFroniusFlavorSelectionReasonMatched {
			return SunSpecQualificationObservation{}, fmt.Errorf("SunSpec qualification flavor is ambiguous")
		}
		decision, ok := selection.Decision()
		if !ok || !decision.Matched() || decision.Reason() != SunSpecFroniusFlavorReasonMatched {
			return SunSpecQualificationObservation{}, fmt.Errorf("SunSpec qualification flavor decision is invalid")
		}
		flavor = cloneSunSpecFroniusFlavorDecision(decision)
	} else if selection.Reason() == SunSpecFroniusFlavorSelectionReasonAmbiguousMatch {
		return SunSpecQualificationObservation{}, fmt.Errorf("SunSpec qualification flavor is ambiguous")
	} else if selection.Reason() != SunSpecFroniusFlavorSelectionReasonNoMatch {
		return SunSpecQualificationObservation{}, fmt.Errorf("SunSpec qualification flavor selection is invalid")
	} else {
		flavor = SunSpecFroniusFlavorDecision{
			flavorID:   sunSpecQualificationFlavorNotObservedID,
			reason:     SunSpecQualificationFlavorNotObserved,
			capability: base.Capability(),
		}
	}
	identity := base.SampleIdentity()
	return SunSpecQualificationObservation{
		capability: base.Capability(),
		flavor:     flavor,
		chain:      base.Chain(),
		sampleID:   base.SampleID(),
		identity: SunSpecQualificationSampleIdentity{
			pollGeneration: identity.PollGeneration(), deadlineIdentity: identity.DeadlineIdentity(),
		},
		snapshot: cloneSunSpecQualificationSnapshot(base.Snapshot()),
	}, nil
}

// Capability returns a copy of the admitted three-phase monitoring decision.
func (observation SunSpecQualificationObservation) Capability() sunspec.CapabilityDecision {
	return observation.capability.Clone()
}

// Flavor returns a copy of the recorded Fronius flavor. When none was
// observed, Matched is false and Reason is SunSpecQualificationFlavorNotObserved.
func (observation SunSpecQualificationObservation) Flavor() SunSpecFroniusFlavorDecision {
	return cloneSunSpecFroniusFlavorDecision(observation.flavor)
}

// Chain returns a copy of the terminal-verified wire chain, end marker
// included.
func (observation SunSpecQualificationObservation) Chain() []sunspec.WireKey {
	return append([]sunspec.WireKey(nil), observation.chain...)
}

// SampleID returns the sample identifier derived from the sample identity, in
// the form sunspec-<poll generation>-<deadline identity>.
func (observation SunSpecQualificationObservation) SampleID() string { return observation.sampleID }

// SampleIdentity returns the capture identity shared by the source views.
func (observation SunSpecQualificationObservation) SampleIdentity() SunSpecQualificationSampleIdentity {
	return observation.identity
}

// RawWords returns a copy of the retained raw chain image.
func (observation SunSpecQualificationObservation) RawWords() []uint16 {
	return observation.snapshot.RawWords()
}

// Occurrences returns copies of the retained occurrences in chain order.
func (observation SunSpecQualificationObservation) Occurrences() []sunspec.Occurrence {
	return observation.snapshot.Occurrences()
}

// SourceViews returns copies of the retained source views in admission order.
func (observation SunSpecQualificationObservation) SourceViews() []modbusreg.LogicalViewSnapshot {
	return observation.snapshot.SourceViews()
}

// Replay revalidates the retained snapshot and returns a detached copy of it.
// It returns an error when the retained snapshot is not a valid qualification
// capture, as for the zero value.
func (observation SunSpecQualificationObservation) Replay() (SunSpecQualificationReplay, error) {
	if _, _, err := validateSunSpecQualificationSnapshot(observation.snapshot); err != nil {
		return SunSpecQualificationReplay{}, err
	}
	return SunSpecQualificationReplay{snapshot: cloneSunSpecQualificationSnapshot(observation.snapshot)}, nil
}

func validateSunSpecQualificationSnapshot(input sunspec.ChainSnapshot) (sunspec.ChainSnapshot, SunSpecQualificationSampleIdentity, error) {
	snapshot, identity, err := sunspec.ValidateObservationSnapshot(input)
	if err != nil {
		return sunspec.ChainSnapshot{}, SunSpecQualificationSampleIdentity{}, err
	}
	return snapshot, SunSpecQualificationSampleIdentity{pollGeneration: identity.PollGeneration(), deadlineIdentity: identity.DeadlineIdentity()}, nil
}

func cloneSunSpecQualificationSnapshot(snapshot sunspec.ChainSnapshot) sunspec.ChainSnapshot {
	return snapshot.Clone()
}

// MarshalJSON revalidates the observation and encodes it as the versioned
// snake_case JSON evidence record, including raw words, occurrences and the
// full source-view provenance. An admitted observation whose flavor was not
// observed encodes that flavor as not observed, together with the sample,
// occurrences and source views. A matched flavor still has to agree in flavor
// identity, chain and source views. It returns an error instead of encoding a
// zero or internally inconsistent observation. The capability check accepts
// the admitted capability the observation carries.
func (observation SunSpecQualificationObservation) MarshalJSON() ([]byte, error) {
	snapshot, identity, err := validateSunSpecQualificationSnapshot(observation.snapshot)
	if err != nil {
		return nil, fmt.Errorf("SunSpec qualification observation cannot serialize invalid retained snapshot: %w", err)
	}
	chain, terminal := snapshot.WireChain()
	if !terminal || !reflect.DeepEqual(observation.chain, chain) {
		return nil, fmt.Errorf("SunSpec qualification observation cannot serialize inconsistent chain")
	}
	expectedSampleID := fmt.Sprintf("sunspec-%d-%d", identity.pollGeneration, identity.deadlineIdentity)
	if observation.sampleID == "" || observation.sampleID != expectedSampleID || observation.identity != identity {
		return nil, fmt.Errorf("SunSpec qualification observation cannot serialize inconsistent sample identity")
	}
	capability := observation.Capability()
	if !capability.Admitted() || capability.Reason() != sunspec.CapabilityReasonAdmitted {
		return nil, fmt.Errorf("SunSpec qualification observation cannot serialize non-admitted capability")
	}
	flavor := observation.Flavor()
	if flavor.Reason() == SunSpecQualificationFlavorNotObserved {
		if flavor.Matched() || flavor.FlavorID() != sunSpecQualificationFlavorNotObservedID || len(flavor.Chain()) != 0 || len(flavor.SourceViews()) != 0 {
			return nil, fmt.Errorf("SunSpec qualification observation cannot serialize inconsistent flavor")
		}
	} else if !flavor.Matched() || flavor.Reason() != SunSpecFroniusFlavorReasonMatched || !supportedSunSpecQualificationFlavorID(flavor.FlavorID()) || !reflect.DeepEqual(flavor.Chain(), chain) || !reflect.DeepEqual(flavor.SourceViews(), snapshot.SourceViews()) {
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
		Chain:       append([]sunspec.WireKey(nil), chain...),
		RawWords:    snapshot.RawWords(),
		Occurrences: encodedOccurrences,
		SourceViews: encodedViews,
	})
}

func supportedSunSpecQualificationFlavorID(flavorID string) bool {
	return flavorID == SunSpecFroniusObservedFlavorID || flavorID == SunSpecFroniusObservedFlavorV11ID ||
		flavorID == SunSpecFroniusObservedFlavorV12ID
}

type sunSpecQualificationObservationJSON struct {
	Schema           string                                 `json:"schema"`
	CapabilityID     string                                 `json:"capability_id"`
	CapabilityReason sunspec.CapabilityReason               `json:"capability_reason"`
	FlavorID         string                                 `json:"flavor_id"`
	FlavorReason     SunSpecFroniusFlavorReason             `json:"flavor_reason"`
	SampleID         string                                 `json:"sample_id"`
	SampleIdentity   sunSpecQualificationSampleIdentityJSON `json:"sample_identity"`
	Chain            []sunspec.WireKey                      `json:"chain"`
	RawWords         []uint16                               `json:"raw_words"`
	Occurrences      []sunSpecQualificationOccurrenceJSON   `json:"occurrences"`
	SourceViews      []sunSpecQualificationLogicalViewJSON  `json:"source_views"`
}

type sunSpecQualificationSampleIdentityJSON struct {
	PollGeneration   uint64 `json:"poll_generation"`
	DeadlineIdentity uint64 `json:"deadline_identity"`
}

type sunSpecQualificationOccurrenceJSON struct {
	Ordinal        uint32                   `json:"ordinal"`
	WireKey        sunspec.WireKey          `json:"wire_key"`
	SchemaRevision sunspec.SchemaRevision   `json:"schema_revision"`
	HeaderOffset   uint16                   `json:"header_offset"`
	PayloadOffset  uint16                   `json:"payload_offset"`
	Disposition    sunspec.ChainDisposition `json:"disposition"`
	DecoderKey     *sunspec.DecoderKey      `json:"decoder_key,omitempty"`
	Words          []uint16                 `json:"words"`
	SourceSpans    []sunspec.SourceSpan     `json:"source_spans"`
}

type sunSpecQualificationLogicalViewJSON struct {
	LogicalViewID       uint64                    `json:"logical_view_id"`
	WireResponseID      uint64                    `json:"wire_response_id"`
	PhysicalRequestID   uint64                    `json:"physical_request_id"`
	Endpoint            string                    `json:"endpoint"`
	ConnectionID        uint64                    `json:"connection_id"`
	Transport           modbusreg.TransportFamily `json:"transport"`
	TransportGeneration uint64                    `json:"transport_generation"`
	UnitID              byte                      `json:"unit_id"`
	RequestedFunction   modbusreg.FunctionCode    `json:"requested_function"`
	ReceivedFunction    modbusreg.FunctionCode    `json:"received_function"`
	Table               modbusreg.LogicalTable    `json:"table"`
	PhysicalOffset      uint16                    `json:"physical_offset"`
	PhysicalWordCount   uint16                    `json:"physical_word_count"`
	AuthorizationScope  string                    `json:"authorization_scope"`
	PollGeneration      uint64                    `json:"poll_generation"`
	DeadlineIdentity    uint64                    `json:"deadline_identity"`
	LogicalOffset       uint16                    `json:"logical_offset"`
	LogicalWordCount    uint16                    `json:"logical_word_count"`
	SliceOffset         uint16                    `json:"slice_offset"`
	SliceWordCount      uint16                    `json:"slice_word_count"`
	Words               []uint16                  `json:"words"`
	WireResponseBytes   []byte                    `json:"wire_response_bytes"`
}

func newSunSpecQualificationLogicalViewJSON(record modbusreg.LogicalViewRecord) sunSpecQualificationLogicalViewJSON {
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
