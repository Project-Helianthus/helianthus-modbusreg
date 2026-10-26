package modbusreg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

const m85FixturePath = "testdata/huawei/selection/sun2000_catalogue_decisions.json"

type m85Row struct {
	Card       string `json:"card"`
	Identifier int    `json:"identifier"`
}

type m85Input struct {
	Scope         string   `json:"scope"`
	ModelText     string   `json:"model_text"`
	Identifier    *uint16  `json:"identifier"`
	SoftwareText  string   `json:"software_text"`
	FirmwareText  string   `json:"firmware_text"`
	ProtocolWords []uint16 `json:"protocol_words"`
}

type m85Expect struct {
	Outcome    string   `json:"outcome"`
	Reason     string   `json:"reason"`
	Grade      string   `json:"grade"`
	Card       string   `json:"card"`
	MatchedBy  string   `json:"matched_by"`
	Candidates []m85Row `json:"candidates"`
}

type m85DecisionCase struct {
	Name   string    `json:"name"`
	Input  m85Input  `json:"input"`
	Expect m85Expect `json:"expect"`
}

type m85Family struct {
	Family      string `json:"family"`
	GatewayPath string `json:"gateway_path"`
	Case        string `json:"case"`
	Card        string `json:"card"`
}

type m85Fixture struct {
	Schema        string            `json:"schema"`
	Kind          string            `json:"kind"`
	Family        string            `json:"family"`
	License       string            `json:"license"`
	Provenance    string            `json:"provenance"`
	Qualification []m85Family       `json:"qualification_families"`
	Cases         []m85DecisionCase `json:"cases"`
}

func m85Load(t testing.TB) (m85Fixture, []byte) {
	t.Helper()
	data, err := os.ReadFile(m85FixturePath)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var fixture m85Fixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if decoder.More() {
		t.Fatal("trailing content")
	}
	return fixture, data
}

func m85Case(t testing.TB, name string) m85DecisionCase {
	t.Helper()
	fixture, _ := m85Load(t)
	for _, tc := range fixture.Cases {
		if tc.Name == name {
			return tc
		}
	}
	t.Fatalf("case %s not found", name)
	return m85DecisionCase{}
}

func (input m85Input) catalogueInput() HuaweiSUN2000CatalogueInput {
	out := HuaweiSUN2000CatalogueInput{
		Scope: HuaweiSUN2000EndpointScope(input.Scope), ModelText: input.ModelText, SoftwareText: input.SoftwareText,
		FirmwareText: input.FirmwareText, ProtocolWords: input.ProtocolWords,
	}
	if input.Identifier != nil {
		identifier := *input.Identifier
		out.ModelIdentifier = &identifier
	}
	return out
}

func m85Identifier(value uint16) *uint16 { return &value }

func m85Rows(rows []HuaweiSUN2000CatalogueRow) []m85Row {
	out := make([]m85Row, 0, len(rows))
	for _, row := range rows {
		out = append(out, m85Row{Card: row.Card, Identifier: row.Identifier})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Card != out[j].Card {
			return out[i].Card < out[j].Card
		}
		return out[i].Identifier < out[j].Identifier
	})
	return out
}

// m85CatalogueRow is a model row of a card edition read straight from the
// committed catalogue files, independently of the production loader.
type m85CatalogueRow struct {
	Card, Edition, Title, Issue, Date, Token, Floor, FloorBasis, Requirement string
	Page, Identifier                                                         int
	Quarantine                                                               []string
}

var (
	m85CatalogueRowsOnce  sync.Once
	m85CatalogueRowsCache []m85CatalogueRow
	m85CatalogueRowsErr   error
)

func m85CatalogueRows(t testing.TB) []m85CatalogueRow {
	t.Helper()
	m85CatalogueRowsOnce.Do(func() { m85CatalogueRowsCache, m85CatalogueRowsErr = m85ReadCatalogueRows() })
	if m85CatalogueRowsErr != nil {
		t.Fatal(m85CatalogueRowsErr)
	}
	return m85CatalogueRowsCache
}

func m85ReadCatalogueRows() ([]m85CatalogueRow, error) {
	var editions struct {
		Editions []map[string]any `json:"editions"`
	}
	var models struct {
		Rows []map[string]any `json:"rows"`
	}
	for name, target := range map[string]any{"editions.json": &editions, "models.json": &models} {
		data, err := os.ReadFile(filepath.Join(huaweiSUN2000CatalogueDirectory, name))
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, target); err != nil {
			return nil, err
		}
	}
	byEdition := map[string]map[string]any{}
	for _, edition := range editions.Editions {
		if card, _ := edition["card"].(string); card != "" {
			byEdition[edition["id"].(string)] = edition
		}
	}
	var rows []m85CatalogueRow
	for _, row := range models.Rows {
		edition, ok := byEdition[row["edition"].(string)]
		if !ok {
			continue
		}
		out := m85CatalogueRow{
			Card: edition["card"].(string), Edition: edition["id"].(string), Title: edition["title"].(string), Issue: edition["issue"].(string),
			Date: edition["date"].(string), Token: row["token"].(string), Floor: row["minimum_software"].(string), FloorBasis: row["floor_basis"].(string),
			Page: int(row["page"].(float64)), Identifier: -1,
		}
		if identifier, ok := row["identifier"].(float64); ok {
			out.Identifier = int(identifier)
		}
		if requirement, ok := row["document_requirement"].(string); ok {
			out.Requirement = requirement
		}
		if flags, ok := row["quarantine"].([]any); ok {
			for _, flag := range flags {
				out.Quarantine = append(out.Quarantine, flag.(string))
			}
		}
		rows = append(rows, out)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no card rows")
	}
	return rows, nil
}

func m85CatalogueRowOf(t testing.TB, card string, identifier int) m85CatalogueRow {
	t.Helper()
	var found []m85CatalogueRow
	for _, row := range m85CatalogueRows(t) {
		if row.Card == card && row.Identifier == identifier {
			found = append(found, row)
		}
	}
	if len(found) != 1 {
		t.Fatalf("card %s identifier %d: %d rows", card, identifier, len(found))
	}
	return found[0]
}

// m85RequireRowFacts checks a decided row against the committed catalogue:
// edition identity, page, token, floor and floor basis.
func m85RequireRowFacts(t testing.TB, row HuaweiSUN2000CatalogueRow) {
	t.Helper()
	want := m85CatalogueRowOf(t, row.Card, row.Identifier)
	if row.Edition != (HuaweiSUN2000CatalogueEdition{ID: want.Edition, Title: want.Title, Issue: want.Issue, Date: want.Date}) ||
		row.Page != want.Page || row.Token != want.Token || row.Floor != want.Floor || row.FloorBasis != want.FloorBasis ||
		row.DocumentRequirement != want.Requirement || !reflect.DeepEqual(row.Quarantine, want.Quarantine) || !row.HasIdentifier {
		t.Fatalf("row %+v disagrees with the catalogue row %+v", row, want)
	}
}

func m85Decide(t testing.TB, input HuaweiSUN2000CatalogueInput) HuaweiSUN2000CatalogueDecision {
	t.Helper()
	decision := DecideHuaweiSUN2000CatalogueSelection(input)
	if decision.LiveQualified() || decision.SupportClaim() || decision.WriteAuthority() {
		t.Fatal("a catalogue decision grants authority")
	}
	return decision
}

func TestM85DecisionFixtureRoundTripsAndIsSanitized(t *testing.T) {
	fixture, data := m85Load(t)
	encoded, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(append(encoded, '\n'), data) {
		t.Fatal("fixture does not round-trip byte for byte")
	}
	if fixture.Schema != "helianthus-huawei-catalogue-selection-fixture/v1" || fixture.Kind != "catalogue_decision_table" ||
		fixture.Family != "SUN2000" || fixture.License != "CC0-1.0" || !strings.HasPrefix(fixture.Provenance, "synthetic") {
		t.Fatalf("unexpected header: %+v", fixture)
	}
	walked := false
	for _, path := range h03CommittedHuaweiTestdata(t) {
		walked = walked || filepath.ToSlash(path) == m85FixturePath
	}
	if !walked {
		t.Fatal("the decision fixture is outside the committed shape scan")
	}
	if findings := h03ShapeFindings(string(data)); len(findings) != 0 {
		t.Fatalf("the decision fixture holds serial-, address- or time-shaped values: %v", findings)
	}
	names := map[string]bool{}
	for _, tc := range fixture.Cases {
		if names[tc.Name] || tc.Name == "" {
			t.Fatalf("case name %q repeats", tc.Name)
		}
		names[tc.Name] = true
	}
	families := map[string]bool{}
	for _, family := range fixture.Qualification {
		families[family.Family] = true
		if !names[family.Case] {
			t.Fatalf("%s names no case", family.Family)
		}
	}
	if !reflect.DeepEqual(families, map[string]bool{huaweiSUN2000M3Family: true, huaweiSUN2000M1Family: true, "SUN2000 MAP0 series": true}) {
		t.Fatalf("qualification families %v", families)
	}
}

// TestM85DecisionTableFixtureCases runs every row of the decision table:
// outcome, closed reason, grade, card, match kind and the named candidates,
// and the floor, edition and page of a selected row against the catalogue.
func TestM85DecisionTableFixtureCases(t *testing.T) {
	fixture, _ := m85Load(t)
	outcomes := map[string]int{}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			decision := m85Decide(t, tc.Input.catalogueInput())
			row, selected := decision.Row()
			candidates := m85Rows(decision.Candidates())
			if string(decision.Outcome()) != tc.Expect.Outcome || string(decision.Reason()) != tc.Expect.Reason ||
				string(decision.Grade()) != tc.Expect.Grade || row.Card != tc.Expect.Card || string(row.MatchedBy) != tc.Expect.MatchedBy ||
				selected != (tc.Expect.Outcome == "SELECTED") || !reflect.DeepEqual(candidates, append([]m85Row{}, tc.Expect.Candidates...)) {
				t.Fatalf("outcome=%s reason=%s grade=%s card=%q by=%q candidates=%v want %+v",
					decision.Outcome(), decision.Reason(), decision.Grade(), row.Card, row.MatchedBy, candidates, tc.Expect)
			}
			if decision.ModelText() != tc.Input.ModelText || decision.Scope() != HuaweiSUN2000EndpointScope(tc.Input.Scope) ||
				decision.FirmwareText() != tc.Input.FirmwareText || !reflect.DeepEqual(decision.ProtocolWords(), append([]uint16(nil), tc.Input.ProtocolWords...)) {
				t.Fatalf("the decision does not record its input: %+v", decision)
			}
			if identifier, read := decision.Identifier(); read != (tc.Input.Identifier != nil) || (read && identifier != *tc.Input.Identifier) {
				t.Fatalf("identifier %d read=%t", identifier, read)
			}
			if selected {
				m85RequireRowFacts(t, row)
				if row.Scope != decision.Scope() || (row.MatchedBy == HuaweiSUN2000CatalogueMatchWireAlias) != (row.WireAlias == tc.Input.ModelText && row.AliasEvidence != "") {
					t.Fatalf("selected row %+v", row)
				}
			}
			for _, candidate := range decision.Candidates() {
				m85RequireRowFacts(t, candidate)
			}
		})
		outcomes[tc.Expect.Outcome]++
	}
	if outcomes["SELECTED"] == 0 || outcomes["NOT_SELECTED"] == 0 || outcomes["AMBIGUOUS"] == 0 {
		t.Fatalf("the table misses an outcome: %v", outcomes)
	}
}

// TestM85QualificationFamiliesResolveToTheirCards resolves the three
// qualification families by identifier and checks every MAP0 identifier of the
// MA-V200 card: each source token selects its row, and the hyphenated form
// selects only where the catalogue records it as a wire alias.
func TestM85QualificationFamiliesResolveToTheirCards(t *testing.T) {
	fixture, _ := m85Load(t)
	for _, family := range fixture.Qualification {
		tc := m85Case(t, family.Case)
		row, ok := m85Decide(t, tc.Input.catalogueInput()).Row()
		if !ok || row.Card != family.Card || tc.Input.Scope != string(HuaweiSUN2000ScopeInverterMap) {
			t.Fatalf("%s resolves to %+v, want %s", family.Family, row, family.Card)
		}
	}
	aliases := 0
	for _, catalogueRow := range m85CatalogueRows(t) {
		if catalogueRow.Card != "MA-V200" {
			continue
		}
		identifier := uint16(catalogueRow.Identifier)
		input := HuaweiSUN2000CatalogueInput{Scope: HuaweiSUN2000ScopeInverterMap, ModelText: catalogueRow.Token, ModelIdentifier: &identifier, SoftwareText: "V200R024C00SPC100"}
		row, ok := m85Decide(t, input).Row()
		if !ok || row.Card != "MA-V200" || row.MatchedBy != HuaweiSUN2000CatalogueMatchToken {
			t.Fatalf("%s/%d does not select its MA-V200 row: %+v", catalogueRow.Token, identifier, row)
		}
		input.ModelText = strings.ReplaceAll(catalogueRow.Token, "_", "-")
		decision := m85Decide(t, input)
		if row, ok := decision.Row(); ok {
			if row.MatchedBy != HuaweiSUN2000CatalogueMatchWireAlias || row.Identifier != int(identifier) {
				t.Fatalf("%s selected %+v", input.ModelText, row)
			}
			aliases++
		} else if decision.Reason() != HuaweiSUN2000CatalogueNoCatalogueRow {
			t.Fatalf("%s: %s", input.ModelText, decision.Reason())
		}
	}
	if aliases != 1 {
		t.Fatalf("%d hyphenated MAP0 forms selected, want the one recorded alias", aliases)
	}
}

// TestM85FloorRelationVariesEveryComponent compares one build with floors that
// differ in each component: product, line, release, branch and build, and the
// shortened forms of the build.
func TestM85FloorRelationVariesEveryComponent(t *testing.T) {
	cases := []struct {
		build, floor string
		want         HuaweiSUN2000FloorRelation
	}{
		{"V200R023C00SPC128", "SUN2000MC V200R023C00", HuaweiSUN2000FloorAtOrAbove},
		{"V200R023C00SPC128", "V200R023C00SPC128", HuaweiSUN2000FloorAtOrAbove},
		{"V200R023C00SPC129", "V200R023C00SPC128", HuaweiSUN2000FloorAtOrAbove},
		{"V200R023C00SPC127", "V200R023C00SPC128", HuaweiSUN2000FloorBelow},
		{"V200R023C00SPC099", "V200R023C00SPC100", HuaweiSUN2000FloorBelow},
		{"V200R023C10SPC001", "V200R023C00SPC100", HuaweiSUN2000FloorNoDocumentedRelation},
		{"V200R022C00SPC900", "V200R023C00", HuaweiSUN2000FloorNoDocumentedRelation},
		{"V200R024C00SPC001", "V200R023C00", HuaweiSUN2000FloorNoDocumentedRelation},
		{"V100R023C00SPC900", "V200R023C00", HuaweiSUN2000FloorNoDocumentedRelation},
		{"V300R023C00SPC001", "V200R023C00", HuaweiSUN2000FloorNoDocumentedRelation},
		{"SUN2000MA V200R023C00SPC128", "SUN2000MC V200R023C00", HuaweiSUN2000FloorNoDocumentedRelation},
		{"SUN2000MC V200R023C00SPC128", "SUN2000MC V200R023C00", HuaweiSUN2000FloorAtOrAbove},
		{"SUN2000MC V200R023C00SPC128", "V200R023C00", HuaweiSUN2000FloorAtOrAbove},
		{"V100R001C81SPC100", "SUN2000 V100R001", HuaweiSUN2000FloorAtOrAbove},
		{"V100R002C81SPC100", "SUN2000 V100R001", HuaweiSUN2000FloorNoDocumentedRelation},
		{"V200R023", "SUN2000MC V200R023C00", HuaweiSUN2000FloorEqualOnCarriedParts},
		{"V200R023C00", "V200R023C00SPC128", HuaweiSUN2000FloorEqualOnCarriedParts},
		{"V200R023C00", "V200R023C00", HuaweiSUN2000FloorAtOrAbove},
		{"V200R024", "SUN2000MC V200R023C00", HuaweiSUN2000FloorNoDocumentedRelation},
		{"V200R023C00SPC128", "SUN2000MA V100R001C10SUN2000MA", HuaweiSUN2000FloorOutsideBranchForm},
		{"V200R023C00SPC128", "SUN2000L V200R001C0", HuaweiSUN2000FloorOutsideBranchForm},
		{"V200R023C10SPC110", "V200R023C10PSC110", HuaweiSUN2000FloorOutsideBranchForm},
	}
	for _, tc := range cases {
		build, ok := parseHuaweiSUN2000Version(tc.build)
		if !ok {
			t.Fatalf("%s does not parse", tc.build)
		}
		if got := huaweiSUN2000FloorRelationOf(build, tc.floor); got != tc.want {
			t.Errorf("%s against %s: %s want %s", tc.build, tc.floor, got, tc.want)
		}
	}
	for _, malformed := range []string{"", "V200R023C0", "V200R023C00SPC12", "v200r023c00spc128", "V200R023C00SPC128B001", "V200R023SPC128",
		"V20R023C00", "SUN2000MC  V200R023C00", "sun2000mc V200R023C00", "V200R023C00 SPC128", " V200R023C00"} {
		if _, ok := parseHuaweiSUN2000Version(malformed); ok {
			t.Errorf("%q parsed", malformed)
		}
	}
}

func TestM85MalformedAndAbsentInputsSelectNothing(t *testing.T) {
	m3 := m85Case(t, "m3_series_identifier_selects_the_mc_card").Input.catalogueInput()
	cases := []struct {
		name   string
		mutate func(*HuaweiSUN2000CatalogueInput)
		reason HuaweiSUN2000CatalogueReason
	}{
		{"empty model text", func(i *HuaweiSUN2000CatalogueInput) { i.ModelText = "" }, HuaweiSUN2000CatalogueModelTextInvalid},
		{"model text with a NUL", func(i *HuaweiSUN2000CatalogueInput) { i.ModelText += "\x00" }, HuaweiSUN2000CatalogueModelTextInvalid},
		{"model text beyond the read", func(i *HuaweiSUN2000CatalogueInput) { i.ModelText = strings.Repeat("S", 31) }, HuaweiSUN2000CatalogueModelTextInvalid},
		{"model text with a trailing space", func(i *HuaweiSUN2000CatalogueInput) { i.ModelText += " " }, HuaweiSUN2000CatalogueNoCatalogueRow},
		{"model text in lower case", func(i *HuaweiSUN2000CatalogueInput) { i.ModelText = strings.ToLower(i.ModelText) }, HuaweiSUN2000CatalogueNoCatalogueRow},
		{"model text prefix", func(i *HuaweiSUN2000CatalogueInput) { i.ModelText = "SUN2000-50KTL-M" }, HuaweiSUN2000CatalogueNoCatalogueRow},
		{"unknown scope", func(i *HuaweiSUN2000CatalogueInput) { i.Scope = "INVERTER" }, HuaweiSUN2000CatalogueScopeInvalid},
		{"empty scope", func(i *HuaweiSUN2000CatalogueInput) { i.Scope = "" }, HuaweiSUN2000CatalogueScopeInvalid},
		{"malformed software text", func(i *HuaweiSUN2000CatalogueInput) { i.SoftwareText = "V200R023C0" }, HuaweiSUN2000CatalogueSoftwareMalformed},
		{"suffixed software text", func(i *HuaweiSUN2000CatalogueInput) { i.SoftwareText += "B001" }, HuaweiSUN2000CatalogueSoftwareMalformed},
		{"malformed software text beside a valid firmware text", func(i *HuaweiSUN2000CatalogueInput) {
			i.SoftwareText, i.FirmwareText = "V200R023C0", "V200R023"
		}, HuaweiSUN2000CatalogueSoftwareMalformed},
		{"malformed firmware text without software text", func(i *HuaweiSUN2000CatalogueInput) {
			i.SoftwareText, i.FirmwareText = "", "X000X000"
		}, HuaweiSUN2000CatalogueSoftwareMalformed},
		{"neither software nor firmware text", func(i *HuaweiSUN2000CatalogueInput) { i.SoftwareText = "" }, HuaweiSUN2000CatalogueSoftwareAbsent},
		{"another identifier", func(i *HuaweiSUN2000CatalogueInput) { i.ModelIdentifier = m85Identifier(472) }, HuaweiSUN2000CatalogueIdentifierMismatch},
		{"identifier zero", func(i *HuaweiSUN2000CatalogueInput) { i.ModelIdentifier = m85Identifier(0) }, HuaweiSUN2000CatalogueIdentifierMismatch},
		{"identifier absent", func(i *HuaweiSUN2000CatalogueInput) { i.ModelIdentifier = nil }, HuaweiSUN2000CatalogueMultipleCandidateRows},
	}
	for _, tc := range cases {
		input := m3
		tc.mutate(&input)
		decision := m85Decide(t, input)
		if _, ok := decision.Row(); ok || decision.Outcome() == HuaweiSUN2000CatalogueSelected || decision.Reason() != tc.reason || decision.Grade() != HuaweiGradeUnknown {
			t.Errorf("%s: %s %s", tc.name, decision.Outcome(), decision.Reason())
		}
	}
	// The firmware text is compared only when no software text was read, and
	// the build source says which word was compared.
	input := m3
	input.FirmwareText = "V100R001"
	if build, source := m85Decide(t, input).Build(); build != m3.SoftwareText || source != HuaweiSUN2000BuildSoftwareText {
		t.Fatalf("build %q from %s", build, source)
	}
	input.SoftwareText = ""
	if decision := m85Decide(t, input); decision.Reason() != HuaweiSUN2000CatalogueNoDocumentedRelation {
		t.Fatalf("firmware text of another line: %s", decision.Reason())
	} else if build, source := decision.Build(); build != "V100R001" || source != HuaweiSUN2000BuildFirmwareText {
		t.Fatalf("build %q from %s", build, source)
	}
}

// TestM85QuarantinedAndDocumentFloorsAreRecorded selects a row whose floor is
// outside the branch form and a row whose floor is a document-wide
// requirement, each found in the committed catalogue.
func TestM85QuarantinedAndDocumentFloorsAreRecorded(t *testing.T) {
	pairs := map[string]int{}
	rows := m85CatalogueRows(t)
	scopeOf := func(card string) HuaweiSUN2000EndpointScope {
		if card == "SUN-C81" {
			return HuaweiSUN2000ScopeC81Map
		}
		return HuaweiSUN2000ScopeInverterMap
	}
	for _, row := range rows {
		pairs[fmt.Sprintf("%s|%s|%d", scopeOf(row.Card), row.Token, row.Identifier)]++
	}
	var quarantined, document, prefixed int
	for _, row := range rows {
		if row.Identifier < 0 || pairs[fmt.Sprintf("%s|%s|%d", scopeOf(row.Card), row.Token, row.Identifier)] != 1 {
			continue
		}
		outside := !huaweiSUN2000VersionForm.MatchString(row.Floor)
		if !outside && row.FloorBasis != "document" && !strings.Contains(row.Floor, " ") {
			continue
		}
		build := "V900R900C90SPC900"
		if minimum, ok := parseHuaweiSUN2000Version(row.Floor); ok {
			build = "V" + minimum.line + "R" + minimum.release + "C00SPC999"
			if minimum.hasBranch {
				build = "V" + minimum.line + "R" + minimum.release + "C" + minimum.branch + "SPC999"
			}
		}
		decision := m85Decide(t, HuaweiSUN2000CatalogueInput{Scope: scopeOf(row.Card), ModelText: row.Token, ModelIdentifier: m85Identifier(uint16(row.Identifier)), SoftwareText: build})
		selected, ok := decision.Row()
		if !ok || selected.Card != row.Card {
			t.Fatalf("%s %s/%d with %s: %s %s", row.Card, row.Token, row.Identifier, build, decision.Outcome(), decision.Reason())
		}
		m85RequireRowFacts(t, selected)
		switch {
		case outside:
			if decision.Reason() != HuaweiSUN2000CatalogueFloorOutsideBranchForm || len(selected.Quarantine) == 0 {
				t.Fatalf("%s %s: %s quarantine %v", row.Card, row.Token, decision.Reason(), selected.Quarantine)
			}
			quarantined++
		case row.FloorBasis == "document":
			if !selected.DocumentFloor() || selected.DocumentRequirement == "" || decision.Reason() != HuaweiSUN2000CatalogueAtOrAboveFloor {
				t.Fatalf("%s %s: document floor %+v", row.Card, row.Token, selected)
			}
			document++
		default:
			if selected.DocumentFloor() || decision.Reason() != HuaweiSUN2000CatalogueAtOrAboveFloor {
				t.Fatalf("%s %s: product floor %+v %s", row.Card, row.Token, selected, decision.Reason())
			}
			prefixed++
		}
	}
	if quarantined == 0 || document == 0 || prefixed == 0 {
		t.Fatalf("quarantined=%d document=%d prefixed=%d", quarantined, document, prefixed)
	}
}

// TestM85DuplicatePairsAreAmbiguousWhateverTheRowOrder decides every pair that
// two card rows of one scope share, and decides one pair with the model rows in
// reverse order: no order, card name or position chooses a winner.
func TestM85DuplicatePairsAreAmbiguousWhateverTheRowOrder(t *testing.T) {
	catalogue := huaweiSUN2000LoadedCatalogue()
	if catalogue.err != nil {
		t.Fatal(catalogue.err)
	}
	groups := map[string][]HuaweiSUN2000CatalogueRow{}
	var keys []string
	for _, entry := range catalogue.entries {
		key := fmt.Sprintf("%s|%s|%d", entry.row.Scope, entry.row.Token, entry.row.Identifier)
		if len(groups[key]) == 0 {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], entry.row)
	}
	duplicates := 0
	for _, key := range keys {
		rows := groups[key]
		if len(rows) < 2 {
			continue
		}
		duplicates++
		// A build of a line no floor names relates to none of them.
		input := HuaweiSUN2000CatalogueInput{Scope: rows[0].Scope, ModelText: rows[0].Token, ModelIdentifier: m85Identifier(uint16(rows[0].Identifier)), SoftwareText: "V900R900C90SPC900"}
		decision := decideHuaweiSUN2000Catalogue(catalogue, input)
		if decision.Outcome() != HuaweiSUN2000CatalogueAmbiguous || len(decision.Candidates()) != len(rows) {
			t.Fatalf("%s: %s with %d candidates", key, decision.Outcome(), len(decision.Candidates()))
		}
		reversed := huaweiSUN2000Catalogue{entries: make([]huaweiSUN2000CatalogueEntry, len(catalogue.entries))}
		for index, entry := range catalogue.entries {
			reversed.entries[len(catalogue.entries)-1-index] = entry
		}
		again := decideHuaweiSUN2000Catalogue(reversed, input)
		if again.Outcome() != decision.Outcome() || !reflect.DeepEqual(m85Rows(again.Candidates()), m85Rows(decision.Candidates())) {
			t.Fatalf("%s: the row order changed the decision", key)
		}
	}
	if duplicates == 0 {
		t.Fatal("no duplicate pair in the catalogue")
	}
	t.Logf("%d pairs are listed by two card rows of one scope", duplicates)
}

func m85CatalogueReader(t testing.TB, replace map[string]string) (func(string) ([]byte, error), func() ([]string, error)) {
	t.Helper()
	read := func(name string) ([]byte, error) {
		if text, ok := replace[name]; ok {
			return []byte(text), nil
		}
		return os.ReadFile(filepath.Join(huaweiSUN2000CatalogueDirectory, name))
	}
	list := func() ([]string, error) {
		entries, err := os.ReadDir(huaweiSUN2000CatalogueDirectory)
		if err != nil {
			return nil, err
		}
		var names []string
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		return names, nil
	}
	return read, list
}

// TestM85CardMapScopesComeFromTheIdentityRows checks the scope of every card
// against its committed identity rows and refuses a card file without an
// identity map, a card file of another edition and a repeated card.
func TestM85CardMapScopesComeFromTheIdentityRows(t *testing.T) {
	catalogue := huaweiSUN2000LoadedCatalogue()
	if catalogue.err != nil {
		t.Fatal(catalogue.err)
	}
	scopes := map[string]HuaweiSUN2000EndpointScope{}
	for _, entry := range catalogue.entries {
		if scope, ok := scopes[entry.row.Card]; ok && scope != entry.row.Scope {
			t.Fatalf("card %s has two scopes", entry.row.Card)
		}
		scopes[entry.row.Card] = entry.row.Scope
	}
	files, err := filepath.Glob(filepath.Join(huaweiSUN2000CatalogueDirectory, "registers-*.json"))
	if err != nil || len(files) != 16 {
		t.Fatalf("registers files %v: %v", files, err)
	}
	for _, file := range files {
		var registers struct {
			Card string           `json:"card"`
			Rows []map[string]any `json:"rows"`
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &registers); err != nil {
			t.Fatal(err)
		}
		identity := map[float64]bool{}
		for _, row := range registers.Rows {
			if offset, ok := row["offset"].(float64); ok && row["endpoint"] == "inverter" {
				identity[offset] = true
			}
		}
		want := HuaweiSUN2000ScopeInverterMap
		if !identity[30000] && !identity[30070] {
			want = HuaweiSUN2000ScopeC81Map
		}
		if scope, listed := scopes[registers.Card]; listed && scope != want {
			t.Fatalf("card %s scope %s want %s", registers.Card, scope, want)
		} else if !listed && registers.Card != "COMMERCIAL" {
			t.Fatalf("card %s lists no model row", registers.Card)
		}
	}
	if scopes["SUN-C81"] != HuaweiSUN2000ScopeC81Map || scopes["MC"] != HuaweiSUN2000ScopeInverterMap || len(scopes) != 15 {
		t.Fatalf("scopes %v", scopes)
	}
	for scope := range map[HuaweiSUN2000EndpointScope]bool{HuaweiSUN2000ScopeGatewayNamespace: true} {
		for card, cardScope := range scopes {
			if cardScope == scope {
				t.Fatalf("card %s is a gateway namespace", card)
			}
		}
	}

	mc, err := os.ReadFile(filepath.Join(huaweiSUN2000CatalogueDirectory, "registers-mc.json"))
	if err != nil {
		t.Fatal(err)
	}
	editions, err := os.ReadFile(filepath.Join(huaweiSUN2000CatalogueDirectory, "editions.json"))
	if err != nil {
		t.Fatal(err)
	}
	for name, replace := range map[string]map[string]string{
		"card without an identity map": {"registers-mc.json": strings.NewReplacer(`"offset":30070`, `"offset":30071`, `"offset":30000`, `"offset":30001`).Replace(string(mc))},
		"card file of another edition": {"registers-mc.json": strings.Replace(string(mc), `"edition": "sun2000mc-v200r023c00-01-2023-03-17"`, `"edition": "sun2000mc-v200r023c00-01-2023-03-18"`, 1)},
		"repeated card":                {"editions.json": strings.Replace(string(editions), `"card":"MA-C20"`, `"card":"MC"`, 1)},
		"trailing content":             {"models.json": "{\"rows\":[]}{}"},
	} {
		read, list := m85CatalogueReader(t, replace)
		if _, err := loadHuaweiSUN2000Catalogue(read, list); err == nil {
			t.Errorf("%s: loaded", name)
		}
		if decision := decideHuaweiSUN2000Catalogue(huaweiSUN2000Catalogue{err: errHuaweiSUN2000CatalogueInvalid}, m85Case(t, "m3_series_identifier_selects_the_mc_card").Input.catalogueInput()); decision.Reason() != HuaweiSUN2000CatalogueInvalid {
			t.Fatalf("an invalid catalogue decided %s", decision.Reason())
		}
	}
}

// TestM85WireAliasHoldsOnlyWithinItsRecordedScope rewrites the recorded alias
// scope: an alias whose scope names another identifier, or no inverter
// endpoint, is never applied.
func TestM85WireAliasHoldsOnlyWithinItsRecordedScope(t *testing.T) {
	models, err := os.ReadFile(filepath.Join(huaweiSUN2000CatalogueDirectory, "models.json"))
	if err != nil {
		t.Fatal(err)
	}
	const recorded = "alias only within exact model ID1006, MAP0 source and inverter endpoint scope"
	if strings.Count(string(models), recorded) == 0 {
		t.Fatal("the recorded alias scope changed")
	}
	alias := m85Case(t, "map0_series_wire_alias_selects_the_ma_v200_card").Input.catalogueInput()
	for name, scope := range map[string]string{
		"recorded scope":       recorded,
		"another identifier":   strings.Replace(recorded, "ID1006", "ID1005", 1),
		"no identifier":        strings.Replace(recorded, "ID1006", "model", 1),
		"no inverter endpoint": strings.Replace(recorded, "inverter endpoint", "gateway", 1),
	} {
		read, list := m85CatalogueReader(t, map[string]string{"models.json": strings.ReplaceAll(string(models), recorded, scope)})
		entries, err := loadHuaweiSUN2000Catalogue(read, list)
		if err != nil {
			t.Fatal(err)
		}
		decision := decideHuaweiSUN2000Catalogue(huaweiSUN2000Catalogue{entries: entries}, alias)
		row, selected := decision.Row()
		if selected != (name == "recorded scope") || (selected && row.MatchedBy != HuaweiSUN2000CatalogueMatchWireAlias) ||
			(!selected && decision.Reason() != HuaweiSUN2000CatalogueNoCatalogueRow) {
			t.Fatalf("%s: %s %s", name, decision.Outcome(), decision.Reason())
		}
	}
	// The alias never stands for its row outside the inverter map.
	for _, scope := range []HuaweiSUN2000EndpointScope{HuaweiSUN2000ScopeC81Map, HuaweiSUN2000ScopeGatewayNamespace} {
		input := alias
		input.Scope = scope
		if decision := m85Decide(t, input); decision.Reason() != HuaweiSUN2000CatalogueScopeMismatch {
			t.Fatalf("%s: %s", scope, decision.Reason())
		}
	}
}

// TestM85SelectionRoutesTheMonitoringProfilesByTheSelectedRow routes the two
// monitoring profiles through the catalogue decision: the M3 series by the MC
// row and the M1 series by the MA-C00 row, which the catalogue resolves for
// its identifier; a profile keeps reading the groups of its own contract.
func TestM85SelectionRoutesTheMonitoringProfilesByTheSelectedRow(t *testing.T) {
	for _, tc := range []struct {
		input              HuaweiSUN2000CapabilityInput
		profile, card      string
		identifier         int
		reason             HuaweiSUN2000CatalogueReason
		catalogueCandidate []m85Row
	}{
		{h05M3Input(t, 1), HuaweiSUN2000M3MonitoringProfileID, "MC", 471, HuaweiSUN2000CatalogueAtOrAboveFloor, []m85Row{{"MC", 471}}},
		{h05M1Input(t), HuaweiSUN2000M1MonitoringProfileID, "MA-C00", 428, HuaweiSUN2000CatalogueAtOrAboveFloor, []m85Row{{"MA-C00", 428}}},
		{h05WithIdentity(h05M3Input(t, 2), h05RoutedIdentity(t, "m3_series_identifier_selects_the_mc_card"), func(i *HuaweiSUN2000RoutedIdentity) {
			i.SoftwareBlock = h05RoutedModelBlock("V200R025C00SPC100")
		}), HuaweiSUN2000M3MonitoringProfileID, "MC", 471, HuaweiSUN2000CatalogueNoDocumentedRelation, []m85Row{{"MC", 471}}},
	} {
		selection := h05Select(t, tc.input, tc.profile)
		decision, ok := selection.Catalogue()
		row, selected := decision.Row()
		if !ok || !selected || selection.Reason() != HuaweiSUN2000ReasonMatched || row.Card != tc.card || row.Identifier != tc.identifier ||
			decision.Reason() != tc.reason || decision.Grade() != HuaweiGradeHypothesis || !reflect.DeepEqual(m85Rows(decision.Candidates()), tc.catalogueCandidate) {
			t.Fatalf("%s: reason=%s row=%+v decision=%s", tc.profile, selection.Reason(), row, decision.Reason())
		}
		m85RequireRowFacts(t, row)
		profile, _ := selection.Profile()
		if profile.Document() != huaweiSUN2000MCEdition || !reflect.DeepEqual(selection.Plan(), profile.Plan(selection.UnitID())) {
			t.Fatalf("%s: the routed profile changed its contract", tc.profile)
		}
	}

	// The model text alone selects nothing: without the identifier the M3
	// series token matches three rows, which the decision names, and the M1
	// series token one row, which it does not select. No profile is admitted
	// on the model text alone (helianthus-modbusreg issue 365 removed the
	// observed-path exception).
	for _, tc := range []struct {
		input      HuaweiSUN2000CapabilityInput
		outcome    HuaweiSUN2000CapabilityOutcome
		reason     HuaweiSUN2000CapabilityReason
		decided    HuaweiSUN2000CatalogueOutcome
		closed     HuaweiSUN2000CatalogueReason
		candidates []m85Row
	}{
		{h05ObservedM3Input(t, 1), HuaweiSUN2000AmbiguousMultipleProfiles, HuaweiSUN2000ReasonCatalogueAmbiguous,
			HuaweiSUN2000CatalogueAmbiguous, HuaweiSUN2000CatalogueMultipleCandidateRows, []m85Row{{"MA-C00", 449}, {"MA-C20", 449}, {"MC", 471}}},
		{h05ObservedM1Input(t), HuaweiSUN2000NoAdmissibleProfile, HuaweiSUN2000ReasonCatalogueNotSelected,
			HuaweiSUN2000CatalogueNotSelected, HuaweiSUN2000CatalogueIdentifierAbsent, []m85Row{{"MA-C00", 428}}},
	} {
		selection := SelectHuaweiSUN2000MonitoringProfile(tc.input)
		decision, ok := selection.Catalogue()
		_, profiled := selection.Profile()
		if _, selected := decision.Row(); !ok || selected || profiled || len(selection.Plan()) != 0 || selection.Outcome() != tc.outcome ||
			selection.Reason() != tc.reason || decision.Outcome() != tc.decided || decision.Reason() != tc.closed ||
			!reflect.DeepEqual(m85Rows(decision.Candidates()), tc.candidates) {
			t.Fatalf("model text only: %s %s decision=%s %s %v", selection.Outcome(), selection.Reason(), decision.Outcome(), decision.Reason(), m85Rows(decision.Candidates()))
		}
	}

	// Two rows that the catalogue cannot tell apart select nothing and build
	// no observer; a selected row of another family selects no profile.
	m3Identity := h05RoutedIdentity(t, "m3_series_identifier_selects_the_mc_card")
	ambiguous := SelectHuaweiSUN2000MonitoringProfile(h05WithIdentity(h05M3Input(t, 1), m3Identity, func(i *HuaweiSUN2000RoutedIdentity) { i.IdentifierWords = []uint16{449} }))
	decision, _ := ambiguous.Catalogue()
	if ambiguous.Outcome() != HuaweiSUN2000AmbiguousMultipleProfiles || ambiguous.Reason() != HuaweiSUN2000ReasonCatalogueAmbiguous || len(ambiguous.Plan()) != 0 ||
		!reflect.DeepEqual(m85Rows(decision.Candidates()), []m85Row{{"MA-C00", 449}, {"MA-C20", 449}}) {
		t.Fatalf("449: %s %s %v", ambiguous.Outcome(), ambiguous.Reason(), m85Rows(decision.Candidates()))
	}
	if observer, err := NewHuaweiSUN2000MonitoringObserver(ambiguous, &h05Session{}); err == nil || observer != nil {
		t.Fatal("an ambiguous catalogue decision built an observer")
	}
	alias := h05WithIdentity(h05WithListedModel(t, h05M3Input(t, 6), "SUN2000-12K-MAP0"), h05RoutedIdentity(t, "map0_series_wire_alias_selects_the_ma_v200_card"), nil)
	withoutProfile := SelectHuaweiSUN2000MonitoringProfile(alias)
	decision, _ = withoutProfile.Catalogue()
	if row, ok := decision.Row(); withoutProfile.Outcome() != HuaweiSUN2000NoAdmissibleProfile || withoutProfile.Reason() != HuaweiSUN2000ReasonCatalogueRowNoProfile ||
		!ok || row.Card != "MA-V200" || row.MatchedBy != HuaweiSUN2000CatalogueMatchWireAlias {
		t.Fatalf("MAP0 behind the SmartLogger: %s %s %+v", withoutProfile.Outcome(), withoutProfile.Reason(), row)
	}
	if _, ok := SelectHuaweiSUN2000MonitoringProfile(HuaweiSUN2000CapabilityInput{}).Catalogue(); ok {
		t.Fatal("a decision without gateway evidence")
	}
}

// TestM85DispositionRecordsTheCatalogueSelection compares the selection
// records of the SUN2000 disposition with the decisions they name.
func TestM85DispositionRecordsTheCatalogueSelection(t *testing.T) {
	record := decodeHuaweiObject(t, "profiles/vendor/huawei/sun2000-disposition.json")
	detection := huaweiObject(t, record["capability_detection"])
	catalogue := huaweiObject(t, detection["catalogue_decision"])
	requireHuaweiKeys(t, catalogue, "inputs", "candidate_row", "scope", "floor", "outcomes", "selected_grade", "without_identifier",
		"provider_admits_on", "routing", "fixture", "docs_page")
	if catalogue["fixture"] != m85FixturePath || catalogue["docs_page"] != "protocols/huawei/sun2000-inverter-catalogue-v1.md" ||
		catalogue["scope"] != string(HuaweiSUN2000ScopeInverterMap) || catalogue["selected_grade"] != string(HuaweiGradeHypothesis) ||
		catalogue["provider_admits_on"] != string(HuaweiSUN2000CatalogueSelected) ||
		!reflect.DeepEqual(huaweiStringSlice(t, catalogue["outcomes"]), []string{"SELECTED", "NOT_SELECTED", "AMBIGUOUS"}) {
		t.Fatalf("catalogue decision policy: %#v", catalogue)
	}
	h03RequireFiles(t, []string{m85FixturePath})
	rows := h03Array(t, record["monitoring_profiles"])
	observed := map[string]string{
		HuaweiSUN2000M3MonitoringProfileID: "smartlogger_provider_cycles.json", HuaweiSUN2000M1MonitoringProfileID: "sdongle_provider_cycles.json",
	}
	for _, value := range rows {
		row := huaweiObject(t, value)
		selection := huaweiObject(t, row["catalogue_selection"])
		requireHuaweiKeys(t, selection, "decision_case", "outcome", "reason", "grade", "card", "edition", "page", "token", "identifier",
			"matched_by", "wire_alias", "floor", "floor_basis")
		decision := m85Decide(t, m85Case(t, selection["decision_case"].(string)).Input.catalogueInput())
		decided, ok := decision.Row()
		edition := huaweiObject(t, selection["edition"])
		if !ok || selection["outcome"] != string(decision.Outcome()) || selection["reason"] != string(decision.Reason()) ||
			selection["grade"] != string(decision.Grade()) || selection["card"] != decided.Card || edition["title"] != decided.Edition.Title ||
			edition["issue"] != decided.Edition.Issue || edition["date"] != decided.Edition.Date || huaweiInt(t, selection["page"]) != decided.Page ||
			selection["token"] != decided.Token || huaweiInt(t, selection["identifier"]) != decided.Identifier ||
			selection["matched_by"] != string(decided.MatchedBy) || selection["wire_alias"] != decided.WireAlias ||
			selection["floor"] != decided.Floor || selection["floor_basis"] != decided.FloorBasis {
			t.Fatalf("%v: the record disagrees with the decision %+v", row["family"], decided)
		}
		fixture, _ := m85Load(t)
		for _, family := range fixture.Qualification {
			if family.Family == row["family"] && (family.Case != selection["decision_case"] || family.Card != decided.Card || family.GatewayPath != row["gateway_path"]) {
				t.Fatalf("%s: the record names %v", family.Family, selection)
			}
		}
		id, _ := row["profile_id"].(string)
		fixtureName, monitored := observed[id]
		if !monitored {
			if row["family"] != "SUN2000 MAP0 series" {
				t.Fatalf("row %v has no profile", row["family"])
			}
			continue
		}
		// The observed path record is the decision the provider records when
		// it admits the family's child in its scripted session.
		path := huaweiObject(t, row["observed_path"])
		requireHuaweiKeys(t, path, "provider_fixture", "admission_reads", "identifier_read", "catalogue_outcome", "catalogue_reason", "grade",
			"card", "edition", "page", "identifier", "floor", "floor_basis", "candidates")
		provider, _ := h06Load(t, fixtureName)
		_, cycles := h06RunFixture(t, fixtureName)
		child, found := cycles[0].Child(1)
		if !found || child.Catalogue == nil || child.ProfileID != id || provider.Path != row["gateway_path"] {
			t.Fatalf("%s: the provider admitted no child of the family: %+v", id, child)
		}
		admitted := *child.Catalogue
		admittedRow, rowSelected := admitted.Row()
		identifier, read := admitted.Identifier()
		pathEdition := huaweiObject(t, path["edition"])
		var candidates []m85Row
		for _, value := range h03Array(t, path["candidates"]) {
			candidate := huaweiObject(t, value)
			candidates = append(candidates, m85Row{Card: candidate["card"].(string), Identifier: huaweiInt(t, candidate["identifier"])})
		}
		if path["provider_fixture"] != h06FixtureDirectory+"/"+fixtureName || path["identifier_read"] != read || !read || !rowSelected ||
			!reflect.DeepEqual(huaweiStringSlice(t, path["admission_reads"]), []string{"model_text_30000", "model_identifier_30070", "software_text_30050", "firmware_text_30035"}) ||
			path["catalogue_outcome"] != string(admitted.Outcome()) || path["catalogue_reason"] != string(admitted.Reason()) ||
			path["grade"] != string(admitted.Grade()) || path["card"] != admittedRow.Card || pathEdition["title"] != admittedRow.Edition.Title ||
			pathEdition["issue"] != admittedRow.Edition.Issue || pathEdition["date"] != admittedRow.Edition.Date || huaweiInt(t, path["page"]) != admittedRow.Page ||
			huaweiInt(t, path["identifier"]) != int(identifier) || huaweiInt(t, path["identifier"]) != admittedRow.Identifier ||
			path["floor"] != admittedRow.Floor || path["floor_basis"] != admittedRow.FloorBasis || !reflect.DeepEqual(candidates, m85Rows(admitted.Candidates())) ||
			path["card"] != selection["card"] || huaweiInt(t, path["page"]) != huaweiInt(t, selection["page"]) {
			t.Fatalf("%s observed path record %#v disagrees with the admission %+v", id, path, admittedRow)
		}
	}
	families := huaweiObject(t, record["model_families"])
	if _, refused := families["unsupported"]; refused {
		t.Fatal("the SUN2000 disposition still refuses every other family")
	}
	if strings.Contains(string(mustReadHuaweiFile(t, "profiles/vendor/huawei/sun2000-disposition.json")), "no_consulted_edition") {
		t.Fatal("the MAP0 series still has no consulted edition")
	}
}
