package modbusreg

import (
	"bytes"
	"testing"
)

func TestTeslaGen3HSCProfileSeparatesKnownCandidateAndUnknownVersions(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		disposition TeslaGen3HSCVersionDisposition
		eligible    bool
	}{
		{name: "first raw known observation", version: "24.28.3", disposition: TeslaGen3HSCVersionKnownObservation, eligible: true},
		{name: "second raw known observation", version: "24.44.3", disposition: TeslaGen3HSCVersionKnownObservation, eligible: true},
		{name: "first documented profile", version: "wc3_24_28_3", disposition: TeslaGen3HSCVersionKnownObservation, eligible: true},
		{name: "second documented profile", version: "wc3_24_44_3", disposition: TeslaGen3HSCVersionKnownObservation, eligible: true},
		{name: "candidate", version: "25.1.0", disposition: TeslaGen3HSCVersionCompatibleCandidate, eligible: true},
		{name: "whitespace unknown", version: " \t", disposition: TeslaGen3HSCVersionUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile, err := NewTeslaGen3HSCProfile(TeslaGen3HSCProfileConfig{
				Enabled: true, Version: test.version, ActivationCapable: true,
				PrivateFunctionCapable: true, OperationCapable: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if profile.VersionDisposition() != test.disposition || profile.ExchangeEligible() != test.eligible {
				t.Fatalf("profile=%#v", profile)
			}
		})
	}
}

func TestTeslaGen3HSCProfileRetainsBoundedVersionEvidence(t *testing.T) {
	evidence := []byte{0x76, 0x32, 0x35, 0x2e, 0x31, 0x2e, 0x30}
	profile, err := NewTeslaGen3HSCProfile(TeslaGen3HSCProfileConfig{
		Enabled: true, Version: "25.1.0", VersionEvidence: evidence,
		ActivationCapable: true, PrivateFunctionCapable: true, OperationCapable: true,
	})
	if err != nil || profile.Version() != "25.1.0" || !bytes.Equal(profile.VersionEvidence(), evidence) {
		t.Fatalf("profile=%#v err=%v", profile, err)
	}
	evidence[0] = 0
	if got := profile.VersionEvidence(); !bytes.Equal(got, []byte{0x76, 0x32, 0x35, 0x2e, 0x31, 0x2e, 0x30}) {
		t.Fatalf("evidence mutated: %x", got)
	}

	unknown, err := NewTeslaGen3HSCProfile(TeslaGen3HSCProfileConfig{VersionEvidence: []byte{0xff}})
	if err != nil || unknown.VersionDisposition() != TeslaGen3HSCVersionUnknown || !bytes.Equal(unknown.VersionEvidence(), []byte{0xff}) {
		t.Fatalf("unknown=%#v err=%v", unknown, err)
	}
}

func TestTeslaGen3HSCProfileRequiresEveryCapability(t *testing.T) {
	for _, denied := range []string{"enabled", "activation", "private function", "operation"} {
		t.Run(denied, func(t *testing.T) {
			config := TeslaGen3HSCProfileConfig{Enabled: true, Version: "24.44.3", ActivationCapable: true, PrivateFunctionCapable: true, OperationCapable: true}
			switch denied {
			case "enabled":
				config.Enabled = false
			case "activation":
				config.ActivationCapable = false
			case "private function":
				config.PrivateFunctionCapable = false
			case "operation":
				config.OperationCapable = false
			}
			profile, err := NewTeslaGen3HSCProfile(config)
			if err != nil || profile.ExchangeEligible() {
				t.Fatalf("profile=%#v err=%v", profile, err)
			}
			candidateConfig := config
			candidateConfig.Version = "25.1.0"
			candidate, err := NewTeslaGen3HSCProfile(candidateConfig)
			wantDisposition := TeslaGen3HSCVersionUnknown
			if denied == "enabled" {
				wantDisposition = TeslaGen3HSCVersionCompatibleCandidate
			}
			if err != nil || candidate.VersionDisposition() != wantDisposition || candidate.ExchangeEligible() {
				t.Fatalf("candidate=%#v err=%v", candidate, err)
			}
		})
	}
}
