package meter

import (
	"math/big"

	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

type scope int

const (
	onMeter scope = iota
	onPhase
	onLinePair
	onGrid
)

// row maps one normalised reading kind to an element. Units converts the
// native unit of a source to the unit of the element; a native unit not listed
// is not mapped.
type row struct {
	el     element
	scope  scope
	suffix string
	units  map[string]factor
}

var (
	toWatt  = map[string]factor{"W": one, "kW": kiloToBase}
	toRatio = map[string]factor{"Pct": perHundred, "PF": one, "": one}
	toJoule = map[string]factor{"Wh": perHour, "kWh": kwhToJoule}
	toVAs   = map[string]factor{"VAh": perHour}
)

// rows is the table of the mapping. A reading kind absent from it stays native.
var rows = buildRows()

func buildRows() map[string]row {
	t := map[string]row{}
	for _, p := range []string{"a", "b", "c"} {
		t["current.phase."+p] = row{element{"infrastructure.ac.current", dimPhase, unitAmpere}, onPhase, p, map[string]factor{"A": one}}
		t["voltage.phase."+p] = row{element{"infrastructure.ac.voltage", dimPhase, unitVolt}, onPhase, p, map[string]factor{"V": one}}
		t["power.active.phase."+p] = row{element{"infrastructure.ac.active_power", dimPhase, unitWatt}, onPhase, p, toWatt}
		t["power.apparent.phase."+p] = row{element{"infrastructure.ac.apparent_power", dimPhase, unitVA}, onPhase, p, map[string]factor{"VA": one}}
		t["power.reactive.phase."+p] = row{element{"infrastructure.ac.reactive_power", dimPhase, unitVar}, onPhase, p, map[string]factor{"var": one}}
		t["power_factor.phase."+p] = row{element{"infrastructure.ac.phase_power_factor", dimPhase, unitRatio}, onPhase, p, toRatio}
		t["energy.import.phase."+p] = row{element{"infrastructure.energy.phase_import", dimPhase, unitJoule}, onPhase, p, toJoule}
		t["energy.export.phase."+p] = row{element{"infrastructure.energy.phase_export", dimPhase, unitJoule}, onPhase, p, toJoule}
	}
	for _, p := range []string{"ab", "bc", "ca"} {
		t["voltage.pair."+p] = row{element{"infrastructure.ac.voltage_line_line", dimLinePair, unitVolt}, onLinePair, p, map[string]factor{"V": one}}
	}
	t["frequency"] = row{element{"infrastructure.ac.frequency", dimGridConnection, unitHertz}, onGrid, "", map[string]factor{"Hz": one}}
	t["power_factor.total"] = row{element{"infrastructure.ac.power_factor", dimGridConnection, unitRatio}, onGrid, "", toRatio}
	t["power.apparent.total"] = row{element{"infrastructure.power.apparent", dimGridConnection, unitVA}, onGrid, "", map[string]factor{"VA": one}}
	t["energy.import.total"] = row{element{"infrastructure.energy.import", dimMeter, unitJoule}, onMeter, "", toJoule}
	t["energy.export.total"] = row{element{"infrastructure.energy.export", dimMeter, unitJoule}, onMeter, "", toJoule}
	t["energy.apparent_import.total"] = row{element{"infrastructure.energy.apparent_import", dimMeter, unitVAs}, onMeter, "", toVAs}
	t["energy.apparent_export.total"] = row{element{"infrastructure.energy.apparent_export", dimMeter, unitVAs}, onMeter, "", toVAs}
	return t
}

var (
	activeTotal = row{element{"infrastructure.power.pcc", dimGridConnection, unitWatt}, onGrid, "", toWatt}
	importAct   = element{"infrastructure.power.import_active", dimGridConnection, unitWatt}
	exportAct   = element{"infrastructure.power.export_active", dimGridConnection, unitWatt}
	importReact = element{"infrastructure.power.import_reactive", dimGridConnection, unitVar}
	exportReact = element{"infrastructure.power.export_reactive", dimGridConnection, unitVar}
	faultEl     = element{"infrastructure.status.fault", dimSite, ""}
)

type polarity int

const (
	signNone polarity = iota
	signOpen
	signImportPositive
	signExportPositive
)

func (p polarity) name() string {
	switch p {
	case signImportPositive:
		return "positive_import"
	case signExportPositive:
		return "positive_export"
	case signOpen:
		return "open"
	}
	return ""
}

const (
	openSign    = "sign_not_stated"
	openCounter = "counter_direction_not_stated"
	openNoQuad  = "no_catalog_element"
)

// reading is a decoded value of a meter in the unit of its source.
type reading struct {
	kind     string
	field    string
	unit     string
	value    *big.Rat
	polarity polarity
	open     string
	retained bool
	path     string
}

func (rd reading) native(d semreg.Decimal) Native {
	return Native{Source: rd.field, Unit: rd.unit, Value: d, Sign: rd.polarity.name(), Open: rd.open}
}
