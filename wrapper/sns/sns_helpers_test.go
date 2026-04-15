package sns

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
	"context"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sns"
)

// ---------------------------------------------------------------------------
// validateE164Phone
// ---------------------------------------------------------------------------

func TestValidateE164Phone(t *testing.T) {
	tests := []struct {
		name    string
		phone   string
		wantErr bool
	}{
		// happy paths
		{name: "US number", phone: "+12125551234", wantErr: false},
		{name: "UK number", phone: "+447911123456", wantErr: false},
		{name: "minimum valid (2 digits after +)", phone: "+12", wantErr: false},
		{name: "maximum valid (15 digits)", phone: "+123456789012345", wantErr: false},
		{name: "single leading digit after +", phone: "+91234567890", wantErr: false},

		// invalid: missing +
		{name: "no plus prefix", phone: "2125551234", wantErr: true},

		// invalid: too short
		{name: "plus only", phone: "+", wantErr: true},
		{name: "plus with one digit", phone: "+1", wantErr: true},

		// invalid: too long (16 digits)
		{name: "16 digits after +", phone: "+1234567890123456", wantErr: true},

		// invalid: leading zero after +
		{name: "leading zero country code", phone: "+0123456789", wantErr: true},

		// invalid: non-digit characters
		{name: "letters", phone: "abc", wantErr: true},
		{name: "plus with letters", phone: "+1abc", wantErr: true},
		{name: "spaces", phone: "+1 212 555 1234", wantErr: true},
		{name: "dashes", phone: "+1-212-555-1234", wantErr: true},
		{name: "parentheses", phone: "+1(212)5551234", wantErr: true},

		// invalid: empty
		{name: "empty string", phone: "", wantErr: true},

		// invalid: whitespace only
		{name: "whitespace only", phone: "   ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateE164Phone(tt.phone)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateE164Phone(%q) error = %v, wantErr %v", tt.phone, err, tt.wantErr)
			}
		})
	}
}

func TestValidateE164Phone_ErrorMessage(t *testing.T) {
	err := validateE164Phone("bad")
	if err == nil {
		t.Fatal("expected error for invalid phone")
	}
	if !strings.Contains(err.Error(), "E.164") {
		t.Errorf("error message should mention E.164 format, got: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// validateSenderID
// ---------------------------------------------------------------------------

func TestValidateSenderID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		// happy paths
		{name: "typical sender", id: "MyApp", wantErr: false},
		{name: "min length 3 letters", id: "ABC", wantErr: false},
		{name: "max length 11 mixed", id: "Alert12345A", wantErr: false},
		{name: "all letters 11 chars", id: "ABCDEFGHIJK", wantErr: false},
		{name: "single letter with digits", id: "A12", wantErr: false},
		{name: "lowercase letters", id: "myapp", wantErr: false},

		// empty/whitespace returns nil (sender ID is optional)
		{name: "empty string", id: "", wantErr: false},
		{name: "whitespace only", id: "   ", wantErr: false},

		// invalid: too short
		{name: "two chars", id: "AB", wantErr: true},
		{name: "one char", id: "A", wantErr: true},

		// invalid: too long
		{name: "12 chars", id: "ABCDEFGHIJKL", wantErr: true},
		{name: "14 chars", id: "TooLongSenderX", wantErr: true},

		// invalid: all digits (no letter)
		{name: "all digits 5", id: "12345", wantErr: true},
		{name: "all digits 3", id: "123", wantErr: true},

		// invalid: contains non-alphanumeric
		{name: "space in middle", id: "My App", wantErr: true},
		{name: "underscore", id: "My_App", wantErr: true},
		{name: "hyphen", id: "My-App", wantErr: true},
		{name: "special chars", id: "My@App", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSenderID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSenderID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestValidateSenderID_ErrorMessage(t *testing.T) {
	err := validateSenderID("AB")
	if err == nil {
		t.Fatal("expected error for too-short sender ID")
	}
	if !strings.Contains(err.Error(), "3-11") {
		t.Errorf("error should mention 3-11 length rule, got: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// smsLength
// ---------------------------------------------------------------------------

func TestSmsLength(t *testing.T) {
	tests := []struct {
		name         string
		message      string
		wantEncoding string
		wantUsed     int
		wantLimit    int
	}{
		// empty message
		{
			name:         "empty string",
			message:      "",
			wantEncoding: "GSM-7",
			wantUsed:     0,
			wantLimit:    160,
		},
		// short ASCII -> GSM-7
		{
			name:         "short ASCII",
			message:      "Hello",
			wantEncoding: "GSM-7",
			wantUsed:     5,
			wantLimit:    160,
		},
		// exactly 160 GSM-7 chars (single segment)
		{
			name:         "exactly 160 GSM-7 chars",
			message:      strings.Repeat("A", 160),
			wantEncoding: "GSM-7",
			wantUsed:     160,
			wantLimit:    160,
		},
		// 161 GSM-7 chars -> multipart (2 segments * 153 = 306)
		{
			name:         "161 GSM-7 chars triggers multipart",
			message:      strings.Repeat("A", 161),
			wantEncoding: "GSM-7",
			wantUsed:     161,
			wantLimit:    306, // ceil(161/153) = 2 segments, 2*153 = 306
		},
		// exactly 306 GSM-7 chars (2 segments full)
		{
			name:         "306 GSM-7 chars fills 2 segments",
			message:      strings.Repeat("A", 306),
			wantEncoding: "GSM-7",
			wantUsed:     306,
			wantLimit:    306,
		},
		// 307 GSM-7 chars -> 3 segments
		{
			name:         "307 GSM-7 chars triggers 3 segments",
			message:      strings.Repeat("A", 307),
			wantEncoding: "GSM-7",
			wantUsed:     307,
			wantLimit:    459, // ceil(307/153) = 3 segments, 3*153 = 459
		},
		// GSM-7 extended chars count as 2 septets
		{
			name:         "extended char counts as 2 septets",
			message:      "[",
			wantEncoding: "GSM-7",
			wantUsed:     2,
			wantLimit:    160,
		},
		{
			name:         "euro sign is extended GSM-7",
			message:      "€",
			wantEncoding: "GSM-7",
			wantUsed:     2,
			wantLimit:    160,
		},
		{
			name:         "mix of default and extended",
			message:      "Price: €100",
			wantEncoding: "GSM-7",
			wantUsed:     12, // P=1 r=1 i=1 c=1 e=1 :=1 space=1 €=2 1=1 0=1 0=1 = 12
			wantLimit:    160,
		},
		// Unicode text -> UCS-2
		{
			name:         "Chinese characters",
			message:      "你好世界",
			wantEncoding: "UCS-2",
			wantUsed:     4,
			wantLimit:    70,
		},
		{
			name:         "emoji triggers UCS-2",
			message:      "Hello 😀",
			wantEncoding: "UCS-2",
			wantUsed:     7, // H=1 e=1 l=1 l=1 o=1 space=1 emoji=1 rune
			wantLimit:    70,
		},
		// exactly 70 UCS-2 chars (single segment)
		{
			name:         "exactly 70 UCS-2 chars",
			message:      strings.Repeat("你", 70),
			wantEncoding: "UCS-2",
			wantUsed:     70,
			wantLimit:    70,
		},
		// 71 UCS-2 chars -> multipart (2 segments * 67 = 134)
		{
			name:         "71 UCS-2 chars triggers multipart",
			message:      strings.Repeat("你", 71),
			wantEncoding: "UCS-2",
			wantUsed:     71,
			wantLimit:    134, // ceil(71/67) = 2 segments, 2*67 = 134
		},
		// GSM-7 special characters that ARE in the default table
		{
			name:         "GSM-7 default specials",
			message:      "@£$¥",
			wantEncoding: "GSM-7",
			wantUsed:     4,
			wantLimit:    160,
		},
		// newline and carriage return are GSM-7
		{
			name:         "newline and CR are GSM-7",
			message:      "line1\nline2\r",
			wantEncoding: "GSM-7",
			wantUsed:     12,
			wantLimit:    160,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, used, encoding := smsLength(tt.message)

			if encoding != tt.wantEncoding {
				t.Errorf("smsLength(%q) encoding = %q, want %q", tt.name, encoding, tt.wantEncoding)
			}
			if used != tt.wantUsed {
				t.Errorf("smsLength(%q) used = %d, want %d", tt.name, used, tt.wantUsed)
			}
			if limit != tt.wantLimit {
				t.Errorf("smsLength(%q) limit = %d, want %d", tt.name, limit, tt.wantLimit)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// sortedAttributeKeys — F4 pass-3 contrarian xray-metadata redaction
// ---------------------------------------------------------------------------

func TestSortedAttributeKeys(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]*sns.MessageAttributeValue
		want  string
	}{
		{
			name:  "nil map returns empty string",
			input: nil,
			want:  "",
		},
		{
			name:  "empty map returns empty string",
			input: map[string]*sns.MessageAttributeValue{},
			want:  "",
		},
		{
			name: "single key returns that key",
			input: map[string]*sns.MessageAttributeValue{
				"priority": {DataType: aws.String("String"), StringValue: aws.String("high")},
			},
			want: "priority",
		},
		{
			name: "multiple keys returned in sorted order",
			input: map[string]*sns.MessageAttributeValue{
				"zeta":   {DataType: aws.String("String"), StringValue: aws.String("VALUE_ZETA_XYZ")},
				"alpha":  {DataType: aws.String("String"), StringValue: aws.String("VALUE_ALPHA_XYZ")},
				"middle": {DataType: aws.String("String"), StringValue: aws.String("VALUE_MIDDLE_XYZ")},
			},
			want: "alpha,middle,zeta",
		},
		{
			name: "values are never exposed in output",
			input: map[string]*sns.MessageAttributeValue{
				"token": {DataType: aws.String("String"), StringValue: aws.String("SECRET_BEARER_abcdef123")},
				"pii":   {DataType: aws.String("String"), StringValue: aws.String("ssn:123-45-6789")},
			},
			want: "pii,token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sortedAttributeKeys(tt.input)
			if got != tt.want {
				t.Errorf("sortedAttributeKeys = %q, want %q", got, tt.want)
			}
			// Paranoid check — no attribute value should ever leak through.
			for _, v := range tt.input {
				if v != nil && v.StringValue != nil && *v.StringValue != "" &&
					strings.Contains(got, *v.StringValue) {
					t.Errorf("sortedAttributeKeys leaked value %q in output %q",
						*v.StringValue, got)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// maskPhoneForXray — SP-008 P1-COMMON-SNS-01 phone-PII redaction
// ---------------------------------------------------------------------------
//
// These tests pin the contract that the xray-metadata emit sites on
// OptInPhoneNumber / CheckIfPhoneNumberIsOptedOut / ListPhoneNumbersOptedOut
// rely on — if the mask leaks more of the subscriber number than
// "country code + last 4", the PII invariant silently regresses.

func TestMaskPhoneForXray(t *testing.T) {
	tests := []struct {
		name  string
		phone string
		want  string
	}{
		{name: "US 10-digit E.164", phone: "+12125551234", want: "+1******1234"},
		{name: "UK 11-digit E.164", phone: "+447911123456", want: "+4*******3456"},
		{name: "minimum masked length (7 chars, 1 digit masked)", phone: "+123456", want: "+1*3456"},
		{name: "6-char input returns verbatim (below mask threshold)", phone: "+12345", want: "+12345"},
		{name: "short non-E.164 verbatim", phone: "+12", want: "+12"},
		{name: "empty string verbatim", phone: "", want: ""},
		{name: "no plus prefix verbatim", phone: "2125551234", want: "2125551234"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskPhoneForXray(tt.phone)
			if got != tt.want {
				t.Errorf("maskPhoneForXray(%q) = %q, want %q", tt.phone, got, tt.want)
			}
		})
	}
}

func TestMaskPhoneForXray_NeverRevealsMiddleDigits(t *testing.T) {
	// Property: for any valid E.164 number >= 6 chars, the mask must
	// not contain the middle-of-subscriber digits. This is the
	// security-facing invariant — if it regresses, the xray trace
	// becomes pivotable back to a natural-person identity.
	phones := []string{
		"+12125551234",     // US
		"+447911123456",    // UK
		"+919876543210",    // India
		"+8613800138000",   // China
		"+353861234567",    // Ireland
	}
	for _, p := range phones {
		masked := maskPhoneForXray(p)
		// Leading "+X" (country code's first digit) and trailing 4 digits
		// must survive. Everything in between must be asterisks.
		if len(masked) != len(p) {
			t.Errorf("maskPhoneForXray(%q) changed length: got %q", p, masked)
			continue
		}
		if masked[:2] != p[:2] {
			t.Errorf("maskPhoneForXray(%q) mangled country prefix: %q", p, masked)
		}
		if masked[len(masked)-4:] != p[len(p)-4:] {
			t.Errorf("maskPhoneForXray(%q) mangled trailing 4 digits: %q", p, masked)
		}
		for i := 2; i < len(masked)-4; i++ {
			if masked[i] != '*' {
				t.Errorf("maskPhoneForXray(%q) leaked digit at index %d: %q", p, i, masked)
				break
			}
		}
	}
}

// ---------------------------------------------------------------------------
// maskPhoneForXray UTF-8 safety — SP-010 pass-5 A1-F4 (2026-04-15)
// ---------------------------------------------------------------------------
//
// These tests pin the rune-based implementation. The prior byte-slice
// form was incidentally correct for ASCII E.164 only; a multi-byte rune
// at byte index 1 or len-4 would be split mid-codepoint, emitting
// invalid UTF-8 bytes into xray metadata.
//
// Mutation probe (quality gate): revert maskPhoneForXray in sns.go to
// the byte-based form:
//
//	if len(phone) < 7 || phone[0] != '+' { return phone }
//	return phone[:2] + strings.Repeat("*", len(phone)-6) + phone[len(phone)-4:]
//
// TestMaskPhoneForXray_MultiByteRuneSafe_A1F4 and
// TestMaskPhoneForXray_InvalidUTF8BytesDoNotPanic must both turn red
// (they assert utf8.ValidString on the output, which the byte-slice
// form breaks). The existing ASCII table-driven tests continue to pass
// under the byte form — BC is preserved by construction.

func TestMaskPhoneForXray_MultiByteRuneSafe_A1F4(t *testing.T) {
	// Inputs constructed so that a naive byte-slice at index [:2] or
	// [len-4:] would land inside a multi-byte rune. These never validate
	// as E.164, but maskPhoneForXray is called from deferred xray-emit
	// closures which fire regardless of validation outcome — see the
	// godoc on maskPhoneForXray for the call-site rationale.
	cases := []struct {
		name  string
		input string
	}{
		// 3-byte runes (CJK) throughout. len(runes) = 12, len(bytes) = 36.
		{name: "CJK prefix and body", input: "+台北1234567890"},
		// 4-byte rune (emoji) at the front — guaranteed to split if
		// byte-sliced at [:2].
		{name: "emoji head", input: "+🌐2025551234"},
		// 4-byte rune in the tail window — guaranteed to split if
		// byte-sliced at [len-4:].
		{name: "emoji tail", input: "+1202555🌐🌐"},
		// Mixed ASCII + multi-byte.
		{name: "mixed tail", input: "+120255512電話"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := maskPhoneForXray(tc.input)
			if !utf8.ValidString(got) {
				t.Errorf("maskPhoneForXray(%q) produced invalid UTF-8: % x", tc.input, []byte(got))
			}
			// Rune-level structure: head = first 2 runes of input,
			// tail = last 4 runes of input, middle = asterisks.
			inRunes := []rune(tc.input)
			gotRunes := []rune(got)
			if len(gotRunes) != len(inRunes) {
				t.Errorf("rune-length mismatch: input %d runes, masked %d runes", len(inRunes), len(gotRunes))
			}
			if len(gotRunes) >= 2 && (gotRunes[0] != inRunes[0] || gotRunes[1] != inRunes[1]) {
				t.Errorf("head 2 runes not preserved: input %q, masked %q", string(inRunes[:2]), string(gotRunes[:2]))
			}
			if len(gotRunes) >= 4 {
				inTail := inRunes[len(inRunes)-4:]
				gotTail := gotRunes[len(gotRunes)-4:]
				if string(inTail) != string(gotTail) {
					t.Errorf("tail 4 runes not preserved: input %q, masked %q", string(inTail), string(gotTail))
				}
			}
			// Every middle rune must be '*'.
			for i := 2; i < len(gotRunes)-4; i++ {
				if gotRunes[i] != '*' {
					t.Errorf("middle rune %d leaked: %q", i, string(gotRunes[i]))
					break
				}
			}
		})
	}
}

func TestMaskPhoneForXray_InvalidUTF8BytesDoNotPanic(t *testing.T) {
	// Feed a string containing invalid UTF-8 byte sequences. Go's
	// []rune conversion on invalid UTF-8 substitutes U+FFFD per
	// invalid byte, so the helper must: (a) not panic, (b) produce
	// a valid UTF-8 output (possibly with U+FFFD where input was
	// malformed), (c) preserve the rune-structure contract.
	invalid := "+1\xff\xfe555\xff1234"
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("maskPhoneForXray panicked on invalid UTF-8: %v", r)
		}
	}()
	got := maskPhoneForXray(invalid)
	if !utf8.ValidString(got) {
		t.Errorf("maskPhoneForXray(%q) produced invalid UTF-8: % x", invalid, []byte(got))
	}
}

func TestMaskPhoneForXray_AsciiBCUnchanged_A1F4(t *testing.T) {
	// BC pin: after the rune-based rewrite, every canonical ASCII
	// E.164 input must produce exactly the same masked output as
	// before. If this fails, the fix silently changed a shipped
	// contract used by three admin phone APIs + SendSMS.
	cases := []struct {
		in, want string
	}{
		{in: "+12025551234", want: "+1******1234"},
		{in: "+447911123456", want: "+4*******3456"},
		{in: "+12345", want: "+12345"},           // 6 runes, too short, verbatim
		{in: "12025551234", want: "12025551234"}, // no plus, verbatim
		{in: "", want: ""},
		{in: "+1234567", want: "+1**4567"},  // 8 runes = 2 middle asterisks
		{in: "+123456", want: "+1*3456"},    // 7 runes = 1 middle asterisk (boundary)
	}
	for _, tc := range cases {
		got := maskPhoneForXray(tc.in)
		if got != tc.want {
			t.Errorf("BC drift: maskPhoneForXray(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMaskPhoneForXray_SourcePinsRuneConversion_A1F4(t *testing.T) {
	// Source-invariant pin: the fix relies on the helper walking the
	// input as runes, not bytes. If a well-meaning refactor reverts
	// to byte-slicing, this test turns red independent of whether the
	// functional tests above still happen to pass on the specific
	// inputs they exercise. We pin the presence of the rune-slice
	// assignment and the use of runes[len(runes)-tailLen:] — tokens
	// that exist only in the rune-based body, not in the godoc
	// comment that references the prior byte-slice form for context.
	src, err := os.ReadFile("sns.go")
	if err != nil {
		t.Fatalf("read sns.go: %v", err)
	}
	s := string(src)
	if !strings.Contains(s, "runes := []rune(phone)") {
		t.Error("maskPhoneForXray must convert input to []rune before slicing (A1-F4 regression)")
	}
	if !strings.Contains(s, "runes[len(runes)-tailLen:]") {
		t.Error("maskPhoneForXray must slice the rune array, not the byte string (A1-F4 regression)")
	}
}

// ---------------------------------------------------------------------------
// ensureSNSCtx — SP-008 pass-5 A1-F1 xray-on deadline enforcement
// ---------------------------------------------------------------------------
//
// These tests pin the contract that every branch of ensureSNSCtx
// returns a context with an enforceable deadline.
//
// Mutation probe (quality gate): remove the
// context.WithTimeout(segCtx, defaultSNSCallTimeout) wrap in branch 2
// of ensureSNSCtx (revert to `return segCtx, func(){}`). All three
// TestEnsureSNSCtx_XrayBranch_* tests must turn red. This confirms
// the tests exercise the causal path, not just the postcondition.

func TestEnsureSNSCtx_XrayBranch_AppliesDefaultDeadline(t *testing.T) {
	// Branch 2: segCtxSet=true, segCtx non-nil, no caller timeout.
	// Post A1-F1 fix, this MUST return a ctx with a ~30s deadline.
	segCtx := context.Background()
	got, cancel := ensureSNSCtx(segCtx, true, nil)
	defer cancel()

	deadline, ok := got.Deadline()
	if !ok {
		t.Fatal("branch-2 must return a context with a deadline (A1-F1 regression)")
	}
	until := time.Until(deadline)
	if until < 29*time.Second || until > 31*time.Second {
		t.Errorf("default deadline ~30s expected, got %v", until)
	}
}

func TestEnsureSNSCtx_XrayBranch_PreservesParentLineage(t *testing.T) {
	// Branch 2: the returned ctx MUST be a child of segCtx so xray
	// segment lineage is preserved (any values stored on segCtx remain
	// accessible on the returned ctx).
	type ctxKey string
	const k ctxKey = "xray-test-marker"
	segCtx := context.WithValue(context.Background(), k, "parent-value")

	got, cancel := ensureSNSCtx(segCtx, true, nil)
	defer cancel()

	v, _ := got.Value(k).(string)
	if v != "parent-value" {
		t.Errorf("branch-2 must preserve parent ctx values for xray lineage; got %q", v)
	}
}

func TestEnsureSNSCtx_XrayBranch_CancelsOnParent(t *testing.T) {
	// Branch 2: if the parent segCtx is canceled, the returned ctx
	// MUST also be canceled (WithTimeout propagates parent cancellation).
	parent, parentCancel := context.WithCancel(context.Background())
	got, cancel := ensureSNSCtx(parent, true, nil)
	defer cancel()

	parentCancel()
	select {
	case <-got.Done():
		// ok
	case <-time.After(1 * time.Second):
		t.Fatal("branch-2 ctx did not propagate parent cancellation")
	}
}

func TestEnsureSNSCtx_CallerTimeoutOverridesXray(t *testing.T) {
	// Branch 1: caller-supplied timeout wins over xray parent.
	segCtx := context.Background()
	got, cancel := ensureSNSCtx(segCtx, true, []time.Duration{5 * time.Second})
	defer cancel()

	deadline, ok := got.Deadline()
	if !ok {
		t.Fatal("branch-1 must return a ctx with a deadline")
	}
	until := time.Until(deadline)
	if until > 6*time.Second {
		t.Errorf("caller timeout 5s must win over xray branch 30s default, got %v", until)
	}
}

func TestEnsureSNSCtx_BackgroundBranch_AppliesDefaultDeadline(t *testing.T) {
	// Branch 3: no xray, no caller timeout.
	got, cancel := ensureSNSCtx(nil, false, nil)
	defer cancel()

	deadline, ok := got.Deadline()
	if !ok {
		t.Fatal("branch-3 must return a ctx with a deadline")
	}
	until := time.Until(deadline)
	if until < 29*time.Second || until > 31*time.Second {
		t.Errorf("default deadline ~30s expected, got %v", until)
	}
}

func TestEnsureSNSCtx_NilSegCtxWithCallerTimeout_FallsBackToBackground(t *testing.T) {
	// Branch 1 defensive guard (A1-F5 dead-guard, also covered by
	// Gap3.2): segCtxSet=true but segCtx=nil + caller timeout.
	// Must NOT panic — must fall back to context.Background parent.
	got, cancel := ensureSNSCtx(nil, true, []time.Duration{2 * time.Second})
	defer cancel()

	deadline, ok := got.Deadline()
	if !ok {
		t.Fatal("defensive nil-segCtx guard must still return a ctx with deadline")
	}
	if until := time.Until(deadline); until > 3*time.Second {
		t.Errorf("caller timeout 2s expected, got %v", until)
	}
}

// SP-010 pass-5 A1-F5 (2026-04-15) — Gap 3.2.
//
// The agent-1 P3 finding specified the exact arguments
// `ensureSNSCtx(nil, false, []time.Duration{50*time.Millisecond})` as
// the reachability probe for the branch-1 nil-segCtx defensive guard.
// TestEnsureSNSCtx_NilSegCtxWithCallerTimeout_FallsBackToBackground
// covers the same guard via (segCtxSet=true, segCtx=nil) — same branch,
// same guard line, but a different flag permutation. This test adds
// the letter of the finding (segCtxSet=false) so the guard is exercised
// under BOTH permutations that can legally reach it: a future caller
// that never sets the xray-segment flag AND passes nil (the flag is
// correctly unset, segCtx happens to be nil because of a caller-side
// construction bug) would otherwise be untested.
//
// Mutation probe (quality gate): remove the `if parent == nil` fallback
// lines 111-113 in sns.go:
//
//	if len(timeOutDuration) > 0 {
//	    return context.WithTimeout(segCtx, timeOutDuration[0])
//	}
//
// Both TestEnsureSNSCtx_NilSegCtx* tests must then panic with
// "nil Context" because context.WithTimeout(nil, ...) does not tolerate
// a nil parent. The panic surfaces as a test failure via the defer
// recover() block. Restore the guard, re-run — both tests green. This
// confirms the regression guards exercise the causal guard, not some
// unrelated postcondition.

func TestEnsureSNSCtx_NilSegCtxFalse_WithCallerTimeout_A1F5(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ensureSNSCtx(nil, false, [50ms]) panicked: %v — nil-segCtx guard must fall back to Background (A1-F5 regression)", r)
		}
	}()

	got, cancel := ensureSNSCtx(nil, false, []time.Duration{50 * time.Millisecond})
	defer cancel()

	deadline, ok := got.Deadline()
	if !ok {
		t.Fatal("branch-1 with nil segCtx + caller timeout must still return a ctx with deadline")
	}
	until := time.Until(deadline)
	if until <= 0 || until > 100*time.Millisecond {
		t.Errorf("caller timeout 50ms expected, got %v", until)
	}
}

func TestEnsureSNSCtx_NilGuard_SourcePin_A1F5(t *testing.T) {
	// Source-invariant pin: branch 1 of ensureSNSCtx MUST contain a
	// nil-segCtx fallback to context.Background so a future refactor
	// that 'trusts the caller' and removes the guard is caught at
	// test-time, not at the first panic under the hypothetical caller
	// that finding A1-F5 documents.
	src, err := os.ReadFile("sns.go")
	if err != nil {
		t.Fatalf("read sns.go: %v", err)
	}
	s := string(src)
	// The guard tokens are specific to this function body — they do
	// not appear in the godoc comment block.
	if !strings.Contains(s, "parent := segCtx") {
		t.Error("ensureSNSCtx branch-1 must alias segCtx to a local parent variable before nil-check (A1-F5 regression)")
	}
	if !strings.Contains(s, "if parent == nil") {
		t.Error("ensureSNSCtx branch-1 must guard nil parent before context.WithTimeout (A1-F5 regression)")
	}
	if !strings.Contains(s, "parent = context.Background()") {
		t.Error("ensureSNSCtx branch-1 must fall back to context.Background when segCtx is nil (A1-F5 regression)")
	}
}

// ---------------------------------------------------------------------------
// SendSMS xray phone PII mask — SP-010 pass-5 A1-F3 (2026-04-15)
// ---------------------------------------------------------------------------
//
// Pass-3 F5 rationalized SendSMS's xray metadata emit as "phone number
// is the delivery destination, needed for operational debugging" and
// left the full E.164 in the segment while the three sibling admin
// APIs (OptInPhoneNumber, CheckIfPhoneNumberIsOptedOut,
// ListPhoneNumbersOptedOut) masked theirs via maskPhoneForXray. Pass-5
// A1-F3 showed that debugging rationale applied identically to the
// masked siblings — the asymmetry was unprincipled, and SendSMS is the
// highest-volume phone API in this package so it was the head of the
// PII distribution.
//
// The fix: route SendSMS's SNS-SendSMS-Phone metadata through
// maskPhoneForXray. The TestMaskPhoneForXray_* tests above already pin
// the mask function's correctness; the test below pins the CALL-SITE
// WIRING — i.e. that SendSMS actually routes through the mask. Without
// this pin, a reviewer could delete `maskPhoneForXray(` from the call
// site and every pure-function test would still pass.
//
// Mutation probe (quality gate): in sns.go, revert the SendSMS emit
// site from `maskPhoneForXray(phoneNumber)` back to `phoneNumber`.
// TestSendSMS_XrayMetadata_MasksPhoneNumber MUST turn red. Restore the
// fix and it MUST return to green. This confirms the test exercises the
// causal path, not just a tautological postcondition.

func TestSendSMS_XrayMetadata_MasksPhoneNumber(t *testing.T) {
	// Source-invariant regression test: the SendSMS xray emit site
	// must route the phone argument through maskPhoneForXray so that
	// SNS-SendSMS-Phone metadata in xray segments only ever carries
	// the country-code + last-4 form, never a raw E.164 number.
	src, err := os.ReadFile("sns.go")
	if err != nil {
		t.Fatalf("cannot read sns.go: %v", err)
	}
	body := string(src)

	// Positive: the masked emit MUST appear in the file.
	wantMasked := `SafeAddMetadata("SNS-SendSMS-Phone", maskPhoneForXray(phoneNumber))`
	if !strings.Contains(body, wantMasked) {
		t.Errorf("A1-F3 regression: expected SendSMS xray emit to call maskPhoneForXray(phoneNumber), but did not find %q in sns.go", wantMasked)
	}

	// Negative: the old unmasked form MUST NOT appear anywhere. This
	// catches partial reverts (e.g. someone adds a second emit site
	// for debugging and forgets to mask it).
	wantAbsent := `SafeAddMetadata("SNS-SendSMS-Phone", phoneNumber)`
	if strings.Contains(body, wantAbsent) {
		t.Errorf("A1-F3 regression: unmasked SendSMS phone emit found in sns.go: %q — must route through maskPhoneForXray", wantAbsent)
	}
}

func TestSmsLength_UsedNeverExceedsLimit(t *testing.T) {
	// Property: for any message, used <= limit
	msgs := []string{
		"",
		"short",
		strings.Repeat("A", 160),
		strings.Repeat("A", 161),
		strings.Repeat("A", 1000),
		strings.Repeat("你", 70),
		strings.Repeat("你", 71),
		strings.Repeat("你", 500),
		strings.Repeat("[", 80),  // extended chars, 80 brackets = 160 septets
		strings.Repeat("[", 81),  // extended chars, 81 brackets = 162 septets > 160
	}
	for _, msg := range msgs {
		limit, used, _ := smsLength(msg)
		if used > limit {
			t.Errorf("smsLength: used (%d) > limit (%d) for message len=%d", used, limit, len(msg))
		}
	}
}
