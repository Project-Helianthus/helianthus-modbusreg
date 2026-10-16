package modbusreg

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// These are detached parser vectors, not encoded MEI responses or qualification.
func k19Inventory(children int, separateCountPage bool) HuaweiSmartLoggerOfflineInventoryInput {
	input := HuaweiSmartLoggerOfflineInventoryInput{ChangeCounterBefore: 7, ChangeCounterAfter: 7}
	objects := []HuaweiSmartLoggerInventoryObject{
		{ObjectID: 0x87, Value: []byte{byte(children)}},
		{ObjectID: 0x88, Value: []byte("1=SmartLogger;5=0")},
	}
	for child := 1; child <= children; child++ {
		objects = append(objects, HuaweiSmartLoggerInventoryObject{
			ObjectID: uint8(0x88 + child),
			Value:    []byte(fmt.Sprintf("1=SUN2000;5=%d", child)),
		})
	}
	if !separateCountPage {
		input.Pages = append(input.Pages, HuaweiSmartLoggerInventoryPage{Objects: objects[:2]})
		objects = objects[2:]
	}
	for _, object := range objects {
		input.Pages = append(input.Pages, HuaweiSmartLoggerInventoryPage{Objects: []HuaweiSmartLoggerInventoryObject{object}})
	}
	for page := 0; page < len(input.Pages)-1; page++ {
		input.Pages[page].More = true
		input.Pages[page].NextObjectID = input.Pages[page+1].Objects[0].ObjectID
	}
	return input
}

func k19Bytes(input HuaweiSmartLoggerOfflineInventoryInput) int {
	total := 0
	for _, page := range input.Pages {
		for _, object := range page.Objects {
			total += 2 + len(object.Value)
		}
	}
	return total
}

func k19RequireRejected(t *testing.T, input HuaweiSmartLoggerOfflineInventoryInput) {
	t.Helper()
	inventory, err := ParseHuaweiSmartLoggerOfflineInventory(input)
	if err == nil || !reflect.DeepEqual(inventory, HuaweiSmartLoggerInventory{}) ||
		inventory.DeclaredChildren() != 0 || inventory.ChildCount() != 0 || len(inventory.Children()) != 0 || !inventory.DefaultDenied() {
		t.Fatalf("expected empty default-denied inventory, got %#v, error %v", inventory, err)
	}
}

func TestK19AggregateByteBoundary(t *testing.T) {
	for _, size := range []int{65536, 65537} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			input := k19Inventory(2, false)
			// Split padding between child pages: every page is below the budget.
			padding := size - k19Bytes(input) - 2*len(";2=")
			for page := 1; page <= 2; page++ {
				extra := padding / 2
				if page == 2 {
					extra = padding - extra
				}
				input.Pages[page].Objects[0].Value = append(input.Pages[page].Objects[0].Value, []byte(";2="+strings.Repeat("x", extra))...)
			}
			if got := k19Bytes(input); got != size {
				t.Fatalf("fixture bytes = %d, want %d", got, size)
			}
			for _, page := range input.Pages {
				if k19Bytes(HuaweiSmartLoggerOfflineInventoryInput{Pages: []HuaweiSmartLoggerInventoryPage{page}}) >= 65536 {
					t.Fatal("fixture must exercise an aggregate, not per-page, limit")
				}
			}
			if size > 65536 {
				k19RequireRejected(t, input)
				return
			}
			inventory, err := ParseHuaweiSmartLoggerOfflineInventory(input)
			if err != nil || inventory.ChildCount() != 2 || inventory.DeclaredChildren() != 2 || !inventory.DefaultDenied() {
				t.Fatalf("boundary inventory = %#v, error %v", inventory, err)
			}
			before := inventory.Children()
			for page := range input.Pages {
				for object := range input.Pages[page].Objects {
					for index := range input.Pages[page].Objects[object].Value {
						input.Pages[page].Objects[object].Value[index] = '!'
					}
				}
			}
			if !reflect.DeepEqual(before, inventory.Children()) {
				t.Fatal("input mutation changed parsed inventory")
			}
			output := inventory.Children()
			output[0] = HuaweiSmartLoggerInventoryChild{}
			if !reflect.DeepEqual(before, inventory.Children()) {
				t.Fatal("returned slice mutation changed parsed inventory")
			}
		})
	}
}

func TestK19AggregateObjectAndPageBoundaries(t *testing.T) {
	input := k19Inventory(247, false)
	records := 0
	for _, page := range input.Pages {
		records += len(page.Objects)
	}
	// The count object is separate from the 248 self/child records.
	if len(input.Pages) != 248 || records-1 != 248 {
		t.Fatalf("fixture pages=%d records=%d", len(input.Pages), records-1)
	}
	inventory, err := ParseHuaweiSmartLoggerOfflineInventory(input)
	if err != nil || inventory.ChildCount() != 247 || inventory.DeclaredChildren() != 247 || !inventory.DefaultDenied() {
		t.Fatalf("maximum inventory = %#v, error %v", inventory, err)
	}
	t.Run("one extra page with same valid records", func(t *testing.T) {
		input := k19Inventory(247, true)
		if len(input.Pages) != 249 {
			t.Fatal("fixture must exceed page budget by exactly one")
		}
		k19RequireRejected(t, input)
	})
	t.Run("one extra record on final page", func(t *testing.T) {
		input := k19Inventory(247, false)
		last := &input.Pages[len(input.Pages)-1]
		last.Objects = append(last.Objects, HuaweiSmartLoggerInventoryObject{
			ObjectID: last.Objects[0].ObjectID + 1,
			Value:    []byte("1=SUN2000;5=248"),
		})
		k19RequireRejected(t, input)
	})
}

func TestK19MultiPageFailureDiscardsInventory(t *testing.T) {
	for _, name := range []string{"counter changed", "counter wrapped", "late malformed attribute", "late duplicate address", "late self entry", "missing self entry"} {
		t.Run(name, func(t *testing.T) {
			input := k19Inventory(3, false)
			last := &input.Pages[len(input.Pages)-1].Objects[0]
			switch name {
			case "counter changed":
				input.ChangeCounterAfter++
			case "counter wrapped":
				input.ChangeCounterBefore, input.ChangeCounterAfter = 65535, 0
			case "late malformed attribute":
				last.Value = []byte("1=SUN2000;5")
			case "late duplicate address":
				last.Value = []byte("1=SUN2000;5=1")
			case "late self entry":
				last.Value = []byte("1=SmartLogger;5=3")
			case "missing self entry":
				input.Pages[0].Objects[1].Value = []byte("1=SUN2000;5=0")
			}
			k19RequireRejected(t, input)
		})
	}
}
