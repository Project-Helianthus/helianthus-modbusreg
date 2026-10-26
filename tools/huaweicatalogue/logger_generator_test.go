package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// gatewayReferenceEnvironment names the opt-in root of the consulted gateway
// reference. The reference is private, so public runs skip the regeneration
// test; the round-trip, count and invariant tests run everywhere.
const gatewayReferenceEnvironment = "HELIANTHUS_HUAWEI_GATEWAY_REFERENCE"

func TestGatewayRegenerationIsByteIdenticalAndDeterministic(t *testing.T) {
	root := os.Getenv(gatewayReferenceEnvironment)
	if root == "" {
		t.Skip(gatewayReferenceEnvironment + " is not set; the consulted gateway reference is not public")
	}
	first, err := generateGateway(gatewayOptions{Reference: root, Expect: defaultGatewayExpect})
	if err != nil {
		t.Fatal(err)
	}
	second, err := generateGateway(gatewayOptions{Reference: root, Expect: defaultGatewayExpect})
	if err != nil {
		t.Fatal(err)
	}
	_, raw := loadGatewayCommitted(t)
	if len(first.Files) != len(raw) {
		t.Fatalf("%d files generated, %d committed", len(first.Files), len(raw))
	}
	for name, data := range first.Files {
		if !bytes.Equal(data, second.Files[name]) {
			t.Fatalf("%s differs between two runs", name)
		}
		if !bytes.Equal(data, raw[name]) {
			t.Fatalf("%s differs from the committed file", name)
		}
	}
	for _, change := range []func(*gatewayCounts){
		func(c *gatewayCounts) { c.RO-- }, func(c *gatewayCounts) { c.Editions++ }, func(c *gatewayCounts) { c.Alarms-- },
	} {
		changed := defaultGatewayExpect
		change(&changed)
		if _, err := generateGateway(gatewayOptions{Reference: root, Expect: changed}); err == nil {
			t.Fatalf("changed counts %s were accepted without -expect", changed)
		}
	}
}

func TestGatewayClassRules(t *testing.T) {
	for _, tc := range []struct {
		access, typ string
		codes       []string
		predicates  int
		note        bool
		want        string
	}{
		{"RW", "U16", nil, 0, false, classReadableConfiguration},
		{"RW", "U16", []string{codeWidth}, 0, false, classNoSend},
		{"WO", "U16", nil, 0, true, classNoSend},
		{"RO", "U32", []string{codeWidth}, 1, true, classConflict},
		{"RO", opaqueLayoutType, nil, 1, true, classRawOnly},
		{"RO", "U16", nil, 1, false, classConditional},
		{"RO", "U16", nil, 0, true, classConditional},
		{"RO", "U16", nil, 0, false, classReviewed},
	} {
		got, err := gatewayClass(tc.access, tc.typ, tc.codes, tc.predicates, tc.note)
		if err != nil || got != tc.want {
			t.Fatalf("%+v: class %q, %v", tc, got, err)
		}
	}
	if _, err := gatewayClass("RX", "U16", nil, 0, false); err == nil {
		t.Fatal("an unknown access was accepted")
	}
}

func TestCodeListReadsOnlyCompleteEnumerations(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"Codes: 0 unavailable; 2 WLAN; 3 4G; 5 WLAN-FE.", []string{"0=unavailable", "2=WLAN", "3=4G", "5=WLAN-FE"}},
		{"0 invalid; 1 operating. One operating inverter is enough.", []string{"0=invalid", "1=operating"}},
		{"Bit 0 exposes the condition: 0 normal; 1 abnormal. Every other bit is reserved.", nil},
		{"State 0 means running; 1 means fault shutdown.", nil},
		{"ESS modes: 1 and 3 reserved; 2 self-consumption priority.", nil},
		{"Record periods: 5, 15, 30 or 60 minutes; source default 5.", nil},
		{"1 selects port 1; 2 selects port 2; other codes are invalid.", nil},
		{"SDongleA-4G only. Traffic state: 0 normal; 1 warning.", nil},
		{"DST state: 0 inactive; 0 active.", nil},
		{"Only command value 0 is accepted.", nil},
		{noInterpretation, nil},
	} {
		var got []string
		for _, value := range codeList(tc.in) {
			got = append(got, value.Code+"="+value.Meaning)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("codeList(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestFloorsAreParsedAsInclusiveMinima(t *testing.T) {
	first, second := "SUN2000 V100R001C95 or later", "V200R022C10 or later"
	type entry = struct {
		Interpretation string `json:"interpretation"`
		Relation       string `json:"relation"`
		Requirement    string `json:"source_requirement"`
	}
	relation := strings.TrimPrefix(second, "V200R022C10 ")
	// The joined statement of the edition must equal its entries.
	var scope twinModelScope
	scope.Minima = []entry{{"floor", relation, first}, {"floor", relation, second}}
	scope.SoftwareRequirements = first + "; " + second + "."
	floors, err := parseFloors(scope)
	want := []softwareFloor{{"SUN2000", "V100R001C95", true}, {floorNotStated, "V200R022C10", true}}
	if err != nil || !reflect.DeepEqual(floors, want) {
		t.Fatalf("floors %+v, %v", floors, err)
	}
	for _, mutate := range []func(*twinModelScope){
		func(s *twinModelScope) { s.Minima[0].Relation = "up to" },
		func(s *twinModelScope) { s.Minima[1].Requirement = "V200R022C10 to V200R025C00" },
		func(s *twinModelScope) { s.SoftwareRequirements = second + "." },
	} {
		changed := scope
		changed.Minima = append(changed.Minima[:0:0], scope.Minima...)
		mutate(&changed)
		if _, err := parseFloors(changed); err == nil {
			t.Fatalf("a non-floor requirement was accepted: %+v", changed)
		}
	}
}

const syntheticGatewayRegister = `{"access":"RO","address":32293,"address_kind":"literal_register_address","alias_target_kind":null,"class":"measurement_or_status",
 "conditions":{"detail_dictionary_id":"D1","namespace":["The operation object is the attached meter."],"row_specific":[{"kind":"meter_protocol","operator":"equals","physical_page":68,"source_id":"SRCX","source_scope":{"namespace":"forwarded_external_meter","ordinal_maximum":25,"ordinal_minimum":16,"this_row_ordinal":18},"summary":"DL/T 645 meters only","value":"DL/T 645"}],"summary":"No additional interpretation is specified in this cell."},
 "default_literal":null,"display_name":"Meter No.","execution_status":"documentary only; runtime access and field-specific write function not established by a successful unrelated read",
 "gain_literal":"N/A","namespace":"forwarded_external_meter","operational_marker":null,"original_reconstruction_namespace":"forwarded_external_meter","quantity_words":6,
 "quarantine":[],"reader_path":"x","record_id":"SRCX-p068-t01-r005","source_category":null,"source_id":"SRCX","source_ordinal":18,
 "source_references":[{"physical_page":68,"row_index":5,"table_index":1}],"source_section":"2.4","source_status_literal":null,"type_literal":"MLD","unit_literal":"N/A"}`

func TestGatewayRegisterConversionKeepsLocatorsPrivate(t *testing.T) {
	var register gatewayRegister
	if err := decodeStrict([]byte(syntheticGatewayRegister), &register); err != nil {
		t.Fatal(err)
	}
	row, err := convertGatewayRegister(register, "SRCX", 3)
	if err != nil {
		t.Fatal(err)
	}
	if row.Row.Key != "attached-meter.32293" || row.Row.Endpoint != endpointAttachedMeter || row.Row.Class != classRawOnly ||
		row.Row.Decode != nil || row.Row.Literal == nil || row.Row.Condition != "" || row.Row.Page != 68 || len(row.RowSpecific) != 1 {
		t.Fatalf("row %+v", row.Row)
	}
	encoded, err := json.Marshal(row.Row)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"Meter No.", "SRCX", "2.4", "\"18\""} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("the committed row carries the locator %q: %s", private, encoded)
		}
	}
	if row.Private["record"] != "SRCX-p068-t01-r005" || row.Private["name"] != "Meter No." || row.Private["ordinal"] != "18" {
		t.Fatalf("private locator %v", row.Private)
	}
	for _, mutation := range []struct{ from, to string }{
		{`"namespace":"forwarded_external_meter","operational_marker"`, `"namespace":"forwarded_inverter","operational_marker"`},
		{`"source_id":"SRCX","source_ordinal"`, `"source_id":"SRCY","source_ordinal"`},
		{`"operational_marker":null`, `"operational_marker":"REVIEW"`},
		{`"class":"measurement_or_status"`, `"class":"configuration_or_control"`},
		{`"quarantine":[]`, `"quarantine":[{"code":"x","status":"resolved"}]`},
		{`"alias_target_kind":null`, `"alias_target_kind":"inverter"`},
	} {
		var changed gatewayRegister
		if err := decodeStrict([]byte(strings.Replace(syntheticGatewayRegister, mutation.from, mutation.to, 1)), &changed); err != nil {
			t.Fatal(err)
		}
		if _, err := convertGatewayRegister(changed, "SRCX", 0); err == nil {
			t.Fatalf("mutation %q was accepted", mutation.to)
		}
	}
	var unknown gatewayRegister
	if err := decodeStrict([]byte(strings.Replace(syntheticGatewayRegister, `"access":"RO"`, `"access":"RO","invented":1`, 1)), &unknown); err == nil {
		t.Fatal("a twin field the generator does not know was accepted")
	}
}

func TestRowQuarantineBindingsCoverEveryTwinCode(t *testing.T) {
	row := func(endpoint, typ string, codes ...string) *gatewayRow {
		return &gatewayRow{parsedRow: parsedRow{Row: registerRow{Endpoint: endpoint, Type: typ}}, Codes: codes}
	}
	for _, tc := range []struct {
		row  *gatewayRow
		want string
	}{
		{row(endpointRemappedInverter, "U32", codeOutsideBlock, codeRemapLayout), topicRemapLayouts},
		{row(endpointRemappedInverter, "U32", codeWidth, codeOutsideBlock, codeRemapLayout), topicRemapLayouts},
		{row(endpointRemappedEnvironmental, "I16", codeEnvApplicability), topicEnvironmentScope},
		{row(endpointChildAlias, "U16", codeWidth), topicSerialWidth},
		{row(endpointGatewaySelf, "I64", codeWidth), quarantineWidth64},
		{row(endpointGatewaySelf, "U32", codeWidth), quarantineWidth32},
	} {
		got, err := rowQuarantines(tc.row)
		if err != nil || !reflect.DeepEqual(got, []string{tc.want}) {
			t.Fatalf("%v on %s: %v %v", tc.row.Codes, tc.row.Row.Endpoint, got, err)
		}
	}
	for _, unbound := range []*gatewayRow{
		row(endpointGatewaySelf, "U16", codeWidth), row(endpointAttachedMeter, "U32", codeWidth),
		row(endpointRemappedInverter, "U32", codeOutsideBlock), row(endpointRemappedInverter, "I16", codeEnvApplicability),
		row(endpointGatewaySelf, "U16", "invented_code"),
		row(endpointRemappedInverter, "U32", codeOutsideBlock, codeRemapLayout, "invented_code"),
	} {
		if _, err := rowQuarantines(unbound); err == nil {
			t.Fatalf("unbound codes %v on %s were accepted", unbound.Codes, unbound.Row.Endpoint)
		}
	}
}

func TestEditionLabelsReplaceSourceIdentifiers(t *testing.T) {
	labels := map[string]string{"GW90": "Family A V100R001C00 Issue 02", "GW91": "Family B Issue 01"}
	got, err := withEditionLabels("occur in GW90 and GW91, physical p.11", labels)
	if err != nil || got != "occur in Family A V100R001C00 Issue 02 and Family B Issue 01, physical p.11" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := withEditionLabels("GW99 differs", labels); err == nil {
		t.Fatal("an unknown source identifier was accepted")
	}
}

func TestGatewayProseDropsTerminologyTheGateRefuses(t *testing.T) {
	term := bannedTerms[0]
	if got := gatewayProse("0 describes the host/" + term + " context; route reconciliation is still necessary."); got != "route reconciliation is still necessary." {
		t.Fatalf("got %q", got)
	}
	if publicationProblem("a "+term+" unit", false) == "" {
		t.Fatal("a refused term passed the publication guard")
	}
}

func TestGatewayCheckModeLeavesTheOtherModeDirectoryAlone(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, gatewayDirectory), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"a.json": []byte("{}\n")}
	var out bytes.Buffer
	if err := emit(&out, root, files, nil, "", true, "x", gatewayDirectory); err != nil {
		t.Fatalf("the nested directory was counted: %v", err)
	}
	if err := emit(&out, root, files, nil, "", true, "x", ""); err == nil {
		t.Fatal("an extra directory was not counted in the gateway mode")
	}
	if err := emit(&out, root, files, nil, "", false, "x", ""); err == nil {
		t.Fatal("the gateway mode wrote into a directory with an entry it does not produce")
	}
	if err := run([]string{"-mode", "sideways", "-reference", root}, &out); err == nil || !strings.Contains(err.Error(), "unknown -mode") {
		t.Fatalf("an unknown mode was accepted: %v", err)
	}
	if _, err := parseGatewayExpect("ro=1,cards=2"); err == nil {
		t.Fatal("an inverter count name was accepted in the gateway mode")
	}
	expect, err := parseGatewayExpect("editions=17,ro=1,controls=2,rows=3,alarms=4")
	if err != nil || expect != (gatewayCounts{Editions: 17, RO: 1, Controls: 2, Rows: 3, Alarms: 4}) {
		t.Fatalf("expect %+v %v", expect, err)
	}
}
