package modbusreg

import (
	"fmt"
	"strings"
	"testing"
)

// h03SyntheticEntry pads text with NUL bytes to the observed object extent.
func h03SyntheticEntry(text string) []byte {
	value := make([]byte, 254)
	copy(value, text)
	return value
}

func h03SyntheticEntryText(index, gatewayUnit int) string {
	if index == 0 {
		return fmt.Sprintf("1=Smart Logger;2=V300R023C10SPC620;5=%d;9=0", gatewayUnit)
	}
	return fmt.Sprintf("1=SUN2000-50KTL-M3;5=%d;8=SUN2000;9=0", index+gatewayUnit)
}

// h03SyntheticInput builds a listing of the given number of entries (the self
// entry included) with perPage entries in each response after the count page.
func h03SyntheticInput(entries, perPage int) HuaweiSmartLoggerObservedInventoryInput {
	input := HuaweiSmartLoggerObservedInventoryInput{ChangeCounterBefore: 7, ChangeCounterAfter: 7}
	input.Pages = append(input.Pages, HuaweiSmartLoggerObservedInventoryPage{
		ResponseBytes: 10, DeclaredObjects: 1, More: true, NextObjectID: 0x88,
		Objects: []HuaweiSmartLoggerInventoryObject{{ObjectID: 0x87, Value: []byte{byte(entries)}}},
	})
	for first := 0; first < entries; first += perPage {
		page := HuaweiSmartLoggerObservedInventoryPage{ResponseBytes: 7}
		for index := first; index < entries && index < first+perPage; index++ {
			page.Objects = append(page.Objects, HuaweiSmartLoggerInventoryObject{
				ObjectID: uint8(0x88 + index), Value: h03SyntheticEntry(h03SyntheticEntryText(index, 0)),
			})
			page.ResponseBytes += 2 + 254
		}
		page.DeclaredObjects = len(page.Objects)
		input.Pages = append(input.Pages, page)
	}
	for index := range input.Pages {
		last := index == len(input.Pages)-1
		input.Pages[index].More = !last
		if !last {
			input.Pages[index].NextObjectID = input.Pages[index+1].Objects[0].ObjectID
		} else {
			input.Pages[index].NextObjectID = 0
		}
	}
	return input
}

func h03CloneInput(input HuaweiSmartLoggerObservedInventoryInput) HuaweiSmartLoggerObservedInventoryInput {
	clone := input
	clone.Pages = make([]HuaweiSmartLoggerObservedInventoryPage, len(input.Pages))
	for index, page := range input.Pages {
		clone.Pages[index] = page
		clone.Pages[index].Objects = make([]HuaweiSmartLoggerInventoryObject, len(page.Objects))
		for objectIndex, object := range page.Objects {
			clone.Pages[index].Objects[objectIndex] = HuaweiSmartLoggerInventoryObject{ObjectID: object.ObjectID, Value: append([]byte(nil), object.Value...)}
		}
	}
	return clone
}

func h03RequireParseRejected(t *testing.T, label string, input HuaweiSmartLoggerObservedInventoryInput) {
	t.Helper()
	parsed, err := parseHuaweiSmartLoggerObservedInventory(0, input)
	if err == nil || parsed.inventory.ChildCount() != 0 || parsed.inventory.DeclaredChildren() != 0 || len(parsed.inventory.Children()) != 0 ||
		parsed.anomalies != (HuaweiInventoryPageAnomalies{}) {
		t.Fatalf("%s: expected an empty rejected inventory, got %#v err=%v", label, parsed, err)
	}
}

// Each subtest names a tolerance requirement of the observation record and the
// production behavior it protects.
func TestH03ObservedInventoryAcceptsEveryToleranceRequirement(t *testing.T) {
	t.Run("responses of 263 bytes and above, up to the registry cap", func(t *testing.T) {
		parsed, err := parseHuaweiSmartLoggerObservedInventory(0, h03SyntheticInput(3, 1))
		if err != nil || parsed.anomalies.OversizeResponses != 3 {
			t.Fatalf("one 254-byte entry per response: %#v %v", parsed, err)
		}
		parsed, err = parseHuaweiSmartLoggerObservedInventory(0, h03SyntheticInput(3, 3))
		if err != nil || parsed.anomalies.OversizeResponses != 1 {
			t.Fatalf("a 775-byte response: %#v %v", parsed, err)
		}
		// 7 + 4*256 = 1031 bytes is above the cap of 1024 bytes.
		h03RequireParseRejected(t, "1031-byte response", h03SyntheticInput(4, 4))
		// A response of exactly 1024 bytes is accepted and one more byte is not:
		// 7 + 3*(2+254) + (2+247) = 1024.
		capped := h03SyntheticInput(4, 4)
		fourth := h03SyntheticEntryText(3, 0)
		capped.Pages[1].Objects[3].Value = append([]byte(fourth), make([]byte, 247-len(fourth))...)
		capped.Pages[1].ResponseBytes = 1024
		if _, err := parseHuaweiSmartLoggerObservedInventory(0, capped); err != nil {
			t.Fatalf("1024-byte response: %v", err)
		}
		capped.Pages[1].Objects[3].Value = append(capped.Pages[1].Objects[3].Value, 0)
		capped.Pages[1].ResponseBytes = 1025
		h03RequireParseRejected(t, "1025-byte response", capped)
	})

	t.Run("the declared-object field is retained and never compared", func(t *testing.T) {
		for _, declared := range []int{0, 1, 2, 13, 255} {
			input := h03SyntheticInput(3, 1)
			for index := range input.Pages {
				input.Pages[index].DeclaredObjects = declared
			}
			parsed, err := parseHuaweiSmartLoggerObservedInventory(0, input)
			if err != nil {
				t.Fatalf("declared %d: %v", declared, err)
			}
			want := len(input.Pages)
			if declared == 1 {
				want = 0
			}
			if parsed.anomalies.DeclaredObjectFieldDiffers != want {
				t.Fatalf("declared %d: differing pages=%d want=%d", declared, parsed.anomalies.DeclaredObjectFieldDiffers, want)
			}
		}
	})

	t.Run("the count object includes the self entry", func(t *testing.T) {
		parsed, err := parseHuaweiSmartLoggerObservedInventory(0, h03SyntheticInput(5, 1))
		if err != nil || parsed.inventory.DeclaredChildren() != 4 || parsed.inventory.ChildCount() != 4 {
			t.Fatalf("five entries: %#v %v", parsed, err)
		}
		parsed, err = parseHuaweiSmartLoggerObservedInventory(0, h03SyntheticInput(1, 1))
		if err != nil || parsed.inventory.DeclaredChildren() != 0 || parsed.inventory.ChildCount() != 0 {
			t.Fatalf("self entry only: %#v %v", parsed, err)
		}
		for _, count := range []byte{0, 2, 6} {
			input := h03SyntheticInput(5, 1)
			input.Pages[0].Objects[0].Value = []byte{count}
			h03RequireParseRejected(t, fmt.Sprintf("count %d for five entries", count), input)
		}
	})

	t.Run("keys 9 and 10 are known opaque keys and key order is not validated", func(t *testing.T) {
		for _, text := range []string{
			"9=0;10=ABCD;1=Smart Logger;2=V300R023C10SPC620;5=0",
			"5=0;2=V300R023C10SPC620;1=Smart Logger;10=ABCD;9=0;4=0;3=0;6=0",
			"10=0;9=0;8=SUN2000;6=0;5=0;4=0;3=0;2=V300R023C10SPC620;1=Smart Logger",
		} {
			input := h03SyntheticInput(2, 1)
			input.Pages[1].Objects[0].Value = h03SyntheticEntry(text)
			if _, err := parseHuaweiSmartLoggerObservedInventory(0, input); err != nil {
				t.Fatalf("%q: %v", text, err)
			}
		}
		for _, text := range []string{
			"1=Smart Logger;5=0;7=0", "1=Smart Logger;5=0;11=0", "1=Smart Logger;5=0;1=Smart Logger", "1=Smart Logger;5=0;9=0;9=0",
			"1=Smart Logger;5=0;=0", "1=Smart Logger;5=0;x=0",
		} {
			input := h03SyntheticInput(2, 1)
			input.Pages[1].Objects[0].Value = h03SyntheticEntry(text)
			h03RequireParseRejected(t, text, input)
		}
	})

	t.Run("one object of up to 254 bytes per response", func(t *testing.T) {
		input := h03SyntheticInput(2, 1)
		longest := "1=Smart Logger;2=V300R023C10SPC620;5=0;9=" + strings.Repeat("0", 254-len("1=Smart Logger;2=V300R023C10SPC620;5=0;9="))
		if len(longest) != 254 {
			t.Fatalf("test entry is %d bytes", len(longest))
		}
		input.Pages[1].Objects[0].Value = []byte(longest)
		if _, err := parseHuaweiSmartLoggerObservedInventory(0, input); err != nil {
			t.Fatalf("254-byte unpadded entry: %v", err)
		}
		input = h03SyntheticInput(2, 1)
		input.Pages[1].Objects[0].Value = []byte(longest + "0")
		input.Pages[1].ResponseBytes = 7 + 2 + 255
		h03RequireParseRejected(t, "255-byte entry", input)
	})
}

func TestH03ObservedInventoryRejectsMalformedStructure(t *testing.T) {
	valid := h03SyntheticInput(4, 1)
	if _, err := parseHuaweiSmartLoggerObservedInventory(0, valid); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*HuaweiSmartLoggerObservedInventoryInput)
	}{
		{"counter before differs", func(i *HuaweiSmartLoggerObservedInventoryInput) { i.ChangeCounterAfter++ }},
		{"counter wrap", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.ChangeCounterBefore, i.ChangeCounterAfter = 0xffff, 0
		}},
		{"no pages", func(i *HuaweiSmartLoggerObservedInventoryInput) { i.Pages = nil }},
		{"response size one too small", func(i *HuaweiSmartLoggerObservedInventoryInput) { i.Pages[1].ResponseBytes-- }},
		{"response size one too large", func(i *HuaweiSmartLoggerObservedInventoryInput) { i.Pages[1].ResponseBytes++ }},
		{"response size unset", func(i *HuaweiSmartLoggerObservedInventoryInput) { i.Pages[2].ResponseBytes = 0 }},
		{"count object not first", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[0].Objects[0].ObjectID = 0x88
			i.Pages[0].NextObjectID = 0x89
		}},
		{"count object two bytes", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[0].Objects[0].Value = []byte{4, 0}
			i.Pages[0].ResponseBytes++
		}},
		{"count above the ceiling", func(i *HuaweiSmartLoggerObservedInventoryInput) { i.Pages[0].Objects[0].Value = []byte{249} }},
		{"empty response", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects = nil
			i.Pages[2].ResponseBytes = 7
		}},
		{"skipped object", func(i *HuaweiSmartLoggerObservedInventoryInput) { i.Pages[2].Objects[0].ObjectID = 0x8a }},
		{"repeated object", func(i *HuaweiSmartLoggerObservedInventoryInput) { i.Pages[2].Objects[0].ObjectID = 0x88 }},
		{"cursor loop", func(i *HuaweiSmartLoggerObservedInventoryInput) { i.Pages[1].NextObjectID = 0x87 }},
		{"cursor skips an object", func(i *HuaweiSmartLoggerObservedInventoryInput) { i.Pages[1].NextObjectID = 0x8a }},
		{"final response continues", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[len(i.Pages)-1].More = true
			i.Pages[len(i.Pages)-1].NextObjectID = 0x8c
		}},
		{"final response names a next object", func(i *HuaweiSmartLoggerObservedInventoryInput) { i.Pages[len(i.Pages)-1].NextObjectID = 0x8c }},
		{"inner response ends the stream", func(i *HuaweiSmartLoggerObservedInventoryInput) { i.Pages[1].More = false; i.Pages[1].NextObjectID = 0 }},
		{"response after the end", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			extra := i.Pages[len(i.Pages)-1]
			extra.Objects = []HuaweiSmartLoggerInventoryObject{{ObjectID: 0x8c, Value: h03SyntheticEntry(h03SyntheticEntryText(4, 0))}}
			i.Pages = append(i.Pages, extra)
		}},
		{"missing entry", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages = i.Pages[:len(i.Pages)-1]
			i.Pages[len(i.Pages)-1].More, i.Pages[len(i.Pages)-1].NextObjectID = false, 0
		}},
		{"entry text empty", func(i *HuaweiSmartLoggerObservedInventoryInput) { i.Pages[2].Objects[0].Value = h03SyntheticEntry("") }},
		{"entry text only spaces", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects[0].Value = h03SyntheticEntry("   ")
		}},
		{"empty value", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects[0].Value = h03SyntheticEntry("1=;5=1")
		}},
		{"no equals sign", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects[0].Value = h03SyntheticEntry("1=SUN2000;5")
		}},
		{"trailing delimiter", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects[0].Value = h03SyntheticEntry("1=SUN2000;5=1;")
		}},
		{"value with equals", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects[0].Value = h03SyntheticEntry("1=SUN=2000;5=1")
		}},
		{"nul inside the text", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects[0].Value = h03SyntheticEntry("1=SUN\x002000;5=1")
		}},
		{"control character", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects[0].Value = h03SyntheticEntry("1=SUN\t2000;5=1")
		}},
		{"non-ASCII byte", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects[0].Value = h03SyntheticEntry("1=SUN\xc32000;5=1")
		}},
		{"no model", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects[0].Value = h03SyntheticEntry("5=1;8=SUN2000")
		}},
		{"no unit", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects[0].Value = h03SyntheticEntry("1=SUN2000;8=SUN2000")
		}},
		{"unit with leading zero", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects[0].Value = h03SyntheticEntry("1=SUN2000;5=01")
		}},
		{"unit with sign", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects[0].Value = h03SyntheticEntry("1=SUN2000;5=+1")
		}},
		{"unit with space", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects[0].Value = h03SyntheticEntry("1=SUN2000;5= 1")
		}},
		{"unit above 247", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects[0].Value = h03SyntheticEntry("1=SUN2000;5=248")
		}},
		{"unit four digits", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects[0].Value = h03SyntheticEntry("1=SUN2000;5=1000")
		}},
		{"child on the gateway unit", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[2].Objects[0].Value = h03SyntheticEntry("1=SUN2000;5=0")
		}},
		{"repeated child unit", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[3].Objects[0].Value = h03SyntheticEntry("1=SUN2000;5=1")
		}},
		{"self entry on another unit", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[1].Objects[0].Value = h03SyntheticEntry("1=Smart Logger;5=1")
		}},
		{"self entry unit with leading zero", func(i *HuaweiSmartLoggerObservedInventoryInput) {
			i.Pages[1].Objects[0].Value = h03SyntheticEntry("1=Smart Logger;5=00")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := h03CloneInput(valid)
			test.mutate(&input)
			h03RequireParseRejected(t, test.name, input)
		})
	}
}

func TestH03ObservedInventoryBoundsAndObjectIdentifierWrap(t *testing.T) {
	// 248 entries (the self entry and 247 children) cross the 0xff to 0x00 wrap.
	parsed, err := parseHuaweiSmartLoggerObservedInventory(0, h03SyntheticInput(248, 2))
	if err != nil || parsed.inventory.ChildCount() != 247 || parsed.inventory.DeclaredChildren() != 247 {
		t.Fatalf("maximum listing: %#v %v", parsed.inventory, err)
	}
	// 247 entries, one per response, make exactly 248 responses with the count response.
	if _, err := parseHuaweiSmartLoggerObservedInventory(0, h03SyntheticInput(247, 1)); err != nil {
		t.Fatalf("248 responses: %v", err)
	}
	h03RequireParseRejected(t, "249 responses", h03SyntheticInput(248, 1))
	h03RequireParseRejected(t, "249 entries", h03SyntheticInput(249, 2))

	// The sequence is strict across the wrap.
	input := h03SyntheticInput(248, 2)
	for index := range input.Pages {
		for objectIndex := range input.Pages[index].Objects {
			if input.Pages[index].Objects[objectIndex].ObjectID == 0x00 {
				input.Pages[index].Objects[objectIndex].ObjectID = 0x01
			}
		}
	}
	h03RequireParseRejected(t, "wrap skips object 0x00", input)
}

func TestH03ObservedInventoryRetainsOnlyNonSensitiveChildFields(t *testing.T) {
	input := h03SyntheticInput(2, 1)
	input.Pages[2].Objects[0].Value = h03SyntheticEntry("1=SUN2000-50KTL-M3;2=V200R023C00SPC128;3=P;4=SECRET0123;5=7;6=F;8=SUN2000;9=0")
	parsed, err := parseHuaweiSmartLoggerObservedInventory(0, input)
	if err != nil {
		t.Fatal(err)
	}
	child := parsed.inventory.Children()[0]
	if child.ObjectID() != 0x89 || child.Address() != "7" || child.Attribute("model") != "SUN2000-50KTL-M3" || child.Attribute("software_version") != "V200R023C00SPC128" ||
		child.Attribute("protocol_version") != "P" || child.Attribute("feature_version") != "F" || child.Attribute("product_type") != "SUN2000" || child.Attribute("4") != "" {
		t.Fatalf("unexpected child: %#v", child)
	}
	if strings.Contains(fmt.Sprintf("%#v", parsed), "SECRET0123") {
		t.Fatal("the sensitive attribute was retained")
	}
	if parsed.self.Address() != "0" || parsed.self.Attribute("model") != "Smart Logger" {
		t.Fatalf("unexpected self entry: %#v", parsed.self)
	}
}

func TestH03ObservedInventoryBindsRoutingToTheReadUnit(t *testing.T) {
	// The parser is not tied to unit 0: the self entry must name the unit the
	// listing was read on, and no child may name that unit or unit 0.
	build := func(self, child string) HuaweiSmartLoggerObservedInventoryInput {
		input := h03SyntheticInput(2, 1)
		input.Pages[1].Objects[0].Value = h03SyntheticEntry("1=Smart Logger;2=V300R023C10SPC620;5=" + self + ";9=0")
		input.Pages[2].Objects[0].Value = h03SyntheticEntry("1=SUN2000-50KTL-M3;5=" + child + ";8=SUN2000;9=0")
		return input
	}
	if parsed, err := parseHuaweiSmartLoggerObservedInventory(5, build("5", "6")); err != nil || parsed.inventory.ChildCount() != 1 || parsed.self.Address() != "5" {
		t.Fatalf("control for unit 5: %#v %v", parsed, err)
	}
	for _, test := range []struct{ name, self, child string }{
		{"child on unit 0", "5", "0"},
		{"child on the read unit", "5", "5"},
		{"self entry on unit 0", "0", "6"},
		{"self entry on another unit", "6", "7"},
	} {
		if parsed, err := parseHuaweiSmartLoggerObservedInventory(5, build(test.self, test.child)); err == nil || parsed.inventory.ChildCount() != 0 {
			t.Fatalf("%s: %#v", test.name, parsed)
		}
	}
	// The card applies to unit 0 only, even when the listing is self-consistent on another unit.
	card := NewHuaweiSmartLoggerObservedQualificationCard()
	for _, unit := range []uint8{1, 5, 100, 247} {
		input := HuaweiSmartLoggerObservedQualificationInput{UnitID: unit, Inventory: build(fmt.Sprint(unit), "2")}
		if unit == 2 {
			input.Inventory = build(fmt.Sprint(unit), "3")
		}
		if _, err := parseHuaweiSmartLoggerObservedInventory(unit, input.Inventory); err != nil {
			t.Fatalf("unit %d: the listing must be valid on its own unit: %v", unit, err)
		}
		h03RequireRejected(t, fmt.Sprintf("self-consistent exact tuple read on unit %d", unit), card.Evaluate(input))
	}
	control := HuaweiSmartLoggerObservedQualificationInput{UnitID: 0, Inventory: build("0", "2")}
	if result := card.Evaluate(control); !result.IdentityMatched() {
		t.Fatalf("the same listing on unit 0 must be admitted: %#v", result)
	}
}

func TestH03ObservedInventoryStripsOnlyTerminalPadding(t *testing.T) {
	// The unit is the last attribute, so padding that is not stripped would corrupt it.
	text := "9=0;1=Smart Logger;2=V300R023C10SPC620;5=0"
	card := NewHuaweiSmartLoggerObservedQualificationCard()
	padded := func(pad string) HuaweiSmartLoggerObservedQualificationInput {
		input := h03SyntheticInput(2, 1)
		value := []byte(text + strings.Repeat(pad, 254-len(text)))
		input.Pages[1].Objects[0].Value = value
		return HuaweiSmartLoggerObservedQualificationInput{UnitID: 0, Inventory: input}
	}
	for _, pad := range []string{"\x00", " "} {
		if result := card.Evaluate(padded(pad)); !result.IdentityMatched() {
			t.Fatalf("terminal padding %q was not stripped: %#v", pad, result)
		}
	}
	// Padding between attributes belongs to the value, so the tuple no longer matches.
	for _, inner := range []string{
		"1=Smart Logger ;2=V300R023C10SPC620;5=0;9=0",
		"1=Smart Logger;2=V300R023C10SPC620 ;5=0;9=0",
		"1=Smart Logger;2=V300R023C10SPC620;5=0 ;9=0",
		"1=Smart Logger;2= V300R023C10SPC620;5=0;9=0",
		"1=Smart Logger;2=V300R023C10SPC620;5=0;9=0;",
	} {
		input := h03SyntheticInput(2, 1)
		input.Pages[1].Objects[0].Value = h03SyntheticEntry(inner)
		h03RequireRejected(t, inner, card.Evaluate(HuaweiSmartLoggerObservedQualificationInput{UnitID: 0, Inventory: input}))
	}
}
