package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	originReference = "reference"
	originGenerator = "generator"

	affectedRegisters = "registers"
	affectedModels    = "models"
)

var (
	contradictionRowPattern = regexp.MustCompile(`^\| (Q\d{2}) \| ([^|]+) \| ([^|]+) \| ([^|]+) \|\s*$`)
	aliasLinkPattern        = regexp.MustCompile(`\[[A-Z0-9-]+\]\(sources\.md#([a-z0-9-]+)\)`)
	fiveDigitPattern        = regexp.MustCompile(`(?:^|\D)(\d{5})(?:\D|$)`)
)

// quarantineRowScope narrows the rows a contradiction flags to the editions
// whose rows the reference itself names as inconsistent: Q07 quarantines the
// M10 row and preserves the MA-V200 tuple in its own scope.
var quarantineRowScope = map[string][]string{"Q07": {"M10"}}

// anomalySpec binds one source-specific anomaly of the reference to the rows
// it affects. The meaning is the reference text from Start through End, taken
// verbatim after link and digest removal; a missing anchor stops the generator.
type anomalySpec struct {
	ID          string
	File        string
	Start, End  string
	Subject     string
	Cards       []string
	Offsets     []int
	FloorSuffix string
}

var anomalySpecs = []anomalySpec{
	{ID: "A01", File: "appendices/contradictions.md", Start: "HA Issue 08 p. 11 gives the extra-string formula", End: "no silent index correction.",
		Subject: "HA Issue 08 extra-string formula against the explicit PV25 pair", Cards: []string{"HA"}, Offsets: []int{32344, 32345}},
	{ID: "A02", File: "appendices/contradictions.md", Start: "P. 12 duplicates PV15 current 32045", End: "PV14-current row.",
		Subject: "HA Issue 08 PV15 current listed twice, PV14 current absent", Cards: []string{"HA"}, Offsets: []int{32045}},
	{ID: "A03", File: "appendices/contradictions.md", Start: "HD Issue 01 p. 11 gives a second-bank formula", End: "inferred formula origin.",
		Subject: "HD Issue 01 second-bank formula origin", Cards: []string{"HD"}},
	{ID: "A04", File: "appendices/contradictions.md", Start: "MB02 p. 29 names 35010 as an alarm", End: "the semantic label is quarantined.",
		Subject: "MB02 35010 alarm name against a voltage unit", Cards: []string{"MB02"}, Offsets: []int{35010}},
	{ID: "A05", File: "appendices/contradictions.md", Start: "Its pp. 70–71 optimizer table", End: "format selection is unresolved.",
		Subject: "MB02 optimizer feature-table caption against its version cell", Cards: []string{"MB02"}},
	{ID: "A06", File: "appendices/contradictions.md", Start: "Its literal PSC110", End: "rewritten to SPC110.",
		Subject: "MB02 literal PSC110 floor suffix", Cards: []string{"MB02"}, FloorSuffix: unnormalizedFloorSuffix},
	{ID: "A07", File: "03-encoding.md", Start: "MA-V200 phase-power-labelled 32133/32135/32137", End: "silently changing the unit to kW.",
		Subject: "MA-V200 phase-power rows with an energy unit", Cards: []string{"MA-V200"}, Offsets: []int{32133, 32135, 32137}},
	{ID: "A08", File: "08-attached-interfaces.md", Start: "Both sources describe optimizer files with a V102 caption/V103 version-cell conflict", End: "108-byte feature records.",
		Subject: "MA-V200 and MC optimizer feature-table caption against its version cell", Cards: []string{"MA-V200", "MC"}},
}

var generatorQuarantines = []quarantine{
	{ID: quarantineSequenceOrdinal, Origin: originGenerator, Subject: "Model row numbered by a sequence ordinal, not a model identifier",
		Meaning: "The edition numbers this row with a sequence ordinal that it does not label as a model identifier. The ordinal is kept as the sequence number and is never used as a model identifier."},
	{ID: quarantineFloorCell, Origin: originGenerator, Subject: "Minimum-software cell outside the branch form",
		Meaning: "The row minimum is a merged cell or a literal outside the V-R-C-SPC form. The literal is kept as printed and serves as no floor until the cell is clarified from the edition."},
}

// referenceQuarantines parses the quarantined contradictions Q01 to Q07.
func referenceQuarantines(text string, anchors map[string]*censusEntry) ([]quarantine, map[string][]int, map[string][]*censusEntry, error) {
	var out []quarantine
	offsets := map[string][]int{}
	editions := map[string][]*censusEntry{}
	for _, line := range strings.Split(text, "\n") {
		match := contradictionRowPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		id := match[1]
		for _, link := range aliasLinkPattern.FindAllStringSubmatch(match[3], -1) {
			entry, ok := anchors[link[1]]
			if !ok {
				return nil, nil, nil, fmt.Errorf("%s cites unknown source %s", id, link[1])
			}
			editions[id] = append(editions[id], entry)
		}
		for _, number := range fiveDigitPattern.FindAllStringSubmatch(match[2], -1) {
			value, _ := strconv.Atoi(number[1])
			offsets[id] = append(offsets[id], value)
		}
		out = append(out, quarantine{
			ID: id, Origin: originReference, Subject: cleanText(match[2]), Evidence: cleanText(match[3]), Meaning: prose(match[4]),
		})
	}
	if len(out) != 7 {
		return nil, nil, nil, fmt.Errorf("%d quarantined contradictions, want Q01 to Q07", len(out))
	}
	return out, offsets, editions, nil
}

// anomalyMeaning returns the verbatim reference text of one anomaly.
func anomalyMeaning(spec anomalySpec, text string) (string, error) {
	cleaned := cleanText(text)
	start := strings.Index(cleaned, spec.Start)
	if start < 0 {
		return "", fmt.Errorf("%s: start anchor not found", spec.ID)
	}
	end := strings.Index(cleaned[start:], spec.End)
	if end < 0 {
		return "", fmt.Errorf("%s: end anchor not found", spec.ID)
	}
	return cleaned[start : start+end+len(spec.End)], nil
}

type quarantineBuilder struct {
	items    map[string]*quarantine
	affected map[string]map[string]affectedRow
	editions map[string]map[string]bool
}

func newQuarantineBuilder() *quarantineBuilder {
	return &quarantineBuilder{items: map[string]*quarantine{}, affected: map[string]map[string]affectedRow{}, editions: map[string]map[string]bool{}}
}

func (b *quarantineBuilder) add(item quarantine) {
	copied := item
	b.items[item.ID] = &copied
	b.affected[item.ID] = map[string]affectedRow{}
	b.editions[item.ID] = map[string]bool{}
}

func (b *quarantineBuilder) edition(id, editionID string) { b.editions[id][editionID] = true }

func (b *quarantineBuilder) flagRegister(id, card, editionID string, row *registerRow) {
	row.Quarantine = appendUnique(row.Quarantine, id)
	reference := affectedRow{Set: affectedRegisters, Card: card, Offset: row.Offset, Key: row.Key, Occurrence: row.Occurrence}
	b.affected[id][affectedIdentity(reference)] = reference
	b.edition(id, editionID)
}

func (b *quarantineBuilder) flagModel(id string, row *modelRow) {
	row.Quarantine = appendUnique(row.Quarantine, id)
	b.recordModel(id, row)
}

func (b *quarantineBuilder) recordModel(id string, row *modelRow) {
	reference := affectedRow{Set: affectedModels, Edition: row.Edition, Token: row.Token, Identifier: row.Identifier, Page: row.Page}
	b.affected[id][affectedIdentity(reference)] = reference
	b.edition(id, row.Edition)
}

func (b *quarantineBuilder) build() []quarantine {
	ids := make([]string, 0, len(b.items))
	for id := range b.items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]quarantine, 0, len(ids))
	for _, id := range ids {
		item := *b.items[id]
		item.Editions = sortedKeys(b.editions[id])
		item.Affected = make([]affectedRow, 0, len(b.affected[id]))
		keys := make([]string, 0, len(b.affected[id]))
		for key := range b.affected[id] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			item.Affected = append(item.Affected, b.affected[id][key])
		}
		out = append(out, item)
	}
	return out
}

func affectedIdentity(row affectedRow) string {
	identifier := ""
	if row.Identifier != nil {
		identifier = fmt.Sprintf("%08d", *row.Identifier)
	}
	offset := ""
	if row.Offset != nil {
		offset = fmt.Sprintf("%08d", *row.Offset)
	}
	return strings.Join([]string{row.Set, row.Card, row.Edition, offset, row.Key, fmt.Sprintf("%03d", row.Occurrence), row.Token, identifier, fmt.Sprintf("%06d", row.Page)}, "\x00")
}

func appendUnique(list []string, value string) []string {
	for _, existing := range list {
		if existing == value {
			return list
		}
	}
	list = append(list, value)
	sort.Strings(list)
	return list
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
