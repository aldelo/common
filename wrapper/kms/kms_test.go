package kms

import (
	"net"
	"net/http"
	"os"
	"sync"
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

// TestKMS_ConcurrentReconfigureDoesNotRace pins the SP-008 P1-COMMON-KMS-01
// invariant: setSessionAndClient (the writer) running concurrently with
// getClient (the reader) must never surface a torn pointer read and must
// never trip the race detector. Before the atomic.Pointer migration,
// this property held only because the RLock hoist had been applied at
// every reader site — a future refactor that added a new *KMS method
// reading `k.kmsClient` without taking the mutex would have silently
// regressed the invariant. Post-migration the compiler forces every
// access through .Load()/.Store(), so this test is now also a compile-
// time guardrail: if anyone reverts the type, the inline hoists break
// the build before the test even runs.
//
// The test uses two zero-value *awskms.KMS stub pointers — no LocalStack,
// no network, no real AWS — because the invariant being exercised lives
// entirely in the pointer publication path, not in any SDK behavior.
// Run with `go test -race` to enforce the full guarantee; without
// `-race` the test still exercises Load/Store correctness but cannot
// detect a regressed torn read.
func TestKMS_ConcurrentReconfigureDoesNotRace(t *testing.T) {
	k := &KMS{}

	// Two distinct stub clients — the test doesn't call any SDK method
	// on them, it only cares that Load() always returns one of the two
	// pointers and never an intermediate/torn value.
	cliA := &awskms.KMS{}
	cliB := &awskms.KMS{}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writer: flip the published client back and forth. Each Store goes
	// through setSessionAndClient, which holds k.mu for the duration of
	// the field mutation — the Store is under the lock, which is a
	// stricter happens-before than the hot-path reader needs.
	wg.Add(1)
	go func() {
		defer wg.Done()
		next := cliA
		for {
			select {
			case <-stop:
				return
			default:
				k.setSessionAndClient(nil, next)
				if next == cliA {
					next = cliB
				} else {
					next = cliA
				}
			}
		}
	}()

	// Readers: hammer getClient() in parallel. Each iteration must see
	// either cliA or cliB (or nil on the very first iterations before
	// the writer's first Store has landed) — never a torn pointer.
	const readers = 8
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					cli, err := k.getClient()
					if err == nil && cli != cliA && cli != cliB {
						t.Errorf("getClient returned unexpected pointer %p (want cliA=%p or cliB=%p)", cli, cliA, cliB)
						return
					}
				}
			}
		}()
	}

	// Run for 500ms — long enough for the race detector to see many
	// concurrent Load/Store pairs but short enough to keep the unit
	// test in sub-second territory.
	time.Sleep(500 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// TestKMS_GetClientReturnsErrWhenUnset pins the contract of getClient
// against a KMS whose kmsClient atomic.Pointer has never been Stored:
// Load returns nil, getClient returns the sentinel error. Before the
// atomic.Pointer migration this path was covered implicitly by the nil
// comparison under RLock; after the migration Load returns a nil *kms.KMS
// and the nil check must still work.
func TestKMS_GetClientReturnsErrWhenUnset(t *testing.T) {
	k := &KMS{}
	cli, err := k.getClient()
	if err == nil {
		t.Fatal("expected error when kmsClient is unset, got nil")
	}
	if cli != nil {
		t.Errorf("expected nil client when unset, got %p", cli)
	}
}

// TestKMS_DisconnectClearsPublishedClient pins the Disconnect contract
// against the atomic.Pointer scheme: after Disconnect the next getClient
// call must return the unset-client error, i.e. Store(nil) must be
// observable through Load().
func TestKMS_DisconnectClearsPublishedClient(t *testing.T) {
	k := &KMS{}
	k.setSessionAndClient(nil, &awskms.KMS{})
	if _, err := k.getClient(); err != nil {
		t.Fatalf("expected non-nil client after Store, got error: %v", err)
	}
	k.Disconnect()
	if _, err := k.getClient(); err == nil {
		t.Fatal("expected error after Disconnect, got nil")
	}
}
