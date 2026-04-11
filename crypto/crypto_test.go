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

// -----------------------------------------------------------------------
// P0-6 — Md5 helper: observable contract pin + deprecation call-site test.
//
// Md5 is deprecated in v1.7.9 (scheduled for removal in v2.0.0) but must
// remain CALLABLE and must produce the same output it has always produced
// for the remainder of the v1.x series, or downstream consumers that
// still call it will break on upgrade.
//
// The godoc on Md5 has been rewritten to warn callers away from every
// security use (passwords, signatures, tokens), but the test below
// locks the hex-digest-uppercase format as the observable contract.
// -----------------------------------------------------------------------
func TestMd5_ObservableContractUnchangedByDeprecation(t *testing.T) {
	// Fixed vectors derived from RFC 1321 / standard test inputs fed
	// through the existing Md5(data, salt) signature.
	//
	// The MD5 digest of "abc" is 900150983cd24fb0d6963f7d28e17f72; the
	// Md5 helper upper-cases and formats with %X. We also verify that a
	// non-empty salt changes the digest (simple differential test) and
	// that the function is deterministic across calls.
	const (
		input        = "abc"
		wantEmptyAlt = "900150983CD24FB0D6963F7D28E17F72"
	)

	// Contract 1: empty salt, known digest, uppercase hex.
	if got := Md5(input, ""); got != wantEmptyAlt {
		t.Errorf("Md5(%q, \"\") = %q, want %q — deprecated helper must "+
			"remain callable and produce the same digest it always has "+
			"through the entire v1.x series",
			input, got, wantEmptyAlt)
	}

	// Contract 2: non-empty salt produces a different digest.
	// (This is the bare-minimum sanity check that salt is actually
	// concatenated; it does NOT endorse MD5+salt as a password-hashing
	// construction, which the deprecation godoc explicitly forbids.)
	if salted := Md5(input, "salt"); salted == wantEmptyAlt {
		t.Errorf("Md5 ignored salt parameter: Md5(%q, %q) == Md5(%q, \"\")",
			input, "salt", input)
	}

	// Contract 3: deterministic — two consecutive calls return the
	// same value.
	a := Md5(input, "salt")
	b := Md5(input, "salt")
	if a != b {
		t.Errorf("Md5 non-deterministic: %q vs %q", a, b)
	}

	// Contract 4: 32-character uppercase hex digest. Any format change
	// (lowercase, base64, prefixing, truncation) would break downstream
	// consumers and therefore must be a v2.0.0 change.
	if len(a) != 32 {
		t.Errorf("Md5 digest length = %d, want 32", len(a))
	}
	for _, r := range a {
		isHex := (r >= '0' && r <= '9') || (r >= 'A' && r <= 'F')
		if !isHex {
			t.Errorf("Md5 digest %q contains non-uppercase-hex rune %q",
				a, r)
			break
		}
	}
}

// -----------------------------------------------------------------------
// P0-4 — AES-CBC observable-contract pin + NUL-padding hazard documentation.
//
// AesCbcEncrypt / AesCbcDecrypt are deprecated in v1.7.9 (scheduled for
// removal in v2.0.0) in favor of the already-existing AES-GCM pair which
// is authenticated (AEAD) and requires no padding. The CBC helpers remain
// CALLABLE and must produce the same output they always have for the
// remainder of the v1.x series or downstream consumers break on upgrade.
//
// The hazard: AesCbcEncrypt pads plaintext up to the next 16-byte block
// boundary with 0x00 bytes via util.Padding(..., ascii.NUL), and
// AesCbcDecrypt strips ALL trailing 0x00 bytes from the decrypted output
// via strings.ReplaceAll (crypto.go:~530). This means any plaintext whose
// natural last byte is 0x00 (or any plaintext containing embedded 0x00
// anywhere, since ReplaceAll is not limited to trailing bytes) will be
// silently corrupted on the round-trip. The three tests below pin the
// current behavior so a future "fix" (e.g., someone swapping to PKCS#7
// padding without coordinating a v2.0.0 release) is caught by the suite
// rather than by a downstream incident.
//
// Rule #10 (observable-contract stability): these tests pin the EXISTING
// behavior, warts and all. Changing the behavior requires a v2.0.0 major
// release with all 36+ consumers migrated in one coordinated batch.
// -----------------------------------------------------------------------
func TestAesCbc_DeprecationObservableContracts(t *testing.T) {
	const passphrase = "01234567890123456789012345678901" // 32-byte ASCII

	// Contract A: block-aligned plaintext (no padding path) round-trips cleanly.
	// This is the "happy path" that the existing TestCryptoRoundtrip_UnicodePassphrase
	// cbcPlaintext case also covers, repeated here so all P0-4 contracts live in one place.
	t.Run("block_aligned_roundtrip", func(t *testing.T) {
		const pt = "hello-cbc-block!" // exactly 16 bytes
		enc, err := AesCbcEncrypt(pt, passphrase)
		if err != nil {
			t.Fatalf("AesCbcEncrypt: %v", err)
		}
		dec, err := AesCbcDecrypt(enc, passphrase)
		if err != nil {
			t.Fatalf("AesCbcDecrypt: %v", err)
		}
		if dec != pt {
			t.Errorf("block-aligned round-trip:\n got: %q\nwant: %q", dec, pt)
		}
	})

	// Contract B: non-block-aligned plaintext WITHOUT trailing 0x00 round-trips
	// cleanly. AesCbcEncrypt pads with 0x00 bytes; AesCbcDecrypt strips them.
	// Because the plaintext has no legitimate trailing 0x00, the strip is a
	// clean no-op and the round-trip is exact.
	t.Run("non_block_aligned_no_trailing_nul_roundtrip", func(t *testing.T) {
		const pt = "short" // 5 bytes, padded to 16
		enc, err := AesCbcEncrypt(pt, passphrase)
		if err != nil {
			t.Fatalf("AesCbcEncrypt: %v", err)
		}
		dec, err := AesCbcDecrypt(enc, passphrase)
		if err != nil {
			t.Fatalf("AesCbcDecrypt: %v", err)
		}
		if dec != pt {
			t.Errorf("non-aligned round-trip:\n got: %q\nwant: %q", dec, pt)
		}
	})

	// Contract C: HAZARD PIN — plaintext whose last byte is legitimately 0x00
	// is CORRUPTED on round-trip because AesCbcDecrypt strips all trailing NULs.
	// This test pins the BUGGY behavior intentionally. If a future refactor
	// swaps to PKCS#7 padding, this test will break and force a conscious
	// decision about whether to coordinate a v2.0.0 release for the fix.
	//
	// This is NOT an endorsement of the current behavior — AesCbcEncrypt/Decrypt
	// are Deprecated: in godoc and callers should migrate to AesGcmEncrypt/Decrypt
	// which preserve trailing NULs exactly.
	t.Run("trailing_nul_hazard_pinned", func(t *testing.T) {
		// Plaintext: 4 bytes "data" + one 0x00 byte at the end. Total 5 bytes,
		// padded to 16 on encrypt, all trailing NULs stripped on decrypt.
		pt := "data" + string([]byte{0x00})

		enc, err := AesCbcEncrypt(pt, passphrase)
		if err != nil {
			t.Fatalf("AesCbcEncrypt: %v", err)
		}
		dec, err := AesCbcDecrypt(enc, passphrase)
		if err != nil {
			t.Fatalf("AesCbcDecrypt: %v", err)
		}

		// The deprecated helper corrupts: the trailing 0x00 is gone.
		if dec == pt {
			t.Errorf("AesCbcDecrypt preserved trailing NUL — this would be a "+
				"behavior CHANGE from v1.7.8 and must be coordinated as a "+
				"v2.0.0 release. Got %q == plaintext %q unexpectedly.",
				dec, pt)
		}
		if dec != "data" {
			t.Errorf("AesCbcDecrypt hazard-pin: got %q, want %q (all trailing "+
				"NULs stripped — deprecated behavior)", dec, "data")
		}

		// For the migration story: AES-GCM must preserve the exact byte
		// sequence including the trailing NUL. This is the differential
		// test that proves the migration target is correct.
		gcmEnc, err := AesGcmEncrypt(pt, passphrase)
		if err != nil {
			t.Fatalf("AesGcmEncrypt: %v", err)
		}
		gcmDec, err := AesGcmDecrypt(gcmEnc, passphrase)
		if err != nil {
			t.Fatalf("AesGcmDecrypt: %v", err)
		}
		if gcmDec != pt {
			t.Errorf("AesGcm round-trip must preserve trailing NUL exactly "+
				"(this is the whole point of the migration):\n got: %q\nwant: %q",
				gcmDec, pt)
		}
	})
}
