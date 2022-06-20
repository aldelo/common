package helper

import (
	"math"
	"strconv"
	"strings"
	"time"
)

/*
 * Copyright 2020-2021 Aldelo, LP
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

// AbsInt returns absolute value of i
func AbsInt(i int) int {
	if i < 0 {
		return i * -1
	} else {
		return i
	}
}

// AbsInt64 returns absolute value of i
func AbsInt64(i int64) int64 {
	if i < 0 {
		return i * -1
	} else {
		return i
	}
}

// AbsDuration returns absolute value of d
func AbsDuration(d time.Duration) time.Duration {
	if d < 0 {
		return d * -1
	} else {
		return d
	}
}

// AbsFloat64 returns absolute value of f
func AbsFloat64(f float64) float64 {
	if f < 0 {
		return f * -1
	} else {
		return f
	}
}

// IsInt32 tests if input string is integer (whole numbers 32 bits)
func IsInt32(s string) bool {
	if _, err := strconv.ParseInt(strings.TrimSpace(s), 10, 32); err != nil {

		return false
	}

	return true
}

// IsInt64 tests if input string is big integer (whole number greater 64 bits)
func IsInt64(s string) bool {
	if _, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err != nil {
		return false
	}

	return true
}

// IsFloat32 tests if input string is float 32 bit (decimal point value)
func IsFloat32(s string) bool {
	if _, err := strconv.ParseFloat(strings.TrimSpace(s), 32); err != nil {
		return false
	}

	return true
}

// IsFloat64 tests if input string is float 64 bit (decimal point value)
func IsFloat64(s string) bool {
	if _, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err != nil {
		return false
	}

	return true
}

// IsBoolType tests if input string is boolean
func IsBoolType(s string) bool {
	x := []string{"yes", "on", "running", "started"}

	if StringSliceContains(&x, strings.ToLower(s)) {
		s = "true"
	}

	if _, err := strconv.ParseBool(s); err != nil {
		return false
	}

	return true
}

// ParseInt32 tests and parses if input string is integer (whole numbers 32 bits)
func ParseInt32(s string) (int, bool) {
	if strings.Index(s, ".") >= 0 {
		s = SplitString(s, ".", 0)
	}

	var result int
	var err error

	if result, err = strconv.Atoi(strings.TrimSpace(s)); err != nil {
		return 0, false
	}

	return result, true
}

// ParseInt64 tests and parses if input string is big integer (whole number greater 64 bits)
func ParseInt64(s string) (int64, bool) {
	if strings.Index(s, ".") >= 0 {
		s = SplitString(s, ".", 0)
	}

	var result int64
	var err error

	if result, err = strconv.ParseInt(strings.TrimSpace(s), 10, 64); err != nil {
		return 0, false
	}

	return result, true
}

// ParseFloat32 tests and parses if input string is float 32 bit (decimal point value)
func ParseFloat32(s string) (float32, bool) {
	var result float64
	var err error

	if result, err = strconv.ParseFloat(strings.TrimSpace(s), 32); err != nil {
		return 0.00, false
	}

	return float32(result), true
}

// ParseFloat64 tests and parses if input string is float 64 bit (decimal point value)
func ParseFloat64(s string) (float64, bool) {
	var result float64
	var err error

	if result, err = strconv.ParseFloat(strings.TrimSpace(s), 64); err != nil {
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

	x := []string{"yes", "on", "running", "started", "y", "1"}

	if StringSliceContains(&x, strings.ToLower(s)) {
		s = "true"
	}

	if result, err = strconv.ParseBool(s); err != nil {
		return false, false
	}

	return result, true
}

// ExponentialToNumber converts exponential representation of a number into actual number equivalent
func ExponentialToNumber(exp string) string {
	if strings.Index(strings.ToLower(exp), "e") >= 0 {
		v, _ := ParseFloat64(exp)
		return strconv.FormatFloat(v, 'f', -1, 64)
	} else {
		return exp
	}
}

// RoundFloat64 converts float64 value to target precision
func RoundFloat64(val float64, precision uint) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}
