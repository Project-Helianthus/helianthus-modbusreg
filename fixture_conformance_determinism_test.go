package modbusreg_test

import (
	"bytes"
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

func TestM203CorpusActuallyReplaysConcreteExpectedOutcomes(t *testing.T) {
	spec := m203SyntheticCorpus(t)
	valid := spec.Records[0]
	codec := valid
	codec.RecordID = "replay-codec"
	detector := valid
	detector.RecordID = "replay-detector"
	qualification := valid
	qualification.RecordID = "replay-qualification"
	normalization := valid
	normalization.RecordID = "replay-normalization"
	normalization.Observation.Dependencies[0].NormalizationVersion = version(t, "2.0.0")
	normalization.ExpectedReplay = reg.FixtureReplayExpectation{Outcome: reg.FixtureReplayRejected, Reason: reg.FixtureReplayReasonNormalizationMismatch}
	generation := valid
	generation.RecordID = "replay-generation"
	generation.Observation.Dependencies[0].View = m203MutateView(t, &spec, 0, 0, func(record *reg.LogicalViewRecord) { record.PollGeneration++ })
	generation.ExpectedReplay = reg.FixtureReplayExpectation{Outcome: reg.FixtureReplayRejected, Reason: reg.FixtureReplayReasonGenerationMismatch}
	torn := valid
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
