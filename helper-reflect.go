package helper

import (
	"database/sql"
	"reflect"
	"time"
)

/*
 * Copyright 2020 Aldelo, LP
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// ================================================================================================================
// Custom Struct Tag Reflect Helpers
// ================================================================================================================

// GetStructTagValueByObject will accept a struct object, struct field name, and struct tag name,
// and return the found tag value and reflect type,
// if reflect type or struct tag is not found, a notFound is returned
// [ Parameters ]
//		structObj = struct object variable
// 		structFieldName = struct's field name (CASE SENSITIVE)
//		structTagName = struct's tag name (the left side of struct tag - the key portion) (CASE SENSITIVE)
func GetStructTagValueByObject(structObj interface{}, structFieldName string, structTagName string) (notFound bool, tagValue string, t reflect.Type) {
	// get reflect type from struct object
	t = reflect.TypeOf(structObj)

	if t == nil {
		// no reflect type found
		return true, "", nil
	}

	// get field
	field, ok := t.FieldByName(structFieldName)

	if !ok {
		// struct field not found
		return true, "", t
	} else {
		// struct field found
		return false, field.Tag.Get(structTagName), t
	}
}

// GetStructTagValueByType will accept a prior obtained reflect type, struct field name, and struct tag name,
// and return the found tag value,
// if struct tag value is not found, a notFound is returned,
// if the reflect type is nil, then not found is returned too
// [ Parameters ]
//		t = reflect type of a struct object (obtained via GetStructTagValueByObject)
// 		structFieldName = struct's field name (CASE SENSITIVE)
//		structTagName = struct's tag name (the left side of struct tag - the key portion) (CASE SENSITIVE)
func GetStructTagValueByType(t reflect.Type, structFieldName string, structTagName string) (notFound bool, tagValue string) {
	// check if reflect type is valid
	if t == nil {
		return true, ""
	}

	// get field
	field, ok := t.FieldByName(structFieldName)

	if !ok {
		// struct field not found
		return true, ""
	} else {
		// struct field found
		return false, field.Tag.Get(structTagName)
	}
}

// ReflectCall uses reflection to invoke a method by name, and pass in param values if any,
// result is returned via reflect.Value object slice
func ReflectCall(o reflect.Value, methodName string, paramValue ...interface{}) (resultSlice []reflect.Value, notFound bool) {
	method := o.MethodByName(methodName)

	if !method.IsZero() {
		var params []reflect.Value

		if len(paramValue) > 0 {
			for _, p := range paramValue {
				params = append(params, reflect.ValueOf(p))
			}
		}

		resultSlice = method.Call(params)

		if len(resultSlice) == 0 {
			return nil, false
		} else {
			return resultSlice, false
		}
	} else {
		return nil, true
	}
}

// ReflectFieldValueToString accepts reflect.Value and returns its underlying field value in string data type
func ReflectFieldValueToString(o reflect.Value) (string, bool) {
	buf := ""

	switch o.Kind() {
	case reflect.String:
		buf = o.String()
	case reflect.Bool:
		if o.Bool() {
			buf = "true"
		} else {
			buf = "false"
		}
	case reflect.Int8:
		fallthrough
	case reflect.Int16:
		fallthrough
	case reflect.Int:
		fallthrough
	case reflect.Int32:
		fallthrough
	case reflect.Int64:
		buf = Int64ToString(o.Int())
	case reflect.Float32:
		fallthrough
	case reflect.Float64:
		buf = FloatToString(o.Float())
	case reflect.Uint8:
		fallthrough
	case reflect.Uint16:
		fallthrough
	case reflect.Uint:
		fallthrough
	case reflect.Uint32:
		fallthrough
	case reflect.Uint64:
		buf = UInt64ToString(o.Uint())
	case reflect.Ptr:
		o2 := o.Elem()
		switch f := o2.Interface().(type) {
		case int8:
			buf = Itoa(int(f))
		case int16:
			buf = Itoa(int(f))
		case int32:
			buf = Itoa(int(f))
		case int64:
			buf = Int64ToString(f)
		case int:
			buf = Itoa(f)
		case bool:
			buf = BoolToString(f)
		case string:
			buf = f
		case float32:
			buf = Float32ToString(f)
		case float64:
			buf = Float64ToString(f)
		case uint:
			buf = UintToStr(f)
		case uint64:
			buf = UInt64ToString(f)
		case time.Time:
			buf = FormatDateTime(f)
		default:
			return "", false
		}
	default:
		switch f := o.Interface().(type) {
		case sql.NullString:
			buf = FromNullString(f)
		case sql.NullBool:
			if FromNullBool(f) {
				buf = "true"
			} else {
				buf = "false"
			}
		case sql.NullFloat64:
			buf = FloatToString(FromNullFloat64(f))
		case sql.NullInt32:
			buf = Itoa(FromNullInt(f))
		case sql.NullInt64:
			buf = Int64ToString(FromNullInt64(f))
		case sql.NullTime:
			buf = FromNullTime(f).String()
		case time.Time:
			buf = f.String()
		default:
			return "", false
		}
	}

	return buf, true
}
