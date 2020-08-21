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
	"strings"
)

// LenTrim returns length of space trimmed string s
func LenTrim(s string) int {
	return len(strings.TrimSpace(s))
}

// SplitString will split the source string using delimiter, and return the element indicated by index,
// if nothing is found, blank is returned,
// index = -1 returns last index
func SplitString(source string, delimiter string, index int) string {
	ar := strings.Split(source, delimiter)

	if len(ar) > 0 {
		if index <= -1 {
			return ar[len(ar)-1]
		} else {
			if len(ar) > index {
				return ar[index]
			} else {
				return ""
			}
		}
	}

	return ""
}