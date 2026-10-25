package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// catalogue is the full set of committed documents.
type catalogue struct {
	Editions     editionsDoc
	Models       modelsDoc
	Registers    map[string]*registersDoc
	Dictionaries dictionariesDoc
	Quarantines  quarantinesDoc
}

// counts is the accounting the generator asserts.
type counts struct {
	RO     int
	Core   int
	Models int
	Cards  int
}

var defaultExpect = counts{RO: 3115, Core: 429, Models: 2113, Cards: 16}

func (c counts) String() string {
	return fmt.Sprintf("ro=%d,core=%d,models=%d,cards=%d", c.RO, c.Core, c.Models, c.Cards)
}

func parseExpect(text string) (counts, error) {
	expect := defaultExpect
	if strings.TrimSpace(text) == "" {
		return expect, nil
	}
	for _, part := range strings.Split(text, ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		number, err := strconv.Atoi(value)
		if !ok || err != nil || number < 0 {
			return counts{}, fmt.Errorf("invalid -expect entry %q", part)
		}
		switch name {
		case "ro":
			expect.RO = number
		case "core":
			expect.Core = number
		case "models":
			expect.Models = number
		case "cards":
			expect.Cards = number
		default:
			return counts{}, fmt.Errorf("unknown -expect count %q", name)
		}
	}
	return expect, nil
}

func countCatalogue(c *catalogue) counts {
	var result counts
	result.Cards = len(c.Registers)
	for _, doc := range c.Registers {
		result.RO += len(doc.Rows)
		for _, row := range doc.Rows {
			if row.Core != nil {
				result.Core++
			}
		}
	}
	for _, row := range c.Models.Rows {
		if row.SourceSet == sourceSetLocator {
			result.Models++
		}
	}
	return result
}

const (
	editionsFile     = "editions.json"
	modelsFile       = "models.json"
	dictionariesFile = "dictionaries.json"
	quarantinesFile  = "quarantines.json"
)

func registersFileName(card string) string { return "registers-" + strings.ToLower(card) + ".json" }

// encodeCatalogue returns the canonical bytes of every committed file.
func encodeCatalogue(c *catalogue) (map[string][]byte, error) {
	files := map[string][]byte{}
	documents := map[string]any{editionsFile: &c.Editions, modelsFile: &c.Models, dictionariesFile: &c.Dictionaries, quarantinesFile: &c.Quarantines}
	for name, doc := range c.Registers {
		documents[name] = doc
	}
	for name, doc := range documents {
		encoded, err := encodeDocument(doc)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		files[name] = encoded
	}
	return files, nil
}

// loadCatalogue reads and strictly decodes every file of a catalogue
// directory. A file the schema does not name is refused.
func loadCatalogue(directory string) (*catalogue, map[string][]byte, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, nil, err
	}
	raw := map[string][]byte{}
	c := &catalogue{Registers: map[string]*registersDoc{}}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			return nil, nil, fmt.Errorf("unexpected directory %s", name)
		}
		data, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			return nil, nil, err
		}
		raw[name] = data
		var target any
		switch {
		case name == editionsFile:
			target = &c.Editions
		case name == modelsFile:
			target = &c.Models
		case name == dictionariesFile:
			target = &c.Dictionaries
		case name == quarantinesFile:
			target = &c.Quarantines
		case strings.HasPrefix(name, "registers-") && strings.HasSuffix(name, ".json"):
			doc := &registersDoc{}
			c.Registers[name] = doc
			target = doc
		default:
			return nil, nil, fmt.Errorf("unexpected file %s", name)
		}
		if err := decodeStrict(data, target); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	return c, raw, nil
}

var (
	validClasses     = map[string]bool{classReviewed: true, classConditional: true, classRawOnly: true, classConflict: true}
	validEndpoints   = map[string]bool{endpointInverter: true, endpointEMMA: true, endpointEMMAAssociate: true}
	validForms       = map[string]bool{formSupportedModel: true, formRevisionHistory: true}
	validFloorBasis  = map[string]bool{floorBasisRow: true, floorBasisReviewed: true, floorNotStated: true}
	validSourceSets  = map[string]bool{sourceSetLocator: true, sourceSetCard: true}
	validSourceClass = map[string]bool{sourceClassVendor: true, sourceClassMirrored: true}
	validKinds       = map[string]bool{kindStatusEnum: true, kindValueEnum: true, kindSelector: true, kindAlarmBits: true}
	validOrigins     = map[string]bool{originReference: true, originGenerator: true}
)

func hasFlag(flags []string, id string) bool {
	for _, flag := range flags {
		if flag == id {
			return true
		}
	}
	return false
}

// checkRegisterRow enforces the class invariant: reviewed and conditional rows
// carry decode fields and no literal cells; raw-only and conflict rows carry
// literal cells and no decode fields.
func checkRegisterRow(row registerRow, access string) error {
	switch {
	case !validClasses[row.Class]:
		return fmt.Errorf("unknown class %q", row.Class)
	case (row.Class == classReviewed || row.Class == classConditional) && (row.Decode == nil || row.Literal != nil):
		return fmt.Errorf("%s row must carry decode fields and no literal cells", row.Class)
	case (row.Class == classRawOnly || row.Class == classConflict) && (row.Decode != nil || row.Literal == nil):
		return fmt.Errorf("%s row must carry literal cells and no decode fields", row.Class)
	case row.Decode != nil && (row.Decode.Gain == "" || row.Decode.Unit == ""):
		return fmt.Errorf("decode fields are incomplete")
	case row.Literal != nil && (row.Literal.Gain == "" || row.Literal.Unit == ""):
		return fmt.Errorf("literal cells are incomplete")
	case !validEndpoints[row.Endpoint]:
		return fmt.Errorf("unknown endpoint %q", row.Endpoint)
	case row.Key == "" || row.Type == "":
		return fmt.Errorf("key or type is empty")
	case row.Access != access:
		return fmt.Errorf("access %q, want %q", row.Access, access)
	case row.Quantity < 1 || row.Quantity > 125:
		return fmt.Errorf("quantity %d outside 1 to 125", row.Quantity)
	case row.Page < 1:
		return fmt.Errorf("page %d", row.Page)
	case (row.Offset == nil) != hasFlag(row.Quarantine, quarantineTemplateRow):
		return fmt.Errorf("a row has no offset exactly when it is a template row")
	case row.Offset != nil && (*row.Offset < 0 || *row.Offset > 65535):
		return fmt.Errorf("offset %d outside the register space", *row.Offset)
	case row.Core != nil && (row.Core.Key == "" || row.Core.Gate == ""):
		return fmt.Errorf("core reference is incomplete")
	case row.Occurrence < 0:
		return fmt.Errorf("occurrence %d", row.Occurrence)
	}
	return nil
}

func rowIdentity(row registerRow) string {
	offset := "template"
	if row.Offset != nil {
		offset = strconv.Itoa(*row.Offset)
	}
	return row.Endpoint + "/" + offset + "/" + row.Key
}

func checkRowSet(rows []registerRow, access string, doc *registersDoc) error {
	gates := map[string]bool{}
	for _, item := range doc.Gates {
		gates[item.ID] = true
	}
	classes := map[string]bool{}
	for _, item := range doc.FunctionalClasses {
		classes[item.ID] = true
	}
	occurrences := map[string][]int{}
	for index, row := range rows {
		if err := checkRegisterRow(row, access); err != nil {
			return fmt.Errorf("row %d (%s): %w", index, rowIdentity(row), err)
		}
		for _, name := range row.Gates {
			if !gates[name] {
				return fmt.Errorf("row %d (%s) names undefined gate %s", index, rowIdentity(row), name)
			}
		}
		if row.FunctionalClass != "" && len(classes) > 0 && access == "RO" && !classes[row.FunctionalClass] {
			return fmt.Errorf("row %d (%s) names undefined functional class %s", index, rowIdentity(row), row.FunctionalClass)
		}
		occurrences[rowIdentity(row)] = append(occurrences[rowIdentity(row)], row.Occurrence)
	}
	for identity, list := range occurrences {
		if len(list) == 1 {
			if list[0] != 0 {
				return fmt.Errorf("%s: a single row carries occurrence %d", identity, list[0])
			}
			continue
		}
		sort.Ints(list)
		for index, value := range list {
			if value != index+1 {
				return fmt.Errorf("%s: repeated rows must be numbered 1 to %d", identity, len(list))
			}
		}
	}
	return nil
}

func checkModelRow(row modelRow) error {
	switch {
	case !validForms[row.Form]:
		return fmt.Errorf("unknown form %q", row.Form)
	case !validFloorBasis[row.FloorBasis] || !validSourceSets[row.SourceSet]:
		return fmt.Errorf("unknown floor basis or source set")
	case row.Page < 1:
		return fmt.Errorf("page %d", row.Page)
	case row.MinimumSoftware == "":
		return fmt.Errorf("no minimum software")
	case (row.FloorBasis == floorNotStated) != (row.MinimumSoftware == floorNotStated):
		return fmt.Errorf("floor basis %q disagrees with minimum software %q", row.FloorBasis, row.MinimumSoftware)
	case row.Form == formRevisionHistory && (row.Token != "" || row.Identifier != nil || row.MinimumSoftware != floorNotStated || !hasFlag(row.Quarantine, quarantineHistoryLine)):
		return fmt.Errorf("a change-history line carries only its edition, page and flag")
	case row.Form == formSupportedModel && row.Token == "":
		return fmt.Errorf("a supported-model row has no token")
	case row.Form == formSupportedModel && (row.Identifier == nil) != hasFlag(row.Quarantine, quarantineNoIdentifier):
		return fmt.Errorf("a supported-model row lacks an identifier exactly when it carries M04")
	case row.MinimumSoftware != floorNotStated && !floorGrammar.MatchString(row.MinimumSoftware) &&
		!hasFlag(row.Quarantine, quarantineFloorCell) && !hasFlag(row.Quarantine, "A06"):
		return fmt.Errorf("floor %q is outside the branch form and carries no quarantine", row.MinimumSoftware)
	case strings.ContainsAny(row.MinimumSoftware, "<>≤≥~*|") || strings.Contains(strings.ToLower(row.MinimumSoftware), " to "):
		return fmt.Errorf("minimum software %q reads as a range", row.MinimumSoftware)
	case (row.WireToken != "") != mapZeroTokenPattern.MatchString(row.Token):
		return fmt.Errorf("wire token present exactly for the MAP0 underscore tokens")
	case row.WireToken != "" && (row.WireToken != strings.ReplaceAll(row.Token, "_", "-") || row.WireTokenGrade != gradeHypothesis):
		return fmt.Errorf("wire token %q of %q is not the hyphenated hypothesis", row.WireToken, row.Token)
	case row.WireToken == "" && row.WireTokenGrade != "":
		return fmt.Errorf("wire token grade without a wire token")
	}
	return nil
}

// validateCatalogue enforces every structural and cross-file invariant.
func validateCatalogue(c *catalogue) error {
	if c.Editions.Schema != editionsSchema || c.Models.Schema != modelsSchema || c.Dictionaries.Schema != dictionariesSchema || c.Quarantines.Schema != quarantinesSchema {
		return fmt.Errorf("unexpected schema identifier")
	}
	if c.Editions.Attribution != attributionNote {
		return fmt.Errorf("the attribution note is missing or changed")
	}
	editions := map[string]edition{}
	cardEdition := map[string]string{}
	identity := map[string][]edition{}
	for _, record := range c.Editions.Editions {
		if _, exists := editions[record.ID]; exists || record.ID == "" {
			return fmt.Errorf("edition id %q repeated or empty", record.ID)
		}
		if record.Title == "" || record.Issue == "" || record.Date == "" || !validSourceClass[record.SourceClass] {
			return fmt.Errorf("edition %s is incomplete", record.ID)
		}
		editions[record.ID] = record
		if record.Card != "" {
			if _, exists := cardEdition[record.Card]; exists {
				return fmt.Errorf("card %s has two source editions", record.Card)
			}
			cardEdition[record.Card] = record.ID
		}
		key := record.Title + "\x00" + record.Issue + "\x00" + record.Date
		identity[key] = append(identity[key], record)
	}
	for _, group := range identity {
		for _, record := range group {
			if (len(group) > 1) != (record.Copy > 0) {
				return fmt.Errorf("edition %s: a copy number is set exactly when title, issue and date repeat", record.ID)
			}
		}
	}
	cardRows := map[string]map[int]bool{}
	for name, doc := range c.Registers {
		if doc.Schema != registersSchema || name != registersFileName(doc.Card) {
			return fmt.Errorf("%s: schema or card name mismatch", name)
		}
		if cardEdition[doc.Card] != doc.Edition || doc.Edition == "" {
			return fmt.Errorf("%s: edition %s is not the source edition of card %s", name, doc.Edition, doc.Card)
		}
		if len(doc.ModelPages) == 0 || len(doc.Rows) == 0 {
			return fmt.Errorf("%s: no model pages or rows", name)
		}
		gateIDs := map[string]bool{}
		for _, item := range doc.Gates {
			if gateIDs[item.ID] || item.Meaning == "" {
				return fmt.Errorf("%s: gate %s repeated or empty", name, item.ID)
			}
			gateIDs[item.ID] = true
		}
		if err := checkRowSet(doc.Rows, "RO", doc); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := checkRowSet(doc.ReadableConfiguration, "RW", doc); err != nil {
			return fmt.Errorf("%s readable configuration: %w", name, err)
		}
		offsets := map[int]bool{}
		for _, row := range append(append([]registerRow(nil), doc.Rows...), doc.ReadableConfiguration...) {
			if row.Offset != nil {
				offsets[*row.Offset] = true
			}
		}
		cardRows[doc.Card] = offsets
	}
	if len(cardEdition) != len(c.Registers) {
		return fmt.Errorf("%d card editions for %d register files", len(cardEdition), len(c.Registers))
	}
	pairs := map[string]bool{}
	tokens := map[string]string{}
	for index, row := range c.Models.Rows {
		if _, ok := editions[row.Edition]; !ok {
			return fmt.Errorf("model row %d names unknown edition %s", index, row.Edition)
		}
		if err := checkModelRow(row); err != nil {
			return fmt.Errorf("model row %d (%s %s): %w", index, row.Edition, row.Token, err)
		}
		if row.Form != formSupportedModel {
			continue
		}
		identifier := "none"
		if row.Identifier != nil {
			identifier = strconv.Itoa(*row.Identifier)
			key := row.Edition + "\x00" + identifier
			if token, exists := tokens[key]; exists && token != row.Token {
				return fmt.Errorf("edition %s gives identifier %s to %s and %s", row.Edition, identifier, token, row.Token)
			}
			tokens[key] = row.Token
		}
		pair := row.Edition + "\x00" + row.Token + "\x00" + identifier
		if pairs[pair] {
			return fmt.Errorf("edition %s repeats %s/%s", row.Edition, row.Token, identifier)
		}
		pairs[pair] = true
	}
	dictionaryIDs := map[string]bool{}
	for _, item := range c.Dictionaries.Dictionaries {
		if dictionaryIDs[item.ID] || item.ID == "" || !validKinds[item.Kind] {
			return fmt.Errorf("dictionary %q repeated, empty or of unknown kind", item.ID)
		}
		dictionaryIDs[item.ID] = true
		if cardEdition[item.Card] != item.Edition || len(item.Offsets) == 0 || len(item.Pages) == 0 || item.ModePredicate == "" || item.Unlisted == "" || !validEndpoints[item.Endpoint] {
			return fmt.Errorf("dictionary %s is incomplete or not bound to its card edition", item.ID)
		}
		for _, offset := range item.Offsets {
			if !cardRows[item.Card][offset] {
				return fmt.Errorf("dictionary %s names offset %d, which card %s does not carry", item.ID, offset, item.Card)
			}
		}
		if (item.Kind == kindAlarmBits) != (len(item.Bits) > 0) || (item.Kind == kindAlarmBits) != (len(item.Modules) > 0) {
			return fmt.Errorf("dictionary %s: bits and modules belong to alarm dictionaries only", item.ID)
		}
		codes := map[string]bool{}
		for _, value := range append(append([]dictValue(nil), item.Values...), item.Modules...) {
			if value.Code == "" || value.Meaning == "" {
				return fmt.Errorf("dictionary %s has an empty value", item.ID)
			}
		}
		for _, value := range item.Values {
			if codes[value.Code] {
				return fmt.Errorf("dictionary %s repeats code %s", item.ID, value.Code)
			}
			codes[value.Code] = true
		}
		bits := map[string]bool{}
		for _, bit := range item.Bits {
			key := strconv.Itoa(bit.Offset) + "/" + strconv.Itoa(bit.Bit)
			if bits[key] || bit.Bit < 0 || bit.Bit > 15 || bit.Label == "" || bit.Page < 1 {
				return fmt.Errorf("dictionary %s bit %s repeated or invalid", item.ID, key)
			}
			bits[key] = true
		}
	}
	return checkQuarantines(c, editions)
}

// checkQuarantines verifies that the affected rows of every quarantine are
// exactly the rows that carry its flag.
func checkQuarantines(c *catalogue, editions map[string]edition) error {
	builder := newQuarantineBuilder()
	for _, item := range c.Quarantines.Quarantines {
		if _, exists := builder.items[item.ID]; exists || !validOrigins[item.Origin] || item.Subject == "" || item.Meaning == "" {
			return fmt.Errorf("quarantine %q repeated or incomplete", item.ID)
		}
		if item.Editions == nil || item.Affected == nil {
			return fmt.Errorf("quarantine %s must list editions and affected rows", item.ID)
		}
		for _, editionID := range item.Editions {
			if _, ok := editions[editionID]; !ok {
				return fmt.Errorf("quarantine %s names unknown edition %s", item.ID, editionID)
			}
		}
		builder.add(quarantine{ID: item.ID, Origin: item.Origin, Subject: item.Subject, Evidence: item.Evidence, Meaning: item.Meaning})
		for _, editionID := range item.Editions {
			builder.edition(item.ID, editionID)
		}
	}
	names := make([]string, 0, len(c.Registers))
	for name := range c.Registers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		doc := c.Registers[name]
		for _, list := range [][]registerRow{doc.Rows, doc.ReadableConfiguration} {
			for index := range list {
				row := list[index]
				for _, flag := range row.Quarantine {
					if _, ok := builder.items[flag]; !ok {
						return fmt.Errorf("%s %s carries unknown quarantine %s", name, rowIdentity(row), flag)
					}
					reference := affectedRow{Set: affectedRegisters, Card: doc.Card, Offset: row.Offset, Key: row.Key, Occurrence: row.Occurrence}
					builder.affected[flag][affectedIdentity(reference)] = reference
				}
			}
		}
	}
	for index := range c.Models.Rows {
		row := c.Models.Rows[index]
		for _, flag := range row.Quarantine {
			if _, ok := builder.items[flag]; !ok {
				return fmt.Errorf("model row %d carries unknown quarantine %s", index, flag)
			}
			builder.recordModel(flag, &row)
		}
	}
	rebuilt := builder.build()
	if len(rebuilt) != len(c.Quarantines.Quarantines) {
		return fmt.Errorf("quarantine list is not one entry per identifier")
	}
	for index, item := range c.Quarantines.Quarantines {
		if rebuilt[index].ID != item.ID {
			return fmt.Errorf("quarantines are not ordered by identifier at %s", item.ID)
		}
		if len(rebuilt[index].Affected) != len(item.Affected) {
			return fmt.Errorf("quarantine %s lists %d affected rows, %d rows carry its flag", item.ID, len(item.Affected), len(rebuilt[index].Affected))
		}
		for position := range item.Affected {
			if affectedIdentity(item.Affected[position]) != affectedIdentity(rebuilt[index].Affected[position]) {
				return fmt.Errorf("quarantine %s affected row %d disagrees with the flagged rows", item.ID, position)
			}
		}
		for _, editionID := range rebuilt[index].Editions {
			if !hasFlag(item.Editions, editionID) {
				return fmt.Errorf("quarantine %s omits edition %s of a flagged row", item.ID, editionID)
			}
		}
	}
	return nil
}

// guardCatalogue applies the publication guard to every committed string.
func guardCatalogue(c *catalogue) error {
	documents := map[string]any{editionsFile: c.Editions, modelsFile: c.Models, dictionariesFile: c.Dictionaries, quarantinesFile: c.Quarantines}
	for name, doc := range c.Registers {
		documents[name] = doc
	}
	names := make([]string, 0, len(documents))
	for name := range documents {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := guardStrings(name, documents[name]); err != nil {
			return err
		}
	}
	return nil
}
