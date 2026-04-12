package s3

/*
 * Integration tests for S3 wrapper against LocalStack.
 *
 * Prerequisites:
 *   - LocalStack running at http://localhost:4566
 *   - S3 bucket "test-bucket" pre-created
 *   - Env vars AWS_ACCESS_KEY_ID=test, AWS_SECRET_ACCESS_KEY=test
 */

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aldelo/common/wrapper/aws/awsregion"
)

const (
	localstackEndpoint = "http://localhost:4566"
	testBucket         = "test-bucket"
)

// newTestS3 creates an S3 struct configured for LocalStack and calls Connect.
// It sets required env vars, skips the test if LocalStack is unreachable.
func newTestS3(t *testing.T) *S3 {
	t.Helper()

	os.Setenv("AWS_ACCESS_KEY_ID", "test")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	s := &S3{
		AwsRegion:      awsregion.AWS_us_east_1_nvirginia,
		BucketName:     testBucket,
		CustomEndpoint: localstackEndpoint,
	}

	if err := s.Connect(); err != nil {
		t.Skipf("LocalStack unavailable, skipping: %v", err)
	}

	return s
}

func TestS3_UploadDownloadDeleteLifecycle(t *testing.T) {
	s := newTestS3(t)
	defer s.Disconnect()

	testKey := fmt.Sprintf("integration-test-%d", time.Now().UnixNano())
	testData := []byte("hello localstack s3 integration test")
	timeout := 10 * time.Second

	// --- Upload ---
	location, err := s.Upload(&timeout, testData, testKey)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	if location == "" {
		t.Fatal("Upload returned empty location")
	}
	t.Logf("Uploaded to: %s", location)

	// --- Download and verify content ---
	data, notFound, err := s.Download(&timeout, testKey)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	if notFound {
		t.Fatal("Download reported notFound for just-uploaded object")
	}
	if !bytes.Equal(data, testData) {
		t.Fatalf("Download content mismatch: got %q, want %q", string(data), string(testData))
	}

	// --- List and verify object appears ---
	fileKeys, _, err := s.ListFileKeys(&timeout, "", 100)
	if err != nil {
		t.Fatalf("ListFileKeys failed: %v", err)
	}
	found := false
	for _, k := range fileKeys {
		if k == testKey {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ListFileKeys did not contain uploaded key %q; keys: %v", testKey, fileKeys)
	}

	// --- Delete ---
	deleteOk, err := s.Delete(&timeout, testKey)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if !deleteOk {
		t.Fatal("Delete returned false")
	}

	// --- Verify object is gone ---
	_, notFound, err = s.Download(&timeout, testKey)
	if err != nil && !notFound {
		t.Fatalf("Download after delete returned unexpected error: %v", err)
	}
	if !notFound {
		t.Fatal("Object still exists after delete")
	}
}

func TestS3_DownloadNonExistentObject(t *testing.T) {
	s := newTestS3(t)
	defer s.Disconnect()

	timeout := 10 * time.Second
	_, notFound, err := s.Download(&timeout, "does-not-exist-key-12345")
	if err != nil && !notFound {
		t.Fatalf("Expected notFound=true for non-existent key, got err: %v", err)
	}
	if !notFound {
		t.Fatal("Expected notFound=true for non-existent key")
	}
}

func TestS3_UploadWithFolder(t *testing.T) {
	s := newTestS3(t)
	defer s.Disconnect()

	testKey := fmt.Sprintf("file-%d.txt", time.Now().UnixNano())
	testData := []byte("folder upload test content")
	timeout := 10 * time.Second

	// Upload into a folder hierarchy
	location, err := s.Upload(&timeout, testData, testKey, "test-folder", "sub-folder")
	if err != nil {
		t.Fatalf("Upload with folder failed: %v", err)
	}
	t.Logf("Uploaded to folder: %s", location)

	// Download from the same folder
	data, notFound, err := s.Download(&timeout, testKey, "test-folder", "sub-folder")
	if err != nil {
		t.Fatalf("Download from folder failed: %v", err)
	}
	if notFound {
		t.Fatal("Download from folder reported notFound")
	}
	if !bytes.Equal(data, testData) {
		t.Fatalf("Content mismatch: got %q, want %q", string(data), string(testData))
	}

	// Cleanup
	_, err = s.Delete(&timeout, testKey, "test-folder", "sub-folder")
	if err != nil {
		t.Logf("Cleanup delete failed (non-fatal): %v", err)
	}
}
