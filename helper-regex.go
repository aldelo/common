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
	"regexp"
	"strings"
)

// RegexReplaceSubString will search for substring between subStringFrom and subStringTo, replace with the replaceWith string, and optionally case insensitive or not
func RegexReplaceSubString(source string, subStringFrom string, subStringTo string, replaceWith string, caseInsensitive bool) string {
	// setup regex
	ci := ""

	if caseInsensitive {
		ci = "(?i)"
	}

	regE := regexp.MustCompile(ci + subStringFrom + "(.*)" + subStringTo)

	// find sub match
	m := regE.FindStringSubmatch(source)

	if len(m) >= 1 {
		// found one or more match, use the first found only
		return strings.ReplaceAll(source, m[0], replaceWith)
	} else {
		// no match found, return source as is
		return source
	}
}
