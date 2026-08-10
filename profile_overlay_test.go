package modbusreg_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func fixtureFactoryWithLedger(
	t *testing.T,
	profile reg.ProfileDescriptor,
	state reg.SampleLedgerState,
	trustedMinimumRevision uint64,
) (*fixtureValidationFactory, *reg.SampleLedger) {
	t.Helper()
	ledger, err := reg.NewSampleLedger(state, trustedMinimumRevision)
	if err != nil {
		t.Fatalf("NewSampleLedger: %v", err)
	}
	return newFixtureValidationFactory(t, profile), ledger
}

func emptyFixtureLedgerState(
	t *testing.T,
	profile reg.ProfileDescriptor,
) reg.SampleLedgerState {
	t.Helper()
	state, err := reg.EmptySampleLedgerState(
		"fixture-issuer",
		profile,
	)
	if err != nil {
		t.Fatalf("EmptySampleLedgerState: %v", err)
	}
	return state
}

func validateFixtureSpec(
	t *testing.T,
	profile reg.ProfileDescriptor,
	spec reg.ObservationSpec,
) (reg.FixtureReplay, error) {
	t.Helper()
	factory, _ := fixtureFactoryWithLedger(
		t,
		profile,
		emptyFixtureLedgerState(t, profile),
		0,
	)
	return replayWithFactory(factory, spec)
}

func TestRetrySetIdentityRejectsMixedAttempts(t *testing.T) {
	profile, validSpec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	if _, err := validateFixtureSpec(t, profile, validSpec); err != nil {
		t.Fatalf("valid whole-set retry identity rejected: %v", err)
	}
	_, firstSpec := boundedFixture(t, reg.AcquisitionOrderDependencyDeclaration)
	_, secondSpec := boundedFixture(t, reg.AcquisitionOrderDependencyDeclaration)
	secondSpec.RetryOrdinal = firstSpec.RetryOrdinal + 1
	for index := range secondSpec.Dependencies {
		secondSpec.Dependencies[index].RetryOrdinal = secondSpec.RetryOrdinal
	}

	state := emptyFixtureLedgerState(t, profile)
	factory, _ := fixtureFactoryWithLedger(t, profile, state, 0)
	first, err := factory.BeginFixtureReplay(reg.AttemptIdentity{
		PollGenerationID: firstSpec.PollGenerationID,
		RetryOrdinal:     firstSpec.RetryOrdinal,
	})
	if err != nil {
		t.Fatalf("BeginFixtureReplay(first): %v", err)
	}
	second, err := factory.BeginFixtureReplay(reg.AttemptIdentity{
		PollGenerationID: secondSpec.PollGenerationID,
		RetryOrdinal:     secondSpec.RetryOrdinal,
	})
	if err != nil {
		t.Fatalf("BeginFixtureReplay(second): %v", err)
	}
	firstBytes, err := reg.MarshalFixtureSpec(firstSpec)
	if err != nil {
		t.Fatalf("MarshalFixtureSpec(first): %v", err)
	}
	firstSpec, err = first.DecodeSpec(firstBytes)
	if err != nil {
		t.Fatalf("DecodeSpec(first): %v", err)
	}
	secondBytes, err := reg.MarshalFixtureSpec(secondSpec)
	if err != nil {
		t.Fatalf("MarshalFixtureSpec(second): %v", err)
	}
	secondSpec, err = second.DecodeSpec(secondBytes)
	if err != nil {
		t.Fatalf("DecodeSpec(second): %v", err)
	}
	mixed := firstSpec
	mixed.Dependencies[1] = secondSpec.Dependencies[1]
	if _, err := first.Replay(mixed); err == nil {
		t.Fatal("mixed retry-set observation was accepted")
	}

	single := profileFixture(t)
	singleSpec := successfulObservationSpec(t, single)
	encoded, err := reg.MarshalFixtureSpec(singleSpec)
	if err != nil {
		t.Fatalf("MarshalFixtureSpec(single): %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(encoded, &record); err != nil {
		t.Fatalf("json.Unmarshal(single): %v", err)
	}
	if _, exists := record["retry_attempt_token"]; exists ||
		record["retry_ordinal"] != float64(0) {
		t.Fatal("single-wire serialization carried a retry token or ordinal")
	}
}

func makeRepeatedWireGroup(
	t *testing.T,
	spec *reg.ObservationSpec,
) {
	t.Helper()
	first := spec.Dependencies[0].View.Record()
	second := spec.Dependencies[1].View.Record()
	second.WireResponseID = first.WireResponseID
	second.PhysicalRequestID = first.PhysicalRequestID
	second.Endpoint = first.Endpoint
	second.ConnectionID = first.ConnectionID
	second.Transport = first.Transport
	second.TransportGeneration = first.TransportGeneration
	second.UnitID = first.UnitID
	second.RequestedFunction = first.RequestedFunction
	second.ReceivedFunction = first.ReceivedFunction
	second.Table = first.Table
	second.PhysicalOffset = first.PhysicalOffset
	second.PhysicalWordCount = first.PhysicalWordCount
	second.AuthorizationScope = first.AuthorizationScope
	second.PollGeneration = first.PollGeneration
	second.DeadlineIdentity = first.DeadlineIdentity
	spec.Dependencies[1].View = snapshotFromRecord(t, second)
	spec.Dependencies[1].SourceTime = spec.Dependencies[0].SourceTime
	spec.Dependencies[1].LocalReceiptTime =
		spec.Dependencies[0].LocalReceiptTime
	spec.Dependencies[1].DocumentaryConsistencyMarker =
		spec.Dependencies[0].DocumentaryConsistencyMarker
	spec.Dependencies[1].AcquisitionOrdinal =
		spec.Dependencies[0].AcquisitionOrdinal
	spec.SourceTime = spec.Dependencies[0].SourceTime
	spec.LocalReceiptTime = spec.Dependencies[0].LocalReceiptTime
}

func TestWireGroupingAppliesToEveryCoherenceMode(t *testing.T) {
	profile, valid := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	makeRepeatedWireGroup(t, &valid)
	if _, err := validateFixtureSpec(t, profile, valid); err != nil {
		t.Fatalf("compatible repeated wire group rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*reg.LogicalViewRecord)
	}{
		{"physical request", func(record *reg.LogicalViewRecord) {
			record.PhysicalRequestID++
		}},
		{"endpoint", func(record *reg.LogicalViewRecord) {
			record.Endpoint = "tcp://forged.example:502"
		}},
		{"connection", func(record *reg.LogicalViewRecord) {
			record.ConnectionID++
		}},
		{"transport", func(record *reg.LogicalViewRecord) {
			record.Transport = reg.TransportRTU
			record.ConnectionID = 0
			record.PhysicalOffset = record.LogicalOffset
			record.PhysicalWordCount = record.LogicalWordCount
			record.SliceOffset = 0
		}},
		{"transport generation", func(record *reg.LogicalViewRecord) {
			record.TransportGeneration++
		}},
		{"unit", func(record *reg.LogicalViewRecord) {
			record.UnitID++
		}},
		{"function and table", func(record *reg.LogicalViewRecord) {
			if record.Table == reg.HoldingRegisters {
				record.RequestedFunction = reg.FunctionReadInputRegisters
				record.ReceivedFunction = reg.FunctionReadInputRegisters
				record.Table = reg.InputRegisters
			} else {
				record.RequestedFunction = reg.FunctionReadHoldingRegisters
				record.ReceivedFunction = reg.FunctionReadHoldingRegisters
				record.Table = reg.HoldingRegisters
			}
		}},
		{"authorization scope", func(record *reg.LogicalViewRecord) {
			record.AuthorizationScope = "forged-scope"
		}},
		{"poll generation", func(record *reg.LogicalViewRecord) {
			record.PollGeneration++
		}},
		{"deadline identity", func(record *reg.LogicalViewRecord) {
			record.DeadlineIdentity++
		}},
		{"physical range", func(record *reg.LogicalViewRecord) {
			record.PhysicalWordCount++
		}},
		{"contradictory overlap", func(record *reg.LogicalViewRecord) {
			record.Words[0] ^= 0xffff
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, candidate := boundedFixture(
				t,
				reg.AcquisitionOrderDependencyDeclaration,
			)
			makeRepeatedWireGroup(t, &candidate)
			record := candidate.Dependencies[1].View.Record()
			test.mutate(&record)
			candidate.Dependencies[1].View = snapshotFromRecord(t, record)
			if _, err := validateFixtureSpec(t, profile, candidate); err == nil {
				t.Fatal("incompatible WireResponseID reuse was accepted")
			}
		})
	}
}

func overlayDeltaProfile(
	t *testing.T,
	base reg.ProfileDescriptor,
	deltas []reg.VendorOverlayDeltaSpec,
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
	spec.Maturity = reg.MaturityExperimental
	spec.DefaultEnabled = false
	spec.State = reg.ProfileActive
	spec.RefinesProfileID = base.ID()
	spec.RefinesProfileVersion = base.Version()
	spec.OverlayDeltas = deltas
	profile, err := reg.NewProfileDescriptor(spec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(overlay delta): %v", err)
	}
	return profile
}

func modelDelta(t *testing.T, value string) reg.VendorOverlayDeltaSpec {
	t.Helper()
	return reg.VendorOverlayDeltaSpec{
		ID:                 "model-delta",
		Version:            version(t, "1.0.0"),
		Kind:               reg.OverlayDeltaModelApplicability,
		Operation:          reg.OverlayDeltaAdd,
		ApplicabilityValue: value,
		EvidenceReferences: []string{"overlay-evidence"},
	}
}

func TestVendorOverlayIsOnlyEvidenceBackedDelta(t *testing.T) {
	base := qualifiedBaseProfile(t)
	clone := base.Spec()
	clone.ID = "example.vendor.clone"
	clone.Kind = reg.ProfileVendorOverlay
	clone.VendorApplicability = []string{"example-vendor"}
	clone.RefinesProfileID = base.ID()
	clone.RefinesProfileVersion = base.Version()
	if _, err := reg.NewProfileDescriptor(clone); err == nil {
		t.Fatal("full copied vendor profile was accepted")
	}

	missingEvidence := modelDelta(t, "vendor-model")
	missingEvidence.EvidenceReferences = nil
	spec := base.Spec()
	spec.ID = "example.vendor.no-evidence"
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
	spec.RefinesProfileVersion = base.Version()
	spec.OverlayDeltas = []reg.VendorOverlayDeltaSpec{missingEvidence}
	if _, err := reg.NewProfileDescriptor(spec); err == nil {
		t.Fatal("overlay delta without evidence was accepted")
	}

	overlay := overlayDeltaProfile(
		t,
		base,
		[]reg.VendorOverlayDeltaSpec{modelDelta(t, "vendor-model")},
	)
	if _, err := reg.NewCatalog(base, overlay); err != nil {
		t.Fatalf("real evidence-backed delta rejected: %v", err)
	}
	overlayBytes, err := reg.MarshalProfileDescriptor(overlay)
	if err != nil {
		t.Fatalf("MarshalProfileDescriptor(overlay): %v", err)
	}
	decodedOverlay, err := reg.UnmarshalProfileDescriptor(overlayBytes)
	if err != nil {
		t.Fatalf("UnmarshalProfileDescriptor(overlay): %v", err)
	}
	reencodedOverlay, err := reg.MarshalProfileDescriptor(decodedOverlay)
	if err != nil {
		t.Fatalf("MarshalProfileDescriptor(decoded overlay): %v", err)
	}
	if !bytes.Equal(overlayBytes, reencodedOverlay) ||
		!reflect.DeepEqual(
			overlay.Spec().OverlayDeltas,
			decodedOverlay.Spec().OverlayDeltas,
		) {
		t.Fatal("overlay delta serialization is not lossless and deterministic")
	}

	noOp := overlayDeltaProfile(
		t,
		base,
		[]reg.VendorOverlayDeltaSpec{modelDelta(t, "model-a")},
	)
	if _, err := reg.NewCatalog(base, noOp); err == nil {
		t.Fatal("overlay no-op against base was accepted")
	}
}

func TestConstructorsAndSerializationAreBounded(t *testing.T) {
	if reg.MaxProfileDependencies != reg.PinnedMaxCoalescedDependents ||
		reg.PinnedMaxCoalescedDependents != 4096 {
		t.Fatal("dependency cap is not the pinned runtime absolute cap")
	}
	if reg.MaxSampleLedgerRecords != 0 {
		t.Fatal("O(1) ledger unexpectedly permits per-sample records")
	}
	overlongIssuer := strings.Repeat(
		"i",
		reg.MaxSampleIssuerDomainBytes+1,
	)
	if _, err := reg.EmptySampleLedgerState(
		overlongIssuer,
		profileFixture(t),
	); err == nil {
		t.Fatal("issuer domain could produce an oversized sample ID")
	}

	codecSpec := numericCodecSpec(t)
	codecSpec.WordPermutation = make(
		[]uint16,
		reg.MaxProfileDependencies+1,
	)
	if _, err := reg.NewCodec(codecSpec); err == nil {
		t.Fatal("oversized permutation was cloned or accepted")
	}

	codecSpec = numericCodecSpec(t)
	codecSpec.Sentinels = make(
		[]reg.RawSentinel,
		reg.MaxCodecSentinels+1,
	)
	if _, err := reg.NewCodec(codecSpec); err == nil {
		t.Fatal("oversized sentinel set was cloned or accepted")
	}

	dependency := dependencySpec(t, "bounded-dependency", 0)
	dependency.EvidenceReferences = make(
		[]string,
		reg.MaxProfileEvidenceReferences+1,
	)
	if _, err := reg.NewDependency(dependency); err == nil {
		t.Fatal("oversized dependency evidence was cloned or accepted")
	}

	if _, err := reg.NewDependencySet(
		version(t, "1.0.0"),
		make([]reg.Dependency, reg.MaxProfileDependencies+1),
	); err == nil {
		t.Fatal("dependency set above pinned runtime cap was accepted")
	}

	profileSpec := profileFixture(t).Spec()
	profileSpec.Codecs = make([]reg.Codec, reg.MaxProfileCodecs+1)
	if _, err := reg.NewProfileDescriptor(profileSpec); err == nil {
		t.Fatal("oversized codec catalog was cloned or accepted")
	}
	profileSpec = profileFixture(t).Spec()
	profileSpec.Evidence = make(
		[]reg.EvidenceReference,
		reg.MaxProfileEvidenceReferences+1,
	)
	if _, err := reg.NewProfileDescriptor(profileSpec); err == nil {
		t.Fatal("oversized profile evidence was cloned or accepted")
	}

	base := qualifiedBaseProfile(t)
	overlay := overlayDeltaProfile(
		t,
		base,
		[]reg.VendorOverlayDeltaSpec{modelDelta(t, "bounded-model")},
	)
	oversizedOverlay := overlay.Spec()
	oversizedCodec := numericCodecSpec(t)
	oversizedCodec.Sentinels = make(
		[]reg.RawSentinel,
		reg.MaxCodecSentinels+1,
	)
	oversizedOverlay.OverlayDeltas = []reg.VendorOverlayDeltaSpec{{
		ID:                 "oversized-codec-delta",
		Version:            version(t, "1.0.0"),
		Kind:               reg.OverlayDeltaCodec,
		Operation:          reg.OverlayDeltaReplace,
		TargetID:           oversizedCodec.ID,
		Codec:              &oversizedCodec,
		EvidenceReferences: []string{"overlay-evidence"},
	}}
	if _, err := reg.NewProfileDescriptor(oversizedOverlay); err == nil {
		t.Fatal("oversized nested overlay codec was cloned or accepted")
	}

	record := logicalViewRecord(
		5001,
		100,
		0,
		make([]uint16, reg.MaxRawWords+1),
	)
	record.LogicalWordCount = uint16(len(record.Words))
	record.SliceWordCount = uint16(len(record.Words))
	record.PhysicalWordCount = uint16(len(record.Words))
	if _, err := reg.NewLogicalViewSnapshot(record); err == nil {
		t.Fatal("oversized raw words were cloned or accepted")
	}

	oversized := bytes.Repeat(
		[]byte(" "),
		reg.MaxSerializedContractBytes+1,
	)
	if _, err := reg.UnmarshalProfileDescriptor(oversized); err == nil {
		t.Fatal("oversized serialized profile was accepted")
	}

	deep := strings.Repeat("[", reg.MaxContractJSONDepth+1) +
		strings.Repeat("]", reg.MaxContractJSONDepth+1)
	if _, err := reg.UnmarshalProfileDescriptor([]byte(deep)); err == nil {
		t.Fatal("over-depth serialized profile was accepted")
	}

	longID := strings.Repeat("a", reg.MaxContractStringBytes+1)
	spec := profileFixture(t).Spec()
	spec.ID = longID
	if _, err := reg.NewProfileDescriptor(spec); err == nil {
		t.Fatal("oversized direct-constructor string was accepted")
	}
}

func TestJSONPreflightRejectsDuplicateAndCaseFoldedKeys(t *testing.T) {
	profileBytes, err := reg.MarshalProfileDescriptor(profileFixture(t))
	if err != nil {
		t.Fatalf("MarshalProfileDescriptor: %v", err)
	}
	tests := [][]byte{
		bytes.Replace(
			profileBytes,
			[]byte(`"schema_version":"1.0.0"`),
			[]byte(
				`"schema_version":"1.0.0","schema_version":"1.0.0"`,
			),
			1,
		),
		bytes.Replace(
			profileBytes,
			[]byte(`"schema_version":"1.0.0"`),
			[]byte(
				`"schema_version":"1.0.0","Schema_Version":"1.0.0"`,
			),
			1,
		),
		bytes.Replace(
			profileBytes,
			[]byte(`"ID":"u32-energy"`),
			[]byte(`"ID":"u32-energy","id":"forged"`),
			1,
		),
	}
	for index, candidate := range tests {
		if _, err := reg.UnmarshalProfileDescriptor(candidate); err == nil {
			t.Fatalf("duplicate/case-fold JSON case %d was accepted", index)
		}
	}
}

func TestAdmissionCanonicalizesTimestamps(t *testing.T) {
	profile := profileFixture(t)
	state := emptyFixtureLedgerState(t, profile)
	factory, ledger := fixtureFactoryWithLedger(t, profile, state, 0)
	spec := successfulObservationSpec(t, profile)
	spec.LocalReceiptTime = time.Date(
		10000,
		time.January,
		1,
		0,
		0,
		0,
		0,
		time.UTC,
	)
	if _, err := replayWithFactory(factory, spec); err == nil {
		t.Fatal("RFC3339-inexpressible year was admitted")
	}
	if ledger.ExportState().HighWater != 0 {
		t.Fatal("failed timestamp validation consumed a sample identity")
	}

	now := time.Now()
	location := time.FixedZone("fixture-offset", 2*60*60)
	spec = successfulObservationSpec(t, profile)
	spec.SourceTime = reg.SourceTimeObserved(now)
	spec.LocalReceiptTime = time.Date(
		2026,
		time.July,
		28,
		22,
		30,
		0,
		123,
		location,
	)
	observation, err := replayWithFactory(factory, spec)
	if err != nil {
		t.Fatalf("canonicalizable timestamp rejected: %v", err)
	}
	got := observation.Spec()
	if got.SourceTime.Time.Location() != time.UTC ||
		got.LocalReceiptTime.Location() != time.UTC ||
		!reflect.DeepEqual(
			got.SourceTime.Time,
			got.SourceTime.Time.Round(0).UTC(),
		) ||
		!reflect.DeepEqual(
			got.LocalReceiptTime,
			got.LocalReceiptTime.Round(0).UTC(),
		) {
		t.Fatal("admitted timestamps retained location or monotonic metadata")
	}
}

func TestSampleLedgerStateRestartValidationIsBounded(t *testing.T) {
	profile := profileFixture(t)
	state := emptyFixtureLedgerState(t, profile)
	const count = 512
	state.Revision = count
	state.HighWater = count
	state.LastCommittedAttempt = reg.AttemptIdentity{PollGenerationID: count}
	encoded, err := reg.MarshalSampleLedgerState(state)
	if err != nil {
		t.Fatalf("MarshalSampleLedgerState: %v", err)
	}
	decoded, err := reg.UnmarshalSampleLedgerState(encoded)
	if err != nil {
		t.Fatalf("UnmarshalSampleLedgerState: %v", err)
	}
	if _, err := reg.NewSampleLedger(decoded, count+1); err == nil {
		t.Fatal("stale ledger restore ignored trusted minimum revision")
	}
	restarted, err := reg.NewSampleLedger(decoded, count)
	if err != nil {
		t.Fatalf("trusted ledger restore rejected: %v", err)
	}
	if restarted.ExportState() != decoded {
		t.Fatal("trusted restart changed persisted sample state")
	}

	badState := decoded
	badState.DependencySetID = "not-a-content-id"
	if _, err := reg.NewSampleLedger(badState, 0); err == nil {
		t.Fatal("malformed dependency-set identity was restored")
	}
	badState.DependencySetID =
		"sha256:" + strings.Repeat("A", 64)
	if _, err := reg.NewSampleLedger(badState, 0); err == nil {
		t.Fatal("uppercase dependency-set identity was restored")
	}
}

func TestRevokedProfileRejectsSuccessorFields(t *testing.T) {
	base := profileFixture(t)
	spec := base.Spec()
	spec.State = reg.ProfileRevoked
	spec.DefaultEnabled = false
	spec.SupersededByID = "example.standard.replacement"
	spec.SupersededByVersion = version(t, "2.0.0")
	if _, err := reg.NewProfileDescriptor(spec); err == nil {
		t.Fatal("revoked profile carried successor fields")
	}
}

func TestSerializationRoundTripsNewFields(t *testing.T) {
	profile, spec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	state := emptyFixtureLedgerState(t, profile)
	factory, ledger := fixtureFactoryWithLedger(t, profile, state, 0)
	encoded, err := reg.MarshalFixtureSpec(spec)
	if err != nil {
		t.Fatalf("MarshalFixtureSpec: %v", err)
	}
	replayAttempt, err := factory.BeginFixtureReplay(reg.AttemptIdentity{
		PollGenerationID: spec.PollGenerationID,
		RetryOrdinal:     spec.RetryOrdinal,
	})
	if err != nil {
		t.Fatalf("BeginFixtureReplay(replay): %v", err)
	}
	decodedSpec, err := replayAttempt.DecodeSpec(encoded)
	if err != nil {
		t.Fatalf("DecodeSpec: %v", err)
	}
	reencoded, err := replayAttempt.MarshalSpec(decodedSpec)
	if err != nil {
		t.Fatalf("MarshalSpec(round trip): %v", err)
	}
	if !bytes.Equal(encoded, reencoded) {
		t.Fatal("new observation fields are not byte-stable")
	}
	var observationRecord map[string]any
	if err := json.Unmarshal(encoded, &observationRecord); err != nil {
		t.Fatalf("json.Unmarshal(observation): %v", err)
	}
	if observationRecord["retry_ordinal"] != float64(1) ||
		bytes.Contains(encoded, []byte(`"retry_attempt_token"`)) {
		t.Fatal("deterministic retry-attempt identity was not preserved")
	}
	if _, err := replayAttempt.Replay(decodedSpec); err != nil {
		t.Fatalf("Replay(decoded): %v", err)
	}

	ledgerBytes, err := reg.MarshalSampleLedgerState(
		ledger.ExportState(),
	)
	if err != nil {
		t.Fatalf("MarshalSampleLedgerState: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(ledgerBytes, &generic); err != nil {
		t.Fatalf("json.Unmarshal(ledger): %v", err)
	}
	for _, field := range []string{
		"issuer_domain",
		"profile_id",
		"profile_version",
		"dependency_set_id",
		"revision",
		"high_water",
	} {
		if _, exists := generic[field]; !exists {
			t.Fatalf("ledger serialization omitted %s", field)
		}
	}
	if strings.Contains(string(ledgerBytes), `"samples"`) {
		t.Fatal("O(1) ledger serialized an unbounded sample collection")
	}
}

func TestLimitErrorsRemainDeterministic(t *testing.T) {
	tooLong := strings.Repeat("z", reg.MaxContractStringBytes+1)
	first := fmt.Sprintf(`{"schema_version":"%s"}`, tooLong)
	second := fmt.Sprintf(`{"schema_version":"%s"}`, tooLong)
	_, firstErr := reg.UnmarshalProfileDescriptor([]byte(first))
	_, secondErr := reg.UnmarshalProfileDescriptor([]byte(second))
	if firstErr == nil || secondErr == nil || firstErr.Error() != secondErr.Error() {
		t.Fatal("bounded JSON rejection is absent or nondeterministic")
	}
}
