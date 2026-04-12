package ratelimit

import (
	"testing"
	"time"
)

// TestInitWithPositiveRate verifies Init with a positive rate creates a limiter
// that can be used via Take().
func TestInitWithPositiveRate(t *testing.T) {
	rl := &RateLimiter{
		RateLimitPerSecond: 100,
	}
	rl.Init()

	got := rl.Take()
	if got.IsZero() {
		t.Error("Take() returned zero time after Init with positive rate")
	}
}

// TestInitWithZeroRate verifies Init with zero rate creates an unlimited limiter.
func TestInitWithZeroRate(t *testing.T) {
	rl := &RateLimiter{
		RateLimitPerSecond: 0,
	}
	rl.Init()

	got := rl.Take()
	if got.IsZero() {
		t.Error("Take() returned zero time after Init with zero rate (unlimited)")
	}
}

// TestInitWithNegativeRate verifies Init clamps negative rate to zero (unlimited).
func TestInitWithNegativeRate(t *testing.T) {
	rl := &RateLimiter{
		RateLimitPerSecond: -5,
	}
	rl.Init()

	// After Init, RateLimitPerSecond should have been clamped to 0
	if rl.RateLimitPerSecond != 0 {
		t.Errorf("RateLimitPerSecond = %d after Init with -5, expected 0", rl.RateLimitPerSecond)
	}

	got := rl.Take()
	if got.IsZero() {
		t.Error("Take() returned zero time after Init with negative rate (should be unlimited)")
	}
}

// TestTakeReturnsNonZeroTime verifies that Take always returns a non-zero time.
func TestTakeReturnsNonZeroTime(t *testing.T) {
	rl := &RateLimiter{
		RateLimitPerSecond: 1000,
	}
	rl.Init()

	for i := 0; i < 5; i++ {
		got := rl.Take()
		if got.IsZero() {
			t.Errorf("Take() call %d returned zero time", i+1)
		}
	}
}

// TestReInitAfterAlreadyInitialized verifies that calling Init() again
// reconfigures the rate limiter without panicking.
func TestReInitAfterAlreadyInitialized(t *testing.T) {
	rl := &RateLimiter{
		RateLimitPerSecond: 100,
	}
	rl.Init()

	// First take should work
	t1 := rl.Take()
	if t1.IsZero() {
		t.Fatal("First Take() returned zero time")
	}

	// Re-init with a different rate
	rl.RateLimitPerSecond = 500
	rl.Init()

	// Take should still work after re-init
	t2 := rl.Take()
	if t2.IsZero() {
		t.Error("Take() returned zero time after re-init")
	}
}

// TestInitWithoutSlack verifies Init with InitializeWithoutSlack=true.
func TestInitWithoutSlack(t *testing.T) {
	rl := &RateLimiter{
		RateLimitPerSecond:     100,
		InitializeWithoutSlack: true,
	}
	rl.Init()

	got := rl.Take()
	if got.IsZero() {
		t.Error("Take() returned zero time after Init with WithoutSlack")
	}
}

// TestTakeWithoutExplicitInit verifies that calling Take() without
// calling Init() first still works (via ensureLimiter lazy init).
func TestTakeWithoutExplicitInit(t *testing.T) {
	rl := &RateLimiter{
		RateLimitPerSecond: 50,
	}
	// Deliberately skip Init()

	got := rl.Take()
	if got.IsZero() {
		t.Error("Take() returned zero time without explicit Init()")
	}
}

// TestTakeTimestampsAreNonDecreasing verifies that successive Take() calls
// return non-decreasing timestamps.
func TestTakeTimestampsAreNonDecreasing(t *testing.T) {
	rl := &RateLimiter{
		RateLimitPerSecond: 10000, // high rate to avoid long test
	}
	rl.Init()

	var prev time.Time
	for i := 0; i < 10; i++ {
		got := rl.Take()
		if got.Before(prev) {
			t.Errorf("Take() call %d returned time %v which is before previous %v", i+1, got, prev)
		}
		prev = got
	}
}
