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
	"strings"
	"testing"
	"time"

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
