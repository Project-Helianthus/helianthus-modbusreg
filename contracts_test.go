package modbusreg_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func version(t *testing.T, value string) reg.Version {
	t.Helper()
	parsed, err := reg.ParseVersion(value)
	if err != nil {
		t.Fatalf("ParseVersion(%q): %v", value, err)
	}
	return parsed
}

func numericCodecSpec(t *testing.T) reg.CodecSpec {
	t.Helper()
	return reg.CodecSpec{
		ID:                 "u32-energy",
		Version:            version(t, "1.0.0"),
		RawWordCount:       2,
		WordPermutation:    []uint16{1, 0},
		IntraWordByteOrder: reg.ByteOrderModbus,
		Representation:     reg.RepresentationUnsignedInteger,
		Scale: reg.ScaleSpec{
			Source:           reg.ScaleConstant,
			ApplicationOrder: reg.ScaleAfterRepresentation,
			Numerator:        1,
			Denominator:      10,
		},
		Sentinels: []reg.RawSentinel{
			{Kind: reg.SentinelInvalid, Words: []uint16{0xffff, 0xffff}},
		},
		String: reg.StringSpec{
			Applicability: reg.StringNotApplicable,
		},
		OutputProfileType: "decimal",
		ValidityBehavior:  reg.ValidityRejectSentinel,
	}
}

func normalizationSpec(t *testing.T, address uint32) reg.AddressNormalizationSpec {
	t.Helper()
	return reg.AddressNormalizationSpec{
		Version:             version(t, "1.0.0"),
		SourceLocator:       "urn:helianthus:evidence:example-register-map",
		DocumentaryNotation: "one-based input register",
		DocumentaryBase:     reg.AddressBaseOneBased,
		AddressSpaceLabel:   "input_registers",
		DocumentaryAddress:  address,
		Transformation:      reg.TransformSubtractOne,
		ResolvedPDUOffset:   uint16(address - 1),
	}
}

func dependencySpec(t *testing.T, id string, offset uint16) reg.DependencySpec {
	t.Helper()
	return reg.DependencySpec{
		ID:                 id,
		Version:            version(t, "1.0.0"),
		Table:              reg.InputRegisters,
		Normalization:      normalizationSpec(t, uint32(offset)+1),
		WordCount:          2,
		CodecID:            "u32-energy",
		CodecVersion:       version(t, "1.0.0"),
		CoherenceGroup:     "sample",
		EvidenceReferences: []string{"evidence-register-map"},
		ApplicabilityRefs:  []string{"applicability-v1"},
	}
}

func profileFixture(t *testing.T) reg.ProfileDescriptor {
	t.Helper()
	codec, err := reg.NewCodec(numericCodecSpec(t))
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	first, err := reg.NewDependency(dependencySpec(t, "energy-a", 100))
	if err != nil {
		t.Fatalf("NewDependency(first): %v", err)
	}
	second, err := reg.NewDependency(dependencySpec(t, "energy-b", 101))
	if err != nil {
		t.Fatalf("NewDependency(second): %v", err)
	}
	set, err := reg.NewDependencySet(
		version(t, "1.0.0"),
		[]reg.Dependency{first, second},
	)
	if err != nil {
		t.Fatalf("NewDependencySet: %v", err)
	}
	profile, err := reg.NewProfileDescriptor(reg.ProfileDescriptorSpec{
		SchemaVersion:          version(t, "1.0.0"),
		ID:                     "example.standard.energy",
		Version:                version(t, "1.0.0"),
		Kind:                   reg.ProfileStandardFamily,
		StandardApplicability:  []string{"public-standard-v1"},
		ModelApplicability:     []string{"model-a"},
		KnownExclusions:        []string{"model-b"},
		RuntimeContractVersion: version(t, "1.0.0"),
		DetectorVersion:        version(t, "1.0.0"),
		CodecContractVersion:   version(t, "1.0.0"),
		NormalizationVersion:   version(t, "1.0.0"),
		CoherenceVersion:       version(t, "1.0.0"),
		QualificationVersion:   version(t, "1.0.0"),
		Codecs:                 []reg.Codec{codec},
		Dependencies:           set,
		Coherence: reg.CoherencePolicySpec{
			Version:          reg.MustParseVersion("1.0.0"),
			Mode:             reg.CoherenceSingleWireResponse,
			AcquisitionOrder: reg.AcquisitionOrderNotApplicable,
			RetrySetBehavior: reg.RetrySetNotApplicable,
		},
		Evidence: []reg.EvidenceReference{
			{
				ID:                     "evidence-register-map",
				PublicationDisposition: reg.PublicationMetadataOnly,
			},
		},
		Maturity:       reg.MaturityExperimental,
		DefaultEnabled: false,
		State:          reg.ProfileActive,
	})
	if err != nil {
		t.Fatalf("NewProfileDescriptor: %v", err)
	}
	return profile
}

func newFactory(
	t *testing.T,
	profile reg.ProfileDescriptor,
	state reg.SampleLedgerState,
) (*reg.ObservationFactory, *reg.SampleLedger) {
	t.Helper()
	ledger, err := reg.NewSampleLedger(state, 0)
	if err != nil {
		t.Fatalf("NewSampleLedger: %v", err)
	}
	factory, err := reg.NewObservationFactory(profile, ledger)
	if err != nil {
		t.Fatalf("NewObservationFactory: %v", err)
	}
	return factory, ledger
}

func emptyLedgerState(
	t *testing.T,
	profile reg.ProfileDescriptor,
) reg.SampleLedgerState {
	t.Helper()
	state, err := reg.EmptySampleLedgerState(
		"fixture-issuer",
		profile.Dependencies().ID(),
	)
	if err != nil {
		t.Fatalf("EmptySampleLedgerState: %v", err)
	}
	return state
}

func buildObservation(
	t *testing.T,
	profile reg.ProfileDescriptor,
	spec reg.ObservationSpec,
) (reg.Observation, error) {
	t.Helper()
	factory, _ := newFactory(t, profile, emptyLedgerState(t, profile))
	admission, err := factory.NewObservation(spec)
	if err != nil {
		return reg.Observation{}, err
	}
	return admission.Observation(), nil
}

func logicalViewRecord(
	id uint64,
	logicalOffset uint16,
	sliceOffset uint16,
	words []uint16,
) reg.LogicalViewRecord {
	return reg.LogicalViewRecord{
		LogicalViewID:       id,
		WireResponseID:      77,
		PhysicalRequestID:   55,
		Endpoint:            "fixture://endpoint-a",
		ConnectionID:        11,
		Transport:           reg.TransportTCP,
		TransportGeneration: 9,
		UnitID:              1,
		RequestedFunction:   reg.FunctionReadInputRegisters,
		ReceivedFunction:    reg.FunctionReadInputRegisters,
		Table:               reg.InputRegisters,
		PhysicalOffset:      100,
		PhysicalWordCount:   3,
		AuthorizationScope:  "read-only",
		PollGeneration:      41,
		DeadlineIdentity:    12,
		LogicalOffset:       logicalOffset,
		LogicalWordCount:    uint16(len(words)),
		SliceOffset:         sliceOffset,
		SliceWordCount:      uint16(len(words)),
		Words:               words,
	}
}

func successfulObservationSpec(
	t *testing.T,
	profile reg.ProfileDescriptor,
) reg.ObservationSpec {
	t.Helper()
	dependencies := profile.Dependencies().Dependencies()
	firstView, err := reg.NewLogicalViewSnapshot(
		logicalViewRecord(1001, 100, 0, []uint16{0x0102, 0x0304}),
	)
	if err != nil {
		t.Fatalf("NewLogicalViewSnapshot(first): %v", err)
	}
	secondView, err := reg.NewLogicalViewSnapshot(
		logicalViewRecord(1002, 101, 1, []uint16{0x0304, 0x0506}),
	)
	if err != nil {
		t.Fatalf("NewLogicalViewSnapshot(second): %v", err)
	}
	return reg.ObservationSpec{
		SchemaVersion:          version(t, "1.0.0"),
		RuntimeContractVersion: profile.RuntimeContractVersion(),
		ProfileID:              profile.ID(),
		ProfileVersion:         profile.Version(),
		CodecContractVersion:   profile.CodecContractVersion(),
		DetectorVersion:        profile.DetectorVersion(),
		NormalizationVersion:   profile.NormalizationVersion(),
		CoherenceVersion:       profile.CoherenceVersion(),
		QualificationVersion:   profile.QualificationVersion(),
		SampleID:               "",
		RetryAttemptID:         reg.RetryAttemptNotApplicable,
		PollGenerationID:       41,
		DependencySetID:        profile.Dependencies().ID(),
		DependencySetVersion:   profile.Dependencies().Version(),
		SourceValidity:         reg.SourceValid,
		SourceTime:             reg.SourceTimeUnavailable(),
		LocalReceiptTime:       time.Unix(1_700_000_000, 0).UTC(),
		Endpoint:               "fixture://endpoint-a",
		UnitID:                 1,
		Dependencies: []reg.DependencyResult{
			{
				DependencyID:      dependencies[0].ID(),
				DependencyVersion: dependencies[0].Version(),
				CodecID:           dependencies[0].CodecID(),
				CodecVersion:      dependencies[0].CodecVersion(),
				NormalizationVersion: dependencies[0].
					Normalization().Spec().Version,
				Status:         reg.DependencyReadSuccessful,
				View:           firstView,
				RetryAttemptID: reg.RetryAttemptNotApplicable,
			},
			{
				DependencyID:      dependencies[1].ID(),
				DependencyVersion: dependencies[1].Version(),
				CodecID:           dependencies[1].CodecID(),
				CodecVersion:      dependencies[1].CodecVersion(),
				NormalizationVersion: dependencies[1].
					Normalization().Spec().Version,
				Status:         reg.DependencyReadSuccessful,
				View:           secondView,
				RetryAttemptID: reg.RetryAttemptNotApplicable,
			},
		},
	}
}

func TestProfileCodecAndDependencySetAreImmutableAndVersioned(t *testing.T) {
	codecSpec := numericCodecSpec(t)
	codec, err := reg.NewCodec(codecSpec)
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	codecSpec.WordPermutation[0] = 0
	codecSpec.Sentinels[0].Words[0] = 0
	if got := codec.WordPermutation(); !reflect.DeepEqual(got, []uint16{1, 0}) {
		t.Fatalf("codec permutation mutated: %v", got)
	}
	if got := codec.Sentinels()[0].Words; !reflect.DeepEqual(got, []uint16{0xffff, 0xffff}) {
		t.Fatalf("codec sentinel mutated: %v", got)
	}

	profile := profileFixture(t)
	dependencies := profile.Dependencies().Dependencies()
	if got := []string{dependencies[0].ID(), dependencies[1].ID()}; !reflect.DeepEqual(
		got,
		[]string{"energy-a", "energy-b"},
	) {
		t.Fatalf("dependency order changed: %v", got)
	}
	if profile.Dependencies().ID() == "" || profile.Dependencies().Version().String() != "1.0.0" {
		t.Fatalf("dependency-set identity/version missing")
	}
	reversed, err := reg.NewDependencySet(
		profile.Dependencies().Version(),
		[]reg.Dependency{dependencies[1], dependencies[0]},
	)
	if err != nil {
		t.Fatalf("NewDependencySet(reversed): %v", err)
	}
	if reversed.ID() == profile.Dependencies().ID() {
		t.Fatal("dependency-set identity ignored declaration order")
	}
	copyOfDependencies := profile.Dependencies().Dependencies()
	copyOfDependencies[0] = reg.Dependency{}
	if profile.Dependencies().Dependencies()[0].ID() != "energy-a" {
		t.Fatal("dependency set exposed mutable storage")
	}
}

func TestAddressNormalizationRejectsGuessingAndInconsistency(t *testing.T) {
	tests := []struct {
		name string
		spec reg.AddressNormalizationSpec
	}{
		{
			name: "missing transformation",
			spec: func() reg.AddressNormalizationSpec {
				value := normalizationSpec(t, 101)
				value.Transformation = ""
				return value
			}(),
		},
		{
			name: "wrong resolved offset",
			spec: func() reg.AddressNormalizationSpec {
				value := normalizationSpec(t, 101)
				value.ResolvedPDUOffset = 101
				return value
			}(),
		},
		{
			name: "zero one-based address",
			spec: normalizationSpec(t, 0),
		},
		{
			name: "overflow",
			spec: func() reg.AddressNormalizationSpec {
				value := normalizationSpec(t, 65537)
				value.ResolvedPDUOffset = 0
				return value
			}(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := reg.NewAddressNormalization(test.spec); err == nil {
				t.Fatal("invalid documentary normalization was accepted")
			}
		})
	}
}

func TestDependencyRejectsDocumentaryTableMismatch(t *testing.T) {
	spec := dependencySpec(t, "wrong-space", 100)
	spec.Normalization.AddressSpaceLabel = string(reg.HoldingRegisters)
	if _, err := reg.NewDependency(spec); err == nil {
		t.Fatal("dependency accepted a documentary address-space mismatch")
	}
}

func TestCodecRejectsIncompleteOrCoercedDimensions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*reg.CodecSpec)
	}{
		{"missing permutation", func(spec *reg.CodecSpec) { spec.WordPermutation = nil }},
		{"non permutation", func(spec *reg.CodecSpec) { spec.WordPermutation = []uint16{0, 0} }},
		{"unknown byte order", func(spec *reg.CodecSpec) { spec.IntraWordByteOrder = "" }},
		{"unknown representation", func(spec *reg.CodecSpec) { spec.Representation = "" }},
		{"zero scale denominator", func(spec *reg.CodecSpec) { spec.Scale.Denominator = 0 }},
		{"undeclared string dimension", func(spec *reg.CodecSpec) { spec.String.Applicability = "" }},
		{"silent validity behavior", func(spec *reg.CodecSpec) { spec.ValidityBehavior = "" }},
		{"wrong sentinel width", func(spec *reg.CodecSpec) { spec.Sentinels[0].Words = []uint16{1} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := numericCodecSpec(t)
			test.mutate(&spec)
			if _, err := reg.NewCodec(spec); err == nil {
				t.Fatal("incomplete codec was accepted")
			}
		})
	}
}

func TestStringCodecRequiresEveryPackingDimension(t *testing.T) {
	padding := byte(0)
	spec := numericCodecSpec(t)
	spec.ID = "documentary-string"
	spec.RawWordCount = 4
	spec.WordPermutation = []uint16{0, 1, 2, 3}
	spec.Representation = reg.RepresentationString
	spec.Scale = reg.ScaleSpec{
		Source:           reg.ScaleNotApplicable,
		ApplicationOrder: reg.ScaleOrderNotApplicable,
	}
	spec.String = reg.StringSpec{
		Applicability:                  reg.StringApplicable,
		WordPacking:                    reg.StringHighByteFirst,
		ByteOrder:                      reg.ByteOrderModbus,
		PaddingByte:                    &padding,
		Termination:                    reg.StringNULTerminated,
		RetainedRawLength:              8,
		DocumentaryCharacterRepertoire: "ASCII",
	}
	spec.Sentinels = nil
	if _, err := reg.NewCodec(spec); err != nil {
		t.Fatalf("complete string codec rejected: %v", err)
	}
	spec.String.PaddingByte = nil
	if _, err := reg.NewCodec(spec); err == nil {
		t.Fatal("string codec silently guessed its padding byte")
	}
}

func TestProfileRejectsCodecWidthAndScaleDependencyMismatch(t *testing.T) {
	base := profileFixture(t)
	profileSpec := base.Spec()
	codecSpec := profileSpec.Codecs[0].Spec()
	codecSpec.RawWordCount = 1
	codecSpec.WordPermutation = []uint16{0}
	codecSpec.Sentinels[0].Words = []uint16{0xffff}
	narrow, err := reg.NewCodec(codecSpec)
	if err != nil {
		t.Fatalf("NewCodec(narrow): %v", err)
	}
	profileSpec.Codecs = []reg.Codec{narrow}
	if _, err := reg.NewProfileDescriptor(profileSpec); err == nil {
		t.Fatal("profile accepted codec/dependency raw-width mismatch")
	}

	profileSpec = base.Spec()
	codecSpec = profileSpec.Codecs[0].Spec()
	codecSpec.Scale = reg.ScaleSpec{
		Source:           reg.ScaleDependency,
		ApplicationOrder: reg.ScaleAfterRepresentation,
		DependencyID:     "absent-scale",
	}
	scaled, err := reg.NewCodec(codecSpec)
	if err != nil {
		t.Fatalf("NewCodec(dependency scale): %v", err)
	}
	profileSpec.Codecs = []reg.Codec{scaled}
	if _, err := reg.NewProfileDescriptor(profileSpec); err == nil {
		t.Fatal("profile accepted an absent scale dependency")
	}

	codecSpec.Scale.DependencyID = "energy-a"
	selfScaled, err := reg.NewCodec(codecSpec)
	if err != nil {
		t.Fatalf("NewCodec(self scale): %v", err)
	}
	profileSpec.Codecs = []reg.Codec{selfScaled}
	if _, err := reg.NewProfileDescriptor(profileSpec); err == nil {
		t.Fatal("profile accepted a cyclic scale dependency")
	}
}

func TestCatalogOrderAndDuplicateRejectionAreDeterministic(t *testing.T) {
	first := profileFixture(t)
	secondSpec := first.Spec()
	secondSpec.ID = "aaa.standard.energy"
	secondSpec.Version = version(t, "2.0.0")
	second, err := reg.NewProfileDescriptor(secondSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(second): %v", err)
	}
	catalog, err := reg.NewCatalog(second, first)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	got := catalog.Profiles()
	if got[0].ID() != "aaa.standard.energy" || got[1].ID() != "example.standard.energy" {
		t.Fatalf("catalog order is not deterministic: %q, %q", got[0].ID(), got[1].ID())
	}
	if selected, ok := catalog.Lookup("example.standard.energy"); !ok ||
		selected.Version() != first.Version() {
		t.Fatal("catalog lookup did not return the exact immutable profile")
	}
	if _, err := reg.NewCatalog(first, first); err == nil {
		t.Fatal("duplicate profile tuple was accepted")
	}
	duplicateID := first.Spec()
	duplicateID.Version = version(t, "9.0.0")
	otherVersion, err := reg.NewProfileDescriptor(duplicateID)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(otherVersion): %v", err)
	}
	if _, err := reg.NewCatalog(first, otherVersion); err == nil {
		t.Fatal("duplicate profile ID was accepted")
	}
}

func TestVendorAndStandardProfileBoundariesFailClosed(t *testing.T) {
	explicitEmpty := profileFixture(t).Spec()
	explicitEmpty.ModelApplicability = []string{}
	explicitEmpty.KnownExclusions = []string{}
	if _, err := reg.NewProfileDescriptor(explicitEmpty); err != nil {
		t.Fatalf("explicit empty applicability/exclusions rejected: %v", err)
	}
	explicitEmpty.KnownExclusions = nil
	if _, err := reg.NewProfileDescriptor(explicitEmpty); err == nil {
		t.Fatal("omitted known-exclusion dimension was accepted")
	}

	standard := profileFixture(t).Spec()
	standard.VendorApplicability = []string{"vendor-a"}
	if _, err := reg.NewProfileDescriptor(standard); err == nil {
		t.Fatal("standard family accepted vendor assumptions")
	}

	overlay := profileFixture(t).Spec()
	overlay.ID = "vendor.overlay.energy"
	overlay.Kind = reg.ProfileVendorOverlay
	overlay.VendorApplicability = []string{"vendor-a"}
	overlay.RefinesProfileID = ""
	overlay.RefinesProfileVersion = reg.Version{}
	if _, err := reg.NewProfileDescriptor(overlay); err == nil {
		t.Fatal("vendor overlay without qualified standard-family version was accepted")
	}
}

func TestObservationReplaysUnequalOverlappingViewsExactly(t *testing.T) {
	profile := profileFixture(t)
	spec := successfulObservationSpec(t, profile)
	originalWords := spec.Dependencies[0].View.Record().Words
	observation, err := buildObservation(t, profile, spec)
	if err != nil {
		t.Fatalf("NewObservation: %v", err)
	}
	originalWords[0] = 0xffff
	spec.Dependencies[0] = reg.DependencyResult{}

	replayed := observation.Replay()
	if len(replayed) != 2 {
		t.Fatalf("Replay count = %d, want 2", len(replayed))
	}
	first := replayed[0]
	second := replayed[1]
	if !reflect.DeepEqual(first.RawWords(), []uint16{0x0102, 0x0304}) {
		t.Fatalf("first words = %v", first.RawWords())
	}
	if !reflect.DeepEqual(second.RawWords(), []uint16{0x0304, 0x0506}) {
		t.Fatalf("second words = %v", second.RawWords())
	}
	if first.LogicalViewID() == second.LogicalViewID() ||
		first.WireResponseID() != second.WireResponseID() {
		t.Fatal("logical views did not retain distinct logical/shared wire identities")
	}
	if first.LogicalOffset() != 100 || first.LogicalWordCount() != 2 ||
		first.SliceOffset() != 0 || first.SliceWordCount() != 2 {
		t.Fatal("first view provenance changed")
	}
	if second.LogicalOffset() != 101 || second.LogicalWordCount() != 2 ||
		second.SliceOffset() != 1 || second.SliceWordCount() != 2 {
		t.Fatal("second view provenance changed")
	}
	if first.Normalization().ResolvedPDUOffset() != 100 ||
		second.Normalization().ResolvedPDUOffset() != 101 {
		t.Fatal("documentary normalization was not retained")
	}
	if first.DependencyID() != "energy-a" ||
		first.DependencyVersion() != version(t, "1.0.0") ||
		first.CodecID() != "u32-energy" ||
		first.CodecVersion() != version(t, "1.0.0") {
		t.Fatal("dependency contract identities were not retained")
	}
	envelope := observation.Spec()
	if envelope.ProfileID != profile.ID() ||
		envelope.DependencySetVersion != profile.Dependencies().Version() ||
		envelope.SourceValidity != reg.SourceValid ||
		envelope.SourceTime.State != reg.SourceTimeUnavailableState ||
		envelope.LocalReceiptTime.IsZero() {
		t.Fatal("observation envelope did not retain source and contract facts")
	}
	replayed[0].RawWords()[0] = 0
	if observation.Replay()[0].RawWords()[0] != 0x0102 {
		t.Fatal("replay exposed mutable raw words")
	}
}

func TestLogicalViewSnapshotRejectsMalformedSliceProvenance(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*reg.LogicalViewRecord)
	}{
		{"missing wire identity", func(record *reg.LogicalViewRecord) {
			record.WireResponseID = 0
		}},
		{"table mismatch", func(record *reg.LogicalViewRecord) {
			record.Table = reg.HoldingRegisters
		}},
		{"slice outside response", func(record *reg.LogicalViewRecord) {
			record.SliceOffset = 2
		}},
		{"logical offset mismatch", func(record *reg.LogicalViewRecord) {
			record.LogicalOffset++
		}},
		{"word count mismatch", func(record *reg.LogicalViewRecord) {
			record.Words = record.Words[:1]
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := logicalViewRecord(
				1001,
				100,
				0,
				[]uint16{0x0102, 0x0304},
			)
			test.mutate(&record)
			if _, err := reg.NewLogicalViewSnapshot(record); err == nil {
				t.Fatal("malformed logical-view provenance was accepted")
			}
		})
	}
}

func TestObservationRejectsIncompleteOrIncoherentInputs(t *testing.T) {
	profile := profileFixture(t)
	tests := []struct {
		name   string
		mutate func(*reg.ObservationSpec)
	}{
		{"missing dependency", func(spec *reg.ObservationSpec) {
			spec.Dependencies = spec.Dependencies[:1]
		}},
		{"wrong dependency order", func(spec *reg.ObservationSpec) {
			spec.Dependencies[0], spec.Dependencies[1] = spec.Dependencies[1], spec.Dependencies[0]
		}},
		{"mixed poll generation", func(spec *reg.ObservationSpec) {
			record := spec.Dependencies[1].View.Record()
			record.PollGeneration++
			spec.Dependencies[1].View, _ = reg.NewLogicalViewSnapshot(record)
		}},
		{"torn", func(spec *reg.ObservationSpec) {
			spec.Dependencies[0].Status = reg.DependencyReadTorn
		}},
		{"malformed", func(spec *reg.ObservationSpec) {
			spec.Dependencies[0].Status = reg.DependencyReadMalformed
		}},
		{"exceptional", func(spec *reg.ObservationSpec) {
			spec.Dependencies[0].Status = reg.DependencyReadException
		}},
		{"provenance incomplete", func(spec *reg.ObservationSpec) {
			spec.Dependencies[0].View = reg.LogicalViewSnapshot{}
		}},
		{"wrong dependency set", func(spec *reg.ObservationSpec) {
			spec.DependencySetID = "dependency-set:wrong"
		}},
		{"wrong dependency set version", func(spec *reg.ObservationSpec) {
			spec.DependencySetVersion = version(t, "9.0.0")
		}},
		{"wrong codec version", func(spec *reg.ObservationSpec) {
			spec.Dependencies[0].CodecVersion = version(t, "9.0.0")
		}},
		{"reused generation identity", func(spec *reg.ObservationSpec) {
			spec.PollGenerationID++
		}},
		{"caller-selected sample id", func(spec *reg.ObservationSpec) {
			spec.SampleID = "caller-selected"
		}},
		{"missing source validity", func(spec *reg.ObservationSpec) {
			spec.SourceValidity = ""
		}},
		{"missing local receipt", func(spec *reg.ObservationSpec) {
			spec.LocalReceiptTime = time.Time{}
		}},
		{"unknown observation schema", func(spec *reg.ObservationSpec) {
			spec.SchemaVersion = version(t, "2.0.0")
		}},
		{"mixed endpoint", func(spec *reg.ObservationSpec) {
			record := spec.Dependencies[1].View.Record()
			record.Endpoint = "fixture://endpoint-b"
			spec.Dependencies[1].View, _ = reg.NewLogicalViewSnapshot(record)
		}},
		{"mixed wire response", func(spec *reg.ObservationSpec) {
			record := spec.Dependencies[1].View.Record()
			record.WireResponseID++
			spec.Dependencies[1].View, _ = reg.NewLogicalViewSnapshot(record)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := successfulObservationSpec(t, profile)
			test.mutate(&spec)
			if _, err := buildObservation(t, profile, spec); err == nil {
				t.Fatal("incomplete or incoherent observation was accepted")
			}
		})
	}
}

func TestBoundedMultiResponseEnforcesDeclaredSkewAndMarker(t *testing.T) {
	base := profileFixture(t)
	profileSpec := base.Spec()
	profileSpec.Coherence = reg.CoherencePolicySpec{
		Version:                      base.CoherenceVersion(),
		Mode:                         reg.CoherenceBoundedMultiResponse,
		MaximumSourceSkew:            2 * time.Second,
		MaximumReceiptSkew:           3 * time.Second,
		RequireGenerationEquality:    true,
		AcquisitionOrder:             reg.AcquisitionOrderDependencyDeclaration,
		RetrySetBehavior:             reg.RetryWholeSet,
		DocumentaryConsistencyMarker: "sequence-7",
	}
	profile, err := reg.NewProfileDescriptor(profileSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor: %v", err)
	}
	spec := successfulObservationSpec(t, profile)
	spec.RetryAttemptID = 1
	source := time.Unix(1_700_000_100, 0).UTC()
	receipt := source.Add(time.Second)
	for index := range spec.Dependencies {
		record := spec.Dependencies[index].View.Record()
		record.WireResponseID += uint64(index)
		record.PhysicalRequestID += uint64(index)
		spec.Dependencies[index].View, err = reg.NewLogicalViewSnapshot(record)
		if err != nil {
			t.Fatalf("NewLogicalViewSnapshot(%d): %v", index, err)
		}
		spec.Dependencies[index].SourceTime = reg.SourceTimeObserved(
			source.Add(time.Duration(index) * time.Second),
		)
		spec.Dependencies[index].LocalReceiptTime = receipt.Add(
			time.Duration(index) * time.Second,
		)
		spec.Dependencies[index].DocumentaryConsistencyMarker = "sequence-7"
		spec.Dependencies[index].AcquisitionOrdinal = uint32(index + 1)
		spec.Dependencies[index].RetryAttemptID = 1
	}
	spec.SourceTime = reg.SourceTimeObserved(source.Add(time.Second))
	spec.LocalReceiptTime = receipt.Add(time.Second)
	if _, err := buildObservation(t, profile, spec); err != nil {
		t.Fatalf("bounded coherent observation rejected: %v", err)
	}
	spec.Dependencies[1].LocalReceiptTime = receipt.Add(4 * time.Second)
	if _, err := buildObservation(t, profile, spec); err == nil {
		t.Fatal("receipt skew beyond the declared bound was accepted")
	}
	spec.Dependencies[1].LocalReceiptTime = receipt.Add(time.Second)
	spec.Dependencies[1].DocumentaryConsistencyMarker = "sequence-8"
	if _, err := buildObservation(t, profile, spec); err == nil {
		t.Fatal("documentary consistency marker mismatch was accepted")
	}
}

func TestSourceTimeStateIsExplicit(t *testing.T) {
	profile := profileFixture(t)
	spec := successfulObservationSpec(t, profile)
	spec.SourceTime = reg.SourceTimeObserved(time.Unix(1_699_999_999, 0).UTC())
	if _, err := buildObservation(t, profile, spec); err != nil {
		t.Fatalf("explicit observed source time rejected: %v", err)
	}
	spec.SourceTime = reg.SourceTimeSpec{State: reg.SourceTimeObservedState}
	if _, err := buildObservation(t, profile, spec); err == nil {
		t.Fatal("observed source-time state without a time was accepted")
	}
	spec.SourceTime = reg.SourceTimeSpec{
		State: reg.SourceTimeUnavailableState,
		Time:  time.Unix(1_699_999_999, 0).UTC(),
	}
	if _, err := buildObservation(t, profile, spec); err == nil {
		t.Fatal("unavailable source-time state with a guessed time was accepted")
	}
}

func TestSampleLedgerRejectsEverySampleIDReuse(t *testing.T) {
	profile := profileFixture(t)
	factory, ledger := newFactory(t, profile, emptyLedgerState(t, profile))
	spec := successfulObservationSpec(t, profile)
	first, err := factory.NewObservation(spec)
	if err != nil {
		t.Fatalf("first NewObservation: %v", err)
	}
	second, err := factory.NewObservation(spec)
	if err != nil {
		t.Fatalf("second NewObservation: %v", err)
	}
	if first.Observation().SampleID() == second.Observation().SampleID() ||
		ledger.ExportState().HighWater != 2 {
		t.Fatal("factory reused a sample ID")
	}
}

func TestPublicSurfaceDoesNotIntroduceCanonicalPolicy(t *testing.T) {
	for _, name := range []string{
		"canonical unit",
		"freshness policy",
		"availability policy",
		"publication policy",
	} {
		if strings.Contains(strings.ToLower(reg.OwnershipBoundary()), name) {
			t.Fatalf("registry claimed forbidden canonical policy: %s", name)
		}
	}
}
