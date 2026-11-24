package growatt

// protocolIITableRow is one source-table row. An empty qualifier is the common
// TL3-X map and applies to configured or matched MAX, MID and MAC. A named
// qualifier types the word only for that source family. Scale is the multiplier.
type protocolIITableRow struct {
	offset, width                     uint16
	name, qualifier, unit, kind, enum string
	scale                             float64
}

func (row protocolIITableRow) applies(family string) bool {
	switch row.qualifier {
	case "":
		return family == "MAX" || family == "MID" || family == "MAC"
	case "MAX", "MID", "MAC":
		return row.qualifier == family
	default:
		return false
	}
}

// protocolIIFixedRows is the non-repeating input-table map for words 0 through 249.
// Repeated PV, string and earth-voltage rows are added beside this table.
var protocolIIFixedRows = []protocolIITableRow{
	{0, 1, "inverter_run_state", "", "", "state", "state", 0},
	{1, 2, "aggregate_pv_input_power", "", "W", "u32", "", 0.1},
	{35, 2, "aggregate_output_power", "", "W", "u32", "", 0.1},
	{37, 1, "grid_frequency", "", "Hz", "u16", "", 0.01},
	{38, 1, "phase_1_voltage", "", "V", "u16", "", 0.1},
	{39, 1, "phase_1_current", "", "A", "u16", "", 0.1},
	{40, 2, "phase_1_apparent_power", "", "VA", "u32", "", 0.1},
	{42, 1, "phase_2_voltage", "", "V", "u16", "", 0.1},
	{43, 1, "phase_2_current", "", "A", "u16", "", 0.1},
	{44, 2, "phase_2_apparent_power", "", "VA", "u32", "", 0.1},
	{46, 1, "phase_3_voltage", "", "V", "u16", "", 0.1},
	{47, 1, "phase_3_current", "", "A", "u16", "", 0.1},
	{48, 2, "phase_3_apparent_power", "", "VA", "u32", "", 0.1},
	{50, 1, "line_voltage_rs", "", "V", "u16", "", 0.1},
	{51, 1, "line_voltage_st", "", "V", "u16", "", 0.1},
	{52, 1, "line_voltage_tr", "", "V", "u16", "", 0.1},
	{53, 2, "generated_energy_today", "", "kWh", "u32", "", 0.1},
	{55, 2, "generated_energy_total", "", "kWh", "u32", "", 0.1},
	{57, 2, "total_work_time", "", "s", "u32", "", 0.5},
	{91, 2, "pv_energy_total", "", "kWh", "u32", "", 0.1},
	{93, 1, "inverter_temperature", "", "C", "u16", "", 0.1},
	{94, 1, "inverter_ipm_temperature", "", "", "named", "", 0},
	{95, 1, "boost_temperature", "", "", "named", "", 0},
	{96, 1, "reserved_temperature", "", "", "named", "", 0},
	{97, 1, "battery_voltage", "", "V", "u16", "", 0.1},
	{98, 1, "positive_bus_voltage", "", "V", "u16", "", 0.1},
	{99, 1, "negative_bus_voltage", "", "V", "u16", "", 0.1},
	{100, 1, "output_power_factor", "", "", "named", "", 0},
	{101, 1, "real_output_power_percent", "", "percent", "u16", "", 1},
	{102, 2, "output_power_limit", "", "W", "u32", "", 0.1},
	{104, 1, "derating_code", "", "", "enum", "derating", 0},
	{105, 1, "inverter_fault_main_code", "", "", "named", "", 0},
	{106, 1, "undefined_fault_word", "", "", "named", "", 0},
	{107, 1, "inverter_fault_subcode", "", "", "named", "", 0},
	{108, 1, "storage_remote_control_enable", "SPA", "", "named", "", 0},
	{109, 1, "storage_remote_control_power", "SPA", "", "named", "", 0},
	{110, 1, "warning_bits_high", "", "", "named", "", 0},
	{111, 1, "warning_subcode", "", "", "named", "", 0},
	{112, 1, "warning_main_code", "MAX", "", "named", "", 0},
	{112, 2, "daily_ac_charge_energy", "STORAGE_POWER", "kWh", "u32", "", 0.1},
	{113, 1, "real_output_power_percent", "MAX", "percent", "u16", "", 1},
	{114, 1, "inverter_start_delay", "MAX", "", "named", "", 0},
	{114, 2, "ac_charge_energy_total", "STORAGE_POWER", "kWh", "u32", "", 0.1},
	{115, 1, "combined_fault_code", "MAX", "", "named", "", 0},
	{116, 2, "grid_to_local_load_energy", "STORAGE_POWER", "kWh", "u32", "", 0.1},
	{118, 1, "storage_priority", "STORAGE_POWER", "", "enum", "storage_priority", 0},
	{119, 1, "battery_type", "STORAGE_POWER", "", "enum", "battery_type", 0},
	{120, 1, "storage_auto_calibration", "STORAGE_POWER", "", "named", "", 0},
	{141, 1, "pid_working_status", "", "", "pid_status", "", 0},
	{177, 1, "pid_fault_code", "", "", "bits", "", 0},
	{178, 1, "string_prompt", "", "", "bits", "", 0},
	{179, 1, "pv_warning_value", "", "", "named", "", 0},
	{180, 1, "warning_value", "", "", "named", "", 0},
	{181, 1, "fault_value", "", "", "named", "", 0},
	{198, 1, "aging_test_flag", "", "", "zero_one", "", 0},
	{199, 1, "flash_erase_aging_flag", "", "", "zero_one", "", 0},
	{200, 1, "pv_isolation", "", "kilohm", "u16", "", 1},
	{201, 1, "phase_r_dc_current", "", "", "named", "", 0},
	{202, 1, "phase_s_dc_current", "", "", "named", "", 0},
	{203, 1, "phase_t_dc_current", "", "", "named", "", 0},
	{204, 1, "pid_bus_voltage", "", "V", "u16", "", 0.1},
	{205, 1, "ground_fault_current", "", "", "named", "", 0},
	{206, 1, "compensation_status", "", "", "compensation", "", 0},
	{207, 1, "phase_r_load_current", "", "A", "u16", "", 0.1},
	{208, 1, "phase_s_load_current", "", "A", "u16", "", 0.1},
	{209, 1, "phase_t_load_current", "", "A", "u16", "", 0.1},
	{210, 2, "phase_r_load_reactive_power", "", "var", "u32", "", 0.1},
	{212, 2, "phase_s_load_reactive_power", "", "var", "u32", "", 0.1},
	{214, 2, "phase_t_load_reactive_power", "", "var", "u32", "", 0.1},
	{216, 1, "phase_r_load_harmonic_current", "", "A", "u16", "", 0.1},
	{217, 1, "phase_s_load_harmonic_current", "", "A", "u16", "", 0.1},
	{218, 1, "phase_t_load_harmonic_current", "", "A", "u16", "", 0.1},
	{219, 2, "phase_r_compensated_reactive_power", "", "var", "u32", "", 0.1},
	{221, 2, "phase_s_compensated_reactive_power", "", "var", "u32", "", 0.1},
	{223, 2, "phase_t_compensated_reactive_power", "", "var", "u32", "", 0.1},
	{225, 1, "phase_r_compensated_harmonic_current", "", "A", "u16", "", 0.1},
	{226, 1, "phase_s_compensated_harmonic_current", "", "A", "u16", "", 0.1},
	{227, 1, "phase_t_compensated_harmonic_current", "", "A", "u16", "", 0.1},
	{228, 1, "serial_port_aging_flag", "", "", "zero_one", "", 0},
	{229, 1, "fan_fault_bits", "", "", "bits", "", 0},
	{230, 2, "output_apparent_power", "", "W", "u32", "", 0.1},
	{232, 2, "output_reactive_power", "", "W", "s32", "", 0.1},
	{234, 2, "nominal_output_reactive_power", "", "var", "u32", "", 0.1},
	{236, 2, "reactive_energy", "", "kWh", "u32", "", 0.1},
	{238, 1, "arc_fault_status", "", "", "enum", "arc", 0},
	{239, 1, "transform_value_first", "", "", "named", "", 0},
	{240, 1, "transform_value_second", "", "", "named", "", 0},
	{249, 1, "reserved", "", "", "named", "", 0},
}

// growattProtocolIIRepeatedRows is declared beside protocolIIFixedRows.
// It expands the documented PV, string and earth-voltage groups.
func growattProtocolIIRepeatedRows() []protocolIITableRow {
	var rows []protocolIITableRow
	for index := 0; index < 8; index++ {
		base := uint16(3 + 4*index)
		rows = append(rows,
			protocolIITableRow{base, 1, "pv_input_voltage", "", "V", "u16", "", 0.1},
			protocolIITableRow{base + 1, 1, "pv_input_current", "", "A", "u16", "", 0.1},
			protocolIITableRow{base + 2, 2, "pv_input_power", "", "W", "u32", "", 0.1},
		)
		energy := uint16(59 + 4*index)
		rows = append(rows,
			protocolIITableRow{energy, 2, "pv_input_energy_today", "", "kWh", "u32", "", 0.1},
			protocolIITableRow{energy + 2, 2, "pv_input_energy_total", "", "kWh", "u32", "", 0.1},
		)
		earth := uint16(125 + 2*index)
		rows = append(rows,
			protocolIITableRow{earth, 1, "pv_positive_to_earth_voltage", "PID", "V", "u16", "", 0.1},
			protocolIITableRow{earth, 1, "flyspan_voltage", "MAX_HV", "V", "u16", "", 0.1},
			protocolIITableRow{earth + 1, 1, "pv_positive_to_earth_current", "PID", "mA", "s16", "", 0.1},
		)
	}
	for index := 0; index < 16; index++ {
		base := uint16(142 + 2*index)
		rows = append(rows,
			protocolIITableRow{base, 1, "string_voltage", "", "V", "u16", "", 0.1},
			protocolIITableRow{base + 1, 1, "string_current", "", "A", "s16", "", 0.1},
		)
	}
	for _, offset := range []uint16{121, 122, 123, 124} {
		rows = append(rows, protocolIITableRow{offset, 1, "reserved", "", "", "named", "", 0})
	}
	for offset := uint16(182); offset <= 197; offset++ {
		rows = append(rows, protocolIITableRow{offset, 1, "debug_data", "", "", "named", "", 0})
	}
	for offset := uint16(241); offset <= 248; offset++ {
		rows = append(rows, protocolIITableRow{offset, 1, "debug_data", "", "", "named", "", 0})
	}
	rows = append(rows,
		protocolIITableRow{174, 1, "string_unmatch_bits", "", "", "bits", "", 0},
		protocolIITableRow{175, 1, "string_current_unbalance_bits", "", "", "bits", "", 0},
		protocolIITableRow{176, 1, "string_disconnect_bits", "", "", "bits", "", 0},
	)
	return rows
}

var protocolIIRowTable = append(append([]protocolIITableRow{}, protocolIIFixedRows...), growattProtocolIIRepeatedRows()...)

func growattProtocolIIProject(family string, words []uint16) ([]ProtocolIITableReading, []ProtocolIITableFlag, []ProtocolIITableCode, []ProtocolIITableRawWord, []ProtocolIITableRawBit) {
	return growattProtocolIIProjectAllow(words, func(row protocolIITableRow) bool { return row.applies(family) })
}

func growattProtocolIIProjectAllow(words []uint16, allow func(protocolIITableRow) bool) ([]ProtocolIITableReading, []ProtocolIITableFlag, []ProtocolIITableCode, []ProtocolIITableRawWord, []ProtocolIITableRawBit) {
	var readings []ProtocolIITableReading
	var flags []ProtocolIITableFlag
	var codes []ProtocolIITableCode
	var raw []ProtocolIITableRawWord
	var rawBits []ProtocolIITableRawBit
	if len(words) != protocolIIInputWidth || allow == nil {
		return readings, flags, codes, raw, rawBits
	}
	cover := make([]int, protocolIIInputWidth)
	use := make([]bool, len(protocolIIRowTable))
	for index, row := range protocolIIRowTable {
		if !allow(row) || int(row.offset)+int(row.width) > len(words) || row.width == 0 {
			continue
		}
		use[index] = true
		for word := uint16(0); word < row.width; word++ {
			cover[row.offset+word]++
		}
	}
	emitted := make([]bool, len(words))
	for index, row := range protocolIIRowTable {
		if !use[index] {
			continue
		}
		unique := true
		for word := uint16(0); word < row.width; word++ {
			if cover[row.offset+word] != 1 {
				unique = false
			}
		}
		if !unique || !growattProtocolIIEmit(&readings, &flags, &codes, &raw, &rawBits, row, words) {
			continue
		}
		for word := uint16(0); word < row.width; word++ {
			emitted[row.offset+word] = true
		}
	}
	for offset := uint16(0); offset < protocolIIInputWidth; offset++ {
		if !emitted[offset] {
			raw = append(raw, ProtocolIITableRawWord{Offset: offset, Word: words[offset]})
		}
	}
	return readings, flags, codes, raw, rawBits
}

func growattProtocolIIEmit(readings *[]ProtocolIITableReading, flags *[]ProtocolIITableFlag, codes *[]ProtocolIITableCode, raw *[]ProtocolIITableRawWord, rawBits *[]ProtocolIITableRawBit, row protocolIITableRow, words []uint16) bool {
	switch row.kind {
	case "u16", "u32", "s16", "s32":
		value, ok := growattProtocolIIScaled(words, row)
		if !ok {
			return false
		}
		*readings = append(*readings, ProtocolIITableReading{Offset: row.offset, Width: row.width, Name: row.name, Unit: row.unit, Value: value, Qualifier: row.qualifier})
		return true
	case "named":
		*raw = append(*raw, ProtocolIITableRawWord{Offset: row.offset, Word: words[row.offset], Name: row.name})
		return true
	case "state", "enum":
		text, ok := growattProtocolIIEnum(row.enum, words[row.offset])
		if !ok {
			return false
		}
		*codes = append(*codes, ProtocolIITableCode{Offset: row.offset, Name: row.name, Code: text, Raw: words[row.offset]})
		return true
	case "zero_one":
		word := words[row.offset]
		if word > 1 {
			return false
		}
		code := "0"
		if word == 1 {
			code = "1"
		}
		*codes = append(*codes, ProtocolIITableCode{Offset: row.offset, Name: row.name, Code: code, Raw: word})
		return true
	case "bits":
		growattProtocolIIEmitBits(flags, rawBits, row, words[row.offset])
		return true
	case "compensation":
		return growattProtocolIIEmitCompensation(codes, raw, row, words[row.offset])
	case "pid_status":
		return growattProtocolIIEmitPIDStatus(codes, rawBits, row, words[row.offset])
	default:
		return false
	}
}

func growattProtocolIIScaled(words []uint16, row protocolIITableRow) (float64, bool) {
	if int(row.offset)+int(row.width) > len(words) {
		return 0, false
	}
	switch row.kind {
	case "u16":
		return float64(words[row.offset]) * row.scale, true
	case "s16":
		return float64(int16(words[row.offset])) * row.scale, true
	case "u32":
		return float64(uint32(words[row.offset])<<16|uint32(words[row.offset+1])) * row.scale, true
	case "s32":
		return float64(int32(uint32(words[row.offset])<<16|uint32(words[row.offset+1]))) * row.scale, true
	default:
		return 0, false
	}
}

func growattProtocolIIEnum(kind string, word uint16) (string, bool) {
	switch kind {
	case "derating":
		names := []string{"none", "pv", "unmarked", "grid_voltage", "grid_frequency", "boost_temperature", "inverter_temperature", "control", "unmarked", "over_backup_by_time"}
		if int(word) < len(names) {
			return names[word], true
		}
	case "arc":
		names := []string{"waiting", "self_check", "arc_detection", "fault", "update"}
		if int(word) < len(names) {
			return names[word], true
		}
	case "storage_priority":
		names := []string{"load_first", "battery_first", "grid_first"}
		if int(word) < len(names) {
			return names[word], true
		}
	case "battery_type":
		names := []string{"lead_acid", "lithium"}
		if int(word) < len(names) {
			return names[word], true
		}
	case "pid_status":
		switch word {
		case 1:
			return "waiting", true
		case 2:
			return "normal", true
		case 3:
			return "fault", true
		}
	case "state":
		switch word {
		case 0:
			return "waiting", true
		case 1:
			return "normal", true
		case 3:
			return "fault", true
		}
	}
	return "", false
}

// growattProtocolIIEmitBits is declared beside the bit rows.
func growattProtocolIIEmitBits(flags *[]ProtocolIITableFlag, rawBits *[]ProtocolIITableRawBit, row protocolIITableRow, word uint16) {
	switch row.name {
	case "string_unmatch_bits", "string_current_unbalance_bits", "string_disconnect_bits":
		for bit := uint8(0); bit < 16; bit++ {
			*flags = append(*flags, ProtocolIITableFlag{Offset: row.offset, Bit: bit, Name: row.name, Set: word&(1<<bit) != 0})
		}
	case "fan_fault_bits":
		for bit := uint8(0); bit < 4; bit++ {
			*flags = append(*flags, ProtocolIITableFlag{Offset: row.offset, Bit: bit, Name: "fan_fault", Set: word&(1<<bit) != 0})
		}
		for bit := uint8(4); bit < 16; bit++ {
			if word&(1<<bit) != 0 {
				*rawBits = append(*rawBits, ProtocolIITableRawBit{Offset: row.offset, Bit: bit, Word: word})
			}
		}
	case "pid_fault_code":
		names := []string{"output_overvoltage", "isolation", "bus_voltage"}
		for bit, name := range names {
			*flags = append(*flags, ProtocolIITableFlag{Offset: row.offset, Bit: uint8(bit), Name: name, Set: word&(1<<uint(bit)) != 0})
		}
		for bit := uint8(3); bit < 16; bit++ {
			if word&(1<<bit) != 0 {
				*rawBits = append(*rawBits, ProtocolIITableRawBit{Offset: row.offset, Bit: bit, Word: word})
			}
		}
	case "string_prompt":
		names := []string{"unmatch", "disconnect", "current_unbalance"}
		for bit, name := range names {
			*flags = append(*flags, ProtocolIITableFlag{Offset: row.offset, Bit: uint8(bit), Name: name, Set: word&(1<<uint(bit)) != 0})
		}
		for bit := uint8(3); bit < 16; bit++ {
			if word&(1<<bit) != 0 {
				*rawBits = append(*rawBits, ProtocolIITableRawBit{Offset: row.offset, Bit: bit, Word: word})
			}
		}
	default:
		*rawBits = append(*rawBits, ProtocolIITableRawBit{Offset: row.offset, Word: word})
	}
}

// growattProtocolIIEmitCompensation is declared beside the word 206 row.
func growattProtocolIIEmitCompensation(codes *[]ProtocolIITableCode, raw *[]ProtocolIITableRawWord, row protocolIITableRow, word uint16) bool {
	var code string
	switch word & 0xff {
	case 0:
		code = "none"
	case 1:
		code = "first_mode"
	case 2:
		code = "second_mode"
	case 3:
		code = "both"
	default:
		return false
	}
	*codes = append(*codes, ProtocolIITableCode{Offset: row.offset, Name: row.name, Code: code, Raw: word})
	*raw = append(*raw, ProtocolIITableRawWord{Offset: row.offset, Name: "compensation_ratio", Word: word >> 8})
	return true
}

// growattProtocolIIEmitPIDStatus is declared beside the word 141 row.
func growattProtocolIIEmitPIDStatus(codes *[]ProtocolIITableCode, rawBits *[]ProtocolIITableRawBit, row protocolIITableRow, word uint16) bool {
	text, ok := growattProtocolIIEnum("pid_status", word&0xff)
	if !ok {
		return false
	}
	*codes = append(*codes, ProtocolIITableCode{Offset: row.offset, Name: row.name, Code: text, Raw: word})
	for bit := uint8(8); bit < 16; bit++ {
		if word&(1<<bit) != 0 {
			*rawBits = append(*rawBits, ProtocolIITableRawBit{Offset: row.offset, Bit: bit, Word: word})
		}
	}
	return true
}

func growattProtocolIIApplyRow(words []uint16, offset uint16, name string) (ProtocolIITableReading, bool) {
	var match *protocolIITableRow
	for index := range protocolIIRowTable {
		row := &protocolIIRowTable[index]
		if row.offset != offset || row.name != name {
			continue
		}
		if match != nil {
			return ProtocolIITableReading{}, false
		}
		match = row
	}
	if match == nil {
		return ProtocolIITableReading{}, false
	}
	value, ok := growattProtocolIIScaled(words, *match)
	if !ok {
		return ProtocolIITableReading{}, false
	}
	return ProtocolIITableReading{Offset: match.offset, Width: match.width, Name: match.name, Unit: match.unit, Value: value, Qualifier: match.qualifier}, true
}
