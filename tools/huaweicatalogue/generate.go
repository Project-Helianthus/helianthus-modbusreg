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
	ModelRows string
	Expect    counts
}

// cardCoverage is the per-card parser accounting written to the sidecar.
type cardCoverage struct {
	Card           string         `json:"card"`
	Parser         string         `json:"parser"`
	Rows           int            `json:"rows"`
	StatedRows     int            `json:"stated_rows"`
	Classes        map[string]int `json:"classes"`
	Endpoints      map[string]int `json:"endpoints"`
	Core           int            `json:"core"`
	TemplateRows   []string       `json:"template_rows"`
	Readable       int            `json:"readable_configuration"`
	ModelRows      int            `json:"model_rows_in_card_edition"`
	QuarantineRows map[string]int `json:"quarantined_rows"`
}

type result struct {
	Catalogue *catalogue
	Files     map[string][]byte
	Sidecar   map[string][]byte
	Counts    counts
	Coverage  []cardCoverage
	Notes     []string
}

type reader struct{ root string }

func (r reader) text(relative string) (string, error) {
	data, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(relative)))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (r reader) exists(relative string) bool {
	_, err := os.Stat(filepath.Join(r.root, filepath.FromSlash(relative)))
	return err == nil
}

type cardBuild struct {
	info     *cardInfo
	parser   string
	rows     []parsedRow
	readable []parsedRow
	gates    []gate
	classes  []functionalClass
	review   *fullReview
	markdown string
}

// generate builds the whole catalogue from the reference and verifies it
// against the reference's own accounting and the expected counts.
func generate(opts options) (*result, error) {
	in := reader{root: opts.Reference}
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
	cards, err := parseCards(directory, navigation, func(slug string) (string, error) { return in.text("profiles/" + slug + ".md") }, anchors)
	if err != nil {
		return nil, err
	}
	var validation struct {
		CanonicalTotals struct {
			Profiles      int `json:"profiles"`
			ROOccurrences int `json:"RO_occurrences"`
		} `json:"canonical_totals"`
	}
	validationText, err := in.text("semantic-review/VALIDATION.json")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(validationText), &validation); err != nil {
		return nil, err
	}
	statedRO := 0
	for _, card := range cards {
		statedRO += card.ROCount
	}
	if validation.CanonicalTotals.Profiles != len(cards) || validation.CanonicalTotals.ROOccurrences != statedRO {
		return nil, fmt.Errorf("the reference accounting disagrees with itself: %d cards and %d rows listed, %d and %d totalled",
			len(cards), statedRO, validation.CanonicalTotals.Profiles, validation.CanonicalTotals.ROOccurrences)
	}

	builds := map[string]*cardBuild{}
	for _, card := range cards {
		build := &cardBuild{info: card}
		builds[card.ID] = build
		cardText, err := in.text("profiles/" + card.Slug + ".md")
		if err != nil {
			return nil, err
		}
		switch {
		case in.exists("semantic-review/" + card.Slug + "-full-ro.json"):
			build.parser = "machine-readable review cross-checked against its Markdown table"
			data, err := os.ReadFile(filepath.Join(opts.Reference, "semantic-review", card.Slug+"-full-ro.json"))
			if err != nil {
				return nil, err
			}
			build.markdown, err = in.text("semantic-review/" + card.Slug + "-full-ro.md")
			if err != nil {
				return nil, err
			}
			build.rows, build.classes, build.review, err = parseFullReview(data, build.markdown, card.Source.SHA256)
			if err != nil {
				return nil, err
			}
		case strings.Contains(cardText, "## Complete detected RO tuple set"):
			build.parser = "supplement card tuple table"
			build.markdown = cardText
			build.rows, build.gates, build.classes, err = parseMB02(cardText)
			if err != nil {
				return nil, err
			}
		default:
			build.parser = "derived class-table catalogue"
			text, err := in.text("semantic-review/" + card.Slug + "-full-ro.md")
			if err != nil {
				return nil, err
			}
			var anchor string
			build.rows, build.gates, anchor, err = parseClassTables(text, card.Source.SHA256)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", card.ID, err)
			}
			if anchors[anchor] != card.Source {
				return nil, fmt.Errorf("%s: catalogue source %q is not the card source", card.ID, anchor)
			}
		}
		if card.Source.Mirrored {
			if err := checkSupplementClasses(directory, build.rows); err != nil {
				return nil, err
			}
		}
		if len(build.rows) != card.ROCount {
			return nil, fmt.Errorf("%s: parsed %d read-only rows, the reference lists %d", card.ID, len(build.rows), card.ROCount)
		}
		if in.exists("semantic-review/" + card.Slug + "-controls.md") {
			controls, err := in.text("semantic-review/" + card.Slug + "-controls.md")
			if err != nil {
				return nil, err
			}
			build.readable, err = parseReadableConfiguration(controls)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", card.ID, err)
			}
		}
	}

	coreCount, err := attachCore(in, cards, builds)
	if err != nil {
		return nil, err
	}

	locatorData, err := os.ReadFile(opts.ModelRows)
	if err != nil {
		return nil, err
	}
	models, err := parseLocatorRows(locatorData)
	if err != nil {
		return nil, err
	}
	locatorCount := len(models)
	for _, card := range cards {
		if card.Source.Mirrored {
			for _, model := range parseMB02Models(builds[card.ID].markdown) {
				model.SHA256 = card.Source.SHA256
				models = append(models, model)
			}
		}
	}
	statements := map[string][]cardTuple{}
	for _, card := range cards {
		statements[card.Source.SHA256] = append(statements[card.Source.SHA256], cardTuples(card.EvidenceText)...)
	}
	coverageText, err := in.text("appendices/model-coverage.md")
	if err != nil {
		return nil, err
	}
	for anchor, tuples := range coverageTuples(coverageText) {
		entry, ok := anchors[anchor]
		if !ok {
			return nil, fmt.Errorf("model coverage cites unknown source %s", anchor)
		}
		statements[entry.SHA256] = append(statements[entry.SHA256], tuples...)
	}
	notes, err := completeFloors(models, statements)
	if err != nil {
		return nil, err
	}
	flagModels(models)

	contradictions, err := in.text("appendices/contradictions.md")
	if err != nil {
		return nil, err
	}
	referenceItems, quarantineOffsets, quarantineEditions, err := referenceQuarantines(contradictions, anchors)
	if err != nil {
		return nil, err
	}

	referenced := map[string]*censusEntry{}
	bySHA := map[string]*censusEntry{}
	for _, entry := range census {
		bySHA[entry.SHA256] = entry
	}
	for _, model := range models {
		entry, ok := bySHA[model.SHA256]
		if !ok {
			return nil, fmt.Errorf("model row names an edition outside the source appendix")
		}
		referenced[entry.SHA256] = entry
	}
	cardBySHA := map[string]string{}
	for _, card := range cards {
		referenced[card.Source.SHA256] = card.Source
		cardBySHA[card.Source.SHA256] = card.ID
	}
	for _, list := range quarantineEditions {
		for _, entry := range list {
			referenced[entry.SHA256] = entry
		}
	}
	entries := make([]*censusEntry, 0, len(referenced))
	for _, entry := range referenced {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].SHA256 < entries[j].SHA256 })
	editionBySHA, err := buildEditions(entries, cardBySHA)
	if err != nil {
		return nil, err
	}
	for index := range models {
		models[index].Row.Edition = editionBySHA[models[index].SHA256].ID
	}

	c := &catalogue{Registers: map[string]*registersDoc{}}
	c.Editions = editionsDoc{Schema: editionsSchema, Attribution: attributionNote}
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

	var dictionaries []dictionary
	var coverage []cardCoverage
	for _, card := range cards {
		build := builds[card.ID]
		editionID := editionBySHA[card.Source.SHA256].ID
		sortRows(build.rows)
		sortRows(build.readable)
		for _, list := range [][]parsedRow{build.rows, build.readable} {
			for index := range list {
				row := &list[index].Row
				if row.Offset == nil {
					builder.flagRegister(quarantineTemplateRow, card.ID, editionID, row)
					continue
				}
				for _, item := range referenceItems {
					for _, entry := range quarantineEditions[item.ID] {
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
			SelectionNote: prose(card.Note), Gates: build.gates, FunctionalClasses: build.classes,
		}
		for _, row := range build.rows {
			doc.Rows = append(doc.Rows, row.Row)
		}
		for _, row := range build.readable {
			doc.ReadableConfiguration = append(doc.ReadableConfiguration, row.Row)
		}
		c.Registers[card.CatalogueFile] = doc

		switch {
		case build.review != nil:
			items, err := reviewDictionaries(card, editionID, build.review, build.markdown, build.rows)
			if err != nil {
				return nil, err
			}
			dictionaries = append(dictionaries, items...)
		case card.Source.Mirrored:
			item, err := mb02AlarmDictionary(card, editionID, build.markdown)
			if err != nil {
				return nil, err
			}
			dictionaries = append(dictionaries, item)
		}
		dictionaries = append(dictionaries, selectorDictionaries(card, editionID, build.readable)...)
		if card.Source.Alias == "M10" {
			attached, err := in.text("08-attached-interfaces.md")
			if err != nil {
				return nil, err
			}
			status, err := in.text("07-status-alarms.md")
			if err != nil {
				return nil, err
			}
			items, err := chapterDictionaries(card, editionID, attached, status)
			if err != nil {
				return nil, err
			}
			dictionaries = append(dictionaries, items...)
		}
		coverage = append(coverage, coverCard(card, build, doc, models))
	}
	sort.Slice(dictionaries, func(i, j int) bool {
		if dictionaries[i].Card != dictionaries[j].Card {
			return dictionaries[i].Card < dictionaries[j].Card
		}
		if dictionaries[i].Offsets[0] != dictionaries[j].Offsets[0] {
			return dictionaries[i].Offsets[0] < dictionaries[j].Offsets[0]
		}
		return dictionaries[i].ID < dictionaries[j].ID
	})
	c.Dictionaries = dictionariesDoc{Schema: dictionariesSchema, Dictionaries: dictionaries}

	sort.SliceStable(models, func(i, j int) bool {
		a, b := models[i], models[j]
		if a.Row.Edition != b.Row.Edition {
			return a.Row.Edition < b.Row.Edition
		}
		if a.Row.Page != b.Row.Page {
			return a.Row.Page < b.Row.Page
		}
		if a.Row.SourceSet != b.Row.SourceSet {
			return a.Row.SourceSet > b.Row.SourceSet
		}
		return a.Order < b.Order
	})
	for index := range models {
		row := &models[index].Row
		for _, flag := range append([]string(nil), row.Quarantine...) {
			builder.recordModel(flag, row)
		}
		for _, spec := range anomalySpecs {
			if spec.FloorSuffix != "" && strings.Contains(row.MinimumSoftware, spec.FloorSuffix) {
				builder.flagModel(spec.ID, row)
			}
		}
	}
	c.Models = modelsDoc{Schema: modelsSchema}
	for _, model := range models {
		c.Models.Rows = append(c.Models.Rows, model.Row)
	}
	c.Quarantines = quarantinesDoc{Schema: quarantinesSchema, Quarantines: builder.build()}

	got := countCatalogue(c)
	if got.Models != locatorCount || got.Core != coreCount || got.RO != statedRO {
		return nil, fmt.Errorf("internal accounting mismatch: %s", got)
	}
	if got != opts.Expect {
		return nil, fmt.Errorf("counts %s differ from the expected reference accounting %s; review the reference change and rerun with -expect %s", got, opts.Expect, got)
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
	sidecar, err := buildSidecar(builds, cards, models, editionBySHA, coverage, got, notes)
	if err != nil {
		return nil, err
	}
	return &result{Catalogue: c, Files: files, Sidecar: sidecar, Counts: got, Coverage: coverage, Notes: notes}, nil
}

// attachCore marks the register rows that the reference lists as core rows.
func attachCore(in reader, cards []*cardInfo, builds map[string]*cardBuild) (int, error) {
	text, err := in.text("appendices/core-semantic-review.json")
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
	if err := json.Unmarshal([]byte(text), &core); err != nil {
		return 0, err
	}
	perCard := map[string]int{}
	for _, entry := range core.Rows {
		build, ok := builds[strings.ToUpper(entry.Profile)]
		if !ok {
			return 0, fmt.Errorf("core row names unknown card %s", entry.Profile)
		}
		matched := 0
		for index := range build.rows {
			if build.rows[index].Evidence == entry.EvidenceID {
				if build.rows[index].Row.Core != nil {
					return 0, fmt.Errorf("core row %s matches a row twice", entry.FunctionalKey)
				}
				build.rows[index].Row.Core = &coreRef{Key: entry.FunctionalKey, Gate: cleanText(entry.Gate)}
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

func coverCard(card *cardInfo, build *cardBuild, doc *registersDoc, models []parsedModel) cardCoverage {
	item := cardCoverage{
		Card: card.ID, Parser: build.parser, Rows: len(doc.Rows), StatedRows: card.ROCount,
		Classes: map[string]int{}, Endpoints: map[string]int{}, Readable: len(doc.ReadableConfiguration),
		QuarantineRows: map[string]int{}, TemplateRows: []string{},
	}
	for _, row := range doc.Rows {
		item.Classes[row.Class]++
		item.Endpoints[row.Endpoint]++
		if row.Core != nil {
			item.Core++
		}
		if row.Offset == nil {
			item.TemplateRows = append(item.TemplateRows, row.Key)
		}
		for _, flag := range row.Quarantine {
			item.QuarantineRows[flag]++
		}
	}
	for _, model := range models {
		if model.SHA256 == card.Source.SHA256 {
			item.ModelRows++
		}
	}
	return item
}

var supplementClassPattern = regexp.MustCompile(`(\d+) reviewed, (\d+) conditional, (\d+) raw-only and (\d+) conflict`)

// checkSupplementClasses compares the class counts of the supplement card with
// the counts the profile directory states for it.
func checkSupplementClasses(directory string, rows []parsedRow) error {
	match := supplementClassPattern.FindStringSubmatch(directory)
	if match == nil {
		return fmt.Errorf("the profile directory states no supplement class counts")
	}
	got := map[string]int{}
	for _, row := range rows {
		got[row.Row.Class]++
	}
	for index, class := range []string{classReviewed, classConditional, classRawOnly, classConflict} {
		want, _ := strconv.Atoi(match[index+1])
		if got[class] != want {
			return fmt.Errorf("supplement card: %d %s rows, the profile directory states %d", got[class], class, want)
		}
	}
	return nil
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

// buildSidecar returns the private locator files: digests, table and row
// locators, native model lines and per-card accounting. They are never
// committed.
func buildSidecar(builds map[string]*cardBuild, cards []*cardInfo, models []parsedModel, editions map[string]*edition, coverage []cardCoverage, got counts, notes []string) (map[string][]byte, error) {
	type sidecarEdition struct {
		ID      string   `json:"id"`
		SHA256  string   `json:"sha256"`
		Section []string `json:"census_lines"`
	}
	type sidecarRow struct {
		Card       string            `json:"card"`
		Key        string            `json:"key"`
		Offset     *int              `json:"offset"`
		Occurrence int               `json:"occurrence,omitempty"`
		Evidence   string            `json:"evidence,omitempty"`
		Private    map[string]string `json:"private"`
	}
	type sidecarModel struct {
		Edition    string            `json:"edition"`
		Page       int               `json:"page"`
		Token      string            `json:"token,omitempty"`
		Identifier *int              `json:"identifier,omitempty"`
		Private    map[string]string `json:"private"`
	}
	var editionRows []sidecarEdition
	shas := make([]string, 0, len(editions))
	for sha := range editions {
		shas = append(shas, sha)
	}
	sort.Strings(shas)
	var censusLines = map[string][]string{}
	for _, card := range cards {
		censusLines[card.Source.SHA256] = card.Source.Lines
	}
	for _, sha := range shas {
		editionRows = append(editionRows, sidecarEdition{ID: editions[sha].ID, SHA256: sha, Section: censusLines[sha]})
	}
	var rows []sidecarRow
	for _, card := range cards {
		build := builds[card.ID]
		for _, list := range [][]parsedRow{build.rows, build.readable} {
			for _, row := range list {
				rows = append(rows, sidecarRow{Card: card.ID, Key: row.Row.Key, Offset: row.Row.Offset, Occurrence: row.Row.Occurrence, Evidence: row.Evidence, Private: row.Private})
			}
		}
	}
	var modelRows []sidecarModel
	for _, model := range models {
		modelRows = append(modelRows, sidecarModel{Edition: model.Row.Edition, Page: model.Row.Page, Token: model.Row.Token, Identifier: model.Row.Identifier, Private: model.Private})
	}
	out := map[string][]byte{}
	for name, value := range map[string]any{
		"editions.json": editionRows, "registers.json": rows, "models.json": modelRows,
		"accounting.json": map[string]any{"counts": got, "coverage": coverage, "notes": notes},
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
	return "read-only rows " + strconv.Itoa(c.RO) + ", core rows " + strconv.Itoa(c.Core) +
		", model rows " + strconv.Itoa(c.Models) + ", cards " + strconv.Itoa(c.Cards)
}
