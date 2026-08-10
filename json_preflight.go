package modbusreg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"
)

var versionValueType = reflect.TypeOf(Version{})

type jsonField struct {
	valueType reflect.Type
	required  bool
}

func preflightJSON(data []byte, target any) error {
	if len(data) == 0 || len(data) > MaxSerializedContractBytes {
		return fmt.Errorf("serialized contract exceeds the byte boundary")
	}
	if !utf8.Valid(data) {
		return fmt.Errorf("serialized contract is not valid UTF-8")
	}
	if err := validateJSONSurrogates(data); err != nil {
		return err
	}
	expected := reflect.TypeOf(target)
	if expected == nil || expected.Kind() != reflect.Pointer {
		return fmt.Errorf("serialized contract target is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := scanJSONValue(
		decoder,
		1,
		MaxProfileDependencies,
		expected.Elem(),
	); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("serialized contract contains multiple values")
		}
		return err
	}
	return nil
}

func validateJSONSurrogates(data []byte) error {
	for index := 0; index < len(data); index++ {
		if data[index] != '"' {
			continue
		}
		for index++; index < len(data) && data[index] != '"'; index++ {
			if data[index] != '\\' {
				continue
			}
			index++
			if index >= len(data) {
				return fmt.Errorf("serialized string has an incomplete escape")
			}
			if data[index] != 'u' {
				continue
			}
			value, ok := parseHexQuad(data, index+1)
			if !ok {
				return fmt.Errorf("serialized string has an invalid Unicode escape")
			}
			index += 4
			switch {
			case value >= 0xd800 && value <= 0xdbff:
				if index+6 >= len(data) ||
					data[index+1] != '\\' || data[index+2] != 'u' {
					return fmt.Errorf("serialized string has an unpaired high surrogate")
				}
				low, valid := parseHexQuad(data, index+3)
				if !valid || low < 0xdc00 || low > 0xdfff {
					return fmt.Errorf("serialized string has an unpaired high surrogate")
				}
				index += 6
			case value >= 0xdc00 && value <= 0xdfff:
				return fmt.Errorf("serialized string has an unpaired low surrogate")
			}
		}
	}
	return nil
}

func parseHexQuad(data []byte, start int) (uint16, bool) {
	if start < 0 || start+4 > len(data) {
		return 0, false
	}
	var value uint16
	for _, character := range data[start : start+4] {
		value <<= 4
		switch {
		case character >= '0' && character <= '9':
			value += uint16(character - '0')
		case character >= 'a' && character <= 'f':
			value += uint16(character-'a') + 10
		case character >= 'A' && character <= 'F':
			value += uint16(character-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func indirectJSONType(value reflect.Type) reflect.Type {
	for value != nil && value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	return value
}

func scanJSONValue(
	decoder *json.Decoder,
	depth int,
	arrayLimit int,
	expected reflect.Type,
) error {
	if depth > MaxContractJSONDepth {
		return fmt.Errorf("serialized contract exceeds the nesting boundary")
	}
	expected = indirectJSONType(expected)
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			if expected == nil || expected.Kind() != reflect.Struct ||
				expected == versionValueType {
				return fmt.Errorf("serialized object has an incompatible shape")
			}
			return scanJSONObject(decoder, depth, expected)
		case '[':
			if expected == nil ||
				(expected.Kind() != reflect.Slice && expected.Kind() != reflect.Array) {
				return fmt.Errorf("serialized array has an incompatible shape")
			}
			return scanJSONArray(
				decoder,
				depth,
				arrayLimit,
				expected.Elem(),
			)
		default:
			return fmt.Errorf("serialized contract has an unexpected delimiter")
		}
	case string:
		if err := validateBoundedString("serialized string", value, false); err != nil {
			return err
		}
	case json.Number:
		if len(value.String()) > MaxContractStringBytes {
			return fmt.Errorf("serialized number exceeds the scalar boundary")
		}
	case bool:
	case nil:
		return fmt.Errorf("serialized contract contains a non-canonical null")
	default:
		return fmt.Errorf("serialized contract has an unknown token")
	}
	if expected != nil && expected.Kind() == reflect.Struct &&
		expected != versionValueType {
		return fmt.Errorf("serialized scalar has an incompatible shape")
	}
	return nil
}

func scanJSONObject(
	decoder *json.Decoder,
	depth int,
	expected reflect.Type,
) error {
	fields := jsonFields(expected)
	seen := make(map[string]struct{})
	count := 0
	for decoder.More() {
		count++
		if count > MaxProfileDependencies {
			return fmt.Errorf("serialized object exceeds the member boundary")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return fmt.Errorf("serialized object key is not a string")
		}
		if err := validateBoundedString("serialized object key", key, true); err != nil {
			return err
		}
		canonical := strings.ToLower(key)
		if _, exists := seen[canonical]; exists {
			return fmt.Errorf("serialized object has a duplicate or case-folded key")
		}
		seen[canonical] = struct{}{}
		field, exists := fields[key]
		if !exists {
			return fmt.Errorf("serialized object key %q is not canonical", key)
		}
		if err := scanJSONValue(
			decoder,
			depth+1,
			jsonArrayLimit(key),
			field.valueType,
		); err != nil {
			return fmt.Errorf("serialized key %q: %w", key, err)
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	if closing != json.Delim('}') {
		return fmt.Errorf("serialized object is not closed")
	}
	for _, key := range requiredJSONFieldNames(expected) {
		if _, exists := seen[strings.ToLower(key)]; !exists {
			return fmt.Errorf("serialized object is missing required key %q", key)
		}
	}
	return nil
}

func requiredJSONFieldNames(value reflect.Type) []string {
	result := make([]string, 0, value.NumField())
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		if field.PkgPath != "" {
			continue
		}
		name := field.Name
		required := true
		if tag, exists := field.Tag.Lookup("json"); exists {
			parts := strings.Split(tag, ",")
			if parts[0] == "-" {
				continue
			}
			if parts[0] != "" {
				name = parts[0]
			}
			for _, option := range parts[1:] {
				if option == "omitempty" {
					required = false
				}
			}
		}
		if required {
			result = append(result, name)
		}
	}
	return result
}

func jsonFields(value reflect.Type) map[string]jsonField {
	result := make(map[string]jsonField)
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		if field.PkgPath != "" {
			continue
		}
		name := field.Name
		required := true
		if tag, exists := field.Tag.Lookup("json"); exists {
			parts := strings.Split(tag, ",")
			tagName := parts[0]
			if tagName == "-" {
				continue
			}
			if tagName != "" {
				name = tagName
			}
			for _, option := range parts[1:] {
				if option == "omitempty" {
					required = false
				}
			}
		}
		result[name] = jsonField{valueType: field.Type, required: required}
	}
	return result
}

func scanJSONArray(
	decoder *json.Decoder,
	depth int,
	limit int,
	element reflect.Type,
) error {
	count := 0
	for decoder.More() {
		count++
		if count > limit {
			return fmt.Errorf("serialized array exceeds the item boundary")
		}
		if err := scanJSONValue(
			decoder,
			depth+1,
			MaxProfileDependencies,
			element,
		); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	if closing != json.Delim(']') {
		return fmt.Errorf("serialized array is not closed")
	}
	return nil
}

func jsonArrayLimit(key string) int {
	switch strings.ToLower(key) {
	case "words", "wordpermutation":
		return MaxRawWords
	case "sentinels":
		return MaxCodecSentinels
	case "samples":
		return MaxSampleLedgerRecords
	case "audit_tombstones":
		return maxLedgerRestartAuditTombstones
	default:
		return MaxProfileDependencies
	}
}
