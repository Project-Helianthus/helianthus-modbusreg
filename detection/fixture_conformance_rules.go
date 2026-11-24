package detection

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func validFixtureIdentity(value string) bool {
	return reg.ValidIdentity(value) && (strings.HasPrefix(value, "fixture-") || strings.HasPrefix(value, "fixture:"))
}

func validateSanitizedMetadata(metadata SanitizedFixtureMetadata) error {
	if reg.ValidateBoundedString("fixture corpus ID", metadata.CorpusID, true) != nil ||
		reg.ValidateBoundedString("fixture license expression", metadata.LicenseExpression, true) != nil ||
		reg.ValidateBoundedString("fixture provenance", metadata.Provenance, true) != nil {
		return fixtureMutationError{FixtureMutationReasonOversized}
	}
	if !reg.ValidIdentity(metadata.CorpusID) || metadata.LicenseExpression != "CC0-1.0" || metadata.Provenance != "public synthetic fixture" {
		return fixtureMutationError{FixtureMutationReasonUnsanitized}
	}
	return nil
}

func validateFixtureSanitization(record FixtureConformanceRecordSpec) error {
	if !validFixtureIdentity(record.Observation.Endpoint) {
		return fixtureMutationError{FixtureMutationReasonUnsanitized}
	}
	for _, dependency := range record.Observation.Dependencies {
		if !validFixtureIdentity(dependency.View.Record().Endpoint) {
			return fixtureMutationError{FixtureMutationReasonUnsanitized}
		}
		if dependency.View.Record().Endpoint != record.Observation.Endpoint && (record.ExpectedReplay.Outcome != FixtureReplayRejected || record.ExpectedReplay.Reason != FixtureReplayReasonSourceMismatch) {
			return fixtureMutationError{FixtureMutationReasonUnsanitized}
		}
	}
	return nil
}

func validateFixtureRecordDeclaration(catalog reg.Catalog, profile reg.ProfileDescriptor, record FixtureConformanceRecordSpec) error {
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
	decision, err := detector.Detect(context.Background(), reader, Options{})
	if err != nil || !(FixtureDetectionResult{Actual: decision, expected: record.Detection.Expected}).MatchesExpected() {
		return fixtureMutationError{FixtureMutationReasonContradictory}
	}
	switch record.Qualification.Expected {
	case FixtureQualificationDispositionQualified:
		if decision.Outcome() != Matched {
			return fixtureMutationError{FixtureMutationReasonContradictory}
		}
	case FixtureQualificationDispositionRejected:
		if decision.Outcome() != NoMatch {
			return fixtureMutationError{FixtureMutationReasonContradictory}
		}
	default:
		return fixtureMutationError{FixtureMutationReasonMalformed}
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

func newFixtureDetector(catalog reg.Catalog, declaration FixtureDetectorDeclarationSpec) (*ProfileDetector, error) {
	plan, err := NewProbePlan(declaration.Plan, declaration.Limits)
	if err != nil {
		return nil, err
	}
	candidates := make([]Candidate, len(declaration.Candidates))
	for index, spec := range declaration.Candidates {
		candidate, candidateErr := NewCandidate(spec, declaration.Limits)
		if candidateErr != nil {
			return nil, candidateErr
		}
		candidates[index] = candidate
	}
	return NewProfileDetector(ProfileDetectorSpec{DetectorVersion: declaration.DetectorVersion, Plan: plan, Catalog: catalog, Candidates: candidates, Limits: declaration.Limits})
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

func classifyFixtureObservation(profile reg.ProfileDescriptor, spec reg.ObservationSpec) FixtureReplayReason {
	if fixtureObservationAdmitted(profile, spec) {
		return FixtureReplayReasonAccepted
	}
	declarations := profile.Dependencies().Dependencies()
	if len(spec.Dependencies) != len(declarations) {
		return FixtureReplayReasonObservationRejected
	}
	policy := profile.Spec().Coherence
	firstView := spec.Dependencies[0].View.Record()
	for i, dependency := range spec.Dependencies {
		view := dependency.View.Record()
		if dependency.Status == reg.DependencyReadTorn {
			return FixtureReplayReasonTornRead
		}
		if dependency.NormalizationVersion != declarations[i].Normalization().Spec().Version {
			return FixtureReplayReasonNormalizationMismatch
		}
		if view.UnitID != spec.UnitID {
			return FixtureReplayReasonUnitMismatch
		}
		if policy.Mode == reg.CoherenceBoundedMultiResponse &&
			(view.Endpoint != firstView.Endpoint ||
				view.ConnectionID != firstView.ConnectionID ||
				view.Transport != firstView.Transport ||
				(policy.RequireGenerationEquality && view.TransportGeneration != firstView.TransportGeneration)) {
			return FixtureReplayReasonSourceMismatch
		}
		if view.PollGeneration != spec.PollGenerationID {
			return FixtureReplayReasonGenerationMismatch
		}
		if view.DeadlineIdentity != spec.Dependencies[0].View.Record().DeadlineIdentity {
			return FixtureReplayReasonDeadlineMismatch
		}
		if view.Table != declarations[i].Table() || (view.Table == reg.HoldingRegisters && view.RequestedFunction != reg.FunctionReadHoldingRegisters) || (view.Table == reg.InputRegisters && view.RequestedFunction != reg.FunctionReadInputRegisters) || view.RequestedFunction != view.ReceivedFunction {
			return FixtureReplayReasonTableAccessMismatch
		}
		if policy.Mode == reg.CoherenceSingleWireResponse {
			if dependency.SourceTime.State != reg.SourceTimeUnavailableState || !dependency.SourceTime.Time.IsZero() {
				return FixtureReplayReasonSourceMismatch
			}
		}
		if policy.Mode == reg.CoherenceBoundedMultiResponse &&
			policy.DocumentaryConsistencyMarker != "" &&
			dependency.DocumentaryConsistencyMarker != policy.DocumentaryConsistencyMarker {
			return FixtureReplayReasonCoherenceMismatch
		}
	}
	return FixtureReplayReasonObservationRejected
}

func fixtureObservationAdmitted(profile reg.ProfileDescriptor, spec reg.ObservationSpec) bool {
	encoded, err := reg.MarshalFixtureSpec(spec)
	if err != nil {
		return false
	}
	replayer, err := NewFixtureReplayer(profile)
	if err != nil {
		return false
	}
	_, err = replayer.Replay(encoded)
	return err == nil
}

func actualFixtureReplay(profile reg.ProfileDescriptor, spec reg.ObservationSpec) (FixtureReplayActual, error) {
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
	encoded, err := reg.MarshalFixtureSpec(spec)
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

func replayFixtureRecord(catalog reg.Catalog, profile reg.ProfileDescriptor, spec FixtureConformanceRecordSpec) (FixtureConformanceRecord, error) {
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
	decision, err := detector.Detect(context.Background(), reader, Options{})
	if err != nil {
		return FixtureConformanceRecord{}, err
	}
	view := spec.Observation.Dependencies[0].View.Record()
	result := FixtureConformanceRecord{recordID: spec.RecordID, profileID: spec.ProfileID, profileVersion: spec.ProfileVersion, function: view.RequestedFunction, table: view.Table, unit: spec.Observation.UnitID, normalized: view.LogicalOffset, rawWords: append([]uint16(nil), view.Words...), source: spec.Observation.SourceTime, wire: FixtureWireProvenance{view.WireResponseID}, logical: FixtureLogicalProvenance{view.LogicalViewID}, sample: FixtureSampleProvenance{PollGenerationID: spec.Observation.PollGenerationID}, detection: FixtureDetectionResult{Actual: decision, expected: spec.Detection.Expected}, qualification: spec.Qualification.Expected, replay: FixtureReplayResult{actual: actual, expected: spec.ExpectedReplay}}
	result.dependencies = make([]FixtureDependencyFact, len(spec.Observation.Dependencies))
	for index, dependency := range spec.Observation.Dependencies {
		dependencyView := dependency.View.Record()
		result.dependencies[index] = FixtureDependencyFact{ordinal: uint32(index), function: dependencyView.RequestedFunction, table: dependencyView.Table, unit: dependencyView.UnitID, normalized: dependencyView.LogicalOffset, rawWords: append([]uint16(nil), actual.rawWords[index]...), slice: FixtureLogicalSlice{words: append([]uint16(nil), actual.rawWords[index]...)}, source: dependency.SourceTime, wire: FixtureWireProvenance{dependencyView.WireResponseID}, logical: FixtureLogicalProvenance{dependencyView.LogicalViewID}, sample: FixtureSampleProvenance{PollGenerationID: dependencyView.PollGeneration}}
	}
	result.slices = make([]FixtureLogicalSlice, len(actual.rawWords))
	for index, words := range actual.rawWords {
		result.slices[index] = FixtureLogicalSlice{words: append([]uint16(nil), words...)}
	}
	if len(actual.rawWords) != 0 {
		result.rawWords = append([]uint16(nil), actual.rawWords[0]...)
	}
	return result, nil
}

func preflightFixtureCorpusJSON(data []byte) error {
	if len(data) == 0 {
		return fixtureMutationError{FixtureMutationReasonMalformed}
	}
	if len(data) > reg.MaxSerializedContractBytes {
		return fixtureMutationError{FixtureMutationReasonOversized}
	}
	fields, err := preflightFixtureObjectJSON(data, "schema_version", "metadata", "profiles", "records")
	if err != nil {
		return err
	}
	if _, err := preflightFixtureObjectJSON(fields["metadata"], "corpus_id", "license_expression", "provenance"); err != nil {
		return err
	}
	if err := preflightFixtureArrayJSON(fields["profiles"], nil); err != nil {
		return err
	}
	if err := preflightFixtureArrayJSON(fields["records"], preflightFixtureRecordJSON); err != nil {
		return err
	}
	return nil
}

// preflightFixtureObjectJSON classifies every M2-03-owned object boundary
// before the strict DTO decode. The DTO decode remains the authority for types
// and canonical construction; this pass only preserves the closed mutation
// reason contract without retaining unbounded object graphs.
func preflightFixtureObjectJSON(data []byte, keys ...string) (map[string]json.RawMessage, error) {
	return preflightFixtureObjectJSONOptional(data, nil, keys...)
}

func preflightFixtureObjectJSONOptional(data []byte, optional []string, keys ...string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fixtureMutationError{FixtureMutationReasonMalformed}
	}
	expected := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		expected[key] = struct{}{}
	}
	fields := make(map[string]json.RawMessage, len(keys))
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return nil, fixtureMutationError{FixtureMutationReasonMalformed}
		}
		folded := strings.ToLower(key)
		if _, duplicate := fields[folded]; duplicate {
			if key == strings.ToLower(key) {
				return nil, fixtureMutationError{FixtureMutationReasonDuplicate}
			}
			return nil, fixtureMutationError{FixtureMutationReasonCaseFolded}
		}
		if _, known := expected[key]; !known {
			if _, foldedKnown := expected[folded]; foldedKnown {
				return nil, fixtureMutationError{FixtureMutationReasonCaseFolded}
			}
			return nil, fixtureMutationError{FixtureMutationReasonUnknown}
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, fixtureMutationError{FixtureMutationReasonMalformed}
		}
		fields[folded] = value
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, fixtureMutationError{FixtureMutationReasonMalformed}
	}
	optionalKeys := make(map[string]struct{}, len(optional))
	for _, key := range optional {
		optionalKeys[key] = struct{}{}
	}
	for key := range expected {
		if _, isOptional := optionalKeys[key]; isOptional {
			continue
		}
		if _, present := fields[key]; !present {
			return nil, fixtureMutationError{FixtureMutationReasonMissing}
		}
	}
	return fields, nil
}

func preflightFixtureArrayJSON(data []byte, element func([]byte) error) error {
	return preflightFixtureArrayJSONWithLimit(data, reg.MaxProfileDependencies, element)
}

func preflightFixtureArrayJSONWithLimit(data []byte, limit int, element func([]byte) error) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('[') {
		return fixtureMutationError{FixtureMutationReasonMalformed}
	}
	count := 0
	for decoder.More() {
		if count == limit {
			return fixtureMutationError{FixtureMutationReasonOversized}
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return fixtureMutationError{FixtureMutationReasonMalformed}
		}
		if element != nil {
			if err := element(value); err != nil {
				return err
			}
		}
		count++
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim(']') {
		return fixtureMutationError{FixtureMutationReasonMalformed}
	}
	return nil
}

func preflightFixtureRecordJSON(data []byte) error {
	fields, err := preflightFixtureObjectJSON(data, "record_id", "profile_id", "profile_version", "observation", "detection", "qualification", "expected_replay")
	if err != nil {
		return err
	}
	if err := preflightFixtureDetectionJSON(fields["detection"]); err != nil {
		return err
	}
	if _, err := preflightFixtureObjectJSON(fields["qualification"], "expected"); err != nil {
		return err
	}
	return preflightFixtureExpectedReplayJSON(fields["expected_replay"])
}

func preflightFixtureExpectedReplayJSON(data []byte) error {
	fields, err := preflightFixtureObjectJSONOptional(data, []string{"expected_raw_words"}, "outcome", "reason", "expected_raw_words")
	if err != nil {
		return err
	}
	rawWords, present := fields["expected_raw_words"]
	if !present {
		return nil
	}
	return preflightFixtureArrayJSON(rawWords, func(words []byte) error {
		return preflightFixtureArrayJSONWithLimit(words, reg.MaxRawWords, nil)
	})
}

func preflightFixtureDetectionJSON(data []byte) error {
	fields, err := preflightFixtureObjectJSON(data, "declaration", "input", "expected")
	if err != nil {
		return err
	}
	if _, err := preflightFixtureObjectJSON(fields["declaration"], "detector_version", "plan", "candidates", "limits"); err != nil {
		return err
	}
	input, err := preflightFixtureObjectJSON(fields["input"], "probes")
	if err != nil {
		return err
	}
	if err := preflightFixtureArrayJSON(input["probes"], preflightFixtureProbeJSON); err != nil {
		return err
	}
	expected, err := preflightFixtureObjectJSON(fields["expected"], "outcome", "reason", "selected_profile_id", "selected_profile_version", "evidence")
	if err != nil {
		return err
	}
	return preflightFixtureArrayJSON(expected["evidence"], preflightFixtureDetectionEvidenceJSON)
}

func preflightFixtureProbeJSON(data []byte) error {
	_, err := preflightFixtureObjectJSON(data, "declaration_id", "result")
	return err
}

func preflightFixtureDetectionEvidenceJSON(data []byte) error {
	_, err := preflightFixtureObjectJSON(data, "profile_id", "profile_version", "score", "reason", "matched_gates", "probe_evidence_ids", "detector_version", "qualification_version")
	return err
}

func validateFixtureConformanceLimits(limits FixtureConformanceLimits) error {
	if limits.MaxRecords <= 0 || limits.MaxRecords > reg.MaxProfileDependencies || limits.MaxReportBytes <= 0 || limits.MaxReportBytes > reg.MaxSerializedContractBytes {
		return fixtureMutationError{FixtureMutationReasonOversized}
	}
	return nil
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
