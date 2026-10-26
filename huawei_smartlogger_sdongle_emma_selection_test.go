package modbusreg

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

const m86FixturePath = "testdata/huawei/selection/smartlogger_sdongle_emma_catalogue_decisions.json"

type m86Input struct {
	Family   string `json:"family"`
	Endpoint string `json:"endpoint"`
	Unit     uint8  `json:"unit"`
	Model    string `json:"model"`
	Software string `json:"software"`
}

type m86Expect struct {
	Outcome      string   `json:"outcome"`
	Reason       string   `json:"reason"`
	Grade        string   `json:"grade"`
	Floor        string   `json:"floor"`
	Domain       string   `json:"domain"`
	EditionCount int      `json:"edition_count"`
	Candidates   []string `json:"candidates"`
}

type m86Case struct {
	Name   string    `json:"name"`
	Input  m86Input  `json:"input"`
	Expect m86Expect `json:"expect"`
}

type m86Fixture struct {
	Schema     string    `json:"schema"`
	Kind       string    `json:"kind"`
	Families   []string  `json:"families"`
	License    string    `json:"license"`
	Provenance string    `json:"provenance"`
	Cases      []m86Case `json:"cases"`
}

func m86Load(t testing.TB) m86Fixture {
	t.Helper()
	data, err := os.ReadFile(m86FixturePath)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var fixture m86Fixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if decoder.More() {
		t.Fatal("trailing content")
	}
	return fixture
}

func (input m86Input) selection() HuaweiGatewaySelectionInput {
	return HuaweiGatewaySelectionInput{
		Family: input.Family, Scope: HuaweiGatewayScope{Endpoint: HuaweiGatewayEndpoint(input.Endpoint), Unit: input.Unit},
		Model: input.Model, Software: input.Software,
	}
}

func m86Minimums(floors []HuaweiGatewayFloor) []string {
	out := make([]string, 0, len(floors))
	for _, floor := range floors {
		out = append(out, floor.Minimum)
	}
	return out
}

func m86Got(decision HuaweiGatewaySelection) m86Expect {
	got := m86Expect{
		Outcome: string(decision.Outcome()), Reason: string(decision.Reason()), Grade: string(decision.Grade()),
		Candidates: m86Minimums(decision.Candidates()),
	}
	if floor, ok := decision.Floor(); ok {
		got.Floor, got.Domain, got.EditionCount = floor.Minimum, string(floor.Domain), len(floor.Editions)
	}
	return got
}

// m86SelectionRecord is the record of a selection that the dispositions and
// the observed replay results carry. The dispositions name the editions by
// their catalogue keys; the replay results, which the fixture sanitizer scans,
// count them.
func m86SelectionRecord(decision HuaweiGatewaySelection, grade HuaweiEvidenceGrade, named bool) map[string]any {
	record := map[string]any{
		"outcome": string(decision.Outcome()), "reason": string(decision.Reason()), "grade": string(grade),
		"descriptor_keys": decision.DescriptorKeys(), "attribute_6_grammars": decision.AttributeSixGrammars(),
	}
	if floor, ok := decision.Floor(); ok {
		record["floor"], record["floor_domain"] = floor.Minimum, string(floor.Domain)
		if named {
			record["editions"] = floor.EditionIDs()
		} else {
			record["edition_count"] = len(floor.Editions)
		}
	} else {
		record["candidates"] = m86Minimums(decision.Candidates())
	}
	return record
}

// m86ReplaySelection is the selection record of an observed replay result:
// the selection with its editions counted and, for a card with an observed
// floor, that floor and the relation of the software text to it.
func m86ReplaySelection(result HuaweiQualificationResult) map[string]any {
	selection, _ := result.Selection()
	record := m86SelectionRecord(selection, result.SoftwareGrade(), false)
	if floor, relation := result.ObservedFloor(); floor != "" {
		record["observed_floor"], record["observed_floor_relation"] = floor, string(relation)
	}
	return record
}

func m86Normalize(t testing.TB, value any) any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(encoded, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// m86RequireSoftwareSelection checks the software selection block of a
// disposition against the loaded catalogue: the family, the scope, every own
// floor of the family with its domain, and the outcome rules.
func m86RequireSoftwareSelection(t *testing.T, record map[string]any, family string, unit uint8) {
	t.Helper()
	block := huaweiObject(t, record["software_selection"])
	requireHuaweiKeys(t, block, "issue", "catalogue", "catalogue_files", "family", "scope", "floors", "comparison", "at_or_above_floor",
		"below_floor_in_domain", "no_documented_relation", "several_admitting_floors", "not_selected_reasons", "edition_cover_labels",
		"attribute_dictionary", "attribute_6")
	all := DecideHuaweiGatewaySelection(HuaweiGatewaySelectionInput{Family: family, Scope: HuaweiGatewayScope{Endpoint: HuaweiGatewayEndpointSelf, Unit: unit}})
	var floors []map[string]any
	for _, floor := range all.Candidates() {
		floors = append(floors, map[string]any{"minimum": floor.Minimum, "domain": string(floor.Domain)})
	}
	want := map[string]any{
		"issue": 366, "catalogue": huaweiGatewayCatalogueDirectory, "catalogue_files": []string{"editions.json", "services.json"},
		"family": family, "scope": map[string]any{"endpoint": string(HuaweiGatewayEndpointSelf), "unit": unit}, "floors": floors,
		"comparison": "WITHIN_FLOOR_DOMAIN", "at_or_above_floor": "SELECTED_HYPOTHESIS", "below_floor_in_domain": "NOT_SELECTED",
		"no_documented_relation": "ADMITTED_HYPOTHESIS", "several_admitting_floors": "AMBIGUOUS_NO_EDITION_BOUND",
		"not_selected_reasons": []string{string(HuaweiGatewayFamilyMismatch), string(HuaweiGatewayBelowFloor), string(HuaweiGatewaySoftwareMalformed), string(HuaweiGatewayCatalogueInvalid)},
		"edition_cover_labels": "IDENTIFY_EDITIONS_NOT_FLOORS", "attribute_dictionary": "SERVICES_DESCRIPTOR_ATTRIBUTES_OF_THE_ADMITTED_EDITIONS",
		"attribute_6": "EDITION_GRAMMAR_RECORDED_NEVER_DECODED",
	}
	if all.Reason() != HuaweiGatewaySoftwareNotRead || len(floors) == 0 || !reflect.DeepEqual(m86Normalize(t, block), m86Normalize(t, want)) {
		t.Fatalf("%s software selection=%#v want=%#v", family, block, want)
	}
}

// m86RequireObservedSelection checks the selection record of an observed card
// in its disposition against the card's own result for the fixture tuple.
func m86RequireObservedSelection(t *testing.T, value any, result HuaweiQualificationResult, fields map[string]any) {
	t.Helper()
	block := huaweiObject(t, value)
	selection, ok := result.Selection()
	if !ok || !result.IdentityMatched() {
		t.Fatalf("observed card has no selection: %#v", result)
	}
	want := map[string]any{"record": m86SelectionRecord(selection, result.SoftwareGrade(), true)}
	if floor, relation := result.ObservedFloor(); floor != "" {
		want["record"].(map[string]any)["observed_floor_relation"] = string(relation)
	}
	for key, field := range fields {
		want[key] = field
	}
	if !reflect.DeepEqual(m86Normalize(t, block), m86Normalize(t, want)) {
		t.Fatalf("observed selection=%#v want=%#v", block, want)
	}
}

func TestM86DecisionFixtureIsSanitizedAndCoversEveryFamily(t *testing.T) {
	fixture := m86Load(t)
	if fixture.Schema != "helianthus-huawei-catalogue-selection-fixture/v1" || fixture.Kind != "gateway_catalogue_decision_table" ||
		fixture.License != "CC0-1.0" || !reflect.DeepEqual(fixture.Families, []string{HuaweiSmartLoggerCanonicalClass, HuaweiSDongleCanonicalClass, HuaweiEMMACanonicalClass}) {
		t.Fatalf("fixture header: %+v", fixture)
	}
	data, err := os.ReadFile(m86FixturePath)
	if err != nil {
		t.Fatal(err)
	}
	if h03Digestlike.Match(data) || bytes.Contains(data, []byte("://")) {
		t.Fatal("the fixture carries a digest or a link")
	}
	outcomes := map[string]map[string]bool{}
	names := map[string]bool{}
	for _, tc := range fixture.Cases {
		if names[tc.Name] {
			t.Fatalf("case %s repeated", tc.Name)
		}
		names[tc.Name] = true
		if outcomes[tc.Input.Family] == nil {
			outcomes[tc.Input.Family] = map[string]bool{}
		}
		outcomes[tc.Input.Family][tc.Expect.Outcome+"/"+tc.Expect.Reason] = true
	}
	for _, family := range fixture.Families {
		for _, want := range []string{"SELECTED/at_or_above_floor", "NOT_SELECTED/below_floor", "AMBIGUOUS/no_documented_branch_relation", "NOT_SELECTED/family_mismatch"} {
			if !outcomes[family][want] {
				t.Fatalf("%s has no %s case", family, want)
			}
		}
	}
}

func TestM86DecisionTableFixtureCases(t *testing.T) {
	for _, tc := range m86Load(t).Cases {
		t.Run(tc.Name, func(t *testing.T) {
			decision := DecideHuaweiGatewaySelection(tc.Input.selection())
			got := m86Got(decision)
			want := tc.Expect
			if want.Candidates == nil {
				want.Candidates = []string{}
			}
			if got.Candidates == nil {
				got.Candidates = []string{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("decision=%+v\nwant=%+v", got, want)
			}
			if decision.Admitted() != (want.Outcome != string(HuaweiGatewayNotSelected)) || decision.LiveQualified() || decision.SupportClaim() || decision.WriteAuthority() {
				t.Fatalf("admission or authority: %+v", decision)
			}
			if decision.Family() != tc.Input.Family || decision.Software() != tc.Input.Software || decision.Model() != tc.Input.Model ||
				decision.Scope() != (HuaweiGatewayScope{Endpoint: HuaweiGatewayEndpoint(tc.Input.Endpoint), Unit: tc.Input.Unit}) {
				t.Fatalf("the decision does not carry its inputs: %+v", decision)
			}
			if want.Outcome == string(HuaweiGatewayNotSelected) && (len(decision.DescriptorKeys()) != 0 || len(decision.AttributeSixGrammars()) != 0) {
				t.Fatal("a refused decision carries a dictionary")
			}
		})
	}
}

// TestM86FloorRelationVariesEveryVersionPart varies the line, release, branch
// and build number of a software text around each family's floor and names
// the production mutation each row catches.
func TestM86FloorRelationVariesEveryVersionPart(t *testing.T) {
	type row struct {
		family, software string
		want             HuaweiGatewaySelectionReason
		catches          string
	}
	rows := []row{
		{HuaweiSmartLoggerCanonicalClass, "V300R001C00", HuaweiGatewayAtOrAboveFloor, "a strict comparison at the floor"},
		{HuaweiSmartLoggerCanonicalClass, "V300R000C99SPC999", HuaweiGatewayBelowFloor, "comparing the branch before the release"},
		{HuaweiSmartLoggerCanonicalClass, "V300R002C00", HuaweiGatewayAtOrAboveFloor, "a release ceiling"},
		{HuaweiSmartLoggerCanonicalClass, "V300R001C01", HuaweiGatewayAtOrAboveFloor, "a branch ceiling within the floor release"},
		{HuaweiSmartLoggerCanonicalClass, "V300R001C00SPC001", HuaweiGatewayAtOrAboveFloor, "refusing a build number the floor does not name"},
		{HuaweiSmartLoggerCanonicalClass, "V301R001C00", HuaweiGatewayNoDocumentedRelation, "ordering two lines"},
		{HuaweiSmartLoggerCanonicalClass, "V299R999C99SPC999", HuaweiGatewayNoDocumentedRelation, "ordering two lines downwards"},
		{HuaweiSmartLoggerCanonicalClass, "V100R002C00SPC001", HuaweiGatewayAtOrAboveFloor, "ignoring the floors of the other lines"},
		{HuaweiSmartLoggerCanonicalClass, "V100R003C00", HuaweiGatewayNoDocumentedRelation, "ordering the releases of a line that no edition orders"},
		{HuaweiEMMACanonicalClass, "SmartHEMS V100R024C00SPC100", HuaweiGatewayAtOrAboveFloor, "a strict comparison at the floor"},
		{HuaweiEMMACanonicalClass, "SmartHEMS V100R024C00SPC099", HuaweiGatewayBelowFloor, "admitting a build below its own branch floor"},
		{HuaweiEMMACanonicalClass, "SmartHEMS V100R025C00SPC101", HuaweiGatewayBelowFloor, "comparing an R025 build with the R024 floor"},
		{HuaweiEMMACanonicalClass, "SmartHEMS V100R025C01SPC001", HuaweiGatewayNoDocumentedRelation, "ordering two branches of one release"},
		{HuaweiEMMACanonicalClass, "SmartHEMS V100R023C00SPC999", HuaweiGatewayNoDocumentedRelation, "ordering the EMMA releases, which no edition orders"},
		{HuaweiEMMACanonicalClass, "SmartHEMS V101R025C00SPC131", HuaweiGatewayNoDocumentedRelation, "ordering two lines"},
		{HuaweiSDongleCanonicalClass, "V200R022C10", HuaweiGatewayAtOrAboveFloor, "a strict comparison at the floor"},
		{HuaweiSDongleCanonicalClass, "V200R022C09SPC999", HuaweiGatewayBelowFloor, "comparing the build number before the branch"},
		{HuaweiSDongleCanonicalClass, "V200R021C99", HuaweiGatewayBelowFloor, "comparing the branch before the release"},
		{HuaweiSDongleCanonicalClass, "V200R023C00", HuaweiGatewayAtOrAboveFloor, "a release ceiling in the ordered line"},
		{HuaweiSDongleCanonicalClass, "V100R001C00SPC100", HuaweiGatewayAtOrAboveFloor, "ignoring the branch floor of the other line"},
		{HuaweiSDongleCanonicalClass, "V100R001C00", HuaweiGatewayBelowFloor, "treating the base of a branch as above its build floor"},
		{HuaweiSDongleCanonicalClass, "V100R002C00SPC100", HuaweiGatewayNoDocumentedRelation, "ordering the releases of a line that no edition orders"},
	}
	units := map[string]uint8{HuaweiSmartLoggerCanonicalClass: 0, HuaweiEMMACanonicalClass: 0, HuaweiSDongleCanonicalClass: 100}
	for _, tc := range rows {
		decision := DecideHuaweiGatewaySelection(HuaweiGatewaySelectionInput{
			Family: tc.family, Scope: HuaweiGatewayScope{Endpoint: HuaweiGatewayEndpointSelf, Unit: units[tc.family]}, Software: tc.software,
		})
		if decision.Reason() != tc.want || tc.catches == "" {
			t.Fatalf("%s %s: reason %s want %s (catches %s)", tc.family, tc.software, decision.Reason(), tc.want, tc.catches)
		}
		if (tc.want == HuaweiGatewayBelowFloor) == decision.Admitted() {
			t.Fatalf("%s %s: admitted=%t", tc.family, tc.software, decision.Admitted())
		}
	}
}

func TestM86LinesAreOrderedOnlyWhereAnEditionDocumentsTheOrder(t *testing.T) {
	catalogue := huaweiGatewayLoadedCatalogue()
	if catalogue.err != nil {
		t.Fatal(catalogue.err)
	}
	want := map[string]map[int]bool{
		HuaweiSmartLoggerCanonicalClass: {300: true},
		HuaweiSDongleCanonicalClass:     {200: true},
		HuaweiEMMACanonicalClass:        {},
	}
	for family, lines := range want {
		got := catalogue.families[family].orderedLines
		if !reflect.DeepEqual(got, lines) {
			t.Fatalf("%s ordered lines %v want %v", family, got, lines)
		}
	}
	if !catalogue.families[HuaweiEMMACanonicalClass].buildRequired || catalogue.families[HuaweiSmartLoggerCanonicalClass].buildRequired ||
		catalogue.families[HuaweiSDongleCanonicalClass].buildRequired {
		t.Fatal("the build-number requirement does not follow the floors")
	}
	if catalogue.families[HuaweiSmartLoggerCanonicalClass].selfUnit != 0 || catalogue.families[HuaweiEMMACanonicalClass].selfUnit != 0 ||
		catalogue.families[HuaweiSDongleCanonicalClass].selfUnit != 100 {
		t.Fatal("self units do not follow the catalogue")
	}
}

// TestM86EditionOrderNeverChoosesAWinner reverses and rotates the editions of
// each family: the decision of every fixture case stays the same.
func TestM86EditionOrderNeverChoosesAWinner(t *testing.T) {
	loaded := huaweiGatewayLoadedCatalogue()
	for _, tc := range m86Load(t).Cases {
		reference := m86Got(decideHuaweiGatewaySelection(loaded, tc.Input.selection()))
		for _, permute := range []func([]huaweiGatewayFamilyEdition) []huaweiGatewayFamilyEdition{
			func(in []huaweiGatewayFamilyEdition) []huaweiGatewayFamilyEdition {
				out := append([]huaweiGatewayFamilyEdition(nil), in...)
				for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
					out[i], out[j] = out[j], out[i]
				}
				return out
			},
			func(in []huaweiGatewayFamilyEdition) []huaweiGatewayFamilyEdition {
				return append(append([]huaweiGatewayFamilyEdition(nil), in[1:]...), in[0])
			},
		} {
			permuted := huaweiGatewayCatalogue{families: map[string]huaweiGatewayFamily{}}
			for name, family := range loaded.families {
				family.editions = permute(family.editions)
				permuted.families[name] = family
			}
			if got := m86Got(decideHuaweiGatewaySelection(permuted, tc.Input.selection())); !reflect.DeepEqual(got, reference) {
				t.Fatalf("%s: edition order changed the decision: %+v vs %+v", tc.Name, got, reference)
			}
		}
	}
}

// TestM86CatalogueLoaderFailsClosed mutates the two catalogue files the
// selection reads: every malformed binding refuses the catalogue, and a
// refused catalogue selects nothing.
func TestM86CatalogueLoaderFailsClosed(t *testing.T) {
	read := func(name string) ([]byte, error) {
		return huaweiGatewayCatalogueFiles.ReadFile(huaweiGatewayCatalogueDirectory + "/" + name)
	}
	if _, err := loadHuaweiGatewayCatalogue(read); err != nil {
		t.Fatalf("committed catalogue: %v", err)
	}
	mutated := func(file, old, replacement string) func(string) ([]byte, error) {
		return func(name string) ([]byte, error) {
			data, err := read(name)
			if err != nil || name != file {
				return data, err
			}
			if !bytes.Contains(data, []byte(old)) {
				return nil, fmt.Errorf("%s does not contain %q", file, old)
			}
			return bytes.Replace(data, []byte(old), []byte(replacement), 1), nil
		}
	}
	for name, source := range map[string]func(string) ([]byte, error){
		"floor that is not inclusive":   mutated("editions.json", `"minimum":"V200R022C10","inclusive":true`, `"minimum":"V200R022C10","inclusive":false`),
		"floor outside the form":        mutated("editions.json", `"minimum":"V100R025C00SPC102"`, `"minimum":"V100R025C00SPC10"`),
		"edition without its own floor": mutated("editions.json", `"component":"SmartHEMS","minimum":"V100R024C00SPC100"`, `"component":"SmartHEM","minimum":"V100R024C00SPC100"`),
		"self unit disagrees":           mutated("editions.json", `"self_unit":100`, `"self_unit":101`),
		"services entry missing":        mutated("services.json", `"edition": "emma-v100r024c00-01-2024-07-15"`, `"edition": "emma-v100r024c00-01-2024-07-16"`),
		"descriptor list malformed":     mutated("services.json", `"descriptor_attributes": "1–6"`, `"descriptor_attributes": "6–1"`),
		"descriptor key with a zero":    mutated("services.json", `"descriptor_attributes": "1–6"`, `"descriptor_attributes": "0–6"`),
		"attribute 6 grammar missing":   mutated("services.json", `"attribute_6": "parallel-network integer"`, `"attribute_6": ""`),
		"trailing content": func(name string) ([]byte, error) {
			data, err := read(name)
			if name == "services.json" {
				data = append(append([]byte(nil), data...), []byte("{}")...)
			}
			return data, err
		},
		"file missing": func(name string) ([]byte, error) {
			if name == "services.json" {
				return nil, errors.New("missing")
			}
			return read(name)
		},
	} {
		families, err := loadHuaweiGatewayCatalogue(source)
		if err == nil || families != nil {
			t.Fatalf("%s: catalogue accepted", name)
		}
		decision := decideHuaweiGatewaySelection(huaweiGatewayCatalogue{err: err}, HuaweiGatewaySelectionInput{
			Family: HuaweiSmartLoggerCanonicalClass, Scope: HuaweiGatewayScope{Endpoint: HuaweiGatewayEndpointSelf}, Software: "V300R023C10SPC620",
		})
		if decision.Admitted() || decision.Reason() != HuaweiGatewayCatalogueInvalid {
			t.Fatalf("%s: a refused catalogue selected %+v", name, decision)
		}
	}
}

func TestM86DescriptorDictionaryComesFromTheServicesPerEdition(t *testing.T) {
	for text, want := range map[string][]int{
		"1–6, 8–10; 7 N/A":                    {1, 2, 3, 4, 5, 6, 8, 9, 10},
		"1–6, 8; 7 undefined; 9 example-only": {1, 2, 3, 4, 5, 6, 8},
		"1–6":                                 {1, 2, 3, 4, 5, 6},
	} {
		got, ok := parseHuaweiDescriptorKeys(text)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("%q: %v %t", text, got, ok)
		}
	}
	catalogue := huaweiGatewayLoadedCatalogue()
	keys := map[string][]int{}
	grammars := map[string]string{}
	for _, family := range catalogue.families {
		for _, edition := range family.editions {
			keys[edition.edition.ID], grammars[edition.edition.ID] = edition.edition.DescriptorKeys, edition.edition.AttributeSix
		}
	}
	// Issue 52 defines keys 1 to 6 and 8 to 10; Issue 49 defines 1 to 6 with
	// attribute 6 a parallel-network integer; the EMMA editions call it a
	// feature-version string.
	if !reflect.DeepEqual(keys["smartlogger-v300r024c10spc191-52-2025-06-06"], []int{1, 2, 3, 4, 5, 6, 8, 9, 10}) ||
		!reflect.DeepEqual(keys["smartlogger-v300r024c10spc210-49-2025-03-19"], []int{1, 2, 3, 4, 5, 6}) ||
		grammars["smartlogger-v300r024c10spc210-49-2025-03-19"] != "parallel-network integer" ||
		grammars["smartlogger-v300r024c10spc191-52-2025-06-06"] != "feature-version integer" ||
		grammars["emma-v100r025c00spc102-2025-08-14"] != "feature-version string" || len(keys) != 18 {
		t.Fatalf("descriptor dictionary: %v %v", keys, grammars)
	}
}

// TestM86DocumentarySmartLoggerAdmitsAboveTheFloorAndRetainsKeyTen covers the
// documentary card: a build above the floor and a key that only some
// editions define were refused while the card compared exact tuples and a
// fixed key set.
func TestM86DocumentarySmartLoggerAdmitsAboveTheFloorAndRetainsKeyTen(t *testing.T) {
	card := NewHuaweiSmartLoggerQualificationCard()
	for _, firmware := range []string{"V300R024C10SPC999", "V300R023C10SPC620", "V300R026C10", "V300R001C00", "V400R001C00"} {
		input := validHuaweiSmartLoggerQualificationInput()
		input.Inventory.Pages[0].Objects[1].Value = []byte("1=SmartLogger;2=" + firmware + ";5=0;10=XXXX0XXX;9=0")
		input.Inventory.Pages[0].Objects[2].Value = []byte("1=SUN2000;2=V200R023C00SPC100;3=V1;4=synthetic-private-input;5=1;6=F1;8=PV;9=0;10=ABCD;11=extra")
		result := card.Evaluate(input)
		selection, ok := result.Selection()
		if !result.IdentityMatched() || !result.QualificationTestReady() || !ok || result.SoftwareGrade() != HuaweiGradeHypothesis || selection.Build() != firmware {
			t.Fatalf("%s: %#v", firmware, result)
		}
		inventory, _ := result.Inventory()
		child := inventory.Children()[0]
		if value, ok := child.OpaqueAttribute("10"); !ok || value != "ABCD" {
			t.Fatalf("%s: key 10 not retained: %v", firmware, child.OpaqueAttributes())
		}
		if value, ok := child.OpaqueAttribute("6"); !ok || value != "F1" || child.Attribute("feature_version") != "" {
			t.Fatalf("%s: key 6 decoded instead of retained", firmware)
		}
		if _, ok := child.OpaqueAttribute("4"); ok || child.Attribute("product_type") != "PV" {
			t.Fatalf("%s: key 4 retained or key 8 lost", firmware)
		}
		if !reflect.DeepEqual(child.OpaqueAttributes(), []HuaweiGatewayOpaqueAttribute{{Key: "6", Value: "F1"}, {Key: "9", Value: "0"}, {Key: "10", Value: "ABCD"}, {Key: "11", Value: "extra"}}) {
			t.Fatalf("%s: opaque attributes %v", firmware, child.OpaqueAttributes())
		}
		if firmware != "V400R001C00" {
			if !selection.DefinesAttribute("10") || !selection.DefinesAttribute("9") || selection.DefinesAttribute("11") || selection.DefinesAttribute("7") ||
				!reflect.DeepEqual(selection.AttributeSixGrammars(), []string{"feature-version integer", "parallel-network integer"}) {
				t.Fatalf("%s: dictionary %v %v", firmware, selection.DescriptorKeys(), selection.AttributeSixGrammars())
			}
		}
	}
	for _, firmware := range []string{"V300R000C10", "V300R023C10SPC62", "SmartHEMS V100R025C00SPC131", ""} {
		input := validHuaweiSmartLoggerQualificationInput()
		input.Inventory.Pages[0].Objects[1].Value = []byte("1=SmartLogger;2=" + firmware + ";5=0")
		if firmware == "" {
			input.Inventory.Pages[0].Objects[1].Value = []byte("1=SmartLogger;5=0")
		}
		result := card.Evaluate(input)
		assertHuaweiQualificationFailureHasNoFallback(t, result)
		if inventory, ok := result.Inventory(); ok || inventory.ChildCount() != 0 {
			t.Fatalf("%s: a refused result retained children", firmware)
		}
	}
}

// TestM86OwnedGatewaysStillAdmitWithTheirSelectionRecorded replays the three
// owned gateways' observed fixtures: each still admits, and its observation
// output records the selection.
func TestM86OwnedGatewaysStillAdmitWithTheirSelectionRecorded(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result HuaweiQualificationResult
		grade  HuaweiEvidenceGrade
		floor  string
		reason HuaweiGatewaySelectionReason
	}{
		{"SmartLogger", NewHuaweiSmartLoggerObservedQualificationCard().Evaluate(h03ValidInput(t)), HuaweiGradeProven, "V300R001C00", HuaweiGatewayAtOrAboveFloor},
		{"EMMA", NewHuaweiEMMAObservedQualificationCard().Evaluate(h18ValidInput(t)), HuaweiGradeProven, "V100R025C00SPC102", HuaweiGatewayAtOrAboveFloor},
		{"S-Dongle", NewHuaweiSDongleObservedQualificationCard().Evaluate(h15ValidInput(t)), HuaweiGradeUnknown, "", HuaweiGatewaySoftwareNotRead},
	} {
		selection, ok := tc.result.Selection()
		floor, _ := selection.Floor()
		if !tc.result.IdentityMatched() || !tc.result.QualificationTestReady() || !ok || tc.result.SoftwareGrade() != tc.grade ||
			floor.Minimum != tc.floor || selection.Reason() != tc.reason || !selection.Admitted() {
			t.Fatalf("%s: matched=%t selection=%+v grade=%s", tc.name, tc.result.IdentityMatched(), selection, tc.result.SoftwareGrade())
		}
	}
}

// TestM86SelectionIsAbsentOrRefusingOnEveryRejectedCard keeps a rejected card
// free of an admitting selection, so no consumer reads an admission from a
// refused identity.
func TestM86SelectionIsAbsentOrRefusingOnEveryRejectedCard(t *testing.T) {
	below := h03MutatedInventory(t, h03ReplaceInSelfEntry("2="+h03Firmware+";", "2=V300R023C10SPC619;"))
	result := NewHuaweiSmartLoggerObservedQualificationCard().Evaluate(below)
	h03RequireRejected(t, "below the observed floor", result)
	if floor, relation := result.ObservedFloor(); floor != h03Firmware || relation != HuaweiGatewayRelationBelow || result.SoftwareGrade() != HuaweiGradeUnknown {
		t.Fatalf("below the observed floor: %q %s %s", floor, relation, result.SoftwareGrade())
	}
	catalogueBelow := h03MutatedInventory(t, h03ReplaceInSelfEntry("2="+h03Firmware+";", "2=V300R000C10;"))
	result = NewHuaweiSmartLoggerObservedQualificationCard().Evaluate(catalogueBelow)
	if selection, ok := result.Selection(); result.IdentityMatched() || !ok || selection.Outcome() != HuaweiGatewayNotSelected || selection.Reason() != HuaweiGatewayBelowFloor {
		t.Fatalf("below the catalogue floor: %#v", result)
	}
	emma := NewHuaweiEMMAQualificationCard().Evaluate(HuaweiEMMAQualificationInput{UnitID: 0, Offering: "SmartHEMS", Model: "EMMA-A02", Firmware: "SmartHEMS V100R025C00SPC101"})
	if selection, ok := emma.Selection(); emma.IdentityMatched() || !ok || selection.Reason() != HuaweiGatewayBelowFloor || emma.SoftwareGrade() != HuaweiGradeUnknown {
		t.Fatalf("EMMA below its floor: %#v", emma)
	}
	input := h15ValidInput(t)
	input.Gateway.Software = h18Block("V200R021C00SPC100", 30)
	dongle := NewHuaweiSDongleObservedQualificationCard().Evaluate(input)
	if selection, ok := dongle.Selection(); dongle.IdentityMatched() || !ok || selection.Reason() != HuaweiGatewayBelowFloor {
		t.Fatalf("S-Dongle below its floor: %#v", dongle)
	}
}

// TestM86ObservedReplayResultsRecordTheSelection compares the selection
// recorded in each observed replay result with the card's own result.
func TestM86ObservedReplayResultsRecordTheSelection(t *testing.T) {
	for path, result := range map[string]HuaweiQualificationResult{
		"testdata/huawei/qualification/smartlogger_observed_replay_expected.json": NewHuaweiSmartLoggerObservedQualificationCard().Evaluate(h03ValidInput(t)),
		"testdata/huawei/qualification/emma_observed_replay_expected.json":        NewHuaweiEMMAObservedQualificationCard().Evaluate(h18ValidInput(t)),
		"testdata/huawei/qualification/sdongle_observed_replay_expected.json":     NewHuaweiSDongleObservedQualificationCard().Evaluate(h15ValidInput(t)),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]any
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatal(err)
		}
		want := m86ReplaySelection(result)
		if !reflect.DeepEqual(m86Normalize(t, document["catalogue_selection"]), m86Normalize(t, want)) {
			t.Fatalf("%s: catalogue_selection=%v want=%v", path, document["catalogue_selection"], want)
		}
	}
}

// TestM86NoExactSoftwareComparisonRemains scans the gateway cards for the
// retired exact-tuple vocabulary and for a software text compared with a
// literal build.
func TestM86NoExactSoftwareComparisonRemains(t *testing.T) {
	files := []string{
		"huawei_qualification_cards.go", "huawei_smartlogger_observed.go", "huawei_emma.go", "huawei_emma_observed.go",
		"huawei_sdongle_observation.go", "huawei_sdongle_observed.go", "huawei_smartlogger_inventory.go", "huawei_emma_inventory.go",
		"profiles/vendor/huawei/smartlogger-disposition.json", "profiles/vendor/huawei/emma-disposition.json", "profiles/vendor/huawei/sdongle-disposition.json",
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, retired := range []string{"EXACT_TUPLE_ONLY", "FirmwareTuples", "firmware_gates", "isHuaweiSDongleFirmwareTuple", "matchesHuaweiEMMAFirmwareFloor",
			"huaweiEMMAObservedBranchPrefix", "NO_RANGE_PREFIX_ORDER_OR_INHERITANCE", `"V200R025C00SPC120"`, `"V200R022C10SPC312"`, `"V300R024C10SPC191"`, `"V300R024C10SPC210"`} {
			if bytes.Contains(data, []byte(retired)) {
				t.Fatalf("%s still carries %s", file, retired)
			}
		}
	}
	// The observed builds stay as data: the floor of the observed logger and
	// the grading build of the observed EMMA, never compared for equality as a
	// gate, and no software text is compared with a literal build or prefix.
	// Agreement between two readings of one build (the self entry and the
	// software block, or the basic reply and the self entry) is not a gate on
	// the build and stays.
	for _, file := range files[:8] {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			for _, pattern := range []string{"!= card.Firmware()", "== card.Firmware()", "HasPrefix(token", "HasPrefix(firmware", `!= "V`, `== "V`, `firmware == "`, `HasPrefix(software`} {
				if strings.Contains(line, pattern) {
					t.Fatalf("%s compares a software text exactly: %s", file, strings.TrimSpace(line))
				}
			}
		}
	}
}

func TestM86OpaqueAttributesKeepTheChildComparable(t *testing.T) {
	var child HuaweiGatewayInventoryChild
	child.retainOpaque("9", "0")
	child.retainOpaque("10", "XXXX0XXX")
	same := child
	if same != child || !reflect.DeepEqual(child.OpaqueAttributes(), []HuaweiGatewayOpaqueAttribute{{Key: "9", Value: "0"}, {Key: "10", Value: "XXXX0XXX"}}) {
		t.Fatalf("opaque attributes %v", child.OpaqueAttributes())
	}
	attributes := child.OpaqueAttributes()
	attributes[0].Value = "changed"
	if value, _ := child.OpaqueAttribute("9"); value != "0" {
		t.Fatal("OpaqueAttributes exposes the child's own storage")
	}
	// Entry order is kept: key 10 follows key 9 here, as it was retained.
	keys := []string{}
	for _, attribute := range child.OpaqueAttributes() {
		keys = append(keys, attribute.Key)
	}
	if !reflect.DeepEqual(keys, []string{"9", "10"}) {
		t.Fatalf("unexpected key order %v", keys)
	}
	var empty HuaweiGatewayInventoryChild
	if empty.OpaqueAttributes() != nil {
		t.Fatal("a child without opaque attributes lists some")
	}
}

func TestHuaweiGatewayModelWithCatalogueParentheticalRow(t *testing.T) {
	selection := DecideHuaweiGatewaySelection(HuaweiGatewaySelectionInput{
		Family:   "SmartLogger",
		Scope:    HuaweiGatewayScope{Endpoint: HuaweiGatewayEndpointSelf, Unit: 0},
		Model:    "SmartLogger-3000A",
		Software: "V300R023C10SPC620",
	})
	if !selection.Admitted() {
		t.Fatalf("model of the family refused: %v", selection.Reason())
	}
	if huaweiGatewayModelInRows("SmartLogger", []string{"SmartLogger (generic source scope)"}, false) {
		t.Fatal("the family name alone was held as a model")
	}
}
