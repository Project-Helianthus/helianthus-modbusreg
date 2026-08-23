package modbusreg

import (
	"reflect"
	"testing"
)

const smartLoggerDocsMergeSHA = "2abcb26a06ffa71149177de2cc2817a24d82081f"

func TestHuaweiSmartLoggerDispositionPinsExactCandidateAndNoSend(t *testing.T) {
	record := decodeHuaweiObject(t, "profiles/vendor/huawei/smartlogger-disposition.json")
	requireHuaweiKeys(t, record,
		"schema", "profile", "profile_version", "outcome", "evidence",
		"admission", "change_counter", "child_enumeration", "private_functions",
		"decoder_keys", "catalog_registered", "automatic_runtime_admission",
		"support_claim", "reopen_requires",
	)
	if record["schema"] != "helianthus-modbusreg-huawei-smartlogger-disposition/v1" ||
		record["profile"] != "huawei.smartlogger" || record["profile_version"] != "1.0.0" ||
		record["outcome"] != "NO_ADMISSIBLE_PROFILE" || record["catalog_registered"] != false ||
		record["automatic_runtime_admission"] != false || record["support_claim"] != false ||
		len(record["decoder_keys"].([]any)) != 0 {
		t.Fatalf("unexpected SmartLogger disposition: %#v", record)
	}

	missing := []string{
		"sanitized_extended_mei_self_entry_fixture",
		"exact_child_attribute_encoding_fixture",
		"pairwise_negative_overlap_with_emma_fixture",
	}
	evidence := huaweiObject(t, record["evidence"])
	requireHuaweiKeys(t, evidence, "docs_merge_sha", "candidate_id", "source_license", "eligible", "missing_evidence")
	if evidence["docs_merge_sha"] != smartLoggerDocsMergeSHA || evidence["candidate_id"] != "huawei.smartlogger.v1" ||
		evidence["source_license"] != "CC0-1.0" || evidence["eligible"] != false ||
		!reflect.DeepEqual(huaweiStringSlice(t, evidence["missing_evidence"]), missing) {
		t.Fatalf("unexpected SmartLogger evidence: %#v", evidence)
	}

	admission := huaweiObject(t, record["admission"])
	requireHuaweiKeys(t, admission, "unit_id", "registered", "executable", "required_tuple", "forbidden_identity", "firmware_gates", "version_comparison", "multiple_positive_outcome", "first_match_priority")
	if huaweiInt(t, admission["unit_id"]) != 0 || admission["registered"] != false || admission["executable"] != false ||
		admission["version_comparison"] != "EXACT_TUPLE_ONLY" || admission["multiple_positive_outcome"] != "INSUFFICIENT_EVIDENCE" ||
		admission["first_match_priority"] != false ||
		!reflect.DeepEqual(huaweiStringSlice(t, admission["required_tuple"]), []string{
			"fc03_65521_q1_u16_counter_stable",
			"fc2b_mei0e_code03_object87_inventory",
			"self_entry_model_smartlogger",
			"exact_firmware_tuple",
		}) ||
		!reflect.DeepEqual(huaweiStringSlice(t, admission["forbidden_identity"]), []string{
			"writable_device_name_65524", "esn_40713", "basic_mei_only", "optional_offering_string",
		}) ||
		!reflect.DeepEqual(huaweiStringSlice(t, admission["firmware_gates"]), []string{
			"V300R024C10SPC191", "V300R024C10SPC210",
		}) {
		t.Fatalf("unexpected SmartLogger admission: %#v", admission)
	}

	counter := huaweiObject(t, record["change_counter"])
	requireHuaweiKeys(t, counter, "function", "offset", "quantity", "type", "equal_before_after")
	if counter["function"] != "FC03" || huaweiInt(t, counter["offset"]) != 65521 ||
		huaweiInt(t, counter["quantity"]) != 1 || counter["type"] != "U16" || counter["equal_before_after"] != true {
		t.Fatalf("unexpected SmartLogger counter: %#v", counter)
	}

	inventory := huaweiObject(t, record["child_enumeration"])
	requireHuaweiKeys(t, inventory, "function", "read_device_id_code", "start_object_id", "executable", "max_children", "limits", "reject")
	limits := huaweiObject(t, inventory["limits"])
	requireHuaweiKeys(t, limits, "deadline_ms", "max_pages", "max_objects", "max_bytes")
	if inventory["function"] != "FC2B_MEI_0E" || huaweiInt(t, inventory["read_device_id_code"]) != 3 ||
		huaweiInt(t, inventory["start_object_id"]) != 135 || inventory["executable"] != false ||
		huaweiInt(t, inventory["max_children"]) != 247 || huaweiInt(t, limits["deadline_ms"]) != 15000 ||
		huaweiInt(t, limits["max_pages"]) != 248 || huaweiInt(t, limits["max_objects"]) != 248 ||
		huaweiInt(t, limits["max_bytes"]) != 65536 ||
		!reflect.DeepEqual(huaweiStringSlice(t, inventory["reject"]), []string{
			"cursor_loop", "duplicate_object", "duplicate_child_address", "count_mismatch",
			"malformed_attribute", "second_wrap", "change_counter_mismatch", "limit_exhausted",
		}) {
		t.Fatalf("unexpected SmartLogger inventory: %#v", inventory)
	}

	privateFunctions := huaweiObject(t, record["private_functions"])
	requireHuaweiKeys(t, privateFunctions, "FC0x41", "FC0x17")
	if privateFunctions["FC0x41"] != "NO_SEND" || privateFunctions["FC0x17"] != "NO_SEND" ||
		!reflect.DeepEqual(huaweiStringSlice(t, record["reopen_requires"]), missing) {
		t.Fatalf("unexpected SmartLogger private-function/reopen policy: %#v", record)
	}
}

func TestHuaweiPublicEvidenceIncludesCurrentCandidateContract(t *testing.T) {
	evidence := decodeHuaweiObject(t, "profiles/vendor/huawei/evidence.json")
	sources := evidence["public_sources"].([]any)
	found := false
	for _, value := range sources {
		source := huaweiObject(t, value)
		if source["id"] == "huawei-gateway-readonly-v1" &&
			source["locator"] == "https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/"+smartLoggerDocsMergeSHA+"/protocols/huawei/gateway-readonly-v1.md" &&
			source["license"] == "CC0-1.0" {
			found = true
		}
	}
	if !found {
		t.Fatal("Huawei public evidence does not include the current docs-modbus contract")
	}
}
