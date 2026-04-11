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
