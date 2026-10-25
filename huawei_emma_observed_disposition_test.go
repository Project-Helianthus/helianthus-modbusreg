package modbusreg

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// h18RowOutcome reads the outcome that a negative-overlap row claims out of the
// fixture that the row names, so that the disposition can never say more or less
// than the fixtures hold.
func h18RowOutcome(t *testing.T, row map[string]any) string {
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
		case "emma_gateway_basic_reply.json":
			var fixture h03BasicFixture
			if err := h03Decode(data, &fixture); err != nil || !containsInt(fixture.AnsweredForUnits, unit) || fixture.Read.ReadDeviceIDCode != 1 {
				t.Fatalf("basic reply fixture does not bear on unit %d: %v", unit, err)
			}
			return "ANSWERED_MEI_BASIC"
		case "emma_extended_inventory.json":
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

func TestH18EMMADispositionSupersedesTheNoResponseRecordForIdentityOnly(t *testing.T) {
	card := NewHuaweiEMMAObservedQualificationCard()
	record := decodeHuaweiObject(t, "profiles/vendor/huawei/emma-disposition.json")
	missing := card.MissingEvidence()

	// The profile outcome and every authority field stay where they were.
	if record["outcome"] != "OFFLINE_IDENTITY_ADMITTED" || record["catalog_registered"] != false || record["automatic_runtime_admission"] != false ||
		record["support_claim"] != false || record["write_authority"] != false || record["offline_identity_gate_registered"] != true ||
		len(h03Array(t, record["decoder_keys"])) != 0 || !reflect.DeepEqual(huaweiStringSlice(t, record["reopen_requires"]), missing) {
		t.Fatalf("unexpected EMMA profile status: %#v", record)
	}
	private := huaweiObject(t, record["private_functions"])
	enumeration := huaweiObject(t, record["child_enumeration"])
	if private["FC0x41"] != "NO_SEND" || private["FC0x17"] != "NO_SEND" || enumeration["executable"] != false {
		t.Fatalf("a no-send rule opened: %#v %#v", private, enumeration)
	}

	// Evidence: the overlap gap with the SmartLogger path closes for the observed
	// tuple, the capability gaps and the battery addressing stay open, and every
	// row points at a committed fixture.
	evidence := huaweiObject(t, record["evidence"])
	if !reflect.DeepEqual(huaweiStringSlice(t, evidence["missing_capability_evidence"]), missing) || evidence["eligible"] != true {
		t.Fatalf("missing capability evidence: %#v", evidence["missing_capability_evidence"])
	}
	closed := h03Array(t, evidence["closed_evidence"])
	if len(closed) != 1 {
		t.Fatalf("closed evidence: %#v", closed)
	}
	closedRow := huaweiObject(t, closed[0])
	requireHuaweiKeys(t, closedRow, "gap", "scope", "fixtures")
	if closedRow["gap"] != "negative_overlap_with_smartlogger" || closedRow["scope"] != "observed_tuple" ||
		!reflect.DeepEqual(huaweiStringSlice(t, closedRow["fixtures"]), []string{h18FixturePaths()[4], h03FixturePaths[2]}) || containsString(missing, closedRow["gap"].(string)) {
		t.Fatalf("closed evidence row: %#v", closedRow)
	}
	h03RequireFiles(t, huaweiStringSlice(t, closedRow["fixtures"]))
	if !reflect.DeepEqual(huaweiStringSlice(t, evidence["unknown_after_observation"]), []string{
		"battery_addressing_behind_gateway", "unit_2_entry_identity_and_reads", "inverter_units_other_than_3_and_4",
		"emma_model_aliases_other_than_the_observed_one", "smartlogger_counter_read_at_unit_100", "cause_of_the_earlier_connection_resets",
		"meaning_of_the_declared_object_field",
	}) {
		t.Fatalf("unknown after observation: %#v", evidence["unknown_after_observation"])
	}
	// The EMMA contract is the observed-identity page of its own docs change, not the
	// SmartLogger one.
	docs := huaweiObject(t, evidence["docs_contract"])
	requireHuaweiKeys(t, docs, "repository", "issue", "pull_request", "pages")
	emmaPages := []string{
		"protocols/huawei/gateway-readonly-v1.md",
		"protocols/huawei/emma-observed-identity-v1.md",
		"protocols/huawei/smartlogger-observed-inventory-v1.md",
		"protocols/huawei/qualification-readiness-v1.md",
	}
	if docs["repository"] != "helianthus-docs-modbus" || huaweiInt(t, docs["issue"]) != 249 || huaweiInt(t, docs["pull_request"]) != 252 ||
		!reflect.DeepEqual(huaweiStringSlice(t, docs["pages"]), emmaPages) {
		t.Fatalf("EMMA docs contract pointer: %#v", docs)
	}
	for _, page := range emmaPages {
		if !strings.HasPrefix(page, "protocols/huawei/") || !strings.HasSuffix(page, ".md") || strings.Contains(page, "#") || h03Digestlike.MatchString(page) {
			t.Fatalf("docs page %q is not a plain repository-relative path", page)
		}
	}

	// The recorded non-response stays on record and is superseded for the observed
	// identity only; the earlier evidence it cited is still cited.
	earlier := huaweiObject(t, evidence["earlier_observation"])
	requireHuaweiKeys(t, earlier, "state", "recorded_state", "runs", "requests", "attempts", "answered_requests", "superseded_by", "reproduced", "identification_claim", "incompatibility_claim", "fixtures")
	if earlier["state"] != "SUPERSEDED_FOR_OBSERVED_IDENTITY" || earlier["recorded_state"] != "NO_MODBUS_RESPONSE" || earlier["superseded_by"] != "observed_identity" ||
		huaweiInt(t, earlier["runs"]) != 4 || huaweiInt(t, earlier["requests"]) != 80 || huaweiInt(t, earlier["attempts"]) != 320 || huaweiInt(t, earlier["answered_requests"]) != 0 ||
		earlier["reproduced"] != false || earlier["identification_claim"] != false || earlier["incompatibility_claim"] != false ||
		!reflect.DeepEqual(huaweiStringSlice(t, earlier["fixtures"]), []string{h03FixturePaths[2]}) {
		t.Fatalf("earlier observation: %#v", earlier)
	}
	h03RequireFiles(t, huaweiStringSlice(t, earlier["fixtures"]))
	if _, ok := record["observation_pointer"]; ok {
		t.Fatal("the EMMA disposition still carries a pointer row next to its observed identity")
	}

	observed := huaweiObject(t, record["observed_identity"])
	requireHuaweiKeys(t, observed, "status", "scope", "qualification_card", "gate_registered", "default_denied", "supersedes", "required_tuple",
		"exact_tuple", "tolerance", "retained_not_gating", "forbidden_identity", "outside_admitted_set_outcome", "multiple_positive_outcome",
		"first_match_priority", "timing", "negative_overlap", "batteries", "fixtures", "profile_status")
	if observed["status"] != "OFFLINE_IDENTITY_ADMITTED" || observed["scope"] != "OBSERVED_TUPLE_ONLY" || observed["qualification_card"] != card.ID() ||
		observed["gate_registered"] != true || observed["default_denied"] != true || observed["outside_admitted_set_outcome"] != "INSUFFICIENT_EVIDENCE" ||
		observed["multiple_positive_outcome"] != "INSUFFICIENT_EVIDENCE" || observed["first_match_priority"] != false ||
		!reflect.DeepEqual(huaweiStringSlice(t, observed["fixtures"]), h18FixturePaths()) {
		t.Fatalf("unexpected observed identity: %#v", observed)
	}
	h03RequireFiles(t, h18FixturePaths())
	supersedes := huaweiObject(t, observed["supersedes"])
	requireHuaweiKeys(t, supersedes, "documentary_card", "superseded_for", "superseded_only_by")
	if supersedes["documentary_card"] != HuaweiEMMAQualificationCardID || supersedes["superseded_for"] != "IDENTITY_OF_OBSERVED_TUPLE_ONLY" ||
		supersedes["superseded_only_by"] != "MATCHED_OBSERVED_CARD_OF_THE_SAME_CLASS" {
		t.Fatalf("supersession: %#v", supersedes)
	}

	// The exact tuple in the record is the card's.
	tuple := huaweiObject(t, observed["exact_tuple"])
	requireHuaweiKeys(t, tuple, "gateway_unit", "gateway_family_label", "gateway_model", "gateway_software_label", "gateway_basic_mei", "inventory",
		"inverter_units", "inverter_model", "selected_class", "gateway_model_variant")
	basicMEI := huaweiObject(t, tuple["gateway_basic_mei"])
	inventory := huaweiObject(t, tuple["inventory"])
	requireHuaweiKeys(t, inventory, "count_object", "self_entry_unit", "children")
	members := h03Array(t, inventory["children"])
	if huaweiInt(t, tuple["gateway_unit"]) != int(card.UnitID()) || tuple["gateway_family_label"] != card.FamilyLabel() || tuple["gateway_model"] != card.Model() ||
		tuple["gateway_model_variant"] != card.Model() || tuple["selected_class"] != card.CandidateClass() ||
		tuple["gateway_software_label"] != "FAMILY_LABEL_SPACE_VERSION_TOKEN_MEETING_THE_DOCUMENTARY_FLOOR" || tuple["inverter_model"] != "SUN2000_FAMILY_EQUALS_OWN_ENTRY" ||
		basicMEI["vendor"] != card.Vendor() || basicMEI["product"] != card.Product() || basicMEI["version"] != "EQUALS_SOFTWARE_LABEL_TOKEN" ||
		huaweiInt(t, inventory["count_object"]) != 4 || huaweiInt(t, inventory["self_entry_unit"]) != int(card.UnitID()) || len(members) != len(huaweiEMMAObservedMembers) {
		t.Fatalf("exact tuple: %#v", tuple)
	}
	for index, value := range members {
		row := huaweiObject(t, value)
		requireHuaweiKeys(t, row, "unit", "product_type", "model_family")
		member := huaweiEMMAObservedMembers[index]
		wantFamily := member.modelFamily
		if wantFamily == "" {
			wantFamily = "WITHHELD_NOT_COMPARED"
		}
		if fmt.Sprint(huaweiInt(t, row["unit"])) != member.unit || row["product_type"] != member.productType || row["model_family"] != wantFamily {
			t.Fatalf("member %d: %#v", index, row)
		}
	}
	var units []uint8
	for _, value := range h03Array(t, tuple["inverter_units"]) {
		units = append(units, uint8(huaweiInt(t, value)))
	}
	if !reflect.DeepEqual(units, card.ChildUnits()) {
		t.Fatalf("inverter units %v, card %v", units, card.ChildUnits())
	}
	if !reflect.DeepEqual(huaweiStringSlice(t, observed["required_tuple"]), []string{
		"gateway_unit_0", "gateway_fc03_30000_q15_family_label_exact", "gateway_fc03_30222_q20_model_exact",
		"gateway_fc03_30035_q15_family_label_space_version_token_meeting_the_documentary_floor", "gateway_basic_mei_vendor_and_product_exact_version_equals_software_token",
		"gateway_extended_inventory_count_4_self_backup_box_unit_2_sun2000_units_3_and_4", "gateway_self_entry_model_product_type_and_version_equal_the_blocks",
		"inverter_units_3_and_4_fc03_30000_q15_sun2000_family_model_equals_own_entry",
	}) || !reflect.DeepEqual(huaweiStringSlice(t, observed["retained_not_gating"]), []string{
		"gateway_serial_bearing_block_30015", "inverter_serial_bearing_block_30015", "inverter_firmware_block_30035", "inverter_protocol_words_30068",
		"entry_attributes_3_4_6_9_10", "entry_attribute_2_of_units_2_3_4", "unit_2_model",
		"basic_mei_objects_beyond_the_basic_category", "model_block_residue", "request_timing",
	}) || !reflect.DeepEqual(huaweiStringSlice(t, observed["forbidden_identity"]), []string{
		"serial_number", "esn", "register_readability", "basic_mei_only", "basic_mei_answered_at_another_unit", "extended_inventory_only", "child_listing",
		"unit_100_behaviour", "sunspec_marker_offset_readability", "mei_request_unit_identifier", "prefix_wildcard_case_folded_or_whitespace_normalized_match",
	}) {
		t.Fatalf("observed identity lists: %#v", observed)
	}
	tolerance := huaweiObject(t, observed["tolerance"])
	requireHuaweiKeys(t, tolerance, "model_block_residue", "declared_object_field", "page_layout", "entry_padding", "attribute_order", "unknown_or_repeated_keys",
		"max_pages", "max_response_bytes")
	if huaweiInt(t, tolerance["max_response_bytes"]) != int(card.Limits().MaxResponseBytes) || huaweiInt(t, tolerance["max_pages"]) != int(card.Limits().MaxPages) ||
		tolerance["declared_object_field"] != "RETAINED_NOT_COMPARED" || tolerance["entry_padding"] != "NOT_REQUIRED" || tolerance["attribute_order"] != "NOT_VALIDATED" ||
		tolerance["unknown_or_repeated_keys"] != "REJECT_INVENTORY" || tolerance["page_layout"] != "NOT_COMPARED_EACH_RESPONSE_WITHIN_THE_CLASSIC_FRAME" ||
		tolerance["model_block_residue"] != "PRINTABLE_ASCII_OR_NUL_AFTER_FIRST_NUL_RETAINED_NOT_COMPARED" {
		t.Fatalf("tolerance: %#v", tolerance)
	}
	timing := huaweiObject(t, observed["timing"])
	if timing["request_timeouts"] != "NONE_OBSERVED" || huaweiInt(t, timing["retries"]) != 0 || timing["fixture"] != h18FixturePaths()[5] {
		t.Fatalf("timing: %#v", timing)
	}

	// Monitoring and batteries are not admitted, and the battery group stays Unknown.
	batteries := huaweiObject(t, observed["batteries"])
	requireHuaweiKeys(t, batteries, "status", "listed_in_inventory", "addressing", "profile", "profile_outcome")
	if batteries["status"] != "NOT_ADMITTED" || batteries["listed_in_inventory"] != false || batteries["addressing"] != "UNKNOWN" ||
		batteries["profile"] != "huawei.luna2000" || batteries["profile_outcome"] != "NO_ADMISSIBLE_PROFILE" {
		t.Fatalf("batteries: %#v", batteries)
	}
	luna := decodeHuaweiObject(t, "profiles/vendor/huawei/luna2000-disposition.json")
	if luna["outcome"] != batteries["profile_outcome"] || luna["profile"] != batteries["profile"] {
		t.Fatalf("the battery disposition moved: %#v", luna["outcome"])
	}
	profile := huaweiObject(t, observed["profile_status"])
	requireHuaweiKeys(t, profile, "outcome", "monitoring", "batteries", "blocked_by")
	if profile["outcome"] != record["outcome"] || profile["monitoring"] != "NOT_ADMITTED" || profile["batteries"] != "NOT_ADMITTED" ||
		!reflect.DeepEqual(huaweiStringSlice(t, profile["blocked_by"]), missing) {
		t.Fatalf("profile status: %#v", profile)
	}
	for _, name := range []string{"battery_addressing_behind_gateway_fixture"} {
		if !containsString(missing, name) {
			t.Fatalf("%s is not listed as missing", name)
		}
	}
}

func TestH18EMMANegativeOverlapRowsMatchTheirFixtures(t *testing.T) {
	record := decodeHuaweiObject(t, "profiles/vendor/huawei/emma-disposition.json")
	overlap := huaweiObject(t, huaweiObject(t, record["observed_identity"])["negative_overlap"])
	requireHuaweiKeys(t, overlap, "versus_smartlogger", "versus_sdongle", "versus_sunspec", "versus_inverter_units", "readability_alone_is_never_identity", "open_directions")
	if overlap["readability_alone_is_never_identity"] != true ||
		!reflect.DeepEqual(huaweiStringSlice(t, overlap["open_directions"]), []string{
			"growatt_probes", "smartlogger_counter_read_at_unit_100", "sdongle_probes_at_inverter_unit_4",
		}) {
		t.Fatalf("overlap summary: %#v", overlap)
	}
	known := map[string]bool{}
	for _, path := range h18FixturePaths() {
		known[path] = true
	}
	counts := map[string]int{}
	labels := map[string][]string{}
	for _, group := range []string{"versus_smartlogger", "versus_sdongle", "versus_sunspec", "versus_inverter_units"} {
		for _, value := range h03Array(t, overlap[group]) {
			row := huaweiObject(t, value)
			if !known[row["fixture"].(string)] {
				t.Fatalf("%s: row names an unknown fixture: %#v", group, row)
			}
			if got := h18RowOutcome(t, row); got != row["outcome"] {
				t.Fatalf("%s: row %v claims %v, fixture holds %s", group, row["probe"], row["outcome"], got)
			}
			counts[group]++
			labels[group] = append(labels[group], fmt.Sprintf("%s@%d=%s", row["probe"], huaweiInt(t, row["unit"]), row["outcome"]))
		}
	}
	// Readability is never identity: the gateway answers basic identification at
	// unit 100 and at an inverter unit with its own reply, the SunSpec offset
	// answers non-marker words at both units, and the dongle registers answer an
	// exception at unit 100 that differs from the one at the inverter unit.
	if counts["versus_smartlogger"] != 4 || counts["versus_sdongle"] != 7 || counts["versus_sunspec"] != 2 || counts["versus_inverter_units"] != 3 {
		t.Fatalf("overlap rows: %v", counts)
	}
	for group, want := range map[string][]string{
		"versus_smartlogger": {
			"fc03_65521_q1_counter@0=EXCEPTION_3", "fc03_65521_q1_counter@3=EXCEPTION_3", "fc2b_mei_basic@3=ANSWERED_MEI_BASIC",
			"fc2b_mei_extended_inventory@0=ANSWERED_MEI_EXTENDED",
		},
		"versus_sdongle": {
			"fc2b_mei_basic@100=ANSWERED_MEI_BASIC", "fc03_30068_q2_sdongle_protocol@100=EXCEPTION_4", "fc03_37410_q3_sdongle_search@100=EXCEPTION_4",
			"fc03_37429_q1_sdongle_capacity@100=EXCEPTION_4", "fc03_37410_q3_sdongle_search@3=EXCEPTION_3", "fc03_37429_q1_sdongle_capacity@3=EXCEPTION_3",
			"fc03_30068_q2_sdongle_protocol@3=ANSWERED_WORDS",
		},
		"versus_sunspec":        {"fc03_40000_q2_marker@0=ANSWERED_WORDS", "fc03_40000_q2_marker@3=ANSWERED_WORDS"},
		"versus_inverter_units": {"fc03_30000_q15_emma_offering@3=ANSWERED_TEXT", "fc03_30222_q20_emma_model@3=ANSWERED_WORDS", "fc03_30035_q15_emma_software@3=ANSWERED_TEXT"},
	} {
		if !reflect.DeepEqual(labels[group], want) {
			t.Fatalf("%s rows %v want %v", group, labels[group], want)
		}
	}
	// No row claims a direction that no observation covers.
	for _, group := range []string{"versus_smartlogger", "versus_sdongle", "versus_sunspec", "versus_inverter_units"} {
		for _, value := range h03Array(t, overlap[group]) {
			row := huaweiObject(t, value)
			if unit := huaweiInt(t, row["unit"]); unit == 100 && strings.Contains(row["probe"].(string), "65521") {
				t.Fatalf("the counter was never read at unit 100: %#v", row)
			}
			if unit := huaweiInt(t, row["unit"]); unit != 0 && unit != 3 && unit != 100 {
				t.Fatalf("overlap row at a unit that was not probed: %#v", row)
			}
		}
	}
}

// TestH18OtherHuaweiDispositionsStillRecordTheEMMAOverlapAsOpen pins the
// boundary of this change: the SmartLogger and S-Dongle dispositions and cards
// are not edited here, so they keep listing the EMMA overlap as missing.
func TestH18OtherHuaweiDispositionsStillRecordTheEMMAOverlapAsOpen(t *testing.T) {
	for name, want := range map[string]string{
		"smartlogger-disposition.json": "pairwise_negative_overlap_with_emma_fixture",
		"sdongle-disposition.json":     "pairwise_negative_overlap_with_emma_fixture",
	} {
		record := decodeHuaweiObject(t, "profiles/vendor/huawei/"+name)
		if !containsString(huaweiStringSlice(t, record["reopen_requires"]), want) {
			t.Fatalf("%s no longer lists %s", name, want)
		}
	}
	if !containsString(NewHuaweiSmartLoggerObservedQualificationCard().MissingEvidence(), "pairwise_negative_overlap_with_emma_fixture") ||
		!containsString(NewHuaweiSDongleObservedQualificationCard().MissingEvidence(), "pairwise_negative_overlap_with_emma_fixture") {
		t.Fatal("an unrelated card changed its missing evidence")
	}
}

var (
	h18BuildOrderForward = regexp.MustCompile(`(?i)(older|newer|previous|later|earlier|legacy|new)\b[^.\n]{0,40}\b(firmware|software|version|build|release)`)
	h18BuildOrderReverse = regexp.MustCompile(`(?i)(firmware|software|version|build|release)\b[^.\n]{0,40}\b(older|newer|previous|later|earlier|legacy|new)`)
	h18LongHex           = regexp.MustCompile(`[0-9a-fA-F]{32,}`)
)

// TestH18PublishedTextMakesNoBuildOrderClaimAndHoldsNoDigest scans the text this
// change publishes: no sentence orders builds or releases, and no 32-digit or
// longer hexadecimal run is present.
func TestH18PublishedTextMakesNoBuildOrderClaimAndHoldsNoDigest(t *testing.T) {
	paths := append([]string{
		"docs/huawei-emma-observed-identity.md",
		"testdata/huawei/qualification/emma_observed_replay_expected.json",
	}, h18FixturePaths()...)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if loc := h18BuildOrderForward.FindStringIndex(text); loc != nil {
			t.Fatalf("%s orders builds or releases: %q", path, text[loc[0]:loc[1]])
		}
		if loc := h18BuildOrderReverse.FindStringIndex(text); loc != nil {
			t.Fatalf("%s orders builds or releases: %q", path, text[loc[0]:loc[1]])
		}
		if hit := h18LongHex.FindString(text); hit != "" {
			t.Fatalf("%s holds a digest-shaped run: %q", path, hit)
		}
	}
	// The scan rejects what it is meant to reject.
	for _, text := range []string{
		h03Joined("a ne", "wer firmware"), h03Joined("the pre", "vious release is kept"), h03Joined("software that is ol", "der"),
		h03Joined("build 5 and la", "ter"), h03Joined("a lega", "cy version"),
	} {
		if !h18BuildOrderForward.MatchString(text) && !h18BuildOrderReverse.MatchString(text) {
			t.Errorf("%q was not found", text)
		}
	}
}
