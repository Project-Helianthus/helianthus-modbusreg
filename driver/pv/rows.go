package pv

import (
	"math/big"
	"strings"

	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

// element is one element of a pack with the dimension and the unit the pinned
// pack declares for it.
type element struct {
	pack semreg.DefinitionID
	id   semreg.DefinitionID
	dim  semreg.DefinitionID
	unit semreg.DefinitionID
}

func pvEl(id, dim, unit semreg.DefinitionID) element { return element{pvID, id, dim, unit} }

type scope int

const (
	onInverter scope = iota
	onPhase
	onLinePair
	onSystem
	onInput
)

// convert turns the unit of a native value into the unit of the element.
type convert func(*big.Rat) *big.Rat

func by(f factor) convert { return f.apply }

// row maps one native field to an element. native is the unit the source states
// for it. Every conversion happens once, here, at the source boundary.
type row struct {
	el      element
	scope   scope
	suffix  string
	native  string
	convert convert
}

var asIs = by(one)

// sunSpecRows is the table of SunSpec capability fields. A field of a model the
// table does not list stays native, readable in the namespace. The integer
// models carry the scale in the decoded value; models 111 to 113 carry the valid
// decoded Float32 reading itself, so the same rows convert both.
var sunSpecRows = buildSunSpecRows()

func buildSunSpecRows() map[string]row {
	t := map[string]row{
		"inverter.ac.power.active":     {pvEl("pv.ac.aggregate_active_power", dimInverter, unitWatt), onInverter, "", "W", asIs},
		"inverter.ac.current.total":    {pvEl("pv.ac.aggregate_current", dimInverter, unitAmpere), onInverter, "", "A", asIs},
		"inverter.ac.power_factor":     {pvEl("pv.ac.power_factor", dimInverter, unitRatio), onInverter, "", "Pct", by(perHundred)},
		"inverter.ac.power.apparent":   {pvEl("pv.ac.aggregate_apparent_power", dimInverter, unitVA), onInverter, "", "VA", asIs},
		"inverter.ac.power.reactive":   {pvEl("pv.ac.aggregate_reactive_power", dimInverter, unitVar), onInverter, "", "var", asIs},
		"inverter.ac.frequency":        {pvEl("pv.ac.frequency", dimInverter, unitHertz), onInverter, "", "Hz", asIs},
		"inverter.dc.current":          {pvEl("pv.dc.aggregate_current", dimInverter, unitAmpere), onInverter, "", "A", asIs},
		"inverter.dc.voltage":          {pvEl("pv.dc.aggregate_voltage", dimInverter, unitVolt), onInverter, "", "V", asIs},
		"inverter.dc.power":            {pvEl("pv.dc.aggregate_power", dimInverter, unitWatt), onInverter, "", "W", asIs},
		"inverter.temperature.cabinet": {pvEl("pv.temperature.inverter", dimInverter, unitKelvin), onInverter, "", "C", celsiusToKelvin},
		"inverter.ac.energy_lifetime":  {pvEl("pv.energy.generated", dimSystem, unitJoule), onSystem, "", "Wh", by(whToJoule)},
		"der.ac.current":               {pvEl("pv.ac.aggregate_current", dimInverter, unitAmpere), onInverter, "", "A", asIs},
		"der.dc.current":               {pvEl("pv.dc.aggregate_current", dimInverter, unitAmpere), onInverter, "", "A", asIs},
		"der.dc.power":                 {pvEl("pv.dc.aggregate_power", dimInverter, unitWatt), onInverter, "", "W", asIs},
	}
	for _, p := range []string{"a", "b", "c"} {
		t["inverter.ac.current.phase_"+p] = row{pvEl("pv.ac.current", dimPhase, unitAmpere), onPhase, p, "A", asIs}
		t["inverter.ac.voltage.phase_"+p] = row{pvEl("pv.ac.voltage", dimPhase, unitVolt), onPhase, p, "V", asIs}
		t["der.ac.current.phase_"+p] = row{pvEl("pv.ac.current", dimPhase, unitAmpere), onPhase, p, "A", asIs}
		t["der.ac.power.active.phase_"+p] = row{pvEl("pv.ac.active_power", dimPhase, unitWatt), onPhase, p, "W", asIs}
		t["der.ac.power.apparent.phase_"+p] = row{pvEl("pv.ac.apparent_power", dimPhase, unitVA), onPhase, p, "VA", asIs}
		t["der.ac.power.reactive.phase_"+p] = row{pvEl("pv.ac.reactive_power", dimPhase, unitVar), onPhase, p, "var", asIs}
	}
	for _, p := range []string{"ab", "bc", "ca"} {
		t["inverter.ac.voltage.line_"+p] = row{element{infraID, "infrastructure.ac.voltage_line_line", dimLinePair, unitVolt}, onLinePair, p, "V", asIs}
	}
	return t
}

// mpptRows are the per-module values of model 160, published on one
// pv.service.input resource per module.
var mpptRows = map[string]row{
	"mppt.module.dc_current": {pvEl("pv.dc.current", dimInput, unitAmpere), onInput, "", "A", asIs},
	"mppt.module.dc_voltage": {pvEl("pv.dc.voltage", dimInput, unitVolt), onInput, "", "V", asIs},
	"mppt.module.dc_power":   {pvEl("pv.dc.power", dimInput, unitWatt), onInput, "", "W", asIs},
}

// portRows are the per-port values of a model 714 DC port of type PV.
var portRows = map[string]row{
	"der.dc.port.current": {pvEl("pv.dc.current", dimInput, unitAmpere), onInput, "", "A", asIs},
	"der.dc.port.voltage": {pvEl("pv.dc.voltage", dimInput, unitVolt), onInput, "", "V", asIs},
	"der.dc.port.power":   {pvEl("pv.dc.power", dimInput, unitWatt), onInput, "", "W", asIs},
}

// Status elements. Their symbols are the tokens the pack declares.
var (
	operatingEl = pvEl("pv.status.operating", dimInverter, "")
	deratingEl  = pvEl("pv.status.derating", dimInverter, "")
	faultEl     = pvEl("pv.status.fault", dimInverter, "")
	limitRatio  = pvEl("pv.limit.active_power_ratio", dimInverter, unitRatio)
	limitPower  = pvEl("pv.limit.active_power", dimInverter, unitWatt)
)

type operatingRow struct {
	token string
	// loss is declared when the native symbol is reduced to the token.
	loss string
}

// sunSpecStates is the table of the documented SunSpec operating states
// (models 101 to 103 and 111 to 113; the names of model 111 carry the prefix gg
// that is stripped before the lookup). FAULT has no operating token: it is
// published on pv.status.fault.
var sunSpecStates = map[string]operatingRow{
	"MPPT":          {"generating", "native MPPT mapped to generating"},
	"THROTTLED":     {"generating", "native THROTTLED mapped to generating; derating carries it"},
	"SLEEPING":      {"idle", "native SLEEPING mapped to idle"},
	"STARTING":      {"idle", "native STARTING mapped to idle"},
	"SHUTTING_DOWN": {"idle", "native SHUTTING_DOWN mapped to idle"},
	"OFF":           {"idle", "native OFF mapped to idle"},
	"STANDBY":       {"standby", ""},
}

// sunSpecFaultEvents are the bits of event word 1 that mean a fault: ground
// fault, DC and AC over voltage, AC under voltage, over and under frequency,
// over temperature and hardware test failure.
const sunSpecFaultEvents uint64 = 1<<0 | 1<<1 | 1<<7 | 1<<8 | 1<<9 | 1<<10 | 1<<11 | 1<<15

func stripVendorPrefix(symbol string) string { return strings.TrimPrefix(symbol, "gg") }

type huaweiState struct {
	operating string
	derating  bool
	fault     bool
}

// huaweiStates is the table of the registry's state dictionary names. A name
// absent from it is published raw in the driver namespace with a named reason.
var huaweiStates = map[string]huaweiState{
	"grid_connected":                     {"generating", false, false},
	"running_mode_dependent":             {"generating", false, false},
	"running_power_limited":              {"generating", true, false},
	"running_self_derated":               {"generating", true, false},
	"standby_initializing":               {"standby", false, false},
	"standby_insulation_check":           {"standby", false, false},
	"standby_irradiance_check":           {"standby", false, false},
	"standby_grid_check":                 {"standby", false, false},
	"standby_no_irradiance":              {"standby", false, false},
	"standby_no_dc_input":                {"standby", false, false},
	"starting":                           {"idle", false, false},
	"idle_init":                          {"idle", false, false},
	"idle_griddetection":                 {"idle", false, false},
	"idle_insulationresistancedetection": {"idle", false, false},
	"idle_irradiancedetection":           {"idle", false, false},
	"idle_noirradiance":                  {"idle", false, false},
	"command_shutdown":                   {"idle", false, false},
	"communication_shutdown":             {"idle", false, false},
	"dc_switch_off":                      {"idle", false, false},
	"manual_start_required":              {"idle", false, false},
	"input_underpower":                   {"idle", false, false},
	"fault_shutdown":                     {"idle", false, true},
	"ovgr_shutdown":                      {"idle", false, true},
	"power_limit_shutdown":               {"idle", true, false},
	"rapid_shutdown":                     {"idle", false, false},
	"command_rapid_shutdown":             {"idle", false, false},
	"ns_protection_shutdown":             {"idle", false, true},

	// The same status codes under the spellings of the other cards (MC and HD).
	"standby_irradiation_check": {"standby", false, false},
	"standby_no_irradiation":    {"standby", false, false},
	"grid_operation":            {"generating", false, false},
	"grid_power_limited":        {"generating", true, false},
	"grid_derated":              {"generating", true, false},
	"shutdown_fault":            {"idle", false, true},
	"shutdown_command":          {"idle", false, false},
	"shutdown_ovgr":             {"idle", false, true},
	"shutdown_communication":    {"idle", false, false},
	"shutdown_power_limit":      {"idle", true, false},
	"on_grid_off_gridmode_run":  {"generating", false, false},
	"on_grid_powerlimited_off_gridmode_operatingatlimitedpower":       {"generating", true, false},
	"on_grid_self_derating_off_gridmode_operatingatself_deratedpower": {"generating", true, false},
	"stopped_onfault":               {"idle", false, true},
	"stopped_oncommand":             {"idle", false, false},
	"stopped_ovgr":                  {"idle", false, true},
	"stopped_linkinterrupted":       {"idle", false, false},
	"stopped_powerlimited":          {"idle", true, false},
	"stopped_manualstartuprequired": {"idle", false, false},
	"stopped_dcswitchdislinked":     {"idle", false, false},
	"stopped_rapidstopped":          {"idle", false, false},
	"stopped_inputunderpower":       {"idle", false, false},
	"stopped_nsprotection":          {"idle", false, true},
	"stopped_rapidstoppedoncommand": {"idle", false, false},
}

// huaweiRows is the table of the inverter readings of a dongle-family child,
// keyed by the name after the profile prefix.
var huaweiRows = buildHuaweiRows()

func buildHuaweiRows() map[string]row {
	t := map[string]row{
		"input_power":          {pvEl("pv.dc.aggregate_power", dimInverter, unitWatt), onInverter, "", "kW", by(kiloToBase)},
		"active_power":         {pvEl("pv.ac.aggregate_active_power", dimInverter, unitWatt), onInverter, "", "kW", by(kiloToBase)},
		"reactive_power":       {pvEl("pv.ac.aggregate_reactive_power", dimInverter, unitVar), onInverter, "", "kVar", by(kiloToBase)},
		"power_factor":         {pvEl("pv.ac.power_factor", dimInverter, unitRatio), onInverter, "", "", asIs},
		"frequency":            {pvEl("pv.ac.frequency", dimInverter, unitHertz), onInverter, "", "Hz", asIs},
		"internal_temperature": {pvEl("pv.temperature.inverter", dimInverter, unitKelvin), onInverter, "", "℃", celsiusToKelvin},
		"energy_accumulated":   {pvEl("pv.energy.generated", dimSystem, unitJoule), onSystem, "", "kWh", by(kwhToJoule)},
		"energy_current_day":   {pvEl("pv.energy.yield_day", dimSystem, unitJoule), onSystem, "", "kWh", by(kwhToJoule)},
		"energy_current_month": {pvEl("pv.energy.yield_month", dimSystem, unitJoule), onSystem, "", "kWh", by(kwhToJoule)},
		"energy_current_year":  {pvEl("pv.energy.yield_year", dimSystem, unitJoule), onSystem, "", "kWh", by(kwhToJoule)},
		"pv_string_voltage":    {pvEl("pv.dc.voltage", dimInput, unitVolt), onInput, "", "V", asIs},
		"pv_string_current":    {pvEl("pv.dc.current", dimInput, unitAmpere), onInput, "", "A", asIs},
	}
	for _, p := range []string{"a", "b", "c"} {
		t["phase_voltage_"+p] = row{pvEl("pv.ac.voltage", dimPhase, unitVolt), onPhase, p, "V", asIs}
		t["phase_current_"+p] = row{pvEl("pv.ac.current", dimPhase, unitAmpere), onPhase, p, "A", asIs}
	}
	for _, p := range []string{"ab", "bc", "ca"} {
		t["line_voltage_"+p] = row{element{infraID, "infrastructure.ac.voltage_line_line", dimLinePair, unitVolt}, onLinePair, p, "V", asIs}
	}
	return t
}

// Plant values of the dongle-family paths. The pinned PV pack declares no
// element for the plant AC power, the plant DC power or the DC-input energy
// counter, so these are published in the driver namespace only.
const (
	plantTotalEnergy = "plant_total_energy"
	plantDailyEnergy = "plant_daily_energy"
	counterDCInput   = "energy_dc_input"
)
