// Package helper:
// common project provides commonly used helper utility functions, custom utility types, and third party package wrappers.
// common project helps code reuse, and faster composition of logic without having to delve into commonly recurring code logic and related testings.
// common project source directories and brief description:
// + /ascii = helper types and/or functions related to ascii manipulations.
// + /crypto = helper types and/or functions related to encryption, decryption, hashing, such as rsa, aes, sha, tls etc.
// + /csv = helper types and/or functions related to csv file manipulations.
// + /rest = helper types and/or functions related to http rest api GET, POST, PUT, DELETE actions invoked from client side.
// + /tcp = helper types providing wrapped tcp client and tcp server logic.
// - /wrapper = wrappers provides a simpler usage path to third party packages, as well as adding additional enhancements.
//   - /aws = contains aws sdk helper types and functions.
//   - /cloudmap = wrapper for aws cloudmap service discovery.
//   - /dynamodb = wrapper for aws dynamodb data access and manipulations.
//   - /gin = wrapper for gin-gonic web server, and important middleware, into a ready to use solution.
//   - /hystrixgo = wrapper for circuit breaker logic.
//   - /kms = wrapper for aws key management services.
//   - /mysql = wrapper for mysql + sqlx data access and manipulation.
//   - /ratelimit = wrapper for ratelimit logic.
//   - /redis = wrapper for redis data access and manipulation, using go-redis.
//   - /s3 = wrapper for aws s3 data access and manipulation.
//   - /ses = wrapper for aws simple email services.
//   - /sns = wrapper for aws simple notification services.
//   - /sqlite = wrapper for sqlite + sqlx data access and manipulation.
//   - /sqlserver = wrapper for sqlserver + sqlx data access and manipulation.
//   - /sqs = wrapper for aws simple queue services.
//   - /systemd = wrapper for kardianos service to support systemd, windows, launchd service creations.
//   - /viper = wrapper for viper config.
//   - /waf2 = wrapper for aws waf2 (web application firewall v2).
//   - /xray = wrapper for aws xray distributed tracing.
//   - /zap = wrapper for zap logging.
//
// /helper-conv.go = helpers for data conversion operations.
// /helper-db.go = helpers for database data type operations.
// /helper-emv.go = helpers for emv chip card related operations.
// /helper-io.go = helpers for io related operations.
// /helper-net.go = helpers for network related operations.
// /helper-num.go = helpers for numeric related operations.
// /helper-other.go = helpers for misc. uncategorized operations.
// /helper-reflect.go = helpers for reflection based operations.
// /helper-regex.go = helpers for regular express related operations.
// /helper-str.go = helpers for string operations.
// /helper-struct.go = helpers for struct related operations.
// /helper-time.go = helpers for time related operations.
// /helper-uuid.go = helpers for generating globally unique ids.
package helper

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

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const (
	maxSafeFloatToInt      = float64(math.MaxInt)
	minSafeFloatToInt      = float64(math.MinInt)
	maxSafeFloatToIntCents = maxSafeFloatToInt / 100.0
	minSafeFloatToIntCents = minSafeFloatToInt / 100.0
)

// Itoa converts integer into string.
func Itoa(i int) string {
	return strconv.Itoa(i)
}

// ItoaZeroBlank converts integer into string, if integer is zero, blank is returned.
func ItoaZeroBlank(i int) string {
	if i == 0 {
		return ""
	} else {
		return strconv.Itoa(i)
	}
}

// Atoi converts string to integer.
func Atoi(s string) int {
	i, err := strconv.Atoi(s)

	if err != nil {
		return 0
	}

	return i
}

// Btoa converts bool to string, true = "true", false = "false".
func Btoa(b bool) string {
	if b == true {
		return "true"
	}

	return "false"
}

// Atob converts string to bool.
// string values with first char lower cased "t", "y", "1" treated as true, otherwise false.
func Atob(s string) bool {
	trimmed := strings.TrimSpace(s)
	if LenTrim(trimmed) == 0 {
		return false
	}

	switch strings.ToLower(trimmed[:1]) {
	case "t", "y", "1":
		return true
	case "f", "n", "0":
		return false
	}

	return false
}

// IntPtr int pointer from int value.
func IntPtr(i int) *int {
	return &i
}

// IntVal gets int value from int pointer.
// if pointer is nil, zero is returned.
func IntVal(i *int) int {
	if i == nil {
		return 0
	} else {
		return *i
	}
}

// Int32PtrToInt returns 0 if nil, otherwise actual int value.
func Int32PtrToInt(n *int32) int {
	if n == nil {
		return 0
	}
	return int(*n)
}

// UintToStr converts from uint to string.
func UintToStr(i uint) string {
	return fmt.Sprintf("%d", i)
}

// UintPtr gets uint pointer from uint value.
func UintPtr(i uint) *uint {
	return &i
}

// UintVal gets uint value from uint pointer, if uint pointer is nil, 0 is returned.
func UintVal(i *uint) uint {
	if i == nil {
		return 0
	} else {
		return *i
	}
}

// StrToUint converts from string to uint, if string is not a valid uint, 0 is returned.
func StrToUint(s string) uint {
	if v, e := strconv.ParseUint(s, 10, 0); e != nil {
		return 0
	} else {
		return uint(v)
	}
}

// Int64Ptr gets int64 pointer from int64 value.
func Int64Ptr(i int64) *int64 {
	return &i
}

// Int64Val gets int64 value from int64 pointer, if pointer is nil, 0 is returned.
func Int64Val(i *int64) int64 {
	if i == nil {
		return 0
	} else {
		return *i
	}
}

// Int64ToString converts int64 into string value.
func Int64ToString(n int64) string {
	return strconv.FormatInt(n, 10)
}

// Int64PtrToInt64 returns 0 if pointer is nil, otherwise actual int64 value.
func Int64PtrToInt64(n *int64) int64 {
	if n == nil {
		return 0
	} else {
		return *n
	}
}

// UInt64ToString converts uint64 value to string.
func UInt64ToString(n uint64) string {
	return strconv.FormatUint(n, 10)
}

// StrToUint64 converts from string to uint64, if string is not a valid uint64, 0 is returned.
func StrToUint64(s string) uint64 {
	if v, e := strconv.ParseUint(s, 10, 64); e != nil {
		return 0
	} else {
		return v
	}
}

// StrToInt64 converts from string to int64, if string is not a valid int64, 0 is returned.
func StrToInt64(s string) int64 {
	if v, e := strconv.ParseInt(s, 10, 64); e != nil {
		return 0
	} else {
		return v
	}
}

// Float32Ptr gets float32 pointer from float32 value.
func Float32Ptr(f float32) *float32 {
	return &f
}

// Float32ToString converts float32 value to string.
func Float32ToString(f float32) string {
	return strconv.FormatFloat(float64(f), 'g', -1, 32)
}

// Float32ToStringCents converts float32 value into string representing cent values, 2.12 returned as "212".
// Since float32 can not be precisely calculated in some cases, use math.Round returns the nearest integer
func Float32ToStringCents(f float32) string {
	// guard against NaN/Inf to avoid invalid int conversion
	if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
		return ""
	}
	if math.Abs(float64(f)) > maxSafeFloatToIntCents {
		return ""
	}

	centsInt := int(math.Round(math.Abs(float64(f) * 100)))
	if f >= 0.00 {
		return strings.ReplaceAll(PadLeft(Itoa(centsInt), 3), " ", "0")
	} else {
		return "-" + strings.ReplaceAll(PadLeft(Itoa(centsInt), 3), " ", "0")
	}
}

// Float64ToIntCents converts float64 value into int, representing cent values, 2.12 returned as 212.
// Since float64 can not be precisely calculated in some cases, use math.Round returns the nearest integer
func Float64ToIntCents(f float64) int {
	// guard against NaN/Inf to avoid invalid int conversion
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	if f > maxSafeFloatToIntCents || f < minSafeFloatToIntCents {
		return 0
	}
	return int(math.Round(f * 100))
}

// Float32PtrToFloat32 returns 0 if pointer is nil, otherwise actual float32 value.
func Float32PtrToFloat32(f *float32) float32 {
	if f == nil {
		return 0.00
	} else {
		return *f
	}
}

// Float64Ptr gets float64 pointer from float64 value.
func Float64Ptr(f float64) *float64 {
	return &f
}

// Float64Val gets float64 value from float64 pointer, if pointer is nil, 0 is returned.
func Float64Val(f *float64) float64 {
	if f == nil {
		return 0
	} else {
		return *f
	}
}

// Float64ToInt converts from float64 into int, if conversion fails, 0 is returned.
func Float64ToInt(f float64) int {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	if f > maxSafeFloatToInt || f < minSafeFloatToInt {
		return 0
	}
	return int(math.Round(f))
}

// CentsToFloat64 converts int (cents) into float64 value with two decimal value.
func CentsToFloat64(i int) float64 {
	f, _ := ParseFloat64(fmt.Sprintf("%.2f", float64(i)*0.01))
	return f
}

// FloatToString converts float64 value into string value.
func FloatToString(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// Float64ToString converts float64 value into string value.
func Float64ToString(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// Float64PtrToFloat64 returns 0 if nil, otherwise actual float64 value.
func Float64PtrToFloat64(f *float64) float64 {
	if f == nil {
		return 0.00
	} else {
		return *f
	}
}

// BoolToInt converts bool to int value, true = 1, false = 0.
func BoolToInt(b bool) int {
	if b {
		return 1
	} else {
		return 0
	}
}

// BoolToString converts bool to string value, true = "true", false = "false".
func BoolToString(b bool) string {
	if b {
		return "true"
	} else {
		return "false"
	}
}

// BoolPtrToBool converts bool pointer to bool value.
func BoolPtrToBool(b *bool) bool {
	if b == nil {
		return false
	} else {
		return *b
	}
}

// BoolPtr converts bool value to bool pointer.
func BoolPtr(b bool) *bool {
	return &b
}

// BoolVal gets bool value from bool pointer, if pointer is nil, false is returned.
func BoolVal(b *bool) bool {
	if b == nil {
		return false
	} else {
		return *b
	}
}

// DatePtrToString formats pointer time.Time to string date format yyyy-mm-dd.
func DatePtrToString(t *time.Time) string {
	if t == nil {
		return ""
	}

	return FormatDate(*t)
}

// DateTimePtrToString formats pointer time.Time to string date time format yyyy-mm-dd hh:mm:ss AM/PM.
func DateTimePtrToString(t *time.Time) string {
	if t == nil {
		return ""
	}

	return FormatDateTime(*t)
}

// DateTimePtrToDateTime formats pointer time.Time to time.Time struct.
func DateTimePtrToDateTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	} else {
		return *t
	}
}

// TimePtr casts Time struct to Time pointer.
func TimePtr(t time.Time) *time.Time {
	return &t
}

// TimeVal gets Time struct from Time pointer, if pointer is nil, a time.Time{} is returned.
func TimeVal(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	} else {
		return *t
	}
}

// DurationPtr casts Duration to Duration pointer.
func DurationPtr(d time.Duration) *time.Duration {
	return &d
}

// DurationVal gets Duration value from Duration pointer, if pointer is nil, 0 is returned.
func DurationVal(d *time.Duration) time.Duration {
	if d == nil {
		return 0
	} else {
		return *d
	}
}

// StringPtr gets string pointer from string value.
func StringPtr(s string) *string {
	return &s
}

// StringVal gets string value from string pointer, if pointer is nil, blank string is returned.
func StringVal(s *string) string {
	if s == nil {
		return ""
	} else {
		return *s
	}
}

// StringPtrToString gets string value from string pointer, if pointer is nil, blank string is returned.
func StringPtrToString(s *string) string {
	if s == nil {
		return ""
	} else {
		return *s
	}
}

// SliceObjectsToSliceInterface converts slice of objects into slice of interfaces.
// objectsSlice is received via interface parameter, and is expected to be a Slice,
// the slice is enumerated to convert each object within the slice to interface{},
// the final converted slice of interface is returned, if operation failed, nil is returned.
func SliceObjectsToSliceInterface(objectsSlice interface{}) (output []interface{}) {
	if objectsSlice == nil {
		return nil
	}

	if reflect.TypeOf(objectsSlice).Kind() == reflect.Slice {
		s := reflect.ValueOf(objectsSlice)

		for i := 0; i < s.Len(); i++ {
			output = append(output, s.Index(i).Interface())
		}

		return output
	} else {
		return nil
	}
}

// IntToHex returns HEX string representation of i, in 2 digit blocks.
func IntToHex(i int) string {
	return strings.ToUpper(strconv.FormatInt(int64(i), 16))
}
