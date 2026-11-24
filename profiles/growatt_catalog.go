package profiles

// GrowattProtocolIIRuntimeIdentity is the catalog identity of the
// documented Protocol II FC04 TL3-X profile.
const GrowattProtocolIIRuntimeIdentity = "growatt.protocol-ii.fc04"

// GrowattProtocolIICatalogEntry is the data-only descriptor for that profile.
// It does not admit an endpoint and it does not decode a register image.
type GrowattProtocolIICatalogEntry struct {
	RuntimeIdentity string
	Schema          string
	Families        []string
	Function        uint16
	Offset          uint16
	WidthWords      uint16
}

// GrowattProtocolIICatalog returns the documented TL3-X FC04 descriptor.
func GrowattProtocolIICatalog() GrowattProtocolIICatalogEntry {
	return GrowattProtocolIICatalogEntry{
		RuntimeIdentity: GrowattProtocolIIRuntimeIdentity,
		Schema:          "Protocol II v1.24 TL3-X",
		Families:        []string{"MAX", "MID", "MAC"},
		Function:        4,
		Offset:          0,
		WidthWords:      250,
	}
}
