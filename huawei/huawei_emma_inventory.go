package huawei

// EMMAOfflineInventoryInput is a detached, already-read EMMA extended
// MEI inventory. The guard values are supplied by the caller; this package
// performs no request or inventory acquisition.
type EMMAOfflineInventoryInput struct {
	InventoryGuardBefore uint16
	InventoryGuardAfter  uint16
	Pages                []GatewayInventoryPage
}

// EMMAOfflineInventory is a validated, default-denied inventory view.
type EMMAOfflineInventory = GatewayInventory

// ParseHuaweiEMMAOfflineInventory validates a supplied EMMA inventory. A self
// entry is a model of the EMMA family that the catalog's model rows hold: a
// model token of the family, not only the observed EMMA-A01 and EMMA-A02. A
// canonical family label alone or a model of another family is fail-closed.
func ParseHuaweiEMMAOfflineInventory(input EMMAOfflineInventoryInput) (EMMAOfflineInventory, error) {
	return parseHuaweiExtendedMEIOfflineInventory(SmartLoggerOfflineInventoryInput{
		ChangeCounterBefore: input.InventoryGuardBefore,
		ChangeCounterAfter:  input.InventoryGuardAfter,
		Pages:               input.Pages,
	}, isHuaweiEMMAFamilyModel, parseHuaweiSmartLoggerInventoryChild)
}
