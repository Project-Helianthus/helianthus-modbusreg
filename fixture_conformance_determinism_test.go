package modbusreg_test

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestM203CorpusReplayIsDeterministicAcrossRealPermutationsAndConcurrentRuns(t *testing.T) {
	baseline, err := reg.NewFixtureConformanceCorpus(m203SyntheticCorpus(t))
	if err != nil {
		t.Fatalf("NewFixtureConformanceCorpus: %v", err)
	}
	want, err := baseline.MarshalBoundedReport()
	if err != nil {
		t.Fatalf("MarshalBoundedReport: %v", err)
	}
	permuted := m203SyntheticCorpus(t)
	permuted.Profiles[0], permuted.Profiles[1] = permuted.Profiles[1], permuted.Profiles[0]
	permuted.Records[0], permuted.Records[1] = permuted.Records[1], permuted.Records[0]
	corpus, err := reg.NewFixtureConformanceCorpus(permuted)
	if err != nil {
		t.Fatalf("NewFixtureConformanceCorpus(permuted): %v", err)
	}
	if got, marshalErr := corpus.MarshalBoundedReport(); marshalErr != nil || !bytes.Equal(got, want) {
		t.Fatalf("permuted report = %q, %v; want byte-identical sorted report", got, marshalErr)
	}
	encoded, err := reg.MarshalFixtureConformanceCorpusSpec(m203SyntheticCorpus(t))
	if err != nil {
		t.Fatalf("MarshalFixtureConformanceCorpusSpec: %v", err)
	}
	restored, err := reg.UnmarshalFixtureConformanceCorpus(encoded)
	if err != nil {
		t.Fatalf("UnmarshalFixtureConformanceCorpus: %v", err)
	}
	if got, replayErr := restored.MarshalBoundedReport(); replayErr != nil || !bytes.Equal(got, want) {
		t.Fatalf("round-trip report = %q, %v; want byte-identical replay", got, replayErr)
	}
	const workers, iterations = 8, 32
	errors := make(chan error, workers*iterations)
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				got, replayErr := corpus.MarshalBoundedReport()
				if replayErr != nil {
					errors <- replayErr
					return
				}
				if !bytes.Equal(got, want) {
					errors <- reg.ErrFixtureConformanceNondeterministic
					return
				}
			}
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
}

func TestM203CorpusSpecMarshalIsCanonicalAcrossIndependentPermutations(t *testing.T) {
	baseline := m203SyntheticCorpus(t)
	baselineEncoded, err := reg.MarshalFixtureConformanceCorpusSpec(baseline)
	if err != nil {
		t.Fatalf("MarshalFixtureConformanceCorpusSpec(baseline): %v", err)
	}

	permuted := m203SyntheticCorpus(t)
	permuted.Profiles[0], permuted.Profiles[1] = permuted.Profiles[1], permuted.Profiles[0]
	permuted.Records[0], permuted.Records[1] = permuted.Records[1], permuted.Records[0]
	permutedEncoded, err := reg.MarshalFixtureConformanceCorpusSpec(permuted)
	if err != nil {
		t.Fatalf("MarshalFixtureConformanceCorpusSpec(permuted): %v", err)
	}
	if !bytes.Equal(permutedEncoded, baselineEncoded) {
		t.Fatalf("permuted corpus spec = %s; want byte-identical canonical encoding %s", permutedEncoded, baselineEncoded)
	}

	baselineCorpus, err := reg.UnmarshalFixtureConformanceCorpus(baselineEncoded)
	if err != nil {
		t.Fatalf("UnmarshalFixtureConformanceCorpus(baseline): %v", err)
	}
	permutedCorpus, err := reg.UnmarshalFixtureConformanceCorpus(permutedEncoded)
	if err != nil {
		t.Fatalf("UnmarshalFixtureConformanceCorpus(permuted): %v", err)
	}
	baselineReport, err := baselineCorpus.MarshalBoundedReport()
	if err != nil {
		t.Fatalf("MarshalBoundedReport(baseline): %v", err)
	}
	permutedReport, err := permutedCorpus.MarshalBoundedReport()
	if err != nil {
		t.Fatalf("MarshalBoundedReport(permuted): %v", err)
	}
	if !bytes.Equal(permutedReport, baselineReport) {
		t.Fatalf("permuted replay report = %s; want byte-identical canonical report %s", permutedReport, baselineReport)
	}
}

func TestM203BoundedReportCarriesConformanceEvidence(t *testing.T) {
	corpus, err := reg.NewFixtureConformanceCorpus(m203SyntheticCorpus(t))
	if err != nil {
		t.Fatalf("NewFixtureConformanceCorpus: %v", err)
	}
	encoded, err := corpus.MarshalBoundedReport()
	if err != nil {
		t.Fatalf("MarshalBoundedReport: %v", err)
	}
	var report struct {
		Records []struct {
			ProfileVersion string          `json:"profile_version"`
			Function       json.RawMessage `json:"function"`
			Table          string          `json:"table"`
			UnitID         byte            `json:"unit_id"`
			Normalized     uint16          `json:"normalized_address"`
			RawWords       [][]uint16      `json:"raw_words"`
			LogicalSlices  [][]uint16      `json:"logical_slices"`
			Wire           json.RawMessage `json:"wire"`
			Logical        json.RawMessage `json:"logical"`
			Sample         json.RawMessage `json:"sample"`
			Source         json.RawMessage `json:"source_time"`
			Qualification  string          `json:"qualification"`
			Detection      json.RawMessage `json:"detection"`
		} `json:"records"`
	}
	if err := json.Unmarshal(encoded, &report); err != nil {
		t.Fatalf("report JSON: %v", err)
	}
	if len(report.Records) == 0 || report.Records[0].ProfileVersion == "" || len(report.Records[0].Function) == 0 || report.Records[0].Table == "" || report.Records[0].UnitID == 0 || report.Records[0].Normalized == 0 || len(report.Records[0].RawWords) == 0 || len(report.Records[0].LogicalSlices) == 0 || len(report.Records[0].Wire) == 0 || len(report.Records[0].Logical) == 0 || len(report.Records[0].Sample) == 0 || len(report.Records[0].Source) == 0 || report.Records[0].Qualification == "" || len(report.Records[0].Detection) == 0 {
		t.Fatalf("report omitted conformance evidence: %#v", report.Records)
	}
	limited, err := reg.NewFixtureConformanceCorpusWithLimits(m203SyntheticCorpus(t), reg.FixtureConformanceLimits{MaxRecords: 2, MaxReportBytes: 1})
	if err != nil {
		t.Fatalf("NewFixtureConformanceCorpusWithLimits: %v", err)
	}
	if _, err := limited.MarshalBoundedReport(); !reg.IsFixtureMutationReason(err, reg.FixtureMutationReasonOversized) {
		t.Fatalf("small report limit error=%v", err)
	}
}

func TestM203CorpusActuallyReplaysConcreteExpectedOutcomes(t *testing.T) {
	spec := m203SyntheticCorpus(t)
	codec := m203SyntheticCorpus(t).Records[0]
	codec.RecordID = "replay-codec"
	detector := m203SyntheticCorpus(t).Records[0]
	detector.RecordID = "replay-detector"
	qualification := m203SyntheticCorpus(t).Records[0]
	qualification.RecordID = "replay-qualification"
	normalization := m203SyntheticCorpus(t).Records[0]
	normalization.RecordID = "replay-normalization"
	normalization.Observation.Dependencies[0].NormalizationVersion = version(t, "2.0.0")
	normalization.ExpectedReplay = reg.FixtureReplayExpectation{Outcome: reg.FixtureReplayRejected, Reason: reg.FixtureReplayReasonNormalizationMismatch}
	generationSpec := m203SyntheticCorpus(t)
	generation := generationSpec.Records[0]
	generation.RecordID = "replay-generation"
	generation.Observation.Dependencies[0].View = m203MutateView(t, &generationSpec, 0, 0, func(record *reg.LogicalViewRecord) { record.PollGeneration++ })
	generation.ExpectedReplay = reg.FixtureReplayExpectation{Outcome: reg.FixtureReplayRejected, Reason: reg.FixtureReplayReasonGenerationMismatch}
	torn := m203SyntheticCorpus(t).Records[0]
	torn.RecordID = "replay-torn-read"
	torn.Observation.Dependencies[0].Status = reg.DependencyReadTorn
	torn.ExpectedReplay = reg.FixtureReplayExpectation{Outcome: reg.FixtureReplayRejected, Reason: reg.FixtureReplayReasonTornRead}
	spec.Records = []reg.FixtureConformanceRecordSpec{codec, detector, qualification, normalization, generation, torn}
	corpus, err := reg.NewFixtureConformanceCorpus(spec)
	if err != nil {
		t.Fatalf("NewFixtureConformanceCorpus: %v", err)
	}
	report, err := corpus.Replay()
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	for _, result := range report.Records() {
		if !result.Replay().MatchesExpected() {
			t.Fatalf("%s replay actual=%#v expected=%#v", result.RecordID(), result.Replay().Actual(), result.Replay().Expected())
		}
	}
}
