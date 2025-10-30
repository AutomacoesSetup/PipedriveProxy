package utils

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func FilterSliceByQuery(items interface{}, q url.Values) (interface{}, error) {
	val := reflect.ValueOf(items)
	if val.Kind() != reflect.Slice {
		if val.Kind() == reflect.Invalid {
			return items, nil
		}
		return nil, fmt.Errorf("items must be a slice, got %s", val.Kind())
	}

	if val.Len() == 0 || len(q) == 0 {
		return items, nil
	}

	out := reflect.MakeSlice(val.Type(), 0, 0)

	for i := 0; i < val.Len(); i++ {
		item := val.Index(i)
		if matchesAllReflect(item, q) {
			out = reflect.Append(out, item)
		}
	}
	return out.Interface(), nil
}

func matchesAllReflect(item reflect.Value, q url.Values) bool {
	val := item
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}
	typ := val.Type()

	jsonToIndex := map[string]int{}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		jsonTag := f.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		tagName := strings.Split(jsonTag, ",")[0]
		jsonToIndex[tagName] = i
	}

	for key, values := range q {
		fieldName := key
		exact := false
		if strings.HasSuffix(fieldName, "__exact") {
			exact = true
			fieldName = strings.TrimSuffix(fieldName, "__exact")
		}

		idx, ok := jsonToIndex[fieldName]
		if !ok {
			continue
		}
		fv := val.Field(idx)

		matched := false
		for _, v := range values {
			if matchField(fv, v, exact) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func matchField(fv reflect.Value, raw string, exact bool) bool {
	if fv.Kind() == reflect.Pointer && !fv.IsNil() {
		fv = fv.Elem()
	}

	switch fv.Kind() {
	case reflect.String:
		if exact {
			return strings.EqualFold(fv.String(), raw)
		}
		return strings.Contains(strings.ToLower(fv.String()), strings.ToLower(raw))

	case reflect.Bool:
		want, err := strconv.ParseBool(raw)
		if err != nil {
			return false
		}
		return fv.Bool() == want

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		want, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return false
		}
		if fv.Type().PkgPath() == "time" && fv.Type().Name() == "Duration" {
			return int64(fv.Int()) == want
		}
		return fv.Int() == want

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		want, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return false
		}
		return fv.Uint() == want

	case reflect.Float32, reflect.Float64:
		want, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return false
		}
		return fv.Float() == want

	case reflect.Struct:
		if fv.Type().PkgPath() == "time" && fv.Type().Name() == "Time" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return false
			}
			got := fv.Interface().(time.Time)
			return got.Equal(t)
		}
		return false
	default:
		return false
	}
}

func FilterFieldsByQuery(items interface{}, q url.Values) (interface{}, error) {
	fieldsStr := q.Get("fields")
	if fieldsStr == "" || strings.EqualFold(fieldsStr, "all") {
		// Retorna os dados originais se nenhum campo for especificado ou 'all' for solicitado
		return items, nil
	}

	// 1. Preparar lista de campos JSON solicitados
	requestedFields := strings.Split(fieldsStr, ",")
	fieldSet := make(map[string]struct{})
	for _, f := range requestedFields {
		trimmed := strings.TrimSpace(f)
		if trimmed != "" {
			fieldSet[trimmed] = struct{}{}
		}
	}

	val := reflect.ValueOf(items)
	if val.Kind() != reflect.Slice {
		return nil, fmt.Errorf("items must be a slice for field filtering, got %s", val.Kind())
	}

	outputSlice := make([]map[string]interface{}, 0, val.Len())

	// 2. Iterar sobre cada item da slice
	for i := 0; i < val.Len(); i++ {
		item := val.Index(i)
		if item.Kind() == reflect.Pointer {
			item = item.Elem()
		}

		itemMap := make(map[string]interface{})
		typ := item.Type()

		// 3. Iterar sobre os campos da struct Go
		for j := 0; j < typ.NumField(); j++ {
			field := typ.Field(j)
			jsonTag := field.Tag.Get("json")
			tagName := strings.Split(jsonTag, ",")[0]

			if tagName == "" || tagName == "-" {
				continue // Ignorar campos sem tag json
			}

			// 4. Se o campo foi solicitado, adicioná-lo ao map de saída
			if _, exists := fieldSet[tagName]; exists {
				fieldValue := item.Field(j).Interface()
				itemMap[tagName] = fieldValue
			}
		}
		outputSlice = append(outputSlice, itemMap)
	}

	return outputSlice, nil
}

func FilterMapSliceByFields(items []map[string]interface{}, fields string) ([]map[string]interface{}, error) {
	if fields == "" || strings.EqualFold(fields, "all") {
		return items, nil // Nenhuma filtragem necessária
	}

	fieldList := strings.Split(fields, ",")
	fieldMap := make(map[string]struct{})
	for _, f := range fieldList {
		fieldMap[strings.TrimSpace(f)] = struct{}{}
	}

	filteredItems := make([]map[string]interface{}, 0, len(items))

	for _, item := range items {
		filteredItem := make(map[string]interface{})
		for key, value := range item {
			if _, exists := fieldMap[key]; exists {
				filteredItem[key] = value
			}
		}
		filteredItems = append(filteredItems, filteredItem)
	}

	return filteredItems, nil
}
