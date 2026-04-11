package crypto

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
	"testing"
)

// TestCryptoRoundtrip_UnicodePassphrase pins the byte-indexed contract of util.Left
// across every passphrase/key truncation site in this package (8 call sites in v1.7.9:
// AesGcmEncrypt, AesGcmDecrypt, AesCfbEncrypt, AesCfbDecrypt, AesCbcEncrypt,
// AesCbcDecrypt, AppendHmac, ValidateHmac).
//
// P0-2 (commit 0cabc7e) reverted util.Left/Right/Mid to byte-indexed slicing after a
// prior refactor had made them rune-indexed and broken the 32-byte AES-key contract.
// This test is the P1-3 follow-up from remediation-report-2026-04-11-P1.md that pins
// the restoration so a future regression is caught by the test suite, not by a
// consumer runtime failure.
//
// Regression mechanics: if util.Left ever becomes rune-indexed again, then a passphrase
// such as strings.Repeat("汉", 11) (33 bytes, 11 runes) will be truncated to 11 runes
// instead of 32 bytes by util.Left(passphrase, 32), yielding a string shorter than
// 32 bytes. aes.NewCipher will then reject it with "invalid key size", and every
// sub-test below will fail fast. The test therefore does NOT need to assert key
// length directly — the round-trip itself is the assertion.
func TestCryptoRoundtrip_UnicodePassphrase(t *testing.T) {
	// Test plaintexts:
	//   - `plaintext` is used by GCM/CFB and HMAC; it contains multi-byte runes to
	//     exercise both unicode output and ensure the functions treat it as a byte
	//     stream (AES is byte-oriented; unicode is incidental).
	//   - `cbcPlaintext` is used by CBC; it must be exactly 16 bytes (one AES block)
	//     so the NUL-padding+stripping pair round-trips cleanly (CBC decrypt strips
	//     all NUL bytes from the output — any NUL in the plaintext would fail the
	//     equality check, though the ciphertext itself would still be valid).
	const (
		plaintext    = "hello, unicode world — with some 汉字 and emoji 🔒"
		cbcPlaintext = "hello-cbc-block!" // 16 bytes exactly
	)

	cases := []struct {
		name       string
		passphrase string
	}{
		// Baseline: pure ASCII, exact 32 bytes — proves the test harness works.
		{
			name:       "ascii_exact_32_bytes",
			passphrase: "01234567890123456789012345678901",
		},
		// ASCII with extra suffix — util.Left must drop the extra bytes cleanly.
		{
			name:       "ascii_over_32_bytes",
			passphrase: "0123456789abcdef0123456789abcdef-extra-suffix-to-be-truncated",
		},
		// Pure emoji passphrase at exactly 32 bytes (8 × 4-byte runes). This is the
		// "unicode but byte-aligned" case — no rune is split.
		{
			name:       "unicode_emoji_exact_32_bytes",
			passphrase: strings.Repeat("😀", 8),
		},
		// CRITICAL CASE: "汉" is 3 bytes in UTF-8. 11 × 3 = 33 bytes, 11 runes.
		// util.Left(passphrase, 32) must truncate at byte index 32, slicing through
		// the middle of the 11th rune and producing a 32-byte string whose tail is
		// an incomplete UTF-8 sequence. AES is byte-oriented and does not care.
		// If util.Left were rune-indexed, this call would return 11 × 3 = 33 bytes
		// (or fewer on rune-count 32), NOT exactly 32, and aes.NewCipher would fail.
		{
			name:       "unicode_cjk_33_bytes_splits_mid_rune",
			passphrase: strings.Repeat("汉", 11),
		},
		// Mixed ASCII + multi-byte runes well over 32 bytes.
		{
			name:       "unicode_mixed_over_32_bytes",
			passphrase: "passphrase_汉字_🔑_" + strings.Repeat("x", 32),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Pre-flight: every passphrase must clear the `len(passphrase) < 32`
			// pre-check that each crypto function performs before calling util.Left.
			// A < 32-byte passphrase would fail for an unrelated reason and mask the
			// contract we are actually testing.
			if got := len(tc.passphrase); got < 32 {
				t.Fatalf("test-setup bug: passphrase %q has %d bytes (< 32)", tc.passphrase, got)
			}

			// AES-GCM round-trip — exercises util.Left at crypto.go:165 and :222.
			if enc, err := AesGcmEncrypt(plaintext, tc.passphrase); err != nil {
				t.Fatalf("AesGcmEncrypt: %v", err)
			} else if dec, err := AesGcmDecrypt(enc, tc.passphrase); err != nil {
				t.Fatalf("AesGcmDecrypt: %v", err)
			} else if dec != plaintext {
				t.Errorf("AES-GCM round-trip mismatch:\n got: %q\nwant: %q", dec, plaintext)
			}

			// AES-CFB round-trip — exercises util.Left at crypto.go:290 and :336.
			if enc, err := AesCfbEncrypt(plaintext, tc.passphrase); err != nil {
				t.Fatalf("AesCfbEncrypt: %v", err)
			} else if dec, err := AesCfbDecrypt(enc, tc.passphrase); err != nil {
				t.Fatalf("AesCfbDecrypt: %v", err)
			} else if dec != plaintext {
				t.Errorf("AES-CFB round-trip mismatch:\n got: %q\nwant: %q", dec, plaintext)
			}

			// AES-CBC round-trip — exercises util.Left at crypto.go:394 and :448.
			// Uses the exact-block-size plaintext so NUL-padding+stripping is a no-op
			// and the equality check is meaningful.
			if enc, err := AesCbcEncrypt(cbcPlaintext, tc.passphrase); err != nil {
				t.Fatalf("AesCbcEncrypt: %v", err)
			} else if dec, err := AesCbcDecrypt(enc, tc.passphrase); err != nil {
				t.Fatalf("AesCbcDecrypt: %v", err)
			} else if dec != cbcPlaintext {
				t.Errorf("AES-CBC round-trip mismatch:\n got: %q\nwant: %q", dec, cbcPlaintext)
			}

			// HMAC append/validate round-trip — exercises util.Left at crypto.go:511
			// and :540. AppendHmac returns `message||hex(hmac)`; ValidateHmac splits,
			// recomputes, compares, and returns the original message.
			if msgWithMac, err := AppendHmac(plaintext, tc.passphrase); err != nil {
				t.Fatalf("AppendHmac: %v", err)
			} else if msg, err := ValidateHmac(msgWithMac, tc.passphrase); err != nil {
				t.Fatalf("ValidateHmac: %v", err)
			} else if msg != plaintext {
				t.Errorf("HMAC round-trip mismatch:\n got: %q\nwant: %q", msg, plaintext)
			}
		})
	}
}
