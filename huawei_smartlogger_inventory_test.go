package modbusreg

import "testing"

func TestParseHuaweiSmartLoggerOfflineInventoryAcceptsStableBoundedPages(t *testing.T) {
	inventory, err := ParseHuaweiSmartLoggerOfflineInventory(HuaweiSmartLoggerOfflineInventoryInput{
		ChangeCounterBefore: 7,
		ChangeCounterAfter:  7,
		Pages: []HuaweiSmartLoggerInventoryPage{
			{
				More:         true,
				NextObjectID: 0x8a,
				Objects: []HuaweiSmartLoggerInventoryObject{
					{ObjectID: 0x87, Value: []byte{2}},
					{ObjectID: 0x88, Value: []byte("1=SmartLogger;2=V300R024C10SPC191;5=0")},
					{ObjectID: 0x89, Value: []byte("1=SUN2000;2=V300R024C10SPC191;5=1")},
				},
			},
			{
				More:         false,
				NextObjectID: 0,
				Objects: []HuaweiSmartLoggerInventoryObject{
					{ObjectID: 0x8a, Value: []byte("1=SUN2000;2=V300R024C10SPC191;5=2")},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ParseHuaweiSmartLoggerOfflineInventory() error = %v", err)
	}
	if inventory.DeclaredChildren() != 2 || inventory.ChildCount() != 2 || !inventory.DefaultDenied() {
		t.Fatalf("unexpected inventory summary: declared=%d children=%d defaultDenied=%t", inventory.DeclaredChildren(), inventory.ChildCount(), inventory.DefaultDenied())
	}
	children := inventory.Children()
	if len(children) != 2 || children[0].ObjectID() != 0x88 || children[0].Address() != "0" ||
		children[0].Attribute("model") != "SmartLogger" || children[1].Address() != "1" {
		t.Fatalf("unexpected parsed children: %#v", children)
	}
	children[0] = HuaweiSmartLoggerInventoryChild{}
	if inventory.Children()[0].Address() != "0" {
		t.Fatal("inventory must not expose mutable children")
	}
}

func TestParseHuaweiSmartLoggerOfflineInventoryFailsClosed(t *testing.T) {
	validChild := HuaweiSmartLoggerInventoryObject{ObjectID: 0x88, Value: []byte("1=SUN2000;5=1")}
	tests := []struct {
		name  string
		input HuaweiSmartLoggerOfflineInventoryInput
	}{
		{
			name: "change counter mismatch",
			input: HuaweiSmartLoggerOfflineInventoryInput{
				ChangeCounterBefore: 1, ChangeCounterAfter: 2,
				Pages: []HuaweiSmartLoggerInventoryPage{{Objects: []HuaweiSmartLoggerInventoryObject{{ObjectID: 0x87, Value: []byte{1}}, validChild}}},
			},
		},
		{
			name: "cursor loop",
			input: HuaweiSmartLoggerOfflineInventoryInput{
				ChangeCounterBefore: 1, ChangeCounterAfter: 1,
				Pages: []HuaweiSmartLoggerInventoryPage{{More: true, NextObjectID: 0x87, Objects: []HuaweiSmartLoggerInventoryObject{{ObjectID: 0x87, Value: []byte{1}}, validChild}}},
			},
		},
		{
			name: "duplicate child address",
			input: HuaweiSmartLoggerOfflineInventoryInput{
				ChangeCounterBefore: 1, ChangeCounterAfter: 1,
				Pages: []HuaweiSmartLoggerInventoryPage{{Objects: []HuaweiSmartLoggerInventoryObject{
					{ObjectID: 0x87, Value: []byte{2}},
					validChild,
					{ObjectID: 0x89, Value: []byte("1=SUN2000;5=1")},
				}}},
			},
		},
		{
			name: "malformed attribute",
			input: HuaweiSmartLoggerOfflineInventoryInput{
				ChangeCounterBefore: 1, ChangeCounterAfter: 1,
				Pages: []HuaweiSmartLoggerInventoryPage{{Objects: []HuaweiSmartLoggerInventoryObject{
					{ObjectID: 0x87, Value: []byte{1}},
					{ObjectID: 0x88, Value: []byte("1=SUN2000;5")},
				}}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inventory, err := ParseHuaweiSmartLoggerOfflineInventory(test.input)
			if err == nil || inventory.ChildCount() != 0 || inventory.DeclaredChildren() != 0 {
				t.Fatalf("expected fail-closed result, inventory=%#v err=%v", inventory, err)
			}
		})
	}
}
