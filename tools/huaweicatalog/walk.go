package main

import (
	"reflect"
	"strconv"
)

// walkStrings calls visit for every string reachable from value, with a path
// built from JSON field names and slice indices.
func walkStrings(value any, visit func(path, text string)) {
	walkValue(reflect.ValueOf(value), "$", visit)
}

func walkValue(value reflect.Value, path string, visit func(path, text string)) {
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !value.IsNil() {
			walkValue(value.Elem(), path, visit)
		}
	case reflect.String:
		visit(path, value.String())
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			walkValue(value.Index(index), path+"["+strconv.Itoa(index)+"]", visit)
		}
	case reflect.Map:
		for _, key := range value.MapKeys() {
			walkValue(key, path+"{key}", visit)
			walkValue(value.MapIndex(key), path+"{"+key.String()+"}", visit)
		}
	case reflect.Struct:
		kind := value.Type()
		for index := 0; index < kind.NumField(); index++ {
			name, _ := jsonFieldName(kind.Field(index))
			if name == "" {
				name = kind.Field(index).Name
			}
			walkValue(value.Field(index), path+"."+name, visit)
		}
	}
}
