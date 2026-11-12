package main

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var (
	markdownLinkPattern  = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	digestParenPattern   = regexp.MustCompile("\\s*\\((?:SHA|source ID) `[0-9A-Za-z]+`\\)")
	inlineDigestPattern  = regexp.MustCompile("SHA `[0-9a-fA-F]+`,?\\s*")
	strongPattern        = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	whitespacePattern    = regexp.MustCompile(`\s+`)
	dropSentencePattern  = regexp.MustCompile(`(?i)engineer|authori[szt]|approv|recommend`)
	controlCharacterTest = func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }

	// ageWords are matched as whole words; the release words they may not
	// stand next to are listed at the end of this file.
	ageWords = []string{"older", "newer", "previous", "later", "earlier", "legacy", "new"}

	// locationMarkers are substrings that name a scheme, a host or a file.
	locationMarkers = []string{"http:", "https:", "://", "www.", ".md", ".json", ".pdf", ".zip", ".rar", "../"}
	forbiddenWords  = []string{"led" + "ger"}
)

// cleanText removes Markdown link targets, emphasis, digest parentheticals and
// layout whitespace from a reference cell. It never rewrites words.
func cleanText(text string) string {
	text = digestParenPattern.ReplaceAllString(text, "")
	text = inlineDigestPattern.ReplaceAllString(text, "")
	text = markdownLinkPattern.ReplaceAllString(text, "$1")
	text = strongPattern.ReplaceAllString(text, "$1")
	text = strings.ReplaceAll(text, "`", "")
	text = whitespacePattern.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

// prose keeps the reference wording of an explanatory text and drops every
// sentence that carries operational-review or authorization wording, or that
// places an age word next to a release word. Within such a sentence the
// semicolon clauses free of that wording are kept. A text without such a
// sentence is returned unchanged apart from cleanText.
func prose(text string) string {
	text = cleanText(text)
	if text == "" {
		return ""
	}
	sentences := splitSentences(text)
	kept := make([]string, 0, len(sentences))
	for _, sentence := range sentences {
		if !droppable(sentence) {
			kept = append(kept, sentence)
			continue
		}
		ageIssue := ageNearRelease(sentence)
		var clauses []string
		for _, clause := range strings.Split(sentence, "; ") {
			if droppable(clause) || (ageIssue && len(wordSpans(strings.ToLower(clause), ageWords, true)) > 0) {
				continue
			}
			clauses = append(clauses, strings.TrimSuffix(clause, "."))
		}
		if len(clauses) > 0 {
			kept = append(kept, strings.Join(clauses, "; ")+".")
		}
	}
	return strings.TrimSpace(strings.Join(kept, " "))
}

func droppable(text string) bool {
	return dropSentencePattern.MatchString(text) || ageNearRelease(text)
}

// splitSentences splits at a full stop, question or exclamation mark followed
// by a space and an upper-case letter, except after the page abbreviations
// "p." and "pp.".
func splitSentences(text string) []string {
	var sentences []string
	runes := []rune(text)
	start := 0
	for index := 0; index < len(runes); index++ {
		r := runes[index]
		if r != '.' && r != '?' && r != '!' {
			continue
		}
		next := index + 1
		if next >= len(runes) || runes[next] != ' ' {
			continue
		}
		after := next + 1
		if after >= len(runes) || !unicode.IsUpper(runes[after]) {
			continue
		}
		word := lastWord(runes[start:index])
		if strings.EqualFold(word, "p") || strings.EqualFold(word, "pp") {
			continue
		}
		sentences = append(sentences, strings.TrimSpace(string(runes[start:next])))
		start = after
	}
	if tail := strings.TrimSpace(string(runes[start:])); tail != "" {
		sentences = append(sentences, tail)
	}
	return sentences
}

func lastWord(runes []rune) string {
	end := len(runes)
	begin := end
	for begin > 0 && (unicode.IsLetter(runes[begin-1]) || unicode.IsDigit(runes[begin-1])) {
		begin--
	}
	return string(runes[begin:end])
}

// ageNearRelease reports an age word within 40 characters of a release word.
func ageNearRelease(text string) bool {
	lower := strings.ToLower(text)
	ages := wordSpans(lower, ageWords, true)
	if len(ages) == 0 {
		return false
	}
	releases := wordSpans(lower, releaseWords, false)
	for _, age := range ages {
		for _, release := range releases {
			gap := release[0] - age[1]
			if release[0] < age[0] {
				gap = age[0] - release[1]
			}
			if gap <= 40 {
				return true
			}
		}
	}
	return false
}

// wordSpans returns the byte spans of every occurrence of the words in a
// lower-case text, optionally only where the occurrence is a whole word.
func wordSpans(lower string, words []string, wholeWord bool) [][2]int {
	var spans [][2]int
	for _, word := range words {
		for offset := 0; offset < len(lower); {
			index := strings.Index(lower[offset:], word)
			if index < 0 {
				break
			}
			start := offset + index
			end := start + len(word)
			offset = start + 1
			if wholeWord && (start > 0 && isWordByte(lower[start-1]) || end < len(lower) && isWordByte(lower[end])) {
				continue
			}
			spans = append(spans, [2]int{start, end})
		}
	}
	return spans
}

func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_' || b >= 0x80
}

// hasHexRun reports a run of twelve or more hexadecimal digits.
func hasHexRun(text string) bool {
	run := 0
	for index := 0; index < len(text); index++ {
		c := text[index]
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' {
			run++
			if run >= 12 {
				return true
			}
			continue
		}
		run = 0
	}
	return false
}

func namesLocation(text string) bool {
	lower := strings.ToLower(text)
	for _, marker := range locationMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return len(wordSpans(lower, forbiddenWords, true)) > 0
}

// proseFields are the JSON fields that carry explanatory text. Identifiers
// such as functional keys keep their reference spelling and are exempt from
// the wording check, never from the digest, location or age-word checks.
var proseFields = map[string]bool{
	"attribution": true, "selection_note": true, "meaning": true, "rule": true, "condition": true,
	"subject": true, "evidence": true, "mode_predicate": true, "unlisted": true,
	"boundary": true, "differences": true, "description": true, "impact": true,
}

// publicationProblem reports why a committed string may not be published.
func publicationProblem(text string, isProse bool) string {
	switch {
	case strings.IndexFunc(text, controlCharacterTest) >= 0:
		return "has a control or invisible character"
	case hasHexRun(text):
		return "carries a digest-like hexadecimal run"
	case namesLocation(text):
		return "names a location, a file or a forbidden word"
	case len(wordSpans(strings.ToLower(text), bannedTerms, true)) > 0:
		return "carries a term the repository terminology gate refuses"
	case isProse && dropSentencePattern.MatchString(text):
		return "carries operational-review or authorization wording"
	case ageNearRelease(text):
		return "places an age word next to a release word"
	}
	return ""
}

// guardStrings walks every string of a document and fails on the first one
// that may not be published.
func guardStrings(label string, value any) error {
	var problem error
	walkStrings(value, func(path, text string) {
		if problem != nil {
			return
		}
		field := path[strings.LastIndex(path, ".")+1:]
		if bracket := strings.IndexByte(field, '['); bracket >= 0 {
			field = field[:bracket]
		}
		if reason := publicationProblem(text, proseFields[field]); reason != "" {
			problem = fmt.Errorf("%s %s %s: %q", label, path, reason, text)
		}
	})
	return problem
}

// releaseWords are the words next to which no age word may stand. They are
// kept apart from the age words above so that this file passes the same rule.
var releaseWords = []string{"firmware", "software", "version", "build", "release"}
