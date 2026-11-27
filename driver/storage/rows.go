package storage

import (
	"fmt"
	"math/big"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

// Row is one mapping from a native point to an element of the storage pack.
// Every row converts once, at the source boundary, to the unit the pack
// declares for its element; the native value and unit stay in the evidence.
// Loss names what the conversion cannot carry; it is empty for an exact row.
type Row struct {
	Native     string
	NativeUnit string
	Element    semreg.DefinitionID
	Dimension  semreg.DefinitionID
	Unit       semreg.DefinitionID
	Conv       Conversion
	Loss       string
	// ZeroAbsent marks a point whose zero word is a placeholder, not a value
	// (rule 14).
	ZeroAbsent bool
}

// Convert applies the row to an exact native value.
func (r Row) Convert(native *big.Rat) (semreg.Value, error) {
	d, err := r.Conv.Apply(native)
	if err != nil {
		return semreg.Value{}, fmt.Errorf("%s: %w", r.Native, err)
	}
	return quantity(d, r.Unit), nil
}

var (
	toKelvin   = Conversion{Add: "273.15"}
	percent    = Conversion{Mul: 1, Div: 100}
	whToJoule  = Conversion{Mul: 3600}
	kwhToJoule = Conversion{Mul: 3600000}
	identity   = Conversion{}
)

const (
	lossCurrentSign  = "the native current sign reference stays native evidence"
	lossCounter      = "counter continuity, reset and wrap stay native evidence"
	lossSymbolCoarse = "the native state reduces to the coarser symbol of the element"
)

// growattRows are the rows of the RS-485 BMS family, in the order the typed
// status states them. State of charge and pack temperature carry the stated
// unit repairs; the cumulative counters stay ampere-hours.
var growattRows = []Row{
	{Native: "pack_voltage_volts", NativeUnit: "V", Element: "storage.pack.voltage", Dimension: dimPack, Unit: unitVolt, Conv: identity},
	{Native: "pack_current_amps", NativeUnit: "A", Element: "storage.pack.current", Dimension: dimPack, Unit: unitAmpere, Conv: identity, Loss: lossCurrentSign},
	{Native: "soc_percent", NativeUnit: "percent", Element: "storage.state.soc", Dimension: dimPack, Unit: unitRatio, Conv: percent},
	{Native: "temperature_celsius", NativeUnit: "degree Celsius", Element: "storage.temperature.pack", Dimension: dimPack, Unit: unitKelvin, Conv: toKelvin},
	{Native: "cumulative_charge_amp_hours", NativeUnit: "Ah", Element: "storage.capacity.charge", Dimension: dimPack, Unit: unitAmpereHour, Conv: identity, Loss: lossCounter},
	{Native: "cumulative_discharge_amp_hours", NativeUnit: "Ah", Element: "storage.capacity.discharge", Dimension: dimPack, Unit: unitAmpereHour, Conv: identity, Loss: lossCounter},
	{Native: "remaining_capacity_amp_hours", NativeUnit: "Ah", Element: "storage.capacity.remaining", Dimension: dimPack, Unit: unitAmpereHour, Conv: identity},
	{Native: "full_charge_capacity_amp_hours", NativeUnit: "Ah", Element: "storage.capacity.full_charge", Dimension: dimPack, Unit: unitAmpereHour, Conv: identity},
	{Native: "cycle_count", NativeUnit: "none", Element: "storage.state.cycle_count", Dimension: dimPack, Unit: unitOne, Conv: identity},
}

// growattCellRows are the rows of the multi-group diagnostic: the minimum and
// maximum of the valid cell voltages, one cell each.
var growattCellRows = [2]Row{
	{Native: "cell_voltage_minimum_millivolts", NativeUnit: "mV", Element: "storage.cell.voltage_minimum", Dimension: dimCell, Unit: unitVolt, Conv: Conversion{Mul: 1, Div: 1000}},
	{Native: "cell_voltage_maximum_millivolts", NativeUnit: "mV", Element: "storage.cell.voltage_maximum", Dimension: dimCell, Unit: unitVolt, Conv: Conversion{Mul: 1, Div: 1000}},
}

// growattState is the operating-state table of the RS-485 family. Coarse is the
// token of storage.status.operating, which reduces charging and discharging to
// active and has none for the soft start; Direction is the token of
// storage.status.charge_direction, which only the three directional native
// states state. Standby carries no direction: the sign of the current and the
// word standby never become the canonical idle.
type growattState struct {
	Coarse    string
	Direction string
}

var growattStates = map[string]growattState{
	"soft_starting": {Coarse: "", Direction: "starting"},
	"standby":       {Coarse: "standby", Direction: ""},
	"charging":      {Coarse: "active", Direction: "charging"},
	"discharging":   {Coarse: "active", Direction: "discharging"},
}
