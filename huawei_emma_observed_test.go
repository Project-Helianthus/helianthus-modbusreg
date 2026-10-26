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

const (
	h18VendorText  = "Huawei"
	h18ProductText = "HEMS"
	h18FamilyText  = "SUN2000"
	h18Software    = h18Label + " " + h18Version
)

// h18RequireControl keeps every negative test from passing vacuously: the
// unmodified fixture tuple must be admitted by the same card.
func h18RequireControl(t *testing.T) {
	t.Helper()
	if result := NewHuaweiEMMAObservedQualificationCard().Evaluate(h18ValidInput(t)); !result.IdentityMatched() || !result.QualificationTestReady() {
		t.Fatalf("positive control failed: %#v", result)
	}
}

func h18RequireRejected(t *testing.T, label string, result HuaweiQualificationResult) {
	t.Helper()
	if result.IdentityMatched() || result.SelectedClass() != "" || result.ModelVariant() != "" || result.Readiness() != HuaweiQualificationInsufficientEvidence ||
		result.NativeFactCount() != 0 || result.AutomaticRequestCount() != 0 || result.FallbackAttempted() || !result.DefaultDenied() ||
		result.QualificationTestReady() || result.BasicCorroborated() || result.BasicOpaqueObjectCount() != 0 || len(result.MissingEvidence()) != 0 {
		t.Fatalf("%s: not rejected as INSUFFICIENT_EVIDENCE with no selection: %#v", label, result)
	}
	if inventory, ok := result.Inventory(); ok || inventory.ChildCount() != 0 {
		t.Fatalf("%s: rejected result retained a child list", label)
	}
}

// h18Block builds a gateway extent: the text, then NUL padding.
func h18Block(text string, extent int) []byte {
	block := make([]byte, extent)
	copy(block, text)
	return block
}

// h18ChildBlock builds an inverter model extent: the text, NUL bytes up to the
// twentieth byte and the residue from there.
func h18ChildBlock(text, residue string) []byte {
	block := make([]byte, h18ChildExtent)
	copy(block, text)
	if len(text) < 20 {
		copy(block[20:], residue)
	} else if len(text)+1 < len(block) {
		copy(block[len(text)+1:], residue)
	}
	return block
}

// h18With copies a block and overwrites the bytes from the offset on.
func h18With(block []byte, offset int, value ...byte) []byte {
	out := append([]byte(nil), block...)
	copy(out[offset:], value)
	return out
}

func h18Object(input *HuaweiEMMAObservedQualificationInput, id uint8) *HuaweiGatewayInventoryObject {
	for pageIndex := range input.Gateway.Inventory {
		page := &input.Gateway.Inventory[pageIndex]
		for objectIndex := range page.Objects {
			if page.Objects[objectIndex].ObjectID == id {
				return &page.Objects[objectIndex]
			}
		}
	}
	panic(fmt.Sprintf("no inventory object %#x", id))
}

// h18FixFrames recomputes the frame length of every page from its objects, so
// that only the changed object can be the cause of a rejection.
func h18FixFrames(input *HuaweiEMMAObservedQualificationInput) {
	for index := range input.Gateway.Inventory {
		page := &input.Gateway.Inventory[index]
		page.ResponseBytes = 7
		for _, object := range page.Objects {
			page.ResponseBytes += 2 + len(object.Value)
		}
	}
}

func h18SetEntryBytes(id uint8, value []byte) func(*HuaweiEMMAObservedQualificationInput) {
	return func(input *HuaweiEMMAObservedQualificationInput) {
		h18Object(input, id).Value = value
		h18FixFrames(input)
	}
}

func h18EntryText(input HuaweiEMMAObservedQualificationInput, id uint8) string {
	return string(h18Object(&input, id).Value)
}

func h18ReplaceEntry(id uint8, old, replacement string) func(*HuaweiEMMAObservedQualificationInput) {
	return func(input *HuaweiEMMAObservedQualificationInput) {
		entry := h18EntryText(*input, id)
		if !strings.Contains(entry, old) {
			panic("entry does not contain " + old)
		}
		h18SetEntryBytes(id, []byte(strings.Replace(entry, old, replacement, 1)))(input)
	}
}

// h18SetCount rewrites the count object.
func h18SetCount(value []byte) func(*HuaweiEMMAObservedQualificationInput) {
	return h18SetEntryBytes(0x87, value)
}

// h18Reflow regroups the objects of the inventory into pages of the given sizes
// and recomputes the frame, continuation and next identifier of every page from
// the objects. The declared-object field keeps the value of the first page.
func h18Reflow(sizes ...int) func(*HuaweiEMMAObservedQualificationInput) {
	return func(input *HuaweiEMMAObservedQualificationInput) {
		declared := input.Gateway.Inventory[0].DeclaredObjects
		var objects []HuaweiGatewayInventoryObject
		for _, page := range input.Gateway.Inventory {
			objects = append(objects, page.Objects...)
		}
		var pages []HuaweiEMMAObservedInventoryPage
		for index, size := range sizes {
			chunk := objects[:size]
			objects = objects[size:]
			last := index == len(sizes)-1
			page := HuaweiEMMAObservedInventoryPage{ReadDeviceIDCode: 3, Conformity: 3, DeclaredObjects: declared, More: !last, Objects: chunk}
			if !last {
				page.NextObjectID = chunk[len(chunk)-1].ObjectID + 1
			}
			pages = append(pages, page)
		}
		if len(objects) != 0 {
			panic("reflow leaves objects")
		}
		input.Gateway.Inventory = pages
		h18FixFrames(input)
	}
}

func h18SetBasicObject(id uint8, value []byte) func(*HuaweiEMMAObservedQualificationInput) {
	return func(input *HuaweiEMMAObservedQualificationInput) {
		objects := append([]HuaweiGatewayInventoryObject(nil), input.Gateway.Basic.Objects...)
		for index := range objects {
			if objects[index].ObjectID == id {
				objects[index].Value = value
			}
		}
		input.Gateway.Basic = &HuaweiEMMAObservedBasicReply{Objects: objects}
	}
}

func h18DropBasicObject(id uint8) func(*HuaweiEMMAObservedQualificationInput) {
	return func(input *HuaweiEMMAObservedQualificationInput) {
		var kept []HuaweiGatewayInventoryObject
		for _, object := range input.Gateway.Basic.Objects {
			if object.ObjectID != id {
				kept = append(kept, object)
			}
		}
		input.Gateway.Basic = &HuaweiEMMAObservedBasicReply{Objects: kept}
	}
}

func h18DuplicateBasicObject(id uint8) func(*HuaweiEMMAObservedQualificationInput) {
	return func(input *HuaweiEMMAObservedQualificationInput) {
		objects := append([]HuaweiGatewayInventoryObject(nil), input.Gateway.Basic.Objects...)
		for _, object := range input.Gateway.Basic.Objects {
			if object.ObjectID == id {
				objects = append(objects, object)
			}
		}
		input.Gateway.Basic = &HuaweiEMMAObservedBasicReply{Objects: objects}
	}
}

// h18SetVersionEverywhere rewrites the software label and the two other places
// that carry its version token, so that a test can change the token everywhere.
func h18SetVersionEverywhere(token string) func(*HuaweiEMMAObservedQualificationInput) {
	return func(input *HuaweiEMMAObservedQualificationInput) {
		input.Gateway.Software = h18Block(h18Label+" "+token, 30)
		h18SetBasicObject(0x02, []byte(token))(input)
		h18ReplaceEntry(0x88, "2="+h18Version+";", "2="+token+";")(input)
	}
}

func h18SetChildBlock(unit uint8, block []byte) func(*HuaweiEMMAObservedQualificationInput) {
	return func(input *HuaweiEMMAObservedQualificationInput) {
		for index := range input.Children {
			if input.Children[index].UnitID == unit {
				input.Children[index].ModelBlock = block
			}
		}
	}
}

// h18LeafPaths lists the leaf fields of the input, descending into structs,
// pointers and slices of structs. A byte slice is one leaf.
func h18LeafPaths(kind reflect.Type, prefix string) []string {
	switch kind.Kind() {
	case reflect.Ptr:
		return h18LeafPaths(kind.Elem(), prefix)
	case reflect.Slice:
		if kind.Elem().Kind() == reflect.Uint8 {
			return []string{prefix}
		}
		return h18LeafPaths(kind.Elem(), prefix+"[]")
	case reflect.Struct:
		var paths []string
		for index := 0; index < kind.NumField(); index++ {
			field := kind.Field(index)
			name := field.Name
			if prefix != "" {
				name = prefix + "." + name
			}
			paths = append(paths, h18LeafPaths(field.Type, name)...)
		}
		return paths
	}
	return []string{prefix}
}

// h18Mutation is one change to one input field. Catches names the production
// change that the row detects: removing or weakening the named check makes the
// row's input match.
type h18Mutation struct {
	path    string
	label   string
	catches string
	reason  HuaweiQualificationReason
	mutate  func(*HuaweiEMMAObservedQualificationInput)
}

func h18ModelFamilyPrefixed(model, family string) bool {
	return strings.HasPrefix(model, family+"-") && len(model) > len(family)+1
}

func h18Mutations() []h18Mutation {
	const (
		invalid = HuaweiQualificationIdentityInvalid
		basic   = HuaweiQualificationBasicIdentityConflict
		listing = HuaweiQualificationInventoryInvalid
	)
	var rows []h18Mutation
	add := func(path, label, catches string, reason HuaweiQualificationReason, mutate func(*HuaweiEMMAObservedQualificationInput)) {
		rows = append(rows, h18Mutation{path, label, catches, reason, mutate})
	}

	// ---- gateway unit and the three FC03 blocks ----
	for _, unit := range []uint8{1, 2, 3, 4, 99, 100, 101, 247, 255} {
		unit := unit
		add("Gateway.UnitID", fmt.Sprintf("gateway unit %d", unit), "dropping the gateway unit comparison, or accepting an inverter unit or the S-Dongle gateway unit as the gateway", invalid,
			func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.UnitID = unit })
	}

	for _, mutant := range h15WithoutTerminalPadding(h03OneCharacterMutants(h18Label), h18Label) {
		mutant := mutant
		add("Gateway.Offering", fmt.Sprintf("family label %q", mutant), "matching the family label by prefix, by case folding or by a shorter text", invalid,
			func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Offering = h18Block(mutant, 30) })
	}
	for name, block := range map[string][]byte{
		"nil block":                   nil,
		"block of 29 bytes":           h18Block(h18Label, 30)[:29],
		"block of 31 bytes":           h18Block(h18Label, 31),
		"block of 60 bytes":           h18Block(h18Label, 60),
		"empty text":                  h18Block("", 30),
		"no terminating NUL":          []byte(strings.Repeat("S", 30)),
		"label fills the extent":      []byte(h18Label + strings.Repeat("A", 30-len(h18Label))),
		"residue after the label":     h18With(h18Block(h18Label, 30), 12, 'X'),
		"second text after the NUL":   h18With(h18Block(h18Label, 30), 14, 'A', 'B'),
		"control byte in the text":    h18Block("SmartHEMS\x01", 30),
		"non-ASCII byte in the text":  h18Block("SmartHEM\xb9", 30),
		"label preceded by a space":   h18Block(" "+h18Label, 30),
		"label starts after a NUL":    append([]byte{0}, h18Block(h18Label, 30)[:29]...),
		"lower-case label":            h18Block(strings.ToLower(h18Label), 30),
		"logger label":                h18Block("Smart Logger", 30),
		"gateway model":               h18Block(h18Model, 30),
		"inverter model":              h18Block(h18ChildModel, 30),
		"S-Dongle model":              h18Block("S-DongleA-05", 30),
		"inverter block as the label": h18ChildBlock(h18ChildModel, h18ChildResidue),
	} {
		block := block
		add("Gateway.Offering", name, "accepting a block of another extent, an unterminated text, residue after the label or another family's block", invalid,
			func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Offering = block })
	}

	for _, mutant := range h15WithoutTerminalPadding(h03OneCharacterMutants(h18Model), h18Model) {
		mutant := mutant
		add("Gateway.Model", fmt.Sprintf("model %q", mutant), "matching the model by prefix, by case folding or by a shorter text", invalid,
			func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Model = h18Block(mutant, 40) })
	}
	for name, block := range map[string][]byte{
		"nil block":                   nil,
		"block of 39 bytes":           h18Block(h18Model, 40)[:39],
		"block of 41 bytes":           h18Block(h18Model, 41),
		"block of 20 bytes":           h18Block(h18Model, 20),
		"empty text":                  h18Block("", 40),
		"no terminating NUL":          []byte(strings.Repeat("E", 40)),
		"residue after the model":     h18With(h18Block(h18Model, 40), 12, 'X'),
		"control byte in the model":   h18Block("EMMA-A0\x01", 40),
		"non-ASCII byte":              h18Block("EMMA-A0\xb2", 40),
		"model preceded by a space":   h18Block(" "+h18Model, 40),
		"documented first alias":      h18Block("EMMA-A01", 40),
		"unobserved alias":            h18Block("EMMA-A03", 40),
		"family only":                 h18Block("EMMA", 40),
		"logger model":                h18Block("SmartLogger", 40),
		"inverter model":              h18Block(h18ChildModel, 40),
		"model followed by a variant": h18Block("EMMA-A02B", 40),
		"lower-case model":            h18Block(strings.ToLower(h18Model), 40),
		"offering block as model":     h18Block(h18Label, 40),
	} {
		block := block
		add("Gateway.Model", name, "accepting a block of another extent, an unterminated text, residue after the model or another alias", invalid,
			func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Model = block })
	}

	for name, block := range map[string][]byte{
		"nil block":                  nil,
		"block of 29 bytes":          h18Block(h18Software, 30)[:29],
		"block of 31 bytes":          h18Block(h18Software, 31),
		"label only":                 h18Block(h18Label, 30),
		"token only":                 h18Block(h18Version, 30),
		"label without the space":    h18Block(h18Label+h18Version, 30),
		"two spaces":                 h18Block(h18Label+"  "+h18Version, 30),
		"tab separator":              h18Block(h18Label+"\t"+h18Version, 30),
		"empty token":                h18Block(h18Label+" ", 30),
		"token with a space":         h18Block(h18Label+" X000 X000", 30),
		"token with a slash":         h18Block(h18Label+" X000/X000", 30),
		"token with a control byte":  h18Block(h18Label+" X000\x01", 30),
		"other label":                h18Block("SmartHEMX "+h18Version, 30),
		"lower-case label":           h18Block("smarthems "+h18Version, 30),
		"label preceded by a space":  h18Block(" "+h18Software, 30),
		"logger text":                h18Block("Smart Logger V300R023C10", 30),
		"residue after the label":    h18With(h18Block(h18Software, 30), 28, 'Z'),
		"text without a NUL":         []byte(h18Software + "ABC"),
		"inverter firmware block":    h18Block("X000X000X00", 30),
		"gateway model as the label": h18Block(h18Model, 30),
	} {
		block := block
		add("Gateway.Software", name, "accepting a software label that is not the family label, a space and a version-style token", invalid,
			func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Software = block })
	}
	// Tokens that the software floors admit, in the observed branch, in the
	// other documented branch at or above its floor, and in a branch or line that
	// no floor documents: alone they disagree with the basic reply.
	for _, token := range []string{
		"V100R025C00SPC132", "V100R025C00SPC102", "V100R025C00SPC999", "V100R024C00SPC100", "V100R024C00SPC555", "V100R026C00SPC200",
		"V100R025C01SPC131", "V200R025C00SPC131",
	} {
		token := token
		add("Gateway.Software", fmt.Sprintf("token %q alone", token), "not requiring the software label token to equal the version object of the basic reply", basic,
			func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Software = h18Block(h18Label+" "+token, 30) })
	}
	// Tokens below the floor of their own branch, or outside the version form.
	for _, token := range []string{
		"V100R025C00SPC101", "V100R025C00SPC000", "V100R024C00SPC099", "V100R025C00SPC13", "V100R025C00",
		"V100R025C00SPC1311", "V100R025C00SPC13A", "v100r025c00spc131", h18Fill, h18Fill[:16], "A", "V1.0", "",
	} {
		token := token
		add("Gateway.Software", fmt.Sprintf("token %q alone", token), "applying no branch rule or floor to the software label token", invalid,
			func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Software = h18Block(h18Label+" "+token, 30) })
	}

	// ---- basic identification ----
	add("Gateway.Basic.Objects[].Value", "no basic reply", "treating the gateway basic identification as optional", invalid,
		func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Basic = nil })
	for _, mutant := range h15WithoutTerminalPadding(h03OneCharacterMutants(h18VendorText), h18VendorText) {
		mutant := mutant
		add("Gateway.Basic.Objects[].Value", fmt.Sprintf("vendor %q", mutant), "comparing the vendor by case folding or prefix", basic, h18SetBasicObject(0x00, []byte(mutant)))
	}
	for _, mutant := range []string{"HUAWEI", "huawei", "Huawei Technologies"} {
		mutant := mutant
		add("Gateway.Basic.Objects[].Value", fmt.Sprintf("vendor %q", mutant), "accepting the logger's vendor case", basic, h18SetBasicObject(0x00, []byte(mutant)))
	}
	for _, mutant := range h15WithoutTerminalPadding(h03OneCharacterMutants(h18ProductText), h18ProductText) {
		mutant := mutant
		add("Gateway.Basic.Objects[].Value", fmt.Sprintf("product %q", mutant), "comparing the product by prefix", basic, h18SetBasicObject(0x01, []byte(mutant)))
	}
	for _, mutant := range []string{h18Model, h18Label, "Smart Logger", "S-DongleA-05", h18FamilyText} {
		mutant := mutant
		add("Gateway.Basic.Objects[].Value", fmt.Sprintf("product %q", mutant), "accepting a model or another family as the product", basic, h18SetBasicObject(0x01, []byte(mutant)))
	}
	for name, value := range map[string][]byte{"empty": {}, "spaces": []byte("    "), "NUL padding": make([]byte, 17), "non-ASCII": []byte("X000X000X00XXX00\xff"), "other token": []byte("X000X000X00XXX001")} {
		value := value
		add("Gateway.Basic.Objects[].Value", "version object "+name, "requiring no version object, reading a non-ASCII one or not comparing it with the software label", basic, h18SetBasicObject(0x02, value))
	}
	for _, id := range []uint8{0x00, 0x01, 0x02} {
		id := id
		add("Gateway.Basic.Objects[].ObjectID", fmt.Sprintf("object %02x dropped", id), "defaulting an absent identity object", basic, h18DropBasicObject(id))
		add("Gateway.Basic.Objects[].ObjectID", fmt.Sprintf("object %02x repeated", id), "taking the first of two identity objects", basic, h18DuplicateBasicObject(id))
		add("Gateway.Basic.Objects[].ObjectID", fmt.Sprintf("object %02x renumbered", id), "reading the identity objects by position instead of by identifier", basic,
			func(i *HuaweiEMMAObservedQualificationInput) {
				objects := append([]HuaweiGatewayInventoryObject(nil), i.Gateway.Basic.Objects...)
				for index := range objects {
					if objects[index].ObjectID == id {
						objects[index].ObjectID = 0x30 + id
					}
				}
				i.Gateway.Basic = &HuaweiEMMAObservedBasicReply{Objects: objects}
			})
	}
	add("Gateway.Basic.Objects[].ObjectID", "empty reply", "accepting a reply with no objects", basic,
		func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Basic = &HuaweiEMMAObservedBasicReply{} })

	// ---- extended inventory: the page header and frame fields ----
	add("Gateway.Inventory[].ReadDeviceIDCode", "no pages", "indexing the pages without a length check", listing,
		func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Inventory = nil })
	for _, code := range []uint8{0, 1, 2, 4, 255} {
		code := code
		for _, page := range []int{0, 2, 4} {
			page := page
			add("Gateway.Inventory[].ReadDeviceIDCode", fmt.Sprintf("read code %d on page %d", code, page), "accepting a response to another read code, on any page", listing,
				func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Inventory[page].ReadDeviceIDCode = code })
		}
	}
	for _, conformity := range []uint8{0, 1, 2, 0x81, 0x83, 255} {
		conformity := conformity
		for _, page := range []int{0, 3} {
			page := page
			add("Gateway.Inventory[].Conformity", fmt.Sprintf("conformity %#x on page %d", conformity, page), "not comparing the conformity level on every page", listing,
				func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Inventory[page].Conformity = conformity })
		}
	}
	for page := 0; page < 4; page++ {
		page := page
		add("Gateway.Inventory[].More", fmt.Sprintf("continuation cleared on page %d", page), "accepting an inventory that stops before its last page", listing,
			func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Inventory[page].More = false })
	}
	add("Gateway.Inventory[].More", "continuation set on the last page", "accepting a last page that continues", listing,
		func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Inventory[4].More = true })
	for page := 0; page < 5; page++ {
		page := page
		for _, delta := range []int{-100, -2, -1, 1, 2, 100} {
			delta := delta
			add("Gateway.Inventory[].ResponseBytes", fmt.Sprintf("response length %+d on page %d", delta, page), "trusting the declared frame length", listing,
				func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Inventory[page].ResponseBytes += delta })
		}
	}
	for _, declared := range []int{-1, 256, 1 << 20} {
		declared := declared
		add("Gateway.Inventory[].DeclaredObjects", fmt.Sprintf("declared objects %d", declared), "accepting a declared-object field outside one byte", listing,
			func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Inventory[2].DeclaredObjects = declared })
	}
	for page, nexts := range map[int][]uint8{0: {0x00, 0x87, 0x89, 0xff}, 2: {0x00, 0x89, 0x8b, 0xff}, 3: {0x00, 0x8a, 0x8c, 0xff}} {
		for _, next := range nexts {
			page, next := page, next
			add("Gateway.Inventory[].NextObjectID", fmt.Sprintf("next object %#x on page %d", next, page), "not binding the next identifier to the object that follows", listing,
				func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Inventory[page].NextObjectID = next })
		}
	}
	for _, next := range []uint8{0x01, 0x87, 0x8b, 0x8c, 0xff} {
		next := next
		add("Gateway.Inventory[].NextObjectID", fmt.Sprintf("next object %#x on the last page", next), "accepting a next identifier next to a last page", listing,
			func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Inventory[4].NextObjectID = next })
	}
	add("Gateway.Inventory[].ResponseBytes", "a sixth page with an uncounted entry", "accepting an entry that the count object does not count, or an entry without a unit", listing,
		func(i *HuaweiEMMAObservedQualificationInput) {
			h18Reflow(1, 1, 1, 1, 1)(i)
			last := &i.Gateway.Inventory[4]
			last.More, last.NextObjectID = true, 0x8c
			i.Gateway.Inventory = append(i.Gateway.Inventory, HuaweiEMMAObservedInventoryPage{ReadDeviceIDCode: 3, Conformity: 3, ResponseBytes: 7 + 2 + 3, DeclaredObjects: 5,
				Objects: []HuaweiGatewayInventoryObject{{ObjectID: 0x8c, Value: []byte("1=A")}}})
		})
	add("Gateway.Inventory[].ResponseBytes", "pages in reverse order", "ordering the pages by arrival instead of by the cursor", listing,
		func(i *HuaweiEMMAObservedQualificationInput) {
			pages := i.Gateway.Inventory
			pages[0], pages[4] = pages[4], pages[0]
			pages[1], pages[3] = pages[3], pages[1]
		})

	// ---- extended inventory: objects ----
	add("Gateway.Inventory[].Objects[].ObjectID", "a page without objects", "accepting an empty page", listing,
		func(i *HuaweiEMMAObservedQualificationInput) {
			i.Gateway.Inventory[1].Objects = nil
			h18FixFrames(i)
		})
	for _, id := range []uint8{0x00, 0x86, 0x88, 0xff} {
		id := id
		add("Gateway.Inventory[].Objects[].ObjectID", fmt.Sprintf("count object id %02x", id), "not binding the count to object 0x87", listing,
			func(i *HuaweiEMMAObservedQualificationInput) { h18Object(i, 0x87).ObjectID = id })
	}
	for _, id := range []uint8{0x00, 0x87, 0x89, 0xff} {
		id := id
		add("Gateway.Inventory[].Objects[].ObjectID", fmt.Sprintf("self entry object id %02x", id), "not binding the self entry to object 0x88", listing,
			func(i *HuaweiEMMAObservedQualificationInput) { h18Object(i, 0x88).ObjectID = id })
	}
	for _, count := range [][]byte{nil, {0}, {1}, {2}, {3}, {5}, {12}, {255}, {4, 4}} {
		count := count
		add("Gateway.Inventory[].Objects[].Value", fmt.Sprintf("count object value %v", count), "accepting a count other than four entries, which includes the gateway's own", listing, h18SetCount(count))
	}
	add("Gateway.Inventory[].Objects[].Value", "an entry object dropped", "accepting a count that exceeds the entries present", listing,
		func(i *HuaweiEMMAObservedQualificationInput) {
			i.Gateway.Inventory = i.Gateway.Inventory[:4]
			last := &i.Gateway.Inventory[3]
			last.More, last.NextObjectID = false, 0
		})
	add("Gateway.Inventory[].Objects[].Value", "inventory without children", "accepting the gateway's own entry as a complete inventory", listing,
		func(i *HuaweiEMMAObservedQualificationInput) {
			h18SetCount([]byte{1})(i)
			i.Gateway.Inventory = i.Gateway.Inventory[:2]
			last := &i.Gateway.Inventory[1]
			last.More, last.NextObjectID = false, 0
		})
	add("Gateway.Inventory[].Objects[].Value", "one inverter only", "accepting an inventory with one inverter unit", listing,
		func(i *HuaweiEMMAObservedQualificationInput) {
			h18SetCount([]byte{3})(i)
			i.Gateway.Inventory = i.Gateway.Inventory[:4]
			last := &i.Gateway.Inventory[3]
			last.More, last.NextObjectID = false, 0
		})
	add("Gateway.Inventory[].Objects[].Value", "backup-box entry dropped", "accepting an inventory without the entry of unit 2", listing,
		func(i *HuaweiEMMAObservedQualificationInput) {
			h18SetCount([]byte{3})(i)
			h18Object(i, 0x8a).ObjectID, h18Object(i, 0x8b).ObjectID = 0x89, 0x8a
			i.Gateway.Inventory = append(i.Gateway.Inventory[:2], i.Gateway.Inventory[3:]...)
			i.Gateway.Inventory[2].NextObjectID = 0x8a
		})
	add("Gateway.Inventory[].Objects[].Value", "a fifth entry at an observed unit", "matching an observed member by its first listing and accepting a repeated unit", listing,
		func(i *HuaweiEMMAObservedQualificationInput) {
			h18SetCount([]byte{5})(i)
			extra := h18Object(i, 0x8b).Value
			last := &i.Gateway.Inventory[4]
			last.Objects = append(last.Objects, HuaweiGatewayInventoryObject{ObjectID: 0x8c, Value: extra})
			h18FixFrames(i)
		})
	add("Gateway.Inventory[].Objects[].Value", "inverter entries swapped", "ignoring the order of the entries", listing,
		func(i *HuaweiEMMAObservedQualificationInput) {
			a, b := h18Object(i, 0x8a), h18Object(i, 0x8b)
			a.Value, b.Value = b.Value, a.Value
			h18FixFrames(i)
		})
	add("Gateway.Inventory[].Objects[].Value", "backup-box entry after the inverters", "ignoring the order of the entries", listing,
		func(i *HuaweiEMMAObservedQualificationInput) {
			a, b := h18Object(i, 0x89), h18Object(i, 0x8b)
			a.Value, b.Value = b.Value, a.Value
			h18FixFrames(i)
		})

	// self entry
	for _, mutant := range h03OneCharacterMutants(h18Model) {
		mutant := mutant
		add("Gateway.Inventory[].Objects[].Value", fmt.Sprintf("self entry model %q", mutant), "not requiring the self entry model to equal the model block", invalid,
			h18ReplaceEntry(0x88, "1="+h18Model+";", "1="+mutant+";"))
	}
	for _, unit := range []string{"1", "2", "3", "4", "100", "247", "00", "+0", " 0", "0 ", "a", "0x0", "-0"} {
		unit := unit
		add("Gateway.Inventory[].Objects[].Value", fmt.Sprintf("self entry unit %q", unit), "not requiring the self entry to name the gateway unit", listing,
			h18ReplaceEntry(0x88, ";5=0;", ";5="+unit+";"))
	}
	for _, mutant := range h03OneCharacterMutants(h18ProductText) {
		mutant := mutant
		add("Gateway.Inventory[].Objects[].Value", fmt.Sprintf("self entry product type %q", mutant), "not requiring the self entry product type to equal the basic product", invalid,
			h18ReplaceEntry(0x88, ";8="+h18ProductText+";", ";8="+mutant+";"))
	}
	for _, mutant := range []string{"V100R025C00SPC132", "V100R025C00SPC13", "X000X000X00XXX000", "v100r025c00spc131", "V100R024C00SPC131"} {
		mutant := mutant
		add("Gateway.Inventory[].Objects[].Value", fmt.Sprintf("self entry version %q", mutant), "not requiring the self entry version to equal the software label token", invalid,
			h18ReplaceEntry(0x88, "2="+h18Version+";", "2="+mutant+";"))
	}

	// the unit-2 entry
	for _, mutant := range h03OneCharacterMutants("BackupBox") {
		mutant := mutant
		add("Gateway.Inventory[].Objects[].Value", fmt.Sprintf("unit 2 product type %q", mutant), "not requiring the product type of the backup-box entry", listing,
			h18ReplaceEntry(0x89, ";8=BackupBox;", ";8="+mutant+";"))
	}
	for _, family := range []string{h18FamilyText, h18ProductText} {
		family := family
		add("Gateway.Inventory[].Objects[].Value", fmt.Sprintf("unit 2 product type %q", family), "accepting a third inverter or a second gateway in place of the backup box", listing,
			h18ReplaceEntry(0x89, ";8=BackupBox;", ";8="+family+";"))
	}
	for _, unit := range []string{"0", "1", "3", "4", "5", "100", "247", "02", "+2", " 2", "2 ", "a", "0x2", "-2"} {
		unit := unit
		add("Gateway.Inventory[].Objects[].Value", fmt.Sprintf("unit 2 entry unit %q", unit), "not binding the backup-box entry to unit 2", listing,
			h18ReplaceEntry(0x89, ";5=2;", ";5="+unit+";"))
	}
	add("Gateway.Inventory[].Objects[].Value", "unit 2 entry without a model", "defaulting an absent model of the backup-box entry", listing,
		h18ReplaceEntry(0x89, "1="+h18BackupModel+";", ""))
	add("Gateway.Inventory[].Objects[].Value", "unit 2 entry with an empty model", "accepting an empty model of the backup-box entry", listing,
		h18ReplaceEntry(0x89, "1="+h18BackupModel+";", "1=;"))

	// the inverter entries
	for _, id := range []uint8{0x8a, 0x8b} {
		id := id
		unit := map[uint8]string{0x8a: "3", 0x8b: "4"}[id]
		model := "SUN2000-00X-XXX0"
		for _, mutant := range h03OneCharacterMutants(model) {
			mutant := mutant
			reason := listing
			if h18ModelFamilyPrefixed(mutant, h18FamilyText) {
				reason = invalid
			}
			add("Gateway.Inventory[].Objects[].Value", fmt.Sprintf("unit %s entry model %q", unit, mutant), "not requiring the entry model to equal the routed model, or its family token", reason,
				h18ReplaceEntry(id, "1="+model+";", "1="+mutant+";"))
		}
		for _, mutant := range h03OneCharacterMutants(h18FamilyText) {
			mutant := mutant
			add("Gateway.Inventory[].Objects[].Value", fmt.Sprintf("unit %s entry product type %q", unit, mutant), "not requiring the SUN2000 product type of an inverter entry", listing,
				h18ReplaceEntry(id, ";8="+h18FamilyText+";", ";8="+mutant+";"))
		}
		for _, other := range []string{"0", "1", "2", "3", "4", "5", "100", "247", "03", "+3", " 3", "3 ", "a", "0x3", "-3"} {
			other := other
			if other == unit {
				continue
			}
			add("Gateway.Inventory[].Objects[].Value", fmt.Sprintf("unit %s entry unit %q", unit, other), "not binding the inverter entry to its unit", listing,
				h18ReplaceEntry(id, ";5="+unit+";", ";5="+other+";"))
		}
	}
	for _, swap := range [][3]string{
		{"9=0", "1=EMMA-A02", "repeated key 1"}, {"9=0", "x=0", "key that is not a number"}, {"9=0", "07=0", "key with a leading zero"},
	} {
		swap := swap
		add("Gateway.Inventory[].Objects[].Value", "self entry with "+swap[2], "accepting a repeated key or a key outside the number grammar", listing,
			h18ReplaceEntry(0x88, swap[0], swap[1]))
	}
	for _, extra := range []string{";6=0.0", ";1=EMMA-A02", ";x=0", ";", ";5", ";256=0", ";0=0"} {
		extra := extra
		add("Gateway.Inventory[].Objects[].Value", fmt.Sprintf("inverter entry with %q appended", extra), "accepting repeated keys, keys outside the number grammar or empty attributes", listing,
			func(i *HuaweiEMMAObservedQualificationInput) {
				h18SetEntryBytes(0x8a, append(append([]byte(nil), h18Object(i, 0x8a).Value...), extra...))(i)
			})
	}
	for name, value := range map[string][]byte{
		"empty entry":        {},
		"spaces only":        []byte("   "),
		"NUL only":           make([]byte, 79),
		"control byte":       []byte("1=EMMA-A02;2=" + h18Version + ";3=X0.00-X0.0;4=XX0000000000;5=0;6=0.\x01;8=HEMS;9=0"),
		"embedded NUL":       []byte("1=EMMA-A02;2=" + h18Version + ";3=X0.00-X0.0;4=XX0000000000;5=0;\x006=0.0;8=HEMS;9=0"),
		"non-ASCII byte":     []byte("1=EMMA-A02;2=" + h18Version + ";3=X0.00-X0.0;4=XX0000000000;5=0;6=0.\xe9;8=HEMS;9=0"),
		"empty value":        []byte("1=EMMA-A02;2=;3=X0.00-X0.0;4=XX0000000000;5=0;6=0.0;8=HEMS;9=0"),
		"second equals sign": []byte("1=EMMA-A02;2=" + h18Version + ";3=X0.00-X0.0;4=XX0000000000;5=0;6=0=0;8=HEMS;9=0"),
		"no model":           []byte("2=" + h18Version + ";3=X0.00-X0.0;4=XX0000000000;5=0;6=0.0;8=HEMS;9=0"),
		"no unit":            []byte("1=EMMA-A02;2=" + h18Version + ";3=X0.00-X0.0;4=XX0000000000;6=0.0;8=HEMS;9=0"),
		"no product type":    []byte("1=EMMA-A02;2=" + h18Version + ";3=X0.00-X0.0;4=XX0000000000;5=0;6=0.0;9=0"),
		"no version":         []byte("1=EMMA-A02;3=X0.00-X0.0;4=XX0000000000;5=0;6=0.0;8=HEMS;9=0"),
	} {
		value := value
		reason := listing
		if name == "no product type" || name == "no version" {
			reason = invalid
		}
		add("Gateway.Inventory[].Objects[].Value", "self entry: "+name, "decoding the entry with less than the observed-dialect rules, or defaulting an absent product type or version", reason,
			h18SetEntryBytes(0x88, value))
	}

	// ---- inverter reads ----
	add("Children[].UnitID", "no inverter reads", "treating the inverter reads as optional", invalid,
		func(i *HuaweiEMMAObservedQualificationInput) { i.Children = nil })
	add("Children[].UnitID", "one inverter read", "accepting one inverter read of two", invalid,
		func(i *HuaweiEMMAObservedQualificationInput) { i.Children = i.Children[:1] })
	add("Children[].UnitID", "three inverter reads", "accepting an inverter read for a unit that no entry names", invalid,
		func(i *HuaweiEMMAObservedQualificationInput) {
			i.Children = append(i.Children, HuaweiEMMAObservedChildReads{UnitID: 5, ModelBlock: i.Children[0].ModelBlock})
		})
	add("Children[].UnitID", "unit 3 read twice", "keeping the last or the first of two reads of one unit", invalid,
		func(i *HuaweiEMMAObservedQualificationInput) { i.Children[1].UnitID = 3 })
	add("Children[].UnitID", "unit 4 read twice", "keeping the last or the first of two reads of one unit", invalid,
		func(i *HuaweiEMMAObservedQualificationInput) { i.Children[0].UnitID = 4 })
	for index, names := range [][]uint8{{0, 1, 2, 5, 6, 99, 100, 101, 247, 255}, {0, 1, 2, 5, 6, 99, 100, 101, 247, 255}} {
		for _, unit := range names {
			index, unit := index, unit
			add("Children[].UnitID", fmt.Sprintf("read %d addressed to unit %d", index+1, unit), "accepting an inverter read from the gateway unit, the backup-box unit or any unit that the inventory does not name", invalid,
				func(i *HuaweiEMMAObservedQualificationInput) { i.Children[index].UnitID = unit })
		}
	}
	for _, unit := range []uint8{3, 4} {
		unit := unit
		for _, mutant := range h15WithoutTerminalPadding(h03OneCharacterMutants(h18ChildModel), h18ChildModel) {
			mutant := mutant
			add("Children[].ModelBlock", fmt.Sprintf("unit %d model text %q", unit, mutant), "not requiring the routed model to equal the model of the entry of that unit", invalid,
				h18SetChildBlock(unit, h18ChildBlock(mutant, h18ChildResidue)))
		}
		for name, block := range map[string][]byte{
			"nil block":                       nil,
			"block of 29 bytes":               h18ChildBlock(h18ChildModel, h18ChildResidue)[:29],
			"block of 31 bytes":               append(h18ChildBlock(h18ChildModel, h18ChildResidue), 0),
			"block of 60 bytes":               append(h18ChildBlock(h18ChildModel, h18ChildResidue), h18ChildBlock(h18ChildModel, h18ChildResidue)...),
			"empty text":                      h18ChildBlock("", h18ChildResidue),
			"no terminating NUL":              []byte(strings.Repeat("S", 30)),
			"model fills the extent":          []byte(h18ChildModel + strings.Repeat("A", 30-len(h18ChildModel))),
			"control byte in the residue":     h18ChildBlock(h18ChildModel, "0000\x01000-0"),
			"byte above ASCII in the residue": h18ChildBlock(h18ChildModel, "0000\x80000-0"),
			"model preceded by a space":       h18ChildBlock(" "+h18ChildModel, h18ChildResidue),
			"model starts after a NUL":        append([]byte{0}, h18ChildBlock(h18ChildModel, h18ChildResidue)[:29]...),
			"family only":                     h18ChildBlock(h18FamilyText, h18ChildResidue),
			"family with a hyphen only":       h18ChildBlock(h18FamilyText+"-", h18ChildResidue),
			"lower-case model":                h18ChildBlock(strings.ToLower(h18ChildModel), h18ChildResidue),
			"gateway label":                   h18ChildBlock(h18Label, h18ChildResidue),
			"gateway model":                   h18ChildBlock(h18Model, h18ChildResidue),
			"gateway offering extent":         h18Block(h18Label, 30),
			"backup-box model":                h18ChildBlock(h18BackupModel, h18ChildResidue),
			"logger label":                    h18ChildBlock("Smart Logger", h18ChildResidue),
			"S-Dongle model":                  h18ChildBlock("S-DongleA-05", h18ChildResidue),
		} {
			block := block
			add("Children[].ModelBlock", fmt.Sprintf("unit %d %s", unit, name), "accepting a block of another extent, an unterminated text, non-printable residue or another family's block", invalid,
				h18SetChildBlock(unit, block))
		}
	}
	return rows
}

func TestH18ObservedCardIdentityAndReplayProcedure(t *testing.T) {
	card := NewHuaweiEMMAObservedQualificationCard()
	if card.ID() != "huawei.qualification.emma.observed-readonly@1.0.0" || card.ID() != HuaweiEMMAObservedQualificationCardID ||
		card.ID() == HuaweiEMMAQualificationCardID || card.CandidateClass() != HuaweiEMMACanonicalClass ||
		card.UnitID() != 0 || card.FamilyLabel() != h18Label || card.Model() != h18Model || card.Vendor() != h18VendorText ||
		card.Product() != h18ProductText || card.ChildFamily() != h18FamilyText || !reflect.DeepEqual(card.ChildUnits(), []uint8{3, 4}) ||
		!reflect.DeepEqual(card.MissingEvidence(), []string{
			"sanitized_readonly_capability_fixture", "model_specific_capability_fixture", "battery_addressing_behind_gateway_fixture",
		}) {
		t.Fatalf("unexpected observed card: %#v", card)
	}
	// The accessor hands out a copy: changing it cannot change the card.
	units := card.ChildUnits()
	units[0] = 0
	if !reflect.DeepEqual(card.ChildUnits(), []uint8{3, 4}) {
		t.Fatal("ChildUnits exposes the card's own slice")
	}
	// The observed five responses, five objects, three children and classic
	// frame are lower bounds; the limits are the resource policy.
	if limits := card.Limits(); limits != (HuaweiQualificationInventoryLimits{
		DeadlineMilliseconds: 15000, MaxPages: 248, MaxObjects: 248, MaxBytes: 65536, MaxChildren: 247, MaxResponseBytes: 1024,
	}) {
		t.Fatalf("unexpected observed limits: %#v", limits)
	}
	assertHuaweiQualificationSteps(t, card.Steps(), []HuaweiQualificationStep{
		{operation: "FC03_GATEWAY_FAMILY_LABEL", unitID: 0, offset: 30000, quantity: 15},
		{operation: "FC03_GATEWAY_MODEL", unitID: 0, offset: 30222, quantity: 20},
		{operation: "FC03_GATEWAY_SOFTWARE_LABEL", unitID: 0, offset: 30035, quantity: 15},
		{operation: "FC2B_MEI_0E_READDEV01_BASIC_GATEWAY", unitID: 0, objectID: 0x00},
		{operation: "FC2B_MEI_0E_READDEV03_INVENTORY_GATEWAY", unitID: 0, objectID: 0x87},
		{operation: "FC03_CHILD_MODEL", unitID: 3, offset: 30000, quantity: 15},
		{operation: "FC03_CHILD_MODEL", unitID: 4, offset: 30000, quantity: 15},
	})
	for _, step := range card.Steps() {
		if !strings.HasPrefix(step.Operation(), "FC03_") && !strings.HasPrefix(step.Operation(), "FC2B_MEI_0E_") {
			t.Fatalf("replay step %q is not one of the two read-only operations", step.Operation())
		}
		// No step addresses unit 100, the unit of the S-Dongle gateway, or a unit
		// the inventory does not name; the serial-bearing block is not read.
		if step.UnitID() != 0 && step.UnitID() != 3 && step.UnitID() != 4 {
			t.Fatalf("step %q is addressed to unit %d: no scan, no probe of another unit", step.Operation(), step.UnitID())
		}
		if step.Offset() == 30015 {
			t.Fatalf("step %q reads the serial-bearing block", step.Operation())
		}
	}

	// The documentary card keeps its identity, steps and missing evidence.
	documentary := NewHuaweiEMMAQualificationCard()
	if documentary.ID() != "huawei.qualification.emma.readonly@1.0.0" || documentary.UnitID() != 0 ||
		!reflect.DeepEqual(documentary.MissingCapabilityEvidence(), []string{
			"sanitized_readonly_capability_fixture", "negative_overlap_with_smartlogger", "model_specific_capability_fixture",
		}) || len(documentary.Steps()) != 3 {
		t.Fatalf("documentary EMMA card changed: %#v", documentary)
	}
}

func TestH18ObservedCardAdmitsExactTupleFromFixtures(t *testing.T) {
	card := NewHuaweiEMMAObservedQualificationCard()
	result := card.Evaluate(h18ValidInput(t))
	if !result.IdentityMatched() || result.SelectedClass() != HuaweiEMMACanonicalClass || result.CandidateClass() != HuaweiEMMACanonicalClass ||
		result.Readiness() != HuaweiQualificationTestReady || !result.QualificationTestReady() || result.Reason() != HuaweiQualificationMatchedIdentityInventory ||
		!reflect.DeepEqual(result.MissingEvidence(), card.MissingEvidence()) || len(result.CandidateClasses()) != 0 {
		t.Fatalf("exact observed tuple was not admitted: %#v", result)
	}
	// The gateway names its model, so the match carries it as the variant.
	if result.ModelVariant() != h18Model {
		t.Fatalf("model variant=%q", result.ModelVariant())
	}
	if result.HardwareTestReady() || !result.DefaultDenied() || result.CatalogRegistered() || result.AutomaticRuntimeAdmission() || result.LiveQualified() ||
		result.SupportClaim() || result.WriteAuthority() || result.NativeFactCount() != 0 || result.AutomaticRequestCount() != 0 || result.FallbackAttempted() {
		t.Fatalf("admitted result created authority or telemetry: %#v", result)
	}
	// The inventory is native evidence: the three listed children are retained
	// and none of them selects a class or a profile.
	inventory, ok := result.Inventory()
	if !ok || inventory.DeclaredChildren() != 3 || inventory.ChildCount() != 3 || !inventory.DefaultDenied() {
		t.Fatalf("retained inventory: ok=%t %#v", ok, inventory)
	}
	var units []string
	for _, child := range inventory.Children() {
		units = append(units, child.Address())
	}
	if !reflect.DeepEqual(units, []string{"2", "3", "4"}) {
		t.Fatalf("retained units=%v", units)
	}
	if !result.BasicCorroborated() || result.BasicOpaqueObjectCount() != 11 || result.ModelResidueBytes() != 0 {
		t.Fatalf("corroboration=%t opaque=%d residue=%d", result.BasicCorroborated(), result.BasicOpaqueObjectCount(), result.ModelResidueBytes())
	}
	// The declared-object field is 5 on each of the five pages, one object each.
	if anomalies := result.PageAnomalies(); anomalies != (HuaweiInventoryPageAnomalies{DeclaredObjectFieldDiffers: 5}) {
		t.Fatalf("page anomalies=%#v", anomalies)
	}
}

func TestH18ObservedReplayMatchesSanitizedExpectedResult(t *testing.T) {
	f := h18LoadAll(t)
	card := NewHuaweiEMMAObservedQualificationCard()
	input := h18Input(t, f)
	result := card.Evaluate(input)
	anomalies := result.PageAnomalies()
	inventory, retained := result.Inventory()
	var childUnits []string
	for _, child := range inventory.Children() {
		childUnits = append(childUnits, child.Address())
	}
	objects := 0
	for _, page := range input.Gateway.Inventory {
		objects += len(page.Objects)
	}
	var children []any
	for _, read := range input.Children {
		text, residue, ok := parseHuaweiEMMAObservedBlock(read.ModelBlock, h18ChildExtent)
		family, _ := huaweiEMMAObservedModelFamily(text)
		if !ok {
			t.Fatalf("unit %d model block did not parse", read.UnitID)
		}
		children = append(children, map[string]any{"unit": read.UnitID, "family": family, "model_block_residue_bytes": residue})
	}
	documentary := NewHuaweiEMMAQualificationCard().Evaluate(HuaweiEMMAQualificationInput{
		UnitID: input.Gateway.UnitID, Offering: string(input.Gateway.Offering), Model: string(input.Gateway.Model), Firmware: string(input.Gateway.Software),
	})
	actual := map[string]any{
		"schema":         "helianthus-huawei-emma-observed-replay-expected/v1",
		"card_id":        card.ID(),
		"status":         result.Readiness(),
		"reason":         result.Reason(),
		"selected_class": result.SelectedClass(),
		"model_variant":  result.ModelVariant(),
		"gateway": map[string]any{
			"unit":         input.Gateway.UnitID,
			"family_label": strings.TrimRight(string(input.Gateway.Offering), "\x00"),
			"basic_identification": map[string]any{
				"vendor": card.Vendor(), "product": card.Product(), "opaque_extra_objects": result.BasicOpaqueObjectCount(),
				"version_equals_software_token": string(input.Gateway.Basic.Objects[2].Value) == strings.TrimPrefix(strings.TrimRight(string(input.Gateway.Software), "\x00"), h18Label+" "),
			},
			"inventory": map[string]any{
				"pages":            len(input.Gateway.Inventory),
				"objects":          objects,
				"count_object":     int(input.Gateway.Inventory[0].Objects[0].Value[0]),
				"retained":         retained,
				"retained_units":   childUnits,
				"self_entry_unit":  h03AttributeValue(h18EntryText(input, 0x88), "5"),
				"self_entry_model": h03AttributeValue(h18EntryText(input, 0x88), "1"),
			},
		},
		"documentary_card": map[string]any{
			"card_id": NewHuaweiEMMAQualificationCard().ID(), "status": documentary.Readiness(), "identity_matched": documentary.IdentityMatched(), "model_variant": documentary.ModelVariant(),
		},
		"children":                    children,
		"page_anomalies":              map[string]any{"oversize_responses": anomalies.OversizeResponses, "declared_object_field_differs": anomalies.DeclaredObjectFieldDiffers, "next_object_without_continuation": anomalies.NextObjectWithoutContinuation},
		"request_timing":              map[string]any{"recorded": len(f.Timing.Observations) > 0, "required": f.Timing.RequiredByCard, "retries": f.Timing.Retries},
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
	want, err := os.ReadFile("testdata/huawei/qualification/emma_observed_replay_expected.json")
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

func TestH18ObservedCardRejectsMutationOfEveryTupleField(t *testing.T) {
	h18RequireControl(t)
	card := NewHuaweiEMMAObservedQualificationCard()
	rows := h18Mutations()

	// Every leaf of the input is either varied by at least one row or listed as
	// retained and never compared. A new input field fails here until it is
	// covered.
	tolerated := map[string]string{"Gateway.Inventory[].DeclaredObjects": "retained as an anomaly, rejected only outside one byte"}
	covered := map[string]int{}
	for _, row := range rows {
		covered[row.path]++
	}
	leaves := h18LeafPaths(reflect.TypeOf(HuaweiEMMAObservedQualificationInput{}), "")
	sort.Strings(leaves)
	wantLeaves := []string{
		"Children[].ModelBlock", "Children[].UnitID",
		"Gateway.Basic.Objects[].ObjectID", "Gateway.Basic.Objects[].Value",
		"Gateway.Inventory[].Conformity", "Gateway.Inventory[].DeclaredObjects", "Gateway.Inventory[].More", "Gateway.Inventory[].NextObjectID",
		"Gateway.Inventory[].Objects[].ObjectID", "Gateway.Inventory[].Objects[].Value", "Gateway.Inventory[].ReadDeviceIDCode", "Gateway.Inventory[].ResponseBytes",
		"Gateway.Model", "Gateway.Offering", "Gateway.Software", "Gateway.UnitID",
	}
	if !reflect.DeepEqual(leaves, wantLeaves) {
		t.Fatalf("input leaves changed: %v", leaves)
	}
	for _, leaf := range leaves {
		if covered[leaf] == 0 {
			t.Fatalf("input field %s is varied by no row", leaf)
		}
	}
	for path := range covered {
		if !containsString(leaves, path) {
			t.Fatalf("row path %s is not an input field", path)
		}
	}
	for path := range tolerated {
		if covered[path] == 0 {
			t.Fatalf("%s is tolerated only inside its range and still needs its out-of-range rows", path)
		}
	}

	for _, row := range rows {
		if row.catches == "" || row.label == "" {
			t.Fatalf("row %s lacks its label or the production mutation it catches", row.path)
		}
		input := h18ValidInput(t)
		row.mutate(&input)
		result := card.Evaluate(input)
		h18RequireRejected(t, row.path+" "+row.label, result)
		if result.Reason() != row.reason {
			t.Fatalf("%s %s: reason=%s want=%s", row.path, row.label, result.Reason(), row.reason)
		}
	}
	t.Logf("%d mutation rows over %d input fields", len(rows), len(leaves))
	if len(rows) < 500 {
		t.Fatalf("only %d mutation rows: the fields are not covered in depth", len(rows))
	}
}

func TestH18ObservedCardAcceptsTheToleratedVariationsOfAnExactTuple(t *testing.T) {
	card := NewHuaweiEMMAObservedQualificationCard()
	control := card.Evaluate(h18ValidInput(t))
	if !control.IdentityMatched() {
		t.Fatal("control did not match")
	}
	accept := func(label string, mutate func(*HuaweiEMMAObservedQualificationInput)) HuaweiQualificationResult {
		t.Helper()
		input := h18ValidInput(t)
		mutate(&input)
		result := card.Evaluate(input)
		if !result.IdentityMatched() || !result.QualificationTestReady() || result.SelectedClass() != HuaweiEMMACanonicalClass || result.ModelVariant() != h18Model {
			t.Fatalf("%s: tolerated variation was rejected: %#v", label, result)
		}
		if !reflect.DeepEqual(result.MissingEvidence(), card.MissingEvidence()) {
			t.Fatalf("%s: changed the missing evidence", label)
		}
		return result
	}

	// The residue after an inverter's model text is retained by length and never
	// compared; the NUL gap before it is not a predicate either.
	for _, residue := range []string{"", "A", h18ChildResidue, "ZZZZZZZZZZ", "synthetic~", strings.Repeat("Q", 10)} {
		residue := residue
		accept(fmt.Sprintf("residue %q", residue), func(i *HuaweiEMMAObservedQualificationInput) {
			h18SetChildBlock(3, h18ChildBlock(h18ChildModel, residue))(i)
			h18SetChildBlock(4, h18ChildBlock(h18ChildModel, residue))(i)
		})
	}
	for _, block := range [][]byte{
		h18With(h18ChildBlock(h18ChildModel, "00000"), 27, 'A'),
		func() []byte { b := h18ChildBlock(h18ChildModel, h18ChildResidue); b[22] = 0; return b }(),
		append([]byte(h18ChildModel), make([]byte, 30-len(h18ChildModel))...),
		append(append([]byte(h18ChildModel), 0), append([]byte("ABC"), make([]byte, 30-len(h18ChildModel)-4)...)...),
		append(append(append([]byte(h18ChildModel), make([]byte, 6)...), []byte("ABCD")...), make([]byte, 30-len(h18ChildModel)-10)...),
	} {
		block := block
		accept(fmt.Sprintf("model block %q", block), func(i *HuaweiEMMAObservedQualificationInput) { h18SetChildBlock(3, block)(i) })
	}
	// Terminal space padding of a text is padding, as in every Huawei string.
	accept("space-padded texts", func(i *HuaweiEMMAObservedQualificationInput) {
		i.Gateway.Offering = h18Block(h18Label+"   ", 30)
		i.Gateway.Model = h18Block(h18Model+"  ", 40)
		i.Gateway.Software = h18Block(h18Software+" ", 30)
		h18SetChildBlock(4, h18ChildBlock(h18ChildModel+"  ", h18ChildResidue))(i)
	})
	// The two inverter reads may arrive in either order.
	// The observed count, entries and five responses are lower bounds: a further
	// counted entry at another unit, on a sixth response, is retained listing
	// evidence and never read, and the observed members still match.
	for _, unit := range []string{"5", "6", "247"} {
		unit := unit
		result := accept("a counted fifth entry at unit "+unit+" on a sixth response", func(i *HuaweiEMMAObservedQualificationInput) {
			h18SetCount([]byte{5})(i)
			extra := bytes.Replace(h18Object(i, 0x8b).Value, []byte("5=4"), []byte("5="+unit), 1)
			last := &i.Gateway.Inventory[len(i.Gateway.Inventory)-1]
			last.More, last.NextObjectID = true, 0x8c
			i.Gateway.Inventory = append(i.Gateway.Inventory, HuaweiEMMAObservedInventoryPage{
				ReadDeviceIDCode: 3, Conformity: 3, DeclaredObjects: last.DeclaredObjects,
				Objects: []HuaweiGatewayInventoryObject{{ObjectID: 0x8c, Value: extra}},
			})
			h18FixFrames(i)
		})
		if inventory, _ := result.Inventory(); inventory.ChildCount() != 4 || inventory.DeclaredChildren() != 4 {
			t.Fatalf("fifth entry at unit %s: retained %d children", unit, inventory.ChildCount())
		}
	}
	accept("inverter reads in reverse order", func(i *HuaweiEMMAObservedQualificationInput) {
		i.Children[0], i.Children[1] = i.Children[1], i.Children[0]
	})
	// The version-style token is compared with its two other places and with the
	// branch rule and floors of the documentary gate, so any token that meets
	// them and that all three places share is the same tuple.
	for _, token := range []string{"V100R025C00SPC131", "V100R025C00SPC102", "V100R025C00SPC999"} {
		token := token
		accept(fmt.Sprintf("token %q", token), h18SetVersionEverywhere(token))
	}

	// The declared-object field is retained, not compared.
	for _, declared := range []int{0, 1, 2, 3, 5, 13, 255} {
		declared := declared
		result := accept(fmt.Sprintf("declared objects %d", declared), func(i *HuaweiEMMAObservedQualificationInput) {
			for index := range i.Gateway.Inventory {
				i.Gateway.Inventory[index].DeclaredObjects = declared
			}
		})
		want := 5
		if declared == 1 {
			want = 0
		}
		if got := result.PageAnomalies().DeclaredObjectFieldDiffers; got != want {
			t.Fatalf("declared objects %d counted as %d anomalies, want %d", declared, got, want)
		}
	}

	// The page layout is not compared: the same objects in fewer responses are
	// the same inventory. The observed responses stayed within the classic
	// frame; that is a lower bound, and a response above it within the resource
	// cap is admitted.
	oversize := 0
	for _, sizes := range [][]int{{2, 1, 1, 1}, {1, 2, 1, 1}, {2, 2, 1}, {3, 2}, {2, 3}, {5}} {
		sizes := sizes
		input := h18ValidInput(t)
		h18Reflow(sizes...)(&input)
		above := 0
		for _, page := range input.Gateway.Inventory {
			if page.ResponseBytes > 253 {
				above++
			}
			if page.ResponseBytes > 1024 {
				t.Fatalf("layout %v exceeds the resource cap", sizes)
			}
		}
		oversize += above
		result := card.Evaluate(input)
		if !result.IdentityMatched() || result.PageAnomalies().OversizeResponses != above {
			t.Fatalf("layout %v: matched=%t oversize=%d want %d", sizes, result.IdentityMatched(), result.PageAnomalies().OversizeResponses, above)
		}
	}
	if oversize == 0 {
		t.Fatal("no layout exceeded the classic frame")
	}
	if input := h18ValidInput(t); true {
		h18Reflow(2, 2, 1)(&input)
		if result := card.Evaluate(input); !result.IdentityMatched() || result.PageAnomalies().DeclaredObjectFieldDiffers != 3 {
			t.Fatalf("a regrouped inventory: %#v", result)
		}
	}

	// Padding, attribute order and the values of keys 3, 4, 6 and 10 are not
	// predicates; neither are keys 2, 3, 4 and 6 of the entries of units 2 to 4.
	self := "1=EMMA-A02;2=" + h18Version + ";3=X0.00-X0.0;4=XX0000000000;5=0;6=0.0;8=HEMS;9=0"
	for name, value := range map[string]string{
		"NUL padded":     self + strings.Repeat("\x00", 100),
		"space padded":   self + strings.Repeat(" ", 100),
		"mixed padding":  self + "  \x00\x00",
		"reversed order": "9=0;8=HEMS;6=0.0;5=0;4=XX0000000000;3=X0.00-X0.0;2=" + h18Version + ";1=EMMA-A02",
		"rotated order":  "5=0;6=0.0;8=HEMS;9=0;1=EMMA-A02;2=" + h18Version + ";3=X0.00-X0.0;4=XX0000000000",
		"changed values": "1=EMMA-A02;2=" + h18Version + ";3=v;4=" + strings.Repeat("0", 24) + ";5=0;6=?;8=HEMS;9=7",
		"serial-like":    "1=EMMA-A02;2=" + h18Version + ";3=X0.00-X0.0;4=" + h03CountingDigits(12) + ";5=0;6=0.0;8=HEMS;9=0",
		"ESN-like mixed": "1=EMMA-A02;2=" + h18Version + ";3=X0.00-X0.0;4=" + h03Joined("AB", "12", "CD", "34", "EF", "56") + ";5=0;6=0.0;8=HEMS;9=0",
		"opaque key 10":  self + ";10=anything",
		"opaque key 9":   strings.Replace(self, "9=0", "9=anything", 1),
	} {
		value := value
		accept("self entry "+name, h18SetEntryBytes(0x88, []byte(value)))
	}
	for _, id := range []uint8{0x89, 0x8a, 0x8b} {
		id := id
		for _, key := range []string{"2", "3", "4", "6", "10"} {
			key := key
			if key == "10" && id == 0x89 {
				continue
			}
			accept(fmt.Sprintf("object %02x key %s", id, key), func(i *HuaweiEMMAObservedQualificationInput) {
				entry := h18EntryText(*i, id)
				var kept []string
				for _, attribute := range strings.Split(entry, ";") {
					if strings.HasPrefix(attribute, key+"=") {
						attribute = key + "=any printable text"
					}
					kept = append(kept, attribute)
				}
				h18SetEntryBytes(id, []byte(strings.Join(kept, ";")))(i)
			})
		}
	}
	// The model of the backup-box entry is compared by its family token only.
	for _, model := range []string{"anything", "SUN2000-00X-XXX0", "EMMA-A02", "SmartHEMS", "X", h18BackupModel + h18BackupModel} {
		model := model
		accept(fmt.Sprintf("backup-box model %q", model), h18ReplaceEntry(0x89, "1="+h18BackupModel+";", "1="+model+";"))
	}

	// Objects beyond the basic category are counted and never read; the version
	// object is any token that the software label shares.
	result := accept("extra basic objects", func(i *HuaweiEMMAObservedQualificationInput) {
		objects := append([]HuaweiGatewayInventoryObject(nil), i.Gateway.Basic.Objects...)
		objects = append(objects, HuaweiGatewayInventoryObject{ObjectID: 0x1d, Value: []byte{9, 9}}, HuaweiGatewayInventoryObject{ObjectID: 0x30, Value: []byte("anything")})
		i.Gateway.Basic = &HuaweiEMMAObservedBasicReply{Objects: objects}
	})
	if result.BasicOpaqueObjectCount() != 13 {
		t.Fatalf("opaque objects=%d want 13", result.BasicOpaqueObjectCount())
	}
	accept("fewer opaque basic objects", func(i *HuaweiEMMAObservedQualificationInput) {
		i.Gateway.Basic = &HuaweiEMMAObservedBasicReply{Objects: i.Gateway.Basic.Objects[:3]}
	})
	accept("padded basic objects", func(i *HuaweiEMMAObservedQualificationInput) {
		h18SetBasicObject(0x00, []byte("Huawei\x00\x00"))(i)
		h18SetBasicObject(0x01, []byte("HEMS  "))(i)
		h18SetBasicObject(0x02, []byte(h18Version+"\x00"))(i)
	})
	accept("basic objects in reverse order", func(i *HuaweiEMMAObservedQualificationInput) {
		objects := append([]HuaweiGatewayInventoryObject(nil), i.Gateway.Basic.Objects...)
		for left, right := 0, len(objects)-1; left < right; left, right = left+1, right-1 {
			objects[left], objects[right] = objects[right], objects[left]
		}
		i.Gateway.Basic = &HuaweiEMMAObservedBasicReply{Objects: objects}
	})
}

func TestH18ObservedCardOutsideAdmittedSetIsInsufficientEvidence(t *testing.T) {
	h18RequireControl(t)
	card := NewHuaweiEMMAObservedQualificationCard()

	// Another family label, even when every other element agrees with it: the
	// label, the software label and nothing else are changed.
	for _, label := range []string{"SmartLogger", "Smart Logger", "S-Dongle", "SmartHEMX", "smarthems", "SmartHEMS2", "SUN2000", "HEMS", "EMMA"} {
		input := h18ValidInput(t)
		input.Gateway.Offering = h18Block(label, 30)
		input.Gateway.Software = h18Block(label+" "+h18Version, 30)
		h18RequireRejected(t, "family label "+label, card.Evaluate(input))
	}

	// Another model alias, even when the block, the entry and the children
	// agree with each other.
	for _, model := range []string{"EMMA-A01", "EMMA-A03", "EMMA-B02", "SmartLogger", "S-DongleA-05", "SUN2000-00X-XXX0", "EMMA", "emma-a02"} {
		input := h18ValidInput(t)
		input.Gateway.Model = h18Block(model, 40)
		h18ReplaceEntry(0x88, "1="+h18Model+";", "1="+model+";")(&input)
		h18RequireRejected(t, "agreeing model "+model, card.Evaluate(input))

		blockOnly := h18ValidInput(t)
		blockOnly.Gateway.Model = h18Block(model, 40)
		h18RequireRejected(t, "model block "+model, card.Evaluate(blockOnly))
		entryOnly := h18ValidInput(t)
		h18ReplaceEntry(0x88, "1="+h18Model+";", "1="+model+";")(&entryOnly)
		h18RequireRejected(t, "entry model "+model, card.Evaluate(entryOnly))
	}

	// A software token that the documentary gate refuses, even when the label,
	// the basic reply and the entry all carry it: below the floor of its own
	// branch, a placeholder or free text.
	for _, token := range []string{"V100R025C00SPC101", "V100R024C00SPC099", "V100R025C00SPC13", h18Fill, "A"} {
		input := h18ValidInput(t)
		h18SetVersionEverywhere(token)(&input)
		h18RequireRejected(t, "agreeing software token "+token, card.Evaluate(input))
	}
	// The floors admit the other documented branch at or above its floor and a
	// branch that no floor documents, with grade Hypothesis; the observed build
	// alone is Proven.
	for token, reason := range map[string]HuaweiGatewaySelectionReason{
		"V100R024C00SPC100": HuaweiGatewayAtOrAboveFloor, "V100R024C00SPC555": HuaweiGatewayAtOrAboveFloor,
		"V100R025C00SPC132": HuaweiGatewayAtOrAboveFloor, "V100R026C00SPC200": HuaweiGatewayNoDocumentedRelation,
	} {
		input := h18ValidInput(t)
		h18SetVersionEverywhere(token)(&input)
		result := card.Evaluate(input)
		selection, ok := result.Selection()
		if !result.IdentityMatched() || !ok || selection.Reason() != reason || result.SoftwareGrade() != HuaweiGradeHypothesis {
			t.Fatalf("agreeing software token %s: matched=%t selection=%+v grade=%s", token, result.IdentityMatched(), selection, result.SoftwareGrade())
		}
	}
	if result := card.Evaluate(h18ValidInput(t)); result.SoftwareGrade() != HuaweiGradeProven {
		t.Fatalf("observed build graded %s", result.SoftwareGrade())
	}

	// An inventory without children, in every form.
	for name, mutate := range map[string]func(*HuaweiEMMAObservedQualificationInput){
		"count 1 and the self entry": func(i *HuaweiEMMAObservedQualificationInput) {
			h18SetCount([]byte{1})(i)
			i.Gateway.Inventory = i.Gateway.Inventory[:2]
			i.Gateway.Inventory[1].More, i.Gateway.Inventory[1].NextObjectID = false, 0
		},
		"count only": func(i *HuaweiEMMAObservedQualificationInput) {
			i.Gateway.Inventory = i.Gateway.Inventory[:1]
			i.Gateway.Inventory[0].More, i.Gateway.Inventory[0].NextObjectID = false, 0
		},
		"no pages": func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Inventory = nil },
		"count 4 and the self entry": func(i *HuaweiEMMAObservedQualificationInput) {
			i.Gateway.Inventory = i.Gateway.Inventory[:2]
			i.Gateway.Inventory[1].More = false
		},
		"children without the self entry": func(i *HuaweiEMMAObservedQualificationInput) {
			i.Gateway.Inventory = append(i.Gateway.Inventory[:1], i.Gateway.Inventory[2:]...)
		},
		"inverter reads without any inventory": func(i *HuaweiEMMAObservedQualificationInput) { i.Gateway.Inventory = nil },
	} {
		input := h18ValidInput(t)
		mutate(&input)
		result := card.Evaluate(input)
		h18RequireRejected(t, "inventory without children: "+name, result)
		if result.Reason() != HuaweiQualificationInventoryInvalid {
			t.Fatalf("%s: reason=%s", name, result.Reason())
		}
	}
	// The inverter reads and the gateway blocks without any inventory.
	noInventory := h18ValidInput(t)
	noInventory.Gateway.Inventory = nil
	h18RequireRejected(t, "identity reads without inventory", card.Evaluate(noInventory))
	h18RequireRejected(t, "gateway reads alone", card.Evaluate(HuaweiEMMAObservedQualificationInput{Gateway: h18ValidInput(t).Gateway}))
	h18RequireRejected(t, "inverter reads alone", card.Evaluate(HuaweiEMMAObservedQualificationInput{Children: h18ValidInput(t).Children}))
	h18RequireRejected(t, "empty input", card.Evaluate(HuaweiEMMAObservedQualificationInput{}))
	h18RequireRejected(t, "empty result accessor", card.EmptyResult())

	// A SmartLogger-shaped inventory in the place of the gateway's: the logger's
	// own pages, with the logger's own counter, at the unit the logger uses.
	logger := h03LoadInventory(t)
	loggerPDUs, err := h03InventoryPDUs(logger)
	if err != nil {
		t.Fatal(err)
	}
	var loggerPages []HuaweiEMMAObservedInventoryPage
	for _, pdu := range loggerPDUs {
		parsed, err := h03ParseMEI(pdu)
		if err != nil {
			t.Fatal(err)
		}
		loggerPages = append(loggerPages, HuaweiEMMAObservedInventoryPage{
			ReadDeviceIDCode: parsed.readDeviceIDCode, Conformity: parsed.conformity, ResponseBytes: parsed.length,
			DeclaredObjects: int(parsed.declared), More: parsed.more == 0xff, NextObjectID: parsed.next, Objects: parsed.objects,
		})
	}
	input := h18ValidInput(t)
	input.Gateway.Inventory = loggerPages
	h18RequireRejected(t, "the logger's inventory at the gateway unit", card.Evaluate(input))
	for index := range loggerPages {
		input = h18ValidInput(t)
		input.Gateway.Inventory = loggerPages[index : index+1]
		h18RequireRejected(t, fmt.Sprintf("logger response %d alone", index), card.Evaluate(input))
	}
	// The logger's self entry in the gateway's own page layout.
	input = h18ValidInput(t)
	h18SetEntryBytes(0x88, []byte("1=Smart Logger;2="+h18Version+";3=X0.00-X0.0;4=000000000000;5=0;6=0.0;10=XXXX0XXX;9=0"))(&input)
	h18RequireRejected(t, "the logger's model as the self entry", card.Evaluate(input))

	// An S-Dongle child inventory page, and the S-Dongle child's replies.
	dongle := h15LoadAll(t)
	dongleChild := h15ChildPage(t, dongle.ChildInventory)
	input = h18ValidInput(t)
	input.Gateway.Inventory = []HuaweiEMMAObservedInventoryPage{{
		ReadDeviceIDCode: dongleChild.ReadDeviceIDCode, Conformity: dongleChild.Conformity, ResponseBytes: dongleChild.ResponseBytes,
		DeclaredObjects: dongleChild.DeclaredObjects, More: dongleChild.More, NextObjectID: dongleChild.NextObjectID, Objects: dongleChild.Objects,
	}}
	h18RequireRejected(t, "the S-Dongle child's inventory page", card.Evaluate(input))
	input = h18ValidInput(t)
	input.Gateway.Basic = &HuaweiEMMAObservedBasicReply{Objects: h15BasicReply(t, dongle.ChildBasic).Objects}
	if result := card.Evaluate(input); result.IdentityMatched() || result.Reason() != HuaweiQualificationBasicIdentityConflict {
		t.Fatalf("the S-Dongle child's basic reply: %#v", result)
	}
	input = h18ValidInput(t)
	input.Gateway.Basic = &HuaweiEMMAObservedBasicReply{Objects: h03BasicWith(t, func(*h03BasicFixture) {}).Objects}
	if result := card.Evaluate(input); result.IdentityMatched() || result.Reason() != HuaweiQualificationBasicIdentityConflict {
		t.Fatalf("the logger's basic reply: %#v", result)
	}
	// The S-Dongle child's model block has its residue next to the NUL; an EMMA
	// inverter's has a NUL gap. The S-Dongle block is a SUN2000 string, but it is
	// not the model of the entry of the unit it is presented for.
	input = h18ValidInput(t)
	h18SetChildBlock(3, h15Block(h15Model, h15Residue))(&input)
	h18RequireRejected(t, "the S-Dongle child's model block at unit 3", card.Evaluate(input))

	// A child answering as the gateway, and a gateway answering as a child.
	for _, unit := range []uint8{3, 4} {
		input = h18ValidInput(t)
		input.Gateway.UnitID = unit
		h18RequireRejected(t, fmt.Sprintf("gateway reads addressed to inverter unit %d", unit), card.Evaluate(input))

		input = h18ValidInput(t)
		input.Gateway.Offering = h18Block(h18ChildModel, 30)
		input.Gateway.Model = h18Block(h18ChildModel, 40)
		input.Gateway.Software = h18Block("X000X000X00", 30)
		input.Gateway.UnitID = unit
		h18RequireRejected(t, fmt.Sprintf("inverter %d answering as the gateway", unit), card.Evaluate(input))
	}
	input = h18ValidInput(t)
	input.Gateway.Offering = h18ChildBlock(h18ChildModel, h18ChildResidue)
	h18RequireRejected(t, "inverter model block as the family label", card.Evaluate(input))
	input = h18ValidInput(t)
	input.Gateway.Model = append(h18ChildBlock(h18ChildModel, h18ChildResidue), make([]byte, 10)...)
	h18RequireRejected(t, "inverter model block as the model", card.Evaluate(input))
	input = h18ValidInput(t)
	h18SetChildBlock(3, h18Block(h18Label, 30))(&input)
	h18SetChildBlock(4, h18Block(h18Label, 30))(&input)
	h18RequireRejected(t, "gateway label as both inverter reads", card.Evaluate(input))
	input = h18ValidInput(t)
	h18SetChildBlock(3, h18Block(h18Model, 30))(&input)
	h18RequireRejected(t, "gateway model as an inverter read", card.Evaluate(input))
	input = h18ValidInput(t)
	input.Children[0].UnitID, input.Children[1].UnitID = 0, 0
	h18RequireRejected(t, "both inverter reads addressed to the gateway unit", card.Evaluate(input))
	input = h18ValidInput(t)
	input.Children = append(input.Children[:1:1], HuaweiEMMAObservedChildReads{UnitID: 0, ModelBlock: h18Block(h18Label, 30)})
	h18RequireRejected(t, "the gateway's offering read as the second inverter", card.Evaluate(input))
}

func TestH18SerialEsnReadabilityAndFirmwareNeverSelect(t *testing.T) {
	h18RequireControl(t)
	card := NewHuaweiEMMAObservedQualificationCard()
	reference := card.Evaluate(h18ValidInput(t))
	f := h18LoadAll(t)

	// Readability is no identity. The EMMA model offset answers data at an
	// inverter unit, the SunSpec marker offset answers two words at both, and the
	// S-Dongle registers answer at an inverter unit with an exception or at unit
	// 100 with another: presenting any of these reads as the gateway's fails.
	readable := h18ValidInput(t)
	readable.Gateway.Model = make([]byte, 40)
	for index, word := range h18Words(t, h15ProbeAt(t, f.Overlap, 3, 30222)) {
		readable.Gateway.Model[index*2], readable.Gateway.Model[index*2+1] = byte(word>>8), byte(word)
	}
	h18RequireRejected(t, "the inverter's answer at the EMMA model offset presented as the model", card.Evaluate(readable))

	// Basic identification answers at unit 100 and at an inverter unit with the
	// gateway's own reply, so the unit of a request is no identity.
	for _, unit := range []uint8{3, 100} {
		input := h18ValidInput(t)
		input.Gateway.UnitID = unit
		h18RequireRejected(t, fmt.Sprintf("the same blocks and replies addressed to unit %d", unit), card.Evaluate(input))
	}

	// Gateway reads without any inverter read, and inverter reads without the
	// gateway's, select nothing.
	gatewayOnly := h18ValidInput(t)
	gatewayOnly.Children = nil
	h18RequireRejected(t, "readable gateway blocks alone", card.Evaluate(gatewayOnly))
	basicOnly := HuaweiEMMAObservedQualificationInput{Gateway: HuaweiEMMAObservedGatewayReads{Basic: h18ValidInput(t).Gateway.Basic}}
	h18RequireRejected(t, "basic identification alone", card.Evaluate(basicOnly))
	inventoryOnly := HuaweiEMMAObservedQualificationInput{Gateway: HuaweiEMMAObservedGatewayReads{Inventory: h18ValidInput(t).Gateway.Inventory}}
	h18RequireRejected(t, "extended inventory alone", card.Evaluate(inventoryOnly))

	// Serial-bearing and ESN-class values are never inputs and never predicates.
	for _, value := range []string{"A", "000000000000", h03CountingDigits(12), h03Joined("AB", "12", "CD", "34", "EF", "56"), "XX_XXX_XXXXXX", strings.Repeat("Z", 60)} {
		value := value
		for _, id := range []uint8{0x88, 0x89, 0x8a, 0x8b} {
			input := h18ValidInput(t)
			entry := h18EntryText(input, id)
			key4 := "4=" + h03AttributeValue(entry, "4")
			h18ReplaceEntry(id, key4, "4="+value)(&input)
			result := card.Evaluate(input)
			if !result.IdentityMatched() || !reflect.DeepEqual(result.MissingEvidence(), reference.MissingEvidence()) || result.PageAnomalies() != reference.PageAnomalies() {
				t.Fatalf("sensitive attribute value %q in object %02x changed the outcome: %#v", value, id, result)
			}
		}
	}
	for _, kind := range []reflect.Type{
		reflect.TypeOf(HuaweiEMMAObservedQualificationInput{}), reflect.TypeOf(HuaweiEMMAObservedGatewayReads{}),
		reflect.TypeOf(HuaweiEMMAObservedChildReads{}), reflect.TypeOf(HuaweiEMMAObservedInventoryPage{}), reflect.TypeOf(HuaweiEMMAObservedBasicReply{}),
	} {
		for _, name := range []string{"Serial", "SerialBearing", "Esn", "ESN", "Firmware", "FirmwareBlock", "Protocol", "ProtocolWords", "Residue", "Sequence", "Capacity"} {
			if _, ok := kind.FieldByName(name); ok {
				t.Fatalf("%s carries %s", kind, name)
			}
		}
	}
	if fields := reflect.TypeOf(HuaweiEMMAObservedChildReads{}); fields.NumField() != 2 {
		t.Fatalf("a new inverter input must be reviewed against the forbidden-identity list: %d fields", fields.NumField())
	}
	if fields := reflect.TypeOf(HuaweiEMMAObservedGatewayReads{}); fields.NumField() != 6 {
		t.Fatalf("a new gateway input must be reviewed against the forbidden-identity list: %d fields", fields.NumField())
	}

	// Prefix, wildcard, case and whitespace matches never select the tuple.
	for _, mutant := range []string{"SmartHEM", "SmartHEMS*", "SmartHEMS?", "smarthems", "Smart HEMS", " SmartHEMS", "SmartHEMS\x01"} {
		input := h18ValidInput(t)
		input.Gateway.Offering = h18Block(mutant, 30)
		h18RequireRejected(t, "family label "+mutant, card.Evaluate(input))
	}
	for _, mutant := range []string{"EMMA-A0", "EMMA-A0*", "EMMA-A0?", "emma-a02", "EMMA-A 02", " EMMA-A02", "EMMA"} {
		input := h18ValidInput(t)
		input.Gateway.Model = h18Block(mutant, 40)
		h18ReplaceEntry(0x88, "1="+h18Model+";", "1="+mutant+";")(&input)
		h18RequireRejected(t, "model "+mutant, card.Evaluate(input))
	}
	for _, mutant := range []string{"SUN2000", "SUN2000-", "SUN2000-*", "sun2000-00x-xxx0", "SUN2000 -00X-XXX0", " SUN2000-00X-XXX0"} {
		input := h18ValidInput(t)
		h18SetChildBlock(3, h18ChildBlock(mutant, h18ChildResidue))(&input)
		h18ReplaceEntry(0x8a, "1="+h18ChildModel+";", "1="+mutant+";")(&input)
		h18RequireRejected(t, "inverter model "+mutant, card.Evaluate(input))
	}
}

func TestH18ObservedCardResolvesWithoutWinnerAndSupersedesTheDocumentaryResultOnlyByItsOwnClass(t *testing.T) {
	h18RequireControl(t)
	h03RequireControl(t)
	h15RequireControl(t)
	observed := NewHuaweiEMMAObservedQualificationCard().Evaluate(h18ValidInput(t))
	failed := NewHuaweiEMMAObservedQualificationCard().Evaluate(HuaweiEMMAObservedQualificationInput{})
	documentary := NewHuaweiEMMAQualificationCard().Evaluate(HuaweiEMMAQualificationInput{
		UnitID: 0, Offering: "SmartHEMS", Model: "EMMA-A02", Firmware: "SmartHEMS V100R025C00SPC131",
	})
	fixtureReads := h18ValidInput(t).Gateway
	documentaryOnFixture := NewHuaweiEMMAQualificationCard().Evaluate(HuaweiEMMAQualificationInput{
		UnitID: fixtureReads.UnitID, Offering: string(fixtureReads.Offering), Model: string(fixtureReads.Model), Firmware: string(fixtureReads.Software),
	})
	documentaryFailed := NewHuaweiEMMAQualificationCard().Evaluate(HuaweiEMMAQualificationInput{})
	logger := NewHuaweiSmartLoggerObservedQualificationCard().Evaluate(h03ValidInput(t))
	dongle := NewHuaweiSDongleObservedQualificationCard().Evaluate(h15ValidInput(t))
	blocked := NewHuaweiSDongleQualificationCard().Result()
	if !observed.IdentityMatched() || failed.IdentityMatched() || !documentary.IdentityMatched() || documentaryFailed.IdentityMatched() ||
		!logger.IdentityMatched() || !dongle.IdentityMatched() {
		t.Fatal("controls are not positive and negative as required")
	}

	// The documentary card reads the three identity blocks of the fixtures and
	// returns the identity as known with the capabilities missing.
	if !documentaryOnFixture.IdentityMatched() || documentaryOnFixture.Readiness() != HuaweiQualificationEvidenceBlocked ||
		documentaryOnFixture.ModelVariant() != h18Model || documentaryOnFixture.Reason() != HuaweiQualificationIdentityMatchedCapabilitiesMissing {
		t.Fatalf("the documentary card on the fixture blocks: %#v", documentaryOnFixture)
	}
	for index, results := range [][]HuaweiQualificationResult{{observed, documentaryOnFixture}, {documentaryOnFixture, observed}} {
		if resolution := ResolveHuaweiQualification(results...); resolution.Readiness() != HuaweiQualificationTestReady || resolution.Reason() != HuaweiQualificationMatchedIdentityInventory {
			t.Fatalf("order %d: fixture-based documentary result was not superseded: %#v", index, resolution)
		}
	}

	// Alone, the observed tuple resolves to its class.
	alone := ResolveHuaweiQualification(observed)
	if alone.SelectedClass() != HuaweiEMMACanonicalClass || alone.Readiness() != HuaweiQualificationTestReady || alone.ModelVariant() != h18Model ||
		!reflect.DeepEqual(alone.CandidateClasses(), []string{HuaweiEMMACanonicalClass}) || alone.FallbackAttempted() {
		t.Fatalf("single observed result resolved to %#v", alone)
	}
	if failedAlone := ResolveHuaweiQualification(failed); failedAlone.Readiness() != HuaweiQualificationInsufficientEvidence || failedAlone.SelectedClass() != "" {
		t.Fatalf("failed observed result resolved to %#v", failedAlone)
	}

	// The documentary result alone is unchanged: the identity is known and the
	// capabilities are not.
	if documentaryAlone := ResolveHuaweiQualification(documentary); documentaryAlone.Readiness() != HuaweiQualificationEvidenceBlocked ||
		documentaryAlone.Reason() != HuaweiQualificationIdentityMatchedCapabilitiesMissing || documentaryAlone.SelectedClass() != HuaweiEMMACanonicalClass {
		t.Fatalf("documentary result alone resolved to %#v", documentaryAlone)
	}

	// The same device satisfies both EMMA cards, and the observed one is the more
	// complete: the documentary result is superseded, in every order.
	for index, results := range [][]HuaweiQualificationResult{{observed, documentary}, {documentary, observed}, {documentary, observed, documentaryFailed}, {failed, documentary, observed}} {
		resolution := ResolveHuaweiQualification(results...)
		if resolution.SelectedClass() != HuaweiEMMACanonicalClass || resolution.Readiness() != HuaweiQualificationTestReady ||
			resolution.Reason() != HuaweiQualificationMatchedIdentityInventory || resolution.ModelVariant() != h18Model ||
			!reflect.DeepEqual(resolution.MissingEvidence(), observed.MissingEvidence()) || resolution.FallbackAttempted() {
			t.Fatalf("order %d: the observed tuple did not supersede the documentary result: %#v", index, resolution)
		}
	}
	// A failed observed card does not hide the documentary identity match, and
	// does not turn it into a complete one.
	if mixed := ResolveHuaweiQualification(failed, documentary); mixed.Readiness() != HuaweiQualificationEvidenceBlocked || mixed.SelectedClass() != HuaweiEMMACanonicalClass {
		t.Fatalf("a failed sibling changed the documentary result: %#v", mixed)
	}

	// Two positives of different classes produce no winner, in every order.
	for name, other := range map[string]HuaweiQualificationResult{"SmartLogger": logger, "S-Dongle": dongle} {
		for _, results := range [][]HuaweiQualificationResult{
			{observed, other}, {other, observed}, {observed, documentary, other}, {other, documentary, observed},
		} {
			resolution := ResolveHuaweiQualification(results...)
			want := []string{HuaweiEMMACanonicalClass, other.SelectedClass()}
			sort.Strings(want)
			if resolution.Readiness() != HuaweiQualificationAmbiguous || resolution.Reason() != HuaweiQualificationMultipleClasses ||
				resolution.SelectedClass() != "" || resolution.IdentityMatched() || !reflect.DeepEqual(resolution.CandidateClasses(), want) ||
				resolution.NativeFactCount() != 0 || resolution.AutomaticRequestCount() != 0 || resolution.FallbackAttempted() {
				t.Fatalf("%s next to the EMMA tuple produced a winner: %#v", name, resolution)
			}
		}
	}
	// The documentary EMMA result is superseded only by its own class: next to a
	// complete result of another class it still counts as a positive.
	if resolution := ResolveHuaweiQualification(documentary, logger); resolution.Readiness() != HuaweiQualificationAmbiguous ||
		!reflect.DeepEqual(resolution.CandidateClasses(), []string{HuaweiEMMACanonicalClass, HuaweiSmartLoggerCanonicalClass}) {
		t.Fatalf("the documentary EMMA result was superseded by another class: %#v", resolution)
	}

	// Only an identity match that waits for capability evidence is superseded: a
	// blocked match of another reason, and a ready match that carries that reason,
	// stay positives of the class.
	blockedForAnotherReason := HuaweiQualificationResult{
		candidateClass: HuaweiEMMACanonicalClass, selectedClass: HuaweiEMMACanonicalClass, readiness: HuaweiQualificationEvidenceBlocked,
		reason: HuaweiQualificationInventoryInvalid, identityMatched: true,
	}
	readyWithTheCapabilityReason := HuaweiQualificationResult{
		candidateClass: HuaweiEMMACanonicalClass, selectedClass: HuaweiEMMACanonicalClass, readiness: HuaweiQualificationTestReady,
		reason: HuaweiQualificationIdentityMatchedCapabilitiesMissing, identityMatched: true,
	}
	for name, other := range map[string]HuaweiQualificationResult{"blocked for another reason": blockedForAnotherReason, "ready with the capability reason": readyWithTheCapabilityReason} {
		for _, results := range [][]HuaweiQualificationResult{{observed, other}, {other, observed}} {
			if resolution := ResolveHuaweiQualification(results...); resolution.Readiness() != HuaweiQualificationAmbiguous || resolution.SelectedClass() != "" {
				t.Fatalf("%s was superseded: %#v", name, resolution)
			}
		}
	}

	// One positive next to a failed result of another class stays insufficient.
	if mixed := ResolveHuaweiQualification(observed, NewHuaweiSmartLoggerObservedQualificationCard().Evaluate(HuaweiSmartLoggerObservedQualificationInput{})); mixed.Readiness() != HuaweiQualificationInsufficientEvidence ||
		mixed.SelectedClass() != "" || mixed.FallbackAttempted() {
		t.Fatalf("positive next to a failed other class selected: %#v", mixed)
	}

	// Two complete results of the EMMA class would be two tuples at once.
	second := observed
	second.basicOpaqueObjects = 0
	if contradiction := ResolveHuaweiQualification(observed, second); contradiction.Readiness() != HuaweiQualificationAmbiguous || contradiction.SelectedClass() != "" {
		t.Fatalf("two complete results of one class selected: %#v", contradiction)
	}

	// The S-Dongle hard stop is superseded by its own class only, whatever the
	// EMMA results say.
	for name, results := range map[string][]HuaweiQualificationResult{
		"EMMA match":                 {observed, blocked},
		"EMMA match first":           {blocked, observed},
		"EMMA and documentary match": {documentary, blocked, observed},
	} {
		resolution := ResolveHuaweiQualification(results...)
		if resolution.Readiness() != HuaweiQualificationEvidenceBlocked || resolution.Reason() != HuaweiQualificationPersistentNonResponse ||
			!reflect.DeepEqual(resolution.MissingEvidence(), blocked.MissingEvidence()) || resolution.SelectedClass() != "" {
			t.Fatalf("%s: the hard stop was lost: %#v", name, resolution)
		}
	}
}

func TestH18ObservedCardCarriesNoAuthority(t *testing.T) {
	card := NewHuaweiEMMAObservedQualificationCard()
	results := []HuaweiQualificationResult{card.EmptyResult(), card.Evaluate(h18ValidInput(t)), card.Evaluate(HuaweiEMMAObservedQualificationInput{})}
	for _, result := range results {
		if !result.DefaultDenied() || result.CatalogRegistered() || result.AutomaticRuntimeAdmission() || result.LiveQualified() ||
			result.SupportClaim() || result.WriteAuthority() || result.NativeFactCount() != 0 || result.AutomaticRequestCount() != 0 ||
			result.FallbackAttempted() || result.HardwareTestReady() {
			t.Fatalf("result created authority or telemetry: %#v", result)
		}
	}
	// Evaluation does not retain or alias the caller's buffers.
	input := h18ValidInput(t)
	before := card.Evaluate(input)
	beforeInventory, _ := before.Inventory()
	for index := range input.Gateway.Offering {
		input.Gateway.Offering[index] = 'Z'
	}
	for index := range input.Children[0].ModelBlock {
		input.Children[0].ModelBlock[index] = 'Z'
	}
	for _, page := range input.Gateway.Inventory {
		for _, object := range page.Objects {
			for index := range object.Value {
				object.Value[index] = 'Z'
			}
		}
	}
	after := card.Evaluate(h18ValidInput(t))
	afterInventory, _ := after.Inventory()
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(beforeInventory.Children(), afterInventory.Children()) {
		t.Fatalf("evaluation depends on shared buffers: %#v %#v", before, after)
	}
	if rejected := card.Evaluate(input); rejected.IdentityMatched() {
		t.Fatalf("a changed buffer still matched: %#v", rejected)
	}
	// The retained inventory is a copy: changing it changes neither the result
	// nor a later read of it.
	inventory, _ := after.Inventory()
	children := inventory.Children()
	children[0] = HuaweiGatewayInventoryChild{}
	again, _ := after.Inventory()
	if again.Children()[0].Address() != "2" {
		t.Fatal("the retained child list is aliased")
	}
}
