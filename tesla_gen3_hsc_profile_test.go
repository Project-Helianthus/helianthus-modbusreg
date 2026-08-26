package modbusreg

import "testing"

func TestTeslaGen3HSCProfileSeparatesKnownCandidateAndUnknownVersions(t *testing.T) {
	known, err := NewTeslaGen3HSCProfile(TeslaGen3HSCProfileConfig{
		Enabled: true, Version: "24.28.3", ActivationCapable: true,
		PrivateFunctionCapable: true, OperationCapable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if known.VersionDisposition() != TeslaGen3HSCVersionKnownObservation || !known.ExchangeEligible() {
		t.Fatalf("known=%#v", known)
	}

	candidate, err := NewTeslaGen3HSCProfile(TeslaGen3HSCProfileConfig{
		Enabled: true, Version: "25.1.0", ActivationCapable: true,
		PrivateFunctionCapable: true, OperationCapable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if candidate.VersionDisposition() != TeslaGen3HSCVersionCompatibleCandidate || !candidate.ExchangeEligible() {
		t.Fatalf("candidate=%#v", candidate)
	}

	unknown, err := NewTeslaGen3HSCProfile(TeslaGen3HSCProfileConfig{Enabled: true, Version: "", ActivationCapable: true})
	if err != nil {
		t.Fatal(err)
	}
	if unknown.VersionDisposition() != TeslaGen3HSCVersionUnknown || unknown.ExchangeEligible() {
		t.Fatalf("unknown=%#v", unknown)
	}
}
