package helper

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

import (
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------------------------------------------------
// custom time structs
// ---------------------------------------------------------------------------------------------------------------------

// JsonTime provides custom Json marshal and unmarshal interface implementations.
// JsonTime marshals and unmarshal using RFC3339 time format
type JsonTime time.Time

// MarshalJSON marshals time value to format of RFC3339
func (jt *JsonTime) MarshalJSON() ([]byte, error) {
	if jt != nil {
		t := time.Time(*jt)
		buf := fmt.Sprintf(`"%s"`, t.Format(time.RFC3339))
		return []byte(buf), nil
	} else {
		return []byte{}, fmt.Errorf("JsonTime Nil")
	}
}

// UnmarshalJSON unmarshal time value in format of RFC3339 to JsonTime
func (jt *JsonTime) UnmarshalJSON(b []byte) error {
	if jt == nil {
		return fmt.Errorf("JsonTime Nil")
	}

	buf := strings.Trim(string(b), `"`)
	if t, e := time.Parse(time.RFC3339, buf); e != nil {
		return e
	} else {
		*jt = JsonTime(t)
		return nil
	}
}

// ToTime converts JsonTime to time.Time
func (jt *JsonTime) ToTime() time.Time {
	if jt == nil {
		return time.Time{}
	} else {
		return time.Time(*jt)
	}
}

// ToJsonTime converts time.Time to JsonTime
func ToJsonTime(t time.Time) JsonTime {
	return JsonTime(t)
}

// ToJsonTimePtr converts time.Time to JsonTime pointer
func ToJsonTimePtr(t time.Time) *JsonTime {
	jt := JsonTime(t)
	return &jt
}

// ---------------------------------------------------------------------------------------------------------------------
// time helpers
// ---------------------------------------------------------------------------------------------------------------------

// FormatDate will format the input date value to yyyy-mm-dd
func FormatDate(t time.Time, blankIfZero ...bool) string {
	ifZero := false
	if len(blankIfZero) > 0 {
		ifZero = blankIfZero[0]
	}

	if ifZero {
		if t.IsZero() {
			return ""
		}
	}

	return t.Format("2006-01-02")
}

// FormatTime will format the input date value to hh:mm:ss tt
func FormatTime(t time.Time, blankIfZero ...bool) string {
	ifZero := false
	if len(blankIfZero) > 0 {
		ifZero = blankIfZero[0]
	}

	if ifZero {
		if t.IsZero() {
			return ""
		}
	}

	return t.Format("03:04:05 PM")
}

// FormatDateTime will format the input date value to yyyy-mm-dd hh:mm:ss tt
func FormatDateTime(t time.Time, blankIfZero ...bool) string {
	ifZero := false
	if len(blankIfZero) > 0 {
		ifZero = blankIfZero[0]
	}

	if ifZero {
		if t.IsZero() {
			return ""
		}
	}

	return t.Format("2006-01-02 03:04:05 PM")
}

// DateFormatString returns the date format string constant (yyyy-mm-dd)
func DateFormatString() string {
	return "2006-01-02"
}

// TimeFormatString returns the time format string constant (hh:mm:ss tt)
func TimeFormatString() string {
	return "03:04:05 PM"
}

// DateTimeFormatString returns the date time format string constant (yyyy-mm-dd hh:mm:ss tt)
func DateTimeFormatString() string {
	return "2006-01-02 03:04:05 PM"
}

// ParseDate will parse a date value in yyyy-mm-dd format into time.Time object,
// check time.IsZero() to verify if a zero time is returned indicating parser failure
func ParseDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))

	if err != nil {
		return time.Time{}
	}

	return t
}

// ParseTime will parse a time vaule in hh:mm:ss tt format into time.Time object,
// check time.IsZero() to verify if a zero time is returned indicating parser failure
func ParseTime(s string) time.Time {
	t, err := time.Parse("03:04:05 PM", strings.TrimSpace(s))

	if err != nil {
		return time.Time{}
	}

	return t
}

// ParseTimeFromhhmmss will parse a time value from hhmmss format into time.Time object,
// if parse failed, time.Time{} is returned (use time.IsZero() to check if parse success)
func ParseTimeFromhhmmss(s string) time.Time {
	s = strings.TrimSpace(s)

	if IsNumericIntOnly(s) == false {
		return time.Time{}
	}

	if LenTrim(s) != 6 {
		return time.Time{}
	}

	v := Left(s, 2) + ":" + Mid(s, 2, 2) + ":" + Right(s, 2)
	t, err := time.Parse("15:04:05", v)

	if err != nil {
		return time.Time{}
	}

	return t
}

// ParseDateTime will parse a date time value in yyyy-mm-dd hh:mm:ss tt format into time.Time object,
// check time.IsZero() to verify if a zero time is returned indicating parser failure
func ParseDateTime(s string) time.Time {
	t, err := time.Parse("2006-01-02 03:04:05 PM", strings.TrimSpace(s))

	if err != nil {
		return time.Time{}
	}

	return t
}

// ParseFromExcelDate will handle integer value of excel date to convert to time.Time
func ParseFromExcelDate(s string, format string) time.Time {
	i, _ := ParseInt32(s)

	if i > 0 {
		v := GetDate(1970, 1, 1)
		return v.AddDate(0, 0, i-70*365-19)
	} else {
		return ParseDateTimeCustom(s, format)
	}
}

// ParseDateTime24Hr will parse a date time value in yyyy-mm-dd HH:mm:ss format into time.Time object,
// check time.IsZero() to verify if a zero time is returned indicating parser failure
func ParseDateTime24Hr(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(s))

	if err != nil {
		return time.Time{}
	}

	return t
}

// ParseDateTimeCustom will parse a date time value in s string, based on the f format
// f format is 2006 01 02 15:04:05 / 03:04:05 PM
func ParseDateTimeCustom(s string, f string) time.Time {
	t, err := time.Parse(f, strings.TrimSpace(s))

	if err != nil {
		return time.Time{}
	}

	return t
}

// ParseDateTimeFromYYYYMMDDhhmmss from string value
func ParseDateTimeFromYYYYMMDDhhmmss(s string) time.Time {
	s = strings.TrimSpace(s)

	if IsNumericIntOnly(s) == false {
		return time.Time{}
	}

	if LenTrim(s) != 14 {
		return time.Time{}
	}

	d := Left(s, 4) + "-" + Mid(s, 4, 2) + "-" + Mid(s, 6, 2)
	t := Mid(s, 8, 2) + ":" + Mid(s, 10, 2) + ":" + Mid(s, 12, 2)

	dv := d + " " + t

	return ParseDateTime24Hr(dv)
}

// ParseDateTimeFromMMDDYYYYhhmmss from string value
func ParseDateTimeFromMMDDYYYYhhmmss(s string) time.Time {
	s = strings.TrimSpace(s)

	if IsNumericIntOnly(s) == false {
		return time.Time{}
	}

	if LenTrim(s) != 14 {
		return time.Time{}
	}

	d := Mid(s, 4, 4) + "-" + Left(s, 2) + "-" + Mid(s, 2, 2)
	t := Mid(s, 8, 2) + ":" + Mid(s, 10, 2) + ":" + Mid(s, 12, 2)

	dv := d + " " + t

	return ParseDateTime24Hr(dv)
}

// ParseDateFromYYYYMMDD from string value
func ParseDateFromYYYYMMDD(s string) time.Time {
	s = strings.TrimSpace(s)

	if IsNumericIntOnly(s) == false {
		return time.Time{}
	}

	if LenTrim(s) != 8 {
		return time.Time{}
	}

	d := Left(s, 4) + "-" + Mid(s, 4, 2) + "-" + Mid(s, 6, 2)

	return ParseDate(d)
}

// ParseDateFromDDMMYYYY from string value
func ParseDateFromDDMMYYYY(s string) time.Time {
	s = strings.TrimSpace(s)

	if IsNumericIntOnly(s) == false {
		return time.Time{}
	}

	if LenTrim(s) != 8 {
		return time.Time{}
	}

	d := Right(s, 4) + "-" + Mid(s, 2, 2) + "-" + Left(s, 2)

	return ParseDate(d)
}

// ParseDateFromYYMMDD from string value
func ParseDateFromYYMMDD(s string) time.Time {
	s = strings.TrimSpace(s)

	if IsNumericIntOnly(s) == false {
		return time.Time{}
	}

	if LenTrim(s) != 6 {
		return time.Time{}
	}

	d := Left(s, 2) + "-" + Mid(s, 2, 2) + "-" + Mid(s, 4, 2)

	return ParseDateTimeCustom(d, "06-01-02")
}

// ParseDateFromYYMM from string value
func ParseDateFromYYMM(s string) time.Time {
	s = strings.TrimSpace(s)

	if IsNumericIntOnly(s) == false {
		return time.Time{}
	}

	if LenTrim(s) != 4 {
		return time.Time{}
	}

	d := Left(s, 2) + "-" + Mid(s, 2, 2)

	return ParseDateTimeCustom(d, "06-01")
}

// ParseDateFromMMYY from string value
func ParseDateFromMMYY(s string) time.Time {
	s = strings.TrimSpace(s)

	if IsNumericIntOnly(s) == false {
		return time.Time{}
	}

	if LenTrim(s) != 4 {
		return time.Time{}
	}

	d := Left(s, 2) + "-" + Mid(s, 2, 2)

	return ParseDateTimeCustom(d, "01-06")
}

// ParseDateToLastDayOfMonth takes in a time.Time struct and returns the last date of month
func ParseDateToLastDayOfMonth(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}

	newDate := t.AddDate(0, 1, 0)

	y, m, _ := newDate.Date()

	newDate = ParseDateFromYYYYMMDD(Padding(Itoa(y), 4, false, "0") + Padding(Itoa(int(m)), 2, false, "0") + "01")

	newDate = newDate.AddDate(0, 0, -1)

	return newDate
}

// ParseDateFromMMDD from string value
func ParseDateFromMMDD(s string) time.Time {
	s = strings.TrimSpace(s)

	if IsNumericIntOnly(s) == false {
		return time.Time{}
	}

	if LenTrim(s) != 4 {
		return time.Time{}
	}

	d := Left(s, 2) + "-" + Mid(s, 2, 2)

	return ParseDateTimeCustom(d, "01-02")
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

// IsDateValidYYYYMMDD checks if input string value is a valid date represented in the format of YYYYMMDD
// valid year detected is 1970 - 2099
func IsDateValidYYYYMMDD(s string) bool {
	s = Trim(s)

	if len(s) != 8 {
		return false
	}

	yyyy := 0
	mm := 0
	dd := 0

	if yyyy = Atoi(Left(s, 4)); yyyy < 1970 || yyyy > 2099 {
		return false
	}

	if mm = Atoi(Mid(s, 4, 2)); mm < 1 || mm > 12 {
		return false
	}

	if dd = Atoi(Right(s, 2)); dd < 1 || dd > 31 {
		return false
	}

	if !IsDayOfMonthValid(yyyy, mm, dd) {
		return false
	}

	return true
}

// IsDateValidYYMMDD checks if input string value is a valid date represented in the format of YYMMDD
// valid year detected is 00 - 99, with year 20xx assumed
func IsDateValidYYMMDD(s string) bool {
	s = Trim(s)

	if len(s) != 6 {
		return false
	}

	yy := 0
	mm := 0
	dd := 0

	if yy = Atoi(Left(s, 2)); yy < 0 || yy > 99 {
		return false
	}

	if mm = Atoi(Mid(s, 2, 2)); mm < 1 || mm > 12 {
		return false
	}

	if dd = Atoi(Right(s, 2)); dd < 1 || dd > 31 {
		return false
	}

	if !IsDayOfMonthValid(2000 + yy, mm, dd) {
		return false
	}

	return true
}

// IsDateValidYYYYMM checks if input string value is a valid date represented in the format of YYYYMM
// valid year detected is 1970 - 2099
func IsDateValidYYYYMM(s string) bool {
	s = Trim(s)

	if len(s) != 6 {
		return false
	}

	if yyyy := Atoi(Left(s, 4)); yyyy < 1970 || yyyy > 2099 {
		return false
	}

	if mm := Atoi(Right(s, 2)); mm < 1 || mm > 12 {
		return false
	}

	return true
}

// IsDateValidYYMM checks if input string value is a valid date represented in the format of YYMM
// valid year detected is 00 - 99, with year 20xx assumed
func IsDateValidYYMM(s string) bool {
	s = Trim(s)

	if len(s) != 4 {
		return false
	}

	if yy := Atoi(Left(s, 2)); yy < 0 || yy > 99 {
		return false
	}

	if mm := Atoi(Right(s, 2)); mm < 1 || mm > 12 {
		return false
	}

	return true
}

// IsDateValidMMDDYYYY checks if input string value is a valid date represented in the format of MMDDYYYY
// valid year detected is 1970 - 2099
func IsDateValidMMDDYYYY(s string) bool {
	s = Trim(s)

	if len(s) != 8 {
		return false
	}

	mm := 0
	dd := 0
	yyyy := 0

	if mm = Atoi(Left(s, 2)); mm < 1 || mm > 12 {
		return false
	}

	if dd = Atoi(Mid(s, 2, 2)); dd < 1 || dd > 31 {
		return false
	}

	if yyyy = Atoi(Right(s, 4)); yyyy < 1970 || yyyy > 2099 {
		return false
	}

	if !IsDayOfMonthValid(yyyy, mm, dd) {
		return false
	}

	return true
}

// IsDateValidMMDDYY checks if input string value is a valid date represented in the format of MMDDYY
// valid year detected is 1970 - 2099
func IsDateValidMMDDYY(s string) bool {
	s = Trim(s)

	if len(s) != 6 {
		return false
	}

	mm := 0
	dd := 0
	yy := 0

	if mm = Atoi(Left(s, 2)); mm < 1 || mm > 12 {
		return false
	}

	if dd = Atoi(Mid(s, 2, 2)); dd < 1 || dd > 31 {
		return false
	}

	if yy = Atoi(Right(s, 2)); yy < 0 || yy > 99 {
		return false
	}

	if !IsDayOfMonthValid(2000 + yy, mm, dd) {
		return false
	}

	return true
}

// IsDateValidMMYYYY checks if input string value is a valid date represented in the format of MMYYYY
// valid year detected is 1970 - 2099
func IsDateValidMMYYYY(s string) bool {
	s = Trim(s)

	if len(s) != 6 {
		return false
	}

	if mm := Atoi(Left(s, 2)); mm < 1 || mm > 12 {
		return false
	}

	if yyyy := Atoi(Right(s, 4)); yyyy < 1970 || yyyy > 2099 {
		return false
	}

	return true
}

// IsDateValidMMYY checks if input string value is a valid date represented in the format of MMYY
// valid year detected is 00-99 with year 20xx assumed
func IsDateValidMMYY(s string) bool {
	s = Trim(s)

	if len(s) != 4 {
		return false
	}

	if mm := Atoi(Left(s, 2)); mm < 1 || mm > 12 {
		return false
	}

	if yy := Atoi(Right(s, 2)); yy < 0 || yy > 99 {
		return false
	}

	return true
}

// IsTimeValidhhmmss checks if input string value is a valid time represented in the format of hhmmss (24 hour format)
func IsTimeValidhhmmss(s string) bool {
	s = Trim(s)

	if len(s) != 6 {
		return false
	}

	if hh := Atoi(Left(s, 2)); hh < 0 || hh > 23 {
		return false
	}

	if mm := Atoi(Mid(s, 2, 2)); mm < 0 || mm > 59 {
		return false
	}

	if ss := Atoi(Right(s, 2)); ss < 0 || ss > 59 {
		return false
	}

	return true
}

// IsTimeValidhhmm checks if input string value is a valid time represented in the format of hhmm (24 hour format)
func IsTimeValidhhmm(s string) bool {
	s = Trim(s)

	if len(s) != 4 {
		return false
	}

	if hh := Atoi(Left(s, 2)); hh < 0 || hh > 23 {
		return false
	}

	if mm := Atoi(Right(s, 2)); mm < 0 || mm > 59 {
		return false
	}

	return true
}

// IsDateTimeValidYYYYMMDDhhmmss checks if input string value is a valid date time represented in the format of YYYYMMDDhhmmss (24 hour format)
func IsDateTimeValidYYYYMMDDhhmmss(s string) bool {
	s = Trim(s)

	if len(s) != 14 {
		return false
	}

	if d := Left(s, 8); !IsDateValidYYYYMMDD(d) {
		return false
	}

	if t := Right(s, 6); !IsTimeValidhhmmss(t) {
		return false
	}

	return true
}

// IsDateTimeValidYYYYMMDDhhmm checks if input string value is a valid date time represented in the format of YYYYMMDDhhmm (24 hour format)
func IsDateTimeValidYYYYMMDDhhmm(s string) bool {
	s = Trim(s)

	if len(s) != 12 {
		return false
	}

	if d := Left(s, 8); !IsDateValidYYYYMMDD(d) {
		return false
	}

	if t := Right(s, 4); !IsTimeValidhhmm(t) {
		return false
	}

	return true
}

// IsDateTimeValidYYMMDDhhmmss checks if input string value is a valid date time represented in the format of YYMMDDhhmmss (24 hour format)
func IsDateTimeValidYYMMDDhhmmss(s string) bool {
	s = Trim(s)

	if len(s) != 12 {
		return false
	}

	if d := Left(s, 6); !IsDateValidYYMMDD(d) {
		return false
	}

	if t := Right(s, 6); !IsTimeValidhhmmss(t) {
		return false
	}

	return true
}

// IsDateTimeValidYYMMDDhhmm checks if input string value is a valid date time represented in the format of YYMMDDhhmm (24 hour format)
func IsDateTimeValidYYMMDDhhmm(s string) bool {
	s = Trim(s)

	if len(s) != 10 {
		return false
	}

	if d := Left(s, 6); !IsDateValidYYMMDD(d) {
		return false
	}

	if t := Right(s, 4); !IsTimeValidhhmm(t) {
		return false
	}

	return true
}

// FormatDateTimeToYYYYMMDDhhmmss for the date time struct received
func FormatDateTimeToYYYYMMDDhhmmss(t time.Time) string {
	return t.Format("20060102150405")
}

// FormatDateTimeToMMDDYYYYhhmmss for the date time struct received
func FormatDateTimeToMMDDYYYYhhmmss(t time.Time) string {
	return t.Format("01022006150405")
}

// FormatTimeTohhmmss for the date time struct received
func FormatTimeTohhmmss(t time.Time) string {
	return t.Format("150405")
}

// FormatDateToYYYYMMDD for the date time struct received
func FormatDateToYYYYMMDD(t time.Time) string {
	return t.Format("20060102")
}

// FormatDateToDDMMYYYY for the date time struct received
func FormatDateToDDMMYYYY(t time.Time) string {
	return t.Format("02012006")
}

// FormatDateToYYMMDD for the date time struct received
func FormatDateToYYMMDD(t time.Time) string {
	return t.Format("060102")
}

// FormatDateToYYMM for the date time struct received
func FormatDateToYYMM(t time.Time) string {
	return t.Format("0601")
}

// FormatDateToMMYY for the date time struct received
func FormatDateToMMYY(t time.Time) string {
	return t.Format("0106")
}

// FormatDateToMMDD for the date time struct received
func FormatDateToMMDD(t time.Time) string {
	return t.Format("0102")
}

// GetDate returns date based on given year month day,
// month max day is checked,
// leap year is checked
func GetDate(year int, month int, day int) time.Time {
	if year < 1970 || year > 2199 {
		return time.Time{}
	}

	if month < 1 || month > 12 {
		return time.Time{}
	}

	if day < 1 || day > 31 {
		return time.Time{}
	}

	x := []int{4, 6, 9, 11}

	if IntSliceContains(&x, month) {
		// 30
		if day == 31 {
			return time.Time{}
		}
	} else if month == 2 {
		// either 28 or 29
		ly := 28

		if IsLeapYear(year) {
			ly = 29
		}

		if day > ly {
			return time.Time{}
		}
	}

	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

// GetFirstDateOfMonth returns the given date's first date of month,
// for example, 8/21/2020 => 8/1/2020
func GetFirstDateOfMonth(t time.Time) time.Time {
	return GetDate(t.Year(), int(t.Month()), 1)
}

// GetLastDateOfMonth returns the given date's last day of the month,
// for example, 8/21/2020 => 8/31/2020
func GetLastDateOfMonth(t time.Time) time.Time {
	x := GetFirstDateOfMonth(t).AddDate(0, 1, 0)
	return GetFirstDateOfMonth(x).AddDate(0, 0, -1)
}