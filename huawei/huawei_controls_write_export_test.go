package huawei

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExportRowsFollowTheCard(t *testing.T) {
	want := map[string]bool{
		"MA-V200": true, "MC": true,
		"emma-v100r024c00-01-2024-07-15": true, "emma-v100r025c00spc102-2025-08-14": true,
	}
	seen := map[string]bool{}
	var valueRow, blankScope, noCondition bool
	for _, path := range exportCatalogFiles(t) {
		card, rows := exportCatalogCard(t, path)
		seen[card] = true
		ins := selectExport(card, rows)
		if ins.Bound != want[card] {
			t.Fatalf("%s bound %v reasons %+v", card, ins.Bound, ins.Unbound)
		}
		product, ok, err := BoundExportLimit(card)
		if err != nil {
			t.Fatal(err)
		}
		if ok != ins.Bound {
			t.Fatalf("%s product %v card %v", card, ok, ins.Bound)
		}
		if ins.Bound && !sameExport(product, ins.Limit) {
			t.Fatalf("%s product %#v card %#v", card, product, ins.Limit)
		}
		for _, note := range ins.Unbound {
			switch {
			case note.Reason == "the card states a grid feed-in or grid-power limit and does not identify the value row":
				valueRow = true
			case note.Offset == 47419 && note.Reason == "the card states that the source scope is blank, so the row is not a scope selector":
				blankScope = true
			case note.Offset == 47415 && note.Reason == "the card states no condition for this offset, so the row is not bound":
				noCondition = true
			}
		}
		if !ins.Bound {
			continue
		}
		assertExportShape(t, ins.Limit)
	}
	for card := range want {
		if !seen[card] {
			t.Fatalf("card %s was not read", card)
		}
	}
	if !valueRow || !blankScope || !noCondition {
		t.Fatalf("notes value-row %v blank-scope %v no-condition %v", valueRow, blankScope, noCondition)
	}
}

func TestExportAmbiguousModeIsNotBound(t *testing.T) {
	power := emmaFeedInRows[2]
	scope := emmaFeedInRows[1]
	repeated := catalogRow{Key: exportModeKey, Offset: 40100, Quantity: 1, Type: "ENUM16", Access: "RW", Class: "readable-configuration", Literal: &scale{Gain: "NA", Unit: "NA"}, Condition: "6 feed-in limit in kW; 6 feed-in limit in percent."}
	two := catalogRow{Key: exportModeKey, Offset: 40100, Quantity: 1, Type: "ENUM16", Access: "RW", Class: "readable-configuration", Literal: &scale{Gain: "NA", Unit: "NA"}, Condition: "5 feed-in limit in kW; 6 feed-in limit in kW; 7 feed-in limit in percent."}
	for _, mode := range []catalogRow{repeated, two} {
		ins := selectExport("synthetic", []catalogRow{mode, power, scope, emmaFeedInRows[3]})
		if ins.Bound {
			t.Fatalf("ambiguous mode bound %#v", ins.Limit)
		}
		if len(ins.Unbound) == 0 || ins.Unbound[0].Reason == "" {
			t.Fatalf("ambiguous mode reported nothing %+v", ins.Unbound)
		}
	}
}

func TestEncodeExportLimit(t *testing.T) {
	mode, value, err := EncodeExportLimit("MA-V200", Watts{Coefficient: "5000"})
	if err != nil {
		t.Fatal(err)
	}
	if len(mode) != 1 || mode[0] != 6 || len(value) != 2 || value[0] != 0x0000 || value[1] != 0x1388 {
		t.Fatalf("5000 W mode %v value %v", mode, value)
	}
	for _, card := range []string{"MC", "emma-v100r024c00-01-2024-07-15", "emma-v100r025c00spc102-2025-08-14"} {
		gotMode, gotValue, err := EncodeExportLimit(card, Watts{Coefficient: "5", Exponent10: 3})
		if err != nil || len(gotMode) != 1 || gotMode[0] != mode[0] || len(gotValue) != 2 || gotValue[0] != value[0] || gotValue[1] != value[1] {
			t.Fatalf("%s mode %v value %v %v", card, gotMode, gotValue, err)
		}
	}
	negativeMode, negative, err := EncodeExportLimit("MC", Watts{Coefficient: "-1000"})
	if err != nil || len(negativeMode) != 1 || negativeMode[0] != 6 || len(negative) != 2 || negative[0] != 0xFFFF || negative[1] != 0xFC18 {
		t.Fatalf("negative %v %v %v", negativeMode, negative, err)
	}
	_, top, err := EncodeExportLimit("MA-V200", Watts{Coefficient: "2147483647"})
	if err != nil || len(top) != 2 || top[0] != 0x7FFF || top[1] != 0xFFFF {
		t.Fatalf("int32 max %v %v", top, err)
	}
	if _, _, err = EncodeExportLimit("MA-V200", Watts{Coefficient: "2147483648"}); !errors.Is(err, ErrUnrepresentable) {
		t.Fatalf("overflow %v", err)
	}
	if _, _, err = EncodeExportLimit("MC", Watts{Coefficient: "5", Exponent10: -1}); !errors.Is(err, ErrUnrepresentable) {
		t.Fatalf("fraction %v", err)
	}
	for _, card := range []string{"COMMERCIAL", "L-V200", "smartlogger-v300r024c10spc210-49-2025-03-19"} {
		if _, _, err = EncodeExportLimit(card, Watts{Coefficient: "5000"}); err == nil {
			t.Fatalf("%s encoded", card)
		}
	}
	decoded, err := DecodeExportWatts("MA-V200", value)
	if err != nil || decoded.Coefficient != "5" || decoded.Exponent10 != 3 {
		t.Fatalf("decode %#v %v", decoded, err)
	}
	decodedNegative, err := DecodeExportWatts("emma-v100r024c00-01-2024-07-15", negative)
	if err != nil || decodedNegative.Coefficient != "-1" || decodedNegative.Exponent10 != 3 {
		t.Fatalf("negative decode %#v %v", decodedNegative, err)
	}
	four, err := DecodeExportWatts("MC", []uint16{0x0000, 0x0FA0})
	if err != nil || four.Coefficient != "4" || four.Exponent10 != 3 {
		t.Fatalf("4000 W %#v %v", four, err)
	}
	if CompareExportReadback("MA-V200", "mode", mode, mode) != "match" || CompareExportReadback("MA-V200", "power", value, value) != "match" {
		t.Fatal("readback did not match")
	}
	if CompareExportReadback("MC", "power", value, []uint16{value[1], value[0]}) != "mismatch" {
		t.Fatal("reversed words matched")
	}
	if CompareExportReadback("MA-V200", "power", value, []uint16{0x0000}) != "unknown" || CompareExportReadback("MA-V200", "scope", value, value) != "unknown" {
		t.Fatal("short or unowned read was not unknown")
	}
	if CompareExportReadback("COMMERCIAL", "power", value, value) != "unknown" {
		t.Fatal("unbound card compared")
	}
}

// TestAdmitExportWrite varies every field of ControlWrite.
// Each case starts from a bound write and changes one field.
// card: a predicate that binds by offset admits L-V200 at 47416.
// path: a predicate that skips the configured path admits "other".
// identified unit: a predicate that ignores IdentifiedUnit admits a different unit.
// unit: a predicate that ignores Unit admits a different unit.
// function: a predicate that skips the function code admits function 6.
// address: a predicate that admits the scope or percent offset admits 40101 or 47418.
// quantity: a predicate that skips the row quantity admits the mode row as two words or the power row as one.
func TestAdmitExportWrite(t *testing.T) {
	plant := ControlWrite{Card: "MA-V200", Path: "S-Dongle", IdentifiedUnit: 1, Unit: 1, Function: FunctionWriteMultipleRegisters, Address: 47416, Quantity: 2}
	mode := plant
	mode.Address, mode.Quantity = 47415, 1
	if !AdmitExportWrite(plant) || !AdmitExportWrite(mode) || !AdmitControlWrite(plant) || !AdmitControlWrite(mode) {
		t.Fatal("bound plant write refused")
	}
	mc := plant
	mc.Card = "MC"
	mcMode := mode
	mcMode.Card = "MC"
	if !AdmitExportWrite(mc) || !AdmitExportWrite(mcMode) {
		t.Fatal("MC refused")
	}
	emma := ControlWrite{Card: "emma-v100r025c00spc102-2025-08-14", Path: "EMMA", IdentifiedUnit: 1, Unit: 1, Function: FunctionWriteMultipleRegisters, Address: 40107, Quantity: 2}
	emmaMode := emma
	emmaMode.Address, emmaMode.Quantity = 40100, 1
	older := emma
	older.Card = "emma-v100r024c00-01-2024-07-15"
	if !AdmitExportWrite(emma) || !AdmitExportWrite(emmaMode) || !AdmitExportWrite(older) || !AdmitControlWrite(emma) {
		t.Fatal("energy-manager write refused")
	}
	for _, path := range ConfiguredPaths() {
		write := plant
		write.Path = path
		if !AdmitExportWrite(write) {
			t.Fatalf("path %s refused", path)
		}
	}
	matched := plant
	matched.IdentifiedUnit, matched.Unit = 2, 2
	if !AdmitExportWrite(matched) {
		t.Fatal("matching unit refused")
	}
	cases := []struct {
		name  string
		start ControlWrite
		edit  func(*ControlWrite)
	}{
		{"card", plant, func(w *ControlWrite) { w.Card = "L-V200" }},
		{"path", plant, func(w *ControlWrite) { w.Path = "other" }},
		{"identified-unit", plant, func(w *ControlWrite) { w.IdentifiedUnit = 9 }},
		{"unit", plant, func(w *ControlWrite) { w.Unit = 9 }},
		{"function", plant, func(w *ControlWrite) { w.Function = 6 }},
		{"address-scope", emma, func(w *ControlWrite) { w.Address = 40101; w.Quantity = 1 }},
		{"address-percent", plant, func(w *ControlWrite) { w.Address = 47418; w.Quantity = 1 }},
		{"quantity-power", plant, func(w *ControlWrite) { w.Quantity = 1 }},
		{"quantity-mode", mode, func(w *ControlWrite) { w.Quantity = 2 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			write := tc.start
			tc.edit(&write)
			if AdmitExportWrite(write) || AdmitControlWrite(write) {
				t.Fatalf("%s mutation admitted %#v", tc.name, write)
			}
		})
	}
	active := plant
	active.Address, active.Quantity = 40126, 2
	if !AdmitControlWrite(active) || AdmitExportWrite(active) {
		t.Fatal("active-power row")
	}
	charge := plant
	charge.Address = 47247
	discharge := charge
	discharge.Address = 47249
	if !AdmitControlWrite(charge) || !AdmitControlWrite(discharge) || AdmitExportWrite(charge) {
		t.Fatal("storage row")
	}
	for _, address := range []uint16{40101, 40109, 47418, 47419} {
		write := emma
		write.Address, write.Quantity = address, 1
		if address == 47418 || address == 47419 {
			write = plant
			write.Address, write.Quantity = address, 1
		}
		if AdmitExportWrite(write) || AdmitControlWrite(write) {
			t.Fatalf("address %d admitted", address)
		}
	}
}

func TestExportFileDoesNotOpenTheCatalog(t *testing.T) {
	data, err := os.ReadFile("huawei_controls_write_export.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		`"github.com/Project-Helianthus/helianthus-modbusreg/profiles"`,
		"huawei/catalog",
		"registers-",
		"HuaweiInverterCatalog",
		"//go:embed",
	} {
		if bytes.Contains(data, []byte(marker)) {
			t.Fatalf("export selection opens the catalog via %s", marker)
		}
	}
}

func exportCatalogFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, pattern := range []string{
		"../profiles/vendors/huawei/catalog/registers-*.json",
		"../profiles/vendors/huawei/catalog/smartlogger-sdongle-emma/registers-*.json",
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, matches...)
	}
	if len(out) == 0 {
		t.Fatal("no catalogue files")
	}
	return out
}

func exportCatalogCard(t *testing.T, path string) (string, []catalogRow) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Card     string       `json:"card"`
		Controls []catalogRow `json:"controls"`
	}
	if err = json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw.Card == "" {
		t.Fatalf("%s has no card", path)
	}
	return raw.Card, raw.Controls
}

func assertExportShape(t *testing.T, lim ExportLimit) {
	t.Helper()
	if lim.Mode.Access != "RW" || lim.Mode.Class != "readable-configuration" || !lim.Mode.WayBack || lim.PowerMode != 6 {
		t.Fatalf("mode %#v code %d", lim.Mode, lim.PowerMode)
	}
	if lim.Power.Access != "RW" || lim.Power.Class != "readable-configuration" || lim.Power.Quantity != 2 || lim.Power.Gain != "1000" || !lim.Power.WayBack {
		t.Fatalf("power %#v", lim.Power)
	}
	if !lim.PercentUnreferenced || lim.Percent.Gain != "10" || lim.Percent.Unit != "%" {
		t.Fatalf("percent %#v unreferenced %v", lim.Percent, lim.PercentUnreferenced)
	}
	switch lim.Card {
	case "MA-V200":
		if lim.Scope != nil || !lim.PlantDispatch || lim.Power.Unit != "Kw" || lim.Power.Type != "INT32" || !lim.LowerKnown || lim.LowerWatts != "-1000000" || lim.UpperKnown || lim.Mode.Offset != 47415 || lim.Power.Offset != 47416 || lim.Percent.Offset != 47418 {
			t.Fatalf("MA-V200 %#v", lim)
		}
	case "MC":
		if lim.Scope != nil || !lim.PlantDispatch || lim.Power.Unit != "kW" || lim.Power.Type != "I32" || !lim.LowerKnown || lim.LowerWatts != "-1000000" || lim.UpperKnown || lim.Mode.Offset != 47415 || lim.Power.Offset != 47416 {
			t.Fatalf("MC %#v", lim)
		}
	default:
		if lim.Scope == nil || lim.PlantDispatch || lim.Scope.Offset != 40101 || lim.Total != 0 || lim.Phase != 1 || lim.Power.Unit != "kW" || lim.Power.Type != "I32" || lim.LowerKnown || lim.UpperKnown || lim.Mode.Offset != 40100 || lim.Power.Offset != 40107 || lim.Percent.Offset != 40109 {
			t.Fatalf("energy manager %#v", lim)
		}
	}
}

func sameExport(a, b ExportLimit) bool {
	if a.Card != b.Card || a.PowerMode != b.PowerMode || a.PlantDispatch != b.PlantDispatch || a.PercentUnreferenced != b.PercentUnreferenced {
		return false
	}
	if a.LowerKnown != b.LowerKnown || a.LowerWatts != b.LowerWatts || a.UpperKnown != b.UpperKnown || a.UpperWatts != b.UpperWatts || a.Total != b.Total || a.Phase != b.Phase {
		return false
	}
	if !sameExportRow(a.Mode, b.Mode) || !sameExportRow(a.Power, b.Power) || !sameExportRow(a.Percent, b.Percent) {
		return false
	}
	if (a.Scope == nil) != (b.Scope == nil) {
		return false
	}
	return a.Scope == nil || sameExportRow(*a.Scope, *b.Scope)
}

func sameExportRow(a, b ExportRegister) bool {
	return a.Key == b.Key && a.Offset == b.Offset && a.Quantity == b.Quantity && a.Type == b.Type && a.Access == b.Access && a.Class == b.Class && a.FunctionalClass == b.FunctionalClass && a.Unit == b.Unit && a.Gain == b.Gain && a.Condition == b.Condition && a.WayBack == b.WayBack
}
