package modbusreg

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const h03FixtureDirectory = "testdata/huawei/identity"

// The identity fixture schema is Huawei-local. The generic M2-03 corpus carries
// FC03/FC04 register words only, so it cannot represent an FC2B/MEI response
// that is longer than the classic frame limit or whose declared-object field
// differs from the objects present. This schema carries those wire facts and
// the tests rebuild the wire bytes from it.
type h03Hex uint8

func (value h03Hex) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"0x%02x\"", uint8(value))), nil
}

func (value *h03Hex) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	if len(text) != 4 || text[:2] != "0x" || strings.ToLower(text) != text {
		return fmt.Errorf("hex byte %q is not 0xnn", text)
	}
	parsed, err := strconv.ParseUint(text[2:], 16, 8)
	if err != nil {
		return err
	}
	*value = h03Hex(parsed)
	return nil
}

type h03Sanitization struct {
	Rule string `json:"rule"`
}

type h03Header struct {
	Schema       string          `json:"schema"`
	Kind         string          `json:"kind"`
	Family       string          `json:"family"`
	License      string          `json:"license"`
	Provenance   string          `json:"provenance"`
	Sanitization h03Sanitization `json:"sanitization"`
}

type h03Counter struct {
	Before uint16 `json:"before"`
	After  uint16 `json:"after"`
}

type h03Read struct {
	Function         string `json:"function"`
	MEIType          h03Hex `json:"mei_type"`
	ReadDeviceIDCode uint8  `json:"read_device_id_code"`
	StartObjectID    h03Hex `json:"start_object_id"`
}

type h03Object struct {
	ObjectID            h03Hex   `json:"object_id"`
	Count               *uint8   `json:"count,omitempty"`
	Entry               string   `json:"entry,omitempty"`
	PadTo               int      `json:"pad_to,omitempty"`
	SanitizedAttributes []string `json:"sanitized_attributes,omitempty"`
}

type h03InventoryPage struct {
	RequestObjectID h03Hex      `json:"request_object_id"`
	Conformity      h03Hex      `json:"conformity"`
	MoreFollows     h03Hex      `json:"more_follows"`
	NextObjectID    h03Hex      `json:"next_object_id"`
	DeclaredObjects int         `json:"declared_objects"`
	PDUBytes        int         `json:"pdu_bytes"`
	Objects         []h03Object `json:"objects"`
}

type h03InventoryFixture struct {
	h03Header
	GatewayUnit   int                `json:"gateway_unit"`
	ChangeCounter h03Counter         `json:"change_counter"`
	Read          h03Read            `json:"read"`
	Pages         []h03InventoryPage `json:"pages"`
}

type h03BasicObject struct {
	ObjectID    h03Hex `json:"object_id"`
	Text        string `json:"text,omitempty"`
	OpaqueBytes int    `json:"opaque_bytes,omitempty"`
	Sanitized   bool   `json:"sanitized,omitempty"`
}

type h03BasicReply struct {
	Conformity      h03Hex           `json:"conformity"`
	MoreFollows     h03Hex           `json:"more_follows"`
	NextObjectID    h03Hex           `json:"next_object_id"`
	DeclaredObjects int              `json:"declared_objects"`
	PDUBytes        int              `json:"pdu_bytes"`
	Objects         []h03BasicObject `json:"objects"`
}

type h03BasicFixture struct {
	h03Header
	AnsweredForUnits []int         `json:"answered_for_units"`
	Read             h03Read       `json:"read"`
	Reply            h03BasicReply `json:"reply"`
}

type h03Outcome struct {
	Type      string   `json:"type"`
	Text      string   `json:"text,omitempty"`
	Words     []uint16 `json:"words,omitempty"`
	Code      uint8    `json:"code,omitempty"`
	Synthetic bool     `json:"synthetic,omitempty"`
}

type h03Probe struct {
	Units    []int      `json:"units"`
	Function string     `json:"function"`
	Offset   uint16     `json:"offset"`
	Quantity uint16     `json:"quantity"`
	Roles    []string   `json:"roles"`
	Outcome  h03Outcome `json:"outcome"`
}

type h03ProbeFixture struct {
	h03Header
	Probes []h03Probe `json:"probes"`
}

func h03Read1(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h03FixtureDirectory, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// h03Decode is the strict boundary: unknown fields and trailing data fail.
func h03Decode(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("trailing JSON value: %v", err)
	}
	return nil
}

func h03Canonical(value any) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func h03LoadInventory(t testing.TB) h03InventoryFixture {
	t.Helper()
	var fixture h03InventoryFixture
	if err := h03Decode(h03Read1(t, "smartlogger_extended_inventory.json"), &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func h03LoadBasic(t testing.TB) h03BasicFixture {
	t.Helper()
	var fixture h03BasicFixture
	if err := h03Decode(h03Read1(t, "smartlogger_basic_reply.json"), &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func h03LoadProbes(t testing.TB, name string) h03ProbeFixture {
	t.Helper()
	var fixture h03ProbeFixture
	if err := h03Decode(h03Read1(t, name), &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

// ---- wire reconstruction -------------------------------------------------

func h03MEIHeader(read h03Read, conformity, more, next h03Hex, declared int) []byte {
	return []byte{0x2b, uint8(read.MEIType), read.ReadDeviceIDCode, uint8(conformity), uint8(more), uint8(next), uint8(declared)}
}

func h03ObjectValue(object h03Object) ([]byte, error) {
	switch {
	case object.Count != nil && object.Entry == "":
		return []byte{*object.Count}, nil
	case object.Count == nil && object.Entry != "":
		if object.PadTo < len(object.Entry) || object.PadTo > 254 {
			return nil, fmt.Errorf("entry %02x pad_to %d is outside the entry", uint8(object.ObjectID), object.PadTo)
		}
		value := make([]byte, object.PadTo)
		copy(value, object.Entry)
		return value, nil
	}
	return nil, fmt.Errorf("object %02x is neither a count nor an entry", uint8(object.ObjectID))
}

func h03InventoryPDU(read h03Read, page h03InventoryPage) ([]byte, error) {
	pdu := h03MEIHeader(read, page.Conformity, page.MoreFollows, page.NextObjectID, page.DeclaredObjects)
	for _, object := range page.Objects {
		value, err := h03ObjectValue(object)
		if err != nil {
			return nil, err
		}
		pdu = append(pdu, uint8(object.ObjectID), uint8(len(value)))
		pdu = append(pdu, value...)
	}
	if len(pdu) != page.PDUBytes {
		return nil, fmt.Errorf("rebuilt pdu is %d bytes, fixture says %d", len(pdu), page.PDUBytes)
	}
	return pdu, nil
}

func h03BasicObjectValue(object h03BasicObject) ([]byte, error) {
	switch {
	case object.Text != "" && object.OpaqueBytes == 0:
		return []byte(object.Text), nil
	case object.Text == "" && object.OpaqueBytes > 0:
		return make([]byte, object.OpaqueBytes), nil
	}
	return nil, fmt.Errorf("basic object %02x is neither text nor opaque", uint8(object.ObjectID))
}

func h03BasicPDU(read h03Read, reply h03BasicReply) ([]byte, error) {
	pdu := h03MEIHeader(read, reply.Conformity, reply.MoreFollows, reply.NextObjectID, reply.DeclaredObjects)
	for _, object := range reply.Objects {
		value, err := h03BasicObjectValue(object)
		if err != nil {
			return nil, err
		}
		pdu = append(pdu, uint8(object.ObjectID), uint8(len(value)))
		pdu = append(pdu, value...)
	}
	if len(pdu) != reply.PDUBytes {
		return nil, fmt.Errorf("rebuilt basic pdu is %d bytes, fixture says %d", len(pdu), reply.PDUBytes)
	}
	return pdu, nil
}

// h03ParsedMEI is produced by a parser that shares no code with the registry.
// It enforces only the frame arithmetic, never the classic 253-byte limit and
// never an equality between the declared-object field and the objects present.
type h03ParsedMEI struct {
	readDeviceIDCode uint8
	conformity       uint8
	more             uint8
	next             uint8
	declared         uint8
	objects          []HuaweiSmartLoggerInventoryObject
	length           int
}

func h03ParseMEI(pdu []byte) (h03ParsedMEI, error) {
	if len(pdu) < 7 || pdu[0] != 0x2b || pdu[1] != 0x0e {
		return h03ParsedMEI{}, fmt.Errorf("not an FC2B/MEI 0x0e response")
	}
	parsed := h03ParsedMEI{readDeviceIDCode: pdu[2], conformity: pdu[3], more: pdu[4], next: pdu[5], declared: pdu[6], length: len(pdu)}
	for offset := 7; offset < len(pdu); {
		if offset+2 > len(pdu) || offset+2+int(pdu[offset+1]) > len(pdu) {
			return h03ParsedMEI{}, fmt.Errorf("object overruns the response")
		}
		length := int(pdu[offset+1])
		parsed.objects = append(parsed.objects, HuaweiSmartLoggerInventoryObject{
			ObjectID: pdu[offset],
			Value:    append([]byte(nil), pdu[offset+2:offset+2+length]...),
		})
		offset += 2 + length
	}
	return parsed, nil
}

func h03ProbePDU(probe h03Probe) ([]byte, error) {
	switch probe.Outcome.Type {
	case "exception":
		return []byte{0x83, probe.Outcome.Code}, nil
	case "text":
		extent := int(probe.Quantity) * 2
		if len(probe.Outcome.Text) >= extent {
			return nil, fmt.Errorf("text leaves no padding in its %d byte extent", extent)
		}
		data := make([]byte, extent)
		copy(data, probe.Outcome.Text)
		return append([]byte{0x03, byte(extent)}, data...), nil
	case "words":
		if len(probe.Outcome.Words) != int(probe.Quantity) {
			return nil, fmt.Errorf("%d words for quantity %d", len(probe.Outcome.Words), probe.Quantity)
		}
		pdu := []byte{0x03, byte(len(probe.Outcome.Words) * 2)}
		for _, word := range probe.Outcome.Words {
			pdu = binary.BigEndian.AppendUint16(pdu, word)
		}
		return pdu, nil
	}
	return nil, fmt.Errorf("unknown probe outcome %q", probe.Outcome.Type)
}

type h03ParsedFC03 struct {
	exception uint8
	data      []byte
}

func h03ParseFC03(pdu []byte, quantity uint16) (h03ParsedFC03, error) {
	switch {
	case len(pdu) == 2 && pdu[0] == 0x83:
		return h03ParsedFC03{exception: pdu[1]}, nil
	case len(pdu) >= 2 && pdu[0] == 0x03 && int(pdu[1]) == len(pdu)-2 && int(pdu[1]) == int(quantity)*2:
		return h03ParsedFC03{data: append([]byte(nil), pdu[2:]...)}, nil
	}
	return h03ParsedFC03{}, fmt.Errorf("not a bounded FC03 response")
}

func h03TrimNUL(data []byte) string { return strings.TrimRight(string(data), "\x00") }

// h03InventoryPDUs rebuilds every response of the paged inventory.
func h03InventoryPDUs(fixture h03InventoryFixture) ([][]byte, error) {
	pdus := make([][]byte, 0, len(fixture.Pages))
	for _, page := range fixture.Pages {
		pdu, err := h03InventoryPDU(fixture.Read, page)
		if err != nil {
			return nil, err
		}
		pdus = append(pdus, pdu)
	}
	return pdus, nil
}

// h03ObservedInput turns rebuilt wire responses into the card's decoded input.
// Frame length and the declared-object field come from the wire, not from the
// fixture's own claims.
func h03ObservedInput(t testing.TB, unit uint8, counter h03Counter, pdus [][]byte) HuaweiSmartLoggerObservedQualificationInput {
	t.Helper()
	input := HuaweiSmartLoggerObservedQualificationInput{
		UnitID:    unit,
		Inventory: HuaweiSmartLoggerObservedInventoryInput{ChangeCounterBefore: counter.Before, ChangeCounterAfter: counter.After},
	}
	for _, pdu := range pdus {
		parsed, err := h03ParseMEI(pdu)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.conformity != 0x03 || parsed.readDeviceIDCode != 3 || (parsed.more != 0x00 && parsed.more != 0xff) {
			t.Fatalf("unexpected extended response header: %+v", parsed)
		}
		input.Inventory.Pages = append(input.Inventory.Pages, HuaweiSmartLoggerObservedInventoryPage{
			ResponseBytes:   parsed.length,
			DeclaredObjects: int(parsed.declared),
			More:            parsed.more == 0xff,
			NextObjectID:    parsed.next,
			Objects:         parsed.objects,
		})
	}
	return input
}

func h03ObservedInputFromFixture(t testing.TB, fixture h03InventoryFixture) HuaweiSmartLoggerObservedQualificationInput {
	t.Helper()
	pdus, err := h03InventoryPDUs(fixture)
	if err != nil {
		t.Fatal(err)
	}
	return h03ObservedInput(t, uint8(fixture.GatewayUnit), fixture.ChangeCounter, pdus)
}

func h03BasicReplyFromFixture(t testing.TB, fixture h03BasicFixture) *HuaweiSmartLoggerObservedBasicReply {
	t.Helper()
	pdu, err := h03BasicPDU(fixture.Read, fixture.Reply)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := h03ParseMEI(pdu)
	if err != nil {
		t.Fatal(err)
	}
	return &HuaweiSmartLoggerObservedBasicReply{Objects: parsed.objects}
}

// h03Digest is computed over length-prefixed responses; no digest is stored.
func h03Digest(pdus [][]byte) [sha256.Size]byte {
	hash := sha256.New()
	for _, pdu := range pdus {
		var prefix [4]byte
		binary.BigEndian.PutUint32(prefix[:], uint32(len(pdu)))
		hash.Write(prefix[:])
		hash.Write(pdu)
	}
	var sum [sha256.Size]byte
	copy(sum[:], hash.Sum(nil))
	return sum
}

// ---- structural sanitization ---------------------------------------------

var (
	h03FillText      = regexp.MustCompile(`^[0Xx._\-]+$`)
	h03FirmwareShape = regexp.MustCompile(`^V[0-9]{3}R[0-9]{3}(?:C[0-9]{2}SPC[0-9]{3})?$`)
	h03Decimal       = regexp.MustCompile(`^(0|[1-9][0-9]{0,2})$`)
)

// h03EntryAttributes splits an entry into ordered key/value pairs.
func h03EntryAttributes(entry string) [][2]string {
	var attributes [][2]string
	for _, attribute := range strings.Split(entry, ";") {
		key, value, _ := strings.Cut(attribute, "=")
		attributes = append(attributes, [2]string{key, value})
	}
	return attributes
}

// h03CheckEntrySanitized enforces an allow-list per attribute: only the
// published labels, unit identifiers and firmware strings are literal; every
// other value must be declared sanitized and consist of placeholder characters.
func h03CheckEntrySanitized(object h03Object) error {
	declared := map[string]bool{}
	for _, key := range object.SanitizedAttributes {
		declared[key] = true
	}
	seen := map[string]bool{}
	for _, pair := range h03EntryAttributes(object.Entry) {
		key, value := pair[0], pair[1]
		seen[key] = true
		switch key {
		case "1":
			switch value {
			case "Smart Logger", "SUN2000-50KTL-M3", "PLC", "PowerMeter":
			default:
				return fmt.Errorf("attribute 1 %q is not a published label", value)
			}
		case "2":
			if !declared[key] && value != "V300R023C10SPC620" && value != "V200R023C00SPC128" {
				return fmt.Errorf("attribute 2 is neither a published firmware nor sanitized")
			}
		case "5":
			if !h03Decimal.MatchString(value) || declared[key] {
				return fmt.Errorf("attribute 5 %q is not a plain unit", value)
			}
		case "8":
			if value != "SUN2000" || declared[key] {
				return fmt.Errorf("attribute 8 %q is not the published product type", value)
			}
		case "9":
			if value != "0" || declared[key] {
				return fmt.Errorf("attribute 9 %q is not the published constant", value)
			}
		case "3", "4", "6", "10":
			if !declared[key] {
				return fmt.Errorf("attribute %s is not declared sanitized", key)
			}
		default:
			return fmt.Errorf("attribute %q is not part of the observed key set", key)
		}
		if declared[key] && !h03FillText.MatchString(value) {
			return fmt.Errorf("attribute %s declared sanitized but %q is not a placeholder", key, value)
		}
	}
	for key := range declared {
		if !seen[key] {
			return fmt.Errorf("sanitized attribute %s is absent from the entry", key)
		}
	}
	return nil
}

// ---- shape scan ----------------------------------------------------------

var (
	h03IPv4Shape      = regexp.MustCompile(`(?:^|[^0-9])(?:[0-9]{1,3}\.){3}[0-9]{1,3}(?:[^0-9]|$)`)
	h03IPv6Shape      = regexp.MustCompile(`(?i)(?:[0-9a-f]{1,4}:){2,7}[0-9a-f]{1,4}|(?:[0-9a-f]{1,4}:){1,7}:(?:[0-9a-f]{1,4})?|::[0-9a-f]{1,4}`)
	h03MACShape       = regexp.MustCompile(`(?i)(?:[0-9a-f]{2}[:-]){5}[0-9a-f]{2}`)
	h03URLShape       = regexp.MustCompile(`(?i)[a-z][a-z0-9+.\-]*://|[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	h03HostShape      = regexp.MustCompile(`(?i)\b[a-z0-9\-]+(?:\.[a-z0-9\-]+)*\.(?:local|lan|home|internal|localdomain|com|net|org|io|dev|eu|de)\b`)
	h03HostPortShape  = regexp.MustCompile(`[A-Za-z0-9.\-]:[0-9]{2,5}(?:[^0-9]|$)`)
	h03TimestampShape = regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}|[0-9]{2}:[0-9]{2}:[0-9]{2}`)
	h03AlnumRun       = regexp.MustCompile(`[A-Za-z0-9]{8,}`)
	h03FillRun        = regexp.MustCompile(`^[0Xx]+$`)
)

// h03ShapeFindings reports serial-shaped, ESN-shaped, address-shaped and
// time-shaped values. A run of eight or more alphanumerics that contains a
// digit is a finding unless it is a uniform fill of the sanitization characters or a firmware shape.
func h03ShapeFindings(text string) []string {
	var findings []string
	add := func(kind, value string) { findings = append(findings, kind+":"+value) }
	for _, run := range h03AlnumRun.FindAllString(text, -1) {
		if h03FillRun.MatchString(run) || h03FirmwareShape.MatchString(run) {
			continue
		}
		if strings.ContainsAny(run, "0123456789") {
			add("serial_or_esn_shape", run)
		}
	}
	for _, finding := range []struct {
		kind  string
		shape *regexp.Regexp
	}{
		{"ipv4", h03IPv4Shape}, {"ipv6", h03IPv6Shape}, {"mac", h03MACShape}, {"url_or_account", h03URLShape},
		{"host", h03HostShape}, {"host_port", h03HostPortShape}, {"timestamp", h03TimestampShape},
	} {
		for _, match := range finding.shape.FindAllString(text, -1) {
			add(finding.kind, strings.TrimSpace(match))
		}
	}
	sort.Strings(findings)
	return findings
}

func h03CommittedHuaweiTestdata(t testing.TB) []string {
	t.Helper()
	var paths []string
	err := filepath.Walk(filepath.Join("testdata", "huawei"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	return paths
}

func h03FixtureNames() []string {
	return []string{
		"smartlogger_basic_reply.json",
		"smartlogger_extended_inventory.json",
		"smartlogger_negative_overlap.json",
		"sun2000_routed_identity.json",
	}
}

func h03CheckHeader(header h03Header, kind, family string) error {
	if header.Schema != "helianthus-huawei-identity-fixture/v1" || header.License != "CC0-1.0" ||
		header.Kind != kind || header.Family != family || header.Provenance == "" || header.Sanitization.Rule == "" {
		return fmt.Errorf("unexpected fixture header %+v", header)
	}
	return nil
}
