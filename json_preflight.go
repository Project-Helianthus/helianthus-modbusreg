package modbusreg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func preflightJSON(data []byte) error {
	if len(data) == 0 || len(data) > MaxSerializedContractBytes {
		return fmt.Errorf("serialized contract exceeds the byte boundary")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := scanJSONValue(
		decoder,
		1,
		MaxProfileDependencies,
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

func scanJSONValue(
	decoder *json.Decoder,
	depth int,
	arrayLimit int,
) error {
	if depth > MaxContractJSONDepth {
		return fmt.Errorf("serialized contract exceeds the nesting boundary")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			return scanJSONObject(decoder, depth)
		case '[':
			return scanJSONArray(decoder, depth, arrayLimit)
		default:
			return fmt.Errorf("serialized contract has an unexpected delimiter")
		}
	case string:
		return validateBoundedString("serialized string", value, false)
	case json.Number:
		if len(value.String()) > MaxContractStringBytes {
			return fmt.Errorf("serialized number exceeds the scalar boundary")
		}
	case bool, nil:
	default:
		return fmt.Errorf("serialized contract has an unknown token")
	}
	return nil
}

func scanJSONObject(decoder *json.Decoder, depth int) error {
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
		if err := scanJSONValue(
			decoder,
			depth+1,
			jsonArrayLimit(key),
		); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	if closing != json.Delim('}') {
		return fmt.Errorf("serialized object is not closed")
	}
	return nil
}

func scanJSONArray(
	decoder *json.Decoder,
	depth int,
	limit int,
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
	default:
		return MaxProfileDependencies
	}
}
