package s3

import (
	"context"
	"testing"
	"time"
)

func TestEnsureS3Ctx_CallerTimeout_TakesPrecedence(t *testing.T) {
	dur := 5 * time.Second
	ctx, cancel := ensureS3Ctx(context.Background(), false, &dur)
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

func TestEnsureS3Ctx_XrayCtx_Gets30sDeadline(t *testing.T) {
	ctx, cancel := ensureS3Ctx(context.Background(), true, nil)
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

func TestEnsureS3Ctx_NoTimeout_NoXray_Gets30sDeadline(t *testing.T) {
	ctx, cancel := ensureS3Ctx(context.Background(), false, nil)
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

func TestEnsureS3Ctx_NilSegCtx_WithTimeout_FallsBackToBackground(t *testing.T) {
	dur := 10 * time.Second
	ctx, cancel := ensureS3Ctx(nil, false, &dur)
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
