package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	valueEnumPattern     = regexp.MustCompile(`^((?:\d+=[^;=.]+;\s*)+\d+=[^;=.]+)\.`)
	selectorPattern      = regexp.MustCompile(`selector\s*((?:\d+=[^,=.]+,\s*)+\d+=[^,=.]+)\.`)
	modulesPattern       = regexp.MustCompile(`Module (?:footnotes|conditions):\s*((?:\d=[^;.]+;\s*)*\d=[^;.]+)\.`)
	alarmRowPattern      = regexp.MustCompile(`^\| (3201[0-9]|3200[0-9]) \| (\d+) \| (\d+) \| ([a-z_]+) \| ([A-Za-z]+) \| ([^|]+?) \| (\d+) \|\s*$`)
	meterStatusPattern   = regexp.MustCompile(`\[M10\]\(appendices/sources\.md#m10\) p\. (\d+) defines meter status (\d) as ([a-z]+) and (\d) as ([a-z]+)\.`)
	meterTypePattern     = regexp.MustCompile(`P(\d+) distinguishes (single-phase) type (\d) from (three-phase) type (\d)\.`)
	storageStatusPattern = regexp.MustCompile(`For this profile, 37000 values ([0-9/]+) correspond to ([a-z/]+) states\.`)
	storageRowPattern    = regexp.MustCompile(`^\| attached_storage\[1\]\.status \| 37000 \| 1 \| \S+ \| \S+ \| \S+ \| (\d+) \|\s*$`)
	modeStatusPattern    = regexp.MustCompile(`\[M10\]\(appendices/sources\.md#m10\) pp\. (\d+)–(\d+) spans one status enumeration across three physical pages\. For example, (0x[0-9A-F]{4}) denotes an operating state whose interpretation depends on (off-grid context); (0x[0-9A-F]{4}) and (0x[0-9A-F]{4}) likewise carry mode-dependent limitations\.`)
	modeStatusRowPattern = regexp.MustCompile(`^\| status\.operating_code \| 32089 \|`)
)

const (
	kindStatusEnum = "status_enum"
	kindValueEnum  = "value_enum"
	kindSelector   = "selector"
	kindAlarmBits  = "alarm_bits"

	modeNoneStated  = "none stated"
	modeAlarmModule = "module condition of each bit"
	unlistedRaw     = "raw"
	unlistedUnknown = "unknown"
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

func modulesFrom(text string) ([]dictValue, error) {
	match := modulesPattern.FindStringSubmatch(text)
	if match == nil {
		return nil, fmt.Errorf("no module condition list")
	}
	return splitPairs(match[1], ";"), nil
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

// reviewDictionaries derives the dictionaries of a machine-readable review:
// the operating-status enumeration, the primary alarm bits, and every row
// condition that is an explicit value list.
func reviewDictionaries(card *cardInfo, editionID string, review *fullReview, markdown string, rows []parsedRow) ([]dictionary, error) {
	slug := card.Slug
	var dictionaries []dictionary
	if enum := review.OperatingStatusEnum; enum != nil {
		values := make([]dictValue, 0, len(enum.Values))
		for code, meaning := range enum.Values {
			values = append(values, dictValue{Code: code, Meaning: meaning})
		}
		sort.Slice(values, func(i, j int) bool { return hexValue(values[i].Code) < hexValue(values[j].Code) })
		dictionaries = append(dictionaries, dictionary{
			ID: slug + ".status." + strconv.Itoa(enum.Address), Kind: kindStatusEnum, Card: card.ID, Edition: editionID,
			Endpoint: endpointInverter, Offsets: []int{enum.Address}, Pages: distinctSorted(enum.SourcePages),
			ModePredicate: modeNoneStated, Values: values, Unlisted: cleanText(enum.Unlisted),
		})
	}
	if len(review.AlarmBits) > 0 {
		modules, err := modulesFrom(markdown)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", card.ID, err)
		}
		var bits []alarmBit
		var offsets, pages []int
		for _, bit := range review.AlarmBits {
			module := modeNoneStated
			if bit.FootnoteID != nil {
				module = *bit.FootnoteID
			}
			bits = append(bits, alarmBit{
				Offset: bit.RegisterAddress, Bit: bit.Bit, AlarmID: bit.AlarmID, Label: bit.FunctionalLabel,
				Severity: bit.Severity, Module: module, Page: bit.PhysicalPage,
			})
			offsets = append(offsets, bit.RegisterAddress)
			pages = append(pages, bit.PhysicalPage)
		}
		dictionaries = append(dictionaries, dictionary{
			ID: slug + ".alarm-bits", Kind: kindAlarmBits, Card: card.ID, Edition: editionID, Endpoint: endpointInverter,
			Offsets: distinctSorted(offsets), Pages: distinctSorted(pages), ModePredicate: modeAlarmModule,
			Modules: modules, Bits: bits, Unlisted: unlistedUnknown,
		})
	}
	for _, row := range rows {
		condition := cleanText(row.Private["condition_literal"])
		match := valueEnumPattern.FindStringSubmatch(condition)
		if match == nil {
			continue
		}
		offset := *row.Row.Offset
		dictionaries = append(dictionaries, dictionary{
			ID: slug + ".value." + strconv.Itoa(offset), Kind: kindValueEnum, Card: card.ID, Edition: editionID,
			Endpoint: row.Row.Endpoint, Offsets: []int{offset}, Pages: []int{row.Row.Page},
			ModePredicate: modeNoneStated, Values: splitPairs(match[1], ";"), Unlisted: unlistedRaw,
		})
	}
	return dictionaries, nil
}

// selectorDictionaries derives the product-selector dictionary of a readable
// configuration row whose source condition lists explicit values.
func selectorDictionaries(card *cardInfo, editionID string, rows []parsedRow) []dictionary {
	var dictionaries []dictionary
	for _, row := range rows {
		match := selectorPattern.FindStringSubmatch(cleanText(row.Private["condition_literal"]))
		if match == nil {
			continue
		}
		offset := *row.Row.Offset
		dictionaries = append(dictionaries, dictionary{
			ID: card.Slug + ".selector." + strconv.Itoa(offset), Kind: kindSelector, Card: card.ID, Edition: editionID,
			Endpoint: row.Row.Endpoint, Offsets: []int{offset}, Pages: []int{row.Row.Page},
			ModePredicate: modeNoneStated, Values: splitPairs(match[1], ","), Unlisted: unlistedRaw,
		})
	}
	return dictionaries
}

// mb02AlarmDictionary reads the 41 alarm bits of the MB02 supplement.
func mb02AlarmDictionary(card *cardInfo, editionID, text string) (dictionary, error) {
	modules, err := modulesFrom(text)
	if err != nil {
		return dictionary{}, fmt.Errorf("%s: %w", card.ID, err)
	}
	var bits []alarmBit
	var offsets, pages []int
	for _, line := range strings.Split(text, "\n") {
		match := alarmRowPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		offset, _ := strconv.Atoi(match[1])
		bit, _ := strconv.Atoi(match[2])
		alarm, _ := strconv.Atoi(match[3])
		page, _ := strconv.Atoi(match[7])
		bits = append(bits, alarmBit{Offset: offset, Bit: bit, AlarmID: alarm, Label: match[4], Severity: match[5], Module: strings.TrimSpace(match[6]), Page: page})
		offsets = append(offsets, offset)
		pages = append(pages, page)
	}
	if len(bits) == 0 {
		return dictionary{}, fmt.Errorf("%s: no alarm bits", card.ID)
	}
	return dictionary{
		ID: card.Slug + ".alarm-bits", Kind: kindAlarmBits, Card: card.ID, Edition: editionID, Endpoint: endpointInverter,
		Offsets: distinctSorted(offsets), Pages: distinctSorted(pages), ModePredicate: modeAlarmModule,
		Modules: modules, Bits: bits, Unlisted: unlistedUnknown,
	}, nil
}

// chapterDictionaries derives the dictionaries that the reference states only
// in its chapters for the M10 edition (the MA-C00 card): meter status, meter
// phase type, attached storage status, and the mode dependency of the
// operating status. Each statement is matched verbatim; a missing statement
// stops the generator.
func chapterDictionaries(card *cardInfo, editionID, attached, status string) ([]dictionary, error) {
	var dictionaries []dictionary
	meter := meterStatusPattern.FindStringSubmatch(attached)
	if meter == nil {
		return nil, fmt.Errorf("chapter 8 meter status statement not found")
	}
	meterPage, _ := strconv.Atoi(meter[1])
	dictionaries = append(dictionaries, dictionary{
		ID: card.Slug + ".value.37100", Kind: kindValueEnum, Card: card.ID, Edition: editionID, Endpoint: endpointInverter,
		Offsets: []int{37100}, Pages: []int{meterPage}, ModePredicate: modeNoneStated,
		Values: []dictValue{{Code: meter[2], Meaning: meter[3]}, {Code: meter[4], Meaning: meter[5]}}, Unlisted: unlistedRaw,
	})
	phase := meterTypePattern.FindStringSubmatch(attached)
	if phase == nil {
		return nil, fmt.Errorf("chapter 8 meter type statement not found")
	}
	phasePage, _ := strconv.Atoi(phase[1])
	dictionaries = append(dictionaries, dictionary{
		ID: card.Slug + ".value.37125", Kind: kindValueEnum, Card: card.ID, Edition: editionID, Endpoint: endpointInverter,
		Offsets: []int{37125}, Pages: []int{phasePage}, ModePredicate: modeNoneStated,
		Values: []dictValue{{Code: phase[3], Meaning: phase[2]}, {Code: phase[5], Meaning: phase[4]}}, Unlisted: unlistedRaw,
	})
	storage := storageStatusPattern.FindStringSubmatch(attached)
	if storage == nil {
		return nil, fmt.Errorf("chapter 8 storage status statement not found")
	}
	storagePage := 0
	for _, line := range strings.Split(attached, "\n") {
		if match := storageRowPattern.FindStringSubmatch(line); match != nil {
			storagePage, _ = strconv.Atoi(match[1])
		}
	}
	codes, meanings := strings.Split(storage[1], "/"), strings.Split(storage[2], "/")
	if len(codes) != len(meanings) || storagePage == 0 {
		return nil, fmt.Errorf("chapter 8 storage status statement is not a value list")
	}
	var values []dictValue
	for index := range codes {
		values = append(values, dictValue{Code: codes[index], Meaning: meanings[index]})
	}
	dictionaries = append(dictionaries, dictionary{
		ID: card.Slug + ".value.37000", Kind: kindValueEnum, Card: card.ID, Edition: editionID, Endpoint: endpointInverter,
		Offsets: []int{37000}, Pages: []int{storagePage}, ModePredicate: modeNoneStated, Values: values, Unlisted: unlistedRaw,
	})
	mode := modeStatusPattern.FindStringSubmatch(status)
	if mode == nil || !modeStatusRowPattern.MatchString(findLine(status, "| status.operating_code |")) {
		return nil, fmt.Errorf("chapter 7 operating status statement not found")
	}
	first, _ := strconv.Atoi(mode[1])
	last, _ := strconv.Atoi(mode[2])
	dictionaries = append(dictionaries, dictionary{
		ID: card.Slug + ".status.32089", Kind: kindStatusEnum, Card: card.ID, Edition: editionID, Endpoint: endpointInverter,
		Offsets: []int{32089}, Pages: pageRange(strconv.Itoa(first), strconv.Itoa(last)),
		ModePredicate: mode[4] + "; " + mode[3] + ", " + mode[5] + " and " + mode[6] + " carry mode-dependent meanings",
		Unlisted:      unlistedRaw,
	})
	return dictionaries, nil
}

func findLine(text, prefix string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

func hexValue(code string) int64 {
	value, err := strconv.ParseInt(strings.TrimPrefix(strings.ToLower(code), "0x"), 16, 64)
	if err != nil {
		return -1
	}
	return value
}
