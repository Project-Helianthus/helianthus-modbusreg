package huawei

import (
	"math"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	exportModeKey      = "gateway-self.40100"
	exportPowerKey     = "gateway-self.40107"
	exportPercentKey   = "gateway-self.40109"
	exportScopeKey     = "gateway-self.40101"
	exportPlantMode    = "plant_dispatch.field_47415"
	exportPlantPower   = "plant_dispatch.field_47416"
	exportPlantPercent = "plant_dispatch.field_47418"
)

// ExportRegister is one catalogue row the export-limit selection reports.
type ExportRegister struct {
	Key             string
	Offset          int
	Quantity        int
	Type            string
	Access          string
	Class           string
	FunctionalClass string
	Unit            string
	Gain            string
	Condition       string
	WayBack         bool
}

// ExportLimit is the feed-in limit a card binds, in the card's own unit and gain.
// A nil Scope means the card states no scope selector. PlantDispatch is that card's own plant placement.
type ExportLimit struct {
	Card                string
	Mode                ExportRegister
	Power               ExportRegister
	Percent             ExportRegister
	PercentUnreferenced bool
	Scope               *ExportRegister
	Total               uint16
	Phase               uint16
	PowerMode           uint16
	LowerKnown          bool
	LowerWatts          string
	UpperKnown          bool
	UpperWatts          string
	PlantDispatch       bool
}

// ExportNote is a row the card does not bind, with the reason.
type ExportNote struct {
	Key    string
	Offset int
	Reason string
}

// ExportInspection is the export-limit selection for one card.
type ExportInspection struct {
	Card    string
	Bound   bool
	Limit   ExportLimit
	Unbound []ExportNote
}

// BoundExportLimit returns the card's grid feed-in limit.
// A card that does not state the rows returns false. Offset alone does not select them.
func BoundExportLimit(card string) (ExportLimit, bool, error) {
	ins, err := InspectExport(card)
	if err != nil || !ins.Bound {
		return ExportLimit{}, false, err
	}
	return ins.Limit, true, nil
}

// InspectExport reports the bound feed-in rows and the rows the card does not bind.
func InspectExport(card string) (ExportInspection, error) {
	rows, err := exportRows(card)
	if err != nil {
		return ExportInspection{}, err
	}
	return selectExport(card, rows), nil
}

// EncodeExportLimit encodes pv.limit.export_power into the power-unit value row and the mode row.
// The register integer is the watts expressed in the row's unit, multiplied by the row's literal gain.
// The percent row is not encoded: a card that states no reference base is not given an invented one.
func EncodeExportLimit(card string, watts Watts) (mode []uint16, value []uint16, err error) {
	lim, ok, err := BoundExportLimit(card)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, errNoExportRow
	}
	scaled, gain, power := exportPowerScale(registerOf(lim.Power))
	if !power {
		return nil, nil, errNoExportRow
	}
	raw, err := registerInteger(kilowattUnit(scaled), gain, watts)
	if err != nil {
		if strings.Contains(err.Error(), "unrepresentable") {
			return nil, nil, ErrUnrepresentable
		}
		return nil, nil, err
	}
	value, err = signedRegisterWords(raw)
	if err != nil {
		return nil, nil, err
	}
	if lim.Mode.Quantity != 1 || lim.Mode.Offset < 0 || lim.Mode.Offset > math.MaxUint16 {
		return nil, nil, errNoExportRow
	}
	return []uint16{lim.PowerMode}, value, nil
}

// AdmitExportWrite admits the bound mode row or the bound power value row, on the identified unit, on a configured path.
// It does not admit the scope row or the percent row.
func AdmitExportWrite(write ControlWrite) bool {
	if !commonAdmission(write) {
		return false
	}
	lim, ok, err := BoundExportLimit(write.Card)
	if err != nil || !ok {
		return false
	}
	if lim.Mode.Offset == int(write.Address) && lim.Mode.Quantity == int(write.Quantity) {
		return true
	}
	return lim.Power.Offset == int(write.Address) && lim.Power.Quantity == int(write.Quantity)
}

// CompareExportReadback reports match, mismatch, or unknown for the mode row or the power row.
// Unknown means the card documents no read, or the word counts differ.
func CompareExportReadback(card, part string, wrote, read []uint16) string {
	lim, ok, err := BoundExportLimit(card)
	if err != nil || !ok {
		return "unknown"
	}
	row := lim.Mode
	if part == "power" {
		row = lim.Power
	} else if part != "mode" {
		return "unknown"
	}
	if !row.WayBack || len(wrote) == 0 || len(wrote) != len(read) || len(wrote) != row.Quantity {
		return "unknown"
	}
	for i := range wrote {
		if wrote[i] != read[i] {
			return "mismatch"
		}
	}
	return "match"
}

// DecodeExportWatts decodes the bound power row's words back to watts.
func DecodeExportWatts(card string, words []uint16) (Watts, error) {
	lim, ok, err := BoundExportLimit(card)
	if err != nil {
		return Watts{}, err
	}
	if !ok || len(words) != lim.Power.Quantity || lim.Power.Quantity != 2 {
		return Watts{}, errNoExportRow
	}
	scaled, gain, power := exportPowerScale(registerOf(lim.Power))
	if !power {
		return Watts{}, errNoExportRow
	}
	parsed, err := strconv.ParseInt(gain, 10, 64)
	if err != nil || parsed <= 0 {
		return Watts{}, errNoExportRow
	}
	scale := int64(1)
	if kilowattUnit(scaled) == "kW" {
		scale = 1000
	}
	raw := int64(int32(uint32(words[0])<<16 | uint32(words[1])))
	num := big.NewInt(raw)
	num.Mul(num, big.NewInt(scale))
	den := big.NewInt(parsed)
	if new(big.Int).Rem(num, den).Sign() != 0 {
		return Watts{}, errNoExportRow
	}
	return canonical(new(big.Int).Quo(num, den)), nil
}

var errNoExportRow = errExport("the card binds no feed-in limit")

type exportError string

func (e exportError) Error() string { return "huawei: " + string(e) }

func errExport(text string) error { return exportError(text) }

func exportRows(card string) ([]catalogRow, error) {
	if rows, ok := gatewayExportRows(card); ok {
		return rows, nil
	}
	if err := loadCatalog(); err != nil {
		return nil, err
	}
	idx := byCard[card]
	if idx == nil {
		return nil, nil
	}
	out := append([]catalogRow(nil), idx.rows...)
	return out, nil
}

func gatewayExportRows(card string) ([]catalogRow, bool) {
	switch card {
	case "emma-v100r024c00-01-2024-07-15", "emma-v100r025c00spc102-2025-08-14":
		return append([]catalogRow(nil), emmaFeedInRows...), true
	default:
		return nil, false
	}
}

func selectExport(card string, rows []catalogRow) ExportInspection {
	out := ExportInspection{Card: card}
	var mode, power, percent, scope catalogRow
	var hasMode, hasPower, hasPercent, hasScope bool
	for _, row := range rows {
		switch {
		case row.Key == exportModeKey || strings.HasSuffix(row.Key, exportPlantMode):
			mode, hasMode = row, true
		case row.Key == exportPowerKey || strings.HasSuffix(row.Key, exportPlantPower):
			power, hasPower = row, true
		case row.Key == exportPercentKey || strings.HasSuffix(row.Key, exportPlantPercent):
			percent, hasPercent = row, true
		case row.Key == exportScopeKey && scopePair(row.Condition):
			scope, hasScope = row, true
		}
	}
	limit, ok, reason := bindExport(card, mode, power, percent, scope, hasMode, hasPower, hasPercent, hasScope)
	out.Bound, out.Limit = ok, limit
	for _, row := range rows {
		if ok && exportOwned(row, limit) {
			continue
		}
		if note, noted := exportNote(row, hasMode && !ok, reason); noted {
			out.Unbound = append(out.Unbound, note)
		}
	}
	sort.Slice(out.Unbound, func(i, j int) bool {
		if out.Unbound[i].Offset != out.Unbound[j].Offset {
			return out.Unbound[i].Offset < out.Unbound[j].Offset
		}
		return out.Unbound[i].Key < out.Unbound[j].Key
	})
	return out
}

func bindExport(card string, mode, power, percent, scope catalogRow, hasMode, hasPower, hasPercent, hasScope bool) (ExportLimit, bool, string) {
	if !hasMode {
		return ExportLimit{}, false, ""
	}
	if !readableConfig(mode) {
		return ExportLimit{}, false, "the mode row is not readable configuration"
	}
	powerCode, powerOK := oneCode(mode.Condition, powerClause)
	percentCode, percentOK := oneCode(mode.Condition, percentClause)
	if !powerOK || !percentOK {
		if statesGridLimit(mode.Condition) {
			return ExportLimit{}, false, "the card states a grid feed-in or grid-power limit and does not identify the value row"
		}
		return ExportLimit{}, false, "the card does not state a power-unit limit and a percent limit"
	}
	if !hasPower || !familyMatch(mode, power) || !readableConfig(power) || !signedPower(power) {
		return ExportLimit{}, false, "the card states a grid feed-in or grid-power limit and does not identify the value row"
	}
	plant := mode.FunctionalClass == "plant_dispatch"
	limit := ExportLimit{Card: card, Mode: exportRegister(mode), Power: exportRegister(power), PowerMode: powerCode, PlantDispatch: plant}
	if hasPercent && familyPercent(mode, percent) {
		limit.Percent = exportRegister(percent)
		limit.PercentUnreferenced = !percentReference(percent.Condition)
	}
	if percentCode == powerCode {
		return ExportLimit{}, false, "the card does not distinguish the power-unit limit from the percent limit"
	}
	lowerKnown, lower, upperKnown, upper := exportBounds(power.Condition, limit.Power.Unit)
	limit.LowerKnown, limit.LowerWatts, limit.UpperKnown, limit.UpperWatts = lowerKnown, lower, upperKnown, upper
	if hasScope && scope.Key == exportScopeKey {
		total, phase, ok := scopeWords(scope.Condition)
		if !ok {
			return ExportLimit{}, false, "the card states a scope row and does not state total and individual phase"
		}
		row := exportRegister(scope)
		limit.Scope, limit.Total, limit.Phase = &row, total, phase
		return limit, true, ""
	}
	if !plant {
		return ExportLimit{}, false, "the card states neither a scope selector row nor a plant-dispatch placement"
	}
	return limit, true, ""
}

func exportOwned(row catalogRow, limit ExportLimit) bool {
	if row.Key == limit.Mode.Key || row.Key == limit.Power.Key || row.Key == limit.Percent.Key {
		return true
	}
	return limit.Scope != nil && row.Key == limit.Scope.Key
}

func exportNote(row catalogRow, failedMode bool, reason string) (ExportNote, bool) {
	if failedMode && (row.Key == exportModeKey || strings.HasSuffix(row.Key, exportPlantMode)) && reason != "" {
		return ExportNote{Key: row.Key, Offset: row.Offset, Reason: reason}, true
	}
	low := strings.ToLower(row.Condition)
	switch {
	case strings.Contains(low, "source scope is blank"):
		return ExportNote{Key: row.Key, Offset: row.Offset, Reason: "the card states that the source scope is blank, so the row is not a scope selector"}, true
	case statesGridLimit(row.Condition) && row.Key != exportModeKey && !strings.HasSuffix(row.Key, exportPlantMode):
		return ExportNote{Key: row.Key, Offset: row.Offset, Reason: "the card states a grid feed-in or grid-power limit and does not identify the value row"}, true
	case row.Condition == "" && unnamedOffset(row):
		return ExportNote{Key: row.Key, Offset: row.Offset, Reason: "the card states no condition for this offset, so the row is not bound"}, true
	default:
		return ExportNote{}, false
	}
}

func unnamedOffset(row catalogRow) bool {
	switch row.Offset {
	case 40100, 40101, 40107, 40109, 47415, 47416, 47418, 47419:
		return row.Key != exportModeKey && row.Key != exportPowerKey && row.Key != exportPercentKey && row.Key != exportScopeKey &&
			!strings.HasSuffix(row.Key, exportPlantMode) && !strings.HasSuffix(row.Key, exportPlantPower) && !strings.HasSuffix(row.Key, exportPlantPercent)
	default:
		return false
	}
}

func familyMatch(mode, power catalogRow) bool {
	if mode.Key == exportModeKey {
		return power.Key == exportPowerKey
	}
	return strings.HasSuffix(mode.Key, exportPlantMode) && strings.HasSuffix(power.Key, exportPlantPower)
}

func familyPercent(mode, percent catalogRow) bool {
	if mode.Key == exportModeKey {
		return percent.Key == exportPercentKey
	}
	return strings.HasSuffix(mode.Key, exportPlantMode) && strings.HasSuffix(percent.Key, exportPlantPercent)
}

func readableConfig(row catalogRow) bool {
	return strings.EqualFold(row.Access, "RW") && row.Class == "readable-configuration" && wayBack(row.Access)
}

func signedPower(row catalogRow) bool {
	_, _, ok := exportPowerScale(row)
	return ok && row.Quantity == 2 && (row.Type == "INT32" || row.Type == "I32")
}

func exportPowerScale(row catalogRow) (unit, gain string, ok bool) {
	cell := row.Literal
	if cell == nil {
		cell = row.Decode
	}
	if cell == nil {
		return "", "", false
	}
	switch cell.Unit {
	case "W", "kW", "Kw":
		gain = cell.Gain
		parsed, err := strconv.ParseInt(gain, 10, 64)
		if err != nil || parsed <= 0 {
			return cell.Unit, gain, false
		}
		return cell.Unit, gain, true
	default:
		return cell.Unit, cell.Gain, false
	}
}

func exportRegister(row catalogRow) ExportRegister {
	unit, gain, _ := exportPowerScale(row)
	if unit == "" && row.Literal != nil {
		unit, gain = row.Literal.Unit, row.Literal.Gain
	}
	return ExportRegister{
		Key: row.Key, Offset: row.Offset, Quantity: row.Quantity, Type: row.Type,
		Access: row.Access, Class: row.Class, FunctionalClass: row.FunctionalClass,
		Unit: unit, Gain: gain, Condition: row.Condition, WayBack: wayBack(row.Access),
	}
}

func registerOf(row ExportRegister) catalogRow {
	return catalogRow{
		Key: row.Key, Offset: row.Offset, Quantity: row.Quantity, Type: row.Type,
		Access: row.Access, Class: row.Class, FunctionalClass: row.FunctionalClass,
		Literal: &scale{Gain: row.Gain, Unit: row.Unit}, Condition: row.Condition,
	}
}

func statesGridLimit(condition string) bool {
	low := strings.ToLower(condition)
	switch {
	case strings.Contains(low, "feed-in limit") && strings.Contains(low, "kw"):
		return true
	case strings.Contains(low, "export limit") && strings.Contains(low, "kw"):
		return true
	case strings.Contains(low, "grid power limit"):
		return true
	case strings.Contains(low, "power-limited grid connection") && strings.Contains(low, "kw"):
		return true
	case strings.Contains(low, "kw limit"):
		return true
	default:
		return false
	}
}

func percentReference(condition string) bool {
	low := strings.ToLower(condition)
	if low == "" || (strings.Contains(low, "range") && !strings.Contains(low, "plant capacity") && !strings.Contains(low, "pmax") && !strings.Contains(low, "rated")) {
		return false
	}
	return strings.Contains(low, "plant capacity") || strings.Contains(low, "pmax") || strings.Contains(low, "rated") || strings.Contains(low, "pn")
}

type numberClause struct {
	n    uint16
	text string
}

var numberPattern = regexp.MustCompile(`\d+`)

func numberClauses(condition string) []numberClause {
	loc := numberPattern.FindAllStringIndex(condition, -1)
	out := make([]numberClause, 0, len(loc))
	for i, pair := range loc {
		n, err := strconv.ParseUint(condition[pair[0]:pair[1]], 10, 16)
		if err != nil {
			continue
		}
		end := len(condition)
		if i+1 < len(loc) {
			end = loc[i+1][0]
		}
		out = append(out, numberClause{n: uint16(n), text: condition[pair[1]:end]})
	}
	return out
}

func oneCode(condition string, want func(string) bool) (uint16, bool) {
	var code uint16
	count := 0
	for _, clause := range numberClauses(condition) {
		if !want(clause.text) {
			continue
		}
		code = clause.n
		count++
	}
	return code, count == 1
}

func powerClause(text string) bool {
	low := strings.ToLower(text)
	if strings.Contains(low, "percent") || !strings.Contains(low, "kw") {
		return false
	}
	return strings.Contains(low, "limit") || strings.Contains(low, "export") || strings.Contains(low, "feed-in") || strings.Contains(low, "feed in") || strings.Contains(low, "grid")
}

func percentClause(text string) bool {
	return strings.Contains(strings.ToLower(text), "percent")
}

func scopePair(condition string) bool {
	_, _, ok := scopeWords(condition)
	return ok
}

func scopeWords(condition string) (total, phase uint16, ok bool) {
	total, totalOK := oneCode(condition, func(text string) bool {
		return strings.Contains(strings.ToLower(text), "total")
	})
	phase, phaseOK := oneCode(condition, func(text string) bool {
		return strings.Contains(strings.ToLower(text), "individual")
	})
	if !totalOK || !phaseOK || total == phase {
		return 0, 0, false
	}
	return total, phase, true
}

func exportBounds(condition, unit string) (lowerKnown bool, lower string, upperKnown bool, upper string) {
	if strings.Contains(strings.ToLower(condition), "does not define a numeric") {
		return false, "", false, ""
	}
	norm := strings.NewReplacer("−", "-", "–", "-", "—", "-", "…", "..").Replace(condition)
	low := strings.ToLower(norm)
	at := strings.Index(low, "range")
	if at < 0 {
		return false, "", false, ""
	}
	rest := strings.TrimSpace(norm[at+len("range"):])
	parts := strings.SplitN(rest, "..", 2)
	if len(parts) != 2 {
		return false, "", false, ""
	}
	left, ok := leadingSigned(strings.TrimSpace(parts[0]))
	if !ok {
		return false, "", false, ""
	}
	watts, ok := scaleToWatts(left, unit)
	if !ok {
		return false, "", false, ""
	}
	right := strings.TrimSpace(parts[1])
	if semi := strings.Index(right, ";"); semi >= 0 {
		right = strings.TrimSpace(right[:semi])
	}
	rightLow := strings.ToLower(right)
	if strings.Contains(rightLow, "plant") || strings.Contains(rightLow, "pmax") {
		return true, watts, false, ""
	}
	if number, known := leadingSigned(right); known {
		if upperWatts, scaled := scaleToWatts(number, unit); scaled {
			return true, watts, true, upperWatts
		}
	}
	return true, watts, false, ""
}

func leadingSigned(text string) (string, bool) {
	if text == "" {
		return "", false
	}
	i := 0
	if text[0] == '-' || text[0] == '+' {
		i = 1
	}
	start := i
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		i++
	}
	if i == start || (i < len(text) && text[i] != ' ' && text[i] != '.' && text[i] != ';') {
		if i != len(text) {
			return "", false
		}
	}
	if i == start {
		return "", false
	}
	return text[:i], true
}

func kilowattUnit(unit string) string {
	if unit == "Kw" {
		return "kW"
	}
	return unit
}

func scaleToWatts(number, unit string) (string, bool) {
	n, ok := new(big.Int).SetString(number, 10)
	if !ok {
		return "", false
	}
	scale := int64(1)
	if unit == "kW" || unit == "Kw" {
		scale = 1000
	} else if unit != "W" {
		return "", false
	}
	n.Mul(n, big.NewInt(scale))
	return n.String(), true
}

func signedRegisterWords(v *big.Int) ([]uint16, error) {
	if v.Cmp(big.NewInt(math.MinInt32)) < 0 || v.Cmp(big.NewInt(math.MaxInt32)) > 0 {
		return nil, ErrUnrepresentable
	}
	word := uint32(int32(v.Int64()))
	return []uint16{uint16(word >> 16), uint16(word)}, nil
}

// The Kw spelling is the card's own unit text. The scale is the kilowatt scale.
var emmaFeedInRows = []catalogRow{
	{Key: exportModeKey, Offset: 40100, Quantity: 1, Type: "ENUM16", Access: "RW", Class: "readable-configuration", Literal: &scale{Gain: "NA", Unit: "NA"}, Condition: "Grid-connection power modes: 0 no limit; 5 zero-power grid connection; 6 feed-in limit in kW; 7 feed-in limit in percent."},
	{Key: exportScopeKey, Offset: 40101, Quantity: 1, Type: "ENUM16", Access: "RW", Class: "readable-configuration", Literal: &scale{Gain: "NA", Unit: "NA"}, Condition: "Limit applies to 0 total power or 1 individual phase power."},
	{Key: exportPowerKey, Offset: 40107, Quantity: 2, Type: "I32", Access: "RW", Class: "readable-configuration", Literal: &scale{Gain: "1000", Unit: "kW"}, Condition: "Range −1…Pmax. The cell does not define a numeric Pmax or special meaning for −1."},
	{Key: exportPercentKey, Offset: 40109, Quantity: 1, Type: "U16", Access: "RW", Class: "readable-configuration", Literal: &scale{Gain: "10", Unit: "%"}},
}
