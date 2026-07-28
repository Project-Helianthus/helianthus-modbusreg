package modbusreg

import (
	"fmt"
	"unicode/utf8"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

const (
	// PinnedMaxCoalescedDependents is the absolute V1 runtime guard at
	// helianthus-modbus commit 4f81cbeb6321e64fa51676ed6e375ce36b60d16d.
	// It is not a scheduler configuration value.
	PinnedMaxCoalescedDependents = 4096

	MaxProfileDependencies       = PinnedMaxCoalescedDependents
	MaxProfileCodecs             = PinnedMaxCoalescedDependents
	MaxProfileEvidenceReferences = PinnedMaxCoalescedDependents
	MaxCodecSentinels            = 256
	MaxRawWords                  = modbus.MaxReadRegisters
	MaxContractStringBytes       = 4096
	MaxContractJSONDepth         = 32
	MaxSerializedContractBytes   = 4 * 1024 * 1024
	// MaxSampleIssuerDomainBytes reserves ':' plus the 20 decimal digits
	// required by the largest uint64-issued sequence.
	MaxSampleIssuerDomainBytes = MaxContractStringBytes - 21

	// MaxSampleLedgerRecords is zero because the ledger is an O(1)
	// issuer-domain/high-water state, not an arbitrary sample-ID collection.
	MaxSampleLedgerRecords = 0
)

func validateBoundedString(field, value string, required bool) error {
	if (required && value == "") ||
		len(value) > MaxContractStringBytes ||
		!utf8.ValidString(value) {
		return fmt.Errorf("%s exceeds the contract string boundary", field)
	}
	return nil
}

func validateBoundedStrings(
	field string,
	values []string,
	maximum int,
	required bool,
) error {
	if len(values) > maximum || (required && len(values) == 0) {
		return fmt.Errorf("%s exceeds the contract collection boundary", field)
	}
	for _, value := range values {
		if err := validateBoundedString(field, value, true); err != nil {
			return err
		}
	}
	return nil
}
