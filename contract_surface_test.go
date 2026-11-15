package modbusreg_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestVersionValidAndCompareVersionsOrderNumerically(t *testing.T) {
	if (reg.Version{}).Valid() || !reg.MustParseVersion("0.0.0").Valid() {
		t.Fatal("zero Version must be invalid and 0.0.0 valid")
	}
	for _, tc := range []struct {
		first, second string
		want          int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2.3", "2.2.3", -1},
		{"2.2.3", "1.2.3", 1},
		{"1.2.3", "1.3.3", -1},
		{"1.3.3", "1.2.3", 1},
		{"1.2.3", "1.2.4", -1},
		{"1.2.4", "1.2.3", 1},
		{"9.0.0", "10.0.0", -1},
		{"1.10.0", "1.9.0", 1},
		{"1.0.10", "1.0.9", 1},
	} {
		first, second := reg.MustParseVersion(tc.first), reg.MustParseVersion(tc.second)
		if got := reg.CompareVersions(first, second); got != tc.want {
			t.Fatalf("CompareVersions(%s, %s)=%d, want %d", tc.first, tc.second, got, tc.want)
		}
	}
	valid := reg.MustParseVersion("0.0.0")
	if reg.CompareVersions(reg.Version{}, valid) != -1 ||
		reg.CompareVersions(valid, reg.Version{}) != 1 ||
		reg.CompareVersions(reg.Version{}, reg.Version{}) != 0 {
		t.Fatal("invalid versions must order before every valid version")
	}
}

func TestValidDependencySetIDAcceptsOnlyLowerHexDigest(t *testing.T) {
	sum := sha256.Sum256([]byte("synthetic dependency set"))
	digest := hex.EncodeToString(sum[:])
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{"sha256:" + digest, true},
		{"sha256:" + strings.ToUpper(digest), false},
		{digest, false},
		{"sha256:" + digest[1:], false},
		{"sha256:" + digest + "0", false},
		{"sha512:" + digest, false},
		{"", false},
	} {
		if got := reg.ValidDependencySetID(tc.value); got != tc.valid {
			t.Fatalf("ValidDependencySetID(%q)=%v", tc.value, got)
		}
	}
}

func TestValidateBoundedStringNamesFieldAndBoundsBytes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		value    string
		required bool
		valid    bool
	}{
		{"required_empty", "", true, false},
		{"optional_empty", "", false, true},
		{"exact_limit", strings.Repeat("a", reg.MaxContractStringBytes), true, true},
		{"one_byte_over", strings.Repeat("a", reg.MaxContractStringBytes+1), false, false},
		{"invalid_encoding", "\xff", false, false},
	} {
		err := reg.ValidateBoundedString("synthetic field", tc.value, tc.required)
		if (err == nil) != tc.valid || (err != nil && !strings.Contains(err.Error(), "synthetic field")) {
			t.Fatalf("%s: err=%v, want valid=%v", tc.name, err, tc.valid)
		}
	}
}

func TestPreflightAggregateBoundsCumulativeBudget(t *testing.T) {
	half := strings.Repeat("a", 400_000)
	if err := reg.PreflightAggregate(half, true, 7, []uint16{1}, struct{ Name string }{"a"}, time.Time{}); err != nil {
		t.Fatalf("bounded values rejected: %v", err)
	}
	if err := reg.PreflightAggregate(half, half); err == nil {
		t.Fatal("values over the cumulative budget were accepted")
	}
	for name, value := range map[string]any{"map": map[string]int{}, "float": 1.5, "channel": make(chan int)} {
		if err := reg.PreflightAggregate(value); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

func TestDecodeStrictAndMarshalBoundedKeepOneCompactValue(t *testing.T) {
	type record struct {
		Name string `json:"name"`
	}
	var decoded record
	if err := reg.DecodeStrict([]byte(`{"name":"a"}`), &decoded); err != nil || decoded.Name != "a" {
		t.Fatalf("DecodeStrict=%+v err=%v", decoded, err)
	}
	for name, data := range map[string]string{
		"unknown_field": `{"name":"a","other":1}`,
		"duplicate_key": `{"name":"a","name":"b"}`,
		"trailing":      `{"name":"a"} {}`,
		"empty":         ``,
	} {
		var target record
		if err := reg.DecodeStrict([]byte(data), &target); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	if err := reg.DecodeStrict([]byte(`{"name":"a"}`), record{}); err == nil {
		t.Fatal("non-pointer target was accepted")
	}
	encoded, err := reg.MarshalBounded(record{Name: "<a>"})
	if err != nil || string(encoded) != `{"name":"\u003ca\u003e"}` {
		t.Fatalf("MarshalBounded=%s err=%v", encoded, err)
	}
	if _, err := reg.MarshalBounded(record{Name: strings.Repeat("a", reg.MaxSerializedContractBytes)}); err == nil {
		t.Fatal("oversized record was encoded")
	}
}

func TestParseRequiredVersionNamesField(t *testing.T) {
	if parsed, err := reg.ParseRequiredVersion("profile_version", "1.2.3"); err != nil || parsed.String() != "1.2.3" {
		t.Fatalf("ParseRequiredVersion=%v err=%v", parsed, err)
	}
	if _, err := reg.ParseRequiredVersion("profile_version", "1.2"); err == nil || !strings.HasPrefix(err.Error(), "profile_version: ") {
		t.Fatalf("error does not name the field: %v", err)
	}
}

// Each case zeroes or exceeds one ledger bound; the production check of that
// bound is what rejects it.
func TestValidateLedgerLimitsChecksEveryBound(t *testing.T) {
	if err := reg.ValidateLedgerLimits(reg.DefaultLedgerLimits()); err != nil {
		t.Fatalf("default limits rejected: %v", err)
	}
	cases := map[string]func(*reg.LedgerLimits){
		"max_retained_attempts":                   func(l *reg.LedgerLimits) { l.MaxRetainedAttempts = 0 },
		"max_claim_entries_per_attempt":           func(l *reg.LedgerLimits) { l.MaxClaimEntriesPerAttempt = 0 },
		"max_retained_claim_entries":              func(l *reg.LedgerLimits) { l.MaxRetainedClaimEntries = 0 },
		"max_dependency_set_encoded_bytes":        func(l *reg.LedgerLimits) { l.MaxDependencySetEncodedBytes = 0 },
		"attempt_key_max_bytes":                   func(l *reg.LedgerLimits) { l.AttemptKeyMaxUTF8Bytes = 0 },
		"normalization_record_max_encoded_bytes":  func(l *reg.LedgerLimits) { l.NormalizationRecordMaxEncodedBytes = 0 },
		"retained_diagnostic_count_per_object":    func(l *reg.LedgerLimits) { l.RetainedDiagnosticCountPerObjectMax = 0 },
		"retained_diagnostic_max_bytes":           func(l *reg.LedgerLimits) { l.RetainedDiagnosticMaxUTF8Bytes = 0 },
		"audit_tombstone_limit":                   func(l *reg.LedgerLimits) { l.AuditTombstoneLimit = 0 },
		"audit_tombstone_max_encoded_bytes":       func(l *reg.LedgerLimits) { l.AuditTombstoneMaxEncodedBytes = 0 },
		"retained_attempts_over_maximum":          func(l *reg.LedgerLimits) { l.MaxRetainedAttempts = 1<<16 + 1 },
		"claim_entries_per_attempt_over_maximum":  func(l *reg.LedgerLimits) { l.MaxClaimEntriesPerAttempt = reg.MaxProfileDependencies + 1 },
		"retained_claims_below_per_attempt_bound": func(l *reg.LedgerLimits) { l.MaxRetainedClaimEntries = l.MaxClaimEntriesPerAttempt - 1 },
		"retained_claims_over_product": func(l *reg.LedgerLimits) {
			l.MaxRetainedClaimEntries = l.MaxRetainedAttempts*l.MaxClaimEntriesPerAttempt + 1
		},
	}
	for name, mutate := range cases {
		limits := reg.DefaultLedgerLimits()
		mutate(&limits)
		if err := reg.ValidateLedgerLimits(limits); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	if reg.MaxLedgerRestartAuditTombstones != reg.MaxSerializedContractBytes/64 {
		t.Fatalf("restart tombstone bound = %d", reg.MaxLedgerRestartAuditTombstones)
	}
}

// Each variant changes one accessor's field from the fixture's declaration.
func TestProfileDescriptorAccessorsReturnDeclaredPolicy(t *testing.T) {
	base := profileFixture(t).Spec()
	bounded := base.Coherence
	bounded.Mode = reg.CoherenceBoundedMultiResponse
	bounded.MaximumSourceSkew, bounded.MaximumReceiptSkew = time.Second, 2*time.Second
	bounded.RequireGenerationEquality = true
	bounded.AcquisitionOrder = reg.AcquisitionOrderDependencyDeclaration
	bounded.RetrySetBehavior = reg.RetryWholeSet
	for name, mutate := range map[string]func(*reg.ProfileDescriptorSpec){
		"fixture":   func(*reg.ProfileDescriptorSpec) {},
		"coherence": func(spec *reg.ProfileDescriptorSpec) { spec.Coherence = bounded },
		"maturity":  func(spec *reg.ProfileDescriptorSpec) { spec.Maturity = reg.MaturityQualified },
		"default_enabled": func(spec *reg.ProfileDescriptorSpec) {
			spec.Maturity, spec.DefaultEnabled = reg.MaturityQualified, true
		},
		"state": func(spec *reg.ProfileDescriptorSpec) { spec.State = reg.ProfileRevoked },
	} {
		t.Run(name, func(t *testing.T) {
			spec := profileFixture(t).Spec()
			mutate(&spec)
			profile, err := reg.NewProfileDescriptor(spec)
			if err != nil {
				t.Fatalf("NewProfileDescriptor: %v", err)
			}
			if profile.Coherence() != spec.Coherence || profile.Maturity() != spec.Maturity ||
				profile.DefaultEnabled() != spec.DefaultEnabled || profile.State() != spec.State {
				t.Fatalf("accessors disagree with the declaration %+v", spec)
			}
		})
	}
}
