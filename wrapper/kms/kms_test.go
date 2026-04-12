package kms

import (
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	awskms "github.com/aws/aws-sdk-go/service/kms"
)

const (
	localstackEndpoint = "http://localhost:4566"
	testRegion         = "us-east-1"
	preCreatedKeyID    = "4f750065-4e45-4a71-9cc0-861108a5b149"
	testAliasName      = "localstack-test-key"
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

// ensureAlias creates the KMS alias if it does not already exist.
// This is idempotent -- repeated calls are safe.
func ensureAlias(t *testing.T) {
	t.Helper()

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(testRegion),
		Endpoint:    aws.String(localstackEndpoint),
		Credentials: credentials.NewStaticCredentials("test", "test", ""),
		HTTPClient:  &http.Client{Timeout: 5 * time.Second},
	})
	if err != nil {
		t.Fatalf("ensureAlias: failed to create session: %v", err)
	}

	client := awskms.New(sess)

	// Try to create the alias; ignore AlreadyExistsException.
	_, err = client.CreateAlias(&awskms.CreateAliasInput{
		AliasName:   aws.String("alias/" + testAliasName),
		TargetKeyId: aws.String(preCreatedKeyID),
	})
	if err != nil {
		// LocalStack returns "AlreadyExistsException" if the alias exists.
		// The AWS SDK error message contains this string.
		if !isAlreadyExists(err) {
			t.Fatalf("ensureAlias: failed to create alias: %v", err)
		}
	}
}

func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "AlreadyExistsException") ||
		contains(err.Error(), "already exists")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// newTestKMS returns a KMS struct configured for LocalStack.
func newTestKMS(t *testing.T) *KMS {
	t.Helper()
	return &KMS{
		AwsRegion:      awsregion.AWS_us_east_1_nvirginia,
		AesKmsKeyName:  testAliasName,
		CustomEndpoint: localstackEndpoint,
	}
}

func TestConnect(t *testing.T) {
	if !localstackAvailable() {
		t.Skip("LocalStack not available at localhost:4566")
	}
	os.Setenv("AWS_ACCESS_KEY_ID", "test")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	k := newTestKMS(t)
	err := k.Connect()
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer k.Disconnect()

	// Verify client is non-nil after Connect.
	cli, cliErr := k.getClient()
	if cliErr != nil {
		t.Fatalf("getClient after Connect returned error: %v", cliErr)
	}
	if cli == nil {
		t.Fatal("KMS client is nil after successful Connect")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	if !localstackAvailable() {
		t.Skip("LocalStack not available at localhost:4566")
	}
	os.Setenv("AWS_ACCESS_KEY_ID", "test")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	ensureAlias(t)

	k := newTestKMS(t)
	err := k.Connect()
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer k.Disconnect()

	plainText := "hello localstack kms test"

	cipherText, err := k.EncryptViaCmkAes256(plainText)
	if err != nil {
		t.Fatalf("EncryptViaCmkAes256 failed: %v", err)
	}
	if cipherText == "" {
		t.Fatal("EncryptViaCmkAes256 returned empty cipherText")
	}
	if cipherText == plainText {
		t.Fatal("cipherText should differ from plainText")
	}

	decrypted, err := k.DecryptViaCmkAes256(cipherText)
	if err != nil {
		t.Fatalf("DecryptViaCmkAes256 failed: %v", err)
	}
	if decrypted != plainText {
		t.Fatalf("round-trip mismatch: got %q, want %q", decrypted, plainText)
	}
}

func TestEncryptEmptyData(t *testing.T) {
	if !localstackAvailable() {
		t.Skip("LocalStack not available at localhost:4566")
	}
	os.Setenv("AWS_ACCESS_KEY_ID", "test")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	ensureAlias(t)

	k := newTestKMS(t)
	err := k.Connect()
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer k.Disconnect()

	_, err = k.EncryptViaCmkAes256("")
	if err == nil {
		t.Fatal("expected error when encrypting empty data, got nil")
	}
}

func TestDecryptMalformedCiphertext(t *testing.T) {
	if !localstackAvailable() {
		t.Skip("LocalStack not available at localhost:4566")
	}
	os.Setenv("AWS_ACCESS_KEY_ID", "test")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	ensureAlias(t)

	k := newTestKMS(t)
	err := k.Connect()
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer k.Disconnect()

	// Pass a valid hex string that does not represent a valid KMS ciphertext.
	malformedHex := "deadbeefcafebabe0102030405060708"
	_, err = k.DecryptViaCmkAes256(malformedHex)
	if err == nil {
		t.Fatal("expected error when decrypting malformed ciphertext, got nil")
	}
}

func TestDisconnectPreventsOperations(t *testing.T) {
	if !localstackAvailable() {
		t.Skip("LocalStack not available at localhost:4566")
	}
	os.Setenv("AWS_ACCESS_KEY_ID", "test")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	ensureAlias(t)

	k := newTestKMS(t)
	err := k.Connect()
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	k.Disconnect()

	_, err = k.EncryptViaCmkAes256("should fail after disconnect")
	if err == nil {
		t.Fatal("expected error after Disconnect, got nil")
	}
}
