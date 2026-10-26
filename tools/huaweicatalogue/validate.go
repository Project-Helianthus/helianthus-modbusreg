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
	RO       int
	Core     int
	Controls int
	Models   int
	Cards    int
}

var defaultExpect = counts{RO: 3115, Core: 429, Controls: 2135, Models: 2401, Cards: 16}

func (c counts) String() string {
	return fmt.Sprintf("ro=%d,core=%d,controls=%d,models=%d,cards=%d", c.RO, c.Core, c.Controls, c.Models, c.Cards)
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
		case "controls":
			expect.Controls = number
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
	result := counts{Cards: len(c.Registers), Models: len(c.Models.Rows)}
	for _, doc := range c.Registers {
		result.RO += len(doc.Rows)
		result.Controls += len(doc.Controls)
		for _, row := range doc.Rows {
			if row.Core != nil {
				result.Core++
			}
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
	readOnlyClasses  = map[string]bool{classReviewed: true, classConditional: true, classRawOnly: true, classConflict: true}
	controlClasses   = map[string]bool{classReadableConfiguration: true, classNoSend: true}
	validEndpoints   = map[string]bool{endpointInverter: true, endpointEMMA: true, endpointEMMAAssociate: true}
	validFloorBasis  = map[string]bool{floorBasisRow: true, floorBasisDocument: true}
	validSourceClass = map[string]bool{sourceClassVendor: true, sourceClassMirrored: true}
	validKinds       = map[string]bool{kindStatusEnum: true, kindValueEnum: true, kindBitmap: true, kindSign: true, kindSentinels: true, kindSelector: true, kindAlarmBits: true}
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

// checkRegisterRow enforces the class invariant. Reviewed and conditional rows
// carry decode fields and no literal cells; raw-only and conflict rows carry
// literal cells and no decode fields. Control rows are readable configuration
// or no-send, always with literal cells only.
func checkRegisterRow(row registerRow, control bool) error {
	switch {
	case !control && !readOnlyClasses[row.Class], control && !controlClasses[row.Class]:
		return fmt.Errorf("class %q is not allowed here", row.Class)
	case (row.Class == classReviewed || row.Class == classConditional) && (row.Decode == nil || row.Literal != nil):
		return fmt.Errorf("%s row must carry decode fields and no literal cells", row.Class)
	case (row.Class == classRawOnly || row.Class == classConflict || control) && (row.Decode != nil || row.Literal == nil):
		return fmt.Errorf("%s row must carry literal cells and no decode fields", row.Class)
	case row.Decode != nil && (row.Decode.Gain == "" || row.Decode.Unit == ""):
		return fmt.Errorf("decode fields are incomplete")
	case row.Literal != nil && (row.Literal.Gain == "" || row.Literal.Unit == ""):
		return fmt.Errorf("literal cells are incomplete")
	case !validEndpoints[row.Endpoint]:
		return fmt.Errorf("unknown endpoint %q", row.Endpoint)
	case row.Key == "" || row.Type == "":
		return fmt.Errorf("key or type is empty")
	case !control && row.Access != "RO":
		return fmt.Errorf("a read-only row has access %q", row.Access)
	case control && row.Access != "RW" && row.Access != "WO":
		return fmt.Errorf("a control row has access %q", row.Access)
	case control && row.Class == classReadableConfiguration && row.Access != "RW":
		return fmt.Errorf("a readable-configuration row must be RW")
	case row.Quantity < 1 || row.Quantity > 125:
		return fmt.Errorf("quantity %d outside 1 to 125", row.Quantity)
	case row.Page < 1:
		return fmt.Errorf("page %d", row.Page)
	case (row.Offset == nil) != (row.AddressStatus == addressUnspecified) || (row.AddressStatus != "" && row.AddressStatus != addressUnspecified):
		return fmt.Errorf("a row has no offset exactly when its address is unspecified in the source")
	case control && row.Offset == nil:
		return fmt.Errorf("a control row without an address")
	case row.Offset != nil && (*row.Offset < 0 || *row.Offset > 65535):
		return fmt.Errorf("offset %d outside the register space", *row.Offset)
	case row.Core != nil && (control || row.Core.Key == "" || row.Core.Gate == ""):
		return fmt.Errorf("core reference is incomplete or on a control row")
	case row.Occurrence < 0:
		return fmt.Errorf("occurrence %d", row.Occurrence)
	}
	return nil
}

func rowIdentity(row registerRow) string {
	offset := "unspecified"
	if row.Offset != nil {
		offset = strconv.Itoa(*row.Offset)
	}
	return row.Endpoint + "/" + offset + "/" + row.Key
}

func checkRowSet(rows []registerRow, control bool, doc *registersDoc) error {
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
		if err := checkRegisterRow(row, control); err != nil {
			return fmt.Errorf("row %d (%s): %w", index, rowIdentity(row), err)
		}
		for _, name := range row.Gates {
			if !gates[name] {
				return fmt.Errorf("row %d (%s) names undefined gate %s", index, rowIdentity(row), name)
			}
		}
		if !control && row.FunctionalClass != "" && len(classes) > 0 && !classes[row.FunctionalClass] {
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

func checkReadExtents(extents []readExtent, rows []registerRow) error {
	offsets := map[int]bool{}
	ends := map[int]bool{}
	for _, row := range rows {
		if row.Offset != nil && row.Endpoint == endpointInverter {
			offsets[*row.Offset] = true
			ends[*row.Offset+row.Quantity-1] = true
		}
	}
	previous := -1
	for _, extent := range extents {
		switch {
		case extent.Start < previous || extent.End < extent.Start || extent.Quantity != extent.End-extent.Start+1:
			return fmt.Errorf("read extent at %d is out of order or inconsistent", extent.Start)
		case len(extent.Offsets) == 0 || extent.Offsets[0] != extent.Start || len(extent.SafeQuantities) == 0:
			return fmt.Errorf("read extent at %d has no fields", extent.Start)
		case len(extent.FollowingHole) != 0 && (len(extent.FollowingHole) != 2 || extent.FollowingHole[0] != extent.End+1 || extent.FollowingHole[1] < extent.FollowingHole[0]):
			return fmt.Errorf("read extent at %d has an invalid following hole", extent.Start)
		case len(extent.FollowingHole) != 0 && extent.Hole == "":
			return fmt.Errorf("read extent at %d has a following hole without classification", extent.Start)
		}
		for index, offset := range extent.Offsets {
			if !offsets[offset] || (index > 0 && offset <= extent.Offsets[index-1]) || offset > extent.End {
				return fmt.Errorf("read extent at %d names field %d, which is not an ordered read-only inverter row", extent.Start, offset)
			}
		}
		for index, quantity := range extent.SafeQuantities {
			if quantity < 1 || quantity > extent.Quantity || (index > 0 && quantity <= extent.SafeQuantities[index-1]) {
				return fmt.Errorf("read extent at %d has an invalid safe quantity %d", extent.Start, quantity)
			}
			if !ends[extent.Start+quantity-1] {
				return fmt.Errorf("read extent at %d has safe quantity %d, which ends inside a field", extent.Start, quantity)
			}
		}
		previous = extent.Start
	}
	return nil
}

// sameApartFromPunctuation reports whether two tokens differ only in their
// separators.
func sameApartFromPunctuation(a, b string) bool {
	strip := strings.NewReplacer("_", "", "-", "")
	return a != b && strip.Replace(a) == strip.Replace(b)
}

func checkModelRow(row modelRow) error {
	switch {
	case row.Token == "" || row.Page < 1:
		return fmt.Errorf("token or page missing")
	case !validFloorBasis[row.FloorBasis]:
		return fmt.Errorf("unknown floor basis %q", row.FloorBasis)
	case row.RowMinimum == "" || row.MinimumSoftware == "" || row.MinimumSoftware == floorNotStated:
		return fmt.Errorf("every model row states a floor")
	case row.FloorBasis == floorBasisRow && (row.RowMinimum == floorNotStated || row.MinimumSoftware != row.RowMinimum || row.DocumentRequirement != ""):
		return fmt.Errorf("a row floor is the row minimum")
	case row.FloorBasis == floorBasisDocument && (row.RowMinimum != floorNotStated || row.DocumentRequirement == ""):
		return fmt.Errorf("a document floor needs an absent row minimum and a document requirement")
	case (row.Identifier == nil) != (row.SequenceNumber != "") || (row.Identifier == nil) != hasFlag(row.Quarantine, quarantineSequenceOrdinal):
		return fmt.Errorf("a row lacks a model identifier exactly when it carries a sequence ordinal and M01")
	case !floorGrammar.MatchString(row.MinimumSoftware) && !hasFlag(row.Quarantine, quarantineFloorCell) && !hasFlag(row.Quarantine, "A06"):
		return fmt.Errorf("floor %q is outside the branch form and carries no quarantine", row.MinimumSoftware)
	case strings.ContainsAny(row.MinimumSoftware, "<>≤≥~*|") || strings.Contains(strings.ToLower(row.MinimumSoftware), " to "):
		return fmt.Errorf("minimum software %q reads as a range", row.MinimumSoftware)
	}
	for _, alias := range row.WireAliases {
		if !sameApartFromPunctuation(alias.Token, row.Token) || alias.Evidence == "" || alias.Scope == "" {
			return fmt.Errorf("wire alias %q of %q is not a punctuation variant with evidence and scope", alias.Token, row.Token)
		}
	}
	return nil
}

// validateCatalogue enforces every structural and cross-file invariant.
func validateCatalogue(c *catalogue) error {
	if c.Editions.Schema != editionsSchema || c.Models.Schema != modelsSchema || c.Dictionaries.Schema != dictionariesSchema || c.Quarantines.Schema != quarantinesSchema {
		return fmt.Errorf("unexpected schema identifier")
	}
	if c.Editions.Attribution != attributionNote || c.Editions.ReferenceBaseline == "" {
		return fmt.Errorf("the attribution note or the reference baseline is missing")
	}
	editions := map[string]edition{}
	cardEdition := map[string]string{}
	identity := map[string][]edition{}
	for _, record := range c.Editions.Editions {
		if _, exists := editions[record.ID]; exists || record.ID == "" {
			return fmt.Errorf("edition id %q repeated or empty", record.ID)
		}
		if record.Title == "" || record.Issue == "" || record.Date == "" || !validSourceClass[record.SourceClass] || record.ModelRows < 0 {
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
	cardOffsets := map[string]map[int]bool{}
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
			if gateIDs[item.ID] {
				return fmt.Errorf("%s: gate %s repeated", name, item.ID)
			}
			gateIDs[item.ID] = true
		}
		if err := checkRowSet(doc.Rows, false, doc); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := checkRowSet(doc.Controls, true, doc); err != nil {
			return fmt.Errorf("%s controls: %w", name, err)
		}
		if err := checkReadExtents(doc.ReadExtents, doc.Rows); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if support := doc.AdditiveSupport; support != nil {
			if _, ok := editions[support.Edition]; !ok || support.Token == "" || len(support.Rows) == 0 || !floorGrammar.MatchString(support.Minimum) {
				return fmt.Errorf("%s: additive support is incomplete", name)
			}
		}
		offsets := map[int]bool{}
		for _, row := range append(append([]registerRow(nil), doc.Rows...), doc.Controls...) {
			if row.Offset != nil {
				offsets[*row.Offset] = true
			}
		}
		cardOffsets[doc.Card] = offsets
	}
	if len(cardEdition) != len(c.Registers) {
		return fmt.Errorf("%d card editions for %d register files", len(cardEdition), len(c.Registers))
	}
	requirements := map[string]documentRequirement{}
	for _, requirement := range c.Models.DocumentRequirements {
		if _, exists := requirements[requirement.ID]; exists || requirement.ID == "" {
			return fmt.Errorf("document requirement %q repeated or empty", requirement.ID)
		}
		if _, ok := editions[requirement.Edition]; !ok || !requirement.Inclusive || !floorGrammar.MatchString(requirement.Minimum) || requirement.Page < 1 || len(requirement.ModelTokens) == 0 {
			return fmt.Errorf("document requirement %s is not an inclusive floor of a known edition", requirement.ID)
		}
		requirements[requirement.ID] = requirement
	}
	pairs := map[string]bool{}
	tokens := map[string]string{}
	perEdition := map[string]int{}
	for index, row := range c.Models.Rows {
		if _, ok := editions[row.Edition]; !ok {
			return fmt.Errorf("model row %d names unknown edition %s", index, row.Edition)
		}
		perEdition[row.Edition]++
		if err := checkModelRow(row); err != nil {
			return fmt.Errorf("model row %d (%s %s): %w", index, row.Edition, row.Token, err)
		}
		if row.DocumentRequirement != "" {
			requirement, ok := requirements[row.DocumentRequirement]
			if !ok || requirement.Edition != row.Edition || requirement.Minimum != row.MinimumSoftware || !containsString(requirement.ModelTokens, row.Token) {
				return fmt.Errorf("model row %d (%s %s): its document requirement does not cover it", index, row.Edition, row.Token)
			}
		}
		identifier := "sequence " + row.SequenceNumber
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
	for id, record := range editions {
		if perEdition[id] != record.ModelRows {
			return fmt.Errorf("edition %s states %d model rows, %d are present", id, record.ModelRows, perEdition[id])
		}
	}
	dictionaryIDs := map[string]bool{}
	for _, item := range c.Dictionaries.Dictionaries {
		if dictionaryIDs[item.ID] || item.ID == "" || !validKinds[item.Kind] {
			return fmt.Errorf("dictionary %q repeated, empty or of unknown kind", item.ID)
		}
		dictionaryIDs[item.ID] = true
		if cardEdition[item.Card] != item.Edition || (len(item.Offsets) == 0 && item.Kind != kindAlarmBits) || len(item.Pages) == 0 || item.ModePredicate == "" || item.Unlisted == "" || !validEndpoints[item.Endpoint] {
			return fmt.Errorf("dictionary %s is incomplete or not bound to its card edition", item.ID)
		}
		for _, offset := range item.Offsets {
			if !cardOffsets[item.Card][offset] {
				return fmt.Errorf("dictionary %s names offset %d, which card %s does not carry", item.ID, offset, item.Card)
			}
		}
		if (item.Kind == kindAlarmBits) != (len(item.Bits) > 0) || (item.Kind != kindAlarmBits && (len(item.Modules) > 0 || len(item.Values) == 0)) {
			return fmt.Errorf("dictionary %s: bits and modules belong to alarm dictionaries, values to the others", item.ID)
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
			key := bit.Word + "/" + strconv.Itoa(bit.Bit)
			if bit.Offset != nil {
				key = strconv.Itoa(*bit.Offset) + "/" + strconv.Itoa(bit.Bit)
			}
			if bits[key] || (bit.Offset == nil) == (bit.Word == "") || bit.Bit < 0 || bit.Bit > 31 || bit.Label == "" || bit.Page < 1 {
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
		for _, list := range [][]registerRow{doc.Rows, doc.Controls} {
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
