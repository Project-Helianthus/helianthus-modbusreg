package modbusreg

import (
	"context"
	"reflect"
	"testing"
)

// Word-count overflow uses the internal seam: a locally valid plan cannot
// declare a matching read above MaxWordsPerRead.
func TestU25ResultLocalBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		words    int
		evidence string
		want     bool
	}{
		{"exact_evidence_id", 1, "abcd", true},
		{"evidence_id_one_byte_over", 1, "abcde", false},
		{"exact_word_count", 3, "word", true},
		{"word_count_one_over", 4, "word", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := DefaultDetectionLimits()
			if tc.words == 1 {
				limits.MaxEvidenceIDBytes = 4
			} else {
				limits.MaxWordsPerRead = 3
				limits.MaxTotalWords = 9
			}
			result := u25Result(t, make([]uint16, tc.words), tc.evidence)
			if got := result.validWithin(limits); got != tc.want {
				t.Fatalf("globally admitted result validWithin(local) = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestU25DetectRevalidatesEvidenceID(t *testing.T) {
	limits := DefaultDetectionLimits()
	limits.MaxEvidenceIDBytes = 4
	detector, profile := u25Detector(t, limits)
	for _, tc := range []struct {
		name           string
		excessiveIndex int
	}{
		{"exact_local_boundary", -1},
		{"first_result_one_byte_over", 0},
		{"second_result_one_byte_over", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ids := []string{"aaaa", "bbbb", "cccc"}
			if tc.excessiveIndex >= 0 {
				ids[tc.excessiveIndex] += "x"
			}
			reader := &u25Reader{t: t, results: []ProbeReadResult{
				u25Result(t, []uint16{0x4142, 0, 0}, ids[0]),
				u25Result(t, []uint16{0x4344, 0, 0}, ids[1]),
				u25Result(t, []uint16{0x312e, 0x302e, 0x3000}, ids[2]),
			}}
			decision, err := detector.Detect(context.Background(), reader, DetectionOptions{})
			if err != nil {
				t.Fatal(err)
			}
			order := []string{"manufacturer", "family", "firmware"}
			if tc.excessiveIndex >= 0 {
				order = order[:tc.excessiveIndex+1]
				if decision.Outcome() != DetectionNoMatch || decision.Reason() != DetectionReasonInvalidProbeResult ||
					decision.SelectedProfileID() != "" || decision.SelectedProfileVersion() != (Version{}) {
					t.Fatalf("excessive result: outcome=%s reason=%s selected=%q version=%v", decision.Outcome(), decision.Reason(), decision.SelectedProfileID(), decision.SelectedProfileVersion())
				}
				for _, evidence := range decision.Evidence() {
					if evidence.Reason != DetectionReasonInvalidProbeResult || len(evidence.MatchedGates) != 0 || len(evidence.ProbeEvidenceIDs) != 0 {
						t.Fatalf("excessive result reached candidate evaluation: %+v", evidence)
					}
				}
			} else {
				if decision.Outcome() != DetectionMatched || decision.Reason() != DetectionReasonSelected ||
					decision.SelectedProfileID() != profile.ID() || decision.SelectedProfileVersion() != profile.Version() {
					t.Fatalf("bounded result: outcome=%s reason=%s selected=%q", decision.Outcome(), decision.Reason(), decision.SelectedProfileID())
				}
				evidence := decision.Evidence()
				if len(evidence) != 1 || evidence[0].Reason != DetectionReasonSelected ||
					!reflect.DeepEqual(evidence[0].ProbeEvidenceIDs, ids) ||
					!reflect.DeepEqual(evidence[0].MatchedGates, []ProbeIdentityField{ProbeIdentityManufacturer, ProbeIdentityModel, ProbeIdentityFirmware}) {
					t.Fatalf("bounded result provenance = %+v", evidence)
				}
			}
			if !reflect.DeepEqual(reader.reads, order) {
				t.Fatalf("reads=%v, want %v", reader.reads, order)
			}
		})
	}
}

type u25Reader struct {
	t       *testing.T
	results []ProbeReadResult
	reads   []string
}

func (r *u25Reader) ReadProbe(_ context.Context, request ProbeReadRequest) (ProbeReadResult, error) {
	r.t.Helper()
	index := len(r.reads)
	if index >= len(r.results) {
		r.t.Fatal("unexpected extra probe read")
	}
	if request.WordCount() != uint16(len(r.results[index].Words())) {
		r.t.Fatal("fixture result width does not match declaration")
	}
	r.reads = append(r.reads, request.DeclarationID())
	return r.results[index], nil
}

func u25Result(t *testing.T, words []uint16, id string) ProbeReadResult {
	t.Helper()
	global := DefaultDetectionLimits()
	if len(id) >= global.MaxEvidenceIDBytes || len(words) >= global.MaxWordsPerRead {
		t.Fatal("fixture must stay below global bounds")
	}
	result, err := NewProbeReadResult(ProbeReadResultSpec{Status: ProbeReadSucceeded, Words: words, EvidenceID: id})
	if err != nil {
		t.Fatalf("global constructor rejected fixture: %v", err)
	}
	return result
}

func u25Detector(t *testing.T, limits DetectionLimits) (*ProfileDetector, ProfileDescriptor) {
	t.Helper()
	v := MustParseVersion("1.0.0")
	codec, err := NewCodec(CodecSpec{
		ID: "u25-word", Version: v, RawWordCount: 1, WordPermutation: []uint16{0},
		IntraWordByteOrder: ByteOrderModbus, Representation: RepresentationUnsignedInteger,
		Scale:  ScaleSpec{Source: ScaleConstant, ApplicationOrder: ScaleAfterRepresentation, Numerator: 1, Denominator: 1},
		String: StringSpec{Applicability: StringNotApplicable}, OutputProfileType: "integer", ValidityBehavior: ValidityRejectSentinel,
	})
	if err != nil {
		t.Fatal(err)
	}
	dependency, err := NewDependency(DependencySpec{
		ID: "u25-value", Version: v, Table: InputRegisters, WordCount: 1, CodecID: codec.ID(), CodecVersion: v,
		Normalization:  AddressNormalizationSpec{Version: v, SourceLocator: "https://example.com/synthetic/probe-local-limit-map", DocumentaryNotation: "zero-based register", DocumentaryBase: AddressBaseZeroBased, AddressSpaceLabel: "input_registers", DocumentaryAddress: 1, Transformation: TransformIdentity, ResolvedPDUOffset: 1},
		CoherenceGroup: "sample", EvidenceReferences: []string{"u25-evidence"}, ApplicabilityRefs: []string{"synthetic-family"},
	})
	if err != nil {
		t.Fatal(err)
	}
	dependencies, err := NewDependencySet(v, []Dependency{dependency})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := NewProfileDescriptor(ProfileDescriptorSpec{
		SchemaVersion: v, ID: "example.standard.u25", Version: v, Kind: ProfileStandardFamily,
		StandardApplicability:  []string{"synthetic-family"},
		ModelApplicability:     []string{},
		KnownExclusions:        []string{},
		RuntimeContractVersion: v, DetectorVersion: v, CodecContractVersion: v, NormalizationVersion: v, CoherenceVersion: v, QualificationVersion: v,
		Codecs: []Codec{codec}, Dependencies: dependencies,
		Coherence: CoherencePolicySpec{Version: v, Mode: CoherenceSingleWireResponse, AcquisitionOrder: AcquisitionOrderNotApplicable, RetrySetBehavior: RetrySetNotApplicable},
		Evidence:  []EvidenceReference{{ID: "u25-evidence", PublicationDisposition: PublicationMetadataOnly}},
		Maturity:  MaturityQualified, State: ProfileActive, DefaultEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := NewCatalog(profile)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := NewDetectionCandidate(DetectionCandidateSpec{
		ProfileID: profile.ID(), ProfileVersion: v, Score: 1, Enabled: true,
		Manufacturer: IdentityStringGateSpec{Expected: "AB"}, Model: IdentityStringGateSpec{Expected: "CD"},
		Firmware: FirmwareGateSpec{MinimumInclusive: v, MaximumExclusive: MustParseVersion("2.0.0")},
	}, limits)
	if err != nil {
		t.Fatal(err)
	}
	declarations := make([]ProbeDeclarationSpec, 3)
	for i, field := range []ProbeIdentityField{ProbeIdentityManufacturer, ProbeIdentityModel, ProbeIdentityFirmware} {
		declarations[i] = ProbeDeclarationSpec{ID: []string{"manufacturer", "family", "firmware"}[i], Function: ProbeReadInputRegisters, Address: uint16(i * 3), WordCount: 3, IdentityField: field, Encoding: ProbeIdentityASCII}
	}
	plan, err := NewProbePlan(ProbePlanSpec{Version: v, Declarations: declarations}, limits)
	if err != nil {
		t.Fatal(err)
	}
	detector, err := NewProfileDetector(ProfileDetectorSpec{DetectorVersion: v, Plan: plan, Catalog: catalog, Candidates: []DetectionCandidate{candidate}, Limits: limits})
	if err != nil {
		t.Fatal(err)
	}
	return detector, profile
}
