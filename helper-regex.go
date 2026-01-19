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
	"strings"
)

// RegexReplaceSubString will search for substring between subStringFrom and subStringTo, replace with the replaceWith string, and optionally case insensitive or not
func RegexReplaceSubString(source string, subStringFrom string, subStringTo string, replaceWith string, caseInsensitive bool) string {
	// guard empty delimiters to avoid surprising matches
	if subStringFrom == "" || subStringTo == "" {
		return source
	}

	// replace regex-based implementation with deterministic O(n) scanning to avoid regex backtracking / CPU blow-ups
	if !caseInsensitive {
		return replaceAllBetween(source, subStringFrom, subStringTo, replaceWith)
	}
	return replaceAllBetweenCI(source, subStringFrom, subStringTo, replaceWith)
}

// deterministic, literal, case-sensitive replacement for all occurrences
func replaceAllBetween(source, from, to, replace string) string {
	var b strings.Builder
	start := 0
	replaced := false

	for {
		fromIdx := strings.Index(source[start:], from)
		if fromIdx == -1 {
			break
		}
		fromIdx += start

		toIdx := strings.Index(source[fromIdx+len(from):], to)
		if toIdx == -1 {
			break
		}
		toIdx += fromIdx + len(from)

		b.WriteString(source[start : fromIdx+len(from)])
		b.WriteString(replace)
		start = toIdx + len(to)
		replaced = true
	}

	if !replaced {
		return source
	}

	b.WriteString(source[start:])
	return b.String()
}

// deterministic, literal, case-insensitive replacement for all occurrences
func replaceAllBetweenCI(source, from, to, replace string) string {
	lower := strings.ToLower(source)
	lowerFrom := strings.ToLower(from)
	lowerTo := strings.ToLower(to)

	var b strings.Builder
	start := 0
	replaced := false

	for {
		fromIdx := strings.Index(lower[start:], lowerFrom)
		if fromIdx == -1 {
			break
		}
		fromIdx += start

		toIdx := strings.Index(lower[fromIdx+len(from):], lowerTo)
		if toIdx == -1 {
			break
		}
		toIdx += fromIdx + len(from)

		b.WriteString(source[start : fromIdx+len(from)]) // preserve original casing on boundaries
		b.WriteString(replace)
		start = toIdx + len(to)
		replaced = true
	}

	if !replaced {
		return source
	}

	b.WriteString(source[start:])
	return b.String()
}
