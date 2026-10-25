package main

import (
	"bytes"
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
	locator := filepath.Join(root, "..", "audit", "all-model-row-locators.json")
	first, err := generate(options{Reference: root, ModelRows: locator, Expect: defaultExpect})
	if err != nil {
		t.Fatal(err)
	}
	second, err := generate(options{Reference: root, ModelRows: locator, Expect: defaultExpect})
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
	if _, err := generate(options{Reference: root, ModelRows: locator, Expect: counts{RO: 3114, Core: 429, Models: 2113, Cards: 16}}); err == nil {
		t.Fatal("a changed read-only count was accepted without -expect")
	}
}

func TestProseDropsReviewAndAuthorizationWordingOnly(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Read whole fields; no control write is authorized.", "Read whole fields."},
		{"Use raw words. **Requires an engineer sign-off** for protection.", "Use raw words."},
		{"HA Issue 08 p. 11 gives a formula. P. 13 starts PV25.", "HA Issue 08 p. 11 gives a formula. P. 13 starts PV25."},
		{"See [M10](sources.md#m10) p. 79.", "See M10 p. 79."},
		{"HA Issue 08 (SHA `" + strings.Repeat("ab", 10) + "`) p. 11.", "HA Issue 08 p. 11."},
		{"No write contract approved.", ""},
		{"A shared address does not grant read permission or retry safety.", "A shared address does not grant read permission or retry safety."},
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

var syntheticClassTables = "Exact [X Issue 01](../appendices/sources.md#s-" + strings.Repeat("a", 12) + ").\n\n" +
	"## Conditions\n\n" +
	"- `GATE_A`: Use exact rows.\n" +
	"- `GATE_B`: Read whole fields; no control write is authorized.\n\n" +
	"## pv.channel\n\n" +
	"| Functional key | Endpoint | Address / words | Literal type; gain; unit | Status / conditions | Source page / table / row |\n" +
	"|---|---|---|---|---|---|\n" +
	"| pv.channel.one | inverter | 32016 / 1 | I16; 10; V | DOCUMENT_REVIEWED; GATE_A | p. 9 / t0 / r1 |\n" +
	"| pv.channel.pvn | inverter | ... / 1 | I16; 100; A | RAW_ONLY_UNDEFINED; GATE_A, GATE_B | p. 9 / t0 / r2 |\n" +
	"| emma.alarm.count | EMMA | 30906 / 1 | U16; ; N/A | CONFLICT; GATE_B | p. 65 / t1 / r7 |\n"

func TestClassTableParserPlacesEveryRowWithItsClassAndGates(t *testing.T) {
	rows, gates, anchor, err := parseClassTables(syntheticClassTables, "sha")
	if err != nil {
		t.Fatal(err)
	}
	if anchor != "s-"+strings.Repeat("a", 12) || len(gates) != 2 || gates[1].Meaning != "Read whole fields." || len(rows) != 3 {
		t.Fatalf("anchor=%q gates=%+v rows=%d", anchor, gates, len(rows))
	}
	reviewed, template, emma := rows[0].Row, rows[1].Row, rows[2].Row
	if reviewed.Class != classReviewed || reviewed.Decode == nil || reviewed.Decode.Gain != "10" || reviewed.Literal != nil || *reviewed.Offset != 32016 || rows[0].Evidence != "sha:p9:t0:r1" {
		t.Fatalf("reviewed row %+v", reviewed)
	}
	if template.Offset != nil || template.Class != classRawOnly || template.Decode != nil || template.Literal.Unit != "A" || len(template.Gates) != 2 {
		t.Fatalf("template row %+v", template)
	}
	if emma.Endpoint != endpointEMMA || emma.Class != classConflict || emma.Literal.Gain != floorNotStated || emma.FunctionalClass != "pv.channel" {
		t.Fatalf("EMMA row %+v", emma)
	}
	if _, _, _, err := parseClassTables(strings.Replace(syntheticClassTables, "DOCUMENT_REVIEWED;", "SOMETHING_ELSE;", 1), "sha"); err == nil {
		t.Fatal("an unknown status was accepted")
	}
	if _, _, _, err := parseClassTables(strings.Replace(syntheticClassTables, "| inverter | 32016", "| gateway | 32016", 1), "sha"); err == nil {
		t.Fatal("an unknown endpoint was accepted")
	}
}

func TestModelLocatorFormsAreClassifiedOrRefused(t *testing.T) {
	data := []byte(`[
		{"pdf_sha256":"a","physical_page":5,"native_model_row":"SUN2000-50KTL-M3        471        SUN2000MC V200R023C00"},
		{"pdf_sha256":"a","physical_page":6,"native_model_row":"SUN2000-8KTL       5      SUN2000       8.8     5.28"},
		{"pdf_sha256":"a","physical_page":7,"native_model_row":"33        SUN2000-60KTL-HV-D1        66      39.6      60"},
		{"pdf_sha256":"a","physical_page":7,"native_model_row":"338         SUN2000-2KTL-L0"},
		{"pdf_sha256":"a","physical_page":7,"native_model_row":"SUN2000-55KTL-HV-D1"},
		{"pdf_sha256":"a","physical_page":3,"native_model_row":"05        2022-06-16        Add new model: X,"},
		{"pdf_sha256":"a","physical_page":3,"native_model_row":"SUN2000-125KTL-JPH1"}
	]`)
	models, err := parseLocatorRows(data)
	if err != nil {
		t.Fatal(err)
	}
	flagModels(models)
	want := []struct {
		token, floor string
		identifier   int
		flags        string
	}{
		{"SUN2000-50KTL-M3", "SUN2000MC V200R023C00", 471, ""},
		{"SUN2000-8KTL", "SUN2000", 5, "M02"},
		{"SUN2000-60KTL-HV-D1", floorNotStated, 33, ""},
		{"SUN2000-2KTL-L0", floorNotStated, 338, ""},
		{"SUN2000-55KTL-HV-D1", floorNotStated, -1, "M04"},
		{"", floorNotStated, -1, "M03"},
		{"", floorNotStated, -1, "M03"},
	}
	for index, row := range models {
		identifier := -1
		if row.Row.Identifier != nil {
			identifier = *row.Row.Identifier
		}
		if row.Row.Token != want[index].token || row.Row.MinimumSoftware != want[index].floor || identifier != want[index].identifier || strings.Join(row.Row.Quarantine, ",") != want[index].flags {
			t.Fatalf("row %d: %+v", index, row.Row)
		}
	}
	if _, err := parseLocatorRows([]byte(`[{"pdf_sha256":"a","physical_page":9,"native_model_row":"something else entirely"}]`)); err == nil {
		t.Fatal("an unplaced locator line was accepted")
	}
}

func TestReviewedStatementsCompleteWrappedFloorsAndRefuseContradictions(t *testing.T) {
	identifier := 380
	models := []parsedModel{{
		Row:    modelRow{Token: "SUN2000-4.95K-LB0-NH", Identifier: &identifier, Page: 7, MinimumSoftware: "V300R024C10SPC1", FloorBasis: floorBasisRow},
		SHA256: "lb", Private: map[string]string{},
	}}
	if _, err := completeFloors(models, map[string][]cardTuple{"lb": {{Token: "SUN2000-4.95K-LB0-NH", Identifier: 380, Floor: "V300R024C10SPC100"}}}); err != nil {
		t.Fatal(err)
	}
	if models[0].Row.MinimumSoftware != "V300R024C10SPC100" || models[0].Row.FloorBasis != floorBasisReviewed {
		t.Fatalf("floor not completed: %+v", models[0].Row)
	}
	models[0].Row.MinimumSoftware, models[0].Row.FloorBasis = "V300R024C00", floorBasisRow
	if _, err := completeFloors(models, map[string][]cardTuple{"lb": {{Token: "SUN2000-4.95K-LB0-NH", Identifier: 380, Floor: "V300R024C10SPC100"}}}); err == nil {
		t.Fatal("a contradicting complete floor was accepted")
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
		if err := checkRegisterRow(row, "RO"); (err == nil) != tc.accept {
			t.Fatalf("%+v: err=%v accept=%t", tc, err, tc.accept)
		}
	}
	template := base
	template.Class, template.Literal, template.Offset = classRawOnly, &scale{Gain: "1", Unit: "A"}, nil
	if checkRegisterRow(template, "RO") == nil {
		t.Fatal("a row without an offset and without X01 was accepted")
	}
	template.Quarantine = []string{quarantineTemplateRow}
	if err := checkRegisterRow(template, "RO"); err != nil {
		t.Fatal(err)
	}
}

func TestFloorFormRefusesUnflaggedCellsRangesAndHistoryTokens(t *testing.T) {
	identifier := 471
	base := modelRow{Edition: "e", Page: 5, Form: formSupportedModel, Token: "SUN2000-50KTL-M3", Identifier: &identifier,
		MinimumSoftware: "SUN2000MC V200R023C00", FloorBasis: floorBasisRow, SourceSet: sourceSetLocator}
	if err := checkModelRow(base); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*modelRow){
		"empty floor":             func(r *modelRow) { r.MinimumSoftware = "" },
		"truncated floor":         func(r *modelRow) { r.MinimumSoftware = "V300R024C10SPC1" },
		"range floor":             func(r *modelRow) { r.MinimumSoftware = "V200R023C00 to V200R024C00" },
		"bounded floor":           func(r *modelRow) { r.MinimumSoftware = "<V200R024C00" },
		"basis without floor":     func(r *modelRow) { r.MinimumSoftware = floorNotStated },
		"history with token":      func(r *modelRow) { r.Form = formRevisionHistory; r.Quarantine = []string{quarantineHistoryLine} },
		"missing identifier flag": func(r *modelRow) { r.Identifier = nil },
		"invented wire token":     func(r *modelRow) { r.WireToken = "SUN2000-50KTL-M3"; r.WireTokenGrade = gradeHypothesis },
		"unknown form":            func(r *modelRow) { r.Form = "maximum" },
	}
	for name, mutate := range mutations {
		row := base
		mutate(&row)
		if checkModelRow(row) == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	flagged := base
	flagged.MinimumSoftware, flagged.Quarantine = "V300R024C10SPC1", []string{quarantineFloorCell}
	if err := checkModelRow(flagged); err != nil {
		t.Fatal(err)
	}
	mapZero := base
	mapZero.Token, mapZero.WireToken, mapZero.WireTokenGrade = "SUN2000_8K_MAP0_ZH", "SUN2000-8K-MAP0-ZH", gradeHypothesis
	if err := checkModelRow(mapZero); err != nil {
		t.Fatal(err)
	}
	mapZero.WireToken = ""
	mapZero.WireTokenGrade = ""
	if checkModelRow(mapZero) == nil {
		t.Fatal("a MAP0 underscore token without its wire form was accepted")
	}
}

// TestModelUniquenessCoversEveryIdentityField duplicates one committed model
// row and then varies each identity field: the edition (a mutation that drops
// the edition from the key would accept the duplicate after the edition
// change), the token (dropping the token would refuse the token variant as an
// identifier clash) and the identifier (dropping the identifier would refuse
// the identifier variant).
func TestModelUniquenessCoversEveryIdentityField(t *testing.T) {
	c, raw := loadCommitted(t)
	if err := validateCatalogue(c); err != nil {
		t.Fatal(err)
	}
	var source modelRow
	found := false
	for _, row := range c.Models.Rows {
		if row.Form == formSupportedModel && row.Identifier != nil && len(row.Quarantine) == 0 && row.WireToken == "" {
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
		row := source
		identifier := *source.Identifier
		row.Identifier = &identifier
		variant.mutate(&row)
		copyOf.Models.Rows = append(copyOf.Models.Rows, row)
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

func TestExpectOverrideAndSidecarPlacement(t *testing.T) {
	if got, err := parseExpect(""); err != nil || got != defaultExpect {
		t.Fatalf("default expect %v %v", got, err)
	}
	if got, err := parseExpect("ro=3120,models=2200"); err != nil || got.RO != 3120 || got.Models != 2200 || got.Core != 429 || got.Cards != 16 {
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
