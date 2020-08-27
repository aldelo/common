package helper

import "strconv"

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

// IsInt32 tests if input string is integer (whole numbers 32 bits)
func IsInt32(s string) bool {
	if _, err := strconv.Atoi(s); err != nil {
		return false
	}

	return true
}

// IsInt64 tests if input string is big integer (whole number greater 64 bits)
func IsInt64(s string) bool {
	if _, err := strconv.ParseInt(s, 10, 64); err != nil {
		return false
	}

	return true
}

// IsFloat32 tests if input string is float 32 bit (decimal point value)
func IsFloat32(s string) bool {
	if _, err := strconv.ParseFloat(s, 32); err != nil {
		return false
	}

	return true
}

// IsFloat64 tests if input string is float 64 bit (decimal point value)
func IsFloat64(s string) bool {
	if _, err := strconv.ParseFloat(s, 64); err != nil {
		return false
	}

	return true
}

// IsBool tests if input string is boolean
func IsBool(s string) bool {
	if _, err := strconv.ParseBool(s); err != nil {
		return false
	}

	return true
}

// ParseInt32 tests and parses if input string is integer (whole numbers 32 bits)
func ParseInt32(s string) (int, bool) {
	var result int
	var err error

	if result, err = strconv.Atoi(s); err != nil {
		return 0, false
	}

	return result, true
}

// ParseInt64 tests and parses if input string is big integer (whole number greater 64 bits)
func ParseInt64(s string) (int64, bool) {
	var result int64
	var err error

	if result, err = strconv.ParseInt(s, 10, 64); err != nil {
		return 0, false
	}

	return result, true
}

// ParseFloat32 tests and parses if input string is float 32 bit (decimal point value)
func ParseFloat32(s string) (float32, bool) {
	var result float64
	var err error

	if result, err = strconv.ParseFloat(s, 32); err != nil {
		return 0.00, false
	}

	return float32(result), true
}

// ParseFloat64 tests and parses if input string is float 64 bit (decimal point value)
func ParseFloat64(s string) (float64, bool) {
	var result float64
	var err error

	if result, err = strconv.ParseFloat(s, 64); err != nil {
		return 0.00, false
	}

	return result, true
}

// ParseBool tests and parses if input string is boolean,
// return value 1st bool is the boolean result,
// return value 2nd bool is the ParseBool success or failure indicator
func ParseBool(s string) (bool, bool) {
	var result bool
	var err error

	if result, err = strconv.ParseBool(s); err != nil {
		return false, false
	}

	return result, true
}
