package modbusreg

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func h15FixturePaths() []string {
	var paths []string
	for _, name := range h15FixtureNames() {
		paths = append(paths, filepath.ToSlash(filepath.Join(h03FixtureDirectory, name)))
	}
	return paths
}

func h15OutcomeLabel(outcome h03Outcome) string {
	switch outcome.Type {
	case "exception":
		return fmt.Sprintf("EXCEPTION_%d", outcome.Code)
	case "no_response":
		return "NO_RESPONSE"
	case "words":
		return "ANSWERED_WORDS"
	case "text", "text_with_residue":
		return "ANSWERED_TEXT"
	}
	return "UNKNOWN_OUTCOME"
}

// h15RowOutcome reads the outcome that a negative-overlap row claims out of the
// fixture that the row names, so that the disposition can never say more or less
// than the fixtures hold.
func h15RowOutcome(t *testing.T, row map[string]any) string {
	t.Helper()
	path, _ := row["fixture"].(string)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("row fixture %q: %v", path, err)
	}
	unit := huaweiInt(t, row["unit"])
	if function, ok := row["function"]; ok {
		if function != "FC2B" {
			t.Fatalf("unexpected function in row: %#v", row)
		}
		switch filepath.Base(path) {
		case "sdongle_gateway_basic_exception.json":
			var fixture h15ExceptionFixture
			if err := h03Decode(data, &fixture); err != nil || fixture.Unit != unit {
				t.Fatalf("exception fixture does not bear on unit %d: %v", unit, err)
			}
			return fmt.Sprintf("EXCEPTION_%d", fixture.Reply.ExceptionCode)
		case "sdongle_child_basic_reply.json":
			var fixture h03BasicFixture
			if err := h03Decode(data, &fixture); err != nil || !containsInt(fixture.AnsweredForUnits, unit) {
				t.Fatalf("basic reply fixture does not bear on unit %d: %v", unit, err)
			}
			return "ANSWERED_MEI_BASIC"
		case "sdongle_child_extended_inventory.json":
			var fixture h15InventoryFixture
			if err := h03Decode(data, &fixture); err != nil || fixture.Unit != unit || fixture.Read.ReadDeviceIDCode != 3 {
				t.Fatalf("inventory fixture does not bear on unit %d: %v", unit, err)
			}
			return "ANSWERED_MEI_EXTENDED"
		}
		t.Fatalf("no reader for fixture %s", path)
	}
	var set h03ProbeFixture
	if err := h03Decode(data, &set); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	offset := uint16(huaweiInt(t, row["offset"]))
	return h15OutcomeLabel(h15ProbeAt(t, set, unit, offset).Outcome)
}

func TestH15SDongleDispositionSupersedesHardStopForIdentityOnly(t *testing.T) {
	card := NewHuaweiSDongleObservedQualificationCard()
	record := decodeHuaweiObject(t, "profiles/vendor/huawei/sdongle-disposition.json")
	missing := card.MissingEvidence()

	// The profile outcome and every authority field stay where they were.
	if record["outcome"] != "PRE_LIVE_INSUFFICIENT_EVIDENCE" || record["catalog_registered"] != false || record["automatic_runtime_admission"] != false ||
		record["support_claim"] != false || record["write_authority"] != false || len(h03Array(t, record["decoder_keys"])) != 0 ||
		!reflect.DeepEqual(huaweiStringSlice(t, record["reopen_requires"]), missing) {
		t.Fatalf("unexpected S-Dongle profile status: %#v", record)
	}
	admission := huaweiObject(t, record["admission"])
	if admission["registered"] != false || admission["executable"] != false || admission["default_denied"] != true ||
		admission["multiple_positive_outcome"] != "INSUFFICIENT_EVIDENCE" || admission["first_match_priority"] != false {
		t.Fatalf("the documentary candidate changed: %#v", admission)
	}
	m86RequireSoftwareSelection(t, record, HuaweiSDongleCanonicalClass, 100)
	private := huaweiObject(t, record["private_functions"])
	enumeration := huaweiObject(t, record["child_enumeration"])
	if private["FC0x41"] != "NO_SEND" || private["FC0x17"] != "NO_SEND" || enumeration["gateway_unit_100"] != "NO_SEND" || enumeration["executable"] != false {
		t.Fatalf("a no-send rule opened: %#v %#v", private, enumeration)
	}

	// Evidence: one gap closes for the observed tuple, three stay open, the
	// closed gap is not in the open list, and every row points at a committed fixture.
	evidence := huaweiObject(t, record["evidence"])
	if !reflect.DeepEqual(huaweiStringSlice(t, evidence["missing_evidence"]), missing) || evidence["eligible"] != false {
		t.Fatalf("missing evidence: %#v", evidence["missing_evidence"])
	}
	closed := h03Array(t, evidence["closed_evidence"])
	if len(closed) != 1 {
		t.Fatalf("closed evidence: %#v", closed)
	}
	closedRow := huaweiObject(t, closed[0])
	requireHuaweiKeys(t, closedRow, "gap", "scope", "fixtures")
	if closedRow["gap"] != "separately_qualified_child_unit_inventory_fixture" || closedRow["scope"] != "observed_tuple" ||
		!reflect.DeepEqual(huaweiStringSlice(t, closedRow["fixtures"]), []string{h15FixturePaths()[1]}) || containsString(missing, closedRow["gap"].(string)) {
		t.Fatalf("closed evidence row: %#v", closedRow)
	}
	h03RequireFiles(t, huaweiStringSlice(t, closedRow["fixtures"]))
	partial := h03Array(t, evidence["partially_closed_evidence"])
	wantPartial := []struct{ gap, open string }{
		{"sanitized_basic_mei_product_model_fixture", "gateway_product_model_string"},
		{"exact_protocol_version_encoding_fixture", "encoding_of_the_protocol_words"},
		{"pairwise_negative_overlap_with_emma_fixture", "sdongle_probes_at_emma_endpoint"},
	}
	if len(partial) != len(wantPartial) {
		t.Fatalf("partially closed evidence: %#v", partial)
	}
	for index, want := range wantPartial {
		row := huaweiObject(t, partial[index])
		requireHuaweiKeys(t, row, "gap", "closed", "open", "fixtures")
		if row["gap"] != want.gap || row["open"] != want.open || !containsString(missing, want.gap) {
			t.Fatalf("partially closed row %d: %#v", index, row)
		}
		h03RequireFiles(t, huaweiStringSlice(t, row["fixtures"]))
	}
	if !reflect.DeepEqual(huaweiStringSlice(t, evidence["unknown_after_observation"]), []string{
		"gateway_product_model_string", "encoding_of_the_protocol_words", "child_units_other_than_unit_1", "dongle_with_more_than_one_inverter",
		"extended_inventory_at_gateway_unit_100", "smartlogger_counter_read_at_gateway_unit_100", "cause_of_the_first_request_timeout",
	}) {
		t.Fatalf("unknown after observation: %#v", evidence["unknown_after_observation"])
	}
	h03RequireDocsContract(t, evidence["docs_contract"], []string{
		"protocols/huawei/gateway-readonly-v1.md",
		"protocols/huawei/smartlogger-observed-inventory-v1.md",
		"protocols/huawei/sun2000-observed-identity-v1.md",
		"protocols/huawei/qualification-readiness-v1.md",
	})

	// The recorded stop stays on record and is superseded for the observed
	// identity only: the earlier attempts are untouched, and further I/O needs
	// the operator's approval at action time.
	live := huaweiObject(t, evidence["live_qualification"])
	plan := huaweiObject(t, record["qualification_plan"])
	observed := huaweiObject(t, record["observed_identity"])
	hardStop := huaweiObject(t, observed["hard_stop"])
	requireHuaweiKeys(t, hardStop, "recorded_status", "superseded_for", "non_response_reproduced", "superseded_only_by", "further_live_io")
	if live["status"] != "SUPERSEDED_FOR_OBSERVED_IDENTITY" || live["recorded_status"] != "LIVE_STOPPED_PERSISTENT_NON_RESPONSE" ||
		hardStop["recorded_status"] != live["recorded_status"] || hardStop["superseded_for"] != "IDENTITY_OF_OBSERVED_TUPLE_ONLY" ||
		hardStop["non_response_reproduced"] != false || hardStop["superseded_only_by"] != "MATCHED_OBSERVED_CARD_OF_THE_SAME_CLASS" ||
		hardStop["further_live_io"] != plan["further_live_io"] || plan["further_live_io"] != "OPERATOR_APPROVAL_AT_ACTION_TIME" ||
		plan["operator_confirmation_required"] != true || plan["read_only"] != true || plan["status"] != "HARD_STOP_SUPERSEDED_FOR_OBSERVED_IDENTITY" ||
		len(h03Array(t, live["attempts"])) != 3 || len(h03Array(t, huaweiObject(t, live["retry_matrix"])["attempts"])) != 4 {
		t.Fatalf("hard stop record: %#v %#v %#v", live, hardStop, plan)
	}

	requireHuaweiKeys(t, observed, "status", "scope", "qualification_card", "gate_registered", "default_denied", "hard_stop", "required_tuple",
		"software_selection", "exact_tuple", "tolerance", "retained_not_gating", "forbidden_identity", "outside_admitted_set_outcome", "multiple_positive_outcome",
		"first_match_priority", "timing", "negative_overlap", "fixtures", "profile_status")
	if observed["status"] != "OFFLINE_IDENTITY_ADMITTED" || observed["scope"] != "OBSERVED_TUPLE_ONLY" || observed["qualification_card"] != card.ID() ||
		observed["gate_registered"] != true || observed["default_denied"] != true || observed["outside_admitted_set_outcome"] != "INSUFFICIENT_EVIDENCE" ||
		observed["multiple_positive_outcome"] != "INSUFFICIENT_EVIDENCE" || observed["first_match_priority"] != false ||
		!reflect.DeepEqual(huaweiStringSlice(t, observed["fixtures"]), h15FixturePaths()) {
		t.Fatalf("unexpected observed identity: %#v", observed)
	}
	h03RequireFiles(t, h15FixturePaths())

	// The exact tuple in the record is the card's.
	tuple := huaweiObject(t, observed["exact_tuple"])
	requireHuaweiKeys(t, tuple, "gateway_unit", "gateway_basic_mei", "gateway_protocol_words", "gateway_protocol_comparison", "gateway_search", "child_unit", "child_model",
		"child_basic_mei", "selected_class", "gateway_model_variant")
	requireHuaweiKeys(t, huaweiObject(t, tuple["gateway_search"]), "type", "observed_type", "state", "sequence")
	basicMEI := huaweiObject(t, tuple["gateway_basic_mei"])
	search := huaweiObject(t, tuple["gateway_search"])
	childBasic := huaweiObject(t, tuple["child_basic_mei"])
	words := h03Array(t, tuple["gateway_protocol_words"])
	if huaweiInt(t, tuple["gateway_unit"]) != int(card.UnitID()) || huaweiInt(t, tuple["child_unit"]) != int(card.ChildUnit()) ||
		tuple["child_model"] != card.ChildModel() || tuple["selected_class"] != card.CandidateClass() || tuple["gateway_model_variant"] != "UNKNOWN" ||
		basicMEI["outcome"] != "EXCEPTION" || huaweiInt(t, basicMEI["code"]) != int(card.GatewayBasicException()) ||
		len(words) != 2 || huaweiInt(t, words[0]) != int(card.GatewayProtocolWords()[0]) || huaweiInt(t, words[1]) != int(card.GatewayProtocolWords()[1]) ||
		tuple["gateway_protocol_comparison"] != "LEADING_WORD_EQUAL_SECOND_WORD_AT_OR_ABOVE_NEVER_DECODED" ||
		search["type"] != "RECORDED_NOT_COMPARED" || huaweiInt(t, search["observed_type"]) != 4 || huaweiInt(t, search["state"]) != 0 || search["sequence"] != "EQUAL_BEFORE_AFTER" ||
		childBasic["vendor"] != card.ChildVendor() || childBasic["product"] != card.ChildFamily() || childBasic["version"] != "PRESENT" {
		t.Fatalf("exact tuple: %#v", tuple)
	}
	if !reflect.DeepEqual(huaweiStringSlice(t, observed["required_tuple"]), []string{
		"gateway_unit_100", "gateway_basic_mei_exception_1", "gateway_fc03_37411_q1_search_state_0",
		"gateway_fc03_30068_q2_protocol_leading_word_50_second_word_at_or_above_0",
		"gateway_fc03_37410_q3_state_0_sequence_equal_before_after_type_recorded", "gateway_fc03_37429_q1_capacity_covers_one_child",
		"gateway_fc03_30050_q15_software_admitted_by_catalogue_floors_when_read",
		"child_unit_1", "child_fc03_30000_q15_model_exact_residue_opaque", "child_basic_mei_vendor_and_product_family_exact",
		"child_extended_inventory_one_page_count_equals_entries_keys_1_to_6_at_least", "child_entry_model_equals_routed_model", "child_entry_unit_equals_unit_read",
	}) || !reflect.DeepEqual(huaweiStringSlice(t, observed["retained_not_gating"]), []string{
		"child_firmware_text_30035", "child_protocol_words_30068", "child_entry_attributes_2_3_4_6", "child_basic_mei_version_object",
		"child_basic_mei_objects_beyond_the_basic_category", "gateway_capacity_value", "model_block_residue", "gateway_search_type_word",
	}) || !reflect.DeepEqual(huaweiStringSlice(t, observed["forbidden_identity"]), []string{
		"serial_number", "esn", "register_readability", "unit_100_readability_only", "search_status_only", "basic_mei_only",
		"dongle_registers_answered_at_the_child", "child_listing", "mei_request_unit_identifier", "prefix_wildcard_case_folded_or_whitespace_normalized_match",
	}) {
		t.Fatalf("observed identity lists: %#v", observed)
	}
	m86RequireObservedSelection(t, observed["software_selection"], card.Evaluate(h15ValidInput(t)), map[string]any{
		"software_read": "NO", "observed_tuple_grade": "PROVEN",
	})
	tolerance := huaweiObject(t, observed["tolerance"])
	requireHuaweiKeys(t, tolerance, "model_block_residue", "declared_object_field", "next_object_id_without_more_follows", "entry_padding",
		"attribute_order", "undocumented_attribute_keys", "repeated_or_malformed_keys", "entry_count", "max_response_bytes", "observed_values")
	if huaweiInt(t, tolerance["max_response_bytes"]) != int(card.Limits().MaxResponseBytes) || tolerance["declared_object_field"] != "RETAINED_NOT_COMPARED" ||
		tolerance["next_object_id_without_more_follows"] != "RETAINED_NOT_COMPARED" || tolerance["entry_padding"] != "NOT_REQUIRED" ||
		tolerance["attribute_order"] != "NOT_VALIDATED" || tolerance["undocumented_attribute_keys"] != "RETAINED_OPAQUE" ||
		tolerance["repeated_or_malformed_keys"] != "REJECT_INVENTORY" || tolerance["entry_count"] != "FROM_THE_COUNT_OBJECT_ONE_ENTRY_OF_THE_CHILD_UNIT" ||
		tolerance["observed_values"] != "LOWER_BOUNDS" || tolerance["model_block_residue"] != "PRINTABLE_ASCII_AFTER_FIRST_NUL_RETAINED_NOT_COMPARED" {
		t.Fatalf("tolerance: %#v", tolerance)
	}
	timing := huaweiObject(t, observed["timing"])
	if timing["first_request_timeout"] != "RECORDED_NOT_REQUIRED" || timing["fixture"] != h15FixturePaths()[3] {
		t.Fatalf("timing: %#v", timing)
	}
	profile := huaweiObject(t, observed["profile_status"])
	requireHuaweiKeys(t, profile, "outcome", "monitoring", "blocked_by")
	if profile["outcome"] != record["outcome"] || profile["monitoring"] != "NOT_ADMITTED" || !reflect.DeepEqual(huaweiStringSlice(t, profile["blocked_by"]), missing) {
		t.Fatalf("profile status: %#v", profile)
	}
}

func TestH15SDongleNegativeOverlapRowsMatchTheirFixtures(t *testing.T) {
	record := decodeHuaweiObject(t, "profiles/vendor/huawei/sdongle-disposition.json")
	overlap := huaweiObject(t, huaweiObject(t, record["observed_identity"])["negative_overlap"])
	requireHuaweiKeys(t, overlap, "versus_smartlogger", "versus_emma", "versus_sunspec", "readability_alone_is_never_identity", "open_directions")
	if overlap["readability_alone_is_never_identity"] != true ||
		!reflect.DeepEqual(huaweiStringSlice(t, overlap["open_directions"]), []string{
			"sdongle_probes_at_emma_endpoint", "smartlogger_counter_read_at_gateway_unit_100", "growatt_probes",
		}) {
		t.Fatalf("overlap summary: %#v", overlap)
	}
	known := map[string]bool{}
	for _, path := range h15FixturePaths() {
		known[path] = true
	}
	known[filepath.ToSlash(filepath.Join(h03FixtureDirectory, "smartlogger_negative_overlap.json"))] = true
	counts := map[string]int{}
	for _, group := range []string{"versus_smartlogger", "versus_emma", "versus_sunspec"} {
		for _, value := range h03Array(t, overlap[group]) {
			row := huaweiObject(t, value)
			if !known[row["fixture"].(string)] {
				t.Fatalf("%s: row names an unknown fixture: %#v", group, row)
			}
			if got := h15RowOutcome(t, row); got != row["outcome"] {
				t.Fatalf("%s: row %v claims %v, fixture holds %s", group, row["probe"], row["outcome"], got)
			}
			counts[group]++
		}
	}
	// Readability is never identity: the dongle's own search and capacity reads
	// answer at the child, while the SmartLogger path, the unit 100 refusal and
	// the child's shape differ.
	if counts["versus_smartlogger"] != 8 || counts["versus_emma"] != 6 || counts["versus_sunspec"] != 1 {
		t.Fatalf("overlap rows: %v", counts)
	}
	rows := h03Array(t, overlap["versus_smartlogger"])
	var labels []string
	for _, value := range rows {
		row := huaweiObject(t, value)
		labels = append(labels, fmt.Sprintf("%s@%d=%s", row["probe"], huaweiInt(t, row["unit"]), row["outcome"]))
	}
	for _, want := range []string{
		"fc03_65521_q1_counter@1=EXCEPTION_2", "fc03_65521_q1_counter@0=NO_RESPONSE", "fc2b_mei_basic@100=EXCEPTION_1",
		"fc2b_mei_basic@1=ANSWERED_MEI_BASIC", "fc2b_mei_extended_inventory@1=ANSWERED_MEI_EXTENDED",
		"fc03_37410_q3_sdongle_search@100=EXCEPTION_4", "fc03_37410_q3_sdongle_search@1=ANSWERED_WORDS", "fc03_37429_q1_sdongle_capacity@1=ANSWERED_WORDS",
	} {
		if !containsString(labels, want) {
			t.Fatalf("SmartLogger overlap row %q is missing from %v", want, labels)
		}
	}
	// No row claims a direction that no observation covers.
	for _, group := range []string{"versus_smartlogger", "versus_emma", "versus_sunspec"} {
		for _, value := range h03Array(t, overlap[group]) {
			row := huaweiObject(t, value)
			if unit := huaweiInt(t, row["unit"]); unit == 100 && strings.Contains(row["probe"].(string), "65521") {
				t.Fatalf("the counter was never read at the gateway unit: %#v", row)
			}
		}
	}
}
