package growatt

// BMSNamedBit is one documented bit of a BMS status word.
type BMSNamedBit struct {
	Offset uint16
	Bit    uint8
	Name   string
	Set    bool
}

// BMSRawBit is a bit the BMS contract does not define. Offset and Bit keep
// its position; the word is the native register value.
type BMSRawBit struct {
	Offset uint16
	Bit    uint8
	Word   uint16
}

// BMSScaledWord is one extension word the BMS contract gives a scale and unit.
type BMSScaledWord struct {
	Offset uint16
	Name   string
	Unit   string
	Scale  float64
	Raw    uint16
	Value  float64
}

// BMSRawWord is one native word kept by offset. Name is empty when the
// contract does not name the word.
type BMSRawWord struct {
	Offset uint16
	Name   string
	Word   uint16
}

// BMSStatusWords is the warning, protection, error, status, capability,
// topology and extension view of one typed BMS observation.
// Conditions holds only bits the contract names as warning, protection or
// error. The public contract names none of those bits, so Conditions is empty
// and those words stay in RawWords.
type BMSStatusWords struct {
	Conditions     []BMSNamedBit
	Flags          []BMSNamedBit
	BatteryID      uint16
	BatteryIDOK    bool
	ExtensionWords []BMSScaledWord
	RawBits        []BMSRawBit
	RawWords       []BMSRawWord
}

// bmsConditionRow is a documented warning, protection or error bit.
// The public BMS contract names no such bit; the slice stays empty.
type bmsConditionRow struct {
	Offset uint16
	Bit    uint8
	Name   string
}

// growattBMSConditionRows is the warning, protection and error bit table.
var growattBMSConditionRows = []bmsConditionRow{}

// bmsExtensionRow is one extension word. Num and Den are the scale as a
// rational multiplier. Opaque rows have no scale.
type bmsExtensionRow struct {
	Offset uint16
	Name   string
	Unit   string
	Num    uint16
	Den    uint16
	Opaque bool
}

// growattBMSExtensionRows publishes the extension words the BMS page defines.
// Scaled rows are the typed extension meanings. Opaque rows stay native.
var growattBMSExtensionRows = []bmsExtensionRow{
	{0x0100, "continuous_charge_time", "s", 1, 1, false},
	{0x0101, "current_cycle_charge_capacity", "Ah", 1, 10, false},
	{0x0102, "average_cell_voltage", "mV", 1, 1, false},
	{0x0103, "", "", 0, 0, true},
	{0x0104, "floating_pack_voltage", "V", 1, 10, false},
	{0x0105, "cumulative_charge_capacity", "Ah", 1, 10, false},
	{0x0106, "cumulative_discharge_capacity", "Ah", 1, 10, false},
	{0x0107, "", "", 0, 0, true},
	{0x0108, "", "", 0, 0, true},
	{0x0109, "cell_balance", "", 0, 0, true},
	{0x010A, "", "", 0, 0, true},
	{0x010B, "", "", 0, 0, true},
	{0x010D, "", "", 0, 0, true},
	{0x010E, "", "", 0, 0, true},
}

// growattBMSScaledValue is declared beside growattBMSExtensionRows. A row
// stores the scale as Num/Den so a tenth is division by ten, matching the
// existing typed BMS fields.
func growattBMSScaledValue(word uint16, row bmsExtensionRow) float64 {
	return float64(word) * float64(row.Num) / float64(row.Den)
}

// growattBMSBatteryID is declared beside the 0x001F bits 8 through 13 row.
// Those bits are one battery-id field, which a single bit row cannot express.
func growattBMSBatteryID(word uint16) uint16 {
	return (word >> 8) & 0x3f
}

// growattBMSCellBalanceSet is declared beside the 0x0109 row. Each bit is the
// documented per-cell off/on state in native bit order.
func growattBMSCellBalanceSet(word uint16, bit uint8) bool {
	return word&(1<<bit) != 0
}

func growattBMSStatusWords(wordsByOffset map[uint16][]uint16) BMSStatusWords {
	status := wordsByOffset[0x000d]
	extension := wordsByOffset[0x0100]
	tail := wordsByOffset[0x010d]
	var out BMSStatusWords
	for index, word := range status {
		out.RawWords = append(out.RawWords, BMSRawWord{Offset: 0x000d + uint16(index), Word: word})
	}
	if len(status) > 0x001f-0x000d {
		word := status[0x001f-0x000d]
		out.BatteryID = growattBMSBatteryID(word)
		out.BatteryIDOK = true
		for _, bit := range []uint8{14, 15} {
			out.Flags = append(out.Flags, BMSNamedBit{Offset: 0x001F, Bit: bit, Name: "reserved", Set: word&(1<<bit) != 0})
		}
		for bit := uint8(0); bit <= 7; bit++ {
			if word&(1<<bit) != 0 {
				out.RawBits = append(out.RawBits, BMSRawBit{Offset: 0x001F, Bit: bit, Word: word})
			}
		}
	}
	for _, row := range growattBMSConditionRows {
		word := growattBMSWordAt(status, extension, tail, row.Offset)
		out.Conditions = append(out.Conditions, BMSNamedBit{Offset: row.Offset, Bit: row.Bit, Name: row.Name, Set: word&(1<<row.Bit) != 0})
	}
	if len(extension) > 0x0109-0x0100 {
		word := extension[0x0109-0x0100]
		for bit := uint8(0); bit < 16; bit++ {
			out.Flags = append(out.Flags, BMSNamedBit{Offset: 0x0109, Bit: bit, Name: "cell_balance", Set: growattBMSCellBalanceSet(word, bit)})
		}
	}
	for _, row := range growattBMSExtensionRows {
		word := growattBMSWordAt(status, extension, tail, row.Offset)
		if row.Opaque {
			out.RawWords = append(out.RawWords, BMSRawWord{Offset: row.Offset, Name: row.Name, Word: word})
			continue
		}
		out.ExtensionWords = append(out.ExtensionWords, BMSScaledWord{
			Offset: row.Offset, Name: row.Name, Unit: row.Unit, Scale: float64(row.Num) / float64(row.Den), Raw: word, Value: growattBMSScaledValue(word, row),
		})
	}
	return out
}

func growattBMSWordAt(status, extension, tail []uint16, offset uint16) uint16 {
	switch {
	case offset >= 0x000d && int(offset-0x000d) < len(status):
		return status[offset-0x000d]
	case offset >= 0x0100 && int(offset-0x0100) < len(extension):
		return extension[offset-0x0100]
	case offset >= 0x010d && int(offset-0x010d) < len(tail):
		return tail[offset-0x010d]
	default:
		return 0
	}
}
