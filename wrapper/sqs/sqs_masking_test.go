package sqs

import "testing"

func TestMaskQueueURLForXray(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard URL", "https://sqs.us-east-1.amazonaws.com/123456789012/my-queue", "https://sqs.us-east-1.amazonaws.com/1234****/my-queue"},
		{"different region", "https://sqs.eu-west-1.amazonaws.com/987654321098/prod-queue", "https://sqs.eu-west-1.amazonaws.com/9876****/prod-queue"},
		{"no account ID", "https://sqs.us-east-1.amazonaws.com/my-queue", "https://sqs.us-east-1.amazonaws.com/my-queue"},
		{"empty", "", ""},
		{"just queue name", "my-queue", "my-queue"},
		{"localhost URL", "http://localhost:4566/000000000000/test-queue", "http://localhost:4566/0000****/test-queue"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskQueueURLForXray(tt.input)
			if got != tt.want {
				t.Errorf("maskQueueURLForXray(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMaskARNForXray(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard ARN", "arn:aws:sqs:us-east-1:123456789012:my-queue", "arn:aws:sqs:us-east-1:1234****:my-queue"},
		{"different region", "arn:aws:sqs:eu-west-1:987654321098:prod-queue", "arn:aws:sqs:eu-west-1:9876****:prod-queue"},
		{"short ARN", "arn:aws:sqs", "arn:aws:sqs"},
		{"empty", "", ""},
		{"non-numeric account", "arn:aws:sqs:us-east-1:not-a-number:queue", "arn:aws:sqs:us-east-1:not-a-number:queue"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskARNForXray(tt.input)
			if got != tt.want {
				t.Errorf("maskARNForXray(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
