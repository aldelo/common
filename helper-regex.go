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
		fromIdx, fromEnd := indexEqualFoldSpan(source, from, start) // track matched span (start,end)
		if fromIdx == -1 {
			break
		}

		toIdx, toEnd := indexEqualFoldSpan(source, to, fromEnd) // start search after actual matched bytes
		if toIdx == -1 {
			break
		}

		b.WriteString(source[start:fromEnd]) // preserve boundary using matched end
		b.WriteString(replace)
		b.WriteString(source[toIdx:toEnd]) // preserve trailing delimiter using matched span

		start = toEnd // advance by actual matched length to avoid overlap/corruption
		replaced = true
	}

	if !replaced {
		return source
	}

	b.WriteString(source[start:])
	return b.String()
}

// indexEqualFoldSpan finds the first match of substr in s at or after start using Unicode case-folding.
// Returns the byte start and end of the matched span; (-1, -1) if not found.
// Handles multi-rune fold expansions (e.g., ß <-> SS, İ <-> i̇, ligatures) by searching over a small window.
const maxFoldExpansionRunes = 3 // allow up to 3-rune expansions for safety

func indexEqualFoldSpan(s, substr string, start int) (int, int) { // new span-returning helper
	if substr == "" {
		return start, start
	}

	minRunes := utf8.RuneCountInString(substr)
	maxRunes := minRunes + maxFoldExpansionRunes

	for i := 0; i < len(s); {
		if i < start {
			_, sz := utf8.DecodeRuneInString(s[i:])
			i += sz
			continue
		}

		// Expand a window from i over [minRunes, maxRunes] runes to catch fold expansions.
		for windowRunes, end := 0, i; windowRunes < maxRunes && end <= len(s); windowRunes++ {
			if end == len(s) {
				break
			}
			_, sz := utf8.DecodeRuneInString(s[end:])
			if sz == 0 {
				break
			}
			end += sz
			if windowRunes+1 >= minRunes && strings.EqualFold(s[i:end], substr) {
				return i, end // return both start and end
			}
		}

		_, sz := utf8.DecodeRuneInString(s[i:])
		if sz == 0 {
			break
		}
		i += sz
	}
	return -1, -1
}

// indexEqualFold is kept for any callers expecting just the index.
func indexEqualFold(s, substr string, start int) int { // now delegates to span helper
	idx, _ := indexEqualFoldSpan(s, substr, start)
	return idx
}
