package huawei

import (
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"
)

const activePowerKeySuffix = ".fixed_active_limit_watts"

// ErrUnrepresentable means the watt value is not an exact integer from 0 through 4294967295 in the bound row.
var ErrUnrepresentable = errors.New("huawei: the watt argument is unrepresentable in the row")

// ErrReleaseUnknown means the card has neither a queried maximum nor a numeric upper bound for the bound row.
var ErrReleaseUnknown = errors.New("huawei: release_unknown")

var errNoActiveRow = errors.New("huawei: the card binds no fixed active-power row")

// ActivePowerControl is the one fixed active-power row a card binds, in the card's own unit and gain.
type ActivePowerControl struct {
	Card            string
	Key             string
	Offset          int
	Quantity        int
	Type            string
	Access          string
	FunctionalClass string
	Unit            string
	Gain            string
	WayBack         bool
	LowerKnown      bool
	LowerWatts      string
	UpperKnown      bool
	UpperWatts      string
}

// BoundActivePower returns the card's fixed active-power watts row.
// A card that does not name that row returns false. Offset alone does not select it.
func BoundActivePower(card string) (ActivePowerControl, bool, error) {
	row, err := activeRow(card)
	if errors.Is(err, errNoActiveRow) {
		return ActivePowerControl{}, false, nil
	}
	if err != nil {
		return ActivePowerControl{}, false, err
	}
	unit, gain, _ := powerScale(row)
	lowerKnown, lower, upperKnown, upper := activeBounds(row.Condition)
	return ActivePowerControl{
		Card: card, Key: row.Key, Offset: row.Offset, Quantity: row.Quantity,
		Type: row.Type, Access: row.Access, FunctionalClass: row.FunctionalClass,
		Unit: unit, Gain: gain, WayBack: wayBack(row.Access),
		LowerKnown: lowerKnown, LowerWatts: lower, UpperKnown: upperKnown, UpperWatts: upper,
	}, true, nil
}

// EncodeActivePowerLimit encodes watts for the bound row.
// The register integer is that watt value in the row's unit, multiplied by the row's literal gain.
// The two words are most-significant word first. An unrepresentable value returns before any word is produced.
func EncodeActivePowerLimit(card string, watts Watts) ([]uint16, error) {
	row, err := activeRow(card)
	if err != nil {
		return nil, err
	}
	unit, gain, ok := powerScale(row)
	if !ok {
		return nil, errors.New("huawei: the bound row has no power unit")
	}
	value, err := registerInteger(unit, gain, watts)
	if err != nil {
		if strings.Contains(err.Error(), "unrepresentable") {
			return nil, ErrUnrepresentable
		}
		return nil, err
	}
	if row.Type != "U32" && row.Type != "UINT32" {
		return nil, errors.New("huawei: the bound row is not an unsigned 32-bit control")
	}
	if row.Quantity != 2 {
		return nil, errors.New("huawei: the bound row is not two registers")
	}
	if value.Sign() < 0 || !value.IsUint64() || value.Uint64() > math.MaxUint32 {
		return nil, ErrUnrepresentable
	}
	if row.Offset < 0 || row.Offset > math.MaxUint16 {
		return nil, errors.New("huawei: the bound row offset does not fit a register address")
	}
	word := uint32(value.Uint64())
	return []uint16{uint16(word >> 16), uint16(word)}, nil
}

// ActivePowerReleaseValue is the release written into the bound row.
// It is the queried maximum when that query exists, otherwise the numeric upper bound the card states.
// A rated or nominal reading is not a release value.
func ActivePowerReleaseValue(card string, queried, _ *Watts) (Watts, error) {
	if queried != nil {
		return *queried, nil
	}
	row, err := activeRow(card)
	if err != nil {
		return Watts{}, err
	}
	_, _, upperKnown, upper := activeBounds(row.Condition)
	if upperKnown {
		return Watts{Coefficient: upper}, nil
	}
	return Watts{}, ErrReleaseUnknown
}

// EncodeActivePowerRelease encodes the release value for the bound row.
// Where the value is unknown it returns ErrReleaseUnknown and no words.
func EncodeActivePowerRelease(card string, queried, rated *Watts) ([]uint16, error) {
	value, err := ActivePowerReleaseValue(card, queried, rated)
	if err != nil {
		return nil, err
	}
	return EncodeActivePowerLimit(card, value)
}

// AdmitActivePowerWrite admits exactly the bound active-power row, on the identified unit, on a configured path.
func AdmitActivePowerWrite(write ControlWrite) bool {
	if !commonAdmission(write) {
		return false
	}
	row, err := activeRow(write.Card)
	if err != nil {
		return false
	}
	if row.Quantity != 2 || int(write.Quantity) != row.Quantity {
		return false
	}
	if row.Offset < 0 || row.Offset > math.MaxUint16 {
		return false
	}
	return write.Address == uint16(row.Offset)
}

// AdmitControlWrite admits a bound storage row or the bound active-power row.
// MM09 and MM10 add their rows beside this predicate.
func AdmitControlWrite(write ControlWrite) bool {
	return AdmitStorageControlWrite(write) || AdmitActivePowerWrite(write)
}

// CompareActivePowerReadback reports match, mismatch, or unknown.
// Unknown means the card documents no read, or the word counts differ.
// Comparison uses the same most-significant-word order as the write.
func CompareActivePowerReadback(card string, wrote, read []uint16) string {
	row, err := activeRow(card)
	if err != nil || !wayBack(row.Access) || len(wrote) == 0 || len(wrote) != len(read) || len(wrote) != row.Quantity {
		return "unknown"
	}
	for i := range wrote {
		if wrote[i] != read[i] {
			return "mismatch"
		}
	}
	return "match"
}

// DecodeActivePowerWatts decodes the bound row's words back to watts.
func DecodeActivePowerWatts(card string, words []uint16) (Watts, error) {
	row, err := activeRow(card)
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
	num := new(big.Int).SetUint64(uint64(raw))
	num.Mul(num, big.NewInt(scale))
	den := big.NewInt(parsed)
	if new(big.Int).Rem(num, den).Sign() != 0 {
		return Watts{}, errors.New("huawei: the register value is not an integer number of watts")
	}
	return canonical(new(big.Int).Quo(num, den)), nil
}

func activeRow(card string) (catalogRow, error) {
	if err := loadCatalog(); err != nil {
		return catalogRow{}, err
	}
	idx := byCard[card]
	if idx == nil {
		return catalogRow{}, errNoActiveRow
	}
	var found catalogRow
	count := 0
	for _, row := range idx.rows {
		if strings.HasSuffix(row.Key, activePowerKeySuffix) && row.FunctionalClass == "active_power" {
			found = row
			count++
		}
	}
	if count == 0 {
		return catalogRow{}, errNoActiveRow
	}
	if count != 1 {
		return catalogRow{}, errors.New("huawei: the card names more than one fixed active-power row")
	}
	return found, nil
}

func activeBounds(condition string) (lowerKnown bool, lower string, upperKnown bool, upper string) {
	if strings.Contains(condition, "Range0..Pmax") {
		return true, "0", false, ""
	}
	return false, "", false, ""
}
