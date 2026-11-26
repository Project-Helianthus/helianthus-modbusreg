package meter

import (
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

const (
	infraID      semreg.DefinitionID    = "helianthus.pack.infrastructure"
	infraVersion semreg.SemanticVersion = "1.0.0"

	kindSite           semreg.DefinitionID = "resource.site"
	kindGridConnection semreg.DefinitionID = "resource.grid_connection"
	kindMeter          semreg.DefinitionID = "resource.meter"
	kindLoad           semreg.DefinitionID = "resource.load"

	dimSite           semreg.DefinitionID = "infrastructure.dimension.site"
	dimGridConnection semreg.DefinitionID = "infrastructure.dimension.grid_connection"
	dimMeter          semreg.DefinitionID = "infrastructure.dimension.meter"
	dimLoad           semreg.DefinitionID = "infrastructure.dimension.load"
	dimPhase          semreg.DefinitionID = "infrastructure.dimension.phase"
	dimLinePair       semreg.DefinitionID = "infrastructure.dimension.line_pair"

	unitWatt   semreg.DefinitionID = "unit.watt"
	unitJoule  semreg.DefinitionID = "unit.joule"
	unitRatio  semreg.DefinitionID = "unit.ratio"
	unitVAs    semreg.DefinitionID = "unit.volt_ampere_second"
	unitHertz  semreg.DefinitionID = "unit.hertz"
	unitVolt   semreg.DefinitionID = "unit.volt"
	unitAmpere semreg.DefinitionID = "unit.ampere"
	unitVA     semreg.DefinitionID = "unit.volt_ampere"
	unitVar    semreg.DefinitionID = "unit.volt_ampere_reactive"
)

var infraPack = semreg.PackRef{ID: infraID, Version: infraVersion}

func infra(id semreg.DefinitionID) semreg.DefinitionRef {
	return semreg.DefinitionRef{Pack: infraPack, ID: id, Version: infraVersion}
}

// Role is the role the operator configures for a meter.
type Role string

// RoleGridConnection binds a meter to a grid connection. Any other role, and no
// role, leaves a meter without a grid-connection relation.
const RoleGridConnection Role = "grid_connection"

// RoleFeeder is a meter on a feeder; it contributes no grid-connection value.
const RoleFeeder Role = "feeder"

// Path names the Huawei dongle-family path a meter is read on.
type Path string

// The three documented meter exposure paths.
const (
	PathLogger  Path = "logger"
	PathEMMA    Path = "emma"
	PathSDongle Path = "sdongle"
)

// Site is a site the operator declares with its grid connections.
type Site struct {
	Slot            string
	GridConnections []string
}

// SunSpecMeter configures the role of one SunSpec meter. Position is the
// native position of the meter's Common model in its chain.
type SunSpecMeter struct {
	Unit           uint8
	Position       uint32
	Role           Role
	GridConnection string
}

// HuaweiMeter configures one Huawei meter. Slot is the configured operator
// slot; it anchors the resource. Unit only keys the read.
type HuaweiMeter struct {
	Path           Path
	Slot           string
	Unit           uint8
	Role           Role
	GridConnection string
}

// HuaweiGridPoint configures the grid measurement point of the plant and site
// groups of one path: Slot anchors its meter and its load, GridConnection is the
// declared grid connection it measures.
type HuaweiGridPoint struct {
	Path           Path
	Slot           string
	GridConnection string
}

// Config is the configuration of one producer instance.
type Config struct {
	Installation string
	Driver       driver.DriverKind
	Instance     driver.InstanceKey
	Bridge       driver.BridgeID
	Site         Site
	SunSpec      []SunSpecMeter
	Huawei       []HuaweiMeter
	GridPoints   []HuaweiGridPoint
	Freshness    semreg.FreshnessPolicy
}

// Allocator turns a node key into its resource id.
type Allocator interface {
	Allocate(driver.NodeKey) (semreg.ResourceID, error)
}
