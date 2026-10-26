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
	CatalogueFile string
}

var (
	cardRowPattern    = regexp.MustCompile(`^\| \[([^\]]+)\]\(([a-z0-9-]+)\.md\) \| ([^|]+) \| (\d+) \| (.*) \|\s*$`)
	modelPagesPattern = regexp.MustCompile(`pp?\. (\d+)(?:[–-](\d+))?`)
	reviewRowPattern  = regexp.MustCompile(`^\| ([A-Z0-9-]+) \| (\d+) \| [^|]+ \| \[[^\]]+\]\(([^)]+)\) \|\s*$`)
	editionRowPattern = regexp.MustCompile(`^\| \[(PV\d{3})\]\(sources\.md#pv\d{3}\) \| [^|]+ \| (\d+) \|`)
)

// parseCards reads the profile directory and the read-only catalogue
// navigation: the card list, the core and read-only counts per card, and the
// selection note of each card.
func parseCards(directory, review string) ([]*cardInfo, error) {
	var cards []*cardInfo
	seen := map[string]bool{}
	for _, line := range strings.Split(directory, "\n") {
		match := cardRowPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		core, _ := strconv.Atoi(match[4])
		card := &cardInfo{ID: strings.ToUpper(match[1]), Slug: match[2], CoreCount: core, Note: strings.TrimSpace(match[5])}
		pages := modelPagesPattern.FindStringSubmatch(match[3])
		if pages == nil {
			return nil, fmt.Errorf("card %s states no model pages", card.ID)
		}
		card.ModelPages = pageRange(pages[1], pages[2])
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

// editionModelCounts reads the per-edition model-row accounting of the
// reference's model edition table.
func editionModelCounts(text string) map[string]int {
	counts := map[string]int{}
	for _, line := range strings.Split(text, "\n") {
		if match := editionRowPattern.FindStringSubmatch(line); match != nil {
			count, _ := strconv.Atoi(match[2])
			counts[match[1]] = count
		}
	}
	return counts
}
