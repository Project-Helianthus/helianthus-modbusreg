package modbusreg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const h15Residue = "00000000-000"

// h15RequireControl keeps every negative test from passing vacuously: the
// unmodified fixture tuple must be admitted by the same card.
func h15RequireControl(t *testing.T) {
	t.Helper()
	if result := NewHuaweiSDongleObservedQualificationCard().Evaluate(h15ValidInput(t)); !result.IdentityMatched() || !result.QualificationTestReady() {
		t.Fatalf("positive control failed: %#v", result)
	}
}

func h15RequireRejected(t *testing.T, label string, result HuaweiQualificationResult) {
	t.Helper()
	if result.IdentityMatched() || result.SelectedClass() != "" || result.Readiness() != HuaweiQualificationInsufficientEvidence ||
		result.NativeFactCount() != 0 || result.AutomaticRequestCount() != 0 || result.FallbackAttempted() || !result.DefaultDenied() ||
		result.QualificationTestReady() || result.BasicCorroborated() || result.ModelResidueBytes() != 0 {
		t.Fatalf("%s: not rejected as INSUFFICIENT_EVIDENCE with no selection: %#v", label, result)
	}
	if inventory, ok := result.Inventory(); ok || inventory.ChildCount() != 0 {
		t.Fatalf("%s: rejected result retained a child list", label)
	}
}

func h15Block(text, residue string) []byte {
	block := make([]byte, h15ModelExtent)
	copy(block, text)
	if len(text)+1 <= len(block) {
		copy(block[len(text)+1:], residue)
	}
	return block
}

// h15BlockAt builds a model extent from the model text and single bytes placed
// at the given offsets of an otherwise NUL extent.
func h15BlockAt(placed map[int]byte) []byte {
	block := make([]byte, h15ModelExtent)
	copy(block, h15Model)
	for offset, value := range placed {
		block[offset] = value
	}
	return block
}

// h15SetEntryBytes replaces the entry object and keeps the frame arithmetic
// consistent, so that only the entry itself can be the cause of a rejection.
func h15SetEntryBytes(input *HuaweiSDongleObservedQualificationInput, value []byte) {
	objects := append([]HuaweiGatewayInventoryObject(nil), input.Child.Inventory.Objects...)
	objects[1] = HuaweiGatewayInventoryObject{ObjectID: objects[1].ObjectID, Value: value}
	input.Child.Inventory.Objects = objects
	input.Child.Inventory.ResponseBytes = 7 + 3 + 2 + len(value)
}

func h15EntryText(input HuaweiSDongleObservedQualificationInput) string {
	return string(input.Child.Inventory.Objects[1].Value)
}

func h15ReplaceEntry(old, replacement string) func(*HuaweiSDongleObservedQualificationInput) {
	return func(input *HuaweiSDongleObservedQualificationInput) {
		entry := h15EntryText(*input)
		if !strings.Contains(entry, old) {
			panic("entry does not contain " + old)
		}
		h15SetEntryBytes(input, []byte(strings.Replace(entry, old, replacement, 1)))
	}
}

func h15SetBasicObject(id uint8, value []byte) func(*HuaweiSDongleObservedQualificationInput) {
	return func(input *HuaweiSDongleObservedQualificationInput) {
		objects := append([]HuaweiGatewayInventoryObject(nil), input.Child.Basic.Objects...)
		for index := range objects {
			if objects[index].ObjectID == id {
				objects[index].Value = value
			}
		}
		input.Child.Basic = &HuaweiSDongleObservedBasicReply{Objects: objects}
	}
}

func h15DropBasicObject(id uint8) func(*HuaweiSDongleObservedQualificationInput) {
	return func(input *HuaweiSDongleObservedQualificationInput) {
		var kept []HuaweiGatewayInventoryObject
		for _, object := range input.Child.Basic.Objects {
			if object.ObjectID != id {
				kept = append(kept, object)
			}
		}
		input.Child.Basic = &HuaweiSDongleObservedBasicReply{Objects: kept}
	}
}

func h15DuplicateBasicObject(id uint8) func(*HuaweiSDongleObservedQualificationInput) {
	return func(input *HuaweiSDongleObservedQualificationInput) {
		objects := append([]HuaweiGatewayInventoryObject(nil), input.Child.Basic.Objects...)
		for _, object := range input.Child.Basic.Objects {
			if object.ObjectID == id {
				objects = append(objects, object)
			}
		}
		input.Child.Basic = &HuaweiSDongleObservedBasicReply{Objects: objects}
	}
}

func h15WithoutTerminalPadding(mutants []string, text string) []string {
	var kept []string
	for _, mutant := range mutants {
		if trimHuaweiTerminalPadding(mutant) != text {
			kept = append(kept, mutant)
		}
	}
	return kept
}

func h15LeafPaths(kind reflect.Type, prefix string) []string {
	if kind.Kind() != reflect.Struct {
		return []string{prefix}
	}
	var paths []string
	for index := 0; index < kind.NumField(); index++ {
		field := kind.Field(index)
		name := field.Name
		if prefix != "" {
			name = prefix + "." + name
		}
		paths = append(paths, h15LeafPaths(field.Type, name)...)
	}
	return paths
}

// h15Mutation is one change to one input field. Catches names the production
// change that the row detects: removing or weakening the named check makes the
// row's input match.
type h15Mutation struct {
	path    string
	label   string
	catches string
	reason  HuaweiQualificationReason
	mutate  func(*HuaweiSDongleObservedQualificationInput)
}

func h15Mutations() []h15Mutation {
	const (
		gateway = HuaweiQualificationGatewayBehaviourMismatch
		invalid = HuaweiQualificationIdentityInvalid
		basic   = HuaweiQualificationBasicIdentityConflict
		listing = HuaweiQualificationInventoryInvalid
	)
	var rows []h15Mutation
	add := func(path, label, catches string, reason HuaweiQualificationReason, mutate func(*HuaweiSDongleObservedQualificationInput)) {
		rows = append(rows, h15Mutation{path, label, catches, reason, mutate})
	}

	for _, unit := range []uint8{0, 1, 2, 99, 101, 247, 255} {
		unit := unit
		add("Gateway.UnitID", fmt.Sprintf("gateway unit %d", unit), "dropping the gateway unit comparison, or comparing it with the child unit", gateway,
			func(i *HuaweiSDongleObservedQualificationInput) { i.Gateway.UnitID = unit })
	}
	for _, code := range []uint8{0, 2, 3, 4, 11, 255} {
		code := code
		add("Gateway.Basic.ExceptionCode", fmt.Sprintf("basic exception %d", code), "accepting any exception, or an absent read, as the refusal", gateway,
			func(i *HuaweiSDongleObservedQualificationInput) { i.Gateway.Basic.ExceptionCode = code })
	}
	add("Gateway.Basic.Reply", "basic reply instead of the exception", "accepting a successful basic identification at the gateway unit", gateway,
		func(i *HuaweiSDongleObservedQualificationInput) {
			i.Gateway.Basic = HuaweiSDongleObservedBasicOutcome{Reply: i.Child.Basic}
		})
	add("Gateway.Basic.Reply", "basic reply next to the exception code", "ignoring a reply when an exception code is also set", gateway,
		func(i *HuaweiSDongleObservedQualificationInput) { i.Gateway.Basic.Reply = i.Child.Basic })
	add("Gateway.Basic.Reply", "empty basic reply", "treating a non-nil reply with no objects as absent", gateway,
		func(i *HuaweiSDongleObservedQualificationInput) {
			i.Gateway.Basic = HuaweiSDongleObservedBasicOutcome{Reply: &HuaweiSDongleObservedBasicReply{}}
		})

	for _, words := range [][]uint16{nil, {}, {1}, {0, 0}, {65535}} {
		words := words
		add("Gateway.SearchStatus", fmt.Sprintf("search status %v", words), "dropping the complete-search check or its length", gateway,
			func(i *HuaweiSDongleObservedQualificationInput) { i.Gateway.SearchStatus = words })
	}
	// The protocol words take the floor form: the leading word equal and the
	// second at or above the observed one. Another leading word, a missing or
	// further word, or the child's words are refused.
	for _, words := range [][]uint16{nil, {50}, {0, 50}, {5, 0}, {51, 0}, {49, 0}, {50, 0, 0}, {0, 0}, {65535, 0}, {51, 1}} {
		words := words
		add("Gateway.Protocol", fmt.Sprintf("protocol words %v", words), "comparing only the second word, dropping the length check, or accepting the child's words", gateway,
			func(i *HuaweiSDongleObservedQualificationInput) { i.Gateway.Protocol = words })
	}
	for _, state := range []uint16{1, 2, 255, 65535} {
		state := state
		add("Gateway.SearchBefore", fmt.Sprintf("search state %d", state), "dropping the completed-search comparison", gateway,
			func(i *HuaweiSDongleObservedQualificationInput) {
				i.Gateway.SearchBefore = []uint16{4, state, i.Gateway.SearchBefore[2]}
				i.Gateway.SearchAfter = []uint16{4, state, i.Gateway.SearchAfter[2]}
			})
	}
	for _, words := range [][]uint16{nil, {4}, {4, 0}, {4, 0, 7, 0}} {
		words := words
		add("Gateway.SearchBefore", fmt.Sprintf("search length %d", len(words)), "indexing the sequence without a length check", gateway,
			func(i *HuaweiSDongleObservedQualificationInput) {
				i.Gateway.SearchBefore = words
				i.Gateway.SearchAfter = words
			})
	}
	for _, after := range [][]uint16{nil, {4, 0, 8}, {4, 0, 6}, {4, 0, 0}, {4, 0, 65535}, {4, 1, 7}, {4, 0}, {4, 0, 7, 0}} {
		after := after
		add("Gateway.SearchAfter", fmt.Sprintf("sequence after %v", after), "comparing only the type word, or comparing before with itself", gateway,
			func(i *HuaweiSDongleObservedQualificationInput) { i.Gateway.SearchAfter = after })
	}
	// A software text, when one was read, is compared with the S-Dongle floors.
	for name, block := range map[string][]byte{
		"block of 29 bytes":        h18Block("V200R022C10SPC312", 30)[:29],
		"block of 31 bytes":        append(h18Block("V200R022C10SPC312", 30), 0),
		"no terminating NUL":       []byte(strings.Repeat("V", 30)),
		"residue after the text":   h18With(h18Block("V200R022C10SPC312", 30), 28, 'Z'),
		"control byte in the text": h18Block("V200R022C10\x01PC312", 30),
		"empty text":               make([]byte, 30),
	} {
		block := block
		add("Gateway.Software", "software block "+name, "decoding the software text without the block rules", gateway,
			func(i *HuaweiSDongleObservedQualificationInput) { i.Gateway.Software = block })
	}
	for _, text := range []string{
		"V200R022C00SPC900", "V200R021C10SPC312", "V200R001C00", "V100R001C00SPC099", "V100R001C00", "V200R025C00SPC12", "v200r022c10spc312",
		"SmartHEMS V100R025C00SPC131", "SmartLogger V300R023C10SPC620", "X000X000X00XXX000",
	} {
		text := text
		add("Gateway.Software", "software text "+text, "admitting a software text below the floor of its domain, of another family or outside the version form", invalid,
			func(i *HuaweiSDongleObservedQualificationInput) { i.Gateway.Software = h18Block(text, 30) })
	}
	for _, capacity := range [][]uint16{nil, {0}, {10, 10}, {}} {
		capacity := capacity
		add("Gateway.Capacity", fmt.Sprintf("capacity %v", capacity), "accepting a capacity that cannot hold the one child", gateway,
			func(i *HuaweiSDongleObservedQualificationInput) { i.Gateway.Capacity = capacity })
	}

	for _, unit := range []uint8{0, 2, 3, 9, 100, 247, 255} {
		unit := unit
		add("Child.UnitID", fmt.Sprintf("child unit %d", unit), "accepting any unit, or the gateway unit, for the child reads", invalid,
			func(i *HuaweiSDongleObservedQualificationInput) { i.Child.UnitID = unit })
	}
	for _, mutant := range h15WithoutTerminalPadding(h03OneCharacterMutants(h15Model), h15Model) {
		mutant := mutant
		add("Child.ModelBlock", fmt.Sprintf("model text %q", mutant), "matching the model by prefix, by case folding or by a shorter text", invalid,
			func(i *HuaweiSDongleObservedQualificationInput) { i.Child.ModelBlock = h15Block(mutant, h15Residue) })
	}
	for name, block := range map[string][]byte{
		"nil block":                    nil,
		"block of 29 bytes":            h15Block(h15Model, h15Residue)[:29],
		"block of 31 bytes":            append(h15Block(h15Model, h15Residue), 0),
		"block of 60 bytes":            append(h15Block(h15Model, h15Residue), h15Block(h15Model, h15Residue)...),
		"empty text":                   h15Block("", h15Residue),
		"no terminating NUL":           []byte(strings.Repeat("S", h15ModelExtent)),
		"model fills the extent":       []byte(h15Model + strings.Repeat("A", h15ModelExtent-len(h15Model))),
		"control byte in the residue":  h15Block(h15Model, "0000\x01000-000"),
		"byte above ASCII in residue":  h15Block(h15Model, "0000\x80000-000"),
		"delete byte in the residue":   h15Block(h15Model, "0000\x7f000-000"),
		"printable after NUL padding":  h15BlockAt(map[int]byte{28: 'A', 29: 'B'}),
		"second text after the NUL":    h15BlockAt(map[int]byte{16: 'A', 17: 'B', 19: 'C', 20: 'D'}),
		"control byte in the text":     h15Block("SUN2000-8KTL\x01M1", h15Residue),
		"model split by a NUL":         h15Block("SUN2000-8KTL", "-M1"),
		"model starts after a NUL":     append([]byte{0}, h15Block(h15Model, h15Residue)[:h15ModelExtent-1]...),
		"model preceded by a space":    h15Block(" "+h15Model, h15Residue),
		"model with embedded space":    h15Block("SUN2000-8KTL -M1", h15Residue),
		"non-ASCII byte in the text":   h15Block("SUN2000-8KTL-M\xb9", h15Residue),
		"lower-case model":             h15Block(strings.ToLower(h15Model), h15Residue),
		"routed model of the logger":   h15Block("SUN2000-50KTL-M3", h15Residue),
		"model of the gateway family":  h15Block("S-DongleA-05", h15Residue),
		"logger label":                 h15Block("Smart Logger", h15Residue),
		"family only":                  h15Block("SUN2000", h15Residue),
		"model followed by a variant":  h15Block("SUN2000-8KTL-M12", h15Residue),
		"model with a trailing hyphen": h15Block("SUN2000-8KTL-M1-", h15Residue),
	} {
		block := block
		add("Child.ModelBlock", name, "accepting a block of another extent, an unterminated text or non-printable residue", invalid,
			func(i *HuaweiSDongleObservedQualificationInput) { i.Child.ModelBlock = block })
	}

	add("Child.Basic", "no basic reply", "treating the child basic identification as optional", invalid,
		func(i *HuaweiSDongleObservedQualificationInput) { i.Child.Basic = nil })
	for _, mutant := range h15WithoutTerminalPadding(h03OneCharacterMutants(h15VendorText), h15VendorText) {
		mutant := mutant
		add("Child.Basic", fmt.Sprintf("vendor %q", mutant), "comparing the vendor by case folding or prefix", basic, h15SetBasicObject(0x00, []byte(mutant)))
	}
	for _, mutant := range []string{"HUAWEI", "huawei", "Huawei Technologies"} {
		mutant := mutant
		add("Child.Basic", fmt.Sprintf("vendor %q", mutant), "accepting the logger's vendor case", basic, h15SetBasicObject(0x00, []byte(mutant)))
	}
	for _, mutant := range h15WithoutTerminalPadding(h03OneCharacterMutants(h15FamilyText), h15FamilyText) {
		mutant := mutant
		add("Child.Basic", fmt.Sprintf("product %q", mutant), "comparing the family by prefix", basic, h15SetBasicObject(0x01, []byte(mutant)))
	}
	for _, mutant := range []string{h15Model, "Smart Logger", "S-DongleA-05", "EMMA-A02"} {
		mutant := mutant
		add("Child.Basic", fmt.Sprintf("product %q", mutant), "accepting the model or another family as the product", basic, h15SetBasicObject(0x01, []byte(mutant)))
	}
	for name, value := range map[string][]byte{"empty": {}, "spaces": []byte("    "), "NUL padding": make([]byte, 17), "non-ASCII": []byte("X000X000X00XXX00\xff")} {
		value := value
		add("Child.Basic", "version object "+name, "requiring no version object, or reading a non-ASCII one", basic, h15SetBasicObject(0x02, value))
	}
	for _, id := range []uint8{0x00, 0x01, 0x02} {
		add("Child.Basic", fmt.Sprintf("object %02x dropped", id), "defaulting an absent identity object", basic, h15DropBasicObject(id))
		add("Child.Basic", fmt.Sprintf("object %02x repeated", id), "taking the first of two identity objects", basic, h15DuplicateBasicObject(id))
	}
	add("Child.Basic", "empty reply", "accepting a reply with no objects", basic,
		func(i *HuaweiSDongleObservedQualificationInput) { i.Child.Basic = &HuaweiSDongleObservedBasicReply{} })

	for _, code := range []uint8{0, 1, 2, 4, 255} {
		code := code
		add("Child.Inventory.ReadDeviceIDCode", fmt.Sprintf("read code %d", code), "accepting a response to another read code", listing,
			func(i *HuaweiSDongleObservedQualificationInput) { i.Child.Inventory.ReadDeviceIDCode = code })
	}
	for _, conformity := range []uint8{0, 1, 2, 0x81, 0x83, 255} {
		conformity := conformity
		add("Child.Inventory.Conformity", fmt.Sprintf("conformity %#x", conformity), "not comparing the conformity level", listing,
			func(i *HuaweiSDongleObservedQualificationInput) { i.Child.Inventory.Conformity = conformity })
	}
	add("Child.Inventory.More", "continuation set", "accepting a first page that continues", listing,
		func(i *HuaweiSDongleObservedQualificationInput) { i.Child.Inventory.More = true })
	for _, delta := range []int{-100, -2, -1, 1, 2, 100} {
		delta := delta
		add("Child.Inventory.ResponseBytes", fmt.Sprintf("response length %+d", delta), "trusting the declared frame length", listing,
			func(i *HuaweiSDongleObservedQualificationInput) { i.Child.Inventory.ResponseBytes += delta })
	}
	for _, declared := range []int{-1, 256, 1 << 20} {
		declared := declared
		add("Child.Inventory.DeclaredObjects", fmt.Sprintf("declared objects %d", declared), "accepting a declared-object field outside one byte", listing,
			func(i *HuaweiSDongleObservedQualificationInput) { i.Child.Inventory.DeclaredObjects = declared })
	}
	add("Child.Inventory.Objects", "no objects", "indexing the objects without a length check", listing,
		func(i *HuaweiSDongleObservedQualificationInput) { i.Child.Inventory.Objects = nil })
	add("Child.Inventory.Objects", "count object only", "accepting a count without its entry", listing,
		func(i *HuaweiSDongleObservedQualificationInput) {
			i.Child.Inventory.Objects = i.Child.Inventory.Objects[:1]
			i.Child.Inventory.ResponseBytes = 10
		})
	add("Child.Inventory.Objects", "entry object only", "accepting an entry without its count", listing,
		func(i *HuaweiSDongleObservedQualificationInput) {
			i.Child.Inventory.Objects = i.Child.Inventory.Objects[1:]
			i.Child.Inventory.ResponseBytes = 7 + 2 + len(i.Child.Inventory.Objects[0].Value)
		})
	add("Child.Inventory.Objects", "second entry", "accepting more than the one entry", listing,
		func(i *HuaweiSDongleObservedQualificationInput) {
			entry := i.Child.Inventory.Objects[1]
			second := HuaweiGatewayInventoryObject{ObjectID: 0x89, Value: bytes.Replace(entry.Value, []byte("5=1"), []byte("5=2"), 1)}
			i.Child.Inventory.Objects = append(i.Child.Inventory.Objects, second)
			i.Child.Inventory.ResponseBytes += 2 + len(second.Value)
		})
	for _, id := range []uint8{0x00, 0x86, 0x88, 0xff} {
		id := id
		add("Child.Inventory.Objects", fmt.Sprintf("count object id %02x", id), "not binding the count to object 0x87", listing,
			func(i *HuaweiSDongleObservedQualificationInput) {
				objects := append([]HuaweiGatewayInventoryObject(nil), i.Child.Inventory.Objects...)
				objects[0].ObjectID = id
				i.Child.Inventory.Objects = objects
			})
	}
	for _, id := range []uint8{0x00, 0x87, 0x89, 0xff} {
		id := id
		add("Child.Inventory.Objects", fmt.Sprintf("entry object id %02x", id), "not binding the entry to object 0x88", listing,
			func(i *HuaweiSDongleObservedQualificationInput) {
				objects := append([]HuaweiGatewayInventoryObject(nil), i.Child.Inventory.Objects...)
				objects[1].ObjectID = id
				i.Child.Inventory.Objects = objects
			})
	}
	for _, count := range [][]byte{nil, {0}, {2}, {12}, {255}, {1, 1}} {
		count := count
		add("Child.Inventory.Objects", fmt.Sprintf("count object value %v", count), "accepting a count other than one entry, which includes the entry itself", listing,
			func(i *HuaweiSDongleObservedQualificationInput) {
				objects := append([]HuaweiGatewayInventoryObject(nil), i.Child.Inventory.Objects...)
				i.Child.Inventory.ResponseBytes += len(count) - len(objects[0].Value)
				objects[0].Value = count
				i.Child.Inventory.Objects = objects
			})
	}
	for _, mutant := range h03OneCharacterMutants(h15Model) {
		mutant := mutant
		add("Child.Inventory.Objects", fmt.Sprintf("entry model %q", mutant), "not requiring the entry model to equal the routed model", invalid,
			h15ReplaceEntry("1="+h15Model+";", "1="+mutant+";"))
	}
	for _, unit := range []string{"0", "2", "3", "100", "247", "01", "+1", "-1", " 1", "1 ", "a", "11", "1.0", "0x1"} {
		unit := unit
		add("Child.Inventory.Objects", fmt.Sprintf("entry unit %q", unit), "not requiring attribute 5 to equal the unit that was read", listing,
			h15ReplaceEntry(";5=1;", ";5="+unit+";"))
	}
	for _, key := range []string{"1", "2", "3", "4", "5", "6"} {
		key := key
		add("Child.Inventory.Objects", fmt.Sprintf("entry without key %s", key), "not requiring the exact key set", listing,
			func(i *HuaweiSDongleObservedQualificationInput) {
				var kept []string
				for _, attribute := range strings.Split(h15EntryText(*i), ";") {
					if !strings.HasPrefix(attribute, key+"=") {
						kept = append(kept, attribute)
					}
				}
				h15SetEntryBytes(i, []byte(strings.Join(kept, ";")))
			})
	}
	for _, swap := range [][2]string{{"6=0.0", "8=SUN2000"}, {"6=0.0", "9=0"}, {"6=0.0", "10=XXXX0XXX"}, {"3=X0.00-X0.0", "9=0"}, {"4=XX0000000000", "8=SUN2000"}, {"2=X000X000X00XXX000", "10=XXXX0XXX"}} {
		swap := swap
		add("Child.Inventory.Objects", fmt.Sprintf("entry with %q in place of %q", swap[1], swap[0]), "checking the key count without the key membership", listing,
			h15ReplaceEntry(swap[0], swap[1]))
	}
	for _, extra := range []string{";6=0.0", ";1=SUN2000-8KTL-M1", ";x=0", ";", ";5", ";07=0", ";256=0"} {
		extra := extra
		add("Child.Inventory.Objects", fmt.Sprintf("entry with %q appended", extra), "accepting repeated keys, keys outside the number grammar or empty attributes", listing,
			func(i *HuaweiSDongleObservedQualificationInput) {
				h15SetEntryBytes(i, append(i.Child.Inventory.Objects[1].Value, extra...))
			})
	}
	add("Child.Inventory.Objects", "valid entry padded to 255 bytes", "applying no entry length bound", listing,
		func(i *HuaweiSDongleObservedQualificationInput) {
			value := make([]byte, 255)
			copy(value, h15EntryText(*i))
			h15SetEntryBytes(i, value)
		})
	add("Child.Inventory.Objects", "a counted second entry at the child unit", "matching the child by its first entry and accepting a repeated unit", listing,
		func(i *HuaweiSDongleObservedQualificationInput) {
			entry := i.Child.Inventory.Objects[1]
			second := HuaweiGatewayInventoryObject{ObjectID: 0x89, Value: append([]byte(nil), entry.Value...)}
			i.Child.Inventory.Objects = append([]HuaweiGatewayInventoryObject{{ObjectID: 0x87, Value: []byte{2}}, entry}, second)
			i.Child.Inventory.ResponseBytes += 2 + len(second.Value)
		})
	add("Child.Inventory.Objects", "a counted second entry without the child unit", "accepting a listing that does not name the unit that was read", listing,
		func(i *HuaweiSDongleObservedQualificationInput) {
			entry := i.Child.Inventory.Objects[1]
			first := HuaweiGatewayInventoryObject{ObjectID: 0x88, Value: bytes.Replace(entry.Value, []byte("5=1"), []byte("5=2"), 1)}
			second := HuaweiGatewayInventoryObject{ObjectID: 0x89, Value: bytes.Replace(entry.Value, []byte("5=1"), []byte("5=3"), 1)}
			i.Child.Inventory.Objects = []HuaweiGatewayInventoryObject{{ObjectID: 0x87, Value: []byte{2}}, first, second}
			i.Child.Inventory.ResponseBytes += 2 + len(second.Value)
		})
	add("Child.Inventory.Objects", "a counted second entry out of object order", "accepting entries out of object order", listing,
		func(i *HuaweiSDongleObservedQualificationInput) {
			entry := i.Child.Inventory.Objects[1]
			second := HuaweiGatewayInventoryObject{ObjectID: 0x8a, Value: bytes.Replace(entry.Value, []byte("5=1"), []byte("5=2"), 1)}
			i.Child.Inventory.Objects = []HuaweiGatewayInventoryObject{{ObjectID: 0x87, Value: []byte{2}}, entry, second}
			i.Child.Inventory.ResponseBytes += 2 + len(second.Value)
		})
	for name, value := range map[string][]byte{
		"empty entry":                 {},
		"spaces only":                 []byte("   "),
		"NUL only":                    make([]byte, 75),
		"control byte":                []byte("1=SUN2000-8KTL-M1;2=X000X000X00XXX000;3=X0.00-X0.0;4=XX0000000000;5=1;6=0.\x01"),
		"embedded NUL":                []byte("1=SUN2000-8KTL-M1;2=X000X000X00XXX000;3=X0.00-X0.0;4=XX0000000000;5=1;\x006=0.0"),
		"non-ASCII byte":              []byte("1=SUN2000-8KTL-M1;2=X000X000X00XXX000;3=X0.00-X0.0;4=XX0000000000;5=1;6=0.\xe9"),
		"empty value":                 []byte("1=SUN2000-8KTL-M1;2=;3=X0.00-X0.0;4=XX0000000000;5=1;6=0.0"),
		"second equals sign":          []byte("1=SUN2000-8KTL-M1;2=X000X000X00XXX000;3=X0.00-X0.0;4=XX0000000000;5=1;6=0=0"),
		"entry above the entry bound": []byte("1=SUN2000-8KTL-M1;2=X000X000X00XXX000;3=X0.00-X0.0;4=XX0000000000;5=1;6=0.0" + strings.Repeat(" ", 200) + ";7"),
	} {
		value := value
		add("Child.Inventory.Objects", name, "decoding the entry with less than the observed-dialect rules", listing,
			func(i *HuaweiSDongleObservedQualificationInput) { h15SetEntryBytes(i, value) })
	}
	return rows
}

const (
	h15VendorText = "Huawei"
	h15FamilyText = "SUN2000"
)

func TestH15ObservedCardIdentityAndReplayProcedure(t *testing.T) {
	card := NewHuaweiSDongleObservedQualificationCard()
	if card.ID() != "huawei.qualification.sdongle.observed-readonly@1.0.0" || card.ID() != HuaweiSDongleObservedQualificationCardID ||
		card.ID() == HuaweiSDongleQualificationCardID || card.CandidateClass() != HuaweiSDongleCanonicalClass ||
		card.UnitID() != 100 || card.ChildUnit() != 1 || card.ChildModel() != h15Model || card.ChildVendor() != h15VendorText ||
		card.ChildFamily() != h15FamilyText || card.GatewayBasicException() != 1 ||
		!reflect.DeepEqual(card.GatewayProtocolWords(), []uint16{50, 0}) ||
		!reflect.DeepEqual(card.MissingEvidence(), []string{
			"sanitized_basic_mei_product_model_fixture", "exact_protocol_version_encoding_fixture", "pairwise_negative_overlap_with_emma_fixture",
		}) {
		t.Fatalf("unexpected observed card: %#v", card)
	}
	// The accessor hands out a copy: changing it cannot change the card.
	words := card.GatewayProtocolWords()
	words[0] = 0
	if !reflect.DeepEqual(card.GatewayProtocolWords(), []uint16{50, 0}) {
		t.Fatal("GatewayProtocolWords exposes the card's own slice")
	}
	if limits := card.Limits(); limits != (HuaweiQualificationInventoryLimits{
		DeadlineMilliseconds: 15000, MaxPages: 1, MaxObjects: 121, MaxBytes: 32768, MaxChildren: 0, MaxResponseBytes: 1024,
	}) {
		t.Fatalf("unexpected observed limits: %#v", limits)
	}
	assertHuaweiQualificationSteps(t, card.Steps(), []HuaweiQualificationStep{
		{operation: "FC2B_MEI_0E_READDEV01_BASIC_GATEWAY_EXPECT_EXCEPTION", unitID: 100, objectID: 0x00},
		{operation: "FC03_GATEWAY_SEARCH_STATUS", unitID: 100, offset: 37411, quantity: 1},
		{operation: "FC03_GATEWAY_PROTOCOL", unitID: 100, offset: 30068, quantity: 2},
		{operation: "FC03_GATEWAY_SEARCH_SEQUENCE_BEFORE", unitID: 100, offset: 37410, quantity: 3},
		{operation: "FC03_GATEWAY_CAPACITY", unitID: 100, offset: 37429, quantity: 1},
		{operation: "FC03_CHILD_MODEL", unitID: 1, offset: 30000, quantity: 15},
		{operation: "FC2B_MEI_0E_READDEV01_BASIC_CHILD", unitID: 1, objectID: 0x00},
		{operation: "FC2B_MEI_0E_READDEV03_INVENTORY_CHILD", unitID: 1, objectID: 0x87},
		{operation: "FC03_GATEWAY_SEARCH_SEQUENCE_AFTER", unitID: 100, offset: 37410, quantity: 3},
	})
	for _, step := range card.Steps() {
		if !strings.HasPrefix(step.Operation(), "FC03_") && !strings.HasPrefix(step.Operation(), "FC2B_MEI_0E_") {
			t.Fatalf("replay step %q is not one of the two read-only operations", step.Operation())
		}
		// Extended inventory is a child read; the gateway unit never receives it.
		if step.UnitID() == 100 && (strings.Contains(step.Operation(), "INVENTORY") || step.ObjectID() == 0x87) {
			t.Fatalf("extended inventory step addressed to the gateway unit: %q", step.Operation())
		}
		if step.UnitID() != 100 && step.UnitID() != 1 {
			t.Fatalf("step %q is addressed to unit %d: no scan, no probe of another unit", step.Operation(), step.UnitID())
		}
	}

	// The retained evidence-blocked card keeps its identity, steps and result.
	legacy := NewHuaweiSDongleQualificationCard()
	if legacy.ID() != "huawei.qualification.sdongle.evidence-blocked@1.0.0" || !legacy.HardStop() || len(legacy.Steps()) != 0 ||
		legacy.Result().Reason() != HuaweiQualificationPersistentNonResponse || len(legacy.Result().MissingEvidence()) != 6 {
		t.Fatalf("evidence-blocked card changed: %#v", legacy)
	}
}

func TestH15ObservedCardAdmitsExactTupleFromFixtures(t *testing.T) {
	card := NewHuaweiSDongleObservedQualificationCard()
	result := card.Evaluate(h15ValidInput(t))
	if !result.IdentityMatched() || result.SelectedClass() != HuaweiSDongleCanonicalClass || result.CandidateClass() != HuaweiSDongleCanonicalClass ||
		result.Readiness() != HuaweiQualificationTestReady || !result.QualificationTestReady() || result.Reason() != HuaweiQualificationMatchedIdentityInventory ||
		!reflect.DeepEqual(result.MissingEvidence(), card.MissingEvidence()) {
		t.Fatalf("exact observed tuple was not admitted: %#v", result)
	}
	// The gateway refuses to name its product, so the match carries no gateway
	// model variant, and the child's model never becomes one.
	if result.ModelVariant() != "" || len(result.CandidateClasses()) != 0 {
		t.Fatalf("a model variant or candidate class was invented: %#v", result)
	}
	if result.HardwareTestReady() || !result.DefaultDenied() || result.CatalogRegistered() || result.AutomaticRuntimeAdmission() || result.LiveQualified() ||
		result.SupportClaim() || result.WriteAuthority() || result.NativeFactCount() != 0 || result.AutomaticRequestCount() != 0 || result.FallbackAttempted() {
		t.Fatalf("admitted result created authority or telemetry: %#v", result)
	}
	if inventory, ok := result.Inventory(); ok || inventory.ChildCount() != 0 {
		t.Fatalf("the child's own entry became a child list: %#v", inventory)
	}
	if !result.BasicCorroborated() || result.BasicOpaqueObjectCount() != 3 || result.ModelResidueBytes() != 12 {
		t.Fatalf("corroboration=%t opaque=%d residue=%d", result.BasicCorroborated(), result.BasicOpaqueObjectCount(), result.ModelResidueBytes())
	}
	if anomalies := result.PageAnomalies(); anomalies != (HuaweiInventoryPageAnomalies{NextObjectWithoutContinuation: 1}) {
		t.Fatalf("page anomalies=%#v", anomalies)
	}
}

func TestH15ObservedReplayMatchesSanitizedExpectedResult(t *testing.T) {
	f := h15LoadAll(t)
	card := NewHuaweiSDongleObservedQualificationCard()
	input := h15Input(t, f)
	result := card.Evaluate(input)
	anomalies := result.PageAnomalies()
	entry := input.Child.Inventory.Objects[1].Value
	padding := len(entry) - len(bytes.TrimRight(entry, "\x00 "))
	actual := map[string]any{
		"schema":         "helianthus-huawei-sdongle-observed-replay-expected/v1",
		"card_id":        card.ID(),
		"status":         result.Readiness(),
		"reason":         result.Reason(),
		"selected_class": result.SelectedClass(),
		"gateway": map[string]any{
			"unit":                 input.Gateway.UnitID,
			"basic_identification": map[string]any{"outcome": "exception", "code": input.Gateway.Basic.ExceptionCode},
			"search": map[string]any{
				"type": input.Gateway.SearchBefore[0], "state": input.Gateway.SearchBefore[1],
				"sequence_equal_before_after": reflect.DeepEqual(input.Gateway.SearchBefore, input.Gateway.SearchAfter),
			},
			"protocol_words":           input.Gateway.Protocol,
			"capacity_covers_children": input.Gateway.Capacity[0] >= 1,
		},
		"child": map[string]any{
			"unit":                      input.Child.UnitID,
			"model":                     card.ChildModel(),
			"model_block_residue_bytes": result.ModelResidueBytes(),
			"basic_identification": map[string]any{
				"vendor": card.ChildVendor(), "product": card.ChildFamily(), "opaque_extra_objects": result.BasicOpaqueObjectCount(),
			},
			"inventory": map[string]any{
				"pages":               1,
				"entries":             len(input.Child.Inventory.Objects) - 1,
				"entry_bytes":         len(entry),
				"entry_padding_bytes": padding,
				"entry_unit":          h03AttributeValue(h15EntryText(input), "5"),
				"retained_child_list": func() bool { _, ok := result.Inventory(); return ok }(),
			},
		},
		"page_anomalies": map[string]any{
			"oversize_responses":               anomalies.OversizeResponses,
			"declared_object_field_differs":    anomalies.DeclaredObjectFieldDiffers,
			"next_object_without_continuation": anomalies.NextObjectWithoutContinuation,
		},
		"first_request_timeout":       map[string]any{"recorded": len(f.Timing.Observations) > 0, "required": f.Timing.RequiredByCard},
		"catalogue_selection":         m86ReplaySelection(result),
		"missing_evidence":            result.MissingEvidence(),
		"hardware_test_ready":         result.HardwareTestReady(),
		"live_qualified":              result.LiveQualified(),
		"automatic_request_count":     result.AutomaticRequestCount(),
		"native_fact_count":           result.NativeFactCount(),
		"default_denied":              result.DefaultDenied(),
		"catalog_registered":          result.CatalogRegistered(),
		"automatic_runtime_admission": result.AutomaticRuntimeAdmission(),
		"support_claim":               result.SupportClaim(),
		"write_authority":             result.WriteAuthority(),
	}
	encoded, err := json.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/huawei/qualification/sdongle_observed_replay_expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(encoded, &gotValue); err != nil {
		t.Fatal(err)
	}
	if err := h03Decode(want, &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("replay result=%s\nwant=%s", encoded, want)
	}
}

func TestH15ObservedCardRejectsMutationOfEveryTupleField(t *testing.T) {
	h15RequireControl(t)
	card := NewHuaweiSDongleObservedQualificationCard()
	rows := h15Mutations()

	// Every leaf of the input is either varied by at least one row or listed as
	// retained and never compared. A new input field fails here until it is
	// covered.
	tolerated := map[string]string{"Child.Inventory.NextObjectID": "retained as an anomaly, never compared"}
	covered := map[string]int{}
	for _, row := range rows {
		covered[row.path]++
	}
	var leaves []string
	for _, leaf := range h15LeafPaths(reflect.TypeOf(HuaweiSDongleObservedQualificationInput{}), "") {
		leaves = append(leaves, strings.TrimPrefix(leaf, "."))
	}
	sort.Strings(leaves)
	for _, leaf := range leaves {
		if _, ok := tolerated[leaf]; ok {
			if covered[leaf] != 0 {
				t.Fatalf("%s is tolerated and must not be rejected", leaf)
			}
			continue
		}
		if covered[leaf] == 0 {
			t.Fatalf("input field %s is varied by no row", leaf)
		}
	}
	for path := range covered {
		if !containsString(leaves, path) {
			t.Fatalf("row path %s is not an input field", path)
		}
	}

	for _, row := range rows {
		if row.catches == "" || row.label == "" {
			t.Fatalf("row %s lacks its label or the production mutation it catches", row.path)
		}
		input := h15ValidInput(t)
		row.mutate(&input)
		result := card.Evaluate(input)
		h15RequireRejected(t, row.path+" "+row.label, result)
		if result.Reason() != row.reason {
			t.Fatalf("%s %s: reason=%s want=%s", row.path, row.label, result.Reason(), row.reason)
		}
	}
	if len(rows) < 250 {
		t.Fatalf("only %d mutation rows: the fields are not covered in depth", len(rows))
	}
}

func TestH15ObservedCardAcceptsTheToleratedVariationsOfAnExactTuple(t *testing.T) {
	card := NewHuaweiSDongleObservedQualificationCard()
	control := card.Evaluate(h15ValidInput(t))
	if !control.IdentityMatched() {
		t.Fatal("control did not match")
	}
	accept := func(label string, mutate func(*HuaweiSDongleObservedQualificationInput)) HuaweiQualificationResult {
		t.Helper()
		input := h15ValidInput(t)
		mutate(&input)
		result := card.Evaluate(input)
		if !result.IdentityMatched() || !result.QualificationTestReady() || result.SelectedClass() != HuaweiSDongleCanonicalClass {
			t.Fatalf("%s: tolerated variation was rejected: %#v", label, result)
		}
		return result
	}

	// Capacity is a bound, not a value; the sequence is a counter compared with itself.
	for _, capacity := range []uint16{1, 2, 10, 120, 65535} {
		capacity := capacity
		accept(fmt.Sprintf("capacity %d", capacity), func(i *HuaweiSDongleObservedQualificationInput) { i.Gateway.Capacity = []uint16{capacity} })
	}
	for _, sequence := range []uint16{0, 1, 227, 65535} {
		sequence := sequence
		accept(fmt.Sprintf("sequence %d", sequence), func(i *HuaweiSDongleObservedQualificationInput) {
			i.Gateway.SearchBefore = []uint16{4, 0, sequence}
			i.Gateway.SearchAfter = []uint16{4, 0, sequence}
		})
	}

	// The residue is retained by length and never compared.
	for _, residue := range []string{"", "A", "00000000-000", "ZZZZZZZZZZZZ", "synthetic ~", strings.Repeat("Q", 14)} {
		residue := residue
		result := accept(fmt.Sprintf("residue %q", residue), func(i *HuaweiSDongleObservedQualificationInput) { i.Child.ModelBlock = h15Block(h15Model, residue) })
		if result.ModelResidueBytes() != len(residue) {
			t.Fatalf("residue %q retained as %d bytes", residue, result.ModelResidueBytes())
		}
	}
	// Terminal space padding of the text is padding, as in every Huawei string.
	accept("space-padded model", func(i *HuaweiSDongleObservedQualificationInput) {
		i.Child.ModelBlock = h15Block(h15Model+"   ", h15Residue)
	})

	// The declared-object field and the next identifier are retained, not compared.
	for _, declared := range []int{0, 1, 2, 3, 13, 255} {
		declared := declared
		result := accept(fmt.Sprintf("declared objects %d", declared), func(i *HuaweiSDongleObservedQualificationInput) { i.Child.Inventory.DeclaredObjects = declared })
		if got := result.PageAnomalies().DeclaredObjectFieldDiffers; (declared != 2) != (got == 1) {
			t.Fatalf("declared objects %d counted as %d anomalies", declared, got)
		}
	}
	for _, next := range []uint8{0, 0x87, 0x88, 0xff} {
		next := next
		result := accept(fmt.Sprintf("next object %#x", next), func(i *HuaweiSDongleObservedQualificationInput) { i.Child.Inventory.NextObjectID = next })
		if got := result.PageAnomalies().NextObjectWithoutContinuation; (next != 0) != (got == 1) {
			t.Fatalf("next object %#x counted as %d anomalies", next, got)
		}
	}

	// Entry padding, attribute order and the values of attributes 2, 3, 4 and 6
	// are not predicates.
	entry := "1=SUN2000-8KTL-M1;2=X000X000X00XXX000;3=X0.00-X0.0;4=XX0000000000;5=1;6=0.0"
	for name, value := range map[string]string{
		"NUL padded":                entry + strings.Repeat("\x00", 100),
		"padded to the frame limit": entry + strings.Repeat("\x00", 241-len(entry)),
		"space padded":              entry + strings.Repeat(" ", 100),
		"mixed padding":             entry + "  \x00\x00",
		"reversed order":            "6=0.0;5=1;4=XX0000000000;3=X0.00-X0.0;2=X000X000X00XXX000;1=SUN2000-8KTL-M1",
		"rotated order":             "5=1;6=0.0;1=SUN2000-8KTL-M1;2=X000X000X00XXX000;3=X0.00-X0.0;4=XX0000000000",
		"changed values":            "1=SUN2000-8KTL-M1;2=anything printable;3=v;4=000000000000000000000000;5=1;6=?",
		"serial-like":               "1=SUN2000-8KTL-M1;2=X000X000X00XXX000;3=X0.00-X0.0;4=" + h03CountingDigits(12) + ";5=1;6=0.0",
		"ESN-like mixed":            "1=SUN2000-8KTL-M1;2=X000X000X00XXX000;3=X0.00-X0.0;4=" + h03Joined("AB", "12", "CD", "34", "EF", "56") + ";5=1;6=0.0",
		"firmware-like 2":           "1=SUN2000-8KTL-M1;2=V100R001C00SPC123;3=X0.00-X0.0;4=XX0000000000;5=1;6=0.0",
	} {
		value := value
		result := accept("entry "+name, func(i *HuaweiSDongleObservedQualificationInput) { h15SetEntryBytes(i, []byte(value)) })
		if !reflect.DeepEqual(result.MissingEvidence(), card.MissingEvidence()) {
			t.Fatalf("entry %s changed the missing evidence", name)
		}
	}

	// The type word is the network connection type, a configuration value: it
	// is recorded and never compared, also between the two sequence reads.
	for _, kind := range []uint16{0, 1, 2, 3, 5, 255, 65535} {
		kind := kind
		result := accept(fmt.Sprintf("type word %d", kind), func(i *HuaweiSDongleObservedQualificationInput) {
			i.Gateway.SearchBefore = []uint16{kind, 0, i.Gateway.SearchBefore[2]}
			i.Gateway.SearchAfter = []uint16{kind ^ 1, 0, i.Gateway.SearchAfter[2]}
		})
		if recorded, ok := result.GatewayTypeWord(); !ok || recorded != kind {
			t.Fatalf("type word %d recorded as %d %t", kind, recorded, ok)
		}
	}
	// The protocol words take the floor form: the same leading word and a
	// second word at or above the observed one.
	for _, words := range [][]uint16{{50, 1}, {50, 7}, {50, 65535}} {
		words := words
		result := accept(fmt.Sprintf("protocol words %v", words), func(i *HuaweiSDongleObservedQualificationInput) { i.Gateway.Protocol = words })
		if !reflect.DeepEqual(result.GatewayProtocolWords(), words) {
			t.Fatalf("protocol words %v recorded as %v", words, result.GatewayProtocolWords())
		}
	}
	// A gateway software text, when one was read, is compared with the
	// S-Dongle floors: the V200 line from V200R022C10, the V100R001C00 branch
	// from its SPC100, and a line that no floor documents with grade Hypothesis.
	for text, want := range map[string]HuaweiGatewaySelectionReason{
		"V200R022C10SPC312": HuaweiGatewayAtOrAboveFloor, "V200R025C00SPC120": HuaweiGatewayAtOrAboveFloor, "V200R026C10": HuaweiGatewayAtOrAboveFloor,
		"V100R001C00SPC100": HuaweiGatewayAtOrAboveFloor, "V300R001C00SPC100": HuaweiGatewayNoDocumentedRelation,
	} {
		text, want := text, want
		result := accept("software text "+text, func(i *HuaweiSDongleObservedQualificationInput) { i.Gateway.Software = h18Block(text, 30) })
		selection, ok := result.Selection()
		if !ok || selection.Reason() != want || result.SoftwareGrade() != HuaweiGradeHypothesis || selection.Build() != text {
			t.Fatalf("software text %s: selection %+v grade %s", text, selection, result.SoftwareGrade())
		}
	}
	if selection, ok := control.Selection(); !ok || selection.Outcome() != HuaweiGatewayAmbiguous || selection.Reason() != HuaweiGatewaySoftwareNotRead ||
		control.SoftwareGrade() != HuaweiGradeUnknown || len(selection.Candidates()) != 2 {
		t.Fatalf("observed tuple without a software read: %+v grade %s", selection, control.SoftwareGrade())
	}
	// The observed keys 1 through 6 are a lower bound: a further key is
	// retained as opaque native evidence.
	for _, extra := range []string{";7=0", ";8=SUN2000", ";9=0", ";10=XXXX0XXX", ";8=SUN2000;9=0", ";255=x"} {
		extra := extra
		accept(fmt.Sprintf("entry with %q appended", extra), func(i *HuaweiSDongleObservedQualificationInput) {
			h15SetEntryBytes(i, append(append([]byte(nil), i.Child.Inventory.Objects[1].Value...), extra...))
		})
	}
	// The observed 87-byte response within the classic frame is a lower bound:
	// an entry up to the 254-byte entry bound, whose response exceeds the frame,
	// is admitted within the resource cap and counted as oversize.
	for _, padTo := range []int{241, 242, 243, 250, 254} {
		padTo := padTo
		result := accept(fmt.Sprintf("entry padded to %d bytes", padTo), func(i *HuaweiSDongleObservedQualificationInput) {
			value := make([]byte, padTo)
			copy(value, h15EntryText(*i))
			h15SetEntryBytes(i, value)
		})
		if got, want := result.PageAnomalies().OversizeResponses, map[bool]int{true: 1, false: 0}[7+3+2+padTo > 253]; got != want {
			t.Fatalf("entry padded to %d: oversize %d want %d", padTo, got, want)
		}
	}
	// The observed count of one entry is a lower bound: a counted further entry
	// at another unit is listing evidence and the child's entry still matches.
	for _, unit := range []string{"2", "3", "247"} {
		unit := unit
		accept("a counted second entry at unit "+unit, func(i *HuaweiSDongleObservedQualificationInput) {
			entry := i.Child.Inventory.Objects[1]
			second := HuaweiGatewayInventoryObject{ObjectID: 0x89, Value: bytes.Replace(entry.Value, []byte("5=1"), []byte("5="+unit), 1)}
			i.Child.Inventory.Objects = []HuaweiGatewayInventoryObject{{ObjectID: 0x87, Value: []byte{2}}, entry, second}
			i.Child.Inventory.ResponseBytes += 2 + len(second.Value)
		})
	}

	// Objects beyond the basic category are counted and never read; the version
	// object is any printable text.
	result := accept("extra and changed basic objects", func(i *HuaweiSDongleObservedQualificationInput) {
		objects := append([]HuaweiGatewayInventoryObject(nil), i.Child.Basic.Objects...)
		objects = append(objects, HuaweiGatewayInventoryObject{ObjectID: 0x19, Value: []byte{9, 9}}, HuaweiGatewayInventoryObject{ObjectID: 0x10, Value: []byte("anything")})
		objects[2].Value = []byte("any printable text ")
		i.Child.Basic = &HuaweiSDongleObservedBasicReply{Objects: objects}
	})
	if result.BasicOpaqueObjectCount() != 5 {
		t.Fatalf("opaque objects=%d want 5", result.BasicOpaqueObjectCount())
	}
	// Terminal padding of the vendor and product objects is padding.
	accept("padded basic objects", func(i *HuaweiSDongleObservedQualificationInput) {
		h15SetBasicObject(0x00, []byte("Huawei\x00\x00"))(i)
		h15SetBasicObject(0x01, []byte("SUN2000  "))(i)
	})
}

func TestH15ObservedCardOutsideAdmittedSetIsInsufficientEvidence(t *testing.T) {
	h15RequireControl(t)
	card := NewHuaweiSDongleObservedQualificationCard()

	// Another model string, even when the routed block, the basic product and the
	// entry all agree with each other.
	for _, model := range []string{
		"SUN2000-50KTL-M3", "SUN2000-8KTL-M2", "SUN2000-8KTL-M0", "SUN2000-10KTL-M1", "SUN2000-8K-M1", "SUN2000", "sun2000-8ktl-m1",
		"S-DongleA-05", "S-DongleB-03", "S-DongleB-06", "Smart Logger", "SmartLogger", "EMMA-A02", "PowerMeter", "PLC",
	} {
		input := h15ValidInput(t)
		input.Child.ModelBlock = h15Block(model, h15Residue)
		h15ReplaceEntry("1="+h15Model+";", "1="+model+";")(&input)
		h15RequireRejected(t, "agreeing model "+model, card.Evaluate(input))

		// The same model in the entry alone, and in the routed block alone.
		entryOnly := h15ValidInput(t)
		h15ReplaceEntry("1="+h15Model+";", "1="+model+";")(&entryOnly)
		h15RequireRejected(t, "entry model "+model, card.Evaluate(entryOnly))
		blockOnly := h15ValidInput(t)
		blockOnly.Child.ModelBlock = h15Block(model, h15Residue)
		h15RequireRejected(t, "routed model "+model, card.Evaluate(blockOnly))
	}

	// Basic identification that succeeds at the gateway unit is outside the
	// tuple, whatever the reply says.
	for name, reply := range map[string]*HuaweiSDongleObservedBasicReply{
		"the child's reply": h15ValidInput(t).Child.Basic,
		"a dongle-like reply": {Objects: []HuaweiGatewayInventoryObject{
			{ObjectID: 0x00, Value: []byte("Huawei")}, {ObjectID: 0x01, Value: []byte("S-DongleA-05")}, {ObjectID: 0x02, Value: []byte("V200R025C00SPC120")},
		}},
		"a logger-like reply": {Objects: []HuaweiGatewayInventoryObject{
			{ObjectID: 0x00, Value: []byte("HUAWEI")}, {ObjectID: 0x01, Value: []byte("Smart Logger")}, {ObjectID: 0x02, Value: []byte("V300R023C10SPC620")},
		}},
		"an empty reply": {},
	} {
		for _, code := range []uint8{0, 1} {
			input := h15ValidInput(t)
			input.Gateway.Basic = HuaweiSDongleObservedBasicOutcome{ExceptionCode: code, Reply: reply}
			result := card.Evaluate(input)
			h15RequireRejected(t, fmt.Sprintf("basic succeeds at the gateway unit: %s, code %d", name, code), result)
			if result.Reason() != HuaweiQualificationGatewayBehaviourMismatch {
				t.Fatalf("%s: reason=%s", name, result.Reason())
			}
		}
	}

	// A child without the extended inventory.
	for name, mutate := range map[string]func(*HuaweiSDongleObservedQualificationInput){
		"zero page": func(i *HuaweiSDongleObservedQualificationInput) {
			i.Child.Inventory = HuaweiSDongleObservedChildInventoryPage{}
		},
		"no objects": func(i *HuaweiSDongleObservedQualificationInput) {
			i.Child.Inventory.Objects = nil
			i.Child.Inventory.ResponseBytes = 7
		},
		"count object only": func(i *HuaweiSDongleObservedQualificationInput) {
			i.Child.Inventory.Objects = i.Child.Inventory.Objects[:1]
			i.Child.Inventory.ResponseBytes = 10
		},
		"entry only": func(i *HuaweiSDongleObservedQualificationInput) {
			i.Child.Inventory.Objects = i.Child.Inventory.Objects[1:]
			i.Child.Inventory.ResponseBytes = 7 + 2 + len(i.Child.Inventory.Objects[0].Value)
		},
	} {
		input := h15ValidInput(t)
		mutate(&input)
		result := card.Evaluate(input)
		h15RequireRejected(t, "child without extended inventory: "+name, result)
		if result.Reason() != HuaweiQualificationInventoryInvalid {
			t.Fatalf("%s: reason=%s", name, result.Reason())
		}
	}
	// Identity reads without any inventory, and the gateway reads alone.
	input := h15ValidInput(t)
	input.Child = HuaweiSDongleObservedChildReads{UnitID: 1, ModelBlock: input.Child.ModelBlock, Basic: input.Child.Basic}
	h15RequireRejected(t, "model and basic reply without inventory", card.Evaluate(input))
	h15RequireRejected(t, "gateway reads alone", card.Evaluate(HuaweiSDongleObservedQualificationInput{Gateway: h15ValidInput(t).Gateway}))
	h15RequireRejected(t, "empty input", card.Evaluate(HuaweiSDongleObservedQualificationInput{}))
	h15RequireRejected(t, "empty result accessor", card.EmptyResult())

	// A SmartLogger-shaped inventory presented at the child.
	logger := h03LoadInventory(t)
	loggerPDUs, err := h03InventoryPDUs(logger)
	if err != nil {
		t.Fatal(err)
	}
	for index, pdu := range loggerPDUs {
		parsed, err := h03ParseMEI(pdu)
		if err != nil {
			t.Fatal(err)
		}
		input := h15ValidInput(t)
		input.Child.Inventory = HuaweiSDongleObservedChildInventoryPage{
			ReadDeviceIDCode: parsed.readDeviceIDCode, Conformity: parsed.conformity, ResponseBytes: parsed.length,
			DeclaredObjects: int(parsed.declared), More: parsed.more == 0xff, NextObjectID: parsed.next, Objects: parsed.objects,
		}
		h15RequireRejected(t, fmt.Sprintf("logger response %d at the child", index), card.Evaluate(input))
	}
	// The logger's self entry in the child's one-page frame, padded as the logger
	// pads. The child's own entry with the logger's further keys 8 and 9 is no
	// longer refused: the observed keys are a lower bound (the tolerance test).
	for name, entry := range map[string]string{
		"self entry padded to 254":           "1=Smart Logger;2=V300R023C10SPC620;3=X0.00-X0.0;4=000000000000;5=1;6=0.0;10=XXXX0XXX;9=0",
		"inverter entry of the logger model": "1=SUN2000-50KTL-M3;2=V200R023C00SPC128;3=X0.00-X0.0;4=0X0000000000;5=1;6=0.0;8=SUN2000;9=0",
	} {
		for _, padTo := range []int{len(entry), 100, 241, 254} {
			input := h15ValidInput(t)
			value := make([]byte, padTo)
			copy(value, entry)
			h15SetEntryBytes(&input, value)
			h15RequireRejected(t, fmt.Sprintf("%s, %d bytes", name, padTo), card.Evaluate(input))
		}
	}
	// And in the other direction: the child's page is not a logger inventory.
	childPage := h15ChildPage(t, h15LoadAll(t).ChildInventory)
	loggerCard := NewHuaweiSmartLoggerObservedQualificationCard()
	for _, unit := range []uint8{0, 1, 100} {
		h03Page := HuaweiSmartLoggerObservedInventoryPage{
			ResponseBytes: childPage.ResponseBytes, DeclaredObjects: childPage.DeclaredObjects, More: childPage.More,
			NextObjectID: childPage.NextObjectID, Objects: childPage.Objects,
		}
		result := loggerCard.Evaluate(HuaweiSmartLoggerObservedQualificationInput{
			UnitID:    unit,
			Inventory: HuaweiSmartLoggerObservedInventoryInput{Pages: []HuaweiSmartLoggerObservedInventoryPage{h03Page}},
		})
		h03RequireRejected(t, fmt.Sprintf("the child's page read as a logger inventory at unit %d", unit), result)
	}
	// The logger's basic reply is not the child's.
	input = h15ValidInput(t)
	input.Child.Basic = &HuaweiSDongleObservedBasicReply{Objects: h03BasicWith(t, func(*h03BasicFixture) {}).Objects}
	h15RequireRejected(t, "the logger's basic reply at the child", card.Evaluate(input))
}

func TestH15SerialEsnReadabilityAndFirmwareNeverSelect(t *testing.T) {
	h15RequireControl(t)
	card := NewHuaweiSDongleObservedQualificationCard()
	reference := card.Evaluate(h15ValidInput(t))

	// Readability of the dongle's own registers at the child is no identity: the
	// child answers the search and capacity reads, and presenting those answers as
	// the gateway's fails on the protocol words and on the unit.
	f := h15LoadAll(t)
	childAsGateway := h15ValidInput(t)
	childAsGateway.Gateway.Protocol = h15Words(t, h15ProbeByRole(t, f.ChildBlocks, "identity_protocol"))
	childAsGateway.Gateway.SearchBefore = h15Words(t, h15ProbeAt(t, f.Overlap, 1, 37410))
	childAsGateway.Gateway.SearchAfter = childAsGateway.Gateway.SearchBefore
	childAsGateway.Gateway.Capacity = h15Words(t, h15ProbeAt(t, f.Overlap, 1, 37429))
	h15RequireRejected(t, "the child's reads presented as the gateway's", card.Evaluate(childAsGateway))
	childAsGateway.Gateway.UnitID = 1
	h15RequireRejected(t, "the child's reads addressed to the child unit", card.Evaluate(childAsGateway))

	// Gateway status reads without a child identity select nothing.
	gatewayOnly := h15ValidInput(t)
	gatewayOnly.Child = HuaweiSDongleObservedChildReads{}
	h15RequireRejected(t, "readable gateway status registers alone", card.Evaluate(gatewayOnly))

	// Serial-bearing and ESN-class values are never inputs and never predicates.
	for _, value := range []string{"A", "000000000000", h03CountingDigits(12), h03Joined("AB", "12", "CD", "34", "EF", "56"), "XX_XXX_XXXXXX", strings.Repeat("Z", 60)} {
		value := value
		input := h15ValidInput(t)
		h15ReplaceEntry("4=XX0000000000", "4="+value)(&input)
		result := card.Evaluate(input)
		if !result.IdentityMatched() || !reflect.DeepEqual(result.MissingEvidence(), reference.MissingEvidence()) || result.PageAnomalies() != reference.PageAnomalies() {
			t.Fatalf("sensitive attribute value %q changed the outcome: %#v", value, result)
		}
	}
	if fields := reflect.TypeOf(HuaweiSDongleObservedChildReads{}); fields.NumField() != 4 {
		t.Fatalf("a new child input must be reviewed against the forbidden-identity list: %d fields", fields.NumField())
	}

	// The child's firmware text and protocol words are retained evidence only:
	// the card has no input for the firmware block, so no value of it can gate.
	for _, name := range []string{"Firmware", "Serial", "Esn", "ESN", "ProtocolWords", "Protocol"} {
		if _, ok := reflect.TypeOf(HuaweiSDongleObservedChildReads{}).FieldByName(name); ok {
			t.Fatalf("the child input carries %s", name)
		}
	}

	// Prefix, wildcard, case and whitespace matches never select the tuple.
	for _, mutant := range []string{"SUN2000-8KTL-M", "SUN2000-8KTL", "SUN2000-8KTL-M1*", "SUN2000-8KTL-M?", "sun2000-8ktl-m1", "SUN2000-8KTL-M 1", "SUN2000 8KTL M1", " SUN2000-8KTL-M1"} {
		input := h15ValidInput(t)
		input.Child.ModelBlock = h15Block(mutant, h15Residue)
		h15RequireRejected(t, "routed model "+mutant, card.Evaluate(input))
		input = h15ValidInput(t)
		h15ReplaceEntry("1="+h15Model+";", "1="+mutant+";")(&input)
		h15RequireRejected(t, "entry model "+mutant, card.Evaluate(input))
	}
}

func TestH15ObservedCardResolvesWithoutWinnerAndSupersedesTheHardStopOnlyByItsOwnClass(t *testing.T) {
	h15RequireControl(t)
	h03RequireControl(t)
	observed := NewHuaweiSDongleObservedQualificationCard().Evaluate(h15ValidInput(t))
	failed := NewHuaweiSDongleObservedQualificationCard().Evaluate(HuaweiSDongleObservedQualificationInput{})
	logger := NewHuaweiSmartLoggerObservedQualificationCard().Evaluate(h03ValidInput(t))
	emma := NewHuaweiEMMAQualificationCard().Evaluate(HuaweiEMMAQualificationInput{
		UnitID: 0, Offering: "SmartHEMS", Model: "EMMA-A02", Firmware: "SmartHEMS V100R025C00SPC131",
	})
	emmaFailed := NewHuaweiEMMAQualificationCard().Evaluate(HuaweiEMMAQualificationInput{})
	blocked := NewHuaweiSDongleQualificationCard().Result()
	if !observed.IdentityMatched() || failed.IdentityMatched() || !logger.IdentityMatched() || !emma.IdentityMatched() {
		t.Fatal("controls are not positive and negative as required")
	}

	// Alone, the observed tuple resolves to its class.
	alone := ResolveHuaweiQualification(observed)
	if alone.SelectedClass() != HuaweiSDongleCanonicalClass || alone.Readiness() != HuaweiQualificationTestReady ||
		!reflect.DeepEqual(alone.CandidateClasses(), []string{HuaweiSDongleCanonicalClass}) || alone.FallbackAttempted() {
		t.Fatalf("single observed result resolved to %#v", alone)
	}
	if failedAlone := ResolveHuaweiQualification(failed); failedAlone.Readiness() != HuaweiQualificationInsufficientEvidence || failedAlone.SelectedClass() != "" {
		t.Fatalf("failed observed result resolved to %#v", failedAlone)
	}

	// Two positives of different classes produce no winner, in every order.
	for name, others := range map[string]HuaweiQualificationResult{"SmartLogger": logger, "EMMA": emma} {
		for _, results := range [][]HuaweiQualificationResult{{observed, others}, {others, observed}, {observed, others, blocked}, {blocked, others, observed}} {
			resolution := ResolveHuaweiQualification(results...)
			want := []string{HuaweiSDongleCanonicalClass, others.SelectedClass()}
			sort.Strings(want)
			if resolution.Readiness() != HuaweiQualificationAmbiguous || resolution.Reason() != HuaweiQualificationMultipleClasses ||
				resolution.SelectedClass() != "" || resolution.IdentityMatched() || !reflect.DeepEqual(resolution.CandidateClasses(), want) ||
				resolution.NativeFactCount() != 0 || resolution.AutomaticRequestCount() != 0 || resolution.FallbackAttempted() {
				t.Fatalf("%s next to the S-Dongle tuple produced a winner: %#v", name, resolution)
			}
		}
	}

	// One positive next to a failed result of another class stays insufficient.
	if mixed := ResolveHuaweiQualification(observed, emmaFailed); mixed.Readiness() != HuaweiQualificationInsufficientEvidence || mixed.SelectedClass() != "" || mixed.FallbackAttempted() {
		t.Fatalf("positive next to a failed other class selected: %#v", mixed)
	}

	// The recorded hard stop is superseded for identity by a match of its own
	// class, in every order, and by nothing else.
	for index, results := range [][]HuaweiQualificationResult{{observed, blocked}, {blocked, observed}} {
		resolution := ResolveHuaweiQualification(results...)
		if resolution.SelectedClass() != HuaweiSDongleCanonicalClass || resolution.Readiness() != HuaweiQualificationTestReady ||
			resolution.Reason() != HuaweiQualificationMatchedIdentityInventory || !reflect.DeepEqual(resolution.MissingEvidence(), observed.MissingEvidence()) {
			t.Fatalf("order %d: the observed tuple did not supersede the hard stop: %#v", index, resolution)
		}
	}
	for name, results := range map[string][]HuaweiQualificationResult{
		"failed observed card":                {failed, blocked},
		"failed observed card first":          {blocked, failed},
		"SmartLogger match":                   {logger, blocked},
		"EMMA match":                          {emma, blocked},
		"SmartLogger and EMMA matches":        {logger, emma, blocked},
		"failed observed card and a SL match": {failed, logger, blocked},
	} {
		resolution := ResolveHuaweiQualification(results...)
		if resolution.Readiness() != HuaweiQualificationEvidenceBlocked || resolution.Reason() != HuaweiQualificationPersistentNonResponse ||
			!reflect.DeepEqual(resolution.MissingEvidence(), blocked.MissingEvidence()) || resolution.SelectedClass() != "" ||
			len(resolution.CandidateClasses()) != 0 || resolution.NativeFactCount() != 0 || resolution.FallbackAttempted() {
			t.Fatalf("%s: the hard stop was lost: %#v", name, resolution)
		}
	}

	// Two positive results of the S-Dongle class would be two tuples at once.
	second := observed
	second.modelResidueBytes = 0
	if contradiction := ResolveHuaweiQualification(observed, second); contradiction.Readiness() != HuaweiQualificationAmbiguous || contradiction.SelectedClass() != "" {
		t.Fatalf("two positives of one class selected: %#v", contradiction)
	}
}

func TestH15ObservedCardCarriesNoAuthority(t *testing.T) {
	card := NewHuaweiSDongleObservedQualificationCard()
	results := []HuaweiQualificationResult{card.EmptyResult(), card.Evaluate(h15ValidInput(t)), card.Evaluate(HuaweiSDongleObservedQualificationInput{})}
	for _, result := range results {
		if !result.DefaultDenied() || result.CatalogRegistered() || result.AutomaticRuntimeAdmission() || result.LiveQualified() ||
			result.SupportClaim() || result.WriteAuthority() || result.NativeFactCount() != 0 || result.AutomaticRequestCount() != 0 ||
			result.FallbackAttempted() || result.HardwareTestReady() {
			t.Fatalf("result created authority or telemetry: %#v", result)
		}
	}
	// Evaluation does not retain or alias the caller's buffers.
	input := h15ValidInput(t)
	before := card.Evaluate(input)
	for index := range input.Child.ModelBlock {
		input.Child.ModelBlock[index] = 'Z'
	}
	for index := range input.Gateway.Protocol {
		input.Gateway.Protocol[index] = 0
	}
	if after := card.Evaluate(h15ValidInput(t)); !reflect.DeepEqual(before, after) {
		t.Fatalf("evaluation depends on shared buffers: %#v %#v", before, after)
	}
	if rejected := card.Evaluate(input); rejected.IdentityMatched() {
		t.Fatalf("a changed buffer still matched: %#v", rejected)
	}
}

func TestH15ChildEntryOpaqueAttributesAreRetained(t *testing.T) {
	card := NewHuaweiSDongleObservedQualificationCard()
	input := h15ValidInput(t)
	if got := card.Evaluate(input).ChildOpaqueAttributes(); len(got) != 0 {
		t.Fatalf("observed entry retained opaque attributes %v", got)
	}
	input = h15ValidInput(t)
	h15SetEntryBytes(&input, append(append([]byte(nil), input.Child.Inventory.Objects[1].Value...), ";11=extra"...))
	result := card.Evaluate(input)
	if !result.IdentityMatched() {
		t.Fatalf("entry with a further key was not admitted: %v", result.Reason())
	}
	got := result.ChildOpaqueAttributes()
	found := false
	for _, attribute := range got {
		if attribute.Key == "11" && attribute.Value == "extra" {
			found = true
		}
	}
	if !found {
		t.Fatalf("opaque attribute 11=extra not retained: %v", got)
	}
}

func TestH15ChildEntryProductTypeIsRetainedAsOpaque(t *testing.T) {
	card := NewHuaweiSDongleObservedQualificationCard()
	input := h15ValidInput(t)
	h15SetEntryBytes(&input, append(append([]byte(nil), input.Child.Inventory.Objects[1].Value...), ";8=SUN2000"...))
	result := card.Evaluate(input)
	if !result.IdentityMatched() {
		t.Fatalf("entry with key 8 was not admitted: %v", result.Reason())
	}
	found := false
	for _, attribute := range result.ChildOpaqueAttributes() {
		if attribute.Key == "8" && attribute.Value == "SUN2000" {
			found = true
		}
	}
	if !found {
		t.Fatalf("attribute 8=SUN2000 not retained: %v", result.ChildOpaqueAttributes())
	}
}
