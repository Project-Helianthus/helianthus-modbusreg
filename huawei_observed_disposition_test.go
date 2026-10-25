package modbusreg

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var h03FixturePaths = []string{
	"testdata/huawei/identity/smartlogger_basic_reply.json",
	"testdata/huawei/identity/smartlogger_extended_inventory.json",
	"testdata/huawei/identity/smartlogger_negative_overlap.json",
	"testdata/huawei/identity/sun2000_routed_identity.json",
}

func h03RequireFiles(t *testing.T, paths []string) {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("referenced file is missing: %v", err)
		}
	}
}

var h03Digestlike = regexp.MustCompile(`[0-9a-fA-F]{40}`)

func h03RequireDocsContract(t *testing.T, value any, pages []string) {
	t.Helper()
	contract := huaweiObject(t, value)
	requireHuaweiKeys(t, contract, "repository", "issue", "pull_request", "pages")
	if contract["repository"] != "helianthus-docs-modbus" || huaweiInt(t, contract["issue"]) != 241 || huaweiInt(t, contract["pull_request"]) != 243 ||
		!reflect.DeepEqual(huaweiStringSlice(t, contract["pages"]), pages) {
		t.Fatalf("unexpected docs contract pointer: %#v", contract)
	}
	for _, page := range pages {
		if !strings.HasPrefix(page, "protocols/huawei/") || !strings.HasSuffix(page, ".md") || strings.Contains(page, "#") || h03Digestlike.MatchString(page) {
			t.Fatalf("docs page %q is not a plain repository-relative path", page)
		}
	}
}

func TestH03SmartLoggerDispositionSupersedesIdentityStatusAndKeepsProfileClosed(t *testing.T) {
	record := decodeHuaweiObject(t, "profiles/vendor/huawei/smartlogger-disposition.json")
	requireHuaweiKeys(t, record,
		"schema", "profile", "profile_version", "outcome", "evidence", "admission", "observed_identity",
		"change_counter", "child_enumeration", "private_functions", "decoder_keys", "catalog_registered",
		"automatic_runtime_admission", "support_claim", "write_authority", "production_provider", "reopen_requires",
	)
	if record["schema"] != "helianthus-modbusreg-huawei-smartlogger-disposition/v2" || record["profile"] != "huawei.smartlogger" ||
		record["profile_version"] != "1.0.0" || record["outcome"] != "NO_ADMISSIBLE_PROFILE" || record["catalog_registered"] != false ||
		record["automatic_runtime_admission"] != false || record["support_claim"] != false || record["write_authority"] != false ||
		len(h03Array(t, record["decoder_keys"])) != 0 {
		t.Fatalf("unexpected SmartLogger disposition: %#v", record)
	}

	evidence := huaweiObject(t, record["evidence"])
	requireHuaweiKeys(t, evidence, "docs_merge_sha", "candidate_id", "source_license", "eligible", "missing_evidence",
		"closed_evidence", "partially_closed_evidence", "unknown_after_observation", "docs_contract", "vendor_documentation")
	if evidence["docs_merge_sha"] != smartLoggerDocsMergeSHA || evidence["candidate_id"] != "huawei.smartlogger.v1" ||
		evidence["source_license"] != "CC0-1.0" || evidence["eligible"] != false {
		t.Fatalf("unexpected SmartLogger evidence: %#v", evidence)
	}
	// Only the gaps that the fixtures close leave the lists; the partly closed
	// gap stays, so the profile outcome cannot move.
	stillMissing := []string{"pairwise_negative_overlap_with_emma_fixture"}
	if !reflect.DeepEqual(huaweiStringSlice(t, evidence["missing_evidence"]), stillMissing) ||
		!reflect.DeepEqual(huaweiStringSlice(t, record["reopen_requires"]), stillMissing) {
		t.Fatalf("unexpected missing evidence: %#v / %#v", evidence["missing_evidence"], record["reopen_requires"])
	}
	closed := h03Array(t, evidence["closed_evidence"])
	wantClosed := []struct{ gap, scope string }{
		{"sanitized_extended_mei_self_entry_fixture", "observed_tuple"},
		{"exact_child_attribute_encoding_fixture", "observed_key_set_structure"},
	}
	if len(closed) != len(wantClosed) {
		t.Fatalf("closed evidence: %#v", closed)
	}
	for index, want := range wantClosed {
		row := huaweiObject(t, closed[index])
		requireHuaweiKeys(t, row, "gap", "scope", "fixtures")
		if row["gap"] != want.gap || row["scope"] != want.scope || !reflect.DeepEqual(huaweiStringSlice(t, row["fixtures"]), []string{h03FixturePaths[1]}) {
			t.Fatalf("closed evidence row %d: %#v", index, row)
		}
	}
	partial := h03Array(t, evidence["partially_closed_evidence"])
	if len(partial) != 1 {
		t.Fatalf("partially closed evidence: %#v", partial)
	}
	row := huaweiObject(t, partial[0])
	requireHuaweiKeys(t, row, "gap", "closed", "open", "fixtures")
	if row["gap"] != "pairwise_negative_overlap_with_emma_fixture" || row["closed"] != "emma_probes_at_smartlogger_endpoint" ||
		row["open"] != "smartlogger_probes_at_emma_endpoint" || !reflect.DeepEqual(huaweiStringSlice(t, row["fixtures"]), []string{h03FixturePaths[2]}) {
		t.Fatalf("partially closed row: %#v", row)
	}
	if !reflect.DeepEqual(huaweiStringSlice(t, evidence["unknown_after_observation"]), []string{
		"opaque_attribute_key_meanings_3_6_9_10", "change_counter_moves_when_listing_changes", "connected_identity_at_documentary_tuples",
	}) {
		t.Fatalf("unknown after observation: %#v", evidence["unknown_after_observation"])
	}
	h03RequireDocsContract(t, evidence["docs_contract"], []string{
		"protocols/huawei/gateway-readonly-v1.md",
		"protocols/huawei/smartlogger-observed-inventory-v1.md",
		"protocols/huawei/qualification-readiness-v1.md",
	})

	// The three exact tuples are independent entries; the documentary firmware
	// gates are unchanged and the observed tuple equals the card's constants.
	admission := huaweiObject(t, record["admission"])
	requireHuaweiKeys(t, admission, "unit_id", "registered", "executable", "required_tuple", "forbidden_identity", "firmware_gates",
		"exact_tuples", "tuple_independence", "version_comparison", "multiple_positive_outcome", "first_match_priority")
	if admission["registered"] != false || admission["executable"] != false || admission["version_comparison"] != "EXACT_TUPLE_ONLY" ||
		admission["tuple_independence"] != "NO_RANGE_PREFIX_ORDER_OR_INHERITANCE" || admission["multiple_positive_outcome"] != "INSUFFICIENT_EVIDENCE" ||
		admission["first_match_priority"] != false ||
		!reflect.DeepEqual(huaweiStringSlice(t, admission["firmware_gates"]), []string{"V300R024C10SPC191", "V300R024C10SPC210"}) {
		t.Fatalf("unexpected SmartLogger admission: %#v", admission)
	}
	card := NewHuaweiSmartLoggerObservedQualificationCard()
	wantTuples := []map[string]any{
		{"branch": "documentary", "self_entry_model": "SmartLogger", "firmware": "V300R024C10SPC191", "basis": "documentary_candidate", "observed": false},
		{"branch": "documentary", "self_entry_model": "SmartLogger", "firmware": "V300R024C10SPC210", "basis": "documentary_candidate", "observed": false},
		{"branch": "observed", "self_entry_model": card.Model(), "firmware": card.Firmware(), "self_entry_unit": card.SelfEntryUnit(),
			"basis": "sanitized_readonly_observation", "observed": true},
	}
	tuples := h03Array(t, admission["exact_tuples"])
	if len(tuples) != len(wantTuples) {
		t.Fatalf("exact tuples: %#v", tuples)
	}
	for index, want := range wantTuples {
		encoded, _ := json.Marshal(tuples[index])
		var got map[string]any
		if err := json.Unmarshal(encoded, &got); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("exact tuple %d=%#v want=%#v", index, got, want)
		}
	}

	observed := huaweiObject(t, record["observed_identity"])
	requireHuaweiKeys(t, observed, "status", "scope", "qualification_card", "gate_registered", "default_denied", "required_tuple",
		"optional_corroboration", "forbidden_identity", "outside_admitted_set_outcome", "multiple_positive_outcome", "first_match_priority",
		"inventory_tolerance", "fixtures", "profile_status")
	if observed["status"] != "OFFLINE_IDENTITY_ADMITTED" || observed["scope"] != "OBSERVED_TUPLE_ONLY" || observed["qualification_card"] != card.ID() ||
		observed["gate_registered"] != true || observed["default_denied"] != true || observed["outside_admitted_set_outcome"] != "INSUFFICIENT_EVIDENCE" ||
		observed["multiple_positive_outcome"] != "INSUFFICIENT_EVIDENCE" || observed["first_match_priority"] != false ||
		!reflect.DeepEqual(huaweiStringSlice(t, observed["fixtures"]), h03FixturePaths) {
		t.Fatalf("unexpected observed identity: %#v", observed)
	}
	if !reflect.DeepEqual(huaweiStringSlice(t, observed["required_tuple"]), []string{
		"gateway_unit_0", "fc03_65521_q1_u16_counter_equal_before_after", "complete_valid_extended_inventory",
		"self_entry_model_exact", "self_entry_firmware_exact", "self_entry_unit_equals_gateway_unit",
	}) || !reflect.DeepEqual(huaweiStringSlice(t, observed["forbidden_identity"]), []string{
		"serial_number", "esn", "writable_device_name_65524", "register_readability", "child_listing", "basic_mei_only",
		"product_string_without_firmware", "mei_request_unit_identifier", "prefix_wildcard_case_folded_or_whitespace_normalized_match",
	}) {
		t.Fatalf("observed identity tuple lists: %#v", observed)
	}
	corroboration := huaweiObject(t, observed["optional_corroboration"])
	requireHuaweiKeys(t, corroboration, "basic_mei")
	basic := huaweiObject(t, corroboration["basic_mei"])
	requireHuaweiKeys(t, basic, "vendor", "product", "version", "disagreement_outcome", "absence_outcome")
	if basic["vendor"] != card.Vendor() || basic["product"] != card.Model() || basic["version"] != "EQUALS_SELF_ENTRY_FIRMWARE" ||
		basic["disagreement_outcome"] != "INSUFFICIENT_EVIDENCE" || basic["absence_outcome"] != "NO_EFFECT" {
		t.Fatalf("unexpected basic corroboration: %#v", basic)
	}
	tolerance := huaweiObject(t, observed["inventory_tolerance"])
	requireHuaweiKeys(t, tolerance, "min_accepted_response_bytes", "max_response_bytes", "max_entry_bytes", "declared_object_field",
		"count_object_includes_self_entry", "opaque_attribute_keys", "attribute_order", "unknown_or_repeated_keys")
	if huaweiInt(t, tolerance["min_accepted_response_bytes"]) != 263 || huaweiInt(t, tolerance["max_response_bytes"]) != int(card.Limits().MaxResponseBytes) ||
		huaweiInt(t, tolerance["max_entry_bytes"]) != 254 || tolerance["declared_object_field"] != "RETAINED_NOT_COMPARED" ||
		tolerance["count_object_includes_self_entry"] != true || tolerance["attribute_order"] != "NOT_VALIDATED" ||
		tolerance["unknown_or_repeated_keys"] != "REJECT_INVENTORY" || !reflect.DeepEqual(huaweiStringSlice(t, tolerance["opaque_attribute_keys"]), []string{"9", "10"}) {
		t.Fatalf("unexpected inventory tolerance: %#v", tolerance)
	}
	profile := huaweiObject(t, observed["profile_status"])
	requireHuaweiKeys(t, profile, "outcome", "monitoring", "blocked_by")
	if profile["outcome"] != "NO_ADMISSIBLE_PROFILE" || profile["monitoring"] != "NOT_ADMITTED" ||
		!reflect.DeepEqual(huaweiStringSlice(t, profile["blocked_by"]), stillMissing) {
		t.Fatalf("unexpected profile status: %#v", profile)
	}
	h03RequireFiles(t, h03FixturePaths)

	privateFunctions := huaweiObject(t, record["private_functions"])
	if privateFunctions["FC0x41"] != "NO_SEND" || privateFunctions["FC0x17"] != "NO_SEND" {
		t.Fatalf("private functions opened: %#v", privateFunctions)
	}
}

func TestH03PointerRowsLeaveOtherDispositionsClosed(t *testing.T) {
	type expectation struct {
		file, outcome, state string
		pages                []string
		fixtures             []string
		missing              string
		missingList          []string
	}
	observation := "protocols/huawei/smartlogger-observed-inventory-v1.md"
	identity := "protocols/huawei/sun2000-observed-identity-v1.md"
	tests := []expectation{
		// The SUN2000 monitoring profiles (#348) admit two exact pairs; the
		// pointer row itself still changes nothing, and what stays missing is
		// the pinned edition of the MAP0 series.
		{"sun2000-disposition.json", "OFFLINE_PROFILE_ADMITTED", "OBSERVED_IDENTITY_BEHIND_GATEWAY", []string{identity, observation}, []string{h03FixturePaths[1], h03FixturePaths[3]},
			"missing_evidence", []string{"pinned_vendor_document_identity"}},
		{"luna2000-disposition.json", "NO_ADMISSIBLE_PROFILE", "NOT_OBSERVED", []string{identity}, []string{},
			"missing_evidence", []string{"pinned_vendor_document_identity", "companion_docs_modbus_protocol_page", "clean_room_sanitized_identity_fixture", "clean_room_sanitized_monitoring_fixture", "negative_overlap_with_smartlogger_emma_sdongle"}},
	}
	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			record := decodeHuaweiObject(t, "profiles/vendor/huawei/"+test.file)
			pointer := huaweiObject(t, record["observation_pointer"])
			requireHuaweiKeys(t, pointer, "docs_contract", "observation_state", "fixtures", "changes_outcome", "changes_missing_evidence")
			h03RequireDocsContract(t, pointer["docs_contract"], test.pages)
			if pointer["observation_state"] != test.state || pointer["changes_outcome"] != false || pointer["changes_missing_evidence"] != false ||
				!reflect.DeepEqual(huaweiStringSlice(t, pointer["fixtures"]), test.fixtures) {
				t.Fatalf("unexpected pointer row: %#v", pointer)
			}
			h03RequireFiles(t, test.fixtures)

			// The pointer row records evidence only: outcome, lists and authority are untouched.
			evidence := huaweiObject(t, record["evidence"])
			if record["outcome"] != test.outcome || !reflect.DeepEqual(huaweiStringSlice(t, evidence[test.missing]), test.missingList) ||
				record["catalog_registered"] != false || record["automatic_runtime_admission"] != false || record["support_claim"] != false ||
				(test.file != "sun2000-disposition.json" && len(h03Array(t, record["decoder_keys"])) != 0) {
				t.Fatalf("pointer row changed the disposition: %#v", record)
			}
			if reopen, ok := record["reopen_requires"]; ok && !reflect.DeepEqual(huaweiStringSlice(t, reopen), test.missingList) {
				t.Fatalf("reopen_requires changed: %#v", reopen)
			}
		})
	}

	sun2000 := decodeHuaweiObject(t, "profiles/vendor/huawei/sun2000-disposition.json")
	families := huaweiObject(t, sun2000["model_families"])
	if !reflect.DeepEqual(huaweiStringSlice(t, families["covered"]), []string{"SUN2000 M3 series", "SUN2000 M1 series"}) || families["unsupported"] != "ALL_OTHER_FAMILIES" {
		t.Fatalf("SUN2000 coverage changed: %#v", families)
	}
	luna := decodeHuaweiObject(t, "profiles/vendor/huawei/luna2000-disposition.json")
	if len(h03Array(t, huaweiObject(t, luna["observation_pointer"])["fixtures"])) != 0 {
		t.Fatal("LUNA2000 must point at no fixture: no LUNA2000 identity was observed")
	}
	// The S-Dongle disposition moved to schema v2 and the EMMA disposition to
	// schema v3; each carries its own observed identity instead of a pointer row,
	// and the S-Dongle and EMMA tests pin them.
	for _, name := range []string{"sdongle-disposition.json", "emma-disposition.json"} {
		if _, ok := decodeHuaweiObject(t, "profiles/vendor/huawei/"+name)["observation_pointer"]; ok {
			t.Fatalf("the %s still carries a pointer row next to its observed identity", name)
		}
	}
}

func TestH03PublicEvidenceListsObservedContractPagesWithoutPinnedLinks(t *testing.T) {
	evidence := decodeHuaweiObject(t, "profiles/vendor/huawei/evidence.json")
	const base = "https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/"
	want := map[string]string{
		"huawei-smartlogger-observed-inventory-v1": base + "smartlogger-observed-inventory-v1.md",
		"huawei-sun2000-observed-identity-v1":      base + "sun2000-observed-identity-v1.md",
		"huawei-gateway-power-meter-v1":            base + "gateway-power-meter-v1.md",
	}
	found := map[string]bool{}
	for _, value := range h03Array(t, evidence["public_sources"]) {
		source := huaweiObject(t, value)
		id, _ := source["id"].(string)
		if locator, ok := want[id]; ok {
			if source["locator"] != locator || source["license"] != "CC0-1.0" {
				t.Fatalf("source %s: %#v", id, source)
			}
			found[id] = true
		}
	}
	if len(found) != len(want) {
		t.Fatalf("observed contract sources found: %v", found)
	}
	applicability := strings.Join(huaweiStringSlice(t, evidence["applicability"]), "\n")
	for _, phrase := range []string{"Smart Logger", "SUN2000-50KTL-M3", "default denied"} {
		if !strings.Contains(applicability, phrase) {
			t.Fatalf("applicability lacks %q", phrase)
		}
	}
	for _, line := range huaweiStringSlice(t, evidence["applicability"]) {
		if strings.Contains(line, "NO_ADMISSIBLE_PROFILE disposition only; no public source") {
			t.Fatalf("applicability still states that SUN2000 has no observed identity: %q", line)
		}
	}
}

func TestH03ReadinessExpectedResultListsObservedCardAndKeepsTheOthers(t *testing.T) {
	data, err := os.ReadFile("testdata/huawei/qualification/readiness_expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected struct {
		Classes []map[string]any `json:"classes"`
	}
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	var ids []string
	statuses := map[string]string{}
	for _, class := range expected.Classes {
		id := class["card_id"].(string)
		ids = append(ids, id)
		statuses[id] = class["status"].(string)
		if class["hardware_test_ready"] != false || class["live_qualified"] != false || class["automatic_request_count"].(float64) != 0 || class["native_fact_count"].(float64) != 0 {
			t.Fatalf("class row carries authority: %#v", class)
		}
	}
	wantIDs := []string{
		"huawei.qualification.smartlogger.readonly@1.0.0",
		"huawei.qualification.smartlogger.observed-readonly@1.0.0",
		"huawei.qualification.emma.readonly@1.0.0",
		"huawei.qualification.sdongle.evidence-blocked@1.0.0",
		"huawei.qualification.sdongle.observed-readonly@1.0.0",
		"huawei.qualification.emma.observed-readonly@1.0.0",
	}
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Fatalf("cards=%v want=%v", ids, wantIDs)
	}
	if statuses[wantIDs[1]] != "QUALIFICATION_TEST_READY" || statuses[wantIDs[3]] != "QUALIFICATION_EVIDENCE_BLOCKED" || statuses[wantIDs[2]] != "QUALIFICATION_EVIDENCE_BLOCKED" ||
		statuses[wantIDs[4]] != "QUALIFICATION_TEST_READY" || statuses[wantIDs[5]] != "QUALIFICATION_TEST_READY" {
		t.Fatalf("statuses=%v", statuses)
	}
	if !reflect.DeepEqual(expected.Classes[1]["missing_evidence"], []any{"pairwise_negative_overlap_with_emma_fixture"}) {
		t.Fatalf("observed card missing evidence: %#v", expected.Classes[1]["missing_evidence"])
	}
}

func h03Array(t *testing.T, value any) []any {
	t.Helper()
	array, ok := value.([]any)
	if !ok {
		t.Fatalf("not an array: %#v", value)
	}
	return array
}
