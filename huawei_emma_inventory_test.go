package modbusreg

import "testing"

func TestParseHuaweiEMMAOfflineInventoryAcceptsFiniteSelfAlias(t *testing.T) {
	inventory, err := ParseHuaweiEMMAOfflineInventory(HuaweiEMMAOfflineInventoryInput{
		InventoryGuardBefore: 12,
		InventoryGuardAfter:  12,
		Pages: []HuaweiSmartLoggerInventoryPage{{
			Objects: []HuaweiSmartLoggerInventoryObject{
				{ObjectID: 0x87, Value: []byte{1}},
				{ObjectID: 0x88, Value: []byte("1=EMMA-A02;2=SmartHEMS V100R025C00SPC131;5=0")},
				{ObjectID: 0x89, Value: []byte("1=SUN2000;2=V300R024C10SPC191;5=1")},
			},
		}},
	})
	if err != nil {
		t.Fatalf("ParseHuaweiEMMAOfflineInventory() error = %v", err)
	}
	if inventory.DeclaredChildren() != 1 || inventory.ChildCount() != 1 || !inventory.DefaultDenied() ||
		inventory.Children()[0].Address() != "1" {
		t.Fatalf("unexpected EMMA inventory: %#v", inventory)
	}
}

func TestParseHuaweiEMMAOfflineInventoryFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name  string
		input HuaweiEMMAOfflineInventoryInput
	}{
		{
			name: "canonical family token is not an alias",
			input: HuaweiEMMAOfflineInventoryInput{
				InventoryGuardBefore: 1, InventoryGuardAfter: 1,
				Pages: []HuaweiSmartLoggerInventoryPage{{Objects: []HuaweiSmartLoggerInventoryObject{
					{ObjectID: 0x87, Value: []byte{0}},
					{ObjectID: 0x88, Value: []byte("1=EMMA;5=0")},
				}}},
			},
		},
		{
			name: "case differs",
			input: HuaweiEMMAOfflineInventoryInput{
				InventoryGuardBefore: 1, InventoryGuardAfter: 1,
				Pages: []HuaweiSmartLoggerInventoryPage{{Objects: []HuaweiSmartLoggerInventoryObject{
					{ObjectID: 0x87, Value: []byte{0}},
					{ObjectID: 0x88, Value: []byte("1=emma-a01;5=0")},
				}}},
			},
		},
		{
			name: "inventory guard changes",
			input: HuaweiEMMAOfflineInventoryInput{
				InventoryGuardBefore: 1, InventoryGuardAfter: 2,
				Pages: []HuaweiSmartLoggerInventoryPage{{Objects: []HuaweiSmartLoggerInventoryObject{
					{ObjectID: 0x87, Value: []byte{0}},
					{ObjectID: 0x88, Value: []byte("1=EMMA-A01;5=0")},
				}}},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			inventory, err := ParseHuaweiEMMAOfflineInventory(test.input)
			if err == nil || inventory.DeclaredChildren() != 0 || inventory.ChildCount() != 0 {
				t.Fatalf("expected fail-closed inventory, got %#v err=%v", inventory, err)
			}
		})
	}
}
