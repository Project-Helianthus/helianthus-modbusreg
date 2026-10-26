package modbusreg

// HuaweiEMMAOfflineInventoryInput is a detached, already-read EMMA extended
// MEI inventory. The guard values are supplied by the caller; this package
// performs no request or inventory acquisition.
type HuaweiEMMAOfflineInventoryInput struct {
	InventoryGuardBefore uint16
	InventoryGuardAfter  uint16
	Pages                []HuaweiGatewayInventoryPage
}

// HuaweiEMMAOfflineInventory is a validated, default-denied inventory view.
type HuaweiEMMAOfflineInventory = HuaweiGatewayInventory

// ParseHuaweiEMMAOfflineInventory validates a supplied EMMA inventory. A self
// entry is a model of the EMMA family that the catalogue's model rows hold: a
// model token of the family, not only the observed EMMA-A01 and EMMA-A02. A
// canonical family label alone or a model of another family is fail-closed.
func ParseHuaweiEMMAOfflineInventory(input HuaweiEMMAOfflineInventoryInput) (HuaweiEMMAOfflineInventory, error) {
	return parseHuaweiExtendedMEIOfflineInventory(HuaweiSmartLoggerOfflineInventoryInput{
		ChangeCounterBefore: input.InventoryGuardBefore,
		ChangeCounterAfter:  input.InventoryGuardAfter,
		Pages:               input.Pages,
	}, isHuaweiEMMAFamilyModel, parseHuaweiSmartLoggerInventoryChild)
}
