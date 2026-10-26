package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// twinModel is one row of a reference model table, as it appears in the card
// twins and in the per-edition model tables.
type twinModel struct {
	Token         string  `json:"source_model_token"`
	ModelID       *int    `json:"model_id"`
	Minimum       *string `json:"documented_minimum"`
	MinimumStatus string  `json:"minimum_status"`
	ModelIDStatus string  `json:"model_id_status"`
	SourceID      string  `json:"source_id"`
	Page          int     `json:"physical_page"`
	Table         int     `json:"table_index"`
	Row           int     `json:"row_index"`
	WireAliases   []struct {
		Token         string `json:"token"`
		Evidence      string `json:"evidence"`
		Qualification string `json:"qualification"`
	} `json:"wire_aliases"`
	Sequence       string   `json:"source_sequence_number"`
	Unknowns       []string `json:"unknowns"`
	RequirementIDs []string `json:"document_requirement_ids"`
}

type twinRequirement struct {
	ID            string `json:"id"`
	PredicateKind string `json:"predicate_kind"`
	SourceID      string `json:"source_id"`
	Page          int    `json:"physical_page"`
	Section       string `json:"section"`
	Minimum       string `json:"minimum_version"`
	Inclusive     bool   `json:"inclusive"`
	Scope         struct {
		Component   string   `json:"component"`
		ModelTokens []string `json:"applicable_model_tokens"`
	} `json:"scope"`
}

type modelTable struct {
	Schema               string            `json:"schema"`
	SourceID             string            `json:"source_id"`
	Rows                 []twinModel       `json:"rows"`
	EmptyReason          *string           `json:"empty_reason"`
	DocumentRequirements []twinRequirement `json:"document_requirements"`
}

// parsedModel is a model row with the locator fields that stay private.
type parsedModel struct {
	Row      modelRow
	SourceID string
	Private  map[string]string
	Order    int
}

var (
	floorGrammar = regexp.MustCompile(`^(?:SUN\d+[A-Z]* )?V\d{3}R\d{3}(?:C\d{2}(?:SPC\d{3})?)?$`)
	sectionSlug  = regexp.MustCompile(`[^a-z0-9]+`)
)

// unnormalizedFloorSuffix is the literal floor suffix that the reference keeps
// unresolved (anomaly A06); such floors carry that anomaly instead of M02.
const unnormalizedFloorSuffix = "PSC110"

// Generator quarantines on model rows.
const (
	quarantineSequenceOrdinal = "M01"
	quarantineFloorCell       = "M02"
)

func requirementID(editionID, section string) string {
	slug := strings.Trim(sectionSlug.ReplaceAllString(strings.ToLower(section), "-"), "-")
	return editionID + "/" + slug
}

// convertModelTable turns one per-edition model table into model rows and
// document-wide requirements. The floor is the row minimum or, when the row
// states none, the linked document-wide requirement.
func convertModelTable(table modelTable, editionID string) ([]parsedModel, []documentRequirement, error) {
	requirements := map[string]documentRequirement{}
	var ordered []documentRequirement
	for _, requirement := range table.DocumentRequirements {
		if requirement.SourceID != table.SourceID || !requirement.Inclusive || !floorGrammar.MatchString(requirement.Minimum) {
			return nil, nil, fmt.Errorf("%s: requirement %s is not an inclusive floor of this edition", table.SourceID, requirement.ID)
		}
		converted := documentRequirement{
			ID: requirementID(editionID, requirement.Section), Edition: editionID, Page: requirement.Page,
			Section: cleanText(requirement.Section), Minimum: requirement.Minimum, Inclusive: true,
			Component: cleanText(requirement.Scope.Component), ModelTokens: append([]string{}, requirement.Scope.ModelTokens...),
		}
		requirements[requirement.ID] = converted
		ordered = append(ordered, converted)
	}
	var models []parsedModel
	for index, source := range table.Rows {
		if source.SourceID != table.SourceID || source.Token == "" || source.Page < 1 {
			return nil, nil, fmt.Errorf("%s row %d is incomplete", table.SourceID, index)
		}
		row := modelRow{Edition: editionID, Page: source.Page, Token: source.Token, RowMinimum: floorNotStated}
		switch source.ModelIDStatus {
		case "stated_as_model_id":
			if source.ModelID == nil || source.Sequence != "" {
				return nil, nil, fmt.Errorf("%s %s: model identifier stated but absent", table.SourceID, source.Token)
			}
			row.Identifier = intPointer(*source.ModelID)
		case "not_stated_as_model_id":
			if source.ModelID != nil || source.Sequence == "" {
				return nil, nil, fmt.Errorf("%s %s: a row without a model identifier must carry its sequence number", table.SourceID, source.Token)
			}
			row.SequenceNumber = source.Sequence
			row.Quarantine = append(row.Quarantine, quarantineSequenceOrdinal)
		default:
			return nil, nil, fmt.Errorf("%s %s: unknown identifier status %q", table.SourceID, source.Token, source.ModelIDStatus)
		}
		switch source.MinimumStatus {
		case "stated_in_source":
			if source.Minimum == nil || *source.Minimum == "" {
				return nil, nil, fmt.Errorf("%s %s: minimum stated but absent", table.SourceID, source.Token)
			}
			row.RowMinimum = *source.Minimum
			row.MinimumSoftware = *source.Minimum
			row.FloorBasis = floorBasisRow
			if !floorGrammar.MatchString(row.RowMinimum) && !strings.Contains(row.RowMinimum, unnormalizedFloorSuffix) {
				row.Quarantine = append(row.Quarantine, quarantineFloorCell)
			}
		case "not_stated_in_model_row":
			if source.Minimum != nil || len(source.RequirementIDs) != 1 {
				return nil, nil, fmt.Errorf("%s %s: a row without a minimum must link exactly one document requirement", table.SourceID, source.Token)
			}
			requirement, ok := requirements[source.RequirementIDs[0]]
			if !ok {
				return nil, nil, fmt.Errorf("%s %s: unknown document requirement", table.SourceID, source.Token)
			}
			row.DocumentRequirement = requirement.ID
			row.MinimumSoftware = requirement.Minimum
			row.FloorBasis = floorBasisDocument
		default:
			return nil, nil, fmt.Errorf("%s %s: unknown minimum status %q", table.SourceID, source.Token, source.MinimumStatus)
		}
		for _, alias := range source.WireAliases {
			row.WireAliases = append(row.WireAliases, wireAlias{Token: alias.Token, Evidence: cleanText(alias.Evidence), Scope: prose(alias.Qualification)})
		}
		models = append(models, parsedModel{
			Row: row, SourceID: table.SourceID, Order: index,
			Private: map[string]string{
				"table": strconv.Itoa(source.Table), "row": strconv.Itoa(source.Row), "unknowns": strings.Join(source.Unknowns, " | "),
			},
		})
	}
	return models, ordered, nil
}

// sameModelTable checks that the model table of a card twin equals the
// per-edition table of its source.
func sameModelTable(twin []twinModel, table modelTable) error {
	if len(twin) != len(table.Rows) {
		return fmt.Errorf("%d twin model rows, %d edition rows", len(twin), len(table.Rows))
	}
	for index := range twin {
		a, b := twin[index], table.Rows[index]
		left, _ := json.Marshal(a)
		right, _ := json.Marshal(b)
		if string(left) != string(right) {
			return fmt.Errorf("model row %d differs between the twin and the edition table", index)
		}
	}
	return nil
}

func sortModels(models []parsedModel) {
	sort.SliceStable(models, func(i, j int) bool {
		a, b := models[i], models[j]
		if a.Row.Edition != b.Row.Edition {
			return a.Row.Edition < b.Row.Edition
		}
		if a.Row.Page != b.Row.Page {
			return a.Row.Page < b.Row.Page
		}
		return a.Order < b.Order
	})
}
