package modbusreg

import (
	"reflect"
	"testing"
)

const sdongleDocsMergeSHA = "2d44c22f27c4be0e1de460f63b8330c728a5af8f"

func TestHuaweiSDongleDispositionPinsExactCandidateAndNoSend(t *testing.T) {
	record := decodeHuaweiObject(t, "profiles/vendor/huawei/sdongle-disposition.json")
	requireHuaweiKeys(t, record,
		"schema", "profile", "profile_version", "outcome", "evidence", "admission",
		"protocol_version", "search_sequence", "capacity", "child_enumeration",
		"private_functions", "decoder_keys", "catalog_registered",
		"automatic_runtime_admission", "support_claim", "qualification_plan",
	)
	if record["schema"] != "helianthus-modbusreg-huawei-sdongle-disposition/v1" ||
		record["profile"] != "huawei.sdongle" || record["profile_version"] != "1.0.0" ||
		record["outcome"] != "PRE_LIVE_INSUFFICIENT_EVIDENCE" || record["catalog_registered"] != false ||
		record["automatic_runtime_admission"] != false || record["support_claim"] != false ||
		len(record["decoder_keys"].([]any)) != 0 {
		t.Fatalf("unexpected S-Dongle disposition: %#v", record)
	}

	missing := []string{
		"sanitized_basic_mei_product_model_fixture",
		"exact_protocol_version_encoding_fixture",
		"separately_qualified_child_unit_inventory_fixture",
	}
	evidence := huaweiObject(t, record["evidence"])
	requireHuaweiKeys(t, evidence, "docs_merge_sha", "candidate_id", "source_license", "eligible", "missing_evidence", "live_qualification")
	if evidence["docs_merge_sha"] != sdongleDocsMergeSHA || evidence["candidate_id"] != "huawei.sdongle.v1" ||
		evidence["source_license"] != "CC0-1.0" || evidence["eligible"] != false ||
		!reflect.DeepEqual(huaweiStringSlice(t, evidence["missing_evidence"]), missing) {
		t.Fatalf("unexpected S-Dongle evidence: %#v", evidence)
	}
	live := huaweiObject(t, evidence["live_qualification"])
	requireHuaweiKeys(t, live, "status", "identification_claim", "incompatibility_claim", "subsequent_modbus_requests_sent", "attempts", "retry_matrix")
	if live["status"] != "LIVE_STOPPED_PERSISTENT_NON_RESPONSE" ||
		live["identification_claim"] != false || live["incompatibility_claim"] != false ||
		huaweiInt(t, live["subsequent_modbus_requests_sent"]) != 0 {
		t.Fatalf("unexpected S-Dongle live qualification: %#v", live)
	}
	attempts := live["attempts"].([]any)
	if len(attempts) != 3 {
		t.Fatalf("unexpected S-Dongle live attempts: %#v", attempts)
	}
	for index, deadline := range []int{3000, 10000, 5000} {
		attempt := huaweiObject(t, attempts[index])
		requireHuaweiKeys(t, attempt, "operation", "unit_id", "deadline_ms", "outcome")
		if huaweiInt(t, attempt["unit_id"]) != 100 || huaweiInt(t, attempt["deadline_ms"]) != deadline || attempt["outcome"] != "TIMEOUT" ||
			(index < 2 && attempt["operation"] != "FC2B_MEI_0E_READ_DEVICE_ID_BASIC") ||
			(index == 2 && attempt["operation"] != "FC03_37411_Q1_DEVICE_SEARCH_STATUS") {
			t.Fatalf("unexpected S-Dongle live attempt: %#v", attempt)
		}
	}
	retryMatrix := huaweiObject(t, live["retry_matrix"])
	requireHuaweiKeys(t, retryMatrix, "unit_id", "minimum_idle_ms", "outcome", "attempts")
	if huaweiInt(t, retryMatrix["unit_id"]) != 100 || huaweiInt(t, retryMatrix["minimum_idle_ms"]) != 5000 ||
		retryMatrix["outcome"] != "ALL_TIMEOUT" {
		t.Fatalf("unexpected S-Dongle retry matrix: %#v", retryMatrix)
	}
	retryAttempts := retryMatrix["attempts"].([]any)
	wantRetryOperations := []string{
		"FC2B_MEI_0E_READ_DEVICE_ID_BASIC",
		"FC03_37411_Q1_DEVICE_SEARCH_STATUS",
		"FC2B_MEI_0E_READ_DEVICE_ID_BASIC",
		"FC03_37411_Q1_DEVICE_SEARCH_STATUS",
	}
	if len(retryAttempts) != len(wantRetryOperations) {
		t.Fatalf("unexpected S-Dongle retry attempts: %#v", retryAttempts)
	}
	for index, operation := range wantRetryOperations {
		attempt := huaweiObject(t, retryAttempts[index])
		requireHuaweiKeys(t, attempt, "operation", "unit_id", "deadline_ms", "outcome")
		if attempt["operation"] != operation || huaweiInt(t, attempt["unit_id"]) != 100 ||
			huaweiInt(t, attempt["deadline_ms"]) != 5000 || attempt["outcome"] != "TIMEOUT" {
			t.Fatalf("unexpected S-Dongle retry attempt: %#v", attempt)
		}
	}

	admission := huaweiObject(t, record["admission"])
	requireHuaweiKeys(t, admission, "unit_id", "registered", "executable", "default_denied", "models", "firmware_gates", "protocol_gate", "required_tuple", "forbidden_identity", "version_comparison", "multiple_positive_outcome", "first_match_priority")
	if huaweiInt(t, admission["unit_id"]) != 100 || admission["registered"] != false || admission["executable"] != false ||
		admission["default_denied"] != true ||
		!reflect.DeepEqual(huaweiStringSlice(t, admission["models"]), []string{"S-DongleA-05", "S-DongleB-03", "S-DongleB-06"}) ||
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
	firmwareGates := huaweiObject(t, admission["firmware_gates"])
	requireHuaweiKeys(t, firmwareGates, "offline_candidate", "observed_default_denied")
	if !reflect.DeepEqual(huaweiStringSlice(t, firmwareGates["offline_candidate"]), []string{"V200R025C00SPC120"}) {
		t.Fatalf("unexpected S-Dongle offline firmware gate: %#v", firmwareGates)
	}
	observedTuple := huaweiObject(t, firmwareGates["observed_default_denied"])
	requireHuaweiKeys(t, observedTuple, "model", "firmware", "minimum_gate", "admission")
	if observedTuple["model"] != "S-DongleA-05" || observedTuple["firmware"] != "V200R022C10SPC312" ||
		observedTuple["minimum_gate"] != "V200R022C10" || observedTuple["admission"] != "PRE_LIVE_INSUFFICIENT_EVIDENCE" {
		t.Fatalf("unexpected S-Dongle observed firmware tuple: %#v", observedTuple)
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
	requireHuaweiKeys(t, inventory, "function", "read_device_id_code", "start_object_id", "unit_target", "gateway_unit_100", "executable", "max_children", "limits", "reject")
	limits := huaweiObject(t, inventory["limits"])
	requireHuaweiKeys(t, limits, "deadline_ms", "max_pages", "max_objects", "max_bytes")
	if inventory["function"] != "FC2B_MEI_0E" || huaweiInt(t, inventory["read_device_id_code"]) != 3 ||
		huaweiInt(t, inventory["start_object_id"]) != 135 || inventory["unit_target"] != "SEPARATELY_QUALIFIED_CHILD_UNIT_1_TO_247" ||
		inventory["gateway_unit_100"] != "NO_SEND" ||
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
	if privateFunctions["FC0x41"] != "NO_SEND" || privateFunctions["FC0x17"] != "NO_SEND" {
		t.Fatalf("unexpected S-Dongle private-function policy: %#v", record)
	}

	plan := huaweiObject(t, record["qualification_plan"])
	requireHuaweiKeys(t, plan, "status", "read_only", "live_io_performed", "further_live_io", "operator_confirmation_required", "required_connection_context", "capture_sequence", "redact", "promotion_outcomes")
	if plan["status"] != "LIVE_STOPPED_MEI_TIMEOUT" || plan["read_only"] != true ||
		plan["live_io_performed"] != true || plan["further_live_io"] != "HARD_STOP" || plan["operator_confirmation_required"] != true ||
		!reflect.DeepEqual(huaweiStringSlice(t, plan["required_connection_context"]), []string{
			"endpoint", "port", "unit_id", "topology",
		}) ||
		!reflect.DeepEqual(huaweiStringSlice(t, plan["capture_sequence"]), []string{
			"basic_mei_identity",
			"fc03_37411_q1_device_search_status",
			"bounded_retry_matrix_basic_mei_fc03_37411_unit_100",
			"hard_stop_after_all_timeouts",
			"separately_qualified_child_unit_1_to_247_before_extended_mei",
			"pairwise_negative_overlap_smartlogger_emma",
			"sanitized_fixture_replay",
		}) ||
		!reflect.DeepEqual(huaweiStringSlice(t, plan["redact"]), []string{
			"serial_number", "esn", "registration_material", "credentials", "private_endpoint",
		}) ||
		!reflect.DeepEqual(huaweiStringSlice(t, plan["promotion_outcomes"]), []string{
			"PROFILE_ADMITTED", "NO_ADMISSIBLE_PROFILE",
		}) ||
		!reflect.DeepEqual(huaweiStringSlice(t, evidence["missing_evidence"]), missing) {
		t.Fatalf("unexpected S-Dongle qualification plan: %#v", plan)
	}
}
