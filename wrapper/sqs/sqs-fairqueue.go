package sqs

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

// Standard-queue sends that carry a MessageGroupId (SQS fair queues).
//
// Why this exists as a separate entry point rather than a new parameter on SendMessage /
// SendMessageBatch: those signatures are consumed across every service in this account, so they are
// left untouched. Everything here is additive.
//
// What MessageGroupId means on a STANDARD queue:
//
//   - It is a TENANT TAG, not an ordering key. SQS fair queues use it to keep one noisy tenant from
//     starving the others in the same queue. It does NOT give FIFO ordering, exactly-once delivery,
//     or per-group serialization — a standard queue stays at-least-once and unordered.
//   - It must NOT be paired with MessageDeduplicationId. That field is FIFO-only and a standard
//     queue rejects the request outright, which is exactly why SendMessageFifo cannot be reused
//     here: it always stamps a deduplication id.
//
// Constraints enforced below (AWS limits): up to 128 characters, and the allowed set is
// alphanumerics plus punctuation. An empty group id is rejected for the single-message send (the
// caller asked for a grouped send, so silently dropping the group would defeat the point); in a
// batch, an entry may leave it empty to send that one message ungrouped.

import (
	"context"
	"errors"
	"fmt"
	"time"

	util "github.com/aldelo/common"
	"github.com/aldelo/common/wrapper/xray"
	"github.com/aws/aws-sdk-go/aws"
	awssqs "github.com/aws/aws-sdk-go/service/sqs"
)

// maxMessageGroupIdLength is the AWS limit on MessageGroupId.
const maxMessageGroupIdLength = 128

// SQSStandardMessageWithGroup is one standard-queue batch entry plus its fair-queue tenant tag.
//
// It embeds SQSStandardMessage rather than adding a field to it, so existing unkeyed composite
// literals of SQSStandardMessage keep compiling.
type SQSStandardMessageWithGroup struct {
	SQSStandardMessage

	// MessageGroupId is the fair-queue tenant tag. Optional per entry: empty sends that message
	// ungrouped.
	MessageGroupId string
}

// validateMessageGroupId checks a MessageGroupId against the AWS constraints.
//
// Allowed: alphanumerics and punctuation (!"#$%&'()*+,-./:;<=>?@[\]^_`{|}~), 1 to 128 characters.
// Kept as a standalone function so the rules are unit-testable without an SQS client.
func validateMessageGroupId(messageGroupId string) error {
	if len(messageGroupId) == 0 {
		return errors.New("Message Group Id is Required")
	}

	if len(messageGroupId) > maxMessageGroupIdLength {
		return fmt.Errorf("Message Group Id Limited to %d Characters", maxMessageGroupIdLength)
	}

	for _, r := range messageGroupId {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r >= '!' && r <= '/': // ! " # $ % & ' ( ) * + , - . /
		case r >= ':' && r <= '@': // : ; < = > ? @
		case r >= '[' && r <= '`': // [ \ ] ^ _ `
		case r >= '{' && r <= '~': // { | } ~
		default:
			return fmt.Errorf("Message Group Id Contains Unsupported Character %q", r)
		}
	}

	return nil
}

// SendMessageWithGroup sends one message to a STANDARD queue, tagged with messageGroupId for SQS
// fair queues.
//
// This is SendMessage plus the tenant tag; see the file header for what the tag does and does not
// mean. For a FIFO queue use SendMessageFifo instead — this call never sets a deduplication id.
//
// Parameters:
//  1. queueUrl = required, the standard queue to send the message to
//  2. messageBody = required, the message payload
//  3. messageAttributes = optional, message attribute key value pairs
//  4. delaySeconds = optional, if greater than 0, seconds to delay before the message is available
//     to consumers (clamped to 0..900)
//  5. messageGroupId = required, fair-queue tenant tag (1..128 chars, alphanumerics + punctuation)
//  6. timeOutDuration = optional, timeout value for context if any
//
// Return Values:
//  1. result = struct containing Send... action message result
//  2. err = error info if any
func (s *SQS) SendMessageWithGroup(queueUrl string,
	messageBody string,
	messageAttributes map[string]*awssqs.MessageAttributeValue,
	delaySeconds int64,
	messageGroupId string,
	timeOutDuration ...time.Duration) (result *SQSMessageResult, err error) {

	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SQS-SendMessageWithGroup", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("SQS", seg.SafeAddMetadata("SQS-SendMessageWithGroup-QueueURL", maskQueueURLForXray(queueUrl)))
			xray.LogXrayAddFailure("SQS", seg.SafeAddMetadata("SQS-SendMessageWithGroup-MessageBody-Length", len(messageBody)))
			xray.LogXrayAddFailure("SQS", seg.SafeAddMetadata("SQS-SendMessageWithGroup-MessageAttribute-Count", len(messageAttributes)))
			xray.LogXrayAddFailure("SQS", seg.SafeAddMetadata("SQS-SendMessageWithGroup-DelaySeconds", delaySeconds))
			// The group id is a tenant identifier (merchant / terminal), so only its length goes to
			// X-Ray — same reasoning as masking the queue URL and omitting the body.
			xray.LogXrayAddFailure("SQS", seg.SafeAddMetadata("SQS-SendMessageWithGroup-MessageGroupId-Length", len(messageGroupId)))
			xray.LogXrayAddFailure("SQS", seg.SafeAddMetadata("SQS-SendMessageWithGroup-Result", result))

			if err != nil {
				xray.LogXrayAddFailure("SQS", seg.SafeAddError(err))
			}
		}()
	}

	// validate
	cli := s.getClient()
	if cli == nil {
		err = errors.New("SendMessageWithGroup Failed: SQS Client is Required")
		return nil, err
	}

	if util.LenTrim(queueUrl) <= 0 {
		err = errors.New("SendMessageWithGroup Failed: Queue Url is Required")
		return nil, err
	}

	if util.LenTrim(messageBody) <= 0 {
		err = errors.New("SendMessageWithGroup Failed: Message Body is Required")
		return nil, err
	}

	if e := validateMessageGroupId(messageGroupId); e != nil {
		err = fmt.Errorf("SendMessageWithGroup Failed: %w", e)
		return nil, err
	}

	// create input object
	input := &awssqs.SendMessageInput{
		QueueUrl:       aws.String(queueUrl),
		MessageBody:    aws.String(messageBody),
		MessageGroupId: aws.String(messageGroupId),
	}

	if messageAttributes != nil {
		input.MessageAttributes = messageAttributes
	}

	if delaySeconds != 0 {
		if delaySeconds < 0 {
			delaySeconds = 0
		}

		if delaySeconds > 900 {
			delaySeconds = 900
		}

		input.DelaySeconds = aws.Int64(delaySeconds)
	}

	// perform action
	var output *awssqs.SendMessageOutput

	callCtx, callCancel := ensureSQSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = cli.SendMessageWithContext(callCtx, input)
	callCancel()

	// evaluate result
	if err != nil {
		err = fmt.Errorf("SendMessageWithGroup Failed: (Send Action) %w", err)
		return nil, err
	} else {
		result = &SQSMessageResult{
			MessageId:              aws.StringValue(output.MessageId),
			MD5ofMessageBody:       aws.StringValue(output.MD5OfMessageBody),
			MD5ofMessageAttributes: aws.StringValue(output.MD5OfMessageAttributes),
			FifoSequenceNumber:     "",
		}
		return result, nil
	}
}

// SendMessageBatchWithGroup sends up to 10 messages to a STANDARD queue, each carrying its own
// fair-queue tenant tag.
//
// Per-entry group ids are deliberate: one batch may mix tenants, which is what makes batching worth
// doing when the producer drains a mixed work list. An entry whose MessageGroupId is empty is sent
// ungrouped; a non-empty one must satisfy the AWS constraints or the whole call is rejected before
// anything is sent.
//
// Parameters:
//  1. queueUrl = required, the standard queue to send messages to
//  2. messageEntries = required, up to 10 entries, each with Id + MessageBody and an optional
//     MessageGroupId
//  3. timeOutDuration = optional, timeout value for context if any
//
// Return Values:
//  1. successList = slice of successfully sent messages
//  2. failedList = slice of messages that failed to send
//  3. err = error info if any
func (s *SQS) SendMessageBatchWithGroup(queueUrl string,
	messageEntries []*SQSStandardMessageWithGroup,
	timeOutDuration ...time.Duration) (successList []*SQSSuccessResult, failedList []*SQSFailResult, err error) {

	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SQS-SendMessageBatchWithGroup", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("SQS", seg.SafeAddMetadata("SQS-SendMessageBatchWithGroup-QueueURL", maskQueueURLForXray(queueUrl)))
			xray.LogXrayAddFailure("SQS", seg.SafeAddMetadata("SQS-SendMessageBatchWithGroup-MessageEntry-Count", len(messageEntries)))
			xray.LogXrayAddFailure("SQS", seg.SafeAddMetadata("SQS-SendMessageBatchWithGroup-Result-SuccessList", successList))
			xray.LogXrayAddFailure("SQS", seg.SafeAddMetadata("SQS-SendMessageBatchWithGroup-Result-FailedList", failedList))

			if err != nil {
				xray.LogXrayAddFailure("SQS", seg.SafeAddError(err))
			}
		}()
	}

	// validate
	cli := s.getClient()
	if cli == nil {
		err = errors.New("SendMessageBatchWithGroup Failed: SQS Client is Required")
		return nil, nil, err
	}

	if util.LenTrim(queueUrl) <= 0 {
		err = errors.New("SendMessageBatchWithGroup Failed: " + "Queue Url is Required")
		return nil, nil, err
	}

	if messageEntries == nil {
		err = errors.New("SendMessageBatchWithGroup Failed: " + "Message Entries Required (nil)")
		return nil, nil, err
	}

	if len(messageEntries) <= 0 {
		err = errors.New("SendMessageBatchWithGroup Failed: " + "Message Entries Required (count = 0)")
		return nil, nil, err
	}

	if len(messageEntries) > 10 {
		err = errors.New("SendMessageBatchWithGroup Failed: " + "Message Entries Per Batch Limited to 10")
		return nil, nil, err
	}

	// create input object
	var entries []*awssqs.SendMessageBatchRequestEntry

	for _, v := range messageEntries {
		if v == nil {
			continue
		}

		if util.LenTrim(v.Id) > 0 && util.LenTrim(v.MessageBody) > 0 {
			delay := v.DelaySeconds
			if delay < 0 {
				delay = 0
			}
			if delay > 900 {
				delay = 900
			}

			entry := &awssqs.SendMessageBatchRequestEntry{
				Id:                aws.String(v.Id),
				MessageBody:       aws.String(v.MessageBody),
				MessageAttributes: v.MessageAttributes,
				DelaySeconds:      aws.Int64(delay),
			}

			// Empty = send this one ungrouped; non-empty must be valid, and an invalid value fails
			// the whole call rather than silently sending part of the batch untagged.
			if len(v.MessageGroupId) > 0 {
				if e := validateMessageGroupId(v.MessageGroupId); e != nil {
					err = fmt.Errorf("SendMessageBatchWithGroup Failed: (Entry %s) %w", v.Id, e)
					return nil, nil, err
				}

				entry.MessageGroupId = aws.String(v.MessageGroupId)
			}

			entries = append(entries, entry)
		}
	}

	if len(entries) <= 0 {
		err = errors.New("SendMessageBatchWithGroup Failed: " + "Message Entries Elements Count Must Not Be Zero")
		return nil, nil, err
	}

	input := &awssqs.SendMessageBatchInput{
		QueueUrl: aws.String(queueUrl),
		Entries:  entries,
	}

	// perform action
	var output *awssqs.SendMessageBatchOutput

	callCtx, callCancel := ensureSQSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = cli.SendMessageBatchWithContext(callCtx, input)
	callCancel()

	// evaluate result
	if err != nil {
		err = fmt.Errorf("SendMessageBatchWithGroup Failed: (Send Action) %w", err)
		return nil, nil, err
	} else {
		if len(output.Successful) > 0 {
			for _, v := range output.Successful {
				successList = append(successList, &SQSSuccessResult{
					Id:                     aws.StringValue(v.Id),
					MessageId:              aws.StringValue(v.MessageId),
					MD5ofMessageBody:       aws.StringValue(v.MD5OfMessageBody),
					MD5ofMessageAttributes: aws.StringValue(v.MD5OfMessageAttributes),
				})
			}
		}

		if len(output.Failed) > 0 {
			for _, v := range output.Failed {
				failedList = append(failedList, &SQSFailResult{
					Id:          aws.StringValue(v.Id),
					Code:        aws.StringValue(v.Code),
					Message:     aws.StringValue(v.Message),
					SenderFault: aws.BoolValue(v.SenderFault),
				})
			}
		}

		return successList, failedList, nil
	}
}
