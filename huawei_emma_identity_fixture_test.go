package modbusreg

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The EMMA fixtures reuse the Huawei-local identity schema of the SmartLogger
// and S-Dongle slices. The schema gains one optional outcome field, residue_at,
// for a model block whose printable residue starts after a run of NUL bytes
// instead of directly after the first NUL, and one fixture kind,
// request_timing, that records the timing classes of one connection. Every
// earlier fixture still decodes unchanged.

const (
	h18Label        = "SmartHEMS"
	h18Model        = "EMMA-A02"
	h18Version      = "V100R025C00SPC131"
	h18Fill         = "X000X000X00XXX000"
	h18BackupModel  = "XxxxxXxxxx-00X-X0"
	h18ChildModel   = "SUN2000-00X-XXX0"
	h18ChildResidue = "00000000-0"
	h18Gateway      = 0
	h18ChildExtent  = 30
)

type h18TimingClass struct {
	ElapsedClass string `json:"elapsed_class"`
	Count        int    `json:"count"`
}

type h18TimingObservation struct {
	Role     string             `json:"role"`
	Unit     int                `json:"unit"`
	Function string             `json:"function"`
	Offset   uint16             `json:"offset,omitempty"`
	Attempts []h15TimingAttempt `json:"attempts"`
}

type h18TimingFixture struct {
	h03Header
	Scope             string                 `json:"scope"`
	RequiredByCard    bool                   `json:"required_by_card"`
	AttemptDeadlineMS int                    `json:"attempt_deadline_ms"`
	PromptBoundMS     int                    `json:"prompt_bound_ms"`
	Requests          int                    `json:"requests"`
	Retries           int                    `json:"retries"`
	Classes           []h18TimingClass       `json:"classes"`
	Observations      []h18TimingObservation `json:"observations"`
}

// h18Fixtures holds every EMMA fixture so that a test can change one field and
// rebuild the card input through the wire representation.
type h18Fixtures struct {
	Gateway      h03ProbeFixture
	GatewayBasic h03BasicFixture
	Inventory    h15InventoryFixture
	Children     h03ProbeFixture
	Unit100      h03ProbeFixture
	Overlap      h03ProbeFixture
	Timing       h18TimingFixture
}

func h18FixtureNames() []string {
	return []string{
		"emma_child_identity_blocks.json",
		"emma_extended_inventory.json",
		"emma_gateway_basic_reply.json",
		"emma_gateway_identity_blocks.json",
		"emma_negative_overlap.json",
		"emma_request_timing.json",
		"emma_unit100_behaviour.json",
	}
}

func h18FixturePaths() []string {
	var paths []string
	for _, name := range h18FixtureNames() {
		paths = append(paths, h03FixtureDirectory+"/"+name)
	}
	return paths
}

func h18Target(name string) any {
	switch name {
	case "emma_extended_inventory.json":
		return &h15InventoryFixture{}
	case "emma_gateway_basic_reply.json":
		return &h03BasicFixture{}
	case "emma_request_timing.json":
		return &h18TimingFixture{}
	}
	return &h03ProbeFixture{}
}

func h18LoadAll(t testing.TB) h18Fixtures {
	t.Helper()
	var f h18Fixtures
	for name, target := range map[string]any{
		"emma_gateway_identity_blocks.json": &f.Gateway,
		"emma_gateway_basic_reply.json":     &f.GatewayBasic,
		"emma_extended_inventory.json":      &f.Inventory,
		"emma_child_identity_blocks.json":   &f.Children,
		"emma_unit100_behaviour.json":       &f.Unit100,
		"emma_negative_overlap.json":        &f.Overlap,
		"emma_request_timing.json":          &f.Timing,
	} {
		if err := h03Decode(h03Read1(t, name), target); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	return f
}

// ---- wire reconstruction -------------------------------------------------

// h18ProbePDU rebuilds the response of one probe. A residue that starts at a
// byte offset of its own is placed there; every other outcome is rebuilt by the
// earlier helpers.
func h18ProbePDU(probe h03Probe) ([]byte, error) {
	outcome := probe.Outcome
	if outcome.Type != "text_with_residue" || outcome.ResidueAt == 0 {
		return h15ProbePDU(probe)
	}
	extent := int(probe.Quantity) * 2
	if outcome.ResidueAt <= len(outcome.Text) || outcome.ResidueAt+len(outcome.Residue) > extent {
		return nil, fmt.Errorf("residue at %d does not fit after the %d byte text in the %d byte extent", outcome.ResidueAt, len(outcome.Text), extent)
	}
	data := make([]byte, extent)
	copy(data, outcome.Text)
	copy(data[outcome.ResidueAt:], outcome.Residue)
	return append([]byte{0x03, byte(extent)}, data...), nil
}

// h18Data returns the register bytes of an answered probe and nil for an
// exception or for no response.
func h18Data(t testing.TB, probe h03Probe) []byte {
	t.Helper()
	pdu, err := h18ProbePDU(probe)
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

func h18Words(t testing.TB, probe h03Probe) []uint16 {
	t.Helper()
	data := h18Data(t, probe)
	var words []uint16
	for index := 0; index+1 < len(data); index += 2 {
		words = append(words, uint16(data[index])<<8|uint16(data[index+1]))
	}
	return words
}

func h18BasicReply(t testing.TB, fixture h03BasicFixture) *HuaweiEMMAObservedBasicReply {
	t.Helper()
	pdu, err := h03BasicPDU(fixture.Read, fixture.Reply)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := h03ParseMEI(pdu)
	if err != nil {
		t.Fatal(err)
	}
	return &HuaweiEMMAObservedBasicReply{Objects: parsed.objects}
}

// h18Pages rebuilds every inventory response and takes its frame length,
// declared-object field, continuation and next identifier from the wire.
func h18Pages(t testing.TB, fixture h15InventoryFixture) []HuaweiEMMAObservedInventoryPage {
	t.Helper()
	var pages []HuaweiEMMAObservedInventoryPage
	for _, page := range fixture.Pages {
		pdu, err := h03InventoryPDU(fixture.Read, page)
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
		pages = append(pages, HuaweiEMMAObservedInventoryPage{
			ReadDeviceIDCode: parsed.readDeviceIDCode,
			Conformity:       parsed.conformity,
			ResponseBytes:    parsed.length,
			DeclaredObjects:  int(parsed.declared),
			More:             parsed.more == 0xff,
			NextObjectID:     parsed.next,
			Objects:          parsed.objects,
		})
	}
	return pages
}

// h18Input rebuilds every wire response from the fixtures and decodes them into
// the card input. Nothing in the input comes from a fixture claim about itself.
func h18Input(t testing.TB, f h18Fixtures) HuaweiEMMAObservedQualificationInput {
	t.Helper()
	label := h15ProbeByRole(t, f.Gateway, "emma_family_label")
	model := h15ProbeByRole(t, f.Gateway, "emma_model")
	software := h15ProbeByRole(t, f.Gateway, "emma_software_label")
	if len(label.Units) != 1 || !reflect.DeepEqual(label.Units, model.Units) || !reflect.DeepEqual(label.Units, software.Units) {
		t.Fatalf("gateway blocks are not addressed to one unit: %v %v %v", label.Units, model.Units, software.Units)
	}
	input := HuaweiEMMAObservedQualificationInput{
		Gateway: HuaweiEMMAObservedGatewayReads{
			UnitID:    uint8(label.Units[0]),
			Offering:  h18Data(t, label),
			Model:     h18Data(t, model),
			Software:  h18Data(t, software),
			Basic:     h18BasicReply(t, f.GatewayBasic),
			Inventory: h18Pages(t, f.Inventory),
		},
	}
	block := h15ProbeByRole(t, f.Children, "child_model_block")
	for _, unit := range block.Units {
		input.Children = append(input.Children, HuaweiEMMAObservedChildReads{UnitID: uint8(unit), ModelBlock: h18Data(t, block)})
	}
	return input
}

func h18ValidInput(t testing.TB) HuaweiEMMAObservedQualificationInput {
	t.Helper()
	return h18Input(t, h18LoadAll(t))
}

// h18SetEntry rewrites the entry of one inventory object, unpadded, and keeps
// the fixture's own frame length consistent with it.
func h18SetEntry(objectID uint8, mutate func(string) string) func(*h18Fixtures) {
	return func(f *h18Fixtures) {
		for pageIndex := range f.Inventory.Pages {
			page := &f.Inventory.Pages[pageIndex]
			for objectIndex := range page.Objects {
				object := &page.Objects[objectIndex]
				if uint8(object.ObjectID) != objectID {
					continue
				}
				before := len(object.Entry)
				object.Entry = mutate(object.Entry)
				object.PadTo = len(object.Entry)
				page.PDUBytes += len(object.Entry) - before
			}
		}
	}
}

// h18ReplaceInEntry rewrites part of an entry and keeps it unpadded.
func h18ReplaceInEntry(objectID uint8, old, replacement string) func(*h18Fixtures) {
	return h18SetEntry(objectID, func(entry string) string {
		if !strings.Contains(entry, old) {
			panic("entry does not contain " + old)
		}
		return strings.Replace(entry, old, replacement, 1)
	})
}

// h18SetProbeOutcome replaces the outcome of every probe that has the role.
func h18SetProbeOutcome(set func(*h18Fixtures) *h03ProbeFixture, role string, outcome h03Outcome) func(*h18Fixtures) {
	return func(f *h18Fixtures) {
		probes := set(f).Probes
		for index := range probes {
			for _, candidate := range probes[index].Roles {
				if candidate == role {
					probes[index].Outcome = outcome
				}
			}
		}
	}
}

func h18GatewaySet(f *h18Fixtures) *h03ProbeFixture  { return &f.Gateway }
func h18ChildrenSet(f *h18Fixtures) *h03ProbeFixture { return &f.Children }

// h18PDUs rebuilds every response of the EMMA fixtures; the digest of the whole
// set is computed by the tests and never stored.
func h18PDUs(t testing.TB, f h18Fixtures) [][]byte {
	t.Helper()
	var pdus [][]byte
	inventory, err := h18InventoryPDUs(f.Inventory)
	if err != nil {
		t.Fatal(err)
	}
	pdus = append(pdus, inventory...)
	basic, err := h03BasicPDU(f.GatewayBasic.Read, f.GatewayBasic.Reply)
	if err != nil {
		t.Fatal(err)
	}
	pdus = append(pdus, basic)
	for _, set := range []h03ProbeFixture{f.Gateway, f.Children, f.Unit100, f.Overlap} {
		for _, probe := range set.Probes {
			pdu, err := h18ProbePDU(probe)
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

func h18InventoryPDUs(fixture h15InventoryFixture) ([][]byte, error) {
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

// ---- structural sanitization ---------------------------------------------

var (
	h18ChildModelFill = regexp.MustCompile(`^SUN2000(?:-[0Xx]+)+$`)
	h18Families       = map[string]bool{"HEMS": true, "BackupBox": true, "SUN2000": true}
)

// h18CheckEntrySanitized enforces an allow-list per attribute. Only the gateway
// model alias, the gateway's own version token, the device-type words and the
// unit are literal; every other value must be declared sanitized and consist of
// placeholder characters. An inverter model keeps its family token and a
// placeholder, and the model of the backup-box entry is a placeholder whole.
func h18CheckEntrySanitized(object h03Object) error {
	declared := map[string]bool{}
	for _, key := range object.SanitizedAttributes {
		declared[key] = true
	}
	attributes := map[string]string{}
	for _, pair := range h03EntryAttributes(object.Entry) {
		attributes[pair[0]] = pair[1]
	}
	gateway := attributes["1"] == h18Model
	seen := map[string]bool{}
	for _, pair := range h03EntryAttributes(object.Entry) {
		key, value := pair[0], pair[1]
		if seen[key] {
			return fmt.Errorf("attribute %s is repeated", key)
		}
		seen[key] = true
		switch key {
		case "1":
			switch {
			case declared[key] && attributes["8"] == "SUN2000":
				if !h18ChildModelFill.MatchString(value) {
					return fmt.Errorf("attribute 1 %q is not the family token and a placeholder", value)
				}
			case declared[key] && attributes["8"] == "BackupBox":
				if !h03FillText.MatchString(value) {
					return fmt.Errorf("attribute 1 %q is not a whole placeholder", value)
				}
			case !declared[key] && value == h18Model:
			default:
				return fmt.Errorf("attribute 1 %q is neither the published gateway alias nor a declared placeholder", value)
			}
		case "2":
			switch {
			case gateway && !declared[key] && value == h18Version:
			case !gateway && declared[key] && h03FillText.MatchString(value):
			default:
				return fmt.Errorf("attribute 2 %q is neither the gateway's own token nor a declared placeholder", value)
			}
		case "3", "4", "6", "10":
			if !declared[key] || !h03FillText.MatchString(value) {
				return fmt.Errorf("attribute %s is not a declared placeholder: %q", key, value)
			}
		case "5":
			if !h03Decimal.MatchString(value) || declared[key] {
				return fmt.Errorf("attribute 5 %q is not a plain unit", value)
			}
		case "8":
			if !h18Families[value] || declared[key] {
				return fmt.Errorf("attribute 8 %q is not a published family", value)
			}
		case "9":
			if value != "0" || declared[key] {
				return fmt.Errorf("attribute 9 %q is not the published constant", value)
			}
		default:
			return fmt.Errorf("attribute %q is not part of the observed key set", key)
		}
	}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "8", "9"} {
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

// h18CheckProbeSanitized allows a literal only for the published labels and a
// placeholder everywhere else, and allows data words only when they are small
// published values or marked synthetic.
func h18CheckProbeSanitized(probe h03Probe) error {
	outcome := probe.Outcome
	switch outcome.Type {
	case "text":
		switch {
		case !outcome.Sanitized && (outcome.Text == h18Label || outcome.Text == h18Model || outcome.Text == h18Label+" "+h18Version):
		case outcome.Sanitized && h03FillText.MatchString(outcome.Text):
		default:
			return fmt.Errorf("text %q is neither a published label nor a declared placeholder", outcome.Text)
		}
	case "text_with_residue":
		if !outcome.Sanitized || !h18ChildModelFill.MatchString(outcome.Text) || !h03FillText.MatchString(outcome.Residue) || outcome.ResidueAt <= len(outcome.Text) {
			return fmt.Errorf("model block %+v is not a family token, a placeholder and a placeholder residue", outcome)
		}
	case "words":
		published := reflect.DeepEqual(outcome.Words, []uint16{5, 0}) || reflect.DeepEqual(outcome.Words, []uint16{0, 0})
		if !published && !outcome.Synthetic {
			return fmt.Errorf("data words %v are neither published nor marked synthetic", outcome.Words)
		}
	case "exception":
		if outcome.Code == 0 || outcome.Text != "" || outcome.Residue != "" || outcome.Attempts != 0 {
			return fmt.Errorf("malformed exception outcome %+v", outcome)
		}
	default:
		return fmt.Errorf("unknown outcome %q", outcome.Type)
	}
	return nil
}

// ---- fixture tests -------------------------------------------------------

func TestH18FixturesDecodeStrictlyRoundTripAndRebuildWire(t *testing.T) {
	for _, name := range h18FixtureNames() {
		t.Run(name, func(t *testing.T) {
			data := h03Read1(t, name)
			target := h18Target(name)
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

func TestH18EarlierFixturesStillDecodeUnchanged(t *testing.T) {
	for _, name := range h15FixtureNames() {
		data := h03Read1(t, name)
		target := h15Target(name)
		if err := h03Decode(data, target); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		canonical, err := h03Canonical(target)
		if err != nil || !bytes.Equal(canonical, data) {
			t.Fatalf("%s no longer round-trips: %v", name, err)
		}
	}
}

func TestH18FixtureHeadersAreSyntheticCC0AndFamilyScoped(t *testing.T) {
	f := h18LoadAll(t)
	for name, header := range map[string]struct {
		header h03Header
		kind   string
		family string
	}{
		"gateway blocks": {f.Gateway.h03Header, "fc03_probe_set", "EMMA"},
		"gateway basic":  {f.GatewayBasic.h03Header, "mei_basic_reply", "EMMA"},
		"inventory":      {f.Inventory.h03Header, "mei_extended_inventory", "EMMA"},
		"child blocks":   {f.Children.h03Header, "fc03_probe_set", "SUN2000"},
		"unit 100":       {f.Unit100.h03Header, "fc03_probe_set", "EMMA"},
		"overlap":        {f.Overlap.h03Header, "fc03_probe_set", "EMMA"},
		"timing":         {f.Timing.h03Header, "request_timing", "EMMA"},
	} {
		if err := h03CheckHeader(header.header, header.kind, header.family); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestH18GatewayIdentityFixtureCarriesTheFourBlocksAtUnitZero(t *testing.T) {
	f := h18LoadAll(t)
	if len(f.Gateway.Probes) != 4 {
		t.Fatalf("gateway probes=%d", len(f.Gateway.Probes))
	}
	want := map[string]struct {
		offset   uint16
		quantity uint16
		text     string
		padding  int
	}{
		"emma_family_label":    {30000, 15, h18Label, 21},
		"emma_model":           {30222, 20, h18Model, 32},
		"emma_software_label":  {30035, 15, h18Label + " " + h18Version, 3},
		"serial_bearing_block": {30015, 10, "XX0000000000", 8},
	}
	for role, expected := range want {
		probe := h15ProbeByRole(t, f.Gateway, role)
		data := h18Data(t, probe)
		if !reflect.DeepEqual(probe.Units, []int{h18Gateway}) || probe.Offset != expected.offset || probe.Quantity != expected.quantity ||
			len(data) != int(expected.quantity)*2 || string(data[:len(expected.text)]) != expected.text ||
			!bytes.Equal(data[len(expected.text):], make([]byte, expected.padding)) {
			t.Fatalf("gateway block %s: %+v data=%q", role, probe, data)
		}
	}
	// The software label is the family label, one space and a version-style
	// token; the token is the same placeholder as in the basic reply and entry.
	software := h18Data(t, h15ProbeByRole(t, f.Gateway, "emma_software_label"))
	if label, token, found := strings.Cut(strings.TrimRight(string(software), "\x00"), " "); !found || label != h18Label || token != h18Version {
		t.Fatalf("software label structure: %q", software)
	}
}

func TestH18BasicReplyFixtureCarriesFourteenObjectsAnsweredAtThreeUnits(t *testing.T) {
	f := h18LoadAll(t)
	pdu, err := h03BasicPDU(f.GatewayBasic.Read, f.GatewayBasic.Reply)
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
	if !reflect.DeepEqual(ids, []uint8{0x00, 0x01, 0x02, 0x03, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x17, 0x18, 0x19, 0x1b}) ||
		parsed.length != 118 || parsed.conformity != 0x01 || parsed.more != 0 || parsed.next != 0 || parsed.readDeviceIDCode != 1 ||
		int(parsed.declared) != len(parsed.objects) {
		t.Fatalf("basic reply structure: ids=%v %+v", ids, parsed)
	}
	if string(parsed.objects[0].Value) != "Huawei" || string(parsed.objects[1].Value) != "HEMS" || string(parsed.objects[2].Value) != h18Version {
		t.Fatalf("basic identity objects: %q %q %q", parsed.objects[0].Value, parsed.objects[1].Value, parsed.objects[2].Value)
	}
	var lengths []int
	for _, object := range parsed.objects[3:] {
		lengths = append(lengths, len(object.Value))
		if !bytes.Equal(object.Value, make([]byte, len(object.Value))) {
			t.Fatalf("opaque object %02x carries bytes", object.ObjectID)
		}
	}
	if !reflect.DeepEqual(lengths, []int{2, 12, 2, 2, 2, 2, 2, 2, 8, 2, 20}) {
		t.Fatalf("opaque object lengths: %v", lengths)
	}
	// The same reply was answered at the gateway unit, at the unit that the
	// S-Dongle path uses for its gateway and at an inverter unit: basic
	// identification does not discriminate by unit on this path.
	if !reflect.DeepEqual(f.GatewayBasic.AnsweredForUnits, []int{0, 3, 100}) {
		t.Fatalf("answered for units %v", f.GatewayBasic.AnsweredForUnits)
	}
}

func TestH18ExtendedInventoryFixtureRebuildsFivePagesAndDigestCoversEveryByte(t *testing.T) {
	f := h18LoadAll(t)
	if f.Inventory.Unit != h18Gateway || f.Inventory.Read.ReadDeviceIDCode != 3 || f.Inventory.Read.StartObjectID != 0x87 || len(f.Inventory.Pages) != 5 {
		t.Fatalf("read definition: %+v", f.Inventory)
	}
	pdus, err := h18InventoryPDUs(f.Inventory)
	if err != nil {
		t.Fatal(err)
	}
	wantLengths := []int{10, 88, 102, 111, 111}
	var entries []string
	for index, pdu := range pdus {
		parsed, err := h03ParseMEI(pdu)
		if err != nil {
			t.Fatal(err)
		}
		last := index == len(pdus)-1
		wantMore, wantNext := uint8(0xff), uint8(0x88+index)
		if last {
			wantMore, wantNext = 0x00, 0x00
		}
		// One object per response, the declared-object field is 5 on every page,
		// and no response exceeds the classic frame limit.
		if parsed.length != wantLengths[index] || parsed.length > 253 || parsed.conformity != 0x03 || parsed.readDeviceIDCode != 3 ||
			parsed.more != wantMore || parsed.next != wantNext || int(parsed.declared) != 5 || len(parsed.objects) != 1 ||
			parsed.objects[0].ObjectID != uint8(0x87+index) {
			t.Fatalf("page %d: %+v", index, parsed)
		}
		if index == 0 {
			if !bytes.Equal(parsed.objects[0].Value, []byte{4}) {
				t.Fatalf("count object: %v", parsed.objects[0].Value)
			}
			continue
		}
		entry := parsed.objects[0].Value
		if bytes.IndexByte(entry, 0) != -1 {
			t.Fatalf("entry %d is padded", index)
		}
		entries = append(entries, string(entry))
	}
	// Four entries: the gateway itself at unit 0, a backup-box entry at unit 2
	// and two inverter entries at units 3 and 4.
	var units, families []string
	for _, entry := range entries {
		units = append(units, h03AttributeValue(entry, "5"))
		families = append(families, h03AttributeValue(entry, "8"))
	}
	if !reflect.DeepEqual(units, []string{"0", "2", "3", "4"}) || !reflect.DeepEqual(families, []string{"HEMS", "BackupBox", "SUN2000", "SUN2000"}) {
		t.Fatalf("entry units=%v families=%v", units, families)
	}
	var keys [][]string
	for _, entry := range entries {
		var order []string
		for _, pair := range h03EntryAttributes(entry) {
			order = append(order, pair[0])
		}
		keys = append(keys, order)
	}
	if !reflect.DeepEqual(keys, [][]string{
		{"1", "2", "3", "4", "5", "6", "8", "9"}, {"1", "2", "3", "4", "5", "6", "8", "9"},
		{"1", "2", "3", "4", "5", "6", "8", "10", "9"}, {"1", "2", "3", "4", "5", "6", "8", "10", "9"},
	}) {
		t.Fatalf("entry key order: %v", keys)
	}

	all := h18PDUs(t, f)
	original := h03Digest(all)
	if h03Digest(h18PDUs(t, h18LoadAll(t))) != original {
		t.Fatal("digest is not deterministic")
	}
	for index := range all {
		for offset := range all[index] {
			mutated := make([][]byte, len(all))
			for copyIndex := range all {
				mutated[copyIndex] = append([]byte(nil), all[copyIndex]...)
			}
			mutated[index][offset] ^= 0x01
			if h03Digest(mutated) == original {
				t.Fatalf("digest ignored response %d byte %d", index, offset)
			}
		}
	}
	if h03Digest(all[1:]) == original {
		t.Fatal("digest ignored a missing response")
	}
}

func TestH18ChildIdentityFixtureCarriesTheNulGapResidueAndBothUnits(t *testing.T) {
	f := h18LoadAll(t)
	model := h15ProbeByRole(t, f.Children, "child_model_block")
	data := h18Data(t, model)
	if !reflect.DeepEqual(model.Units, []int{3, 4}) || model.Offset != 30000 || model.Quantity != 15 {
		t.Fatalf("model probe: %+v", model)
	}
	// The model text fills the first ten registers with NUL padding after it; the
	// residue starts at the eleventh register and runs to the end of the extent.
	if len(data) != h18ChildExtent || string(data[:len(h18ChildModel)]) != h18ChildModel ||
		!bytes.Equal(data[len(h18ChildModel):20], make([]byte, 20-len(h18ChildModel))) || string(data[20:]) != h18ChildResidue || len(data[20:]) != 10 {
		t.Fatalf("model block structure: %q", data)
	}
	serial := h15ProbeByRole(t, f.Children, "serial_bearing_block")
	if serialData := h18Data(t, serial); len(serialData) != 20 || len(serial.Outcome.Text) != 12 || bytes.Count(serialData, []byte{0}) != 8 || !reflect.DeepEqual(serial.Units, []int{3, 4}) {
		t.Fatalf("serial-bearing block structure: %+v", serial)
	}
	firmware := h15ProbeByRole(t, f.Children, "child_firmware_block")
	if firmwareData := h18Data(t, firmware); len(firmwareData) != 30 || len(firmware.Outcome.Text) != 11 || bytes.Count(firmwareData, []byte{0}) != 19 {
		t.Fatalf("firmware block structure: %+v", firmware)
	}
	if protocol := h15ProbeByRole(t, f.Children, "child_protocol_words"); !reflect.DeepEqual(h18Words(t, protocol), []uint16{5, 0}) {
		t.Fatalf("child protocol words: %+v", protocol)
	}
	// The inverter units are the two that the inventory names, in its own order.
	var inventoryUnits []int
	for _, page := range f.Inventory.Pages {
		for _, object := range page.Objects {
			if object.Entry != "" && h03AttributeValue(object.Entry, "8") == "SUN2000" {
				unit, err := strconv.Atoi(h03AttributeValue(object.Entry, "5"))
				if err != nil {
					t.Fatal(err)
				}
				inventoryUnits = append(inventoryUnits, unit)
			}
		}
	}
	if !reflect.DeepEqual(inventoryUnits, model.Units) {
		t.Fatalf("inventory names units %v, the model probe reads %v", inventoryUnits, model.Units)
	}
}

func TestH18Unit100AnswersBasicIdentificationAndRefusesTheDongleRegisters(t *testing.T) {
	f := h18LoadAll(t)
	want := map[string]struct {
		offset   uint16
		quantity uint16
	}{"sdongle_protocol": {30068, 2}, "sdongle_search": {37410, 3}, "sdongle_capacity": {37429, 1}}
	if len(f.Unit100.Probes) != len(want) {
		t.Fatalf("unit 100 probes=%d", len(f.Unit100.Probes))
	}
	for role, expected := range want {
		probe := h15ProbeByRole(t, f.Unit100, role)
		if !reflect.DeepEqual(probe.Units, []int{100}) || probe.Offset != expected.offset || probe.Quantity != expected.quantity ||
			probe.Outcome.Type != "exception" || probe.Outcome.Code != 4 {
			t.Fatalf("unit 100 probe %s: %+v", role, probe)
		}
		pdu, err := h18ProbePDU(probe)
		if err != nil {
			t.Fatal(err)
		}
		if parsed, err := h03ParseFC03(pdu, probe.Quantity); err != nil || parsed.exception != 4 {
			t.Fatalf("unit 100 probe %s did not rebuild as exception 4: %+v %v", role, parsed, err)
		}
	}
	// Basic identification is answered at unit 100 with the gateway's own reply,
	// not refused as on the S-Dongle path.
	if !containsInt(f.GatewayBasic.AnsweredForUnits, 100) {
		t.Fatalf("basic identification was not answered at unit 100: %v", f.GatewayBasic.AnsweredForUnits)
	}
}

func TestH18RequestTimingIsRecordedAndNotRequired(t *testing.T) {
	f := h18LoadAll(t)
	timing := f.Timing
	if timing.Scope != "every request of one connection" || timing.RequiredByCard || timing.AttemptDeadlineMS != 5000 || timing.PromptBoundMS != 100 ||
		timing.Requests != 30 || timing.Retries != 0 {
		t.Fatalf("timing fixture: %+v", timing)
	}
	total := 0
	for _, class := range timing.Classes {
		total += class.Count
	}
	if total != timing.Requests || !reflect.DeepEqual(timing.Classes, []h18TimingClass{{"within_prompt_bound", 29}, {"above_prompt_bound", 1}}) {
		t.Fatalf("timing classes: %+v", timing.Classes)
	}
	// Unlike the S-Dongle path, the first request on the connection was answered
	// at once, and the one slow request was an exception, not a lost request.
	first, slow := timing.Observations[0], timing.Observations[1]
	if len(timing.Observations) != 2 || first.Role != "first_request" || len(first.Attempts) != 1 ||
		first.Attempts[0] != (h15TimingAttempt{Outcome: "answered", ElapsedClass: "within_prompt_bound"}) ||
		slow.Role != "slowest_request" || slow.Unit != 0 || slow.Offset != 65521 || len(slow.Attempts) != 1 ||
		slow.Attempts[0] != (h15TimingAttempt{Outcome: "exception", ElapsedClass: "above_prompt_bound"}) {
		t.Fatalf("timing observations: %+v", timing.Observations)
	}
	counter := h15ProbeAt(t, f.Overlap, 0, 65521)
	if counter.Outcome.Type != "exception" || counter.Outcome.Code != 3 {
		t.Fatalf("the slow request is not the recorded counter exception: %+v", counter)
	}

	// The card consumes answered replies only: no input field is a timing,
	// attempt or elapsed-time value, so no latency can be required or forged.
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
			for _, word := range []string{"timeout", "timing", "elapsed", "attempt", "retry", "deadline", "duration", "latency"} {
				if strings.Contains(lower, word) {
					t.Fatalf("input field %s.%s carries timing", path, field.Name)
				}
			}
			walk(field.Type, path+"."+field.Name)
		}
	}
	walk(reflect.TypeOf(HuaweiEMMAObservedQualificationInput{}), "Input")

	if result := NewHuaweiEMMAObservedQualificationCard().Evaluate(h18ValidInput(t)); !result.IdentityMatched() {
		t.Fatalf("answered replies alone did not match: %#v", result)
	}
}

func TestH18CommittedFixturesAreSanitized(t *testing.T) {
	f := h18LoadAll(t)
	for _, page := range f.Inventory.Pages {
		for _, object := range page.Objects {
			if object.Count != nil {
				if *object.Count != 4 || uint8(object.ObjectID) != 0x87 {
					t.Fatalf("count object: %+v", object)
				}
				continue
			}
			if err := h18CheckEntrySanitized(object); err != nil {
				t.Fatalf("object %02x: %v", uint8(object.ObjectID), err)
			}
		}
	}
	for _, object := range f.GatewayBasic.Reply.Objects {
		switch object.ObjectID {
		case 0x00, 0x01:
			if object.Sanitized || (object.Text != "Huawei" && object.Text != "HEMS") {
				t.Fatalf("basic identity object %02x: %+v", uint8(object.ObjectID), object)
			}
		case 0x02:
			if object.Sanitized || object.Text != h18Version {
				t.Fatalf("basic version object is not the gateway's own token: %+v", object)
			}
		default:
			if !object.Sanitized || object.Text != "" || object.OpaqueBytes == 0 {
				t.Fatalf("opaque object %02x: %+v", uint8(object.ObjectID), object)
			}
		}
	}
	for name, set := range map[string]h03ProbeFixture{"gateway": f.Gateway, "children": f.Children, "unit 100": f.Unit100, "overlap": f.Overlap} {
		for _, probe := range set.Probes {
			if err := h18CheckProbeSanitized(probe); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
	for _, name := range h18FixtureNames() {
		if findings := h03ShapeFindings(string(h03Read1(t, name))); len(findings) != 0 {
			t.Fatalf("%s holds serial-, ESN-, address- or time-shaped values: %v", name, findings)
		}
	}
}

func TestH18SanitizationChecksRejectSerialEsnPartNumberAndModelShapes(t *testing.T) {
	f := h18LoadAll(t)
	var self, inverter h03Object
	for _, page := range f.Inventory.Pages {
		for _, object := range page.Objects {
			switch h03AttributeValue(object.Entry, "5") {
			case "0":
				self = object
			case "3":
				inverter = object
			}
		}
	}
	var backup h03Object
	for _, page := range f.Inventory.Pages {
		for _, object := range page.Objects {
			if h03AttributeValue(object.Entry, "5") == "2" {
				backup = object
			}
		}
	}
	for _, valid := range []h03Object{self, backup, inverter} {
		if err := h18CheckEntrySanitized(valid); err != nil {
			t.Fatal(err)
		}
	}
	serial := h03CountingDigits(12)
	mixed := h03Joined("AB", "12", "CD", "34", "EF", "56")
	for name, mutate := range map[string]func(*h03Object){
		"digit serial in attribute 4":    func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "4=XX0000000000", "4="+serial, 1) },
		"mixed serial in attribute 4":    func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "4=XX0000000000", "4="+mixed, 1) },
		"version-like attribute 3":       func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "3=X0.00-X0.0", "3=V1.00-R2.1", 1) },
		"another version in attribute 2": func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "2="+h18Version, "2=V100R001C00SPC123", 1) },
		"attribute 2 declared literal":   func(o *h03Object) { o.SanitizedAttributes = append(o.SanitizedAttributes, "2") },
		"attribute 6 literal":            func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "6=0.0", "6=1.0", 1) },
		"literal attribute declared":     func(o *h03Object) { o.SanitizedAttributes = append(o.SanitizedAttributes, "5") },
		"address as unit":                func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "5=0", "5=192.0.2.1", 1) },
		"other gateway alias":            func(o *h03Object) { o.Entry = strings.Replace(o.Entry, h18Model, "EMMA-A01", 1) },
		"unknown family":                 func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "8=HEMS", "8=Other", 1) },
		"repeated key":                   func(o *h03Object) { o.Entry += ";6=0.0" },
		"dropped key":                    func(o *h03Object) { o.Entry = strings.Replace(o.Entry, ";6=0.0", "", 1) },
		"declared attribute missing":     func(o *h03Object) { o.SanitizedAttributes = append(o.SanitizedAttributes, "10") },
		"unknown key":                    func(o *h03Object) { o.Entry += ";7=0" },
		"constant changed":               func(o *h03Object) { o.Entry = strings.TrimSuffix(o.Entry, "9=0") + "9=1" },
	} {
		object := self
		object.SanitizedAttributes = append([]string(nil), self.SanitizedAttributes...)
		mutate(&object)
		if err := h18CheckEntrySanitized(object); err == nil {
			t.Errorf("%s: structural sanitization accepted %q", name, object.Entry)
		}
	}
	for name, mutate := range map[string]func(*h03Object){
		"model with a rating": func(o *h03Object) {
			o.Entry = strings.Replace(o.Entry, "SUN2000-00X-XXX0", h03Joined("SUN2000-", "50", "K-", "AB1"), 1)
		},
		"model with a part number": func(o *h03Object) {
			o.Entry = strings.Replace(o.Entry, "SUN2000-00X-XXX0", h03Joined("SUN2000-", h03CountingDigits(8), "-0"), 1)
		},
		"key 10 literal": func(o *h03Object) {
			o.Entry = strings.Replace(o.Entry, "10=00XXXX00", "10="+h03Joined("12", "AB", "CD", "34"), 1)
		},
		"model not declared":        func(o *h03Object) { o.SanitizedAttributes = []string{"2", "3", "4", "6", "10"} },
		"attribute 2 not declared":  func(o *h03Object) { o.SanitizedAttributes = []string{"1", "3", "4", "6", "10"} },
		"attribute 2 gateway token": func(o *h03Object) { o.Entry = strings.Replace(o.Entry, "2="+h18Fill, "2="+h18Version, 1) },
	} {
		object := inverter
		object.SanitizedAttributes = append([]string(nil), inverter.SanitizedAttributes...)
		mutate(&object)
		if err := h18CheckEntrySanitized(object); err == nil {
			t.Errorf("%s: structural sanitization accepted %q", name, object.Entry)
		}
	}

	for name, mutate := range map[string]func(*h03Object){
		"model with a family token": func(o *h03Object) {
			o.Entry = strings.Replace(o.Entry, h18BackupModel, h03Joined("Other", "Guard-00X-X0"), 1)
		},
		"model with a rating": func(o *h03Object) {
			o.Entry = strings.Replace(o.Entry, h18BackupModel, h03Joined("XxxxxXxxxx-", "50", "A-T0"), 1)
		},
		"model not declared": func(o *h03Object) { o.SanitizedAttributes = []string{"2", "3", "4", "6"} },
	} {
		object := backup
		object.SanitizedAttributes = append([]string(nil), backup.SanitizedAttributes...)
		mutate(&object)
		if err := h18CheckEntrySanitized(object); err == nil {
			t.Errorf("backup-box %s: structural sanitization accepted %q", name, object.Entry)
		}
	}

	for name, probe := range map[string]h03Probe{
		"serial block literal":             {Outcome: h03Outcome{Type: "text", Text: mixed, Sanitized: true}},
		"serial not declared":              {Outcome: h03Outcome{Type: "text", Text: "XX0000000000"}},
		"software label of another suffix": {Outcome: h03Outcome{Type: "text", Text: h03Joined("SmartHEMS V", "100", "R", "025", "C00SPC", "132")}},
		"software label declared":          {Outcome: h03Outcome{Type: "text", Text: h18Label + " " + h18Version, Sanitized: true}},
		"residue literal":                  {Outcome: h03Outcome{Type: "text_with_residue", Text: h18ChildModel, Residue: h03Joined(h03CountingDigits(8), "-0"), ResidueAt: 20, Sanitized: true}},
		"residue not declared":             {Outcome: h03Outcome{Type: "text_with_residue", Text: h18ChildModel, Residue: h18ChildResidue, ResidueAt: 20}},
		"residue inside text":              {Outcome: h03Outcome{Type: "text_with_residue", Text: h18ChildModel, Residue: h18ChildResidue, ResidueAt: 5, Sanitized: true}},
		"model with a rating":              {Outcome: h03Outcome{Type: "text_with_residue", Text: h03Joined("SUN2000-", "50", "K-", "AB1"), Residue: h18ChildResidue, ResidueAt: 20, Sanitized: true}},
		"clock words":                      {Outcome: h03Outcome{Type: "words", Words: []uint16{0x6c1d, 0x3a40}}},
		"exception 0":                      {Outcome: h03Outcome{Type: "exception"}},
		"unit-100 style silence":           {Outcome: h03Outcome{Type: "no_response", Attempts: 4}},
		"unknown type":                     {Outcome: h03Outcome{Type: "blob"}},
	} {
		if err := h18CheckProbeSanitized(probe); err == nil {
			t.Errorf("%s: probe sanitization accepted %+v", name, probe.Outcome)
		}
	}
}

func TestH18NewFixturesAreCoveredByTheCommittedShapeScan(t *testing.T) {
	scanned := map[string]bool{}
	for _, path := range h03CommittedHuaweiTestdata(t) {
		for _, name := range h18FixtureNames() {
			if strings.HasSuffix(path, "/"+name) {
				scanned[name] = true
			}
		}
	}
	if len(scanned) != len(h18FixtureNames()) {
		t.Fatalf("the scan walk misses fixtures: %v", scanned)
	}
	for name, text := range map[string]string{
		"clock":   `"observed": "2026-10-25"`,
		"time":    `"observed": "06:17:10"`,
		"serial":  `"text": "` + h03CountingDigits(12) + `"`,
		"address": `"endpoint": "198.51.100.7:502"`,
		"esn":     `"text": "` + h03Joined("A0", "LR", "HG", "RE") + `"`,
	} {
		if len(h03ShapeFindings(text)) == 0 {
			t.Errorf("%s was not found", name)
		}
	}
}

func TestH18RebuiltResponsesHaveTheExactLengths(t *testing.T) {
	f := h18LoadAll(t)
	for _, probe := range f.Gateway.Probes {
		pdu, err := h18ProbePDU(probe)
		if err != nil || len(pdu) != 2+int(probe.Quantity)*2 {
			t.Fatalf("gateway probe %v: %d bytes, %v", probe.Roles, len(pdu), err)
		}
	}
	// A residue that does not fit, or that overlaps the text, cannot be rebuilt.
	for name, mutate := range map[string]func(*h03Probe){
		"residue past the extent":     func(p *h03Probe) { p.Outcome.ResidueAt = 21 },
		"residue on the text":         func(p *h03Probe) { p.Outcome.ResidueAt = len(p.Outcome.Text) },
		"residue before the text end": func(p *h03Probe) { p.Outcome.ResidueAt = 3 },
	} {
		probe := h15ProbeByRole(t, f.Children, "child_model_block")
		mutate(&probe)
		if _, err := h18ProbePDU(probe); err == nil {
			t.Errorf("%s: rebuilt", name)
		}
	}
	// The earlier helper is unchanged for every fixture that sets no offset.
	probe := h15ProbeByRole(t, f.Children, "child_model_block")
	probe.Outcome.ResidueAt = 0
	if pdu, err := h18ProbePDU(probe); err != nil || !bytes.Equal(pdu[2:2+len(h18ChildModel)+1], append([]byte(h18ChildModel), 0)) {
		t.Fatalf("a residue without an offset follows the first NUL: %v %q", err, pdu)
	}
}

// TestH18FixtureChangesTravelThroughTheWireIntoTheCard changes the fixtures
// rather than the decoded input: the response bytes are rebuilt, decoded by the
// independent parser and only then evaluated, so a fixture that disagrees with
// its own wire facts, or an exact tuple that a fixture no longer holds, is seen.
func TestH18FixtureChangesTravelThroughTheWireIntoTheCard(t *testing.T) {
	h18RequireControl(t)
	card := NewHuaweiEMMAObservedQualificationCard()
	count := func(value uint8) func(*h18Fixtures) {
		return func(f *h18Fixtures) { f.Inventory.Pages[0].Objects[0].Count = &value }
	}
	rejected := map[string]func(*h18Fixtures){
		"family label changed": h18SetProbeOutcome(h18GatewaySet, "emma_family_label", h03Outcome{Type: "text", Text: "SmartHEMX"}),
		"model alias changed":  h18SetProbeOutcome(h18GatewaySet, "emma_model", h03Outcome{Type: "text", Text: "EMMA-A01"}),
		"software token below its floor": h18SetProbeOutcome(h18GatewaySet, "emma_software_label",
			h03Outcome{Type: "text", Text: h18Label + " V100R025C00SPC101"}),
		"software token of another branch kept in only one place": h18SetProbeOutcome(h18GatewaySet, "emma_software_label",
			h03Outcome{Type: "text", Text: h18Label + " V100R024C00SPC131"}),
		"inverter unit changed in its entry":  h18ReplaceInEntry(0x8a, ";5=3;", ";5=5;"),
		"inverter model changed in its entry": h18ReplaceInEntry(0x8b, "1=SUN2000-00X-XXX0;", "1=SUN2000-00X-XXX1;"),
		"self entry model changed":            h18ReplaceInEntry(0x88, "1=EMMA-A02;", "1=EMMA-A01;"),
		"backup-box product type changed":     h18ReplaceInEntry(0x89, ";8=BackupBox;", ";8=SUN2000;"),
		"count changed":                       count(3),
		"inverter model block changed": h18SetProbeOutcome(h18ChildrenSet, "child_model_block",
			h03Outcome{Type: "text_with_residue", Text: "SUN2000-00X-XXX1", Residue: h18ChildResidue, ResidueAt: 20, Sanitized: true}),
		"inverter model block of the gateway": h18SetProbeOutcome(h18ChildrenSet, "child_model_block", h03Outcome{Type: "text", Text: h18Label}),
		"basic product changed": func(f *h18Fixtures) {
			f.GatewayBasic.Reply.Objects[1].Text = h18FamilyText
			f.GatewayBasic.Reply.PDUBytes += len(h18FamilyText) - len(h18ProductText)
		},
		"basic vendor changed": func(f *h18Fixtures) {
			f.GatewayBasic.Reply.Objects[0].Text = "HUAWEI"
		},
	}
	for name, mutate := range rejected {
		f := h18LoadAll(t)
		mutate(&f)
		h18RequireRejected(t, name, card.Evaluate(h18Input(t, f)))
	}

	accepted := map[string]func(*h18Fixtures){
		"other residue": h18SetProbeOutcome(h18ChildrenSet, "child_model_block",
			h03Outcome{Type: "text_with_residue", Text: h18ChildModel, Residue: "XXXXXXXXXX", ResidueAt: 20, Sanitized: true}),
		"residue starting directly after the NUL": h18SetProbeOutcome(h18ChildrenSet, "child_model_block",
			h03Outcome{Type: "text_with_residue", Text: h18ChildModel, Residue: h18ChildResidue, Sanitized: true}),
		"other serial-bearing text": h18SetProbeOutcome(h18GatewaySet, "serial_bearing_block", h03Outcome{Type: "text", Text: "XXXXXXXXXXXX", Sanitized: true}),
		"entry in reverse order": h18SetEntry(0x88, func(entry string) string {
			attributes := strings.Split(entry, ";")
			for left, right := 0, len(attributes)-1; left < right; left, right = left+1, right-1 {
				attributes[left], attributes[right] = attributes[right], attributes[left]
			}
			return strings.Join(attributes, ";")
		}),
		"other placeholders in the inverter entries": h18ReplaceInEntry(0x8a, "4=000000000000", "4=XXXXXXXXXXXX"),
		"declared-object field changed": func(f *h18Fixtures) {
			for index := range f.Inventory.Pages {
				f.Inventory.Pages[index].DeclaredObjects = 9
			}
		},
	}
	for name, mutate := range accepted {
		f := h18LoadAll(t)
		mutate(&f)
		if result := card.Evaluate(h18Input(t, f)); !result.IdentityMatched() {
			t.Fatalf("%s: rejected: %#v", name, result)
		}
	}

	// A fixture whose own wire claims are wrong cannot be rebuilt.
	f := h18LoadAll(t)
	f.Inventory.Pages[2].PDUBytes++
	if _, err := h18InventoryPDUs(f.Inventory); err == nil {
		t.Fatal("an inventory fixture with a wrong frame length was rebuilt")
	}
	f = h18LoadAll(t)
	f.GatewayBasic.Reply.PDUBytes--
	if _, err := h03BasicPDU(f.GatewayBasic.Read, f.GatewayBasic.Reply); err == nil {
		t.Fatal("a basic reply fixture with a wrong frame length was rebuilt")
	}
	f = h18LoadAll(t)
	h18SetProbeOutcome(h18GatewaySet, "emma_family_label", h03Outcome{Type: "text", Text: strings.Repeat("S", 30)})(&f)
	if _, err := h18ProbePDU(f.Gateway.Probes[0]); err == nil {
		t.Fatal("a text that leaves no padding was rebuilt")
	}
}
