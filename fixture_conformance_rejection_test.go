package modbusreg_test

import (
	"bytes"
	"strings"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func m203MarshalCorpusSpec(t *testing.T) []byte {
	t.Helper()
	encoded, err := reg.MarshalFixtureConformanceCorpusSpec(m203SyntheticCorpus(t))
	if err != nil {
		t.Fatalf("MarshalFixtureConformanceCorpusSpec: %v", err)
	}
	return encoded
}

func TestM203CorpusStrictDecodeRejectsMutatedRawJSON(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]byte) []byte
		reason reg.FixtureMutationReason
	}{
		{"missing", func([]byte) []byte { return []byte(`{"metadata":{}}`) }, reg.FixtureMutationReasonMissing},
		{"unknown", func(encoded []byte) []byte { return append([]byte(`{"unknown":true,`), encoded[1:]...) }, reg.FixtureMutationReasonUnknown},
		{"duplicate", func(encoded []byte) []byte { return append([]byte(`{"schema_version":"1.0.0",`), encoded[1:]...) }, reg.FixtureMutationReasonDuplicate},
		{"case folded", func(encoded []byte) []byte {
			return bytes.Replace(encoded, []byte(`"schema_version"`), []byte(`"Schema_Version"`), 1)
		}, reg.FixtureMutationReasonCaseFolded},
		{"malformed", func([]byte) []byte { return []byte(`{"schema_version":`) }, reg.FixtureMutationReasonMalformed},
		{"oversized", func(encoded []byte) []byte {
			return bytes.Replace(encoded, []byte("public synthetic fixture"), []byte(strings.Repeat("x", reg.MaxContractStringBytes+1)), 1)
		}, reg.FixtureMutationReasonOversized},
		{"contradictory", func(encoded []byte) []byte {
			return bytes.Replace(encoded, []byte(`"expected":"qualified"`), []byte(`"expected":"rejected"`), 1)
		}, reg.FixtureMutationReasonContradictory},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := reg.UnmarshalFixtureConformanceCorpus(test.mutate(m203MarshalCorpusSpec(t)))
			if !reg.IsFixtureMutationReason(err, test.reason) {
				t.Fatalf("UnmarshalFixtureConformanceCorpus error = %v, want stable reason %q", err, test.reason)
			}
		})
	}
}

func TestM203CorpusRejectsUnsanitizedFixtureIdentitiesAtConstruction(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*reg.FixtureConformanceCorpusSpec)
	}{
		{"credential-like endpoint", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Endpoint = "https://reader:secret@fixture.invalid/registers"
		}},
		{"live endpoint identity", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Endpoint = "live-device-987654"
		}},
		{"live source identity", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) { record.Endpoint = "site-installation-42" })
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := m203SyntheticCorpus(t)
			test.mutate(&spec)
			if _, err := reg.NewFixtureConformanceCorpus(spec); !reg.IsFixtureMutationReason(err, reg.FixtureMutationReasonUnsanitized) {
				t.Fatalf("unsanitized %s error = %v, want unsanitized rejection", test.name, err)
			}
		})
	}
}

func TestM203IncompatibleInputsNeverCrossContaminate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*reg.FixtureConformanceCorpusSpec)
		reason reg.FixtureReplayReason
	}{
		{"unit", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) { record.UnitID++ })
		}, reg.FixtureReplayReasonUnitMismatch},
		{"table access", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) {
				record.RequestedFunction = reg.FunctionReadHoldingRegisters
				record.ReceivedFunction = reg.FunctionReadHoldingRegisters
				record.Table = reg.HoldingRegisters
			})
		}, reg.FixtureReplayReasonTableAccessMismatch},
		{"generation", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) { record.PollGeneration++ })
		}, reg.FixtureReplayReasonGenerationMismatch},
		{"source", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].SourceTime = reg.SourceTimeObserved(spec.Records[0].Observation.SourceTime.Time)
		}, reg.FixtureReplayReasonSourceMismatch},
		{"normalization", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].NormalizationVersion = version(t, "2.0.0")
		}, reg.FixtureReplayReasonNormalizationMismatch},
		{"deadline", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) { record.DeadlineIdentity++ })
		}, reg.FixtureReplayReasonDeadlineMismatch},
		{"coherence", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].DocumentaryConsistencyMarker = "other-coherence"
		}, reg.FixtureReplayReasonCoherenceMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := m203SyntheticCorpus(t)
			test.mutate(&spec)
			spec.Records[0].ExpectedReplay = reg.FixtureReplayExpectation{Outcome: reg.FixtureReplayRejected, Reason: test.reason}
			corpus, err := reg.NewFixtureConformanceCorpus(spec)
			if err != nil {
				t.Fatalf("NewFixtureConformanceCorpus(%s): %v", test.name, err)
			}
			report, err := corpus.Replay()
			if err != nil {
				t.Fatalf("Replay(%s): %v", test.name, err)
			}
			negative, unaffected := report.Records()[1], report.Records()[0]
			actual := negative.Replay().Actual()
			if !negative.Replay().MatchesExpected() || actual.Outcome() != reg.FixtureReplayRejected || actual.Reason() != test.reason {
				t.Fatalf("%s actual=%#v expected=%#v", test.name, actual, negative.Replay().Expected())
			}
			if !unaffected.Replay().MatchesExpected() || unaffected.Replay().Actual().Outcome() != reg.FixtureReplayAccepted {
				t.Fatalf("%s cross-contaminated unaffected record: %#v", test.name, unaffected)
			}
		})
	}
}

func TestM203AcceptedExpectationRejectsIncompatibleOrTornFactsAtConstruction(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*reg.FixtureConformanceCorpusSpec)
	}{
		{"unit", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) { record.UnitID++ })
		}},
		{"table access", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) {
				record.RequestedFunction = reg.FunctionReadHoldingRegisters
				record.ReceivedFunction = reg.FunctionReadHoldingRegisters
				record.Table = reg.HoldingRegisters
			})
		}},
		{"generation", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) { record.PollGeneration++ })
		}},
		{"source", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].SourceTime = reg.SourceTimeObserved(spec.Records[0].Observation.SourceTime.Time)
		}},
		{"normalization", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].NormalizationVersion = version(t, "2.0.0")
		}},
		{"deadline", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].View = m203MutateView(t, spec, 0, 1, func(record *reg.LogicalViewRecord) { record.DeadlineIdentity++ })
		}},
		{"coherence", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].DocumentaryConsistencyMarker = "other-coherence"
		}},
		{"torn", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[1].Status = reg.DependencyReadTorn
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := m203SyntheticCorpus(t)
			test.mutate(&spec)
			if _, err := reg.NewFixtureConformanceCorpus(spec); !reg.IsFixtureMutationReason(err, reg.FixtureMutationReasonContradictory) {
				t.Fatalf("accepted %s error = %v, want contradictory", test.name, err)
			}
		})
	}
}

func TestM203ConstructorRejectsSchemaProbeAndExpectationContradictions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*reg.FixtureConformanceCorpusSpec)
		reason reg.FixtureMutationReason
	}{
		{"schema", func(spec *reg.FixtureConformanceCorpusSpec) { spec.SchemaVersion = version(t, "2.0.0") }, reg.FixtureMutationReasonMalformed},
		{"wrong rejection reason", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Observation.Dependencies[0].Status = reg.DependencyReadTorn
			spec.Records[0].ExpectedReplay = reg.FixtureReplayExpectation{Outcome: reg.FixtureReplayRejected, Reason: reg.FixtureReplayReasonGenerationMismatch}
		}, reg.FixtureMutationReasonContradictory},
		{"wrong accepted words", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].ExpectedReplay.ExpectedRawWords[0][0]++
		}, reg.FixtureMutationReasonContradictory},
		{"extra probe", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Detection.Input.Probes = append(spec.Records[0].Detection.Input.Probes, reg.FixtureProbeInput{DeclarationID: "extra", Result: m203ProbeResultSpec("extra", "extra-evidence")})
		}, reg.FixtureMutationReasonMalformed},
		{"detector expectation", func(spec *reg.FixtureConformanceCorpusSpec) {
			spec.Records[0].Detection.Expected.Outcome = reg.DetectionNoMatch
		}, reg.FixtureMutationReasonContradictory},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := m203SyntheticCorpus(t)
			test.mutate(&spec)
			if _, err := reg.NewFixtureConformanceCorpus(spec); !reg.IsFixtureMutationReason(err, test.reason) {
				t.Fatalf("%s error=%v", test.name, err)
			}
		})
	}
}

func TestM203StrictDecodeRejectsUnknownRecordField(t *testing.T) {
	encoded := m203MarshalCorpusSpec(t)
	mutated := bytes.Replace(
		encoded,
		[]byte(`"record_id"`),
		[]byte(`"unexpected":true,"record_id"`),
		1,
	)
	if _, err := reg.UnmarshalFixtureConformanceCorpus(mutated); !reg.IsFixtureMutationReason(err, reg.FixtureMutationReasonMalformed) {
		t.Fatalf("unknown record field error=%v", err)
	}
}

func TestM203CorpusPreflightsAbsoluteArrayCardinality(t *testing.T) {
	items := strings.Repeat("null,", reg.MaxProfileDependencies) + "null"
	payload := []byte(`{"schema_version":"1.0.0","metadata":{"corpus_id":"fixture-bomb","license_expression":"CC0-1.0","provenance":"public synthetic fixture"},"profiles":[` + items + `],"records":[null]}`)
	if len(payload) >= reg.MaxSerializedContractBytes {
		t.Fatalf("cardinality fixture exceeds byte limit: %d", len(payload))
	}
	if _, err := reg.UnmarshalFixtureConformanceCorpus(payload); !reg.IsFixtureMutationReason(err, reg.FixtureMutationReasonOversized) {
		t.Fatalf("cardinality preflight error = %v", err)
	}
}

func m203MutateView(t *testing.T, spec *reg.FixtureConformanceCorpusSpec, recordIndex, dependencyIndex int, mutate func(*reg.LogicalViewRecord)) reg.LogicalViewSnapshot {
	t.Helper()
	record := spec.Records[recordIndex].Observation.Dependencies[dependencyIndex].View.Record()
	mutate(&record)
	return snapshotFromRecord(t, record)
}
