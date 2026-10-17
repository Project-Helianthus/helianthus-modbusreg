package modbusreg_test

import (
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestDetectionDecisionBoundEquality(t *testing.T) {
	for _, fixture := range []struct {
		name, id, profileVersion, detectorVersion, qualificationVersion string
		evidenceBytes                                                   int
	}{
		{"short_id", "family.a", "1.0.0", "1.0.0", "1.0.0", 17},
		{"long_id", "synthetic.standard.family.long", "1.0.0", "1.0.0", "1.0.0", 17},
		{"profile_version_length", "family.a", "12.34.56", "1.0.0", "1.0.0", 17},
		{"detector_version_length", "family.a", "1.0.0", "12.34.56", "1.0.0", 17},
		{"qualification_version_length", "family.a", "1.0.0", "1.0.0", "12.34.56", 17},
		{"evidence_id_budget", "family.a", "1.0.0", "1.0.0", "1.0.0", 43},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			limits := reg.DefaultDetectionLimits()
			limits.MaxEvidenceIDBytes = fixture.evidenceBytes
			profile := decisionBoundProfile(t, fixture.id, fixture.profileVersion, fixture.detectorVersion, fixture.qualificationVersion)
			catalog, err := reg.NewCatalog(profile)
			if err != nil {
				t.Fatalf("NewCatalog: %v", err)
			}
			declarations := []reg.ProbeDeclarationSpec{
				{ID: "manufacturer", Function: reg.ProbeReadHoldingRegisters, Address: 100, WordCount: 16, IdentityField: reg.ProbeIdentityManufacturer, Encoding: reg.ProbeIdentityASCII},
				{ID: "family", Function: reg.ProbeReadInputRegisters, Address: 140, WordCount: 16, IdentityField: reg.ProbeIdentityModel, Encoding: reg.ProbeIdentityASCII},
				{ID: "firmware", Function: reg.ProbeReadHoldingRegisters, Address: 180, WordCount: 16, IdentityField: reg.ProbeIdentityFirmware, Encoding: reg.ProbeIdentityASCII},
			}
			plan, err := reg.NewProbePlan(reg.ProbePlanSpec{
				Version: reg.MustParseVersion(fixture.detectorVersion), Declarations: declarations,
			}, limits)
			if err != nil {
				t.Fatalf("NewProbePlan: %v", err)
			}
			candidate, err := reg.NewDetectionCandidate(reg.DetectionCandidateSpec{
				ProfileID: fixture.id, ProfileVersion: reg.MustParseVersion(fixture.profileVersion),
				Score: 1, Enabled: true, FixtureOnly: true,
				Manufacturer: reg.IdentityStringGateSpec{Expected: "synthetic-manufacturer"},
				Model:        reg.IdentityStringGateSpec{Expected: "synthetic-family"},
				Firmware:     reg.FirmwareGateSpec{MinimumInclusive: reg.MustParseVersion("1.0.0"), MaximumExclusive: reg.MustParseVersion("2.0.0")},
			}, limits)
			if err != nil {
				t.Fatalf("NewDetectionCandidate: %v", err)
			}

			// Independent declaration oracle: fixture strings, one candidate, three reads.
			// No estimator, serialized output, or version formatting supplies the expected budget.
			bound := 512 + 768 + 6*(len(fixture.id)+len(fixture.profileVersion)+len(fixture.detectorVersion)+len(fixture.qualificationVersion)) + 3*(6*fixture.evidenceBytes+128)
			for _, boundary := range []struct {
				name  string
				delta int
			}{
				{"below", -1}, {"equal", 0}, {"above", 1},
			} {
				t.Run(boundary.name, func(t *testing.T) {
					caseLimits := limits
					caseLimits.MaxDecisionBytes = bound + boundary.delta
					// Construction has no reader parameter; no runtime detection is invoked.
					detector, err := reg.NewProfileDetector(reg.ProfileDetectorSpec{
						DetectorVersion: reg.MustParseVersion(fixture.detectorVersion), Plan: plan,
						Catalog: catalog, Candidates: []reg.DetectionCandidate{candidate}, Limits: caseLimits,
					})
					if boundary.delta < 0 {
						if err == nil || err.Error() != "detection decision exceeds the configured byte bound" || detector != nil {
							t.Fatalf("budget %d below bound %d: detector = %v, error = %v", caseLimits.MaxDecisionBytes, bound, detector, err)
						}
					} else if err != nil || detector == nil {
						t.Fatalf("budget %d at/above bound %d: detector = %v, error = %v", caseLimits.MaxDecisionBytes, bound, detector, err)
					}
				})
			}
		})
	}
}

func decisionBoundProfile(t *testing.T, id, profileVersion, detectorVersion, qualificationVersion string) reg.ProfileDescriptor {
	t.Helper()
	v := reg.MustParseVersion("1.0.0")
	codec, err := reg.NewCodec(reg.CodecSpec{
		ID: "synthetic-u16", Version: v, RawWordCount: 1, WordPermutation: []uint16{0},
		IntraWordByteOrder: reg.ByteOrderModbus, Representation: reg.RepresentationUnsignedInteger,
		Scale:             reg.ScaleSpec{Source: reg.ScaleConstant, ApplicationOrder: reg.ScaleAfterRepresentation, Numerator: 1, Denominator: 1},
		String:            reg.StringSpec{Applicability: reg.StringNotApplicable},
		OutputProfileType: "decimal", ValidityBehavior: reg.ValidityRejectSentinel,
	})
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	dependency, err := reg.NewDependency(reg.DependencySpec{
		ID: "synthetic-value", Version: v, Table: reg.InputRegisters,
		Normalization: reg.AddressNormalizationSpec{
			Version: v, SourceLocator: "urn:helianthus:evidence:synthetic-register-map",
			DocumentaryNotation: "one-based input register", DocumentaryBase: reg.AddressBaseOneBased,
			AddressSpaceLabel: "input_registers", DocumentaryAddress: 101,
			Transformation: reg.TransformSubtractOne, ResolvedPDUOffset: 100,
		},
		WordCount: 1, CodecID: "synthetic-u16", CodecVersion: v, CoherenceGroup: "sample",
		EvidenceReferences: []string{"synthetic-map"}, ApplicabilityRefs: []string{"synthetic-family"},
	})
	if err != nil {
		t.Fatalf("NewDependency: %v", err)
	}
	dependencies, err := reg.NewDependencySet(v, []reg.Dependency{dependency})
	if err != nil {
		t.Fatalf("NewDependencySet: %v", err)
	}
	profile, err := reg.NewProfileDescriptor(reg.ProfileDescriptorSpec{
		SchemaVersion: v, ID: id, Version: reg.MustParseVersion(profileVersion), Kind: reg.ProfileStandardFamily,
		StandardApplicability: []string{"synthetic-standard-family"}, ModelApplicability: []string{}, KnownExclusions: []string{},
		RuntimeContractVersion: v, DetectorVersion: reg.MustParseVersion(detectorVersion), CodecContractVersion: v,
		NormalizationVersion: v, CoherenceVersion: v, QualificationVersion: reg.MustParseVersion(qualificationVersion),
		Codecs: []reg.Codec{codec}, Dependencies: dependencies,
		Coherence: reg.CoherencePolicySpec{Version: v, Mode: reg.CoherenceSingleWireResponse, AcquisitionOrder: reg.AcquisitionOrderNotApplicable, RetrySetBehavior: reg.RetrySetNotApplicable},
		Evidence:  []reg.EvidenceReference{{ID: "synthetic-map", PublicationDisposition: reg.PublicationMetadataOnly}},
		Maturity:  reg.MaturityExperimental, State: reg.ProfileActive,
	})
	if err != nil {
		t.Fatalf("NewProfileDescriptor: %v", err)
	}
	return profile
}
