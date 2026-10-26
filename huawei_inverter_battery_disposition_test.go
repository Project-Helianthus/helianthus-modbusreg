package modbusreg

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type huaweiInverterBatteryCase struct {
	name       string
	path       string
	profile    string
	version    string
	outcome    string
	class      string
	optional   bool
	rowsWanted []string
}

func huaweiInverterBatteryCases() []huaweiInverterBatteryCase {
	return []huaweiInverterBatteryCase{
		{
			name: "SUN2000", path: "profiles/vendor/huawei/sun2000-disposition.json",
			profile: "huawei.sun2000", version: "2.0.0", outcome: "OFFLINE_PROFILE_ADMITTED", class: "SUN2000", optional: false,
			rowsWanted: []string{
				"no_identity_evidence",
				"gateway_identity_not_matched",
				"gateway_inventory_child_model_only",
				"unit_not_routed",
				"serial_or_esn_only",
				"register_readability_only",
				"listing_model_conflict",
				"identity_words_invalid",
				"model_prefix_or_wildcard",
				"model_behind_another_gateway_path",
				"family_without_catalogue_row",
				"catalogue_row_without_profile_on_path",
				"catalogue_identifier_mismatch",
				"catalogue_software_malformed_or_absent",
				"catalogue_rows_not_told_apart",
				"multiple_positive_candidates",
				"catalogue_m3_row_behind_smartlogger",
				"catalogue_m1_row_behind_sdongle",
				"token_without_identifier_in_several_rows",
				"token_without_identifier_in_one_row",
			},
		},
		{
			name: "LUNA2000", path: "profiles/vendor/huawei/luna2000-disposition.json",
			profile: "huawei.luna2000", version: "1.1.0", outcome: "NO_ADMISSIBLE_PROFILE", class: "LUNA2000", optional: true,
			rowsWanted: []string{
				"no_identity_evidence",
				"gateway_inventory_child_model_only",
				"parent_inverter_unidentified",
				"parent_identified_battery_layout_unknown",
				"model_without_firmware",
				"serial_or_esn_only",
				"register_readability_only",
				"battery_group_absent",
				"battery_group_exception",
				"multiple_positive_candidates",
				"exact_identity_with_pinned_register_map",
			},
		},
	}
}

func huaweiRequireMonitoringContract(t *testing.T, value any) {
	t.Helper()
	contract := huaweiObject(t, value)
	requireHuaweiKeys(t, contract, "repository", "issue", "pull_request", "pages", "companion_required", "companion_state", "merge_companion_first")
	if contract["repository"] != "helianthus-docs-modbus" || huaweiInt(t, contract["issue"]) != 242 || huaweiInt(t, contract["pull_request"]) != 253 ||
		!reflect.DeepEqual(huaweiStringSlice(t, contract["pages"]), []string{"protocols/huawei/monitoring-readonly-v1.md", "protocols/huawei/read-pacing-v1.md"}) ||
		contract["companion_required"] != true || contract["companion_state"] != "MERGED" || contract["merge_companion_first"] != true {
		t.Fatalf("unexpected monitoring contract pointer: %#v", contract)
	}
}

func TestHuaweiInverterBatteryDispositionsKeepAuthorityClosed(t *testing.T) {
	for _, tc := range huaweiInverterBatteryCases() {
		t.Run(tc.name, func(t *testing.T) {
			record := decodeHuaweiObject(t, tc.path)
			keys := []string{
				"schema", "profile", "profile_version", "outcome", "evidence", "model_families",
				"firmware_ranges", "capability_detection", "registers", "optional_groups",
				"private_functions", "writes", "decoder_keys", "catalog_registered",
				"automatic_runtime_admission", "support_claim", "write_authority", "reopen_requires", "observation_pointer",
			}
			if tc.optional {
				keys = append(keys, "battery_groups")
			} else {
				keys = append(keys, "monitoring_profiles", "version_gate", "battery_groups", "meters", "live_qualified")
			}
			requireHuaweiKeys(t, record, keys...)
			if record["schema"] != "helianthus-modbusreg-huawei-inverter-battery-disposition/v3" ||
				record["profile"] != tc.profile || record["profile_version"] != tc.version ||
				record["outcome"] != tc.outcome || record["catalog_registered"] != false ||
				record["automatic_runtime_admission"] != false || record["support_claim"] != false ||
				record["write_authority"] != false || record["writes"] != "DEFAULT_DENY" {
				t.Fatalf("unexpected %s disposition: %#v", tc.name, record)
			}
			if live, ok := record["live_qualified"]; ok && live != false {
				t.Fatalf("%s claims live qualification", tc.name)
			}

			evidence := huaweiObject(t, record["evidence"])
			requireHuaweiKeys(t, evidence, "source_document", "docs_modbus_page", "monitoring_contract", "eligible", "missing_evidence", "vendor_documentation")
			huaweiRequireMonitoringContract(t, evidence["monitoring_contract"])
			source := huaweiObject(t, evidence["source_document"])
			requireHuaweiKeys(t, source, "name", "classification", "present_in_repository", "publication_policy", "redistributable")
			wantName, wantClass := "Huawei SUN2000 MODBUS Interface Definitions", "UNVERIFIED_PROSPECTIVE_SOURCE"
			if !tc.optional {
				wantName, wantClass = huaweiSUN2000MCEdition.Title, "CONSULTED_OFFICIAL_EDITION"
			}
			if source["name"] != wantName || source["classification"] != wantClass || source["present_in_repository"] != false ||
				!strings.Contains(source["publication_policy"].(string), "official vendor documentation may be consulted and cited") ||
				!strings.Contains(source["publication_policy"].(string), "identity only (title, version and date)") ||
				source["redistributable"] != false {
				t.Fatalf("unexpected source document: %#v", source)
			}
			page := huaweiObject(t, evidence["docs_modbus_page"])
			requireHuaweiKeys(t, page, "exists", "path", "companion_required", "companion_state", "merge_companion_first", "docs_merge_sha")
			if page["exists"] != true || page["path"] != "protocols/huawei/inverter-battery-readonly-v1.md" ||
				page["companion_required"] != true || page["companion_state"] != "MERGED" || page["docs_merge_sha"] != "5d8a25871a9fbc48ca6e87e249b6b8603a131920" || page["merge_companion_first"] != true ||
				evidence["eligible"] != !tc.optional {
				t.Fatalf("unexpected docs-modbus page state: %#v", page)
			}
			missing := huaweiStringSlice(t, evidence["missing_evidence"])
			wantMissing := []string{"catalogue_monitoring_profile"}
			if tc.optional {
				wantMissing = []string{
					"pinned_vendor_document_identity",
					"companion_docs_modbus_protocol_page",
					"clean_room_sanitized_identity_fixture",
					"clean_room_sanitized_monitoring_fixture",
					"negative_overlap_with_smartlogger_emma_sdongle",
				}
			}
			if !reflect.DeepEqual(missing, wantMissing) || !reflect.DeepEqual(huaweiStringSlice(t, record["reopen_requires"]), missing) {
				t.Fatalf("missing_evidence=%v reopen_requires=%v want %v", missing, record["reopen_requires"], wantMissing)
			}

			families := huaweiObject(t, record["model_families"])
			if tc.optional {
				requireHuaweiKeys(t, families, "class", "covered", "unsupported")
			} else {
				requireHuaweiKeys(t, families, "class", "covered", "selectable", "without_catalogue_row")
			}
			ranges := huaweiObject(t, record["firmware_ranges"])
			if families["class"] != tc.class || len(h03Array(t, ranges["covered"])) != 0 {
				t.Fatalf("unexpected coverage: %#v %#v", families, ranges)
			}
			if tc.optional {
				requireHuaweiKeys(t, ranges, "covered", "unsupported")
				if len(h03Array(t, families["covered"])) != 0 || families["unsupported"] != "ALL" || ranges["unsupported"] != "ALL" ||
					len(h03Array(t, record["decoder_keys"])) != 0 {
					t.Fatalf("a LUNA2000 family, firmware range or decoder was claimed: %#v %#v", families, ranges)
				}
			} else {
				requireHuaweiKeys(t, ranges, "covered", "predicate", "reason")
				if !reflect.DeepEqual(huaweiStringSlice(t, families["covered"]), []string{huaweiSUN2000M3Family, huaweiSUN2000M1Family}) ||
					families["selectable"] != "CATALOGUE_MODEL_ROWS" || families["without_catalogue_row"] != "NOT_SELECTED" ||
					ranges["predicate"] != "SOFTWARE_FLOOR_ONLY" {
					t.Fatalf("unexpected SUN2000 coverage: %#v %#v", families, ranges)
				}
			}

			registers := huaweiObject(t, record["registers"])
			if len(h03Array(t, registers["candidate"])) != 0 {
				t.Fatalf("candidate registers claimed: %#v", registers)
			}
			if tc.optional && len(h03Array(t, registers["evidence_backed"])) != 0 {
				t.Fatalf("LUNA2000 registers claimed: %#v", registers)
			}

			optional := huaweiObject(t, record["optional_groups"])
			requireHuaweiKeys(t, optional, "declared", "absent_semantics", "exception_semantics")
			if optional["absent_semantics"] != "UNKNOWN_NOT_ZERO" ||
				optional["exception_semantics"] != "RETAIN_LAST_KNOWN_GOOD_NO_NEGATION" {
				t.Fatalf("unexpected optional group semantics: %#v", optional)
			}
			declared := huaweiStringSlice(t, optional["declared"])
			if tc.optional && !reflect.DeepEqual(declared, []string{"battery_unit_presence"}) {
				t.Fatalf("LUNA2000 optional group not declared: %v", declared)
			}
			if !tc.optional && !reflect.DeepEqual(declared, []string{"battery_attachment"}) {
				t.Fatalf("SUN2000 battery attachment group not declared: %v", declared)
			}

			private := huaweiObject(t, record["private_functions"])
			requireHuaweiKeys(t, private, "FC0x41", "FC0x17")
			if private["FC0x41"] != "NO_SEND" || private["FC0x17"] != "NO_SEND" {
				t.Fatalf("private function codes are not default-denied: %#v", private)
			}
		})
	}
}

func TestHuaweiBatteryGroupsArePresentButNotAdmitted(t *testing.T) {
	luna := huaweiObject(t, decodeHuaweiObject(t, "profiles/vendor/huawei/luna2000-disposition.json")["battery_groups"])
	requireHuaweiKeys(t, luna, "status", "groups", "answered_with_data_on", "presence_grade", "meanings_grade", "zero_word_rule_grade",
		"presence_word_grade", "facts_admitted", "port_definitions")
	var groups []string
	for _, value := range h03Array(t, luna["groups"]) {
		group := huaweiObject(t, value)
		groups = append(groups, fmt.Sprintf("%d/%d", huaweiInt(t, group["offset"]), huaweiInt(t, group["quantity"])))
	}
	if luna["status"] != "PRESENT_NOT_ADMITTED" || !reflect.DeepEqual(groups, []string{"37000/30", "37066/4", "47000/1"}) ||
		!reflect.DeepEqual(huaweiStringSlice(t, luna["answered_with_data_on"]), []string{"SUN2000 MAP0 series"}) ||
		luna["presence_grade"] != "PROVEN" || luna["meanings_grade"] != "UNKNOWN" || luna["zero_word_rule_grade"] != "HYPOTHESIS" ||
		luna["presence_word_grade"] != "UNKNOWN" || luna["facts_admitted"] != false {
		t.Fatalf("LUNA2000 battery groups: %#v", luna)
	}
	sun := decodeHuaweiObject(t, "profiles/vendor/huawei/sun2000-disposition.json")
	battery := huaweiObject(t, sun["battery_groups"])
	meters := huaweiObject(t, sun["meters"])
	if battery["admitted"] != false || meters["admitted"] != false || meters["grade"] != "UNKNOWN" ||
		!reflect.DeepEqual(huaweiStringSlice(t, battery["zero_word_on"]), []string{huaweiSUN2000M3Family, huaweiSUN2000M1Family}) {
		t.Fatalf("SUN2000 battery or meter record: %#v %#v", battery, meters)
	}
	for _, key := range huaweiStringSlice(t, sun["decoder_keys"]) {
		for _, token := range []string{"battery", "soc", "meter", "grid"} {
			if strings.Contains(key, token) {
				t.Fatalf("decoder key %s admits a battery or meter fact", key)
			}
		}
	}
}

func TestHuaweiInverterBatteryCapabilityDetectionDecisionTable(t *testing.T) {
	for _, tc := range huaweiInverterBatteryCases() {
		t.Run(tc.name, func(t *testing.T) {
			record := decodeHuaweiObject(t, tc.path)
			detection := huaweiObject(t, record["capability_detection"])
			forbidden := huaweiStringSlice(t, detection["forbidden_identity"])
			for _, name := range []string{
				"serial_number", "esn", "writable_alias", "register_readability",
				"gateway_inventory_child_model", "model_prefix_or_wildcard",
			} {
				if !containsString(forbidden, name) {
					t.Fatalf("forbidden identity lacks %q: %v", name, forbidden)
				}
			}
			if detection["default_outcome"] != "INSUFFICIENT_EVIDENCE" || detection["multiple_positive_outcome"] != "AMBIGUOUS" ||
				detection["first_match_priority"] != false {
				t.Fatalf("unexpected detection policy: %#v", detection)
			}
			if tc.optional {
				requireHuaweiKeys(t, detection, "positive_identity_required", "forbidden_identity",
					"default_outcome", "multiple_positive_outcome", "first_match_priority", "executable", "rows")
				if detection["executable"] != false || !reflect.DeepEqual(huaweiStringSlice(t, detection["positive_identity_required"]), []string{
					"exact_model_identity", "exact_firmware_identity", "unit_identity", "pinned_register_map",
				}) {
					t.Fatalf("unexpected LUNA2000 detection: %#v", detection)
				}
			} else {
				requireHuaweiKeys(t, detection, "positive_identity_required", "not_predicates", "catalogue_decision", "forbidden_identity",
					"default_outcome", "multiple_positive_outcome", "first_match_priority", "executable", "rows")
				if detection["executable"] != true || !reflect.DeepEqual(huaweiStringSlice(t, detection["positive_identity_required"]), []string{
					"observed_gateway_identity_card", "routed_unit", "catalogue_model_row",
				}) || !reflect.DeepEqual(huaweiStringSlice(t, detection["not_predicates"]), []string{"protocol_words"}) {
					t.Fatalf("unexpected SUN2000 detection: %#v", detection)
				}
			}

			var ids []string
			for _, value := range h03Array(t, detection["rows"]) {
				row := huaweiObject(t, value)
				requireHuaweiKeys(t, row, "id", "outcome", "selected_profile", "native_fact_count", "reason")
				id, _ := row["id"].(string)
				ids = append(ids, id)
				selected := row["selected_profile"] != ""
				if selected != (row["outcome"] == "PROFILE_SELECTED") || selected != (huaweiInt(t, row["native_fact_count"]) > 0) {
					t.Fatalf("row %q mixes selection and facts: %#v", id, row)
				}
				if tc.optional && (selected || (row["outcome"] != "INSUFFICIENT_EVIDENCE" && row["outcome"] != "AMBIGUOUS" && row["outcome"] != "NO_ADMISSIBLE_PROFILE")) {
					t.Fatalf("LUNA2000 row %q selects: %#v", id, row)
				}
			}
			if !reflect.DeepEqual(ids, tc.rowsWanted) {
				t.Fatalf("decision-table rows=%v want=%v", ids, tc.rowsWanted)
			}
		})
	}
}

func TestHuaweiInverterBatteryProfilesAreNotRegistered(t *testing.T) {
	entries, err := os.ReadDir("profiles/vendor/huawei")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	want := []string{
		"catalogue", "emma-disposition.json", "evidence.json", "luna2000-disposition.json",
		"sdongle-disposition.json", "smartlogger-disposition.json", "sun2000-disposition.json",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("huawei vendor files=%v want=%v", names, want)
	}
	// The generated catalogue is registry data only: it names no profile
	// identifier, so it registers no SUN2000 or LUNA2000 profile.
	catalogueFiles, err := filepath.Glob("profiles/vendor/huawei/catalogue/*.json")
	if err != nil || len(catalogueFiles) == 0 {
		t.Fatalf("catalogue files not found: %v", err)
	}
	for _, file := range catalogueFiles {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(data))
		if strings.Contains(lower, "huawei.sun2000") || strings.Contains(lower, "huawei.luna2000") {
			t.Fatalf("%s names a Huawei profile identifier", file)
		}
	}

	closure, err := os.ReadFile("profiles/mixed-catalog-closure-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	// Only the two monitoring files name a SUN2000 profile, and only by the
	// two exact profile identifiers and their decoder keys. No LUNA2000 profile
	// is named anywhere.
	allowed := map[string]bool{"huawei_sun2000_monitoring.go": true, "huawei_sun2000_capability.go": true}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(data))
		if strings.Contains(lower, "huawei.luna2000") {
			t.Fatalf("%s registers or names huawei.luna2000", file)
		}
		if !strings.Contains(lower, "huawei.sun2000") {
			continue
		}
		if !allowed[file] {
			t.Fatalf("%s registers or names huawei.sun2000", file)
		}
		for _, line := range strings.Split(string(data), "\n") {
			index := strings.Index(line, "huawei.sun2000")
			if index < 0 {
				continue
			}
			rest := line[index:]
			if !strings.HasPrefix(rest, "huawei.sun2000.m3.") && !strings.HasPrefix(rest, "huawei.sun2000.m1.") {
				t.Fatalf("%s names a SUN2000 profile outside the two exact families: %q", file, strings.TrimSpace(line))
			}
		}
	}
	for _, token := range []string{"sun2000", "luna2000"} {
		if strings.Contains(strings.ToLower(string(closure)), token) {
			t.Fatalf("mixed catalog closure admits %s", token)
		}
	}
	for _, profile := range HuaweiSUN2000MonitoringProfiles() {
		if profile.CatalogRegistered() || profile.AutomaticRuntimeAdmission() || profile.LiveQualified() || profile.SupportClaim() || profile.WriteAuthority() {
			t.Fatalf("%s grants authority", profile.ID())
		}
	}
}

func TestHuaweiInventoryChildModelDoesNotSelectInverterProfile(t *testing.T) {
	result := NewHuaweiSmartLoggerQualificationCard().Evaluate(validHuaweiSmartLoggerQualificationInput())
	if result.SelectedClass() != HuaweiSmartLoggerCanonicalClass || result.NativeFactCount() != 0 ||
		result.CatalogRegistered() || result.SupportClaim() || result.FallbackAttempted() {
		t.Fatalf("unexpected SmartLogger result: %#v", result)
	}
	inventory, ok := result.Inventory()
	if !ok || inventory.Children()[0].Attribute("model") != "SUN2000" {
		t.Fatalf("SUN2000 child fixture missing: ok=%t", ok)
	}
	for _, class := range result.CandidateClasses() {
		if strings.Contains(strings.ToUpper(class), "SUN2000") || strings.Contains(strings.ToUpper(class), "LUNA2000") {
			t.Fatalf("child model became a candidate class: %v", result.CandidateClasses())
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
