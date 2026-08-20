package modbusreg

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"unicode/utf8"
)

// IdentityStringGateSpec is one exact case-sensitive identity gate.
type IdentityStringGateSpec struct {
	Expected string
}

// FirmwareGateSpec is a half-open semantic firmware interval.
type FirmwareGateSpec struct {
	MinimumInclusive Version
	MaximumExclusive Version
}

// DetectionCandidateSpec binds gates and ranking to one exact catalog profile.
type DetectionCandidateSpec struct {
	ProfileID      string
	ProfileVersion Version
	Score          uint32
	Enabled        bool
	FixtureOnly    bool
	Manufacturer   IdentityStringGateSpec
	Model          IdentityStringGateSpec
	Firmware       FirmwareGateSpec
}

// DetectionCandidate is an immutable profile detection declaration.
type DetectionCandidate struct {
	spec DetectionCandidateSpec
}

func validExpectedIdentity(value string, maximum int) bool {
	if value == "" || len(value) > maximum || !utf8.ValidString(value) {
		return false
	}
	for _, character := range []byte(value) {
		if character < 0x20 || character > 0x7e {
			return false
		}
	}
	return true
}

// NewDetectionCandidate validates exact identity and semantic firmware gates.
func NewDetectionCandidate(
	spec DetectionCandidateSpec,
	limits DetectionLimits,
) (DetectionCandidate, error) {
	if err := validateDetectionLimits(limits); err != nil {
		return DetectionCandidate{}, err
	}
	if !validIdentity(spec.ProfileID) || !spec.ProfileVersion.valid() ||
		spec.Score == 0 ||
		!validExpectedIdentity(spec.Manufacturer.Expected, limits.MaxIdentityBytes) ||
		!validExpectedIdentity(spec.Model.Expected, limits.MaxIdentityBytes) ||
		!spec.Firmware.MinimumInclusive.valid() ||
		!spec.Firmware.MaximumExclusive.valid() ||
		compareVersion(
			spec.Firmware.MinimumInclusive,
			spec.Firmware.MaximumExclusive,
		) >= 0 {
		return DetectionCandidate{}, fmt.Errorf("detection candidate is invalid")
	}
	if err := preflightAggregate(spec); err != nil {
		return DetectionCandidate{}, err
	}
	return DetectionCandidate{spec: spec}, nil
}

// Spec returns an independent complete candidate declaration.
func (candidate DetectionCandidate) Spec() DetectionCandidateSpec {
	return candidate.spec
}

// ProfileDetectorSpec binds one plan and candidate set to an immutable catalog.
type ProfileDetectorSpec struct {
	DetectorVersion Version
	Plan            ProbePlan
	Catalog         Catalog
	Candidates      []DetectionCandidate
	Limits          DetectionLimits
}

type boundDetectionCandidate struct {
	candidate DetectionCandidate
	profile   ProfileDescriptor
}

// ProfileDetector is an immutable concurrent-safe detector declaration.
type ProfileDetector struct {
	detectorVersion Version
	plan            ProbePlan
	candidates      []boundDetectionCandidate
	limits          DetectionLimits
}

func detectionDecisionUpperBound(
	candidates []boundDetectionCandidate,
	plan ProbePlan,
	limits DetectionLimits,
) error {
	bound := uint64(512)
	perEvidenceID := uint64(limits.MaxEvidenceIDBytes)*6 + 128
	planCount := uint64(len(plan.spec.Declarations))
	if planCount != 0 && perEvidenceID > math.MaxUint64/planCount {
		return fmt.Errorf("detection decision bound overflows")
	}
	for _, candidate := range candidates {
		profileBytes := uint64(len(candidate.profile.ID())) * 6
		versionBytes := uint64(len(candidate.profile.Version().String())+
			len(candidate.profile.DetectorVersion().String())+
			len(candidate.profile.QualificationVersion().String())) * 6
		candidateBound := uint64(768) + profileBytes + versionBytes +
			planCount*perEvidenceID
		if candidateBound > math.MaxUint64-bound {
			return fmt.Errorf("detection decision bound overflows")
		}
		bound += candidateBound
	}
	if bound > uint64(limits.MaxDecisionBytes) {
		return fmt.Errorf("detection decision exceeds the configured byte bound")
	}
	return nil
}

// NewProfileDetector validates exact catalog/version bindings before any read.
func NewProfileDetector(spec ProfileDetectorSpec) (*ProfileDetector, error) {
	if err := validateDetectionLimits(spec.Limits); err != nil {
		return nil, err
	}
	if !spec.DetectorVersion.valid() || len(spec.Candidates) == 0 ||
		len(spec.Candidates) > MaxProfileDependencies {
		return nil, fmt.Errorf("profile detector declaration is invalid")
	}
	plan, err := NewProbePlan(spec.Plan.Spec(), spec.Limits)
	if err != nil {
		return nil, err
	}
	if plan.Version() != spec.DetectorVersion {
		return nil, fmt.Errorf("probe plan and detector versions disagree")
	}
	fields := make(map[ProbeIdentityField]struct{}, len(plan.spec.Declarations))
	for _, declaration := range plan.spec.Declarations {
		fields[declaration.IdentityField] = struct{}{}
	}
	for _, required := range []ProbeIdentityField{
		ProbeIdentityManufacturer,
		ProbeIdentityModel,
		ProbeIdentityFirmware,
	} {
		if _, exists := fields[required]; !exists {
			return nil, fmt.Errorf("probe plan lacks a required identity declaration")
		}
	}
	profiles := spec.Catalog.Profiles()
	if len(profiles) == 0 {
		return nil, fmt.Errorf("profile detector catalog is empty")
	}
	byID := make(map[string]ProfileDescriptor, len(profiles))
	for _, profile := range profiles {
		byID[profile.ID()] = profile
	}
	bound := make([]boundDetectionCandidate, len(spec.Candidates))
	seen := make(map[string]struct{}, len(spec.Candidates))
	for index, declared := range spec.Candidates {
		candidate, candidateErr := NewDetectionCandidate(declared.Spec(), spec.Limits)
		if candidateErr != nil {
			return nil, candidateErr
		}
		candidateSpec := candidate.spec
		if _, duplicate := seen[candidateSpec.ProfileID]; duplicate {
			return nil, fmt.Errorf("detection candidate profile binding is duplicated")
		}
		profile, exists := byID[candidateSpec.ProfileID]
		if !exists || profile.Version() != candidateSpec.ProfileVersion {
			return nil, fmt.Errorf("detection candidate profile binding is absent")
		}
		if profile.DetectorVersion() != spec.DetectorVersion ||
			!profile.QualificationVersion().valid() {
			return nil, fmt.Errorf("detection candidate contract versions disagree")
		}
		seen[candidateSpec.ProfileID] = struct{}{}
		bound[index] = boundDetectionCandidate{
			candidate: candidate,
			profile:   profile,
		}
	}
	sort.Slice(bound, func(first, second int) bool {
		firstProfile := bound[first].profile
		secondProfile := bound[second].profile
		if firstProfile.ID() != secondProfile.ID() {
			return firstProfile.ID() < secondProfile.ID()
		}
		return compareVersion(firstProfile.Version(), secondProfile.Version()) < 0
	})
	if err := detectionDecisionUpperBound(bound, plan, spec.Limits); err != nil {
		return nil, err
	}
	return &ProfileDetector{
		detectorVersion: spec.DetectorVersion,
		plan:            plan,
		candidates:      bound,
		limits:          spec.Limits,
	}, nil
}

// DetectionOutcome is the closed detector decision set.
type DetectionOutcome string

const (
	DetectionMatched   DetectionOutcome = "matched"
	DetectionNoMatch   DetectionOutcome = "no_match"
	DetectionAmbiguous DetectionOutcome = "ambiguous"
)

// DetectionReason is the closed decision and candidate evidence reason set.
type DetectionReason string

const (
	DetectionReasonSelected             DetectionReason = "selected"
	DetectionReasonEqualBest            DetectionReason = "equal_best"
	DetectionReasonMultipleMatches      DetectionReason = "multiple_matches"
	DetectionReasonLowerScore           DetectionReason = "lower_score"
	DetectionReasonIdentityMismatch     DetectionReason = "identity_mismatch"
	DetectionReasonFirmwareMismatch     DetectionReason = "firmware_mismatch"
	DetectionReasonInvalidIdentity      DetectionReason = "invalid_identity"
	DetectionReasonFixtureOptInRequired DetectionReason = "fixture_opt_in_required"
	DetectionReasonCandidateDisabled    DetectionReason = "candidate_disabled"
	DetectionReasonProfileDefaultOff    DetectionReason = "profile_default_off"
	DetectionReasonProfileRevoked       DetectionReason = "profile_revoked"
	DetectionReasonProfileSuperseded    DetectionReason = "profile_superseded"
	DetectionReasonProfileUnqualified   DetectionReason = "profile_unqualified"
	DetectionReasonReadError            DetectionReason = "read_error"
	DetectionReasonProbeException       DetectionReason = "probe_exception"
	DetectionReasonIncompleteProbe      DetectionReason = "incomplete_probe"
	DetectionReasonInvalidProbeResult   DetectionReason = "invalid_probe_result"
	DetectionReasonContextCancelled     DetectionReason = "context_cancelled"
)

// DetectionOptions contains per-call policy that cannot alter declarations.
type DetectionOptions struct {
	AllowFixtureOnly bool
	// RequireExclusiveMatch rejects every multi-match before score ranking.
	RequireExclusiveMatch bool
}

// DetectionCandidateEvidence is one immutable-decision candidate projection.
// DetectionDecision.Evidence returns deep copies of these values.
type DetectionCandidateEvidence struct {
	ProfileID            string
	ProfileVersion       Version
	Score                uint32
	Reason               DetectionReason
	MatchedGates         []ProbeIdentityField
	ProbeEvidenceIDs     []string
	DetectorVersion      Version
	QualificationVersion Version
}

// DetectionDecision is one immutable bounded selection result.
type DetectionDecision struct {
	outcome                DetectionOutcome
	reason                 DetectionReason
	selectedProfileID      string
	selectedProfileVersion Version
	evidence               []DetectionCandidateEvidence
	maxDecisionBytes       int
}

func cloneDetectionEvidence(
	values []DetectionCandidateEvidence,
) []DetectionCandidateEvidence {
	result := make([]DetectionCandidateEvidence, len(values))
	for index, value := range values {
		result[index] = value
		result[index].MatchedGates = append(
			[]ProbeIdentityField(nil),
			value.MatchedGates...,
		)
		result[index].ProbeEvidenceIDs = append(
			[]string(nil),
			value.ProbeEvidenceIDs...,
		)
	}
	return result
}

// Outcome returns matched, no-match, or ambiguous.
func (decision DetectionDecision) Outcome() DetectionOutcome { return decision.outcome }

// Reason returns the closed overall decision reason.
func (decision DetectionDecision) Reason() DetectionReason { return decision.reason }

// SelectedProfileID is empty unless the outcome is matched.
func (decision DetectionDecision) SelectedProfileID() string {
	return decision.selectedProfileID
}

// SelectedProfileVersion is zero unless the outcome is matched.
func (decision DetectionDecision) SelectedProfileVersion() Version {
	return decision.selectedProfileVersion
}

// Evidence returns an independent deterministic candidate evidence copy.
func (decision DetectionDecision) Evidence() []DetectionCandidateEvidence {
	return cloneDetectionEvidence(decision.evidence)
}

type detectedIdentity struct {
	value      string
	evidenceID string
}

func (detector *ProfileDetector) baseEvidence(
	reason DetectionReason,
) []DetectionCandidateEvidence {
	evidence := make([]DetectionCandidateEvidence, len(detector.candidates))
	for index, candidate := range detector.candidates {
		evidence[index] = DetectionCandidateEvidence{
			ProfileID:            candidate.profile.ID(),
			ProfileVersion:       candidate.profile.Version(),
			Score:                candidate.candidate.spec.Score,
			Reason:               reason,
			DetectorVersion:      candidate.profile.DetectorVersion(),
			QualificationVersion: candidate.profile.QualificationVersion(),
		}
	}
	return evidence
}

func (detector *ProfileDetector) decision(
	outcome DetectionOutcome,
	reason DetectionReason,
	selectedProfileID string,
	selectedProfileVersion Version,
	evidence []DetectionCandidateEvidence,
) DetectionDecision {
	return DetectionDecision{
		outcome:                outcome,
		reason:                 reason,
		selectedProfileID:      selectedProfileID,
		selectedProfileVersion: selectedProfileVersion,
		evidence:               cloneDetectionEvidence(evidence),
		maxDecisionBytes:       detector.limits.MaxDecisionBytes,
	}
}

func (detector *ProfileDetector) failureDecision(
	reason DetectionReason,
) DetectionDecision {
	return detector.decision(
		DetectionNoMatch,
		reason,
		"",
		Version{},
		detector.baseEvidence(reason),
	)
}

func readerIsNil(reader ProbeReader) bool {
	if reader == nil {
		return true
	}
	value := reflect.ValueOf(reader)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func candidateEligibilityReason(
	profile ProfileDescriptor,
	candidate DetectionCandidateSpec,
	options DetectionOptions,
) DetectionReason {
	switch profile.spec.State {
	case ProfileRevoked:
		return DetectionReasonProfileRevoked
	case ProfileSuperseded:
		return DetectionReasonProfileSuperseded
	case ProfileActive:
	default:
		return DetectionReasonInvalidIdentity
	}
	if profile.spec.Maturity != MaturityQualified {
		return DetectionReasonProfileUnqualified
	}
	if !profile.spec.DefaultEnabled {
		return DetectionReasonProfileDefaultOff
	}
	if !candidate.Enabled {
		return DetectionReasonCandidateDisabled
	}
	if candidate.FixtureOnly && !options.AllowFixtureOnly {
		return DetectionReasonFixtureOptInRequired
	}
	return ""
}

func evaluateCandidate(
	bound boundDetectionCandidate,
	identities map[ProbeIdentityField]detectedIdentity,
	options DetectionOptions,
) DetectionCandidateEvidence {
	spec := bound.candidate.spec
	evidence := DetectionCandidateEvidence{
		ProfileID:            bound.profile.ID(),
		ProfileVersion:       bound.profile.Version(),
		Score:                spec.Score,
		DetectorVersion:      bound.profile.DetectorVersion(),
		QualificationVersion: bound.profile.QualificationVersion(),
	}
	if reason := candidateEligibilityReason(bound.profile, spec, options); reason != "" {
		evidence.Reason = reason
		return evidence
	}
	manufacturer := identities[ProbeIdentityManufacturer]
	if manufacturer.value != spec.Manufacturer.Expected {
		evidence.Reason = DetectionReasonIdentityMismatch
		return evidence
	}
	evidence.MatchedGates = append(evidence.MatchedGates, ProbeIdentityManufacturer)
	evidence.ProbeEvidenceIDs = append(evidence.ProbeEvidenceIDs, manufacturer.evidenceID)
	model := identities[ProbeIdentityModel]
	if model.value != spec.Model.Expected {
		evidence.Reason = DetectionReasonIdentityMismatch
		return evidence
	}
	evidence.MatchedGates = append(evidence.MatchedGates, ProbeIdentityModel)
	evidence.ProbeEvidenceIDs = append(evidence.ProbeEvidenceIDs, model.evidenceID)
	firmware := identities[ProbeIdentityFirmware]
	firmwareVersion, err := ParseVersion(firmware.value)
	if err != nil {
		evidence.Reason = DetectionReasonInvalidIdentity
		return evidence
	}
	if compareVersion(firmwareVersion, spec.Firmware.MinimumInclusive) < 0 ||
		compareVersion(firmwareVersion, spec.Firmware.MaximumExclusive) >= 0 {
		evidence.Reason = DetectionReasonFirmwareMismatch
		return evidence
	}
	evidence.MatchedGates = append(evidence.MatchedGates, ProbeIdentityFirmware)
	evidence.ProbeEvidenceIDs = append(evidence.ProbeEvidenceIDs, firmware.evidenceID)
	evidence.Reason = DetectionReasonSelected
	return evidence
}

func noMatchReason(evidence []DetectionCandidateEvidence) DetectionReason {
	priorities := []DetectionReason{
		DetectionReasonInvalidIdentity,
		DetectionReasonIdentityMismatch,
		DetectionReasonFirmwareMismatch,
		DetectionReasonFixtureOptInRequired,
		DetectionReasonProfileRevoked,
		DetectionReasonProfileSuperseded,
		DetectionReasonProfileUnqualified,
		DetectionReasonProfileDefaultOff,
		DetectionReasonCandidateDisabled,
	}
	for _, priority := range priorities {
		for _, candidate := range evidence {
			if candidate.Reason == priority {
				return priority
			}
		}
	}
	return DetectionReasonInvalidIdentity
}

func (detector *ProfileDetector) finalizeDecision(
	decision DetectionDecision,
) (DetectionDecision, error) {
	if _, err := MarshalDetectionDecision(decision); err != nil {
		return DetectionDecision{}, err
	}
	return decision, nil
}

// Detect executes each read serially in declaration order, then evaluates and
// ranks immutable catalog candidates without mutating detector state.
func (detector *ProfileDetector) Detect(
	ctx context.Context,
	reader ProbeReader,
	options DetectionOptions,
) (DetectionDecision, error) {
	if detector == nil || ctx == nil || readerIsNil(reader) {
		return DetectionDecision{}, fmt.Errorf("profile detection request is invalid")
	}
	identities := make(map[ProbeIdentityField]detectedIdentity, len(detector.plan.spec.Declarations))
	evidenceIDs := make(map[string]struct{}, len(detector.plan.spec.Declarations))
	totalWords := 0
	for _, declaration := range detector.plan.spec.Declarations {
		if err := ctx.Err(); err != nil {
			decision := detector.failureDecision(DetectionReasonContextCancelled)
			validated, decisionErr := detector.finalizeDecision(decision)
			if decisionErr != nil {
				return DetectionDecision{}, decisionErr
			}
			return validated, err
		}
		result, err := reader.ReadProbe(ctx, probeRequest(declaration))
		if contextErr := ctx.Err(); contextErr != nil {
			decision := detector.failureDecision(DetectionReasonContextCancelled)
			validated, decisionErr := detector.finalizeDecision(decision)
			if decisionErr != nil {
				return DetectionDecision{}, decisionErr
			}
			return validated, contextErr
		}
		if err != nil {
			return detector.finalizeDecision(detector.failureDecision(DetectionReasonReadError))
		}
		if !result.validWithin(detector.limits) {
			return detector.finalizeDecision(detector.failureDecision(DetectionReasonInvalidProbeResult))
		}
		if result.status == ProbeReadException {
			return detector.finalizeDecision(detector.failureDecision(DetectionReasonProbeException))
		}
		if len(result.words) != int(declaration.WordCount) {
			return detector.finalizeDecision(detector.failureDecision(DetectionReasonIncompleteProbe))
		}
		if totalWords > detector.limits.MaxTotalWords-len(result.words) {
			return detector.finalizeDecision(detector.failureDecision(DetectionReasonInvalidProbeResult))
		}
		totalWords += len(result.words)
		if _, duplicate := evidenceIDs[result.evidenceID]; duplicate {
			return detector.finalizeDecision(detector.failureDecision(DetectionReasonInvalidProbeResult))
		}
		evidenceIDs[result.evidenceID] = struct{}{}
		identity, decodeErr := decodeProbeASCII(
			result.words,
			detector.limits.MaxIdentityBytes,
		)
		if decodeErr != nil {
			return detector.finalizeDecision(detector.failureDecision(DetectionReasonInvalidIdentity))
		}
		if prior, duplicate := identities[declaration.IdentityField]; duplicate {
			if prior.value != identity || prior.evidenceID != result.evidenceID {
				return detector.finalizeDecision(detector.failureDecision(DetectionReasonInvalidIdentity))
			}
			return detector.finalizeDecision(detector.failureDecision(DetectionReasonInvalidProbeResult))
		}
		identities[declaration.IdentityField] = detectedIdentity{
			value:      identity,
			evidenceID: result.evidenceID,
		}
	}
	firmware := identities[ProbeIdentityFirmware]
	if _, err := ParseVersion(firmware.value); err != nil {
		return detector.finalizeDecision(detector.failureDecision(DetectionReasonInvalidIdentity))
	}
	evidence := make([]DetectionCandidateEvidence, len(detector.candidates))
	matched := make([]int, 0, len(detector.candidates))
	for index, candidate := range detector.candidates {
		evidence[index] = evaluateCandidate(candidate, identities, options)
		if evidence[index].Reason == DetectionReasonSelected {
			matched = append(matched, index)
		}
	}
	if len(matched) == 0 {
		return detector.finalizeDecision(detector.decision(
			DetectionNoMatch,
			noMatchReason(evidence),
			"",
			Version{},
			evidence,
		))
	}
	if options.RequireExclusiveMatch && len(matched) > 1 {
		for _, index := range matched {
			evidence[index].Reason = DetectionReasonMultipleMatches
		}
		return detector.finalizeDecision(detector.decision(
			DetectionAmbiguous,
			DetectionReasonMultipleMatches,
			"",
			Version{},
			evidence,
		))
	}
	highest := uint32(0)
	for _, index := range matched {
		if evidence[index].Score > highest {
			highest = evidence[index].Score
		}
	}
	best := make([]int, 0, len(matched))
	for _, index := range matched {
		if evidence[index].Score == highest {
			best = append(best, index)
		} else {
			evidence[index].Reason = DetectionReasonLowerScore
		}
	}
	if len(best) != 1 {
		for _, index := range best {
			evidence[index].Reason = DetectionReasonEqualBest
		}
		return detector.finalizeDecision(detector.decision(
			DetectionAmbiguous,
			DetectionReasonEqualBest,
			"",
			Version{},
			evidence,
		))
	}
	selected := evidence[best[0]]
	return detector.finalizeDecision(detector.decision(
		DetectionMatched,
		DetectionReasonSelected,
		selected.ProfileID,
		selected.ProfileVersion,
		evidence,
	))
}
