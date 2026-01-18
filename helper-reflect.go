package helper

import (
	"database/sql"
	"fmt"
	"reflect"
	"sync"
	"time"
)

/*
 * Copyright 2020-2026 Aldelo, LP
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
// Custom Type Registry
// ================================================================================================================
var (
	customTypeRegistry   map[string]reflect.Type
	customTypeRegistryMu sync.RWMutex // mutex for concurrent access
)

// ReflectTypeRegistryAdd will accept a custom struct object, and add its type into custom type registry,
// if customFullTypeName is not specified, the type name is inferred from the type itself,
// custom type registry is used by reflect unmarshal helpers to construct custom type for undefined interface targets
func ReflectTypeRegistryAdd(customStructObj interface{}, customFullTypeName ...string) bool {
	if customStructObj == nil {
		return false
	}

	o := reflect.TypeOf(customStructObj)
	if o.Kind() == reflect.Ptr {
		o = o.Elem()
	}
	if o.Kind() != reflect.Struct {
		return false
	}

	typeName := o.Name()

	if len(customFullTypeName) > 0 {
		if LenTrim(customFullTypeName[0]) > 0 {
			typeName = Trim(customFullTypeName[0])
		}
	}

	customTypeRegistryMu.Lock()
	defer customTypeRegistryMu.Unlock()

	if customTypeRegistry == nil {
		customTypeRegistry = make(map[string]reflect.Type)
	}

	customTypeRegistry[typeName] = o
	return true
}

// ReflectTypeRegistryRemove will remove a pre-registered custom type from type registry for the given type name
func ReflectTypeRegistryRemove(customFullTypeName string) {
	customTypeRegistryMu.Lock()
	defer customTypeRegistryMu.Unlock()

	if customTypeRegistry != nil {
		delete(customTypeRegistry, customFullTypeName)
	}
}

// ReflectTypeRegistryRemoveAll will clear all previously registered custom types from type registry
func ReflectTypeRegistryRemoveAll() {
	customTypeRegistryMu.Lock()
	defer customTypeRegistryMu.Unlock()

	if customTypeRegistry != nil {
		customTypeRegistry = make(map[string]reflect.Type)
	}
}

// ReflectTypeRegistryCount returns count of custom types registered in the type registry
func ReflectTypeRegistryCount() int {
	customTypeRegistryMu.RLock()
	defer customTypeRegistryMu.RUnlock()

	if customTypeRegistry != nil {
		return len(customTypeRegistry)
	} else {
		return 0
	}
}

// ReflectTypeRegistryGet returns a previously registered custom type in the type registry, based on the given type name string
func ReflectTypeRegistryGet(customFullTypeName string) reflect.Type {
	customTypeRegistryMu.RLock()
	defer customTypeRegistryMu.RUnlock()

	if customTypeRegistry != nil {
		if t, ok := customTypeRegistry[customFullTypeName]; ok {
			return t
		} else {
			return nil
		}
	} else {
		return nil
	}
}

// ================================================================================================================
// Custom Struct Tag Reflect Helpers
// ================================================================================================================

// GetStructTagValueByObject will accept a struct object, struct field name, and struct tag name,
// and return the found tag value and reflect type,
// if reflect type or struct tag is not found, a notFound is returned
// [ Parameters ]
//
//	structObj = struct object variable
//	structFieldName = struct's field name (CASE SENSITIVE)
//	structTagName = struct's tag name (the left side of struct tag - the key portion) (CASE SENSITIVE)
func GetStructTagValueByObject(structObj interface{}, structFieldName string, structTagName string) (notFound bool, tagValue string, t reflect.Type) {
	// get reflect type from struct object
	t = reflect.TypeOf(structObj)
	if t == nil {
		// no reflect type found
		return true, "", nil
	}

	// dereference pointer-to-struct inputs
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return true, "", nil
	}

	// get field
	field, ok := t.FieldByName(structFieldName)

	if !ok {
		// struct field not found
		return true, "", t
	} else {
		// struct field found
		return false, Replace(field.Tag.Get(structTagName), ",omitempty", ""), t
	}
}

// GetStructTagValueByType will accept a prior obtained reflect type, struct field name, and struct tag name,
// and return the found tag value,
// if struct tag value is not found, a notFound is returned,
// if the reflect type is nil, then not found is returned too
// [ Parameters ]
//
//	t = reflect type of a struct object (obtained via GetStructTagValueByObject)
//	structFieldName = struct's field name (CASE SENSITIVE)
//	structTagName = struct's tag name (the left side of struct tag - the key portion) (CASE SENSITIVE)
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
		return false, Replace(field.Tag.Get(structTagName), ",omitempty", "")
	}
}

// GetStructTagsValueSlice returns named struct tag values from field, in the order queried
func GetStructTagsValueSlice(field reflect.StructField, tagName ...string) (tagValues []string) {
	for _, t := range tagName {
		tagValues = append(tagValues, Replace(field.Tag.Get(t), ",omitempty", ""))
	}

	return
}

type StructFieldTagAndValue struct {
	FieldName  string
	FieldValue string

	TagName  string
	TagValue string

	DynamoDBAttributeTagName string
}

// GetStructFieldTagAndValues extracts values of a specific struct tag and their corresponding field values
// from the given struct object. Returns a slice of StructFieldTagAndValue or an error if the input is not a struct.
//
// input = represents the struct object
// tagName = represents the tag name to extract
func GetStructFieldTagAndValues(input interface{}, tagName string, getDynamoDBAttributeTagName bool) ([]*StructFieldTagAndValue, error) {
	// validate
	if LenTrim(tagName) <= 0 {
		return nil, fmt.Errorf("Struct Tag Name is Required")
	}

	// Get the reflect.Value and reflect.Type of the input
	v := reflect.ValueOf(input)
	t := reflect.TypeOf(input)

	if t == nil {
		return nil, fmt.Errorf("Struct Input Object is Required (Currently Nil)")
	}

	// Ensure the input is a struct or a pointer to a struct
	if t.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil, fmt.Errorf("Struct Input Object is Required (Currently Nil Pointer)")
		}
		v = v.Elem()
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("Struct Input Object Expects a Struct Object or Pointer to Struct Object")
	}

	results := make([]*StructFieldTagAndValue, 0)

	// Iterate over the fields of the struct
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// skip unexported fields to avoid reflect panics and to respect visibility
		if field.PkgPath != "" { // unexported
			continue
		}

		value := v.Field(i)

		valueStr, skip, err := ReflectValueToString(value, "true", "false", false, false, "", false)
		if err != nil {
			return nil, fmt.Errorf("field %q value conversion failed: %w", field.Name, err)
		}
		if skip {
			continue
		}

		// Get the value of the specified tag
		if tagValue := Replace(field.Tag.Get(tagName), ",omitempty", ""); LenTrim(tagValue) > 0 && tagValue != "-" {
			// get dynamodbav attribute tag value
			ddbTagName := ""

			if getDynamoDBAttributeTagName {
				ddbTagName = Replace(field.Tag.Get("dynamodbav"), ",omitempty", "")
			}

			// Add the tag value and field value to the results
			results = append(results, &StructFieldTagAndValue{
				FieldName:                field.Name,
				FieldValue:               valueStr,
				TagName:                  tagName,
				TagValue:                 tagValue,
				DynamoDBAttributeTagName: ddbTagName,
			})
		}
	}

	return results, nil
}

// ================================================================================================================
// Reflection Helpers
// ================================================================================================================

// ReflectCall uses reflection to invoke a method by name, and pass in param values if any,
// result is returned via reflect.Value object slice
func ReflectCall(o reflect.Value, methodName string, paramValue ...interface{}) (resultSlice []reflect.Value, notFound bool) {
	method := o.MethodByName(methodName)

	// guard invalid method to avoid panic on Kind()
	if !method.IsValid() {
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
// skipBlank and skipZero if true indicates if field value is blank (string) or Zero (int, float, time, pointer, bool) then skip render,
// zeroBlank = will blank the value if it is 0, 0.00, or time.IsZero
//
// timeFormat:
//
//	2006, 06 = year,
//	01, 1, Jan, January = month,
//	02, 2, _2 = day (_2 = width two, right justified)
//	03, 3, 15 = hour (15 = 24 hour format)
//	04, 4 = minute
//	05, 5 = second
//	PM pm = AM PM
func ReflectValueToString(o reflect.Value, boolTrue string, boolFalse string, skipBlank bool, skipZero bool, timeFormat string, zeroBlank bool) (valueStr string, skip bool, err error) {
	// handle invalid values and unexported fields safely
	if !o.IsValid() {
		return "", true, fmt.Errorf("invalid reflect.Value")
	}
	if !o.CanInterface() {
		return "", true, fmt.Errorf("unexported or inaccessible field")
	}

	buf := ""

	switch o.Kind() {
	case reflect.String:
		buf = o.String()

		if skipBlank && LenTrim(buf) == 0 {
			return "", true, nil
		}
	case reflect.Bool:
		if o.Bool() {
			if len(boolTrue) == 0 {
				buf = "true"
			} else {
				buf = Trim(boolTrue)
			}
		} else {
			if skipZero {
				return "", true, nil
			} else {
				if len(boolFalse) == 0 {
					buf = "false"
				} else {
					if Trim(boolTrue) == Trim(boolFalse) {
						buf = "false"
					} else {
						buf = Trim(boolFalse)
					}
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
			if zeroBlank && o.Int() == 0 {
				buf = ""
			} else {
				buf = Int64ToString(o.Int())
			}
		}
	case reflect.Float32:
		fallthrough
	case reflect.Float64:
		if skipZero && o.Float() == 0.00 {
			return "", true, nil
		} else {
			if zeroBlank && o.Float() == 0.00 {
				buf = ""
			} else {
				buf = FloatToString(o.Float())
			}
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
			if zeroBlank && o.Uint() == 0 {
				buf = ""
			} else {
				buf = UInt64ToString(o.Uint())
			}
		}
	case reflect.Ptr:
		if o.IsZero() || o.IsNil() {
			if skipZero || skipBlank {
				return "", true, nil
			} else {
				if rt, _, _ := DerefPointersZero(o); rt.Kind() == reflect.Bool {
					if Trim(boolTrue) == Trim(boolFalse) {
						return "false", false, nil
					} else {
						if LenTrim(boolFalse) > 0 {
							return boolFalse, false, nil
						} else {
							return "", false, nil
						}
					}
				} else {
					return "", false, nil
				}
			}
		}

		o2 := o.Elem()

		if o2.IsZero() {
			if skipZero || skipBlank {
				return "", true, nil
			}
		}

		switch f := o2.Interface().(type) {
		case int8:
			if skipZero && f == 0 {
				return "", true, nil
			} else {
				if zeroBlank && f == 0 {
					buf = ""
				} else {
					buf = Itoa(int(f))
				}
			}
		case int16:
			if skipZero && f == 0 {
				return "", true, nil
			} else {
				if zeroBlank && f == 0 {
					buf = ""
				} else {
					buf = Itoa(int(f))
				}
			}
		case int32:
			if skipZero && f == 0 {
				return "", true, nil
			} else {
				if zeroBlank && f == 0 {
					buf = ""
				} else {
					buf = Itoa(int(f))
				}
			}
		case int64:
			if skipZero && f == 0 {
				return "", true, nil
			} else {
				if zeroBlank && f == 0 {
					buf = ""
				} else {
					buf = Int64ToString(f)
				}
			}
		case int:
			if skipZero && f == 0 {
				return "", true, nil
			} else {
				if zeroBlank && f == 0 {
					buf = ""
				} else {
					buf = Itoa(f)
				}
			}
		case bool:
			if f {
				if len(boolTrue) == 0 {
					buf = "true"
				} else {
					buf = Trim(boolTrue)
				}
			} else {
				if skipZero {
					return "", true, nil
				} else {
					if len(boolFalse) == 0 {
						buf = "false"
					} else {
						if Trim(boolTrue) == Trim(boolFalse) {
							buf = "false"
						} else {
							buf = Trim(boolFalse)
						}
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
				if zeroBlank && f == 0.00 {
					buf = ""
				} else {
					buf = Float32ToString(f)
				}
			}
		case float64:
			if skipZero && f == 0.00 {
				return "", true, nil
			} else {
				if zeroBlank && f == 0.00 {
					buf = ""
				} else {
					buf = Float64ToString(f)
				}
			}
		case uint:
			if skipZero && f == 0 {
				return "", true, nil
			} else {
				if zeroBlank && f == 0.00 {
					buf = ""
				} else {
					buf = UintToStr(f)
				}
			}
		case uint64:
			if skipZero && f == 0 {
				return "", true, nil
			} else {
				if zeroBlank && f == 0.00 {
					buf = ""
				} else {
					buf = UInt64ToString(f)
				}
			}
		case time.Time:
			if skipZero && f.IsZero() {
				return "", true, nil
			} else {
				if LenTrim(timeFormat) == 0 {
					if zeroBlank && f.IsZero() {
						buf = ""
					} else {
						buf = FormatDateTime(f)
					}
				} else {
					if zeroBlank && f.IsZero() {
						buf = ""
					} else {
						buf = f.Format(timeFormat)
					}
				}
			}
		default:
			return "", false, fmt.Errorf("%s Unhandled [1]", o2.Type().Name())
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
				if len(boolTrue) == 0 {
					buf = "true"
				} else {
					buf = Trim(boolTrue)
				}
			} else {
				if skipZero {
					return "", true, nil
				} else {
					if len(boolFalse) == 0 {
						buf = "false"
					} else {
						if Trim(boolTrue) == Trim(boolFalse) {
							buf = "false"
						} else {
							buf = Trim(boolFalse)
						}
					}
				}
			}
		case sql.NullFloat64:
			f64 := FromNullFloat64(f)

			if skipZero && f64 == 0.00 {
				return "", true, nil
			} else {
				if zeroBlank && f64 == 0.00 {
					buf = ""
				} else {
					buf = FloatToString(f64)
				}
			}
		case sql.NullInt32:
			i32 := FromNullInt(f)

			if skipZero && i32 == 0 {
				return "", true, nil
			} else {
				if zeroBlank && i32 == 0 {
					buf = ""
				} else {
					buf = Itoa(i32)
				}
			}
		case sql.NullInt64:
			i64 := FromNullInt64(f)

			if skipZero && i64 == 0 {
				return "", true, nil
			} else {
				if zeroBlank && i64 == 0 {
					buf = ""
				} else {
					buf = Int64ToString(i64)
				}
			}
		case sql.NullTime:
			t := FromNullTime(f)

			if skipZero && t.IsZero() {
				return "", true, nil
			} else {
				if LenTrim(timeFormat) == 0 {
					buf = FormatDateTime(t)
				} else {
					if zeroBlank && t.IsZero() {
						buf = ""
					} else {
						buf = t.Format(timeFormat)
					}
				}
			}
		case time.Time:
			if skipZero && f.IsZero() {
				return "", true, nil
			} else {
				if LenTrim(timeFormat) == 0 {
					if zeroBlank && f.IsZero() {
						buf = ""
					} else {
						buf = FormatDateTime(f)
					}
				} else {
					if zeroBlank && f.IsZero() {
						buf = ""
					} else {
						buf = f.Format(timeFormat)
					}
				}
			}
		case nil:
			if skipZero || skipBlank {
				return "", true, nil
			} else {
				buf = ""
			}
		default:
			return "", false, fmt.Errorf("%s Unhandled [2]", o.Type().Name())
		}
	}

	return buf, false, nil
}

// ReflectStringToField accepts string value and reflects into reflect.Value field based on the field data type
//
// timeFormat:
//
//	2006, 06 = year,
//	01, 1, Jan, January = month,
//	02, 2, _2 = day (_2 = width two, right justified)
//	03, 3, 15 = hour (15 = 24 hour format)
//	04, 4 = minute
//	05, 5 = second
//	PM pm = AM PM
func ReflectStringToField(o reflect.Value, v string, timeFormat string) error {
	switch o.Kind() {
	case reflect.String:
		o.SetString(v)
	case reflect.Bool:
		b, _ := ParseBool(v)
		o.SetBool(b)
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
			b, _ := ParseBool(v)
			o2.SetBool(b)
		case time.Time:
			if LenTrim(timeFormat) == 0 {
				o2.Set(reflect.ValueOf(ParseDate(v)))
			} else {
				o2.Set(reflect.ValueOf(ParseDateTimeCustom(v, timeFormat)))
			}
		default:
			return fmt.Errorf(o2.Type().Name() + " Unhandled [1]")
		}
	default:
		switch o.Interface().(type) {
		case sql.NullString:
			o.Set(reflect.ValueOf(sql.NullString{String: v, Valid: true}))
		case sql.NullBool:
			b, _ := ParseBool(v)
			o.Set(reflect.ValueOf(sql.NullBool{Bool: b, Valid: true}))
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
		case nil:
			return nil
		default:
			return fmt.Errorf(o.Type().Name() + " Unhandled [2]")
		}
	}

	return nil
}

// DerefPointersZero gets pointer base type
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

// DerefError dereferences reflect.Value to error object if underlying type was error
func DerefError(v reflect.Value) error {
	if e, ok := v.Interface().(error); ok {
		// v is error, check if error exists
		if e != nil {
			return e
		}
	}

	return nil
}

// ReflectGetType returns the type of obj interface{} passed in.
// if obj interface{} is a pointer, then its base type will be returned instead
func ReflectGetType(obj interface{}) reflect.Type {
	if obj == nil {
		return nil
	} else {
		t1 := reflect.TypeOf(obj)

		if t1.Kind() == reflect.Ptr {
			return t1.Elem()
		} else {
			return t1
		}
	}
}

// ReflectObjectNewPtr creates a new object ptr for the object type given at parameter.
// the return interface{} represents the actual object ptr created
func ReflectObjectNewPtr(objType reflect.Type) interface{} {
	if objType == nil {
		return nil
	} else {
		return reflect.New(objType).Interface()
	}
}

// ReflectInterfaceSliceLen returns the length of a slice passed as interface{}.
// It returns the length and a boolean indicating whether the input was a slice or array.
// If the input is not a slice or array, it returns 0 and false.
func ReflectInterfaceSliceLen(slice interface{}) (int, bool) {
	v := reflect.ValueOf(slice)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return 0, false
	}
	return v.Len(), true
}

// ReflectAppendSlices appends two slices provided as interface{} and returns the resulting slice.
// It returns an error if the inputs are not slices or their types are incompatible.
func ReflectAppendSlices(slice1, slice2 interface{}) (interface{}, error) {
	// validate
	if slice1 == nil {
		return nil, fmt.Errorf("Input 1 is Nil")
	}

	if slice2 == nil {
		return nil, fmt.Errorf("Input 2 is Nil")
	}

	// Get the reflect.Value of the inputs
	v1 := reflect.ValueOf(slice1)
	v2 := reflect.ValueOf(slice2)

	// Check that both inputs are slices
	if v1.Kind() != reflect.Slice || v2.Kind() != reflect.Slice {
		return nil, fmt.Errorf("Both Inputs Expected To Be Slices")
	}

	// Ensure the slices have the same type
	if v1.Type() != v2.Type() {
		return nil, fmt.Errorf("Both Inputs Expects To Be Same Type")
	}

	// Create a new slice to hold the appended elements
	result := reflect.MakeSlice(v1.Type(), v1.Len()+v2.Len(), v1.Len()+v2.Len())

	// Copy elements from the first slice
	reflect.Copy(result, v1)

	// Copy elements from the second slice
	reflect.Copy(result.Slice(v1.Len(), result.Len()), v2)

	return result.Interface(), nil
}

// ReflectInterfaceToString converts interface{} to string
func ReflectInterfaceToString(v interface{}) string {
	if v == nil {
		return ""
	}

	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	case int:
		return Itoa(t)
	case int64:
		return Int64ToString(t)
	case float32:
		return Float32ToString(t)
	case float64:
		return Float64ToString(t)
	case bool:
		return BoolToString(t)
	case error:
		return t.Error()
	default:
		return fmt.Sprintf("%v", t)
	}
}

// GetStructFieldValue retrieves the value of a specific field from a struct.
// Returns the field value as an string and an error if the field does not exist or is inaccessible.
func GetStructFieldValue(input interface{}, fieldName string) (string, error) {
	if input == nil {
		return "", fmt.Errorf("Input Object is Nil")
	}

	if LenTrim(fieldName) <= 0 {
		return "", fmt.Errorf("Field Name is Required")
	}

	// Get the reflect.Value and reflect.Type of the input
	v := reflect.ValueOf(input)
	t := reflect.TypeOf(input)

	// Handle pointer to struct
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return "", fmt.Errorf("Input Object is Nil Pointer")
		}
		v = v.Elem()
		t = t.Elem()
	}

	// Ensure the input is a struct
	if v.Kind() != reflect.Struct {
		return "", fmt.Errorf("Input Object Expects a Struct Object or Pointer to Struct Object")
	}

	// Check if the field exists
	fieldValue := v.FieldByName(fieldName)

	if !fieldValue.IsValid() {
		return "", fmt.Errorf("Field %q Does Not Exist In the Struct", fieldName)
	}

	// Check if the field is accessible
	if !fieldValue.CanInterface() {
		return "", fmt.Errorf("Field %q is Inaccessible", fieldName)
	}

	// return string value
	if fieldValueStr, _, err := ReflectValueToString(fieldValue, "true", "false", false, false, "", false); err != nil {
		return "", fmt.Errorf("Field %q Value Conversion Failed: %v", fieldName, err)
	} else {
		return fieldValueStr, nil
	}
}

// ConvertStructToSlice converts a struct passed as an interface{} to a slice of that struct's type.
// Returns an error if the input is not a struct or a pointer to a struct.
func ConvertStructToSlice(input interface{}) (interface{}, error) {
	if input == nil {
		return nil, fmt.Errorf("Struct Input is Nil")
	}

	// Get the reflect.Value and reflect.Type of the input
	v := reflect.ValueOf(input)
	t := reflect.TypeOf(input)

	var v2 reflect.Value

	// Handle pointer to struct
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil, fmt.Errorf("Struct Input is Nil Pointer")
		}
		v2 = v.Elem()
	} else {
		v2 = v // allow direct struct values
	}

	// Ensure the input is a struct
	if v2.Kind() != reflect.Struct {
		return nil, fmt.Errorf("Struct Input Expects a Struct Object or Pointer to Struct Object")
	}

	// Create a slice of the struct's type with one element
	sliceType := reflect.SliceOf(t)
	slice := reflect.MakeSlice(sliceType, 1, 1)
	slice.Index(0).Set(v)

	return slice.Interface(), nil
}

// ReflectSetStringSliceToField sets a slice of strings to a specified field in a struct passed as an interface{}.
// Returns an error if the field does not exist or is not compatible with a []string type.
func ReflectSetStringSliceToField(target interface{}, fieldName string, values []string) error {
	if target == nil {
		return fmt.Errorf("Target is Nil")
	}

	if LenTrim(fieldName) <= 0 {
		return fmt.Errorf("Field Name is Required")
	}

	// Get the reflect.Value and reflect.Type of the target
	v := reflect.ValueOf(target)

	// allow addressable struct values or pointers; clearer errors
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return fmt.Errorf("Target Pointer is Nil")
		}
		v = v.Elem()
	} else if v.Kind() == reflect.Struct {
		if !v.CanAddr() {
			return fmt.Errorf("Target Struct Not Addressable; pass a pointer instead")
		}
	} else {
		return fmt.Errorf("Target Expects a Pointer to a Struct or an Addressable Struct Value")
	}

	field := v.FieldByName(fieldName)

	// Check if the field exists
	if !field.IsValid() {
		return fmt.Errorf("Field %q Not Exist in Struct", fieldName)
	}

	// Ensure the field is a slice of strings
	if field.Type() != reflect.TypeOf([]string{}) {
		return fmt.Errorf("Field %q Expects Type of []string", fieldName)
	}

	// Ensure the field is settable
	if !field.CanSet() {
		return fmt.Errorf("Field %q Not Settable", fieldName)
	}

	// Set the slice of strings to the field
	field.Set(reflect.ValueOf(values))
	return nil
}
