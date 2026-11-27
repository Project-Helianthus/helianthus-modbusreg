package certification

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func validSet(t *testing.T) *Set {
	t.Helper()
	document, err := Parse(readTestdata(t, "valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	set, err := NewSet(document)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func validTree(t *testing.T) map[string]any {
	t.Helper()
	var tree map[string]any
	if err := json.Unmarshal(readTestdata(t, "valid.json"), &tree); err != nil {
		t.Fatal(err)
	}
	return tree
}

func entryAt(tree map[string]any, index int) map[string]any {
	return tree["entries"].([]any)[index].(map[string]any)
}

func wantFinding(t *testing.T, tree map[string]any, reason Reason, entry int, member string) {
	t.Helper()
	data, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(data)
	finding, ok := err.(*Finding)
	if !ok {
		t.Fatalf("want finding %s, got %v", reason, err)
	}
	if finding.Reason != reason || finding.Member != member || (finding.HasEntry && finding.Entry != entry) {
		t.Fatalf("want %s entry %d member %q, got %v", reason, entry, member, finding)
	}
}

func TestParseValid(t *testing.T) {
	set := validSet(t)
	entries := set.Entries()
	want := []Category{CategoryReference, CategoryReference, CategoryReference, CategoryReference, CategoryGap, CategoryGap, CategoryUnavailable}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries", len(entries))
	}
	for index, category := range want {
		if entries[index].Category != category {
			t.Fatalf("entry %d is %s", index, entries[index].Category)
		}
	}
	value, ok := entries[0].Value()
	if !ok || value.MeasurementType != "active_power" || value.Measured == nil || !*value.Measured ||
		!reflect.DeepEqual(value.RatioMaximum, &Decimal{Coefficient: "1", Exponent: -2}) || value.RangeMinimum != nil ||
		value.Statement != "" {
		t.Fatalf("stated value %+v", value)
	}
}

func TestLoadEmbeddedParsesEveryDataFile(t *testing.T) {
	set, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if set == nil {
		t.Fatal("no set")
	}
}

func TestEmbeddedSchemaStatesContract(t *testing.T) {
	var schema struct {
		Properties struct {
			Contract struct {
				Const string `json:"const"`
			} `json:"contract"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(readTestdataSchema(t), &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Properties.Contract.Const != Contract {
		t.Fatalf("schema contract %q", schema.Properties.Contract.Const)
	}
}

func readTestdataSchema(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("references.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// Production mutations caught: a lookup that compares firmware as text (1.10 below 1.4), that treats the
// release as a ceiling (earlier releases covered, later not), or that drops a missing component's zero.
func TestLookupCertifiedReleaseAppliesToLaterReleases(t *testing.T) {
	set := validSet(t)
	query := Query{Driver: "example-driver", Vendor: "Example Vendor", Flavor: "example-profile", Model: "EX-1000"}
	for release, want := range map[string]int{
		"1.3.9": 0, "1.3": 0, "0.9": 0, "1.4": 2, "1.4.0": 2, "1.4.0.0": 2, "1.4.1": 2, "1.10": 2, "2.0": 2, "10.0": 2,
		"": 0, "1.4.x": 0,
	} {
		query.Firmware = release
		got := set.Lookup(query)
		if len(got) != want {
			t.Fatalf("release %q: %d matches, want %d", release, len(got), want)
		}
		for _, match := range got {
			if match.Coverage != Covered {
				t.Fatalf("release %q: coverage %s", release, match.Coverage)
			}
		}
	}
}

// Production mutations caught: a range that ignores its upper bound, includes it wrongly, or ignores its lower bound.
func TestLookupRangeStopsAtStatedUpperBound(t *testing.T) {
	set := validSet(t)
	query := Query{Driver: "example-driver", Vendor: "Example Vendor", Flavor: "example-profile", Model: "EX-1100"}
	for release, want := range map[string]int{
		"1.9": 0, "1.99.9": 0, "2.0": 1, "2.3": 1, "2.5": 1, "2.5.0": 1, "2.5.1": 0, "2.10": 0, "3.0": 0, "": 0,
	} {
		query.Firmware = release
		if got := len(set.Lookup(query)); got != want {
			t.Fatalf("release %q: %d matches, want %d", release, got, want)
		}
	}
}

func TestLookupFirmwareNotStatedAppliesToTheModel(t *testing.T) {
	set := validSet(t)
	query := Query{Driver: "example-driver", Vendor: "Example Vendor", Flavor: "example-profile", Model: "EX-1200"}
	for _, release := range []string{"", "1.0", "9.9.9"} {
		query.Firmware = release
		got := set.Lookup(query)
		if len(got) != 1 || got[0].Coverage != NotStated {
			t.Fatalf("release %q: %+v", release, got)
		}
	}
}

// Production mutations caught: a gap or unavailable entry turned into a value, dropped, or changed on the way out.
func TestLookupReturnsGapAndUnavailableUnchanged(t *testing.T) {
	set := validSet(t)
	all := set.Entries()
	for _, test := range []struct {
		model   string
		release string
		index   int
		refusal bool
	}{
		{"EX-2000", "", 4, false}, {"EX-2000", "7.1", 4, false},
		{"EX-2100", "3.1", 5, false}, {"EX-3000", "", 6, true}, {"EX-3000", "1.0", 6, true},
	} {
		got := set.Lookup(Query{Driver: "example-driver", Vendor: "Example Vendor", Flavor: "example-profile", Model: test.model, Firmware: test.release})
		if len(got) != 1 || !reflect.DeepEqual(got[0].Entry, all[test.index]) {
			t.Fatalf("%s %q: %+v", test.model, test.release, got)
		}
		if _, ok := got[0].Entry.Value(); ok || got[0].Entry.StatedValue != nil {
			t.Fatalf("%s: a value from a %s entry", test.model, got[0].Entry.Category)
		}
		if (got[0].Entry.Refusal != "") != test.refusal {
			t.Fatalf("%s: refusal %q", test.model, got[0].Entry.Refusal)
		}
	}
	if got := set.Lookup(Query{Driver: "example-driver", Vendor: "Example Vendor", Flavor: "example-profile", Model: "EX-2100", Firmware: "2.9"}); len(got) != 0 {
		t.Fatalf("a gap with a floor applied below it: %+v", got)
	}
}

// Identity fields of a lookup: driver, vendor, flavor, model (varied here) and firmware (varied in the floor and range tests).
// Production mutation per field: the lookup ignores that field.
func TestLookupEachIdentityFieldMatters(t *testing.T) {
	set := validSet(t)
	base := Query{Driver: "example-driver", Vendor: "Example Vendor", Flavor: "example-profile", Model: "EX-1000", Firmware: "1.4.0"}
	if len(set.Lookup(base)) != 2 {
		t.Fatal("base query must match")
	}
	for name, query := range map[string]Query{
		"driver": {Driver: "other-driver", Vendor: base.Vendor, Flavor: base.Flavor, Model: base.Model, Firmware: base.Firmware},
		"vendor": {Driver: base.Driver, Vendor: "Other Vendor", Flavor: base.Flavor, Model: base.Model, Firmware: base.Firmware},
		"flavor": {Driver: base.Driver, Vendor: base.Vendor, Flavor: "other-profile", Model: base.Model, Firmware: base.Firmware},
		"model":  {Driver: base.Driver, Vendor: base.Vendor, Flavor: base.Flavor, Model: "EX-9999", Firmware: base.Firmware},
		"case":   {Driver: base.Driver, Vendor: "example vendor", Flavor: base.Flavor, Model: base.Model, Firmware: base.Firmware},
	} {
		if got := set.Lookup(query); len(got) != 0 {
			t.Fatalf("%s varied: %+v", name, got)
		}
	}
}

func TestLookupReturnsCopies(t *testing.T) {
	set := validSet(t)
	query := Query{Driver: "example-driver", Vendor: "Example Vendor", Flavor: "example-profile", Model: "EX-1000", Firmware: "1.4.0"}
	first := set.Lookup(query)
	first[0].Entry.StatedValue.RatioMaximum.Coefficient = "9"
	*first[0].Entry.StatedValue.Measured = false
	again := set.Lookup(query)
	if again[0].Entry.StatedValue.RatioMaximum.Coefficient != "1" || !*again[0].Entry.StatedValue.Measured {
		t.Fatal("a returned entry changed the set")
	}
}

func TestSecondEntryOfOneCategoryForOneDeviceIsAllowed(t *testing.T) {
	if got := len(validSet(t).Lookup(Query{Driver: "example-driver", Vendor: "Example Vendor", Flavor: "example-profile", Model: "EX-1000", Firmware: "1.4.0"})); got != 2 {
		t.Fatalf("%d matches", got)
	}
}

func TestFileWithTwoCategoriesForOneDeviceFails(t *testing.T) {
	_, err := Parse(readTestdata(t, "category_conflict.json"))
	if finding, ok := err.(*Finding); !ok || finding.Reason != ReasonCategoryConflict || !finding.HasEntry || finding.Entry != 1 {
		t.Fatalf("got %v", err)
	}
}

func TestEntryWithDevicePublishedMemberFails(t *testing.T) {
	_, err := Parse(readTestdata(t, "device_published_member.json"))
	if finding, ok := err.(*Finding); !ok || finding.Reason != ReasonUnknownMember || finding.Member != "published_value" {
		t.Fatalf("got %v", err)
	}
	for _, name := range []string{"published_value", "reading", "measured_value", "device_value", "accuracy", "value"} {
		for index := range 7 {
			tree := validTree(t)
			entryAt(tree, index)[name] = "x"
			wantFinding(t, tree, ReasonUnknownMember, index, name)
		}
	}
}

// Identity of the conflict rule: driver, vendor, flavor, model, firmware. Production mutation per field: the
// conflict check ignores that field and rejects two categories that differ in it.
func TestCategoryConflictComparesEveryIdentityField(t *testing.T) {
	conflicting := func(change func(entry map[string]any)) map[string]any {
		tree := validTree(t)
		gap := map[string]any{}
		for name, value := range entryAt(tree, 4) {
			gap[name] = value
		}
		reference := entryAt(tree, 3)
		for _, name := range []string{"driver", "vendor", "flavor", "model", "firmware"} {
			gap[name] = reference[name]
		}
		change(gap)
		tree["entries"] = append(tree["entries"].([]any), gap)
		return tree
	}
	wantFinding(t, conflicting(func(map[string]any) {}), ReasonCategoryConflict, 7, "")
	for name, change := range map[string]func(map[string]any){
		"driver":   func(e map[string]any) { e["driver"] = "other-driver" },
		"vendor":   func(e map[string]any) { e["vendor"] = "Other Vendor" },
		"flavor":   func(e map[string]any) { e["flavor"] = "other-profile" },
		"model":    func(e map[string]any) { e["model"] = "EX-9999" },
		"firmware": func(e map[string]any) { e["firmware"] = map[string]any{"form": "release", "release": "1.0"} },
	} {
		data, err := json.Marshal(conflicting(change))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Parse(data); err != nil {
			t.Fatalf("%s varied: %v", name, err)
		}
	}
}

func TestNewSetRejectsCategoryConflictAcrossDocuments(t *testing.T) {
	first, err := Parse(readTestdata(t, "valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	other := Document{Entries: []Entry{first.Entries[4]}}
	other.Entries[0].Category = CategoryUnavailable
	if _, err := NewSet(first, other); err == nil {
		t.Fatal("conflict across documents accepted")
	}
}

func TestEveryMemberOfEveryEntryIsRequired(t *testing.T) {
	for index := range 7 {
		for name := range entryAt(validTree(t), index) {
			tree := validTree(t)
			delete(entryAt(tree, index), name)
			data, _ := json.Marshal(tree)
			if _, err := Parse(data); err == nil {
				t.Fatalf("entry %d accepted without %s", index, name)
			}
		}
	}
}

func TestSourceOtherThanCertificationFails(t *testing.T) {
	for index := range 7 {
		tree := validTree(t)
		entryAt(tree, index)["source"] = "device"
		wantFinding(t, tree, ReasonSourceNotCertification, index, "source")
	}
}

func TestRejectionReasons(t *testing.T) {
	type edit func(tree map[string]any)
	set := func(index int, name string, value any) edit {
		return func(tree map[string]any) { entryAt(tree, index)[name] = value }
	}
	for name, test := range map[string]struct {
		change edit
		reason Reason
		entry  int
		member string
	}{
		"gap with value":          {set(4, "stated_value", map[string]any{"measurement_type": "active_power", "measured": true}), ReasonStatedValueNotAllowed, 4, "stated_value"},
		"unavailable with value":  {set(6, "stated_value", map[string]any{"measurement_type": "active_power", "measured": true}), ReasonStatedValueNotAllowed, 6, "stated_value"},
		"refusal empty":           {set(6, "refusal", ""), ReasonRefusalMissing, 6, "refusal"},
		"refusal on reference":    {set(0, "refusal", "x"), ReasonUnknownMember, 0, "refusal"},
		"certificate on gap":      {set(4, "certificate_number", "x"), ReasonUnknownMember, 4, "certificate_number"},
		"category absent":         {func(t map[string]any) { delete(entryAt(t, 0), "category") }, ReasonMissingMember, 0, "category"},
		"category unknown":        {set(0, "category", "estimate"), ReasonInvalidValue, 0, "category"},
		"driver empty":            {set(1, "driver", ""), ReasonInvalidValue, 1, "driver"},
		"model not text":          {set(1, "model", 5), ReasonInvalidValue, 1, "model"},
		"date not calendar":       {set(0, "issue_date", "2000-02-30"), ReasonInvalidValue, 0, "issue_date"},
		"retrieval not date":      {set(0, "retrieved_on", "2000/02/01"), ReasonInvalidValue, 0, "retrieved_on"},
		"url not https":           {set(0, "url", "http://example.invalid/x"), ReasonInvalidValue, 0, "url"},
		"firmware unknown form":   {set(0, "firmware", map[string]any{"form": "ceiling"}), ReasonFirmwareInvalid, 0, "firmware"},
		"firmware extra member":   {set(0, "firmware", map[string]any{"form": "release", "release": "1.0", "to": "2.0"}), ReasonFirmwareInvalid, 0, "firmware"},
		"firmware bad release":    {set(0, "firmware", map[string]any{"form": "release", "release": "1.0-rc"}), ReasonFirmwareInvalid, 0, "firmware"},
		"range from above to":     {set(2, "firmware", map[string]any{"form": "range", "from": "3.0", "to": "2.0", "upper_bound_stated_by": "certificate"}), ReasonFirmwareInvalid, 2, "firmware"},
		"range no bound source":   {set(2, "firmware", map[string]any{"form": "range", "from": "2.0", "to": "2.5"}), ReasonFirmwareUpperBoundUnset, 2, "firmware"},
		"range bound by operator": {set(2, "firmware", map[string]any{"form": "range", "from": "2.0", "to": "2.5", "upper_bound_stated_by": "operator"}), ReasonFirmwareUpperBoundUnset, 2, "firmware"},
		"stated only type":        {set(0, "stated_value", map[string]any{"measurement_type": "active_power"}), ReasonStatedValueInvalid, 0, "stated_value"},
		"stated unknown member":   {set(0, "stated_value", map[string]any{"measurement_type": "active_power", "measured": true, "reading": true}), ReasonStatedValueInvalid, 0, "stated_value"},
		"stated null member":      {set(0, "stated_value", map[string]any{"measurement_type": "active_power", "measured": nil}), ReasonStatedValueInvalid, 0, "stated_value"},
		"stated bad type token":   {set(0, "stated_value", map[string]any{"measurement_type": "Active Power", "measured": true}), ReasonStatedValueInvalid, 0, "stated_value"},
		"decimal not canonical":   {set(0, "stated_value", map[string]any{"measurement_type": "active_power", "ratio_maximum": map[string]any{"coefficient": "10", "exponent10": -3}}), ReasonStatedValueInvalid, 0, "stated_value"},
		"decimal exponent range":  {set(0, "stated_value", map[string]any{"measurement_type": "active_power", "ratio_maximum": map[string]any{"coefficient": "1", "exponent10": 19}}), ReasonStatedValueInvalid, 0, "stated_value"},
		"decimal exponent float":  {set(0, "stated_value", map[string]any{"measurement_type": "active_power", "ratio_maximum": map[string]any{"coefficient": "1", "exponent10": 1.5}}), ReasonStatedValueInvalid, 0, "stated_value"},
		"entry not object":        {func(t map[string]any) { t["entries"] = []any{"x"} }, ReasonSchema, 0, ""},
		"contract differs":        {func(t map[string]any) { t["contract"] = "other" }, ReasonSchema, 0, ""},
		"entries empty":           {func(t map[string]any) { t["entries"] = []any{} }, ReasonSchema, 0, ""},
		"top member extra":        {func(t map[string]any) { t["note"] = "x" }, ReasonSchema, 0, ""},
		"entries not array":       {func(t map[string]any) { t["entries"] = "x" }, ReasonSchema, 0, ""},
	} {
		tree := validTree(t)
		test.change(tree)
		t.Run(name, func(t *testing.T) { wantFinding(t, tree, test.reason, test.entry, test.member) })
	}
}

func TestMalformedDocumentsFail(t *testing.T) {
	valid := readTestdata(t, "valid.json")
	for name, data := range map[string][]byte{
		"empty":     nil,
		"not json":  []byte("{"),
		"array":     []byte("[]"),
		"trailing":  append(append([]byte{}, valid...), []byte("{}")...),
		"repeated":  bytes.Replace(valid, []byte(`"driver": "example-driver"`), []byte(`"driver": "example-driver", "driver": "other"`), 1),
		"repeated0": []byte(`{"contract":"x","contract":"y","entries":[]}`),
	} {
		_, err := Parse(data)
		if finding, ok := err.(*Finding); !ok || finding.Reason != ReasonSchema {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

// Stated members of a statement-only reference: measurement_type, statement.
// Production mutation per field: the loader drops that field or replaces it with another value.
// Production mutation of the numeric members: the statement text is parsed into ratio_maximum,
// range_minimum or range_maximum.
func TestStatementOnlyReferenceLoadsAndLookupReturnsItUnchanged(t *testing.T) {
	const (
		wantType      = "active_energy"
		wantStatement = "Accuracy class designation: class B, with the standard the certificate names."
	)
	document, err := Parse(readTestdata(t, "statement.json"))
	if err != nil {
		t.Fatal(err)
	}
	set, err := NewSet(document)
	if err != nil {
		t.Fatal(err)
	}
	query := Query{Driver: "example-driver", Vendor: "Example Vendor", Flavor: "example-profile", Model: "EX-4000", Firmware: "9.9.9"}
	got := set.Lookup(query)
	if len(got) != 1 {
		t.Fatalf("%d matches", len(got))
	}
	value, ok := got[0].Entry.Value()
	if !ok || value.MeasurementType != wantType || value.Statement != wantStatement {
		t.Fatalf("stated value %+v", value)
	}
	if value.Measured != nil || value.RatioMaximum != nil || value.MeasuredMinimum != nil || value.MeasuredMaximum != nil ||
		value.RangeMinimum != nil || value.RangeMaximum != nil {
		t.Fatalf("statement-only record yielded a ratio or a range: %+v", value)
	}
	got[0].Entry.StatedValue.Statement = "rewritten"
	got[0].Entry.StatedValue.MeasurementType = "other_type"
	again := set.Lookup(query)
	if len(again) != 1 {
		t.Fatalf("%d matches after editing the returned entry", len(again))
	}
	againValue, ok := again[0].Entry.Value()
	if !ok || againValue.Statement != wantStatement || againValue.MeasurementType != wantType {
		t.Fatalf("lookup changed the statement: %+v", againValue)
	}
	for _, test := range []struct {
		measurementType string
		statement       string
	}{
		{"reactive_energy", "Accuracy class designation: class C, with the standard the certificate names."},
		{"active_energy", "Accuracy class designation: class B, with the standard the certificate names."},
	} {
		tree := map[string]any{}
		if err := json.Unmarshal(readTestdata(t, "statement.json"), &tree); err != nil {
			t.Fatal(err)
		}
		entryAt(tree, 0)["stated_value"] = map[string]any{
			"measurement_type": test.measurementType,
			"statement":        test.statement,
		}
		data, err := json.Marshal(tree)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := Parse(data)
		if err != nil {
			t.Fatalf("%s / %s: %v", test.measurementType, test.statement, err)
		}
		loaded, err := NewSet(parsed)
		if err != nil {
			t.Fatal(err)
		}
		match := loaded.Lookup(query)
		if len(match) != 1 {
			t.Fatalf("%s: %d matches", test.measurementType, len(match))
		}
		gotValue, ok := match[0].Entry.Value()
		if !ok || gotValue.MeasurementType != test.measurementType || gotValue.Statement != test.statement ||
			gotValue.RatioMaximum != nil || gotValue.RangeMinimum != nil || gotValue.RangeMaximum != nil {
			t.Fatalf("%s: %+v", test.measurementType, gotValue)
		}
	}
	tree := map[string]any{}
	if err := json.Unmarshal(readTestdata(t, "statement.json"), &tree); err != nil {
		t.Fatal(err)
	}
	stated := entryAt(tree, 0)["stated_value"].(map[string]any)
	stated["statement"] = ""
	wantFinding(t, tree, ReasonStatedValueInvalid, 0, "stated_value")
	stated["statement"] = 1
	wantFinding(t, tree, ReasonStatedValueInvalid, 0, "stated_value")
	delete(stated, "statement")
	wantFinding(t, tree, ReasonStatedValueInvalid, 0, "stated_value")
	stated["statement"] = wantStatement
	delete(stated, "measurement_type")
	wantFinding(t, tree, ReasonStatedValueInvalid, 0, "stated_value")
}

func TestCoverageOf(t *testing.T) {
	release := Firmware{Form: FirmwareRelease, Release: "1.4"}
	rng := Firmware{Form: FirmwareRange, From: "2", To: "2.5", UpperBoundStatedBy: "vendor_document"}
	for _, test := range []struct {
		firmware Firmware
		release  string
		want     Coverage
	}{
		{release, "1.3", NotCovered}, {release, "1.4.0", Covered}, {release, "1.10", Covered},
		{rng, "1.9", NotCovered}, {rng, "2.0.0", Covered}, {rng, "2.5", Covered}, {rng, "2.5.1", NotCovered},
		{Firmware{Form: FirmwareNotStated}, "1.0", NotStated}, {Firmware{Form: FirmwareNotStated}, "", NotStated},
		{release, "007.0", Covered}, {release, "99999999999999999999999.0", Covered},
	} {
		if got := CoverageOf(test.firmware, test.release); got != test.want {
			t.Fatalf("%+v %q: %s, want %s", test.firmware, test.release, got, test.want)
		}
	}
}
