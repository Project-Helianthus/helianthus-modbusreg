package huawei

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	// OpSetChargeLimit is the storage-pack charge limit operation.
	// The binding is the catalogue row, not a word match.
	OpSetChargeLimit = "storage.operation.set_charge_limit"
	// OpSetDischargeLimit is the storage-pack discharge limit operation.
	// The binding is the catalogue row, not a word match.
	OpSetDischargeLimit = "storage.operation.set_discharge_limit"

	// FunctionWriteMultipleRegisters is Modbus function 16.
	FunctionWriteMultipleRegisters uint8 = 16

	chargeOffset    = 47247
	dischargeOffset = 47249
	activePower     = 40126
	boundCard       = "MA-V200"
)

// Watts is an exact decimal coefficient × 10^Exponent10, in watts.
type Watts struct {
	Coefficient string
	Exponent10  int32
}

// ControlWrite is one register write presented to admission.
// IdentifiedUnit is the inverter's unit; Unit is the unit on the write.
type ControlWrite struct {
	Card           string
	Path           string
	IdentifiedUnit byte
	Unit           byte
	Function       uint8
	Address        uint16
	Quantity       uint16
}

// StorageRow is one catalogue row this selection reports.
type StorageRow struct {
	Card       string
	Key        string
	Offset     int
	Quantity   int
	Type       string
	Access     string
	Unit       string
	Gain       string
	Condition  string
	Reason     string
	Operation  string
	WayBack    bool
	LowerKnown bool
	LowerWatts string
	UpperKnown bool
	UpperWatts string
	Rating     bool
	NotALimit  bool
}

// Inspection is the selection for one card.
type Inspection struct {
	Card      string
	Bound     []StorageRow
	Unbound   []StorageRow
	Ratings   []StorageRow
	NotLimits []StorageRow
}

type catalogRow struct {
	Key             string `json:"key"`
	Offset          int    `json:"offset"`
	Quantity        int    `json:"quantity"`
	Type            string `json:"type"`
	Access          string `json:"access"`
	FunctionalClass string `json:"functional_class"`
	Literal         *scale `json:"literal"`
	Decode          *scale `json:"decode"`
	Condition       string `json:"condition"`
}

type scale struct {
	Gain string `json:"gain"`
	Unit string `json:"unit"`
}

type cardIndex struct {
	rows []catalogRow
}

var (
	loadOnce         sync.Once
	loadErr          error
	byCard           map[string]*cardIndex
	cardIDs          []string
	retainedControls = map[string][]catalogRow{}
	retainedMu       sync.Mutex
)

// UnmarshalJSON keeps each card's control rows. The selection retains the
// register rows and does not store the control rows, and this file does not
// open those documents itself.
func (f *catalogRegisterFile) UnmarshalJSON(data []byte) error {
	var raw struct {
		Card     string               `json:"card"`
		Rows     []catalogRegisterRow `json:"rows"`
		Controls []catalogRow         `json:"controls"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	f.Card = raw.Card
	f.Rows = raw.Rows
	if raw.Card == "" {
		return nil
	}
	controls := append([]catalogRow(nil), raw.Controls...)
	retainedMu.Lock()
	retainedControls[raw.Card] = controls
	retainedMu.Unlock()
	return nil
}

func loadCatalog() error {
	loadOnce.Do(func() {
		byCard = map[string]*cardIndex{}
		files, err := huaweiInverterMonitoringRegisterCards()
		if err != nil {
			loadErr = err
			return
		}
		for _, file := range files {
			if file.Card == "" {
				loadErr = errors.New("huawei: a register card has no card id")
				return
			}
			idx := byCard[file.Card]
			if idx == nil {
				idx = &cardIndex{}
				byCard[file.Card] = idx
				cardIDs = append(cardIDs, file.Card)
			}
			for _, row := range file.Rows {
				idx.rows = append(idx.rows, rowFromRegister(row))
			}
			retainedMu.Lock()
			idx.rows = append(idx.rows, retainedControls[file.Card]...)
			retainedMu.Unlock()
		}
		sort.Strings(cardIDs)
	})
	return loadErr
}

func rowFromRegister(row catalogRegisterRow) catalogRow {
	out := catalogRow{
		Key: row.Key, Quantity: row.Quantity, Type: row.Type, Access: row.Access,
		FunctionalClass: row.FunctionalClass,
	}
	if row.Offset != nil {
		out.Offset = *row.Offset
	}
	if row.Decode != nil {
		out.Decode = &scale{Gain: row.Decode.Gain, Unit: row.Decode.Unit}
	}
	return out
}

// Cards returns the pinned inverter card ids in ascending order.
func Cards() ([]string, error) {
	if err := loadCatalog(); err != nil {
		return nil, err
	}
	out := make([]string, len(cardIDs))
	copy(out, cardIDs)
	return out, nil
}

// ConfiguredPaths are the gateway paths named for this family.
func ConfiguredPaths() []string {
	return []string{"SmartLogger", "S-Dongle", "EMMA"}
}

func configuredPath(path string) bool {
	switch path {
	case "SmartLogger", "S-Dongle", "EMMA":
		return true
	default:
		return false
	}
}

// Inspect reports the bound rows, the unbound power-unit storage controls,
// the nameplate ratings, and the rows that are not a limit.
func Inspect(card string) (Inspection, error) {
	if err := loadCatalog(); err != nil {
		return Inspection{}, err
	}
	idx := byCard[card]
	out := Inspection{Card: card}
	if idx == nil {
		return out, nil
	}
	for _, row := range idx.rows {
		unit, gain, power := powerScale(row)
		switch {
		case row.Offset == 37007 || row.Offset == 37009:
			out.Ratings = append(out.Ratings, reportRow(card, row, unit, gain, "rating fact, not an operation limit", "", true, false))
		case row.Offset == 30073 || row.Offset == 37001:
			out.NotLimits = append(out.NotLimits, reportRow(card, row, unit, gain, "not a limit", "", false, true))
		case row.FunctionalClass == "storage" && power:
			if op, ok := boundOperation(card, row); ok {
				item := reportRow(card, row, unit, gain, "", op, false, false)
				item.LowerKnown = true
				item.LowerWatts = "0"
				out.Bound = append(out.Bound, item)
				continue
			}
			out.Unbound = append(out.Unbound, reportRow(card, row, unit, gain, unboundReason(card, row.Offset, row.Condition), "", false, false))
		}
	}
	sort.Slice(out.Bound, func(i, j int) bool { return out.Bound[i].Offset < out.Bound[j].Offset })
	sort.Slice(out.Unbound, func(i, j int) bool { return out.Unbound[i].Offset < out.Unbound[j].Offset })
	sort.Slice(out.Ratings, func(i, j int) bool { return out.Ratings[i].Offset < out.Ratings[j].Offset })
	sort.Slice(out.NotLimits, func(i, j int) bool { return out.NotLimits[i].Offset < out.NotLimits[j].Offset })
	return out, nil
}

func powerScale(row catalogRow) (unit, gain string, ok bool) {
	if row.Literal != nil && (row.Literal.Unit == "W" || row.Literal.Unit == "kW") {
		return row.Literal.Unit, row.Literal.Gain, true
	}
	if row.Decode != nil && (row.Decode.Unit == "W" || row.Decode.Unit == "kW") {
		return row.Decode.Unit, row.Decode.Gain, true
	}
	if row.Literal != nil {
		return row.Literal.Unit, row.Literal.Gain, false
	}
	if row.Decode != nil {
		return row.Decode.Unit, row.Decode.Gain, false
	}
	return "", "", false
}

func reportRow(card string, row catalogRow, unit, gain, reason, operation string, rating, notLimit bool) StorageRow {
	return StorageRow{
		Card: card, Key: row.Key, Offset: row.Offset, Quantity: row.Quantity,
		Type: row.Type, Access: row.Access, Unit: unit, Gain: gain, Condition: row.Condition,
		Reason: reason, Operation: operation, WayBack: wayBack(row.Access),
		Rating: rating, NotALimit: notLimit,
	}
}

func wayBack(access string) bool {
	return strings.Contains(strings.ToUpper(access), "R")
}

func boundOperation(card string, row catalogRow) (string, bool) {
	if card != boundCard || row.FunctionalClass != "storage" {
		return "", false
	}
	unit, _, power := powerScale(row)
	if !power || unit != "kW" {
		return "", false
	}
	switch row.Offset {
	case chargeOffset:
		return OpSetChargeLimit, true
	case dischargeOffset:
		return OpSetDischargeLimit, true
	default:
		return "", false
	}
}

func unboundReason(card string, offset int, condition string) string {
	switch offset {
	case 47075, 47077:
		if card == boundCard {
			return "MA-V200 gives both one condition, so charge and discharge are not distinguished"
		}
		if condition == "" {
			return "the card states no condition"
		}
		return "the card does not state which limit the row is"
	case 47247, 47249:
		if condition == "" {
			return "the card states no condition, so it does not state that the row is the limit"
		}
		return "the card does not state that the row is the limit"
	case 47242:
		if card == boundCard {
			return "MA-V200 states a grid-charge range. Not either operation."
		}
		if condition == "" {
			return "the card states no condition"
		}
		return "not either operation"
	case 47084:
		if card == boundCard {
			return "MA-V200 states a signed forced-power span. Not either limit."
		}
		if condition == "" {
			return "the card states no condition"
		}
		return "not either limit"
	case 47079:
		return "the card does not state its meaning, so it is not bound"
	default:
		return "the card does not state that the row is either limit"
	}
}

func findRow(card string, offset int) (catalogRow, bool) {
	idx := byCard[card]
	if idx == nil {
		return catalogRow{}, false
	}
	for _, row := range idx.rows {
		if row.Offset == offset {
			return row, true
		}
	}
	return catalogRow{}, false
}

// Bound returns the storage controls the card states are the two limits.
func Bound(card string) ([]StorageRow, error) {
	ins, err := Inspect(card)
	if err != nil {
		return nil, err
	}
	return ins.Bound, nil
}

// EncodeStorageControl encodes a watt argument for a bound row.
// The register integer is the watts expressed in the row's unit, multiplied by the row's literal gain.
func EncodeStorageControl(card string, offset int, watts Watts) ([]uint16, error) {
	row, err := boundRow(card, offset)
	if err != nil {
		return nil, err
	}
	unit, gain, ok := powerScale(row)
	if !ok {
		return nil, errors.New("huawei: the bound row has no power unit")
	}
	value, err := registerInteger(unit, gain, watts)
	if err != nil {
		return nil, err
	}
	if row.Type != "UINT32" && row.Type != "U32" {
		return nil, errors.New("huawei: the bound row is not an unsigned 32-bit control")
	}
	if value.Sign() < 0 || !value.IsUint64() || value.Uint64() > math.MaxUint32 {
		return nil, errors.New("huawei: the watt argument is unrepresentable in the row")
	}
	if row.Quantity != 2 {
		return nil, errors.New("huawei: the bound row is not two registers")
	}
	word := uint32(value.Uint64())
	return []uint16{uint16(word >> 16), uint16(word)}, nil
}

func boundRow(card string, offset int) (catalogRow, error) {
	if err := loadCatalog(); err != nil {
		return catalogRow{}, err
	}
	row, ok := findRow(card, offset)
	if !ok {
		return catalogRow{}, errors.New("huawei: the card has no row at that offset")
	}
	if _, bound := boundOperation(card, row); !bound {
		return catalogRow{}, errors.New("huawei: the card does not bind that offset")
	}
	return row, nil
}

func registerInteger(unit, gain string, watts Watts) (*big.Int, error) {
	scale := int64(0)
	switch unit {
	case "W":
		scale = 1
	case "kW":
		scale = 1000
	default:
		return nil, errors.New("huawei: the row unit is not W or kW")
	}
	parsed, err := strconv.ParseInt(gain, 10, 64)
	if err != nil || parsed <= 0 {
		return nil, errors.New("huawei: the row gain is not a positive integer")
	}
	coeff, ok := new(big.Int).SetString(watts.Coefficient, 10)
	if !ok {
		return nil, errors.New("huawei: the watt coefficient is not an integer")
	}
	num := new(big.Int).Set(coeff)
	den := big.NewInt(1)
	if watts.Exponent10 >= 0 {
		num.Mul(num, pow10(watts.Exponent10))
	} else {
		den.Mul(den, pow10(-watts.Exponent10))
	}
	num.Mul(num, big.NewInt(parsed))
	den.Mul(den, big.NewInt(scale))
	if den.Sign() == 0 {
		return nil, errors.New("huawei: the row scale is zero")
	}
	rem := new(big.Int).Rem(num, den)
	if rem.Sign() != 0 {
		return nil, errors.New("huawei: the watt argument is unrepresentable in the row")
	}
	return new(big.Int).Quo(num, den), nil
}

func pow10(exp int32) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp)), nil)
}

// AdmitStorageControlWrite admits exactly the bound storage rows, on the
// identified unit, on a configured path. It does not admit the active-power row.
func AdmitStorageControlWrite(write ControlWrite) bool {
	if !commonAdmission(write) {
		return false
	}
	row, err := boundRow(write.Card, int(write.Address))
	if err != nil {
		return false
	}
	return int(write.Quantity) == row.Quantity
}

// AdmitControlWrite is the transport callback. It admits the bound storage
// rows and the active-power row at offset 40126 where the card's functional
// class for that offset is active_power. A storage-only predicate is AdmitStorageControlWrite.
func AdmitControlWrite(write ControlWrite) bool {
	if AdmitStorageControlWrite(write) {
		return true
	}
	if !commonAdmission(write) {
		return false
	}
	if err := loadCatalog(); err != nil {
		return false
	}
	row, ok := findRow(write.Card, int(write.Address))
	if !ok || row.Offset != activePower || row.FunctionalClass != "active_power" {
		return false
	}
	return int(write.Quantity) == row.Quantity && row.Quantity == 2
}

func commonAdmission(write ControlWrite) bool {
	if write.Unit != write.IdentifiedUnit {
		return false
	}
	if write.Function != FunctionWriteMultipleRegisters {
		return false
	}
	return configuredPath(write.Path)
}

// CompareStorageReadback reports match, mismatch, or unknown.
// Unknown means the card does not document a read of that bound row, or the words cannot be compared.
// The result is native evidence only.
func CompareStorageReadback(card string, offset int, wrote, read []uint16) string {
	row, err := boundRow(card, offset)
	if err != nil || !wayBack(row.Access) || len(wrote) == 0 || len(wrote) != len(read) {
		return "unknown"
	}
	for i := range wrote {
		if wrote[i] != read[i] {
			return "mismatch"
		}
	}
	return "match"
}

// DecodeStorageWatts decodes a bound row's register words back to watts.
func DecodeStorageWatts(card string, offset int, words []uint16) (Watts, error) {
	row, err := boundRow(card, offset)
	if err != nil {
		return Watts{}, err
	}
	if len(words) != row.Quantity || row.Quantity != 2 {
		return Watts{}, errors.New("huawei: the read is not the bound row")
	}
	unit, gain, ok := powerScale(row)
	if !ok {
		return Watts{}, errors.New("huawei: the bound row has no power unit")
	}
	parsed, err := strconv.ParseInt(gain, 10, 64)
	if err != nil || parsed <= 0 {
		return Watts{}, errors.New("huawei: the row gain is not a positive integer")
	}
	scale := int64(1)
	if unit == "kW" {
		scale = 1000
	}
	raw := uint32(words[0])<<16 | uint32(words[1])
	// watts = raw * scale / gain. For kW and gain 1000 that is the raw integer.
	num := new(big.Int).SetUint64(uint64(raw))
	num.Mul(num, big.NewInt(scale))
	den := big.NewInt(parsed)
	rem := new(big.Int).Rem(num, den)
	if rem.Sign() != 0 {
		return Watts{}, errors.New("huawei: the register value is not an integer number of watts")
	}
	q := new(big.Int).Quo(num, den)
	return canonical(q), nil
}

func canonical(n *big.Int) Watts {
	if n.Sign() == 0 {
		return Watts{Coefficient: "0"}
	}
	exp := int32(0)
	ten := big.NewInt(10)
	zero := big.NewInt(0)
	value := new(big.Int).Set(n)
	for new(big.Int).Rem(value, ten).Cmp(zero) == 0 {
		value.Quo(value, ten)
		exp++
	}
	return Watts{Coefficient: value.String(), Exponent10: exp}
}
