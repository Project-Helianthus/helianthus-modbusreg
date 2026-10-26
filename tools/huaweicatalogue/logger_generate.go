package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type gatewayOptions struct {
	Reference string
	Expect    gatewayCounts
}

type gatewayResult struct {
	Catalogue *gatewayCatalogue
	Files     map[string][]byte
	Sidecar   map[string][]byte
	Counts    gatewayCounts
}

type gatewayIndex struct {
	Baseline     string `json:"baseline"`
	EditionCount int    `json:"edition_count"`
	Editions     []struct {
		Alarms       int    `json:"alarm_record_count"`
		Dictionaries int    `json:"dictionary_count"`
		Path         string `json:"path"`
		Registers    int    `json:"register_count"`
		SourceID     string `json:"source_id"`
		Title        string `json:"title"`
	} `json:"editions"`
	ObservationFile string `json:"observation_file"`
	RegisterCount   int    `json:"register_count"`
	Schema          string `json:"schema"`
}

var gatewayBaselinePattern = regexp.MustCompile("Baseline: `(\\d{4}-\\d{2}-\\d{2}\\.\\d+)`")

// gatewayBuild is everything the generator keeps for one edition.
type gatewayBuild struct {
	SourceID string
	Twin     *gatewayTwin
	Card     *profileCard
	Edition  *edition
	Label    string
	Rows     []gatewayRow
	Controls []gatewayRow
	Doc      *registersDoc
	Alarms   []*alarmEntry
	Service  *serviceEdition
}

type alarmEntry struct {
	Bit      alarmBit
	RecordID string
	Private  map[string]string
}

// Reference labels of the topic table that bind quarantines to rows or
// services. Every other topic is carried without bound rows.
const (
	topicRoutingOverlap   = "Routing overlap"
	topicCRCRows          = "TCP CRC rows"
	topicShiftedBasic     = "Shifted Basic response"
	topicZeroCount        = "Zero FC10 count"
	topicFrameMismatch    = "Frame/count mismatch"
	topicExceptionByte    = "Issue 49 exception byte"
	topicObjectNumbering  = "Issue 49 object numbering"
	topicRemapLayouts     = "Conflicting remap layouts"
	topicSerialWidth      = "Issue 49 serial width"
	topicPresence         = "Document differences"
	topicOpaqueMeter      = "Opaque meter identity"
	topicPacing           = "Pacing editions"
	topicEnvironmentScope = "Environmental remap scope"
)

const (
	quarantineWidth64        = "W01"
	quarantineWidth32        = "W02"
	quarantineAttributeSix   = "G01"
	quarantineAlarmCollision = "A01"
	quarantineAlarmLocations = "A02"
	quarantineDictionaryOnly = "A03"
	affectedServices         = "services"
	affectedAlarms           = "alarms"
	shiftedBasicEdition      = "s-dongle-v100r001c00-05-2022-07-06"
	presencePage             = 11
)

var presenceOffsets = []int{37502, 37504, 37516}

// advisoryWording renders the reference's word for vendor advice as
// "advisory": the publication guard reserves that word for operational-review
// wording, and the pacing topic would otherwise lose its only sentence.
var advisoryWording = strings.NewReplacer("recommended ", "advisory ")

// generateGateway builds the gateway catalogue from the gateway reference
// baseline and verifies it against the baseline's own accounting and the
// expected counts.
func generateGateway(opts gatewayOptions) (*gatewayResult, error) {
	in := reader{root: opts.Reference}
	readme, err := in.text("README.md")
	if err != nil {
		return nil, err
	}
	baselineMatch := gatewayBaselinePattern.FindStringSubmatch(readme)
	if baselineMatch == nil {
		return nil, fmt.Errorf("the reference states no baseline")
	}
	baseline := baselineMatch[1]
	indexData, err := in.bytes("data/index.json")
	if err != nil {
		return nil, err
	}
	indexData, err = withoutLocatorKey(indexData)
	if err != nil {
		return nil, fmt.Errorf("index: %w", err)
	}
	var index gatewayIndex
	if err := decodeStrict(indexData, &index); err != nil {
		return nil, fmt.Errorf("index: %w", err)
	}
	if index.Schema != gatewayIndexSchema || index.Baseline != baseline || index.EditionCount != len(index.Editions) {
		return nil, fmt.Errorf("index: schema, baseline or edition count mismatch")
	}
	comparison, err := readText(in, "appendices/protocol-editions.md", parseProtocolEditions)
	if err != nil {
		return nil, err
	}
	wire, err := readText(in, "08-services.md", parseWireChapter)
	if err != nil {
		return nil, err
	}
	discovery, err := readText(in, "03-discovery.md", parseDiscoveryChapter)
	if err != nil {
		return nil, err
	}
	connection, err := in.text("01-connection.md")
	if err != nil {
		return nil, err
	}
	limits, err := frameLimits(connection)
	if err != nil {
		return nil, err
	}
	contradictionText, err := in.text("appendices/contradictions.md")
	if err != nil {
		return nil, err
	}
	contradictions, err := parseContradictions(contradictionText)
	if err != nil {
		return nil, err
	}
	discoveryText, err := in.text("03-discovery.md")
	if err != nil {
		return nil, err
	}
	if _, err := requireAnchor("discovery chapter", discoveryText, attributeSixAnchor); err != nil {
		return nil, err
	}

	var builds []*gatewayBuild
	rowTotal := 0
	labels := map[string]string{}
	for _, entry := range index.Editions {
		build, err := buildGatewayEdition(in, entry.Path, baseline, connection, comparison)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.SourceID, err)
		}
		twin := build.Twin
		if twin.SourceID != entry.SourceID || twin.Title != entry.Title || twin.RegisterCount != entry.Registers ||
			len(twin.Registers) != entry.Registers || len(twin.Dictionaries) != entry.Dictionaries || len(twin.AlarmDictionary) != entry.Alarms {
			return nil, fmt.Errorf("%s: the twin disagrees with the index accounting", entry.SourceID)
		}
		rowTotal += len(twin.Registers)
		labels[build.SourceID] = build.Label
		builds = append(builds, build)
	}
	if rowTotal != index.RegisterCount {
		return nil, fmt.Errorf("%d register rows, the index states %d", rowTotal, index.RegisterCount)
	}

	c := &gatewayCatalogue{Registers: map[string]*registersDoc{}}
	c.Editions = editionsDoc{Schema: editionsSchema, ReferenceBaseline: baseline, Attribution: attributionNote}
	var dictionaries []dictionary
	for _, build := range builds {
		c.Editions.Editions = append(c.Editions.Editions, *build.Edition)
		c.Registers[registersFileName(build.Edition.Card)] = build.Doc
		items, err := gatewayDictionaries(build)
		if err != nil {
			return nil, fmt.Errorf("%s dictionaries: %w", build.SourceID, err)
		}
		dictionaries = append(dictionaries, items...)
	}
	sort.Slice(c.Editions.Editions, func(i, j int) bool { return c.Editions.Editions[i].ID < c.Editions.Editions[j].ID })

	quarantines, err := gatewayQuarantines(builds, contradictions, labels)
	if err != nil {
		return nil, err
	}
	// Alarm flags are set by the quarantine pass; the alarm dictionaries are
	// written afterwards so that the bits carry them.
	for _, build := range builds {
		item, ok, err := alarmDictionary(build)
		if err != nil {
			return nil, fmt.Errorf("%s alarms: %w", build.SourceID, err)
		}
		if ok {
			dictionaries = append(dictionaries, item)
		}
	}
	sort.SliceStable(dictionaries, func(i, j int) bool {
		if dictionaries[i].Card != dictionaries[j].Card {
			return dictionaries[i].Card < dictionaries[j].Card
		}
		return dictionaries[i].ID < dictionaries[j].ID
	})
	c.Dictionaries = dictionariesDoc{Schema: dictionariesSchema, Dictionaries: dictionaries}
	c.Quarantines = quarantinesDoc{Schema: quarantinesSchema, Quarantines: quarantines}
	for _, build := range builds {
		build.Doc.Rows, build.Doc.Controls = []registerRow{}, []registerRow{}
		for _, row := range build.Rows {
			build.Doc.Rows = append(build.Doc.Rows, row.Row)
		}
		for _, row := range build.Controls {
			build.Doc.Controls = append(build.Doc.Controls, row.Row)
		}
	}
	c.Services, err = gatewayServices(builds, wire, discovery, limits)
	if err != nil {
		return nil, err
	}

	got := countGatewayCatalogue(c)
	alarmTotal := 0
	for _, entry := range index.Editions {
		alarmTotal += entry.Alarms
	}
	if got.Rows != index.RegisterCount || got.Editions != index.EditionCount || got.Alarms != alarmTotal {
		return nil, fmt.Errorf("internal accounting mismatch: %s against %d rows, %d editions and %d alarm records stated", got, index.RegisterCount, index.EditionCount, alarmTotal)
	}
	if got != opts.Expect {
		return nil, fmt.Errorf("counts %s differ from the expected baseline accounting %s; review the reference change and rerun with -expect %s", got, opts.Expect, got)
	}
	if err := validateGatewayCatalogue(c); err != nil {
		return nil, err
	}
	if err := guardGatewayCatalogue(c); err != nil {
		return nil, err
	}
	files, err := encodeGatewayCatalogue(c)
	if err != nil {
		return nil, err
	}
	locators, err := in.bytes("data/source-locators.json")
	if err != nil {
		return nil, err
	}
	sidecar, err := gatewaySidecar(builds, locators, got)
	if err != nil {
		return nil, err
	}
	return &gatewayResult{Catalogue: c, Files: files, Sidecar: sidecar, Counts: got}, nil
}

func readText[T any](in reader, relative string, parse func(string) (T, error)) (T, error) {
	var zero T
	text, err := in.text(relative)
	if err != nil {
		return zero, err
	}
	value, err := parse(text)
	if err != nil {
		return zero, fmt.Errorf("%s: %w", relative, err)
	}
	return value, nil
}

// buildGatewayEdition reads one twin and its profile card and converts the
// edition record and its register rows.
func buildGatewayEdition(in reader, path, baseline, connection string, comparison map[string]*editionComparison) (*gatewayBuild, error) {
	data, err := in.bytes("data/" + path)
	if err != nil {
		return nil, err
	}
	twin, err := decodeGatewayTwin(data)
	if err != nil {
		return nil, err
	}
	policy := twin.IntegrationPolicy
	if twin.Schema != gatewayTwinSchema || twin.Baseline != baseline || twin.Protocol.SourceID != twin.SourceID ||
		!policy.MinimumIsFloor || !policy.CitationKeyOnly || policy.SilenceRetracts {
		return nil, fmt.Errorf("schema, baseline, protocol source or floor policy mismatch")
	}
	cardText, err := in.text("profiles/" + strings.ToLower(twin.SourceID) + ".md")
	if err != nil {
		return nil, err
	}
	card, err := parseProfileCard(cardText)
	if err != nil {
		return nil, fmt.Errorf("profile card: %w", err)
	}
	build := &gatewayBuild{SourceID: twin.SourceID, Twin: twin, Card: card}
	if err := buildGatewayRecord(build); err != nil {
		return nil, err
	}
	if err := checkFactSummaries(twin); err != nil {
		return nil, err
	}
	for order, register := range twin.Registers {
		row, err := convertGatewayRegister(register, twin.SourceID, order)
		if err != nil {
			return nil, err
		}
		if register.Access == "RO" {
			build.Rows = append(build.Rows, row)
		} else {
			build.Controls = append(build.Controls, row)
		}
	}
	sortGatewayRows(build.Rows)
	sortGatewayRows(build.Controls)
	if err := buildRegisterDoc(build); err != nil {
		return nil, err
	}
	if err := convertAlarms(build); err != nil {
		return nil, err
	}
	service, err := buildServiceEdition(build, connection, comparison[twin.SourceID])
	if err != nil {
		return nil, err
	}
	build.Service = service
	return build, nil
}

// checkFactSummaries verifies that every row's cell note is the factual
// summary its twin dictionary states, and that every summary is bound to
// exactly the rows that name it.
func checkFactSummaries(twin *gatewayTwin) error {
	summaries := map[string]gatewayFactSummary{}
	for _, item := range twin.Dictionaries {
		if _, exists := summaries[item.ID]; exists || item.Summary == "" || len(item.RecordIDs) == 0 {
			return fmt.Errorf("factual summary %s repeated, empty or unbound", item.ID)
		}
		summaries[item.ID] = item
	}
	named := map[string]string{}
	for _, register := range twin.Registers {
		item, ok := summaries[register.Conditions.DetailID]
		if !ok || item.Summary != register.Conditions.Summary {
			return fmt.Errorf("%s: cell note differs from factual summary %s", register.RecordID, register.Conditions.DetailID)
		}
		named[register.RecordID] = register.Conditions.DetailID
	}
	listed := 0
	for _, item := range summaries {
		for _, record := range item.RecordIDs {
			if named[record] != item.ID {
				return fmt.Errorf("factual summary %s names %s, which does not carry it", item.ID, record)
			}
			listed++
		}
	}
	if listed != len(twin.Registers) {
		return fmt.Errorf("factual summaries list %d rows, the twin has %d", listed, len(twin.Registers))
	}
	return nil
}

// buildGatewayRecord converts the edition identity, applicability and
// namespaces of one twin.
func buildGatewayRecord(build *gatewayBuild) error {
	twin, card := build.Twin, build.Card
	family, err := gatewayFamily(twin.Title)
	if err != nil {
		return err
	}
	cover := twin.Edition.CoverVersion
	stated := cover != coverNotStated
	issue := floorNotStated
	switch {
	case twoDigitIssuePattern.MatchString(twin.Edition.Issue):
		issue = "Issue " + twin.Edition.Issue
	case twin.Edition.Issue == cover:
		// The cover prints its branch where an issue would stand.
	default:
		return fmt.Errorf("issue %q", twin.Edition.Issue)
	}
	if !isoDatePattern.MatchString(twin.Edition.Date) {
		return fmt.Errorf("date %q", twin.Edition.Date)
	}
	// The reference labels an edition by family, cover branch (or the word
	// legacy where the cover prints none) and issue.
	referenceLabel := family + " legacy"
	title := family
	if stated {
		referenceLabel = family + " " + cover
		title = family + " " + cover
	}
	if issue != floorNotStated {
		referenceLabel += " / " + issue
	}
	if twin.Title != referenceLabel && !strings.HasPrefix(twin.Title, referenceLabel+" (") {
		return fmt.Errorf("title %q does not follow the edition identity", twin.Title)
	}
	id := slug(family)
	if stated {
		id += "-" + slug(cover)
	}
	if issue != floorNotStated {
		id += "-" + twin.Edition.Issue
	}
	id += "-" + twin.Edition.Date
	floors, err := parseFloors(twin.ModelScope)
	if err != nil {
		return err
	}
	if card.Requirements != twin.ModelScope.SoftwareRequirements {
		return fmt.Errorf("profile card requirements disagree with the twin")
	}
	self := twin.Protocol.Transport.Units.Self
	if self == nil || *self != card.SelfUnit {
		return fmt.Errorf("self unit disagrees between twin and profile card")
	}
	if len(twin.ModelScope.Entries) == 0 || len(twin.ModelScope.RequirementPages) == 0 {
		return fmt.Errorf("model scope is empty")
	}
	exhaustive := twin.ModelScope.Exhaustive
	record := &edition{
		ID: id, Title: title, Issue: issue, Date: twin.Edition.Date, Family: family, SourceClass: sourceClassVendor, Card: id,
		Models: append([]string(nil), twin.ModelScope.Entries...), ModelListExhaustive: &exhaustive, Floors: floors,
		FloorPages: distinctSorted(twin.ModelScope.RequirementPages), SelfUnit: intPointer(*self), RegisterRows: len(twin.Registers),
	}
	if stated {
		record.Branch = cover
	}
	build.Edition = record
	build.Label = title
	if issue != floorNotStated {
		build.Label += " " + issue
	}
	return nil
}

func sortGatewayRows(rows []gatewayRow) {
	parsed := make([]parsedRow, len(rows))
	extra := map[int]gatewayRow{}
	for index, row := range rows {
		parsed[index] = row.parsedRow
		extra[row.Order] = row
	}
	sortRows(parsed)
	for index := range parsed {
		row := extra[parsed[index].Order]
		row.parsedRow = parsed[index]
		rows[index] = row
	}
}

// buildRegisterDoc derives the gates, namespaces and register document of
// one edition. Each endpoint's inherited conditions become gates named after
// the endpoint; each row predicate becomes a gate named after its kind and
// value.
func buildRegisterDoc(build *gatewayBuild) error {
	gates := map[string]gate{}
	endpointNotes := map[string][]string{}
	endpointPages := map[string][]int{}
	endpointRows := map[string]int{}
	register := func(rows []gatewayRow) error {
		for index := range rows {
			row := &rows[index]
			endpoint := row.Row.Endpoint
			notes := build.Twin.Registers[row.Order].Conditions.Namespace
			if previous, ok := endpointNotes[endpoint]; ok && strings.Join(previous, "\x00") != strings.Join(notes, "\x00") {
				return fmt.Errorf("%s: endpoint %s carries two sets of inherited conditions", row.Evidence, endpoint)
			}
			endpointNotes[endpoint] = notes
			endpointPages[endpoint] = append(endpointPages[endpoint], row.Row.Page)
			endpointRows[endpoint]++
			for position := range notes {
				row.Row.Gates = append(row.Row.Gates, endpoint+"-"+strconv.Itoa(position+1))
			}
			for _, predicate := range row.RowSpecific {
				id := slug(predicate.Kind + " " + predicate.Value)
				item := gate{ID: id, Meaning: gatewayProse(predicate.Summary), Pages: []int{predicate.Page}, Kind: predicate.Kind, Operator: predicate.Operator, Value: predicate.Value}
				if previous, ok := gates[id]; ok && fmt.Sprint(previous) != fmt.Sprint(item) {
					return fmt.Errorf("%s: predicate %s differs between rows", row.Evidence, id)
				}
				gates[id] = item
				row.Row.Gates = append(row.Row.Gates, id)
			}
		}
		return nil
	}
	if err := register(build.Rows); err != nil {
		return err
	}
	if err := register(build.Controls); err != nil {
		return err
	}
	for endpoint, notes := range endpointNotes {
		for position, note := range notes {
			meaning := gatewayProse(note)
			if meaning == "" {
				return fmt.Errorf("endpoint %s condition %d is empty after cleaning", endpoint, position+1)
			}
			gates[endpoint+"-"+strconv.Itoa(position+1)] = gate{ID: endpoint + "-" + strconv.Itoa(position+1), Meaning: meaning, Pages: distinctSorted(endpointPages[endpoint])}
		}
	}
	ids := make([]string, 0, len(gates))
	for id := range gates {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	doc := &registersDoc{
		Schema: registersSchema, Card: build.Edition.Card, Edition: build.Edition.ID, ModelPages: build.Edition.FloorPages,
		SelectionNote: gatewayProse(build.Card.Note), ReadExtents: []readExtent{},
	}
	for _, id := range ids {
		doc.Gates = append(doc.Gates, gates[id])
	}
	build.Doc = doc

	// A namespace carries the physical start page of its source section as
	// the profile card states it; its rows begin on that page or after it.
	seen := map[string]bool{}
	for _, namespace := range build.Card.Namespaces {
		remapped := namespace.Label == "Remapped child values"
		for _, endpoint := range cardNamespaceEndpoints[namespace.Label] {
			if endpointRows[endpoint] > 0 && distinctSorted(endpointPages[endpoint])[0] < namespace.Page {
				return fmt.Errorf("%s rows start on page %d, before the section start page %d", endpoint, distinctSorted(endpointPages[endpoint])[0], namespace.Page)
			}
			if remapped && endpointRows[endpoint] == 0 {
				continue
			}
			build.Edition.Namespaces = append(build.Edition.Namespaces, namespaceRef{Endpoint: endpoint, Page: namespace.Page, Rows: endpointRows[endpoint]})
			seen[endpoint] = true
		}
	}
	for endpoint := range endpointRows {
		if !seen[endpoint] {
			return fmt.Errorf("endpoint %s has rows but no profile-card namespace", endpoint)
		}
	}
	return nil
}

// convertAlarms converts the alarm tuples and narratives of one edition.
// Inherited cells and record identifiers stay private.
func convertAlarms(build *gatewayBuild) error {
	for _, alarm := range build.Twin.AlarmDictionary {
		if alarm.SourceID != build.SourceID || alarm.Namespace != "gateway_self" || alarm.Label == "" || alarm.Locator.Page < 1 {
			return fmt.Errorf("alarm %s is not a gateway-self record of the edition", alarm.RecordID)
		}
		name, ok := alarm.Semantic["name"]
		if !ok || name.Summary != alarm.Label {
			return fmt.Errorf("alarm %s: label and name summary differ", alarm.RecordID)
		}
		private := map[string]string{"record": alarm.RecordID, "section": alarm.SectionID, "table": strconv.Itoa(alarm.Locator.Table), "row": strconv.Itoa(alarm.Locator.Row)}
		for field, record := range alarm.Inherited {
			private["inherited_"+field] = record
		}
		switch alarm.Kind {
		case alarmTuple:
			condition, ok := alarm.Semantic["condition"]
			if alarm.Address == nil || alarm.Bit == nil || alarm.AlarmID == nil || alarm.Cause == nil || alarm.Severity == nil || !ok || len(alarm.Semantic) != 2 ||
				strconv.Itoa(*alarm.AlarmID) != alarm.AlarmLiteral {
				return fmt.Errorf("alarm %s is an incomplete tuple", alarm.RecordID)
			}
			build.Alarms = append(build.Alarms, &alarmEntry{RecordID: alarm.RecordID, Private: private, Bit: alarmBit{
				Offset: intPointer(*alarm.Address), Bit: *alarm.Bit, AlarmID: *alarm.AlarmID, Cause: intPointer(*alarm.Cause),
				Label: cleanText(alarm.Label), Severity: *alarm.Severity, Module: modeNoneStated, Page: alarm.Locator.Page,
				Condition: gatewayProse(condition.Summary),
			}})
		case alarmNarrative:
			if alarm.Address != nil || alarm.Bit != nil || len(alarm.Semantic) != 3 {
				return fmt.Errorf("alarm %s is an unexpected narrative", alarm.RecordID)
			}
		default:
			return fmt.Errorf("alarm %s has kind %q", alarm.RecordID, alarm.Kind)
		}
	}
	return nil
}

// gatewayDictionaries derives the value enumerations of one edition: a cell
// note whose first sentence is a code list becomes one dictionary per
// endpoint and note, bound to every reviewed or conditional read-only row
// that carries the note. Raw-only, conflict and control rows get none.
func gatewayDictionaries(build *gatewayBuild) ([]dictionary, error) {
	type key struct{ endpoint, detail string }
	groups := map[key]*dictionary{}
	var order []key
	for _, row := range build.Rows {
		if row.Row.Class != classReviewed && row.Row.Class != classConditional {
			continue
		}
		values := codeList(row.Summary)
		if values == nil {
			continue
		}
		k := key{row.Row.Endpoint, row.DetailID}
		item, ok := groups[k]
		if !ok {
			item = &dictionary{Kind: kindValueEnum, Card: build.Edition.Card, Edition: build.Edition.ID, Endpoint: row.Row.Endpoint,
				ModePredicate: modeNoneStated, Values: values, Unlisted: unlistedRaw}
			groups[k] = item
			order = append(order, k)
		}
		item.Offsets = append(item.Offsets, *row.Row.Offset)
		item.Pages = append(item.Pages, row.Row.Page)
	}
	var out []dictionary
	for _, k := range order {
		item := groups[k]
		item.Offsets, item.Pages = distinctSorted(item.Offsets), distinctSorted(item.Pages)
		item.ID = build.Edition.ID + "." + item.Endpoint + ".value-enum." + strconv.Itoa(item.Offsets[0])
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	for index := 1; index < len(out); index++ {
		if out[index].ID == out[index-1].ID {
			return nil, fmt.Errorf("two enumerations share the identifier %s", out[index].ID)
		}
	}
	return out, nil
}

// alarmDictionary writes the alarm dictionary of one edition. Bits whose
// alarm register is a raw-only or conflict row are withheld.
func alarmDictionary(build *gatewayBuild) (dictionary, bool, error) {
	if len(build.Twin.AlarmDictionary) == 0 {
		return dictionary{}, false, nil
	}
	rowClass := map[int]string{}
	for _, row := range build.Rows {
		if row.Row.Endpoint == endpointGatewaySelf {
			rowClass[*row.Row.Offset] = row.Row.Class
		}
	}
	item := dictionary{
		ID: build.Edition.ID + ".alarm-bits", Kind: kindAlarmBits, Card: build.Edition.Card, Edition: build.Edition.ID, Endpoint: endpointGatewaySelf,
		ModePredicate: modeNoneStated, Unlisted: unlistedUnknown,
	}
	var offsets, pages []int
	for _, entry := range build.Alarms {
		offset := *entry.Bit.Offset
		if class := rowClass[offset]; class == classRawOnly || class == classConflict {
			continue
		}
		if _, ok := rowClass[offset]; ok {
			offsets = append(offsets, offset)
		}
		pages = append(pages, entry.Bit.Page)
		item.Bits = append(item.Bits, entry.Bit)
	}
	for _, alarm := range build.Twin.AlarmDictionary {
		if alarm.Kind != alarmNarrative {
			continue
		}
		pages = append(pages, alarm.Locator.Page)
		item.Narratives = append(item.Narratives, alarmNote{
			AlarmID: alarm.AlarmLiteral, Label: cleanText(alarm.Label), Description: gatewayProse(alarm.Semantic["description"].Summary),
			Impact: gatewayProse(alarm.Semantic["impact"].Summary), Page: alarm.Locator.Page,
		})
	}
	item.Offsets, item.Pages = distinctSorted(offsets), distinctSorted(pages)
	if len(item.Bits) == 0 {
		return dictionary{}, false, fmt.Errorf("alarm dictionary without bits")
	}
	return item, true, nil
}

// gatewayQuarantines builds the quarantine list and sets the flags on the
// affected rows, services entries and alarm bits.
func gatewayQuarantines(builds []*gatewayBuild, tables *contradictionTables, labels map[string]string) ([]quarantine, error) {
	builder := newQuarantineBuilder()
	topicIDs := map[string]string{}
	for index, topic := range tables.Topics {
		id := fmt.Sprintf("T%02d", index+1)
		meaning, err := withEditionLabels(advisoryWording.Replace(cleanText(topic[1])), labels)
		if err != nil {
			return nil, fmt.Errorf("topic %q: %w", topic[0], err)
		}
		meaning = gatewayProse(meaning)
		if meaning == "" {
			return nil, fmt.Errorf("topic %q has no meaning after cleaning", topic[0])
		}
		builder.add(quarantine{ID: id, Origin: originReference, Subject: topic[0], Meaning: meaning})
		topicIDs[topic[0]] = id
	}
	for _, name := range []string{topicRoutingOverlap, topicCRCRows, topicShiftedBasic, topicZeroCount, topicFrameMismatch, topicExceptionByte,
		topicObjectNumbering, topicRemapLayouts, topicSerialWidth, topicPresence, topicOpaqueMeter, topicPacing, topicEnvironmentScope} {
		if topicIDs[name] == "" {
			return nil, fmt.Errorf("the topic table lacks %q", name)
		}
	}
	builder.add(quarantine{ID: quarantineWidth64, Origin: originReference, Subject: "40446, 40450, 40454 and 40458 printed as I64 with two words in the SPC275-container edition",
		Evidence: "register type and layout conflict table; four-word rows of other editions are facts in their own scope and do not repair these rows", Meaning: gatewayProse(conflictTableMeaning)})
	builder.add(quarantine{ID: quarantineWidth32, Origin: originReference, Subject: "30054 and 30058 printed as U32 with four words in Issue 49",
		Evidence: "register type and layout conflict table", Meaning: gatewayProse(conflictTableMeaning)})
	builder.add(quarantine{ID: quarantineAttributeSix, Origin: originReference, Subject: "Attribute 6 grammar of the private device list",
		Evidence: "attribute grammar of the discovery chapter and the edition comparison",
		Meaning:  "Attribute 6 is a parallel-network integer in the S-Dongle editions and in SmartLogger Issues 32, 36, 37 and 49, a feature-version integer in the other SmartLogger editions and a feature-version string in the EMMA editions. No global decoder applies to attribute 6; each edition's services entry names its own grammar."})
	builder.add(quarantine{ID: quarantineAlarmCollision, Origin: originReference, Subject: "Colliding alarm bits carried as candidate sets", Evidence: "alarm mapping limits", Meaning: gatewayProse(alarmCollisionMeaning)})
	builder.add(quarantine{ID: quarantineAlarmLocations, Origin: originReference, Subject: "One alarm and cause printed at two bit locations", Evidence: "alarm mapping limits", Meaning: gatewayProse(alarmLocationsMeaning)})
	builder.add(quarantine{ID: quarantineDictionaryOnly, Origin: originReference, Subject: "Alarm dictionary addresses without a register row", Evidence: "alarm mapping limits", Meaning: gatewayProse(dictionaryOnlyMeaning)})

	conflictRows := map[string]bool{}
	presenceRows := 0
	for _, build := range builds {
		editionID := build.Edition.ID
		for index := range build.Rows {
			row := &build.Rows[index]
			flags, err := rowQuarantines(row)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", row.Evidence, err)
			}
			for _, flag := range flags {
				id := flag
				if topic, ok := topicIDs[flag]; ok {
					id = topic
				}
				builder.flagRegister(id, build.Edition.Card, editionID, &row.Row)
			}
			if len(row.Codes) > 0 && !containsString(row.Codes, codeEnvApplicability) {
				conflictRows[row.Evidence] = true
			}
			if build.Edition.Family == "S-Dongle" && row.Row.Endpoint == endpointGatewaySelf && containsInt(presenceOffsets, *row.Row.Offset) {
				if row.Row.Page != presencePage {
					return nil, fmt.Errorf("%s: presence row on page %d", row.Evidence, row.Row.Page)
				}
				builder.flagRegister(topicIDs[topicPresence], build.Edition.Card, editionID, &row.Row)
				presenceRows++
			}
			if row.Row.Endpoint == endpointAttachedMeter && row.Row.Type == opaqueLayoutType {
				builder.flagRegister(topicIDs[topicOpaqueMeter], build.Edition.Card, editionID, &row.Row)
			}
		}
		for index := range build.Controls {
			if len(build.Controls[index].Codes) > 0 {
				return nil, fmt.Errorf("%s: a quarantined control row has no binding", build.Controls[index].Evidence)
			}
		}
	}
	if presenceRows != len(presenceOffsets)*4 {
		return nil, fmt.Errorf("%d presence rows, want three in each of the four S-Dongle editions", presenceRows)
	}
	if len(conflictRows) != len(tables.Conflicts) {
		return nil, fmt.Errorf("%d quarantined conflict rows, the conflict table lists %d", len(conflictRows), len(tables.Conflicts))
	}
	for record := range tables.Conflicts {
		if !conflictRows[record] {
			return nil, fmt.Errorf("conflict table row %s is not quarantined by its twin", record)
		}
	}

	for _, build := range builds {
		service := build.Service
		flag := func(name string) {
			id := name
			if topic, ok := topicIDs[name]; ok {
				id = topic
			}
			service.Quarantine = appendUnique(service.Quarantine, id)
			reference := affectedRow{Set: affectedServices, Edition: build.Edition.ID}
			builder.affected[id][affectedIdentity(reference)] = reference
			builder.edition(id, build.Edition.ID)
		}
		flag(topicRoutingOverlap)
		flag(topicZeroCount)
		flag(topicFrameMismatch)
		flag(quarantineAttributeSix)
		if build.Edition.Family == "S-Dongle" {
			flag(topicCRCRows)
		}
		if build.Edition.ID == shiftedBasicEdition {
			flag(topicShiftedBasic)
		}
		if build.Edition.Family == "SmartLogger" && build.Edition.Issue == "Issue 49" {
			flag(topicExceptionByte)
			flag(topicObjectNumbering)
		}
		if len(service.Spacing) > 0 {
			flag(topicPacing)
		}
	}

	if err := flagAlarms(builds, tables, builder); err != nil {
		return nil, err
	}
	return builder.build(), nil
}

// rowQuarantines binds the twin quarantine codes of a read-only row to the
// quarantine identifiers or topic names that carry them. A code combination
// without a binding stops the generator.
func rowQuarantines(row *gatewayRow) ([]string, error) {
	codes := row.Codes
	if len(codes) == 0 {
		return nil, nil
	}
	switch {
	case containsString(codes, codeRemapLayout) || containsString(codes, codeOutsideBlock):
		if !containsString(codes, codeRemapLayout) || !containsString(codes, codeOutsideBlock) || row.Row.Endpoint != endpointRemappedInverter {
			return nil, fmt.Errorf("remap layout codes %v on %s", codes, row.Row.Endpoint)
		}
		for _, code := range codes {
			if code != codeRemapLayout && code != codeOutsideBlock && code != codeWidth {
				return nil, fmt.Errorf("code %q on a remapped inverter row has no binding", code)
			}
		}
		return []string{topicRemapLayouts}, nil
	case len(codes) == 1 && codes[0] == codeEnvApplicability:
		if row.Row.Endpoint != endpointRemappedEnvironmental {
			return nil, fmt.Errorf("environmental scope code on %s", row.Row.Endpoint)
		}
		return []string{topicEnvironmentScope}, nil
	case len(codes) == 1 && codes[0] == codeWidth && row.Row.Endpoint == endpointChildAlias:
		return []string{topicSerialWidth}, nil
	case len(codes) == 1 && codes[0] == codeWidth && row.Row.Endpoint == endpointGatewaySelf && row.Row.Type == "I64":
		return []string{quarantineWidth64}, nil
	case len(codes) == 1 && codes[0] == codeWidth && row.Row.Endpoint == endpointGatewaySelf && row.Row.Type == "U32":
		return []string{quarantineWidth32}, nil
	}
	return nil, fmt.Errorf("quarantine codes %v on %s have no binding", codes, row.Row.Endpoint)
}

// flagAlarms flags the colliding alarm bits, the alarm printed at two
// locations and the bits at dictionary-only addresses, and checks each set
// against the contradiction appendix.
func flagAlarms(builds []*gatewayBuild, tables *contradictionTables, builder *quarantineBuilder) error {
	collisions, only := 0, 0
	for _, build := range builds {
		quarantines := build.Twin.AlarmQuarantines
		flag := func(id string, entry *alarmEntry) {
			entry.Bit.Quarantine = appendUnique(entry.Bit.Quarantine, id)
			reference := affectedRow{Set: affectedAlarms, Card: build.Edition.Card, Offset: entry.Bit.Offset, Key: alarmKey(entry.Bit), Page: entry.Bit.Page}
			builder.affected[id][affectedIdentity(reference)] = reference
			builder.edition(id, build.Edition.ID)
		}
		for _, collision := range quarantines.Colliding {
			key := build.SourceID + " " + strconv.Itoa(collision.Address) + "/" + strconv.Itoa(collision.Bit)
			var pairs []string
			matched := 0
			for _, candidate := range collision.Candidates {
				pairs = append(pairs, strconv.Itoa(candidate[0])+"/"+strconv.Itoa(candidate[1]))
				for _, entry := range build.Alarms {
					bit := entry.Bit
					if *bit.Offset == collision.Address && bit.Bit == collision.Bit && bit.AlarmID == candidate[0] && *bit.Cause == candidate[1] {
						flag(quarantineAlarmCollision, entry)
						matched++
					}
				}
			}
			if strings.Join(pairs, ",") != strings.Join(tables.Collisions[key], ",") || matched != len(collision.Candidates) {
				return fmt.Errorf("%s: colliding bit %s disagrees with the appendix or the tuples", build.SourceID, key)
			}
			collisions++
		}
		for _, location := range quarantines.MultipleLocations {
			matched := 0
			for _, entry := range build.Alarms {
				bit := entry.Bit
				for _, place := range location.Locations {
					if bit.AlarmID == location.AlarmID && *bit.Cause == location.CauseID && *bit.Offset == place[0] && bit.Bit == place[1] {
						flag(quarantineAlarmLocations, entry)
						matched++
					}
				}
			}
			if matched != len(location.Locations) || location.AlarmID != 1122 || location.CauseID != 1 {
				return fmt.Errorf("%s: alarm %d/%d locations disagree with the appendix", build.SourceID, location.AlarmID, location.CauseID)
			}
		}
		var addresses []int
		for _, item := range quarantines.DictionaryOnly {
			addresses = append(addresses, item.Address)
			matched := 0
			for _, entry := range build.Alarms {
				if *entry.Bit.Offset == item.Address {
					flag(quarantineDictionaryOnly, entry)
					matched++
				}
			}
			if matched == 0 {
				return fmt.Errorf("%s: dictionary-only address %d has no bits", build.SourceID, item.Address)
			}
			for _, row := range build.Rows {
				if row.Row.Endpoint == endpointGatewaySelf && *row.Row.Offset <= item.Address && item.Address < *row.Row.Offset+row.Row.Quantity {
					return fmt.Errorf("%s: dictionary-only address %d lies inside a register row", build.SourceID, item.Address)
				}
			}
		}
		if fmt.Sprint(addresses) != fmt.Sprint(tables.DictionaryOnly[build.SourceID]) {
			return fmt.Errorf("%s: dictionary-only addresses %v disagree with the appendix", build.SourceID, addresses)
		}
		only += len(addresses)
		// Every bit that carries two different alarm and cause pairs must be a
		// flagged candidate set; a pair repeated with another condition text is
		// one decode and stays unflagged.
		bits := make([]alarmBit, 0, len(build.Alarms))
		for _, entry := range build.Alarms {
			bits = append(bits, entry.Bit)
		}
		if err := checkCandidateSets(bits); err != nil {
			return fmt.Errorf("%s: %w", build.SourceID, err)
		}
	}
	if collisions != len(tables.Collisions) {
		return fmt.Errorf("%d colliding bits, the appendix lists %d", collisions, len(tables.Collisions))
	}
	stated := 0
	for _, list := range tables.DictionaryOnly {
		stated += len(list)
	}
	if only != stated {
		return fmt.Errorf("%d dictionary-only addresses, the appendix lists %d", only, stated)
	}
	return nil
}

// checkCandidateSets requires the collision flag exactly on the bits whose
// offset and bit carry more than one alarm and cause pair.
func checkCandidateSets(bits []alarmBit) error {
	pairs := map[string]map[string]bool{}
	for _, bit := range bits {
		key := strconv.Itoa(*bit.Offset) + "/" + strconv.Itoa(bit.Bit)
		if pairs[key] == nil {
			pairs[key] = map[string]bool{}
		}
		pairs[key][alarmKey(bit)] = true
	}
	for _, bit := range bits {
		key := strconv.Itoa(*bit.Offset) + "/" + strconv.Itoa(bit.Bit)
		if (len(pairs[key]) > 1) != hasFlag(bit.Quarantine, quarantineAlarmCollision) {
			return fmt.Errorf("bit %s carries %d alarm and cause pairs; the candidate-set flag must mark exactly the bits with several", key, len(pairs[key]))
		}
	}
	return nil
}

func alarmKey(bit alarmBit) string {
	cause := 0
	if bit.Cause != nil {
		cause = *bit.Cause
	}
	return "bit-" + strconv.Itoa(bit.Bit) + "-alarm-" + strconv.Itoa(bit.AlarmID) + "-cause-" + strconv.Itoa(cause)
}

// contractOrder fixes the order of the section page ranges.
var contractOrder = []string{
	"requirements", "constraints", "physical", "addressing", "mbap", "encoding", "interaction", "query_period", "function_list",
	"exceptions", "read", "write_single", "write_multiple", "identity_basic", "identity_list", "identity_description",
}

// buildServiceEdition converts the protocol facts of one edition and checks
// them against the edition comparison.
func buildServiceEdition(build *gatewayBuild, connection string, row *editionComparison) (*serviceEdition, error) {
	protocol := build.Twin.Protocol
	transport := protocol.Transport
	if row == nil {
		return nil, fmt.Errorf("the edition comparison has no rows for the edition")
	}
	if len(protocol.Sections) != len(contractOrder) {
		return nil, fmt.Errorf("%d section page ranges, want %d", len(protocol.Sections), len(contractOrder))
	}
	service := &serviceEdition{
		Edition: build.Edition.ID, SelfUnit: *build.Edition.SelfUnit, ChildUnits: transport.Units.ChildRange, ReservedUnits: transport.Units.Reserved,
		UnitNotes: transport.Units.Conflicts, ByteOrder: transport.ByteOrder, Port502Stated: transport.Port502, FrameLimitBytes: transport.FrameLimit,
		ReadQuantity: "1–125", WriteQuantity: "0–123", Functions: build.Card.Functions, ExceptionCodes: protocol.ExceptionCodes,
		ExceptionPages: protocol.ExceptionPages, RequestObject: protocol.RequestObject, WrapsBeyondFF: protocol.Wraps,
		Attribute6: row.Attribute6, DescriptorAttributes: row.Descriptors,
	}
	if transport.Timeout5s {
		service.ResponseTimeout = 5
	}
	switch {
	case transport.ByteOrder != "big_endian" || !transport.Timeout5s || transport.FrameLimit != 256:
		return nil, fmt.Errorf("transport facts disagree with the comparison statement")
	case row.SelfUnit != service.SelfUnit || row.Port502 != transport.Port502:
		return nil, fmt.Errorf("self unit or port statement disagrees with the comparison")
	case row.RequestObject != protocol.RequestObject || row.Wraps != protocol.Wraps:
		return nil, fmt.Errorf("request object or wrap disagrees with the comparison")
	case strings.Join(row.ExceptionCodes, ",") != strings.Join(protocol.ExceptionCodes, ",") || fmt.Sprint(row.ExceptionPages) != fmt.Sprint(protocol.ExceptionPages):
		return nil, fmt.Errorf("exception inventory disagrees with the comparison")
	case transport.Units.ChildRange == "" || transport.Units.Reserved == "":
		return nil, fmt.Errorf("unit ranges missing")
	}
	for _, name := range contractOrder {
		section, ok := protocol.Sections[name]
		if !ok {
			return nil, fmt.Errorf("section %s missing", name)
		}
		if section == nil {
			continue
		}
		if len(section.Pages) != 2 || section.Pages[0] < 1 || section.Pages[1] < section.Pages[0] {
			return nil, fmt.Errorf("section %s pages %v", name, section.Pages)
		}
		service.Pages = append(service.Pages, contractPages{Contract: name, First: section.Pages[0], Last: section.Pages[1]})
	}
	list, descriptor := protocol.Sections["identity_list"], protocol.Sections["identity_description"]
	if list == nil || descriptor == nil || fmt.Sprint(list.Pages) != fmt.Sprint(row.ListPages) || fmt.Sprint(descriptor.Pages) != fmt.Sprint(row.DescriptorPage) {
		return nil, fmt.Errorf("private list pages disagree with the comparison")
	}
	spacing, periods, err := spacingFor(connection, build.Edition.ID)
	if err != nil {
		return nil, err
	}
	service.Spacing, service.QueryPeriods = spacing, periods
	return service, nil
}

// gatewayServices assembles the services document.
func gatewayServices(builds []*gatewayBuild, wire *wireChapter, discovery *discoveryChapter, limits []frameLimit) (servicesDoc, error) {
	doc := servicesDoc{
		Schema: servicesSchema, DeviceListGrammar: discovery.Grammar, Functions: wire.Functions, ListObjects: discovery.Objects,
		Attributes: discovery.Attributes, Limits: limits,
	}
	families := map[string]map[string]bool{}
	for _, build := range builds {
		for _, code := range build.Service.ExceptionCodes {
			if families[code] == nil {
				families[code] = map[string]bool{}
			}
			families[code][build.Edition.Family] = true
		}
		doc.Editions = append(doc.Editions, *build.Service)
	}
	sort.Slice(doc.Editions, func(i, j int) bool { return doc.Editions[i].Edition < doc.Editions[j].Edition })
	codes := make([]string, 0, len(families))
	for code := range families {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		meaning, ok := wire.Exceptions[code]
		if !ok {
			return servicesDoc{}, fmt.Errorf("exception code %s has no meaning in the wire chapter", code)
		}
		doc.Exceptions = append(doc.Exceptions, exceptionMeaning{Code: code, Meaning: meaning, Families: sortedKeys(families[code])})
	}
	if len(codes) != len(wire.Exceptions) {
		return servicesDoc{}, fmt.Errorf("%d exception meanings, %d codes listed by the editions", len(wire.Exceptions), len(codes))
	}
	return doc, nil
}

// gatewaySidecar returns the private locator files of the gateway mode:
// source identifiers, document names and digests from the reference's locator
// file, row and alarm locators, and the accounting. They are never committed.
func gatewaySidecar(builds []*gatewayBuild, locators []byte, got gatewayCounts) (map[string][]byte, error) {
	var sources struct {
		Sources []map[string]any `json:"sources"`
	}
	if err := json.Unmarshal(locators, &sources); err != nil {
		return nil, fmt.Errorf("locator file: %w", err)
	}
	bySource := map[string]map[string]any{}
	for _, source := range sources.Sources {
		if id, ok := source["source_id"].(string); ok {
			bySource[id] = source
		}
	}
	type sidecarEdition struct {
		ID       string         `json:"id"`
		SourceID string         `json:"source_id"`
		Title    string         `json:"reference_title"`
		Locator  map[string]any `json:"locator"`
	}
	type sidecarRow struct {
		Edition    string             `json:"edition"`
		Set        string             `json:"set"`
		Key        string             `json:"key"`
		Occurrence int                `json:"occurrence,omitempty"`
		Private    map[string]string  `json:"private"`
		Predicates []gatewayPredicate `json:"predicates,omitempty"`
	}
	type sidecarAlarm struct {
		Edition string            `json:"edition"`
		Key     string            `json:"key"`
		Offset  int               `json:"offset"`
		Private map[string]string `json:"private"`
	}
	var editions []sidecarEdition
	var rows []sidecarRow
	var alarms []sidecarAlarm
	coverage := map[string]map[string]int{}
	for _, build := range builds {
		locator, ok := bySource[build.SourceID]
		if !ok {
			return nil, fmt.Errorf("%s has no locator entry", build.SourceID)
		}
		editions = append(editions, sidecarEdition{ID: build.Edition.ID, SourceID: build.SourceID, Title: build.Twin.Title, Locator: locator})
		counts := map[string]int{}
		for set, list := range map[string][]gatewayRow{"rows": build.Rows, "controls": build.Controls} {
			for _, row := range list {
				rows = append(rows, sidecarRow{Edition: build.Edition.ID, Set: set, Key: row.Row.Key, Occurrence: row.Row.Occurrence, Private: row.Private, Predicates: row.RowSpecific})
				counts[row.Row.Class]++
				counts["endpoint "+row.Row.Endpoint]++
			}
		}
		for _, entry := range build.Alarms {
			alarms = append(alarms, sidecarAlarm{Edition: build.Edition.ID, Key: alarmKey(entry.Bit), Offset: *entry.Bit.Offset, Private: entry.Private})
		}
		counts["alarm tuples"] = len(build.Alarms)
		counts["alarm records"] = len(build.Twin.AlarmDictionary)
		coverage[build.Edition.ID] = counts
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Edition != rows[j].Edition {
			return rows[i].Edition < rows[j].Edition
		}
		return rows[i].Set < rows[j].Set
	})
	sort.SliceStable(editions, func(i, j int) bool { return editions[i].ID < editions[j].ID })
	out := map[string][]byte{}
	for name, value := range map[string]any{
		"editions.json": editions, "registers.json": rows, "alarms.json": alarms,
		"accounting.json": map[string]any{"counts": got, "coverage": coverage},
	} {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil, err
		}
		out[name] = append(data, '\n')
	}
	return out, nil
}
