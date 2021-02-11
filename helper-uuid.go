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
	"database/sql"
	"github.com/google/uuid"
	"github.com/oklog/ulid"
	"time"
	"math/rand"
)

// ================================================================================================================
// UUID HELPERS
// ================================================================================================================

// GenerateUUIDv4 will generate a UUID Version 4 (Random) to represent a globally unique identifier (extremely rare chance of collision)
func GenerateUUIDv4() (string, error) {
	id, err := uuid.NewRandom()

	if err != nil {
		// error
		return "", err
	} else {
		// has id
		return id.String(), nil
	}
}

// NewUUID will generate a UUID Version 4 (Random) and ignore error if any
func NewUUID() string {
	id, _ := GenerateUUIDv4()
	return id
}

// ================================================================================================================
// ULID HELPERS
// ================================================================================================================

// GenerateULID will generate a ULID that is globally unique (very slim chance of collision)
func GenerateULID() (string, error) {
	t := time.Now()
	entropy := ulid.Monotonic(rand.New(rand.NewSource(t.UnixNano())), 0)

	id, err := ulid.New(ulid.Timestamp(t), entropy)

	if err != nil {
		// error
		return "", err
	} else {
		// has id
		return id.String(), nil
	}
}

// NewULID will generate a new ULID and ignore error if any
func NewULID() string {
	id, _ := GenerateULID()
	return id
}

// ================================================================================================================
// Random Number Generator
// ================================================================================================================

// GenerateRandomNumber with unix nano as seed
func GenerateRandomNumber(maxNumber int) int {
	seed := rand.NewSource(time.Now().UnixNano())
	r := rand.New(seed)

	return r.Intn(maxNumber)
}

// GenerateRandomChar will create a random character, using unix nano as seed
func GenerateRandomChar() string {
	r := GenerateRandomNumber(3)

	// valid range of ascii
	// 		33 - 126

	// half the r until within range
	// if not within min, double it until within range
	if r <= 0 {
		attempts := 0

		for {
			if r = GenerateRandomNumber(3); r > 0 {
				break
			} else {
				if attempts > 25 {
					return "~"
				}
			}

			attempts++
		}
	}

	if r < 33 {
		for {
			if r < 33 {
				r *= 2
			} else {
				break
			}
		}
	}

	if r > 126 {
		for {
			if r > 126 {
				r /= 2
			} else {
				break
			}
		}
	}

	// convert decimal ascii to char
	return string(r)
}

// GenerateNewUniqueInt32 will take in old value and return new unique value with randomized seed and negated
func GenerateNewUniqueInt32(oldIntVal int) int {
	seed1 := GenerateRandomNumber(999)
	seed2 := GenerateRandomNumber(99)

	buf := Right(Itoa(oldIntVal), 5) + Padding(Itoa(seed2), 2, false, "0") + Padding(Itoa(seed1),3, false, "0")

	val, ok := ParseInt32(buf)

	if !ok {
		return oldIntVal*-1
	} else {
		return val*-1
	}
}

// GenerateNewUniqueNullInt32 will take in old value and return new unique value with randomized seed and negated
func GenerateNewUniqueNullInt32(oldIntVal sql.NullInt32) sql.NullInt32 {
	if !oldIntVal.Valid {
		return oldIntVal
	}

	seed1 := GenerateRandomNumber(999)
	seed2 := GenerateRandomNumber(99)

	buf := Right(Itoa(FromNullInt(oldIntVal)), 5) + Padding(Itoa(seed2), 2, false, "0") + Padding(Itoa(seed1),3, false, "0")

	val, ok := ParseInt32(buf)

	if !ok {
		return ToNullInt(int(oldIntVal.Int32) *-1, true)
	} else {
		return ToNullInt(val *-1, true)
	}
}

// GenerateNewUniqueInt64 will take in old value and return new unique value with randomized seed and negated
func GenerateNewUniqueInt64(oldIntVal int64) int64 {
	seed1 := GenerateRandomNumber(999)
	seed2 := GenerateRandomNumber(999)

	buf := Right(Int64ToString(oldIntVal), 13) + Padding(Itoa(seed2), 3, false, "0") + Padding(Itoa(seed1),3, false, "0")

	val, ok := ParseInt64(buf)

	if !ok {
		return oldIntVal*-1
	} else {
		return val*-1
	}
}

// GenerateNewUniqueNullInt64 will take in old value and return new unique value with randomized seed and negated
func GenerateNewUniqueNullInt64(oldIntVal sql.NullInt64) sql.NullInt64 {
	if !oldIntVal.Valid {
		return oldIntVal
	}

	seed1 := GenerateRandomNumber(999)
	seed2 := GenerateRandomNumber(999)

	buf := Right(Int64ToString(FromNullInt64(oldIntVal)), 13) + Padding(Itoa(seed2), 3, false, "0") + Padding(Itoa(seed1),3, false, "0")

	val, ok := ParseInt64(buf)

	if !ok {
		return ToNullInt64(oldIntVal.Int64*-1, true)
	} else {
		return ToNullInt64(val*-1, true)
	}
}

// ================================================================================================================
// String Randomizer
// ================================================================================================================

// GenerateNewUniqueString will take in old value and return new unique value with randomized seed
// 		stringLimit = 0 no limit, > 0 has limit
func GenerateNewUniqueString(oldStrVal string, stringLimit int) string {
	seed1 := Padding(Itoa(GenerateRandomNumber(999)), 3, false, "0")
	seed2 := GenerateRandomChar()
	seed3 := GenerateRandomChar()
	seed4 := GenerateRandomChar()

	buf := oldStrVal + seed2 + seed3 + seed4 + seed1

	if stringLimit > 0 {
		if stringLimit >= 6 {
			buf = Right(buf, stringLimit)
		} else {
			if stringLimit >= 3 {
				buf = Left(seed2 + seed3 + seed4 + seed1, stringLimit)
			} else {
				buf = Left(seed2 + seed3, stringLimit)
			}
		}
	}

	return buf
}

// GenerateNewUniqueNullString will take in old value and return new unique value with randomized seed
// 		stringLimit = 0 no limit, > 0 has limit
func GenerateNewUniqueNullString(oldStrVal sql.NullString, stringLimit int) sql.NullString {
	if !oldStrVal.Valid {
		return oldStrVal
	}

	seed1 := Padding(Itoa(GenerateRandomNumber(999)), 3, false, "0")
	seed2 := GenerateRandomChar()
	seed3 := GenerateRandomChar()
	seed4 := GenerateRandomChar()

	buf := FromNullString(oldStrVal) + seed2 + seed3 + seed4 + seed1

	if stringLimit > 0 {
		if stringLimit >= 6 {
			buf = Right(buf, stringLimit)
		} else {
			if stringLimit >= 3 {
				buf = Left(seed2 + seed3 + seed4 + seed1, stringLimit)
			} else {
				buf = Left(seed2 + seed3, stringLimit)
			}
		}
	}

	return ToNullString(buf, true)
}
