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

func TestHuaweiV1FamilyDispositionsRetainFailClosedContracts(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		schema           string
		profile          string
		outcome          string
		docsMergeSHA     string
		admission        func(*testing.T, map[string]any)
		childEnumeration func(*testing.T, map[string]any)
		remaining        func(*testing.T, map[string]any)
	}{
		{
			name:         "SmartLogger",
			path:         "profiles/vendor/huawei/smartlogger-disposition.json",
			schema:       "helianthus-modbusreg-huawei-smartlogger-disposition/v1",
			profile:      "huawei.smartlogger",
			outcome:      "NO_ADMISSIBLE_PROFILE",
			docsMergeSHA: smartLoggerDocsMergeSHA,
			admission: func(t *testing.T, admission map[string]any) {
				t.Helper()
				requireHuaweiKeys(t, admission, "unit_id", "registered", "executable", "required_tuple", "forbidden_identity", "firmware_gates", "version_comparison", "multiple_positive_outcome", "first_match_priority")
				if huaweiInt(t, admission["unit_id"]) != 0 || admission["registered"] != false || admission["executable"] != false ||
					admission["version_comparison"] != "EXACT_TUPLE_ONLY" || admission["multiple_positive_outcome"] != "INSUFFICIENT_EVIDENCE" || admission["first_match_priority"] != false ||
					!reflect.DeepEqual(huaweiStringSlice(t, admission["required_tuple"]), []string{"fc03_65521_q1_u16_counter_stable", "fc2b_mei0e_code03_object87_inventory", "self_entry_model_smartlogger", "exact_firmware_tuple"}) ||
					!reflect.DeepEqual(huaweiStringSlice(t, admission["forbidden_identity"]), []string{"writable_device_name_65524", "esn_40713", "basic_mei_only", "optional_offering_string"}) ||
					!reflect.DeepEqual(huaweiStringSlice(t, admission["firmware_gates"]), []string{"V300R024C10SPC191", "V300R024C10SPC210"}) {
					t.Fatalf("unexpected SmartLogger admission: %#v", admission)
				}
			},
			childEnumeration: func(t *testing.T, inventory map[string]any) {
				t.Helper()
				requireHuaweiKeys(t, inventory, "function", "read_device_id_code", "start_object_id", "executable", "max_children", "limits", "reject")
				limits := huaweiObject(t, inventory["limits"])
				requireHuaweiKeys(t, limits, "deadline_ms", "max_pages", "max_objects", "max_bytes")
				if inventory["function"] != "FC2B_MEI_0E" || huaweiInt(t, inventory["read_device_id_code"]) != 3 || huaweiInt(t, inventory["start_object_id"]) != 135 ||
					inventory["executable"] != false || huaweiInt(t, inventory["max_children"]) != 247 || huaweiInt(t, limits["deadline_ms"]) != 15000 ||
					huaweiInt(t, limits["max_pages"]) != 248 || huaweiInt(t, limits["max_objects"]) != 248 || huaweiInt(t, limits["max_bytes"]) != 65536 ||
					!reflect.DeepEqual(huaweiStringSlice(t, inventory["reject"]), []string{"cursor_loop", "duplicate_object", "duplicate_child_address", "count_mismatch", "malformed_attribute", "second_wrap", "change_counter_mismatch", "limit_exhausted"}) {
					t.Fatalf("unexpected SmartLogger child enumeration: %#v", inventory)
				}
			},
			remaining: func(t *testing.T, record map[string]any) {
				t.Helper()
				requireHuaweiKeys(t, record, "schema", "profile", "profile_version", "outcome", "evidence", "admission", "change_counter", "child_enumeration", "private_functions", "decoder_keys", "catalog_registered", "automatic_runtime_admission", "support_claim", "reopen_requires")
				evidence := huaweiObject(t, record["evidence"])
				requireHuaweiKeys(t, evidence, "docs_merge_sha", "candidate_id", "source_license", "eligible", "missing_evidence")
				if evidence["candidate_id"] != "huawei.smartlogger.v1" || evidence["source_license"] != "CC0-1.0" ||
					!reflect.DeepEqual(huaweiStringSlice(t, evidence["missing_evidence"]), []string{"sanitized_extended_mei_self_entry_fixture", "exact_child_attribute_encoding_fixture", "pairwise_negative_overlap_with_emma_fixture"}) {
					t.Fatalf("unexpected SmartLogger evidence: %#v", evidence)
				}
				counter := huaweiObject(t, record["change_counter"])
				requireHuaweiKeys(t, counter, "function", "offset", "quantity", "type", "equal_before_after")
				if counter["function"] != "FC03" || huaweiInt(t, counter["offset"]) != 65521 || huaweiInt(t, counter["quantity"]) != 1 || counter["type"] != "U16" || counter["equal_before_after"] != true ||
					!reflect.DeepEqual(huaweiStringSlice(t, record["reopen_requires"]), []string{"sanitized_extended_mei_self_entry_fixture", "exact_child_attribute_encoding_fixture", "pairwise_negative_overlap_with_emma_fixture"}) {
					t.Fatalf("unexpected SmartLogger counter or reopen contract: %#v", record)
				}
			},
		},
		{
			name:         "S-Dongle",
			path:         "profiles/vendor/huawei/sdongle-disposition.json",
			schema:       "helianthus-modbusreg-huawei-sdongle-disposition/v1",
			profile:      "huawei.sdongle",
			outcome:      "PRE_LIVE_INSUFFICIENT_EVIDENCE",
			docsMergeSHA: sdongleDocsMergeSHA,
			admission: func(t *testing.T, admission map[string]any) {
				t.Helper()
				requireHuaweiKeys(t, admission, "unit_id", "registered", "executable", "default_denied", "models", "firmware_gates", "protocol_gate", "required_tuple", "forbidden_identity", "version_comparison", "multiple_positive_outcome", "first_match_priority")
				if huaweiInt(t, admission["unit_id"]) != 100 || admission["registered"] != false || admission["executable"] != false || admission["default_denied"] != true ||
					admission["protocol_gate"] != "D5.0" || admission["version_comparison"] != "EXACT_TUPLE_ONLY" || admission["multiple_positive_outcome"] != "INSUFFICIENT_EVIDENCE" || admission["first_match_priority"] != false ||
					!reflect.DeepEqual(huaweiStringSlice(t, admission["models"]), []string{"S-DongleA-05", "S-DongleB-03", "S-DongleB-06"}) ||
					!reflect.DeepEqual(huaweiStringSlice(t, admission["required_tuple"]), []string{"basic_mei_product_identity", "fc03_30068_q2_protocol_version", "fc03_37410_q3_type_search_state_change_sequence", "fc03_37429_q1_capacity", "exact_model_firmware_protocol_tuple"}) ||
					!reflect.DeepEqual(huaweiStringSlice(t, admission["forbidden_identity"]), []string{"unit_100_readability_only", "search_status_only", "serial_number", "basic_mei_only"}) {
					t.Fatalf("unexpected S-Dongle admission: %#v", admission)
				}
			},
			childEnumeration: func(t *testing.T, inventory map[string]any) {
				t.Helper()
				requireHuaweiKeys(t, inventory, "function", "read_device_id_code", "start_object_id", "unit_target", "gateway_unit_100", "executable", "max_children", "limits", "reject")
				limits := huaweiObject(t, inventory["limits"])
				requireHuaweiKeys(t, limits, "deadline_ms", "max_pages", "max_objects", "max_bytes")
				if inventory["function"] != "FC2B_MEI_0E" || huaweiInt(t, inventory["read_device_id_code"]) != 3 || huaweiInt(t, inventory["start_object_id"]) != 135 ||
					inventory["unit_target"] != "SEPARATELY_QUALIFIED_CHILD_UNIT_1_TO_247" || inventory["gateway_unit_100"] != "NO_SEND" || inventory["executable"] != false ||
					huaweiInt(t, inventory["max_children"]) != 120 || huaweiInt(t, limits["deadline_ms"]) != 15000 || huaweiInt(t, limits["max_pages"]) != 121 ||
					huaweiInt(t, limits["max_objects"]) != 121 || huaweiInt(t, limits["max_bytes"]) != 32768 ||
					!reflect.DeepEqual(huaweiStringSlice(t, inventory["reject"]), []string{"cursor_loop", "duplicate_object", "duplicate_child_address", "count_mismatch", "search_in_progress", "change_sequence_mismatch", "unit_target_ambiguous", "limit_exhausted"}) {
					t.Fatalf("unexpected S-Dongle child enumeration: %#v", inventory)
				}
			},
			remaining: func(t *testing.T, record map[string]any) {
				t.Helper()
				requireHuaweiKeys(t, record, "schema", "profile", "profile_version", "outcome", "evidence", "admission", "protocol_version", "search_sequence", "capacity", "child_enumeration", "private_functions", "decoder_keys", "catalog_registered", "automatic_runtime_admission", "support_claim", "qualification_plan")
				evidence := huaweiObject(t, record["evidence"])
				requireHuaweiKeys(t, evidence, "docs_merge_sha", "candidate_id", "source_license", "eligible", "missing_evidence", "live_qualification")
				live := huaweiObject(t, evidence["live_qualification"])
				requireHuaweiKeys(t, live, "status", "identification_claim", "incompatibility_claim", "subsequent_modbus_requests_sent", "attempts", "retry_matrix")
				if evidence["candidate_id"] != "huawei.sdongle.v1" || evidence["source_license"] != "CC0-1.0" || live["status"] != "LIVE_STOPPED_PERSISTENT_NON_RESPONSE" ||
					live["identification_claim"] != false || live["incompatibility_claim"] != false || huaweiInt(t, live["subsequent_modbus_requests_sent"]) != 0 {
					t.Fatalf("unexpected S-Dongle evidence: %#v", evidence)
				}
				protocol := huaweiObject(t, record["protocol_version"])
				search := huaweiObject(t, record["search_sequence"])
				capacity := huaweiObject(t, record["capacity"])
				requireHuaweiKeys(t, protocol, "function", "offset", "quantity", "type")
				requireHuaweiKeys(t, search, "function", "offset", "quantity", "fields", "search_must_be_complete", "change_sequence_stable")
				requireHuaweiKeys(t, capacity, "function", "offset", "quantity", "reconcile_child_count")
				if protocol["function"] != "FC03" || huaweiInt(t, protocol["offset"]) != 30068 || huaweiInt(t, protocol["quantity"]) != 2 || protocol["type"] != "U32" ||
					search["function"] != "FC03" || huaweiInt(t, search["offset"]) != 37410 || huaweiInt(t, search["quantity"]) != 3 || search["search_must_be_complete"] != true || search["change_sequence_stable"] != true ||
					!reflect.DeepEqual(huaweiStringSlice(t, search["fields"]), []string{"type", "search_state", "change_sequence"}) ||
					capacity["function"] != "FC03" || huaweiInt(t, capacity["offset"]) != 37429 || huaweiInt(t, capacity["quantity"]) != 1 || capacity["reconcile_child_count"] != true {
					t.Fatalf("unexpected S-Dongle protocol contract: %#v", record)
				}
				plan := huaweiObject(t, record["qualification_plan"])
				requireHuaweiKeys(t, plan, "status", "read_only", "live_io_performed", "further_live_io", "operator_confirmation_required", "required_connection_context", "capture_sequence", "redact", "promotion_outcomes")
				if plan["status"] != "LIVE_STOPPED_PERSISTENT_NON_RESPONSE" || plan["read_only"] != true || plan["live_io_performed"] != true ||
					plan["further_live_io"] != "HARD_STOP" || plan["operator_confirmation_required"] != true ||
					!reflect.DeepEqual(huaweiStringSlice(t, plan["promotion_outcomes"]), []string{"PROFILE_ADMITTED", "NO_ADMISSIBLE_PROFILE"}) {
					t.Fatalf("unexpected S-Dongle qualification contract: %#v", plan)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := decodeHuaweiObject(t, test.path)
			if record["schema"] != test.schema || record["profile"] != test.profile || record["profile_version"] != "1.0.0" || record["outcome"] != test.outcome ||
				record["catalog_registered"] != false || record["automatic_runtime_admission"] != false || record["support_claim"] != false || len(record["decoder_keys"].([]any)) != 0 {
				t.Fatalf("unexpected v1 fail-closed disposition: %#v", record)
			}
			evidence := huaweiObject(t, record["evidence"])
			if evidence["docs_merge_sha"] != test.docsMergeSHA || evidence["eligible"] != false {
				t.Fatalf("unexpected evidence: %#v", evidence)
			}
			test.admission(t, huaweiObject(t, record["admission"]))
			test.childEnumeration(t, huaweiObject(t, record["child_enumeration"]))
			test.remaining(t, record)
			privateFunctions := huaweiObject(t, record["private_functions"])
			if privateFunctions["FC0x41"] != "NO_SEND" || privateFunctions["FC0x17"] != "NO_SEND" {
				t.Fatalf("unexpected private-function policy: %#v", privateFunctions)
			}
		})
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
		!reflect.DeepEqual(huaweiStringSlice(t, detector["required_tuple"]), []string{"bounded_contextual_offering", "exact_model_alias", "same_branch_firmware_floor"}) ||
		!reflect.DeepEqual(huaweiStringSlice(t, detector["optional_enrichment"]), []string{"basic_mei", "extended_mei"}) ||
		!reflect.DeepEqual(huaweiStringSlice(t, detector["forbidden_identity"]), []string{"serial_number_30015", "basic_mei_only", "extended_mei_only", "prefix_or_wildcard_model_match"}) {
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
		{"SmartLogger", "profiles/vendor/huawei/smartlogger-disposition.json", smartLoggerDocsMergeSHA},
		{"S-Dongle", "profiles/vendor/huawei/sdongle-disposition.json", sdongleDocsMergeSHA},
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
