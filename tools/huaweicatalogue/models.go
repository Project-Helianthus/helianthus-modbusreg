package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// locatorRow is one entry of the model-row locator file.
type locatorRow struct {
	PDFSHA256      string `json:"pdf_sha256"`
	PhysicalPage   int    `json:"physical_page"`
	NativeModelRow string `json:"native_model_row"`
	Confidence     string `json:"confidence"`
	Classification string `json:"classification"`
}

// parsedModel is a model row with the locator fields that stay private.
type parsedModel struct {
	Row     modelRow
	SHA256  string
	Private map[string]string
	Order   int
}

var (
	floorGrammar         = regexp.MustCompile(`^(?:SUN\d+[A-Z]* )?V\d{3}R\d{3}(?:C\d{2}(?:SPC\d{3})?)?$`)
	modelThreeCellLine   = regexp.MustCompile(`^(\S+)\s{2,}(\d{1,4})\s{2,}(\S+(?: \S+)?)$`)
	modelRatingLine      = regexp.MustCompile(`^(\S+)\s{2,}(\d{1,4})\s{2,}(SUN\d+[A-Z]*)\s{2,}[0-9.]+(?:\s+[0-9.]+)+$`)
	modelIDFirstRating   = regexp.MustCompile(`^(\d{1,4})\s{2,}(SUN\S+)\s{2,}[0-9.]+(?:\s+[0-9.]+)+$`)
	modelIDFirstLine     = regexp.MustCompile(`^(\d{1,4})\s{2,}(SUN\S+)$`)
	modelTokenOnlyLine   = regexp.MustCompile(`^(SUN\S+)$`)
	revisionHistoryLine  = regexp.MustCompile(`^\d{2}\s{2,}\d{4}-\d{2}-\d{2}\s{2,}`)
	mapZeroTokenPattern  = regexp.MustCompile(`^SUN\d+_[0-9A-Z.]+_MAP0(?:_[0-9A-Z]+)?$`)
	mb02ModelRowPattern  = regexp.MustCompile(`^\| (SUN\S+) \| (\d+) \| (\S+) \| (\d+) \|\s*$`)
	wrappedTokenSuffixes = []string{"-", "_"}
)

// unnormalizedFloorSuffix is the literal floor suffix that the reference keeps
// unresolved (anomaly A06); such floors carry that anomaly instead of M02.
const unnormalizedFloorSuffix = "PSC110"

// Generator quarantines on model rows.
const (
	quarantineWrappedToken = "M01"
	quarantineFloorCell    = "M02"
	quarantineHistoryLine  = "M03"
	quarantineNoIdentifier = "M04"
	quarantineTemplateRow  = "X01"
)

// parseLocatorRows classifies every locator line. A line on a page that holds
// a revision-history entry is a history line; every other line must match one
// of the supported-model forms or the generator stops.
func parseLocatorRows(data []byte) ([]parsedModel, error) {
	var locators []locatorRow
	if err := json.Unmarshal(data, &locators); err != nil {
		return nil, err
	}
	historyPages := map[string]bool{}
	for _, locator := range locators {
		if revisionHistoryLine.MatchString(strings.TrimSpace(locator.NativeModelRow)) {
			historyPages[locator.PDFSHA256+":"+strconv.Itoa(locator.PhysicalPage)] = true
		}
	}
	var models []parsedModel
	var unplaced []string
	for index, locator := range locators {
		text := strings.TrimSpace(locator.NativeModelRow)
		row := modelRow{
			Page:            locator.PhysicalPage,
			Form:            formSupportedModel,
			MinimumSoftware: floorNotStated,
			FloorBasis:      floorNotStated,
			SourceSet:       sourceSetLocator,
		}
		switch match := matchModelLine(text); {
		case historyPages[locator.PDFSHA256+":"+strconv.Itoa(locator.PhysicalPage)]:
			row.Form = formRevisionHistory
			row.Quarantine = []string{quarantineHistoryLine}
		case match != nil:
			row.Token = match.token
			row.Identifier = match.identifier
			if match.floor != "" {
				row.MinimumSoftware = match.floor
				row.FloorBasis = floorBasisRow
			}
		default:
			unplaced = append(unplaced, fmt.Sprintf("page %d: %q", locator.PhysicalPage, text))
			continue
		}
		models = append(models, parsedModel{
			Row:    row,
			SHA256: locator.PDFSHA256,
			Private: map[string]string{
				"native_model_row": locator.NativeModelRow, "confidence": locator.Confidence,
				"classification": locator.Classification,
			},
			Order: index,
		})
	}
	if len(unplaced) > 0 {
		return nil, fmt.Errorf("%d model locator lines match no form: %s", len(unplaced), strings.Join(unplaced, "; "))
	}
	return models, nil
}

type modelLineMatch struct {
	token      string
	identifier *int
	floor      string
}

func matchModelLine(text string) *modelLineMatch {
	if match := modelThreeCellLine.FindStringSubmatch(text); match != nil {
		identifier, _ := strconv.Atoi(match[2])
		return &modelLineMatch{token: match[1], identifier: intPointer(identifier), floor: match[3]}
	}
	if match := modelRatingLine.FindStringSubmatch(text); match != nil {
		identifier, _ := strconv.Atoi(match[2])
		return &modelLineMatch{token: match[1], identifier: intPointer(identifier), floor: match[3]}
	}
	if match := modelIDFirstRating.FindStringSubmatch(text); match != nil {
		identifier, _ := strconv.Atoi(match[1])
		return &modelLineMatch{token: match[2], identifier: intPointer(identifier)}
	}
	if match := modelIDFirstLine.FindStringSubmatch(text); match != nil {
		identifier, _ := strconv.Atoi(match[1])
		return &modelLineMatch{token: match[2], identifier: intPointer(identifier)}
	}
	if match := modelTokenOnlyLine.FindStringSubmatch(text); match != nil {
		return &modelLineMatch{token: match[1]}
	}
	return nil
}

// parseMB02Models reads the exact model rows of the MB02 supplement card,
// which are outside the base locator set.
func parseMB02Models(text string) []parsedModel {
	var models []parsedModel
	inTable := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## ") {
			inTable = strings.TrimSpace(line) == "## Exact model rows"
			continue
		}
		if !inTable {
			continue
		}
		match := mb02ModelRowPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		identifier, _ := strconv.Atoi(match[2])
		page, _ := strconv.Atoi(match[4])
		models = append(models, parsedModel{
			Row: modelRow{
				Page: page, Form: formSupportedModel, Token: match[1], Identifier: intPointer(identifier),
				MinimumSoftware: match[3], FloorBasis: floorBasisRow, SourceSet: sourceSetCard,
			},
			Private: map[string]string{"card_row": line},
			Order:   len(models),
		})
	}
	return models
}

// completeFloors cross-checks the exact model statements of the cards and of
// the model coverage appendix against the rows of their edition. A statement
// completes a row whose floor cell is cut at the end of its locator line; a
// statement that contradicts a complete floor cell stops the generator.
func completeFloors(models []parsedModel, statements map[string][]cardTuple) ([]string, error) {
	var notes []string
	shas := make([]string, 0, len(statements))
	for sha := range statements {
		shas = append(shas, sha)
	}
	sort.Strings(shas)
	for _, sha := range shas {
		for _, tuple := range statements[sha] {
			if tuple.Floor == "" {
				continue
			}
			matched := 0
			for index := range models {
				model := &models[index]
				if model.SHA256 != sha || model.Row.Token != tuple.Token || model.Row.Identifier == nil || *model.Row.Identifier != tuple.Identifier {
					continue
				}
				if tuple.Page != 0 && model.Row.Page != tuple.Page {
					continue
				}
				matched++
				switch {
				case model.Row.MinimumSoftware == tuple.Floor:
				case !floorGrammar.MatchString(model.Row.MinimumSoftware) && strings.HasPrefix(tuple.Floor, model.Row.MinimumSoftware):
					model.Private["floor_cell"] = model.Row.MinimumSoftware
					model.Row.MinimumSoftware = tuple.Floor
					model.Row.FloorBasis = floorBasisReviewed
				default:
					return nil, fmt.Errorf("model %s/%d: floor cell %q contradicts the reviewed statement %q", tuple.Token, tuple.Identifier, model.Row.MinimumSoftware, tuple.Floor)
				}
			}
			if matched == 0 {
				notes = append(notes, fmt.Sprintf("reviewed statement %s/%d has no locator row in its edition", tuple.Token, tuple.Identifier))
			}
		}
	}
	return notes, nil
}

// flagModels assigns the generator quarantines and the wire tokens of the
// model rows.
func flagModels(models []parsedModel) {
	for index := range models {
		row := &models[index].Row
		if row.Form != formSupportedModel {
			continue
		}
		var flags []string
		for _, suffix := range wrappedTokenSuffixes {
			if strings.HasSuffix(row.Token, suffix) {
				flags = append(flags, quarantineWrappedToken)
				break
			}
		}
		if row.MinimumSoftware != floorNotStated && !floorGrammar.MatchString(row.MinimumSoftware) && !strings.Contains(row.MinimumSoftware, unnormalizedFloorSuffix) {
			flags = append(flags, quarantineFloorCell)
		}
		if row.Identifier == nil {
			flags = append(flags, quarantineNoIdentifier)
		}
		row.Quarantine = append(row.Quarantine, flags...)
		if mapZeroTokenPattern.MatchString(row.Token) {
			row.WireToken = strings.ReplaceAll(row.Token, "_", "-")
			row.WireTokenGrade = gradeHypothesis
		}
	}
}
