package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const gatewayCatalogueDirectory = catalogueDirectory + "/" + gatewayDirectory

func loadGatewayCommitted(t *testing.T) (*gatewayCatalogue, map[string][]byte) {
	t.Helper()
	c, raw, err := loadGatewayCatalogue(gatewayCatalogueDirectory)
	if err != nil {
		t.Fatal(err)
	}
	return c, raw
}

// gatewayEditionsBy returns the edition records keyed by family and issue
// label, e.g. "SmartLogger Issue 49".
func gatewayEditionsBy(c *gatewayCatalogue) map[string]edition {
	out := map[string]edition{}
	for _, record := range c.Editions.Editions {
		out[record.Family+" "+record.Issue+" "+record.Date] = record
	}
	return out
}

func gatewayRegisters(c *gatewayCatalogue, editionID string) *registersDoc {
	return c.Registers[registersFileName(editionID)]
}

func TestGatewayCatalogueRoundTripsByteForByte(t *testing.T) {
	c, raw := loadGatewayCommitted(t)
	encoded, err := encodeGatewayCatalogue(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != len(raw) || len(raw) != 22 {
		t.Fatalf("%d files encoded, %d committed, want 22", len(encoded), len(raw))
	}
	for name, data := range raw {
		if !bytes.Equal(encoded[name], data) {
			t.Fatalf("%s does not round-trip byte for byte", name)
		}
	}
}

func TestGatewayCatalogueMatchesTheReferenceAccounting(t *testing.T) {
	c, _ := loadGatewayCommitted(t)
	if got := countGatewayCatalogue(c); got != defaultGatewayExpect {
		t.Fatalf("counts %s, want %s", got, defaultGatewayExpect)
	}
	if c.Editions.ReferenceBaseline != "2026-10-26.1" || len(c.Editions.Editions) != 18 {
		t.Fatalf("baseline %q with %d editions", c.Editions.ReferenceBaseline, len(c.Editions.Editions))
	}
	families := map[string]int{}
	rows := map[string]int{}
	for _, record := range c.Editions.Editions {
		families[record.Family]++
		rows[record.Family+" "+record.Issue+" "+record.Date] = record.RegisterRows
	}
	if !reflect.DeepEqual(families, map[string]int{"S-Dongle": 4, "EMMA": 2, "SmartLogger": 12}) {
		t.Fatalf("families %v", families)
	}
	wantRows := map[string]int{
		"S-Dongle Issue 02 2023-04-20": 47, "S-Dongle Issue 01 2025-12-23": 47, "S-Dongle Issue 05 2022-07-06": 47, "S-Dongle Issue 01 2025-04-27": 47,
		"EMMA Issue 01 2024-07-15": 138, "EMMA not stated 2025-08-14": 138,
		"SmartLogger Issue 32 2018-11-15": 176, "SmartLogger Issue 36 2020-04-13": 182, "SmartLogger Issue 37 2020-06-24": 184,
		"SmartLogger Issue 53 2025-10-29": 269, "SmartLogger Issue 54 2026-02-06": 297, "SmartLogger Issue 01 2025-06-23": 194,
		"SmartLogger Issue 52 2025-06-06": 194, "SmartLogger Issue 49 2025-03-19": 262, "SmartLogger Issue 01 2025-12-22": 194,
		"SmartLogger Issue 02 2026-10-09": 332, "SmartLogger Issue 01 2026-10-15": 313,
	}
	// Two R025/R026 Issue 02 editions share an issue and a date; the key
	// above holds the count once, so compare the totals per key.
	for key, want := range wantRows {
		if rows[key] != want {
			t.Fatalf("%s: %d rows, want %d", key, rows[key], want)
		}
	}
	classes := map[string]int{}
	endpoints := map[string]int{}
	for _, doc := range c.Registers {
		for _, row := range append(append([]registerRow(nil), doc.Rows...), doc.Controls...) {
			classes[row.Class]++
			endpoints[row.Endpoint+" "+row.Access]++
		}
	}
	wantClasses := map[string]int{classReviewed: 1275, classConditional: 1170, classRawOnly: 4, classConflict: 115, classReadableConfiguration: 593, classNoSend: 236}
	if !reflect.DeepEqual(classes, wantClasses) {
		t.Fatalf("class totals %v", classes)
	}
	wantEndpoints := map[string]int{
		"gateway-self RO": 1631, "gateway-self RW": 547, "gateway-self WO": 236, "attached-meter RO": 438, "attached-meter RW": 30,
		"attached-charger RO": 22, "attached-environmental-instrument RO": 108, "child-alias RO": 49, "child-alias RW": 16,
		"inventory-counter RO": 12, "remapped-inverter RO": 157, "remapped-environmental-instrument RO": 147,
	}
	if !reflect.DeepEqual(endpoints, wantEndpoints) {
		t.Fatalf("endpoint totals %v", endpoints)
	}
	kinds := map[string]int{}
	tuples, narratives := 0, 0
	for _, item := range c.Dictionaries.Dictionaries {
		kinds[item.Kind]++
		tuples += len(item.Bits)
		narratives += len(item.Narratives)
	}
	if !reflect.DeepEqual(kinds, map[string]int{kindValueEnum: 197, kindAlarmBits: 12}) || tuples != 671 || narratives != 363 {
		t.Fatalf("dictionaries %v with %d alarm tuples and %d narratives", kinds, tuples, narratives)
	}
	for _, item := range c.Dictionaries.Dictionaries {
		if item.Kind == kindValueEnum && containsInt(item.Offsets, 42454) {
			t.Fatalf("%s turns the bit selection at 42454 into a whole-register enumeration", item.ID)
		}
	}
}

func TestGatewayCatalogueSatisfiesEveryInvariant(t *testing.T) {
	c, _ := loadGatewayCommitted(t)
	if err := validateGatewayCatalogue(c); err != nil {
		t.Fatal(err)
	}
	if err := guardGatewayCatalogue(c); err != nil {
		t.Fatal(err)
	}
}

func TestGatewayRowsDecodeOnlyWithinTheirClass(t *testing.T) {
	c, raw := loadGatewayCommitted(t)
	for _, doc := range c.Registers {
		for _, row := range doc.Rows {
			switch row.Class {
			case classRawOnly, classConflict:
				if row.Decode != nil || row.Literal == nil {
					t.Fatalf("%s %s: %s row grants decoding", doc.Card, rowIdentity(row), row.Class)
				}
			case classReviewed, classConditional:
				if row.Decode == nil || row.Literal != nil {
					t.Fatalf("%s %s: %s row lacks its decode fields", doc.Card, rowIdentity(row), row.Class)
				}
			default:
				t.Fatalf("%s %s: class %q on a read-only row", doc.Card, rowIdentity(row), row.Class)
			}
		}
		for _, row := range doc.Controls {
			if row.Decode != nil || row.Literal == nil {
				t.Fatalf("%s %s: a control row decodes", doc.Card, rowIdentity(row))
			}
			if (row.Access == "RW") != (row.Class == classReadableConfiguration) || (row.Access == "WO") != (row.Class == classNoSend) {
				t.Fatalf("%s %s: %s row is %s", doc.Card, rowIdentity(row), row.Access, row.Class)
			}
		}
	}
	for name, data := range raw {
		if !strings.HasPrefix(name, "registers-") {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			nonDecoding := strings.Contains(line, `"class":"raw-only"`) || strings.Contains(line, `"class":"conflict"`) ||
				strings.Contains(line, `"class":"readable-configuration"`) || strings.Contains(line, `"class":"no-send"`)
			if nonDecoding && strings.Contains(line, `"decode":`) {
				t.Fatalf("%s: a non-decoding row carries decode fields: %s", name, line)
			}
		}
	}
}

var gatewayEditionKeys = map[string]bool{
	"id": true, "title": true, "issue": true, "date": true, "family": true, "branch": true, "source_class": true, "card": true,
	"model_rows": true, "models": true, "model_list_exhaustive": true, "floors": true, "floor_pages": true, "self_unit": true,
	"namespaces": true, "register_rows": true,
}

func TestGatewayFloorsAreInclusiveFloorsAndNeverMaxima(t *testing.T) {
	c, raw := loadGatewayCommitted(t)
	var document struct {
		Editions []map[string]json.RawMessage `json:"editions"`
	}
	if err := json.Unmarshal(raw[editionsFile], &document); err != nil {
		t.Fatal(err)
	}
	for index, record := range document.Editions {
		for key := range record {
			if !gatewayEditionKeys[key] {
				t.Fatalf("edition %d carries %q; an edition states floors and nothing above them", index, key)
			}
		}
		var floors []map[string]json.RawMessage
		if err := json.Unmarshal(record["floors"], &floors); err != nil {
			t.Fatal(err)
		}
		for _, floor := range floors {
			if len(floor) != 3 || string(floor["inclusive"]) != "true" {
				t.Fatalf("edition %d floor %v is not an inclusive minimum", index, floor)
			}
		}
	}
	minimum := func(record edition, component string) []string {
		var out []string
		for _, floor := range record.Floors {
			if floor.Component == component {
				out = append(out, floor.Minimum)
			}
		}
		return out
	}
	for _, record := range c.Editions.Editions {
		switch record.Family {
		case "SmartLogger":
			hasV300 := containsString(minimum(record, "SmartLogger"), "V300R001C00")
			if hasV300 == (record.Issue == "Issue 32") || !containsString(minimum(record, "SUN2000"), "V100R001C95") {
				t.Fatalf("%s: SmartLogger floors %+v", record.ID, record.Floors)
			}
		case "S-Dongle":
			want := "V200R022C10"
			if record.Branch == "V100R001C00" {
				want = "V100R001C00SPC100"
			}
			if !reflect.DeepEqual(record.Floors, []softwareFloor{{Component: floorNotStated, Minimum: want, Inclusive: true}}) {
				t.Fatalf("%s: S-Dongle floors %+v", record.ID, record.Floors)
			}
		case "EMMA":
			if len(record.Floors) != 1 || record.Floors[0].Component != "SmartHEMS" || !strings.HasPrefix(record.Floors[0].Minimum, record.Branch[:len("V100R02xC00")]) {
				t.Fatalf("%s: EMMA floors %+v", record.ID, record.Floors)
			}
		}
	}
}

func TestGatewaySelfUnitsFollowTheFamily(t *testing.T) {
	c, _ := loadGatewayCommitted(t)
	services := map[string]serviceEdition{}
	for _, entry := range c.Services.Editions {
		services[entry.Edition] = entry
	}
	for _, record := range c.Editions.Editions {
		want := 0
		if record.Family == "S-Dongle" {
			want = 100
		}
		if *record.SelfUnit != want || services[record.ID].SelfUnit != want {
			t.Fatalf("%s: self unit %d, services %d, want %d", record.ID, *record.SelfUnit, services[record.ID].SelfUnit, want)
		}
	}
}

// Answer A1: 65521 is a public inventory-change counter whose route the
// tables do not establish; it is never a per-child alias.
func TestInventoryCounterIsSeparateFromChildAliases(t *testing.T) {
	c, _ := loadGatewayCommitted(t)
	counters := 0
	for _, record := range c.Editions.Editions {
		found := 0
		for _, row := range gatewayRegisters(c, record.ID).Rows {
			if *row.Offset == 65521 {
				if row.Endpoint != endpointInventoryCounter || row.Class == classConflict {
					t.Fatalf("%s: 65521 sits on %s as %s", record.ID, row.Endpoint, row.Class)
				}
				found++
			}
			if row.Endpoint == endpointInventoryCounter && *row.Offset != 65521 {
				t.Fatalf("%s: the inventory counter endpoint holds %d", record.ID, *row.Offset)
			}
		}
		if (found == 1) != (record.Family == "SmartLogger") || found > 1 {
			t.Fatalf("%s: %d inventory counters", record.ID, found)
		}
		counters += found
	}
	if counters != 12 {
		t.Fatalf("%d inventory counters, want one per SmartLogger edition", counters)
	}
}

// Answer A5: the meter-protocol guard of the source table rides on exactly
// the rows the reference names, as a gate of the attached-meter rows.
func TestMeterProtocolGuardStaysOnTheNamedMeterRows(t *testing.T) {
	c, _ := loadGatewayCommitted(t)
	perEdition := map[string]int{}
	total := 0
	for _, doc := range c.Registers {
		var guard *gate
		for index := range doc.Gates {
			if doc.Gates[index].Kind != "" {
				guard = &doc.Gates[index]
			}
		}
		for _, row := range append(append([]registerRow(nil), doc.Rows...), doc.Controls...) {
			if !hasFlag(row.Gates, "meter-protocol-dl-t-645") {
				continue
			}
			if guard == nil || guard.Kind != "meter_protocol" || guard.Operator != "equals" || guard.Value != "DL/T 645" || guard.Meaning != "DL/T 645 meters only" {
				t.Fatalf("%s: meter-protocol gate %+v", doc.Card, guard)
			}
			if row.Endpoint != endpointAttachedMeter || row.Access != "RO" || row.Class == classReviewed {
				t.Fatalf("%s %s: a guarded row is %s %s %s", doc.Card, rowIdentity(row), row.Endpoint, row.Access, row.Class)
			}
			perEdition[doc.Edition]++
			total++
		}
	}
	editions := gatewayEditionsBy(c)
	want := map[string]int{}
	for _, key := range []string{"SmartLogger Issue 32 2018-11-15", "SmartLogger Issue 36 2020-04-13", "SmartLogger Issue 37 2020-06-24", "SmartLogger Issue 49 2025-03-19"} {
		want[editions[key].ID] = 10
	}
	if total != 40 || !reflect.DeepEqual(perEdition, want) {
		t.Fatalf("%d guarded rows %v", total, perEdition)
	}
	issue49 := gatewayRegisters(c, editions["SmartLogger Issue 49 2025-03-19"].ID)
	for _, row := range issue49.Rows {
		if *row.Offset == 32293 && row.Endpoint == endpointAttachedMeter && !hasFlag(row.Gates, "meter-protocol-dl-t-645") {
			t.Fatal("the Issue 49 meter identifier lost its DL/T 645 guard")
		}
	}
}

func quarantineBySubject(c *gatewayCatalogue, subject string) quarantine {
	for _, item := range c.Quarantines.Quarantines {
		if item.Subject == subject {
			return item
		}
	}
	return quarantine{}
}

func TestGatewayQuarantinesFlagTheNamedConflicts(t *testing.T) {
	c, _ := loadGatewayCommitted(t)
	editions := gatewayEditionsBy(c)
	conflicts := map[string]bool{quarantineWidth64: true, quarantineWidth32: true}
	for subject := range conflictSubjects {
		conflicts[quarantineBySubject(c, subject).ID] = true
	}
	flagged := func(id string) map[string][]int {
		out := map[string][]int{}
		for _, doc := range c.Registers {
			for _, row := range append(append([]registerRow(nil), doc.Rows...), doc.Controls...) {
				if hasFlag(row.Quarantine, id) {
					out[doc.Edition] = append(out[doc.Edition], *row.Offset)
					if conflicts[id] && (row.Class != classConflict || row.Decode != nil) {
						t.Fatalf("%s %s carries %s but is %s", doc.Card, rowIdentity(row), id, row.Class)
					}
				}
			}
		}
		for key := range out {
			sort.Ints(out[key])
		}
		return out
	}
	spc275 := editions["SmartLogger Issue 01 2026-10-15"].ID
	if got := flagged(quarantineWidth64); !reflect.DeepEqual(got, map[string][]int{spc275: {40446, 40450, 40454, 40458}}) {
		t.Fatalf("the 40450 width conflict flags %v", got)
	}
	for _, row := range gatewayRegisters(c, spc275).Rows {
		if hasFlag(row.Quarantine, quarantineWidth64) && (row.Type != "I64" || row.Quantity != 2 || row.Decode != nil) {
			t.Fatalf("%s: width conflict row %+v", spc275, row)
		}
	}
	issue49 := editions["SmartLogger Issue 49 2025-03-19"].ID
	if got := flagged(quarantineWidth32); !reflect.DeepEqual(got, map[string][]int{issue49: {30054, 30058}}) {
		t.Fatalf("the Issue 49 width conflict flags %v", got)
	}
	remap := flagged(quarantineBySubject(c, topicRemapLayouts).ID)
	issue53 := editions["SmartLogger Issue 53 2025-10-29"].ID
	if len(remap) != 2 || len(remap[issue53]) != 12 || len(remap[issue49]) != 12 || remap[issue53][0] != 38000 || remap[issue53][11] != 38105 {
		t.Fatalf("the 38000 remap layout flags %v", remap)
	}
	environment := flagged(quarantineBySubject(c, topicEnvironmentScope).ID)
	envRows := 0
	for _, offsets := range environment {
		envRows += len(offsets)
	}
	if len(environment) != 4 || envRows != 84 {
		t.Fatalf("the environmental remap scope flags %d rows in %d editions", envRows, len(environment))
	}
	if got := flagged(quarantineBySubject(c, topicSerialWidth).ID); !reflect.DeepEqual(got, map[string][]int{issue49: {65510}}) {
		t.Fatalf("the Issue 49 serial width flags %v", got)
	}
	// Answer A2: 37502, 37504 and 37516 are documented in all four S-Dongle
	// editions on page 11; the note binds them, it removes nothing.
	presence := quarantineBySubject(c, topicPresence)
	if !strings.Contains(presence.Meaning, "all four selected maps") || len(presence.Editions) != 4 || len(presence.Affected) != 12 {
		t.Fatalf("presence note %+v", presence)
	}
	for editionID, offsets := range flagged(presence.ID) {
		if !reflect.DeepEqual(offsets, presenceOffsets) || gatewayEditionKey(c, editionID) != "S-Dongle" {
			t.Fatalf("%s: presence rows %v", editionID, offsets)
		}
		for _, row := range gatewayRegisters(c, editionID).Rows {
			if hasFlag(row.Quarantine, presence.ID) && row.Page != presencePage {
				t.Fatalf("%s: presence row on page %d", editionID, row.Page)
			}
		}
	}
	attribute := quarantineBySubject(c, "Attribute 6 grammar of the private device list")
	if attribute.ID != quarantineAttributeSix || len(attribute.Editions) != 18 {
		t.Fatalf("attribute 6 quarantine %+v", attribute)
	}
	for _, entry := range c.Services.Editions {
		if !hasFlag(entry.Quarantine, quarantineAttributeSix) {
			t.Fatalf("%s: services entry lacks the attribute 6 quarantine", entry.Edition)
		}
	}
	if len(c.Quarantines.Quarantines) != 27+6 {
		t.Fatalf("%d quarantines, want the 27 topics of the reference table and six more", len(c.Quarantines.Quarantines))
	}
}

func gatewayEditionKey(c *gatewayCatalogue, editionID string) string {
	for _, record := range c.Editions.Editions {
		if record.ID == editionID {
			return record.Family
		}
	}
	return ""
}

func TestGatewayAlarmCollisionsAreCandidateSets(t *testing.T) {
	c, _ := loadGatewayCommitted(t)
	editions := gatewayEditionsBy(c)
	sets := map[string]map[string][]string{}
	for _, item := range c.Dictionaries.Dictionaries {
		for _, bit := range item.Bits {
			if !hasFlag(bit.Quarantine, quarantineAlarmCollision) {
				continue
			}
			if sets[item.Edition] == nil {
				sets[item.Edition] = map[string][]string{}
			}
			key := itoa(*bit.Offset) + "/" + itoa(bit.Bit)
			sets[item.Edition][key] = append(sets[item.Edition][key], itoa(bit.AlarmID)+"/"+itoa(*bit.Cause))
		}
	}
	issue49 := editions["SmartLogger Issue 49 2025-03-19"].ID
	want := map[string][]string{"50005/13": {"1141/2", "1142/5"}, "50005/14": {"1141/3", "1143/1"}}
	for _, key := range []string{"SmartLogger Issue 53 2025-10-29", "SmartLogger Issue 54 2026-02-06", "SmartLogger Issue 01 2026-10-15"} {
		if !reflect.DeepEqual(sets[editions[key].ID], want) {
			t.Fatalf("%s candidate sets %v", key, sets[editions[key].ID])
		}
	}
	want49 := map[string][]string{"50002/6": {"1122/1", "1123/1"}, "50005/13": {"1141/2", "1142/5"}, "50005/14": {"1141/3", "1143/1"}}
	if !reflect.DeepEqual(sets[issue49], want49) || len(sets) != 4 {
		t.Fatalf("Issue 49 candidate sets %v; %d editions with sets", sets[issue49], len(sets))
	}
	// The same alarm and cause printed twice on one bit is one decode, not a
	// candidate set; the alarm printed at two bits is flagged A02.
	for _, item := range c.Dictionaries.Dictionaries {
		if item.Edition != issue49 || item.Kind != kindAlarmBits {
			continue
		}
		repeated, locations := 0, 0
		for _, bit := range item.Bits {
			if *bit.Offset == 50002 && bit.Bit == 4 {
				repeated++
				if hasFlag(bit.Quarantine, quarantineAlarmCollision) {
					t.Fatal("a repeated identical pair was flagged as a candidate set")
				}
			}
			if hasFlag(bit.Quarantine, quarantineAlarmLocations) {
				locations++
				if bit.AlarmID != 1122 || *bit.Cause != 1 {
					t.Fatalf("A02 flags %d/%d", bit.AlarmID, *bit.Cause)
				}
			}
		}
		if repeated != 2 || locations != 2 {
			t.Fatalf("Issue 49: %d tuples at 50002/4, %d flagged locations", repeated, locations)
		}
	}
}

func TestGatewayDictionariesAreWithheldFromRawOnlyAndConflictRows(t *testing.T) {
	c, _ := loadGatewayCommitted(t)
	withheld := map[string]bool{}
	for _, doc := range c.Registers {
		for _, row := range doc.Rows {
			if row.Class == classRawOnly || row.Class == classConflict {
				withheld[doc.Card+"/"+row.Endpoint+"/"+itoa(*row.Offset)] = true
			}
		}
	}
	for _, item := range c.Dictionaries.Dictionaries {
		if item.Kind != kindValueEnum {
			continue
		}
		for _, offset := range item.Offsets {
			if withheld[item.Card+"/"+item.Endpoint+"/"+itoa(offset)] {
				t.Fatalf("%s describes a raw-only or conflict row at %d", item.ID, offset)
			}
		}
	}
	// A bound row turned into a conflict row must be refused.
	for _, item := range c.Dictionaries.Dictionaries {
		if item.Kind != kindValueEnum {
			continue
		}
		doc := c.Registers[registersFileName(item.Card)]
		for index, row := range doc.Rows {
			if *row.Offset == item.Offsets[0] && row.Endpoint == item.Endpoint {
				saved := doc.Rows[index]
				doc.Rows[index].Class, doc.Rows[index].Literal, doc.Rows[index].Decode = classRawOnly, row.Decode, nil
				err := validateGatewayCatalogue(c)
				doc.Rows[index] = saved
				if err == nil {
					t.Fatalf("%s: a dictionary on a raw-only row was accepted", item.ID)
				}
				return
			}
		}
	}
	t.Fatal("no enumeration found")
}

func TestGatewayServicesCarryExceptionsGrammarAndLimits(t *testing.T) {
	c, _ := loadGatewayCommitted(t)
	editions := map[string]edition{}
	for _, record := range c.Editions.Editions {
		editions[record.ID] = record
	}
	for _, entry := range c.Services.Editions {
		record := editions[entry.Edition]
		codes := strings.Join(entry.ExceptionCodes, ",")
		switch {
		case record.Family == "S-Dongle":
			if codes != "01,02,03,04,06,80" || entry.Attribute6 != "parallel-network integer" || entry.Port502Stated {
				t.Fatalf("%s: %s %q", entry.Edition, codes, entry.Attribute6)
			}
		case record.Family == "EMMA" && record.Date == "2025-08-14":
			if !strings.HasSuffix(codes, ",80,81,A1,A2,B0") || entry.Attribute6 != "feature-version string" || entry.RequestObject != "0x87–0xFF" {
				t.Fatalf("%s: %s %q %s", entry.Edition, codes, entry.Attribute6, entry.RequestObject)
			}
		case record.Family == "SmartLogger" && (record.Issue == "Issue 49" || record.Branch == ""):
			if entry.Attribute6 != "parallel-network integer" {
				t.Fatalf("%s: attribute 6 %q", entry.Edition, entry.Attribute6)
			}
		case record.Family == "SmartLogger":
			if entry.Attribute6 != "feature-version integer" {
				t.Fatalf("%s: attribute 6 %q", entry.Edition, entry.Attribute6)
			}
		}
		if entry.FrameLimitBytes != 256 || entry.ReadQuantity != "1–125" || entry.WriteQuantity != "0–123" || entry.ResponseTimeout != 5 ||
			!reflect.DeepEqual(entry.Functions, []string{"03", "06", "10", "2B/0E"}) {
			t.Fatalf("%s: transport facts %+v", entry.Edition, entry)
		}
	}
	limits := map[string]frameLimit{}
	for _, item := range c.Services.Limits {
		limits[item.ID] = item
	}
	for id, want := range map[string][2]int{"client-read-cap": {123, 255}, "client-write-cap": {121, 255}, "fc03-response-123": {123, 255},
		"fc03-response-124": {124, 257}, "fc03-response-125": {125, 259}, "fc10-request-121": {121, 255}, "fc10-request-123": {123, 259}} {
		if limits[id].Words != want[0] || limits[id].Bytes != want[1] {
			t.Fatalf("limit %s = %+v, want %v", id, limits[id], want)
		}
	}
	if limits["client-read-cap"].Origin != limitClientPolicy || !strings.Contains(limits["client-read-cap"].Rule, "neither block support nor value alignment") ||
		limits["standard-tcp-adu"].Bytes != 260 || limits["vendor-tcp-frame"].Bytes != 256 {
		t.Fatalf("limits %v", limits)
	}
	spacing := map[string]string{}
	for _, entry := range c.Services.Editions {
		var parts []string
		for _, rule := range entry.Spacing {
			parts = append(parts, rule.Between+"="+itoa(rule.MinimumMS)+" "+rule.Basis)
		}
		spacing[editions[entry.Edition].Issue+" "+editions[entry.Edition].Date] = strings.Join(parts, ",")
	}
	for key, want := range map[string]string{
		"Issue 01 2025-12-22": "query=200 required,control=200 required", "Issue 02 2026-10-09": "query=100 required,control=200 required",
		"Issue 53 2025-10-29": "query=100 advisory,control=200 advisory", "Issue 01 2026-10-15": "query=100 advisory,control=200 advisory",
		"Issue 32 2018-11-15": "",
	} {
		if spacing[key] != want {
			t.Fatalf("%s spacing %q, want %q", key, spacing[key], want)
		}
	}
	if len(c.Services.Attributes) != 10 || c.Services.Attributes[5].Meaning != "Edition-dependent" || !strings.Contains(c.Services.DeviceListGrammar, "number=value") {
		t.Fatalf("device-list grammar or attributes %+v", c.Services.Attributes)
	}
}

func TestGatewayTextPassesThePublicationRules(t *testing.T) {
	_, raw := loadGatewayCommitted(t)
	for name, data := range raw {
		text := string(data)
		lower := strings.ToLower(text)
		switch {
		case ageNearRelease(text):
			t.Fatalf("%s places an age word next to a release word", name)
		case hasHexRun(text):
			t.Fatalf("%s carries a digest-like hexadecimal run", name)
		case len(wordSpans(lower, forbiddenWords, true)) > 0 || len(wordSpans(lower, bannedTerms, true)) > 0:
			t.Fatalf("%s carries a forbidden word", name)
		case sourceIDTokenPattern.MatchString(text):
			t.Fatalf("%s names a source identifier of the reference", name)
		case strings.Contains(lower, "engineer") || strings.Contains(lower, "requires licensed"):
			t.Fatalf("%s carries review wording", name)
		}
		for _, marker := range []string{"http:", "https:", "://", "www.", "../", ".pdf", ".md"} {
			if strings.Contains(lower, marker) {
				t.Fatalf("%s carries a link or path marker %q", name, marker)
			}
		}
		for _, profile := range []string{"huawei.smartlogger", "huawei.sdongle", "huawei.emma", "huawei.sun2000"} {
			if strings.Contains(lower, profile) {
				t.Fatalf("%s names a profile identifier; the catalogue registers nothing", name)
			}
		}
	}
}

func TestInverterLoaderSkipsOnlyTheGatewayDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, gatewayDirectory), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadCatalogue(root); err != nil {
		t.Fatalf("the gateway directory was not skipped: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadCatalogue(root); err == nil {
		t.Fatal("an unexpected directory was accepted")
	}
	if _, _, err := loadGatewayCatalogue(root); err == nil {
		t.Fatal("the gateway loader accepted a directory")
	}
}

// Each mutation changes one field of an identity, class or binding the
// validator covers and must be refused.
func TestGatewayValidatorRefusesIdentityAndBindingMutations(t *testing.T) {
	first := func(c *gatewayCatalogue) *registersDoc {
		names := make([]string, 0, len(c.Registers))
		for name := range c.Registers {
			names = append(names, name)
		}
		sort.Strings(names)
		return c.Registers[names[0]]
	}
	for _, mutation := range []struct {
		name   string
		mutate func(c *gatewayCatalogue)
	}{
		{"row key", func(c *gatewayCatalogue) { first(c).Rows[0].Key += "x" }},
		{"row endpoint", func(c *gatewayCatalogue) { first(c).Rows[0].Endpoint = endpointInverter }},
		{"row offset", func(c *gatewayCatalogue) { *first(c).Rows[0].Offset++ }},
		{"row class", func(c *gatewayCatalogue) { first(c).Rows[0].Class = classConflict }},
		{"control class", func(c *gatewayCatalogue) { first(c).Controls[0].Class = classNoSend }},
		{"edition id", func(c *gatewayCatalogue) { c.Editions.Editions[1].ID = c.Editions.Editions[0].ID }},
		{"edition floor", func(c *gatewayCatalogue) { c.Editions.Editions[0].Floors[0].Inclusive = false }},
		{"edition self unit", func(c *gatewayCatalogue) { *c.Editions.Editions[0].SelfUnit = 7 }},
		{"namespace rows", func(c *gatewayCatalogue) { c.Editions.Editions[0].Namespaces[0].Rows++ }},
		{"dropped row flag", func(c *gatewayCatalogue) {
			for _, doc := range c.Registers {
				for index := range doc.Rows {
					if len(doc.Rows[index].Quarantine) > 0 {
						doc.Rows[index].Quarantine = nil
						return
					}
				}
			}
		}},
		{"services self unit", func(c *gatewayCatalogue) { c.Services.Editions[0].SelfUnit = 9 }},
		{"services exception", func(c *gatewayCatalogue) {
			c.Services.Editions[0].ExceptionCodes = append(c.Services.Editions[0].ExceptionCodes, "C1")
		}},
		{"services flag", func(c *gatewayCatalogue) { c.Services.Editions[0].Quarantine = nil }},
		{"alarm flag", func(c *gatewayCatalogue) {
			for index := range c.Dictionaries.Dictionaries {
				for position := range c.Dictionaries.Dictionaries[index].Bits {
					if hasFlag(c.Dictionaries.Dictionaries[index].Bits[position].Quarantine, quarantineAlarmCollision) {
						c.Dictionaries.Dictionaries[index].Bits[position].Quarantine = nil
						return
					}
				}
			}
		}},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			c, _ := loadGatewayCommitted(t)
			mutation.mutate(c)
			if err := validateGatewayCatalogue(c); err == nil {
				t.Fatalf("mutation %q was accepted", mutation.name)
			}
		})
	}
}
