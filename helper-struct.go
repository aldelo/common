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

// =====================================================================================================================
// Csv Parser Helpers
// =====================================================================================================================

type csvFieldConfig struct {
	pos         int
	tagType     string
	tagRegEx    string
	sizeMin     int
	sizeMax     int
	tagModulo   int
	rangeMin    int
	rangeMax    int
	tagReq      string
	boolTrue    string
	boolFalse   string
	timeFormat  string
	outPrefix   string
	skipBlank   bool
	skipZero    bool
	zeroBlank   bool
	defVal      string
	validate    string
	uniqueID    string
	tagGetter   string
	getterBase  bool
	getterParam bool
}

func csvComputeBufferLength(s reflect.Value) (int, error) { // extracted
	maxPos := -1
	for i := 0; i < s.NumField(); i++ {
		if tagPos, ok := ParseInt32(s.Type().Field(i).Tag.Get("pos")); ok && tagPos >= 0 {
			if int(tagPos) > maxPos {
				maxPos = int(tagPos)
			}
		}
	}
	if maxPos < 0 {
		return 0, nil
	}
	return maxPos + 1, nil
}

func csvParseFieldConfig(field reflect.StructField) (cfg csvFieldConfig, ok bool) { // extracted
	tagPos, posOK := ParseInt32(field.Tag.Get("pos"))
	if !posOK || tagPos < 0 {
		return cfg, false
	}
	cfg.pos = int(tagPos)
	cfg.uniqueID = Trim(field.Tag.Get("uniqueid"))
	cfg.tagType = Trim(strings.ToLower(field.Tag.Get("type")))
	switch cfg.tagType {
	case "a", "n", "an", "ans", "b", "b64", "regex", "h":
	default:
		cfg.tagType = ""
	}

	cfg.tagRegEx = Trim(field.Tag.Get("regex"))
	if cfg.tagType != "regex" || LenTrim(cfg.tagRegEx) == 0 {
		if cfg.tagType == "regex" && LenTrim(cfg.tagRegEx) == 0 {
			cfg.tagType = ""
		}
		cfg.tagRegEx = ""
	}

	cfg.tagReq = Trim(strings.ToLower(field.Tag.Get("req")))
	if cfg.tagReq != "true" && cfg.tagReq != "false" {
		cfg.tagReq = ""
	}

	cfg.defVal = field.Tag.Get("def")
	cfg.validate = Trim(field.Tag.Get("validate"))

	tagSize := Trim(strings.ToLower(field.Tag.Get("size")))
	arModulo := strings.Split(tagSize, "+%")
	if len(arModulo) == 2 {
		tagSize = arModulo[0]
		if cfg.tagModulo, _ = ParseInt32(arModulo[1]); cfg.tagModulo < 0 {
			cfg.tagModulo = 0
		}
	}
	arSize := strings.Split(tagSize, "..")
	if len(arSize) == 2 {
		cfg.sizeMin, _ = ParseInt32(arSize[0])
		cfg.sizeMax, _ = ParseInt32(arSize[1])
	} else {
		cfg.sizeMin, _ = ParseInt32(tagSize)
		cfg.sizeMax = cfg.sizeMin
	}

	tagRange := Trim(strings.ToLower(field.Tag.Get("range")))
	arRange := strings.Split(tagRange, "..")
	if len(arRange) == 2 {
		cfg.rangeMin, _ = ParseInt32(arRange[0])
		cfg.rangeMax, _ = ParseInt32(arRange[1])
	} else {
		cfg.rangeMin, _ = ParseInt32(tagRange)
		cfg.rangeMax = cfg.rangeMin
	}

	if vs := GetStructTagsValueSlice(field, "booltrue", "boolfalse", "skipblank", "skipzero", "timeformat", "outprefix", "zeroblank"); len(vs) == 7 {
		cfg.boolTrue = vs[0]
		cfg.boolFalse = vs[1]
		cfg.skipBlank, _ = ParseBool(vs[2])
		cfg.skipZero, _ = ParseBool(vs[3])
		cfg.timeFormat = vs[4]
		cfg.outPrefix = vs[5]
		cfg.zeroBlank, _ = ParseBool(vs[6])
	}

	// safer getter parsing (no Left/Right on short strings)
	if tagGetter := Trim(field.Tag.Get("getter")); len(tagGetter) > 0 {
		lg := strings.ToLower(tagGetter)
		if strings.HasPrefix(lg, "base.") {
			cfg.getterBase = true
			tagGetter = tagGetter[5:]
			lg = strings.ToLower(tagGetter)
		}
		if strings.HasSuffix(lg, "(x)") && len(tagGetter) >= 3 {
			cfg.getterParam = true
			tagGetter = tagGetter[:len(tagGetter)-3]
		}
		cfg.tagGetter = tagGetter
	}

	return cfg, true
}

func csvApplyGetter(s reflect.Value, cfg csvFieldConfig, o reflect.Value, boolTrue, boolFalse string, skipBlank, skipZero bool, timeFormat string, zeroBlank bool) reflect.Value { // extracted
	if len(cfg.tagGetter) == 0 {
		return o
	}

	isBase := cfg.getterBase
	useParam := cfg.getterParam
	paramVal := ""
	var paramSlice interface{}

	if useParam {
		if o.Kind() != reflect.Slice {
			paramVal, _, _ = ReflectValueToString(o, boolTrue, boolFalse, skipBlank, skipZero, timeFormat, zeroBlank)
		} else if o.Len() > 0 {
			paramSlice = o.Slice(0, o.Len()).Interface()
		}
	}

	var ov []reflect.Value
	var notFound bool

	if isBase {
		if useParam {
			if paramSlice == nil {
				ov, notFound = ReflectCall(s.Addr(), cfg.tagGetter, paramVal)
			} else {
				ov, notFound = ReflectCall(s.Addr(), cfg.tagGetter, paramSlice)
			}
		} else {
			ov, notFound = ReflectCall(s.Addr(), cfg.tagGetter)
		}
	} else {
		if useParam {
			if paramSlice == nil {
				ov, notFound = ReflectCall(o, cfg.tagGetter, paramVal)
			} else {
				ov, notFound = ReflectCall(o, cfg.tagGetter, paramSlice)
			}
		} else {
			ov, notFound = ReflectCall(o, cfg.tagGetter)
		}
	}

	if !notFound && len(ov) > 0 {
		return ov[0]
	}

	return o
}

func csvValidateAndNormalize(fv string, cfg csvFieldConfig, oldVal reflect.Value, hasGetter bool, trueList []string) (string, bool, error) {
	origFv := fv

	switch cfg.tagType {
	case "a":
		fv, _ = ExtractAlpha(fv)
	case "n":
		fv, _ = ExtractNumeric(fv)
	case "an":
		fv, _ = ExtractAlphaNumeric(fv)
	case "ans":
		if !hasGetter {
			fv, _ = ExtractAlphaNumericPrintableSymbols(fv)
		}
	case "b":
		if len(cfg.boolTrue) == 0 && len(cfg.boolFalse) == 0 {
			if StringSliceContains(&trueList, strings.ToLower(fv)) {
				fv = "true"
			} else {
				fv = "false"
			}
		} else if Trim(cfg.boolTrue) == Trim(cfg.boolFalse) && fv == "false" {
			return "", true, nil
		}
	case "regex":
		fv, _ = ExtractByRegex(fv, cfg.tagRegEx)
	case "h":
		fv, _ = ExtractHex(fv)
	case "b64":
		fv, _ = ExtractAlphaNumericPrintableSymbols(fv)
	}

	if cfg.boolFalse == " " && origFv == "false" && len(cfg.outPrefix) > 0 {
		return "", true, nil
	}

	if len(fv) == 0 && len(cfg.defVal) > 0 {
		fv = cfg.defVal
	}

	if cfg.tagType == "a" || cfg.tagType == "an" || cfg.tagType == "ans" || cfg.tagType == "n" || cfg.tagType == "regex" || cfg.tagType == "h" || cfg.tagType == "b64" {
		if cfg.sizeMin > 0 && len(fv) > 0 && len(fv) < cfg.sizeMin {
			return "", false, fmt.Errorf("Min Length is %d", cfg.sizeMin)
		}
		if cfg.sizeMax > 0 && len(fv) > cfg.sizeMax {
			fv = Left(fv, cfg.sizeMax)
		}
		if cfg.tagModulo > 0 && len(fv)%cfg.tagModulo != 0 {
			return "", false, fmt.Errorf("Expects Value In Blocks of %d Characters", cfg.tagModulo)
		}
	}

	// enforce numeric validation for type "n" even during marshal normalization
	if cfg.tagType == "n" {
		n, ok := ParseInt32(fv)
		if len(fv) > 0 && !ok { // previously silently accepted non-numeric
			return "", false, fmt.Errorf("expects numeric value")
		}
		if ok {
			if cfg.rangeMin > 0 && n < cfg.rangeMin && !(n == 0 && cfg.tagReq != "true") {
				return "", false, fmt.Errorf("Range Minimum is %d", cfg.rangeMin)
			}
			if cfg.rangeMax > 0 && n > cfg.rangeMax {
				return "", false, fmt.Errorf("Range Maximum is %d", cfg.rangeMax)
			}
		}
	}

	if cfg.tagReq == "true" && len(fv) == 0 {
		return "", false, fmt.Errorf("is a Required Field")
	}

	return fv, false, nil
}

func csvValidateCustom(fv string, cfg csvFieldConfig, tagReq string, s reflect.Value, field reflect.StructField) error { // extracted
	if valData := Trim(cfg.validate); len(valData) >= 3 {
		valComp := Left(valData, 2)
		valData = Right(valData, len(valData)-2)

		switch valComp {
		case "==":
			valAr := strings.Split(valData, "||")
			if len(valAr) <= 1 {
				if strings.ToLower(fv) != strings.ToLower(valData) && (len(fv) > 0 || tagReq == "true") {
					return fmt.Errorf("Validation Failed: Expected To Match '%s', But Received '%s'", valData, fv)
				}
			} else {
				found := false
				for _, va := range valAr {
					if strings.ToLower(fv) == strings.ToLower(va) {
						found = true
						break
					}
				}
				if !found && (len(fv) > 0 || tagReq == "true") {
					return fmt.Errorf("Validation Failed: Expected To Match '%s', But Received '%s'", strings.ReplaceAll(valData, "||", " or "), fv)
				}
			}
		case "!=":
			valAr := strings.Split(valData, "&&")
			if len(valAr) <= 1 {
				if strings.ToLower(fv) == strings.ToLower(valData) && (len(fv) > 0 || tagReq == "true") {
					return fmt.Errorf("Validation Failed: Expected To Not Match '%s', But Received '%s'", valData, fv)
				}
			} else {
				found := false
				for _, va := range valAr {
					if strings.ToLower(fv) == strings.ToLower(va) {
						found = true
						break
					}
				}
				if found && (len(fv) > 0 || tagReq == "true") {
					return fmt.Errorf("Validation Failed: Expected To Not Match '%s', But Received '%s'", strings.ReplaceAll(valData, "&&", " and "), fv)
				}
			}
		case "<=":
			if valNum, valOk := ParseFloat64(valData); valOk {
				if srcNum, _ := ParseFloat64(fv); srcNum > valNum && (len(fv) > 0 || tagReq == "true") {
					return fmt.Errorf("Validation Failed: Expected To Be Less or Equal To '%s', But Received '%s'", valData, fv)
				}
			}
		case "<<":
			if valNum, valOk := ParseFloat64(valData); valOk {
				if srcNum, _ := ParseFloat64(fv); srcNum >= valNum && (len(fv) > 0 || tagReq == "true") {
					return fmt.Errorf("Validation Failed: Expected To Be Less Than '%s', But Received '%s'", valData, fv)
				}
			}
		case ">=":
			if valNum, valOk := ParseFloat64(valData); valOk {
				if srcNum, _ := ParseFloat64(fv); srcNum < valNum && (len(fv) > 0 || tagReq == "true") {
					return fmt.Errorf("Validation Failed: Expected To Be Greater or Equal To '%s', But Received '%s'", valData, fv)
				}
			}
		case ">>":
			if valNum, valOk := ParseFloat64(valData); valOk {
				if srcNum, _ := ParseFloat64(fv); srcNum <= valNum && (len(fv) > 0 || tagReq == "true") {
					return fmt.Errorf("Validation Failed: Expected To Be Greater Than '%s', But Received '%s'", valData, fv)
				}
			}
		case ":=":
			if len(valData) > 0 {
				if retV, nf := ReflectCall(s.Addr(), valData); !nf && len(retV) > 0 {
					// also honor error returns and any boolean false result
					if err := DerefError(retV[len(retV)-1]); err != nil {
						return fmt.Errorf("Validation On %s() Failed: %s", valData, err.Error())
					}
					if retV[0].Kind() == reflect.Bool && !retV[0].Bool() {
						return fmt.Errorf("Validation Failed: %s() Returned Result is False", valData)
					}
				}
			}
		}
	}
	return nil
}

// =====================================================================================================================
// Csv Unmarshal Helpers
// =====================================================================================================================

type csvUnmarshalConfig struct {
	pos         int32
	tagType     string
	tagRegEx    string
	sizeMin     int32
	sizeMax     int32
	tagModulo   int32
	tagReq      string
	tagSetter   string
	setterBase  bool
	timeFormat  string
	tagRangeMin int32
	tagRangeMax int32
	outPrefix   string
	boolTrue    string
	boolFalse   string
	validate    string
}

func csvParseUnmarshalConfig(field reflect.StructField) (cfg csvUnmarshalConfig, ok bool) {
	tagPosBuf := field.Tag.Get("pos")
	tagPos, posOK := ParseInt32(tagPosBuf)
	if !posOK {
		if tagPosBuf != "-" || LenTrim(field.Tag.Get("setter")) == 0 {
			return cfg, false
		}
		cfg.pos = -1
	} else {
		if tagPos < 0 {
			return cfg, false
		}
		cfg.pos = int32(tagPos)
	}

	cfg.tagType = Trim(strings.ToLower(field.Tag.Get("type")))
	switch cfg.tagType {
	case "a", "n", "an", "ans", "b", "b64", "regex", "h":
	default:
		cfg.tagType = ""
	}

	cfg.tagRegEx = Trim(field.Tag.Get("regex"))
	if cfg.tagType != "regex" {
		cfg.tagRegEx = ""
	} else if LenTrim(cfg.tagRegEx) == 0 {
		cfg.tagType = ""
		cfg.tagRegEx = ""
	}

	cfg.tagReq = Trim(strings.ToLower(field.Tag.Get("req")))
	if cfg.tagReq != "true" && cfg.tagReq != "false" {
		cfg.tagReq = ""
	}

	cfg.tagSetter = Trim(field.Tag.Get("setter"))
	if len(cfg.tagSetter) > 0 && strings.HasPrefix(strings.ToLower(cfg.tagSetter), "base.") {
		cfg.setterBase = true
		cfg.tagSetter = cfg.tagSetter[5:]
	}

	cfg.timeFormat = Trim(field.Tag.Get("timeformat"))
	cfg.outPrefix = Trim(field.Tag.Get("outprefix"))
	cfg.boolTrue = Trim(field.Tag.Get("booltrue"))
	cfg.boolFalse = Trim(field.Tag.Get("boolfalse"))
	cfg.validate = Trim(field.Tag.Get("validate"))

	// size
	tagSize := Trim(strings.ToLower(field.Tag.Get("size")))
	arModulo := strings.Split(tagSize, "+%")
	if len(arModulo) == 2 {
		tagSize = arModulo[0]
		if n, _ := ParseInt32(arModulo[1]); n < 0 {
			cfg.tagModulo = 0
		} else {
			cfg.tagModulo = int32(n)
		}
	}
	arSize := strings.Split(tagSize, "..")
	if len(arSize) == 2 {
		iMin, _ := ParseInt32(arSize[0])
		cfg.sizeMin = int32(iMin)
		iMax, _ := ParseInt32(arSize[1])
		cfg.sizeMax = int32(iMax)
	} else {
		iMin, _ := ParseInt32(tagSize)
		cfg.sizeMin = int32(iMin)
		cfg.sizeMax = int32(iMin)
	}

	// range
	tagRange := Trim(strings.ToLower(field.Tag.Get("range")))
	arRange := strings.Split(tagRange, "..")
	if len(arRange) == 2 {
		iMin, _ := ParseInt32(arRange[0])
		cfg.tagRangeMin = int32(iMin)
		iMax, _ := ParseInt32(arRange[1])
		cfg.tagRangeMax = int32(iMax)
	} else {
		iMin, _ := ParseInt32(tagRange)
		cfg.tagRangeMin = int32(iMin)
		cfg.tagRangeMax = int32(iMin)
	}

	return cfg, true
}

func csvExtractValue(csvElements []string, cfg csvUnmarshalConfig, prefixProcessedMap map[string]string) (string, bool) {
	// returns value, ok
	if cfg.pos < 0 {
		return "", false
	}
	if LenTrim(cfg.outPrefix) == 0 {
		if int(cfg.pos) >= len(csvElements) {
			return "", false
		}
		return csvElements[cfg.pos], true
	}

	// variable-length with outprefix
	for _, v := range csvElements {
		// guard against short strings before slicing
		if len(v) < len(cfg.outPrefix) {
			continue
		}
		if strings.ToLower(Left(v, len(cfg.outPrefix))) == strings.ToLower(cfg.outPrefix) {
			if _, ok := prefixProcessedMap[strings.ToLower(cfg.outPrefix)]; ok {
				continue
			}
			prefixProcessedMap[strings.ToLower(cfg.outPrefix)] = Itoa(int(cfg.pos))
			if len(v) == len(cfg.outPrefix) {
				return "", true
			}
			return Right(v, len(v)-len(cfg.outPrefix)), true
		}
	}
	return "", false
}

func csvPreprocessValue(raw string, cfg csvUnmarshalConfig, hasSetter bool, trueList []string) (string, error) {
	csvValue := raw

	switch cfg.tagType {
	case "a":
		csvValue, _ = ExtractAlpha(csvValue)
	case "n":
		csvValue, _ = ExtractNumeric(csvValue)
	case "an":
		csvValue, _ = ExtractAlphaNumeric(csvValue)
	case "ans":
		if !hasSetter {
			csvValue, _ = ExtractAlphaNumericPrintableSymbols(csvValue)
		}
	case "b":
		if StringSliceContains(&trueList, strings.ToLower(csvValue)) {
			csvValue = "true"
		} else {
			csvValue = "false"
		}
	case "regex":
		csvValue, _ = ExtractByRegex(csvValue, cfg.tagRegEx)
	case "h":
		csvValue, _ = ExtractHex(csvValue)
	case "b64":
		csvValue, _ = ExtractAlphaNumericPrintableSymbols(csvValue)
	}

	// size checks (truncate only on max; min enforced in validation below)
	if cfg.tagType == "a" || cfg.tagType == "an" || cfg.tagType == "ans" || cfg.tagType == "n" || cfg.tagType == "regex" || cfg.tagType == "h" || cfg.tagType == "b64" {
		if cfg.sizeMax > 0 && len(csvValue) > int(cfg.sizeMax) {
			csvValue = Left(csvValue, int(cfg.sizeMax))
		}
		if cfg.tagModulo > 0 && len(csvValue)%int(cfg.tagModulo) != 0 {
			return "", fmt.Errorf("Expects Value In Blocks of %d Characters", cfg.tagModulo)
		}
	}

	return csvValue, nil
}

func csvApplySetter(s reflect.Value, cfg csvUnmarshalConfig, o reflect.Value, csvValue string) (string, bool, error) {
	if LenTrim(cfg.tagSetter) == 0 {
		return csvValue, false, nil
	}

	if o.Kind() != reflect.Ptr && o.Kind() != reflect.Interface && o.Kind() != reflect.Struct && o.Kind() != reflect.Slice {
		var ov []reflect.Value
		var notFound bool

		// try both value and pointer receivers when possible
		callTarget := o
		if callTarget.CanAddr() {
			callTarget = callTarget.Addr() // enable pointer-receiver methods
		}

		if cfg.setterBase {
			ov, notFound = ReflectCall(s.Addr(), cfg.tagSetter, csvValue)
		} else {
			ov, notFound = ReflectCall(callTarget, cfg.tagSetter, csvValue)
		}
		if notFound {
			return csvValue, false, nil
		}
		if len(ov) == 1 {
			if ov[0].Kind() == reflect.Ptr || ov[0].Kind() == reflect.Slice {
				o.Set(ov[0])
				return csvValue, true, nil
			}
			if val, _, err := ReflectValueToString(ov[0], cfg.boolTrue, cfg.boolFalse, false, false, cfg.timeFormat, false); err == nil {
				return val, false, nil
			}
			return csvValue, false, nil
		} else if len(ov) > 1 {
			// propagate setter error if present
			if err := DerefError(ov[len(ov)-1]); err != nil {
				return csvValue, false, err
			}
			// honor booltrue/boolfalse literals when stringifying setter return
			if val, _, err := ReflectValueToString(ov[0], cfg.boolTrue, cfg.boolFalse, false, false, cfg.timeFormat, false); err == nil { // CHANGED
				return val, false, nil
			}
			return csvValue, false, nil
		}
		return csvValue, false, nil
	}

	// pointer/interface/struct/slice: ensure allocation then set from setter
	if o.Kind() != reflect.Slice {
		if baseType, _, isNilPtr := DerefPointersZero(o); isNilPtr {
			o.Set(reflect.New(baseType.Type()))
		} else if o.Kind() == reflect.Interface && o.Interface() == nil {
			if customType := ReflectTypeRegistryGet(o.Type().String()); customType != nil {
				o.Set(reflect.New(customType))
			} else {
				return csvValue, false, fmt.Errorf("%s Struct Field %s is Interface Without Actual Object Assignment", s.Type(), o.Type())
			}
		}
	}

	var ov []reflect.Value
	var notFound bool

	// try pointer receiver when the field is addressable
	callTarget := o
	if callTarget.CanAddr() {
		callTarget = callTarget.Addr()
	}

	if cfg.setterBase {
		ov, notFound = ReflectCall(s.Addr(), cfg.tagSetter, csvValue)
	} else {
		ov, notFound = ReflectCall(callTarget, cfg.tagSetter, csvValue)
	}
	if !notFound {
		if len(ov) == 1 && (ov[0].Kind() == reflect.Ptr || ov[0].Kind() == reflect.Slice) {
			o.Set(ov[0])
			return csvValue, true, nil
		} else if len(ov) > 1 {
			// propagate setter error if present
			if err := DerefError(ov[len(ov)-1]); err != nil {
				return csvValue, false, err
			}
			if (ov[0].Kind() == reflect.Ptr || ov[0].Kind() == reflect.Slice) && !ov[0].IsNil() {
				o.Set(ov[0])
				return csvValue, true, nil
			}
			// honor booltrue/boolfalse literals when stringifying setter return
			if val, _, err := ReflectValueToString(ov[0], cfg.boolTrue, cfg.boolFalse, false, false, cfg.timeFormat, false); err == nil {
				return val, false, nil
			}
		}
	}
	return csvValue, false, nil
}

func csvValidateValue(csvValue string, cfg csvUnmarshalConfig, fieldName string) error {
	if cfg.tagType == "a" || cfg.tagType == "an" || cfg.tagType == "ans" || cfg.tagType == "n" || cfg.tagType == "regex" || cfg.tagType == "h" || cfg.tagType == "b64" {
		if cfg.sizeMin > 0 && len(csvValue) > 0 && len(csvValue) < int(cfg.sizeMin) {
			return fmt.Errorf("%s Min Length is %d", fieldName, cfg.sizeMin)
		}
	}
	if cfg.tagType == "n" {
		n, ok := ParseInt32(csvValue)
		if len(csvValue) > 0 && !ok { // enforce numeric validation error when parsing fails
			return fmt.Errorf("%s expects numeric value", fieldName)
		}
		if ok {
			if cfg.tagRangeMin > 0 && int32(n) < cfg.tagRangeMin && !(n == 0 && cfg.tagReq != "true") {
				return fmt.Errorf("%s Range Minimum is %d", fieldName, cfg.tagRangeMin)
			}
			if cfg.tagRangeMax > 0 && int32(n) > cfg.tagRangeMax {
				return fmt.Errorf("%s Range Maximum is %d", fieldName, cfg.tagRangeMax)
			}
		}
	}
	if cfg.tagReq == "true" && len(csvValue) == 0 {
		return fmt.Errorf("%s is a Required Field", fieldName)
	}
	return nil
}

func csvValidateCustomUnmarshal(csvValue string, cfg csvUnmarshalConfig, s reflect.Value, fieldName string) error {
	valData := cfg.validate
	if len(valData) < 3 {
		return nil
	}
	valComp := Left(valData, 2)
	valData = Right(valData, len(valData)-2)

	switch valComp {
	case "==":
		valAr := strings.Split(valData, "||")
		if len(valAr) <= 1 {
			if strings.ToLower(csvValue) != strings.ToLower(valData) && (len(csvValue) > 0 || cfg.tagReq == "true") {
				return fmt.Errorf("%s Validation Failed: Expected To Match '%s', But Received '%s'", fieldName, valData, csvValue)
			}
		} else {
			found := false
			for _, va := range valAr {
				if strings.ToLower(csvValue) == strings.ToLower(va) {
					found = true
					break
				}
			}
			if !found && (len(csvValue) > 0 || cfg.tagReq == "true") {
				return fmt.Errorf("%s Validation Failed: Expected To Match '%s', But Received '%s'", fieldName, strings.ReplaceAll(valData, "||", " or "), csvValue)
			}
		}
	case "!=":
		valAr := strings.Split(valData, "&&")
		if len(valAr) <= 1 {
			if strings.ToLower(csvValue) == strings.ToLower(valData) && (len(csvValue) > 0 || cfg.tagReq == "true") {
				return fmt.Errorf("%s Validation Failed: Expected To Not Match '%s', But Received '%s'", fieldName, valData, csvValue)
			}
		} else {
			found := false
			for _, va := range valAr {
				if strings.ToLower(csvValue) == strings.ToLower(va) {
					found = true
					break
				}
			}
			if found && (len(csvValue) > 0 || cfg.tagReq == "true") {
				return fmt.Errorf("%s Validation Failed: Expected To Not Match '%s', But Received '%s'", fieldName, strings.ReplaceAll(valData, "&&", " and "), csvValue)
			}
		}
	case "<=":
		if valNum, valOk := ParseFloat64(valData); valOk {
			if srcNum, _ := ParseFloat64(csvValue); srcNum > valNum && (len(csvValue) > 0 || cfg.tagReq == "true") {
				return fmt.Errorf("%s Validation Failed: Expected To Be Less or Equal To '%s', But Received '%s'", fieldName, valData, csvValue)
			}
		}
	case "<<":
		if valNum, valOk := ParseFloat64(valData); valOk {
			if srcNum, _ := ParseFloat64(csvValue); srcNum >= valNum && (len(csvValue) > 0 || cfg.tagReq == "true") {
				return fmt.Errorf("%s Validation Failed: Expected To Be Less Than '%s', But Received '%s'", fieldName, valData, csvValue)
			}
		}
	case ">=":
		if valNum, valOk := ParseFloat64(valData); valOk {
			if srcNum, _ := ParseFloat64(csvValue); srcNum < valNum && (len(csvValue) > 0 || cfg.tagReq == "true") {
				return fmt.Errorf("%s Validation Failed: Expected To Be Greater or Equal To '%s', But Received '%s'", fieldName, valData, csvValue)
			}
		}
	case ">>":
		if valNum, valOk := ParseFloat64(valData); valOk {
			if srcNum, _ := ParseFloat64(csvValue); srcNum <= valNum && (len(csvValue) > 0 || cfg.tagReq == "true") {
				return fmt.Errorf("%s Validation Failed: Expected To Be Greater Than '%s', But Received '%s'", fieldName, valData, csvValue)
			}
		}
	case ":=":
		if len(valData) > 0 {
			if retV, nf := ReflectCall(s.Addr(), valData); !nf && len(retV) > 0 {
				// capture errors and boolean failures
				if err := DerefError(retV[len(retV)-1]); err != nil {
					return fmt.Errorf("%s Validation On %s() Failed: %s", fieldName, valData, err.Error())
				}
				if retV[0].Kind() == reflect.Bool && !retV[0].Bool() {
					return fmt.Errorf("%s Validation Failed: %s() Returned Result is False", fieldName, valData)
				}
			}
		}
	}
	return nil
}

// =====================================================================================================================
// Json Parser Helpers
// =====================================================================================================================

// NEW: json field config + helpers for validation parity with CSV
type jsonFieldConfig struct {
	tagType     string
	tagRegEx    string
	sizeMin     int32
	sizeMax     int32
	tagModulo   int32
	tagReq      string
	tagSetter   string
	setterBase  bool
	timeFormat  string
	tagRangeMin int32
	tagRangeMax int32
	outPrefix   string
	boolTrue    string
	boolFalse   string
	validate    string
}

func jsonParseFieldConfig(field reflect.StructField) (cfg jsonFieldConfig, ok bool) {
	cfg.tagType = Trim(strings.ToLower(field.Tag.Get("type")))
	switch cfg.tagType {
	case "a", "n", "an", "ans", "b", "b64", "regex", "h":
	default:
		cfg.tagType = ""
	}

	cfg.tagRegEx = Trim(field.Tag.Get("regex"))
	if cfg.tagType != "regex" {
		cfg.tagRegEx = ""
	} else if LenTrim(cfg.tagRegEx) == 0 {
		cfg.tagType = ""
		cfg.tagRegEx = ""
	}

	cfg.tagReq = Trim(strings.ToLower(field.Tag.Get("req")))
	if cfg.tagReq != "true" && cfg.tagReq != "false" {
		cfg.tagReq = ""
	}

	cfg.tagSetter = Trim(field.Tag.Get("setter"))
	// safer setter parsing
	if len(cfg.tagSetter) > 0 && strings.HasPrefix(strings.ToLower(cfg.tagSetter), "base.") {
		cfg.setterBase = true
		cfg.tagSetter = cfg.tagSetter[5:]
	}

	cfg.timeFormat = Trim(field.Tag.Get("timeformat"))
	cfg.outPrefix = Trim(field.Tag.Get("outprefix"))
	cfg.boolTrue = Trim(field.Tag.Get("booltrue"))
	cfg.boolFalse = Trim(field.Tag.Get("boolfalse"))
	cfg.validate = Trim(field.Tag.Get("validate"))

	// size
	tagSize := Trim(strings.ToLower(field.Tag.Get("size")))
	arModulo := strings.Split(tagSize, "+%")
	if len(arModulo) == 2 {
		tagSize = arModulo[0]
		if n, _ := ParseInt32(arModulo[1]); n < 0 {
			cfg.tagModulo = 0
		} else {
			cfg.tagModulo = int32(n)
		}
	}
	arSize := strings.Split(tagSize, "..")
	if len(arSize) == 2 {
		iMin, _ := ParseInt32(arSize[0])
		cfg.sizeMin = int32(iMin)
		iMax, _ := ParseInt32(arSize[1])
		cfg.sizeMax = int32(iMax)
	} else {
		iMin, _ := ParseInt32(tagSize)
		cfg.sizeMin = int32(iMin)
		cfg.sizeMax = int32(iMin)
	}

	// range
	tagRange := Trim(strings.ToLower(field.Tag.Get("range")))
	arRange := strings.Split(tagRange, "..")
	if len(arRange) == 2 {
		iMin, _ := ParseInt32(arRange[0])
		cfg.tagRangeMin = int32(iMin)
		iMax, _ := ParseInt32(arRange[1])
		cfg.tagRangeMax = int32(iMax)
	} else {
		iMin, _ := ParseInt32(tagRange)
		cfg.tagRangeMin = int32(iMin)
		cfg.tagRangeMax = int32(iMin)
	}

	return cfg, true
}

func jsonPreprocessValue(raw string, cfg jsonFieldConfig, hasSetter bool, trueList []string) (string, error) {
	val := raw
	switch cfg.tagType {
	case "a":
		val, _ = ExtractAlpha(val)
	case "n":
		val, _ = ExtractNumeric(val)
	case "an":
		val, _ = ExtractAlphaNumeric(val)
	case "ans":
		if !hasSetter {
			val, _ = ExtractAlphaNumericPrintableSymbols(val)
		}
	case "b":
		if StringSliceContains(&trueList, strings.ToLower(val)) {
			val = "true"
		} else {
			val = "false"
		}
	case "regex":
		val, _ = ExtractByRegex(val, cfg.tagRegEx)
	case "h":
		val, _ = ExtractHex(val)
	case "b64":
		val, _ = ExtractAlphaNumericPrintableSymbols(val)
	}

	if cfg.tagType == "a" || cfg.tagType == "an" || cfg.tagType == "ans" || cfg.tagType == "n" || cfg.tagType == "regex" || cfg.tagType == "h" || cfg.tagType == "b64" {
		if cfg.sizeMax > 0 && len(val) > int(cfg.sizeMax) {
			val = Left(val, int(cfg.sizeMax))
		}
		if cfg.tagModulo > 0 && len(val)%int(cfg.tagModulo) != 0 {
			return "", fmt.Errorf("Expects Value In Blocks of %d Characters", cfg.tagModulo)
		}
	}

	return val, nil
}

func jsonValidateValue(val string, cfg jsonFieldConfig, fieldName string) error {
	if cfg.tagType == "a" || cfg.tagType == "an" || cfg.tagType == "ans" || cfg.tagType == "n" || cfg.tagType == "regex" || cfg.tagType == "h" || cfg.tagType == "b64" {
		if cfg.sizeMin > 0 && len(val) > 0 && len(val) < int(cfg.sizeMin) {
			return fmt.Errorf("%s Min Length is %d", fieldName, cfg.sizeMin)
		}
	}
	if cfg.tagType == "n" {
		n, ok := ParseInt32(val)
		if len(val) > 0 && !ok {
			return fmt.Errorf("%s expects numeric value", fieldName)
		}
		if ok {
			if cfg.tagRangeMin > 0 && int32(n) < cfg.tagRangeMin && !(n == 0 && cfg.tagReq != "true") {
				return fmt.Errorf("%s Range Minimum is %d", fieldName, cfg.tagRangeMin)
			}
			if cfg.tagRangeMax > 0 && int32(n) > cfg.tagRangeMax {
				return fmt.Errorf("%s Range Maximum is %d", fieldName, cfg.tagRangeMax)
			}
		}
	}
	if cfg.tagReq == "true" && len(val) == 0 {
		return fmt.Errorf("%s is a Required Field", fieldName)
	}
	return nil
}

func jsonValidateCustom(val string, cfg jsonFieldConfig, s reflect.Value, fieldName string) error {
	if len(cfg.validate) < 3 {
		return nil
	}
	valComp := Left(cfg.validate, 2)
	valData := Right(cfg.validate, len(cfg.validate)-2)

	switch valComp {
	case "==":
		valAr := strings.Split(valData, "||")
		if len(valAr) <= 1 {
			if strings.ToLower(val) != strings.ToLower(valData) && (len(val) > 0 || cfg.tagReq == "true") {
				return fmt.Errorf("%s Validation Failed: Expected To Match '%s', But Received '%s'", fieldName, valData, val)
			}
		} else {
			found := false
			for _, va := range valAr {
				if strings.ToLower(val) == strings.ToLower(va) {
					found = true
					break
				}
			}
			if !found && (len(val) > 0 || cfg.tagReq == "true") {
				return fmt.Errorf("%s Validation Failed: Expected To Match '%s', But Received '%s'", fieldName, strings.ReplaceAll(valData, "||", " or "), val)
			}
		}
	case "!=":
		valAr := strings.Split(valData, "&&")
		if len(valAr) <= 1 {
			if strings.ToLower(val) == strings.ToLower(valData) && (len(val) > 0 || cfg.tagReq == "true") {
				return fmt.Errorf("%s Validation Failed: Expected To Not Match '%s', But Received '%s'", fieldName, valData, val)
			}
		} else {
			found := false
			for _, va := range valAr {
				if strings.ToLower(val) == strings.ToLower(va) {
					found = true
					break
				}
			}
			if found && (len(val) > 0 || cfg.tagReq == "true") {
				return fmt.Errorf("%s Validation Failed: Expected To Not Match '%s', But Received '%s'", fieldName, strings.ReplaceAll(valData, "&&", " and "), val)
			}
		}
	case "<=":
		if valNum, valOk := ParseFloat64(valData); valOk {
			if srcNum, _ := ParseFloat64(val); srcNum > valNum && (len(val) > 0 || cfg.tagReq == "true") {
				return fmt.Errorf("%s Validation Failed: Expected To Be Less or Equal To '%s', But Received '%s'", fieldName, valData, val)
			}
		}
	case "<<":
		if valNum, valOk := ParseFloat64(valData); valOk {
			if srcNum, _ := ParseFloat64(val); srcNum >= valNum && (len(val) > 0 || cfg.tagReq == "true") {
				return fmt.Errorf("%s Validation Failed: Expected To Be Less Than '%s', But Received '%s'", fieldName, valData, val)
			}
		}
	case ">=":
		if valNum, valOk := ParseFloat64(valData); valOk {
			if srcNum, _ := ParseFloat64(val); srcNum < valNum && (len(val) > 0 || cfg.tagReq == "true") {
				return fmt.Errorf("%s Validation Failed: Expected To Be Greater or Equal To '%s', But Received '%s'", fieldName, valData, val)
			}
		}
	case ">>":
		if valNum, valOk := ParseFloat64(valData); valOk {
			if srcNum, _ := ParseFloat64(val); srcNum <= valNum && (len(val) > 0 || cfg.tagReq == "true") {
				return fmt.Errorf("%s Validation Failed: Expected To Be Greater Than '%s', But Received '%s'", fieldName, valData, val)
			}
		}
	case ":=":
		if len(valData) > 0 {
			if retV, nf := ReflectCall(s.Addr(), valData); !nf && len(retV) > 0 {
				// capture errors and boolean failures
				if err := DerefError(retV[len(retV)-1]); err != nil {
					return fmt.Errorf("%s Validation On %s() Failed: %s", fieldName, valData, err.Error())
				}
				if retV[0].Kind() == reflect.Bool && !retV[0].Bool() {
					return fmt.Errorf("%s Validation Failed: %s() Returned Result is False", fieldName, valData)
				}
			}
		}
	}
	return nil
}

func jsonApplySetter(s reflect.Value, cfg jsonFieldConfig, o reflect.Value, jsonValue string) (string, bool, error) {
	return csvApplySetter(s, csvUnmarshalConfig{ // reuse proven setter logic
		tagSetter:  cfg.tagSetter,
		setterBase: cfg.setterBase,
		timeFormat: cfg.timeFormat,
		boolTrue:   cfg.boolTrue,  // preserve bool literal mapping in JSON setters
		boolFalse:  cfg.boolFalse, // preserve bool literal mapping in JSON setters
	}, o, jsonValue)
}

// =====================================================================================================================
// implementation functions
// =====================================================================================================================

// Fill copies the src struct with same tag name to dst struct tag pointer,
// src and dst both must be struct，and dst must be pointer
func Fill(src interface{}, dst interface{}) error {
	if src == nil {
		return errors.New("src cannot be nil")
	}
	if dst == nil {
		return errors.New("dst cannot be nil")
	}

	srcValue := reflect.ValueOf(src)
	srcType := srcValue.Type()

	// allow pointer-to-struct for src while still rejecting non-structs
	if srcType.Kind() == reflect.Ptr {
		if srcValue.IsNil() {
			return errors.New("src pointer cannot be nil")
		}
		srcValue = srcValue.Elem()
		srcType = srcType.Elem()
	}

	dstValue := reflect.ValueOf(dst)

	if srcType.Kind() != reflect.Struct {
		return errors.New("src must be struct")
	}
	if dstValue.Kind() != reflect.Ptr {
		return errors.New("dst must be pointer")
	}
	if dstValue.IsNil() {
		return errors.New("dst pointer cannot be nil")
	}
	if dstValue.Elem().Kind() != reflect.Struct {
		return errors.New("dst must point to struct")
	}

	for i := 0; i < srcType.NumField(); i++ {
		dstField := dstValue.Elem().FieldByName(srcType.Field(i).Name)
		if dstField.CanSet() {
			srcField := srcValue.Field(i)

			// avoid panic when types differ; allow assignable or convertible only
			switch {
			case srcField.Type().AssignableTo(dstField.Type()):
				dstField.Set(srcField)
			case srcField.Type().ConvertibleTo(dstField.Type()):
				dstField.Set(srcField.Convert(dstField.Type()))
			default:
				return fmt.Errorf("field %s types are not assignable or convertible (src=%s, dst=%s)",
					srcType.Field(i).Name, srcField.Type(), dstField.Type())
			}
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
//  1. `getter:"Key"`			// if field type is custom struct or enum,
//     specify the custom method getter (no parameters allowed) that returns the expected value in first ordinal result position
//     NOTE: if the method to invoke resides at struct level, precede the method name with 'base.', for example, 'base.XYZ' where XYZ is method name to invoke
//     NOTE: if the method is to receive a parameter value, always in string data type, add '(x)' after the method name, such as 'XYZ(x)' or 'base.XYZ(x)'
//  2. `booltrue:"1"` 			// if field is defined, contains bool literal for true condition, such as 1 or true, that overrides default system bool literal value,
//     if bool literal value is determined by existence of outprefix and itself is blank, place a space in both booltrue and boolfalse (setting blank will negate literal override)
//  3. `boolfalse:"0"`			// if field is defined, contains bool literal for false condition, such as 0 or false, that overrides default system bool literal value
//     if bool literal value is determined by existence of outprefix and itself is blank, place a space in both booltrue and boolfalse (setting blank will negate literal override)
//  4. `uniqueid:"xyz"`			// if two or more struct field is set with the same uniqueid, then only the first encountered field with the same uniqueid will be used in marshal
//  5. `skipblank:"false"`		// if true, then any fields that is blank string will be excluded from marshal (this only affects fields that are string)
//  6. `skipzero:"false"`		// if true, then any fields that are 0, 0.00, time.Zero(), false, nil will be excluded from marshal (this only affects fields that are number, bool, time, pointer)
//  7. `timeformat:"20060102"`	// for time.Time field, optional date time format, specified as:
//     2006, 06 = year,
//     01, 1, Jan, January = month,
//     02, 2, _2 = day (_2 = width two, right justified)
//     03, 3, 15 = hour (15 = 24 hour format)
//     04, 4 = minute
//     05, 5 = second
//     PM pm = AM PM
//  8. `outprefix:""`			// for marshal method, if field value is to precede with an output prefix, such as XYZ= (affects marshal queryParams / csv methods only)
//  9. `zeroblank:"false"`		// set true to set blank to data when value is 0, 0.00, or time.IsZero
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

				var boolTrue, boolFalse, timeFormat, outPrefix string
				var skipBlank, skipZero, zeroblank bool

				if vs := GetStructTagsValueSlice(field, "booltrue", "boolfalse", "skipblank", "skipzero", "timeformat", "outprefix", "zeroblank"); len(vs) == 7 {
					boolTrue = vs[0]
					boolFalse = vs[1]
					skipBlank, _ = ParseBool(vs[2])
					skipZero, _ = ParseBool(vs[3])
					timeFormat = vs[4]
					outPrefix = vs[5]
					zeroblank, _ = ParseBool(vs[6])
				}

				oldVal := o

				if tagGetter := Trim(field.Tag.Get("getter")); len(tagGetter) > 0 {
					isBase := false
					useParam := false
					paramVal := ""
					var paramSlice interface{}
					lg := strings.ToLower(tagGetter)

					if strings.HasPrefix(lg, "base.") {
						isBase = true
						tagGetter = tagGetter[5:]
						lg = strings.ToLower(tagGetter)
					}

					if strings.HasSuffix(lg, "(x)") && len(tagGetter) >= 3 {
						useParam = true

						if o.Kind() != reflect.Slice {
							paramVal, _, _ = ReflectValueToString(o, boolTrue, boolFalse, skipBlank, skipZero, timeFormat, zeroblank)
						} else {
							if o.Len() > 0 {
								paramSlice = o.Slice(0, o.Len()).Interface()
							}
						}

						tagGetter = tagGetter[:len(tagGetter)-3]
					}

					var ov []reflect.Value
					var notFound bool

					if isBase {
						if useParam {
							if paramSlice == nil {
								ov, notFound = ReflectCall(s.Addr(), tagGetter, paramVal)
							} else {
								ov, notFound = ReflectCall(s.Addr(), tagGetter, paramSlice)
							}
						} else {
							ov, notFound = ReflectCall(s.Addr(), tagGetter)
						}
					} else {
						if useParam {
							if paramSlice == nil {
								ov, notFound = ReflectCall(o, tagGetter, paramVal)
							} else {
								ov, notFound = ReflectCall(o, tagGetter, paramSlice)
							}
						} else {
							ov, notFound = ReflectCall(o, tagGetter)
						}
					}

					if !notFound {
						if len(ov) > 0 {
							o = ov[0]
						}
					}
				}

				if buf, skip, err := ReflectValueToString(o, boolTrue, boolFalse, skipBlank, skipZero, timeFormat, zeroblank); err != nil || skip {
					if tagUniqueId := Trim(field.Tag.Get("uniqueid")); len(tagUniqueId) > 0 {
						if _, ok := uniqueMap[strings.ToLower(tagUniqueId)]; ok {
							delete(uniqueMap, strings.ToLower(tagUniqueId))
						}
					}

					continue
				} else {
					defVal := field.Tag.Get("def")

					if oldVal.Kind() == reflect.Int && oldVal.Int() == 0 && strings.ToLower(buf) == "unknown" {
						// unknown enum value will be serialized as blank
						buf = ""

						if len(defVal) > 0 {
							buf = defVal
						} else {
							if tagUniqueId := Trim(field.Tag.Get("uniqueid")); len(tagUniqueId) > 0 {
								if _, ok := uniqueMap[strings.ToLower(tagUniqueId)]; ok {
									// remove uniqueid if skip
									delete(uniqueMap, strings.ToLower(tagUniqueId))
									continue
								}
							}
						}
					}

					if boolFalse == " " && len(outPrefix) > 0 && buf == "false" {
						buf = ""
					} else {
						if len(buf) == 0 && len(defVal) > 0 {
							buf = defVal
						}

						if skipBlank && LenTrim(buf) == 0 {
							buf = ""
						} else if skipZero && buf == "0" {
							buf = ""
						} else {
							buf = outPrefix + buf
						}
					}

					if LenTrim(output) > 0 {
						output += "&"
					}

					output += fmt.Sprintf("%s=%s", tag, url.PathEscape(buf))
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
//  1. `getter:"Key"`			// if field type is custom struct or enum,
//     specify the custom method getter (no parameters allowed) that returns the expected value in first ordinal result position
//     NOTE: if the method to invoke resides at struct level, precede the method name with 'base.', for example, 'base.XYZ' where XYZ is method name to invoke
//     NOTE: if the method is to receive a parameter value, always in string data type, add '(x)' after the method name, such as 'XYZ(x)' or 'base.XYZ(x)'
//  2. `booltrue:"1"` 			// if field is defined, contains bool literal for true condition, such as 1 or true, that overrides default system bool literal value
//     if bool literal value is determined by existence of outprefix and itself is blank, place a space in both booltrue and boolfalse (setting blank will negate literal override)
//  3. `boolfalse:"0"`			// if field is defined, contains bool literal for false condition, such as 0 or false, that overrides default system bool literal value
//     if bool literal value is determined by existence of outprefix and itself is blank, place a space in both booltrue and boolfalse (setting blank will negate literal override)
//  4. `uniqueid:"xyz"`			// if two or more struct field is set with the same uniqueid, then only the first encountered field with the same uniqueid will be used in marshal
//  5. `skipblank:"false"`		// if true, then any fields that is blank string will be excluded from marshal (this only affects fields that are string)
//  6. `skipzero:"false"`		// if true, then any fields that are 0, 0.00, time.Zero(), false, nil will be excluded from marshal (this only affects fields that are number, bool, time, pointer)
//  7. `timeformat:"20060102"`	// for time.Time field, optional date time format, specified as:
//     2006, 06 = year,
//     01, 1, Jan, January = month,
//     02, 2, _2 = day (_2 = width two, right justified)
//     03, 3, 15 = hour (15 = 24 hour format)
//     04, 4 = minute
//     05, 5 = second
//     PM pm = AM PM
//  8. `zeroblank:"false"`		// set true to set blank to data when value is 0, 0.00, or time.IsZero
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

	uniqueMap := make(map[string]string)
	jsonMap := make(map[string]string) // build a map and let json.Marshal handle escaping

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

				var boolTrue, boolFalse, timeFormat string
				var skipBlank, skipZero, zeroBlank bool

				if vs := GetStructTagsValueSlice(field, "booltrue", "boolfalse", "skipblank", "skipzero", "timeformat", "zeroblank"); len(vs) == 6 {
					boolTrue = vs[0]
					boolFalse = vs[1]
					skipBlank, _ = ParseBool(vs[2])
					skipZero, _ = ParseBool(vs[3])
					timeFormat = vs[4]
					zeroBlank, _ = ParseBool(vs[5])
				}

				oldVal := o

				if tagGetter := Trim(field.Tag.Get("getter")); len(tagGetter) > 0 {
					isBase := false
					useParam := false
					paramVal := ""
					var paramSlice interface{}
					lg := strings.ToLower(tagGetter)

					if strings.HasPrefix(lg, "base.") {
						isBase = true
						tagGetter = tagGetter[5:]
						lg = strings.ToLower(tagGetter)
					}

					if strings.HasSuffix(lg, "(x)") && len(tagGetter) >= 3 {
						useParam = true

						if o.Kind() != reflect.Slice {
							paramVal, _, _ = ReflectValueToString(o, boolTrue, boolFalse, skipBlank, skipZero, timeFormat, zeroBlank)
						} else {
							if o.Len() > 0 {
								paramSlice = o.Slice(0, o.Len()).Interface()
							}
						}

						tagGetter = tagGetter[:len(tagGetter)-3]
					}

					var ov []reflect.Value
					var notFound bool

					if isBase {
						if useParam {
							if paramSlice == nil {
								ov, notFound = ReflectCall(s.Addr(), tagGetter, paramVal)
							} else {
								ov, notFound = ReflectCall(s.Addr(), tagGetter, paramSlice)
							}
						} else {
							ov, notFound = ReflectCall(s.Addr(), tagGetter)
						}
					} else {
						if useParam {
							if paramSlice == nil {
								ov, notFound = ReflectCall(o, tagGetter, paramVal)
							} else {
								ov, notFound = ReflectCall(o, tagGetter, paramSlice)
							}
						} else {
							ov, notFound = ReflectCall(o, tagGetter)
						}
					}

					if !notFound {
						if len(ov) > 0 {
							o = ov[0]
						}
					}
				}

				buf, skip, err := ReflectValueToString(o, boolTrue, boolFalse, skipBlank, skipZero, timeFormat, zeroBlank)

				if err != nil || skip {
					if tagUniqueId := Trim(field.Tag.Get("uniqueid")); len(tagUniqueId) > 0 {
						if _, ok := uniqueMap[strings.ToLower(tagUniqueId)]; ok {
							delete(uniqueMap, strings.ToLower(tagUniqueId))
						}
					}

					continue
				}

				defVal := field.Tag.Get("def")

				if oldVal.Kind() == reflect.Int && oldVal.Int() == 0 && strings.ToLower(buf) == "unknown" {
					// unknown enum value will be serialized as blank
					buf = ""

					if len(defVal) > 0 {
						buf = defVal
					} else {
						if tagUniqueId := Trim(field.Tag.Get("uniqueid")); len(tagUniqueId) > 0 {
							if _, ok := uniqueMap[strings.ToLower(tagUniqueId)]; ok {
								// remove uniqueid if skip
								delete(uniqueMap, strings.ToLower(tagUniqueId))
								continue
							}
						}
					}
				}

				outPrefix := field.Tag.Get("outprefix")

				if boolTrue == " " && len(buf) == 0 && len(outPrefix) > 0 {
					buf = outPrefix + defVal
				} else if boolFalse == " " && buf == "false" && len(outPrefix) > 0 {
					buf = ""
				} else if len(defVal) > 0 && len(buf) == 0 {
					buf = outPrefix + defVal
				}

				jsonMap[tag] = buf // defer escaping/encoding to json.Marshal
			}
		}
	}

	if len(jsonMap) == 0 {
		return "", fmt.Errorf("MarshalStructToJson Yielded Blank Output")
	}

	encoded, err := json.Marshal(jsonMap) // safe JSON encoding
	if err != nil {
		return "", err
	}

	return string(encoded), nil
}

// UnmarshalJsonToStruct will parse jsonPayload string,
// and set parsed json element value into struct fields based on struct tag named by tagName,
// any tagName value with - will be ignored, any excludeTagName defined with value of - will also cause parser to ignore the field
//
// note: this method expects simple json in key value pairs only, not json containing slices or more complex json structs within existing json field
//
// Predefined Struct Tags Usable:
//  1. `setter:"ParseByKey`		// if field type is custom struct or enum,
//     specify the custom method (only 1 lookup parameter value allowed) setter that sets value(s) into the field
//     NOTE: if the method to invoke resides at struct level, precede the method name with 'base.', for example, 'base.XYZ' where XYZ is method name to invoke
//     NOTE: setter method always intake a string parameter
//  2. `def:""`					// default value to set into struct field in case unmarshal doesn't set the struct field value
//  3. `timeformat:"20060102"`	// for time.Time field, optional date time format, specified as:
//     2006, 06 = year,
//     01, 1, Jan, January = month,
//     02, 2, _2 = day (_2 = width two, right justified)
//     03, 3, 15 = hour (15 = 24 hour format)
//     04, 4 = minute
//     05, 5 = second
//     PM pm = AM PM
//  4. `booltrue:"1"` 			// if field is defined, contains bool literal for true condition, such as 1 or true, that overrides default system bool literal value,
//     if bool literal value is determined by existence of outprefix and itself is blank, place a space in both booltrue and boolfalse (setting blank will negate literal override)
//  5. `boolfalse:"0"`			// if field is defined, contains bool literal for false condition, such as 0 or false, that overrides default system bool literal value
//     if bool literal value is determined by existence of outprefix and itself is blank, place a space in both booltrue and boolfalse (setting blank will negate literal override)
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

	// reset state and defaults up front
	StructClearFields(inputStructPtr)
	if _, err := SetStructFieldDefaultValues(inputStructPtr); err != nil {
		return err
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

	trueList := []string{"true", "yes", "on", "1", "enabled"}

	for i := 0; i < s.NumField(); i++ {
		field := s.Type().Field(i)

		if o := s.FieldByName(field.Name); o.IsValid() && o.CanSet() {
			cfg, _ := jsonParseFieldConfig(field)

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

			jRaw, ok := jsonMap[jName]
			if !ok {
				// enforce required when missing and no default
				if cfg.tagReq == "true" && LenTrim(field.Tag.Get("def")) == 0 {
					StructClearFields(inputStructPtr)
					return fmt.Errorf("%s is a Required Field", field.Name)
				}
				continue
			}

			jValue := JsonFromEscaped(string(jRaw))
			if strings.EqualFold(Trim(jValue), "null") { // treat JSON null as empty to avoid spurious validation failures
				jValue = ""
			}

			// bool literal overrides and outprefix handling
			if cfg.boolTrue == " " && len(cfg.outPrefix) > 0 && jValue == cfg.outPrefix {
				jValue = "true"
			} else {
				if LenTrim(cfg.boolTrue) > 0 && jValue == cfg.boolTrue {
					jValue = "true"
				} else if LenTrim(cfg.boolFalse) > 0 && jValue == cfg.boolFalse {
					jValue = "false"
				}
			}

			hasSetter := LenTrim(cfg.tagSetter) > 0

			valPrep, err := jsonPreprocessValue(jValue, cfg, hasSetter, trueList)
			if err != nil {
				StructClearFields(inputStructPtr)
				return fmt.Errorf("%s %s", field.Name, err.Error())
			}
			jValue = valPrep

			if newVal, setDone, err := jsonApplySetter(s, cfg, o, jValue); err != nil {
				StructClearFields(inputStructPtr)
				return err
			} else if setDone { // setter handled assignment; still validate
				if err := jsonValidateValue(newVal, cfg, field.Name); err != nil {
					StructClearFields(inputStructPtr)
					return err
				}
				if err := jsonValidateCustom(newVal, cfg, s, field.Name); err != nil {
					StructClearFields(inputStructPtr)
					return err
				}
				continue
			} else {
				jValue = newVal
			}

			if err := jsonValidateValue(jValue, cfg, field.Name); err != nil {
				StructClearFields(inputStructPtr)
				return err
			}
			if err := jsonValidateCustom(jValue, cfg, s, field.Name); err != nil {
				StructClearFields(inputStructPtr)
				return err
			}

			if err := ReflectStringToField(o, jValue, cfg.timeFormat); err != nil {
				StructClearFields(inputStructPtr) // avoid partial state on assignment failure
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
			case reflect.Slice:
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
				default:
					o.Set(reflect.Zero(o.Type()))
				}
			}
		}
	}
}

// StructNonDefaultRequiredFieldsCount returns count of struct fields that are tagged as required but not having any default values pre-set
func StructNonDefaultRequiredFieldsCount(inputStructPtr interface{}) int {
	if inputStructPtr == nil {
		return 0
	}

	s := reflect.ValueOf(inputStructPtr)

	if s.Kind() != reflect.Ptr {
		return 0
	} else {
		s = s.Elem()
	}

	if s.Kind() != reflect.Struct {
		return 0
	}

	count := 0

	for i := 0; i < s.NumField(); i++ {
		field := s.Type().Field(i)

		if o := s.FieldByName(field.Name); o.IsValid() && o.CanSet() {
			tagDef := field.Tag.Get("def")
			tagReq := field.Tag.Get("req")

			if len(tagDef) == 0 && strings.ToLower(tagReq) == "true" {
				// required and no default value
				count++
			}
		}
	}

	return count
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

			if len(tagDef) == 0 {
				continue
			}

			// safer setter parsing (no Left/Right on short strings)
			tagSetter := Trim(field.Tag.Get("setter"))
			setterBase := false
			if len(tagSetter) > 0 && strings.HasPrefix(strings.ToLower(tagSetter), "base.") {
				setterBase = true
				tagSetter = tagSetter[5:]
			}

			applySetter := func() (handled bool, updatedDef string, err error) { // error surfaced
				updatedDef = tagDef
				if LenTrim(tagSetter) == 0 {
					return false, updatedDef, nil
				}

				// ensure allocation for pointer/interface targets
				if o.Kind() != reflect.Slice {
					if baseType, _, isNilPtr := DerefPointersZero(o); isNilPtr {
						o.Set(reflect.New(baseType.Type()))
					} else if o.Kind() == reflect.Interface && o.Interface() == nil {
						if customType := ReflectTypeRegistryGet(o.Type().String()); customType != nil {
							o.Set(reflect.New(customType))
						} else {
							return true, updatedDef, fmt.Errorf("%s Struct Field %s is Interface Without Actual Object Assignment", s.Type(), o.Type())
						}
					}
				}

				var ov []reflect.Value
				var notFound bool

				if setterBase {
					ov, notFound = ReflectCall(s.Addr(), tagSetter, updatedDef)
				} else {
					// allow pointer-receiver setters for value fields by using an addressable target when possible
					callTarget := o
					if o.Kind() != reflect.Ptr && callTarget.CanAddr() {
						callTarget = callTarget.Addr()
					}
					ov, notFound = ReflectCall(callTarget, tagSetter, updatedDef)
				}
				if notFound {
					return false, updatedDef, nil
				}

				if len(ov) == 1 {
					if ov[0].Kind() == reflect.Ptr || ov[0].Kind() == reflect.Slice {
						o.Set(ov[0])
						return true, updatedDef, nil
					}
					if val, _, err := ReflectValueToString(ov[0], "", "", false, false, "", false); err == nil {
						return false, val, nil
					}
					return true, updatedDef, nil
				} else if len(ov) > 1 {
					if err := DerefError(ov[len(ov)-1]); err != nil { // propagate setter error
						return true, updatedDef, err
					}
					if ov[0].Kind() == reflect.Ptr || ov[0].Kind() == reflect.Slice {
						o.Set(ov[0])
						return true, updatedDef, nil
					}
					if val, _, err := ReflectValueToString(ov[0], "", "", false, false, "", false); err == nil {
						return false, val, nil
					}
					return true, updatedDef, nil
				}

				return false, updatedDef, nil
			}

			if handled, newDef, err := applySetter(); err != nil { // propagate error
				return false
			} else if handled {
				continue
			} else {
				tagDef = newDef
			}

			switch o.Kind() {
			case reflect.String:
				if LenTrim(o.String()) > 0 {
					if o.String() != tagDef {
						return true
					}
				}
			case reflect.Bool:
				if o.Bool() {
					return true
				}
			case reflect.Int8, reflect.Int16, reflect.Int, reflect.Int32, reflect.Int64:
				if o.Int() != 0 {
					if Int64ToString(o.Int()) != tagDef {
						return true
					}
				}
			case reflect.Float32, reflect.Float64:
				if o.Float() != 0 {
					if Float64ToString(o.Float()) != tagDef {
						return true
					}
				}
			case reflect.Uint8, reflect.Uint16, reflect.Uint, reflect.Uint32, reflect.Uint64:
				if o.Uint() > 0 {
					if UInt64ToString(o.Uint()) != tagDef {
						return true
					}
				}
			case reflect.Ptr:
				if !o.IsNil() {
					return true
				}
			case reflect.Slice:
				if o.Len() > 0 {
					return true
				}
			default:
				switch f := o.Interface().(type) {
				case sql.NullString:
					if f.Valid {
						if len(tagDef) == 0 || f.String != tagDef {
							return true
						}
					}
				case sql.NullBool:
					if f.Valid {
						if len(tagDef) == 0 {
							return true
						}
						if defVal, _ := ParseBool(tagDef); f.Bool != defVal { // compare without overwriting actual value
							return true
						}
					}
				case sql.NullFloat64:
					if f.Valid {
						if len(tagDef) == 0 || Float64ToString(f.Float64) != tagDef {
							return true
						}
					}
				case sql.NullInt32:
					if f.Valid {
						if len(tagDef) == 0 || Itoa(int(f.Int32)) != tagDef {
							return true
						}
					}
				case sql.NullInt64:
					if f.Valid {
						if len(tagDef) == 0 || Int64ToString(f.Int64) != tagDef {
							return true
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
				default:
					if o.Kind() == reflect.Interface && o.Interface() != nil {
						return true
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
// time format is used if field is datetime, for overriding default format of ISO style
func SetStructFieldDefaultValues(inputStructPtr interface{}) (bool, error) {
	if inputStructPtr == nil {
		return false, nil
	}

	s := reflect.ValueOf(inputStructPtr)

	if s.Kind() != reflect.Ptr {
		return false, nil
	} else {
		s = s.Elem()
	}

	if s.Kind() != reflect.Struct {
		return false, nil
	}

	for i := 0; i < s.NumField(); i++ {
		field := s.Type().Field(i)

		if o := s.FieldByName(field.Name); o.IsValid() && o.CanSet() {
			tagDef := field.Tag.Get("def")

			if len(tagDef) == 0 {
				continue
			}

			// normalize setter info (support base. prefix) and allocate pointer/interface targets before calling setters.
			tagSetter := Trim(field.Tag.Get("setter"))
			setterBase := false
			if LenTrim(tagSetter) > 0 && strings.ToLower(Left(tagSetter, 5)) == "base." {
				setterBase = true
				tagSetter = Right(tagSetter, len(tagSetter)-5)
			}

			applySetter := func() (handled bool, updatedDef string, err error) { // error surfaced
				updatedDef = tagDef
				if LenTrim(tagSetter) == 0 {
					return false, updatedDef, nil
				}

				// ensure allocation for pointer/interface targets
				if o.Kind() != reflect.Slice {
					if baseType, _, isNilPtr := DerefPointersZero(o); isNilPtr {
						o.Set(reflect.New(baseType.Type()))
					} else if o.Kind() == reflect.Interface && o.Interface() == nil {
						if customType := ReflectTypeRegistryGet(o.Type().String()); customType != nil {
							o.Set(reflect.New(customType))
						} else {
							return true, updatedDef, fmt.Errorf("%s Struct Field %s is Interface Without Actual Object Assignment", s.Type(), o.Type())
						}
					}
				}

				var ov []reflect.Value
				var notFound bool

				if setterBase {
					ov, notFound = ReflectCall(s.Addr(), tagSetter, updatedDef)
				} else {
					// allow pointer-receiver setters for value fields by using an addressable target when possible
					callTarget := o
					if o.Kind() != reflect.Ptr && callTarget.CanAddr() {
						callTarget = callTarget.Addr()
					}
					ov, notFound = ReflectCall(callTarget, tagSetter, updatedDef)
				}
				if notFound {
					return false, updatedDef, nil
				}

				if len(ov) == 1 {
					if ov[0].Kind() == reflect.Ptr || ov[0].Kind() == reflect.Slice {
						o.Set(ov[0])
						return true, updatedDef, nil
					}
					if val, _, err := ReflectValueToString(ov[0], "", "", false, false, "", false); err == nil {
						return false, val, nil
					}
					return true, updatedDef, nil
				} else if len(ov) > 1 {
					if err := DerefError(ov[len(ov)-1]); err != nil { // propagate setter error
						return true, updatedDef, err
					}
					if ov[0].Kind() == reflect.Ptr || ov[0].Kind() == reflect.Slice {
						o.Set(ov[0])
						return true, updatedDef, nil
					}
					if val, _, err := ReflectValueToString(ov[0], "", "", false, false, "", false); err == nil {
						return false, val, nil
					}
					return true, updatedDef, nil
				}

				return false, updatedDef, nil
			}

			if handled, newDef, err := applySetter(); err != nil { // propagate error
				return false, err
			} else if handled {
				continue
			} else {
				tagDef = newDef
			}

			switch o.Kind() {
			case reflect.String:
				if LenTrim(o.String()) == 0 {
					o.SetString(tagDef)
				}
			case reflect.Int8, reflect.Int16, reflect.Int, reflect.Int32, reflect.Int64:
				if o.Int() == 0 {
					if i64, ok := ParseInt64(tagDef); ok && !o.OverflowInt(i64) { // allow zero defaults
						o.SetInt(i64)
					}
				}
			case reflect.Float32, reflect.Float64:
				if o.Float() == 0 {
					if f64, ok := ParseFloat64(tagDef); ok && !o.OverflowFloat(f64) { // allow zero defaults
						o.SetFloat(f64)
					}
				}
			case reflect.Uint8, reflect.Uint16, reflect.Uint, reflect.Uint32, reflect.Uint64:
				if o.Uint() == 0 {
					if u64 := StrToUint64(tagDef); u64 >= 0 && !o.OverflowUint(u64) { // allow zero defaults
						o.SetUint(u64)
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
						b, _ := ParseBool(tagDef)
						o.Set(reflect.ValueOf(sql.NullBool{Bool: b, Valid: true}))
					}
				case sql.NullFloat64:
					if !f.Valid {
						if f64, ok := ParseFloat64(tagDef); ok {
							o.Set(reflect.ValueOf(sql.NullFloat64{Float64: f64, Valid: true})) // allow zero defaults
						}
					}
				case sql.NullInt32:
					if !f.Valid {
						if i32, ok := ParseInt32(tagDef); ok {
							o.Set(reflect.ValueOf(sql.NullInt32{Int32: int32(i32), Valid: true})) // allow zero defaults
						}
					}
				case sql.NullInt64:
					if !f.Valid {
						if i64, ok := ParseInt64(tagDef); ok {
							o.Set(reflect.ValueOf(sql.NullInt64{Int64: i64, Valid: true})) // allow zero defaults
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

	return true, nil
}

// UnmarshalCSVToStruct will parse csvPayload string (one line of csv data) using csvDelimiter, (if csvDelimiter = "", then customDelimiterParserFunc is required)
// and set parsed csv element value into struct fields based on Ordinal Position defined via struct tag,
// additionally processes struct tag data validation and length / range (if not valid, will set to data type default)
//
// Predefined Struct Tags Usable:
//  1. `pos:"1"`				// ordinal position of the field in relation to the csv parsed output expected (Zero-Based Index)
//     NOTE: if field is mutually exclusive with one or more uniqueId, then pos # should be named the same for all uniqueIds,
//     if multiple fields are in exclusive condition, and skipBlank or skipZero, always include a blank default field as the last of unique field list
//     if value is '-', this means position value is calculated from other fields and set via `setter:"base.Xyz"` during unmarshal csv, there is no marshal to csv for this field
//  2. `type:"xyz"`				// data type expected:
//     A = AlphabeticOnly, N = NumericOnly 0-9, AN = AlphaNumeric, ANS = AN + PrintableSymbols,
//     H = Hex, B64 = Base64, B = true/false, REGEX = Regular Expression, Blank = Any,
//  3. `regex:"xyz"`			// if Type = REGEX, this struct tag contains the regular expression string,
//     regex express such as [^A-Za-z0-9_-]+
//     method will replace any regex matched string to blank
//  4. `size:"x..y"`			// data type size rule:
//     x = Exact size match
//     x.. = From x and up
//     ..y = From 0 up to y
//     x..y = From x to y
//     +%z = Append to x, x.., ..y, x..y; adds additional constraint that the result size must equate to 0 from modulo of z
//  5. `range:"x..y"`			// data type range value when Type is N, if underlying data type is string, method will convert first before testing
//  6. `req:"true"`				// indicates data value is required or not, true or false
//  7. `getter:"Key"`			// if field type is custom struct or enum, specify the custom method getter (no parameters allowed) that returns the expected value in first ordinal result position
//     NOTE: if the method to invoke resides at struct level, precede the method name with 'base.', for example, 'base.XYZ' where XYZ is method name to invoke
//     NOTE: if the method is to receive a parameter value, always in string data type, add '(x)' after the method name, such as 'XYZ(x)' or 'base.XYZ(x)'
//  8. `setter:"ParseByKey`		// if field type is custom struct or enum, specify the custom method (only 1 lookup parameter value allowed) setter that sets value(s) into the field
//     NOTE: if the method to invoke resides at struct level, precede the method name with 'base.', for example, 'base.XYZ' where XYZ is method name to invoke
//     NOTE: setter method always intake a string parameter value
//  9. `outprefix:""`			// for marshal method, if field value is to precede with an output prefix, such as XYZ= (affects marshal queryParams / csv methods only)
//     WARNING: if csv is variable elements count, rather than fixed count ordinal, then csv MUST include outprefix for all fields in order to properly identify target struct field
//  10. `def:""`				// default value to set into struct field in case unmarshal doesn't set the struct field value
//  11. `timeformat:"20060102"`	// for time.Time field, optional date time format, specified as:
//     2006, 06 = year,
//     01, 1, Jan, January = month,
//     02, 2, _2 = day (_2 = width two, right justified)
//     03, 3, 15 = hour (15 = 24 hour format)
//     04, 4 = minute
//     05, 5 = second
//     PM pm = AM PM
//  12. `booltrue:"1"` 			// if field is defined, contains bool literal for true condition, such as 1 or true, that overrides default system bool literal value,
//     if bool literal value is determined by existence of outprefix and itself is blank, place a space in both booltrue and boolfalse (setting blank will negate literal override)
//  13. `boolfalse:"0"`			// if field is defined, contains bool literal for false condition, such as 0 or false, that overrides default system bool literal value
//     if bool literal value is determined by existence of outprefix and itself is blank, place a space in both booltrue and boolfalse (setting blank will negate literal override)
//  14. `validate:"==x"`		// if field has to match a specific value or the entire method call will fail, match data format as:
//     ==xyz (== refers to equal, for numbers and string match, xyz is data to match, case insensitive)
//     [if == validate against one or more values, use ||]
//     !=xyz (!= refers to not equal)
//     [if != validate against one or more values, use &&]
//     >=xyz >>xyz <<xyz <=xyz (greater equal, greater, less than, less equal; xyz must be int or float)
//     :=Xyz where Xyz is a parameterless function defined at struct level, that performs validation, returns bool or error where true or nil indicates validation success
//     note: expected source data type for validate to be effective is string, int, float64; if field is blank and req = false, then validate will be skipped
func UnmarshalCSVToStruct(inputStructPtr interface{}, csvPayload string, csvDelimiter string, customDelimiterParserFunc func(string) []string) error {
	if inputStructPtr == nil {
		return fmt.Errorf("InputStructPtr is Required")
	}
	if LenTrim(csvPayload) == 0 {
		return fmt.Errorf("CSV Payload is Required")
	}
	if len(csvDelimiter) == 0 && customDelimiterParserFunc == nil {
		return fmt.Errorf("CSV Delimiter or Custom Delimiter Func is Required")
	}

	s := reflect.ValueOf(inputStructPtr)
	if s.Kind() != reflect.Ptr {
		return fmt.Errorf("InputStructPtr Must Be Pointer")
	}
	s = s.Elem()
	if s.Kind() != reflect.Struct {
		return fmt.Errorf("InputStructPtr Must Be Struct")
	}

	trueList := []string{"true", "yes", "on", "1", "enabled"}

	var csvElements []string
	if len(csvDelimiter) > 0 {
		csvElements = strings.Split(csvPayload, csvDelimiter)
	} else {
		csvElements = customDelimiterParserFunc(csvPayload)
	}
	if len(csvElements) == 0 {
		return fmt.Errorf("CSV Payload Contains Zero Elements")
	}

	StructClearFields(inputStructPtr)
	if _, err := SetStructFieldDefaultValues(inputStructPtr); err != nil { // propagate default-setting errors
		return err
	}
	prefixProcessedMap := make(map[string]string)

	// collect virtual fields (pos < 0) that rely on setters so we can execute them after positional fields are loaded.
	type virtualSetter struct {
		cfg   csvUnmarshalConfig
		field reflect.StructField
		o     reflect.Value
	}
	var virtualSetters []virtualSetter

	for i := 0; i < s.NumField(); i++ {
		field := s.Type().Field(i)
		o := s.FieldByName(field.Name)
		if !o.IsValid() || !o.CanSet() {
			continue
		}

		cfg, ok := csvParseUnmarshalConfig(field)
		if !ok {
			continue
		}

		// capture virtual setters immediately and skip positional parsing
		if cfg.pos < 0 {
			// fail fast if a required virtual field has no setter
			if LenTrim(cfg.tagSetter) == 0 && strings.ToLower(cfg.tagReq) == "true" {
				StructClearFields(inputStructPtr)
				return fmt.Errorf("%s is a Required Field", field.Name)
			}
			if LenTrim(cfg.tagSetter) > 0 {
				virtualSetters = append(virtualSetters, virtualSetter{cfg: cfg, field: field, o: o})
			}
			continue
		}

		// get raw CSV value
		rawVal, found := csvExtractValue(csvElements, cfg, prefixProcessedMap)

		defVal := Trim(field.Tag.Get("def")) // consider defaults for required fields
		// enforce required fields when missing entirely
		if !found {
			if cfg.tagReq == "true" && LenTrim(defVal) == 0 {
				StructClearFields(inputStructPtr)
				return fmt.Errorf("%s is a Required Field", field.Name)
			}
			continue
		}

		if !found && cfg.pos >= 0 && LenTrim(cfg.outPrefix) == 0 {
			continue
		}
		csvValue := rawVal
		if strings.EqualFold(Trim(csvValue), "null") { // treat literal null as empty input to prevent numeric/regex validation errors
			csvValue = ""
		}

		// bool literal overrides
		if cfg.boolTrue == " " && len(cfg.outPrefix) > 0 && csvValue == cfg.outPrefix {
			csvValue = "true"
		} else {
			if LenTrim(cfg.boolTrue) > 0 && csvValue == cfg.boolTrue {
				csvValue = "true"
			} else if LenTrim(cfg.boolFalse) > 0 && csvValue == cfg.boolFalse {
				csvValue = "false"
			}
		}

		hasSetter := LenTrim(cfg.tagSetter) > 0
		// preprocess & normalize
		valPrep, err := csvPreprocessValue(csvValue, cfg, hasSetter, trueList)
		if err != nil {
			StructClearFields(inputStructPtr)
			return err
		}
		csvValue = valPrep

		// apply setter if needed
		if newVal, setDone, err := csvApplySetter(s, cfg, o, csvValue); err != nil {
			StructClearFields(inputStructPtr)
			return err
		} else if setDone {
			if err := csvValidateValue(newVal, cfg, field.Name); err != nil {
				StructClearFields(inputStructPtr)
				return err
			}
			if err := csvValidateCustomUnmarshal(newVal, cfg, s, field.Name); err != nil {
				StructClearFields(inputStructPtr)
				return err
			}
			continue
		} else {
			csvValue = newVal
		}

		// validation
		if err := csvValidateValue(csvValue, cfg, field.Name); err != nil {
			StructClearFields(inputStructPtr)
			return err
		}
		if err := csvValidateCustomUnmarshal(csvValue, cfg, s, field.Name); err != nil {
			StructClearFields(inputStructPtr)
			return err
		}

		// set field
		if err := ReflectStringToField(o, csvValue, cfg.timeFormat); err != nil {
			StructClearFields(inputStructPtr) // clear partial state on assignment failure
			return err
		}
	}

	// execute virtual setters (pos < 0) after positional fields are populated.
	for _, vs := range virtualSetters {
		if newVal, setDone, err := csvApplySetter(s, vs.cfg, vs.o, ""); err != nil { // clear struct on setter error
			StructClearFields(inputStructPtr)
			return err
		} else if setDone { // validate virtual setter output
			// validation for setter-handled assignments (e.g., pointer/slice returns)
			if err := csvValidateValue(newVal, vs.cfg, vs.field.Name); err != nil { // ensure validation runs even when setter handled assignment
				StructClearFields(inputStructPtr)
				return err
			}
			if err := csvValidateCustomUnmarshal(newVal, vs.cfg, s, vs.field.Name); err != nil {
				StructClearFields(inputStructPtr)
				return err
			}
			continue
		} else {
			// handle scalar-returning setters by validating and assigning to the field
			if err := csvValidateValue(newVal, vs.cfg, vs.field.Name); err != nil {
				StructClearFields(inputStructPtr)
				return err
			}
			if err := csvValidateCustomUnmarshal(newVal, vs.cfg, s, vs.field.Name); err != nil {
				StructClearFields(inputStructPtr)
				return err
			}
			if err := ReflectStringToField(vs.o, newVal, vs.cfg.timeFormat); err != nil {
				StructClearFields(inputStructPtr)
				return err
			}
		}
	}

	// enforce required virtual fields after setter execution
	for _, vs := range virtualSetters {
		if strings.ToLower(vs.cfg.tagReq) == "true" {
			defVal := Trim(vs.field.Tag.Get("def")) // respect defaults for virtual required fields
			if vs.o.IsZero() && LenTrim(defVal) == 0 {
				StructClearFields(inputStructPtr)
				return fmt.Errorf("%s is a Required Field", vs.field.Name)
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
//  1. `pos:"1"`				// ordinal position of the field in relation to the csv parsed output expected (Zero-Based Index)
//     NOTE: if field is mutually exclusive with one or more uniqueId, then pos # should be named the same for all uniqueIds
//     if multiple fields are in exclusive condition, and skipBlank or skipZero, always include a blank default field as the last of unique field list
//     if value is '-', this means position value is calculated from other fields and set via `setter:"base.Xyz"` during unmarshal csv, there is no marshal to csv for this field
//  2. `type:"xyz"`				// data type expected:
//     A = AlphabeticOnly, N = NumericOnly 0-9, AN = AlphaNumeric, ANS = AN + PrintableSymbols,
//     H = Hex, B64 = Base64, B = true/false, REGEX = Regular Expression, Blank = Any,
//  3. `regex:"xyz"`			// if Type = REGEX, this struct tag contains the regular expression string,
//     regex express such as [^A-Za-z0-9_-]+
//     method will replace any regex matched string to blank
//  4. `size:"x..y"`			// data type size rule:
//     x = Exact size match
//     x.. = From x and up
//     ..y = From 0 up to y
//     x..y = From x to y
//     +%z = Append to x, x.., ..y, x..y; adds additional constraint that the result size must equate to 0 from modulo of z
//  5. `range:"x..y"`			// data type range value when Type is N, if underlying data type is string, method will convert first before testing
//  6. `req:"true"`				// indicates data value is required or not, true or false
//  7. `getter:"Key"`			// if field type is custom struct or enum, specify the custom method getter (no parameters allowed) that returns the expected value in first ordinal result position
//     NOTE: if the method to invoke resides at struct level, precede the method name with 'base.', for example, 'base.XYZ' where XYZ is method name to invoke
//     NOTE: if the method is to receive a parameter value, always in string data type, add '(x)' after the method name, such as 'XYZ(x)' or 'base.XYZ(x)'
//  8. `setter:"ParseByKey`		// if field type is custom struct or enum, specify the custom method (only 1 lookup parameter value allowed) setter that sets value(s) into the field
//     NOTE: if the method to invoke resides at struct level, precede the method name with 'base.', for example, 'base.XYZ' where XYZ is method name to invoke
//     NOTE: setter method always intake a string parameter value
//  9. `booltrue:"1"` 			// if field is defined, contains bool literal for true condition, such as 1 or true, that overrides default system bool literal value,
//     if bool literal value is determined by existence of outprefix and itself is blank, place a space in both booltrue and boolfalse (setting blank will negate literal override)
//  10. `boolfalse:"0"`			// if field is defined, contains bool literal for false condition, such as 0 or false, that overrides default system bool literal value
//     if bool literal value is determined by existence of outprefix and itself is blank, place a space in both booltrue and boolfalse (setting blank will negate literal override)
//  11. `uniqueid:"xyz"`		// if two or more struct field is set with the same uniqueid, then only the first encountered field with the same uniqueid will be used in marshal,
//     NOTE: if field is mutually exclusive with one or more uniqueId, then pos # should be named the same for all uniqueIds
//  12. `skipblank:"false"`		// if true, then any fields that is blank string will be excluded from marshal (this only affects fields that are string)
//  13. `skipzero:"false"`		// if true, then any fields that are 0, 0.00, time.Zero(), false, nil will be excluded from marshal (this only affects fields that are number, bool, time, pointer)
//  14. `timeformat:"20060102"`	// for time.Time field, optional date time format, specified as:
//     2006, 06 = year,
//     01, 1, Jan, January = month,
//     02, 2, _2 = day (_2 = width two, right justified)
//     03, 3, 15 = hour (15 = 24 hour format)
//     04, 4 = minute
//     05, 5 = second
//     PM pm = AM PM
//  15. `outprefix:""`			// for marshal method, if field value is to precede with an output prefix, such as XYZ= (affects marshal queryParams / csv methods only)
//     WARNING: if csv is variable elements count, rather than fixed count ordinal, then csv MUST include outprefix for all fields in order to properly identify target struct field
//  16. `zeroblank:"false"`		// set true to set blank to data when value is 0, 0.00, or time.IsZero
//  17. `validate:"==x"`		// if field has to match a specific value or the entire method call will fail, match data format as:
//     ==xyz (== refers to equal, for numbers and string match, xyz is data to match, case insensitive)
//     [if == validate against one or more values, use ||]
//     !=xyz (!= refers to not equal)
//     [if != validate against one or more values, use &&]
//     >=xyz >>xyz <<xyz <=xyz (greater equal, greater, less than, less equal; xyz must be int or float)
//     :=Xyz where Xyz is a parameterless function defined at struct level, that performs validation, returns bool or error where true or nil indicates validation success
//     note: expected source data type for validate to be effective is string, int, float64; if field is blank and req = false, then validate will be skipped
func MarshalStructToCSV(inputStructPtr interface{}, csvDelimiter string) (csvPayload string, err error) {
	if inputStructPtr == nil {
		return "", fmt.Errorf("InputStructPtr is Required")
	}

	s := reflect.ValueOf(inputStructPtr)
	if s.Kind() != reflect.Ptr {
		return "", fmt.Errorf("InputStructPtr Must Be Pointer")
	}
	s = s.Elem()
	if s.Kind() != reflect.Struct {
		return "", fmt.Errorf("InputStructPtr Must Be Struct")
	}

	if !IsStructFieldSet(inputStructPtr) && StructNonDefaultRequiredFieldsCount(inputStructPtr) > 0 {
		// fail fast instead of silently returning blank when required fields are missing
		return "", fmt.Errorf("MarshalStructToCSV Requires Required Fields To Be Set")
	}

	trueList := []string{"true", "yes", "on", "1", "enabled"}

	csvLen, err := csvComputeBufferLength(s)
	if err != nil {
		return "", err
	}
	if csvLen == 0 {
		return "", nil
	}

	csvList := make([]string, csvLen)
	for i := range csvList {
		csvList[i] = "{?}"
	}

	excludePlaceholders := true
	uniqueMap := make(map[string]string)

	for i := 0; i < s.NumField(); i++ {
		field := s.Type().Field(i)
		o := s.FieldByName(field.Name)
		if !o.IsValid() || !o.CanSet() {
			continue
		}

		cfg, ok := csvParseFieldConfig(field)
		if !ok || cfg.pos > len(csvList)-1 {
			continue
		}

		uniqueKey := "" // defer uniqueID claim until we know the field will be emitted
		if len(cfg.uniqueID) > 0 {
			uniqueKey = strings.ToLower(cfg.uniqueID)
			if _, seen := uniqueMap[uniqueKey]; seen {
				continue
			}
		}

		// only exclude placeholders if ALL fields have outprefix; once a field lacks outprefix, keep placeholders
		if excludePlaceholders && LenTrim(cfg.outPrefix) == 0 {
			excludePlaceholders = false
		}

		oldVal := o
		hasGetter := len(cfg.tagGetter) > 0
		if hasGetter {
			o = csvApplyGetter(s, cfg, o, cfg.boolTrue, cfg.boolFalse, cfg.skipBlank, cfg.skipZero, cfg.timeFormat, cfg.zeroBlank)
		}

		fv, skip, e := ReflectValueToString(o, cfg.boolTrue, cfg.boolFalse, cfg.skipBlank, cfg.skipZero, cfg.timeFormat, cfg.zeroBlank)
		if e != nil {
			return "", e
		}
		if skip {
			continue
		}

		if oldVal.Kind() == reflect.Int && oldVal.Int() == 0 && strings.ToLower(fv) == "unknown" {
			fv = ""
			if len(cfg.defVal) > 0 {
				fv = cfg.defVal
			} else if len(uniqueKey) > 0 {
				continue
			}
		}

		fv, skipVal, errVal := csvValidateAndNormalize(fv, cfg, oldVal, hasGetter, trueList)
		if errVal != nil {
			return "", fmt.Errorf("%s %s", field.Name, errVal.Error())
		}
		if skipVal {
			csvList[cfg.pos] = ""
			continue
		}

		if errVal = csvValidateCustom(fv, cfg, cfg.tagReq, s, field); errVal != nil {
			return "", fmt.Errorf("%s %s", field.Name, errVal.Error())
		}

		if cfg.skipBlank && LenTrim(fv) == 0 {
			csvList[cfg.pos] = ""
			continue
		} else if cfg.skipZero && fv == "0" {
			csvList[cfg.pos] = ""
			continue
		}

		if len(uniqueKey) > 0 {
			uniqueMap[uniqueKey] = field.Name // claim uniqueID only when we actually emit a value
		}

		csvList[cfg.pos] = cfg.outPrefix + fv
	}

	for _, v := range csvList {
		if excludePlaceholders {
			if v != "{?}" && LenTrim(v) > 0 {
				if len(csvPayload) > 0 { // only add delimiter after an emitted value
					csvPayload += csvDelimiter
				}
				csvPayload += v
			}
			continue
		}

		if v == "{?}" {
			continue // skip placeholders without advancing delimiter state
		}
		if len(csvPayload) > 0 { // avoid leading delimiter
			csvPayload += csvDelimiter
		}
		csvPayload += v
	}

	return csvPayload, nil
}
