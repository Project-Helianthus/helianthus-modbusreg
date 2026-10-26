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

func TestCommittedCatalogueMatchesTheBaselineAccounting(t *testing.T) {
	c, _ := loadCommitted(t)
	if got := countCatalogue(c); got != defaultExpect {
		t.Fatalf("counts %s, want %s", got, defaultExpect)
	}
	if c.Editions.ReferenceBaseline != "2026-10-26.1" || len(c.Editions.Editions) != 49 {
		t.Fatalf("baseline %q with %d editions", c.Editions.ReferenceBaseline, len(c.Editions.Editions))
	}
	wantRows := map[string]int{
		"SUN-C81": 87, "SUN-V300": 71, "COMMERCIAL": 50, "HA": 122, "HB": 54, "HD": 142, "L-LEGACY": 51,
		"L-V200": 224, "LB-C00": 232, "LB-C10": 315, "LC": 232, "MA-C00": 224, "MA-C20": 364,
		"MA-V200": 410, "MC": 364, "MB02": 173,
	}
	wantControls := map[string]int{
		"SUN-C81": 90, "SUN-V300": 32, "COMMERCIAL": 7, "HA": 25, "HB": 21, "HD": 29, "L-LEGACY": 22,
		"L-V200": 75, "LB-C00": 78, "LB-C10": 112, "LC": 79, "MA-C00": 78, "MA-C20": 482,
		"MA-V200": 282, "MC": 482, "MB02": 241,
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
		for _, row := range doc.Controls {
			classes[row.Class]++
		}
		if len(doc.Rows) != wantRows[doc.Card] || len(doc.Controls) != wantControls[doc.Card] || core != wantCore[doc.Card] {
			t.Fatalf("%s: %d rows, %d controls, %d core rows", doc.Card, len(doc.Rows), len(doc.Controls), core)
		}
	}
	want := map[string]int{classReviewed: 372, classConditional: 2127, classRawOnly: 596, classConflict: 20, classReadableConfiguration: 1910, classNoSend: 225}
	if !reflect.DeepEqual(classes, want) {
		t.Fatalf("class totals %v", classes)
	}
	stated := 0
	for _, record := range c.Editions.Editions {
		stated += record.ModelRows
	}
	if stated != len(c.Models.Rows) {
		t.Fatalf("editions state %d model rows, %d present", stated, len(c.Models.Rows))
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

func TestNoRowDecodesBeyondItsClass(t *testing.T) {
	c, raw := loadCommitted(t)
	for _, doc := range c.Registers {
		for _, row := range doc.Rows {
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
				t.Fatalf("%s %s: class %q on a read-only row", doc.Card, rowIdentity(row), row.Class)
			}
		}
		for _, row := range doc.Controls {
			if row.Decode != nil || row.Literal == nil || (row.Class != classReadableConfiguration && row.Class != classNoSend) {
				t.Fatalf("%s %s: control row %q decodes or has a read-only class", doc.Card, rowIdentity(row), row.Class)
			}
			if row.Class == classReadableConfiguration && row.Access != "RW" {
				t.Fatalf("%s %s: readable configuration with access %s", doc.Card, rowIdentity(row), row.Access)
			}
		}
	}
	for name, data := range raw {
		if !strings.HasPrefix(name, "registers-") {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			nonDecoding := strings.Contains(line, `"class":"raw-only"`) || strings.Contains(line, `"class":"conflict"`) ||
				strings.Contains(line, `"class":"readable-configuration"`) || strings.Contains(line, `"class":"no-send"`)
			if nonDecoding && strings.Contains(line, `"decode":`) {
				t.Fatalf("%s: a non-decoding row carries decode fields: %s", name, line)
			}
		}
	}
}

var modelRowKeys = map[string]bool{
	"edition": true, "page": true, "token": true, "identifier": true, "sequence_number": true, "row_minimum": true,
	"document_requirement": true, "minimum_software": true, "floor_basis": true, "wire_aliases": true, "quarantine": true,
}

func TestEveryModelRowStatesAFloorAndNoModelRowStatesAMaximum(t *testing.T) {
	c, raw := loadCommitted(t)
	var document struct {
		Rows []map[string]json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(raw[modelsFile], &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Rows) != defaultExpect.Models {
		t.Fatalf("%d model rows", len(document.Rows))
	}
	for index, row := range document.Rows {
		for key := range row {
			if !modelRowKeys[key] {
				t.Fatalf("model row %d carries %q; a row states a floor and nothing above it", index, key)
			}
		}
	}
	requirements := map[string]documentRequirement{}
	for _, requirement := range c.Models.DocumentRequirements {
		requirements[requirement.ID] = requirement
	}
	bases := map[string]int{}
	for index, row := range c.Models.Rows {
		if err := checkModelRow(row); err != nil {
			t.Fatalf("model row %d: %v", index, err)
		}
		bases[row.FloorBasis]++
		switch row.FloorBasis {
		case floorBasisRow:
			if row.MinimumSoftware != row.RowMinimum || row.DocumentRequirement != "" {
				t.Fatalf("model row %d: a row floor is its own minimum", index)
			}
		case floorBasisDocument:
			requirement, ok := requirements[row.DocumentRequirement]
			if !ok || row.RowMinimum != floorNotStated || row.MinimumSoftware != requirement.Minimum || !requirement.Inclusive {
				t.Fatalf("model row %d: a document floor is the linked inclusive requirement", index)
			}
		}
	}
	if !reflect.DeepEqual(bases, map[string]int{floorBasisRow: 2282, floorBasisDocument: 119}) {
		t.Fatalf("floor bases %v", bases)
	}
}

func TestSequenceOrdinalsAreNeverModelIdentifiers(t *testing.T) {
	c, _ := loadCommitted(t)
	ordinals := 0
	for _, row := range c.Models.Rows {
		if row.SequenceNumber == "" {
			continue
		}
		ordinals++
		if row.Identifier != nil || !hasFlag(row.Quarantine, quarantineSequenceOrdinal) {
			t.Fatalf("%s %s: a sequence ordinal is treated as a model identifier", row.Edition, row.Token)
		}
	}
	if ordinals != 12 {
		t.Fatalf("%d sequence-ordinal rows, want 12", ordinals)
	}
}

func TestModelTokenAndIdentifierAreUniquePerEdition(t *testing.T) {
	c, _ := loadCommitted(t)
	pairs := map[string]bool{}
	identifiers := map[string]string{}
	for _, row := range c.Models.Rows {
		identifier := "sequence " + row.SequenceNumber
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
		if !reflect.DeepEqual(cards, []string{tc.card}) || len(rows) != 1 || rows[0].Token != tc.token || rows[0].MinimumSoftware != tc.floor ||
			rows[0].FloorBasis != floorBasisRow || len(rows[0].Quarantine) != 0 {
			t.Fatalf("identifier %d resolves to %v with rows %+v", tc.identifier, cards, rows)
		}
	}
	aliases := 0
	for identifier := 1001; identifier <= 1013; identifier++ {
		cards, rows := cardsOfIdentifier(c, identifier)
		if !reflect.DeepEqual(cards, []string{"MA-V200"}) || len(rows) != 1 || rows[0].MinimumSoftware != "V200R024C00" {
			t.Fatalf("MAP0 identifier %d resolves to %v with rows %+v", identifier, cards, rows)
		}
		for _, alias := range rows[0].WireAliases {
			if alias.Token != strings.ReplaceAll(rows[0].Token, "_", "-") {
				t.Fatalf("MAP0 identifier %d: wire alias %q is not the hyphenated token", identifier, alias.Token)
			}
			aliases++
		}
	}
	if aliases == 0 {
		t.Fatal("no MAP0 row of the MA-V200 edition carries its hyphenated wire alias")
	}
}

func TestUnspecifiedAddressesStayUnrequested(t *testing.T) {
	c, _ := loadCommitted(t)
	var unspecified []string
	for _, doc := range c.Registers {
		for _, row := range doc.Rows {
			if row.AddressStatus == "" {
				continue
			}
			if row.Offset != nil || row.Class != classRawOnly {
				t.Fatalf("%s %s: an unspecified address carries an offset or decodes", doc.Card, row.Key)
			}
			unspecified = append(unspecified, doc.Card+" "+row.Key)
		}
	}
	sort.Strings(unspecified)
	if !reflect.DeepEqual(unspecified, []string{"COMMERCIAL pv.channel.pvnamps", "COMMERCIAL pv.channel.pvnvolts"}) {
		t.Fatalf("unspecified addresses %v", unspecified)
	}
}

func TestReadExtentsKeepValuesWhole(t *testing.T) {
	c, _ := loadCommitted(t)
	for _, card := range []string{"MC", "MA-C00", "MA-V200"} {
		doc := c.Registers[registersFileName(card)]
		found := false
		for _, extent := range doc.ReadExtents {
			if extent.Start != 32064 {
				continue
			}
			found = true
			if !containsInt(extent.SafeQuantities, 31) || containsInt(extent.SafeQuantities, 32) {
				t.Fatalf("%s: the extent from 32064 must allow 31 words and refuse 32: %v", card, extent.SafeQuantities)
			}
		}
		if !found {
			t.Fatalf("%s: no read extent starts at 32064", card)
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

func TestNoEnumerationCodeIsATruncatedSentinel(t *testing.T) {
	c, _ := loadCommitted(t)
	sentinels := map[string][]string{}
	key := func(d dictionary) string { return d.Card + "/" + itoa(d.Offsets[0]) }
	for _, d := range c.Dictionaries.Dictionaries {
		if d.Kind == kindSentinels && len(d.Offsets) > 0 {
			for _, v := range d.Values {
				sentinels[key(d)] = append(sentinels[key(d)], strings.ToLower(v.Code))
			}
		}
	}
	for _, d := range c.Dictionaries.Dictionaries {
		if d.Kind != kindValueEnum || len(d.Offsets) == 0 {
			continue
		}
		for _, v := range d.Values {
			code := strings.ToLower(v.Code)
			for _, s := range sentinels[key(d)] {
				if strings.HasPrefix(code, "0x") && len(code) < len(s) && strings.HasPrefix(s, code) {
					t.Errorf("%s: code %s truncates sentinel %s", d.ID, v.Code, s)
				}
			}
		}
	}
}
