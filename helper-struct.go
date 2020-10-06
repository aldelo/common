package helper

import (
	"database/sql"
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

// StructToQueryParams marshals a struct pointer's fields to query params string,
// output query param names are based on values given in tagName,
// to exclude certain struct fields from being marshaled, include excludeTagName with - as value in struct definition
func StructToQueryParams(inputStructPtr interface{}, tagName string, excludeTagName string) (string, error) {
	if inputStructPtr == nil {
		return "", fmt.Errorf("StructToQueryParams Require Input Struct Variable Pointer")
	}

	if LenTrim(tagName) == 0 {
		return "", fmt.Errorf("StructToQueryParams Require TagName (Tag Name defines query parameter name)")
	}

	s := reflect.ValueOf(inputStructPtr)

	if s.Kind() != reflect.Ptr {
		return "", fmt.Errorf("StructToQueryParams Expects inputStructPtr To Be a Pointer")
	} else {
		s = s.Elem()
	}

	if s.Kind() != reflect.Struct {
		return "", fmt.Errorf("StructToQueryParams Require Struct Object")
	}

	output := ""

	for i := 0; i < s.NumField(); i++ {
		field := s.Type().Field(i)

		if o := s.FieldByName(field.Name); o.IsValid() {
			if tag := field.Tag.Get(tagName); LenTrim(tag) > 0 {
				if LenTrim(excludeTagName) > 0 {
					if field.Tag.Get(excludeTagName) == "-" {
						continue
					}
				}

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
						continue
					}
				}

				if LenTrim(output) > 0 {
					output += "&"
				}

				output += fmt.Sprintf("%s=%s", tag, url.PathEscape(buf))
			}
		}
	}

	if LenTrim(output) == 0 {
		return "", fmt.Errorf("StructToQueryParameters Yielded Blank Output")
	} else {
		return output, nil
	}
}

// StructToJson marshals a struct pointer's fields to json string,
// output json names are based on values given in tagName,
// to exclude certain struct fields from being marshaled, include excludeTagName with - as value in struct definition
func StructToJson(inputStructPtr interface{}, tagName string, excludeTagName string) (string, error) {
	if inputStructPtr == nil {
		return "", fmt.Errorf("StructToJson Require Input Struct Variable Pointer")
	}

	if LenTrim(tagName) == 0 {
		return "", fmt.Errorf("StructToJson Require TagName (Tag Name defines Json name)")
	}

	s := reflect.ValueOf(inputStructPtr)

	if s.Kind() != reflect.Ptr {
		return "", fmt.Errorf("StructToJson Expects inputStructPtr To Be a Pointer")
	} else {
		s = s.Elem()
	}

	if s.Kind() != reflect.Struct {
		return "", fmt.Errorf("StructToJson Require Struct Object")
	}

	output := ""

	for i := 0; i < s.NumField(); i++ {
		field := s.Type().Field(i)

		if o := s.FieldByName(field.Name); o.IsValid() {
			if tag := field.Tag.Get(tagName); LenTrim(tag) > 0 {
				if LenTrim(excludeTagName) > 0 {
					if field.Tag.Get(excludeTagName) == "-" {
						continue
					}
				}

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
						continue
					}
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
		return "", fmt.Errorf("StructToJson Yielded Blank Output")
	} else {
		return fmt.Sprintf("{%s}", output), nil
	}
}

// SliceStructToJson accepts a slice of struct pointer, then using tagName and excludeTagName to marshal to json array
// To pass in inputSliceStructPtr, convert slice of actual objects at the calling code, using SliceObjectsToSliceInterface()
func SliceStructToJson(inputSliceStructPtr []interface{}, tagName string, excludeTagName string) (jsonArrayOutput string, err error) {
	if len(inputSliceStructPtr) == 0 {
		return "", fmt.Errorf("Input Slice Struct Pointer Nil")
	}

	for _, v := range inputSliceStructPtr {
		if s, e := StructToJson(v, tagName, excludeTagName); e != nil {
			return "", fmt.Errorf("StructToJson Failed: %s", e)
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
		return "", fmt.Errorf("SliceStructToJson Yielded Blank String")
	}
}


