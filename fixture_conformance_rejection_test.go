package modbusreg_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/detection"
)

func m203MarshalCorpusSpec(t *testing.T) []byte {
	t.Helper()
	encoded, err := detection.MarshalFixtureConformanceCorpusSpec(m203SyntheticCorpus(t))
	if err != nil {
		t.Fatalf("MarshalFixtureConformanceCorpusSpec: %v", err)
	}
	return encoded
}

func TestM203CorpusStrictDecodeRejectsMutatedRawJSON(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]byte) []byte
		reason detection.FixtureMutationReason
	}{
		{"missing", func([]byte) []byte { return []byte(`{"metadata":{}}`) }, detection.FixtureMutationReasonMissing},
		{"unknown", func(encoded []byte) []byte { return append([]byte(`{"unknown":true,`), encoded[1:]...) }, detection.FixtureMutationReasonUnknown},
		{"duplicate", func(encoded []byte) []byte { return append([]byte(`{"schema_version":"1.0.0",`), encoded[1:]...) }, detection.FixtureMutationReasonDuplicate},
		{"case folded", func(encoded []byte) []byte {
			return bytes.Replace(encoded, []byte(`"schema_version"`), []byte(`"Schema_Version"`), 1)
		}, detection.FixtureMutationReasonCaseFolded},
		{"malformed", func([]byte) []byte { return []byte(`{"schema_version":`) }, detection.FixtureMutationReasonMalformed},
		{"oversized", func(encoded []byte) []byte {
			return bytes.Replace(encoded, []byte("public synthetic fixture"), []byte(strings.Repeat("x", reg.MaxContractStringBytes+1)), 1)
		}, detection.FixtureMutationReasonOversized},
		{"contradictory", func(encoded []byte) []byte {
			return bytes.Replace(encoded, []byte(`"expected":"qualified"`), []byte(`"expected":"rejected"`), 1)
		}, detection.FixtureMutationReasonContradictory},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := detection.UnmarshalFixtureConformanceCorpus(test.mutate(m203MarshalCorpusSpec(t)))
			if !detection.IsFixtureMutationReason(err, test.reason) {
				t.Fatalf("UnmarshalFixtureConformanceCorpus error = %v, want stable reason %q", err, test.reason)
			}
		})
	}
}

func TestM203CorpusRejectsUnsanitizedFixtureIdentitiesAtConstruction(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*detection.FixtureConformanceCorpusSpec)
	}{
		{"credential-like endpoint", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Endpoint = "https://reader:secret@fixture.invalid/registers"
		}},
		{"live endpoint identity", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Endpoint = "live-device-987654"
		}},
		{"live source identity", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) { record.Endpoint = "site-installation-42" })
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := m203SyntheticCorpus(t)
			test.mutate(&spec)
			if _, err := detection.NewFixtureConformanceCorpus(spec); !detection.IsFixtureMutationReason(err, detection.FixtureMutationReasonUnsanitized) {
				t.Fatalf("unsanitized %s error = %v, want unsanitized rejection", test.name, err)
			}
		})
	}
}

func TestM203IncompatibleInputsNeverCrossContaminate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*detection.FixtureConformanceCorpusSpec)
		reason detection.FixtureReplayReason
	}{
		{"unit", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) { record.UnitID++ })
		}, detection.FixtureReplayReasonUnitMismatch},
		{"table access", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) {
				record.RequestedFunction = reg.FunctionReadHoldingRegisters
				record.ReceivedFunction = reg.FunctionReadHoldingRegisters
				record.Table = reg.HoldingRegisters
			})
		}, detection.FixtureReplayReasonTableAccessMismatch},
		{"generation", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) { record.PollGeneration++ })
		}, detection.FixtureReplayReasonGenerationMismatch},
		{"source", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].SourceTime = reg.SourceTimeObserved(spec.Records[0].Observation.SourceTime.Time)
		}, detection.FixtureReplayReasonSourceMismatch},
		{"normalization", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].NormalizationVersion = version(t, "2.0.0")
		}, detection.FixtureReplayReasonNormalizationMismatch},
		{"deadline", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) { record.DeadlineIdentity++ })
		}, detection.FixtureReplayReasonDeadlineMismatch},
		{"coherence", func(spec *detection.FixtureConformanceCorpusSpec) {
			m203RequireDocumentaryMarkerPolicy(t, spec)
			spec.Records[0].Observation.Dependencies[1].DocumentaryConsistencyMarker = "other-coherence"
		}, detection.FixtureReplayReasonCoherenceMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := m203SyntheticCorpus(t)
			test.mutate(&spec)
			spec.Records[0].ExpectedReplay = detection.FixtureReplayExpectation{Outcome: detection.FixtureReplayRejected, Reason: test.reason}
			corpus, err := detection.NewFixtureConformanceCorpus(spec)
			if err != nil {
				t.Fatalf("NewFixtureConformanceCorpus(%s): %v", test.name, err)
			}
			report, err := corpus.Replay()
			if err != nil {
				t.Fatalf("Replay(%s): %v", test.name, err)
			}
			negative, unaffected := report.Records()[1], report.Records()[0]
			actual := negative.Replay().Actual()
			if !negative.Replay().MatchesExpected() || actual.Outcome() != detection.FixtureReplayRejected || actual.Reason() != test.reason {
				t.Fatalf("%s actual=%#v expected=%#v", test.name, actual, negative.Replay().Expected())
			}
			if !unaffected.Replay().MatchesExpected() || unaffected.Replay().Actual().Outcome() != detection.FixtureReplayAccepted {
				t.Fatalf("%s cross-contaminated unaffected record: %#v", test.name, unaffected)
			}
		})
	}
}

func TestM203AcceptedExpectationRejectsIncompatibleOrTornFactsAtConstruction(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*detection.FixtureConformanceCorpusSpec)
	}{
		{"unit", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) { record.UnitID++ })
		}},
		{"table access", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) {
				record.RequestedFunction = reg.FunctionReadHoldingRegisters
				record.ReceivedFunction = reg.FunctionReadHoldingRegisters
				record.Table = reg.HoldingRegisters
			})
		}},
		{"generation", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) { record.PollGeneration++ })
		}},
		{"source", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].SourceTime = reg.SourceTimeObserved(spec.Records[0].Observation.SourceTime.Time)
		}},
		{"normalization", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].NormalizationVersion = version(t, "2.0.0")
		}},
		{"deadline", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) { record.DeadlineIdentity++ })
		}},
		{"coherence", func(spec *detection.FixtureConformanceCorpusSpec) {
			m203RequireDocumentaryMarkerPolicy(t, spec)
			spec.Records[0].Observation.Dependencies[1].DocumentaryConsistencyMarker = "other-coherence"
		}},
		{"torn", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].Status = reg.DependencyReadTorn
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := m203SyntheticCorpus(t)
			test.mutate(&spec)
			if _, err := detection.NewFixtureConformanceCorpus(spec); !detection.IsFixtureMutationReason(err, detection.FixtureMutationReasonContradictory) {
				t.Fatalf("accepted %s error = %v, want contradictory", test.name, err)
			}
		})
	}
}

func TestM203ConstructorRejectsSchemaProbeAndExpectationContradictions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*detection.FixtureConformanceCorpusSpec)
		reason detection.FixtureMutationReason
	}{
		{"schema", func(spec *detection.FixtureConformanceCorpusSpec) { spec.SchemaVersion = version(t, "2.0.0") }, detection.FixtureMutationReasonMalformed},
		{"wrong rejection reason", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[0].Status = reg.DependencyReadTorn
			spec.Records[0].ExpectedReplay = detection.FixtureReplayExpectation{Outcome: detection.FixtureReplayRejected, Reason: detection.FixtureReplayReasonGenerationMismatch}
		}, detection.FixtureMutationReasonContradictory},
		{"wrong accepted words", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].ExpectedReplay.ExpectedRawWords[0][0]++
		}, detection.FixtureMutationReasonContradictory},
		{"extra probe", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Detection.Input.Probes = append(spec.Records[0].Detection.Input.Probes, detection.FixtureProbeInput{DeclarationID: "extra", Result: m203ProbeResultSpec("extra", "extra-evidence")})
		}, detection.FixtureMutationReasonMalformed},
		{"detector expectation", func(spec *detection.FixtureConformanceCorpusSpec) {
			spec.Records[0].Detection.Expected.Outcome = detection.NoMatch
		}, detection.FixtureMutationReasonContradictory},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := m203SyntheticCorpus(t)
			test.mutate(&spec)
			if _, err := detection.NewFixtureConformanceCorpus(spec); !detection.IsFixtureMutationReason(err, test.reason) {
				t.Fatalf("%s error=%v", test.name, err)
			}
		})
	}
}

func TestM203StrictDecodeClassifiesRecordAndNestedFixtureFields(t *testing.T) {
	encoded := m203MarshalCorpusSpec(t)
	tests := []struct {
		name   string
		mutate func([]byte) []byte
		reason detection.FixtureMutationReason
	}{
		{"record unknown", func(data []byte) []byte {
			return bytes.Replace(data, []byte(`"record_id"`), []byte(`"unexpected":true,"record_id"`), 1)
		}, detection.FixtureMutationReasonUnknown},
		{"record duplicate", func(data []byte) []byte {
			return bytes.Replace(data, []byte(`"record_id"`), []byte(`"record_id":"duplicate","record_id"`), 1)
		}, detection.FixtureMutationReasonDuplicate},
		{"record case folded", func(data []byte) []byte { return bytes.Replace(data, []byte(`"record_id"`), []byte(`"Record_ID"`), 1) }, detection.FixtureMutationReasonCaseFolded},
		{"record missing", func(data []byte) []byte {
			return bytes.Replace(data, []byte(`"record_id":"synthetic-fc04-alpha",`), nil, 1)
		}, detection.FixtureMutationReasonMissing},
		{"nested unknown", func(data []byte) []byte {
			return bytes.Replace(data, []byte(`"qualification":{"expected"`), []byte(`"qualification":{"unexpected":true,"expected"`), 1)
		}, detection.FixtureMutationReasonUnknown},
		{"nested duplicate", func(data []byte) []byte {
			return bytes.Replace(data, []byte(`"qualification":{"expected"`), []byte(`"qualification":{"expected":"qualified","expected"`), 1)
		}, detection.FixtureMutationReasonDuplicate},
		{"nested case folded", func(data []byte) []byte {
			return bytes.Replace(data, []byte(`"qualification":{"expected"`), []byte(`"qualification":{"Expected"`), 1)
		}, detection.FixtureMutationReasonCaseFolded},
		{"nested missing", func(data []byte) []byte {
			return bytes.Replace(data, []byte(`"qualification":{"expected":"qualified"}`), []byte(`"qualification":{}`), 1)
		}, detection.FixtureMutationReasonMissing},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := detection.UnmarshalFixtureConformanceCorpus(test.mutate(encoded)); !detection.IsFixtureMutationReason(err, test.reason) {
				t.Fatalf("error=%v, want stable reason %q", err, test.reason)
			}
		})
	}
}

func TestM203CorpusPreflightsAbsoluteArrayCardinality(t *testing.T) {
	items := strings.Repeat("null,", reg.MaxProfileDependencies) + "null"
	payload := []byte(`{"schema_version":"1.0.0","metadata":{"corpus_id":"fixture-bomb","license_expression":"CC0-1.0","provenance":"public synthetic fixture"},"profiles":[` + items + `],"records":[null]}`)
	if len(payload) >= reg.MaxSerializedContractBytes {
		t.Fatalf("cardinality fixture exceeds byte limit: %d", len(payload))
	}
	if _, err := detection.UnmarshalFixtureConformanceCorpus(payload); !detection.IsFixtureMutationReason(err, detection.FixtureMutationReasonOversized) {
		t.Fatalf("cardinality preflight error = %v", err)
	}
}

func TestM203RejectedCorpusRoundTripsWithoutExpectedRawWords(t *testing.T) {
	spec := m203SyntheticCorpus(t)
	spec.Records[0].Observation.Dependencies[0].Status = reg.DependencyReadTorn
	spec.Records[0].ExpectedReplay = detection.FixtureReplayExpectation{
		Outcome: detection.FixtureReplayRejected,
		Reason:  detection.FixtureReplayReasonTornRead,
	}
	corpus, err := detection.NewFixtureConformanceCorpus(spec)
	if err != nil {
		t.Fatalf("NewFixtureConformanceCorpus: %v", err)
	}
	want, err := corpus.MarshalBoundedReport()
	if err != nil {
		t.Fatalf("MarshalBoundedReport: %v", err)
	}
	encoded, err := detection.MarshalFixtureConformanceCorpusSpec(spec)
	if err != nil {
		t.Fatalf("MarshalFixtureConformanceCorpusSpec: %v", err)
	}
	if bytes.Contains(encoded, []byte(`"expected_replay":{"outcome":"rejected","reason":"torn_read","expected_raw_words"`)) {
		t.Fatalf("rejected corpus unexpectedly serialized raw words: %s", encoded)
	}
	restored, err := detection.UnmarshalFixtureConformanceCorpus(encoded)
	if err != nil {
		t.Fatalf("UnmarshalFixtureConformanceCorpus: %v", err)
	}
	if got, replayErr := restored.MarshalBoundedReport(); replayErr != nil || !bytes.Equal(got, want) {
		t.Fatalf("rejected round-trip report = %q, %v; want deterministic replay", got, replayErr)
	}
}

func TestM203BoundedMultiBlankPolicyAllowsDifferentDocumentaryMarkers(t *testing.T) {
	spec := m203SyntheticCorpus(t)
	profileSpec := spec.Profiles[0].Spec()
	profileSpec.Coherence = reg.CoherencePolicySpec{
		Version:                   profileSpec.Coherence.Version,
		Mode:                      reg.CoherenceBoundedMultiResponse,
		MaximumSourceSkew:         time.Second,
		MaximumReceiptSkew:        time.Second,
		RequireGenerationEquality: true,
		AcquisitionOrder:          reg.AcquisitionOrderDependencyDeclaration,
		RetrySetBehavior:          reg.RetryWholeSet,
	}
	profile, err := reg.NewProfileDescriptor(profileSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor: %v", err)
	}
	record := m203Record(t, "fixture-blank-policy-markers", profile, 23, 503)
	record.Observation.RetryOrdinal = 1
	for index := range record.Observation.Dependencies {
		dependency := &record.Observation.Dependencies[index]
		view := dependency.View.Record()
		view.WireResponseID += uint64(index)
		view.PhysicalRequestID += uint64(index)
		dependency.View = snapshotFromRecord(t, view)
		dependency.AcquisitionOrdinal = uint32(index + 1)
		dependency.RetryOrdinal = record.Observation.RetryOrdinal
		dependency.DocumentaryConsistencyMarker = "marker-" + string(rune('a'+index))
		dependency.SourceTime = reg.SourceTimeObserved(record.Observation.SourceTime.Time.Add(time.Duration(index) * time.Millisecond))
		dependency.LocalReceiptTime = record.Observation.LocalReceiptTime.Add(time.Duration(index) * time.Millisecond)
	}
	record.Observation.LocalReceiptTime = record.Observation.Dependencies[len(record.Observation.Dependencies)-1].LocalReceiptTime
	record.Observation.SourceTime = record.Observation.Dependencies[len(record.Observation.Dependencies)-1].SourceTime
	spec.Profiles[0] = profile
	spec.Records = []detection.FixtureConformanceRecordSpec{record}
	corpus, err := detection.NewFixtureConformanceCorpus(spec)
	if err != nil {
		t.Fatalf("NewFixtureConformanceCorpus: %v", err)
	}
	report, err := corpus.Replay()
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if got := report.Records()[0].Replay().Actual(); got.Outcome() != detection.FixtureReplayAccepted || got.Reason() != detection.FixtureReplayReasonAccepted {
		t.Fatalf("blank-policy markers replay = %#v", got)
	}
}

func TestM203CorpusPreflightsNestedExpectedRawWordCardinality(t *testing.T) {
	encoded := m203MarshalCorpusSpec(t)
	wordSets := make([][]uint16, reg.MaxProfileDependencies+1)
	for index := range wordSets {
		wordSets[index] = []uint16{}
	}
	words, err := json.Marshal(wordSets)
	if err != nil {
		t.Fatalf("json.Marshal nested words: %v", err)
	}
	payload := bytes.Replace(encoded, []byte(`"expected_raw_words":[[258,772],[772,1286]]`), append([]byte(`"expected_raw_words":`), words...), 1)
	if bytes.Equal(payload, encoded) {
		t.Fatal("nested raw-word fixture did not mutate corpus")
	}
	if len(payload) >= reg.MaxSerializedContractBytes {
		t.Fatalf("nested cardinality fixture exceeds byte limit: %d", len(payload))
	}
	if _, err := detection.UnmarshalFixtureConformanceCorpus(payload); !detection.IsFixtureMutationReason(err, detection.FixtureMutationReasonOversized) {
		t.Fatalf("nested cardinality preflight error = %v", err)
	}
}

func m203RequireDocumentaryMarkerPolicy(t *testing.T, spec *detection.FixtureConformanceCorpusSpec) {
	t.Helper()
	profileSpec := spec.Profiles[0].Spec()
	profileSpec.Coherence = reg.CoherencePolicySpec{
		Version:                      profileSpec.Coherence.Version,
		Mode:                         reg.CoherenceBoundedMultiResponse,
		MaximumSourceSkew:            time.Second,
		MaximumReceiptSkew:           time.Second,
		RequireGenerationEquality:    true,
		AcquisitionOrder:             reg.AcquisitionOrderDependencyDeclaration,
		RetrySetBehavior:             reg.RetryWholeSet,
		DocumentaryConsistencyMarker: "required-marker",
	}
	profile, err := reg.NewProfileDescriptor(profileSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor: %v", err)
	}
	spec.Profiles[0] = profile
	observation := &spec.Records[0].Observation
	observation.RetryOrdinal = 1
	for index := range observation.Dependencies {
		dependency := &observation.Dependencies[index]
		view := dependency.View.Record()
		view.WireResponseID += uint64(index)
		view.PhysicalRequestID += uint64(index)
		dependency.View = snapshotFromRecord(t, view)
		dependency.AcquisitionOrdinal = uint32(index + 1)
		dependency.RetryOrdinal = observation.RetryOrdinal
		dependency.DocumentaryConsistencyMarker = "required-marker"
		dependency.SourceTime = reg.SourceTimeObserved(observation.SourceTime.Time.Add(time.Duration(index) * time.Millisecond))
		dependency.LocalReceiptTime = observation.LocalReceiptTime.Add(time.Duration(index) * time.Millisecond)
	}
	last := observation.Dependencies[len(observation.Dependencies)-1]
	observation.SourceTime = last.SourceTime
	observation.LocalReceiptTime = last.LocalReceiptTime
}

func m203MutateView(t *testing.T, spec *detection.FixtureConformanceCorpusSpec, recordIndex, dependencyIndex int, mutate func(*reg.LogicalViewRecord)) reg.LogicalViewSnapshot {
	t.Helper()
	record := spec.Records[recordIndex].Observation.Dependencies[dependencyIndex].View.Record()
	mutate(&record)
	return snapshotFromRecord(t, record)
}
