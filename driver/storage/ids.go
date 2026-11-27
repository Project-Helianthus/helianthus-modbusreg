package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

const (
	packID      semreg.DefinitionID    = "helianthus.pack.storage"
	packVersion semreg.SemanticVersion = "1.1.0"

	kindSystem    semreg.DefinitionID = "resource.storage_system"
	kindBattery   semreg.DefinitionID = "resource.battery"
	kindPack      semreg.DefinitionID = "resource.battery_pack"
	kindInterface semreg.DefinitionID = "resource.storage_interface"
	kindCell      semreg.DefinitionID = "resource.battery_cell"

	dimPack      semreg.DefinitionID = "storage.dimension.pack"
	dimCell      semreg.DefinitionID = "storage.dimension.cell"
	dimInterface semreg.DefinitionID = "storage.dimension.interface"

	svcPack      semreg.DefinitionID = "storage.service.pack"
	svcInterface semreg.DefinitionID = "storage.service.interface"

	relSystemBattery semreg.DefinitionID = "storage.relation.system_battery"
	relBatteryPack   semreg.DefinitionID = "storage.relation.battery_pack"
	relPackInterface semreg.DefinitionID = "storage.relation.pack_interface"

	unitJoule      semreg.DefinitionID = "unit.joule"
	unitWatt       semreg.DefinitionID = "unit.watt"
	unitVolt       semreg.DefinitionID = "unit.volt"
	unitAmpere     semreg.DefinitionID = "unit.ampere"
	unitKelvin     semreg.DefinitionID = "unit.kelvin"
	unitRatio      semreg.DefinitionID = "unit.ratio"
	unitAmpereHour semreg.DefinitionID = "unit.ampere_hour"
	unitOne        semreg.DefinitionID = "unit.one"

	elementOperating semreg.DefinitionID = "storage.status.operating"
	elementDirection semreg.DefinitionID = "storage.status.charge_direction"

	evidenceOwner semreg.DefinitionID    = "helianthus.modbus"
	contractNS    semreg.ContractVersion = "helianthus.modbus.storage/v1"
	namespaceKey  driver.NamespaceID     = "modbus.storage"
)

var storagePack = semreg.PackRef{ID: packID, Version: packVersion}

func def(id semreg.DefinitionID) semreg.DefinitionRef {
	return semreg.DefinitionRef{Pack: storagePack, ID: id, Version: packVersion}
}

// Allocator turns a node key into its resource identifier. The host owns it;
// the identifiers a host already published stay the ones it allocates.
type Allocator interface {
	Allocate(driver.NodeKey) (semreg.ResourceID, error)
}

// hashHex is the first-incarnation hash: SHA-256 over the parts joined by one
// zero byte.
func hashHex(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func digestOf(parts ...string) semreg.Digest {
	return semreg.Digest("sha256:" + hashHex(parts...))
}

func evidence(kind semreg.DefinitionID, parts ...string) semreg.EvidenceRef {
	return semreg.EvidenceRef{Owner: evidenceOwner, Kind: kind, Digest: digestOf(parts...), Contract: contractNS,
		Access: semreg.EvidenceAccessPublic, Redaction: semreg.RedactionNone}
}

func text(s string) semreg.Value { return semreg.Value{Kind: semreg.ValueText, Text: &s} }

// packSlot is the operator slot of the RS-485 pack: a domain-separated digest
// of the configured asset label, so the anchor carries no configured text.
func packSlot(asset semreg.AssetID) string {
	return hashHex("helianthus.gateway.storage.pack_slot/v1", string(asset))[:32]
}
