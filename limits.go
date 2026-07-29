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

	// MaxDeclaredCoherenceSkew bounds source/receipt collection windows without
	// defining downstream freshness or availability policy.
	MaxDeclaredCoherenceSkew = 24 * time.Hour
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
