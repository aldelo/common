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
	"database/sql"
	"math"
	"time"
)

// FromNullString casts sql null string variable to string variable, if null, blank string is returned
func FromNullString(s sql.NullString) string {
	if !s.Valid {
		return ""
	}

	return s.String
}

// ToNullString sets string value into NullString output
func ToNullString(s string, emptyAsNull bool) sql.NullString {
	if emptyAsNull == true {
		if LenTrim(s) > 0 {
			return sql.NullString{Valid: true, String: s}
		}

		return sql.NullString{Valid: false, String: ""}
	}

	// if not emptyAsNull
	return sql.NullString{Valid: true, String: s}
}

// LenTrimNullString returns string length
func LenTrimNullString(s sql.NullString) int {
	if !s.Valid {
		return 0
	}

	return LenTrim(s.String)
}

// FromNullInt64 casts sql null int64 variable to int64 variable, if null, 0 is returned
func FromNullInt64(d sql.NullInt64) int64 {
	if !d.Valid {
		return 0
	}

	return d.Int64
}

// ToNullInt64 sets int64 value into NullInt64 output
func ToNullInt64(d int64, emptyAsNull bool) sql.NullInt64 {
	if emptyAsNull == true {
		if d == 0 {
			return sql.NullInt64{Valid: false, Int64: 0}
		}

		return sql.NullInt64{Valid: true, Int64: d}
	}

	// not using emptyAsNull
	return sql.NullInt64{Valid: true, Int64: d}
}

// FromNullInt casts sql NullInt32 into int variable, if null, 0 is returned
func FromNullInt(d sql.NullInt32) int {
	if !d.Valid {
		return 0
	}

	return int(d.Int32)
}

// ToNullInt sets int value into NullInt32 output
func ToNullInt(d int, emptyAsNull bool) sql.NullInt32 {
	// guard against int32 overflow to prevent truncated values
	if d > math.MaxInt32 || d < math.MinInt32 {
		return sql.NullInt32{Valid: false, Int32: 0}
	}

	if emptyAsNull == true {
		if d == 0 {
			return sql.NullInt32{Valid: false, Int32: 0}
		}

		return sql.NullInt32{Valid: true, Int32: int32(d)}
	}

	// not using emptyAsNull
	return sql.NullInt32{Valid: true, Int32: int32(d)}
}

// FromNullFloat64 casts sql null float64 variable to float64 variable, if null, 0.00 is returned
func FromNullFloat64(d sql.NullFloat64) float64 {
	if !d.Valid {
		return 0.00
	}

	// sanitize NaN/Inf coming from the DB to avoid propagating invalid floats
	if math.IsNaN(d.Float64) || math.IsInf(d.Float64, 0) {
		return 0.00
	}

	return d.Float64
}

// ToNullFloat64 sets float64 into NullFloat64 output
func ToNullFloat64(d float64, emptyAsNull bool) sql.NullFloat64 {
	// treat NaN/Inf as null to avoid DB driver errors
	if math.IsNaN(d) || math.IsInf(d, 0) {
		return sql.NullFloat64{Valid: false, Float64: 0}
	}

	if emptyAsNull == true {
		if d == 0.00 {
			return sql.NullFloat64{Valid: false, Float64: 0.00}
		}

		return sql.NullFloat64{Valid: true, Float64: d}
	}

	// not using emptyAsNull
	return sql.NullFloat64{Valid: true, Float64: d}
}

// FromNullFloat32 casts sql null float64 into float32 variable
func FromNullFloat32(d sql.NullFloat64) float32 {
	return float32(FromNullFloat64(d))
}

// ToNullFloat32 sets float32 into NullFloat64 output
func ToNullFloat32(d float32, emptyAsNull bool) sql.NullFloat64 {
	return ToNullFloat64(float64(d), emptyAsNull)
}

// FromNullBool casts sql null bool variable to bool variable, if null, false is returned
func FromNullBool(b sql.NullBool) bool {
	if !b.Valid {
		return false
	}

	return b.Bool
}

// ToNullBoolWithEmpty sets bool into NullBool output, optionally treating false as NULL
// new helper to allow generating a NULL bool when desired
func ToNullBoolWithEmpty(b bool, emptyAsNull bool) sql.NullBool {
	if emptyAsNull && b == false {
		return sql.NullBool{Valid: false, Bool: false}
	}

	return sql.NullBool{Valid: true, Bool: b}
}

// ToNullBool sets bool into NullBool output
func ToNullBool(b bool) sql.NullBool {
	return ToNullBoolWithEmpty(b, false)
}

// FromNullTime parses string into time.Time
func FromNullTime(t sql.NullTime) time.Time {
	if t.Valid == false {
		return time.Time{}
	}

	return t.Time
}

// ToNullTime sets time.Time into NullTime output
func ToNullTime(t time.Time) sql.NullTime {
	if t.IsZero() == true {
		return sql.NullTime{Valid: false, Time: time.Time{}}
	}

	return sql.NullTime{Valid: true, Time: t}
}
