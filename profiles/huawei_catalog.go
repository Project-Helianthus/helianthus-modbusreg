package profiles

import "embed"

// The generated Huawei catalog has two parts with separate readers. Each
// file system embeds only the files its reader needs; no dictionary or
// quarantine file of the catalog is embedded.

//go:embed vendors/huawei/catalog/editions.json vendors/huawei/catalog/models.json vendors/huawei/catalog/registers-*.json
var huaweiInverterCatalog embed.FS

//go:embed vendors/huawei/catalog/smartlogger-sdongle-emma/editions.json vendors/huawei/catalog/smartlogger-sdongle-emma/services.json
var huaweiGatewayCatalog embed.FS

// HuaweiInverterCatalog returns the read-only inverter part of the Huawei
// catalog: the editions, the model rows and the register files under
// vendors/huawei/catalog. Paths inside it keep that directory.
func HuaweiInverterCatalog() embed.FS {
	return huaweiInverterCatalog
}

// HuaweiGatewayCatalog returns the read-only SmartLogger, S-Dongle and EMMA
// part of the Huawei catalog: the editions and the services under
// vendors/huawei/catalog/smartlogger-sdongle-emma. Paths inside it keep that
// directory.
func HuaweiGatewayCatalog() embed.FS {
	return huaweiGatewayCatalog
}
