package modbusreg

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// The S-Dongle fixtures reuse the Huawei-local identity schema of the
// SmartLogger slice. The schema gains three optional outcome fields
// (residue, sanitized, attempts) and two outcome types (text_with_residue and
// no_response), and three fixture kinds that the earlier slice had no use for:
// mei_exception, first_request_timing and a one-unit mei_extended_inventory.
// Every fixture of the earlier slice still decodes unchanged.

const (
	h15Model       = "SUN2000-8KTL-M1"
	h15ModelExtent = 30
)

type h15ExceptionReply struct {
	ExceptionCode uint8 `json:"exception_code"`
	PDUBytes      int   `json:"pdu_bytes"`
}

type h15ExceptionFixture struct {
	h03Header
	Unit  int               `json:"unit"`
	Read  h03Read           `json:"read"`
	Reply h15ExceptionReply `json:"reply"`
}

type h15TimingAttempt struct {
	Outcome      string `json:"outcome"`
	ElapsedClass string `json:"elapsed_class"`
}

type h15TimingObservation struct {
	Run      int                `json:"run"`
	Unit     int                `json:"unit"`
	Function string             `json:"function"`
	Attempts []h15TimingAttempt `json:"attempts"`
}

type h15TimingFixture struct {
	h03Header
	Scope             string                 `json:"scope"`
	RequiredByCard    bool                   `json:"required_by_card"`
	AttemptDeadlineMS int                    `json:"attempt_deadline_ms"`
	PromptBoundMS     int                    `json:"prompt_bound_ms"`
	Observations      []h15TimingObservation `json:"observations"`
}

type h15InventoryFixture struct {
	h03Header
	Unit  int                `json:"unit"`
	Read  h03Read            `json:"read"`
	Pages []h03InventoryPage `json:"pages"`
}

// h15Fixtures holds every S-Dongle fixture so that a test can change one field
// and rebuild the card input through the wire representation.
type h15Fixtures struct {
	Gateway        h03ProbeFixture
	GatewayBasic   h15ExceptionFixture
	Timing         h15TimingFixture
	ChildBlocks    h03ProbeFixture
	ChildBasic     h03BasicFixture
	ChildInventory h15InventoryFixture
	Overlap        h03ProbeFixture
}

func h15FixtureNames() []string {
	return []string{
		"sdongle_child_basic_reply.json",
		"sdongle_child_extended_inventory.json",
		"sdongle_child_identity_blocks.json",
		"sdongle_first_request_timing.json",
		"sdongle_gateway_basic_exception.json",
		"sdongle_gateway_fc03_replies.json",
		"sdongle_negative_overlap.json",
	}
}

func h15Target(name string) any {
	switch name {
	case "sdongle_child_basic_reply.json":
		return &h03BasicFixture{}
	case "sdongle_child_extended_inventory.json":
		return &h15InventoryFixture{}
	case "sdongle_first_request_timing.json":
		return &h15TimingFixture{}
	case "sdongle_gateway_basic_exception.json":
		return &h15ExceptionFixture{}
	}
	return &h03ProbeFixture{}
}

func h15LoadAll(t testing.TB) h15Fixtures {
	t.Helper()
	var f h15Fixtures
	for name, target := range map[string]any{
		"sdongle_gateway_fc03_replies.json":     &f.Gateway,
		"sdongle_gateway_basic_exception.json":  &f.GatewayBasic,
		"sdongle_first_request_timing.json":     &f.Timing,
		"sdongle_child_identity_blocks.json":    &f.ChildBlocks,
		"sdongle_child_basic_reply.json":        &f.ChildBasic,
		"sdongle_child_extended_inventory.json": &f.ChildInventory,
		"sdongle_negative_overlap.json":         &f.Overlap,
	} {
		if err := h03Decode(h03Read1(t, name), target); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	return f
}

// ---- wire reconstruction -------------------------------------------------

// h15ProbePDU rebuilds the response of one probe. A no_response outcome has no
// response, so it returns a nil PDU.
func h15ProbePDU(probe h03Probe) ([]byte, error) {
	switch probe.Outcome.Type {
	case "text_with_residue":
		extent := int(probe.Quantity) * 2
		if len(probe.Outcome.Text)+1+len(probe.Outcome.Residue) >= extent {
			return nil, fmt.Errorf("text, NUL and residue leave no padding in the %d byte extent", extent)
		}
		data := make([]byte, extent)
		copy(data, probe.Outcome.Text)
		copy(data[len(probe.Outcome.Text)+1:], probe.Outcome.Residue)
		return append([]byte{0x03, byte(extent)}, data...), nil
	case "no_response":
		return nil, nil
	}
	return h03ProbePDU(probe)
}

// h15Data returns the register bytes of an answered probe and nil for an
// exception or for no response.
func h15Data(t testing.TB, probe h03Probe) []byte {
	t.Helper()
	pdu, err := h15ProbePDU(probe)
	if err != nil {
		t.Fatal(err)
	}
	if pdu == nil {
		return nil
	}
	parsed, err := h03ParseFC03(pdu, probe.Quantity)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.data
}

func h15Words(t testing.TB, probe h03Probe) []uint16 {
	t.Helper()
	data := h15Data(t, probe)
	var words []uint16
	for index := 0; index+1 < len(data); index += 2 {
		words = append(words, uint16(data[index])<<8|uint16(data[index+1]))
	}
	return words
}

func h15ProbeByRole(t testing.TB, set h03ProbeFixture, role string) h03Probe {
	t.Helper()
	var found []h03Probe
	for _, probe := range set.Probes {
		for _, candidate := range probe.Roles {
			if candidate == role {
				found = append(found, probe)
			}
		}
	}
	if len(found) != 1 {
		t.Fatalf("role %q matches %d probes", role, len(found))
	}
	return found[0]
}

func h15ProbeAt(t testing.TB, set h03ProbeFixture, unit int, offset uint16) h03Probe {
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

// h15MEIExceptionPDU is the exception response to FC2B: the function code with
// its high bit set, then the code.
func h15MEIExceptionPDU(reply h15ExceptionReply) ([]byte, error) {
	pdu := []byte{0x80 | 0x2b, reply.ExceptionCode}
	if len(pdu) != reply.PDUBytes {
		return nil, fmt.Errorf("rebuilt exception pdu is %d bytes, fixture says %d", len(pdu), reply.PDUBytes)
	}
	return pdu, nil
}

// h15ParseMEIException shares no code with the registry.
func h15ParseMEIException(pdu []byte) (uint8, error) {
	if len(pdu) != 2 || pdu[0] != 0xab {
		return 0, fmt.Errorf("not an FC2B exception response")
	}
	return pdu[1], nil
}

func h15BasicReply(t testing.TB, fixture h03BasicFixture) *HuaweiSDongleObservedBasicReply {
	t.Helper()
	pdu, err := h03BasicPDU(fixture.Read, fixture.Reply)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := h03ParseMEI(pdu)
	if err != nil {
		t.Fatal(err)
	}
	return &HuaweiSDongleObservedBasicReply{Objects: parsed.objects}
}

// h15ChildPage rebuilds the one inventory response and takes its frame length,
// declared-object field, continuation and next identifier from the wire.
func h15ChildPage(t testing.TB, fixture h15InventoryFixture) HuaweiSDongleObservedChildInventoryPage {
	t.Helper()
	if len(fixture.Pages) != 1 {
		t.Fatalf("child inventory fixture has %d pages", len(fixture.Pages))
	}
	pdu, err := h03InventoryPDU(fixture.Read, fixture.Pages[0])
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := h03ParseMEI(pdu)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.more != 0x00 && parsed.more != 0xff {
		t.Fatalf("unexpected continuation byte %#x", parsed.more)
	}
	return HuaweiSDongleObservedChildInventoryPage{
		ReadDeviceIDCode: parsed.readDeviceIDCode,
		Conformity:       parsed.conformity,
		ResponseBytes:    parsed.length,
		DeclaredObjects:  int(parsed.declared),
		More:             parsed.more == 0xff,
		NextObjectID:     parsed.next,
		Objects:          parsed.objects,
	}
}

// h15Input rebuilds every wire response from the fixtures and decodes them into
// the card input. Nothing in the input comes from a fixture claim about itself.
func h15Input(t testing.TB, f h15Fixtures) HuaweiSDongleObservedQualificationInput {
	t.Helper()
	exceptionPDU, err := h15MEIExceptionPDU(f.GatewayBasic.Reply)
	if err != nil {
		t.Fatal(err)
	}
	code, err := h15ParseMEIException(exceptionPDU)
	if err != nil {
		t.Fatal(err)
	}
	model := h15ProbeAt(t, f.ChildBlocks, 1, 30000)
	return HuaweiSDongleObservedQualificationInput{
		Gateway: HuaweiSDongleObservedGatewayReads{
			UnitID:       uint8(f.GatewayBasic.Unit),
			Basic:        HuaweiSDongleObservedBasicOutcome{ExceptionCode: code},
			SearchStatus: h15Words(t, h15ProbeByRole(t, f.Gateway, "sdongle_search_status")),
			Protocol:     h15Words(t, h15ProbeByRole(t, f.Gateway, "sdongle_protocol")),
			SearchBefore: h15Words(t, h15ProbeByRole(t, f.Gateway, "sdongle_search_before")),
			SearchAfter:  h15Words(t, h15ProbeByRole(t, f.Gateway, "sdongle_search_after")),
			Capacity:     h15Words(t, h15ProbeByRole(t, f.Gateway, "sdongle_capacity")),
		},
		Child: HuaweiSDongleObservedChildReads{
			UnitID:     uint8(model.Units[0]),
			ModelBlock: h15Data(t, model),
			Basic:      h15BasicReply(t, f.ChildBasic),
			Inventory:  h15ChildPage(t, f.ChildInventory),
		},
	}
}

func h15ValidInput(t testing.TB) HuaweiSDongleObservedQualificationInput {
	t.Helper()
	return h15Input(t, h15LoadAll(t))
}

// h15SetEntry rewrites the one child entry, unpadded, and keeps the fixture's
// own frame length consistent with it.
func h15SetEntry(entry string) func(*h15Fixtures) {
	return func(f *h15Fixtures) {
		object := &f.ChildInventory.Pages[0].Objects[1]
		object.Entry = entry
		object.PadTo = len(entry)
		f.ChildInventory.Pages[0].PDUBytes = 7 + 3 + 2 + len(entry)
	}
}

// h15ReplaceInEntry rewrites part of the entry and keeps the entry unpadded.
func h15ReplaceInEntry(old, replacement string) func(*h15Fixtures) {
	return func(f *h15Fixtures) {
		entry := f.ChildInventory.Pages[0].Objects[1].Entry
		if !strings.Contains(entry, old) {
			panic("entry does not contain " + old)
		}
		h15SetEntry(strings.Replace(entry, old, replacement, 1))(f)
	}
}

// h15SetModelBlock replaces the model probe with a text and a residue.
func h15SetModelBlock(text, residue string) func(*h15Fixtures) {
	return func(f *h15Fixtures) {
		for index := range f.ChildBlocks.Probes {
			probe := &f.ChildBlocks.Probes[index]
			if probe.Offset == 30000 && probe.Units[0] == 1 {
				probe.Outcome = h03Outcome{Type: "text_with_residue", Text: text, Residue: residue, Sanitized: true}
				if residue == "" {
					probe.Outcome = h03Outcome{Type: "text", Text: text}
				}
			}
		}
	}
}

// h15ChildPDUs rebuilds every response of the S-Dongle fixtures; the digest of
// the whole set is computed by the tests and never stored.
func h15ChildPDUs(t testing.TB, f h15Fixtures) [][]byte {
	t.Helper()
	inventory, err := h03InventoryPDU(f.ChildInventory.Read, f.ChildInventory.Pages[0])
	if err != nil {
		t.Fatal(err)
	}
	basic, err := h03BasicPDU(f.ChildBasic.Read, f.ChildBasic.Reply)
	if err != nil {
		t.Fatal(err)
	}
	exception, err := h15MEIExceptionPDU(f.GatewayBasic.Reply)
	if err != nil {
		t.Fatal(err)
	}
	pdus := [][]byte{inventory, basic, exception}
	for _, set := range []h03ProbeFixture{f.Gateway, f.ChildBlocks, f.Overlap} {
		for _, probe := range set.Probes {
			pdu, err := h15ProbePDU(probe)
			if err != nil {
				t.Fatal(err)
			}
			if pdu != nil {
				pdus = append(pdus, pdu)
			}
		}
	}
	return pdus
}

// ---- structural sanitization ---------------------------------------------

// h15CheckEntrySanitized enforces an allow-list per attribute. Only the child
// model and the unit are literal; every other value must be declared sanitized
// and consist of placeholder characters. The key set is exactly 1 through 6.
func h15CheckEntrySanitized(object h03Object) error {
	declared := map[string]bool{}
	for _, key := range object.SanitizedAttributes {
		declared[key] = true
	}
	seen := map[string]bool{}
	for _, pair := range h03EntryAttributes(object.Entry) {
		key, value := pair[0], pair[1]
		if seen[key] {
			return fmt.Errorf("attribute %s is repeated", key)
		}
		seen[key] = true
		switch key {
		case "1":
			if value != h15Model || declared[key] {
				return fmt.Errorf("attribute 1 %q is not the published model", value)
			}
		case "5":
			if !h03Decimal.MatchString(value) || declared[key] {
				return fmt.Errorf("attribute 5 %q is not a plain unit", value)
			}
		case "2", "3", "4", "6":
			if !declared[key] || !h03FillText.MatchString(value) {
				return fmt.Errorf("attribute %s is not a declared placeholder: %q", key, value)
			}
		default:
			return fmt.Errorf("attribute %q is not part of the observed key set", key)
		}
	}
	for _, key := range []string{"1", "2", "3", "4", "5", "6"} {
		if !seen[key] {
			return fmt.Errorf("attribute %s is absent", key)
		}
	}
	for key := range declared {
		if !seen[key] {
			return fmt.Errorf("sanitized attribute %s is absent from the entry", key)
		}
	}
	return nil
}

// h15CheckProbeSanitized allows a literal only for the published child model
// and a placeholder everywhere else, and allows data words only when they are
// small published values or marked synthetic.
func h15CheckProbeSanitized(probe h03Probe) error {
	outcome := probe.Outcome
	switch outcome.Type {
	case "text":
		if !outcome.Sanitized || !h03FillText.MatchString(outcome.Text) {
			return fmt.Errorf("text %q is neither marked sanitized nor a placeholder", outcome.Text)
		}
	case "text_with_residue":
		if outcome.Text != h15Model || !outcome.Sanitized || !h03FillText.MatchString(outcome.Residue) {
			return fmt.Errorf("residue %q is not a placeholder after the published model", outcome.Residue)
		}
	case "words":
		published := reflect.DeepEqual(outcome.Words, []uint16{5, 0}) || reflect.DeepEqual(outcome.Words, []uint16{50, 0}) ||
			reflect.DeepEqual(outcome.Words, []uint16{0}) || reflect.DeepEqual(outcome.Words, []uint16{10}) ||
			reflect.DeepEqual(outcome.Words, []uint16{4, 0, 0})
		if !published && !outcome.Synthetic {
			return fmt.Errorf("data words %v are neither published nor marked synthetic", outcome.Words)
		}
	case "exception":
		if outcome.Code == 0 || outcome.Text != "" || outcome.Residue != "" || outcome.Attempts != 0 {
			return fmt.Errorf("malformed exception outcome %+v", outcome)
		}
	case "no_response":
		if outcome.Attempts < 1 || outcome.Text != "" || outcome.Words != nil || outcome.Residue != "" {
			return fmt.Errorf("malformed no_response outcome %+v", outcome)
		}
	default:
		return fmt.Errorf("unknown outcome %q", outcome.Type)
	}
	return nil
}

// ---- fixture tests -------------------------------------------------------

func TestH15FixturesDecodeStrictlyRoundTripAndRebuildWire(t *testing.T) {
	for _, name := range h15FixtureNames() {
		t.Run(name, func(t *testing.T) {
			data := h03Read1(t, name)
			target := h15Target(name)
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
			unknown := append([]byte(`{"unexpected":1,`), data[1:]...)
			if err := h03Decode(unknown, reflect.New(reflect.TypeOf(target).Elem()).Interface()); err == nil {
				t.Fatal("unknown field was accepted")
			}
			if err := h03Decode(append(append([]byte(nil), data...), []byte("{}")...), reflect.New(reflect.TypeOf(target).Elem()).Interface()); err == nil {
				t.Fatal("trailing JSON value was accepted")
			}
			if bytes.Contains(data, []byte(`"0x`)) {
				if err := h03Decode(bytes.Replace(data, []byte(`"0x`), []byte(`"0X`), 1), reflect.New(reflect.TypeOf(target).Elem()).Interface()); err == nil {
					t.Fatal("upper-case hex byte was accepted")
				}
			}
		})
	}
}

func TestH15FixtureHeadersAreSyntheticCC0AndFamilyScoped(t *testing.T) {
	f := h15LoadAll(t)
	for name, header := range map[string]struct {
		header h03Header
		kind   string
		family string
	}{
		"gateway fc03":    {f.Gateway.h03Header, "fc03_probe_set", "S-Dongle"},
		"gateway basic":   {f.GatewayBasic.h03Header, "mei_exception", "S-Dongle"},
		"timing":          {f.Timing.h03Header, "first_request_timing", "S-Dongle"},
		"child blocks":    {f.ChildBlocks.h03Header, "fc03_probe_set", "SUN2000"},
		"child basic":     {f.ChildBasic.h03Header, "mei_basic_reply", "SUN2000"},
		"child inventory": {f.ChildInventory.h03Header, "mei_extended_inventory", "SUN2000"},
		"overlap":         {f.Overlap.h03Header, "fc03_probe_set", "S-Dongle"},
	} {
		if err := h03CheckHeader(header.header, header.kind, header.family); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestH15GatewayFixturesCarryTheRefusalAndTheAnsweredStatusReads(t *testing.T) {
	f := h15LoadAll(t)
	if f.GatewayBasic.Unit != 100 || f.GatewayBasic.Read.Function != "FC2B" || f.GatewayBasic.Read.MEIType != 0x0e ||
		f.GatewayBasic.Read.ReadDeviceIDCode != 1 || f.GatewayBasic.Read.StartObjectID != 0x00 {
		t.Fatalf("gateway basic read: %+v", f.GatewayBasic)
	}
	pdu, err := h15MEIExceptionPDU(f.GatewayBasic.Reply)
	if err != nil {
		t.Fatal(err)
	}
	if code, err := h15ParseMEIException(pdu); err != nil || code != 1 {
		t.Fatalf("exception code=%d err=%v; want illegal function 1", code, err)
	}

	want := map[string][]uint16{
		"sdongle_search_status": {0}, "sdongle_protocol": {50, 0}, "sdongle_search_before": {4, 0, 7},
		"sdongle_capacity": {10}, "sdongle_search_after": {4, 0, 7},
	}
	offsets := map[string]uint16{
		"sdongle_search_status": 37411, "sdongle_protocol": 30068, "sdongle_search_before": 37410,
		"sdongle_capacity": 37429, "sdongle_search_after": 37410,
	}
	if len(f.Gateway.Probes) != len(want) {
		t.Fatalf("gateway probes=%d", len(f.Gateway.Probes))
	}
	for role, words := range want {
		probe := h15ProbeByRole(t, f.Gateway, role)
		if !reflect.DeepEqual(probe.Units, []int{100}) || probe.Offset != offsets[role] || int(probe.Quantity) != len(words) ||
			!reflect.DeepEqual(h15Words(t, probe), words) {
			t.Fatalf("gateway probe %s: %+v", role, probe)
		}
	}
	// The sequence word is a stand-in, so the fixture marks the whole read synthetic;
	// the read before and after the child steps is identical.
	before := h15ProbeByRole(t, f.Gateway, "sdongle_search_before")
	after := h15ProbeByRole(t, f.Gateway, "sdongle_search_after")
	if !before.Outcome.Synthetic || !reflect.DeepEqual(before.Outcome, after.Outcome) {
		t.Fatalf("search sequence before/after: %+v %+v", before.Outcome, after.Outcome)
	}

	// Extended inventory is never a gateway-unit read.
	if f.ChildInventory.Unit == 100 || f.ChildInventory.Unit == 0 {
		t.Fatalf("extended inventory fixture is addressed to unit %d", f.ChildInventory.Unit)
	}
}

func TestH15ChildIdentityFixtureCarriesResidueAndNeighbourOutcomes(t *testing.T) {
	f := h15LoadAll(t)
	model := h15ProbeAt(t, f.ChildBlocks, 1, 30000)
	data := h15Data(t, model)
	if len(data) != h15ModelExtent || string(data[:15]) != h15Model || data[15] != 0 || len(model.Outcome.Residue) != 12 ||
		string(data[16:28]) != model.Outcome.Residue || !bytes.Equal(data[28:], []byte{0, 0}) {
		t.Fatalf("model block structure: %q", data)
	}
	if !reflect.DeepEqual(model.Units, []int{1}) || model.Offset != 30000 || model.Quantity != 15 {
		t.Fatalf("model probe: %+v", model)
	}

	serial := h15ProbeByRole(t, f.ChildBlocks, "serial_bearing_block")
	if serialData := h15Data(t, serial); len(serialData) != 20 || len(serial.Outcome.Text) != 12 || bytes.Count(serialData, []byte{0}) != 8 {
		t.Fatalf("serial-bearing block structure: %+v", serial)
	}
	firmware := h15ProbeByRole(t, f.ChildBlocks, "identity_firmware")
	if firmwareData := h15Data(t, firmware); len(firmwareData) != 30 || len(firmware.Outcome.Text) != 11 || bytes.Count(firmwareData, []byte{0}) != 19 {
		t.Fatalf("firmware block structure: %+v", firmware)
	}
	protocol := h15ProbeByRole(t, f.ChildBlocks, "identity_protocol")
	if !reflect.DeepEqual(h15Words(t, protocol), []uint16{5, 0}) {
		t.Fatalf("child protocol words: %+v", protocol)
	}

	neighbour := h15ProbeAt(t, f.ChildBlocks, 2, 30000)
	if !reflect.DeepEqual(neighbour.Units, []int{2, 3}) || neighbour.Outcome.Type != "exception" || neighbour.Outcome.Code != 4 {
		t.Fatalf("neighbour units: %+v", neighbour)
	}
	silent := h15ProbeAt(t, f.ChildBlocks, 0, 30000)
	if silent.Outcome.Type != "no_response" || silent.Outcome.Attempts != 4 {
		t.Fatalf("unit 0: %+v", silent)
	}
	if pdu, err := h15ProbePDU(silent); err != nil || pdu != nil {
		t.Fatalf("a silent unit has no response: %v %v", pdu, err)
	}
	units := map[int]bool{}
	for _, probe := range f.ChildBlocks.Probes {
		for _, unit := range probe.Units {
			units[unit] = true
		}
	}
	if !reflect.DeepEqual(units, map[int]bool{0: true, 1: true, 2: true, 3: true}) {
		t.Fatalf("units probed: %v", units)
	}
}

func TestH15BasicReplyFixtureCarriesSixObjectsAnsweredAtTheChild(t *testing.T) {
	f := h15LoadAll(t)
	pdu, err := h03BasicPDU(f.ChildBasic.Read, f.ChildBasic.Reply)
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
	if !reflect.DeepEqual(ids, []uint8{0x00, 0x01, 0x02, 0x03, 0x15, 0x17}) || parsed.length != 55 || parsed.conformity != 0x01 ||
		parsed.more != 0 || parsed.next != 0 || parsed.readDeviceIDCode != 1 || int(parsed.declared) != len(parsed.objects) {
		t.Fatalf("basic reply structure: ids=%v %+v", ids, parsed)
	}
	if string(parsed.objects[0].Value) != "Huawei" || string(parsed.objects[1].Value) != "SUN2000" || len(parsed.objects[2].Value) != 17 {
		t.Fatalf("basic identity objects: %v", parsed.objects[:3])
	}
	for _, object := range parsed.objects[3:] {
		if len(object.Value) != 2 {
			t.Fatalf("opaque object %02x has length %d", object.ObjectID, len(object.Value))
		}
	}
	if !reflect.DeepEqual(f.ChildBasic.AnsweredForUnits, []int{1}) {
		t.Fatalf("answered for units %v", f.ChildBasic.AnsweredForUnits)
	}
}

func TestH15ExtendedInventoryFixtureRebuildsOneUnpaddedEntryAndDigestCoversEveryByte(t *testing.T) {
	f := h15LoadAll(t)
	page := f.ChildInventory.Pages[0]
	if f.ChildInventory.Unit != 1 || f.ChildInventory.Read.ReadDeviceIDCode != 3 || f.ChildInventory.Read.StartObjectID != 0x87 || len(f.ChildInventory.Pages) != 1 {
		t.Fatalf("read definition: %+v", f.ChildInventory)
	}
	pdu, err := h03InventoryPDU(f.ChildInventory.Read, page)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := h03ParseMEI(pdu)
	if err != nil {
		t.Fatal(err)
	}
	// One response of 87 bytes, conformity 3, no continuation, a next identifier
	// that is set anyway, and a declared-object field equal to the objects present.
	if parsed.length != 87 || parsed.conformity != 0x03 || parsed.more != 0x00 || parsed.next != 0x87 || int(parsed.declared) != 2 || len(parsed.objects) != 2 {
		t.Fatalf("inventory response: %+v", parsed)
	}
	if parsed.objects[0].ObjectID != 0x87 || !bytes.Equal(parsed.objects[0].Value, []byte{1}) || parsed.objects[1].ObjectID != 0x88 {
		t.Fatalf("inventory objects: %+v", parsed.objects)
	}
	entry := parsed.objects[1].Value
	if len(entry) != 75 || bytes.IndexByte(entry, 0) != -1 || page.Objects[1].PadTo != len(page.Objects[1].Entry) {
		t.Fatalf("the entry must be 75 bytes and unpadded: %d", len(entry))
	}
	var keys []string
	for _, pair := range h03EntryAttributes(page.Objects[1].Entry) {
		keys = append(keys, pair[0])
	}
	if got := strings.Join(keys, ";"); got != "1;2;3;4;5;6" {
		t.Fatalf("entry key order %q", got)
	}
	if h03AttributeValue(page.Objects[1].Entry, "1") != h15Model || h03AttributeValue(page.Objects[1].Entry, "5") != "1" ||
		h03AttributeValue(page.Objects[1].Entry, "8") != "" || h03AttributeValue(page.Objects[1].Entry, "9") != "" {
		t.Fatalf("entry attributes: %q", page.Objects[1].Entry)
	}

	pdus := h15ChildPDUs(t, f)
	original := h03Digest(pdus)
	if h03Digest(h15ChildPDUs(t, h15LoadAll(t))) != original {
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
				t.Fatalf("digest ignored response %d byte %d", index, offset)
			}
		}
	}
	if h03Digest(pdus[1:]) == original {
		t.Fatal("digest ignored a missing response")
	}
}

func TestH15FirstRequestTimeoutIsRecordedAndNotRequired(t *testing.T) {
	f := h15LoadAll(t)
	timing := f.Timing
	if timing.Scope != "first request on a fresh connection" || timing.RequiredByCard || timing.AttemptDeadlineMS != 5000 || timing.PromptBoundMS != 100 || len(timing.Observations) != 2 {
		t.Fatalf("timing fixture: %+v", timing)
	}
	for index, observation := range timing.Observations {
		if observation.Run != index+1 || len(observation.Attempts) != 2 ||
			observation.Attempts[0] != (h15TimingAttempt{Outcome: "timeout", ElapsedClass: "at_deadline"}) ||
			observation.Attempts[1].ElapsedClass != "within_prompt_bound" || observation.Attempts[1].Outcome == "timeout" {
			t.Fatalf("observation %d: %+v", index, observation)
		}
	}
	// One observation is a gateway-unit request, the other a child-unit request.
	if timing.Observations[0].Unit != 100 || timing.Observations[0].Function != "FC2B" || timing.Observations[1].Unit != 1 || timing.Observations[1].Function != "FC03" {
		t.Fatalf("timing observations: %+v", timing.Observations)
	}
	// The retry's answer is the evidence the other fixtures hold.
	if timing.Observations[0].Attempts[1].Outcome != "exception" || f.GatewayBasic.Reply.ExceptionCode != 1 {
		t.Fatalf("the answered retry of the gateway request is not the recorded refusal: %+v", timing.Observations[0])
	}

	// The card consumes answered replies only: no input field is a timing, attempt
	// or elapsed-time value, so no timeout can be required or forged.
	var walk func(reflect.Type, string)
	seen := map[reflect.Type]bool{}
	walk = func(kind reflect.Type, path string) {
		for kind.Kind() == reflect.Ptr || kind.Kind() == reflect.Slice {
			kind = kind.Elem()
		}
		if kind.Kind() != reflect.Struct || seen[kind] {
			return
		}
		seen[kind] = true
		for index := 0; index < kind.NumField(); index++ {
			field := kind.Field(index)
			lower := strings.ToLower(field.Name)
			for _, word := range []string{"timeout", "timing", "elapsed", "attempt", "retry", "deadline", "duration"} {
				if strings.Contains(lower, word) {
					t.Fatalf("input field %s.%s carries timing", path, field.Name)
				}
			}
			walk(field.Type, path+"."+field.Name)
		}
	}
	walk(reflect.TypeOf(HuaweiSDongleObservedQualificationInput{}), "Input")

	// Removing the timing record changes nothing about the evaluation.
	input := h15ValidInput(t)
	if result := NewHuaweiSDongleObservedQualificationCard().Evaluate(input); !result.IdentityMatched() {
		t.Fatalf("answered replies alone did not match: %#v", result)
	}
}

func TestH15CommittedFixturesAreSanitized(t *testing.T) {
	f := h15LoadAll(t)
	for _, object := range f.ChildInventory.Pages[0].Objects {
		if object.Count != nil {
			continue
		}
		if err := h15CheckEntrySanitized(object); err != nil {
			t.Fatal(err)
		}
	}
	for _, object := range f.ChildBasic.Reply.Objects {
		switch object.ObjectID {
		case 0x00, 0x01:
			if object.Sanitized || (object.Text != "Huawei" && object.Text != "SUN2000") {
				t.Fatalf("basic identity object %02x: %+v", uint8(object.ObjectID), object)
			}
		case 0x02:
			if !object.Sanitized || !h03FillText.MatchString(object.Text) || len(object.Text) != 17 {
				t.Fatalf("basic version object is not a placeholder: %+v", object)
			}
		default:
			if !object.Sanitized || object.Text != "" || object.OpaqueBytes != 2 {
				t.Fatalf("opaque object %02x: %+v", uint8(object.ObjectID), object)
			}
		}
	}
	for name, set := range map[string]h03ProbeFixture{"gateway": f.Gateway, "child blocks": f.ChildBlocks, "overlap": f.Overlap} {
		for _, probe := range set.Probes {
			if err := h15CheckProbeSanitized(probe); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
	for _, name := range h15FixtureNames() {
		if findings := h03ShapeFindings(string(h03Read1(t, name))); len(findings) != 0 {
			t.Fatalf("%s holds serial-, ESN-, address- or time-shaped values: %v", name, findings)
		}
	}
}

func TestH15SanitizationChecksRejectSerialEsnAndPartNumberShapes(t *testing.T) {
	f := h15LoadAll(t)
	valid := f.ChildInventory.Pages[0].Objects[1]
	if err := h15CheckEntrySanitized(valid); err != nil {
		t.Fatal(err)
	}
	serial := h03CountingDigits(12)
	mixed := h03Joined("AB", "12", "CD", "34", "EF", "56")
	for name, mutate := range map[string]func(*h03Object){
		"digit serial in attribute 4": func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "4=XX0000000000", "4="+serial, 1) },
		"mixed serial in attribute 4": func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "4=XX0000000000", "4="+mixed, 1) },
		"version-like attribute 3":    func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "3=X0.00-X0.0", "3=V1.00-R2.1", 1) },
		"firmware-like attribute 2": func(o *h03Object) {
			o.Entry = strings.Replace(o.Entry, "2=X000X000X00XXX000", "2=V100R001C00SPC123", 1)
		},
		"attribute 6 literal":            func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "6=0.0", "6=9.9", 1) },
		"attribute 2 not declared":       func(o *h03Object) { o.SanitizedAttributes = []string{"3", "4", "6"} },
		"literal attribute declared":     func(o *h03Object) { o.SanitizedAttributes = append(o.SanitizedAttributes, "5") },
		"address as unit":                func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "5=1", "5=192.0.2.1", 1) },
		"other model":                    func(o *h03Object) { o.Entry = strings.Replace(o.Entry, h15Model, "SUN2000-50KTL-M3", 1) },
		"smartlogger key 8":              func(o *h03Object) { o.Entry += ";8=SUN2000" },
		"smartlogger key 9":              func(o *h03Object) { o.Entry += ";9=0" },
		"repeated key":                   func(o *h03Object) { o.Entry += ";6=0.0" },
		"dropped key":                    func(o *h03Object) { o.Entry = strings.Replace(o.Entry, ";6=0.0", "", 1) },
		"declared attribute not present": func(o *h03Object) { o.SanitizedAttributes = append(o.SanitizedAttributes, "10") },
	} {
		object := valid
		object.SanitizedAttributes = append([]string(nil), valid.SanitizedAttributes...)
		mutate(&object)
		if err := h15CheckEntrySanitized(object); err == nil {
			t.Errorf("%s: structural sanitization accepted %q", name, object.Entry)
		}
	}

	for name, probe := range map[string]h03Probe{
		"serial block literal": {Outcome: h03Outcome{Type: "text", Text: mixed, Sanitized: true}},
		"serial not declared":  {Outcome: h03Outcome{Type: "text", Text: "XX0000000000"}},
		"residue literal":      {Outcome: h03Outcome{Type: "text_with_residue", Text: h15Model, Residue: h03Joined("0123", "4567", "-", "890"), Sanitized: true}},
		"residue not declared": {Outcome: h03Outcome{Type: "text_with_residue", Text: h15Model, Residue: "00000000-000"}},
		"other model":          {Outcome: h03Outcome{Type: "text_with_residue", Text: "SUN2000-50KTL-M3", Residue: "00000000-000", Sanitized: true}},
		"clock words":          {Outcome: h03Outcome{Type: "words", Words: []uint16{0x6c1d, 0x3a40}}},
		"zero attempts":        {Outcome: h03Outcome{Type: "no_response"}},
		"exception 0":          {Outcome: h03Outcome{Type: "exception"}},
		"unknown type":         {Outcome: h03Outcome{Type: "blob"}},
	} {
		if err := h15CheckProbeSanitized(probe); err == nil {
			t.Errorf("%s: probe sanitization accepted %+v", name, probe.Outcome)
		}
	}
}

func TestH15NewFixturesAreCoveredByTheCommittedShapeScan(t *testing.T) {
	scanned := map[string]bool{}
	for _, path := range h03CommittedHuaweiTestdata(t) {
		for _, name := range h15FixtureNames() {
			if strings.HasSuffix(path, "/"+name) {
				scanned[name] = true
			}
		}
	}
	if len(scanned) != len(h15FixtureNames()) {
		t.Fatalf("the scan walk misses fixtures: %v", scanned)
	}
	// The scan flags a clock value, a serial and an address placed in a new fixture.
	for name, text := range map[string]string{
		"clock":   `"observed": "2026-10-24"`,
		"time":    `"observed": "22:04:20"`,
		"serial":  `"text": "` + h03CountingDigits(12) + `"`,
		"address": `"endpoint": "198.51.100.7:502"`,
	} {
		if len(h03ShapeFindings(text)) == 0 {
			t.Errorf("%s was not found", name)
		}
	}
}

func TestH15NegativeOverlapProbesAreExecutableAgainstTheRegistryGates(t *testing.T) {
	f := h15LoadAll(t)
	words := func(unit int, offset uint16) []uint16 {
		return h15Words(t, h15ProbeAt(t, f.Overlap, unit, offset))
	}
	asText := func(values []uint16) string {
		var data []byte
		for _, word := range values {
			data = append(data, byte(word>>8), byte(word))
		}
		return string(data)
	}

	// SmartLogger counter: an exception at the child, silence at unit 0.
	if probe := h15ProbeAt(t, f.Overlap, 1, 65521); probe.Outcome.Type != "exception" || probe.Outcome.Code != 2 {
		t.Fatalf("counter read at the child: %+v", probe)
	}
	if probe := h15ProbeAt(t, f.Overlap, 0, 65521); probe.Outcome.Type != "no_response" {
		t.Fatalf("counter read at unit 0: %+v", probe)
	}

	// EMMA: the offering and software offsets answer the child's own text, the
	// model offset answers non-text words, and unit 0 is silent.
	model := h15ProbeAt(t, f.ChildBlocks, 1, 30000)
	firmware := h15ProbeByRole(t, f.ChildBlocks, "identity_firmware")
	offering := string(h15Data(t, model))
	software := string(h15Data(t, firmware))
	// Read as raw extents, the offering carries a NUL inside its text and the
	// EMMA gate stops there; cut at the first NUL, as the model read is cut, it
	// is plain text and the gate stops at the non-text model block.
	raw := EvaluateHuaweiEMMAOfflineIdentity(offering, asText(words(1, 30222)), software)
	if raw.Matched() || raw.Reason() != HuaweiEMMAOfflineOfferingMismatch {
		t.Fatalf("the child satisfied the EMMA gate on the raw extent: %#v", raw)
	}
	cut := EvaluateHuaweiEMMAOfflineIdentity(offering[:strings.IndexByte(offering, 0)], asText(words(1, 30222)), software)
	if cut.Matched() || cut.Reason() != HuaweiEMMAOfflineModelMismatch {
		t.Fatalf("the child satisfied the EMMA gate on the cut text: %#v", cut)
	}
	for _, offset := range []uint16{30222, 30035} {
		if probe := h15ProbeAt(t, f.Overlap, 0, offset); probe.Outcome.Type != "no_response" {
			t.Fatalf("EMMA read %d at unit 0: %+v", offset, probe)
		}
	}
	if result := NewHuaweiEMMAQualificationCard().Evaluate(HuaweiEMMAQualificationInput{UnitID: 0}); result.IdentityMatched() {
		t.Fatalf("EMMA matched on silence: %#v", result)
	}

	// S-Dongle registers answer at the child, so readability is no identity: the
	// offline gate refuses the child's unit and the child's model, and the card
	// refuses the child's replies presented as the gateway's.
	if got := words(1, 37410); !reflect.DeepEqual(got, []uint16{4, 0, 0}) {
		t.Fatalf("search read at the child: %v", got)
	}
	if got := words(1, 37429); !reflect.DeepEqual(got, []uint16{10}) {
		t.Fatalf("capacity read at the child: %v", got)
	}
	for _, unit := range []uint8{1, 100} {
		decision := EvaluateHuaweiSDongleOfflineObservation(HuaweiSDongleOfflineObservation{
			UnitID: unit, Model: h15Model, Firmware: "V200R025C00SPC120", ProtocolMajor: 5, ProtocolMinor: 0,
			SearchState: 0, ChangeSequenceBefore: 0, ChangeSequenceAfter: 0, Capacity: 10, ChildCount: 1,
		})
		want := HuaweiSDongleOfflineUnitMismatch
		if unit == 100 {
			want = HuaweiSDongleOfflineModelMismatch
		}
		if decision.Matched() || decision.Reason() != want {
			t.Fatalf("the child satisfied the S-Dongle offline gate at unit %d: %#v", unit, decision)
		}
	}

	// SunSpec: the marker read answers two words that are not the marker.
	probe := h15ProbeAt(t, f.Overlap, 1, 40000)
	if len(probe.Outcome.Words) != 2 || (probe.Outcome.Words[0] == sunSpecSignatureFirst && probe.Outcome.Words[1] == sunSpecSignatureSecond) {
		t.Fatalf("SunSpec marker read: %+v", probe)
	}
	if _, err := h03SunSpecDecoder(t).Parse(probe.Outcome.Words); err == nil {
		t.Fatal("SunSpec decoder accepted the read at the child")
	}

	// Directions with no evidence stay Unknown: no probe at a unit outside the
	// observed set, and no Growatt offset was read.
	growatt := map[uint16]bool{9: true, 23: true, 43: true, 82: true, 88: true}
	for _, set := range []h03ProbeFixture{f.Overlap, f.ChildBlocks, f.Gateway} {
		for _, probe := range set.Probes {
			if growatt[probe.Offset] {
				t.Fatalf("a Growatt offset was probed: %+v", probe)
			}
			for _, unit := range probe.Units {
				if unit > 3 && unit != 100 {
					t.Fatalf("probe at unit %d implies a unit that was not observed", unit)
				}
			}
		}
	}
	for _, probe := range f.Overlap.Probes {
		if probe.Offset == 65521 && containsInt(probe.Units, 100) {
			t.Fatal("the SmartLogger counter was never read at the gateway unit and must not appear there")
		}
	}
}

func containsInt(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// TestH15FixtureChangesTravelThroughTheWireIntoTheCard changes the fixtures
// rather than the decoded input: the response bytes are rebuilt, decoded by the
// independent parser and only then evaluated, so a fixture that disagrees with
// its own wire facts, or an exact tuple that a fixture no longer holds, is seen.
func TestH15FixtureChangesTravelThroughTheWireIntoTheCard(t *testing.T) {
	h15RequireControl(t)
	card := NewHuaweiSDongleObservedQualificationCard()
	rejected := map[string]func(*h15Fixtures){
		"model block of another model": h15SetModelBlock("SUN2000-8KTL-M2", "00000000-000"),
		"entry unit changed":           h15ReplaceInEntry(";5=1;", ";5=2;"),
		"entry model changed":          h15ReplaceInEntry("1=SUN2000-8KTL-M1;", "1=SUN2000-8KTL-M2;"),
		"entry with a missing key":     h15ReplaceInEntry(";6=0.0", ""),
		"gateway refusal code changed": func(f *h15Fixtures) { f.GatewayBasic.Reply.ExceptionCode = 2 },
		"gateway protocol words of the child": func(f *h15Fixtures) {
			h15ProbeSetWords(f, "sdongle_protocol", []uint16{5, 0})
		},
		"search sequence moved": func(f *h15Fixtures) {
			h15ProbeSetWords(f, "sdongle_search_after", []uint16{4, 0, 8})
		},
		"search still running": func(f *h15Fixtures) {
			h15ProbeSetWords(f, "sdongle_search_status", []uint16{1})
		},
	}
	for name, mutate := range rejected {
		f := h15LoadAll(t)
		mutate(&f)
		h15RequireRejected(t, name, card.Evaluate(h15Input(t, f)))
	}

	accepted := map[string]func(*h15Fixtures){
		"model block without residue":    h15SetModelBlock(h15Model, ""),
		"model block with other residue": h15SetModelBlock(h15Model, "XXXXXXXXXXXX"),
		"entry in reverse order":         h15SetEntry("6=0.0;5=1;4=XX0000000000;3=X0.00-X0.0;2=X000X000X00XXX000;1=" + h15Model),
		"entry with other placeholders":  h15ReplaceInEntry("4=XX0000000000", "4=XXXXXXXXXXXX"),
		// The observed keys 1 through 6 are a lower bound: further keys are
		// retained as opaque native evidence.
		"entry with the logger's keys": h15ReplaceInEntry(";6=0.0", ";6=0.0;8=SUN2000;9=0"),
	}
	for name, mutate := range accepted {
		f := h15LoadAll(t)
		mutate(&f)
		if result := card.Evaluate(h15Input(t, f)); !result.IdentityMatched() {
			t.Fatalf("%s: rejected: %#v", name, result)
		}
	}

	// A fixture whose own wire claims are wrong cannot be rebuilt.
	f := h15LoadAll(t)
	f.ChildInventory.Pages[0].PDUBytes++
	if _, err := h03InventoryPDU(f.ChildInventory.Read, f.ChildInventory.Pages[0]); err == nil {
		t.Fatal("an inventory fixture with a wrong frame length was rebuilt")
	}
	f = h15LoadAll(t)
	f.ChildBasic.Reply.PDUBytes--
	if _, err := h03BasicPDU(f.ChildBasic.Read, f.ChildBasic.Reply); err == nil {
		t.Fatal("a basic reply fixture with a wrong frame length was rebuilt")
	}
	f = h15LoadAll(t)
	f.GatewayBasic.Reply.PDUBytes = 3
	if _, err := h15MEIExceptionPDU(f.GatewayBasic.Reply); err == nil {
		t.Fatal("an exception fixture with a wrong frame length was rebuilt")
	}
	f = h15LoadAll(t)
	f.ChildBlocks.Probes[0].Outcome.Residue = strings.Repeat("0", 14)
	if _, err := h15ProbePDU(f.ChildBlocks.Probes[0]); err == nil {
		t.Fatal("a model block that leaves no padding was rebuilt")
	}
}

func h15ProbeSetWords(f *h15Fixtures, role string, words []uint16) {
	for index := range f.Gateway.Probes {
		for _, candidate := range f.Gateway.Probes[index].Roles {
			if candidate == role {
				f.Gateway.Probes[index].Outcome.Words = words
				f.Gateway.Probes[index].Quantity = uint16(len(words))
			}
		}
	}
}
