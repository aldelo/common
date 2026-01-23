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
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"math"
	"math/big"
	mrand "math/rand"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
)

var (
	ulidEntropy     = ulid.Monotonic(rand.Reader, 0)
	ulidEntropyLock sync.Mutex

	fallbackRand     = mrand.New(mrand.NewSource(time.Now().UnixNano())) // CHANGED: fallback RNG instance
	fallbackRandLock sync.Mutex
)

// ensure fallback RNG gets a fresh, high-entropy seed when crypto/rand is unavailable
func reseedFallbackRandLocked() {
	var seed int64
	if err := binary.Read(rand.Reader, binary.LittleEndian, &seed); err != nil {
		seed = time.Now().UnixNano()
	}
	fallbackRand.Seed(seed)
}

// helper to safely get Intn with shared RNG
func randomIntn(max int) int {
	if max <= 0 { // defensive guard against panic
		return 0
	}

	// CHANGED: use crypto-strength result when available
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err == nil {
		return int(n.Int64())
	}

	// fallback to non-crypto RNG to avoid deterministic zeros on entropy failure
	fallbackRandLock.Lock()
	reseedFallbackRandLocked()
	v := fallbackRand.Intn(max)
	fallbackRandLock.Unlock()
	return v
}

// crypto-friendly, bounded int32 generator with fallback
func randomInt32n(max int32) int32 {
	if max <= 0 {
		return 0
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err == nil {
		return int32(n.Int64())
	}

	// fallback to non-crypto RNG
	fallbackRandLock.Lock()
	reseedFallbackRandLocked()
	v := int32(fallbackRand.Int31n(max))
	fallbackRandLock.Unlock()
	return v
}

// crypto-friendly, bounded int64 generator with fallback
func randomInt64n(max int64) int64 {
	if max <= 0 {
		return 0
	}

	n, err := rand.Int(rand.Reader, big.NewInt(max))
	if err == nil {
		return n.Int64()
	}

	// fallback to non-crypto RNG
	fallbackRandLock.Lock()
	reseedFallbackRandLocked()
	v := fallbackRand.Int63n(max)
	fallbackRandLock.Unlock()
	return v
}

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

	ulidEntropyLock.Lock()
	defer ulidEntropyLock.Unlock() // ensure unlock on all paths

	id, err := ulid.New(ulid.Timestamp(t), ulidEntropy)
	if err != nil {
		return "", err
	}

	return id.String(), nil
}

// NewULID will generate a new ULID and ignore error if any
func NewULID() string {
	id, _ := GenerateULID()
	return id
}

// GetULIDTimestamp will return the timestamp of the ulid string
func GetULIDTimestamp(ulidStr string) (time.Time, error) {
	if id, err := ulid.Parse(ulidStr); err != nil {
		return time.Time{}, err
	} else {
		return time.UnixMilli(int64(id.Time())), nil
	}
}

// IsULIDValid will check if the ulid string is valid
func IsULIDValid(ulidStr string) bool {
	if _, err := ulid.Parse(ulidStr); err != nil {
		return false
	} else {
		return true
	}
}

// ================================================================================================================
// Random Number Generator
// ================================================================================================================

// GenerateRandomNumber with unix nano as seed
func GenerateRandomNumber(maxNumber int) int {
	// guard to avoid panic when maxNumber <= 0
	if maxNumber <= 0 {
		return 0
	}

	// fast path for degenerate bound
	if maxNumber == 1 {
		return 0
	}

	return randomIntn(maxNumber)
}

// GenerateRandomChar will create a random character, using unix nano as seed
func GenerateRandomChar() string {
	const (
		printableStart = 33
		printableRange = 94 // 126 - 33 + 1
	)

	r := randomIntn(printableRange) + printableStart
	return string(r)
}

// GenerateNewUniqueInt32 will take in old value and return new unique value with randomized seed and negated
func GenerateNewUniqueInt32(oldIntVal int) int {
	seed1 := GenerateRandomNumber(999)
	seed2 := GenerateRandomNumber(99)

	buf := Right(Itoa(oldIntVal), 5) + Padding(Itoa(seed2), 2, false, "0") + Padding(Itoa(seed1), 3, false, "0")

	val, ok := ParseInt32(buf)

	if !ok {
		return safeNegateInt(int(randomInt32n(math.MaxInt32)))
	} else {
		return safeNegateInt(val) // avoid overflow
	}
}

// GenerateNewUniqueNullInt32 will take in old value and return new unique value with randomized seed and negated
func GenerateNewUniqueNullInt32(oldIntVal sql.NullInt32) sql.NullInt32 {
	if !oldIntVal.Valid {
		return oldIntVal
	}

	seed1 := GenerateRandomNumber(999)
	seed2 := GenerateRandomNumber(99)

	buf := Right(Itoa(FromNullInt(oldIntVal)), 5) + Padding(Itoa(seed2), 2, false, "0") + Padding(Itoa(seed1), 3, false, "0")

	val, ok := ParseInt32(buf)

	if !ok {
		return ToNullInt(int(randomInt32n(math.MaxInt32))*-1, true)
	} else {
		return ToNullInt(int(safeNegateInt(val)), true)
	}
}

// GenerateNewUniqueInt64 will take in old value and return new unique value with randomized seed and negated
func GenerateNewUniqueInt64(oldIntVal int64) int64 {
	seed1 := GenerateRandomNumber(999)
	seed2 := GenerateRandomNumber(999)

	buf := Right(Int64ToString(oldIntVal), 13) + Padding(Itoa(seed2), 3, false, "0") + Padding(Itoa(seed1), 3, false, "0")

	val, ok := ParseInt64(buf)

	if !ok {
		return safeNegateInt64(randomInt64n(math.MaxInt64))
	} else {
		return safeNegateInt64(val)
	}
}

func safeNegateInt(v int) int { // prevent MinInt overflow
	minInt := ^int(^uint(0) >> 1) // compute min int without overflow
	maxInt := int(^uint(0) >> 1)  // compute max int without overflow

	if v == minInt {
		return maxInt
	}
	return -v
}

func safeNegateInt64(v int64) int64 { // prevent MinInt64 overflow
	if v == math.MinInt64 {
		return math.MaxInt64
	}
	return -v
}

// GenerateNewUniqueNullInt64 will take in old value and return new unique value with randomized seed and negated
func GenerateNewUniqueNullInt64(oldIntVal sql.NullInt64) sql.NullInt64 {
	if !oldIntVal.Valid {
		return oldIntVal
	}

	seed1 := GenerateRandomNumber(999)
	seed2 := GenerateRandomNumber(999)

	buf := Right(Int64ToString(FromNullInt64(oldIntVal)), 13) + Padding(Itoa(seed2), 3, false, "0") + Padding(Itoa(seed1), 3, false, "0")

	val, ok := ParseInt64(buf)

	if !ok {
		return ToNullInt64(safeNegateInt64(randomInt64n(math.MaxInt64)), true)
	} else {
		return ToNullInt64(safeNegateInt64(val), true)
	}
}

// ================================================================================================================
// String Randomizer
// ================================================================================================================

// GenerateNewUniqueString will take in old value and return new unique value with randomized seed
//
//	stringLimit = 0 no limit, > 0 has limit
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
				buf = Left(seed2+seed3+seed4+seed1, stringLimit)
			} else {
				buf = Left(seed2+seed3, stringLimit)
			}
		}
	}

	return buf
}

// GenerateNewUniqueNullString will take in old value and return new unique value with randomized seed
//
//	stringLimit = 0 no limit, > 0 has limit
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
				buf = Left(seed2+seed3+seed4+seed1, stringLimit)
			} else {
				buf = Left(seed2+seed3, stringLimit)
			}
		}
	}

	return ToNullString(buf, true)
}
