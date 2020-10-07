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
			tag := field.Tag.Get(tagName)

			if LenTrim(tag) == 0 {
				tag = field.Name
			}

			if tag != "-" {
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
			tag := field.Tag.Get(tagName)

			if LenTrim(tag) == 0 {
				tag = field.Name
			}

			if tag != "-" {
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
			switch o.Kind() {
			case reflect.String:
				if LenTrim(o.String()) > 0 {
					return true
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
					return true
				}
			case reflect.Float32:
				fallthrough
			case reflect.Float64:
				if o.Float() != 0 {
					return true
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
					return true
				}
			case reflect.Ptr:
				if !o.IsNil() {
					return true
				}
			default:
				switch f := o.Interface().(type) {
				case sql.NullString:
					if f.Valid {
						return true
					}
				case sql.NullBool:
					if f.Valid {
						return true
					}
				case sql.NullFloat64:
					if f.Valid {
						return true
					}
				case sql.NullInt32:
					if f.Valid {
						return true
					}
				case sql.NullInt64:
					if f.Valid {
						return true
					}
				case sql.NullTime:
					if f.Valid {
						return true
					}
				case time.Time:
					if !f.IsZero() {
						return true
					}
				}
			}
		}
	}

	return false
}

// UnmarshalCSVToStruct will parse csvPayload string (one line of csv data) using csvDelimiter,
// and set parsed csv element value into struct fields based on Ordinal Position defined via struct tag,
// additionally processes struct tag data validation and length / range (if not valid, will set to data type default)
//
// Predefined Struct Tags Usable:
//		1) `pos:"1"`		// ordinal position of the field in relation to the csv parsed output expected (Zero-Based Index)
//		2) `type:"xyz"`		// data type expected:
//									A = AlphabeticOnly, N = NumericOnly 0-9, AN = AlphaNumeric, ANS = AN + PrintableSymbols,
//									H = Hex, B64 = Base64, B = true/false, REGEX = Regular Expression, Blank = Any,
//		3) `regex:"xyz"`	// if Type = REGEX, this struct tag contains the regular expression string,
//										 regex express such as [^A-Za-z0-9_-]+
//										 method will replace any regex matched string to blank
//		4) `size:"x..y"`	// data type size rule:
//									x = Exact size match
//									x.. = From x and up
//									..y = From 0 up to y
//									x..y = From x to y
//		5) `range:"x..y"`	// data type range value when Type is N, if underlying data type is string, method will convert first before testing
//		6) `req:"true"`		// indicates data value is required or not, true or false
func UnmarshalCSVToStruct(inputStructPtr interface{}, csvPayload string, csvDelimiter string) error {
	if inputStructPtr == nil {
		return fmt.Errorf("InputStructPtr is Required")
	}

	if LenTrim(csvPayload) == 0 {
		return fmt.Errorf("CSV Payload is Required")
	}

	if LenTrim(csvDelimiter) == 0 {
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

	csvElements := strings.Split(csvPayload, csvDelimiter)
	csvLen := len(csvElements)

	if csvLen == 0 {
		return fmt.Errorf("CSV Payload Contains Zero Elements")
	}

	StructClearFields(inputStructPtr)

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

			// get csv value by ordinal position
			csvValue := csvElements[tagPos]

			// pre-process csv value with validation
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

			// set validated csv value into corresponding struct field
			switch o.Kind() {
			case reflect.String:
				o.SetString(csvValue)
			case reflect.Bool:
				o.SetBool(IsBool(csvValue))
			case reflect.Int8:
				fallthrough
			case reflect.Int16:
				fallthrough
			case reflect.Int:
				fallthrough
			case reflect.Int32:
				fallthrough
			case reflect.Int64:
				i64, _ := ParseInt64(csvValue)
				if !o.OverflowInt(i64) {
					o.SetInt(i64)
				}
			case reflect.Float32:
				fallthrough
			case reflect.Float64:
				f64, _ := ParseFloat64(csvValue)
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
				ui64 := StrToUint64(csvValue)
				if !o.OverflowUint(ui64) {
					o.SetUint(ui64)
				}
			case reflect.Ptr:
				o2 := o.Elem()
				switch o2.Interface().(type) {
				case int:
					i64, _ := ParseInt64(csvValue)
					if !o2.OverflowInt(i64) {
						o2.SetInt(i64)
					}
				case int8:
					i64, _ := ParseInt64(csvValue)
					if !o2.OverflowInt(i64) {
						o2.SetInt(i64)
					}
				case int16:
					i64, _ := ParseInt64(csvValue)
					if !o2.OverflowInt(i64) {
						o2.SetInt(i64)
					}
				case int32:
					i64, _ := ParseInt64(csvValue)
					if !o2.OverflowInt(i64) {
						o2.SetInt(i64)
					}
				case int64:
					i64, _ := ParseInt64(csvValue)
					if !o2.OverflowInt(i64) {
						o2.SetInt(i64)
					}
				case float32:
					f64, _ := ParseFloat64(csvValue)
					if !o2.OverflowFloat(f64) {
						o2.SetFloat(f64)
					}
				case float64:
					f64, _ := ParseFloat64(csvValue)
					if !o2.OverflowFloat(f64) {
						o2.SetFloat(f64)
					}
				case uint:
					if !o2.OverflowUint(StrToUint64(csvValue)) {
						o2.SetUint(StrToUint64(csvValue))
					}
				case uint64:
					if !o2.OverflowUint(StrToUint64(csvValue)) {
						o2.SetUint(StrToUint64(csvValue))
					}
				case string:
					o2.SetString(csvValue)
				case bool:
					o2.SetBool(IsBool(csvValue))
				case time.Time:
					o2.Set(reflect.ValueOf(ParseDate(csvValue)))
				default:
					return fmt.Errorf(o2.Type().Name() + " Unhandled")
				}
			default:
				switch o.Interface().(type) {
				case sql.NullString:
					o.Set(reflect.ValueOf(sql.NullString{String: csvValue, Valid: true}))
				case sql.NullBool:
					o.Set(reflect.ValueOf(sql.NullBool{Bool: IsBool(csvValue), Valid: true}))
				case sql.NullFloat64:
					f64, _ := ParseFloat64(csvValue)
					o.Set(reflect.ValueOf(sql.NullFloat64{Float64: f64, Valid: true}))
				case sql.NullInt32:
					i32, _ := ParseInt32(csvValue)
					o.Set(reflect.ValueOf(sql.NullInt32{Int32: int32(i32), Valid: true}))
				case sql.NullInt64:
					i64, _ := ParseInt64(csvValue)
					o.Set(reflect.ValueOf(sql.NullInt64{Int64: i64, Valid: true}))
				case sql.NullTime:
					tv := ParseDateTime(csvValue)
					o.Set(reflect.ValueOf(sql.NullTime{Time: tv, Valid: true}))
				case time.Time:
					o.Set(reflect.ValueOf(ParseDateTime(csvValue)))
				default:
					return fmt.Errorf(o.Type().Name() + " Unhandled")
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
//		1) `pos:"1"`		// ordinal position of the field in relation to the csv parsed output expected (Zero-Based Index)
//		2) `type:"xyz"`		// data type expected:
//									A = AlphabeticOnly, N = NumericOnly 0-9, AN = AlphaNumeric, ANS = AN + PrintableSymbols,
//									H = Hex, B64 = Base64, B = true/false, REGEX = Regular Expression, Blank = Any,
//		3) `regex:"xyz"`	// if Type = REGEX, this struct tag contains the regular expression string,
//										 regex express such as [^A-Za-z0-9_-]+
//										 method will replace any regex matched string to blank
//		4) `size:"x..y"`	// data type size rule:
//									x = Exact size match
//									x.. = From x and up
//									..y = From 0 up to y
//									x..y = From x to y
//		5) `range:"x..y"`	// data type range value when Type is N, if underlying data type is string, method will convert first before testing
//		6) `req:"true"`		// indicates data value is required or not, true or false
func MarshalStructToCSV(inputStructPtr interface{}, csvDelimiter string) (csvPayload string, err error) {
	if inputStructPtr == nil {
		return "", fmt.Errorf("InputStructPtr is Required")
	}

	if LenTrim(csvDelimiter) == 0 {
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

			// get csv value from current struct field
			fv := ""

			switch o.Kind() {
			case reflect.String:
				fv = o.String()
			case reflect.Bool:
				fv = BoolToString(o.Bool())
			case reflect.Int8:
				fallthrough
			case reflect.Int16:
				fallthrough
			case reflect.Int:
				fallthrough
			case reflect.Int32:
				fallthrough
			case reflect.Int64:
				 fv = Int64ToString(o.Int())
			case reflect.Float32:
				fallthrough
			case reflect.Float64:
				fv = FloatToString(o.Float())
			case reflect.Uint8:
				fallthrough
			case reflect.Uint16:
				fallthrough
			case reflect.Uint:
				fallthrough
			case reflect.Uint32:
				fallthrough
			case reflect.Uint64:
				fv = UInt64ToString(o.Uint())
			case reflect.Ptr:
				o2 := o.Elem()
				switch f := o2.Interface().(type) {
				case int:
					fv = Itoa(f)
				case int8:
					fv = Itoa(int(f))
				case int16:
					fv = Itoa(int(f))
				case int32:
					fv = Itoa(int(f))
				case int64:
					fv = Int64ToString(f)
				case float32:
					fv = Float32ToString(f)
				case float64:
					fv = FloatToString(f)
				case uint:
					fv = UintToStr(f)
				case uint64:
					fv = UInt64ToString(f)
				case string:
					fv = f
				case bool:
					fv = BoolToString(f)
				case time.Time:
					fv = FormatDateTime(f)
				default:
					return "", fmt.Errorf(field.Name + " Unhandled")
				}
			default:
				switch f := o.Interface().(type) {
				case sql.NullString:
					fv = FromNullString(f)
				case sql.NullBool:
					fv = BoolToString(FromNullBool(f))
				case sql.NullFloat64:
					fv = FloatToString(FromNullFloat64(f))
				case sql.NullInt32:
					fv = Itoa(FromNullInt(f))
				case sql.NullInt64:
					fv = Int64ToString(FromNullInt64(f))
				case sql.NullTime:
					fv = FormatDateTime(FromNullTime(f))
				case time.Time:
					fv = FormatDateTime(f)
				default:
					return "", fmt.Errorf(field.Name + " Unhandled")
				}
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
				if sizeMin > 0 {
					if len(fv) < sizeMin {
						return "", fmt.Errorf("%s Min Length is %d", field.Name, sizeMin)
					}
				}

				if sizeMax > 0 {
					if len(fv) > sizeMax {
						fv = Left(fv, sizeMax)
					}
				}
			}

			if tagType == "n" {
				n, _ := ParseInt32(fv)

				if rangeMin > 0 {
					if n < rangeMin {
						return "", fmt.Errorf("%s Range Minimum is %d", field.Name, rangeMin)
					}
				}

				if rangeMax > 0 {
					if n > rangeMax {
						return "", fmt.Errorf("%s Range Maximum is %d", field.Name, rangeMax)
					}
				}
			}

			if tagReq == "true" && len(fv) == 0 {
				return "", fmt.Errorf("%s is a Required Field", field.Name)
			}

			// store fv into sorted slice
			csvList[tagPos] = fv
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


