package modbusreg_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func snapshotFromRecord(
	t *testing.T,
	record reg.LogicalViewRecord,
) reg.LogicalViewSnapshot {
	t.Helper()
	snapshot, err := reg.NewLogicalViewSnapshot(record)
	if err != nil {
		t.Fatalf("NewLogicalViewSnapshot: %v", err)
	}
	return snapshot
}

func TestRound1SharedWireRejectsForgeryAndContradictoryWords(t *testing.T) {
	profile := profileFixture(t)
	tests := []struct {
		name   string
		mutate func(*reg.ObservationSpec)
	}{
		{
			name: "duplicate logical view id",
			mutate: func(spec *reg.ObservationSpec) {
				record := spec.Dependencies[1].View.Record()
				record.LogicalViewID = spec.Dependencies[0].View.Record().LogicalViewID
				spec.Dependencies[1].View = snapshotFromRecord(t, record)
			},
		},
		{
			name: "contradictory overlapping physical word",
			mutate: func(spec *reg.ObservationSpec) {
				record := spec.Dependencies[1].View.Record()
				record.Words[0] ^= 0xffff
				spec.Dependencies[1].View = snapshotFromRecord(t, record)
			},
		},
		{
			name: "forged connection",
			mutate: func(spec *reg.ObservationSpec) {
				record := spec.Dependencies[1].View.Record()
				record.ConnectionID++
				spec.Dependencies[1].View = snapshotFromRecord(t, record)
			},
		},
		{
			name: "forged authorization scope",
			mutate: func(spec *reg.ObservationSpec) {
				record := spec.Dependencies[1].View.Record()
				record.AuthorizationScope = "other-scope"
				spec.Dependencies[1].View = snapshotFromRecord(t, record)
			},
		},
		{
			name: "forged deadline identity",
			mutate: func(spec *reg.ObservationSpec) {
				record := spec.Dependencies[1].View.Record()
				record.DeadlineIdentity++
				spec.Dependencies[1].View = snapshotFromRecord(t, record)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := successfulObservationSpec(t, profile)
			test.mutate(&spec)
			if _, err := buildObservation(t, profile, spec); err == nil {
				t.Fatal("forged shared-wire observation was accepted")
			}
		})
	}

	spec := successfulObservationSpec(t, profile)
	for index := range spec.Dependencies {
		record := spec.Dependencies[index].View.Record()
		record.PhysicalOffset--
		record.PhysicalWordCount += 2
		record.SliceOffset++
		spec.Dependencies[index].View = snapshotFromRecord(t, record)
	}
	if _, err := buildObservation(t, profile, spec); err != nil {
		t.Fatalf("legitimate wider coalesced response was rejected: %v", err)
	}
}

func TestRound1RTUAndTCPLogicalViewInvariants(t *testing.T) {
	rtu := logicalViewRecord(2001, 100, 0, []uint16{1, 2})
	rtu.Transport = reg.TransportRTU
	rtu.ConnectionID = 0
	rtu.PhysicalWordCount = rtu.LogicalWordCount
	if _, err := reg.NewLogicalViewSnapshot(rtu); err != nil {
		t.Fatalf("valid pinned-runtime RTU snapshot rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*reg.LogicalViewRecord)
	}{
		{"RTU connection identity", func(record *reg.LogicalViewRecord) {
			record.ConnectionID = 1
		}},
		{"RTU physical range", func(record *reg.LogicalViewRecord) {
			record.PhysicalWordCount++
		}},
		{"RTU physical offset", func(record *reg.LogicalViewRecord) {
			record.PhysicalOffset--
			record.SliceOffset = 1
		}},
		{"RTU slice offset", func(record *reg.LogicalViewRecord) {
			record.SliceOffset = 1
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := rtu
			test.mutate(&record)
			if _, err := reg.NewLogicalViewSnapshot(record); err == nil {
				t.Fatal("forged RTU snapshot was accepted")
			}
		})
	}

	tcp := logicalViewRecord(2002, 100, 0, []uint16{1, 2})
	tcp.ConnectionID = 0
	if _, err := reg.NewLogicalViewSnapshot(tcp); err == nil {
		t.Fatal("TCP snapshot without connection identity was accepted")
	}

	base := profileFixture(t)
	dependencies := base.Dependencies().Dependencies()
	secondSpec := dependencies[1].Spec()
	secondSpec.Normalization = normalizationSpec(t, 101)
	second, err := reg.NewDependency(secondSpec)
	if err != nil {
		t.Fatalf("NewDependency(second same range): %v", err)
	}
	set, err := reg.NewDependencySet(
		base.Dependencies().Version(),
		[]reg.Dependency{dependencies[0], second},
	)
	if err != nil {
		t.Fatalf("NewDependencySet(RTU duplicate range): %v", err)
	}
	profileSpec := base.Spec()
	profileSpec.Dependencies = set
	profile, err := reg.NewProfileDescriptor(profileSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(RTU duplicate range): %v", err)
	}
	observationSpec := successfulObservationSpec(t, profile)
	for index := range observationSpec.Dependencies {
		record := observationSpec.Dependencies[index].View.Record()
		record.Transport = reg.TransportRTU
		record.ConnectionID = 0
		record.PhysicalOffset = record.LogicalOffset
		record.PhysicalWordCount = record.LogicalWordCount
		record.SliceOffset = 0
		observationSpec.Dependencies[index].View = snapshotFromRecord(t, record)
	}
	if _, err := buildObservation(t, profile, observationSpec); err == nil {
		t.Fatal("one RTU response produced multiple logical views")
	}
}

func boundedFixture(
	t *testing.T,
	order reg.AcquisitionOrder,
) (reg.ProfileDescriptor, reg.ObservationSpec) {
	t.Helper()
	base := profileFixture(t)
	profileSpec := base.Spec()
	profileSpec.Coherence = reg.CoherencePolicySpec{
		Version:                      base.CoherenceVersion(),
		Mode:                         reg.CoherenceBoundedMultiResponse,
		MaximumSourceSkew:            2 * time.Second,
		MaximumReceiptSkew:           3 * time.Second,
		RequireGenerationEquality:    true,
		AcquisitionOrder:             order,
		RetrySetBehavior:             reg.RetryWholeSet,
		DocumentaryConsistencyMarker: "sequence-7",
	}
	profile, err := reg.NewProfileDescriptor(profileSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor: %v", err)
	}
	spec := successfulObservationSpec(t, profile)
	spec.RetryOrdinal = 1
	source := time.Unix(1_700_000_100, 0).UTC()
	receipt := source.Add(time.Second)
	for index := range spec.Dependencies {
		record := spec.Dependencies[index].View.Record()
		record.WireResponseID += uint64(index)
		record.PhysicalRequestID += uint64(index)
		spec.Dependencies[index].View = snapshotFromRecord(t, record)
		spec.Dependencies[index].SourceTime = reg.SourceTimeObserved(
			source.Add(time.Duration(index) * time.Second),
		)
		spec.Dependencies[index].LocalReceiptTime = receipt.Add(
			time.Duration(index) * time.Second,
		)
		spec.Dependencies[index].DocumentaryConsistencyMarker = "sequence-7"
		spec.Dependencies[index].AcquisitionOrdinal = uint32(index + 1)
		spec.Dependencies[index].RetryOrdinal = spec.RetryOrdinal
	}
	spec.SourceTime = reg.SourceTimeObserved(source.Add(time.Second))
	spec.LocalReceiptTime = receipt.Add(time.Second)
	return profile, spec
}

func TestRound1CoherenceDeclarationIsExplicit(t *testing.T) {
	base := profileFixture(t)
	spec := base.Spec()
	spec.Coherence.RetrySetBehavior = ""
	if _, err := reg.NewProfileDescriptor(spec); err == nil {
		t.Fatal("single-wire profile omitted explicit retry behavior")
	}
	spec = base.Spec()
	spec.Coherence.AcquisitionOrder = ""
	if _, err := reg.NewProfileDescriptor(spec); err == nil {
		t.Fatal("single-wire profile omitted explicit acquisition order")
	}

	spec = base.Spec()
	spec.Coherence = reg.CoherencePolicySpec{
		Version:                   base.CoherenceVersion(),
		Mode:                      reg.CoherenceBoundedMultiResponse,
		MaximumSourceSkew:         time.Second,
		MaximumReceiptSkew:        time.Second,
		RequireGenerationEquality: true,
		RetrySetBehavior:          reg.RetryWholeSet,
	}
	if _, err := reg.NewProfileDescriptor(spec); err == nil {
		t.Fatal("bounded profile omitted explicit acquisition order")
	}

	spec.Coherence.AcquisitionOrder = reg.AcquisitionOrderDependencyDeclaration
	spec.Coherence.RequireGenerationEquality = false
	if _, err := reg.NewProfileDescriptor(spec); err == nil {
		t.Fatal("bounded profile omitted required generation equality")
	}
}

func TestRound1BoundedMultiResponseRejectsMixedSourcesAndOrder(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*reg.ObservationSpec)
	}{
		{
			name: "mixed transport generation",
			mutate: func(spec *reg.ObservationSpec) {
				record := spec.Dependencies[1].View.Record()
				record.TransportGeneration++
				spec.Dependencies[1].View = snapshotFromRecord(t, record)
			},
		},
		{
			name: "mixed transport family",
			mutate: func(spec *reg.ObservationSpec) {
				record := spec.Dependencies[1].View.Record()
				record.Transport = reg.TransportRTU
				record.ConnectionID = 0
				record.PhysicalOffset = record.LogicalOffset
				record.PhysicalWordCount = record.LogicalWordCount
				record.SliceOffset = 0
				spec.Dependencies[1].View = snapshotFromRecord(t, record)
			},
		},
		{
			name: "mixed endpoint",
			mutate: func(spec *reg.ObservationSpec) {
				record := spec.Dependencies[1].View.Record()
				record.Endpoint = "fixture://endpoint-b"
				spec.Dependencies[1].View = snapshotFromRecord(t, record)
			},
		},
		{
			name: "mixed unit",
			mutate: func(spec *reg.ObservationSpec) {
				record := spec.Dependencies[1].View.Record()
				record.UnitID++
				spec.Dependencies[1].View = snapshotFromRecord(t, record)
			},
		},
		{
			name: "reversed declared acquisition",
			mutate: func(spec *reg.ObservationSpec) {
				spec.Dependencies[0].AcquisitionOrdinal = 2
				spec.Dependencies[1].AcquisitionOrdinal = 1
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile, spec := boundedFixture(
				t,
				reg.AcquisitionOrderDependencyDeclaration,
			)
			test.mutate(&spec)
			if _, err := buildObservation(t, profile, spec); err == nil {
				t.Fatal("incoherent bounded response was accepted")
			}
		})
	}

	profile, spec := boundedFixture(t, reg.AcquisitionOrderSourceTimeAscending)
	firstTime := spec.Dependencies[0].SourceTime.Time
	spec.Dependencies[0].SourceTime = reg.SourceTimeObserved(
		spec.Dependencies[1].SourceTime.Time.Add(time.Second),
	)
	spec.SourceTime = spec.Dependencies[0].SourceTime
	if _, err := buildObservation(t, profile, spec); err == nil {
		t.Fatal("reversed source-time acquisition order was accepted")
	}
	spec.Dependencies[0].SourceTime = reg.SourceTimeObserved(firstTime)

	profile, spec = boundedFixture(t, reg.AcquisitionOrderReceiptTimeAscending)
	spec.Dependencies[0].LocalReceiptTime =
		spec.Dependencies[1].LocalReceiptTime.Add(time.Second)
	spec.LocalReceiptTime = spec.Dependencies[0].LocalReceiptTime
	if _, err := buildObservation(t, profile, spec); err == nil {
		t.Fatal("reversed receipt-time acquisition order was accepted")
	}
}

func TestRound1CodecAndDependencyBounds(t *testing.T) {
	for _, rawWords := range []uint16{126, 32768} {
		spec := numericCodecSpec(t)
		spec.RawWordCount = rawWords
		spec.WordPermutation = make([]uint16, rawWords)
		for index := range spec.WordPermutation {
			spec.WordPermutation[index] = uint16(index)
		}
		spec.Sentinels = nil
		if _, err := reg.NewCodec(spec); err == nil {
			t.Fatalf("codec raw width %d was accepted", rawWords)
		}

		dependency := dependencySpec(t, "oversized", 0)
		dependency.WordCount = rawWords
		if _, err := reg.NewDependency(dependency); err == nil {
			t.Fatalf("dependency word width %d was accepted", rawWords)
		}
	}

	padding := byte(0)
	spec := numericCodecSpec(t)
	spec.RawWordCount = 32768
	spec.WordPermutation = make([]uint16, spec.RawWordCount)
	for index := range spec.WordPermutation {
		spec.WordPermutation[index] = uint16(index)
	}
	spec.Representation = reg.RepresentationString
	spec.Scale = reg.ScaleSpec{
		Source:           reg.ScaleNotApplicable,
		ApplicationOrder: reg.ScaleOrderNotApplicable,
	}
	spec.Sentinels = nil
	spec.String = reg.StringSpec{
		Applicability:                  reg.StringApplicable,
		WordPacking:                    reg.StringHighByteFirst,
		ByteOrder:                      reg.ByteOrderModbus,
		PaddingByte:                    &padding,
		Termination:                    reg.StringFixedLength,
		RetainedRawLength:              65536,
		DocumentaryCharacterRepertoire: "ASCII",
	}
	if _, err := reg.NewCodec(spec); err == nil {
		t.Fatal("string codec accepted a widened retained-length overflow case")
	}
}

func qualifiedBaseProfile(t *testing.T) reg.ProfileDescriptor {
	t.Helper()
	spec := profileFixture(t).Spec()
	spec.Maturity = reg.MaturityQualified
	profile, err := reg.NewProfileDescriptor(spec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(qualified base): %v", err)
	}
	return profile
}

func overlayProfile(
	t *testing.T,
	base reg.ProfileDescriptor,
	baseVersion reg.Version,
) reg.ProfileDescriptor {
	t.Helper()
	spec := base.Spec()
	spec.ID = "example.vendor.overlay"
	spec.Kind = reg.ProfileVendorOverlay
	spec.StandardApplicability = nil
	spec.ModelApplicability = nil
	spec.KnownExclusions = nil
	spec.Codecs = nil
	spec.Dependencies = reg.DependencySet{}
	spec.Coherence = reg.CoherencePolicySpec{}
	spec.VendorApplicability = []string{"example-vendor"}
	spec.Evidence = []reg.EvidenceReference{{
		ID:                     "overlay-evidence",
		PublicationDisposition: reg.PublicationMetadataOnly,
	}}
	spec.RefinesProfileID = base.ID()
	spec.RefinesProfileVersion = baseVersion
	spec.Maturity = reg.MaturityExperimental
	spec.OverlayDeltas = []reg.VendorOverlayDeltaSpec{{
		ID:                 "model-delta",
		Version:            version(t, "1.0.0"),
		Kind:               reg.OverlayDeltaModelApplicability,
		Operation:          reg.OverlayDeltaAdd,
		ApplicabilityValue: "vendor-model",
		EvidenceReferences: []string{"overlay-evidence"},
	}}
	profile, err := reg.NewProfileDescriptor(spec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(overlay): %v", err)
	}
	return profile
}

func TestRound1CatalogValidatesOverlayAndSupersessionGraph(t *testing.T) {
	base := qualifiedBaseProfile(t)
	overlay := overlayProfile(t, base, base.Version())
	if _, err := reg.NewCatalog(overlay); err == nil {
		t.Fatal("overlay without in-catalog base was accepted")
	}
	if _, err := reg.NewCatalog(base, overlay); err != nil {
		t.Fatalf("valid qualified overlay graph rejected: %v", err)
	}

	wrongVersion := overlayProfile(t, base, version(t, "9.0.0"))
	if _, err := reg.NewCatalog(base, wrongVersion); err == nil {
		t.Fatal("overlay with mismatched base version was accepted")
	}
	unqualified := profileFixture(t)
	unqualifiedOverlay := overlayProfile(t, unqualified, unqualified.Version())
	if _, err := reg.NewCatalog(unqualified, unqualifiedOverlay); err == nil {
		t.Fatal("overlay with unqualified base was accepted")
	}
	inactiveSpec := base.Spec()
	inactiveSpec.State = reg.ProfileRevoked
	inactiveSpec.DefaultEnabled = false
	inactive, err := reg.NewProfileDescriptor(inactiveSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(inactive base): %v", err)
	}
	inactiveOverlay := overlayProfile(t, inactive, inactive.Version())
	if _, err := reg.NewCatalog(inactive, inactiveOverlay); err == nil {
		t.Fatal("overlay with inactive base was accepted")
	}

	replacementSpec := base.Spec()
	replacementSpec.ID = "example.standard.energy.v2"
	replacementSpec.Version = version(t, "2.0.0")
	replacement, err := reg.NewProfileDescriptor(replacementSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(replacement): %v", err)
	}
	legacySpec := base.Spec()
	legacySpec.ID = "example.standard.energy.legacy"
	legacySpec.State = reg.ProfileSuperseded
	legacySpec.SupersededByID = replacement.ID()
	legacySpec.SupersededByVersion = replacement.Version()
	legacy, err := reg.NewProfileDescriptor(legacySpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(legacy): %v", err)
	}
	if _, err := reg.NewCatalog(legacy); err == nil {
		t.Fatal("superseded profile without target was accepted")
	}
	if _, err := reg.NewCatalog(legacy, replacement); err != nil {
		t.Fatalf("valid supersession graph rejected: %v", err)
	}
	wrongTargetVersionSpec := legacy.Spec()
	wrongTargetVersionSpec.SupersededByVersion = version(t, "9.0.0")
	wrongTargetVersion, err := reg.NewProfileDescriptor(wrongTargetVersionSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(wrong target version): %v", err)
	}
	if _, err := reg.NewCatalog(wrongTargetVersion, replacement); err == nil {
		t.Fatal("supersession with mismatched target version was accepted")
	}

	incompatibleTarget := overlayProfile(t, base, base.Version())
	incompatibleSpec := legacy.Spec()
	incompatibleSpec.SupersededByID = incompatibleTarget.ID()
	incompatibleSpec.SupersededByVersion = incompatibleTarget.Version()
	incompatible, err := reg.NewProfileDescriptor(incompatibleSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(incompatible target): %v", err)
	}
	if _, err := reg.NewCatalog(base, incompatible, incompatibleTarget); err == nil {
		t.Fatal("supersession with incompatible target kind was accepted")
	}

	selfSpec := base.Spec()
	selfSpec.State = reg.ProfileSuperseded
	selfSpec.SupersededByID = selfSpec.ID
	selfSpec.SupersededByVersion = selfSpec.Version
	self, err := reg.NewProfileDescriptor(selfSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(self supersession): %v", err)
	}
	if _, err := reg.NewCatalog(self); err == nil {
		t.Fatal("self-supersession was accepted")
	}
}

func TestRound1SingleWireRejectsImpossibleDependencySets(t *testing.T) {
	base := profileFixture(t)
	dependencies := base.Dependencies().Dependencies()

	holdingSpec := dependencySpec(t, "holding", 100)
	holdingSpec.Table = reg.HoldingRegisters
	holdingSpec.Normalization.AddressSpaceLabel = string(reg.HoldingRegisters)
	holding, err := reg.NewDependency(holdingSpec)
	if err != nil {
		t.Fatalf("NewDependency(holding): %v", err)
	}
	mixed, err := reg.NewDependencySet(
		base.Dependencies().Version(),
		[]reg.Dependency{dependencies[0], holding},
	)
	if err != nil {
		t.Fatalf("NewDependencySet(mixed): %v", err)
	}
	spec := base.Spec()
	spec.Dependencies = mixed
	if _, err := reg.NewProfileDescriptor(spec); err == nil {
		t.Fatal("single-wire FC03/FC04 dependency set was accepted")
	}

	farSpec := dependencySpec(t, "far", 224)
	far, err := reg.NewDependency(farSpec)
	if err != nil {
		t.Fatalf("NewDependency(far): %v", err)
	}
	oversized, err := reg.NewDependencySet(
		base.Dependencies().Version(),
		[]reg.Dependency{dependencies[0], far},
	)
	if err != nil {
		t.Fatalf("NewDependencySet(oversized): %v", err)
	}
	spec = base.Spec()
	spec.Dependencies = oversized
	if _, err := reg.NewProfileDescriptor(spec); err == nil {
		t.Fatal("single-wire oversized physical union was accepted")
	}
}

func TestRound1ProfileAndObservationSerializationIsLossless(t *testing.T) {
	profile := profileFixture(t)
	profileBytes, err := reg.MarshalProfileDescriptor(profile)
	if err != nil {
		t.Fatalf("MarshalProfileDescriptor: %v", err)
	}
	decodedProfile, err := reg.UnmarshalProfileDescriptor(profileBytes)
	if err != nil {
		t.Fatalf("UnmarshalProfileDescriptor: %v", err)
	}
	reencodedProfile, err := reg.MarshalProfileDescriptor(decodedProfile)
	if err != nil {
		t.Fatalf("MarshalProfileDescriptor(round trip): %v", err)
	}
	if !bytes.Equal(profileBytes, reencodedProfile) {
		t.Fatal("profile serialization is not byte-stable")
	}
	profileSpecBytes, err := json.Marshal(profile.Spec())
	if err != nil {
		t.Fatalf("json.Marshal(ProfileDescriptorSpec): %v", err)
	}
	if !bytes.Equal(profileBytes, profileSpecBytes) {
		t.Fatal("profile construction spec uses a different serialized contract")
	}
	var decodedProfileSpec reg.ProfileDescriptorSpec
	if err := json.Unmarshal(profileSpecBytes, &decodedProfileSpec); err != nil {
		t.Fatalf("json.Unmarshal(ProfileDescriptorSpec): %v", err)
	}
	if bytes.Contains(profileBytes, []byte(`"refines_profile_version":"1.0.0"`)) ||
		bytes.Contains(profileBytes, []byte(`"superseded_by_version":"1.0.0"`)) {
		t.Fatal("zero optional versions silently became schema versions")
	}
	badProfile := bytes.Replace(
		profileBytes,
		[]byte(`"schema_version":"1.0.0"`),
		[]byte(`"schema_version":"9.0.0"`),
		1,
	)
	if _, err := reg.UnmarshalProfileDescriptor(badProfile); err == nil {
		t.Fatal("unknown profile schema was accepted")
	}
	unknownProfileField := append(
		append([]byte(nil), profileBytes[:len(profileBytes)-1]...),
		[]byte(`,"unknown_contract_field":true}`)...,
	)
	if _, err := reg.UnmarshalProfileDescriptor(unknownProfileField); err == nil {
		t.Fatal("unknown profile field was accepted")
	}

	factory, _ := newFactory(t, profile, emptyLedgerState(t, profile))
	observation, err := publishWithFactory(
		factory,
		successfulObservationSpec(t, profile),
	)
	if err != nil {
		t.Fatalf("NewObservation: %v", err)
	}
	observationBytes, err := reg.MarshalFixtureSpec(observation.Spec())
	if err != nil {
		t.Fatalf("MarshalFixtureSpec: %v", err)
	}
	replayRecord := observation.Replay()[0].LogicalViewRecord()
	freshFactory, _ := newFactory(t, profile, emptyLedgerState(t, profile))
	freshAttempt, err := freshFactory.BeginObservationAttempt(reg.AttemptIdentity{
		PollGenerationID: observation.Spec().PollGenerationID,
		RetryOrdinal:     observation.Spec().RetryOrdinal,
	})
	if err != nil {
		t.Fatalf("BeginObservationAttempt: %v", err)
	}
	decodedObservationSpec, err := freshAttempt.DecodeSpec(observationBytes)
	if err != nil {
		t.Fatalf("DecodeSpec: %v", err)
	}
	reencodedObservation, err := freshAttempt.MarshalSpec(decodedObservationSpec)
	if err != nil {
		t.Fatalf("MarshalObservation(round trip): %v", err)
	}
	if !bytes.Equal(observationBytes, reencodedObservation) {
		t.Fatal("observation serialization is not byte-stable")
	}
	observationSpecBytes, err := json.Marshal(observation.Spec())
	if err != nil {
		t.Fatalf("json.Marshal(ObservationSpec): %v", err)
	}
	if !bytes.Equal(observationBytes, observationSpecBytes) {
		t.Fatal("observation construction spec lost its logical-view snapshot")
	}
	var JSONDecodedObservationSpec reg.ObservationSpec
	if err := json.Unmarshal(
		observationSpecBytes,
		&JSONDecodedObservationSpec,
	); err != nil {
		t.Fatalf("json.Unmarshal(ObservationSpec): %v", err)
	}
	JSONDecodedObservationSpec.SampleID = ""
	specFactory, _ := newFactory(t, profile, emptyLedgerState(t, profile))
	if _, err := publishWithFactory(
		specFactory,
		JSONDecodedObservationSpec,
	); err != nil {
		t.Fatalf("strict fixture JSON replay failed: %v", err)
	}
	if !reflect.DeepEqual(
		replayRecord,
		decodedObservationSpec.Dependencies[0].View.Record(),
	) {
		t.Fatal("observation round trip lost raw words or provenance")
	}
	badObservation := bytes.Replace(
		observationBytes,
		[]byte(`"schema_version":"1.0.0"`),
		[]byte(`"schema_version":"9.0.0"`),
		1,
	)
	thirdFactory, _ := newFactory(t, profile, emptyLedgerState(t, profile))
	thirdAttempt, _ := thirdFactory.BeginObservationAttempt(reg.AttemptIdentity{
		PollGenerationID: observation.Spec().PollGenerationID,
		RetryOrdinal:     observation.Spec().RetryOrdinal,
	})
	if _, err := thirdAttempt.DecodeSpec(badObservation); err == nil {
		t.Fatal("unknown observation schema was accepted")
	}
	unknownObservationField := append(
		append([]byte(nil), observationBytes[:len(observationBytes)-1]...),
		[]byte(`,"unknown_contract_field":true}`)...,
	)
	fourthFactory, _ := newFactory(t, profile, emptyLedgerState(t, profile))
	fourthAttempt, _ := fourthFactory.BeginObservationAttempt(reg.AttemptIdentity{
		PollGenerationID: observation.Spec().PollGenerationID,
		RetryOrdinal:     observation.Spec().RetryOrdinal,
	})
	if _, err := fourthAttempt.DecodeSpec(
		unknownObservationField,
	); err == nil {
		t.Fatal("unknown observation field was accepted")
	}
}

func TestRound1SchemaAuthoritiesAreReadOnlyValues(t *testing.T) {
	if reg.CurrentSchemaVersion().String() != "1.0.0" ||
		reg.CurrentCodecContractVersion().String() != "1.0.0" {
		t.Fatal("schema authority accessor returned the wrong value")
	}
	original := reg.CurrentSchemaVersion()
	replacement := version(t, "9.0.0")
	if original == replacement || reg.CurrentSchemaVersion() != original {
		t.Fatal("schema authority did not remain immutable")
	}
}

func TestFixtureReplayDoesNotAdvanceProductionLedger(t *testing.T) {
	profile := profileFixture(t)
	factory, ledger := newFactory(t, profile, emptyLedgerState(t, profile))
	replay, err := publishWithFactory(factory, successfulObservationSpec(t, profile))
	if err != nil {
		t.Fatalf("fixture replay: %v", err)
	}
	if replay.FixtureID() == "" || replay.Spec().SampleID != "" ||
		ledger.ExportState().Revision != 0 {
		t.Fatal("fixture replay acquired production identity or advanced the ledger")
	}
}

func TestRound1ReplayExposesFullImmutableProvenance(t *testing.T) {
	profile := profileFixture(t)
	spec := successfulObservationSpec(t, profile)
	observation, err := buildObservation(t, profile, spec)
	if err != nil {
		t.Fatalf("NewObservation: %v", err)
	}
	want := spec.Dependencies[0].View.Record()
	got := observation.Replay()[0].LogicalViewRecord()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("replay record differs:\ngot  %#v\nwant %#v", got, want)
	}
	got.Words[0] = 0xffff
	if reflect.DeepEqual(got, observation.Replay()[0].LogicalViewRecord()) {
		t.Fatal("replay provenance exposed mutable raw words")
	}
}
