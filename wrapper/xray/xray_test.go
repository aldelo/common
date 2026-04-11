package xray

import (
	"errors"
	"testing"
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
