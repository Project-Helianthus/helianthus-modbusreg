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
	TeslaFC100OperationWCGetVitals:           {},
	TeslaFC100OperationWCGetLifetimeStats:    {},
	TeslaFC100OperationWCGetSystemInfo:       {},
	TeslaFC100OperationWCGetLoadSharingState: {},
	TeslaFC100OperationWCGetProvisional:      {},
}

// ValidateTeslaFC100RequestMatrix validates an operation's exact empty or
// non-empty request form. Non-empty bodies retain all validated bytes and
// preserve unknown protobuf fields through RequestBody and Entries. This is
// structural validation only, not current-limit admission or send authority.
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
	return TeslaFC100RequestMatrix{
		Operation:   operation,
		RequestBody: append([]byte(nil), body...),
		Entries:     append([]TeslaFC100WireEntry(nil), entries...),
	}, nil
}

type teslaFC100RequestField struct {
	number   uint64
	wireType uint8
	value    []byte
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
