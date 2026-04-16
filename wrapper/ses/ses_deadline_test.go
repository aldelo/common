package ses

import (
	"context"
	"testing"
	"time"
)

func TestEnsureSESCtx_CallerTimeout_TakesPrecedence(t *testing.T) {
	bg := context.Background()
	ctx, cancel := ensureSESCtx(bg, false, []time.Duration{5 * time.Second})
	defer cancel()
	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline")
	}
	// Should be ~5s from now, not 30s
	remaining := time.Until(dl)
	if remaining > 6*time.Second || remaining < 4*time.Second {
		t.Fatalf("expected ~5s deadline, got %v", remaining)
	}
}

func TestEnsureSESCtx_XrayCtx_Gets30sDeadline(t *testing.T) {
	parent := context.Background()
	ctx, cancel := ensureSESCtx(parent, true, nil)
	defer cancel()
	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline")
	}
	remaining := time.Until(dl)
	if remaining > 31*time.Second || remaining < 29*time.Second {
		t.Fatalf("expected ~30s deadline, got %v", remaining)
	}
}

func TestEnsureSESCtx_NoTimeout_NoXray_Gets30sDeadline(t *testing.T) {
	ctx, cancel := ensureSESCtx(context.Background(), false, nil)
	defer cancel()
	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline")
	}
	remaining := time.Until(dl)
	if remaining > 31*time.Second || remaining < 29*time.Second {
		t.Fatalf("expected ~30s deadline, got %v", remaining)
	}
}

func TestEnsureSESCtx_NilSegCtx_WithTimeout_FallsBackToBackground(t *testing.T) {
	ctx, cancel := ensureSESCtx(nil, false, []time.Duration{10 * time.Second})
	defer cancel()
	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline")
	}
	remaining := time.Until(dl)
	if remaining > 11*time.Second || remaining < 9*time.Second {
		t.Fatalf("expected ~10s deadline, got %v", remaining)
	}
}
