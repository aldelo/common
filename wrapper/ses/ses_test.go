package ses

/*
 * Integration tests for SES wrapper against LocalStack.
 *
 * Prerequisites:
 *   - LocalStack running at http://localhost:4566
 *   - Env vars AWS_ACCESS_KEY_ID=test, AWS_SECRET_ACCESS_KEY=test
 *
 * Note: LocalStack SES accepts all operations without real email delivery.
 * Sender email identity must be verified via VerifyEmailIdentity before sending.
 */

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/aldelo/common/wrapper/aws/awsregion"
	awsses "github.com/aws/aws-sdk-go/service/ses"
)

const (
	localstackEndpoint = "http://localhost:4566"
)

// localstackAvailable returns true if LocalStack is reachable on port 4566.
func localstackAvailable() bool {
	conn, err := net.DialTimeout("tcp", "localhost:4566", 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// newTestSES creates an SES struct configured for LocalStack and calls Connect.
// It sets required env vars, skips the test if LocalStack is unreachable.
func newTestSES(t *testing.T) *SES {
	t.Helper()

	if !localstackAvailable() {
		t.Skip("LocalStack not available at localhost:4566")
	}

	os.Setenv("AWS_ACCESS_KEY_ID", "test")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	s := &SES{
		AwsRegion:      awsregion.AWS_us_east_1_nvirginia,
		CustomEndpoint: localstackEndpoint,
	}

	if err := s.Connect(); err != nil {
		t.Skipf("LocalStack unavailable, skipping: %v", err)
	}

	return s
}

// verifyEmailIdentity registers a sender identity with LocalStack SES so that
// SendEmail does not reject the message. This uses the raw AWS SDK client
// because the wrapper does not expose VerifyEmailIdentity.
func verifyEmailIdentity(t *testing.T, s *SES, email string) {
	t.Helper()
	client := s.getClient()
	if client == nil {
		t.Fatal("SES client is nil, cannot verify email identity")
	}
	_, err := client.VerifyEmailIdentity(&awsses.VerifyEmailIdentityInput{
		EmailAddress: &email,
	})
	if err != nil {
		t.Fatalf("VerifyEmailIdentity failed: %v", err)
	}
}

func TestSES_GetSendQuota(t *testing.T) {
	s := newTestSES(t)
	defer s.Disconnect()

	timeout := 10 * time.Second
	quota, err := s.GetSendQuota(timeout)
	if err != nil {
		t.Fatalf("GetSendQuota failed: %v", err)
	}
	if quota == nil {
		t.Fatal("GetSendQuota returned nil quota")
	}

	// LocalStack returns default quota values
	t.Logf("SendQuota: Max24Hour=%d, MaxPerSecond=%d, SentLast24Hours=%d",
		quota.Max24HourSendLimit, quota.MaxPerSecondSendLimit, quota.SentLast24Hours)
}

func TestSES_SendEmail(t *testing.T) {
	s := newTestSES(t)
	defer s.Disconnect()

	// LocalStack requires verified sender identity
	verifyEmailIdentity(t, s, "sender@example.com")

	email := &Email{}
	email.Initial(
		"sender@example.com",
		"recipient@example.com",
		"Integration Test Subject",
		"This is a plain text body for testing.",
		"<html><body><h1>Integration Test</h1><p>HTML body for testing.</p></body></html>",
	)

	timeout := 10 * time.Second
	messageId, err := s.SendEmail(email, timeout)
	if err != nil {
		t.Fatalf("SendEmail failed: %v", err)
	}
	if messageId == "" {
		t.Fatal("SendEmail returned empty messageId")
	}
	t.Logf("Sent email, messageId: %s", messageId)
}

func TestSES_SendEmailTextOnly(t *testing.T) {
	s := newTestSES(t)
	defer s.Disconnect()

	// LocalStack requires verified sender identity
	verifyEmailIdentity(t, s, "sender@example.com")

	email := &Email{}
	email.Initial(
		"sender@example.com",
		"recipient@example.com",
		"Text Only Test",
		"Plain text email body only.",
	)

	timeout := 10 * time.Second
	messageId, err := s.SendEmail(email, timeout)
	if err != nil {
		t.Fatalf("SendEmail (text-only) failed: %v", err)
	}
	if messageId == "" {
		t.Fatal("SendEmail (text-only) returned empty messageId")
	}
	t.Logf("Sent text-only email, messageId: %s", messageId)
}

func TestSES_EmailValidation_MissingFrom(t *testing.T) {
	email := &Email{
		To:       []string{"recipient@example.com"},
		Subject:  "Test Subject",
		BodyText: "Test body",
		Charset:  "UTF-8",
	}

	_, err := email.GenerateSendEmailInput()
	if err == nil {
		t.Fatal("Expected validation error for missing From, got nil")
	}
	t.Logf("Got expected validation error: %v", err)
}

func TestSES_EmailValidation_MissingTo(t *testing.T) {
	email := &Email{
		From:     "sender@example.com",
		Subject:  "Test Subject",
		BodyText: "Test body",
		Charset:  "UTF-8",
	}

	_, err := email.GenerateSendEmailInput()
	if err == nil {
		t.Fatal("Expected validation error for missing To, got nil")
	}
	t.Logf("Got expected validation error: %v", err)
}

func TestSES_EmailValidation_MissingSubject(t *testing.T) {
	email := &Email{
		From:     "sender@example.com",
		To:       []string{"recipient@example.com"},
		BodyText: "Test body",
		Charset:  "UTF-8",
	}

	_, err := email.GenerateSendEmailInput()
	if err == nil {
		t.Fatal("Expected validation error for missing Subject, got nil")
	}
	t.Logf("Got expected validation error: %v", err)
}

func TestSES_EmailValidation_MissingBody(t *testing.T) {
	email := &Email{
		From:    "sender@example.com",
		To:      []string{"recipient@example.com"},
		Subject: "Test Subject",
		Charset: "UTF-8",
	}

	_, err := email.GenerateSendEmailInput()
	if err == nil {
		t.Fatal("Expected validation error for missing Body, got nil")
	}
	t.Logf("Got expected validation error: %v", err)
}

func TestSES_SendEmail_NilEmail(t *testing.T) {
	s := newTestSES(t)
	defer s.Disconnect()

	timeout := 10 * time.Second
	_, err := s.SendEmail(nil, timeout)
	if err == nil {
		t.Fatal("Expected error when sending nil email, got nil")
	}
	t.Logf("Got expected error for nil email: %v", err)
}

func TestSES_EmailInitialAndChaining(t *testing.T) {
	email := &Email{}
	result := email.Initial(
		"sender@example.com",
		"to@example.com",
		"Test Subject",
		"Test body text",
	).SetCC("cc@example.com").SetBCC("bcc@example.com").SetReplyTo("reply@example.com")

	if result != email {
		t.Fatal("Chaining should return the same Email pointer")
	}
	if email.From != "sender@example.com" {
		t.Fatalf("From mismatch: %q", email.From)
	}
	if len(email.To) < 1 || email.To[0] != "to@example.com" {
		t.Fatalf("To mismatch: %v", email.To)
	}
	if len(email.CC) < 1 || email.CC[0] != "cc@example.com" {
		t.Fatalf("CC mismatch: %v", email.CC)
	}
	if len(email.BCC) < 1 || email.BCC[0] != "bcc@example.com" {
		t.Fatalf("BCC mismatch: %v", email.BCC)
	}
	if email.Charset != "UTF-8" {
		t.Fatalf("Charset not defaulted: %q", email.Charset)
	}

	// Validate generates proper input
	input, err := email.GenerateSendEmailInput()
	if err != nil {
		t.Fatalf("GenerateSendEmailInput failed: %v", err)
	}
	if input == nil {
		t.Fatal("GenerateSendEmailInput returned nil")
	}
	t.Logf("Generated SendEmailInput successfully for chained email")
}
