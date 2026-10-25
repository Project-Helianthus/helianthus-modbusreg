package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// parsedRow is a register row with the locator fields that stay private.
type parsedRow struct {
	Row      registerRow
	Evidence string
	Private  map[string]string
	Order    int
}

var (
	conditionBulletPattern = regexp.MustCompile("^- `([A-Z0-9_]+)`: (.+)$")
	classHeadingPattern    = regexp.MustCompile(`^## ([a-z0-9_]+(?:\.[a-z0-9_]+)+)\s*$`)
	addressWordsPattern    = regexp.MustCompile(`^(\d+|\.\.\.) / (\d+)$`)
	pageTableRowPattern    = regexp.MustCompile(`^p\. (\d+) / (t\d+) / (r\d+)$`)
	pageSourceRowPattern   = regexp.MustCompile(`^(\d+) / (\d+)$`)
	mb02ConditionPattern   = regexp.MustCompile(`^\| (G-[A-Z0-9-]+) \| (.+?) \| ([0-9, ]+) \|\s*$`)
	twoColumnPattern       = regexp.MustCompile(`^\| ([a-z0-9_]+) \| (.+?) \|\s*$`)
)

var statusClasses = map[string]string{
	"DOCUMENT_REVIEWED":             classReviewed,
	"DOCUMENT_REVIEWED_CONDITIONAL": classConditional,
	"RAW_ONLY_UNDEFINED":            classRawOnly,
	"CONFLICT":                      classConflict,
}

var levelClasses = map[string]string{
	"Reviewed":    classReviewed,
	"Conditional": classConditional,
	"Raw only":    classRawOnly,
	"Conflict":    classConflict,
}

var endpointNames = map[string]string{
	"inverter":    endpointInverter,
	"pv_inverter": endpointInverter,
	"EMMA":        endpointEMMA,
	"EMMA-associated-device-public-interface": endpointEMMAAssociate,
}

func tableCells(line string) []string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return nil
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(trimmed, "|"), "|"), "|")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts
}

func literalCell(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || text == "None" {
		return floorNotStated
	}
	return text
}

func applyScale(row *registerRow, gain, unit string) {
	cells := &scale{Gain: literalCell(gain), Unit: literalCell(unit)}
	if row.Class == classReviewed || row.Class == classConditional {
		row.Decode = cells
		return
	}
	row.Literal = cells
}

func intPointer(value int) *int { return &value }

// parseClassTables reads one of the thirteen derived class-table catalogues.
func parseClassTables(text string, sourceSHA string) ([]parsedRow, []gate, string, error) {
	var rows []parsedRow
	var gates []gate
	sourceAnchor := ""
	functionalClass := ""
	inConditions := false
	for number, line := range strings.Split(text, "\n") {
		if match := catalogueRefPattern.FindStringSubmatch(line); match != nil && sourceAnchor == "" {
			sourceAnchor = match[1]
		}
		if strings.HasPrefix(line, "## ") {
			inConditions = strings.TrimSpace(line) == "## Conditions"
			functionalClass = ""
			if match := classHeadingPattern.FindStringSubmatch(line); match != nil {
				functionalClass = match[1]
			}
			continue
		}
		if inConditions {
			if match := conditionBulletPattern.FindStringSubmatch(line); match != nil {
				gates = append(gates, gate{ID: match[1], Meaning: prose(match[2])})
			}
			continue
		}
		cells := tableCells(line)
		if functionalClass == "" || len(cells) != 6 || cells[0] == "Functional key" || strings.HasPrefix(cells[0], "---") {
			continue
		}
		address := addressWordsPattern.FindStringSubmatch(cells[2])
		literals := strings.Split(cells[3], ";")
		status := strings.SplitN(cells[4], ";", 2)
		locator := pageTableRowPattern.FindStringSubmatch(cells[5])
		if address == nil || len(literals) != 3 || len(status) != 2 || locator == nil {
			return nil, nil, "", fmt.Errorf("line %d: unrecognized class-table row %q", number+1, line)
		}
		class, ok := statusClasses[strings.TrimSpace(status[0])]
		if !ok {
			return nil, nil, "", fmt.Errorf("line %d: unknown status %q", number+1, status[0])
		}
		endpoint, ok := endpointNames[cells[1]]
		if !ok {
			return nil, nil, "", fmt.Errorf("line %d: unknown endpoint %q", number+1, cells[1])
		}
		quantity, _ := strconv.Atoi(address[2])
		page, _ := strconv.Atoi(locator[1])
		row := registerRow{
			Key:             cells[0],
			Endpoint:        endpoint,
			Quantity:        quantity,
			Type:            strings.TrimSpace(literals[0]),
			Access:          "RO",
			Class:           class,
			FunctionalClass: functionalClass,
			Page:            page,
		}
		if address[1] != "..." {
			offset, _ := strconv.Atoi(address[1])
			row.Offset = intPointer(offset)
		}
		for _, name := range strings.Split(status[1], ",") {
			if name = strings.TrimSpace(name); name != "" {
				row.Gates = append(row.Gates, name)
			}
		}
		applyScale(&row, literals[1], literals[2])
		rows = append(rows, parsedRow{
			Row:      row,
			Evidence: sourceSHA + ":p" + locator[1] + ":" + locator[2] + ":" + locator[3],
			Private:  map[string]string{"table": locator[2], "row": locator[3]},
			Order:    len(rows),
		})
	}
	return rows, gates, sourceAnchor, nil
}

// fullReview is the machine-readable review of the MC and MA-V200 cards.
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
			ReadFC     int     `json:"read_fc"`
			Address    int     `json:"address"`
			Words      int     `json:"words"`
			SourceType string  `json:"source_type"`
			Gain       *string `json:"gain_literal"`
			Unit       *string `json:"unit_literal"`
			Access     string  `json:"access"`
		} `json:"tuple"`
		Source struct {
			PDFSHA256    string `json:"pdf_sha256"`
			PhysicalPage int    `json:"physical_page"`
			EvidenceID   string `json:"evidence_id"`
			SourceRow    string `json:"source_row"`
			Section      string `json:"section"`
		} `json:"source"`
		ReadGate       string  `json:"read_gate"`
		UseGate        string  `json:"use_gate"`
		RowCondition   *string `json:"row_condition"`
		SentinelPolicy string  `json:"sentinel_policy"`
		Coherence      string  `json:"coherence"`
		Endpoint       string  `json:"endpoint_namespace"`
		RouteGate      string  `json:"route_gate"`
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

func stringOrNotStated(value *string) string {
	if value == nil {
		return floorNotStated
	}
	return literalCell(*value)
}

// parseFullReview reads a machine-readable review and checks it against the
// Markdown tuple table of the same card, cell by cell.
func parseFullReview(data []byte, markdown string, sourceSHA string) ([]parsedRow, []functionalClass, *fullReview, error) {
	var review fullReview
	if err := json.Unmarshal(data, &review); err != nil {
		return nil, nil, nil, err
	}
	var classes []functionalClass
	classLevel := map[string]string{}
	for _, class := range review.Classes {
		mapped, ok := levelClasses[class.Level]
		if !ok {
			return nil, nil, nil, fmt.Errorf("%s class %s has unknown level %q", review.Profile, class.ID, class.Level)
		}
		classLevel[class.ID] = mapped
		classes = append(classes, functionalClass{ID: class.ID, Class: mapped, Rule: prose(class.Rule)})
	}
	table := map[string][]string{}
	for _, line := range strings.Split(markdown, "\n") {
		cells := tableCells(line)
		if len(cells) == 9 && strings.Contains(cells[0], ".") && cells[0] != "Functional key" {
			table[cells[1]] = cells
		}
	}
	if len(table) != len(review.Entries) {
		return nil, nil, nil, fmt.Errorf("%s: %d Markdown rows for %d machine rows", review.Profile, len(table), len(review.Entries))
	}
	var rows []parsedRow
	for index, entry := range review.Entries {
		class, ok := levelClasses[entry.DocumentUse]
		if !ok {
			return nil, nil, nil, fmt.Errorf("%s %s: unknown level %q", review.Profile, entry.FunctionalKey, entry.DocumentUse)
		}
		endpoint, ok := endpointNames[entry.Endpoint]
		if !ok {
			return nil, nil, nil, fmt.Errorf("%s %s: unknown endpoint %q", review.Profile, entry.FunctionalKey, entry.Endpoint)
		}
		if entry.Tuple.ReadFC != 3 || entry.Tuple.Access != "RO" || entry.Source.PDFSHA256 != sourceSHA {
			return nil, nil, nil, fmt.Errorf("%s %s: not an FC03 read-only row of the card source", review.Profile, entry.FunctionalKey)
		}
		cells, ok := table[strconv.Itoa(entry.Tuple.Address)]
		gain, unit := stringOrNotStated(entry.Tuple.Gain), stringOrNotStated(entry.Tuple.Unit)
		wantCells := []string{
			entry.FunctionalKey, strconv.Itoa(entry.Tuple.Address), strconv.Itoa(entry.Tuple.Words), entry.Tuple.SourceType,
			gain, unit, strconv.Itoa(entry.Source.PhysicalPage) + " / " + entry.Source.SourceRow, entry.ClassID, entry.DocumentUse,
		}
		if !ok || withoutSpaces(strings.Join(cells, "|")) != withoutSpaces(strings.Join(wantCells, "|")) {
			return nil, nil, nil, fmt.Errorf("%s %s: Markdown row %q disagrees with the machine row %q", review.Profile, entry.FunctionalKey, cells, wantCells)
		}
		row := registerRow{
			Key:             entry.FunctionalKey,
			Endpoint:        endpoint,
			Offset:          intPointer(entry.Tuple.Address),
			Quantity:        entry.Tuple.Words,
			Type:            entry.Tuple.SourceType,
			Access:          entry.Tuple.Access,
			Class:           class,
			FunctionalClass: entry.ClassID,
			Page:            entry.Source.PhysicalPage,
		}
		private := map[string]string{
			"source_row": entry.Source.SourceRow, "section": entry.Source.Section, "read_gate": entry.ReadGate,
			"use_gate": entry.UseGate, "sentinel_policy": entry.SentinelPolicy, "coherence": entry.Coherence,
			"route_gate": entry.RouteGate,
		}
		if entry.RowCondition != nil {
			row.Condition = prose(*entry.RowCondition)
			private["condition_literal"] = *entry.RowCondition
		}
		applyScale(&row, gain, unit)
		rows = append(rows, parsedRow{Row: row, Evidence: entry.Source.EvidenceID, Private: private, Order: index})
	}
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.Row.Class]++
	}
	for level, want := range review.Coverage.StatusCounts {
		if counts[levelClasses[level]] != want {
			return nil, nil, nil, fmt.Errorf("%s: %d %s rows, the review states %d", review.Profile, counts[levelClasses[level]], level, want)
		}
	}
	if review.Coverage.DetectedRORows != len(rows) {
		return nil, nil, nil, fmt.Errorf("%s: %d rows, the review states %d", review.Profile, len(rows), review.Coverage.DetectedRORows)
	}
	return rows, classes, &review, nil
}

// parseMB02 reads the read-only tuple set, the conditions and the functional
// classes of the MB02 supplement card.
func parseMB02(text string) ([]parsedRow, []gate, []functionalClass, error) {
	var rows []parsedRow
	var gates []gate
	var classes []functionalClass
	section := ""
	for number, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## ") {
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			continue
		}
		switch section {
		case "Conditions":
			if match := mb02ConditionPattern.FindStringSubmatch(line); match != nil {
				gates = append(gates, gate{ID: match[1], Meaning: prose(match[2]), Pages: pageList(match[3])})
			}
		case "Functional classes":
			if match := twoColumnPattern.FindStringSubmatch(line); match != nil && match[1] != "class" {
				classes = append(classes, functionalClass{ID: match[1], Rule: prose(match[2])})
			}
		case "Complete detected RO tuple set":
			cells := tableCells(line)
			if len(cells) != 9 || !strings.HasPrefix(cells[0], "mb02.") {
				continue
			}
			offset, errOffset := strconv.Atoi(cells[1])
			quantity, errQuantity := strconv.Atoi(cells[2])
			locator := pageSourceRowPattern.FindStringSubmatch(cells[6])
			class, ok := levelClasses[cells[7]]
			if errOffset != nil || errQuantity != nil || locator == nil || !ok {
				return nil, nil, nil, fmt.Errorf("line %d: unrecognized MB02 row %q", number+1, line)
			}
			page, _ := strconv.Atoi(locator[1])
			row := registerRow{
				Key: cells[0], Endpoint: endpointInverter, Offset: intPointer(offset), Quantity: quantity,
				Type: cells[3], Access: "RO", Class: class, Page: page,
			}
			for _, name := range strings.Split(cells[8], ",") {
				if name = strings.TrimSpace(name); name != "" {
					row.Gates = append(row.Gates, name)
				}
			}
			applyScale(&row, cells[4], cells[5])
			rows = append(rows, parsedRow{Row: row, Private: map[string]string{"source_row": locator[2]}, Order: len(rows)})
		}
	}
	return rows, gates, classes, nil
}

// withoutSpaces drops the layout spaces that the editorial revision of the
// Markdown tables inserted into keys and cells.
func withoutSpaces(text string) string { return strings.ReplaceAll(text, " ", "") }

func pageList(text string) []int {
	var pages []int
	for _, part := range strings.Split(text, ",") {
		if page, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			pages = append(pages, page)
		}
	}
	return pages
}

// Readable configuration fields taken from the control catalogues: the local
// epoch, the output mode, the time-zone offset and the storage product
// selector. Their access stays the literal source access.
var readableConfigurationOffsets = map[int]bool{40000: true, 42001: true, 43006: true, 47000: true}

var controlStatusClasses = map[string]string{
	"DOCUMENTED_METADATA":                           classConditional,
	"DOCUMENTED_METADATA_WITH_EXPLICIT_VALUE_FACTS": classConditional,
	"DOCUMENTED_METADATA_VALUE_CONTRACT_UNKNOWN":    classRawOnly,
	"CONFLICT":                      classConflict,
	"QUARANTINED_SPECIFIC_CONFLICT": classConflict,
}

// parseReadableConfiguration reads the readable configuration fields of one
// control catalogue together with their source conditions.
func parseReadableConfiguration(text string) ([]parsedRow, error) {
	conditions := map[int]string{}
	var rows []parsedRow
	for number, line := range strings.Split(text, "\n") {
		cells := tableCells(line)
		if len(cells) >= 3 && len(cells) <= 4 {
			if offset, err := strconv.Atoi(cells[0]); err == nil && readableConfigurationOffsets[offset] {
				conditions[offset] = cells[2]
			}
			continue
		}
		if len(cells) < 10 || !strings.Contains(cells[0], ".control.") {
			continue
		}
		offset, err := strconv.Atoi(cells[1])
		if err != nil || !readableConfigurationOffsets[offset] {
			continue
		}
		quantity, errQuantity := strconv.Atoi(cells[2])
		locator := pageSourceRowPattern.FindStringSubmatch(cells[7])
		class, ok := controlStatusClasses[cells[9]]
		if errQuantity != nil || locator == nil || !ok || cells[6] != "RW" {
			return nil, fmt.Errorf("line %d: unrecognized readable configuration row %q", number+1, line)
		}
		page, _ := strconv.Atoi(locator[1])
		row := registerRow{
			Key: cells[0], Endpoint: endpointInverter, Offset: intPointer(offset), Quantity: quantity,
			Type: cells[3], Access: cells[6], Class: class, FunctionalClass: cells[8], Page: page,
		}
		applyScale(&row, cells[4], cells[5])
		rows = append(rows, parsedRow{Row: row, Private: map[string]string{"source_row": locator[2], "status": cells[9]}, Order: len(rows)})
	}
	for index := range rows {
		offset := *rows[index].Row.Offset
		condition, ok := conditions[offset]
		if !ok {
			continue
		}
		rows[index].Row.Condition = prose(condition)
		rows[index].Private["condition_literal"] = condition
	}
	return rows, nil
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
		identity := row.Endpoint + "\x00" + strconv.Itoa(offsetKey(row.Offset)) + "\x00" + row.Key
		seen[identity]++
		row.Occurrence = 0
		if seen[identity] > 1 {
			row.Occurrence = seen[identity]
		}
	}
	for index := range rows {
		row := &rows[index].Row
		identity := row.Endpoint + "\x00" + strconv.Itoa(offsetKey(row.Offset)) + "\x00" + row.Key
		if seen[identity] > 1 && row.Occurrence == 0 {
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
