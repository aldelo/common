package ses

import (
	"testing"
	"unicode/utf8"
)

func TestMaskEmailForXray(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Normal cases
		{"standard email", "harry@example.com", "ha***@example.com"},
		{"long local part", "verylongemail@domain.org", "ve***@domain.org"},

		// Short local parts
		{"2-char local", "ab@test.com", "a***@test.com"},
		{"1-char local", "a@test.com", "a***@test.com"},
		{"empty local", "@test.com", "@test.com"},

		// No @ sign
		{"no at sign long", "notanemail", "no***"},
		{"no at sign 2 chars", "ab", "ab"},
		{"no at sign 1 char", "a", "a"},

		// Edge cases
		{"empty string", "", ""},
		{"just @", "@", "@"},

		// Unicode local parts (rune-based per L18)
		{"unicode local", "\u00e9lise@domain.com", "\u00e9l***@domain.com"},
		{"emoji local", "\U0001F600\U0001F601\U0001F602@emoji.com", "\U0001F600\U0001F601***@emoji.com"},
		{"cjk local", "\u4e2d\u6587\u6d4b\u8bd5@example.cn", "\u4e2d\u6587***@example.cn"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskEmailForXray(tt.input)
			if got != tt.want {
				t.Errorf("maskEmailForXray(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("maskEmailForXray(%q) produced invalid UTF-8", tt.input)
			}
		})
	}
}

func TestMaskEmailsForXray(t *testing.T) {
	emails := []string{"alice@a.com", "bob@b.com", "c@d.com"}
	masked := maskEmailsForXray(emails)

	if len(masked) != 3 {
		t.Fatalf("expected 3 results, got %d", len(masked))
	}
	if masked[0] != "al***@a.com" {
		t.Errorf("masked[0] = %q, want %q", masked[0], "al***@a.com")
	}
	if masked[1] != "bo***@b.com" {
		t.Errorf("masked[1] = %q, want %q", masked[1], "bo***@b.com")
	}
	if masked[2] != "c***@d.com" {
		t.Errorf("masked[2] = %q, want %q", masked[2], "c***@d.com")
	}

	// Empty slice
	empty := maskEmailsForXray(nil)
	if len(empty) != 0 {
		t.Errorf("expected 0 results for nil input, got %d", len(empty))
	}
}

func TestMaskSubjectForXray(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"short subject", "Hello", "Hello"},
		{"exactly 10 chars", "1234567890", "1234567890"},
		{"11 chars truncated", "12345678901", "1234567890..."},
		{"long subject", "Your payment of $500 was processed successfully", "Your payme..."},
		{"empty", "", ""},

		// Unicode subjects (rune-based)
		{"unicode short", "\u4e2d\u6587\u6d4b", "\u4e2d\u6587\u6d4b"},
		{"unicode long", "\u4e2d\u6587\u6d4b\u8bd5\u4e00\u4e8c\u4e09\u56db\u4e94\u516d\u4e03", "\u4e2d\u6587\u6d4b\u8bd5\u4e00\u4e8c\u4e09\u56db\u4e94\u516d..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskSubjectForXray(tt.input)
			if got != tt.want {
				t.Errorf("maskSubjectForXray(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("maskSubjectForXray(%q) produced invalid UTF-8", tt.input)
			}
		})
	}
}
