package detection

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strings"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// FixtureMutationReason is the closed construction/decoding rejection set.
type FixtureMutationReason string

// The fixture mutation reasons. Construction, decoding and bounded report
// encoding reject with exactly one of them; IsFixtureMutationReason tests it.
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
	return FixtureConformanceLimits{MaxRecords: reg.MaxProfileDependencies, MaxReportBytes: reg.MaxSerializedContractBytes}
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
	DetectorVersion reg.Version     `json:"detector_version"`
	Plan            ProbePlanSpec   `json:"plan"`
	Candidates      []CandidateSpec `json:"candidates"`
	Limits          Limits          `json:"limits"`
}

// FixtureProbeInput is one recorded probe read, bound to a probe declaration of
// the detector plan by DeclarationID.
type FixtureProbeInput struct {
	DeclarationID string              `json:"declaration_id"`
	Result        ProbeReadResultSpec `json:"result"`
}

// FixtureDetectorInput is the recorded probe set the fixture detector reads
// instead of a device. It must answer every plan declaration exactly once.
type FixtureDetectorInput struct {
	Probes []FixtureProbeInput `json:"probes"`
}

// FixtureDetectionEvidenceExpectation is the expected evidence row of one
// candidate in a recorded detection decision.
type FixtureDetectionEvidenceExpectation struct {
	ProfileID            string               `json:"profile_id"`
	ProfileVersion       reg.Version          `json:"profile_version"`
	Score                uint32               `json:"score"`
	Reason               Reason               `json:"reason"`
	MatchedGates         []ProbeIdentityField `json:"matched_gates"`
	ProbeEvidenceIDs     []string             `json:"probe_evidence_ids"`
	DetectorVersion      reg.Version          `json:"detector_version"`
	QualificationVersion reg.Version          `json:"qualification_version"`
}

// FixtureDetectionExpectation is the complete expected detection decision,
// including the ordered evidence rows.
type FixtureDetectionExpectation struct {
	Outcome                Outcome                               `json:"outcome"`
	Reason                 Reason                                `json:"reason"`
	SelectedProfileID      string                                `json:"selected_profile_id"`
	SelectedProfileVersion reg.Version                           `json:"selected_profile_version"`
	Evidence               []FixtureDetectionEvidenceExpectation `json:"evidence"`
}

// FixtureDetectionCaseSpec pairs a detector declaration and its recorded probe
// input with the decision the replay must reproduce.
type FixtureDetectionCaseSpec struct {
	Declaration FixtureDetectorDeclarationSpec `json:"declaration"`
	Input       FixtureDetectorInput           `json:"input"`
	Expected    FixtureDetectionExpectation    `json:"expected"`
}

// FixtureQualificationDisposition is the recorded qualification expectation of
// one fixture record. It is carried verbatim and does not qualify a profile.
type FixtureQualificationDisposition string

// The fixture qualification dispositions.
const (
	FixtureQualificationDispositionQualified FixtureQualificationDisposition = "qualified"
	FixtureQualificationDispositionRejected  FixtureQualificationDisposition = "rejected"
)

// FixtureQualificationExpectation wraps the expected qualification disposition.
type FixtureQualificationExpectation struct {
	Expected FixtureQualificationDisposition `json:"expected"`
}

// FixtureReplayOutcome is the replay verdict for one fixture observation.
type FixtureReplayOutcome string

// The fixture replay outcomes.
const (
	FixtureReplayAccepted FixtureReplayOutcome = "accepted"
	FixtureReplayRejected FixtureReplayOutcome = "rejected"
)

// FixtureReplayReason is the closed set of replay verdict reasons. An accepted
// replay always carries FixtureReplayReasonAccepted; every other reason names
// the first observation fact that rejected the replay.
type FixtureReplayReason string

// The fixture replay reasons.
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

// FixtureReplayExpectation is the recorded replay verdict. ExpectedRawWords is
// compared only for an accepted outcome, one word set per dependency.
type FixtureReplayExpectation struct {
	Outcome          FixtureReplayOutcome `json:"outcome"`
	Reason           FixtureReplayReason  `json:"reason"`
	ExpectedRawWords [][]uint16           `json:"expected_raw_words,omitempty"`
}

// FixtureConformanceRecordSpec is one fixture record: an observation for a
// profile of the corpus, its detection case, and the recorded qualification
// and replay expectations.
type FixtureConformanceRecordSpec struct {
	RecordID       string                          `json:"record_id"`
	ProfileID      string                          `json:"profile_id"`
	ProfileVersion reg.Version                     `json:"profile_version"`
	Observation    reg.ObservationSpec             `json:"observation"`
	Detection      FixtureDetectionCaseSpec        `json:"detection"`
	Qualification  FixtureQualificationExpectation `json:"qualification"`
	ExpectedReplay FixtureReplayExpectation        `json:"expected_replay"`
}

// FixtureConformanceCorpusSpec is the complete serializable corpus declaration.
// It needs at least one profile and one record, and its metadata must be
// sanitized public corpus identity.
type FixtureConformanceCorpusSpec struct {
	SchemaVersion reg.Version                    `json:"schema_version"`
	Metadata      SanitizedFixtureMetadata       `json:"metadata"`
	Profiles      []reg.ProfileDescriptor        `json:"profiles"`
	Records       []FixtureConformanceRecordSpec `json:"records"`
}

// FixtureConformanceCorpus is an admitted, immutable fixture corpus with its
// profile catalog. Records are held sorted by record identity; replay reads no
// device.
type FixtureConformanceCorpus struct {
	spec    FixtureConformanceCorpusSpec
	catalog reg.Catalog
	limits  FixtureConformanceLimits
}

func cloneFixtureCorpusSpec(spec FixtureConformanceCorpusSpec) FixtureConformanceCorpusSpec {
	spec.Profiles = append([]reg.ProfileDescriptor(nil), spec.Profiles...)
	for i := range spec.Profiles {
		spec.Profiles[i], _ = reg.NewProfileDescriptor(spec.Profiles[i].Spec())
	}
	spec.Records = append([]FixtureConformanceRecordSpec(nil), spec.Records...)
	for i := range spec.Records {
		spec.Records[i] = cloneFixtureRecordSpec(spec.Records[i])
	}
	return spec
}

func cloneFixtureRecordSpec(record FixtureConformanceRecordSpec) FixtureConformanceRecordSpec {
	record.Observation = reg.CloneObservationSpec(record.Observation)
	record.Detection.Declaration.Plan = cloneProbePlanSpec(record.Detection.Declaration.Plan)
	record.Detection.Declaration.Candidates = append([]CandidateSpec(nil), record.Detection.Declaration.Candidates...)
	record.Detection.Input.Probes = append([]FixtureProbeInput(nil), record.Detection.Input.Probes...)
	for i := range record.Detection.Input.Probes {
		record.Detection.Input.Probes[i].Result.Words = append([]uint16(nil), record.Detection.Input.Probes[i].Result.Words...)
	}
	record.Detection.Expected.Evidence = append([]FixtureDetectionEvidenceExpectation(nil), record.Detection.Expected.Evidence...)
	for i := range record.Detection.Expected.Evidence {
		record.Detection.Expected.Evidence[i].MatchedGates = append([]ProbeIdentityField(nil), record.Detection.Expected.Evidence[i].MatchedGates...)
		record.Detection.Expected.Evidence[i].ProbeEvidenceIDs = slices.Clone(record.Detection.Expected.Evidence[i].ProbeEvidenceIDs)
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

// NewFixtureConformanceCorpus constructs immutable offline evidence only.
func NewFixtureConformanceCorpus(spec FixtureConformanceCorpusSpec) (*FixtureConformanceCorpus, error) {
	return NewFixtureConformanceCorpusWithLimits(spec, defaultFixtureConformanceLimits())
}

// NewFixtureConformanceCorpusWithLimits admits spec under limits, which may only
// tighten the default bounds. It fails closed with a fixture mutation reason
// for an oversized, malformed, duplicate or unsanitized corpus, and keeps an
// independent copy of spec.
func NewFixtureConformanceCorpusWithLimits(spec FixtureConformanceCorpusSpec, limits FixtureConformanceLimits) (*FixtureConformanceCorpus, error) {
	if err := validateFixtureConformanceLimits(limits); err != nil {
		return nil, err
	}
	if err := reg.PreflightAggregate(spec); err != nil {
		return nil, fixtureMutationError{FixtureMutationReasonOversized}
	}
	if len(spec.Profiles) > limits.MaxRecords || len(spec.Records) > limits.MaxRecords {
		return nil, fixtureMutationError{FixtureMutationReasonOversized}
	}
	if spec.SchemaVersion != reg.CurrentSchemaVersion() || len(spec.Profiles) == 0 || len(spec.Records) == 0 {
		return nil, fixtureMutationError{FixtureMutationReasonMalformed}
	}
	if err := validateSanitizedMetadata(spec.Metadata); err != nil {
		return nil, err
	}
	catalog, err := reg.NewCatalog(spec.Profiles...)
	if err != nil {
		return nil, fixtureMutationError{FixtureMutationReasonMalformed}
	}
	copy := cloneFixtureCorpusSpec(spec)
	seen := make(map[string]struct{}, len(copy.Records))
	profiles := make(map[string]reg.ProfileDescriptor, len(copy.Profiles))
	for _, profile := range copy.Profiles {
		profiles[profile.ID()] = profile
	}
	for _, record := range copy.Records {
		if !reg.ValidIdentity(record.RecordID) || !reg.ValidIdentity(record.ProfileID) || !record.ProfileVersion.Valid() {
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

// Spec returns an independent copy of the admitted corpus declaration, or the
// zero value for a nil corpus.
func (corpus *FixtureConformanceCorpus) Spec() FixtureConformanceCorpusSpec {
	if corpus == nil {
		return FixtureConformanceCorpusSpec{}
	}
	return cloneFixtureCorpusSpec(corpus.spec)
}

// FixtureReplayActual is the replay verdict actually reached for one record.
type FixtureReplayActual struct {
	outcome  FixtureReplayOutcome
	reason   FixtureReplayReason
	rawWords [][]uint16
}

// Outcome returns the actual replay outcome.
func (actual FixtureReplayActual) Outcome() FixtureReplayOutcome { return actual.outcome }

// Reason returns the actual replay reason.
func (actual FixtureReplayActual) Reason() FixtureReplayReason { return actual.reason }

// FixtureReplayResult pairs the actual replay verdict with the recorded
// expectation.
type FixtureReplayResult struct {
	actual   FixtureReplayActual
	expected FixtureReplayExpectation
}

// Actual returns an independent copy of the actual replay verdict.
func (result FixtureReplayResult) Actual() FixtureReplayActual {
	actual := result.actual
	actual.rawWords = cloneWordSets(actual.rawWords)
	return actual
}

// Expected returns an independent copy of the recorded replay expectation.
func (result FixtureReplayResult) Expected() FixtureReplayExpectation {
	expected := result.expected
	expected.ExpectedRawWords = cloneWordSets(expected.ExpectedRawWords)
	return expected
}

// MatchesExpected reports whether outcome and reason match and, for an
// accepted replay, every dependency's raw words match exactly.
func (result FixtureReplayResult) MatchesExpected() bool {
	return result.actual.outcome == result.expected.Outcome && result.actual.reason == result.expected.Reason && (result.actual.outcome == FixtureReplayRejected || reflectWordSetsEqual(result.actual.rawWords, result.expected.ExpectedRawWords))
}

// FixtureDetectionResult pairs the decision the fixture detector reached with
// the recorded expectation.
type FixtureDetectionResult struct {
	Actual   Decision
	expected FixtureDetectionExpectation
}

// MatchesExpected reports whether the decision matches the expectation field by
// field, including every evidence row in order.
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

// FixtureWireProvenance names the physical wire response a fact came from.
type FixtureWireProvenance struct{ WireResponseID uint64 }

// FixtureLogicalProvenance names the logical view a fact came from.
type FixtureLogicalProvenance struct{ LogicalViewID uint64 }

// FixtureSampleProvenance names the poll generation of the sample.
type FixtureSampleProvenance struct{ PollGenerationID uint64 }

// FixtureLogicalSlice is the replayed word slice of one dependency.
type FixtureLogicalSlice struct{ words []uint16 }

// CanonicalBytes returns the slice as big-endian bytes, two per word, in wire
// order.
func (slice FixtureLogicalSlice) CanonicalBytes() []byte {
	result := make([]byte, len(slice.words)*2)
	for i, word := range slice.words {
		binary.BigEndian.PutUint16(result[i*2:], word)
	}
	return result
}

// FixtureDependencyFact is immutable, per-dependency replay evidence. Its
// ordinal binds the fact to the corresponding observation dependency while the
// view, wire, and sample identities distinguish overlapping FC03/FC04 reads.
type FixtureDependencyFact struct {
	ordinal    uint32
	function   reg.FunctionCode
	table      reg.LogicalTable
	unit       byte
	normalized uint16
	rawWords   []uint16
	slice      FixtureLogicalSlice
	source     reg.SourceTimeSpec
	wire       FixtureWireProvenance
	logical    FixtureLogicalProvenance
	sample     FixtureSampleProvenance
}

// Ordinal returns the zero-based dependency position in the observation.
func (fact FixtureDependencyFact) Ordinal() uint32 { return fact.ordinal }

// FunctionCode returns the Modbus function the dependency was read with.
func (fact FixtureDependencyFact) FunctionCode() reg.FunctionCode { return fact.function }

// Table returns the logical table of the dependency.
func (fact FixtureDependencyFact) Table() reg.LogicalTable { return fact.table }

// UnitID returns the unit identifier recorded on the dependency view.
func (fact FixtureDependencyFact) UnitID() byte { return fact.unit }

// NormalizedAddress returns the zero-based logical offset of the dependency.
func (fact FixtureDependencyFact) NormalizedAddress() uint16 { return fact.normalized }

// RawWords returns an independent copy of the replayed raw words.
func (fact FixtureDependencyFact) RawWords() []uint16 { return append([]uint16(nil), fact.rawWords...) }

// LogicalSlice returns an independent copy of the replayed slice.
func (fact FixtureDependencyFact) LogicalSlice() FixtureLogicalSlice {
	return FixtureLogicalSlice{words: append([]uint16(nil), fact.slice.words...)}
}

// SourceTime returns the dependency's source-time fact.
func (fact FixtureDependencyFact) SourceTime() reg.SourceTimeSpec { return fact.source }

// WireProvenance returns the dependency's wire response identity.
func (fact FixtureDependencyFact) WireProvenance() FixtureWireProvenance { return fact.wire }

// LogicalProvenance returns the dependency's logical view identity.
func (fact FixtureDependencyFact) LogicalProvenance() FixtureLogicalProvenance {
	return fact.logical
}

// SampleProvenance returns the poll generation recorded on the dependency view.
func (fact FixtureDependencyFact) SampleProvenance() FixtureSampleProvenance { return fact.sample }

// FixtureConformanceRecord is the replayed result of one fixture record. The
// record-level function, table, address, wire, logical-view and raw-word facts
// describe the first dependency; DependencyFacts lists every dependency.
type FixtureConformanceRecord struct {
	recordID, profileID string
	profileVersion      reg.Version
	function            reg.FunctionCode
	table               reg.LogicalTable
	unit                byte
	normalized          uint16
	rawWords            []uint16
	source              reg.SourceTimeSpec
	wire                FixtureWireProvenance
	logical             FixtureLogicalProvenance
	sample              FixtureSampleProvenance
	slices              []FixtureLogicalSlice
	dependencies        []FixtureDependencyFact
	detection           FixtureDetectionResult
	qualification       FixtureQualificationDisposition
	replay              FixtureReplayResult
}

// RecordID returns the fixture record identity.
func (record FixtureConformanceRecord) RecordID() string { return record.recordID }

// ProfileID returns the identity of the profile the record replays against.
func (record FixtureConformanceRecord) ProfileID() string { return record.profileID }

// ProfileVersion returns the exact profile version of the record.
func (record FixtureConformanceRecord) ProfileVersion() reg.Version { return record.profileVersion }

// FunctionCode returns the requested function of the first dependency.
func (record FixtureConformanceRecord) FunctionCode() reg.FunctionCode { return record.function }

// Table returns the logical table of the first dependency.
func (record FixtureConformanceRecord) Table() reg.LogicalTable { return record.table }

// UnitID returns the observation's unit identifier.
func (record FixtureConformanceRecord) UnitID() byte { return record.unit }

// NormalizedAddress returns the logical offset of the first dependency.
func (record FixtureConformanceRecord) NormalizedAddress() uint16 { return record.normalized }

// RawWords returns an independent copy of the first dependency's replayed words.
func (record FixtureConformanceRecord) RawWords() []uint16 {
	return append([]uint16(nil), record.rawWords...)
}

// SourceTime returns the observation's source-time fact.
func (record FixtureConformanceRecord) SourceTime() reg.SourceTimeSpec { return record.source }

// WireProvenance returns the wire response identity of the first dependency.
func (record FixtureConformanceRecord) WireProvenance() FixtureWireProvenance { return record.wire }

// LogicalProvenance returns the logical view identity of the first dependency.
func (record FixtureConformanceRecord) LogicalProvenance() FixtureLogicalProvenance {
	return record.logical
}

// SampleProvenance returns the observation's poll generation.
func (record FixtureConformanceRecord) SampleProvenance() FixtureSampleProvenance {
	return record.sample
}

// LogicalSlices returns independent copies of every dependency's replayed slice
// in declaration order.
func (record FixtureConformanceRecord) LogicalSlices() []FixtureLogicalSlice {
	result := make([]FixtureLogicalSlice, len(record.slices))
	for i := range record.slices {
		result[i].words = append([]uint16(nil), record.slices[i].words...)
	}
	return result
}

// DependencyFacts returns independent copies of every dependency fact in
// declaration order.
func (record FixtureConformanceRecord) DependencyFacts() []FixtureDependencyFact {
	result := make([]FixtureDependencyFact, len(record.dependencies))
	for i := range record.dependencies {
		result[i] = record.dependencies[i]
		result[i].rawWords = append([]uint16(nil), record.dependencies[i].rawWords...)
		result[i].slice.words = append([]uint16(nil), record.dependencies[i].slice.words...)
	}
	return result
}

// Detection returns the detection decision and its expectation.
func (record FixtureConformanceRecord) Detection() FixtureDetectionResult { return record.detection }

// Qualification returns the recorded qualification disposition.
func (record FixtureConformanceRecord) Qualification() FixtureQualificationDisposition {
	return record.qualification
}

// Replay returns the replay verdict and its expectation.
func (record FixtureConformanceRecord) Replay() FixtureReplayResult { return record.replay }

// FixtureConformanceReport is the replay result of a whole corpus, with records
// sorted by record identity.
type FixtureConformanceReport struct {
	metadata SanitizedFixtureMetadata
	records  []FixtureConformanceRecord
}

// Metadata returns the sanitized corpus metadata.
func (report FixtureConformanceReport) Metadata() SanitizedFixtureMetadata { return report.metadata }

// Records returns the replayed records in record-identity order.
func (report FixtureConformanceReport) Records() []FixtureConformanceRecord {
	return append([]FixtureConformanceRecord(nil), report.records...)
}

// RecordCount returns the number of replayed records.
func (report FixtureConformanceReport) RecordCount() int { return len(report.records) }

// RejectedCount returns the number of records whose actual replay was rejected.
func (report FixtureConformanceReport) RejectedCount() int {
	count := 0
	for _, record := range report.records {
		if record.replay.actual.outcome == FixtureReplayRejected {
			count++
		}
	}
	return count
}

// Replay replays every record offline: the observation against its profile and
// the detection case against the recorded probe input. A rejected replay is a
// record verdict, not an error; an error means the corpus could not be
// replayed at all, and a nil corpus fails as malformed.
func (corpus *FixtureConformanceCorpus) Replay() (FixtureConformanceReport, error) {
	if corpus == nil {
		return FixtureConformanceReport{}, fixtureMutationError{FixtureMutationReasonMalformed}
	}
	records := make([]FixtureConformanceRecord, len(corpus.spec.Records))
	profiles := make(map[string]reg.ProfileDescriptor, len(corpus.spec.Profiles))
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

// ErrFixtureConformanceNondeterministic is used by callers detecting a broken report invariant.
var ErrFixtureConformanceNondeterministic = errors.New("fixture conformance report is nondeterministic")

type fixtureCorpusDTO struct {
	SchemaVersion reg.Version       `json:"schema_version"`
	Metadata      json.RawMessage   `json:"metadata"`
	Profiles      []json.RawMessage `json:"profiles"`
	Records       []json.RawMessage `json:"records"`
}
type fixtureRecordDTO struct {
	RecordID       string          `json:"record_id"`
	ProfileID      string          `json:"profile_id"`
	ProfileVersion reg.Version     `json:"profile_version"`
	Observation    json.RawMessage `json:"observation"`
	Detection      json.RawMessage `json:"detection"`
	Qualification  json.RawMessage `json:"qualification"`
	ExpectedReplay json.RawMessage `json:"expected_replay"`
}

// MarshalFixtureConformanceCorpusSpec admits spec under the default limits and
// encodes its canonical bounded JSON form, with profiles and records sorted by
// identity.
func MarshalFixtureConformanceCorpusSpec(spec FixtureConformanceCorpusSpec) ([]byte, error) {
	corpus, err := NewFixtureConformanceCorpus(spec)
	if err != nil {
		return nil, err
	}
	spec = corpus.Spec()
	metadata, err := json.Marshal(spec.Metadata)
	if err != nil {
		return nil, err
	}
	dto := fixtureCorpusDTO{SchemaVersion: spec.SchemaVersion, Metadata: metadata, Profiles: make([]json.RawMessage, len(spec.Profiles)), Records: make([]json.RawMessage, len(spec.Records))}
	for i, profile := range spec.Profiles {
		encoded, err := reg.MarshalProfileDescriptor(profile)
		if err != nil {
			return nil, err
		}
		dto.Profiles[i] = encoded
	}
	for i, record := range spec.Records {
		encoded, err := reg.MarshalFixtureSpec(record.Observation)
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
	return reg.MarshalBounded(dto)
}

// UnmarshalFixtureConformanceCorpus decodes a corpus strictly. Missing, unknown,
// duplicate or case-folded keys and oversized values fail with their fixture
// mutation reason before the corpus is admitted as by NewFixtureConformanceCorpus.
func UnmarshalFixtureConformanceCorpus(data []byte) (*FixtureConformanceCorpus, error) {
	if err := preflightFixtureCorpusJSON(data); err != nil {
		return nil, err
	}
	var dto fixtureCorpusDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, fixtureMutationError{FixtureMutationReasonMalformed}
	}
	if err := reg.PreflightAggregate(dto); err != nil {
		return nil, fixtureMutationError{FixtureMutationReasonOversized}
	}
	var metadata SanitizedFixtureMetadata
	if err := json.Unmarshal(dto.Metadata, &metadata); err != nil {
		return nil, fixtureMutationError{FixtureMutationReasonMalformed}
	}
	if len(metadata.CorpusID) > reg.MaxContractStringBytes || len(metadata.LicenseExpression) > reg.MaxContractStringBytes || len(metadata.Provenance) > reg.MaxContractStringBytes {
		return nil, fixtureMutationError{FixtureMutationReasonOversized}
	}
	if err := reg.DecodeStrict(dto.Metadata, &metadata); err != nil {
		return nil, fixtureMutationError{FixtureMutationReasonMalformed}
	}
	spec := FixtureConformanceCorpusSpec{SchemaVersion: dto.SchemaVersion, Metadata: metadata, Profiles: make([]reg.ProfileDescriptor, len(dto.Profiles)), Records: make([]FixtureConformanceRecordSpec, len(dto.Records))}
	for i, encoded := range dto.Profiles {
		profile, err := reg.UnmarshalProfileDescriptor(encoded)
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
		var observation reg.ObservationSpec
		if err := json.Unmarshal(record.Observation, &observation); err != nil {
			return nil, fixtureMutationError{FixtureMutationReasonMalformed}
		}
		var detection FixtureDetectionCaseSpec
		if err := reg.DecodeStrict(record.Detection, &detection); err != nil {
			return nil, fixtureMutationError{FixtureMutationReasonMalformed}
		}
		var qualification FixtureQualificationExpectation
		if err := reg.DecodeStrict(record.Qualification, &qualification); err != nil {
			return nil, fixtureMutationError{FixtureMutationReasonMalformed}
		}
		var expected FixtureReplayExpectation
		if err := reg.DecodeStrict(record.ExpectedReplay, &expected); err != nil {
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

type fixtureReportDTO struct {
	SchemaVersion reg.Version              `json:"schema_version"`
	Metadata      SanitizedFixtureMetadata `json:"metadata"`
	Records       []fixtureReportRecordDTO `json:"records"`
}
type fixtureDependencyFactDTO struct {
	Ordinal    uint32                   `json:"ordinal"`
	Function   reg.FunctionCode         `json:"function"`
	Table      reg.LogicalTable         `json:"table"`
	UnitID     byte                     `json:"unit_id"`
	Normalized uint16                   `json:"normalized_address"`
	RawWords   []uint16                 `json:"raw_words"`
	Logical    []uint16                 `json:"logical_slice"`
	Source     reg.SourceTimeSpec       `json:"source_time"`
	Wire       FixtureWireProvenance    `json:"wire"`
	View       FixtureLogicalProvenance `json:"logical"`
	Sample     FixtureSampleProvenance  `json:"sample"`
}
type fixtureReportRecordDTO struct {
	RecordID       string                          `json:"record_id"`
	ProfileID      string                          `json:"profile_id"`
	ProfileVersion reg.Version                     `json:"profile_version"`
	Outcome        FixtureReplayOutcome            `json:"outcome"`
	Reason         FixtureReplayReason             `json:"reason"`
	Function       reg.FunctionCode                `json:"function"`
	Table          reg.LogicalTable                `json:"table"`
	UnitID         byte                            `json:"unit_id"`
	Normalized     uint16                          `json:"normalized_address"`
	RawWords       [][]uint16                      `json:"raw_words,omitempty"`
	LogicalSlices  [][]uint16                      `json:"logical_slices,omitempty"`
	Dependencies   []fixtureDependencyFactDTO      `json:"dependencies"`
	Wire           FixtureWireProvenance           `json:"wire"`
	Logical        FixtureLogicalProvenance        `json:"logical"`
	Sample         FixtureSampleProvenance         `json:"sample"`
	Source         reg.SourceTimeSpec              `json:"source_time"`
	Qualification  FixtureQualificationDisposition `json:"qualification"`
	Detection      detectionDecisionDTO            `json:"detection"`
}

// MarshalBoundedReport replays the corpus and encodes the report as bounded
// JSON. It fails as oversized when the encoding exceeds the corpus report-byte
// limit.
func (corpus *FixtureConformanceCorpus) MarshalBoundedReport() ([]byte, error) {
	report, err := corpus.Replay()
	if err != nil {
		return nil, err
	}
	dto := fixtureReportDTO{SchemaVersion: reg.CurrentSchemaVersion(), Metadata: report.metadata, Records: make([]fixtureReportRecordDTO, len(report.records))}
	for i, record := range report.records {
		slices := make([][]uint16, len(record.slices))
		for index, slice := range record.slices {
			slices[index] = append([]uint16(nil), slice.words...)
		}
		decision := detectionDecisionToDTO(record.detection.Actual)
		dependencies := make([]fixtureDependencyFactDTO, len(record.dependencies))
		for index, fact := range record.dependencies {
			dependencies[index] = fixtureDependencyFactDTO{Ordinal: fact.ordinal, Function: fact.function, Table: fact.table, UnitID: fact.unit, Normalized: fact.normalized, RawWords: append([]uint16(nil), fact.rawWords...), Logical: append([]uint16(nil), fact.slice.words...), Source: fact.source, Wire: fact.wire, View: fact.logical, Sample: fact.sample}
		}
		dto.Records[i] = fixtureReportRecordDTO{RecordID: record.recordID, ProfileID: record.profileID, ProfileVersion: record.profileVersion, Outcome: record.replay.actual.outcome, Reason: record.replay.actual.reason, Function: record.function, Table: record.table, UnitID: record.unit, Normalized: record.normalized, RawWords: cloneWordSets(record.replay.actual.rawWords), LogicalSlices: slices, Dependencies: dependencies, Wire: record.wire, Logical: record.logical, Sample: record.sample, Source: record.source, Qualification: record.qualification, Detection: decision}
	}
	encoded, err := reg.MarshalBounded(dto)
	if err != nil {
		return nil, err
	}
	if len(encoded) > corpus.limits.MaxReportBytes {
		return nil, fixtureMutationError{FixtureMutationReasonOversized}
	}
	return encoded, nil
}
