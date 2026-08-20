package modbusreg_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

type recordedProbeReader struct {
	mu      sync.Mutex
	results map[string]reg.ProbeReadResult
	errors  map[string]error
	reads   []reg.ProbeReadRequest
}

func (reader *recordedProbeReader) ReadProbe(
	_ context.Context,
	request reg.ProbeReadRequest,
) (reg.ProbeReadResult, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	reader.reads = append(reader.reads, request)
	if err := reader.errors[request.DeclarationID()]; err != nil {
		return reg.ProbeReadResult{}, err
	}
	return reader.results[request.DeclarationID()], nil
}

func (reader *recordedProbeReader) declarationOrder() []string {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	result := make([]string, len(reader.reads))
	for index, request := range reader.reads {
		result[index] = request.DeclarationID()
	}
	return result
}

func detectionWords(value string) []uint16 {
	encoded := []byte(value)
	if len(encoded)%2 != 0 {
		encoded = append(encoded, 0)
	}
	words := make([]uint16, len(encoded)/2)
	for index := range words {
		words[index] = uint16(encoded[index*2])<<8 | uint16(encoded[index*2+1])
	}
	for len(words) < 16 {
		words = append(words, 0)
	}
	return words
}

func mustProbeResult(
	t *testing.T,
	value string,
	evidenceID string,
) reg.ProbeReadResult {
	t.Helper()
	result, err := reg.NewProbeReadResult(reg.ProbeReadResultSpec{
		Status:     reg.ProbeReadSucceeded,
		Words:      detectionWords(value),
		EvidenceID: evidenceID,
	})
	if err != nil {
		t.Fatalf("NewProbeReadResult: %v", err)
	}
	return result
}

func detectionLimits() reg.DetectionLimits {
	return reg.DefaultDetectionLimits()
}

func detectionPlan(t *testing.T) reg.ProbePlan {
	t.Helper()
	plan, err := reg.NewProbePlan(reg.ProbePlanSpec{
		Version: version(t, "1.0.0"),
		Declarations: []reg.ProbeDeclarationSpec{
			{
				ID:            "manufacturer-identity",
				Function:      reg.ProbeReadHoldingRegisters,
				Address:       100,
				WordCount:     16,
				IdentityField: reg.ProbeIdentityManufacturer,
				Encoding:      reg.ProbeIdentityASCII,
			},
			{
				ID:            "model-identity",
				Function:      reg.ProbeReadInputRegisters,
				Address:       140,
				WordCount:     16,
				IdentityField: reg.ProbeIdentityModel,
				Encoding:      reg.ProbeIdentityASCII,
			},
			{
				ID:            "firmware-identity",
				Function:      reg.ProbeReadHoldingRegisters,
				Address:       180,
				WordCount:     16,
				IdentityField: reg.ProbeIdentityFirmware,
				Encoding:      reg.ProbeIdentityASCII,
			},
		},
	}, detectionLimits())
	if err != nil {
		t.Fatalf("NewProbePlan: %v", err)
	}
	return plan
}

func detectionProfile(
	t *testing.T,
	id string,
	profileVersion string,
	maturity reg.ProfileMaturity,
	state reg.ProfileState,
	defaultEnabled bool,
) reg.ProfileDescriptor {
	t.Helper()
	spec := profileFixture(t).Spec()
	spec.ID = id
	spec.Version = version(t, profileVersion)
	spec.Maturity = maturity
	spec.State = state
	spec.DefaultEnabled = defaultEnabled
	spec.SupersededByID = ""
	spec.SupersededByVersion = reg.Version{}
	profile, err := reg.NewProfileDescriptor(spec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(%s): %v", id, err)
	}
	return profile
}

func detectionCandidate(
	t *testing.T,
	profile reg.ProfileDescriptor,
	score uint32,
	enabled bool,
	fixtureOnly bool,
) reg.DetectionCandidate {
	t.Helper()
	candidate, err := reg.NewDetectionCandidate(reg.DetectionCandidateSpec{
		ProfileID:      profile.ID(),
		ProfileVersion: profile.Version(),
		Score:          score,
		Enabled:        enabled,
		FixtureOnly:    fixtureOnly,
		Manufacturer: reg.IdentityStringGateSpec{
			Expected: "manufacturer-alpha",
		},
		Model: reg.IdentityStringGateSpec{
			Expected: "model-series-a",
		},
		Firmware: reg.FirmwareGateSpec{
			MinimumInclusive: version(t, "1.9.0"),
			MaximumExclusive: version(t, "2.0.0"),
		},
	}, detectionLimits())
	if err != nil {
		t.Fatalf("NewDetectionCandidate(%s): %v", profile.ID(), err)
	}
	return candidate
}

func detectionReader(t *testing.T) *recordedProbeReader {
	t.Helper()
	return &recordedProbeReader{
		results: map[string]reg.ProbeReadResult{
			"manufacturer-identity": mustProbeResult(
				t,
				"manufacturer-alpha",
				"probe-evidence-manufacturer",
			),
			"model-identity": mustProbeResult(
				t,
				"model-series-a",
				"probe-evidence-model",
			),
			"firmware-identity": mustProbeResult(
				t,
				"1.10.0",
				"probe-evidence-firmware",
			),
		},
		errors: make(map[string]error),
	}
}

func newDetector(
	t *testing.T,
	catalog reg.Catalog,
	candidates ...reg.DetectionCandidate,
) *reg.ProfileDetector {
	t.Helper()
	detector, err := reg.NewProfileDetector(reg.ProfileDetectorSpec{
		DetectorVersion: version(t, "1.0.0"),
		Plan:            detectionPlan(t),
		Catalog:         catalog,
		Candidates:      candidates,
		Limits:          detectionLimits(),
	})
	if err != nil {
		t.Fatalf("NewProfileDetector: %v", err)
	}
	return detector
}

func TestProbeContractIsReadOnlyBoundedAndTransportNeutral(t *testing.T) {
	plan := detectionPlan(t)
	declarations := plan.Declarations()
	if len(declarations) != 3 ||
		declarations[0].Function != reg.ProbeReadHoldingRegisters ||
		declarations[1].Function != reg.ProbeReadInputRegisters {
		t.Fatalf("probe declarations=%+v", declarations)
	}
	declarations[0].ID = "mutated"
	if plan.Declarations()[0].ID != "manufacturer-identity" {
		t.Fatal("probe plan declarations are mutable through an accessor")
	}

	readerType := reflect.TypeOf((*reg.ProbeReader)(nil)).Elem()
	if readerType.NumMethod() != 1 || readerType.Method(0).Name != "ReadProbe" {
		t.Fatalf("probe reader surface=%v", readerType)
	}
	requestType := reflect.TypeOf(reg.ProbeReadRequest{})
	for index := 0; index < requestType.NumField(); index++ {
		name := strings.ToLower(requestType.Field(index).Name)
		for _, forbidden := range []string{"endpoint", "transport", "write", "socket", "serial"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("probe request exposes forbidden field %q", name)
			}
		}
	}

	invalidFunctions := []reg.ProbeFunction{"", reg.ProbeFunction("fc06"), reg.ProbeFunction("write")}
	for _, function := range invalidFunctions {
		_, err := reg.NewProbePlan(reg.ProbePlanSpec{
			Version: version(t, "1.0.0"),
			Declarations: []reg.ProbeDeclarationSpec{{
				ID:            "invalid-function",
				Function:      function,
				Address:       1,
				WordCount:     1,
				IdentityField: reg.ProbeIdentityManufacturer,
				Encoding:      reg.ProbeIdentityASCII,
			}},
		}, detectionLimits())
		if err == nil {
			t.Fatalf("probe function %q was accepted", function)
		}
	}
}

func TestDetectorExecutesDeclarationsOnceInDeclaredOrder(t *testing.T) {
	profile := detectionProfile(
		t,
		"example.standard.detectable",
		"1.0.0",
		reg.MaturityQualified,
		reg.ProfileActive,
		true,
	)
	catalog, err := reg.NewCatalog(profile)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	detector := newDetector(t, catalog, detectionCandidate(t, profile, 100, true, false))
	reader := detectionReader(t)
	decision, err := detector.Detect(context.Background(), reader, reg.DetectionOptions{})
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if decision.Outcome() != reg.DetectionMatched ||
		decision.SelectedProfileID() != profile.ID() ||
		decision.SelectedProfileVersion() != profile.Version() {
		t.Fatalf("decision=%+v", decision)
	}
	wantOrder := []string{
		"manufacturer-identity",
		"model-identity",
		"firmware-identity",
	}
	if got := reader.declarationOrder(); !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("read order=%v want %v", got, wantOrder)
	}
}

func TestDetectorUsesStrictIdentityAndSemanticFirmwareGates(t *testing.T) {
	profile := detectionProfile(
		t,
		"example.standard.semantic-version",
		"1.0.0",
		reg.MaturityQualified,
		reg.ProfileActive,
		true,
	)
	catalog, _ := reg.NewCatalog(profile)
	detector := newDetector(t, catalog, detectionCandidate(t, profile, 10, true, false))

	tests := []struct {
		name       string
		field      string
		value      string
		wantReason reg.DetectionReason
	}{
		{name: "semantic 1.10 exceeds 1.9", field: "firmware-identity", value: "1.10.0"},
		{name: "manufacturer case differs", field: "manufacturer-identity", value: "Manufacturer-Alpha", wantReason: reg.DetectionReasonIdentityMismatch},
		{name: "model suffix differs", field: "model-identity", value: "model-series-a-plus", wantReason: reg.DetectionReasonIdentityMismatch},
		{name: "firmware below range", field: "firmware-identity", value: "1.8.99", wantReason: reg.DetectionReasonFirmwareMismatch},
		{name: "firmware upper bound excluded", field: "firmware-identity", value: "2.0.0", wantReason: reg.DetectionReasonFirmwareMismatch},
		{name: "firmware is not semantic", field: "firmware-identity", value: "release-1.10", wantReason: reg.DetectionReasonInvalidIdentity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := detectionReader(t)
			reader.results[test.field] = mustProbeResult(t, test.value, "probe-evidence-case")
			decision, err := detector.Detect(context.Background(), reader, reg.DetectionOptions{})
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if test.wantReason == "" {
				if decision.Outcome() != reg.DetectionMatched {
					t.Fatalf("semantic firmware was not matched: %+v", decision)
				}
				return
			}
			if decision.Outcome() != reg.DetectionNoMatch || decision.Reason() != test.wantReason {
				t.Fatalf("decision=(%q,%q)", decision.Outcome(), decision.Reason())
			}
		})
	}
}

func TestDetectorFirmwareComparisonDoesNotUseMachineIntegers(t *testing.T) {
	profile := detectionProfile(
		t,
		"example.standard.large-firmware",
		"1.0.0",
		reg.MaturityQualified,
		reg.ProfileActive,
		true,
	)
	catalog, _ := reg.NewCatalog(profile)
	minimum := strings.Repeat("8", 120) + ".0.0"
	actual := strings.Repeat("9", 120) + ".0.0"
	maximum := "1" + strings.Repeat("0", 120) + ".0.0"
	candidate, err := reg.NewDetectionCandidate(reg.DetectionCandidateSpec{
		ProfileID:      profile.ID(),
		ProfileVersion: profile.Version(),
		Score:          1,
		Enabled:        true,
		Manufacturer:   reg.IdentityStringGateSpec{Expected: "manufacturer-alpha"},
		Model:          reg.IdentityStringGateSpec{Expected: "model-series-a"},
		Firmware: reg.FirmwareGateSpec{
			MinimumInclusive: version(t, minimum),
			MaximumExclusive: version(t, maximum),
		},
	}, detectionLimits())
	if err != nil {
		t.Fatalf("NewDetectionCandidate: %v", err)
	}
	planSpec := detectionPlan(t).Spec()
	planSpec.Declarations[2].WordCount = uint16(len(detectionWords(actual)))
	plan, err := reg.NewProbePlan(planSpec, detectionLimits())
	if err != nil {
		t.Fatalf("NewProbePlan: %v", err)
	}
	detector, err := reg.NewProfileDetector(reg.ProfileDetectorSpec{
		DetectorVersion: version(t, "1.0.0"),
		Plan:            plan,
		Catalog:         catalog,
		Candidates:      []reg.DetectionCandidate{candidate},
		Limits:          detectionLimits(),
	})
	if err != nil {
		t.Fatalf("NewProfileDetector: %v", err)
	}
	reader := detectionReader(t)
	reader.results["firmware-identity"] = mustProbeResult(
		t,
		actual,
		"probe-evidence-large-firmware",
	)
	decision, err := detector.Detect(context.Background(), reader, reg.DetectionOptions{})
	if err != nil || decision.Outcome() != reg.DetectionMatched {
		t.Fatalf("large numeric firmware decision=(%+v,%v)", decision, err)
	}
}

func TestDetectorRejectsEveryMultipleEligibleMatchIndependentOfOrderAndScore(t *testing.T) {
	first := detectionProfile(t, "example.standard.alpha", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	second := detectionProfile(t, "example.standard.beta", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	firstCandidate := detectionCandidate(t, first, 20, true, false)
	secondCandidate := detectionCandidate(t, second, 10, true, false)

	for _, profiles := range [][]reg.ProfileDescriptor{{first, second}, {second, first}} {
		catalog, err := reg.NewCatalog(profiles...)
		if err != nil {
			t.Fatalf("NewCatalog: %v", err)
		}
		detector := newDetector(t, catalog, secondCandidate, firstCandidate)
		decision, err := detector.Detect(context.Background(), detectionReader(t), reg.DetectionOptions{})
		if err != nil || decision.Outcome() != reg.DetectionAmbiguous ||
			decision.Reason() != reg.DetectionReasonMultipleMatches ||
			decision.SelectedProfileID() != "" {
			t.Fatalf("permuted selection=(%+v,%v)", decision, err)
		}
	}

	tied := detectionCandidate(t, second, 20, true, false)
	catalog, _ := reg.NewCatalog(first, second)
	decision, err := newDetector(t, catalog, firstCandidate, tied).Detect(
		context.Background(),
		detectionReader(t),
		reg.DetectionOptions{},
	)
	if err != nil || decision.Outcome() != reg.DetectionAmbiguous ||
		decision.Reason() != reg.DetectionReasonMultipleMatches ||
		decision.SelectedProfileID() != "" {
		t.Fatalf("equal-best decision=(%+v,%v)", decision, err)
	}
	evidence := decision.Evidence()
	if len(evidence) != 2 || evidence[0].ProfileID != first.ID() || evidence[1].ProfileID != second.ID() {
		t.Fatalf("ambiguity evidence is not identity-sorted: %+v", evidence)
	}
	encoded, err := reg.MarshalDetectionDecision(decision)
	if err != nil {
		t.Fatalf("MarshalDetectionDecision: %v", err)
	}
	var contradictory map[string]any
	if err := json.Unmarshal(encoded, &contradictory); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	contradictory["evidence"].([]any)[1].(map[string]any)["detector_version"] = "2.0.0"
	malformed, _ := json.Marshal(contradictory)
	if _, err := reg.UnmarshalDetectionDecision(malformed); err == nil {
		t.Fatal("contradictory detector contract versions were accepted")
	}
}

func TestDetectorExplicitNoMatchAndFixtureOptIn(t *testing.T) {
	profile := detectionProfile(t, "example.standard.fixture-candidate", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	catalog, _ := reg.NewCatalog(profile)
	detector := newDetector(t, catalog, detectionCandidate(t, profile, 10, true, true))

	decision, err := detector.Detect(context.Background(), detectionReader(t), reg.DetectionOptions{})
	if err != nil || decision.Outcome() != reg.DetectionNoMatch ||
		decision.Reason() != reg.DetectionReasonFixtureOptInRequired {
		t.Fatalf("fixture default decision=(%+v,%v)", decision, err)
	}
	decision, err = detector.Detect(
		context.Background(),
		detectionReader(t),
		reg.DetectionOptions{AllowFixtureOnly: true},
	)
	if err != nil || decision.Outcome() != reg.DetectionMatched {
		t.Fatalf("fixture opt-in decision=(%+v,%v)", decision, err)
	}

	reader := detectionReader(t)
	reader.results["model-identity"] = mustProbeResult(t, "unknown-model", "probe-evidence-no-match")
	decision, err = detector.Detect(
		context.Background(),
		reader,
		reg.DetectionOptions{AllowFixtureOnly: true},
	)
	if err != nil || decision.Outcome() != reg.DetectionNoMatch ||
		decision.Reason() != reg.DetectionReasonIdentityMismatch {
		t.Fatalf("explicit no-match=(%+v,%v)", decision, err)
	}
}

func TestActiveSelectionRejectsIneligibleProfilesBeforeScoring(t *testing.T) {
	eligible := detectionProfile(t, "example.standard.eligible", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	disabled := detectionProfile(t, "example.standard.disabled", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	defaultOff := detectionProfile(t, "example.standard.default-off", "1.0.0", reg.MaturityQualified, reg.ProfileActive, false)
	unqualified := detectionProfile(t, "example.standard.unqualified", "1.0.0", reg.MaturityExperimental, reg.ProfileActive, false)
	revoked := detectionProfile(t, "example.standard.revoked", "1.0.0", reg.MaturityQualified, reg.ProfileRevoked, false)
	replacement := detectionProfile(t, "example.standard.replacement", "2.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	supersededSpec := detectionProfile(t, "example.standard.superseded", "1.0.0", reg.MaturityQualified, reg.ProfileActive, false).Spec()
	supersededSpec.State = reg.ProfileSuperseded
	supersededSpec.SupersededByID = replacement.ID()
	supersededSpec.SupersededByVersion = replacement.Version()
	superseded, err := reg.NewProfileDescriptor(supersededSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(superseded): %v", err)
	}
	catalog, err := reg.NewCatalog(
		revoked,
		defaultOff,
		eligible,
		unqualified,
		disabled,
		superseded,
		replacement,
	)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	detector := newDetector(t, catalog,
		detectionCandidate(t, revoked, 100, true, false),
		detectionCandidate(t, disabled, 100, false, false),
		detectionCandidate(t, defaultOff, 100, true, false),
		detectionCandidate(t, unqualified, 100, true, false),
		detectionCandidate(t, superseded, 100, true, false),
		detectionCandidate(t, eligible, 1, true, false),
	)
	decision, err := detector.Detect(context.Background(), detectionReader(t), reg.DetectionOptions{})
	if err != nil || decision.Outcome() != reg.DetectionMatched ||
		decision.SelectedProfileID() != eligible.ID() {
		t.Fatalf("active selection=(%+v,%v)", decision, err)
	}
	reasons := make(map[string]reg.DetectionReason)
	for _, evidence := range decision.Evidence() {
		reasons[evidence.ProfileID] = evidence.Reason
	}
	want := map[string]reg.DetectionReason{
		revoked.ID():     reg.DetectionReasonProfileRevoked,
		disabled.ID():    reg.DetectionReasonCandidateDisabled,
		defaultOff.ID():  reg.DetectionReasonProfileDefaultOff,
		unqualified.ID(): reg.DetectionReasonProfileUnqualified,
		superseded.ID():  reg.DetectionReasonProfileSuperseded,
	}
	for profileID, reason := range want {
		if reasons[profileID] != reason {
			t.Fatalf("reason[%s]=%q want %q", profileID, reasons[profileID], reason)
		}
	}
}

func TestDetectorFailsClosedOnProbeFailuresAndIdentityConflicts(t *testing.T) {
	profile := detectionProfile(t, "example.standard.fail-closed", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	catalog, _ := reg.NewCatalog(profile)
	detector := newDetector(t, catalog, detectionCandidate(t, profile, 10, true, false))
	exception, err := reg.NewProbeReadResult(reg.ProbeReadResultSpec{
		Status:        reg.ProbeReadException,
		EvidenceID:    "probe-evidence-exception",
		ExceptionCode: 2,
	})
	if err != nil {
		t.Fatalf("NewProbeReadResult(exception): %v", err)
	}
	incomplete, err := reg.NewProbeReadResult(reg.ProbeReadResultSpec{
		Status:     reg.ProbeReadSucceeded,
		Words:      []uint16{1},
		EvidenceID: "probe-evidence-incomplete",
	})
	if err != nil {
		t.Fatalf("NewProbeReadResult(incomplete): %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*recordedProbeReader)
		reason reg.DetectionReason
	}{
		{name: "reader error", mutate: func(reader *recordedProbeReader) {
			reader.errors["manufacturer-identity"] = errors.New("forced read failure")
		}, reason: reg.DetectionReasonReadError},
		{name: "modbus exception", mutate: func(reader *recordedProbeReader) { reader.results["manufacturer-identity"] = exception }, reason: reg.DetectionReasonProbeException},
		{name: "incomplete words", mutate: func(reader *recordedProbeReader) { reader.results["manufacturer-identity"] = incomplete }, reason: reg.DetectionReasonIncompleteProbe},
		{name: "missing result", mutate: func(reader *recordedProbeReader) { delete(reader.results, "manufacturer-identity") }, reason: reg.DetectionReasonInvalidProbeResult},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := detectionReader(t)
			test.mutate(reader)
			decision, err := detector.Detect(context.Background(), reader, reg.DetectionOptions{})
			if err != nil || decision.Outcome() != reg.DetectionNoMatch ||
				decision.Reason() != test.reason || decision.SelectedProfileID() != "" {
				t.Fatalf("fail-closed decision=(%+v,%v)", decision, err)
			}
		})
	}

	base := detectionPlan(t).Spec()
	duplicateID := base
	duplicateID.Declarations = append(
		append([]reg.ProbeDeclarationSpec(nil), base.Declarations...),
		base.Declarations[0],
	)
	if _, err := reg.NewProbePlan(duplicateID, detectionLimits()); err == nil {
		t.Fatal("duplicate probe declaration was accepted")
	}
	contradictory := base
	secondManufacturer := base.Declarations[1]
	secondManufacturer.ID = "second-manufacturer"
	secondManufacturer.IdentityField = reg.ProbeIdentityManufacturer
	contradictory.Declarations = append(
		append([]reg.ProbeDeclarationSpec(nil), base.Declarations...),
		secondManufacturer,
	)
	if _, err := reg.NewProbePlan(contradictory, detectionLimits()); err == nil {
		t.Fatal("contradictory identity producers were accepted")
	}
}

func TestDetectionBoundsAndImmutableSerializableEvidence(t *testing.T) {
	profile := detectionProfile(t, "example.standard.evidence", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	catalog, _ := reg.NewCatalog(profile)
	limits := detectionLimits()
	invalidLimits := []reg.DetectionLimits{
		{},
		func() reg.DetectionLimits { value := limits; value.MaxPlanDeclarations = 0; return value }(),
		func() reg.DetectionLimits { value := limits; value.MaxReads = 0; return value }(),
		func() reg.DetectionLimits { value := limits; value.MaxWordsPerRead = 0; return value }(),
		func() reg.DetectionLimits { value := limits; value.MaxTotalWords = 0; return value }(),
		func() reg.DetectionLimits { value := limits; value.MaxIdentityBytes = 0; return value }(),
		func() reg.DetectionLimits { value := limits; value.MaxEvidenceIDBytes = 0; return value }(),
		func() reg.DetectionLimits {
			value := limits
			value.MaxDecisionBytes = reg.MaxSerializedContractBytes + 1
			return value
		}(),
	}
	for index, invalid := range invalidLimits {
		_, err := reg.NewProfileDetector(reg.ProfileDetectorSpec{
			DetectorVersion: version(t, "1.0.0"),
			Plan:            detectionPlan(t),
			Catalog:         catalog,
			Candidates:      []reg.DetectionCandidate{detectionCandidate(t, profile, 1, true, false)},
			Limits:          invalid,
		})
		if err == nil {
			t.Fatalf("invalid limits %d were accepted", index)
		}
	}

	maximumWords, err := reg.NewProbeReadResult(reg.ProbeReadResultSpec{
		Status:     reg.ProbeReadSucceeded,
		Words:      make([]uint16, limits.MaxWordsPerRead),
		EvidenceID: strings.Repeat("e", limits.MaxEvidenceIDBytes),
	})
	if err != nil || len(maximumWords.Words()) != limits.MaxWordsPerRead {
		t.Fatalf("maximum probe result was rejected: %v", err)
	}
	if _, err := reg.NewProbeReadResult(reg.ProbeReadResultSpec{
		Status:     reg.ProbeReadSucceeded,
		Words:      make([]uint16, limits.MaxWordsPerRead+1),
		EvidenceID: "probe-evidence-oversized-words",
	}); err == nil {
		t.Fatal("oversized probe words were accepted")
	}
	if _, err := reg.NewProbeReadResult(reg.ProbeReadResultSpec{
		Status:     reg.ProbeReadSucceeded,
		Words:      []uint16{1},
		EvidenceID: strings.Repeat("e", limits.MaxEvidenceIDBytes+1),
	}); err == nil {
		t.Fatal("oversized probe evidence ID was accepted")
	}

	basePlan := detectionPlan(t)
	for name, constrained := range map[string]reg.DetectionLimits{
		"plan declarations": func() reg.DetectionLimits {
			value := limits
			value.MaxPlanDeclarations = len(basePlan.Declarations()) - 1
			return value
		}(),
		"executed reads": func() reg.DetectionLimits {
			value := limits
			value.MaxReads = len(basePlan.Declarations()) - 1
			return value
		}(),
		"aggregate words": func() reg.DetectionLimits {
			value := limits
			value.MaxTotalWords = 16
			return value
		}(),
	} {
		_, err := reg.NewProfileDetector(reg.ProfileDetectorSpec{
			DetectorVersion: version(t, "1.0.0"),
			Plan:            basePlan,
			Catalog:         catalog,
			Candidates:      []reg.DetectionCandidate{detectionCandidate(t, profile, 1, true, false)},
			Limits:          constrained,
		})
		if err == nil {
			t.Fatalf("%s bound was not enforced", name)
		}
	}
	if _, err := reg.NewDetectionCandidate(reg.DetectionCandidateSpec{
		ProfileID:      profile.ID(),
		ProfileVersion: profile.Version(),
		Score:          1,
		Enabled:        true,
		Manufacturer: reg.IdentityStringGateSpec{
			Expected: strings.Repeat("i", limits.MaxIdentityBytes+1),
		},
		Model: reg.IdentityStringGateSpec{Expected: "model-series-a"},
		Firmware: reg.FirmwareGateSpec{
			MinimumInclusive: version(t, "1.0.0"),
			MaximumExclusive: version(t, "2.0.0"),
		},
	}, limits); err == nil {
		t.Fatal("oversized identity gate was accepted")
	}

	detector := newDetector(t, catalog, detectionCandidate(t, profile, 1, true, false))
	decision, err := detector.Detect(context.Background(), detectionReader(t), reg.DetectionOptions{})
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	evidence := decision.Evidence()
	if len(evidence) != 1 ||
		evidence[0].ProfileID != profile.ID() ||
		evidence[0].ProfileVersion != profile.Version() ||
		evidence[0].Reason != reg.DetectionReasonSelected ||
		evidence[0].DetectorVersion != profile.DetectorVersion() ||
		evidence[0].QualificationVersion != profile.QualificationVersion() ||
		!reflect.DeepEqual(evidence[0].MatchedGates, []reg.ProbeIdentityField{
			reg.ProbeIdentityManufacturer,
			reg.ProbeIdentityModel,
			reg.ProbeIdentityFirmware,
		}) || len(evidence[0].ProbeEvidenceIDs) != 3 {
		t.Fatalf("decision evidence=%+v", evidence)
	}
	evidence[0].ProfileID = "mutated"
	evidence[0].MatchedGates[0] = "mutated"
	evidence[0].ProbeEvidenceIDs[0] = "mutated"
	if fresh := decision.Evidence(); fresh[0].ProfileID != profile.ID() ||
		fresh[0].MatchedGates[0] != reg.ProbeIdentityManufacturer ||
		fresh[0].ProbeEvidenceIDs[0] != "probe-evidence-manufacturer" {
		t.Fatal("decision evidence is mutable through an accessor")
	}

	encoded, err := reg.MarshalDetectionDecision(decision)
	if err != nil || len(encoded) > limits.MaxDecisionBytes {
		t.Fatalf("MarshalDetectionDecision=(%d,%v)", len(encoded), err)
	}
	restored, err := reg.UnmarshalDetectionDecision(encoded)
	if err != nil {
		t.Fatalf("UnmarshalDetectionDecision: %v", err)
	}
	reencoded, err := reg.MarshalDetectionDecision(restored)
	if err != nil || !reflect.DeepEqual(reencoded, encoded) {
		t.Fatalf("decision round trip changed: %v", err)
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	object["unknown"] = true
	unknown, _ := json.Marshal(object)
	if _, err := reg.UnmarshalDetectionDecision(unknown); err == nil {
		t.Fatal("unknown decision field was accepted")
	}
	missingObject := make(map[string]any)
	if err := json.Unmarshal(encoded, &missingObject); err != nil {
		t.Fatalf("json.Unmarshal(missing fixture): %v", err)
	}
	delete(missingObject, "reason")
	missing, _ := json.Marshal(missingObject)
	if _, err := reg.UnmarshalDetectionDecision(missing); err == nil {
		t.Fatal("decision with a missing field was accepted")
	}
	caseFolded := strings.Replace(
		string(encoded),
		`"outcome"`,
		`"Outcome"`,
		1,
	)
	if _, err := reg.UnmarshalDetectionDecision([]byte(caseFolded)); err == nil {
		t.Fatal("case-folded decision field was accepted")
	}
	duplicate := strings.Replace(
		string(encoded),
		`"outcome":"matched"`,
		`"outcome":"matched","outcome":"matched"`,
		1,
	)
	if _, err := reg.UnmarshalDetectionDecision([]byte(duplicate)); err == nil {
		t.Fatal("duplicate decision field was accepted")
	}
	impossibleObject := make(map[string]any)
	if err := json.Unmarshal(encoded, &impossibleObject); err != nil {
		t.Fatalf("json.Unmarshal(impossible fixture): %v", err)
	}
	impossibleEvidence := impossibleObject["evidence"].([]any)[0].(map[string]any)
	impossibleEvidence["matched_gates"] = []any{}
	impossibleEvidence["probe_evidence_ids"] = []any{}
	impossible, _ := json.Marshal(impossibleObject)
	if _, err := reg.UnmarshalDetectionDecision(impossible); err == nil {
		t.Fatal("selected evidence without matched gates was accepted")
	}
	if _, err := reg.UnmarshalDetectionDecision(
		[]byte(strings.Repeat("x", reg.MaxSerializedContractBytes+1)),
	); err == nil {
		t.Fatal("oversized decision was accepted")
	}
}

func TestDetectorRejectsDuplicateAndInexactCatalogBindingsBeforeReads(t *testing.T) {
	profile := detectionProfile(t, "example.standard.bound", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	catalog, _ := reg.NewCatalog(profile)
	candidate := detectionCandidate(t, profile, 1, true, false)
	base := reg.ProfileDetectorSpec{
		DetectorVersion: version(t, "1.0.0"),
		Plan:            detectionPlan(t),
		Catalog:         catalog,
		Candidates:      []reg.DetectionCandidate{candidate, candidate},
		Limits:          detectionLimits(),
	}
	if _, err := reg.NewProfileDetector(base); err == nil {
		t.Fatal("duplicate candidate profile binding was accepted")
	}

	otherProfile := detectionProfile(t, "example.standard.bound", "2.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	inexact := detectionCandidate(t, otherProfile, 1, true, false)
	base.Candidates = []reg.DetectionCandidate{inexact}
	if _, err := reg.NewProfileDetector(base); err == nil {
		t.Fatal("candidate with an inexact catalog version was accepted")
	}

	wrongContractSpec := profile.Spec()
	wrongContractSpec.ID = "example.standard.wrong-contract"
	wrongContractSpec.DetectorVersion = version(t, "2.0.0")
	wrongContract, err := reg.NewProfileDescriptor(wrongContractSpec)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(wrong contract): %v", err)
	}
	wrongCatalog, _ := reg.NewCatalog(wrongContract)
	base.Catalog = wrongCatalog
	base.Candidates = []reg.DetectionCandidate{detectionCandidate(t, wrongContract, 1, true, false)}
	if _, err := reg.NewProfileDetector(base); err == nil {
		t.Fatal("profile with an inexact detector contract was accepted")
	}

	base.Catalog = catalog
	base.Candidates = []reg.DetectionCandidate{candidate}
	base.Limits.MaxDecisionBytes = 100
	if _, err := reg.NewProfileDetector(base); err == nil {
		t.Fatal("detector with an infeasible decision bound was accepted")
	}
}
