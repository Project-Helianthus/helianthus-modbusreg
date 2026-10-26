package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// parsedRow is a register or control row with the locator fields that stay
// private and the twin data that dictionaries are derived from.
type parsedRow struct {
	Row       registerRow
	Evidence  string
	TwinIDs   []string
	Meaning   map[string]json.RawMessage
	Condition string
	Enum      []dictValue
	Private   map[string]string
	Order     int
}

// twinDoc is the deterministic JSON twin of one profile card.
type twinDoc struct {
	Schema               string            `json:"schema"`
	Profile              string            `json:"profile"`
	Baseline             string            `json:"baseline"`
	SourceID             string            `json:"source_id"`
	Models               []twinModel       `json:"models"`
	ReadOnly             json.RawMessage   `json:"read_only"`
	Controls             json.RawMessage   `json:"controls"`
	Extents              []twinExtent      `json:"documented_read_extents"`
	Quarantines          []twinQuarantine  `json:"quarantines"`
	DocumentRequirements []twinRequirement `json:"document_requirements"`
	Additive             *twinAdditive     `json:"additive_documentary_support"`
}

type twinExtent struct {
	Start  int `json:"start"`
	End    int `json:"end_inclusive"`
	Fields []struct {
		Address int `json:"address"`
		Words   int `json:"words"`
		End     int `json:"end_inclusive"`
	} `json:"fields"`
	Quantity       int    `json:"quantity_words"`
	Safe           []int  `json:"safe_quantities_from_start"`
	Maximum        int    `json:"maximum_value_aligned_quantity_from_start"`
	FollowingHole  []int  `json:"following_source_hole"`
	Classification string `json:"following_gap_classification"`
}

type twinQuarantine struct {
	Section string `json:"section"`
	EntryID string `json:"entry_id"`
	Status  string `json:"status"`
}

type twinAdditive struct {
	SourceID string `json:"source_id"`
	Model    struct {
		Token   string `json:"source_model_token"`
		ModelID int    `json:"model_id"`
		Minimum string `json:"documented_minimum"`
		Page    int    `json:"physical_page"`
	} `json:"model"`
	Rows []struct {
		Address int    `json:"address"`
		Words   int    `json:"words"`
		Type    string `json:"type"`
		Meaning string `json:"meaning"`
		Page    int    `json:"physical_page"`
	} `json:"rows"`
	Conditions string `json:"conditions"`
}

var statusClasses = map[string]string{
	"DOCUMENT_REVIEWED":             classReviewed,
	"DOCUMENT_REVIEWED_CONDITIONAL": classConditional,
	"RAW_ONLY_UNDEFINED":            classRawOnly,
	"RAW_ONLY_MEANING_UNRESOLVED":   classRawOnly,
	"CONFLICT":                      classConflict,
	"QUARANTINED_SEMANTIC_CONFLICT": classConflict,
	"Reviewed":                      classReviewed,
	"Conditional":                   classConditional,
	"Raw only":                      classRawOnly,
	"Conflict":                      classConflict,
}

var endpointNames = map[string]string{
	"inverter":                         endpointInverter,
	"pv_inverter":                      endpointInverter,
	"sun2000mb_inverter":               endpointInverter,
	"inverter_and_associated_channels": endpointInverter,
	"":                                 endpointInverter,
	"EMMA":                             endpointEMMA,
	"emma_unit0":                       endpointEMMA,
	"EMMA-associated-device-public-interface": endpointEMMAAssociate,
	"emma_associated_device":                  endpointEMMAAssociate,
}

// Control metadata statuses. Only the documented statuses of an RW row with a
// documented FC03 association make a readable-configuration row; every other
// control row is no-send.
var controlStatuses = map[string]bool{
	"DOCUMENTED_METADATA":                           true,
	"DOCUMENTED_CONTROL_METADATA":                   true,
	"DOCUMENTED_METADATA_WITH_EXPLICIT_VALUE_FACTS": true,
	"DOCUMENTED_METADATA_VALUE_CONTRACT_UNKNOWN":    true,
	"CONFLICT":                        false,
	"QUARANTINED_SPECIFIC_CONFLICT":   false,
	"RESTRICTED_INTERFACE":            false,
	"DOCUMENTED_RESTRICTED_INTERFACE": false,
}

func literalCell(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || text == "None" {
		return floorNotStated
	}
	return text
}

// literalOf renders one literal cell of a twin: null and blank read "not
// stated", numbers keep their JSON text, strings stay as printed.
func literalOf(raw json.RawMessage) (string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return floorNotStated, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return literalCell(text), nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String(), nil
	}
	return "", fmt.Errorf("literal cell %s is neither text nor number", trimmed)
}

func applyScale(row *registerRow, gain, unit string) {
	cells := &scale{Gain: gain, Unit: unit}
	if row.Class == classReviewed || row.Class == classConditional {
		row.Decode = cells
		return
	}
	row.Literal = cells
}

func intPointer(value int) *int { return &value }

func endpointOf(name string) (string, error) {
	endpoint, ok := endpointNames[name]
	if !ok {
		return "", fmt.Errorf("unknown endpoint %q", name)
	}
	return endpoint, nil
}

func schemaOf(raw json.RawMessage) (string, error) {
	var head struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return "", err
	}
	return head.Schema, nil
}

// cardRows is everything one twin contributes to its register file.
type cardRows struct {
	Rows       []parsedRow
	Controls   []parsedRow
	Gates      []gate
	Classes    []functionalClass
	AlarmBits  []alarmBit
	Modules    []dictValue
	StatusEnum *statusEnum
	Parser     string
}

type statusEnum struct {
	Offset   int
	Pages    []int
	Values   []dictValue
	Unlisted string
}

// parseReadOnly dispatches on the schema of the read-only section.
func parseReadOnly(raw json.RawMessage, sourceID string, out *cardRows) error {
	schema, err := schemaOf(raw)
	if err != nil {
		return err
	}
	switch schema {
	case "huawei-pv-canonical-full-ro-review-v1":
		out.Parser = "canonical review twin"
		return parseFullReview(raw, sourceID, out)
	case "huawei-pv-base-profile-semantic-review-v1":
		out.Parser = "base review twin"
		return parseBaseReview(raw, sourceID, out)
	case "huawei-pv-mb02-semantic-review-v1":
		out.Parser = "supplement review twin"
		return parseMB02Review(raw, sourceID, out)
	}
	return fmt.Errorf("unknown read-only schema %q", schema)
}

type fullReview struct {
	Profile string `json:"profile"`
	Classes []struct {
		ID    string `json:"id"`
		Level string `json:"level"`
		Rule  string `json:"rule"`
	} `json:"classes"`
	Entries []struct {
		FunctionalKey string `json:"functional_key"`
		ClassID       string `json:"class_id"`
		DocumentUse   string `json:"document_use"`
		Tuple         struct {
			ReadFC     int             `json:"read_fc"`
			Address    int             `json:"address"`
			Words      int             `json:"words"`
			SourceType string          `json:"source_type"`
			Gain       json.RawMessage `json:"gain_literal"`
			Unit       json.RawMessage `json:"unit_literal"`
			Access     string          `json:"access"`
		} `json:"tuple"`
		Source struct {
			SourceID     string `json:"source_id"`
			PhysicalPage int    `json:"physical_page"`
			EvidenceID   string `json:"evidence_id"`
			SourceRow    string `json:"source_row"`
			Section      string `json:"section"`
		} `json:"source"`
		RowCondition *string `json:"row_condition"`
		Endpoint     string  `json:"endpoint_namespace"`
	} `json:"entries"`
	AlarmBits []struct {
		RegisterAddress int     `json:"register_address"`
		Bit             int     `json:"bit"`
		AlarmID         int     `json:"alarm_id"`
		FunctionalLabel string  `json:"functional_label"`
		Severity        string  `json:"severity"`
		FootnoteID      *string `json:"footnote_id"`
		PhysicalPage    int     `json:"physical_page"`
	} `json:"alarm_bits"`
	Coverage struct {
		DetectedRORows int            `json:"detected_ro_rows"`
		StatusCounts   map[string]int `json:"status_counts"`
	} `json:"coverage"`
	OperatingStatusEnum *struct {
		Address     int               `json:"address"`
		SourcePages []int             `json:"source_pages"`
		Values      map[string]string `json:"values"`
		Unlisted    string            `json:"unlisted"`
	} `json:"operating_status_enum"`
}

func parseFullReview(raw json.RawMessage, sourceID string, out *cardRows) error {
	var review fullReview
	if err := json.Unmarshal(raw, &review); err != nil {
		return err
	}
	for _, class := range review.Classes {
		mapped, ok := statusClasses[class.Level]
		if !ok {
			return fmt.Errorf("class %s has unknown level %q", class.ID, class.Level)
		}
		out.Classes = append(out.Classes, functionalClass{ID: class.ID, Class: mapped, Rule: prose(class.Rule)})
	}
	statuses := map[string]int{}
	for index, entry := range review.Entries {
		class, ok := statusClasses[entry.DocumentUse]
		if !ok {
			return fmt.Errorf("%s: unknown level %q", entry.FunctionalKey, entry.DocumentUse)
		}
		statuses[entry.DocumentUse]++
		endpoint, err := endpointOf(entry.Endpoint)
		if err != nil {
			return fmt.Errorf("%s: %w", entry.FunctionalKey, err)
		}
		if entry.Tuple.ReadFC != 3 || entry.Tuple.Access != "RO" || entry.Source.SourceID != sourceID {
			return fmt.Errorf("%s: not an FC03 read-only row of the card source", entry.FunctionalKey)
		}
		gain, err := literalOf(entry.Tuple.Gain)
		if err != nil {
			return err
		}
		unit, err := literalOf(entry.Tuple.Unit)
		if err != nil {
			return err
		}
		row := registerRow{
			Key: entry.FunctionalKey, Endpoint: endpoint, Offset: intPointer(entry.Tuple.Address), Quantity: entry.Tuple.Words,
			Type: entry.Tuple.SourceType, Access: entry.Tuple.Access, Class: class, FunctionalClass: entry.ClassID,
			Page: entry.Source.PhysicalPage,
		}
		parsed := parsedRow{
			Evidence: entry.Source.EvidenceID, TwinIDs: []string{entry.FunctionalKey, entry.Source.EvidenceID},
			Private: map[string]string{"evidence": entry.Source.EvidenceID, "source_row": entry.Source.SourceRow, "section": entry.Source.Section},
			Order:   index,
		}
		if entry.RowCondition != nil {
			row.Condition = prose(*entry.RowCondition)
			parsed.Condition = *entry.RowCondition
		}
		applyScale(&row, gain, unit)
		parsed.Row = row
		out.Rows = append(out.Rows, parsed)
	}
	if review.Coverage.DetectedRORows != len(out.Rows) {
		return fmt.Errorf("%d rows, the twin states %d", len(out.Rows), review.Coverage.DetectedRORows)
	}
	if err := sameCounts(statuses, review.Coverage.StatusCounts); err != nil {
		return err
	}
	for _, bit := range review.AlarmBits {
		module := modeNoneStated
		if bit.FootnoteID != nil {
			module = *bit.FootnoteID
		}
		out.AlarmBits = append(out.AlarmBits, alarmBit{
			Offset: intPointer(bit.RegisterAddress), Bit: bit.Bit, AlarmID: bit.AlarmID, Label: bit.FunctionalLabel,
			Severity: bit.Severity, Module: module, Page: bit.PhysicalPage,
		})
	}
	if enum := review.OperatingStatusEnum; enum != nil {
		values := make([]dictValue, 0, len(enum.Values))
		for code, meaning := range enum.Values {
			values = append(values, dictValue{Code: code, Meaning: meaning})
		}
		sortCodes(values)
		out.StatusEnum = &statusEnum{Offset: enum.Address, Pages: distinctSorted(enum.SourcePages), Values: values, Unlisted: cleanText(enum.Unlisted)}
	}
	return nil
}

type baseReview struct {
	Conditions map[string]string `json:"conditions"`
	AlarmBits  []struct {
		Word    string   `json:"register_or_word_literal"`
		Address *int     `json:"register_address"`
		Bit     int      `json:"bit"`
		AlarmID *int     `json:"alarm_id"`
		Meaning string   `json:"meaning_key"`
		Sever   string   `json:"severity_key"`
		Markers []string `json:"conditional_source_markers"`
		Source  struct {
			Page int `json:"physical_page"`
		} `json:"source"`
	} `json:"alarm_bits"`
	Entries []struct {
		CapabilityClass string                     `json:"capability_class"`
		FunctionalKey   string                     `json:"functional_key"`
		Endpoint        string                     `json:"endpoint"`
		Status          string                     `json:"status"`
		ConditionIDs    []string                   `json:"condition_ids"`
		Meaning         map[string]json.RawMessage `json:"meaning"`
		Unresolved      []string                   `json:"unresolved"`
		Tuple           struct {
			Access  json.RawMessage `json:"access"`
			Type    json.RawMessage `json:"type"`
			Unit    json.RawMessage `json:"unit"`
			Gain    json.RawMessage `json:"gain"`
			Address json.RawMessage `json:"address"`
			Words   json.RawMessage `json:"words"`
		} `json:"source_tuple_literal"`
		AddressContract *struct {
			Status string `json:"status"`
		} `json:"address_contract"`
		EvidenceID   string `json:"evidence_id"`
		FunctionCode int    `json:"function_code"`
		SourceID     string `json:"source_id"`
		Page         int    `json:"physical_page"`
	} `json:"entries"`
	Coverage struct {
		DetectedRORows int            `json:"detected_RO_rows"`
		StatusCounts   map[string]int `json:"status_counts"`
	} `json:"coverage"`
}

func parseBaseReview(raw json.RawMessage, sourceID string, out *cardRows) error {
	var review baseReview
	if err := json.Unmarshal(raw, &review); err != nil {
		return err
	}
	ids := make([]string, 0, len(review.Conditions))
	for id := range review.Conditions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		out.Gates = append(out.Gates, gate{ID: id, Meaning: prose(review.Conditions[id])})
	}
	statuses := map[string]int{}
	for index, entry := range review.Entries {
		class, ok := statusClasses[entry.Status]
		if !ok {
			return fmt.Errorf("%s: unknown status %q", entry.EvidenceID, entry.Status)
		}
		statuses[entry.Status]++
		endpoint, err := endpointOf(entry.Endpoint)
		if err != nil {
			return fmt.Errorf("%s: %w", entry.EvidenceID, err)
		}
		cells := map[string]string{}
		for name, value := range map[string]json.RawMessage{"access": entry.Tuple.Access, "type": entry.Tuple.Type, "unit": entry.Tuple.Unit, "gain": entry.Tuple.Gain, "address": entry.Tuple.Address, "words": entry.Tuple.Words} {
			cell, err := literalOf(value)
			if err != nil {
				return fmt.Errorf("%s: %w", entry.EvidenceID, err)
			}
			cells[name] = cell
		}
		quantity, err := strconv.Atoi(cells["words"])
		if err != nil || entry.FunctionCode != 3 || cells["access"] != "RO" || entry.SourceID != sourceID {
			return fmt.Errorf("%s: not an FC03 read-only row of the card source", entry.EvidenceID)
		}
		row := registerRow{
			Key: entry.FunctionalKey, Endpoint: endpoint, Quantity: quantity, Type: cells["type"], Access: "RO",
			Class: class, FunctionalClass: entry.CapabilityClass, Page: entry.Page,
		}
		switch {
		case entry.AddressContract != nil:
			if entry.AddressContract.Status != addressUnspecified {
				return fmt.Errorf("%s: unknown address contract %q", entry.EvidenceID, entry.AddressContract.Status)
			}
			row.AddressStatus = addressUnspecified
		default:
			offset, err := strconv.Atoi(cells["address"])
			if err != nil {
				return fmt.Errorf("%s: address %q is not a number and carries no address contract", entry.EvidenceID, cells["address"])
			}
			row.Offset = intPointer(offset)
		}
		row.Gates = append(row.Gates, entry.ConditionIDs...)
		if len(entry.Unresolved) > 0 {
			row.Condition = prose(strings.Join(entry.Unresolved, " "))
		}
		applyScale(&row, cells["gain"], cells["unit"])
		out.Rows = append(out.Rows, parsedRow{
			Row: row, Evidence: entry.EvidenceID, TwinIDs: []string{entry.EvidenceID}, Meaning: entry.Meaning,
			Private: map[string]string{"evidence": entry.EvidenceID}, Order: index,
		})
	}
	if review.Coverage.DetectedRORows != len(out.Rows) {
		return fmt.Errorf("%d rows, the twin states %d", len(out.Rows), review.Coverage.DetectedRORows)
	}
	if err := sameCounts(statuses, review.Coverage.StatusCounts); err != nil {
		return err
	}
	for _, bit := range review.AlarmBits {
		if bit.AlarmID == nil || (bit.Address == nil && strings.TrimSpace(bit.Word) == "") {
			return fmt.Errorf("alarm bit %s/%d has no alarm identifier or word", bit.Word, bit.Bit)
		}
		module := modeNoneStated
		if len(bit.Markers) > 0 {
			module = strings.Join(bit.Markers, ",")
		}
		item := alarmBit{Offset: bit.Address, Bit: bit.Bit, AlarmID: *bit.AlarmID, Label: bit.Meaning, Severity: bit.Sever, Module: module, Page: bit.Source.Page}
		if bit.Address == nil {
			item.Word = cleanText(bit.Word)
		}
		out.AlarmBits = append(out.AlarmBits, item)
	}
	return nil
}

type mb02Review struct {
	Conditions []struct {
		ID                 string            `json:"id"`
		Rule               string            `json:"rule"`
		EvidencePages      []int             `json:"evidence_pages"`
		FootnoteConditions map[string]string `json:"footnote_conditions"`
	} `json:"conditions"`
	Classes []struct {
		ClassID         string `json:"class_id"`
		DocumentUse     string `json:"document_use"`
		ReviewedMeaning string `json:"reviewed_meaning"`
	} `json:"classes"`
	Entries []struct {
		EntryID     string `json:"entry_id"`
		ClassID     string `json:"class_id"`
		Label       string `json:"functional_label"`
		DocumentUse string `json:"document_use"`
		Tuple       struct {
			ReadFC  int             `json:"read_function_code"`
			Address int             `json:"address"`
			Length  int             `json:"register_length"`
			Type    string          `json:"source_type"`
			Access  string          `json:"access"`
			Gain    json.RawMessage `json:"gain"`
			Unit    json.RawMessage `json:"unit"`
		} `json:"tuple"`
		Provenance struct {
			SourceID string `json:"source_id"`
			Page     int    `json:"physical_page"`
			Evidence string `json:"geometry_evidence_id"`
		} `json:"provenance"`
		ConditionIDs []string `json:"condition_ids"`
		Endpoint     string   `json:"endpoint_namespace"`
	} `json:"entries"`
	AlarmBits []struct {
		RegisterAddress int     `json:"register_address"`
		Bit             int     `json:"bit"`
		AlarmID         int     `json:"alarm_id"`
		FunctionalLabel string  `json:"functional_label"`
		Severity        string  `json:"severity"`
		FootnoteID      *string `json:"footnote_id"`
		PhysicalPage    int     `json:"physical_page"`
	} `json:"alarm_bits"`
	Coverage struct {
		RORows       int            `json:"ro_rows"`
		StatusCounts map[string]int `json:"status_counts"`
	} `json:"coverage"`
}

func parseMB02Review(raw json.RawMessage, sourceID string, out *cardRows) error {
	var review mb02Review
	if err := json.Unmarshal(raw, &review); err != nil {
		return err
	}
	for _, condition := range review.Conditions {
		out.Gates = append(out.Gates, gate{ID: condition.ID, Meaning: prose(condition.Rule), Pages: condition.EvidencePages})
		codes := make([]string, 0, len(condition.FootnoteConditions))
		for code := range condition.FootnoteConditions {
			codes = append(codes, code)
		}
		sort.Strings(codes)
		for _, code := range codes {
			out.Modules = append(out.Modules, dictValue{Code: code, Meaning: cleanText(condition.FootnoteConditions[code])})
		}
	}
	for _, class := range review.Classes {
		mapped, ok := statusClasses[class.DocumentUse]
		if !ok {
			return fmt.Errorf("class %s has unknown level %q", class.ClassID, class.DocumentUse)
		}
		out.Classes = append(out.Classes, functionalClass{ID: class.ClassID, Class: mapped, Rule: prose(class.ReviewedMeaning)})
	}
	statuses := map[string]int{}
	for index, entry := range review.Entries {
		class, ok := statusClasses[entry.DocumentUse]
		if !ok {
			return fmt.Errorf("%s: unknown level %q", entry.EntryID, entry.DocumentUse)
		}
		statuses[entry.DocumentUse]++
		endpoint, err := endpointOf(entry.Endpoint)
		if err != nil {
			return fmt.Errorf("%s: %w", entry.EntryID, err)
		}
		if entry.Tuple.ReadFC != 3 || entry.Tuple.Access != "RO" || entry.Provenance.SourceID != sourceID {
			return fmt.Errorf("%s: not an FC03 read-only row of the card source", entry.EntryID)
		}
		gain, err := literalOf(entry.Tuple.Gain)
		if err != nil {
			return err
		}
		unit, err := literalOf(entry.Tuple.Unit)
		if err != nil {
			return err
		}
		row := registerRow{
			Key: "mb02." + entry.Label, Endpoint: endpoint, Offset: intPointer(entry.Tuple.Address), Quantity: entry.Tuple.Length,
			Type: entry.Tuple.Type, Access: "RO", Class: class, FunctionalClass: entry.ClassID, Page: entry.Provenance.Page,
		}
		row.Gates = append(row.Gates, entry.ConditionIDs...)
		applyScale(&row, gain, unit)
		out.Rows = append(out.Rows, parsedRow{
			Row: row, Evidence: entry.Provenance.Evidence, TwinIDs: []string{entry.EntryID, entry.Provenance.Evidence},
			Private: map[string]string{"evidence": entry.Provenance.Evidence, "entry": entry.EntryID}, Order: index,
		})
	}
	if review.Coverage.RORows != len(out.Rows) {
		return fmt.Errorf("%d rows, the twin states %d", len(out.Rows), review.Coverage.RORows)
	}
	if err := sameCounts(statuses, review.Coverage.StatusCounts); err != nil {
		return err
	}
	for _, bit := range review.AlarmBits {
		module := modeNoneStated
		if bit.FootnoteID != nil {
			module = *bit.FootnoteID
		}
		out.AlarmBits = append(out.AlarmBits, alarmBit{
			Offset: intPointer(bit.RegisterAddress), Bit: bit.Bit, AlarmID: bit.AlarmID, Label: bit.FunctionalLabel,
			Severity: bit.Severity, Module: module, Page: bit.PhysicalPage,
		})
	}
	return nil
}

// controlEntry is the common reading of the three control schemas.
type controlEntry struct {
	FunctionalKey string `json:"functional_key"`
	EntryID       string `json:"entry_id"`
	ClassID       string `json:"class_id"`
	Label         string `json:"functional_label"`
	Status        string `json:"metadata_status"`
	Namespace     string `json:"namespace"`
	Endpoint      string `json:"endpoint_namespace"`
	ScopeReview   string `json:"scope_review"`
	ReadFC        *int   `json:"read_fc"`
	ReadAssoc     *struct {
		FunctionCode *int `json:"function_code"`
	} `json:"read_function_association"`
	Tuple struct {
		Address int             `json:"address"`
		Words   int             `json:"words"`
		Length  int             `json:"register_length"`
		Type    string          `json:"source_type"`
		Access  string          `json:"access"`
		Gain    json.RawMessage `json:"gain"`
		Unit    json.RawMessage `json:"unit"`
		GainLit json.RawMessage `json:"gain_literal"`
		UnitLit json.RawMessage `json:"unit_literal"`
	} `json:"tuple"`
	Source struct {
		SourceID string `json:"source_id"`
		Page     int    `json:"physical_page"`
		Evidence string `json:"evidence_id"`
	} `json:"source"`
	Provenance struct {
		SourceID string `json:"source_id"`
		Page     int    `json:"physical_page"`
		Evidence string `json:"geometry_evidence_id"`
	} `json:"provenance"`
	ValueFacts *struct {
		EnumMembers []struct {
			Code  string `json:"code_literal"`
			Label string `json:"source_label_compacted"`
		} `json:"enum_members"`
	} `json:"value_facts"`
}

type controlsSection struct {
	Schema   string         `json:"schema"`
	Entries  []controlEntry `json:"entries"`
	Coverage struct {
		Rows                 int            `json:"rows"`
		Recognized           int            `json:"recognized_control_occurrences"`
		ControlRows          int            `json:"control_rows"`
		StatusCounts         map[string]int `json:"status_counts"`
		MetadataStatusCounts map[string]int `json:"metadata_status_counts"`
	} `json:"coverage"`
}

// parseControls reads the control section of a twin. Every control row keeps
// its literal cells and is either readable configuration or no-send.
func parseControls(raw json.RawMessage, slug, sourceID string, out *cardRows) error {
	var section controlsSection
	if err := json.Unmarshal(raw, &section); err != nil {
		return err
	}
	statuses := map[string]int{}
	for index, entry := range section.Entries {
		readable, known := controlStatuses[entry.Status]
		if !known {
			return fmt.Errorf("control %s %s: unknown status %q", entry.FunctionalKey, entry.EntryID, entry.Status)
		}
		statuses[entry.Status]++
		page, source, evidence := entry.Source.Page, entry.Source.SourceID, entry.Source.Evidence
		if source == "" {
			page, source, evidence = entry.Provenance.Page, entry.Provenance.SourceID, entry.Provenance.Evidence
		}
		if source != sourceID {
			return fmt.Errorf("control %s %s: not a row of the card source", entry.FunctionalKey, entry.EntryID)
		}
		readFC := entry.ReadFC
		if entry.ReadAssoc != nil && entry.ReadAssoc.FunctionCode != nil {
			readFC = entry.ReadAssoc.FunctionCode
		}
		quantity := entry.Tuple.Words
		if quantity == 0 {
			quantity = entry.Tuple.Length
		}
		gainRaw, unitRaw := entry.Tuple.GainLit, entry.Tuple.UnitLit
		if len(gainRaw) == 0 && len(unitRaw) == 0 {
			gainRaw, unitRaw = entry.Tuple.Gain, entry.Tuple.Unit
		}
		gain, err := literalOf(gainRaw)
		if err != nil {
			return err
		}
		unit, err := literalOf(unitRaw)
		if err != nil {
			return err
		}
		namespace := entry.Namespace
		if namespace == "" {
			namespace = entry.Endpoint
		}
		endpoint, err := endpointOf(namespace)
		if err != nil {
			return fmt.Errorf("control %s %s: %w", entry.FunctionalKey, entry.EntryID, err)
		}
		key := entry.FunctionalKey
		switch {
		case key != "":
		case strings.HasPrefix(entry.EntryID, "MB02-"):
			key = "mb02.control." + entry.Label
		default:
			key = slug + ".control." + entry.ClassID + "." + strconv.Itoa(entry.Tuple.Address)
		}
		class := classNoSend
		if readable && entry.Tuple.Access == "RW" && readFC != nil && *readFC == 3 {
			class = classReadableConfiguration
		}
		row := registerRow{
			Key: key, Endpoint: endpoint, Offset: intPointer(entry.Tuple.Address), Quantity: quantity, Type: entry.Tuple.Type,
			Access: entry.Tuple.Access, Class: class, FunctionalClass: entry.ClassID, Literal: &scale{Gain: gain, Unit: unit}, Page: page,
		}
		parsed := parsedRow{
			Row: row, Evidence: evidence, TwinIDs: nonEmpty(entry.FunctionalKey, entry.EntryID, evidence),
			Private: map[string]string{"evidence": evidence, "status": entry.Status}, Order: index,
		}
		if entry.ScopeReview != "" {
			parsed.Row.Condition = prose(entry.ScopeReview)
			parsed.Condition = entry.ScopeReview
		}
		if entry.ValueFacts != nil {
			for _, member := range entry.ValueFacts.EnumMembers {
				parsed.Enum = append(parsed.Enum, dictValue{Code: member.Code, Meaning: member.Label})
			}
		}
		out.Controls = append(out.Controls, parsed)
	}
	stated := section.Coverage.Rows
	if stated == 0 {
		stated = section.Coverage.Recognized
	}
	if stated == 0 {
		stated = section.Coverage.ControlRows
	}
	if stated != len(out.Controls) {
		return fmt.Errorf("%d control rows, the twin states %d", len(out.Controls), stated)
	}
	counts := section.Coverage.StatusCounts
	if counts == nil {
		counts = section.Coverage.MetadataStatusCounts
	}
	return sameCounts(statuses, counts)
}

func nonEmpty(values ...string) []string {
	var out []string
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func sameCounts(got, stated map[string]int) error {
	if len(stated) == 0 {
		return fmt.Errorf("the twin states no status counts")
	}
	for status, want := range stated {
		if got[status] != want {
			return fmt.Errorf("%d rows with status %s, the twin states %d", got[status], status, want)
		}
	}
	for status, count := range got {
		if stated[status] != count {
			return fmt.Errorf("%d rows with status %s, which the twin does not state", count, status)
		}
	}
	return nil
}

// readExtents converts the documented value-aligned read extents.
func readExtents(extents []twinExtent) ([]readExtent, error) {
	out := make([]readExtent, 0, len(extents))
	for _, extent := range extents {
		if extent.Quantity != extent.End-extent.Start+1 || extent.Maximum != extent.Quantity || len(extent.Fields) == 0 {
			return nil, fmt.Errorf("read extent at %d is inconsistent", extent.Start)
		}
		item := readExtent{Start: extent.Start, End: extent.End, Quantity: extent.Quantity, SafeQuantities: extent.Safe,
			FollowingHole: extent.FollowingHole, Hole: cleanText(extent.Classification)}
		for _, field := range extent.Fields {
			item.Offsets = append(item.Offsets, field.Address)
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Start != out[j].Start {
			return out[i].Start < out[j].Start
		}
		return out[i].End < out[j].End
	})
	return out, nil
}

var endpointRank = map[string]int{endpointInverter: 0, endpointEMMA: 1, endpointEMMAAssociate: 2}

// sortRows orders rows by endpoint, offset, key and page, with the private
// source order as the last key, and numbers repeated rows.
func sortRows(rows []parsedRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].Row, rows[j].Row
		if endpointRank[a.Endpoint] != endpointRank[b.Endpoint] {
			return endpointRank[a.Endpoint] < endpointRank[b.Endpoint]
		}
		ao, bo := offsetKey(a.Offset), offsetKey(b.Offset)
		if ao != bo {
			return ao < bo
		}
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		if a.Page != b.Page {
			return a.Page < b.Page
		}
		return rows[i].Order < rows[j].Order
	})
	seen := map[string]int{}
	for index := range rows {
		row := &rows[index].Row
		identity := rowIdentity(*row)
		seen[identity]++
		row.Occurrence = 0
		if seen[identity] > 1 {
			row.Occurrence = seen[identity]
		}
	}
	for index := range rows {
		row := &rows[index].Row
		if seen[rowIdentity(*row)] > 1 && row.Occurrence == 0 {
			row.Occurrence = 1
		}
	}
}

func offsetKey(offset *int) int {
	if offset == nil {
		return 1 << 30
	}
	return *offset
}

func sortCodes(values []dictValue) {
	sort.SliceStable(values, func(i, j int) bool {
		a, b := codeValue(values[i].Code), codeValue(values[j].Code)
		if a != b {
			return a < b
		}
		return values[i].Code < values[j].Code
	})
}

func codeValue(code string) int64 {
	lower := strings.ToLower(strings.TrimSpace(code))
	base := 10
	if strings.HasPrefix(lower, "0x") {
		lower, base = strings.TrimPrefix(lower, "0x"), 16
	}
	value, err := strconv.ParseInt(lower, base, 64)
	if err != nil {
		return 1 << 62
	}
	return value
}
