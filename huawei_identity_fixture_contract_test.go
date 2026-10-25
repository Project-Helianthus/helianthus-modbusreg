package modbusreg

import (
	"bytes"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestH03FixturesDecodeStrictlyRoundTripAndRebuildWire(t *testing.T) {
	for _, name := range h03FixtureNames() {
		t.Run(name, func(t *testing.T) {
			data := h03Read1(t, name)
			var target any
			switch name {
			case "smartlogger_extended_inventory.json":
				target = &h03InventoryFixture{}
			case "smartlogger_basic_reply.json":
				target = &h03BasicFixture{}
			default:
				target = &h03ProbeFixture{}
			}
			if err := h03Decode(data, target); err != nil {
				t.Fatal(err)
			}
			canonical, err := h03Canonical(target)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(canonical, data) {
				t.Fatalf("%s is not the canonical encoding of its decoded form", name)
			}

			// Strict boundary: an unknown field, trailing data or a changed
			// schema string is rejected before any value is used.
			unknown := append([]byte(`{"unexpected":1,`), data[1:]...)
			fresh := reflect.New(reflect.TypeOf(target).Elem()).Interface()
			if err := h03Decode(unknown, fresh); err == nil {
				t.Fatal("unknown field was accepted")
			}
			fresh = reflect.New(reflect.TypeOf(target).Elem()).Interface()
			if err := h03Decode(append(append([]byte(nil), data...), []byte("{}")...), fresh); err == nil {
				t.Fatal("trailing JSON value was accepted")
			}
			if bytes.Contains(data, []byte(`"0x`)) {
				fresh = reflect.New(reflect.TypeOf(target).Elem()).Interface()
				if err := h03Decode(bytes.Replace(data, []byte(`"0x`), []byte(`"0X`), 1), fresh); err == nil {
					t.Fatal("upper-case hex byte was accepted")
				}
			}
		})
	}
}

func TestH03InventoryFixtureRebuildsWireAndDigestCoversEveryByte(t *testing.T) {
	fixture := h03LoadInventory(t)
	if err := h03CheckHeader(fixture.h03Header, "mei_extended_inventory", "SmartLogger"); err != nil {
		t.Fatal(err)
	}
	pdus, err := h03InventoryPDUs(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for index, pdu := range pdus {
		parsed, err := h03ParseMEI(pdu)
		if err != nil {
			t.Fatal(err)
		}
		page := fixture.Pages[index]
		if parsed.conformity != uint8(page.Conformity) || parsed.more != uint8(page.MoreFollows) || parsed.next != uint8(page.NextObjectID) ||
			int(parsed.declared) != page.DeclaredObjects || parsed.length != page.PDUBytes || len(parsed.objects) != len(page.Objects) {
			t.Fatalf("page %d does not parse back to its fixture: %+v", index, parsed)
		}
		for objectIndex, object := range parsed.objects {
			want, err := h03ObjectValue(page.Objects[objectIndex])
			if err != nil {
				t.Fatal(err)
			}
			if object.ObjectID != uint8(page.Objects[objectIndex].ObjectID) || !bytes.Equal(object.Value, want) {
				t.Fatalf("page %d object %d changed in the wire round trip", index, objectIndex)
			}
		}
	}

	// The digest is computed, never stored: it is stable for equal input and
	// changes when any single bit of any response changes.
	original := h03Digest(pdus)
	again, _ := h03InventoryPDUs(h03LoadInventory(t))
	if h03Digest(again) != original {
		t.Fatal("digest is not deterministic")
	}
	for index := range pdus {
		for offset := range pdus[index] {
			mutated := make([][]byte, len(pdus))
			for copyIndex := range pdus {
				mutated[copyIndex] = append([]byte(nil), pdus[copyIndex]...)
			}
			mutated[index][offset] ^= 0x01
			if h03Digest(mutated) == original {
				t.Fatalf("digest ignored page %d byte %d", index, offset)
			}
		}
	}
	if h03Digest(pdus[:len(pdus)-1]) == original {
		t.Fatal("digest ignored a missing page")
	}
}

func TestH03InventoryFixtureCarriesObservedOversizeStructure(t *testing.T) {
	fixture := h03LoadInventory(t)
	if fixture.GatewayUnit != 0 || fixture.ChangeCounter.Before != fixture.ChangeCounter.After ||
		fixture.Read.Function != "FC2B" || fixture.Read.MEIType != 0x0e || fixture.Read.ReadDeviceIDCode != 3 || fixture.Read.StartObjectID != 0x87 {
		t.Fatalf("unexpected read definition: %+v", fixture)
	}
	if len(fixture.Pages) != 13 {
		t.Fatalf("pages=%d want 13", len(fixture.Pages))
	}
	oversize := 0
	var units []string
	for index, page := range fixture.Pages {
		if len(page.Objects) != 1 || uint8(page.RequestObjectID) != uint8(page.Objects[0].ObjectID) || uint8(page.RequestObjectID) != uint8(0x87+index) ||
			page.Conformity != 0x03 || page.DeclaredObjects != 13 {
			t.Fatalf("page %d header: %+v", index, page)
		}
		last := index == len(fixture.Pages)-1
		if (!last && (page.MoreFollows != 0xff || uint8(page.NextObjectID) != uint8(page.RequestObjectID)+1)) ||
			(last && (page.MoreFollows != 0x00 || page.NextObjectID != 0x00)) {
			t.Fatalf("page %d continuation: %+v", index, page)
		}
		if page.PDUBytes > 253 {
			oversize++
		}
		if index == 0 {
			if page.PDUBytes != 10 || page.Objects[0].Count == nil || *page.Objects[0].Count != 12 {
				t.Fatalf("count page: %+v", page)
			}
			continue
		}
		if page.PDUBytes != 263 || page.Objects[0].PadTo != 254 || page.Objects[0].Entry == "" {
			t.Fatalf("entry page %d: %+v", index, page)
		}
		units = append(units, h03AttributeValue(page.Objects[0].Entry, "5"))
	}
	if oversize != 12 {
		t.Fatalf("oversize pages=%d want 12", oversize)
	}
	if want := []string{"0", "2", "3", "4", "5", "6", "7", "8", "9", "1", "11", "19"}; !reflect.DeepEqual(units, want) {
		t.Fatalf("entry units in listing order=%v want=%v", units, want)
	}
	wantKeys := map[string]string{
		"Smart Logger":     "1;2;3;4;5;6;10;9",
		"SUN2000-50KTL-M3": "1;2;3;4;5;6;8;9",
		"PLC":              "1;2;3;4;5;6;9",
		"PowerMeter":       "1;2;3;4;5;6;9",
	}
	wantPad := map[string]int{"Smart Logger": 166, "SUN2000-50KTL-M3": 164, "PLC": 187, "PowerMeter": 178}
	labels := map[string]int{}
	for _, page := range fixture.Pages[1:] {
		entry := page.Objects[0].Entry
		label := h03AttributeValue(entry, "1")
		labels[label]++
		var keys []string
		for _, pair := range h03EntryAttributes(entry) {
			keys = append(keys, pair[0])
		}
		if got := strings.Join(keys, ";"); got != wantKeys[label] {
			t.Fatalf("%s key order %q want %q", label, got, wantKeys[label])
		}
		if pad := page.Objects[0].PadTo - len(entry); pad != wantPad[label] {
			t.Fatalf("%s pad %d want %d", label, pad, wantPad[label])
		}
		if (label == "SUN2000-50KTL-M3") != (h03AttributeValue(entry, "8") == "SUN2000") {
			t.Fatalf("product type attribute is not exclusive to the inverter entries: %q", entry)
		}
	}
	if !reflect.DeepEqual(labels, map[string]int{"Smart Logger": 1, "SUN2000-50KTL-M3": 8, "PLC": 1, "PowerMeter": 2}) {
		t.Fatalf("labels=%v", labels)
	}
}

func h03AttributeValue(entry, key string) string {
	for _, pair := range h03EntryAttributes(entry) {
		if pair[0] == key {
			return pair[1]
		}
	}
	return ""
}

func TestH03BasicReplyFixtureCarriesTwelveObjectsAnsweredForEveryUnit(t *testing.T) {
	fixture := h03LoadBasic(t)
	if err := h03CheckHeader(fixture.h03Header, "mei_basic_reply", "SmartLogger"); err != nil {
		t.Fatal(err)
	}
	pdu, err := h03BasicPDU(fixture.Read, fixture.Reply)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := h03ParseMEI(pdu)
	if err != nil {
		t.Fatal(err)
	}
	var ids []uint8
	for _, object := range parsed.objects {
		ids = append(ids, object.ObjectID)
	}
	wantIDs := []uint8{0x00, 0x01, 0x02, 0x03, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x17, 0x19}
	if !reflect.DeepEqual(ids, wantIDs) || parsed.length != 94 || parsed.conformity != 0x01 || parsed.more != 0 || parsed.next != 0 ||
		parsed.readDeviceIDCode != 1 || int(parsed.declared) != len(parsed.objects) {
		t.Fatalf("basic reply structure: ids=%v %+v", ids, parsed)
	}
	if string(parsed.objects[0].Value) != "HUAWEI" || string(parsed.objects[1].Value) != h03Model || string(parsed.objects[2].Value) != h03Firmware ||
		len(parsed.objects[2].Value) != 17 || len(parsed.objects[4].Value) != 12 {
		t.Fatalf("basic identity objects: %v", parsed.objects[:3])
	}
	extra := 0
	for _, object := range parsed.objects[3:] {
		extra++
		if object.ObjectID != 0x10 && len(object.Value) != 2 {
			t.Fatalf("opaque object %02x has length %d", object.ObjectID, len(object.Value))
		}
	}
	if extra != 9 {
		t.Fatalf("objects beyond the basic category=%d want 9", extra)
	}
	if !reflect.DeepEqual(fixture.AnsweredForUnits, []int{0, 1, 100}) {
		t.Fatalf("answered for units %v", fixture.AnsweredForUnits)
	}
}

func TestH03ProbeFixturesCarryObservedProbes(t *testing.T) {
	sun2000 := h03LoadProbes(t, "sun2000_routed_identity.json")
	overlap := h03LoadProbes(t, "smartlogger_negative_overlap.json")
	if err := h03CheckHeader(sun2000.h03Header, "fc03_probe_set", "SUN2000"); err != nil {
		t.Fatal(err)
	}
	if err := h03CheckHeader(overlap.h03Header, "fc03_probe_set", "SmartLogger"); err != nil {
		t.Fatal(err)
	}
	all := append(append([]h03Probe(nil), sun2000.Probes...), overlap.Probes...)
	growattOffsets := map[uint16]bool{9: true, 23: true, 43: true, 82: true, 88: true}
	units := map[int]bool{}
	for _, probe := range all {
		if probe.Function != "FC03" || len(probe.Units) == 0 || len(probe.Roles) == 0 {
			t.Fatalf("unexpected probe: %+v", probe)
		}
		if growattOffsets[probe.Offset] {
			t.Fatalf("a Growatt offset was probed: %+v", probe)
		}
		pdu, err := h03ProbePDU(probe)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := h03ParseFC03(pdu, probe.Quantity)
		if err != nil {
			t.Fatal(err)
		}
		switch probe.Outcome.Type {
		case "exception":
			if parsed.exception != probe.Outcome.Code || (probe.Outcome.Code != 3 && probe.Outcome.Code != 4) {
				t.Fatalf("exception probe: %+v", probe)
			}
		case "text":
			if len(parsed.data) != int(probe.Quantity)*2 || h03TrimNUL(parsed.data) != probe.Outcome.Text || bytes.Count(parsed.data, []byte{0}) < 1 {
				t.Fatalf("text probe leaves no padding: %+v", probe)
			}
		}
		for _, unit := range probe.Units {
			units[unit] = true
		}
	}
	if units[19] || units[247] {
		t.Fatal("unit 19 was never read in the observation and must not appear")
	}

	byRole := func(set h03ProbeFixture, role string) []h03Probe {
		var found []h03Probe
		for _, probe := range set.Probes {
			for _, candidate := range probe.Roles {
				if candidate == role {
					found = append(found, probe)
				}
			}
		}
		return found
	}
	model, firmware, protocol := byRole(sun2000, "identity_model"), byRole(sun2000, "identity_firmware"), byRole(sun2000, "identity_protocol")
	if len(model) != 3 || len(firmware) != 1 || len(protocol) != 1 {
		t.Fatalf("SUN2000 probes: model=%d firmware=%d protocol=%d", len(model), len(firmware), len(protocol))
	}
	eight := []int{1, 2, 3, 4, 5, 6, 7, 8}
	if !reflect.DeepEqual(model[0].Units, eight) || model[0].Offset != 30000 || model[0].Quantity != 15 || model[0].Outcome.Text != "SUN2000-50KTL-M3" ||
		!reflect.DeepEqual(firmware[0].Units, eight) || firmware[0].Offset != 30035 || firmware[0].Quantity != 15 || firmware[0].Outcome.Text != "V200R023" ||
		!reflect.DeepEqual(protocol[0].Units, eight) || protocol[0].Offset != 30068 || protocol[0].Quantity != 2 || !reflect.DeepEqual(protocol[0].Outcome.Words, []uint16{5, 0}) {
		t.Fatalf("SUN2000 identity probes: %+v %+v %+v", model[0], firmware[0], protocol[0])
	}
	if !reflect.DeepEqual(model[1].Units, []int{9, 11}) || model[1].Outcome.Code != 3 || !reflect.DeepEqual(model[2].Units, []int{10, 12}) || model[2].Outcome.Code != 4 {
		t.Fatalf("listed and unlisted unit exceptions: %+v %+v", model[1], model[2])
	}
	if len(byRole(overlap, "smartlogger_counter")) != 1 || len(byRole(overlap, "sunspec_marker")) != 2 {
		t.Fatalf("counter probes=%d marker probes=%d", len(byRole(overlap, "smartlogger_counter")), len(byRole(overlap, "sunspec_marker")))
	}
}

func TestH03CommittedFixturesAreSanitized(t *testing.T) {
	inventory := h03LoadInventory(t)
	for index, page := range inventory.Pages {
		for _, object := range page.Objects {
			if object.Count != nil {
				continue
			}
			if err := h03CheckEntrySanitized(object); err != nil {
				t.Fatalf("page %d: %v", index, err)
			}
		}
	}
	basic := h03LoadBasic(t)
	for _, object := range basic.Reply.Objects {
		identity := object.ObjectID <= 0x02
		if identity == object.Sanitized || (object.Sanitized && object.Text != "" && !h03FillText.MatchString(object.Text)) {
			t.Fatalf("basic object %02x is neither a published identity object nor a placeholder: %+v", uint8(object.ObjectID), object)
		}
	}
	for _, name := range []string{"sun2000_routed_identity.json", "smartlogger_negative_overlap.json"} {
		for _, probe := range h03LoadProbes(t, name).Probes {
			switch probe.Outcome.Type {
			case "text":
				if probe.Outcome.Text != "SUN2000-50KTL-M3" && probe.Outcome.Text != "V200R023" {
					t.Fatalf("%s: text %q is not a published identity value", name, probe.Outcome.Text)
				}
			case "words":
				published := reflect.DeepEqual(probe.Outcome.Words, []uint16{5, 0}) || reflect.DeepEqual(probe.Outcome.Words, []uint16{0, 0})
				if !published && !probe.Outcome.Synthetic {
					t.Fatalf("%s: data words %v are neither published nor marked synthetic", name, probe.Outcome.Words)
				}
			case "exception":
			default:
				t.Fatalf("%s: unknown outcome %q", name, probe.Outcome.Type)
			}
		}
	}

	scanned := 0
	for _, path := range h03CommittedHuaweiTestdata(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		if findings := h03ShapeFindings(string(data)); len(findings) != 0 {
			t.Fatalf("%s holds serial-, ESN-, address- or time-shaped values: %v", path, findings)
		}
	}
	if scanned < 6 {
		t.Fatalf("scanned %d files; the identity and qualification fixtures are missing", scanned)
	}
}

func TestH03ShapeScanRejectsSerialEsnAndAddressShapes(t *testing.T) {
	forbidden := map[string]string{
		"twelve-digit serial":        `"4": "` + h03CountingDigits(12) + `"`,
		"mixed alphanumeric serial":  `"4": "` + h03Joined("AB", "12", "CD", "34", "EF", "56") + `"`,
		"eight-character code":       `"10": "` + h03Joined("AB", "1", "CD", "2", "EF") + `"`,
		"ten-digit ESN":              `"esn": "` + h03CountingDigits(10) + `"`,
		"digest literal":             `"digest": "` + strings.Repeat("a1", 32) + `"`,
		"ipv4 address":               `"endpoint": "192.0.2.10"`,
		"ipv4 with port":             `"endpoint": "198.51.100.7:502"`,
		"full ipv6 address":          `"endpoint": "2001:0db8:0000:0000:0000:0000:0000:0001"`,
		"compressed ipv6 address":    `"endpoint": "2001:db8::1"`,
		"mac address":                `"link": "00:1a:2b:3c:4d:5e"`,
		"host name":                  `"endpoint": "logger.example.com"`,
		"local host name":            `"endpoint": "logger.local"`,
		"url":                        `"locator": "https://example.org/x"`,
		"account":                    `"owner": "someone@example.org"`,
		"date":                       `"observed": "2026-10-24"`,
		"clock time":                 `"observed": "17:16:00"`,
		"host and port":              `"endpoint": "logger:502"`,
		"serial inside a longer run": `"note": "unit SN` + h03CountingDigits(10) + `AB"`,
	}
	for name, text := range forbidden {
		if len(h03ShapeFindings(text)) == 0 {
			t.Errorf("%s was not found in %s", name, text)
		}
	}
	allowed := []string{
		`"2": "V300R023C10SPC620"`, `"1": "SUN2000-50KTL-M3"`, `"1": "Smart Logger"`, `"4": "000000000000"`, `"4": "0X0000000000"`,
		`"4": "XX_XXX_XXXXXX"`, `"3": "X0.00-X0.0"`, `"object_id": "0x88"`, `"card_id": "huawei.qualification.smartlogger.observed-readonly@1.0.0"`,
		`"schema": "helianthus-huawei-identity-fixture/v1"`, `"license": "CC0-1.0"`, `"units": [1, 2, 3]`, `"offset": 65521`,
		`"words": [5, 0]`, `"pad_to": 254`, `"firmware": "V200R023"`,
	}
	for _, text := range allowed {
		if findings := h03ShapeFindings(text); len(findings) != 0 {
			t.Errorf("%s was flagged: %v", text, findings)
		}
	}

	// The structural check catches the same values inside an entry.
	valid := h03LoadInventory(t).Pages[1].Objects[0]
	for _, mutate := range []func(*h03Object){
		func(o *h03Object) {
			o.Entry = strings.Replace(o.Entry, "4=000000000000", "4="+h03CountingDigits(12), 1)
		},
		func(o *h03Object) {
			o.Entry = strings.Replace(o.Entry, "10=XXXX0XXX", "10="+h03Joined("AB", "1", "CD", "2", "EF"), 1)
		},
		func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "3=X0.00-X0.0", "3=Q9.99-Q9.9", 1) },
		func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "6=0.0", "6=9.9", 1) },
		func(o *h03Object) { o.SanitizedAttributes = []string{"3", "6", "10"} },
		func(o *h03Object) { o.SanitizedAttributes = append(o.SanitizedAttributes, "8") },
		func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "5=0", "5=192.0.2.1", 1) },
		func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "1=Smart Logger", "1=Other Logger", 1) },
		func(o *h03Object) { o.Entry += ";7=0" },
	} {
		object := valid
		object.SanitizedAttributes = append([]string(nil), valid.SanitizedAttributes...)
		mutate(&object)
		if err := h03CheckEntrySanitized(object); err == nil {
			t.Errorf("structural sanitization accepted %q", object.Entry)
		}
	}
	if err := h03CheckEntrySanitized(valid); err != nil {
		t.Fatal(err)
	}
}

// TestH03NegativeOverlapMatrixIsExecutable evaluates every pair that the
// fixtures can speak to with the registry's own gates, and records the
// directions that have no evidence as Unknown rather than as a pass.
func TestH03NegativeOverlapMatrixIsExecutable(t *testing.T) {
	h03RequireControl(t)
	overlap := h03LoadProbes(t, "smartlogger_negative_overlap.json")
	sun2000 := h03LoadProbes(t, "sun2000_routed_identity.json")
	probeAt := func(set h03ProbeFixture, unit int, offset uint16) h03Probe {
		t.Helper()
		for _, probe := range set.Probes {
			for _, candidate := range probe.Units {
				if candidate == unit && probe.Offset == offset {
					return probe
				}
			}
		}
		t.Fatalf("no probe at unit %d offset %d", unit, offset)
		return h03Probe{}
	}
	wordsText := func(probe h03Probe) string {
		var data []byte
		for _, word := range probe.Outcome.Words {
			data = append(data, byte(word>>8), byte(word))
		}
		return string(data)
	}

	// SmartLogger vs EMMA, at the SmartLogger endpoint: the three EMMA reads are exceptions.
	for _, offset := range []uint16{30000, 30222, 30035} {
		if probe := probeAt(overlap, 0, offset); probe.Outcome.Type != "exception" || probe.Outcome.Code != 3 {
			t.Fatalf("EMMA read %d at unit 0: %+v", offset, probe)
		}
	}
	if result := NewHuaweiEMMAQualificationCard().Evaluate(HuaweiEMMAQualificationInput{UnitID: 0}); result.IdentityMatched() || result.Readiness() != HuaweiQualificationInsufficientEvidence {
		t.Fatalf("EMMA matched on exceptions: %#v", result)
	}

	// EMMA vs SUN2000, at a routed unit: the offering offset answers the
	// inverter model string, the model offset answers non-text words.
	offering := probeAt(sun2000, 1, 30000)
	emmaModel := probeAt(overlap, 1, 30222)
	if offering.Outcome.Text != "SUN2000-50KTL-M3" || emmaModel.Outcome.Type != "words" || len(emmaModel.Outcome.Words) != 20 {
		t.Fatalf("routed EMMA probes: %+v %+v", offering, emmaModel)
	}
	decision := EvaluateHuaweiEMMAOfflineIdentity(offering.Outcome.Text, wordsText(emmaModel), probeAt(sun2000, 1, 30035).Outcome.Text)
	if decision.Matched() || decision.Reason() != HuaweiEMMAOfflineModelMismatch {
		t.Fatalf("routed inverter satisfied the EMMA gate: %#v", decision)
	}

	// SmartLogger vs S-Dongle, at logical unit 100: three exceptions, and the
	// basic reply is the gateway's own identity.
	for _, offset := range []uint16{30068, 37410, 37429} {
		if probe := probeAt(overlap, 100, offset); probe.Outcome.Type != "exception" || probe.Outcome.Code != 4 {
			t.Fatalf("S-Dongle read %d at unit 100: %+v", offset, probe)
		}
	}
	// S-Dongle vs SUN2000: the protocol read answers 5 then 0, so it is no discriminator; unit and model are.
	protocol := probeAt(sun2000, 1, 30068).Outcome.Words
	for _, unit := range []uint8{1, 100} {
		decision := EvaluateHuaweiSDongleOfflineObservation(HuaweiSDongleOfflineObservation{
			UnitID: unit, Model: offering.Outcome.Text, Firmware: probeAt(sun2000, 1, 30035).Outcome.Text,
			ProtocolMajor: uint8(protocol[0]), ProtocolMinor: uint8(protocol[1]), SearchState: 0, ChangeSequenceBefore: 1, ChangeSequenceAfter: 1, Capacity: 2, ChildCount: 1,
		})
		wantReason := HuaweiSDongleOfflineUnitMismatch
		if unit == 100 {
			wantReason = HuaweiSDongleOfflineModelMismatch
		}
		if decision.Matched() || decision.Reason() != wantReason {
			t.Fatalf("routed inverter at unit %d satisfied the S-Dongle gate: %#v", unit, decision)
		}
	}
	for _, offset := range []uint16{37410, 37429} {
		if probe := probeAt(overlap, 1, offset); probe.Outcome.Type != "exception" || probe.Outcome.Code != 3 {
			t.Fatalf("S-Dongle read %d at routed unit 1: %+v", offset, probe)
		}
	}

	// SmartLogger vs SUN2000: the counter is not served by the inverter, and the
	// logger card refuses a routed unit.
	if probe := probeAt(overlap, 1, 65521); probe.Outcome.Type != "exception" || probe.Outcome.Code != 3 {
		t.Fatalf("counter read at routed unit 1: %+v", probe)
	}
	input := h03ValidInput(t)
	input.UnitID = 1
	h03RequireRejected(t, "logger tuple on a routed inverter unit", NewHuaweiSmartLoggerObservedQualificationCard().Evaluate(input))

	// Meter and PLC units vs the SUN2000 tuple: the model read is an exception.
	for _, unit := range []int{9, 11} {
		if probe := probeAt(sun2000, unit, 30000); probe.Outcome.Type != "exception" || probe.Outcome.Code != 3 {
			t.Fatalf("model read at listed unit %d: %+v", unit, probe)
		}
	}

	// SunSpec/Fronius: the marker is absent at the gateway unit and at a routed unit.
	decoder := h03SunSpecDecoder(t)
	for _, unit := range []int{0, 1} {
		probe := probeAt(overlap, unit, 40000)
		if len(probe.Outcome.Words) != 2 || (probe.Outcome.Words[0] == sunSpecSignatureFirst && probe.Outcome.Words[1] == sunSpecSignatureSecond) {
			t.Fatalf("SunSpec marker read at unit %d: %+v", unit, probe)
		}
		if _, err := decoder.Parse(probe.Outcome.Words); err == nil {
			t.Fatalf("SunSpec decoder accepted the read at unit %d", unit)
		}
	}

	// Directions with no evidence stay Unknown: the fixtures hold no probe at
	// an EMMA or S-Dongle endpoint, and no Growatt offset was read.
	for _, probe := range append(append([]h03Probe(nil), overlap.Probes...), sun2000.Probes...) {
		for _, unit := range probe.Units {
			if unit > 12 && unit != 100 {
				t.Fatalf("probe at unit %d implies an endpoint that was not observed", unit)
			}
		}
	}
	record := decodeHuaweiObject(t, "profiles/vendor/huawei/smartlogger-disposition.json")
	evidence := huaweiObject(t, record["evidence"])
	partial := h03Array(t, evidence["partially_closed_evidence"])
	if len(partial) != 1 || huaweiObject(t, partial[0])["open"] != "smartlogger_probes_at_emma_endpoint" {
		t.Fatalf("the open reverse direction is not recorded: %#v", partial)
	}
}

func h03SunSpecDecoder(t testing.TB) SunSpecPhaseOneDecoder {
	t.Helper()
	profile, err := NewSunSpecPhaseOneProfile(SunSpecPhaseOneVersions{Profile: CurrentSchemaVersion(), Codec: CurrentCodecContractVersion()})
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewSunSpecPhaseOneDecoder(profile)
	if err != nil {
		t.Fatal(err)
	}
	return decoder
}

func TestH03FixtureCorpusIsExactlyTheDocumentedSet(t *testing.T) {
	entries, err := os.ReadDir(h03FixtureDirectory)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
		if bytes.Contains(h03Read1(t, entry.Name()), []byte("1=SmartLogger;")) {
			t.Fatalf("%s carries the documentary model string as a self entry", entry.Name())
		}
	}
	want := append(append(append([]string(nil), h03FixtureNames()...), h15FixtureNames()...), h18FixtureNames()...)
	sort.Strings(want)
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("identity fixtures=%v want=%v", names, want)
	}
}

// h03CountingDigits and h03Joined build serial-shaped test inputs at run time,
// so that no serial-shaped literal is committed.
func h03CountingDigits(length int) string {
	digits := make([]byte, length)
	for index := range digits {
		digits[index] = byte('1' + index%9)
	}
	return string(digits)
}

func h03Joined(parts ...string) string { return strings.Join(parts, "") }
