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
// downstream repos and this package's own validators depend on.
//
// Why not an integration test: the wrapper is a thin shim over aws-sdk-go v1
// DynamoDB APIs. Driving the real service from CI would require AWS
// credentials, a live table, and non-deterministic latency. Unit-testing the
// exported constants is the highest-value contract pin we can establish
// without introducing those CI dependencies, and it catches the specific
// regression class we care about (a future change silently drifting the
// transaction limit away from the AWS-documented value).

import (
	"testing"
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
