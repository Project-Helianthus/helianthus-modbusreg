package modbusreg

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"testing"
)

type sunSpecFroniusAuditDisposition struct {
	Schema    string `json:"schema"`
	Milestone string `json:"milestone"`
	Outcome   string `json:"outcome"`
	Docs      struct {
		MergeSHA         string                `json:"merge_sha"`
		LogicalSchemaID  SunSpecSchemaRevision `json:"logical_schema_id"`
		RegistryRevision SunSpecSchemaRevision `json:"registry_revision"`
		SourceCommit     string                `json:"source_commit"`
	} `json:"docs"`
	Registry struct {
		BaseSHA                   string   `json:"base_sha"`
		DecoderBehaviorChanged    bool     `json:"decoder_behavior_changed"`
		ExactModelCatalogVerified bool     `json:"exact_model_catalog_verified"`
		UnknownRetentionVerified  bool     `json:"unknown_retention_verified"`
		Equivalence103113Verified bool     `json:"equivalence_103_113_verified"`
		SentinelHandlingVerified  bool     `json:"sentinel_handling_verified"`
		ExecutableEvidence        []string `json:"executable_evidence"`
	} `json:"registry"`
	Fronius struct {
		FlavorID                    string           `json:"flavor_id"`
		Manufacturer                string           `json:"manufacturer"`
		Model                       string           `json:"model"`
		Firmware                    string           `json:"firmware"`
		Chain                       []SunSpecWireKey `json:"chain"`
		OfflineFixture              string           `json:"offline_fixture"`
		OfflineClassifierExecutable bool             `json:"offline_classifier_executable"`
		AutomaticRuntimeAdmission   bool             `json:"automatic_runtime_admission"`
		LiveQualified               bool             `json:"live_qualified"`
		WriteAuthority              bool             `json:"write_authority"`
		SupportClaim                bool             `json:"support_claim"`
	} `json:"fronius"`
}

type sunSpecFroniusPublicEvidence struct {
	Schema         string `json:"schema"`
	Profile        string `json:"profile"`
	ProfileVersion string `json:"profile_version"`
	PublicSources  []struct {
		ID      string `json:"id"`
		Locator string `json:"locator"`
		License string `json:"license"`
	} `json:"public_sources"`
	Applicability []string `json:"applicability"`
	License       string   `json:"license"`
}

func readSunSpecFroniusAuditDisposition(t *testing.T) sunSpecFroniusAuditDisposition {
	t.Helper()
	data, err := os.ReadFile("profiles/vendor/fronius/disposition.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var disposition sunSpecFroniusAuditDisposition
	if err := decoder.Decode(&disposition); err != nil {
		t.Fatal(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("trailing JSON value: %v", err)
	}
	return disposition
}

func TestSunSpecFroniusAuditClosesAgainstPinnedRegistry(t *testing.T) {
	disposition := readSunSpecFroniusAuditDisposition(t)
	if disposition.Schema != "helianthus-modbusreg-sunspec-fronius-audit/v1" ||
		disposition.Milestone != "REG-SF-01" || disposition.Outcome != "OFFLINE_PROFILE_ADMITTED" {
		t.Fatalf("unexpected audit identity: %#v", disposition)
	}
	if disposition.Docs.MergeSHA != "2abcb26a06ffa71149177de2cc2817a24d82081f" ||
		disposition.Docs.LogicalSchemaID != "sunspec.models.v1" ||
		disposition.Docs.RegistryRevision != SunSpecModelsRevisionV1 ||
		disposition.Docs.SourceCommit != "7abdf8982d5364f8ae916deee18aac86c11be36d" {
		t.Fatalf("unexpected docs binding: %#v", disposition.Docs)
	}
	wantEvidence := []string{
		"TestSunSpecDecoderRegistryUsesExactImmutableKeys",
		"TestSunSpecChainRetainsOrderedDuplicatesUnknownAndWrongLength",
		"TestSunSpecModels103And113CanonicalEquivalence",
		"TestSunSpecValueSentinelsAndExactDecimal",
		"TestFroniusObservedFlavorV11MatchesOnlyTheModel123Chain",
	}
	if disposition.Registry.BaseSHA != "b7248d6c60529fd11c4ea02c9f374dd2dccd6536" ||
		disposition.Registry.DecoderBehaviorChanged ||
		!disposition.Registry.ExactModelCatalogVerified ||
		!disposition.Registry.UnknownRetentionVerified ||
		!disposition.Registry.Equivalence103113Verified ||
		!disposition.Registry.SentinelHandlingVerified ||
		!reflect.DeepEqual(disposition.Registry.ExecutableEvidence, wantEvidence) {
		t.Fatalf("unexpected registry audit: %#v", disposition.Registry)
	}

	wantChain := []SunSpecWireKey{{1, 65}, {113, 60}, {120, 26}, {121, 30}, {122, 44}, {123, 24}, {160, 88}, {124, 24}, {0xffff, 0}}
	if disposition.Fronius.FlavorID != SunSpecFroniusObservedFlavorV11ID ||
		disposition.Fronius.Manufacturer != "Fronius" ||
		disposition.Fronius.Model != "Symo GEN24 10.0" ||
		disposition.Fronius.Firmware != "1.41.11-1" ||
		!reflect.DeepEqual(disposition.Fronius.Chain, wantChain) ||
		disposition.Fronius.OfflineFixture != "testdata/sunspec/chains/fronius_gen24_float_v1_1.json" ||
		!disposition.Fronius.OfflineClassifierExecutable ||
		disposition.Fronius.AutomaticRuntimeAdmission || disposition.Fronius.LiveQualified ||
		disposition.Fronius.WriteAuthority || disposition.Fronius.SupportClaim {
		t.Fatalf("unexpected Fronius disposition: %#v", disposition.Fronius)
	}
}

func TestSunSpecFroniusAuditUsesPublicProtocolEvidence(t *testing.T) {
	data, err := os.ReadFile("profiles/vendor/fronius/evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var evidence sunSpecFroniusPublicEvidence
	if err := decoder.Decode(&evidence); err != nil {
		t.Fatal(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("trailing JSON value: %v", err)
	}
	wantSources := []struct {
		ID      string `json:"id"`
		Locator string `json:"locator"`
		License string `json:"license"`
	}{
		{
			ID:      "fronius-sunspec-float-v1",
			Locator: "https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/2abcb26a06ffa71149177de2cc2817a24d82081f/protocols/fronius/sunspec-float-v1.md",
			License: "CC0-1.0",
		},
	}
	wantApplicability := []string{
		"Fronius Symo GEN24 10.0 firmware 1.41.11-1",
		"exact SunSpec V1.1 chain ending in ffff/0",
		"offline read-only fixture classification only; no live support or write authority",
	}
	if evidence.Schema != "helianthus-modbusreg-vendor-evidence/v1" || evidence.Profile != "fronius" ||
		evidence.ProfileVersion != "1.0.0" || evidence.License != "CC0-1.0" ||
		!reflect.DeepEqual(evidence.PublicSources, wantSources) ||
		!reflect.DeepEqual(evidence.Applicability, wantApplicability) {
		t.Fatalf("unexpected Fronius public evidence: %#v", evidence)
	}
}
