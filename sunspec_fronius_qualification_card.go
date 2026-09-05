package modbusreg

import (
	"encoding/json"
	"fmt"
	"reflect"
)

const (
	SunSpecFroniusQualificationCardID    = "sunspec.qualification.fronius.gen24.float.readonly@1.0.0"
	sunSpecFroniusQualificationBase      = uint16(40000)
	sunSpecFroniusQualificationUnitID    = byte(1)
	sunSpecFroniusQualificationMaxModels = uint32(64)
	sunSpecFroniusQualificationResultV1  = "helianthus-sunspec-fronius-qualification-expected/v1"
	sunSpecFroniusOfflineReplayMatch     = "OFFLINE_REPLAY_MATCH"
)

// SunSpecFroniusQualificationCard defines the bounded, read-only acquisition
// and evaluation inputs for the exact observed Fronius flavor. The card does
// not carry endpoint authority and cannot represent a live qualification.
type SunSpecFroniusQualificationCard struct{}

func NewSunSpecFroniusQualificationCard() SunSpecFroniusQualificationCard {
	return SunSpecFroniusQualificationCard{}
}

func (SunSpecFroniusQualificationCard) ID() string { return SunSpecFroniusQualificationCardID }
func (SunSpecFroniusQualificationCard) FlavorID() string {
	return SunSpecFroniusObservedFlavorV11ID
}
func (SunSpecFroniusQualificationCard) SchemaRevision() SunSpecSchemaRevision {
	return SunSpecModelsRevisionV1
}
func (SunSpecFroniusQualificationCard) Manufacturer() string { return "Fronius" }
func (SunSpecFroniusQualificationCard) Model() string        { return "Symo GEN24 10.0" }
func (SunSpecFroniusQualificationCard) Firmware() string     { return "1.41.11-1" }
func (SunSpecFroniusQualificationCard) UnitID() byte         { return sunSpecFroniusQualificationUnitID }
func (SunSpecFroniusQualificationCard) Function() FunctionCode {
	return FunctionReadHoldingRegisters
}
func (SunSpecFroniusQualificationCard) ReadOnly() bool                  { return true }
func (SunSpecFroniusQualificationCard) LiveQualified() bool             { return false }
func (SunSpecFroniusQualificationCard) AutomaticRuntimeAdmission() bool { return false }
func (SunSpecFroniusQualificationCard) WriteAuthority() bool            { return false }

func (SunSpecFroniusQualificationCard) BaseCandidates() []uint16 {
	return []uint16{sunSpecFroniusQualificationBase}
}

func (SunSpecFroniusQualificationCard) Limits() SunSpecChainLimits {
	return SunSpecChainLimits{
		MaxTotalWords:  MaxSunSpecPhaseOneChainWords,
		MaxOccurrences: sunSpecFroniusQualificationMaxModels,
	}
}

func (SunSpecFroniusQualificationCard) ExpectedChain() []SunSpecWireKey {
	return append([]SunSpecWireKey(nil), froniusObservedChainV11...)
}

// SemanticCandidateFieldIDs lists the exact qualified native facts that may be
// considered by a later semantic owner. It is not a promotion or publication
// decision and does not expose facts from any other model in the retained chain.
func (SunSpecFroniusQualificationCard) SemanticCandidateFieldIDs() []string {
	fields := make([]string, len(threePhaseMonitoringFields))
	for index, field := range threePhaseMonitoringFields {
		fields[index] = field.id
	}
	return fields
}

// NewReplayChain returns a fresh stateful acquisition adapter for one replay.
// Every request is FC03 and remains subject to the card's finite bounds.
func (card SunSpecFroniusQualificationCard) NewReplayChain(registry SunSpecDecoderRegistry) (*SunSpecChain, error) {
	if registry.revision != card.SchemaRevision() {
		return nil, fmt.Errorf("qualification requires the pinned SunSpec V1 registry")
	}
	plan, err := NewSunSpecChainPlan(SunSpecChainPlanSpec{
		SchemaRevision: card.SchemaRevision(),
		BaseCandidates: card.BaseCandidates(),
		Limits:         card.Limits(),
		DecoderKeys:    registry.DecoderKeys(),
	})
	if err != nil {
		return nil, err
	}
	return NewSunSpecChain(plan), nil
}

// Qualify derives the existing immutable qualification observation only after
// the complete replay also matches the card's exact observed unit and flavor.
func (card SunSpecFroniusQualificationCard) Qualify(registry SunSpecDecoderRegistry, snapshot SunSpecChainSnapshot) (SunSpecQualificationObservation, error) {
	if registry.revision != card.SchemaRevision() {
		return SunSpecQualificationObservation{}, fmt.Errorf("qualification requires the pinned SunSpec V1 registry")
	}
	snapshot, err := card.replayExactAcquisition(registry, snapshot)
	if err != nil {
		return SunSpecQualificationObservation{}, err
	}
	views := snapshot.SourceViews()
	if len(views) == 0 {
		return SunSpecQualificationObservation{}, fmt.Errorf("qualification requires source-backed views")
	}
	for _, view := range views {
		record := view.Record()
		if record.UnitID != card.UnitID() || record.RequestedFunction != card.Function() ||
			record.ReceivedFunction != card.Function() || record.Table != HoldingRegisters {
			return SunSpecQualificationObservation{}, fmt.Errorf("qualification source is outside the read-only card")
		}
	}
	observation, err := NewSunSpecQualificationObservation(registry, snapshot)
	if err != nil {
		return SunSpecQualificationObservation{}, err
	}
	if observation.Flavor().FlavorID() != card.FlavorID() {
		return SunSpecQualificationObservation{}, fmt.Errorf("qualification matched a different observed flavor")
	}
	return observation, nil
}

func (card SunSpecFroniusQualificationCard) replayExactAcquisition(registry SunSpecDecoderRegistry, input SunSpecChainSnapshot) (SunSpecChainSnapshot, error) {
	chain, err := card.NewReplayChain(registry)
	if err != nil {
		return SunSpecChainSnapshot{}, err
	}
	views := input.SourceViews()
	var terminal SunSpecChainSnapshot
	for index, view := range views {
		requests := chain.NextRequests()
		if len(requests) != 1 {
			return SunSpecChainSnapshot{}, fmt.Errorf("qualification source sequence is outside the exact card plan")
		}
		snapshot, err := chain.AdmitReplay(requests[0], view)
		if err != nil {
			return SunSpecChainSnapshot{}, fmt.Errorf("qualification source sequence is outside the exact card plan: %w", err)
		}
		if len(snapshot.RawWords()) != 0 {
			if index != len(views)-1 {
				return SunSpecChainSnapshot{}, fmt.Errorf("qualification source sequence continues after the terminal marker")
			}
			terminal = snapshot
		}
	}
	if len(terminal.RawWords()) == 0 || len(chain.NextRequests()) != 0 {
		return SunSpecChainSnapshot{}, fmt.Errorf("qualification source sequence is incomplete")
	}
	if !reflect.DeepEqual(terminal.RawWords(), input.RawWords()) ||
		!reflect.DeepEqual(terminal.Occurrences(), input.Occurrences()) ||
		!reflect.DeepEqual(terminal.SourceViews(), input.SourceViews()) {
		return SunSpecChainSnapshot{}, fmt.Errorf("qualification snapshot differs from the exact card replay")
	}
	return terminal, nil
}

// SunSpecFroniusQualificationExpectedResult is a sanitized static expectation.
// The source-backed qualification observation remains the owner of raw words
// and acquisition provenance.
type SunSpecFroniusQualificationExpectedResult struct {
	card SunSpecFroniusQualificationCard
}

func (card SunSpecFroniusQualificationCard) ExpectedResult() SunSpecFroniusQualificationExpectedResult {
	return SunSpecFroniusQualificationExpectedResult{card: card}
}

func (result SunSpecFroniusQualificationExpectedResult) MarshalJSON() ([]byte, error) {
	card := result.card
	return json.Marshal(struct {
		Schema                    string           `json:"schema"`
		CardID                    string           `json:"card_id"`
		Status                    string           `json:"status"`
		CapabilityID              string           `json:"capability_id"`
		FlavorID                  string           `json:"flavor_id"`
		Manufacturer              string           `json:"manufacturer"`
		Model                     string           `json:"model"`
		Firmware                  string           `json:"firmware"`
		Function                  string           `json:"function"`
		UnitID                    byte             `json:"unit_id"`
		PDUOffset                 uint16           `json:"pdu_offset"`
		MaxTotalWords             uint32           `json:"max_total_words"`
		MaxOccurrences            uint32           `json:"max_occurrences"`
		Chain                     []SunSpecWireKey `json:"chain"`
		SemanticCandidateFieldIDs []string         `json:"semantic_candidate_field_ids"`
		RetainedEvidence          []string         `json:"retained_evidence"`
		LiveQualified             bool             `json:"live_qualified"`
		AutomaticRuntimeAdmission bool             `json:"automatic_runtime_admission"`
		WriteAuthority            bool             `json:"write_authority"`
	}{
		Schema:                    sunSpecFroniusQualificationResultV1,
		CardID:                    card.ID(),
		Status:                    sunSpecFroniusOfflineReplayMatch,
		CapabilityID:              SunSpecThreePhaseMonitoringCapabilityID,
		FlavorID:                  card.FlavorID(),
		Manufacturer:              card.Manufacturer(),
		Model:                     card.Model(),
		Firmware:                  card.Firmware(),
		Function:                  "FC03",
		UnitID:                    card.UnitID(),
		PDUOffset:                 card.BaseCandidates()[0],
		MaxTotalWords:             card.Limits().MaxTotalWords,
		MaxOccurrences:            card.Limits().MaxOccurrences,
		Chain:                     card.ExpectedChain(),
		SemanticCandidateFieldIDs: card.SemanticCandidateFieldIDs(),
		RetainedEvidence:          []string{"raw_words", "occurrence_order", "source_spans", "logical_view_provenance"},
		LiveQualified:             card.LiveQualified(),
		AutomaticRuntimeAdmission: card.AutomaticRuntimeAdmission(),
		WriteAuthority:            card.WriteAuthority(),
	})
}
