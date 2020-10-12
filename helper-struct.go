package helper

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
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

// src and dst both must be struct，and dst must be point
// it will copy the src struct with same tag name as dst struct tag
func Fill(src interface{}, dst interface{}) error {
	srcType := reflect.TypeOf(src)
	srcValue := reflect.ValueOf(src)
	dstValue := reflect.ValueOf(dst)

	if srcType.Kind() != reflect.Struct {
		return errors.New("src must be struct")
	}
	if dstValue.Kind() != reflect.Ptr {
		return errors.New("dst must be point")
	}

	for i := 0; i < srcType.NumField(); i++ {
		dstField := dstValue.Elem().FieldByName(srcType.Field(i).Name)
		if dstField.CanSet() {
			dstField.Set(srcValue.Field(i))
		}
	}

	return nil
}

// MarshalStructToQueryParams marshals a struct pointer's fields to query params string,
// output query param names are based on values given in tagName,
// to exclude certain struct fields from being marshaled, use - as value in struct tag defined by tagName,
// if there is a need to name the value of tagName, but still need to exclude from output, use the excludeTagName with -, such as `x:"-"`
//
// special struct tags:
//		1) `getter:"Key"`			// if field type is custom struct or enum,
//									   specify the custom method getter (no parameters allowed) that returns the expected value in first ordinal result position
//		2) `booltrue:"1"` 			// if field is defined, contains bool literal for true condition, such as 1 or true, that overrides default system bool literal value
//		3) `boolfalse:"0"`			// if field is defined, contains bool literal for false condition, such as 0 or false, that overrides default system bool literal value
// 		4) `uniqueid:"xyz"`			// if two or more struct field is set with the same uniqueid, then only the first encountered field with the same uniqueid will be used in marshal
//		5) `skipblank:"false"`		// if true, then any fields that is blank string will be excluded from marshal (this only affects fields that are string)
//		6) `skipzero:"false"`		// if true, then any fields that are 0, 0.00, time.Zero(), false, nil will be excluded from marshal (this only affects fields that are number, bool, time, pointer)
//		7) `timeformat:"20060102"`	// for time.Time field, optional date time format, specified as:
//											2006, 06 = year,
//											01, 1, Jan, January = month,
//											02, 2, _2 = day (_2 = width two, right justified)
//											03, 3, 15 = hour (15 = 24 hour format)
//											04, 4 = minute
//											05, 5 = second
//											PM pm = AM PM
//		8) `outprefix:""`			// for marshal method, if field value is to precede with an output prefix, such as XYZ= (affects marshal queryParams / csv methods only)
func MarshalStructToQueryParams(inputStructPtr interface{}, tagName string, excludeTagName string) (string, error) {
	if inputStructPtr == nil {
		return "", fmt.Errorf("MarshalStructToQueryParams Requires Input Struct Variable Pointer")
	}

	if LenTrim(tagName) == 0 {
		return "", fmt.Errorf("MarshalStructToQueryParams Requires TagName (Tag Name defines query parameter name)")
	}

	s := reflect.ValueOf(inputStructPtr)

	if s.Kind() != reflect.Ptr {
		return "", fmt.Errorf("MarshalStructToQueryParams Expects inputStructPtr To Be a Pointer")
	} else {
		s = s.Elem()
	}

	if s.Kind() != reflect.Struct {
		return "", fmt.Errorf("MarshalStructToQueryParams Requires Struct Object")
	}

	output := ""
	uniqueMap := make(map[string]string)

	for i := 0; i < s.NumField(); i++ {
		field := s.Type().Field(i)

		if o := s.FieldByName(field.Name); o.IsValid() {
			tag := field.Tag.Get(tagName)

			if LenTrim(tag) == 0 {
				tag = field.Name
			}

			if tag != "-" {
				if LenTrim(excludeTagName) > 0 {
					if Trim(field.Tag.Get(excludeTagName)) == "-" {
						continue
					}
				}

				if tagUniqueId := Trim(field.Tag.Get("uniqueid")); len(tagUniqueId) > 0 {
					if _, ok := uniqueMap[strings.ToLower(tagUniqueId)]; ok {
						continue
					} else {
						uniqueMap[strings.ToLower(tagUniqueId)] = field.Name
					}
				}

				if tagGetter := Trim(field.Tag.Get("getter")); len(tagGetter) > 0 {
					if ov, notFound := ReflectCall(o, tagGetter); !notFound {
						if len(ov) > 0 {
							o = ov[0]
						}
					}
				}

				var boolTrue, boolFalse, timeFormat, outPrefix string
				var skipBlank, skipZero bool

				if vs := GetStructTagsValueSlice(field, "booltrue", "boolfalse", "skipblank", "skipzero", "timeformat", "outprefix"); len(vs) == 6 {
					boolTrue = vs[0]
					boolFalse = vs[1]
					skipBlank = IsBool(vs[2])
					skipZero = IsBool(vs[3])
					timeFormat = vs[4]
					outPrefix = vs[5]
				}

				if buf, skip, err := ReflectValueToString(o, boolTrue, boolFalse, skipBlank, skipZero, timeFormat); err != nil || skip {
					if tagUniqueId := Trim(field.Tag.Get("uniqueid")); len(tagUniqueId) > 0 {
						if _, ok := uniqueMap[strings.ToLower(tagUniqueId)]; ok {
							delete(uniqueMap, strings.ToLower(tagUniqueId))
						}
					}

					continue
				} else {
					if LenTrim(output) > 0 {
						output += "&"
					}

					output += fmt.Sprintf("%s=%s", tag, url.PathEscape(outPrefix + buf))
				}
			}
		}
	}

	if LenTrim(output) == 0 {
		return "", fmt.Errorf("MarshalStructToQueryParams Yielded Blank Output")
	} else {
		return output, nil
	}
}

// MarshalStructToJson marshals a struct pointer's fields to json string,
// output json names are based on values given in tagName,
// to exclude certain struct fields from being marshaled, include - as value in struct tag defined by tagName,
// if there is a need to name the value of tagName, but still need to exclude from output, use the excludeTagName with -, such as `x:"-"`
//
// special struct tags:
//		1) `getter:"Key"`			// if field type is custom struct or enum,
//									   specify the custom method getter (no parameters allowed) that returns the expected value in first ordinal result position
//		2) `booltrue:"1"` 			// if field is defined, contains bool literal for true condition, such as 1 or true, that overrides default system bool literal value
//		3) `boolfalse:"0"`			// if field is defined, contains bool literal for false condition, such as 0 or false, that overrides default system bool literal value
// 		4) `uniqueid:"xyz"`			// if two or more struct field is set with the same uniqueid, then only the first encountered field with the same uniqueid will be used in marshal
//		5) `skipblank:"false"`		// if true, then any fields that is blank string will be excluded from marshal (this only affects fields that are string)
//		6) `skipzero:"false"`		// if true, then any fields that are 0, 0.00, time.Zero(), false, nil will be excluded from marshal (this only affects fields that are number, bool, time, pointer)
//		7) `timeformat:"20060102"`	// for time.Time field, optional date time format, specified as:
//											2006, 06 = year,
//											01, 1, Jan, January = month,
//											02, 2, _2 = day (_2 = width two, right justified)
//											03, 3, 15 = hour (15 = 24 hour format)
//											04, 4 = minute
//											05, 5 = second
//											PM pm = AM PM
func MarshalStructToJson(inputStructPtr interface{}, tagName string, excludeTagName string) (string, error) {
	if inputStructPtr == nil {
		return "", fmt.Errorf("MarshalStructToJson Requires Input Struct Variable Pointer")
	}

	if LenTrim(tagName) == 0 {
		return "", fmt.Errorf("MarshalStructToJson Requires TagName (Tag Name defines Json name)")
	}

	s := reflect.ValueOf(inputStructPtr)

	if s.Kind() != reflect.Ptr {
		return "", fmt.Errorf("MarshalStructToJson Expects inputStructPtr To Be a Pointer")
	} else {
		s = s.Elem()
	}

	if s.Kind() != reflect.Struct {
		return "", fmt.Errorf("MarshalStructToJson Requires Struct Object")
	}

	output := ""
	uniqueMap := make(map[string]string)

	for i := 0; i < s.NumField(); i++ {
		field := s.Type().Field(i)

		if o := s.FieldByName(field.Name); o.IsValid() {
			tag := field.Tag.Get(tagName)

			if LenTrim(tag) == 0 {
				tag = field.Name
			}

			if tag != "-" {
				if LenTrim(excludeTagName) > 0 {
					if Trim(field.Tag.Get(excludeTagName)) == "-" {
						continue
					}
				}

				if tagUniqueId := Trim(field.Tag.Get("uniqueid")); len(tagUniqueId) > 0 {
					if _, ok := uniqueMap[strings.ToLower(tagUniqueId)]; ok {
						continue
					} else {
						uniqueMap[strings.ToLower(tagUniqueId)] = field.Name
					}
				}

				if tagGetter := Trim(field.Tag.Get("getter")); len(tagGetter) > 0 {
					if ov, notFound := ReflectCall(o, tagGetter); !notFound {
						if len(ov) > 0 {
							o = ov[0]
						}
					}
				}

				var boolTrue, boolFalse, timeFormat string
				var skipBlank, skipZero bool

				if vs := GetStructTagsValueSlice(field, "booltrue", "boolfalse", "skipblank", "skipzero", "timeformat"); len(vs) == 5 {
					boolTrue = vs[0]
					boolFalse = vs[1]
					skipBlank = IsBool(vs[2])
					skipZero = IsBool(vs[3])
					timeFormat = vs[4]
				}

				buf, skip, err := ReflectValueToString(o, boolTrue, boolFalse, skipBlank, skipZero, timeFormat)

				if err != nil || skip {
					if tagUniqueId := Trim(field.Tag.Get("uniqueid")); len(tagUniqueId) > 0 {
						if _, ok := uniqueMap[strings.ToLower(tagUniqueId)]; ok {
							delete(uniqueMap, strings.ToLower(tagUniqueId))
						}
					}

					continue
				}

				buf = strings.Replace(buf, `"`, `\"`, -1)
				buf = strings.Replace(buf, `'`, `\'`, -1)

				if LenTrim(output) > 0 {
					output += ", "
				}

				output += fmt.Sprintf(`"%s":"%s"`, tag, JsonToEscaped(buf))
			}
		}
	}

	if LenTrim(output) == 0 {
		return "", fmt.Errorf("MarshalStructToJson Yielded Blank Output")
	} else {
		return fmt.Sprintf("{%s}", output), nil
	}
}

// UnmarshalJsonToStruct will parse jsonPayload string,
// and set parsed json element value into struct fields based on struct tag named by tagName,
// any tagName value with - will be ignored, any excludeTagName defined with value of - will also cause parser to ignore the field
//
// note: this method expects simple json in key value pairs only, not json containing slices or more complex json structs within existing json field
//
// Predefined Struct Tags Usable:
// 		1) `setter:"ParseByKey`		// if field type is custom struct or enum,
//									   specify the custom method (only 1 lookup parameter value allowed) setter that sets value(s) into the field
//		2) `def:""`					// default value to set into struct field in case unmarshal doesn't set the struct field value
//		3) `timeformat:"20060102"`	// for time.Time field, optional date time format, specified as:
//											2006, 06 = year,
//											01, 1, Jan, January = month,
//											02, 2, _2 = day (_2 = width two, right justified)
//											03, 3, 15 = hour (15 = 24 hour format)
//											04, 4 = minute
//											05, 5 = second
//											PM pm = AM PM
func UnmarshalJsonToStruct(inputStructPtr interface{}, jsonPayload string, tagName string, excludeTagName string) error {
	if inputStructPtr == nil {
		return fmt.Errorf("InputStructPtr is Required")
	}

	if LenTrim(jsonPayload) == 0 {
		return fmt.Errorf("JsonPayload is Required")
	}

	if LenTrim(tagName) == 0 {
		return fmt.Errorf("TagName is Required")
	}

	s := reflect.ValueOf(inputStructPtr)

	if s.Kind() != reflect.Ptr {
		return fmt.Errorf("InputStructPtr Must Be Pointer")
	} else {
		s = s.Elem()
	}

	if s.Kind() != reflect.Struct {
		return fmt.Errorf("InputStructPtr Must Be Struct")
	}

	// unmarshal json to map
	jsonMap := make(map[string]json.RawMessage)

	if err := json.Unmarshal([]byte(jsonPayload), &jsonMap); err != nil {
		return fmt.Errorf("Unmarshal Json Failed: %s", err)
	}

	if jsonMap == nil {
		return fmt.Errorf("Unmarshaled Json Map is Nil")
	}

	if len(jsonMap) == 0 {
		return fmt.Errorf("Unmarshaled Json Map Has No Elements")
	}

	StructClearFields(inputStructPtr)
	SetStructFieldDefaultValues(inputStructPtr)

	for i := 0; i < s.NumField(); i++ {
		field := s.Type().Field(i)

		if o := s.FieldByName(field.Name); o.IsValid() && o.CanSet() {
			// get json field name if defined
			jName := Trim(field.Tag.Get(tagName))

			if jName == "-" {
				continue
			}

			if LenTrim(excludeTagName) > 0 {
				if Trim(field.Tag.Get(excludeTagName)) == "-" {
					continue
				}
			}

			if LenTrim(jName) == 0 {
				jName = field.Name
			}

			// get json field value based on jName from jsonMap
			jValue := ""
			timeFormat := Trim(field.Tag.Get("timeformat"))

			if jRaw, ok := jsonMap[jName]; !ok {
				continue
			} else {
				jValue = JsonFromEscaped(string(jRaw))

				if len(jValue) > 0 {
					if tagSetter := Trim(field.Tag.Get("setter")); len(tagSetter) > 0 {
						if o.Kind() != reflect.Ptr {
							// o is not ptr
							if results, notFound := ReflectCall(o, tagSetter, jValue); !notFound && len(results) > 0 {
								if len(results) == 1 {
									if jv, _, err := ReflectValueToString(results[0], "", "", false, false, timeFormat); err == nil {
										jValue = jv
									}
								} else if len(results) > 1 {
									getFirstVar := true

									if e, ok := results[len(results)-1].Interface().(error); ok {
										// last var is error, check if error exists
										if e != nil {
											getFirstVar = false
										}
									}

									if getFirstVar {
										if jv, _, err := ReflectValueToString(results[0], "", "", false, false, timeFormat); err == nil {
											jValue = jv
										}
									}
								}
							}
						} else {
							// o is ptr
							// get base type
							if baseType, _, isNilPtr := DerefPointersZero(o); isNilPtr {
								// create new struct pointer
								o.Set(reflect.New(baseType.Type()))
							}

							if ov, notFound := ReflectCall(o, tagSetter, jValue); !notFound {
								if len(ov) == 1 {
									if ov[0].Kind() == reflect.Ptr {
										o.Set(ov[0])
									}
								} else if len(ov) > 1 {
									getFirstVar := true

									if e := DerefError(ov[len(ov)-1]); e != nil {
										getFirstVar = false
									}

									if getFirstVar {
										if ov[0].Kind() == reflect.Ptr {
											o.Set(ov[0])
										}
									}
								}
							}

							// for o as ptr
							// once complete, continue
							continue
						}
					}
				}
			}

			// set validated csv value into corresponding struct field
			if err := ReflectStringToField(o, jValue, timeFormat); err != nil {
				return err
			}
		}
	}

	return nil
}

// MarshalSliceStructToJson accepts a slice of struct pointer, then using tagName and excludeTagName to marshal to json array
// To pass in inputSliceStructPtr, convert slice of actual objects at the calling code, using SliceObjectsToSliceInterface(),
// if there is a need to name the value of tagName, but still need to exclude from output, use the excludeTagName with -, such as `x:"-"`
func MarshalSliceStructToJson(inputSliceStructPtr []interface{}, tagName string, excludeTagName string) (jsonArrayOutput string, err error) {
	if len(inputSliceStructPtr) == 0 {
		return "", fmt.Errorf("Input Slice Struct Pointer Nil")
	}

	for _, v := range inputSliceStructPtr {
		if s, e := MarshalStructToJson(v, tagName, excludeTagName); e != nil {
			return "", fmt.Errorf("MarshalSliceStructToJson Failed: %s", e)
		} else {
			if LenTrim(jsonArrayOutput) > 0 {
				jsonArrayOutput += ", "
			}

			jsonArrayOutput += s
		}
	}

	if LenTrim(jsonArrayOutput) > 0 {
		return fmt.Sprintf("[%s]", jsonArrayOutput), nil
	} else {
		return "", fmt.Errorf("MarshalSliceStructToJson Yielded Blank String")
	}
}

// StructClearFields will clear all fields within struct with default value
func StructClearFields(inputStructPtr interface{}) {
	if inputStructPtr == nil {
		return
	}

	s := reflect.ValueOf(inputStructPtr)

	if s.Kind() != reflect.Ptr {
		return
	} else {
		s = s.Elem()
	}

	if s.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < s.NumField(); i++ {
		field := s.Type().Field(i)

		if o := s.FieldByName(field.Name); o.IsValid() && o.CanSet() {
			switch o.Kind() {
			case reflect.String:
				o.SetString("")
			case reflect.Bool:
				o.SetBool(false)
			case reflect.Int8:
				fallthrough
			case reflect.Int16:
				fallthrough
			case reflect.Int:
				fallthrough
			case reflect.Int32:
				fallthrough
			case reflect.Int64:
				o.SetInt(0)
			case reflect.Float32:
				fallthrough
			case reflect.Float64:
				o.SetFloat(0)
			case reflect.Uint8:
				fallthrough
			case reflect.Uint16:
				fallthrough
			case reflect.Uint:
				fallthrough
			case reflect.Uint32:
				fallthrough
			case reflect.Uint64:
				o.SetUint(0)
			case reflect.Ptr:
				o.Set(reflect.Zero(o.Type()))
			default:
				switch o.Interface().(type) {
				case sql.NullString:
					o.Set(reflect.ValueOf(sql.NullString{}))
				case sql.NullBool:
					o.Set(reflect.ValueOf(sql.NullBool{}))
				case sql.NullFloat64:
					o.Set(reflect.ValueOf(sql.NullFloat64{}))
				case sql.NullInt32:
					o.Set(reflect.ValueOf(sql.NullInt32{}))
				case sql.NullInt64:
					o.Set(reflect.ValueOf(sql.NullInt64{}))
				case sql.NullTime:
					o.Set(reflect.ValueOf(sql.NullTime{}))
				case time.Time:
					o.Set(reflect.ValueOf(time.Time{}))
				}
			}
		}
	}
}

// IsStructFieldSet checks if any field value is not default blank or zero
func IsStructFieldSet(inputStructPtr interface{}) bool {
	if inputStructPtr == nil {
		return false
	}

	s := reflect.ValueOf(inputStructPtr)

	if s.Kind() != reflect.Ptr {
		return false
	} else {
		s = s.Elem()
	}

	if s.Kind() != reflect.Struct {
		return false
	}

	for i := 0; i < s.NumField(); i++ {
		field := s.Type().Field(i)

		if o := s.FieldByName(field.Name); o.IsValid() && o.CanSet() {
			tagDef := field.Tag.Get("def")

			switch o.Kind() {
			case reflect.String:
				if LenTrim(o.String()) > 0 {
					if o.String() != tagDef	{
						return true
					}
				}
			case reflect.Bool:
				if o.Bool() {
					return true
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
				if o.Int() != 0 {
					if Int64ToString(o.Int()) != tagDef	{
						return true
					}
				}
			case reflect.Float32:
				fallthrough
			case reflect.Float64:
				if o.Float() != 0 {
					if Float64ToString(o.Float()) != tagDef	{
						return true
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
				if o.Uint() > 0 {
					if UInt64ToString(o.Uint()) != tagDef {
						return true
					}
				}
			case reflect.Ptr:
				if !o.IsNil() {
					return true
				}
			default:
				switch f := o.Interface().(type) {
				case sql.NullString:
					if f.Valid {
						if len(tagDef) == 0 {
							return true
						} else {
							if f.String != tagDef {
								return true
							}
						}
					}
				case sql.NullBool:
					if f.Valid {
						if len(tagDef) == 0 {
							return true
						} else {
							if f.Bool != IsBool(tagDef) {
								return true
							}
						}
					}
				case sql.NullFloat64:
					if f.Valid {
						if len(tagDef) == 0 {
							return true
						} else {
							if Float64ToString(f.Float64) != tagDef {
								return true
							}
						}
					}
				case sql.NullInt32:
					if f.Valid {
						if len(tagDef) == 0 {
							return true
						} else {
							if Itoa(int(f.Int32)) != tagDef {
								return true
							}
						}
					}
				case sql.NullInt64:
					if f.Valid {
						if len(tagDef) == 0 {
							return true
						} else {
							if Int64ToString(f.Int64) != tagDef {
								return true
							}
						}
					}
				case sql.NullTime:
					if f.Valid {
						if len(tagDef) == 0 {
							return true
						} else {
							tagTimeFormat := Trim(field.Tag.Get("timeformat"))

							if LenTrim(tagTimeFormat) == 0 {
								tagTimeFormat = DateTimeFormatString()
							}

							if f.Time != ParseDateTimeCustom(tagDef, tagTimeFormat) {
								return true
							}
						}
					}
				case time.Time:
					if !f.IsZero() {
						if len(tagDef) == 0 {
							return true
						} else {
							tagTimeFormat := Trim(field.Tag.Get("timeformat"))

							if LenTrim(tagTimeFormat) == 0 {
								tagTimeFormat = DateTimeFormatString()
							}

							if f != ParseDateTimeCustom(tagDef, tagTimeFormat) {
								return true
							}
						}
					}
				}
			}
		}
	}

	return false
}

// SetStructFieldDefaultValues sets default value defined in struct tag `def:""` into given field,
// this method is used during unmarshal action only,
// default value setting is for value types and fields with `setter:""` defined only,
// timeformat is used if field is datetime, for overriding default format of ISO style
func SetStructFieldDefaultValues(inputStructPtr interface{}) bool {
	if inputStructPtr == nil {
		return false
	}

	s := reflect.ValueOf(inputStructPtr)

	if s.Kind() != reflect.Ptr {
		return false
	} else {
		s = s.Elem()
	}

	if s.Kind() != reflect.Struct {
		return false
	}

	for i := 0; i < s.NumField(); i++ {
		field := s.Type().Field(i)

		if o := s.FieldByName(field.Name); o.IsValid() && o.CanSet() {
			tagDef := field.Tag.Get("def")

			if len(tagDef) == 0 {
				continue
			}

			switch o.Kind() {
			case reflect.String:
				if LenTrim(o.String()) == 0 {
					o.SetString(tagDef)
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
				if o.Int() == 0 {
					tagSetter := Trim(field.Tag.Get("setter"))

					if LenTrim(tagSetter) == 0 {
						if i64, ok := ParseInt64(tagDef); ok && i64 != 0 {
							if !o.OverflowInt(i64) {
								o.SetInt(i64)
							}
						}
					} else {
						if res, notFound := ReflectCall(o, tagSetter, tagDef); !notFound {
							if len(res) == 1 {
								if val, skip, err := ReflectValueToString(res[0], "", "", false, false, ""); err == nil && !skip {
									tagDef = val
								} else {
									continue
								}
							} else if len(res) > 1 {
								if err := DerefError(res[len(res)-1:][0]); err == nil {
									if val, skip, err := ReflectValueToString(res[0], "", "", false, false, ""); err == nil && !skip {
										tagDef = val
									} else {
										continue
									}
								}
							}

							if i64, ok := ParseInt64(tagDef); ok && i64 != 0 {
								if !o.OverflowInt(i64) {
									o.SetInt(i64)
								}
							}
						}
					}
				}
			case reflect.Float32:
				fallthrough
			case reflect.Float64:
				if o.Float() == 0 {
					if f64, ok := ParseFloat64(tagDef); ok && f64 != 0 {
						if !o.OverflowFloat(f64) {
							o.SetFloat(f64)
						}
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
				if o.Uint() == 0 {
					if u64 := StrToUint64(tagDef); u64 != 0 {
						if !o.OverflowUint(u64) {
							o.SetUint(u64)
						}
					}
				}
			default:
				switch f := o.Interface().(type) {
				case sql.NullString:
					if !f.Valid {
						o.Set(reflect.ValueOf(sql.NullString{String: tagDef, Valid: true}))
					}
				case sql.NullBool:
					if !f.Valid {
						o.Set(reflect.ValueOf(sql.NullBool{Bool: IsBool(tagDef), Valid: true}))
					}
				case sql.NullFloat64:
					if !f.Valid {
						if f64, ok := ParseFloat64(tagDef); ok && f64 != 0 {
							o.Set(reflect.ValueOf(sql.NullFloat64{Float64: f64, Valid: true}))
						}
					}
				case sql.NullInt32:
					if !f.Valid {
						if i32, ok := ParseInt32(tagDef); ok && i32 != 0 {
							o.Set(reflect.ValueOf(sql.NullInt32{Int32: int32(i32), Valid: true}))
						}
					}
				case sql.NullInt64:
					if !f.Valid {
						if i64, ok := ParseInt64(tagDef); ok && i64 != 0 {
							o.Set(reflect.ValueOf(sql.NullInt64{Int64: i64, Valid: true}))
						}
					}
				case sql.NullTime:
					if !f.Valid {
						tagTimeFormat := Trim(field.Tag.Get("timeformat"))

						if LenTrim(tagTimeFormat) == 0 {
							tagTimeFormat = DateTimeFormatString()
						}

						if t := ParseDateTimeCustom(tagDef, tagTimeFormat); !t.IsZero() {
							o.Set(reflect.ValueOf(sql.NullTime{Time: t, Valid: true}))
						}
					}
				case time.Time:
					if f.IsZero() {
						tagTimeFormat := Trim(field.Tag.Get("timeformat"))

						if LenTrim(tagTimeFormat) == 0 {
							tagTimeFormat = DateTimeFormatString()
						}

						if t := ParseDateTimeCustom(tagDef, tagTimeFormat); !t.IsZero() {
							o.Set(reflect.ValueOf(t))
						}
					}
				}
			}
		}
	}

	return true
}

// UnmarshalCSVToStruct will parse csvPayload string (one line of csv data) using csvDelimiter,
// and set parsed csv element value into struct fields based on Ordinal Position defined via struct tag,
// additionally processes struct tag data validation and length / range (if not valid, will set to data type default)
//
// Predefined Struct Tags Usable:
//		1) `pos:"1"`				// ordinal position of the field in relation to the csv parsed output expected (Zero-Based Index)
//									   NOTE: if field is mutually exclusive with one or more uniqueId, then pos # should be named the same for all uniqueIds,
//											 if multiple fields are in exclusive condition, and skipBlank or skipZero, always include a blank default field as the last of unique field list
//		2) `type:"xyz"`				// data type expected:
//											A = AlphabeticOnly, N = NumericOnly 0-9, AN = AlphaNumeric, ANS = AN + PrintableSymbols,
//											H = Hex, B64 = Base64, B = true/false, REGEX = Regular Expression, Blank = Any,
//		3) `regex:"xyz"`			// if Type = REGEX, this struct tag contains the regular expression string,
//										 	regex express such as [^A-Za-z0-9_-]+
//										 	method will replace any regex matched string to blank
//		4) `size:"x..y"`			// data type size rule:
//											x = Exact size match
//											x.. = From x and up
//											..y = From 0 up to y
//											x..y = From x to y
//		5) `range:"x..y"`			// data type range value when Type is N, if underlying data type is string, method will convert first before testing
//		6) `req:"true"`				// indicates data value is required or not, true or false
//		7) `getter:"Key"`			// if field type is custom struct or enum, specify the custom method getter (no parameters allowed) that returns the expected value in first ordinal result position
// 		8) `setter:"ParseByKey`		// if field type is custom struct or enum, specify the custom method (only 1 lookup parameter value allowed) setter that sets value(s) into the field
//		9) `outprefix:""`			// for marshal method, if field value is to precede with an output prefix, such as XYZ= (affects marshal queryParams / csv methods only)
//									   WARNING: if csv is variable elements count, rather than fixed count ordinal, then csv MUST include outprefix for all fields in order to properly identify target struct field
//		10) `def:""`				// default value to set into struct field in case unmarshal doesn't set the struct field value
//		11) `timeformat:"20060102"`	// for time.Time field, optional date time format, specified as:
//											2006, 06 = year,
//											01, 1, Jan, January = month,
//											02, 2, _2 = day (_2 = width two, right justified)
//											03, 3, 15 = hour (15 = 24 hour format)
//											04, 4 = minute
//											05, 5 = second
//											PM pm = AM PM
func UnmarshalCSVToStruct(inputStructPtr interface{}, csvPayload string, csvDelimiter string, forceNoDelimiter ...bool) error {
	if inputStructPtr == nil {
		return fmt.Errorf("InputStructPtr is Required")
	}

	if LenTrim(csvPayload) == 0 {
		return fmt.Errorf("CSV Payload is Required")
	}

	noDelimiter := false

	if len(forceNoDelimiter) > 0 && forceNoDelimiter[0] {
		noDelimiter = true
	}

	if len(csvDelimiter) == 0 && !noDelimiter {
		return fmt.Errorf("CSV Delimiter is Required")
	}

	s := reflect.ValueOf(inputStructPtr)

	if s.Kind() != reflect.Ptr {
		return fmt.Errorf("InputStructPtr Must Be Pointer")
	} else {
		s = s.Elem()
	}

	if s.Kind() != reflect.Struct {
		return fmt.Errorf("InputStructPtr Must Be Struct")
	}

	trueList := []string{"true", "yes", "on", "1", "enabled"}

	var csvElements []string

	if len(csvDelimiter) > 0 {
		csvElements = strings.Split(csvPayload, csvDelimiter)
	} else {
		for _, c := range csvPayload{
			csvElements = append(csvElements, string(c))
		}
	}

	csvLen := len(csvElements)

	if csvLen == 0 {
		return fmt.Errorf("CSV Payload Contains Zero Elements")
	}

	StructClearFields(inputStructPtr)
	SetStructFieldDefaultValues(inputStructPtr)

	for i := 0; i < s.NumField(); i++ {
		field := s.Type().Field(i)

		if o := s.FieldByName(field.Name); o.IsValid() && o.CanSet() {
			// extract struct tag values
			tagPos, ok := ParseInt32(field.Tag.Get("pos"))
			if !ok {
				continue
			} else if tagPos < 0 {
				continue
			} else if tagPos > csvLen-1 {
				continue
			}

			tagType := Trim(strings.ToLower(field.Tag.Get("type")))
			switch tagType {
			case "a":
				fallthrough
			case "n":
				fallthrough
			case "an":
				fallthrough
			case "ans":
				fallthrough
			case "b":
				fallthrough
			case "b64":
				fallthrough
			case "regex":
				fallthrough
			case "h":
				// valid type
			default:
				tagType = ""
			}

			tagRegEx := Trim(field.Tag.Get("regex"))
			if tagType != "regex" {
				tagRegEx = ""
			} else {
				if LenTrim(tagRegEx) == 0 {
					tagType = ""
				}
			}

			// unmarshal only validates max
			tagSize := Trim(strings.ToLower(field.Tag.Get("size")))
			arSize := strings.Split(tagSize, "..")
			sizeMin := 0
			sizeMax := 0
			if len(arSize) == 2 {
				sizeMin, _ = ParseInt32(arSize[0])
				sizeMax, _ = ParseInt32(arSize[1])
			} else {
				sizeMin, _ = ParseInt32(tagSize)
				sizeMax = sizeMin
			}

			/*
			// tagRange not used in unmarshal
			tagRange := Trim(strings.ToLower(field.Tag.Get("range")))
			arRange := strings.Split(tagRange, "..")
			rangeMin := 0
			rangeMax := 0
			if len(arRange) == 2 {
				rangeMin, _ = ParseInt32(arRange[0])
				rangeMax, _ = ParseInt32(arRange[1])
			} else {
				rangeMin, _ = ParseInt32(tagRange)
				rangeMax = rangeMin
			}

			// tagReq not used in unmarshal
			tagReq := Trim(strings.ToLower(field.Tag.Get("req")))
			if tagReq != "true" && tagReq != "false" {
				tagReq = ""
			}
			*/

			// if outPrefix exists, remove from csvValue
			outPrefix := Trim(field.Tag.Get("outprefix"))

			// get csv value by ordinal position
			csvValue := ""

			if LenTrim(outPrefix) == 0 {
				// ordinal based csv parsing
				if csvElements != nil {
					if tagPos > csvLen-1 {
						return fmt.Errorf("Struct Field Tag Position %d Exceeds CSV Elements", tagPos)
					} else {
						csvValue = csvElements[tagPos]
					}
				}
			} else {
				// variable element based csv, using outPrefix as the identifying key
				// instead of getting csv value from element position, acquire from outPrefix
				for _, v := range csvElements {
					if strings.ToLower(Left(v, len(outPrefix))) == strings.ToLower(outPrefix) {
						// match
						if len(v)-len(outPrefix) == 0 {
							csvValue = ""
						} else {
							csvValue = Right(v, len(v)-len(outPrefix))
						}

						break
					}
				}
			}

			// pre-process csv value with validation
			tagSetter := Trim(field.Tag.Get("setter"))
			timeFormat := Trim(field.Tag.Get("timeformat"))

			if o.Kind() != reflect.Ptr {
				switch tagType {
				case "a":
					csvValue, _ = ExtractAlpha(csvValue)
				case "n":
					csvValue, _ = ExtractNumeric(csvValue)
				case "an":
					csvValue, _ = ExtractAlphaNumeric(csvValue)
				case "ans":
					csvValue, _ = ExtractAlphaNumericPrintableSymbols(csvValue)
				case "b":
					if StringSliceContains(&trueList, strings.ToLower(csvValue)) {
						csvValue = "true"
					} else {
						csvValue = "false"
					}
				case "regex":
					csvValue, _ = ExtractByRegex(csvValue, tagRegEx)
				}

				if tagType == "a" || tagType == "an" || tagType == "ans" || tagType == "n" || tagType == "regex" {
					if sizeMax > 0 {
						if len(csvValue) > sizeMax {
							csvValue = Left(csvValue, sizeMax)
						}
					}
				}

				if LenTrim(tagSetter) > 0 {
					if ov, notFound := ReflectCall(o, tagSetter, csvValue); !notFound {
						if len(ov) == 1 {
							csvValue, _, _ = ReflectValueToString(ov[0], "", "", false, false, timeFormat)
						} else if len(ov) > 1 {
							getFirstVar := true

							if e, ok := ov[len(ov)-1].Interface().(error); ok {
								// last var is error, check if error exists
								if e != nil {
									getFirstVar = false
								}
							}

							if getFirstVar {
								csvValue, _, _ = ReflectValueToString(ov[0], "", "", false, false, timeFormat)
							}
						}
					}
				}

				// set validated csv value into corresponding struct field
				if err := ReflectStringToField(o, csvValue, timeFormat); err != nil {
					return err
				}
			} else {
				if LenTrim(tagSetter) > 0 {
					// get base type
					if baseType, _, isNilPtr := DerefPointersZero(o); isNilPtr {
						// create new struct pointer
						o.Set(reflect.New(baseType.Type()))
					}

					if ov, notFound := ReflectCall(o, tagSetter, csvValue); !notFound {
						if len(ov) == 1 {
							if ov[0].Kind() == reflect.Ptr {
								o.Set(ov[0])
							}
						} else if len(ov) > 1 {
							getFirstVar := true

							if e := DerefError(ov[len(ov)-1]); e != nil {
								getFirstVar = false
							}

							if getFirstVar {
								if ov[0].Kind() == reflect.Ptr {
									o.Set(ov[0])
								}
							}
						}
					}
				} else {
					// set validated csv value into corresponding struct pointer field
					if err := ReflectStringToField(o, csvValue, timeFormat); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

// MarshalStructToCSV will serialize struct fields defined with strug tags below, to csvPayload string (one line of csv data) using csvDelimiter,
// the csv payload ordinal position is based on the struct tag pos defined for each struct field,
// additionally processes struct tag data validation and length / range (if not valid, will set to data type default),
// this method provides data validation and if fails, will return error (for string if size exceeds max, it will truncate)
//
// Predefined Struct Tags Usable:
//		1) `pos:"1"`				// ordinal position of the field in relation to the csv parsed output expected (Zero-Based Index)
//									   NOTE: if field is mutually exclusive with one or more uniqueId, then pos # should be named the same for all uniqueIds
//											 if multiple fields are in exclusive condition, and skipBlank or skipZero, always include a blank default field as the last of unique field list
//		2) `type:"xyz"`				// data type expected:
//											A = AlphabeticOnly, N = NumericOnly 0-9, AN = AlphaNumeric, ANS = AN + PrintableSymbols,
//											H = Hex, B64 = Base64, B = true/false, REGEX = Regular Expression, Blank = Any,
//		3) `regex:"xyz"`			// if Type = REGEX, this struct tag contains the regular expression string,
//										 	regex express such as [^A-Za-z0-9_-]+
//										 	method will replace any regex matched string to blank
//		4) `size:"x..y"`			// data type size rule:
//											x = Exact size match
//											x.. = From x and up
//											..y = From 0 up to y
//											x..y = From x to y
//		5) `range:"x..y"`			// data type range value when Type is N, if underlying data type is string, method will convert first before testing
//		6) `req:"true"`				// indicates data value is required or not, true or false
//		7) `getter:"Key"`			// if field type is custom struct or enum, specify the custom method getter (no parameters allowed) that returns the expected value in first ordinal result position
// 		8) `setter:"ParseByKey`		// if field type is custom struct or enum, specify the custom method (only 1 lookup parameter value allowed) setter that sets value(s) into the field
//		9) `booltrue:"1"` 			// if field is defined, contains bool literal for true condition, such as 1 or true, that overrides default system bool literal value
//		10) `boolfalse:"0"`			// if field is defined, contains bool literal for false condition, such as 0 or false, that overrides default system bool literal value
// 		11) `uniqueid:"xyz"`		// if two or more struct field is set with the same uniqueid, then only the first encountered field with the same uniqueid will be used in marshal,
//									   NOTE: if field is mutually exclusive with one or more uniqueId, then pos # should be named the same for all uniqueIds
//		12) `skipblank:"false"`		// if true, then any fields that is blank string will be excluded from marshal (this only affects fields that are string)
//		13) `skipzero:"false"`		// if true, then any fields that are 0, 0.00, time.Zero(), false, nil will be excluded from marshal (this only affects fields that are number, bool, time, pointer)
//		14) `timeformat:"20060102"`	// for time.Time field, optional date time format, specified as:
//											2006, 06 = year,
//											01, 1, Jan, January = month,
//											02, 2, _2 = day (_2 = width two, right justified)
//											03, 3, 15 = hour (15 = 24 hour format)
//											04, 4 = minute
//											05, 5 = second
//											PM pm = AM PM
//		15) `outprefix:""`			// for marshal method, if field value is to precede with an output prefix, such as XYZ= (affects marshal queryParams / csv methods only)
//									   WARNING: if csv is variable elements count, rather than fixed count ordinal, then csv MUST include outprefix for all fields in order to properly identify target struct field
func MarshalStructToCSV(inputStructPtr interface{}, csvDelimiter string, forceNoDelimiter ...bool) (csvPayload string, err error) {
	if inputStructPtr == nil {
		return "", fmt.Errorf("InputStructPtr is Required")
	}

	noDelimiter := false

	if len(forceNoDelimiter) > 0 && forceNoDelimiter[0] {
		noDelimiter = true
	}

	if len(csvDelimiter) == 0 && !noDelimiter {
		return "", fmt.Errorf("CSV Delimiter is Required")
	}

	s := reflect.ValueOf(inputStructPtr)

	if s.Kind() != reflect.Ptr {
		return "", fmt.Errorf("InputStructPtr Must Be Pointer")
	} else {
		s = s.Elem()
	}

	if s.Kind() != reflect.Struct {
		return "", fmt.Errorf("InputStructPtr Must Be Struct")
	}

	if !IsStructFieldSet(inputStructPtr) {
		return "", nil
	}

	trueList := []string{"true", "yes", "on", "1", "enabled"}

	csvList := make([]string, s.NumField())
	csvLen := len(csvList)

	for i := 0; i < csvLen; i++ {
		csvList[i] = "{?}"	// indicates value not set, to be excluded
	}

	uniqueMap := make(map[string]string)

	for i := 0; i < s.NumField(); i++ {
		field := s.Type().Field(i)

		if o := s.FieldByName(field.Name); o.IsValid() && o.CanSet() {
			// extract struct tag values
			tagPos, ok := ParseInt32(field.Tag.Get("pos"))
			if !ok {
				continue
			} else if tagPos < 0 {
				continue
			} else if tagPos > csvLen-1 {
				continue
			}

			if tagUniqueId := Trim(field.Tag.Get("uniqueid")); len(tagUniqueId) > 0 {
				if _, ok := uniqueMap[strings.ToLower(tagUniqueId)]; ok {
					continue
				} else {
					uniqueMap[strings.ToLower(tagUniqueId)] = field.Name
				}
			}

			tagType := Trim(strings.ToLower(field.Tag.Get("type")))
			switch tagType {
			case "a":
				fallthrough
			case "n":
				fallthrough
			case "an":
				fallthrough
			case "ans":
				fallthrough
			case "b":
				fallthrough
			case "b64":
				fallthrough
			case "regex":
				fallthrough
			case "h":
				// valid type
			default:
				tagType = ""
			}

			tagRegEx := Trim(field.Tag.Get("regex"))
			if tagType != "regex" {
				tagRegEx = ""
			} else {
				if LenTrim(tagRegEx) == 0 {
					tagType = ""
				}
			}

			tagSize := Trim(strings.ToLower(field.Tag.Get("size")))
			arSize := strings.Split(tagSize, "..")
			sizeMin := 0
			sizeMax := 0
			if len(arSize) == 2 {
				sizeMin, _ = ParseInt32(arSize[0])
				sizeMax, _ = ParseInt32(arSize[1])
			} else {
				sizeMin, _ = ParseInt32(tagSize)
				sizeMax = sizeMin
			}

			tagRange := Trim(strings.ToLower(field.Tag.Get("range")))
			arRange := strings.Split(tagRange, "..")
			rangeMin := 0
			rangeMax := 0
			if len(arRange) == 2 {
				rangeMin, _ = ParseInt32(arRange[0])
				rangeMax, _ = ParseInt32(arRange[1])
			} else {
				rangeMin, _ = ParseInt32(tagRange)
				rangeMax = rangeMin
			}

			tagReq := Trim(strings.ToLower(field.Tag.Get("req")))
			if tagReq != "true" && tagReq != "false" {
				tagReq = ""
			}

			if tagGetter := Trim(field.Tag.Get("getter")); len(tagGetter) > 0 {
				if ov, notFound := ReflectCall(o, tagGetter); !notFound {
					if len(ov) > 0 {
						o = ov[0]
					}
				}
			}

			// get csv value from current struct field
			var boolTrue, boolFalse, timeFormat, outPrefix string
			var skipBlank, skipZero bool

			if vs := GetStructTagsValueSlice(field, "booltrue", "boolfalse", "skipblank", "skipzero", "timeformat", "outprefix"); len(vs) == 6 {
				boolTrue = vs[0]
				boolFalse = vs[1]
				skipBlank = IsBool(vs[2])
				skipZero = IsBool(vs[3])
				timeFormat = vs[4]
				outPrefix = vs[5]
			}

			fv, skip, e := ReflectValueToString(o, boolTrue, boolFalse, skipBlank, skipZero, timeFormat)

			if e != nil {
				if tagUniqueId := Trim(field.Tag.Get("uniqueid")); len(tagUniqueId) > 0 {
					if _, ok := uniqueMap[strings.ToLower(tagUniqueId)]; ok {
						// remove uniqueid if skip
						delete(uniqueMap, strings.ToLower(tagUniqueId))
					}
				}

				return "", e
			}

			if skip {
				if tagUniqueId := Trim(field.Tag.Get("uniqueid")); len(tagUniqueId) > 0 {
					if _, ok := uniqueMap[strings.ToLower(tagUniqueId)]; ok {
						// remove uniqueid if skip
						delete(uniqueMap, strings.ToLower(tagUniqueId))
					}
				}

				continue
			}

			// validate output csv value
			switch tagType {
			case "a":
				fv, _ = ExtractAlpha(fv)
			case "n":
				fv, _ = ExtractNumeric(fv)
			case "an":
				fv, _ = ExtractAlphaNumeric(fv)
			case "ans":
				fv, _ = ExtractAlphaNumericPrintableSymbols(fv)
			case "b":
				if StringSliceContains(&trueList, strings.ToLower(fv)) {
					fv = "true"
				} else {
					fv = "false"
				}
			case "regex":
				fv, _ = ExtractByRegex(fv, tagRegEx)
			case "h":
				// not validated
			case "b64":
				// not validated
			}

			if tagType == "a" || tagType == "an" || tagType == "ans" || tagType == "n" || tagType == "regex" {
				if sizeMin > 0 && len(fv) > 0 {
					if len(fv) < sizeMin {
						return "", fmt.Errorf("%s Min Length is %d", field.Name, sizeMin)
					}
				}

				if sizeMax > 0 && len(fv) > sizeMax {
					fv = Left(fv, sizeMax)
				}
			}

			if tagType == "n" {
				n, ok := ParseInt32(fv)

				if ok {
					if rangeMin > 0 {
						if n < rangeMin {
							if !(n == 0 && tagReq != "true") {
								return "", fmt.Errorf("%s Range Minimum is %d", field.Name, rangeMin)
							}
						}
					}

					if rangeMax > 0 {
						if n > rangeMax {
							return "", fmt.Errorf("%s Range Maximum is %d", field.Name, rangeMax)
						}
					}
				}
			}

			if tagReq == "true" && len(fv) == 0 {
				return "", fmt.Errorf("%s is a Required Field", field.Name)
			}

			// store fv into sorted slice
			csvList[tagPos] = outPrefix + fv
		}
	}

	for _, v := range csvList {
		if v != "{?}" {
			if LenTrim(csvPayload) > 0 {
				csvPayload += csvDelimiter
			}

			csvPayload += v
		}
	}

	return csvPayload, nil
}


