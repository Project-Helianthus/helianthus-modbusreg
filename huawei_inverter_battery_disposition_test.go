package modbusreg

import (
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
	class      string
	optional   bool
	rowsWanted []string
}

func huaweiInverterBatteryCases() []huaweiInverterBatteryCase {
	return []huaweiInverterBatteryCase{
		{
			name: "SUN2000", path: "profiles/vendor/huawei/sun2000-disposition.json",
			profile: "huawei.sun2000", class: "SUN2000", optional: false,
			rowsWanted: []string{
				"no_identity_evidence",
				"gateway_inventory_child_model_only",
				"model_without_firmware",
				"firmware_without_model",
				"serial_or_esn_only",
				"register_readability_only",
				"battery_group_absent",
				"battery_group_exception",
				"multiple_positive_candidates",
				"exact_identity_with_pinned_register_map",
			},
		},
		{
			name: "LUNA2000", path: "profiles/vendor/huawei/luna2000-disposition.json",
			profile: "huawei.luna2000", class: "LUNA2000", optional: true,
			rowsWanted: []string{
				"no_identity_evidence",
				"gateway_inventory_child_model_only",
				"parent_inverter_unidentified",
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

func TestHuaweiInverterBatteryDispositionsFailClosedWithoutEvidence(t *testing.T) {
	for _, tc := range huaweiInverterBatteryCases() {
		t.Run(tc.name, func(t *testing.T) {
			record := decodeHuaweiObject(t, tc.path)
			requireHuaweiKeys(t, record,
				"schema", "profile", "profile_version", "outcome", "evidence", "model_families",
				"firmware_ranges", "capability_detection", "registers", "optional_groups",
				"private_functions", "writes", "decoder_keys", "catalog_registered",
				"automatic_runtime_admission", "support_claim", "write_authority", "reopen_requires", "observation_pointer",
			)
			if record["schema"] != "helianthus-modbusreg-huawei-inverter-battery-disposition/v2" ||
				record["profile"] != tc.profile || record["profile_version"] != "1.0.0" ||
				record["outcome"] != "NO_ADMISSIBLE_PROFILE" || record["catalog_registered"] != false ||
				record["automatic_runtime_admission"] != false || record["support_claim"] != false ||
				record["write_authority"] != false || record["writes"] != "DEFAULT_DENY" ||
				len(record["decoder_keys"].([]any)) != 0 {
				t.Fatalf("unexpected %s disposition: %#v", tc.name, record)
			}

			evidence := huaweiObject(t, record["evidence"])
			requireHuaweiKeys(t, evidence, "source_document", "docs_modbus_page", "eligible", "missing_evidence", "vendor_documentation")
			source := huaweiObject(t, evidence["source_document"])
			requireHuaweiKeys(t, source, "name", "classification", "present_in_repository", "publication_policy", "redistributable")
			if source["name"] != "Huawei SUN2000 MODBUS Interface Definitions" ||
				source["classification"] != "UNVERIFIED_PROSPECTIVE_SOURCE" || source["present_in_repository"] != false ||
				!strings.Contains(source["publication_policy"].(string), "official vendor documentation may be consulted and cited") ||
				!strings.Contains(source["publication_policy"].(string), "identity only (title, version and date)") ||
				source["redistributable"] != false {
				t.Fatalf("unexpected source document: %#v", source)
			}
			page := huaweiObject(t, evidence["docs_modbus_page"])
			requireHuaweiKeys(t, page, "exists", "path", "companion_required", "companion_state", "merge_companion_first", "docs_merge_sha")
			if page["exists"] != true || page["path"] != "protocols/huawei/inverter-battery-readonly-v1.md" ||
				page["companion_required"] != true || page["companion_state"] != "MERGED" || page["docs_merge_sha"] != "5d8a25871a9fbc48ca6e87e249b6b8603a131920" || page["merge_companion_first"] != true || evidence["eligible"] != false {
				t.Fatalf("unexpected docs-modbus page state: %#v", page)
			}
			missing := huaweiStringSlice(t, evidence["missing_evidence"])
			for _, name := range []string{
				"pinned_vendor_document_identity",
				"companion_docs_modbus_protocol_page",
				"clean_room_sanitized_identity_fixture",
				"clean_room_sanitized_monitoring_fixture",
				"negative_overlap_with_smartlogger_emma_sdongle",
			} {
				if !containsString(missing, name) {
					t.Fatalf("missing_evidence lacks %q: %v", name, missing)
				}
			}
			if !reflect.DeepEqual(huaweiStringSlice(t, record["reopen_requires"]), missing) {
				t.Fatalf("reopen_requires diverges from missing_evidence")
			}

			families := huaweiObject(t, record["model_families"])
			requireHuaweiKeys(t, families, "class", "covered", "unsupported")
			ranges := huaweiObject(t, record["firmware_ranges"])
			requireHuaweiKeys(t, ranges, "covered", "unsupported")
			if families["class"] != tc.class || len(families["covered"].([]any)) != 0 || families["unsupported"] != "ALL" ||
				len(ranges["covered"].([]any)) != 0 || ranges["unsupported"] != "ALL" {
				t.Fatalf("a model family or firmware range was claimed: %#v %#v", families, ranges)
			}

			registers := huaweiObject(t, record["registers"])
			requireHuaweiKeys(t, registers, "evidence_backed", "candidate", "unknown")
			if len(registers["evidence_backed"].([]any)) != 0 || len(registers["candidate"].([]any)) != 0 {
				t.Fatalf("registers claimed without admissible evidence: %#v", registers)
			}
			unknown := huaweiStringSlice(t, registers["unknown"])
			for _, name := range []string{
				"model_identity", "firmware_identity", "unit_identity", "monitoring_registers",
				"scale_and_unit", "access_mode", "availability_markers",
			} {
				if !containsString(unknown, name) {
					t.Fatalf("unknown registers lack %q: %v", name, unknown)
				}
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

func TestHuaweiInverterBatteryCapabilityDetectionDecisionTable(t *testing.T) {
	for _, tc := range huaweiInverterBatteryCases() {
		t.Run(tc.name, func(t *testing.T) {
			record := decodeHuaweiObject(t, tc.path)
			detection := huaweiObject(t, record["capability_detection"])
			requireHuaweiKeys(t, detection, "positive_identity_required", "forbidden_identity",
				"default_outcome", "multiple_positive_outcome", "first_match_priority", "executable", "rows")
			if detection["default_outcome"] != "INSUFFICIENT_EVIDENCE" || detection["multiple_positive_outcome"] != "AMBIGUOUS" ||
				detection["first_match_priority"] != false || detection["executable"] != false {
				t.Fatalf("unexpected detection policy: %#v", detection)
			}
			if !reflect.DeepEqual(huaweiStringSlice(t, detection["positive_identity_required"]), []string{
				"exact_model_identity", "exact_firmware_identity", "unit_identity", "pinned_register_map",
			}) {
				t.Fatalf("unexpected positive identity tuple: %#v", detection["positive_identity_required"])
			}
			forbidden := huaweiStringSlice(t, detection["forbidden_identity"])
			for _, name := range []string{
				"serial_number", "esn", "writable_alias", "register_readability",
				"gateway_inventory_child_model", "model_prefix_or_wildcard",
			} {
				if !containsString(forbidden, name) {
					t.Fatalf("forbidden identity lacks %q: %v", name, forbidden)
				}
			}

			rows, ok := detection["rows"].([]any)
			if !ok {
				t.Fatalf("rows is not an array")
			}
			var ids []string
			for _, value := range rows {
				row := huaweiObject(t, value)
				requireHuaweiKeys(t, row, "id", "outcome", "selected_profile", "native_fact_count", "reason")
				id, _ := row["id"].(string)
				ids = append(ids, id)
				if row["selected_profile"] != "" || huaweiInt(t, row["native_fact_count"]) != 0 {
					t.Fatalf("row %q selects a profile or emits facts: %#v", id, row)
				}
				want := "INSUFFICIENT_EVIDENCE"
				switch id {
				case "multiple_positive_candidates":
					want = "AMBIGUOUS"
				case "exact_identity_with_pinned_register_map":
					want = "NO_ADMISSIBLE_PROFILE"
				}
				if row["outcome"] != want {
					t.Fatalf("row %q outcome=%v want %s", id, row["outcome"], want)
				}
			}
			sortedIDs := append([]string(nil), ids...)
			sort.Strings(sortedIDs)
			wantIDs := append([]string(nil), tc.rowsWanted...)
			sort.Strings(wantIDs)
			if !reflect.DeepEqual(sortedIDs, wantIDs) {
				t.Fatalf("decision-table rows=%v want=%v", sortedIDs, wantIDs)
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
		"emma-disposition.json", "evidence.json", "luna2000-disposition.json",
		"sdongle-disposition.json", "smartlogger-disposition.json", "sun2000-disposition.json",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("huawei vendor files=%v want=%v", names, want)
	}

	closure, err := os.ReadFile("profiles/mixed-catalog-closure-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(data))
		for _, token := range []string{"huawei.sun2000", "huawei.luna2000"} {
			if strings.Contains(lower, token) {
				t.Fatalf("%s registers or names %s", file, token)
			}
		}
	}
	for _, token := range []string{"sun2000", "luna2000"} {
		if strings.Contains(strings.ToLower(string(closure)), token) {
			t.Fatalf("mixed catalog closure admits %s", token)
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
