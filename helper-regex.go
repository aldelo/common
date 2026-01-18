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
	"regexp"
)

// RegexReplaceSubString will search for substring between subStringFrom and subStringTo, replace with the replaceWith string, and optionally case insensitive or not
func RegexReplaceSubString(source string, subStringFrom string, subStringTo string, replaceWith string, caseInsensitive bool) string {
	// guard empty delimiters to avoid surprising matches
	if subStringFrom == "" || subStringTo == "" {
		return source
	}

	// setup regex
	ci := ""
	if caseInsensitive {
		ci = "(?i)"
	}

	// safely escape user input to avoid regex injection / invalid patterns
	fromEsc := regexp.QuoteMeta(subStringFrom)
	toEsc := regexp.QuoteMeta(subStringTo)

	// capture delimiters and use dot-all so matches can span newlines
	pattern := ci + "(" + fromEsc + ")(?s)(.*?)(" + toEsc + ")"

	// handle compile errors instead of panicking
	regE, err := regexp.Compile(pattern)
	if err != nil {
		return source
	}

	// preserve delimiters and treat replaceWith literally (no $ expansion)
	return regE.ReplaceAllStringFunc(source, func(segment string) string {
		parts := regE.FindStringSubmatch(segment)
		if len(parts) != 4 {
			return segment
		}
		return parts[1] + replaceWith + parts[3]
	})
}
