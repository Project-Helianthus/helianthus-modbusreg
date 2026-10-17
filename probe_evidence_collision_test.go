package modbusreg_test

import (
	"context"
	"reflect"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestS29LateProbeEvidenceCollisionStopsDetection(t *testing.T) {
	tests := []struct {
		name        string
		declaration string
		prior       string
		wantReads   []string
	}{
		{
			name:        "second_reuses_first",
			declaration: "model-identity", prior: "manufacturer-identity",
			wantReads: []string{"manufacturer-identity", "model-identity"},
		},
		{
			name:        "third_reuses_first",
			declaration: "firmware-identity", prior: "manufacturer-identity",
			wantReads: []string{"manufacturer-identity", "model-identity", "firmware-identity"},
		},
		{
			name:        "third_reuses_second",
			declaration: "firmware-identity", prior: "model-identity",
			wantReads: []string{"manufacturer-identity", "model-identity", "firmware-identity"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			detector, profile := s29Detector(t)
			reader := detectionReader(t)
			original := reader.results[test.declaration]
			collision, err := reg.NewProbeReadResult(reg.ProbeReadResultSpec{
				Status:     reg.ProbeReadSucceeded,
				Words:      original.Words(),
				EvidenceID: reader.results[test.prior].EvidenceID(),
			})
			if err != nil {
				t.Fatalf("construct collision: %v", err)
			}
			reader.results[test.declaration] = collision
			decision, err := detector.Detect(context.Background(), reader, reg.DetectionOptions{})
			if err != nil || decision.Outcome() != reg.DetectionNoMatch ||
				decision.Reason() != reg.DetectionReasonInvalidProbeResult ||
				decision.SelectedProfileID() != "" || decision.SelectedProfileVersion() != (reg.Version{}) {
				t.Fatalf("collision: outcome=%s reason=%s selected=%q version=%v err=%v",
					decision.Outcome(), decision.Reason(), decision.SelectedProfileID(), decision.SelectedProfileVersion(), err)
			}
			if got := reader.declarationOrder(); !reflect.DeepEqual(got, test.wantReads) {
				t.Fatalf("collision reads=%v want executed prefix %v", got, test.wantReads)
			}

			// The same valid words with distinct evidence must match, even after failure.
			s29AssertMatched(t, detector, profile, detectionReader(t))
		})
	}
}

func TestS29DistinctEvidenceAllowsEqualIdentityTextAcrossInvocations(t *testing.T) {
	detector, profile := s29DetectorWithEqualIdentity(t)
	for _, invocation := range []string{"first", "second"} {
		t.Run(invocation, func(t *testing.T) {
			reader := detectionReader(t)
			reader.results["manufacturer-identity"] = mustProbeResult(t, "synthetic-family", "probe-evidence-manufacturer")
			reader.results["model-identity"] = mustProbeResult(t, "synthetic-family", "probe-evidence-model")
			// Both calls deliberately reuse the same three evidence IDs.
			s29AssertMatched(t, detector, profile, reader)
		})
	}
}

func s29Detector(t *testing.T) (*reg.ProfileDetector, reg.ProfileDescriptor) {
	t.Helper()
	profile := detectionProfile(t, "example.standard.evidence-collision", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	catalog, err := reg.NewCatalog(profile)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	return newDetector(t, catalog, detectionCandidate(t, profile, 1, true, false)), profile
}

func s29DetectorWithEqualIdentity(t *testing.T) (*reg.ProfileDetector, reg.ProfileDescriptor) {
	t.Helper()
	_, profile := s29Detector(t)
	catalog, err := reg.NewCatalog(profile)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	spec := detectionCandidate(t, profile, 1, true, false).Spec()
	spec.Manufacturer.Expected = "synthetic-family"
	spec.Model.Expected = "synthetic-family"
	candidate, err := reg.NewDetectionCandidate(spec, detectionLimits())
	if err != nil {
		t.Fatalf("NewDetectionCandidate: %v", err)
	}
	return newDetector(t, catalog, candidate), profile
}

func s29AssertMatched(t *testing.T, detector *reg.ProfileDetector, profile reg.ProfileDescriptor, reader *recordedProbeReader) {
	t.Helper()
	decision, err := detector.Detect(context.Background(), reader, reg.DetectionOptions{})
	if err != nil || decision.Outcome() != reg.DetectionMatched ||
		decision.Reason() != reg.DetectionReasonSelected ||
		decision.SelectedProfileID() != profile.ID() || decision.SelectedProfileVersion() != profile.Version() {
		t.Fatalf("distinct evidence: outcome=%s reason=%s selected=%q version=%v err=%v",
			decision.Outcome(), decision.Reason(), decision.SelectedProfileID(), decision.SelectedProfileVersion(), err)
	}
	wantReads := []string{"manufacturer-identity", "model-identity", "firmware-identity"}
	if got := reader.declarationOrder(); !reflect.DeepEqual(got, wantReads) {
		t.Fatalf("distinct evidence reads=%v want %v", got, wantReads)
	}
	evidence := decision.Evidence()
	if len(evidence) != 1 || evidence[0].Reason != reg.DetectionReasonSelected ||
		!reflect.DeepEqual(evidence[0].ProbeEvidenceIDs, []string{
			"probe-evidence-manufacturer", "probe-evidence-model", "probe-evidence-firmware",
		}) {
		t.Fatalf("distinct evidence provenance=%+v", evidence)
	}
}
