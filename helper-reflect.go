package helper

import (
	"database/sql"
	"fmt"
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

// GetStructTagsValueSlice returns named struct tag values from field, in the order queried
func GetStructTagsValueSlice(field reflect.StructField, tagName ...string) (tagValues []string) {
	for _, t := range tagName {
		tagValues = append(tagValues, Trim(field.Tag.Get(t)))
	}

	return
}

// ReflectCall uses reflection to invoke a method by name, and pass in param values if any,
// result is returned via reflect.Value object slice
func ReflectCall(o reflect.Value, methodName string, paramValue ...interface{}) (resultSlice []reflect.Value, notFound bool) {
	if o.IsZero() {
		return nil, true
	}

	method := o.MethodByName(methodName)

	if method.Kind() == reflect.Invalid {
		return nil, true
	}

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

// ReflectValueToString accepts reflect.Value and returns its underlying field value in string data type
// boolTrue is the literal value to use for bool true condition, boolFalse is the false condition literal,
// if boolTrue or boolFalse is not defined, then default 'true' or 'false' is used,
// skipBlank and skipZero if true indicates if field value is blank (string) or Zero (int, float, time, pointer, bool) then skip render
//
// timeFormat:
// 		2006, 06 = year,
//		01, 1, Jan, January = month,
//		02, 2, _2 = day (_2 = width two, right justified)
//		03, 3, 15 = hour (15 = 24 hour format)
//		04, 4 = minute
//		05, 5 = second
//		PM pm = AM PM
func ReflectValueToString(o reflect.Value, boolTrue string, boolFalse string, skipBlank bool, skipZero bool, timeFormat string) (valueStr string, skip bool, err error) {
	buf := ""

	switch o.Kind() {
	case reflect.String:
		buf = o.String()

		if skipBlank && LenTrim(buf) == 0 {
			return "", true, nil
		}
	case reflect.Bool:
		if o.Bool() {
			if LenTrim(boolTrue) == 0 {
				buf = "true"
			} else {
				buf = boolTrue
			}
		} else {
			if skipZero {
				return "", true, nil
			} else {
				if LenTrim(boolFalse) == 0 {
					buf = "false"
				} else {
					buf = boolFalse
				}
			}
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
		if skipZero && o.Int() == 0 {
			return "", true, nil
		} else {
			buf = Int64ToString(o.Int())
		}
	case reflect.Float32:
		fallthrough
	case reflect.Float64:
		if skipZero && o.Float() == 0.00 {
			return "", true, nil
		} else {
			buf = FloatToString(o.Float())
		}
	case reflect.Uint8:
		fallthrough
	case reflect.Uint16:
		fallthrough
	case reflect.Uint:
		fallthrough
	case reflect.Uint32:
		fallthrough
	case reflect.Uint64:
		if skipZero && o.Uint() == 0 {
			return "", true, nil
		} else {
			buf = UInt64ToString(o.Uint())
		}
	case reflect.Ptr:
		if o.IsZero() || o.IsNil() {
			if skipZero || skipBlank {
				return "", true, nil
			} else {
				return "", false, nil
			}
		}

		o2 := o.Elem()

		if o2.IsZero() {
			if skipZero || skipBlank {
				return "", true, nil
			} else {
				return "", false, nil
			}
		}

		switch f := o2.Interface().(type) {
		case int8:
			if skipZero && f == 0 {
				return "", true, nil
			} else {
				buf = Itoa(int(f))
			}
		case int16:
			if skipZero && f == 0 {
				return "", true, nil
			} else {
				buf = Itoa(int(f))
			}
		case int32:
			if skipZero && f == 0 {
				return "", true, nil
			} else {
				buf = Itoa(int(f))
			}
		case int64:
			if skipZero && f == 0 {
				return "", true, nil
			} else {
				buf = Int64ToString(f)
			}
		case int:
			if skipZero && f == 0 {
				return "", true, nil
			} else {
				buf = Itoa(f)
			}
		case bool:
			if f {
				if LenTrim(boolTrue) == 0 {
					buf = "true"
				} else {
					buf = boolTrue
				}
			} else {
				if skipZero {
					return "", true, nil
				} else {
					if LenTrim(boolFalse) == 0 {
						buf = "false"
					} else {
						buf = boolFalse
					}
				}
			}
		case string:
			if skipBlank && LenTrim(f) == 0 {
				return "", true, nil
			} else {
				buf = f
			}
		case float32:
			if skipZero && f == 0.00 {
				return "", true, nil
			} else {
				buf = Float32ToString(f)
			}
		case float64:
			if skipZero && f == 0.00 {
				return "", true, nil
			} else {
				buf = Float64ToString(f)
			}
		case uint:
			if skipZero && f == 0 {
				return "", true, nil
			} else {
				buf = UintToStr(f)
			}
		case uint64:
			if skipZero && f == 0 {
				return "", true, nil
			} else {
				buf = UInt64ToString(f)
			}
		case time.Time:
			if skipZero && f.IsZero() {
				return "", true, nil
			} else {
				if LenTrim(timeFormat) == 0 {
					buf = FormatDateTime(f)
				} else {
					buf = f.Format(timeFormat)
				}
			}
		default:
			return "", false, fmt.Errorf("%s Unhandled", o2.Type().Name())
		}
	default:
		switch f := o.Interface().(type) {
		case sql.NullString:
			buf = FromNullString(f)

			if skipBlank && LenTrim(buf) == 0 {
				return "", true, nil
			}
		case sql.NullBool:
			if FromNullBool(f) {
				if LenTrim(boolTrue) == 0 {
					buf = "true"
				} else {
					buf = boolTrue
				}
			} else {
				if skipZero {
					return "", true, nil
				} else {
					if LenTrim(boolFalse) == 0 {
						buf = "false"
					} else {
						buf = boolFalse
					}
				}
			}
		case sql.NullFloat64:
			f64 := FromNullFloat64(f)

			if skipZero && f64 == 0.00 {
				return "", true, nil
			} else {
				buf = FloatToString(f64)
			}
		case sql.NullInt32:
			i32 := FromNullInt(f)

			if skipZero && i32 == 0 {
				return "", true, nil
			} else {
				buf = Itoa(i32)
			}
		case sql.NullInt64:
			i64 := FromNullInt64(f)

			if skipZero && i64 == 0 {
				return "", true, nil
			} else {
				buf = Int64ToString(i64)
			}
		case sql.NullTime:
			t := FromNullTime(f)

			if skipZero && t.IsZero() {
				return "", true, nil
			} else {
				if LenTrim(timeFormat) == 0 {
					buf = FormatDateTime(t)
				} else {
					buf = t.Format(timeFormat)
				}
			}
		case time.Time:
			if skipZero && f.IsZero() {
				return "", true, nil
			} else {
				if LenTrim(timeFormat) == 0 {
					buf = FormatDateTime(f)
				} else {
					buf = f.Format(timeFormat)
				}
			}
		default:
			return "", false, fmt.Errorf("%s Unhandled", o.Type().Name())
		}
	}

	return buf, false, nil
}

// ReflectStringToField accepts string value and reflects into reflect.Value field based on the field data type
//
// timeFormat:
// 		2006, 06 = year,
//		01, 1, Jan, January = month,
//		02, 2, _2 = day (_2 = width two, right justified)
//		03, 3, 15 = hour (15 = 24 hour format)
//		04, 4 = minute
//		05, 5 = second
//		PM pm = AM PM
func ReflectStringToField(o reflect.Value, v string, timeFormat string) error {
	switch o.Kind() {
	case reflect.String:
		o.SetString(v)
	case reflect.Bool:
		o.SetBool(IsBool(v))
	case reflect.Int8:
		fallthrough
	case reflect.Int16:
		fallthrough
	case reflect.Int:
		fallthrough
	case reflect.Int32:
		fallthrough
	case reflect.Int64:
		i64, _ := ParseInt64(v)
		if !o.OverflowInt(i64) {
			o.SetInt(i64)
		}
	case reflect.Float32:
		fallthrough
	case reflect.Float64:
		f64, _ := ParseFloat64(v)
		if !o.OverflowFloat(f64) {
			o.SetFloat(f64)
		}
	case reflect.Uint8:
		fallthrough
	case reflect.Uint16:
		fallthrough
	case reflect.Uint:
		fallthrough
	case reflect.Uint32:
		fallthrough
	case reflect.Uint64:
		ui64 := StrToUint64(v)
		if !o.OverflowUint(ui64) {
			o.SetUint(ui64)
		}
	case reflect.Ptr:
		if o.IsZero() || o.IsNil() {
			// create object
			baseType, _, _ := DerefPointersZero(o)
			o.Set(reflect.New(baseType.Type()))
		}

		o2 := o.Elem()

		if o.IsZero() {
			return nil
		}

		switch o2.Interface().(type) {
		case int:
			i64, _ := ParseInt64(v)
			if !o2.OverflowInt(i64) {
				o2.SetInt(i64)
			}
		case int8:
			i64, _ := ParseInt64(v)
			if !o2.OverflowInt(i64) {
				o2.SetInt(i64)
			}
		case int16:
			i64, _ := ParseInt64(v)
			if !o2.OverflowInt(i64) {
				o2.SetInt(i64)
			}
		case int32:
			i64, _ := ParseInt64(v)
			if !o2.OverflowInt(i64) {
				o2.SetInt(i64)
			}
		case int64:
			i64, _ := ParseInt64(v)
			if !o2.OverflowInt(i64) {
				o2.SetInt(i64)
			}
		case float32:
			f64, _ := ParseFloat64(v)
			if !o2.OverflowFloat(f64) {
				o2.SetFloat(f64)
			}
		case float64:
			f64, _ := ParseFloat64(v)
			if !o2.OverflowFloat(f64) {
				o2.SetFloat(f64)
			}
		case uint:
			if !o2.OverflowUint(StrToUint64(v)) {
				o2.SetUint(StrToUint64(v))
			}
		case uint64:
			if !o2.OverflowUint(StrToUint64(v)) {
				o2.SetUint(StrToUint64(v))
			}
		case string:
			o2.SetString(v)
		case bool:
			o2.SetBool(IsBool(v))
		case time.Time:
			if LenTrim(timeFormat) == 0 {
				o2.Set(reflect.ValueOf(ParseDate(v)))
			} else {
				o2.Set(reflect.ValueOf(ParseDateTimeCustom(v, timeFormat)))
			}
		default:
			return fmt.Errorf(o2.Type().Name() + " Unhandled")
		}
	default:
		switch o.Interface().(type) {
		case sql.NullString:
			o.Set(reflect.ValueOf(sql.NullString{String: v, Valid: true}))
		case sql.NullBool:
			o.Set(reflect.ValueOf(sql.NullBool{Bool: IsBool(v), Valid: true}))
		case sql.NullFloat64:
			f64, _ := ParseFloat64(v)
			o.Set(reflect.ValueOf(sql.NullFloat64{Float64: f64, Valid: true}))
		case sql.NullInt32:
			i32, _ := ParseInt32(v)
			o.Set(reflect.ValueOf(sql.NullInt32{Int32: int32(i32), Valid: true}))
		case sql.NullInt64:
			i64, _ := ParseInt64(v)
			o.Set(reflect.ValueOf(sql.NullInt64{Int64: i64, Valid: true}))
		case sql.NullTime:
			var tv time.Time

			if LenTrim(timeFormat) == 0 {
				tv = ParseDateTime(v)
			} else {
				tv = ParseDateTimeCustom(v, timeFormat)
			}

			o.Set(reflect.ValueOf(sql.NullTime{Time: tv, Valid: true}))
		case time.Time:
			if LenTrim(timeFormat) == 0 {
				o.Set(reflect.ValueOf(ParseDateTime(v)))
			} else {
				o.Set(reflect.ValueOf(ParseDateTimeCustom(v, timeFormat)))
			}
		default:
			return fmt.Errorf(o.Type().Name() + " Unhandled")
		}
	}

	return nil
}

// Get pointer base type
func DerefPointersZero(rv reflect.Value) (drv reflect.Value, isPtr bool, isNilPtr bool) {
	for rv.Kind() == reflect.Ptr {
		isPtr = true
		if rv.IsNil() {
			isNilPtr = true
			rt := rv.Type().Elem()
			for rt.Kind() == reflect.Ptr {
				rt = rt.Elem()
			}
			drv = reflect.New(rt).Elem()
			return
		}
		rv = rv.Elem()
	}
	drv = rv
	return
}

// Deference reflect.Value to error object if underlying type was error
func DerefError(v reflect.Value) error {
	if e, ok := v.Interface().(error); ok {
		// v is error, check if error exists
		if e != nil {
			return e
		}
	}

	return nil
}