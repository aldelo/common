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
	"time"
	"fmt"
)

// FormatDate will format the input date value to yyyy-mm-dd
func FormatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// FormatTime will format the input date value to hh:mm:ss tt
func FormatTime(t time.Time) string {
	return t.Format("03:04:05 PM")
}

// FormatDateTime will format the input date value to yyyy-mm-dd hh:mm:ss tt
func FormatDateTime(t time.Time) string {
	return t.Format("2006-01-02 03:04:05 PM")
}

// ParseDate will parse a date value in yyyy-mm-dd format into time.Time object,
// check time.IsZero() to verify if a zero time is returned indicating parser failure
func ParseDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)

	if err != nil {
		return time.Time{}
	}

	return t
}

// ParseTime will parse a time vaule in hh:mm:ss tt format into time.Time object,
// check time.IsZero() to verify if a zero time is returned indicating parser failure
func ParseTime(s string) time.Time {
	t, err := time.Parse("03:04:05 PM", s)

	if err != nil {
		return time.Time{}
	}

	return t
}

// ParseDateTime will parse a date time value in yyyy-mm-dd hh:mm:ss tt format into time.Time object,
// check time.IsZero() to verify if a zero time is returned indicating parser failure
func ParseDateTime(s string) time.Time {
	t, err := time.Parse("2006-01-02 03:04:05 PM", s)

	if err != nil {
		return time.Time{}
	}

	return t
}

// ParseDateTime24Hr will parse a date time value in yyyy-mm-dd HH:mm:ss format into time.Time object,
// check time.IsZero() to verify if a zero time is returned indicating parser failure
func ParseDateTime24Hr(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", s)

	if err != nil {
		return time.Time{}
	}

	return t
}

// ParseDateTimeCustom will parse a date time value in s string, based on the f format
// f format is 2006 01 02 15:04:05 / 03:04:05 PM
func ParseDateTimeCustom(s string, f string) time.Time {
	t, err := time.Parse(f, s)

	if err != nil {
		return time.Time{}
	}

	return t
}

// CurrentDate returns current date in yyyy-mm-dd format
func CurrentDate() string {
	return time.Now().Format("2006-01-02")
}

// CurrentDateStruct returns current date in yyyy-mm-dd format via time.Time struct
func CurrentDateStruct() time.Time {
	return ParseDate(CurrentDate())
}

// CurrentDateTime returns current date and time in yyyy-mm-dd hh:mm:ss tt format
func CurrentDateTime() string {
	return time.Now().Format("2006-01-02 03:04:05 PM")
}

// CurrentDateTimeStruct returns current date and time in yyyy-mm-dd hh:mm:ss tt format via time.Time struct
func CurrentDateTimeStruct() time.Time {
	return ParseDateTime(CurrentDateTime())
}

// CurrentTime returns current time in hh:mm:ss tt format
func CurrentTime() string {
	s := time.Now().Format("2006-01-02 03:04:05 PM")
	s = s[11:]

	return s
}

// DaysDiff gets the days difference between from and to date
func DaysDiff(timeFrom time.Time, timeTo time.Time) int {
	d := timeTo.Sub(timeFrom)
	dv := d.Hours() / 24.0
	days := int(dv)
	return days
}

// HoursDiff gets the hours difference between from and to date
func HoursDiff(timeFrom time.Time, timeTo time.Time) int {
	d := timeTo.Sub(timeFrom)
	dv := d.Hours()
	hr := int(dv)
	return hr
}

// MinutesDiff gets the minutes difference between from and to date
func MinutesDiff(timeFrom time.Time, timeTo time.Time) int {
	d := timeTo.Sub(timeFrom)
	dv := d.Minutes()
	mn := int(dv)
	return mn
}

// SecondsDiff gets the seconds difference between from and to date
func SecondsDiff(timeFrom time.Time, timeTo time.Time) int {
	d := timeTo.Sub(timeFrom)
	dv := d.Seconds()
	s := int(dv)
	return s
}

// DateBefore checks if testDate is before the beforeDate
func DateBefore(testDate time.Time, beforeDate time.Time) bool {
	if testDate.Before(beforeDate) {
		return true
	}

	return false
}

// DateBeforeOrEqual checks if testDate is before or equal to the beforeEqualDate
func DateBeforeOrEqual(testDate time.Time, beforeEqualDate time.Time) bool {
	if testDate.Equal(beforeEqualDate) {
		return true
	}

	if testDate.Before(beforeEqualDate) {
		return true
	}

	return false
}

// DateAfter checks if testDate is after the afterDate
func DateAfter(testDate time.Time, afterDate time.Time) bool {
	if testDate.After(afterDate) {
		return true
	}

	return false
}

// DateAfterOrEqual checks if testDate is after or equal to the afterEqualDate
func DateAfterOrEqual(testDate time.Time, afterEqualDate time.Time) bool {
	if testDate.Equal(afterEqualDate) {
		return true
	}

	if testDate.After(afterEqualDate) {
		return true
	}

	return false
}

// DateBetween checks if testDate is within the fromDate and toDate,
// if doNotIncludeEqual = true, then testDate equals fromDate and toDate are skipped
func DateBetween(testDate time.Time, fromDate time.Time, toDate time.Time, doNotIncludeEqual bool) bool {
	if doNotIncludeEqual == false {
		if testDate.Equal(fromDate) {
			return true
		}

		if testDate.Equal(toDate) {
			return true
		}
	}

	if testDate.After(fromDate) {
		return true
	}

	if testDate.Before(toDate) {
		return true
	}

	return false
}

// DateOutside checks if the testDate is outside of the fromDate and toDate
func DateOutside(testDate time.Time, fromDate time.Time, toDate time.Time) bool {
	if testDate.Before(fromDate) {
		return true
	}

	if testDate.After(toDate) {
		return true
	}

	return false
}

// DateEqual checks if the testDate equals to the equalDate
func DateEqual(testDate time.Time, equalDate time.Time) bool {
	if testDate.Equal(equalDate) {
		return true
	}

	return false
}

// DateToUTC converts given time to utc
func DateToUTC(t time.Time) (time.Time, error) {
	loc, err := time.LoadLocation("UTC")

	if err != nil {
		return time.Time{}, err
	}

	if loc == nil {
		return time.Time{}, fmt.Errorf("DateToUTC Location Target is Not Retrieved")
	}

	return t.In(loc), nil
}

// DateToUTC2 returns utc value directly without error info
func DateToUTC2(t time.Time) time.Time {
	v, _ := DateToUTC(t)
	return v
}

// DateToLocal converts given time to local time
func DateToLocal(t time.Time) (time.Time, error) {
	loc, err := time.LoadLocation("Local")

	if err != nil {
		return time.Time{}, err
	}

	if loc == nil {
		return time.Time{}, fmt.Errorf("DateToLocal Location Targe is Not Retrieved")
	}

	return t.In(loc), nil
}

// DateToLocal2 returns local value directly without error info
func DateToLocal2(t time.Time) time.Time {
	v, _ := DateToLocal(t)
	return v
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

// IsLeapYear checks if the year input is leap year or not
func IsLeapYear(year int) bool {
	if year % 100 == 0 {
		// is century year, divisible by 400 is leap year
		if year % 400 == 0 {
			return true
		} else {
			return false
		}
	} else {
		// not a century year, divisible by 4 is leap year
		if year % 4 == 0 {
			return true
		} else {
			return false
		}
	}
}

// IsDayOfMonthValid checks if the month day number is valid
func IsDayOfMonthValid(year int, month int, day int) bool {
	switch month {
	case 1:
		fallthrough
	case 3:
		fallthrough
	case 5:
		fallthrough
	case 7:
		fallthrough
	case 8:
		fallthrough
	case 10:
		fallthrough
	case 12:
		if day < 1 || day > 31 {
			return false
		} else {
			return true
		}

	case 4:
		fallthrough
	case 6:
		fallthrough
	case 9:
		fallthrough
	case 11:
		if day < 1 || day > 30 {
			return false
		} else {
			return true
		}

	case 2:
		d := 28

		if IsLeapYear(year) {
			d = 29
		}

		if day < 1 || day > d {
			return false
		} else {
			return true
		}

	default:
		return false
	}
}
