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
	"strings"
	"testing"
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
