package helper

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"time"
)

/*
 * Copyright 2020-2023 Aldelo, LP
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
// Variadic Optional Value Helpers
// ================================================================================================================

// GetFirstOrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstOrDefault(defaultValue interface{}, paramValue ...interface{}) interface{} {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// GetFirstIntOrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstIntOrDefault(defaultValue int, paramValue ...int) int {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// GetFirstInt64OrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstInt64OrDefault(defaultValue int64, paramValue ...int64) int64 {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// GetFirstStringOrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstStringOrDefault(defaultValue string, paramValue ...string) string {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// GetFirstBoolOrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstBoolOrDefault(defaultValue bool, paramValue ...bool) bool {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// GetFirstFloat32OrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstFloat32OrDefault(defaultValue float32, paramValue ...float32) float32 {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// GetFirstFloat64OrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstFloat64OrDefault(defaultValue float64, paramValue ...float64) float64 {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// GetFirstTimeOrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstTimeOrDefault(defaultValue time.Time, paramValue ...time.Time) time.Time {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// GetFirstByteOrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstByteOrDefault(defaultValue byte, paramValue ...byte) byte {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// ================================================================================================================
// slice helpers
// ================================================================================================================

// IntSliceContains checks if value is contained within the intSlice
func IntSliceContains(intSlice *[]int, value int) bool {
	if intSlice == nil {
		return false
	} else {
		for _, v := range *intSlice {
			if v == value {
				return true
			}
		}

		return false
	}
}

// StringSliceContains checks if value is contained within the strSlice
func StringSliceContains(strSlice *[]string, value string) bool {
	if strSlice == nil {
		return false
	} else {
		for _, v := range *strSlice {
			if strings.ToLower(v) == strings.ToLower(value) {
				return true
			}
		}

		return false
	}
}

// StringSliceExtractUnique returns unique string slice elements
func StringSliceExtractUnique(strSlice []string) (result []string) {
	if strSlice == nil {
		return []string{}
	} else if len(strSlice) <= 1 {
		return strSlice
	} else {
		for _, v := range strSlice {
			if !StringSliceContains(&result, v) {
				result = append(result, v)
			}
		}

		return result
	}
}

// SliceSeekElement returns the first filterFunc input object's true response
// note: use SliceObjectToSliceInterface to convert slice of objects to slice of interface before passing to slice parameter
func SliceSeekElement(slice []interface{}, filterFunc func(input interface{}, filter ...interface{}) bool, filterParam ...interface{}) interface{} {
	if len(slice) == 0 {
		return nil
	}

	if filterFunc == nil {
		return nil
	}

	if len(filterParam) == 0 {
		return nil
	}

	for _, v := range slice {
		if filterFunc(v, filterParam...) {
			// found
			return v
		}
	}

	// not found
	return nil
}

// SliceDeleteElement accepts slice (value type or pointer type, primitive or complex struct),
// removes element by index position removalIndex,
// and returns the reassembled result slice without the removed element
//
// note: this method does not preserve element ordering, this is in order to achieve faster call performance
//
// removalIndex = positive number indicates element removal index position (0-based index)
//
//	  negative number indicates element removal index from right,
//		-1 = last element to remove; -2 = second to the last to remove, and so on
//	  positive / negative number out of bound = returns original slice unchanged
//
// if resultSlice is nil, then no slice remain
func SliceDeleteElement(slice interface{}, removalIndex int) (resultSlice interface{}) {
	sliceObj := reflect.ValueOf(slice)

	if sliceObj.Kind() == reflect.Ptr {
		sliceObj = sliceObj.Elem()
	}

	if sliceObj.Kind() != reflect.Slice {
		return nil
	}

	if removalIndex < 0 {
		removalIndex = sliceObj.Len() - AbsInt(removalIndex)

		if removalIndex < 0 {
			return slice
		}
	}

	if removalIndex > sliceObj.Len()-1 {
		return slice
	}

	rm := sliceObj.Index(removalIndex)
	last := sliceObj.Index(sliceObj.Len() - 1)

	if rm.CanSet() {
		rm.Set(last)
	} else {
		return slice
	}

	return sliceObj.Slice(0, sliceObj.Len()-1).Interface()
}

// ================================================================================================================
// console helpers
// ================================================================================================================

// ConsolePromptAndAnswer is a helper to prompt a message and then scan a response in console
func ConsolePromptAndAnswer(prompt string, replyLowercase bool, autoTrim ...bool) string {
	fmt.Print(prompt)

	answer := ""

	if _, e := fmt.Scanln(&answer); e != nil {
		answer = ""
		fmt.Println()
	} else {
		answer = RightTrimLF(answer)

		if replyLowercase {
			answer = strings.ToLower(answer)
		}

		if len(autoTrim) > 0 {
			if autoTrim[0] {
				answer = Trim(answer)
			}
		}

		fmt.Println()
	}

	return answer
}

// ConsolePromptAndAnswerBool is a helper to prompt a message and then scan a response in console
func ConsolePromptAndAnswerBool(prompt string, defaultTrue ...bool) bool {
	fmt.Print(prompt)

	answer := ""
	result := false
	defVal := false

	if len(defaultTrue) > 0 {
		if defaultTrue[0] {
			defVal = true
		}
	}

	if _, e := fmt.Scanln(&answer); e != nil {
		fmt.Println()
		return defVal
	} else {
		answer = RightTrimLF(answer)

		if LenTrim(answer) > 0 {
			result, _ = ParseBool(answer)
		} else {
			result = defVal
		}

		fmt.Println()
	}

	return result
}

// ConsolePromptAndAnswerInt is a helper to prompt a message and then scan a response in console
func ConsolePromptAndAnswerInt(prompt string, preventNegative ...bool) int {
	fmt.Print(prompt)

	answer := ""
	result := 0

	if _, e := fmt.Scanln(&answer); e != nil {
		fmt.Println()
		return 0
	} else {
		answer = RightTrimLF(answer)
		result, _ = ParseInt32(answer)

		if result < 0 {
			if len(preventNegative) > 0 {
				if preventNegative[0] {
					result = 0
				}
			}
		}

		fmt.Println()
	}

	return result
}

// ConsolePromptAndAnswerFloat64 is a helper to prompt a message and then scan a response in console
func ConsolePromptAndAnswerFloat64(prompt string, preventNegative ...bool) float64 {
	fmt.Print(prompt)

	answer := ""
	result := float64(0)

	if _, e := fmt.Scanln(&answer); e != nil {
		fmt.Println()
		return 0
	} else {
		answer = RightTrimLF(answer)
		result, _ = ParseFloat64(answer)

		if result < 0 {
			if len(preventNegative) > 0 {
				if preventNegative[0] {
					result = 0
				}
			}
		}

		fmt.Println()
	}

	return result
}

// ErrorMessage find the error cause time, file, code line number and error message
func ErrorMessage(err error) string {

	_, file, line, _ := runtime.Caller(1)
	indexFunc := func(file string) string {
		backup := "/" + file
		lastSlashIndex := strings.LastIndex(backup, "/")
		if lastSlashIndex < 0 {
			return backup
		}
		secondLastSlashIndex := strings.LastIndex(backup[:lastSlashIndex], "/")
		if secondLastSlashIndex < 0 {
			return backup[lastSlashIndex+1:]
		}
		return backup[secondLastSlashIndex+1:]
	}

	logmessage := fmt.Sprintf("%v %v:%v:%v", time.Now().UTC().Format("2006-01-02 15:04:05.000"), indexFunc(file), line, err.Error())

	return logmessage
}
