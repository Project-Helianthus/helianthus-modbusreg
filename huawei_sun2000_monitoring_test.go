package modbusreg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

const h05FixtureDirectory = "testdata/huawei/monitoring"

type h05Replaced struct {
	Offset uint16 `json:"offset"`
	Words  int    `json:"words"`
	Class  string `json:"class"`
}

type h05Outcome struct {
	Type  string   `json:"type"`
	Words []uint16 `json:"words,omitempty"`
	Code  uint8    `json:"code,omitempty"`
}

type h05Read struct {
	Function string     `json:"function"`
	Offset   uint16     `json:"offset"`
	Quantity uint16     `json:"quantity"`
	Outcome  h05Outcome `json:"outcome"`
}

type h05Expected struct {
	Key   string `json:"key"`
	Index int    `json:"index"`
	Value string `json:"value"`
	Unit  string `json:"unit"`
	State string `json:"state,omitempty"`
	Grade string `json:"grade"`
}

type h05Reading struct {
	Unit     uint8         `json:"unit"`
	Reads    []h05Read     `json:"reads"`
	Expected []h05Expected `json:"expected"`
}

type h05Fixture struct {
	Schema       string `json:"schema"`
	Kind         string `json:"kind"`
	Family       string `json:"family"`
	GatewayPath  string `json:"gateway_path"`
	ProfileID    string `json:"profile_id"`
	License      string `json:"license"`
	Provenance   string `json:"provenance"`
	Sanitization struct {
		Rule     string        `json:"rule"`
		Replaced []h05Replaced `json:"replaced"`
	} `json:"sanitization"`
	Readings []h05Reading `json:"readings"`
}

func h05FixtureNames() []string {
	return []string{"sun2000_m1_sdongle_monitoring.json", "sun2000_m3_smartlogger_monitoring.json"}
}

func h05FixtureFor(profileID string) string {
	if profileID == HuaweiSUN2000M3MonitoringProfileID {
		return "sun2000_m3_smartlogger_monitoring.json"
	}
	return "sun2000_m1_sdongle_monitoring.json"
}

func h05Load(t testing.TB, name string) (h05Fixture, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h05FixtureDirectory, name))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var fixture h05Fixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if decoder.More() {
		t.Fatalf("%s: trailing content", name)
	}
	return fixture, data
}

func h05Profile(t testing.TB, id string) HuaweiSUN2000MonitoringProfile {
	t.Helper()
	for _, profile := range HuaweiSUN2000MonitoringProfiles() {
		if profile.ID() == id {
			return profile
		}
	}
	t.Fatalf("profile %s not found", id)
	return HuaweiSUN2000MonitoringProfile{}
}

// h05GroupReadings converts the fixture reads that belong to the profile's
// plan into group readings; reads outside the plan stay in the fixture as
// observations only.
func h05GroupReadings(profile HuaweiSUN2000MonitoringProfile, reading h05Reading, cycle uint64) []HuaweiSUN2000GroupReading {
	var out []HuaweiSUN2000GroupReading
	for _, step := range profile.Plan(reading.Unit) {
		for _, read := range reading.Reads {
			if read.Offset != step.Offset || read.Quantity != step.Quantity {
				continue
			}
			group := HuaweiSUN2000GroupReading{Offset: read.Offset, Quantity: read.Quantity, Cycle: cycle}
			switch read.Outcome.Type {
			case "words":
				group.Outcome, group.Words = HuaweiSUN2000GroupAnswered, append([]uint16(nil), read.Outcome.Words...)
			case "exception":
				group.Outcome, group.ExceptionCode, group.Cycle = HuaweiSUN2000GroupException, read.Outcome.Code, 0
			}
			out = append(out, group)
		}
	}
	return out
}

func h05RoutedModelBlock(text string) []byte {
	block := make([]byte, huaweiSUN2000ModelBlockBytes)
	copy(block, text)
	return block
}

// h05M3Input is the M3 series evidence behind the SmartLogger with the routed
// identity words of the decision table, so the catalogue selects the MC row.
func h05M3Input(t testing.TB, unit uint8) HuaweiSUN2000CapabilityInput {
	t.Helper()
	input := h05ObservedM3Input(t, unit)
	input.Identity = h05RoutedIdentity(t, "m3_series_identifier_selects_the_mc_card")
	return input
}

func h05M1Input(t testing.TB) HuaweiSUN2000CapabilityInput {
	t.Helper()
	input := h05ObservedM1Input(t)
	input.Identity = h05RoutedIdentity(t, "m1_series_identifier_selects_the_ma_c00_card")
	return input
}

// h05ObservedM3Input is the evidence of the observed SmartLogger path without
// the routed identity words, the model text only: the catalogue cannot select
// a row without the identifier.
func h05ObservedM3Input(t testing.TB, unit uint8) HuaweiSUN2000CapabilityInput {
	t.Helper()
	gateway := h03ValidInput(t)
	var model string
	for _, probe := range h03LoadProbes(t, "sun2000_routed_identity.json").Probes {
		if probe.Offset == 30000 && probe.Outcome.Type == "text" {
			model = probe.Outcome.Text
		}
	}
	if model == "" {
		t.Fatal("routed model probe missing")
	}
	return HuaweiSUN2000CapabilityInput{SmartLogger: &gateway, UnitID: unit, RoutedModelBlock: h05RoutedModelBlock(model)}
}

// h05ObservedM1Input is the evidence of the observed S-Dongle path without the
// routed identity words.
func h05ObservedM1Input(t testing.TB) HuaweiSUN2000CapabilityInput {
	t.Helper()
	gateway := h15ValidInput(t)
	return HuaweiSUN2000CapabilityInput{SDongle: &gateway, UnitID: 1}
}

// h05RoutedIdentity builds the routed identity words of a decision-table case
// as they would be read: the identifier word, the software and firmware text
// extents and the protocol words.
func h05RoutedIdentity(t testing.TB, name string) HuaweiSUN2000RoutedIdentity {
	t.Helper()
	tc := m85Case(t, name)
	var identity HuaweiSUN2000RoutedIdentity
	if tc.Input.Identifier != nil {
		identity.IdentifierWords = []uint16{*tc.Input.Identifier}
	}
	if tc.Input.SoftwareText != "" {
		identity.SoftwareBlock = h05RoutedModelBlock(tc.Input.SoftwareText)
	}
	if tc.Input.FirmwareText != "" {
		identity.FirmwareBlock = h05RoutedModelBlock(tc.Input.FirmwareText)
	}
	if len(tc.Input.ProtocolWords) != 0 {
		identity.ProtocolWords = append([]uint16(nil), tc.Input.ProtocolWords...)
	}
	return identity
}

func h05Select(t testing.TB, input HuaweiSUN2000CapabilityInput, want string) HuaweiSUN2000CapabilitySelection {
	t.Helper()
	selection := SelectHuaweiSUN2000MonitoringProfile(input)
	profile, ok := selection.Profile()
	if selection.Outcome() != HuaweiSUN2000ProfileSelected || !ok || profile.ID() != want {
		t.Fatalf("selection=%s reason=%s profile=%q want %s", selection.Outcome(), selection.Reason(), profile.ID(), want)
	}
	return selection
}

func h05Decode(t testing.TB, profile HuaweiSUN2000MonitoringProfile, unit uint8, groups []HuaweiSUN2000GroupReading) HuaweiSUN2000MonitoringReading {
	t.Helper()
	reading, err := profile.Decode(unit, groups)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return reading
}

func h05FactValue(t testing.TB, reading HuaweiSUN2000MonitoringReading, key string, index int) HuaweiSUN2000Fact {
	t.Helper()
	fact, ok := reading.Fact(key, index)
	if !ok {
		t.Fatalf("fact %s[%d] missing", key, index)
	}
	return fact
}

func TestH05FixturesDecodeStrictlyRoundTripAndAreSanitized(t *testing.T) {
	entries, err := os.ReadDir(h05FixtureDirectory)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if !reflect.DeepEqual(names, h05FixtureNames()) {
		t.Fatalf("monitoring fixtures=%v want=%v", names, h05FixtureNames())
	}
	walked := map[string]bool{}
	for _, path := range h03CommittedHuaweiTestdata(t) {
		walked[filepath.Base(path)] = true
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			fixture, data := h05Load(t, name)
			encoded, err := json.MarshalIndent(fixture, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(append(encoded, '\n'), data) {
				t.Fatal("fixture does not round-trip byte for byte")
			}
			if fixture.Schema != "helianthus-huawei-monitoring-fixture/v1" || fixture.Kind != "fc03_monitoring_reads" ||
				fixture.License != "CC0-1.0" || !strings.HasPrefix(fixture.Provenance, "synthetic re-creation") || fixture.Sanitization.Rule == "" {
				t.Fatalf("unexpected header: %+v", fixture)
			}
			profile := h05Profile(t, fixture.ProfileID)
			if fixture.Family != profile.Family() || h05FixtureFor(profile.ID()) != name ||
				(profile.GatewayClass() == HuaweiSmartLoggerCanonicalClass) != (fixture.GatewayPath == "SmartLogger") {
				t.Fatalf("fixture %s is not the fixture of %s", name, profile.ID())
			}
			if !walked[name] {
				t.Fatalf("%s is outside the committed shape scan", name)
			}
			if findings := h03ShapeFindings(string(data)); len(findings) != 0 {
				t.Fatalf("%s holds serial-, address- or time-shaped values: %v", name, findings)
			}
			// The serial-derived word and the time words are zero in every reading.
			for _, reading := range fixture.Readings {
				for _, replaced := range fixture.Sanitization.Replaced {
					if replaced.Class == "lifetime_total" {
						continue
					}
					for _, read := range reading.Reads {
						if read.Outcome.Type != "words" || replaced.Offset < read.Offset || replaced.Offset >= read.Offset+read.Quantity {
							continue
						}
						for word := 0; word < replaced.Words; word++ {
							if read.Outcome.Words[int(replaced.Offset-read.Offset)+word] != 0 {
								t.Fatalf("unit %d: replaced %s word at %d is not zero", reading.Unit, replaced.Class, int(replaced.Offset)+word)
							}
						}
					}
				}
			}
		})
	}
	// The scan flags the identity-like values that the replaced ranges hide.
	for kind, text := range map[string]string{
		"capture date": `"observed": "2026-10-25"`,
		"clock time":   `"observed": "06:24:00"`,
		"serial":       `"text": "` + h03CountingDigits(10) + `"`,
	} {
		if len(h03ShapeFindings(text)) == 0 {
			t.Errorf("%s was not flagged", kind)
		}
	}
}

func TestH05EachDecodedFieldEqualsTheFixtureExpectedValue(t *testing.T) {
	cases := []struct {
		name    string
		profile string
		input   func(t testing.TB, unit uint8) HuaweiSUN2000CapabilityInput
		facts   int
	}{
		{"M3 behind SmartLogger", HuaweiSUN2000M3MonitoringProfileID, h05M3Input, 19},
		{"M1 behind S-Dongle", HuaweiSUN2000M1MonitoringProfileID, func(t testing.TB, _ uint8) HuaweiSUN2000CapabilityInput { return h05M1Input(t) }, 22},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture, _ := h05Load(t, h05FixtureFor(tc.profile))
			if len(fixture.Readings) == 0 {
				t.Fatal("no reading")
			}
			for _, fixtureReading := range fixture.Readings {
				selection := h05Select(t, tc.input(t, fixtureReading.Unit), tc.profile)
				profile, _ := selection.Profile()
				reading := h05Decode(t, profile, selection.UnitID(), h05GroupReadings(profile, fixtureReading, 7))
				if len(reading.Facts()) != tc.facts || len(fixtureReading.Expected) != tc.facts {
					t.Fatalf("unit %d: facts=%d expected=%d want %d", fixtureReading.Unit, len(reading.Facts()), len(fixtureReading.Expected), tc.facts)
				}
				for index, fact := range reading.Facts() {
					want := fixtureReading.Expected[index]
					got := h05Expected{Key: fact.Key, Index: fact.Index, Value: fact.Value.String(), Unit: fact.Unit, State: fact.State, Grade: string(fact.Grade)}
					if fact.Availability != HuaweiFactValue || got != want || fact.Cycle != 7 || fact.Retained {
						t.Fatalf("unit %d fact %d: got %+v (%s) want %+v", fixtureReading.Unit, index, got, fact.Availability, want)
					}
				}
			}
		})
	}
}

func TestH05RefusalMapAndReadPlansMatchTheObservation(t *testing.T) {
	published := map[[2]uint16]bool{{32000, 16}: true, {32016, 24}: true, {32064, 32}: true, {32106, 10}: true}
	for _, profile := range HuaweiSUN2000MonitoringProfiles() {
		t.Run(profile.ID(), func(t *testing.T) {
			fixture, _ := h05Load(t, h05FixtureFor(profile.ID()))
			refused := map[[2]uint16]uint8{}
			answered := map[[2]uint16]bool{}
			for _, reading := range fixture.Readings {
				for _, read := range reading.Reads {
					key := [2]uint16{read.Offset, read.Quantity}
					if read.Function != "FC03" {
						t.Fatalf("fixture read %v is not FC03", key)
					}
					switch read.Outcome.Type {
					case "exception":
						refused[key] = read.Outcome.Code
					case "words":
						answered[key] = true
						if len(read.Outcome.Words) != int(read.Quantity) {
							t.Fatalf("read %v carries %d words", key, len(read.Outcome.Words))
						}
					default:
						t.Fatalf("unknown outcome %q", read.Outcome.Type)
					}
				}
			}
			var gotRefused []HuaweiSUN2000RefusedGroup
			for key, code := range refused {
				cause := HuaweiGradeUnknown
				if key == [2]uint16{37100, 38} {
					cause = HuaweiGradeHypothesis
				}
				gotRefused = append(gotRefused, HuaweiSUN2000RefusedGroup{Offset: key[0], Quantity: key[1], ExceptionCode: code, Cause: cause})
			}
			sort.Slice(gotRefused, func(i, j int) bool { return gotRefused[i].Offset < gotRefused[j].Offset })
			if want := profile.RefusedGroups(); len(want) != len(gotRefused) || (len(want) > 0 && !reflect.DeepEqual(want, gotRefused)) {
				t.Fatalf("refusal map=%+v observation=%+v", want, gotRefused)
			}
			for _, unit := range []uint8{1, 2, 247} {
				plan := profile.Plan(unit)
				if len(plan) == 0 {
					t.Fatal("empty plan")
				}
				for _, step := range plan {
					key := [2]uint16{step.Offset, step.Quantity}
					if step.Function != 0x03 || step.UnitID != unit || step.Quantity == 0 || step.Quantity > 125 || !published[key] || !answered[key] {
						t.Fatalf("plan step %+v is not a published, observed and answered FC03 group", step)
					}
					if _, ok := refused[key]; ok {
						t.Fatalf("plan step %+v reads a refused group", step)
					}
				}
			}
			for _, field := range profile.fields {
				width := uint16(1)
				if field.wordType == huaweiSUN2000I32 || field.wordType == huaweiSUN2000U32 {
					width = 2
				}
				if field.offset < field.group.offset || field.offset+width > field.group.offset+field.group.quantity {
					t.Fatalf("field %s[%d] lies outside its group", field.name, field.index)
				}
			}
		})
	}
	m3 := h05Profile(t, HuaweiSUN2000M3MonitoringProfileID).Plan(1)
	m1 := h05Profile(t, HuaweiSUN2000M1MonitoringProfileID).Plan(1)
	if len(m3) != 3 || len(m1) != 4 || m1[2].Offset != 32064 {
		t.Fatalf("plans: M3=%+v M1=%+v", m3, m1)
	}
}

type h05Answer struct {
	words []uint16
	err   error
}

type h05Session struct {
	answers map[uint16]h05Answer
	calls   []string
}

func (s *h05Session) ReadHolding(_ context.Context, unit byte, request modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error) {
	s.calls = append(s.calls, fmt.Sprintf("%d:%d:%d:%d", request.Function(), unit, request.Offset(), request.Quantity()))
	answer, ok := s.answers[request.Offset()]
	if !ok {
		return modbus.ReadRegistersResponse{}, errors.New("timeout")
	}
	if answer.err != nil {
		return modbus.ReadRegistersResponse{}, answer.err
	}
	return modbus.ReadRegistersResponse{
		Words:      append([]uint16(nil), answer.words...),
		Provenance: modbus.ReadProvenance{Function: request.Function(), Table: request.Table(), Offset: request.Offset(), Quantity: request.Quantity()},
	}, nil
}

func h05SessionFromFixture(t testing.TB, profileID string, unit uint8) *h05Session {
	t.Helper()
	fixture, _ := h05Load(t, h05FixtureFor(profileID))
	session := &h05Session{answers: map[uint16]h05Answer{}}
	for _, reading := range fixture.Readings {
		if reading.Unit != unit {
			continue
		}
		for _, read := range reading.Reads {
			if read.Outcome.Type == "words" {
				session.answers[read.Offset] = h05Answer{words: read.Outcome.Words}
			} else {
				session.answers[read.Offset] = h05Answer{err: &modbus.ProtocolError{Kind: modbus.ErrorExceptionResponse, ExceptionCode: read.Outcome.Code}}
			}
		}
	}
	return session
}

type h05CancelSession struct {
	*h05Session
	cancel context.CancelFunc
}

func (s *h05CancelSession) ReadHolding(ctx context.Context, unit byte, request modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error) {
	response, err := s.h05Session.ReadHolding(ctx, unit, request)
	if request.Offset() == 32016 {
		s.cancel()
	}
	return response, err
}

func TestH05CancellationReturnsTheGroupsAlreadyRead(t *testing.T) {
	selection := h05Select(t, h05M3Input(t, 2), HuaweiSUN2000M3MonitoringProfileID)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := &h05CancelSession{h05Session: h05SessionFromFixture(t, HuaweiSUN2000M3MonitoringProfileID, selection.UnitID()), cancel: cancel}
	observer, err := NewHuaweiSUN2000MonitoringObserver(selection, session)
	if err != nil {
		t.Fatal(err)
	}
	reading, err := observer.Observe(ctx, 5)
	if !errors.Is(err, context.Canceled) || reading.ProfileID() != HuaweiSUN2000M3MonitoringProfileID {
		t.Fatalf("profile=%q err=%v", reading.ProfileID(), err)
	}
	if len(reading.Facts()) == 0 {
		t.Fatal("completed string group was discarded")
	}
	if len(session.calls) != 2 {
		t.Fatalf("calls=%v", session.calls)
	}
}

type h05InFlightCancelSession struct {
	*h05Session
	cancel context.CancelFunc
}

func (s *h05InFlightCancelSession) ReadHolding(ctx context.Context, unit byte, request modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error) {
	if request.Offset() == 32016 {
		s.calls = append(s.calls, "inflight")
		s.cancel()
		return modbus.ReadRegistersResponse{}, ctx.Err()
	}
	return s.h05Session.ReadHolding(ctx, unit, request)
}

func TestH05InFlightCancellationReturnsTheContextError(t *testing.T) {
	selection := h05Select(t, h05M3Input(t, 2), HuaweiSUN2000M3MonitoringProfileID)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := &h05InFlightCancelSession{h05Session: h05SessionFromFixture(t, HuaweiSUN2000M3MonitoringProfileID, selection.UnitID()), cancel: cancel}
	observer, err := NewHuaweiSUN2000MonitoringObserver(selection, session)
	if err != nil {
		t.Fatal(err)
	}
	reading, err := observer.Observe(ctx, 5)
	if !errors.Is(err, context.Canceled) || reading.ProfileID() != HuaweiSUN2000M3MonitoringProfileID {
		t.Fatalf("profile=%q err=%v", reading.ProfileID(), err)
	}
}

func TestH05ObserverSessionHasNoWritePathAndReadsOnlyThePlan(t *testing.T) {
	sessionType := reflect.TypeOf((*HuaweiSUN2000MonitoringSession)(nil)).Elem()
	if sessionType.NumMethod() != 1 || sessionType.Method(0).Name != "ReadHolding" {
		t.Fatalf("session interface exposes %d methods", sessionType.NumMethod())
	}
	observerType := reflect.TypeOf(&HuaweiSUN2000MonitoringObserver{})
	for index := 0; index < observerType.NumMethod(); index++ {
		if name := observerType.Method(index).Name; name != "Observe" {
			t.Fatalf("observer exposes %s", name)
		}
	}
	for _, typ := range []reflect.Type{
		reflect.TypeOf(HuaweiSUN2000MonitoringProfile{}), reflect.TypeOf(HuaweiSUN2000CapabilitySelection{}),
		reflect.TypeOf(HuaweiSUN2000MonitoringReading{}), observerType,
	} {
		for index := 0; index < typ.NumMethod(); index++ {
			name := typ.Method(index).Name
			if name == "WriteAuthority" || name == "SupportClaim" || name == "CatalogRegistered" || name == "LiveQualified" || name == "AutomaticRuntimeAdmission" {
				if reflect.ValueOf(reflect.Zero(typ).Interface()).MethodByName(name).Call(nil)[0].Bool() {
					t.Fatalf("%s.%s is true", typ, name)
				}
				continue
			}
			for _, verb := range []string{"Write", "Send", "Control", "Set", "Command"} {
				if strings.HasPrefix(name, verb) {
					t.Fatalf("%s exposes %s", typ, name)
				}
			}
		}
	}

	for _, tc := range []struct {
		profile string
		input   HuaweiSUN2000CapabilityInput
		want    []string
	}{
		{HuaweiSUN2000M3MonitoringProfileID, h05M3Input(t, 2), []string{"3:2:32000:16", "3:2:32016:24", "3:2:32106:10"}},
		{HuaweiSUN2000M1MonitoringProfileID, h05M1Input(t), []string{"3:1:32000:16", "3:1:32016:24", "3:1:32064:32", "3:1:32106:10"}},
	} {
		selection := h05Select(t, tc.input, tc.profile)
		session := h05SessionFromFixture(t, tc.profile, selection.UnitID())
		observer, err := NewHuaweiSUN2000MonitoringObserver(selection, session)
		if err != nil {
			t.Fatal(err)
		}
		reading, err := observer.Observe(context.Background(), 3)
		if err != nil || !reflect.DeepEqual(session.calls, tc.want) || reading.ProfileID() != tc.profile {
			t.Fatalf("%s: calls=%v err=%v", tc.profile, session.calls, err)
		}
		for _, fact := range reading.Facts() {
			if fact.Availability != HuaweiFactValue || fact.Cycle != 3 {
				t.Fatalf("%s: observed fact %+v", tc.profile, fact)
			}
		}
	}

	for name, selection := range map[string]HuaweiSUN2000CapabilitySelection{
		"insufficient": SelectHuaweiSUN2000MonitoringProfile(HuaweiSUN2000CapabilityInput{}),
		"ambiguous":    SelectHuaweiSUN2000MonitoringProfile(h05AmbiguousInput(t)),
	} {
		if observer, err := NewHuaweiSUN2000MonitoringObserver(selection, &h05Session{}); err == nil || observer != nil {
			t.Fatalf("%s selection built an observer", name)
		}
	}
	if _, err := NewHuaweiSUN2000MonitoringObserver(h05Select(t, h05M1Input(t), HuaweiSUN2000M1MonitoringProfileID), nil); err == nil {
		t.Fatal("observer without a session")
	}
	var nilObserver *HuaweiSUN2000MonitoringObserver
	if _, err := nilObserver.Observe(context.Background(), 1); err == nil {
		t.Fatal("nil observer observed")
	}
}

// h05WithListedModel rewrites the model attribute of the listed child at the
// unit, keeping the entry length, so the listing agrees with a routed read.
func h05WithListedModel(t testing.TB, input HuaweiSUN2000CapabilityInput, model string) HuaweiSUN2000CapabilityInput {
	t.Helper()
	gateway := *input.SmartLogger
	pages := make([]HuaweiSmartLoggerObservedInventoryPage, len(gateway.Inventory.Pages))
	rewritten := 0
	for pageIndex, page := range gateway.Inventory.Pages {
		page.Objects = append([]HuaweiSmartLoggerInventoryObject(nil), page.Objects...)
		for objectIndex, object := range page.Objects {
			attributes := strings.Split(string(bytes.TrimRight(object.Value, "\x00 ")), ";")
			if !reflect.DeepEqual(huaweiFilter(attributes, "5="), []string{fmt.Sprintf("5=%d", input.UnitID)}) {
				continue
			}
			for index, attribute := range attributes {
				if strings.HasPrefix(attribute, "1=") {
					attributes[index] = "1=" + model
				}
			}
			text := strings.Join(attributes, ";")
			if len(text) > len(object.Value) {
				t.Fatalf("model %q does not fit the entry", model)
			}
			value := make([]byte, len(object.Value))
			copy(value, text)
			page.Objects[objectIndex].Value = value
			rewritten++
		}
		pages[pageIndex] = page
	}
	if rewritten != 1 {
		t.Fatalf("rewrote %d entries for unit %d", rewritten, input.UnitID)
	}
	gateway.Inventory.Pages = pages
	input.SmartLogger = &gateway
	input.RoutedModelBlock = h05RoutedModelBlock(model)
	return input
}

func huaweiFilter(values []string, prefix string) []string {
	var out []string
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			out = append(out, value)
		}
	}
	return out
}

// h05AmbiguousInput supplies both observed gateway identities for one unit,
// each admitting its profile on its observed path.
func h05AmbiguousInput(t testing.TB) HuaweiSUN2000CapabilityInput {
	input := h05ObservedM3Input(t, 1)
	sdongle := h15ValidInput(t)
	input.SDongle = &sdongle
	return input
}

// h05WithIdentity replaces the routed identity words; mutate edits a copy.
func h05WithIdentity(input HuaweiSUN2000CapabilityInput, identity HuaweiSUN2000RoutedIdentity, mutate func(*HuaweiSUN2000RoutedIdentity)) HuaweiSUN2000CapabilityInput {
	identity.IdentifierWords = append([]uint16(nil), identity.IdentifierWords...)
	identity.SoftwareBlock = append([]byte(nil), identity.SoftwareBlock...)
	identity.FirmwareBlock = append([]byte(nil), identity.FirmwareBlock...)
	identity.ProtocolWords = append([]uint16(nil), identity.ProtocolWords...)
	for _, part := range []*[]byte{&identity.SoftwareBlock, &identity.FirmwareBlock} {
		if len(*part) == 0 {
			*part = nil
		}
	}
	if len(identity.IdentifierWords) == 0 {
		identity.IdentifierWords = nil
	}
	if len(identity.ProtocolWords) == 0 {
		identity.ProtocolWords = nil
	}
	if mutate != nil {
		mutate(&identity)
	}
	input.Identity = identity
	return input
}

// h05RowInputs builds the executable cases of every decision-table row.
func h05RowInputs(t testing.TB) map[string][]HuaweiSUN2000CapabilityInput {
	m3 := func(mutate func(*HuaweiSUN2000CapabilityInput)) HuaweiSUN2000CapabilityInput {
		input := h05M3Input(t, 1)
		mutate(&input)
		return input
	}
	m1 := func(mutate func(*HuaweiSUN2000CapabilityInput)) HuaweiSUN2000CapabilityInput {
		input := h05M1Input(t)
		mutate(&input)
		return input
	}
	m3Identity := h05RoutedIdentity(t, "m3_series_identifier_selects_the_mc_card")
	m1Identity := h05RoutedIdentity(t, "m1_series_identifier_selects_the_ma_c00_card")
	m3With := func(unit uint8, model string, mutate func(*HuaweiSUN2000RoutedIdentity)) HuaweiSUN2000CapabilityInput {
		input := h05M3Input(t, unit)
		if model != huaweiSUN2000M3Model {
			input = h05WithListedModel(t, input, model)
		}
		return h05WithIdentity(input, m3Identity, mutate)
	}
	identifier := func(value uint16) func(*HuaweiSUN2000RoutedIdentity) {
		return func(i *HuaweiSUN2000RoutedIdentity) { i.IdentifierWords = []uint16{value} }
	}
	map0 := h05RoutedIdentity(t, "map0_series_wire_alias_selects_the_ma_v200_card")
	return map[string][]HuaweiSUN2000CapabilityInput{
		"no_identity_evidence": {{}, {UnitID: 1}, {UnitID: 1, Identity: m3Identity}},
		"gateway_identity_not_matched": {
			m3(func(i *HuaweiSUN2000CapabilityInput) {
				gateway := *i.SmartLogger
				gateway.UnitID = 1
				i.SmartLogger = &gateway
			}),
			m1(func(i *HuaweiSUN2000CapabilityInput) {
				gateway := *i.SDongle
				gateway.Gateway.UnitID = 1
				i.SDongle = &gateway
			}),
		},
		"gateway_inventory_child_model_only": {m3(func(i *HuaweiSUN2000CapabilityInput) { i.RoutedModelBlock = nil })},
		"unit_not_routed": {
			m3(func(i *HuaweiSUN2000CapabilityInput) { i.UnitID = 0 }),
			m3(func(i *HuaweiSUN2000CapabilityInput) { i.UnitID = 10 }),
			m3(func(i *HuaweiSUN2000CapabilityInput) { i.UnitID = 248 }),
			m1(func(i *HuaweiSUN2000CapabilityInput) { i.UnitID = 2 }),
			m1(func(i *HuaweiSUN2000CapabilityInput) { i.UnitID = 100 }),
		},
		"serial_or_esn_only": {m3(func(i *HuaweiSUN2000CapabilityInput) {
			block := make([]byte, 20)
			copy(block, h03Joined("XX", "0000000000"))
			i.RoutedModelBlock = block
		})},
		"register_readability_only": {
			m3(func(i *HuaweiSUN2000CapabilityInput) { i.RoutedModelBlock = make([]byte, huaweiSUN2000ModelBlockBytes) }),
			m3(func(i *HuaweiSUN2000CapabilityInput) {
				i.RoutedModelBlock = h05RoutedModelBlock(huaweiSUN2000M3Model)
				i.RoutedModelBlock[25] = 'X'
			}),
		},
		"listing_model_conflict": {
			m3(func(i *HuaweiSUN2000CapabilityInput) { i.UnitID = 9 }),
			m3(func(i *HuaweiSUN2000CapabilityInput) { i.UnitID = 11 }),
		},
		"identity_words_invalid": {
			m3With(1, huaweiSUN2000M3Model, func(i *HuaweiSUN2000RoutedIdentity) { i.IdentifierWords = []uint16{} }),
			m3With(1, huaweiSUN2000M3Model, func(i *HuaweiSUN2000RoutedIdentity) { i.IdentifierWords = []uint16{471, 0} }),
			m3With(1, huaweiSUN2000M3Model, func(i *HuaweiSUN2000RoutedIdentity) { i.SoftwareBlock = i.SoftwareBlock[:28] }),
			m3With(1, huaweiSUN2000M3Model, func(i *HuaweiSUN2000RoutedIdentity) { i.SoftwareBlock[27] = 'X' }),
			m3With(1, huaweiSUN2000M3Model, func(i *HuaweiSUN2000RoutedIdentity) { i.FirmwareBlock = make([]byte, huaweiSUN2000ModelBlockBytes) }),
			h05WithIdentity(h05M1Input(t), m1Identity, func(i *HuaweiSUN2000RoutedIdentity) { i.ProtocolWords = []uint16{5} }),
		},
		"model_prefix_or_wildcard": {
			h05WithListedModel(t, h05ObservedM3Input(t, 1), "SUN2000-50KTL"),
			h05WithListedModel(t, h05ObservedM3Input(t, 2), huaweiSUN2000M3Model+"X"),
			h05WithListedModel(t, h05ObservedM3Input(t, 3), "SUN2000-50KTL-M*"),
			m3With(1, "SUN2000-50KTL", nil),
			m3With(2, huaweiSUN2000M3Model+"X", nil),
		},
		"model_behind_another_gateway_path": {
			h05WithListedModel(t, h05ObservedM3Input(t, 1), huaweiSDongleObservedChildModel),
			h05WithIdentity(h05WithListedModel(t, h05M3Input(t, 1), huaweiSDongleObservedChildModel), m1Identity, nil),
		},
		"family_without_catalogue_row": {
			h05WithListedModel(t, h05ObservedM3Input(t, 4), "SUN2000-00K-MAP0"),
			m3With(4, "SUN2000-00K-MAP0", nil),
		},
		"catalogue_row_without_profile_on_path": {
			m3With(5, "SUN2000-50KTL-NHM3", identifier(472)),
			h05WithIdentity(h05WithListedModel(t, h05M3Input(t, 6), "SUN2000-12K-MAP0"), map0, nil),
		},
		"catalogue_identifier_mismatch": {
			m3With(1, huaweiSUN2000M3Model, identifier(470)),
			m3With(1, huaweiSUN2000M3Model, identifier(472)),
			h05WithIdentity(h05M1Input(t), m1Identity, identifier(471)),
		},
		"catalogue_software_malformed_or_absent": {
			m3With(1, huaweiSUN2000M3Model, func(i *HuaweiSUN2000RoutedIdentity) { i.SoftwareBlock = h05RoutedModelBlock("V200R023C0") }),
			m3With(1, huaweiSUN2000M3Model, func(i *HuaweiSUN2000RoutedIdentity) { i.SoftwareBlock = nil }),
		},
		"catalogue_rows_not_told_apart": {m3With(1, huaweiSUN2000M3Model, identifier(449))},
		"multiple_positive_candidates":  {h05AmbiguousInput(t)},
		"catalogue_m3_row_behind_smartlogger": {
			h05M3Input(t, 1), h05M3Input(t, 2), h05M3Input(t, 8),
			m3With(1, huaweiSUN2000M3Model, func(i *HuaweiSUN2000RoutedIdentity) { i.SoftwareBlock = h05RoutedModelBlock("V200R025C00SPC100") }),
		},
		"catalogue_m1_row_behind_sdongle":          {h05M1Input(t)},
		"token_without_identifier_in_several_rows": {h05ObservedM3Input(t, 1), h05ObservedM3Input(t, 2), h05ObservedM3Input(t, 8)},
		"token_without_identifier_in_one_row":      {h05ObservedM1Input(t)},
	}
}

func TestH05CapabilityDecisionTableRowsAreExecutable(t *testing.T) {
	record := decodeHuaweiObject(t, "profiles/vendor/huawei/sun2000-disposition.json")
	detection := huaweiObject(t, record["capability_detection"])
	if detection["executable"] != true || detection["first_match_priority"] != false {
		t.Fatalf("detection policy: %#v", detection)
	}
	inputs := h05RowInputs(t)
	seen := map[string]bool{}
	for _, value := range h03Array(t, detection["rows"]) {
		row := huaweiObject(t, value)
		id := row["id"].(string)
		cases, ok := inputs[id]
		if !ok || len(cases) == 0 {
			t.Fatalf("row %q has no executable case", id)
		}
		seen[id] = true
		for index, input := range cases {
			selection := SelectHuaweiSUN2000MonitoringProfile(input)
			profile, selected := selection.Profile()
			if string(selection.Outcome()) != row["outcome"] || profile.ID() != row["selected_profile"] || selected != (row["selected_profile"] != "") {
				t.Fatalf("row %q case %d: outcome=%s reason=%s profile=%q want %v %v", id, index, selection.Outcome(), selection.Reason(), profile.ID(), row["outcome"], row["selected_profile"])
			}
			facts := 0
			if selected {
				facts = len(profile.fields)
			}
			if facts != huaweiInt(t, row["native_fact_count"]) || len(selection.Plan()) != map[bool]int{true: len(profile.groups), false: 0}[selected] {
				t.Fatalf("row %q case %d: facts=%d plan=%d", id, index, facts, len(selection.Plan()))
			}
			if selection.BatteryProfileSelected() || selection.WriteAuthority() || selection.SupportClaim() || selection.AutomaticRequestCount() != 0 {
				t.Fatalf("row %q case %d grants authority", id, index)
			}
		}
	}
	if len(seen) != len(inputs) {
		t.Fatalf("rows=%v cases=%d", seen, len(inputs))
	}
}

// TestH05MutatedTupleSelectsNoProfile varies every field of the selection key
// one at a time: the gateway tuple (unit, inventory, model block at the child),
// the routed unit, each character of the routed model text on the catalogue
// and the observed path, and each routed identity word: the identifier value
// and shape, the software text shape and the protocol word shape.
func TestH05MutatedTupleSelectsNoProfile(t *testing.T) {
	type mutation struct {
		name  string
		input HuaweiSUN2000CapabilityInput
	}
	var mutations []mutation
	for position := 0; position < len(huaweiSUN2000M3Model); position++ {
		model := []byte(huaweiSUN2000M3Model)
		model[position] ^= 0x01
		for name, base := range map[string]HuaweiSUN2000CapabilityInput{"catalogue": h05M3Input(t, 1), "observed": h05ObservedM3Input(t, 1)} {
			input := base
			input.RoutedModelBlock = h05RoutedModelBlock(string(model))
			mutations = append(mutations, mutation{fmt.Sprintf("M3 model character %d on the %s path", position, name), input})
		}
	}
	for position := 0; position < len(huaweiSDongleObservedChildModel); position++ {
		input := h05M1Input(t)
		gateway := *input.SDongle
		block := append([]byte(nil), gateway.Child.ModelBlock...)
		block[position] ^= 0x01
		gateway.Child.ModelBlock = block
		input.SDongle = &gateway
		mutations = append(mutations, mutation{fmt.Sprintf("M1 child model character %d", position), input})
	}
	for _, base := range []struct {
		name     string
		input    HuaweiSUN2000CapabilityInput
		identity HuaweiSUN2000RoutedIdentity
	}{
		{"M3", h05M3Input(t, 1), h05RoutedIdentity(t, "m3_series_identifier_selects_the_mc_card")},
		{"M1", h05M1Input(t), h05RoutedIdentity(t, "m1_series_identifier_selects_the_ma_c00_card")},
	} {
		value := base.identity.IdentifierWords[0]
		for _, other := range []uint16{value ^ 0x0001, value + 1, value - 1, 0, 0xffff} {
			other := other
			mutations = append(mutations, mutation{fmt.Sprintf("%s identifier %d", base.name, other),
				h05WithIdentity(base.input, base.identity, func(i *HuaweiSUN2000RoutedIdentity) { i.IdentifierWords = []uint16{other} })})
		}
		for name, mutate := range map[string]func(*HuaweiSUN2000RoutedIdentity){
			"identifier word count": func(i *HuaweiSUN2000RoutedIdentity) { i.IdentifierWords = append(i.IdentifierWords, 0) },
			"software text shape":   func(i *HuaweiSUN2000RoutedIdentity) { i.SoftwareBlock[len(i.SoftwareBlock)-1] = 'X' },
			"software text form":    func(i *HuaweiSUN2000RoutedIdentity) { i.SoftwareBlock = h05RoutedModelBlock("V000R000C0") },
			"software text absent":  func(i *HuaweiSUN2000RoutedIdentity) { i.SoftwareBlock = nil },
			"protocol word count":   func(i *HuaweiSUN2000RoutedIdentity) { i.ProtocolWords = []uint16{5, 0, 0} },
		} {
			mutations = append(mutations, mutation{base.name + " " + name, h05WithIdentity(base.input, base.identity, mutate)})
		}
	}
	smartLogger := h05M3Input(t, 1)
	gateway := *smartLogger.SmartLogger
	gateway.Inventory.ChangeCounterAfter++
	smartLogger.SmartLogger = &gateway
	mutations = append(mutations, mutation{"SmartLogger counter bracket", smartLogger})
	sdongle := h05M1Input(t)
	sdongleGateway := *sdongle.SDongle
	sdongleGateway.Child.UnitID = 2
	sdongle.SDongle = &sdongleGateway
	mutations = append(mutations, mutation{"S-Dongle child unit", sdongle})
	for _, tc := range mutations {
		selection := SelectHuaweiSUN2000MonitoringProfile(tc.input)
		if _, ok := selection.Profile(); ok || selection.Outcome() == HuaweiSUN2000ProfileSelected || len(selection.Plan()) != 0 {
			t.Fatalf("%s selected %s", tc.name, selection.Outcome())
		}
	}
	if len(mutations) < 2*len(huaweiSUN2000M3Model)+len(huaweiSDongleObservedChildModel)+2*10+2 {
		t.Fatalf("only %d mutations", len(mutations))
	}
}

func TestH05AbsentBatteryGroupYieldsUnknownNeverZeroAndMetersStayUnknown(t *testing.T) {
	for _, profile := range HuaweiSUN2000MonitoringProfiles() {
		fixture, _ := h05Load(t, h05FixtureFor(profile.ID()))
		reading := fixture.Readings[0]
		zeroWords := 0
		for _, read := range reading.Reads {
			if read.Offset != 37000 && read.Offset != 47000 {
				continue
			}
			if read.Outcome.Type != "words" || !reflect.DeepEqual(read.Outcome.Words, []uint16{0}) {
				t.Fatalf("%s: battery read %+v is not the observed zero word", profile.ID(), read)
			}
			zeroWords++
			// A battery offset is outside every plan: supplying it is refused.
			if _, err := profile.Decode(reading.Unit, []HuaweiSUN2000GroupReading{{Offset: read.Offset, Quantity: read.Quantity, Outcome: HuaweiSUN2000GroupAnswered, Words: []uint16{0}}}); err == nil {
				t.Fatalf("%s: battery word decoded", profile.ID())
			}
		}
		if zeroWords != 2 {
			t.Fatalf("%s: %d battery zero words", profile.ID(), zeroWords)
		}
		decoded := h05Decode(t, profile, reading.Unit, h05GroupReadings(profile, reading, 1))
		for _, fact := range decoded.Facts() {
			for _, token := range []string{"battery", "soc", "charge", "meter", "grid"} {
				if strings.Contains(fact.Key, token) {
					t.Fatalf("%s: fact %s", profile.ID(), fact.Key)
				}
			}
		}
		if profile.BatteryFactsAdmitted() || profile.MeterFactsAdmitted() || decoded.BatteryFactsAdmitted() || decoded.MeterFactsAdmitted() {
			t.Fatalf("%s admits battery or meter facts", profile.ID())
		}
		// An unread group is Unknown, never zero.
		empty := h05Decode(t, profile, reading.Unit, nil)
		for _, fact := range empty.Facts() {
			if fact.Availability != HuaweiFactUnknown || fact.Value != (HuaweiDecimal{}) || fact.Grade != HuaweiGradeUnknown || fact.State != "" {
				t.Fatalf("%s: unread fact %+v", profile.ID(), fact)
			}
		}
	}
	// The inverter-exposed meter group stays refused on the M3 series.
	refused := h05Profile(t, HuaweiSUN2000M3MonitoringProfileID).RefusedGroups()
	if refused[1] != (HuaweiSUN2000RefusedGroup{Offset: 37100, Quantity: 38, ExceptionCode: 3, Cause: HuaweiGradeHypothesis}) {
		t.Fatalf("meter refusal: %+v", refused)
	}
}

func TestH05LunaWithoutIdentifiedParentSelectsNothing(t *testing.T) {
	for name, input := range map[string]HuaweiSUN2000CapabilityInput{
		"parent unidentified": {},
		"parent M3 selected":  h05M3Input(t, 1),
		"parent M1 selected":  h05M1Input(t),
	} {
		if SelectHuaweiSUN2000MonitoringProfile(input).BatteryProfileSelected() {
			t.Fatalf("%s selected a battery profile", name)
		}
	}
	record := decodeHuaweiObject(t, "profiles/vendor/huawei/luna2000-disposition.json")
	if record["outcome"] != "NO_ADMISSIBLE_PROFILE" || len(h03Array(t, record["decoder_keys"])) != 0 {
		t.Fatalf("LUNA2000 disposition: %#v", record["outcome"])
	}
	for _, source := range []string{"huawei_sun2000_monitoring.go", "huawei_sun2000_capability.go"} {
		if strings.Contains(strings.ToLower(string(mustReadHuaweiFile(t, source))), "huawei.luna2000") {
			t.Fatalf("%s names a LUNA2000 profile", source)
		}
	}
}

func TestH05OptionalGroupExceptionRetainsLastKnownGood(t *testing.T) {
	profile := h05Profile(t, HuaweiSUN2000M1MonitoringProfileID)
	fixture, _ := h05Load(t, h05FixtureFor(profile.ID()))
	first := h05Decode(t, profile, 1, h05GroupReadings(profile, fixture.Readings[0], 10))

	// Cycle 11: the power group is refused, the other groups answer with new words.
	groups := h05GroupReadings(profile, fixture.Readings[0], 11)
	for index := range groups {
		switch groups[index].Offset {
		case 32064:
			groups[index] = HuaweiSUN2000GroupReading{Offset: 32064, Quantity: 32, Outcome: HuaweiSUN2000GroupException, ExceptionCode: 3}
		case 32016:
			groups[index].Words[0] = 6000
		}
	}
	second := h05Decode(t, profile, 1, groups)
	if fact := h05FactValue(t, second, "huawei.sun2000.m1.active_power", 0); fact.Availability != HuaweiFactUnknown {
		t.Fatalf("refused group produced %+v", fact)
	}
	merged := RetainHuaweiSUN2000Monitoring(first, second)
	power := h05FactValue(t, merged, "huawei.sun2000.m1.active_power", 0)
	voltage := h05FactValue(t, merged, "huawei.sun2000.m1.pv_string_voltage", 1)
	if power.Availability != HuaweiFactValue || power.Value.String() != "0.466" || power.Cycle != 10 || !power.Retained ||
		voltage.Value.String() != "600.0" || voltage.Cycle != 11 || voltage.Retained {
		t.Fatalf("retention: power=%+v voltage=%+v", power, voltage)
	}
	if groups := merged.Groups(); groups[2].Outcome != HuaweiSUN2000GroupAnswered || !groups[2].Retained || groups[2].Cycle != 10 {
		t.Fatalf("retained group: %+v", groups[2])
	}

	// A failed read ends the unit's cycle; later groups are not read and the
	// earlier complete readings are kept with their age.
	session := h05SessionFromFixture(t, profile.ID(), 1)
	delete(session.answers, 32016)
	observer, err := NewHuaweiSUN2000MonitoringObserver(h05Select(t, h05M1Input(t), profile.ID()), session)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := observer.Observe(context.Background(), 12)
	if err != nil || len(session.calls) != 2 {
		t.Fatalf("failed cycle: calls=%v err=%v", session.calls, err)
	}
	outcomes := []HuaweiSUN2000GroupOutcome{}
	for _, group := range failed.Groups() {
		outcomes = append(outcomes, group.Outcome)
	}
	if !reflect.DeepEqual(outcomes, []HuaweiSUN2000GroupOutcome{HuaweiSUN2000GroupAnswered, HuaweiSUN2000GroupFailed, HuaweiSUN2000GroupNotRead, HuaweiSUN2000GroupNotRead}) {
		t.Fatalf("outcomes=%v", outcomes)
	}
	kept := RetainHuaweiSUN2000Monitoring(first, failed)
	for _, fact := range kept.Facts() {
		if fact.Availability != HuaweiFactValue || fact.Cycle != 10 || !fact.Retained {
			t.Fatalf("kept fact %+v", fact)
		}
	}

	// Identity fencing: nothing migrates from another profile or unit.
	m3 := h05Profile(t, HuaweiSUN2000M3MonitoringProfileID)
	m3Fixture, _ := h05Load(t, h05FixtureFor(m3.ID()))
	other := h05Decode(t, m3, 1, h05GroupReadings(m3, m3Fixture.Readings[0], 10))
	otherUnit := h05Decode(t, m3, 2, h05GroupReadings(m3, m3Fixture.Readings[1], 10))
	emptyM3 := h05Decode(t, m3, 1, nil)
	for name, previous := range map[string]HuaweiSUN2000MonitoringReading{"other profile": first, "other unit": otherUnit} {
		for _, fact := range RetainHuaweiSUN2000Monitoring(previous, emptyM3).Facts() {
			if fact.Availability != HuaweiFactUnknown {
				t.Fatalf("%s migrated %+v", name, fact)
			}
		}
	}
	if fact := h05FactValue(t, RetainHuaweiSUN2000Monitoring(other, emptyM3), "huawei.sun2000.m3.energy_accumulated", 0); fact.Value.String() != "42000.00" || !fact.Retained {
		t.Fatalf("same identity did not retain: %+v", fact)
	}
}

func TestH05SignConventionAndUnobservedStateCodes(t *testing.T) {
	profile := h05Profile(t, HuaweiSUN2000M1MonitoringProfileID)
	fixture, _ := h05Load(t, h05FixtureFor(profile.ID()))
	withPower := func(mutate func(words []uint16)) HuaweiSUN2000MonitoringReading {
		groups := h05GroupReadings(profile, fixture.Readings[0], 1)
		for index := range groups {
			if groups[index].Offset == 32064 {
				mutate(groups[index].Words)
			}
		}
		return h05Decode(t, profile, 1, groups)
	}
	for _, tc := range []struct {
		name         string
		high, low    uint16
		availability HuaweiFactAvailability
		value        string
	}{
		{"positive while producing", 0, 466, HuaweiFactValue, "0.466"},
		{"zero", 0, 0, HuaweiFactValue, "0.000"},
		{"largest positive", 0x7fff, 0xffff, HuaweiFactValue, "2147483.647"},
		{"negative is outside the observed range", 0xffff, 0xfe2e, HuaweiFactUnknown, "0"},
		{"most negative", 0x8000, 0x0000, HuaweiFactUnknown, "0"},
	} {
		reading := withPower(func(words []uint16) { words[16], words[17] = tc.high, tc.low })
		fact := h05FactValue(t, reading, "huawei.sun2000.m1.active_power", 0)
		if fact.Availability != tc.availability || fact.Value.String() != tc.value || fact.Unit != "kW" {
			t.Fatalf("%s: %+v", tc.name, fact)
		}
		if tc.availability == HuaweiFactUnknown && fact.Grade != HuaweiGradeUnknown {
			t.Fatalf("%s: unknown fact graded %s", tc.name, fact.Grade)
		}
	}
	// Input power and string values are two's complement with no sign claim.
	input := h05FactValue(t, withPower(func(words []uint16) { words[0], words[1] = 0xffff, 0xffff }), "huawei.sun2000.m1.input_power", 0)
	if input.Value.String() != "-0.001" || input.Availability != HuaweiFactValue {
		t.Fatalf("input power: %+v", input)
	}
	groups := h05GroupReadings(profile, fixture.Readings[0], 1)
	groups[1].Words[0], groups[1].Words[1] = 0xfff6, 0xffff
	stringReading := h05Decode(t, profile, 1, groups)
	if v, i := h05FactValue(t, stringReading, "huawei.sun2000.m1.pv_string_voltage", 1), h05FactValue(t, stringReading, "huawei.sun2000.m1.pv_string_current", 1); v.Value.String() != "-1.0" || i.Value.String() != "-0.01" {
		t.Fatalf("two's complement strings: %+v %+v", v, i)
	}

	for _, code := range []uint16{0x0000, 0x0001, 0x0100, 0x0201, 0x0202, 0x0300, 0x0307, 0x0500, 0xa000, 0x01ff, 0x0210, 0xffff} {
		reading := withPower(func(words []uint16) { words[25] = code })
		fact := h05FactValue(t, reading, "huawei.sun2000.m1.device_status", 0)
		if fact.Availability != HuaweiFactUnknown || fact.State != "" || fact.Value != (HuaweiDecimal{}) {
			t.Fatalf("code %#04x: %+v", code, fact)
		}
		if retained := reading.Groups()[2].Words[25]; retained != code {
			t.Fatalf("code %#04x not retained as native evidence: %#04x", code, retained)
		}
	}
	observed := h05FactValue(t, withPower(func([]uint16) {}), "huawei.sun2000.m1.device_status", 0)
	if observed.State != "grid_connected" || observed.Value.String() != "512" || observed.Grade != HuaweiGradeHypothesis {
		t.Fatalf("observed code: %+v", observed)
	}
}

func TestH05GradesFollowTheContractAtObservedAndOtherUnits(t *testing.T) {
	profile := h05Profile(t, HuaweiSUN2000M3MonitoringProfileID)
	fixture, _ := h05Load(t, h05FixtureFor(profile.ID()))
	reading := fixture.Readings[0]
	for _, unit := range []uint8{3, 8} {
		h05Select(t, h05M3Input(t, unit), profile.ID())
		decoded := h05Decode(t, profile, unit, h05GroupReadings(profile, reading, 1))
		for _, fact := range decoded.Facts() {
			if fact.Grade != HuaweiGradeHypothesis {
				t.Fatalf("unit %d: %s[%d] graded %s; only units 1 and 2 were checked", unit, fact.Key, fact.Index, fact.Grade)
			}
		}
	}
	proven := 0
	for _, fact := range h05Decode(t, profile, 1, h05GroupReadings(profile, reading, 1)).Facts() {
		if fact.Grade == HuaweiGradeProven {
			proven++
		}
	}
	if proven != 9 {
		t.Fatalf("M3 proven facts at unit 1: %d, want the eight string voltages and the accumulated energy", proven)
	}
}

func TestH05EveryConsumedWordChangesOnlyItsOwnFact(t *testing.T) {
	for _, profile := range HuaweiSUN2000MonitoringProfiles() {
		fixture, _ := h05Load(t, h05FixtureFor(profile.ID()))
		base := h05GroupReadings(profile, fixture.Readings[0], 1)
		reference := h05Decode(t, profile, fixture.Readings[0].Unit, base).Facts()
		for groupIndex, group := range base {
			for word := range group.Words {
				mutated := make([]HuaweiSUN2000GroupReading, len(base))
				for index := range base {
					mutated[index] = base[index].clone()
				}
				mutated[groupIndex].Words[word] ^= 0x0101
				facts := h05Decode(t, profile, fixture.Readings[0].Unit, mutated).Facts()
				var changed []string
				for index := range facts {
					if facts[index] != reference[index] {
						changed = append(changed, fmt.Sprintf("%s[%d]", facts[index].Key, facts[index].Index))
					}
				}
				var want []string
				offset := group.Offset + uint16(word)
				for _, field := range profile.fields {
					width := uint16(1)
					if field.wordType == huaweiSUN2000I32 || field.wordType == huaweiSUN2000U32 {
						width = 2
					}
					if field.group.offset == group.Offset && offset >= field.offset && offset < field.offset+width {
						want = append(want, fmt.Sprintf("%s[%d]", profile.decoderKey(field), field.index))
					}
				}
				if !reflect.DeepEqual(changed, want) {
					t.Fatalf("%s offset %d: changed=%v want=%v", profile.ID(), offset, changed, want)
				}
			}
		}
	}
}

func TestH05DecodeRefusesReadingsOutsideThePlan(t *testing.T) {
	profile := h05Profile(t, HuaweiSUN2000M3MonitoringProfileID)
	answered := func(offset, quantity uint16) HuaweiSUN2000GroupReading {
		return HuaweiSUN2000GroupReading{Offset: offset, Quantity: quantity, Outcome: HuaweiSUN2000GroupAnswered, Words: make([]uint16, quantity)}
	}
	for name, readings := range map[string][]HuaweiSUN2000GroupReading{
		"refused power group":  {answered(32064, 32)},
		"candidate extent 31":  {answered(32064, 31)},
		"candidate extent 33":  {answered(32064, 33)},
		"meter group":          {answered(37100, 38)},
		"shifted strings":      {answered(32017, 24)},
		"longer strings":       {answered(32016, 25)},
		"repeated group":       {answered(32016, 24), answered(32016, 24)},
		"short words":          {{Offset: 32016, Quantity: 24, Outcome: HuaweiSUN2000GroupAnswered, Words: make([]uint16, 23)}},
		"exception with words": {{Offset: 32016, Quantity: 24, Outcome: HuaweiSUN2000GroupException, ExceptionCode: 3, Words: []uint16{1}}},
		"exception code zero":  {{Offset: 32016, Quantity: 24, Outcome: HuaweiSUN2000GroupException}},
		"unknown outcome":      {{Offset: 32016, Quantity: 24, Outcome: "PARTIAL", Words: make([]uint16, 24)}},
		"failed with words":    {{Offset: 32016, Quantity: 24, Outcome: HuaweiSUN2000GroupFailed, Words: make([]uint16, 24)}},
	} {
		if _, err := profile.Decode(1, readings); err == nil {
			t.Fatalf("%s was decoded", name)
		}
	}
	for _, unit := range []uint8{0, 248} {
		if _, err := profile.Decode(unit, nil); err == nil {
			t.Fatalf("unit %d was decoded", unit)
		}
	}
	if _, err := (HuaweiSUN2000MonitoringProfile{}).Decode(1, nil); err == nil {
		t.Fatal("zero profile decoded")
	}
}

func TestH05ExactDecimalText(t *testing.T) {
	for _, tc := range []struct {
		value HuaweiDecimal
		want  string
	}{
		{HuaweiDecimal{6128, -1}, "612.8"}, {HuaweiDecimal{0, -2}, "0.00"}, {HuaweiDecimal{5, -3}, "0.005"},
		{HuaweiDecimal{-1, -3}, "-0.001"}, {HuaweiDecimal{-12345, -2}, "-123.45"}, {HuaweiDecimal{512, 0}, "512"},
		{HuaweiDecimal{7, 2}, "700"}, {HuaweiDecimal{4294967295, -2}, "42949672.95"}, {HuaweiDecimal{-2147483648, -3}, "-2147483.648"},
	} {
		if got := tc.value.String(); got != tc.want {
			t.Fatalf("%+v rendered %q want %q", tc.value, got, tc.want)
		}
	}
}

// TestH05NegativeOverlapWithTheGatewayClasses evaluates the inverter profiles
// at the gateway units and the gateway cards against inverter evidence.
func TestH05NegativeOverlapWithTheGatewayClasses(t *testing.T) {
	// The profiles never select at a gateway's own unit.
	smartLoggerUnit := h05M3Input(t, 0)
	sdongleUnit := h05M1Input(t)
	sdongleUnit.UnitID = huaweiSDongleObservedGatewayUnit
	for name, input := range map[string]HuaweiSUN2000CapabilityInput{"SmartLogger unit 0": smartLoggerUnit, "S-Dongle unit 100": sdongleUnit} {
		if selection := SelectHuaweiSUN2000MonitoringProfile(input); selection.Outcome() != HuaweiSUN2000InsufficientEvidence || selection.Reason() != HuaweiSUN2000ReasonUnitNotRouted {
			t.Fatalf("%s: %s %s", name, selection.Outcome(), selection.Reason())
		}
	}
	// The gateway cards never match inverter evidence.
	smartLogger := h03ValidInput(t)
	smartLogger.UnitID = 1
	if NewHuaweiSmartLoggerObservedQualificationCard().Evaluate(smartLogger).IdentityMatched() {
		t.Fatal("the SmartLogger card matched at an inverter unit")
	}
	sdongle := h15ValidInput(t)
	sdongle.Gateway.UnitID = huaweiSDongleObservedChildUnit
	if NewHuaweiSDongleObservedQualificationCard().Evaluate(sdongle).IdentityMatched() {
		t.Fatal("the S-Dongle card matched at the inverter child unit")
	}
	for _, model := range []string{huaweiSUN2000M3Model, huaweiSDongleObservedChildModel} {
		if NewHuaweiEMMAQualificationCard().Evaluate(HuaweiEMMAQualificationInput{UnitID: 1, Offering: model, Model: model}).IdentityMatched() {
			t.Fatalf("the EMMA card matched the inverter model %s", model)
		}
	}
	// No EMMA path input exists: the MAP0 series behind the EMMA has no profile.
	inputType := reflect.TypeOf(HuaweiSUN2000CapabilityInput{})
	for index := 0; index < inputType.NumField(); index++ {
		if strings.Contains(strings.ToUpper(inputType.Field(index).Name), "EMMA") {
			t.Fatal("an EMMA path selects a SUN2000 profile")
		}
	}
}

func TestH05ProfilesAgreeWithTheDisposition(t *testing.T) {
	record := decodeHuaweiObject(t, "profiles/vendor/huawei/sun2000-disposition.json")
	rows := h03Array(t, record["monitoring_profiles"])
	profiles := HuaweiSUN2000MonitoringProfiles()
	if len(rows) != len(profiles)+1 {
		t.Fatalf("monitoring rows=%d", len(rows))
	}
	var keys []string
	typeNames := map[huaweiSUN2000WordType]string{huaweiSUN2000I16: "I16", huaweiSUN2000U16: "U16", huaweiSUN2000I32: "I32", huaweiSUN2000U32: "U32"}
	gains := map[int16]float64{0: 1, -1: 10, -2: 100, -3: 1000}
	for index, profile := range profiles {
		row := huaweiObject(t, rows[index])
		document := huaweiObject(t, row["document"])
		if row["profile_id"] != profile.ID() || row["family"] != profile.Family() || row["model"] != profile.Model() ||
			row["identity_card"] != profile.IdentityCard() || row["outcome"] != "OFFLINE_PROFILE_ADMITTED" ||
			row["edition_binding"] != string(profile.EditionBinding()) || row["fixture"] != h05FixtureDirectory+"/"+h05FixtureFor(profile.ID()) ||
			document["title"] != profile.Document().Title || document["version"] != profile.Document().Version || document["date"] != profile.Document().Date ||
			len(h03Array(t, row["missing_evidence"])) != 0 {
			t.Fatalf("row %d disagrees with %s: %#v", index, profile.ID(), row)
		}
		h03RequireFiles(t, []string{row["fixture"].(string)})
		var units []uint8
		for _, unit := range h03Array(t, row["observed_units"]) {
			units = append(units, uint8(huaweiInt(t, unit)))
		}
		if !reflect.DeepEqual(units, profile.ObservedUnits()) {
			t.Fatalf("%s observed units %v", profile.ID(), units)
		}
		var plan []HuaweiSUN2000ReadStep
		for _, value := range h03Array(t, row["read_plan"]) {
			step := huaweiObject(t, value)
			plan = append(plan, HuaweiSUN2000ReadStep{Function: 3, UnitID: 1, Offset: uint16(huaweiInt(t, step["offset"])), Quantity: uint16(huaweiInt(t, step["quantity"]))})
			if want := map[bool]string{true: "RETAINED_NO_FACTS", false: "CONSUMED"}[step["offset"] == json.Number("32000")]; step["use"] != want {
				t.Fatalf("%s plan row %#v", profile.ID(), step)
			}
		}
		if !reflect.DeepEqual(plan, profile.Plan(1)) {
			t.Fatalf("%s plan %v want %v", profile.ID(), plan, profile.Plan(1))
		}
		var refused []HuaweiSUN2000RefusedGroup
		for _, value := range h03Array(t, row["refused_groups"]) {
			group := huaweiObject(t, value)
			refused = append(refused, HuaweiSUN2000RefusedGroup{Offset: uint16(huaweiInt(t, group["offset"])), Quantity: uint16(huaweiInt(t, group["quantity"])),
				ExceptionCode: uint8(huaweiInt(t, group["exception"])), Cause: HuaweiEvidenceGrade(group["cause"].(string))})
		}
		if !reflect.DeepEqual(refused, profile.RefusedGroups()) {
			t.Fatalf("%s refused %v want %v", profile.ID(), refused, profile.RefusedGroups())
		}
		var fields []string
		for _, field := range profile.fields {
			if field.index > 1 {
				continue
			}
			stride, count := 0, 1
			if field.index == 1 {
				stride, count = 2, huaweiSUN2000StringCount
			}
			fields = append(fields, fmt.Sprintf("%s %d %d %d %s %g %s %s", field.name, field.offset, stride, count, typeNames[field.wordType], gains[field.exponent], field.unit, field.grade))
		}
		var recorded []string
		for _, value := range h03Array(t, row["facts"]) {
			fact := huaweiObject(t, value)
			recorded = append(recorded, fmt.Sprintf("%s %d %d %d %s %d %s %s", fact["key"], huaweiInt(t, fact["offset"]), huaweiInt(t, fact["stride"]),
				huaweiInt(t, fact["count"]), fact["type"], huaweiInt(t, fact["gain"]), fact["unit"], fact["grade"]))
		}
		if !reflect.DeepEqual(recorded, fields) {
			t.Fatalf("%s facts\n%v\nwant\n%v", profile.ID(), recorded, fields)
		}
		keys = append(keys, profile.DecoderKeys()...)
	}
	if !reflect.DeepEqual(huaweiStringSlice(t, record["decoder_keys"]), keys) {
		t.Fatalf("decoder keys %v want %v", record["decoder_keys"], keys)
	}
	mapZero := huaweiObject(t, rows[len(rows)-1])
	requireHuaweiKeys(t, mapZero, "family", "gateway_path", "outcome", "reason", "catalogue_selection", "missing_evidence")
	if mapZero["family"] != "SUN2000 MAP0 series" || mapZero["gateway_path"] != "EMMA" || mapZero["outcome"] != "NO_ADMISSIBLE_PROFILE" ||
		mapZero["reason"] != "catalogue_card_without_monitoring_profile" || !reflect.DeepEqual(huaweiStringSlice(t, mapZero["missing_evidence"]), []string{"catalogue_monitoring_profile"}) {
		t.Fatalf("MAP0 row: %#v", mapZero)
	}
	for _, profile := range profiles {
		if strings.Contains(profile.Family(), "MAP0") {
			t.Fatal("a MAP0 profile exists")
		}
	}
	gate := huaweiObject(t, record["version_gate"])
	if gate["cause"] != "UNKNOWN" || gate["fail_closed"] != true ||
		!reflect.DeepEqual(huaweiStringSlice(t, gate["refused_on"]), []string{huaweiSUN2000M3Family, "SUN2000 MAP0 series"}) ||
		!reflect.DeepEqual(huaweiStringSlice(t, gate["answered_on"]), []string{huaweiSUN2000M1Family}) {
		t.Fatalf("version gate: %#v", gate)
	}
	for _, value := range h03Array(t, gate["candidate_extents"]) {
		extent := huaweiObject(t, value)
		for _, profile := range profiles {
			for _, step := range profile.Plan(1) {
				if step.Offset == uint16(huaweiInt(t, extent["offset"])) && step.Quantity == uint16(huaweiInt(t, extent["quantity"])) {
					t.Fatalf("candidate extent %#v is planned", extent)
				}
			}
		}
	}
}
