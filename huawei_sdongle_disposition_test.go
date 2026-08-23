package modbusreg

import (
	"reflect"
	"testing"
)

func TestHuaweiSDongleDispositionPinsExactCandidateAndNoSend(t *testing.T) {
	record := decodeHuaweiObject(t, "profiles/vendor/huawei/sdongle-disposition.json")
	requireHuaweiKeys(t, record,
		"schema", "profile", "profile_version", "outcome", "evidence", "admission",
		"protocol_version", "search_sequence", "capacity", "child_enumeration",
		"private_functions", "decoder_keys", "catalog_registered",
		"automatic_runtime_admission", "support_claim", "reopen_requires",
	)
	if record["schema"] != "helianthus-modbusreg-huawei-sdongle-disposition/v1" ||
		record["profile"] != "huawei.sdongle" || record["profile_version"] != "1.0.0" ||
		record["outcome"] != "NO_ADMISSIBLE_PROFILE" || record["catalog_registered"] != false ||
		record["automatic_runtime_admission"] != false || record["support_claim"] != false ||
		len(record["decoder_keys"].([]any)) != 0 {
		t.Fatalf("unexpected S-Dongle disposition: %#v", record)
	}

	missing := []string{
		"sanitized_basic_mei_product_model_fixture",
		"exact_protocol_version_encoding_fixture",
		"tcp_child_inventory_unit_target_fixture",
	}
	evidence := huaweiObject(t, record["evidence"])
	requireHuaweiKeys(t, evidence, "docs_merge_sha", "candidate_id", "source_license", "eligible", "missing_evidence")
	if evidence["docs_merge_sha"] != smartLoggerDocsMergeSHA || evidence["candidate_id"] != "huawei.sdongle.v1" ||
		evidence["source_license"] != "CC0-1.0" || evidence["eligible"] != false ||
		!reflect.DeepEqual(huaweiStringSlice(t, evidence["missing_evidence"]), missing) {
		t.Fatalf("unexpected S-Dongle evidence: %#v", evidence)
	}

	admission := huaweiObject(t, record["admission"])
	requireHuaweiKeys(t, admission, "unit_id", "registered", "executable", "models", "firmware_gates", "protocol_gate", "required_tuple", "forbidden_identity", "version_comparison", "multiple_positive_outcome", "first_match_priority")
	if huaweiInt(t, admission["unit_id"]) != 100 || admission["registered"] != false || admission["executable"] != false ||
		!reflect.DeepEqual(huaweiStringSlice(t, admission["models"]), []string{"SDongleA-05", "SDongleB-03", "SDongleB-06"}) ||
		!reflect.DeepEqual(huaweiStringSlice(t, admission["firmware_gates"]), []string{"V200R025C00SPC120"}) ||
		admission["protocol_gate"] != "D5.0" || admission["version_comparison"] != "EXACT_TUPLE_ONLY" ||
		admission["multiple_positive_outcome"] != "INSUFFICIENT_EVIDENCE" || admission["first_match_priority"] != false ||
		!reflect.DeepEqual(huaweiStringSlice(t, admission["required_tuple"]), []string{
			"basic_mei_product_identity", "fc03_30068_q2_protocol_version",
			"fc03_37410_q3_type_search_state_change_sequence",
			"fc03_37429_q1_capacity", "exact_model_firmware_protocol_tuple",
		}) ||
		!reflect.DeepEqual(huaweiStringSlice(t, admission["forbidden_identity"]), []string{
			"unit_100_readability_only", "search_status_only", "serial_number", "basic_mei_only",
		}) {
		t.Fatalf("unexpected S-Dongle admission: %#v", admission)
	}

	protocol := huaweiObject(t, record["protocol_version"])
	requireHuaweiKeys(t, protocol, "function", "offset", "quantity", "type")
	if protocol["function"] != "FC03" || huaweiInt(t, protocol["offset"]) != 30068 ||
		huaweiInt(t, protocol["quantity"]) != 2 || protocol["type"] != "U32" {
		t.Fatalf("unexpected S-Dongle protocol read: %#v", protocol)
	}

	search := huaweiObject(t, record["search_sequence"])
	requireHuaweiKeys(t, search, "function", "offset", "quantity", "fields", "search_must_be_complete", "change_sequence_stable")
	if search["function"] != "FC03" || huaweiInt(t, search["offset"]) != 37410 ||
		huaweiInt(t, search["quantity"]) != 3 ||
		!reflect.DeepEqual(huaweiStringSlice(t, search["fields"]), []string{"type", "search_state", "change_sequence"}) ||
		search["search_must_be_complete"] != true || search["change_sequence_stable"] != true {
		t.Fatalf("unexpected S-Dongle search read: %#v", search)
	}

	capacity := huaweiObject(t, record["capacity"])
	requireHuaweiKeys(t, capacity, "function", "offset", "quantity", "reconcile_child_count")
	if capacity["function"] != "FC03" || huaweiInt(t, capacity["offset"]) != 37429 ||
		huaweiInt(t, capacity["quantity"]) != 1 || capacity["reconcile_child_count"] != true {
		t.Fatalf("unexpected S-Dongle capacity read: %#v", capacity)
	}

	inventory := huaweiObject(t, record["child_enumeration"])
	requireHuaweiKeys(t, inventory, "function", "read_device_id_code", "start_object_id", "unit_target", "executable", "max_children", "limits", "reject")
	limits := huaweiObject(t, inventory["limits"])
	requireHuaweiKeys(t, limits, "deadline_ms", "max_pages", "max_objects", "max_bytes")
	if inventory["function"] != "FC2B_MEI_0E" || huaweiInt(t, inventory["read_device_id_code"]) != 3 ||
		huaweiInt(t, inventory["start_object_id"]) != 135 || inventory["unit_target"] != "UNQUALIFIED_TCP_UNIT_TARGET" ||
		inventory["executable"] != false || huaweiInt(t, inventory["max_children"]) != 120 ||
		huaweiInt(t, limits["deadline_ms"]) != 15000 || huaweiInt(t, limits["max_pages"]) != 121 ||
		huaweiInt(t, limits["max_objects"]) != 121 || huaweiInt(t, limits["max_bytes"]) != 32768 ||
		!reflect.DeepEqual(huaweiStringSlice(t, inventory["reject"]), []string{
			"cursor_loop", "duplicate_object", "duplicate_child_address", "count_mismatch",
			"search_in_progress", "change_sequence_mismatch", "unit_target_ambiguous", "limit_exhausted",
		}) {
		t.Fatalf("unexpected S-Dongle inventory: %#v", inventory)
	}

	privateFunctions := huaweiObject(t, record["private_functions"])
	requireHuaweiKeys(t, privateFunctions, "FC0x41", "FC0x17")
	if privateFunctions["FC0x41"] != "NO_SEND" || privateFunctions["FC0x17"] != "NO_SEND" ||
		!reflect.DeepEqual(huaweiStringSlice(t, record["reopen_requires"]), missing) {
		t.Fatalf("unexpected S-Dongle private-function/reopen policy: %#v", record)
	}
}
