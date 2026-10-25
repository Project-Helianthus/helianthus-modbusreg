package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const catalogueDirectory = "../../profiles/vendor/huawei/catalogue"

func loadCommitted(t *testing.T) (*catalogue, map[string][]byte) {
	t.Helper()
	c, raw, err := loadCatalogue(catalogueDirectory)
	if err != nil {
		t.Fatal(err)
	}
	return c, raw
}

func TestCommittedCatalogueRoundTripsByteForByte(t *testing.T) {
	c, raw := loadCommitted(t)
	encoded, err := encodeCatalogue(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != len(raw) {
		t.Fatalf("%d files encoded, %d committed", len(encoded), len(raw))
	}
	for name, data := range raw {
		if !bytes.Equal(encoded[name], data) {
			t.Fatalf("%s does not round-trip byte for byte", name)
		}
	}
}

func TestCommittedCatalogueMatchesTheReferenceAccounting(t *testing.T) {
	c, _ := loadCommitted(t)
	if got := countCatalogue(c); got != defaultExpect {
		t.Fatalf("counts %s, want %s", got, defaultExpect)
	}
	wantRows := map[string]int{
		"SUN-C81": 87, "SUN-V300": 71, "COMMERCIAL": 50, "HA": 122, "HB": 54, "HD": 142, "L-LEGACY": 51,
		"L-V200": 224, "LB-C00": 232, "LB-C10": 315, "LC": 232, "MA-C00": 224, "MA-C20": 364,
		"MA-V200": 410, "MC": 364, "MB02": 173,
	}
	wantCore := map[string]int{
		"SUN-C81": 47, "SUN-V300": 28, "COMMERCIAL": 27, "HA": 28, "HB": 22, "HD": 28, "L-LEGACY": 25,
		"L-V200": 28, "LB-C00": 28, "LB-C10": 28, "LC": 28, "MA-C00": 28, "MA-C20": 28,
		"MA-V200": 28, "MC": 28, "MB02": 0,
	}
	classes := map[string]int{}
	for _, doc := range c.Registers {
		core := 0
		for _, row := range doc.Rows {
			classes[row.Class]++
			if row.Core != nil {
				core++
			}
		}
		if len(doc.Rows) != wantRows[doc.Card] || core != wantCore[doc.Card] {
			t.Fatalf("%s: %d rows and %d core rows, want %d and %d", doc.Card, len(doc.Rows), core, wantRows[doc.Card], wantCore[doc.Card])
		}
	}
	if !reflect.DeepEqual(classes, map[string]int{classReviewed: 372, classConditional: 2127, classRawOnly: 596, classConflict: 20}) {
		t.Fatalf("class totals %v", classes)
	}
	supplement := 0
	for _, row := range c.Models.Rows {
		if row.SourceSet == sourceSetCard {
			supplement++
		}
	}
	if supplement != 23 {
		t.Fatalf("%d supplement model rows, want the 23 rows of the MB02 card", supplement)
	}
}

func TestCommittedCatalogueSatisfiesEveryInvariant(t *testing.T) {
	c, _ := loadCommitted(t)
	if err := validateCatalogue(c); err != nil {
		t.Fatal(err)
	}
	if err := guardCatalogue(c); err != nil {
		t.Fatal(err)
	}
}

func TestRawOnlyAndConflictRowsCarryNoDecodeFields(t *testing.T) {
	c, raw := loadCommitted(t)
	checked := 0
	for _, doc := range c.Registers {
		for _, row := range append(append([]registerRow(nil), doc.Rows...), doc.ReadableConfiguration...) {
			switch row.Class {
			case classRawOnly, classConflict:
				if row.Decode != nil || row.Literal == nil {
					t.Fatalf("%s %s: %s row grants decoding", doc.Card, rowIdentity(row), row.Class)
				}
			case classReviewed, classConditional:
				if row.Decode == nil || row.Literal != nil {
					t.Fatalf("%s %s: %s row lacks its decode fields", doc.Card, rowIdentity(row), row.Class)
				}
			default:
				t.Fatalf("%s %s: unknown class %q", doc.Card, rowIdentity(row), row.Class)
			}
			checked++
		}
	}
	if checked < defaultExpect.RO {
		t.Fatalf("checked %d rows", checked)
	}
	for name, data := range raw {
		if !strings.HasPrefix(name, "registers-") {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			nonDecoding := strings.Contains(line, `"class":"raw-only"`) || strings.Contains(line, `"class":"conflict"`)
			if nonDecoding && strings.Contains(line, `"decode":`) {
				t.Fatalf("%s: a non-decoding row carries decode fields: %s", name, line)
			}
		}
	}
}

var modelRowKeys = map[string]bool{
	"edition": true, "page": true, "form": true, "token": true, "identifier": true, "minimum_software": true,
	"floor_basis": true, "wire_token": true, "wire_token_grade": true, "source_set": true, "quarantine": true,
}

func TestEveryModelRowStatesAFloorAndNoModelRowStatesAMaximum(t *testing.T) {
	_, raw := loadCommitted(t)
	var document struct {
		Rows []map[string]json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(raw[modelsFile], &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Rows) == 0 {
		t.Fatal("no model rows")
	}
	floors := 0
	for index, row := range document.Rows {
		for key := range row {
			if !modelRowKeys[key] {
				t.Fatalf("model row %d carries %q; a row states a floor and nothing above it", index, key)
			}
		}
		var floor string
		if err := json.Unmarshal(row["minimum_software"], &floor); err != nil || floor == "" {
			t.Fatalf("model row %d has no minimum software", index)
		}
		if floor != floorNotStated {
			floors++
		}
	}
	c, _ := loadCommitted(t)
	for index, row := range c.Models.Rows {
		if err := checkModelRow(row); err != nil {
			t.Fatalf("model row %d: %v", index, err)
		}
	}
	if floors < 2000 {
		t.Fatalf("only %d rows state a floor", floors)
	}
}

func TestModelTokenAndIdentifierAreUniquePerEdition(t *testing.T) {
	c, _ := loadCommitted(t)
	pairs := map[string]bool{}
	identifiers := map[string]string{}
	for _, row := range c.Models.Rows {
		if row.Form != formSupportedModel {
			continue
		}
		identifier := "none"
		if row.Identifier != nil {
			identifier = itoa(*row.Identifier)
			key := row.Edition + "/" + identifier
			if token, ok := identifiers[key]; ok && token != row.Token {
				t.Fatalf("%s: identifier %s names %s and %s", row.Edition, identifier, token, row.Token)
			}
			identifiers[key] = row.Token
		}
		pair := row.Edition + "/" + row.Token + "/" + identifier
		if pairs[pair] {
			t.Fatalf("%s repeats %s/%s", row.Edition, row.Token, identifier)
		}
		pairs[pair] = true
	}
}

// cardsOfIdentifier returns the cards whose source edition lists a model row
// with the identifier, and those rows.
func cardsOfIdentifier(c *catalogue, identifier int) ([]string, []modelRow) {
	cardOf := map[string]string{}
	for _, record := range c.Editions.Editions {
		if record.Card != "" {
			cardOf[record.ID] = record.Card
		}
	}
	set := map[string]bool{}
	var rows []modelRow
	for _, row := range c.Models.Rows {
		if row.Identifier == nil || *row.Identifier != identifier || cardOf[row.Edition] == "" {
			continue
		}
		set[cardOf[row.Edition]] = true
		rows = append(rows, row)
	}
	cards := make([]string, 0, len(set))
	for card := range set {
		cards = append(cards, card)
	}
	sort.Strings(cards)
	return cards, rows
}

func TestQualificationIdentifiersResolveToOneCardEach(t *testing.T) {
	c, _ := loadCommitted(t)
	for _, tc := range []struct {
		identifier int
		card       string
		token      string
		floor      string
	}{
		{471, "MC", "SUN2000-50KTL-M3", "SUN2000MC V200R023C00"},
		{428, "MA-C00", "SUN2000-8KTL-M1", "SUN2000MA V100R001C00"},
	} {
		cards, rows := cardsOfIdentifier(c, tc.identifier)
		if !reflect.DeepEqual(cards, []string{tc.card}) || len(rows) != 1 || rows[0].Token != tc.token || rows[0].MinimumSoftware != tc.floor || len(rows[0].Quarantine) != 0 {
			t.Fatalf("identifier %d resolves to %v with rows %+v", tc.identifier, cards, rows)
		}
	}
	for identifier := 1001; identifier <= 1013; identifier++ {
		cards, rows := cardsOfIdentifier(c, identifier)
		if !reflect.DeepEqual(cards, []string{"MA-V200"}) || len(rows) != 1 {
			t.Fatalf("MAP0 identifier %d resolves to %v", identifier, cards)
		}
		row := rows[0]
		if row.MinimumSoftware != "V200R024C00" || row.WireToken != strings.ReplaceAll(row.Token, "_", "-") || row.WireTokenGrade != gradeHypothesis {
			t.Fatalf("MAP0 identifier %d row %+v", identifier, row)
		}
	}
}

func TestCatalogueIsDataOnly(t *testing.T) {
	files, err := filepath.Glob("../../*.go")
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
		if bytes.Contains(data, []byte("huawei/catalogue")) || bytes.Contains(data, []byte("registers-")) {
			t.Fatalf("%s reads the catalogue; selection is out of scope here", file)
		}
	}
}

func TestCommittedCatalogueTextPassesThePublicationRules(t *testing.T) {
	_, raw := loadCommitted(t)
	for name, data := range raw {
		text := string(data)
		if ageNearRelease(text) {
			t.Fatalf("%s places an age word next to a release word", name)
		}
		if hasHexRun(text) {
			t.Fatalf("%s carries a digest-like hexadecimal run", name)
		}
		lower := strings.ToLower(text)
		for _, marker := range []string{"http:", "https:", "://", "www.", "../"} {
			if strings.Contains(lower, marker) {
				t.Fatalf("%s carries a link or path marker %q", name, marker)
			}
		}
		if len(wordSpans(lower, forbiddenWords, true)) > 0 {
			t.Fatalf("%s carries a forbidden word", name)
		}
	}
}

func itoa(value int) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
