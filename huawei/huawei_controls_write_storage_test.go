package huawei

import "testing"

// TestStorageLimitSelection binds only the MA-V200 charge and discharge rows.
// Each listed offset is asserted on its own so a production change that binds
// one of them fails that case:
//   - card: L-V200 at 47247 stays unbound; only MA-V200 binds 47247 and 47249
//   - offset 47075 and 47077 stay unbound on every card
//   - offset 47242, 47084 and 47079 stay unbound
//   - offset 47247 and 47249 stay unbound on every card other than MA-V200
func TestStorageLimitSelection(t *testing.T) {
	cards, err := Cards()
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) == 0 {
		t.Fatal("no cards")
	}
	none := map[string]bool{"COMMERCIAL": true, "HA": true, "HB": true, "HD": true, "MB02": true, "MC": true, "SUN-C81": true}
	sawMA := false
	for _, card := range cards {
		ins, err := Inspect(card)
		if err != nil {
			t.Fatal(err)
		}
		if card == "MA-V200" {
			sawMA = true
			assertMAV200(t, ins)
			continue
		}
		if len(ins.Bound) != 0 {
			t.Fatalf("%s bound %v", card, ins.Bound)
		}
		for _, row := range ins.Unbound {
			switch row.Offset {
			case 47075, 47077, 47247, 47249, 47242, 47084, 47079:
			default:
				if row.Operation != "" {
					t.Fatalf("%s offset %d bound", card, row.Offset)
				}
			}
			if row.Operation != "" {
				t.Fatalf("%s offset %d operation %s", card, row.Offset, row.Operation)
			}
			if row.Reason == "" {
				t.Fatalf("%s offset %d has no reason", card, row.Offset)
			}
		}
		assertNotLimits(t, ins)
		if none[card] && len(ins.Unbound) != 0 {
			t.Fatalf("%s states a storage power control %#v", card, ins.Unbound)
		}
	}
	if !sawMA {
		t.Fatal("MA-V200 missing")
	}
	for _, card := range []string{"L-LEGACY", "MA-C20", "SUN-V300"} {
		row, ok := unboundOffset(t, card, 47079)
		if !ok {
			t.Fatalf("%s has no storage 47079", card)
		}
		if row.Reason != "the card does not state its meaning, so it is not bound" {
			t.Fatalf("%s 47079 reason %q", card, row.Reason)
		}
		if row.Unit != "W" || row.Gain != "1" {
			t.Fatalf("%s 47079 unit %s gain %s", card, row.Unit, row.Gain)
		}
	}
}

func assertMAV200(t *testing.T, ins Inspection) {
	t.Helper()
	if len(ins.Bound) != 2 {
		t.Fatalf("bound %#v", ins.Bound)
	}
	charge, discharge := ins.Bound[0], ins.Bound[1]
	if charge.Offset != 47247 || charge.Operation != OpSetChargeLimit || discharge.Offset != 47249 || discharge.Operation != OpSetDischargeLimit {
		t.Fatalf("bound %#v", ins.Bound)
	}
	for _, row := range []StorageRow{charge, discharge} {
		if row.Unit != "kW" || row.Gain != "1000" || row.Type != "UINT32" || row.Access != "RW" || row.Quantity != 2 || !row.WayBack {
			t.Fatalf("bound shape %#v", row)
		}
		if !row.LowerKnown || row.LowerWatts != "0" || row.UpperKnown || row.UpperWatts != "" {
			t.Fatalf("limit sides %#v", row)
		}
	}
	if charge.Condition != "Range0..maximum charge power." || discharge.Condition != "Range0..maximum discharge power." {
		t.Fatalf("conditions %q %q", charge.Condition, discharge.Condition)
	}
	left := mustUnbound(t, ins, 47075)
	right := mustUnbound(t, ins, 47077)
	if left.Condition == "" || left.Condition != right.Condition {
		t.Fatalf("47075/47077 conditions %q %q", left.Condition, right.Condition)
	}
	if left.Reason != "MA-V200 gives both one condition, so charge and discharge are not distinguished" {
		t.Fatalf("47075 reason %q", left.Reason)
	}
	for _, offset := range []int{47242, 47084, 47244, 47251, 47253, 47321} {
		row := mustUnbound(t, ins, offset)
		if row.Reason == "" {
			t.Fatalf("%d missing reason", offset)
		}
	}
	if mustUnbound(t, ins, 47242).Reason != "MA-V200 states a grid-charge range. Not either operation." {
		t.Fatal(mustUnbound(t, ins, 47242).Reason)
	}
	if mustUnbound(t, ins, 47084).Reason != "MA-V200 states a signed forced-power span. Not either limit." {
		t.Fatal(mustUnbound(t, ins, 47084).Reason)
	}
	assertNotLimits(t, ins)
	for _, offset := range []int{37046, 37048} {
		if _, ok := findUnbound(ins, offset); ok {
			t.Fatalf("%d is a read scalar and is not a storage control", offset)
		}
		for _, row := range ins.Bound {
			if row.Offset == offset {
				t.Fatalf("%d bound", offset)
			}
		}
	}
}

func assertNotLimits(t *testing.T, ins Inspection) {
	t.Helper()
	for _, row := range ins.Ratings {
		if row.Offset != 37007 && row.Offset != 37009 {
			t.Fatalf("rating %d", row.Offset)
		}
		if row.Operation != "" || !row.Rating || row.Unit != "W" || row.Gain != "1" {
			t.Fatalf("rating row %#v", row)
		}
	}
	for _, row := range ins.NotLimits {
		if row.Offset != 30073 && row.Offset != 37001 {
			t.Fatalf("not-limit %d", row.Offset)
		}
		if row.Operation != "" || !row.NotALimit {
			t.Fatalf("not-limit row %#v", row)
		}
	}
	if ins.Card == "MA-V200" {
		if len(ins.Ratings) != 2 || len(ins.NotLimits) != 2 {
			t.Fatalf("ratings %#v not-limits %#v", ins.Ratings, ins.NotLimits)
		}
	}
}

func mustUnbound(t *testing.T, ins Inspection, offset int) StorageRow {
	t.Helper()
	row, ok := findUnbound(ins, offset)
	if !ok {
		t.Fatalf("offset %d not unbound in %#v", offset, ins.Unbound)
	}
	return row
}

func unboundOffset(t *testing.T, card string, offset int) (StorageRow, bool) {
	t.Helper()
	ins, err := Inspect(card)
	if err != nil {
		t.Fatal(err)
	}
	return findUnbound(ins, offset)
}

func findUnbound(ins Inspection, offset int) (StorageRow, bool) {
	for _, row := range ins.Unbound {
		if row.Offset == offset {
			return row, true
		}
	}
	return StorageRow{}, false
}

// TestAdmitControlWrite varies every field of the write identity.
// A mutation of card, path, unit, function, address or quantity that leaves
// the other fields at the bound MA-V200 charge write is refused.
// AdmitStorageControlWrite refuses offset 40126; AdmitControlWrite admits it
// only where the card's row is functional class active_power.
func TestAdmitControlWrite(t *testing.T) {
	bound := ControlWrite{Card: "MA-V200", Path: "S-Dongle", IdentifiedUnit: 1, Unit: 1, Function: FunctionWriteMultipleRegisters, Address: 47247, Quantity: 2}
	if !AdmitStorageControlWrite(bound) || !AdmitControlWrite(bound) {
		t.Fatal("bound charge write refused")
	}
	discharge := bound
	discharge.Address = 47249
	if !AdmitStorageControlWrite(discharge) {
		t.Fatal("bound discharge write refused")
	}
	for _, path := range ConfiguredPaths() {
		write := bound
		write.Path = path
		if !AdmitStorageControlWrite(write) {
			t.Fatalf("path %s refused", path)
		}
	}
	cases := []struct {
		name string
		edit func(*ControlWrite)
	}{
		{"card", func(w *ControlWrite) { w.Card = "L-V200" }},
		{"path", func(w *ControlWrite) { w.Path = "other" }},
		{"unit", func(w *ControlWrite) { w.Unit = 2 }},
		{"function", func(w *ControlWrite) { w.Function = 6 }},
		{"address", func(w *ControlWrite) { w.Address = 47075 }},
		{"quantity", func(w *ControlWrite) { w.Quantity = 1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			write := bound
			tc.edit(&write)
			if AdmitStorageControlWrite(write) || AdmitControlWrite(write) {
				t.Fatalf("%s mutation admitted %#v", tc.name, write)
			}
		})
	}
	matched := bound
	matched.IdentifiedUnit = 2
	matched.Unit = 2
	if !AdmitStorageControlWrite(matched) {
		t.Fatal("matching identified unit refused")
	}
	for _, offset := range []uint16{47075, 47077, 47242, 47084, 47079, 40126} {
		write := bound
		write.Address = offset
		if AdmitStorageControlWrite(write) {
			t.Fatalf("storage admission admitted %d", offset)
		}
	}
	active := bound
	active.Address = 40126
	if AdmitStorageControlWrite(active) || !AdmitControlWrite(active) {
		t.Fatal("active-power admission")
	}
	for _, card := range []string{"MB02", "MC"} {
		write := active
		write.Card = card
		if !AdmitControlWrite(write) || AdmitStorageControlWrite(write) {
			t.Fatalf("%s active-power admission", card)
		}
	}
	legacy := active
	legacy.Card = "L-LEGACY"
	if AdmitControlWrite(legacy) || AdmitStorageControlWrite(legacy) {
		t.Fatal("dispatch 40126 admitted")
	}
	short := active
	short.Quantity = 1
	if AdmitControlWrite(short) {
		t.Fatal("active-power quantity admitted")
	}
}

func TestEncodeStorageControl(t *testing.T) {
	words, err := EncodeStorageControl("MA-V200", 47247, Watts{Coefficient: "25", Exponent10: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 2 || words[0] != 0 || words[1] != 0x09C4 {
		t.Fatalf("charge words %v", words)
	}
	words, err = EncodeStorageControl("MA-V200", 47249, Watts{Coefficient: "18", Exponent10: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 2 || words[0] != 0 || words[1] != 0x0708 {
		t.Fatalf("discharge words %v", words)
	}
	if _, err := EncodeStorageControl("MA-V200", 47247, Watts{Coefficient: "5", Exponent10: -1}); err == nil {
		t.Fatal("fractional watts encoded")
	}
	if _, err := EncodeStorageControl("MA-V200", 47247, Watts{Coefficient: "-1"}); err == nil {
		t.Fatal("negative watts encoded")
	}
	for _, offset := range []int{47075, 47077, 47242, 47084, 47079, 40126, 37007, 30073} {
		if _, err := EncodeStorageControl("MA-V200", offset, Watts{Coefficient: "25", Exponent10: 2}); err == nil {
			t.Fatalf("encoded unbound %d", offset)
		}
	}
	if _, err := EncodeStorageControl("L-V200", 47247, Watts{Coefficient: "25", Exponent10: 2}); err == nil {
		t.Fatal("encoded unbound card")
	}
	decoded, err := DecodeStorageWatts("MA-V200", 47247, []uint16{0, 0x09C4})
	if err != nil || decoded.Coefficient != "25" || decoded.Exponent10 != 2 {
		t.Fatalf("decode %#v %v", decoded, err)
	}
	if CompareStorageReadback("MA-V200", 47247, []uint16{0, 0x09C4}, []uint16{0, 0x09C4}) != "match" {
		t.Fatal("match")
	}
	if CompareStorageReadback("MA-V200", 47247, []uint16{0, 0x09C4}, []uint16{0, 1}) != "mismatch" {
		t.Fatal("mismatch")
	}
	if CompareStorageReadback("MA-V200", 47075, []uint16{0, 1}, []uint16{0, 1}) != "unknown" {
		t.Fatal("unbound compare")
	}
	if CompareStorageReadback("L-V200", 47247, []uint16{0, 1}, []uint16{0, 1}) != "unknown" {
		t.Fatal("unbound card compare")
	}
}
