package helper

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

import (
	"reflect"
	"strconv"
	"strings"
	"time"
	"fmt"
)

// Itoa converts integer into string
func Itoa(i int) string {
	return strconv.Itoa(i)
}

// Atoi converts string to integer
func Atoi(s string) int {
	i, err := strconv.Atoi(s)

	if err != nil {
		return 0
	}

	return i
}

// Btoa converts bool to string
func Btoa(b bool) string {
	if b == true {
		return "true"
	}

	return "false"
}

// Atob converts string to bool
func Atob(s string) bool {
	if LenTrim(s) == 0 {
		return false
	}

	var b bool
	b = false

	switch strings.ToLower(s[:1]) {
	case "t":
		b = true
	case "y":
		b = true
	case "1":
		b = true
	case "f":
		b = false
	case "n":
		b=false
	case "0":
		b=false
	}

	return b
}

// IntPtr casts int to int pointer
func IntPtr(i int) *int {
	return &i
}

// Int32PtrToInt returns 0 if nil, otherwise actual int value
func Int32PtrToInt(n *int) int {
	if n == nil {
		return 0
	} else {
		return *n
	}
}

// UintToStr converts from uint to string
func UintToStr(i uint) string {
	return fmt.Sprintf("%d", i)
}

// StrToUint converts from string to uint
func StrToUint(s string) uint {
	if v, e := strconv.ParseUint(s, 10, 32); e != nil {
		return 0
	} else {
		return uint(v)
	}
}

// Int64Ptr casts int64 to int64 pointer
func Int64Ptr(i int64) *int64 {
	return &i
}

// Int64ToString converts int64 into string value
func Int64ToString(n int64) string {
	return strconv.FormatInt(n, 10)
}

// Int64PtrToInt64 returns 0 if nil, otherwise actual int64 value
func Int64PtrToInt64(n *int64) int64 {
	if n == nil {
		return 0
	} else {
		return *n
	}
}

// UInt64ToString converts uint64 into string value
func UInt64ToString(n uint64) string {
	return strconv.FormatUint(n, 10)
}

// Float32Ptr casts float32 to float32 pointer
func Float32Ptr(f float32) *float32 {
	return &f
}

// Float32ToString converts float32 into string value
func Float32ToString(f float32) string {
	return fmt.Sprintf("%f", f)
}

// Float32ToStringCents converts float32 into string representing cent values
func Float32ToStringCents(f float32) string {
	if f >= 0.00 {
		return strings.ReplaceAll(PadLeft(Itoa(int(f * 100)), 3), " ", "0")
	} else {
		return  "-" + strings.ReplaceAll(PadLeft(Itoa(int(f * 100 * -1)), 3), " ", "0")
	}
}

// Float32PtrToFloat32 returns 0 if nil, otherwise actual float32 value
func Float32PtrToFloat32(f *float32) float32 {
	if f == nil {
		return 0.00
	} else {
		return *f
	}
}

// Float64Ptr casts float64 to float64 pointer
func Float64Ptr(f float64) *float64 {
	return &f
}

// Float64ToInt converts from float64 into int
func Float64ToInt(f float64) int {
	if v, b := ParseInt32(FloatToString(f)); !b {
		return 0
	} else {
		return v
	}
}

// FloatToString converts float64 into string value
func FloatToString(f float64) string {
	return fmt.Sprintf("%f", f)
}

// Float64PtrToFloat64 returns 0 if nil, otherwise actual float64 value
func Float64PtrToFloat64(f *float64) float64 {
	if f == nil {
		return 0.00
	} else {
		return *f
	}
}

// BoolToInt converts bool to int value
func BoolToInt(b bool) int {
	if b {
		return 1
	} else {
		return 0
	}
}

// BoolPtrToBool converts bool pointer to bool value
func BoolPtrToBool(b *bool) bool {
	if b == nil {
		return false
	} else {
		return *b
	}
}

// BoolPtr converts bool to *bool
func BoolPtr(b bool) *bool {
	return &b
}

// DatePtrToString formats pointer time.Time to string date format
func DatePtrToString(t *time.Time) string {
	if t == nil {
		return ""
	}

	return FormatDate(*t)
}

// DateTimePtrToString formats pointer time.Time to string date time format
func DateTimePtrToString(t *time.Time) string {
	if t == nil {
		return ""
	}

	return FormatDateTime(*t)
}

// DateTimePtrToDateTime formats pointer time.Time to time.Time struct
func DateTimePtrToDateTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	} else {
		return *t
	}
}

// TimePtr casts Time to Time pointer
func TimePtr(t time.Time) *time.Time {
	return &t
}

// DurationPtr casts Duration to Duration pointer
func DurationPtr(d time.Duration) *time.Duration {
	return &d
}

// StringPtr casts string to string pointer
func StringPtr(s string) *string {
	return &s
}

// StringPtrToString gets string value from string pointer
func StringPtrToString(s *string) string {
	if s == nil {
		return ""
	} else {
		return *s
	}
}

// SliceObjectsToSliceInterfaces will convert slice of objects into slice of interfaces
func SliceObjectsToSliceInterface(objectsSlice interface{}) (output []interface{}) {
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


