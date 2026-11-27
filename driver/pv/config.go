package pv

import (
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

const (
	pvID      semreg.DefinitionID    = "helianthus.pack.pv"
	infraID   semreg.DefinitionID    = "helianthus.pack.infrastructure"
	packVer   semreg.SemanticVersion = "1.0.0"
	coreOwner semreg.DefinitionID    = "helianthus.semantic.kernel"

	kindSystem    semreg.DefinitionID = "resource.pv_system"
	kindInverter  semreg.DefinitionID = "resource.inverter"
	kindPhase     semreg.DefinitionID = "resource.phase"
	kindInput     semreg.DefinitionID = "resource.dc_input"
	kindEquipment semreg.DefinitionID = "resource.equipment"

	dimSystem   semreg.DefinitionID = "pv.dimension.system"
	dimInverter semreg.DefinitionID = "pv.dimension.inverter"
	dimPhase    semreg.DefinitionID = "pv.dimension.phase"
	dimInput    semreg.DefinitionID = "pv.dimension.input"
	dimLinePair semreg.DefinitionID = "infrastructure.dimension.line_pair"

	unitWatt   semreg.DefinitionID = "unit.watt"
	unitJoule  semreg.DefinitionID = "unit.joule"
	unitRatio  semreg.DefinitionID = "unit.ratio"
	unitHertz  semreg.DefinitionID = "unit.hertz"
	unitVolt   semreg.DefinitionID = "unit.volt"
	unitAmpere semreg.DefinitionID = "unit.ampere"
	unitVA     semreg.DefinitionID = "unit.volt_ampere"
	unitVar    semreg.DefinitionID = "unit.volt_ampere_reactive"
	unitKelvin semreg.DefinitionID = "unit.kelvin"
)

var (
	pvPack    = semreg.PackRef{ID: pvID, Version: packVer}
	infraPack = semreg.PackRef{ID: infraID, Version: packVer}
)

func pvDef(id semreg.DefinitionID) semreg.DefinitionRef {
	return semreg.DefinitionRef{Pack: pvPack, ID: id, Version: packVer}
}

func infraDef(id semreg.DefinitionID) semreg.DefinitionRef {
	return semreg.DefinitionRef{Pack: infraPack, ID: id, Version: packVer}
}

func coreDef(id semreg.DefinitionID) semreg.DefinitionRef {
	return semreg.DefinitionRef{Pack: semreg.PackRef{ID: coreOwner, Version: packVer}, ID: id, Version: packVer}
}

// Config is the configuration of one producer instance.
//
// System is the configured PV system slot that groups every SunSpec device of
// the instance; empty, each device is its own PV system, anchored on the slot
// of its inverter. Endpoint is the slot of the endpoint equipment that relays
// every device of a SunSpec chain; empty, the relation stays unknown.
// LoggerSlot is the immutable link node slot of the logger that owns the link
// equipment, reserved before any exchange; empty, the logger relation stays
// unknown. The Huawei children are anchored on the instance label and the
// routed unit.
type Config struct {
	Installation string
	Driver       driver.DriverKind
	Instance     driver.InstanceKey
	Bridge       driver.BridgeID
	System       string
	Endpoint     string
	LoggerSlot   string
	Freshness    semreg.FreshnessPolicy
}

// Allocator turns a node key into its resource id.
type Allocator interface {
	Allocate(driver.NodeKey) (semreg.ResourceID, error)
}
