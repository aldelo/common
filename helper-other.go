package helper

import (
	"fmt"
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

// ================================================================================================================
// console helpers
// ================================================================================================================

// ConsoleLPromptAndAnswer is a helper to prompt a message and then scan a response in console
func ConsolePromptAndAnswer(prompt string, replyLowercase bool) string {
	fmt.Print(prompt)

	answer := ""

	if _, e := fmt.Scanln(&answer); e != nil {
		fmt.Println("Scan Error: ", e)
	} else {
		answer = RightTrimLF(answer)

		if replyLowercase {
			answer = strings.ToLower(answer)
		}

		fmt.Println()
	}

	return answer
}

// ConsoleLPromptAndAnswerBool is a helper to prompt a message and then scan a response in console
func ConsolePromptAndAnswerBool(prompt string) bool {
	fmt.Print(prompt)

	answer := ""
	result := false

	if _, e := fmt.Scanln(&answer); e != nil {
		fmt.Println("Scan Error: ", e)
	} else {
		answer = RightTrimLF(answer)
		result, _ = ParseBool(answer)

		fmt.Println()
	}

	return result
}

// ConsoleLPromptAndAnswerInt is a helper to prompt a message and then scan a response in console
func ConsolePromptAndAnswerInt(prompt string) int {
	fmt.Print(prompt)

	answer := ""
	result := 0

	if _, e := fmt.Scanln(&answer); e != nil {
		fmt.Println("Scan Error: ", e)
	} else {
		answer = RightTrimLF(answer)
		result, _ = ParseInt32(answer)

		fmt.Println()
	}

	return result
}

// ConsoleLPromptAndAnswerFloat64 is a helper to prompt a message and then scan a response in console
func ConsolePromptAndAnswerFloat64(prompt string) float64 {
	fmt.Print(prompt)

	answer := ""
	result := float64(0)

	if _, e := fmt.Scanln(&answer); e != nil {
		fmt.Println("Scan Error: ", e)
	} else {
		answer = RightTrimLF(answer)
		result, _ = ParseFloat64(answer)

		fmt.Println()
	}

	return result
}