package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// compactJSON marshals v without HTML escaping and without a trailing newline.
func compactJSON(v any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

// encodeDocument writes the canonical form of a catalogue document: one
// top-level key per line, every element of an array of objects on its own
// line, and every other value compact. The form is a pure function of the
// value, so decoding and re-encoding a committed file reproduces its bytes.
func encodeDocument(document any) ([]byte, error) {
	value := reflect.ValueOf(document)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil, fmt.Errorf("document is not a struct")
	}
	var out bytes.Buffer
	out.WriteString("{\n")
	kind := value.Type()
	first := true
	for index := 0; index < kind.NumField(); index++ {
		field := kind.Field(index)
		name, omitEmpty := jsonFieldName(field)
		if name == "" {
			continue
		}
		fieldValue := value.Field(index)
		if omitEmpty && isEmptyValue(fieldValue) {
			continue
		}
		if !first {
			out.WriteString(",\n")
		}
		first = false
		key, err := compactJSON(name)
		if err != nil {
			return nil, err
		}
		out.WriteString("  ")
		out.Write(key)
		out.WriteString(": ")
		if fieldValue.Kind() == reflect.Slice && fieldValue.Type().Elem().Kind() == reflect.Struct {
			if fieldValue.Len() == 0 {
				out.WriteString("[]")
				continue
			}
			out.WriteString("[\n")
			for element := 0; element < fieldValue.Len(); element++ {
				encoded, err := compactJSON(fieldValue.Index(element).Interface())
				if err != nil {
					return nil, err
				}
				out.WriteString("    ")
				out.Write(encoded)
				if element < fieldValue.Len()-1 {
					out.WriteString(",")
				}
				out.WriteString("\n")
			}
			out.WriteString("  ]")
			continue
		}
		if fieldValue.Kind() == reflect.Slice && fieldValue.IsNil() {
			out.WriteString("[]")
			continue
		}
		encoded, err := compactJSON(fieldValue.Interface())
		if err != nil {
			return nil, err
		}
		out.Write(encoded)
	}
	out.WriteString("\n}\n")
	return out.Bytes(), nil
}

func jsonFieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "" || tag == "-" {
		return "", false
	}
	parts := strings.Split(tag, ",")
	omitEmpty := false
	for _, option := range parts[1:] {
		if option == "omitempty" {
			omitEmpty = true
		}
	}
	return parts[0], omitEmpty
}

func isEmptyValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Slice, reflect.Map, reflect.String:
		return value.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return value.IsNil()
	case reflect.Int:
		return value.Int() == 0
	case reflect.Bool:
		return !value.Bool()
	}
	return false
}

// decodeStrict decodes one catalogue document and refuses unknown fields and
// trailing data.
func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return fmt.Errorf("trailing data after the document")
	}
	return nil
}
