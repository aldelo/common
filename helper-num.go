package helper

import (
	"math"
	"strconv"
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

// AbsInt returns absolute value of i
func AbsInt(i int) int {
	maxInt := int(^uint(0) >> 1) // compute platform max int
	minInt := -maxInt - 1        // compute platform min int

	if i == minInt { // guard overflow on MinInt
		return maxInt // saturate to max to avoid overflow
	}

	if i < 0 {
		return -i
	} else {
		return i
	}
}

// AbsInt64 returns absolute value of i
func AbsInt64(i int64) int64 {
	if i == math.MinInt64 { // guard overflow on MinInt64
		return math.MaxInt64 // saturate to max to avoid overflow
	}

	if i < 0 {
		return i * -1
	} else {
		return i
	}
}

// AbsDuration returns absolute value of d
func AbsDuration(d time.Duration) time.Duration {
	if d == time.Duration(math.MinInt64) { // guard overflow on MinInt64
		return time.Duration(math.MaxInt64) // saturate to max to avoid overflow
	}

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
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 32)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) { // reject NaN/Inf
		return false
	}
	return true
}

// IsFloat64 tests if input string is float 64 bit (decimal point value)
func IsFloat64(s string) bool {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) { // reject NaN/Inf
		return false
	}
	return true
}

// IsBoolType tests if input string is boolean
func IsBoolType(s string) bool {
	s = strings.TrimSpace(s) // trim surrounding whitespace

	truthy := []string{"yes", "on", "running", "started", "y", "1"}
	falsy := []string{"no", "off", "stopped", "n", "0"}

	ls := strings.ToLower(s)
	switch {
	case StringSliceContains(&truthy, ls):
		s = "true"
	case StringSliceContains(&falsy, ls):
		s = "false"
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

	var result int64
	var err error

	if result, err = strconv.ParseInt(strings.TrimSpace(s), 10, 32); err != nil {
		return 0, false
	}

	// validate using 32-bit bounds before converting to platform int
	if result < int64(math.MinInt32) || result > int64(math.MaxInt32) {
		return 0, false
	}

	return int(result), true
}

// ParseInt64 tests and parses if input string is big integer (whole number greater 64 bits)
func ParseInt64(s string) (int64, bool) {
	if strings.Index(s, ".") >= 0 {
		s = SplitString(s, ".", 0)
	}

	result, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, false
	}

	return result, true
}

// ParseUint64 tests and parses if input string is big integer (whole number greater 64 bits)
func ParseUint64(s string) (uint64, bool) {
	if strings.Index(s, ".") >= 0 {
		s = SplitString(s, ".", 0)
	}

	result, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, false
	}

	return result, true
}

// ParseFloat32 tests and parses if input string is float 32 bit (decimal point value)
func ParseFloat32(s string) (float32, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 32)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) { // reject NaN/Inf
		return 0.00, false
	}
	return float32(v), true
}

// ParseFloat64 tests and parses if input string is float 64 bit (decimal point value)
func ParseFloat64(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) { // reject NaN/Inf
		return 0.00, false
	}
	return v, true
}

// ParseBool tests and parses if input string is boolean,
// return value 1st bool is the boolean result,
// return value 2nd bool is the ParseBool success or failure indicator
func ParseBool(s string) (bool, bool) {
	s = strings.TrimSpace(s) // trim surrounding whitespace

	var result bool
	var err error

	xTruthy := []string{"yes", "on", "running", "started", "y", "1"}
	xFalsy := []string{"no", "off", "stopped", "n", "0"}

	ls := strings.ToLower(s)
	switch {
	case StringSliceContains(&xTruthy, ls):
		s = "true"
	case StringSliceContains(&xFalsy, ls):
		s = "false"
	}

	if result, err = strconv.ParseBool(s); err != nil {
		return false, false
	}

	return result, true
}

// ExponentialToNumber converts exponential representation of a number into actual number equivalent
func ExponentialToNumber(exp string) string {
	exp = strings.TrimSpace(exp) // accept inputs with surrounding whitespace

	if strings.ContainsAny(exp, "eE") { // cheaper check for exponent marker
		v, err := strconv.ParseFloat(exp, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) { // preserve original on error/NaN/Inf
			return exp
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	}

	return exp
}

// RoundFloat64 converts float64 value to target precision
func RoundFloat64(val float64, precision uint) float64 {
	if math.IsNaN(val) || math.IsInf(val, 0) { // passthrough special values
		return val
	}

	const maxSafePrecision = 308 // 10^308 fits in float64; 10^309 overflows to +Inf
	if precision > maxSafePrecision {
		precision = maxSafePrecision // cap to avoid Inf ratio
	}

	ratio := math.Pow(10, float64(precision))
	if math.IsInf(ratio, 0) || ratio == 0 { // extra safety (should not occur after cap)
		return val
	}

	scaled := val * ratio                            // compute once to check overflow
	if math.IsInf(scaled, 0) || math.IsNaN(scaled) { // avoid overflow/NaN on scaling
		return val // fall back to original value if scaling overflows
	}

	return math.Round(scaled) / ratio // use precomputed scaled value
}

// RoundIntegerToNearestBlock returns conformed block size value where val fall into.
// For example: if val is 3 and blockSize is 5, then return value is 5;
// where as if val is 13 and blockSize is 5, then return value is 15;
// val must be > 0
func RoundIntegerToNearestBlock(val int, blockSize int) int {
	if val < 0 {
		return -1
	}

	if blockSize < 1 {
		return -1
	}

	if val == 0 {
		return blockSize
	}

	remainder := val % blockSize // integer-only rounding
	if remainder == 0 {          // exact multiple, return as-is
		return val
	}
	return val + blockSize - remainder // round up to next block
}
