package modbusreg_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func round2Factory(
	t *testing.T,
	profile reg.ProfileDescriptor,
	state reg.SampleLedgerState,
	trustedMinimumRevision uint64,
) (*reg.ObservationFactory, *reg.SampleLedger) {
	t.Helper()
	ledger, err := reg.NewSampleLedger(state, trustedMinimumRevision)
	if err != nil {
		t.Fatalf("NewSampleLedger: %v", err)
	}
	factory, err := reg.NewObservationFactory(profile, ledger)
	if err != nil {
		t.Fatalf("NewObservationFactory: %v", err)
	}
	return factory, ledger
}

func round2EmptyLedgerState(
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

func admitRound2(
	t *testing.T,
	profile reg.ProfileDescriptor,
	spec reg.ObservationSpec,
) (reg.SampleAdmission, error) {
	t.Helper()
	factory, _ := round2Factory(
		t,
		profile,
		round2EmptyLedgerState(t, profile),
		0,
	)
	return factory.NewObservation(spec)
}

func setRetryAttempt(spec *reg.ObservationSpec, identity reg.RetryAttemptID) {
	spec.RetryAttemptID = identity
	for index := range spec.Dependencies {
		spec.Dependencies[index].RetryAttemptID = identity
	}
}

func TestRound2RetrySetIdentityRejectsMixedAttempts(t *testing.T) {
	profile, spec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	setRetryAttempt(&spec, 71)
	if _, err := admitRound2(t, profile, spec); err != nil {
		t.Fatalf("valid whole-set retry identity rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*reg.ObservationSpec)
	}{
		{
			name: "missing envelope identity",
			mutate: func(candidate *reg.ObservationSpec) {
				candidate.RetryAttemptID = reg.RetryAttemptNotApplicable
			},
		},
		{
			name: "retained old dependency",
			mutate: func(candidate *reg.ObservationSpec) {
				candidate.Dependencies[0].RetryAttemptID = 70
			},
		},
		{
			name: "missing dependency identity",
			mutate: func(candidate *reg.ObservationSpec) {
				candidate.Dependencies[1].RetryAttemptID =
					reg.RetryAttemptNotApplicable
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, candidate := boundedFixture(
				t,
				reg.AcquisitionOrderDependencyDeclaration,
			)
			setRetryAttempt(&candidate, 71)
			test.mutate(&candidate)
			if _, err := admitRound2(t, profile, candidate); err == nil {
				t.Fatal("mixed retry-set observation was accepted")
			}
		})
	}

	single := profileFixture(t)
	singleSpec := successfulObservationSpec(t, single)
	setRetryAttempt(&singleSpec, reg.RetryAttemptNotApplicable)
	if _, err := admitRound2(t, single, singleSpec); err != nil {
		t.Fatalf("single-wire retry N/A rejected: %v", err)
	}
	singleSpec.RetryAttemptID = 1
	if _, err := admitRound2(t, single, singleSpec); err == nil {
		t.Fatal("single-wire envelope accepted a retry identity")
	}
	singleSpec = successfulObservationSpec(t, single)
	singleSpec.Dependencies[0].RetryAttemptID = 1
	if _, err := admitRound2(t, single, singleSpec); err == nil {
		t.Fatal("single-wire dependency accepted a retry identity")
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
}

func TestRound2WireGroupingAppliesToEveryCoherenceMode(t *testing.T) {
	profile, valid := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	setRetryAttempt(&valid, 88)
	makeRepeatedWireGroup(t, &valid)
	if _, err := admitRound2(t, profile, valid); err != nil {
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
			setRetryAttempt(&candidate, 88)
			makeRepeatedWireGroup(t, &candidate)
			record := candidate.Dependencies[1].View.Record()
			test.mutate(&record)
			candidate.Dependencies[1].View = snapshotFromRecord(t, record)
			if _, err := admitRound2(t, profile, candidate); err == nil {
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

func TestRound2VendorOverlayIsOnlyEvidenceBackedDelta(t *testing.T) {
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

func TestRound2ConstructorsAndSerializationAreBounded(t *testing.T) {
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
		profileFixture(t).Dependencies().ID(),
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

func TestRound2JSONPreflightRejectsDuplicateAndCaseFoldedKeys(t *testing.T) {
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

func TestRound2AdmissionCanonicalizesTimestamps(t *testing.T) {
	profile := profileFixture(t)
	state := round2EmptyLedgerState(t, profile)
	factory, ledger := round2Factory(t, profile, state, 0)
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
	if _, err := factory.NewObservation(spec); err == nil {
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
	admission, err := factory.NewObservation(spec)
	if err != nil {
		t.Fatalf("canonicalizable timestamp rejected: %v", err)
	}
	got := admission.Observation().Spec()
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

func TestRound2LedgerIsBoundedIssuerHighWaterWithCASAnchor(t *testing.T) {
	profile := profileFixture(t)
	state := round2EmptyLedgerState(t, profile)
	factory, ledger := round2Factory(t, profile, state, 0)
	spec := successfulObservationSpec(t, profile)
	spec.SampleID = "caller-selected"
	if _, err := factory.NewObservation(spec); err == nil {
		t.Fatal("factory trusted a caller-selected sample ID")
	}
	spec.SampleID = ""

	const count = 512
	var wait sync.WaitGroup
	admissions := make(chan reg.SampleAdmission, count)
	errors := make(chan error, count)
	for range count {
		wait.Add(1)
		go func() {
			defer wait.Done()
			admission, err := factory.NewObservation(spec)
			if err != nil {
				errors <- err
				return
			}
			admissions <- admission
		}()
	}
	wait.Wait()
	close(admissions)
	close(errors)
	for err := range errors {
		t.Fatalf("concurrent issuance failed: %v", err)
	}

	ids := make([]string, 0, count)
	anchors := make([]uint64, 0, count)
	for admission := range admissions {
		ids = append(ids, admission.Observation().SampleID())
		anchors = append(anchors, admission.ExpectedRevision())
		state := admission.PersistedState()
		if state.Revision != admission.ExpectedRevision()+1 ||
			state.HighWater != state.Revision {
			t.Fatal("admission lacks an exact CAS transition")
		}
	}
	sort.Strings(ids)
	sort.Slice(anchors, func(first, second int) bool {
		return anchors[first] < anchors[second]
	})
	if len(ids) != count || len(anchors) != count {
		t.Fatalf("issued %d IDs and %d anchors", len(ids), len(anchors))
	}
	for index := 1; index < len(ids); index++ {
		if ids[index] == ids[index-1] {
			t.Fatal("factory reused an issued sample ID")
		}
	}
	for index, anchor := range anchors {
		if anchor != uint64(index) {
			t.Fatalf("CAS anchor %d = %d", index, anchor)
		}
	}

	exported := ledger.ExportState()
	if exported.Revision != count || exported.HighWater != count {
		t.Fatalf("ledger state = %#v", exported)
	}
	encoded, err := reg.MarshalSampleLedgerState(exported)
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
	restartedFactory, err := reg.NewObservationFactory(profile, restarted)
	if err != nil {
		t.Fatalf("NewObservationFactory(restarted): %v", err)
	}
	next, err := restartedFactory.NewObservation(spec)
	if err != nil {
		t.Fatalf("post-restart issuance failed: %v", err)
	}
	if next.ExpectedRevision() != count {
		t.Fatalf("post-restart CAS anchor = %d", next.ExpectedRevision())
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

func TestRound2RevokedProfileRejectsSuccessorFields(t *testing.T) {
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

func TestRound2SerializationRoundTripsNewFields(t *testing.T) {
	profile, spec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	setRetryAttempt(&spec, 99)
	admission, err := admitRound2(t, profile, spec)
	if err != nil {
		t.Fatalf("NewObservation: %v", err)
	}
	encoded, err := reg.MarshalObservation(admission.Observation())
	if err != nil {
		t.Fatalf("MarshalObservation: %v", err)
	}
	state := round2EmptyLedgerState(t, profile)
	factory, _ := round2Factory(t, profile, state, 0)
	decodedAdmission, err := factory.UnmarshalObservation(encoded)
	if err != nil {
		t.Fatalf("UnmarshalObservation: %v", err)
	}
	reencoded, err := reg.MarshalObservation(decodedAdmission.Observation())
	if err != nil {
		t.Fatalf("MarshalObservation(round trip): %v", err)
	}
	if !bytes.Equal(encoded, reencoded) {
		t.Fatal("new observation fields are not byte-stable")
	}
	decodedSpec := decodedAdmission.Observation().Spec()
	if decodedSpec.RetryAttemptID != 99 {
		t.Fatal("observation retry identity was lost")
	}
	for index, dependency := range decodedSpec.Dependencies {
		if dependency.RetryAttemptID != 99 {
			t.Fatalf("dependency %d retry identity was lost", index)
		}
	}

	ledgerBytes, err := reg.MarshalSampleLedgerState(
		decodedAdmission.PersistedState(),
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

func TestRound2LimitErrorsRemainDeterministic(t *testing.T) {
	tooLong := strings.Repeat("z", reg.MaxContractStringBytes+1)
	first := fmt.Sprintf(`{"schema_version":"%s"}`, tooLong)
	second := fmt.Sprintf(`{"schema_version":"%s"}`, tooLong)
	_, firstErr := reg.UnmarshalProfileDescriptor([]byte(first))
	_, secondErr := reg.UnmarshalProfileDescriptor([]byte(second))
	if firstErr == nil || secondErr == nil || firstErr.Error() != secondErr.Error() {
		t.Fatal("bounded JSON rejection is absent or nondeterministic")
	}
}
