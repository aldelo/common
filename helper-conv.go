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
	"strconv"
	"time"
)

// IntPtr casts int to int pointer
func IntPtr(i int) *int {
	return &i
}

// DurationPtr casts Duration to Duration pointer
func DurationPtr(d time.Duration) *time.Duration {
	return &d
}

// StrToUint converts from string to uint
func StrToUint(s string) uint {
	if v, e := strconv.ParseUint(s, 10, 32); e != nil {
		return 0
	} else {
		return uint(v)
	}
}


