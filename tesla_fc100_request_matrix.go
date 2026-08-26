package modbusreg

import "fmt"

// TeslaFC100RequestMatrix retains the complete caller-supplied request body
// and its validated protobuf key sequence for one version-scoped operation.
// The body is copied so later caller mutation cannot change the validation
// result.
type TeslaFC100RequestMatrix struct {
	Operation   TeslaFC100Operation
	RequestBody []byte
	Entries     []TeslaFC100WireEntry
}

var teslaFC100EmptyRequestOperations = map[TeslaFC100Operation]struct{}{
	TeslaFC100OperationCommonSystemInfo:      {},
	TeslaFC100OperationCommonPerformUpdate:   {},
	TeslaFC100OperationCommonFactoryReset:    {},
	TeslaFC100OperationCommonClearUpdate:     {},
	TeslaFC100OperationWCGetVitals:           {},
	TeslaFC100OperationWCGetLifetimeStats:    {},
	TeslaFC100OperationWCGetConfig:           {},
	TeslaFC100OperationWCGetSystemInfo:       {},
	TeslaFC100OperationWCGetLoadSharingState: {},
	TeslaFC100OperationWCGetPPU:              {},
	TeslaFC100OperationWCGetProvisional:      {},
	TeslaFC100OperationWCGetAccessControl:    {},
	TeslaFC100OperationWCGetRecentVehicles:   {},
	TeslaFC100OperationWCGetOperational:      {},
}

// ValidateTeslaFC100RequestMatrix validates an operation's exact empty or
// non-empty request form. Non-empty bodies retain all validated bytes and
// preserve unknown protobuf fields through RequestBody and Entries.
func ValidateTeslaFC100RequestMatrix(version string, operation TeslaFC100Operation, body []byte) (TeslaFC100RequestMatrix, error) {
	if version != TeslaHSCFC100OperationCompatibilityV1 {
		return TeslaFC100RequestMatrix{}, fmt.Errorf("tesla FC100 request matrix compatibility is unsupported")
	}
	if _, ok := teslaFC100OperationSpecs[operation]; !ok {
		return TeslaFC100RequestMatrix{}, fmt.Errorf("tesla FC100 request matrix operation is unsupported")
	}
	if _, err := BuildTeslaFC100OperationRequest(version, operation, body); err != nil {
		return TeslaFC100RequestMatrix{}, fmt.Errorf("tesla FC100 request matrix bound: %w", err)
	}
	if _, empty := teslaFC100EmptyRequestOperations[operation]; empty {
		if len(body) != 0 {
			return TeslaFC100RequestMatrix{}, fmt.Errorf("tesla FC100 fixed request has a body")
		}
		return TeslaFC100RequestMatrix{Operation: operation}, nil
	}
	if len(body) == 0 {
		return TeslaFC100RequestMatrix{}, fmt.Errorf("tesla FC100 structured request body is empty")
	}
	entries, err := decodeTeslaFC100TEDAPIWireEntries(body)
	if err != nil {
		return TeslaFC100RequestMatrix{}, fmt.Errorf("tesla FC100 structured request: %w", err)
	}
	switch operation {
	case TeslaFC100OperationCommonWifiScan:
		fields, err := decodeTeslaFC100RequestFields(body)
		if err != nil {
			return TeslaFC100RequestMatrix{}, err
		}
		if err := validateTeslaFC100WifiScanRequest(fields); err != nil {
			return TeslaFC100RequestMatrix{}, err
		}
	case TeslaFC100OperationCommonConfigureWifi:
		fields, err := decodeTeslaFC100RequestFields(body)
		if err != nil {
			return TeslaFC100RequestMatrix{}, err
		}
		if err := validateTeslaFC100ConfigureWifiRequest(fields); err != nil {
			return TeslaFC100RequestMatrix{}, err
		}
	}
	return TeslaFC100RequestMatrix{
		Operation:   operation,
		RequestBody: append([]byte(nil), body...),
		Entries:     append([]TeslaFC100WireEntry(nil), entries...),
	}, nil
}

// TeslaFC100DeterministicApplicationStatus identifies operations with a
// version-scoped normal FC100 application result instead of a terminal body.
func TeslaFC100DeterministicApplicationStatus(operation TeslaFC100Operation) (uint64, bool) {
	if operation == TeslaFC100OperationWCPushPPUAuthorization {
		return 7, true
	}
	return 0, false
}

type teslaFC100RequestField struct {
	number   uint64
	wireType uint8
	value    []byte
}

func decodeTeslaFC100RequestFields(body []byte) ([]teslaFC100RequestField, error) {
	fields := make([]teslaFC100RequestField, 0)
	for offset := 0; offset < len(body); {
		field, consumed, err := decodeTeslaFC100RequestField(body[offset:])
		if err != nil {
			return nil, fmt.Errorf("tesla FC100 structured request field: %w", err)
		}
		fields = append(fields, field)
		offset += consumed
	}
	return fields, nil
}

func decodeTeslaFC100RequestField(input []byte) (teslaFC100RequestField, int, error) {
	key, keyWidth, err := decodeTeslaFC100Varint(input)
	if err != nil {
		return teslaFC100RequestField{}, 0, err
	}
	field := key >> 3
	wireType := uint8(key & 0x07)
	if field == 0 || wireType > 5 {
		return teslaFC100RequestField{}, 0, fmt.Errorf("protobuf key is invalid")
	}
	offset := keyWidth
	result := teslaFC100RequestField{number: field, wireType: wireType}
	switch wireType {
	case 0:
		_, width, err := decodeTeslaFC100Varint(input[offset:])
		if err != nil {
			return teslaFC100RequestField{}, 0, err
		}
		offset += width
	case 1:
		if len(input)-offset < 8 {
			return teslaFC100RequestField{}, 0, fmt.Errorf("fixed64 value is truncated")
		}
		offset += 8
	case 2:
		length, width, err := decodeTeslaFC100Varint(input[offset:])
		if err != nil || length > uint64(len(input)-offset-width) {
			return teslaFC100RequestField{}, 0, fmt.Errorf("length-delimited value is truncated")
		}
		offset += width
		result.value = append([]byte(nil), input[offset:offset+int(length)]...)
		offset += int(length)
	case 3:
		width, err := skipTeslaFC100RequestGroup(input[offset:], field)
		if err != nil {
			return teslaFC100RequestField{}, 0, err
		}
		offset += width
	case 4:
		return teslaFC100RequestField{}, 0, fmt.Errorf("unexpected protobuf end group")
	case 5:
		if len(input)-offset < 4 {
			return teslaFC100RequestField{}, 0, fmt.Errorf("fixed32 value is truncated")
		}
		offset += 4
	}
	return result, offset, nil
}

func skipTeslaFC100RequestGroup(input []byte, groupField uint64) (int, error) {
	offset := 0
	for offset < len(input) {
		key, keyWidth, err := decodeTeslaFC100Varint(input[offset:])
		if err != nil {
			return 0, err
		}
		field := key >> 3
		wireType := uint8(key & 0x07)
		if field == 0 || wireType > 5 {
			return 0, fmt.Errorf("protobuf group key is invalid")
		}
		if wireType == 4 {
			if field != groupField {
				return 0, fmt.Errorf("protobuf group boundary is invalid")
			}
			return offset + keyWidth, nil
		}
		_, width, err := decodeTeslaFC100RequestField(input[offset:])
		if err != nil {
			return 0, err
		}
		offset += width
	}
	return 0, fmt.Errorf("protobuf group boundary is truncated")
}

func validateTeslaFC100WifiScanRequest(fields []teslaFC100RequestField) error {
	known := false
	for _, field := range fields {
		if field.number >= 1 && field.number <= 3 {
			if field.wireType != 0 {
				return fmt.Errorf("tesla FC100 Wi-Fi scan field %d has invalid wire type", field.number)
			}
			known = true
		}
	}
	if !known {
		return fmt.Errorf("tesla FC100 Wi-Fi scan lacks a known structural field")
	}
	return nil
}

func validateTeslaFC100ConfigureWifiRequest(fields []teslaFC100RequestField) error {
	known := false
	for _, field := range fields {
		switch field.number {
		case 1:
			if field.wireType != 0 {
				return fmt.Errorf("tesla FC100 configure Wi-Fi enabled has invalid wire type")
			}
			known = true
		case 2:
			if field.wireType != 2 {
				return fmt.Errorf("tesla FC100 configure Wi-Fi config has invalid wire type")
			}
			if _, err := decodeTeslaFC100RequestFields(field.value); err != nil {
				return fmt.Errorf("tesla FC100 configure Wi-Fi config: %w", err)
			}
			known = true
		}
	}
	if !known {
		return fmt.Errorf("tesla FC100 configure Wi-Fi lacks a known structural field")
	}
	return nil
}
