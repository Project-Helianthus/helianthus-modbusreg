package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// referenceEnvironment names the opt-in root of the consulted reference. The
// reference is private, so public runs skip the regeneration test; the
// canonical round-trip, count and invariant tests run everywhere.
const referenceEnvironment = "HELIANTHUS_HUAWEI_REFERENCE"

func TestRegenerationFromTheReferenceIsByteIdenticalAndDeterministic(t *testing.T) {
	root := os.Getenv(referenceEnvironment)
	if root == "" {
		t.Skip(referenceEnvironment + " is not set; the consulted reference is not public")
	}
	first, err := generate(options{Reference: root, Expect: defaultExpect})
	if err != nil {
		t.Fatal(err)
	}
	second, err := generate(options{Reference: root, Expect: defaultExpect})
	if err != nil {
		t.Fatal(err)
	}
	_, raw := loadCommitted(t)
	if len(first.Files) != len(raw) {
		t.Fatalf("%d files generated, %d committed", len(first.Files), len(raw))
	}
	for name, data := range first.Files {
		if !bytes.Equal(data, second.Files[name]) {
			t.Fatalf("%s differs between two runs", name)
		}
		if !bytes.Equal(data, raw[name]) {
			t.Fatalf("%s differs from the committed file", name)
		}
	}
	changed := defaultExpect
	changed.Controls--
	if _, err := generate(options{Reference: root, Expect: changed}); err == nil {
		t.Fatal("a changed control count was accepted without -expect")
	}
}

func TestProseDropsReviewAndAuthorizationWordingOnly(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Read whole fields; no control write is authorized.", "Read whole fields."},
		{"Use raw words. **Requires an engineer sign-off** for protection.", "Use raw words."},
		{"HA Issue 08 p. 11 gives a formula. P. 13 starts PV25.", "HA Issue 08 p. 11 gives a formula. P. 13 starts PV25."},
		{"See [M10](sources.md#m10) p. 79.", "See M10 p. 79."},
		{"HA Issue 08 (SHA `" + strings.Repeat("ab", 10) + "`) p. 11.", "HA Issue 08 p. 11."},
		{"HA Issue 08 (source ID `PV034`) p. 11.", "HA Issue 08 p. 11."},
		{"No write contract approved.", ""},
		{"A shared address does not grant read permission or retry safety.", "A shared address does not grant read permission or retry safety."},
		{"FC03 only; " + "l" + "ater partial silence is not retraction; " + "s" + "oftware confidence is separate.", "FC03 only; " + "s" + "oftware confidence is separate."},
	} {
		if got := prose(tc.in); got != tc.want {
			t.Fatalf("prose(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
	if !ageNearRelease("an "+"o"+"lder "+"f"+"irmware") || ageNearRelease("an "+"o"+"lder table, far away from anything that names the documented branch") {
		t.Fatal("age word distance is not enforced")
	}
}

func TestPublicationGuardRefusesDigestsLocationsAndReviewWording(t *testing.T) {
	for _, tc := range []struct {
		text    string
		isProse bool
	}{
		{strings.Repeat("ab", 6), false},
		{"see " + "https" + "://example.org/doc", false},
		{"the source " + "l" + "edger", false},
		{"file.pdf", false},
		{"requires engineer sign-off", true},
		{"from a " + "n" + "ewer " + "f" + "irmware", false},
		{"bell\a", false},
	} {
		if publicationProblem(tc.text, tc.isProse) == "" {
			t.Fatalf("%q was accepted", tc.text)
		}
	}
	if publicationProblem("license.status_time.authorizationfunction", false) != "" {
		t.Fatal("an identifier was refused for wording")
	}
}

const syntheticBaseReview = `{
  "schema": "huawei-pv-base-profile-semantic-review-v1",
  "conditions": {"GATE_A": "Use exact rows.", "GATE_B": "Read whole fields; no control write is authorized."},
  "alarm_bits": [{"register_or_word_literal": "Alarm9", "register_address": null, "bit": 3, "alarm_id": 2001, "meaning_key": "alarm.example", "severity_key": "major", "conditional_source_markers": ["1"], "source": {"physical_page": 40}}],
  "entries": [
    {"capability_class": "pv.channel", "functional_key": "pv.channel.one", "endpoint": "inverter", "status": "DOCUMENT_REVIEWED", "condition_ids": ["GATE_A"], "meaning": {"enum": {"1": "on", "0": "off"}}, "unresolved": [],
     "source_tuple_literal": {"access": "RO", "type": "I16", "unit": "V", "gain": "10", "address": "32016", "words": "1"}, "evidence_id": "PVX:p9:t0:r1", "function_code": 3, "source_id": "PVX", "physical_page": 9},
    {"capability_class": "pv.channel", "functional_key": "pv.channel.pvn", "endpoint": "inverter", "status": "RAW_ONLY_UNDEFINED", "condition_ids": ["GATE_A", "GATE_B"], "meaning": {}, "unresolved": ["Formula address."],
     "source_tuple_literal": {"access": "RO", "type": "I16", "unit": "A", "gain": "100", "address": "...", "words": "1"}, "address_contract": {"value": null, "status": "unspecified_in_source"}, "evidence_id": "PVX:p9:t0:r2", "function_code": 3, "source_id": "PVX", "physical_page": 9},
    {"capability_class": "emma.alarm", "functional_key": "emma.alarm.count", "endpoint": "EMMA", "status": "CONFLICT", "condition_ids": ["GATE_B"], "meaning": {"sign": {"positive": "export_to_grid", "negative": "import_from_grid"}}, "unresolved": [],
     "source_tuple_literal": {"access": "RO", "type": "U16", "unit": "N/A", "gain": "", "address": "30906", "words": "1"}, "evidence_id": "PVX:p65:t1:r7", "function_code": 3, "source_id": "PVX", "physical_page": 65}
  ],
  "coverage": {"detected_RO_rows": 3, "status_counts": {"DOCUMENT_REVIEWED": 1, "RAW_ONLY_UNDEFINED": 1, "CONFLICT": 1}}
}`

func TestBaseTwinParserKeepsClassesGatesAndUnspecifiedAddresses(t *testing.T) {
	var out cardRows
	if err := parseReadOnly(json.RawMessage(syntheticBaseReview), "PVX", &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Gates) != 2 || out.Gates[1].Meaning != "Read whole fields." || len(out.Rows) != 3 || len(out.AlarmBits) != 1 {
		t.Fatalf("gates=%+v rows=%d bits=%d", out.Gates, len(out.Rows), len(out.AlarmBits))
	}
	reviewed, unspecified, emma := out.Rows[0].Row, out.Rows[1].Row, out.Rows[2].Row
	if reviewed.Class != classReviewed || reviewed.Decode == nil || reviewed.Decode.Gain != "10" || reviewed.Literal != nil || *reviewed.Offset != 32016 {
		t.Fatalf("reviewed row %+v", reviewed)
	}
	if unspecified.Offset != nil || unspecified.AddressStatus != addressUnspecified || unspecified.Class != classRawOnly || unspecified.Decode != nil || unspecified.Condition != "Formula address." {
		t.Fatalf("unspecified row %+v", unspecified)
	}
	if emma.Endpoint != endpointEMMA || emma.Class != classConflict || emma.Literal.Gain != floorNotStated {
		t.Fatalf("EMMA row %+v", emma)
	}
	if bit := out.AlarmBits[0]; bit.Offset != nil || bit.Word != "Alarm9" || bit.Module != "1" {
		t.Fatalf("alarm bit %+v", bit)
	}
	dictionaries, err := meaningDictionaries(out.Rows[0])
	if err != nil || len(dictionaries) != 1 || dictionaries[0].Values[0].Code != "0" {
		t.Fatalf("dictionaries %+v %v", dictionaries, err)
	}
	for _, mutation := range []struct{ from, to string }{
		{`"status": "DOCUMENT_REVIEWED"`, `"status": "SOMETHING_ELSE"`},
		{`"endpoint": "inverter", "status": "DOCUMENT_REVIEWED"`, `"endpoint": "gateway", "status": "DOCUMENT_REVIEWED"`},
		{`"detected_RO_rows": 3`, `"detected_RO_rows": 4`},
		{`"source_id": "PVX", "physical_page": 9},`, `"source_id": "PVY", "physical_page": 9},`},
		{`"unspecified_in_source"`, `"guessed"`},
	} {
		var refused cardRows
		if parseReadOnly(json.RawMessage(strings.Replace(syntheticBaseReview, mutation.from, mutation.to, 1)), "PVX", &refused) == nil {
			t.Fatalf("mutation %q was accepted", mutation.to)
		}
	}
}

const syntheticControls = `{
  "schema": "huawei-pv-base-control-review-v1",
  "entries": [
    {"entry_id": "PVX:p28:t0:r2", "namespace": "inverter_and_associated_channels", "class_id": "time", "metadata_status": "DOCUMENTED_CONTROL_METADATA",
     "tuple": {"address": 40000, "register_length": 2, "source_type": "EPOCHTIME", "access": "RW", "gain": 1, "unit": "s"}, "provenance": {"source_id": "PVX", "physical_page": 28}, "read_function_association": {"function_code": 3}},
    {"entry_id": "PVX:p28:t0:r3", "namespace": "inverter_and_associated_channels", "class_id": "lifecycle", "metadata_status": "DOCUMENTED_CONTROL_METADATA",
     "tuple": {"address": 40200, "register_length": 1, "source_type": "U16", "access": "WO", "gain": null, "unit": null}, "provenance": {"source_id": "PVX", "physical_page": 28}, "read_function_association": {"function_code": null}},
    {"entry_id": "PVX:p29:t0:r1", "namespace": "emma_unit0", "class_id": "storage", "metadata_status": "QUARANTINED_SPECIFIC_CONFLICT",
     "tuple": {"address": 47000, "register_length": 1, "source_type": "U16", "access": "RW", "gain": 1, "unit": "N/A"}, "provenance": {"source_id": "PVX", "physical_page": 29}, "read_function_association": {"function_code": 3},
     "value_facts": {"enum_members": [{"code_literal": "0", "source_label_compacted": "None"}, {"code_literal": "2", "source_label_compacted": "LUNA2000"}]}}
  ],
  "coverage": {"recognized_control_occurrences": 3, "status_counts": {"DOCUMENTED_CONTROL_METADATA": 2, "QUARANTINED_SPECIFIC_CONFLICT": 1}}
}`

func TestControlParserKeepsControlsReadableOrNoSendAndNeverDecoded(t *testing.T) {
	var out cardRows
	if err := parseControls(json.RawMessage(syntheticControls), "x", "PVX", &out); err != nil {
		t.Fatal(err)
	}
	want := []struct{ key, class, endpoint string }{
		{"x.control.time.40000", classReadableConfiguration, endpointInverter},
		{"x.control.lifecycle.40200", classNoSend, endpointInverter},
		{"x.control.storage.47000", classNoSend, endpointEMMA},
	}
	for index, row := range out.Controls {
		if row.Row.Key != want[index].key || row.Row.Class != want[index].class || row.Row.Endpoint != want[index].endpoint || row.Row.Decode != nil || row.Row.Literal == nil {
			t.Fatalf("control %d: %+v", index, row.Row)
		}
		if err := checkRegisterRow(row.Row, true); err != nil {
			t.Fatalf("control %d: %v", index, err)
		}
	}
	if item := selectorDictionary(out.Controls[2]); item == nil || len(item.Values) != 2 || item.Values[1].Meaning != "LUNA2000" {
		t.Fatalf("selector %+v", item)
	}
	var refused cardRows
	if parseControls(json.RawMessage(strings.Replace(syntheticControls, `"QUARANTINED_SPECIFIC_CONFLICT",`, `"INVENTED_STATUS",`, 1)), "x", "PVX", &refused) == nil {
		t.Fatal("an unknown control status was accepted")
	}
}

func TestModelTableConversionKeepsRowAndDocumentFloorsApart(t *testing.T) {
	table := modelTable{
		Schema: "huawei-pv-model-table-v1", SourceID: "PVX",
		DocumentRequirements: []twinRequirement{{ID: "PVX:section1.2", SourceID: "PVX", Page: 7, Section: "1.2 System Requirements", Minimum: "V100R001C00", Inclusive: true}},
	}
	five, seven := 5, 7
	stated, merged, psc := "SUN2000MC V200R023C00", "SUN2000MA V100R001C10SUN2000MA", "V200R023C10PSC110"
	table.DocumentRequirements[0].Scope.ModelTokens = []string{"SUN2000-A", "SUN2000-B"}
	table.Rows = []twinModel{
		{Token: "SUN2000-50KTL-M3", ModelID: &five, Minimum: &stated, MinimumStatus: "stated_in_source", ModelIDStatus: "stated_as_model_id", SourceID: "PVX", Page: 5},
		{Token: "SUN2000-A", MinimumStatus: "not_stated_in_model_row", ModelIDStatus: "not_stated_as_model_id", Sequence: "33", SourceID: "PVX", Page: 7, RequirementIDs: []string{"PVX:section1.2"}},
		{Token: "SUN2000-20KTL-M0", ModelID: &seven, Minimum: &merged, MinimumStatus: "stated_in_source", ModelIDStatus: "stated_as_model_id", SourceID: "PVX", Page: 8},
		{Token: "SUN2000-12K-MB0", ModelID: &seven, Minimum: &psc, MinimumStatus: "stated_in_source", ModelIDStatus: "stated_as_model_id", SourceID: "PVX", Page: 6},
	}
	models, requirements, err := convertModelTable(table, "e")
	if err != nil {
		t.Fatal(err)
	}
	if len(requirements) != 1 || requirements[0].ID != "e/1-2-system-requirements" {
		t.Fatalf("requirements %+v", requirements)
	}
	row, document, irregular, mirrored := models[0].Row, models[1].Row, models[2].Row, models[3].Row
	if row.FloorBasis != floorBasisRow || row.MinimumSoftware != stated || row.RowMinimum != stated || row.Identifier == nil || len(row.Quarantine) != 0 {
		t.Fatalf("row floor %+v", row)
	}
	if document.FloorBasis != floorBasisDocument || document.RowMinimum != floorNotStated || document.MinimumSoftware != "V100R001C00" ||
		document.Identifier != nil || document.SequenceNumber != "33" || strings.Join(document.Quarantine, ",") != quarantineSequenceOrdinal {
		t.Fatalf("document floor %+v", document)
	}
	if strings.Join(irregular.Quarantine, ",") != quarantineFloorCell || len(mirrored.Quarantine) != 0 {
		t.Fatalf("irregular %+v mirrored %+v", irregular, mirrored)
	}
	broken := table
	broken.Rows = append([]twinModel(nil), table.Rows...)
	broken.Rows[1].RequirementIDs = nil
	if _, _, err := convertModelTable(broken, "e"); err == nil {
		t.Fatal("a row without a minimum and without a document requirement was accepted")
	}
	broken.Rows = append([]twinModel(nil), table.Rows...)
	broken.Rows[1].Sequence = ""
	if _, _, err := convertModelTable(broken, "e"); err == nil {
		t.Fatal("a row without a model identifier and without a sequence number was accepted")
	}
}

func TestClassInvariantRefusesDecodingBeyondTheClass(t *testing.T) {
	offset := 32016
	base := registerRow{Key: "k", Endpoint: endpointInverter, Offset: &offset, Quantity: 1, Type: "I16", Access: "RO", Page: 1}
	for _, tc := range []struct {
		class           string
		decode, literal bool
		accept          bool
	}{
		{classReviewed, true, false, true},
		{classConditional, true, false, true},
		{classRawOnly, false, true, true},
		{classConflict, false, true, true},
		{classReviewed, false, true, false},
		{classConditional, true, true, false},
		{classRawOnly, true, false, false},
		{classConflict, true, true, false},
		{classConflict, false, false, false},
		{classNoSend, false, true, false},
		{"unknown", true, false, false},
	} {
		row := base
		row.Class = tc.class
		if tc.decode {
			row.Decode = &scale{Gain: "10", Unit: "V"}
		}
		if tc.literal {
			row.Literal = &scale{Gain: "10", Unit: "V"}
		}
		if err := checkRegisterRow(row, false); (err == nil) != tc.accept {
			t.Fatalf("%+v: err=%v accept=%t", tc, err, tc.accept)
		}
	}
	control := base
	control.Access, control.Class, control.Literal = "RW", classReadableConfiguration, &scale{Gain: "1", Unit: "s"}
	if err := checkRegisterRow(control, true); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*registerRow){
		"decoding control":   func(r *registerRow) { r.Decode = &scale{Gain: "1", Unit: "s"} },
		"readable WO":        func(r *registerRow) { r.Access = "WO" },
		"read-only class":    func(r *registerRow) { r.Class = classReviewed },
		"control without at": func(r *registerRow) { r.Offset = nil; r.AddressStatus = addressUnspecified },
	} {
		row := control
		mutate(&row)
		if checkRegisterRow(row, true) == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	unspecified := base
	unspecified.Class, unspecified.Literal, unspecified.Offset = classRawOnly, &scale{Gain: "1", Unit: "A"}, nil
	if checkRegisterRow(unspecified, false) == nil {
		t.Fatal("a row without an offset and without the unspecified address status was accepted")
	}
	unspecified.AddressStatus = addressUnspecified
	if err := checkRegisterRow(unspecified, false); err != nil {
		t.Fatal(err)
	}
}

func TestFloorFormRefusesMissingFloorsRangesAndPromotedOrdinals(t *testing.T) {
	identifier := 471
	base := modelRow{Edition: "e", Page: 5, Token: "SUN2000-50KTL-M3", Identifier: &identifier,
		RowMinimum: "SUN2000MC V200R023C00", MinimumSoftware: "SUN2000MC V200R023C00", FloorBasis: floorBasisRow}
	if err := checkModelRow(base); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*modelRow){
		"no floor":               func(r *modelRow) { r.MinimumSoftware = "" },
		"floor not stated":       func(r *modelRow) { r.MinimumSoftware = floorNotStated },
		"floor differs from row": func(r *modelRow) { r.MinimumSoftware = "SUN2000MC V200R024C00" },
		"truncated floor":        func(r *modelRow) { r.RowMinimum, r.MinimumSoftware = "V300R024C10SPC1", "V300R024C10SPC1" },
		"range floor": func(r *modelRow) {
			r.RowMinimum, r.MinimumSoftware = "V200R023C00 to V200R024C00", "V200R023C00 to V200R024C00"
		},
		"document without link": func(r *modelRow) { r.FloorBasis, r.RowMinimum = floorBasisDocument, floorNotStated },
		"ordinal as identifier": func(r *modelRow) { r.SequenceNumber = "33" },
		"identifier missing":    func(r *modelRow) { r.Identifier = nil },
		"unknown basis":         func(r *modelRow) { r.FloorBasis = "maximum" },
		"invented wire alias":   func(r *modelRow) { r.WireAliases = []wireAlias{{Token: "SUN3000-50KTL-M3", Evidence: "e", Scope: "s"}} },
	}
	for name, mutate := range mutations {
		row := base
		mutate(&row)
		if checkModelRow(row) == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	document := base
	document.Identifier, document.SequenceNumber, document.Quarantine = nil, "33", []string{quarantineSequenceOrdinal}
	document.FloorBasis, document.RowMinimum, document.MinimumSoftware, document.DocumentRequirement = floorBasisDocument, floorNotStated, "V100R001C00", "e/1-2"
	if err := checkModelRow(document); err != nil {
		t.Fatal(err)
	}
	alias := base
	alias.Token, alias.WireAliases = "SUN2000_12K_MAP0", []wireAlias{{Token: "SUN2000-12K-MAP0", Evidence: "reported", Scope: "exact identifier scope"}}
	if err := checkModelRow(alias); err != nil {
		t.Fatal(err)
	}
}

// TestModelUniquenessCoversEveryIdentityField duplicates one committed model
// row and then varies each identity field: the edition (a mutation that drops
// the edition from the key would refuse the edition variant), the token
// (dropping the token would accept the duplicate token change as a new pair
// and miss the identifier clash) and the identifier (dropping the identifier
// would refuse the identifier variant).
func TestModelUniquenessCoversEveryIdentityField(t *testing.T) {
	c, raw := loadCommitted(t)
	if err := validateCatalogue(c); err != nil {
		t.Fatal(err)
	}
	var source modelRow
	found := false
	for _, row := range c.Models.Rows {
		if row.Identifier != nil && len(row.Quarantine) == 0 && len(row.WireAliases) == 0 && row.FloorBasis == floorBasisRow {
			source, found = row, true
			break
		}
	}
	if !found {
		t.Fatal("no plain model row")
	}
	used := map[string]bool{}
	for _, row := range c.Models.Rows {
		if row.Token == source.Token || (row.Identifier != nil && *row.Identifier == *source.Identifier) {
			used[row.Edition] = true
		}
	}
	otherEdition := ""
	for _, record := range c.Editions.Editions {
		if !used[record.ID] {
			otherEdition = record.ID
			break
		}
	}
	if otherEdition == "" {
		t.Fatal("no edition free of the source token and identifier")
	}
	unused := 65000
	variants := []struct {
		name   string
		mutate func(*modelRow)
		accept bool
	}{
		{"exact duplicate", func(*modelRow) {}, false},
		{"other edition", func(r *modelRow) { r.Edition = otherEdition }, true},
		{"other token, same identifier", func(r *modelRow) { r.Token += "-X" }, false},
		{"other token and identifier", func(r *modelRow) { r.Token += "-X"; r.Identifier = &unused }, true},
		{"other identifier", func(r *modelRow) { r.Identifier = &unused }, true},
	}
	for _, variant := range variants {
		copyOf := *c
		if err := decodeStrict(raw[modelsFile], &copyOf.Models); err != nil {
			t.Fatal(err)
		}
		if err := decodeStrict(raw[editionsFile], &copyOf.Editions); err != nil {
			t.Fatal(err)
		}
		row := source
		identifier := *source.Identifier
		row.Identifier = &identifier
		variant.mutate(&row)
		copyOf.Models.Rows = append(copyOf.Models.Rows, row)
		target := row.Edition
		for index := range copyOf.Editions.Editions {
			if copyOf.Editions.Editions[index].ID == target {
				copyOf.Editions.Editions[index].ModelRows++
			}
		}
		if err := validateCatalogue(&copyOf); (err == nil) != variant.accept {
			t.Fatalf("%s: err=%v accept=%t", variant.name, err, variant.accept)
		}
	}
}

func TestQuarantineFlagsAndAffectedRowsMustAgree(t *testing.T) {
	c, _ := loadCommitted(t)
	for name, doc := range c.Registers {
		for index := range doc.Rows {
			if len(doc.Rows[index].Quarantine) == 0 && doc.Rows[index].Offset != nil {
				doc.Rows[index].Quarantine = []string{"Q02"}
				if validateCatalogue(c) == nil {
					t.Fatalf("%s: a flag without an affected entry was accepted", name)
				}
				doc.Rows[index].Quarantine = []string{"Z99"}
				if validateCatalogue(c) == nil {
					t.Fatalf("%s: an unknown quarantine was accepted", name)
				}
				doc.Rows[index].Quarantine = nil
				if err := validateCatalogue(c); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
	}
	t.Fatal("no unflagged row")
}

func TestCanonicalEncodingIsStableAndStrict(t *testing.T) {
	doc := &quarantinesDoc{Schema: quarantinesSchema, Quarantines: []quarantine{{ID: "Q", Origin: originReference, Subject: "s", Meaning: "a < b & c"}}}
	encoded, err := encodeDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"meaning":"a < b & c","editions":null`) {
		t.Fatalf("unexpected encoding %s", encoded)
	}
	var decoded quarantinesDoc
	if err := decodeStrict(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	again, err := encodeDocument(&decoded)
	if err != nil || !bytes.Equal(again, encoded) {
		t.Fatalf("re-encoding differs: %v", err)
	}
	if decodeStrict([]byte(`{"schema":"x","maximum":"V1"}`), &decoded) == nil {
		t.Fatal("an unknown field was accepted")
	}
	if decodeStrict(append(append([]byte(nil), encoded...), []byte("{}")...), &decoded) == nil {
		t.Fatal("trailing data was accepted")
	}
}

func TestSidecarRefusedInsideRepositoryWithExternalOut(t *testing.T) {
	repository := t.TempDir()
	if err := os.WriteFile(filepath.Join(repository, "go.mod"), []byte("module example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := checkSidecar(out, filepath.Join(repository, "private-sidecar")); err == nil {
		t.Fatal("a sidecar inside a repository was accepted with an external -out")
	}
	if err := checkSidecar(out, filepath.Join(out, "sidecar")); err == nil {
		t.Fatal("a sidecar inside -out was accepted")
	}
	if err := checkSidecar(out, filepath.Join(t.TempDir(), "sidecar")); err != nil {
		t.Fatalf("an outside sidecar was refused: %v", err)
	}
}

func TestExpectOverrideAndSidecarPlacement(t *testing.T) {
	if got, err := parseExpect(""); err != nil || got != defaultExpect {
		t.Fatalf("default expect %v %v", got, err)
	}
	if got, err := parseExpect("ro=3120,controls=2200"); err != nil || got.RO != 3120 || got.Controls != 2200 || got.Core != 429 || got.Models != 2401 || got.Cards != 16 {
		t.Fatalf("override %v %v", got, err)
	}
	for _, bad := range []string{"ro=-1", "rows=1", "ro"} {
		if _, err := parseExpect(bad); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
	root := t.TempDir()
	for path, want := range map[string]bool{
		root:                                      true,
		filepath.Join(root, "sidecar"):            true,
		filepath.Join(root, "..", "sidecar"):      false,
		filepath.Join(root+"-other", "x"):         false,
		filepath.Join(root, "a", "..", "..", "b"): false,
	} {
		if got, err := insideDirectory(root, path); err != nil || got != want {
			t.Fatalf("insideDirectory(%q)=%t,%v want %t", path, got, err, want)
		}
	}
}

func TestInsideDirectoryResolvesSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(outside, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if got, err := insideDirectory(root, filepath.Join(link, "sidecar")); err != nil || !got {
		t.Fatalf("a link into the checkout was treated as outside: %t,%v", got, err)
	}
}

func TestTruncatedSentinelCodesAreDroppedOnlyWithTailEvidence(t *testing.T) {
	values := []dictValue{
		{Code: "0XF2A5", Meaning: "2380_2099_permanent"}, {Code: "0XFFFF", Meaning: "ffff_isreported"},
		{Code: "0xff", Meaning: "unset_package"}, {Code: "3", Meaning: "d"},
	}
	raw := json.RawMessage(`[{"value":"0xF2A52380"},{"value":"0xFFFFFFFF"},{"value":"0xFFFF"}]`)
	got, err := withoutTruncatedSentinels(values, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Code != "0xff" || got[1].Code != "3" {
		t.Fatalf("a distinct code was dropped or a cut code kept: %v", got)
	}
	truncated, suspected, err := truncatedSentinelCodes(values, raw)
	if err != nil || len(truncated) != 2 || len(suspected) != 1 || suspected[0].Code != "0xff" {
		t.Fatalf("truncated %v suspected %v err %v", truncated, suspected, err)
	}
}

func TestRowDefectsQuarantineTruncatedAndMergedEnumerations(t *testing.T) {
	offset := 30800
	row := parsedRow{Row: registerRow{Offset: &offset}, Meaning: map[string]json.RawMessage{
		"enum": json.RawMessage(`{"0":"thesearchiscomplete1_beingsearched2_thesearchfails"}`),
	}}
	defects, err := rowDefects(row)
	if err != nil || len(defects) != 1 || defects[0] != quarantineMergedEnum {
		t.Fatalf("merged enumeration: %v %v", defects, err)
	}
	items, err := meaningDictionaries(row)
	if err != nil || len(items) != 0 {
		t.Fatalf("a merged enumeration was published: %+v %v", items, err)
	}
	row.Meaning = map[string]json.RawMessage{
		"enum":      json.RawMessage(`{"0xff":"unset","1":"on"}`),
		"sentinels": json.RawMessage(`[{"value":"0xFFFF","meaning":"na"}]`),
	}
	defects, err = rowDefects(row)
	if err != nil || len(defects) != 1 || defects[0] != quarantineTruncatedCode {
		t.Fatalf("suspected truncation: %v %v", defects, err)
	}
	items, err = meaningDictionaries(row)
	if err != nil || len(items) != 2 || len(items[0].Values) != 2 {
		t.Fatalf("a distinct code was deleted: %+v %v", items, err)
	}
}

func TestRuleDictionariesReadConditionTextOfFullReviewRowsOnly(t *testing.T) {
	offset := 35139
	row := parsedRow{Row: registerRow{Endpoint: endpointInverter, Offset: &offset, Page: 64, Class: classConditional,
		Condition: "0xF2A52380 is the source permanent-license marker;0 or0xFFFFFFFF means no license. Do not convert these markers."}}
	items := ruleDictionaries(row)
	if len(items) != 1 || items[0].Kind != kindSentinels || len(items[0].Values) != 3 || items[0].Values[2].Code != "0xFFFFFFFF" || items[0].Values[1].Meaning != "no license" {
		t.Fatalf("sentinels %+v", items)
	}
	row.Row.Condition = "Positive is grid export; negative is grid import for this exact meter row."
	if items := ruleDictionaries(row); len(items) != 1 || items[0].Kind != kindSign || items[0].Values[1].Meaning != "grid import" {
		t.Fatalf("sign %+v", items)
	}
	row.Row.Class = classRawOnly
	if items := ruleDictionaries(row); len(items) != 0 {
		t.Fatalf("a raw-only row got %+v", items)
	}
	row.Row.Class = classConditional
	row.Meaning = map[string]json.RawMessage{"sign": json.RawMessage(`{}`)}
	if items := ruleDictionaries(row); len(items) != 0 {
		t.Fatalf("a row with structured meaning got %+v", items)
	}
}
