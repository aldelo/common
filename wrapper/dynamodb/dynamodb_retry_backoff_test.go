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

// Pins the retry backoff schedule. The *WithRetry methods previously slept a
// FIXED 500ms (backoff) or 100ms (no-backoff) on every attempt — not the
// exponential backoff AWS recommends, which under sustained throttling lets many
// callers retry in lockstep (thundering herd). retryBackoffDelay replaces the flat
// sleeps with an exponentially growing delay derived from the depleting retry
// budget (no signature change to the recursive methods). These tests pin the
// schedule's shape so a future refactor cannot silently flatten it again.

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

// TestRetryBackoffDelay_MonotonicAndExponential verifies the delay grows as the
// retry budget depletes (remainingRetries decreases on each recursive attempt),
// doubling in the un-floored region — i.e. real exponential backoff.
func TestRetryBackoffDelay_MonotonicAndExponential(t *testing.T) {
	// remainingRetries goes high->low as retries are consumed; delay must be
	// non-decreasing as remainingRetries decreases.
	var prev time.Duration
	for r := uint(10); r >= 1; r-- {
		d := retryBackoffDelay(r, true)
		if d < prev {
			t.Errorf("retryBackoffDelay(%d,true)=%v < previous %v — schedule must be non-decreasing as budget depletes", r, d, prev)
		}
		prev = d
	}

	// doubling in the exponential (un-floored) region for needsBackOff: r=3->500ms, r=2->1s, r=1->2s
	if got := retryBackoffDelay(3, true); got != 500*time.Millisecond {
		t.Errorf("retryBackoffDelay(3,true)=%v, want 500ms", got)
	}
	if got := retryBackoffDelay(2, true); got != 1*time.Second {
		t.Errorf("retryBackoffDelay(2,true)=%v, want 1s", got)
	}
	if got := retryBackoffDelay(1, true); got != 2*time.Second {
		t.Errorf("retryBackoffDelay(1,true)=%v, want 2s (final-retry cap)", got)
	}
}

// TestRetryBackoffDelay_FlooredCappedClamped pins the floor, the ceiling, and the
// clamp on remainingRetries.
func TestRetryBackoffDelay_FlooredCappedClamped(t *testing.T) {
	// floor: early attempts of a large budget still pause a minimum
	if got := retryBackoffDelay(10, true); got != 100*time.Millisecond {
		t.Errorf("retryBackoffDelay(10,true)=%v, want floor 100ms", got)
	}
	if got := retryBackoffDelay(10, false); got != 50*time.Millisecond {
		t.Errorf("retryBackoffDelay(10,false)=%v, want floor 50ms", got)
	}
	// ceiling: the largest single delay is the final-retry value
	if got := retryBackoffDelay(1, true); got > 2*time.Second {
		t.Errorf("retryBackoffDelay(1,true)=%v exceeds 2s cap", got)
	}
	if got := retryBackoffDelay(1, false); got > 500*time.Millisecond {
		t.Errorf("retryBackoffDelay(1,false)=%v exceeds 500ms cap", got)
	}
	// clamp: values above the maxRetries band behave like the band max
	if retryBackoffDelay(100, true) != retryBackoffDelay(10, true) {
		t.Error("remainingRetries must be clamped to the [1,10] band (same as maxRetries clamp)")
	}
	// zero budget => no sleep
	if retryBackoffDelay(0, true) != 0 || retryBackoffDelay(0, false) != 0 {
		t.Error("retryBackoffDelay(0, *) must be 0 — no retry, no sleep")
	}
}

// TestRetryBackoffDelay_BackoffExceedsTransient verifies the throttle/5xx path
// (needsBackOff) waits at least as long as the transient path at every step.
func TestRetryBackoffDelay_BackoffExceedsTransient(t *testing.T) {
	for r := uint(1); r <= 10; r++ {
		bo := retryBackoffDelay(r, true)
		tr := retryBackoffDelay(r, false)
		if bo < tr {
			t.Errorf("at remaining=%d: needsBackOff delay %v < transient delay %v", r, bo, tr)
		}
	}
}

// TestHandleError_InternalServerError_NowBacksOff pins the classification fix:
// DynamoDB InternalServerError is a transient server fault that AWS says to retry
// with EXPONENTIAL BACKOFF. It was previously classified retry-WITHOUT-backoff,
// which under an ISE brownout hammered the partition at a flat 100ms. It must now
// be AllowRetry + RetryNeedsBackOff.
func TestHandleError_InternalServerError_NowBacksOff(t *testing.T) {
	d := &DynamoDB{}
	aerr := awserr.New(dynamodb.ErrCodeInternalServerError, "test", fmt.Errorf("test"))
	ddbErr := d.handleError(aerr)
	if ddbErr == nil {
		t.Fatal("handleError(InternalServerError) returned nil")
	}
	if !ddbErr.AllowRetry {
		t.Error("InternalServerError must remain AllowRetry=true")
	}
	if !ddbErr.RetryNeedsBackOff {
		t.Error("InternalServerError must now RetryNeedsBackOff=true (AWS recommends exponential backoff for 5xx)")
	}
}
