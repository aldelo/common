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

// This file pins the fail-stop guardrails added for the two unbounded DynamoDB
// read loops in this wrapper:
//
//  1. do_Query_Pagination_Data — the "pre-walk" that drives QueryPaginationData*.
//     It walks EVERY page of a query result just to collect each page's
//     ExclusiveStartKey for pre-building pagination buttons. That is
//     O(total-matching-items / itemsPerPage) DynamoDB round-trips per list call.
//  2. QueryPagedItemsWithRetry — the unbounded full-result gather reached by
//     crud.Query() (taken when itemsPerPage<=0). It accumulates EVERY matching
//     item into one in-memory slice.
//
// Both are latent O(N)/OOM bombs: harmless while partitions are tiny, fatal for
// a large partition. The guardrail must FAIL-STOP with a typed sentinel
// (ErrResultSetTooLarge) — it must NEVER truncate, because silent truncation of
// a payment-transaction list is a data-integrity regression.
//
// Backward-compat is the hard constraint (validation-not-stricter-than-runtime):
// the hard caps default to 0 = unlimited, so every existing DAL-service caller
// behaves byte-identically to today. A soft [WARN] threshold (default on) gives
// early visibility of a growing partition WITHOUT ever failing a legitimate
// caller.
//
// Why not an integration test: the cap/warn DECISION logic is pure and is
// exercised here directly, and the retry-classification contract is exercised
// through handleError — neither needs an AWS connection. Driving the actual
// QueryPages/Query loops would require live AWS, credentials and a large table,
// which CI cannot provide deterministically.

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// =============================================================================
// Pure cap-decision helpers — the heart of the fail-stop
// =============================================================================

// TestExceedsHardCap_ZeroOrNegativeCapIsUnlimited pins the load-bearing
// backward-compat contract: a cap <= 0 means "unlimited", so the helper returns
// false no matter how large the running count is. The zero value of the new
// MaxQuery* fields therefore preserves the pre-cap (today's) behavior exactly.
func TestExceedsHardCap_ZeroOrNegativeCapIsUnlimited(t *testing.T) {
	for _, capLimit := range []int64{0, -1, -1000} {
		if exceedsHardCap(1<<62, capLimit) {
			t.Errorf("exceedsHardCap(huge, %d) = true, want false — cap<=0 must mean unlimited (backward-compat)", capLimit)
		}
		if exceedsHardCap(0, capLimit) {
			t.Errorf("exceedsHardCap(0, %d) = true, want false", capLimit)
		}
	}
}

// TestExceedsHardCap_Boundary pins the exact fail-stop boundary: the cap fires
// at-or-above the limit, not just strictly above. Reaching the cap means the
// loop has already gathered cap items/pages, which is precisely the work we
// want bounded, so >= (not >) is correct.
func TestExceedsHardCap_Boundary(t *testing.T) {
	const capLimit = int64(100)
	cases := []struct {
		count int64
		want  bool
	}{
		{99, false},
		{100, true},
		{101, true},
	}
	for _, c := range cases {
		if got := exceedsHardCap(c.count, capLimit); got != c.want {
			t.Errorf("exceedsHardCap(%d, %d) = %v, want %v", c.count, capLimit, got, c.want)
		}
	}
}

// TestEffectiveWarnThreshold pins the three-way warn-field convention:
//
//	field == 0  -> built-in default (warn is on by default)
//	field <  0  -> 0 (warn explicitly disabled)
//	field >  0  -> field (custom threshold)
//
// This is deliberately different from the hard-cap convention (0 = unlimited)
// because a [WARN] has no functional consequence, so default-on is safe and
// useful; disabling requires an explicit negative opt-out.
func TestEffectiveWarnThreshold(t *testing.T) {
	const builtin = int64(1000)
	cases := []struct {
		field int64
		want  int64
	}{
		{0, builtin},   // unset -> default on
		{-1, 0},        // negative -> disabled
		{-9999, 0},     // any negative -> disabled
		{1, 1},         // custom low
		{50000, 50000}, // custom high
	}
	for _, c := range cases {
		if got := effectiveWarnThreshold(c.field, builtin); got != c.want {
			t.Errorf("effectiveWarnThreshold(%d, %d) = %d, want %d", c.field, builtin, got, c.want)
		}
	}
}

// TestShouldWarnAtCrossing_FiresExactlyOnce verifies the warn fires once, on the
// transition from below-threshold to at-or-above-threshold. Both loops increment
// their running count (the pre-walk by 1 per page; the gather by up to a page of
// items), so a crossing predicate keyed on (prev, new) fires once for either
// step size and never spams the log.
func TestShouldWarnAtCrossing_FiresExactlyOnce(t *testing.T) {
	const warn = int64(1000)

	// pre-walk style: count increments by 1 each page
	if shouldWarnAtCrossing(998, 999, warn) {
		t.Error("below threshold must not warn")
	}
	if !shouldWarnAtCrossing(999, 1000, warn) {
		t.Error("exact crossing (999->1000) must warn")
	}
	if shouldWarnAtCrossing(1000, 1001, warn) {
		t.Error("already past threshold must not warn again (no spam)")
	}

	// gather style: count jumps by a full page, overshooting the threshold
	if !shouldWarnAtCrossing(800, 1050, warn) {
		t.Error("overshoot crossing (800->1050) must warn exactly once")
	}
	if shouldWarnAtCrossing(1050, 1300, warn) {
		t.Error("subsequent gather steps past threshold must not warn again")
	}
}

// TestShouldWarnAtCrossing_DisabledThresholdNeverWarns confirms a 0/negative
// effective threshold disables the warn entirely.
func TestShouldWarnAtCrossing_DisabledThresholdNeverWarns(t *testing.T) {
	for _, warn := range []int64{0, -1} {
		if shouldWarnAtCrossing(0, 1<<62, warn) {
			t.Errorf("shouldWarnAtCrossing with warn=%d must never fire", warn)
		}
	}
}

// =============================================================================
// Typed sentinel — detectability + non-retryability
// =============================================================================

// TestNewResultSetTooLargeError_IsErrorsIsDetectable verifies the constructed
// error both wraps the package sentinel (so plain-error callers, e.g. the
// QueryPagedItemsWithRetry path, can use errors.Is) AND carries actionable
// context (operation, count, cap, table) in its message for operators.
func TestNewResultSetTooLargeError_IsErrorsIsDetectable(t *testing.T) {
	err := newResultSetTooLargeError("QueryPagedItemsWithRetry", 250, 100, "live-edc-tran-2026-06")
	if err == nil {
		t.Fatal("newResultSetTooLargeError returned nil")
	}
	if !errors.Is(err, ErrResultSetTooLarge) {
		t.Errorf("errors.Is(err, ErrResultSetTooLarge) = false, want true — caller cannot detect the fail-stop")
	}
	msg := err.Error()
	for _, want := range []string{"QueryPagedItemsWithRetry", "250", "100", "live-edc-tran-2026-06"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q missing %q", msg, want)
		}
	}
}

// TestHandleError_ResultSetTooLarge_FlaggedAndNonRetryable is the most important
// contract in this file. It pins TWO things at once:
//
//   - Detectability across the *DynamoDBError boundary: the WithRetry entry
//     points return *DynamoDBError (which is a flat struct with no Unwrap), so a
//     dedicated ResultSetTooLarge bool — mirroring TransactionConditionalCheckFailed
//     — is how callers of QueryPaginationDataWithRetry detect the fail-stop.
//   - No retry amplification: the fail-stop is deterministic, so it MUST NOT be
//     retried. QueryPaginationDataWithRetry recurses only when AllowRetry is
//     true; a retried walk would re-incur the full bounded walk N times. The
//     sentinel is a non-AWS error, so handleError's general branch keeps
//     AllowRetry=false — this test guards that the cap stays non-retryable.
func TestHandleError_ResultSetTooLarge_FlaggedAndNonRetryable(t *testing.T) {
	d := &DynamoDB{} // nil connection is fine — handleError doesn't use it

	// the wrapped sentinel as it would arrive from do_Query_Pagination_Data
	walkErr := newResultSetTooLargeError("do_Query_Pagination_Data", 1000, 1000, "live-edc-tran-2026-06")
	ddbErr := d.handleError(walkErr, "QueryPaginationDataNormal Failed:")
	if ddbErr == nil {
		t.Fatal("handleError returned nil for the fail-stop sentinel")
	}
	if !ddbErr.ResultSetTooLarge {
		t.Error("ResultSetTooLarge = false, want true — caller cannot detect the fail-stop on the *DynamoDBError path")
	}
	if ddbErr.AllowRetry {
		t.Error("AllowRetry = true, want false — the deterministic fail-stop must NOT be retried (retry amplification)")
	}
}

// TestHandleError_OrdinaryError_DoesNotSetResultSetTooLarge guards against the
// flag leaking onto unrelated errors.
func TestHandleError_OrdinaryError_DoesNotSetResultSetTooLarge(t *testing.T) {
	d := &DynamoDB{}
	ddbErr := d.handleError(errors.New("some other failure"))
	if ddbErr == nil {
		t.Fatal("handleError returned nil")
	}
	if ddbErr.ResultSetTooLarge {
		t.Error("ResultSetTooLarge = true for an ordinary error, want false")
	}
}

// =============================================================================
// Backward-compat + contract pins
// =============================================================================

// TestDynamoDB_ZeroValue_HardCapsAreUnlimited pins that a freshly constructed
// DynamoDB (as every DAL service constructs it today, setting none of the new
// fields) has both hard caps at 0 and therefore unlimited behavior — the
// validation-not-stricter-than-runtime guarantee in struct-field form.
func TestDynamoDB_ZeroValue_HardCapsAreUnlimited(t *testing.T) {
	d := &DynamoDB{TableName: "live-edc-tran-2026-06"}
	if d.MaxQueryPaginationPageWalk != 0 {
		t.Errorf("zero-value MaxQueryPaginationPageWalk = %d, want 0", d.MaxQueryPaginationPageWalk)
	}
	if d.MaxQueryPagedItems != 0 {
		t.Errorf("zero-value MaxQueryPagedItems = %d, want 0", d.MaxQueryPagedItems)
	}
	if exceedsHardCap(1<<62, d.MaxQueryPaginationPageWalk) || exceedsHardCap(1<<62, d.MaxQueryPagedItems) {
		t.Error("zero-value caps must be unlimited — existing callers must behave byte-identically to today")
	}
}

// TestWarnDefaults_ContractPin locks the soft-WARN defaults agreed with the
// operator (1000 pages / 100000 items). These are early-visibility thresholds,
// far above today's ~49-item partitions; changing them is a deliberate decision,
// not an accidental refactor — hence the pin, mirroring TestMaxTransactItems_ContractPin.
func TestWarnDefaults_ContractPin(t *testing.T) {
	if defaultWarnPaginationPageWalk != 1000 {
		t.Errorf("defaultWarnPaginationPageWalk = %d, want 1000", defaultWarnPaginationPageWalk)
	}
	if defaultWarnPagedItems != 100000 {
		t.Errorf("defaultWarnPagedItems = %d, want 100000", defaultWarnPagedItems)
	}
}

// =============================================================================
// Remediation of contrarian-review findings (detection-contract + boundary)
// =============================================================================

// TestDynamoDBError_Unwrap_ResultSetTooLarge pins the detectability fix for the
// *DynamoDBError boundary. The WithRetry entry points return *DynamoDBError; a
// caller (incl. the crud layer wrapping with %w) must be able to use
// errors.Is(err, ErrResultSetTooLarge). Since the struct is flat, Unwrap() must
// surface the sentinel exactly when the ResultSetTooLarge flag is set, and never
// otherwise (so unrelated *DynamoDBError values don't spuriously match).
func TestDynamoDBError_Unwrap_ResultSetTooLarge(t *testing.T) {
	capped := &DynamoDBError{ErrorMessage: "x", ResultSetTooLarge: true}
	if !errors.Is(capped, ErrResultSetTooLarge) {
		t.Error("errors.Is(*DynamoDBError{ResultSetTooLarge:true}, ErrResultSetTooLarge) = false, want true")
	}
	// and through a %w wrapper, as the crud layer now wraps it
	wrapped := fmt.Errorf("crud layer: %w", capped)
	if !errors.Is(wrapped, ErrResultSetTooLarge) {
		t.Error("errors.Is through %w wrapper = false, want true — crud-layer detection broken")
	}
	ordinary := &DynamoDBError{ErrorMessage: "some other failure"}
	if errors.Is(ordinary, ErrResultSetTooLarge) {
		t.Error("an ordinary *DynamoDBError must NOT match ErrResultSetTooLarge")
	}
}

// TestRewrapNonRetryable_PreservesFlags guards the P1 bug the contrarian review
// found: QueryPaginationDataWithRetry re-wrapped a non-retryable *DynamoDBError
// into a fresh one that copied ONLY ErrorMessage, silently dropping
// ResultSetTooLarge (and TransactionConditionalCheckFailed). The detection flag
// is the documented mechanism on this path, so it MUST survive the re-wrap.
func TestRewrapNonRetryable_PreservesFlags(t *testing.T) {
	src := &DynamoDBError{
		ErrorMessage:                      "do_Query... fail-stop",
		ResultSetTooLarge:                 true,
		TransactionConditionalCheckFailed: true,
	}
	got := rewrapNonRetryable("QueryPaginationDataWithRetry Failed: ", src)
	if got == nil {
		t.Fatal("rewrapNonRetryable returned nil")
	}
	if !got.ResultSetTooLarge {
		t.Error("ResultSetTooLarge dropped by re-wrap — the exact P1 bug this guards")
	}
	if !got.TransactionConditionalCheckFailed {
		t.Error("TransactionConditionalCheckFailed dropped by re-wrap")
	}
	if got.AllowRetry {
		t.Error("re-wrapped non-retryable error must keep AllowRetry=false")
	}
	if !strings.Contains(got.ErrorMessage, "QueryPaginationDataWithRetry Failed:") || !strings.Contains(got.ErrorMessage, "fail-stop") {
		t.Errorf("ErrorMessage = %q, want prefix + original", got.ErrorMessage)
	}
	if !errors.Is(got, ErrResultSetTooLarge) {
		t.Error("re-wrapped error must remain errors.Is-detectable via Unwrap")
	}
}

// TestFailStopOnMorePages pins the boundary fix: a fail-stop must fire ONLY when
// the result set is incomplete (more pages remain beyond the cap). A complete
// result set whose size lands exactly on the cap is NOT an error — failing it
// would reject a fully-retrieved, legitimately-complete query (the
// validation-not-stricter-than-runtime trap, and the cap-of-1 / zero-result
// false-positive the contrarian review flagged).
func TestFailStopOnMorePages(t *testing.T) {
	const capLimit = int64(100)
	cases := []struct {
		name      string
		count     int64
		morePages bool
		want      bool
	}{
		{"at cap, more pages remain -> fail-stop", 100, true, true},
		{"over cap, more pages remain -> fail-stop", 250, true, true},
		{"exactly at cap but COMPLETE -> no fail (never reject a complete set)", 100, false, false},
		{"over cap but COMPLETE -> no fail", 250, false, false},
		{"under cap, more pages -> no fail", 99, true, false},
	}
	for _, c := range cases {
		if got := failStopOnMorePages(c.count, capLimit, c.morePages); got != c.want {
			t.Errorf("%s: failStopOnMorePages(%d, %d, %v) = %v, want %v", c.name, c.count, capLimit, c.morePages, got, c.want)
		}
	}
	// cap=0 (unlimited) never fails, even with more pages
	if failStopOnMorePages(1<<62, 0, true) {
		t.Error("cap=0 must be unlimited even when more pages remain")
	}
}

// TestScanFailStop_ZeroValueUnlimited pins that the new Scan caps default to 0 =
// unlimited (same backward-compat contract as the query caps), so the existing
// ScanPagedItemsWithRetry behavior is preserved for every caller that sets nothing.
func TestScanFailStop_ZeroValueUnlimited(t *testing.T) {
	d := &DynamoDB{TableName: "live-edc-tran-2026-06"}
	if d.MaxScanPagedItems != 0 {
		t.Errorf("zero-value MaxScanPagedItems = %d, want 0", d.MaxScanPagedItems)
	}
	if failStopOnMorePages(1<<62, d.MaxScanPagedItems, true) {
		t.Error("zero-value scan cap must be unlimited")
	}
}
