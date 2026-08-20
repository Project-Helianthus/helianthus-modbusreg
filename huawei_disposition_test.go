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
	huaweiDocsMergeSHA = "aa67e0c2a7c2042c7c1dccad6ebe3c4900dab04f"
	huaweiModbusMerge  = "c78030472c24f0f2b849fd30124611157a81f834"
	huaweiModbusPin    = "v0.0.0-20260820212315-c78030472c24"
)

type huaweiFamilyExpectation struct {
	file                  string
	profile               string
	candidateID           string
	gatewayKind           string
	missingDiscriminators []string
	requiredTuple         []string
	forbiddenIdentity     []string
	firmwareGateCount     int
	negativeFixtures      []string
	unitTarget            string
	maxChildren           int
	maxPages              int
	maxObjects            int
	maxBytes              int
	reject                []string
}

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

func TestHuaweiGatewayDispositionsFailClosedIndependently(t *testing.T) {
	goMod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	wantDependency := "github.com/Project-Helianthus/helianthus-modbus " + huaweiModbusPin
	if !strings.Contains(string(goMod), wantDependency) {
		t.Fatalf("missing exact transport dependency %q", wantDependency)
	}

	evidence := decodeHuaweiObject(t, "profiles/vendor/huawei/evidence.json")
	requireHuaweiKeys(t, evidence, "schema", "profile", "profile_version", "public_sources", "applicability", "license")
	if evidence["schema"] != "helianthus-modbusreg-vendor-evidence/v1" ||
		evidence["profile"] != "huawei" || evidence["profile_version"] != "1.0.0" ||
		evidence["license"] != "CC0-1.0" {
		t.Fatalf("unexpected Huawei evidence identity: %#v", evidence)
	}

	expectations := []huaweiFamilyExpectation{
		{
			file: "smartlogger-disposition.json", profile: "huawei.smartlogger", candidateID: "huawei.smartlogger.v1", gatewayKind: "SmartLogger",
			missingDiscriminators: []string{"negative_overlap_with_emma", "sanitized_self_entry_and_firmware_fixture"},
			requiredTuple:         []string{"device_list_change_readable", "mei_child_inventory_self_entry_model_smartlogger", "structured_firmware_gate"},
			forbiddenIdentity:     []string{"writable_device_name_65524", "esn_only", "basic_mei_only"},
			firmwareGateCount:     8, negativeFixtures: []string{"smartlogger-unknown-v300r024-spc200", "smartlogger-cross-branch-v300r025"},
			unitTarget: "0", maxChildren: 247, maxPages: 248, maxObjects: 248, maxBytes: 65536,
			reject: []string{"cursor_loop", "duplicate_object", "duplicate_device_id", "count_mismatch", "sequence_changed"},
		},
		{
			file: "sdongle-disposition.json", profile: "huawei.sdongle", candidateID: "huawei.sdongle.v1", gatewayKind: "S-Dongle",
			missingDiscriminators: []string{"sanitized_product_code_and_revision_fixture", "live_child_inventory_target_fixture"},
			requiredTuple:         []string{"basic_mei_product_code_sdongle", "protocol_version_branch_match", "dongle_type_search_sequence_tuple", "structured_firmware_gate"},
			forbiddenIdentity:     []string{"search_status_only", "serial_number", "unit_100_readability_only"},
			firmwareGateCount:     5, negativeFixtures: []string{"sdongle-cross-branch-v200r023", "sdongle-v100-below-tcp-minimum"},
			unitTarget: "DOCUMENTARY_1_TO_247_TARGET_UNRESOLVED_FOR_TCP_GATEWAY", maxChildren: 120, maxPages: 121, maxObjects: 121, maxBytes: 32768,
			reject: []string{"cursor_loop", "duplicate_object", "duplicate_device_id", "search_in_progress", "sequence_changed", "count_mismatch", "unit_target_ambiguous"},
		},
		{
			file: "emma-disposition.json", profile: "huawei.emma", candidateID: "huawei.emma.v1", gatewayKind: "EMMA",
			missingDiscriminators: []string{"sanitized_model_software_self_entry_fixture", "negative_overlap_with_smartlogger", "live_mei_child_inventory_fixture"},
			requiredTuple:         []string{"offering_name", "model_emma_family", "software_version_branch_match", "mei_child_inventory_self_entry_device_id_zero_product_type_hems"},
			forbiddenIdentity:     []string{"serial_number_30015", "model_register_readability_only", "basic_mei_only", "smarthems_prefix_without_fixture"},
			firmwareGateCount:     2, negativeFixtures: []string{"emma-cross-branch-r026", "emma-r025-below-minimum"},
			unitTarget: "0", maxChildren: 247, maxPages: 248, maxObjects: 248, maxBytes: 65536,
			reject: []string{"cursor_loop", "duplicate_object", "duplicate_device_id", "count_mismatch", "presence_or_count_changed"},
		},
	}

	privateEndpoint := regexp.MustCompile(`(?m)\b(?:10|127|169\.254|172\.(?:1[6-9]|2\d|3[01])|192\.168)\.`)
	for _, expected := range expectations {
		t.Run(expected.gatewayKind, func(t *testing.T) {
			path := "profiles/vendor/huawei/" + expected.file
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if privateEndpoint.Match(data) || bytes.Contains(bytes.ToLower(data), []byte("credential")) {
				t.Fatal("disposition contains private endpoint or credential material")
			}
			record := decodeHuaweiObject(t, path)
			requireHuaweiKeys(t, record, "schema", "profile", "profile_version", "outcome", "evidence", "transport_dependency", "detector", "firmware_gate_count", "negative_fixture_ids", "child_enumeration", "decoder_keys", "catalog_registered", "support_claim", "reopen_requires")
			if record["schema"] != "helianthus-modbusreg-profile-disposition/v1" || record["profile"] != expected.profile ||
				record["profile_version"] != "1.0.0" || record["outcome"] != "NO_ADMISSIBLE_PROFILE" ||
				record["catalog_registered"] != false || record["support_claim"] != false {
				t.Fatalf("unexpected disposition: %#v", record)
			}
			if len(record["decoder_keys"].([]any)) != 0 {
				t.Fatal("decoder key registered for an inadmissible profile")
			}

			source := huaweiObject(t, record["evidence"])
			requireHuaweiKeys(t, source, "packet_id", "docs_merge_sha", "candidate_id", "gateway_kind", "documentary_status", "eligible", "missing_discriminators")
			if source["packet_id"] != "HUAWEI-GATEWAYS-V1" || source["docs_merge_sha"] != huaweiDocsMergeSHA ||
				source["candidate_id"] != expected.candidateID || source["gateway_kind"] != expected.gatewayKind ||
				source["documentary_status"] != "DOCUMENTARY_CANDIDATE" || source["eligible"] != false ||
				!reflect.DeepEqual(huaweiStringSlice(t, source["missing_discriminators"]), expected.missingDiscriminators) {
				t.Fatalf("unexpected evidence: %#v", source)
			}

			dependency := huaweiObject(t, record["transport_dependency"])
			requireHuaweiKeys(t, dependency, "module", "merge_sha", "version", "prerequisites")
			if dependency["module"] != "github.com/Project-Helianthus/helianthus-modbus" || dependency["merge_sha"] != huaweiModbusMerge || dependency["version"] != huaweiModbusPin ||
				!reflect.DeepEqual(huaweiStringSlice(t, dependency["prerequisites"]), []string{"modbus.unit-id-zero.v1", "modbus.mei-vendor-cursor.v1", "modbus.mei-object-wrap.v1"}) {
				t.Fatalf("unexpected transport dependency: %#v", dependency)
			}

			detector := huaweiObject(t, record["detector"])
			requireHuaweiKeys(t, detector, "registered", "executable", "required_tuple", "forbidden_identity", "candidate_ambiguity_outcome", "multiple_positive_outcome", "first_match_priority")
			if detector["registered"] != false || detector["executable"] != false || detector["candidate_ambiguity_outcome"] != "NO_ADMISSIBLE_PROFILE" || detector["multiple_positive_outcome"] != "INSUFFICIENT_EVIDENCE" || detector["first_match_priority"] != false ||
				!reflect.DeepEqual(huaweiStringSlice(t, detector["required_tuple"]), expected.requiredTuple) || !reflect.DeepEqual(huaweiStringSlice(t, detector["forbidden_identity"]), expected.forbiddenIdentity) {
				t.Fatalf("unexpected detector disposition: %#v", detector)
			}
			if huaweiInt(t, record["firmware_gate_count"]) != expected.firmwareGateCount ||
				!reflect.DeepEqual(huaweiStringSlice(t, record["negative_fixture_ids"]), expected.negativeFixtures) {
				t.Fatalf("unexpected gate evidence: %#v", record)
			}

			inventory := huaweiObject(t, record["child_enumeration"])
			requireHuaweiKeys(t, inventory, "operation", "executable", "unit_target", "read_device_id_code", "start_object_id", "max_children", "limits", "reject")
			limits := huaweiObject(t, inventory["limits"])
			requireHuaweiKeys(t, limits, "deadline_ms", "max_pages", "max_objects", "max_bytes")
			if inventory["operation"] != "FC2B_MEI_0E" || inventory["executable"] != false || inventory["unit_target"] != expected.unitTarget ||
				huaweiInt(t, inventory["read_device_id_code"]) != 3 || huaweiInt(t, inventory["start_object_id"]) != 135 || huaweiInt(t, inventory["max_children"]) != expected.maxChildren ||
				huaweiInt(t, limits["deadline_ms"]) != 15000 || huaweiInt(t, limits["max_pages"]) != expected.maxPages || huaweiInt(t, limits["max_objects"]) != expected.maxObjects || huaweiInt(t, limits["max_bytes"]) != expected.maxBytes ||
				!reflect.DeepEqual(huaweiStringSlice(t, inventory["reject"]), expected.reject) {
				t.Fatalf("unexpected child enumeration: %#v", inventory)
			}
			if !reflect.DeepEqual(huaweiStringSlice(t, record["reopen_requires"]), expected.missingDiscriminators) {
				t.Fatalf("reopen requirements differ from missing discriminators: %#v", record["reopen_requires"])
			}
		})
	}
}
