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
	"strings"
)

// RegexReplaceSubString will search for substring between subStringFrom and subStringTo, replace with the replaceWith string, and optionally case insensitive or not
func RegexReplaceSubString(source string, subStringFrom string, subStringTo string, replaceWith string, caseInsensitive bool) string {
	// guard empty delimiters to avoid surprising matches
	if subStringFrom == "" || subStringTo == "" {
		return source
	}

	// fast-path bail-out if delimiters aren’t present, to avoid regex work
	if !caseInsensitive {
		if !strings.Contains(source, subStringFrom) || !strings.Contains(source, subStringTo) {
			return source
		}
	} else {
		lowerSource := strings.ToLower(source)
		if !strings.Contains(lowerSource, strings.ToLower(subStringFrom)) || !strings.Contains(lowerSource, strings.ToLower(subStringTo)) {
			return source
		}
	}

	// setup regex
	ci := ""
	if caseInsensitive {
		ci = "(?i)"
	}

	// safely escape user input to avoid regex injection / invalid patterns
	fromEsc := regexp.QuoteMeta(subStringFrom)
	toEsc := regexp.QuoteMeta(subStringTo)

	// scope dot-all to the middle only and avoid later re-matches
	pattern := ci + "(" + fromEsc + ")(?s:.*?)(" + toEsc + ")"

	// handle compile errors instead of panicking
	regE, err := regexp.Compile(pattern)
	if err != nil {
		return source
	}

	// build a literal-safe replacement (escape $ and \ so they are not treated as backrefs)
	literalReplace := strings.ReplaceAll(strings.ReplaceAll(replaceWith, `\`, `\\`), "$", "$$")

	// single-pass replace without re-running the regex per match
	return regE.ReplaceAllString(source, "${1}"+literalReplace+"${2}")
}
