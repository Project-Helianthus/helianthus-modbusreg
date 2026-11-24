package huawei

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// Identity fields asserted below, and the production change each assertion
// catches: Card, Key, Address and Occurrence (lookup misses the row);
// Quantity, Type, Gain, Unit, Access and RegisterAddressed (the found row
// no longer has the card's fact); Readable, Writable, ReadSpan, EncodeWrite
// and DecodeRead (the access token admits the other direction, or the word
// image is not the card type).

func TestGridCodeReadWriteRoundTripUsesTheSameRow(t *testing.T) {
	row := mustGridCode(t, "MA-V200", "ma-v200.control.night_reactive_power", 40129, 0)
	if row.Card != "MA-V200" || row.Key != "ma-v200.control.night_reactive_power" || row.Address != 40129 || row.Quantity != 2 || row.Type != "I32" || row.Gain != "1000" || row.Unit != "kVar" || row.Access != GridCodeAccessReadWrite || row.Occurrence != 0 || row.RegisterAddressed || row.CallName() != row.Key {
		t.Fatalf("rw identity changed: %+v", row)
	}
	gain, ok := row.IntegerGain()
	if !ok || gain != 1000 {
		t.Fatalf("gain %d %v", gain, ok)
	}
	if !row.Readable() || !row.Writable() {
		t.Fatal("rw row lost a direction")
	}
	function, address, quantity, err := row.ReadSpan()
	if err != nil || function != uint8(modbusreg.FunctionReadHoldingRegisters) || address != 40129 || quantity != 2 {
		t.Fatalf("read span %d %d %d %v", function, address, quantity, err)
	}
	words, err := row.EncodeWrite(-123456)
	if err != nil || len(words) != 2 || words[0] != 0xFFFE || words[1] != 0x1DC0 {
		t.Fatalf("encode %+v %v", words, err)
	}
	got, err := row.DecodeRead(words)
	if err != nil || got != -123456 {
		t.Fatalf("readback %d %v", got, err)
	}
	if _, err := row.EncodeBlock(words); err == nil {
		t.Fatal("scalar row accepted a block")
	}
}

func TestGridCodeReadOnlyOffersNoWrite(t *testing.T) {
	row := mustGridCode(t, "MA-C20", "identity.version_component.powergridstandardcodeprotocolversion", 30109, 0)
	if row.Card != "MA-C20" || row.Key != "identity.version_component.powergridstandardcodeprotocolversion" || row.Address != 30109 || row.Quantity != 1 || row.Type != "U16" || row.Gain != "not stated" || row.Unit != "not stated" || row.Access != GridCodeAccessReadOnly || row.Occurrence != 0 || row.RegisterAddressed || row.CallName() == "" {
		t.Fatalf("ro identity changed: %+v", row)
	}
	if _, ok := row.IntegerGain(); ok || row.Writable() || !row.Readable() {
		t.Fatal("ro row gained a write or a gain")
	}
	function, address, quantity, err := row.ReadSpan()
	if err != nil || function != uint8(modbusreg.FunctionReadHoldingRegisters) || address != 30109 || quantity != 1 {
		t.Fatalf("read span %d %d %d %v", function, address, quantity, err)
	}
	words, err := row.EncodeWrite(7)
	if err == nil || words != nil {
		t.Fatalf("ro write produced %v %v", words, err)
	}
	if block, err := row.EncodeBlock([]uint16{7}); err == nil || block != nil {
		t.Fatalf("ro block write produced %v", block)
	}
	got, err := row.DecodeRead([]uint16{7})
	if err != nil || got != 7 {
		t.Fatalf("ro read %d %v", got, err)
	}
}

func TestGridCodeWriteOnlyEncodesAndOffersNoRead(t *testing.T) {
	row := mustGridCode(t, "HA", "ha.control.dispatch.40200", 40200, 0)
	if row.Card != "HA" || row.Key != "ha.control.dispatch.40200" || row.Address != 40200 || row.Quantity != 1 || row.Type != "U16" || row.Gain != "1" || row.Unit != "not stated" || row.Access != GridCodeAccessWriteOnly || row.Occurrence != 0 || !row.RegisterAddressed || row.CallName() != "" {
		t.Fatalf("wo identity changed: %+v", row)
	}
	if row.Readable() || !row.Writable() {
		t.Fatal("wo row offered a read or lost the write")
	}
	if _, _, _, err := row.ReadSpan(); err == nil {
		t.Fatal("wo read span was offered")
	}
	if got, err := row.DecodeRead([]uint16{4}); err == nil || got != 0 {
		t.Fatalf("wo read returned %d %v", got, err)
	}
	if block, err := row.DecodeBlock([]uint16{4}); err == nil || block != nil {
		t.Fatalf("wo block read returned %v", block)
	}
	words, err := row.EncodeWrite(4)
	if err != nil || len(words) != 1 || words[0] != 4 {
		t.Fatalf("wo encode %+v %v", words, err)
	}
	rejected, err := row.EncodeWrite(65536)
	if err == nil || rejected != nil {
		t.Fatalf("rejected wo write produced %v", rejected)
	}
}

func TestGridCodeUnnamedRowStaysOnRegisterAndType(t *testing.T) {
	rows := GridCodeSettingsForRegister("L-V200", 42000, "U16")
	if len(rows) != 1 {
		t.Fatalf("register lookup %d", len(rows))
	}
	row := rows[0]
	if row.Card != "L-V200" || row.Key != "l-v200.control.grid_protection.42000" || row.Address != 42000 || row.Quantity != 1 || row.Type != "U16" || row.Gain != "not stated" || row.Unit != "not stated" || row.Access != GridCodeAccessReadWrite || row.Occurrence != 0 || !row.RegisterAddressed || row.CallName() != "" {
		t.Fatalf("unnamed identity changed: %+v", row)
	}
	// SUN-C81 declares no U16 at 42000. MA-V200 does, as its named grid-code row,
	// so that card cannot show that a lookup ignored Card.
	if len(GridCodeSettingsForRegister("L-V200", 42000, "I16")) != 0 || len(GridCodeSettingsForRegister("L-V200", 42001, "U16")) != 0 || len(GridCodeSettingsForRegister("SUN-C81", 42000, "U16")) != 0 {
		t.Fatal("register lookup ignored type, address or card")
	}
	words, err := row.EncodeWrite(42)
	if err != nil || len(words) != 1 || words[0] != 42 {
		t.Fatalf("unnamed encode %+v %v", words, err)
	}
	got, err := row.DecodeRead(words)
	if err != nil || got != 42 {
		t.Fatalf("unnamed readback %d %v", got, err)
	}
}

func TestGridCodeSplitGainStaysRaw(t *testing.T) {
	row := mustGridCode(t, "smartlogger-32-2018-11-15", "gateway-self.40742", 40742, 0)
	if row.Type != "I32" || row.Quantity != 2 || row.Gain != "10/1000" || row.Unit != "kVar" || row.Access != GridCodeAccessReadOnly {
		t.Fatalf("split gain identity changed: %+v", row)
	}
	if _, ok := row.IntegerGain(); ok {
		t.Fatal("split gain became a single gain")
	}
	if _, err := row.EncodeWrite(1000); err == nil {
		t.Fatal("read-only split gain encoded a write")
	}
	got, err := row.DecodeRead([]uint16{0x0000, 0x03E8})
	if err != nil || got != 1000 {
		t.Fatalf("split gain decoded %d %v", got, err)
	}
}

func TestGridCodeUnresolvedGeometrySendsNothing(t *testing.T) {
	row := mustGridCode(t, "HB", "hb.control.dispatch.40129", 40129, 0)
	if row.Type != "I32" || row.Quantity != 1 || row.Access != GridCodeAccessReadWrite || row.Readable() || row.Writable() {
		t.Fatalf("geometry identity changed: %+v readable %v writable %v", row, row.Readable(), row.Writable())
	}
	if _, _, _, err := row.ReadSpan(); err == nil {
		t.Fatal("unresolved geometry offered a read")
	}
	words, err := row.EncodeWrite(1)
	if err == nil || words != nil {
		t.Fatalf("unresolved geometry encoded %v", words)
	}
	if _, err := row.DecodeRead([]uint16{1}); err == nil {
		t.Fatal("unresolved geometry decoded a value")
	}
	wide := mustGridCode(t, "HD", "dispatch.snapshot.adjustmentmode", 35300, 0)
	if wide.Type != "U16" || wide.Quantity != 4 || wide.Access != GridCodeAccessReadOnly || wide.Readable() || wide.Writable() {
		t.Fatalf("wide snapshot changed: %+v", wide)
	}
	if words, err := wide.EncodeWrite(1); err == nil || words != nil {
		t.Fatal("wide snapshot encoded a write")
	}
}

func TestGridCodeBlockKeepsItsSpanAndNoInterior(t *testing.T) {
	row := mustGridCode(t, "MA-V200", "ma-v200.control.cosphi_pn_curve", 40133, 0)
	if row.Type != "MLD" || row.Quantity != 21 || row.Access != GridCodeAccessReadWrite || row.RegisterAddressed || !row.Readable() || !row.Writable() {
		t.Fatalf("block identity changed: %+v", row)
	}
	if words, err := row.EncodeWrite(1); err == nil || words != nil {
		t.Fatal("block accepted a scalar")
	}
	in := make([]uint16, 21)
	in[0], in[20] = 9, 8
	encoded, err := row.EncodeBlock(in)
	if err != nil || len(encoded) != 21 || encoded[0] != 9 || encoded[20] != 8 {
		t.Fatalf("block encode %+v %v", encoded, err)
	}
	encoded[0] = 1
	decoded, err := row.DecodeBlock(in)
	if err != nil || decoded[0] != 9 || decoded[20] != 8 {
		t.Fatalf("block readback %+v %v", decoded, err)
	}
	in[20] = 3
	again, err := row.DecodeBlock(in)
	if err != nil || again[20] != 3 || again[0] != 9 {
		t.Fatalf("block word mutation lost %+v %v", again, err)
	}
	if out, err := row.EncodeBlock(in[:20]); err == nil || out != nil {
		t.Fatal("short block was encoded")
	}
}

func TestGridCodeOccurrencesStayDistinct(t *testing.T) {
	first := mustGridCode(t, "SUN-C81", "sun-c81.control.dispatch.40117", 40117, 1)
	second := mustGridCode(t, "SUN-C81", "sun-c81.control.dispatch.40117", 40117, 2)
	if first.Occurrence != 1 || second.Occurrence != 2 || first.Type != "U16" || second.Type != "U16" || first.Gain != "1" || second.Gain != "1" {
		t.Fatalf("occurrences changed: %+v %+v", first, second)
	}
	left, err := first.EncodeWrite(11)
	if err != nil || len(left) != 1 || left[0] != 11 {
		t.Fatalf("first occurrence %+v %v", left, err)
	}
	right, err := second.EncodeWrite(11)
	if err != nil || len(right) != 1 || right[0] != 11 {
		t.Fatalf("second occurrence %+v %v", right, err)
	}
	found := GridCodeSettingsForRegister("SUN-C81", 40117, "U16")
	if len(found) != 2 || found[0].Occurrence != 1 || found[1].Occurrence != 2 {
		t.Fatalf("occurrence lookup %+v", found)
	}
}

func TestGridCodeIntegerBoundsSendNothingWhenRejected(t *testing.T) {
	signed := mustGridCode(t, "HA", "ha.control.dispatch.40122", 40122, 0)
	if signed.Type != "I16" || signed.Quantity != 1 || signed.Access != GridCodeAccessReadWrite {
		t.Fatalf("i16 identity changed: %+v", signed)
	}
	wide := mustGridCode(t, "L-V200", "l-v200.control.grid_protection.42015", 42015, 0)
	if wide.Type != "U32" || wide.Quantity != 2 || wide.Gain != "1000" || wide.Unit != "%/s" || !wide.RegisterAddressed {
		t.Fatalf("u32 identity changed: %+v", wide)
	}
	for _, sample := range []struct {
		row   GridCodeSetting
		value int64
		ok    bool
	}{
		{signed, -32768, true},
		{signed, 32767, true},
		{signed, -32769, false},
		{signed, 32768, false},
		{wide, 4294967295, true},
		{wide, 4294967296, false},
		{wide, -1, false},
	} {
		words, err := sample.row.EncodeWrite(sample.value)
		if !sample.ok {
			if err == nil || words != nil {
				t.Fatalf("rejected %s %d produced %v", sample.row.Type, sample.value, words)
			}
			continue
		}
		got, err := sample.row.DecodeRead(words)
		if err != nil || got != sample.value || words == nil {
			t.Fatalf("bounds readback %s %d -> %d %v %+v", sample.row.Type, sample.value, got, err, words)
		}
	}
}

func TestGridCodeSettingsMatchTheCatalogue(t *testing.T) {
	settings := GridCodeSettings()
	if len(settings) != 1074 {
		t.Fatalf("row count %d", len(settings))
	}
	catalog := loadGridCodeCatalogue(t)
	seen := map[string]bool{}
	var readOnly, readWrite, writeOnly, unnamed int
	for _, row := range settings {
		id := row.Card + "\x00" + row.Key + "\x00" + strconv.Itoa(int(row.Address)) + "\x00" + strconv.Itoa(int(row.Occurrence))
		if seen[id] {
			t.Fatalf("duplicate %s", id)
		}
		seen[id] = true
		got, ok := catalog[id]
		if !ok || got.quantity != int(row.Quantity) || got.wire != row.Type || got.access != string(row.Access) || got.gain != row.Gain || got.unit != row.Unit {
			t.Fatalf("catalogue mismatch %+v %#v", row, got)
		}
		switch row.Access {
		case GridCodeAccessReadOnly:
			readOnly++
		case GridCodeAccessReadWrite:
			readWrite++
		case GridCodeAccessWriteOnly:
			writeOnly++
		default:
			t.Fatalf("access %q", row.Access)
		}
		if row.RegisterAddressed {
			unnamed++
			if row.CallName() != "" {
				t.Fatalf("unnamed row has a call name %s", row.Key)
			}
		} else if row.CallName() != row.Key {
			t.Fatalf("named call %s", row.Key)
		}
		if row.Card == "L-LEGACY" || strings.HasPrefix(row.Card, "s-dongle-") {
			t.Fatalf("empty card listed a row %s", row.Card)
		}
	}
	if readOnly != 128 || readWrite != 922 || writeOnly != 24 || unnamed != 456 {
		t.Fatalf("counts ro %d rw %d wo %d unnamed %d", readOnly, readWrite, writeOnly, unnamed)
	}
	text := string(mustReadGridCode(t, filepath.Join("..", "docs", "huawei-grid-code-settings.md")))
	if !strings.Contains(text, "no SemReg element") {
		t.Fatal("docs dropped the namespace statement")
	}
	for _, card := range []string{"L-LEGACY", "s-dongle-v100r001c00-05-2022-07-06", "s-dongle-v200r022cc10-02-2023-04-20", "s-dongle-v200r025c00-01-2025-04-27", "s-dongle-v200r025c00spc120-01-2025-12-23"} {
		if !strings.Contains(text, "## "+card) || !strings.Contains(text, card+" defines no grid-code row") {
			t.Fatalf("docs dropped empty card %s", card)
		}
	}
	for _, row := range settings {
		if !strings.Contains(text, row.Key) {
			t.Fatalf("docs omitted %s", row.Key)
		}
	}
}

func mustGridCode(t *testing.T, card, key string, address, occurrence uint16) GridCodeSetting {
	t.Helper()
	for _, row := range GridCodeSettings() {
		if row.Card == card && row.Key == key && row.Address == address && row.Occurrence == occurrence {
			return row
		}
	}
	t.Fatalf("missing %s %s %d occurrence %d", card, key, address, occurrence)
	return GridCodeSetting{}
}

type gridCodeCatalogueFact struct {
	quantity int
	wire     string
	access   string
	gain     string
	unit     string
}

func loadGridCodeCatalogue(t *testing.T) map[string]gridCodeCatalogueFact {
	t.Helper()
	root := filepath.Join("..", "profiles", "vendors", "huawei", "catalog")
	files := gridCodeCatalogueFiles(t, root)
	files = append(files, gridCodeCatalogueFiles(t, filepath.Join(root, "smartlogger-sdongle-emma"))...)
	out := map[string]gridCodeCatalogueFact{}
	for _, name := range files {
		var doc struct {
			Card     string         `json:"card"`
			Rows     []gridCodeJSON `json:"rows"`
			Controls []gridCodeJSON `json:"controls"`
		}
		if err := json.Unmarshal(mustReadGridCode(t, name), &doc); err != nil {
			t.Fatal(err)
		}
		for _, row := range append(doc.Rows, doc.Controls...) {
			if row.Offset == nil {
				continue
			}
			scale := row.Decode
			if scale == nil {
				scale = row.Literal
			}
			gain, unit := "", ""
			if scale != nil {
				gain, unit = scale.Gain, scale.Unit
			}
			id := doc.Card + "\x00" + row.Key + "\x00" + strconv.Itoa(*row.Offset) + "\x00" + strconv.Itoa(row.Occurrence)
			out[id] = gridCodeCatalogueFact{row.Quantity, row.Type, row.Access, gain, unit}
		}
	}
	return out
}

type gridCodeJSON struct {
	Key        string         `json:"key"`
	Offset     *int           `json:"offset"`
	Quantity   int            `json:"quantity"`
	Type       string         `json:"type"`
	Access     string         `json:"access"`
	Occurrence int            `json:"occurrence"`
	Decode     *gridCodeScale `json:"decode"`
	Literal    *gridCodeScale `json:"literal"`
}

type gridCodeScale struct {
	Gain string `json:"gain"`
	Unit string `json:"unit"`
}

func gridCodeCatalogueFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "registers-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		out = append(out, filepath.Join(dir, entry.Name()))
	}
	return out
}

func mustReadGridCode(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
