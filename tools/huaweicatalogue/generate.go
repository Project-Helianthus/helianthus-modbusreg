package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type options struct {
	Reference string
	Expect    counts
}

// cardCoverage is the per-card parser accounting written to the sidecar.
type cardCoverage struct {
	Card           string         `json:"card"`
	Parser         string         `json:"parser"`
	Rows           int            `json:"rows"`
	StatedRows     int            `json:"stated_rows"`
	Controls       int            `json:"controls"`
	Classes        map[string]int `json:"classes"`
	ControlClasses map[string]int `json:"control_classes"`
	Endpoints      map[string]int `json:"endpoints"`
	Core           int            `json:"core"`
	Unspecified    []string       `json:"address_unspecified"`
	ReadExtents    int            `json:"read_extents"`
	ModelRows      int            `json:"model_rows_in_card_edition"`
	QuarantineRows map[string]int `json:"quarantined_rows"`
}

type result struct {
	Catalogue *catalogue
	Files     map[string][]byte
	Sidecar   map[string][]byte
	Counts    counts
	Coverage  []cardCoverage
}

type reader struct{ root string }

func (r reader) bytes(relative string) ([]byte, error) {
	return os.ReadFile(filepath.Join(r.root, filepath.FromSlash(relative)))
}

func (r reader) text(relative string) (string, error) {
	data, err := r.bytes(relative)
	return string(data), err
}

var baselinePattern = regexp.MustCompile(`Baseline \*\*(\d{4}-\d{2}-\d{2}\.\d+)\*\*`)

type cardBuild struct {
	info    *cardInfo
	twin    twinDoc
	rows    cardRows
	extents []readExtent
}

// generate builds the whole catalogue from the reference baseline and
// verifies it against the baseline's own accounting and the expected counts.
func generate(opts options) (*result, error) {
	in := reader{root: opts.Reference}
	readme, err := in.text("README.md")
	if err != nil {
		return nil, err
	}
	baselineMatch := baselinePattern.FindStringSubmatch(readme)
	if baselineMatch == nil {
		return nil, fmt.Errorf("the reference states no baseline")
	}
	baseline := baselineMatch[1]
	sources, err := in.text("appendices/sources.md")
	if err != nil {
		return nil, err
	}
	anchors, census, err := parseSources(sources)
	if err != nil {
		return nil, err
	}
	directory, err := in.text("profiles/README.md")
	if err != nil {
		return nil, err
	}
	navigation, err := in.text("semantic-review/README.md")
	if err != nil {
		return nil, err
	}
	cards, err := parseCards(directory, navigation)
	if err != nil {
		return nil, err
	}
	var validation struct {
		CanonicalTotals struct {
			Profiles           int `json:"profiles"`
			ROOccurrences      int `json:"RO_occurrences"`
			ControlOccurrences int `json:"control_occurrences"`
		} `json:"canonical_totals"`
	}
	validationData, err := in.bytes("semantic-review/VALIDATION.json")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(validationData, &validation); err != nil {
		return nil, err
	}

	builds := map[string]*cardBuild{}
	statedRO, roTotal, controlTotal := 0, 0, 0
	for _, card := range cards {
		statedRO += card.ROCount
		data, err := in.bytes("profiles/" + card.Slug + ".json")
		if err != nil {
			return nil, fmt.Errorf("card %s has no JSON twin: %w", card.ID, err)
		}
		build := &cardBuild{info: card}
		if err := json.Unmarshal(data, &build.twin); err != nil {
			return nil, fmt.Errorf("%s twin: %w", card.ID, err)
		}
		if build.twin.Schema != "huawei-pv-profile-portable-v1" || build.twin.Profile != card.Slug || build.twin.Baseline != baseline {
			return nil, fmt.Errorf("%s twin: schema, profile or baseline mismatch", card.ID)
		}
		entry, ok := anchors[strings.ToLower(build.twin.SourceID)]
		if !ok || entry.SourceID != build.twin.SourceID {
			return nil, fmt.Errorf("%s twin names unknown source %s", card.ID, build.twin.SourceID)
		}
		card.Source = entry
		if err := parseReadOnly(build.twin.ReadOnly, build.twin.SourceID, &build.rows); err != nil {
			return nil, fmt.Errorf("%s read-only: %w", card.ID, err)
		}
		if err := parseControls(build.twin.Controls, card.Slug, build.twin.SourceID, &build.rows); err != nil {
			return nil, fmt.Errorf("%s controls: %w", card.ID, err)
		}
		if len(build.rows.Rows) != card.ROCount {
			return nil, fmt.Errorf("%s: %d read-only rows, the navigation lists %d", card.ID, len(build.rows.Rows), card.ROCount)
		}
		build.extents, err = readExtents(build.twin.Extents)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", card.ID, err)
		}
		if err := checkTwinQuarantines(build); err != nil {
			return nil, fmt.Errorf("%s: %w", card.ID, err)
		}
		roTotal += len(build.rows.Rows)
		controlTotal += len(build.rows.Controls)
		builds[card.ID] = build
	}
	if validation.CanonicalTotals.Profiles != len(cards) || validation.CanonicalTotals.ROOccurrences != statedRO ||
		validation.CanonicalTotals.ROOccurrences != roTotal || validation.CanonicalTotals.ControlOccurrences != controlTotal {
		return nil, fmt.Errorf("baseline accounting disagrees: %d cards, %d read-only and %d control rows parsed; %d, %d and %d stated",
			len(cards), roTotal, controlTotal, validation.CanonicalTotals.Profiles, validation.CanonicalTotals.ROOccurrences, validation.CanonicalTotals.ControlOccurrences)
	}
	coreCount, err := attachCore(in, cards, builds)
	if err != nil {
		return nil, err
	}

	editionsText, err := in.text("appendices/model-editions.md")
	if err != nil {
		return nil, err
	}
	statedModels := editionModelCounts(editionsText)
	sourced := append([]*censusEntry(nil), census...)
	sort.Slice(sourced, func(i, j int) bool { return sourced[i].SourceID < sourced[j].SourceID })
	if len(statedModels) != len(sourced) {
		return nil, fmt.Errorf("%d editions in the model accounting, %d in the source appendix", len(statedModels), len(sourced))
	}
	cardBySHA := map[string]string{}
	for _, card := range cards {
		cardBySHA[card.Source.SHA256] = card.ID
	}
	editionBySHA, err := buildEditions(sourced, cardBySHA)
	if err != nil {
		return nil, err
	}
	var models []parsedModel
	var requirements []documentRequirement
	tables := map[string]modelTable{}
	for _, entry := range sourced {
		data, err := in.bytes("data/models/" + entry.SourceID + ".json")
		if err != nil {
			return nil, fmt.Errorf("edition %s has no model table: %w", entry.SourceID, err)
		}
		var table modelTable
		if err := json.Unmarshal(data, &table); err != nil {
			return nil, fmt.Errorf("%s model table: %w", entry.SourceID, err)
		}
		if table.Schema != "huawei-pv-model-table-v1" || table.SourceID != entry.SourceID {
			return nil, fmt.Errorf("%s model table: schema or source mismatch", entry.SourceID)
		}
		if len(table.Rows) != statedModels[entry.SourceID] || (len(table.Rows) == 0) != (table.EmptyReason != nil) {
			return nil, fmt.Errorf("%s: %d model rows, the accounting states %d", entry.SourceID, len(table.Rows), statedModels[entry.SourceID])
		}
		editionID := editionBySHA[entry.SHA256].ID
		editionBySHA[entry.SHA256].ModelRows = len(table.Rows)
		converted, reqs, err := convertModelTable(table, editionID)
		if err != nil {
			return nil, err
		}
		models = append(models, converted...)
		requirements = append(requirements, reqs...)
		tables[entry.SourceID] = table
	}
	for _, card := range cards {
		if err := sameModelTable(builds[card.ID].twin.Models, tables[card.Source.SourceID]); err != nil {
			return nil, fmt.Errorf("%s: %w", card.ID, err)
		}
	}
	sortModels(models)
	sort.Slice(requirements, func(i, j int) bool { return requirements[i].ID < requirements[j].ID })

	contradictions, err := in.text("appendices/contradictions.md")
	if err != nil {
		return nil, err
	}
	referenceItems, quarantineOffsets, quarantineEditions, err := referenceQuarantines(contradictions, anchors)
	if err != nil {
		return nil, err
	}

	c := &catalogue{Registers: map[string]*registersDoc{}}
	c.Editions = editionsDoc{Schema: editionsSchema, ReferenceBaseline: baseline, Attribution: attributionNote}
	for _, record := range editionBySHA {
		c.Editions.Editions = append(c.Editions.Editions, *record)
	}
	sort.Slice(c.Editions.Editions, func(i, j int) bool { return c.Editions.Editions[i].ID < c.Editions.Editions[j].ID })

	builder := newQuarantineBuilder()
	for _, item := range referenceItems {
		builder.add(item)
		for _, entry := range quarantineEditions[item.ID] {
			builder.edition(item.ID, editionBySHA[entry.SHA256].ID)
		}
	}
	for _, item := range generatorQuarantines {
		builder.add(item)
	}
	for _, spec := range anomalySpecs {
		text, err := in.text(spec.File)
		if err != nil {
			return nil, err
		}
		meaning, err := anomalyMeaning(spec, text)
		if err != nil {
			return nil, err
		}
		builder.add(quarantine{ID: spec.ID, Origin: originReference, Subject: spec.Subject, Meaning: meaning})
		for _, cardID := range spec.Cards {
			build, ok := builds[cardID]
			if !ok {
				return nil, fmt.Errorf("%s names unknown card %s", spec.ID, cardID)
			}
			builder.edition(spec.ID, editionBySHA[build.info.Source.SHA256].ID)
		}
	}

	statusChapter, err := in.text("07-status-alarms.md")
	if err != nil {
		return nil, err
	}
	var dictionaries []dictionary
	var coverage []cardCoverage
	for _, card := range cards {
		build := builds[card.ID]
		editionID := editionBySHA[card.Source.SHA256].ID
		sortRows(build.rows.Rows)
		sortRows(build.rows.Controls)
		for _, list := range [][]parsedRow{build.rows.Rows, build.rows.Controls} {
			for index := range list {
				row := &list[index].Row
				if row.Offset == nil {
					continue
				}
				for _, item := range referenceItems {
					for _, entry := range quarantineEditions[item.ID] {
						if scope, narrowed := quarantineRowScope[item.ID]; narrowed && !containsString(scope, entry.Alias) {
							continue
						}
						if entry == card.Source && containsInt(quarantineOffsets[item.ID], *row.Offset) {
							builder.flagRegister(item.ID, card.ID, editionID, row)
						}
					}
				}
				for _, spec := range anomalySpecs {
					if containsString(spec.Cards, card.ID) && containsInt(spec.Offsets, *row.Offset) {
						builder.flagRegister(spec.ID, card.ID, editionID, row)
					}
				}
			}
		}
		doc := &registersDoc{
			Schema: registersSchema, Card: card.ID, Edition: editionID, ModelPages: card.ModelPages,
			SelectionNote: prose(card.Note), Gates: build.rows.Gates, FunctionalClasses: build.rows.Classes,
			ReadExtents: build.extents, Rows: []registerRow{}, Controls: []registerRow{},
		}
		for _, row := range build.rows.Rows {
			doc.Rows = append(doc.Rows, row.Row)
		}
		for _, row := range build.rows.Controls {
			doc.Controls = append(doc.Controls, row.Row)
		}
		if additive := build.twin.Additive; additive != nil {
			support, err := convertAdditive(additive, anchors, editionBySHA)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", card.ID, err)
			}
			doc.AdditiveSupport = support
		}
		c.Registers[card.CatalogueFile] = doc
		items, err := cardDictionaries(card, editionID, &build.rows)
		if err != nil {
			return nil, fmt.Errorf("%s dictionaries: %w", card.ID, err)
		}
		if card.Source.Alias == "M10" {
			if err := applyModePredicate(items, card, statusChapter); err != nil {
				return nil, err
			}
		}
		dictionaries = append(dictionaries, items...)
		coverage = append(coverage, coverCard(card, build, doc))
	}
	sort.SliceStable(dictionaries, func(i, j int) bool {
		if dictionaries[i].Card != dictionaries[j].Card {
			return dictionaries[i].Card < dictionaries[j].Card
		}
		if firstOffset(dictionaries[i]) != firstOffset(dictionaries[j]) {
			return firstOffset(dictionaries[i]) < firstOffset(dictionaries[j])
		}
		return dictionaries[i].ID < dictionaries[j].ID
	})
	c.Dictionaries = dictionariesDoc{Schema: dictionariesSchema, Dictionaries: dictionaries}

	for index := range models {
		row := &models[index].Row
		for _, flag := range append([]string(nil), row.Quarantine...) {
			builder.recordModel(flag, row)
		}
		for _, spec := range anomalySpecs {
			if spec.FloorSuffix != "" && strings.Contains(row.RowMinimum, spec.FloorSuffix) {
				builder.flagModel(spec.ID, row)
			}
		}
	}
	c.Models = modelsDoc{Schema: modelsSchema, DocumentRequirements: requirements}
	for _, model := range models {
		c.Models.Rows = append(c.Models.Rows, model.Row)
	}
	c.Quarantines = quarantinesDoc{Schema: quarantinesSchema, Quarantines: builder.build()}

	got := countCatalogue(c)
	if got.RO != roTotal || got.Controls != controlTotal || got.Core != coreCount || got.Models != sumCounts(statedModels) {
		return nil, fmt.Errorf("internal accounting mismatch: %s", got)
	}
	if got != opts.Expect {
		return nil, fmt.Errorf("counts %s differ from the expected baseline accounting %s; review the reference change and rerun with -expect %s", got, opts.Expect, got)
	}
	if err := validateCatalogue(c); err != nil {
		return nil, err
	}
	if err := guardCatalogue(c); err != nil {
		return nil, err
	}
	files, err := encodeCatalogue(c)
	if err != nil {
		return nil, err
	}
	sidecar, err := buildSidecar(builds, cards, census, models, editionBySHA, coverage, got)
	if err != nil {
		return nil, err
	}
	return &result{Catalogue: c, Files: files, Sidecar: sidecar, Counts: got, Coverage: coverage}, nil
}

func firstOffset(item dictionary) int {
	if len(item.Offsets) == 0 {
		return -1
	}
	return item.Offsets[0]
}

func sumCounts(values map[string]int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

// checkTwinQuarantines verifies that every row the twin quarantines is a
// conflict row or a no-send control row.
func checkTwinQuarantines(build *cardBuild) error {
	index := func(rows []parsedRow) map[string]*registerRow {
		out := map[string]*registerRow{}
		for position := range rows {
			for _, id := range rows[position].TwinIDs {
				out[id] = &rows[position].Row
			}
		}
		return out
	}
	readOnly, controls := index(build.rows.Rows), index(build.rows.Controls)
	for _, item := range build.twin.Quarantines {
		switch item.Section {
		case "read_only":
			row, ok := readOnly[item.EntryID]
			if !ok || row.Class != classConflict {
				return fmt.Errorf("twin quarantine %s is not a conflict row", item.EntryID)
			}
		case "controls":
			row, ok := controls[item.EntryID]
			if !ok || row.Class != classNoSend {
				return fmt.Errorf("twin quarantine %s is not a no-send control row", item.EntryID)
			}
		default:
			return fmt.Errorf("twin quarantine %s has unknown section %q", item.EntryID, item.Section)
		}
	}
	return nil
}

func convertAdditive(additive *twinAdditive, anchors map[string]*censusEntry, editions map[string]*edition) (*additiveSupport, error) {
	entry, ok := anchors[strings.ToLower(additive.SourceID)]
	if !ok {
		return nil, fmt.Errorf("additive support names unknown source %s", additive.SourceID)
	}
	support := &additiveSupport{
		Edition: editions[entry.SHA256].ID, Token: additive.Model.Token, Model: additive.Model.ModelID,
		Minimum: additive.Model.Minimum, Page: additive.Model.Page, Note: prose(additive.Conditions),
	}
	for _, row := range additive.Rows {
		support.Rows = append(support.Rows, additiveRow{Offset: row.Address, Quantity: row.Words, Type: row.Type, Meaning: prose(row.Meaning), Page: row.Page})
	}
	return support, nil
}

// attachCore marks the register rows that the reference lists as core rows.
func attachCore(in reader, cards []*cardInfo, builds map[string]*cardBuild) (int, error) {
	data, err := in.bytes("appendices/core-semantic-review.json")
	if err != nil {
		return 0, err
	}
	var core struct {
		Rows []struct {
			Profile       string `json:"profile"`
			FunctionalKey string `json:"functional_key"`
			EvidenceID    string `json:"evidence_id"`
			Gate          string `json:"gate"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(data, &core); err != nil {
		return 0, err
	}
	perCard := map[string]int{}
	for _, entry := range core.Rows {
		build, ok := builds[strings.ToUpper(entry.Profile)]
		if !ok {
			return 0, fmt.Errorf("core row names unknown card %s", entry.Profile)
		}
		matched := 0
		for index := range build.rows.Rows {
			if build.rows.Rows[index].Evidence == entry.EvidenceID {
				if build.rows.Rows[index].Row.Core != nil {
					return 0, fmt.Errorf("core row %s matches a row twice", entry.FunctionalKey)
				}
				build.rows.Rows[index].Row.Core = &coreRef{Key: entry.FunctionalKey, Gate: cleanText(entry.Gate)}
				matched++
			}
		}
		if matched != 1 {
			return 0, fmt.Errorf("core row %s/%s matches %d register rows", entry.Profile, entry.FunctionalKey, matched)
		}
		perCard[build.info.ID]++
	}
	for _, card := range cards {
		if perCard[card.ID] == 0 && card.CoreCount == card.ROCount {
			continue
		}
		if perCard[card.ID] != card.CoreCount {
			return 0, fmt.Errorf("%s: %d core rows matched, the card lists %d", card.ID, perCard[card.ID], card.CoreCount)
		}
	}
	return len(core.Rows), nil
}

func coverCard(card *cardInfo, build *cardBuild, doc *registersDoc) cardCoverage {
	item := cardCoverage{
		Card: card.ID, Parser: build.rows.Parser, Rows: len(doc.Rows), StatedRows: card.ROCount, Controls: len(doc.Controls),
		Classes: map[string]int{}, ControlClasses: map[string]int{}, Endpoints: map[string]int{}, QuarantineRows: map[string]int{},
		Unspecified: []string{}, ReadExtents: len(doc.ReadExtents), ModelRows: len(build.twin.Models),
	}
	for _, row := range doc.Rows {
		item.Classes[row.Class]++
		item.Endpoints[row.Endpoint]++
		if row.Core != nil {
			item.Core++
		}
		if row.AddressStatus != "" {
			item.Unspecified = append(item.Unspecified, row.Key)
		}
		for _, flag := range row.Quarantine {
			item.QuarantineRows[flag]++
		}
	}
	for _, row := range doc.Controls {
		item.ControlClasses[row.Class]++
		for _, flag := range row.Quarantine {
			item.QuarantineRows[flag]++
		}
	}
	return item
}

func containsInt(list []int, value int) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// buildSidecar returns the private locator files: digests, source
// identifiers, evidence locators and per-card accounting. They are never
// committed.
func buildSidecar(builds map[string]*cardBuild, cards []*cardInfo, entries []*censusEntry, models []parsedModel, editions map[string]*edition, coverage []cardCoverage, got counts) (map[string][]byte, error) {
	type sidecarEdition struct {
		ID       string   `json:"id"`
		SourceID string   `json:"source_id"`
		SHA256   string   `json:"sha256"`
		Lines    []string `json:"census_lines"`
	}
	type sidecarRow struct {
		Card       string            `json:"card"`
		Set        string            `json:"set"`
		Key        string            `json:"key"`
		Offset     *int              `json:"offset"`
		Occurrence int               `json:"occurrence,omitempty"`
		Private    map[string]string `json:"private"`
	}
	type sidecarModel struct {
		Edition    string            `json:"edition"`
		SourceID   string            `json:"source_id"`
		Page       int               `json:"page"`
		Token      string            `json:"token"`
		Identifier *int              `json:"identifier,omitempty"`
		Private    map[string]string `json:"private"`
	}
	shas := make([]string, 0, len(editions))
	for sha := range editions {
		shas = append(shas, sha)
	}
	sort.Strings(shas)
	census := map[string]*censusEntry{}
	for _, entry := range entries {
		census[entry.SHA256] = entry
	}
	var editionRows []sidecarEdition
	for _, sha := range shas {
		row := sidecarEdition{ID: editions[sha].ID, SHA256: sha}
		if entry, ok := census[sha]; ok {
			row.SourceID, row.Lines = entry.SourceID, entry.Lines
		}
		editionRows = append(editionRows, row)
	}
	var rows []sidecarRow
	for _, card := range cards {
		build := builds[card.ID]
		for set, list := range map[string][]parsedRow{"rows": build.rows.Rows, "controls": build.rows.Controls} {
			for _, row := range list {
				rows = append(rows, sidecarRow{Card: card.ID, Set: set, Key: row.Row.Key, Offset: row.Row.Offset, Occurrence: row.Row.Occurrence, Private: row.Private})
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Card != rows[j].Card {
			return rows[i].Card < rows[j].Card
		}
		return rows[i].Set < rows[j].Set
	})
	var modelRows []sidecarModel
	for _, model := range models {
		modelRows = append(modelRows, sidecarModel{Edition: model.Row.Edition, SourceID: model.SourceID, Page: model.Row.Page, Token: model.Row.Token, Identifier: model.Row.Identifier, Private: model.Private})
	}
	out := map[string][]byte{}
	for name, value := range map[string]any{
		"editions.json": editionRows, "registers.json": rows, "models.json": modelRows,
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

// moduleRoot returns the nearest ancestor of path that holds a go.mod file.
func moduleRoot(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	for directory := absolute; ; directory = filepath.Dir(directory) {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if filepath.Dir(directory) == directory {
			return "", fmt.Errorf("no module root above %s", path)
		}
	}
}

// insideDirectory reports whether path lies inside root after cleaning.
func insideDirectory(root, path string) (bool, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return false, nil
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))), nil
}

func countLine(c counts) string {
	return "read-only rows " + strconv.Itoa(c.RO) + ", core rows " + strconv.Itoa(c.Core) + ", control rows " + strconv.Itoa(c.Controls) +
		", model rows " + strconv.Itoa(c.Models) + ", cards " + strconv.Itoa(c.Cards)
}
