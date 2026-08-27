package modbusreg

import (
	"os"
	"strings"
	"testing"
)

func TestStableTestTaxonomyUsesVendorAndCapabilityNames(t *testing.T) {
	for _, name := range []string{
		"outback_axs_flavor_contract_test.go",
		"outback_axs_sunspec_observation_test.go",
		"profile_codec_contract_test.go",
		"profile_coherence_contract_test.go",
		"registry_chronology_contract_test.go",
		"completion_record_m303_contract_test.go",
	} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("taxonomy file %q: %v", name, err)
		}
	}
	content, err := os.ReadFile("completion_record_m303_contract_test.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "TestFMV3M303") || !strings.Contains(string(content), "TestM303") {
		t.Fatal("M303 completion tests do not use the stable milestone label")
	}
}
