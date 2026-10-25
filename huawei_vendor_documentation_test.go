package modbusreg

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"
)

// huaweiDocumentIdentity is the registry pin of one consulted vendor document:
// the title, the version and the date, each as the document states it. It
// carries no excerpt, link, locator or digest.
type huaweiDocumentIdentity struct {
	Title   string
	Version string
	Date    string
}

const (
	huaweiDocumentTitleLimit    = 200
	huaweiDocumentVersionLimit  = 64
	huaweiDocumentDateNotStated = "not stated"
	huaweiDocumentFirstYear     = 1979
)

var (
	huaweiDispositionPaths = []string{
		"profiles/vendor/huawei/smartlogger-disposition.json",
		"profiles/vendor/huawei/sdongle-disposition.json",
		"profiles/vendor/huawei/emma-disposition.json",
		"profiles/vendor/huawei/sun2000-disposition.json",
		"profiles/vendor/huawei/luna2000-disposition.json",
	}

	huaweiVendorDocumentationKeys = []string{"source_class", "monitoring_profile_basis", "consulted_documents", "changes_outcome"}

	huaweiDocumentLocatorPattern = regexp.MustCompile(`(?i)(?:[a-z][a-z0-9+.-]*://|[a-z0-9._%+-]+@[a-z0-9-]+(?:\.[a-z0-9-]+)+|\bwww\.|\b(?:mailto|file|ftp):|(?:^|[\s(\[<])[a-z0-9-]+(?:\.[a-z0-9-]+)*\.[a-z]{2,24}(?:/|[\s)\]>,;]|$))`)
	huaweiDocumentDigestPattern  = regexp.MustCompile(`(?i)(?:[0-9a-f]{32,}|\b(?:md5|sha-?[0-9]{1,3}|blake[0-9a-z]*|crc-?[0-9]+)\b|[a-z0-9+/]{40,})`)
	huaweiDocumentRangePattern   = regexp.MustCompile(`[*<>~|]`)

	huaweiDocumentPlaceholders = map[string]bool{
		"none": true, "unknown": true, "unpinned": true, "tbd": true, "n/a": true, "not stated": true,
	}
)

func huaweiDocumentTextProblem(text string, limit int) string {
	switch {
	case !utf8.ValidString(text):
		return "is not valid UTF-8"
	case strings.TrimSpace(text) == "":
		return "is empty"
	case strings.TrimSpace(text) != text:
		return "has leading or trailing space"
	case utf8.RuneCountInString(text) > limit:
		return "is too long"
	case strings.IndexFunc(text, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) >= 0:
		return "has a control or invisible character"
	case huaweiDocumentPlaceholders[strings.ToLower(text)]:
		return "is a placeholder"
	case huaweiDocumentLocatorPattern.MatchString(text):
		return "names a location"
	case huaweiDocumentDigestPattern.MatchString(text):
		return "carries a digest"
	}
	return ""
}

func huaweiDocumentDateProblem(text string) string {
	if text == huaweiDocumentDateNotStated {
		return ""
	}
	for _, layout := range []string{"2006-01-02", "2006-01", "2006"} {
		parsed, err := time.Parse(layout, text)
		if err != nil {
			continue
		}
		if parsed.Year() < huaweiDocumentFirstYear {
			return "is before any Modbus document"
		}
		return ""
	}
	return "is not a year, a year and month, a date, or not stated"
}

func huaweiDocumentIdentityOf(entry any) (huaweiDocumentIdentity, error) {
	var identity huaweiDocumentIdentity
	object, ok := entry.(map[string]any)
	if !ok {
		return identity, fmt.Errorf("is not an object")
	}
	for key := range object {
		if key != "title" && key != "version" && key != "date" {
			return identity, fmt.Errorf("carries %q, which is not document identity", key)
		}
	}
	for _, field := range []struct {
		name  string
		limit int
		dest  *string
	}{
		{"title", huaweiDocumentTitleLimit, &identity.Title},
		{"version", huaweiDocumentVersionLimit, &identity.Version},
		{"date", 0, &identity.Date},
	} {
		raw, present := object[field.name]
		if !present {
			return identity, fmt.Errorf("lacks %s", field.name)
		}
		text, ok := raw.(string)
		if !ok {
			return identity, fmt.Errorf("%s is not a string", field.name)
		}
		problem := ""
		if field.name == "date" {
			problem = huaweiDocumentDateProblem(text)
		} else {
			problem = huaweiDocumentTextProblem(text, field.limit)
		}
		if problem == "" && field.name == "version" && huaweiDocumentRangePattern.MatchString(text) {
			problem = "reads as a range"
		}
		if problem != "" {
			return identity, fmt.Errorf("%s %s", field.name, problem)
		}
		*field.dest = text
	}
	return identity, nil
}

func validateHuaweiConsultedDocuments(value any) ([]huaweiDocumentIdentity, error) {
	entries, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("consulted_documents is not an array")
	}
	identities := make([]huaweiDocumentIdentity, 0, len(entries))
	seen := make(map[huaweiDocumentIdentity]bool, len(entries))
	for index, entry := range entries {
		identity, err := huaweiDocumentIdentityOf(entry)
		if err != nil {
			return nil, fmt.Errorf("consulted_documents[%d] %w", index, err)
		}
		if seen[identity] {
			return nil, fmt.Errorf("consulted_documents[%d] repeats an earlier document row", index)
		}
		seen[identity] = true
		identities = append(identities, identity)
	}
	return identities, nil
}

// huaweiConsultedDocuments reads the optional vendor_documentation block of a
// disposition's evidence. An absent block is an empty set.
func huaweiConsultedDocuments(evidence map[string]any) ([]huaweiDocumentIdentity, error) {
	raw, present := evidence["vendor_documentation"]
	if !present {
		return nil, nil
	}
	block, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("vendor_documentation is not an object")
	}
	for _, key := range huaweiVendorDocumentationKeys {
		if _, ok := block[key]; !ok {
			return nil, fmt.Errorf("vendor_documentation lacks %s", key)
		}
	}
	if len(block) != len(huaweiVendorDocumentationKeys) {
		return nil, fmt.Errorf("vendor_documentation carries a key outside the field set")
	}
	return validateHuaweiConsultedDocuments(block["consulted_documents"])
}

// huaweiRestsOnConsultedDocument is false for a record whose pin is absent,
// empty, incomplete or invalid: such a record has no consulted document.
func huaweiRestsOnConsultedDocument(evidence map[string]any) bool {
	identities, err := huaweiConsultedDocuments(evidence)
	return err == nil && len(identities) > 0
}

func huaweiDocumentEntry(title, version, date any) map[string]any {
	return map[string]any{"title": title, "version": version, "date": date}
}

func huaweiSyntheticDigest() (hexDigits, base64Digits string) {
	sum := sha256.Sum256([]byte("synthetic document content"))
	return hex.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString(sum[:])
}

func huaweiTextOfLength(length int) string {
	text := strings.Repeat("Ab ", length/3+2)[:length]
	if strings.HasSuffix(text, " ") {
		text = text[:length-1] + "Z"
	}
	return text
}

const (
	huaweiSampleTitle   = "Example Interface Definitions"
	huaweiSampleVersion = "Issue 7"
	huaweiSampleDate    = "2025-03-14"
)

func TestHuaweiDispositionsCarryTheEmptyVendorDocumentationFieldSet(t *testing.T) {
	for _, path := range huaweiDispositionPaths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			record := decodeHuaweiObject(t, path)
			evidence := huaweiObject(t, record["evidence"])
			block := huaweiObject(t, evidence["vendor_documentation"])
			requireHuaweiKeys(t, block, huaweiVendorDocumentationKeys...)
			if block["source_class"] != "OFFICIAL_VENDOR_DOCUMENTATION_ONLY" || block["changes_outcome"] != false ||
				!reflect.DeepEqual(huaweiStringSlice(t, block["monitoring_profile_basis"]), []string{
					"pinned_vendor_document_identity", "operator_site_observation_fixtures",
				}) {
				t.Fatalf("unexpected vendor documentation policy: %#v", block)
			}
			if documents := h03Array(t, block["consulted_documents"]); len(documents) != 0 {
				t.Fatalf("a document identity was supplied without the operator: %#v", documents)
			}
			identities, err := huaweiConsultedDocuments(evidence)
			if err != nil || len(identities) != 0 || huaweiRestsOnConsultedDocument(evidence) {
				t.Fatalf("empty field set read as %v, %v", identities, err)
			}
			if record["catalog_registered"] != false || record["automatic_runtime_admission"] != false || record["support_claim"] != false ||
				len(h03Array(t, record["decoder_keys"])) != 0 {
				t.Fatalf("the field set changed an authority field: %#v", record)
			}
		})
	}
}

func TestHuaweiDispositionsNoLongerForbidUsingTheVendorDocument(t *testing.T) {
	var paths []string
	paths = append(paths, huaweiDispositionPaths...)
	paths = append(paths, "profiles/vendor/huawei/evidence.json", "README.md")
	docs, err := filepath.Glob("docs/huawei-*.md")
	if err != nil || len(docs) == 0 {
		t.Fatalf("Huawei documents not found: %v", err)
	}
	paths = append(paths, docs...)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(data))
		for _, stale := range []string{"inspection-only", "inspection only", "unpinned", "version_pinned", "pinned_vendor_document_version", "treated conservatively"} {
			if strings.Contains(lower, stale) {
				t.Fatalf("%s still carries %q", path, stale)
			}
		}
	}
	for _, path := range huaweiDispositionPaths {
		if strings.Contains(string(mustReadHuaweiFile(t, path)), `"locator"`) {
			t.Fatalf("%s carries a locator; a pin is identity only", path)
		}
	}
	if !strings.Contains(string(mustReadHuaweiFile(t, "README.md")), "docs/huawei-vendor-documentation.md") {
		t.Fatal("README does not point at the vendor documentation page")
	}
	page := string(mustReadHuaweiFile(t, "docs/huawei-vendor-documentation.md"))
	for _, phrase := range []string{"vendor_documentation", "consulted_documents", "pinned_vendor_document_identity", "title", "version", "date"} {
		if !strings.Contains(page, phrase) {
			t.Fatalf("the vendor documentation page lacks %q", phrase)
		}
	}
}

func mustReadHuaweiFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestHuaweiConsultedDocumentsAcceptExactIdentities(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  []huaweiDocumentIdentity
	}{
		{"empty set", []any{}, []huaweiDocumentIdentity{}},
		{"full date", []any{huaweiDocumentEntry(huaweiSampleTitle, huaweiSampleVersion, "2025-03-14")},
			[]huaweiDocumentIdentity{{huaweiSampleTitle, huaweiSampleVersion, "2025-03-14"}}},
		{"year and month", []any{huaweiDocumentEntry(huaweiSampleTitle, huaweiSampleVersion, "2025-03")},
			[]huaweiDocumentIdentity{{huaweiSampleTitle, huaweiSampleVersion, "2025-03"}}},
		{"year", []any{huaweiDocumentEntry(huaweiSampleTitle, huaweiSampleVersion, "2025")},
			[]huaweiDocumentIdentity{{huaweiSampleTitle, huaweiSampleVersion, "2025"}}},
		{"date not stated by the document", []any{huaweiDocumentEntry(huaweiSampleTitle, huaweiSampleVersion, "not stated")},
			[]huaweiDocumentIdentity{{huaweiSampleTitle, huaweiSampleVersion, "not stated"}}},
		{"first year", []any{huaweiDocumentEntry(huaweiSampleTitle, huaweiSampleVersion, "1979")},
			[]huaweiDocumentIdentity{{huaweiSampleTitle, huaweiSampleVersion, "1979"}}},
		{"leap day", []any{huaweiDocumentEntry(huaweiSampleTitle, huaweiSampleVersion, "2024-02-29")},
			[]huaweiDocumentIdentity{{huaweiSampleTitle, huaweiSampleVersion, "2024-02-29"}}},
		{"version label with dots and parentheses", []any{huaweiDocumentEntry(huaweiSampleTitle, "V3.0 (Issue 05)", huaweiSampleDate)},
			[]huaweiDocumentIdentity{{huaweiSampleTitle, "V3.0 (Issue 05)", huaweiSampleDate}}},
		{"abbreviation with a trailing dot", []any{huaweiDocumentEntry("Interface Definitions Rev. B", "Rev. B", huaweiSampleDate)},
			[]huaweiDocumentIdentity{{"Interface Definitions Rev. B", "Rev. B", huaweiSampleDate}}},
		{"title at the limit", []any{huaweiDocumentEntry(huaweiTextOfLength(huaweiDocumentTitleLimit), huaweiSampleVersion, huaweiSampleDate)},
			[]huaweiDocumentIdentity{{huaweiTextOfLength(huaweiDocumentTitleLimit), huaweiSampleVersion, huaweiSampleDate}}},
		{"version at the limit", []any{huaweiDocumentEntry(huaweiSampleTitle, huaweiTextOfLength(huaweiDocumentVersionLimit), huaweiSampleDate)},
			[]huaweiDocumentIdentity{{huaweiSampleTitle, huaweiTextOfLength(huaweiDocumentVersionLimit), huaweiSampleDate}}},
		{"two versions of one title are two rows", []any{
			huaweiDocumentEntry(huaweiSampleTitle, "Issue 6", "2024-11-02"),
			huaweiDocumentEntry(huaweiSampleTitle, "Issue 7", huaweiSampleDate),
		}, []huaweiDocumentIdentity{
			{huaweiSampleTitle, "Issue 6", "2024-11-02"},
			{huaweiSampleTitle, "Issue 7", huaweiSampleDate},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateHuaweiConsultedDocuments(tc.value)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, %v want %v", got, err, tc.want)
			}
		})
	}
}

func TestHuaweiConsultedDocumentsRejectInvalidFieldSets(t *testing.T) {
	hexDigits, base64Digits := huaweiSyntheticDigest()
	entry := func(title, version, date any) []any { return []any{huaweiDocumentEntry(title, version, date)} }
	withExtra := func(key string) []any {
		row := huaweiDocumentEntry(huaweiSampleTitle, huaweiSampleVersion, huaweiSampleDate)
		row[key] = "x"
		return []any{row}
	}
	cases := []struct {
		name  string
		value any
	}{
		// The value as a whole.
		{"set is absent", nil},
		{"set is an object", map[string]any{"title": huaweiSampleTitle}},
		{"set is text", huaweiSampleTitle},
		{"row is text", []any{huaweiSampleTitle}},
		{"row is empty", []any{map[string]any{}}},
		{"identical rows repeat", []any{
			huaweiDocumentEntry(huaweiSampleTitle, huaweiSampleVersion, huaweiSampleDate),
			huaweiDocumentEntry(huaweiSampleTitle, huaweiSampleVersion, huaweiSampleDate),
		}},
		// Content that is not identity.
		{"row carries a link field", withExtra("url")},
		{"row carries a locator field", withExtra("locator")},
		{"row carries a digest field", withExtra("digest")},
		{"row carries an excerpt field", withExtra("excerpt")},
		{"row carries a page field", withExtra("page")},

		// Title.
		{"title is missing", []any{map[string]any{"version": huaweiSampleVersion, "date": huaweiSampleDate}}},
		{"title is empty", entry("", huaweiSampleVersion, huaweiSampleDate)},
		{"title is blank", entry("   ", huaweiSampleVersion, huaweiSampleDate)},
		{"title has leading space", entry(" "+huaweiSampleTitle, huaweiSampleVersion, huaweiSampleDate)},
		{"title has trailing space", entry(huaweiSampleTitle+" ", huaweiSampleVersion, huaweiSampleDate)},
		{"title has a line break", entry("Example\nDefinitions", huaweiSampleVersion, huaweiSampleDate)},
		{"title has an invisible character", entry("Example"+string(rune(0x200b))+"Definitions", huaweiSampleVersion, huaweiSampleDate)},
		{"title is not text", entry(7, huaweiSampleVersion, huaweiSampleDate)},
		{"title is not valid UTF-8", entry("Example\xffDefinitions", huaweiSampleVersion, huaweiSampleDate)},
		{"title is over the limit", entry(huaweiTextOfLength(huaweiDocumentTitleLimit+1), huaweiSampleVersion, huaweiSampleDate)},
		{"title is a placeholder", entry("none", huaweiSampleVersion, huaweiSampleDate)},
		{"title is the unpinned marker", entry("UNPINNED", huaweiSampleVersion, huaweiSampleDate)},
		{"title carries a link with a scheme", entry("https://documents.invalid/definitions", huaweiSampleVersion, huaweiSampleDate)},
		{"title carries a link inside text", entry("Definitions at https://documents.invalid/a", huaweiSampleVersion, huaweiSampleDate)},
		{"title carries a web host", entry("www.documents.invalid Definitions", huaweiSampleVersion, huaweiSampleDate)},
		{"title carries a host and path", entry("documents.invalid/definitions", huaweiSampleVersion, huaweiSampleDate)},
		{"title carries a host in parentheses", entry("Definitions (documents.invalid)", huaweiSampleVersion, huaweiSampleDate)},
		{"title carries a mail address scheme", entry("mailto:owner@documents.invalid", huaweiSampleVersion, huaweiSampleDate)},
		{"title carries a bare mail address", entry("Definitions owner@documents.invalid", huaweiSampleVersion, huaweiSampleDate)},
		{"title carries a bare mail address alone", entry("owner@documents.invalid", huaweiSampleVersion, huaweiSampleDate)},
		{"title carries a 64-digit hexadecimal digest", entry(huaweiSampleTitle+" "+hexDigits, huaweiSampleVersion, huaweiSampleDate)},
		{"title carries a 40-digit hexadecimal digest", entry(huaweiSampleTitle+" "+hexDigits[:40], huaweiSampleVersion, huaweiSampleDate)},
		{"title carries a base64 digest", entry(huaweiSampleTitle+" "+base64Digits, huaweiSampleVersion, huaweiSampleDate)},
		{"title names a digest algorithm", entry("sha256 "+huaweiSampleTitle, huaweiSampleVersion, huaweiSampleDate)},

		// Version.
		{"version is missing", []any{map[string]any{"title": huaweiSampleTitle, "date": huaweiSampleDate}}},
		{"version is empty", entry(huaweiSampleTitle, "", huaweiSampleDate)},
		{"version is blank", entry(huaweiSampleTitle, "  ", huaweiSampleDate)},
		{"version has leading space", entry(huaweiSampleTitle, " "+huaweiSampleVersion, huaweiSampleDate)},
		{"version has trailing space", entry(huaweiSampleTitle, huaweiSampleVersion+" ", huaweiSampleDate)},
		{"version has a tab", entry(huaweiSampleTitle, "Issue\t7", huaweiSampleDate)},
		{"version is not text", entry(huaweiSampleTitle, 7, huaweiSampleDate)},
		{"version is over the limit", entry(huaweiSampleTitle, huaweiTextOfLength(huaweiDocumentVersionLimit+1), huaweiSampleDate)},
		{"version is the unpinned marker", entry(huaweiSampleTitle, "UNPINNED", huaweiSampleDate)},
		{"version carries a bare mail address", entry(huaweiSampleTitle, "owner@documents.invalid", huaweiSampleDate)},
		{"version is not stated", entry(huaweiSampleTitle, "not stated", huaweiSampleDate)},
		{"version is a placeholder", entry(huaweiSampleTitle, "TBD", huaweiSampleDate)},
		{"version carries a link", entry(huaweiSampleTitle, "https://documents.invalid/issue-7", huaweiSampleDate)},
		{"version carries a host and path", entry(huaweiSampleTitle, "documents.invalid/issue-7", huaweiSampleDate)},
		{"version carries a 64-digit hexadecimal digest", entry(huaweiSampleTitle, hexDigits, huaweiSampleDate)},
		{"version carries a 32-digit hexadecimal digest", entry(huaweiSampleTitle, hexDigits[:32], huaweiSampleDate)},
		{"version carries a base64 digest", entry(huaweiSampleTitle, base64Digits, huaweiSampleDate)},
		{"version names a digest algorithm", entry(huaweiSampleTitle, "md5 Issue 7", huaweiSampleDate)},
		{"version is a wildcard", entry(huaweiSampleTitle, "Issue 7.*", huaweiSampleDate)},
		{"version is a lower bound", entry(huaweiSampleTitle, ">= Issue 7", huaweiSampleDate)},
		{"version is an alternative", entry(huaweiSampleTitle, "Issue 6 | Issue 7", huaweiSampleDate)},
		{"version is approximate", entry(huaweiSampleTitle, "~Issue 7", huaweiSampleDate)},

		// Date.
		{"date is missing", []any{map[string]any{"title": huaweiSampleTitle, "version": huaweiSampleVersion}}},
		{"date is empty", entry(huaweiSampleTitle, huaweiSampleVersion, "")},
		{"date is not text", entry(huaweiSampleTitle, huaweiSampleVersion, 2025)},
		{"date has leading space", entry(huaweiSampleTitle, huaweiSampleVersion, " 2025")},
		{"date has trailing space", entry(huaweiSampleTitle, huaweiSampleVersion, "2025 ")},
		{"date month is past twelve", entry(huaweiSampleTitle, huaweiSampleVersion, "2025-13-01")},
		{"date month is zero", entry(huaweiSampleTitle, huaweiSampleVersion, "2025-00")},
		{"date day is not on the calendar", entry(huaweiSampleTitle, huaweiSampleVersion, "2025-02-30")},
		{"date leap day is not on the calendar", entry(huaweiSampleTitle, huaweiSampleVersion, "2025-02-29")},
		{"date day is not padded", entry(huaweiSampleTitle, huaweiSampleVersion, "2025-03-4")},
		{"date month is not padded", entry(huaweiSampleTitle, huaweiSampleVersion, "2025-3")},
		{"date year has two digits", entry(huaweiSampleTitle, huaweiSampleVersion, "25-03-14")},
		{"date uses slashes", entry(huaweiSampleTitle, huaweiSampleVersion, "2025/03/14")},
		{"date is written in words", entry(huaweiSampleTitle, huaweiSampleVersion, "March 2025")},
		{"date carries a time", entry(huaweiSampleTitle, huaweiSampleVersion, "2025-03-14T00:00:00Z")},
		{"date is before any Modbus document", entry(huaweiSampleTitle, huaweiSampleVersion, "1978")},
		{"date is the zero year", entry(huaweiSampleTitle, huaweiSampleVersion, "0000")},
		{"date marker is spelled differently", entry(huaweiSampleTitle, huaweiSampleVersion, "Not Stated")},
		{"date marker is hyphenated", entry(huaweiSampleTitle, huaweiSampleVersion, "not-stated")},
		{"date is a placeholder", entry(huaweiSampleTitle, huaweiSampleVersion, "none")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := validateHuaweiConsultedDocuments(tc.value); err == nil {
				t.Fatalf("invalid field set was accepted: %v", got)
			}
			evidence := map[string]any{"vendor_documentation": huaweiBlockOf(tc.value)}
			if huaweiRestsOnConsultedDocument(evidence) {
				t.Fatalf("a record with an invalid pin rests on a consulted document")
			}
		})
	}
}

func huaweiBlockOf(documents any) map[string]any {
	return map[string]any{
		"source_class":             "OFFICIAL_VENDOR_DOCUMENTATION_ONLY",
		"monitoring_profile_basis": []any{"pinned_vendor_document_identity", "operator_site_observation_fixtures"},
		"consulted_documents":      documents,
		"changes_outcome":          false,
	}
}

func TestHuaweiConsultedDocumentRowsAreDistinctByEveryField(t *testing.T) {
	// A row is one exact document version: rows that differ in exactly one
	// field stay separate, and only a row equal in all three fields repeats.
	base := huaweiDocumentEntry(huaweiSampleTitle, huaweiSampleVersion, huaweiSampleDate)
	variants := []struct {
		name string
		row  map[string]any
	}{
		{"title differs", huaweiDocumentEntry("Another Interface Definitions", huaweiSampleVersion, huaweiSampleDate)},
		{"version differs", huaweiDocumentEntry(huaweiSampleTitle, "Issue 8", huaweiSampleDate)},
		{"date differs", huaweiDocumentEntry(huaweiSampleTitle, huaweiSampleVersion, "2025-03-15")},
		{"date precision differs", huaweiDocumentEntry(huaweiSampleTitle, huaweiSampleVersion, "2025-03")},
		{"case of the title differs", huaweiDocumentEntry(strings.ToUpper(huaweiSampleTitle), huaweiSampleVersion, huaweiSampleDate)},
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			got, err := validateHuaweiConsultedDocuments([]any{base, variant.row})
			if err != nil || len(got) != 2 {
				t.Fatalf("rows differing in one field were merged or refused: %v, %v", got, err)
			}
		})
	}
	if _, err := validateHuaweiConsultedDocuments([]any{base, variants[0].row, base}); err == nil {
		t.Fatal("a repeated row was accepted after a distinct row")
	}
}

func TestHuaweiVendorDocumentationIsOptionalAndOnlyIdentityReachesTheRecord(t *testing.T) {
	complete := huaweiDocumentEntry(huaweiSampleTitle, huaweiSampleVersion, huaweiSampleDate)
	for _, path := range huaweiDispositionPaths {
		t.Run(filepath.Base(path)+" absent block", func(t *testing.T) {
			evidence := huaweiObject(t, decodeHuaweiObject(t, path)["evidence"])
			delete(evidence, "vendor_documentation")
			identities, err := huaweiConsultedDocuments(evidence)
			if err != nil || len(identities) != 0 || huaweiRestsOnConsultedDocument(evidence) {
				t.Fatalf("an absent block must read as an empty set: %v, %v", identities, err)
			}
		})
		t.Run(filepath.Base(path)+" complete pin", func(t *testing.T) {
			evidence := huaweiObject(t, decodeHuaweiObject(t, path)["evidence"])
			block := huaweiObject(t, evidence["vendor_documentation"])
			block["consulted_documents"] = []any{complete}
			identities, err := huaweiConsultedDocuments(evidence)
			if err != nil || len(identities) != 1 || !huaweiRestsOnConsultedDocument(evidence) {
				t.Fatalf("a complete pin was not read: %v, %v", identities, err)
			}
		})
		for _, field := range []string{"title", "version", "date"} {
			t.Run(filepath.Base(path)+" pin without "+field, func(t *testing.T) {
				evidence := huaweiObject(t, decodeHuaweiObject(t, path)["evidence"])
				block := huaweiObject(t, evidence["vendor_documentation"])
				row := huaweiDocumentEntry(huaweiSampleTitle, huaweiSampleVersion, huaweiSampleDate)
				row[field] = ""
				block["consulted_documents"] = []any{row}
				if _, err := huaweiConsultedDocuments(evidence); err == nil || huaweiRestsOnConsultedDocument(evidence) {
					t.Fatalf("a pin that lacks the %s was accepted", field)
				}
			})
		}
		for _, key := range huaweiVendorDocumentationKeys {
			t.Run(filepath.Base(path)+" block without "+key, func(t *testing.T) {
				evidence := huaweiObject(t, decodeHuaweiObject(t, path)["evidence"])
				delete(huaweiObject(t, evidence["vendor_documentation"]), key)
				if _, err := huaweiConsultedDocuments(evidence); err == nil {
					t.Fatalf("a block without %s was accepted", key)
				}
			})
		}
		for _, key := range huaweiVendorDocumentationKeys {
			t.Run(filepath.Base(path)+" block with "+key+" renamed", func(t *testing.T) {
				evidence := huaweiObject(t, decodeHuaweiObject(t, path)["evidence"])
				block := huaweiObject(t, evidence["vendor_documentation"])
				block[key+"_renamed"] = block[key]
				delete(block, key)
				if _, err := huaweiConsultedDocuments(evidence); err == nil {
					t.Fatalf("a block with %s renamed was accepted", key)
				}
			})
		}
		t.Run(filepath.Base(path)+" block with an extra key", func(t *testing.T) {
			evidence := huaweiObject(t, decodeHuaweiObject(t, path)["evidence"])
			huaweiObject(t, evidence["vendor_documentation"])["locator"] = "x"
			if _, err := huaweiConsultedDocuments(evidence); err == nil {
				t.Fatal("a block with a key outside the field set was accepted")
			}
		})
		t.Run(filepath.Base(path)+" block is not an object", func(t *testing.T) {
			evidence := huaweiObject(t, decodeHuaweiObject(t, path)["evidence"])
			evidence["vendor_documentation"] = []any{}
			if _, err := huaweiConsultedDocuments(evidence); err == nil {
				t.Fatal("a block that is not an object was accepted")
			}
		})
	}
}

func TestHuaweiPublicEvidenceCitesTheVendorDocumentationPolicy(t *testing.T) {
	evidence := decodeHuaweiObject(t, "profiles/vendor/huawei/evidence.json")
	const want = "https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/applicability-and-licensing.md"
	found := 0
	for _, value := range h03Array(t, evidence["public_sources"]) {
		source := huaweiObject(t, value)
		if source["id"] == "modbus-applicability-and-licensing" {
			if source["locator"] != want || source["license"] != "CC0-1.0" {
				t.Fatalf("policy source: %#v", source)
			}
			found++
		}
	}
	if found != 1 {
		t.Fatalf("the policy page is listed %d times", found)
	}
	applicability := strings.Join(huaweiStringSlice(t, evidence["applicability"]), "\n")
	for _, phrase := range []string{
		"official vendor documentation", "title, version and date", "no document is consulted", "default denied",
	} {
		if !strings.Contains(applicability, phrase) {
			t.Fatalf("applicability lacks %q", phrase)
		}
	}
}
