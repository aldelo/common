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
	"unicode"
	"unicode/utf8"
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
		b.WriteString(source[toIdx : toIdx+len(to)]) // preserve trailing delimiter
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
	var b strings.Builder
	start := 0
	replaced := false

	for {
		fromIdx := indexEqualFold(source, from, start) // Unicode-safe case-insensitive search
		if fromIdx == -1 {
			break
		}

		toIdx := indexEqualFold(source, to, fromIdx+len(from)) // Unicode-safe case-insensitive search
		if toIdx == -1 {
			break
		}

		b.WriteString(source[start : fromIdx+len(from)]) // preserve original casing on boundaries
		b.WriteString(replace)
		b.WriteString(source[toIdx : toIdx+len(to)]) // preserve trailing delimiter

		start = toIdx + len(to)
		replaced = true
	}

	if !replaced {
		return source
	}

	b.WriteString(source[start:])
	return b.String()
}

// indexEqualFold finds the first index of substr in s at or after start using Unicode case-folding without
// changing string length (avoids ToLower length-mismatch bugs). Returns -1 if not found.
func indexEqualFold(s, substr string, start int) int {
	if substr == "" {
		return start
	}
	// walk s on rune boundaries to stay UTF-8 safe
	for i := start; i < len(s); {
		if hasPrefixEqualFold(s[i:], substr) {
			return i
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		if size == 0 {
			break
		}
		i += size
	}
	return -1
}

// hasPrefixEqualFold reports whether s has the given substr prefix using Unicode case-folding.
func hasPrefixEqualFold(s, substr string) bool {
	for len(substr) > 0 {
		r1, size1 := utf8.DecodeRuneInString(s)
		r2, size2 := utf8.DecodeRuneInString(substr)
		if size1 == 0 || size2 == 0 { // ran out of runes in either string
			return false
		}
		if !foldRuneEqual(r1, r2) {
			return false
		}
		s = s[size1:]
		substr = substr[size2:]
	}
	return true
}

// foldRuneEqual performs Unicode-aware case folding comparison for two runes.
func foldRuneEqual(a, b rune) bool {
	if a == b {
		return true
	}
	for r := unicode.SimpleFold(a); r != a; r = unicode.SimpleFold(r) {
		if r == b {
			return true
		}
	}
	return false
}
