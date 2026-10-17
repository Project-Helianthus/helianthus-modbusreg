package modbusreg

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

func TestDetectionReasonPrecedencePairs(t *testing.T) {
	// Literal contract oracle, independent of the production priority table.
	priorities := []DetectionReason{
		"invalid_identity", "identity_mismatch", "firmware_mismatch",
		"fixture_opt_in_required", "profile_revoked", "profile_superseded",
		"profile_unqualified", "profile_default_off", "candidate_disabled",
	}
	for higher, want := range priorities {
		for _, lower := range priorities[higher+1:] {
			for _, reverse := range []bool{false, true} {
				for _, scores := range [][2]uint32{{1, 99}, {99, 1}} {
					t.Run(fmt.Sprintf("%s_over_%s/reverse_%t/scores_%d_%d", want, lower, reverse, scores[0], scores[1]), func(t *testing.T) {
						evidence := []DetectionCandidateEvidence{
							{ProfileID: "synthetic.family.alpha", Score: scores[0], Reason: want},
							{ProfileID: "synthetic.family.beta", Score: scores[1], Reason: lower},
						}
						if reverse {
							evidence[0], evidence[1] = evidence[1], evidence[0]
						}
						if got := noMatchReason(evidence); got != want {
							t.Fatalf("reason=%q, want %q", got, want)
						}
					})
				}
			}
		}
	}
	t.Run("all_nine_reversed", func(t *testing.T) {
		evidence := make([]DetectionCandidateEvidence, len(priorities))
		for i := range priorities {
			evidence[i] = DetectionCandidateEvidence{Reason: priorities[len(priorities)-1-i], Score: uint32(len(priorities) - i)}
		}
		if got := noMatchReason(evidence); got != "invalid_identity" {
			t.Fatalf("reason=%q, want invalid_identity", got)
		}
	})
}

func TestDetectionReasonPrecedenceFallback(t *testing.T) {
	cases := []struct {
		name    string
		reasons []DetectionReason
		want    DetectionReason
	}{
		{"nil", nil, "invalid_identity"},
		{"empty", []DetectionReason{}, "invalid_identity"},
		{"empty_reason", []DetectionReason{""}, "invalid_identity"},
		{"unknown", []DetectionReason{"synthetic_unknown"}, "invalid_identity"},
		{"unknown_and_empty", []DetectionReason{"synthetic_unknown", ""}, "invalid_identity"},
		{"unknown_before_known", []DetectionReason{"synthetic_unknown", "candidate_disabled"}, "candidate_disabled"},
		{"unknown_after_known", []DetectionReason{"candidate_disabled", "synthetic_unknown"}, "candidate_disabled"},
		{"empty_before_known", []DetectionReason{"", "candidate_disabled"}, "candidate_disabled"},
		{"empty_after_known", []DetectionReason{"candidate_disabled", ""}, "candidate_disabled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var evidence []DetectionCandidateEvidence
			for _, reason := range tc.reasons {
				evidence = append(evidence, DetectionCandidateEvidence{Reason: reason})
			}
			if tc.reasons != nil && len(tc.reasons) == 0 {
				evidence = []DetectionCandidateEvidence{}
			}
			if got := noMatchReason(evidence); got != tc.want {
				t.Fatalf("reason=%q, want %q", got, tc.want)
			}
		})
	}
}

// All declarations and probe responses below are synthetic, with no device I/O.
func precedenceProfile(t *testing.T, id string) ProfileDescriptor {
	t.Helper()
	v := MustParseVersion("1.0.0")
	codec, err := NewCodec(CodecSpec{
		ID: "synthetic-value", Version: v, RawWordCount: 1, WordPermutation: []uint16{0},
		IntraWordByteOrder: ByteOrderModbus, Representation: RepresentationUnsignedInteger,
		Scale:  ScaleSpec{Source: ScaleConstant, ApplicationOrder: ScaleAfterRepresentation, Numerator: 1, Denominator: 1},
		String: StringSpec{Applicability: StringNotApplicable}, OutputProfileType: "integer", ValidityBehavior: ValidityRejectSentinel,
	})
	if err != nil {
		t.Fatal(err)
	}
	dependency, err := NewDependency(DependencySpec{
		ID: "synthetic-value", Version: v, Table: InputRegisters,
		Normalization: AddressNormalizationSpec{Version: v, SourceLocator: "urn:helianthus:evidence:synthetic-precedence", DocumentaryNotation: "zero-based input register", DocumentaryBase: AddressBaseZeroBased, AddressSpaceLabel: "input_registers", Transformation: TransformIdentity},
		WordCount:     1, CodecID: codec.ID(), CodecVersion: v, CoherenceGroup: "synthetic", EvidenceReferences: []string{"synthetic-precedence"}, ApplicabilityRefs: []string{"synthetic-family"},
	})
	if err != nil {
		t.Fatal(err)
	}
	dependencies, err := NewDependencySet(v, []Dependency{dependency})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := NewProfileDescriptor(ProfileDescriptorSpec{
		SchemaVersion: v, ID: id, Version: v, Kind: ProfileStandardFamily,
		StandardApplicability: []string{"synthetic-family"}, ModelApplicability: []string{}, KnownExclusions: []string{},
		RuntimeContractVersion: v, DetectorVersion: v, CodecContractVersion: v, NormalizationVersion: v, CoherenceVersion: v, QualificationVersion: v,
		Codecs: []Codec{codec}, Dependencies: dependencies,
		Coherence: CoherencePolicySpec{Version: v, Mode: CoherenceSingleWireResponse, AcquisitionOrder: AcquisitionOrderNotApplicable, RetrySetBehavior: RetrySetNotApplicable},
		Evidence:  []EvidenceReference{{ID: "synthetic-precedence", PublicationDisposition: PublicationMetadataOnly}},
		Maturity:  MaturityQualified, DefaultEnabled: true, State: ProfileActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

type precedenceReader struct{}

func (precedenceReader) ReadProbe(_ context.Context, request ProbeReadRequest) (ProbeReadResult, error) {
	values := map[string]string{"manufacturer": "synthetic-maker", "family": "synthetic-family", "firmware": "1.5.0"}
	value := []byte(values[request.DeclarationID()])
	words := make([]uint16, 16)
	for i, b := range value {
		if i%2 == 0 {
			words[i/2] |= uint16(b) << 8
		} else {
			words[i/2] |= uint16(b)
		}
	}
	return NewProbeReadResult(ProbeReadResultSpec{Status: ProbeReadSucceeded, Words: words, EvidenceID: "synthetic-" + request.DeclarationID()})
}

func TestDetectionReasonPrecedencePublicDetector(t *testing.T) {
	v := MustParseVersion("1.0.0")
	limits := DefaultDetectionLimits()
	plan, err := NewProbePlan(ProbePlanSpec{Version: v, Declarations: []ProbeDeclarationSpec{
		{ID: "manufacturer", Function: ProbeReadHoldingRegisters, Address: 0, WordCount: 16, IdentityField: ProbeIdentityManufacturer, Encoding: ProbeIdentityASCII},
		{ID: "family", Function: ProbeReadHoldingRegisters, Address: 16, WordCount: 16, IdentityField: ProbeIdentityModel, Encoding: ProbeIdentityASCII},
		{ID: "firmware", Function: ProbeReadHoldingRegisters, Address: 32, WordCount: 16, IdentityField: ProbeIdentityFirmware, Encoding: ProbeIdentityASCII},
	}}, limits)
	if err != nil {
		t.Fatal(err)
	}
	for _, firmwareGate := range []string{"minimum", "maximum"} {
		for _, identityGate := range []string{"manufacturer", "family"} {
			for _, identityFirst := range []bool{false, true} {
				for _, reverse := range []bool{false, true} {
					for _, scores := range [][2]uint32{{1, 99}, {99, 1}} {
						t.Run(fmt.Sprintf("firmware_%s/%s/identity_first_%t/reverse_%t/scores_%d_%d", firmwareGate, identityGate, identityFirst, reverse, scores[0], scores[1]), func(t *testing.T) {
							profiles := []ProfileDescriptor{precedenceProfile(t, "synthetic.family.alpha"), precedenceProfile(t, "synthetic.family.beta")}
							identityIndex := 1
							if identityFirst {
								identityIndex = 0
							}
							candidates := make([]DetectionCandidate, 2)
							for i, profile := range profiles {
								spec := DetectionCandidateSpec{ProfileID: profile.ID(), ProfileVersion: v, Score: scores[i], Enabled: true,
									Manufacturer: IdentityStringGateSpec{Expected: "synthetic-maker"}, Model: IdentityStringGateSpec{Expected: "synthetic-family"},
									Firmware: FirmwareGateSpec{MinimumInclusive: MustParseVersion("1.0.0"), MaximumExclusive: MustParseVersion("2.0.0")},
								}
								if i == identityIndex {
									if identityGate == "manufacturer" {
										spec.Manufacturer.Expected = "other-maker"
									} else {
										spec.Model.Expected = "other-family"
									}
								} else {
									if firmwareGate == "minimum" {
										spec.Firmware.MinimumInclusive = MustParseVersion("1.6.0")
									} else {
										spec.Firmware.MaximumExclusive = MustParseVersion("1.5.0")
									}
								}
								candidates[i], err = NewDetectionCandidate(spec, limits)
								if err != nil {
									t.Fatal(err)
								}
							}
							if reverse {
								profiles[0], profiles[1] = profiles[1], profiles[0]
								candidates[0], candidates[1] = candidates[1], candidates[0]
							}
							catalog, err := NewCatalog(profiles...)
							if err != nil {
								t.Fatal(err)
							}
							detector, err := NewProfileDetector(ProfileDetectorSpec{DetectorVersion: v, Plan: plan, Catalog: catalog, Candidates: candidates, Limits: limits})
							if err != nil {
								t.Fatal(err)
							}
							decision, err := detector.Detect(context.Background(), precedenceReader{}, DetectionOptions{})
							if err != nil {
								t.Fatal(err)
							}
							if decision.Outcome() != DetectionNoMatch || decision.Reason() != "identity_mismatch" || decision.SelectedProfileID() != "" || decision.SelectedProfileVersion() != (Version{}) {
								t.Fatalf("unexpected no-match decision: %+v", decision)
							}
							evidence := decision.Evidence()
							if len(evidence) != 2 {
								t.Fatalf("evidence=%+v", evidence)
							}
							for i, item := range evidence {
								wantReason := DetectionReason("firmware_mismatch")
								wantGates := []ProbeIdentityField{ProbeIdentityManufacturer, ProbeIdentityModel}
								wantIDs := []string{"synthetic-manufacturer", "synthetic-family"}
								if i == identityIndex {
									wantReason = "identity_mismatch"
									wantGates = nil
									wantIDs = nil
									if identityGate == "family" {
										wantGates = []ProbeIdentityField{ProbeIdentityManufacturer}
										wantIDs = []string{"synthetic-manufacturer"}
									}
								}
								wantID := []string{"synthetic.family.alpha", "synthetic.family.beta"}[i]
								if item.ProfileID != wantID || item.ProfileVersion != v || item.Score != scores[i] || item.Reason != wantReason || !reflect.DeepEqual(item.MatchedGates, wantGates) || !reflect.DeepEqual(item.ProbeEvidenceIDs, wantIDs) {
									t.Fatalf("candidate %d evidence=%+v; want profile=%s score=%d reason=%s gates=%v evidence=%v", i, item, wantID, scores[i], wantReason, wantGates, wantIDs)
								}
							}
						})
					}
				}
			}
		}
	}
}
