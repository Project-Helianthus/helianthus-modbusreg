package modbusreg_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

type round3MemoryCAS struct {
	mu      sync.Mutex
	state   reg.SampleLedgerState
	reject  bool
	commits int
}

func (store *round3MemoryCAS) CompareAndSwap(
	expected reg.SampleLedgerState,
	next reg.SampleLedgerState,
) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.reject || store.state != expected {
		return false, nil
	}
	store.state = next
	store.commits++
	return true, nil
}

func round3State(
	t *testing.T,
	profile reg.ProfileDescriptor,
	issuer string,
) reg.SampleLedgerState {
	t.Helper()
	state, err := reg.EmptySampleLedgerState(issuer, profile)
	if err != nil {
		t.Fatalf("EmptySampleLedgerState: %v", err)
	}
	return state
}

func round3Factory(
	t *testing.T,
	profile reg.ProfileDescriptor,
	state reg.SampleLedgerState,
	store reg.SampleStateCAS,
) *reg.ObservationFactory {
	t.Helper()
	ledger, err := reg.NewSampleLedger(state, state.Revision)
	if err != nil {
		t.Fatalf("NewSampleLedger: %v", err)
	}
	factory, err := reg.NewObservationFactory(profile, ledger, store)
	if err != nil {
		t.Fatalf("NewObservationFactory: %v", err)
	}
	return factory
}

func round3Attempt(
	t *testing.T,
	factory *reg.ObservationFactory,
	spec reg.ObservationSpec,
) (*reg.ObservationAttempt, reg.ObservationSpec) {
	t.Helper()
	spec.Dependencies = append(
		[]reg.DependencyResult(nil),
		spec.Dependencies...,
	)
	attempt, err := factory.BeginObservationAttempt()
	if err != nil {
		t.Fatalf("BeginObservationAttempt: %v", err)
	}
	for index := range spec.Dependencies {
		spec.Dependencies[index], err = attempt.BindDependency(
			spec.Dependencies[index],
		)
		if err != nil {
			t.Fatalf("BindDependency(%d): %v", index, err)
		}
	}
	return attempt, spec
}

func round3Publish(
	t *testing.T,
	profile reg.ProfileDescriptor,
	spec reg.ObservationSpec,
) (reg.Observation, error) {
	t.Helper()
	state := round3State(t, profile, "round3-issuer")
	store := &round3MemoryCAS{state: state}
	factory := round3Factory(t, profile, state, store)
	attempt, bound := round3Attempt(t, factory, spec)
	return attempt.Publish(bound)
}

func TestRound3LogicalViewIDIsUniqueOnlyWithinPhysicalGroup(t *testing.T) {
	profile, spec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	first := spec.Dependencies[0].View.Record()
	second := spec.Dependencies[1].View.Record()
	second.LogicalViewID = first.LogicalViewID
	spec.Dependencies[1].View = snapshotFromRecord(t, second)
	if _, err := round3Publish(t, profile, spec); err != nil {
		t.Fatalf("distinct physical responses reused a legal logical ID: %v", err)
	}

	_, samePhysical := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	makeRepeatedWireGroup(t, &samePhysical)
	first = samePhysical.Dependencies[0].View.Record()
	second = samePhysical.Dependencies[1].View.Record()
	second.LogicalViewID = first.LogicalViewID
	samePhysical.Dependencies[1].View = snapshotFromRecord(t, second)
	if _, err := round3Publish(t, profile, samePhysical); err == nil {
		t.Fatal("one physical response accepted a duplicate logical-view ID")
	}
}

func TestRound3PhysicalAndWireIdentityIsBidirectional(t *testing.T) {
	profile, aliasedPhysical := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	originalWire := aliasedPhysical.Dependencies[1].View.Record().WireResponseID
	makeRepeatedWireGroup(t, &aliasedPhysical)
	second := aliasedPhysical.Dependencies[1].View.Record()
	if originalWire == second.WireResponseID {
		originalWire++
	}
	second.WireResponseID = originalWire
	aliasedPhysical.Dependencies[1].View = snapshotFromRecord(t, second)
	if _, err := round3Publish(t, profile, aliasedPhysical); err == nil {
		t.Fatal("one physical identity mapped to two wire-response IDs")
	}

	_, aliasedWire := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	first := aliasedWire.Dependencies[0].View.Record()
	second = aliasedWire.Dependencies[1].View.Record()
	second.WireResponseID = first.WireResponseID
	aliasedWire.Dependencies[1].View = snapshotFromRecord(t, second)
	if _, err := round3Publish(t, profile, aliasedWire); err == nil {
		t.Fatal("one wire-response ID mapped to two physical identities")
	}

	_, conflictingWords := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	makeRepeatedWireGroup(t, &conflictingWords)
	second = conflictingWords.Dependencies[1].View.Record()
	second.Words[0] ^= 0xffff
	conflictingWords.Dependencies[1].View = snapshotFromRecord(t, second)
	if _, err := round3Publish(t, profile, conflictingWords); err == nil {
		t.Fatal("one physical group accepted contradictory overlapping words")
	}
}

func TestRound3RetryAttemptBindingIsOpaqueAndNonRelabelable(t *testing.T) {
	profile, firstSpec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	state := round3State(t, profile, "retry-binding")
	store := &round3MemoryCAS{state: state}
	factory := round3Factory(t, profile, state, store)

	firstAttempt, firstBound := round3Attempt(t, factory, firstSpec)
	_, secondSpec := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	secondAttempt, secondBound := round3Attempt(t, factory, secondSpec)

	mixed := firstBound
	mixed.Dependencies = append(
		[]reg.DependencyResult(nil),
		firstBound.Dependencies...,
	)
	mixed.Dependencies[1] = secondBound.Dependencies[1]
	if _, err := firstAttempt.Publish(mixed); err == nil {
		t.Fatal("retained-old and new-attempt dependencies were mixed")
	}

	encoded, err := firstAttempt.MarshalSpec(firstBound)
	if err != nil {
		t.Fatalf("MarshalSpec: %v", err)
	}
	if bytes.Contains(encoded, []byte(`"retry_attempt_id"`)) ||
		!bytes.Contains(encoded, []byte(`"retry_attempt_token"`)) {
		t.Fatal("serialized retry identity remained a caller-owned numeric label")
	}
	decoded, err := firstAttempt.DecodeSpec(encoded)
	if err != nil {
		t.Fatalf("DecodeSpec(same attempt): %v", err)
	}
	if _, err := secondAttempt.DecodeSpec(encoded); err == nil {
		t.Fatal("serialized dependencies rebound to a different attempt")
	}
	secondCurrent := secondBound
	secondCurrent.Dependencies = append(
		[]reg.DependencyResult(nil),
		secondBound.Dependencies...,
	)
	currentRecord := secondCurrent.Dependencies[0].View.Record()
	currentRecord.Words[0] ^= 0x0001
	secondCurrent.Dependencies[0].View = snapshotFromRecord(t, currentRecord)
	secondEncoded, err := secondAttempt.MarshalSpec(secondCurrent)
	if err != nil {
		t.Fatalf("MarshalSpec(second attempt): %v", err)
	}
	var firstRecord, secondRecord map[string]any
	if err := json.Unmarshal(encoded, &firstRecord); err != nil {
		t.Fatalf("json.Unmarshal(first token): %v", err)
	}
	if err := json.Unmarshal(secondEncoded, &secondRecord); err != nil {
		t.Fatalf("json.Unmarshal(second token): %v", err)
	}
	firstToken, _ := firstRecord["retry_attempt_token"].(string)
	secondToken, _ := secondRecord["retry_attempt_token"].(string)
	if firstToken == "" || secondToken == "" || firstToken == secondToken {
		t.Fatal("attempt seals are absent or not attempt-specific")
	}
	relabelled := bytes.ReplaceAll(
		encoded,
		[]byte(firstToken),
		[]byte(secondToken),
	)
	if _, err := secondAttempt.DecodeSpec(relabelled); err == nil {
		t.Fatal("caller relabelled an old serialized set into a new attempt")
	}
	if _, err := firstAttempt.Publish(decoded); err != nil {
		t.Fatalf("same-attempt serialized input was rejected: %v", err)
	}
}

func TestRound3ObservationPublishesOnlyAfterExternalCAS(t *testing.T) {
	profile := profileFixture(t)
	spec := successfulObservationSpec(t, profile)
	initial := round3State(t, profile, "cas-domain")
	if initial.ProfileID != profile.ID() ||
		initial.ProfileVersion != profile.Version() ||
		initial.DependencySetID != profile.Dependencies().ID() {
		t.Fatal("ledger state is not bound to the exact profile and dependency set")
	}

	rejectingStore := &round3MemoryCAS{state: initial, reject: true}
	rejectingFactory := round3Factory(t, profile, initial, rejectingStore)
	rejectingAttempt, rejectingSpec := round3Attempt(
		t,
		rejectingFactory,
		spec,
	)
	unpublished, err := rejectingAttempt.Publish(rejectingSpec)
	if err == nil || unpublished.SampleID() != "" {
		t.Fatal("an observation escaped after a failed external CAS")
	}
	if _, marshalErr := reg.MarshalObservation(unpublished); marshalErr == nil {
		t.Fatal("an uncommitted observation was serializable")
	}

	sharedStore := &round3MemoryCAS{state: initial}
	firstFactory := round3Factory(t, profile, initial, sharedStore)
	secondFactory := round3Factory(t, profile, initial, sharedStore)
	firstAttempt, firstSpec := round3Attempt(t, firstFactory, spec)
	secondAttempt, secondSpec := round3Attempt(t, secondFactory, spec)

	type result struct {
		observation reg.Observation
		err         error
	}
	results := make(chan result, 2)
	var wait sync.WaitGroup
	for _, publish := range []func() (reg.Observation, error){
		func() (reg.Observation, error) {
			return firstAttempt.Publish(firstSpec)
		},
		func() (reg.Observation, error) {
			return secondAttempt.Publish(secondSpec)
		},
	} {
		wait.Add(1)
		go func(publish func() (reg.Observation, error)) {
			defer wait.Done()
			observation, err := publish()
			results <- result{observation: observation, err: err}
		}(publish)
	}
	wait.Wait()
	close(results)

	successes := 0
	failures := 0
	for result := range results {
		if result.err == nil {
			successes++
			if result.observation.SampleID() == "" {
				t.Fatal("successful CAS returned an empty observation")
			}
		} else {
			failures++
			if result.observation.SampleID() != "" {
				t.Fatal("failed CAS returned a publishable observation")
			}
		}
	}
	if successes != 1 || failures != 1 || sharedStore.commits != 1 {
		t.Fatalf(
			"CAS publication successes=%d failures=%d commits=%d",
			successes,
			failures,
			sharedStore.commits,
		)
	}

	otherSpec := profile.Spec()
	otherSpec.ID = "example.standard.other"
	otherProfile, err := reg.NewProfileDescriptor(otherSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(other): %v", err)
	}
	otherInitial := round3State(t, otherProfile, "cas-domain")
	otherFactory := round3Factory(t, otherProfile, otherInitial, sharedStore)
	otherAttempt, otherObservation := round3Attempt(
		t,
		otherFactory,
		successfulObservationSpec(t, otherProfile),
	)
	if published, err := otherAttempt.Publish(otherObservation); err == nil ||
		published.SampleID() != "" {
		t.Fatal("one issuer domain restarted at zero for a different profile")
	}
}

func TestRound3RuntimeContractVersionIsPinned(t *testing.T) {
	if reg.PinnedRuntimeContractVersion().String() != "1.0.0" {
		t.Fatalf(
			"pinned runtime version = %q",
			reg.PinnedRuntimeContractVersion().String(),
		)
	}
	spec := profileFixture(t).Spec()
	spec.RuntimeContractVersion = version(t, "9.0.0")
	if _, err := reg.NewProfileDescriptor(spec); err == nil {
		t.Fatal("profile accepted an arbitrary runtime contract version")
	}

	encoded, err := reg.MarshalProfileDescriptor(profileFixture(t))
	if err != nil {
		t.Fatalf("MarshalProfileDescriptor: %v", err)
	}
	forged := bytes.Replace(
		encoded,
		[]byte(`"runtime_contract_version":"1.0.0"`),
		[]byte(`"runtime_contract_version":"9.0.0"`),
		1,
	)
	if _, err := reg.UnmarshalProfileDescriptor(forged); err == nil {
		t.Fatal("serialized profile accepted an incompatible runtime version")
	}
}

func overlayCodecDelta(
	t *testing.T,
	id string,
	codec reg.CodecSpec,
) reg.VendorOverlayDeltaSpec {
	t.Helper()
	return reg.VendorOverlayDeltaSpec{
		ID:                 id,
		Version:            version(t, "1.0.0"),
		Kind:               reg.OverlayDeltaCodec,
		Operation:          reg.OverlayDeltaReplace,
		TargetID:           codec.ID,
		Codec:              &codec,
		EvidenceReferences: []string{"overlay-evidence"},
	}
}

func overlayDependencyDelta(
	t *testing.T,
	id string,
	dependency reg.DependencySpec,
) reg.VendorOverlayDeltaSpec {
	t.Helper()
	return reg.VendorOverlayDeltaSpec{
		ID:                 id,
		Version:            version(t, "1.0.0"),
		Kind:               reg.OverlayDeltaDependency,
		Operation:          reg.OverlayDeltaReplace,
		TargetID:           dependency.ID,
		Dependency:         &dependency,
		EvidenceReferences: []string{"overlay-evidence"},
	}
}

func TestRound3OverlayMaterializesAndValidatesEffectiveGraph(t *testing.T) {
	base := qualifiedBaseProfile(t)

	sameVersionCodec := numericCodecSpec(t)
	sameVersionCodec.Scale.Denominator = 20
	sameVersionOverlay := overlayDeltaProfile(
		t,
		base,
		[]reg.VendorOverlayDeltaSpec{
			overlayCodecDelta(t, "same-version-codec", sameVersionCodec),
		},
	)
	if _, err := reg.NewCatalog(base, sameVersionOverlay); err == nil {
		t.Fatal("semantic codec replacement kept the base codec version")
	}

	widthChangedCodec := numericCodecSpec(t)
	widthChangedCodec.Version = version(t, "2.0.0")
	widthChangedCodec.RawWordCount = 1
	widthChangedCodec.WordPermutation = []uint16{0}
	widthChangedCodec.Sentinels = []reg.RawSentinel{{
		Kind:  reg.SentinelInvalid,
		Words: []uint16{0xffff},
	}}
	widthOverlay := overlayDeltaProfile(
		t,
		base,
		[]reg.VendorOverlayDeltaSpec{
			overlayCodecDelta(t, "width-codec", widthChangedCodec),
		},
	)
	if _, err := reg.NewCatalog(base, widthOverlay); err == nil {
		t.Fatal("effective graph ignored dependencies broken by a codec replacement")
	}

	missingCodecDependency := dependencySpec(t, "vendor-extra", 110)
	missingCodecDependency.Version = version(t, "2.0.0")
	missingCodecDependency.CodecID = "missing-codec"
	missingCodecDependency.CodecVersion = version(t, "2.0.0")
	missingCodecDependency.EvidenceReferences = []string{"overlay-evidence"}
	missingCodecOverlay := overlayDeltaProfile(
		t,
		base,
		[]reg.VendorOverlayDeltaSpec{{
			ID:                 "missing-codec-dependency",
			Version:            version(t, "1.0.0"),
			Kind:               reg.OverlayDeltaDependency,
			Operation:          reg.OverlayDeltaAdd,
			TargetID:           missingCodecDependency.ID,
			Dependency:         &missingCodecDependency,
			EvidenceReferences: []string{"overlay-evidence"},
		}},
	)
	if _, err := reg.NewCatalog(base, missingCodecOverlay); err == nil {
		t.Fatal("added dependency referenced an absent effective codec")
	}

	changedCodec := numericCodecSpec(t)
	changedCodec.Version = version(t, "2.0.0")
	changedCodec.Scale.Denominator = 20
	deltas := []reg.VendorOverlayDeltaSpec{
		overlayCodecDelta(t, "codec-v2", changedCodec),
	}
	for index, dependency := range base.Dependencies().Dependencies() {
		spec := dependency.Spec()
		spec.Version = version(t, "2.0.0")
		spec.CodecVersion = version(t, "2.0.0")
		spec.EvidenceReferences = []string{"overlay-evidence"}
		deltas = append(
			deltas,
			overlayDependencyDelta(
				t,
				fmt.Sprintf("dependency-v2-%d", index),
				spec,
			),
		)
	}
	validOverlay := overlayDeltaProfile(t, base, deltas)
	if _, err := reg.NewCatalog(base, validOverlay); err != nil {
		t.Fatalf("fully coherent effective overlay graph rejected: %v", err)
	}

	coherence := base.Spec().Coherence
	coherence.Mode = reg.CoherenceBoundedMultiResponse
	coherence.MaximumSourceSkew = time.Second
	coherence.MaximumReceiptSkew = time.Second
	coherence.RequireGenerationEquality = true
	coherence.AcquisitionOrder = reg.AcquisitionOrderDependencyDeclaration
	coherence.RetrySetBehavior = reg.RetryWholeSet
	sameVersionCoherence := overlayDeltaProfile(
		t,
		base,
		[]reg.VendorOverlayDeltaSpec{{
			ID:                 "same-version-coherence",
			Version:            version(t, "1.0.0"),
			Kind:               reg.OverlayDeltaCoherence,
			Operation:          reg.OverlayDeltaReplace,
			TargetID:           "coherence-policy",
			Coherence:          &coherence,
			EvidenceReferences: []string{"overlay-evidence"},
		}},
	)
	if _, err := reg.NewCatalog(base, sameVersionCoherence); err == nil {
		t.Fatal("semantic coherence replacement kept the base version")
	}
}

func TestRound3BoundedSkewUsesCheckedArithmetic(t *testing.T) {
	profile, observation := boundedFixture(
		t,
		reg.AcquisitionOrderDependencyDeclaration,
	)
	invalidProfile := profile.Spec()
	invalidProfile.Coherence.MaximumSourceSkew = time.Duration(1<<63 - 1)
	invalidProfile.Coherence.MaximumReceiptSkew = time.Duration(1<<63 - 1)
	if _, err := reg.NewProfileDescriptor(invalidProfile); err == nil {
		t.Fatal("profile accepted a saturated multi-century skew declaration")
	}

	boundedProfile := profile.Spec()
	boundedProfile.Coherence.MaximumSourceSkew = reg.MaxDeclaredCoherenceSkew
	boundedProfile.Coherence.MaximumReceiptSkew = reg.MaxDeclaredCoherenceSkew
	profile, err := reg.NewProfileDescriptor(boundedProfile)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(max bounded skew): %v", err)
	}
	early := time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(9999, time.December, 31, 0, 0, 0, 0, time.UTC)
	observation.Dependencies[0].SourceTime = reg.SourceTimeObserved(early)
	observation.Dependencies[0].LocalReceiptTime = early
	observation.Dependencies[1].SourceTime = reg.SourceTimeObserved(late)
	observation.Dependencies[1].LocalReceiptTime = late
	observation.SourceTime = reg.SourceTimeObserved(late)
	observation.LocalReceiptTime = late
	if _, err := round3Publish(t, profile, observation); err == nil {
		t.Fatal("year 1..9999 skew passed through time.Duration saturation")
	}
}

func TestRound3JSONRequiresExactKeysAndUnicode(t *testing.T) {
	encoded, err := reg.MarshalProfileDescriptor(profileFixture(t))
	if err != nil {
		t.Fatalf("MarshalProfileDescriptor: %v", err)
	}
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "standalone root case alias",
			data: bytes.Replace(
				encoded,
				[]byte(`"schema_version"`),
				[]byte(`"Schema_Version"`),
				1,
			),
		},
		{
			name: "standalone nested case alias",
			data: bytes.Replace(
				encoded,
				[]byte(`"ID":"u32-energy"`),
				[]byte(`"id":"u32-energy"`),
				1,
			),
		},
		{
			name: "invalid UTF-8",
			data: bytes.Replace(
				encoded,
				[]byte("example.standard.energy"),
				[]byte{'e', 'x', 0xff},
				1,
			),
		},
		{
			name: "unpaired high surrogate",
			data: bytes.Replace(
				encoded,
				[]byte("example.standard.energy"),
				[]byte(`example.standard.\ud800`),
				1,
			),
		},
		{
			name: "unpaired low surrogate",
			data: bytes.Replace(
				encoded,
				[]byte("example.standard.energy"),
				[]byte(`example.standard.\udc00`),
				1,
			),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := reg.UnmarshalProfileDescriptor(test.data); err == nil {
				t.Fatal("non-canonical JSON was accepted")
			}
		})
	}
}

func TestRound3ConstructorsUseOneCumulativeBudget(t *testing.T) {
	dependency := dependencySpec(t, "aggregate-budget", 0)
	dependency.EvidenceReferences = make(
		[]string,
		reg.MaxProfileEvidenceReferences,
	)
	for index := range dependency.EvidenceReferences {
		dependency.EvidenceReferences[index] = fmt.Sprintf(
			"evidence-%04d-%s",
			index,
			strings.Repeat("x", reg.MaxContractStringBytes-20),
		)
	}
	if _, err := reg.NewDependency(dependency); err == nil {
		t.Fatal("nested dependency strings exceeded the cumulative byte budget")
	}

	base := profileFixture(t).Spec()
	profiles := make([]reg.ProfileDescriptor, 0, 8)
	totalSerialized := 0
	for profileIndex := 0; profileIndex < 8; profileIndex++ {
		spec := base
		spec.ID = fmt.Sprintf("example.standard.aggregate-%d", profileIndex)
		spec.ModelApplicability = make([]string, 600)
		for modelIndex := range spec.ModelApplicability {
			spec.ModelApplicability[modelIndex] = fmt.Sprintf(
				"model-%03d-%s",
				modelIndex,
				strings.Repeat("m", 980),
			)
		}
		profile, err := reg.NewProfileDescriptor(spec)
		if err != nil {
			t.Fatalf("NewProfileDescriptor(%d): %v", profileIndex, err)
		}
		serialized, err := reg.MarshalProfileDescriptor(profile)
		if err != nil {
			t.Fatalf("MarshalProfileDescriptor(%d): %v", profileIndex, err)
		}
		totalSerialized += len(serialized)
		profiles = append(profiles, profile)
	}
	if totalSerialized <= reg.MaxSerializedContractBytes {
		t.Fatalf("catalog fixture is only %d bytes", totalSerialized)
	}
	if _, err := reg.NewCatalog(profiles...); err == nil {
		t.Fatal("catalog ignored its cumulative aggregate byte budget")
	}
}

func TestRound3DependencySetDigestShapeIsValidatedEverywhere(t *testing.T) {
	profile := profileFixture(t)
	state := round3State(t, profile, "digest-shape")
	store := &round3MemoryCAS{state: state}
	factory := round3Factory(t, profile, state, store)
	attempt, spec := round3Attempt(
		t,
		factory,
		successfulObservationSpec(t, profile),
	)
	encoded, err := attempt.MarshalSpec(spec)
	if err != nil {
		t.Fatalf("MarshalSpec: %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(encoded, &record); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	record["dependency_set_id"] = "sha256:" + strings.Repeat("A", 64)
	forged, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if _, err := attempt.DecodeSpec(forged); err == nil {
		t.Fatal("persisted observation accepted a malformed dependency-set digest")
	}
}
