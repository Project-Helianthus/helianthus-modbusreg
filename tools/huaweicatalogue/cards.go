package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// cardInfo is one profile card of the reference with the accounting the
// reference states for it.
type cardInfo struct {
	ID            string
	Slug          string
	CoreCount     int
	ROCount       int
	Note          string
	Source        *censusEntry
	ModelPages    []int
	EvidenceText  string
	CatalogueFile string
}

var (
	cardRowPattern       = regexp.MustCompile(`^\| \[([^\]]+)\]\(([a-z0-9-]+)\.md\) \| [^|]+ \| (\d+) \| (.*) \|\s*$`)
	reviewRowPattern     = regexp.MustCompile(`^\| ([A-Z0-9-]+) \| (\d+) \| [^|]+ \| \[[^\]]+\]\(([^)]+)\) \|\s*$`)
	cardSourcePattern    = regexp.MustCompile(`^Source: \[[^\]]+\]\(\.\./appendices/sources\.md#([a-z0-9-]+)\)`)
	modelContextPattern  = regexp.MustCompile(`Source-model context: physical pp?\. (\d+)(?:[–-](\d+))?\.`)
	evidenceLinePattern  = regexp.MustCompile(`^\*\*Exact-model evidence:\*\* (.*)$`)
	catalogueRefPattern  = regexp.MustCompile(`^Exact (?:source: )?\[[^\]]+\]\(\.\./appendices/sources\.md#([a-z0-9-]+)\)`)
	mb02ModelPagePattern = regexp.MustCompile(`^\| SUN\S+ \| \d+ \| \S+ \| (\d+) \|\s*$`)
)

// parseCards reads the profile directory, the catalogue navigation and every
// card, and resolves each card's source edition.
func parseCards(directory, review string, readCard func(slug string) (string, error), anchors map[string]*censusEntry) ([]*cardInfo, error) {
	var cards []*cardInfo
	seen := map[string]bool{}
	for _, line := range strings.Split(directory, "\n") {
		match := cardRowPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		core, _ := strconv.Atoi(match[3])
		card := &cardInfo{ID: strings.ToUpper(match[1]), Slug: match[2], CoreCount: core, Note: strings.TrimSpace(match[4])}
		if seen[card.ID] {
			return nil, fmt.Errorf("card %s listed twice", card.ID)
		}
		seen[card.ID] = true
		card.CatalogueFile = "registers-" + card.Slug + ".json"
		cards = append(cards, card)
	}
	roCounts := map[string]int{}
	for _, line := range strings.Split(review, "\n") {
		if match := reviewRowPattern.FindStringSubmatch(line); match != nil {
			count, _ := strconv.Atoi(match[2])
			roCounts[match[1]] = count
		}
	}
	for _, card := range cards {
		count, ok := roCounts[card.ID]
		if !ok {
			return nil, fmt.Errorf("card %s has no read-only accounting row", card.ID)
		}
		card.ROCount = count
		text, err := readCard(card.Slug)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(text, "\n") {
			if match := cardSourcePattern.FindStringSubmatch(line); match != nil && card.Source == nil {
				entry, ok := anchors[match[1]]
				if !ok {
					return nil, fmt.Errorf("card %s cites unknown source anchor %s", card.ID, match[1])
				}
				card.Source = entry
			}
			if match := modelContextPattern.FindStringSubmatch(line); match != nil && card.ModelPages == nil {
				card.ModelPages = pageRange(match[1], match[2])
			}
			if match := evidenceLinePattern.FindStringSubmatch(line); match != nil {
				card.EvidenceText = match[1]
			}
			if match := mb02ModelPagePattern.FindStringSubmatch(line); match != nil {
				page, _ := strconv.Atoi(match[1])
				if len(card.ModelPages) == 0 || card.ModelPages[len(card.ModelPages)-1] != page {
					card.ModelPages = append(card.ModelPages, page)
				}
			}
		}
		if card.Source == nil {
			return nil, fmt.Errorf("card %s has no source edition", card.ID)
		}
		if len(card.ModelPages) == 0 {
			return nil, fmt.Errorf("card %s has no model pages", card.ID)
		}
	}
	return cards, nil
}

func pageRange(first, last string) []int {
	start, _ := strconv.Atoi(first)
	end := start
	if last != "" {
		end, _ = strconv.Atoi(last)
	}
	var pages []int
	for page := start; page <= end; page++ {
		pages = append(pages, page)
	}
	return pages
}

// cardTuple is one exact model statement of a card or of the model coverage
// appendix: a token, an identifier and, where stated, the explicit floor.
type cardTuple struct {
	Token      string
	Identifier int
	Floor      string
	Page       int
}

var (
	tupleListPattern   = regexp.MustCompile(`(SUN[0-9A-Za-z_.\-]+) / (?:ID )?(\d{1,4})(?: / (same branch|SUN\d+[A-Z]* V\d{3}R\d{3}(?:C\d{2})?(?:SPC\d{3})?|V\d{3}R\d{3}(?:C\d{2})?(?:SPC\d{3})?))?`)
	commonFloorPattern = regexp.MustCompile(`Explicit earliest (?:branch|version)(?: for these rows)?: ((?:SUN\d+[A-Z]* )?V\d{3}R\d{3}(?:C\d{2})?(?:SPC\d{3})?)\.`)
	coverageRowPattern = regexp.MustCompile(`^\| (SUN\S+) \| (\d+) \| (\S+(?: \S+)?) \| \[([A-Z0-9]+)\]\(sources\.md#([a-z0-9]+)\), p\. (\d+) \|\s*$`)
)

// cardTuples parses the exact-model evidence sentence of a card. Abbreviated
// tokens that do not start with the product prefix are display shorthand and
// are skipped.
func cardTuples(text string) []cardTuple {
	common := ""
	if match := commonFloorPattern.FindStringSubmatch(text); match != nil {
		common = match[1]
	}
	var tuples []cardTuple
	previous := ""
	for _, match := range tupleListPattern.FindAllStringSubmatch(text, -1) {
		identifier, _ := strconv.Atoi(match[2])
		floor := match[3]
		switch floor {
		case "same branch":
			floor = previous
		case "":
			floor = common
		}
		previous = floor
		tuples = append(tuples, cardTuple{Token: strings.TrimSuffix(match[1], "."), Identifier: identifier, Floor: floor})
	}
	return tuples
}

// coverageTuples parses the reference's table of exact models and explicit
// firmware minima, keyed by the source anchor of each row.
func coverageTuples(text string) map[string][]cardTuple {
	tuples := map[string][]cardTuple{}
	for _, line := range strings.Split(text, "\n") {
		match := coverageRowPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		identifier, _ := strconv.Atoi(match[2])
		page, _ := strconv.Atoi(match[6])
		tuples[match[5]] = append(tuples[match[5]], cardTuple{Token: match[1], Identifier: identifier, Floor: match[3], Page: page})
	}
	return tuples
}
