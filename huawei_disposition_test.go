package modbusreg

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const (
	huaweiDocsMergeSHA = "fa8838c92f3ce2eac3ad953d7059eccc21be19c7"
	currentModbusPin   = "v0.3.0"
)

func decodeHuaweiObject(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("trailing JSON value: %v", err)
	}
	return value
}

func requireHuaweiKeys(t *testing.T, value map[string]any, keys ...string) {
	t.Helper()
	actual := make([]string, 0, len(value))
	for key := range value {
		actual = append(actual, key)
	}
	want := append([]string(nil), keys...)
	sort.Strings(actual)
	sort.Strings(want)
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("keys=%v want=%v", actual, want)
	}
}

func huaweiStringSlice(t *testing.T, value any) []string {
	t.Helper()
	raw, ok := value.([]any)
	if !ok {
		t.Fatalf("not an array: %#v", value)
	}
	result := make([]string, len(raw))
	for index, item := range raw {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("array item %d is not text: %#v", index, item)
		}
		result[index] = text
	}
	return result
}

func huaweiObject(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("not an object: %#v", value)
	}
	return result
}

func huaweiInt(t *testing.T, value any) int {
	t.Helper()
	number, ok := value.(json.Number)
	if !ok {
		t.Fatalf("not a number: %#v", value)
	}
	integer, err := number.Int64()
	if err != nil {
		t.Fatal(err)
	}
	return int(integer)
}

func TestHuaweiGatewayEvidenceRetainsIndependentFamilyContracts(t *testing.T) {
	evidence := decodeHuaweiObject(t, "profiles/vendor/huawei/evidence.json")
	requireHuaweiKeys(t, evidence, "schema", "profile", "profile_version", "public_sources", "applicability", "license")
	if evidence["schema"] != "helianthus-modbusreg-vendor-evidence/v1" ||
		evidence["profile"] != "huawei" || evidence["profile_version"] != "1.0.0" ||
		evidence["license"] != "CC0-1.0" {
		t.Fatalf("unexpected Huawei evidence identity: %#v", evidence)
	}
}

func TestHuaweiEMMAOfflineDispositionIsDefaultDeniedAndPinnedToDocs(t *testing.T) {
	goMod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	wantDependency := "github.com/Project-Helianthus/helianthus-modbus " + currentModbusPin
	if !strings.Contains(string(goMod), wantDependency) {
		t.Fatalf("missing exact transport dependency %q", wantDependency)
	}

	privateEndpoint := regexp.MustCompile(`(?m)\b(?:10|127|169\.254|172\.(?:1[6-9]|2\d|3[01])|192\.168)\.`)
	path := "profiles/vendor/huawei/emma-disposition.json"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if privateEndpoint.Match(data) || bytes.Contains(bytes.ToLower(data), []byte("credential")) {
		t.Fatal("disposition contains private endpoint or credential material")
	}
	record := decodeHuaweiObject(t, path)
	requireHuaweiKeys(t, record, "schema", "profile", "profile_version", "outcome", "evidence", "offline_identity_gate_registered", "catalog_registered", "automatic_runtime_admission", "support_claim", "detector", "firmware_gates", "child_enumeration", "decoder_keys", "reopen_requires")
	if record["schema"] != "helianthus-modbusreg-huawei-emma-disposition/v2" || record["profile"] != HuaweiEMMAReadOnlyProfileID ||
		record["profile_version"] != "1.0.0" || record["outcome"] != "OFFLINE_IDENTITY_ADMITTED" ||
		record["offline_identity_gate_registered"] != true || record["catalog_registered"] != false ||
		record["automatic_runtime_admission"] != false || record["support_claim"] != false || len(record["decoder_keys"].([]any)) != 0 {
		t.Fatalf("unexpected disposition: %#v", record)
	}

	source := huaweiObject(t, record["evidence"])
	requireHuaweiKeys(t, source, "docs_merge_sha", "candidate_id", "canonical_class", "eligible", "missing_capability_evidence")
	if source["docs_merge_sha"] != huaweiDocsMergeSHA || source["candidate_id"] != "huawei.emma.offline.v1" ||
		source["canonical_class"] != HuaweiEMMACanonicalClass || source["eligible"] != true {
		t.Fatalf("unexpected evidence: %#v", source)
	}

	detector := huaweiObject(t, record["detector"])
	requireHuaweiKeys(t, detector, "registered", "executable", "default_denied", "normalization", "model_aliases", "required_tuple", "optional_enrichment", "forbidden_identity", "multiple_positive_outcome", "first_match_priority")
	if detector["registered"] != false || detector["executable"] != true || detector["default_denied"] != true ||
		detector["normalization"] != "terminal_nul_or_space_only" || detector["multiple_positive_outcome"] != "INSUFFICIENT_EVIDENCE" || detector["first_match_priority"] != false ||
		!reflect.DeepEqual(huaweiStringSlice(t, detector["model_aliases"]), []string{"EMMA-A01", "EMMA-A02"}) ||
		!reflect.DeepEqual(huaweiStringSlice(t, detector["required_tuple"]), []string{"exact_model_alias", "same_branch_firmware_floor"}) ||
		!reflect.DeepEqual(huaweiStringSlice(t, detector["optional_enrichment"]), []string{"basic_mei", "extended_mei"}) ||
		!reflect.DeepEqual(huaweiStringSlice(t, detector["forbidden_identity"]), []string{"offering_name", "serial_number_30015", "basic_mei_only", "extended_mei_only", "prefix_or_wildcard_model_match"}) {
		t.Fatalf("unexpected detector disposition: %#v", detector)
	}

	gates := huaweiObject(t, record["firmware_gates"])
	requireHuaweiKeys(t, gates, "V100R024C00", "V100R025C00")
	if huaweiInt(t, huaweiObject(t, gates["V100R024C00"])["minimum_spc"]) != 100 ||
		huaweiInt(t, huaweiObject(t, gates["V100R025C00"])["minimum_spc"]) != 102 {
		t.Fatalf("unexpected firmware gates: %#v", gates)
	}
}

func TestHuaweiFamilyDispositionsPinOwnDocumentation(t *testing.T) {
	tests := []struct {
		name, path, docsMergeSHA string
	}{
		{"SmartLogger", "profiles/vendor/huawei/smartlogger-disposition.json", "2abcb26a06ffa71149177de2cc2817a24d82081f"},
		{"S-Dongle", "profiles/vendor/huawei/sdongle-disposition.json", "2d44c22f27c4be0e1de460f63b8330c728a5af8f"},
		{"EMMA", "profiles/vendor/huawei/emma-disposition.json", huaweiDocsMergeSHA},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := decodeHuaweiObject(t, test.path)
			evidence := huaweiObject(t, record["evidence"])
			if evidence["docs_merge_sha"] != test.docsMergeSHA {
				t.Fatalf("docs merge SHA=%q want %q", evidence["docs_merge_sha"], test.docsMergeSHA)
			}
		})
	}
}
