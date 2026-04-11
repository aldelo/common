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
	"sync"
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

// -----------------------------------------------------------------------
// P0-5 — RSA/AES envelope V2 (HMAC-keyed sibling pair) regression suite.
//
// RsaAesPublicKeyEncryptAndSignHmac / RsaAesPrivateKeyDecryptAndVerifyHmac
// are additive siblings to the deprecated V1 pair. The V1 wire format,
// behavior, and error messages are unchanged per workspace rule #10; the
// V2 pair is a wholly separate envelope that new callers should migrate
// to before v2.0.0 (when V1 is scheduled for removal).
//
// Coverage goals:
//
//	A. Happy path V2 round-trip with an ASCII plaintext.
//	B. Happy path V2 round-trip with a plaintext containing embedded
//	   VT (0x0B), NUL (0x00), and other control bytes — the exact
//	   category of inputs the V1 format corrupts.
//	C. V2 round-trip with a large plaintext (> 4 KiB) to exercise the
//	   length-prefix decoder past trivial sizes.
//	D. Cross-version isolation: a V1 envelope fed to the V2 decrypter
//	   is rejected (format marker check), and a V2 envelope fed to the
//	   V1 decrypter is rejected (V1's checks fail on the 'V2' literal).
//	E. HMAC tamper detection: flipping a single byte of the V2
//	   envelope body produces an integrity-check failure, not a
//	   successful decrypt of corrupted data.
//	F. Signature verification still runs on the V2 path (an envelope
//	   signed by one sender but presented as signed by another fails).
//
// Key generation is slow (~100 ms per 2048-bit RSA keypair); tests share
// keypairs via package-level sync.Once to keep the suite under a second.
// -----------------------------------------------------------------------

var (
	v2TestRecipientOnce sync.Once
	v2TestRecipientPriv string
	v2TestRecipientPub  string
	v2TestSenderPriv    string
	v2TestSenderPub     string
)

func v2TestKeys(t *testing.T) (recipPriv, recipPub, senderPriv, senderPub string) {
	t.Helper()
	v2TestRecipientOnce.Do(func() {
		var err error
		v2TestRecipientPriv, v2TestRecipientPub, err = RsaCreateKey()
		if err != nil {
			t.Fatalf("RsaCreateKey(recipient): %v", err)
		}
		v2TestSenderPriv, v2TestSenderPub, err = RsaCreateKey()
		if err != nil {
			t.Fatalf("RsaCreateKey(sender): %v", err)
		}
	})
	return v2TestRecipientPriv, v2TestRecipientPub, v2TestSenderPriv, v2TestSenderPub
}

func TestRsaAesHmac_V2RoundTripAsciiPlaintext(t *testing.T) {
	recipPriv, recipPub, senderPriv, senderPub := v2TestKeys(t)

	const pt = "hello V2 envelope — ASCII sanity check."

	env, err := RsaAesPublicKeyEncryptAndSignHmac(pt, recipPub, senderPub, senderPriv)
	if err != nil {
		t.Fatalf("RsaAesPublicKeyEncryptAndSignHmac: %v", err)
	}

	gotPlain, gotSenderPub, err := RsaAesPrivateKeyDecryptAndVerifyHmac(env, recipPriv)
	if err != nil {
		t.Fatalf("RsaAesPrivateKeyDecryptAndVerifyHmac: %v", err)
	}
	if gotPlain != pt {
		t.Errorf("plainText round-trip:\n got: %q\nwant: %q", gotPlain, pt)
	}
	if gotSenderPub != senderPub {
		t.Errorf("senderPublicKey round-trip mismatch — V2 envelope must "+
			"return the exact sender key that was passed in at encrypt time")
	}
}

func TestRsaAesHmac_V2RoundTripControlByteHostile(t *testing.T) {
	// This is the headline test for P0-5. A plaintext containing embedded
	// VT (0x0B), NUL (0x00), unit-separator (0x1F), and other control
	// bytes is EXACTLY what breaks the V1 format's strings.Split(VT)
	// inner parser. V2 uses length-prefixed fields and must round-trip
	// every byte exactly. If this test fails, the length-prefix framing
	// is broken — which would be a ship-gate regression.
	recipPriv, recipPub, senderPriv, senderPub := v2TestKeys(t)

	// Build a plaintext with every control byte 0x00..0x1F plus a VT
	// and a NUL specifically at tail and head positions.
	var hostile []byte
	hostile = append(hostile, 0x00)           // leading NUL
	hostile = append(hostile, 0x0B)           // leading VT
	hostile = append(hostile, []byte("abc")...)
	for b := byte(0x00); b < 0x20; b++ {
		hostile = append(hostile, b) // every control byte
	}
	hostile = append(hostile, []byte("汉字 😀")...)
	hostile = append(hostile, 0x0B)           // trailing VT
	hostile = append(hostile, 0x00)           // trailing NUL
	pt := string(hostile)

	env, err := RsaAesPublicKeyEncryptAndSignHmac(pt, recipPub, senderPub, senderPriv)
	if err != nil {
		t.Fatalf("RsaAesPublicKeyEncryptAndSignHmac: %v", err)
	}

	gotPlain, _, err := RsaAesPrivateKeyDecryptAndVerifyHmac(env, recipPriv)
	if err != nil {
		t.Fatalf("RsaAesPrivateKeyDecryptAndVerifyHmac: %v", err)
	}
	if gotPlain != pt {
		t.Errorf("control-byte-hostile round-trip mismatch — V2 length-"+
			"prefixed framing must preserve arbitrary bytes exactly:\n"+
			" got len=%d: %q\nwant len=%d: %q",
			len(gotPlain), gotPlain, len(pt), pt)
	}
}

func TestRsaAesHmac_V2RoundTripLargePlaintext(t *testing.T) {
	// ~4 KiB plaintext exercises the length-prefix decoder past the
	// range where a miscoded uint32 shift could accidentally still
	// work with small values.
	recipPriv, recipPub, senderPriv, senderPub := v2TestKeys(t)

	pt := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 100) // ~4.5 KiB

	env, err := RsaAesPublicKeyEncryptAndSignHmac(pt, recipPub, senderPub, senderPriv)
	if err != nil {
		t.Fatalf("RsaAesPublicKeyEncryptAndSignHmac: %v", err)
	}
	gotPlain, _, err := RsaAesPrivateKeyDecryptAndVerifyHmac(env, recipPriv)
	if err != nil {
		t.Fatalf("RsaAesPrivateKeyDecryptAndVerifyHmac: %v", err)
	}
	if gotPlain != pt {
		t.Errorf("large-plaintext round-trip mismatch:\n got len=%d\nwant len=%d",
			len(gotPlain), len(pt))
	}
}

func TestRsaAesHmac_V1V2CrossVersionIsolation(t *testing.T) {
	recipPriv, recipPub, senderPriv, senderPub := v2TestKeys(t)

	// Use a simple ASCII plaintext with NO VT bytes so V1 itself can
	// encrypt it cleanly.
	const pt = "cross-version isolation test"

	v1Env, err := RsaAesPublicKeyEncryptAndSign(pt, recipPub, senderPub, senderPriv)
	if err != nil {
		t.Fatalf("RsaAesPublicKeyEncryptAndSign (V1): %v", err)
	}

	v2Env, err := RsaAesPublicKeyEncryptAndSignHmac(pt, recipPub, senderPub, senderPriv)
	if err != nil {
		t.Fatalf("RsaAesPublicKeyEncryptAndSignHmac (V2): %v", err)
	}

	// V1 decrypter MUST reject V2 payload. (V2 starts with "V2" after
	// STX, and 'V' is not a hex char, so V1's RSA-decrypt of the first
	// 512 chars fails. The exact error message is not pinned here —
	// only the fact that an error is returned.)
	if _, _, err := RsaAesPrivateKeyDecryptAndVerify(v2Env, recipPriv); err == nil {
		t.Errorf("V1 decrypter accepted V2 payload — cross-version " +
			"isolation is broken. This must never happen.")
	}

	// V2 decrypter MUST reject V1 payload. Its format-marker check
	// fires as soon as STX is stripped and the first two bytes are
	// examined — they will be hex chars from V1's RSA-wrapped-key
	// segment, not "V2".
	if _, _, err := RsaAesPrivateKeyDecryptAndVerifyHmac(v1Env, recipPriv); err == nil {
		t.Errorf("V2 decrypter accepted V1 payload — cross-version " +
			"isolation is broken. This must never happen.")
	}
}

func TestRsaAesHmac_V2TamperDetection(t *testing.T) {
	recipPriv, recipPub, senderPriv, senderPub := v2TestKeys(t)

	const pt = "tamper detection test"

	env, err := RsaAesPublicKeyEncryptAndSignHmac(pt, recipPub, senderPub, senderPriv)
	if err != nil {
		t.Fatalf("RsaAesPublicKeyEncryptAndSignHmac: %v", err)
	}

	// Pick a byte deep in the AES-GCM body (well past the 512-char
	// RSA-wrapped key and inside the ciphertext) and flip it. The
	// envelope layout is:
	//   STX(1) + "V2"(2) + rsaKey(512) + body(variable) + hmac(64) + ETX(1)
	// so STX+V2+rsaKey = 515 bytes. Flip the char at offset 600, which
	// is guaranteed to be inside the GCM body for any reasonable
	// plaintext size.
	if len(env) < 700 {
		t.Fatalf("envelope unexpectedly short (%d bytes) — test assumption broken", len(env))
	}
	tampered := []byte(env)
	// toggle a hex char: 'a'<->'b', '0'<->'1', etc.
	switch tampered[600] {
	case 'a':
		tampered[600] = 'b'
	case 'b':
		tampered[600] = 'a'
	default:
		// any other byte: xor low bit so the hex char flips to an
		// adjacent hex char (if it's hex) or to a non-hex char
		// (which also fails decode — still a negative result).
		tampered[600] ^= 0x01
	}

	_, _, err = RsaAesPrivateKeyDecryptAndVerifyHmac(string(tampered), recipPriv)
	if err == nil {
		t.Errorf("tampered envelope decrypted cleanly — HMAC or GCM " +
			"integrity check failed to detect corruption. This is a " +
			"security-critical regression.")
	}
}

func TestRsaAesHmac_V2SignatureVerificationRunsOnV2Path(t *testing.T) {
	// Build an envelope signed by senderA, then try to decrypt and
	// present senderA's key — this should pass. Then tamper with
	// JUST the sender-public-key field inside the envelope so the
	// RSA verify step fails against the (still-valid) signature.
	//
	// The cleanest way to prove the V2 signature-verify path runs
	// without hand-crafting a tampered inner plaintext is to use
	// TWO different sender keypairs and prove that an envelope
	// signed by senderA cannot be forged to claim senderB signed
	// it. We do this at the encrypt-API level: re-sign with
	// senderA's private key but pass senderB's public key as the
	// purported sender — the decrypt-side RSA verify should then
	// fail because signatureA does not verify under publicKeyB.
	//
	// Strategy: encrypt with sender=A. Then take a SECOND envelope
	// encrypted with sender=B. The first envelope's signature was
	// built over plainText using senderA's private key. A third
	// party who swaps in senderB's public key cannot forge a new
	// signature (they don't have senderB's private key), so
	// anything they produce will fail verify. We therefore test
	// only the positive path (envelope signed and verified) and
	// trust that the verify call itself is wired up — its error
	// path is already covered by RsaPublicKeyVerify's own tests.
	recipPriv, recipPub, senderAPriv, senderAPub := v2TestKeys(t)

	// Generate a second sender keypair for this test.
	senderBPriv, senderBPub, err := RsaCreateKey()
	if err != nil {
		t.Fatalf("RsaCreateKey(senderB): %v", err)
	}
	_ = senderBPriv // unused; we never hold B's private key in this test

	const pt = "sig path test"

	// envA: signed by senderA, presented as senderA — must verify.
	envA, err := RsaAesPublicKeyEncryptAndSignHmac(pt, recipPub, senderAPub, senderAPriv)
	if err != nil {
		t.Fatalf("RsaAesPublicKeyEncryptAndSignHmac(A): %v", err)
	}
	_, gotSpk, err := RsaAesPrivateKeyDecryptAndVerifyHmac(envA, recipPriv)
	if err != nil {
		t.Fatalf("envA decrypt: %v", err)
	}
	if gotSpk != senderAPub {
		t.Errorf("envA returned sender=%q, want %q", gotSpk, senderAPub)
	}

	// envForged: try to encrypt with senderA's private key but
	// claim senderB is the sender. This is what an attacker with
	// access to senderA's private key (but not senderB's) would
	// try to do. The signature will be valid PKCS1v15 over pt
	// using senderA's private key, but RsaPublicKeyVerify will
	// check it against senderB's public key and fail.
	envForged, err := RsaAesPublicKeyEncryptAndSignHmac(pt, recipPub, senderBPub, senderAPriv)
	if err != nil {
		t.Fatalf("RsaAesPublicKeyEncryptAndSignHmac(forged): %v", err)
	}
	if _, _, err := RsaAesPrivateKeyDecryptAndVerifyHmac(envForged, recipPriv); err == nil {
		t.Errorf("forged envelope (signed by A, claiming B) decrypted " +
			"cleanly — V2 signature verification path is not wired " +
			"up. Security-critical regression.")
	}
}
