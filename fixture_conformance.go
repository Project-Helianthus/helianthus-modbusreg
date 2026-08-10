package modbusreg

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// FixtureMutationReason is the closed construction/decoding rejection set.
type FixtureMutationReason string

const (
	FixtureMutationReasonMissing       FixtureMutationReason = "missing"
	FixtureMutationReasonUnknown       FixtureMutationReason = "unknown"
	FixtureMutationReasonDuplicate     FixtureMutationReason = "duplicate"
	FixtureMutationReasonCaseFolded    FixtureMutationReason = "case_folded"
	FixtureMutationReasonMalformed     FixtureMutationReason = "malformed"
	FixtureMutationReasonOversized     FixtureMutationReason = "oversized"
	FixtureMutationReasonContradictory FixtureMutationReason = "contradictory"
	FixtureMutationReasonUnsanitized   FixtureMutationReason = "unsanitized"
)

type fixtureMutationError struct{ reason FixtureMutationReason }

func (err fixtureMutationError) Error() string {
	return "fixture corpus rejected: " + string(err.reason)
}

// IsFixtureMutationReason reports a stable M2-03 rejection classification.
func IsFixtureMutationReason(err error, reason FixtureMutationReason) bool {
	var classified fixtureMutationError
	return errors.As(err, &classified) && classified.reason == reason
}

// FixtureConformanceLimits bounds corpus-owned allocations.
type FixtureConformanceLimits struct {
	MaxRecords     int
	MaxReportBytes int
}

func defaultFixtureConformanceLimits() FixtureConformanceLimits {
	return FixtureConformanceLimits{MaxRecords: MaxProfileDependencies, MaxReportBytes: MaxSerializedContractBytes}
}

func validateFixtureConformanceLimits(limits FixtureConformanceLimits) error {
	if limits.MaxRecords <= 0 || limits.MaxRecords > MaxProfileDependencies || limits.MaxReportBytes <= 0 || limits.MaxReportBytes > MaxSerializedContractBytes {
		return fixtureMutationError{FixtureMutationReasonOversized}
	}
	return nil
}

// SanitizedFixtureMetadata is deliberately limited to public corpus identity.
type SanitizedFixtureMetadata struct {
	CorpusID          string `json:"corpus_id"`
	LicenseExpression string `json:"license_expression"`
	Provenance        string `json:"provenance"`
}

// FixtureDetectorDeclarationSpec is the serializable input used to rebuild an
// existing ProfileDetector against the immutable corpus catalog.
type FixtureDetectorDeclarationSpec struct {
	DetectorVersion Version                  `json:"detector_version"`
	Plan            ProbePlanSpec            `json:"plan"`
	Candidates      []DetectionCandidateSpec `json:"candidates"`
	Limits          DetectionLimits          `json:"limits"`
}

type FixtureProbeInput struct {
	DeclarationID string              `json:"declaration_id"`
	Result        ProbeReadResultSpec `json:"result"`
}

type FixtureDetectorInput struct {
	Probes []FixtureProbeInput `json:"probes"`
}

type FixtureDetectionEvidenceExpectation struct {
	ProfileID            string               `json:"profile_id"`
	ProfileVersion       Version              `json:"profile_version"`
	Score                uint32               `json:"score"`
	Reason               DetectionReason      `json:"reason"`
	MatchedGates         []ProbeIdentityField `json:"matched_gates"`
	ProbeEvidenceIDs     []string             `json:"probe_evidence_ids"`
	DetectorVersion      Version              `json:"detector_version"`
	QualificationVersion Version              `json:"qualification_version"`
}

type FixtureDetectionExpectation struct {
	Outcome                DetectionOutcome                      `json:"outcome"`
	Reason                 DetectionReason                       `json:"reason"`
	SelectedProfileID      string                                `json:"selected_profile_id"`
	SelectedProfileVersion Version                               `json:"selected_profile_version"`
	Evidence               []FixtureDetectionEvidenceExpectation `json:"evidence"`
}

type FixtureDetectionCaseSpec struct {
	Declaration FixtureDetectorDeclarationSpec `json:"declaration"`
	Input       FixtureDetectorInput           `json:"input"`
	Expected    FixtureDetectionExpectation    `json:"expected"`
}

type FixtureQualificationDisposition string

const (
	FixtureQualificationDispositionQualified FixtureQualificationDisposition = "qualified"
	FixtureQualificationDispositionRejected  FixtureQualificationDisposition = "rejected"
)

type FixtureQualificationExpectation struct {
	Expected FixtureQualificationDisposition `json:"expected"`
}

type FixtureReplayOutcome string

const (
	FixtureReplayAccepted FixtureReplayOutcome = "accepted"
	FixtureReplayRejected FixtureReplayOutcome = "rejected"
)

type FixtureReplayReason string

const (
	FixtureReplayReasonAccepted              FixtureReplayReason = "accepted"
	FixtureReplayReasonUnitMismatch          FixtureReplayReason = "unit_mismatch"
	FixtureReplayReasonTableAccessMismatch   FixtureReplayReason = "table_access_mismatch"
	FixtureReplayReasonGenerationMismatch    FixtureReplayReason = "generation_mismatch"
	FixtureReplayReasonSourceMismatch        FixtureReplayReason = "source_mismatch"
	FixtureReplayReasonNormalizationMismatch FixtureReplayReason = "normalization_mismatch"
	FixtureReplayReasonDeadlineMismatch      FixtureReplayReason = "deadline_mismatch"
	FixtureReplayReasonCoherenceMismatch     FixtureReplayReason = "coherence_mismatch"
	FixtureReplayReasonTornRead              FixtureReplayReason = "torn_read"
	FixtureReplayReasonObservationRejected   FixtureReplayReason = "observation_rejected"
)

type FixtureReplayExpectation struct {
	Outcome          FixtureReplayOutcome `json:"outcome"`
	Reason           FixtureReplayReason  `json:"reason"`
	ExpectedRawWords [][]uint16           `json:"expected_raw_words,omitempty"`
}

type FixtureConformanceRecordSpec struct {
	RecordID       string                          `json:"record_id"`
	ProfileID      string                          `json:"profile_id"`
	ProfileVersion Version                         `json:"profile_version"`
	Observation    ObservationSpec                 `json:"observation"`
	Detection      FixtureDetectionCaseSpec        `json:"detection"`
	Qualification  FixtureQualificationExpectation `json:"qualification"`
	ExpectedReplay FixtureReplayExpectation        `json:"expected_replay"`
}

type FixtureConformanceCorpusSpec struct {
	SchemaVersion Version                        `json:"schema_version"`
	Metadata      SanitizedFixtureMetadata       `json:"metadata"`
	Profiles      []ProfileDescriptor            `json:"profiles"`
	Records       []FixtureConformanceRecordSpec `json:"records"`
}

type FixtureConformanceCorpus struct {
	spec    FixtureConformanceCorpusSpec
	catalog Catalog
	limits  FixtureConformanceLimits
}

func cloneFixtureCorpusSpec(spec FixtureConformanceCorpusSpec) FixtureConformanceCorpusSpec {
	spec.Profiles = append([]ProfileDescriptor(nil), spec.Profiles...)
	for i := range spec.Profiles {
		spec.Profiles[i], _ = NewProfileDescriptor(spec.Profiles[i].Spec())
	}
	spec.Records = append([]FixtureConformanceRecordSpec(nil), spec.Records...)
	for i := range spec.Records {
		spec.Records[i] = cloneFixtureRecordSpec(spec.Records[i])
	}
	return spec
}

func cloneFixtureRecordSpec(record FixtureConformanceRecordSpec) FixtureConformanceRecordSpec {
	record.Observation = cloneObservationSpec(record.Observation)
	record.Detection.Declaration.Plan = cloneProbePlanSpec(record.Detection.Declaration.Plan)
	record.Detection.Declaration.Candidates = append([]DetectionCandidateSpec(nil), record.Detection.Declaration.Candidates...)
	record.Detection.Input.Probes = append([]FixtureProbeInput(nil), record.Detection.Input.Probes...)
	for i := range record.Detection.Input.Probes {
		record.Detection.Input.Probes[i].Result.Words = append([]uint16(nil), record.Detection.Input.Probes[i].Result.Words...)
	}
	record.Detection.Expected.Evidence = append([]FixtureDetectionEvidenceExpectation(nil), record.Detection.Expected.Evidence...)
	for i := range record.Detection.Expected.Evidence {
		record.Detection.Expected.Evidence[i].MatchedGates = append([]ProbeIdentityField(nil), record.Detection.Expected.Evidence[i].MatchedGates...)
		record.Detection.Expected.Evidence[i].ProbeEvidenceIDs = cloneStrings(record.Detection.Expected.Evidence[i].ProbeEvidenceIDs)
	}
	record.ExpectedReplay.ExpectedRawWords = cloneWordSets(record.ExpectedReplay.ExpectedRawWords)
	return record
}

func cloneWordSets(values [][]uint16) [][]uint16 {
	result := make([][]uint16, len(values))
	for i := range values {
		result[i] = append([]uint16(nil), values[i]...)
	}
	return result
}

func validFixtureIdentity(value string) bool {
	return validIdentity(value) && (strings.HasPrefix(value, "fixture-") || strings.HasPrefix(value, "fixture:"))
}

func validateSanitizedMetadata(metadata SanitizedFixtureMetadata) error {
	if validateBoundedString("fixture corpus ID", metadata.CorpusID, true) != nil ||
		validateBoundedString("fixture license expression", metadata.LicenseExpression, true) != nil ||
		validateBoundedString("fixture provenance", metadata.Provenance, true) != nil {
		return fixtureMutationError{FixtureMutationReasonOversized}
	}
	if !validIdentity(metadata.CorpusID) || metadata.LicenseExpression != "CC0-1.0" || metadata.Provenance != "public synthetic fixture" {
		return fixtureMutationError{FixtureMutationReasonUnsanitized}
	}
	return nil
}

func validateFixtureSanitization(record FixtureConformanceRecordSpec) error {
	if !validFixtureIdentity(record.Observation.Endpoint) {
		return fixtureMutationError{FixtureMutationReasonUnsanitized}
	}
	for _, dependency := range record.Observation.Dependencies {
		if !validFixtureIdentity(dependency.View.Record().Endpoint) || dependency.View.Record().Endpoint != record.Observation.Endpoint {
			return fixtureMutationError{FixtureMutationReasonUnsanitized}
		}
	}
	return nil
}

// NewFixtureConformanceCorpus constructs immutable offline evidence only.
func NewFixtureConformanceCorpus(spec FixtureConformanceCorpusSpec) (*FixtureConformanceCorpus, error) {
	return NewFixtureConformanceCorpusWithLimits(spec, defaultFixtureConformanceLimits())
}

func NewFixtureConformanceCorpusWithLimits(spec FixtureConformanceCorpusSpec, limits FixtureConformanceLimits) (*FixtureConformanceCorpus, error) {
	if err := validateFixtureConformanceLimits(limits); err != nil {
		return nil, err
	}
	if err := preflightAggregate(spec); err != nil {
		return nil, fixtureMutationError{FixtureMutationReasonOversized}
	}
	if len(spec.Profiles) > limits.MaxRecords || len(spec.Records) > limits.MaxRecords {
		return nil, fixtureMutationError{FixtureMutationReasonOversized}
	}
	if spec.SchemaVersion != schemaVersionV1 || len(spec.Profiles) == 0 || len(spec.Records) == 0 {
		return nil, fixtureMutationError{FixtureMutationReasonMalformed}
	}
	if err := validateSanitizedMetadata(spec.Metadata); err != nil {
		return nil, err
	}
	catalog, err := NewCatalog(spec.Profiles...)
	if err != nil {
		return nil, fixtureMutationError{FixtureMutationReasonMalformed}
	}
	copy := cloneFixtureCorpusSpec(spec)
	seen := make(map[string]struct{}, len(copy.Records))
	profiles := make(map[string]ProfileDescriptor, len(copy.Profiles))
	for _, profile := range copy.Profiles {
		profiles[profile.ID()] = profile
	}
	for _, record := range copy.Records {
		if !validIdentity(record.RecordID) || !validIdentity(record.ProfileID) || !record.ProfileVersion.valid() {
			return nil, fixtureMutationError{FixtureMutationReasonMalformed}
		}
		if _, ok := seen[strings.ToLower(record.RecordID)]; ok {
			return nil, fixtureMutationError{FixtureMutationReasonDuplicate}
		}
		seen[strings.ToLower(record.RecordID)] = struct{}{}
		profile, ok := profiles[record.ProfileID]
		if !ok || profile.Version() != record.ProfileVersion {
			return nil, fixtureMutationError{FixtureMutationReasonMalformed}
		}
		if err := validateFixtureSanitization(record); err != nil {
			return nil, err
		}
		if err := validateFixtureRecordDeclaration(catalog, profile, record); err != nil {
			return nil, err
		}
	}
	sort.Slice(copy.Profiles, func(i, j int) bool { return copy.Profiles[i].ID() < copy.Profiles[j].ID() })
	sort.Slice(copy.Records, func(i, j int) bool { return copy.Records[i].RecordID < copy.Records[j].RecordID })
	return &FixtureConformanceCorpus{spec: copy, catalog: catalog, limits: limits}, nil
}

func validateFixtureRecordDeclaration(catalog Catalog, profile ProfileDescriptor, record FixtureConformanceRecordSpec) error {
	if record.Qualification.Expected != FixtureQualificationDispositionQualified || profile.Spec().Maturity != MaturityQualified {
		return fixtureMutationError{FixtureMutationReasonContradictory}
	}
	detector, err := newFixtureDetector(catalog, record.Detection.Declaration)
	if err != nil {
		return fixtureMutationError{FixtureMutationReasonMalformed}
	}
	if err := validateFixtureProbeSet(record.Detection.Input, record.Detection.Declaration.Plan); err != nil {
		return fixtureMutationError{FixtureMutationReasonMalformed}
	}
	reader, err := fixtureReader(record.Detection.Input)
	if err != nil {
		return fixtureMutationError{FixtureMutationReasonMalformed}
	}
	decision, err := detector.Detect(context.Background(), reader, DetectionOptions{})
	if err != nil || !(FixtureDetectionResult{Actual: decision, expected: record.Detection.Expected}).MatchesExpected() {
		return fixtureMutationError{FixtureMutationReasonContradictory}
	}
	if record.ExpectedReplay.Outcome != FixtureReplayAccepted && record.ExpectedReplay.Outcome != FixtureReplayRejected {
		return fixtureMutationError{FixtureMutationReasonMalformed}
	}
	replay, err := actualFixtureReplay(profile, record.Observation)
	if err != nil {
		return fixtureMutationError{FixtureMutationReasonMalformed}
	}
	if !(FixtureReplayResult{actual: replay, expected: record.ExpectedReplay}).MatchesExpected() {
		return fixtureMutationError{FixtureMutationReasonContradictory}
	}
	return nil
}

func newFixtureDetector(catalog Catalog, declaration FixtureDetectorDeclarationSpec) (*ProfileDetector, error) {
	plan, err := NewProbePlan(declaration.Plan, declaration.Limits)
	if err != nil {
		return nil, err
	}
	candidates := make([]DetectionCandidate, len(declaration.Candidates))
	for index, spec := range declaration.Candidates {
		candidate, candidateErr := NewDetectionCandidate(spec, declaration.Limits)
		if candidateErr != nil {
			return nil, candidateErr
		}
		candidates[index] = candidate
	}
	return NewProfileDetector(ProfileDetectorSpec{DetectorVersion: declaration.DetectorVersion, Plan: plan, Catalog: catalog, Candidates: candidates, Limits: declaration.Limits})
}

func (corpus *FixtureConformanceCorpus) Spec() FixtureConformanceCorpusSpec {
	if corpus == nil {
		return FixtureConformanceCorpusSpec{}
	}
	return cloneFixtureCorpusSpec(corpus.spec)
}

type fixtureProbeReader struct{ results map[string]ProbeReadResult }

func (reader fixtureProbeReader) ReadProbe(_ context.Context, request ProbeReadRequest) (ProbeReadResult, error) {
	result, ok := reader.results[request.DeclarationID()]
	if !ok {
		return ProbeReadResult{}, fmt.Errorf("fixture probe absent")
	}
	return result, nil
}

func fixtureReader(input FixtureDetectorInput) (fixtureProbeReader, error) {
	reader := fixtureProbeReader{results: make(map[string]ProbeReadResult, len(input.Probes))}
	for _, probe := range input.Probes {
		if _, exists := reader.results[probe.DeclarationID]; exists {
			return fixtureProbeReader{}, fmt.Errorf("duplicate fixture probe")
		}
		result, err := NewProbeReadResult(probe.Result)
		if err != nil {
			return fixtureProbeReader{}, err
		}
		reader.results[probe.DeclarationID] = result
	}
	return reader, nil
}

func validateFixtureProbeSet(input FixtureDetectorInput, plan ProbePlanSpec) error {
	if len(input.Probes) != len(plan.Declarations) {
		return fmt.Errorf("fixture probe set is incomplete")
	}
	expected := make(map[string]struct{}, len(plan.Declarations))
	for _, declaration := range plan.Declarations {
		expected[declaration.ID] = struct{}{}
	}
	for _, probe := range input.Probes {
		if _, ok := expected[probe.DeclarationID]; !ok {
			return fmt.Errorf("fixture probe is undeclared")
		}
		delete(expected, probe.DeclarationID)
	}
	if len(expected) != 0 {
		return fmt.Errorf("fixture probe set is incomplete")
	}
	return nil
}

func classifyFixtureObservation(profile ProfileDescriptor, spec ObservationSpec) FixtureReplayReason {
	declarations := profile.Dependencies().Dependencies()
	if len(spec.Dependencies) != len(declarations) {
		return FixtureReplayReasonObservationRejected
	}
	for i, dependency := range spec.Dependencies {
		view := dependency.View.Record()
		if dependency.Status == DependencyReadTorn {
			return FixtureReplayReasonTornRead
		}
		if dependency.NormalizationVersion != declarations[i].Normalization().Spec().Version {
			return FixtureReplayReasonNormalizationMismatch
		}
		if view.UnitID != spec.UnitID {
			return FixtureReplayReasonUnitMismatch
		}
		if view.PollGeneration != spec.PollGenerationID {
			return FixtureReplayReasonGenerationMismatch
		}
		if view.DeadlineIdentity != spec.Dependencies[0].View.Record().DeadlineIdentity {
			return FixtureReplayReasonDeadlineMismatch
		}
		if view.Table != declarations[i].Table() || (view.Table == HoldingRegisters && view.RequestedFunction != FunctionReadHoldingRegisters) || (view.Table == InputRegisters && view.RequestedFunction != FunctionReadInputRegisters) || view.RequestedFunction != view.ReceivedFunction {
			return FixtureReplayReasonTableAccessMismatch
		}
		if profile.Spec().Coherence.Mode == CoherenceSingleWireResponse {
			if dependency.SourceTime.State != SourceTimeUnavailableState || !dependency.SourceTime.Time.IsZero() {
				return FixtureReplayReasonSourceMismatch
			}
		} else if !sourceTimesEqual(dependency.SourceTime, spec.SourceTime) {
			return FixtureReplayReasonSourceMismatch
		}
		if dependency.DocumentaryConsistencyMarker != spec.Dependencies[0].DocumentaryConsistencyMarker {
			return FixtureReplayReasonCoherenceMismatch
		}
	}
	encoded, err := MarshalFixtureSpec(spec)
	if err != nil {
		return FixtureReplayReasonObservationRejected
	}
	replayer, err := NewFixtureReplayer(profile)
	if err != nil {
		return FixtureReplayReasonObservationRejected
	}
	if _, err := replayer.Replay(encoded); err != nil {
		return FixtureReplayReasonObservationRejected
	}
	return FixtureReplayReasonAccepted
}

func actualFixtureReplay(
	profile ProfileDescriptor,
	spec ObservationSpec,
) (FixtureReplayActual, error) {
	reason := classifyFixtureObservation(profile, spec)
	actual := FixtureReplayActual{
		outcome:  FixtureReplayRejected,
		reason:   reason,
		rawWords: make([][]uint16, len(spec.Dependencies)),
	}
	for index, dependency := range spec.Dependencies {
		actual.rawWords[index] = dependency.View.Record().Words
	}
	if reason != FixtureReplayReasonAccepted {
		return actual, nil
	}
	encoded, err := MarshalFixtureSpec(spec)
	if err != nil {
		return FixtureReplayActual{}, err
	}
	replayer, err := NewFixtureReplayer(profile)
	if err != nil {
		return FixtureReplayActual{}, err
	}
	replay, err := replayer.Replay(encoded)
	if err != nil {
		return FixtureReplayActual{}, err
	}
	dependencies := replay.Replay()
	if len(dependencies) != len(actual.rawWords) {
		return FixtureReplayActual{}, fmt.Errorf("fixture replay dependency count changed")
	}
	actual.outcome = FixtureReplayAccepted
	for index, dependency := range dependencies {
		actual.rawWords[index] = dependency.RawWords()
	}
	return actual, nil
}

type FixtureReplayActual struct {
	outcome  FixtureReplayOutcome
	reason   FixtureReplayReason
	rawWords [][]uint16
}

func (actual FixtureReplayActual) Outcome() FixtureReplayOutcome { return actual.outcome }
func (actual FixtureReplayActual) Reason() FixtureReplayReason   { return actual.reason }

type FixtureReplayResult struct {
	actual   FixtureReplayActual
	expected FixtureReplayExpectation
}

func (result FixtureReplayResult) Actual() FixtureReplayActual {
	actual := result.actual
	actual.rawWords = cloneWordSets(actual.rawWords)
	return actual
}
func (result FixtureReplayResult) Expected() FixtureReplayExpectation {
	expected := result.expected
	expected.ExpectedRawWords = cloneWordSets(expected.ExpectedRawWords)
	return expected
}
func (result FixtureReplayResult) MatchesExpected() bool {
	return result.actual.outcome == result.expected.Outcome && result.actual.reason == result.expected.Reason && (result.actual.outcome == FixtureReplayRejected || reflectWordSetsEqual(result.actual.rawWords, result.expected.ExpectedRawWords))
}
func reflectWordSetsEqual(first, second [][]uint16) bool {
	if len(first) != len(second) {
		return false
	}
	for i := range first {
		if !slicesEqual(first[i], second[i]) {
			return false
		}
	}
	return true
}
func slicesEqual(first, second []uint16) bool {
	if len(first) != len(second) {
		return false
	}
	for i := range first {
		if first[i] != second[i] {
			return false
		}
	}
	return true
}

type FixtureDetectionResult struct {
	Actual   DetectionDecision
	expected FixtureDetectionExpectation
}

func (result FixtureDetectionResult) MatchesExpected() bool {
	if result.Actual.Outcome() != result.expected.Outcome || result.Actual.Reason() != result.expected.Reason || result.Actual.SelectedProfileID() != result.expected.SelectedProfileID || result.Actual.SelectedProfileVersion() != result.expected.SelectedProfileVersion {
		return false
	}
	evidence := result.Actual.Evidence()
	if len(evidence) != len(result.expected.Evidence) {
		return false
	}
	for i := range evidence {
		want := result.expected.Evidence[i]
		if evidence[i].ProfileID != want.ProfileID || evidence[i].ProfileVersion != want.ProfileVersion || evidence[i].Score != want.Score || evidence[i].Reason != want.Reason || evidence[i].DetectorVersion != want.DetectorVersion || evidence[i].QualificationVersion != want.QualificationVersion || !probeFieldsEqual(evidence[i].MatchedGates, want.MatchedGates) || !stringSlicesEqual(evidence[i].ProbeEvidenceIDs, want.ProbeEvidenceIDs) {
			return false
		}
	}
	return true
}
func probeFieldsEqual(first, second []ProbeIdentityField) bool {
	if len(first) != len(second) {
		return false
	}
	for i := range first {
		if first[i] != second[i] {
			return false
		}
	}
	return true
}
func stringSlicesEqual(first, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for i := range first {
		if first[i] != second[i] {
			return false
		}
	}
	return true
}

type FixtureWireProvenance struct{ WireResponseID uint64 }
type FixtureLogicalProvenance struct{ LogicalViewID uint64 }
type FixtureSampleProvenance struct{ PollGenerationID uint64 }
type FixtureLogicalSlice struct{ words []uint16 }

func (slice FixtureLogicalSlice) CanonicalBytes() []byte {
	result := make([]byte, len(slice.words)*2)
	for i, word := range slice.words {
		binary.BigEndian.PutUint16(result[i*2:], word)
	}
	return result
}

type FixtureConformanceRecord struct {
	recordID, profileID string
	profileVersion      Version
	function            FunctionCode
	table               LogicalTable
	unit                byte
	normalized          uint16
	rawWords            []uint16
	source              SourceTimeSpec
	wire                FixtureWireProvenance
	logical             FixtureLogicalProvenance
	sample              FixtureSampleProvenance
	slices              []FixtureLogicalSlice
	detection           FixtureDetectionResult
	qualification       FixtureQualificationDisposition
	replay              FixtureReplayResult
}

func (record FixtureConformanceRecord) RecordID() string           { return record.recordID }
func (record FixtureConformanceRecord) ProfileID() string          { return record.profileID }
func (record FixtureConformanceRecord) ProfileVersion() Version    { return record.profileVersion }
func (record FixtureConformanceRecord) FunctionCode() FunctionCode { return record.function }
func (record FixtureConformanceRecord) Table() LogicalTable        { return record.table }
func (record FixtureConformanceRecord) UnitID() byte               { return record.unit }
func (record FixtureConformanceRecord) NormalizedAddress() uint16  { return record.normalized }
func (record FixtureConformanceRecord) RawWords() []uint16 {
	return append([]uint16(nil), record.rawWords...)
}
func (record FixtureConformanceRecord) SourceTime() SourceTimeSpec            { return record.source }
func (record FixtureConformanceRecord) WireProvenance() FixtureWireProvenance { return record.wire }
func (record FixtureConformanceRecord) LogicalProvenance() FixtureLogicalProvenance {
	return record.logical
}
func (record FixtureConformanceRecord) SampleProvenance() FixtureSampleProvenance {
	return record.sample
}
func (record FixtureConformanceRecord) LogicalSlices() []FixtureLogicalSlice {
	result := make([]FixtureLogicalSlice, len(record.slices))
	for i := range record.slices {
		result[i].words = append([]uint16(nil), record.slices[i].words...)
	}
	return result
}
func (record FixtureConformanceRecord) Detection() FixtureDetectionResult { return record.detection }
func (record FixtureConformanceRecord) Qualification() FixtureQualificationDisposition {
	return record.qualification
}
func (record FixtureConformanceRecord) Replay() FixtureReplayResult { return record.replay }

type FixtureConformanceReport struct {
	metadata SanitizedFixtureMetadata
	records  []FixtureConformanceRecord
}

func (report FixtureConformanceReport) Metadata() SanitizedFixtureMetadata { return report.metadata }
func (report FixtureConformanceReport) Records() []FixtureConformanceRecord {
	return append([]FixtureConformanceRecord(nil), report.records...)
}
func (report FixtureConformanceReport) RecordCount() int { return len(report.records) }
func (report FixtureConformanceReport) RejectedCount() int {
	count := 0
	for _, record := range report.records {
		if record.replay.actual.outcome == FixtureReplayRejected {
			count++
		}
	}
	return count
}

func (corpus *FixtureConformanceCorpus) Replay() (FixtureConformanceReport, error) {
	if corpus == nil {
		return FixtureConformanceReport{}, fixtureMutationError{FixtureMutationReasonMalformed}
	}
	records := make([]FixtureConformanceRecord, len(corpus.spec.Records))
	profiles := make(map[string]ProfileDescriptor, len(corpus.spec.Profiles))
	for _, profile := range corpus.spec.Profiles {
		profiles[profile.ID()] = profile
	}
	for i, spec := range corpus.spec.Records {
		record, err := replayFixtureRecord(corpus.catalog, profiles[spec.ProfileID], spec)
		if err != nil {
			return FixtureConformanceReport{}, err
		}
		records[i] = record
	}
	sort.Slice(records, func(i, j int) bool { return records[i].recordID < records[j].recordID })
	return FixtureConformanceReport{metadata: corpus.spec.Metadata, records: records}, nil
}

func replayFixtureRecord(catalog Catalog, profile ProfileDescriptor, spec FixtureConformanceRecordSpec) (FixtureConformanceRecord, error) {
	actual, err := actualFixtureReplay(profile, spec.Observation)
	if err != nil {
		return FixtureConformanceRecord{}, err
	}
	detector, err := newFixtureDetector(catalog, spec.Detection.Declaration)
	if err != nil {
		return FixtureConformanceRecord{}, err
	}
	reader, err := fixtureReader(spec.Detection.Input)
	if err != nil {
		return FixtureConformanceRecord{}, err
	}
	decision, err := detector.Detect(context.Background(), reader, DetectionOptions{})
	if err != nil {
		return FixtureConformanceRecord{}, err
	}
	view := spec.Observation.Dependencies[0].View.Record()
	result := FixtureConformanceRecord{recordID: spec.RecordID, profileID: spec.ProfileID, profileVersion: spec.ProfileVersion, function: view.RequestedFunction, table: view.Table, unit: spec.Observation.UnitID, normalized: view.LogicalOffset, rawWords: append([]uint16(nil), view.Words...), source: spec.Observation.SourceTime, wire: FixtureWireProvenance{view.WireResponseID}, logical: FixtureLogicalProvenance{view.LogicalViewID}, sample: FixtureSampleProvenance{PollGenerationID: spec.Observation.PollGenerationID}, detection: FixtureDetectionResult{Actual: decision, expected: spec.Detection.Expected}, qualification: spec.Qualification.Expected, replay: FixtureReplayResult{actual: actual, expected: spec.ExpectedReplay}}
	result.slices = make([]FixtureLogicalSlice, len(actual.rawWords))
	for index, words := range actual.rawWords {
		result.slices[index] = FixtureLogicalSlice{words: append([]uint16(nil), words...)}
	}
	if len(actual.rawWords) != 0 {
		result.rawWords = append([]uint16(nil), actual.rawWords[0]...)
	}
	return result, nil
}

// ErrFixtureConformanceNondeterministic is used by callers detecting a broken report invariant.
var ErrFixtureConformanceNondeterministic = errors.New("fixture conformance report is nondeterministic")

type fixtureCorpusDTO struct {
	SchemaVersion Version           `json:"schema_version"`
	Metadata      json.RawMessage   `json:"metadata"`
	Profiles      []json.RawMessage `json:"profiles"`
	Records       []json.RawMessage `json:"records"`
}
type fixtureRecordDTO struct {
	RecordID       string          `json:"record_id"`
	ProfileID      string          `json:"profile_id"`
	ProfileVersion Version         `json:"profile_version"`
	Observation    json.RawMessage `json:"observation"`
	Detection      json.RawMessage `json:"detection"`
	Qualification  json.RawMessage `json:"qualification"`
	ExpectedReplay json.RawMessage `json:"expected_replay"`
}

func MarshalFixtureConformanceCorpusSpec(spec FixtureConformanceCorpusSpec) ([]byte, error) {
	if _, err := NewFixtureConformanceCorpus(spec); err != nil {
		return nil, err
	}
	metadata, err := json.Marshal(spec.Metadata)
	if err != nil {
		return nil, err
	}
	dto := fixtureCorpusDTO{SchemaVersion: spec.SchemaVersion, Metadata: metadata, Profiles: make([]json.RawMessage, len(spec.Profiles)), Records: make([]json.RawMessage, len(spec.Records))}
	for i, profile := range spec.Profiles {
		encoded, err := MarshalProfileDescriptor(profile)
		if err != nil {
			return nil, err
		}
		dto.Profiles[i] = encoded
	}
	for i, record := range spec.Records {
		encoded, err := MarshalFixtureSpec(record.Observation)
		if err != nil {
			return nil, err
		}
		detection, detectionErr := json.Marshal(record.Detection)
		if detectionErr != nil {
			return nil, detectionErr
		}
		qualification, qualificationErr := json.Marshal(record.Qualification)
		if qualificationErr != nil {
			return nil, qualificationErr
		}
		expected, expectedErr := json.Marshal(record.ExpectedReplay)
		if expectedErr != nil {
			return nil, expectedErr
		}
		recordBytes, recordErr := json.Marshal(fixtureRecordDTO{RecordID: record.RecordID, ProfileID: record.ProfileID, ProfileVersion: record.ProfileVersion, Observation: encoded, Detection: detection, Qualification: qualification, ExpectedReplay: expected})
		if recordErr != nil {
			return nil, recordErr
		}
		dto.Records[i] = recordBytes
	}
	return marshalBounded(dto)
}

func preflightFixtureCorpusJSON(data []byte) error {
	if len(data) == 0 {
		return fixtureMutationError{FixtureMutationReasonMalformed}
	}
	if len(data) > MaxSerializedContractBytes {
		return fixtureMutationError{FixtureMutationReasonOversized}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return fixtureMutationError{FixtureMutationReasonMalformed}
	}
	expected := map[string]struct{}{"schema_version": {}, "metadata": {}, "profiles": {}, "records": {}}
	seen := map[string]struct{}{}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return fixtureMutationError{FixtureMutationReasonMalformed}
		}
		folded := strings.ToLower(key)
		if _, ok := seen[folded]; ok {
			if key == strings.ToLower(key) {
				return fixtureMutationError{FixtureMutationReasonDuplicate}
			}
			return fixtureMutationError{FixtureMutationReasonCaseFolded}
		}
		seen[folded] = struct{}{}
		if _, ok := expected[key]; !ok {
			if _, foldedKnown := expected[folded]; foldedKnown {
				return fixtureMutationError{FixtureMutationReasonCaseFolded}
			}
			return fixtureMutationError{FixtureMutationReasonUnknown}
		}
		var discard json.RawMessage
		if err := decoder.Decode(&discard); err != nil {
			return fixtureMutationError{FixtureMutationReasonMalformed}
		}
	}
	if _, err := decoder.Token(); err != nil {
		return fixtureMutationError{FixtureMutationReasonMalformed}
	}
	for key := range expected {
		if _, ok := seen[key]; !ok {
			return fixtureMutationError{FixtureMutationReasonMissing}
		}
	}
	if decoder.More() {
		return fixtureMutationError{FixtureMutationReasonMalformed}
	}
	return nil
}

func UnmarshalFixtureConformanceCorpus(data []byte) (*FixtureConformanceCorpus, error) {
	if err := preflightFixtureCorpusJSON(data); err != nil {
		return nil, err
	}
	var dto fixtureCorpusDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, fixtureMutationError{FixtureMutationReasonMalformed}
	}
	if err := preflightAggregate(dto); err != nil {
		return nil, fixtureMutationError{FixtureMutationReasonOversized}
	}
	var metadata SanitizedFixtureMetadata
	if err := json.Unmarshal(dto.Metadata, &metadata); err != nil {
		return nil, fixtureMutationError{FixtureMutationReasonMalformed}
	}
	if len(metadata.CorpusID) > MaxContractStringBytes || len(metadata.LicenseExpression) > MaxContractStringBytes || len(metadata.Provenance) > MaxContractStringBytes {
		return nil, fixtureMutationError{FixtureMutationReasonOversized}
	}
	if err := decodeStrict(dto.Metadata, &metadata); err != nil {
		return nil, fixtureMutationError{FixtureMutationReasonMalformed}
	}
	spec := FixtureConformanceCorpusSpec{SchemaVersion: dto.SchemaVersion, Metadata: metadata, Profiles: make([]ProfileDescriptor, len(dto.Profiles)), Records: make([]FixtureConformanceRecordSpec, len(dto.Records))}
	for i, encoded := range dto.Profiles {
		profile, err := UnmarshalProfileDescriptor(encoded)
		if err != nil {
			return nil, fixtureMutationError{FixtureMutationReasonMalformed}
		}
		spec.Profiles[i] = profile
	}
	for i, encodedRecord := range dto.Records {
		if err := preflightFixtureRecordJSON(encodedRecord); err != nil {
			return nil, err
		}
		var record fixtureRecordDTO
		if err := json.Unmarshal(encodedRecord, &record); err != nil {
			return nil, fixtureMutationError{FixtureMutationReasonMalformed}
		}
		var observation ObservationSpec
		if err := json.Unmarshal(record.Observation, &observation); err != nil {
			return nil, fixtureMutationError{FixtureMutationReasonMalformed}
		}
		var detection FixtureDetectionCaseSpec
		if err := decodeStrict(record.Detection, &detection); err != nil {
			return nil, fixtureMutationError{FixtureMutationReasonMalformed}
		}
		var qualification FixtureQualificationExpectation
		if err := decodeStrict(record.Qualification, &qualification); err != nil {
			return nil, fixtureMutationError{FixtureMutationReasonMalformed}
		}
		var expected FixtureReplayExpectation
		if err := decodeStrict(record.ExpectedReplay, &expected); err != nil {
			return nil, fixtureMutationError{FixtureMutationReasonMalformed}
		}
		spec.Records[i] = FixtureConformanceRecordSpec{RecordID: record.RecordID, ProfileID: record.ProfileID, ProfileVersion: record.ProfileVersion, Observation: observation, Detection: detection, Qualification: qualification, ExpectedReplay: expected}
	}
	corpus, err := NewFixtureConformanceCorpus(spec)
	if err != nil {
		return nil, err
	}
	return corpus, nil
}

func preflightFixtureRecordJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return fixtureMutationError{FixtureMutationReasonMalformed}
	}
	expected := map[string]struct{}{"record_id": {}, "profile_id": {}, "profile_version": {}, "observation": {}, "detection": {}, "qualification": {}, "expected_replay": {}}
	seen := map[string]struct{}{}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return fixtureMutationError{FixtureMutationReasonMalformed}
		}
		folded := strings.ToLower(key)
		if _, duplicate := seen[folded]; duplicate {
			return fixtureMutationError{FixtureMutationReasonMalformed}
		}
		seen[folded] = struct{}{}
		if _, ok := expected[key]; !ok {
			return fixtureMutationError{FixtureMutationReasonMalformed}
		}
		var discard json.RawMessage
		if err := decoder.Decode(&discard); err != nil {
			return fixtureMutationError{FixtureMutationReasonMalformed}
		}
	}
	if _, err := decoder.Token(); err != nil {
		return fixtureMutationError{FixtureMutationReasonMalformed}
	}
	for key := range expected {
		if _, ok := seen[key]; !ok {
			return fixtureMutationError{FixtureMutationReasonMalformed}
		}
	}
	return nil
}

type fixtureReportDTO struct {
	Metadata SanitizedFixtureMetadata `json:"metadata"`
	Records  []fixtureReportRecordDTO `json:"records"`
}
type fixtureReportRecordDTO struct {
	RecordID       string                          `json:"record_id"`
	ProfileID      string                          `json:"profile_id"`
	ProfileVersion Version                         `json:"profile_version"`
	Outcome        FixtureReplayOutcome            `json:"outcome"`
	Reason         FixtureReplayReason             `json:"reason"`
	Function       FunctionCode                    `json:"function"`
	Table          LogicalTable                    `json:"table"`
	UnitID         byte                            `json:"unit_id"`
	Normalized     uint16                          `json:"normalized_address"`
	RawWords       [][]uint16                      `json:"raw_words,omitempty"`
	LogicalSlices  [][]uint16                      `json:"logical_slices,omitempty"`
	Wire           FixtureWireProvenance           `json:"wire"`
	Logical        FixtureLogicalProvenance        `json:"logical"`
	Sample         FixtureSampleProvenance         `json:"sample"`
	Source         SourceTimeSpec                  `json:"source_time"`
	Qualification  FixtureQualificationDisposition `json:"qualification"`
	Detection      detectionDecisionDTO            `json:"detection"`
}

func (corpus *FixtureConformanceCorpus) MarshalBoundedReport() ([]byte, error) {
	report, err := corpus.Replay()
	if err != nil {
		return nil, err
	}
	dto := fixtureReportDTO{Metadata: report.metadata, Records: make([]fixtureReportRecordDTO, len(report.records))}
	for i, record := range report.records {
		slices := make([][]uint16, len(record.slices))
		for index, slice := range record.slices {
			slices[index] = append([]uint16(nil), slice.words...)
		}
		decision := detectionDecisionToDTO(record.detection.Actual)
		dto.Records[i] = fixtureReportRecordDTO{RecordID: record.recordID, ProfileID: record.profileID, ProfileVersion: record.profileVersion, Outcome: record.replay.actual.outcome, Reason: record.replay.actual.reason, Function: record.function, Table: record.table, UnitID: record.unit, Normalized: record.normalized, RawWords: cloneWordSets(record.replay.actual.rawWords), LogicalSlices: slices, Wire: record.wire, Logical: record.logical, Sample: record.sample, Source: record.source, Qualification: record.qualification, Detection: decision}
	}
	encoded, err := marshalBounded(dto)
	if err != nil {
		return nil, err
	}
	if len(encoded) > corpus.limits.MaxReportBytes {
		return nil, fixtureMutationError{FixtureMutationReasonOversized}
	}
	return encoded, nil
}
