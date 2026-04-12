package ascii

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

// ---------------------------------------------------------------------------
// AsciiToString
// ---------------------------------------------------------------------------

func TestAsciiToString(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected string
	}{
		{"NUL", NUL, "\x00"},
		{"STX", STX, "\x02"},
		{"ETX", ETX, "\x03"},
		{"Space", SP, " "},
		{"LetterA", 65, "A"},
		{"LetterZ", 90, "Z"},
		{"Digit0", 48, "0"},
		{"DEL", DEL, "\x7f"},
		{"HighUnicode", 0x1F600, "\U0001F600"}, // emoji codepoint
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AsciiToString(tc.input)
			if got != tc.expected {
				t.Errorf("AsciiToString(%d) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetLRC
// ---------------------------------------------------------------------------

func TestGetLRC(t *testing.T) {
	t.Run("EmptyString_ReturnsError", func(t *testing.T) {
		_, err := GetLRC("")
		if err == nil {
			t.Error("expected error for empty string, got nil")
		}
	})

	t.Run("WhitespaceOnly_ReturnsError", func(t *testing.T) {
		_, err := GetLRC("   ")
		if err == nil {
			t.Error("expected error for whitespace-only string, got nil")
		}
	})

	t.Run("SingleChar_ReturnsError", func(t *testing.T) {
		_, err := GetLRC("A")
		if err == nil {
			t.Error("expected error for single char, got nil")
		}
	})

	t.Run("TwoChars_ReturnsXOR", func(t *testing.T) {
		// XOR of 'A' (0x41) and 'B' (0x42) = 0x03
		lrc, err := GetLRC("AB")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := string(rune(0x41 ^ 0x42))
		if lrc != expected {
			t.Errorf("GetLRC(\"AB\") = %q, want %q", lrc, expected)
		}
	})

	t.Run("ThreeChars_XORChain", func(t *testing.T) {
		// XOR chain: 'A'^'B'^'C' = 0x41^0x42^0x43 = 0x03^0x43 = 0x40
		lrc, err := GetLRC("ABC")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := string(rune(0x41 ^ 0x42 ^ 0x43))
		if lrc != expected {
			t.Errorf("GetLRC(\"ABC\") = %q, want %q", lrc, expected)
		}
	})

	t.Run("WithSTXPrefix_STXExcluded", func(t *testing.T) {
		// STX is stripped, so LRC is computed on "AB" only
		stx := AsciiToString(STX)
		lrcWithSTX, err := GetLRC(stx + "AB")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lrcWithout, err := GetLRC("AB")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if lrcWithSTX != lrcWithout {
			t.Errorf("STX prefix should be stripped: with=%q without=%q", lrcWithSTX, lrcWithout)
		}
	})

	t.Run("STXOnly_ReturnsError", func(t *testing.T) {
		// After stripping STX, only "" remains which is < 2 chars
		stx := AsciiToString(STX)
		_, err := GetLRC(stx + "A")
		if err == nil {
			t.Error("expected error when data after STX strip is single char")
		}
	})

	t.Run("Deterministic", func(t *testing.T) {
		// Same input always produces the same LRC
		for i := 0; i < 10; i++ {
			lrc, err := GetLRC("Hello")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			expected := string(rune('H' ^ 'e' ^ 'l' ^ 'l' ^ 'o'))
			if lrc != expected {
				t.Errorf("iteration %d: GetLRC(\"Hello\") = %q, want %q", i, lrc, expected)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// IsLRCValid
// ---------------------------------------------------------------------------

func TestIsLRCValid(t *testing.T) {
	t.Run("EmptyString", func(t *testing.T) {
		if IsLRCValid("") {
			t.Error("expected false for empty string")
		}
	})

	t.Run("SingleChar", func(t *testing.T) {
		if IsLRCValid("A") {
			t.Error("expected false for single char")
		}
	})

	t.Run("TwoChars", func(t *testing.T) {
		if IsLRCValid("AB") {
			t.Error("expected false for two chars (too short for meaningful validation)")
		}
	})

	t.Run("ValidLRC_ManuallyComputed", func(t *testing.T) {
		// Build: data = "AB", LRC of "AB" = 'A'^'B' = 0x03
		data := "AB"
		lrc, err := GetLRC(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		full := data + lrc
		if !IsLRCValid(full) {
			t.Errorf("expected valid LRC for %q", full)
		}
	})

	t.Run("InvalidLRC", func(t *testing.T) {
		// Append wrong LRC
		if IsLRCValid("ABX") {
			// Only valid if 'X' happens to be 'A'^'B' = 0x03, and 'X' is 0x58, so invalid
			t.Error("expected false for invalid LRC")
		}
	})

	t.Run("ValidWithSTXETX", func(t *testing.T) {
		// Use Envelop to create valid framed data, then verify
		enveloped, err := EnvelopWithStxEtxLrcWithError("TestData")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !IsLRCValid(enveloped) {
			t.Errorf("expected valid LRC for enveloped data %q", EscapeNonPrintable(enveloped))
		}
	})

	t.Run("CorruptedData_Invalid", func(t *testing.T) {
		enveloped, err := EnvelopWithStxEtxLrcWithError("TestData")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Corrupt one byte in the middle
		corrupted := enveloped[:3] + "X" + enveloped[4:]
		if IsLRCValid(corrupted) {
			t.Error("expected false for corrupted data")
		}
	})
}

// ---------------------------------------------------------------------------
// IsCreditCardMod10Valid (Luhn algorithm)
// ---------------------------------------------------------------------------

func TestIsCreditCardMod10Valid(t *testing.T) {
	t.Run("ValidCards", func(t *testing.T) {
		validCards := []struct {
			name   string
			number string
		}{
			{"Visa_Test", "4111111111111111"},
			{"MasterCard_Test", "5500000000000004"},
			{"Amex_Test", "378282246310005"},
			{"Discover_Test", "6011111111111117"},
			{"Visa_Alt", "4012888888881881"},
			{"MasterCard_Alt", "5105105105105100"},
		}

		for _, tc := range validCards {
			t.Run(tc.name, func(t *testing.T) {
				valid, err := IsCreditCardMod10Valid(tc.number)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !valid {
					t.Errorf("expected %s to be Luhn-valid", tc.number)
				}
			})
		}
	})

	t.Run("InvalidCards", func(t *testing.T) {
		invalidCards := []struct {
			name   string
			number string
		}{
			{"LastDigitOff", "4111111111111112"},
			{"RandomInvalid", "1234567890123456"},
			{"AllOnesInvalid", "1111111111111111"},
		}

		for _, tc := range invalidCards {
			t.Run(tc.name, func(t *testing.T) {
				valid, err := IsCreditCardMod10Valid(tc.number)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if valid {
					t.Errorf("expected %s to be Luhn-invalid", tc.number)
				}
			})
		}
	})

	t.Run("TooShort_ReturnsError", func(t *testing.T) {
		_, err := IsCreditCardMod10Valid("1234")
		if err == nil {
			t.Error("expected error for card number with fewer than 5 digits")
		}
	})

	t.Run("EmptyString_ReturnsError", func(t *testing.T) {
		_, err := IsCreditCardMod10Valid("")
		if err == nil {
			t.Error("expected error for empty string")
		}
	})

	t.Run("NonNumeric_ReturnsError", func(t *testing.T) {
		_, err := IsCreditCardMod10Valid("4111abcd11111111")
		if err == nil {
			t.Error("expected error for non-numeric input")
		}
	})

	t.Run("WithSpaces_TrimmedThenError", func(t *testing.T) {
		// Leading/trailing spaces are trimmed, but embedded spaces make it non-numeric
		_, err := IsCreditCardMod10Valid("4111 1111 1111 1111")
		if err == nil {
			t.Error("expected error for card number with embedded spaces")
		}
	})

	t.Run("FiveDigit_MinLength", func(t *testing.T) {
		// "00000" — Luhn check: 0 is valid mod 10
		valid, err := IsCreditCardMod10Valid("00000")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !valid {
			t.Error("expected 00000 to be Luhn-valid (sum=0, checkdigit=0)")
		}
	})

	t.Run("LeadingTrailingSpaces_Trimmed", func(t *testing.T) {
		valid, err := IsCreditCardMod10Valid("  4111111111111111  ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !valid {
			t.Error("expected trimmed card number to be valid")
		}
	})
}

// ---------------------------------------------------------------------------
// EnvelopWithStxEtxLrcWithError
// ---------------------------------------------------------------------------

func TestEnvelopWithStxEtxLrcWithError(t *testing.T) {
	t.Run("EmptyContent_ReturnsError", func(t *testing.T) {
		_, err := EnvelopWithStxEtxLrcWithError("")
		if err == nil {
			t.Error("expected error for empty content")
		}
	})

	t.Run("NormalContent_WrapsCorrectly", func(t *testing.T) {
		result, err := EnvelopWithStxEtxLrcWithError("Hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		stx := AsciiToString(STX)
		etx := AsciiToString(ETX)

		// Must start with STX
		if result[:1] != stx {
			t.Error("result should start with STX")
		}

		// Character before LRC must be ETX
		// result = STX + content + ETX + LRC
		if result[len(result)-2:len(result)-1] != etx {
			t.Error("second-to-last char should be ETX")
		}

		// LRC must be valid for the entire envelope
		if !IsLRCValid(result) {
			t.Error("enveloped data should have valid LRC")
		}
	})

	t.Run("ContentAlreadyHasSTX_NoDuplicate", func(t *testing.T) {
		stx := AsciiToString(STX)
		result, err := EnvelopWithStxEtxLrcWithError(stx + "Data")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should not have double STX
		if strings.HasPrefix(result, stx+stx) {
			t.Error("should not double-prefix STX")
		}
		if result[:1] != stx {
			t.Error("result should start with STX")
		}
	})

	t.Run("SingleChar_Envelops", func(t *testing.T) {
		result, err := EnvelopWithStxEtxLrcWithError("X")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		stx := AsciiToString(STX)
		etx := AsciiToString(ETX)

		if result[:1] != stx {
			t.Error("result should start with STX")
		}

		// Should contain ETX before LRC
		if !strings.Contains(result, etx) {
			t.Error("result should contain ETX")
		}

		if !IsLRCValid(result) {
			t.Error("enveloped data should have valid LRC")
		}
	})
}

// ---------------------------------------------------------------------------
// EnvelopWithStxEtxLrc (non-error variant)
// ---------------------------------------------------------------------------

func TestEnvelopWithStxEtxLrc(t *testing.T) {
	t.Run("EmptyContent_ReturnsBlank", func(t *testing.T) {
		result := EnvelopWithStxEtxLrc("")
		if result != "" {
			t.Errorf("expected empty string for empty content, got %q", result)
		}
	})

	t.Run("NormalContent_MatchesErrorVariant", func(t *testing.T) {
		resultNoErr := EnvelopWithStxEtxLrc("TestPayload")
		resultErr, err := EnvelopWithStxEtxLrcWithError("TestPayload")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resultNoErr != resultErr {
			t.Errorf("non-error variant %q differs from error variant %q",
				EscapeNonPrintable(resultNoErr), EscapeNonPrintable(resultErr))
		}
	})
}

// ---------------------------------------------------------------------------
// StripStxEtxLrcFromEnvelop
// ---------------------------------------------------------------------------

func TestStripStxEtxLrcFromEnvelop(t *testing.T) {
	t.Run("EmptyString_ReturnsEmpty", func(t *testing.T) {
		result := StripStxEtxLrcFromEnvelop("")
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("InvalidLRC_ReturnsEmpty", func(t *testing.T) {
		result := StripStxEtxLrcFromEnvelop("garbage")
		if result != "" {
			t.Errorf("expected empty string for invalid LRC data, got %q", result)
		}
	})

	t.Run("RoundTrip_RecoverOriginal", func(t *testing.T) {
		original := "PaymentData12345"
		enveloped, err := EnvelopWithStxEtxLrcWithError(original)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		stripped := StripStxEtxLrcFromEnvelop(enveloped)
		if stripped != original {
			t.Errorf("round-trip failed: got %q, want %q", stripped, original)
		}
	})

	t.Run("RoundTrip_SingleChar", func(t *testing.T) {
		original := "X"
		enveloped, err := EnvelopWithStxEtxLrcWithError(original)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		stripped := StripStxEtxLrcFromEnvelop(enveloped)
		if stripped != original {
			t.Errorf("round-trip failed for single char: got %q, want %q", stripped, original)
		}
	})

	t.Run("WithDelimiter_StripsLeftSide", func(t *testing.T) {
		original := "LeftPart|RightPart"
		enveloped, err := EnvelopWithStxEtxLrcWithError(original)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		stripped := StripStxEtxLrcFromEnvelop(enveloped, "|")
		if stripped != "RightPart" {
			t.Errorf("delimiter strip failed: got %q, want %q", stripped, "RightPart")
		}
	})

	t.Run("WithDelimiter_NoMatch_ReturnsAll", func(t *testing.T) {
		original := "NoDelimiter"
		enveloped, err := EnvelopWithStxEtxLrcWithError(original)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		stripped := StripStxEtxLrcFromEnvelop(enveloped, "|")
		if stripped != original {
			t.Errorf("no-match delimiter: got %q, want %q", stripped, original)
		}
	})

	t.Run("MultipleRoundTrips_Stable", func(t *testing.T) {
		payloads := []string{
			"Simple",
			"With Numbers 12345",
			"Special!@#$%",
			"abcdefghijklmnopqrstuvwxyz",
		}

		for _, p := range payloads {
			enveloped, err := EnvelopWithStxEtxLrcWithError(p)
			if err != nil {
				t.Fatalf("unexpected error enveloping %q: %v", p, err)
			}
			stripped := StripStxEtxLrcFromEnvelop(enveloped)
			if stripped != p {
				t.Errorf("round-trip failed for %q: got %q", p, stripped)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// ControlCharToWord / ControlCharToASCII round-trip
// ---------------------------------------------------------------------------

func TestControlCharToWord(t *testing.T) {
	t.Run("EmptyString", func(t *testing.T) {
		result := ControlCharToWord("")
		if result != "" {
			t.Errorf("expected empty, got %q", result)
		}
	})

	t.Run("NoControlChars_Unchanged", func(t *testing.T) {
		input := "Hello123"
		result := ControlCharToWord(input)
		if result != input {
			t.Errorf("expected %q unchanged, got %q", input, result)
		}
	})

	t.Run("STX_Converted", func(t *testing.T) {
		input := AsciiToString(STX)
		result := ControlCharToWord(input)
		if result != "[STX]" {
			t.Errorf("expected [STX], got %q", result)
		}
	})

	t.Run("ETX_Converted", func(t *testing.T) {
		input := AsciiToString(ETX)
		result := ControlCharToWord(input)
		if result != "[ETX]" {
			t.Errorf("expected [ETX], got %q", result)
		}
	})

	t.Run("MultipleControlChars", func(t *testing.T) {
		input := AsciiToString(STX) + "DATA" + AsciiToString(ETX)
		result := ControlCharToWord(input)
		if result != "[STX]DATA[ETX]" {
			t.Errorf("expected [STX]DATA[ETX], got %q", result)
		}
	})

	t.Run("AllMappedChars_TableDriven", func(t *testing.T) {
		// Test a representative set of control char mappings
		mappings := []struct {
			code int
			word string
		}{
			{NUL, "[NULL]"},
			{SOH, "[SOH]"},
			{STX, "[STX]"},
			{ETX, "[ETX]"},
			{EOT, "[EOT]"},
			{ENQ, "[ENQ]"},
			{ACK, "[ACK]"},
			{BEL, "[BEL]"},
			{BS, "[BS]"},
			{HT, "[HT]"},
			{LF, "[LF]"},
			{VT, "[VT]"},
			{FF, "[FF]"},
			{CR, "[CR]"},
			{SO, "[SO]"},
			{SI, "[SI]"},
			{DLE, "[DLE]"},
			{DC1, "[DC1]"},
			{DC2, "[DC2]"},
			{DC3, "[DC3]"},
			{DC4, "[DC4]"},
			{NAK, "[NAK]"},
			{SYN, "[SYN]"},
			{ETB, "[ETB]"},
			{CAN, "[CAN]"},
			{EM, "[EM]"},
			{SUB, "[SUB]"},
			{ESC, "[ESC]"},
			{FS, "[FS]"},
			{GS, "[GS]"},
			{RS, "[RS]"},
			{US, "[US]"},
			{SP, "[SP]"},
			{DEL, "[DEL]"},
			{COMMA, "[COMMA]"},
			{COLON, "[COLON]"},
			{PIPE, "[PIPE]"},
		}

		for _, m := range mappings {
			t.Run(m.word, func(t *testing.T) {
				input := AsciiToString(m.code)
				result := ControlCharToWord(input)
				if result != m.word {
					t.Errorf("ControlCharToWord(%d) = %q, want %q", m.code, result, m.word)
				}
			})
		}
	})

	t.Run("MixedContent", func(t *testing.T) {
		input := AsciiToString(ACK) + "OK" + AsciiToString(NAK)
		result := ControlCharToWord(input)
		expected := "[ACK]OK[NAK]"
		if result != expected {
			t.Errorf("got %q, want %q", result, expected)
		}
	})
}

func TestControlCharToASCII(t *testing.T) {
	t.Run("EmptyString", func(t *testing.T) {
		result := ControlCharToASCII("")
		if result != "" {
			t.Errorf("expected empty, got %q", result)
		}
	})

	t.Run("NoTags_Unchanged", func(t *testing.T) {
		input := "Hello123"
		result := ControlCharToASCII(input)
		if result != input {
			t.Errorf("expected %q unchanged, got %q", input, result)
		}
	})

	t.Run("STXTag_Converted", func(t *testing.T) {
		result := ControlCharToASCII("[STX]")
		if result != AsciiToString(STX) {
			t.Errorf("expected STX byte, got %q", result)
		}
	})

	t.Run("MultipleTags", func(t *testing.T) {
		result := ControlCharToASCII("[STX]DATA[ETX]")
		expected := AsciiToString(STX) + "DATA" + AsciiToString(ETX)
		if result != expected {
			t.Errorf("got %q, want %q", EscapeNonPrintable(result), EscapeNonPrintable(expected))
		}
	})
}

func TestControlCharRoundTrip(t *testing.T) {
	// Build a string with every mapped control char, convert to words, convert back
	// and verify we get the original. Note: some chars like COMMA (','), COLON (':'),
	// PIPE ('|'), and SP (' ') are printable but still have mappings.
	// We test them individually to confirm the round-trip.
	codes := []int{
		NUL, SOH, STX, ETX, EOT, ENQ, ACK, BEL, BS, HT, LF, VT, FF, CR,
		SO, SI, DLE, DC1, DC2, DC3, DC4, NAK, SYN, ETB, CAN, EM, SUB, ESC,
		FS, GS, RS, US, SP, DEL, COMMA, COLON, PIPE,
	}

	for _, code := range codes {
		original := AsciiToString(code)
		wordForm := ControlCharToWord(original)
		recovered := ControlCharToASCII(wordForm)
		if recovered != original {
			t.Errorf("round-trip failed for code 0x%02X: original=%q word=%q recovered=%q",
				code, original, wordForm, recovered)
		}
	}
}

// ---------------------------------------------------------------------------
// EscapeNonPrintable / UnescapeNonPrintable round-trip
// ---------------------------------------------------------------------------

func TestEscapeNonPrintable(t *testing.T) {
	t.Run("EmptyString", func(t *testing.T) {
		result := EscapeNonPrintable("")
		if result != "" {
			t.Errorf("expected empty, got %q", result)
		}
	})

	t.Run("PrintableOnly_Unchanged", func(t *testing.T) {
		input := "Hello World 123"
		result := EscapeNonPrintable(input)
		if result != input {
			t.Errorf("expected %q unchanged, got %q", input, result)
		}
	})

	t.Run("NUL_Escaped", func(t *testing.T) {
		result := EscapeNonPrintable(AsciiToString(NUL))
		if result != "[NUL_00]" {
			t.Errorf("expected [NUL_00], got %q", result)
		}
	})

	t.Run("STX_Escaped", func(t *testing.T) {
		result := EscapeNonPrintable(AsciiToString(STX))
		if result != "[STX_02]" {
			t.Errorf("expected [STX_02], got %q", result)
		}
	})

	t.Run("AllControlChars_0x00_to_0x1F", func(t *testing.T) {
		// Mapping of all 32 control chars (0x00 - 0x1F)
		escapeMappings := []struct {
			code    int
			escaped string
		}{
			{0x00, "[NUL_00]"},
			{0x01, "[SOH_01]"},
			{0x02, "[STX_02]"},
			{0x03, "[ETX_03]"},
			{0x04, "[EOT_04]"},
			{0x05, "[ENQ_05]"},
			{0x06, "[ACK_06]"},
			{0x07, "[BEL_07]"},
			{0x08, "[BS_08]"},
			{0x09, "[HT_09]"},
			{0x0A, "[LF_0A]"},
			{0x0B, "[VT_0B]"},
			{0x0C, "[FF_0C]"},
			{0x0D, "[CR_0D]"},
			{0x0E, "[SO_0E]"},
			{0x0F, "[SI_0F]"},
			{0x10, "[DLE_10]"},
			{0x11, "[DC1_11]"},
			{0x12, "[DC2_12]"},
			{0x13, "[DC3_13]"},
			{0x14, "[DC4_14]"},
			{0x15, "[NAK_15]"},
			{0x16, "[SYN_16]"},
			{0x17, "[ETB_17]"},
			{0x18, "[CAN_18]"},
			{0x19, "[EM_19]"},
			{0x1A, "[SUB_1A]"},
			{0x1B, "[ESC_1B]"},
			{0x1C, "[FS_1C]"},
			{0x1D, "[GS_1D]"},
			{0x1E, "[RS_1E]"},
			{0x1F, "[US_1F]"},
		}

		for _, m := range escapeMappings {
			input := AsciiToString(m.code)
			result := EscapeNonPrintable(input)
			if result != m.escaped {
				t.Errorf("EscapeNonPrintable(0x%02X) = %q, want %q", m.code, result, m.escaped)
			}
		}
	})

	t.Run("MixedContent", func(t *testing.T) {
		input := AsciiToString(STX) + "DATA" + AsciiToString(ETX)
		result := EscapeNonPrintable(input)
		expected := "[STX_02]DATA[ETX_03]"
		if result != expected {
			t.Errorf("got %q, want %q", result, expected)
		}
	})
}

func TestUnescapeNonPrintable(t *testing.T) {
	t.Run("EmptyString", func(t *testing.T) {
		result := UnescapeNonPrintable("")
		if result != "" {
			t.Errorf("expected empty, got %q", result)
		}
	})

	t.Run("NoEscapes_Unchanged", func(t *testing.T) {
		input := "Hello World 123"
		result := UnescapeNonPrintable(input)
		if result != input {
			t.Errorf("expected %q unchanged, got %q", input, result)
		}
	})

	t.Run("STX_Unescaped", func(t *testing.T) {
		result := UnescapeNonPrintable("[STX_02]")
		if result != AsciiToString(STX) {
			t.Errorf("expected STX byte, got %q", result)
		}
	})
}

func TestEscapeNonPrintableRoundTrip(t *testing.T) {
	// Build string with all control chars 0x00-0x1F, escape, unescape, verify identical
	var builder strings.Builder
	for i := 0; i <= 0x1F; i++ {
		builder.WriteRune(rune(i))
	}
	original := builder.String()

	escaped := EscapeNonPrintable(original)
	unescaped := UnescapeNonPrintable(escaped)

	if unescaped != original {
		t.Errorf("round-trip failed for all control chars 0x00-0x1F")
	}

	// The escaped form should not contain any actual control chars
	for _, r := range escaped {
		if r <= 0x1F {
			t.Errorf("escaped form still contains control char 0x%02X", r)
		}
	}
}

func TestEscapeRoundTrip_WithPrintableMix(t *testing.T) {
	// Mix of printable and non-printable
	original := AsciiToString(STX) + "Hello" + AsciiToString(FS) + "World" + AsciiToString(ETX)
	escaped := EscapeNonPrintable(original)
	unescaped := UnescapeNonPrintable(escaped)

	if unescaped != original {
		t.Errorf("round-trip failed: got %q, want %q",
			EscapeNonPrintable(unescaped), EscapeNonPrintable(original))
	}
}

// ---------------------------------------------------------------------------
// Full Envelope Round-Trip Integration
// ---------------------------------------------------------------------------

func TestFullEnvelopeRoundTrip(t *testing.T) {
	payloads := []struct {
		name string
		data string
	}{
		{"SimpleText", "Hello"},
		{"Numeric", "1234567890"},
		{"SpecialChars", "!@#$%^&*()_+-="},
		{"LongPayload", strings.Repeat("ABCDEFGHIJ", 100)},
		{"TwoChars", "AB"},
	}

	for _, tc := range payloads {
		t.Run(tc.name, func(t *testing.T) {
			// Envelop
			enveloped, err := EnvelopWithStxEtxLrcWithError(tc.data)
			if err != nil {
				t.Fatalf("envelop error: %v", err)
			}

			// Validate LRC
			if !IsLRCValid(enveloped) {
				t.Fatal("enveloped data should have valid LRC")
			}

			// Strip and recover
			stripped := StripStxEtxLrcFromEnvelop(enveloped)
			if stripped != tc.data {
				t.Errorf("round-trip failed: got %q, want %q", stripped, tc.data)
			}

			// Escape/unescape the enveloped form
			escaped := EscapeNonPrintable(enveloped)
			unescaped := UnescapeNonPrintable(escaped)
			if unescaped != enveloped {
				t.Error("escape/unescape round-trip of enveloped data failed")
			}

			// ControlChar word/ascii round-trip of enveloped form
			wordForm := ControlCharToWord(enveloped)
			asciiForm := ControlCharToASCII(wordForm)
			if asciiForm != enveloped {
				t.Error("control-char word/ascii round-trip of enveloped data failed")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Edge case: LRC computation consistency
// ---------------------------------------------------------------------------

func TestLRC_KnownValues(t *testing.T) {
	// Manually compute: "AB" => 'A'^'B' = 0x41^0x42 = 0x03
	lrc, err := GetLRC("AB")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lrc != string(rune(0x03)) {
		t.Errorf("LRC of 'AB' should be 0x03, got 0x%02X", lrc[0])
	}

	// "AA" => 'A'^'A' = 0x00
	lrc2, err := GetLRC("AA")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lrc2 != string(rune(0x00)) {
		t.Errorf("LRC of 'AA' should be 0x00, got 0x%02X", lrc2[0])
	}
}

// ---------------------------------------------------------------------------
// Edge case: IsCreditCardMod10Valid with uint64 overflow boundary
// ---------------------------------------------------------------------------

func TestIsCreditCardMod10Valid_LargeNumber(t *testing.T) {
	// 19-digit number that overflows uint64 (max ~18.4 * 10^18)
	_, err := IsCreditCardMod10Valid("99999999999999999999")
	if err == nil {
		t.Error("expected error for number exceeding uint64 range")
	}
}

// ---------------------------------------------------------------------------
// Edge case: StripStxEtxLrcFromEnvelop with multiple delimiters
// ---------------------------------------------------------------------------

func TestStripStxEtxLrcFromEnvelop_MultipleDelimiters(t *testing.T) {
	original := "A|B|C"
	enveloped, err := EnvelopWithStxEtxLrcWithError(original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// SplitN with limit 2 means only first occurrence is split on
	stripped := StripStxEtxLrcFromEnvelop(enveloped, "|")
	if stripped != "B|C" {
		t.Errorf("expected 'B|C', got %q", stripped)
	}
}
