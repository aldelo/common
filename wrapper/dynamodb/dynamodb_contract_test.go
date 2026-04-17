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

// This is the first test file in the wrapper/dynamodb package. Its purpose is
// to pin observable contracts of exported constants and validation limits that
// downstream repos and this package's own validators depend on, plus
// unit-level tests for the pure conversion helpers added in P0-2 for the
// BatchWriteItemsWithRetry UnprocessedItems retry loop.
//
// Why not an integration test: the wrapper is a thin shim over aws-sdk-go v1
// DynamoDB APIs. Driving the real service from CI would require AWS
// credentials, a live table, and non-deterministic latency. Unit-testing the
// exported constants and the pure helper conversion functions is the
// highest-value coverage we can establish without introducing those CI
// dependencies — and it catches the specific regression classes we care about
// (transaction-limit drift, and typed↔AWS-SDK-shape conversion bugs in the
// new P0-2 retry plumbing).

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"

	"github.com/aws/aws-sdk-go/aws/awserr"
)

// TestMaxTransactItems_ContractPin locks the value of MaxTransactItems to
// 100 — the current AWS limit for TransactWriteItems and TransactGetItems.
//
// Background (P0-1 from remediation-report-2026-04-11):
// AWS raised the per-transaction item limit from 25 to 100 on 2022-09-27.
// Prior to v1.7.9 this wrapper rejected any transaction with more than 25
// items at validation time, so callers could not benefit from the raised
// limit even though the SDK and service both supported it. v1.7.9 bumps the
// validator to 100 via this exported constant.
//
// If AWS ever raises the limit again, or if this wrapper must be pinned to
// a lower value for some compatibility reason, the change MUST be a
// deliberate release event and not an accidental refactor. This test is
// the guardrail.
//
// AWS reference:
//
//	https://docs.aws.amazon.com/amazondynamodb/latest/APIReference/API_TransactWriteItems.html
//	https://docs.aws.amazon.com/amazondynamodb/latest/APIReference/API_TransactGetItems.html
func TestMaxTransactItems_ContractPin(t *testing.T) {
	const wantTransact = 100
	if MaxTransactItems != wantTransact {
		t.Errorf("MaxTransactItems = %d, want %d — "+
			"AWS TransactWriteItems/TransactGetItems cap is %d per request; "+
			"see API_TransactWriteItems.html / API_TransactGetItems.html",
			MaxTransactItems, wantTransact, wantTransact)
	}
}

// TestMaxBatchWriteItems_ContractPin locks MaxBatchWriteItems to 25 — the
// AWS limit for BatchWriteItem, which is UNCHANGED from launch and must
// NOT be accidentally bumped during the same remediation pass that bumped
// MaxTransactItems. The two limits are distinct:
//
//   - TransactWriteItems / TransactGetItems: 100 (raised 2022-09-27)
//   - BatchWriteItem:                         25 (unchanged)
//   - BatchGetItem:                          100
//
// This test exists specifically because a casual "update all the 25s to 100"
// refactor would silently break BatchWriteItem callers — the server rejects
// batches larger than 25 and returns ValidationException.
//
// AWS reference:
//
//	https://docs.aws.amazon.com/amazondynamodb/latest/APIReference/API_BatchWriteItem.html
func TestMaxBatchWriteItems_ContractPin(t *testing.T) {
	const wantBatch = 25
	if MaxBatchWriteItems != wantBatch {
		t.Errorf("MaxBatchWriteItems = %d, want %d — "+
			"AWS BatchWriteItem cap is %d per request (unchanged since launch); "+
			"see API_BatchWriteItem.html",
			MaxBatchWriteItems, wantBatch, wantBatch)
	}
}

// TestTransactAndBatchLimits_AreDistinct is a differential test that catches
// a specific refactor hazard: someone reading only one of the two constants
// and "fixing" the other to match. The constants are intentionally different
// because the AWS service limits they represent are different.
func TestTransactAndBatchLimits_AreDistinct(t *testing.T) {
	if MaxTransactItems == MaxBatchWriteItems {
		t.Errorf("MaxTransactItems (%d) must not equal MaxBatchWriteItems (%d) — "+
			"Transact* APIs and BatchWriteItem have DIFFERENT AWS limits; "+
			"if these match, one of them was changed incorrectly",
			MaxTransactItems, MaxBatchWriteItems)
	}
}

// =============================================================================
// P0-2 — UnprocessedItems retry-loop helper unit tests
// =============================================================================
//
// Background (P0-2 from remediation-report-2026-04-11):
// BatchWriteItemsWithRetry previously only retried on explicit errors from
// do_BatchWriteItem. AWS, however, may return a successful response whose
// BatchWriteItemOutput.UnprocessedItems is non-empty — items that were
// silently deferred due to throttling. Under the old code those items were
// returned to the caller unretried. P0-2 adds a retry loop that calls
// do_BatchWriteItem directly on just the deferred items. That loop depends on
// two pure conversion helpers, tested below without any AWS connection:
//
//   - unprocessedItemsToAwsRequestItems — typed UnprocessedItemsAndKeys →
//     raw aws-sdk-go RequestItems map (for BatchWriteItemInput).
//   - awsRequestItemsToUnprocessedItems — raw aws-sdk-go RequestItems map →
//     typed UnprocessedItemsAndKeys (for returning residuals to the caller).
//
// These tests pin the conversion contracts so future refactors cannot
// silently drop items, lose table groupings, or skip error-escape paths.

// testAttrString is a tiny constructor that builds a string AttributeValue
// without repeating the aws.String + dynamodb.AttributeValue boilerplate at
// every call site. Kept local to the test file — no production value.
func testAttrString(s string) *dynamodb.AttributeValue {
	return &dynamodb.AttributeValue{S: aws.String(s)}
}

// TestUnprocessedItemsToAwsRequestItems_EmptyInput pins the nil/empty fast
// path. The production retry loop relies on returning (nil, 0) to bail out
// early rather than calling do_BatchWriteItem with an empty request map,
// which would otherwise produce a meaningless AWS call.
func TestUnprocessedItemsToAwsRequestItems_EmptyInput(t *testing.T) {
	if got, count := unprocessedItemsToAwsRequestItems(nil); got != nil || count != 0 {
		t.Errorf("nil input: got (%v, %d), want (nil, 0)", got, count)
	}
	if got, count := unprocessedItemsToAwsRequestItems([]*DynamoDBUnprocessedItemsAndKeys{}); got != nil || count != 0 {
		t.Errorf("empty slice: got (%v, %d), want (nil, 0)", got, count)
	}
}

// TestUnprocessedItemsToAwsRequestItems_PutItemsOnly verifies that pure-put
// residuals round-trip into PutRequest entries, grouped by TableName, with
// the correct total count.
func TestUnprocessedItemsToAwsRequestItems_PutItemsOnly(t *testing.T) {
	input := []*DynamoDBUnprocessedItemsAndKeys{
		{
			TableName: "Orders",
			PutItems: []map[string]*dynamodb.AttributeValue{
				{"PK": testAttrString("o1"), "SK": testAttrString("meta")},
				{"PK": testAttrString("o2"), "SK": testAttrString("meta")},
			},
		},
	}
	got, count := unprocessedItemsToAwsRequestItems(input)
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if len(got["Orders"]) != 2 {
		t.Fatalf("Orders bucket: got %d write requests, want 2", len(got["Orders"]))
	}
	for i, wr := range got["Orders"] {
		if wr.PutRequest == nil || wr.PutRequest.Item == nil {
			t.Errorf("entry %d: missing PutRequest/Item", i)
		}
		if wr.DeleteRequest != nil {
			t.Errorf("entry %d: unexpected DeleteRequest on a put-only item", i)
		}
	}
}

// TestUnprocessedItemsToAwsRequestItems_DeleteKeysOnly verifies that pure-
// delete residuals round-trip into DeleteRequest entries. DeleteKeys go
// through dynamodbattribute.MarshalMap, which is the same path the typed
// BatchWriteItems entry point uses, so they must produce equivalent Key
// attribute maps.
func TestUnprocessedItemsToAwsRequestItems_DeleteKeysOnly(t *testing.T) {
	input := []*DynamoDBUnprocessedItemsAndKeys{
		{
			TableName: "Orders",
			DeleteKeys: []*DynamoDBTableKeys{
				{PK: "o1", SK: "meta"},
				{PK: "o2", SK: "meta"},
			},
		},
	}
	got, count := unprocessedItemsToAwsRequestItems(input)
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if len(got["Orders"]) != 2 {
		t.Fatalf("Orders bucket: got %d write requests, want 2", len(got["Orders"]))
	}
	for i, wr := range got["Orders"] {
		if wr.DeleteRequest == nil || wr.DeleteRequest.Key == nil {
			t.Errorf("entry %d: missing DeleteRequest/Key", i)
		}
		if wr.PutRequest != nil {
			t.Errorf("entry %d: unexpected PutRequest on a delete-only item", i)
		}
	}
}

// TestUnprocessedItemsToAwsRequestItems_MixedTablesAndOps verifies that
// multi-table input with a mix of puts and deletes is grouped correctly,
// preserving per-table boundaries and accumulating a correct total count.
// This is the realistic shape of a partial BatchWriteItem response.
func TestUnprocessedItemsToAwsRequestItems_MixedTablesAndOps(t *testing.T) {
	input := []*DynamoDBUnprocessedItemsAndKeys{
		{
			TableName: "Orders",
			PutItems: []map[string]*dynamodb.AttributeValue{
				{"PK": testAttrString("o1")},
			},
			DeleteKeys: []*DynamoDBTableKeys{
				{PK: "o2", SK: "meta"},
			},
		},
		{
			TableName: "Customers",
			PutItems: []map[string]*dynamodb.AttributeValue{
				{"PK": testAttrString("c1")},
				{"PK": testAttrString("c2")},
			},
		},
	}
	got, count := unprocessedItemsToAwsRequestItems(input)
	if count != 4 {
		t.Errorf("count = %d, want 4 (1 put + 1 del + 2 puts)", count)
	}
	if len(got["Orders"]) != 2 {
		t.Errorf("Orders bucket: got %d, want 2", len(got["Orders"]))
	}
	if len(got["Customers"]) != 2 {
		t.Errorf("Customers bucket: got %d, want 2", len(got["Customers"]))
	}
}

// TestUnprocessedItemsToAwsRequestItems_SkipsNilAndEmpty verifies that the
// helper silently skips nil residuals, empty table names, and nil item
// entries without panicking. This matters because the retry loop may feed
// this helper a heterogeneous residual list produced from a previous pass.
func TestUnprocessedItemsToAwsRequestItems_SkipsNilAndEmpty(t *testing.T) {
	input := []*DynamoDBUnprocessedItemsAndKeys{
		nil, // nil entry
		{TableName: "", PutItems: []map[string]*dynamodb.AttributeValue{{"PK": testAttrString("x")}}}, // empty tbl name
		{
			TableName: "Orders",
			PutItems: []map[string]*dynamodb.AttributeValue{
				nil, // nil item inside a real table
				{"PK": testAttrString("o1")},
			},
			DeleteKeys: []*DynamoDBTableKeys{
				nil, // nil delete key
				{PK: "o2", SK: "meta"},
			},
		},
	}
	got, count := unprocessedItemsToAwsRequestItems(input)
	if count != 2 {
		t.Errorf("count = %d, want 2 (only the two valid entries under Orders)", count)
	}
	if _, ok := got[""]; ok {
		t.Errorf("empty table name must not appear in output")
	}
	if len(got["Orders"]) != 2 {
		t.Errorf("Orders bucket: got %d, want 2", len(got["Orders"]))
	}
}

// TestAwsRequestItemsToUnprocessedItems_EmptyInput pins the nil/empty fast
// path. The production retry loop uses this to decide whether the residual
// list it returns to the caller is empty (and should therefore be nil
// rather than a non-nil empty slice).
func TestAwsRequestItemsToUnprocessedItems_EmptyInput(t *testing.T) {
	if got, count := awsRequestItemsToUnprocessedItems(nil); got != nil || count != 0 {
		t.Errorf("nil input: got (%v, %d), want (nil, 0)", got, count)
	}
	if got, count := awsRequestItemsToUnprocessedItems(map[string][]*dynamodb.WriteRequest{}); got != nil || count != 0 {
		t.Errorf("empty map: got (%v, %d), want (nil, 0)", got, count)
	}
}

// TestAwsRequestItemsToUnprocessedItems_RoundTrip verifies that a residual
// produced by unprocessedItemsToAwsRequestItems can be converted back to the
// typed shape, and that item counts are preserved across the round trip.
// This is the symmetry guarantee the retry loop depends on when it returns
// residual items to the caller.
func TestAwsRequestItemsToUnprocessedItems_RoundTrip(t *testing.T) {
	typed := []*DynamoDBUnprocessedItemsAndKeys{
		{
			TableName: "Orders",
			PutItems: []map[string]*dynamodb.AttributeValue{
				{"PK": testAttrString("o1")},
			},
			DeleteKeys: []*DynamoDBTableKeys{
				{PK: "o2", SK: "meta"},
			},
		},
	}

	// typed → AWS
	awsShape, fwdCount := unprocessedItemsToAwsRequestItems(typed)
	if fwdCount != 2 {
		t.Fatalf("forward count = %d, want 2", fwdCount)
	}

	// AWS → typed
	back, backCount := awsRequestItemsToUnprocessedItems(awsShape)
	if backCount != 2 {
		t.Errorf("round-trip count = %d, want 2", backCount)
	}
	if len(back) != 1 {
		t.Fatalf("round-trip table count = %d, want 1", len(back))
	}
	if back[0].TableName != "Orders" {
		t.Errorf("round-trip table name = %q, want %q", back[0].TableName, "Orders")
	}
	if len(back[0].PutItems) != 1 {
		t.Errorf("round-trip PutItems = %d, want 1", len(back[0].PutItems))
	}
	if len(back[0].DeleteKeys) != 1 {
		t.Errorf("round-trip DeleteKeys = %d, want 1", len(back[0].DeleteKeys))
	}
}

// TestAwsRequestItemsToUnprocessedItems_SkipsNilAndEmpty verifies that the
// helper silently drops empty tables, nil requests, and tables that end up
// with zero items after filtering — so the caller never sees a residual
// with TableName set but PutItems and DeleteKeys both empty (which would
// be a confusing no-op record).
func TestAwsRequestItemsToUnprocessedItems_SkipsNilAndEmpty(t *testing.T) {
	input := map[string][]*dynamodb.WriteRequest{
		"":      {{PutRequest: &dynamodb.PutRequest{Item: map[string]*dynamodb.AttributeValue{"PK": testAttrString("x")}}}},
		"Empty": {}, // no requests at all
		"Orders": {
			nil,
			{PutRequest: &dynamodb.PutRequest{Item: map[string]*dynamodb.AttributeValue{"PK": testAttrString("o1")}}},
		},
	}
	got, count := awsRequestItemsToUnprocessedItems(input)
	if count != 1 {
		t.Errorf("count = %d, want 1 (only the single valid Orders put)", count)
	}
	for _, r := range got {
		if r.TableName == "" {
			t.Errorf("empty table name leaked into output")
		}
		if r.TableName == "Empty" {
			t.Errorf("empty-requests table leaked into output")
		}
		if len(r.PutItems) == 0 && len(r.DeleteKeys) == 0 {
			t.Errorf("empty residual table %q leaked into output", r.TableName)
		}
	}
}

// =============================================================================
// DB-F1/AD-1 — SuppressError must NOT suppress final failure after retry
// exhaustion. These tests pin the corrected contract.
// =============================================================================

// TestHandleError_SuppressErrorAlwaysHasAllowRetry verifies that every error
// code classified with SuppressError=true also has AllowRetry=true.
// This is the foundational invariant: SuppressError means "transient, retry
// silently" — it should never mean "hide the failure from the caller."
func TestHandleError_SuppressErrorAlwaysHasAllowRetry(t *testing.T) {
	d := &DynamoDB{} // nil connection is fine — handleError doesn't use it

	// Error codes that should produce SuppressError=true
	suppressCodes := []string{
		dynamodb.ErrCodeProvisionedThroughputExceededException,
		dynamodb.ErrCodeRequestLimitExceeded,
		dynamodb.ErrCodeInternalServerError,
	}
	for _, code := range suppressCodes {
		aerr := awserr.New(code, "test", fmt.Errorf("test"))
		ddbErr := d.handleError(aerr)
		if ddbErr == nil {
			t.Fatalf("handleError(%s) returned nil", code)
		}
		if !ddbErr.SuppressError {
			t.Errorf("handleError(%s): SuppressError = false, want true", code)
		}
		if !ddbErr.AllowRetry {
			t.Errorf("handleError(%s): AllowRetry = false, want true — "+
				"SuppressError without AllowRetry is the old silent-data-loss bug", code)
		}
	}
}

// TestHandleError_NonSuppressErrorCodes verifies that error codes NOT in the
// suppress set produce SuppressError=false, ensuring callers always see the
// failure on the first attempt.
func TestHandleError_NonSuppressErrorCodes(t *testing.T) {
	d := &DynamoDB{}

	nonSuppressCodes := []string{
		dynamodb.ErrCodeConditionalCheckFailedException,
		dynamodb.ErrCodeResourceNotFoundException,
		dynamodb.ErrCodeItemCollectionSizeLimitExceededException,
		dynamodb.ErrCodeTransactionCanceledException,
	}
	for _, code := range nonSuppressCodes {
		aerr := awserr.New(code, "test", fmt.Errorf("test"))
		ddbErr := d.handleError(aerr)
		if ddbErr == nil {
			t.Fatalf("handleError(%s) returned nil", code)
		}
		if ddbErr.SuppressError {
			t.Errorf("handleError(%s): SuppressError = true, want false", code)
		}
	}
}

// TestWithRetry_ReturnsErrorOnRetryExhaustion verifies that *WithRetry methods
// return a non-nil error when retries are exhausted, even for error codes that
// have SuppressError=true. This is the core fix for DB-F1/AD-1.
//
// We test PutItemWithRetry with maxRetries=0 on a nil-connection DynamoDB
// object. The nil connection causes PutItem to fail immediately. Since
// maxRetries=0 means no retries allowed, the method must return the error.
// Before the fix, SuppressError=true errors would return nil here.
func TestWithRetry_ReturnsErrorOnRetryExhaustion(t *testing.T) {
	d := &DynamoDB{
		TableName: "test-table",
		PKName:    "PK",
		SKName:    "SK",
	}
	// PutItemWithRetry with maxRetries=0 on a nil connection.
	// The call will fail (nil connection), and with maxRetries=0
	// the error must be returned — never suppressed.
	err := d.PutItemWithRetry(0, map[string]string{"PK": "test"}, nil)
	if err == nil {
		t.Fatal("PutItemWithRetry(maxRetries=0) returned nil error on failed operation — " +
			"this is the DB-F1/AD-1 silent data loss bug")
	}
	if err.ErrorMessage == "" {
		t.Error("PutItemWithRetry error has empty ErrorMessage")
	}
}

// TestWithRetry_ReturnsErrorOnMaxRetriesExhausted tests with maxRetries=1
// to verify that even after one retry attempt, the error is still returned
// when all retries are exhausted.
func TestWithRetry_ReturnsErrorOnMaxRetriesExhausted(t *testing.T) {
	d := &DynamoDB{
		TableName: "test-table",
		PKName:    "PK",
		SKName:    "SK",
	}
	// maxRetries=1: will attempt once, fail, retry once, fail again.
	// The final failure must be returned.
	err := d.PutItemWithRetry(1, map[string]string{"PK": "test"}, nil)
	if err == nil {
		t.Fatal("PutItemWithRetry(maxRetries=1) returned nil error after retry exhaustion — " +
			"retry-exhausted errors must always be returned to caller")
	}
}

// =============================================================================
// DB-F2 — Unbounded scan warning tests
// =============================================================================

// TestScanItems_UnboundedScanWarning verifies that ScanItems logs a warning
// when called without a Limit (pageLimit == nil), which signals a potential
// full table scan, and does NOT log the warning when a Limit is provided.
//
// The DynamoDB struct has no connection, so the call fails after the warning
// check with "DynamoDB Connection is Required". We only care about the log
// output — the error is expected and ignored.
func TestScanItems_UnboundedScanWarning(t *testing.T) {
	d := &DynamoDB{
		TableName: "test-table",
	}

	filter := expression.Name("pk").Equal(expression.Value("x"))
	var result []map[string]string

	t.Run("no_limit_logs_warning", func(t *testing.T) {
		var buf bytes.Buffer
		log.SetOutput(&buf)
		defer log.SetOutput(nil) // restore default

		// pageLimit = nil → should trigger warning
		_, _ = d.ScanItems(&result, nil, nil, nil, nil, false, nil, nil, filter)

		output := buf.String()
		if !strings.Contains(output, "[WARN] DynamoDB ScanItems called without Limit") {
			t.Errorf("expected unbounded scan warning in log output, got: %q", output)
		}
		if !strings.Contains(output, "test-table") {
			t.Errorf("expected table name in warning, got: %q", output)
		}
	})

	t.Run("with_limit_no_warning", func(t *testing.T) {
		var buf bytes.Buffer
		log.SetOutput(&buf)
		defer log.SetOutput(nil)

		limit := aws.Int64(100)
		// pageLimit = 100 → should NOT trigger warning
		_, _ = d.ScanItems(&result, nil, nil, nil, limit, false, nil, nil, filter)

		output := buf.String()
		if strings.Contains(output, "[WARN] DynamoDB ScanItems called without Limit") {
			t.Errorf("did not expect unbounded scan warning when Limit is set, got: %q", output)
		}
	})
}

// =============================================================================
// DB-F3 — Timeout clamping warning tests
// =============================================================================

// TestTimeoutClamping_WarningLogged verifies that *WithRetry methods log a
// [WARN] when the caller-supplied timeout is clamped to the allowed range.
//
// There are two clamping tiers:
//   - 5–15s: PutItemWithRetry, DeleteItemWithRetry, GetItemWithRetry,
//     QueryPaginationDataWithRetry, QueryItemsWithRetry,
//     BatchGetItemsWithRetry, BatchDeleteItemsWithRetry,
//     TransactionGetItemsWithRetry
//   - 10–30s: UpdateItemWithRetry, RemoveItemAttributeWithRetry,
//     ScanItemsWithRetry, BatchWriteItemsWithRetry,
//     TransactionWriteItemsWithRetry
//
// Each sub-test calls the method with a timeout below the minimum and above
// the maximum, then checks that the log contains the expected warning. The
// DynamoDB struct has no AWS connection, so the call itself fails — but the
// clamping warning fires before the AWS call, which is all we verify.
func TestTimeoutClamping_WarningLogged(t *testing.T) {
	d := &DynamoDB{
		TableName: "clamp-test-table",
		PKName:    "PK",
		SKName:    "SK",
	}

	// helper: run fn, capture log, check for expected substring
	assertLogContains := func(t *testing.T, label string, fn func(), expected string) {
		t.Helper()
		var buf bytes.Buffer
		log.SetOutput(&buf)
		defer log.SetOutput(nil)

		fn()

		output := buf.String()
		if !strings.Contains(output, expected) {
			t.Errorf("%s: expected log to contain %q, got: %q", label, expected, output)
		}
	}

	// helper: run fn, capture log, check substring is absent
	assertLogNotContains := func(t *testing.T, label string, fn func(), unexpected string) {
		t.Helper()
		var buf bytes.Buffer
		log.SetOutput(&buf)
		defer log.SetOutput(nil)

		fn()

		output := buf.String()
		if strings.Contains(output, unexpected) {
			t.Errorf("%s: did not expect log to contain %q, got: %q", label, unexpected, output)
		}
	}

	// --- PutItemWithRetry (5–15s tier) ---
	t.Run("PutItemWithRetry_below_min", func(t *testing.T) {
		dur := 1 * time.Second
		assertLogContains(t, "below_min", func() {
			_ = d.PutItemWithRetry(0, map[string]string{"PK": "x"}, &dur)
		}, "[WARN] DynamoDB PutItemWithRetry timeout 1s clamped to minimum 5s for table clamp-test-table")
	})

	t.Run("PutItemWithRetry_above_max", func(t *testing.T) {
		dur := 30 * time.Second
		assertLogContains(t, "above_max", func() {
			_ = d.PutItemWithRetry(0, map[string]string{"PK": "x"}, &dur)
		}, "[WARN] DynamoDB PutItemWithRetry timeout 30s clamped to maximum 15s for table clamp-test-table")
	})

	t.Run("PutItemWithRetry_within_range_no_warning", func(t *testing.T) {
		dur := 10 * time.Second
		assertLogNotContains(t, "within_range", func() {
			_ = d.PutItemWithRetry(0, map[string]string{"PK": "x"}, &dur)
		}, "[WARN] DynamoDB PutItemWithRetry timeout")
	})

	// --- UpdateItemWithRetry (10–30s tier) ---
	t.Run("UpdateItemWithRetry_below_min", func(t *testing.T) {
		dur := 3 * time.Second
		assertLogContains(t, "below_min", func() {
			_ = d.UpdateItemWithRetry(0, "pk1", "sk1", "", "", nil, nil, &dur)
		}, "[WARN] DynamoDB UpdateItemWithRetry timeout 3s clamped to minimum 10s for table clamp-test-table")
	})

	t.Run("UpdateItemWithRetry_above_max", func(t *testing.T) {
		dur := 60 * time.Second
		assertLogContains(t, "above_max", func() {
			_ = d.UpdateItemWithRetry(0, "pk1", "sk1", "", "", nil, nil, &dur)
		}, "[WARN] DynamoDB UpdateItemWithRetry timeout 1m0s clamped to maximum 30s for table clamp-test-table")
	})

	// --- DeleteItemWithRetry (5–15s tier) ---
	t.Run("DeleteItemWithRetry_below_min", func(t *testing.T) {
		dur := 2 * time.Second
		assertLogContains(t, "below_min", func() {
			_ = d.DeleteItemWithRetry(0, "pk1", "sk1", &dur)
		}, "[WARN] DynamoDB DeleteItemWithRetry timeout 2s clamped to minimum 5s for table clamp-test-table")
	})

	t.Run("DeleteItemWithRetry_above_max", func(t *testing.T) {
		dur := 20 * time.Second
		assertLogContains(t, "above_max", func() {
			_ = d.DeleteItemWithRetry(0, "pk1", "sk1", &dur)
		}, "[WARN] DynamoDB DeleteItemWithRetry timeout 20s clamped to maximum 15s for table clamp-test-table")
	})

	// --- GetItemWithRetry (5–15s tier) ---
	t.Run("GetItemWithRetry_below_min", func(t *testing.T) {
		dur := 1 * time.Second
		var result map[string]string
		assertLogContains(t, "below_min", func() {
			_ = d.GetItemWithRetry(0, &result, "pk1", "sk1", &dur, nil)
		}, "[WARN] DynamoDB GetItemWithRetry timeout 1s clamped to minimum 5s for table clamp-test-table")
	})

	// --- TransactionGetItemsWithRetry (5–15s tier) ---
	t.Run("TransactionGetItemsWithRetry_below_min", func(t *testing.T) {
		dur := 2 * time.Second
		assertLogContains(t, "below_min", func() {
			_, _ = d.TransactionGetItemsWithRetry(0, &dur)
		}, "[WARN] DynamoDB TransactionGetItemsWithRetry timeout 2s clamped to minimum 5s for table clamp-test-table")
	})

	// --- TransactionWriteItemsWithRetry (10–30s tier) ---
	t.Run("TransactionWriteItemsWithRetry_above_max", func(t *testing.T) {
		dur := 45 * time.Second
		assertLogContains(t, "above_max", func() {
			_, _ = d.TransactionWriteItemsWithRetry(0, &dur)
		}, "[WARN] DynamoDB TransactionWriteItemsWithRetry timeout 45s clamped to maximum 30s for table clamp-test-table")
	})
}

// stringerFunc adapts a func() string to fmt.Stringer without heap-escaping
// the closure on every call (it escapes once at test setup — acceptable for
// tests; production callers pass *dynamodb.PutItemInput which has a real
// String() method).
type stringerFunc func() string

func (s stringerFunc) String() string { return s() }

// TestSetLastExecuteParamsPayload_DefaultEnabled_P1PERF2 pins the v1.8.6
// observable contract for LastExecuteParamsPayload: with the default zero
// value of DisableLastExecuteParamsPayload (false), setLastExecuteParamsPayload
// MUST record the prefix + stringer.String() concatenation into the public
// LastExecuteParamsPayload field so downstream consumers that read it for
// diagnostic logging continue to see the same payload they saw in v1.8.6.
//
// Regression guard for P1-PERF-2 (v1.8.7): the opt-out flag must default to
// "record" so no caller loses diagnostics without explicitly opting out.
// If a future refactor flips the sense of the flag or removes the helper,
// this test fails and the regression is caught at build time.
func TestSetLastExecuteParamsPayload_DefaultEnabled_P1PERF2(t *testing.T) {
	called := 0
	stringer := stringerFunc(func() string {
		called++
		return "FAKE_INPUT"
	})
	d := &DynamoDB{}
	d.setLastExecuteParamsPayload("TestOp = ", stringer)
	if called != 1 {
		t.Fatalf("stringer.String() calls = %d, want 1 (default path must record)", called)
	}
	if got, want := d.LastExecuteParamsPayload, "TestOp = FAKE_INPUT"; got != want {
		t.Fatalf("LastExecuteParamsPayload = %q, want %q — default contract broken", got, want)
	}
}

// TestSetLastExecuteParamsPayload_DisabledSkipsAllocation_P1PERF2 verifies the
// opt-out path: when DisableLastExecuteParamsPayload is true, the helper MUST
// NOT call stringer.String() (so no allocation, no mutex acquisition, no GC
// pressure) and the field MUST remain untouched.
//
// This is the whole point of P1-PERF-2: at 10K DDB ops/s, every String() call
// that can be skipped is allocation avoided on the hot path.
func TestSetLastExecuteParamsPayload_DisabledSkipsAllocation_P1PERF2(t *testing.T) {
	called := 0
	stringer := stringerFunc(func() string {
		called++
		return "FAKE_INPUT"
	})
	d := &DynamoDB{DisableLastExecuteParamsPayload: true}
	d.LastExecuteParamsPayload = "SENTINEL"
	d.setLastExecuteParamsPayload("TestOp = ", stringer)
	if called != 0 {
		t.Fatalf("stringer.String() calls = %d, want 0 (disabled path must not serialize)", called)
	}
	if got, want := d.LastExecuteParamsPayload, "SENTINEL"; got != want {
		t.Fatalf("LastExecuteParamsPayload = %q, want %q — disabled path must not mutate field", got, want)
	}
}
