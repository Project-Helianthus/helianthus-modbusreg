package huawei

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

const m85FixturePath = "testdata/huawei/selection/sun2000_catalog_decisions.json"

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
	data, err := os.ReadFile(huaweiModulePath(m85FixturePath))
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

func (input m85Input) catalogInput() SUN2000CatalogInput {
	out := SUN2000CatalogInput{
		Scope: SUN2000EndpointScope(input.Scope), ModelText: input.ModelText, SoftwareText: input.SoftwareText,
		FirmwareText: input.FirmwareText, ProtocolWords: input.ProtocolWords,
	}
	if input.Identifier != nil {
		identifier := *input.Identifier
		out.ModelIdentifier = &identifier
	}
	return out
}

func m85Identifier(value uint16) *uint16 { return &value }

func m85Rows(rows []SUN2000CatalogRow) []m85Row {
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

// m85CatalogRow is a model row of a card edition read straight from the
// committed catalog files, independently of the production loader.
type m85CatalogRow struct {
	Card, Edition, Title, Issue, Date, Token, Floor, FloorBasis, Requirement string
	Page, Identifier                                                         int
	Quarantine                                                               []string
}

var (
	m85CatalogRowsOnce  sync.Once
	m85CatalogRowsCache []m85CatalogRow
	m85CatalogRowsErr   error
)

func m85CatalogRows(t testing.TB) []m85CatalogRow {
	t.Helper()
	m85CatalogRowsOnce.Do(func() { m85CatalogRowsCache, m85CatalogRowsErr = m85ReadCatalogRows() })
	if m85CatalogRowsErr != nil {
		t.Fatal(m85CatalogRowsErr)
	}
	return m85CatalogRowsCache
}

func m85ReadCatalogRows() ([]m85CatalogRow, error) {
	var editions struct {
		Editions []map[string]any `json:"editions"`
	}
	var models struct {
		Rows []map[string]any `json:"rows"`
	}
	for name, target := range map[string]any{"editions.json": &editions, "models.json": &models} {
		data, err := os.ReadFile(filepath.Join("../profiles", huaweiSUN2000CatalogDirectory, name))
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
	var rows []m85CatalogRow
	for _, row := range models.Rows {
		edition, ok := byEdition[row["edition"].(string)]
		if !ok {
			continue
		}
		out := m85CatalogRow{
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

func m85CatalogRowOf(t testing.TB, card string, identifier int) m85CatalogRow {
	t.Helper()
	var found []m85CatalogRow
	for _, row := range m85CatalogRows(t) {
		if row.Card == card && row.Identifier == identifier {
			found = append(found, row)
		}
	}
	if len(found) != 1 {
		t.Fatalf("card %s identifier %d: %d rows", card, identifier, len(found))
	}
	return found[0]
}

// m85RequireRowFacts checks a decided row against the committed catalog:
// edition identity, page, token, floor and floor basis.
func m85RequireRowFacts(t testing.TB, row SUN2000CatalogRow) {
	t.Helper()
	want := m85CatalogRowOf(t, row.Card, row.Identifier)
	if row.Edition != (SUN2000CatalogEdition{ID: want.Edition, Title: want.Title, Issue: want.Issue, Date: want.Date}) ||
		row.Page != want.Page || row.Token != want.Token || row.Floor != want.Floor || row.FloorBasis != want.FloorBasis ||
		row.DocumentRequirement != want.Requirement || !reflect.DeepEqual(row.Quarantine, want.Quarantine) || !row.HasIdentifier {
		t.Fatalf("row %+v disagrees with the catalog row %+v", row, want)
	}
}

func m85Decide(t testing.TB, input SUN2000CatalogInput) SUN2000CatalogDecision {
	t.Helper()
	decision := DecideHuaweiSUN2000CatalogSelection(input)
	if decision.LiveQualified() || decision.SupportClaim() || decision.WriteAuthority() {
		t.Fatal("a catalog decision grants authority")
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
	if fixture.Schema != "helianthus-huawei-catalog-selection-fixture/v1" || fixture.Kind != "catalog_decision_table" ||
		fixture.Family != "SUN2000" || fixture.License != "CC0-1.0" || !strings.HasPrefix(fixture.Provenance, "synthetic") {
		t.Fatalf("unexpected header: %+v", fixture)
	}
	walked := false
	for _, path := range h03CommittedHuaweiTestdata(t) {
		walked = walked || path == huaweiModulePath(m85FixturePath)
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
// and the floor, edition and page of a selected row against the catalog.
func TestM85DecisionTableFixtureCases(t *testing.T) {
	fixture, _ := m85Load(t)
	outcomes := map[string]int{}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			decision := m85Decide(t, tc.Input.catalogInput())
			row, selected := decision.Row()
			candidates := m85Rows(decision.Candidates())
			if string(decision.Outcome()) != tc.Expect.Outcome || string(decision.Reason()) != tc.Expect.Reason ||
				string(decision.Grade()) != tc.Expect.Grade || row.Card != tc.Expect.Card || string(row.MatchedBy) != tc.Expect.MatchedBy ||
				selected != (tc.Expect.Outcome == "SELECTED") || !reflect.DeepEqual(candidates, append([]m85Row{}, tc.Expect.Candidates...)) {
				t.Fatalf("outcome=%s reason=%s grade=%s card=%q by=%q candidates=%v want %+v",
					decision.Outcome(), decision.Reason(), decision.Grade(), row.Card, row.MatchedBy, candidates, tc.Expect)
			}
			if decision.ModelText() != tc.Input.ModelText || decision.Scope() != SUN2000EndpointScope(tc.Input.Scope) ||
				decision.FirmwareText() != tc.Input.FirmwareText || !reflect.DeepEqual(decision.ProtocolWords(), append([]uint16(nil), tc.Input.ProtocolWords...)) {
				t.Fatalf("the decision does not record its input: %+v", decision)
			}
			if identifier, read := decision.Identifier(); read != (tc.Input.Identifier != nil) || (read && identifier != *tc.Input.Identifier) {
				t.Fatalf("identifier %d read=%t", identifier, read)
			}
			if selected {
				m85RequireRowFacts(t, row)
				if row.Scope != decision.Scope() || (row.MatchedBy == SUN2000CatalogMatchWireAlias) != (row.WireAlias == tc.Input.ModelText && row.AliasEvidence != "") {
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
// selects only where the catalog records it as a wire alias.
func TestM85QualificationFamiliesResolveToTheirCards(t *testing.T) {
	fixture, _ := m85Load(t)
	for _, family := range fixture.Qualification {
		tc := m85Case(t, family.Case)
		row, ok := m85Decide(t, tc.Input.catalogInput()).Row()
		if !ok || row.Card != family.Card || tc.Input.Scope != string(SUN2000ScopeInverterMap) {
			t.Fatalf("%s resolves to %+v, want %s", family.Family, row, family.Card)
		}
	}
	aliases := 0
	for _, catalogRow := range m85CatalogRows(t) {
		if catalogRow.Card != "MA-V200" {
			continue
		}
		identifier := uint16(catalogRow.Identifier)
		input := SUN2000CatalogInput{Scope: SUN2000ScopeInverterMap, ModelText: catalogRow.Token, ModelIdentifier: &identifier, SoftwareText: "V200R024C00SPC100"}
		row, ok := m85Decide(t, input).Row()
		if !ok || row.Card != "MA-V200" || row.MatchedBy != SUN2000CatalogMatchToken {
			t.Fatalf("%s/%d does not select its MA-V200 row: %+v", catalogRow.Token, identifier, row)
		}
		input.ModelText = strings.ReplaceAll(catalogRow.Token, "_", "-")
		decision := m85Decide(t, input)
		if row, ok := decision.Row(); ok {
			if row.MatchedBy != SUN2000CatalogMatchWireAlias || row.Identifier != int(identifier) {
				t.Fatalf("%s selected %+v", input.ModelText, row)
			}
			aliases++
		} else if decision.Reason() != SUN2000CatalogNoCatalogRow {
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
		want         SUN2000FloorRelation
	}{
		{"V200R023C00SPC128", "SUN2000MC V200R023C00", SUN2000FloorAtOrAbove},
		{"V200R023C00SPC128", "V200R023C00SPC128", SUN2000FloorAtOrAbove},
		{"V200R023C00SPC129", "V200R023C00SPC128", SUN2000FloorAtOrAbove},
		{"V200R023C00SPC127", "V200R023C00SPC128", SUN2000FloorBelow},
		{"V200R023C00SPC099", "V200R023C00SPC100", SUN2000FloorBelow},
		{"V200R023C10SPC001", "V200R023C00SPC100", SUN2000FloorNoDocumentedRelation},
		{"V200R022C00SPC900", "V200R023C00", SUN2000FloorNoDocumentedRelation},
		{"V200R024C00SPC001", "V200R023C00", SUN2000FloorNoDocumentedRelation},
		{"V100R023C00SPC900", "V200R023C00", SUN2000FloorNoDocumentedRelation},
		{"V300R023C00SPC001", "V200R023C00", SUN2000FloorNoDocumentedRelation},
		{"SUN2000MA V200R023C00SPC128", "SUN2000MC V200R023C00", SUN2000FloorNoDocumentedRelation},
		{"SUN2000MC V200R023C00SPC128", "SUN2000MC V200R023C00", SUN2000FloorAtOrAbove},
		{"SUN2000MC V200R023C00SPC128", "V200R023C00", SUN2000FloorAtOrAbove},
		{"V100R001C81SPC100", "SUN2000 V100R001", SUN2000FloorAtOrAbove},
		{"V100R002C81SPC100", "SUN2000 V100R001", SUN2000FloorNoDocumentedRelation},
		{"V200R023", "SUN2000MC V200R023C00", SUN2000FloorEqualOnCarriedParts},
		{"V200R023C00", "V200R023C00SPC128", SUN2000FloorEqualOnCarriedParts},
		{"V200R023C00", "V200R023C00", SUN2000FloorAtOrAbove},
		{"V200R024", "SUN2000MC V200R023C00", SUN2000FloorNoDocumentedRelation},
		{"V200R023C00SPC128", "SUN2000MA V100R001C10SUN2000MA", SUN2000FloorOutsideBranchForm},
		{"V200R023C00SPC128", "SUN2000L V200R001C0", SUN2000FloorOutsideBranchForm},
		{"V200R023C10SPC110", "V200R023C10PSC110", SUN2000FloorOutsideBranchForm},
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
	m3 := m85Case(t, "m3_series_identifier_selects_the_mc_card").Input.catalogInput()
	cases := []struct {
		name   string
		mutate func(*SUN2000CatalogInput)
		reason SUN2000CatalogReason
	}{
		{"empty model text", func(i *SUN2000CatalogInput) { i.ModelText = "" }, SUN2000CatalogModelTextInvalid},
		{"model text with a NUL", func(i *SUN2000CatalogInput) { i.ModelText += "\x00" }, SUN2000CatalogModelTextInvalid},
		{"model text beyond the read", func(i *SUN2000CatalogInput) { i.ModelText = strings.Repeat("S", 31) }, SUN2000CatalogModelTextInvalid},
		{"model text with a trailing space", func(i *SUN2000CatalogInput) { i.ModelText += " " }, SUN2000CatalogNoCatalogRow},
		{"model text in lower case", func(i *SUN2000CatalogInput) { i.ModelText = strings.ToLower(i.ModelText) }, SUN2000CatalogNoCatalogRow},
		{"model text prefix", func(i *SUN2000CatalogInput) { i.ModelText = "SUN2000-50KTL-M" }, SUN2000CatalogNoCatalogRow},
		{"unknown scope", func(i *SUN2000CatalogInput) { i.Scope = "INVERTER" }, SUN2000CatalogScopeInvalid},
		{"empty scope", func(i *SUN2000CatalogInput) { i.Scope = "" }, SUN2000CatalogScopeInvalid},
		{"malformed software text", func(i *SUN2000CatalogInput) { i.SoftwareText = "V200R023C0" }, SUN2000CatalogSoftwareMalformed},
		{"suffixed software text", func(i *SUN2000CatalogInput) { i.SoftwareText += "B001" }, SUN2000CatalogSoftwareMalformed},
		{"malformed software text beside a valid firmware text", func(i *SUN2000CatalogInput) {
			i.SoftwareText, i.FirmwareText = "V200R023C0", "V200R023"
		}, SUN2000CatalogSoftwareMalformed},
		{"malformed firmware text without software text", func(i *SUN2000CatalogInput) {
			i.SoftwareText, i.FirmwareText = "", "X000X000"
		}, SUN2000CatalogSoftwareMalformed},
		{"neither software nor firmware text", func(i *SUN2000CatalogInput) { i.SoftwareText = "" }, SUN2000CatalogSoftwareAbsent},
		{"another identifier", func(i *SUN2000CatalogInput) { i.ModelIdentifier = m85Identifier(472) }, SUN2000CatalogIdentifierMismatch},
		{"identifier zero", func(i *SUN2000CatalogInput) { i.ModelIdentifier = m85Identifier(0) }, SUN2000CatalogIdentifierMismatch},
		{"identifier absent", func(i *SUN2000CatalogInput) { i.ModelIdentifier = nil }, SUN2000CatalogMultipleCandidateRows},
	}
	for _, tc := range cases {
		input := m3
		tc.mutate(&input)
		decision := m85Decide(t, input)
		if _, ok := decision.Row(); ok || decision.Outcome() == SUN2000CatalogSelected || decision.Reason() != tc.reason || decision.Grade() != GradeUnknown {
			t.Errorf("%s: %s %s", tc.name, decision.Outcome(), decision.Reason())
		}
	}
	// The firmware text is compared only when no software text was read, and
	// the build source says which word was compared.
	input := m3
	input.FirmwareText = "V100R001"
	if build, source := m85Decide(t, input).Build(); build != m3.SoftwareText || source != SUN2000BuildSoftwareText {
		t.Fatalf("build %q from %s", build, source)
	}
	input.SoftwareText = ""
	if decision := m85Decide(t, input); decision.Reason() != SUN2000CatalogNoDocumentedRelation {
		t.Fatalf("firmware text of another line: %s", decision.Reason())
	} else if build, source := decision.Build(); build != "V100R001" || source != SUN2000BuildFirmwareText {
		t.Fatalf("build %q from %s", build, source)
	}
}

// TestM85QuarantinedAndDocumentFloorsAreRecorded selects a row whose floor is
// outside the branch form and a row whose floor is a document-wide
// requirement, each found in the committed catalog.
func TestM85QuarantinedAndDocumentFloorsAreRecorded(t *testing.T) {
	pairs := map[string]int{}
	rows := m85CatalogRows(t)
	scopeOf := func(card string) SUN2000EndpointScope {
		if card == "SUN-C81" {
			return SUN2000ScopeC81Map
		}
		return SUN2000ScopeInverterMap
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
		decision := m85Decide(t, SUN2000CatalogInput{Scope: scopeOf(row.Card), ModelText: row.Token, ModelIdentifier: m85Identifier(uint16(row.Identifier)), SoftwareText: build})
		selected, ok := decision.Row()
		if !ok || selected.Card != row.Card {
			t.Fatalf("%s %s/%d with %s: %s %s", row.Card, row.Token, row.Identifier, build, decision.Outcome(), decision.Reason())
		}
		m85RequireRowFacts(t, selected)
		switch {
		case outside:
			if decision.Reason() != SUN2000CatalogFloorOutsideBranchForm || len(selected.Quarantine) == 0 {
				t.Fatalf("%s %s: %s quarantine %v", row.Card, row.Token, decision.Reason(), selected.Quarantine)
			}
			quarantined++
		case row.FloorBasis == "document":
			if !selected.DocumentFloor() || selected.DocumentRequirement == "" || decision.Reason() != SUN2000CatalogAtOrAboveFloor {
				t.Fatalf("%s %s: document floor %+v", row.Card, row.Token, selected)
			}
			document++
		default:
			if selected.DocumentFloor() || decision.Reason() != SUN2000CatalogAtOrAboveFloor {
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
	catalog := huaweiSUN2000LoadedCatalog()
	if catalog.err != nil {
		t.Fatal(catalog.err)
	}
	groups := map[string][]SUN2000CatalogRow{}
	var keys []string
	for _, entry := range catalog.entries {
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
		input := SUN2000CatalogInput{Scope: rows[0].Scope, ModelText: rows[0].Token, ModelIdentifier: m85Identifier(uint16(rows[0].Identifier)), SoftwareText: "V900R900C90SPC900"}
		decision := decideHuaweiSUN2000Catalog(catalog, input)
		if decision.Outcome() != SUN2000CatalogAmbiguous || len(decision.Candidates()) != len(rows) {
			t.Fatalf("%s: %s with %d candidates", key, decision.Outcome(), len(decision.Candidates()))
		}
		reversed := huaweiSUN2000Catalog{entries: make([]huaweiSUN2000CatalogEntry, len(catalog.entries))}
		for index, entry := range catalog.entries {
			reversed.entries[len(catalog.entries)-1-index] = entry
		}
		again := decideHuaweiSUN2000Catalog(reversed, input)
		if again.Outcome() != decision.Outcome() || !reflect.DeepEqual(m85Rows(again.Candidates()), m85Rows(decision.Candidates())) {
			t.Fatalf("%s: the row order changed the decision", key)
		}
	}
	if duplicates == 0 {
		t.Fatal("no duplicate pair in the catalog")
	}
	t.Logf("%d pairs are listed by two card rows of one scope", duplicates)
}

func m85CatalogReader(t testing.TB, replace map[string]string) (func(string) ([]byte, error), func() ([]string, error)) {
	t.Helper()
	read := func(name string) ([]byte, error) {
		if text, ok := replace[name]; ok {
			return []byte(text), nil
		}
		return os.ReadFile(filepath.Join("../profiles", huaweiSUN2000CatalogDirectory, name))
	}
	list := func() ([]string, error) {
		entries, err := os.ReadDir(filepath.Join("../profiles", huaweiSUN2000CatalogDirectory))
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
	catalog := huaweiSUN2000LoadedCatalog()
	if catalog.err != nil {
		t.Fatal(catalog.err)
	}
	scopes := map[string]SUN2000EndpointScope{}
	for _, entry := range catalog.entries {
		if scope, ok := scopes[entry.row.Card]; ok && scope != entry.row.Scope {
			t.Fatalf("card %s has two scopes", entry.row.Card)
		}
		scopes[entry.row.Card] = entry.row.Scope
	}
	files, err := filepath.Glob(filepath.Join("../profiles", huaweiSUN2000CatalogDirectory, "registers-*.json"))
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
		want := SUN2000ScopeInverterMap
		if !identity[30000] && !identity[30070] {
			want = SUN2000ScopeC81Map
		}
		if scope, listed := scopes[registers.Card]; listed && scope != want {
			t.Fatalf("card %s scope %s want %s", registers.Card, scope, want)
		} else if !listed && registers.Card != "COMMERCIAL" {
			t.Fatalf("card %s lists no model row", registers.Card)
		}
	}
	if scopes["SUN-C81"] != SUN2000ScopeC81Map || scopes["MC"] != SUN2000ScopeInverterMap || len(scopes) != 15 {
		t.Fatalf("scopes %v", scopes)
	}
	for scope := range map[SUN2000EndpointScope]bool{SUN2000ScopeGatewayNamespace: true} {
		for card, cardScope := range scopes {
			if cardScope == scope {
				t.Fatalf("card %s is a gateway namespace", card)
			}
		}
	}

	mc, err := os.ReadFile(filepath.Join("../profiles", huaweiSUN2000CatalogDirectory, "registers-mc.json"))
	if err != nil {
		t.Fatal(err)
	}
	editions, err := os.ReadFile(filepath.Join("../profiles", huaweiSUN2000CatalogDirectory, "editions.json"))
	if err != nil {
		t.Fatal(err)
	}
	for name, replace := range map[string]map[string]string{
		"card without an identity map": {"registers-mc.json": strings.NewReplacer(`"offset":30070`, `"offset":30071`, `"offset":30000`, `"offset":30001`).Replace(string(mc))},
		"card file of another edition": {"registers-mc.json": strings.Replace(string(mc), `"edition": "sun2000mc-v200r023c00-01-2023-03-17"`, `"edition": "sun2000mc-v200r023c00-01-2023-03-18"`, 1)},
		"repeated card":                {"editions.json": strings.Replace(string(editions), `"card":"MA-C20"`, `"card":"MC"`, 1)},
		"trailing content":             {"models.json": "{\"rows\":[]}{}"},
	} {
		read, list := m85CatalogReader(t, replace)
		if _, err := loadHuaweiSUN2000Catalog(read, list); err == nil {
			t.Errorf("%s: loaded", name)
		}
		if decision := decideHuaweiSUN2000Catalog(huaweiSUN2000Catalog{err: errHuaweiSUN2000CatalogInvalid}, m85Case(t, "m3_series_identifier_selects_the_mc_card").Input.catalogInput()); decision.Reason() != SUN2000CatalogInvalid {
			t.Fatalf("an invalid catalog decided %s", decision.Reason())
		}
	}
}

// TestM85WireAliasHoldsOnlyWithinItsRecordedScope rewrites the recorded alias
// scope: an alias whose scope names another identifier, or no inverter
// endpoint, is never applied.
func TestM85WireAliasHoldsOnlyWithinItsRecordedScope(t *testing.T) {
	models, err := os.ReadFile(filepath.Join("../profiles", huaweiSUN2000CatalogDirectory, "models.json"))
	if err != nil {
		t.Fatal(err)
	}
	const recorded = "alias only within exact model ID1006, MAP0 source and inverter endpoint scope"
	if strings.Count(string(models), recorded) == 0 {
		t.Fatal("the recorded alias scope changed")
	}
	alias := m85Case(t, "map0_series_wire_alias_selects_the_ma_v200_card").Input.catalogInput()
	for name, scope := range map[string]string{
		"recorded scope":       recorded,
		"another identifier":   strings.Replace(recorded, "ID1006", "ID1005", 1),
		"no identifier":        strings.Replace(recorded, "ID1006", "model", 1),
		"no inverter endpoint": strings.Replace(recorded, "inverter endpoint", "gateway", 1),
	} {
		read, list := m85CatalogReader(t, map[string]string{"models.json": strings.ReplaceAll(string(models), recorded, scope)})
		entries, err := loadHuaweiSUN2000Catalog(read, list)
		if err != nil {
			t.Fatal(err)
		}
		decision := decideHuaweiSUN2000Catalog(huaweiSUN2000Catalog{entries: entries}, alias)
		row, selected := decision.Row()
		if selected != (name == "recorded scope") || (selected && row.MatchedBy != SUN2000CatalogMatchWireAlias) ||
			(!selected && decision.Reason() != SUN2000CatalogNoCatalogRow) {
			t.Fatalf("%s: %s %s", name, decision.Outcome(), decision.Reason())
		}
	}
	// The alias never stands for its row outside the inverter map.
	for _, scope := range []SUN2000EndpointScope{SUN2000ScopeC81Map, SUN2000ScopeGatewayNamespace} {
		input := alias
		input.Scope = scope
		if decision := m85Decide(t, input); decision.Reason() != SUN2000CatalogScopeMismatch {
			t.Fatalf("%s: %s", scope, decision.Reason())
		}
	}
}

// TestM85SelectionRoutesTheMonitoringProfilesByTheSelectedRow routes the two
// monitoring profiles through the catalog decision: the M3 series by the MC
// row and the M1 series by the MA-C00 row, which the catalog resolves for
// its identifier; a profile keeps reading the groups of its own contract.
func TestM85SelectionRoutesTheMonitoringProfilesByTheSelectedRow(t *testing.T) {
	for _, tc := range []struct {
		input            SUN2000CapabilityInput
		profile, card    string
		identifier       int
		reason           SUN2000CatalogReason
		catalogCandidate []m85Row
	}{
		{h05M3Input(t, 1), SUN2000M3MonitoringProfileID, "MC", 471, SUN2000CatalogAtOrAboveFloor, []m85Row{{"MC", 471}}},
		{h05M1Input(t), SUN2000M1MonitoringProfileID, "MA-C00", 428, SUN2000CatalogAtOrAboveFloor, []m85Row{{"MA-C00", 428}}},
		{h05WithIdentity(h05M3Input(t, 2), h05RoutedIdentity(t, "m3_series_identifier_selects_the_mc_card"), func(i *SUN2000RoutedIdentity) {
			i.SoftwareBlock = h05RoutedModelBlock("V200R025C00SPC100")
		}), SUN2000M3MonitoringProfileID, "MC", 471, SUN2000CatalogNoDocumentedRelation, []m85Row{{"MC", 471}}},
	} {
		selection := h05Select(t, tc.input, tc.profile)
		decision, ok := selection.Catalog()
		row, selected := decision.Row()
		if !ok || !selected || selection.Reason() != SUN2000ReasonMatched || row.Card != tc.card || row.Identifier != tc.identifier ||
			decision.Reason() != tc.reason || decision.Grade() != GradeHypothesis || !reflect.DeepEqual(m85Rows(decision.Candidates()), tc.catalogCandidate) {
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
		input      SUN2000CapabilityInput
		outcome    SUN2000CapabilityOutcome
		reason     SUN2000CapabilityReason
		decided    SUN2000CatalogOutcome
		closed     SUN2000CatalogReason
		candidates []m85Row
	}{
		{h05ObservedM3Input(t, 1), SUN2000AmbiguousMultipleProfiles, SUN2000ReasonCatalogAmbiguous,
			SUN2000CatalogAmbiguous, SUN2000CatalogMultipleCandidateRows, []m85Row{{"MA-C00", 449}, {"MA-C20", 449}, {"MC", 471}}},
		{h05ObservedM1Input(t), SUN2000NoAdmissibleProfile, SUN2000ReasonCatalogNotSelected,
			SUN2000CatalogNotSelected, SUN2000CatalogIdentifierAbsent, []m85Row{{"MA-C00", 428}}},
	} {
		selection := SelectHuaweiSUN2000MonitoringProfile(tc.input)
		decision, ok := selection.Catalog()
		_, profiled := selection.Profile()
		if _, selected := decision.Row(); !ok || selected || profiled || len(selection.Plan()) != 0 || selection.Outcome() != tc.outcome ||
			selection.Reason() != tc.reason || decision.Outcome() != tc.decided || decision.Reason() != tc.closed ||
			!reflect.DeepEqual(m85Rows(decision.Candidates()), tc.candidates) {
			t.Fatalf("model text only: %s %s decision=%s %s %v", selection.Outcome(), selection.Reason(), decision.Outcome(), decision.Reason(), m85Rows(decision.Candidates()))
		}
	}

	// Two rows that the catalog cannot tell apart select nothing and build
	// no observer; a selected row of another family selects no profile.
	m3Identity := h05RoutedIdentity(t, "m3_series_identifier_selects_the_mc_card")
	ambiguous := SelectHuaweiSUN2000MonitoringProfile(h05WithIdentity(h05M3Input(t, 1), m3Identity, func(i *SUN2000RoutedIdentity) { i.IdentifierWords = []uint16{449} }))
	decision, _ := ambiguous.Catalog()
	if ambiguous.Outcome() != SUN2000AmbiguousMultipleProfiles || ambiguous.Reason() != SUN2000ReasonCatalogAmbiguous || len(ambiguous.Plan()) != 0 ||
		!reflect.DeepEqual(m85Rows(decision.Candidates()), []m85Row{{"MA-C00", 449}, {"MA-C20", 449}}) {
		t.Fatalf("449: %s %s %v", ambiguous.Outcome(), ambiguous.Reason(), m85Rows(decision.Candidates()))
	}
	if observer, err := NewSUN2000MonitoringObserver(ambiguous, &h05Session{}); err == nil || observer != nil {
		t.Fatal("an ambiguous catalog decision built an observer")
	}
	alias := h05WithIdentity(h05WithListedModel(t, h05M3Input(t, 6), "SUN2000-12K-MAP0"), h05RoutedIdentity(t, "map0_series_wire_alias_selects_the_ma_v200_card"), nil)
	withoutProfile := SelectHuaweiSUN2000MonitoringProfile(alias)
	decision, _ = withoutProfile.Catalog()
	if row, ok := decision.Row(); withoutProfile.Outcome() != SUN2000NoAdmissibleProfile || withoutProfile.Reason() != SUN2000ReasonCatalogRowNoProfile ||
		!ok || row.Card != "MA-V200" || row.MatchedBy != SUN2000CatalogMatchWireAlias {
		t.Fatalf("MAP0 behind the SmartLogger: %s %s %+v", withoutProfile.Outcome(), withoutProfile.Reason(), row)
	}
	if _, ok := SelectHuaweiSUN2000MonitoringProfile(SUN2000CapabilityInput{}).Catalog(); ok {
		t.Fatal("a decision without gateway evidence")
	}
}

// TestM85DispositionRecordsTheCatalogSelection compares the selection
// records of the SUN2000 disposition with the decisions they name.
func TestM85DispositionRecordsTheCatalogSelection(t *testing.T) {
	record := decodeHuaweiObject(t, "../profiles/vendors/huawei/sun2000-disposition.json")
	detection := huaweiObject(t, record["capability_detection"])
	catalog := huaweiObject(t, detection["catalog_decision"])
	requireHuaweiKeys(t, catalog, "inputs", "candidate_row", "scope", "floor", "outcomes", "selected_grade", "without_identifier",
		"provider_admits_on", "routing", "fixture", "docs_page")
	if catalog["fixture"] != m85FixturePath || catalog["docs_page"] != "protocols/huawei/sun2000-inverter-catalog-v1.md" ||
		catalog["scope"] != string(SUN2000ScopeInverterMap) || catalog["selected_grade"] != string(GradeHypothesis) ||
		catalog["provider_admits_on"] != string(SUN2000CatalogSelected) ||
		!reflect.DeepEqual(huaweiStringSlice(t, catalog["outcomes"]), []string{"SELECTED", "NOT_SELECTED", "AMBIGUOUS"}) {
		t.Fatalf("catalog decision policy: %#v", catalog)
	}
	h03RequireFiles(t, []string{m85FixturePath})
	rows := h03Array(t, record["monitoring_profiles"])
	observed := map[string]string{
		SUN2000M3MonitoringProfileID: "smartlogger_provider_cycles.json", SUN2000M1MonitoringProfileID: "sdongle_provider_cycles.json",
	}
	for _, value := range rows {
		row := huaweiObject(t, value)
		selection := huaweiObject(t, row["catalog_selection"])
		requireHuaweiKeys(t, selection, "decision_case", "outcome", "reason", "grade", "card", "edition", "page", "token", "identifier",
			"matched_by", "wire_alias", "floor", "floor_basis")
		decision := m85Decide(t, m85Case(t, selection["decision_case"].(string)).Input.catalogInput())
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
		requireHuaweiKeys(t, path, "provider_fixture", "admission_reads", "identifier_read", "catalog_outcome", "catalog_reason", "grade",
			"card", "edition", "page", "identifier", "floor", "floor_basis", "candidates")
		provider, _ := h06Load(t, fixtureName)
		_, cycles := h06RunFixture(t, fixtureName)
		child, found := cycles[0].Child(1)
		if !found || child.Catalog == nil || child.ProfileID != id || provider.Path != row["gateway_path"] {
			t.Fatalf("%s: the provider admitted no child of the family: %+v", id, child)
		}
		admitted := *child.Catalog
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
			path["catalog_outcome"] != string(admitted.Outcome()) || path["catalog_reason"] != string(admitted.Reason()) ||
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
	if strings.Contains(string(mustReadHuaweiFile(t, "../profiles/vendors/huawei/sun2000-disposition.json")), "no_consulted_edition") {
		t.Fatal("the MAP0 series still has no consulted edition")
	}
}
