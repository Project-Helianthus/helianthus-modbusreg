package detection

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"unicode/utf8"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// IdentityStringGateSpec is one exact case-sensitive identity gate.
type IdentityStringGateSpec struct {
	Expected string
}

// FirmwareGateSpec is a half-open semantic firmware interval.
type FirmwareGateSpec struct {
	MinimumInclusive modbusreg.Version
	MaximumExclusive modbusreg.Version
}

// CandidateSpec binds gates and ranking to one exact catalog profile.
type CandidateSpec struct {
	ProfileID      string
	ProfileVersion modbusreg.Version
	Score          uint32
	Enabled        bool
	FixtureOnly    bool
	Manufacturer   IdentityStringGateSpec
	Model          IdentityStringGateSpec
	Firmware       FirmwareGateSpec
}

// Candidate is an immutable profile detection declaration.
type Candidate struct {
	spec CandidateSpec
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

// NewCandidate validates exact identity and semantic firmware gates.
func NewCandidate(
	spec CandidateSpec,
	limits Limits,
) (Candidate, error) {
	if err := validateDetectionLimits(limits); err != nil {
		return Candidate{}, err
	}
	if !modbusreg.ValidIdentity(spec.ProfileID) || !spec.ProfileVersion.Valid() ||
		spec.Score == 0 ||
		!validExpectedIdentity(spec.Manufacturer.Expected, limits.MaxIdentityBytes) ||
		!validExpectedIdentity(spec.Model.Expected, limits.MaxIdentityBytes) ||
		!spec.Firmware.MinimumInclusive.Valid() ||
		!spec.Firmware.MaximumExclusive.Valid() ||
		modbusreg.CompareVersions(
			spec.Firmware.MinimumInclusive,
			spec.Firmware.MaximumExclusive,
		) >= 0 {
		return Candidate{}, fmt.Errorf("detection candidate is invalid")
	}
	if err := modbusreg.PreflightAggregate(spec); err != nil {
		return Candidate{}, err
	}
	return Candidate{spec: spec}, nil
}

// Spec returns an independent complete candidate declaration.
func (candidate Candidate) Spec() CandidateSpec {
	return candidate.spec
}

// ProfileDetectorSpec binds one plan and candidate set to an immutable catalog.
type ProfileDetectorSpec struct {
	DetectorVersion modbusreg.Version
	Plan            ProbePlan
	Catalog         modbusreg.Catalog
	Candidates      []Candidate
	Limits          Limits
}

type boundDetectionCandidate struct {
	candidate Candidate
	profile   modbusreg.ProfileDescriptor
}

// ProfileDetector is an immutable concurrent-safe detector declaration.
type ProfileDetector struct {
	detectorVersion modbusreg.Version
	plan            ProbePlan
	candidates      []boundDetectionCandidate
	limits          Limits
}

func detectionDecisionUpperBound(
	candidates []boundDetectionCandidate,
	plan ProbePlan,
	limits Limits,
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
	if !spec.DetectorVersion.Valid() || len(spec.Candidates) == 0 ||
		len(spec.Candidates) > modbusreg.MaxProfileDependencies {
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
	byID := make(map[string]modbusreg.ProfileDescriptor, len(profiles))
	for _, profile := range profiles {
		byID[profile.ID()] = profile
	}
	bound := make([]boundDetectionCandidate, len(spec.Candidates))
	seen := make(map[string]struct{}, len(spec.Candidates))
	for index, declared := range spec.Candidates {
		candidate, candidateErr := NewCandidate(declared.Spec(), spec.Limits)
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
			!profile.QualificationVersion().Valid() {
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
		return modbusreg.CompareVersions(firstProfile.Version(), secondProfile.Version()) < 0
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

// Outcome is the closed detector decision set.
type Outcome string

// Detection outcomes. Matched names exactly one selected profile.
// Ambiguous names none: two or more candidates matched and neither
// score nor order picks a winner. NoMatch names none and carries the
// reason that explains it, including every probe or context failure.
const (
	Matched   Outcome = "matched"
	NoMatch   Outcome = "no_match"
	Ambiguous Outcome = "ambiguous"
)

// Reason is the closed decision and candidate evidence reason set.
type Reason string

// Detection reasons, used both as the decision reason and per candidate.
//
// Ranking: Selected marks the single matching candidate; MultipleMatches marks
// every matching candidate of an ambiguous decision. EqualBest and LowerScore
// are score-ranking reasons that decision decoding still validates; Detect
// never emits them.
//
// Candidate rejection: IdentityMismatch (manufacturer or model differs),
// FirmwareMismatch (firmware outside the candidate's half-open range),
// InvalidIdentity (unparsable or inconsistent identity), FixtureOptInRequired
// (fixture-only candidate without Options.AllowFixtureOnly), and the
// eligibility reasons CandidateDisabled, ProfileDefaultOff, ProfileRevoked,
// ProfileSuperseded and ProfileUnqualified.
//
// Probe failure, always with NoMatch: ReadError (the reader returned
// an error), ProbeException (Modbus exception response), IncompleteProbe
// (fewer or more words than declared), InvalidProbeResult (result outside the
// detector limits or a duplicated evidence identity) and ContextCancelled.
const (
	ReasonSelected             Reason = "selected"
	ReasonEqualBest            Reason = "equal_best"
	ReasonMultipleMatches      Reason = "multiple_matches"
	ReasonLowerScore           Reason = "lower_score"
	ReasonIdentityMismatch     Reason = "identity_mismatch"
	ReasonFirmwareMismatch     Reason = "firmware_mismatch"
	ReasonInvalidIdentity      Reason = "invalid_identity"
	ReasonFixtureOptInRequired Reason = "fixture_opt_in_required"
	ReasonCandidateDisabled    Reason = "candidate_disabled"
	ReasonProfileDefaultOff    Reason = "profile_default_off"
	ReasonProfileRevoked       Reason = "profile_revoked"
	ReasonProfileSuperseded    Reason = "profile_superseded"
	ReasonProfileUnqualified   Reason = "profile_unqualified"
	ReasonReadError            Reason = "read_error"
	ReasonProbeException       Reason = "probe_exception"
	ReasonIncompleteProbe      Reason = "incomplete_probe"
	ReasonInvalidProbeResult   Reason = "invalid_probe_result"
	ReasonContextCancelled     Reason = "context_cancelled"
)

// Options contains per-call policy that cannot alter declarations.
type Options struct {
	AllowFixtureOnly bool
}

// CandidateEvidence is one immutable-decision candidate projection.
// Decision.Evidence returns deep copies of these values.
type CandidateEvidence struct {
	ProfileID            string
	ProfileVersion       modbusreg.Version
	Score                uint32
	Reason               Reason
	MatchedGates         []ProbeIdentityField
	ProbeEvidenceIDs     []string
	DetectorVersion      modbusreg.Version
	QualificationVersion modbusreg.Version
}

// Decision is one immutable bounded selection result.
type Decision struct {
	outcome                Outcome
	reason                 Reason
	selectedProfileID      string
	selectedProfileVersion modbusreg.Version
	evidence               []CandidateEvidence
	maxDecisionBytes       int
}

func cloneDetectionEvidence(
	values []CandidateEvidence,
) []CandidateEvidence {
	result := make([]CandidateEvidence, len(values))
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
func (decision Decision) Outcome() Outcome { return decision.outcome }

// Reason returns the closed overall decision reason.
func (decision Decision) Reason() Reason { return decision.reason }

// SelectedProfileID is empty unless the outcome is matched.
func (decision Decision) SelectedProfileID() string {
	return decision.selectedProfileID
}

// SelectedProfileVersion is zero unless the outcome is matched.
func (decision Decision) SelectedProfileVersion() modbusreg.Version {
	return decision.selectedProfileVersion
}

// Evidence returns an independent deterministic candidate evidence copy.
func (decision Decision) Evidence() []CandidateEvidence {
	return cloneDetectionEvidence(decision.evidence)
}

type detectedIdentity struct {
	value      string
	evidenceID string
}

func (detector *ProfileDetector) baseEvidence(
	reason Reason,
) []CandidateEvidence {
	evidence := make([]CandidateEvidence, len(detector.candidates))
	for index, candidate := range detector.candidates {
		evidence[index] = CandidateEvidence{
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
	outcome Outcome,
	reason Reason,
	selectedProfileID string,
	selectedProfileVersion modbusreg.Version,
	evidence []CandidateEvidence,
) Decision {
	return Decision{
		outcome:                outcome,
		reason:                 reason,
		selectedProfileID:      selectedProfileID,
		selectedProfileVersion: selectedProfileVersion,
		evidence:               cloneDetectionEvidence(evidence),
		maxDecisionBytes:       detector.limits.MaxDecisionBytes,
	}
}

func (detector *ProfileDetector) failureDecision(
	reason Reason,
) Decision {
	return detector.decision(
		NoMatch,
		reason,
		"",
		modbusreg.Version{},
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
	profile modbusreg.ProfileDescriptor,
	candidate CandidateSpec,
	options Options,
) Reason {
	switch profile.State() {
	case modbusreg.ProfileRevoked:
		return ReasonProfileRevoked
	case modbusreg.ProfileSuperseded:
		return ReasonProfileSuperseded
	case modbusreg.ProfileActive:
	default:
		return ReasonInvalidIdentity
	}
	if profile.Maturity() != modbusreg.MaturityQualified {
		return ReasonProfileUnqualified
	}
	if !profile.DefaultEnabled() {
		return ReasonProfileDefaultOff
	}
	if !candidate.Enabled {
		return ReasonCandidateDisabled
	}
	if candidate.FixtureOnly && !options.AllowFixtureOnly {
		return ReasonFixtureOptInRequired
	}
	return ""
}

func evaluateCandidate(
	bound boundDetectionCandidate,
	identities map[ProbeIdentityField]detectedIdentity,
	options Options,
) CandidateEvidence {
	spec := bound.candidate.spec
	evidence := CandidateEvidence{
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
		evidence.Reason = ReasonIdentityMismatch
		return evidence
	}
	evidence.MatchedGates = append(evidence.MatchedGates, ProbeIdentityManufacturer)
	evidence.ProbeEvidenceIDs = append(evidence.ProbeEvidenceIDs, manufacturer.evidenceID)
	model := identities[ProbeIdentityModel]
	if model.value != spec.Model.Expected {
		evidence.Reason = ReasonIdentityMismatch
		return evidence
	}
	evidence.MatchedGates = append(evidence.MatchedGates, ProbeIdentityModel)
	evidence.ProbeEvidenceIDs = append(evidence.ProbeEvidenceIDs, model.evidenceID)
	firmware := identities[ProbeIdentityFirmware]
	firmwareVersion, err := modbusreg.ParseVersion(firmware.value)
	if err != nil {
		evidence.Reason = ReasonInvalidIdentity
		return evidence
	}
	if modbusreg.CompareVersions(firmwareVersion, spec.Firmware.MinimumInclusive) < 0 ||
		modbusreg.CompareVersions(firmwareVersion, spec.Firmware.MaximumExclusive) >= 0 {
		evidence.Reason = ReasonFirmwareMismatch
		return evidence
	}
	evidence.MatchedGates = append(evidence.MatchedGates, ProbeIdentityFirmware)
	evidence.ProbeEvidenceIDs = append(evidence.ProbeEvidenceIDs, firmware.evidenceID)
	evidence.Reason = ReasonSelected
	return evidence
}

func noMatchReason(evidence []CandidateEvidence) Reason {
	priorities := []Reason{
		ReasonInvalidIdentity,
		ReasonIdentityMismatch,
		ReasonFirmwareMismatch,
		ReasonFixtureOptInRequired,
		ReasonProfileRevoked,
		ReasonProfileSuperseded,
		ReasonProfileUnqualified,
		ReasonProfileDefaultOff,
		ReasonCandidateDisabled,
	}
	for _, priority := range priorities {
		for _, candidate := range evidence {
			if candidate.Reason == priority {
				return priority
			}
		}
	}
	return ReasonInvalidIdentity
}

func (detector *ProfileDetector) finalizeDecision(
	decision Decision,
) (Decision, error) {
	if _, err := MarshalDetectionDecision(decision); err != nil {
		return Decision{}, err
	}
	return decision, nil
}

// Detect executes each read serially in declaration order, then evaluates and
// ranks immutable catalog candidates without mutating detector state.
func (detector *ProfileDetector) Detect(
	ctx context.Context,
	reader ProbeReader,
	options Options,
) (Decision, error) {
	if detector == nil || ctx == nil || readerIsNil(reader) {
		return Decision{}, fmt.Errorf("profile detection request is invalid")
	}
	identities := make(map[ProbeIdentityField]detectedIdentity, len(detector.plan.spec.Declarations))
	evidenceIDs := make(map[string]struct{}, len(detector.plan.spec.Declarations))
	totalWords := 0
	for _, declaration := range detector.plan.spec.Declarations {
		if err := ctx.Err(); err != nil {
			decision := detector.failureDecision(ReasonContextCancelled)
			validated, decisionErr := detector.finalizeDecision(decision)
			if decisionErr != nil {
				return Decision{}, decisionErr
			}
			return validated, err
		}
		result, err := reader.ReadProbe(ctx, probeRequest(declaration))
		if contextErr := ctx.Err(); contextErr != nil {
			decision := detector.failureDecision(ReasonContextCancelled)
			validated, decisionErr := detector.finalizeDecision(decision)
			if decisionErr != nil {
				return Decision{}, decisionErr
			}
			return validated, contextErr
		}
		if err != nil {
			return detector.finalizeDecision(detector.failureDecision(ReasonReadError))
		}
		if !result.validWithin(detector.limits) {
			return detector.finalizeDecision(detector.failureDecision(ReasonInvalidProbeResult))
		}
		if result.status == ProbeReadException {
			return detector.finalizeDecision(detector.failureDecision(ReasonProbeException))
		}
		if len(result.words) != int(declaration.WordCount) {
			return detector.finalizeDecision(detector.failureDecision(ReasonIncompleteProbe))
		}
		if totalWords > detector.limits.MaxTotalWords-len(result.words) {
			return detector.finalizeDecision(detector.failureDecision(ReasonInvalidProbeResult))
		}
		totalWords += len(result.words)
		if _, duplicate := evidenceIDs[result.evidenceID]; duplicate {
			return detector.finalizeDecision(detector.failureDecision(ReasonInvalidProbeResult))
		}
		evidenceIDs[result.evidenceID] = struct{}{}
		identity, decodeErr := decodeProbeASCII(
			result.words,
			detector.limits.MaxIdentityBytes,
		)
		if decodeErr != nil {
			return detector.finalizeDecision(detector.failureDecision(ReasonInvalidIdentity))
		}
		if prior, duplicate := identities[declaration.IdentityField]; duplicate {
			if prior.value != identity || prior.evidenceID != result.evidenceID {
				return detector.finalizeDecision(detector.failureDecision(ReasonInvalidIdentity))
			}
			return detector.finalizeDecision(detector.failureDecision(ReasonInvalidProbeResult))
		}
		identities[declaration.IdentityField] = detectedIdentity{
			value:      identity,
			evidenceID: result.evidenceID,
		}
	}
	firmware := identities[ProbeIdentityFirmware]
	if _, err := modbusreg.ParseVersion(firmware.value); err != nil {
		return detector.finalizeDecision(detector.failureDecision(ReasonInvalidIdentity))
	}
	evidence := make([]CandidateEvidence, len(detector.candidates))
	matched := make([]int, 0, len(detector.candidates))
	for index, candidate := range detector.candidates {
		evidence[index] = evaluateCandidate(candidate, identities, options)
		if evidence[index].Reason == ReasonSelected {
			matched = append(matched, index)
		}
	}
	if len(matched) == 0 {
		return detector.finalizeDecision(detector.decision(
			NoMatch,
			noMatchReason(evidence),
			"",
			modbusreg.Version{},
			evidence,
		))
	}
	if len(matched) > 1 {
		for _, index := range matched {
			evidence[index].Reason = ReasonMultipleMatches
		}
		return detector.finalizeDecision(detector.decision(
			Ambiguous,
			ReasonMultipleMatches,
			"",
			modbusreg.Version{},
			evidence,
		))
	}
	selected := evidence[matched[0]]
	return detector.finalizeDecision(detector.decision(
		Matched,
		ReasonSelected,
		selected.ProfileID,
		selected.ProfileVersion,
		evidence,
	))
}
