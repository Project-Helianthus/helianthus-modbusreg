package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// censusEntry is one source edition of the reference's source appendix. The
// digest and the locator lines stay in the private sidecar.
type censusEntry struct {
	Section  string
	SHA256   string
	Title    string
	Issue    string
	Date     string
	Pages    int
	Alias    string
	Mirrored bool
	Anchors  []string
	Lines    []string
}

var (
	sectionHeadingPattern = regexp.MustCompile(`^### (S-[0-9a-f]{12})\s*$`)
	anchorLinePattern     = regexp.MustCompile(`^<a id="([^"]+)"></a>\s*$`)
	aliasRowPattern       = regexp.MustCompile(`^\| <a id="([^"]+)"></a>([A-Z0-9]+) \|.*\]\(#(s-[0-9a-f]{12})\) \|\s*$`)
	shaPattern            = regexp.MustCompile("PDF SHA-256: `([0-9a-f]{64})`")
	aliasLinePattern      = regexp.MustCompile(`^- Alias: ([A-Z0-9]+)\.`)
	coverPattern          = regexp.MustCompile(`^- Cover issue/date: (\S+) / (\S+); physical pages: (\d+)\.`)
	coverInlinePattern    = regexp.MustCompile(`cover issue (\d+), date (\d{4}-\d{2}-\d{2}); (\d+) physical pages`)
	digestFilePattern     = regexp.MustCompile(`^[0-9a-f]{64}\.pdf$`)
	titleFamilyPattern    = regexp.MustCompile(`^(SUN\d+[A-Z]*)\s`)
	titleBranchPattern    = regexp.MustCompile(`^SUN\d+[A-Z]* (V\d{3}R\d{3}(?:C\d{2})?(?:SPC\d{3})?)\b`)
	isoDatePattern        = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// parseSources reads the source appendix into census entries keyed by every
// anchor that resolves to them (section anchor, explicit anchor, alias anchor).
func parseSources(text string) (map[string]*censusEntry, []*censusEntry, error) {
	lines := strings.Split(text, "\n")
	byAnchor := map[string]*censusEntry{}
	var entries []*censusEntry
	aliasTargets := map[string]string{}
	aliasNames := map[string]string{}
	var pendingAnchors []string
	var current *censusEntry
	for _, line := range lines {
		if match := aliasRowPattern.FindStringSubmatch(line); match != nil {
			aliasTargets[match[1]] = match[3]
			aliasNames[match[3]] = match[2]
			continue
		}
		if match := anchorLinePattern.FindStringSubmatch(line); match != nil {
			pendingAnchors = append(pendingAnchors, match[1])
			continue
		}
		if match := sectionHeadingPattern.FindStringSubmatch(line); match != nil {
			current = &censusEntry{Section: match[1]}
			current.Anchors = append([]string{strings.ToLower(match[1])}, pendingAnchors...)
			pendingAnchors = nil
			entries = append(entries, current)
			continue
		}
		if strings.HasPrefix(line, "## ") {
			current = nil
			pendingAnchors = nil
			continue
		}
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "<a ") {
			pendingAnchors = nil
		}
		if current == nil {
			continue
		}
		current.Lines = append(current.Lines, line)
		if match := shaPattern.FindStringSubmatch(line); match != nil {
			current.SHA256 = match[1]
		}
		if match := aliasLinePattern.FindStringSubmatch(line); match != nil {
			current.Alias = match[1]
		}
		if strings.HasPrefix(line, "- Acquisition:") && strings.Contains(strings.ToLower(line), "mirror") {
			current.Mirrored = true
		}
		if strings.HasPrefix(line, "- Publication: ") {
			title, err := publicationTitle(strings.TrimPrefix(line, "- Publication: "))
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", current.Section, err)
			}
			current.Title = title
			if match := coverInlinePattern.FindStringSubmatch(line); match != nil {
				current.Issue, current.Date = match[1], match[2]
				current.Pages, _ = strconv.Atoi(match[3])
			}
		}
		if match := coverPattern.FindStringSubmatch(line); match != nil {
			current.Issue, current.Date = match[1], match[2]
			current.Pages, _ = strconv.Atoi(match[3])
		}
	}
	for _, entry := range entries {
		if entry.SHA256 == "" || entry.Title == "" || entry.Pages == 0 {
			return nil, nil, fmt.Errorf("%s: incomplete census entry", entry.Section)
		}
		if !strings.HasPrefix(entry.SHA256, strings.TrimPrefix(strings.ToLower(entry.Section), "s-")) {
			return nil, nil, fmt.Errorf("%s: section and digest disagree", entry.Section)
		}
		if name, ok := aliasNames[strings.ToLower(entry.Section)]; ok {
			if entry.Alias != "" && entry.Alias != name {
				return nil, nil, fmt.Errorf("%s: two aliases", entry.Section)
			}
			entry.Alias = name
		}
		for _, anchor := range entry.Anchors {
			if _, exists := byAnchor[anchor]; exists {
				return nil, nil, fmt.Errorf("anchor %s resolves twice", anchor)
			}
			byAnchor[anchor] = entry
		}
	}
	for anchor, target := range aliasTargets {
		entry, ok := byAnchor[target]
		if !ok {
			return nil, nil, fmt.Errorf("alias anchor %s points at unknown section %s", anchor, target)
		}
		if existing, exists := byAnchor[anchor]; exists && existing != entry {
			return nil, nil, fmt.Errorf("alias anchor %s resolves twice", anchor)
		}
		byAnchor[anchor] = entry
	}
	return byAnchor, entries, nil
}

func publicationTitle(text string) (string, error) {
	for _, candidate := range strings.Split(text, "; ") {
		candidate = strings.TrimSpace(candidate)
		candidate = strings.TrimSuffix(candidate, ".")
		if candidate == "" || digestFilePattern.MatchString(candidate) ||
			strings.HasPrefix(candidate, "cover issue") || strings.Contains(candidate, "physical pages") {
			continue
		}
		return strings.TrimSpace(strings.TrimSuffix(candidate, ".pdf")), nil
	}
	return "", fmt.Errorf("no publication title in %q", text)
}

var (
	slugWordPattern = regexp.MustCompile(`[A-Za-z0-9]+`)
	slugStopWords   = map[string]bool{"modbus": true, "interface": true, "definitions": true}
)

func editionBase(entry *censusEntry) string {
	var words []string
	for _, word := range slugWordPattern.FindAllString(entry.Title, -1) {
		lower := strings.ToLower(word)
		if slugStopWords[lower] {
			continue
		}
		words = append(words, lower)
	}
	if len(words) == 0 {
		words = []string{"untitled"}
	}
	issue := "na"
	if _, err := strconv.Atoi(entry.Issue); err == nil {
		issue = entry.Issue
	}
	date := "na"
	if isoDatePattern.MatchString(entry.Date) {
		date = entry.Date
	}
	return strings.Join(words, "-") + "-" + issue + "-" + date
}

// buildEditions turns the referenced census entries into committed edition
// records. Entries that share a title, an issue and a date are distinct copies
// and are numbered in digest order; the digest itself stays in the sidecar.
func buildEditions(entries []*censusEntry, cards map[string]string) (map[string]*edition, error) {
	groups := map[string][]*censusEntry{}
	for _, entry := range entries {
		key := entry.Title + "\x00" + entry.Issue + "\x00" + entry.Date
		groups[key] = append(groups[key], entry)
	}
	bySHA := map[string]*edition{}
	ids := map[string]string{}
	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool { return group[i].SHA256 < group[j].SHA256 })
		for index, entry := range group {
			record := &edition{
				Title:       entry.Title,
				Issue:       issueText(entry.Issue),
				Date:        dateText(entry.Date),
				SourceClass: sourceClassVendor,
				Alias:       entry.Alias,
				Card:        cards[entry.SHA256],
			}
			if entry.Mirrored {
				record.SourceClass = sourceClassMirrored
			}
			if match := titleFamilyPattern.FindStringSubmatch(entry.Title); match != nil {
				record.Family = match[1]
			}
			if match := titleBranchPattern.FindStringSubmatch(entry.Title); match != nil {
				record.Branch = match[1]
			}
			record.ID = editionBase(entry)
			if len(group) > 1 {
				record.Copy = index + 1
				record.ID += "-copy-" + strconv.Itoa(index+1)
			}
			if other, exists := ids[record.ID]; exists {
				return nil, fmt.Errorf("edition id %s is shared by %s and %s", record.ID, other, entry.Section)
			}
			ids[record.ID] = entry.Section
			bySHA[entry.SHA256] = record
		}
	}
	return bySHA, nil
}

func issueText(issue string) string {
	if _, err := strconv.Atoi(issue); err == nil {
		return "Issue " + issue
	}
	return floorNotStated
}

func dateText(date string) string {
	if isoDatePattern.MatchString(date) {
		return date
	}
	return floorNotStated
}
