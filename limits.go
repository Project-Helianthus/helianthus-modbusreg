package modbusreg

import (
	"fmt"
	"reflect"
	"time"
	"unicode/utf8"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

const (
	// PinnedMaxCoalescedDependents is the absolute V1 runtime guard at
	// helianthus-modbus commit eab30aed9eb6f78a61c679c3bd9403d587025214.
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
	// MaxWireResponseEvidenceBytes covers the largest pinned FC03/FC04 data
	// payload plus a bounded transport envelope. It is independent of the
	// serialized-contract aggregate budget.
	MaxWireResponseEvidenceBytes = 2*MaxRawWords + 32
	// MaxSampleIssuerDomainBytes reserves ':' plus the 20 decimal digits
	// required by the largest uint64-issued sequence.
	MaxSampleIssuerDomainBytes = MaxContractStringBytes - 21

	// MaxSampleLedgerRecords is zero because the ledger is an O(1)
	// issuer-domain/high-water state, not an arbitrary sample-ID collection.
	MaxSampleLedgerRecords = 0

	// MaxDeclaredCoherenceSkew bounds source/receipt collection windows without
	// defining downstream freshness or availability policy.
	MaxDeclaredCoherenceSkew = 24 * time.Hour
)

// LedgerLimits closes every allocation dimension owned by the M2 attempt
// ledger. The runtime source has its own independent M1 limits.
type LedgerLimits struct {
	MaxRetainedAttempts                 int
	MaxClaimEntriesPerAttempt           int
	MaxRetainedClaimEntries             int
	MaxDependencySetEncodedBytes        int
	AttemptKeyMaxUTF8Bytes              int
	NormalizationRecordMaxEncodedBytes  int
	RetainedDiagnosticCountPerObjectMax int
	RetainedDiagnosticMaxUTF8Bytes      int
	AuditTombstoneLimit                 int
	AuditTombstoneMaxEncodedBytes       int
}

// DefaultLedgerLimits returns finite production-safe contract bounds. Callers
// may choose lower limits, but zero and internally inconsistent limits fail.
func DefaultLedgerLimits() LedgerLimits {
	return LedgerLimits{
		MaxRetainedAttempts:                 64,
		MaxClaimEntriesPerAttempt:           MaxProfileDependencies,
		MaxRetainedClaimEntries:             64 * MaxProfileDependencies,
		MaxDependencySetEncodedBytes:        MaxSerializedContractBytes,
		AttemptKeyMaxUTF8Bytes:              MaxContractStringBytes,
		NormalizationRecordMaxEncodedBytes:  MaxSerializedContractBytes,
		RetainedDiagnosticCountPerObjectMax: 32,
		RetainedDiagnosticMaxUTF8Bytes:      MaxContractStringBytes,
		AuditTombstoneLimit:                 4096,
		AuditTombstoneMaxEncodedBytes:       256,
	}
}

func validateLedgerLimits(limits LedgerLimits) error {
	positive := []int{
		limits.MaxRetainedAttempts,
		limits.MaxClaimEntriesPerAttempt,
		limits.MaxRetainedClaimEntries,
		limits.MaxDependencySetEncodedBytes,
		limits.AttemptKeyMaxUTF8Bytes,
		limits.NormalizationRecordMaxEncodedBytes,
		limits.RetainedDiagnosticCountPerObjectMax,
		limits.RetainedDiagnosticMaxUTF8Bytes,
		limits.AuditTombstoneLimit,
		limits.AuditTombstoneMaxEncodedBytes,
	}
	for _, value := range positive {
		if value <= 0 {
			return fmt.Errorf("ledger limits must be finite and positive")
		}
	}
	if limits.MaxRetainedAttempts > 1<<16 ||
		limits.MaxClaimEntriesPerAttempt > MaxProfileDependencies ||
		limits.MaxDependencySetEncodedBytes > MaxSerializedContractBytes ||
		limits.AttemptKeyMaxUTF8Bytes > MaxContractStringBytes ||
		limits.NormalizationRecordMaxEncodedBytes > MaxSerializedContractBytes ||
		limits.RetainedDiagnosticCountPerObjectMax > MaxProfileDependencies ||
		limits.RetainedDiagnosticMaxUTF8Bytes > MaxContractStringBytes ||
		limits.AuditTombstoneLimit > 1<<20 ||
		limits.AuditTombstoneMaxEncodedBytes > MaxSerializedContractBytes {
		return fmt.Errorf("ledger limits exceed the contract maximum")
	}
	if limits.MaxRetainedAttempts > int(^uint(0)>>1)/limits.MaxClaimEntriesPerAttempt {
		return fmt.Errorf("ledger claim-entry product overflows")
	}
	product := limits.MaxRetainedAttempts * limits.MaxClaimEntriesPerAttempt
	if limits.MaxRetainedClaimEntries < limits.MaxClaimEntriesPerAttempt ||
		limits.MaxRetainedClaimEntries > product {
		return fmt.Errorf("ledger claim-entry limits are inconsistent")
	}
	return nil
}

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

type aggregateBudget struct {
	remaining uint64
}

func (budget *aggregateBudget) consume(amount uint64) error {
	if amount > budget.remaining {
		return fmt.Errorf("contract input exceeds the cumulative byte boundary")
	}
	budget.remaining -= amount
	return nil
}

var timeValueType = reflect.TypeOf(time.Time{})
var dependencyResultValueType = reflect.TypeOf(DependencyResult{})

func preflightAggregate(values ...any) error {
	budget := aggregateBudget{remaining: MaxSerializedContractBytes}
	for _, value := range values {
		if err := consumeAggregateValue(
			&budget,
			reflect.ValueOf(value),
			1,
		); err != nil {
			return err
		}
	}
	return nil
}

func consumeAggregateValue(
	budget *aggregateBudget,
	value reflect.Value,
	depth int,
) error {
	if depth > MaxContractJSONDepth {
		return fmt.Errorf("contract input exceeds the nesting boundary")
	}
	if !value.IsValid() {
		return budget.consume(4)
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return budget.consume(4)
		}
		return consumeAggregateValue(budget, value.Elem(), depth+1)
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return budget.consume(4)
		}
		if err := budget.consume(8); err != nil {
			return err
		}
		return consumeAggregateValue(budget, value.Elem(), depth+1)
	}
	if value.Type() == timeValueType {
		return budget.consume(48)
	}
	switch value.Kind() {
	case reflect.String:
		length := uint64(len(value.String()))
		if length > (^uint64(0)-32)/6 {
			return fmt.Errorf("contract string size overflows")
		}
		return budget.consume(length*6 + 32)
	case reflect.Bool:
		return budget.consume(8)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return budget.consume(32)
	case reflect.Slice, reflect.Array:
		length := uint64(value.Len())
		if length > (^uint64(0)-16)/8 {
			return fmt.Errorf("contract collection size overflows")
		}
		if err := budget.consume(length*8 + 16); err != nil {
			return err
		}
		for index := 0; index < value.Len(); index++ {
			if err := consumeAggregateValue(
				budget,
				value.Index(index),
				depth+1,
			); err != nil {
				return err
			}
		}
		return nil
	case reflect.Struct:
		valueType := value.Type()
		if err := budget.consume(uint64(value.NumField())*16 + 16); err != nil {
			return err
		}
		for index := 0; index < value.NumField(); index++ {
			field := valueType.Field(index)
			if valueType == dependencyResultValueType &&
				(field.Name == "claim" || field.Name == "owner") {
				continue
			}
			if err := budget.consume(uint64(len(field.Name))*2 + 8); err != nil {
				return err
			}
			if err := consumeAggregateValue(
				budget,
				value.Field(index),
				depth+1,
			); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("contract input contains an unsupported aggregate kind")
	}
}
