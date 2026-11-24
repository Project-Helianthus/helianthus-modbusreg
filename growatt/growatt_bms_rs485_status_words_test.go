package growatt

import (
	"reflect"
	"testing"
)

func TestGrowattBMSStatusWordsNamedConditions(t *testing.T) {
	for _, row := range growattBMSConditionRows {
		input := validGrowattBMSTypedReadOnlyInput()
		word := uint16(1) << row.Bit
		growattBMSSetWord(&input, row.Offset, word)
		status, err := DecodeGrowattBMSTypedReadOnlyStatus(input)
		if err != nil {
			t.Fatalf("bit %s: %v", row.Name, err)
		}
		found := false
		for _, condition := range status.StatusWords.Conditions {
			if condition.Offset == row.Offset && condition.Bit == row.Bit && condition.Name == row.Name && condition.Set {
				found = true
			}
			if condition.Set && (condition.Offset != row.Offset || condition.Bit != row.Bit) {
				t.Fatalf("bit %s also set %#v", row.Name, condition)
			}
		}
		if !found {
			t.Fatalf("bit %s was not named", row.Name)
		}
	}
}

func TestGrowattBMSStatusWordsFlagsExtensionAndRawBit(t *testing.T) {
	input := validGrowattBMSTypedReadOnlyInput()
	input.Slices[1].Words[0x001f-0x000d] = 0x4000
	input.Slices[2].Words[0x0109-0x0100] = 0x0008
	status, err := DecodeGrowattBMSTypedReadOnlyStatus(input)
	if err != nil {
		t.Fatal(err)
	}
	if !status.StatusWords.BatteryIDOK || status.StatusWords.BatteryID != 0 {
		t.Fatalf("battery id %#v", status.StatusWords)
	}
	if !bmsFlagSet(status.StatusWords.Flags, 0x001F, 14, "reserved") || bmsFlagSet(status.StatusWords.Flags, 0x001F, 15, "reserved") {
		t.Fatalf("reserved flags %#v", status.StatusWords.Flags)
	}
	if !bmsFlagSet(status.StatusWords.Flags, 0x0109, 3, "cell_balance") || bmsFlagSet(status.StatusWords.Flags, 0x0109, 0, "cell_balance") {
		t.Fatalf("balance flags %#v", status.StatusWords.Flags)
	}
	want := map[uint16]struct {
		name, unit string
		value      float64
	}{
		0x0100: {"continuous_charge_time", "s", 100},
		0x0101: {"current_cycle_charge_capacity", "Ah", 12.3},
		0x0102: {"average_cell_voltage", "mV", 3300},
		0x0104: {"floating_pack_voltage", "V", 51.2},
		0x0105: {"cumulative_charge_capacity", "Ah", 0.5},
		0x0106: {"cumulative_discharge_capacity", "Ah", 0.6},
	}
	for _, word := range status.StatusWords.ExtensionWords {
		expected, ok := want[word.Offset]
		if !ok {
			t.Fatalf("unexpected extension %#v", word)
		}
		if word.Name != expected.name || word.Unit != expected.unit || word.Value != expected.value {
			t.Fatalf("extension %#v want %#v", word, expected)
		}
		delete(want, word.Offset)
	}
	if len(want) != 0 {
		t.Fatalf("missing extensions %#v", want)
	}
	input = validGrowattBMSTypedReadOnlyInput()
	input.Slices[1].Words[0x0012-0x000d] = 0x0001
	status, err = DecodeGrowattBMSTypedReadOnlyStatus(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.StatusWords.Conditions) != len(growattBMSConditionRows) {
		t.Fatal("undefined bit became a condition")
	}
	raw, ok := bmsRawWord(status.StatusWords.RawWords, 0x0012)
	if !ok || raw.Word != 1 {
		t.Fatalf("undefined bit raw=%#v", status.StatusWords.RawWords)
	}
	if status.OperatingState != BMSStateCharging || status.SOCPercent != 75 {
		t.Fatal("status words replaced the typed observation")
	}
}

func TestGrowattBMSStatusWordsRejectMalformedBase(t *testing.T) {
	input := validGrowattBMSTypedReadOnlyInput()
	input.UnitID = 0
	status, err := DecodeGrowattBMSTypedReadOnlyStatus(input)
	if err == nil || !reflect.DeepEqual(status, BMSTypedReadOnlyStatus{}) {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func growattBMSSetWord(input *BMSReadOnlyInput, offset, word uint16) {
	switch {
	case offset >= 0x000d && int(offset-0x000d) < len(input.Slices[1].Words):
		input.Slices[1].Words[offset-0x000d] = word
	case offset >= 0x0100 && int(offset-0x0100) < len(input.Slices[2].Words):
		input.Slices[2].Words[offset-0x0100] = word
	case offset >= 0x010d && int(offset-0x010d) < len(input.Slices[3].Words):
		input.Slices[3].Words[offset-0x010d] = word
	}
}

func bmsFlagSet(flags []BMSNamedBit, offset uint16, bit uint8, name string) bool {
	for _, flag := range flags {
		if flag.Offset == offset && flag.Bit == bit && flag.Name == name {
			return flag.Set
		}
	}
	return false
}

func bmsRawWord(words []BMSRawWord, offset uint16) (BMSRawWord, bool) {
	for _, word := range words {
		if word.Offset == offset {
			return word, true
		}
	}
	return BMSRawWord{}, false
}
