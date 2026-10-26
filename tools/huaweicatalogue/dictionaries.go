package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	valueEnumPattern  = regexp.MustCompile(`^((?:\d+=[^;=.]+;\s*)+\d+=[^;=.]+)\.`)
	selectorPattern   = regexp.MustCompile(`selector\s*((?:\d+=[^,=.]+,\s*)+\d+=[^,=.]+)\.`)
	modeStatusPattern = regexp.MustCompile(`\[M10\]\(appendices/sources\.md#m10\) pp\. (\d+)–(\d+) spans one status enumeration across three physical pages\. For example, (0x[0-9A-F]{4}) denotes an operating state whose interpretation depends on (off-grid context); (0x[0-9A-F]{4}) and (0x[0-9A-F]{4}) likewise carry mode-dependent limitations\.`)
)

const (
	kindStatusEnum = "status_enum"
	kindValueEnum  = "value_enum"
	kindBitmap     = "bitmap"
	kindSign       = "sign"
	kindSentinels  = "sentinels"
	kindSelector   = "selector"
	kindAlarmBits  = "alarm_bits"

	modeNoneStated  = "none stated"
	modeAlarmModule = "module condition of each bit"
	unlistedRaw     = "raw"
	unlistedUnknown = "unknown"
	selectorOffset  = 47000
)

func splitPairs(text, separator string) []dictValue {
	var values []dictValue
	for _, pair := range strings.Split(text, separator) {
		code, meaning, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok {
			continue
		}
		values = append(values, dictValue{Code: strings.TrimSpace(code), Meaning: strings.TrimSpace(meaning)})
	}
	return values
}

func distinctSorted(values []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Ints(out)
	return out
}

// meaningDictionaries derives the dictionaries a base twin attaches to one
// row: value enumerations, bitmaps, sign conventions and sentinels.
func meaningDictionaries(row parsedRow) ([]dictionary, error) {
	var out []dictionary
	if row.Row.Offset == nil {
		return nil, nil
	}
	base := dictionary{Endpoint: row.Row.Endpoint, Offsets: []int{*row.Row.Offset}, Pages: []int{row.Row.Page}, ModePredicate: modeNoneStated, Unlisted: unlistedRaw}
	if raw, ok := row.Meaning["enum"]; ok {
		values, err := enumValues(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", row.Row.Key, err)
		}
		values, err = withoutTruncatedSentinels(values, row.Meaning["sentinels"])
		if err != nil {
			return nil, fmt.Errorf("%s sentinels: %w", row.Row.Key, err)
		}
		if len(values) > 0 {
			item := base
			item.Kind, item.Values = kindValueEnum, values
			out = append(out, item)
		}
	}
	if raw, ok := row.Meaning["bits"]; ok {
		var bits []struct {
			Bit     int    `json:"bit"`
			Meaning string `json:"meaning_key"`
		}
		if err := json.Unmarshal(raw, &bits); err != nil {
			return nil, fmt.Errorf("%s bits: %w", row.Row.Key, err)
		}
		if len(bits) > 0 {
			item := base
			item.Kind = kindBitmap
			for _, bit := range bits {
				item.Values = append(item.Values, dictValue{Code: strconv.Itoa(bit.Bit), Meaning: bit.Meaning})
			}
			item.Unlisted = unlistedUnknown
			out = append(out, item)
		}
	}
	if raw, ok := row.Meaning["sign"]; ok {
		var sign struct {
			Positive string `json:"positive"`
			Negative string `json:"negative"`
		}
		if err := json.Unmarshal(raw, &sign); err != nil {
			return nil, fmt.Errorf("%s sign: %w", row.Row.Key, err)
		}
		if sign.Positive != "" && sign.Negative != "" {
			item := base
			item.Kind = kindSign
			item.Values = []dictValue{{Code: "positive", Meaning: sign.Positive}, {Code: "negative", Meaning: sign.Negative}}
			out = append(out, item)
		}
	}
	if raw, ok := row.Meaning["sentinels"]; ok {
		var sentinels []struct {
			Value   string `json:"value"`
			Meaning string `json:"meaning"`
		}
		if err := json.Unmarshal(raw, &sentinels); err != nil {
			return nil, fmt.Errorf("%s sentinels: %w", row.Row.Key, err)
		}
		if len(sentinels) > 0 {
			item := base
			item.Kind = kindSentinels
			for _, sentinel := range sentinels {
				item.Values = append(item.Values, dictValue{Code: sentinel.Value, Meaning: sentinel.Meaning})
			}
			out = append(out, item)
		}
	}
	return out, nil
}

// withoutTruncatedSentinels drops enumeration codes that are a proper
// hexadecimal prefix of a sentinel code of the same row: the extraction cut
// the literal, so publishing it would give the full sentinel a second meaning.
func withoutTruncatedSentinels(values []dictValue, raw json.RawMessage) ([]dictValue, error) {
	if len(raw) == 0 {
		return values, nil
	}
	var sentinels []struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &sentinels); err != nil {
		return nil, err
	}
	kept := values[:0:0]
	for _, value := range values {
		truncated := false
		code := strings.ToLower(value.Code)
		for _, sentinel := range sentinels {
			full := strings.ToLower(sentinel.Value)
			if strings.HasPrefix(code, "0x") && len(code) < len(full) && strings.HasPrefix(full, code) {
				truncated = true
			}
		}
		if !truncated {
			kept = append(kept, value)
		}
	}
	return kept, nil
}

// enumValues reads an enumeration given either as a code-to-meaning object or
// as a list of value and meaning entries.
func enumValues(raw json.RawMessage) ([]dictValue, error) {
	var list []struct {
		Value   string `json:"value_literal"`
		Meaning string `json:"meaning_key"`
	}
	if err := json.Unmarshal(raw, &list); err == nil {
		var values []dictValue
		for _, item := range list {
			values = append(values, dictValue{Code: item.Value, Meaning: item.Meaning})
		}
		return values, nil
	}
	var object map[string]string
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("enumeration is neither a list nor an object")
	}
	values := make([]dictValue, 0, len(object))
	for code, meaning := range object {
		values = append(values, dictValue{Code: code, Meaning: meaning})
	}
	sortCodes(values)
	return values, nil
}

// conditionDictionary derives a value list from a row condition that is an
// explicit list of codes.
func conditionDictionary(row parsedRow) *dictionary {
	match := valueEnumPattern.FindStringSubmatch(cleanText(row.Condition))
	if match == nil || row.Row.Offset == nil {
		return nil
	}
	return &dictionary{
		Kind: kindValueEnum, Endpoint: row.Row.Endpoint, Offsets: []int{*row.Row.Offset}, Pages: []int{row.Row.Page},
		ModePredicate: modeNoneStated, Values: splitPairs(match[1], ";"), Unlisted: unlistedRaw,
	}
}

// selectorDictionary derives the storage product selector from a control row:
// the explicit members of a base twin or the value list of a scope review.
func selectorDictionary(row parsedRow) *dictionary {
	if row.Row.Offset == nil || *row.Row.Offset != selectorOffset {
		return nil
	}
	values := row.Enum
	if len(values) == 0 {
		if match := selectorPattern.FindStringSubmatch(cleanText(row.Condition)); match != nil {
			values = splitPairs(match[1], ",")
		}
	}
	if len(values) == 0 {
		return nil
	}
	return &dictionary{
		Kind: kindSelector, Endpoint: row.Row.Endpoint, Offsets: []int{selectorOffset}, Pages: []int{row.Row.Page},
		ModePredicate: modeNoneStated, Values: values, Unlisted: unlistedRaw,
	}
}

// cardDictionaries collects every dictionary of one card and binds it to the
// card edition, with identifiers made unique in row order.
func cardDictionaries(card *cardInfo, editionID string, rows *cardRows) ([]dictionary, error) {
	var out []dictionary
	if enum := rows.StatusEnum; enum != nil {
		out = append(out, dictionary{
			Kind: kindStatusEnum, Endpoint: endpointInverter, Offsets: []int{enum.Offset}, Pages: enum.Pages,
			ModePredicate: modeNoneStated, Values: enum.Values, Unlisted: enum.Unlisted,
		})
	}
	if len(rows.AlarmBits) > 0 {
		var offsets, pages []int
		for _, bit := range rows.AlarmBits {
			if bit.Offset != nil {
				offsets = append(offsets, *bit.Offset)
			}
			pages = append(pages, bit.Page)
		}
		out = append(out, dictionary{
			Kind: kindAlarmBits, Endpoint: endpointInverter, Offsets: distinctSorted(offsets), Pages: distinctSorted(pages),
			ModePredicate: modeAlarmModule, Modules: rows.Modules, Bits: rows.AlarmBits, Unlisted: unlistedUnknown,
		})
	}
	for _, row := range rows.Rows {
		items, err := meaningDictionaries(row)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if item := conditionDictionary(row); item != nil {
			out = append(out, *item)
		}
	}
	for _, row := range rows.Controls {
		if item := selectorDictionary(row); item != nil {
			out = append(out, *item)
		}
	}
	seen := map[string]int{}
	for index := range out {
		out[index].Card = card.ID
		out[index].Edition = editionID
		id := card.Slug + ".alarm-bits"
		if out[index].Kind != kindAlarmBits {
			id = card.Slug + "." + strings.ReplaceAll(out[index].Kind, "_", "-") + "." + strconv.Itoa(out[index].Offsets[0])
		}
		seen[id]++
		if seen[id] > 1 {
			id += "-" + strconv.Itoa(seen[id])
		}
		out[index].ID = id
	}
	return out, nil
}

// applyModePredicate binds the off-grid mode dependency that the reference
// states for the M10 operating status to that edition's status dictionary.
func applyModePredicate(dictionaries []dictionary, card *cardInfo, chapter string) error {
	match := modeStatusPattern.FindStringSubmatch(chapter)
	if match == nil {
		return fmt.Errorf("chapter 7 operating status statement not found")
	}
	predicate := match[4] + "; " + match[3] + ", " + match[5] + " and " + match[6] + " carry mode-dependent meanings"
	applied := 0
	for index := range dictionaries {
		item := &dictionaries[index]
		if item.Card == card.ID && len(item.Offsets) == 1 && item.Offsets[0] == 32089 && (item.Kind == kindValueEnum || item.Kind == kindStatusEnum) {
			item.ModePredicate = predicate
			applied++
		}
	}
	if applied != 1 {
		return fmt.Errorf("%s: %d operating status dictionaries for the mode predicate", card.ID, applied)
	}
	return nil
}
