package cloudmap

import (
	"context"
	"testing"
	"time"
)

func TestEnsureCloudMapCtx_CallerTimeout_TakesPrecedence(t *testing.T) {
	bg := context.Background()
	ctx, cancel := ensureCloudMapCtx(bg, false, []time.Duration{5 * time.Second})
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

func TestEnsureCloudMapCtx_XrayCtx_Gets30sDeadline(t *testing.T) {
	parent := context.Background()
	ctx, cancel := ensureCloudMapCtx(parent, true, nil)
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

func TestEnsureCloudMapCtx_NoTimeout_NoXray_Gets30sDeadline(t *testing.T) {
	ctx, cancel := ensureCloudMapCtx(context.Background(), false, nil)
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

func TestEnsureCloudMapCtx_NilSegCtx_WithTimeout_FallsBackToBackground(t *testing.T) {
	ctx, cancel := ensureCloudMapCtx(nil, false, []time.Duration{10 * time.Second})
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
