package sqs

/*
 * Tests for the fair-queue (MessageGroupId on a standard queue) send helpers.
 *
 * validateMessageGroupId is pure, so its table runs everywhere. The round-trip tests need a real
 * endpoint and follow the same LocalStack gating as sqs_test.go — they skip when it is not up.
 */

import (
	"strings"
	"testing"
)

func TestValidateMessageGroupId(t *testing.T) {
	cases := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"typical merchant gid", "01H9XK7YQ2ZB3N4M5P6R7S8T9V", false},
		{"fiserv terminal id", "TERM-00123", false},
		{"punctuation is allowed", `a!"#$%&'()*+,-./:;<=>?@[\]^_{|}~`, false},
		{"single char", "x", false},
		{"max length 128", strings.Repeat("a", 128), false},

		{"empty is rejected", "", true},
		{"over 128 is rejected", strings.Repeat("a", 129), true},
		{"space is rejected", "merchant 123", true},
		{"non-ascii is rejected", "商户1", true},
		{"control char is rejected", "abc\tdef", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateMessageGroupId(c.id)
			if c.wantErr && err == nil {
				t.Errorf("validateMessageGroupId(%q) = nil, want an error", c.id)
			}
			if !c.wantErr && err != nil {
				t.Errorf("validateMessageGroupId(%q) = %v, want nil", c.id, err)
			}
		})
	}
}

// TestSendMessageWithGroupValidatesBeforeCallingAWS pins that a bad group id is rejected locally.
// A nil client short-circuits first, so this uses a connected client and an invalid id: the point
// is that no request is attempted and the error names the group id, not a network failure.
func TestSendMessageWithGroupValidatesBeforeCallingAWS(t *testing.T) {
	s := newTestSQS(t) // skips when LocalStack is unavailable
	queueUrl := mustTestQueueURL(t, s)

	if _, err := s.SendMessageWithGroup(queueUrl, "body", nil, 0, ""); err == nil {
		t.Error("empty group id must be rejected")
	} else if !strings.Contains(err.Error(), "Message Group Id is Required") {
		t.Errorf("error = %v, want it to name the missing group id", err)
	}

	if _, err := s.SendMessageWithGroup(queueUrl, "body", nil, 0, strings.Repeat("a", 129)); err == nil {
		t.Error("over-length group id must be rejected")
	}
}

// TestSendMessageWithGroupRoundTrip sends a grouped message to a STANDARD queue and reads it back.
// This is the behaviour the whole change exists for: a standard queue must ACCEPT MessageGroupId
// (it would reject MessageDeduplicationId, which is why SendMessageFifo cannot be reused).
func TestSendMessageWithGroupRoundTrip(t *testing.T) {
	s := newTestSQS(t)
	queueUrl := mustTestQueueURL(t, s)

	result, err := s.SendMessageWithGroup(queueUrl, "fair-queue-round-trip", nil, 0, "tenant-A")
	if err != nil {
		t.Fatalf("SendMessageWithGroup failed: %v", err)
	}
	if result == nil || result.MessageId == "" {
		t.Fatal("SendMessageWithGroup returned no message id")
	}

	msgs, err := s.ReceiveMessage(queueUrl, 1, nil, nil, 5, 5, "")
	if err != nil {
		t.Fatalf("ReceiveMessage failed: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("no message received back")
	}
	for _, m := range msgs {
		_ = s.DeleteMessage(queueUrl, m.ReceiptHandle)
	}
}

// TestSendMessageBatchWithGroupMixedTenants covers the batch path: entries may carry different
// tenants, and one entry may opt out by leaving the group id empty.
func TestSendMessageBatchWithGroupMixedTenants(t *testing.T) {
	s := newTestSQS(t)
	queueUrl := mustTestQueueURL(t, s)

	entries := []*SQSStandardMessageWithGroup{
		{SQSStandardMessage: SQSStandardMessage{Id: "1", MessageBody: "a"}, MessageGroupId: "tenant-A"},
		{SQSStandardMessage: SQSStandardMessage{Id: "2", MessageBody: "b"}, MessageGroupId: "tenant-B"},
		{SQSStandardMessage: SQSStandardMessage{Id: "3", MessageBody: "c"}}, // ungrouped on purpose
	}

	success, failed, err := s.SendMessageBatchWithGroup(queueUrl, entries)
	if err != nil {
		t.Fatalf("SendMessageBatchWithGroup failed: %v", err)
	}
	if len(failed) > 0 {
		t.Errorf("batch reported %d failures, want 0", len(failed))
	}
	if len(success) != 3 {
		t.Errorf("sent %d messages, want 3", len(success))
	}

	// drain what we just sent so the shared test queue does not accumulate
	for i := 0; i < 3; i++ {
		msgs, rerr := s.ReceiveMessage(queueUrl, 10, nil, nil, 5, 2, "")
		if rerr != nil || len(msgs) == 0 {
			break
		}
		for _, m := range msgs {
			_ = s.DeleteMessage(queueUrl, m.ReceiptHandle)
		}
	}
}

// TestSendMessageBatchWithGroupRejectsBadEntry pins that one invalid group id fails the whole call
// rather than sending part of the batch untagged.
func TestSendMessageBatchWithGroupRejectsBadEntry(t *testing.T) {
	s := newTestSQS(t)
	queueUrl := mustTestQueueURL(t, s)

	entries := []*SQSStandardMessageWithGroup{
		{SQSStandardMessage: SQSStandardMessage{Id: "1", MessageBody: "a"}, MessageGroupId: "tenant-A"},
		{SQSStandardMessage: SQSStandardMessage{Id: "2", MessageBody: "b"}, MessageGroupId: "bad tenant"},
	}

	if _, _, err := s.SendMessageBatchWithGroup(queueUrl, entries); err == nil {
		t.Error("an invalid group id in any entry must fail the whole batch")
	} else if !strings.Contains(err.Error(), "Entry 2") {
		t.Errorf("error = %v, want it to name the offending entry", err)
	}
}

// mustTestQueueURL resolves the shared LocalStack test queue, failing the test if it is missing.
func mustTestQueueURL(t *testing.T, s *SQS) string {
	t.Helper()

	queueUrl, notFound, err := s.GetQueueUrl(testQueueName)
	if err != nil {
		t.Fatalf("GetQueueUrl failed: %v", err)
	}
	if notFound {
		t.Skipf("test queue %q not present in LocalStack", testQueueName)
	}
	return queueUrl
}
