package dynamodb

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

// Tests for the diagnostic detail handleError puts into DynamoDBError.ErrorMessage.
//
// These pin behaviour that only shows up in a log line, which is exactly the kind that
// regresses silently: the failure that motivated them arrived in production as
// "[AWS] Unknown - An unknown error has occurred - OrigErr = Nil" and was not
// attributable to anything, because the request id had been dropped one frame earlier.
//
// handleError touches no field of the receiver once past its nil guards, so an empty
// &DynamoDB{} is a sufficient receiver and no AWS credentials or live table are needed.

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

// A DAX server error that the DAX SDK could not map onto a DynamoDB exception arrives
// exactly like this: code "Unknown", a generic message, and a nil OrigErr. The request id
// is the only thing that identifies it, so it has to survive into the message.
func TestHandleError_UnknownRequestFailure_KeepsRequestIDAndStatus(t *testing.T) {
	d := &DynamoDB{}

	err := awserr.NewRequestFailure(
		awserr.New("Unknown", "An unknown error has occurred", nil),
		500,
		"dax-req-0123456789",
	)

	got := d.handleError(err, transactionWriteItemsErrorPrefix)

	if got == nil {
		t.Fatal("expected a DynamoDBError, got nil")
	}

	for _, want := range []string{
		"RequestID = dax-req-0123456789",
		"HttpStatus = 500",
		"OrigErr = Nil",
	} {
		if !strings.Contains(got.ErrorMessage, want) {
			t.Errorf("ErrorMessage missing %q\ngot: %s", want, got.ErrorMessage)
		}
	}

	// the caller supplied prefix must no longer assert a cancellation that did not happen
	if strings.Contains(got.ErrorMessage, "Transaction Canceled") {
		t.Errorf("ErrorMessage should not claim the transaction was cancelled\ngot: %s", got.ErrorMessage)
	}
}

// A plain awserr with no request id must not gain empty trailing fields.
func TestHandleError_PlainAwsError_AddsNoEmptyDetail(t *testing.T) {
	d := &DynamoDB{}

	got := d.handleError(awserr.New(dynamodb.ErrCodeResourceNotFoundException, "table missing", nil))

	if got == nil {
		t.Fatal("expected a DynamoDBError, got nil")
	}

	if strings.Contains(got.ErrorMessage, "RequestID =") || strings.Contains(got.ErrorMessage, "HttpStatus =") {
		t.Errorf("ErrorMessage gained an empty detail field\ngot: %s", got.ErrorMessage)
	}
}

// A real TransactionCanceledException carries per item reasons. They are the only thing
// that says which item failed and why, and they were previously discarded.
func TestHandleError_TransactionCanceled_SurfacesCancellationReasons(t *testing.T) {
	d := &DynamoDB{}

	err := &dynamodb.TransactionCanceledException{
		Message_: aws.String("Transaction cancelled, please refer cancellation reasons for specific reasons"),
		CancellationReasons: []*dynamodb.CancellationReason{
			{Code: aws.String("None")},
			nil, // a nil entry must not panic
			{Code: nil, Message: aws.String("no code")}, // a nil code must not panic
			{
				Code:    aws.String("ConditionalCheckFailed"),
				Message: aws.String("The conditional request failed"),
				// Item is deliberately populated here: neither its keys nor its values may
				// reach the message. On a payments table these are card and account data.
				Item: map[string]*dynamodb.AttributeValue{
					"CardTokenMustNotAppear": {S: aws.String("value-must-not-be-logged")},
				},
			},
			{Code: aws.String("TransactionConflict")}, // no message: code alone must still render
		},
	}

	got := d.handleError(err, transactionWriteItemsErrorPrefix)

	if got == nil {
		t.Fatal("expected a DynamoDBError, got nil")
	}

	for _, want := range []string{
		"[3] ConditionalCheckFailed: The conditional request failed",
		"[4] TransactionConflict",
	} {
		if !strings.Contains(got.ErrorMessage, want) {
			t.Errorf("ErrorMessage missing reason %q\ngot: %s", want, got.ErrorMessage)
		}
	}

	// "None" and code-less entries mark items that did not cause the cancellation, and are noise
	for _, unwanted := range []string{"[0]", "[1]", "[2]"} {
		if strings.Contains(got.ErrorMessage, unwanted) {
			t.Errorf("ErrorMessage should skip reason %s\ngot: %s", unwanted, got.ErrorMessage)
		}
	}

	// item keys AND values are card data on a payments table and must never be logged
	for _, leak := range []string{"CardTokenMustNotAppear", "value-must-not-be-logged"} {
		if strings.Contains(got.ErrorMessage, leak) {
			t.Errorf("ErrorMessage leaked CancellationReason.Item (%q)\ngot: %s", leak, got.ErrorMessage)
		}
	}
}

// Regression test for a review finding: TransactionConditionalCheckFailed must NOT be derived
// from the cancellation reason codes.
//
// It is not a diagnostic flag. Crud.Set and Crud.Update turn it into the
// "[Possible Unique Attribute Duplicate Blocked]" sentinel and four call sites across
// go-ms-apgs-web-portals-api, go-ms-apgs-fiserv-provider and go-ms-apgs-tsys-transit-provider
// branch on that text. A single transaction carries both the caller's own ConditionExpression
// and the uniqueness guard's attribute_not_exists(PK) puts, so deriving the flag from any
// ConditionalCheckFailed reason would report a failed optimistic concurrency check as a unique
// key duplicate.
func TestHandleError_TransactionCanceled_ReasonCodesDoNotSetDuplicateFlag(t *testing.T) {
	d := &DynamoDB{}

	err := &dynamodb.TransactionCanceledException{
		// note: the message does NOT mention ConditionalCheckFailed, only the reasons do
		Message_: aws.String("Transaction cancelled, please refer cancellation reasons for specific reasons"),
		CancellationReasons: []*dynamodb.CancellationReason{
			{Code: aws.String("ConditionalCheckFailed"), Message: aws.String("The conditional request failed")},
		},
	}

	got := d.handleError(err, transactionWriteItemsErrorPrefix)

	if got == nil {
		t.Fatal("expected a DynamoDBError, got nil")
	}

	// the reason is still reported ...
	if !strings.Contains(got.ErrorMessage, "ConditionalCheckFailed") {
		t.Errorf("the reason should still be surfaced in the message\ngot: %s", got.ErrorMessage)
	}

	// ... but it must not be promoted into the duplicate-blocked classification
	if got.TransactionConditionalCheckFailed {
		t.Error("TransactionConditionalCheckFailed must not be derived from cancellation reason codes")
	}
}

// The prefix attached to every TransactionWriteItems failure must not name a cause. It is
// applied to any failure of the underlying call, including timeouts and DAX faults.
func TestTransactionWriteItemsErrorPrefix_IsNeutral(t *testing.T) {
	for _, banned := range []string{"Canceled", "Cancelled", "Throttl", "Timeout"} {
		if strings.Contains(transactionWriteItemsErrorPrefix, banned) {
			t.Errorf("prefix %q names a cause it cannot know (%q)", transactionWriteItemsErrorPrefix, banned)
		}
	}
}

// The flag must still be set when the reason codes are absent and only the message says so,
// which is how it behaved before cancellation reasons were parsed.
func TestHandleError_TransactionCanceled_FlagStillSetFromMessageAlone(t *testing.T) {
	d := &DynamoDB{}

	err := awserr.New(
		dynamodb.ErrCodeTransactionCanceledException,
		"Transaction cancelled ... [None, ConditionalCheckFailed]",
		nil,
	)

	got := d.handleError(err)

	if got == nil {
		t.Fatal("expected a DynamoDBError, got nil")
	}

	if !got.TransactionConditionalCheckFailed {
		t.Error("TransactionConditionalCheckFailed should still be derived from the message")
	}
}
