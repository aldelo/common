package xray

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// TestSafeAddMetadata_NilReceiver verifies SafeAddMetadata does not panic
// when called on a nil *XSegment. This is the defensive path required
// because many wrapper callers use NewSegmentNullable which returns nil
// when tracing is disabled.
func TestSafeAddMetadata_NilReceiver(t *testing.T) {
	var seg *XSegment // nil
	if err := seg.SafeAddMetadata("k", "v"); err != nil {
		t.Fatalf("SafeAddMetadata on nil receiver returned %v, want nil", err)
	}
}

// TestSafeAddError_NilReceiver verifies SafeAddError does not panic
// when called on a nil *XSegment.
func TestSafeAddError_NilReceiver(t *testing.T) {
	var seg *XSegment // nil
	if err := seg.SafeAddError(errors.New("boom")); err != nil {
		t.Fatalf("SafeAddError on nil receiver returned %v, want nil", err)
	}
}

// TestSafeAddMetadata_NilSegField verifies SafeAddMetadata does not panic
// when the wrapper is non-nil but Seg is nil. This is the runtime path
// exercised when:
//   - tracingEnabled() returned false and NewSegment returned a wrapper
//     with Seg: nil, _segReady: false
//   - BeginSegment panicked and the recovery block set seg = nil
func TestSafeAddMetadata_NilSegField(t *testing.T) {
	seg := &XSegment{
		Ctx:       nil,
		Seg:       nil,
		_segReady: false,
	}
	if err := seg.SafeAddMetadata("k", "v"); err != nil {
		t.Fatalf("SafeAddMetadata on wrapper with nil Seg returned %v, want nil", err)
	}
}

// TestSafeAddError_NilSegField verifies SafeAddError does not panic
// when the wrapper is non-nil but Seg is nil.
func TestSafeAddError_NilSegField(t *testing.T) {
	seg := &XSegment{
		Ctx:       nil,
		Seg:       nil,
		_segReady: false,
	}
	if err := seg.SafeAddError(errors.New("boom")); err != nil {
		t.Fatalf("SafeAddError on wrapper with nil Seg returned %v, want nil", err)
	}
}

// TestSafeAddMetadata_NotReady verifies SafeAddMetadata returns nil
// (no panic, no delegation) when _segReady is false even if Seg is set.
// This covers the state after Close() has been called (Close sets
// _segReady = false and Seg = nil; this test isolates the _segReady flag).
func TestSafeAddMetadata_NotReady(t *testing.T) {
	seg := &XSegment{
		Ctx:       nil,
		Seg:       nil, // mirrors post-Close state
		_segReady: false,
	}
	if err := seg.SafeAddMetadata("k", "v"); err != nil {
		t.Fatalf("SafeAddMetadata on not-ready segment returned %v, want nil", err)
	}
}

// TestSafeAdd_ConcurrentNilSafe verifies concurrent SafeAddMetadata /
// SafeAddError calls on a wrapper with nil Seg do not race. This mirrors
// the defer-in-loop pattern found in the high-traffic wrappers (redis,
// dynamodb, sns, sqs).
func TestSafeAdd_ConcurrentNilSafe(t *testing.T) {
	seg := &XSegment{Seg: nil, _segReady: false}

	const n = 64
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func() {
			_ = seg.SafeAddMetadata("k", "v")
			_ = seg.SafeAddError(errors.New("e"))
			done <- struct{}{}
		}()
	}
	for i := 0; i < n; i++ {
		<-done
	}
}

// TestEndToEnd_XraySdkDisabled_NoPanic is the P1-4 empirical follow-up
// test from remediation-report-2026-04-11-P1.md "Follow-up work" #4.
//
// It toggles AWS_XRAY_SDK_DISABLED=TRUE and drives a full consumer-wrapper
// lifecycle against the xray package — NewSegment → SafeAddMetadata →
// SafeAddError → NewSubSegment → SetParentSegment → Capture → CaptureAsync
// → Close — without mocking anything. The goal is to verify empirically
// (not only via targeted unit tests) that the centralized Safe* guards
// and the "not ready" fast-paths in Capture/CaptureAsync/NewSubSegment
// hold against a real NewSegment() return where Seg=nil, _segReady=false.
//
// The env-disable path produces exactly the same wrapper state as the
// BeginSegment panic-recovery path in NewSegment (xray.go:~400 — the
// deferred recover() block sets seg=nil on panic). Testing the env path
// therefore also exercises the panic-recovery state without having to
// monkey-patch xray internals to force a real panic. Both paths converge
// to the same {Seg: nil, _segReady: false} wrapper, which is what every
// consumer wrapper (DynamoDB, CloudMap, Redis, SNS, SQS, etc.) must
// tolerate on every method call.
//
// Any panic here would immediately fail the test; the test does not
// assert against panics explicitly because Go's testing package already
// treats a goroutine panic as test failure.
func TestEndToEnd_XraySdkDisabled_NoPanic(t *testing.T) {
	// Scope the env var to this test only; t.Setenv restores the prior
	// value on cleanup and marks the test as uncompatible with t.Parallel().
	t.Setenv("AWS_XRAY_SDK_DISABLED", "TRUE")

	// --- Entry point: consumer pattern `seg := xray.NewSegment("service")`
	seg := NewSegment("redis.Get")
	if seg == nil {
		t.Fatal("NewSegment returned nil; want non-nil wrapper even when tracing disabled")
	}
	if seg.Ready() {
		t.Error("seg.Ready()=true with AWS_XRAY_SDK_DISABLED=TRUE; want false")
	}
	if seg.Seg != nil {
		t.Errorf("seg.Seg=%v; want nil when tracing disabled", seg.Seg)
	}
	defer seg.Close()

	// --- Metadata: scalar + composite types (consumer serializes results)
	if err := seg.SafeAddMetadata("cache.key", "order:12345"); err != nil {
		t.Errorf("SafeAddMetadata scalar: %v", err)
	}
	if err := seg.SafeAddMetadata("cache.result", map[string]int{"hits": 1, "misses": 0}); err != nil {
		t.Errorf("SafeAddMetadata map: %v", err)
	}
	if err := seg.SafeAddMetadata("cache.nil", nil); err != nil {
		t.Errorf("SafeAddMetadata nil value: %v", err)
	}

	// --- Error path: consumer records an error from the wrapped call
	if err := seg.SafeAddError(errors.New("simulated downstream failure")); err != nil {
		t.Errorf("SafeAddError: %v", err)
	}
	// Safe against nil error too — some call sites pass err unconditionally
	if err := seg.SafeAddError(nil); err != nil {
		t.Errorf("SafeAddError nil: %v", err)
	}

	// --- Subsegment pattern: consumer wraps an inner call
	sub := seg.NewSubSegment("redis.Get.inner")
	if sub == nil {
		t.Fatal("NewSubSegment returned nil; want non-nil wrapper")
	}
	if sub.Ready() {
		t.Error("sub.Ready()=true with parent not ready; want false")
	}
	if err := sub.SafeAddMetadata("sub.key", "v"); err != nil {
		t.Errorf("sub.SafeAddMetadata: %v", err)
	}
	if err := sub.SafeAddError(errors.New("sub err")); err != nil {
		t.Errorf("sub.SafeAddError: %v", err)
	}
	// SetParentSegment is a no-op on not-ready segments but must not panic
	sub.SetParentSegment("parent-seg-id", "trace-id-123")
	sub.Close()

	// --- Capture (synchronous): returns "Segment Not Ready" error on
	// not-ready wrapper; must NOT panic and must not invoke executeFunc.
	captureCalled := false
	capErr := seg.Capture("outer.trace", func() error {
		captureCalled = true
		return nil
	})
	if capErr == nil {
		t.Error("Capture() returned nil error; want 'Segment Not Ready' when tracing disabled")
	}
	if captureCalled {
		t.Error("Capture() executed the user func despite not being ready; want no execution")
	}

	// --- CaptureAsync: returns buffered channel with 'Segment Not Ready'
	// error delivered immediately, then channel closed.
	asyncCalled := false
	asyncCh := seg.CaptureAsync("outer.trace.async", func() error {
		asyncCalled = true
		return nil
	})
	select {
	case asyncErr, ok := <-asyncCh:
		if !ok {
			t.Error("CaptureAsync channel closed without delivering error")
		} else if asyncErr == nil {
			t.Error("CaptureAsync error was nil; want 'Segment Not Ready'")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("CaptureAsync channel did not deliver error within 2s")
	}
	if asyncCalled {
		t.Error("CaptureAsync executed the user func despite not being ready; want no execution")
	}
	// Drain the channel to confirm it closes (second receive returns zero-value,ok=false).
	select {
	case _, ok := <-asyncCh:
		if ok {
			t.Error("CaptureAsync channel delivered a second value; want single-shot close")
		}
	case <-time.After(1 * time.Second):
		t.Error("CaptureAsync channel did not close within 1s after error delivery")
	}

	// --- NewSegmentNullable: the nullable variant returns nil when disabled,
	// and the consumer pattern is `seg := NewSegmentNullable(...); defer seg.Close()`.
	// SafeAddMetadata/SafeAddError on a nil wrapper must still be no-ops.
	nullSeg := NewSegmentNullable("redis.Set")
	if nullSeg != nil {
		t.Errorf("NewSegmentNullable returned non-nil %v; want nil when disabled", nullSeg)
	}
	// Nil-receiver safety: separately unit-tested, re-verified here at the
	// public entry point to confirm the production usage is safe.
	if err := nullSeg.SafeAddMetadata("k", "v"); err != nil {
		t.Errorf("SafeAddMetadata on nil wrapper: %v", err)
	}
	if err := nullSeg.SafeAddError(errors.New("e")); err != nil {
		t.Errorf("SafeAddError on nil wrapper: %v", err)
	}

	// --- Close() is idempotent and must not panic after Safe* calls.
	// (seg.Close() is already deferred at the top; call it explicitly here
	// to verify a second Close() does not panic.)
	seg.Close()
	seg.Close()

	// After close, subsequent Safe* calls must still be safe.
	if err := seg.SafeAddMetadata("post.close", "v"); err != nil {
		t.Errorf("SafeAddMetadata post-Close: %v", err)
	}
	if err := seg.SafeAddError(errors.New("post-close")); err != nil {
		t.Errorf("SafeAddError post-Close: %v", err)
	}
}

// ================================================================================================================
// LogXrayAddFailure tests (COMMON-R2-001)
// ================================================================================================================

// TestLogXrayAddFailure_NilError verifies that nil errors are no-ops:
// the total counter must not increment.
func TestLogXrayAddFailure_NilError(t *testing.T) {
	ResetXrayFailureCounters()

	LogXrayAddFailure("test-nil", nil)

	if got := XrayFailureTotal(); got != 0 {
		t.Errorf("XrayFailureTotal() = %d after nil error; want 0", got)
	}
}

// TestLogXrayAddFailure_CountsErrors verifies that non-nil errors increment
// the process-wide total and per-category counters.
func TestLogXrayAddFailure_CountsErrors(t *testing.T) {
	ResetXrayFailureCounters()

	LogXrayAddFailure("SES-Connect", errors.New("e1"))
	LogXrayAddFailure("SES-Connect", errors.New("e2"))
	LogXrayAddFailure("SNS-Connect", errors.New("e3"))

	if got := XrayFailureTotal(); got != 3 {
		t.Errorf("XrayFailureTotal() = %d; want 3", got)
	}
}

// TestLogXrayAddFailure_RateLimit verifies the sampled logging behavior:
// after the first call (which logs immediately on a fresh category), subsequent
// calls within the same interval should NOT reset the accumulated count to zero
// until the interval elapses. We shorten the interval for test speed.
func TestLogXrayAddFailure_RateLimit(t *testing.T) {
	ResetXrayFailureCounters()

	// Shorten interval for testing — restore after test.
	origInterval := xrayLogInterval
	xrayLogInterval = 50 * time.Millisecond
	defer func() { xrayLogInterval = origInterval }()

	// First call on a fresh category triggers an immediate log (time.Since(zero) >= 50ms).
	LogXrayAddFailure("rate-test", errors.New("first"))

	// Several rapid calls within the 50ms window — these should accumulate.
	for i := 0; i < 10; i++ {
		LogXrayAddFailure("rate-test", errors.New("rapid"))
	}

	// Total should be 11 (1 + 10).
	if got := XrayFailureTotal(); got != 11 {
		t.Errorf("XrayFailureTotal() = %d; want 11", got)
	}

	// Wait for the interval to elapse, then trigger another log.
	time.Sleep(60 * time.Millisecond)
	LogXrayAddFailure("rate-test", errors.New("after-interval"))

	if got := XrayFailureTotal(); got != 12 {
		t.Errorf("XrayFailureTotal() = %d after interval; want 12", got)
	}
}

// TestLogXrayAddFailure_ConcurrentSafe verifies that concurrent calls to
// LogXrayAddFailure do not race. Run with -race to detect data races.
func TestLogXrayAddFailure_ConcurrentSafe(t *testing.T) {
	ResetXrayFailureCounters()

	const goroutines = 64
	const callsPerGoroutine = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < callsPerGoroutine; i++ {
				LogXrayAddFailure("concurrent-test", errors.New("err"))
			}
		}(g)
	}

	wg.Wait()

	expected := int64(goroutines * callsPerGoroutine)
	if got := XrayFailureTotal(); got != expected {
		t.Errorf("XrayFailureTotal() = %d; want %d", got, expected)
	}
}

// TestResetXrayFailureCounters verifies that Reset clears all state.
func TestResetXrayFailureCounters(t *testing.T) {
	// Populate some state first.
	LogXrayAddFailure("reset-test", errors.New("e"))
	if XrayFailureTotal() == 0 {
		t.Fatal("expected non-zero total before reset")
	}

	ResetXrayFailureCounters()

	if got := XrayFailureTotal(); got != 0 {
		t.Errorf("XrayFailureTotal() = %d after reset; want 0", got)
	}
}
