package tesla

import (
	"context"
	"strings"
	"testing"
)

func TestTeslaReadVersionFloor(t *testing.T) {
	for _, tc := range []struct {
		version, reason string
		grade           ReadApplicabilityGrade
	}{
		{"wc3_24_44_4", "", ReadGradeHypothesis}, {"24.44.3", "", ReadGradeProven}, {"26.18.0", "", ReadGradeHypothesis}, {"MID 25.2.2", "", ReadGradeHypothesis},
		{"wc3_24_44_2", "below_floor", ""}, {"wc3_24_28_3", "below_floor", ""}, {"24.28.3", "below_floor", ""}, {"", "version_unidentified", ""}, {"wc3_24_44_3/4", "version_unidentified", ""}, {string(LegacyWallConnectorGenerationCandidate), "legacy_profile_required", ""},
	} {
		t.Run(tc.version, func(t *testing.T) {
			config := teslaGen3ReadTestConfig(t, nil)
			declared := config.Gen3.config
			declared.Version = tc.version
			config.Gen3 = mustTeslaGen3Profile(t, declared)
			got, err := NewGen3ReadAcquisition(config)
			if tc.reason != "" {
				if got != nil || err == nil || !strings.Contains(err.Error(), tc.reason) {
					t.Fatalf("wanted %s, got %v", tc.reason, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := got.Close(); err != nil {
					t.Error(err)
				}
			})
			if err := got.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			endpoint := got.endpoint.(*teslaGen3FakeEndpoint)
			for _, operation := range config.Operations {
				endpoint.push(teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, operation, nil)})
			}
			records, err := got.PollOnce(context.Background())
			if err != nil || len(records) != 5 || endpoint.exchanges() != 5 {
				t.Fatalf("poll = %d records, %d exchanges, %v", len(records), endpoint.exchanges(), err)
			}
			for i, record := range records {
				if len(record.RequestADU()) == 0 || record.State != Gen3ReadEvidenceAck || record.Operation != config.Operations[i] || record.ReadApplicabilityGrade != tc.grade || record.Gen3Version != tc.version || record.Gen3VersionDisposition != config.Gen3.VersionDisposition() {
					t.Fatalf("record %d = %+v", i, record)
				}
			}
		})
	}
}

func TestTeslaReadVersionRequiresEveryCapability(t *testing.T) {
	for _, disable := range []func(*Gen3HSCProfileConfig){
		func(c *Gen3HSCProfileConfig) { c.Enabled = false },
		func(c *Gen3HSCProfileConfig) { c.ActivationCapable = false },
		func(c *Gen3HSCProfileConfig) { c.PrivateFunctionCapable = false },
		func(c *Gen3HSCProfileConfig) { c.OperationCapable = false },
	} {
		config := teslaGen3ReadTestConfig(t, nil)
		declared := config.Gen3.config
		declared.Version = "wc3_24_44_4"
		disable(&declared)
		config.Gen3 = mustTeslaGen3Profile(t, declared)
		got, err := NewGen3ReadAcquisition(config)
		if got != nil || err == nil || !strings.Contains(err.Error(), "capability_incompatible") {
			t.Fatalf("capability mismatch: %v", err)
		}
	}
}
