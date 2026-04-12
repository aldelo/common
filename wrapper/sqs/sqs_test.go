package sqs

/*
 * Integration tests for SQS wrapper against LocalStack.
 *
 * Prerequisites:
 *   - LocalStack running at http://localhost:4566
 *   - SQS queue "test-queue" pre-created
 *   - Env vars AWS_ACCESS_KEY_ID=test, AWS_SECRET_ACCESS_KEY=test
 */

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aldelo/common/wrapper/sqs/sqsgetqueueattribute"
)

const (
	localstackEndpoint = "http://localhost:4566"
	testQueueName      = "test-queue"
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

// newTestSQS creates an SQS struct configured for LocalStack and calls Connect.
// It sets required env vars, skips the test if LocalStack is unreachable.
func newTestSQS(t *testing.T) *SQS {
	t.Helper()

	if !localstackAvailable() {
		t.Skip("LocalStack not available at localhost:4566")
	}

	os.Setenv("AWS_ACCESS_KEY_ID", "test")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	s := &SQS{
		AwsRegion:      awsregion.AWS_us_east_1_nvirginia,
		CustomEndpoint: localstackEndpoint,
	}

	if err := s.Connect(); err != nil {
		t.Skipf("LocalStack unavailable, skipping: %v", err)
	}

	return s
}

// getTestQueueUrl resolves the URL of the pre-created test-queue.
func getTestQueueUrl(t *testing.T, s *SQS) string {
	t.Helper()

	timeout := 10 * time.Second
	queueUrl, notFound, err := s.GetQueueUrl(testQueueName, timeout)
	if err != nil {
		t.Fatalf("GetQueueUrl failed: %v", err)
	}
	if notFound {
		t.Fatalf("Pre-created queue %q not found", testQueueName)
	}
	return queueUrl
}

func TestSQS_CreateAndDeleteQueue(t *testing.T) {
	s := newTestSQS(t)
	defer s.Disconnect()

	queueName := fmt.Sprintf("integ-test-queue-%d", time.Now().UnixNano())
	timeout := 10 * time.Second

	// Create
	queueUrl, err := s.CreateQueue(queueName, nil, timeout)
	if err != nil {
		t.Fatalf("CreateQueue failed: %v", err)
	}
	if queueUrl == "" {
		t.Fatal("CreateQueue returned empty URL")
	}
	t.Logf("Created queue: %s", queueUrl)

	// Verify via GetQueueUrl
	resolvedUrl, notFound, err := s.GetQueueUrl(queueName, timeout)
	if err != nil {
		t.Fatalf("GetQueueUrl failed: %v", err)
	}
	if notFound {
		t.Fatal("Newly created queue not found via GetQueueUrl")
	}
	if resolvedUrl != queueUrl {
		t.Fatalf("URL mismatch: CreateQueue=%q, GetQueueUrl=%q", queueUrl, resolvedUrl)
	}

	// Delete
	err = s.DeleteQueue(queueUrl, timeout)
	if err != nil {
		t.Fatalf("DeleteQueue failed: %v", err)
	}

	// Verify gone
	_, notFound, _ = s.GetQueueUrl(queueName, timeout)
	if !notFound {
		t.Logf("Queue may still appear briefly after delete (eventual consistency)")
	}
}

func TestSQS_SendReceiveDeleteMessage(t *testing.T) {
	s := newTestSQS(t)
	defer s.Disconnect()

	queueUrl := getTestQueueUrl(t, s)
	timeout := 10 * time.Second
	msgBody := fmt.Sprintf("integration-test-message-%d", time.Now().UnixNano())

	// Send
	result, err := s.SendMessage(queueUrl, msgBody, nil, 0, timeout)
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if result == nil || result.MessageId == "" {
		t.Fatal("SendMessage returned nil result or empty MessageId")
	}
	t.Logf("Sent message ID: %s", result.MessageId)

	// Receive
	messages, err := s.ReceiveMessage(
		queueUrl,
		1,    // maxNumberOfMessages
		nil,  // messageAttributeNames
		nil,  // systemAttributeNames
		30,   // visibilityTimeOutSeconds
		0,    // waitTimeSeconds (no long-poll, message should be available)
		"",   // receiveRequestAttemptId
		timeout,
	)
	if err != nil {
		t.Fatalf("ReceiveMessage failed: %v", err)
	}
	if len(messages) == 0 {
		t.Fatal("ReceiveMessage returned no messages")
	}

	receivedMsg := messages[0]
	if receivedMsg.Body != msgBody {
		t.Fatalf("Message body mismatch: got %q, want %q", receivedMsg.Body, msgBody)
	}
	t.Logf("Received message ID: %s, body: %s", receivedMsg.MessageId, receivedMsg.Body)

	// Delete the received message
	err = s.DeleteMessage(queueUrl, receivedMsg.ReceiptHandle, timeout)
	if err != nil {
		t.Fatalf("DeleteMessage failed: %v", err)
	}
}

func TestSQS_GetQueueAttributes(t *testing.T) {
	s := newTestSQS(t)
	defer s.Disconnect()

	queueUrl := getTestQueueUrl(t, s)
	timeout := 10 * time.Duration(time.Second)

	attrs, err := s.GetQueueAttributes(
		queueUrl,
		[]sqsgetqueueattribute.SQSGetQueueAttribute{
			sqsgetqueueattribute.ApproximateNumberOfMessages,
			sqsgetqueueattribute.VisibilityTimeout,
		},
		timeout,
	)
	if err != nil {
		t.Fatalf("GetQueueAttributes failed: %v", err)
	}
	if attrs == nil {
		t.Fatal("GetQueueAttributes returned nil map")
	}

	// Both requested attributes should be present in the result
	if _, ok := attrs[sqsgetqueueattribute.ApproximateNumberOfMessages]; !ok {
		t.Error("Missing ApproximateNumberOfMessages in attributes")
	}
	if _, ok := attrs[sqsgetqueueattribute.VisibilityTimeout]; !ok {
		t.Error("Missing VisibilityTimeout in attributes")
	}

	t.Logf("Queue attributes: %+v", attrs)
}

func TestSQS_SendToNonExistentQueue(t *testing.T) {
	s := newTestSQS(t)
	defer s.Disconnect()

	timeout := 10 * time.Second
	// Use a URL that references a non-existent queue
	fakeQueueUrl := localstackEndpoint + "/000000000000/queue-does-not-exist-12345"

	_, err := s.SendMessage(fakeQueueUrl, "test body", nil, 0, timeout)
	if err == nil {
		t.Fatal("Expected error when sending to non-existent queue, got nil")
	}
	t.Logf("Got expected error for non-existent queue: %v", err)
}
