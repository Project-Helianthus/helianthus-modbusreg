package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// encodeGatewayCatalogue returns the canonical bytes of every gateway file.
func encodeGatewayCatalogue(c *gatewayCatalogue) (map[string][]byte, error) {
	files := map[string][]byte{}
	documents := map[string]any{editionsFile: &c.Editions, dictionariesFile: &c.Dictionaries, quarantinesFile: &c.Quarantines, servicesFile: &c.Services}
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

// loadGatewayCatalogue reads and strictly decodes every file of the gateway
// directory. A file the schema does not name, or a directory, is refused.
func loadGatewayCatalogue(directory string) (*gatewayCatalogue, map[string][]byte, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, nil, err
	}
	raw := map[string][]byte{}
	c := &gatewayCatalogue{Registers: map[string]*registersDoc{}}
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
		case name == dictionariesFile:
			target = &c.Dictionaries
		case name == quarantinesFile:
			target = &c.Quarantines
		case name == servicesFile:
			target = &c.Services
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

// conflictSubjects are the topics whose rows are conflict rows; together with
// the two width quarantines they are exactly the quarantines a twin places
// on a read-only row.
var conflictSubjects = map[string]bool{topicRemapLayouts: true, topicEnvironmentScope: true, topicSerialWidth: true}

func validateGatewayEditions(c *gatewayCatalogue) (map[string]edition, error) {
	if c.Editions.Schema != editionsSchema || c.Dictionaries.Schema != dictionariesSchema || c.Quarantines.Schema != quarantinesSchema || c.Services.Schema != servicesSchema {
		return nil, fmt.Errorf("unexpected schema identifier")
	}
	if c.Editions.Attribution != attributionNote || c.Editions.ReferenceBaseline == "" {
		return nil, fmt.Errorf("the attribution note or the reference baseline is missing")
	}
	editions := map[string]edition{}
	identities := map[string]bool{}
	for _, record := range c.Editions.Editions {
		switch {
		case record.ID == "" || editions[record.ID].ID != "":
			return nil, fmt.Errorf("edition id %q repeated or empty", record.ID)
		case record.Title == "" || record.Issue == "" || !isoDatePattern.MatchString(record.Date) || record.SourceClass != sourceClassVendor || record.Card != record.ID:
			return nil, fmt.Errorf("edition %s is incomplete", record.ID)
		case !containsString(gatewayFamilies, record.Family) || !strings.HasPrefix(record.Title, record.Family):
			return nil, fmt.Errorf("edition %s names no gateway family", record.ID)
		case record.ModelRows != 0 || record.Copy != 0 || record.Alias != "":
			return nil, fmt.Errorf("edition %s carries inverter fields", record.ID)
		case len(record.Models) == 0 || record.ModelListExhaustive == nil || len(record.Floors) == 0 || len(record.FloorPages) == 0 || record.SelfUnit == nil:
			return nil, fmt.Errorf("edition %s lacks models, floors, floor pages or its self unit", record.ID)
		case len(record.Namespaces) == 0 || record.RegisterRows < 1:
			return nil, fmt.Errorf("edition %s lacks namespaces or rows", record.ID)
		}
		for _, floor := range record.Floors {
			if floor.Component == "" || !floor.Inclusive || !floorGrammar.MatchString(floor.Minimum) {
				return nil, fmt.Errorf("edition %s: floor %+v is not an inclusive floor in the branch form", record.ID, floor)
			}
		}
		for _, namespace := range record.Namespaces {
			if (!gatewayEndpoints[namespace.Endpoint] && namespace.Endpoint != endpointForwardedInverter) || namespace.Page < 1 || namespace.Rows < 0 ||
				(namespace.Endpoint == endpointForwardedInverter && namespace.Rows != 0) {
				return nil, fmt.Errorf("edition %s: namespace %+v", record.ID, namespace)
			}
		}
		identity := record.Title + "\x00" + record.Issue + "\x00" + record.Date
		if identities[identity] {
			return nil, fmt.Errorf("edition %s repeats a title, issue and date", record.ID)
		}
		identities[identity] = true
		editions[record.ID] = record
	}
	return editions, nil
}

// validateGatewayCatalogue enforces every structural and cross-file invariant
// of the gateway documents.
func validateGatewayCatalogue(c *gatewayCatalogue) error {
	editions, err := validateGatewayEditions(c)
	if err != nil {
		return err
	}
	conflictIDs := map[string]bool{quarantineWidth64: true, quarantineWidth32: true}
	for _, item := range c.Quarantines.Quarantines {
		if conflictSubjects[item.Subject] {
			conflictIDs[item.ID] = true
		}
	}
	if len(conflictIDs) != 2+len(conflictSubjects) {
		return fmt.Errorf("the conflict topics are not all present")
	}
	rowsByCard := map[string]map[int][]registerRow{}
	cardOffsets := map[string]map[int]bool{}
	for name, doc := range c.Registers {
		record, ok := editions[doc.Edition]
		if doc.Schema != registersSchema || name != registersFileName(doc.Card) || !ok || record.Card != doc.Card {
			return fmt.Errorf("%s: schema, card or edition mismatch", name)
		}
		if fmt.Sprint(doc.ModelPages) != fmt.Sprint(record.FloorPages) || len(doc.ReadExtents) != 0 || doc.AdditiveSupport != nil || len(doc.FunctionalClasses) != 0 {
			return fmt.Errorf("%s: model pages, read extents, additive support or functional classes out of place", name)
		}
		gateIDs := map[string]bool{}
		for _, item := range doc.Gates {
			if gateIDs[item.ID] || item.Meaning == "" || len(item.Pages) == 0 {
				return fmt.Errorf("%s: gate %s repeated or incomplete", name, item.ID)
			}
			gateIDs[item.ID] = true
		}
		if err := checkRowSetIn(doc.Rows, false, doc, gatewayEndpoints); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := checkRowSetIn(doc.Controls, true, doc, gatewayEndpoints); err != nil {
			return fmt.Errorf("%s controls: %w", name, err)
		}
		perEndpoint := map[string]int{}
		offsets := map[int]bool{}
		byOffset := map[int][]registerRow{}
		for _, row := range doc.Rows {
			conflict := false
			for _, flag := range row.Quarantine {
				conflict = conflict || conflictIDs[flag]
			}
			switch {
			case row.Key != row.Endpoint+"."+strconv.Itoa(*row.Offset):
				return fmt.Errorf("%s %s: key is not the endpoint and offset", name, rowIdentity(row))
			case (row.Class == classConflict) != conflict:
				return fmt.Errorf("%s %s: a read-only row is a conflict row exactly when a conflict quarantine flags it", name, rowIdentity(row))
			case (row.Class == classRawOnly) != (!conflict && row.Type == opaqueLayoutType):
				return fmt.Errorf("%s %s: a raw-only row is an opaque layout without a conflict", name, rowIdentity(row))
			case len(row.Gates) == 0:
				return fmt.Errorf("%s %s: no endpoint condition", name, rowIdentity(row))
			}
		}
		for _, row := range doc.Controls {
			switch {
			case row.Key != row.Endpoint+"."+strconv.Itoa(*row.Offset):
				return fmt.Errorf("%s %s: key is not the endpoint and offset", name, rowIdentity(row))
			case row.Access == "WO" && row.Class != classNoSend:
				return fmt.Errorf("%s %s: a write-only row is no-send", name, rowIdentity(row))
			case row.Access == "RW" && (row.Class == classReadableConfiguration) != (len(row.Quarantine) == 0):
				return fmt.Errorf("%s %s: a read-write row is readable configuration unless quarantined", name, rowIdentity(row))
			}
		}
		for _, row := range append(append([]registerRow(nil), doc.Rows...), doc.Controls...) {
			perEndpoint[row.Endpoint]++
			offsets[*row.Offset] = true
			byOffset[*row.Offset] = append(byOffset[*row.Offset], row)
		}
		listed := map[string]bool{}
		for _, namespace := range record.Namespaces {
			if namespace.Endpoint != endpointForwardedInverter && perEndpoint[namespace.Endpoint] != namespace.Rows {
				return fmt.Errorf("%s: namespace %s states %d rows, %d are present", name, namespace.Endpoint, namespace.Rows, perEndpoint[namespace.Endpoint])
			}
			listed[namespace.Endpoint] = true
		}
		for endpoint := range perEndpoint {
			if !listed[endpoint] {
				return fmt.Errorf("%s: endpoint %s is not a namespace of its edition", name, endpoint)
			}
		}
		if record.RegisterRows != len(doc.Rows)+len(doc.Controls) {
			return fmt.Errorf("%s: the edition states %d rows, %d are present", name, record.RegisterRows, len(doc.Rows)+len(doc.Controls))
		}
		rowsByCard[doc.Card] = byOffset
		cardOffsets[doc.Card] = offsets
	}
	if len(c.Registers) != len(editions) {
		return fmt.Errorf("%d register files for %d editions", len(c.Registers), len(editions))
	}
	if err := validateGatewayDictionaries(c, editions, rowsByCard, cardOffsets); err != nil {
		return err
	}
	if err := validateServices(c, editions); err != nil {
		return err
	}
	return checkGatewayQuarantines(c, editions)
}

func validateGatewayDictionaries(c *gatewayCatalogue, editions map[string]edition, rowsByCard map[string]map[int][]registerRow, cardOffsets map[string]map[int]bool) error {
	ids := map[string]bool{}
	for _, item := range c.Dictionaries.Dictionaries {
		record, ok := editions[item.Edition]
		switch {
		case ids[item.ID] || item.ID == "" || (item.Kind != kindValueEnum && item.Kind != kindAlarmBits):
			return fmt.Errorf("dictionary %q repeated, empty or of a kind the gateway mode does not write", item.ID)
		case !ok || record.Card != item.Card || !gatewayEndpoints[item.Endpoint] || len(item.Pages) == 0 || item.ModePredicate == "" || item.Unlisted == "":
			return fmt.Errorf("dictionary %s is incomplete or not bound to its edition", item.ID)
		case (item.Kind == kindAlarmBits) != (len(item.Bits) > 0) || (item.Kind == kindValueEnum && (len(item.Values) < 2 || len(item.Offsets) == 0 || len(item.Narratives) > 0)):
			return fmt.Errorf("dictionary %s: bits belong to alarm dictionaries, values to enumerations", item.ID)
		}
		ids[item.ID] = true
		for _, offset := range item.Offsets {
			if !cardOffsets[item.Card][offset] {
				return fmt.Errorf("dictionary %s names offset %d, which its edition does not carry", item.ID, offset)
			}
			if item.Kind != kindValueEnum {
				continue
			}
			decoding := 0
			for _, row := range rowsByCard[item.Card][offset] {
				// Control rows never decode; a dictionary describes the read-only
				// rows of its endpoint at the offset.
				if row.Endpoint != item.Endpoint || row.Access != "RO" {
					continue
				}
				if row.Class != classReviewed && row.Class != classConditional {
					return fmt.Errorf("dictionary %s is bound to %s, which is not a decoding read-only row", item.ID, rowIdentity(row))
				}
				decoding++
			}
			if decoding == 0 {
				return fmt.Errorf("dictionary %s names offset %d without a row of its endpoint", item.ID, offset)
			}
		}
		codes := map[string]bool{}
		for _, value := range item.Values {
			if value.Code == "" || value.Meaning == "" || codes[value.Code] {
				return fmt.Errorf("dictionary %s repeats or empties code %q", item.ID, value.Code)
			}
			codes[value.Code] = true
		}
		for _, bit := range item.Bits {
			if bit.Offset == nil || bit.Word != "" || bit.Cause == nil || bit.Bit < 0 || bit.Bit > 15 || bit.Label == "" || bit.Severity == "" || bit.Page < 1 {
				return fmt.Errorf("dictionary %s has an invalid bit %+v", item.ID, bit)
			}
		}
		if err := checkCandidateSets(item.Bits); err != nil {
			return fmt.Errorf("dictionary %s: %w", item.ID, err)
		}
		for _, bit := range item.Bits {
			if !hasFlag(bit.Quarantine, quarantineDictionaryOnly) && !cardOffsets[item.Card][*bit.Offset] {
				return fmt.Errorf("dictionary %s bit at %d has no register row and no dictionary-only flag", item.ID, *bit.Offset)
			}
		}
		for _, note := range item.Narratives {
			if note.AlarmID == "" || note.Label == "" || note.Page < 1 {
				return fmt.Errorf("dictionary %s has an incomplete narrative", item.ID)
			}
		}
	}
	return nil
}

func validateServices(c *gatewayCatalogue, editions map[string]edition) error {
	services := c.Services
	if services.DeviceListGrammar == "" || len(services.Functions) == 0 || len(services.Exceptions) == 0 || len(services.ListObjects) == 0 ||
		len(services.Attributes) == 0 || len(services.Limits) == 0 {
		return fmt.Errorf("services document is incomplete")
	}
	meanings := map[string]bool{}
	for _, item := range services.Exceptions {
		if item.Code == "" || item.Meaning == "" || len(item.Families) == 0 || meanings[item.Code] {
			return fmt.Errorf("exception %q repeated or incomplete", item.Code)
		}
		meanings[item.Code] = true
	}
	for _, item := range services.Functions {
		if item.Code == "" || item.Purpose == "" || (item.Status != functionListed && item.Status != functionNotEstablished) {
			return fmt.Errorf("function %q incomplete", item.Code)
		}
	}
	for index, item := range services.Attributes {
		if item.Number != index+1 || item.Meaning == "" {
			return fmt.Errorf("attribute %d out of order or without meaning", item.Number)
		}
	}
	limitIDs := map[string]bool{}
	for _, item := range services.Limits {
		if item.ID == "" || limitIDs[item.ID] || item.Rule == "" || (item.Origin != limitStandard && item.Origin != limitVendor && item.Origin != limitClientPolicy) {
			return fmt.Errorf("limit %q repeated or incomplete", item.ID)
		}
		limitIDs[item.ID] = true
	}
	seen := map[string]bool{}
	for _, entry := range services.Editions {
		record, ok := editions[entry.Edition]
		switch {
		case !ok || seen[entry.Edition]:
			return fmt.Errorf("services entry %q names no edition or repeats one", entry.Edition)
		case entry.SelfUnit != *record.SelfUnit || entry.ChildUnits == "" || entry.ReservedUnits == "" || entry.ByteOrder == "":
			return fmt.Errorf("services entry %s disagrees with its edition or lacks units", entry.Edition)
		case len(entry.Functions) == 0 || len(entry.ExceptionCodes) == 0 || len(entry.ExceptionPages) != 2 || entry.RequestObject == "" || entry.Attribute6 == "" || entry.DescriptorAttributes == "":
			return fmt.Errorf("services entry %s is incomplete", entry.Edition)
		case len(entry.Pages) == 0:
			return fmt.Errorf("services entry %s has no section pages", entry.Edition)
		}
		seen[entry.Edition] = true
		for _, code := range entry.ExceptionCodes {
			if !meanings[code] {
				return fmt.Errorf("services entry %s lists exception %s without a meaning", entry.Edition, code)
			}
		}
		for _, page := range entry.Pages {
			if page.Contract == "" || page.First < 1 || page.Last < page.First {
				return fmt.Errorf("services entry %s has invalid pages %+v", entry.Edition, page)
			}
		}
		for _, rule := range entry.Spacing {
			if rule.MinimumMS < 1 || rule.Page < 1 || (rule.Basis != basisRequired && rule.Basis != basisAdvisory) || (rule.Between != "query" && rule.Between != "control") {
				return fmt.Errorf("services entry %s has an invalid spacing rule", entry.Edition)
			}
		}
		for _, period := range entry.QueryPeriods {
			if period.Equipment == "" || period.Seconds == "" || period.Page < 1 || period.Basis != basisAdvisory {
				return fmt.Errorf("services entry %s has an invalid query period", entry.Edition)
			}
		}
	}
	if len(seen) != len(editions) {
		return fmt.Errorf("%d services entries for %d editions", len(seen), len(editions))
	}
	return nil
}

// checkGatewayQuarantines verifies that the affected rows, services entries
// and alarm bits of every quarantine are exactly the ones that carry its flag.
func checkGatewayQuarantines(c *gatewayCatalogue, editions map[string]edition) error {
	builder := newQuarantineBuilder()
	for _, item := range c.Quarantines.Quarantines {
		if _, exists := builder.items[item.ID]; exists || !validOrigins[item.Origin] || item.Subject == "" || item.Meaning == "" || item.Editions == nil || item.Affected == nil {
			return fmt.Errorf("quarantine %q repeated or incomplete", item.ID)
		}
		for _, editionID := range item.Editions {
			if _, ok := editions[editionID]; !ok {
				return fmt.Errorf("quarantine %s names unknown edition %s", item.ID, editionID)
			}
		}
		builder.add(quarantine{ID: item.ID, Origin: item.Origin, Subject: item.Subject, Evidence: item.Evidence, Meaning: item.Meaning})
	}
	known := func(flag string) error {
		if _, ok := builder.items[flag]; !ok {
			return fmt.Errorf("unknown quarantine %s", flag)
		}
		return nil
	}
	for _, doc := range c.Registers {
		for _, list := range [][]registerRow{doc.Rows, doc.Controls} {
			for index := range list {
				row := list[index]
				for _, flag := range row.Quarantine {
					if err := known(flag); err != nil {
						return fmt.Errorf("%s %s: %w", doc.Card, rowIdentity(row), err)
					}
					builder.flagRegister(flag, doc.Card, doc.Edition, &row)
				}
			}
		}
	}
	for _, entry := range c.Services.Editions {
		for _, flag := range entry.Quarantine {
			if err := known(flag); err != nil {
				return fmt.Errorf("services %s: %w", entry.Edition, err)
			}
			reference := affectedRow{Set: affectedServices, Edition: entry.Edition}
			builder.affected[flag][affectedIdentity(reference)] = reference
			builder.edition(flag, entry.Edition)
		}
	}
	for _, item := range c.Dictionaries.Dictionaries {
		for _, bit := range item.Bits {
			for _, flag := range bit.Quarantine {
				if err := known(flag); err != nil {
					return fmt.Errorf("%s: %w", item.ID, err)
				}
				reference := affectedRow{Set: affectedAlarms, Card: item.Card, Offset: bit.Offset, Key: alarmKey(bit), Page: bit.Page}
				builder.affected[flag][affectedIdentity(reference)] = reference
				builder.edition(flag, item.Edition)
			}
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
			return fmt.Errorf("quarantine %s lists %d affected entries, %d carry its flag", item.ID, len(item.Affected), len(rebuilt[index].Affected))
		}
		for position := range item.Affected {
			if affectedIdentity(item.Affected[position]) != affectedIdentity(rebuilt[index].Affected[position]) {
				return fmt.Errorf("quarantine %s affected entry %d disagrees with the flags", item.ID, position)
			}
		}
		if strings.Join(rebuilt[index].Editions, ",") != strings.Join(item.Editions, ",") {
			return fmt.Errorf("quarantine %s lists editions %v, its flags name %v", item.ID, item.Editions, rebuilt[index].Editions)
		}
	}
	return nil
}

// guardGatewayCatalogue applies the publication guard to every committed
// string, and refuses source identifiers of the reference.
func guardGatewayCatalogue(c *gatewayCatalogue) error {
	documents := map[string]any{editionsFile: c.Editions, dictionariesFile: c.Dictionaries, quarantinesFile: c.Quarantines, servicesFile: c.Services}
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
		var problem error
		walkStrings(documents[name], func(path, text string) {
			if problem == nil && sourceIDTokenPattern.MatchString(text) {
				problem = fmt.Errorf("%s %s names a source identifier of the reference: %q", name, path, text)
			}
		})
		if problem != nil {
			return problem
		}
	}
	return nil
}
