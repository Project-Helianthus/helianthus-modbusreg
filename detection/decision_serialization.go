package detection

import (
	"fmt"
	"sort"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

type detectionCandidateEvidenceDTO struct {
	ProfileID            string               `json:"profile_id"`
	ProfileVersion       string               `json:"profile_version"`
	Score                uint32               `json:"score"`
	Reason               Reason               `json:"reason"`
	MatchedGates         []ProbeIdentityField `json:"matched_gates"`
	ProbeEvidenceIDs     []string             `json:"probe_evidence_ids"`
	DetectorVersion      string               `json:"detector_version"`
	QualificationVersion string               `json:"qualification_version"`
}

type detectionDecisionDTO struct {
	SchemaVersion          string                          `json:"schema_version"`
	Outcome                Outcome                         `json:"outcome"`
	Reason                 Reason                          `json:"reason"`
	SelectedProfileID      string                          `json:"selected_profile_id"`
	SelectedProfileVersion string                          `json:"selected_profile_version"`
	Evidence               []detectionCandidateEvidenceDTO `json:"evidence"`
}

func validDetectionOutcome(outcome Outcome) bool {
	switch outcome {
	case Matched, NoMatch, Ambiguous:
		return true
	default:
		return false
	}
}

func validDetectionReason(reason Reason) bool {
	switch reason {
	case ReasonSelected,
		ReasonEqualBest,
		ReasonMultipleMatches,
		ReasonLowerScore,
		ReasonIdentityMismatch,
		ReasonFirmwareMismatch,
		ReasonInvalidIdentity,
		ReasonFixtureOptInRequired,
		ReasonCandidateDisabled,
		ReasonProfileDefaultOff,
		ReasonProfileRevoked,
		ReasonProfileSuperseded,
		ReasonProfileUnqualified,
		ReasonReadError,
		ReasonProbeException,
		ReasonIncompleteProbe,
		ReasonInvalidProbeResult,
		ReasonContextCancelled:
		return true
	default:
		return false
	}
}

func identityFieldRank(field ProbeIdentityField) int {
	switch field {
	case ProbeIdentityManufacturer:
		return 0
	case ProbeIdentityModel:
		return 1
	case ProbeIdentityFirmware:
		return 2
	default:
		return -1
	}
}

func validEvidenceOrder(evidence []CandidateEvidence) bool {
	for index := 1; index < len(evidence); index++ {
		previous := evidence[index-1]
		current := evidence[index]
		if previous.ProfileID >= current.ProfileID {
			return false
		}
	}
	return true
}

func validateDetectionEvidence(
	evidence CandidateEvidence,
	limits Limits,
) error {
	if !modbusreg.ValidIdentity(evidence.ProfileID) ||
		!evidence.ProfileVersion.Valid() ||
		evidence.Score == 0 ||
		!validDetectionReason(evidence.Reason) ||
		!evidence.DetectorVersion.Valid() ||
		!evidence.QualificationVersion.Valid() ||
		len(evidence.MatchedGates) > limits.MaxPlanDeclarations ||
		len(evidence.ProbeEvidenceIDs) != len(evidence.MatchedGates) {
		return fmt.Errorf("detection candidate evidence is invalid")
	}
	previousRank := -1
	seenEvidenceIDs := make(map[string]struct{}, len(evidence.ProbeEvidenceIDs))
	for index, field := range evidence.MatchedGates {
		rank := identityFieldRank(field)
		if rank <= previousRank {
			return fmt.Errorf("detection candidate gates are not canonical")
		}
		previousRank = rank
		evidenceID := evidence.ProbeEvidenceIDs[index]
		if !validProbeEvidenceID(evidenceID, limits.MaxEvidenceIDBytes) {
			return fmt.Errorf("detection candidate evidence identity is invalid")
		}
		if _, duplicate := seenEvidenceIDs[evidenceID]; duplicate {
			return fmt.Errorf("detection candidate evidence identity is duplicated")
		}
		seenEvidenceIDs[evidenceID] = struct{}{}
	}
	gateCount := len(evidence.MatchedGates)
	switch evidence.Reason {
	case ReasonSelected,
		ReasonEqualBest,
		ReasonMultipleMatches,
		ReasonLowerScore:
		if gateCount != 3 {
			return fmt.Errorf("ranked detection evidence lacks complete gates")
		}
	case ReasonIdentityMismatch:
		if gateCount > 1 {
			return fmt.Errorf("identity mismatch evidence has impossible gates")
		}
	case ReasonFirmwareMismatch:
		if gateCount != 2 {
			return fmt.Errorf("firmware mismatch evidence has impossible gates")
		}
	default:
		if gateCount != 0 {
			return fmt.Errorf("detection evidence reason has impossible gates")
		}
	}
	return nil
}

func validateDetectionDecision(
	decision Decision,
	limits Limits,
) error {
	if err := validateDetectionLimits(limits); err != nil {
		return err
	}
	if !validDetectionOutcome(decision.outcome) ||
		!validDetectionReason(decision.reason) ||
		len(decision.evidence) == 0 ||
		len(decision.evidence) > modbusreg.MaxProfileDependencies ||
		!validEvidenceOrder(decision.evidence) {
		return fmt.Errorf("detection decision is invalid")
	}
	selectedCount := 0
	equalBestCount := 0
	multipleMatchCount := 0
	lowerScoreCount := 0
	var equalBestScore uint32
	var selectedScore uint32
	var detectorVersion modbusreg.Version
	overallReasonPresent := false
	for index, evidence := range decision.evidence {
		if err := validateDetectionEvidence(evidence, limits); err != nil {
			return err
		}
		if index == 0 {
			detectorVersion = evidence.DetectorVersion
		} else if evidence.DetectorVersion != detectorVersion {
			return fmt.Errorf("detection evidence contract versions disagree")
		}
		switch evidence.Reason {
		case ReasonSelected:
			selectedCount++
			selectedScore = evidence.Score
		case ReasonEqualBest:
			if equalBestCount == 0 {
				equalBestScore = evidence.Score
			} else if evidence.Score != equalBestScore {
				return fmt.Errorf("ambiguous candidates do not have equal scores")
			}
			equalBestCount++
		case ReasonMultipleMatches:
			multipleMatchCount++
		case ReasonLowerScore:
			lowerScoreCount++
		}
		overallReasonPresent = overallReasonPresent || evidence.Reason == decision.reason
	}
	switch decision.outcome {
	case Matched:
		if decision.reason != ReasonSelected ||
			!modbusreg.ValidIdentity(decision.selectedProfileID) ||
			!decision.selectedProfileVersion.Valid() ||
			selectedCount != 1 || equalBestCount != 0 || multipleMatchCount != 0 {
			return fmt.Errorf("matched detection decision is inconsistent")
		}
		found := false
		for _, evidence := range decision.evidence {
			if evidence.Reason == ReasonSelected {
				found = evidence.ProfileID == decision.selectedProfileID &&
					evidence.ProfileVersion == decision.selectedProfileVersion
			}
		}
		if !found {
			return fmt.Errorf("selected profile is absent from decision evidence")
		}
		for _, evidence := range decision.evidence {
			if evidence.Reason == ReasonLowerScore &&
				evidence.Score >= selectedScore {
				return fmt.Errorf("matched detection ranking is inconsistent")
			}
		}
	case Ambiguous:
		if decision.selectedProfileID != "" ||
			decision.selectedProfileVersion.Valid() || selectedCount != 0 {
			return fmt.Errorf("ambiguous detection decision is inconsistent")
		}
		switch decision.reason {
		case ReasonEqualBest:
			if equalBestCount < 2 || multipleMatchCount != 0 {
				return fmt.Errorf("ambiguous detection decision is inconsistent")
			}
			for _, evidence := range decision.evidence {
				if evidence.Reason == ReasonLowerScore &&
					evidence.Score >= equalBestScore {
					return fmt.Errorf("ambiguous detection ranking is inconsistent")
				}
			}
		case ReasonMultipleMatches:
			if multipleMatchCount < 2 || equalBestCount != 0 || lowerScoreCount != 0 {
				return fmt.Errorf("exclusive detection decision is inconsistent")
			}
		default:
			return fmt.Errorf("ambiguous detection decision is inconsistent")
		}
	case NoMatch:
		if decision.reason == ReasonSelected ||
			decision.reason == ReasonEqualBest ||
			decision.reason == ReasonMultipleMatches ||
			decision.reason == ReasonLowerScore ||
			decision.selectedProfileID != "" ||
			decision.selectedProfileVersion.Valid() ||
			selectedCount != 0 || equalBestCount != 0 || multipleMatchCount != 0 || lowerScoreCount != 0 ||
			!overallReasonPresent {
			return fmt.Errorf("no-match detection decision is inconsistent")
		}
	default:
		return fmt.Errorf("detection decision outcome is invalid")
	}
	return nil
}

func detectionDecisionToDTO(
	decision Decision,
) detectionDecisionDTO {
	evidence := make([]detectionCandidateEvidenceDTO, len(decision.evidence))
	for index, item := range decision.evidence {
		evidence[index] = detectionCandidateEvidenceDTO{
			ProfileID:            item.ProfileID,
			ProfileVersion:       item.ProfileVersion.String(),
			Score:                item.Score,
			Reason:               item.Reason,
			MatchedGates:         append([]ProbeIdentityField{}, item.MatchedGates...),
			ProbeEvidenceIDs:     append([]string{}, item.ProbeEvidenceIDs...),
			DetectorVersion:      item.DetectorVersion.String(),
			QualificationVersion: item.QualificationVersion.String(),
		}
	}
	selectedVersion := ""
	if decision.selectedProfileVersion.Valid() {
		selectedVersion = decision.selectedProfileVersion.String()
	}
	return detectionDecisionDTO{
		SchemaVersion:          modbusreg.CurrentSchemaVersion().String(),
		Outcome:                decision.outcome,
		Reason:                 decision.reason,
		SelectedProfileID:      decision.selectedProfileID,
		SelectedProfileVersion: selectedVersion,
		Evidence:               evidence,
	}
}

// MarshalDetectionDecision emits the canonical bounded durable decision form.
func MarshalDetectionDecision(decision Decision) ([]byte, error) {
	limits := DefaultDetectionLimits()
	if decision.maxDecisionBytes > 0 {
		limits.MaxDecisionBytes = decision.maxDecisionBytes
	}
	if err := validateDetectionDecision(decision, limits); err != nil {
		return nil, err
	}
	encoded, err := modbusreg.MarshalBounded(detectionDecisionToDTO(decision))
	if err != nil {
		return nil, err
	}
	if len(encoded) > limits.MaxDecisionBytes {
		return nil, fmt.Errorf("detection decision exceeds the configured byte bound")
	}
	return encoded, nil
}

func detectionEvidenceFromDTO(
	record detectionCandidateEvidenceDTO,
) (CandidateEvidence, error) {
	profileVersion, err := modbusreg.ParseRequiredVersion("profile_version", record.ProfileVersion)
	if err != nil {
		return CandidateEvidence{}, err
	}
	detectorVersion, err := modbusreg.ParseRequiredVersion("detector_version", record.DetectorVersion)
	if err != nil {
		return CandidateEvidence{}, err
	}
	qualificationVersion, err := modbusreg.ParseRequiredVersion(
		"qualification_version",
		record.QualificationVersion,
	)
	if err != nil {
		return CandidateEvidence{}, err
	}
	return CandidateEvidence{
		ProfileID:            record.ProfileID,
		ProfileVersion:       profileVersion,
		Score:                record.Score,
		Reason:               record.Reason,
		MatchedGates:         append([]ProbeIdentityField(nil), record.MatchedGates...),
		ProbeEvidenceIDs:     append([]string(nil), record.ProbeEvidenceIDs...),
		DetectorVersion:      detectorVersion,
		QualificationVersion: qualificationVersion,
	}, nil
}

// UnmarshalDetectionDecision strictly reconstructs one canonical bounded
// decision. Unknown, missing, duplicate, and case-folded keys are rejected.
func UnmarshalDetectionDecision(data []byte) (Decision, error) {
	var record detectionDecisionDTO
	if err := modbusreg.DecodeStrict(data, &record); err != nil {
		return Decision{}, err
	}
	schemaVersion, err := modbusreg.ParseRequiredVersion("schema_version", record.SchemaVersion)
	if err != nil || schemaVersion != modbusreg.CurrentSchemaVersion() {
		return Decision{}, fmt.Errorf("detection decision schema is incompatible")
	}
	selectedVersion := modbusreg.Version{}
	if record.SelectedProfileVersion != "" {
		selectedVersion, err = modbusreg.ParseRequiredVersion(
			"selected_profile_version",
			record.SelectedProfileVersion,
		)
		if err != nil {
			return Decision{}, err
		}
	}
	evidence := make([]CandidateEvidence, len(record.Evidence))
	for index, item := range record.Evidence {
		evidence[index], err = detectionEvidenceFromDTO(item)
		if err != nil {
			return Decision{}, err
		}
	}
	if !sort.SliceIsSorted(evidence, func(first, second int) bool {
		if evidence[first].ProfileID != evidence[second].ProfileID {
			return evidence[first].ProfileID < evidence[second].ProfileID
		}
		return modbusreg.CompareVersions(
			evidence[first].ProfileVersion,
			evidence[second].ProfileVersion,
		) < 0
	}) {
		return Decision{}, fmt.Errorf("detection evidence is not canonical")
	}
	decision := Decision{
		outcome:                record.Outcome,
		reason:                 record.Reason,
		selectedProfileID:      record.SelectedProfileID,
		selectedProfileVersion: selectedVersion,
		evidence:               evidence,
		maxDecisionBytes:       modbusreg.MaxSerializedContractBytes,
	}
	if err := validateDetectionDecision(decision, DefaultDetectionLimits()); err != nil {
		return Decision{}, err
	}
	reencoded, err := MarshalDetectionDecision(decision)
	if err != nil {
		return Decision{}, err
	}
	if len(reencoded) > modbusreg.MaxSerializedContractBytes {
		return Decision{}, fmt.Errorf("detection decision exceeds the byte bound")
	}
	return decision, nil
}
