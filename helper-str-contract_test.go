package helper

/*
 * Copyright 2020-2026 Aldelo, LP
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 */

// Regression tests pinning the v1.6.7 observable contract of
// LenTrim, Left, Right, Mid, and NextFixedLength.
//
// These tests exist because commit af0d217 silently changed these helpers
// from byte-based to rune-based semantics (and changed NextFixedLength's
// block-boundary formula), breaking the contract relied on by 36+
// downstream repos — most critically crypto/crypto.go, which uses
// Left(passphrase, 32) to derive AES-256 keys that MUST be 32 bytes long.
//
// Rule #10 (workspace): preserve observable contracts of shared libraries
// across minor-version bumps; fix in the shared library, not in consumers.

import (
	"crypto/aes"
	"testing"
)

// -----------------------------------------------------------------------
// LenTrim — MUST count bytes, not runes. (P0-1)
// -----------------------------------------------------------------------

func TestLenTrim_ByteSemantics_ASCII(t *testing.T) {
	// ASCII: byte count == rune count, sanity check.
	if got := LenTrim("hello"); got != 5 {
		t.Fatalf("LenTrim(\"hello\") = %d, want 5", got)
	}
}

func TestLenTrim_ByteSemantics_MultibyteRune(t *testing.T) {
	// "café" = 5 bytes ('c','a','f', 0xC3, 0xA9) but 4 runes.
	// v1.6.7 contract returns 5 (bytes). Rune-based impl returns 4 → FAIL.
	if got := LenTrim("café"); got != 5 {
		t.Fatalf("LenTrim(\"café\") = %d, want 5 (byte count, not rune count)", got)
	}
}

func TestLenTrim_ByteSemantics_TrimsFirst(t *testing.T) {
	// Whitespace trimming still applies, then byte-count the trimmed result.
	if got := LenTrim("  café  "); got != 5 {
		t.Fatalf("LenTrim(\"  café  \") = %d, want 5", got)
	}
}

func TestLenTrim_Empty(t *testing.T) {
	if got := LenTrim(""); got != 0 {
		t.Fatalf("LenTrim(\"\") = %d, want 0", got)
	}
	if got := LenTrim("   "); got != 0 {
		t.Fatalf("LenTrim(\"   \") = %d, want 0", got)
	}
}

// -----------------------------------------------------------------------
// Left — MUST byte-slice, not rune-slice. (P0-2)
// This is the regression that silently breaks AES-256 key derivation in
// crypto/crypto.go for any passphrase containing non-ASCII runes.
// -----------------------------------------------------------------------

func TestLeft_ByteSemantics_ASCII(t *testing.T) {
	if got := Left("abcdef", 3); got != "abc" {
		t.Fatalf("Left(\"abcdef\", 3) = %q, want \"abc\"", got)
	}
}

func TestLeft_ByteSemantics_MultibyteRune(t *testing.T) {
	// "café" = 5 bytes. v1.6.7 Left("café", 3) returns the first 3 bytes = "caf".
	// Rune-based impl returns "caf" too by coincidence here — but the point is
	// that Left MUST return a byte-slice. Let's pick a case that disambiguates:
	// "é" = 2 bytes (0xC3 0xA9). Left("éabc", 2) must return "é" (2 bytes),
	// not the first 2 runes "éa".
	if got := Left("éabc", 2); got != "é" {
		t.Fatalf("Left(\"éabc\", 2) = %q, want %q (2 bytes = 1 rune for é)", got, "é")
	}
}

func TestLeft_AESKeyDerivation_32ByteContract(t *testing.T) {
	// This is the crypto/crypto.go call-site contract:
	// util.Left(passphrase, 32) MUST return exactly 32 bytes so aes.NewCipher
	// can produce an AES-256 cipher. Rune-slicing breaks this for unicode
	// passphrases.
	//
	// Build a passphrase that has at least 32 bytes but fewer than 32 runes
	// when any multi-byte rune is included.
	passphrase := "passwordé-passwordé-passwordé-passwordé" // 32+ bytes, < 32 runes

	key := Left(passphrase, 32)
	if len(key) != 32 {
		t.Fatalf("Left(unicode-passphrase, 32) = %d bytes, want 32 (AES-256 requires exactly 32)", len(key))
	}

	// And it must actually work as an AES-256 key.
	if _, err := aes.NewCipher([]byte(key)); err != nil {
		t.Fatalf("aes.NewCipher rejected the Left() output: %v", err)
	}
}

func TestLeft_LongerThanString_ReturnsOriginal(t *testing.T) {
	// v1.6.7 contract: len(s) <= l returns s.
	if got := Left("abc", 10); got != "abc" {
		t.Fatalf("Left(\"abc\", 10) = %q, want \"abc\"", got)
	}
}

// -----------------------------------------------------------------------
// Right — MUST byte-slice, not rune-slice. (P0-2)
// -----------------------------------------------------------------------

func TestRight_ByteSemantics_ASCII(t *testing.T) {
	if got := Right("abcdef", 3); got != "def" {
		t.Fatalf("Right(\"abcdef\", 3) = %q, want \"def\"", got)
	}
}

func TestRight_ByteSemantics_MultibyteRune(t *testing.T) {
	// "abcé" = 5 bytes. Right("abcé", 2) must return the last 2 bytes = "é"
	// (the 2-byte UTF-8 encoding of é). Rune-based impl returns "cé" (2 runes
	// = 3 bytes).
	if got := Right("abcé", 2); got != "é" {
		t.Fatalf("Right(\"abcé\", 2) = %q, want %q (2 bytes)", got, "é")
	}
}

func TestRight_LongerThanString_ReturnsOriginal(t *testing.T) {
	if got := Right("abc", 10); got != "abc" {
		t.Fatalf("Right(\"abc\", 10) = %q, want \"abc\"", got)
	}
}

// -----------------------------------------------------------------------
// Mid — MUST byte-slice, not rune-slice. (P0-2)
// -----------------------------------------------------------------------

func TestMid_ByteSemantics_ASCII(t *testing.T) {
	// v1.6.7: Mid("abcdef", 2, 3) → s[2:2+3] = "cde"
	if got := Mid("abcdef", 2, 3); got != "cde" {
		t.Fatalf("Mid(\"abcdef\", 2, 3) = %q, want \"cde\"", got)
	}
}

func TestMid_ByteSemantics_MultibyteRune(t *testing.T) {
	// "abéde" = 6 bytes (a,b,0xC3,0xA9,d,e). Mid with byte offsets:
	// start=2, l=2 → s[2:4] = "é" (the two UTF-8 bytes).
	if got := Mid("abéde", 2, 2); got != "é" {
		t.Fatalf("Mid(\"abéde\", 2, 2) = %q, want %q", got, "é")
	}
}

// -----------------------------------------------------------------------
// NextFixedLength — MUST use (len(data)/blockSize+1)*blockSize formula,
// which always ADVANCES to the next block even when already aligned.
// This is PKCS#7-compatible padding behavior. (P0-3)
// -----------------------------------------------------------------------

func TestNextFixedLength_AlignedInputAdvancesToNextBlock(t *testing.T) {
	// v1.6.7 contract: len("16-byte-str_____") = 16, blockSize = 16
	//   → (16/16 + 1) * 16 = 32
	// Ceiling-div impl returns 16 — WRONG for CBC padding which requires
	// even aligned inputs to get a full extra block.
	data := "0123456789abcdef" // exactly 16 bytes
	if got := NextFixedLength(data, 16); got != 32 {
		t.Fatalf("NextFixedLength(16-byte, 16) = %d, want 32 (always advance on aligned boundary)", got)
	}
}

func TestNextFixedLength_SubBlockInput(t *testing.T) {
	// len("abc") = 3, blockSize = 16 → (3/16 + 1) * 16 = 16
	if got := NextFixedLength("abc", 16); got != 16 {
		t.Fatalf("NextFixedLength(\"abc\", 16) = %d, want 16", got)
	}
}

func TestNextFixedLength_JustOverBlock(t *testing.T) {
	// len = 17 → (17/16 + 1) * 16 = 32
	if got := NextFixedLength("0123456789abcdefX", 16); got != 32 {
		t.Fatalf("NextFixedLength(17-byte, 16) = %d, want 32", got)
	}
}

func TestNextFixedLength_ByteCount_NotRuneCount(t *testing.T) {
	// "café" = 5 bytes, 4 runes. Block=16 → (5/16+1)*16 = 16
	// Rune-based impl gives (4/16+1)*16 = 16 too — same answer in this case.
	// But pick a case that disambiguates: 15 bytes of "é" × 7 = 14 bytes,
	// 7 runes. We want byte-based (5-byte-aligned example):
	// "éé" = 4 bytes, 2 runes. With blockSize=4 → byte: (4/4+1)*4 = 8
	//                                              rune: (2/4+1)*4 = 4 ← wrong
	data := "éé" // 4 bytes
	if got := NextFixedLength(data, 4); got != 8 {
		t.Fatalf("NextFixedLength(\"éé\", 4) = %d, want 8 (byte-count × always-advance)", got)
	}
}

func TestNextFixedLength_ZeroBlockSizeIsSafe(t *testing.T) {
	// This is a safety guard, not part of v1.6.7. v1.6.7 would panic
	// on integer-divide-by-zero. Keep the guard: return 0 instead of panic.
	if got := NextFixedLength("anything", 0); got != 0 {
		t.Fatalf("NextFixedLength with blockSize=0 must return 0 (safety), got %d", got)
	}
	if got := NextFixedLength("anything", -1); got != 0 {
		t.Fatalf("NextFixedLength with blockSize=-1 must return 0 (safety), got %d", got)
	}
}

// -----------------------------------------------------------------------
// Base64StdDecode — empty input must return ("", nil), not error. (P1-1)
// Mirrors the Base64UrlDecode fix already shipped in v1.7.8 (43e842c).
// v1.6.7 was a thin wrapper over base64.StdEncoding.DecodeString which
// returns ([]byte{}, nil) for empty input — the observable contract is
// ("", nil). HEAD rejects empty input with an error, which breaks any
// decode pipeline that tolerates optional/empty fields.
// -----------------------------------------------------------------------

func TestBase64StdDecode_EmptyInput_ReturnsEmptyNilError(t *testing.T) {
	got, err := Base64StdDecode("")
	if err != nil {
		t.Fatalf("Base64StdDecode(\"\") returned error %v, want nil (v1.6.7 contract)", err)
	}
	if got != "" {
		t.Fatalf("Base64StdDecode(\"\") = %q, want \"\"", got)
	}
}

func TestBase64StdDecode_WhitespaceOnly_ReturnsEmptyNilError(t *testing.T) {
	// Whitespace-only input strips to empty — same contract as true empty.
	got, err := Base64StdDecode("   \n\t  ")
	if err != nil {
		t.Fatalf("Base64StdDecode(whitespace) returned error %v, want nil", err)
	}
	if got != "" {
		t.Fatalf("Base64StdDecode(whitespace) = %q, want \"\"", got)
	}
}

// -----------------------------------------------------------------------
// Is*Only validators — empty input must return true (vacuously satisfies
// "contains only X"). (P1-2)
//
// Flipped in HEAD to return false, which breaks validation chains that
// pre-filter empty optional fields. The sibling IsNumericIntOnly was
// already restored to true in commit 43e842c; these four were missed.
// -----------------------------------------------------------------------

func TestIsAlphanumericOnly_EmptyReturnsTrue(t *testing.T) {
	if !IsAlphanumericOnly("") {
		t.Fatalf("IsAlphanumericOnly(\"\") = false, want true (v1.6.7 contract)")
	}
}

func TestIsAlphanumericAndSpaceOnly_EmptyReturnsTrue(t *testing.T) {
	if !IsAlphanumericAndSpaceOnly("") {
		t.Fatalf("IsAlphanumericAndSpaceOnly(\"\") = false, want true (v1.6.7 contract)")
	}
}

func TestIsHexOnly_EmptyReturnsTrue(t *testing.T) {
	if !IsHexOnly("") {
		t.Fatalf("IsHexOnly(\"\") = false, want true (v1.6.7 contract)")
	}
}

func TestIsBase64Only_EmptyReturnsTrue(t *testing.T) {
	if !IsBase64Only("") {
		t.Fatalf("IsBase64Only(\"\") = false, want true (v1.6.7 contract)")
	}
}

// Positive-path guard: the fix must NOT break valid-input behavior.

func TestIsAlphanumericOnly_PositivePathStillWorks(t *testing.T) {
	if !IsAlphanumericOnly("abc123") {
		t.Fatal("IsAlphanumericOnly(\"abc123\") = false, want true")
	}
	if IsAlphanumericOnly("abc 123") {
		t.Fatal("IsAlphanumericOnly(\"abc 123\") = true, want false (space not allowed)")
	}
}

func TestIsHexOnly_PositivePathStillWorks(t *testing.T) {
	if !IsHexOnly("deadBEEF") {
		t.Fatal("IsHexOnly(\"deadBEEF\") = false, want true")
	}
	if IsHexOnly("xyz") {
		t.Fatal("IsHexOnly(\"xyz\") = true, want false")
	}
}

// -----------------------------------------------------------------------
// SliceStringToCSVString — no RFC 4180 quoting; dumb comma join. (P1-7)
//
// v1.6.7 contract: plain comma-join, zero escaping. If a field contained
// a delimiter or quote character, it was emitted verbatim (ambiguous but
// stable). HEAD silently switched to RFC 4180 quoting, producing
// different output that breaks downstream parsers tuned to the old form.
//
// Restore v1.6.7 behavior. Per review, if RFC 4180 behavior is genuinely
// wanted, introduce a sibling SliceStringToCSVStringRFC4180 as a
// deliberate breaking add-on rather than silently upgrading the existing
// function. This protects 36+ downstream consumers.
// -----------------------------------------------------------------------

func TestSliceStringToCSVString_NoQuoting_PlainConcat(t *testing.T) {
	// Simple case — no delimiter collision. Must be identical in both
	// v1.6.7 and HEAD, sanity check.
	got := SliceStringToCSVString([]string{"a", "b", "c"}, false)
	if got != "a,b,c" {
		t.Fatalf("SliceStringToCSVString([a,b,c], false) = %q, want %q", got, "a,b,c")
	}
}

func TestSliceStringToCSVString_ElementContainsComma_EmittedVerbatim(t *testing.T) {
	// v1.6.7: ["a,b", "c"] → "a,b,c" (ambiguous but stable).
	// HEAD pre-fix: ["a,b", "c"] → "\"a,b\",c" (RFC 4180).
	// Restore the dumb-join contract.
	got := SliceStringToCSVString([]string{"a,b", "c"}, false)
	want := "a,b,c"
	if got != want {
		t.Fatalf("SliceStringToCSVString([\"a,b\", \"c\"]) = %q, want %q (v1.6.7 dumb-join contract)", got, want)
	}
}

func TestSliceStringToCSVString_ElementContainsQuote_EmittedVerbatim(t *testing.T) {
	// v1.6.7: ["a\"b", "c"] → `a"b,c`.
	// HEAD pre-fix: wraps and doubles quotes → `"a""b",c`.
	got := SliceStringToCSVString([]string{`a"b`, "c"}, false)
	want := `a"b,c`
	if got != want {
		t.Fatalf("SliceStringToCSVString with embedded quote = %q, want %q", got, want)
	}
}

func TestSliceStringToCSVString_SpaceAfterComma_Preserved(t *testing.T) {
	got := SliceStringToCSVString([]string{"a", "b"}, true)
	if got != "a, b" {
		t.Fatalf("SliceStringToCSVString([a,b], true) = %q, want %q", got, "a, b")
	}
}

func TestSliceStringToCSVString_EmptySlice_ReturnsEmpty(t *testing.T) {
	got := SliceStringToCSVString(nil, false)
	if got != "" {
		t.Fatalf("SliceStringToCSVString(nil) = %q, want \"\"", got)
	}
	got = SliceStringToCSVString([]string{}, false)
	if got != "" {
		t.Fatalf("SliceStringToCSVString([]) = %q, want \"\"", got)
	}
}

// -----------------------------------------------------------------------
// Replace — thin wrapper over strings.Replace(-1), no empty-old guard. (P1-8)
//
// NOTE on review wording: the deep-review text labelled this as
// "guard against old == \"\" changed from panic-safe early-return to
// unbounded-loop-prone path — fix: restore guard." Reading both sources
// directly contradicts that:
//   v1.6.7:  return strings.Replace(s, oldChar, newChar, -1)   // NO guard
//   HEAD  :  if oldChar == "" { return s }; strings.Replace(...) // ADDED guard
// Go's strings.Replace with empty old does NOT panic — it inserts newChar
// between every rune (stdlib defined behavior). So the real contract drift
// is the opposite of the review's claim: HEAD silently changed the return
// value for empty-old calls from "inserted between runes" to "unchanged
// input". Under Rule #10 literal application, the v1.6.7 observable
// contract must be preserved — which means REMOVING the HEAD guard.
// -----------------------------------------------------------------------

func TestReplace_NonEmptyOld_NormalPath(t *testing.T) {
	got := Replace("hello world", "world", "Go")
	if got != "hello Go" {
		t.Fatalf("Replace(\"hello world\", \"world\", \"Go\") = %q, want %q", got, "hello Go")
	}
}

func TestReplace_EmptyOld_InsertsBetweenRunes_v167Contract(t *testing.T) {
	// strings.Replace(s, "", new, -1) inserts `new` between every rune
	// AND at both ends, so "abc" + old="" + new="X" → "XaXbXcX".
	// HEAD pre-fix returns "abc" because of the added guard. Restore v1.6.7.
	got := Replace("abc", "", "X")
	want := "XaXbXcX"
	if got != want {
		t.Fatalf("Replace(\"abc\", \"\", \"X\") = %q, want %q (v1.6.7 contract = stdlib strings.Replace)", got, want)
	}
}

func TestReplace_EmptyString_Empty(t *testing.T) {
	// Empty s + non-empty old → no matches → empty result (sanity).
	got := Replace("", "x", "y")
	if got != "" {
		t.Fatalf("Replace(\"\", \"x\", \"y\") = %q, want \"\"", got)
	}
}
