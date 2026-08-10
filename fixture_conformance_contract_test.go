package modbusreg_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// m203SyntheticCorpus contains only generic standard-family declarations and
// offline fixture identities. Detection declarations reconstruct the existing
// ProfileDetector from the corpus catalog; no executable pointer is persisted.
func m203SyntheticCorpus(t *testing.T) reg.FixtureConformanceCorpusSpec {
	t.Helper()
	alpha := detectionProfile(t, "synthetic.standard.alpha", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	beta := m203HoldingProfile(t, "synthetic.standard.beta", "2.0.0")
	return reg.FixtureConformanceCorpusSpec{
		SchemaVersion: version(t, "1.0.0"),
		Metadata:      reg.SanitizedFixtureMetadata{CorpusID: "synthetic-conformance-public", LicenseExpression: "CC0-1.0", Provenance: "public synthetic fixture"},
		Profiles:      []reg.ProfileDescriptor{alpha, beta},
		Records: []reg.FixtureConformanceRecordSpec{
			m203Record(t, "synthetic-fc04-alpha", alpha, 17, 501),
			m203Record(t, "synthetic-fc03-beta", beta, 18, 502),
		},
	}
}

func m203HoldingProfile(t *testing.T, id, profileVersion string) reg.ProfileDescriptor {
	t.Helper()
	base := detectionProfile(t, id, profileVersion, reg.MaturityQualified, reg.ProfileActive, true)
	dependencies := base.Dependencies().Dependencies()
	holding := make([]reg.Dependency, len(dependencies))
	for index, dependency := range dependencies {
		spec := dependency.Spec()
		spec.Table = reg.HoldingRegisters
		spec.Normalization.DocumentaryNotation = "one-based holding register"
		spec.Normalization.AddressSpaceLabel = string(reg.HoldingRegisters)
		converted, err := reg.NewDependency(spec)
		if err != nil {
			t.Fatalf("NewDependency(holding): %v", err)
		}
		holding[index] = converted
	}
	set, err := reg.NewDependencySet(base.Dependencies().Version(), holding)
	if err != nil {
		t.Fatalf("NewDependencySet(holding): %v", err)
	}
	spec := base.Spec()
	spec.Dependencies = set
	profile, err := reg.NewProfileDescriptor(spec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(holding): %v", err)
	}
	return profile
}

func m203Record(t *testing.T, id string, profile reg.ProfileDescriptor, unit byte, generation uint64) reg.FixtureConformanceRecordSpec {
	t.Helper()
	observation := successfulObservationSpec(t, profile)
	observation.Endpoint, observation.UnitID, observation.PollGenerationID = "fixture-endpoint-"+id, unit, generation
	observation.SourceTime = reg.SourceTimeObserved(time.Unix(1_700_001_000+int64(unit), 0).UTC())
	for index := range observation.Dependencies {
		record := observation.Dependencies[index].View.Record()
		record.Endpoint, record.UnitID, record.PollGeneration = observation.Endpoint, unit, generation
		if profile.Dependencies().Dependencies()[index].Table() == reg.HoldingRegisters {
			record.RequestedFunction = reg.FunctionReadHoldingRegisters
			record.ReceivedFunction = reg.FunctionReadHoldingRegisters
			record.Table = reg.HoldingRegisters
		}
		observation.Dependencies[index].View = snapshotFromRecord(t, record)
	}
	return reg.FixtureConformanceRecordSpec{
		RecordID: id, ProfileID: profile.ID(), ProfileVersion: profile.Version(), Observation: observation,
		Detection:      m203DetectionCase(t, profile),
		Qualification:  reg.FixtureQualificationExpectation{Expected: reg.FixtureQualificationDispositionQualified},
		ExpectedReplay: reg.FixtureReplayExpectation{Outcome: reg.FixtureReplayAccepted, Reason: reg.FixtureReplayReasonAccepted, ExpectedRawWords: [][]uint16{{0x0102, 0x0304}, {0x0304, 0x0506}}},
	}
}

func m203DetectionCase(t *testing.T, profile reg.ProfileDescriptor) reg.FixtureDetectionCaseSpec {
	t.Helper()
	return reg.FixtureDetectionCaseSpec{
		Declaration: reg.FixtureDetectorDeclarationSpec{
			DetectorVersion: profile.DetectorVersion(),
			Plan:            detectionPlan(t).Spec(),
			Candidates: []reg.DetectionCandidateSpec{
				detectionCandidate(t, profile, 100, true, false).Spec(),
			},
			Limits: detectionLimits(),
		},
		Input: reg.FixtureDetectorInput{Probes: []reg.FixtureProbeInput{
			{DeclarationID: "manufacturer-identity", Result: m203ProbeResultSpec("manufacturer-alpha", "probe-evidence-manufacturer")},
			{DeclarationID: "model-identity", Result: m203ProbeResultSpec("model-series-a", "probe-evidence-model")},
			{DeclarationID: "firmware-identity", Result: m203ProbeResultSpec("1.10.0", "probe-evidence-firmware")},
		}},
		Expected: reg.FixtureDetectionExpectation{Outcome: reg.DetectionMatched, Reason: reg.DetectionReasonSelected, SelectedProfileID: profile.ID(), SelectedProfileVersion: profile.Version(), Evidence: []reg.FixtureDetectionEvidenceExpectation{{
			ProfileID: profile.ID(), ProfileVersion: profile.Version(), Score: 100, Reason: reg.DetectionReasonSelected,
			MatchedGates:     []reg.ProbeIdentityField{reg.ProbeIdentityManufacturer, reg.ProbeIdentityModel, reg.ProbeIdentityFirmware},
			ProbeEvidenceIDs: []string{"probe-evidence-manufacturer", "probe-evidence-model", "probe-evidence-firmware"},
			DetectorVersion:  profile.DetectorVersion(), QualificationVersion: profile.QualificationVersion(),
		}}},
	}
}

func m203ProbeResultSpec(value, evidenceID string) reg.ProbeReadResultSpec {
	return reg.ProbeReadResultSpec{Status: reg.ProbeReadSucceeded, Words: detectionWords(value), EvidenceID: evidenceID}
}

func TestM203FixtureCorpusBindsSanitizedTransportNeutralEvidence(t *testing.T) {
	corpus, err := reg.NewFixtureConformanceCorpus(m203SyntheticCorpus(t))
	if err != nil {
		t.Fatalf("NewFixtureConformanceCorpus: %v", err)
	}
	report, err := corpus.Replay()
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if report.RecordCount() != 2 || report.RejectedCount() != 0 {
		t.Fatalf("report counts = accepted %d rejected %d", report.RecordCount(), report.RejectedCount())
	}
	metadata := report.Metadata()
	if metadata.CorpusID != "synthetic-conformance-public" || metadata.LicenseExpression != "CC0-1.0" || metadata.Provenance != "public synthetic fixture" {
		t.Fatalf("sanitized metadata = %#v", metadata)
	}
	result := report.Records()[0]
	tables := map[reg.FunctionCode]reg.LogicalTable{}
	for _, record := range report.Records() {
		tables[record.FunctionCode()] = record.Table()
	}
	if tables[reg.FunctionReadInputRegisters] != reg.InputRegisters || tables[reg.FunctionReadHoldingRegisters] != reg.HoldingRegisters || result.UnitID() == 0 || result.NormalizedAddress() == 0 || len(result.RawWords()) == 0 {
		t.Fatalf("FC03/FC04 table/unit/address/raw-word provenance = %#v", report.Records())
	}
	if result.WireProvenance().WireResponseID == 0 || result.LogicalProvenance().LogicalViewID == 0 || result.SampleProvenance().PollGenerationID == 0 || result.SourceTime().State != reg.SourceTimeObservedState {
		t.Fatalf("incomplete provenance: %#v", result)
	}
	detection := result.Detection()
	if detection.Actual.Outcome() != reg.DetectionMatched || detection.Actual.Reason() != reg.DetectionReasonSelected || detection.Actual.SelectedProfileID() != result.ProfileID() || detection.Actual.SelectedProfileVersion() != result.ProfileVersion() || !detection.MatchesExpected() || result.Qualification() != reg.FixtureQualificationDispositionQualified {
		t.Fatalf("detector/qualification result = %#v", result)
	}
}

func TestM203CorpusSpecAndResultAccessorsAreImmutableAndBounded(t *testing.T) {
	corpus, err := reg.NewFixtureConformanceCorpus(m203SyntheticCorpus(t))
	if err != nil {
		t.Fatalf("NewFixtureConformanceCorpus: %v", err)
	}
	spec := corpus.Spec()
	spec.Metadata.CorpusID, spec.Profiles = "mutated", nil
	spec.Records[0].ExpectedReplay.ExpectedRawWords[0][0] = 0xffff
	if got := corpus.Spec(); got.Metadata.CorpusID != "synthetic-conformance-public" || len(got.Profiles) != 2 || got.Records[0].ExpectedReplay.ExpectedRawWords[0][0] != 0x0102 {
		t.Fatalf("corpus spec accessor leaked mutation: %#v", got)
	}
	report, err := corpus.Replay()
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	words := report.Records()[0].RawWords()
	words[0] = 0xffff
	if report.Records()[0].RawWords()[0] == 0xffff {
		t.Fatal("report raw-word accessor leaked mutation")
	}
	if _, err := reg.NewFixtureConformanceCorpusWithLimits(m203SyntheticCorpus(t), reg.FixtureConformanceLimits{MaxRecords: 1, MaxReportBytes: reg.MaxSerializedContractBytes}); !reg.IsFixtureMutationReason(err, reg.FixtureMutationReasonOversized) {
		t.Fatalf("bounded corpus error = %v, want oversized", err)
	}
}

func TestM203CompatibleUnequalOverlapsRetainExactLogicalSlices(t *testing.T) {
	spec := m203SyntheticCorpus(t)
	corpus, err := reg.NewFixtureConformanceCorpus(spec)
	if err != nil {
		t.Fatalf("NewFixtureConformanceCorpus: %v", err)
	}
	report, err := corpus.Replay()
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	slices := report.Records()[1].LogicalSlices()
	if len(slices) != 2 || !bytes.Equal(slices[0].CanonicalBytes(), []byte{1, 2, 3, 4}) || !bytes.Equal(slices[1].CanonicalBytes(), []byte{3, 4, 5, 6}) {
		t.Fatalf("unequal compatible overlap lost exact logical slices: %#v", slices)
	}
}

func TestM203BoundedMultiReportRetainsEveryFC03FC04DependencyFact(t *testing.T) {
	base, observation := boundedFixture(t, reg.AcquisitionOrderDependencyDeclaration)
	dependencies := base.Dependencies().Dependencies()
	second := dependencies[1].Spec()
	second.Table = reg.HoldingRegisters
	second.Normalization.DocumentaryNotation = "one-based holding register"
	second.Normalization.AddressSpaceLabel = string(reg.HoldingRegisters)
	converted, err := reg.NewDependency(second)
	if err != nil {
		t.Fatalf("NewDependency(mixed holding): %v", err)
	}
	dependencies[1] = converted
	set, err := reg.NewDependencySet(base.Dependencies().Version(), dependencies)
	if err != nil {
		t.Fatalf("NewDependencySet(mixed): %v", err)
	}
	profileSpec := base.Spec()
	profileSpec.Dependencies = set
	profileSpec.Maturity = reg.MaturityQualified
	profileSpec.DefaultEnabled = true
	profile, err := reg.NewProfileDescriptor(profileSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(mixed): %v", err)
	}
	observation.DependencySetID = profile.Dependencies().ID()
	observation.DependencySetVersion = profile.Dependencies().Version()
	for index := range observation.Dependencies {
		declaration := profile.Dependencies().Dependencies()[index]
		observation.Dependencies[index].DependencyID = declaration.ID()
		observation.Dependencies[index].DependencyVersion = declaration.Version()
		observation.Dependencies[index].CodecID = declaration.CodecID()
		observation.Dependencies[index].CodecVersion = declaration.CodecVersion()
		observation.Dependencies[index].NormalizationVersion = declaration.Normalization().Spec().Version
		view := observation.Dependencies[index].View.Record()
		view.Endpoint, view.UnitID, view.PollGeneration = "fixture-mixed-fc03-fc04", 31, 701
		if index == 1 {
			view.RequestedFunction = reg.FunctionReadHoldingRegisters
			view.ReceivedFunction = reg.FunctionReadHoldingRegisters
			view.Table = reg.HoldingRegisters
			view.WireResponseBytes = append([]byte(nil), view.WireResponseBytes...)
			view.WireResponseBytes[1] = byte(reg.FunctionReadHoldingRegisters)
		}
		observation.Dependencies[index].View = snapshotFromRecord(t, view)
	}
	observation.Endpoint, observation.UnitID, observation.PollGenerationID = "fixture-mixed-fc03-fc04", 31, 701
	record := m203Record(t, "fixture-mixed-fc03-fc04", profile, 31, 701)
	record.Observation = observation
	record.ExpectedReplay.ExpectedRawWords = make([][]uint16, len(observation.Dependencies))
	for index, dependency := range observation.Dependencies {
		record.ExpectedReplay.ExpectedRawWords[index] = dependency.View.Record().Words
	}
	spec := reg.FixtureConformanceCorpusSpec{SchemaVersion: version(t, "1.0.0"), Metadata: reg.SanitizedFixtureMetadata{CorpusID: "fixture-mixed-fc03-fc04", LicenseExpression: "CC0-1.0", Provenance: "public synthetic fixture"}, Profiles: []reg.ProfileDescriptor{profile}, Records: []reg.FixtureConformanceRecordSpec{record}}
	corpus, err := reg.NewFixtureConformanceCorpus(spec)
	if err != nil {
		t.Fatalf("NewFixtureConformanceCorpus: %v", err)
	}
	report, err := corpus.Replay()
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	facts := report.Records()[0].DependencyFacts()
	if len(facts) != 2 {
		t.Fatalf("dependency facts = %d, want 2", len(facts))
	}
	for index, want := range []struct {
		function reg.FunctionCode
		table    reg.LogicalTable
	}{{reg.FunctionReadInputRegisters, reg.InputRegisters}, {reg.FunctionReadHoldingRegisters, reg.HoldingRegisters}} {
		fact := facts[index]
		if fact.Ordinal() != uint32(index) || fact.FunctionCode() != want.function || fact.Table() != want.table || fact.UnitID() != 31 || fact.NormalizedAddress() == 0 || len(fact.RawWords()) == 0 || len(fact.LogicalSlice().CanonicalBytes()) == 0 || fact.SourceTime().State != reg.SourceTimeObservedState || fact.WireProvenance().WireResponseID == 0 || fact.LogicalProvenance().LogicalViewID == 0 || fact.SampleProvenance().PollGenerationID != 701 {
			t.Fatalf("dependency %d lost FC03/FC04 association: %#v", index, fact)
		}
	}
	facts[0].RawWords()[0] = 0xffff
	if report.Records()[0].DependencyFacts()[0].RawWords()[0] == 0xffff {
		t.Fatal("dependency facts accessor leaked mutation")
	}
	encoded, err := corpus.MarshalBoundedReport()
	if err != nil {
		t.Fatalf("MarshalBoundedReport: %v", err)
	}
	var serialized struct {
		SchemaVersion string `json:"schema_version"`
		Records       []struct {
			Dependencies []struct {
				Ordinal  uint32   `json:"ordinal"`
				Function uint8    `json:"function"`
				Table    string   `json:"table"`
				Words    []uint16 `json:"raw_words"`
				Logical  []uint16 `json:"logical_slice"`
				Wire     struct {
					ID uint64 `json:"WireResponseID"`
				} `json:"wire"`
				View struct {
					ID uint64 `json:"LogicalViewID"`
				} `json:"logical"`
			} `json:"dependencies"`
		} `json:"records"`
	}
	if err := json.Unmarshal(encoded, &serialized); err != nil {
		t.Fatalf("report JSON: %v", err)
	}
	if serialized.SchemaVersion != "1.0.0" || len(serialized.Records) != 1 || len(serialized.Records[0].Dependencies) != 2 {
		t.Fatalf("serialized dependency report lost version or facts: %#v", serialized)
	}
	for index, want := range []struct {
		function uint8
		table    string
	}{{uint8(reg.FunctionReadInputRegisters), string(reg.InputRegisters)}, {uint8(reg.FunctionReadHoldingRegisters), string(reg.HoldingRegisters)}} {
		fact := serialized.Records[0].Dependencies[index]
		if fact.Ordinal != uint32(index) || fact.Function != want.function || fact.Table != want.table || len(fact.Words) == 0 || len(fact.Logical) == 0 || fact.Wire.ID == 0 || fact.View.ID == 0 {
			t.Fatalf("serialized dependency %d lost association: %#v", index, fact)
		}
	}
	if repeat, marshalErr := corpus.MarshalBoundedReport(); marshalErr != nil || !bytes.Equal(encoded, repeat) {
		t.Fatalf("report serialization is nondeterministic: %q, %v", repeat, marshalErr)
	}
	limited, err := reg.NewFixtureConformanceCorpusWithLimits(spec, reg.FixtureConformanceLimits{MaxRecords: 1, MaxReportBytes: len(encoded) - 1})
	if err != nil {
		t.Fatalf("NewFixtureConformanceCorpusWithLimits: %v", err)
	}
	if _, err := limited.MarshalBoundedReport(); !reg.IsFixtureMutationReason(err, reg.FixtureMutationReasonOversized) {
		t.Fatalf("one-byte-short report limit error = %v", err)
	}
}

func TestM203BoundedMultiPreservesCanonicalSourceSkew(t *testing.T) {
	profile, observation := boundedFixture(t, reg.AcquisitionOrderDependencyDeclaration)
	profileSpec := profile.Spec()
	profileSpec.Maturity = reg.MaturityQualified
	profileSpec.DefaultEnabled = true
	profile, err := reg.NewProfileDescriptor(profileSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(qualified bounded): %v", err)
	}
	observation.Endpoint, observation.UnitID = "fixture-bounded-source", 19
	for index := range observation.Dependencies {
		view := observation.Dependencies[index].View.Record()
		view.Endpoint, view.UnitID = observation.Endpoint, observation.UnitID
		observation.Dependencies[index].View = snapshotFromRecord(t, view)
	}
	record := m203Record(t, "fixture-bounded-valid", profile, observation.UnitID, observation.PollGenerationID)
	record.Observation = observation
	corpusSpec := reg.FixtureConformanceCorpusSpec{SchemaVersion: version(t, "1.0.0"), Metadata: reg.SanitizedFixtureMetadata{CorpusID: "fixture-bounded-corpus", LicenseExpression: "CC0-1.0", Provenance: "public synthetic fixture"}, Profiles: []reg.ProfileDescriptor{profile}, Records: []reg.FixtureConformanceRecordSpec{record}}
	if _, err := reg.NewFixtureConformanceCorpus(corpusSpec); err != nil {
		t.Fatalf("canonical bounded source skew rejected: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*reg.LogicalViewRecord)
	}{
		{"endpoint", func(view *reg.LogicalViewRecord) { view.Endpoint = "fixture-bounded-other" }},
		{"connection", func(view *reg.LogicalViewRecord) { view.ConnectionID++ }},
		{"transport generation", func(view *reg.LogicalViewRecord) { view.TransportGeneration++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			negative := corpusSpec
			negative.Records = append([]reg.FixtureConformanceRecordSpec(nil), corpusSpec.Records...)
			negative.Records[0].Observation.Dependencies = append([]reg.DependencyResult(nil), corpusSpec.Records[0].Observation.Dependencies...)
			view := negative.Records[0].Observation.Dependencies[1].View.Record()
			test.mutate(&view)
			negative.Records[0].Observation.Dependencies[1].View = snapshotFromRecord(t, view)
			negative.Records[0].ExpectedReplay = reg.FixtureReplayExpectation{Outcome: reg.FixtureReplayRejected, Reason: reg.FixtureReplayReasonSourceMismatch}
			if _, err := reg.NewFixtureConformanceCorpus(negative); err != nil {
				t.Fatalf("bounded source mismatch rejected: %v", err)
			}
		})
	}
}

func TestM203RejectedQualificationMatchesUnqualifiedDetection(t *testing.T) {
	profile := detectionProfile(t, "synthetic.standard.unqualified", "1.0.0", reg.MaturityExperimental, reg.ProfileActive, false)
	record := m203Record(t, "fixture-unqualified", profile, 20, 601)
	record.Qualification.Expected = reg.FixtureQualificationDispositionRejected
	record.Detection.Expected = reg.FixtureDetectionExpectation{Outcome: reg.DetectionNoMatch, Reason: reg.DetectionReasonProfileUnqualified, Evidence: []reg.FixtureDetectionEvidenceExpectation{{ProfileID: profile.ID(), ProfileVersion: profile.Version(), Score: 100, Reason: reg.DetectionReasonProfileUnqualified, DetectorVersion: profile.DetectorVersion(), QualificationVersion: profile.QualificationVersion()}}}
	spec := reg.FixtureConformanceCorpusSpec{SchemaVersion: version(t, "1.0.0"), Metadata: reg.SanitizedFixtureMetadata{CorpusID: "fixture-unqualified-corpus", LicenseExpression: "CC0-1.0", Provenance: "public synthetic fixture"}, Profiles: []reg.ProfileDescriptor{profile}, Records: []reg.FixtureConformanceRecordSpec{record}}
	corpus, err := reg.NewFixtureConformanceCorpus(spec)
	if err != nil {
		t.Fatalf("unqualified/rejected corpus: %v", err)
	}
	report, err := corpus.Replay()
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	result := report.Records()[0]
	if result.Qualification() != reg.FixtureQualificationDispositionRejected || result.Detection().Actual.Outcome() != reg.DetectionNoMatch || result.Detection().Actual.Reason() != reg.DetectionReasonProfileUnqualified || !result.Detection().MatchesExpected() {
		t.Fatalf("unqualified result = %#v", result)
	}
	qualifiedRejected := m203SyntheticCorpus(t)
	qualifiedRejected.Records[0].Qualification.Expected = reg.FixtureQualificationDispositionRejected
	if _, err := reg.NewFixtureConformanceCorpus(qualifiedRejected); !reg.IsFixtureMutationReason(err, reg.FixtureMutationReasonContradictory) {
		t.Fatalf("qualified/rejected error = %v", err)
	}
	unqualifiedQualified := spec
	unqualifiedQualified.Records = append([]reg.FixtureConformanceRecordSpec(nil), spec.Records...)
	unqualifiedQualified.Records[0].Qualification.Expected = reg.FixtureQualificationDispositionQualified
	if _, err := reg.NewFixtureConformanceCorpus(unqualifiedQualified); !reg.IsFixtureMutationReason(err, reg.FixtureMutationReasonContradictory) {
		t.Fatalf("unqualified/qualified error = %v", err)
	}
}
