// Package certification holds, checks and looks up the values that
// certificates state about the measurement accuracy of the devices this
// registry drives. A record is a statement about a device under certified
// conditions; it is kept apart from everything a device publishes, and the
// loader reads no network and fabricates no entry.
package certification

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Contract is the identifier every document states.
const Contract = "helianthus.semreg.certification-references/v1"

// Source is the one source category of the contract.
const Source = "certification"

// Category is the category of an entry.
type Category string

// The three categories of an entry.
const (
	CategoryReference   Category = "reference"
	CategoryGap         Category = "gap"
	CategoryUnavailable Category = "unavailable"
)

// FirmwareForm is the form of the firmware member of an entry.
type FirmwareForm string

// The three firmware forms.
const (
	FirmwareNotStated FirmwareForm = "not_stated"
	FirmwareRelease   FirmwareForm = "release"
	FirmwareRange     FirmwareForm = "range"
)

// Coverage is the coverage of a release by a firmware record.
type Coverage string

// The coverage results.
const (
	Covered    Coverage = "covered"
	NotCovered Coverage = "not_covered"
	NotStated  Coverage = "not_stated"
)

// Reason is the stable reason of a rejected document.
type Reason string

// The failure reasons of the contract.
const (
	ReasonSchema                  Reason = "schema"
	ReasonSourceNotCertification  Reason = "source_not_certification"
	ReasonUnknownMember           Reason = "unknown_member"
	ReasonMissingMember           Reason = "missing_member"
	ReasonStatedValueNotAllowed   Reason = "stated_value_not_allowed"
	ReasonRefusalMissing          Reason = "refusal_missing"
	ReasonInvalidValue            Reason = "invalid_value"
	ReasonFirmwareInvalid         Reason = "firmware_invalid"
	ReasonFirmwareUpperBoundUnset Reason = "firmware_upper_bound_unstated"
	ReasonStatedValueInvalid      Reason = "stated_value_invalid"
	ReasonCategoryConflict        Reason = "category_conflict"
)

// Finding is the first failure of a document: one stable reason, the
// zero-based entry index when an entry is concerned, and the member when one
// is named.
type Finding struct {
	Reason   Reason
	Entry    int
	HasEntry bool
	Member   string
}

// Error implements error.
func (f *Finding) Error() string {
	text := string(f.Reason)
	if f.HasEntry {
		text += " entry " + strconv.Itoa(f.Entry)
	}
	if f.Member != "" {
		text += " member " + f.Member
	}
	return text
}

// Firmware is the firmware record of an entry. Firmware is a floor, not a
// ceiling: a release covers itself and every later release.
type Firmware struct {
	Form               FirmwareForm
	Release            string
	From               string
	To                 string
	UpperBoundStatedBy string
}

// Decimal is the canonical decimal of the stated value: Coefficient times ten
// to the Exponent, in the SI unit of the fact it feeds.
type Decimal struct {
	Coefficient string
	Exponent    int
}

// StatedValue carries only the members the certificate states; a member the
// certificate does not state is nil.
type StatedValue struct {
	MeasurementType string
	Measured        *bool
	RatioMaximum    *Decimal
	MeasuredMinimum *Decimal
	MeasuredMaximum *Decimal
	RangeMinimum    *Decimal
	RangeMaximum    *Decimal
}

// Entry is one record. StatedValue is non-nil only for a reference; Refusal is
// non-empty only for an unavailable entry.
type Entry struct {
	Category          Category
	Driver            string
	Vendor            string
	Flavor            string
	Model             string
	Firmware          Firmware
	Issuer            string
	CertificateNumber string
	IssueDate         string
	StatedValue       *StatedValue
	URL               string
	RetrievedOn       string
	Refusal           string
}

// Value returns the stated value of a reference. A gap and an unavailable
// entry hold no value, so it reports false for them.
func (e Entry) Value() (StatedValue, bool) {
	if e.Category != CategoryReference || e.StatedValue == nil {
		return StatedValue{}, false
	}
	return *cloneStated(e.StatedValue), true
}

// Document is one parsed data file.
type Document struct {
	Entries []Entry
}

var (
	releasePattern     = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)
	datePattern        = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	coefficientPattern = regexp.MustCompile(`^(0|-?[1-9]([0-9]*[1-9])?)$`)
	tokenPattern       = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	integerPattern     = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
)

var (
	commonMembers   = []string{"source", "category", "driver", "vendor", "flavor", "model", "firmware"}
	locationMembers = []string{"issuer", "url", "retrieved_on"}
	textMembers     = []string{"driver", "vendor", "flavor", "model", "issuer", "certificate_number"}
	dateMembers     = []string{"issue_date", "retrieved_on"}
	decimalMembers  = []string{"ratio_maximum", "measured_minimum", "measured_maximum", "range_minimum", "range_maximum"}
)

func categoryMembers(category string) []string {
	members := append([]string{}, commonMembers...)
	switch category {
	case string(CategoryReference):
		return append(members, "issuer", "certificate_number", "issue_date", "stated_value", "url", "retrieved_on")
	case string(CategoryGap):
		return append(members, locationMembers...)
	default:
		return append(append(members, locationMembers...), "refusal")
	}
}

func isCategory(value any) bool {
	text, ok := value.(string)
	return ok && (text == string(CategoryReference) || text == string(CategoryGap) || text == string(CategoryUnavailable))
}

func isText(value any) bool {
	text, ok := value.(string)
	return ok && text != ""
}

func isDate(value any) bool {
	text, ok := value.(string)
	if !ok || !datePattern.MatchString(text) {
		return false
	}
	_, err := time.Parse("2006-01-02", text)
	return err == nil
}

func isURL(value any) bool {
	text, ok := value.(string)
	return ok && strings.HasPrefix(text, "https://") && len(text) > len("https://") && !strings.Contains(text, " ")
}

func isRelease(value any) bool {
	text, ok := value.(string)
	return ok && releasePattern.MatchString(text)
}

func isDecimal(value any) bool {
	object, ok := value.(map[string]any)
	if !ok || len(object) != 2 {
		return false
	}
	coefficient, ok := object["coefficient"].(string)
	if !ok || !coefficientPattern.MatchString(coefficient) {
		return false
	}
	number, ok := object["exponent10"].(json.Number)
	if !ok || !integerPattern.MatchString(number.String()) {
		return false
	}
	exponent, err := strconv.Atoi(number.String())
	if err != nil || exponent < -18 || exponent > 18 {
		return false
	}
	return coefficient != "0" || exponent == 0
}

func firstMember(entry map[string]any, names []string, bad func(any) bool) string {
	for _, name := range names {
		if value, present := entry[name]; present && bad(value) {
			return name
		}
	}
	return ""
}

func memberFinding(reason Reason, member string) *Finding {
	return &Finding{Reason: reason, Member: member}
}

func checkSource(entry map[string]any) *Finding {
	if value, present := entry["source"]; present && value != Source {
		return memberFinding(ReasonSourceNotCertification, "source")
	}
	return nil
}

func checkCategory(entry map[string]any) *Finding {
	value, present := entry["category"]
	if !present {
		return memberFinding(ReasonMissingMember, "category")
	}
	if !isCategory(value) {
		return memberFinding(ReasonInvalidValue, "category")
	}
	return nil
}

func checkMembers(entry map[string]any) *Finding {
	category := entry["category"].(string)
	allowed := categoryMembers(category)
	if _, present := entry["stated_value"]; present && category != string(CategoryReference) {
		return memberFinding(ReasonStatedValueNotAllowed, "stated_value")
	}
	var extra []string
	for name := range entry {
		if !contains(allowed, name) {
			extra = append(extra, name)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return memberFinding(ReasonUnknownMember, extra[0])
	}
	if category == string(CategoryUnavailable) && !isText(entry["refusal"]) {
		return memberFinding(ReasonRefusalMissing, "refusal")
	}
	for _, name := range allowed {
		if _, present := entry[name]; !present {
			return memberFinding(ReasonMissingMember, name)
		}
	}
	return nil
}

func checkValues(entry map[string]any) *Finding {
	bad := firstMember(entry, textMembers, func(v any) bool { return !isText(v) })
	if bad == "" {
		bad = firstMember(entry, dateMembers, func(v any) bool { return !isDate(v) })
	}
	if bad == "" {
		bad = firstMember(entry, []string{"url"}, func(v any) bool { return !isURL(v) })
	}
	if bad != "" {
		return memberFinding(ReasonInvalidValue, bad)
	}
	return nil
}

func firmwareMembers(form string) []string {
	switch form {
	case string(FirmwareRelease):
		return []string{"form", "release"}
	case string(FirmwareRange):
		return []string{"form", "from", "to", "upper_bound_stated_by"}
	default:
		return []string{"form"}
	}
}

func checkFirmware(entry map[string]any) *Finding {
	invalid := memberFinding(ReasonFirmwareInvalid, "firmware")
	firmware, ok := entry["firmware"].(map[string]any)
	if !ok {
		return invalid
	}
	form, _ := firmware["form"].(string)
	if form != string(FirmwareNotStated) && form != string(FirmwareRelease) && form != string(FirmwareRange) {
		return invalid
	}
	if form == string(FirmwareRange) {
		by, _ := firmware["upper_bound_stated_by"].(string)
		if by != "certificate" && by != "vendor_document" {
			return memberFinding(ReasonFirmwareUpperBoundUnset, "firmware")
		}
	}
	expected := firmwareMembers(form)
	if len(firmware) != len(expected) {
		return invalid
	}
	for _, name := range expected {
		if _, present := firmware[name]; !present {
			return invalid
		}
	}
	for _, name := range []string{"release", "from", "to"} {
		if value, present := firmware[name]; present && !isRelease(value) {
			return invalid
		}
	}
	if form == string(FirmwareRange) && compareReleases(firmware["from"].(string), firmware["to"].(string)) > 0 {
		return invalid
	}
	return nil
}

func checkStatedValue(entry map[string]any) *Finding {
	value, present := entry["stated_value"]
	if !present {
		return nil
	}
	invalid := memberFinding(ReasonStatedValueInvalid, "stated_value")
	stated, ok := value.(map[string]any)
	if !ok {
		return invalid
	}
	states := 0
	for name, member := range stated {
		switch {
		case name == "measurement_type":
			text, isString := member.(string)
			if !isString || !tokenPattern.MatchString(text) {
				return invalid
			}
		case name == "measured":
			if _, isBool := member.(bool); !isBool {
				return invalid
			}
			states++
		case contains(decimalMembers, name):
			if !isDecimal(member) {
				return invalid
			}
			states++
		default:
			return invalid
		}
	}
	if _, named := stated["measurement_type"]; !named || states == 0 {
		return invalid
	}
	return nil
}

func contains(list []string, name string) bool {
	for _, item := range list {
		if item == name {
			return true
		}
	}
	return false
}

func checkEntry(value any) *Finding {
	entry, ok := value.(map[string]any)
	if !ok {
		return memberFinding(ReasonSchema, "")
	}
	for _, check := range []func(map[string]any) *Finding{
		checkSource, checkCategory, checkMembers, checkValues, checkFirmware, checkStatedValue,
	} {
		if found := check(entry); found != nil {
			return found
		}
	}
	return nil
}

func checkTop(top map[string]any, entries []any) *Finding {
	if len(top) != 2 || top["contract"] != Contract || len(entries) == 0 {
		return memberFinding(ReasonSchema, "")
	}
	return nil
}

func identityKey(entry Entry) string {
	f := entry.Firmware
	return strings.Join([]string{
		entry.Driver, entry.Vendor, entry.Flavor, entry.Model,
		string(f.Form), f.Release, f.From, f.To, f.UpperBoundStatedBy,
	}, "\x00")
}

func checkCategories(entries []Entry) *Finding {
	seen := map[string]Category{}
	for index, entry := range entries {
		key := identityKey(entry)
		if first, found := seen[key]; found && first != entry.Category {
			return &Finding{Reason: ReasonCategoryConflict, Entry: index, HasEntry: true}
		}
		if _, found := seen[key]; !found {
			seen[key] = entry.Category
		}
	}
	return nil
}

// Parse checks one data file and returns its entries. A rejected file returns
// a *Finding; a repeated member name or a syntax error is the schema reason.
func Parse(data []byte) (Document, error) {
	tree, err := decodeStrict(data)
	if err != nil {
		return Document{}, &Finding{Reason: ReasonSchema}
	}
	top, ok := tree.(map[string]any)
	if !ok {
		return Document{}, &Finding{Reason: ReasonSchema}
	}
	raw, ok := top["entries"].([]any)
	if !ok {
		return Document{}, &Finding{Reason: ReasonSchema}
	}
	for index, item := range raw {
		if found := checkEntry(item); found != nil {
			found.Entry, found.HasEntry = index, true
			return Document{}, found
		}
	}
	if found := checkTop(top, raw); found != nil {
		return Document{}, found
	}
	entries := make([]Entry, len(raw))
	for index, item := range raw {
		entries[index] = buildEntry(item.(map[string]any))
	}
	if found := checkCategories(entries); found != nil {
		return Document{}, found
	}
	return Document{Entries: entries}, nil
}

func decodeStrict(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := readValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("data after the document")
	}
	return value, nil
}

func readValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return token, nil
	}
	if delim == '[' {
		items := []any{}
		for decoder.More() {
			item, err := readValue(decoder)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		_, err := decoder.Token()
		return items, err
	}
	object := map[string]any{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key := keyToken.(string)
		if _, repeated := object[key]; repeated {
			return nil, fmt.Errorf("repeated member %q", key)
		}
		if object[key], err = readValue(decoder); err != nil {
			return nil, err
		}
	}
	_, err = decoder.Token()
	return object, err
}

func text(entry map[string]any, name string) string {
	value, _ := entry[name].(string)
	return value
}

func buildEntry(raw map[string]any) Entry {
	firmware := raw["firmware"].(map[string]any)
	entry := Entry{
		Category:          Category(text(raw, "category")),
		Driver:            text(raw, "driver"),
		Vendor:            text(raw, "vendor"),
		Flavor:            text(raw, "flavor"),
		Model:             text(raw, "model"),
		Issuer:            text(raw, "issuer"),
		CertificateNumber: text(raw, "certificate_number"),
		IssueDate:         text(raw, "issue_date"),
		URL:               text(raw, "url"),
		RetrievedOn:       text(raw, "retrieved_on"),
		Refusal:           text(raw, "refusal"),
		Firmware: Firmware{
			Form:               FirmwareForm(text(firmware, "form")),
			Release:            text(firmware, "release"),
			From:               text(firmware, "from"),
			To:                 text(firmware, "to"),
			UpperBoundStatedBy: text(firmware, "upper_bound_stated_by"),
		},
	}
	if stated, present := raw["stated_value"].(map[string]any); present {
		entry.StatedValue = buildStated(stated)
	}
	return entry
}

func buildDecimal(value any) *Decimal {
	object, present := value.(map[string]any)
	if !present {
		return nil
	}
	exponent, _ := strconv.Atoi(object["exponent10"].(json.Number).String())
	return &Decimal{Coefficient: object["coefficient"].(string), Exponent: exponent}
}

func buildStated(stated map[string]any) *StatedValue {
	value := &StatedValue{
		MeasurementType: text(stated, "measurement_type"),
		RatioMaximum:    buildDecimal(stated["ratio_maximum"]),
		MeasuredMinimum: buildDecimal(stated["measured_minimum"]),
		MeasuredMaximum: buildDecimal(stated["measured_maximum"]),
		RangeMinimum:    buildDecimal(stated["range_minimum"]),
		RangeMaximum:    buildDecimal(stated["range_maximum"]),
	}
	if measured, present := stated["measured"].(bool); present {
		value.Measured = &measured
	}
	return value
}

func compareComponents(left, right string) int {
	left, right = strings.TrimLeft(left, "0"), strings.TrimLeft(right, "0")
	if len(left) != len(right) {
		if len(left) < len(right) {
			return -1
		}
		return 1
	}
	return strings.Compare(left, right)
}

// compareReleases compares two releases component by component as integers, a
// missing component counting as zero.
func compareReleases(left, right string) int {
	a, b := strings.Split(left, "."), strings.Split(right, ".")
	for index := 0; index < len(a) || index < len(b); index++ {
		x, y := "0", "0"
		if index < len(a) {
			x = a[index]
		}
		if index < len(b) {
			y = b[index]
		}
		if result := compareComponents(x, y); result != 0 {
			return result
		}
	}
	return 0
}

// CoverageOf reports whether a firmware record covers a release. A release
// that is not dot-separated integers has no order and is covered by no
// stated release or range; a record with firmware not stated reports
// NotStated for every release.
func CoverageOf(firmware Firmware, release string) Coverage {
	if firmware.Form == FirmwareNotStated {
		return NotStated
	}
	if !releasePattern.MatchString(release) {
		return NotCovered
	}
	floor := firmware.Release
	if firmware.Form == FirmwareRange {
		floor = firmware.From
	}
	if compareReleases(release, floor) < 0 {
		return NotCovered
	}
	if firmware.Form == FirmwareRange && compareReleases(release, firmware.To) > 0 {
		return NotCovered
	}
	return Covered
}

// Set is the merged entries of the data files.
type Set struct {
	entries []Entry
}

// NewSet merges documents in the order given and rejects two entries of
// different categories for one driver, vendor, flavor, model and firmware.
func NewSet(documents ...Document) (*Set, error) {
	var entries []Entry
	for _, document := range documents {
		for _, entry := range document.Entries {
			entries = append(entries, cloneEntry(entry))
		}
	}
	if found := checkCategories(entries); found != nil {
		return nil, found
	}
	return &Set{entries: entries}, nil
}

// Entries returns a copy of every entry in order.
func (s *Set) Entries() []Entry {
	out := make([]Entry, len(s.entries))
	for index, entry := range s.entries {
		out[index] = cloneEntry(entry)
	}
	return out
}

// Query names the device a lookup is for. Firmware is the release the device
// reports; empty when it is not known.
type Query struct {
	Driver   string
	Vendor   string
	Flavor   string
	Model    string
	Firmware string
}

// Match is one entry that applies to a query, with its firmware coverage.
type Match struct {
	Entry    Entry
	Coverage Coverage
}

// Lookup returns, in order, every entry whose driver, vendor, flavor and model
// equal the query and whose firmware record covers the queried release, or
// states no firmware. A gap or an unavailable entry is returned as it is; it
// is never turned into a value.
func (s *Set) Lookup(query Query) []Match {
	var matches []Match
	for _, entry := range s.entries {
		if entry.Driver != query.Driver || entry.Vendor != query.Vendor ||
			entry.Flavor != query.Flavor || entry.Model != query.Model {
			continue
		}
		coverage := CoverageOf(entry.Firmware, query.Firmware)
		if coverage == NotCovered {
			continue
		}
		matches = append(matches, Match{Entry: cloneEntry(entry), Coverage: coverage})
	}
	return matches
}

func cloneEntry(entry Entry) Entry {
	entry.StatedValue = cloneStated(entry.StatedValue)
	return entry
}

func cloneDecimal(value *Decimal) *Decimal {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneStated(value *StatedValue) *StatedValue {
	if value == nil {
		return nil
	}
	copied := *value
	if value.Measured != nil {
		measured := *value.Measured
		copied.Measured = &measured
	}
	copied.RatioMaximum = cloneDecimal(value.RatioMaximum)
	copied.MeasuredMinimum = cloneDecimal(value.MeasuredMinimum)
	copied.MeasuredMaximum = cloneDecimal(value.MeasuredMaximum)
	copied.RangeMinimum = cloneDecimal(value.RangeMinimum)
	copied.RangeMaximum = cloneDecimal(value.RangeMaximum)
	return &copied
}

//go:embed *.json
var dataFiles embed.FS

// Load parses every embedded data file, one per device family, in file name
// order, and merges them. The schema file is not a data file.
func Load() (*Set, error) {
	names, err := dataFiles.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var documents []Document
	for _, name := range names {
		if name.IsDir() || !strings.HasSuffix(name.Name(), ".json") || strings.HasSuffix(name.Name(), ".schema.json") {
			continue
		}
		data, err := dataFiles.ReadFile(name.Name())
		if err != nil {
			return nil, err
		}
		document, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name.Name(), err)
		}
		documents = append(documents, document)
	}
	return NewSet(documents...)
}
