package huawei

import (
	"errors"
	"strings"
	"testing"
)

func TestActivePowerSelection(t *testing.T) {
	cards, err := Cards()
	if err != nil {
		t.Fatal(err)
	}
	bound := map[string]bool{"MA-V200": true, "MB02": true, "MC": true}
	unbound := map[string]bool{
		"COMMERCIAL": true, "HA": true, "HB": true, "HD": true, "L-LEGACY": true,
		"L-V200": true, "LB-C00": true, "LB-C10": true, "LC": true, "MA-C00": true,
		"MA-C20": true, "SUN-C81": true, "SUN-V300": true,
	}
	if len(cards) != len(bound)+len(unbound) {
		t.Fatalf("cards %d", len(cards))
	}
	for _, card := range cards {
		row, ok, err := BoundActivePower(card)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case bound[card]:
			if !ok || !strings.HasSuffix(row.Key, ".fixed_active_limit_watts") || row.FunctionalClass != "active_power" {
				t.Fatalf("%s bound %#v", card, row)
			}
			if row.Offset != 40126 || row.Unit != "W" || row.Gain != "1" || row.Type != "U32" || row.Quantity != 2 || row.Access != "RW" || !row.WayBack {
				t.Fatalf("%s row %#v", card, row)
			}
			if card == "MB02" {
				if row.LowerKnown || row.UpperKnown {
					t.Fatalf("MB02 limits %#v", row)
				}
			} else if !row.LowerKnown || row.LowerWatts != "0" || row.UpperKnown {
				t.Fatalf("%s range %#v", card, row)
			}
		case unbound[card]:
			if ok {
				t.Fatalf("%s bound %#v", card, row)
			}
		default:
			t.Fatalf("card %s is neither bound nor unbound", card)
		}
	}
	legacy, ok, err := BoundActivePower("L-LEGACY")
	if err != nil || ok {
		t.Fatalf("dispatch row %#v %v", legacy, err)
	}
	commercial, ok, err := BoundActivePower("COMMERCIAL")
	if err != nil || ok {
		t.Fatalf("commercial %#v %v", commercial, err)
	}
}

func TestEncodeActivePowerLimit(t *testing.T) {
	words, err := EncodeActivePowerLimit("MA-V200", Watts{Coefficient: "5000"})
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 2 || words[0] != 0x0000 || words[1] != 0x1388 {
		t.Fatalf("5000 W %v", words)
	}
	for _, card := range []string{"MB02", "MC"} {
		got, err := EncodeActivePowerLimit(card, Watts{Coefficient: "5000"})
		if err != nil || len(got) != 2 || got[0] != words[0] || got[1] != words[1] {
			t.Fatalf("%s %v %v", card, got, err)
		}
	}
	maxWords, err := EncodeActivePowerLimit("MB02", Watts{Coefficient: "4294967295"})
	if err != nil || len(maxWords) != 2 || maxWords[0] != 0xffff || maxWords[1] != 0xffff {
		t.Fatalf("max %v %v", maxWords, err)
	}
	if _, err := EncodeActivePowerLimit("MB02", Watts{Coefficient: "4294967296"}); !errors.Is(err, ErrUnrepresentable) {
		t.Fatalf("overflow %v", err)
	}
	if _, err := EncodeActivePowerLimit("MA-V200", Watts{Coefficient: "5", Exponent10: -1}); !errors.Is(err, ErrUnrepresentable) {
		t.Fatalf("fraction %v", err)
	}
	if _, err := EncodeActivePowerLimit("MA-V200", Watts{Coefficient: "-1"}); err == nil {
		t.Fatal("negative encoded")
	}
	if _, err := EncodeActivePowerLimit("L-LEGACY", Watts{Coefficient: "5000"}); err == nil {
		t.Fatal("dispatch row encoded")
	}
	if _, err := EncodeActivePowerLimit("COMMERCIAL", Watts{Coefficient: "5000"}); err == nil {
		t.Fatal("unbound card encoded")
	}
	decoded, err := DecodeActivePowerWatts("MA-V200", words)
	if err != nil || decoded.Coefficient != "5" || decoded.Exponent10 != 3 {
		t.Fatalf("decode %#v %v", decoded, err)
	}
	reversed := []uint16{words[1], words[0]}
	if CompareActivePowerReadback("MA-V200", words, reversed) != "mismatch" {
		t.Fatal("reversed words matched")
	}
	reversedWatts, err := DecodeActivePowerWatts("MA-V200", reversed)
	if err != nil || (reversedWatts.Coefficient == "5" && reversedWatts.Exponent10 == 3) {
		t.Fatalf("reversed decode %#v %v", reversedWatts, err)
	}
	if CompareActivePowerReadback("MA-V200", words, words) != "match" {
		t.Fatal("readback did not match")
	}
	if CompareActivePowerReadback("MA-V200", words, []uint16{0x0000}) != "unknown" {
		t.Fatal("short read was not unknown")
	}
}

func TestActivePowerReleaseEncoding(t *testing.T) {
	queried := Watts{Coefficient: "10000"}
	rated := Watts{Coefficient: "8000"}
	words, err := EncodeActivePowerRelease("MA-V200", &queried, &rated)
	if err != nil || len(words) != 2 || words[0] != 0x0000 || words[1] != 0x2710 {
		t.Fatalf("queried release %v %v", words, err)
	}
	if _, err := EncodeActivePowerRelease("MA-V200", nil, &rated); !errors.Is(err, ErrReleaseUnknown) {
		t.Fatalf("rated release %v", err)
	}
	for _, card := range []string{"MA-V200", "MC", "MB02"} {
		if _, err := EncodeActivePowerRelease(card, nil, nil); !errors.Is(err, ErrReleaseUnknown) {
			t.Fatalf("%s release %v", card, err)
		}
	}
	if _, err := ActivePowerReleaseValue("MA-V200", nil, &rated); !errors.Is(err, ErrReleaseUnknown) {
		t.Fatal(err)
	}
	got, err := ActivePowerReleaseValue("MC", &queried, &rated)
	if err != nil || got != queried {
		t.Fatalf("release value %#v %v", got, err)
	}
}

// TestAdmitActivePowerWrite varies every field of ControlWrite.
// Each case starts from the bound MA-V200 write and changes one field.
// card: a predicate that binds by offset 40126 admits L-LEGACY dispatch.40126.
// path: a predicate that skips the configured path admits "other".
// identified unit: a predicate that ignores IdentifiedUnit admits a different unit.
// unit: a predicate that ignores Unit admits a different unit.
// function: a predicate that skips the function code admits function 6.
// address: a predicate that admits every active_power row admits 40125 percent.
// quantity: a predicate that skips the row quantity admits one register.
func TestAdmitActivePowerWrite(t *testing.T) {
	bound := ControlWrite{Card: "MA-V200", Path: "S-Dongle", IdentifiedUnit: 1, Unit: 1, Function: FunctionWriteMultipleRegisters, Address: 40126, Quantity: 2}
	if !AdmitActivePowerWrite(bound) || !AdmitControlWrite(bound) {
		t.Fatal("bound write refused")
	}
	for _, path := range ConfiguredPaths() {
		write := bound
		write.Path = path
		if !AdmitActivePowerWrite(write) {
			t.Fatalf("path %s refused", path)
		}
	}
	for _, card := range []string{"MB02", "MC"} {
		write := bound
		write.Card = card
		if !AdmitActivePowerWrite(write) {
			t.Fatalf("%s refused", card)
		}
	}
	matched := bound
	matched.IdentifiedUnit = 2
	matched.Unit = 2
	if !AdmitActivePowerWrite(matched) {
		t.Fatal("matching unit refused")
	}
	cases := []struct {
		name string
		edit func(*ControlWrite)
	}{
		{"card", func(w *ControlWrite) { w.Card = "L-LEGACY" }},
		{"path", func(w *ControlWrite) { w.Path = "other" }},
		{"identified-unit", func(w *ControlWrite) { w.IdentifiedUnit = 9 }},
		{"unit", func(w *ControlWrite) { w.Unit = 9 }},
		{"function", func(w *ControlWrite) { w.Function = 6 }},
		{"address", func(w *ControlWrite) { w.Address = 40125 }},
		{"quantity", func(w *ControlWrite) { w.Quantity = 1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			write := bound
			tc.edit(&write)
			if AdmitActivePowerWrite(write) || AdmitControlWrite(write) {
				t.Fatalf("%s mutation admitted %#v", tc.name, write)
			}
		})
	}
	for _, address := range []uint16{40120, 40125, 40199, 42021, 42023, 42027, 47415, 47416, 47418} {
		write := bound
		write.Address = address
		if AdmitActivePowerWrite(write) {
			t.Fatalf("address %d admitted", address)
		}
	}
	legacy := bound
	legacy.Card = "L-LEGACY"
	if AdmitActivePowerWrite(legacy) || AdmitControlWrite(legacy) {
		t.Fatal("dispatch 40126 admitted")
	}
	commercial := bound
	commercial.Card = "COMMERCIAL"
	commercial.Address = 40120
	if AdmitActivePowerWrite(commercial) {
		t.Fatal("commercial dispatch admitted")
	}
	if AdmitActivePowerWrite(ControlWrite{}) {
		t.Fatal("empty write admitted")
	}
}
