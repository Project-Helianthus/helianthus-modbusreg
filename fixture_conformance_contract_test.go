package modbusreg_test

import (
	"bytes"
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
		observation.Dependencies[index].SourceTime = observation.SourceTime
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
		Input: reg.FixtureDetectorInput{Manufacturer: "manufacturer-alpha", Model: "model-series-a", Firmware: "1.10.0", Probes: []reg.FixtureProbeInput{
			{DeclarationID: "manufacturer-identity", Result: mustProbeResult(t, "manufacturer-alpha", "probe-evidence-manufacturer")},
			{DeclarationID: "model-identity", Result: mustProbeResult(t, "model-series-a", "probe-evidence-model")},
			{DeclarationID: "firmware-identity", Result: mustProbeResult(t, "1.10.0", "probe-evidence-firmware")},
		}},
		Expected: reg.FixtureDetectionExpectation{Outcome: reg.DetectionMatched, Reason: reg.DetectionReasonSelected, SelectedProfileID: profile.ID(), SelectedProfileVersion: profile.Version(), Evidence: []reg.FixtureDetectionEvidenceExpectation{{
			ProfileID: profile.ID(), ProfileVersion: profile.Version(), Reason: reg.DetectionReasonSelected,
			MatchedGates:     []reg.ProbeIdentityField{reg.ProbeIdentityManufacturer, reg.ProbeIdentityModel, reg.ProbeIdentityFirmware},
			ProbeEvidenceIDs: []string{"probe-evidence-manufacturer", "probe-evidence-model", "probe-evidence-firmware"},
			DetectorVersion:  profile.DetectorVersion(), QualificationVersion: profile.QualificationVersion(),
		}}},
	}
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
	first, second := spec.Records[0].Observation.Dependencies[0].View.Record(), spec.Records[0].Observation.Dependencies[1].View.Record()
	first.PhysicalOffset, first.PhysicalWordCount, first.LogicalOffset, first.SliceOffset, first.LogicalWordCount, first.SliceWordCount, first.Words = 100, 6, 100, 0, 2, 2, []uint16{11, 12}
	second.PhysicalOffset, second.PhysicalWordCount, second.LogicalOffset, second.SliceOffset, second.LogicalWordCount, second.SliceWordCount, second.Words = 100, 6, 102, 2, 2, 2, []uint16{13, 14}
	spec.Records[0].Observation.Dependencies[0].View, spec.Records[0].Observation.Dependencies[1].View = snapshotFromRecord(t, first), snapshotFromRecord(t, second)
	corpus, err := reg.NewFixtureConformanceCorpus(spec)
	if err != nil {
		t.Fatalf("NewFixtureConformanceCorpus: %v", err)
	}
	report, err := corpus.Replay()
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	slices := report.Records()[0].LogicalSlices()
	if len(slices) != 2 || !bytes.Equal(slices[0].CanonicalBytes(), []byte{0, 11, 0, 12}) || !bytes.Equal(slices[1].CanonicalBytes(), []byte{0, 13, 0, 14}) {
		t.Fatalf("unequal compatible overlap lost exact logical slices: %#v", slices)
	}
}
