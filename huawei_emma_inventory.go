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

// ParseHuaweiEMMAOfflineInventory validates a supplied EMMA inventory. Only
// the finite EMMA aliases can be a self entry; a canonical family label or an
// unrecognized variant is fail-closed.
func ParseHuaweiEMMAOfflineInventory(input HuaweiEMMAOfflineInventoryInput) (HuaweiEMMAOfflineInventory, error) {
	return parseHuaweiExtendedMEIOfflineInventory(HuaweiSmartLoggerOfflineInventoryInput{
		ChangeCounterBefore: input.InventoryGuardBefore,
		ChangeCounterAfter:  input.InventoryGuardAfter,
		Pages:               input.Pages,
	}, func(model string) bool {
		return model == "EMMA-A01" || model == "EMMA-A02"
	})
}
