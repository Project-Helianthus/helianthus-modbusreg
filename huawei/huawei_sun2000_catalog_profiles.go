package huawei

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/profiles"
)

// CatalogMonitoringProfile is the monitoring profile generated from one
// catalogue card. Catalogue facts are documentary, so their grade is
// Hypothesis. The profile sends nothing.
type CatalogMonitoringProfile struct {
	id      string
	card    string
	family  string
	prefix  string
	strings int
	groups  []catalogGroup
	fields  []catalogField
	states  map[uint16]string
	alarms  map[uint16]map[int]string
}

// CatalogMonitoringReading is one decoded catalogue profile.
type CatalogMonitoringReading struct {
	facts  []SUN2000Fact
	groups []SUN2000GroupReading
	losses []ProviderLoss
}

type catalogKind uint8

const (
	catalogString catalogKind = iota + 1
	catalogPower
	catalogEnergy
	catalogStatus
	catalogAlarm
	catalogTracker
	catalogRating
)

type catalogGroup struct {
	offset, quantity uint16
}

type catalogField struct {
	name     string
	index    int
	offset   uint16
	width    uint16
	signed   bool
	exponent int16
	unit     string
	kind     catalogKind
}

type catalogRegisterFile struct {
	Card string               `json:"card"`
	Rows []catalogRegisterRow `json:"rows"`
}

type catalogRegisterRow struct {
	Key             string       `json:"key"`
	Endpoint        string       `json:"endpoint"`
	Offset          *int         `json:"offset"`
	Quantity        int          `json:"quantity"`
	Type            string       `json:"type"`
	Access          string       `json:"access"`
	Class           string       `json:"class"`
	FunctionalClass string       `json:"functional_class"`
	Decode          *catalogCell `json:"decode"`
}

type catalogCell struct {
	Gain string `json:"gain"`
	Unit string `json:"unit"`
}

type catalogDictFile struct {
	Dictionaries []catalogDict `json:"dictionaries"`
}

type catalogDict struct {
	Kind    string            `json:"kind"`
	Card    string            `json:"card"`
	Offsets []int             `json:"offsets"`
	Values  []catalogDictCode `json:"values"`
	Bits    []catalogDictBit  `json:"bits"`
}

type catalogDictCode struct {
	Code    string `json:"code"`
	Meaning string `json:"meaning"`
}

type catalogDictBit struct {
	Offset *int   `json:"offset"`
	Bit    int    `json:"bit"`
	Label  string `json:"label"`
}

type catalogBundle struct {
	profiles []CatalogMonitoringProfile
}

var (
	catalogOnce   sync.Once
	catalogLoaded catalogBundle

	catalogStringNumber  = regexp.MustCompile(`(?i)pv_channel(\d+)`)
	catalogDirectChannel = regexp.MustCompile(`(?i)dc_channel_(\d+)`)
	catalogStringShort   = regexp.MustCompile(`(?i)pv(\d+)`)
	catalogTrackerNumber = regexp.MustCompile(`(?i)mppt_?(\d+)`)
)

// ID returns the profile identifier. The two hand families keep theirs.
func (p CatalogMonitoringProfile) ID() string { return p.id }

// Card returns the catalogue card key.
func (p CatalogMonitoringProfile) Card() string { return p.card }

// Family returns the family name the profile serves.
func (p CatalogMonitoringProfile) Family() string { return p.family }

// IDPrefix returns the decoder-key prefix, including its trailing dot.
func (p CatalogMonitoringProfile) IDPrefix() string { return p.prefix }

// StringCount returns the number of documented string voltage channels.
func (p CatalogMonitoringProfile) StringCount() int { return p.strings }

// Plan returns the FC03 reads of the profile at one unit.
func (p CatalogMonitoringProfile) Plan(unit uint8) []SUN2000ReadStep {
	steps := make([]SUN2000ReadStep, 0, len(p.groups))
	for _, group := range p.groups {
		steps = append(steps, SUN2000ReadStep{Function: uint8(modbusreg.FunctionReadHoldingRegisters), UnitID: unit, Offset: group.offset, Quantity: group.quantity})
	}
	return steps
}

// Facts returns a copy of the decoded facts.
func (r CatalogMonitoringReading) Facts() []SUN2000Fact {
	return append([]SUN2000Fact(nil), r.facts...)
}

// Fact returns the fact with the decoder key and index.
func (r CatalogMonitoringReading) Fact(key string, index int) (SUN2000Fact, bool) {
	for _, fact := range r.facts {
		if fact.Key == key && fact.Index == index {
			return fact, true
		}
	}
	return SUN2000Fact{}, false
}

// Groups returns copies of the group readings in plan order.
func (r CatalogMonitoringReading) Groups() []SUN2000GroupReading {
	out := make([]SUN2000GroupReading, len(r.groups))
	for index, group := range r.groups {
		out[index] = group.clone()
	}
	return out
}

// Losses returns the unit losses recorded for this reading.
func (r CatalogMonitoringReading) Losses() []ProviderLoss {
	return append([]ProviderLoss(nil), r.losses...)
}

// CatalogMonitoringProfiles returns one profile per catalogue card, ordered
// by card key.
func CatalogMonitoringProfiles() []CatalogMonitoringProfile {
	catalogOnce.Do(func() {
		catalogLoaded = catalogLoad()
	})
	return append([]CatalogMonitoringProfile(nil), catalogLoaded.profiles...)
}

// Decode validates the group readings and decodes the fields they carry.
// A group the unit refuses is a loss for this unit. The other groups still
// decode. A group outside the plan, a repeated group, or a short reply is
// refused as a whole.
func (p CatalogMonitoringProfile) Decode(unit uint8, readings []SUN2000GroupReading) (CatalogMonitoringReading, error) {
	if p.id == "" || unit == 0 || unit > huaweiSUN2000MaxUnit {
		return CatalogMonitoringReading{}, errHuaweiSUN2000ReadingInvalid
	}
	groups := make([]SUN2000GroupReading, len(p.groups))
	for index, group := range p.groups {
		groups[index] = SUN2000GroupReading{Offset: group.offset, Quantity: group.quantity, Outcome: SUN2000GroupNotRead}
	}
	seen := make([]bool, len(p.groups))
	var losses []ProviderLoss
	for _, reading := range readings {
		position := catalogGroupPosition(p.groups, reading)
		if position < 0 || seen[position] || !validHuaweiSUN2000GroupReading(reading) {
			return CatalogMonitoringReading{}, errHuaweiSUN2000ReadingInvalid
		}
		seen[position] = true
		groups[position] = reading.clone()
		if reading.Outcome == SUN2000GroupException {
			losses = append(losses, ProviderLoss{Kind: ProviderLossRefusedGroup, HasUnit: true, UnitID: unit,
				Offset: reading.Offset, Quantity: reading.Quantity, Detail: huaweiRuntimeRefusalDetail(reading.ExceptionCode)})
		}
	}
	return CatalogMonitoringReading{facts: p.catalogFacts(groups), groups: groups, losses: losses}, nil
}

func catalogGroupPosition(groups []catalogGroup, reading SUN2000GroupReading) int {
	for index, group := range groups {
		if reading.Offset == group.offset && reading.Quantity == group.quantity {
			return index
		}
	}
	return -1
}

func (p CatalogMonitoringProfile) catalogFacts(groups []SUN2000GroupReading) []SUN2000Fact {
	facts := make([]SUN2000Fact, 0, len(p.fields))
	for _, field := range p.fields {
		if field.kind == catalogAlarm {
			continue
		}
		facts = append(facts, p.catalogFact(field, groups))
	}
	facts = append(facts, p.catalogAlarmFacts(groups)...)
	return facts
}

func (p CatalogMonitoringProfile) catalogFact(field catalogField, groups []SUN2000GroupReading) SUN2000Fact {
	fact := SUN2000Fact{Key: p.prefix + field.name, Index: field.index, Unit: field.unit, Availability: FactUnknown, Grade: GradeUnknown}
	group, words, ok := catalogFieldWords(field, groups)
	if !ok {
		return fact
	}
	raw, ok := catalogDecodeWord(field, words)
	if !ok {
		return fact
	}
	if field.kind == catalogStatus {
		meaning, named := p.states[uint16(raw)]
		if !named || meaning == "" {
			return fact
		}
		fact.Availability, fact.Value, fact.State = FactValue, Decimal{Coefficient: raw}, meaning
		fact.Grade, fact.Cycle, fact.Retained = GradeHypothesis, group.Cycle, group.Retained
		return fact
	}
	fact.Availability, fact.Value = FactValue, Decimal{Coefficient: raw, Exponent: field.exponent}
	fact.Grade, fact.Cycle, fact.Retained = GradeHypothesis, group.Cycle, group.Retained
	return fact
}

func catalogFieldWords(field catalogField, groups []SUN2000GroupReading) (SUN2000GroupReading, []uint16, bool) {
	for _, group := range groups {
		if group.Outcome != SUN2000GroupAnswered || field.offset < group.Offset {
			continue
		}
		if field.offset >= group.Offset+group.Quantity {
			continue
		}
		return group, group.Words[field.offset-group.Offset:], true
	}
	return SUN2000GroupReading{}, nil, false
}

func catalogDecodeWord(field catalogField, words []uint16) (int64, bool) {
	if len(words) < int(field.width) || field.width == 0 {
		return 0, false
	}
	if field.width == 1 {
		if field.signed {
			return int64(int16(words[0])), true
		}
		return int64(words[0]), true
	}
	combined := uint32(words[0])<<16 | uint32(words[1])
	if field.signed {
		return int64(int32(combined)), true
	}
	return int64(combined), true
}

func (p CatalogMonitoringProfile) catalogAlarmFacts(groups []SUN2000GroupReading) []SUN2000Fact {
	var facts []SUN2000Fact
	for _, field := range p.fields {
		if field.kind != catalogAlarm {
			continue
		}
		group, words, ok := catalogFieldWords(field, groups)
		if !ok || len(words) == 0 {
			continue
		}
		facts = append(facts, catalogBits(p.prefix, field.offset, words[0], p.alarms[field.offset], group)...)
	}
	return facts
}

func catalogBits(prefix string, offset, word uint16, labels map[int]string, group SUN2000GroupReading) []SUN2000Fact {
	var facts []SUN2000Fact
	for bit := 0; bit < 16; bit++ {
		if word&(1<<uint(bit)) == 0 {
			continue
		}
		label := ""
		if labels != nil {
			label = labels[bit]
		}
		fact := SUN2000Fact{Availability: FactValue, Value: Decimal{Coefficient: 1}, Cycle: group.Cycle, Retained: group.Retained}
		if label != "" {
			fact.Key, fact.State, fact.Grade = prefix+label, label, GradeHypothesis
		} else {
			fact.Key = prefix + "bit_" + strconv.Itoa(int(offset)) + "_" + strconv.Itoa(bit)
			fact.Index, fact.Grade = bit, GradeUnknown
		}
		facts = append(facts, fact)
	}
	return facts
}

func catalogLoad() catalogBundle {
	names, err := catalogRegisterNames()
	if err != nil {
		return catalogBundle{}
	}
	states, alarms := catalogDictionaries()
	var built []CatalogMonitoringProfile
	for _, name := range names {
		data, err := profiles.HuaweiInverterCatalog().ReadFile(huaweiSUN2000CatalogDirectory + "/" + name)
		if err != nil {
			return catalogBundle{}
		}
		var file catalogRegisterFile
		if err := decodeHuaweiSUN2000CatalogFile(data, &file); err != nil || file.Card == "" {
			return catalogBundle{}
		}
		built = append(built, catalogBuild(file, states[file.Card], alarms[file.Card]))
	}
	sort.Slice(built, func(i, j int) bool { return built[i].card < built[j].card })
	return catalogBundle{profiles: built}
}

func catalogRegisterNames() ([]string, error) {
	found, err := profiles.HuaweiInverterCatalog().ReadDir(huaweiSUN2000CatalogDirectory)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range found {
		name := entry.Name()
		if strings.HasPrefix(name, "registers-") && strings.HasSuffix(name, ".json") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func catalogDictionaries() (map[string]map[uint16]string, map[string]map[uint16]map[int]string) {
	states := map[string]map[uint16]string{}
	alarms := map[string]map[uint16]map[int]string{}
	path := catalogDictionaryPath()
	if path == "" {
		return states, alarms
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return states, alarms
	}
	var file catalogDictFile
	if decodeHuaweiSUN2000CatalogFile(data, &file) != nil {
		return map[string]map[uint16]string{}, map[string]map[uint16]map[int]string{}
	}
	for _, item := range file.Dictionaries {
		catalogTakeDict(item, states, alarms)
	}
	return states, alarms
}

func catalogDictionaryPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	candidate := filepath.Join(filepath.Dir(file), "..", "profiles", "vendors", "huawei", "catalog", "dictionaries.json")
	if _, err := os.Stat(candidate); err != nil {
		return ""
	}
	return candidate
}

func catalogTakeDict(item catalogDict, states map[string]map[uint16]string, alarms map[string]map[uint16]map[int]string) {
	switch item.Kind {
	case "status_enum", "value_enum":
		if !catalogCovers(item.Offsets, 32089) {
			return
		}
		for _, value := range item.Values {
			code, ok := catalogCode(value.Code)
			if ok && value.Meaning != "" {
				catalogPutState(states, item.Card, code, value.Meaning)
			}
		}
	case "alarm_bits":
		for _, bit := range item.Bits {
			if bit.Offset == nil || bit.Bit < 0 || bit.Bit > 15 || bit.Label == "" {
				continue
			}
			if !catalogCovers(item.Offsets, *bit.Offset) {
				continue
			}
			catalogPutAlarm(alarms, item.Card, uint16(*bit.Offset), bit.Bit, bit.Label)
		}
	}
}

func catalogCovers(offsets []int, want int) bool {
	for _, offset := range offsets {
		if offset == want {
			return true
		}
	}
	return false
}

func catalogCode(text string) (uint16, bool) {
	text = strings.TrimSpace(strings.ToLower(text))
	base := 10
	if strings.HasPrefix(text, "0x") {
		text = text[2:]
		base = 16
	}
	value, err := strconv.ParseUint(text, base, 16)
	if err != nil {
		return 0, false
	}
	return uint16(value), true
}

func catalogPutState(states map[string]map[uint16]string, card string, code uint16, meaning string) {
	if states[card] == nil {
		states[card] = map[uint16]string{}
	}
	if previous, ok := states[card][code]; ok && previous != meaning {
		delete(states[card], code)
		return
	}
	states[card][code] = meaning
}

func catalogPutAlarm(alarms map[string]map[uint16]map[int]string, card string, offset uint16, bit int, label string) {
	if alarms[card] == nil {
		alarms[card] = map[uint16]map[int]string{}
	}
	if alarms[card][offset] == nil {
		alarms[card][offset] = map[int]string{}
	}
	alarms[card][offset][bit] = label
}

func catalogBuild(file catalogRegisterFile, states map[uint16]string, alarms map[uint16]map[int]string) CatalogMonitoringProfile {
	fields := catalogFields(file.Rows, alarms)
	if (file.Card == "MC" || file.Card == "MA-C00") && !catalogHasOffset(fields, 32108) {
		fields = append(fields, catalogField{name: "energy_dc_input", offset: 32108, width: 2, exponent: -2, unit: "kWh", kind: catalogEnergy})
	}
	fields = catalogDropOverlaps(fields)
	groups := catalogGroups(file.Card, fields)
	profile := CatalogMonitoringProfile{
		id: catalogIdentity(file.Card), card: file.Card, family: catalogFamily(file.Card),
		prefix: catalogPrefix(file.Card), groups: groups, fields: catalogKeepCovered(fields, groups),
		states: states, alarms: alarms,
	}
	for _, field := range profile.fields {
		if field.kind == catalogString && field.unit == "V" {
			profile.strings++
		}
	}
	return profile
}

func catalogIdentity(card string) string {
	switch card {
	case "MC":
		return SUN2000M3MonitoringProfileID
	case "MA-C00":
		return SUN2000M1MonitoringProfileID
	default:
		return "huawei.sun2000." + strings.ToLower(card) + ".monitoring-readonly@1.0.0"
	}
}

func catalogFamily(card string) string {
	switch card {
	case "MC":
		return huaweiSUN2000M3Family
	case "MA-C00":
		return huaweiSUN2000M1Family
	default:
		return "SUN2000 " + card
	}
}

func catalogPrefix(card string) string {
	switch card {
	case "MC":
		return "huawei.sun2000.m3."
	case "MA-C00":
		return "huawei.sun2000.m1."
	default:
		return "huawei.sun2000." + strings.ToLower(card) + "."
	}
}

func catalogHasOffset(fields []catalogField, offset uint16) bool {
	for _, field := range fields {
		if field.offset == offset {
			return true
		}
	}
	return false
}

func catalogFields(rows []catalogRegisterRow, alarms map[uint16]map[int]string) []catalogField {
	var fields []catalogField
	seen := map[int]int{}
	for _, row := range rows {
		field, ok := catalogClassify(row, alarms)
		if !ok || row.Offset == nil {
			continue
		}
		seen[*row.Offset]++
		fields = append(fields, field)
	}
	var kept []catalogField
	for _, field := range fields {
		if seen[int(field.offset)] == 1 {
			kept = append(kept, field)
		}
	}
	return kept
}

func catalogClassify(row catalogRegisterRow, alarms map[uint16]map[int]string) (catalogField, bool) {
	if row.Offset == nil || row.Endpoint != "inverter" || row.Access == "WO" {
		return catalogField{}, false
	}
	if row.Class != "reviewed" && row.Class != "conditional" {
		return catalogField{}, false
	}
	width, signed, word := catalogWordShape(row.Type)
	if !word || row.Quantity != width {
		return catalogField{}, false
	}
	gain, unit := "", ""
	if row.Decode != nil {
		gain, unit = row.Decode.Gain, row.Decode.Unit
	}
	field := catalogField{offset: uint16(*row.Offset), width: uint16(width), signed: signed, unit: catalogPhysicalUnit(unit)}
	exponent, scaled := catalogScale(gain)
	field.exponent = exponent
	switch {
	case catalogStringClass(row.FunctionalClass) && width == 1 && (unit == "V" || unit == "A") && scaled:
		number, ok := catalogChannel(row.Key, *row.Offset)
		if !ok {
			return catalogField{}, false
		}
		field.kind, field.index = catalogString, number
		if unit == "V" {
			field.name = "pv_string_voltage"
		} else {
			field.name = "pv_string_current"
		}
	case catalogTrackerClass(row.FunctionalClass) && *row.Offset < 40000 && !strings.Contains(strings.ToLower(row.Key), "predict") && scaled && (unit == "kW" || unit == "kWh"):
		number, ok := catalogNumber(catalogTrackerNumber, row.Key)
		if !ok {
			return catalogField{}, false
		}
		field.kind, field.index = catalogTracker, number
		if unit == "kW" {
			field.name = "mppt_power"
		} else {
			field.name = "mppt_energy"
		}
	case catalogRatingOffset(*row.Offset) && catalogRatingUnit(unit) && scaled:
		name, ok := catalogKnownName(*row.Offset)
		if !ok {
			return catalogField{}, false
		}
		field.kind, field.name = catalogRating, name
	case catalogEnergyClass(row.FunctionalClass) && unit == "kWh" && *row.Offset < 37000 && scaled:
		field.kind = catalogEnergy
		if name, ok := catalogKnownName(*row.Offset); ok {
			field.name = name
		} else {
			field.name = "energy_" + strconv.Itoa(*row.Offset)
		}
	case *row.Offset == 32089:
		field.kind, field.name, field.unit, field.exponent = catalogStatus, "status", "", 0
	case alarms != nil && alarms[uint16(*row.Offset)] != nil && width == 1:
		field.kind, field.name = catalogAlarm, "alarm_word"
	case *row.Offset >= 32064 && *row.Offset < 32106 && !catalogSkipsPower(row.FunctionalClass) && scaled:
		field.kind = catalogPower
		if name, ok := catalogKnownName(*row.Offset); ok {
			field.name = name
		} else {
			field.name = "register_" + strconv.Itoa(*row.Offset)
		}
	default:
		return catalogField{}, false
	}
	return field, true
}

func catalogStringClass(class string) bool {
	switch class {
	case "pv.channel", "dc_channel", "pv12", "pv36":
		return true
	default:
		return false
	}
}

func catalogTrackerClass(class string) bool {
	return class == "mppt" || class == "pv.mppt"
}

func catalogEnergyClass(class string) bool {
	return class == "energy" || class == "energy.period_or_total"
}

func catalogSkipsPower(class string) bool {
	switch class {
	case "epoch", "epoch_times", "fault_code", "alarm.priority_code", "time.event_or_collection":
		return true
	default:
		return false
	}
}

func catalogRatingOffset(offset int) bool {
	switch offset {
	case 30073, 30075, 30077, 30079, 30081, 30083, 30085:
		return true
	default:
		return false
	}
}

func catalogRatingUnit(unit string) bool {
	switch unit {
	case "kW", "kVA", "kVar":
		return true
	default:
		return false
	}
}

func catalogWordShape(typeName string) (int, bool, bool) {
	switch strings.ToUpper(strings.TrimSpace(typeName)) {
	case "I16", "INT16":
		return 1, true, true
	case "U16", "UINT16", "ENUM16", "E16", "BITFIELD16":
		return 1, false, true
	case "I32", "INT32":
		return 2, true, true
	case "U32", "UINT32":
		return 2, false, true
	default:
		return 0, false, false
	}
}

func catalogScale(gain string) (int16, bool) {
	switch strings.TrimSpace(gain) {
	case "1":
		return 0, true
	case "10":
		return -1, true
	case "100":
		return -2, true
	case "1000":
		return -3, true
	default:
		return 0, false
	}
}

func catalogPhysicalUnit(unit string) string {
	switch strings.TrimSpace(unit) {
	case "", "N/A", "n/a", "not stated", "-":
		return ""
	default:
		return strings.TrimSpace(unit)
	}
}

func catalogKnownName(offset int) (string, bool) {
	switch offset {
	case 30073:
		return "rated_power", true
	case 30075:
		return "active_limit", true
	case 30077:
		return "apparent_limit", true
	case 30079:
		return "reactive_export_limit", true
	case 30081:
		return "reactive_import_limit", true
	case 30083:
		return "nameplate_active_capacity", true
	case 30085:
		return "nameplate_apparent_capacity", true
	case 32064:
		return "input_power", true
	case 32066:
		return "line_voltage_ab", true
	case 32067:
		return "line_voltage_bc", true
	case 32068:
		return "line_voltage_ca", true
	case 32069:
		return "phase_voltage_a", true
	case 32070:
		return "phase_voltage_b", true
	case 32071:
		return "phase_voltage_c", true
	case 32072:
		return "phase_current_a", true
	case 32074:
		return "phase_current_b", true
	case 32076:
		return "phase_current_c", true
	case 32078:
		return "peak_active_power", true
	case 32080:
		return "active_power", true
	case 32082:
		return "reactive_power", true
	case 32084:
		return "power_factor", true
	case 32085:
		return "frequency", true
	case 32086:
		return "efficiency", true
	case 32087:
		return "internal_temperature", true
	case 32088:
		return "insulation_resistance", true
	case 32106:
		return "energy_accumulated", true
	case 32108:
		return "energy_dc_input", true
	case 32112:
		return "energy_current_hour", true
	case 32114:
		return "energy_current_day", true
	case 32116:
		return "energy_current_month", true
	case 32118:
		return "energy_current_year", true
	default:
		return "", false
	}
}

func catalogChannel(key string, offset int) (int, bool) {
	if number, ok := catalogNumber(catalogStringNumber, key); ok {
		return number, true
	}
	if number, ok := catalogNumber(catalogDirectChannel, key); ok {
		return number, true
	}
	if number, ok := catalogNumber(catalogStringShort, key); ok {
		return number, true
	}
	// A channel pair keeps its place when the card names the word only by
	// address. The low block starts at 32016. The high block starts at
	// 32344, which the cards that name it call channel 25. Aggregate
	// voltages outside both blocks are not channels.
	if offset >= 32016 && offset < 32064 {
		return (offset-32016)/2 + 1, true
	}
	if offset >= 32344 && offset < 32400 {
		return 25 + (offset-32344)/2, true
	}
	return 0, false
}

func catalogNumber(expression *regexp.Regexp, text string) (int, bool) {
	match := expression.FindStringSubmatch(text)
	if match == nil {
		return 0, false
	}
	for _, part := range match[1:] {
		if part == "" {
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil || value <= 0 {
			return 0, false
		}
		return value, true
	}
	return 0, false
}

func catalogDropOverlaps(fields []catalogField) []catalogField {
	sort.Slice(fields, func(i, j int) bool {
		if fields[i].offset != fields[j].offset {
			return fields[i].offset < fields[j].offset
		}
		return fields[i].name < fields[j].name
	})
	var kept []catalogField
	var end uint16
	for _, field := range fields {
		if len(kept) > 0 && field.offset < end {
			continue
		}
		kept = append(kept, field)
		end = field.offset + field.width
	}
	return kept
}

func catalogGroups(card string, fields []catalogField) []catalogGroup {
	rest := fields
	var forced []catalogField
	if catalogHasOffset(fields, 32064) {
		quantity := catalogWindowQuantity(fields, catalogPower, catalogStatus, 32064, 32106, 33)
		forced, rest = catalogTakeRange(fields, catalogPower, 32064, 32064+quantity, catalogStatus)
	}
	energyStart := uint16(32106)
	energyWidth := uint16(14)
	hand := card == "MC" || card == "MA-C00"
	var energy []catalogField
	if hand {
		energy, rest = catalogTakeRange(rest, catalogEnergy, energyStart, energyStart+energyWidth)
	}
	groups := catalogPack(rest)
	if len(forced) > 0 {
		groups = append(groups, catalogGroup{offset: 32064, quantity: catalogWindowQuantity(forced, catalogPower, catalogStatus, 32064, 32106, 33)})
	}
	if hand && len(energy) > 0 {
		groups = append(groups, catalogGroup{offset: energyStart, quantity: energyWidth})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].offset < groups[j].offset })
	return groups
}

func catalogWindowQuantity(fields []catalogField, left, right catalogKind, start, limit, minimum uint16) uint16 {
	end := start + minimum
	if end > limit {
		end = limit
	}
	for _, field := range fields {
		if field.kind != left && field.kind != right {
			continue
		}
		fieldEnd := field.offset + field.width
		if field.offset >= start && fieldEnd <= limit && fieldEnd > end {
			end = fieldEnd
		}
	}
	return end - start
}

func catalogTakeRange(fields []catalogField, kind catalogKind, start, end uint16, extra ...catalogKind) (inside, rest []catalogField) {
	for _, field := range fields {
		matches := field.kind == kind
		for _, other := range extra {
			if field.kind == other {
				matches = true
			}
		}
		if matches && field.offset >= start && field.offset < end {
			inside = append(inside, field)
			continue
		}
		rest = append(rest, field)
	}
	return inside, rest
}

func catalogPack(fields []catalogField) []catalogGroup {
	if len(fields) == 0 {
		return nil
	}
	sorted := append([]catalogField(nil), fields...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].offset < sorted[j].offset })
	var groups []catalogGroup
	start := sorted[0].offset
	end := start + sorted[0].width
	for _, field := range sorted[1:] {
		fieldEnd := field.offset + field.width
		if field.offset > end || fieldEnd-start > huaweiSUN2000MaxReadWords {
			groups = append(groups, catalogGroup{offset: start, quantity: end - start})
			start, end = field.offset, fieldEnd
			continue
		}
		end = fieldEnd
	}
	return append(groups, catalogGroup{offset: start, quantity: end - start})
}

func catalogKeepCovered(fields []catalogField, groups []catalogGroup) []catalogField {
	var kept []catalogField
	for _, field := range fields {
		for _, group := range groups {
			if field.offset >= group.offset && field.offset+field.width <= group.offset+group.quantity {
				kept = append(kept, field)
				break
			}
		}
	}
	return kept
}
